# Reames Agent Changelog

## Unreleased

### Agent Reliability

- Made Goal completion evidence-gated in every mode; repeated completion claims can no longer override incomplete canonical todos or project checks, and the host no longer fabricates final todo events.
- Added a versioned session runtime projection for recoverable Goal/Plan/Todo state, continuation budgets, strict self-checks, transcript-digest freshness, and per-path monotonic revisions across resume, branch, fork, switch, and rewind.
- Hardened checkpoint rewind with transcript-prefix digests, runtime preflight, durable truncate tombstones, workspace/symlink confinement, path-alias handling, transactional file rollback including a partially written failing target, and relative-path mode restoration. Checkpoint writes use `AtomicWriteFile`; its Windows cross-device fallback and the remaining path/dual-resource crash windows are documented rather than described as unconditionally crash-safe.
- Corrected idle-loop detection, required strict completion to follow an actual host self-check turn, and exposed current-turn evidence to Board without treating it as durable proof.
- Added fail-closed background task recovery: persisted subagents save every provider/tool/compaction boundary, recoverable jobs publish running metadata before launch, stale running work reloads as an explicit `interrupted`/`continue_from` tombstone, and side-effecting tools are never replayed automatically. Unified tests cover compacted transcript continuation and explainable, disableable, deletable memory recall without dynamic system-prefix pollution.

### Security / Governance

- Added public-readiness gates for ownership, release safety, deployment docs, and attribution.
- Restored CodeQL workflow for Go, JavaScript/TypeScript, and GitHub Actions analysis.
- Documented SemVer source, changelog expectations, and signing/checksum strategy before production releases.
- Removed inherited Hermes/Python scripts from the public `scripts/` surface and added a regression gate to keep old release, live-test, Open WebUI, and WhatsApp bridge entrypoints from returning.
- Rebranded the private root Node workspace metadata from inherited Hermes links to Reames and added public-readiness gates against tracked build artifacts.
- Added a manual native Desktop candidate workflow that builds short-lived Wails artifacts without publishing or signing.
- Added public-readiness gates for telemetry/crash-reporting boundaries so feedback upload stays disabled until a Reames-owned endpoint exists.

### Desktop

- Added true-modal background isolation, stable dialog identities, inherited opener restore chains, transcript accessibility semantics, and a strict Windows UIA accessibility smoke. Actual NVDA/Narrator listening and Windows High Contrast validation remain manual evidence.
- Added real-Chromium and native Wails plugin lifecycle smoke coverage for stale-plan rejection, disabled-by-default install, exact permission approval, generation update/rollback, diagnostics, removal, and isolated-state cleanup; the installed Windows candidate now retains the native evidence artifact.

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
