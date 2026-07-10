#!/usr/bin/env python3
"""Windows native Desktop startup smoke with an isolated Reames Agent home.

This proves that a built Desktop executable starts, owns a responsive native
window for the observation period, confines state to ``--home``, and can be
stopped after observation. It does not claim to exercise the Wails command
bridge, user clicks, or a zero exit status on ``WM_CLOSE``.
"""

from __future__ import annotations

import argparse
import ctypes
import hashlib
import json
import os
import subprocess
import sys
import tempfile
import time
from contextlib import contextmanager
from dataclasses import asdict, dataclass, field
from datetime import datetime, timezone
from pathlib import Path
from typing import Callable, Iterator


SCHEMA_VERSION = 1
MIN_OBSERVATION_SECONDS = 10
MAX_OBSERVATION_SECONDS = 300
POLL_INTERVAL_SECONDS = 0.5
REQUIRED_CONSECUTIVE_RESPONSES = 3
SMOKE_CONFIG = """\
[desktop]
close_behavior = "quit"
check_updates = false
onboarding_dismissed = true
language = "en"
"""


def utc_now() -> str:
    return datetime.now(timezone.utc).isoformat()


@dataclass
class Observation:
    elapsed_seconds: float = 0.0
    checks: int = 0
    responsive_checks: int = 0
    max_consecutive_responses: int = 0
    max_visible_windows: int = 0
    final_check_responsive: bool = False
    early_exit_code: int | None = None

    @property
    def responding(self) -> bool:
        return (
            self.early_exit_code is None
            and self.final_check_responsive
            and self.max_consecutive_responses >= REQUIRED_CONSECUTIVE_RESPONSES
        )


@dataclass
class SmokeResult:
    schema_version: int = SCHEMA_VERSION
    platform: str = sys.platform
    started_at: str = field(default_factory=utc_now)
    finished_at: str = ""
    outcome: str = "unknown"
    failure_kind: str | None = None
    artifact_name: str = ""
    artifact_sha256: str = ""
    artifact_size: int = 0
    executable_name: str = ""
    executable_sha256: str = ""
    executable_size: int = 0
    observation_seconds: int = 0
    observed_seconds: float = 0.0
    responding: bool = False
    responsive_checks: int = 0
    window_checks: int = 0
    max_visible_windows: int = 0
    process_alive_after_observation: bool = False
    observed_exit_code: int | None = None
    cleanup_exit_code: int | None = None
    cleanup_method: str = ""
    cleanup_ok: bool = False
    temp_cleaned: bool = False
    kept_temp: bool = False
    home_dir: str = ""
    home_files: list[str] = field(default_factory=list)
    boundary_changes: list[str] = field(default_factory=list)
    errors: list[str] = field(default_factory=list)

    def to_dict(self) -> dict[str, object]:
        return asdict(self)


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest().upper()


def validate_observation_seconds(value: int) -> int:
    if not MIN_OBSERVATION_SECONDS <= value <= MAX_OBSERVATION_SECONDS:
        raise ValueError(
            f"observation seconds must be between {MIN_OBSERVATION_SECONDS} "
            f"and {MAX_OBSERVATION_SECONDS}"
        )
    return value


def list_home_files(home: Path) -> list[str]:
    """List relative state metadata without reading file contents."""
    files: list[str] = []
    webview_files = 0
    webview_bytes = 0
    if not home.exists():
        return files
    for path in sorted(home.rglob("*")):
        if not path.is_file():
            continue
        rel = path.relative_to(home).as_posix()
        if rel.startswith("webview2/"):
            webview_files += 1
            try:
                webview_bytes += path.stat().st_size
            except OSError:
                pass
            continue
        if path.name == ".env" or "credential" in path.name.lower():
            files.append(f"{rel} [REDACTED]")
            continue
        try:
            files.append(f"{rel} ({path.stat().st_size} bytes)")
        except OSError:
            files.append(rel)
    if webview_files:
        files.append(
            f"webview2/ [{webview_files} files, {webview_bytes} bytes; contents not read]"
        )
    return files


def prepare_smoke_home(home: Path) -> None:
    """Seed deterministic Desktop behavior without credentials or user state."""
    home.mkdir(parents=True, exist_ok=True)
    (home / "config.toml").write_text(SMOKE_CONFIG, encoding="utf-8")


