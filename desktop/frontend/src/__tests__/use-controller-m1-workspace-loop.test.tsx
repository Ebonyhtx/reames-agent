// Run: tsx src/__tests__/use-controller-m1-workspace-loop.test.tsx

import { JSDOM } from "jsdom";
import React, { act } from "react";
import { createRoot } from "react-dom/client";
import type { AppBindings } from "../lib/bridge";
import { useController } from "../lib/useController";
import type { BalanceInfo, CheckpointMeta, ContextInfo, EffortInfo, HistoryMessage, JobView, Meta, TabMeta } from "../lib/types";

let passed = 0;
let failed = 0;

function ok(value: boolean, label: string) {
  if (value) {
    process.stdout.write(`  PASS  ${label}\n`);
    passed += 1;
  } else {
    process.stdout.write(`  FAIL  ${label}\n`);
    failed += 1;
  }
}

function eq(actual: unknown, expected: unknown, label: string) {
  ok(actual === expected, `${label}${actual === expected ? "" : `: expected ${JSON.stringify(expected)}, got ${JSON.stringify(actual)}`}`);
}

function flushPromises(ms = 0): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

async function waitFor(label: string, predicate: () => boolean) {
  for (let attempt = 0; attempt < 50; attempt += 1) {
    await act(async () => {
      await flushPromises(10);
    });
    if (predicate()) return;
  }
  throw new Error(`timed out waiting for ${label}`);
}

function tabMeta(id: string, workspaceRoot: string, topicId: string, overrides: Partial<TabMeta> = {}): TabMeta {
  return {
    id,
    scope: "project",
    workspaceRoot,
    workspaceName: workspaceRoot.split(/[\\/]/).filter(Boolean).at(-1) || id,
    workspacePath: workspaceRoot,
    topicId,
    topicTitle: topicId,
    sessionPath: `${workspaceRoot}/sessions/${topicId}.jsonl`,
    label: `model-${id}`,
    ready: true,
    running: false,
    cancellable: false,
    mode: "normal",
    toolApprovalMode: "ask",
    tokenMode: "full",
    active: false,
    cwd: workspaceRoot,
    ...overrides,
  };
}

function metaFor(tab: TabMeta): Meta {
  return {
    label: tab.label,
    ready: tab.ready,
    startupErr: tab.startupErr,
    eventChannel: "agent:event",
    cwd: tab.cwd || tab.workspaceRoot,
    workspaceRoot: tab.workspaceRoot,
    workspaceName: tab.workspaceName,
    workspacePath: tab.workspacePath,
    sessionPath: tab.sessionPath,
    gitBranch: tab.gitBranch,
    autoApproveTools: false,
    bypass: false,
    collaborationMode: tab.collaborationMode ?? "normal",
    toolApprovalMode: tab.toolApprovalMode ?? "ask",
    tokenMode: tab.tokenMode ?? "full",
    goal: "",
    goalStatus: "stopped",
  };
}

function userMessage(content: string): HistoryMessage {
  return { role: "user", content };
}

console.log("\nuse controller M1 workspace loop");

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
globalThis.Event = dom.window.Event;
globalThis.CustomEvent = dom.window.CustomEvent;
globalThis.KeyboardEvent = dom.window.KeyboardEvent;
globalThis.MouseEvent = dom.window.MouseEvent;
globalThis.localStorage = dom.window.localStorage;
globalThis.requestAnimationFrame = dom.window.requestAnimationFrame.bind(dom.window);
globalThis.cancelAnimationFrame = dom.window.cancelAnimationFrame.bind(dom.window);

const context: ContextInfo = { used: 0, window: 100, sessionTokens: 0 };
const effort: EffortInfo = { supported: true, current: "auto", default: "auto", levels: ["auto"] };
const balance: BalanceInfo = { available: false, display: "" };
const jobs: JobView[] = [];
const checkpoints: CheckpointMeta[] = [];
const tabA = tabMeta("tab-a", "/repo/workspace-a", "topic-a");
const tabB = tabMeta("tab-b", "/repo/workspace-b", "topic-b");
const tabsById = new Map<string, TabMeta>();
const runningTabs = new Set<string>();
const ensureBlankCalls: Array<{ scope: string; workspaceRoot: string }> = [];
const openProjectCalls: Array<{ workspaceRoot: string; topicId: string }> = [];
const submitCalls: Array<{ tabId: string; prompt: string }> = [];
const cancelCalls: string[] = [];
let backendActiveId = "";

function currentTab(tab: TabMeta): TabMeta {
  const running = runningTabs.has(tab.id);
  return { ...tab, active: tab.id === backendActiveId, running, cancellable: running };
}

function currentTabs(): TabMeta[] {
  return Array.from(tabsById.values()).map(currentTab);
}

