# Reames Agent 权威接手文档 v2

> 状态：当前项目唯一权威接手入口
> 更新时间：2026-07-09
> 当前主仓库：`F:\reames-agent` / `https://github.com/Ebonyhtx/reames-agent.git` / `main`
> 一句话结论：**Reames Agent 是以 Reasonix 最新源码为底座改造出来的独立新项目，不是继续在旧 Reames Lite 上补桌面壳。**

## 0. 本文档解决什么问题

本项目已经经历过多轮方向调整：旧 Reames Lite、手写桌面 UI、Reasonix 复刻、Apple 风格、云部署、IM 网关、参考项目融合等信息混在一起，容易让接手者误判方向。

从现在开始：

- 新主项目是 `F:\reames-agent`；
- 旧 `F:\Reames-Lite` 只作为 legacy/contract/reference；
- Reasonix 是源码底座，不只是 UI 参考；
- 桌面端优先目标是真正的桌面 Agent，而不是 CLI 可视化面板；
- 文档入口只看本文和 `docs/REAMES_AGENT_EXECUTION_PLAN.md`。

## 1. 最终产品目标

把 Reames Agent 做成一个 **Reasonix 源码底座 + Reames 公共边界与缓存约束 + Apple-light 中文桌面体验 + 可本地/云端部署 + 可接多渠道网关** 的多平台 Agent。

目标形态：

```text
Reames Agent
├─ Desktop：主产品；像 Reasonix/Codex 桌面 Agent，Apple-light，中文优先，可交互、可设置、可查看、可审批、可控制
├─ CLI：保留本地编程 Agent 能力，但必须走同一 runtime/control boundary
├─ Server/Web：可本机、局域网、云服务器部署；提供 HTTP/SSE/API/Web UI
└─ Gateway：Hermes-style 多渠道入口；消息渠道 metadata 不得污染 provider prompt
```

## 2. 当前仓库角色

### 2.1 `F:\reames-agent`：新主项目

- 远端：`https://github.com/Ebonyhtx/reames-agent.git`
- 分支：`main`
- 当前角色：唯一主线开发仓库。
- 源码基座：Reasonix-derived Go/Wails/React runtime。
- 当前状态：已经接管远端旧仓库并建立初始 baseline；桌面关键 Go 测试可跑；前端依赖尚未安装，UI build 仍需单独恢复验证。

### 2.2 `F:\code-reference\DeepSeek-Reasonix`：主底座参考

- 角色：源码真相和设计基准。
- 重点保留：
  - Go runtime / controller / agent loop；
  - Wails desktop bridge；
  - React/Vite desktop UI skeleton；
  - settings、permission、workspace、memory、MCP/plugin、server/auth；
  - DeepSeek/OpenAI-compatible cache-hit/prefix-cache 机制。

Reasonix 不是简单“借鉴界面”，而是新项目的工程底盘。后续改造应先确认 Reasonix 原机制，再做 Reames 化，不要重写已有成熟能力。

### 2.3 `F:\Reames-Lite`：旧项目参考

旧 Reames Lite 不再作为主线仓库。它贡献的是思想和契约，不是桌面 UI 底座：

| 旧资产 | 保留价值 | 新项目处理方式 |
|---|---|---|
| ReamesClient 公共边界思想 | UI/CLI/Web/Gateway 不能直接穿透 runtime 内部 | 在 Go 项目里建立 Reames public boundary / boundary tests |
| cache-first 测试思想 | 防止新增 UI/metadata 污染 provider-visible prefix | 保留为测试约束，不替换 Reasonix cache pipeline |
| provider-visible metadata 隔离 | 渠道、布局、设置、调试状态不能进 prompt | 加 guard/test |
| 中文产品体验 | 默认中文、用户语言、少工程自嗨 | 用于桌面 copy pass 和 i18n |
| 旧手写 desktop | 已被否定 | 不再作为主路线，只作反例 |

## 3. 最重要的技术纠错

### 3.1 不要再说“把 Reames cache-first 移植到 Reasonix”

这个说法是错的。

Reasonix 已经有 DeepSeek/OpenAI-compatible cache-hit 相关机制。新项目要做的是：

```text
保留 Reasonix cache pipeline
+ 加 Reames metadata-invisibility guard
+ 加 cache-first boundary tests
+ 在 UI 中用用户能理解的方式展示缓存/上下文状态
```

不得用旧 Python provider 或手写 cache 层覆盖 Reasonix 的 provider/cache 机制。

### 3.2 不要再做“CLI 桌面可视化工作台”

用户明确否定过这个方向。桌面端不是：

- 终端日志面板；
- 工程状态仪表盘；
- 给开发者看的源码/验证说明页；
- 一堆“基线、迁移、契约、phase”的工程文案。

桌面端应该是用户实际使用的 Agent：

- 新建/切换会话；
- 输入任务、附加文件/图片/上下文；
- 查看 Agent 思考、工具调用、文件变更、审批请求；
- 调整模型、权限、工作区、插件、MCP、记忆、网络、主题、更新；
- 能看懂当前 Agent 在做什么，能暂停/继续/取消/批准/拒绝。

## 4. 产品和 UI 方向

### 4.1 桌面端优先

近期最高优先级是桌面端视觉效果和真实交互体验。验收标准不是“代码里有页面”，而是用户打开后觉得它像一个可用的桌面 Agent。

必须具备的主入口：

