# Reames Agent Changelog

## Unreleased

### Agent Reliability

- Made Goal completion evidence-gated in every mode; repeated completion claims can no longer override incomplete canonical todos or project checks, and the host no longer fabricates final todo events.
- Added a versioned session runtime projection for recoverable Goal/Plan/Todo state, continuation budgets, strict self-checks, transcript-digest freshness, and per-path monotonic revisions across resume, branch, fork, switch, and rewind.
- Hardened checkpoint rewind with transcript-prefix digests, runtime preflight, durable truncate tombstones, workspace/symlink confinement, path-alias handling, transactional file rollback including a partially written failing target, and relative-path mode restoration. Checkpoint writes use `AtomicWriteFile`; its Windows cross-device fallback and the remaining path/dual-resource crash windows are documented rather than described as unconditionally crash-safe.
- Corrected idle-loop detection, required strict completion to follow an actual host self-check turn, and exposed current-turn evidence to Board without treating it as durable proof.
- Added fail-closed background task recovery: persisted subagents save every provider/tool/compaction boundary, recoverable jobs publish running metadata before launch, stale running work reloads as an explicit `interrupted`/`continue_from` tombstone, and side-effecting tools are never replayed automatically. Unified tests cover compacted transcript continuation and explainable, disableable, deletable memory recall without dynamic system-prefix pollution.
- Preserved the coherent partial transcript and runtime boundary after an exhausted provider stream recovery, so the Desktop Continue action sees the response it promises to keep; commit failures and non-stream errors still roll back fail closed.
- Honored DeepSeek thinking-mode responses that explicitly stop after a non-empty reasoning stream but leave ordinary content empty, preventing redundant expensive retries while preserving the empty-answer guard for every other provider protocol.

### Security / Governance

- Added the strict `.reames-theme` v1 contract with semantic-token and recipe allowlists; ZIP traversal, symlink, Windows-device-name, duplicate, count, size, expansion-ratio, compression-bomb, image-format, edge, pixel, and full-SHA-256 validation; and a public schema checked against the Go allowlist.
- Raised the pinned build toolchain to Go 1.26.5 so release and source builds include the standard-library fixes for GO-2026-5856 and GO-2026-4970; the source minimum remains Go 1.25 with automatic toolchain selection.
- Migrated CI, CodeQL support steps, candidates, upstream watch, and deploy workflows from embedded Node.js 20 action majors to official Node.js 24 majors, with a public-readiness ratchet that rejects their return.
- Added an opt-in TUF plugin registry client with an out-of-band bootstrap root, project-resistant trust configuration, persistent rollback/freeze metadata, sequential root rotation, strict signed index/provenance bindings, canonical cross-platform Git source digests, ambient-Git-config isolation, apply-time re-resolution, lifecycle evidence persistence, and packaged Apache-2.0/NOTICE attribution. No public registry or TOFU endpoint is enabled; optional attestation targets are authenticated bytes, not claimed DSSE identity or SLSA policy verification.
- Added a read-only production registry auditor that replays sequential dual-threshold root rotation, authenticates the full metadata/target chain, enforces independent 2-of-3 root/targets keys and bounded expiries, verifies every referenced attestation byte, and emits machine-readable evidence that retains the external ceremony/HSM/endpoint/monitoring boundary.
- Unified plugin state, install, and signed-registry names under one portable ASCII identity: case aliases, trailing dots, and Windows reserved device names now fail before materialization, preventing cross-platform generation/state ownership collisions.
- Isolated installed-package Hook/MCP processes behind a fail-closed OS sandbox with core-only wrapper environments, post-confinement child-environment restoration that keeps manifest secrets out of wrapper argv, managed state/temp roots, sensitive-path read barriers, bounded/redacted diagnostics, process-tree cancellation, and active Hook revocation. Missing helper payloads or dispatch routes fail closed. Added Windows shebang compatibility for verified package hooks and validated a pinned unsigned `obra/superpowers` install/enable/SessionStart flow in the native Windows sandbox.
- Added exact-plan structured approval for model-driven `install_source` apply calls across Controller, Desktop, CLI, Bot, Serve/event wire, and ACP. Planning remains invocation-level read-only; apply requires a fresh human decision that YOLO, auto mode, Guardian, plan execution windows, grants, and headless autonomy cannot replace. Unsupported and headless hosts fail closed before preview or mutation, and MCP URL/command/args/environment/header details are structurally redacted before display or persistence.
- Added public-readiness gates for ownership, release safety, deployment docs, and attribution.
- Restored CodeQL workflow for Go, JavaScript/TypeScript, and GitHub Actions analysis.
- Documented SemVer source, changelog expectations, and signing/checksum strategy before production releases.
- Removed inherited Hermes/Python scripts from the public `scripts/` surface and added a regression gate to keep old release, live-test, Open WebUI, and WhatsApp bridge entrypoints from returning.
- Rebranded the private root Node workspace metadata from inherited Hermes links to Reames and added public-readiness gates against tracked build artifacts.
- Added a manual native Desktop candidate workflow that builds short-lived Wails artifacts without publishing or signing.
- Added public-readiness gates for telemetry/crash-reporting boundaries so feedback upload stays disabled until a Reames-owned endpoint exists.
- Added an all-workflow release-surface ratchet: pre-stable Reames permits only the read-only snapshot candidate workflow and rejects production publishing permissions, GitHub Release actions, npm publishing, and non-snapshot GoReleaser commands.
- Corrected Auto/YOLO/Full Access copy in English, Simplified Chinese, and Traditional Chinese so it promises automatic approval only for ordinary tools and explicitly retains deny, ask, plan, and fresh-trust prompts.

