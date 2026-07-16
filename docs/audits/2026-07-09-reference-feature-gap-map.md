# 参考项目功能差异与吸收清单

> 日期：2026-07-09
>
> 范围：`F:\reames-agent` 当前源码与文档、`F:\code-reference` 参考仓库、`F:\Reames-Lite` 前身资料。
>
> 性质：证据化差异审计与候选 backlog，不替代 `docs/PROJECT.md`、`docs/DEVELOPMENT_PLAN.md` 和 `docs/REFERENCE_GOVERNANCE.md`。
>
> 状态：历史快照。此后 M2–M5 已关闭多项当时缺口；当前优先级必须以 `docs/DEVELOPMENT_PLAN.md` 为准。

## 1. 审计原则

这份文档只记录经过源码或明确参考资料核对的结论。每条候选都必须回答三个问题：

1. Reames 当前是否已有相同或相近机制；
2. 参考项目的证据来自源码、测试、配置、文档还是 changelog；
3. 后续应补“新能力”、补“产品化闭环”，还是只保留为长期参考。

证据等级：

- **S 源码证据**：直接来自 `.go`、`.ts`、`.py`、`.rs`、测试或配置实现。
- **D 文档证据**：来自 README、AGENTS、架构文档、配置示例、CHANGELOG；可作为候选，但不能单独证明实现细节。
- **R Reames 已有**：Reames 当前源码中已有等价或近似机制。
- **P 部分已有**：Reames 有基础，但缺参考项目中的成熟闭环。
- **G 缺口**：当前未发现 Reames 有等价机制。

## 2. 总体结论

Reames 当前已经有不少“全能 Agent”基础，不应重复造轮子：

- 统一运行边界：`internal/control/port.go:208` 已有 `SessionAPI`，`internal/control/controller.go:255` 已有 `RuntimeStatus`。
- Typed event stream：`internal/event/event.go` 定义 `TurnStarted`、`ToolDispatch`、`ApprovalRequest`、`TurnDone`、`SubagentStarted`、`CacheUpdated`、`GuardianAssessment` 等事件。
- Gateway 基础：`internal/bot/types.go:28` 有 `SessionSource`，`internal/bot/types.go:39` 有 `InboundMessage`，`internal/bot/gateway.go:23` 有 `GatewayConfig`，`internal/bot/session.go:88` 有 Hermes 风格 session key。
- Gateway service：`internal/gatewayservice/service.go:51` 有 lifecycle `Plan`，`internal/gatewayservice/service.go:98` 有跨 OS `BuildPlan`。
- 后台任务：`internal/jobs/jobs.go:2` 说明 jobs 支持 `bash run_in_background` 和 `task run_in_background`；`internal/jobs/jobs.go:33` 有 `running/done/failed/killed` 状态。
- 子 Agent：`internal/agent/task.go:177` 有 `TaskTool`，`internal/agent/task.go:307` 有 `ReadOnlyTaskTool`，`internal/agent/parallel_tasks.go:32` 有 `parallel_tasks`。
- 证据收据：`internal/evidence/evidence.go:36` 已有 `Receipt`，`internal/evidence/evidence.go:690` 已有 `ReceiptFromToolCall`。
- 缓存诊断：`internal/event/event.go:231` 已有 `CacheDiagnostics`；`internal/agent/cache_shape.go:67` 已有 cache shape 比较。
- MCP 生命周期：`internal/plugin/plugin.go:100` 有 `Host`，`internal/plugin/plugin.go:177` 有 `StartPolicy`，`internal/plugin/lazy.go` 有懒加载/缓存 schema 机制，`internal/mcpdiag/auth.go:14` 有认证诊断。
- 安全评估：`internal/event/event.go:208` 有 `GuardianResult`，`internal/guardian/policy.go` 有 risk/user authorization 规范化，`internal/installsource/errors.go:11` 有安装来源风险等级。

所以本文件的结论不是“Reames 没有这些功能”，而是：**已有基础很多，但缺少跨入口协议稳定性、真实部署验证、后台 Agent 产品闭环、worktree 隔离、确定性 workflow、市场/安装信任体验、远程审批和自我迭代 gate。**

## 3. 修正后的优先级矩阵

| 优先级 | 方向 | Reames 现状 | 参考来源 | 可靠结论 | 建议动作 |
|---|---|---|---|---|---|
| P0 | 控制协议硬化 | P：已有 `SessionAPI`、typed events、eventwire，但 command/event/error wire contract 仍需版本化 | Codex/Kimi/Scream | 不能新建第二套 runtime；应固化现有 control 边界 | 为提交、取消、审批、状态、会话恢复建立 schema + fixtures + 依赖守卫 |
| P0 | Gateway 真实部署闭环 | P/R：已有 bot envelope、路由、队列、slash、service plan | Hermes/Scream | 不是从零做 Gateway；缺真实渠道和服务运维闭环 | 做飞书优先的 setup、service smoke、诊断、文档和失败恢复 |
| P0 | 后台 Agent 产品化 | P：已有 jobs、background task、subagent store；缺 Claude 式 agent roster/worktree/attach | Claude Code/MiMo/Scream | 不能重复 jobs；要提升成可观察、可恢复的后台 Agent | 增加 BackgroundAgent 投影、NeedsInput/Stopped 语义、worktree/checkpoint 隔离 |
| P1 | Evidence/Receipt 增强 | R/P：已有 `evidence.Receipt` 和 readiness audit | Reames Lite/Reasonix/AgentArk | 不是新增 ToolReceipt；是补字段和展示 | 扩展 duration、permission、sandbox、rollback、approval id、持久化/桌面展示 |
| P1 | Cache diagnostics 产品化 | R/P：已有 `CacheDiagnostics` 和 CLI 展示 | Reasonix/Reames Lite | 缺面板、CI 元数据和原因归因闭环 | Desktop Cache Health、cache-sensitive PR gate、tool schema token 预算 |
| P1 | MCP/Plugin/Skill 生态 | P：已有 MCP 状态、懒加载、超时、认证诊断、pluginpkg/installsource | Claude Code/Kimi/Scream | 缺市场级安装信任、OAuth helper、回滚和容错 frontmatter | trust level、安装 consent、auth helper、transient retry、disable/rollback |
| P1 | 风险分类和远程审批 | P：已有 guardian risk、installsource risk、permission/sandbox/trust | AgentArk/Claude Code | 需要统一风险标签和远程 companion approval | `RiskLabel` 投影到审批、日志、远程设备 scoped token |
| P2 | 确定性 workflow | P/G：已有 autoresearch store、goal、plan、evidence；未发现通用 workflow runner | MiMo/Scream | 适合研究、fact-check、compose，但必须复用权限和证据 | 先做 deep-research/fact-check 的固定状态机和文件证据 |
| P2 | 通用办公/媒体能力 | G/P：当前主线以代码工具为主，未发现完整 DOCX/PDF/PPTX/XLSX/视频闭环 | MiMo/Scream/Kimi | 应通过 Skill/Tool 包增量接入 | 做 office/media skill 包，复用权限、检查点、证据 |
| P2 | 实验功能开关 | P/G：有单项配置和 token economy，未发现统一 feature flag registry | Kimi | 避免半成品默认暴露 | 建统一实验开关 registry，默认 off，配置/环境变量开启 |
| P2 | 自我迭代 gate | P：已有 upstream watch/autoresearch；缺 replay/canary/promotion gate | AgentArk/MiMo | 只能自动研究/建单/草稿，不自动合并 | 建离线 replay、canary、promotion report、rollback 规则 |

