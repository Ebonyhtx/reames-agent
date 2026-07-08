# Reames Agent Changelog

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
