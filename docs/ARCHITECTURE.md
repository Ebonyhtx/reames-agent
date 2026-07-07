# Reames Agent — 架构设计文档

> 创建：2026-07-08
> 基座：DeepSeek Reasonix main-v2，Go 1.25，MIT License

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

## 三、界面隔离规则（铁律）

所有界面层（CLI / Desktop / Web / IM）必须通过 `control.Controller` 访问内核：

| 允许 ✅ | 禁止 ❌ |
|---|---|
| `ctrl.Submit(ctx, input)` | 直接 import `internal/agent` |
| `ctrl.Cancel()` | 直接 import `internal/provider` |
| 通过 event.Sink 消费事件 | 直接调用 `Agent.Run()` |

## 四、缓存优先约束

1. System prompt 在会话期间不可变
2. Tool schemas 按稳定顺序导出
3. Compaction/prune 必须 bump rewrite version
4. 动态状态（Goal/Plan/Memory）走 user-turn compose
5. UI 状态、诊断、面板状态不得注入 prompt
