import { useCallback, useEffect, useMemo, useState } from "react";
import { ShieldCheck, ShieldOff } from "lucide-react";
import { asArray } from "../lib/array";
import { app, openExternal } from "../lib/bridge";
import { useT } from "../lib/i18n";
import { mcpServerLifecycleActions, mcpServerRetryableFromAvailableList } from "../lib/mcpServerLifecycle";
import type { CapabilitiesView, MCPServerInput, PluginHookView, PluginInstallOptions, PluginMCPServerView, PluginOperationAction, PluginOperationView, PluginRegistryEntryView, PluginSkillView, PluginView, ServerView, SkillRootSkillView, SkillRootView, SkillsSettingsView, SkillView, TabMeta } from "../lib/types";
import { InlineConfirmButton } from "./InlineConfirmButton";
import { ResizableDrawer } from "./ResizableDrawer";
import { Tooltip } from "./Tooltip";
import { ModalCloseButton } from "./ModalCloseButton";

// CapabilitiesPanel is the desktop MCP & Skills drawer — the GUI counterpart to
// the CLI's /mcp + /skill, aligning with Claude Code's Customize → Connectors:
// each server shows a connected/failed dot, transport, and tool/prompt/resource
// counts, with add / remove / retry; skills list their scope and run mode.
type CapTab = "servers" | "skills";

type SettingsSnapshot<T> = { key: string; value: T };

let mcpSettingsSnapshot: SettingsSnapshot<ServerView[]> | null = null;
let skillsSettingsSnapshot: SettingsSnapshot<SkillsSettingsView> | null = null;
let pluginsSettingsSnapshot: SettingsSnapshot<PluginView[]> | null = null;

function settingsSnapshotKey(meta: Awaited<ReturnType<typeof app.Meta>> | null | undefined, tabs: TabMeta[] | null | undefined): string {
  const active = tabs?.find((tab) => tab.active);
  const tabID = (active?.id || "").trim();
  const root = (active?.workspaceRoot || active?.workspacePath || active?.cwd || meta?.workspaceRoot || meta?.workspacePath || meta?.cwd || "").trim();
  const channel = (meta?.eventChannel || "").trim();
  return `${channel}|${tabID}|${root}`;
}

