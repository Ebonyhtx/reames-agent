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

export async function inspectBundle(distDir) {
  const html = await readFile(path.join(distDir, "index.html"), "utf8");
  const references = bundleReferences(html);
  const assets = await Promise.all(references.map((reference) => measuredAsset(distDir, reference)));
  const entries = assets.filter((asset) => asset.kind === "entry-js");
  if (entries.length !== 1) throw new Error(`expected exactly one module entry, found ${entries.length}`);

  const initialJS = assets.filter((asset) => asset.kind === "entry-js" || asset.kind === "preload-js");
  const initialCSS = assets.filter((asset) => asset.kind === "initial-css");
  const jsAssets = await allJSAssets(distDir);
  const largestJS = jsAssets.reduce((largest, asset) => asset.bytes > largest.bytes ? asset : largest, { file: "", bytes: 0 });

  return {
    entryJS: entries[0],
    initialJS,
    initialJSBytes: initialJS.reduce((sum, asset) => sum + asset.bytes, 0),
    initialCSS,
    initialCSSBytes: initialCSS.reduce((sum, asset) => sum + asset.bytes, 0),
    largestJS,
  };
}

export function evaluateBundleBudget(metrics, budget) {
  const checks = [
    ["entry JS", metrics.entryJS.bytes, budget.maxEntryJSBytes],
    ["initial JS", metrics.initialJSBytes, budget.maxInitialJSBytes],
    ["initial CSS", metrics.initialCSSBytes, budget.maxInitialCSSBytes],
    ["largest JS asset", metrics.largestJS.bytes, budget.maxSingleJSAssetBytes],
    ["initial JS files", metrics.initialJS.length, budget.maxInitialJSFiles],
  ];
  return checks
    .filter(([, actual, maximum]) => !Number.isFinite(maximum) || actual > maximum)
    .map(([label, actual, maximum]) => ({ label, actual, maximum }));
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
  console.log(`Bundle initial CSS: ${formatBytes(metrics.initialCSSBytes)} across ${metrics.initialCSS.length} files`);
  console.log(`Bundle largest JS: ${formatBytes(metrics.largestJS.bytes)} (${metrics.largestJS.file})`);
  if (failures.length === 0) {
    console.log("Bundle budget check passed.");
    return;
  }
  for (const failure of failures) {
    console.error(`Bundle budget exceeded: ${failure.label} ${failure.actual} > ${failure.maximum}`);
  }
  process.exitCode = 1;
}

if (process.argv[1] && pathToFileURL(path.resolve(process.argv[1])).href === import.meta.url) {
  await main();
}
