# M4 子代理共享委派预算审计

日期：2026-07-14

状态：本地实现与完整门禁完成；本批远端 CI/CodeQL 待集中 push 后补证据。本文只关闭共享预算子项，不声明 M4 完成。

## 问题

原 `parallel_tasks` 会一次启动全部 goroutine，每个 child 只有自己的 `max_steps`；`task`、`read_only_task`、嵌套 skill 和后台 task 之间没有共享的并发、token、time、step 或 cancellation 账本。简单地给整个 subagent 生命周期加 semaphore 会让父代理占槽等待嵌套 child，形成递归死锁；把后台 task 绑定到发起 turn 又会在父 turn 结束时错误取消本应跨 turn 运行的 job。

## 参考取舍

按 `REFERENCE_GOVERNANCE.md` 只吸收机制：

| 参考 | 审计版本 | 吸收 | 未吸收 |
|---|---|---|---|
| DeepSeek Reasonix | `0e0cb63c712e` | 保留现有 Go Agent/tool/Jobs 主路径 | 不回退到无共享预算的原始 fanout |
| Codex CLI | `dc23c7bcc8fd` | root/child cancellation cause 与分层传播思路 | 不复制 Rust runtime 或 app-server 协议 |
| Reames Lite | `1230f781cf10` | 默认并发 3、bounded scheduler 和 prompt-invisible runtime ledger 思路 | 不迁回 Python `DelegateManager`，也不沿用“整个 child 生命周期持有 semaphore”的嵌套死锁风险 |

引用只说明设计来源；完成证据来自本仓库测试。

## 已实现不变量

1. `task`、`read_only_task`、`parallel_tasks`、只读/可写 subagent skill 和独立 review 入口都通过 `DelegationLedger`；嵌套入口从 tool-call context 继承同一指针。
2. `MaxConcurrent` 只覆盖一次活跃 Provider stream。模型返回 tool call 后立即释放槽，父 subagent 等待嵌套 child 时不占槽；默认并发为 3。
3. aggregate step 在取得并发槽后原子预留。等待槽时被取消不会消费 step；默认每棵树 100 个 Provider round，compaction Provider round 也计入。
4. Provider usage 按树原子累计。已达到上限时拒绝下一 round；单响应越过上限时记录真实用量并取消整棵树，同时保留已收到的 assistant receipt。Terminal stream error/interrupt 或 compaction error 前已收到的 usage 也会结算，越界时不会发起恢复 round。Provider 无法预报响应 token，因此不声称零超量。
5. time budget 从树根创建时开始。前台树派生自 parent turn；后台 `task` 在 `jobs.Manager` 的 session/job context 内新建树，因此跨 turn 存活，`kill_shell`/session teardown/manager close 仍能取消它。
6. 父/root 取消传播到全部 descendants；独立 child context 的局部取消不写入 root cause，也不毒化无关 sibling。step/token/time 超限使用稳定 sentinel cause，后台失败产物包含实际原因。
7. 用户级配置为 `subagent_max_concurrency`、`subagent_max_steps`、`subagent_max_tokens`、`subagent_max_duration_seconds`。项目 `reames-agent.toml` 不能覆盖这些资源边界。
8. ledger snapshot 只包含计数、上限、deadline 与 cause，不包含 prompt、模型文本、工具输出、transcript 或 secret，也不注入 Provider prompt。

## 回归覆盖

- `delegation_budget_test.go`：并发峰值、排队取消不耗 step、原子 step 上限、token 越界整树取消、deadline、父取消、局部 child 取消、Agent step/token 接线、嵌套共享与并发 1 无死锁，以及 interrupted/error/compaction error 前 usage 不漏账。
- `parallel_tasks_test.go`：5 个 sibling 在并发 2 下排队，Provider 实测峰值严格为 2。
- `task_test.go`：后台 task 使用 job-scoped deadline，失败 metadata 与可读错误原因均持久化。
- `config` 测试：TOML round-trip、user/project scope 隔离，以及恶意项目级放宽被恢复为用户/内置上限。

本批当前已执行：

```text
go test ./internal/agent -run "TestDelegation|TestAgentUsesShared|TestNestedTask|TestParallelTasksShares|TestTaskToolBackground" -count=1 -v
go test ./internal/config -run "TestRenderTOMLRoundTrips$|TestScopedRenderSeparatesUserAndProjectConfig|TestLoadForRootKeepsGlobalAgentStepLimitsOverProject|TestLoadForRootIgnoresProjectAgentStepLimitsWithoutUserConfig" -count=1 -v
go test ./internal/agent ./internal/config ./internal/boot -count=1
```

聚焦回归之后，本大批又统一通过 `go build ./...`、`go vet ./...`、`go test ./internal/...`、Agent/Control/Evidence/Checkpoint/Board race、Desktop `go vet ./...` 与 `go test ./...`、前端 `pnpm test:all` 与 production build、111 个 Python 测试（2 skipped）、Node/工具/docs/public/deploy/release 合同、六目标 `CGO_ENABLED=0` 交叉编译和 `verify-baseline.ps1 -SkipDesktop -SkipFrontendHint`。基线脚本实际完成 localhost Gateway smoke，报告与六平台二进制均写入系统 TEMP；品牌残留计数为 0，`git diff --check` 通过。远端证据仍只在本批单次集中 push 后成立。

## 未关闭边界

- ledger 是进程内运行态，不做后台 task crash-resume；进程崩溃后的预算剩余量不会恢复。
- writable child 的进程内 effects 归并已由 `2026-07-14-m4-writable-subagent-effects.md` 关闭；它和预算 ledger 都不是 crash-resume 状态，迟到后台 receipt 会 fail closed 丢弃而非升级为 durable proof。
- durable evidence、writer persistence fail-closed、后台 Task crash-resume、handle-relative 恢复与跨资源断电事务仍按 M4 后续顺序推进。
