# Reames Agent 源流与参考项目治理

> 状态：当前项目来源、上游跟踪和参考吸收的权威说明
> 更新：2026-07-20

## 1. 项目源流

Reames Agent 的来源关系不是“多个项目平级拼装”，而是清晰的三级上游加前身结构：

```text
esengine/DeepSeek-Reasonix (main-v2)
        │ 一级：主源码底座、DeepSeek 原生能力、持续跟踪的 primary upstream
        ▼
Reames Agent
        │ 独立产品、独立品牌、单一 Go/Wails runtime
        ├── OpenAI Codex / Claude Code：二级战略代码上游
        │     GPT/OpenAI 与 Claude/Anthropic 原生协议、产品/runtime 能力
        ├── F:\code-reference\其他项目：三级机制、体验和测试思想参考
        └── F:\Reames-Lite：项目前身、产品契约和历史经验
```

### DeepSeek Reasonix

- 仓库：`https://github.com/esengine/DeepSeek-Reasonix`
- 本地镜像：`F:\code-reference\DeepSeek-Reasonix`
- 跟踪分支：`main-v2`
- Reames Agent 初始导入基线：`07c65c22`
- 角色：Go Agent runtime、provider/cache、tool、Desktop、Serve、插件与安全机制的源码底座。

Reasonix 是持续跟进的主上游，不是一次性参考。但 Reames Agent 已有品牌、产品和架构改造，不能直接合并上游主分支；应按功能批次审查和移植。

### OpenAI Codex 与 Claude Code

- 本地镜像：`F:\code-reference\codex`、`F:\code-reference\claude-code`；
- 角色：二级 `strategic-code-upstream`，不是普通 UX/机制参考；
- Codex 对应 GPT/OpenAI 原生 Responses、reasoning、tool/stream、多模态、App-Server/headless、插件和
  Browser/CDP 等代码级能力；
- Claude Code 对应 Claude/Anthropic Messages、thinking、tool/vision/cache、插件/Skill/Hook 与会话体验；
- “原生支持”不能由一个 OpenAI-compatible/Anthropic-compatible endpoint 或模型名称证明：必须分别验证官方
  wire、流式事件、reasoning/thinking replay、usage/cache、tool/vision、错误语义与相关 Agent/runtime 能力；
- 两者有新提交时 Upstream Watch 必须给出 `review-required`，启用路径级 diff，并进行逐提交/逐文件能力
  审查；不能只根据 CHANGELOG、release notes 或 GitHub 页面决定 Reames 是否“已跟进”。

二级战略不表示成为 Reames 的第二/第三源码底座，也不授权整体 vendor Rust/TypeScript runtime、品牌、
登录、托管服务或 marketplace。适用能力必须按 Reames 的 Go/Wails、Controller、permission、sandbox、
credential、evidence 和 cache-stable schema 重构。

### `F:\code-reference`

除 Codex/Claude 二级战略上游外，这些仓库是“机制采矿场”，不是 vendor 目录：

- 可以吸收算法、交互模式、协议、测试案例和小型独立机制；
- 不整套复制另一个项目的 runtime、UI 或依赖体系；
- 每次吸收都要先证明 Reames Agent 当前确有缺口；
- 实现应落入 Reames Agent 的统一控制面和测试体系。

初始迁移中曾临时保留的 Hermes/Python/Electron/TUI/插件/测试快照已于
2026-07-17 从当前树删除；Git 历史负责追溯，`F:\code-reference\Hermes` 负责后续机制研究。
任何参考项目都不得再以整套 runtime、UI、依赖体系或测试树形式回到主分支。

### Reames Lite

`F:\Reames-Lite` 是新项目的前身。旧项目在桌面产品化时遇到工程瓶颈，因此转向以 Reasonix 的 Go/Wails 底座建立新项目。

它继续提供：

