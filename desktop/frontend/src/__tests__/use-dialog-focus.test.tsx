// Run: tsx src/__tests__/use-dialog-focus.test.tsx

import { JSDOM } from "jsdom";
import { readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import React, { useEffect, useRef } from "react";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { isTopModalDialog, useDialogFocus } from "../lib/useDialogFocus";

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

function installDom() {
  const dom = new JSDOM("<!doctype html><html><body><button id='opener' data-dialog-return-focus='settings'>Open</button><div id='root'></div><div id='nested'></div></body></html>", {
    pretendToBeVisual: true,
    url: "http://localhost/",
  });
  (globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
  globalThis.window = dom.window as unknown as Window & typeof globalThis;
  globalThis.document = dom.window.document;
  globalThis.Node = dom.window.Node;
  globalThis.HTMLElement = dom.window.HTMLElement;
  globalThis.HTMLButtonElement = dom.window.HTMLButtonElement;
  globalThis.KeyboardEvent = dom.window.KeyboardEvent;
  globalThis.MutationObserver = dom.window.MutationObserver;
  globalThis.requestAnimationFrame = dom.window.requestAnimationFrame.bind(dom.window);
  globalThis.cancelAnimationFrame = dom.window.cancelAnimationFrame.bind(dom.window);
  return dom;
}

function TestDialog({ id, withControls = true }: { id: string; withControls?: boolean }) {
  const dialogRef = useRef<HTMLDivElement>(null);
  const initialRef = useRef<HTMLButtonElement>(null);
  useDialogFocus(true, dialogRef, withControls ? initialRef : undefined);
  return (
    <div ref={dialogRef} id={id} role="dialog" aria-modal="true" tabIndex={-1}>
      {withControls && <button ref={initialRef} id={`${id}-first`}>First</button>}
      {withControls && <button id={`${id}-last`}>Last</button>}
    </div>
  );
}

function ReopenableDialog({ open }: { open: boolean }) {
  const dialogRef = useRef<HTMLDivElement>(null);
  const firstRef = useRef<HTMLButtonElement>(null);
  useDialogFocus(open, dialogRef, firstRef);
  return (
    <div ref={dialogRef} id="reopenable" role="dialog" aria-modal="true" tabIndex={-1}>
      <button ref={firstRef} id="reopenable-first">First</button>
    </div>
  );
}

function EscapableDialog({ id, onClose }: { id: string; onClose: () => void }) {
  const dialogRef = useRef<HTMLDivElement>(null);
  const firstRef = useRef<HTMLButtonElement>(null);
  useDialogFocus(true, dialogRef, firstRef);
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.key !== "Escape" || !isTopModalDialog(dialogRef.current)) return;
      event.preventDefault();
      event.stopImmediatePropagation();
      onClose();
    };
    document.addEventListener("keydown", onKey, { capture: true });
    return () => document.removeEventListener("keydown", onKey, { capture: true });
  }, [onClose]);
  return (
    <div ref={dialogRef} id={id} role="dialog" aria-modal="true" tabIndex={-1}>
      <button ref={firstRef}>First</button>
    </div>
  );
}

async function render(root: Root, node: React.ReactNode) {
  await act(async () => {
    root.render(node);
  });
}

console.log("\ndialog focus lifecycle");

{
  const dom = installDom();
  const opener = document.getElementById("opener") as HTMLButtonElement;
  const preservedBackground = document.getElementById("nested") as HTMLDivElement;
  preservedBackground.setAttribute("aria-hidden", "false");
  preservedBackground.setAttribute("inert", "preserved");
  opener.focus();
  const root = createRoot(document.getElementById("root")!);
  await render(root, <TestDialog id="primary" />);

  const first = document.getElementById("primary-first") as HTMLButtonElement;
  const last = document.getElementById("primary-last") as HTMLButtonElement;
  ok(document.activeElement === first, "open moves focus to the explicit initial control");
  ok(opener.getAttribute("aria-hidden") === "true" && opener.hasAttribute("inert"), "open removes background siblings from interaction and the accessibility tree");

  last.focus();
  last.dispatchEvent(new KeyboardEvent("keydown", { key: "Tab", bubbles: true, cancelable: true }));
  ok(document.activeElement === first, "Tab wraps from the last control to the first");

  first.focus();
  first.dispatchEvent(new KeyboardEvent("keydown", { key: "Tab", shiftKey: true, bubbles: true, cancelable: true }));
  ok(document.activeElement === last, "Shift+Tab wraps from the first control to the last");

  await act(async () => root.unmount());
  await act(async () => Promise.resolve());
  ok(document.activeElement === opener, "closing restores focus to the opener");
  ok(!opener.hasAttribute("aria-hidden") && !opener.hasAttribute("inert"), "closing restores the background's original accessibility state");
  ok(preservedBackground.getAttribute("aria-hidden") === "false" && preservedBackground.getAttribute("inert") === "preserved", "closing preserves pre-existing background accessibility attributes");
  dom.window.close();
}

