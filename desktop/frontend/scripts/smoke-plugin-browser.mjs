import { createHash } from "node:crypto";
import { access, mkdir, readFile, stat, writeFile } from "node:fs/promises";
import { createServer } from "node:http";
import { tmpdir } from "node:os";
import { dirname, extname, isAbsolute, join, relative, resolve, sep } from "node:path";
import { pathToFileURL } from "node:url";
import { chromium } from "playwright-core";

const PLUGIN_NAME = "reames-browser-smoke";

const MIME_TYPES = new Map([
  [".css", "text/css; charset=utf-8"],
  [".html", "text/html; charset=utf-8"],
  [".ico", "image/x-icon"],
  [".js", "text/javascript; charset=utf-8"],
  [".json", "application/json; charset=utf-8"],
  [".map", "application/json; charset=utf-8"],
  [".png", "image/png"],
  [".svg", "image/svg+xml"],
  [".woff2", "font/woff2"],
]);

const DEFAULT_BROWSER_PATHS = process.platform === "win32"
  ? [
      "C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe",
      "C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe",
      "C:\\Program Files\\Microsoft\\Edge\\Application\\msedge.exe",
    ]
  : process.platform === "darwin"
    ? [
        "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
        "/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
      ]
    : [
        "/usr/bin/google-chrome",
        "/usr/bin/google-chrome-stable",
        "/usr/bin/chromium",
        "/usr/bin/chromium-browser",
      ];

export function parseArgs(argv) {
  const result = {
    browser: process.env.REAMES_BROWSER_PATH || process.env.CHROME_BIN || "",
    dist: resolve(process.cwd(), "dist"),
    out: process.env.REAMES_PLUGIN_BROWSER_SMOKE_OUT || "",
    screenshot: process.env.REAMES_PLUGIN_BROWSER_SMOKE_SCREENSHOT || "",
  };
  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index];
    if (!["--browser", "--dist", "--out", "--screenshot"].includes(arg)) {
      throw new Error(`unknown argument: ${arg}`);
    }
    const value = argv[index + 1];
    if (!value) throw new Error(`${arg} requires a value`);
    result[arg.slice(2)] = value;
    index += 1;
  }
  result.dist = resolve(result.dist);
  if (result.out) result.out = resolve(result.out);
  if (result.screenshot) result.screenshot = resolve(result.screenshot);
  return result;
}

export async function resolveBrowserExecutable(explicit = "", candidates = DEFAULT_BROWSER_PATHS) {
  const paths = explicit ? [explicit] : candidates;
  for (const candidate of paths) {
    try {
      await access(candidate);
      return resolve(candidate);
    } catch {
      // Continue through the platform candidates.
    }
  }
  throw new Error("Chrome or Edge executable not found; set REAMES_BROWSER_PATH");
}

export function safeStaticPath(dist, pathname) {
  const decoded = decodeURIComponent(pathname === "/" ? "/index.html" : pathname);
  const candidate = resolve(dist, `.${decoded}`);
  const rel = relative(dist, candidate);
  if (rel.startsWith(`..${sep}`) || rel === ".." || isAbsolute(rel)) {
    throw new Error("static path escapes dist root");
  }
  return candidate;
}

async function startStaticServer(dist) {
  await access(join(dist, "index.html"));
  const server = createServer(async (request, response) => {
    try {
      const url = new URL(request.url || "/", "http://127.0.0.1");
      if (url.pathname === "/favicon.ico") {
        response.writeHead(204);
        response.end();
        return;
      }
      const path = safeStaticPath(dist, url.pathname);
      const body = await readFile(path);
      response.writeHead(200, {
        "Cache-Control": "no-store",
        "Content-Type": MIME_TYPES.get(extname(path)) || "application/octet-stream",
      });
      response.end(body);
    } catch (error) {
      response.writeHead(error?.code === "ENOENT" ? 404 : 400, { "Content-Type": "text/plain; charset=utf-8" });
      response.end("not found\n");
    }
  });
  await new Promise((resolveListen, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", resolveListen);
  });
  const address = server.address();
  if (!address || typeof address === "string") throw new Error("static server did not bind TCP");
  return { server, url: `http://127.0.0.1:${address.port}/?platform=windows` };
}

async function closeServer(server) {
  await new Promise((resolveClose) => server.close(resolveClose));
}

async function waitForEnabled(locator) {
  await locator.waitFor({ state: "visible" });
  await locator.evaluate((element) => new Promise((resolveWait, reject) => {
    const deadline = Date.now() + 10_000;
    const check = () => {
      if (!element.disabled) return resolveWait();
      if (Date.now() >= deadline) return reject(new Error(`control remained disabled: ${element.id}`));
      setTimeout(check, 25);
    };
    check();
  }));
}

async function sha256(path) {
  return createHash("sha256").update(await readFile(path)).digest("hex");
}