- 公共客户端边界和传输隔离思想；
- cache-first、metadata 不污染 prompt 的约束；
- 压缩、记忆、事件和工具契约；
- 中文产品经验以及旧实现踩坑记录。

它不再承担：

- 新项目主线开发；
- 桌面 Shell 或 runtime 底座；
- Provider/cache 的直接实现来源。

## 2. 产品北极星

Reames Agent 的长期目标是一个**以编程能力为最先成熟核心的全能 Agent**。

“全能”不等于把所有参考项目功能堆进同一个进程，而是通过一个可组合的 Agent 内核覆盖：

- 软件开发：理解、修改、验证和交付代码；
- 研究与知识工作：检索、阅读、归纳、证据追踪；
- 文件与数据工作：本地文件、结构化数据和多媒体上下文；
- 自动化：后台任务、Goal Loop、定时任务和长期执行；
- 记忆：项目记忆、用户偏好和可控的长期知识；
- 多入口协作：Desktop、CLI、Web、API 和 IM 渠道；
- 能力扩展：Tool、MCP、Plugin、Skill、Hook、LSP；
- 安全治理：权限、沙箱、密钥保护、审计、检查点和恢复。

所有能力必须汇入同一套会话、事件、权限和证据模型。若一个新入口需要复制 Agent loop，它就不符合“全能 Agent”的架构目标。

## 3. 参考项目职责

| 项目 | 主要吸收方向 |
|---|---|
| DeepSeek Reasonix | 主底座；runtime、provider/cache、Desktop、Serve、安全修复 |
| Hermes | 多渠道、远程部署、消息 envelope、错误分类与渠道运维 |
| OpenAI Codex | 二级战略代码上游；GPT/Responses、App Server、审批、线程/任务控制、插件、Hook、LSP、Browser/CDP |
| MiMo Code | 设计 token、OKLCH、Hook 热更新和任务编排 |
| Impeccable | 设计规则、反模式检查和跨平台设计约束 |
| Scream Code | Goal Loop、Storm Breaker、主题与频道会话纪律 |
| AgentArk | Intent、安全边界、密钥、沙箱与 replay gate |
| Claude Code | 二级战略代码上游；Claude/Messages、Thinking、工具/视觉/缓存、Plugin/Skill/Hook 生态 |
| Kimi Code | 桌面 Shell、浏览器通知、TUI 与 Provider 错误体验 |
| Grok Build | 权限/沙箱、持久会话与子代理、TUI/ACP/headless 交互及终端可靠性 |
| Reames Lite | 前身契约、cache/metadata 约束、压缩、记忆和中文体验 |

## 4. 上游更新规则

1. 参考仓库可先执行 `git pull --ff-only`；工作树不干净时不得覆盖本地研究。
2. 运行：

   ```powershell
   python scripts/check_upstreams.py --out-dir artifacts/upstream-watch
   ```

   完整运维、Issue 生命周期和接受版本流程见 `docs/upstreams/README.md`。

3. Reasonix 差异按以下顺序审查：
   - security / secret / sandbox；
   - provider / cache / stream；
   - agent / control / persistence；
   - Desktop bridge / recovery；
   - UI 与文档。
4. Codex 与 Claude Code 每次变化都做代码级能力审查：先原生模型协议，再 Agent/runtime、插件/headless、
   Browser/交互和 bug-fix；不能降级为普通候选机制。
5. 其余项目只形成候选机制，不自动产生移植任务。
6. 每个移植批次必须记录：来源提交、缺口、Reames 适配、测试和缓存影响。
7. 不使用批量 merge 覆盖 Reames 的品牌、中文体验、公共边界和产品方向。

## 5. 2026-07-09 上游快照

本次已安全更新所有干净参考仓库。Reames Lite 存在未跟踪调研文件，保留现场，未执行 pull。

Reasonix 从初始跟踪点之后新增 133 个提交，值得优先审查的候选包括：

