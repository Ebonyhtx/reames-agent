// Run: tsx src/__tests__/theme-pack.test.ts

import { JSDOM } from "jsdom";
import type { ThemePackView } from "../lib/types";

const dom = new JSDOM("<!doctype html><html><body></body></html>", { url: "http://localhost" });
let prefersLight = true;
let mediaListener: (() => void) | null = null;
const mediaQuery = {
  get matches() { return prefersLight; },
  media: "(prefers-color-scheme: light)",
  onchange: null,
  addEventListener: (_name: string, listener: () => void) => { mediaListener = listener; },
  removeEventListener: (_name: string, listener: () => void) => { if (mediaListener === listener) mediaListener = null; },
  addListener: (listener: () => void) => { mediaListener = listener; },
  removeListener: (listener: () => void) => { if (mediaListener === listener) mediaListener = null; },
  dispatchEvent: () => true,
};
Object.defineProperty(dom.window, "matchMedia", { value: () => mediaQuery });
Object.assign(globalThis, { window: dom.window, document: dom.window.document, localStorage: dom.window.localStorage });

const { applyTheme } = await import("../lib/theme");
const { applyThemePack, getThemePack, restoreActiveTheme } = await import("../lib/themePack");

let passed = 0;
let failed = 0;
function ok(value: boolean, label: string) {
  process.stdout.write(`  ${value ? "PASS" : "FAIL"}  ${label}\n`);
  if (value) passed += 1;
  else failed += 1;
}

const pack: ThemePackView = {
  id: "reames-test",
  name: "Reames Test",
  version: "1.0.0",
  license: "CC-BY-4.0",
  provenance: { kind: "original", source: "test" },
  baseStyle: "slate",
  tokens: {
    light: { accent: "#315f8c", bg: "#f7f8fb" },
    dark: { accent: "#88baf0", bg: "#0c0d10" },
  },
  recipes: { density: "compact", corners: "round" },
  scenes: {
    home: {
      image: { file: "home.png", sha256: "a".repeat(64) },
      imageUrl: `/__reames_agent_theme_asset/${"a".repeat(64)}`,
      focusX: .25,
      focusY: .75,
      safeArea: "right",
      opacity: .8,
      overlayStrength: .5,
    },
  },
  kind: "user",
  applied: true,
  preview: false,
  packageDigest: "b".repeat(64),
  contrastWarnings: [],
};

console.log("\ncontrolled theme pack DOM contract");
applyThemePack(pack);
applyTheme("auto", "graphite", { persist: false });
const root = document.documentElement;
ok(root.dataset.themePack === pack.id, "pack identity is projected separately from base style");
ok(root.dataset.themeStyle === "slate", "pack base style controls the effective visual direction");
ok(root.style.getPropertyValue("--accent") === "#315f8c", "auto light mode applies only light semantic tokens");
ok(root.dataset.themeCorners === "round" && root.dataset.themeDensity === "compact", "bounded recipes use enum data attributes");
ok(root.style.getPropertyValue("--theme-home-background").includes(`/__reames_agent_theme_asset/${"a".repeat(64)}`), "scene uses a digest-addressed local URL");
ok(root.style.getPropertyValue("--theme-home-position") === "25% 75%", "scene focus coordinates are projected");

// Reasonix 8bb0e549 regression: a generic settings refresh must update the
// configured base appearance without replacing the active pack in the live DOM.
applyTheme("light", "amber", { persist: false });
ok(getThemePack()?.id === pack.id, "settings refresh preserves the active pack identity");
ok(root.dataset.themeStyle === "slate", "settings refresh preserves the pack effective style");
ok(root.style.getPropertyValue("--accent") === "#315f8c", "settings refresh reapplies pack tokens for the resolved scheme");
applyThemePack(null);
ok(root.dataset.themeStyle === "amber", "clearing after settings refresh restores the latest configured base style");
applyThemePack(pack);
applyTheme("auto", "graphite", { persist: false });

const official = structuredClone(pack) as ThemePackView;
official.id = "reames-official-test";
official.kind = "official";
official.baseStyle = "nocturne";
applyThemePack(official);
ok(root.dataset.themePack === official.id && root.dataset.themeStyle === "nocturne", "immutable official packs use the same controlled runtime projection");

const unknownKind = structuredClone(pack) as ThemePackView;
(unknownKind as unknown as { kind: string }).kind = "remote";
applyThemePack(unknownKind);
ok(getThemePack() === null && !root.hasAttribute("data-theme-pack"), "frontend rejects pack kinds outside the managed allow-list");
applyThemePack(official);

prefersLight = false;
(mediaListener as (() => void) | null)?.();
ok(root.style.getPropertyValue("--accent") === "#88baf0", "auto color-scheme changes reapply dark pack tokens");

const unsafe = structuredClone(pack) as ThemePackView;
unsafe.tokens.dark.accent = "url(https://example.com/track)";
unsafe.scenes.home!.imageUrl = "https://example.com/track.png";
applyThemePack(unsafe);
ok(root.style.getPropertyValue("--accent") === "", "frontend repeats the color allow-list before touching CSS");
ok(root.style.getPropertyValue("--theme-home-background") === "", "frontend refuses non-digest scene URLs");

applyThemePack(null);
ok(getThemePack() === null && !root.hasAttribute("data-theme-pack"), "cancel removes pack tokens and identity");
ok(root.style.getPropertyValue("--accent") === "", "cancel restores stylesheet-owned base tokens");

restoreActiveTheme({ themeMode: "dark", baseStyle: "amber", effectiveStyle: "graphite", safeMode: true });
ok(root.dataset.themeStyle === "graphite" && getThemePack() === null, "Safe Mode restore forces Graphite and no user pack");

const mock = await import("../lib/themePackMock");
const mockExperience = mock.experience();
ok(mockExperience.packs.filter((item) => item.kind === "official").length === 2, "browser mock projects both immutable official packs");
ok(mock.preview("reames-dawn").effectiveId === "reames-dawn", "browser mock previews an official pack");
let officialDeleteRejected = false;
try { mock.remove("reames-dawn"); } catch { officialDeleteRejected = true; }
ok(officialDeleteRejected, "browser mock refuses official pack deletion");

console.log(`\n${passed} passed, ${failed} failed, ${passed + failed} total`);
if (failed > 0) process.exit(1);
