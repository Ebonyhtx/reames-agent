import { readFile, readdir, stat } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

function attribute(source, name) {
  const match = source.match(new RegExp(`\\b${name}\\s*=\\s*["']([^"']+)["']`, "i"));
  return match?.[1] ?? "";
}

function bundleReferences(html) {
  const references = [];
  for (const match of html.matchAll(/<(script|link)\b([^>]*)>/gi)) {
    const tag = match[1].toLowerCase();
    const attrs = match[2];
    const rel = attribute(attrs, "rel").toLowerCase();
    const source = tag === "script" ? attribute(attrs, "src") : attribute(attrs, "href");
    if (!source) continue;
    if (tag === "script" && attribute(attrs, "type").toLowerCase() === "module") {
      references.push({ kind: "entry-js", source });
    } else if (tag === "link" && rel === "modulepreload") {
      references.push({ kind: "preload-js", source });
    } else if (tag === "link" && rel === "stylesheet") {
      references.push({ kind: "initial-css", source });
    }
  }
  return references;
}

async function measuredAsset(distDir, reference) {
  if (/^(?:[a-z]+:)?\/\//i.test(reference.source)) {
    throw new Error(`bundle reference must be local: ${reference.source}`);
  }
  const clean = decodeURIComponent(reference.source.split(/[?#]/, 1)[0]).replace(/^\.\//, "");
  const root = path.resolve(distDir);
  const fullPath = path.resolve(root, clean);
  if (fullPath !== root && !fullPath.startsWith(`${root}${path.sep}`)) {
    throw new Error(`bundle reference escapes dist: ${reference.source}`);
  }
  const info = await stat(fullPath);
  if (!info.isFile()) throw new Error(`bundle reference is not a file: ${reference.source}`);
  return { ...reference, file: path.relative(root, fullPath).replaceAll(path.sep, "/"), bytes: info.size };
}

async function allJSAssets(dir, root = dir) {
  const entries = await readdir(dir, { withFileTypes: true });
  const files = [];
  for (const entry of entries) {
    const fullPath = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      files.push(...await allJSAssets(fullPath, root));
    } else if (entry.isFile() && entry.name.endsWith(".js")) {
      const info = await stat(fullPath);
      files.push({ file: path.relative(root, fullPath).replaceAll(path.sep, "/"), bytes: info.size });
    }
  }
  return files;
}

async function allCSSAssets(dir, root = dir) {
  const entries = await readdir(dir, { withFileTypes: true });
  const files = [];
  for (const entry of entries) {
    const fullPath = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      files.push(...await allCSSAssets(fullPath, root));
    } else if (entry.isFile() && entry.name.endsWith(".css")) {
      const info = await stat(fullPath);
      files.push({ file: path.relative(root, fullPath).replaceAll(path.sep, "/"), bytes: info.size });
    }
  }
  return files;
}

async function bundleManifest(distDir) {
  const source = await readFile(path.join(distDir, ".vite", "manifest.json"), "utf8");
  const manifest = JSON.parse(source);
  if (!manifest || typeof manifest !== "object" || Array.isArray(manifest)) {
    throw new Error("bundle manifest must be an object");
  }
  return manifest;
}

function staticDependencyAssets(manifest, entryKeyOrFile) {
  const root = manifest[entryKeyOrFile]
    ? entryKeyOrFile
    : Object.entries(manifest).find(([, entry]) => entry?.file === entryKeyOrFile)?.[0];
  if (!root) throw new Error(`bundle manifest has no entry for ${entryKeyOrFile}`);
  const visited = new Set();
  const js = new Set();
  const css = new Set();
  const visit = (key) => {
    if (visited.has(key)) return;
    visited.add(key);
    const entry = manifest[key];
    if (!entry || typeof entry !== "object") throw new Error(`bundle manifest import is missing: ${key}`);
    if (typeof entry.file === "string" && entry.file.endsWith(".js")) js.add(entry.file);
    for (const file of Array.isArray(entry.css) ? entry.css : []) {
      if (typeof file !== "string" || !file.endsWith(".css")) throw new Error(`bundle manifest CSS is invalid: ${key}`);
      css.add(file);
    }
    for (const imported of Array.isArray(entry.imports) ? entry.imports : []) {
      if (typeof imported !== "string") throw new Error(`bundle manifest import is invalid: ${key}`);
      visit(imported);
    }
  };
  visit(root);
  return { root, js, css };
}

function routeStartup(manifest, entryKey, initialJSBytes, initialCSSBytes, initialJSFiles, initialCSSFiles, jsByFile, cssByFile) {
  const graph = staticDependencyAssets(manifest, entryKey);
  const entry = manifest[graph.root];
  if (entry.isDynamicEntry !== true) throw new Error(`bundle route must stay a dynamic entry: ${entryKey}`);
  if (initialJSFiles.has(entry.file)) throw new Error(`bundle route JS leaked into the initial graph: ${entryKey}`);
  for (const file of Array.isArray(entry.css) ? entry.css : []) {
    if (initialCSSFiles.has(file)) throw new Error(`bundle route CSS leaked into the initial graph: ${entryKey}`);
  }
  const additionalJSBytes = [...graph.js].reduce((sum, file) => {
    if (initialJSFiles.has(file)) return sum;
    const asset = jsByFile.get(file);
    if (!asset) throw new Error(`bundle manifest JS is missing from dist: ${file}`);
    return sum + asset.bytes;
  }, 0);
  const additionalCSSBytes = [...graph.css].reduce((sum, file) => {
    if (initialCSSFiles.has(file)) return sum;
    const asset = cssByFile.get(file);
    if (!asset) throw new Error(`bundle manifest CSS is missing from dist: ${file}`);
    return sum + asset.bytes;
  }, 0);
  return {
    entryKey,
    file: entry.file,
    jsBytes: initialJSBytes + additionalJSBytes,
    cssBytes: initialCSSBytes + additionalCSSBytes,
    jsFiles: [...graph.js],
    cssFiles: [...graph.css],
  };
}

export async function inspectBundle(distDir) {
  const html = await readFile(path.join(distDir, "index.html"), "utf8");
  const references = bundleReferences(html);
  const assets = await Promise.all(references.map((reference) => measuredAsset(distDir, reference)));
  const entries = assets.filter((asset) => asset.kind === "entry-js");
  if (entries.length !== 1) throw new Error(`expected exactly one module entry, found ${entries.length}`);

  const initialJS = assets.filter((asset) => asset.kind === "entry-js" || asset.kind === "preload-js");
  const initialCSS = assets.filter((asset) => asset.kind === "initial-css");
  const jsAssets = await allJSAssets(distDir);
  const cssAssets = await allCSSAssets(distDir);
  const manifest = await bundleManifest(distDir);
  const jsByFile = new Map(jsAssets.map((asset) => [asset.file, asset]));
  const cssByFile = new Map(cssAssets.map((asset) => [asset.file, asset]));
  const largestJS = jsAssets.reduce((largest, asset) => asset.bytes > largest.bytes ? asset : largest, { file: "", bytes: 0 });
  const localeJS = jsAssets.filter((asset) => /^assets\/locale-(?:zh|zh-tw)-.+\.js$/i.test(asset.file));
  const largestLocaleJS = localeJS.reduce(
    (largest, asset) => asset.bytes > largest.bytes ? asset : largest,
    { file: "", bytes: 0 },
  );
  const initialJSBytes = initialJS.reduce((sum, asset) => sum + asset.bytes, 0);
  const initialCSSBytes = initialCSS.reduce((sum, asset) => sum + asset.bytes, 0);
  const initialFiles = new Set(initialJS.map((asset) => asset.file));
  const initialCSSFiles = new Set(initialCSS.map((asset) => asset.file));
  const localeStartupJS = localeJS.map((locale) => {
    const files = [...staticDependencyAssets(manifest, locale.file).js];
    const additionalBytes = files.reduce((sum, file) => {
      if (initialFiles.has(file)) return sum;
      const asset = jsByFile.get(file);
      if (!asset) throw new Error(`bundle manifest JS is missing from dist: ${file}`);
      return sum + asset.bytes;
    }, 0);
    return { file: locale.file, bytes: initialJSBytes + additionalBytes, files };
  });
  const largestLocaleStartup = localeStartupJS.reduce(
    (largest, asset) => asset.bytes > largest.bytes ? asset : largest,
    { file: "", bytes: initialJSBytes, files: [] },
  );
  const bridgeMockStartup = routeStartup(manifest, "src/lib/bridgeMock.ts", initialJSBytes, initialCSSBytes, initialFiles, initialCSSFiles, jsByFile, cssByFile);
  const virtualMenuStartup = routeStartup(manifest, "src/components/VirtualMenuImpl.tsx", initialJSBytes, initialCSSBytes, initialFiles, initialCSSFiles, jsByFile, cssByFile);
  const settingsStartup = routeStartup(manifest, "src/components/SettingsPanelRoute.tsx", initialJSBytes, initialCSSBytes, initialFiles, initialCSSFiles, jsByFile, cssByFile);

  return {
    entryJS: entries[0],
    initialJS,
    initialJSBytes,
    localeJS,
    largestLocaleJS,
    localeStartupJS,
    largestLocaleStartup,
    localizedInitialJSBytes: largestLocaleStartup.bytes,
    bridgeMockStartup,
    virtualMenuStartup,
    settingsStartup,
    initialCSS,
    initialCSSBytes,
    largestJS,
  };
}

export function evaluateBundleBudget(metrics, budget) {
  const checks = [
    ["entry JS", metrics.entryJS.bytes, budget.maxEntryJSBytes],
    ["initial JS", metrics.initialJSBytes, budget.maxInitialJSBytes],
    ["localized initial JS", metrics.localizedInitialJSBytes, budget.maxLocalizedInitialJSBytes],
    ["initial CSS", metrics.initialCSSBytes, budget.maxInitialCSSBytes],
    ["browser mock startup JS", metrics.bridgeMockStartup.jsBytes, budget.maxBridgeMockStartupJSBytes],
    ["virtual menu startup JS", metrics.virtualMenuStartup.jsBytes, budget.maxVirtualMenuStartupJSBytes],
    ["settings startup JS", metrics.settingsStartup.jsBytes, budget.maxSettingsStartupJSBytes],
    ["settings startup CSS", metrics.settingsStartup.cssBytes, budget.maxSettingsStartupCSSBytes],
    ["largest JS asset", metrics.largestJS.bytes, budget.maxSingleJSAssetBytes],
    ["initial JS files", metrics.initialJS.length, budget.maxInitialJSFiles],
  ];
  const failures = checks
    .filter(([, actual, maximum]) => !Number.isFinite(maximum) || actual > maximum)
    .map(([label, actual, maximum]) => ({ label, actual, maximum }));
  const expectedLocaleJSFiles = budget.expectedLocaleJSFiles;
  if (!Number.isInteger(expectedLocaleJSFiles) || expectedLocaleJSFiles < 0) {
    failures.push({
      label: "locale JS file count budget",
      actual: expectedLocaleJSFiles,
      invalidBudget: true,
    });
  } else if (metrics.localeJS.length !== expectedLocaleJSFiles) {
    failures.push({
      label: "locale JS files",
      actual: metrics.localeJS.length,
      expected: expectedLocaleJSFiles,
    });
  }
  return failures;
}

function formatBytes(bytes) {
  return `${bytes.toLocaleString("en-US")} B`;
}

async function main() {
  const scriptDir = path.dirname(fileURLToPath(import.meta.url));
  const frontendRoot = path.resolve(scriptDir, "..");
  const distDir = path.resolve(frontendRoot, process.argv[2] || "dist");
  const budgetPath = path.resolve(frontendRoot, process.argv[3] || "bundle-budget.json");
  const budget = JSON.parse(await readFile(budgetPath, "utf8"));
  const metrics = await inspectBundle(distDir);
  const failures = evaluateBundleBudget(metrics, budget);

  console.log(`Bundle entry JS: ${formatBytes(metrics.entryJS.bytes)} (${metrics.entryJS.file})`);
  console.log(`Bundle initial JS: ${formatBytes(metrics.initialJSBytes)} across ${metrics.initialJS.length} files`);
  console.log(`Bundle localized initial JS: ${formatBytes(metrics.localizedInitialJSBytes)} (${metrics.largestLocaleStartup.files.length} locale-graph files; direct chunk ${formatBytes(metrics.largestLocaleJS.bytes)})`);
  console.log(`Bundle initial CSS: ${formatBytes(metrics.initialCSSBytes)} across ${metrics.initialCSS.length} files`);
  console.log(`Bundle browser mock startup JS: ${formatBytes(metrics.bridgeMockStartup.jsBytes)}`);
  console.log(`Bundle virtual menu startup JS: ${formatBytes(metrics.virtualMenuStartup.jsBytes)}`);
  console.log(`Bundle settings startup: ${formatBytes(metrics.settingsStartup.jsBytes)} JS + ${formatBytes(metrics.settingsStartup.cssBytes)} CSS`);
  console.log(`Bundle largest JS: ${formatBytes(metrics.largestJS.bytes)} (${metrics.largestJS.file})`);
  if (failures.length === 0) {
    console.log("Bundle budget check passed.");
    return;
  }
  for (const failure of failures) {
    if (failure.invalidBudget) {
      console.error(`Bundle budget invalid: ${failure.label} must be a non-negative integer`);
    } else if (Number.isFinite(failure.expected)) {
      console.error(`Bundle contract failed: ${failure.label} ${failure.actual} != ${failure.expected}`);
    } else {
      console.error(`Bundle budget exceeded: ${failure.label} ${failure.actual} > ${failure.maximum}`);
    }
  }
  process.exitCode = 1;
}

if (process.argv[1] && pathToFileURL(path.resolve(process.argv[1])).href === import.meta.url) {
  await main();
}
