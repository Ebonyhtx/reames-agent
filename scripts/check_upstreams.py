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
import hashlib
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
    ("gateway", ("internal/bot/**", "internal/gatewayservice/**", "gateway/**", "bot/**")),
    ("mcp-plugin-skill", ("internal/plugin/**", "internal/pluginpkg/**", "internal/skill/**", "plugins/**", "skills/**")),
    (
        "security",
        (
            "internal/permission/**",
            "internal/sandbox/**",
            "internal/trust/**",
            "internal/crypto/**",
            "internal/secrets/**",
            "internal/guardian/**",
            "internal/doctor/session_redact*",
            "internal/tool/configwrite*",
            "internal/tool/builtin/managed_config*",
            ".github/workflows/**",
        ),
    ),
    ("build-release", ("go.mod", "go.sum", "package*.json", "pnpm-lock.yaml", ".github/**", "scripts/**", "npm/**", "packaging/**")),
    ("docs", ("docs/**", "README*", "CHANGELOG*", "*.md")),
]

HIGH_RISK_AREAS = {"provider-cache", "agent-runtime", "security", "server-cloud"}
MEDIUM_RISK_AREAS = {"desktop-go", "desktop-ui", "mcp-plugin-skill", "gateway", "build-release"}


def run(args: list[str], cwd: Path | None = None, check: bool = True) -> subprocess.CompletedProcess[str]:
    # Git commit subjects and patches are UTF-8 even when Windows' active code
    # page is GBK. Relying on locale decoding makes deep upstream analysis crash
    # as soon as an upstream contains bilingual commit messages.
    return subprocess.run(
        args,
        cwd=cwd,
        check=check,
        text=True,
        encoding="utf-8",
        errors="replace",
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )


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


def fetch_comparison_refs(work: Path, repo: str, branch: str, base_sha: str, head_sha: str) -> str:
    """Initialize a temporary repository and fetch both comparison commits."""
    run(["git", "init", "-q"], cwd=work)
    run(["git", "remote", "add", "origin", repo], cwd=work)
    branch_fetch = run(["git", "fetch", "--no-tags", "--depth=300", "origin", branch], cwd=work, check=False)
    if branch_fetch.returncode != 0:
        return f"fetch failed: {branch_fetch.stderr.strip()[:200]}"
    for sha in dict.fromkeys((base_sha, head_sha)):
        present = run(["git", "cat-file", "-e", f"{sha}^{{commit}}"], cwd=work, check=False)
        if present.returncode == 0:
            continue
        fetched = run(["git", "fetch", "--no-tags", "--depth=1", "origin", sha], cwd=work, check=False)
        if fetched.returncode != 0:
            return f"commit {sha[:12]} unavailable: {fetched.stderr.strip()[:160]}"
    return ""


def comparison_evidence(
    repo: str, branch: str, base_sha: str, head_sha: str, deep: bool = False
) -> tuple[list[dict[str, str]], dict[str, str]]:
    if not base_sha or not head_sha or base_sha == head_sha:
        return [], {}
    with tempfile.TemporaryDirectory(prefix="reames-upstream-") as tmp:
        work = Path(tmp)
        fetch_error = fetch_comparison_refs(work, repo, branch, base_sha, head_sha)
        if fetch_error:
            unavailable = [{"status": "?", "path": f"<diff unavailable: {fetch_error}>"}]
            return unavailable, {"error": fetch_error} if deep else {}

        names = run(["git", "diff", "--name-status", base_sha, head_sha], cwd=work, check=False)
        if names.returncode != 0:
            error = f"diff failed: {names.stderr.strip()[:200]}"
            unavailable = [{"status": "?", "path": f"<diff unavailable: {error}>"}]
            return unavailable, {"error": error} if deep else {}
        files: list[dict[str, str]] = []
        for line in names.stdout.splitlines():
            parts = line.split("\t")
            if len(parts) >= 2:
                files.append({"status": parts[0], "path": parts[-1]})
        if not deep:
            return files, {}

        log = run(
            ["git", "log", "--oneline", "--no-merges", f"{base_sha}..{head_sha}"],
            cwd=work, check=False,
        )
        patch = run(
            ["git", "diff", "--patch", "--stat", base_sha, head_sha],
            cwd=work, check=False,
        )
        result: dict[str, str] = {}
        if log.returncode == 0:
            result["commits"] = log.stdout.strip()[:10000]
        else:
            result["error"] = f"log failed: {log.stderr.strip()[:200]}"
        if patch.returncode == 0:
            result["diff"] = patch.stdout.strip()[:50000]
        elif "error" not in result:
            result["error"] = f"diff failed: {patch.stderr.strip()[:200]}"
        return files, result


def diff_changed_files(repo: str, branch: str, base_sha: str, head_sha: str) -> list[dict[str, str]]:
    files, _ = comparison_evidence(repo, branch, base_sha, head_sha)
    return files


