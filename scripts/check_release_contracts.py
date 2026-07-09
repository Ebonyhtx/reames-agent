#!/usr/bin/env python3
"""Validate release safety, version, changelog, and signing contracts."""

from __future__ import annotations

from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]


def read(rel: str) -> str:
    return (ROOT / rel).read_text(encoding="utf-8")


def require(condition: bool, message: str, failures: list[str]) -> None:
    if not condition:
        failures.append(message)


def check_release_candidate_workflow(failures: list[str]) -> None:
    workflow = read(".github/workflows/release-candidate.yml")
    require("workflow_dispatch:" in workflow, "release candidate must be manually triggered.", failures)
    require("contents: read" in workflow, "release candidate workflow must not request content write permission.", failures)
    require("release --snapshot --clean" in workflow, "release candidate must run GoReleaser in snapshot mode.", failures)
    require("actions/upload-artifact" in workflow, "release candidate must upload artifacts, not publish releases.", failures)
    require("retention-days: 14" in workflow, "candidate artifacts should have short retention.", failures)
    forbidden = ["gh release create", "GITHUB_TOKEN:", "secrets.", "npm publish", "brew tap", "goreleaser release --clean"]
    for token in forbidden:
        require(token not in workflow, f"release candidate workflow must not contain {token!r}.", failures)


def check_goreleaser_contract(failures: list[str]) -> None:
    config = read(".goreleaser.yaml")
    require("project_name: reames-agent" in config, "GoReleaser project name must be reames-agent.", failures)
    require("-X main.version={{ .Tag }}" in config, "GoReleaser must inject main.version from the tag.", failures)
    require("goos: [darwin, linux, windows]" in config, "GoReleaser must build darwin/linux/windows.", failures)
    require("goarch: [amd64, arm64]" in config, "GoReleaser must build amd64/arm64.", failures)
    require("SHA256SUMS" in config, "GoReleaser must emit SHA256SUMS.", failures)


def check_release_docs(failures: list[str]) -> None:
    releasing = read("docs/RELEASING.md")
    for token in [
        "vMAJOR.MINOR.PATCH",
        "CHANGELOG.md",
        "Unreleased",
        "SHA256SUMS",
        "Sigstore/cosign",
        "OIDC keyless signing",
        "fail closed",
        "不会发布任何内容",
    ]:
        require(token in releasing, f"docs/RELEASING.md must document {token!r}.", failures)

    changelog = read("CHANGELOG.md")
    require("## Unreleased" in changelog, "CHANGELOG.md must contain an Unreleased section.", failures)
    require("## v0.1.0" in changelog, "CHANGELOG.md must retain the initial release section.", failures)


def main() -> int:
    failures: list[str] = []
    check_release_candidate_workflow(failures)
    check_goreleaser_contract(failures)
    check_release_docs(failures)

    if failures:
        print("Release contract check failed:")
        for failure in failures:
            print(f"- {failure}")
        return 1

    print("Release contract check passed.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
