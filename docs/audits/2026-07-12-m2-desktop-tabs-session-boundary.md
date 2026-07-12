# M2 Desktop tabs 会话边界审计

日期：2026-07-12

状态：已交付；主 commit `9effae6`，索引修复 `cc92f67`

## 当前结论

本批关闭 `desktop/tabs.go` 对 `internal/agent` 的最后一条生产依赖：

- 新增稳定 `control.SessionMeta`，只投影 Desktop 实际消费的 ownership、topic、profile、recovery 和 activity 字段，不暴露 revision、writer、content digest、in-flight turn 等持久层内部状态；
- 新增 `control.UpdateSessionMeta`，在 agent session-meta 锁内执行原子读改写。legacy migration、topic restore/rename、tab profile 保存不再由 Desktop 获取锁或直接保存 branch sidecar；更新以原始 agent metadata 为底，未投影字段不会在 DTO 往返时丢失；
- session model、preview、user task、listing/order、cleanup marker、branch ID、content probe 和 recovery metadata callback 全部改走 control；`boot.Options` 与 `control.Options` 的恢复回调不再暴露 `agent.BranchMeta`；
- `desktop/tabs.go` 删除 `internal/agent` import，transport dependency ratchet 同步删除该豁免。Desktop app/tabs 现均无 `agent/provider/tool` 生产直连；累计收缩二十九条受守卫依赖边；
- control 回归覆盖原子 mutation、错误不落盘，以及 revision/digest/writer/in-flight/listing/recovery-depth 等隐藏字段保留；Desktop 既有 topic migration/index repair/recovery/profile/cache 测试继续覆盖真实调用链。

## 参考与取舍

DeepSeek Reasonix 上游当前仍由 Desktop tabs 直接读取 `agent.BranchMeta`、获取 session-meta 锁并持有 listing 类型。Reames 保留上游的 sidecar、event-log、跨进程 lease 和 per-path lock 语义，但将 transport 所需能力投影为 control DTO 与原子操作，不复制第二套 session store。`F:\Reames-Lite` 没有同构 Go 持久化边界，本批仅核对用户可见的会话归属、标题、恢复和 profile 合同。

## 当前验证

```text
go build ./...                                                        PASS
go vet ./...                                                          PASS
go test ./internal/... -count=1 -timeout 10m                          PASS
desktop/go test ./... -count=1 -timeout 10m                           PASS (198.1s wall time)
desktop/frontend/corepack pnpm test:all                               PASS
desktop/frontend/corepack pnpm build                                  PASS
CI-scoped Python scripts.* suites                                     PASS (69 tests, 2 platform skips)
Node upstream issue + docs/public/deploy/release contracts            PASS
go test ./internal/tool -run TestBuiltinToolContractDocumentation -v  PASS
python scripts/check_upstreams.py --out-dir artifacts/upstream-watch  PASS
six-target CGO_ENABLED=0 cross-compile                                PASS
.\scripts\verify-baseline.ps1 -SkipFrontendHint                       PASS
go test ./internal/control -run TestTransportRuntimeImportRatchet     PASS
go test ./internal/control -run TestUpdateSessionMetaPreserves...     PASS
git diff --check                                                      PASS
```

Python 数量仅统计 `.github/workflows/ci.yml` 显式运行的当前脚本合同，不把已隔离、依赖旧 Hermes Python 环境的遗留 `tests/` 树冒充产品门禁。前端 build 只有既有 dynamic-import/chunk-size 警告且成功。首个 CI run `29193639119` 在干净 clone 暴露新审计漏列 `DOCS_INDEX.md`，随后以索引修复 `cc92f67` 重跑：CI run `29193741370` 为 8/8、CodeQL run `29193741351` 为 3/3；本批未重复触发原生 Desktop candidate。M2 与长期 GOAL 仍未完成。
