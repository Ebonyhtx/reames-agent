import assert from "node:assert/strict";
import { mkdtemp, mkdir, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { parseArgs, resolveBrowserExecutable, safeStaticPath } from "./smoke-plugin-browser.mjs";

const root = await mkdtemp(join(tmpdir(), "reames-plugin-browser-contract-"));
try {
  const dist = join(root, "dist");
  await mkdir(dist);
  const browser = join(root, process.platform === "win32" ? "browser.exe" : "browser");
  await writeFile(browser, "fixture", "utf8");

  const args = parseArgs(["--browser", browser, "--dist", dist, "--out", join(root, "out.json")]);
  assert.equal(args.browser, browser);
  assert.equal(args.dist, dist);
  assert.equal(await resolveBrowserExecutable(browser, []), browser);
  await assert.rejects(resolveBrowserExecutable(join(root, "missing-browser"), [browser]), /not found/);
  assert.equal(safeStaticPath(dist, "/"), join(dist, "index.html"));
  assert.equal(safeStaticPath(dist, "/assets/app.js"), join(dist, "assets", "app.js"));
  assert.throws(() => safeStaticPath(dist, "/../outside"), /escapes dist root/);
  assert.throws(() => parseArgs(["--unknown", "value"]), /unknown argument/);
} finally {
  await rm(root, { recursive: true, force: true });
}

console.log("plugin browser smoke contract: passed");
