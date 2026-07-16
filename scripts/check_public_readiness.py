#!/usr/bin/env python3
"""Validate the minimum gates for making the repository public.

This is intentionally a contract checker rather than a broad secret scanner.
It catches high-signal regressions in public-facing docs, ownership, and
release/deployment controls. GitHub secret scanning and history review are still
required after the repository is made public.
"""

from __future__ import annotations

import re
import subprocess
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]

LEGACY_ROOTS = {
    ".plans",
    ".signpath",
    "acp_adapter",
    "acp_registry",
    "agent",
    "apps",
    "assets",
    "cron",
    "datagen-config-examples",
    "docker",
    "gateway",
    "hooks",
    "infographic",
    "locales",
    "nix",
    "npm",
    "optional-mcps",
    "optional-skills",
    "packaging",
    "plans",
    "plugins",
    "providers",
    "reames_cli",
    "site",
    "skills",
    "tests",
    "tools",
    "tui_gateway",
    "ui-tui",
    "workers",
}

LEGACY_ROOT_FILES = {
    ".hadolint.yaml",
    ".mailmap",
    "INTEGRATION_REFERENCE.md",
    "MANIFEST.in",
    "README.ur-pk.md",
    "REAMES_AGENT.md",
    "batch_runner.py",
    "cli-config.yaml.example",
    "cli.py",
    "constraints-termux.txt",
    "flake.lock",
    "flake.nix",
    "hermes",
    "install.ps1",
    "mcp_serve.py",
    "mini_swe_runner.py",
    "model_tools.py",
    "package-lock.json",
    "package.json",
    "prod_test",
    "pyproject.toml",
    "reames",
    "reames-setup.py",
    "reames_bootstrap.py",
    "reames_constants.py",
    "reames_logging.py",
    "reames_state.py",
    "reames_time.py",
    "run_agent.py",
    "setup.py",
    "toolset_distributions.py",
    "toolsets.py",
    "trajectory_compressor.py",
    "utils.py",
    "uv.lock",
}

LEGACY_WORKFLOWS = {
    ".github/workflows/deploy-accounts-worker.yml",
    ".github/workflows/deploy-crash-worker.yml",
    ".github/workflows/deploy-forum-worker.yml",
    ".github/workflows/pages.yml",
}

ACTIVE_BRAND_ROOTS = (
    ".github/",
    "cmd/",
    "deploy/",
    "desktop/",
    "internal/",
    "scripts/",
)

ACTIVE_BRAND_EXEMPT = {
    "scripts/check_deploy_contracts.py",
    "scripts/check_public_readiness.py",
    "scripts/test_check_public_readiness.py",
}

INHERITED_RUNTIME_TOKENS = (
    "Hermes Agent",
    "HERMES_",
    ".hermes",
    "Nous Research",
    "NousResearch/hermes-agent",
    "hermes-agent.nousresearch",
)


def read(rel: str) -> str:
    return (ROOT / rel).read_text(encoding="utf-8")


def require(condition: bool, message: str, failures: list[str]) -> None:
    if not condition:
        failures.append(message)


def legacy_path_failure(rel: str) -> str | None:
    normalized = rel.replace("\\", "/").strip("/")
    top = normalized.split("/", 1)[0]
    if top in LEGACY_ROOTS:
        return f"{normalized} belongs to the removed legacy Hermes/Python tree."
    if normalized in LEGACY_ROOT_FILES:
        return f"{normalized} is removed legacy root metadata or an obsolete entry point."
    if normalized in LEGACY_WORKFLOWS:
        return f"{normalized} is an obsolete legacy deployment workflow."
    return None


def brand_failures_for_text(rel: str, text: str) -> list[str]:
    normalized = rel.replace("\\", "/")
    if normalized in ACTIVE_BRAND_EXEMPT or not normalized.startswith(ACTIVE_BRAND_ROOTS):
        return []

    failures: list[str] = []
    for token in INHERITED_RUNTIME_TOKENS:
        if token in text:
            failures.append(f"{normalized} contains inherited runtime brand token {token!r}.")

    if normalized.endswith(".go"):
        compatibility_only = re.sub(r"REASONIX(?:\.local)?\.md", "", text, flags=re.IGNORECASE)
        if re.search(r"reasonix", compatibility_only, flags=re.IGNORECASE):
            failures.append(
                f"{normalized} contains Reasonix branding outside the audited REASONIX.md compatibility filename."
            )
    return failures


