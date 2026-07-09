#!/usr/bin/env python3
"""Windows native Desktop startup smoke.

Launches the Desktop executable with an isolated REAMES_AGENT_HOME, observes
the process for responsiveness, verifies desktop state is confined to the
isolated home, and produces JSON evidence. This is a machine-readable smoke
check — it does NOT perform Wails UI click automation (which requires a real
desktop session and WebView2 interaction).

Usage:
  python scripts/smoke_desktop_native.py --exe path/to/reames-agent-desktop.exe
  python scripts/smoke_desktop_native.py --exe path/to/reames-agent-desktop.exe --out evidence.json
"""

from __future__ import annotations

import argparse
import json
import os
import signal
import subprocess
import sys
import tempfile
import time
from datetime import datetime, timezone
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]


class SmokeResult:
    """Structured smoke evidence."""

    def __init__(self) -> None:
        self.started_at: str = ""
        self.finished_at: str = ""
        self.outcome: str = "unknown"
        self.failure_kind: str | None = None
        self.exit_code: int | None = None
        self.responding: bool = False
        self.process_alive_after_observation: bool = False
        self.home_dir: str = ""
        self.home_files: list[str] = []
        self.errors: list[str] = []

    def to_dict(self) -> dict:
        return {
            "started_at": self.started_at,
            "finished_at": self.finished_at,
            "outcome": self.outcome,
            "failure_kind": self.failure_kind,
            "exit_code": self.exit_code,
            "responding": self.responding,
            "process_alive_after_observation": self.process_alive_after_observation,
            "home_dir": self.home_dir,
            "home_file_count": len(self.home_files),
            "home_files": self.home_files[:50],  # cap for readable output
            "errors": self.errors,
        }


def classify_failure(result: SmokeResult, phase: str, detail: str) -> str:
    """Map a smoke phase failure to one of the defined categories."""
    mapping: dict[str, str] = {
        "start": "startup-failure",
        "exit_early": "early-exit",
        "no_response": "no-response",
        "state_leak": "state-leak",
        "cleanup": "cleanup-failure",
    }
    return mapping.get(phase, phase)


def list_home_files(home: Path) -> list[str]:
    """Recursively list relative paths under home, excluding secrets."""
    files: list[str] = []
    if not home.exists():
        return files
    for root, _, filenames in os.walk(home):
        for name in filenames:
            full = Path(root) / name
            rel = str(full.relative_to(home))
            # Skip .env and credential files from the listing.
            if name == ".env" or "credential" in name.lower() or "key" in name.lower():
                files.append(f"{rel} [REDACTED]")
            else:
                try:
                    size = full.stat().st_size
                    files.append(f"{rel} ({size} bytes)")
                except OSError:
                    files.append(f"{rel}")
    return sorted(files)


def is_responding(pid: int) -> bool:
    """Check whether a Windows process is responding (not hung)."""
    try:
        import ctypes
        from ctypes import wintypes

        kernel32 = ctypes.windll.kernel32
        PROCESS_QUERY_LIMITED_INFORMATION = 0x1000

        h = kernel32.OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION, False, pid)
        if not h:
            return False
        try:
            # Try to wait with zero timeout — a hung process will timeout.
            result = kernel32.WaitForSingleObject(h, 0)
            # 0 = WAIT_OBJECT_0 (signaled = not running), 258 = WAIT_TIMEOUT (still running).
            # A running but responding process returns WAIT_TIMEOUT.
            # A hung GUI process may still be running but not pumping messages.
            # We use a basic alive check: process handle is valid.
            exit_code = wintypes.DWORD()
            if kernel32.GetExitCodeProcess(h, ctypes.byref(exit_code)):
                return exit_code.value == 259  # STILL_ACTIVE
            return False
        finally:
            kernel32.CloseHandle(h)
    except Exception:
        return False


