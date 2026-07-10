#!/usr/bin/env python3
"""Windows native Desktop interaction smoke using screenshot-free UIA.

The smoke creates an isolated Reames Agent home and workspace, starts a real
Wails executable, selects the project by invoking its New session control,
sends a marker, starts and stops a long shell command, restarts the app, and
verifies session/workspace recovery. No API key, screenshot, or default user
state is read.
"""

from __future__ import annotations

import argparse
import http.server
import json
import os
import subprocess
import sys
import tempfile
import threading
import time
import uuid
from contextlib import contextmanager
from dataclasses import asdict, dataclass, field
from datetime import datetime, timezone
from pathlib import Path
from typing import Callable, Iterator

try:
    from scripts import smoke_desktop_native as native
except ModuleNotFoundError:  # direct ``python scripts/...`` execution
    import smoke_desktop_native as native  # type: ignore[no-redef]


SCHEMA_VERSION = 1
MIN_TIMEOUT_SECONDS = 10
MAX_TIMEOUT_SECONDS = 180
WORKSPACE_TITLE = "Native UI Workspace"
COMPOSER_AUTOMATION_ID = "composer-input"
ONBOARDING_AUTOMATION_ID = "onboarding-key"
LOOPBACK_PROVIDER_NAME = "native-smoke"
LOOPBACK_MODEL = "native-smoke-model"
LOOPBACK_RESPONSE = "Native Desktop interaction smoke response"


def utc_now() -> str:
    return datetime.now(timezone.utc).isoformat()


@dataclass
class InteractionSmokeResult:
    schema_version: int = SCHEMA_VERSION
    platform: str = sys.platform
    started_at: str = field(default_factory=utc_now)
    finished_at: str = ""
    outcome: str = "unknown"
    failure_kind: str | None = None
    executable_name: str = ""
    executable_sha256: str = ""
    executable_size: int = 0
    artifact_name: str = ""
    artifact_sha256: str = ""
    artifact_size: int = 0
    home_dir: str = ""
    workspace_dir: str = ""
    marker: str = ""
    onboarding_absent: bool = False
    project_visible: bool = False
    new_session_invoked: bool = False
    workspace_selected: bool = False
    message_sent: bool = False
    message_persisted: bool = False
    provider_bound_to_loopback: bool = False
    provider_requests: int = 0
    provider_received_marker: bool = False
    assistant_response_persisted: bool = False
    stop_visible: bool = False
    stop_invoked: bool = False
    stop_completed: bool = False
    recovery_verified: bool = False
    initial_session_path: str = ""
    recovered_session_path: str = ""
    uia_actions: list[dict[str, object]] = field(default_factory=list)
    cleanup_methods: list[str] = field(default_factory=list)
    cleanup_exit_codes: list[int | None] = field(default_factory=list)
    cleanup_ok: bool = False
    temp_cleaned: bool = False
    kept_temp: bool = False
    home_files: list[str] = field(default_factory=list)
    boundary_changes: list[str] = field(default_factory=list)
    errors: list[str] = field(default_factory=list)

    def to_dict(self) -> dict[str, object]:
        return asdict(self)


def validate_timeout(value: int) -> int:
    if not MIN_TIMEOUT_SECONDS <= value <= MAX_TIMEOUT_SECONDS:
        raise ValueError(
            f"timeout seconds must be between {MIN_TIMEOUT_SECONDS} and {MAX_TIMEOUT_SECONDS}"
        )
    return value


@contextmanager
def managed_fixture_root(
    keep_temp: bool, temp_parent: Path | None = None
) -> Iterator[Path]:
    parent = temp_parent or Path(tempfile.gettempdir()) / "reames-agent-interaction-smoke"
    parent.mkdir(parents=True, exist_ok=True)
    if keep_temp:
        yield Path(tempfile.mkdtemp(prefix="fixture-", dir=parent))
        return
    with tempfile.TemporaryDirectory(prefix="fixture-", dir=parent) as raw:
        yield Path(raw)


