# Reames Agent

A multi-platform AI coding agent. Terminal-native CLI, desktop app, cloud-deployable server, and IM bot gateway — all from a single Go binary.

**Based on DeepSeek Reasonix (MIT), informed by 9 reference projects and the original Reames Lite.**

## Quick Start

```bash
# Build from source (requires Go 1.25+)
git clone https://github.com/Ebonyhtx/reames-agent.git
cd reames-agent
go build -o bin/reames-agent ./cmd/reames-agent

# Stable public releases are not enabled yet.
# Maintainer-only candidate artifacts are documented in docs/RELEASING.md.

# Setup
./bin/reames-agent setup

# Start interactive session
./bin/reames-agent
```

One-command source installer while stable release artifacts are still disabled:

```bash
curl -fsSL https://raw.githubusercontent.com/Ebonyhtx/reames-agent/main/scripts/install.sh | bash
powershell -ExecutionPolicy Bypass -c "iex (irm https://raw.githubusercontent.com/Ebonyhtx/reames-agent/main/scripts/install.ps1)"
```

## Features

- **Multi-model**: DeepSeek, OpenAI-compatible, Anthropic — config-driven, no hardcoded models
- **Cache-first**: DeepSeek prefix cache optimization, 95%+ hit rate target
- **Shared controller, multiple surfaces**: CLI/TUI, Desktop (Wails + React), Web/Cloud (HTTP/SSE), ACP, and IM gateway
- **Three work modes**: `economy` minimizes optional tool-schema cost, `balanced` exposes the full stable tool set, and `delivery` adds an evidence-backed completion contract
- **IM Gateway**: Feishu, QQ, WeChat, Telegram bot adapters
- **Plugin/MCP**: MCP stdio + HTTP transports, skill playbook system
- **Offline recovery**: credential-free Guard, crash-loop detection, verified update rollback, and Safe Mode
- **Portable CLI**: CGO_ENABLED=0, cross-compile to 6 targets; Desktop packages add a sibling Guard launcher

## Usage

```bash
reames-agent                        # Interactive CLI session
reames-agent run "fix the auth bug" # Headless single task
reames-agent serve                  # Start web UI on localhost:8787
reames-agent gateway run --channels feishu    # Run IM gateway in foreground
reames-agent gateway install --dry-run --channels feishu  # Preview background service install
reames-agent guard check --json        # Credential-free recovery report
reames-agent guard launch --safe-mode  # Open the recovery-only Desktop shell
```

Select a work mode at startup, or use `/work-mode` inside the interactive TUI:

```bash
reames-agent --profile economy
reames-agent run --profile delivery "ship the requested change"
reames-agent serve --profile balanced
reames-agent acp --profile delivery
```

`balanced` is the default. Work modes change the stable execution contract, not
permission, sandbox, evidence, checkpoint, or project-check enforcement.

## Cloud Deployment

```bash
docker build -t reames-agent .
docker run -p 127.0.0.1:8787:8787 \
  -e DEEPSEEK_API_KEY=replace-with-your-key \
  -e REAMES_AGENT_SERVE_TOKEN=change-this-long-random-token \
  reames-agent
```

See [docs/DEPLOY.md](docs/DEPLOY.md) for systemd, nginx, and SSH deployment guides.

## Release Status

This repository is still before its first public stable release. The current
safe distribution path is source builds and maintainer-reviewed candidate
artifacts. Production release, updater, and package-manager publishing remain
disabled until the gates in [docs/RELEASING.md](docs/RELEASING.md) and
[docs/PUBLIC_READINESS.md](docs/PUBLIC_READINESS.md) are satisfied. Reames-owned
startup, metrics, performance, and crash upload endpoints are permanently out of
scope; diagnostics and feedback stay local unless the user explicitly exports
them.

## Documentation

- [Project Direction](docs/PROJECT.md)
- [Development Plan](docs/DEVELOPMENT_PLAN.md)
- [Documentation Index](docs/DOCS_INDEX.md)
- [Architecture](docs/ARCHITECTURE.md)
- [Deployment Guide](docs/DEPLOY.md)
- [Recovery, Guard, and Safe Mode](docs/RECOVERY.md)
- [Upstream Governance](docs/REFERENCE_GOVERNANCE.md)

## License

MIT. Based on [DeepSeek Reasonix](https://github.com/esengine/DeepSeek-Reasonix).
See [NOTICE.md](NOTICE.md) for attribution notes.

## Development

See [AGENTS.md](AGENTS.md) for AI agent instructions and [CONTRIBUTING.md](CONTRIBUTING.md) for developer setup.

```bash
git clone <repo-url> && cd reames-agent
go build -o bin/reames-agent.exe ./cmd/reames-agent
go test ./internal/... -count=1
```

China mainland: `export GOPROXY=https://goproxy.cn,direct`

## New Modules (from reference projects)

| Module | Source | Purpose |
|---|---|---|
| `internal/crypto/` | AgentArk | AES-256-GCM encrypted credential store |
| `internal/trust/` | AgentArk | HTML sanitization + output envelope |
| `internal/cron/` | Hermes | Persistent scheduled tasks |
| `internal/provider/classify.go` | Hermes | Error classifier (12 failover reasons) |
| `internal/lsp/` | Codex | LSP Delta baseline diagnostics |
| `internal/bot/telegram/` | — | Telegram Bot API adapter |
