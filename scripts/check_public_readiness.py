#!/usr/bin/env python3
"""Validate the minimum gates for making the repository public.

This is intentionally a contract checker rather than a broad secret scanner.
It catches high-signal regressions in public-facing docs, ownership, and
release/deployment controls. GitHub secret scanning and history review are still
required after the repository is made public.
"""

from __future__ import annotations

import subprocess
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]


def read(rel: str) -> str:
    return (ROOT / rel).read_text(encoding="utf-8")


def require(condition: bool, message: str, failures: list[str]) -> None:
    if not condition:
        failures.append(message)


def check_required_files(failures: list[str]) -> None:
    required = [
        "README.md",
        "LICENSE",
        "NOTICE.md",
        "SECURITY.md",
        "CONTRIBUTING.md",
        "AGENTS.md",
        "docs/PROJECT.md",
        "docs/DEVELOPMENT_PLAN.md",
        "docs/DOCS_INDEX.md",
        "docs/REFERENCE_GOVERNANCE.md",
        "docs/RELEASING.md",
        "docs/PUBLIC_READINESS.md",
        "scripts/install.sh",
        "scripts/install.ps1",
        "scripts/install.cmd",
        ".github/workflows/codeql.yml",
    ]
    for rel in required:
        require((ROOT / rel).is_file(), f"missing required public-facing file: {rel}", failures)


def check_readme(failures: list[str]) -> None:
    readme = read("README.md")
    readme_zh = read("README.zh-CN.md")
    require("reames-agent gateway run" in readme, "README.md must document current gateway run command.", failures)
    require("reames-agent gateway run" in readme_zh, "README.zh-CN.md must document current gateway run command.", failures)
    require("gateway start" not in readme, "README.md must not document obsolete gateway start command.", failures)
    require("gateway start" not in readme_zh, "README.zh-CN.md must not document obsolete gateway start command.", failures)
    require("sk-xxx" not in readme, "README.md must not use sk-xxx style API-key examples.", failures)
    require("sk-xxx" not in readme_zh, "README.zh-CN.md must not use sk-xxx style API-key examples.", failures)
    require("Release Status" in readme, "README.md must describe pre-stable release status.", failures)
    require("docs/PUBLIC_READINESS.md" in readme, "README.md must link public readiness gates.", failures)
    require("NOTICE.md" in readme, "README.md must link attribution notices.", failures)
    require("9 reference projects" in readme, "README.md must match the 9-reference-project governance count.", failures)
    require("9 个参考项目" in readme_zh, "README.zh-CN.md must match the 9-reference-project governance count.", failures)
    require("npm i -g reames-agent" not in readme, "README.md must not imply npm stable distribution is enabled.", failures)
    require("brew install" not in readme, "README.md must not imply Homebrew stable distribution is enabled.", failures)


def check_ownership_and_license(failures: list[str]) -> None:
    codeowners = read(".github/CODEOWNERS")
    require("@Ebonyhtx" in codeowners, ".github/CODEOWNERS must request current maintainer review.", failures)
    require("@esengine" not in codeowners, ".github/CODEOWNERS must not request upstream maintainer review.", failures)
    require("@SivanCola" not in codeowners, ".github/CODEOWNERS must not request inherited maintainer review.", failures)

    license_text = read("LICENSE")
    require("Reames Agent Contributors" in license_text, "LICENSE must include Reames Agent copyright.", failures)
    require("Reasonix Contributors" in license_text, "LICENSE must preserve Reasonix copyright.", failures)

    notice = read("NOTICE.md")
    require("DeepSeek Reasonix" in notice, "NOTICE.md must attribute DeepSeek Reasonix.", failures)
    require("https://github.com/esengine/DeepSeek-Reasonix" in notice, "NOTICE.md must link the upstream repository.", failures)


def check_release_and_deploy_controls(failures: list[str]) -> None:
    releasing = read("docs/RELEASING.md")
    require("不会发布任何内容" in releasing, "docs/RELEASING.md must state tag pushes do not publish.", failures)
    require("不向 npm、Homebrew、Cloudflare R2" in releasing, "docs/RELEASING.md must keep production publish targets disabled.", failures)

    for rel in [
        ".github/workflows/deploy-accounts-worker.yml",
        ".github/workflows/deploy-crash-worker.yml",
        ".github/workflows/deploy-forum-worker.yml",
        ".github/workflows/pages.yml",
    ]:
        workflow = read(rel)
        require("workflow_dispatch" not in workflow, f"{rel} must not expose manual production deployment.", failures)
        require("branches: [main-v2]" in workflow, f"{rel} must not auto-deploy from the current main branch.", failures)

    deploy = read("docs/DEPLOY.md")
    require("sk-xxx" not in deploy, "docs/DEPLOY.md must not use sk-xxx style API-key examples.", failures)
    require("REAMES_AGENT_SERVE_TOKEN" in deploy, "docs/DEPLOY.md must document serve token env.", failures)


