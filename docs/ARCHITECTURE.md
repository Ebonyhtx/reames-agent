# Reames Agent — 架构设计文档

> 创建：2026-07-08
>
> 更新：2026-07-12
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