def interaction_smoke_config(base_url: str) -> str:
    return f'''\
default_model = "{LOOPBACK_PROVIDER_NAME}/{LOOPBACK_MODEL}"

[[providers]]
name = "{LOOPBACK_PROVIDER_NAME}"
kind = "openai"
base_url = "{base_url}"
models = ["{LOOPBACK_MODEL}"]
default = "{LOOPBACK_MODEL}"
no_proxy = true

[desktop]
close_behavior = "quit"
check_updates = false
onboarding_dismissed = true
language = "en"
provider_access = ["{LOOPBACK_PROVIDER_NAME}"]
'''


@contextmanager
def local_openai_server() -> Iterator[tuple[str, list[dict[str, object]]]]:
    """Serve one deterministic, keyless OpenAI-compatible loopback endpoint."""

    requests: list[dict[str, object]] = []

    class Handler(http.server.BaseHTTPRequestHandler):
        protocol_version = "HTTP/1.1"

        def do_POST(self) -> None:  # noqa: N802 - stdlib handler API
            if self.path != "/v1/chat/completions":
                self.send_error(404)
                return
            try:
                length = int(self.headers.get("Content-Length", "0"))
                if length <= 0 or length > 2 * 1024 * 1024:
                    raise ValueError("invalid request length")
                payload = json.loads(self.rfile.read(length).decode("utf-8"))
                if not isinstance(payload, dict):
                    raise ValueError("request body is not an object")
            except (OSError, UnicodeDecodeError, ValueError, json.JSONDecodeError) as exc:
                body = json.dumps({"error": {"message": str(exc)}}).encode("utf-8")
                self.send_response(400)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(body)))
                self.send_header("Connection", "close")
                self.end_headers()
                self.wfile.write(body)
                return

            requests.append(payload)
            chunks = [
                {
                    "choices": [
                        {
                            "index": 0,
                            "delta": {"role": "assistant", "content": LOOPBACK_RESPONSE},
                            "finish_reason": None,
                        }
                    ]
                },
                {
                    "choices": [
                        {"index": 0, "delta": {}, "finish_reason": "stop"}
                    ],
                    "usage": {
                        "prompt_tokens": 8,
                        "completion_tokens": 5,
                        "total_tokens": 13,
                    },
                },
            ]
            stream = "".join(
                f"data: {json.dumps(chunk, separators=(',', ':'))}\n\n"
                for chunk in chunks
            ) + "data: [DONE]\n\n"
            body = stream.encode("utf-8")
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.send_header("Cache-Control", "no-cache")
            self.send_header("Content-Length", str(len(body)))
            self.send_header("Connection", "close")
            self.end_headers()
            self.wfile.write(body)

        def log_message(self, _format: str, *_args: object) -> None:
            return

    server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    server.daemon_threads = True
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        host, port = server.server_address[:2]
        yield f"http://{host}:{port}/v1", requests
    finally:
        server.shutdown()
        server.server_close()
        thread.join(timeout=5)


def prepare_fixture(root: Path, provider_base_url: str) -> tuple[Path, Path]:
    home = root / "home"
    workspace = root / "workspace"
    home.mkdir(parents=True, exist_ok=True)
    (home / "config.toml").write_text(
        interaction_smoke_config(provider_base_url), encoding="utf-8"
    )
    workspace.mkdir(parents=True, exist_ok=True)
    (workspace / "README.md").write_text(
        "# Reames Agent native UIA smoke workspace\n", encoding="utf-8"
    )
    projects = {
        "globalTopics": [],
        "projects": [
            {
                "root": str(workspace.resolve()),
                "title": WORKSPACE_TITLE,
                "topics": [],
            }
        ],
    }
    (home / "desktop-projects.json").write_text(
        json.dumps(projects, indent=2) + "\n", encoding="utf-8"
    )
    return home, workspace


def active_tab(home: Path) -> dict[str, object] | None:
    path = home / "desktop-tabs.json"
    try:
        payload = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, ValueError, TypeError):
        return None
    tabs = payload.get("tabs")
    active_id = payload.get("activeTab")
    if not isinstance(tabs, list):
        return None
    for tab in tabs:
        if isinstance(tab, dict) and tab.get("id") == active_id:
            return tab
    return tabs[0] if tabs and isinstance(tabs[0], dict) else None