## 4. 分领域审计

### 4.1 统一控制面与协议

Reames 证据：

- `internal/control/port.go:1` 说明 control 是 transport-agnostic session driver。
- `internal/control/port.go:208` 定义 `SessionAPI`，组合 lifecycle、turn、approval、goal、history、memory、capabilities、status、persistence、input、settings。
- `internal/event/event.go` 定义 agent typed event stream。
- `internal/eventwire/wire.go:237` 已有 wire 形态的 `CacheDiagnostics`，说明部分事件已进入 JSON contract。

参考证据：

- Codex `F:\code-reference\codex\codex-rs\app-server-client\README.md:50` 记录 `thread/start`、`thread/resume`。
- Codex `README.md:52` 记录 `SessionConfigured` 可晚到。
- Codex `README.md:61-67` 记录满队列和审批不应挂死。
- Codex `app-server-client/src/lib.rs:680`、`:711`、`:743` 记录 pending server request resolve/reject/shutdown 行为。
- Kimi `F:\code-reference\kimi-code\AGENTS.md:17-27` 明确 TUI/Web/Server/Core/SDK 边界。

判定：

- **P 部分已有。** Reames 已经有正确方向，不应再另起一套 app-server runtime。
- 真实缺口是：跨 Desktop/CLI/Serve/Gateway 的 command/event/error wire schema 还不够显式、版本化和可回归。

建议：

1. 以 `control.SessionAPI` 为内核边界，定义最小 `control/protocol` schema。
2. 固定事件顺序：session configured/resumed、turn started、approval requested、tool dispatch/result、turn done。
3. 加依赖守卫：前端入口只能依赖 control/protocol，不直接穿透 agent/tool/provider 内部结构。

### 4.2 Gateway、云端部署与多渠道

Reames 证据：

- `internal/bot/types.go:28` `SessionSource`，`internal/bot/types.go:39` `InboundMessage`，`internal/bot/types.go:74` `OutboundMessage`。
- `internal/bot/types.go:163` `PlatformCapabilities`，已有 rich text、cards、media、reactions、threads、voice 能力声明。
- `internal/bot/gateway.go:23` `GatewayConfig`，包含 queue、pairing、allowlist、control、approval timeout、workspace/channel/route 配置。
- `internal/bot/gateway.go:72` `SessionMapping`，`internal/bot/gateway.go:88` `RouteConfig`。
- `internal/bot/session.go:13` queue mode：steer/followup/collect/interrupt。
- `internal/bot/session.go:127` slash bypass 命令已含 `/stop`、`/new`、`/approve`、`/deny`、`/mode`、`/queue`、`/projects`、`/sessions`、`/attach`、`/status`。
- `internal/gatewayservice/service.go:98` 已能按 OS 渲染 service plan；`:187` systemd，`:207` launchd，`:235` Windows Scheduled Task。

参考证据：

- Hermes `F:\code-reference\Hermes\AGENTS.md:244` 记录 `gateway/` 是 messaging gateway。
- Hermes `AGENTS.md:385-424` 记录中央命令注册、gateway help、Telegram/Slack 菜单派生。
- Hermes `AGENTS.md:1237-1246` 记录 gateway 有两层 message guard，`/stop`、`/new`、`/queue`、`/status`、`/approve`、`/deny` 必须绕过队列。
- Hermes `cli-config.yaml.example:592-599` 记录全局 `max_concurrent_sessions`。
- Hermes `cli-config.yaml.example:686-696` 记录 gateway timeout、warning、graceful drain。
- Scream `F:\code-reference\scream-code\README.md:136-156` 记录 cc-connect 与远程 chat commands。

判定：

- **R/P 已有大量基础。** 旧文档如果写“定义 GatewayEnvelope / 增加 slash 命令”会误导后来人。
- 缺口在真实部署和运维成熟度：setup wizard、真实飞书/微信/QQ/Telegram smoke、后台 service 状态诊断、失败恢复、跨平台命令目录、渠道消息渲染策略。

建议：

1. 不新增并行 gateway loop；继续使用 `internal/bot` + `internal/gatewayservice`。
2. 把现有 envelope 固化成文档化 wire contract，尤其是附件、审批、错误、进度、reply policy。
3. 首个验收闭环选飞书：安装 → 配置 → service start/status/restart → 收消息 → 审批 → 停止 → 会话恢复。

### 4.3 后台 Agent、jobs 与子 Agent

Reames 证据：

- `internal/jobs/jobs.go:2` 说明 jobs 是 background tools registry。
- `internal/jobs/jobs.go:33` 状态已有 running/done/failed/killed。
- `internal/jobs/jobs.go:80` `Job` 持有 status、cancel、done、artifact path 等。
- `internal/agent/task.go:169` 说明 `TaskTool` 会创建独立 session 的子 Agent。
- `internal/agent/task.go:274` `run_in_background` 支持子 Agent 异步执行。
- `internal/agent/task.go:307` `ReadOnlyTaskTool` 是只读 research 子 Agent。
- `internal/agent/parallel_tasks.go:17` 说明并发只读 sub-agent 调度。
- `internal/agent/subagent_store.go:31` `SubagentMeta`，`:104` `SubagentStore`，`:200` `CleanupStaleRunning`。

