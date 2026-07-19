// Run: tsx src/__tests__/shortcuts-recorder-focus.test.tsx
//
// WebKit does not focus button elements on mouse click. The shortcut recorder
// listens on its button, so it must explicitly take focus and release it on
// blur, Escape, or Tab without storing those cancellation keys.

import { JSDOM } from "jsdom";
import React from "react";
import { act } from "react";
import { createRoot } from "react-dom/client";

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
  globalThis.Event = dom.window.Event;
  globalThis.MouseEvent = dom.window.MouseEvent;
  globalThis.KeyboardEvent = dom.window.KeyboardEvent;
  globalThis.FocusEvent = dom.window.FocusEvent;
  globalThis.CustomEvent = dom.window.CustomEvent;
  globalThis.localStorage = dom.window.localStorage;
  globalThis.requestAnimationFrame = dom.window.requestAnimationFrame.bind(dom.window);
  globalThis.cancelAnimationFrame = dom.window.cancelAnimationFrame.bind(dom.window);
  Object.defineProperty(window, "matchMedia", {
    configurable: true,
    value: () => ({
      matches: false,
      media: "",
      addEventListener: () => {},
      removeEventListener: () => {},
      addListener: () => {},
      removeListener: () => {},
    }),
  });
}

async function main() {
  installDom();

  const { ShortcutsSection } = await import("../components/SettingsPanel");
  const { LocaleProvider } = await import("../lib/i18n");
  const { loadCustomShortcuts, resetCustomShortcuts } = await import("../lib/keyboardShortcuts");

  resetCustomShortcuts();
  const container = document.getElementById("root")!;
  const root = createRoot(container);
  await act(async () => {
    root.render(
      <LocaleProvider>
        <ShortcutsSection />
      </LocaleProvider>,
    );
    await flushPromises();
  });

  const keyButton = container.querySelector<HTMLButtonElement>('[data-shortcut-action="app.newSession"]');
  ok(Boolean(keyButton), "recorder button renders");
  if (!keyButton) throw new Error("no recorder button");

  await act(async () => {
    keyButton.dispatchEvent(new MouseEvent("click", { bubbles: true, cancelable: true }));
    await flushPromises();
  });
  ok(keyButton.classList.contains("shortcuts-settings__key--recording"), "click enters recording state");
  ok(document.activeElement === keyButton, "click explicitly focuses the recorder for WebKit");

  await act(async () => {
    keyButton.dispatchEvent(new KeyboardEvent("keydown", { key: "x", ctrlKey: true, bubbles: true, cancelable: true }));
    await flushPromises();
  });
  const saved = loadCustomShortcuts()["app.newSession"];
  ok(Boolean(saved && saved.key === "x" && saved.ctrl), "focused recorder stores the pressed combo");
  ok(!keyButton.classList.contains("shortcuts-settings__key--recording"), "stored combo exits recording state");

  await act(async () => {
    keyButton.dispatchEvent(new MouseEvent("click", { bubbles: true, cancelable: true }));
    await flushPromises();
    keyButton.dispatchEvent(new FocusEvent("blur", { bubbles: false }));
    keyButton.dispatchEvent(new FocusEvent("focusout", { bubbles: true }));
    await flushPromises();
  });
  ok(!keyButton.classList.contains("shortcuts-settings__key--recording"), "blur cancels recording");

  await act(async () => {
    keyButton.dispatchEvent(new MouseEvent("click", { bubbles: true, cancelable: true }));
    await flushPromises();
    keyButton.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true, cancelable: true }));
    await flushPromises();
  });
  ok(!keyButton.classList.contains("shortcuts-settings__key--recording"), "Escape cancels recording");
  ok(loadCustomShortcuts()["app.newSession"]?.key === "x", "Escape preserves the saved combo");

  await act(async () => {
    keyButton.dispatchEvent(new MouseEvent("click", { bubbles: true, cancelable: true }));
    await flushPromises();
  });
  const tabEvent = new KeyboardEvent("keydown", { key: "Tab", bubbles: true, cancelable: true });
  await act(async () => {
    keyButton.dispatchEvent(tabEvent);
    await flushPromises();
  });
  ok(!tabEvent.defaultPrevented, "Tab remains available for focus navigation");
  ok(document.activeElement !== keyButton, "Tab fallback releases recorder focus");
  ok(!keyButton.classList.contains("shortcuts-settings__key--recording"), "Tab exits recording state");
  ok(loadCustomShortcuts()["app.newSession"]?.key === "x", "Tab preserves the saved combo");

  await act(async () => {
    resetCustomShortcuts();
    root.unmount();
  });

  console.log(`\n${passed} passed, ${failed} failed, ${passed + failed} total`);
  if (failed > 0) process.exit(1);
}

main().catch((error) => {
  console.error(error);
  process.exit(1);
});
