import { useMemo, useState } from "react";
import {
  AlertTriangle,
  ArchiveRestore,
  Ban,
  CheckCircle2,
  DatabaseBackup,
  RefreshCw,
  RotateCcw,
  ShieldAlert,
  ShieldCheck,
  Wrench,
  X,
} from "lucide-react";
import { useRecoveryT, type RecoveryTranslator } from "./recoveryCopy";
import { app } from "../lib/bridge";
import { recoveryNeedsAttention } from "../lib/recovery";
import type { RecoveryActionRequest, RecoveryFinding, RecoveryReport } from "../lib/types";

export interface RecoveryActionConfirmation {
  title: string;
  message: string;
  detail: string;
  confirmLabel: string;
  destructive: boolean;
}

interface RecoveryCenterProps {
  report: RecoveryReport | null;
  loading: boolean;
  error: string;
  forced: boolean;
  cancelLabel: string;
  onClose: () => void;
  onRefresh: () => void;
  onReport: (report: RecoveryReport) => void;
  onAction?: (request: RecoveryActionRequest, confirmation: RecoveryActionConfirmation) => void;
}

function findingTitle(finding: RecoveryFinding, t: RecoveryTranslator): string {
  switch (finding.code) {
    case "startup.state_invalid": return t("recoveryCenter.finding.startupStateInvalid");
    case "startup.crash_loop": return t("recoveryCenter.finding.crashLoop");
    case "config.invalid": return t("recoveryCenter.finding.configInvalid");
    case "config.snapshots_unreadable": return t("recoveryCenter.finding.snapshotsUnreadable");
    case "config.repair_metadata_invalid": return t("recoveryCenter.finding.repairMetadataInvalid");
    case "update.probationary": return t("recoveryCenter.finding.updateProbationary");
    case "update.metadata_invalid": return t("recoveryCenter.finding.updateMetadataInvalid");
    case "plugins.state_invalid": return t("recoveryCenter.finding.pluginsInvalid");
    default: return finding.code;
  }
}

function formatTime(value: string): string {
  if (!value) return "—";
  const parsed = new Date(value);
  return Number.isNaN(parsed.getTime()) ? value : parsed.toLocaleString();
}

