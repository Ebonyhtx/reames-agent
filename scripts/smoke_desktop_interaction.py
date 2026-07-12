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


SCHEMA_VERSION = 3
MIN_TIMEOUT_SECONDS = 10
MAX_TIMEOUT_SECONDS = 180
WORKSPACE_TITLE = "Native UI Workspace"
COMPOSER_AUTOMATION_ID = "composer-input"
SEND_AUTOMATION_ID = "composer-send"
STOP_AUTOMATION_ID = "composer-stop"
ONBOARDING_AUTOMATION_ID = "onboarding-key"
LOOPBACK_PROVIDER_NAME = "native-smoke"
LOOPBACK_MODEL = "native-smoke-model"
LOOPBACK_RESPONSE = "Native Desktop interaction smoke response"
LOOPBACK_API_KEY_ENV = "REAMES_NATIVE_SMOKE_API_KEY"
LOOPBACK_API_KEY = "invalid-local-fixture-key"
LONG_RUNNING_COMMAND = 'python -c "import time; time.sleep(30)"'
INVALID_KEY_PROMPT = "Native failure fixture: invalid API key"
RATE_LIMIT_PROMPT = "Native failure fixture: rate limit then recover"
STREAM_INTERRUPTION_PROMPT = "Native failure fixture: interrupt the response stream"
PERMISSION_DENIAL_PROMPT = "Native failure fixture: request a denied file write"
TOOL_TIMEOUT_PROMPT = "Native failure fixture: run a command that times out"
STREAM_PARTIAL_RESPONSE = "Native partial response before disconnect"
STREAM_RECOVERY_MARKER = "previous assistant response was interrupted"
PERMISSION_DENIAL_RESPONSE = "Native permission denial handled without writing the file"
TOOL_TIMEOUT_RESPONSE = "Native tool timeout handled after the command was stopped"
DENIED_RELATIVE_PATH = "native-denied.txt"
PERMISSION_TOOL_CALL_ID = "native-denied-call"
TIMEOUT_TOOL_CALL_ID = "native-timeout-call"
TOOL_APPROVAL_AUTOMATION_ID = "tool-approval-dialog"
TOOL_DENY_AUTOMATION_ID = "tool-approval-deny"
RETRY_STATUS_PREFIX = "retrying ("
AUTH_SETTINGS_ACTION_AUTOMATION_ID = "error-action-settings-provider_auth"
STREAM_RETRY_ACTION_AUTOMATION_ID = "error-action-retry-stream_interrupted"
SETTINGS_MODAL_AUTOMATION_ID = "settings-modal"
SETTINGS_CLOSE_AUTOMATION_ID = "settings-modal-close"

FAILURE_SCENARIOS = (
    "invalid_key",
    "rate_limit",
    "stream_interruption",
    "permission_denial",
    "tool_timeout",
)


def utc_now() -> str:
    return datetime.now(timezone.utc).isoformat()


@dataclass
class FailureScenarioResult:
    provider_requests: int = 0
    signal_visible: bool = False
    idle_recovered: bool = False
    followup_succeeded: bool = False


def new_failure_scenarios() -> dict[str, FailureScenarioResult]:
    return {name: FailureScenarioResult() for name in FAILURE_SCENARIOS}


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
    failure_scenarios: dict[str, FailureScenarioResult] = field(
        default_factory=new_failure_scenarios
    )
    stream_partial_persisted: bool = False
    auth_settings_opened: bool = False
    stream_retry_invoked: bool = False
    permission_denied: bool = False
    permission_write_blocked: bool = False
    tool_timeout_error_visible: bool = False
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
    root = Path(tempfile.mkdtemp(prefix="fixture-", dir=parent))
    try:
        yield root
    finally:
        native.remove_tree_with_retries(root)


