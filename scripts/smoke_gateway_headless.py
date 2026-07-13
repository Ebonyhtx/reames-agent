#!/usr/bin/env python3
"""Credential-free cloud-node operations preflight.

This is intentionally stronger than a text contract and weaker than a real IM
round trip: it runs the actual CLI binary against an isolated Reames Agent home,
applies `gateway setup`, checks `gateway doctor --home`, renders a service-manager
dry-run plan, completes a localhost-backed one-shot CLI turn, and exercises the
local feedback ledger through maintenance-draft generation. It does not start a
background service and never requires real provider or bot secrets.
"""

from __future__ import annotations

import argparse
import http.server
import json
import os
import platform
import subprocess
import tempfile
import threading
from contextlib import contextmanager
from pathlib import Path
from typing import Iterator


ROOT = Path(__file__).resolve().parents[1]
SCHEMA_VERSION = 2
SMOKE_APP_ID = "cli-smoke-feishu-app"
SMOKE_SECRET_ENV = "FEISHU_BOT_APP_SECRET"
LOOPBACK_PROVIDER = "headless-smoke"
LOOPBACK_MODEL = "headless-smoke-model"
LOOPBACK_KEY_ENV = "REAMES_HEADLESS_SMOKE_API_KEY"
LOOPBACK_KEY = "synthetic-local-fixture-key"
CLI_MARKER = "Headless cloud operations marker"
CLI_RESPONSE = "Headless cloud operations response"
FEEDBACK_EMAIL = "operator@example.invalid"
FEEDBACK_SECRET = "sk-headless-smoke-secret-1234567890"
FEEDBACK_MESSAGE = f"Gateway delivery failed for {FEEDBACK_EMAIL}; api_key={FEEDBACK_SECRET}"


def run_result(args: list[str], *, env: dict[str, str] | None = None, timeout: int = 30) -> tuple[int, str]:
    proc = subprocess.run(
        args,
        cwd=ROOT,
        env=env,
        text=True,
        encoding="utf-8",
        errors="replace",
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        check=False,
        timeout=timeout,
    )
    return proc.returncode, proc.stdout


def run(args: list[str], *, env: dict[str, str] | None = None, timeout: int = 30) -> str:
    code, out = run_result(args, env=env, timeout=timeout)
    if code != 0:
        raise RuntimeError(
            f"command failed with exit code {code}: {args!r}\n{out}"
        )
    return out


def build_binary(work: Path) -> Path:
    suffix = ".exe" if platform.system() == "Windows" else ""
    binary = work / f"reames-agent-smoke{suffix}"
    run(["go", "build", "-o", str(binary), "./cmd/reames-agent"], timeout=180)
    return binary


def prepare_workspace(home: Path) -> Path:
    workspace = home / "workspace"
    workspace.mkdir(parents=True, exist_ok=True)
    return workspace


def setup_args(binary: Path, home: Path, workspace: Path, *, dry_run: bool = False) -> list[str]:
    args = [
        str(binary),
        "gateway",
        "setup",
        "--home",
        str(home),
        "--channel",
        "feishu",
        "--app-id",
        SMOKE_APP_ID,
        "--app-secret-env",
        SMOKE_SECRET_ENV,
        "--workspace",
        str(workspace),
        "--model",
        "deepseek-flash",
        "--users",
        "ou-smoke-user",
    ]
    if dry_run:
        args.append("--dry-run")
    return args


def assert_contains(text: str, needle: str) -> None:
    if needle not in text:
        raise AssertionError(f"output missing {needle!r}:\n{text}")


def assert_not_contains(text: str, needle: str) -> None:
    if needle in text:
        raise AssertionError(f"output leaked {needle!r}:\n{text}")


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
    body = "".join(
        f"data: {json.dumps(chunk, separators=(',', ':'))}\n\n" for chunk in chunks
    ) + "data: [DONE]\n\n"
    return body.encode("utf-8")


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


@contextmanager
def local_openai_server() -> Iterator[tuple[str, list[dict[str, object]]]]:
    requests: list[dict[str, object]] = []
    request_lock = threading.Lock()

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
            with request_lock:
                requests.append(payload)
            body = sse_text_response(CLI_RESPONSE)
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.send_header("Content-Length", str(len(body)))
            self.send_header("Cache-Control", "no-cache")
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


def append_loopback_provider(config_path: Path, base_url: str) -> None:
    body = config_path.read_text(encoding="utf-8")
    if f'name        = "{LOOPBACK_PROVIDER}"' in body:
        raise AssertionError("loopback provider already exists in smoke config")
    provider = f'''\

[[providers]]
name        = "{LOOPBACK_PROVIDER}"
kind        = "openai"
base_url    = "{base_url}"
model       = "{LOOPBACK_MODEL}"
api_key_env = "{LOOPBACK_KEY_ENV}"
no_proxy    = true
'''
    config_path.write_text(body.rstrip() + provider, encoding="utf-8")


