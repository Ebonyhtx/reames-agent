import { useCallback, useEffect, useRef, useState } from "react";
import type { KeyboardEvent as ReactKeyboardEvent } from "react";
import gsap from "gsap";
import { useT, type Translator } from "../lib/i18n";
import type { ComposerInsertRequest, DirEntry, WireApproval, WireApprovalPlan } from "../lib/types";
import { PromptAction, PromptBadge, PromptHeaderAction, PromptShelf } from "./PromptShelf";
import { DUR_FAST } from "../lib/gsapAnimations";
import { fileDiffFromWire, summarizeFileDiff } from "../lib/tools";
import {
  FileReferenceMenu,
  insertTextAtSelection,
  pickInlineFileReference,
  useFileReferenceMenu,
} from "./FileReferenceMenu";
import { DiffView } from "./DiffView";

function animateShelfExit(
  el: HTMLDivElement,
  options: { opacity: number; y: number; duration: number; ease: string; onComplete: () => void },
) {
  const animator = typeof gsap.to === "function"
    ? gsap
    : (gsap as unknown as { default?: typeof gsap }).default;
  if (animator && typeof animator.to === "function") {
    animator.to(el, options);
    return;
  }
  options.onComplete();
}

function requiresFreshHumanApproval(tool: string): boolean {
  return tool === "remember" || tool === "forget" || tool === "exit_plan_mode" || tool === "sandbox_escape" || tool === "install_source";
}

function approvalToolLabel(tool: string, t: Translator): string {
  switch (tool) {
    case "bash":
      return t("approval.toolLabelBash");
    case "edit_file":
      return t("approval.toolLabelEditFile");
    case "write_file":
      return t("approval.toolLabelWriteFile");
    case "multi_edit":
      return t("approval.toolLabelMultiEdit");
    case "move_file":
      return t("approval.toolLabelMoveFile");
    case "web_fetch":
      return t("approval.toolLabelWebFetch");
    case "run_skill":
      return t("approval.toolLabelRunSkill");
    case "remember":
      return t("approval.toolLabelRemember");
    case "forget":
      return t("approval.toolLabelForget");
    case "sandbox_escape":
      return t("approval.toolLabelSandboxEscape");
    case "plan_mode_read_only_command":
      return t("approval.toolLabelPlanModeReadOnly");
    case "install_source":
      return t("approval.toolLabelInstallSource");
    default:
      return tool;
  }
}

