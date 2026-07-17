#!/usr/bin/env python3
"""Build a deterministic code-level inventory for a Reasonix generation range.

The output is evidence, not an automatic adoption decision: every non-merge
commit is enumerated with its touched paths and review areas, while exact
lowercase conventional ``fix(...)``, ``fix:``, ``perf(...)`` and ``perf:``
subjects form the bug-fix parity population used by the manual audit.
"""

from __future__ import annotations

import argparse
import json
import re
import subprocess
from collections import Counter
from pathlib import Path
from typing import Iterable


FIX_PERF_RE = re.compile(r"^(?:fix|perf)(?:\([^()\r\n]+\))?:")

AREA_PATTERNS: tuple[tuple[str, tuple[str, ...]], ...] = (
    ("build-release", (".github/", "scripts/", "release", "goreleaser", "package.json", "site/", "docs/changelog")),
    ("recovery-update", ("guard", "recovery", "repair", "updater", "upgrade", "checkpoint", "safe_mode", "safe-mode")),
    ("permissions-sandbox", ("permission", "sandbox", "processpolicy", "trust", "crypto", "credential", "secret", "security")),
    ("provider-cache", ("provider", "cache", "billing", "openai", "anthropic", "deepseek", "mimo", "schema")),
    ("tools-mcp-plugins", ("tool/", "tools/", "mcp", "plugin", "hook", "skill", "lsp", "codegraph")),
    ("session-memory-compaction", ("session", "memory", "retrieval", "compaction", "compact", "history", "transcript")),
    ("goal-plan-work-modes", ("goal", "planmode", "plan_mode", "plan-mode", "delivery", "work_mode", "work-mode", "token_profile")),
    ("desktop-ui-accessibility", ("desktop/", "frontend/", "accessibility", "a11y", "theme", "composer", "tab")),
    ("cli-acp", ("internal/cli", "internal/acp", "cmd/", "tui", "terminal", "renderer")),
    ("serve-gateway", ("internal/serve", "internal/bot", "gateway", "feishu", "telegram", "weixin", "wechat", "qq")),
    ("controller-transports", ("internal/control", "controller", "eventwire", "event/", "transport", "protocol")),
    ("agent-runtime", ("internal/agent", "internal/boot", "agent", "subagent", "evidence", "todo", "board", "runtime")),
)


def git(repo: Path, *args: str) -> str:
    proc = subprocess.run(
        ["git", "-C", str(repo), *args],
        check=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
        encoding="utf-8",
        errors="strict",
    )
    return proc.stdout


def classify_areas(subject: str, files: Iterable[str]) -> list[str]:
    haystack = "\n".join([subject.lower(), *(path.lower().replace("\\", "/") for path in files)])
    areas = [area for area, needles in AREA_PATTERNS if any(needle in haystack for needle in needles)]
    return areas or ["agent-runtime"]


def parse_commit_log(raw: str) -> list[dict[str, object]]:
    commits: list[dict[str, object]] = []
    current: dict[str, object] | None = None
    for line in raw.splitlines():
        if line.startswith("@@COMMIT@@"):
            if current is not None:
                current["areas"] = classify_areas(str(current["subject"]), current["files"])
                commits.append(current)
            sha, authored_at, subject = line.removeprefix("@@COMMIT@@").split("\t", 2)
            current = {
                "sha": sha,
                "authored_at": authored_at,
                "subject": subject,
                "fix_perf": bool(FIX_PERF_RE.match(subject)),
                "files": [],
            }
        elif current is not None and line.strip():
            current["files"].append(line.strip().replace("\\", "/"))
    if current is not None:
        current["areas"] = classify_areas(str(current["subject"]), current["files"])
        commits.append(current)
    return commits


def commit_inventory(repo: Path, baseline: str, reviewed: str) -> list[dict[str, object]]:
    raw = git(
        repo,
        "log",
        "--reverse",
        "--no-merges",
        "--format=@@COMMIT@@%H%x09%aI%x09%s",
        "--name-only",
        f"{baseline}..{reviewed}",
    )
    return parse_commit_log(raw)


def unmerged_remote_refs(repo: Path, reviewed: str) -> list[dict[str, object]]:
    refs: list[dict[str, object]] = []
    raw = git(repo, "for-each-ref", "--format=%(refname:short)%09%(objectname)%09%(subject)", "refs/remotes/origin")
    for line in raw.splitlines():
        if not line.strip():
            continue
        name, sha, subject = line.split("\t", 2)
        if name in {"origin/HEAD", "origin/main-v2"}:
            continue
        merged = subprocess.run(
            ["git", "-C", str(repo), "merge-base", "--is-ancestor", sha, reviewed],
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        ).returncode == 0
        if merged:
            continue
        ahead = int(git(repo, "rev-list", "--count", f"{reviewed}..{sha}").strip() or "0")
        refs.append({"ref": name, "sha": sha, "ahead": ahead, "subject": subject})
    return sorted(refs, key=lambda item: str(item["ref"]))


def tags_at_or_before(repo: Path, reviewed: str, limit: int = 30) -> list[dict[str, str]]:
    raw = git(repo, "tag", "--merged", reviewed, "--sort=-creatordate", "--format=%(refname:strip=2)%09%(objectname)")
    tags = []
    for line in raw.splitlines()[:limit]:
        name, sha = line.split("\t", 1)
        tags.append({"name": name, "sha": sha})
    return tags


def build_inventory(repo: Path, baseline: str, reviewed: str) -> dict[str, object]:
    baseline = git(repo, "rev-parse", baseline).strip()
    reviewed = git(repo, "rev-parse", reviewed).strip()
    commits = commit_inventory(repo, baseline, reviewed)
    area_counts: Counter[str] = Counter()
    fix_area_counts: Counter[str] = Counter()
    for commit in commits:
        for area in commit["areas"]:
            area_counts[str(area)] += 1
            if commit["fix_perf"]:
                fix_area_counts[str(area)] += 1
    shortstat = git(repo, "diff", "--shortstat", baseline, reviewed).strip()
    total_commits = int(git(repo, "rev-list", "--count", f"{baseline}..{reviewed}").strip())
    reviewed_at = git(repo, "show", "-s", "--format=%cI", reviewed).strip()
    return {
        "version": 1,
        "upstream": "reasonix",
        "baseline": baseline,
        "reviewed": reviewed,
        "reviewed_at": reviewed_at,
        "counts": {
            "all_commits": total_commits,
            "non_merge_commits": len(commits),
            "fix_perf_commits": sum(1 for commit in commits if commit["fix_perf"]),
            "diff_shortstat": shortstat,
        },
        "area_counts": dict(sorted(area_counts.items())),
        "fix_perf_area_counts": dict(sorted(fix_area_counts.items())),
        "unmerged_remote_refs": unmerged_remote_refs(repo, reviewed),
        "recent_merged_tags": tags_at_or_before(repo, reviewed),
        "commits": commits,
    }


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--repo", required=True, type=Path)
    parser.add_argument("--baseline", required=True)
    parser.add_argument("--reviewed", required=True)
    parser.add_argument("--out", required=True, type=Path)
    args = parser.parse_args()
    payload = build_inventory(args.repo.resolve(), args.baseline, args.reviewed)
    args.out.parent.mkdir(parents=True, exist_ok=True)
    args.out.write_text(json.dumps(payload, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps(payload["counts"], ensure_ascii=False))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