def same_path(left: object, right: Path) -> bool:
    if not isinstance(left, str) or not left.strip():
        return False
    return os.path.normcase(str(Path(left).resolve(strict=False))) == os.path.normcase(
        str(right.resolve(strict=False))
    )


def durable_session_messages(session_path: object) -> list[dict[str, object]]:
    if not isinstance(session_path, str) or not session_path:
        return []
    transcript = Path(session_path)
    event_log = transcript.with_name(transcript.stem + ".events.jsonl")
    messages: list[dict[str, object]] = []
    try:
        event_lines = (
            event_log.read_text(encoding="utf-8").splitlines()
            if event_log.is_file() and event_log.stat().st_size > 0
            else []
        )
        first_event = json.loads(event_lines[0]) if event_lines else None
        native_events = (
            isinstance(first_event, dict)
            and first_event.get("schema_version") == 1
            and first_event.get("type") in {"replace", "append"}
        )
        source_lines = (
            event_lines
            if native_events
            else transcript.read_text(encoding="utf-8").splitlines()
        )
        for line in source_lines:
            if not line.strip():
                continue
            record = json.loads(line)
            if not isinstance(record, dict):
                continue
            if native_events:
                if record.get("schema_version") != 1:
                    break
                batch = record.get("messages")
                if not isinstance(batch, list):
                    break
                clean = [item for item in batch if isinstance(item, dict)]
                if record.get("type") == "replace":
                    messages = clean
                elif record.get("type") == "append":
                    index = record.get("message_index", 0)
                    if not isinstance(index, int) or index != len(messages):
                        break
                    messages.extend(clean)
                else:
                    break
            else:
                messages.append(record)
    except (OSError, ValueError, TypeError, json.JSONDecodeError):
        return []
    return messages


def durable_session_has_message(session_path: object, role: str, content: str) -> bool:
    return any(
        message.get("role") == role and message.get("content") == content
        for message in durable_session_messages(session_path)
    )


def wait_until(
    predicate: Callable[[], bool], timeout_seconds: float, description: str
) -> None:
    deadline = time.monotonic() + timeout_seconds
    while time.monotonic() < deadline:
        if predicate():
            return
        time.sleep(0.1)
    raise TimeoutError(description)


def launch_desktop(exe: Path, home: Path) -> subprocess.Popen:
    env = os.environ.copy()
    env["REAMES_AGENT_HOME"] = str(home)
    env.pop("REAMES_AGENT_STATE_HOME", None)
    env.pop("REAMES_AGENT_CACHE_HOME", None)
    env.pop("REAMES_AGENT_DEV", None)
    return subprocess.Popen(
        [str(exe), "--home", str(home)],
        env=env,
        stdin=subprocess.DEVNULL,
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
        creationflags=subprocess.CREATE_NO_WINDOW,
    )


def _fail(result: InteractionSmokeResult, kind: str, message: str) -> None:
    if result.failure_kind is None:
        result.failure_kind = kind
    result.outcome = "failed"
    result.errors.append(message)


