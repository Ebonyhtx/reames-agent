# Goal 模式执行与恢复

Reames Agent 的 `/goal` 用宿主状态机持续推进长任务。模型负责提出进展和状态标记，宿主负责预算、证据门控、持久化和失败恢复；重复声明完成不能替代证据。

## 完成语义

| 机制 | 默认模式 | `--strict` |
|---|---|---|
| Canonical Todo 门控 | 必须全部完成 | 必须全部完成 |
| Project checks 门控 | 必须通过 | 必须通过 |
| 重复 `[goal:complete]` | 继续拦截，不可覆盖 | 继续拦截，不可覆盖 |
| 最终质量自检 | 不额外执行 | 门控通过后执行一个宿主发起的自检 turn |

模型应在最后一行使用以下标记：

```text
[goal:continue]
[goal:complete]
[goal:blocked:<short reason>]
```

相同 blocker 连续出现三次后，Goal 才进入 `blocked`。不同 blocker 会重新计数；重新启动 Goal 也会开启新的 blocked audit。Goal 最多自动续接 50 个 turn，连续两个没有工具活动的 turn 会收到推进或说明阻塞的提醒。

## 证据门控

`Agent.GoalReadinessFailure()` 汇总两类完成条件：

- Canonical Todo：`todo_write` 建立的当前任务列表，以及成功 `complete_step` 推进后的状态。
- Project checks：从项目约束解析的验证命令，以及本 turn 或经恢复复验的最新 writer epoch 证据。

任一条件未满足时，`[goal:complete]` 只会产生 continuation intercept。宿主不会生成伪造的 `todo_write`，也不会把未完成项强制改成 `completed`。

完整 evidence ledger 仍是单 turn、内存态收据。Board 可以显示当前收据数、写入/命令活动和触及路径，但这些摘要不会进入 provider prompt，也不是跨进程 durable proof。跨 continuation 持久化两类最小状态：配置项目检查命令的 SHA-256 + root bash tool-call ID，以及已折叠 child effect journal 的 `{ref,journalID,sequence}` cursor。writer 或 mutation attempt 开启新 epoch 并清空旧引用；同一检查的最新失败覆盖旧成功。恢复时只有 sidecar 的 transcript count/digest 精确匹配，并且 root ID 能解析到最新 writer 之后该检查最新一次成功 tool result，引用才生效。命令正文、工具输出、模型文本、任意手工证据和 child-only receipt 不进入 root project-check 引用。

## 恢复语义

每个持久化会话都有 v2 runtime sidecar，和 transcript 分开保存：

```text
Goal 状态与 strict 标志
continuation / blocker / intercept / idle / self-check 计数
PlanMode
Canonical Todo
transcript message count
transcript digest（忽略可刷新 leading system prompt）
最新 writer epoch 的项目检查哈希/root tool-call 引用
已确认 child effect journal cursor
monotonic revision
```

状态在每个完整 turn 边界和每次 Goal transition 后用原子替换写入。恢复时：

- 新写入的 v2 Todo 在 sidecar 的 message count + transcript digest 与加载后的 transcript 锚点完全相等时直接恢复；append-only extension 以 sidecar Todo 为基础重放后缀的成功 `todo_write` / `complete_step`，即使 compaction 已移除基础 `todo_write` 也不会丢失任务；rewrite/divergence 保留 transcript 重建结果，不能让压缩前较大的旧 message count 冒充更新。
- compaction、manual summarize、rewind、cancel/interrupted-turn cleanup 等 history rewrite 成功落盘后会刷新 runtime 锚点，因此同一压缩边界仍能从 sidecar 恢复 canonical Todo。没有 digest 的旧 v2 sidecar继续使用 message count 兼容回退。
- Resume、Switch、Branch、Fork 和 conversation rewind 使用 replace-style 恢复，目标会话没有 sidecar 时清空旧 Goal/Plan，不把来源分支状态叠加过去。
- Checkpoint 在可见 user turn 开始前记录 transcript 边界、prefix digest 和完整 runtime projection；conversation rewind/Fork 对缺少 digest 的 legacy checkpoint、同长度 divergence、负边界以及非空无效/future runtime 均在修改前失败。
- 每个 synthetic Goal/Plan continuation 也分配独立但不在 UI 暴露的 checkpoint。In-flight marker 绑定精确 checkpoint turn；成功时 transcript/runtime 落盘后写 commit anchor 再清 marker，重启只有在两者同时匹配时保留 workspace，否则恢复 workspace/runtime 并截断 partial transcript。
- Conversation/RewindBoth 通过 branch sidecar 的 `prepared -> resources_applied -> checkpoint retirement -> clear` 日志 roll-forward；启动恢复、新 turn 和保留内容的 session rotation 前都会完成遗留阶段，checkpoint 不会在 prepared 恢复仍需文件快照时提前退休。
- strict Goal 只有实际执行宿主发起的 self-check turn 后才能完成。崩溃恢复出的“待自检”状态不能被普通 completion 跳过。
- durable evidence 不使用 Todo 的 append-only 恢复规则：只有 transcript anchor 完全相等才读取；append、rewrite、divergence、引用丢失或最新检查失败均清空/拒绝引用并要求重新验证。

