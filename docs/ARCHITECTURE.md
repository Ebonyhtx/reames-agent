# Reames Agent — 架构设计文档

> 创建：2026-07-08
>
> 更新：2026-07-17
>
> 基座：DeepSeek Reasonix main-v2，Go 1.25+，MIT License

## 一、项目结构

```
cmd/reames-agent/              # CLI 入口（Bubble Tea TUI）
cmd/reames-agent-guard/        # credential-free Desktop 启动与恢复入口
cmd/reames-agent-plugin-example/ # MCP 插件示例
internal/
  agent/          # 核心 Agent loop、Session、Compaction、Task 子代理
  control/        # 传输无关 Controller（CLI/serve/Desktop 共享）
  provider/       # LLM Provider 接口 + OpenAI/Anthropic 实现
  tool/           # 工具抽象 + Registry + 内置工具
  serve/          # HTTP/SSE 服务（Web/Cloud 前端）
  bot/            # IM Bot（飞书/QQ/微信）
  cli/            # Bubble Tea TUI 实现
  config/         # TOML 配置加载
  plugin/         # MCP 插件宿主（stdio + HTTP）
  skill/          # Playbook 技能系统
  memory/         # 层级化记忆（默认 AGENTS.md，兼容旧 REASONIX.md）
  memorycompiler/ # Memory v5 执行编译器
  hook/           # 生命周期 Hooks
  sandbox/        # OS 沙箱（macOS/Windows/Linux）
  permission/     # 权限门控
  planmode/       # Plan 模式只读门控
  i18n/           # 国际化
  checkpoint/     # 文件检查点/回退
  autoresearch/   # 自动调研目标追踪
  billing/        # Token 计费
  boot/           # 装配：Config → Controller
  termrender/     # CLI/TUI ANSI 事件渲染与终端宽度
  repair/         # Guard 共享启动账本、配置快照、更新回滚与恢复报告
desktop/          # Wails v2 桌面应用
  app.go          # Go 后端（Wails 绑定）
  frontend/       # React 19 + Vite 8 + Zustand 前端
docs/             # 当前产品、架构、用户、运维和审计文档
deploy/           # Docker/systemd/反向代理等自托管部署资产
```

## 二、核心架构

```
┌──────────────────────────────────────────────┐
│              cmd/reames-agent                 │
│            (thin process entry)               │
└──────────────┬───────────────────────────────┘
               │
    ┌──────────▼──────────┐
    │   internal/cli/      │  Bubble Tea TUI
    │   internal/serve/    │  HTTP/SSE server
    │   desktop/app.go     │  Wails desktop
    └──────────┬──────────┘
               │  全部驱动同一接口
    ┌──────────▼──────────┐
    │ internal/boot/       │  注册与装配
    │ internal/control/    │  传输无关 Controller
    └──────────┬──────────┘
               │
    ┌──────────▼──────────┐
    │  internal/agent/     │  Agent / Coordinator
    │   Agent.Run() loop   │
    └──────┬──────┬────────┘
           │      │
    ┌──────▼──┐ ┌─▼──────────────┐
    │provider/│ │  internal/tool/ │
    └─────────┘ └────────────────┘
```

## 三、界面隔离目标与当前约束

目标边界是所有界面层（CLI / Desktop / Web / IM / ACP）通过 `control.SessionAPI` 驱动运行时，并通过 `event.Sink` / `eventwire` 消费共享事件合同：

| 稳定边界 | 禁止新增的耦合 |
|---|---|
| `control.CommandControl.ExecuteCommand(...)` 与版本化 `Command` / `CommandResult` | 新增直接 import `internal/agent` |
| `control.SessionAPI` 的分区接口 | 新增直接 import `internal/provider` |
| `event.Sink` 与 `internal/eventwire` | 新增直接 import `internal/tool` |

该目标边界已达到关闭门槛。`TestTransportRuntimeImportRatchet` 扫描 Desktop、CLI、Serve、Bot 和 ACP，历史 allowlist 已为空；新增任意 `internal/agent`、`internal/provider` 或 `internal/tool` 生产 import 都会使 CI 失败。Compile-time provider 注册与 review 装配由 `boot` 拥有；CLI resume/rebuild 使用 opaque control handle，历史展示使用 `TranscriptMessage`；终端 ANSI 输出由 `termrender` 拥有。共享 `ErrorInfo`、版本化 command/event DTO、HTTP/WS 兼容适配器和 prompt metadata 隔离继续构成跨入口合同。收官 commit `453a51c` 的 CI `29195337394` 为 8/8、CodeQL `29195337395` 为 3/3。

迁移已经覆盖命令、event/display DTO、会话 persistence/copy/meta、Desktop rebuild/settings/history、Serve title、ACP metadata、CLI composition/review/model discovery 与终端渲染。所有受守卫 transport 生产文件均无 runtime 直连。CLI/Bot/ACP 为拥有 turn 生命周期而保留同步 `RunTurn`，它与异步 command acknowledgement 是不同语义，不强行合并成伪统一接口。

