#!/usr/bin/env python3
"""Check reference upstream repositories and write advisory reports.

The script is intentionally advisory-only: it never edits source code and never
merges upstream changes. It compares the pinned refs in
docs/upstreams/upstreams.lock.json with the latest remote branch/tag refs from
docs/upstreams/upstreams.json, classifies changed paths when possible, and emits
Markdown + JSON reports for humans or automation to review.
"""

from __future__ import annotations

import argparse
import fnmatch
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
from collections import Counter
from datetime import datetime, timezone
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[1]
DEFAULT_MANIFEST = ROOT / "docs" / "upstreams" / "upstreams.json"
DEFAULT_LOCK = ROOT / "docs" / "upstreams" / "upstreams.lock.json"


AREA_PATTERNS: list[tuple[str, tuple[str, ...]]] = [
    ("desktop-ui", ("desktop/frontend/**", "apps/desktop/**", "**/*.tsx", "**/*.css")),
    ("desktop-go", ("desktop/**",)),
    ("provider-cache", ("internal/provider/**", "internal/agent/cache*", "**/cache*")),
    ("agent-runtime", ("internal/agent/**", "internal/control/**")),
    ("server-cloud", ("internal/serve/**", "workers/**", "deploy/**", "Dockerfile", "docker-compose*")),
    ("gateway", ("internal/bot/**", "gateway/**", "bot/**")),
    ("mcp-plugin-skill", ("internal/plugin/**", "internal/pluginpkg/**", "internal/skill/**", "plugins/**", "skills/**")),
    ("security", ("internal/permission/**", "internal/sandbox/**", "internal/trust/**", "internal/crypto/**", ".github/workflows/**")),
    ("build-release", ("go.mod", "go.sum", "package*.json", "pnpm-lock.yaml", ".github/**", "scripts/**", "npm/**", "packaging/**")),
    ("docs", ("docs/**", "README*", "CHANGELOG*", "*.md")),
]

HIGH_RISK_AREAS = {"provider-cache", "agent-runtime", "security", "server-cloud"}
MEDIUM_RISK_AREAS = {"desktop-go", "desktop-ui", "mcp-plugin-skill", "gateway", "build-release"}


def run(args: list[str], cwd: Path | None = None, check: bool = True) -> subprocess.CompletedProcess[str]:
    return subprocess.run(args, cwd=cwd, check=check, text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)


def git_ls_remote(repo: str) -> dict[str, str]:
    proc = run(["git", "ls-remote", "--heads", "--tags", repo])
    refs: dict[str, str] = {}
    for line in proc.stdout.splitlines():
        if not line.strip():
            continue
        sha, ref = line.split("\t", 1)
        # Prefer peeled annotated tag objects for comparisons.
        if ref.endswith("^{}"):
            refs[ref[:-3]] = sha
        else:
            refs.setdefault(ref, sha)
    return refs


def semver_key(tag: str) -> tuple[int, ...]:
    nums = [int(x) for x in re.findall(r"\d+", tag)]
    return tuple(nums or [0])


def latest_matching_tag(refs: dict[str, str], pattern: str) -> dict[str, str] | None:
    tags: list[tuple[tuple[int, ...], str, str]] = []
    for ref, sha in refs.items():
        if not ref.startswith("refs/tags/"):
            continue
        tag = ref.removeprefix("refs/tags/")
        if fnmatch.fnmatch(tag, pattern):
            tags.append((semver_key(tag), tag, sha))
    if not tags:
        return None
    _, tag, sha = sorted(tags)[-1]
    return {"tag": tag, "sha": sha, "pattern": pattern}


def classify_path(path: str) -> str:
    normalized = path.replace("\\", "/")
    for area, patterns in AREA_PATTERNS:
        if any(fnmatch.fnmatch(normalized, pat) for pat in patterns):
            return area
    return "other"


def risk_from_areas(areas: Counter[str], changed: bool) -> str:
    if not changed:
        return "none"
    if any(a in HIGH_RISK_AREAS for a in areas):
        return "high"
    if any(a in MEDIUM_RISK_AREAS for a in areas):
        return "medium"
    if areas and set(areas) <= {"docs"}:
        return "low"
    return "medium"


def diff_changed_files(repo: str, branch: str, base_sha: str, head_sha: str) -> list[dict[str, str]]:
    if not base_sha or not head_sha or base_sha == head_sha:
        return []
    with tempfile.TemporaryDirectory(prefix="reames-upstream-") as tmp:
        work = Path(tmp)
        run(["git", "init", "-q"], cwd=work)
        run(["git", "remote", "add", "origin", repo], cwd=work)
        fetch = run(["git", "fetch", "--depth=300", "origin", branch], cwd=work, check=False)
        if fetch.returncode != 0:
            return [{"status": "?", "path": f"<diff unavailable: {fetch.stderr.strip()}>"}]
        diff = run(["git", "diff", "--name-status", base_sha, head_sha], cwd=work, check=False)
        if diff.returncode != 0:
            return [{"status": "?", "path": "<diff unavailable: base not found in shallow fetch>"}]
        out: list[dict[str, str]] = []
        for line in diff.stdout.splitlines():
            parts = line.split("\t")
            if len(parts) >= 2:
                out.append({"status": parts[0], "path": parts[-1]})
        return out


