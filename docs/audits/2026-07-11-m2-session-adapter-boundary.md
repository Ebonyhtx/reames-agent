# M2 会话适配器边界审计

日期：2026-07-11

状态：本地全量验证完成，待集中提交与推送

## 当前结论

本批继续按纵向路径收口会话边界，不改变 JSONL、branch meta、lease 或恢复格式：

- `control.SessionInfo` 增加 Bot 列表真正需要的稳定 `LastActivityAt` 与 `Scope`，`control.ListSessions` 保留 agent store 的排序与摘要语义；
- Bot project index 改用 `control.ListSessions`，附着会话恢复改用事务式 `Controller.ResumeSessionPath`，不再自行 `agent.LoadSession` 后绑定；
- CLI branch tree 通过 `control.BranchID` 识别当前分支，rename 通过 `control.RenameSession` 更新标题，用户错误前缀与 sidecar 锁语义保持不变；
- Serve resume 保留传输层路径验证与 HTTP 状态语义，但加载、租约 callback 和绑定改由 `ResumeSessionPath` 保证顺序；session list 复用 canonical 最近活动排序与 event-log-aware 摘要，cleanup/subagent 删除走 control；
- ACP session 用 `control.SessionLeaseKeeper` 代替 persistence lease 类型，recovery branch 原子 rebind、load/resume、延迟 cleanup 与重启 reconcile 均通过 control；
- Desktop session trash/restore 使用 control removal guard、内容一致性与 subagent artifact DTO，recovery GC 使用固定安全窗口，设置延迟 rebuild 使用 control lease probe/default/continue path；
- `TestTransportRuntimeImportRatchet` 同步删除 Bot gateway/project index、CLI branch/rename、Serve、ACP service、Desktop sessions/recovery GC/deferred rebuild/settings 的十条 `agent` allowlist 边。

前一批 CLI resume 已移除两条直连；至此稳定 session DTO/生命周期纵向路径累计从 CLI/Bot/Serve/ACP/Desktop 生产适配器移除十二条 `agent` 依赖。M2 仍未完成，Desktop prompt/tab、装配/provider 设置以及 CLI 专用渲染/装配直连继续保留在精确 allowlist 中。

## 本地验证

```text
go test ./internal/control ./internal/cli ./internal/bot -count=1   PASS
go test ./internal/control ./internal/serve ./internal/acp -count=1 PASS
desktop/go test . -run 'Settings|Deferred|Rebuild|Session|Trash|Cleanup|Recovery' -count=1 PASS
go build ./...                                                       PASS
go vet ./...                                                         PASS
go test ./internal/... -count=1 -timeout 10m                         PASS
desktop/go test ./... -count=1 -timeout 10m                          PASS
desktop/frontend/corepack pnpm test:all                               PASS
desktop/frontend/corepack pnpm build                                  PASS
Python Desktop/upstream contracts                                     PASS (44 tests, 1 skip)
builtin tool/docs/public/release contracts                            PASS
.\scripts\verify-baseline.ps1 -SkipFrontendHint                       PASS
git diff --check                                                      PASS
```

新增 control 测试覆盖活动时间映射、branch ID、rename 后列表读取、同进程 lease probe 与 removal guard；既有 CLI/Bot/Serve/ACP 与 Desktop 测试覆盖路径校验、HTTP 状态、命令路由、租约拒绝、snapshot recovery、重启恢复、cleanup、trash/restore、recovery GC 与设置延迟 rebuild。前端 build 只有既有 chunk/dynamic-import 警告且成功。本批尚未推送，因此仍不得冒充远端 CI 证据。