参考证据：

- Claude Code `F:\code-reference\claude-code\CHANGELOG.md:39-56` 记录 background session recover、agent roster、Needs input 等修复。
- Claude Code `CHANGELOG.md:45-46` 记录 worktree-isolated subagents 相关修复。
- Claude Code `CHANGELOG.md:125-127` 记录 daemon/stop 永久语义问题。
- Claude Code `CHANGELOG.md:207-208` 记录后台命令和 worker 在重启/更新后恢复。
- Claude Code `CHANGELOG.md:332` 记录后台 subagent 的权限提示回到主会话。
- MiMo `F:\code-reference\MiMo-Code\README.md:136` 记录 compose workflow 将独立任务分发到隔离 git worktree。

判定：

- **P 部分已有。** Reames 已有 jobs 和 subagent，不应再做一个重叠的 background task 系统。
- 真缺口是“后台 Agent 产品层”：durable roster、NeedsInput/Working/Completed、attach/resume、永久 stop、父会话审批代理、writable worktree 隔离和清理。

建议：

1. 在现有 `jobs.Manager` 和 `SubagentStore` 上方增加 `BackgroundAgentView` 投影，不替换底层。
2. 先把状态语义补齐：Queued、Working、NeedsInput、BlockedApproval、Stopping、Stopped、Failed、Completed。
3. writable 子 Agent 默认进入 worktree 或 checkpoint sandbox；只读子 Agent 保持现有轻量隔离。

### 4.4 Evidence、Tool Receipt 与完成判定

Reames 证据：

- `internal/evidence/evidence.go:36` `Receipt` 已记录 tool、args、success、command、step、paths、read/write、todos。
- `internal/evidence/evidence.go:690` `ReceiptFromToolCall` 提取 bash command、complete_step、todo、paths。
- `internal/event/event.go:302` `ReadinessAuditSink`，`:307` `RecordReadinessAudit`。
- `internal/agent/agent.go:2459`、`:2465` 在工具执行路径记录 receipt。
- `internal/tool/builtin/completestep_test.go` 和 `internal/evidence/*_test.go` 已大量验证 receipt 驱动的 readiness。

参考证据：

- Reames Lite `F:\Reames-Lite\20260627-RL项目大刀计划.md` 记录 ToolRuntime/ToolReceipt/ToolScheduler 思路。
- AgentArk `F:\code-reference\AgentArk\ARCHITECTURE.md:7` 强调 approvals、sandboxing、verifiable records。
- Reasonix `F:\code-reference\DeepSeek-Reasonix\cmd\e2ebench\main.go:1-2` 有真实 provider e2e benchmark 报告。

判定：

- **R/P 已有。** 旧文档中的“建立 ToolReceipt”不准确。
- 缺口是收据字段、持久化和 UI 可见性：权限决策、sandbox 模式、耗时、approval id、rollback/checkpoint、输出摘要、是否进入模型上下文。

建议：

1. 扩展 `evidence.Receipt` 而不是另建 `ToolReceipt`。
2. Desktop/CLI/Gateway 展示同一份 readiness/receipt 摘要。
3. `complete_step` 继续以 receipt 和真实验证为准，避免模型自报完成。

### 4.5 Cache diagnostics 与 cache-first 纪律

Reames 证据：

- `internal/event/event.go:231` `CacheDiagnostics` 包含 prefix/system/tools/log rewrite hash、tool schema tokens、cache hit/miss tokens。
- `internal/agent/cache_shape.go:67` `CompareShape` 生成 cache diagnostics。
- `internal/agent/cache_diagnostics_test.go:30` 测试 usage event 携带 cache diagnostics。
- `internal/cli/chat_tui_test.go:816` 测试 CLI 展示 cache prefix changed。

参考证据：

- Reasonix `F:\code-reference\DeepSeek-Reasonix\CONTRIBUTING.md:97-116` 明确 cache-first review gate 和 cache-sensitive PR metadata。
- Reasonix `internal/boot/boot.go:264-312` 多处注释说明 cache-stable prefix、环境快照、memory prefix 和 skill index。
- Reames Lite 旧计划强调 system prompt/tool schema 稳定前缀和 tool schema token cost。

判定：

- **R/P 已有。** Reames 已经吸收了核心机制。
- 缺口是治理闭环：PR 元数据、CI 检查、Desktop Cache Health、tool schema token 预算可视化。

建议：

1. 对 cache-sensitive path 加 `Cache-impact` 和 `Cache-guard` 检查。
2. Desktop 增加 Cache Health 卡片，显示 prefix hash、tools hash、schema tokens、变更原因。
3. 文档明确 UI/channel metadata 不得进入稳定前缀。

### 4.6 MCP、Plugin、Skill 生态

Reames 证据：

- `internal/plugin/plugin.go:30` 默认 MCP call timeout。
- `internal/plugin/plugin.go:177` `StartPolicy` 支持 per-plugin timeout、concurrency、abort-on-error。
- `internal/plugin/plugin.go:240` `StartAvailable` 记录失败但保留可用 server。
- `internal/plugin/plugin.go:618` `RecordFailure` 支持状态 UI。
- `internal/plugin/lazy.go` 说明 cache-hit placeholder 固定 tool schema，避免 mid-session 工具面变动破坏缓存。
- `internal/mcpdiag/auth.go:14` `AuthDiagnosis`，`:21` 识别 401/403 等 auth failure。
- `desktop/app.go:5630-5633` `ServerView` 状态包含 connected/deferred/failed/initializing/disabled。
- `internal/cli/mcp_manager_actions.go:40` 有 disable action，`internal/cli/mcp_manager_view.go:241-257` 有 auth/retry/connect/disable 行为。

参考证据：

- Claude Code `CHANGELOG.md:244` 记录 MCP auth helper 在 401/403 后重跑并重连。
- Claude Code `CHANGELOG.md:264-265` 记录 tools/list/prompts/list/resources/list 和 OAuth transient retry。
- Claude Code `CHANGELOG.md:282` 记录 remote MCP tool idle timeout。
- Kimi `F:\code-reference\kimi-code\README.md:65-66` 记录 conversational MCP config 和 marketplace trust level。
- Scream `F:\code-reference\scream-code\AGENTS.md:418-431` 记录 Skill Center 安装/卸载/marketplace。