def recommendation(up: dict[str, Any], changed: bool, risk: str, areas: Counter[str]) -> str:
    if not changed:
        return "No action."
    if up.get("importance") == "primary-base":
        if risk == "high":
            return "Human review required. Prioritize cache/provider/runtime/security diffs; do not auto-merge."
        return "Review for staged adoption into Reames Agent after desktop/UI baseline remains green."
    if risk == "high":
        return "Open an advisory task only; cherry-pick concepts after review."
    if "desktop-ui" in areas or "docs" in areas:
        return "Review for product/design inspiration; do not copy wholesale."
    return "Track as reference signal; no automatic code change."


def analyze_upstream(up: dict[str, Any], lock_entry: dict[str, Any]) -> dict[str, Any]:
    refs = git_ls_remote(up["repo"])
    branch = up["branch"]
    branch_ref = f"refs/heads/{branch}"
    latest = refs.get(branch_ref, "")
    pinned = lock_entry.get("pinned") or lock_entry.get("latest_seen") or ""
    changed = bool(latest and pinned and latest != pinned)

    tag_reports = []
    for pat in up.get("tag_patterns", []):
        tag = latest_matching_tag(refs, pat)
        if tag:
            tag_reports.append(tag)

    files: list[dict[str, str]] = []
    if changed and up.get("diff", False):
        files = diff_changed_files(up["repo"], branch, pinned, latest)
    areas = Counter(classify_path(f["path"]) for f in files if not f["path"].startswith("<diff unavailable"))
    risk = risk_from_areas(areas, changed)
    return {
        "id": up["id"],
        "name": up["name"],
        "repo": up["repo"],
        "branch": branch,
        "importance": up.get("importance", ""),
        "policy": up.get("policy", "advisory-report"),
        "pinned": pinned,
        "latest": latest,
        "changed": changed,
        "tags": tag_reports,
        "files": files,
        "areas": dict(areas),
        "risk": risk,
        "recommendation": recommendation(up, changed, risk, areas),
    }


def render_markdown(report: dict[str, Any]) -> str:
    lines = [
        "# Upstream Watch Report",
        "",
        f"- Generated at: `{report['generated_at']}`",
        f"- Changed upstreams: **{report['changed_count']}** / {len(report['upstreams'])}",
        "",
        "## Summary",
        "",
        "| Upstream | Branch | Pinned | Latest | Changed | Risk | Recommendation |",
        "|---|---|---|---|---|---|---|",
    ]
    for u in report["upstreams"]:
        lines.append(
            "| {name} | `{branch}` | `{pinned}` | `{latest}` | {changed} | {risk} | {rec} |".format(
                name=u["name"],
                branch=u["branch"],
                pinned=(u["pinned"] or "")[:12],
                latest=(u["latest"] or "")[:12],
                changed="yes" if u["changed"] else "no",
                risk=u["risk"],
                rec=u["recommendation"],
            )
        )
    for u in report["upstreams"]:
        lines += ["", f"## {u['name']}", ""]
        lines += [
            f"- Repo: `{u['repo']}`",
            f"- Branch: `{u['branch']}`",
            f"- Pinned: `{u['pinned']}`",
            f"- Latest: `{u['latest']}`",
            f"- Risk: **{u['risk']}**",
            f"- Recommendation: {u['recommendation']}",
            "",
        ]
        if u["tags"]:
            lines += ["Latest matching tags:", ""]
            for t in u["tags"]:
                lines.append(f"- `{t['pattern']}` -> `{t['tag']}` (`{t['sha'][:12]}`)")
            lines.append("")
        if u["areas"]:
            lines += ["Changed areas:", ""]
            for area, count in sorted(u["areas"].items()):
                lines.append(f"- {area}: {count}")
            lines.append("")
        if u["files"]:
            lines += ["Changed files sample:", ""]
            for f in u["files"][:80]:
                lines.append(f"- `{f['status']}` `{f['path']}`")
            if len(u["files"]) > 80:
                lines.append(f"- ... {len(u['files']) - 80} more")
            lines.append("")
    return "\n".join(lines).rstrip() + "\n"


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--manifest", type=Path, default=DEFAULT_MANIFEST)
    parser.add_argument("--lock", type=Path, default=DEFAULT_LOCK)
    parser.add_argument("--out-dir", type=Path, default=ROOT / "artifacts" / "upstream-watch")
    parser.add_argument("--update-lock", action="store_true", help="Update latest_seen/pinned to latest branch refs.")
    args = parser.parse_args()

    manifest = json.loads(args.manifest.read_text(encoding="utf-8-sig"))
    lock = json.loads(args.lock.read_text(encoding="utf-8-sig")) if args.lock.exists() else {"version": 1, "upstreams": {}}
    lock_entries = lock.setdefault("upstreams", {})

    upstreams = []
    for up in manifest["upstreams"]:
        upstreams.append(analyze_upstream(up, lock_entries.get(up["id"], {})))

    now = datetime.now(timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")
    report = {
        "generated_at": now,
        "changed_count": sum(1 for u in upstreams if u["changed"]),
        "upstreams": upstreams,
    }
    args.out_dir.mkdir(parents=True, exist_ok=True)
    (args.out_dir / "upstream-report.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    (args.out_dir / "upstream-report.md").write_text(render_markdown(report), encoding="utf-8")

    if args.update_lock:
        for u in upstreams:
            lock_entries[u["id"]] = {"branch": u["branch"], "pinned": u["latest"], "latest_seen": u["latest"]}
        lock["updated_at"] = now
        args.lock.write_text(json.dumps(lock, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")

    print(f"changed_count={report['changed_count']}")
    print(f"report={args.out_dir / 'upstream-report.md'}")
    if shutil.which("git") is None:
        print("warning: git not found", file=sys.stderr)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
