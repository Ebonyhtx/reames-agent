// Run: tsx src/__tests__/workspace-changes-errors.test.tsx

import { JSDOM } from "jsdom";
import React from "react";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { WorkspacePanel } from "../components/WorkspacePanel";
import { LocaleProvider } from "../lib/i18n";
import type { AppBindings } from "../lib/bridge";
import type { DirEntry, SubagentDeliveryView, WorkspaceChangesView } from "../lib/types";

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

function flushPromises(): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, 0));
}

async function waitFor(label: string, predicate: () => boolean) {
  for (let attempt = 0; attempt < 20; attempt += 1) {
    await act(async () => {
      await flushPromises();
    });
    if (predicate()) return;
  }
  throw new Error(`timed out waiting for ${label}`);
}

class TestResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
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
  globalThis.Event = dom.window.Event;
  globalThis.CustomEvent = dom.window.CustomEvent;
  globalThis.KeyboardEvent = dom.window.KeyboardEvent;
  globalThis.MouseEvent = dom.window.MouseEvent;
  globalThis.PointerEvent = dom.window.MouseEvent as unknown as typeof PointerEvent;
  globalThis.MutationObserver = dom.window.MutationObserver;
  globalThis.ResizeObserver = TestResizeObserver;
  globalThis.localStorage = dom.window.localStorage;
  globalThis.requestAnimationFrame = dom.window.requestAnimationFrame.bind(dom.window);
  globalThis.cancelAnimationFrame = dom.window.cancelAnimationFrame.bind(dom.window);
  Object.defineProperty(dom.window.HTMLElement.prototype, "scrollIntoView", { configurable: true, value: () => {} });
  return dom;
}

async function renderWorkspace(changes: WorkspaceChangesView, methods: Partial<AppBindings> = {}) {
  const dom = installDom();
  window.go = {
    main: {
      App: {
        ListDir: async () => [],
        WorkspaceGitHistory: async () => [],
        WorkspaceChanges: async () => changes,
        SubagentDeliveriesForTab: async () => [],
        ...methods,
      } as Partial<AppBindings> as AppBindings,
    },
  };
  const rootEl = document.getElementById("root");
  if (!rootEl) throw new Error("missing root");
  const root = createRoot(rootEl);
  await act(async () => {
    root.render(
      <LocaleProvider>
        <WorkspacePanel
          open
          tabId="tab-a"
          cwd="/repo"
          maximized={false}
          initialViewMode="changed"
          onClose={() => {}}
          onToggleMaximized={() => {}}
        />
      </LocaleProvider>,
    );
    await flushPromises();
  });
  await waitFor("workspace changes", () => Boolean(document.querySelector(".workspace-preview__body")));
  return { dom, root };
}

async function renderFilesWorkspace(methods: Partial<AppBindings>, props: Partial<Parameters<typeof WorkspacePanel>[0]> = {}) {
  const dom = installDom();
  window.go = {
    main: {
      App: {
        ListDir: async () => [],
        SearchFileRefs: async () => [],
        WorkspaceGitHistory: async () => [],
        WorkspaceChanges: async () => ({ files: [], gitAvailable: true }),
        ...methods,
      } as Partial<AppBindings> as AppBindings,
    },
  };
  const rootEl = document.getElementById("root");
  if (!rootEl) throw new Error("missing root");
  const root = createRoot(rootEl);
  let currentProps: Parameters<typeof WorkspacePanel>[0] = {
    open: true,
    tabId: "tab-a",
    cwd: "/repo",
    maximized: false,
    initialViewMode: "files",
    onClose: () => {},
    onToggleMaximized: () => {},
    ...props,
  };
  const rerender = async (nextProps: Partial<Parameters<typeof WorkspacePanel>[0]> = {}) => {
    currentProps = { ...currentProps, ...nextProps };
    await act(async () => {
      root.render(
        <LocaleProvider>
          <WorkspacePanel {...currentProps} />
        </LocaleProvider>,
      );
      await flushPromises();
    });
  };
  await rerender();
  return { dom, root, rerender };
}

console.log("\nworkspace changes git errors");

{
  const { dom, root } = await renderWorkspace({ files: [], gitAvailable: false });
  await waitFor("git unavailable warning", () => document.body.textContent?.includes("Git status is unavailable for this workspace.") === true);
  ok(document.body.textContent?.includes("Git status is unavailable for this workspace.") === true, "gitAvailable=false renders a warning");
  ok(document.body.textContent?.includes("No changed files") === false, "gitAvailable=false is not shown as a clean workspace");
  await act(async () => {
    root.unmount();
  });
  dom.window.close();
}

{
  const { dom, root } = await renderWorkspace({
    files: [],
    gitAvailable: true,
    gitErr: "git status timed out",
  });
  await waitFor("git error warning without files", () => document.body.textContent?.includes("Git status is unavailable for this workspace.") === true);
  ok(document.body.textContent?.includes("Git status is unavailable for this workspace.") === true, "gitErr without files renders a warning");
  ok(document.body.textContent?.includes("No changed files") === false, "empty files plus gitErr is not shown as a clean workspace");
  await act(async () => {
    root.unmount();
  });
  dom.window.close();
}