判定：

- **P 部分已有。** Reames MCP 生命周期不弱，不能写成空白。
- 缺口在“生态产品化”：OAuth/auth helper、安装信任等级、marketplace/包升级、frontmatter 容错、disable/rollback/cleanup 的统一 UX。

建议：

1. 先补 remote MCP auth helper，不要把 auth stub 暴露给模型。
2. 对 transient network errors 扩展 retry 到 tools/prompts/resources/OAuth。
3. Skill/Plugin/MCP 安装走 `installsource` 风险预览 + consent + rollback。

### 4.7 安全、风险分类与远程审批

Reames 证据：

- `internal/guardian/policy.go:98` 有 risk levels，`:105` 有 user authorization levels。
- `internal/event/event.go:208` `GuardianResult` 会进入事件流。
- `internal/installsource/errors.go:11` 定义 `RiskLevel` low/medium/high。
- `internal/installsource/mcp.go:55`、`internal/installsource/skill.go:59` 对 MCP/Skill 安装计算风险。
- `internal/permission`、`internal/sandbox`、`internal/trust`、`internal/crypto` 已存在对应安全包。

参考证据：

- AgentArk `F:\code-reference\AgentArk\ARCHITECTURE.md:7` secure-first：encrypted secrets、approvals、sandboxing、verifiable records。
- AgentArk `ARCHITECTURE.md:47-48` security 分类与 WASM/Docker sandbox。
- AgentArk `ARCHITECTURE.md:75-77` extension packs、companion devices、secrets handling。
- AgentArk `clients/companion/README.md:18-23` companion WebSocket、pairing、scoped token、高风险 fresh approval。
- Claude Code `CHANGELOG.md:233` 记录 auto-mode denial reasons。
- Claude Code `CHANGELOG.md:263` 记录 network host permission 记忆。

判定：

- **P 部分已有。** Reames 已有风险和 guardian 机制，但风险标签没有统一贯穿 approval、日志、远程设备和 UI。
- 远程 companion approval 是云端部署形态的重要缺口。

建议：

1. 定义统一 `RiskLabel` 投影：read-only、workspace-write、shell-local、network、credential、destructive、external-side-effect。
2. ApprovalRequest 增加 risk labels、可回退性、checkpoint id、需要 fresh approval 的原因。
3. 远程设备只拿 scoped token；高风险操作必须 fresh approval。

### 4.8 Goal、Workflow、研究与事实核查

Reames 证据：

- `internal/control/goal.go`、`internal/control/auto_plan.go`、`internal/autoresearch` 已有 goal/auto research 方向。
- `internal/autoresearch/store.go` 创建 `.reames-agent/autoresearch/<task>/state`，包含 task spec、progress、findings、iteration log。
- `internal/autoresearch/readiness.go` 按 success criteria 和 accepted findings 判断 readiness。
- `internal/control/auto_plan.go` 与 `internal/agent/parallel_tasks.go` 已支持并行调研的局部机制。

参考证据：

- MiMo `F:\code-reference\MiMo-Code\README.md:99-110` 记录 project memory、checkpoint、task progress、context reconstruction、budgeted injection。
- MiMo `README.md:122` 记录独立 judge 判断 goal stop condition。
- MiMo `README.md:130-142` 记录 deterministic workflows、自定义 workflow。
- MiMo `packages/opencode/src/workflow/builtin/compose.js:2-14` 是 compose workflow 源码入口。
- MiMo `packages/opencode/src/workflow/builtin/deep-research.js:2-9` 是 deep-research workflow 源码入口。
- MiMo `packages/opencode/src/workflow/builtin/fact-check.js:2-4` 是 fact-check workflow 源码入口。
- Scream `F:\code-reference\scream-code\AGENTS.md:344-358` 记录 Goal Loop 和 `WriteGoalNote`，工作笔记不在 conversation context。
- Scream `AGENTS.md:440-447` 记录 FusionPlan。

判定：

- **P/G 部分已有。** Reames 有 autoresearch store 和 goal/plan 基础，但没有通用 deterministic workflow runner，也没有 MiMo 那种 compose/deep-research/fact-check 固定流程。
- 旧文档写“已有 autoresearch 雏形”是准确的；应进一步明确缺口是 workflow runtime 和独立 judge。

建议：

1. 保留 `internal/autoresearch`，不要另建研究存储。
2. 增加 workflow runner：固定阶段、输入输出文件、重试预算、证据 ledger、可恢复 checkpoint。
3. 第一批 workflow 只做 deep-research 和 fact-check，避免先做大而全 compose。

### 4.9 通用办公、媒体和多模态

Reames 证据：

- 当前工具契约以代码、shell、文件、LSP、MCP、memory、research 为主；未在 `internal/tool/builtin` 中发现完整 DOCX/PDF/PPTX/XLSX/视频编辑闭环。
- Reames 不是完全没有文件/媒体基础：`internal/control/inputimages_test.go` 覆盖图片输入方向，`internal/control/refs_test.go` 覆盖 PDF 文本引用提取；缺口是可复用的 office/media 工具包、权限契约、检查点和端到端产物验证。

参考证据：

- MiMo `README.md:157-172` 记录官方 skills：docx/pdf/pptx/xlsx/html-to-video。
- MiMo `README.md:180-245` 记录 voice input、ASR provider 配置。
- Scream `packages/agent-core/src/tools/builtin/file/read-media.ts:2-7` 是 image/video read media 工具源码。
- Kimi `F:\code-reference\kimi-code\README.md:64` 记录 video input。

判定：

- **G/P 缺口为主。** 这是 M7 能力，不应抢在 M1-M3 前面。
- 应通过 Skill/MCP/Tool package 加入，而不是污染核心 Agent loop。

建议：

1. 先定义 office/media skill 包的权限和证据契约。
2. 每类只做一个 fixture：PDF 摘要、XLSX 分析、PPT 生成、视频摘要。
3. 所有文件写入必须走 checkpoint/approval/receipt。

### 4.10 Feature flags 与实验能力治理

Reames 证据：

