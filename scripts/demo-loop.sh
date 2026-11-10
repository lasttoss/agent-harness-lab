#!/usr/bin/env bash
# End-to-end: layer-2 proxy + layer-1 loop, offline.
# Proves that the loop cannot spend past its budget and that the stop reason is
# written down, instead of a run that "just ended".
set -euo pipefail
cd "$(dirname "$0")/.."
ADDR=":8098"
rm -f out/loop.json out/loop-requests.jsonl out/loop-budget.json
go build -o bin/harness ./proxy
go build -o bin/loop ./cmd/loop
HARNESS_ADDR="$ADDR" HARNESS_PROVIDER=fixture HARNESS_LOG=out/loop-requests.jsonl \
HARNESS_BUDGET=out/loop-budget.json HARNESS_DAILY_TOKENS=100000 ./bin/harness &
PID=$!
trap 'kill $PID 2>/dev/null || true' EXIT
for _ in $(seq 1 50); do curl -sf "http://localhost${ADDR}/healthz" >/dev/null && break; sleep 0.1; done

echo "== run 1: normal, task completes"
./bin/loop -proxy "http://localhost${ADDR}" -steps 3 -task "say something then stop"

echo
echo "== run 2: per-step ceiling far below the prompt size (expect a written stop reason)"
./bin/loop -proxy "http://localhost${ADDR}" -steps 3 -max-step-tokens 1 -out out/loop-tight.json || true
python3 - <<'PY'
import json
r = json.load(open("out/loop-tight.json"))
print("   stop_reason:", r["stop_reason"])
assert r["stop_reason"].startswith("step_budget") or r["stop_reason"].startswith("total_budget"), r["stop_reason"]
PY

echo
echo "== run 1 record"
python3 - <<'PY'
import json
r = json.load(open("out/loop.json"))
print("   stop:", r["stop_reason"], "| steps:", len(r["steps"]), "| tokens:", r["total_tokens"], "| prefix_stable:", r["prefix_stable"])
for s in r["steps"]:
    print(f"   step {s['step']}: prompt={s['prompt_tokens']} cache_read={s['cache_read_tokens']} completion={s['completion_tokens']}")
PY
echo "DEMO-LOOP OK"