{
  const { dom, root } = await renderWorkspace({
    files: [
      {
        path: "src/app.ts",
        sources: ["session"],
        gitStatus: "modified",
        latestPrompt: "edit app",
      },
    ],
    gitAvailable: true,
    gitErr: "git status timed out",
  });
  await waitFor("git error warning with files", () => document.body.textContent?.includes("app.ts") === true);
  ok(document.body.textContent?.includes("Git status is unavailable for this workspace.") === true, "gitErr renders a warning");
  ok(document.body.textContent?.includes("app.ts") === true, "files still render when gitErr is present");
  await act(async () => {
    root.unmount();
  });
  dom.window.close();
}

{
  const calls: string[] = [];
  const listDir = async (dir: string): Promise<DirEntry[]> => {
    calls.push(dir);
    return [];
  };
  const { dom, root, rerender } = await renderFilesWorkspace(
    { ListDir: listDir },
    { fileListRequest: { id: 1, paths: ["src/app.ts"] } },
  );

  await waitFor("initial referenced file dirs", () => calls.filter((dir) => dir === "src/").length === 1);
  await rerender({ fileListRequest: { id: 2, paths: ["src/app.ts"] } });
  await waitFor("referenced file dirs revalidated", () => calls.filter((dir) => dir === "src/").length === 2);

  ok(calls.filter((dir) => dir === "src/").length === 2, "workspace file tree revalidates cached directories for repeated file-list requests");

  await act(async () => {
    root.unmount();
  });
  dom.window.close();
}

{
  const delivery: SubagentDeliveryView = {
    ref: "sa_delivery",
    kind: "task",
    name: "task",
    status: "completed",
    workspace: {
      mode: "git_worktree",
      sourceRoot: "/repo",
      executionRoot: "/state/worktree",
      branch: "reames/subagent-delivery",
    },
    delivery: {
      status: "ready",
      files: [{ path: "src/delivery.ts", kind: "create", added: 2 }],
      commits: [{ hash: "abc123", subject: "seal" }],
      tests: [{ command: "pnpm test", success: true }],
      worktreeLive: true,
    },
    changes: [{ path: "src/delivery.ts", kind: "create", old_text: "", new_text: "ok\n", added: 2, removed: 0, diff: "+ok", binary: false }],
  };
  const mutations: string[] = [];
  const { dom, root } = await renderWorkspace(
    { files: [], gitAvailable: true },
    {
      SubagentDeliveriesForTab: async () => [delivery],
      MutateSubagentDelivery: async (_tab, _ref, op) => {
        mutations.push(op);
        return { ...delivery, delivery: { ...delivery.delivery, status: "applied" } };
      },
    },
  );
  await waitFor("delivery lane", () => document.body.textContent?.includes("reames/subagent-delivery") === true);
  const header = document.querySelector<HTMLButtonElement>(".workspace-delivery__head");
  await act(async () => {
    header?.click();
    await flushPromises();
  });
  ok(document.body.textContent?.includes("src/delivery.ts") === true, "delivery expansion shows changed files");
  ok(document.body.textContent?.includes("pnpm test") === true, "delivery expansion shows test evidence");
  const apply = Array.from(document.querySelectorAll<HTMLButtonElement>(".workspace-delivery__actions button")).find((button) => button.textContent?.includes("Apply"));
  await act(async () => {
    apply?.click();
    await flushPromises();
  });
  await waitFor("delivery apply", () => mutations.includes("apply"));
  ok(mutations.includes("apply"), "delivery apply uses the shared desktop binding");
  await act(async () => {
    root.unmount();
  });
  dom.window.close();
}

{
  const interrupted: SubagentDeliveryView = {
    ref: "sa_acceptance_interrupted",
    kind: "task",
    name: "task",
    status: "completed",
    workspace: {
      mode: "git_worktree",
      sourceRoot: "/repo",
      executionRoot: "/state/worktree",
      branch: "reames/subagent-acceptance-interrupted",
    },
    delivery: {
      status: "acceptance_interrupted",
      lastError: "inspect Git state manually",
      worktreeLive: true,
    },
  };
  const { dom, root } = await renderWorkspace(
    { files: [], gitAvailable: true },
    { SubagentDeliveriesForTab: async () => [interrupted] },
  );
  await waitFor("acceptance interrupted lane", () => document.body.textContent?.includes("reames/subagent-acceptance-interrupted") === true);
  const header = document.querySelector<HTMLButtonElement>(".workspace-delivery__head");
  await act(async () => {
    header?.click();
    await flushPromises();
  });
  ok(document.body.textContent?.includes("Acceptance interrupted") === true, "acceptance interruption has a localized status");
  ok(document.body.textContent?.includes("inspect Git state manually") === true, "acceptance interruption surfaces recovery guidance");
  ok(document.querySelectorAll(".workspace-delivery__actions button").length === 0, "acceptance interruption exposes no automatic source mutation action");
  await act(async () => {
    root.unmount();
  });
  dom.window.close();
}

console.log(`\n${passed} passed, ${failed} failed, ${passed + failed} total`);
if (failed > 0) process.exit(1);