旧 v0/v1 sidecar 仍可读取。旧格式没有完整 revision、PlanMode、Todo 新鲜度和 continuation 字段，因此只提供兼容恢复，不获得 v2 的完整保证。

## Checkpoint 文件恢复

代码回退在写入前预检全部目标：拒绝工作区逃逸、symlink/reparse 路径和非普通文件，保存当前 bytes 与 mode；路径别名、Windows 大小写和现存硬链接共享 earliest snapshot bytes。预检、删除、写入与失败回滚复用单一 workspace `os.Root`，previewable built-in writer 同样以 rooted target 执行 read/stat/temp-write/fsync/chmod/rename/remove，组件被替换成 symlink/reparse point 时不会把写入重定向到 root 外。任何中途失败都会按反序恢复已应用文件和可能部分写入的当前失败文件。Checkpoint JSON 与 truncate manifest 使用 `AtomicWriteFile`，物理删除失败后 tombstone 仍阻止未来 checkpoint 在重启时复活，turn ID 不复用。`AtomicWriteFile` 现以 sibling temp + fsync + atomic replace + directory/write-through flush 发布，cross-device/filter-driver rename 直接 fail closed，不再原地复制；Rewind 日志负责跨 transcript/runtime/checkpoint/workspace 的崩溃收敛。

Checkpoint 只覆盖可预览 writer tool 的文件变化，不承诺追踪任意 shell 副作用。Checkpoint/runtime/in-flight marker 持久化失败已经向可预览写工具 fail-closed 传播；`bash` 等 opaque side effect 仍没有逐文件或 exactly-once 保证。

## Plan 与子任务

PlanMode 是 runtime projection 的一部分；批准计划后只在当前执行 turn 临时自动批准对应 writer，后续 turn 恢复常规审批。`task`、`parallel_tasks` 与 subagent skill 已共享整棵树的 concurrency/token/time/step/cancellation 账本；writable child 的结构化 read/write/command receipt、父 tool-call provenance 和可预览 pre-edit checkpoint 也会归并到发起 turn，partial failure/cancel 保留 mutation boundary。Evidence generation 与 turn-scoped checkpoint callback 会拒绝迟到后台效果污染新 turn。顶层持久 `task` 与 writer-capable `run_skill` 另写有界版本化 effects sidecar；previewable child writer 在执行前必须持久化 mutation intent，恢复只接纳匹配 parent session/workspace/delegation call/journal cursor 的新事件，重复事件不会再次清空后续 root checks。Child-only bash 仍只满足当前 turn，跨 continuation 必须由 root 重跑项目检查。持久后台 Task/skill 在 Provider/tool/compaction 边界保存 transcript，冷启动显式恢复且不自动重放副作用。M4 已在这些明确边界内关闭；opaque side effect 仍要求读取真实状态，不得自动重放或声称 exactly-once。

## 相关实现

- `internal/control/goal.go`：Goal FSM、v2 runtime projection 与恢复
- `internal/control/turn_orchestrator.go`：turn 边界、continuation 和 strict self-check
- `internal/control/controller.go`：Resume/Branch/Fork/Switch/Rewind 生命周期
- `internal/checkpoint/checkpoint.go`：文件快照、预检、事务式恢复
- `internal/fileutil/rootwrite.go`、`internal/tool/builtin/rooted_target.go`：handle-relative rooted writer I/O
- `internal/evidence/evidence.go`：当前 turn 的宿主证据账本与项目检查哈希引用
- `internal/agent/parallel_tasks.go`：并行子任务调度
- `internal/agent/delegation_budget.go`、`subagent_effects.go`、`subagent_effect_journal.go`：整树资源账本、prompt 不可见 effects 归并与 durable cursor
