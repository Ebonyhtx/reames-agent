# Reames Agent — 架构设计文档

> 创建：2026-07-08
>
> 更新：2026-07-11
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
│  (blank imports → providers, tools → cli)     │
└──────────────┬───────────────────────────────┘
               │
    ┌──────────▼──────────┐
    │   internal/cli/      │  Bubble Tea TUI
    │   internal/serve/    │  HTTP/SSE server
    │   desktop/app.go     │  Wails desktop
    └──────────┬──────────┘
               │  全部驱动同一接口
    ┌──────────▼──────────┐
    │ internal/control/    │  传输无关 Controller
    │   Controller         │  (SessionAPI 接口)
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

当前代码仍有历史直连，主要用于会话持久化、装配注册和设置重建，因此这是一项目标架构而不是已经完全满足的事实。`TestTransportRuntimeImportRatchet` 用精确 allowlist 冻结 Desktop、CLI、Serve、Bot 和 ACP 的现有直连：新增依赖会使 CI 失败，迁移删除依赖后也必须同步收缩 allowlist。Provider 与内置工具的 blank import 只允许保留在明确的装配入口。当前已完成四条可验证的纵向路径：共享 `ErrorInfo` 由 Desktop 按 code/category 消费；CLI `/resume` 通过 `control.SessionInfo`、`control.ListSessions` 和事务式 `ResumeSessionPath` 工作；提交/取消/审批/状态通过版本化 `control.Command` / `CommandResult` 与服务端选择的 `CommandScope` 驱动；事件通过 `eventwire` v1 输出完整 source/cache payload，历史展示通过 `control.TranscriptMessage` 投影。Serve history 与 ACP replay 不再消费 `provider.Message`，不会把 system、合成恢复指令、compose 控制块或 referenced-context payload当作用户历史发给远端前端。Serve 的新 `/command` 与 WebSocket `method=command` 共用相同执行器，旧端点/WS method 只是兼容适配器。

迁移按纵向路径进行：命令、event/display DTO、主要会话 persistence 路径、Desktop system-prompt-aware rebuild、settings provider-kind view、展示安全 memory suggestions、Serve title provider、ACP metadata title 与 Desktop history/pagination/planner replay 已经收口；Serve/Bot/ACP service 已无 runtime 直连，Desktop app/tabs 已无 `provider` 直连。下一步迁移 Desktop app/tab 剩余 session-store 类型与 CLI/ACP composition root/专用渲染边界。CLI/Bot/ACP 为拥有 turn 生命周期而保留同步 `RunTurn`，它与异步 command acknowledgement 是不同语义，不强行合并成伪统一接口。

## 四、缓存优先约束

1. System prompt 在会话期间不可变
2. Tool schemas 按稳定顺序导出
3. Compaction/prune 必须 bump rewrite version
4. 动态状态（Goal/Plan/Memory）走 user-turn compose
5. UI 状态、诊断、面板状态不得注入 prompt
6. `MemoryCitations`、`Edited`、`Original` 等本地展示 metadata 在 Provider interface 前剥离，不能依赖具体 Provider 恰好忽略它们
7. IM connection/domain/chat/user/operator/message ID 只用于路由和授权，不进入 prompt；群聊中显式的参与者名称标签属于用户可见语义