function InstallApprovalPlan({ plan, t }: { plan: WireApprovalPlan; t: Translator }) {
  const formatMap = (values?: Record<string, string>) => Object.entries(values ?? {})
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([key, value]) => `${key}=${value}`)
    .join(", ");
  return (
    <div className="approval-install-plan" data-automation-id="install-source-approval-plan">
      <div className="approval-install-plan__header">
        <strong>{t("approval.installPlan")}</strong>
        <span><span>{t("approval.planId")}</span> <code>{plan.planId}</code></span>
      </div>
      <dl className="approval-install-plan__meta">
        <div><dt>{t("approval.operation")}</dt><dd>{plan.operation}</dd></div>
        {plan.name && <div><dt>{t("approval.name")}</dt><dd>{plan.name}</dd></div>}
        {plan.kind && <div><dt>{t("approval.kind")}</dt><dd>{plan.kind}</dd></div>}
        {plan.scope && <div><dt>{t("approval.scope")}</dt><dd>{plan.scope}</dd></div>}
        {plan.mode && <div><dt>{t("approval.mode")}</dt><dd>{plan.mode}</dd></div>}
        {plan.source && <div><dt>{t("approval.source")}</dt><dd>{plan.source}</dd></div>}
      </dl>
      <div className="approval-install-plan__actions">
        {plan.actions.map((action, index) => {
          const permissions = action.permissions ?? [];
          const addedPermissions = action.addedPermissions ?? [];
          const removedPermissions = action.removedPermissions ?? [];
          return (
            <div className="approval-install-action" key={`${action.action}:${action.name ?? action.target ?? index}`}>
              <div className="approval-install-action__title">
                <span>{index + 1}. {action.kind} / {action.action}</span>
                <span className={`approval-risk approval-risk--${action.riskLevel || "unknown"}`}>{action.riskLevel || "unknown"}</span>
              </div>
              {(action.name || action.target) && (
                <div className="approval-install-action__line">
                  <span>{t("approval.target")}</span>
                  <code>{action.target || action.name}</code>
                </div>
              )}
              {action.source && <div className="approval-install-action__line"><span>{t("approval.actionSource")}</span><code>{action.source}</code></div>}
              {action.configPath && <div className="approval-install-action__line"><span>{t("approval.configPath")}</span><code>{action.configPath}</code></div>}
              {action.scope && <div className="approval-install-action__line"><span>{t("approval.actionScope")}</span><span>{action.scope}</span></div>}
              {action.mode && <div className="approval-install-action__line"><span>{t("approval.mode")}</span><span>{action.mode}</span></div>}
              {action.transport && <div className="approval-install-action__line"><span>{t("approval.transport")}</span><span>{action.transport}</span></div>}
              {action.url && <div className="approval-install-action__line"><span>{t("approval.url")}</span><code>{action.url}</code></div>}
              {(action.command || (action.args?.length ?? 0) > 0) && <div className="approval-install-action__line"><span>{t("approval.command")}</span><code>{[action.command, ...(action.args ?? [])].filter(Boolean).join(" ")}</code></div>}
              {Object.keys(action.env ?? {}).length > 0 && <div className="approval-install-action__line"><span>{t("approval.environment")}</span><code>{formatMap(action.env)}</code></div>}
              {Object.keys(action.headers ?? {}).length > 0 && <div className="approval-install-action__line"><span>{t("approval.headers")}</span><code>{formatMap(action.headers)}</code></div>}
              {(action.currentVersion || action.version) && (
                <div className="approval-install-action__line"><span>{t("approval.version")}</span><code>{[action.currentVersion, action.version].filter(Boolean).join(" -> ")}</code></div>
              )}
              {(action.currentDigest || action.digest) && (
                <div className="approval-install-action__line"><span>{t("approval.digest")}</span><code>{[action.currentDigest, action.digest].filter(Boolean).join(" -> ")}</code></div>
              )}
              {action.sourceRevision && (
                <div className="approval-install-action__line"><span>{t("approval.sourceRevision")}</span><code>{action.sourceRevision}</code></div>
              )}
              {action.sourceKind && <div className="approval-install-action__line"><span>{t("approval.sourceKind")}</span><span>{action.sourceKind}</span></div>}
              {action.trustStatus && (
                <div className="approval-install-action__line"><span>{t("approval.trust")}</span><span>{action.trustStatus}</span></div>
              )}
              {action.registryName && (
                <div className="approval-install-action__line"><span>{t("approval.registryName")}</span><span>{action.registryName}</span></div>
              )}
              {action.registryMetadataUrl && (
                <div className="approval-install-action__line"><span>{t("approval.registryMetadataUrl")}</span><code>{action.registryMetadataUrl}</code></div>
              )}
              {action.registryRootVersion != null && (
                <div className="approval-install-action__line"><span>{t("approval.registryRootVersion")}</span><code>{action.registryRootVersion}</code></div>
              )}
              {action.registryRootDigest && (
                <div className="approval-install-action__line"><span>{t("approval.registryRootDigest")}</span><code>{action.registryRootDigest}</code></div>
              )}
              {action.registryEntryDigest && (
                <div className="approval-install-action__line"><span>{t("approval.registryEntryDigest")}</span><code>{action.registryEntryDigest}</code></div>
              )}
              {action.provenanceStatus && (
                <div className="approval-install-action__line"><span>{t("approval.provenanceStatus")}</span><span>{action.provenanceStatus}</span></div>
              )}
              {action.attestationDigest && (
                <div className="approval-install-action__line"><span>{t("approval.attestationDigest")}</span><code>{action.attestationDigest}</code></div>
              )}
              {action.kind === "plugin" && (
                <div className="approval-install-action__line"><span>{t("approval.activeAfterApply")}</span><span>{t(action.willEnable ? "approval.yes" : "approval.no")}</span></div>
              )}
              {permissions.length > 0 && (
                <div className="approval-install-action__line"><span>{t("approval.permissions")}</span><code>{permissions.join(", ")}</code></div>
              )}
              {addedPermissions.length > 0 && (
                <div className="approval-install-action__line"><span>{t("approval.addedPermissions")}</span><code>{addedPermissions.join(", ")}</code></div>
              )}
              {removedPermissions.length > 0 && (
                <div className="approval-install-action__line"><span>{t("approval.removedPermissions")}</span><code>{removedPermissions.join(", ")}</code></div>
              )}
              {(action.riskReasons?.length ?? 0) > 0 && (
                <div className="approval-install-action__line"><span>{t("approval.risk")}</span><span>{action.riskReasons?.join("; ")}</span></div>
              )}
            </div>
          );
        })}
      </div>
      {(plan.warnings?.length ?? 0) > 0 && (
        <div className="approval-install-plan__warnings"><strong>{t("approval.warnings")}</strong><span>{plan.warnings?.join("; ")}</span></div>
      )}
    </div>
  );
}