def interaction_smoke_config(base_url: str) -> str:
    return f'''\
default_model = "{LOOPBACK_PROVIDER_NAME}/{LOOPBACK_MODEL}"

[[providers]]
name = "{LOOPBACK_PROVIDER_NAME}"
kind = "openai"
base_url = "{base_url}"
api_key_env = "{LOOPBACK_API_KEY_ENV}"
models = ["{LOOPBACK_MODEL}"]
default = "{LOOPBACK_MODEL}"
no_proxy = true

[tools]
bash_timeout_seconds = 1

[permissions]
mode = "ask"
allow = ["Bash(python -c:*)"]

[desktop]
close_behavior = "quit"
check_updates = false
onboarding_dismissed = true
language = "en"
default_tool_approval_mode = "ask"
provider_access = ["{LOOPBACK_PROVIDER_NAME}"]
'''


def latest_user_text(payload: dict[str, object]) -> str:
    messages = payload.get("messages")
    if not isinstance(messages, list):
        return ""
    for message in reversed(messages):
        if not isinstance(message, dict) or message.get("role") != "user":
            continue
        content = message.get("content")
        return content if isinstance(content, str) else json.dumps(content)
    return ""


def last_message_role(payload: dict[str, object]) -> str:
    messages = payload.get("messages")
    if not isinstance(messages, list):
        return ""
    for message in reversed(messages):
        if isinstance(message, dict) and isinstance(message.get("role"), str):
            return str(message["role"])
    return ""


def request_scenario(payload: dict[str, object]) -> str:
    prompt = latest_user_text(payload)
    if STREAM_RECOVERY_MARKER in prompt.lower():
        return "stream_interruption"
    scenarios = {
        INVALID_KEY_PROMPT: "invalid_key",
        RATE_LIMIT_PROMPT: "rate_limit",
        STREAM_INTERRUPTION_PROMPT: "stream_interruption",
        PERMISSION_DENIAL_PROMPT: "permission_denial",
        TOOL_TIMEOUT_PROMPT: "tool_timeout",
    }
    return next((name for marker, name in scenarios.items() if marker in prompt), "success")


def success_response(payload: dict[str, object]) -> str:
    prompt = latest_user_text(payload).strip()
    return f"{LOOPBACK_RESPONSE}: {prompt}" if prompt else LOOPBACK_RESPONSE


def sse_text_response(text: str) -> bytes:
    chunks = [
        {
            "choices": [
                {
                    "index": 0,
                    "delta": {"role": "assistant", "content": text},
                    "finish_reason": None,
                }
            ]
        },
        {
            "choices": [{"index": 0, "delta": {}, "finish_reason": "stop"}],
            "usage": {
                "prompt_tokens": 8,
                "completion_tokens": 5,
                "total_tokens": 13,
            },
        },
    ]
    stream = "".join(
        f"data: {json.dumps(chunk, separators=(',', ':'))}\n\n" for chunk in chunks
    ) + "data: [DONE]\n\n"
    return stream.encode("utf-8")


def sse_tool_call_response(call_id: str, name: str, arguments: dict[str, str]) -> bytes:
    chunks = [
        {
            "choices": [
                {
                    "index": 0,
                    "delta": {
                        "role": "assistant",
                        "tool_calls": [
                            {
                                "index": 0,
                                "id": call_id,
                                "type": "function",
                                "function": {
                                    "name": name,
                                    "arguments": json.dumps(arguments, separators=(",", ":")),
                                },
                            }
                        ],
                    },
                    "finish_reason": None,
                }
            ]
        },
        {
            "choices": [
                {"index": 0, "delta": {}, "finish_reason": "tool_calls"}
            ]
        },
    ]
    stream = "".join(
        f"data: {json.dumps(chunk, separators=(',', ':'))}\n\n" for chunk in chunks
    ) + "data: [DONE]\n\n"
    return stream.encode("utf-8")


