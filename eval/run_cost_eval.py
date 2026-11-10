#!/usr/bin/env python3
"""Cost per successful task, measured through the harness.

Two runs per task, same evidence, same checks:
  A  cheap tier only, no escalation
  B  cheap tier, one escalation to the strong tier when the answer fails a check

The success criterion is a regex written before the run, applied by the loop, not
by the model and not by this script. A wrong answer costs money; that is the point
of the measurement.
"""
import argparse
import json
import os
import pathlib
import shutil
import subprocess
import sys
import tempfile
import time
import urllib.request

ROOT = pathlib.Path(__file__).resolve().parent.parent

# Cumulative, so the rows compare: repair-cheap only, then repair-cheap plus the
# expensive repair. Anything the first row fixes does not need the strong tier.
MODES = [
    ("cheap_only", False, False),
    ("retry_empty", False, True),
    ("retry_and_escalate", True, True),
]


def wait_health(url, timeout=15):
    end = time.time() + timeout
    while time.time() < end:
        try:
            with urllib.request.urlopen(url + "/healthz", timeout=1) as r:
                if r.status == 200:
                    return True
        except Exception:
            time.sleep(0.2)
    return False


def run_loop(task, evidence_path, out_path, cheap, strong, escalate, retry_empty, proxy):
    cmd = [
        str(ROOT / "bin" / "loop"),
        "-task", task["question"],
        "-evidence", str(evidence_path),
        "-expect", task["expect"],
        "-model", cheap,
        "-steps", "3",
        "-proxy", proxy,
        "-out", str(out_path),
    ]
    if task.get("derived"):
        cmd += ["-derived"]
    if retry_empty:
        cmd += ["-retry-empty"]
    if escalate and strong:
        cmd += ["-escalate", "-model-strong", strong]
    p = subprocess.run(cmd, capture_output=True, text=True, cwd=ROOT)
    if not pathlib.Path(out_path).exists():
        raise SystemExit(f"loop produced no record for {task['id']}: {p.stderr.strip()}")
    return json.loads(pathlib.Path(out_path).read_text())


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--mode", choices=["fixture", "upstream"], default="fixture")
    ap.add_argument("--tasks", default="eval/tasks.json")
    ap.add_argument("--cheap", default="deepseek-v4.1-flash")
    ap.add_argument("--strong", default="glm-5.3")
    ap.add_argument("--port", default="8123")
    ap.add_argument("--out", default="out/cost-eval.json")
    ap.add_argument("--daily-tokens", default="400000")
    ap.add_argument("--repo", default="../legacy-revive-lab", help="repository the file-based tasks read from")
    ap.add_argument("--max-bytes", type=int, default=9000, help="byte cap for concatenated file evidence")
    ap.add_argument("--repeats", type=int, default=1, help="runs per task per mode; >1 separates variance from effect")
    args = ap.parse_args()

    tasks = json.loads((ROOT / args.tasks).read_text())
    proxy = f"http://127.0.0.1:{args.port}"
    env = dict(os.environ)
    env["HARNESS_PROVIDER"] = args.mode
    env["HARNESS_ADDR"] = f":{args.port}"
    env["HARNESS_DAILY_TOKENS"] = args.daily_tokens
    env["HARNESS_BUDGET"] = tempfile.mktemp(suffix="-budget.json")
    env["HARNESS_LOG"] = str(ROOT / "out" / "requests-cost-eval.jsonl")
    pathlib.Path(env["HARNESS_LOG"]).parent.mkdir(exist_ok=True)

    proc = subprocess.Popen([str(ROOT / "bin" / "harness")], env=env, cwd=ROOT,
                            stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
    try:
        if not wait_health(proxy):
            raise SystemExit("proxy did not become healthy: " + (proc.stdout.read() if proc.stdout else ""))
        rows = []
        repo = (ROOT / args.repo).resolve()
        for t in tasks:
            ev = ROOT / "out" / f"ev-{t['id']}.txt"
            if t.get("files"):
                # No curated facts: the prompt is the file content, which is what a
                # real task looks like. Truncation is by bytes and is reported, because
                # a silently truncated file would turn a reading task into a guess.
                chunks, used = [], 0
                for rel in t["files"]:
                    body = (repo / rel).read_text()
                    room = args.max_bytes - used
                    if room <= 0:
                        chunks.append(f"\n--- {rel} --- (not included: byte cap reached)")
                        continue
                    clipped = body[:room]
                    used += len(clipped)
                    chunks.append(f"\n--- {rel} ---\n{clipped}")
                ev.write_text("".join(chunks))
            else:
                ev.write_text(t["evidence"])
            for mode, escalate, retry_empty in MODES:
              for rep in range(1, args.repeats + 1):
                out = ROOT / "out" / f"loop-{t['id']}-{mode}-r{rep}.json"
                rec = run_loop(t, ev, out, args.cheap, args.strong, escalate, retry_empty, proxy)
                rows.append({
                    "repeat": rep,
                    "task": t["id"],
                    "mode": mode,
                    "succeeded": rec["succeeded"],
                    "escalations": rec["escalations"],
                    "retries": rec["retries"],
                    "cost_usd": rec["total_cost_usd"],
                    "tokens": rec["total_tokens"],
                    "stop_reason": rec["stop_reason"],
                    "steps": [{k: s[k] for k in ("tier", "model", "verdict", "prompt_tokens", "cache_read_tokens", "completion_tokens", "cost_usd")} for s in rec["steps"]],
                })
                if args.repeats == 1:
                    print(f"  {t['id']:<18} {mode:<16} ok={rec['succeeded']!s:<5} esc={rec['escalations']} ${rec['total_cost_usd']:.6f} {rec['stop_reason']}", flush=True)
    finally:
        proc.terminate()
        try:
            proc.wait(timeout=5)
        except subprocess.TimeoutExpired:
            proc.kill()

    summary = {}
    for mode, _, _ in MODES:
        sel = [r for r in rows if r["mode"] == mode]
        ok = [r for r in sel if r["succeeded"]]
        total = sum(r["cost_usd"] for r in sel)
        summary[mode] = {
            "runs": len(sel),
            "succeeded": len(ok),
            "total_cost_usd": round(total, 6),
            "cost_per_success_usd": round(total / len(ok), 6) if ok else None,
            "retries": sum(r["retries"] for r in sel),
            "escalations": sum(r["escalations"] for r in sel),
        }

    # Repeat-to-repeat disagreement: same prompt, same model, different verdict. If
    # this is not zero, the gap between the two modes may be sampling noise rather
    # than the effect of escalation.
    flaky = {}
    for t in tasks:
        for mode, _, _ in MODES:
            sel = [r for r in rows if r["task"] == t["id"] and r["mode"] == mode]
            if len({r["succeeded"] for r in sel}) > 1:
                flaky[f"{t['id']}/{mode}"] = f"{sum(1 for r in sel if r['succeeded'])}/{len(sel)}"

    result = {
        "mode": args.mode,
        "repeats": args.repeats,
        "cheap": args.cheap,
        "strong": args.strong,
        "summary": summary,
        "flaky": flaky,
        "rows": rows,
    }
    pathlib.Path(ROOT / args.out).write_text(json.dumps(result, indent=2))

    print("\nmode                  success      spend      cost/success   retries  escalations")
    for mode, s in summary.items():
        cps = f"${s['cost_per_success_usd']:.6f}" if s["cost_per_success_usd"] is not None else "n/a"
        print(f"{mode:<21} {s['succeeded']}/{s['runs']:<10} ${s['total_cost_usd']:.6f}  {cps:<14} {s['retries']:<8} {s['escalations']}")
    if flaky:
        print("\nrepeat-to-repeat disagreement (same prompt, different verdict):")
        for k, v in flaky.items():
            print(f"  {k}: {v} runs passed")
    else:
        print("\nrepeat-to-repeat disagreement: none")
    if args.mode == "fixture":
        print("\nfixture mode: these numbers prove the plumbing only, not the cost of a real answer.")
    print(f"\nwrote {args.out}")


if __name__ == "__main__":
    main()