- Reames 有配置项、plan mode、token economy、auto-plan 等单项开关，但本次未发现统一的 `featureflag` registry。
- `docs/SPEC.md:349` 提到 auto-plan 是 feature flag，但源码层面更多是具体配置项，不是统一 registry。

参考证据：

- Kimi `F:\code-reference\kimi-code\AGENTS.md:59-61` 明确实验功能进入 `packages/agent-core/src/flags/registry.ts`，环境变量默认 off。
- MiMo `README.md:319` 记录 `experimental.maxMode`。

判定：

- **G/P 缺口。** Reames 需要统一实验开关，防止 Gateway/workflow/marketplace 等半成品默认进入用户主流程。

建议：

1. 新增统一 experimental registry。
2. 默认 off；支持配置和环境变量；Desktop 设置页只显示可理解的稳定开关。
3. CI 或测试确保未完成能力不会默认启用。

### 4.11 自我迭代与上游参考追踪

Reames 证据：

- `scripts/check_upstreams.py`、`scripts/test_check_upstreams.py`、`scripts/test_upstream_watch_issue.mjs` 已有上游检测和 issue 机制。
- `internal/autoresearch` 可为研究任务提供文件状态。
- `docs/REFERENCE_GOVERNANCE.md` 已明确“不自动合并上游”，只自动发现、分类、建单、人工接受。

参考证据：

- AgentArk `ARCHITECTURE.md:73` 记录 Evolve：prompt、policy、classifier、specialist evolution with canaries、replay evaluation、promotion gates、rollback。
- AgentArk `ARCHITECTURE.md:87-88` 记录 evolve candidate → test/approval，prompt/policy candidate → benchmark/limited rollout/promotion/rollback。
- AgentArk frontend `F:\code-reference\AgentArk\frontend\src\components\pages\TracePage.tsx` 与 `traceEvolutionHelpers.tsx` 有 trace/evolution UI 证据；`src\core\self_evolve\gates\replay_gate.rs`、`promotion_gate.rs` 是 replay/promotion gate 源码证据。
- MiMo `README.md:253-254` 记录 `/dream`、`/distill`。

判定：

- **P 部分已有。** Reames 已有上游研究/建单基础，但缺 self-improvement 的 promotion gate。
- 必须保持人工决策，不允许“自动升级并合并”。

建议：

1. 上游 worker 输出：报告、Issue、草稿分支/PR、风险说明、测试建议。
2. 自我迭代输出进入 replay/canary/promotion report，不能热改生产 prompt/tool/policy。
3. 所有自动修改必须有 rollback path 和 CI/bench evidence。

## 5. 参考项目逐项结论

### DeepSeek Reasonix

角色：主源码上游。

可靠吸收：

- 安全/provider/cache/runtime/Desktop 修复必须持续人工审查。
- `CONTRIBUTING.md:97-116` 的 cache-first gate 应继续同步到 Reames。
- `cmd/e2ebench/main.go` 的真实 provider benchmark 思路已经在 Reames 近似存在，但应继续扩展任务集。

风险：

- 不能批量 merge 覆盖 Reames 品牌、中文体验、Gateway/Cloud 方向和控制面改造。

### Hermes

角色：Gateway 和多渠道部署参考。

可靠吸收：

- 中央命令注册、Gateway help/menu 派生、两层 message guard、graceful drain、session cap、渠道工具集策略。

Reames 状态：

- Reames 已经吸收了 session key、route、queue、slash、service plan 的核心；后续重点是真实渠道和部署验证。

### Codex

角色：app-server 协议、审批可靠性、daemon/remote control 参考。

可靠吸收：

- versioned protocol、queue saturation 行为、pending request shutdown、daemon pairing/readiness。

Reames 状态：

- Reames 有 control port 和 eventwire，但需要 schema fixtures 和跨入口兼容测试。

### Claude Code

角色：后台 Agent、worktree、MCP/Plugin/Skill 生态成熟度参考。

证据限制：

- 背景 Agent/worktree 大多来自 `CHANGELOG.md`，属于 **D 文档/changelog 证据**；不能当作可直接移植的源码设计。
- Plugin manifest/skills 目录有可读文档和样例，可作为生态格式参考。

可靠吸收：

- 后台 Agent roster、permanent stop、daemon restart recovery、main-session approval broker、MCP transient retry、auth helper、skill/plugin 容错。

### Kimi Code

角色：SDK 边界、Web peer surface、experimental flags、MCP 配置向导、视频输入和市场 trust。

可靠吸收：

- `AGENTS.md:17-27` 的 UI/Core/Server/SDK 边界。
- `AGENTS.md:59-61` 的实验开关 registry。
- `README.md:64-66` 的视频输入、AI-native MCP config、marketplace trust。

Reames 状态：

- control 边界已有，但统一 feature flag 和对话式 MCP setup 尚缺。

### Scream Code

角色：Goal Loop、Wolfpack/FusionPlan、cc-connect、主题纪律、Skill Center、媒体/记忆工具参考。

可靠吸收：

- Goal notes 不放 conversation context。
- FusionPlan/Wolfpack 可先做只读计划/研究模式。
- cc-connect setup/daemon UX 可参考，但 Reames 应落在自身 `internal/bot`/`gatewayservice`。

### MiMo Code

角色：持久记忆、确定性 workflow、deep-research/fact-check、voice/office/media skills。

可靠吸收：

- `README.md:99-110` 的 checkpoint/memory/progress/context reconstruction。
- `workflow/builtin/*.js` 的 deterministic workflow 源码结构。
- `/goal` 独立 judge 和 `/dream`/`/distill` 思路。

Reames 状态：

- autoresearch store 有基础；缺 workflow runner 和 judge。

### AgentArk

角色：安全、companion approval、自我迭代 gate、runtime health。

可靠吸收：

- secure-first 分类、companion pairing/scoped token、approval audit、自我迭代 replay/canary/promotion/rollback。

Reames 状态：

- guardian/installsource/permission/sandbox 已有基础；缺统一风险标签和 companion 远程审批。

### Reames Lite

角色：旧项目经验和契约参考。

可靠吸收：

- prompt/display 分离、cache-stable prefix、ToolRuntime 顺序、子 Agent context 隔离、MCP state、Desktop runtime panels。

Reames 状态：

