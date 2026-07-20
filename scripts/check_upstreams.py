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
import shlex
import shutil
import subprocess
import sys
import tempfile
import urllib.parse
import urllib.request
from collections import Counter
from datetime import datetime, timezone
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[1]
DEFAULT_MANIFEST = ROOT / "docs" / "upstreams" / "upstreams.json"
DEFAULT_LOCK = ROOT / "docs" / "upstreams" / "upstreams.lock.json"
REVIEW_COMPLETE = "complete"
FULL_GIT_SHA = re.compile(r"[0-9a-fA-F]{40}")
COMMAND_TIMEOUT_SECONDS = max(1, int(os.environ.get("REAMES_UPSTREAM_COMMAND_TIMEOUT_SECONDS", "20")))
GITHUB_ATOM_COMMIT = re.compile(r"Grit::Commit/([0-9a-fA-F]{40})")


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
        timeout=COMMAND_TIMEOUT_SECONDS,
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


def github_repo_parts(repo: str) -> tuple[str, str]:
    parsed = urllib.parse.urlsplit(repo)
    parts = [part for part in parsed.path.strip("/").split("/") if part]
    if parsed.scheme != "https" or parsed.netloc.lower() != "github.com" or len(parts) != 2:
        raise ValueError(f"unsupported GitHub repository URL: {repo}")
    return parts[0], parts[1].removesuffix(".git")


def github_request_text(url: str) -> str:
    request = urllib.request.Request(url, headers={"User-Agent": "reames-agent-upstream-watch"})
    with urllib.request.urlopen(request, timeout=COMMAND_TIMEOUT_SECONDS) as response:
        return response.read().decode("utf-8", errors="replace")


def github_atom_branch_head(repo: str, branch: str) -> str:
    owner, name = github_repo_parts(repo)
    encoded_branch = urllib.parse.quote(branch, safe="")
    feed = github_request_text(f"https://github.com/{owner}/{name}/commits/{encoded_branch}.atom")
    match = GITHUB_ATOM_COMMIT.search(feed)
    if not match:
        raise ValueError(f"GitHub Atom feed did not expose a commit for branch {branch}")
    return match.group(1).lower()


def remote_refs(repo: str, branch: str) -> tuple[dict[str, str], str]:
    try:
        return git_ls_remote(repo), "git-ls-remote"
    except (OSError, subprocess.SubprocessError, ValueError) as git_error:
        try:
            head = github_atom_branch_head(repo, branch)
        except (OSError, ValueError) as atom_error:
            raise OSError(
                f"git ls-remote failed ({git_error}); GitHub Atom fallback failed ({atom_error})"
            ) from atom_error
        return {f"refs/heads/{branch}": head}, "github-atom"


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
        try:
            fetch_error = fetch_comparison_refs(work, repo, branch, base_sha, head_sha)
        except (OSError, subprocess.SubprocessError, ValueError) as exc:
            fetch_error = str(exc).strip()[:500] or type(exc).__name__
        if fetch_error:
            try:
                patch = github_compare_patch(repo, base_sha, head_sha)
                files = changed_files_from_patch(patch)
                if files:
                    fallback = {"warning": fetch_error}
                    if deep:
                        fallback["diff"] = patch.strip()[:50000]
                    return files, fallback if deep else {}
            except (OSError, ValueError):
                pass
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


def github_compare_patch(repo: str, base_sha: str, head_sha: str) -> str:
    owner, name = github_repo_parts(repo)
    patch = github_request_text(f"https://github.com/{owner}/{name}/compare/{base_sha}...{head_sha}.patch")
    if not patch.strip():
        raise ValueError("GitHub compare patch was empty")
    return patch


