#!/usr/bin/env python3
"""Installed Desktop recovery smoke for Linux, macOS, and Windows.

The smoke exercises the packaged Guard against an isolated home. It proves
that a damaged config and credential file do not prevent a forced Safe Mode
shell from reaching DOM-ready, that config quarantine is exactly undoable,
and that every supported derived Desktop state is quarantined rather than
deleted. Evidence contains hashes and bounded summaries, never credential
contents or raw recovery paths.
"""

from __future__ import annotations

import argparse
import ctypes
import hashlib
import json
import os
import shutil
import signal
import subprocess
import sys
import tempfile
import time
from dataclasses import asdict, dataclass, field
from datetime import datetime, timezone
from pathlib import Path
from typing import Callable


SCHEMA_VERSION = 1
DEFAULT_LAUNCH_TIMEOUT_SECONDS = 20.0
POLL_INTERVAL_SECONDS = 0.25
TEMP_CLEANUP_RETRY_SECONDS = (0.1, 0.2, 0.4, 0.8, 1.0, 1.5, 2.0, 3.0)
INVALID_CONFIG = b"[desktop\nthis is not valid toml\n"
SYNTHETIC_ENV = b"OPENAI_API_KEY=smoke-not-a-real-secret\nMALFORMED ENV LINE\n"
DERIVED_FIXTURES = {
    "tabs": ("desktop-tabs.json", b'{"smoke":"tabs"}\n'),
    "projects": ("desktop-projects.json", b'{"smoke":"projects"}\n'),
    "window": ("desktop-window.json", b'{"smoke":"window"}\n'),
    "zoom": ("desktop-zoom.json", b'{"smoke":"zoom"}\n'),
}


def utc_now() -> str:
    return datetime.now(timezone.utc).isoformat()


def host_platform() -> str:
    if sys.platform.startswith("linux"):
        return "linux"
    if sys.platform == "darwin":
        return "darwin"
    if sys.platform == "win32":
        return "windows"
    return sys.platform


@dataclass
class RecoverySmokeResult:
    schema_version: int = SCHEMA_VERSION
    platform: str = field(default_factory=host_platform)
    started_at: str = field(default_factory=utc_now)
    finished_at: str = ""
    outcome: str = "unknown"
    failure_kind: str | None = None
    artifact_name: str = ""
    artifact_sha256: str = ""
    executable_name: str = ""
    executable_sha256: str = ""
    guard_name: str = ""
    guard_sha256: str = ""
    safe_mode_ready: bool = False
    safe_mode_phase: str = ""
    safe_mode_pid_recorded: bool = False
    safe_mode_process_alive: bool = False
    safe_mode_cleanup_ok: bool = False
    config_unchanged_during_safe_mode: bool = False
    credentials_unchanged_during_safe_mode: bool = False
    config_repaired: bool = False
    config_undo_exact: bool = False
    derived_state_quarantined: bool = False
    final_guard_clean: bool = False
    boundary_changes: list[str] = field(default_factory=list)
    commands: list[dict[str, object]] = field(default_factory=list)
    checks: dict[str, object] = field(default_factory=dict)
    home_files: list[str] = field(default_factory=list)
    temp_cleaned: bool = False
    kept_temp: bool = False
    errors: list[str] = field(default_factory=list)

    def to_dict(self) -> dict[str, object]:
        return asdict(self)