- 大量思想已进入新项目；后续应逐项验证和产品化，而不是重写旧 Python 实现。

## 6. 明确避免误导的修正

以下旧说法不再采用：

- “定义 GatewayEnvelope” → 改为“固化现有 `InboundMessage`/`OutboundMessage`/routing 为 wire contract”。
- “增加 slash commands” → 改为“统一命令目录、权限分级、平台菜单派生和文档”。
- “建立 ToolReceipt” → 改为“扩展已有 `evidence.Receipt`”。
- “建立 MCP state” → 改为“补强已有 MCP state/auth/retry/install UX”。
- “建立后台任务系统” → 改为“在已有 jobs/subagent 上做 Background Agent roster 和恢复语义”。
- “建立风险分类” → 改为“统一 guardian/installsource/permission 的风险标签并贯穿审批和远程设备”。

## 7. 下一批真实可执行任务

1. **Control protocol fixtures**：基于现有 `SessionAPI`、`event`、`eventwire`，给 submit/cancel/approval/status/session recovery 建 schema 和 fixtures。
2. **Gateway Feishu service smoke**：用现有 `internal/bot` 和 `internal/gatewayservice` 完成一台机器的 install/start/status/message/approval/stop 验证。
3. **BackgroundAgentView**：基于 `jobs.Manager` + `SubagentStore` 增加统一 roster 投影，不重写 jobs。
4. **Receipt 扩展**：给 `evidence.Receipt` 增加 duration、permission、sandbox、approval、rollback/checkpoint 元数据。
5. **Cache Health 面板**：把已有 `CacheDiagnostics` 在 Desktop 中产品化。
6. **MCP auth helper**：在现有 `mcpdiag`/plugin 状态基础上补 OAuth/auth helper 和 transient retry。
7. **RiskLabel 投影**：整合 guardian/installsource/permission 风险输出，审批和日志使用同一标签。
8. **Feature flag registry**：为实验 Gateway/workflow/marketplace 能力建立默认 off 的统一 registry。
9. **Deep-research workflow MVP**：复用 `internal/autoresearch`，做固定阶段和证据文件，不新增平行研究存储。
10. **Companion approval 设计稿**：参考 AgentArk companion protocol，先写 Reames 的 scoped device token 和 fresh approval 契约。

## 8. 维护规则

- 本文档的每个新增候选必须附 Reames 证据和参考证据。
- 若证据仅来自 README/CHANGELOG，必须标明 **D 文档证据**，不得写成“源码已实现”。
- 若 Reames 已有相似机制，必须写“扩展/固化/产品化”，不得写“新增/建立”。
- 若候选进入正式排期，应迁移到 `docs/DEVELOPMENT_PLAN.md`；本文保留来源、差异和证据。
- 若后续源码审计发现本文判断错误，应直接修正文档并写明修正原因。

## 9. 二次审查补遗：原文档未充分覆盖的横切能力

本节来自 2026-07-10 的二次只读审查，目标是补充前文未充分覆盖的项目源码差距。审查原则仍然是：

- 不把 Reames 已有能力写成“从零新增”。
- 不把参考项目 README/CHANGELOG 宣传写成源码事实。
- 优先记录可落地、可验证、不会和现有包职责冲突的补全方向。

### 9.1 Voice / STT / TTS 语音交互

Reames 证据：

- 当前未在 `internal/`、`cmd/`、`desktop/` 下发现完整 voice runtime、录音状态机、STT/TTS provider dispatcher 或 `/voice` 产品闭环。
- Reames 不是没有多模态基础：`internal/control/inputimages_test.go` 覆盖图片输入方向，`internal/control/refs_test.go` 覆盖 PDF 文本引用提取方向。缺口应限定为语音/音频产品闭环，而不是“完全没有多模态”。

参考证据：

- Hermes `F:\code-reference\Hermes\cli.py` 有 `_voice_mode`、`_voice_recording`、`_voice_processing`、`_voice_continuous`、`_voice_start_recording`、`_voice_stop_and_transcribe`、`_voice_speak_response_async` 等语音状态机和 TTS/STT 调用。
- Hermes `F:\code-reference\Hermes\tests\tools\test_transcription.py`、`test_transcription_command_providers.py`、`test_tui_gateway_server.py` 覆盖音频文件校验、本地/OpenAI/命令 provider dispatcher、TUI gateway voice action。
- MiMo `F:\code-reference\MiMo-Code\README.md:180-245` 记录 `/voice`、TenVAD、MiMo ASR、WSL/sox/pulseaudio 配置，属于 **D 文档证据**。

判定：

- **G 真缺口。** Reames 当前没有等价语音输入/TTS 闭环。
- 该能力依赖音频库、设备权限和平台差异，不宜直接污染核心 agent loop。

建议：

1. 作为可选官方 skill/plugin 增加：`voice record`、`voice transcribe`、`tts speak`。
2. provider dispatcher 复用现有 provider/config 思路，但音频 API key、模型和本地依赖单独诊断。
3. CLI/Desktop/Gateway 只暴露状态和动作，不把音频依赖放进默认启动路径。

### 9.2 通用 workflow runtime 与 worktree 隔离

Reames 证据：

- Reames 已有 Goal + AutoResearch，不应写成“没有长期研究/工作流基础”。
- `docs/superpowers/audits/2026-06-30-autoresearch-runtime-verification.md` 记录 `internal/autoresearch.Store.CreateTask`、`ValidateTask`、`Readiness`、`Controller.RecordAutoResearchEvidence`、Desktop AutoResearch API 等已验证项。
- `docs/GUIDE.md:694-741` 记录 Goal 与 AutoResearch 使用方式；`docs/GUIDE.md:771-787` 记录 workflow skill、executor 和 subagent 深度边界。

参考证据：

- MiMo `F:\code-reference\MiMo-Code\README.md:126-142` 记录 deterministic JavaScript workflows、compose/deep-research/fact-check、自定义 `.mimocode/workflows/*.js`，属于 **D 文档证据**。
- MiMo `F:\code-reference\MiMo-Code\packages\opencode\src\server\routes\instance\workflows.ts` 有 `workflow.list`、`workflow.resume`、`workflow.transcript`、`workflow.structure` API。
- MiMo `F:\code-reference\MiMo-Code\packages\opencode\src\server\routes\instance\experimental.ts` 有 worktree create/list/remove/reset API。
- MiMo `F:\code-reference\MiMo-Code\packages\sdk\js\src\v2\gen\types.gen.ts` 记录 workflow events、workflow config、worktree routes 和 workflow API 类型。