- tool output、历史会话和诊断包的密钥脱敏；
- 敏感环境变量过滤与敏感文件保护；
- Agent 配置文件写入的强制人工审批；
- ACP 客户端文件系统/终端能力；
- 工具工作区约束与客户端 I/O；
- Desktop 滚动意图、恢复副本、上下文窗口环和审批交互。

具体差异报告位于 `artifacts/upstream-watch/upstream-report.md`。

## 6. 2026-07-17 上游再同步

Reasonix `main-v2` 已审查至 `099879592742ddeb25b312347b4c37316e8b76f9`，相对原 reviewed
点累计 664 个提交（490 个非 merge）。本轮直接采用 MCP schema 隔离、保存凭据的环境/读取
保护、credential-free cache identity、Anthropic usage 合并、MiMo schema dialect，以及身份漂移
失败的类型化 re-verification 投影；Reames
适配、测试、拒绝项和后续门槛见
[`audits/2026-07-17-reasonix-upstream-sync.md`](audits/2026-07-17-reasonix-upstream-sync.md)。

本轮同时安全快进并审查 Hermes、Codex、MiMo、Impeccable、Scream Code、AgentArk、Claude Code
和 Kimi Code 的干净本地参考仓库。只形成三个新的路线图信号：

1. Reasonix 的 MCP identity receipt/launcher lock 已按 Reames control/security 边界完成 M5 P0，
   见 [`audits/2026-07-17-m5-mcp-identity-trust.md`](audits/2026-07-17-m5-mcp-identity-trust.md)；
2. Reasonix + MiMo 的 workspace lease/worktree 已按 Reames Controller、durable child journal 与 Windows 路径边界关闭 M4 持续加固 P1，见 `audits/2026-07-17-m4-writer-worktree-isolation.md`；
3. Reasonix offline Guard/Safe Mode P2 已按“复用现有恢复事务、不建立第二套 Agent/runtime”关闭，
   见 `audits/2026-07-17-p2-offline-guard-safe-mode.md`。

其他 UI、主题、发布、遥测、Python/Electron runtime 和参考项目依赖体系不进入 Reames 主树。
正式 reviewed SHA 保存在 `docs/upstreams/upstreams.lock.json`；`artifacts/upstream-watch/` 仍只
是本地分析输出，不提交。

同日最终 deep scan 将 Hermes 补审至 `bd0021233734`。其中 stored-id rotation 只适用于其压缩后
fork SessionDB continuation 的模型；PTY attach-token session scope 只适用于其 keep-alive registry；
basic-auth deny-list 修复只适用于其可插拔 dashboard auth provider；`_HERMES_GATEWAY` 清理只适用于
dashboard 在 gateway 进程内再拉起 gateway action 的递归模型。
Codex 补审至 `315195492c80`。
Hermes 的统一 worktree dialog 仅补强现有 P1 交付 UX；其 transcript 优化在 Reames hot/warm/cold
分页与长历史 benchmark 中已有等价或更强机制，compute host 不进入主树。Reames compaction 原地
重写同一 session path，且 session path recovery 已有 metadata/hydration 对账，因此不复制 Hermes 的
stored-id route/selection atom。Reames serve 的 controller/session 切换已有 `bindMu` + lease，password
auth 是编译内置 `authGate`，因此也不复制 Hermes 的 PTY registry key 或 plugin deny-list 修复。Codex 的 SQLite
migration writer-slot、Bedrock custom transport、approval payload structs、memory provenance 与
thread originator 变化均完成分类：除通用回归信号外，没有证据支持在本批新增产品路线或依赖。

## 7. 2026-07-17 P2 补审

Reasonix 已从 `099879592742` 补审到 `c966d0279629`。直接采用 `dae65e25` 的 stalled error-body
deadline、verified snapshot fast path、damaged event-log salvage 和 Desktop last-click-wins；
`f590a66e` 的 model picker 问题在 Reames 已有等价修复。`8f2c209a` 不用于放宽已关闭的
identity-bound MCP trust，`1bd5f04d` 保留为体验候选，`c966d027` 删除 Memory Compiler 的
产品决策不直接跟随。