def changed_files_from_patch(patch: str) -> list[dict[str, str]]:
    files: list[dict[str, str]] = []
    for block in re.split(r"(?=^diff --git )", patch, flags=re.MULTILINE):
        first_line = block.partition("\n")[0]
        if not first_line.startswith("diff --git "):
            continue
        try:
            header = shlex.split(first_line)
        except ValueError:
            continue
        if len(header) < 4:
            continue
        path = header[3].removeprefix("b/")
        status = "M"
        if "\nnew file mode " in block:
            status = "A"
        elif "\ndeleted file mode " in block:
            status = "D"
            path = header[2].removeprefix("a/")
        elif "\nrename from " in block and "\nrename to " in block:
            status = "R"
        files.append({"status": status, "path": path})
    return files


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
    if up.get("importance") == "strategic-code-upstream":
        return "Human code-level capability review required. Compare native model protocol and product/runtime behavior; do not rely on release notes or auto-merge."
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
    if up.get("importance") == "strategic-code-upstream":
        return "review-required"
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
        required_areas = up.get("required_review_areas", [])
        if required_areas:
            if not isinstance(required_areas, list) or any(not str(area).strip() for area in required_areas):
                raise ValueError(f"upstream {upstream_id}: required_review_areas must be non-empty strings")
            normalized = [str(area).strip() for area in required_areas]
            if any(not re.fullmatch(r"[a-z0-9]+(?:-[a-z0-9]+)*", area) for area in normalized):
                raise ValueError(
                    f"upstream {upstream_id}: required_review_areas must use lowercase kebab-case names"
                )
            if len(set(normalized)) != len(normalized):
                raise ValueError(f"upstream {upstream_id}: required_review_areas contains duplicates")
            if not str(up.get("review_record", "")).strip():
                raise ValueError(f"upstream {upstream_id}: review_record is required with required_review_areas")


def review_coverage(up: dict[str, Any], baseline: str, reviewed: str) -> dict[str, Any]:
    required = [str(area).strip() for area in up.get("required_review_areas", []) if str(area).strip()]
    if not required:
        return {"status": "not-required", "record": "", "missing": [], "error": ""}
    record_name = str(up.get("review_record", "")).strip()
    result: dict[str, Any] = {
        "status": "incomplete",
        "record": record_name,
        "missing": list(required),
        "error": "",
    }
    if not record_name:
        result["error"] = "review record is not configured"
        return result
    root = ROOT.resolve()
    record_path = (ROOT / record_name).resolve()
    try:
        record_path.relative_to(root)
    except ValueError:
        result["error"] = "review record must stay inside the repository"
        return result
    try:
        record = json.loads(record_path.read_text(encoding="utf-8-sig"))
    except (OSError, ValueError, TypeError, json.JSONDecodeError) as exc:
        result["error"] = f"read review record: {str(exc).strip()[:240]}"
        return result
    areas = record.get("areas")
    if not isinstance(areas, dict):
        result["error"] = "review record areas must be an object"
        return result
    missing = []
    evidence_errors = []
    for area in required:
        entry = areas.get(area)
        status = entry if isinstance(entry, str) else entry.get("status") if isinstance(entry, dict) else ""
        if str(status).strip().lower() != REVIEW_COMPLETE:
            missing.append(area)
            continue
        evidence = entry.get("evidence") if isinstance(entry, dict) else None
        if not isinstance(evidence, list) or not evidence or any(not str(item).strip() for item in evidence):
            missing.append(area)
            evidence_errors.append(area)
    mismatches = []
    if str(record.get("upstream", "")).strip() not in ("", str(up.get("id", ""))):
        mismatches.append("upstream id")
    if str(record.get("baseline", "")).strip() != baseline:
        mismatches.append("baseline")
    if str(record.get("reviewed", "")).strip() != reviewed:
        mismatches.append("reviewed revision")
    result["missing"] = missing
    errors = []
    if mismatches:
        errors.append("review record mismatch: " + ", ".join(mismatches))
    if evidence_errors:
        errors.append("complete areas require non-empty evidence: " + ", ".join(evidence_errors))
    if errors:
        result["error"] = "; ".join(errors)
    elif not missing:
        result["status"] = REVIEW_COMPLETE
    return result


def lock_points(lock_entry: dict[str, Any]) -> tuple[str, str]:
    legacy = str(lock_entry.get("pinned") or lock_entry.get("latest_seen") or "")
    baseline = str(lock_entry.get("baseline") or legacy)
    reviewed = str(lock_entry.get("reviewed") or legacy)
    return baseline, reviewed