| 区域 | 用户视角目标 |
|---|---|
| 左侧会话/项目栏 | 找到最近任务、项目、历史会话、新建任务 |
| 中央对话/工作区 | 和 Agent 对话，看到输出、工具进度、结果和问题 |
| 输入区 | 输入任务、切换模式、附加上下文、发送/停止 |
| 右侧上下文/工作区栏 | 查看文件、变更、上下文、记忆、成本/cache、待处理事项 |
| 设置中心 | 模型、密钥、权限、MCP、插件、记忆、外观、网络、更新 |
| 审批弹窗 | 清楚说明要执行什么、风险是什么、允许/拒绝/记住选择 |

### 4.2 Apple-light 视觉风格

视觉风格保留用户指定的 Apple-light，而不是黑金、游戏化、工程仪表盘。

关键词：

- 浅色；
- 柔和灰白背景；
- 低噪音；
- 清晰层级；
- 合理留白；
- 圆角卡片；
- 高质量空状态；
- 中文排版舒适；
- 交互控件像真实桌面应用，而不是网页 demo。

### 4.3 中文优先和禁用文案

默认中文。UI 文案必须面向普通使用者。

禁止在主 UI 暴露这类工程自嗨文字：

- “Phase A/B/C”
- “baseline”
- “migration”
- “contract”
- “cache-first boundary”
- “Reasonix import”
- “P0 verification”
- “provider-visible payload”

这些只能放在开发文档、日志或高级诊断中。用户界面应改成：

| 工程说法 | 用户界面说法 |
|---|---|
| baseline failed | 应用自检未通过 |
| provider-visible payload | 发送给模型的内容 |
| cache hit rate | 上下文缓存 |
| migration | 数据升级 |
| contract test | 兼容性检查 |
| gateway channel | 消息渠道 |

## 5. 必须保留的 Reasonix 模块

| 模块 | 处理原则 |
|---|---|
| `internal/control` | 作为 runtime/control 核心保留，CLI/Desktop/Server/Gateway 都应经由它 |
| `internal/agent` | 保留 agent loop、session、plan、cache diagnostics、tools、subagent |
| `internal/provider` / `internal/provider/openai` | 保留 DeepSeek/OpenAI-compatible provider/cache/pricing |
| `internal/permission` | 保留权限/审批模型，并映射成桌面审批体验 |
| `internal/plugin` / `internal/skill` | 保留 MCP/plugin/skill 生命周期 |
| `internal/serve` | 保留 server/auth/SSE，为云部署和 Web UI 打底 |
| `desktop/app.go` / `desktop/settings_app.go` | 保留 Wails bridge，后续收敛成 Reames public boundary |
| `desktop/frontend/src` | 保留 Reasonix desktop skeleton，做 Apple-light/中文/交互产品化，不再手写假 UI |

## 6. 其他参考项目融合原则

| 参考项目 | 角色 |
|---|---|
| Reasonix | 主底座，优先保留源码结构和桌面能力 |
| Reames Lite | 公共边界、cache/metadata 约束、中文体验、测试纪律 |
| Hermes | 云部署、IM/渠道网关、消息 envelope、部署姿态 |
| Codex | 桌面 Agent 交互、审批、会话、工作区、任务控制体验参考 |
| Claude Code | 插件/技能/Hook UX 参考 |
| Kimi / MiMo / Scream | 局部交互、事件、权限队列、goal/plan/subagent 概念参考 |
| AgentArk | secret/audit/sentinel/server 安全参考 |
| Impeccable / awesome-design-md Apple | 视觉克制、设计 token、Apple-light 参考 |

原则：**只吸收机制和体验，不再把多个项目代码乱拼成一个不可维护仓库。**

## 7. 当前已知风险

| 风险 | 当前判断 | 处理方式 |
|---|---|---|
| 文档历史太多 | 容易让接手者迷路 | 本文 + 执行计划作为唯一入口 |
| 仓库混有 Python/Hermes 残留 | 会影响维护判断 | 先不暴删；按执行计划分批标记、迁移、删除 |
| desktop nested module 未被 root test 覆盖 | 容易误判“全仓测试已过” | 每批单独跑 desktop 测试 |
| 前端依赖未安装 | UI 无法完整验证 | 桌面 UI 批次必须先恢复 frontend build |
| UI 容易再次工程化 | 用户已多次否定 | 每次 UI 变更必须截图/点击验证/中文文案检查 |
| 直接重写 Reasonix 机制 | 会破坏已有缓存、权限、server 能力 | 先读源码和测试，优先包裹/适配，不覆盖 |

## 8. 新接手者工作规则

1. 先读本文，再读 `docs/REAMES_AGENT_EXECUTION_PLAN.md`。
2. 不要在旧 `F:\Reames-Lite` 开主线功能。
3. 不要把桌面端做成工程仪表盘。
4. 不要覆盖 Reasonix cache/provider/runtime。
5. 每批必须有验证命令。
6. UI 批次必须有截图或浏览器/桌面验证。
7. 面向用户的文字必须中文优先、少术语。
8. 方向更新改本文；执行顺序改执行计划；不要再新增一堆相互竞争的路线文档。

## 9. 当前结论

Reames Agent 现在应该按这个顺序推进：

```text
稳定新仓库基线
→ 恢复 Reasonix desktop/frontend 可运行可验证
→ 做 Apple-light 中文桌面 Agent 体验
→ 建立 Reames public boundary 和 metadata/cache guard
→ 清理 legacy/Hermes/Python 残留
→ 融合云部署、Web、Gateway、插件、记忆等能力
```

最重要的一句话：**先把桌面端做成用户愿意打开、看得懂、点得动的桌面 Agent，再谈全面融合。**
