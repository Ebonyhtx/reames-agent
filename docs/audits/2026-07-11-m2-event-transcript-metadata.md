# M2 事件、转录与 metadata 边界审计

日期：2026-07-11

## 结论

本批关闭 M2 的 event/display DTO 与 Provider prompt metadata 两条纵向路径，但不声明 M2 或长期 GOAL 完成。共享事件现在具有版本化 envelope 和完整 cache payload；Serve/ACP 历史恢复改用展示安全的 control DTO；本地 UI metadata 与不透明渠道路由字段不再越过 Provider prompt 边界。

## 稳定事件与展示投影

- `eventwire.Event` 固定 `version=1`，保留 `source`，并将 `cache_updated` 的 diagnostics、session hit/miss counters 放入共享 JSON 合同；Desktop tab wrapper 不再重复声明同名字段。
- Desktop reducer 实际消费独立 cache update，不把它误算为新一轮 token usage；Go/TypeScript 合同测试和 reducer 测试覆盖 payload。
- 新增 `control.TranscriptMessage` / `TranscriptToolCall` / `TranscriptMemoryCitation`。投影保留可见用户/助手/工具内容、推理、diff、引用与 edit 展示数据，但隐藏 system、合成恢复指令，剥离 compose 控制块和 referenced-context payload；转换不修改运行时 session。
- Serve `/history` 与 ACP replay 改用该投影。HTTP 集成回归证明 system、reasoning-language 控制块和 referenced file 内容不出现在响应，ACP 回放回归证明隐藏项不会形成 notification。

## Provider 与渠道 metadata

- `provider.MessagesForRequest` 在消息跨越抽象 Provider interface 前剥离 `MemoryCitations`、`Edited`、`Original`；干净切片保持零分配路径，带 metadata 的切片浅复制且不修改持久化历史。
- Agent executor 与 Coordinator planner 均在调用 Provider 前应用该边界，`SanitizeToolPairing` 也做防御性剥离。未来自定义 Provider 不再需要“恰好知道”哪些字段只供 UI 使用。
- OpenAI-compatible 与 Anthropic builder 的完整 JSON wire bytes 在有/无本地 metadata 时一致。真实 Agent 双轮测试在首轮后只修改本地展示 metadata，第二轮同时证明 Provider request 无泄漏且 `PrefixChanged=false`。
- Gateway 群聊 prompt 只包含显式参与者标签和用户正文；connection/domain/chat/user/operator/message ID 均不进入 prompt。参与者名称是用户可见对话语义，不等同于不透明路由 metadata。

## 依赖棘轮

ACP replay 不再 import `internal/provider`，对应 `TestTransportRuntimeImportRatchet` allowlist 边已删除。其他会话持久化、装配和设置直连继续按路线图逐条迁移，未作一次性大搬家。

## 自动化证据

本批新增或扩展的关键覆盖：

- `internal/eventwire/wire_test.go`：版本/source/cache JSON 合同及 Desktop TypeScript 字段合同；
- `internal/control/transcript_dto_test.go`、`internal/serve/serve_test.go`、`internal/acp/dispatch_test.go`：隐藏 prompt material 与安全历史回放；
- `internal/provider/provider_test.go`、OpenAI/Anthropic builder tests、`internal/agent/metadata_boundary_test.go`、`cache_diagnostics_test.go`：Provider interface、wire bytes 与 cache prefix；
- `internal/bot/turn_dispatch_test.go`：群聊 prompt 与渠道路由 metadata 隔离；
- `desktop/frontend/src/__tests__/use-controller-meta.test.ts`：独立 cache event 的实际状态消费。

提交前门禁全部通过：

```text
go build ./...
go vet ./...
go test ./internal/... -count=1 -timeout 10m
desktop/go test ./... -count=1 -timeout 10m
desktop/frontend/corepack pnpm test:all
desktop/frontend/corepack pnpm build
python -m unittest scripts.test_smoke_desktop_interaction scripts.test_smoke_desktop_native scripts.test_check_upstreams -v
node scripts/test_upstream_watch_issue.mjs
go test ./internal/tool -run TestBuiltinToolContractDocumentation -v -count=1
python scripts/check_public_readiness.py
python scripts/check_release_contracts.py
.\scripts\verify-baseline.ps1 -SkipFrontendHint
git diff --check
```

前端 production build 只有既有的大 chunk 和 ineffective dynamic import 警告，构建成功；这属于 M3 性能债务，不是本批伪装关闭的事项。远端普通 CI/CodeQL 仍需本批集中 push 后确认。

## 证据边界

本审计证明确定性单元/集成合同、localhost HTTP 路径和前端 reducer 行为；它不证明真实 Provider 计费缓存命中、真实 IM 平台、公共云节点、生产签名或 notarization。上述外部证据仍按路线图保持已完成历史证据或 `external-blocked`，不得由本批 mock/fixture 替代。