@contextmanager
def local_openai_server() -> Iterator[tuple[str, list[dict[str, object]]]]:
    """Serve deterministic success and failure turns on localhost only."""

    requests: list[dict[str, object]] = []
    request_lock = threading.Lock()
    rate_limit_attempts = 0

    class Handler(http.server.BaseHTTPRequestHandler):
        protocol_version = "HTTP/1.1"

        def send_body(
            self,
            status: int,
            body: bytes,
            content_type: str,
            headers: dict[str, str] | None = None,
        ) -> None:
            self.send_response(status)
            self.send_header("Content-Type", content_type)
            self.send_header("Content-Length", str(len(body)))
            self.send_header("Connection", "close")
            for key, value in (headers or {}).items():
                self.send_header(key, value)
            self.end_headers()
            self.wfile.write(body)

        def send_sse(self, body: bytes) -> None:
            self.send_body(
                200,
                body,
                "text/event-stream",
                {"Cache-Control": "no-cache"},
            )

        def do_POST(self) -> None:  # noqa: N802 - stdlib handler API
            nonlocal rate_limit_attempts
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
                self.send_body(400, body, "application/json")
                return

            with request_lock:
                requests.append(payload)
            scenario = request_scenario(payload)

            if scenario == "invalid_key":
                body = json.dumps(
                    {"error": {"message": "Incorrect API key", "code": "invalid_api_key"}}
                ).encode("utf-8")
                self.send_body(401, body, "application/json")
                return

            if scenario == "rate_limit":
                with request_lock:
                    rate_limit_attempts += 1
                    attempt = rate_limit_attempts
                if attempt == 1:
                    body = json.dumps(
                        {"error": {"message": "Rate limit reached", "type": "rate_limit_error"}}
                    ).encode("utf-8")
                    self.send_body(
                        429,
                        body,
                        "application/json",
                        {"Retry-After": "8"},
                    )
                    return
                self.send_sse(sse_text_response(success_response(payload)))
                return

            if scenario == "stream_interruption":
                partial = {
                    "choices": [
                        {
                            "index": 0,
                            "delta": {
                                "role": "assistant",
                                "content": STREAM_PARTIAL_RESPONSE,
                            },
                            "finish_reason": None,
                        }
                    ]
                }
                body = (
                    f"data: {json.dumps(partial, separators=(',', ':'))}\n\n"
                ).encode("utf-8")
                self.send_response(200)
                self.send_header("Content-Type", "text/event-stream")
                self.send_header("Cache-Control", "no-cache")
                self.send_header("Connection", "close")
                self.end_headers()
                self.wfile.write(body)
                self.wfile.flush()
                self.close_connection = True
                return

            if scenario == "permission_denial":
                if last_message_role(payload) == "tool":
                    self.send_sse(sse_text_response(PERMISSION_DENIAL_RESPONSE))
                else:
                    self.send_sse(
                        sse_tool_call_response(
                            PERMISSION_TOOL_CALL_ID,
                            "write_file",
                            {
                                "path": DENIED_RELATIVE_PATH,
                                "content": "this fixture must never be written\n",
                            },
                        )
                    )
                return

            if scenario == "tool_timeout":
                if last_message_role(payload) == "tool":
                    self.send_sse(sse_text_response(TOOL_TIMEOUT_RESPONSE))
                else:
                    self.send_sse(
                        sse_tool_call_response(
                            TIMEOUT_TOOL_CALL_ID,
                            "bash",
                            {"command": LONG_RUNNING_COMMAND},
                        )
                    )
                return

            self.send_sse(sse_text_response(success_response(payload)))

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
            try:
                record = json.loads(line)
            except json.JSONDecodeError:
                # The app appends canonical events while the smoke samples the
                # file. A trailing partial record must not erase the valid
                # prefix that already proves persistence; stop at the first
                # incomplete record and let the next poll observe it finished.
                break
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
    env[LOOPBACK_API_KEY_ENV] = LOOPBACK_API_KEY
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


