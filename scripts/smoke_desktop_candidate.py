#!/usr/bin/env python3
"""Native Linux/macOS Desktop candidate install/start smoke.

The caller installs or copies the real candidate artifact, then passes the
installed executable here. The smoke hashes both inputs, runs with an isolated
Reames Agent home, checks that state stays confined, and emits JSON evidence.
Linux additionally requires a visible X11 window discovered through xdotool.
"""

from __future__ import annotations

import argparse
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


SCHEMA_VERSION = 2
MIN_OBSERVATION_SECONDS = 10
MAX_OBSERVATION_SECONDS = 300
MIN_STARTUP_BUDGET_SECONDS = 1.0
MAX_STARTUP_BUDGET_SECONDS = 60.0
DEFAULT_STARTUP_BUDGET_SECONDS = 10.0
POLL_INTERVAL_SECONDS = 0.5
REQUIRED_WINDOW_CHECKS = 3
REQUIRED_CONSECUTIVE_READY_CHECKS = 3
SMOKE_CONFIG = """\
[desktop]
close_behavior = "quit"
check_updates = false
onboarding_dismissed = true
language = "en"
"""


def utc_now() -> str:
    return datetime.now(timezone.utc).isoformat()


def host_platform() -> str:
    if sys.platform.startswith("linux"):
        return "linux"
    if sys.platform == "darwin":
        return "darwin"
    return sys.platform


@dataclass
class Observation:
    elapsed_seconds: float = 0.0
    checks: int = 0
    window_checks: int = 0
    max_visible_windows: int = 0
    early_exit_code: int | None = None
    state_checks: int = 0
    ready_checks: int = 0
    max_consecutive_ready_checks: int = 0
    final_ready: bool = False
    first_state_ready_seconds: float | None = None
    first_visible_seconds: float | None = None
    stable_ready_seconds: float | None = None

    @property
    def ready(self) -> bool:
        return (
            self.early_exit_code is None
            and self.final_ready
            and self.max_consecutive_ready_checks
            >= REQUIRED_CONSECUTIVE_READY_CHECKS
        )


@dataclass
class CandidateSmokeResult:
    schema_version: int = SCHEMA_VERSION
    platform: str = field(default_factory=host_platform)
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
    startup_budget_seconds: float = DEFAULT_STARTUP_BUDGET_SECONDS
    first_state_ready_seconds: float | None = None
    first_visible_seconds: float | None = None
    stable_ready_seconds: float | None = None
    startup_budget_met: bool = False
    ready: bool = False
    readiness_kind: str = ""
    process_alive_after_observation: bool = False
    observed_exit_code: int | None = None
    window_required: bool = False
    window_observed: bool = False
    window_checks: int = 0
    max_visible_windows: int = 0
    cleanup_method: str = ""
    cleanup_exit_code: int | None = None
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


def validate_startup_budget_seconds(value: float) -> float:
    if not MIN_STARTUP_BUDGET_SECONDS <= value <= MAX_STARTUP_BUDGET_SECONDS:
        raise ValueError(
            f"startup budget seconds must be between {MIN_STARTUP_BUDGET_SECONDS:g} "
            f"and {MAX_STARTUP_BUDGET_SECONDS:g}"
        )
    return value


def prepare_smoke_home(home: Path) -> None:
    home.mkdir(parents=True, exist_ok=True)
    (home / "config.toml").write_text(SMOKE_CONFIG, encoding="utf-8")


def desktop_state_ready(home: Path) -> bool:
    """Return true once Desktop has written a state file inside its isolated home."""
    return any(
        path.is_file() and path.name.startswith("desktop-")
        for path in home.glob("desktop-*")
    )


def list_home_files(home: Path) -> list[str]:
    files: list[str] = []
    if not home.exists():
        return files
    for path in sorted(home.rglob("*")):
        if not path.is_file():
            continue
        rel = path.relative_to(home).as_posix()
        if path.name == ".env" or "credential" in path.name.lower():
            files.append(f"{rel} [REDACTED]")
            continue
        try:
            files.append(f"{rel} ({path.stat().st_size} bytes)")
        except OSError:
            files.append(rel)
    return files


