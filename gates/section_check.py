#!/usr/bin/env python3
"""A gated answer must contain the four sections that make it falsifiable."""
from __future__ import annotations

import json
from pathlib import Path

REQUIRED = ["Finding", "Evidence", "Confidence", "What would change my mind"]


def check(answer: str) -> list[str]:
    low = answer.lower()
    return [s for s in REQUIRED if s.lower() not in low]


def main(answer_path: str) -> int:
    answer = Path(answer_path).read_text(encoding="utf-8")
    missing = check(answer)
    if missing:
        print("FAIL section_check: missing -> " + ", ".join(missing))
        return 1
    print("OK section_check")
    return 0


if __name__ == "__main__":
    import sys

    if len(sys.argv) != 2:
        print("usage: section_check.py <answer.md>")
        raise SystemExit(2)
    raise SystemExit(main(sys.argv[1]))