参考项目按本批相关性补审至 Hermes `ef9e0c98f5c2`、Codex `e0ac6d0ec9ee`、MiMo
`28a0ced5e8e9`、Claude Code `67f390c9a0b1`、Kimi Code `7d393b56fb32` 与 Scream Code
`b37627ad8e3b`。Codex installed-app runtime projection 保留为 P3 Recovery Center 的只读运行态
投影参考；后续 realtime handoff、executor capability discovery、multi-exec workspace isolation 与测试
环境隔离只作为 token-economy/P3 和回归信号，Reames 已有父控制面/worktree 边界，不复制第二套
thread/exec-server 模型。Hermes 后续 auxiliary runtime turn/context 隔离、fallback 同步和 FTS 自愈
不适用于 Reames 当前 Go runtime/非 FTS 存储；Scream Code 新增仅为 README 下载链接。以上均无本批
代码采用项，MiMo 与 Kimi 的既有信号也已完成适用性分类。

本次明确使用逐项 `--accept`，没有使用 `--accept-all`。权威 SHA 以
`docs/upstreams/upstreams.lock.json` 为准，详细采用与拒绝理由见 P2 审计。

## 8. 2026-07-18 Grok Build 首次纳入

SpaceXAI 官方 `xai-org/grok-build` 于 2026-07-14 公开，当前 GitHub `main` 是从内部 monorepo
周期同步的 Rust 快照，不具备逐个内部提交的完整公开历史。Reames 将它作为
`security-interaction-reference` 跟踪，优先观察权限解析、shell 门禁、OS sandbox、持久会话、
subagent/worktree、终端交互、headless 和 ACP；不把它升级为第二主上游，也不引入 Rust runtime、
xAI 登录/模型 endpoint、遥测、在线 memory、managed policy 或 marketplace。

首次源码分类固定在 `98c3b2438aa922fbbe6178a5c0a4c48f85edc8ce`。Plan Mode 已公开说明 shell
重定向和可写 subagent 可越过父级 edit gate，因此只能作为负面回归参考，不能降低 Reames 的
planmode/permission 边界；plugin `require_sha` 也不替代 Reames 已有 TUF、provenance 和 fresh-human
审批链。完整 intake 见
[`audits/2026-07-18-grok-build-reference-intake.md`](audits/2026-07-18-grok-build-reference-intake.md)。

## 9. 2026-07-18 全参考最新版冻结

电脑清理前的最终代码级冻结已完成，逐项接受而未使用 `--accept-all`。权威 reviewed SHA 为：

| 项目 | reviewed SHA |
|---|---|
| DeepSeek Reasonix | `40ef98de92a30a273ee582ec682ab338483109d2` |
| Hermes | `4c96172d9bee8542a356610802b9aabc1419f650` |
| Codex | `56395bddaf26eb2829387ca6a417bf9128e5b239` |
| MiMo Code | `72e9002e48a71b383b8851b23d65e30c692d68fb` |
| Impeccable | `8967edc988ee146823bca3c51fcf51296e9dec18` |
| Scream Code | `6474e33ad13ffcf11c8eb8a1691af943fe707b2d` |
| AgentArk | `63985cf819d1760f50f2a5c0dc11d82815e74623` |
| Claude Code | `07dcb0e13580b21174ff1bf6a7e1d5ead3b61d60` |
| Kimi Code | `3086e4703992fbbe7a41379405ee243713ad9ced` |
| Grok Build | `98c3b2438aa922fbbe6178a5c0a4c48f85edc8ce` |
| awesome-design-md | `664b3e78fd1a298ba11973822da988483256d4b4` |