## 四、缓存优先约束

1. System prompt 在会话期间不可变
2. Tool schemas 按稳定顺序导出
3. Compaction/prune 必须 bump rewrite version
4. 动态状态（Goal/Plan/Memory）走 user-turn compose
5. UI 状态、诊断、面板状态不得注入 prompt
6. `MemoryCitations`、`Edited`、`Original` 等本地展示 metadata 在 Provider interface 前剥离，不能依赖具体 Provider 恰好忽略它们
7. IM connection/domain/chat/user/operator/message ID 只用于路由和授权，不进入 prompt；群聊中显式的参与者名称标签属于用户可见语义

## 五、会话运行态与恢复

动态 Goal/Plan/Todo 不进入稳定 system prompt，但必须和 transcript 一起恢复。`control.Controller` 为每个 session 维护 v2 runtime sidecar：Goal FSM、continuation/blocker/intercept/idle/self-check 计数、PlanMode、canonical Todo、transcript message count、忽略可刷新 leading system prompt 的 transcript digest、monotonic revision、最新 writer epoch 的 root project-check 引用和已确认 child effect journal cursor。Sidecar 与 checkpoint JSON 使用 `fileutil.AtomicWriteFile`；同一进程内相同 sidecar 路径的 revision 检查与替换处于同一临界区，跨进程写入依赖 transport 持有的 session lease。该 helper 通过同目录临时文件、文件 fsync、原子替换和目录项持久化发布；Windows 使用 `MoveFileEx(REPLACE_EXISTING|WRITE_THROUGH)`，Unix rename 后 fsync 父目录，cross-device/filter-driver rename 直接 fail closed，不再原地复制。

```text
orchestrated turn start (visible or hidden synthetic)
  ├─ checkpoint: transcript boundary + runtime projection + later file snapshots
  ├─ branch meta: in-flight marker + exact checkpoint turn
  ├─ Agent/Coordinator turn
  │    └─ previewable writer gate: checkpoint + runtime sidecar + in-flight marker
  ├─ persist transcript append/replace and runtime projection
  ├─ publish transcript/runtime commit anchor
  └─ clear in-flight marker
```

恢复采用整体替换而不是字段叠加：Resume/Switch 先让 Agent 从目标 transcript 重建 Todo；带 digest 的 v2 sidecar 在 transcript 锚点完全相等时直接恢复 Todo，append-only extension 以 sidecar Todo 为基础重放后缀 `todo_write`/`complete_step`，rewrite/divergence 则保留 transcript 重建结果，避免 compaction 后较大的旧 message count 冒充新状态。旧 v2 sidecar 才使用 message count 兼容回退。Branch/Fork 复制对应 tip/turn-start runtime；conversation rewind 同时截断 transcript 并恢复 checkpoint runtime。目标没有 sidecar 时，旧 Goal 和 PlanMode 必须清空。

`checkpoint.Store` 只有在 allocation manifest 与 turn/runtime record 均成功后才开放当前 turn；pre-edit record 失败会回滚内存 `seen/Files`，避免重试误判。Agent 的 previewable writer hook 可返回错误，root writer 还会在同一 session handoff 临界区刷新 runtime sidecar，并复验 in-flight marker 来自当前 turn 且匹配 session/message boundary/`preserveUser`，再进入工具执行；child 的祖先 callback 失败同样阻断 writer。每个 visible 或 synthetic turn 都分配 checkpoint，synthetic checkpoint 不进入 rewind picker/boundary map。成功结束先持久化 transcript 和 runtime，再在 branch meta 写两者的 commit anchor，最后清 marker；冷启动只有在 transcript 与 runtime sidecar 同时匹配 anchor 时保留 workspace，否则按 marker 的 checkpoint 恢复 workspace/runtime，并截断 partial transcript。失败和 cancel 在当前进程也走同一回滚；若自动恢复本身失败，Resume、新模型 turn 和所有保留 session 内容的 rotation 操作都会保留 marker 并阻断后续变更。无 session path 的内存 Controller 保持非持久兼容语义。

Previewable built-in writer 先把用户路径绑定为持有中的 `os.Root` 与 root-relative path；read/stat/temp-write/fsync/chmod/rename/remove 都通过该 handle 完成，组件替换成 symlink/reparse point 时不会把操作重定向到 workspace 外。`move_file` 跨 root 时以 rooted copy + source remove 执行，`apply_patch` 在完整 preflight 后逐文件原子替换，并在后续文件失败时按预检 bytes/mode 回滚。Agent 的 `MultiPreviewer` 会在任一写入前快照所有目标。

