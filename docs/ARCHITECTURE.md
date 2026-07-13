# Reames Agent — 架构设计文档

> 创建：2026-07-08
>
> 更新：2026-07-14
>
> 基座：DeepSeek Reasonix main-v2，Go 1.25+，MIT License

## 一、项目结构

```
cmd/reames-agent/              # CLI 入口（Bubble Tea TUI）
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
  memory/         # 层级化记忆（REAMES_AGENT.md 等）
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
desktop/          # Wails v2 桌面应用
  app.go          # Go 后端（Wails 绑定）
  frontend/       # React 19 + Vite 8 + Zustand 前端
site/             # Astro 文档站点
workers/          # Cloudflare Workers（accounts, crash-report, forum）
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

动态 Goal/Plan/Todo 不进入稳定 system prompt，但必须和 transcript 一起恢复。`control.Controller` 为每个 session 维护 v2 runtime sidecar：Goal FSM、continuation/blocker/intercept/idle/self-check 计数、PlanMode、canonical Todo、transcript message count、忽略可刷新 leading system prompt 的 transcript digest 和 monotonic revision。Sidecar 与 checkpoint JSON 使用 `fileutil.AtomicWriteFile`；同一进程内相同 sidecar 路径的 revision 检查与替换处于同一临界区，跨进程写入依赖 transport 持有的 session lease。该 helper 在 Windows cross-device/filter-driver rename 失败时会降级为原地复制，因此不能无条件外推为断电 crash-safe。

```text
visible user turn start
  ├─ checkpoint: transcript boundary + runtime projection + later file snapshots
  ├─ Agent/Coordinator turn
  │    └─ previewable writer gate: checkpoint + runtime sidecar + in-flight marker
  ├─ turn-boundary runtime sidecar refresh
  └─ Goal advance: evidence gate + counters + second runtime refresh
```

恢复采用整体替换而不是字段叠加：Resume/Switch 先让 Agent 从目标 transcript 重建 Todo；带 digest 的 v2 sidecar 在 transcript 锚点完全相等时直接恢复 Todo，append-only extension 以 sidecar Todo 为基础重放后缀 `todo_write`/`complete_step`，rewrite/divergence 则保留 transcript 重建结果，避免 compaction 后较大的旧 message count 冒充新状态。旧 v2 sidecar 才使用 message count 兼容回退。Branch/Fork 复制对应 tip/turn-start runtime；conversation rewind 同时截断 transcript 并恢复 checkpoint runtime。目标没有 sidecar 时，旧 Goal 和 PlanMode 必须清空。

`checkpoint.Store` 只有在 allocation manifest 与 turn/runtime record 均成功后才开放当前 turn；pre-edit record 失败会回滚内存 `seen/Files`，避免重试误判。Agent 的 previewable writer hook 可返回错误，root writer 还会在同一 session handoff 临界区刷新 runtime sidecar，并复验 in-flight marker 来自当前 turn 且匹配 session/message boundary/`preserveUser`，再进入工具执行；child 的祖先 callback 失败同样阻断 writer。持久快照可在新进程加载后恢复 writer 的部分落盘，但这不是 transcript/runtime/workspace 的单一断电事务，`bash` 等无静态目标工具也没有逐文件 checkpoint。

`checkpoint.Store.RestoreCode` 先构造并验证全部操作，再执行写入；路径必须留在已解析 workspace 内，不能穿过 symlink/reparse point，且只能覆盖普通文件。路径别名按规范化 identity 合并，Windows 大小写折叠，现存硬链接使用 `os.SameFile` 共享 earliest bytes。失败时以预检保存的 bytes/mode 反向回滚已成功操作和可能部分写入的当前失败目标；相对路径的 mode 也按 workspace root 捕获。Conversation rewind/Fork/Summarize 必须验证 turn-start transcript prefix digest，Rewind/Fork 对非空无效 runtime 在修改前失败；truncate manifest 让物理 checkpoint 删除失败后也不会在重启时复活，turn ID 保持单调。路径预检到按路径写入之间仍有 TOCTOU，完整关闭需要 handle-relative no-reparse/resolve-beneath 写入；transcript/runtime/workspace 也不构成单一断电事务。

当前 evidence ledger 是 turn-scoped 内存状态，只用于 `complete_step`、Goal readiness 和 Board 投影，不是跨进程证明。Durable evidence、共享子代理预算和 writable 子代理归并仍属于 M4 后续边界。