Reasonix 仍是唯一主源码上游；Codex 与 Claude Code 是二级战略代码上游，其余项目只提供机制信号。完整差距、采用、已有等价、延后和拒绝结论见
[`audits/2026-07-18-upstream-reference-freeze.md`](audits/2026-07-18-upstream-reference-freeze.md)，
Reasonix 最新 5 提交的机器账本和 required-area 关闭证据位于 `docs/upstreams/reviews/`。此冻结点之后
只审 `docs/upstreams/upstreams.lock.json` 到新 `latest` 的差异。

## 10. 2026-07-19 P7 增量接受

本轮继续只审 lock → latest，没有重新解释旧区间，也没有使用 `--accept-all`。新的 reviewed SHA：

| 项目 | reviewed SHA |
|---|---|
| DeepSeek Reasonix | `2335d0df9ea4029108ed965f76c2efff30fe6cf4` |
| Hermes | `581e92e42c89645b5dacf8263abebb15348c791b` |
| OpenAI Codex | `b8b61bc692517adcd18622df260f2ddd80635122` |
| MiMo Code | `d48888f7b1d22e830ee5c10faf2b9e455f3cd881` |
| Scream Code | `53fa61b2a9b1bbc1914949f328928fe8f03b16d2` |
| Kimi Code | `4f3c7240c4adc7c748e536bf578e468c1b5bcd7b` |

Reasonix 的 regional typography 与 Fleet 未整体移植：字体偏好需要独立跨平台可访问性合同；Fleet 的
共享工作区 write claim 不替代 Reames writer worktree、delivery transaction、durable effects 和整树预算。
Codex 的压缩 rollout inventory 因 Reames 当前没有 compressed session/SQLite thread index 而不直接采用，
但 logical path、plain sibling、corruption/temp-file 语义进入未来存储回归合同。Hermes 的 systemd
`READY/WATCHDOG/STOPPING` 与 bounded shutdown 以纯 Go 机制采用；Python/Electron、billing/subscription
和第二 runtime 继续拒绝；其 full-load/resize zoom 修复缺少 WebView2 同构证据，保留为 native smoke
信号而不复制 Electron 事件。MiMo 的 bundle 重命名、Scream Goal/WolfPack/Provider/多代理帮助和 Kimi Web
前台默认分别记录为无缺口、已有更强边界或 UX 候选。完整审计见
[`audits/2026-07-19-p7-upstream-gateway-watchdog.md`](audits/2026-07-19-p7-upstream-gateway-watchdog.md)。

用户要求的 GPT/Claude、Codex 类插件/headless 和 CDP 浏览器控制已进入 P8/P9/P10，但“参考 Codex”
不改变来源治理：只移植 MIT/兼容许可下适用机制，复用 Reames Controller、permission、sandbox、
credential 和 evidence，不整体 vendor Codex runtime 或接入未经治理的 marketplace。

## 11. 2026-07-19 P8 战略上游与错误分类增量

P8 继续按代码路径和真实 wire 审查，而不是按 README 或模型名猜测协议。当前权威 reviewed SHA 为：

| 项目 | reviewed SHA | 本批结论 |
|---|---|---|
| DeepSeek Reasonix | `a46fc6f47a00ffffeaee6184c4748cac6cc4ae7d` | 一级主源码上游；最新可靠性批已代码级审查并采用适用修复 |
| OpenAI Codex | `0fb559f0f6e231a88ac02ea002d3ecd248e2b515` | Responses、Realtime、code-mode/App-Server 与 Browser/CDP 战略矩阵已推进；本轮无新增 |
| Claude Code | `015170d3fd84fb57ef4685a64b673fadd0690dc1` | 最新只有 changelog/feed；不从发布说明反推 Messages/runtime parity |
| Hermes | `3a6e40b297d505cf42b6c35b94e6b0efd967527e` | durable delivery、Telegram watchdog、turn lease、配置与插件信号完成机制级分类 |
| Grok Build | `7cfcb20d2b50b0d18801a6c0af2e401c0e060894` | 采用无歧义 MCP 名称合同，其余保持机制参考 |