{
  const dom = installDom();
  const nativeFocus = dom.window.HTMLElement.prototype.focus;
  let inertFocusRejections = 0;
  dom.window.HTMLElement.prototype.focus = function focus(options?: FocusOptions) {
    if (this.closest("[inert]")) {
      inertFocusRejections += 1;
      return;
    }
    nativeFocus.call(this, options);
  };
  const parentRoot = createRoot(document.getElementById("root")!);
  const childRoot = createRoot(document.getElementById("nested")!);
  await render(parentRoot, <TestDialog id="retry-parent" />);
  await render(childRoot, <TestDialog id="retry-child" />);
  ok(inertFocusRejections >= 1 && document.activeElement?.id === "retry-child-first", "nested dialog retries focus after exposing an inert portal path");
  await act(async () => childRoot.unmount());
  ok(document.activeElement?.id === "retry-parent-first", "closing a child dialog restores its immediate parent control");
  await act(async () => parentRoot.unmount());
  dom.window.close();
}

{
  const dom = installDom();
  const parentRoot = createRoot(document.getElementById("root")!);
  const childRoot = createRoot(document.getElementById("nested")!);
  let parentClosed = 0;
  let childClosed = 0;
  await render(parentRoot, <EscapableDialog id="escape-parent" onClose={() => { parentClosed += 1; }} />);
  await render(childRoot, <EscapableDialog id="escape-child" onClose={() => { childClosed += 1; }} />);
  document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true, cancelable: true }));
  ok(childClosed === 1 && parentClosed === 0, "one Escape reaches only the top nested dialog");
  await act(async () => parentRoot.unmount());
  await act(async () => childRoot.unmount());
  dom.window.close();
}

{
  const dom = installDom();
  const root = createRoot(document.getElementById("root")!);
  await render(root, <ReopenableDialog open />);
  await render(root, <ReopenableDialog open={false} />);
  await render(root, <ReopenableDialog open />);
  await act(async () => new Promise((resolve) => dom.window.setTimeout(resolve, 1050)));
  ok(document.getElementById("opener")!.hasAttribute("inert"), "a stale closing lease cannot expose the background after a fast reopen");

  const lateSibling = document.createElement("button");
  lateSibling.id = "late-background";
  document.body.append(lateSibling);
  await act(async () => Promise.resolve());
  ok(lateSibling.hasAttribute("inert") && lateSibling.getAttribute("aria-hidden") === "true", "background nodes mounted after the dialog are isolated");

  await act(async () => root.unmount());
  await act(async () => Promise.resolve());
  dom.window.close();
}

{
  const dom = installDom();
  const opener = document.getElementById("opener") as HTMLButtonElement;
  opener.focus();
  const root = createRoot(document.getElementById("root")!);
  await render(root, <TestDialog id="remounted-opener" />);

  opener.remove();
  const replacement = document.createElement("button");
  replacement.id = "replacement-opener";
  replacement.setAttribute("data-dialog-return-focus", "settings");
  replacement.textContent = "Open";
  document.body.prepend(replacement);

  await act(async () => root.unmount());
  ok(document.activeElement === replacement, "closing restores focus to a semantically equivalent remounted opener");
  dom.window.close();
}

{
  const dom = installDom();
  const root = createRoot(document.getElementById("root")!);
  await render(root, <TestDialog id="empty" withControls={false} />);
  const dialog = document.getElementById("empty") as HTMLDivElement;
  ok(document.activeElement === dialog, "dialog root is the fallback when no control is focusable");
  dialog.dispatchEvent(new KeyboardEvent("keydown", { key: "Tab", bubbles: true, cancelable: true }));
  ok(document.activeElement === dialog, "empty dialog keeps Tab focus on its root");
  await act(async () => root.unmount());
  dom.window.close();
}