def deep_diff_content(repo: str, branch: str, base_sha: str, head_sha: str) -> dict[str, str]:
    """Return bounded commit-log and patch evidence for two upstream revisions."""
    _, deep = comparison_evidence(repo, branch, base_sha, head_sha, deep=True)
    return deep


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


def decision_for(up: dict[str, Any], changed: bool, risk: str, error: str = "") -> str:
    if error:
        return "check-failed"
    if not changed:
        return "up-to-date"
    if up.get("importance") == "primary-base":
        return "review-required" if risk == "high" else "adoption-candidate"
    if risk == "high":
        return "security-signal"
    if risk == "low":
        return "low-priority"
    return "reference-review"


def validate_manifest(manifest: dict[str, Any]) -> None:
    upstreams = manifest.get("upstreams")
    if not isinstance(upstreams, list) or not upstreams:
        raise ValueError("manifest.upstreams must be a non-empty list")
    seen: set[str] = set()
    for index, up in enumerate(upstreams):
        if not isinstance(up, dict):
            raise ValueError(f"manifest.upstreams[{index}] must be an object")
        missing = [key for key in ("id", "name", "repo", "branch") if not str(up.get(key, "")).strip()]
        if missing:
            raise ValueError(f"upstream #{index + 1} missing required fields: {', '.join(missing)}")
        upstream_id = str(up["id"]).strip()
        if upstream_id in seen:
            raise ValueError(f"duplicate upstream id: {upstream_id}")
        seen.add(upstream_id)
        repo = str(up["repo"]).strip()
        if not re.match(r"^https://github\.com/[^/]+/[^/]+(?:\.git)?$", repo):
            raise ValueError(f"upstream {upstream_id}: repo must be an official HTTPS GitHub repository URL")


def lock_points(lock_entry: dict[str, Any]) -> tuple[str, str]:
    legacy = str(lock_entry.get("pinned") or lock_entry.get("latest_seen") or "")
    baseline = str(lock_entry.get("baseline") or legacy)
    reviewed = str(lock_entry.get("reviewed") or legacy)
    return baseline, reviewed


def analyze_upstream(up: dict[str, Any], lock_entry: dict[str, Any], deep: bool = False) -> dict[str, Any]:
    branch = up["branch"]
    baseline, reviewed = lock_points(lock_entry)
    try:
        refs = git_ls_remote(up["repo"])
    except (OSError, subprocess.SubprocessError, ValueError) as exc:
        error = str(exc).strip()[:500] or type(exc).__name__
        return {
            "id": up["id"],
            "name": up["name"],
            "repo": up["repo"],
            "branch": branch,
            "importance": up.get("importance", ""),
            "policy": up.get("policy", "advisory-report"),
            "baseline": baseline,
            "reviewed": reviewed,
            "latest": "",
            "changed": False,
            "tags": [],
            "files": [],
            "areas": {},
            "risk": "unknown",
            "decision": decision_for(up, False, "unknown", error),
            "error": error,
            "recommendation": "Upstream check failed; inspect network, repository, and branch configuration.",
            "deep": None,
        }

    branch_ref = f"refs/heads/{branch}"
    latest = refs.get(branch_ref, "")
    error = "" if latest else f"branch not found: {branch}"
    changed = bool(latest and reviewed and latest != reviewed)
    tag_reports = []
    for pat in up.get("tag_patterns", []):
        tag = latest_matching_tag(refs, pat)
        if tag:
            tag_reports.append(tag)

    files: list[dict[str, str]] = []
    deep_info: dict[str, str] = {}
    if deep and changed and up.get("diff", False):
        files, deep_info = comparison_evidence(up["repo"], branch, reviewed, latest, deep=True)
    elif changed and up.get("diff", False):
        files = diff_changed_files(up["repo"], branch, reviewed, latest)
    areas = Counter(classify_path(f["path"]) for f in files if not f["path"].startswith("<diff unavailable"))
    risk = risk_from_areas(areas, changed)
    return {
        "id": up["id"],
        "name": up["name"],
        "repo": up["repo"],
        "branch": branch,
        "importance": up.get("importance", ""),
        "policy": up.get("policy", "advisory-report"),
        "baseline": baseline,
        "reviewed": reviewed,
        "latest": latest,
        "changed": changed,
        "tags": tag_reports,
        "files": files,
        "areas": dict(areas),
        "risk": "unknown" if error else risk,
        "decision": decision_for(up, changed, risk, error),
        "error": error,
        "deep": deep_info if deep else None,
        "recommendation": (
            "Upstream check failed; inspect network, repository, and branch configuration."
            if error
            else recommendation(up, changed, risk, areas)
        ),
    }


def report_fingerprint(upstreams: list[dict[str, Any]]) -> str:
    state = [
        {
            "id": up["id"],
            "reviewed": up["reviewed"],
            "latest": up["latest"],
            "decision": up["decision"],
            "error": up["error"],
        }
        for up in upstreams
    ]
    payload = json.dumps(state, ensure_ascii=True, sort_keys=True, separators=(",", ":")).encode()
    return hashlib.sha256(payload).hexdigest()[:20]


