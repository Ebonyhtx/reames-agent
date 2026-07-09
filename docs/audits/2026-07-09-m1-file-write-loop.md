# M1 文件写入闭环审计

日期：2026-07-09

## 结论

M1 的“文件写入任务”核心安全闭环已有自动化证据：模型发起真实内置 `write_file` 后，Controller 会发出审批请求，审批请求和 ToolDispatch 都携带同一份补丁预览；用户批准后文件落盘；随后 `RewindCode` 可通过 checkpoint 删除该轮新建文件。

这不是 M1 整体完成声明。原生 Desktop 的真实模型端到端冒烟、停止/恢复和会话恢复仍需继续推进。

## 本批改动

- `event.Approval` 增加 `FileDiff`，使审批弹窗/重连 replay 不必从另一条 ToolDispatch 事件推断补丁预览。
- `agent.Agent.PreviewFileDiff` 暴露只读预览能力，复用 writer tool 的 `tool.PreviewChange` 合约，不执行工具。
- Controller 在注册审批前计算 `FileDiff`，并把它保存进 pending approval；`ReplayPendingPrompts` 重新发出的 ApprovalRequest 也保留 diff。
- 新增 `TestApprovalRealWriteFilePreviewDiskAndRewindEndToEnd`，使用真实 builtin `write_file` 和 workspace-bound registry 覆盖审批、预览、落盘和回退。
- `eventwire` 将 ApprovalRequest 的 diff/added/removed 写入共享 JSON 合约，Desktop `WireApproval` 同步接收该字段。
- Desktop `ApprovalModal` 在工具审批详情中渲染补丁预览和 +/- 统计，serve 内置 Web 页面也显示同样的审批前 diff。

## 验证

```powershell
go test ./internal/control -run TestApprovalRealWriteFilePreviewDiskAndRewindEndToEnd -count=1 -timeout 240s
go test ./internal/agent ./internal/control ./internal/tool ./internal/tool/builtin -count=1 -timeout 300s
go test ./internal/eventwire ./internal/serve ./internal/control -count=1 -timeout 300s
Push-Location desktop/frontend; corepack pnpm test:all; corepack pnpm build; Pop-Location
Push-Location desktop; go test . -count=1 -timeout 300s; Pop-Location
```

结果：通过。

## 仍需后续验证

- 用真实 API + Desktop 执行一次小型改码任务，串联流式输出、审批、写入、停止/恢复和会话恢复。