const sandboxEscapeEnglishSubjectFallback = "run shell command unconfined once";
const sandboxEscapeEnglishSubjectPrefix = "run unconfined once: ";
const planModeMcpEnglishSubject = /^MCP (.+) as read-only for planning and research$/;
const planModeBashEnglishSubject = /^Trust (.+) as a read-only command prefix while planning\r?\nCommand: ([\s\S]+)$/;

function localizeApprovalSubject(tool: string, subject: string, t: Translator): string {
  const trimmed = subject.trim();
  if (tool === "sandbox_escape") {
    if (!trimmed || trimmed === sandboxEscapeEnglishSubjectFallback) return t("approval.sandboxEscapeSubjectFallback");
    if (trimmed.startsWith(sandboxEscapeEnglishSubjectPrefix)) {
      return `${t("approval.sandboxEscapeSubjectPrefix")}${trimmed.slice(sandboxEscapeEnglishSubjectPrefix.length)}`;
    }
    return trimmed;
  }
  if (tool === "remember") {
    return trimmed
      .replace(/^Save\/update memory/, t("approval.memorySaveUpdate"))
      .replace(/\bbody: /g, `${t("approval.memoryBodyLabel")}: `);
  }
  if (tool === "forget" && trimmed.startsWith("Archive memory ")) {
    return `${t("approval.memoryArchivePrefix")}${trimmed.slice("Archive memory ".length)}`;
  }
  const mcpTrust = trimmed.match(planModeMcpEnglishSubject);
  if (mcpTrust) {
    return t("approval.planModeMcpTrustSubject", { target: mcpTrust[1] ?? "" });
  }
  const bashTrust = trimmed.match(planModeBashEnglishSubject);
  if (bashTrust) {
    return t("approval.planModeBashTrustSubject", { prefix: bashTrust[1] ?? "", command: bashTrust[2] ?? "" });
  }
  return trimmed;
}

function localizeApprovalReason(tool: string, reason: string | undefined, t: Translator): string {
  const trimmed = reason?.trim() ?? "";
  if (tool !== "sandbox_escape") return trimmed;
  if (trimmed.includes("could not wrap this command")) return t("approval.sandboxEscapeWrapReason");
  if (trimmed.includes("failed while starting this command") || trimmed.includes("Run this command unconfined once?")) {
    return t("approval.sandboxEscapeRuntimeReason");
  }
  return trimmed || t("approval.sandboxEscapeRuntimeReason");
}

function localizePlanModeApprovalReason(tool: string, reason: string, t: Translator): string {
  if (tool === "plan_mode_read_only_command" && reason.includes("built-in read-only set")) {
    return t("approval.planModeBashTrustReason");
  }
  if (reason.includes("external read-only hints need your confirmation")) {
    return t("approval.planModeMcpTrustReason");
  }
  return reason;
}