def check_codeql_workflow(failures: list[str]) -> None:
    codeql = read(".github/workflows/codeql.yml")
    require("github/codeql-action/init@v4" in codeql, "CodeQL workflow must initialize CodeQL.", failures)
    require("github/codeql-action/analyze@v4" in codeql, "CodeQL workflow must upload analysis.", failures)
    require("security-events: write" in codeql, "CodeQL workflow must grant security-events write to the analyze job.", failures)
    for language in ["go", "javascript-typescript", "actions"]:
        require(language in codeql, f"CodeQL workflow must analyze {language}.", failures)
    require("github/codeql-action/autobuild@v4" in codeql, "CodeQL workflow must autobuild compiled Go analysis.", failures)


def check_brand_env_regressions(failures: list[str]) -> None:
    public_docs = [
        "docs/BOT_GUIDE.md",
        "docs/BOT_GUIDE.zh-CN.md",
        "reames-agent.example.toml",
    ]
    for rel in public_docs:
        text = read(rel)
        require("REASONIX_BOT_CONTROL_TOKEN" not in text, f"{rel} must use REAMES_AGENT_BOT_CONTROL_TOKEN.", failures)


def check_root_package_metadata(failures: list[str]) -> None:
    package = read("package.json")
    package_lock = read("package-lock.json")
    for rel, text in [("package.json", package), ("package-lock.json", package_lock)]:
        require('"name": "reames-agent"' in text, f"{rel} root package metadata must use the Reames package name.", failures)
        require("NousResearch/Hermes-Agent" not in text, f"{rel} root package metadata must not link the inherited Hermes repository.", failures)
    require("python run_agent.py" not in package, "package.json postinstall must not advertise the inherited Python runtime.", failures)
    require("scripts/install.sh" in package, "package.json postinstall should point users to the current Reames installer surface.", failures)


def check_tracked_artifacts(failures: list[str]) -> None:
    try:
        tracked = subprocess.check_output(["git", "ls-files"], cwd=ROOT, text=True).splitlines()
    except (OSError, subprocess.CalledProcessError) as exc:
        failures.append(f"could not enumerate tracked files with git ls-files: {exc}")
        return

    forbidden_exact = {
        "reames-agent",
        "reames-agent.exe",
        "bin/reames-agent",
        "bin/reames-agent.exe",
    }
    forbidden_suffixes = (
        ".exe",
        ".dll",
        ".dylib",
        ".so",
        ".test",
        ".coverprofile",
    )
    allowed_suffixes = (
        ".md",
        ".go",
        ".js",
        ".mjs",
        ".ts",
        ".tsx",
        ".json",
        ".yaml",
        ".yml",
        ".toml",
        ".svg",
    )
    for rel in tracked:
        normalized = rel.replace("\\", "/")
        lower = normalized.lower()
        if normalized in forbidden_exact:
            failures.append(f"{normalized} must not be tracked; build binaries stay local or in release artifacts.")
            continue
        if lower.startswith(("bin/", "dist/", "stage/")):
            failures.append(f"{normalized} must not be tracked from generated artifact directories.")
            continue
        if lower.endswith(forbidden_suffixes) and not lower.endswith(allowed_suffixes):
            failures.append(f"{normalized} looks like a generated binary/test artifact and must not be tracked.")


