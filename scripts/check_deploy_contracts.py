#!/usr/bin/env python3
"""Validate deployment-facing contracts that should not regress silently.

The checks are intentionally text-level and fast. They cover the cloud/server
deployment surface that is easy to break without touching Go code: Docker
healthchecks, systemd binding, compose environment forwarding, and user-facing
deployment commands.
"""

from __future__ import annotations

import pathlib
import sys


ROOT = pathlib.Path(__file__).resolve().parents[1]


def read(path: str) -> str:
    return (ROOT / path).read_text(encoding="utf-8")


def require(condition: bool, message: str, failures: list[str]) -> None:
    if not condition:
        failures.append(message)


def check() -> list[str]:
    failures: list[str] = []

    dockerfile = read("Dockerfile")
    require(
        'HEALTHCHECK --interval=30s --timeout=5s --retries=3 CMD ["/reames-agent", "serve", "--health-check"]'
        in dockerfile,
        "Dockerfile must use the real `reames-agent serve --health-check` exec-form healthcheck.",
        failures,
    )
    require("|| exit" not in dockerfile, "Dockerfile healthcheck must not rely on shell syntax in distroless.", failures)

    for path in ("docker-compose.yml", "docker-compose.windows.yml"):
        compose = read(path)
        require(
            '["CMD", "/reames-agent", "serve", "--health-check"]' in compose,
            f"{path} must keep the serve healthcheck command.",
            failures,
        )
        require(
            "REAMES_AGENT_SERVE_TOKEN=${REAMES_AGENT_SERVE_TOKEN:-}" in compose,
            f"{path} must forward REAMES_AGENT_SERVE_TOKEN for token_env deployments.",
            failures,
        )
        require("nousresearch/hermes" not in compose.lower(), f"{path} must not reference inherited Hermes images.", failures)
        require(".hermes" not in compose.lower(), f"{path} must not mount inherited Hermes data paths.", failures)
        require("HERMES_" not in compose, f"{path} must not contain inherited HERMES_* environment variables.", failures)

    windows_compose = read("docker-compose.windows.yml")
    require(
        '"127.0.0.1:8787:8787"' in windows_compose,
        "docker-compose.windows.yml should bind the browser surface to loopback by default.",
        failures,
    )

    unit = read("deploy/systemd/reames-agent.service")
    require(
        "EnvironmentFile=-/opt/reames-agent/.env" in unit,
        "systemd unit must load deployment secrets from /opt/reames-agent/.env.",
        failures,
    )
    require(
        "ExecStart=/opt/reames-agent/bin/reames-agent serve --addr 127.0.0.1:8787" in unit,
        "systemd unit must keep serve bound to loopback; expose through a reverse proxy.",
        failures,
    )
    require("${DEEPSEEK_API_KEY}" not in unit, "systemd unit must not use shell-style env interpolation.", failures)
    require("--addr 0.0.0.0:8787" not in unit, "systemd unit must not expose unauthenticated serve directly.", failures)

    deploy = read("docs/DEPLOY.md")
    require('reames-agent gateway run --home "$REAMES_AGENT_HOME" --channels feishu' in deploy, "docs/DEPLOY.md must document current gateway run command bound to REAMES_AGENT_HOME.", failures)
    require('reames-agent bot start --home "$REAMES_AGENT_HOME" --channels feishu' in deploy, "docs/DEPLOY.md must document legacy bot start compatibility bound to REAMES_AGENT_HOME.", failures)
    require('token_env = "REAMES_AGENT_SERVE_TOKEN"' in deploy, "docs/DEPLOY.md must document serve token_env.", failures)
    require("CLI + 独立 Gateway" in deploy, "docs/DEPLOY.md must lead with separate CLI and gateway deployment shape.", failures)
    require("tmux" in deploy and "reames-agent run" in deploy, "docs/DEPLOY.md must document SSH/tmux CLI usage.", failures)
    require("<Reames Agent home>/.env" in deploy, "docs/DEPLOY.md must document server user credential storage.", failures)
    require("Gateway preflight" in deploy and "reames-agent setup" in deploy, "docs/DEPLOY.md must document setup-to-Gateway preflight next steps.", failures)
    require("gateway setup --dry-run" in deploy and "不创建 synthetic `.env`" in deploy, "docs/DEPLOY.md must document the credential-free Gateway setup smoke.", failures)
    require("不是 CLI 或 gateway 的前置条件" in deploy, "docs/DEPLOY.md must state serve is optional after CLI/gateway setup.", failures)
    require("reames-agent gateway install --dry-run" in deploy, "docs/DEPLOY.md must document safe gateway service dry-run.", failures)
    require("reames-agent gateway install --start-now" in deploy, "docs/DEPLOY.md must document the Hermes-like gateway service lifecycle.", failures)
    require("reames-agent gateway doctor --deep --home" in deploy, "docs/DEPLOY.md must document Gateway diagnostics bound to REAMES_AGENT_HOME.", failures)
    require("--home \"$REAMES_AGENT_HOME\"" in deploy, "docs/DEPLOY.md must bind gateway services to the same REAMES_AGENT_HOME as CLI.", failures)
    require(
        "service definitions do not embed secret values" in deploy or "不会嵌入 secret 值" in deploy,
        "docs/DEPLOY.md must state gateway service definitions do not embed secret values.",
        failures,
    )
    require(
        "<Reames Agent home>/.env" in deploy,
        "docs/DEPLOY.md must keep the gateway credentials .env source visible.",
        failures,
    )
    require(
        "前台调试与后台常驻" in deploy and "gateway install/start/status" in deploy,
        "docs/DEPLOY.md must distinguish foreground gateway debugging from the current background service lifecycle.",
        failures,
    )
    require(
        "reames-agent backup create --offline" in deploy
        and "reames-agent backup verify" in deploy
        and "reames-agent backup restore --dry-run" in deploy,
        "docs/DEPLOY.md must document the offline backup, independent verification, and restore dry-run workflow.",
        failures,
    )
    require(
        "内嵌 manifest" in deploy and "只证明归档自洽" in deploy and "单独保存、可信传递" in deploy,
        "docs/DEPLOY.md must distinguish embedded backup self-consistency from an independently trusted digest.",
        failures,
    )
    require(
        "reames-agent upgrade --rollback" in deploy
        and "<executable>.previous" in deploy
        and "reames-agent gateway restart" in deploy,
        "docs/DEPLOY.md must document updater predecessor retention, rollback, and explicit Gateway restart.",
        failures,
    )
    require(
        "没有 durable crash journal" in deploy and "Windows 的实际保护还依赖目标目录 ACL" in deploy,
        "docs/DEPLOY.md must preserve backup/upgrade crash and Windows ACL limits.",
        failures,
    )

    env_example = read(".env.example")
    require("REAMES_AGENT_SERVE_TOKEN" in env_example, ".env.example must include the serve token env hint.", failures)
    require("REAMES_AGENT_LANG" in env_example, ".env.example must use the current language override env name.", failures)
    require("REAMES_LANG" not in env_example, ".env.example must not use the stale REAMES_LANG name.", failures)
    require("REASONIX_LANG" not in env_example, ".env.example must not use the stale REASONIX_LANG name.", failures)

    for path in ("scripts/install.sh", "scripts/install.ps1"):
        installer = read(path)
        require("reames-agent gateway install" in installer or "gateway install" in installer, f"{path} must support installing the gateway service.", failures)
        require("--home" in installer and "REAMES_AGENT_HOME" in installer, f"{path} must bind gateway services to the selected Reames Agent home.", failures)
        require("--dry-run" in installer or "DryRun" in installer, f"{path} must support safe dry-run planning.", failures)
        require("binary-source" in installer.lower() or "binarysource" in installer.lower(), f"{path} must expose an explicit source/release binary mode.", failures)
        require("SHA256SUMS" in installer, f"{path} must verify release artifacts with SHA256SUMS.", failures)
        require("reames-agent-" in installer and "releases/download" in installer, f"{path} must know the Reames GitHub release artifact shape.", failures)
        require(".env" in installer, f"{path} must keep the Gateway credential .env source visible.", failures)
        require("do not embed secret values" in installer, f"{path} must state service definitions do not embed secret values.", failures)
        require("NousResearch/hermes-agent" not in installer, f"{path} must not install inherited Hermes repositories.", failures)
        require("HERMES_HOME" not in installer, f"{path} must not use inherited HERMES_HOME.", failures)

    installer_tests = read("scripts/test_installers.py")
    require("InstallerDryRunTests" in installer_tests, "scripts/test_installers.py must cover installer dry-run contracts.", failures)
    require("Gateway credential source" in installer_tests, "installer tests must assert the Gateway credential source note.", failures)
    require("verify SHA256SUMS" in installer_tests, "installer tests must assert release checksum verification dry-runs.", failures)

    gateway_smoke = read("scripts/smoke_gateway_headless.py")
    require("gateway" in gateway_smoke and "setup" in gateway_smoke and "action: create" in gateway_smoke, "headless Gateway smoke must exercise gateway setup.", failures)
    require("write: skipped (dry-run)" in gateway_smoke and "dry_run_zero_write" in gateway_smoke, "headless Gateway smoke must prove setup dry-run leaves config untouched.", failures)
    require("action: unchanged" in gateway_smoke and "idempotent_bytes" in gateway_smoke, "headless Gateway smoke must prove setup is byte-for-byte idempotent.", failures)
    require("missing_secret_reported" in gateway_smoke and "no_credential_file" in gateway_smoke, "headless Gateway smoke must validate deployment readiness without synthetic secrets.", failures)
    require("gateway" in gateway_smoke and "doctor" in gateway_smoke and "--home" in gateway_smoke, "headless Gateway smoke must exercise gateway doctor --home.", failures)
    require("gateway" in gateway_smoke and "run" in gateway_smoke and "home_overrides_ambient_env" in gateway_smoke, "headless Gateway smoke must exercise foreground gateway run --home.", failures)
    require("install" in gateway_smoke and "--dry-run" in gateway_smoke, "headless Gateway smoke must exercise gateway install --dry-run.", failures)
    require("service definitions do not embed secret values" in gateway_smoke, "headless Gateway smoke must guard the no-secret service contract.", failures)
    require("cli_run" in gateway_smoke and "provider_bound_to_loopback" in gateway_smoke and "session_persisted" in gateway_smoke, "headless Gateway smoke must exercise a localhost-backed one-shot CLI turn and persist its session.", failures)
    require("feedback" in gateway_smoke and "duplicate_groups" in gateway_smoke and "draft_persisted" in gateway_smoke, "headless Gateway smoke must exercise the feedback submit-summary-draft lifecycle.", failures)
    require("sensitive_values_redacted" in gateway_smoke and "external_publish_attempted" in gateway_smoke, "headless Gateway smoke must prove feedback redaction and local-only draft generation.", failures)
    require("external_blocked" in gateway_smoke and "real provider API round trip" in gateway_smoke, "headless Gateway evidence must keep real external verification explicitly blocked.", failures)
    require("--out" in gateway_smoke and "json.dumps" in gateway_smoke, "headless Gateway smoke must support a JSON evidence report.", failures)

    return failures


def main() -> int:
    failures = check()
    if failures:
        print("Deployment contract check failed:", file=sys.stderr)
        for failure in failures:
            print(f"- {failure}", file=sys.stderr)
        return 1
    print("Deployment contract check passed.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
