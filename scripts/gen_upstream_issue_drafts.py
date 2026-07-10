#!/usr/bin/env python3
"""Generate local upstream research issue draft from the upstream report.

Reads artifacts/upstream-watch/upstream-report.json (produced by
check_upstreams.py) and generates a Markdown issue draft for each
changed upstream. Drafts are written to artifacts/upstream-watch/drafts/.

This script NEVER calls GitHub APIs — it only generates local files.
Real issue creation requires explicit human authorisation.
"""

from __future__ import annotations

import json
import re
import sys
from datetime import datetime, timezone
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
DEFAULT_REPORT = ROOT / "artifacts" / "upstream-watch" / "upstream-report.json"
DRAFT_DIR = ROOT / "artifacts" / "upstream-watch" / "drafts"


DECISION_LABELS = {
    "review-required": "⚠️ review-required",
    "adoption-candidate": "📋 adoption-candidate",
    "security-signal": "🔒 security-signal",
    "reference-review": "📖 reference-review",
    "low-priority": "📎 low-priority",
    "up-to-date": "✅ up-to-date",
    "check-failed": "❌ check-failed",
}

RISK_LABELS = {
    "high": "🔴 high",
    "medium": "🟡 medium",
    "low": "🟢 low",
    "none": "⚪ none",
    "unknown": "❓ unknown",
}


def safe_slug(value: str) -> str:
    """Return a bounded filename component for a report-controlled id."""
    slug = re.sub(r"[^A-Za-z0-9._-]+", "-", value).strip("._-")
    if not slug:
        raise ValueError("upstream id does not contain a safe filename component")
    return slug[:80]


def generate_issue_draft(upstream: dict) -> str:
    """Generate a single-issue Markdown draft for one upstream."""
    name = upstream["name"]
    repo = upstream["repo"]
    branch = upstream["branch"]
    decision = upstream.get("decision", "unknown")
    risk = upstream.get("risk", "unknown")
    recommendation = upstream.get("recommendation", "")
    reviewed = (upstream.get("reviewed") or "")[:12]
    latest = (upstream.get("latest") or "")[:12]
    files = upstream.get("files", [])
    areas = upstream.get("areas", {})
    error = upstream.get("error", "")

    lines = [
        f"# Upstream: {name}",
        "",
        f"**Decision**: {DECISION_LABELS.get(decision, decision)}",
        f"**Risk**: {RISK_LABELS.get(risk, risk)}",
        "",
        "## Details",
        "",
        f"- Repository: `{repo}`",
        f"- Branch: `{branch}`",
        f"- Reviewed: `{reviewed}`",
        f"- Latest: `{latest}`",
        f"- Recommendation: {recommendation}",
        "",
    ]

    if error:
        lines += [f"⚠️ Check error: `{error}`", ""]

    if areas:
        lines += ["## Changed Areas", ""]
        for area, count in sorted(areas.items()):
            lines.append(f"- {area}: {count} files")
        lines.append("")

    if files:
        lines += ["## Changed Files", ""]
        lines.append("```")
        for f in files[:80]:
            lines.append(f"{f['status']}\t{f['path']}")
        if len(files) > 80:
            lines.append(f"... {len(files) - 80} more files")
        lines += ["```", ""]

    deep = upstream.get("deep", {})
    if deep:
        if deep.get("commits"):
            lines += ["## Commits", "", "```", deep["commits"][:5000], "```", ""]
        if deep.get("diff"):
            diff_len = len(deep["diff"])
            lines += [f"## Diff ({diff_len} bytes)", "", "```diff", deep["diff"][:30000], "```", ""]

    lines += [
        "## Review Checklist",
        "",
        "- [ ] Read all changed files",
        "- [ ] Assess breaking changes / API compatibility",
        "- [ ] Check for security implications",
        "- [ ] Determine adoption strategy (adopt / defer / ignore)",
        "- [ ] Estimate adaptation effort",
        "- [ ] Update `docs/upstreams/upstreams.lock.json` via `--accept`",
        "",
        "---",
        f"_Generated {datetime.now(timezone.utc).strftime('%Y-%m-%d %H:%M UTC')} by scripts/gen_upstream_issue_drafts.py_",
        "",
    ]

    return "\n".join(lines)


def main() -> int:
    report_path = DEFAULT_REPORT
    if not report_path.exists():
        print(f"error: report not found at {report_path}", file=sys.stderr)
        print("run: python scripts/check_upstreams.py --deep first", file=sys.stderr)
        return 1

    report = json.loads(report_path.read_text(encoding="utf-8"))
    upstreams = report.get("upstreams", [])

    changed = [u for u in upstreams if u.get("changed") or u.get("error")]
    if not changed:
        print("No changed upstreams — nothing to draft.")
        return 0

    DRAFT_DIR.mkdir(parents=True, exist_ok=True)

    for u in changed:
        slug = safe_slug(str(u["id"]))
        draft = generate_issue_draft(u)
        path = DRAFT_DIR / f"{slug}.md"
        path.write_text(draft, encoding="utf-8")
        print(f"  wrote {path}")

    print(f"\nGenerated {len(changed)} issue drafts in {DRAFT_DIR}")
    print("To publish: review drafts, then use GitHub CLI or web UI.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