def default_boundary_roots(
    home: Path, executable_name: str = "reames-agent-desktop.exe"
) -> dict[str, Path]:
    """Return user roots that an isolated launch must leave unchanged."""
    roots: dict[str, Path] = {}
    appdata = os.environ.get("APPDATA", "")
    localappdata = os.environ.get("LOCALAPPDATA", "")
    candidates = {
        "APPDATA": Path(appdata) / "reames-agent" if appdata else None,
        "LOCALAPPDATA": Path(localappdata) / "reames-agent" if localappdata else None,
        "WEBVIEW2": Path(appdata) / executable_name
        if appdata and executable_name
        else None,
    }
    home_resolved = home.resolve(strict=False)
    for label, candidate in candidates.items():
        if candidate is None:
            continue
        root = candidate.resolve(strict=False)
        if root == home_resolved:
            continue
        roots[label] = root
    return roots


def snapshot_roots(roots: dict[str, Path]) -> dict[str, tuple[int, int]]:
    """Capture only path, size, and mtime; never read user state contents."""
    snapshot: dict[str, tuple[int, int]] = {}
    for label, root in roots.items():
        if not root.exists():
            continue
        for path in root.rglob("*"):
            if not path.is_file():
                continue
            try:
                stat = path.stat()
                rel = path.relative_to(root).as_posix()
                snapshot[f"<{label}>/{rel}"] = (stat.st_size, stat.st_mtime_ns)
            except OSError:
                continue
    return snapshot


def changed_snapshot(
    before: dict[str, tuple[int, int]], after: dict[str, tuple[int, int]]
) -> list[str]:
    changes: list[str] = []
    for path in sorted(set(before) | set(after)):
        if path not in before:
            changes.append(f"added {path}")
        elif path not in after:
            changes.append(f"removed {path}")
        elif before[path] != after[path]:
            changes.append(f"modified {path}")
    return changes


@contextmanager
def managed_smoke_home(
    keep_temp: bool, temp_parent: Path | None = None
) -> Iterator[Path]:
    parent = temp_parent or Path(tempfile.gettempdir()) / "reames-agent-desktop-smoke"
    parent.mkdir(parents=True, exist_ok=True)
    if keep_temp:
        yield Path(tempfile.mkdtemp(prefix="home-", dir=parent))
        return
    with tempfile.TemporaryDirectory(prefix="home-", dir=parent) as raw:
        yield Path(raw)


def _windows_for_pid(pid: int) -> list[int]:
    from ctypes import wintypes

    user32 = ctypes.WinDLL("user32", use_last_error=True)
    callback_type = ctypes.WINFUNCTYPE(wintypes.BOOL, wintypes.HWND, wintypes.LPARAM)
    user32.EnumWindows.argtypes = [callback_type, wintypes.LPARAM]
    user32.EnumWindows.restype = wintypes.BOOL
    user32.GetWindowThreadProcessId.argtypes = [
        wintypes.HWND,
        ctypes.POINTER(wintypes.DWORD),
    ]
    user32.GetWindowThreadProcessId.restype = wintypes.DWORD
    user32.IsWindowVisible.argtypes = [wintypes.HWND]
    user32.IsWindowVisible.restype = wintypes.BOOL
    windows: list[int] = []

    def callback(hwnd: int, _lparam: int) -> bool:
        owner = wintypes.DWORD()
        user32.GetWindowThreadProcessId(hwnd, ctypes.byref(owner))
        if owner.value == pid and user32.IsWindowVisible(hwnd):
            windows.append(int(hwnd))
        return True

    callback_ref = callback_type(callback)
    if not user32.EnumWindows(callback_ref, 0):
        error = ctypes.get_last_error()
        if error:
            raise OSError(error, "EnumWindows failed")
    return windows


def responsive_windows(pid: int) -> tuple[bool, int]:
    """Probe the target GUI message pump with WM_NULL and a bounded timeout."""
    if sys.platform != "win32":
        return False, 0
    from ctypes import wintypes

    user32 = ctypes.WinDLL("user32", use_last_error=True)
    wm_null = 0x0000
    smto_block = 0x0001
    smto_abort_if_hung = 0x0002
    user32.SendMessageTimeoutW.argtypes = [
        wintypes.HWND,
        wintypes.UINT,
        wintypes.WPARAM,
        wintypes.LPARAM,
        wintypes.UINT,
        wintypes.UINT,
        ctypes.POINTER(ctypes.c_size_t),
    ]
    user32.SendMessageTimeoutW.restype = wintypes.LPARAM
    windows = _windows_for_pid(pid)
    any_responsive = False
    for hwnd in windows:
        response = ctypes.c_size_t()
        ok = user32.SendMessageTimeoutW(
            hwnd,
            wm_null,
            0,
            0,
            smto_block | smto_abort_if_hung,
            1000,
            ctypes.byref(response),
        )
        any_responsive = any_responsive or bool(ok)
    return any_responsive, len(windows)