{
  const dom = installDom();
  const opener = document.getElementById("opener") as HTMLButtonElement;
  opener.focus();
  const parentRoot = createRoot(document.getElementById("root")!);
  const childRoot = createRoot(document.getElementById("nested")!);
  await render(parentRoot, <TestDialog id="parent" />);
  const parentFirst = document.getElementById("parent-first") as HTMLButtonElement;
  await render(childRoot, <TestDialog id="child" />);
  const childFirst = document.getElementById("child-first") as HTMLButtonElement;
  const childLast = document.getElementById("child-last") as HTMLButtonElement;
  ok(!document.getElementById("nested")!.hasAttribute("inert") && document.getElementById("root")!.hasAttribute("inert"), "nested dialog exposes its portal path and isolates the parent dialog");

  parentFirst.focus();
  parentFirst.dispatchEvent(new KeyboardEvent("keydown", { key: "Tab", bubbles: true, cancelable: true }));
  ok(document.activeElement === childFirst, "only the top nested dialog contains focus");

  await act(async () => parentRoot.unmount());
  ok(document.activeElement === childFirst, "closing a background dialog does not steal focus from the top dialog");
  childLast.focus();
  await act(async () => childRoot.unmount());
  ok(document.activeElement === opener, "closing a replacement dialog follows the retired parent's restore chain to the original opener");
  dom.window.close();
}

{
  const dom = installDom();
  const opener = document.getElementById("opener") as HTMLButtonElement;
  opener.focus();
  const root = createRoot(document.getElementById("root")!);
  await render(root, <React.StrictMode><TestDialog id="strict-dialog" /></React.StrictMode>);
  ok(document.activeElement?.id === "strict-dialog-first", "StrictMode effect replay keeps focus inside the dialog");
  await act(async () => root.unmount());
  ok(document.activeElement === opener, "StrictMode effect replay preserves the original opener restore chain");
  dom.window.close();
}

{
  const here = dirname(fileURLToPath(import.meta.url));
  const modalContracts = [
    ["../components/CommandPalette.tsx", "command-palette-dialog"],
    ["../components/SettingsPanel.tsx", "settings-dialog"],
    ["../components/HistoryPanel.tsx", "history-dialog"],
    ["../components/ShortcutsCheatsheet.tsx", "shortcuts-cheatsheet-dialog"],
    ["../components/ImageViewer.tsx", "image-viewer-dialog"],
    ["../components/OnboardingOverlay.tsx", "onboarding-dialog"],
    ["../custom/features/heartbeat/HeartbeatPanel.tsx", "heartbeat-dialog"],
  ] as const;
  for (const [path, id] of modalContracts) {
    const source = readFileSync(resolve(here, path), "utf8");
    ok(source.includes(`id="${id}"`) && source.includes('aria-modal="true"') && source.includes("useDialogFocus("), `${id} is a stable true-modal focus boundary`);
  }

  const settingsSource = readFileSync(resolve(here, "../components/SettingsPanel.tsx"), "utf8");
  ok(!settingsSource.includes('<main className="settings-center__content"') && settingsSource.includes('aria-label={settingsTabLabel(tab, t)}'), "settings content no longer creates a duplicate main landmark and has a region name");
  const historySource = readFileSync(resolve(here, "../components/HistoryPanel.tsx"), "utf8");
  ok(historySource.includes("isTopModalDialog(dialogRef.current)") && historySource.includes("event.nativeEvent.stopImmediatePropagation()"), "History Escape only closes the top modal");
  ok(historySource.includes('logId="history-preview-transcript-log"') && historySource.includes("announcerId={null}"), "History preview keeps transcript ids unique and does not create a second live announcer");
  const promptShelfSource = readFileSync(resolve(here, "../components/PromptShelf.tsx"), "utf8");
  ok(promptShelfSource.includes('aria-modal={role === "dialog" ? "false" : undefined}'), "non-blocking prompt shelf stays outside modal isolation");

  const appSource = readFileSync(resolve(here, "../App.tsx"), "utf8");
  ok(appSource.includes('id="app-main"') && appSource.includes('id="skip-to-composer"'), "app main and skip link expose stable automation ids");
  const settingsOpeners = appSource.match(/<button\s+[\s\S]*?id="settings-open"[\s\S]*?<\/button>/g) ?? [];
  ok(
    settingsOpeners.length === 2 && settingsOpeners.every((button) => button.includes('data-dialog-return-focus="settings"') && button.includes("settingsRestoreFocusRef.current = event.currentTarget") && button.includes('setSettingsTarget("general")')),
    "mutually exclusive settings openers share one stable automation id, capture their opener, and open Settings",
  );
  ok(appSource.includes("restoreFocusRef={settingsRestoreFocusRef}") && appSource.includes("settingsRestoreFocusRef.current = null;"), "Settings receives and clears the explicit opener ref");
}

console.log(`\n${passed} passed, ${failed} failed`);
if (failed > 0) process.exit(1);
