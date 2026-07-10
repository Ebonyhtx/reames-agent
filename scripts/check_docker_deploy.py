#!/usr/bin/env python3
"""Docker deployment contract verifier.

Validates Dockerfile, docker-compose, and container deployment
readiness. Actual container registry publishing and cloud deployment
require real credentials (external-blocked).
"""

from __future__ import annotations

import platform
import subprocess
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]


def read(path: str) -> str:
    return (ROOT / path).read_text(encoding="utf-8")


def check_dockerfile(failures: list[str]) -> None:
    df = ROOT / "Dockerfile"
    if not df.exists():
        failures.append("Dockerfile is missing")
        return

    content = df.read_text(encoding="utf-8")
    checks = {
        "FROM": "must declare base image",
        "COPY": "must copy application binary",
        "ENTRYPOINT": "must have entrypoint or CMD",
        "CMD": "must have entrypoint or CMD",
    }
    for token, msg in checks.items():
        if token == "CMD" and "ENTRYPOINT" in content:
            continue  # ENTRYPOINT without CMD is fine
        if token not in content:
            failures.append(f"Dockerfile {msg}: missing '{token}'")

    # Check for multi-stage build (smaller final image).
    if content.count("FROM") >= 2:
        print("  Dockerfile: multi-stage build detected (good for SBOM)")

    # Check that binary is CGO_ENABLED=0 compiled.
    if "CGO_ENABLED=0" not in content and "CGO_ENABLED" not in content:
        failures.append("Dockerfile should set CGO_ENABLED=0 for static binary")


def check_docker_compose(failures: list[str]) -> None:
    for name in ["docker-compose.yml", "docker-compose.yaml"]:
        dc = ROOT / name
        if dc.exists():
            content = dc.read_text(encoding="utf-8")
            if "reames-agent" not in content:
                failures.append(f"{name} should reference reames-agent service")
            if "REAMES_AGENT_HOME" not in content:
                failures.append(f"{name} should set REAMES_AGENT_HOME volume")
            return
    # docker-compose is optional.
    print("  docker-compose: not found (optional)")


def check_container_healthcheck(failures: list[str]) -> None:
    df = ROOT / "Dockerfile"
    if not df.exists():
        return
    content = df.read_text(encoding="utf-8")
    if "HEALTHCHECK" not in content:
        failures.append("Dockerfile should include HEALTHCHECK instruction")
    if "serve" not in content and "gateway" not in content:
        failures.append("Dockerfile should document which service mode (serve/gateway) is used")


def main() -> int:
    failures: list[str] = []
    print("Docker deployment contract check:")
    check_dockerfile(failures)
    check_docker_compose(failures)
    check_container_healthcheck(failures)

    if failures:
        print("\nFAILED:")
        for f in failures:
            print(f"  - {f}")
        return 1
    print("PASSED")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
