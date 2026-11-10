#!/usr/bin/env python3
"""Run every gate over every case and compare the verdict with the expectation.

This is the gate's own test: a gate that never rejects anything passes no case
here, and a gate that rejects everything fails the accept cases. `make eval`
must be red before the gates can be trusted.
"""
from __future__ import annotations

import json
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(ROOT / "gates"))

import citation_check  # noqa: E402
import number_check  # noqa: E402
import section_check  # noqa: E402

CASES = sorted((ROOT / "eval" / "cases").glob("*.json"))


def run_case(case: dict) -> tuple[str, list[str]]:
    answer = case["answer"]
    evidence = case.get("evidence", {})
    reasons = []
    if number_check.check(answer, evidence):
        reasons.append("number:" + ",".join(number_check.check(answer, evidence)))
    if citation_check.check(answer, evidence):
        reasons.append("citation:" + ",".join(citation_check.check(answer, evidence)))
    if section_check.check(answer):
        reasons.append("section:" + ",".join(section_check.check(answer)))
    return ("reject" if reasons else "accept"), reasons


def main() -> int:
    rows = []
    failed = 0
    for path in CASES:
        case = json.loads(path.read_text(encoding="utf-8"))
        verdict, reasons = run_case(case)
        ok = verdict == case["expect"]
        if not ok:
            failed += 1
        rows.append((case["id"], case["expect"], verdict, ok, reasons, case.get("why", "")))

    width = max(len(r[0]) for r in rows)
    print(f"{'case'.ljust(width)}  {'expect':6} {'actual':6} result")
    for cid, exp, act, ok, reasons, _ in rows:
        print(f"{cid.ljust(width)}  {exp:6} {act:6} {'PASS' if ok else 'FAIL'}"
              + ("" if ok else f"   <- {', '.join(reasons)}"))
    print()
    print(f"{len(rows) - failed}/{len(rows)} cases behaved as expected")
    return 1 if failed else 0


if __name__ == "__main__":
    raise SystemExit(main())