def check_required_files(failures: list[str]) -> None:
    required = [
        "README.md",
        "LICENSE",
        "NOTICE.md",
        "third_party/go-tuf/LICENSE",
        "third_party/go-tuf/NOTICE",
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
    require("The Update Framework Authors" in notice, "NOTICE.md must preserve go-tuf attribution.", failures)
    require("third_party/go-tuf/LICENSE" in notice and "third_party/go-tuf/NOTICE" in notice, "NOTICE.md must point to preserved go-tuf legal files.", failures)
    go_tuf_license = read("third_party/go-tuf/LICENSE")
    go_tuf_notice = read("third_party/go-tuf/NOTICE")
    require("Apache License" in go_tuf_license and "Version 2.0" in go_tuf_license, "go-tuf Apache-2.0 license text must be preserved.", failures)
    require("Copyright 2024 The Update Framework Authors" in go_tuf_notice, "go-tuf NOTICE attribution must be preserved.", failures)


def check_release_and_deploy_controls(failures: list[str]) -> None:
    releasing = read("docs/RELEASING.md")
    require("不会发布任何内容" in releasing, "docs/RELEASING.md must state tag pushes do not publish.", failures)
    require("不向 npm、Homebrew、Cloudflare R2" in releasing, "docs/RELEASING.md must keep production publish targets disabled.", failures)

    for rel in sorted(LEGACY_WORKFLOWS):
        require(not (ROOT / rel).exists(), f"{rel} must stay removed with its unowned legacy service.", failures)
    for rel in ["site", "workers"]:
        require(not (ROOT / rel).exists(), f"{rel}/ must stay removed from the current product repository.", failures)

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


def check_workflow_action_runtimes(failures: list[str]) -> None:
    """Reject action majors that still embed the retired Node.js 20 runtime."""
    minimum_node24_major = {
        "actions/checkout": 5,
        "actions/setup-go": 6,
        "actions/setup-python": 6,
        "actions/setup-node": 6,
        "actions/upload-artifact": 6,
        "actions/github-script": 8,
        "pnpm/action-setup": 6,
    }
    approved_node24_pins = {
        "actions/checkout": {"9c091bb21b7c1c1d1991bb908d89e4e9dddfe3e0"},
        "actions/setup-go": {"b7ad1dad31e06c5925ef5d2fc7ad053ef454303e"},
        "actions/setup-python": {"ece7cb06caefa5fff74198d8649806c4678c61a1"},
        "actions/setup-node": {"820762786026740c76f36085b0efc47a31fe5020"},
        "actions/upload-artifact": {"043fb46d1a93c77aae656e7c1c64a875d1fc6a0a"},
        "actions/github-script": {"3a2844b7e9c422d3c10d287c895573f7108da1b3"},
        "pnpm/action-setup": {"0ebf47130e4866e96fce0953f49152a61190b271"},
    }
    pattern = re.compile(
        r"\buses:\s*['\"]?([A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+)(?:/[^@\s'\"]+)?@([^\s#'\"]+)"
    )
    workflow_root = ROOT / ".github" / "workflows"
    paths = sorted([*workflow_root.glob("*.yml"), *workflow_root.glob("*.yaml")])
    for path in paths:
        text = path.read_text(encoding="utf-8")
        for action, ref in pattern.findall(text):
            minimum = minimum_node24_major.get(action)
            if minimum is None:
                continue
            if re.fullmatch(r"[0-9a-fA-F]{40}", ref):
                require(
                    ref.lower() in approved_node24_pins[action],
                    f"{path.relative_to(ROOT).as_posix()} pins unaudited {action}@{ref}; add a verified Node 24 commit before using it.",
                    failures,
                )
                continue
            version = re.fullmatch(r"v(\d+)(?:\.\d+\.\d+)?", ref)
            if version is None:
                failures.append(
                    f"{path.relative_to(ROOT).as_posix()} uses unsupported {action}@{ref}; use an audited Node 24 major or commit."
                )
                continue
            major = int(version.group(1))
            require(
                major >= minimum,
                f"{path.relative_to(ROOT).as_posix()} uses {action}@v{major}, below the Node 24 baseline v{minimum}.",
                failures,
            )


def check_brand_env_regressions(failures: list[str]) -> None:
    public_docs = [
        "docs/BOT_GUIDE.md",
        "docs/BOT_GUIDE.zh-CN.md",
        "reames-agent.example.toml",
    ]
    for rel in public_docs:
        text = read(rel)
        require("REASONIX_BOT_CONTROL_TOKEN" not in text, f"{rel} must use REAMES_AGENT_BOT_CONTROL_TOKEN.", failures)


def check_legacy_tree_and_brand(failures: list[str]) -> None:
    try:
        tracked = subprocess.check_output(["git", "ls-files"], cwd=ROOT, text=True).splitlines()
    except (OSError, subprocess.CalledProcessError) as exc:
        failures.append(f"could not enumerate tracked files with git ls-files: {exc}")
        return

    for rel in tracked:
        failure = legacy_path_failure(rel)
        if failure:
            failures.append(failure)
            continue
        normalized = rel.replace("\\", "/")
        if not normalized.startswith(ACTIVE_BRAND_ROOTS):
            continue
        path = ROOT / normalized
        if not path.is_file():
            continue
        text = path.read_text(encoding="utf-8", errors="ignore")
        failures.extend(brand_failures_for_text(normalized, text))


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
        # Pre-commit validation can run after a tracked optional file was
        # removed from the worktree but before its deletion is staged.
        if not path.is_file():
            continue
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
        "scripts/test_check_public_readiness.py",
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
        check_workflow_action_runtimes(failures)
        check_brand_env_regressions(failures)
        check_legacy_tree_and_brand(failures)
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
