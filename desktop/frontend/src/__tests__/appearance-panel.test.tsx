// Run: tsx src/__tests__/appearance-panel.test.tsx

import { JSDOM } from "jsdom";
import React, { act } from "react";
import { createRoot } from "react-dom/client";
import { AppearancePanel } from "../components/AppearancePanel";
import type { AppBindings } from "../lib/bridge";
import { LocaleProvider } from "../lib/i18n";
import type { ThemeExperienceView, ThemePackView } from "../lib/types";

function ok(value: unknown, message: string) {
  if (!value) throw new Error(message);
  process.stdout.write(`  PASS  ${message}\n`);
}

function pack(id: string, kind: ThemePackView["kind"], baseStyle: ThemePackView["baseStyle"]): ThemePackView {
  return {
    id,
    name: id === "reames-dawn" ? "Reames Dawn" : id === "user-theme" ? "User Theme" : "Graphite",
    version: kind === "base" ? "" : "1.0.0",
    author: kind === "base" ? "" : "Reames Project",
    description: `${id} description`,
    license: kind === "base" ? "" : "MIT",
    provenance: { kind: kind === "base" ? "" : "original", source: kind === "base" ? "" : "test source" },
    baseStyle,
    tokens: { light: { accent: "#315f8c" }, dark: { accent: "#88baf0" } },
    recipes: { density: "comfortable", corners: "soft" },
    scenes: {},
    kind,
    applied: kind === "base",
    preview: false,
    packageDigest: kind === "base" ? "" : "a".repeat(64),
    contrastWarnings: [],
  };
}

const base = pack("graphite", "base", "graphite");
const official = pack("reames-dawn", "official", "slate");
const user = pack("user-theme", "user", "aurora");

function snapshot(overrides: Partial<ThemeExperienceView> = {}): ThemeExperienceView {
  return {
    themeMode: "dark",
    baseStyle: "graphite",
    effectiveStyle: "graphite",
    appliedThemeId: "",
    previewThemeId: "",
    effectiveId: "",
    packs: [base, official, user],
    warnings: [],
    safeMode: false,
    ...overrides,
  };
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
globalThis.Event = dom.window.Event;
globalThis.MouseEvent = dom.window.MouseEvent;
globalThis.localStorage = dom.window.localStorage;
Object.defineProperty(dom.window.HTMLCanvasElement.prototype, "getContext", { configurable: true, value: () => null });
Object.defineProperty(window, "matchMedia", {
  configurable: true,
  value: () => ({ matches: false, addEventListener() {}, removeEventListener() {}, addListener() {}, removeListener() {} }),
});

const previewCalls: string[] = [];
const applyCalls: string[] = [];
let cancelCalls = 0;
let current = snapshot();
window.go = {
  main: {
    App: {
      ThemeExperience: async () => current,
      PreviewThemePack: async (id: string) => {
        previewCalls.push(id);
        const selected = current.packs.find((item) => item.id === id)!;
        current = snapshot({ previewThemeId: id, effectiveId: id, effectiveStyle: selected.baseStyle });
        return current;
      },
      CancelThemePreview: async () => {
        cancelCalls += 1;
        current = snapshot({ appliedThemeId: current.appliedThemeId, effectiveId: current.appliedThemeId });
        return current;
      },
      ApplyThemePack: async (id: string) => {
        applyCalls.push(id);
        const selected = current.packs.find((item) => item.id === id);
        current = snapshot({ appliedThemeId: id, effectiveId: id, effectiveStyle: selected?.baseStyle ?? "graphite" });
        return current;
      },
      DeleteThemePack: async () => current,
      ImportThemePack: async () => ({ pack: user }),
      ConfirmThemePackImport: async () => ({ pack: user }),
      CancelThemePackImport: async () => {},
      SetDesktopAppearance: async () => {},
    } as Partial<AppBindings> as AppBindings,
  },
};

const flush = () => new Promise((resolve) => setTimeout(resolve, 0));
async function waitFor(label: string, predicate: () => boolean) {
  for (let attempt = 0; attempt < 30; attempt += 1) {
    await act(async () => { await flush(); });
    if (predicate()) return;
  }
  throw new Error(`timed out waiting for ${label}`);
}

function card(name: string): HTMLButtonElement {
  const found = Array.from(document.querySelectorAll<HTMLButtonElement>("button.theme-pack-card"))
    .find((button) => button.textContent?.includes(name));
  if (!found) throw new Error(`missing theme card ${name}`);
  return found;
}

console.log("\nappearance panel interaction contract");
const rootElement = document.getElementById("root");
if (!rootElement) throw new Error("missing root");
const root = createRoot(rootElement);
await act(async () => {
  root.render(
    <LocaleProvider>
      <AppearancePanel
        theme="dark"
        themeStyle="graphite"
        textSize="default"
        showDisplayZoom={false}
        zoomPct={100}
        zoomRestartRequired={false}
        zoomSaving={false}
        zoomRestarting={false}
        fontFamily="system"
        monoFontFamily="system"
        customFontName=""
        customMonoFontName=""
        onTheme={() => {}}
        onCommittedThemeStyle={() => {}}
        onTextSize={() => {}}
        onRestartZoom={async () => {}}
        onRestartForZoom={async () => {}}
        onFontFamily={() => {}}
        onMonoFontFamily={() => {}}
        onCustomFontNameChange={() => {}}
        onCustomMonoFontNameChange={() => {}}
      />
    </LocaleProvider>,
  );
  await flush();
});
await waitFor("Gallery", () => document.querySelectorAll("button.theme-pack-card").length === 3);

await act(async () => { card("Reames Dawn").click(); await flush(); });
await waitFor("official preview", () => previewCalls.at(-1) === "reames-dawn");
ok(!document.querySelector(".theme-pack-selection .btn--danger"), "official packs preview but never expose delete");
const save = document.querySelector<HTMLButtonElement>(".theme-pack-selection .btn--primary");
if (!save) throw new Error("missing apply button");
await act(async () => { save.click(); await flush(); });
await waitFor("official apply", () => applyCalls.at(-1) === "reames-dawn");
ok(applyCalls.length === 1, "official selection applies exactly once through the controlled binding");

await act(async () => { card("User Theme").click(); await flush(); });
await waitFor("user preview", () => previewCalls.at(-1) === "user-theme");
ok(Boolean(document.querySelector(".theme-pack-selection .btn--danger")), "only user packs expose delete");

await act(async () => { root.unmount(); await flush(); });
ok(cancelCalls > 0, "unmount cancels an outstanding preview lease");
dom.window.close();