def analyze_upstream(up: dict[str, Any], lock_entry: dict[str, Any], deep: bool = False) -> dict[str, Any]:
    branch = up["branch"]
    baseline, reviewed = lock_points(lock_entry)
    coverage = review_coverage(up, baseline, reviewed)
    try:
        refs, transport = remote_refs(up["repo"], branch)
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
            "coverage": coverage,
            "transport": "unavailable",
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
    decision = decision_for(up, changed, risk, error)
    rec = (
        "Upstream check failed; inspect network, repository, and branch configuration."
        if error
        else recommendation(up, changed, risk, areas)
    )
    if not error and up.get("importance") == "primary-base" and coverage["status"] != REVIEW_COMPLETE:
        decision = "review-required"
        detail = ", ".join(coverage["missing"][:8]) or coverage["error"] or "coverage evidence"
        rec = f"Primary-base review coverage is incomplete ({detail}); complete the parity record before accepting this revision."
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
        "decision": decision,
        "error": error,
        "deep": deep_info if deep else None,
        "recommendation": rec,
        "coverage": coverage,
        "transport": transport,
    }


def report_fingerprint(upstreams: list[dict[str, Any]]) -> str:
    state = [
        {
            "id": up["id"],
            "reviewed": up["reviewed"],
            "latest": up["latest"],
            "decision": up["decision"],
            "error": up["error"],
            "coverage": up.get("coverage", {}).get("status", "not-required"),
            "coverage_record": up.get("coverage", {}).get("record", ""),
            "coverage_missing": up.get("coverage", {}).get("missing", []),
            "coverage_error": up.get("coverage", {}).get("error", ""),
        }
        for up in upstreams
    ]
    payload = json.dumps(state, ensure_ascii=True, sort_keys=True, separators=(",", ":")).encode()
    return hashlib.sha256(payload).hexdigest()[:20]


def parse_accept_revisions(values: list[str]) -> dict[str, str]:
    """Parse repeatable ID=SHA acceptance arguments with a full immutable SHA."""
    revisions: dict[str, str] = {}
    for value in values:
        upstream_id, separator, revision = value.partition("=")
        upstream_id = upstream_id.strip()
        revision = revision.strip().lower()
        if not separator or not upstream_id or not FULL_GIT_SHA.fullmatch(revision):
            raise ValueError(
                f"invalid --accept-revision {value!r}: expected ID=FULL_40_CHARACTER_GIT_SHA"
            )
        previous = revisions.get(upstream_id)
        if previous and previous != revision:
            raise ValueError(f"conflicting accepted revisions for upstream {upstream_id}")
        revisions[upstream_id] = revision
    return revisions


def acceptance_revisions(
    legacy_ids: list[str], accept_all: bool, update_lock: bool, values: list[str]
) -> dict[str, str]:
    if legacy_ids or accept_all or update_lock:
        raise ValueError(
            "unbound upstream acceptance is disabled; use --accept-revision ID=FULL_40_CHARACTER_GIT_SHA"
        )
    return parse_accept_revisions(values)


