# M2 transport 边界收官审计

日期：2026-07-12

状态：已交付；commit `453a51c`

## 当前结论

本批关闭依赖棘轮中最后四个生产文件的十一条 `agent/provider/tool` 直连，使 Desktop、CLI、Serve、Bot 与 ACP 的受守卫 transport allowlist 归零：

- compile-time OpenAI/Anthropic 注册归入 `internal/boot`；`cmd/reames-agent/main.go` 与 `desktop/main.go` 不再各自 blank-import provider/tool 实现，boot 测试直接证明 provider kinds 和关键 builtins 已注册；
- CLI run/serve/chat 的 resume 使用 opaque `control.LoadedSession`，model/effort/skill rebuild 使用 opaque `SessionHistorySnapshot`，session list/model/path/lease error 均走 control；模型切换采用新 controller 的 system prompt，避免继续携带旧装配前缀；
- TUI 初始回放、branch replay、copy 与 export 改用展示安全 `control.TranscriptMessage`，隐藏 system、合成 user 和 referenced-context payload；hidden synthetic user 不再错误截断当前回答的复制范围；
- ANSI TextSink、usage line 与 ANSI/CJK/emoji 终端宽度实现迁入 `internal/termrender`，agent 仅保留内部兼容转发；CLI 专用渲染不再由 agent runtime 拥有；
- review subagent 的 provider/tool/guard 装配迁入 boot，CLI 只负责参数、diff、skill 选择和输出；setup 模型探测复用 `config.FetchModelListCompat`，不再直连 OpenAI 实现；
- `TestTransportRuntimeImportRatchet` 的历史 allowlist 为空。累计从 transport 生产文件收缩四十条 `agent/provider/tool` import，新增任意一条都会直接使 CI 失败。

## 参考与取舍

DeepSeek Reasonix 上游当前仍在 Desktop/CLI main 维护 blank imports，CLI 直接调用 agent TextSink/OpenAI fetch，并在 review 命令内构建 tool registry。本批保留其 provider 注册、resume、review guard 与终端字节输出合同，但把装配、runtime handoff 和终端展示放回 boot/control/termrender 的职责边界；不复制另一套 provider 或 session 实现。`F:\Reames-Lite` 没有同构 Go 边界，仅用于核对 CLI resume/export 与用户可见输出合同。

## 当前验证

```text
go build ./...                                                       PASS
go vet ./...                                                         PASS
go test ./internal/... -count=1 -timeout 10m                         PASS
desktop/go test ./... -count=1 -timeout 10m                          PASS
desktop/frontend/corepack pnpm test:all                              PASS
desktop/frontend/corepack pnpm build                                 PASS
CI-scoped Python scripts.* suites                                    PASS (69 tests, 2 platform skips)
Node upstream issue + docs/public/deploy/release contracts           PASS
go test ./internal/tool -run TestBuiltinToolContractDocumentation -v PASS
python scripts/check_upstreams.py --out-dir artifacts/upstream-watch PASS
six-target CGO_ENABLED=0 cross-compile                               PASS
.\scripts\verify-baseline.ps1 -SkipFrontendHint                      PASS
go test ./internal/control -run TestTransportRuntimeImportRatchet    PASS
guarded production import scan                                      0 matches
git diff --check                                                     PASS
```

Python 数量仅统计 `.github/workflows/ci.yml` 显式运行的当前脚本合同；前端 build 只有既有 dynamic-import/chunk-size 警告且成功。远端 CI run `29195337394` 为 8/8、CodeQL run `29195337395` 为 3/3；缓存恢复与 Node 20 弃用提示均为非阻断 annotation。本批不改变 Wails UI 或安装工件，未重复触发 Desktop candidate。M2 统一控制面里程碑关闭，长期 GOAL 继续进入 M3。
