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
    require("reames-agent gateway run --channels feishu" in deploy, "docs/DEPLOY.md must document current gateway run command.", failures)
    require("reames-agent bot start --channels feishu" in deploy, "docs/DEPLOY.md must document legacy bot start compatibility.", failures)
    require('token_env = "REAMES_AGENT_SERVE_TOKEN"' in deploy, "docs/DEPLOY.md must document serve token_env.", failures)
    require("CLI + 独立 Gateway" in deploy, "docs/DEPLOY.md must lead with separate CLI and gateway deployment shape.", failures)
    require("tmux" in deploy and "reames-agent run" in deploy, "docs/DEPLOY.md must document SSH/tmux CLI usage.", failures)
    require("<Reames Agent home>/.env" in deploy, "docs/DEPLOY.md must document server user credential storage.", failures)
    require("不是 CLI 或 gateway 的前置条件" in deploy, "docs/DEPLOY.md must state serve is optional after CLI/gateway setup.", failures)
    require("reames-agent gateway install --start-now" in deploy, "docs/DEPLOY.md must document the planned Hermes-like gateway service lifecycle.", failures)
    require("当前实现的前台入口" in deploy, "docs/DEPLOY.md must distinguish current foreground bot entry from target gateway service.", failures)

    env_example = read(".env.example")
    require("REAMES_AGENT_SERVE_TOKEN" in env_example, ".env.example must include the serve token env hint.", failures)
    require("REAMES_AGENT_LANG" in env_example, ".env.example must use the current language override env name.", failures)
    require("REAMES_LANG" not in env_example, ".env.example must not use the stale REAMES_LANG name.", failures)
    require("REASONIX_LANG" not in env_example, ".env.example must not use the stale REASONIX_LANG name.", failures)

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