def accepted_lock_entry(
    up: dict[str, Any], previous: dict[str, Any], expected_revision: str
) -> dict[str, str]:
    baseline, _ = lock_points(previous)
    latest = str(up.get("latest", "")).lower()
    if not latest:
        raise ValueError(f"cannot accept {up['id']}: latest revision is unavailable")
    if latest != expected_revision.lower():
        raise ValueError(
            f"cannot accept {up['id']}: remote {latest} does not match reviewed revision {expected_revision.lower()}"
        )
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
        "| Upstream | Branch | Reviewed | Latest | Decision | Risk | Coverage | Recommendation |",
        "|---|---|---|---|---|---|---|---|",
    ]
    for u in report["upstreams"]:
        lines.append(
            "| {name} | `{branch}` | `{pinned}` | `{latest}` | {changed} | {risk} | {coverage} | {rec} |".format(
                name=u["name"],
                branch=u["branch"],
                pinned=(u["reviewed"] or "")[:12],
                latest=(u["latest"] or "")[:12],
                changed=u["decision"],
                risk=u["risk"],
                coverage=u.get("coverage", {}).get("status", "not-required"),
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
            f"- Remote transport: `{u.get('transport', 'unknown')}`",
            f"- Decision: **{u['decision']}**",
            f"- Risk: **{u['risk']}**",
            f"- Recommendation: {u['recommendation']}",
            f"- Review coverage: **{u.get('coverage', {}).get('status', 'not-required')}**",
            "",
        ]
        coverage = u.get("coverage", {})
        if coverage.get("record"):
            lines += [f"- Coverage record: `{coverage['record']}`"]
        if coverage.get("missing"):
            lines += [f"- Missing review areas: `{', '.join(coverage['missing'])}`"]
        if coverage.get("error"):
            lines += [f"- Coverage error: `{coverage['error']}`"]
        if coverage.get("record") or coverage.get("missing") or coverage.get("error"):
            lines.append("")
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
    parser.add_argument("--accept", action="append", default=[], metavar="ID", help=argparse.SUPPRESS)
    parser.add_argument("--accept-all", action="store_true", help=argparse.SUPPRESS)
    parser.add_argument("--update-lock", action="store_true", help=argparse.SUPPRESS)
    parser.add_argument(
        "--accept-revision",
        action="append",
        default=[],
        metavar="ID=SHA",
        help="Mark exactly one reviewed full Git SHA as accepted; repeat for multiple upstreams.",
    )
    parser.add_argument("--deep", action="store_true", help="Fetch actual diff content and commit messages (slower, more network I/O).")
    args = parser.parse_args()
    expected_revisions = acceptance_revisions(
        args.accept, args.accept_all, args.update_lock, args.accept_revision
    )

    manifest = json.loads(args.manifest.read_text(encoding="utf-8-sig"))
    validate_manifest(manifest)
    lock = json.loads(args.lock.read_text(encoding="utf-8-sig")) if args.lock.exists() else {"version": 1, "upstreams": {}}
    lock_entries = lock.setdefault("upstreams", {})

    upstreams = []
    for up in manifest["upstreams"]:
        upstreams.append(analyze_upstream(up, lock_entries.get(up["id"], {}), deep=args.deep))

    now = datetime.now(timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")
    accepted_ids = set(expected_revisions)
    known_ids = {u["id"] for u in upstreams}
    unknown_ids = accepted_ids - known_ids
    if unknown_ids:
        raise ValueError(f"unknown upstream ids: {', '.join(sorted(unknown_ids))}")
    if accepted_ids:
        manifest_by_id = {str(up["id"]): up for up in manifest["upstreams"]}
        for u in upstreams:
            if u["id"] in accepted_ids:
                source = manifest_by_id[u["id"]]
                expected_revision = expected_revisions[u["id"]]
                if str(u.get("latest", "")).lower() != expected_revision:
                    raise ValueError(
                        f"cannot accept {u['id']}: remote {u.get('latest') or '<unavailable>'} "
                        f"does not match reviewed revision {expected_revision}"
                    )
                baseline, _ = lock_points(lock_entries.get(u["id"], {}))
                coverage = review_coverage(source, baseline or expected_revision, expected_revision)
                if source.get("required_review_areas") and coverage["status"] != REVIEW_COMPLETE:
                    detail = coverage["error"] or ", ".join(coverage["missing"])
                    raise ValueError(f"cannot accept {u['id']}: review coverage incomplete ({detail})")
                lock_entries[u["id"]] = accepted_lock_entry(
                    u, lock_entries.get(u["id"], {}), expected_revision
                )
                u["reviewed"] = expected_revision
                u["changed"] = False
                u["files"] = []
                u["areas"] = {}
                u["risk"] = "none"
                u["decision"] = "up-to-date"
                u["recommendation"] = "Accepted as reviewed by explicit operator action."
                u["coverage"] = coverage
        lock["version"] = 2
        lock["updated_at"] = now
        args.lock.write_text(json.dumps(lock, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")

    fingerprint = report_fingerprint(upstreams)
    report = {
        "generated_at": now,
        "changed_count": sum(1 for u in upstreams if u["changed"]),
        "failed_count": sum(1 for u in upstreams if u["error"]),
        "attention_count": sum(
            1
            for u in upstreams
            if u["changed"] or u["error"] or u.get("coverage", {}).get("status") == "incomplete"
        ),
        "coverage_incomplete_count": sum(1 for u in upstreams if u.get("coverage", {}).get("status") == "incomplete"),
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