async function runFlow(page, evidence) {
  await page.goto(evidence.url, { waitUntil: "networkidle" });
  await page.locator("#settings-open").first().click();
  await page.locator("#settings-tab-plugins").click();
  await page.locator("#plugin-settings-page").waitFor({ state: "visible" });
  evidence.settings_opened = true;

  await page.locator("#plugin-install-local-source").fill(`/tmp/${PLUGIN_NAME}`);
  const preview = page.locator("#plugin-install-preview");
  await waitForEnabled(preview);
  await preview.click();
  await page.locator("#plugin-install-plan").waitFor({ state: "visible" });
  const install = page.locator("#plugin-install-apply");
  await waitForEnabled(install);
  evidence.install_planned = true;
  await install.click();

  const row = page.locator(`#plugin-row-${PLUGIN_NAME}`);
  await row.waitFor({ state: "visible" });
  evidence.installed = true;
  await page.locator(`#plugin-${PLUGIN_NAME}-details`).click();

  const enabled = page.locator(`#plugin-${PLUGIN_NAME}-enabled`);
  await enabled.evaluate((element) => element.click());
  await page.locator(`#plugin-${PLUGIN_NAME}-enable-review`).waitFor({ state: "visible" });
  evidence.enable_reviewed = true;
  await page.locator(`#plugin-${PLUGIN_NAME}-enable-approve`).click();
  await page.waitForFunction((id) => document.getElementById(id)?.checked === true, `plugin-${PLUGIN_NAME}-enabled`);
  evidence.enabled = true;

  const update = page.locator(`#plugin-${PLUGIN_NAME}-update`);
  await update.click();
  await page.locator(`#plugin-${PLUGIN_NAME}-update-plan`).waitFor({ state: "visible" });
  evidence.update_planned = true;
  await update.click();
  await page.locator(`#plugin-${PLUGIN_NAME}-rollback`).waitFor({ state: "visible" });
  evidence.updated = true;

  const rollback = page.locator(`#plugin-${PLUGIN_NAME}-rollback`);
  await rollback.click();
  await page.locator(`#plugin-${PLUGIN_NAME}-rollback-plan`).waitFor({ state: "visible" });
  evidence.rollback_planned = true;
  await rollback.click();
  evidence.rolled_back = true;

  await page.locator(`#plugin-${PLUGIN_NAME}-doctor`).click();
  evidence.doctor_invoked = true;

  await page.locator(`#plugin-${PLUGIN_NAME}-remove`).click();
  await page.locator(`#plugin-${PLUGIN_NAME}-remove-confirm`).click();
  await page.locator(`#plugin-${PLUGIN_NAME}-remove-plan`).waitFor({ state: "visible" });
  evidence.remove_planned = true;
  await page.locator(`#plugin-${PLUGIN_NAME}-remove-apply`).click();
  await row.waitFor({ state: "detached" });
  evidence.removed = true;

  if (await page.locator("#plugin-operation-error").count()) {
    throw new Error(`plugin browser flow ended with an error: ${await page.locator("#plugin-operation-error").innerText()}`);
  }
  evidence.layout = await page.locator("#settings-dialog").evaluate((dialog) => {
    const rect = dialog.getBoundingClientRect();
    return {
      width: Math.round(rect.width),
      height: Math.round(rect.height),
      horizontal_overflow: dialog.scrollWidth > dialog.clientWidth + 1,
    };
  });
  if (evidence.layout.horizontal_overflow) throw new Error("plugin settings dialog has horizontal overflow");
}

export async function main(argv = process.argv.slice(2)) {
  const options = parseArgs(argv);
  const evidence = {
    schema_version: 1,
    backend: "browser-mock",
    outcome: "failed",
    started_at: new Date().toISOString(),
    browser_executable: "",
    browser_version: "",
    url: "",
    settings_opened: false,
    install_planned: false,
    installed: false,
    enable_reviewed: false,
    enabled: false,
    update_planned: false,
    updated: false,
    rollback_planned: false,
    rolled_back: false,
    doctor_invoked: false,
    remove_planned: false,
    removed: false,
    layout: null,
    screenshot: "",
    screenshot_sha256: "",
    screenshot_size: 0,
    console_errors: [],
    page_errors: [],
    errors: [],
    finished_at: "",
  };

  let browser;
  let server;
  try {
    evidence.browser_executable = await resolveBrowserExecutable(options.browser);
    const staticSite = await startStaticServer(options.dist);
    server = staticSite.server;
    evidence.url = staticSite.url;
    browser = await chromium.launch({ executablePath: evidence.browser_executable, headless: true });
    evidence.browser_version = browser.version();
    const context = await browser.newContext({ viewport: { width: 1440, height: 960 }, locale: "en-US" });
    const page = await context.newPage();
    page.on("console", (message) => {
      if (message.type() === "error") evidence.console_errors.push(message.text());
    });
    page.on("pageerror", (error) => evidence.page_errors.push(error.message));
    await runFlow(page, evidence);

    const screenshot = options.screenshot || join(tmpdir(), "reames-agent-plugin-browser-smoke.png");
    await mkdir(dirname(screenshot), { recursive: true });
    await page.screenshot({ path: screenshot, fullPage: true });
    evidence.screenshot = screenshot;
    evidence.screenshot_sha256 = await sha256(screenshot);
    evidence.screenshot_size = (await stat(screenshot)).size;
    if (evidence.console_errors.length) throw new Error(`browser console errors: ${evidence.console_errors.join("; ")}`);
    if (evidence.page_errors.length) throw new Error(`browser page errors: ${evidence.page_errors.join("; ")}`);
    evidence.outcome = "passed";
  } catch (error) {
    evidence.errors.push(error instanceof Error ? error.message : String(error));
  } finally {
    if (browser) await browser.close();
    if (server) await closeServer(server);
    evidence.finished_at = new Date().toISOString();
    const body = `${JSON.stringify(evidence, null, 2)}\n`;
    if (options.out) {
      await mkdir(dirname(options.out), { recursive: true });
      await writeFile(options.out, body, "utf8");
      console.log(`Evidence written to ${options.out}`);
    }
    console.log(body.trimEnd());
  }
  return evidence.outcome === "passed" ? 0 : 1;
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  process.exitCode = await main();
}
