# M4 可写子代理 Effects 归并审计

日期：2026-07-14

状态：本地实现与完整门禁完成；commit `84167d3` 的 CodeQL 3/3 通过，CI 7/8，唯一失败为本文与 companion audit 漏入总索引。索引修复需由后续 CI 复验。本文只关闭进程内 writable effects 归并子项，不声明 M4 完成。

## 问题

原来 `task` 和可写 subagent skill 可以修改同一 workspace，但父 Agent 只收到一段 tool result。Child 的 read/write/command receipt 不进入父 evidence，父 checkpoint 也不知道 child 即将改哪些文件。因此 child 即使实际写入并验证，父 project checks 仍可能误判为未执行；反过来，失败或取消的 writer 可能已部分落盘，却没有父级 mutation boundary。后台 task 跨 turn 后若继续向动态“当前”内存状态写回，还会把旧任务效果错误归到新 turn。

## 参考取舍

按 `REFERENCE_GOVERNANCE.md` 只吸收机制：

| 参考 | 审计版本 | 吸收 | 未吸收 |
|---|---|---|---|
| DeepSeek Reasonix | `0e0cb63c712e` | 保留 Go Agent/tool/checkpoint 主路径 | 不让 child transcript 或模型正文进入父 prompt |
| Codex CLI | `dc23c7bcc8fd` | 结构化 tool receipt、父调用 provenance 和取消边界思路 | 不复制 Rust app-server state 或协议 |
| Reames Lite | `1230f781cf10` | prompt-invisible runtime effects/ledger 与 parent-child ownership 思路 | 不迁回 Python host，也不把内存 effects 冒充 durable journal |

引用只说明设计来源；完成证据来自本仓库生产路径与回归测试。

## 已实现不变量

1. Root Agent 在调用 child 时建立 prompt-invisible `SubagentEffects`。只携带 ancestor evidence target、发起 generation、turn-scoped pre-edit callback 和直接父 tool-call ID，不携带 child prompt、模型文本、工具输出、transcript、Todo 或 `complete_step` 状态。
2. Child 的结构化 read/write/command receipt 归并到全部祖先 ledger，并标记 `source=subagent`、`parent_tool_call_id` 和 `subagent_depth`。Child Todo 与完成判定保持本地，不能直接替父任务签收步骤。
3. Previewable writer 在真正执行前调用父 checkpoint callback。成功 child write 可由父 checkpoint 恢复；成功验证命令只有出现在该 write 之后才能满足父 project checks。
4. Preview 成功即标记 `mutation_attempt`。工具随后失败或 context 取消时，receipt 保持失败，不能成为成功 proof，但 `LatestWriterBoundaryIndex` 会要求其后重新验证，覆盖“已部分落盘”的不确定窗口。
5. 后台 task 切换到 `jobs.Manager` context 时显式保留 effects bridge。若仍处于发起 turn，其 receipt/checkpoint 正常归并；新 turn 开始后 evidence generation 不匹配会原子拒绝迟到 receipt，turn-scoped checkpoint callback 也不会写入新 checkpoint。
6. `Receipt` 的 args、paths、todos 和 `TodoStep` 在 record/snapshot 边界深拷贝，调用方不能通过共享指针改写历史 evidence。
7. Provider 在 terminal stream error 前已上报的 usage 仍进入 Agent 总量和共享委派 token 预算；compaction error/timeout 同样结算已收到 usage。越过 token 上限后不再执行恢复 round。

## 回归覆盖

- `internal/agent/subagent_effects_test.go`：Root → task → child write/bash → parent evidence/project checks；父 checkpoint 回退 child 创建文件；write-then-error、write-then-cancel 的失败 mutation boundary 与恢复；后台 job context 保留 bridge；旧 turn receipt 被拒绝。
- `internal/evidence/evidence_test.go`：generation 原子拒绝迟到 receipt；`TodoStep` 输入与输出快照深拷贝。
- `internal/checkpoint/checkpoint_test.go`、`internal/control/checkpoint_test.go`：指定 turn 的 snapshot 在新 turn 开始后 fail closed；controller 捕获的 scoped callback 不污染当前 checkpoint。
- `internal/agent/delegation_budget_test.go`：interrupted stream recovery 聚合两次 usage、错误前 token 越界停止恢复、compaction error 仍结算 usage。

当前已执行并通过：

```text
go test ./internal/evidence -count=1
go test ./internal/checkpoint -count=1
go test ./internal/agent -run "TestDelegation|TestAgentUsesShared|TestNestedTask|TestWritableSubagent|TestBackgroundWritable|TestSubagentEffects" -count=1 -v
go test ./internal/control -run "TestCheckpointManagerScopedSnapshotRejectsLaterTurn" -count=1 -v
git diff --check
```

聚焦回归之后，本大批又统一通过：

```text
go build ./...
go vet ./...
go test ./internal/... -count=1 -timeout 600s
go test -race ./internal/agent ./internal/evidence ./internal/checkpoint ./internal/control ./internal/board -count=1 -timeout 900s
desktop: go vet ./...
desktop: go test ./... -count=1 -timeout 600s
desktop/frontend: corepack pnpm test:all
desktop/frontend: corepack pnpm build
python: 111 tests passed, 2 platform skips
Node + builtin tool + docs/public/deploy/release contracts: PASS
six GOOS/GOARCH targets with CGO_ENABLED=0: PASS
scripts/verify-baseline.ps1 -SkipDesktop -SkipFrontendHint: PASS
brand residue: 0
git diff --check: PASS
```

基线脚本实际执行 localhost Provider/Gateway/会话落盘/反馈 smoke；报告和交叉编译二进制只写入系统 TEMP。

远端 commit `84167d37540f0df55aac7c8cce3bb90ddf940e4e` 的 CodeQL run `29289472229` 为 Go、Actions、JavaScript/TypeScript 3/3 通过；CI run `29289472287` 为 7/8 jobs 通过，Core Go、Desktop Go、Desktop frontend、Upstream watch、Deployment、Release 与 Cross-compile 均成功。唯一失败的 `Public readiness and docs` 明确报告 `DOCS_INDEX.md` 未索引本审计与 companion audit；这不否定实现门禁，但在索引修复的新 CI 全绿前不能声称该 commit 达到完整远端基线。

## 未关闭边界

- Effects bridge 和 delegation ledger 都是进程内状态。Evidence 在每个 user turn 重置；迟到后台 receipt 会 fail closed 丢弃，不能跨 turn 自动成为 durable proof。必须从 job artifact/transcript 和磁盘重新读取后再验证。
- Background checkpoint callback 只保护“不会写错到新 turn”，不构成跨 turn/crash 的 durable effect journal；后台 Task 的 crash-resume、attach 与永久状态仍未关闭。
- `bash` 没有可静态确定的文件目标，因此 child 与 root 的 shell 命令都不能自动逐文件 preview/checkpoint。
- Checkpoint/runtime 持久化错误尚未全部向 writer fail closed 传播；handle-relative restore、跨 transcript/runtime/workspace 断电事务、ACL/xattr/硬链接身份仍未关闭。
- Provider usage 只能响应后获得，单响应仍可能越过 token cap；实现只保证收到 usage 后准确记账并取消剩余树。