window.runtime = {
  EventsOn: () => () => {},
  BrowserOpenURL: () => {},
};
window.go = {
  main: {
    App: {
      ListTabs: async () => currentTabs(),
      EnsureBlankTab: async (scope: string, workspaceRoot: string) => {
        ensureBlankCalls.push({ scope, workspaceRoot });
        tabsById.set(tabA.id, tabA);
        backendActiveId = tabA.id;
        return currentTab(tabA);
      },
      OpenProjectTab: async (workspaceRoot: string, topicId: string) => {
        openProjectCalls.push({ workspaceRoot, topicId });
        const target = workspaceRoot === tabA.workspaceRoot ? tabA : tabB;
        tabsById.set(target.id, target);
        backendActiveId = target.id;
        return currentTab(target);
      },
      MetaForTab: async (tabID: string) => metaFor(currentTab(tabsById.get(tabID) ?? tabA)),
      ContextUsageForTab: async () => context,
      EffortForTab: async () => effort,
      BalanceForTab: async () => balance,
      JobsForTab: async () => jobs,
      CheckpointsForTab: async () => checkpoints,
      HistoryForTab: async (tabID: string) => {
        if (tabID === tabB.id) return [userMessage("workspace B history")];
        return [];
      },
      HistoryPageForTab: async (tabID: string) => {
        const messages = await window.go.main.App.HistoryForTab(tabID);
        return { messages, startTurn: 0, endTurn: messages.filter((message) => message.role === "user").length, totalTurns: messages.filter((message) => message.role === "user").length, hasOlder: false };
      },
      HistoryCheckpointTurnsForTab: async () => [],
      ReplayPendingPrompts: async () => {},
      SubmitToTab: async (tabID: string, prompt: string) => {
        submitCalls.push({ tabId: tabID, prompt });
        runningTabs.add(tabID);
      },
      CancelTab: async (tabID: string) => {
        cancelCalls.push(tabID);
        runningTabs.delete(tabID);
      },
    } as Partial<AppBindings> as AppBindings,
  },
};

type Controller = ReturnType<typeof useController>;
let controller: Controller | undefined;

function Probe() {
  controller = useController();
  return null;
}

const rootEl = document.getElementById("root");
if (!rootEl) throw new Error("missing root");
const root = createRoot(rootEl);

await act(async () => {
  root.render(<Probe />);
  await flushPromises();
});
eq(controller?.activeTabId, undefined, "startup has no active tab before workspace selection");

await act(async () => {
  await controller?.ensureBlankTab("project", tabA.workspaceRoot);
  await flushPromises();
});
await waitFor("blank workspace tab active", () => controller?.activeTabId === tabA.id);
eq(ensureBlankCalls.length, 1, "EnsureBlankTab is called once for the selected workspace");
eq(ensureBlankCalls[0]?.workspaceRoot, tabA.workspaceRoot, "EnsureBlankTab receives workspace A root");

await act(async () => {
  await controller?.send("inspect workspace A");
  await flushPromises();
});
eq(submitCalls.length, 1, "send submits once");
eq(submitCalls[0]?.tabId, tabA.id, "send uses SubmitToTab for workspace A");
eq(submitCalls[0]?.prompt, "inspect workspace A", "send passes the prompt to workspace A");
eq(controller?.state.running, true, "workspace A shows a running turn after send");
ok(controller?.state.items.some((item) => item.kind === "user" && item.text === "inspect workspace A") ?? false, "workspace A keeps its optimistic user turn");

await act(async () => {
  await controller?.openProjectTab(tabB.workspaceRoot, tabB.topicId || "");
  await flushPromises();
});
await waitFor("workspace B active", () => controller?.activeTabId === tabB.id && controller.state.items.some((item) => item.kind === "user" && item.text === "workspace B history"));
eq(openProjectCalls.at(-1)?.workspaceRoot, tabB.workspaceRoot, "OpenProjectTab receives workspace B root");
eq(controller?.state.running, false, "workspace B does not inherit workspace A running state");
ok(controller?.state.items.some((item) => item.kind === "user" && item.text === "workspace B history") ?? false, "workspace B shows its own transcript");

await act(async () => {
  await controller?.openProjectTab(tabA.workspaceRoot, tabA.topicId || "");
  await flushPromises();
});
await waitFor("workspace A reactivated", () => controller?.activeTabId === tabA.id);
eq(controller?.state.running, true, "workspace A running state survives switching away and back");
ok(controller?.state.items.some((item) => item.kind === "user" && item.text === "inspect workspace A") ?? false, "workspace A optimistic turn survives switching away and back");

await act(async () => {
  controller?.cancel();
  await flushPromises();
});
await waitFor("workspace A cancelled", () => cancelCalls.length === 1);
eq(cancelCalls[0], tabA.id, "Stop calls CancelTab for workspace A after switching back");
ok(!cancelCalls.includes(tabB.id), "workspace B is not cancelled by stopping workspace A");
await waitFor("workspace A reconciled idle", () => controller?.state.running === false);
ok(!runningTabs.has(tabA.id), "backend no longer marks workspace A running");
ok(!runningTabs.has(tabB.id), "backend never marked workspace B running");

await act(async () => {
  await controller?.openProjectTab(tabB.workspaceRoot, tabB.topicId || "");
  await flushPromises();
});
await waitFor("workspace B reactivated after stop", () => controller?.activeTabId === tabB.id);
eq(controller?.state.running, false, "workspace B remains idle after workspace A stop");
ok(controller?.state.items.some((item) => item.kind === "user" && item.text === "workspace B history") ?? false, "workspace B transcript remains isolated after workspace A stop");

await act(async () => {
  root.unmount();
});
dom.window.close();

console.log(`\n${passed} passed, ${failed} failed, ${passed + failed} total`);
if (failed > 0) process.exit(1);
