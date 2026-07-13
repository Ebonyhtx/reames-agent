import assert from "node:assert/strict";
import { mkdtemp, mkdir, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { evaluateBundleBudget, inspectBundle } from "./check-bundle-budget.mjs";

const root = await mkdtemp(path.join(os.tmpdir(), "reames-bundle-budget-"));
let passed = 0;

async function check(label, run) {
  await run();
  passed += 1;
  console.log(`  PASS  ${label}`);
}

try {
  const dist = path.join(root, "dist");
  const assets = path.join(dist, "assets");
  const vite = path.join(dist, ".vite");
  await mkdir(assets, { recursive: true });
  await mkdir(vite, { recursive: true });
  await writeFile(path.join(dist, "index.html"), `
    <script type="module" src="./assets/entry.js"></script>
    <link rel="modulepreload" href="./assets/vendor.js">
    <link rel="stylesheet" href="./assets/app.css">
  `);
  await writeFile(path.join(assets, "entry.js"), "e".repeat(100));
  await writeFile(path.join(assets, "vendor.js"), "v".repeat(40));
  await writeFile(path.join(assets, "async.js"), "a".repeat(120));
  await writeFile(path.join(assets, "locale-zh-fixture.js"), "z".repeat(50));
  await writeFile(path.join(assets, "locale-zh-tw-fixture.js"), "t".repeat(60));
  await writeFile(path.join(assets, "locale-shared.js"), "s".repeat(10));
  await writeFile(path.join(assets, "bridge-mock.js"), "b".repeat(20));
  await writeFile(path.join(assets, "provider.js"), "p".repeat(5));
  await writeFile(path.join(assets, "virtual-menu.js"), "m".repeat(15));
  await writeFile(path.join(assets, "tanstack.js"), "k".repeat(25));
  await writeFile(path.join(assets, "settings.js"), "g".repeat(30));
  await writeFile(path.join(assets, "app.css"), "c".repeat(30));
  await writeFile(path.join(assets, "settings.css"), "d".repeat(20));
  await writeFile(path.join(vite, "manifest.json"), JSON.stringify({
    "index.html": { file: "assets/entry.js", isEntry: true, imports: ["_vendor"], css: ["assets/app.css"] },
    "_vendor": { file: "assets/vendor.js" },
    "_provider": { file: "assets/provider.js" },
    "_tanstack": { file: "assets/tanstack.js" },
    "_locale-shared": { file: "assets/locale-shared.js" },
    "src/locales/zh.ts": {
      file: "assets/locale-zh-fixture.js",
      isDynamicEntry: true,
      imports: ["_locale-shared"],
    },
    "src/locales/zh-TW.ts": {
      file: "assets/locale-zh-tw-fixture.js",
      isDynamicEntry: true,
      imports: ["_locale-shared"],
    },
    "src/lib/bridgeMock.ts": {
      file: "assets/bridge-mock.js",
      isDynamicEntry: true,
      imports: ["index.html", "_provider"],
    },
    "src/components/VirtualMenuImpl.tsx": {
      file: "assets/virtual-menu.js",
      isDynamicEntry: true,
      imports: ["_vendor", "_tanstack"],
    },
    "src/components/SettingsPanelRoute.tsx": {
      file: "assets/settings.js",
      isDynamicEntry: true,
      imports: ["index.html", "_provider"],
      css: ["assets/settings.css"],
    },
  }));

  const metrics = await inspectBundle(dist);
  await check("inspector measures entry, initial, and largest assets", async () => {
    assert.equal(metrics.entryJS.bytes, 100);
    assert.equal(metrics.initialJSBytes, 140);
    assert.equal(metrics.localizedInitialJSBytes, 210);
    assert.equal(metrics.initialCSSBytes, 30);
    assert.equal(metrics.bridgeMockStartup.jsBytes, 165);
    assert.equal(metrics.virtualMenuStartup.jsBytes, 180);
    assert.equal(metrics.settingsStartup.jsBytes, 175);
    assert.equal(metrics.settingsStartup.cssBytes, 50);
    assert.deepEqual(metrics.largestJS, { file: "assets/async.js", bytes: 120 });
    assert.deepEqual(metrics.largestLocaleJS, { file: "assets/locale-zh-tw-fixture.js", bytes: 60 });
    assert.deepEqual(metrics.largestLocaleStartup, {
      file: "assets/locale-zh-tw-fixture.js",
      bytes: 210,
      files: ["assets/locale-zh-tw-fixture.js", "assets/locale-shared.js"],
    });
  });
  await check("budget accepts measurements within every limit", async () => {
    assert.deepEqual(evaluateBundleBudget(metrics, {
      maxEntryJSBytes: 100,
      maxInitialJSBytes: 140,
      maxLocalizedInitialJSBytes: 210,
      maxInitialCSSBytes: 30,
      maxBridgeMockStartupJSBytes: 165,
      maxVirtualMenuStartupJSBytes: 180,
      maxSettingsStartupJSBytes: 175,
      maxSettingsStartupCSSBytes: 50,
      maxSingleJSAssetBytes: 120,
      maxInitialJSFiles: 2,
      expectedLocaleJSFiles: 2,
    }), []);
  });
  await check("budget reports each exceeded metric", async () => {
    const failures = evaluateBundleBudget(metrics, {
      maxEntryJSBytes: 99,
      maxInitialJSBytes: 139,
      maxLocalizedInitialJSBytes: 209,
      maxInitialCSSBytes: 29,
      maxBridgeMockStartupJSBytes: 164,
      maxVirtualMenuStartupJSBytes: 179,
      maxSettingsStartupJSBytes: 174,
      maxSettingsStartupCSSBytes: 49,
      maxSingleJSAssetBytes: 119,
      maxInitialJSFiles: 1,
      expectedLocaleJSFiles: 2,
    });
    assert.deepEqual(failures.map((failure) => failure.label), [
      "entry JS", "initial JS", "localized initial JS", "initial CSS", "browser mock startup JS",
      "virtual menu startup JS", "settings startup JS", "settings startup CSS", "largest JS asset", "initial JS files",
    ]);
  });
  await check("budget requires both lazy locale chunks and a finite expected count", async () => {
    const failures = evaluateBundleBudget(metrics, {
      maxEntryJSBytes: 100,
      maxInitialJSBytes: 140,
      maxLocalizedInitialJSBytes: 210,
      maxInitialCSSBytes: 30,
      maxBridgeMockStartupJSBytes: 165,
      maxVirtualMenuStartupJSBytes: 180,
      maxSettingsStartupJSBytes: 175,
      maxSettingsStartupCSSBytes: 50,
      maxSingleJSAssetBytes: 120,
      maxInitialJSFiles: 2,
      expectedLocaleJSFiles: 1,
    });
    assert.deepEqual(failures, [{ label: "locale JS files", actual: 2, expected: 1 }]);
    const invalidBudget = evaluateBundleBudget(metrics, {
      maxEntryJSBytes: 100,
      maxInitialJSBytes: 140,
      maxLocalizedInitialJSBytes: 210,
      maxInitialCSSBytes: 30,
      maxBridgeMockStartupJSBytes: 165,
      maxVirtualMenuStartupJSBytes: 180,
      maxSettingsStartupJSBytes: 175,
      maxSettingsStartupCSSBytes: 50,
      maxSingleJSAssetBytes: 120,
      maxInitialJSFiles: 2,
    });
    assert.deepEqual(invalidBudget, [{
      label: "locale JS file count budget",
      actual: undefined,
      invalidBudget: true,
    }]);
  });
  await check("inspector rejects a deferred route hoisted into the initial contract", async () => {
    const manifestPath = path.join(vite, "manifest.json");
    const manifest = JSON.parse(await readFile(manifestPath, "utf8"));
    manifest["src/components/SettingsPanelRoute.tsx"].isDynamicEntry = false;
    await writeFile(manifestPath, JSON.stringify(manifest));
    await assert.rejects(() => inspectBundle(dist), /must stay a dynamic entry/);
    manifest["src/components/SettingsPanelRoute.tsx"].isDynamicEntry = true;
    await writeFile(manifestPath, JSON.stringify(manifest));
  });
  await check("inspector rejects paths outside dist", async () => {
    await writeFile(path.join(dist, "index.html"), '<script type="module" src="../outside.js"></script>');
    await assert.rejects(() => inspectBundle(dist), /escapes dist/);
  });
} finally {
  await rm(root, { recursive: true, force: true });
}

console.log(`bundle budget contract: ${passed} passed`);