Hermes `bf391030..862b1b37` 的 advisory 会同时出现 “empty response” 和 `max_tokens`。Reames 的共享
classifier 原先也会因匹配顺序返回 `ShouldCompact=true`；本批将窄化的 empty-response 形状优先分类为
可重试 server error，并用反向测试保证真实 `max_tokens > context_window` 仍返回 compaction 建议。当前
Agent 自动压缩由 usage 阈值驱动，尚未消费该分类合同，所以这里只证明分类加固，不声称复现了运行时
压缩事故。该采用落在统一 Go Provider 层，没有引回 Python runtime。

Codex `35eaf3ff..312caf17` 只增加 Realtime V3/Frameless Bidi 的 role-bearing `initial_items`，并以
128 项、单项/总计 8192 估算 token 为边界；Reames 当前 P8 是 HTTP Responses transport，因此该变化
进入 P9 Realtime/App-Server，不以普通 Responses history 冒充采用。Hermes `862b1b37..7a43ab04` 的
Desktop gateway reconnect 在其双 runtime/REST+RPC 投影中保留 pending turn；Reames Desktop 与 Controller
同进程、已有 canonical event log、session transaction、hydration 和 pending approval replay，没有同构
双权威补丁。其 Discord durable cursor/final-delivery gate 是 M6 明确缺口；`/model --once` 是 P9 候选；
computer-use 的 verify→foreground escalation、按 session+delivery-mode 审批和 dead-driver reconnect 是
P10 验收输入。以上均按机制边界登记，不复制 Python/Electron runtime。

## 12. 2026-07-19 Reasonix `a46fc6f` 与全参考再冻结

本轮再次把全部本地镜像安全快进并运行深度 Upstream Watch。接受过程逐项执行
`--accept reasonix`、`--accept hermes`、`--accept mimo`、`--accept scream-code`、
`--accept claude-code`、`--accept kimi-code`，没有使用 `--accept-all`。最终权威 SHA
始终以 `docs/upstreams/upstreams.lock.json` 为准。

Reasonix `2335d0df..a46fc6f` 共 21 个提交、16 个非 merge 提交和 106 个变化文件。
Reames 采用测试 home/cache/temp 隔离、Windows batch Hook、有界 session save lock 与
shutdown recovery、每模型 context window、本地 Mermaid/兼容/线性脱敏可靠性、
Windows-safe session filename 和 native maximized resize cursor 修复。取消轮次只读
partial display、PRIMARY/tmux 中键粘贴和 conversation width 明确延后；远程 crash
upload、遥测与当前依赖不存在的 TanStack 私有 API 明确拒绝。完整代码审计与机器账本：

- `audits/2026-07-19-reasonix-2335d0d-a46fc6f.md`
- `upstreams/reviews/reasonix-generation-2335d0d-a46fc6f.json`
- `upstreams/reviews/reasonix-current.json`

Codex 本轮无新增。Claude Code 的唯一变化是 2.1.215 changelog/feed，表示 `/verify` 与
`/code-review` 改为显式调用；没有可公开的 Messages、thinking、cache、tool/vision 或
插件 runtime 代码变化，因此只登记产品信号。Hermes 的 outbound final-response
obligation 暴露了 Reames 下一项 M6 durability 缺口：当前 inbound ledger 能阻止 cursor
在发送失败时推进，但进程在平台 ACK 前崩溃后仍可能重跑 Agent；后续应持久化有界答复
obligation，并对 mid-send 歧义显示“可能重复”标记。MiMo Skill BM25、Scream 全项目 bug
audit、Kimi 多实例 Web/分片 read model/stat-lstat 只作为 P9/P10 或可靠性回归输入。

完整跨参考结论见
[`audits/2026-07-19-upstream-a46fc6f-reference-delta.md`](audits/2026-07-19-upstream-a46fc6f-reference-delta.md)。

