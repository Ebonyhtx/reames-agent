// Run: tsx src/__tests__/bridge-lazy-mock.test.ts

import { app, onEvent, onUpdaterProgress, type AppBindings } from "../lib/bridge";

let passed = 0;
let failed = 0;

function ok(value: boolean, label: string) {
  process.stdout.write(`  ${value ? "PASS" : "FAIL"}  ${label}\n`);
  if (value) passed += 1;
  else failed += 1;
}

function eq(actual: unknown, expected: unknown, label: string) {
  ok(actual === expected, actual === expected ? label : `${label}: expected ${JSON.stringify(expected)}, got ${JSON.stringify(actual)}`);
}

async function waitFor(label: string, predicate: () => boolean) {
  for (let attempt = 0; attempt < 100; attempt += 1) {
    if (predicate()) return;
    await new Promise((resolve) => setTimeout(resolve, 2));
  }
  throw new Error(`timed out waiting for ${label}`);
}

console.log("\nlazy browser bridge mock");

const bridgeWindow = {
  location: { search: "" },
  go: undefined,
  runtime: undefined,
} as unknown as Window & typeof globalThis;
Object.defineProperty(globalThis, "window", { configurable: true, value: bridgeWindow });

const lateApp = {
  async Platform() {
    return "late-wails";
  },
} as AppBindings;

const platform = app.Platform();
window.go = { main: { App: lateApp } };
eq(await platform, "late-wails", "a Wails binding injected during mock loading wins the first method call");

const channels: string[] = [];
let unsubscribed = 0;
window.go = undefined;
window.runtime = undefined;
const stopEvents = onEvent(() => {});
window.go = { main: { App: lateApp } };
window.runtime = {
  EventsOn(channel: string) {
    channels.push(channel);
    return () => { unsubscribed += 1; };
  },
} as unknown as NonNullable<Window["runtime"]>;
await waitFor("agent event subscription", () => channels.includes("agent:event"));
ok(channels.includes("agent:event"), "late Wails injection receives the agent event subscription");

window.go = undefined;
window.runtime = undefined;
const stopUpdater = onUpdaterProgress(() => {});
window.go = { main: { App: lateApp } };
window.runtime = {
  EventsOn(channel: string) {
    channels.push(channel);
    return () => { unsubscribed += 1; };
  },
} as unknown as NonNullable<Window["runtime"]>;
await waitFor("updater subscription", () => channels.includes("updater:progress"));
ok(channels.includes("updater:progress"), "late Wails injection receives the updater subscription");

stopEvents();
stopUpdater();
eq(unsubscribed, 2, "late native subscriptions are disposed normally");

window.go = undefined;
window.runtime = undefined;
eq(await app.Platform(), "linux", "plain browser development still falls back to the lazy mock");

console.log(`\n${passed} passed, ${failed} failed, ${passed + failed} total`);
if (failed > 0) process.exit(1);