export function RecoveryCenter({ report, loading, error, forced, cancelLabel, onClose, onRefresh, onReport, onAction }: RecoveryCenterProps) {
  const t = useRecoveryT();
  const [snapshotID, setSnapshotID] = useState("");
  const [rebuildTarget, setRebuildTarget] = useState("all");
  const [busyAction, setBusyAction] = useState("");
  const [actionError, setActionError] = useState("");
  const [actionStatus, setActionStatus] = useState("");
  const snapshots = report?.configSnapshots ?? [];
  const selectedSnapshot = snapshotID || snapshots[0]?.id || "";
  const invalidGlobal = report?.config.checks.some((check) => check.scope === "global" && check.exists && !check.valid) ?? false;
  const invalidProject = report?.config.checks.some((check) => check.scope === "project" && check.exists && !check.valid) ?? false;
  const pendingUpdate = report?.pendingUpdate;
  const undoableRepair = report?.lastRepair && !report.lastRepair.undone ? report.lastRepair : undefined;
  const busy = busyAction !== "";
  const severity = useMemo(() => {
    if (!report) return "loading";
    if (report.safeModeRequested) return "safe";
    if (report.findings.some((finding) => finding.severity === "error")) return "error";
    if (recoveryNeedsAttention(report)) return "warning";
    return "healthy";
  }, [report]);
  const validConfigs = report?.config.checks.filter((check) => !check.exists || check.valid).length ?? 0;
  const totalConfigs = report?.config.checks.length ?? 0;

  const confirm = async (request: RecoveryActionRequest, title: string, message: string, detail: string, confirmLabel: string, destructive = false) => {
    const confirmation = { title, message, detail, confirmLabel, destructive };
    if (onAction) {
      onAction(request, confirmation);
      return;
    }
    if (busyAction) return;
    setActionError("");
    setActionStatus("");
    try {
      const confirmed = await app.ConfirmAction({ ...confirmation, cancelLabel });
      if (!confirmed) return;
      setBusyAction(request.action);
      const result = await app.RunRecoveryAction(request);
      onReport(result.report);
      setActionStatus(t(result.changed ? "recoveryCenter.actionChanged" : "recoveryCenter.actionNoChange"));
    } catch (err) {
      setActionError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusyAction("");
    }
  };

  return (
    <section className="recovery-center" aria-labelledby="recovery-center-title" data-severity={severity}>
      <header className="recovery-center__hero">
        <div className="recovery-center__hero-icon" aria-hidden="true">
          {severity === "healthy" ? <ShieldCheck size={24} /> : severity === "loading" ? <RefreshCw size={24} /> : <ShieldAlert size={24} />}
        </div>
        <div className="recovery-center__hero-copy">
          <p className="recovery-center__eyebrow">{t("recoveryCenter.eyebrow")}</p>
          <h1 id="recovery-center-title">{t("recoveryCenter.title")}</h1>
          <p>
            {report?.safeModeRequested
              ? t("recoveryCenter.safeModeBody")
              : report?.safeModeRecommended
                ? t("recoveryCenter.crashLoopBody")
                : severity === "healthy"
                  ? t("recoveryCenter.healthyBody")
                  : t("recoveryCenter.attentionBody")}
          </p>
        </div>
        <div className="recovery-center__hero-actions">
          <button className="btn" type="button" onClick={onRefresh} disabled={loading || busy}>
            <RefreshCw size={14} className={loading ? "recovery-center__spin" : ""} />
            {t("recoveryCenter.refresh")}
          </button>
          {!forced && (
            <button className="btn" type="button" onClick={onClose} aria-label={t("recoveryCenter.close")}>
              <X size={14} />
              {t("recoveryCenter.close")}
            </button>
          )}
        </div>
      </header>

      <div className="recovery-center__notice" role="status" aria-live="polite">
        <ShieldCheck size={15} aria-hidden="true" />
        <span>{t("recoveryCenter.credentialFree")}</span>
      </div>

      {(error || actionError) && (
        <div className="recovery-center__error" role="alert">
          <AlertTriangle size={16} />
          <span>{actionError || error}</span>
        </div>
      )}

      {actionStatus && <div className="recovery-center__notice" role="status" aria-live="polite">{actionStatus}</div>}

      {!report && loading ? (
        <div className="recovery-center__loading" role="status">{t("recoveryCenter.loading")}</div>
      ) : report ? (
        <>
          <div className="recovery-center__status-grid" aria-label={t("recoveryCenter.statusOverview")}>
            <StatusCard label={t("recoveryCenter.startup")} value={report.startup.phase || t("recoveryCenter.notRecorded")} detail={t("recoveryCenter.failures", { count: report.startup.consecutiveFailures ?? 0 })} tone={report.safeModeRecommended ? "warning" : "normal"} />
            <StatusCard label={t("recoveryCenter.configuration")} value={t("recoveryCenter.configValid", { valid: validConfigs, total: totalConfigs })} detail={snapshots.length > 0 ? t("recoveryCenter.snapshots", { count: snapshots.length }) : t("recoveryCenter.noSnapshots")} tone={invalidGlobal || invalidProject ? "error" : "normal"} />
            <StatusCard label={t("recoveryCenter.releaseUnit")} value={pendingUpdate ? t("recoveryCenter.updatePending", { version: pendingUpdate.toVersion }) : t("recoveryCenter.noPendingUpdate")} detail={report.binaries.length > 0 ? t("recoveryCenter.binaries", { count: report.binaries.length }) : t("recoveryCenter.noBinaryEvidence")} tone={pendingUpdate ? "warning" : "normal"} />
            <StatusCard label={t("recoveryCenter.extensions")} value={t("recoveryCenter.pluginsEnabled", { count: report.plugins.enabled ?? 0 })} detail={report.plugins.error ? t("recoveryCenter.storeUnreadable") : t("recoveryCenter.pluginsDisabled", { count: report.plugins.disabled ?? 0 })} tone={report.plugins.error ? "error" : "normal"} />
          </div>

          <section className="recovery-center__section" aria-labelledby="recovery-findings-title">
            <div className="recovery-center__section-head">
              <div>
                <h2 id="recovery-findings-title">{t("recoveryCenter.findings")}</h2>
                <p>{t("recoveryCenter.findingsHint")}</p>
              </div>
              <span className="recovery-center__count">{report.findings.length}</span>
            </div>
            {report.findings.length === 0 ? (
              <div className="recovery-center__empty"><CheckCircle2 size={16} />{t("recoveryCenter.noFindings")}</div>
            ) : (
              <ul className="recovery-center__findings">
                {report.findings.map((finding, index) => (
                  <li key={`${finding.code}-${index}`} data-severity={finding.severity}>
                    <AlertTriangle size={15} aria-hidden="true" />
                    <div>
                      <strong>{findingTitle(finding, t)}</strong>
                      <p>{finding.message}</p>
                      {finding.action && <small>{finding.action}</small>}
                    </div>
                  </li>
                ))}
              </ul>
            )}
          </section>

          <section className="recovery-center__section" aria-labelledby="recovery-actions-title">
            <div className="recovery-center__section-head">
              <div>
                <h2 id="recovery-actions-title">{t("recoveryCenter.actions")}</h2>
                <p>{t("recoveryCenter.actionsHint")}</p>
              </div>
            </div>
            <div className="recovery-center__action-grid">
              <RecoveryActionCard icon={<Wrench size={17} />} title={t("recoveryCenter.repairConfig")} body={t("recoveryCenter.repairConfigBody")}>
                <button className="btn btn--primary" type="button" disabled={!invalidGlobal || busy} onClick={() => void confirm(
                  { action: "repair-config", target: "global" },
                  t("recoveryCenter.confirmRepairTitle"), t("recoveryCenter.confirmRepairGlobal"), t("recoveryCenter.reversibleHint"), t("recoveryCenter.repair"),
                )}>{busyAction === "repair-config" ? t("recoveryCenter.working") : t("recoveryCenter.repairGlobal")}</button>
                <button className="btn" type="button" disabled={!invalidProject || busy} onClick={() => void confirm(
                  { action: "repair-config", target: "project" },
                  t("recoveryCenter.confirmRepairTitle"), t("recoveryCenter.confirmRepairProject"), t("recoveryCenter.reversibleHint"), t("recoveryCenter.repair"),
                )}>{t("recoveryCenter.repairProject")}</button>
              </RecoveryActionCard>

              <RecoveryActionCard icon={<RotateCcw size={17} />} title={t("recoveryCenter.rollbackUpdate")} body={t("recoveryCenter.rollbackUpdateBody")}>
                <button className="btn btn--danger" type="button" disabled={!pendingUpdate || busy} onClick={() => pendingUpdate && void confirm(
                  { action: "rollback-update", expectedUpdateVersion: pendingUpdate.toVersion, expectedUpdateCreatedAt: pendingUpdate.createdAt },
                  t("recoveryCenter.confirmRollbackTitle"), t("recoveryCenter.confirmRollback", { version: pendingUpdate.toVersion }), pendingUpdate.targetPath, t("recoveryCenter.rollback"), true,
                )}>{busyAction === "rollback-update" ? t("recoveryCenter.working") : t("recoveryCenter.rollback")}</button>
              </RecoveryActionCard>

              <RecoveryActionCard icon={<DatabaseBackup size={17} />} title={t("recoveryCenter.restoreSnapshot")} body={t("recoveryCenter.restoreSnapshotBody")}>
                <label className="recovery-center__field">
                  <span>{t("recoveryCenter.snapshot")}</span>
                  <select value={selectedSnapshot} onChange={(event) => setSnapshotID(event.target.value)} disabled={snapshots.length === 0 || busy}>
                    {snapshots.length === 0 && <option value="">{t("recoveryCenter.noSnapshots")}</option>}
                    {snapshots.map((snapshot) => <option key={snapshot.id} value={snapshot.id}>{snapshot.version || formatTime(snapshot.recordedAt)}</option>)}
                  </select>
                </label>
                <button className="btn" type="button" disabled={!selectedSnapshot || busy} onClick={() => void confirm(
                  { action: "restore-config", snapshotId: selectedSnapshot },
                  t("recoveryCenter.confirmRestoreTitle"), t("recoveryCenter.confirmRestore"), t("recoveryCenter.reversibleHint"), t("recoveryCenter.restore"),
                )}>{busyAction === "restore-config" ? t("recoveryCenter.working") : t("recoveryCenter.restore")}</button>
              </RecoveryActionCard>

              <RecoveryActionCard icon={<ArchiveRestore size={17} />} title={t("recoveryCenter.undoRepair")} body={t("recoveryCenter.undoRepairBody")}>
                <button className="btn" type="button" disabled={!undoableRepair || busy} onClick={() => undoableRepair && void confirm(
                  { action: "undo-repair", expectedRepairId: undoableRepair.id },
                  t("recoveryCenter.confirmUndoTitle"), t("recoveryCenter.confirmUndo"), undoableRepair.id, t("recoveryCenter.undo"),
                )}>{busyAction === "undo-repair" ? t("recoveryCenter.working") : t("recoveryCenter.undo")}</button>
              </RecoveryActionCard>

              <RecoveryActionCard icon={<Ban size={17} />} title={t("recoveryCenter.disablePlugins")} body={t("recoveryCenter.disablePluginsBody")}>
                <button className="btn" type="button" disabled={(report.plugins.enabled ?? 0) === 0 || busy} onClick={() => void confirm(
                  { action: "disable-plugins" },
                  t("recoveryCenter.confirmDisableTitle"), t("recoveryCenter.confirmDisable"), t("recoveryCenter.pluginsCanReenable"), t("recoveryCenter.disable"),
                )}>{busyAction === "disable-plugins" ? t("recoveryCenter.working") : t("recoveryCenter.disable")}</button>
              </RecoveryActionCard>

              <RecoveryActionCard icon={<ArchiveRestore size={17} />} title={t("recoveryCenter.rebuildState")} body={t("recoveryCenter.rebuildStateBody")}>
                <label className="recovery-center__field">
                  <span>{t("recoveryCenter.rebuildTarget")}</span>
                  <select value={rebuildTarget} onChange={(event) => setRebuildTarget(event.target.value)} disabled={busy}>
                    <option value="all">{t("recoveryCenter.rebuildAll")}</option>
                    <option value="tabs">{t("recoveryCenter.rebuildTabs")}</option>
                    <option value="projects">{t("recoveryCenter.rebuildProjects")}</option>
                    <option value="window">{t("recoveryCenter.rebuildWindow")}</option>
                    <option value="zoom">{t("recoveryCenter.rebuildZoom")}</option>
                  </select>
                </label>
                <button className="btn" type="button" disabled={busy} onClick={() => void confirm(
                  { action: "rebuild-state", target: rebuildTarget },
                  t("recoveryCenter.confirmRebuildTitle"), t("recoveryCenter.confirmRebuild"), t("recoveryCenter.rebuildQuarantineHint"), t("recoveryCenter.rebuild"),
                )}>{busyAction === "rebuild-state" ? t("recoveryCenter.working") : t("recoveryCenter.rebuild")}</button>
              </RecoveryActionCard>
            </div>
          </section>

          <details className="recovery-center__details">
            <summary>{t("recoveryCenter.evidenceDetails")}</summary>
            <div className="recovery-center__evidence-grid">
              <EvidenceList title={t("recoveryCenter.binariesTitle")} rows={report.binaries.map((item) => `${item.role}: ${item.path} · ${item.exists && item.regular && !item.error ? t("recoveryCenter.ok") : t("recoveryCenter.unavailable")}`)} />
              <EvidenceList title={t("recoveryCenter.sessionsTitle")} rows={report.sessions.map((item) => `${item.path} · ${t("recoveryCenter.files", { count: item.fileCount ?? 0 })}`)} />
              <EvidenceList title={t("recoveryCenter.configTitle")} rows={report.config.checks.map((item) => `${item.scope}: ${item.path} · ${!item.exists ? t("recoveryCenter.missing") : item.valid ? t("recoveryCenter.ok") : t("recoveryCenter.invalid")}`)} />
            </div>
            <p className="recovery-center__generated">{t("recoveryCenter.generatedAt", { time: formatTime(report.generatedAt) })}</p>
          </details>
        </>
      ) : null}
    </section>
  );
}

function StatusCard({ label, value, detail, tone }: { label: string; value: string; detail: string; tone: "normal" | "warning" | "error" }) {
  return <article className="recovery-center__status" data-tone={tone}><span>{label}</span><strong>{value}</strong><small>{detail}</small></article>;
}

function RecoveryActionCard({ icon, title, body, children }: { icon: React.ReactNode; title: string; body: string; children: React.ReactNode }) {
  return <article className="recovery-center__action"><div className="recovery-center__action-title">{icon}<strong>{title}</strong></div><p>{body}</p><div className="recovery-center__action-controls">{children}</div></article>;
}

function EvidenceList({ title, rows }: { title: string; rows: string[] }) {
  return <div><h3>{title}</h3>{rows.length === 0 ? <p>—</p> : <ul>{rows.map((row, index) => <li key={`${row}-${index}`}>{row}</li>)}</ul>}</div>;
}