def submit_prompt(uia: object, prompt: str, timeout_seconds: float) -> None:
    uia.type_text(prompt, automation_id=COMPOSER_AUTOMATION_ID)
    uia.wait_enabled(
        automation_id=SEND_AUTOMATION_ID,
        timeout_seconds=timeout_seconds,
    )
    try:
        uia.press_enter(
            automation_id=COMPOSER_AUTOMATION_ID,
            timeout_seconds=timeout_seconds,
        )
    except RuntimeError as exc:
        # WebView2 occasionally accepts ValuePattern input but drops both the
        # posted and SendInput Enter events on a busy native runner. The send
        # button is the same product action and has a stable automation ID, so
        # use its InvokePattern as a bounded fallback instead of discarding an
        # otherwise valid end-to-end run. Do not hide unrelated UIA failures.
        if "UIA Enter did not submit composer" not in str(exc):
            raise
        uia.invoke(
            automation_id=SEND_AUTOMATION_ID,
            timeout_seconds=timeout_seconds,
        )


def verify_idle_and_followup(
    uia: object,
    home: Path,
    scenario: str,
    timeout_seconds: float,
) -> tuple[bool, bool]:
    uia.wait_absent(
        automation_id=STOP_AUTOMATION_ID,
        timeout_seconds=timeout_seconds,
    )
    followup = f"Native recovery check {scenario} {uuid.uuid4().hex[:8]}"
    uia.type_text(followup, automation_id=COMPOSER_AUTOMATION_ID)
    uia.wait_enabled(
        automation_id=SEND_AUTOMATION_ID,
        timeout_seconds=timeout_seconds,
    )
    idle_recovered = True
    uia.press_enter(
        automation_id=COMPOSER_AUTOMATION_ID,
        timeout_seconds=timeout_seconds,
    )
    expected = f"{LOOPBACK_RESPONSE}: {followup}"
    wait_until(
        lambda: durable_session_has_message(
            (active_tab(home) or {}).get("sessionPath"), "assistant", expected
        ),
        timeout_seconds,
        f"{scenario} recovery turn was not persisted",
    )
    wait_until(
        lambda: uia.has(name=expected),
        timeout_seconds,
        f"{scenario} recovery response was not visible",
    )
    uia.wait_absent(
        automation_id=STOP_AUTOMATION_ID,
        timeout_seconds=timeout_seconds,
    )
    return idle_recovered, True


def wait_for_retry_signal(uia: object, timeout_seconds: float) -> None:
    observed: dict[tuple[str, str], tuple[int, bool]] = {}
    deadline = time.monotonic() + min(timeout_seconds, 12.0)
    while time.monotonic() < deadline:
        elements = uia.refresh()
        for item in elements:
            identity = (item.automation_id, item.name)
            if (
                item.automation_id
                in {
                    STOP_AUTOMATION_ID,
                    "composer-runstatus",
                }
                or "retry" in item.name.lower()
            ):
                observed[identity] = (item.control_type, item.enabled)
        if any(item.name.lower().startswith(RETRY_STATUS_PREFIX) for item in elements):
            return
        time.sleep(0.1)
    raise TimeoutError(
        "429 retry status was not visible; observed retry controls: "
        + json.dumps(
            [
                {
                    "automation_id": automation_id,
                    "name": name,
                    "control_type": meta[0],
                    "enabled": meta[1],
                }
                for (automation_id, name), meta in observed.items()
            ],
            ensure_ascii=False,
        )
    )