判定：

- **P/G。** Reames 已有 AutoResearch 运行时；真实缺口是更通用的宿主管理 workflow runtime、worktree 隔离、workflow API/详情页，而不是新增第二套 AutoResearch。

建议：

1. 保留 `internal/autoresearch` 作为研究任务专用 runtime。
2. 新增通用 workflow 层时复用 `control.Controller`、`agent/task`、`jobs` 和 evidence/checkpoint；不要复制研究任务存储。
3. 第一阶段只做 workflow 状态模型、固定阶段、transcript/structure API；worktree 隔离作为高风险写入能力单独 gate。

### 9.3 Design QA / UI 反模式检测 / 设计 Hook

Reames 证据：

- Reames 已有 hook 系统：`internal/hook/hook.go` 定义 `PreToolUse`、`PostToolUse`、`PermissionRequest`、`UserPromptSubmit`、`Stop`、`PostLLMCall`、`SessionStart`、`SessionEnd`、`SubagentStop`、`Notification`、`PreCompact`。
- Reames 已有主题/i18n/桌面前端基础：`internal/cli/theme.go`、`internal/i18n`、`desktop/frontend`。
- 当前未发现类似 Impeccable 的 design detector、UI 反模式规则库、低对比/层级/AI 设计痕迹检测器。

参考证据：

- Impeccable `F:\code-reference\impeccable\cli\engine\registry\antipatterns.mjs` 有 `ANTIPATTERNS` registry。
- Impeccable `F:\code-reference\impeccable\cli\engine\rules\checks.mjs` 有 `low-contrast`、`flat-type-hierarchy` 等检测逻辑，并包含 OKLCH 解析/转换。
- Impeccable `F:\code-reference\impeccable\skill\scripts\hook.mjs`、`hook-before-edit.mjs`、`hook-admin.mjs` 是 design hook 管理与执行入口。
- Impeccable `F:\code-reference\impeccable\plugin\skills\impeccable\SKILL.md:144` 记录 `detect.mjs --json` 作为真实当前信号，属于 skill 文档证据。

判定：

- **G 真缺口。** Reames 有 hook 承载面，但没有设计质量自动审计能力。
- 这不应进入核心 Agent loop；更适合作为官方可选 plugin/skill 使用 Reames hooks。

建议：

1. 先做 `reames-design-audit` 可选插件：监听 UI 文件写入后的 `PostToolUse`，输出低对比、扁平层级、过度 AI 风格等 findings。
2. 如果要阻断写入，只允许在用户显式启用后挂到 `PreToolUse`。
3. findings 进入 receipt/evidence，而不是直接改 prompt 或自动改 UI。

### 9.4 Hook 管理和信任审查产品化

Reames 证据：

- `internal/hook/hook.go`/`runner.go` 已有事件、scope、timeout、match、env、cwd、stdout 输出处理和 blocking 判定。
- `internal/control/slash.go` 已有 `/hooks list/trust` 入口。
- `internal/control/input.go` 已接入 `SessionStart` 上下文注入；`internal/agent/compact_test.go` 覆盖 `PreCompact` 输出注入。

参考证据：

- Codex `F:\code-reference\codex\codex-rs\app-server-protocol\schema\json\v2\HooksListResponse.json` 有 hook list 协议形态。
- Codex TUI snapshots 中有 hooks browser / startup hook review 相关快照，可作为 UI 行为参考。
- Claude Code `F:\code-reference\claude-code\CHANGELOG.md` 多处记录 hook 成熟度增强：hook duration、safe-mode、PreCompact block、PostToolUse 替换输出、大输出落盘、SessionStart reloadSkills/sessionTitle、Notification hook 等，属于 **D changelog 证据**。

判定：

- **P 产品化缺口。** Reames 已有 hook 系统；缺口不是“新增 hooks”，而是管理 UI、诊断、信任审查和执行契约细节。

建议：

1. Desktop/CLI 增加 hook browser：按 scope/source/event/match/timeout 展示。
2. `doctor` 报告 hook 配置错误、不可执行命令、超时和最近失败。
3. 统一 large stdout spill、duration_ms、safe-mode disable customizations 的契约。

### 9.5 Cron / Automation 调度语义硬化

Reames 证据：

- `internal/cron/cron.go` 已有 `Store`、`Job`、`ParseSchedule`、`Due`、`MarkRun`、`Ticker`。
- 但 `nextRun` 中 `KindCron` 当前返回 `time.Now().Add(1 * time.Hour)`，代码注释标明是 placeholder，尚不是真实 cron parser。
- `docs/TOOL_CONTRACT.md` 已记录 `cronjob` 工具，说明它已有工具契约入口。

参考证据：

- Hermes `F:\code-reference\Hermes\AGENTS.md:1051-1082` 记录 cron jobs/scheduler、3 分钟 hard interrupt、120s grace window、`.tick.lock`、`skip_memory=True`、独立 cron session header/footer，属于项目规则文档证据。
- Hermes `F:\code-reference\Hermes\AGENTS.md:1090-1124` 记录 Kanban dispatcher、heartbeat、失败自动 block、systemd 部署，属于项目规则文档证据。
- AgentArk `F:\code-reference\AgentArk\ARCHITECTURE.md:71-74` 记录 Sentinel/Pulse 的 follow-up、scheduled work、health findings，属于 **D 文档证据**。

判定：

- **P/G。** Reames 有 cron 基础，但真实 cron 表达式、missed-fire、并发锁、超时、独立 session、失败治理和可视化状态仍需补全。

建议：

1. 替换 `KindCron` placeholder 为真实 parser，并新增 timezone/missed-fire 策略。
2. 增加 per-job lock、hard timeout、grace shutdown、failure threshold 和 disabled reason。
3. 定时任务执行必须拥有独立 session/receipt，避免污染普通聊天记忆。

### 9.6 LSP Delta 诊断接入写入验证

Reames 证据：

