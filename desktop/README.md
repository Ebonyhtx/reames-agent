# Reames Agent Desktop (Wails shell)

A native desktop window around the Reames Agent Go kernel. The same
transport-agnostic `control.Controller` that backs the chat TUI and the HTTP/SSE
server is bound **directly** to a React webview — Go methods in, typed events
out, no HTTP hop.

```
┌─────────────────────────────────────────────────────────────┐
│  webview (React + TS, Vite)                                  │
│    bridge.ts ──calls──▶ window.go.main.App.{Submit,Cancel,…} │
│    bridge.ts ◀─events── window.runtime.EventsOn("agent:event")│
└───────────────▲───────────────────────────┬─────────────────┘
        bound methods                  runtime.EventsEmit
┌───────────────┴───────────────────────────▼─────────────────┐
│  desktop/app.go   App (bound)  +  eventSink (event.Sink)     │
│  desktop/main.go  Wails options, window, embed frontend/dist │
└───────────────▲───────────────────────────┬─────────────────┘
       commands │                            │ typed event stream
┌───────────────┴────────────────────────────▼────────────────┐
│  internal/boot.Build → internal/control.Controller (kernel)  │
│  (same assembly the CLI uses: providers, tools, gate, …)     │
└──────────────────────────────────────────────────────────────┘
```

## Why a nested module