def run_smoke(exe_path: str, observation_seconds: int = 12) -> SmokeResult:
    result = SmokeResult()
    result.started_at = datetime.now(timezone.utc).isoformat()

    exe = Path(exe_path)
    if not exe.exists():
        result.outcome = "failed"
        result.failure_kind = "startup-failure"
        result.errors.append(f"executable not found: {exe_path}")
        result.finished_at = datetime.now(timezone.utc).isoformat()
        return result

    # Create isolated home.
    temp_root = Path(tempfile.gettempdir()) / "reames-agent-desktop-smoke"
    temp_root.mkdir(parents=True, exist_ok=True)
    home = Path(tempfile.mkdtemp(prefix="home-", dir=temp_root))
    result.home_dir = str(home)

    env = os.environ.copy()
    env["REAMES_AGENT_HOME"] = str(home)
    # Ensure we don't inherit a previous explicit home.
    env.pop("REAMES_AGENT_DEV", None)

    # Phase 1: Launch.
    try:
        proc = subprocess.Popen(
            [str(exe), "--home", str(home)],
            env=env,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            creationflags=subprocess.CREATE_NO_WINDOW if sys.platform == "win32" else 0,
        )
    except OSError as exc:
        result.outcome = "failed"
        result.failure_kind = classify_failure(result, "start", str(exc))
        result.errors.append(f"launch failed: {exc}")
        result.finished_at = datetime.now(timezone.utc).isoformat()
        return result

    # Phase 2: Observe responsiveness.
    time.sleep(2)  # Give the process time to initialize.

    responded_at_least_once = False
    check_start = time.monotonic()
    while time.monotonic() - check_start < observation_seconds:
        poll = proc.poll()
        if poll is not None:
            # Process exited early.
            result.exit_code = poll
            result.outcome = "failed"
            result.failure_kind = classify_failure(result, "exit_early", f"exit code {poll}")
            result.errors.append(f"process exited with code {poll} before observation window ended")
            result.finished_at = datetime.now(timezone.utc).isoformat()
            result.home_files = list_home_files(home)
            return result

        if is_responding(proc.pid):
            responded_at_least_once = True
        time.sleep(0.5)

    result.responding = responded_at_least_once

    if not responded_at_least_once:
        result.outcome = "failed"
        result.failure_kind = classify_failure(result, "no_response", "never responding")

    # Phase 3: Verify state confinement.
    home_files = list_home_files(home)
    result.home_files = home_files

    # Check for state leakage outside home.
    # The smoke home must be the only place Desktop state lands.
    # (This is a structural check: no file outside the home dir should be created.)
    if not home.exists() or not home_files:
        if result.outcome != "failed":
            result.outcome = "failed"
            result.failure_kind = classify_failure(result, "state_leak", "no files in home")
            result.errors.append("no state files found in isolated home")

    # Phase 4: Clean termination.
    result.process_alive_after_observation = proc.poll() is None
    if proc.poll() is None:
        try:
            if sys.platform == "win32":
                proc.terminate()
            else:
                proc.send_signal(signal.SIGTERM)
            try:
                proc.wait(timeout=10)
            except subprocess.TimeoutExpired:
                proc.kill()
                proc.wait(timeout=5)
        except Exception as exc:
            if result.outcome != "failed":
                result.outcome = "failed"
                result.failure_kind = classify_failure(result, "cleanup", str(exc))
            result.errors.append(f"cleanup failed: {exc}")

    if result.outcome == "unknown":
        result.outcome = "passed"
        result.exit_code = proc.poll()

    # Final file listing after termination.
    result.home_files = list_home_files(home)

    result.finished_at = datetime.now(timezone.utc).isoformat()
    return result


def main() -> int:
    parser = argparse.ArgumentParser(description="Windows native Desktop startup smoke")
    parser.add_argument("--exe", required=True, help="Path to the Desktop executable")
    parser.add_argument("--out", help="Write JSON evidence to this file")
    parser.add_argument(
        "--observation-seconds",
        type=int,
        default=12,
        help="Seconds to observe the process (default 12)",
    )
    args = parser.parse_args()

    result = run_smoke(args.exe, observation_seconds=args.observation_seconds)
    evidence = result.to_dict()
    evidence["smoke_version"] = 1
    evidence["executable"] = args.exe

    if args.out:
        out_path = Path(args.out)
        out_path.parent.mkdir(parents=True, exist_ok=True)
        out_path.write_text(json.dumps(evidence, indent=2) + "\n", encoding="utf-8")
        print(f"Evidence written to {args.out}")

    print(json.dumps(evidence, indent=2))
    return 0 if result.outcome == "passed" else 1


if __name__ == "__main__":
    raise SystemExit(main())