- `internal/lsp/tool.go` 暴露 `lsp_definition`、`lsp_references`、`lsp_hover`、`lsp_diagnostics`。
- `internal/lsp/manager.go` 已有 `SnapshotBaseline` 和 `DeltaDiagnostics`，用于编辑前后只返回新增 diagnostics。
- `docs/TOOL_CONTRACT.md` 已列出 LSP 工具，并建议优先使用 `lsp_*` 做语言语义。

参考证据：

- Codex 参考方向中 LSP Delta 是变更验证和诊断基线的一部分；本轮主要以 Reames 源码为事实依据。

判定：

- **P 已有底层，缺产品闭环。** Reames 有 delta 诊断能力，但应进一步贯穿 write/apply_patch/edit 后验证、receipt、complete_step 和 Desktop 诊断面板。

建议：

1. 文件写入前自动 snapshot，写入后在相关语言文件上调用 delta diagnostics。
2. 新增 diagnostics receipt 字段：baseline count、new count、server、timeout、file list。
3. Desktop 显示“本次变更新增诊断”，不要只显示全量历史问题。

### 9.7 安装、部署、备份、低内存和供应链验证

Reames 证据：

- `scripts/install.sh` 支持 release/source、`--gateway`、`--channels feishu`、`--gateway-dir`、dry-run 和 checksum 验证。
- `scripts/install.ps1` 支持 Windows 安装和 checksum 验证。
- `internal/gatewayservice/service.go` 已有 systemd、launchd、Windows Scheduled Task service plan。
- `internal/cli/upgrade.go` 有 GitHub release 拉取、SHA256 校验和 rollback。
- `docs/audits/2026-07-09-gateway-service-lifecycle.md`、`docs/audits/2026-07-09-hermes-gateway-reference.md` 已记录 gateway service 生命周期和 Hermes 对照。

参考证据：

- AgentArk `F:\code-reference\AgentArk\README.md:124-138` 记录 Docker image quick start 和安装脚本，属于 **D 文档证据**。
- AgentArk `README.md:164-176` 记录 managed backups、Postgres/data/config backup、失败时 Pulse finding，属于 **D 文档证据**。
- AgentArk `README.md:178-200` 记录 low-memory override 和资源测量，属于 **D 文档证据**。
- AgentArk `F:\code-reference\AgentArk\VERIFY.md:10-127` 记录 cosign、GitHub artifact attestation、GHCR attestation 和 SBOM 验证，属于 **D 文档证据**。

判定：

- **P 产品化/运维缺口。** Reames 不是没有安装/升级；缺口是生产运维层：备份/恢复、低内存 profile、attestation/签名验证文档、自托管 server profile。

建议：

1. 增加 `reames-agent backup` / `restore` / `verify-install` 或等价脚本，覆盖 config、sessions、memory、bot/gateway state。
2. server/gateway 安装文档增加低内存 profile、systemd user lingering、日志轮转和恢复步骤。
3. Release 后补 provenance/attestation 验证说明；不要在没有签名基础设施时宣称供应链已完整可信。

### 9.8 Memory Review / Reflect / Provenance UI

Reames 证据：

- `internal/memory` 已有 remember/recall/forget store 基础。
- `internal/memorycompiler` 已有 runtime、compression、feedback、candidates、report 和大量测试。
- `docs/SESSION_MEMORY_RETRIEVAL.md` 明确 agent-initiated `remember`/`forget` 需要 fresh approval，忘记后仍保留 traceability。

参考证据：

- AgentArk `F:\code-reference\AgentArk\ARCHITECTURE.md:70,82` 记录 memory dedup、provenance、review、rollback 和 memory capture lifecycle，属于 **D 文档证据**。
- AgentArk `F:\code-reference\AgentArk\README.md:316-319` 记录 Memory/Reflect/Sentinel/Evolve 面板，属于 **D 文档证据**。
- Hermes `F:\code-reference\Hermes\AGENTS.md:1023-1038` 记录 curator review loop、backup、archive/restore/prune/rollback、只处理 `created_by: "agent"` provenance 的 skills，属于项目规则文档证据。
- MiMo `F:\code-reference\MiMo-Code\README.md:253-254` 记录 `/dream`、`/distill`，属于 **D 文档证据**。

判定：

- **P 产品化缺口。** Reames 记忆引擎基础不弱；缺 review queue、provenance/rollback UI、日/周/月 Reflect 总览和从经验中提炼技能/流程的人工审查闭环。

建议：

1. 增加 Memory Review 视图：pending/accepted/archived/forgotten、source session、approval id、created_by。
2. 增加 Reflect 报告：最近任务、失败模式、重复流程、可提炼 skill 候选。
3. 自动提炼只能进入候选和 review queue，不能自动修改核心 prompt、policy 或已启用 skill。

### 9.9 二次审查后新增优先级建议

以下建议补充第 7 节，不替代已有任务：

1. **Cron parser + job isolation**：修正 `KindCron` placeholder，补 missed-fire、lock、timeout、独立 session 和 receipt。
2. **LSP delta write verification**：把 `SnapshotBaseline`/`DeltaDiagnostics` 接入文件写入和 Desktop 诊断显示。
3. **Hook browser/doctor**：产品化已有 hook 系统，优先做可见性和信任审查。
4. **Workflow runtime API**：在 AutoResearch 之外设计通用 workflow 状态、transcript、structure API；worktree 隔离单独 gate。
5. **Server backup/restore/verify**：补自托管部署的备份恢复、低内存 profile、release provenance 说明。
6. **Voice optional plugin**：语音能力走可选插件，不进入默认核心路径。
7. **Design QA optional plugin**：基于 hooks 接入 UI 反模式检测，不默认阻断写入。
8. **Memory review/Reflect**：在现有 memory/compiler 上补 UI 和人工审查闭环。

### 9.10 二次审查修正约束

- 不能写“Reames 缺少 hooks/cron/LSP/memory/install”，只能写对应产品化或硬化缺口。
- 不能把 MiMo workflow 与 Reames AutoResearch 混为一谈：前者是通用动态 workflow/runtime/worktree，后者是研究目标的 host-managed runtime。
- 不能把 AgentArk Docker Compose/attestation/backup 作为 Reames 已有事实；只能作为部署运维参考。
- Voice、Design QA、Office/Media 应优先通过 plugin/skill/MCP 扩展，不应扩大核心 agent loop。
