import { JSDOM } from "jsdom";
import React from "react";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { MCPServersSettingsPage, PluginsSettingsPage } from "../components/CapabilitiesPanel";
import type { AppBindings } from "../lib/bridge";
import { LocaleProvider } from "../lib/i18n";
import { mcpServerLifecycleActions, mcpServerRetryableFromAvailableList } from "../lib/mcpServerLifecycle";
import type { Meta, PluginInstallOptions, PluginOperationView, PluginRegistryEntryView, PluginView, ServerView, TabMeta } from "../lib/types";

function ok(value: unknown, message: string) {
  if (!value) throw new Error(message);
}

function server(status: ServerView["status"]): ServerView {
  return {
    name: "codegraph",
    transport: "stdio",
    status,
    configured: true,
    autoStart: true,
    tier: "background",
    tools: 0,
    prompts: 0,
    resources: 0,
  };
}

const initializing = mcpServerLifecycleActions(server("initializing"));
ok(initializing.enabled, "initializing server should still be treated as enabled");
ok(!initializing.showRetryInRow, "initializing server should not expose retry until it fails");
ok(!initializing.canReconnect, "initializing server should not expose reconnect while already connecting");
ok(!initializing.canConnectNow, "initializing server should not use the deferred connect-now action");

const connected = mcpServerLifecycleActions(server("connected"));
ok(!connected.showRetryInRow, "connected server row should keep the toggle UI");
ok(connected.canReconnect, "connected server details should expose reconnect");

const manuallyConnected = mcpServerLifecycleActions({ ...server("connected"), autoStart: false, startIntent: "off", runtimeState: "ready" });
ok(manuallyConnected.enabled, "connected manual server should still render as enabled");
ok(!manuallyConnected.canConnectNow, "connected manual server should not expose connect-now");
ok(manuallyConnected.canReconnect, "connected manual server should expose reconnect");

const automaticIdle = mcpServerLifecycleActions({ ...server("deferred"), startIntent: "automatic" });
ok(!automaticIdle.canConnectNow, "automatic idle server should not look like a manual connector");
ok(!automaticIdle.canReconnect, "automatic idle server should wait for background connection or failure");

const failed = mcpServerLifecycleActions({ ...server("failed"), runtimeState: "issue" });
ok(failed.showRetryInRow, "failed server row should expose retry");

ok(mcpServerRetryableFromAvailableList(server("initializing")), "connecting server should be included in available-list retry all");
ok(mcpServerRetryableFromAvailableList({ ...server("deferred"), startIntent: "automatic" }), "automatic idle server should be included in available-list retry all");
ok(!mcpServerRetryableFromAvailableList(server("connected")), "connected server should be excluded from available-list retry all");
ok(!mcpServerRetryableFromAvailableList({ ...server("disabled"), startIntent: "off" }), "disabled server should be excluded from available-list retry all");
ok(!mcpServerRetryableFromAvailableList({ ...server("failed"), runtimeState: "issue" }), "failed server is handled by the failure banner retry all");

function flush(): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, 0));
}

async function waitFor(label: string, predicate: () => boolean) {
  for (let attempt = 0; attempt < 20; attempt += 1) {
    await act(async () => {
      await flush();
    });
    if (predicate()) return;
  }
  throw new Error(`timed out waiting for ${label}: ${document.body.textContent?.trim() ?? ""}`);
}