def request_window_close(pid: int) -> int:
    if sys.platform != "win32":
        return 0
    user32 = ctypes.WinDLL("user32", use_last_error=True)
    wm_close = 0x0010
    from ctypes import wintypes

    user32.PostMessageW.argtypes = [
        wintypes.HWND,
        wintypes.UINT,
        wintypes.WPARAM,
        wintypes.LPARAM,
    ]
    user32.PostMessageW.restype = wintypes.BOOL
    windows = _windows_for_pid(pid)
    for hwnd in windows:
        user32.PostMessageW(hwnd, wm_close, 0, 0)
    return len(windows)


def observe_process(
    proc: subprocess.Popen,
    seconds: int,
    responder: Callable[[int], tuple[bool, int]] = responsive_windows,
    clock: Callable[[], float] = time.monotonic,
    sleeper: Callable[[float], None] = time.sleep,
) -> Observation:
    observation = Observation()
    start = clock()
    consecutive = 0
    while clock() - start < seconds:
        exit_code = proc.poll()
        if exit_code is not None:
            observation.early_exit_code = exit_code
            break
        responsive, window_count = responder(proc.pid)
        observation.checks += 1
        observation.max_visible_windows = max(
            observation.max_visible_windows, window_count
        )
        observation.final_check_responsive = responsive
        if responsive:
            observation.responsive_checks += 1
            consecutive += 1
            observation.max_consecutive_responses = max(
                observation.max_consecutive_responses, consecutive
            )
        else:
            consecutive = 0
        remaining = seconds - (clock() - start)
        if remaining > 0:
            sleeper(min(POLL_INTERVAL_SECONDS, remaining))
    observation.elapsed_seconds = max(0.0, clock() - start)
    return observation


def cleanup_process(proc: subprocess.Popen, errors: list[str]) -> tuple[bool, str]:
    if proc.poll() is not None:
        return True, "already-exited"
    try:
        request_window_close(proc.pid)
        proc.wait(timeout=8)
        return True, "wm-close"
    except subprocess.TimeoutExpired:
        pass
    except Exception as exc:  # pragma: no cover - OS-specific defensive path
        errors.append(f"graceful close failed: {exc}")
    try:
        proc.terminate()
        proc.wait(timeout=5)
        return True, "terminate"
    except subprocess.TimeoutExpired:
        pass
    except Exception as exc:  # pragma: no cover - OS-specific defensive path
        errors.append(f"terminate failed: {exc}")
    try:
        proc.kill()
        proc.wait(timeout=5)
        return True, "kill"
    except Exception as exc:  # pragma: no cover - OS-specific defensive path
        errors.append(f"kill failed: {exc}")
        return proc.poll() is not None, "kill-failed"


def _fail(result: SmokeResult, kind: str, message: str) -> None:
    if result.failure_kind is None:
        result.failure_kind = kind
    result.outcome = "failed"
    result.errors.append(message)


