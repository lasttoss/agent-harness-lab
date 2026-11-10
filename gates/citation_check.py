#!/usr/bin/env python3
"""Every file path quoted in an answer must exist in the evidence pack.

A citation to a file nobody can open is worse than no citation: it reads as
proof and is not one.
"""
from __future__ import annotations

import json
import re
from pathlib import Path

# path-like tokens: contain a slash and end with a short extension
PATHY = re.compile(r"[\w./-]*/[\w.-]+\.[A-Za-z0-9]{1,6}\b")


def check(answer: str, evidence: dict) -> list[str]:
    known = set(evidence.get("files", []))
    known_by_name = {Path(p).name for p in known}
    bad = []
    for token in PATHY.findall(answer):
        token = token.strip("`'\"()[]{}.,;:")
        if token in known or Path(token).name in known_by_name:
            continue
        bad.append(token)
    return sorted(set(bad))


def main(answer_path: str, evidence_path: str) -> int:
    answer = Path(answer_path).read_text(encoding="utf-8")
    evidence = json.loads(Path(evidence_path).read_text(encoding="utf-8"))
    bad = check(answer, evidence)
    if bad:
        print("FAIL citation_check: unknown paths -> " + ", ".join(bad))
        return 1
    print("OK citation_check")
    return 0


if __name__ == "__main__":
    import sys

    if len(sys.argv) != 3:
        print("usage: citation_check.py <answer.md> <evidence.json>")
        raise SystemExit(2)
    raise SystemExit(main(sys.argv[1], sys.argv[2]))
