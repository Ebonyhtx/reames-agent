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
const markdownSource = readFileSync(resolve(here, "../components/Markdown.tsx"), "utf8");
const heartbeatSurfaceSource = readFileSync(resolve(here, "../custom/features/heartbeat/HeartbeatPanelSurface.tsx"), "utf8");
const scrollManagerSource = readFileSync(resolve(here, "../lib/useScrollManager.ts"), "utf8");
const packageSource = readFileSync(resolve(here, "../../package.json"), "utf8");
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
  appSource.includes('import("./components/SettingsPanel")') &&
    appSource.includes('import("./components/HistoryPanel")'),
  "App loads secondary drawers on demand",
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
  wordmarkSource.includes('data-brand="reames-agent"') &&
    wordmarkSource.includes("<title") &&
    wordmarkSource.includes("Reames Agent</title>"),
  "desktop wordmark identifies the Reames Agent brand",
);

console.log(`\n${passed} passed, ${failed} failed`);
if (failed > 0) process.exit(1);