def assert_sensitive_values_absent(text: str) -> None:
    for forbidden in (FEEDBACK_EMAIL, FEEDBACK_SECRET, LOOPBACK_KEY):
        assert_not_contains(text, forbidden)


def assert_sensitive_values_absent_from_tree(root: Path) -> None:
    forbidden_values = (FEEDBACK_EMAIL, FEEDBACK_SECRET, LOOPBACK_KEY)
    for path in sorted(root.rglob("*")):
        if not path.is_file():
            continue
        body = path.read_bytes()
        for forbidden in forbidden_values:
            if forbidden.encode("utf-8") in body:
                raise AssertionError(f"persisted evidence leaked {forbidden!r}: {path}")


def assert_report_sections_complete(report: dict[str, object]) -> None:
    for section in ("setup", "doctor", "service_plan", "foreground_run", "cli_run", "feedback"):
        value = report.get(section)
        if not isinstance(value, dict) or not value:
            raise AssertionError(f"smoke report section {section!r} is missing or empty")


def run_cli_preflight(
    binary: Path,
    home: Path,
    workspace: Path,
    base_url: str,
    requests: list[dict[str, object]],
    env: dict[str, str],
) -> dict[str, object]:
    config_path = home / "config.toml"
    append_loopback_provider(config_path, base_url)
    run_env = env.copy()
    run_env["REAMES_AGENT_HOME"] = str(home)
    run_env[LOOPBACK_KEY_ENV] = LOOPBACK_KEY
    metrics_path = home / "evidence" / "cli-run-metrics.json"
    metrics_path.parent.mkdir(parents=True, exist_ok=True)
    output = run(
        [
            str(binary),
            "run",
            "--model",
            LOOPBACK_PROVIDER,
            "--max-steps",
            "1",
            "--metrics",
            str(metrics_path),
            "--dir",
            str(workspace),
            CLI_MARKER,
        ],
        env=run_env,
        timeout=90,
    )
    assert_contains(output, CLI_RESPONSE)
    assert_sensitive_values_absent(output)
    if len(requests) != 1:
        raise AssertionError(f"one-shot CLI issued {len(requests)} Provider requests, want exactly one")
    if CLI_MARKER not in latest_user_text(requests[0]):
        raise AssertionError("one-shot CLI marker did not reach the localhost provider")
    try:
        metrics = json.loads(metrics_path.read_text(encoding="utf-8"))
    except (OSError, ValueError, TypeError) as exc:
        raise AssertionError(f"one-shot CLI metrics were not persisted: {exc}") from exc
    if not isinstance(metrics, dict):
        raise AssertionError("one-shot CLI metrics are not a JSON object")
    session_files = [
        path
        for path in home.rglob("*.jsonl")
        if "feedback" not in {part.lower() for part in path.parts}
    ]
    persisted = False
    for path in session_files:
        text = path.read_text(encoding="utf-8", errors="replace")
        if CLI_MARKER in text and CLI_RESPONSE in text:
            persisted = True
            break
    if not persisted:
        raise AssertionError("one-shot CLI turn was not persisted under the selected home")
    assert_sensitive_values_absent(config_path.read_text(encoding="utf-8"))
    return {
        "provider_bound_to_loopback": base_url.startswith("http://127.0.0.1:"),
        "provider_requests": len(requests),
        "provider_received_marker": CLI_MARKER in latest_user_text(requests[0]),
        "response_visible": CLI_RESPONSE in output,
        "metrics_persisted": metrics_path.is_file(),
        "session_persisted": persisted,
        "session_files": len(session_files),
        "synthetic_key_not_persisted": LOOPBACK_KEY not in config_path.read_text(encoding="utf-8"),
    }


