#!/usr/bin/env python3
"""Render one CHANGELOG section as GitHub release notes.

GitHub renders a single newline inside release notes as a hard line break, so
publishing the hard-wrapped CHANGELOG source verbatim produced a ragged column
of short lines with the right-hand side empty. This script extracts the section
for a version and unwraps each paragraph/bullet into one flowing line, keeping
headings, blank lines, nested lists, tables and fenced code blocks intact.

Usage:  scripts/release_notes.py v1.1.3 > notes.md
"""

from __future__ import annotations

import re
import sys
from pathlib import Path

CHANGELOG = Path(__file__).resolve().parent.parent / "CHANGELOG.md"

HEADING = re.compile(r"^##\s+(\S+)")
ORDERED = re.compile(r"^\d+[.)]\s")
BLOCK_START = ("#", "-", "*", ">", "|", "+")


def section(version: str, text: str) -> list[str]:
    """Return the lines of the `## <version>` section, without its heading."""
    captured: list[str] = []
    capturing = False
    for line in text.splitlines():
        match = HEADING.match(line)
        if match:
            if capturing:
                break
            capturing = match.group(1) == version
            continue
        if capturing:
            captured.append(line)
    return captured


def unwrap(lines: list[str]) -> list[str]:
    """Join soft-wrapped lines so the rendered output flows as paragraphs."""
    out: list[str] = []
    in_fence = False
    for line in lines:
        stripped = line.strip()
        if stripped.startswith("```"):
            in_fence = not in_fence
            out.append(line.rstrip())
            continue
        if in_fence:
            out.append(line.rstrip())
            continue
        if not stripped:
            out.append("")
            continue
        starts_block = stripped.startswith(BLOCK_START) or ORDERED.match(stripped)
        if starts_block or not out or out[-1] == "":
            out.append(stripped)
        else:
            out[-1] = f"{out[-1]} {stripped}"

    while out and not out[0].strip():
        out.pop(0)
    while out and not out[-1].strip():
        out.pop()
    return out


def main(argv: list[str]) -> int:
    if len(argv) != 2:
        print("usage: release_notes.py <version>", file=sys.stderr)
        return 2

    version = argv[1].strip().removeprefix("v")
    notes = unwrap(section(version, CHANGELOG.read_text(encoding="utf-8")))
    if not notes:
        print(f"See CHANGELOG.md for the changes in v{version}.")
        return 0
    print("\n".join(notes))
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv))
