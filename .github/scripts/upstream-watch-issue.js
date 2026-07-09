const fs = require("fs");

const marker = "<!-- reames-agent-upstream-watch -->";

module.exports = async function reconcileUpstreamIssue({ github, context, core, reportPath, markdownPath }) {
  const report = JSON.parse(fs.readFileSync(reportPath, "utf8"));
  const fingerprintMarker = `<!-- upstream-fingerprint:${report.fingerprint} -->`;
  const title = `Upstream watch: ${report.changed_count} changed, ${report.failed_count} check failure(s)`;
  const markdown = fs.readFileSync(markdownPath, "utf8");
  const body = `${marker}
${fingerprintMarker}

${markdown}

## Review workflow

- [ ] Review primary-upstream security/provider/runtime changes first
- [ ] Record adopt/defer/ignore decisions
- [ ] Open scoped implementation issues for accepted changes
- [ ] Run \`python scripts/check_upstreams.py --accept <id>\` only after review

This is advisory-only. Never auto-merge upstream code.`;

  const repo = { owner: context.repo.owner, repo: context.repo.repo };
  const { data: issues } = await github.rest.issues.listForRepo({
    ...repo,
    state: "open",
    labels: "upstream-watch",
    per_page: 100,
  });
  const existing = issues.find((issue) => (issue.body || "").includes(marker));

  if (!report.attention_count) {
    core.info("No upstream changes or check failures detected.");
    if (existing) {
      await github.rest.issues.createComment({
        ...repo,
        issue_number: existing.number,
        body: `All tracked upstreams now match their reviewed revisions. Closing automatically at ${report.generated_at}.`,
      });
      await github.rest.issues.update({
        ...repo,
        issue_number: existing.number,
        state: "closed",
        state_reason: "completed",
      });
      core.info(`Closed issue #${existing.number}`);
    }
    return { action: existing ? "closed" : "none", issue: existing?.number };
  }

  if (existing) {
    if ((existing.body || "").includes(fingerprintMarker)) {
      core.info(`Issue #${existing.number} already represents fingerprint ${report.fingerprint}; no update needed.`);
      return { action: "unchanged", issue: existing.number };
    }
    await github.rest.issues.update({
      ...repo,
      issue_number: existing.number,
      title,
      body,
    });
    core.info(`Updated issue #${existing.number}`);
    return { action: "updated", issue: existing.number };
  }

  try {
    await github.rest.issues.createLabel({
      ...repo,
      name: "upstream-watch",
      color: "6f42c1",
      description: "Reference upstream changed; advisory review needed",
    });
  } catch (error) {
    if (error.status !== 422) throw error;
  }
  const created = await github.rest.issues.create({
    ...repo,
    title,
    body,
    labels: ["upstream-watch"],
  });
  core.info(`Created issue #${created.data.number}`);
  return { action: "created", issue: created.data.number };
};

module.exports.marker = marker;
