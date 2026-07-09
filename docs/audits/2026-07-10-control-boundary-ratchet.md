# Control 传输边界依赖棘轮审计

日期：2026-07-10

## 目标

将“前端经 `control.SessionAPI` 驱动 runtime”从文档愿景变成可持续收敛的 CI 约束，同时不做高风险的一次性重构。

## 审计事实

- `internal/control/port.go` 已定义分区接口和完整 `SessionAPI`。
- `internal/eventwire` 已被 Desktop 与 Serve 共用，并检查所有 `event.Kind` 都有 wire 名称。
- Desktop、CLI、Serve、Bot 和 ACP 的生产代码仍有 24 个文件直接 import `internal/agent`、`internal/provider` 或 `internal/tool`，共 41 条 import 边；用途包含会话/历史 DTO、迁移、渲染数据和装配注册。
- 因此 `docs/ARCHITECTURE.md` 原先把“禁止直接 import”写成已经满足的铁律并不准确。

## 本批实现

新增 `internal/control/transport_boundary_test.go`：

1. 使用 Go parser 扫描 Desktop、CLI、Serve、Bot、Bot runtime 和 ACP 的生产 `.go` 文件。
2. 将现有 41 条 runtime 直连记录为精确到“文件 + import path”的迁移基线。
3. 新增任意 `agent/provider/tool` 直连都会失败，并要求改经 `control` / `eventwire`。
4. 删除历史直连后也会失败，提示同步缩小 allowlist，避免基线永久膨胀。
5. 跳过测试、前端和生成目录，防止测试桩或 Wails 产物污染架构判断。

这项守卫只冻结依赖面，不宣称 M2 已完成。后续按提交、取消、审批、会话与状态纵向迁移，每移除一条直连就同步收缩 allowlist。

完整门禁还发现 Windows `network=false` sandbox 测试依赖 PowerShell 在 AppContainer 中创建 .NET socket；Constrained Language 会在发起连接前拒绝类型创建，并被旧断言误报为网络连通。本批将探针改为复用当前 Go 测试二进制执行 `net.DialTimeout`，直接验证 sandbox 网络能力，不修改生产 sandbox 实现。

## 验证

```powershell
go test ./internal/control -run TestTransportRuntimeImportRatchet -count=1
go test ./internal/eventwire -count=1
go test ./internal/winsandbox -run TestWindowsSandboxNetworkDisabledBlocksLoopbackConnect -count=1
python scripts/check_docs_contracts.py
```