def default_boundary_roots(platform_name: str) -> dict[str, Path]:
    home = Path.home()
    roots = {"DEFAULT_HOME": home / ".reames-agent"}
    if platform_name == "darwin":
        roots["DEFAULT_CACHE"] = home / "Library" / "Caches" / "reames-agent"
        roots["LEGACY_SUPPORT"] = (
            home / "Library" / "Application Support" / "reames-agent"
        )
    else:
        cache_base = Path(os.environ.get("XDG_CACHE_HOME", home / ".cache"))
        config_base = Path(os.environ.get("XDG_CONFIG_HOME", home / ".config"))
        roots["DEFAULT_CACHE"] = cache_base / "reames-agent"
        roots["LEGACY_SUPPORT"] = config_base / "reames-agent"
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
    parent = temp_parent or Path(tempfile.gettempdir()) / "reames-agent-candidate-smoke"
    parent.mkdir(parents=True, exist_ok=True)
    if keep_temp:
        yield Path(tempfile.mkdtemp(prefix="home-", dir=parent))
        return
    with tempfile.TemporaryDirectory(prefix="home-", dir=parent) as raw:
        yield Path(raw)


def linux_window_ids(pid: int) -> list[str]:
    completed = subprocess.run(
        [
            "xdotool",
            "search",
            "--onlyvisible",
            "--pid",
            str(pid),
            "--name",
            "Reames Agent",
        ],
        check=False,
        capture_output=True,
        text=True,
        timeout=3,
    )
    if completed.returncode not in (0, 1):
        detail = completed.stderr.strip() or completed.stdout.strip()
        raise RuntimeError(f"xdotool search failed ({completed.returncode}): {detail}")
    return [line.strip() for line in completed.stdout.splitlines() if line.strip()]


def observe_process(
    proc: subprocess.Popen,
    seconds: int,
    window_probe: Callable[[int], list[str]] | None = None,
    state_probe: Callable[[], bool] | None = None,
    clock: Callable[[], float] = time.monotonic,
    sleeper: Callable[[float], None] = time.sleep,
) -> Observation:
    observation = Observation()
    start = clock()
    consecutive_ready = 0
    while clock() - start < seconds:
        exit_code = proc.poll()
        if exit_code is not None:
            observation.early_exit_code = exit_code
            break
        observation.checks += 1
        elapsed = max(0.0, clock() - start)
        state_ready = state_probe() if state_probe is not None else True
        if state_ready:
            observation.state_checks += 1
            if observation.first_state_ready_seconds is None:
                observation.first_state_ready_seconds = elapsed
        visible_ready = True
        if window_probe is not None:
            windows = window_probe(proc.pid)
            visible_ready = bool(windows)
            if windows:
                observation.window_checks += 1
                if observation.first_visible_seconds is None:
                    observation.first_visible_seconds = elapsed
            observation.max_visible_windows = max(
                observation.max_visible_windows, len(windows)
            )
        ready_now = state_ready and visible_ready
        observation.final_ready = ready_now
        if ready_now:
            observation.ready_checks += 1
            consecutive_ready += 1
            observation.max_consecutive_ready_checks = max(
                observation.max_consecutive_ready_checks, consecutive_ready
            )
            if (
                consecutive_ready >= REQUIRED_CONSECUTIVE_READY_CHECKS
                and observation.stable_ready_seconds is None
            ):
                observation.stable_ready_seconds = elapsed
        else:
            consecutive_ready = 0
        remaining = seconds - (clock() - start)
        if remaining > 0:
            sleeper(min(POLL_INTERVAL_SECONDS, remaining))
    observation.elapsed_seconds = max(0.0, clock() - start)
    return observation


def classify_startup_observation(
    observation: Observation, budget_seconds: float, platform_name: str
) -> tuple[str, str] | None:
    if observation.early_exit_code is not None:
        return "early-exit", f"process exited with code {observation.early_exit_code}"
    if not observation.ready:
        requirement = (
            "isolated Desktop state and a visible window"
            if platform_name == "linux"
            else "isolated Desktop state"
        )
        return (
            "startup-not-ready",
            f"{platform_name} candidate did not sustain {requirement} through the final check",
        )
    if (
        observation.stable_ready_seconds is None
        or observation.stable_ready_seconds > budget_seconds
    ):
        elapsed = observation.stable_ready_seconds
        elapsed_text = "no stable readiness" if elapsed is None else f"{elapsed:.3f}s"
        return (
            "startup-budget",
            f"{platform_name} candidate needed {elapsed_text}; budget is {budget_seconds:.3f}s",
        )
    return None


