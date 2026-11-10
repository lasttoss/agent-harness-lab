#!/usr/bin/env python3
"""Every number in an answer must already exist in the evidence pack.

The model may phrase a conclusion however it likes. It may not introduce a
quantity that nobody measured. This is the check that would have caught the
classic "reduces latency by 40%" line nobody can trace.
"""
from __future__ import annotations

import json
import re
from pathlib import Path

NUMBER = re.compile(r"\d+(?:[.,]\d+)?")
# Single digits and small ordinals are prose ("2 gates", "3 layers"), not claims.
TRIVIAL = {"0", "1", "2", "3", "4", "5"}


def extract_numbers(text: str) -> list[str]:
    out = []
    for raw in NUMBER.findall(text):
        norm = raw.replace(",", ".")
        if norm.endswith(".0"):
            norm = norm[:-2]
        if norm in TRIVIAL:
            continue
        out.append(norm)
    return out


def check(answer: str, evidence: dict) -> list[str]:
    allowed: set[str] = set()
    for v in evidence.get("numbers", []):
        s = str(v).replace(",", ".")
        if s.endswith(".0"):
            s = s[:-2]
        allowed.add(s)
    unsourced = [n for n in extract_numbers(answer) if n not in allowed]
    return sorted(set(unsourced))


def main(answer_path: str, evidence_path: str) -> int:
    answer = Path(answer_path).read_text(encoding="utf-8")
    evidence = json.loads(Path(evidence_path).read_text(encoding="utf-8"))
    bad = check(answer, evidence)
    if bad:
        print("FAIL number_check: unsourced numbers -> " + ", ".join(bad))
        return 1
    print("OK number_check")
    return 0


if __name__ == "__main__":
    import sys

    if len(sys.argv) != 3:
        print("usage: number_check.py <answer.md> <evidence.json>")
        raise SystemExit(2)
    raise SystemExit(main(sys.argv[1], sys.argv[2]))
