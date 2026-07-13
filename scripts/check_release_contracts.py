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
    require("hdiutil attach" in workflow and "codesign --verify" in workflow and "lipo " in workflow and "-verify_arch x86_64 arm64" in workflow, "desktop candidate must mount and smoke the universal macOS dmg.", failures)
    require("scripts/smoke_desktop_candidate.py" in workflow, "desktop candidate must run the Linux/macOS native smoke script.", failures)
    require(workflow.count("--max-startup-seconds 10") == 2, "desktop candidate must enforce Linux and macOS readiness budgets.", failures)
    require("scripts/smoke_desktop_native.py" in workflow, "desktop candidate must run the Windows native smoke script.", failures)
    require("--observation-seconds 20" in workflow, "desktop candidate must leave enough time to observe stable Windows readiness.", failures)
    require("--max-startup-seconds 15" in workflow and "--max-warm-startup-seconds 6" in workflow, "desktop candidate must enforce installed cold and same-home warm startup budgets.", failures)
    require("scripts/smoke_desktop_interaction.py" in workflow, "desktop candidate must run the Windows screenshot-free interaction smoke.", failures)
    accessibility_command = "python scripts/smoke_desktop_accessibility.py"
    require(workflow.count(accessibility_command) == 1, "desktop candidate must run the Windows strict UIA accessibility smoke exactly once.", failures)
    accessibility_start = workflow.find(accessibility_command)
    accessibility_end = workflow.find("} finally {", accessibility_start)
    accessibility_block = workflow[accessibility_start:accessibility_end] if accessibility_start >= 0 and accessibility_end > accessibility_start else ""
    windows_step_start = workflow.rfind("- name: Smoke installed Windows candidate", 0, accessibility_start)
    windows_step = workflow[windows_step_start:accessibility_start] if windows_step_start >= 0 and accessibility_start >= 0 else ""
    require("if: runner.os == 'Windows'" in windows_step, "strict UIA accessibility smoke must stay in the Windows candidate step.", failures)
    for token in [
        "--artifact $installer",
        "--exe $exe",
        "--out artifacts/desktop-windows-accessibility-smoke.json",
        'if ($LASTEXITCODE -ne 0) { throw "installed Desktop accessibility smoke failed with exit code $LASTEXITCODE" }',
    ]:
        require(token in accessibility_block, f"Windows accessibility smoke must retain {token!r}.", failures)
    require("Start-Process -FilePath $installer" in workflow and "uninstall.exe" in workflow and "InstallLocation" in workflow, "desktop candidate must install, smoke, and uninstall the Windows NSIS package.", failures)
    require("artifacts/desktop-*-native-smoke.json" in workflow, "desktop candidate must upload native smoke evidence.", failures)
    require("artifacts/desktop-*-interaction-smoke.json" in workflow, "desktop candidate must upload Windows interaction evidence.", failures)
    require("artifacts/desktop-*-accessibility-smoke.json" in workflow, "desktop candidate must upload Windows accessibility evidence.", failures)
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

    upgrade = read("internal/cli/upgrade.go")
    require('ghOwner        = "Ebonyhtx"' in upgrade, "CLI upgrade must use the official Reames Agent GitHub owner.", failures)
    require('ghRepo         = "reames-agent"' in upgrade, "CLI upgrade must use the official Reames Agent repository.", failures)
    require('fmt.Sprintf("reames-agent-%s-%s%s"' in upgrade, "CLI upgrade asset names must match GoReleaser archives.", failures)
    require('return "reames-agent.exe"' in upgrade, "CLI upgrade must extract the GoReleaser Windows binary name.", failures)


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
        "scripts/smoke_desktop_interaction.py",
        "scripts/smoke_desktop_accessibility.py",
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
