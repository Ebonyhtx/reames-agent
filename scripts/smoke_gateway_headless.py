#!/usr/bin/env python3
"""Headless Gateway deployment smoke.

This is intentionally stronger than a text contract and weaker than a real IM
round trip: it runs the actual CLI binary against an isolated Reames Agent home,
applies `gateway setup`, checks `gateway doctor --home`, and renders a
service-manager dry-run plan. It does not start a background service and never
requires real provider or bot secrets.
"""

from __future__ import annotations

import argparse
import json
import os
import platform
import subprocess
import tempfile
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SMOKE_APP_ID = "cli-smoke-feishu-app"
SMOKE_SECRET_ENV = "FEISHU_BOT_APP_SECRET"


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
    return {
        "status": "passed",
        "os": platform.system().lower(),
        "binary": str(binary),
        "home": str(home),
        "workspace": str(workspace),
        "setup": setup_contracts,
        "doctor": doctor_contracts,
        "service_plan": plan_contracts,
        "foreground_run": foreground_contracts,
    }


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
