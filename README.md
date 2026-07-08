# Reames Agent

A multi-platform AI coding agent. Terminal-native CLI, desktop app, cloud-deployable server, and IM bot gateway — all from a single Go binary.

**Based on DeepSeek Reasonix (MIT), enhanced with features from 8 reference projects and the original Reames Lite.**

## Quick Start

```bash
# Build from source (requires Go 1.25+)
git clone https://github.com/reames-agent/reames-agent.git
cd reames-agent
go build -o bin/reames-agent ./cmd/reames-agent

# Or download prebuilt binary from releases
# https://github.com/reames-agent/reames-agent/releases

# Setup
./bin/reames-agent setup

# Start interactive session
./bin/reames-agent
```

## Features

- **Multi-model**: DeepSeek, OpenAI-compatible, Anthropic — config-driven, no hardcoded models
- **Cache-first**: DeepSeek prefix cache optimization, 95%+ hit rate target
- **Three surfaces**: CLI (Bubble Tea TUI), Desktop (Wails + React), Web/Cloud (HTTP/SSE server)
- **IM Gateway**: Feishu, QQ, WeChat, Telegram bot adapters
- **Plugin/MCP**: MCP stdio + HTTP transports, skill playbook system
- **Single binary**: CGO_ENABLED=0, cross-compile to 6 targets

## Usage

```bash
reames-agent                        # Interactive CLI session
reames-agent run "fix the auth bug" # Headless single task
reames-agent serve                  # Start web UI on localhost:8787
reames-agent gateway start --channels feishu  # Start IM bot
```

## Cloud Deployment

```bash
docker build -t reames-agent .
docker run -p 8787:8787 -e DEEPSEEK_API_KEY=sk-xxx reames-agent
```

See [docs/DEPLOY.md](docs/DEPLOY.md) for systemd, nginx, and SSH deployment guides.

## Documentation

- [Architecture](docs/ARCHITECTURE.md)
- [Deployment Guide](docs/DEPLOY.md)
- [Product Roadmap](docs/PRODUCT_ROADMAP.md)
- [Reference Porting Roadmap](docs/REFERENCE_PORTING_ROADMAP.md)

## License

MIT. Based on [DeepSeek Reasonix](https://github.com/esengine/DeepSeek-Reasonix).

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