def sha256_bytes(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest().upper()


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest().upper()


def smoke_environment(home: Path) -> dict[str, str]:
    env = os.environ.copy()
    for name in (
        "REAMES_AGENT_SAFE_MODE",
        "REAMES_AGENT_PENDING_HELPER",
        "REAMES_AGENT_PENDING_TARGET",
    ):
        env.pop(name, None)
    env["REAMES_AGENT_HOME"] = str(home)
    env["REAMES_AGENT_STATE_HOME"] = str(home)
    env["REAMES_AGENT_CACHE_HOME"] = str(home / "cache")
    return env


def guard_launch_command(guard: Path, executable: Path) -> list[str]:
    # The recovery smoke must own the Guard and Desktop as one foreground
    # process tree. The packaged launcher defaults to detached mode, which can
    # leave the Safe Mode child alive and racing the derived-state fixtures.
    return [
        str(guard),
        "launch",
        "--detach=false",
        "--app",
        str(executable),
        "--safe-mode",
    ]


def default_boundary_roots(platform_name: str, executable_name: str) -> dict[str, Path]:
    user_home = Path.home()
    if platform_name == "windows":
        roots: dict[str, Path] = {}
        appdata = os.environ.get("APPDATA", "")
        localappdata = os.environ.get("LOCALAPPDATA", "")
        if appdata:
            roots["APPDATA"] = Path(appdata) / "reames-agent"
            roots["WEBVIEW2"] = Path(appdata) / executable_name
        if localappdata:
            roots["LOCALAPPDATA"] = Path(localappdata) / "reames-agent"
        return {label: path.resolve(strict=False) for label, path in roots.items()}
    roots = {"DEFAULT_HOME": user_home / ".reames-agent"}
    if platform_name == "darwin":
        roots["DEFAULT_CACHE"] = user_home / "Library" / "Caches" / "reames-agent"
        roots["LEGACY_SUPPORT"] = user_home / "Library" / "Application Support" / "reames-agent"
    else:
        roots["DEFAULT_CACHE"] = Path(os.environ.get("XDG_CACHE_HOME", user_home / ".cache")) / "reames-agent"
        roots["LEGACY_SUPPORT"] = Path(os.environ.get("XDG_CONFIG_HOME", user_home / ".config")) / "reames-agent"
    return {label: path.resolve(strict=False) for label, path in roots.items()}


def snapshot_roots(roots: dict[str, Path]) -> dict[str, tuple[int, int]]:
    snapshot: dict[str, tuple[int, int]] = {}
    for label, root in roots.items():
        if not root.exists():
            continue
        for path in root.rglob("*"):
            if not path.is_file():
                continue
            try:
                stat = path.stat()
                snapshot[f"<{label}>/{path.relative_to(root).as_posix()}"] = (
                    stat.st_size,
                    stat.st_mtime_ns,
                )
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


def list_home_files(home: Path) -> list[str]:
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


def remove_tree_with_retries(path: Path) -> None:
    for delay in (*TEMP_CLEANUP_RETRY_SECONDS, None):
        try:
            shutil.rmtree(path)
            return
        except FileNotFoundError:
            if not path.exists():
                return
            if delay is None:
                raise
            time.sleep(delay)
        except OSError:
            if delay is None:
                raise
            time.sleep(delay)


def summarize_report(report: dict[str, object]) -> dict[str, object]:
    config = report.get("config") if isinstance(report.get("config"), dict) else {}
    checks = config.get("checks") if isinstance(config, dict) else []
    findings = report.get("findings") if isinstance(report.get("findings"), list) else []
    return {
        "schemaVersion": report.get("schemaVersion"),
        "safeModeRequested": report.get("safeModeRequested"),
        "safeModeRecommended": report.get("safeModeRecommended"),
        "startupPhase": (report.get("startup") or {}).get("phase") if isinstance(report.get("startup"), dict) else None,
        "config": [
            {
                "scope": item.get("scope"),
                "exists": item.get("exists"),
                "valid": item.get("valid"),
            }
            for item in checks
            if isinstance(item, dict)
        ],
        "findingCodes": [
            f"{item.get('severity')}:{item.get('code')}"
            for item in findings
            if isinstance(item, dict)
        ],
    }


def _record_command(
    result: RecoverySmokeResult,
    name: str,
    completed: subprocess.CompletedProcess[str],
    expected: tuple[int, ...],
) -> None:
    result.commands.append(
        {"name": name, "exitCode": completed.returncode, "expectedExitCodes": list(expected)}
    )


def run_json_command(
    result: RecoverySmokeResult,
    name: str,
    command: list[str],
    env: dict[str, str],
    cwd: Path,
    expected: tuple[int, ...] = (0,),
) -> dict[str, object] | list[object]:
    completed = subprocess.run(
        command,
        cwd=cwd,
        env=env,
        check=False,
        capture_output=True,
        text=True,
        timeout=30,
    )
    _record_command(result, name, completed, expected)
    if completed.returncode not in expected:
        detail = completed.stderr.strip() or completed.stdout.strip()
        raise RuntimeError(f"{name} exited {completed.returncode}: {detail[:500]}")
    try:
        parsed = json.loads(completed.stdout)
    except json.JSONDecodeError as exc:
        raise RuntimeError(f"{name} did not emit JSON: {exc}") from exc
    if not isinstance(parsed, (dict, list)):
        raise RuntimeError(f"{name} emitted an unexpected JSON value")
    return parsed


def pid_is_alive(pid: int) -> bool:
    if pid <= 0:
        return False
    if sys.platform == "win32":
        process_query_limited_information = 0x1000
        still_active = 259
        handle = ctypes.windll.kernel32.OpenProcess(
            process_query_limited_information, False, pid
        )
        if not handle:
            return False
        try:
            code = ctypes.c_ulong()
            return bool(ctypes.windll.kernel32.GetExitCodeProcess(handle, ctypes.byref(code))) and code.value == still_active
        finally:
            ctypes.windll.kernel32.CloseHandle(handle)
    try:
        os.kill(pid, 0)
        return True
    except (ProcessLookupError, PermissionError):
        return False


def read_startup_state(path: Path) -> dict[str, object] | None:
    try:
        value = json.loads(path.read_text(encoding="utf-8"))
    except (FileNotFoundError, OSError, json.JSONDecodeError):
        return None
    return value if isinstance(value, dict) else None


def wait_for_safe_mode_ready(
    path: Path,
    timeout_seconds: float,
    state_reader: Callable[[Path], dict[str, object] | None] = read_startup_state,
    alive: Callable[[int], bool] = pid_is_alive,
    clock: Callable[[], float] = time.monotonic,
    sleeper: Callable[[float], None] = time.sleep,
) -> dict[str, object]:
    started = clock()
    last: dict[str, object] | None = None
    while clock() - started < timeout_seconds:
        state = state_reader(path)
        if state:
            last = state
            pid = int(state.get("pid") or 0)
            if (
                state.get("safeMode") is True
                and state.get("phase") in ("ready", "healthy")
                and pid > 0
                and alive(pid)
            ):
                return state
        sleeper(POLL_INTERVAL_SECONDS)
    phase = last.get("phase") if last else "missing"
    raise TimeoutError(f"Safe Mode did not reach a live ready state (last phase: {phase})")


def terminate_launch_process(proc: subprocess.Popen[bytes]) -> tuple[bool, str]:
    if proc.poll() is not None:
        return True, "already-exited"
    if sys.platform == "win32":
        completed = subprocess.run(
            ["taskkill", "/PID", str(proc.pid), "/T", "/F"],
            check=False,
            capture_output=True,
            text=True,
            timeout=15,
        )
        for _ in range(40):
            if proc.poll() is not None:
                return True, "taskkill"
            time.sleep(0.1)
        return False, f"taskkill-{completed.returncode}"
    try:
        os.killpg(proc.pid, signal.SIGTERM)
    except ProcessLookupError:
        return True, "already-exited"
    for _ in range(40):
        if proc.poll() is not None:
            return True, "sigterm"
        time.sleep(0.1)
    try:
        os.killpg(proc.pid, signal.SIGKILL)
    except ProcessLookupError:
        return True, "sigterm"
    for _ in range(20):
        if proc.poll() is not None:
            return True, "sigkill"
        time.sleep(0.1)
    return False, "sigkill-failed"


def verify_derived_quarantine(home: Path, paths: list[object]) -> bool:
    if len(paths) != len(DERIVED_FIXTURES):
        return False
    returned = [Path(str(path)) for path in paths]
    for name, (filename, expected) in DERIVED_FIXTURES.items():
        original = home / filename
        matches = [
            path
            for path in returned
            if path.parent == home
            and path.name.startswith(filename + ".reames-rebuild-")
        ]
        if original.exists() or len(matches) != 1 or matches[0].read_bytes() != expected:
            return False
        if name not in ("tabs", "projects", "window", "zoom"):
            return False
    return True


def _fail(result: RecoverySmokeResult, kind: str, message: str) -> None:
    if result.failure_kind is None:
        result.failure_kind = kind
    result.outcome = "failed"
    result.errors.append(message)


def run_smoke(
    artifact_path: str,
    executable_path: str,
    guard_path: str,
    platform_name: str,
    launch_timeout_seconds: float = DEFAULT_LAUNCH_TIMEOUT_SECONDS,
    keep_temp: bool = False,
) -> RecoverySmokeResult:
    result = RecoverySmokeResult(platform=platform_name, kept_temp=keep_temp)
    safe_pid = 0
    launch_proc: subprocess.Popen[bytes] | None = None
    temp_root: Path | None = None
    try:
        if platform_name != host_platform():
            raise ValueError(f"requested {platform_name} smoke on {host_platform()} host")
        if not 1 <= launch_timeout_seconds <= 60:
            raise ValueError("launch timeout must be between 1 and 60 seconds")
        artifact = Path(artifact_path).resolve(strict=False)
        executable = Path(executable_path).resolve(strict=False)
        guard = Path(guard_path).resolve(strict=False)
        for path, label in ((artifact, "artifact"), (executable, "executable"), (guard, "guard")):
            if not path.is_file():
                raise FileNotFoundError(f"{label} not found: {path}")
        if platform_name != "windows" and (not os.access(executable, os.X_OK) or not os.access(guard, os.X_OK)):
            raise PermissionError("installed Desktop and Guard must be executable")

        result.artifact_name = artifact.name
        result.artifact_sha256 = sha256_file(artifact)
        result.executable_name = executable.name
        result.executable_sha256 = sha256_file(executable)
        result.guard_name = guard.name
        result.guard_sha256 = sha256_file(guard)

        parent = Path(tempfile.gettempdir()) / "reames-agent-recovery-smoke"
        parent.mkdir(parents=True, exist_ok=True)
        temp_root = Path(tempfile.mkdtemp(prefix="case-", dir=parent))
        home = temp_root / "home"
        workspace = temp_root / "workspace"
        home.mkdir()
        workspace.mkdir()
        env = smoke_environment(home)
        boundaries = default_boundary_roots(platform_name, executable.name)
        before_boundary = snapshot_roots(boundaries)

        config_path = home / "config.toml"
        credentials_path = home / ".env"
        config_path.write_bytes(INVALID_CONFIG)
        credentials_path.write_bytes(SYNTHETIC_ENV)
        config_hash = sha256_file(config_path)
        credentials_hash = sha256_file(credentials_path)

        initial = run_json_command(
            result,
            "initial-check",
            [str(guard), "check", "--json", "--root", str(workspace), "--app", str(executable)],
            env,
            workspace,
            expected=(1,),
        )
        if not isinstance(initial, dict):
            raise RuntimeError("initial Guard check did not return a report")
        summary = summarize_report(initial)
        result.checks["initial"] = summary
        if "error:config.invalid" not in summary["findingCodes"]:
            raise RuntimeError("initial Guard check did not classify the invalid config")

        launch_options: dict[str, object] = {}
        if sys.platform == "win32":
            launch_options["creationflags"] = subprocess.CREATE_NEW_PROCESS_GROUP
        else:
            launch_options["start_new_session"] = True
        launch_proc = subprocess.Popen(
            guard_launch_command(guard, executable),
            cwd=workspace,
            env=env,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            **launch_options,
        )
        result.commands.append(
            {
                "name": "safe-mode-launch",
                "exitCode": None,
                "expectedExitCodes": [],
                "lifecycle": "terminated-by-smoke-after-ready",
            }
        )
        state = wait_for_safe_mode_ready(
            home / "repair" / "startup-state.json", launch_timeout_seconds
        )
        safe_pid = int(state.get("pid") or 0)
        result.safe_mode_ready = True
        result.safe_mode_phase = str(state.get("phase") or "")
        result.safe_mode_pid_recorded = safe_pid > 0
        result.safe_mode_process_alive = pid_is_alive(safe_pid)
        result.config_unchanged_during_safe_mode = (
            config_path.is_file() and sha256_file(config_path) == config_hash
        )
        result.credentials_unchanged_during_safe_mode = (
            credentials_path.is_file()
            and sha256_file(credentials_path) == credentials_hash
        )
        if not result.config_unchanged_during_safe_mode or not result.credentials_unchanged_during_safe_mode:
            raise RuntimeError("Safe Mode modified damaged config or credential bytes")
        cleanup_ok, cleanup_method = terminate_launch_process(launch_proc)
        result.safe_mode_cleanup_ok = cleanup_ok
        result.checks["safeModeCleanup"] = cleanup_method
        result.commands[-1]["exitCode"] = launch_proc.poll()
        launch_proc = None
        safe_pid = 0
        if not cleanup_ok:
            raise RuntimeError("Safe Mode Desktop process could not be cleaned up")

        repaired = run_json_command(
            result,
            "repair-config",
            [str(guard), "repair", "--json", "--root", str(workspace)],
            env,
            workspace,
        )
        if not isinstance(repaired, dict) or config_path.exists() or not repaired.get("applied"):
            raise RuntimeError("Guard did not quarantine the invalid global config")
        result.config_repaired = True

        undone = run_json_command(
            result,
            "undo-config-repair",
            [str(guard), "undo", "--json"],
            env,
            workspace,
        )
        result.config_undo_exact = (
            isinstance(undone, dict)
            and undone.get("undone") is True
            and config_path.is_file()
            and sha256_file(config_path) == config_hash
        )
        if not result.config_undo_exact:
            raise RuntimeError("Guard undo did not restore the exact invalid config bytes")

        run_json_command(
            result,
            "final-config-quarantine",
            [str(guard), "repair", "--json", "--root", str(workspace)],
            env,
            workspace,
        )
        for _, (filename, body) in DERIVED_FIXTURES.items():
            (home / filename).write_bytes(body)
        quarantined = run_json_command(
            result,
            "rebuild-derived-state",
            [str(guard), "rebuild", "--target", "all", "--json"],
            env,
            workspace,
        )
        result.derived_state_quarantined = isinstance(quarantined, list) and verify_derived_quarantine(home, quarantined)
        if not result.derived_state_quarantined:
            raise RuntimeError("Guard did not quarantine every derived Desktop state exactly")

        final = run_json_command(
            result,
            "final-check",
            [str(guard), "check", "--json", "--root", str(workspace), "--app", str(executable)],
            env,
            workspace,
        )
        if not isinstance(final, dict):
            raise RuntimeError("final Guard check did not return a report")
        final_summary = summarize_report(final)
        result.checks["final"] = final_summary
        result.final_guard_clean = not any(
            str(code).startswith("error:") for code in final_summary["findingCodes"]
        )
        if not result.final_guard_clean:
            raise RuntimeError("final Guard report still contains an error finding")
        if sha256_file(credentials_path) != credentials_hash:
            raise RuntimeError("recovery operations modified credential bytes")

        result.home_files = list_home_files(home)
        result.boundary_changes = changed_snapshot(
            before_boundary, snapshot_roots(boundaries)
        )
        if result.boundary_changes:
            raise RuntimeError("isolated recovery smoke changed default user state")
        result.outcome = "passed"
    except (ValueError, FileNotFoundError, PermissionError) as exc:
        _fail(result, "invalid-input", str(exc))
    except TimeoutError as exc:
        _fail(result, "safe-mode-timeout", str(exc))
    except Exception as exc:  # pragma: no cover - native integration failure path
        _fail(result, "recovery-contract", str(exc))
    finally:
        if launch_proc is not None:
            ok, method = terminate_launch_process(launch_proc)
            result.safe_mode_cleanup_ok = result.safe_mode_cleanup_ok or ok
            result.checks.setdefault("safeModeCleanup", method)
        if temp_root is not None and not keep_temp:
            try:
                remove_tree_with_retries(temp_root)
                result.temp_cleaned = True
            except OSError as exc:
                result.errors.append(f"temporary recovery fixture cleanup failed: {exc}")
                if result.outcome == "passed":
                    result.outcome = "failed"
                    result.failure_kind = "temp-cleanup"
        result.finished_at = utc_now()
    return result


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--platform", choices=("linux", "darwin", "windows"), required=True)
    parser.add_argument("--artifact", required=True)
    parser.add_argument("--exe", required=True)
    parser.add_argument("--guard", required=True)
    parser.add_argument("--launch-timeout-seconds", type=float, default=DEFAULT_LAUNCH_TIMEOUT_SECONDS)
    parser.add_argument("--out", required=True)
    parser.add_argument("--keep-temp", action="store_true")
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    result = run_smoke(
        args.artifact,
        args.exe,
        args.guard,
        args.platform,
        args.launch_timeout_seconds,
        args.keep_temp,
    )
    output = Path(args.out)
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(result.to_dict(), indent=2) + "\n", encoding="utf-8")
    print(f"Desktop recovery smoke: {result.outcome}")
    if result.failure_kind:
        print(f"Failure kind: {result.failure_kind}")
    for error in result.errors:
        print(f"- {error}")
    print(f"Evidence: {output}")
    return 0 if result.outcome == "passed" else 1


if __name__ == "__main__":
    raise SystemExit(main())