def check_telemetry_boundaries(failures: list[str]) -> None:
    crash_app = read("desktop/crash_app.go")
    metrics_app = read("desktop/metrics_app.go")
    require('var crashEndpoint = ""' in crash_app, "desktop crash reporting must default to no endpoint.", failures)
    require('var metricsEndpoint = ""' in metrics_app, "desktop metrics upload must default to no endpoint.", failures)
    require("crash reporting is unavailable in this build" in crash_app, "desktop crash reporting must fail closed without an owned endpoint.", failures)
    require("Gated on config desktop.metrics and a repository-owned endpoint." in metrics_app, "desktop metrics must document the owned-endpoint gate.", failures)

    forbidden_tokens = [
        "SENTRY_DSN",
        "POSTHOG_KEY",
        "AMPLITUDE_API_KEY",
        "DATADOG_API_KEY",
        "CRASH_ENDPOINT",
        "TELEMETRY_ENDPOINT",
        "METRICS_ENDPOINT",
        "crashEndpoint = \"http",
        "metricsEndpoint = \"http",
    ]
    scan_roots = (
        ".github/workflows",
        "desktop",
        "docs",
        "deploy",
        "scripts",
        "README.md",
        "README.zh-CN.md",
    )
    allowed = {
        "scripts/check_public_readiness.py",
        "docs/PUBLIC_READINESS.md",
        "docs/CLOUD_AGENT_PLAN.md",
        "docs/audits/2026-07-09-telemetry-feedback-boundary.md",
    }
    try:
        tracked = subprocess.check_output(["git", "ls-files"], cwd=ROOT, text=True).splitlines()
    except (OSError, subprocess.CalledProcessError) as exc:
        failures.append(f"could not enumerate tracked files with git ls-files: {exc}")
        return
    for rel in tracked:
        normalized = rel.replace("\\", "/")
        if normalized in allowed or not normalized.startswith(scan_roots):
            continue
        path = ROOT / normalized
        text = path.read_text(encoding="utf-8", errors="ignore")
        for token in forbidden_tokens:
            require(token not in text, f"{normalized} must not configure telemetry/crash token {token!r}.", failures)


def check_installers(failures: list[str]) -> None:
    for rel in ["scripts/install.sh", "scripts/install.ps1", "scripts/install.cmd"]:
        text = read(rel)
        require("Reames Agent" in text, f"{rel} must be a Reames installer.", failures)
        require("Ebonyhtx/reames-agent" in text, f"{rel} must install from the official Reames repository.", failures)
        require("NousResearch/hermes-agent" not in text, f"{rel} must not install inherited Hermes repositories.", failures)
        require("HERMES_HOME" not in text, f"{rel} must not use inherited HERMES_HOME.", failures)
        require(".hermes" not in text, f"{rel} must not write inherited .hermes data paths.", failures)


def check_script_surface(failures: list[str]) -> None:
    removed_legacy_entries = [
        "scripts/LIVETEST_README.md",
        "scripts/analyze_livetest.py",
        "scripts/benchmark_browser_eval.py",
        "scripts/build_model_catalog.py",
        "scripts/build_skills_index.py",
        "scripts/check-windows-footguns.py",
        "scripts/contributor_audit.py",
        "scripts/discord-voice-doctor.py",
        "scripts/docker_config_migrate.py",
        "scripts/install_psutil_android.py",
        "scripts/keystroke_diagnostic.py",
        "scripts/lib/node-bootstrap.sh",
        "scripts/profile-tui.py",
        "scripts/release.py",
        "scripts/run_tests.sh",
        "scripts/run_tests_parallel.py",
        "scripts/sample_and_compress.py",
        "scripts/setup_open_webui.sh",
        "scripts/tool_search_livetest.py",
        "scripts/whatsapp-bridge",
    ]
    for rel in removed_legacy_entries:
        path = ROOT / rel
        if path.is_dir():
            has_files = any(child.is_file() for child in path.rglob("*"))
            require(not has_files, f"{rel} must not return to the public script surface.", failures)
        else:
            require(not path.exists(), f"{rel} must not return to the public script surface.", failures)

    allowed_legacy_mentions = {
        "scripts/check_public_readiness.py",
        "scripts/check_deploy_contracts.py",
    }
    forbidden_tokens = [
        "HERMES_HOME",
        ".hermes",
        "hermes_cli",
        "reames_cli",
        "NousResearch/hermes-agent",
        "hermes-agent.nousresearch",
        "Hermes Agent",
    ]
    for path in (ROOT / "scripts").rglob("*"):
        if not path.is_file() or "__pycache__" in path.parts:
            continue
        rel = path.relative_to(ROOT).as_posix()
        if rel in allowed_legacy_mentions:
            continue
        text = path.read_text(encoding="utf-8", errors="ignore")
        for token in forbidden_tokens:
            require(token not in text, f"{rel} must not contain inherited script token {token!r}.", failures)


def main() -> int:
    failures: list[str] = []
    check_required_files(failures)
    if not failures:
        check_readme(failures)
        check_ownership_and_license(failures)
        check_release_and_deploy_controls(failures)
        check_codeql_workflow(failures)
        check_brand_env_regressions(failures)
        check_root_package_metadata(failures)
        check_tracked_artifacts(failures)
        check_telemetry_boundaries(failures)
        check_installers(failures)
        check_script_surface(failures)

    if failures:
        print("Public readiness check failed:")
        for failure in failures:
            print(f"- {failure}")
        return 1

    print("Public readiness check passed.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