def run_smoke(
    exe_path: str,
    timeout_seconds: int = 30,
    keep_temp: bool = False,
    artifact_path: str | None = None,
) -> InteractionSmokeResult:
    result = InteractionSmokeResult(kept_temp=keep_temp)
    try:
        validate_timeout(timeout_seconds)
    except ValueError as exc:
        _fail(result, "invalid-arguments", str(exc))
        result.finished_at = utc_now()
        return result

    if sys.platform != "win32":
        _fail(result, "unsupported-platform", "Desktop interaction smoke requires Windows")
        result.finished_at = utc_now()
        return result

    try:
        from scripts.windows_uia import (
            NEW_SESSION_NAMES,
            SEND_NAMES,
            STOP_NAMES,
            WindowsUIAutomation,
            wait_for_window,
        )
    except ModuleNotFoundError:  # direct ``python scripts/...`` execution
        from windows_uia import (  # type: ignore[no-redef]
            NEW_SESSION_NAMES,
            SEND_NAMES,
            STOP_NAMES,
            WindowsUIAutomation,
            wait_for_window,
        )

    exe = Path(exe_path).resolve(strict=False)
    result.executable_name = exe.name
    if not exe.is_file():
        _fail(result, "startup-failure", f"executable not found: {exe}")
        result.finished_at = utc_now()
        return result
    try:
        result.executable_size = exe.stat().st_size
        result.executable_sha256 = native.sha256_file(exe)
        if artifact_path:
            artifact = Path(artifact_path).resolve(strict=False)
            result.artifact_name = artifact.name
            if not artifact.is_file():
                raise FileNotFoundError(f"artifact not found: {artifact}")
            result.artifact_size = artifact.stat().st_size
            result.artifact_sha256 = native.sha256_file(artifact)
    except OSError as exc:
        _fail(result, "startup-failure", f"inspect inputs: {exc}")
        result.finished_at = utc_now()
        return result

    fixture_root: Path | None = None
    proc: subprocess.Popen | None = None
    uia: WindowsUIAutomation | None = None
    all_cleanups_ok = True
    with (
        local_openai_server() as (provider_base_url, provider_requests),
        managed_fixture_root(keep_temp) as root,
    ):
        fixture_root = root
        home, workspace = prepare_fixture(root, provider_base_url)
        result.provider_bound_to_loopback = provider_base_url.startswith(
            "http://127.0.0.1:"
        )
        result.home_dir = str(home)
        result.workspace_dir = str(workspace)
        result.marker = f"Reames native UIA marker {uuid.uuid4().hex[:12]}"
        boundary_roots = native.default_boundary_roots(home, exe.name)
        boundary_before = native.snapshot_roots(boundary_roots)

        try:
            proc = launch_desktop(exe, home)
            hwnd = wait_for_window(proc.pid, timeout_seconds)
            uia = WindowsUIAutomation(hwnd)
            wait_until(
                lambda: uia.has(automation_id=COMPOSER_AUTOMATION_ID),
                timeout_seconds,
                "composer did not become accessible",
            )
            result.onboarding_absent = not uia.has(
                automation_id=ONBOARDING_AUTOMATION_ID
            )
            if not result.onboarding_absent:
                raise RuntimeError("first-run onboarding still blocks the isolated smoke")
            wait_until(
                lambda: uia.has(name=WORKSPACE_TITLE),
                timeout_seconds,
                "pre-seeded smoke workspace is not visible",
            )
            result.project_visible = True

            uia.invoke(name=NEW_SESSION_NAMES, occurrence=-1)
            result.new_session_invoked = True
            wait_until(
                lambda: (
                    (tab := active_tab(home)) is not None
                    and tab.get("scope") == "project"
                    and same_path(tab.get("workspaceRoot"), workspace)
                ),
                timeout_seconds,
                "project New session did not select the smoke workspace",
            )
            result.workspace_selected = True
            tab = active_tab(home) or {}
            result.initial_session_path = str(tab.get("sessionPath") or "")

            uia.type_text(result.marker, automation_id=COMPOSER_AUTOMATION_ID)
            uia.wait_enabled(name=SEND_NAMES, timeout_seconds=timeout_seconds)
            uia.press_enter(automation_id=COMPOSER_AUTOMATION_ID)
            result.message_sent = True
            wait_until(
                lambda: durable_session_has_message(
                    (active_tab(home) or {}).get("sessionPath"),
                    "user",
                    result.marker,
                ),
                timeout_seconds,
                "sent marker was not persisted to the active session",
            )
            result.message_persisted = True
            wait_until(
                lambda: durable_session_has_message(
                    (active_tab(home) or {}).get("sessionPath"),
                    "assistant",
                    LOOPBACK_RESPONSE,
                ),
                timeout_seconds,
                "loopback assistant response was not persisted to the active session",
            )
            result.assistant_response_persisted = True
            wait_until(
                lambda: result.marker
                in json.dumps(provider_requests, ensure_ascii=False),
                timeout_seconds,
                "loopback provider did not receive the submitted marker",
            )
            result.provider_received_marker = True

            uia.type_text(
                "!Start-Sleep -Seconds 30", automation_id=COMPOSER_AUTOMATION_ID
            )
            uia.wait_enabled(name=SEND_NAMES, timeout_seconds=timeout_seconds)
            uia.press_enter(
                automation_id=COMPOSER_AUTOMATION_ID,
                timeout_seconds=timeout_seconds,
            )
            result.stop_visible = True
            uia.invoke(name=STOP_NAMES, timeout_seconds=timeout_seconds)
            result.stop_invoked = True
            uia.wait_absent(name=STOP_NAMES, timeout_seconds=timeout_seconds)
            result.stop_completed = True
            result.uia_actions.extend(uia.actions)
            uia.actions.clear()
            uia.close()
            uia = None

            ok, method = native.cleanup_process(proc, result.errors)
            result.cleanup_methods.append(method)
            result.cleanup_exit_codes.append(proc.poll())
            all_cleanups_ok = all_cleanups_ok and ok
            proc = None
            if not ok:
                raise RuntimeError("initial Desktop process did not close")

            proc = launch_desktop(exe, home)
            hwnd = wait_for_window(proc.pid, timeout_seconds)
            uia = WindowsUIAutomation(hwnd)
            wait_until(
                lambda: uia.has(name=result.marker) and uia.has(name=LOOPBACK_RESPONSE),
                timeout_seconds,
                "restarted Desktop did not restore the user and assistant messages",
            )
            recovered = active_tab(home) or {}
            result.recovered_session_path = str(recovered.get("sessionPath") or "")
            result.recovery_verified = (
                recovered.get("scope") == "project"
                and same_path(recovered.get("workspaceRoot"), workspace)
                and result.recovered_session_path == result.initial_session_path
                and not uia.has(automation_id=ONBOARDING_AUTOMATION_ID)
            )
            if not result.recovery_verified:
                raise RuntimeError("session/workspace recovery state does not match")
            result.uia_actions.extend(uia.actions)
            uia.actions.clear()
        except Exception as exc:
            _fail(result, "interaction-failure", str(exc))
        finally:
            if uia is not None:
                result.uia_actions.extend(uia.actions)
                uia.actions.clear()
                uia.close()
            if proc is not None:
                ok, method = native.cleanup_process(proc, result.errors)
                result.cleanup_methods.append(method)
                result.cleanup_exit_codes.append(proc.poll())
                all_cleanups_ok = all_cleanups_ok and ok

        result.provider_requests = len(provider_requests)
        result.cleanup_ok = all_cleanups_ok
        if not result.cleanup_ok:
            _fail(result, "cleanup-failure", "one or more Desktop processes did not stop")
        result.home_files = native.list_home_files(home)
        result.boundary_changes = native.changed_snapshot(
            boundary_before, native.snapshot_roots(boundary_roots)
        )
        if result.boundary_changes:
            _fail(result, "state-leak", "default user state changed during isolated interaction")
        if result.outcome == "unknown":
            result.outcome = "passed"

    if fixture_root is not None:
        result.temp_cleaned = not fixture_root.exists()
        if not keep_temp and not result.temp_cleaned:
            _fail(result, "cleanup-failure", "temporary interaction fixture was not removed")
    result.finished_at = utc_now()
    return result


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--exe", required=True, help="Path to the Desktop executable")
    parser.add_argument("--artifact", help="Candidate package that installed the executable")
    parser.add_argument("--out", help="Write JSON evidence to this path")
    parser.add_argument(
        "--timeout-seconds",
        type=int,
        default=30,
        help=f"Per-step timeout ({MIN_TIMEOUT_SECONDS}-{MAX_TIMEOUT_SECONDS} seconds)",
    )
    parser.add_argument("--keep-temp", action="store_true")
    args = parser.parse_args(argv)
    try:
        validate_timeout(args.timeout_seconds)
    except ValueError as exc:
        parser.error(str(exc))

    result = run_smoke(
        args.exe,
        timeout_seconds=args.timeout_seconds,
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