`checkpoint.Store.RestoreCode` 先构造并验证全部操作，再打开单一 workspace `os.Root` 执行预检、删除、写入和反向回滚；路径别名按规范化 identity 合并，Windows 大小写折叠，现存硬链接使用 `os.SameFile` 共享 earliest bytes。Conversation rewind/Fork/Summarize 必须验证 turn-start transcript prefix digest，Rewind/Fork 对非空无效 runtime 在修改前失败；truncate manifest 让物理 checkpoint 删除失败后也不会在重启时复活，turn ID 保持单调。Conversation/RewindBoth 先在 branch meta 写 `prepared` intent（含 turn、prefix anchor、runtime 与 code scope），再发布 transcript/runtime/workspace，写 `resources_applied` barrier 后才退休 checkpoint 并清 journal。启动恢复和新 turn 前都会重放未完成 phase；即使 checkpoint 已退休但 clear 前断电，journal 中的 transcript/runtime 和 barrier 也足以收敛。该协议提供跨资源 crash recovery，但不声称底层多个文件在同一瞬间原子变化。

持久 subagent 通过 `Agent.Options.SessionSync` 在初始 user message、steer/retry/nudge、assistant tool-call envelope（工具执行前）、tool results、final 和 compaction rewrite 后保存 transcript；writer-capable `run_skill` 与 `task` 使用同一同步边界。`SubagentStore` 先发布 running metadata 再写 transcript，完成态只在 transcript 保存成功后发布；启动清理把遗留 running ref 转为可显式 `continue_from` 的 interrupted ref。`jobs.Manager.StartRecoverableForSession` 只有在 job log 与 running metadata 成功落盘后才启动 goroutine；冷加载 running job 时生成 interrupted tombstone、保留部分 log 并提示续跑，不自动重放工具。

顶层持久 subagent ref 另有 `*.effects.json`：0600、schema v1、最多 256 个 retained event/1 MiB，超限时把被压缩前缀折叠为保守 mutation summary。Previewable child writer 在工具执行前写入 intent；结果 receipt 在祖先 ledger 发布前落盘。Sidecar 只含结构化 read/write/command metadata、parent session/tool-call、workspace、depth 与 sequence，不含 child model text、tool output、args、Todo/step；command 使用 `trust.RedactSecrets`。Runtime sidecar 保存 `{ref,journalID,sequence}` cursor；未确认事件恢复时必须匹配 parent `task`/`run_skill` transcript anchor，已确认事件可在 parent compaction 移除旧调用后幂等跳过。Child mutation 使旧 root verification refs 失效，child-only bash 不会恢复成 root proof。损坏或身份不匹配时每进程首次保守失效并提示 root 重验。

当前完整 evidence ledger、委派预算和实时 effects bridge 仍是 turn/process-scoped；durable 部分覆盖 root 项目检查引用、subagent transcript/job tombstone 和 child effect mutation boundary，不把完整 child receipt 账本升级为父 proof。上述 turn/rewind 事务只覆盖 previewable built-in writer 与会话本地资源；`bash`、MCP、外部 API 和后台 opaque side effect 没有 exactly-once 语义，ACL/xattr/硬链接身份也不恢复。

## 六、进程级 Guard 与 Safe Mode

Desktop 的进程级恢复位于普通 `boot -> control -> agent` 图之外。`cmd/reames-agent-guard` 和 CLI 的
early `guard` dispatch 只依赖 `internal/guardcmd`、`internal/repair`、配置路径/验证与原子插件状态，
不装配 Provider、MCP transport、Hook、Bot、LSP 或 Agent loop。

```text
OS shortcut / bundle entry / .desktop
  -> reames-agent-guard
       -> lock + startup ledger + pending update/config/session/plugin inspection
       -> [proven verified rollback] OR [Safe Mode] OR [normal launch]
       -> reames-agent-desktop
            -> starting -> DOM ready -> 30s healthy -> update commit
```

`internal/repair` 是唯一持久恢复模型：启动账本、五份配置健康快照、repair undo transaction、
pending update 完整安装单元、installer failure marker、current/previous binary 与 session/plugin
inspection 都在该包内。`control.Controller.RecoveryStatus()`、Serve `/api/recovery`、Desktop
`GetRecoveryStatus()`、Guard check 和 Gateway recovery-status 只返回同一 `repair.Report`。

自动二进制 mutation 必须持有 pending-update 跨进程锁，并证明 crash-loop、失败版本与 `toVersion`
一致、目标属于当前安装目录、transaction identity 未变化且全部备份 SHA-256 有效。回滚先 staging
全体文件再 swap；补偿不能证明一致版本时返回 mixed install 并拒绝普通启动。证据不足时只进入
Safe Mode 或要求可信重装。已有 pending 未清算时不允许准备下一次更新；Windows helper 缺失时
自动更新失败，不旁路启动无法归因的 installer。

Safe Mode 通过 built-in recovery defaults 切断用户/项目 TOML 与 dotenv，不恢复旧 tab/session，
Desktop 只建立 recovery-only shell，`boot.Build` 直接拒绝 Provider、Controller、工具 registry 与
普通 Agent 装配；Skill、Hook、plugin host、Bot、LSP、planner/Guardian/subagent/Memory Compiler、
heartbeat、更新/遥测/metrics 等也在各自边界关闭。该设计保持“正常 runtime 只有一套 Controller”，
而不是在 Guard 中复制第二套 Agent。
