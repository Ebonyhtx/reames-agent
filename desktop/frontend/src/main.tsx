import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import App from "./App";
import { ErrorBoundary } from "./components/ErrorBoundary";
import { installGlobalCrashHandlers, installPerformancePressureMonitor } from "./lib/crash";
import { app, installWailsNonFileDragErrorSuppression } from "./lib/bridge";
import { installBreadcrumbConsoleHook } from "./lib/breadcrumbs";
import { installMessageSelectionCopy } from "./lib/messageSelectionCopy";
import {
  LocaleProvider,
  normalizeLangPref,
  preloadInitialLocale,
  readLegacyLangPref,
  type LangPref,
} from "./lib/i18n";
import { ToastProvider } from "./lib/toast";
import { initFontFamily } from "./lib/fontFamily";
import { initTextSize } from "./lib/textSize";
import { initTheme } from "./lib/theme";
import "./styles.css";
import "./components/RecoveryCenter.css";

// Install first so startup/runtime failures paint a useful error instead of a
// featureless webview background, with the recent console trail attached.
installWailsNonFileDragErrorSuppression();
installGlobalCrashHandlers();
installBreadcrumbConsoleHook();
installPerformancePressureMonitor();

// Apply the saved appearance (auto/light/dark) before the first paint.
function initTypographyPlatform() {
  if (typeof document === "undefined" || typeof navigator === "undefined") return;
  const params = new URLSearchParams(window.location.search);
  const override = params.get("platform");
  const marker = `${navigator.platform} ${navigator.userAgent}`;
  const platform =
    override === "darwin" || override === "windows" || override === "linux"
      ? override
      : /Win/i.test(marker)
        ? "windows"
        : /Mac/i.test(marker)
          ? "darwin"
          : "linux";
  document.documentElement.setAttribute("data-platform", platform);
}

initTypographyPlatform();
initTheme();
initTextSize();
initFontFamily();

// Pre-warm font fallback stacks so the first frame doesn't flicker between the
// browser default font and the app's configured typeface. Inserting a hidden span
// with CJK + emoji + math glyphs forces the OS font subsystem to resolve and
// cache the fallback chains before React mounts.
function prewarmFontFallbacks() {
  const span = document.createElement("span");
  span.style.cssText = "position:absolute;visibility:hidden;font-size:1px;pointer-events:none";
  span.textContent = "中文日本語한국어 математика 😀🎉✓⚠∑∏∫";
  document.body.appendChild(span);
  // Force layout so the browser resolves font fallback chains.
  void span.offsetHeight;
  requestAnimationFrame(() => {
    requestAnimationFrame(() => {
      span.remove();
    });
  });
}
prewarmFontFallbacks();

installMessageSelectionCopy(document);

// Inside the Wails shell, suppress the webview's default right-click menu — its
// Reload / Back / Inspect entries are easy to hit by accident and can reset or
// navigate away from the app. Text inputs keep their native Cut/Copy/Paste menu.
// Left alone in a plain browser (pnpm dev) so devtools stay reachable.
if (typeof window !== "undefined" && window.runtime) {
  window.addEventListener("contextmenu", (e) => {
    const target = e.target as HTMLElement | null;
    if (!target?.closest("input, textarea")) e.preventDefault();
  });
}

const root = document.getElementById("root");
if (!root) throw new Error("missing #root");
const rootElement = root;

async function initialDesktopLanguage(): Promise<LangPref> {
  try {
    const settings = await app.DesktopStartupSettings();
    const saved = normalizeLangPref(settings.desktopLanguage);
    // Existing user config is authoritative. Only consult the browser-era
    // preference when no desktop language has been persisted yet.
    return saved || readLegacyLangPref();
  } catch (error) {
    console.warn("startup desktop language read failed", error);
    return readLegacyLangPref();
  }
}

async function mountApp() {
  // Resolve only the saved (or OS-preferred auto) dictionary before the first
  // usable frame. The lightweight local bridge call avoids loading a second
  // locale after mount when the saved preference differs from the OS.
  // load failures resolve to the synchronous English fallback, so the Desktop
  // can still render offline even if a packaged locale chunk is damaged.
  const initialLocalePref = await initialDesktopLanguage();
  await preloadInitialLocale(initialLocalePref);
  createRoot(rootElement).render(
    <StrictMode>
      <ErrorBoundary>
        <LocaleProvider initialPref={initialLocalePref}>
          <ToastProvider>
            <App />
          </ToastProvider>
        </LocaleProvider>
      </ErrorBoundary>
    </StrictMode>,
  );
}

void mountApp();