`desktop/` is its own Go module (`module reames-agent/desktop`, `replace reames-agent =>
../`). That keeps the CGO + WebKit desktop build entirely separate from the CLI's
`CGO_ENABLED=0` single-static-binary guarantee: the parent module's `go build /
vet / test ./...` skip this directory, while the import path stays under
`reames-agent/` so it can still import the `reames-agent/internal/*` kernel.

## Prerequisites

- Go (matches the parent module).
- Node + **pnpm** (`npm i -g pnpm`).
- Wails CLI: `go install github.com/wailsapp/wails/v2/cmd/wails@latest`
- Platform webview libs: macOS ships WebKit; Windows needs the Edge **WebView2**
  runtime; Linux needs `libgtk-3-dev` plus WebKitGTK. The default build links
  against **WebKitGTK 4.0**; distros that only ship **4.1** (Fedora 40+, Ubuntu
  24.04+, Arch) build with `-tags webkit2_41` — see [Build](#build). Run
  `wails doctor` to verify.

## Develop

```sh
cd desktop
wails dev            # hot-reloads Go + frontend (Vite dev server)
```

Frontend-only iteration without the Go side:

```sh
cd desktop/frontend
pnpm install
pnpm dev             # opens in a plain browser; bridge.ts uses the dev mock
```

In a plain browser the native bindings are absent, so `bridge.ts` falls back to a
**mock** that streams a canned turn (text + one `edit_file` tool call) through the
exact same event contract — so layout, streaming, markdown, tool cards, and the
diff seam can all be built without rebuilding Go.

## Test

The desktop package is a nested Go module, so parent `go test ./...` does not run
it. Use the full lane before merging desktop changes, and the short lane for fast
local feedback:

```sh
make desktop-test        # cd desktop && go test .
make desktop-test-short  # skips slow desktop integration/e2e checks
```

To find the next bottleneck, rank individual test cases from the JSON stream:

```sh
make desktop-test-times
# or: cd desktop && go test -count=1 -json . | python3 ../scripts/desktop-test-times.py
```

### Frontend UI review checklist

For anchored menus, dropdowns, tooltips, and other portaled UI, review both the
component code and the CSS positioning contract:

- If a component uses `createPortal` plus `getBoundingClientRect()`, it must
  handle scrollable ancestors, window resize, and `visualViewport` changes.
- Add a focused regression test when changing shared positioning primitives such
  as `AnchoredPopover`, not only the specific menu that exposed the bug.
- Exercise at least one scrollable container path, such as Settings content, when
  manually checking dropdown or popover changes.

## Build

```sh
cd desktop
wails build          # → build/bin/Reames Agent(.app/.exe)
```

### Isolated native startup smoke

Desktop accepts `--home <path>` and `--home=<path>`. The override is applied
before config, migration, window-state, and single-instance paths are resolved;
different isolated homes therefore run independently. This is intended for CI,
portable setups, and smoke tests rather than normal interactive launches.

On Windows, build the current frontend and native executable, then run:

```powershell
python scripts/smoke_desktop_native.py `
  --exe desktop/build/bin/reames-agent-desktop.exe `
  --out artifacts/desktop-native-smoke.json
```

The smoke waits for the process's visible native window to answer bounded
message-pump probes, verifies that the default AppData roots did not change, and
seeds the isolated config with update checks disabled and close behavior set to
`quit` before requesting `WM_CLOSE`. A bounded terminate/kill fallback is still
available and recorded in the JSON evidence. The smoke proves startup, state
confinement, and shutdown only; it does not replace the M1 Wails click workflow.
Use `--keep-temp` only for local debugging.

**Linux on WebKitGTK 4.1 only** (Fedora 40+, Ubuntu 24.04+, Arch — no
`webkit2gtk-4.0` package): pass the Wails build tag so cgo links against 4.1.

```sh
wails build -tags webkit2_41
wails dev   -tags webkit2_41   # same tag for hot-reload
```

Fedora deps: `sudo dnf install webkit2gtk4.1-devel gtk3-devel`.

`frontend/dist` is generated by the build (it's git-ignored except for a
`.gitkeep` that keeps the Go `//go:embed all:frontend/dist` compilable on a fresh
checkout). A bare `go build` without a prior `pnpm build` produces a blank window.

## Releases & auto-update

Production Desktop publishing is currently disabled while the project establishes
its own signing identities and native candidate pipeline. Do not create a
`desktop-v*` tag expecting it to publish.

The intended pipeline builds on one native runner per platform (Wails cannot
cross-compile a CGO/WebKit binary), signs each artifact, generates `latest.json`,
and publishes only after a canary and explicit approval.
The Linux artifact links against WebKitGTK 4.1 (`-tags webkit2_41`), so it needs
`libwebkit2gtk-4.1-0` at runtime — present by default on Ubuntu 22.04+, Fedora 40+.

The app checks `latest.json` only from this repository's GitHub Releases and
shows an update banner when a newer version is published; **Settings → Software
update** has a manual check. Self-update behavior by platform:

- **Linux / Windows** — download, verify the minisign signature, then update in
  place: Linux replaces the binary and relaunches; Windows runs the per-user NSIS
  installer (no admin rights needed).
- **macOS** — *not* self-updating yet. The build is unsigned/un-notarized, so an
  in-place swap would be blocked by Gatekeeper; the banner links to the download
  page for a manual update instead.

### Unsigned builds — first launch

There are no Apple/Windows code-signing certificates yet, so a downloaded build
trips the OS gatekeepers on first run:

- **macOS** — open `Reames Agent-darwin-universal.dmg` and drag Reames Agent into
  Applications. Gatekeeper may then report the app "is damaged" or is from an
  unidentified developer; clear the quarantine attribute and open it:
  ```sh
  xattr -dr com.apple.quarantine /Applications/Reames Agent.app
  ```
- **Windows** — SmartScreen shows "Windows protected your PC". Click *More info →
  Run anyway*.

When Developer ID / Authenticode certificates are added, the release workflow's
`HAS_APPLE_CERT` gate flips to the signed path and these steps go away.

### Verifying a download

Artifacts are signed with minisign (public key ID `AF12CA46F4A9EBB0`). The `.minisig`
signature sits next to each artifact in the release; verify with the
[minisign](https://jedisct1.github.io/minisign/) CLI:

```sh
minisign -Vm Reames Agent-darwin-arm64.zip \
  -P RWSw66n0RsoSr6Zhh6qt5YO95YkpCayTOCMFVDNUQSjJYwxoYngNVBSq
```

## Editor seam (Monaco / CodeMirror)

Code and diff rendering go through two components with stable prop contracts and a
lazy boundary, so a heavy editor stays out of the initial bundle and dropping one
in is a one-line change — no consumer touches:

| Component | Props | Default impl | Upgrade |
|---|---|---|---|
| `components/CodeViewer.tsx` | `EditorProps` | `editors/PlainCode.tsx` (`<pre>`) | swap the lazy import for `editors/MonacoCode` or `editors/CodeMirrorCode` |
| `components/DiffView.tsx` | `DiffProps` | `editors/PlainDiff.tsx` (LCS line diff) | swap for `editors/MonacoDiff` or `editors/CodeMirrorMerge` |

```sh
# Monaco
pnpm add @monaco-editor/react monaco-editor
# or CodeMirror 6
pnpm add @uiw/react-codemirror @codemirror/lang-javascript @codemirror/merge
```

Then add `editors/MonacoCode.tsx` (default-export a component taking
`EditorProps`) and point `CodeViewer.tsx`'s `lazy(() => import(...))` at it.
`ToolCard` already routes `edit_file` calls' `old_string`/`new_string` through
`DiffView`, and `Markdown` routes fenced code blocks through `CodeViewer`, so
both seams light up everywhere at once.

Markdown itself is currently minimal (fenced code + plain text). Upgrade path:
`pnpm add react-markdown remark-gfm` and render in `components/Markdown.tsx`,
keeping fenced code delegated to `CodeViewer`.

## Multi-platform adaptation

Wails is the right shell for a Go kernel (no sidecar), but a Go+webview stack uses
the **native** webview per OS, so the rough edges are platform-specific. What's
handled here, and what to reach for if a target misbehaves:

- **Linux / WebKitGTK** is the one real pain point — rendering varies by distro &
  GPU driver. `main.go` keeps `WebviewGpuPolicy: OnDemand` when a DRI render node
  is usable, and falls back to `Never` for xrdp/headless/software-rendered sessions
  that cannot access `/dev/dri`. If artifacts persist, launch with
  `WEBKIT_DISABLE_COMPOSITING_MODE=1`. Test on at least one GTK target before release;
  the CSS deliberately avoids `backdrop-filter`/blur (slow & inconsistent there).
  - **Wayland + NVIDIA**: On KDE Plasma Wayland with NVIDIA GPUs, WebKitGTK can
    crash at startup (`Error 71: Protocol error`) due to an upstream WebKit
    explicit-sync bug (WebKit #280210, #317089, NVIDIA/egl-wayland #179).
    Reames Agent automatically sets `__NV_DISABLE_EXPLICIT_SYNC=1` when it detects
    Wayland + NVIDIA GPU. To opt out, set `__NV_DISABLE_EXPLICIT_SYNC=0`.
    Alternative fallbacks: `WEBKIT_DISABLE_DMABUF_RENDERER=1` (poor performance)
    or `GDK_BACKEND=x11` (forces XWayland).
- **Windows / WebView2** — `Theme: SystemDefault` follows the OS light/dark
  setting; the installer embeds the WebView2 bootstrapper. Canary builds disable
  WebView2 GPU acceleration by default to smoke-test blank-window reports; set
  `REAMES_AGENT_DESKTOP_DISABLE_WEBVIEW2_GPU=1` or `0` to force the fallback on
  or off.
- **macOS / WebKit** — inset/hidden title bar (`TitleBarHiddenInset`); the CSS
  marks the top bar as an OS drag region (`--wails-draggable: drag`) and leaves
  room for the traffic lights.
- **Theming** — colors are CSS variables gated on `prefers-color-scheme`, which all
  three webviews honor, so the UI follows the OS theme without native glue.
- **Fonts / offline** — system font stack only; no web-font fetches, so first paint
  is instant and identical offline.
- **First paint** — the window background is set to the dark shell color so there's
  no white flash before CSS loads (most visible on WebKitGTK).

## Files

```
desktop/
  main.go            Wails options, window, embed frontend/dist
  app.go             App (bound command surface) + eventSink (event.Sink → webview)
  wire.go            event.Event → JSON wire form (mirrors internal/serve/wire.go)
  wails.json         Wails project config (pnpm install/build/dev)
  frontend/
    src/
      lib/
        types.ts         wire contract (mirrors wire.go)
        bridge.ts        window.go/window.runtime wrapper + browser dev mock
        useController.ts event-stream reducer + command surface (the hook)
      components/
        Transcript, Message, ToolCard, Composer, ApprovalModal, ContextGauge,
        Markdown, CodeViewer, DiffView
        editors/  PlainCode, PlainDiff   ← editor seam impls (swap targets)
```

## Local diagnostics

The desktop has no anonymous launch ping, aggregate metrics uploader, crash
uploader, or project-owned reporting endpoint. Crash and performance prompts can
copy a scrubbed report or save it under the local Reames Agent diagnostics
directory. A Go panic or native hang watchdog report is archived there on the
next launch and is never transmitted automatically.

Local session telemetry remains available for token, cost, cache, read-file, and
Memory v5 diagnostics. It is stored beside local session state and is not a
network reporting pipeline. The Memory v5 runtime itself is controlled from
Settings > General > "Memory v5" and shares the user/global
`agent.memory_compiler.enabled` setting with the CLI/TUI and `reames-agent serve`;
CLI users can also run `/memory-v5 off|observe|compact|on|status` in a session
or `reames-agent config memory-v5 off|observe|compact|on|status` from a shell.