def cleanup_process(
    proc: subprocess.Popen, platform_name: str, errors: list[str]
) -> tuple[bool, str]:
    if proc.poll() is not None:
        return True, "already-exited"
    if platform_name == "linux":
        try:
            for window_id in linux_window_ids(proc.pid):
                subprocess.run(
                    ["xdotool", "windowclose", window_id],
                    check=False,
                    capture_output=True,
                    timeout=3,
                )
            proc.wait(timeout=8)
            return True, "window-close"
        except subprocess.TimeoutExpired:
            pass
        except Exception as exc:  # pragma: no cover - native defensive path
            errors.append(f"window close failed: {exc}")
    try:
        proc.terminate()
        proc.wait(timeout=8)
        return True, "terminate"
    except subprocess.TimeoutExpired:
        pass
    except Exception as exc:  # pragma: no cover - native defensive path
        errors.append(f"terminate failed: {exc}")
    try:
        proc.kill()
        proc.wait(timeout=5)
        return True, "kill"
    except Exception as exc:  # pragma: no cover - native defensive path
        errors.append(f"kill failed: {exc}")
        return proc.poll() is not None, "kill-failed"


def _fail(result: CandidateSmokeResult, kind: str, message: str) -> None:
    if result.failure_kind is None:
        result.failure_kind = kind
    result.outcome = "failed"
    result.errors.append(message)


