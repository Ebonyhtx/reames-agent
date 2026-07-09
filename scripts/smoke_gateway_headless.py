#!/usr/bin/env python3
"""Headless Gateway deployment smoke.

This is intentionally stronger than a text contract and weaker than a real IM
round trip: it runs the actual CLI binary against an isolated Reames Agent home,
checks `gateway doctor --home`, and renders a service-manager dry-run plan. It
does not start a background service and never requires real provider or bot
secrets.
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
SMOKE_SECRET = "smoke-secret-never-print"


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


def write_smoke_home(home: Path) -> Path:
    workspace = home / "workspace"
    workspace.mkdir(parents=True, exist_ok=True)
    (home / "config.toml").write_text(
        """
language = "en"
default_model = "deepseek-flash"

[bot]
enabled = true

[bot.allowlist]
enabled = true
feishu_users = ["ou-smoke-user"]

[bot.feishu]
enabled = true
app_id = "cli-smoke-feishu"
app_secret_env = "FEISHU_BOT_APP_SECRET"
mode = "webhook"
""".lstrip(),
        encoding="utf-8",
    )
    (home / ".env").write_text(f"FEISHU_BOT_APP_SECRET={SMOKE_SECRET}\n", encoding="utf-8")
    return workspace


def write_foreground_probe_home(home: Path, *, enabled: bool) -> None:
    home.mkdir(parents=True, exist_ok=True)
    (home / "config.toml").write_text(
        f"""
language = "en"
default_model = "deepseek-flash"

[bot]
enabled = {str(enabled).lower()}

[bot.allowlist]
allow_all = true
""".lstrip(),
        encoding="utf-8",
    )


def assert_contains(text: str, needle: str) -> None:
    if needle not in text:
        raise AssertionError(f"output missing {needle!r}:\n{text}")


def assert_not_contains(text: str, needle: str) -> None:
    if needle in text:
        raise AssertionError(f"output leaked {needle!r}:\n{text}")


def smoke(binary: Path, home: Path) -> dict[str, object]:
    workspace = write_smoke_home(home)
    env = os.environ.copy()
    env.pop("REAMES_AGENT_HOME", None)
    env["REAMES_AGENT_CREDENTIALS_STORE"] = "file"

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
    if checks.get("bot.credentials", {}).get("status") != "ok":
        raise AssertionError(f"bot.credentials was not ok:\n{doctor}")
    if checks.get("bot.feishu.app_secret", {}).get("status") != "ok":
        raise AssertionError(f"bot.feishu.app_secret was not ok:\n{doctor}")
    assert_not_contains(doctor, SMOKE_SECRET)
    doctor_contracts = {
        "home_bound": checks.get("bot.home", {}).get("detail") == str(home),
        "credentials_ok": checks.get("bot.credentials", {}).get("status") == "ok",
        "feishu_secret_resolved": checks.get("bot.feishu.app_secret", {}).get("status") == "ok",
        "secret_redacted": SMOKE_SECRET not in doctor,
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
    assert_not_contains(plan, SMOKE_SECRET)
    plan_contracts = {
        "renders_service_plan": "gateway service plan:" in plan,
        "pins_reames_agent_home": "REAMES_AGENT_HOME" in plan and str(home) in plan,
        "documents_credentials_env": ".env" in plan,
        "documents_no_secret_embedding": "service definitions do not embed secret values" in plan,
        "secret_redacted": SMOKE_SECRET not in plan,
    }
    ambient_home = home.parent / "ambient-home"
    selected_home = home.parent / "selected-foreground-home"
    write_foreground_probe_home(ambient_home, enabled=True)
    write_foreground_probe_home(selected_home, enabled=False)
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
    assert_not_contains(foreground, SMOKE_SECRET)
    foreground_contracts = {
        "home_overrides_ambient_env": foreground_code == 1 and "gateway is not enabled" in foreground,
        "secret_redacted": SMOKE_SECRET not in foreground,
    }
    return {
        "status": "passed",
        "os": platform.system().lower(),
        "binary": str(binary),
        "home": str(home),
        "workspace": str(workspace),
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