function installDom() {
  const dom = new JSDOM("<!doctype html><html><body><div id=\"root\"></div></body></html>", {
    pretendToBeVisual: true,
    url: "http://localhost/",
  });
  (globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
  globalThis.window = dom.window as unknown as Window & typeof globalThis;
  globalThis.document = dom.window.document;
  Object.defineProperty(globalThis, "navigator", { configurable: true, value: dom.window.navigator });
  globalThis.Node = dom.window.Node;
  globalThis.HTMLElement = dom.window.HTMLElement;
  globalThis.HTMLButtonElement = dom.window.HTMLButtonElement;
  globalThis.HTMLInputElement = dom.window.HTMLInputElement;
  globalThis.Event = dom.window.Event;
  globalThis.KeyboardEvent = dom.window.KeyboardEvent;
  globalThis.MouseEvent = dom.window.MouseEvent;
  globalThis.localStorage = dom.window.localStorage;
  globalThis.requestAnimationFrame = dom.window.requestAnimationFrame.bind(dom.window);
  globalThis.cancelAnimationFrame = dom.window.cancelAnimationFrame.bind(dom.window);
  Object.defineProperty(window, "matchMedia", {
    configurable: true,
    value: () => ({
      matches: true,
      media: "(prefers-reduced-motion: reduce)",
      onchange: null,
      addEventListener() {},
      removeEventListener() {},
      addListener() {},
      removeListener() {},
      dispatchEvent: () => false,
    }),
  });
  return dom;
}

function findButton(label: string): HTMLButtonElement | undefined {
  return Array.from(document.querySelectorAll("button")).find((button) => button.textContent?.trim() === label) as HTMLButtonElement | undefined;
}

function setInputValue(input: HTMLInputElement, value: string) {
  const win = input.ownerDocument.defaultView;
  const setter = Object.getOwnPropertyDescriptor((win?.HTMLInputElement ?? HTMLInputElement).prototype, "value")?.set;
  setter?.call(input, value);
  const eventCtor = win?.Event ?? Event;
  const inputEventCtor = win?.InputEvent ?? eventCtor;
  input.dispatchEvent(new inputEventCtor("input", { bubbles: true, data: value, inputType: "insertText" } as InputEventInit));
  input.dispatchEvent(new eventCtor("change", { bubbles: true }));
}

console.log("capabilities panel MCP actions");

{
  const dom = installDom();
  const rootEl = document.getElementById("root");
  if (!rootEl) throw new Error("missing root");
  const root = createRoot(rootEl);
  const meta: Meta = { label: "test", ready: true, eventChannel: "test-channel", cwd: "/tmp/reames-agent-test", workspaceRoot: "/tmp/reames-agent-test" };
  const tabs: TabMeta[] = [{
    id: "tab-1",
    scope: "project",
    workspaceRoot: "/tmp/reames-agent-test",
    workspaceName: "reames-agent-test",
    topicId: "topic-1",
    topicTitle: "Test",
    label: "Test",
    ready: true,
    running: false,
    mode: "normal",
    toolApprovalMode: "auto",
    active: true,
    cwd: "/tmp/reames-agent-test",
  }];
  let trustCalls = 0;
  let bulkTrustCalls = 0;
  let untrustCalls = 0;
  let reverifyCalls = 0;
  let servers: ServerView[] = [{
    name: "github",
    transport: "stdio",
    status: "connected",
    configured: true,
    autoStart: true,
    tools: 4,
    prompts: 0,
    resources: 0,
    toolList: [
      { name: "issue_read", description: "Read issues.", readOnlyHint: true },
      { name: "issue_write", description: "Write issues." },
      { name: "issue_delete", description: "Delete issues.", readOnlyHint: true, destructiveHint: true },
      { name: "broken_read", description: "Broken tool.", readOnlyHint: true, schemaError: "invalid input schema: bad nested type" },
    ],
    trustedReadOnlyTools: [],
  }, {
    name: "linear",
    transport: "stdio",
    status: "connected",
    configured: true,
    autoStart: true,
    tools: 0,
    prompts: 0,
    resources: 0,
    trustState: "changed",
    changedTools: ["removed_read"],
    trustError: "trust receipt needs review",
  }];
  window.go = {
    main: {
      App: {
        Meta: async () => meta,
        ListTabs: async () => tabs,
        MCPServers: async () => servers.map((s) => ({
          ...s,
          toolList: s.toolList?.map((tool) => ({ ...tool })),
          trustedReadOnlyTools: [...(s.trustedReadOnlyTools ?? [])],
        })),
        TrustMCPServerTool: async (name: string, toolName: string) => {
          trustCalls += 1;
          servers = servers.map((s) => s.name === name ? { ...s, trustedReadOnlyTools: [...(s.trustedReadOnlyTools ?? []), toolName] } : s);
        },
        TrustMCPServerTools: async (name: string, toolNames: string[]) => {
          bulkTrustCalls += 1;
          servers = servers.map((s) => {
            if (s.name !== name) return s;
            const trusted = Array.from(new Set([...(s.trustedReadOnlyTools ?? []), ...toolNames]));
            return { ...s, trustedReadOnlyTools: trusted };
          });
        },
        UntrustMCPServerTool: async (name: string, toolName: string) => {
          untrustCalls += 1;
          servers = servers.map((s) => s.name === name ? { ...s, trustedReadOnlyTools: (s.trustedReadOnlyTools ?? []).filter((tool) => tool !== toolName) } : s);
        },
        ReverifyMCPServer: async () => {
          reverifyCalls += 1;
        },
      } as Partial<AppBindings> as AppBindings,
    },
  };

  await act(async () => {
    root.render(React.createElement(LocaleProvider, null, React.createElement(MCPServersSettingsPage)));
    await flush();
  });
  await waitFor("github server row", () => Boolean(document.querySelector(".cap-row__name")?.textContent?.includes("github")));

  const disclosure = document.querySelector<HTMLButtonElement>(".cap-disclosure");
  if (!disclosure) throw new Error("missing MCP disclosure button");
  await act(async () => {
    disclosure.click();
    await flush();
  });

  const trustReadOnly = findButton("Pre-trust read-only (1)");
  if (!trustReadOnly) throw new Error("missing bulk Pre-trust read-only button");
  await act(async () => {
    trustReadOnly.click();
    await flush();
  });
  await waitFor("bulk trusted tool", () => servers[0]?.trustedReadOnlyTools?.includes("issue_read") ?? false);

  const viewTools = findButton("View tools");
  if (!viewTools) throw new Error("missing View tools button");
  await act(async () => {
    viewTools.click();
    await flush();
  });

  await waitFor("trusted badge", () => Boolean(document.querySelector(".cap-tool-trust")?.textContent?.includes("Trusted")));
  await waitFor("unavailable tool", () => Array.from(document.querySelectorAll(".cap-tool-hint--error")).some((node) => node.textContent?.includes("Unavailable")));
  ok(document.body.textContent?.includes("1 unavailable"), "server summary reports the schema-invalid tool as unavailable");
  ok(document.body.textContent?.includes("invalid input schema: bad nested type"), "tool list shows the schema diagnostic");
  ok(document.body.textContent?.includes("Destructive"), "destructive MCP hint is visible in the tool list");
  ok(document.querySelectorAll(".cap-tool-trust-btn").length === 0, "writers, destructive tools, and invalid schemas never expose reader-trust actions");
  const untrust = findButton("Untrust");
  if (!untrust) throw new Error("missing Untrust button");
  await act(async () => {
    untrust.click();
    await flush();
  });
  await waitFor("untrusted tool", () => !(servers[0]?.trustedReadOnlyTools?.includes("issue_read") ?? false));

  await waitFor("Pre-trust button", () => Boolean(findButton("Pre-trust")));
  ok(document.querySelectorAll(".cap-tool-trust-btn").length === 1, "only the eligible declared reader exposes a trust action");
  const trust = findButton("Pre-trust");
  if (!trust) throw new Error("missing Pre-trust button");
  await act(async () => {
    trust.click();
    await flush();
  });

  await waitFor("trusted badge", () => Boolean(document.querySelector(".cap-tool-trust")?.textContent?.includes("Trusted")));
  ok(bulkTrustCalls === 1, "clicking Pre-trust read-only invokes the MCP bulk trust action once");
  ok(untrustCalls === 1, "clicking Untrust invokes the MCP untrust action once");
  ok(trustCalls === 1, "clicking Trust invokes the MCP trust action once");
  ok(servers[0]?.trustedReadOnlyTools?.includes("issue_read") ?? false, "trusted raw tool name is added to the server snapshot");

  const disclosures = document.querySelectorAll<HTMLButtonElement>(".cap-disclosure");
  const linearDisclosure = disclosures[1];
  if (!linearDisclosure) throw new Error("missing capability-drift server disclosure");
  await act(async () => {
    linearDisclosure.click();
    await flush();
  });
  await waitFor("capability drift details", () => Boolean(document.body.textContent?.includes("removed_read")));
  ok(document.body.textContent?.includes("trust receipt needs review"), "trust-store diagnostics are visible in server details");
  const reverifyTrust = findButton("Reverify trust");
  if (!reverifyTrust) throw new Error("missing capability trust re-verification button");
  await act(async () => {
    reverifyTrust.click();
    await flush();
  });
  await waitFor("capability trust reverify call", () => reverifyCalls === 1);

  await act(async () => {
    root.unmount();
  });
  dom.window.close();
}

console.log("capabilities panel identity drift recovery");

{
  const dom = installDom();
  const rootEl = document.getElementById("root");
  if (!rootEl) throw new Error("missing root");
  const root = createRoot(rootEl);
  let reconnectCalls = 0;
  let reverifyCalls = 0;
  const failedServer: ServerView = {
    name: "github",
    transport: "stdio",
    status: "failed",
    configured: true,
    autoStart: true,
    tools: 0,
    prompts: 0,
    resources: 0,
    identityChanged: true,
    trustState: "changed",
    error: "MCP server \"github\" identity changed; blocked before process startup",
  };
  window.go = {
    main: {
      App: {
        Meta: async () => ({ label: "identity", ready: true, eventChannel: "identity-channel", cwd: "/tmp/identity", workspaceRoot: "/tmp/identity" }),
        ListTabs: async (): Promise<TabMeta[]> => [{
          id: "identity-tab",
          scope: "project",
          workspaceRoot: "/tmp/identity",
          workspaceName: "identity",
          topicId: "identity-topic",
          topicTitle: "Identity",
          label: "Identity",
          cwd: "/tmp/identity",
          ready: true,
          running: false,
          mode: "normal",
          toolApprovalMode: "auto",
          active: true,
        }],
        MCPServers: async () => [failedServer],
        ReconnectMCPServer: async () => {
          reconnectCalls += 1;
        },
        ReverifyMCPServer: async () => {
          reverifyCalls += 1;
        },
      } as Partial<AppBindings> as AppBindings,
    },
  };

  await act(async () => {
    root.render(React.createElement(LocaleProvider, null, React.createElement(MCPServersSettingsPage)));
    await flush();
  });
  await waitFor("identity drift failure", () => Boolean(document.body.textContent?.includes("1 MCP startup issues")));

  const retryAll = findButton("Retry all");
  ok(Boolean(retryAll?.disabled), "bulk retry is disabled when every failure requires identity re-verification");
  const showDetails = findButton("View details");
  if (!showDetails) throw new Error("missing identity failure details button");
  await act(async () => {
    showDetails.click();
    await flush();
  });
  const reverify = findButton("Reverify identity");
  if (!reverify) throw new Error(`missing identity re-verification button: ${document.body.textContent?.trim() ?? ""}`);
  ok(!findButton("Retry"), "identity drift does not expose an ordinary reconnect action");
  await act(async () => {
    reverify.click();
    await flush();
  });
  await waitFor("identity reverify call", () => reverifyCalls === 1);
  ok(reconnectCalls === 0, "identity recovery never bypasses explicit re-verification through reconnect");

  await act(async () => {
    root.unmount();
  });
  dom.window.close();
}

console.log("capabilities panel plugin result classification");

{
  const dom = installDom();
  const rootEl = document.getElementById("root");
  if (!rootEl) throw new Error("missing root");
  const root = createRoot(rootEl);
  let blocked = false;
  const operation = (status: "planned" | "partial" | "blocked"): PluginOperationView => ({
    ok: status === "planned",
    status,
    op: "install",
    applied: status === "partial",
    source: "git:github.com/example/plugin",
    kind: "plugin",
    planId: status === "blocked" ? undefined : "classification-plan",
    actions: status === "blocked" ? [] : [{
      kind: "plugin",
      action: "install_plugin_package",
      name: "classification",
      status: status === "planned" ? "planned" : "failed",
      error: status === "partial" ? "hook registration failed" : undefined,
    }],
    next: status === "partial" ? "Some actions succeeded; review failed actions." : status === "blocked" ? "Source is blocked." : undefined,
  });
  window.go = {
    main: {
      App: {
        Meta: async () => ({ label: "classification", ready: true, eventChannel: "plugin-classification", cwd: "/tmp/plugin-classification", workspaceRoot: "/tmp/plugin-classification" }),
        ListTabs: async (): Promise<TabMeta[]> => [{
          id: "plugin-classification",
          scope: "project",
          workspaceRoot: "/tmp/plugin-classification",
          workspaceName: "plugin-classification",
          topicId: "plugin-classification-topic",
          topicTitle: "Plugin classification",
          label: "Plugin classification",
          cwd: "/tmp/plugin-classification",
          ready: true,
          running: false,
          mode: "normal",
          toolApprovalMode: "auto",
          active: true,
        }],
        Plugins: async () => [],
        PlanPluginInstall: async () => operation(blocked ? "blocked" : "planned"),
        InstallPlugin: async (_source: string, options: PluginInstallOptions) => {
          ok(options.planId === "classification-plan", "partial apply uses the preview planId");
          return operation("partial");
        },
        PickPluginFolder: async () => "",
      } as Partial<AppBindings> as AppBindings,
    },
  };

  await act(async () => {
    root.render(React.createElement(LocaleProvider, null, React.createElement(PluginsSettingsPage)));
    await flush();
  });
  await waitFor("classification page", () => Boolean(findButton("Git repository")));
  await act(async () => {
    findButton("Git repository")?.click();
    await flush();
  });
  const source = document.querySelector<HTMLInputElement>('input[aria-label="Git repository URL"]');
  if (!source) throw new Error("missing classification source input");
  await act(async () => {
    setInputValue(source, "git:github.com/example/plugin");
    await flush();
  });
  await act(async () => {
    findButton("Preview")?.click();
    await flush();
  });
  await waitFor("classification apply enabled", () => findButton("Install plugin")?.disabled === false);
  await act(async () => {
    findButton("Install plugin")?.click();
    await flush();
  });
  await waitFor("partial warning", () => document.querySelector(".banner--warning")?.textContent?.includes("Some actions succeeded") ?? false);
  ok(!document.querySelector(".banner--success"), "partial plugin operations never render as success");

  blocked = true;
  await act(async () => {
    setInputValue(source, "git:github.com/example/blocked");
    await flush();
  });
  await act(async () => {
    findButton("Preview")?.click();
    await flush();
  });
  await waitFor("blocked error", () => document.querySelector(".banner--error")?.textContent?.includes("Source is blocked") ?? false);
  ok(findButton("Install plugin")?.disabled === true, "blocked plugin plans cannot be applied");

  await act(async () => {
    root.unmount();
  });
  dom.window.close();
}

console.log("capabilities panel plugin actions");

{
  const dom = installDom();
  const rootEl = document.getElementById("root");
  if (!rootEl) throw new Error("missing root");
  const root = createRoot(rootEl);
  const meta: Meta = { label: "test", ready: true, eventChannel: "plugin-channel", cwd: "/tmp/reames-agent-test", workspaceRoot: "/tmp/reames-agent-test" };
  const tabs: TabMeta[] = [{
    id: "tab-plugin",
    scope: "project",
    workspaceRoot: "/tmp/reames-agent-test",
    workspaceName: "reames-agent-test",
    topicId: "topic-plugin",
    topicTitle: "Plugins",
    label: "Plugins",
    ready: true,
    running: false,
    mode: "normal",
    toolApprovalMode: "auto",
    active: true,
    cwd: "/tmp/reames-agent-test",
  }];
  let planCalls = 0;
  let installCalls = 0;
  let toggleCalls = 0;
  let updatePlanCalls = 0;
  let updateCalls = 0;
  let rollbackPlanCalls = 0;
  let rollbackCalls = 0;
  let doctorCalls = 0;
  let removePlanCalls = 0;
  let removeCalls = 0;
  let pickFolderCalls = 0;
  let registrySearchCalls = 0;
  let latestInstallPlanId = "";
  let enabledBinding: { digest: string; permissions: string[] } | null = null;
  const plannedSources: string[] = [];
  const installedSources: string[] = [];
  const registryQueries: string[] = [];
  const signedRegistryEntry: PluginRegistryEntryView = {
    name: "superpowers",
    description: "Authenticated agent skills and hooks.",
    version: "5.1.0",
    author: "obra",
    category: "workflow",
    source: "https://github.com/obra/superpowers",
    revision: "d72560e462a74e10d161b7f993d5fc3282bfa1e2",
    digest: `sha256-git-tree-v1:${"a".repeat(64)}`,
    permissions: ["skills.load", "hooks.context", "hooks.execute"],
    registryName: "reames-signed-test",
    registryMetadataUrl: "https://registry.example/metadata",
    registryRootVersion: 7,
    registryRootDigest: `sha256:${"b".repeat(64)}`,
    registryEntryDigest: `sha256:${"d".repeat(64)}`,
    provenanceStatus: "tuf-attestation-target-integrity-verified",
    attestationDigest: `sha256:${"c".repeat(64)}`,
  };
  const pluginOperation = (status: string, planId: string, action: string, overrides: Partial<PluginOperationView> = {}): PluginOperationView => ({
    ok: status === "planned" || status === "done",
    status,
    op: action === "rollback_plugin_package" ? "rollback" : action === "uninstall_plugin_package" ? "uninstall" : "install",
    applied: status === "done" || status === "partial",
    planId,
    kind: "plugin",
    actions: [{
      kind: "plugin",
      action,
      name: "superpowers",
      status,
      riskLevel: action === "install_plugin_package" ? "medium" : "high",
      riskReasons: action === "install_plugin_package" ? ["unsigned source"] : ["changes active plugin generation"],
      version: "0.1.1",
      currentVersion: "0.1.0",
      permissions: ["skills.load"],
      addedPermissions: ["skills.load"],
      trustStatus: "github-https-unsigned",
      willEnable: false,
      rollbackAvailable: action !== "install_plugin_package",
    }],
    ...overrides,
  });
  let plugins: PluginView[] = [{
    name: "superpowers",
    version: "0.1.0",
    description: "Shared agent skills and hooks.",
    source: "git:github.com/obra/superpowers",
    root: "~/.reames-agent/plugins/superpowers",
    manifestKind: "reames-agent",
    manifestSchema: 1,
    installMode: "copy",
    sourceKind: "github",
    sourceRevision: "abc123",
    trustStatus: "github-https-unsigned",
    digest: "sha256:superpowers-v1",
    permissions: ["skills.load"],
    grantedPermissions: ["skills.load"],
    lifecycleSecurity: 1,
    enabled: true,
    skills: 2,
    hooks: 1,
    mcpServers: 0,
  }];
  window.go = {
    main: {
      App: {
        Meta: async () => meta,
        ListTabs: async () => tabs,
        Plugins: async () => plugins.map((plugin) => ({ ...plugin, warnings: [...(plugin.warnings ?? [])] })),
        SearchPluginRegistry: async (query: string) => {
          registrySearchCalls += 1;
          registryQueries.push(query);
          return [{ ...signedRegistryEntry, permissions: [...signedRegistryEntry.permissions] }];
        },
        PluginRegistryEntry: async (name: string) => {
          if (name !== signedRegistryEntry.name) throw new Error(`missing registry entry ${name}`);
          return { ...signedRegistryEntry, permissions: [...signedRegistryEntry.permissions] };
        },
        PlanPluginInstall: async (source: string, options: PluginInstallOptions) => {
          planCalls += 1;
          plannedSources.push(source);
          ok(options.dryRun === true, "plugin preview asks for dry-run planning");
          latestInstallPlanId = `install-plan-${planCalls}`;
          const result = pluginOperation("planned", latestInstallPlanId, "install_plugin_package", { source, name: "superpowers" });
          if (source.startsWith("registry:")) {
            Object.assign(result.actions[0] ?? {}, {
              source,
              sourceRevision: "d72560e462a74e10d161b7f993d5fc3282bfa1e2",
              trustStatus: "tuf-registry-signed",
              registryName: "reames-signed-test",
              registryMetadataUrl: "https://registry.example/metadata",
              registryRootVersion: 7,
              registryRootDigest: `sha256:${"b".repeat(64)}`,
              registryEntryDigest: `sha256:${"d".repeat(64)}`,
              provenanceStatus: "tuf-attestation-target-integrity-verified",
              attestationDigest: `sha256:${"c".repeat(64)}`,
              riskReasons: ["plugin package changes executable capabilities"],
            });
          }
          return result;
        },
        InstallPlugin: async (source: string, options: PluginInstallOptions) => {
          installCalls += 1;
          installedSources.push(source);
          ok(options.planId === latestInstallPlanId, "plugin apply echoes the exact preview planId");
          const next: PluginView = {
            name: "superpowers",
            version: "0.1.1",
            description: "Shared agent skills and hooks.",
            source,
            root: "~/.reames-agent/plugins/superpowers",
            manifestKind: "reames-agent",
            manifestSchema: 1,
            installMode: "copy",
            sourceKind: "github",
            sourceRevision: "def456",
            trustStatus: "github-https-unsigned",
            digest: "sha256:superpowers-v2",
            permissions: ["skills.load", "hooks.execute"],
            grantedPermissions: [],
            lifecycleSecurity: 1,
            rollback: {
              version: "0.1.0",
              digest: "sha256:superpowers-v1",
              trustStatus: "github-https-unsigned",
              permissions: ["skills.load"],
              grantedPermissions: ["skills.load"],
              enabled: true,
            },
            enabled: false,
            skills: 3,
            hooks: 1,
            mcpServers: 1,
            skillDetails: [{ name: "plan", description: "Plan work before implementation.", invocation: "/plan", runAs: "inline" }],
            hookDetails: [{ event: "SessionStart", contextFile: "CLAUDE.md", description: "Load startup context." }],
            mcpServerDetails: [{ name: "context", transport: "stdio", command: "node server.js" }],
          };
          plugins = plugins.filter((plugin) => plugin.name !== next.name).concat(next);
          return pluginOperation("done", latestInstallPlanId, "install_plugin_package", { source, name: next.name });
        },
        SetPluginEnabled: async (name: string, enabled: boolean, digest: string, permissions: string[]) => {
          toggleCalls += 1;
          enabledBinding = { digest, permissions: [...permissions] };
          plugins = plugins.map((plugin) => plugin.name === name ? { ...plugin, enabled } : plugin);
        },
        PlanPluginUpdate: async (name: string) => {
          updatePlanCalls += 1;
          return pluginOperation("planned", "update-plan", "update_plugin_package", { name });
        },
        UpdatePlugin: async (name: string, planId: string) => {
          updateCalls += 1;
          ok(planId === "update-plan", "plugin update echoes the exact preview planId");
          plugins = plugins.map((plugin) => plugin.name === name ? { ...plugin, version: "0.1.2" } : plugin);
          return pluginOperation("done", planId, "update_plugin_package", { name });
        },
        PlanPluginRollback: async (name: string) => {
          rollbackPlanCalls += 1;
          return pluginOperation("planned", "rollback-plan", "rollback_plugin_package", { name });
        },
        RollbackPlugin: async (name: string, planId: string) => {
          rollbackCalls += 1;
          ok(planId === "rollback-plan", "plugin rollback echoes the exact preview planId");
          plugins = plugins.map((plugin) => plugin.name === name ? { ...plugin, version: plugin.rollback?.version || plugin.version } : plugin);
          return pluginOperation("done", planId, "rollback_plugin_package", { name });
        },
        PluginDoctor: async (name: string) => {
          doctorCalls += 1;
          return { ...(plugins.find((plugin) => plugin.name === name) ?? plugins[0]), warnings: ["manifest exports no MCP auth metadata"] };
        },
        PlanPluginRemove: async (name: string) => {
          removePlanCalls += 1;
          return pluginOperation("planned", "remove-plan", "uninstall_plugin_package", { name });
        },
        RemovePlugin: async (name: string, planId: string) => {
          removeCalls += 1;
          ok(planId === "remove-plan", "plugin removal echoes the exact preview planId");
          plugins = plugins.filter((plugin) => plugin.name !== name);
          return pluginOperation("done", planId, "uninstall_plugin_package", { name });
        },
        PickPluginFolder: async () => {
          pickFolderCalls += 1;
          return "/tmp/superpowers-plugin";
        },
      } as Partial<AppBindings> as AppBindings,
    },
  };

  await act(async () => {
    root.render(React.createElement(LocaleProvider, null, React.createElement(PluginsSettingsPage)));
    await flush();
  });
  await waitFor("superpowers plugin row", () => Boolean(document.querySelector(".cap-row__name")?.textContent?.includes("superpowers")));
  ok(Boolean(document.getElementById("plugin-settings-page")), "plugin settings exposes a stable automation root");
  ok(document.getElementById("plugin-settings-page")?.getAttribute("aria-label") === "Install plugin", "plugin settings root is exposed to accessibility automation");
  ok(Boolean(document.getElementById("plugin-row-superpowers")), "plugin rows expose name-scoped automation IDs");
  ok(document.getElementById("plugin-row-superpowers")?.getAttribute("role") === "group", "plugin rows remain visible in the accessibility tree");
  ok(Boolean(document.querySelector(".cap-plugin-form-grid .cap-plugin-fields--local")), "local plugin install mode uses the shared form grid");
  const localOptionTexts = Array.from(document.querySelectorAll(".cap-plugin-installer__options > .cap-plugin-option-block"))
    .map((option) => option.textContent ?? "");
  ok(localOptionTexts[0]?.includes("Overwrite same-name plugin"), "local install mode shows overwrite before link mode");
  ok(localOptionTexts[1]?.includes("Developer mode: link source folder"), "local install mode shows link mode after overwrite");
  const localSourceInput = document.getElementById("plugin-install-local-source") as HTMLInputElement | null;
  if (!localSourceInput) throw new Error("missing editable local plugin source input");
  await act(async () => {
    setInputValue(localSourceInput, "/tmp/typed-plugin");
    await flush();
  });
  await waitFor("typed local plugin source", () => localSourceInput.value === "/tmp/typed-plugin" && findButton("Preview")?.disabled === false);

  const chooseFolder = findButton("Choose plugin folder");
  if (!chooseFolder) throw new Error("missing plugin folder picker button");
  await act(async () => {
    chooseFolder.click();
    await flush();
  });
  await waitFor("picked plugin folder source", () => localSourceInput.value === "/tmp/superpowers-plugin");
  ok(pickFolderCalls === 1, "clicking Choose folder invokes the plugin folder picker once");
  const localPreview = document.getElementById("plugin-install-preview") as HTMLButtonElement | null;
  const localInstall = document.getElementById("plugin-install-apply") as HTMLButtonElement | null;
  const localOptions = document.querySelectorAll<HTMLInputElement>('.cap-plugin-installer__options input[type="checkbox"]');
  if (!localPreview || !localInstall || !localOptions[1]) throw new Error("missing local plugin plan controls");
  await act(async () => {
    localPreview.click();
    await flush();
  });
  await waitFor("local plugin plan", () => planCalls === 1 && localInstall.disabled === false);
  ok(Boolean(document.getElementById("plugin-install-plan")), "install preview exposes a stable plan automation ID");
  ok(document.getElementById("plugin-install-plan")?.getAttribute("role") === "group", "install preview remains visible in the accessibility tree");
  await act(async () => {
    localOptions[1]?.click();
    await flush();
  });
  await waitFor("link invalidates plan", () => findButton("Install plugin")?.disabled === true);

  const registryMode = findButton("Signed registry");
  if (!registryMode) throw new Error("missing signed registry install mode");
  await act(async () => {
    registryMode.click();
    await flush();
  });
  await waitFor("initial signed registry search", () => registrySearchCalls === 1);
  ok(Boolean(document.querySelector(".cap-plugin-form-grid .cap-plugin-fields--registry")), "signed registry install mode uses the shared form grid");
  const registrySearch = document.getElementById("plugin-registry-search") as HTMLInputElement | null;
  if (!registrySearch) throw new Error("missing signed registry search input");
  await act(async () => {
    setInputValue(registrySearch, "superpowers");
    await flush();
  });
  await act(async () => {
    findButton("Search")?.click();
    await flush();
  });
  await waitFor("explicit signed registry search", () => registrySearchCalls === 2);
  ok(registryQueries[1] === "superpowers", "signed registry search forwards the entered query");
  const registryEntry = document.getElementById("plugin-registry-entry") as HTMLSelectElement | null;
  if (!registryEntry) throw new Error("missing signed registry release selector");
  ok(registryEntry.value === "superpowers", "signed registry search selects the authenticated release");
  ok(document.body.textContent?.includes("Authenticated by reames-signed-test, TUF root version 7") ?? false, "signed registry selection displays trust-root evidence");
  ok(document.body.textContent?.includes(signedRegistryEntry.registryEntryDigest) ?? false, "signed registry selection displays the authenticated entry digest");
  await act(async () => {
    findButton("Preview")?.click();
    await flush();
  });
  await waitFor("signed registry plugin plan", () => planCalls === 2 && findButton("Install plugin")?.disabled === false);
  ok(plannedSources[1] === "registry:superpowers", "signed registry preview plans the stable registry source identity");
  ok(document.body.textContent?.includes("tuf-registry-signed") ?? false, "signed registry plan displays authenticated trust metadata");
  ok(document.body.textContent?.includes("tuf-attestation-target-integrity-verified") ?? false, "signed registry plan scopes attestation evidence to target-byte integrity");

  const gitMode = findButton("Git repository");
  if (!gitMode) throw new Error("missing Git repository install mode");
  await act(async () => {
    gitMode.click();
    await flush();
  });
  ok(Boolean(document.querySelector(".cap-plugin-form-grid .cap-plugin-fields--git")), "Git plugin install mode uses the shared form grid");
  const sourceInput = document.querySelector<HTMLInputElement>('input[aria-label="Git repository URL"]');
  if (!sourceInput) throw new Error("missing plugin git source input");
  await act(async () => {
    setInputValue(sourceInput, "git:github.com/obra/superpowers");
    await flush();
  });
  await waitFor("plugin preview enabled", () => findButton("Preview")?.disabled === false);
  const install = findButton("Install plugin");
  if (!install) throw new Error("missing plugin install button");
  ok(install.disabled, "plugin apply is disabled before a preview plan exists");
  const nameInput = document.querySelector<HTMLInputElement>('input[aria-label="Install name (optional)"]');
  if (!nameInput) throw new Error("missing plugin install name input");
  await act(async () => {
    setInputValue(nameInput, "superpowers");
    await flush();
  });

  const preview = findButton("Preview");
  if (!preview) throw new Error("missing plugin preview button");
  await act(async () => {
    preview.click();
    await flush();
  });
  await waitFor("plugin install plan", () => document.body.textContent?.includes("install_plugin_package") ?? false);
  ok(planCalls === 3, "clicking Preview invokes plugin install planning once for the Git source");
  ok(plannedSources[2] === "git:github.com/obra/superpowers", "plugin preview receives the entered Git source");
  ok(findButton("Install plugin")?.disabled === false, "a planned response with planId enables plugin apply");
  ok(document.body.textContent?.includes("Risk: medium") ?? false, "plugin plan displays risk metadata");
  ok(document.body.textContent?.includes("github-https-unsigned") ?? false, "plugin plan displays trust metadata");
  ok(document.body.textContent?.includes("Added permissions") ?? false, "plugin plan displays permission expansion");

  await act(async () => {
    setInputValue(nameInput, "renamed-plugin");
    await flush();
  });
  await waitFor("name invalidates plan", () => findButton("Install plugin")?.disabled === true);
  await act(async () => {
    setInputValue(nameInput, "superpowers");
    await flush();
  });
  await act(async () => {
    findButton("Preview")?.click();
    await flush();
  });
  await waitFor("replanned after name change", () => planCalls === 4 && findButton("Install plugin")?.disabled === false);
  const replaceInput = document.querySelector<HTMLInputElement>('.cap-plugin-installer__options input[type="checkbox"]');
  if (!replaceInput) throw new Error("missing replace option");
  await act(async () => {
    replaceInput.click();
    await flush();
  });
  await waitFor("replace invalidates plan", () => findButton("Install plugin")?.disabled === true);
  await act(async () => {
    preview.click();
    await flush();
  });
  await waitFor("replanned after replace change", () => planCalls === 5 && findButton("Install plugin")?.disabled === false);
  await act(async () => {
    setInputValue(sourceInput, "git:github.com/obra/superpowers-next");
    await flush();
  });
  await waitFor("source invalidates plan", () => findButton("Install plugin")?.disabled === true);
  await act(async () => {
    setInputValue(sourceInput, "git:github.com/obra/superpowers");
    await flush();
  });
  await act(async () => {
    findButton("Preview")?.click();
    await flush();
  });
  await waitFor("replanned after source change", () => planCalls === 6 && findButton("Install plugin")?.disabled === false);
  await act(async () => {
    install.click();
    await flush();
  });
  await waitFor("plugin install result", () => installCalls === 1 && plugins[0]?.version === "0.1.1");
  ok(installedSources[0] === "git:github.com/obra/superpowers", "plugin install receives the entered Git source");

  const disclosure = document.querySelector<HTMLButtonElement>(".cap-plugin-entry .cap-disclosure");
  if (!disclosure) throw new Error("missing plugin disclosure");
  ok(disclosure.id === "plugin-superpowers-details", "plugin disclosure uses its name-scoped automation ID");
  await act(async () => {
    disclosure.click();
    await flush();
  });
  await waitFor("plugin update action", () => Boolean(findButton("Update")));
  ok(document.body.textContent?.includes("How to use") ?? false, "expanded plugin details explain how to use the plugin");
  ok(document.body.textContent?.includes("/plan") ?? false, "expanded plugin details list exported skill invocations");
  ok(document.body.textContent?.includes("SessionStart") ?? false, "expanded plugin details list exported hooks");
  ok(document.body.textContent?.includes("context") ?? false, "expanded plugin details list exported MCP servers");
  ok(document.body.textContent?.includes("sha256:superpowers-v2") ?? false, "expanded plugin details display the verified digest");
  ok(document.body.textContent?.includes("Required permissions") ?? false, "expanded plugin details display required permissions");
  ok(document.body.textContent?.includes("Rollback target") ?? false, "expanded plugin details display the rollback generation");

  const update = findButton("Update");
  if (!update) throw new Error("missing plugin update button");
  await act(async () => {
    update.click();
    await flush();
  });
  await waitFor("plugin update plan", () => updatePlanCalls === 1 && updateCalls === 0 && Boolean(findButton("Apply update")));
  const applyUpdate = findButton("Apply update");
  if (!applyUpdate) throw new Error("missing plugin apply update button");
  ok(applyUpdate.id === "plugin-superpowers-update", "plugin update keeps one stable automation ID across plan and apply");
  await act(async () => {
    applyUpdate.click();
    await flush();
  });
  await waitFor("plugin update call", () => updateCalls === 1 && plugins[0]?.version === "0.1.2");

  const rollback = findButton("Rollback");
  if (!rollback) throw new Error("missing plugin rollback button");
  await act(async () => {
    rollback.click();
    await flush();
  });
  await waitFor("plugin rollback plan", () => rollbackPlanCalls === 1 && rollbackCalls === 0 && Boolean(findButton("Apply rollback")));
  const applyRollback = findButton("Apply rollback");
  if (!applyRollback) throw new Error("missing plugin apply rollback button");
  ok(applyRollback.id === "plugin-superpowers-rollback", "plugin rollback keeps one stable automation ID across plan and apply");
  await act(async () => {
    applyRollback.click();
    await flush();
  });
  await waitFor("plugin rollback call", () => rollbackCalls === 1 && plugins[0]?.version === "0.1.0");

  const doctor = findButton("Doctor");
  if (!doctor) throw new Error("missing plugin doctor button");
  await act(async () => {
    doctor.click();
    await flush();
  });
  await waitFor("plugin diagnostic warning", () => document.body.textContent?.includes("manifest exports no MCP auth metadata") ?? false);
  ok(doctorCalls === 1, "clicking Doctor invokes plugin diagnostics once");

  const toggle = document.querySelector<HTMLInputElement>(".cap-plugin-entry .cap-switch input");
  if (!toggle) throw new Error("missing plugin enable toggle");
  await act(async () => {
    toggle.click();
    await flush();
  });
  await waitFor("plugin enable review", () => Boolean(findButton("Grant permissions and enable")));
  ok(toggleCalls === 0 && plugins[0]?.enabled === false, "plugin toggle does not grant permissions before explicit review");
  ok(document.body.textContent?.includes("sha256:superpowers-v2") ?? false, "plugin authorization review displays the bound digest");
  ok(document.body.textContent?.includes("skills.load, hooks.execute") ?? false, "plugin authorization review displays the exact permission set");
  const approveEnable = findButton("Grant permissions and enable");
  if (!approveEnable) throw new Error("missing plugin enable approval button");
  ok(approveEnable.id === "plugin-superpowers-enable-approve", "plugin authorization exposes a stable approval automation ID");
  await act(async () => {
    approveEnable.click();
    await flush();
  });
  await waitFor("plugin enabled", () => toggleCalls === 1 && plugins[0]?.enabled === true);
  const approvedBinding = enabledBinding as unknown as { digest: string; permissions: string[] };
  ok(approvedBinding.digest === "sha256:superpowers-v2", "plugin enable binds the displayed digest");
  ok(JSON.stringify(approvedBinding.permissions) === JSON.stringify(["skills.load", "hooks.execute"]), "plugin enable binds the exact displayed permission set");

  const remove = findButton("Remove plugin");
  if (!remove) throw new Error("missing plugin remove button");
  await act(async () => {
    remove.click();
    await flush();
  });
  const confirmRemove = findButton("Confirm remove");
  if (!confirmRemove) throw new Error("missing plugin confirm remove button");
  ok(confirmRemove.id === "plugin-superpowers-remove-confirm", "destructive confirmation exposes a distinct automation ID");
  await act(async () => {
    confirmRemove.click();
    await flush();
  });
  await waitFor("plugin remove plan", () => removePlanCalls === 1 && removeCalls === 0 && Boolean(findButton("Apply removal")));
  const applyRemove = findButton("Apply removal");
  if (!applyRemove) throw new Error("missing apply removal button");
  ok(applyRemove.id === "plugin-superpowers-remove-apply", "planned removal exposes a distinct apply automation ID");
  await act(async () => {
    applyRemove.click();
    await flush();
  });
  await waitFor("plugin removed", () => removeCalls === 1 && plugins.length === 0);

  await act(async () => {
    root.unmount();
  });
  dom.window.close();
}
