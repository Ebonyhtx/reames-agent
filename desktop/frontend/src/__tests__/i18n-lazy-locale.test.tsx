// Run: tsx src/__tests__/i18n-lazy-locale.test.tsx

import { JSDOM } from "jsdom";
import React, { act } from "react";
import { createRoot } from "react-dom/client";
import {
  LocaleProvider,
  getLocale,
  isLocaleLoaded,
  preloadInitialLocale,
  readLegacyLangPref,
  t,
  useI18n,
  type I18nValue,
} from "../lib/i18n";

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
  ok(actual === expected, actual === expected ? label : `${label}: expected ${JSON.stringify(expected)}, got ${JSON.stringify(actual)}`);
}

async function waitFor(label: string, predicate: () => boolean) {
  for (let attempt = 0; attempt < 100; attempt += 1) {
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 10));
    });
    if (predicate()) return;
  }
  throw new Error(`timed out waiting for ${label}`);
}

console.log("\nlazy locale loading");

const dom = new JSDOM("<!doctype html><html><body><div id=\"root\"></div></body></html>", {
  pretendToBeVisual: true,
  url: "http://localhost/",
});
(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
globalThis.window = dom.window as unknown as Window & typeof globalThis;
globalThis.document = dom.window.document;
Object.defineProperty(globalThis, "navigator", { configurable: true, value: dom.window.navigator });
globalThis.localStorage = dom.window.localStorage;

let i18n: I18nValue | undefined;
function Probe() {
  i18n = useI18n();
  return null;
}

const rootElement = document.getElementById("root");
if (!rootElement) throw new Error("missing root");
const root = createRoot(rootElement);

ok(isLocaleLoaded("en"), "English fallback is synchronously available");
ok(!isLocaleLoaded("zh"), "Simplified Chinese is absent from the initial module graph");
ok(!isLocaleLoaded("zh-TW"), "Traditional Chinese is absent from the initial module graph");

Object.defineProperty(globalThis, "localStorage", {
  configurable: true,
  value: { getItem: () => { throw new DOMException("blocked", "SecurityError"); } },
});
eq(readLegacyLangPref(), "", "blocked legacy storage cannot abort desktop startup");
globalThis.localStorage = dom.window.localStorage;

Object.defineProperty(dom.window.navigator, "language", { configurable: true, value: "zh-TW" });
eq(await preloadInitialLocale("zh"), "zh", "startup preload prefers the saved locale over the OS locale");
ok(isLocaleLoaded("zh"), "startup preload caches only the saved dictionary");
ok(!isLocaleLoaded("zh-TW"), "startup preload leaves the other dictionary lazy");

await act(async () => {
  root.render(<LocaleProvider initialPref="zh"><Probe /></LocaleProvider>);
});
eq(i18n?.locale, "zh", "provider starts on the saved locale without an OS-language frame");
eq(document.documentElement.lang, "zh-CN", "preloaded document language is preserved");

await act(async () => {
  i18n?.setPref("en");
});
await waitFor("English dictionary", () => i18n?.locale === "en");
eq(i18n?.t("runtimeError.unknown"), "The operation could not be completed.", "English text is synchronous");

await act(async () => {
  i18n?.setPref("");
});
await waitFor("Traditional Chinese dictionary", () => i18n?.locale === "zh-TW");
ok(isLocaleLoaded("zh-TW"), "auto mode loads the Traditional Chinese OS locale");
eq(i18n?.t("runtimeError.providerTimeout"), "服務商請求逾時。", "Traditional Chinese switches atomically");
eq(t("runtimeError.providerTimeout"), "服務商請求逾時。", "non-React translator follows the loaded locale");
eq(getLocale(), "zh-TW", "module locale mirror follows the loaded locale");
eq(document.documentElement.lang, "zh-TW", "Traditional Chinese document language is preserved");

await act(async () => {
  i18n?.setPref("zh");
});
await waitFor("Simplified Chinese dictionary", () => i18n?.locale === "zh");
ok(isLocaleLoaded("zh"), "Simplified Chinese startup chunk is reused after selection");
eq(i18n?.t("runtimeError.unknown"), "操作未能完成。", "Simplified Chinese text is available after the cached switch");
eq(document.documentElement.lang, "zh-CN", "Simplified Chinese document language is explicit");

await act(async () => root.unmount());
dom.window.close();

console.log(`\n${passed} passed, ${failed} failed, ${passed + failed} total`);
if (failed > 0) process.exit(1);