def accepted_lock_entry(up: dict[str, Any], previous: dict[str, Any]) -> dict[str, str]:
    baseline, _ = lock_points(previous)
    latest = str(up.get("latest", ""))
    if not latest:
        raise ValueError(f"cannot accept {up['id']}: latest revision is unavailable")
    return {
        "branch": str(up["branch"]),
        "baseline": baseline or latest,
        "reviewed": latest,
        "latest_seen": latest,
    }


def render_markdown(report: dict[str, Any]) -> str:
    lines = [
        "# Upstream Watch Report",
        "",
        f"- Generated at: `{report['generated_at']}`",
        f"- Changed upstreams: **{report['changed_count']}** / {len(report['upstreams'])}",
        f"- Failed checks: **{report['failed_count']}**",
        f"- State fingerprint: `{report['fingerprint']}`",
        "",
        "## Summary",
        "",
        "| Upstream | Branch | Reviewed | Latest | Decision | Risk | Recommendation |",
        "|---|---|---|---|---|---|---|",
    ]
    for u in report["upstreams"]:
        lines.append(
            "| {name} | `{branch}` | `{pinned}` | `{latest}` | {changed} | {risk} | {rec} |".format(
                name=u["name"],
                branch=u["branch"],
                pinned=(u["reviewed"] or "")[:12],
                latest=(u["latest"] or "")[:12],
                changed=u["decision"],
                risk=u["risk"],
                rec=u["recommendation"],
            )
        )
    for u in report["upstreams"]:
        lines += ["", f"## {u['name']}", ""]
        lines += [
            f"- Repo: `{u['repo']}`",
            f"- Branch: `{u['branch']}`",
            f"- Source baseline: `{u['baseline']}`",
            f"- Reviewed: `{u['reviewed']}`",
            f"- Latest: `{u['latest']}`",
            f"- Decision: **{u['decision']}**",
            f"- Risk: **{u['risk']}**",
            f"- Recommendation: {u['recommendation']}",
            "",
        ]
        if u["error"]:
            lines += [f"- Check error: `{u['error']}`", ""]
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
    parser.add_argument("--accept", action="append", default=[], metavar="ID", help="Mark one upstream revision as reviewed; repeat for multiple IDs.")
    parser.add_argument("--accept-all", action="store_true", help="Mark every successfully checked upstream revision as reviewed.")
    parser.add_argument("--update-lock", action="store_true", help=argparse.SUPPRESS)  # legacy alias for --accept-all
    parser.add_argument("--deep", action="store_true", help="Fetch actual diff content and commit messages (slower, more network I/O).")
    args = parser.parse_args()

    manifest = json.loads(args.manifest.read_text(encoding="utf-8-sig"))
    validate_manifest(manifest)
    lock = json.loads(args.lock.read_text(encoding="utf-8-sig")) if args.lock.exists() else {"version": 1, "upstreams": {}}
    lock_entries = lock.setdefault("upstreams", {})

    upstreams = []
    for up in manifest["upstreams"]:
        upstreams.append(analyze_upstream(up, lock_entries.get(up["id"], {}), deep=args.deep))

    now = datetime.now(timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")
    accepted_ids = set(args.accept)
    if args.accept_all or args.update_lock:
        accepted_ids = {u["id"] for u in upstreams if u["latest"]}
    known_ids = {u["id"] for u in upstreams}
    unknown_ids = accepted_ids - known_ids
    if unknown_ids:
        raise ValueError(f"unknown upstream ids: {', '.join(sorted(unknown_ids))}")
    if accepted_ids:
        for u in upstreams:
            if u["id"] in accepted_ids:
                lock_entries[u["id"]] = accepted_lock_entry(u, lock_entries.get(u["id"], {}))
                u["reviewed"] = u["latest"]
                u["changed"] = False
                u["files"] = []
                u["areas"] = {}
                u["risk"] = "none"
                u["decision"] = "up-to-date"
                u["recommendation"] = "Accepted as reviewed by explicit operator action."
        lock["version"] = 2
        lock["updated_at"] = now
        args.lock.write_text(json.dumps(lock, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")

    fingerprint = report_fingerprint(upstreams)
    report = {
        "generated_at": now,
        "changed_count": sum(1 for u in upstreams if u["changed"]),
        "failed_count": sum(1 for u in upstreams if u["error"]),
        "attention_count": sum(1 for u in upstreams if u["changed"] or u["error"]),
        "fingerprint": fingerprint,
        "upstreams": upstreams,
    }
    args.out_dir.mkdir(parents=True, exist_ok=True)
    (args.out_dir / "upstream-report.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    (args.out_dir / "upstream-report.md").write_text(render_markdown(report), encoding="utf-8")

    print(f"changed_count={report['changed_count']}")
    if accepted_ids:
        print(f"accepted={','.join(sorted(accepted_ids))}")
    print(f"report={args.out_dir / 'upstream-report.md'}")
    if shutil.which("git") is None:
        print("warning: git not found", file=sys.stderr)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
