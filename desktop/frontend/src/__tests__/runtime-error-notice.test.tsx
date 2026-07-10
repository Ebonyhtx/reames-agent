// Run: tsx src/__tests__/runtime-error-notice.test.tsx

import { JSDOM } from "jsdom";
import React, { act } from "react";
import { createRoot } from "react-dom/client";
import { RuntimeErrorNotice } from "../components/RuntimeErrorNotice";
import { LocaleProvider } from "../lib/i18n";
import type { Item } from "../lib/useController";

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

const dom = new JSDOM("<!doctype html><html><body><div id=\"root\"></div></body></html>", {
  pretendToBeVisual: true,
  url: "http://localhost/",
});
(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
globalThis.window = dom.window as unknown as Window & typeof globalThis;
globalThis.document = dom.window.document;
Object.defineProperty(dom.window.navigator, "language", { configurable: true, value: "en-US" });
Object.defineProperty(globalThis, "navigator", { configurable: true, value: dom.window.navigator });
globalThis.Node = dom.window.Node;
globalThis.Element = dom.window.Element;
globalThis.HTMLElement = dom.window.HTMLElement;
globalThis.MouseEvent = dom.window.MouseEvent;

const rootElement = document.getElementById("root");
if (!rootElement) throw new Error("missing test root");
const root = createRoot(rootElement);
type NoticeItem = Extract<Item, { kind: "notice" }>;

console.log("\nruntime error notice");

let settingsOpened = 0;
const auth: NoticeItem = {
  kind: "notice",
  id: "auth-1",
  level: "warn",
  text: "legacy secret-bearing backend text",
  code: "provider_auth",
  error: { code: "provider_auth", category: "auth", message: "Backend auth text", retryable: false },
};
await act(async () => {
  root.render(
    React.createElement(
      LocaleProvider,
      null,
      React.createElement(RuntimeErrorNotice, { item: auth, onOpenSettings: () => { settingsOpened += 1; } }),
    ),
  );
});
ok(rootElement.textContent?.includes("Authentication failed") === true, "known codes render localized stable copy");
ok(rootElement.textContent?.includes("legacy secret-bearing") === false, "known codes do not render the legacy error string");
const settingsButton = document.getElementById("error-action-settings-provider_auth") as HTMLButtonElement | null;
ok(Boolean(settingsButton), "authentication error exposes a stable settings action");
await act(async () => settingsButton?.click());
eq(settingsOpened, 1, "settings action invokes its handler");

const retried: string[] = [];
const interrupted: NoticeItem = {
  kind: "notice",
  id: "stream-1",
  level: "warn",
  text: "stream closed",
  code: "stream_interrupted",
  retryText: "original prompt",
  error: { code: "stream_interrupted", category: "retryable", message: "Stream interrupted", retryable: true },
};
await act(async () => {
  root.render(
    React.createElement(
      LocaleProvider,
      null,
      React.createElement(RuntimeErrorNotice, { item: interrupted, onRetry: (text) => retried.push(text) }),
    ),
  );
});
ok(rootElement.textContent?.includes("partial response was kept") === true, "stream interruption explains partial-response preservation");
const retryButton = document.getElementById("error-action-retry-stream_interrupted") as HTMLButtonElement | null;
ok(Boolean(retryButton), "retryable error exposes a stable retry action");
await act(async () => retryButton?.click());
eq(retried[0], "Continue from the interrupted response without repeating completed work.", "interrupted stream retries with a continuation prompt");

await act(async () => root.unmount());
dom.window.close();

console.log(`\n${passed} passed, ${failed} failed, ${passed + failed} total`);
if (failed > 0) process.exit(1);
