// Run: tsx src/__tests__/bundle-contract.test.ts

import { readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

let passed = 0;
let failed = 0;

function ok(cond: boolean, label: string) {
  if (cond) {
    process.stdout.write(`  PASS  ${label}\n`);
    passed += 1;
  } else {
    process.stdout.write(`  FAIL  ${label}\n`);
    failed += 1;
  }
}

const here = dirname(fileURLToPath(import.meta.url));
const appSource = readFileSync(resolve(here, "../App.tsx"), "utf8");
const commandPaletteSource = readFileSync(resolve(here, "../components/CommandPalette.tsx"), "utf8");
const settingsSource = readFileSync(resolve(here, "../components/SettingsPanel.tsx"), "utf8");
const settingsRouteSource = readFileSync(resolve(here, "../components/SettingsPanelRoute.tsx"), "utf8");
const appearanceSource = readFileSync(resolve(here, "../components/AppearancePanel.tsx"), "utf8");
const appearanceRouteSource = readFileSync(resolve(here, "../components/AppearancePanelRoute.tsx"), "utf8");
const settingsStyles = readFileSync(resolve(here, "../components/SettingsPanel.css"), "utf8");
const baseStyles = readFileSync(resolve(here, "../styles.css"), "utf8");
const bridgeSource = readFileSync(resolve(here, "../lib/bridge.ts"), "utf8");
const bridgeMockSource = readFileSync(resolve(here, "../lib/bridgeMock.ts"), "utf8");
const virtualMenuSource = readFileSync(resolve(here, "../components/VirtualMenu.tsx"), "utf8");
const virtualMenuImplSource = readFileSync(resolve(here, "../components/VirtualMenuImpl.tsx"), "utf8");
const markdownSource = readFileSync(resolve(here, "../components/Markdown.tsx"), "utf8");
const heartbeatSurfaceSource = readFileSync(resolve(here, "../custom/features/heartbeat/HeartbeatPanelSurface.tsx"), "utf8");
const scrollManagerSource = readFileSync(resolve(here, "../lib/useScrollManager.ts"), "utf8");
const i18nSource = readFileSync(resolve(here, "../lib/i18n.tsx"), "utf8");
const mainSource = readFileSync(resolve(here, "../main.tsx"), "utf8");
const packageSource = readFileSync(resolve(here, "../../package.json"), "utf8");
const viteSource = readFileSync(resolve(here, "../../vite.config.ts"), "utf8");
const distPlaceholder = readFileSync(resolve(here, "../../dist/.gitkeep"));
const wordmarkSource = readFileSync(resolve(here, "../assets/logo-wordmark.svg"), "utf8");

console.log("\nbundle contract");

ok(
  !/import\s+\{[^}]*\}\s+from\s+["']\.\/lib\/sessionExport["']/.test(appSource),
  "App keeps session export code out of the initial chunk",
);
ok(
  appSource.includes('import("./lib/sessionExport")'),
  "App loads session export code on demand",
);
ok(
  !/import\s+\{[^}]*\}\s+from\s+["']\.\/components\/SettingsPanel["']/.test(appSource) &&
    !/import\s+\{[^}]*\}\s+from\s+["']\.\/components\/HistoryPanel["']/.test(appSource),
  "App keeps secondary drawers out of the initial chunk",
);
ok(
  appSource.includes('import("./components/SettingsPanelRoute")') &&
    appSource.includes('import("./components/HistoryPanel")'),
  "App loads secondary drawers on demand",
);
ok(
  bridgeSource.includes('import("./bridgeMock")') &&
    !bridgeSource.includes("function makeMockApp") &&
    bridgeMockSource.includes("export function makeMockApp"),
  "native bridge keeps the browser development mock out of its static module graph",
);
ok(
  virtualMenuSource.includes('import("./VirtualMenuImpl")') &&
    !virtualMenuSource.includes("@tanstack/react-virtual") &&
    virtualMenuImplSource.includes('from "@tanstack/react-virtual"'),
  "composer menus load the TanStack virtualizer only when a virtual menu opens",
);
ok(
  settingsRouteSource.includes('import "./SettingsPanel.css"') &&
    settingsRouteSource.includes('export { SettingsPanel } from "./SettingsPanel"') &&
    !settingsSource.includes('import "./SettingsPanel.css"') &&
    settingsStyles.includes("settings centre modal") &&
    !settingsStyles.includes(".drawer--wide") &&
    !settingsStyles.includes(".bot-detail__summary") &&
    baseStyles.includes(".drawer--wide") &&
    baseStyles.includes(".bot-detail__summary") &&
    !baseStyles.includes("settings centre modal"),
  "settings lazy route owns only settings CSS while shared responsive surfaces stay in the app shell",
);
ok(
  settingsSource.includes('import("./AppearancePanelRoute")') &&
    settingsSource.includes('import("./AppearancePanel")') &&
    appearanceRouteSource.includes('import "./AppearancePanel.css"') &&
    appearanceRouteSource.includes('export { AppearancePanel } from "./AppearancePanel"') &&
    !appearanceSource.includes('import "./AppearancePanel.css"'),
  "Appearance Gallery keeps its stylesheet on the nested lazy route and remains directly testable",
);
ok(
  packageSource.includes("check-css-syntax.mjs src/styles.css src/components/SettingsPanel.css") &&
    packageSource.includes("check-z-index-tokens.mjs src/styles.css src/components/SettingsPanel.css"),
  "every production stylesheet stays behind syntax and z-index gates",
);
ok(
  packageSource.includes('"pretest": "tsx src/__tests__/theme-pack.test.ts && tsx src/__tests__/appearance-panel.test.tsx"') &&
    packageSource.includes('"pretest:all": "tsx src/__tests__/theme-pack.test.ts && tsx src/__tests__/appearance-panel.test.tsx"'),
  "controlled-theme DOM and Gallery interaction contracts run in ordinary and full frontend test lifecycles",
);
ok(
  distPlaceholder.byteLength === 0 &&
    viteSource.includes('writeFile(resolve(distDir, ".gitkeep"), "")'),
  "production builds restore a byte-empty dist placeholder without line-ending worktree drift",
);
const deferredSurfaces = [
  "ApprovalModal",
  "AskCard",
  "ClearContextCard",
  "CommandPalette",
  "ContextPanel",
  "OnboardingOverlay",
  "ShortcutsCheatsheet",
  "TodoPanel",
  "UndoRewindBanner",
  "WorkspacePanel",
];
ok(
  deferredSurfaces.every((name) => appSource.includes(`import("./components/${name}")`)) &&
    appSource.includes('import("./custom/features/heartbeat/HeartbeatPanelSurface")'),
  "App loads closed and secondary surfaces on demand",
);
ok(
  appSource.includes("const [paletteLoaded, setPaletteLoaded] = useState(false)") &&
    appSource.includes("{paletteLoaded && (") &&
    appSource.includes("open={paletteOpen}"),
  "command palette stays mounted after first load so its close transition can finish",
);
ok(
  commandPaletteSource.includes("useDialogFocus(open, dialogRef, inputRef, restoreFocusRef)") &&
    commandPaletteSource.includes("ref={inputRef}") &&
    !commandPaletteSource.includes("inputCallbackRef"),
  "command palette leaves focus capture and restoration to the shared dialog lifecycle",
);
ok(
  appSource.includes("paletteRestoreFocusRef.current = active instanceof HTMLElement") &&
    appSource.includes("restoreFocusRef={paletteRestoreFocusRef}") &&
    appSource.includes("openPalette(event.currentTarget)"),
  "lazy command palette receives the opener captured before its chunk mounts",
);
ok(
  (appSource.match(/data-dialog-return-focus="settings"/g) ?? []).length === 2,
  "workbench and creation settings openers share a stable remount focus key",
);
ok(
  !appSource.includes('import "./custom/features/heartbeat/heartbeat.css"') &&
    heartbeatSurfaceSource.includes('import "./heartbeat.css"'),
  "heartbeat styles follow the deferred feature chunk",
);
ok(
  !packageSource.includes('"@gsap/react"') &&
    !appSource.includes("gsap.registerPlugin") &&
    scrollManagerSource.includes("gsap.registerPlugin(ScrollToPlugin)"),
  "ScrollToPlugin registration stays with its only consumer",
);
ok(
  !/import\s+\{[^}]*\b(?:MCPServersSettingsPage|SkillsSettingsPage|PluginsSettingsPage)\b[^}]*\}\s+from\s+["']\.\/CapabilitiesPanel["']/.test(settingsSource) &&
    !/import\s+\{[^}]*\bMemorySettingsPage\b[^}]*\}\s+from\s+["']\.\/MemoryPanel["']/.test(settingsSource),
  "SettingsPanel keeps secondary settings pages out of the first settings chunk",
);
ok(
  settingsSource.includes('import("./CapabilitiesPanel")') &&
    settingsSource.includes('import("./MemoryPanel")'),
  "SettingsPanel loads secondary settings pages on demand",
);
ok(
  !/from\s+["']qrcode\.react["']/.test(settingsSource),
  "SettingsPanel keeps QR rendering code out of the first settings chunk",
);
ok(
  settingsSource.includes('import("qrcode.react")'),
  "SettingsPanel loads QR rendering code on demand",
);
ok(
  !/from\s+["']react-markdown["']/.test(markdownSource) &&
    !/from\s+["']remark-gfm["']/.test(markdownSource) &&
    !/from\s+["']remark-math["']/.test(markdownSource) &&
    !/from\s+["']rehype-katex["']/.test(markdownSource) &&
    !/katex\/dist\/katex\.min\.css/.test(markdownSource),
  "Markdown wrapper keeps markdown/math vendor code out of the initial chunk",
);
ok(
  markdownSource.includes('import("./MarkdownRenderer")'),
  "Markdown wrapper loads markdown renderer on demand",
);
ok(
  !/import\s+\{\s*zh\s*\}\s+from\s+["']\.\.\/locales\/zh["']/.test(i18nSource) &&
    !/import\s+\{\s*zhTW\s*\}\s+from\s+["']\.\.\/locales\/zh-TW["']/.test(i18nSource),
  "i18n keeps non-English dictionaries out of the initial module graph",
);
ok(
  i18nSource.includes('import("../locales/zh")') &&
    i18nSource.includes('import("../locales/zh-TW")'),
  "i18n loads non-English dictionaries on demand",
);
ok(
  mainSource.includes("await app.DesktopStartupSettings()") &&
    mainSource.includes("return saved || readLegacyLangPref()") &&
    mainSource.indexOf("await app.DesktopStartupSettings()") < mainSource.indexOf("return saved || readLegacyLangPref()") &&
    mainSource.indexOf("await app.DesktopStartupSettings()") < mainSource.indexOf("await preloadInitialLocale(initialLocalePref)"),
  "Desktop resolves the saved language before choosing the startup locale chunk",
);
ok(
  mainSource.includes("await preloadInitialLocale(initialLocalePref)") &&
    mainSource.includes("<LocaleProvider initialPref={initialLocalePref}>") &&
    mainSource.indexOf("await preloadInitialLocale(initialLocalePref)") < mainSource.indexOf("createRoot(rootElement).render("),
  "Desktop preloads and mounts the resolved locale before the first React frame",
);
ok(
  wordmarkSource.includes('data-brand="reames-agent"') &&
    wordmarkSource.includes("<title") &&
    wordmarkSource.includes("Reames Agent</title>"),
  "desktop wordmark identifies the Reames Agent brand",
);

console.log(`\n${passed} passed, ${failed} failed`);
if (failed > 0) process.exit(1);