def run_smoke(
    exe_path: str,
    observation_seconds: int = 12,
    keep_temp: bool = False,
    artifact_path: str | None = None,
) -> SmokeResult:
    result = SmokeResult(
        platform=sys.platform,
        observation_seconds=observation_seconds,
        kept_temp=keep_temp,
    )
    proc: subprocess.Popen | None = None
    home: Path | None = None

    try:
        validate_observation_seconds(observation_seconds)
    except ValueError as exc:
        _fail(result, "invalid-arguments", str(exc))
        result.finished_at = utc_now()
        return result

    exe = Path(exe_path).resolve(strict=False)
    result.executable_name = exe.name
    if sys.platform != "win32":
        _fail(result, "unsupported-platform", "native Desktop smoke requires Windows")
        result.finished_at = utc_now()
        return result
    if not exe.is_file():
        _fail(result, "startup-failure", f"executable not found: {exe}")
        result.finished_at = utc_now()
        return result

    artifact: Path | None = None
    if artifact_path:
        artifact = Path(artifact_path).resolve(strict=False)
        result.artifact_name = artifact.name
        if not artifact.is_file():
            _fail(result, "startup-failure", f"artifact not found: {artifact}")
            result.finished_at = utc_now()
            return result

    try:
        result.executable_size = exe.stat().st_size
        result.executable_sha256 = sha256_file(exe)
        if artifact is not None:
            result.artifact_size = artifact.stat().st_size
            result.artifact_sha256 = sha256_file(artifact)
    except OSError as exc:
        _fail(result, "startup-failure", f"inspect executable: {exc}")
        result.finished_at = utc_now()
        return result

    with managed_smoke_home(keep_temp) as managed_home:
        home = managed_home
        result.home_dir = str(home)
        prepare_smoke_home(home)
        boundary_roots = default_boundary_roots(home, exe.name)
        boundary_before = snapshot_roots(boundary_roots)
        env = os.environ.copy()
        env["REAMES_AGENT_HOME"] = str(home)
        env.pop("REAMES_AGENT_STATE_HOME", None)
        env.pop("REAMES_AGENT_CACHE_HOME", None)
        env.pop("REAMES_AGENT_DEV", None)

        try:
            proc = subprocess.Popen(
                [str(exe), "--home", str(home)],
                env=env,
                stdin=subprocess.DEVNULL,
                stdout=subprocess.DEVNULL,
                stderr=subprocess.DEVNULL,
                creationflags=subprocess.CREATE_NO_WINDOW,
            )
        except OSError as exc:
            _fail(result, "startup-failure", f"launch failed: {exc}")
        else:
            try:
                observation = observe_process(proc, observation_seconds)
                result.observed_seconds = round(observation.elapsed_seconds, 3)
                result.responding = observation.responding
                result.responsive_checks = observation.responsive_checks
                result.window_checks = observation.checks
                result.max_visible_windows = observation.max_visible_windows
                result.observed_exit_code = observation.early_exit_code
                result.process_alive_after_observation = proc.poll() is None
                if observation.early_exit_code is not None:
                    _fail(
                        result,
                        "early-exit",
                        f"process exited with code {observation.early_exit_code} before the observation window ended",
                    )
                elif not observation.responding:
                    _fail(
                        result,
                        "no-response",
                        "native window did not produce three consecutive bounded message-pump responses through the final check",
                    )
            except Exception as exc:
                _fail(result, "no-response", f"window response probe failed: {exc}")
            finally:
                result.cleanup_ok, result.cleanup_method = cleanup_process(
                    proc, result.errors
                )
                result.cleanup_exit_code = proc.poll()
                if not result.cleanup_ok:
                    _fail(result, "cleanup-failure", "Desktop process could not be stopped")

        result.home_files = list_home_files(home)
        if not any(
            Path(entry.split(" (", 1)[0]).name.startswith("desktop-")
            for entry in result.home_files
        ):
            _fail(result, "state-missing", "no Desktop state file was written inside the isolated home")

        boundary_after = snapshot_roots(boundary_roots)
        result.boundary_changes = changed_snapshot(boundary_before, boundary_after)
        if result.boundary_changes:
            _fail(result, "state-leak", "default user state changed during isolated launch")

        if result.outcome == "unknown":
            result.outcome = "passed"

    if home is not None:
        result.temp_cleaned = not home.exists()
        if not keep_temp and not result.temp_cleaned:
            _fail(result, "cleanup-failure", "temporary smoke home was not removed")
    result.finished_at = utc_now()
    return result


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--exe", required=True, help="Path to the Desktop executable")
    parser.add_argument("--artifact", help="Candidate package that installed the executable")
    parser.add_argument("--out", help="Write JSON evidence to this path")
    parser.add_argument(
        "--observation-seconds",
        type=int,
        default=12,
        help=f"Observation duration ({MIN_OBSERVATION_SECONDS}-{MAX_OBSERVATION_SECONDS} seconds)",
    )
    parser.add_argument(
        "--keep-temp",
        action="store_true",
        help="Keep the isolated home for local debugging",
    )
    args = parser.parse_args(argv)
    try:
        validate_observation_seconds(args.observation_seconds)
    except ValueError as exc:
        parser.error(str(exc))

    result = run_smoke(
        args.exe,
        observation_seconds=args.observation_seconds,
        keep_temp=args.keep_temp,
        artifact_path=args.artifact,
    )
    evidence = result.to_dict()
    if args.out:
        out_path = Path(args.out)
        out_path.parent.mkdir(parents=True, exist_ok=True)
        out_path.write_text(json.dumps(evidence, indent=2) + "\n", encoding="utf-8")
        print(f"Evidence written to {out_path}")
    print(json.dumps(evidence, indent=2))
    return 0 if result.outcome == "passed" else 1


if __name__ == "__main__":
    raise SystemExit(main())
