import type { RecoveryReport } from "./types";

export function recoveryNeedsAttention(report: RecoveryReport | null): boolean {
  if (!report) return false;
  return report.safeModeRequested
    || report.safeModeRecommended
    || Boolean(report.pendingUpdate)
    || report.findings.some((finding) => finding.severity === "error" || finding.severity === "warning");
}