def run_feedback_preflight(binary: Path, home: Path, env: dict[str, str]) -> dict[str, object]:
    feedback_env = env.copy()
    feedback_env["REAMES_AGENT_HOME"] = str(home)
    submit_args = [
        str(binary),
        "feedback",
        "submit",
        "--json",
        "--home",
        str(home),
        "--kind",
        "bot",
        "--source",
        "gateway",
        "--label",
        "feishu",
        "--channel",
        "feishu",
        "--message",
        FEEDBACK_MESSAGE,
    ]
    first_output = run(submit_args, env=feedback_env)
    second_output = run(submit_args, env=feedback_env)
    first = json.loads(first_output)
    second = json.loads(second_output)
    if first.get("fingerprint") != second.get("fingerprint"):
        raise AssertionError("duplicate feedback did not retain a stable fingerprint")

    summary_output = run(
        [str(binary), "feedback", "summary", "--json", "--home", str(home)],
        env=feedback_env,
    )
    summary = json.loads(summary_output)
    groups = summary.get("groups")
    if summary.get("total") != 2 or not isinstance(groups, list) or len(groups) != 1 or groups[0].get("count") != 2:
        raise AssertionError(f"feedback summary did not aggregate duplicates: {summary}")

    draft_output = run(
        [str(binary), "feedback", "draft", "--json", "--home", str(home), "--limit", "20"],
        env=feedback_env,
    )
    draft = json.loads(draft_output)
    draft_path = Path(str(draft.get("path", "")))
    if draft.get("total") != 2 or draft.get("groups") != 1 or not draft_path.is_file():
        raise AssertionError(f"feedback maintenance draft was not persisted: {draft}")
    ledger_path = home / "feedback" / "feedback.jsonl"
    evidence_text = "\n".join(
        [
            first_output,
            second_output,
            summary_output,
            draft_output,
            ledger_path.read_text(encoding="utf-8"),
            draft_path.read_text(encoding="utf-8"),
        ]
    )
    assert_sensitive_values_absent(evidence_text)
    return {
        "records": summary["total"],
        "duplicate_groups": len(groups),
        "duplicate_count": groups[0]["count"],
        "ledger_persisted": ledger_path.is_file(),
        "draft_persisted": draft_path.is_file(),
        "sensitive_values_redacted": True,
        "external_publish_attempted": False,
    }


def smoke(binary: Path, home: Path) -> dict[str, object]:
    config_path = home / "config.toml"
    if config_path.exists():
        raise AssertionError(f"headless Gateway smoke requires a clean home: {config_path} already exists")
    workspace = prepare_workspace(home)
    env = os.environ.copy()
    env.pop("REAMES_AGENT_HOME", None)
    env["REAMES_AGENT_CREDENTIALS_STORE"] = "file"

    setup_dry_run = run(setup_args(binary, home, workspace, dry_run=True), env=env)
    assert_contains(setup_dry_run, "gateway setup plan:")
    assert_contains(setup_dry_run, "action: create")
    assert_contains(setup_dry_run, "write: skipped (dry-run)")
    assert_not_contains(setup_dry_run, SMOKE_APP_ID)
    if config_path.exists():
        raise AssertionError("gateway setup --dry-run created config.toml")

    setup_apply = run(setup_args(binary, home, workspace), env=env)
    assert_contains(setup_apply, "action: create")
    assert_contains(setup_apply, "write: applied atomically")
    assert_not_contains(setup_apply, SMOKE_APP_ID)
    if not config_path.is_file():
        raise AssertionError("gateway setup did not create config.toml")
    first_config = config_path.read_bytes()

    setup_repeat = run(setup_args(binary, home, workspace), env=env)
    assert_contains(setup_repeat, "action: unchanged")
    assert_contains(setup_repeat, "write: unchanged")
    assert_not_contains(setup_repeat, SMOKE_APP_ID)
    if config_path.read_bytes() != first_config:
        raise AssertionError("idempotent gateway setup rewrote config.toml")
    if (home / ".env").exists():
        raise AssertionError("gateway setup unexpectedly created a credential file")
    setup_contracts = {
        "dry_run_zero_write": "write: skipped (dry-run)" in setup_dry_run,
        "applied_atomically": "write: applied atomically" in setup_apply,
        "idempotent_action": "action: unchanged" in setup_repeat,
        "idempotent_bytes": config_path.read_bytes() == first_config,
        "app_id_redacted": SMOKE_APP_ID not in setup_dry_run + setup_apply + setup_repeat,
        "no_credential_file": not (home / ".env").exists(),
    }

    doctor = run(
        [
            str(binary),
            "gateway",
            "doctor",
            "--json",
            "--deep",
            "--home",
            str(home),
        ],
        env=env,
    )
    checks = {item.get("name"): item for item in json.loads(doctor)}
    if checks.get("bot.home", {}).get("status") != "ok" or checks.get("bot.home", {}).get("detail") != str(home):
        raise AssertionError(f"bot.home did not bind to selected home {home}:\n{doctor}")
    secret_check = checks.get("bot.feishu.app_secret", {})
    if secret_check.get("status") != "missing" or secret_check.get("detail") != f"{SMOKE_SECRET_ENV} is not set":
        raise AssertionError(f"bot.feishu.app_secret did not report the missing environment variable:\n{doctor}")
    assert_not_contains(doctor, SMOKE_APP_ID)
    doctor_contracts = {
        "home_bound": checks.get("bot.home", {}).get("detail") == str(home),
        "connection_recorded": checks.get("bot.connections", {}).get("status") == "ok",
        "missing_secret_reported": secret_check.get("status") == "missing",
        "missing_secret_named": secret_check.get("detail") == f"{SMOKE_SECRET_ENV} is not set",
        "app_id_redacted": SMOKE_APP_ID not in doctor,
    }

    plan = run(
        [
            str(binary),
            "gateway",
            "install",
            "--dry-run",
            "--home",
            str(home),
            "--channels",
            "feishu",
            "--dir",
            str(workspace),
            "--exe",
            str(binary),
        ],
        env=env,
    )
    assert_contains(plan, "gateway service plan:")
    assert_contains(plan, "REAMES_AGENT_HOME")
    assert_contains(plan, str(home))
    assert_contains(plan, ".env")
    assert_contains(plan, "service definitions do not embed secret values")
    assert_not_contains(plan, SMOKE_APP_ID)
    plan_contracts = {
        "renders_service_plan": "gateway service plan:" in plan,
        "pins_reames_agent_home": "REAMES_AGENT_HOME" in plan and str(home) in plan,
        "documents_credentials_env": ".env" in plan,
        "documents_no_secret_embedding": "service definitions do not embed secret values" in plan,
        "app_id_redacted": SMOKE_APP_ID not in plan,
    }
    ambient_home = home.parent / "ambient-home"
    selected_home = home.parent / "selected-foreground-home"
    ambient_workspace = prepare_workspace(ambient_home)
    run(setup_args(binary, ambient_home, ambient_workspace), env=env)
    selected_home.mkdir(parents=True, exist_ok=True)
    foreground_env = env.copy()
    foreground_env["REAMES_AGENT_HOME"] = str(ambient_home)
    foreground_code, foreground = run_result(
        [
            str(binary),
            "gateway",
            "run",
            "--home",
            str(selected_home),
            "--channels",
            "feishu",
        ],
        env=foreground_env,
    )
    if foreground_code != 1 or "gateway is not enabled" not in foreground:
        raise AssertionError(
            "gateway run --home did not bind to the selected foreground home "
            f"(exit={foreground_code}):\n{foreground}"
        )
    assert_not_contains(foreground, SMOKE_APP_ID)
    foreground_contracts = {
        "home_overrides_ambient_env": foreground_code == 1 and "gateway is not enabled" in foreground,
        "app_id_redacted": SMOKE_APP_ID not in foreground,
    }
    with local_openai_server() as (base_url, requests):
        cli_contracts = run_cli_preflight(binary, home, workspace, base_url, requests, env)
    feedback_contracts = run_feedback_preflight(binary, home, env)
    report = {
        "schema_version": SCHEMA_VERSION,
        "status": "passed",
        "evidence_scope": "credential-free-clean-node-preflight",
        "os": platform.system().lower(),
        "binary": str(binary),
        "home": str(home),
        "workspace": str(workspace),
        "setup": setup_contracts,
        "doctor": doctor_contracts,
        "service_plan": plan_contracts,
        "foreground_run": foreground_contracts,
        "cli_run": cli_contracts,
        "feedback": feedback_contracts,
        "external_blocked": [
            "real provider API round trip",
            "real IM text/approval/cancel/recovery round trip",
            "installed service-manager start/status/restart cycle",
        ],
    }
    assert_sensitive_values_absent_from_tree(home)
    assert_report_sections_complete(report)
    assert_sensitive_values_absent(json.dumps(report, sort_keys=True))
    return report


