import {
  applyTheme,
  getResolvedTheme,
  getTheme,
  getThemeStyle,
  isThemeStyle,
  normalizeThemePreference,
  normalizeThemeStyleForTheme,
  registerThemePackRuntime,
  type ThemeStyle,
} from "./theme";
import type { ThemeActiveView, ThemePackView, ThemeTokenKey } from "./types";

let currentThemePack: ThemePackView | null = null;

const THEME_TOKEN_CSS: Record<ThemeTokenKey, string> = {
  bg: "--bg", bgSoft: "--bg-soft", bgElev: "--bg-elev",
  panel: "--panel", sidebar: "--sidebar-bg", chat: "--chat-bg",
  workspace: "--workspace-preview-bg", workspaceFiles: "--workspace-files-bg",
  border: "--border", borderSoft: "--border-soft",
  fg: "--fg", fgDim: "--fg-dim", fgFaint: "--fg-faint",
  accent: "--accent", accentFg: "--accent-fg",
  ok: "--ok", warn: "--warn", err: "--err",
};

registerThemePackRuntime({
  effectiveStyle(base) {
    const packStyle = currentThemePack?.baseStyle;
    return isThemeStyle(packStyle) ? packStyle : base;
  },
  apply: applyThemePackToDOM,
});

export function getThemePack(): ThemePackView | null {
  return currentThemePack;
}

// applyThemePack applies only allow-listed semantic values produced by the Go
// validator. Passing null restores the configured built-in base style.
export function applyThemePack(pack: ThemePackView | null): void {
  currentThemePack = pack && (pack.kind === "user" || pack.kind === "official") ? pack : null;
  if (typeof document === "undefined") return;
  const base = getThemeStyle();
  document.documentElement.setAttribute("data-theme-style", effectiveStyle(base));
  applyThemePackToDOM();
}

export function restoreActiveTheme(view: ThemeActiveView): void {
  const theme = normalizeThemePreference(view.themeMode);
  const baseStyle = normalizeThemeStyleForTheme(view.baseStyle, theme);
  currentThemePack = view.safeMode ? null : view.pack ?? null;
  applyTheme(theme, view.safeMode ? "graphite" : baseStyle, { persist: false });
}

function effectiveStyle(base: ThemeStyle): ThemeStyle {
  const packStyle = currentThemePack?.baseStyle;
  return isThemeStyle(packStyle) ? packStyle : base;
}

function applyThemePackToDOM(): void {
  if (typeof document === "undefined") return;
  const root = document.documentElement;
  for (const cssVariable of Object.values(THEME_TOKEN_CSS)) root.style.removeProperty(cssVariable);
  for (const property of [
    "--theme-home-background", "--theme-home-position",
    "--theme-workspace-background", "--theme-workspace-position",
    "--theme-density-scale", "--theme-corner-scale",
  ]) root.style.removeProperty(property);

  const pack = currentThemePack;
  if (!pack) {
    root.removeAttribute("data-theme-pack");
    root.removeAttribute("data-theme-density");
    root.removeAttribute("data-theme-corners");
    return;
  }
  root.setAttribute("data-theme-pack", pack.id);
  root.setAttribute("data-theme-density", pack.recipes.density);
  root.setAttribute("data-theme-corners", pack.recipes.corners);
  const tokens = getResolvedTheme() === "light" ? pack.tokens.light : pack.tokens.dark;
  for (const [key, value] of Object.entries(tokens)) {
    const cssVariable = THEME_TOKEN_CSS[key as ThemeTokenKey];
    if (cssVariable && /^#[0-9a-f]{6}([0-9a-f]{2})?$/i.test(value)) root.style.setProperty(cssVariable, value);
  }
  root.style.setProperty("--theme-density-scale", pack.recipes.density === "compact" ? "0.9" : "1");
  root.style.setProperty("--theme-corner-scale", pack.recipes.corners === "square" ? "0" : pack.recipes.corners === "round" ? "1.4" : "1");
  applyScene(root, "home", pack.scenes.home);
  applyScene(root, "workspace", pack.scenes.workspace);
}

function applyScene(root: HTMLElement, slot: "home" | "workspace", scene: ThemePackView["scenes"]["home"]): void {
  if (!scene?.imageUrl || !/^\/__reames_agent_theme_asset\/[0-9a-f]{64}$/.test(scene.imageUrl)) return;
  const opacity = Math.min(1, Math.max(0, scene.opacity));
  const strength = Math.min(1, Math.max(0, scene.overlayStrength));
  const cover = Math.round((1 - (1 - strength) * opacity) * 100);
  const overlay = `color-mix(in srgb, var(--bg) ${cover}%, transparent)`;
  root.style.setProperty(`--theme-${slot}-background`, `linear-gradient(${overlay}, ${overlay}), url("${scene.imageUrl}")`);
  root.style.setProperty(`--theme-${slot}-position`, `${Math.round(scene.focusX * 100)}% ${Math.round(scene.focusY * 100)}%`);
}

// Loading the pack runtime after base-theme startup must preserve the current
// base selection even when there is no active managed pack.
if (typeof document !== "undefined") applyTheme(getTheme(), getThemeStyle(), { persist: false });