export function ApprovalModal({
  approval,
  onAnswer,
  onRevisePlan,
  onExitPlan,
  onStop,
  cwd,
  insertRequest,
  onRevisionActiveChange,
}: {
  approval: WireApproval;
  onAnswer: (allow: boolean, session: boolean, persist: boolean) => void;
  onRevisePlan?: (text: string) => void;
  onExitPlan?: () => void;
  onStop: () => void;
  cwd?: string;
  insertRequest?: ComposerInsertRequest | null;
  onRevisionActiveChange?: (active: boolean) => void;
}) {
  const t = useT();
  const isPlanApproval = approval.tool === "exit_plan_mode";
  const toolLabel = approvalToolLabel(approval.tool, t);
  const isFreshHumanApproval = requiresFreshHumanApproval(approval.tool);
  const hasFreshSessionGrant = approval.tool === "sandbox_escape";
  const subject = localizeApprovalSubject(approval.tool, approval.subject, t);
  const reason = localizePlanModeApprovalReason(approval.tool, localizeApprovalReason(approval.tool, approval.reason, t), t);
  const subjectSummary = subject.split(/\r?\n/).find((line) => line.trim())?.trim() ?? "";
  const toolMeta = reason || subjectSummary || approval.tool;
  const approvalFileDiff = fileDiffFromWire(approval);
  const hasToolDetails = Boolean(reason || subject || approval.plan || approvalFileDiff?.diff);
  const showToolDetailsByDefault = !isPlanApproval && hasToolDetails;
  const [revisionOpen, setRevisionOpen] = useState(false);
  const [revisionText, setRevisionText] = useState("");
  const [detailsOpen, setDetailsOpen] = useState(() => showToolDetailsByDefault);
  const [selectedIndex, setSelectedIndex] = useState(() => (isPlanApproval ? 1 : 0));
  const cardRef = useRef<HTMLDivElement | null>(null);
  const shelfRef = useRef<HTMLDivElement | null>(null);
  const inputRef = useRef<HTMLTextAreaElement | null>(null);
  const consumedInsertIdRef = useRef(0);
  // When consecutive approvals arrive, animate the old card out before
  // the new one slides in.  GSAP fromTo on the shelf wrapper avoids the
  // jarring pop when the API cycles through 4+ pending approvals.
  const closingRef = useRef(false);
  const fileMenu = useFileReferenceMenu(revisionText, cwd);

  const answerWithExit = (fn: () => void) => {
    if (closingRef.current) return;
    closingRef.current = true;
    const el = shelfRef.current;
    if (el) {
      animateShelfExit(el, {
        opacity: 0,
        y: 8,
        duration: DUR_FAST,
        ease: "power2.in",
        onComplete: fn,
      });
    } else {
      fn();
    }
  };

  const choosePlanAction = (key: string) => {
    if (key === "1") setRevisionOpen((open) => !open);
    else if (key === "2") answerWithExit(() => onAnswer(true, false, false));
    else if (key === "3") answerWithExit(() => (onExitPlan ?? (() => onAnswer(false, false, false)))());
    else if (key === "Escape") answerWithExit(onStop);
  };

  const chooseToolAction = (key: string) => {
    if (key === "1") answerWithExit(() => onAnswer(true, false, false));
    else if (hasFreshSessionGrant && key === "2") answerWithExit(() => onAnswer(true, true, false));
    else if (hasFreshSessionGrant && key === "3") answerWithExit(() => onAnswer(false, false, false));
    else if (isFreshHumanApproval && key === "2") answerWithExit(() => onAnswer(false, false, false));
    else if (isFreshHumanApproval && key === "4") answerWithExit(() => onAnswer(false, false, false));
    else if (!isFreshHumanApproval && key === "2") answerWithExit(() => onAnswer(true, true, false));
    else if (!isFreshHumanApproval && key === "3") answerWithExit(() => onAnswer(true, true, true));
    else if (!isFreshHumanApproval && key === "4") answerWithExit(() => onAnswer(false, false, false));
    else if (key === "Escape") answerWithExit(onStop);
  };

  const selectedToolActionKey = (index: number) => {
    if (!isFreshHumanApproval) return String(index + 1);
    if (hasFreshSessionGrant) return index === 0 ? "1" : index === 1 ? "2" : "3";
    return index === 0 ? "1" : "2";
  };

  useEffect(() => {
    cardRef.current?.focus();
    setRevisionOpen(false);
    setRevisionText("");
    setDetailsOpen(showToolDetailsByDefault);
    setSelectedIndex(isPlanApproval ? 1 : 0);
  }, [approval.id, isPlanApproval, showToolDetailsByDefault]);

  const actionCount = isPlanApproval ? 3 : isFreshHumanApproval ? (hasFreshSessionGrant ? 3 : 2) : 4;
  const selectedIndexRef = useRef(selectedIndex);
  selectedIndexRef.current = selectedIndex;

  useEffect(() => {
    const onKeyDown = (event: globalThis.KeyboardEvent) => {
      const target = event.target instanceof Element ? event.target : null;
      const tag = target?.tagName.toLowerCase();
      if (tag === "input" || tag === "textarea" || tag === "select" || (target instanceof HTMLElement && target.isContentEditable)) return;
      const interactiveTarget = target?.closest("button, a, [role='button'], [role='link']");
      if (interactiveTarget && (event.key === "ArrowLeft" || event.key === "ArrowRight" || event.key === "Enter")) return;
      if (event.key === "ArrowLeft") {
        event.preventDefault();
        setSelectedIndex((i) => (i - 1 + actionCount) % actionCount);
      } else if (event.key === "ArrowRight") {
        event.preventDefault();
        setSelectedIndex((i) => (i + 1) % actionCount);
      } else if (event.key === "Enter") {
        event.preventDefault();
        const key = String(selectedIndexRef.current + 1);
        if (isPlanApproval) choosePlanAction(key);
        else chooseToolAction(selectedToolActionKey(selectedIndexRef.current));
      } else if (event.key === "1" || event.key === "2" || event.key === "3" || event.key === "4" || event.key === "Escape") {
        event.preventDefault();
        if (isPlanApproval) choosePlanAction(event.key);
        else chooseToolAction(event.key);
      }
    };
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [isPlanApproval, isFreshHumanApproval, hasFreshSessionGrant, onAnswer, onExitPlan, onStop, actionCount]);

  useEffect(() => {
    if (revisionOpen) {
      onRevisionActiveChange?.(true);
      inputRef.current?.focus();
      return () => onRevisionActiveChange?.(false);
    }
    onRevisionActiveChange?.(false);
  }, [revisionOpen, onRevisionActiveChange]);

  const focusRevisionInput = (caret = revisionText.length) => {
    requestAnimationFrame(() => {
      const input = inputRef.current;
      if (!input) return;
      input.focus();
      input.setSelectionRange(caret, caret);
    });
  };

  const insertRevisionText = useCallback((text: string) => {
    const input = inputRef.current;
    const start = input?.selectionStart ?? revisionText.length;
    const end = input?.selectionEnd ?? start;
    const next = insertTextAtSelection(revisionText, text, start, end);
    setRevisionText(next.value);
    focusRevisionInput(next.caret);
  }, [revisionText]);

  useEffect(() => {
    if (!insertRequest || insertRequest.id === consumedInsertIdRef.current) return;
    consumedInsertIdRef.current = insertRequest.id;
    insertRevisionText(insertRequest.text);
  }, [insertRequest, insertRevisionText]);

  const pickRevisionFile = (entry: DirEntry) => {
    const next = pickInlineFileReference(revisionText, fileMenu.atRaw, fileMenu.atDir, entry);
    setRevisionText(next);
    focusRevisionInput(next.length);
  };

  const onRevisionKeyDown = (event: ReactKeyboardEvent<HTMLTextAreaElement>) => {
    if ((event.metaKey || event.ctrlKey) && event.key === "Enter") {
      submitRevision();
      event.stopPropagation();
      return;
    }
    if (fileMenu.open) {
      if (event.key === "ArrowDown" && fileMenu.count > 0) {
        event.preventDefault();
        fileMenu.setActive((index) => (index + 1) % fileMenu.count);
        return;
      }
      if (event.key === "ArrowUp" && fileMenu.count > 0) {
        event.preventDefault();
        fileMenu.setActive((index) => (index - 1 + fileMenu.count) % fileMenu.count);
        return;
      }
      if ((event.key === "Enter" || event.key === "Tab") && fileMenu.count > 0) {
        event.preventDefault();
        const entry = fileMenu.items[fileMenu.active];
        if (entry) pickRevisionFile(entry);
        return;
      }
      if (event.key === "Escape") {
        event.preventDefault();
        fileMenu.dismiss();
        return;
      }
    }
    event.stopPropagation();
  };

  const submitRevision = () => {
    const text = revisionText.trim();
    if (!text) {
      inputRef.current?.focus();
      return;
    }
    answerWithExit(() => onRevisePlan?.(text));
  };

  // The plan is already shown above as the assistant's reply; this is just the gate.
  if (isPlanApproval) {
    return (
      <div ref={shelfRef}>
        <PromptShelf
          className="prompt-shelf--compact prompt-shelf--plan-approval"
          barRef={cardRef}
          titleId="plan-approval-title"
          title={t("approval.planReady")}
          meta={t("approval.planReadyHint")}
          badges={revisionOpen ? <PromptBadge>{t("approval.revisePlan")}</PromptBadge> : undefined}
          headerActions={
            <PromptHeaderAction onClick={() => answerWithExit(onStop)} ariaLabel={t("composer.stopShort")}>
              Esc
            </PromptHeaderAction>
          }
          actions={
            <>
              <PromptAction keyLabel="1" label={t("approval.revisePlan")} onClick={() => setRevisionOpen((open) => !open)} selected={selectedIndex === 0} />
              <PromptAction keyLabel="2" label={t("approval.startExecution")} onClick={() => answerWithExit(() => onAnswer(true, false, false))} selected={selectedIndex === 1} />
              <PromptAction
                keyLabel="3"
                label={t("approval.exitPlan")}
                onClick={() => answerWithExit(() => (onExitPlan ?? (() => onAnswer(false, false, false)))())}
                selected={selectedIndex === 2}
              />
            </>
          }
        >
          {revisionOpen && (
            <div className="plan-revision">
              <textarea
                ref={inputRef}
                className="plan-revision__input"
                value={revisionText}
                rows={3}
                placeholder={t("approval.revisePlanPlaceholder")}
                onChange={(event) => setRevisionText(event.target.value)}
                onFocus={() => onRevisionActiveChange?.(true)}
                onKeyDown={onRevisionKeyDown}
              />
              {fileMenu.open && (
                <FileReferenceMenu
                  items={fileMenu.items}
                  activeIndex={fileMenu.active}
                  onPick={pickRevisionFile}
                  onHover={fileMenu.setActive}
                />
              )}
              <div className="plan-revision__actions">
                <button className="btn" onClick={() => setRevisionOpen(false)}>
                  {t("common.cancel")}
                </button>
                <button className="btn btn--primary" onClick={submitRevision}>
                  {t("approval.sendRevision")}
                </button>
              </div>
            </div>
          )}
        </PromptShelf>
      </div>
    );
  }

  return (
    <div ref={shelfRef}>
      <PromptShelf
        automationId="tool-approval-dialog"
        className="prompt-shelf--compact prompt-shelf--tool-approval"
        barRef={cardRef}
        titleId="tool-approval-title"
        title={t("approval.toolPending")}
        badges={<PromptBadge>{toolLabel}</PromptBadge>}
        meta={toolMeta}
        headerActions={
          <>
            {hasToolDetails && (
              <PromptHeaderAction onClick={() => setDetailsOpen((open) => !open)}>
                {t(detailsOpen ? "approval.hideDetails" : "approval.details")}
              </PromptHeaderAction>
            )}
            <PromptHeaderAction onClick={() => answerWithExit(onStop)} ariaLabel={t("composer.stopShort")}>
              Esc
            </PromptHeaderAction>
          </>
        }
        actions={
          <>
            <PromptAction keyLabel="1" label={t("approval.allowOnce")} onClick={() => answerWithExit(() => onAnswer(true, false, false))} selected={selectedIndex === 0} />
            {isFreshHumanApproval ? (
              hasFreshSessionGrant ? (
                <>
                  <PromptAction keyLabel="2" label={t("approval.allowSandboxEscapeSession")} onClick={() => answerWithExit(() => onAnswer(true, true, false))} selected={selectedIndex === 1} />
                  <PromptAction automationId="tool-approval-deny" keyLabel="3" label={t("approval.deny")} onClick={() => answerWithExit(() => onAnswer(false, false, false))} selected={selectedIndex === 2} />
                </>
              ) : (
                <PromptAction automationId="tool-approval-deny" keyLabel="2" label={t("approval.deny")} onClick={() => answerWithExit(() => onAnswer(false, false, false))} selected={selectedIndex === 1} />
              )
            ) : (
              <>
                <PromptAction keyLabel="2" label={t("approval.allowRuleSession")} onClick={() => answerWithExit(() => onAnswer(true, true, false))} selected={selectedIndex === 1} />
                <PromptAction keyLabel="3" label={t("approval.allowRulePersistent")} onClick={() => answerWithExit(() => onAnswer(true, true, true))} selected={selectedIndex === 2} />
                <PromptAction automationId="tool-approval-deny" keyLabel="4" label={t("approval.deny")} onClick={() => answerWithExit(() => onAnswer(false, false, false))} selected={selectedIndex === 3} />
              </>
            )}
          </>
        }
      >
        {detailsOpen && (
          <div className="approval-details">
            {reason && <div className="approval-reason">{reason}</div>}
            {subject && (
              <pre className="approval-subject">{subject}</pre>
            )}
            {approval.plan && <InstallApprovalPlan plan={approval.plan} t={t} />}
            {approvalFileDiff?.diff && (
              <div className="approval-diff">
                <div className="approval-diff__header">
                  <span>{t("approval.patchPreview")}</span>
                  <span className="approval-diff__stats">{summarizeFileDiff(approvalFileDiff)}</span>
                </div>
                <DiffView diff={approvalFileDiff.diff} maxHeight={220} />
              </div>
            )}
          </div>
        )}
      </PromptShelf>
    </div>
  );
}