## 13. 2026-07-19 Reasonix `65fcd465` 增量冻结

Reasonix `a46fc6f..65fcd465` 共 5 个非 merge 提交、141 个变化文件。代码级结论如下：

- 采用 `00af3a03`：LongCat-2.0 context window 改为 1,048,576，并只迁移未修改的官方预设；
- 采用 `d50a9888`：保持 Wails v2.12，回移 Linux WebKit/JavaScriptCore `SA_ONSTACK` 启动修复；
  NVIDIA DMA-BUF 广义降级只在 Safe Mode 启用且尊重用户环境；
- 采用 `0a54d504`：会话图片/PDF 分页导出、外部资源隔离、独占多文件 staging、碰撞拒绝、失败回滚与可见反馈；
- `a144e55e` 的 pane opacity 会扩大已关闭的 Theme Pack v1 schema，延后；其编辑器保存/激活竞态在 Reames
  当前受控主题流程中不具同构路径；
- `65fcd465` Remote SSH 是 109 文件的新安全面，进入 P11；不整套复制，不新增第二套 Controller/Agent/runtime。

同期 Hermes `3a6e40b2..36f2a966` 只作为 Capabilities 搜索/提取分离和设置 deep-link 机制参考；voice provider
不适用。Kimi `a3e773f9..df689955` 只增加远端 thinking-effort telemetry，因 Reames 不建设自有遥测服务而不采用；
未来只可进入本地诊断/evidence。Codex `0fb559f0` 与 Claude Code `015170d3` 无新增。

完整证据见：

- `audits/2026-07-19-reasonix-a46fc6f-65fcd46.md`
- `upstreams/reviews/reasonix-generation-a46fc6f-65fcd46.json`
- `upstreams/reviews/reasonix-current.json`

## 14. 2026-07-19 Reasonix `8bb0e549` 最终增量

Reasonix `65fcd465..8bb0e549` 只有 1 个非 merge 提交和 4 个 Desktop frontend 文件，修复设置刷新覆盖活动
Theme Pack 的状态。Reames 的 `themePackRuntime.effectiveStyle/apply` 已让所有 `applyTheme` 调用同时更新配置基底、
保留活动包的 effective style 并重投影受控 tokens；`applyThemePack(null)` 会恢复最新配置基底。因此该提交为
`existing-equivalent`，生产代码不复制，只增加“活动包期间刷新基础 theme/style，清包后恢复最新基底”的显式回归。

同期 Scream Code `c6b24f60..22a2adaf` 将动画与 footer timer 改为 component-scoped render；Reames Desktop 使用
React state 与局部组件更新，没有同构的全屏 TUI `requestRender` 热路径，只登记为渲染性能机制信号。Reasonix、
Scream 均逐项 `--accept`，未使用 `--accept-all`。完整证据见：

- `audits/2026-07-19-reasonix-65fcd46-8bb0e54.md`
- `upstreams/reviews/reasonix-generation-65fcd46-8bb0e54.json`
- `upstreams/reviews/reasonix-current.json`

## 15. 2026-07-20 Hermes obligation 机制采用

Hermes 在 2026-07-19 增量中暴露的 outbound final-response obligation 缺口已完成机制级采用。Reames
没有复制 Hermes 的 Python/Electron Gateway 或第二套 runtime，而是在现有 Go `internal/bot` delivery ledger、
render 和 Gateway 中加入 schema-v2 最终文本 obligation、单 writer OS 锁、发送前 `attempting`、逐分片 ACK、
最后 ACK 与 inbound cursor 原子结算，以及 mid-send 歧义的可见“可能重复”标记。重复 inbound 直接恢复原答复，
不创建 Controller、不调用 Provider。

该采用不改变上游层级：Reasonix 仍是唯一一级源码上游；Codex/Claude 仍是二级战略代码上游；Hermes 仍只提供
Gateway/运维机制信号。完整边界与测试见
`audits/2026-07-20-m6-outbound-final-response-obligation.md`。