### Desktop

- Added a lazy Appearance/Gallery with separate select, reversible preview, and apply actions; content-addressed immutable scene delivery; atomic install/active-replace/delete recovery; restart rollback of uncommitted previews; Safe Mode Graphite fallback; and read-only official/user pack partitioning. Official packs now use the same allow-listed frontend runtime projection as user packs while remaining non-replaceable and non-deletable; every Wails pack-mutation binding rejects Safe Mode before opening a picker or touching the store.
- Added two original MIT-licensed Reames themes with embedded digest-verified artwork and repository-recorded prompts, generation IDs, dimensions, conversion steps, and SHA-256 provenance. No Reasonix branding, artwork, marketplace, endpoint, telemetry, or second runtime was imported.
- Kept installed recovery smoke ownership attached across platforms by launching Guard with `--detach=false`; the harness now terminates the real Safe Mode Desktop process tree before writing derived-state fixtures, eliminating a macOS quarantine race without weakening the recovery contract.
- Kept fresh-clone Windows production builds Git-clean by restoring the embedded frontend `dist` placeholder as a byte-empty file, avoiding CRLF/LF-only worktree drift after Vite clears stale assets.
- Added authenticated signed-registry search and release selection to Plugin settings, including trusted registry/root evidence in preview and installed-plugin details; an unconfigured registry fails closed without changing direct local/Git installs.
- Added a structured plugin/source approval modal that displays operation, risk, target, MCP execution details, version/digest changes, trust, requested permissions, source revision, warnings, and enable state from the same exact plan used by every host.
- Added true-modal background isolation, stable dialog identities, inherited opener restore chains, transcript accessibility semantics, and a strict Windows UIA accessibility smoke. Actual NVDA/Narrator listening and Windows High Contrast validation remain manual evidence.
- Added real-Chromium and native Wails plugin lifecycle smoke coverage for stale-plan rejection, disabled-by-default install, exact permission approval, generation update/rollback, diagnostics, removal, and isolated-state cleanup; the installed Windows candidate now retains the native evidence artifact.
- Added a lazy-loaded Desktop Recovery Center for normal and recovery-only Safe Mode, with redacted shared evidence and bounded config repair/restore/undo, verified update rollback, derived-state quarantine, and plugin-disable actions. Installed Linux/macOS/Windows candidates now run a credential-free recovery smoke; the latest local Windows Wails/Guard run passes, while signed public-release drills remain external evidence.

### Deployment

- Hardened cloud serve deployment contracts around token-based authentication, Docker health checks, systemd environment loading, and loopback defaults.
- Clarified the Hermes-like deployment shape: CLI/TUI remains interactive, while social channels run through an independent background gateway service.
- Added `reames-agent gateway run` as the foreground social gateway entrypoint; `reames-agent bot start` remains compatible.
- Added `reames-agent gateway install/start/stop/restart/status/uninstall` with dry-run service plans for systemd, launchd, and Windows Scheduled Task.
- Added `reames-agent gateway setup` for atomic, idempotent, fail-closed headless connection configuration across Feishu/Lark, QQ, and WeChat. It accepts secret environment-variable names only and provides redacted no-write previews.
- Hardened Linux Gateway user services with directive-aware systemd rendering, absolute persistent paths, atomic unit writes, immediate same-name reinstall restart/readiness checks, correct uninstall reload ordering, and a credential-free installed lifecycle smoke.
- Repaired CLI self-upgrade discovery to use the official Reames Agent repository and exact six-platform GoReleaser archive/binary names.
- Replaced inherited Hermes installer scripts with Reames source-build installers for Unix, PowerShell, and CMD.
- Audited Reasonix, Hermes, and Reames install/deploy entrypoints; fixed stale Chinese README gateway and API-key examples.

### Upstream

- Kept upstream/reference tracking issue-driven; automatic discovery may propose review work but must not auto-merge upgrades.
- Reviewed Reasonix Theme Pack V2 at the source/test/schema level and implemented the independently verified manifest, storage, preview, and recovery mechanisms without importing its branding, assets, marketplace, or production release infrastructure.

## v0.1.0 (2026-07-08)

### Initial Release

Based on DeepSeek Reasonix main-v2 @ 07c65c2 (MIT License).

**Core:**
- Multi-model support (DeepSeek, OpenAI-compatible, Anthropic)
- Cache-first context management with prefix stability
- Bubble Tea CLI, Wails Desktop, HTTP/SSE Web server
- MCP/Plugin system with skill playbooks
- Memory v5 execution compiler
- Session management with branching/forking/checkpoints

**Cloud:**
- Docker support with healthcheck
- systemd + nginx deployment guides
- SSH headless execution

**IM Gateway:**
- Feishu, QQ, WeChat adapters (from Reasonix)
- Telegram adapter (new)
- PlatformAdapter extensible interface

**Enhancements (from Reames Lite + reference projects):**
- web_search tool (DuckDuckGo, no API key)
- apply_patch tool (unified diff)
- list_jobs tool (background task discovery)
- Goal triple budget (steps + tokens + wall-clock)
- Pending prompt snapshots (crash recovery)
- Board system (unified work-board projection)
- Plugin registry (search, categories)
- Skill frontmatter: tags, platforms, related-skills
- Hook glob matching (`bash*`, `write_*`)
- Cron job persistence
- AES-256-GCM + Argon2id crypto
- HTML sanitization + secret redaction
- Health/readiness/WebSocket endpoints
