# M2 Desktop session-store 边界审计

日期：2026-07-11

状态：已交付；commit `dc93f2a`

## 当前结论

本批关闭 Desktop Wails app 的 session-store 纵向路径，同时为 tabs 后续迁移建立 opaque handle：

- `control.SessionInfo` 补齐 Desktop 真正消费的 created/activity、workspace/topic、recovery 与 parent 字段；新增 metadata-only `SessionOrderInfo`、event-log-aware `SessionUserMessage`、`SessionUpdatedAt` 与 `SessionTopicBinding`，app 不再直接读取 agent listing/order/branch 类型；
- 新增 opaque `control.LoadedSession`：Desktop 可判断 placeholder 是否为空并交回 control 恢复，但不能读取/修改 provider message 或借用 persistence baseline；system prompt 刷新与 legacy system-less 兼容继续由既有 control 策略决定；
- 新增 opaque `control.SessionLease` 与 acquire/reclaim/owner-candidate API；tab lease transfer、rebuild self-reclaim 和 canonical path 不再暴露 agent lease/error/writer ID 类型，foreign readable owner 仍不可 reclaim，OS lock 仍是 metadata 损坏时的最终仲裁；
- cleanup、subagent deletion、snapshot conflict、session path、continue path、rename 与 fork topic binding 全部改用 control；`desktop/app.go` 删除最后一条 `agent` import，依赖棘轮同步收缩。tab 的 loaded session/lease 字段也已换成 opaque handle，剩余直连集中在 `tabs.go` 的 branch/index/migration 元数据；
- control 生产测试覆盖完整 DTO、topic binding、order/user-message/time、opaque loaded adoption、cleanup-pending 拒绝、lease canonical ownership/release 与 reclaim owner boundary；Desktop 既有 session/rebind/history/lease/clear/fork/prompt 回归继续覆盖真实调用链。

## 参考与取舍

DeepSeek Reasonix 上游当前仍由 Desktop 直接持有 `agent.SessionLease`、`agent.Session` 与 branch/listing 类型；本批保留其跨进程锁、event-log、sidecar 与 recovery 语义，只把 transport 所需的最小能力投影到 control，不复制另一套存储实现。`F:\Reames-Lite` 没有同构 Go session-store 边界，因此只用于核对用户可见会话/恢复合同。

## 当前验证

```text
go build ./...                                                       PASS
go vet ./...                                                         PASS
go test ./internal/... -count=1 -timeout 10m                         PASS
desktop/go test ./... -count=1 -timeout 10m                          PASS (222.0s)
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

前端 build 只有既有 dynamic-import/chunk-size 警告且成功。远端 CI run `29192648091` 为 8/8、CodeQL run `29192648051` 为 3/3；本批未重复触发原生 Desktop candidate。M2 与长期 GOAL 仍保持未完成。