def run_smoke(
    artifact_path: str,
    executable_path: str,
    platform_name: str,
    observation_seconds: int = 12,
    max_startup_seconds: float = DEFAULT_STARTUP_BUDGET_SECONDS,
    keep_temp: bool = False,
) -> CandidateSmokeResult:
    result = CandidateSmokeResult(
        platform=platform_name,
        observation_seconds=observation_seconds,
        startup_budget_seconds=max_startup_seconds,
        kept_temp=keep_temp,
        window_required=platform_name == "linux",
        readiness_kind="state+visible-window" if platform_name == "linux" else "state",
    )
    proc: subprocess.Popen | None = None
    home: Path | None = None

    try:
        validate_observation_seconds(observation_seconds)
        validate_startup_budget_seconds(max_startup_seconds)
    except ValueError as exc:
        _fail(result, "invalid-arguments", str(exc))
        result.finished_at = utc_now()
        return result
    if platform_name not in ("linux", "darwin"):
        _fail(result, "unsupported-platform", f"unsupported platform: {platform_name}")
        result.finished_at = utc_now()
        return result
    if platform_name != host_platform():
        _fail(
            result,
            "platform-mismatch",
            f"requested {platform_name} smoke on {host_platform()} host",
        )
        result.finished_at = utc_now()
        return result

    artifact = Path(artifact_path).resolve(strict=False)
    executable = Path(executable_path).resolve(strict=False)
    result.artifact_name = artifact.name
    result.executable_name = executable.name
    for path, label in ((artifact, "artifact"), (executable, "executable")):
        if not path.is_file():
            _fail(result, "missing-input", f"{label} not found: {path}")
            result.finished_at = utc_now()
            return result
    if not os.access(executable, os.X_OK):
        _fail(result, "missing-input", f"executable is not runnable: {executable}")
        result.finished_at = utc_now()
        return result

    try:
        result.artifact_size = artifact.stat().st_size
        result.artifact_sha256 = sha256_file(artifact)
        result.executable_size = executable.stat().st_size
        result.executable_sha256 = sha256_file(executable)
    except OSError as exc:
        _fail(result, "inspect-failure", str(exc))
        result.finished_at = utc_now()
        return result

    with managed_smoke_home(keep_temp) as managed_home:
        home = managed_home
        result.home_dir = str(home)
        prepare_smoke_home(home)
        boundary_roots = default_boundary_roots(platform_name)
        boundary_before = snapshot_roots(boundary_roots)
        env = os.environ.copy()
        env["REAMES_AGENT_HOME"] = str(home)
        env.pop("REAMES_AGENT_STATE_HOME", None)
        env.pop("REAMES_AGENT_CACHE_HOME", None)
        env.pop("REAMES_AGENT_DEV", None)

        try:
            proc = subprocess.Popen(
                [str(executable), "--home", str(home)],
                env=env,
                stdin=subprocess.DEVNULL,
                stdout=subprocess.DEVNULL,
                stderr=subprocess.DEVNULL,
                start_new_session=True,
            )
        except OSError as exc:
            _fail(result, "startup-failure", f"launch failed: {exc}")
        else:
            try:
                probe = linux_window_ids if platform_name == "linux" else None
                observation = observe_process(
                    proc,
                    observation_seconds,
                    window_probe=probe,
                    state_probe=lambda: desktop_state_ready(home),
                )
                result.observed_seconds = round(observation.elapsed_seconds, 3)
                result.observed_exit_code = observation.early_exit_code
                result.first_state_ready_seconds = (
                    round(observation.first_state_ready_seconds, 3)
                    if observation.first_state_ready_seconds is not None
                    else None
                )
                result.first_visible_seconds = (
                    round(observation.first_visible_seconds, 3)
                    if observation.first_visible_seconds is not None
                    else None
                )
                result.stable_ready_seconds = (
                    round(observation.stable_ready_seconds, 3)
                    if observation.stable_ready_seconds is not None
                    else None
                )
                result.startup_budget_met = (
                    observation.stable_ready_seconds is not None
                    and observation.stable_ready_seconds <= max_startup_seconds
                )
                result.ready = observation.ready
                result.window_checks = observation.window_checks
                result.max_visible_windows = observation.max_visible_windows
                result.window_observed = (
                    observation.window_checks >= REQUIRED_WINDOW_CHECKS
                )
                result.process_alive_after_observation = proc.poll() is None
                failure = classify_startup_observation(
                    observation, max_startup_seconds, platform_name
                )
                if failure is not None:
                    _fail(result, *failure)
            except Exception as exc:
                _fail(result, "observation-failure", str(exc))
            finally:
                result.cleanup_ok, result.cleanup_method = cleanup_process(
                    proc, platform_name, result.errors
                )
                result.cleanup_exit_code = proc.poll()
                if not result.cleanup_ok:
                    _fail(result, "cleanup-failure", "candidate process did not stop")

        result.home_files = list_home_files(home)
        if not any(
            Path(entry.split(" (", 1)[0]).name.startswith("desktop-")
            for entry in result.home_files
        ):
            _fail(result, "state-missing", "no Desktop state file in isolated home")
        result.boundary_changes = changed_snapshot(
            boundary_before, snapshot_roots(boundary_roots)
        )
        if result.boundary_changes:
            _fail(result, "state-leak", "default Reames Agent home changed")
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
    parser.add_argument("--platform", required=True, choices=("linux", "darwin"))
    parser.add_argument("--artifact", required=True)
    parser.add_argument("--exe", required=True)
    parser.add_argument("--out")
    parser.add_argument("--observation-seconds", type=int, default=12)
    parser.add_argument(
        "--max-startup-seconds",
        type=float,
        default=DEFAULT_STARTUP_BUDGET_SECONDS,
    )
    parser.add_argument("--keep-temp", action="store_true")
    args = parser.parse_args(argv)
    try:
        validate_observation_seconds(args.observation_seconds)
        validate_startup_budget_seconds(args.max_startup_seconds)
    except ValueError as exc:
        parser.error(str(exc))

    result = run_smoke(
        args.artifact,
        args.exe,
        args.platform,
        args.observation_seconds,
        args.max_startup_seconds,
        args.keep_temp,
    )
    evidence = result.to_dict()
    if args.out:
        output = Path(args.out)
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(json.dumps(evidence, indent=2) + "\n", encoding="utf-8")
        print(f"Evidence written to {output}")
    print(json.dumps(evidence, indent=2))
    return 0 if result.outcome == "passed" else 1


if __name__ == "__main__":
    raise SystemExit(main())
