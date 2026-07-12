// Run: tsx src/__tests__/use-dialog-focus.test.tsx

import { JSDOM } from "jsdom";
import React, { useRef } from "react";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { useDialogFocus } from "../lib/useDialogFocus";

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

async function render(root: Root, node: React.ReactNode) {
  await act(async () => {
    root.render(node);
  });
}

console.log("\ndialog focus lifecycle");

{
  const dom = installDom();
  const opener = document.getElementById("opener") as HTMLButtonElement;
  opener.focus();
  const root = createRoot(document.getElementById("root")!);
  await render(root, <TestDialog id="primary" />);

  const first = document.getElementById("primary-first") as HTMLButtonElement;
  const last = document.getElementById("primary-last") as HTMLButtonElement;
  ok(document.activeElement === first, "open moves focus to the explicit initial control");

  last.focus();
  last.dispatchEvent(new KeyboardEvent("keydown", { key: "Tab", bubbles: true, cancelable: true }));
  ok(document.activeElement === first, "Tab wraps from the last control to the first");

  first.focus();
  first.dispatchEvent(new KeyboardEvent("keydown", { key: "Tab", shiftKey: true, bubbles: true, cancelable: true }));
  ok(document.activeElement === last, "Shift+Tab wraps from the first control to the last");

  await act(async () => root.unmount());
  ok(document.activeElement === opener, "closing restores focus to the opener");
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

  parentFirst.focus();
  parentFirst.dispatchEvent(new KeyboardEvent("keydown", { key: "Tab", bubbles: true, cancelable: true }));
  ok(document.activeElement === childFirst, "only the top nested dialog contains focus");

  await act(async () => parentRoot.unmount());
  ok(document.activeElement === childFirst, "closing a background dialog does not steal focus from the top dialog");
  childLast.focus();
  await act(async () => childRoot.unmount());
  ok(document.activeElement === parentFirst || document.activeElement === document.body, "nested close never focuses a detached opener");
  dom.window.close();
}

console.log(`\n${passed} passed, ${failed} failed`);
if (failed > 0) process.exit(1);
