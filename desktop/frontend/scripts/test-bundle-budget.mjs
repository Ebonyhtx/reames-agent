import assert from "node:assert/strict";
import { mkdtemp, mkdir, rm, writeFile } from "node:fs/promises";
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
  await mkdir(assets, { recursive: true });
  await writeFile(path.join(dist, "index.html"), `
    <script type="module" src="./assets/entry.js"></script>
    <link rel="modulepreload" href="./assets/vendor.js">
    <link rel="stylesheet" href="./assets/app.css">
  `);
  await writeFile(path.join(assets, "entry.js"), "e".repeat(100));
  await writeFile(path.join(assets, "vendor.js"), "v".repeat(40));
  await writeFile(path.join(assets, "async.js"), "a".repeat(120));
  await writeFile(path.join(assets, "app.css"), "c".repeat(30));

  const metrics = await inspectBundle(dist);
  await check("inspector measures entry, initial, and largest assets", async () => {
    assert.equal(metrics.entryJS.bytes, 100);
    assert.equal(metrics.initialJSBytes, 140);
    assert.equal(metrics.initialCSSBytes, 30);
    assert.deepEqual(metrics.largestJS, { file: "assets/async.js", bytes: 120 });
  });
  await check("budget accepts measurements within every limit", async () => {
    assert.deepEqual(evaluateBundleBudget(metrics, {
      maxEntryJSBytes: 100,
      maxInitialJSBytes: 140,
      maxInitialCSSBytes: 30,
      maxSingleJSAssetBytes: 120,
      maxInitialJSFiles: 2,
    }), []);
  });
  await check("budget reports each exceeded metric", async () => {
    const failures = evaluateBundleBudget(metrics, {
      maxEntryJSBytes: 99,
      maxInitialJSBytes: 139,
      maxInitialCSSBytes: 29,
      maxSingleJSAssetBytes: 119,
      maxInitialJSFiles: 1,
    });
    assert.deepEqual(failures.map((failure) => failure.label), [
      "entry JS", "initial JS", "initial CSS", "largest JS asset", "initial JS files",
    ]);
  });
  await check("inspector rejects paths outside dist", async () => {
    await writeFile(path.join(dist, "index.html"), '<script type="module" src="../outside.js"></script>');
    await assert.rejects(() => inspectBundle(dist), /escapes dist/);
  });
} finally {
  await rm(root, { recursive: true, force: true });
}

console.log(`bundle budget contract: ${passed} passed`);