def write_report(path: Path, report: dict[str, object]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(report, indent=2, sort_keys=True) + "\n", encoding="utf-8")


def main() -> int:
    parser = argparse.ArgumentParser(description="Smoke-test headless Gateway deployment contracts.")
    parser.add_argument("--binary", type=Path, help="Existing reames-agent binary to smoke-test.")
    parser.add_argument("--home", type=Path, help="Existing or new Reames Agent home to use.")
    parser.add_argument("--out", type=Path, help="Write a JSON smoke report to this path.")
    parser.add_argument("--keep", action="store_true", help="Keep generated temporary files.")
    args = parser.parse_args()

    tmp_ctx = tempfile.TemporaryDirectory(prefix="reames-gateway-smoke-")
    tmp = Path(tmp_ctx.name)
    try:
        binary = args.binary.resolve() if args.binary else build_binary(tmp)
        if not binary.exists():
            raise FileNotFoundError(binary)
        home = args.home.resolve() if args.home else tmp / "home"
        home.mkdir(parents=True, exist_ok=True)
        report = smoke(binary, home)
        if args.out:
            write_report(args.out, report)
        print(f"Headless Gateway smoke passed: binary={binary} home={home}")
        if args.out:
            print(f"Smoke report: {args.out}")
        return 0
    finally:
        if args.keep:
            print(f"Kept smoke directory: {tmp}")
        else:
            tmp_ctx.cleanup()


if __name__ == "__main__":
    raise SystemExit(main())
