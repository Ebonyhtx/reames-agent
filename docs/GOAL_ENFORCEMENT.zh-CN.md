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
- Project checks：从项目约束解析的验证命令及本 turn 的宿主可观察执行证据。

任一条件未满足时，`[goal:complete]` 只会产生 continuation intercept。宿主不会生成伪造的 `todo_write`，也不会把未完成项强制改成 `completed`。

当前 evidence ledger 是单 turn、内存态收据。Board 可以显示当前收据数、写入/命令活动和触及路径，但这些摘要不会进入 provider prompt，也不是跨进程 durable proof。跨 continuation 的最小证据引用仍是 M4 后续项。

## 恢复语义

每个持久化会话都有 v2 runtime sidecar，和 transcript 分开保存：

```text
Goal 状态与 strict 标志
continuation / blocker / intercept / idle / self-check 计数
PlanMode
Canonical Todo
transcript message count
transcript digest（忽略可刷新 leading system prompt）
monotonic revision
```

状态在每个完整 turn 边界和每次 Goal transition 后用原子替换写入。恢复时：

- 新写入的 v2 Todo 在 sidecar 的 message count + transcript digest 与加载后的 transcript 锚点完全相等时直接恢复；append-only extension 以 sidecar Todo 为基础重放后缀的成功 `todo_write` / `complete_step`，即使 compaction 已移除基础 `todo_write` 也不会丢失任务；rewrite/divergence 保留 transcript 重建结果，不能让压缩前较大的旧 message count 冒充更新。
- compaction、manual summarize、rewind、cancel/interrupted-turn cleanup 等 history rewrite 成功落盘后会刷新 runtime 锚点，因此同一压缩边界仍能从 sidecar 恢复 canonical Todo。没有 digest 的旧 v2 sidecar继续使用 message count 兼容回退。
- Resume、Switch、Branch、Fork 和 conversation rewind 使用 replace-style 恢复，目标会话没有 sidecar 时清空旧 Goal/Plan，不把来源分支状态叠加过去。
- Checkpoint 在可见 user turn 开始前记录 transcript 边界、prefix digest 和完整 runtime projection；conversation rewind/Fork 对缺少 digest 的 legacy checkpoint、同长度 divergence、负边界以及非空无效/future runtime 均在修改前失败。
- strict Goal 只有实际执行宿主发起的 self-check turn 后才能完成。崩溃恢复出的“待自检”状态不能被普通 completion 跳过。

旧 v0/v1 sidecar 仍可读取。旧格式没有完整 revision、PlanMode、Todo 新鲜度和 continuation 字段，因此只提供兼容恢复，不获得 v2 的完整保证。

## Checkpoint 文件恢复

代码回退在写入前预检全部目标：拒绝工作区逃逸、symlink/reparse 路径和非普通文件，保存当前 bytes 与 mode；路径别名、Windows 大小写和现存硬链接共享 earliest snapshot bytes。任何中途失败都会按反序恢复已应用文件和可能部分写入的当前失败文件，相对路径权限按 workspace root 捕获。Checkpoint JSON 与 truncate manifest 使用 `AtomicWriteFile`，物理删除失败后 tombstone 仍阻止未来 checkpoint 在重启时复活，turn ID 不复用。该 helper 的 Windows cross-device fallback 可能原地复制；路径复查后仍按路径写入也存在 TOCTOU，不能声明无条件 crash-safe。`RewindBoth` 的 transcript/runtime/workspace 跨资源更新同样没有 durable 事务日志。

Checkpoint 只覆盖可预览 writer tool 的文件变化，不承诺追踪任意 shell 副作用。Checkpoint 持久化失败向写工具 fail-closed 传播仍是 M4 后续项。

## Plan 与子任务

PlanMode 是 runtime projection 的一部分；批准计划后只在当前执行 turn 临时自动批准对应 writer，后续 turn 恢复常规审批。`parallel_tasks` 支持依赖调度和结果聚合，但共享 token/time/step 预算、writable 子代理与父 checkpoint/evidence 的结构化归并、后台 Task crash-resume 尚未达到 M4 完成门槛。

## 相关实现

- `internal/control/goal.go`：Goal FSM、v2 runtime projection 与恢复
- `internal/control/turn_orchestrator.go`：turn 边界、continuation 和 strict self-check
- `internal/control/controller.go`：Resume/Branch/Fork/Switch/Rewind 生命周期
- `internal/checkpoint/checkpoint.go`：文件快照、预检、事务式恢复
- `internal/evidence/evidence.go`：当前 turn 的宿主证据账本
- `internal/agent/parallel_tasks.go`：并行子任务调度
