# M1 失败场景合同审计

日期：2026-07-09

## 结论

本批次补强 M1 的失败路径自动化：失败必须可见、运行态必须复位、待处理提示不能残留，且安全失败不能偷偷落盘。

新增证据覆盖两类用户会遇到的高频场景：

- Provider 鉴权失败：以 `TurnDone.Err` 发给前端，并包含可操作的 key 环境变量提示；Controller 运行态归零。
- Provider 429/503：以带 HTTP 状态的可操作 `TurnDone.Err` 发给前端；Controller 运行态归零。
- Provider 流中断恢复耗尽：保留部分响应上下文，重试耗尽后以“stream interrupted/continue”行动提示发给前端；Controller 运行态归零。
- 写文件审批超时：`write_file` 不落盘，pending approval 被清理，阻塞原因作为 `ToolResult.Err` 反馈给模型，随后 turn 可正常结束。
- 工具运行超时：真实 `bash` 工具在 workspace timeout 下停止执行，超时原因作为 `ToolResult.Err` 反馈给模型，随后 turn 正常结束且 Controller 运行态归零。

## 本批改动

- 新增 `TestProviderAuthErrorEmitsTurnDoneAndClearsRuntimeStatus`。
- 新增 `TestProviderAPIErrorEmitsActionableTurnDoneAndClearsRuntimeStatus`。
- 新增 `TestProviderStreamInterruptionExhaustionEmitsTurnDoneAndClearsRuntimeStatus`。
- 新增 `TestApprovalTimeoutBlocksWriteAndClearsPendingPrompt`。
- 新增 `TestToolTimeoutEmitsToolResultAndClearsRuntimeStatus`。

## 语义边界

- Provider 鉴权、网络等无法继续的上游错误属于 turn 级失败，应体现在 `TurnDone.Err`。
- 审批拒绝/超时属于工具调用被阻塞，应优先作为 tool result 反馈给模型，让模型解释、改方案或请求用户重新授权；除非后续模型也失败，否则不应把整个 turn 直接标红。
- “没有落盘、没有残留 pending、运行态归零”是失败路径的最低验收线。

## 验证

```powershell
go test ./internal/control -run "TestProvider(AuthError|APIError|StreamInterruption)|TestApprovalTimeoutBlocksWriteAndClearsPendingPrompt|TestToolTimeoutEmitsToolResultAndClearsRuntimeStatus" -count=1 -timeout 120s
```

结果：通过。

## 仍需后续覆盖

- Desktop 原生 UI 上的错误 toast/banner 与停止按钮状态。
- 真实 provider 的 429/5xx/断流组合烟测（当前为 Controller 级模拟 provider 合同）。
- 工具运行超时在 Desktop 原生 UI 上的展示合同。
