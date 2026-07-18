import type { ThemeActiveView, ThemeExperienceView, ThemeImportResult, ThemePackView } from "./types";

const base = (id: string, name: string): ThemePackView => ({
  id, name, provenance: { kind: "", source: "" }, baseStyle: id,
  tokens: { light: {}, dark: {} }, recipes: { density: "comfortable", corners: "soft" },
  scenes: {}, kind: "base", applied: id === "graphite", preview: false, contrastWarnings: [],
});

const official = (id: string, name: string, baseStyle: string, accent: string): ThemePackView => ({
  id, name, version: "1.0.0", author: "Reames Project", description: `${name} official browser preview`, license: "MIT",
  provenance: { kind: "original", source: "Original Reames Agent artwork generated on 2026-07-18", generatedWith: "OpenAI image generation via Codex imagegen" },
  baseStyle, tokens: { light: { accent }, dark: { accent } },
  recipes: { density: id === "reames-workshop" ? "compact" : "comfortable", corners: id === "reames-workshop" ? "square" : "soft" },
  scenes: {}, kind: "official", applied: false, preview: false, packageDigest: id === "reames-dawn" ? "9".repeat(64) : "7".repeat(64), contrastWarnings: [],
});

let state: ThemeExperienceView = {
  themeMode: "auto", baseStyle: "graphite", effectiveStyle: "graphite", warnings: [], safeMode: false,
  packs: [
    ...[["graphite", "Graphite"], ["aurora", "Aurora"], ["slate", "Slate"], ["carbon", "Carbon"], ["nocturne", "Nocturne"], ["amber", "Amber"]].map(([id, name]) => base(id, name)),
    official("reames-dawn", "Reames Dawn", "slate", "#e3a15f"),
    official("reames-workshop", "Reames Workshop", "nocturne", "#58b7bd"),
    {
      id: "local-example", name: "Local Example", version: "1.0.0", license: "MIT",
      provenance: { kind: "original", source: "Reames browser mock user package" }, baseStyle: "aurora",
      tokens: { light: { accent: "#315f8c" }, dark: { accent: "#88baf0" } },
      recipes: { density: "comfortable", corners: "round" }, scenes: {}, kind: "user",
      applied: false, preview: false, packageDigest: "a".repeat(64), contrastWarnings: [],
    },
  ],
};

const clone = <T>(value: T): T => JSON.parse(JSON.stringify(value)) as T;

export function active(): ThemeActiveView {
	const pack = state.packs.find((item) => item.id === state.appliedThemeId && item.kind !== "base");
  return clone({ themeMode: state.themeMode, baseStyle: state.baseStyle, effectiveStyle: pack?.baseStyle ?? state.baseStyle, appliedThemeId: state.appliedThemeId, pack, safeMode: state.safeMode });
}

export function experience(): ThemeExperienceView { return clone(state); }

export function preview(id: string): ThemeExperienceView {
	const pack = state.packs.find((item) => item.id === id && item.kind !== "base");
  if (!pack) throw new Error(`mock theme not found: ${id}`);
  state = { ...state, previewThemeId: id, effectiveId: id, effectiveStyle: pack.baseStyle, packs: state.packs.map((item) => ({ ...item, preview: item.id === id })) };
  return clone(state);
}

export function cancelPreview(): ThemeExperienceView {
  const applied = state.packs.find((item) => item.id === state.appliedThemeId);
  state = { ...state, previewThemeId: undefined, effectiveId: state.appliedThemeId, effectiveStyle: applied?.baseStyle ?? state.baseStyle, packs: state.packs.map((item) => ({ ...item, preview: false })) };
  return clone(state);
}

export function apply(id: string): ThemeExperienceView {
	const pack = id ? state.packs.find((item) => item.id === id && item.kind !== "base") : undefined;
  if (id && !pack) throw new Error(`mock theme not found: ${id}`);
  state = { ...state, appliedThemeId: id || undefined, previewThemeId: undefined, effectiveId: id || undefined, effectiveStyle: pack?.baseStyle ?? state.baseStyle, packs: state.packs.map((item) => ({ ...item, applied: item.id === id || (!id && item.id === state.baseStyle), preview: false })) };
  return clone(state);
}

export function remove(id: string): ThemeExperienceView {
	const pack = state.packs.find((item) => item.id === id);
	if (!pack || pack.kind !== "user") throw new Error(`mock user theme not found: ${id}`);
	if (state.appliedThemeId === id) state.appliedThemeId = undefined;
	state = { ...state, previewThemeId: undefined, effectiveId: state.appliedThemeId, effectiveStyle: state.baseStyle, packs: state.packs.filter((item) => item.id !== id) };
  return clone(state);
}

export function canceledImport(): ThemeImportResult { return { pack: state.packs[0], canceled: true }; }
