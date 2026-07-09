#!/usr/bin/env python3
"""Validate lightweight documentation contracts.

This intentionally checks high-signal repository hygiene rather than imposing a
full Markdown style guide. The goal is to keep handoff docs discoverable and to
catch broken local references before they land in CI.
"""

from __future__ import annotations

import re
import subprocess
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
DOCS = ROOT / "docs"
DOCS_INDEX = DOCS / "DOCS_INDEX.md"

MARKDOWN_LINK_RE = re.compile(r"(?<!!)\[[^\]]+\]\(([^)]+)\)")
LOCAL_AUDIT_RE = re.compile(r"\b(?:docs/)?audits/([A-Za-z0-9_.-]+\.md)\b")

# Old env var names that must not appear as current contracts in core docs.
# REASONIX.md as a file name is excluded — it is a valid convention file name
# in the memory system (see internal/memory/doc.go).
_OLD_ENV_PATTERNS = [
    (re.compile(r"\bREASONIX_HOME\b"), "REAMES_AGENT_HOME"),
    (re.compile(r"\bREASONIX_STATE_HOME\b"), "REAMES_AGENT_STATE_HOME"),
    (re.compile(r"\bREASONIX_CACHE_HOME\b"), "REAMES_AGENT_CACHE_HOME"),
    (re.compile(r"\bREASONIX_DISABLE_MOUSE\b"), "REAMES_AGENT_DISABLE_MOUSE"),
    (re.compile(r"\bREASONIX_PLUGIN_ROOT\b"), "REAMES_AGENT_PLUGIN_ROOT"),
    (re.compile(r"\bREASONIX_PLUGIN_NAME\b"), "REAMES_AGENT_PLUGIN_NAME"),
    (re.compile(r"\bREASONIX_PLUGIN_VERSION\b"), "REAMES_AGENT_PLUGIN_VERSION"),
    (re.compile(r"\bREASONIX_WORKSPACE_ROOT\b"), "REAMES_AGENT_WORKSPACE_ROOT"),
]

# Docs that serve as the current configuration/plugin contract.
# Legacy migration, audit, and upstream-reference docs are exempt.
_CURRENT_CONTRACT_GLOBS = [
    "docs/CONFIG_PATHS.md",
    "docs/CONFIG_PATHS.zh-CN.md",
    "docs/GUIDE.md",
    "docs/PLUGIN_PACKAGES.md",
    "docs/PLUGIN_PACKAGES.zh-CN.md",
]


def read_utf8(path: Path, failures: list[str]) -> str:
    try:
        return path.read_text(encoding="utf-8")
    except UnicodeDecodeError as exc:
        failures.append(f"{path.relative_to(ROOT).as_posix()} is not valid UTF-8: {exc}")
    except OSError as exc:
        failures.append(f"could not read {path.relative_to(ROOT).as_posix()}: {exc}")
    return ""


def tracked_files(pattern: str) -> list[Path]:
    try:
        out = subprocess.check_output(["git", "ls-files", pattern], cwd=ROOT, text=True)
    except (OSError, subprocess.CalledProcessError):
        return []
    return [ROOT / rel for rel in out.splitlines() if rel.strip()]


def is_external_link(target: str) -> bool:
    return (
        "://" in target
        or target.startswith("#")
        or target.startswith("mailto:")
        or target.startswith("tel:")
    )


def resolve_doc_link(source: Path, target: str) -> Path:
    clean = target.split("#", 1)[0].strip()
    if clean.startswith("<") and clean.endswith(">"):
        clean = clean[1:-1]
    return (source.parent / clean).resolve()


def check_all_docs_are_utf8(failures: list[str]) -> None:
    for path in sorted(tracked_files("docs/**/*.md")):
        text = read_utf8(path, failures)
        if "�" in text:
            failures.append(f"{path.relative_to(ROOT).as_posix()} contains the Unicode replacement character.")


def check_docs_index_links(failures: list[str]) -> None:
    index = read_utf8(DOCS_INDEX, failures)
    if not index:
        return
    for match in MARKDOWN_LINK_RE.finditer(index):
        target = match.group(1).strip()
        if is_external_link(target):
            continue
        path = resolve_doc_link(DOCS_INDEX, target)
        try:
            path.relative_to(ROOT)
        except ValueError:
            failures.append(f"docs/DOCS_INDEX.md link escapes the repository: {target}")
            continue
        if not path.exists():
            failures.append(f"docs/DOCS_INDEX.md links to missing file: {target}")


def check_audits_are_indexed(failures: list[str]) -> None:
    index = read_utf8(DOCS_INDEX, failures)
    if not index:
        return
    missing: list[str] = []
    for path in sorted(tracked_files("docs/audits/*.md")):
        rel = f"audits/{path.name}"
        if rel not in index:
            missing.append(rel)
    if missing:
        failures.append("docs/DOCS_INDEX.md must index every docs/audits/*.md file: " + ", ".join(missing))


def check_audit_references_exist(failures: list[str]) -> None:
    for source in [DOCS / "DEVELOPMENT_PLAN.md", DOCS_INDEX]:
        text = read_utf8(source, failures)
        for match in LOCAL_AUDIT_RE.finditer(text):
            path = DOCS / "audits" / match.group(1)
            if not path.exists():
                failures.append(
                    f"{source.relative_to(ROOT).as_posix()} references missing audit: audits/{match.group(1)}"
                )


def check_no_old_env_vars_in_current_contracts(failures: list[str]) -> None:
    """Ensure current-contract docs do not reference old REASONIX_* env var names."""
    for pattern, replacement in _OLD_ENV_PATTERNS:
        for glob_pattern in _CURRENT_CONTRACT_GLOBS:
            for path in sorted(tracked_files(glob_pattern)):
                text = read_utf8(path, failures)
                if not text:
                    continue
                for lineno, line in enumerate(text.splitlines(), 1):
                    if pattern.search(line):
                        failures.append(
                            f"{path.relative_to(ROOT).as_posix()}:{lineno}: "
                            f"old env var '{pattern.pattern[2:-2]}' should be '{replacement}'"
                        )


def main() -> int:
    failures: list[str] = []
    check_all_docs_are_utf8(failures)
    check_docs_index_links(failures)
    check_audits_are_indexed(failures)
    check_audit_references_exist(failures)
    check_no_old_env_vars_in_current_contracts(failures)

    if failures:
        print("Documentation contract check failed:")
        for failure in failures:
            print(f"- {failure}")
        return 1

    print("Documentation contract check passed.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
