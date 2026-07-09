#!/usr/bin/env python3
"""Validate the minimum gates for making the repository public.

This is intentionally a contract checker rather than a broad secret scanner.
It catches high-signal regressions in public-facing docs, ownership, and
release/deployment controls. GitHub secret scanning and history review are still
required after the repository is made public.
"""

from __future__ import annotations

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
        ".github/workflows/codeql.yml",
    ]
    for rel in required:
        require((ROOT / rel).is_file(), f"missing required public-facing file: {rel}", failures)


def check_readme(failures: list[str]) -> None:
    readme = read("README.md")
    require("reames-agent gateway run" in readme, "README.md must document current gateway run command.", failures)
    require("gateway start" not in readme, "README.md must not document obsolete gateway start command.", failures)
    require("sk-xxx" not in readme, "README.md must not use sk-xxx style API-key examples.", failures)
    require("Release Status" in readme, "README.md must describe pre-stable release status.", failures)
    require("docs/PUBLIC_READINESS.md" in readme, "README.md must link public readiness gates.", failures)
    require("NOTICE.md" in readme, "README.md must link attribution notices.", failures)


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


def main() -> int:
    failures: list[str] = []
    check_required_files(failures)
    if not failures:
        check_readme(failures)
        check_ownership_and_license(failures)
        check_release_and_deploy_controls(failures)
        check_codeql_workflow(failures)
        check_brand_env_regressions(failures)

    if failures:
        print("Public readiness check failed:")
        for failure in failures:
            print(f"- {failure}")
        return 1

    print("Public readiness check passed.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
