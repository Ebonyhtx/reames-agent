# M2 CLI/ACP composition 边界审计

日期：2026-07-11

状态：本地全量验收通过；commit 与远端 CI/CodeQL 证据待交付后回填

## 当前结论

本批关闭 ACP production composition、CLI session copy 与 MCP name rendering 三条相邻边界：

- ACP 每个 session 的真实生产装配早已统一调用 `boot.Build`；删除 `acpBuiltinTools`、`acpTaskProfileDefaults`、`newACPSubagentProviderResolver` 三组只由同文件测试调用、无生产 caller 的 Reasonix 初始导入遗留，以及相应死代码测试。`internal/boot` 现有测试继续覆盖 workspace builtins、task/subagent model 与 effort precedence；
- 新增 `control.CopySessionForWriting`，CLI `run/chat --copy` 不再直接装配 `agent.Session`、branch meta 与路径。复制仍拒绝 cleanup-pending source，保持 event-log-aware transcript、父分支、显示标题、模型与无 lease 的新路径语义；control 生产测试验证源文件只读、transcript/meta/lineage 与 lease 状态；
- 新增零 runtime 依赖的 `internal/mcpname`，集中定义 `mcp__<server>__<tool>` 的 model-visible 命名合同。Agent、capability、skill 与 CLI 共用该解析器，CLI tool card 和 approval renderer 不再仅为解析名称而 import tool registry；
- `TestTransportRuntimeImportRatchet` 删除 `internal/cli/acp.go` 的三条 provider/tool 边、`session_lease.go` 的 agent 边、`toolcard.go` 与 `chat_tui.go` 的两条 tool 边。本批收缩六条，项目累计收缩二十七条；Serve、Bot 与 ACP 已无受守卫 runtime 直连。
- 全量门禁暴露 Windows `C:\Windows\System32\bash.exe` 可能只是不可用 WSL app alias，并输出 Python 默认代码页无法解码的错误；安装器合同现在先以 bytes 探测真正 GNU bash，并在 Windows 回退 Git for Windows，不再把 app alias 当成可用 shell。

## 参考与取舍

`git blame` 与全仓 `rg` 证明三组 ACP helper 来自 DeepSeek Reasonix 初始导入且只剩测试 caller；当前 Reames 的 `boot.Build` 已取代这套平行装配。上游会话 lease/store 仍提供底层语义，本批不复制或改写磁盘格式，只把 transport 所需的完整 copy 操作提升到 control 边界。`F:\Reames-Lite` 没有可直接复用的 Go control/tool naming 边界，因此只保留既有用户合同，不人为移植另一套实现。

## 当前验证

```text
go build ./...                                                       PASS
go vet ./...                                                         PASS
go test ./internal/... -count=1 -timeout 10m                         PASS
desktop/go test ./... -count=1 -timeout 10m                          PASS (201.3s)
desktop/frontend/corepack pnpm test:all                               PASS
desktop/frontend/corepack pnpm build                                  PASS
Python Desktop/upstream/installer contracts                           PASS (69 tests, 2 platform skips)
Node upstream issue + docs/public/deploy/release contracts            PASS
go test ./internal/tool -run TestBuiltinToolContractDocumentation -v  PASS
python scripts/check_upstreams.py --out-dir artifacts/upstream-watch  PASS
six-target CGO_ENABLED=0 cross-compile                                PASS
.\scripts\verify-baseline.ps1 -SkipFrontendHint                       PASS
go test ./internal/control -run TestTransportRuntimeImportRatchet     PASS
git diff --check                                                      PASS
```

前端 build 只有既有 dynamic-import/chunk-size 警告且成功。远端 CI/CodeQL 尚未运行；在远端证据补齐前，本批不得声明已交付，M2 与长期 GOAL 仍保持未完成。
