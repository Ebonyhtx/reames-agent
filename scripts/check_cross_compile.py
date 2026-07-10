#!/usr/bin/env python3
"""Cross-compile contract: verify that the project builds for all 6 targets
with CGO_ENABLED=0. This is a local contract check — actual signed release
binaries require code signing certificates (external-blocked).
"""

from __future__ import annotations

import platform
import subprocess
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]

TARGETS = [
    ("linux", "amd64"),
    ("linux", "arm64"),
    ("darwin", "amd64"),
    ("darwin", "arm64"),
    ("windows", "amd64"),
    ("windows", "arm64"),
]


def run(args: list[str], **kwargs) -> subprocess.CompletedProcess:
    return subprocess.run(args, cwd=ROOT, check=False, text=True, capture_output=True, **kwargs)


def check_cross_compile() -> int:
    host_os = platform.system().lower()
    failures = 0
    tested = 0

    for goos, goarch in TARGETS:
        env = {"GOOS": goos, "GOARCH": goarch, "CGO_ENABLED": "0"}
        result = run(
            ["go", "build", "-o", f"bin/reames-agent-{goos}-{goarch}{'.exe' if goos == 'windows' else ''}", "./cmd/reames-agent"],
            env={**__import__("os").environ, **env},
            timeout=120,
        )
        tested += 1
        status = "PASS" if result.returncode == 0 else "FAIL"
        if result.returncode != 0:
            failures += 1
            print(f"  {status}  {goos}/{goarch}: {result.stderr.strip()[:200]}")
        else:
            print(f"  {status}  {goos}/{goarch}")

    print(f"\nCross-compile: {tested - failures}/{tested} targets passed")
    return failures


if __name__ == "__main__":
    sys.exit(check_cross_compile())
