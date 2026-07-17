import type { RecoveryActionRequest, RecoveryActionResult, RecoveryReport } from "./types";

const safeMode = typeof window !== "undefined" && new URLSearchParams(window.location.search).get("recovery") === "safe";
const now = Date.now();

let report: RecoveryReport = {
  schemaVersion: 1,
  generatedAt: new Date(now).toISOString(),
  safeModeRequested: safeMode,
  safeModeRecommended: safeMode,
  startup: { schemaVersion: 1, phase: safeMode ? "failed" : "healthy", version: "v0.1.0-mock", consecutiveFailures: safeMode ? 3 : 0 },
  config: {
    checks: [
      { scope: "global", path: "$REAMES_AGENT_HOME/config.toml", exists: safeMode, valid: !safeMode, error: safeMode ? "invalid TOML" : "" },
      { scope: "project", path: "$WORKSPACE/reames-agent.toml", exists: false, valid: true },
    ],
    applied: [],
  },
  configSnapshots: [{ schemaVersion: 1, id: "mock-snapshot", path: "$REAMES_AGENT_STATE/repair/mock.toml", sha256: "a".repeat(64), sourcePath: "$REAMES_AGENT_HOME/config.toml", recordedAt: new Date(now - 86_400_000).toISOString(), version: "v0.0.9" }],
  pendingUpdate: safeMode ? { schemaVersion: 1, fromVersion: "v0.0.9", toVersion: "v0.1.0-mock", platform: "mock", targetKind: "file", targetPath: "$INSTALL/reames-agent-desktop", backupPath: "$REAMES_AGENT_STATE/repair/desktop.previous", createdAt: new Date(now - 60_000).toISOString() } : undefined,
  binaries: [{ role: "current", path: "$INSTALL/reames-agent-desktop", exists: true, regular: true, size: 1_024, sha256: "b".repeat(64) }],
  sessions: [{ path: "$REAMES_AGENT_HOME/sessions", exists: true, fileCount: 4 }],
  plugins: { path: "$REAMES_AGENT_HOME/plugins.json", exists: true, fileCount: 2, enabled: safeMode ? 2 : 0, disabled: safeMode ? 0 : 2 },
  findings: safeMode ? [
    { severity: "warning", code: "startup.crash_loop", scope: "startup", message: "three incomplete startups occurred inside the bounded crash window" },
    { severity: "error", code: "config.invalid", scope: "global", message: "invalid TOML" },
  ] : [],
};

export function getMockRecoveryStatus(): RecoveryReport {
  return structuredClone(report);
}

export function runMockRecoveryAction(request: RecoveryActionRequest): RecoveryActionResult {
  let changed = false;
  if (request.action === "repair-config") {
    report.config.checks = report.config.checks.map((check) => check.scope === request.target ? { ...check, exists: false, valid: true, error: "" } : check);
    report.findings = report.findings.filter((finding) => !(finding.code === "config.invalid" && finding.scope === request.target));
    report.lastRepair = { schemaVersion: 1, id: "mock-repair", createdAt: new Date().toISOString(), changes: [{ scope: request.target || "global", targetPath: "$REAMES_AGENT_HOME/config.toml", previousPath: "$REAMES_AGENT_HOME/config.toml.reames-quarantine" }] };
    changed = true;
  } else if (request.action === "rollback-update" && report.pendingUpdate) {
    report.pendingUpdate = undefined;
    report.findings = report.findings.filter((finding) => finding.code !== "update.probationary");
    changed = true;
  } else if (request.action === "restore-config") {
    report.lastRepair = { schemaVersion: 1, id: "mock-restore", createdAt: new Date().toISOString(), changes: [{ scope: "global", targetPath: "$REAMES_AGENT_HOME/config.toml", previousPath: "$REAMES_AGENT_HOME/config.toml.reames-restore" }] };
    changed = true;
  } else if (request.action === "undo-repair" && report.lastRepair && !report.lastRepair.undone) {
    report.lastRepair = { ...report.lastRepair, undone: true, undoneAt: new Date().toISOString() };
    changed = true;
  } else if (request.action === "disable-plugins" && (report.plugins.enabled ?? 0) > 0) {
    report.plugins = { ...report.plugins, disabled: report.plugins.fileCount ?? 0, enabled: 0 };
    changed = true;
  } else if (request.action === "rebuild-state") {
    changed = true;
  }
  report = { ...report, generatedAt: new Date().toISOString() };
  return { action: request.action, changed, affected: [], report: structuredClone(report) };
}
