// Run: tsx src/__tests__/recovery-center.test.tsx

import { JSDOM } from "jsdom";
import { readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import * as React from "react";
import { createRoot } from "react-dom/client";
import { RecoveryCenter } from "../components/RecoveryCenter";
import { LocaleProvider } from "../lib/i18n";
import { recoveryNeedsAttention } from "../lib/recovery";
import type { RecoveryActionRequest, RecoveryReport } from "../lib/types";

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

function installDom() {
  const dom = new JSDOM("<!doctype html><html><body><div id='root'></div></body></html>", { pretendToBeVisual: true, url: "http://localhost/" });
  (globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
  globalThis.window = dom.window as unknown as Window & typeof globalThis;
  globalThis.document = dom.window.document;
  globalThis.Node = dom.window.Node;
  globalThis.HTMLElement = dom.window.HTMLElement;
  globalThis.HTMLButtonElement = dom.window.HTMLButtonElement;
  globalThis.MouseEvent = dom.window.MouseEvent;
  Object.defineProperty(globalThis, "navigator", { configurable: true, value: dom.window.navigator });
  return dom;
}

function report(overrides: Partial<RecoveryReport> = {}): RecoveryReport {
  return {
    schemaVersion: 1,
    generatedAt: "2026-07-17T00:00:00Z",
    safeModeRequested: false,
    safeModeRecommended: false,
    startup: { schemaVersion: 1, phase: "healthy", consecutiveFailures: 0 },
    config: {
      checks: [
        { scope: "global", path: "$REAMES_AGENT_HOME/config.toml", exists: false, valid: true },
        { scope: "project", path: "$WORKSPACE/reames-agent.toml", exists: false, valid: true },
      ],
      applied: [],
    },
    configSnapshots: [],
    binaries: [],
    sessions: [],
    plugins: { path: "$REAMES_AGENT_HOME/plugins.json", exists: false },
    findings: [],
    ...overrides,
  };
}

console.log("\nrecovery center");

ok(!recoveryNeedsAttention(report()), "healthy report stays quiet");
ok(recoveryNeedsAttention(report({ safeModeRecommended: true })), "crash-loop recommendation is actionable");
ok(recoveryNeedsAttention(report({ pendingUpdate: { schemaVersion: 1, toVersion: "v2", platform: "test", targetKind: "file", targetPath: "$INSTALL/app", backupPath: "$STATE/app", createdAt: "now" } })), "pending update is actionable");
const sourceRoot = dirname(fileURLToPath(import.meta.url));
const appSource = readFileSync(resolve(sourceRoot, "../App.tsx"), "utf8");
const stylesSource = readFileSync(resolve(sourceRoot, "../styles.css"), "utf8");
ok(appSource.includes("!recoveryForced && (\n        <>\n        <aside"), "forced recovery omits the ordinary sidebar tree");
ok(appSource.includes("!recoveryForced && workspacePanelRenderable"), "forced recovery omits the workspace dock");
ok(appSource.includes("!sidebarImDetailConnection && !recoveryVisible"), "recovery surface omits the ordinary composer footer");
ok(appSource.includes("recoveryRequestSeq.current += 1"), "completed actions invalidate older in-flight refreshes");
ok(stylesSource.includes(".layout.layout--recovery"), "recovery shell owns a single-column layout");

{
  const dom = installDom();
  const root = createRoot(document.getElementById("root")!);
  let action: RecoveryActionRequest | null = null;
  const unsafe = report({
    safeModeRequested: true,
    config: {
      checks: [
        { scope: "global", path: "$REAMES_AGENT_HOME/config.toml", exists: true, valid: false, error: "invalid TOML" },
        { scope: "project", path: "$WORKSPACE/reames-agent.toml", exists: false, valid: true },
      ],
      applied: [],
    },
    findings: [{ severity: "error", code: "config.invalid", scope: "global", message: "invalid TOML" }],
  });
  await React.act(async () => {
    root.render(
      <LocaleProvider initialPref="en">
        <RecoveryCenter
          report={unsafe}
          loading={false}
          error=""
          forced
          cancelLabel="Cancel"
          onClose={() => {}}
          onRefresh={() => {}}
          onReport={() => {}}
          onAction={(request) => { action = request; }}
        />
      </LocaleProvider>,
    );
  });

  ok(document.body.textContent?.includes("Safe Mode is active") === true, "forced view explains Safe Mode isolation");
  ok(![...document.querySelectorAll("button")].some((button) => button.textContent?.includes("Back to workspace")), "forced Safe Mode has no close escape");
  const repair = [...document.querySelectorAll<HTMLButtonElement>("button")].find((button) => button.textContent?.includes("Repair global"));
  await React.act(async () => repair?.click());
  const emitted = action as RecoveryActionRequest | null;
  ok(emitted?.action === "repair-config" && emitted.target === "global", "repair button emits one bounded global-config request");

  await React.act(async () => root.unmount());
  dom.window.close();
}

if (failed > 0) {
  console.error(`\n${failed} recovery center test(s) failed; ${passed} passed`);
  process.exit(1);
}
console.log(`\n${passed} recovery center tests passed`);
