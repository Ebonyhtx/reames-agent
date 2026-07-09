import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { createRequire } from "node:module";

const require = createRequire(import.meta.url);
const reconcile = require("../.github/scripts/upstream-watch-issue.js");

function fixture(report, issues = []) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "reames-upstream-watch-"));
  const reportPath = path.join(dir, "report.json");
  const markdownPath = path.join(dir, "report.md");
  fs.writeFileSync(reportPath, JSON.stringify(report));
  fs.writeFileSync(markdownPath, "# Report\n");
  const calls = [];
  const api = (name, result = {}) => async (args) => {
    calls.push([name, args]);
    return result;
  };
  const github = {
    rest: {
      issues: {
        listForRepo: api("list", { data: issues }),
        createComment: api("comment"),
        update: api("update"),
        createLabel: api("label"),
        create: api("create", { data: { number: 42 } }),
      },
    },
  };
  const core = { info: (message) => calls.push(["info", message]) };
  return { dir, reportPath, markdownPath, github, core, calls };
}

const baseReport = {
  generated_at: "2026-07-09T00:00:00Z",
  changed_count: 1,
  failed_count: 0,
  attention_count: 1,
  fingerprint: "abc123",
};
const context = { repo: { owner: "owner", repo: "repo" } };

{
  const f = fixture(baseReport);
  const result = await reconcile({ ...f, context });
  assert.deepEqual(result, { action: "created", issue: 42 });
  assert.equal(f.calls.filter(([name]) => name === "create").length, 1);
  fs.rmSync(f.dir, { recursive: true, force: true });
}

{
  const body = `${reconcile.marker}\n<!-- upstream-fingerprint:abc123 -->`;
  const f = fixture(baseReport, [{ number: 7, body }]);
  const result = await reconcile({ ...f, context });
  assert.deepEqual(result, { action: "unchanged", issue: 7 });
  assert.equal(f.calls.filter(([name]) => name === "update").length, 0);
  fs.rmSync(f.dir, { recursive: true, force: true });
}

{
  const report = { ...baseReport, changed_count: 0, attention_count: 0, fingerprint: "clean" };
  const f = fixture(report, [{ number: 9, body: reconcile.marker }]);
  const result = await reconcile({ ...f, context });
  assert.deepEqual(result, { action: "closed", issue: 9 });
  const update = f.calls.find(([name]) => name === "update");
  assert.equal(update[1].state, "closed");
  fs.rmSync(f.dir, { recursive: true, force: true });
}

console.log("upstream watch issue reconciliation: 3 passed");
