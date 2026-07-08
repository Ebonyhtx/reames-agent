# Contributing to Reames Agent

## Setup

```bash
git clone <repo-url>
cd reames-agent
# Go 1.25+ required
go build -o bin/reames-agent.exe ./cmd/reames-agent
go test ./internal/... -count=1
```

中国大陆用户设置 Go 代理：

```bash
export GOPROXY=https://goproxy.cn,direct
export GOSUMDB=sum.golang.google.cn
```

## Development workflow

1. **Pick a task** from `docs/FUTURE_PLAN.md` or create an issue
2. **Read the relevant package comment** in `internal/<package>/` 
3. **Implement** following Go conventions
4. **Write tests** — every new package or function needs `_test.go`
5. **Verify**:
   ```bash
   go build ./...
   go vet ./...
   go test ./internal/<your-package>/... -count=1
   ```
6. **Check brand**: `grep -rn 'reasonix\|Reasonix' --include='*.go' -l | grep -v 'reames-agent' | wc -l` must be 0
7. **Commit** with descriptive message

## Architecture rules

- CLI/Desktop/Web must talk to core through `control.Controller` — never import `internal/agent` directly
- System prompt is cache-stable — dynamic state goes in user-turn compose
- UI state, diagnostics, settings MUST NOT enter provider-visible prompt content
- New tools register via `tool.RegisterBuiltin()` in `internal/tool/builtin/`
- Document new tools in `docs/TOOL_CONTRACT.md` and `docs/TOOL_CONTRACT.zh-CN.md`

## Testing

```bash
# All new-package tests (fast)
go test ./internal/crypto/... ./internal/trust/... ./internal/cron/... \
  ./internal/board/... ./internal/pluginpkg/... ./internal/lsp/... -count=1

# Core tests
go test ./internal/config/... ./internal/agent/... ./internal/provider/... -count=1

# Builtin tool tests
go test ./internal/tool/builtin/... -count=1

# Tool contract verification
go test ./internal/tool -run TestBuiltinToolContractDocumentation -v
```

## Building

```bash
# Local
go build -o bin/reames-agent.exe ./cmd/reames-agent

# Cross-compile
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/reames-agent-linux-amd64 ./cmd/reames-agent
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/reames-agent-darwin-arm64 ./cmd/reames-agent
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/reames-agent-windows-amd64.exe ./cmd/reames-agent
```

## Release

```bash
git tag v0.1.0
git push origin v0.1.0
# CI will build and publish via .goreleaser.yaml
```
