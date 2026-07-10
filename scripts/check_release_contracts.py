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


def check_desktop_candidate_workflow(failures: list[str]) -> None:
    workflow = read(".github/workflows/desktop-candidate.yml")
    require("workflow_dispatch:" in workflow, "desktop candidate must be manually triggered.", failures)
    require("contents: read" in workflow, "desktop candidate workflow must not request content write permission.", failures)
    require("scripts/desktop-build.sh" in workflow, "desktop candidate must use the shared desktop build script.", failures)
    require("linux/amd64" in workflow, "desktop candidate must include a Linux target.", failures)
    require("windows/amd64" in workflow, "desktop candidate must include a Windows target.", failures)
    require("darwin/universal" in workflow, "desktop candidate must include a macOS universal target.", failures)
    require("actions/upload-artifact@v4" in workflow, "desktop candidate must upload artifacts, not publish releases.", failures)
    require("retention-days: 14" in workflow, "desktop candidate artifacts should have short retention.", failures)
    require("GITHUB_PATH" in workflow and "makensis -VERSION" in workflow, "desktop candidate must put NSIS/makensis on PATH before Wails packaging.", failures)
    require("sudo dpkg -i" in workflow and "xvfb-run" in workflow and "xdotool" in workflow, "desktop candidate must install and window-smoke the Linux deb.", failures)
    require("hdiutil attach" in workflow and "codesign --verify" in workflow and "lipo -verify_arch" in workflow, "desktop candidate must mount and smoke the universal macOS dmg.", failures)
    require("scripts/smoke_desktop_candidate.py" in workflow, "desktop candidate must run the Linux/macOS native smoke script.", failures)
    require("scripts/smoke_desktop_native.py" in workflow, "desktop candidate must run the Windows native smoke script.", failures)
    require("artifacts/desktop-*-native-smoke.json" in workflow, "desktop candidate must upload native smoke evidence.", failures)
    forbidden = [
        "gh release create",
        "GITHUB_TOKEN:",
        "secrets.",
        "npm publish",
        "brew tap",
        "aws s3",
        "wrangler deploy",
        "goreleaser release --clean",
    ]
    for token in forbidden:
        require(token not in workflow, f"desktop candidate workflow must not contain {token!r}.", failures)


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
        "Desktop candidate",
        "scripts/check_desktop_artifacts.py",
        "scripts/smoke_desktop_candidate.py",
        "scripts/smoke_desktop_native.py",
        "Sigstore/cosign",
        "OIDC keyless signing",
        "fail closed",
        "不会发布任何内容",
    ]:
        require(token in releasing, f"docs/RELEASING.md must document {token!r}.", failures)

    desktop_candidate = read("docs/audits/2026-07-09-desktop-candidate-governance.md")
    for token in [
        "Desktop candidate",
        "workflow_dispatch",
        "contents: read",
        "linux/amd64",
        "windows/amd64",
        "darwin/universal",
        "不创建 GitHub Release",
        "不读取 signing secrets",
        "scripts/check_desktop_artifacts.py",
    ]:
        require(token in desktop_candidate, f"desktop candidate audit must document {token!r}.", failures)

    changelog = read("CHANGELOG.md")
    require("## Unreleased" in changelog, "CHANGELOG.md must contain an Unreleased section.", failures)
    require("## v0.1.0" in changelog, "CHANGELOG.md must retain the initial release section.", failures)


def main() -> int:
    failures: list[str] = []
    check_release_candidate_workflow(failures)
    check_desktop_candidate_workflow(failures)
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
