# M2 Desktop transcript 边界审计

日期：2026-07-11

状态：本地全量验证完成，待集中提交与单次推送

## 当前结论

本批关闭 Desktop history 出口与 rebuild 的 `provider.Message` 纵向路径：

- `control.TranscriptMessage` 新增本地-only `DisplayKey` 与 `ReplayText`：前者保持既有 SHA-256 sidecar key 字节兼容，后者只提供剥离 compose/referenced-context 后的正常用户文本；system、synthetic、steer 与 Memory Compiler execution 均不可 replay；两字段使用 `json:"-"`，不会进入 Serve/ACP wire；
- Desktop history、分页、checkpoint、tool/todo replay、planner sidecar 和 legacy transcript preview 改用展示安全 transcript；checkpoint 直接使用原始 `Index`，分页不再重建 provider index 映射；
- `.display.json` 与 planner sidecar 使用 control 生成的 opaque key 关联，含 referenced-context 的旧记录仍可命中展示文本，但文件内容不会进入 `Content` 或 `SubmitText`；
- model/effort/token-mode rebuild 改用 opaque `SessionHistorySnapshot`，Desktop 不再声明 carried `[]provider.Message`；
- event-log preview 与 planner citation 改用稳定 control citation/tool DTO，Desktop app/tabs 两条 `provider` 生产依赖棘轮已删除。

## 当前验证

```text
go build ./...                                                        PASS
go vet ./...                                                          PASS
go test ./internal/... -count=1 -timeout 10m                          PASS
desktop/go test ./... -count=1 -timeout 10m                           PASS (169.9s)
desktop/frontend/corepack pnpm test:all                                PASS
desktop/frontend/corepack pnpm build                                   PASS
Python Desktop/upstream/installer contracts                            PASS (69 tests, 2 platform skips)
Node upstream issue + docs/public/deploy/release contracts             PASS
six-target CGO_ENABLED=0 cross-compile                                 PASS
.\scripts\verify-baseline.ps1 -SkipFrontendHint                        PASS
go test ./internal/control -run TestTransportRuntimeImportRatchet     PASS
git diff --check                                                       PASS
```

定向回归覆盖 sidecar/planner hash 兼容、referenced-context 不可 replay、Memory Compiler 不可 replay、synthetic/steer 隐藏、checkpoint 原索引、历史分页、tool payload 归档、todo `complete_step` replay、UTF-8 错误截断及三类 runtime rebuild。前端 build 只有既有 dynamic-import/chunk-size 警告且成功。本批尚未 commit/push；当前证据不得冒充远端 CI 或原生 Desktop 交互证据。