## 16. 2026-07-20 Reasonix 与战略/机制参考再冻结

本轮深扫与代码审查固定到：

| 层级 | 项目 | reviewed SHA | 结论 |
|---|---|---|---|
| 一级 | DeepSeek Reasonix | `2301e24827bf62c7584f34c4f541c432dd4f6e0b` | 6 个提交逐项审查；采用 Provider env、MCP stdio/lifecycle、中断恢复和 WebKit focus，Remote SSH 延后 P11 |
| 二级 | OpenAI Codex | `3e2f79727a4e8ddfc8e3acb838d496b121094b9e` | 6 个 TUI 内存/side-thread 提交已代码级分类；无 Responses/App-Server/Browser wire 增量 |
| 二级 | Claude Code | `015170d3fd84fb57ef4685a64b673fadd0690dc1` | 无新公开源码，不从 changelog 伪造 parity |
| 三级 | Hermes | `299e409f15aa5615a8a64be488580be92cda351e` | cron profile 进入 M7；subagent live view 只从 Reames canonical child transcript 投影 |
| 三级 | Grok Build | `ba76b0a683fa52e4e60685017b85905451be17bc` | Stop gate、session import、permission deny budget 等只形成 P9/安全机制信号 |

Reasonix 的 MCP 生命周期采用保留 Reames 更强的 identity receipt、launcher pin、provenance、destructive
approval 和所有 visible/detached Controller reservation；不跟随上游删除 trust workflow。Codex/Claude 的
二级地位继续要求未来提交进行真实 code/wire review，其余项目仍无版本 parity 义务。

完整证据：

- `audits/2026-07-20-reasonix-8bb0e54-2301e24.md`
- `upstreams/reviews/reasonix-generation-8bb0e54-2301e24.json`
- `audits/2026-07-20-upstream-strategic-reference-delta.md`

## 17. 2026-07-20 Codex / Hermes 提交前增量

最终提交前深扫发现 Codex `3e2f7972..7844386e` 和 Hermes `299e409f..1b17015f`。Codex 前 6 个提交为
TUI Markdown/visualization context/command lifecycle/replay/diff 性能，最后一个修复多 Agent exec completion
backfill identity：Reames 的 typed `ToolProgress`/`ToolResult` 与中断 `LocalOnly` 已覆盖“输出不等于完成”；
CLI finalized Markdown 已提交为 ANSI transcript，单 diff 也不深克隆 Go string backing bytes，child sink 不转发
`TurnDone`。P9 明确禁止把 raw audio/progress/output delta 写入 replay store，且 completion backfill 必须同时
绑定 primary thread+turn；另需 benchmark Desktop 无界 live emitter，动态 Mermaid/visualization 不做静态缓存。

Hermes 的 cron profile 与 Electron `simple-git` spaced-path/session-color 修复对当前单 home Go/Wails runtime 非同构；
Kimi adaptive thinking 则暴露了真实同构缺口。Reames 已在不放宽 Claude 边界的前提下允许 Kimi/Moonshot
Anthropic stream 捕获的无签名 thinking block 续轮回放，通用 `ReasoningContent` 仍不能跨 Provider 伪装。
Hermes 隔离 spawn/settle/无网络噪声的 Desktop perf 方法进入 P9/P10 benchmark 合同，不复制 Electron harness。

旧的未绑定 `--accept` 在 Codex 移动期间暴露 TOCTOU，本批已改为强制
`--accept-revision ID=FULL_SHA`，并禁用 `--accept`/`--accept-all`/`--update-lock`。Codex 与 Hermes 最终
reviewed SHA 为 `7844386e3de08febd13075eaaaf0e6f9dbe52c58` 和
`1b17015f7a8d0c0d68b1f08aa389538e7fd172e3`；精确 SHA 接受后 11/11 无变化。完整证据见
`audits/2026-07-20-codex-hermes-late-delta.md`。
