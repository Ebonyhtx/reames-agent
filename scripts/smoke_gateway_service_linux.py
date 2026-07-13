"""Exercise the real Linux systemd user lifecycle for the Gateway binary.

This smoke uses a loopback Feishu webhook challenge with a random verification
token. It proves that the service process is listening without using a real IM
application or provider credential; it does not claim an external round trip.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import platform
import shutil
import socket
import stat
import subprocess
import tempfile
import time
import uuid
import urllib.error
import urllib.request
from dataclasses import asdict, dataclass, field
from pathlib import Path


SCHEMA_VERSION = 1
EVIDENCE_SCOPE = "linux-systemd-user-gateway-lifecycle"
START_BANNER = "reames-agent gateway starting"


@dataclass
class GatewayServiceSmokeResult:
    schema_version: int = SCHEMA_VERSION
    status: str = "failed"
    evidence_scope: str = EVIDENCE_SCOPE
    platform: str = field(default_factory=lambda: platform.system().lower())
    binary_sha256: str = ""
    binary_size: int = 0
    systemd_manager_state: str = ""
    linger_state: str = ""
    logout_persistence_verified: bool = False
    service_name: str = ""
    config_written: bool = False
    config_mode: str = ""
    unit_written: bool = False
    unit_mode: str = ""
    unit_verified: bool = False
    unit_verify_output_empty: bool = False
    initial_active: bool = False
    initial_pid: int = 0
    initial_webhook_verified: bool = False
    reinstall_completed: bool = False
    reinstall_config_mode: str = ""
    reinstall_pid: int = 0
    reinstall_pid_changed: bool = False
    reinstall_unit_updated: bool = False
    reinstall_old_webhook_unreachable: bool = False
    reinstall_new_webhook_verified: bool = False
    status_completed: bool = False
    restart_active: bool = False
    restart_pid: int = 0
    restart_pid_changed: bool = False
    restart_webhook_verified: bool = False
    stop_inactive: bool = False
    stop_webhook_unreachable: bool = False
    start_active: bool = False
    start_pid: int = 0
    start_pid_changed: bool = False
    start_webhook_verified: bool = False
    journal_has_start_banner: bool = False
    token_absent_from_outputs: bool = False
    token_absent_from_unit: bool = False
    uninstall_completed: bool = False
    unit_removed: bool = False
    unit_load_state_after_uninstall: str = ""
    temp_cleaned: bool = False
    kept_temp: bool = False
    external_blocked: list[str] = field(
        default_factory=lambda: [
            "real provider API round trip",
            "real IM text/approval/cancel/recovery round trip",
            "logout persistence across SSH/session end",
            "maintained cloud VM service lifecycle",
        ]
    )
    errors: list[str] = field(default_factory=list)


def run_result(
    args: list[str],
    *,
    env: dict[str, str],
    timeout: int = 30,
) -> tuple[int, str]:
    proc = subprocess.run(
        args,
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


def run(args: list[str], *, env: dict[str, str], timeout: int = 30) -> str:
    code, output = run_result(args, env=env, timeout=timeout)
    if code != 0:
        raise RuntimeError(
            f"command failed with exit code {code}: {args!r}\n{output}"
        )
    return output


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest().upper()


def parse_systemctl_show(output: str) -> dict[str, str]:
    state: dict[str, str] = {}
    for line in output.splitlines():
        key, separator, value = line.partition("=")
        if separator and key:
            state[key] = value
    return state


def systemd_state(unit: str, env: dict[str, str]) -> dict[str, str]:
    code, output = run_result(
        [
            "systemctl",
            "--user",
            "show",
            unit,
            "--property=LoadState,ActiveState,SubState,MainPID",
        ],
        env=env,
    )
    if code != 0:
        raise RuntimeError(f"systemctl show failed for {unit}:\n{output}")
    return parse_systemctl_show(output)


def wait_for_state(
    unit: str,
    env: dict[str, str],
    *,
    active: str,
    timeout_seconds: float = 20.0,
    different_pid: int = 0,
) -> dict[str, str]:
    deadline = time.monotonic() + timeout_seconds
    last: dict[str, str] = {}
    while time.monotonic() < deadline:
        last = systemd_state(unit, env)
        pid = int(last.get("MainPID", "0") or "0")
        if last.get("ActiveState") == active:
            if active != "active" or (pid > 0 and pid != different_pid):
                return last
        time.sleep(0.2)
    raise TimeoutError(
        f"{unit} did not reach ActiveState={active} with a new pid; last={last}"
    )


def assert_secret_absent(secret: str, values: list[str]) -> None:
    for value in values:
        if secret and secret in value:
            raise AssertionError("synthetic service credential leaked into persisted evidence")


def reserve_loopback_port() -> int:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as probe:
        probe.bind(("127.0.0.1", 0))
        return int(probe.getsockname()[1])


def webhook_config(token: str, port: int) -> str:
    return f'''[bot]
enabled = true

[bot.pairing]
enabled = true

[bot.allowlist]
enabled = true
allow_all = false

[bot.feishu]
enabled = true
mode = "webhook"
verification_token = "{token}"
webhook_port = {port}
require_mention = true
'''


def verify_webhook(
    port: int,
    token: str,
    *,
    timeout_seconds: float = 20.0,
) -> bool:
    challenge = "reames-systemd-smoke-" + uuid.uuid4().hex
    payload = json.dumps(
        {"type": "url_verification", "token": token, "challenge": challenge}
    ).encode("utf-8")
    deadline = time.monotonic() + timeout_seconds
    last_error = "webhook did not answer"
    while time.monotonic() < deadline:
        request = urllib.request.Request(
            f"http://127.0.0.1:{port}/feishu/event",
            data=payload,
            headers={"Content-Type": "application/json"},
            method="POST",
        )
        try:
            with urllib.request.urlopen(request, timeout=1.0) as response:
                body = json.loads(response.read().decode("utf-8"))
                if response.status == 200 and body == {"challenge": challenge}:
                    return True
                last_error = f"unexpected webhook response: {response.status} {body!r}"
        except (OSError, urllib.error.URLError, json.JSONDecodeError) as exc:
            last_error = str(exc)
        time.sleep(0.2)
    raise TimeoutError(f"loopback webhook challenge failed: {last_error}")


def webhook_unreachable(port: int, token: str, *, timeout_seconds: float = 5.0) -> bool:
    try:
        verify_webhook(port, token, timeout_seconds=timeout_seconds)
    except TimeoutError:
        return True
    return False


def gateway_args(binary: Path, action: str, service_name: str) -> list[str]:
    return [str(binary), "gateway", action, "--name", service_name]


def run_smoke(binary: Path, *, keep_temp: bool = False) -> GatewayServiceSmokeResult:
    result = GatewayServiceSmokeResult(kept_temp=keep_temp)
    if platform.system() != "Linux":
        result.errors.append("Linux is required for the systemd user lifecycle smoke")
        return result

    binary = binary.expanduser().resolve()
    if not binary.is_file():
        result.errors.append(f"binary does not exist: {binary}")
        return result
    if not os.access(binary, os.X_OK):
        result.errors.append(f"binary is not executable: {binary}")
        return result

    result.binary_sha256 = sha256_file(binary)
    result.binary_size = binary.stat().st_size
    token = uuid.uuid4().hex[:10]
    service_name = f"reames-agent-gateway-smoke-{token}"
    unit = service_name + ".service"
    verification_token = "local-systemd-smoke-" + uuid.uuid4().hex
    webhook_port = reserve_loopback_port()
    result.service_name = service_name
    temp_root = Path(tempfile.mkdtemp(prefix="reames-gateway-systemd-smoke-"))
    home = temp_root / "agent home"
    workspace = temp_root / "workspace with spaces"
    home.mkdir(parents=True)
    workspace.mkdir(parents=True)
    config_path = home / "config.toml"
    config_path.write_text(
        webhook_config(verification_token, webhook_port), encoding="utf-8"
    )
    config_path.chmod(0o600)
    result.config_written = config_path.is_file()
    result.config_mode = oct(stat.S_IMODE(config_path.stat().st_mode))
    unit_path = Path.home() / ".config" / "systemd" / "user" / unit
    env = os.environ.copy()
    env.update(
        {
            "SYSTEMD_PAGER": "cat",
            "SYSTEMD_COLORS": "0",
            "REAMES_AGENT_HOME": str(home),
        }
    )
    outputs: list[str] = []
    uninstalled = False

    try:
        result.systemd_manager_state = run(
            ["systemctl", "--user", "is-system-running"], env=env
        ).strip()
        if result.systemd_manager_state not in {"running", "degraded"}:
            raise RuntimeError(
                f"systemd user manager is {result.systemd_manager_state!r}"
            )
        result.linger_state = run(
            [
                "loginctl",
                "show-user",
                str(os.getuid()),
                "--property=Linger",
                "--value",
            ],
            env=env,
        ).strip()

        install = run(
            gateway_args(binary, "install", service_name)
            + [
                "--home",
                str(home),
                "--channels",
                "feishu",
                "--dir",
                str(workspace),
                "--start-now",
            ],
            env=env,
            timeout=45,
        )
        outputs.append(install)
        result.unit_written = unit_path.is_file()
        if result.unit_written:
            result.unit_mode = oct(stat.S_IMODE(unit_path.stat().st_mode))
            outputs.append(unit_path.read_text(encoding="utf-8", errors="replace"))
        verify_output = run(
            ["systemd-analyze", "--user", "verify", str(unit_path)],
            env=env,
            timeout=30,
        )
        outputs.append(verify_output)
        result.unit_verified = True
        result.unit_verify_output_empty = not verify_output.strip()
        if not result.unit_verify_output_empty:
            raise RuntimeError(
                f"systemd unit verification emitted diagnostics:\n{verify_output}"
            )

        initial = wait_for_state(unit, env, active="active")
        result.initial_active = initial.get("ActiveState") == "active"
        result.initial_pid = int(initial.get("MainPID", "0") or "0")
        result.initial_webhook_verified = verify_webhook(
            webhook_port, verification_token
        )

        replacement_home = temp_root / "replacement agent home"
        replacement_workspace = temp_root / "replacement workspace with spaces"
        replacement_home.mkdir(parents=True)
        replacement_workspace.mkdir(parents=True)
        replacement_token = "local-systemd-reinstall-" + uuid.uuid4().hex
        replacement_port = reserve_loopback_port()
        replacement_config = replacement_home / "config.toml"
        replacement_config.write_text(
            webhook_config(replacement_token, replacement_port), encoding="utf-8"
        )
        replacement_config.chmod(0o600)
        result.reinstall_config_mode = oct(
            stat.S_IMODE(replacement_config.stat().st_mode)
        )
        reinstall_output = run(
            gateway_args(binary, "install", service_name)
            + [
                "--home",
                str(replacement_home),
                "--channels",
                "feishu",
                "--dir",
                str(replacement_workspace),
                "--start-now",
            ],
            env=env,
            timeout=45,
        )
        outputs.append(reinstall_output)
        result.reinstall_completed = "gateway service install completed" in reinstall_output
        reinstalled = wait_for_state(
            unit, env, active="active", different_pid=result.initial_pid
        )
        result.reinstall_pid = int(reinstalled.get("MainPID", "0") or "0")
        result.reinstall_pid_changed = result.reinstall_pid != result.initial_pid
        current_unit = unit_path.read_text(encoding="utf-8", errors="replace")
        result.reinstall_unit_updated = (
            str(replacement_home) in current_unit
            and str(replacement_workspace) in current_unit
        )
        result.reinstall_old_webhook_unreachable = webhook_unreachable(
            webhook_port, verification_token
        )
        result.reinstall_new_webhook_verified = verify_webhook(
            replacement_port, replacement_token
        )

        status_output = run(
            gateway_args(binary, "status", service_name), env=env, timeout=30
        )
        outputs.append(status_output)
        result.status_completed = "gateway service status completed" in status_output

        restart_output = run(
            gateway_args(binary, "restart", service_name), env=env, timeout=30
        )
        outputs.append(restart_output)
        restarted = wait_for_state(
            unit, env, active="active", different_pid=result.reinstall_pid
        )
        result.restart_active = restarted.get("ActiveState") == "active"
        result.restart_pid = int(restarted.get("MainPID", "0") or "0")
        result.restart_pid_changed = result.restart_pid != result.reinstall_pid
        result.restart_webhook_verified = verify_webhook(
            replacement_port, replacement_token
        )

        stop_output = run(
            gateway_args(binary, "stop", service_name), env=env, timeout=30
        )
        outputs.append(stop_output)
        stopped = wait_for_state(unit, env, active="inactive")
        result.stop_inactive = stopped.get("ActiveState") == "inactive"
        result.stop_webhook_unreachable = webhook_unreachable(
            replacement_port, replacement_token
        )

        start_output = run(
            gateway_args(binary, "start", service_name), env=env, timeout=30
        )
        outputs.append(start_output)
        started = wait_for_state(
            unit, env, active="active", different_pid=result.restart_pid
        )
        result.start_active = started.get("ActiveState") == "active"
        result.start_pid = int(started.get("MainPID", "0") or "0")
        result.start_pid_changed = result.start_pid != result.restart_pid
        result.start_webhook_verified = verify_webhook(
            replacement_port, replacement_token
        )

        journal = run(
            ["journalctl", "--user", "--unit", unit, "--no-pager", "--output=cat"],
            env=env,
        )
        outputs.append(journal)
        result.journal_has_start_banner = START_BANNER in journal

        assert_secret_absent(verification_token, outputs)
        assert_secret_absent(replacement_token, outputs)
        result.token_absent_from_outputs = True
        unit_text = unit_path.read_text(encoding="utf-8", errors="replace")
        assert_secret_absent(verification_token, [unit_text])
        assert_secret_absent(replacement_token, [unit_text])
        result.token_absent_from_unit = True

        uninstall_output = run(
            gateway_args(binary, "uninstall", service_name), env=env, timeout=30
        )
        outputs.append(uninstall_output)
        uninstalled = True
        result.uninstall_completed = "gateway service uninstall completed" in uninstall_output
        result.unit_removed = not unit_path.exists()
        after = systemd_state(unit, env)
        result.unit_load_state_after_uninstall = after.get("LoadState", "")
        if result.unit_load_state_after_uninstall != "not-found":
            raise RuntimeError(
                "uninstalled unit remains loaded: "
                f"{result.unit_load_state_after_uninstall!r}"
            )

        required = [
            result.config_written,
            result.config_mode == "0o600",
            result.unit_written,
            result.unit_mode == "0o644",
            result.unit_verified,
            result.unit_verify_output_empty,
            result.initial_active,
            result.initial_webhook_verified,
            result.reinstall_completed,
            result.reinstall_config_mode == "0o600",
            result.reinstall_pid_changed,
            result.reinstall_unit_updated,
            result.reinstall_old_webhook_unreachable,
            result.reinstall_new_webhook_verified,
            result.status_completed,
            result.restart_active,
            result.restart_pid_changed,
            result.restart_webhook_verified,
            result.stop_inactive,
            result.stop_webhook_unreachable,
            result.start_active,
            result.start_pid_changed,
            result.start_webhook_verified,
            result.journal_has_start_banner,
            result.token_absent_from_outputs,
            result.token_absent_from_unit,
            result.uninstall_completed,
            result.unit_removed,
        ]
        if not all(required):
            raise RuntimeError(f"gateway service lifecycle contract incomplete: {required}")
        result.status = "passed"
    except Exception as exc:
        result.errors.append(str(exc))
    finally:
        if not uninstalled and binary.is_file():
            run_result(
                gateway_args(binary, "uninstall", service_name),
                env=env,
                timeout=30,
            )
        if unit_path.exists():
            try:
                unit_path.unlink()
            except OSError as exc:
                result.errors.append(f"remove service unit: {exc}")
        run_result(["systemctl", "--user", "daemon-reload"], env=env)
        if not keep_temp:
            shutil.rmtree(temp_root, ignore_errors=True)
        result.temp_cleaned = not temp_root.exists()
        if not keep_temp and not result.temp_cleaned:
            result.errors.append(f"temporary root was not removed: {temp_root}")
        if result.errors:
            result.status = "failed"

    return result


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Smoke-test the Linux systemd user Gateway lifecycle."
    )
    parser.add_argument("--binary", type=Path, required=True)
    parser.add_argument("--out", type=Path)
    parser.add_argument("--keep-temp", action="store_true")
    args = parser.parse_args()
    result = run_smoke(args.binary, keep_temp=args.keep_temp)
    payload = json.dumps(asdict(result), indent=2, sort_keys=True) + "\n"
    if args.out:
        args.out.parent.mkdir(parents=True, exist_ok=True)
        args.out.write_text(payload, encoding="utf-8")
    print(payload, end="")
    return 0 if result.status == "passed" else 1


if __name__ == "__main__":
    raise SystemExit(main())