def wait_for_error_notice(
    uia: object, error_code: str, timeout_seconds: float
) -> None:
    expected = f"notice-{error_code}"
    observed: dict[tuple[str, str], tuple[int, bool]] = {}
    deadline = time.monotonic() + min(timeout_seconds, 12.0)
    while time.monotonic() < deadline:
        elements = uia.refresh()
        for item in elements:
            identity = (item.automation_id, item.name)
            if item.automation_id.startswith("notice-") or "interrupt" in item.name.lower():
                observed[identity] = (item.control_type, item.enabled)
        if any(item.automation_id == expected for item in elements):
            return
        time.sleep(0.1)
    raise TimeoutError(
        f"{error_code} warning was not visible; observed notices: "
        + json.dumps(
            [
                {
                    "automation_id": automation_id,
                    "name": name,
                    "control_type": meta[0],
                    "enabled": meta[1],
                }
                for (automation_id, name), meta in observed.items()
            ],
            ensure_ascii=False,
        )
    )


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

            baseline_response = f"{LOOPBACK_RESPONSE}: {result.marker}"
            submit_prompt(uia, result.marker, timeout_seconds)
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
                    baseline_response,
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

            submit_prompt(uia, INVALID_KEY_PROMPT, timeout_seconds)
            wait_until(
                lambda: uia.has(automation_id="notice-provider_auth"),
                timeout_seconds,
                "invalid API key warning was not visible",
            )
            invalid_key = result.failure_scenarios["invalid_key"]
            invalid_key.signal_visible = True
            uia.invoke(
                automation_id=AUTH_SETTINGS_ACTION_AUTOMATION_ID,
                timeout_seconds=timeout_seconds,
            )
            wait_until(
                lambda: uia.has(automation_id=SETTINGS_MODAL_AUTOMATION_ID),
                timeout_seconds,
                "authentication failure action did not open model settings",
            )
            result.auth_settings_opened = True
            uia.invoke(
                automation_id=SETTINGS_CLOSE_AUTOMATION_ID,
                timeout_seconds=timeout_seconds,
            )
            uia.wait_absent(
                automation_id=SETTINGS_MODAL_AUTOMATION_ID,
                timeout_seconds=timeout_seconds,
            )
            (
                invalid_key.idle_recovered,
                invalid_key.followup_succeeded,
            ) = verify_idle_and_followup(
                uia, home, "invalid_key", timeout_seconds
            )

            submit_prompt(uia, RATE_LIMIT_PROMPT, timeout_seconds)
            wait_for_retry_signal(uia, timeout_seconds)
            rate_limit = result.failure_scenarios["rate_limit"]
            rate_limit.signal_visible = True
            rate_limit_response = f"{LOOPBACK_RESPONSE}: {RATE_LIMIT_PROMPT}"
            wait_until(
                lambda: durable_session_has_message(
                    (active_tab(home) or {}).get("sessionPath"),
                    "assistant",
                    rate_limit_response,
                ),
                timeout_seconds,
                "429 retry did not recover to a persisted response",
            )
            wait_until(
                lambda: uia.has(name=rate_limit_response),
                timeout_seconds,
                "429 retry recovery response was not visible",
            )
            (
                rate_limit.idle_recovered,
                rate_limit.followup_succeeded,
            ) = verify_idle_and_followup(
                uia, home, "rate_limit", timeout_seconds
            )

            submit_prompt(uia, STREAM_INTERRUPTION_PROMPT, timeout_seconds)
            wait_for_error_notice(uia, "stream_interrupted", timeout_seconds)
            stream_interruption = result.failure_scenarios["stream_interruption"]
            stream_interruption.signal_visible = True
            wait_until(
                lambda: durable_session_has_message(
                    (active_tab(home) or {}).get("sessionPath"),
                    "assistant",
                    STREAM_PARTIAL_RESPONSE,
                ),
                timeout_seconds,
                "partial stream output was not persisted after disconnect",
            )
            result.stream_partial_persisted = True
            uia.invoke(
                automation_id=STREAM_RETRY_ACTION_AUTOMATION_ID,
                timeout_seconds=timeout_seconds,
            )
            result.stream_retry_invoked = True
            continuation_prompt = (
                "Continue from the interrupted response without repeating completed work."
            )
            continuation_response = f"{LOOPBACK_RESPONSE}: {continuation_prompt}"
            wait_until(
                lambda: durable_session_has_message(
                    (active_tab(home) or {}).get("sessionPath"),
                    "assistant",
                    continuation_response,
                ),
                timeout_seconds,
                "stream continuation action did not persist a follow-up response",
            )
            wait_until(
                lambda: uia.has(name=continuation_response),
                timeout_seconds,
                "stream continuation response was not visible",
            )
            uia.wait_enabled(
                automation_id=COMPOSER_AUTOMATION_ID,
                timeout_seconds=timeout_seconds,
            )
            stream_interruption.idle_recovered = True
            stream_interruption.followup_succeeded = True

            submit_prompt(uia, PERMISSION_DENIAL_PROMPT, timeout_seconds)
            wait_until(
                lambda: uia.has(automation_id=TOOL_APPROVAL_AUTOMATION_ID),
                timeout_seconds,
                "write_file approval dialog was not visible",
            )
            permission_denial = result.failure_scenarios["permission_denial"]
            permission_denial.signal_visible = True
            uia.invoke(
                automation_id=TOOL_DENY_AUTOMATION_ID,
                timeout_seconds=timeout_seconds,
            )
            result.permission_denied = True
            uia.wait_absent(
                automation_id=TOOL_APPROVAL_AUTOMATION_ID,
                timeout_seconds=timeout_seconds,
            )
            wait_until(
                lambda: uia.has(
                    automation_id=f"tool-error-{PERMISSION_TOOL_CALL_ID}"
                ),
                timeout_seconds,
                "denied write_file tool error was not visible",
            )
            wait_until(
                lambda: uia.has(name=PERMISSION_DENIAL_RESPONSE),
                timeout_seconds,
                "permission denial explanation was not visible",
            )
            result.permission_write_blocked = not (
                workspace / DENIED_RELATIVE_PATH
            ).exists()
            if not result.permission_write_blocked:
                raise RuntimeError("denied write_file unexpectedly modified the workspace")
            (
                permission_denial.idle_recovered,
                permission_denial.followup_succeeded,
            ) = verify_idle_and_followup(
                uia, home, "permission_denial", timeout_seconds
            )

            submit_prompt(uia, TOOL_TIMEOUT_PROMPT, timeout_seconds)
            wait_until(
                lambda: uia.has(automation_id=f"tool-error-{TIMEOUT_TOOL_CALL_ID}"),
                timeout_seconds,
                "timed-out bash tool error was not visible",
            )
            tool_timeout = result.failure_scenarios["tool_timeout"]
            tool_timeout.signal_visible = True
            result.tool_timeout_error_visible = True
            wait_until(
                lambda: uia.has(name=TOOL_TIMEOUT_RESPONSE),
                timeout_seconds,
                "tool timeout explanation was not visible",
            )
            (
                tool_timeout.idle_recovered,
                tool_timeout.followup_succeeded,
            ) = verify_idle_and_followup(
                uia, home, "tool_timeout", timeout_seconds
            )

            uia.type_text(
                f"!{LONG_RUNNING_COMMAND}", automation_id=COMPOSER_AUTOMATION_ID
            )
            uia.wait_enabled(
                name=SEND_NAMES,
                automation_id=SEND_AUTOMATION_ID,
                timeout_seconds=timeout_seconds,
            )
            uia.press_enter(
                automation_id=COMPOSER_AUTOMATION_ID,
                timeout_seconds=timeout_seconds,
            )
            uia.invoke(
                name=STOP_NAMES,
                automation_id=STOP_AUTOMATION_ID,
                timeout_seconds=timeout_seconds,
            )
            result.stop_visible = True
            result.stop_invoked = True
            uia.wait_absent(
                name=STOP_NAMES,
                automation_id=STOP_AUTOMATION_ID,
                timeout_seconds=timeout_seconds,
            )
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
                lambda: uia.has(name=result.marker) and uia.has(name=baseline_response),
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
        for scenario, evidence in result.failure_scenarios.items():
            evidence.provider_requests = sum(
                1
                for payload in provider_requests
                if request_scenario(payload) == scenario
            )
            if (
                evidence.provider_requests == 0
                or not evidence.signal_visible
                or not evidence.idle_recovered
                or not evidence.followup_succeeded
            ):
                _fail(
                    result,
                    "interaction-failure",
                    f"failure scenario did not close its native contract: {scenario}",
                )
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
