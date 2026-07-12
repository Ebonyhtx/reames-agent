// Run: tsx src/__tests__/command-palette-focus.test.tsx

import { JSDOM } from "jsdom";
import React from "react";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { CommandPalette, type PaletteItem } from "../components/CommandPalette";
import { LocaleProvider } from "../lib/i18n";

let failed = 0;

function ok(value: boolean, label: string) {
  process.stdout.write(`  ${value ? "PASS" : "FAIL"}  ${label}\n`);
  if (!value) failed += 1;
}

const dom = new JSDOM("<!doctype html><html><body><div id='root'></div></body></html>", {
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
globalThis.MutationObserver = dom.window.MutationObserver;
globalThis.requestAnimationFrame = dom.window.requestAnimationFrame.bind(dom.window);
globalThis.cancelAnimationFrame = dom.window.cancelAnimationFrame.bind(dom.window);
Object.defineProperty(dom.window.HTMLElement.prototype, "attachEvent", { configurable: true, value: () => {} });
Object.defineProperty(dom.window.HTMLElement.prototype, "detachEvent", { configurable: true, value: () => {} });
Object.defineProperty(window, "matchMedia", {
  configurable: true,
  value: () => ({
    matches: false,
    media: "(prefers-reduced-motion: reduce)",
    onchange: null,
    addEventListener() {},
    removeEventListener() {},
    addListener() {},
    removeListener() {},
    dispatchEvent: () => false,
  }),
});

const items: PaletteItem[] = [{ id: "cmd-new", group: "Commands", title: "New session", run: () => {} }];

function Harness({ open }: { open: boolean }) {
  return (
    <LocaleProvider>
      <button id="palette-opener" type="button">Open palette</button>
      <CommandPalette open={open} onClose={() => {}} items={items} placeholder="Search" emptyText="No results" />
    </LocaleProvider>
  );
}

console.log("\ncommand palette focus lifecycle");
const root = createRoot(document.getElementById("root")!);
await act(async () => {
  root.render(<Harness open={false} />);
});
const opener = document.getElementById("palette-opener") as HTMLButtonElement;
opener.focus();
await act(async () => {
  root.render(<Harness open />);
});
await act(async () => {
  await new Promise((resolve) => setTimeout(resolve, 80));
});
const input = document.querySelector<HTMLInputElement>(".palette__input");
ok(
  document.activeElement === input,
  `opening focuses the search combobox after its deferred mount (input=${Boolean(input)}, active=${document.activeElement?.tagName}:${document.activeElement?.getAttribute("role") ?? ""})`,
);
await act(async () => {
  root.render(<Harness open={false} />);
});
await act(async () => {
  await new Promise((resolve) => setTimeout(resolve, 250));
});
ok(document.activeElement === opener, "focus remains on the opener after the exit animation removes the combobox");

await act(async () => root.unmount());
dom.window.close();
if (failed > 0) process.exit(1);
