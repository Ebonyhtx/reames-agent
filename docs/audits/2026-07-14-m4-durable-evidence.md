# M4 跨 continuation 最小证据审计

日期：2026-07-14

状态：本地实现、聚焦回归、完整本地门禁与六目标交叉编译完成；当前批远端 CI/CodeQL 待本大批集中 push 后补证据。本文只关闭项目检查最小引用，不声明完整 receipt journal、writer fail-closed 或 M4 完成。

## 问题

原 evidence ledger 会在每个 `Agent.Run` 开始时清空。Writer 和项目检查若发生在一个 continuation，模型在下一个 continuation 才发出 `[goal:complete]`，宿主既可能忘记未验证 writer，也可能依赖 transcript 扫描偶然找到旧命令。进程重启、compaction 或 transcript rewrite 后，这种间接证据更弱；持久化完整命令、输出或模型文本又会扩大 secret 与 prompt 污染面。

## 参考取舍

| 参考 | 审计版本 | 吸收 | 未吸收 |
|---|---|---|---|
| DeepSeek Reasonix | `0e0cb63c712e` | 保留 Go Agent、session JSONL 与现有 tool receipt 主路径 | 不把 transcript 自身升级为无条件 durable proof |
| Codex CLI | `dc23c7bcc8fd` | host-owned receipt reference、generation 与 fail-closed 恢复思路 | 不复制 Rust app-server state 或事件协议 |
| Reames Lite | `1230f781cf10` | prompt-invisible execution ledger 与最小状态投影思路 | 不迁回 Python runtime，也不保存原始验证输出 |

引用只说明设计来源；完成证据来自本仓库实现与测试。

## 已实现不变量

1. 完整 `Receipt` 仍是当前 turn 内存态；sidecar 只保存 `WritePending`、配置项目检查命令的 SHA-256 和 root bash `ToolCallID`，不保存命令正文、工具输出、模型文本或任意手工声明。
2. 成功 writer 或带 `MutationAttempt` 的失败/取消 writer 开启新验证 epoch，原子清空此前引用。可写 child 归并的 writer receipt 也触发该边界。
3. 同一项目检查按 receipt 顺序采用最新结果：后续失败删除旧成功引用，后续再次成功才恢复。Final readiness 若本 turn 重新运行检查，最新当前结果优先；只有本 turn 未运行时才读取 durable ref。
4. Ledger 使用 `Rotate` 在同一锁内返回旧 receipt、清空并推进 generation；后台 `RecordAtGeneration` 要么进入旧批次，要么因 generation 改变被拒绝，不存在 snapshot/reset 之间的漏收窗口。
5. 新 Goal、ClearGoal 和会话切换清空 durable epoch；重复设置同一个仍在运行的 Goal 保留 continuation 状态。新 Goal 同时丢弃上一普通 turn ledger，不能在下一次 `Run` 中重新吸收无关检查。
6. Runtime sidecar 与 checkpoint projection 深拷贝引用。恢复前必须满足 transcript message count/digest 精确相等；Todo 的 append-only suffix replay 不适用于 evidence。Append、rewrite、divergence 或旧 sidecar 无 anchor 都清空引用。
7. Agent 再从加载后的 transcript 解析 root tool calls/results：引用必须对应配置哈希、最新可见 writer 之后该检查最新一次 bash 调用，且 tool result 成功。引用丢失、ID 不匹配、后续失败或后续 writer 均拒绝。
8. Root/checkpoint/Resume/Switch/Branch/Fork 使用同一 runtime projection。磁盘级测试实际保存 session JSONL 与 sidecar，重新构造 Controller/Agent 后恢复 Goal 与引用，并证明 sidecar 不含项目检查命令正文。

## 回归覆盖

- `internal/evidence/evidence_test.go`：原子 rotate/generation、深拷贝、稳定 SHA-256 以及“最新匹配检查结果”语义。
- `internal/agent/final_readiness_test.go`：跨 continuation 复用、mutation attempt 失效、后续失败覆盖、成功/失败 transcript tool result 复验、后续 writer 拒绝与新 Goal epoch 隔离。
- `internal/control/goal_state_dto_test.go`：sidecar round-trip/深拷贝/命令正文不泄漏、exact-anchor/append 拒绝，以及 session JSONL + sidecar 的新 Controller crash-resume。

当前已执行：

```text
go test ./internal/evidence ./internal/agent ./internal/control -count=1 -timeout 600s
go test ./internal/control -run "Test(DurableEvidenceSurvivesSessionCrashResume|RestoreRuntimeEvidenceRequiresExactTranscriptAnchor|GoalStateV2RoundTrip|GoalStateDurableEvidence)" -count=1
```

## 未关闭边界

- Child-only bash receipt 不存在于 parent transcript，当前 turn 可以满足父 project check，但不能形成 parent durable ref；后续 continuation 必须由 root 重新运行检查。
- Sidecar 是本地单用户运行态，不是签名或防本机篡改的审计日志；最小引用也不覆盖 manual、任意 MCP、网络或 shell 文件副作用。
- Runtime/checkpoint 写入仍是 best-effort，失败尚未向 writer fail closed；`AtomicWriteFile` 的平台 fallback 和 transcript/runtime/workspace 跨资源断电窗口仍存在。
- 后台 Task 自身的 crash-resume、durable child effect journal、compaction 后 child receipt 恢复仍未关闭。