export function CapabilitiesPanel({
  onClose,
  initialTab = "servers",
}: {
  onClose: () => void;
  initialTab?: CapTab;
}) {
  const t = useT();
  const [view, setView] = useState<CapabilitiesView | null>(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [adding, setAdding] = useState(false);
  const [editing, setEditing] = useState<string | null>(null);
  const [tab, setTab] = useState<CapTab>(initialTab);
  const [skillQuery, setSkillQuery] = useState("");
  const [expandedSkills, setExpandedSkills] = useState<Set<string>>(() => new Set());
  const [expandedErrors, setExpandedErrors] = useState<Set<string>>(() => new Set());
  const [expandedServers, setExpandedServers] = useState<Set<string>>(() => new Set());
  const [expandedServerTools, setExpandedServerTools] = useState<Set<string>>(() => new Set());

  const reload = useCallback(async () => {
    setView(normalizeCapabilitiesView(await app.Capabilities().catch(() => ({ servers: [], skills: [], skillRoots: [], plugins: [] }))));
  }, []);
  useEffect(() => {
    void reload();
  }, [reload]);
  useEffect(() => {
    if (tab !== "servers" || !view?.servers.some((s) => s.status === "initializing" || s.status === "deferred")) return;
    const id = window.setInterval(() => void reload(), 2500);
    return () => window.clearInterval(id);
  }, [reload, tab, view?.servers]);

  // mutate runs an MCP edit, re-reads the snapshot, and surfaces any failure as an
  // inline banner (a connect error, a missing binary, a bad URL).
  const mutate = async (fn: () => Promise<unknown>) => {
    setBusy(true);
    setErr(null);
    try {
      await fn();
      await reload();
      return true;
    } catch (e) {
      setErr(String((e as Error)?.message ?? e));
      await reload();
      return false;
    } finally {
      setBusy(false);
    }
  };

  const summary = useMemo(() => {
    if (!view) return "";
    return t("caps.summary", {
      connected: view.servers.filter((s) => s.status === "connected").length,
      failed: view.servers.filter((s) => s.status === "failed").length,
      skills: view.skills.length,
    });
  }, [view, t]);

  const filteredSkills = useMemo(() => {
    if (!view) return [];
    const q = skillQuery.trim().toLowerCase();
    if (!q) return view.skills;
    return view.skills.filter((sk) => {
      const text = [sk.name, `/${sk.name}`, sk.description, sk.scope, sk.runAs].join(" ").toLowerCase();
      return text.includes(q);
    });
  }, [view, skillQuery]);
  const skillSummary = useMemo(() => {
    if (!view) return "";
    return skillListSummary(view.skills, filteredSkills, skillQuery.trim().length > 0, t);
  }, [filteredSkills, skillQuery, t, view]);

  const serverGroups = useMemo(() => {
    const servers = sortServersForDisplay(view?.servers ?? []);
    return {
      failed: servers.filter((s) => s.status === "failed"),
      active: servers.filter((s) => s.status !== "failed"),
    };
  }, [view]);
  const retryableActiveServerNames = useMemo(() => retryableAvailableServerNames(serverGroups.active), [serverGroups.active]);
  const toggleSkill = useCallback((name: string) => {
    setExpandedSkills((prev) => {
      const next = new Set(prev);
      if (next.has(name)) next.delete(name);
      else next.add(name);
      return next;
    });
  }, []);

  const toggleError = useCallback((name: string) => {
    setExpandedErrors((prev) => {
      const next = new Set(prev);
      if (next.has(name)) next.delete(name);
      else next.add(name);
      return next;
    });
  }, []);

  const toggleServer = useCallback((name: string) => {
    setExpandedServers((prev) => {
      const next = new Set(prev);
      if (next.has(name)) next.delete(name);
      else next.add(name);
      return next;
    });
  }, []);

  const toggleServerTools = useCallback((name: string) => {
    setExpandedServerTools((prev) => {
      const next = new Set(prev);
      if (next.has(name)) next.delete(name);
      else next.add(name);
      return next;
    });
  }, []);

  return (
    <ResizableDrawer onClose={onClose} subtle>
        <header className="drawer__head">
          <div>
            <div className="drawer__title">{t("caps.title")}</div>
            {view && <div className="drawer__summary">{summary}</div>}
          </div>
          <div className="drawer__actions">
            <Tooltip label={t("caps.refresh")}>
              <button className="chip" disabled={busy} onClick={() => void reload()}>
                ↻
              </button>
            </Tooltip>
            <ModalCloseButton label={t("common.close")} onClick={onClose} />
          </div>
        </header>

        {!view ? (
          <div className="empty">{t("caps.loading")}</div>
        ) : (
          <div className="drawer__body">
            {err && <div className="banner banner--error">{err}</div>}

            <div className="cap-tabs" role="tablist" aria-label={t("caps.title")}>
              <button
                className={`cap-tab${tab === "servers" ? " cap-tab--active" : ""}`}
                role="tab"
                aria-selected={tab === "servers"}
                onClick={() => setTab("servers")}
              >
                {t("caps.connectorsTab")}
              </button>
              <button
                className={`cap-tab${tab === "skills" ? " cap-tab--active" : ""}`}
                role="tab"
                aria-selected={tab === "skills"}
                onClick={() => setTab("skills")}
              >
                {t("caps.skillsTab")}
              </button>
            </div>

            {tab === "servers" ? (
              <section className="mem-section">
                <div className="cap-mcp-toolbar cap-mcp-toolbar--drawer">
                  {!adding && (
                    <button className="btn btn--small" disabled={busy} onClick={() => setAdding(true)}>
                      {t("caps.addServer")}
                    </button>
                  )}
                </div>
                {serverGroups.failed.length > 0 && (
                  <FailedServersNotice
                    servers={serverGroups.failed}
                    expanded={expandedErrors}
                    onToggle={toggleError}
                    onRetry={(name) => void mutate(() => app.ReconnectMCPServer(name))}
                    onRetryMany={(names) => void mutate(() => Promise.allSettled(names.map((name) => app.ReconnectMCPServer(name))))}
                    onConfirmClearAuth={(name) => void mutate(() => app.ClearMCPServerAuthentication(name))}
                    onConfirm={(name) => void mutate(() => app.RemoveMCPServer(name))}
                    onConfirmMany={(names) => void mutate(() => Promise.allSettled(names.map((name) => app.RemoveMCPServer(name))))}
                    busy={busy}
                  />
                )}
                {view.servers.length === 0 && !adding && (
                  <div className="mem-empty">{t("caps.noServers")}</div>
                )}
                {serverGroups.active.length > 0 && (
                  <div className="cap-server-section">
                    <div className="cap-server-section__head">
                      <div className="cap-server-section__title">{t("caps.availableServers")}</div>
                      <button
                        className="btn btn--small"
                        disabled={busy || retryableActiveServerNames.length === 0}
                        type="button"
                        onClick={() => void mutate(() => Promise.allSettled(retryableActiveServerNames.map((name) => app.ReconnectMCPServer(name))))}
                      >
                        {t("caps.retryAll")}
                      </button>
                    </div>
                    <ServerGroup
                      busy={busy}
                      servers={serverGroups.active}
                      expanded={expandedServers}
                      expandedTools={expandedServerTools}
                      editing={editing}
                      onConfirm={(name) => void mutate(() => app.RemoveMCPServer(name))}
                      onEdit={(name) => {
                        setEditing(name);
                      }}
                      onCancelEdit={() => setEditing(null)}
                      onRetry={(name) => void mutate(() => app.ReconnectMCPServer(name))}
                      onReconnect={(name) => void mutate(() => app.ReconnectMCPServer(name))}
                      onConfirmClearAuth={(name) => void mutate(() => app.ClearMCPServerAuthentication(name))}
                      onTrustTool={(name, toolName) => void mutate(() => app.TrustMCPServerTool(name, toolName))}
                      onTrustTools={(name, toolNames) => void mutate(() => app.TrustMCPServerTools(name, toolNames))}
                      onUntrustTool={(name, toolName) => void mutate(() => app.UntrustMCPServerTool(name, toolName))}
                      onToggle={(name, on) => void mutate(() => app.SetMCPServerEnabled(name, on))}
                      onUpdate={(name, input) =>
                        void mutate(() => app.UpdateMCPServer(name, input)).then((ok) => {
                          if (ok) setEditing(null);
                        })
                      }
                      onToggleDetails={toggleServer}
                      onToggleTools={toggleServerTools}
                    />
                  </div>
                )}
                {adding ? (
                  <AddServerForm busy={busy} onCancel={() => setAdding(false)} onAdd={async (input) => (await mutate(() => app.AddMCPServer(input))) && setAdding(false)} />
                ) : null}
              </section>
            ) : (
              <section className="mem-section">
                <div className="cap-search">
                  <input
                    className="mem-input"
                    type="search"
                    placeholder={t("caps.searchSkills")}
                    value={skillQuery}
                    onChange={(e) => setSkillQuery(e.target.value)}
                  />
                </div>
                <SkillSources
                  roots={view.skillRoots ?? []}
                  busy={busy}
                  onAdd={() => mutate(async () => {
                    const path = await app.PickSkillFolder();
                    if (path) await app.AddSkillPath(path);
                  })}
                  onRefresh={() => mutate(() => app.RefreshSkills())}
                  onRemove={(path) => mutate(() => app.RemoveSkillPath(path))}
                />
                <div className="cap-skills-head">
                  <div className="cap-skills-head__copy">
                    <div className="cap-skills-head__title">{t("caps.skills")}</div>
                    <div className="cap-skills-head__summary">{skillSummary}</div>
                  </div>
                </div>
                {view.skills.length === 0 ? (
                  <div className="mem-empty">{t("caps.noSkills")}</div>
                ) : filteredSkills.length === 0 ? (
                  <div className="mem-empty">{t("caps.noSkillMatches")}</div>
                ) : (
                  <div className="cap-skills">
                    {filteredSkills.map((sk) => (
                      <SkillRow
                        key={sk.name}
                        skill={sk}
                        busy={busy}
                        expanded={expandedSkills.has(sk.name)}
                        onToggle={() => toggleSkill(sk.name)}
                        onToggleEnabled={(enabled) => void mutate(() => app.SetSkillEnabled(sk.name, enabled))}
                      />
                    ))}
                  </div>
                )}
              </section>
            )}
          </div>
        )}
    </ResizableDrawer>
  );
}

function normalizeCapabilitiesView(view: CapabilitiesView | null | undefined): CapabilitiesView {
  return {
    servers: normalizeServerViews(view?.servers),
    plugins: asArray(view?.plugins),
    ...normalizeSkillsSettingsView(view),
  };
}

function normalizeServerViews(servers: ServerView[] | null | undefined): ServerView[] {
  return sortServersForDisplay(
    asArray(servers).map((server) => ({
      ...server,
      args: asArray(server.args),
      envKeys: asArray(server.envKeys),
      headerKeys: asArray(server.headerKeys),
      toolList: asArray(server.toolList),
      trustedReadOnlyTools: asArray(server.trustedReadOnlyTools),
    })),
  );
}

function normalizeSkillsSettingsView(view: SkillsSettingsView | CapabilitiesView | null | undefined): SkillsSettingsView {
  return {
    skills: asArray(view?.skills),
    skillRoots: asArray(view?.skillRoots).map((root) => ({
      ...root,
      removable: Boolean(root.removable),
      skillItems: asArray(root.skillItems),
    })),
  };
}

function sortServersForDisplay(servers: ServerView[]): ServerView[] {
  return [...servers].sort((a, b) => {
    const priority = serverDisplayPriority(a) - serverDisplayPriority(b);
    if (priority !== 0) return priority;
    return a.name.localeCompare(b.name, undefined, { sensitivity: "base" });
  });
}

function serverDisplayPriority(server: ServerView): number {
  if (server.status === "failed" || server.authStatus === "required") return 0;
  if (server.builtIn) return 1;
  if (server.status !== "disabled") return 2;
  return 3;
}

function skillListSummary(skills: SkillView[], filtered: SkillView[], searching: boolean, t: ReturnType<typeof useT>): string {
  if (searching) {
    return t("caps.skillsSummaryMatches", { matched: filtered.length, total: skills.length });
  }
  const parts = [t("caps.skillsSummaryAvailable", { skills: skills.length })];
  const scopes = ["project", "custom", "global", "builtin"];
  for (const scope of scopes) {
    const count = skills.filter((skill) => skill.scope === scope).length;
    if (count > 0) parts.push(skillScopeSummary(scope, count, t));
  }
  return parts.join(" · ");
}

function mcpServerSummary(servers: ServerView[], t: ReturnType<typeof useT>): string {
  return t("caps.mcpSummary", {
    connected: servers.filter((s) => s.status === "connected").length,
    failed: servers.filter((s) => s.status === "failed").length,
    tools: servers.reduce((total, server) => total + (server.tools || 0), 0),
  });
}

function skillScopeSummary(scope: string, count: number, t: ReturnType<typeof useT>): string {
  switch (scope) {
    case "builtin":
      return t("caps.skillsSummaryBuiltin", { count });
    case "project":
      return t("caps.skillsSummaryProject", { count });
    case "custom":
      return t("caps.skillsSummaryCustom", { count });
    case "global":
      return t("caps.skillsSummaryGlobal", { count });
    default:
      return `${count} ${scope}`;
  }
}

function skillSourceSummary(active: number, missing: number, empty: number, t: ReturnType<typeof useT>): string {
  const parts: string[] = [];
  if (active > 0) parts.push(t("caps.sourcesSummaryActive", { active }));
  if (missing > 0) parts.push(t("caps.sourcesSummaryMissing", { missing }));
  if (empty > 0) parts.push(t("caps.sourcesSummaryEmpty", { empty }));
  return parts.length > 0 ? parts.join(" · ") : t("caps.sourcesSummaryNone");
}

function SkillSources({
  roots,
  busy,
  onAdd,
  onRefresh,
  onRemove,
}: {
  roots: SkillRootView[];
  busy: boolean;
  onAdd: () => void;
  onRefresh: () => void;
  onRemove: (path: string) => void;
}) {
  const t = useT();
  const [expanded, setExpanded] = useState(false);
  const [showDiagnostics, setShowDiagnostics] = useState(false);
  const [expandedRootSkills, setExpandedRootSkills] = useState<Set<string>>(() => new Set());
  const [fullRootSkills, setFullRootSkills] = useState<Set<string>>(() => new Set());
  const primaryRoots = roots.filter(isPrimarySkillRoot);
  const diagnosticRoots = roots.filter((root) => !isPrimarySkillRoot(root));
  const diagnosticsVisible = expanded && showDiagnostics;
  const shownRoots = diagnosticsVisible ? [...primaryRoots, ...diagnosticRoots] : primaryRoots;
  const summaryRoots = diagnosticsVisible ? roots : primaryRoots;
  const active = summaryRoots.filter((root) => root.skills > 0).length;
  const missing = summaryRoots.filter((root) => root.status === "missing").length;
  const empty = summaryRoots.filter((root) => root.status === "ok" && root.skills === 0).length;
  const toggleRootSkills = (key: string) => {
    setExpandedRootSkills((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  };
  const toggleRootSkillFull = (key: string) => {
    setFullRootSkills((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  };
  return (
    <div className={`cap-sources${expanded ? " cap-sources--expanded" : ""}`}>
      <div className="cap-sources__head">
        <div className="cap-sources__copy">
          <div className="cap-sources__title">{t("caps.sources")}</div>
          <div className="cap-sources__summary">{skillSourceSummary(active, missing, empty, t)}</div>
        </div>
        {!expanded && (
          <div className="cap-sources__actions">
            <button className="btn btn--small" type="button" onClick={() => setExpanded(true)} aria-expanded={expanded}>
              {t("caps.manageSkillSources")}
            </button>
          </div>
        )}
      </div>
      {expanded && (
        <>
          <div className="cap-sources__manage">
            <div className="cap-sources__manage-actions">
              <button className="btn btn--small" disabled={busy} onClick={onRefresh}>
                {t("caps.refreshSkills")}
              </button>
              <button className="btn btn--small" disabled={busy} onClick={onAdd}>
                {t("caps.addSkillFolder")}
              </button>
            </div>
            <button
              className="btn btn--small"
              type="button"
              onClick={() => {
                setShowDiagnostics(false);
                setExpanded(false);
              }}
              aria-expanded={expanded}
            >
              {t("common.collapse")}
            </button>
          </div>
          {shownRoots.length === 0 ? (
            <div className="mem-empty">{t("caps.noSkillRoots")}</div>
          ) : (
            <div className="cap-source-list">
              {shownRoots.map((root) => {
                const key = skillRootKey(root);
                const rootSkills = root.skillItems ?? [];
                const rootSkillsExpanded = expandedRootSkills.has(key);
                const rootSkillsFull = fullRootSkills.has(key);
                const canShowRootSkills = rootSkills.length > 0;
                const canRemoveRoot = root.removable;
                return (
                  <div className={`cap-source cap-source--${skillRootTone(root)}`} key={key}>
                    <span className={`cap-dot cap-dot--${skillRootDot(root)}`} />
                    <div className="cap-source__text">
                      <div className="cap-source__head">
                        <div className="cap-source__label" title={root.dir}>
                          {skillRootLabel(root)}
                        </div>
                      </div>
                      <div className="cap-source__meta">
                        <span>{skillRootStatus(root, t)}</span>
                        <span>{t("caps.skillRootCount", { skills: root.skills })}</span>
                        {root.configured && <span>{t("caps.skillRootConfigured")}</span>}
                      </div>
                      {(canShowRootSkills || canRemoveRoot) && (
                        <div className="cap-source-actions">
                          <>
                            {canShowRootSkills && (
                              <button
                                className="btn btn--small"
                                disabled={busy}
                                type="button"
                                aria-expanded={rootSkillsExpanded}
                                onClick={() => toggleRootSkills(key)}
                              >
                                {rootSkillsExpanded ? t("caps.hideSkills") : t("caps.showSkills")}
                              </button>
                              )}
                              {canRemoveRoot && (
                                <InlineConfirmButton
                                  label={t("caps.skillRootRemove")}
                                  confirmLabel={t("caps.skillRootConfirmRemove")}
                                  cancelLabel={t("common.cancel")}
                                  disabled={busy}
                                  danger
                                  onConfirm={() => onRemove(root.dir)}
                                />
                              )}
                            </>
                        </div>
                      )}
                      {rootSkillsExpanded && rootSkills.length > 0 && (
                        <SkillRootSkillsList
                          skills={rootSkills}
                          showAll={rootSkillsFull}
                          onToggleAll={() => toggleRootSkillFull(key)}
                        />
                      )}
                      {root.warning && <div className="cap-source__warning">{root.warning}</div>}
                    </div>
                    <div className="cap-source__badges">
                      {skillRootBadges(root, t).map((badge) => (
                        <span className={`cap-source-badge cap-source-badge--${badge.tone}`} key={badge.label}>
                          {badge.label}
                        </span>
                      ))}
                    </div>
                  </div>
                );
              })}
            </div>
          )}
          {diagnosticRoots.length > 0 && (
            <button className="cap-diagnostics" type="button" onClick={() => setShowDiagnostics((v) => !v)}>
              {diagnosticsVisible ? t("caps.hideDiagnostics") : t("caps.showDiagnostics", { count: diagnosticRoots.length })}
            </button>
          )}
        </>
      )}
    </div>
  );
}

const skillRootPreviewLimit = 5;

function SkillRootSkillsList({
  skills,
  showAll,
  onToggleAll,
}: {
  skills: SkillRootSkillView[];
  showAll: boolean;
  onToggleAll: () => void;
}) {
  const t = useT();
  const visible = showAll ? skills : skills.slice(0, skillRootPreviewLimit);
  return (
    <div className="cap-source-skills">
      {visible.map((skill) => (
        <div className="cap-source-skill" key={`${skill.scope}:${skill.name}`}>
          <div className="cap-source-skill__head">
            <span className="cap-source-skill__name">/{skill.name}</span>
            <span className="cap-source-skill__badges">
              <span className={`cap-skill-badge cap-skill-badge--${skill.scope}`}>{skillScopeLabel(skill.scope, t)}</span>
              {skill.runAs === "subagent" && <span className="cap-skill-badge cap-skill-badge--run">{t("caps.subagent")}</span>}
            </span>
          </div>
          {skill.description && <div className="cap-source-skill__desc">{skill.description}</div>}
        </div>
      ))}
      {skills.length > skillRootPreviewLimit && (
        <button className="cap-source-skills__more" type="button" onClick={onToggleAll}>
          {showAll ? t("common.collapse") : t("caps.skillRootShowAllSkills", { count: skills.length })}
        </button>
      )}
    </div>
  );
}

function skillRootKey(root: SkillRootView): string {
  return `${root.scope}:${root.priority}:${root.dir}`;
}

function isPrimarySkillRoot(root: SkillRootView): boolean {
  return root.skills > 0 || root.configured || Boolean(root.warning);
}

function skillRootTone(root: SkillRootView): "active" | "empty" | "problem" {
  if (root.warning || root.status === "inactive" || root.status === "unreadable") return "problem";
  if (root.skills > 0) return "active";
  return "empty";
}

function skillRootDot(root: SkillRootView): "connected" | "disabled" | "failed" {
  const tone = skillRootTone(root);
  if (tone === "active") return "connected";
  if (tone === "empty") return "disabled";
  return "failed";
}

function skillRootStatus(root: SkillRootView, t: ReturnType<typeof useT>): string {
  if (root.status === "ok" && root.skills > 0) return t("caps.skillRootActive");
  if (root.status === "ok") return t("caps.skillRootEmpty");
  return root.status;
}

function skillRootLabel(root: SkillRootView): string {
  return root.dir;
}

function skillRootBadges(root: SkillRootView, t: ReturnType<typeof useT>): Array<{ label: string; tone: "scope" | "builtin" | "configured" | "missing" }> {
  const badges: Array<{ label: string; tone: "scope" | "builtin" | "configured" | "missing" }> = [
    { label: skillScopeLabel(root.scope, t), tone: "scope" },
    root.scope === "custom"
      ? { label: root.configured ? t("caps.skillRootUserConfigured") : t("caps.skillRootConfiguredPath"), tone: "configured" }
      : { label: t("caps.skillRootBuiltinPath"), tone: "builtin" },
  ];
  if (root.status === "missing") {
    badges.push({ label: t("caps.skillRootMissing"), tone: "missing" });
  }
  return badges;
}

function ServerGroup({
  servers,
  expanded,
  expandedTools,
  busy,
  editing,
  onConfirm,
  onEdit,
  onCancelEdit,
  onRetry,
  onReconnect,
  onConfirmClearAuth,
  onTrustTool,
  onTrustTools,
  onUntrustTool,
  onToggle,
  onUpdate,
  onToggleDetails,
  onToggleTools,
}: {
  servers: ServerView[];
  expanded: Set<string>;
  expandedTools: Set<string>;
  busy: boolean;
  editing: string | null;
  onConfirm: (name: string) => void;
  onEdit: (name: string) => void;
  onCancelEdit: () => void;
  onRetry: (name: string) => void;
  onReconnect: (name: string) => void;
  onConfirmClearAuth: (name: string) => void;
  onTrustTool: (name: string, toolName: string) => void;
  onTrustTools: (name: string, toolNames: string[]) => void;
  onUntrustTool: (name: string, toolName: string) => void;
  onToggle: (name: string, on: boolean) => void;
  onUpdate: (name: string, input: MCPServerInput) => void;
  onToggleDetails: (name: string) => void;
  onToggleTools: (name: string) => void;
}) {
  if (servers.length === 0) return null;
  return (
    <div className="cap-server-group">
      {servers.map((s) => (
        <ServerRow
          key={s.name}
          s={s}
          expanded={expanded.has(s.name)}
          toolsExpanded={expandedTools.has(s.name)}
          busy={busy}
          editing={editing === s.name}
          onConfirm={() => onConfirm(s.name)}
          onEdit={() => onEdit(s.name)}
          onCancelEdit={onCancelEdit}
          onRetry={() => onRetry(s.name)}
          onReconnect={() => onReconnect(s.name)}
          onConfirmClearAuth={() => onConfirmClearAuth(s.name)}
          onTrustTool={(toolName) => onTrustTool(s.name, toolName)}
          onTrustTools={(toolNames) => onTrustTools(s.name, toolNames)}
          onUntrustTool={(toolName) => onUntrustTool(s.name, toolName)}
          onToggle={(on) => onToggle(s.name, on)}
          onUpdate={(input) => onUpdate(s.name, input)}
          onToggleDetails={() => onToggleDetails(s.name)}
          onToggleTools={() => onToggleTools(s.name)}
        />
      ))}
    </div>
  );
}

function FailedServersNotice({
  servers,
  expanded,
  busy,
  onToggle,
  onRetry,
  onRetryMany,
  onConfirmClearAuth,
  onConfirm,
  onConfirmMany,
}: {
  servers: ServerView[];
  expanded: Set<string>;
  busy: boolean;
  onToggle: (name: string) => void;
  onRetry: (name: string) => void;
  onRetryMany: (names: string[]) => void;
  onConfirmClearAuth: (name: string) => void;
  onConfirm: (name: string) => void;
  onConfirmMany: (names: string[]) => void;
}) {
  const t = useT();
  const [detailsOpen, setDetailsOpen] = useState(false);
  const [bulkOpen, setBulkOpen] = useState(false);
  const groups = useMemo(() => failureGroups(servers, t), [servers, t]);
  const removableFailures = useMemo(() => servers.filter(canBulkRemoveFailure), [servers]);
  const retryNames = useMemo(() => servers.map((s) => s.name), [servers]);
  return (
    <div className="cap-failures" role="region" aria-label={t("caps.failureTitle", { failed: servers.length })}>
      <div className="cap-failures__head">
        <div>
          <div className="cap-failures__title">{t("caps.failureTitle", { failed: servers.length })}</div>
          <div className="cap-failures__hint">{t("caps.failureHint")}</div>
        </div>
        <div className="cap-failures__actions">
          <button className="btn btn--small" disabled={busy} type="button" onClick={() => setDetailsOpen((v) => !v)} aria-expanded={detailsOpen}>
            {detailsOpen ? t("caps.hideFailureDetails") : t("caps.showFailureDetails")}
          </button>
          <button className="btn btn--small" disabled={busy || retryNames.length === 0} type="button" onClick={() => onRetryMany(retryNames)}>
            {t("caps.retryAll")}
          </button>
          {removableFailures.length > 0 && (
            <button className="btn btn--small" disabled={busy} type="button" onClick={() => setBulkOpen((v) => !v)} aria-expanded={bulkOpen}>
              {t("caps.bulkActions")}
            </button>
          )}
        </div>
      </div>
      <div className="cap-failures__meta">
        <div className="cap-failures__chips" aria-label={t("caps.failureGroups")}>
          {groups.map((group) => (
            <span className="cap-failure-chip" key={group.kind}>{group.label}</span>
          ))}
        </div>
      </div>
      {bulkOpen && removableFailures.length > 0 && (
        <div className="cap-failures__bulk">
          <InlineConfirmButton
            label={t("caps.removeInvalid", { count: removableFailures.length })}
            confirmLabel={t("caps.confirmRemoveInvalid", { count: removableFailures.length })}
            cancelLabel={t("common.cancel")}
            disabled={busy}
            danger
            onConfirm={() => onConfirmMany(removableFailures.map((s) => s.name))}
          />
        </div>
      )}
      {detailsOpen && <div className="cap-failures__list">
        {servers.map((s) => {
          const open = expanded.has(s.name);
          const error = s.error || t("caps.failed");
          const actionLabel = serverActionLabel(s, t);
          const handlePrimaryAction = () => {
            if (shouldOpenAuth(s)) {
              openExternal((s.authUrl || "").trim());
              return;
            }
            onRetry(s.name);
          };
          return (
            <div className="cap-failure" key={s.name}>
              <div className="cap-failure__main">
                <span className="cap-dot cap-dot--failed" />
                <div className="cap-failure__text">
                  <div className="cap-failure__name">{s.name}</div>
                  <div className="cap-failure__summary">{s.authStatus === "required" ? t("caps.authRequiredSummary") : summarizeServerError(error)}</div>
                </div>
              </div>
              <div className="cap-failure__actions">
                <button className="btn btn--small" disabled={busy} onClick={handlePrimaryAction}>
                  {actionLabel}
                </button>
                {canClearAuth(s) && (
                  <InlineConfirmButton
                    label={t("caps.clearAuth")}
                    confirmLabel={t("caps.confirmClearAuth")}
                    cancelLabel={t("common.cancel")}
                    disabled={busy}
                    onConfirm={() => onConfirmClearAuth(s.name)}
                  />
                )}
                <button className="btn btn--small" onClick={() => onToggle(s.name)} aria-expanded={open}>
                  {open ? t("common.collapse") : t("caps.showLog")}
                </button>
                {!s.builtIn && (
                  <InlineConfirmButton
                    label={t("caps.remove")}
                    confirmLabel={t("caps.confirmRemove")}
                    cancelLabel={t("common.cancel")}
                    disabled={busy}
                    danger
                    onConfirm={() => onConfirm(s.name)}
                  />
                )}
              </div>
              {open && (
                <div className="cap-failure__logbox">
                  <div className="cap-failure__logbar">
                    <span>{t("caps.rawLog")}</span>
                    <button className="btn btn--small" onClick={() => void navigator.clipboard?.writeText(error)}>
                      {t("caps.copyLog")}
                    </button>
                  </div>
                  <pre className="cap-failure__log">{error}</pre>
                </div>
              )}
            </div>
          );
        })}
      </div>}
    </div>
  );
}

function ServerRow({
  s,
  expanded,
  toolsExpanded,
  busy,
  editing,
  onConfirm,
  onEdit,
  onCancelEdit,
  onRetry,
  onReconnect,
  onConfirmClearAuth,
  onTrustTool,
  onTrustTools,
  onUntrustTool,
  onToggle,
  onUpdate,
  onToggleDetails,
  onToggleTools,
}: {
  s: ServerView;
  expanded: boolean;
  toolsExpanded: boolean;
  busy: boolean;
  editing: boolean;
  onConfirm: () => void;
  onEdit: () => void;
  onCancelEdit: () => void;
  onRetry: () => void;
  onReconnect: () => void;
  onConfirmClearAuth: () => void;
  onTrustTool: (toolName: string) => void;
  onTrustTools: (toolNames: string[]) => void;
  onUntrustTool: (toolName: string) => void;
  onToggle: (on: boolean) => void;
  onUpdate: (input: MCPServerInput) => void;
  onToggleDetails: () => void;
  onToggleTools: () => void;
}) {
  const t = useT();
  const actionLabel = serverActionLabel(s, t);
  const lifecycle = mcpServerLifecycleActions(s);
  const tools = s.toolList ?? [];
  let sub =
    s.status === "failed"
      ? s.error || t("caps.failed")
      : s.status === "initializing"
        ? t("caps.initializing")
      : s.status === "deferred"
        ? t("caps.deferred")
      : s.status === "disabled"
        ? s.configured && !s.autoStart
          ? t("caps.disabledAutoStart")
          : t("caps.disabled")
        : t("caps.counts", { tools: s.tools, prompts: s.prompts, resources: s.resources });
  if (s.authStatus === "possible" && s.status !== "failed") {
    sub = `${sub} · ${t("caps.authPossibleShort")}`;
  }
  const handlePrimaryAction = () => {
    if (shouldOpenAuth(s)) {
      openExternal((s.authUrl || "").trim());
      return;
    }
    onRetry();
  };
  return (
    <div className={`cap-server-entry${s.status === "disabled" ? " cap-server-entry--disabled" : ""}`}>
      <Tooltip label={s.error} disabled={!s.error} fill block>
        <div className={`cap-row${s.status === "disabled" ? " cap-row--disabled" : ""}`}>
          <Tooltip label={expanded ? t("caps.collapseDetails") : t("caps.expandDetails")}>
            <button
              className="cap-disclosure"
              aria-expanded={expanded}
              onClick={onToggleDetails}
            >
              {expanded ? "⌄" : "›"}
            </button>
          </Tooltip>
          <span className={`cap-dot cap-dot--${s.status}`} />
          <div className="cap-row__text">
            <div className="cap-row__head">
              <span className="cap-row__name">{s.name}</span>
              <span className="cap-row__transport">{s.transport}</span>
              {s.builtIn && <span className="cap-row__builtin">{t("caps.builtIn")}</span>}
            </div>
            <div className="cap-row__sub">{sub}</div>
          </div>
          <div className="cap-row__actions">
            {lifecycle.showRetryInRow ? (
              <button className="btn btn--small" disabled={busy} onClick={handlePrimaryAction}>
                {actionLabel}
              </button>
            ) : (
              <Tooltip label={lifecycle.enabled ? t("caps.disable") : t("caps.enable")}>
                <label className="cap-switch">
                  <input
                    type="checkbox"
                    checked={lifecycle.enabled}
                    disabled={busy}
                    onChange={(e) => onToggle(e.target.checked)}
                  />
                  <span className="cap-switch__track" />
                </label>
              </Tooltip>
            )}
          </div>
        </div>
      </Tooltip>
      {expanded && (
        <ServerDetails
          s={s}
          tools={tools}
          busy={busy}
          onConfirm={onConfirm}
          onConnectNow={onRetry}
          onReconnect={onReconnect}
          onConfirmClearAuth={onConfirmClearAuth}
          onTrustTool={onTrustTool}
          onTrustTools={onTrustTools}
          onUntrustTool={onUntrustTool}
          toolsExpanded={toolsExpanded}
          editing={editing}
          onEdit={onEdit}
          onCancelEdit={onCancelEdit}
          onUpdate={onUpdate}
          onToggleTools={onToggleTools}
        />
      )}
    </div>
  );
}

function ServerDetails({
  s,
  tools,
  busy,
  onConfirm,
  onConnectNow,
  onReconnect,
  onConfirmClearAuth,
  onTrustTool,
  onTrustTools,
  onUntrustTool,
  toolsExpanded,
  editing,
  onEdit,
  onCancelEdit,
  onUpdate,
  onToggleTools,
}: {
  s: ServerView;
  tools: ServerView["toolList"];
  busy: boolean;
  onConfirm: () => void;
  onConnectNow: () => void;
  onReconnect: () => void;
  onConfirmClearAuth: () => void;
  onTrustTool: (toolName: string) => void;
  onTrustTools: (toolNames: string[]) => void;
  onUntrustTool: (toolName: string) => void;
  toolsExpanded: boolean;
  editing: boolean;
  onEdit: () => void;
  onCancelEdit: () => void;
  onUpdate: (input: MCPServerInput) => void;
  onToggleTools: () => void;
}) {
  const t = useT();
  const command = serverCommand(s);
  const canEditConfig = s.configured && !s.builtIn;
  const lifecycle = mcpServerLifecycleActions(s);
  const canConnectNow = lifecycle.canConnectNow;
  const canReconnect = lifecycle.canReconnect;
  const canShowTools = s.status === "connected" && ((s.tools ?? 0) > 0 || (tools?.length ?? 0) > 0);
  const showClearAuth = canClearAuth(s);
  const authLabel = serverAuthLabel(s, t);
  const trustedReadOnlyTools = s.trustedReadOnlyTools ?? [];
  const trustedReadOnlyToolNames = new Set(trustedReadOnlyTools);
  const canTrustTool = s.configured && !s.builtIn;
  const reportedReadOnlyToolNames = (tools ?? []).filter((tool) => tool.readOnlyHint).map((tool) => tool.name);
  const bulkTrustToolNames = reportedReadOnlyToolNames.filter((name) => !trustedReadOnlyToolNames.has(name));
  if (editing && canEditConfig) {
    return (
      <div className="cap-server-details">
        <EditServerForm s={s} busy={busy} onCancel={onCancelEdit} onSave={onUpdate} />
      </div>
    );
  }
  return (
    <div className="cap-server-details">
      <div className="cap-detail-grid">
        <div className="cap-detail">
          <span className="cap-detail__label">{t("caps.status")}</span>
          <span className="cap-detail__value">{serverStatusLabel(s, t)}</span>
        </div>
        <div className="cap-detail">
          <span className="cap-detail__label">{t("caps.transport")}</span>
          <span className="cap-detail__value">{s.transport}</span>
        </div>
        {authLabel && (
          <div className="cap-detail">
            <span className="cap-detail__label">{t("caps.auth")}</span>
            <span className="cap-detail__value">{authLabel}</span>
          </div>
        )}
        {command && (
          <div className="cap-detail cap-detail--wide">
            <span className="cap-detail__label">{s.transport === "stdio" ? t("caps.command") : t("caps.url")}</span>
            <span className="cap-detail__code">{command}</span>
          </div>
        )}
        {s.envKeys && s.envKeys.length > 0 && (
          <div className="cap-detail cap-detail--wide">
            <span className="cap-detail__label">{t("caps.envKeys")}</span>
            <span className="cap-detail__value">{s.envKeys.join(", ")}</span>
          </div>
        )}
        {s.headerKeys && s.headerKeys.length > 0 && (
          <div className="cap-detail cap-detail--wide">
            <span className="cap-detail__label">{t("caps.headerKeys")}</span>
            <span className="cap-detail__value">{s.headerKeys.join(", ")}</span>
          </div>
        )}
        {trustedReadOnlyTools.length > 0 && (
          <div className="cap-detail cap-detail--wide">
            <span className="cap-detail__label">{t("caps.trustedReadOnlyTools")}</span>
            <span className="cap-detail__code">{trustedReadOnlyTools.join(", ")}</span>
          </div>
        )}
      </div>
      <div className="cap-detail-actions">
        {canConnectNow && (
          <button className="btn btn--small" disabled={busy} onClick={onConnectNow}>
            {t("caps.connectNow")}
          </button>
        )}
        {canReconnect && (
          <button className="btn btn--small" disabled={busy} onClick={onReconnect}>
            {t("caps.reconnect")}
          </button>
        )}
        {canShowTools && (
          <button className="btn btn--small" disabled={busy} onClick={onToggleTools} aria-expanded={toolsExpanded}>
            {toolsExpanded ? t("caps.hideTools") : t("caps.showTools")}
          </button>
        )}
        {canTrustTool && bulkTrustToolNames.length > 0 && (
          <button
            className="btn btn--small cap-trust-bulk"
            disabled={busy}
            onClick={() => onTrustTools(bulkTrustToolNames)}
            title={t("caps.trustReportedReadOnlyTitle")}
            type="button"
          >
            <ShieldCheck aria-hidden size={13} strokeWidth={2.2} />
            {t("caps.trustReportedReadOnly", { count: bulkTrustToolNames.length })}
          </button>
        )}
        {showClearAuth && (
          <InlineConfirmButton
            label={t("caps.clearAuth")}
            confirmLabel={t("caps.confirmClearAuth")}
            cancelLabel={t("common.cancel")}
            disabled={busy}
            onConfirm={onConfirmClearAuth}
          />
        )}
        {canEditConfig && (
          <>
            <button className="btn btn--small" disabled={busy} onClick={onEdit}>
              {t("caps.editConfig")}
            </button>
            <InlineConfirmButton
              label={t("caps.remove")}
              confirmLabel={t("caps.confirmRemove")}
              cancelLabel={t("common.cancel")}
              disabled={busy}
              danger
              onConfirm={onConfirm}
            />
          </>
        )}
      </div>
      {toolsExpanded && (
        tools && tools.length > 0 ? (
          <div className="cap-tool-list">
            <div className="cap-tool-list__title">{t("caps.tools")}</div>
            {tools.map((tool) => {
              const trusted = trustedReadOnlyToolNames.has(tool.name);
              return (
                <div className="cap-tool" key={tool.name}>
                  <div className="cap-tool__name">{tool.name}</div>
                  <div className="cap-tool__desc">
                    <span>{tool.description}</span>
                    {tool.readOnlyHint && (
                      <span className="cap-tool-hint" title={t("caps.reportedReadOnlyTitle")}>
                        {t("caps.reportedReadOnly")}
                      </span>
                    )}
                  </div>
                  <div className="cap-tool__action">
                    {canTrustTool ? (
                      trusted ? (
                        <div className="cap-tool-trust-stack">
                          <span className="cap-tool-trust cap-tool-trust--trusted" title={t("caps.trustedReadOnlyTitle")}>
                            <ShieldCheck aria-hidden size={12} strokeWidth={2.2} />
                            {t("caps.trustedReadOnly")}
                          </span>
                          <button
                            className="btn btn--small cap-tool-untrust-btn"
                            disabled={busy}
                            onClick={() => onUntrustTool(tool.name)}
                            title={t("caps.untrustReadOnlyTitle")}
                            type="button"
                          >
                            <ShieldOff aria-hidden size={12} strokeWidth={2.2} />
                            {t("caps.untrustReadOnly")}
                          </button>
                        </div>
                      ) : (
                        <button
                          className="btn btn--small cap-tool-trust-btn"
                          disabled={busy}
                          onClick={() => onTrustTool(tool.name)}
                          title={t("caps.trustReadOnlyTitle")}
                          type="button"
                        >
                          <ShieldCheck aria-hidden size={12} strokeWidth={2.2} />
                          {t("caps.trustReadOnly")}
                        </button>
                      )
                    ) : null}
                  </div>
                </div>
              );
            })}
          </div>
        ) : (
          <div className="cap-tool-empty">{t("caps.noToolDetails")}</div>
        )
      )}
    </div>
  );
}

function EditServerForm({
  s,
  busy,
  onCancel,
  onSave,
}: {
  s: ServerView;
  busy: boolean;
  onCancel: () => void;
  onSave: (input: MCPServerInput) => void;
}) {
  const t = useT();
  const initialTransport = normalizeTransportValue(s.transport);
  const [transport, setTransport] = useState(initialTransport);
  const [command, setCommand] = useState(initialTransport === "stdio" ? serverCommand(s) : "");
  const [url, setUrl] = useState(initialTransport === "stdio" ? "" : s.url || serverCommand(s));
  const [headers, setHeaders] = useState("");
  const [env, setEnv] = useState("");
  const isStdio = transport === "stdio";
  const ready = isStdio ? command.trim() !== "" : url.trim() !== "";

  const submit = () => {
    const envText = env.trim();
    const headerText = headers.trim();
    onSave({
      name: s.name,
      transport,
      command: isStdio ? command.trim() : "",
      args: [],
      url: isStdio ? "" : url.trim(),
      env: envText === "" ? null : parseKeyValueText(envText),
      headers: isStdio || headerText === "" ? null : parseKeyValueText(headerText),
      trustedReadOnlyTools: s.trustedReadOnlyTools ?? [],
    });
  };

  return (
    <div className="cap-config-edit">
      <div className="cap-detail-grid">
        <div className="cap-detail">
          <span className="cap-detail__label">{t("caps.name")}</span>
          <span className="cap-detail__value">{s.name}</span>
        </div>
        <label className="cap-detail cap-detail--select">
          <span className="cap-detail__label">{t("caps.transport")}</span>
          <select className="mem-select" value={transport} disabled={busy} onChange={(e) => setTransport(e.target.value)}>
            <option value="stdio">stdio</option>
            <option value="http">http</option>
            <option value="sse">sse</option>
          </select>
        </label>
        {isStdio ? (
          <label className="cap-detail cap-detail--wide">
            <span className="cap-detail__label">{t("caps.command")}</span>
            <input className="mem-input" value={command} disabled={busy} onChange={(e) => setCommand(e.target.value)} placeholder={t("caps.commandPlaceholder")} />
          </label>
        ) : (
          <label className="cap-detail cap-detail--wide">
            <span className="cap-detail__label">{t("caps.url")}</span>
            <input className="mem-input" value={url} disabled={busy} onChange={(e) => setUrl(e.target.value)} placeholder={t("caps.urlPlaceholder")} />
          </label>
        )}
        {!isStdio && (
          <label className="cap-detail cap-detail--wide">
            <span className="cap-detail__label">{t("caps.headersLabel")}</span>
            <textarea className="mem-textarea cap-config-edit__env" value={headers} disabled={busy} onChange={(e) => setHeaders(e.target.value)} placeholder={t("caps.headersPlaceholder")} spellCheck={false} />
          </label>
        )}
        {!isStdio && s.headerKeys && s.headerKeys.length > 0 && (
          <div className="cap-detail cap-detail--wide">
            <span className="cap-detail__label">{t("caps.headerKeys")}</span>
            <span className="cap-detail__value">{s.headerKeys.join(", ")}</span>
            <span className="cap-edit-hint">{t("caps.headersPreserveHint")}</span>
          </div>
        )}
        <label className="cap-detail cap-detail--wide">
          <span className="cap-detail__label">{t("caps.envLabel")}</span>
          <textarea className="mem-textarea cap-config-edit__env" value={env} disabled={busy} onChange={(e) => setEnv(e.target.value)} placeholder={t("caps.envPlaceholder")} spellCheck={false} />
        </label>
        {s.envKeys && s.envKeys.length > 0 && (
          <div className="cap-detail cap-detail--wide">
            <span className="cap-detail__label">{t("caps.envKeys")}</span>
            <span className="cap-detail__value">{s.envKeys.join(", ")}</span>
            <span className="cap-edit-hint">{t("caps.envPreserveHint")}</span>
          </div>
        )}
      </div>
      <div className="cap-detail-actions">
        <button className="btn btn--small" disabled={busy} onClick={onCancel}>
          {t("common.cancel")}
        </button>
        <button className="btn btn--primary btn--small" disabled={busy || !ready} onClick={submit}>
          {t("caps.saveConfig")}
        </button>
      </div>
    </div>
  );
}

function serverCommand(s: ServerView): string {
  if (s.transport === "stdio") return [s.command, ...(s.args ?? [])].filter(Boolean).join(" ").trim();
  return (s.url || "").trim();
}

function normalizeTransportValue(transport: string): string {
  return transport === "http" || transport === "sse" ? transport : "stdio";
}

function parseKeyValueText(text: string): Record<string, string> {
  const values: Record<string, string> = {};
  for (const rawLine of text.split("\n")) {
    const line = rawLine.trim();
    if (!line) continue;
    const eq = line.indexOf("=");
    if (eq > 0) values[line.slice(0, eq).trim()] = line.slice(eq + 1).trim();
  }
  return values;
}

function serverStatusLabel(s: ServerView, t: ReturnType<typeof useT>): string {
  switch (s.status) {
    case "connected":
      return t("caps.connected");
    case "deferred":
      return t("caps.deferred");
    case "initializing":
      return t("caps.initializing");
    case "disabled":
      return s.configured && !s.autoStart ? t("caps.disabledAutoStart") : t("caps.disabled");
    case "failed":
      if (s.authStatus === "required") return t("caps.authRequired");
      return t("caps.failed");
    default:
      return s.status;
  }
}

function summarizeServerError(error: string): string {
  const normalized = error.replace(/\s+/g, " ").trim();
  const plugin = normalized.match(/plugin "([^"]+)"/i)?.[1];
  const npmCode = normalized.match(/\bnpm error code ([A-Z0-9_]+)/i)?.[1];
  const errno = normalized.match(/\berrno (-?\d+)/i)?.[1];
  const reason = npmCode
    ? `npm ${npmCode}${errno ? ` (${errno})` : ""}`
    : normalized.split(/(?:\.\s+|\n)/)[0];
  const summary = plugin ? `${plugin}: ${reason}` : reason;
  return summary.length > 180 ? `${summary.slice(0, 176).trim()}…` : summary;
}

type FailureKind = "auth" | "missing-command" | "command-unavailable" | "network" | "other";

function failureKind(server: ServerView): FailureKind {
  if (server.authStatus === "required") return "auth";
  const err = (server.error || "").toLowerCase();
  if (err.includes("command is required")) return "missing-command";
  if (
    err.includes("command not found") ||
    err.includes("executable file not found") ||
    err.includes("no such file") ||
    err.includes("enoent")
  ) {
    return "command-unavailable";
  }
  if (
    err.includes("401") ||
    err.includes("403") ||
    err.includes("unauthorized") ||
    err.includes("forbidden") ||
    err.includes("timeout") ||
    err.includes("network")
  ) {
    return "network";
  }
  return "other";
}

function failureGroups(servers: ServerView[], t: ReturnType<typeof useT>): Array<{ kind: FailureKind; label: string }> {
  const counts = new Map<FailureKind, number>();
  for (const server of servers) {
    const kind = failureKind(server);
    counts.set(kind, (counts.get(kind) ?? 0) + 1);
  }
  const order: FailureKind[] = ["missing-command", "command-unavailable", "auth", "network", "other"];
  return order.flatMap((kind) => {
    const count = counts.get(kind) ?? 0;
    if (count === 0) return [];
    return [{ kind, label: failureGroupLabel(kind, count, t) }];
  });
}

function failureGroupLabel(kind: FailureKind, count: number, t: ReturnType<typeof useT>): string {
  switch (kind) {
    case "auth":
      return t("caps.failureGroupAuth", { count });
    case "missing-command":
      return t("caps.failureGroupMissingCommand", { count });
    case "command-unavailable":
      return t("caps.failureGroupCommandUnavailable", { count });
    case "network":
      return t("caps.failureGroupNetwork", { count });
    default:
      return t("caps.failureGroupOther", { count });
  }
}

function canBulkRemoveFailure(server: ServerView): boolean {
  if (server.builtIn || !server.configured) return false;
  const kind = failureKind(server);
  return kind === "missing-command" || kind === "command-unavailable";
}

function retryableAvailableServerNames(servers: ServerView[]): string[] {
  return servers.filter(mcpServerRetryableFromAvailableList).map((s) => s.name);
}

function serverActionLabel(s: ServerView, t: ReturnType<typeof useT>): string {
  const err = (s.error || "").toLowerCase();
  if (shouldOpenAuth(s)) return t("caps.reauthorize");
  if (
    err.includes("command not found") ||
    err.includes("executable file not found") ||
    err.includes("no such file") ||
    err.includes("enoent")
  ) {
    return t("caps.checkCommand");
  }
  return t("caps.retry");
}

function serverAuthLabel(s: ServerView, t: ReturnType<typeof useT>): string {
  if (s.authStatus === "required") return t("caps.authRequired");
  if (s.authStatus === "possible") return t("caps.authPossible");
  return "";
}

function shouldOpenAuth(s: ServerView): boolean {
  const url = (s.authUrl || "").trim();
  return s.authStatus === "required" && /^https?:\/\//i.test(url);
}

function canClearAuth(s: ServerView): boolean {
  if (!s.configured || s.builtIn) return false;
  return Boolean(s.authConfigured || s.authStatus === "required" || s.authStatus === "possible" || isRemoteTransport(s.transport));
}

function isRemoteTransport(transport?: string): boolean {
  const value = (transport || "").trim().toLowerCase();
  return value === "http" || value === "streamable-http" || value === "sse";
}

function SkillRow({
  skill,
  busy,
  expanded,
  onToggle,
  onToggleEnabled,
}: {
  skill: SkillView;
  busy: boolean;
  expanded: boolean;
  onToggle: () => void;
  onToggleEnabled: (enabled: boolean) => void;
}) {
  const t = useT();
  const summary = summarizeSkillDescription(skill.description);
  const canExpand = summary !== skill.description;
  return (
    <div
      className={`cap-skill-card${expanded ? " cap-skill-card--expanded" : ""}${canExpand ? " cap-skill-card--expandable" : ""}${!skill.enabled ? " cap-skill-card--disabled" : ""}`}
    >
      <div className="cap-skill-card__top">
        <button className="cap-skill-card__toggle" type="button" onClick={onToggle} aria-expanded={expanded}>
          <span className="cap-skill-card__head">
            <span className="cap-skill-card__icon">/</span>
            <span className="cap-skill-card__main">
              <span className="cap-skill-card__command">{skill.name}</span>
              <span className="cap-skill-card__badges">
                <span className={`cap-skill-badge cap-skill-badge--${skill.scope}`}>{skillScopeLabel(skill.scope, t)}</span>
                {skill.runAs === "subagent" && <span className="cap-skill-badge cap-skill-badge--run">{t("caps.subagent")}</span>}
                {!skill.enabled && <span className="cap-skill-badge cap-skill-badge--off">{t("caps.skillDisabled")}</span>}
              </span>
            </span>
          </span>
        </button>
        <Tooltip label={skill.enabled ? t("caps.disableSkill") : t("caps.enableSkill")}>
          <label className="cap-switch">
            <input
              type="checkbox"
              checked={skill.enabled}
              disabled={busy}
              onChange={(e) => onToggleEnabled(e.target.checked)}
            />
            <span className="cap-switch__track" />
          </label>
        </Tooltip>
      </div>
      <div className="cap-skill-card__desc">{expanded ? skill.description : summary}</div>
      {canExpand && (
        <button className="cap-skill-card__more" type="button" onClick={onToggle} aria-expanded={expanded}>
          {expanded ? t("common.collapse") : t("common.expand")}
        </button>
      )}
    </div>
  );
}

function skillScopeLabel(scope: string, t: ReturnType<typeof useT>): string {
  switch (scope) {
    case "builtin":
      return t("caps.skillScopeBuiltin");
    case "project":
      return t("caps.skillScopeProject");
    case "custom":
      return t("caps.skillScopeCustom");
    case "global":
      return t("caps.skillScopeGlobal");
    default:
      return scope;
  }
}

function summarizeSkillDescription(description: string): string {
  const normalized = description.replace(/\s+/g, " ").trim();
  if (normalized.length <= 132) return normalized;
  const sentence = normalized.match(/^.{48,132}?[。.!?；;，,]/u)?.[0]?.trim();
  if (sentence && sentence.length >= 48) return sentence.replace(/[。.!?；;，,]$/u, "");
  return `${normalized.slice(0, 128).trim()}…`;
}

function AddServerForm({
  busy,
  onCancel,
  onAdd,
}: {
  busy: boolean;
  onCancel: () => void;
  onAdd: (input: MCPServerInput) => void;
}) {
  const t = useT();
  const [name, setName] = useState("");
  const [transport, setTransport] = useState("stdio");
  const [command, setCommand] = useState("");
  const [url, setUrl] = useState("");
  const [headers, setHeaders] = useState("");
  const [env, setEnv] = useState("");

  const isStdio = transport === "stdio";
  const ready = name.trim() !== "" && (isStdio ? command.trim() !== "" : url.trim() !== "");

  const submit = () => {
    const envText = env.trim();
    const headerText = headers.trim();
    onAdd({
      name: name.trim(),
      transport,
      command: isStdio ? command.trim() : "",
      args: [],
      url: isStdio ? "" : url.trim(),
      env: envText === "" ? null : parseKeyValueText(envText),
      headers: isStdio || headerText === "" ? null : parseKeyValueText(headerText),
    });
  };

  return (
    <div className="prov-card prov-card--edit">
      <input className="mem-input" placeholder={t("caps.namePlaceholder")} value={name} onChange={(e) => setName(e.target.value)} />
      <label className="set-label">{t("caps.transport")}</label>
      <select className="mem-select" value={transport} onChange={(e) => setTransport(e.target.value)}>
        <option value="stdio">stdio</option>
        <option value="http">http</option>
        <option value="sse">sse</option>
      </select>
      {isStdio ? (
        <input className="mem-input" placeholder={t("caps.commandPlaceholder")} value={command} onChange={(e) => setCommand(e.target.value)} />
      ) : (
        <input className="mem-input" placeholder={t("caps.urlPlaceholder")} value={url} onChange={(e) => setUrl(e.target.value)} />
      )}
      {!isStdio && (
        <>
          <label className="set-label">{t("caps.headersLabel")}</label>
          <textarea className="mem-textarea" value={headers} onChange={(e) => setHeaders(e.target.value)} placeholder={t("caps.headersPlaceholder")} spellCheck={false} />
        </>
      )}
      <label className="set-label">{t("caps.envLabel")}</label>
      <textarea className="mem-textarea" value={env} onChange={(e) => setEnv(e.target.value)} placeholder={t("caps.envPlaceholder")} spellCheck={false} />
      <div className="prov-card__actions">
        <button className="btn btn--small" onClick={onCancel} disabled={busy}>
          {t("common.cancel")}
        </button>
        <button className="btn btn--primary btn--small" onClick={submit} disabled={busy || !ready}>
          {t("caps.add")}
        </button>
      </div>
    </div>
  );
}

type PluginInstallMode = "local" | "git" | "registry";
type PluginLifecycleKind = "update" | "rollback" | "remove";
type PluginLifecyclePlan = { kind: PluginLifecycleKind; plan: PluginOperationView };
type PluginNotice = { message: string; tone: "success" | "warning" };
type PluginEnableApproval = {
	name: string;
	digest: string;
	permissions: string[];
	trustStatus: string;
	skills: number;
	hooks: number;
	mcpServers: number;
};

// PluginsSettingsPage is the desktop plugin package manager embedded inside
// Settings. It mirrors the MCP/Skills density: install planning on top, package
// rows below, and diagnostics/details only when a row is expanded.
export function PluginsSettingsPage() {
	const t = useT();
	const [snapshotKey, setSnapshotKey] = useState("");
	const [plugins, setPlugins] = useState<PluginView[] | null>(null);
	const [busy, setBusy] = useState(false);
	const [err, setErr] = useState<string | null>(null);
	const [installMode, setInstallMode] = useState<PluginInstallMode>("local");
	const [localSource, setLocalSource] = useState("");
	const [gitSource, setGitSource] = useState("");
	const [registryQuery, setRegistryQuery] = useState("");
	const [registryEntries, setRegistryEntries] = useState<PluginRegistryEntryView[] | null>(null);
	const [registryName, setRegistryName] = useState("");
	const [name, setName] = useState("");
	const [link, setLink] = useState(false);
	const [replace, setReplace] = useState(false);
	const [plan, setPlan] = useState<PluginOperationView | null>(null);
	const [plannedInputKey, setPlannedInputKey] = useState("");
	const [lifecyclePlans, setLifecyclePlans] = useState<Record<string, PluginLifecyclePlan>>({});
	const [notice, setNotice] = useState<PluginNotice | null>(null);
	const [expanded, setExpanded] = useState<Set<string>>(() => new Set());
	const [diagnostics, setDiagnostics] = useState<Record<string, PluginView>>({});
	const [enableApproval, setEnableApproval] = useState<PluginEnableApproval | null>(null);

	const reload = useCallback(async () => {
		const [meta, tabs] = await Promise.all([
			app.Meta().catch(() => null),
			app.ListTabs().catch(() => []),
		]);
		const key = settingsSnapshotKey(meta, tabs);
		setSnapshotKey(key);
		const cached = key ? pluginsSettingsSnapshot : null;
		if (cached?.key === key) {
			setPlugins(cached.value);
		} else {
			setPlugins(null);
		}
		const next = normalizePluginViews(await app.Plugins().catch(() => []));
		pluginsSettingsSnapshot = { key, value: next };
		setPlugins(next);
	}, []);
	useEffect(() => { void reload(); }, [reload]);

	const run = async (fn: () => Promise<unknown>, reloadAfter = true) => {
		setBusy(true);
		setErr(null);
		setNotice(null);
		try {
			const result = await fn();
			if (isPluginOperation(result)) {
				if (["failed", "blocked", "denied"].includes(result.status) || (!result.ok && result.status !== "partial")) {
					throw new Error(pluginOperationFailure(result));
				}
				const nextNotice = pluginOperationNotice(result, t);
				if (nextNotice) setNotice(nextNotice);
			}
			if (reloadAfter) await reload();
			return true;
		} catch (e) {
			setErr(String((e as Error)?.message ?? e));
			if (reloadAfter) await reload();
			return false;
		} finally {
			setBusy(false);
		}
	};

	const sourceValue = installMode === "local"
		? localSource.trim()
		: installMode === "registry"
			? (registryName ? `registry:${registryName}` : "")
			: gitSource.trim();
	const selectedRegistryEntry = registryEntries?.find((entry) => entry.name === registryName);
	const installOptions = (planId?: string): PluginInstallOptions => ({
		dryRun: false,
		link: installMode === "local" ? link : false,
		replace,
		name: installMode === "git" ? name.trim() || undefined : undefined,
		planId,
	});
	const installInputKey = JSON.stringify({ source: sourceValue, mode: installMode, link: installMode === "local" && link, replace, name: installMode === "git" ? name.trim() : "" });
	const actionBusy = busy || !snapshotKey || !plugins;
	const canPlan = sourceValue.length > 0 && !actionBusy;
	const canInstall = canApplyPluginPlan(plan) && plannedInputKey === installInputKey && !actionBusy;
	const summary = plugins ? pluginListSummary(plugins, t) : "";
	const togglePlugin = useCallback((pluginName: string) => {
		setExpanded((prev) => { const next = new Set(prev); if (next.has(pluginName)) next.delete(pluginName); else next.add(pluginName); return next; });
	}, []);
	const requestPluginEnabled = (plugin: PluginView, enabled: boolean) => {
		if (!enabled) {
			setEnableApproval(null);
			void run(() => app.SetPluginEnabled(plugin.name, false, plugin.digest || "", asArray(plugin.permissions)));
			return;
		}
		setEnableApproval({
			name: plugin.name,
			digest: plugin.digest || "",
			permissions: [...asArray(plugin.permissions)],
			trustStatus: plugin.trustStatus || "",
			skills: plugin.skills,
			hooks: plugin.hooks,
			mcpServers: plugin.mcpServers,
		});
		setExpanded((prev) => {
			const next = new Set(prev);
			next.add(plugin.name);
			return next;
		});
	};
	const approvePluginEnabled = () => {
		const approval = enableApproval;
		if (!approval) return;
		void run(async () => {
			try {
				await app.SetPluginEnabled(approval.name, true, approval.digest, approval.permissions);
			} finally {
				setEnableApproval(null);
			}
		});
	};
	const setMode = (mode: PluginInstallMode) => {
		setInstallMode(mode);
		setPlan(null);
		setPlannedInputKey("");
	};
	const searchRegistry = () => {
		void run(async () => {
			const entries = await app.SearchPluginRegistry(registryQuery.trim());
			setRegistryEntries(entries);
			setRegistryName((current) => entries.some((entry) => entry.name === current) ? current : (entries[0]?.name ?? ""));
		}, false);
	};
	useEffect(() => {
		if (installMode === "registry" && registryEntries === null && !busy) searchRegistry();
		// Search is explicitly refreshed after the first registry-mode load.
		// eslint-disable-next-line react-hooks/exhaustive-deps
	}, [installMode, registryEntries, busy]);
	const previewInstall = () => {
		if (!sourceValue) return;
		setPlan(null);
		setPlannedInputKey("");
		void run(async () => {
			const operation = await app.PlanPluginInstall(sourceValue, { ...installOptions(), dryRun: true });
			if (!canApplyPluginPlan(operation)) throw new Error(pluginOperationFailure(operation));
			setPlan(operation);
			setPlannedInputKey(installInputKey);
		}, false);
	};
	const install = () => {
		if (!sourceValue || !canApplyPluginPlan(plan) || plannedInputKey !== installInputKey) return;
		void run(async () => {
			try {
				const operation = await app.InstallPlugin(sourceValue, installOptions(plan.planId));
				setPlan(operation);
				setPlannedInputKey("");
				return operation;
			} catch (error) {
				setPlan(null);
				setPlannedInputKey("");
				throw error;
			}
		});
	};
	const runDoctor = (pluginName: string) => {
		void run(async () => {
			const view = normalizePluginView(await app.PluginDoctor(pluginName));
			setDiagnostics((prev) => ({ ...prev, [pluginName]: view }));
			setExpanded((prev) => {
				const next = new Set(prev);
				next.add(pluginName);
				return next;
			});
		}, false);
	};
	const updateLocalSource = (value: string) => {
		setLocalSource(value);
		setPlan(null);
		setPlannedInputKey("");
	};
	const updateGitSource = (value: string) => {
		setGitSource(value);
		setPlan(null);
		setPlannedInputKey("");
	};
	const updateRegistryName = (value: string) => {
		setRegistryName(value);
		setPlan(null);
		setPlannedInputKey("");
	};
	const invalidateInstallPlan = () => {
		setPlan(null);
		setPlannedInputKey("");
	};
	const planLifecycle = (kind: PluginLifecycleKind, pluginName: string) => {
		setLifecyclePlans((prev) => {
			const next = { ...prev };
			delete next[pluginName];
			return next;
		});
		void run(async () => {
			const operation = kind === "update"
				? await app.PlanPluginUpdate(pluginName)
				: kind === "rollback"
					? await app.PlanPluginRollback(pluginName)
					: await app.PlanPluginRemove(pluginName);
			if (!canApplyPluginPlan(operation)) throw new Error(pluginOperationFailure(operation));
			setLifecyclePlans((prev) => ({ ...prev, [pluginName]: { kind, plan: operation } }));
			return operation;
		}, false);
	};
	const applyLifecycle = (kind: PluginLifecycleKind, pluginName: string) => {
		const pending = lifecyclePlans[pluginName];
		if (!pending || pending.kind !== kind || !canApplyPluginPlan(pending.plan)) return;
		const planId = pending.plan.planId;
		void run(async () => {
			try {
				const operation = kind === "update"
					? await app.UpdatePlugin(pluginName, planId)
					: kind === "rollback"
						? await app.RollbackPlugin(pluginName, planId)
						: await app.RemovePlugin(pluginName, planId);
				return operation;
			} finally {
				setLifecyclePlans((prev) => {
					const next = { ...prev };
					delete next[pluginName];
					return next;
				});
			}
		});
	};
	const pickPluginFolder = () => {
		void run(async () => {
			const path = await app.PickPluginFolder();
			if (path) {
				setInstallMode("local");
				updateLocalSource(path);
			}
		}, false);
	};

	return (
		<section id="plugin-settings-page" className="mem-section" aria-label={t("caps.pluginInstallTitle")}>
			{err && <div id="plugin-operation-error" className="banner banner--error" role="alert">{err}</div>}
			{notice && !err && <div id="plugin-operation-notice" className={`banner banner--${notice.tone}`} role="status">{notice.message}</div>}
			<div className="cap-plugin-installer">
				<div className="cap-plugin-installer__head">
					<div className="cap-plugin-installer__copy">
						<div className="cap-plugin-installer__title">{t("caps.pluginInstallTitle")}</div>
						<div className="cap-plugin-installer__hint">{t("caps.pluginInstallHint")}</div>
					</div>
					<div className="cap-tabs cap-plugin-installer__mode" role="group" aria-label={t("caps.pluginInstallMethod")}>
						<button
							id="plugin-install-mode-local"
							className={`cap-tab${installMode === "local" ? " cap-tab--active" : ""}`}
							type="button"
							aria-pressed={installMode === "local"}
							onClick={() => setMode("local")}
						>
							{t("caps.pluginInstallLocal")}
						</button>
						<button
							id="plugin-install-mode-git"
							className={`cap-tab${installMode === "git" ? " cap-tab--active" : ""}`}
							type="button"
							aria-pressed={installMode === "git"}
							onClick={() => setMode("git")}
						>
							{t("caps.pluginInstallGit")}
						</button>
						<button
							id="plugin-install-mode-registry"
							className={`cap-tab${installMode === "registry" ? " cap-tab--active" : ""}`}
							type="button"
							aria-pressed={installMode === "registry"}
							onClick={() => setMode("registry")}
						>
							{t("caps.pluginInstallRegistry")}
						</button>
					</div>
				</div>
				<div className="cap-plugin-form-grid">
					{installMode === "local" ? (
						<div className="cap-plugin-fields cap-plugin-fields--local">
							<div className="cap-plugin-folder-field">
								<button id="plugin-install-pick-folder" className="btn btn--small" disabled={actionBusy} type="button" onClick={pickPluginFolder}>
									{t("caps.pluginChooseLocalFolder")}
								</button>
								<input
									id="plugin-install-local-source"
									className={`mem-input cap-plugin-path${localSource ? "" : " cap-plugin-path--empty"}`}
									aria-label={t("caps.pluginLocalFolder")}
									placeholder={t("caps.pluginNoLocalFolder")}
									value={localSource}
									onInput={(e) => updateLocalSource(e.currentTarget.value)}
									onChange={(e) => updateLocalSource(e.target.value)}
								/>
							</div>
						</div>
					) : installMode === "git" ? (
						<div className="cap-plugin-fields cap-plugin-fields--git">
							<input
								id="plugin-install-git-source"
								className="mem-input"
								aria-label={t("caps.pluginGitSource")}
								placeholder={t("caps.pluginSourcePlaceholder")}
								value={gitSource}
								onInput={(e) => updateGitSource(e.currentTarget.value)}
								onChange={(e) => updateGitSource(e.target.value)}
							/>
							<div className="cap-plugin-field">
								<input
									id="plugin-install-name"
									className="mem-input"
									aria-label={t("caps.pluginInstallName")}
									placeholder={t("caps.pluginInstallNamePlaceholder")}
									value={name}
									onInput={(e) => { setName(e.currentTarget.value); invalidateInstallPlan(); }}
								/>
							</div>
						</div>
					) : (
						<div className="cap-plugin-fields cap-plugin-fields--registry">
							<div className="cap-plugin-registry-search">
								<input
									id="plugin-registry-search"
									className="mem-input"
									aria-label={t("caps.pluginRegistrySearch")}
									placeholder={t("caps.pluginRegistrySearchPlaceholder")}
									value={registryQuery}
									onInput={(e) => setRegistryQuery(e.currentTarget.value)}
									onChange={(e) => setRegistryQuery(e.target.value)}
									onKeyDown={(e) => { if (e.key === "Enter") { e.preventDefault(); searchRegistry(); } }}
								/>
								<button className="btn btn--small" type="button" disabled={busy} onClick={searchRegistry}>{t("caps.pluginRegistrySearchAction")}</button>
							</div>
							<select
								id="plugin-registry-entry"
								className="mem-input"
								aria-label={t("caps.pluginRegistryEntry")}
								value={registryName}
								disabled={busy || !registryEntries?.length}
								onChange={(e) => updateRegistryName(e.target.value)}
							>
								{!registryEntries?.length && <option value="">{t("caps.pluginRegistryEmpty")}</option>}
								{registryEntries?.map((entry) => <option key={entry.name} value={entry.name}>{entry.name} - {entry.version} - {entry.description || entry.category}</option>)}
							</select>
							{selectedRegistryEntry && (
								<div className="cap-plugin-registry-evidence">
									{t("caps.pluginRegistryEvidence", {
										registry: selectedRegistryEntry.registryName,
										version: String(selectedRegistryEntry.registryRootVersion),
									})}
									<div><code>{selectedRegistryEntry.registryEntryDigest}</code></div>
								</div>
							)}
						</div>
					)}
					<div className="cap-plugin-installer__options">
						<div className="cap-plugin-option-block">
							<label className="cap-plugin-option">
								<input type="checkbox" checked={replace} disabled={actionBusy} onChange={(e) => { setReplace(e.target.checked); invalidateInstallPlan(); }} />
								<span>{t("caps.pluginReplace")}</span>
							</label>
							<div className="cap-plugin-option-hint">{t("caps.pluginReplaceHint")}</div>
						</div>
						{installMode === "local" && (
							<div className="cap-plugin-option-block">
								<label className="cap-plugin-option">
									<input type="checkbox" checked={link} disabled={actionBusy} onChange={(e) => { setLink(e.target.checked); invalidateInstallPlan(); }} />
									<span>{t("caps.pluginLink")}</span>
								</label>
								<div className="cap-plugin-option-hint">{t("caps.pluginLinkHint")}</div>
							</div>
						)}
					</div>
					<div className="cap-plugin-installer__actions">
						<button id="plugin-install-preview" className="btn btn--small" type="button" disabled={!canPlan} onClick={previewInstall}>
							{t("caps.pluginPreview")}
						</button>
						<button id="plugin-install-apply" className="btn btn--primary btn--small" type="button" disabled={!canInstall} onClick={install}>
							{t("caps.pluginInstall")}
						</button>
					</div>
				</div>
			</div>
			{plan && <PluginPlanPreview plan={plan} id="plugin-install-plan" />}
			<div className="cap-server-section cap-plugin-section">
				<div className="cap-server-section__head">
					<div className="cap-server-section__copy">
						<div className="cap-server-section__title">{t("caps.installedPlugins")}</div>
						{plugins && plugins.length > 0 && <div className="drawer__summary">{summary}</div>}
					</div>
					<button className="btn btn--small" disabled={actionBusy} type="button" onClick={() => void reload()}>
						{t("caps.pluginRefresh")}
					</button>
				</div>
				{!plugins ? (
					<div className="mem-empty">{t("caps.loading")}</div>
				) : plugins.length === 0 ? (
					<div className="mem-empty mem-empty--cta">
						<strong>{t("caps.noPluginsTitle")}</strong>
						<span>{t("caps.noPluginsHint")}</span>
					</div>
				) : (
					<div className="cap-server-group">
						{plugins.map((plugin) => (
							<PluginRow
								key={plugin.name}
								plugin={plugin}
								diagnostic={diagnostics[plugin.name]}
								busy={actionBusy}
								expanded={expanded.has(plugin.name)}
								pending={lifecyclePlans[plugin.name]}
								enableApproval={enableApproval?.name === plugin.name ? enableApproval : undefined}
								onToggleDetails={() => togglePlugin(plugin.name)}
								onToggleEnabled={(enabled) => requestPluginEnabled(plugin, enabled)}
								onApproveEnable={approvePluginEnabled}
								onCancelEnable={() => setEnableApproval(null)}
								onUpdate={() => lifecyclePlans[plugin.name]?.kind === "update" ? applyLifecycle("update", plugin.name) : planLifecycle("update", plugin.name)}
								onRollback={() => lifecyclePlans[plugin.name]?.kind === "rollback" ? applyLifecycle("rollback", plugin.name) : planLifecycle("rollback", plugin.name)}
								onDoctor={() => runDoctor(plugin.name)}
								onRemove={() => lifecyclePlans[plugin.name]?.kind === "remove" ? applyLifecycle("remove", plugin.name) : planLifecycle("remove", plugin.name)}
							/>
						))}
					</div>
				)}
			</div>
		</section>
	);
}

function PluginPlanPreview({ plan, id }: { plan: PluginOperationView; id?: string }) {
	const t = useT();
	const isError = ["failed", "blocked", "denied"].includes(plan.status) || (!plan.ok && plan.status !== "partial");
	const isWarning = plan.status === "partial" || plan.actions.some((action) => action.riskLevel === "high");
	return (
		<div
			id={id}
			className={`cap-plugin-plan${isError ? " cap-plugin-plan--error" : isWarning ? " cap-plugin-plan--warning" : ""}`}
			role="group"
			aria-label={isError ? t("caps.pluginPlanError") : t("caps.pluginPlanReady")}
		>
			<div className="cap-plugin-plan__head">
				<div className="cap-plugin-plan__title">{isError ? t("caps.pluginPlanError") : t("caps.pluginPlanReady")}</div>
				{plan.status && <span className="cap-source-badge">{plan.status}</span>}
			</div>
			{plan.name && <div className="cap-plugin-plan__meta">{plan.name}</div>}
			{plan.planId && <div className="cap-plugin-plan__meta">{t("caps.pluginPlanId")}: <code>{plan.planId}</code></div>}
			{asArray(plan.warnings).map((warning, idx) => (
				<div className="cap-plugin-plan__warning" key={`${warning}-${idx}`}>{warning}</div>
			))}
			{plan.actions.length > 0 ? (
				<div className="cap-plugin-actions">
					{plan.actions.map((action, idx) => (
						<div className="cap-plugin-action" key={`${action.action || action.kind || "action"}-${idx}`}>
							<span className="cap-plugin-action__name">{pluginPlanActionLabel(action, t)}</span>
							{action.status && <span className="cap-source-badge">{action.status}</span>}
							{action.riskLevel && <span className={`cap-source-badge cap-plugin-risk--${action.riskLevel}`}>{t("caps.pluginRisk", { risk: action.riskLevel })}</span>}
							{action.source && <span className="cap-plugin-action__source">{action.source}</span>}
							{(action.currentVersion || action.version) && <span className="cap-plugin-action__source">{t("caps.pluginVersionChange", { current: action.currentVersion || "-", next: action.version || "-" })}</span>}
							{action.trustStatus && <span className="cap-plugin-action__source">{t("caps.pluginTrust")}: {action.trustStatus}</span>}
							{action.registryName && <span className="cap-plugin-action__source">{t("caps.pluginRegistryTrustRoot")}: {action.registryName} @ root v{action.registryRootVersion || "-"}</span>}
							{action.registryEntryDigest && <span className="cap-plugin-action__source">{t("caps.pluginRegistryEntryDigest")}: {action.registryEntryDigest}</span>}
							{action.provenanceStatus && <span className="cap-plugin-action__source">{t("caps.pluginProvenance")}: {action.provenanceStatus}{action.attestationDigest ? ` · ${action.attestationDigest}` : ""}</span>}
							{action.willEnable !== undefined && <span className="cap-plugin-action__source">{action.willEnable ? t("caps.pluginWillEnable") : t("caps.pluginWillStayDisabled")}</span>}
							{action.rollbackAvailable && <span className="cap-plugin-action__source">{t("caps.pluginRollbackAvailable")}</span>}
							{asArray(action.addedPermissions).length > 0 && <span className="cap-plugin-plan__warning">{t("caps.pluginAddedPermissions")}: {asArray(action.addedPermissions).join(", ")}</span>}
							{asArray(action.removedPermissions).length > 0 && <span className="cap-plugin-action__source">{t("caps.pluginRemovedPermissions")}: {asArray(action.removedPermissions).join(", ")}</span>}
							{asArray(action.permissions).length > 0 && <span className="cap-plugin-action__source">{t("caps.pluginPermissions")}: {asArray(action.permissions).join(", ")}</span>}
							{asArray(action.riskReasons).map((reason, reasonIndex) => <span className="cap-plugin-plan__warning" key={`${reason}-${reasonIndex}`}>{reason}</span>)}
							{asArray(action.warnings).map((warning, warningIndex) => <span className="cap-plugin-plan__warning" key={`${warning}-${warningIndex}`}>{warning}</span>)}
							{action.error && <span className="cap-plugin-plan__warning">{action.error}</span>}
						</div>
					))}
				</div>
			) : (
				plan.next && <div className="cap-plugin-plan__warning">{plan.next}</div>
			)}
		</div>
	);
}

function PluginRow({
	plugin,
	diagnostic,
	busy,
	expanded,
	pending,
	enableApproval,
	onToggleDetails,
	onToggleEnabled,
	onApproveEnable,
	onCancelEnable,
	onUpdate,
	onRollback,
	onDoctor,
	onRemove,
}: {
	plugin: PluginView;
	diagnostic?: PluginView;
	busy: boolean;
	expanded: boolean;
	pending?: PluginLifecyclePlan;
	enableApproval?: PluginEnableApproval;
	onToggleDetails: () => void;
	onToggleEnabled: (enabled: boolean) => void;
	onApproveEnable: () => void;
	onCancelEnable: () => void;
	onUpdate: () => void;
	onRollback: () => void;
	onDoctor: () => void;
	onRemove: () => void;
}) {
	const t = useT();
	const status = plugin.error ? "failed" : plugin.enabled ? "connected" : "disabled";
	const warnings = pluginWarnings(plugin, diagnostic);
	const sub = plugin.error || pluginCapabilitiesSummary(plugin, t);
	const canEnable = plugin.enabled || Boolean(plugin.digest);
	return (
		<div
			id={`plugin-row-${plugin.name}`}
			className={`cap-server-entry cap-plugin-entry${plugin.enabled ? "" : " cap-server-entry--disabled"}`}
			role="group"
			aria-label={plugin.name}
		>
			<Tooltip label={plugin.error} disabled={!plugin.error} fill block>
				<div className={`cap-row${plugin.enabled ? "" : " cap-row--disabled"}`}>
					<Tooltip label={expanded ? t("caps.collapseDetails") : t("caps.expandDetails")}>
						<button
							id={`plugin-${plugin.name}-details`}
							className="cap-disclosure"
							aria-expanded={expanded}
							type="button"
							onClick={onToggleDetails}
						>
							{expanded ? "⌄" : "›"}
						</button>
					</Tooltip>
					<span className={`cap-dot cap-dot--${status}`} />
					<div className="cap-row__text">
						<div className="cap-row__head">
							<span className="cap-row__name">{plugin.name}</span>
							{plugin.manifestKind && <span className="cap-row__transport">{plugin.manifestKind}</span>}
							{plugin.version && <span className="cap-source-badge">{plugin.version}</span>}
							{warnings.length > 0 && <span className="cap-row__update cap-row__update--error">{t("caps.pluginWarnings", { count: warnings.length })}</span>}
						</div>
						<div className="cap-row__sub">{sub}</div>
					</div>
					<div className="cap-row__actions">
						<Tooltip label={plugin.enabled ? t("caps.pluginDisable") : t("caps.pluginEnable")}>
							<label id={`plugin-${plugin.name}-enable-toggle`} className="cap-switch" aria-label={plugin.enabled ? t("caps.pluginDisable") : t("caps.pluginEnable")}>
								<input
									id={`plugin-${plugin.name}-enabled`}
									type="checkbox"
									checked={plugin.enabled}
									disabled={busy || !canEnable}
									onChange={(e) => onToggleEnabled(e.target.checked)}
								/>
								<span className="cap-switch__track" />
							</label>
						</Tooltip>
					</div>
				</div>
			</Tooltip>
			{expanded && (
				<div className="cap-server-details">
					{enableApproval && (
						<PluginEnableApprovalReview
							approval={enableApproval}
							busy={busy}
							onApprove={onApproveEnable}
							onCancel={onCancelEnable}
						/>
					)}
					<div className="cap-detail-grid">
						<div className="cap-detail">
							<span className="cap-detail__label">{t("caps.status")}</span>
							<span className="cap-detail__value">{plugin.enabled ? t("caps.pluginEnabled") : t("caps.pluginDisabled")}</span>
						</div>
						{plugin.version && (
							<div className="cap-detail">
								<span className="cap-detail__label">{t("caps.pluginVersion")}</span>
								<span className="cap-detail__value">{plugin.version}</span>
							</div>
						)}
						{plugin.source && (
							<div className="cap-detail cap-detail--wide">
								<span className="cap-detail__label">{t("caps.pluginSource")}</span>
								<span className="cap-detail__code">{plugin.source}</span>
							</div>
						)}
						{plugin.root && (
							<div className="cap-detail cap-detail--wide">
								<span className="cap-detail__label">{t("caps.pluginRoot")}</span>
								<span className="cap-detail__code">{plugin.root}</span>
							</div>
						)}
						{plugin.manifestSchema !== undefined && (
							<div className="cap-detail">
								<span className="cap-detail__label">{t("caps.pluginSchema")}</span>
								<span className="cap-detail__value">{plugin.manifestSchema}</span>
							</div>
						)}
						{plugin.installMode && (
							<div className="cap-detail">
								<span className="cap-detail__label">{t("caps.pluginInstallMode")}</span>
								<span className="cap-detail__value">{plugin.installMode}</span>
							</div>
						)}
						{plugin.trustStatus && (
							<div className="cap-detail">
								<span className="cap-detail__label">{t("caps.pluginTrust")}</span>
								<span className="cap-detail__value">{plugin.trustStatus}</span>
							</div>
						)}
						{plugin.sourceKind && (
							<div className="cap-detail">
								<span className="cap-detail__label">{t("caps.pluginSourceKind")}</span>
								<span className="cap-detail__value">{plugin.sourceKind}{plugin.sourceRevision ? ` @ ${plugin.sourceRevision}` : ""}</span>
							</div>
						)}
						{plugin.registryName && (
							<div className="cap-detail cap-detail--wide">
								<span className="cap-detail__label">{t("caps.pluginRegistryTrustRoot")}</span>
								<span className="cap-detail__code">{plugin.registryName} @ root v{plugin.registryRootVersion || "-"} · {plugin.registryMetadataUrl}</span>
							</div>
						)}
						{plugin.registryRootDigest && (
							<div className="cap-detail cap-detail--wide">
								<span className="cap-detail__label">{t("caps.pluginRegistryRootDigest")}</span>
								<span className="cap-detail__code">{plugin.registryRootDigest}</span>
							</div>
						)}
						{plugin.registryEntryDigest && (
							<div className="cap-detail cap-detail--wide">
								<span className="cap-detail__label">{t("caps.pluginRegistryEntryDigest")}</span>
								<span className="cap-detail__code">{plugin.registryEntryDigest}</span>
							</div>
						)}
						{plugin.provenanceStatus && (
							<div className="cap-detail cap-detail--wide">
								<span className="cap-detail__label">{t("caps.pluginProvenance")}</span>
								<span className="cap-detail__code">{plugin.provenanceStatus}{plugin.attestationDigest ? ` · ${plugin.attestationDigest}` : ""}</span>
							</div>
						)}
						{plugin.digest && (
							<div className="cap-detail cap-detail--wide">
								<span className="cap-detail__label">{t("caps.pluginDigest")}</span>
								<span className="cap-detail__code">{plugin.digest}</span>
							</div>
						)}
						<div className="cap-detail cap-detail--wide">
							<span className="cap-detail__label">{t("caps.pluginPermissions")}</span>
							<span className="cap-detail__code">{asArray(plugin.permissions).join(", ") || t("caps.pluginNoPermissions")}</span>
						</div>
						<div className="cap-detail cap-detail--wide">
							<span className="cap-detail__label">{t("caps.pluginGrantedPermissions")}</span>
							<span className="cap-detail__code">{asArray(plugin.grantedPermissions).join(", ") || t("caps.pluginNoPermissions")}</span>
						</div>
						{plugin.rollback && (
							<div className="cap-detail cap-detail--wide">
								<span className="cap-detail__label">{t("caps.pluginRollbackTarget")}</span>
								<span className="cap-detail__code">{plugin.rollback.version || "-"}{plugin.rollback.digest ? ` · ${plugin.rollback.digest}` : ""}</span>
							</div>
						)}
					</div>
					{plugin.description && <div className="cap-plugin-description">{plugin.description}</div>}
					<PluginUsageDetails plugin={plugin} />
					{diagnostic?.error && <div className="cap-source__warning">{diagnostic.error}</div>}
					{warnings.map((warning, idx) => (
						<div className="cap-source__warning" key={`${plugin.name}-warning-${idx}`}>{warning}</div>
					))}
					{pending && <PluginPlanPreview plan={pending.plan} id={`plugin-${plugin.name}-${pending.kind}-plan`} />}
					<div className="cap-detail-actions">
						<button id={`plugin-${plugin.name}-update`} className="btn btn--small" disabled={busy} type="button" onClick={onUpdate}>
							{pending?.kind === "update" ? t("caps.pluginApplyUpdate") : t("caps.pluginUpdate")}
						</button>
						{plugin.rollback && (
							<button id={`plugin-${plugin.name}-rollback`} className="btn btn--small" disabled={busy} type="button" onClick={onRollback}>
								{pending?.kind === "rollback" ? t("caps.pluginApplyRollback") : t("caps.pluginRollback")}
							</button>
						)}
						<button id={`plugin-${plugin.name}-doctor`} className="btn btn--small" disabled={busy} type="button" onClick={onDoctor}>
							{t("caps.pluginDoctor")}
						</button>
						{pending?.kind === "remove" ? (
							<button id={`plugin-${plugin.name}-remove-apply`} className="btn btn--danger btn--small" disabled={busy} type="button" onClick={onRemove}>{t("caps.pluginApplyRemove")}</button>
						) : (
							<InlineConfirmButton
								id={`plugin-${plugin.name}-remove`}
								confirmId={`plugin-${plugin.name}-remove-confirm`}
								cancelId={`plugin-${plugin.name}-remove-cancel`}
								label={t("caps.pluginRemove")}
								confirmLabel={t("caps.pluginConfirmRemove")}
								cancelLabel={t("common.cancel")}
								disabled={busy}
								danger
								onConfirm={onRemove}
							/>
						)}
					</div>
				</div>
			)}
		</div>
	);
}

function PluginEnableApprovalReview({
	approval,
	busy,
	onApprove,
	onCancel,
}: {
	approval: PluginEnableApproval;
	busy: boolean;
	onApprove: () => void;
	onCancel: () => void;
}) {
	const t = useT();
	return (
		<div id={`plugin-${approval.name}-enable-review`} className="cap-plugin-plan cap-plugin-plan--warning" role="group" aria-label={t("caps.pluginEnableReview")}>
			<div className="cap-plugin-plan__head">
				<div className="cap-plugin-plan__title">{t("caps.pluginEnableReview")}</div>
				<span className="cap-source-badge">{approval.name}</span>
			</div>
			<div className="cap-plugin-plan__meta">{t("caps.pluginTrust")}: {approval.trustStatus || "-"}</div>
			<div className="cap-plugin-plan__meta">{t("caps.pluginDigest")}: <code>{approval.digest}</code></div>
			<div className="cap-plugin-plan__meta">{t("caps.pluginPermissions")}: <code>{approval.permissions.join(", ") || t("caps.pluginNoPermissions")}</code></div>
			<div className="cap-plugin-plan__meta">{t("caps.pluginCounts", { skills: approval.skills, hooks: approval.hooks, mcps: approval.mcpServers })}</div>
			<div className="cap-detail-actions">
				<button id={`plugin-${approval.name}-enable-cancel`} className="btn btn--small" disabled={busy} type="button" onClick={onCancel}>{t("common.cancel")}</button>
				<button id={`plugin-${approval.name}-enable-approve`} className="btn btn--primary btn--small" disabled={busy || !approval.digest} type="button" onClick={onApprove}>{t("caps.pluginApproveEnable")}</button>
			</div>
		</div>
	);
}

function PluginUsageDetails({ plugin }: { plugin: PluginView }) {
	const t = useT();
	const skills = asArray(plugin.skillDetails);
	const hooks = asArray(plugin.hookDetails);
	const mcps = asArray(plugin.mcpServerDetails);
	const hasDetails = skills.length > 0 || hooks.length > 0 || mcps.length > 0;
	return (
		<div className="cap-plugin-usage">
			<div className="cap-plugin-usage__title">{t("caps.pluginUsageTitle")}</div>
			<div className="cap-plugin-usage__hint">
				{plugin.enabled ? t("caps.pluginUsageEnabledHint") : t("caps.pluginUsageDisabledHint")}
			</div>
			{hasDetails ? (
				<div className="cap-plugin-capabilities">
					{skills.length > 0 && <PluginSkillList skills={skills} />}
					{hooks.length > 0 && <PluginHookList hooks={hooks} />}
					{mcps.length > 0 && <PluginMCPList servers={mcps} />}
				</div>
			) : (
				<div className="cap-plugin-usage__empty">{t("caps.pluginNoCapabilityDetails")}</div>
			)}
		</div>
	);
}

function PluginSkillList({ skills }: { skills: PluginSkillView[] }) {
	const t = useT();
	return (
		<div className="cap-plugin-capability">
			<div className="cap-plugin-capability__head">{t("caps.pluginSkillsTitle")}</div>
			<div className="cap-plugin-capability__hint">{t("caps.pluginSkillsHint")}</div>
			<div className="cap-plugin-capability__list">
				{skills.map((skill) => (
					<div className="cap-plugin-capability__item" key={`${skill.name}-${skill.path || skill.invocation || ""}`}>
						<div className="cap-plugin-capability__line">
							<span className="cap-plugin-capability__name">{skill.invocation || `/${skill.name}`}</span>
							{skill.runAs && <span className="cap-source-badge">{skill.runAs}</span>}
						</div>
						<div className="cap-plugin-capability__desc">{skill.description || t("caps.pluginNoDescription")}</div>
					</div>
				))}
			</div>
		</div>
	);
}

function PluginHookList({ hooks }: { hooks: PluginHookView[] }) {
	const t = useT();
	return (
		<div className="cap-plugin-capability">
			<div className="cap-plugin-capability__head">{t("caps.pluginHooksTitle")}</div>
			<div className="cap-plugin-capability__hint">{t("caps.pluginHooksHint")}</div>
			<div className="cap-plugin-capability__list">
				{hooks.map((hook, idx) => {
					const target = hook.command || hook.contextFile || t("caps.pluginHookNoTarget");
					return (
						<div className="cap-plugin-capability__item" key={`${hook.event}-${hook.match || "*"}-${target}-${idx}`}>
							<div className="cap-plugin-capability__line">
								<span className="cap-plugin-capability__name">{hook.event}</span>
								<span className="cap-source-badge">{hook.match || "*"}</span>
							</div>
							<div className="cap-plugin-capability__desc">{hook.description || target}</div>
						</div>
					);
				})}
			</div>
		</div>
	);
}

function PluginMCPList({ servers }: { servers: PluginMCPServerView[] }) {
	const t = useT();
	return (
		<div className="cap-plugin-capability">
			<div className="cap-plugin-capability__head">{t("caps.pluginMCPTitle")}</div>
			<div className="cap-plugin-capability__hint">{t("caps.pluginMCPHint")}</div>
			<div className="cap-plugin-capability__list">
				{servers.map((server) => (
					<div className="cap-plugin-capability__item" key={server.name}>
						<div className="cap-plugin-capability__line">
							<span className="cap-plugin-capability__name">{server.name}</span>
							{server.transport && <span className="cap-source-badge">{server.transport}</span>}
						</div>
						<div className="cap-plugin-capability__desc">{server.command || server.url || t("caps.pluginMCPNoTarget")}</div>
					</div>
				))}
			</div>
		</div>
	);
}

function normalizePluginViews(plugins: PluginView[] | null | undefined): PluginView[] {
	return sortPluginsForDisplay(asArray(plugins).map(normalizePluginView));
}

function normalizePluginView(plugin: PluginView): PluginView {
	return {
		...plugin,
		name: plugin.name || "plugin",
		root: plugin.root || "",
		enabled: Boolean(plugin.enabled),
		skills: Number.isFinite(plugin.skills) ? plugin.skills : 0,
		hooks: Number.isFinite(plugin.hooks) ? plugin.hooks : 0,
		mcpServers: Number.isFinite(plugin.mcpServers) ? plugin.mcpServers : 0,
		skillDetails: asArray(plugin.skillDetails),
		hookDetails: asArray(plugin.hookDetails),
		mcpServerDetails: asArray(plugin.mcpServerDetails),
		permissions: asArray(plugin.permissions),
		grantedPermissions: asArray(plugin.grantedPermissions),
		rollback: plugin.rollback ? {
			...plugin.rollback,
			enabled: Boolean(plugin.rollback.enabled),
			permissions: asArray(plugin.rollback.permissions),
			grantedPermissions: asArray(plugin.rollback.grantedPermissions),
		} : undefined,
		warnings: asArray(plugin.warnings),
	};
}

function sortPluginsForDisplay(plugins: PluginView[]): PluginView[] {
	return [...plugins].sort((a, b) => {
		const priority = pluginDisplayPriority(a) - pluginDisplayPriority(b);
		if (priority !== 0) return priority;
		return a.name.localeCompare(b.name, undefined, { sensitivity: "base" });
	});
}

function pluginDisplayPriority(plugin: PluginView): number {
	if (plugin.error) return 0;
	if (plugin.enabled) return 1;
	return 2;
}

function pluginListSummary(plugins: PluginView[], t: ReturnType<typeof useT>): string {
	const enabled = plugins.filter((plugin) => plugin.enabled && !plugin.error).length;
	const issues = plugins.filter((plugin) => Boolean(plugin.error) || asArray(plugin.warnings).length > 0).length;
	return t("caps.pluginsSummary", { enabled, total: plugins.length, issues });
}

function pluginCapabilitiesSummary(plugin: PluginView, t: ReturnType<typeof useT>): string {
	if (plugin.skills === 0 && plugin.hooks === 0 && plugin.mcpServers === 0) return t("caps.pluginNoCapabilities");
	return t("caps.pluginCounts", { skills: plugin.skills, hooks: plugin.hooks, mcps: plugin.mcpServers });
}

function pluginWarnings(plugin: PluginView, diagnostic?: PluginView): string[] {
	const warnings = [...asArray(plugin.warnings), ...asArray(diagnostic?.warnings)];
	return Array.from(new Set(warnings.filter((warning) => warning.trim().length > 0)));
}

function isPluginOperation(value: unknown): value is PluginOperationView {
	if (!value || typeof value !== "object") return false;
	const operation = value as Partial<PluginOperationView>;
	return typeof operation.ok === "boolean" && typeof operation.status === "string" && Array.isArray(operation.actions);
}

function canApplyPluginPlan(plan: PluginOperationView | null | undefined): plan is PluginOperationView & { planId: string } {
	return Boolean(plan?.ok && plan.status === "planned" && plan.planId?.trim());
}

function pluginOperationFailure(operation: PluginOperationView): string {
	const actionError = operation.actions.find((action) => action.error)?.error;
	return actionError || operation.next || asArray(operation.warnings)[0] || `Plugin operation ${operation.status || "failed"}`;
}

function pluginPlanActionLabel(action: PluginOperationAction, t: ReturnType<typeof useT>): string {
	const verb = action.action || action.kind || t("caps.pluginAction");
	return [verb, action.name].filter(Boolean).join(" · ");
}

function pluginOperationNotice(operation: PluginOperationView, t: ReturnType<typeof useT>): PluginNotice | null {
	if (operation.status === "planned") return null;
	if (operation.status === "partial") {
		return { message: operation.next || t("caps.pluginPlanPartial"), tone: "warning" };
	}
	if (operation.status === "done" && operation.ok) {
		return { message: t("caps.pluginPlanComplete"), tone: "success" };
	}
	return { message: pluginOperationFailure(operation), tone: "warning" };
}

// MCPServersSettingsPage is a self-contained MCP servers management page
// embedded inside the settings centre.
export function MCPServersSettingsPage() {
	const t = useT();
	const [snapshotKey, setSnapshotKey] = useState("");
	const [servers, setServers] = useState<ServerView[] | null>(null);
	const [busy, setBusy] = useState(false);
	const [err, setErr] = useState<string | null>(null);
	const [adding, setAdding] = useState(false);
	const [editing, setEditing] = useState<string | null>(null);
	const [expandedErrors, setExpandedErrors] = useState<Set<string>>(() => new Set());
	const [expandedServers, setExpandedServers] = useState<Set<string>>(() => new Set());
	const [expandedServerTools, setExpandedServerTools] = useState<Set<string>>(() => new Set());

	const reload = useCallback(async () => {
		const [meta, tabs] = await Promise.all([
			app.Meta().catch(() => null),
			app.ListTabs().catch(() => []),
		]);
		const key = settingsSnapshotKey(meta, tabs);
		setSnapshotKey(key);
		const cached = key ? mcpSettingsSnapshot : null;
		if (cached?.key === key) {
			setServers(cached.value);
		} else {
			setServers(null);
		}
		const next = normalizeServerViews(await app.MCPServers().catch(() => []));
		mcpSettingsSnapshot = { key, value: next };
		setServers(next);
	}, []);
	useEffect(() => { void reload(); }, [reload]);
	useEffect(() => {
		if (!servers?.some((s) => s.status === "initializing" || s.status === "deferred")) return;
		const id = window.setInterval(() => void reload(), 2500);
		return () => window.clearInterval(id);
	}, [reload, servers]);

	const mutate = async (fn: () => Promise<unknown>) => {
		setBusy(true);
		setErr(null);
		try {
			await fn();
			await reload();
			return true;
		} catch (e) {
			setErr(String((e as Error)?.message ?? e));
			await reload();
			return false;
		} finally {
			setBusy(false);
		}
	};
	const serverGroups = useMemo(() => {
		const sorted = sortServersForDisplay(servers ?? []);
		return {
			failed: sorted.filter((s) => s.status === "failed"),
			active: sorted.filter((s) => s.status !== "failed"),
		};
	}, [servers]);
	const retryableActiveServerNames = useMemo(() => retryableAvailableServerNames(serverGroups.active), [serverGroups.active]);
	const toggleError = useCallback((name: string) => {
		setExpandedErrors((prev) => { const next = new Set(prev); if (next.has(name)) next.delete(name); else next.add(name); return next; });
	}, []);
	const toggleServer = useCallback((name: string) => {
		setExpandedServers((prev) => { const next = new Set(prev); if (next.has(name)) next.delete(name); else next.add(name); return next; });
	}, []);
	const toggleServerTools = useCallback((name: string) => {
		setExpandedServerTools((prev) => { const next = new Set(prev); if (next.has(name)) next.delete(name); else next.add(name); return next; });
	}, []);

	const summary = useMemo(() => {
		if (!servers) return "";
		return mcpServerSummary(servers, t);
	}, [servers, t]);

	const loading = !servers;
	const actionBusy = busy || !snapshotKey || loading;

		return (
			<section className="mem-section">
				{err && serverGroups.failed.length === 0 && <div className="banner banner--error">{err}</div>}
				<div className="cap-mcp-toolbar">
				{servers && servers.length > 0 ? <div className="drawer__summary">{summary}</div> : <span />}
				<div className="cap-mcp-toolbar__actions">
					{!adding && (
						<button className="btn btn--small" disabled={actionBusy} onClick={() => setAdding(true)}>
							{t("caps.addServer")}
						</button>
					)}
				</div>
			</div>
				{serverGroups.failed.length > 0 && (
					<FailedServersNotice
						servers={serverGroups.failed}
						expanded={expandedErrors}
						busy={actionBusy}
						onToggle={toggleError}
						onRetry={(name) => void mutate(() => app.ReconnectMCPServer(name))}
						onRetryMany={(names) => void mutate(() => Promise.allSettled(names.map((name) => app.ReconnectMCPServer(name))))}
					onConfirmClearAuth={(name) => void mutate(() => app.ClearMCPServerAuthentication(name))}
					onConfirm={(name) => void mutate(() => app.RemoveMCPServer(name))}
					onConfirmMany={(names) => void mutate(() => Promise.allSettled(names.map((name) => app.RemoveMCPServer(name))))}
					/>
			)}
			{loading && !adding && (
				<div className="mem-empty">{t("caps.loading")}</div>
			)}
			{!loading && servers.length === 0 && !adding && (
				<div className="mem-empty">{t("caps.noServers")}</div>
			)}
			{serverGroups.active.length > 0 && (
				<div className="cap-server-section">
					<div className="cap-server-section__head">
						<div className="cap-server-section__title">{t("caps.availableServers")}</div>
						<button
							className="btn btn--small"
							disabled={actionBusy || retryableActiveServerNames.length === 0}
							type="button"
							onClick={() => void mutate(() => Promise.allSettled(retryableActiveServerNames.map((name) => app.ReconnectMCPServer(name))))}
						>
							{t("caps.retryAll")}
						</button>
					</div>
						<ServerGroup
							busy={actionBusy}
							servers={serverGroups.active}
							expanded={expandedServers}
						expandedTools={expandedServerTools}
						editing={editing}
						onConfirm={(name) => void mutate(() => app.RemoveMCPServer(name))}
						onEdit={(name) => { setEditing(name); }}
						onCancelEdit={() => setEditing(null)}
						onRetry={(name) => void mutate(() => app.ReconnectMCPServer(name))}
						onReconnect={(name) => void mutate(() => app.ReconnectMCPServer(name))}
						onConfirmClearAuth={(name) => void mutate(() => app.ClearMCPServerAuthentication(name))}
						onTrustTool={(name, toolName) => void mutate(() => app.TrustMCPServerTool(name, toolName))}
						onTrustTools={(name, toolNames) => void mutate(() => app.TrustMCPServerTools(name, toolNames))}
						onUntrustTool={(name, toolName) => void mutate(() => app.UntrustMCPServerTool(name, toolName))}
						onToggle={(name, on) => void mutate(() => app.SetMCPServerEnabled(name, on))}
						onUpdate={(name, input) =>
							void mutate(() => app.UpdateMCPServer(name, input)).then((ok) => {
								if (ok) setEditing(null);
							})
						}
						onToggleDetails={toggleServer}
						onToggleTools={toggleServerTools}
					/>
				</div>
			)}
			{adding ? (
				<AddServerForm busy={busy} onCancel={() => setAdding(false)} onAdd={async (input) => (await mutate(() => app.AddMCPServer(input))) && setAdding(false)} />
			) : null}
		</section>
	);
}

// SkillsSettingsPage is a self-contained skills management page embedded inside
// the settings centre.
export function SkillsSettingsPage() {
	const t = useT();
	const [snapshotKey, setSnapshotKey] = useState("");
	const [view, setView] = useState<SkillsSettingsView | null>(null);
	const [busy, setBusy] = useState(false);
	const [err, setErr] = useState<string | null>(null);
	const [skillQuery, setSkillQuery] = useState("");
	const [expandedSkills, setExpandedSkills] = useState<Set<string>>(() => new Set());

	const reload = useCallback(async () => {
		const [meta, tabs] = await Promise.all([
			app.Meta().catch(() => null),
			app.ListTabs().catch(() => []),
		]);
		const key = settingsSnapshotKey(meta, tabs);
		setSnapshotKey(key);
		const cached = key ? skillsSettingsSnapshot : null;
		if (cached?.key === key) {
			setView(cached.value);
		} else {
			setView(null);
		}
		const next = normalizeSkillsSettingsView(await app.SkillsSettings().catch(() => ({ skills: [], skillRoots: [] })));
		skillsSettingsSnapshot = { key, value: next };
		setView(next);
	}, []);
	useEffect(() => { void reload(); }, [reload]);

	const mutate = async (fn: () => Promise<unknown>) => {
		setBusy(true);
		setErr(null);
		try {
			await fn();
			await reload();
			return true;
		} catch (e) {
			setErr(String((e as Error)?.message ?? e));
			await reload();
			return false;
		} finally {
			setBusy(false);
		}
	};

	const filteredSkills = useMemo(() => {
		if (!view) return [];
		const q = skillQuery.trim().toLowerCase();
		if (!q) return view.skills;
		return view.skills.filter((sk) => {
			const text = [sk.name, "/" + sk.name, sk.description, sk.scope, sk.runAs].join(" ").toLowerCase();
			return text.includes(q);
		});
	}, [view, skillQuery]);

	const skillSummary = useMemo(() => {
		if (!view) return "";
		return skillListSummary(view.skills, filteredSkills, skillQuery.trim().length > 0, t);
	}, [filteredSkills, skillQuery, t, view]);

	const toggleSkill = useCallback((name: string) => {
		setExpandedSkills((prev) => { const next = new Set(prev); if (next.has(name)) next.delete(name); else next.add(name); return next; });
	}, []);

	if (!view) return <div className="empty">{t("caps.loading")}</div>;
	const actionBusy = busy || !snapshotKey;

	return (
		<section className="mem-section">
			{err && <div className="banner banner--error">{err}</div>}
			<div className="cap-search">
				<input
					className="mem-input"
					type="search"
					placeholder={t("caps.searchSkills")}
					value={skillQuery}
					onChange={(e) => setSkillQuery(e.target.value)}
				/>
			</div>
			<SkillSources
				roots={view.skillRoots ?? []}
				busy={actionBusy}
				onAdd={() => mutate(async () => {
					const path = await app.PickSkillFolder();
					if (path) await app.AddSkillPath(path);
				})}
				onRefresh={() => mutate(() => app.RefreshSkills())}
				onRemove={(path) => mutate(() => app.RemoveSkillPath(path))}
			/>
			<div className="cap-skills-head">
				<div className="cap-skills-head__copy">
					<div className="cap-skills-head__title">{t("caps.skills")}</div>
					<div className="cap-skills-head__summary">{skillSummary}</div>
				</div>
			</div>
			{view.skills.length === 0 ? (
				<div className="mem-empty">{t("caps.noSkills")}</div>
			) : filteredSkills.length === 0 ? (
				<div className="mem-empty">{t("caps.noSkillMatches")}</div>
			) : (
				<div className="cap-skills">
					{filteredSkills.map((sk) => (
						<SkillRow
							key={sk.name}
							skill={sk}
							busy={actionBusy}
							expanded={expandedSkills.has(sk.name)}
							onToggle={() => toggleSkill(sk.name)}
							onToggleEnabled={(enabled) => void mutate(() => app.SetSkillEnabled(sk.name, enabled))}
						/>
					))}
				</div>
			)}
		</section>
	);
}
