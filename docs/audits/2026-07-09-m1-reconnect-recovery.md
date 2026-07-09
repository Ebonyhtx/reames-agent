# M1 重连与恢复审计

日期：2026-07-09

## 结论

本批次补强的是“前端重载/重连后仍能继续处理待审批提示”的自动化证据，而不是声称进程崩溃后可以复活同一个阻塞中的工具调用。

当前已证明：

- Controller 的 `ReplayPendingPrompts` 会重新发出阻塞中的 `ApprovalRequest`，并保留原始 `FileDiff`。
- Desktop 的 `App.ReplayPendingPrompts` 会通过 tab event sink 重新发出带 `tabId` 的审批事件，前端可重新构建正确 tab 的审批弹窗。
- `pending_prompts.json` 诊断快照会记录审批的 `FileDiff`，用于崩溃/关闭后的排障与恢复提示；当审批被处理后快照会清理。
- Desktop 后端可以从上次保存的 `desktop-tabs.json` 恢复 classic 多 tab 布局：active tab、project workspace、pinned session path 和历史消息都能在新 `App` 启动后恢复到可见状态。

边界仍然明确：如果整个进程退出，原来的阻塞 goroutine 已经消失，不能简单“批准旧工具调用”。进程级恢复应走会话/历史恢复、重新提交或安全重放机制，而不是伪造旧审批继续执行。

## 本批改动

- `PendingPromptSnapshot` 增加 `file_diff`，让磁盘诊断快照包含审批前补丁预览。
- 新增 `TestReplayPendingPromptsPreservesApprovalFileDiff`，用真实 `write_file` builtin 的预览能力验证 replay 后 diff 不丢失。
- 新增 `TestPendingSnapshotPersistsApprovalFileDiff`，验证 pending snapshot 记录并在审批处理后清理 diff 诊断。
- 新增 `TestDesktopReplayPendingPromptsReEmitsApprovalWithTabAndDiff`，用真实 `agent.Agent + write_file builtin + mock provider` 验证 Desktop replay 事件携带 `tabId` 和完整 diff。
- 新增 `TestRestoreOrBuildTabsRestoresSavedProjectSessionWorkspaceAndActiveTab`，模拟上次退出保存 Global + Project tabs 后，新 `App.restoreOrBuildTabs()` 恢复顺序、active tab、workspace root、session path 和 history。

## 验证

```powershell
go test ./internal/control -run "TestReplayPendingPromptsPreservesApprovalFileDiff|TestPendingSnapshotPersistsApprovalFileDiff" -count=1 -timeout 120s
Push-Location desktop; go test . -run TestDesktopReplayPendingPromptsReEmitsApprovalWithTabAndDiff -count=1 -timeout 180s; Pop-Location
Push-Location desktop; go test . -run TestRestoreOrBuildTabsRestoresSavedProjectSessionWorkspaceAndActiveTab -count=1 -timeout 180s; Pop-Location
```

结果：通过。

## 仍需后续验证

- 原生 Wails 启动后真实点击/发送/停止/恢复烟测。
- 原生 Wails 关闭并重新打开应用后，做一次真实窗口级 session/topic/workspace 恢复烟测。
- 设计进程级“中断后安全重跑/重放”策略，不能把 in-memory pending approval 误写成可跨进程继续执行能力。
