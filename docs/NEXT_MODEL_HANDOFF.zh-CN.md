# Reames Agent 新会话无痛交接

> 日期：2026-07-20
>
> 仓库：`F:\reames-agent`
>
> 分支：`main`，不保留第二开发分支
>
> 用途：即使 Codex 会话、`F:\code-reference` 或本机缓存在电脑清理后丢失，新会话也只凭 Git
> 仓库恢复当前边界、证据和下一步。

## 1. 权威信息顺序

发生冲突时按以下顺序判断，不依赖旧聊天记录：

1. `git status --short --branch`、`git log -1 --oneline --decorate` 与远端 Actions；
2. `docs/PROJECT.md`：产品方向和当前事实；
3. `docs/DEVELOPMENT_PLAN.md`：执行顺序和关闭门槛；
4. `docs/REFERENCE_GOVERNANCE.md`、`docs/upstreams/upstreams.lock.json`：上游来源和 reviewed SHA；
5. `docs/audits/`：完成声明、限制和实际证据。

冻结提交就是“包含本文件最终版本的 `main` 提交”。提交不能可靠地自引用自身 SHA；新会话执行
`git log -1 --format="%H %s"` 即可取得精确值。不要用聊天摘要中的短 SHA 覆盖 Git。

## 2. 用户目标和工作节奏

持续把 Reames Agent 推进到高可信可交付状态；Reasonix 是唯一一级主源码上游，持续跟进 DeepSeek
原生协议、Agent/runtime、Desktop/CLI 和可靠性修复；Codex/Claude Code 是二级战略代码上游，分别
跟进 GPT/OpenAI 与 Claude/Anthropic 的原生协议和代码级 Agent 能力；其他项目只吸收适用机制。
“原生”不能用兼容 endpoint 或模型名称代替，必须分别验证官方 wire、stream、reasoning/thinking replay、
usage/cache、tool/vision、错误与 runtime 行为。
每个大批同步实现、测试、文档和证据；充分本地验证后集中 commit/push，避免碎片 push 浪费 CI。

永久边界：

- 不恢复启动、metrics、crash、performance 或用户使用数据的远端上传；
- 不使用用户服务器承担 Reames 遥测或反馈接收；反馈默认本地落盘、由用户显式导出；
- 不整体复制 Python/Electron/Rust runtime、品牌站点、生产 endpoint、xAI auth、online memory、
  managed policy、marketplace 或上游发布权限；
- Controller 保持传输无关，system prompt/tool schema 保持缓存稳定；
- Safe Mode、permission、sandbox、evidence、writer worktree 和 fresh-human 边界不能因参考项目而放宽。

## 3. 当前项目状态

- M0、M1、M2、M3、M4 已按路线图关闭。
- M5 所有仓库内、clean-clone 和 CI/CodeQL 可验证事项已关闭；真实运营 registry 的 endpoint、人员/HSM
  密钥仪式、轮换/compromise drill 与独立 DSSE/SLSA policy verifier 保持 `external-blocked`。
- P1 writer worktree、P2 Offline Guard/Safe Mode、P3 Recovery Center、P4 Reasonix 代际 parity、
  P5 受控 Theme Pack 已关闭。P5 最近远端证据：CI `29635818559`、CodeQL `29635818555`、
  Desktop candidate `29635823162` 全绿。
- P6 已在本批关闭：全部 11 个上游/参考仓库更新、代码级分类并冻结；Reasonix 最新 CLI 缺口和
  Hermes BOM 信号已适配。
- P7 已完成仓库内实现与代码级审计：Reasonix 最新 Fleet/区域字体没有机械复制；Codex compressed
  rollout inventory 已完成战略审查；Hermes 的 systemd
  `READY/WATCHDOG/STOPPING`、adapter-health gate 和 bounded shutdown 已按 Go runtime 采用。最终完成
  声明已由后续 P8 集中交付覆盖验证。
- P8 已由 `a58f7691` 集中交付，CI `29663429224` 8/8、CodeQL `29663429258` 3/3 全绿；官方
  OpenAI Responses/GPT 与 Anthropic Messages/Claude 的仓库内原生协议门槛已关闭，真实公网 API 回环
  仍是 `external-blocked`。
- M6 durable channel recovery core 已实现持久 claim/去重、原消息身份、每频道连续前缀 cursor、最终发送
  门禁、全局补扫上限和隐私安全状态投影。Telegram 已升级为正式 long-poll Adapter。schema-v2 outbound
  final-response obligation 也已关闭：最终文本发送前持久化，平台 ACK 后逐分片提交，最后 ACK 与 inbound
  claims/cursor 原子结算；冷启动直接恢复原答复，不重跑模型，ACK 歧义显示“可能重复”。真实渠道历史
  分页/掉线回环仍未完成。
- Reasonix 最新 `8bb0e549..2301e248` 已完成代码级采用：数字开头 Provider env、MCP stdio reply queue、
  Desktop 全 MCP/插件 lifecycle admission 与 visible/detached Controller reservation、中断轮次 LocalOnly
  恢复和 WebKit recorder focus 已落地；Remote SSH UX/host-key 保持 P11。Codex 二级战略审至
  `678157ac`：除前批 TUI/command/replay/exec identity 外，新增 paginated name canonical store、动态 cell 重测和
  fresh/fork/resume subagent backfill 请求边界；Claude 无新增。Hermes 审至 `a7d7c02c`，其中 Kimi/Moonshot
  thinking 已窄化采用，custom endpoint/模型刷新已有等价，selector race 与 cold-start/first-token perf 进入
  P9 合同，全零 revision fallback 被拒绝；MiMo `ec413ade` 的学习 Skill 只形成 checkpoint state 机制信号。
- 本批新增 `internal/testenv`，隔离 HOME/USERPROFILE、XDG、AppData、TEMP/TMP 和 Reames home/state/cache，
  会写状态的 Go/Desktop 测试不再默认污染真实用户目录或 C 盘通用 Temp。
- M6 微信 iLink 已把 `get_updates_buf` 收到最终投递结算之后并原子持久化，危险 `account_id` 不能逃逸
  `weixin/accounts`；飞书/QQ/微信实时队列满时
  改为可取消背压。P9 Desktop `asyncRuntimeEmitter` 已完成 2048 项上限、瞬态 delta 合并、零语义 drop
  和 race/微基准；原生 WebView frame pacing 仍未由合成 benchmark 证明。
- 后续方向已由用户明确：P8 官方 OpenAI Responses/GPT 与 Claude parity 已关闭；P9 Codex-class
  Plugin/Skill/Hook/MCP/headless；P10 第一方 CDP Browser Control；P11 受治理 Remote SSH。现有兼容端点、插件基础或
  `web_search`/`web_fetch`/Playwright MCP 不能冒充这些阶段完成。
- 当前树只保留 Go/Wails 产品；旧 Hermes/Python/Electron/TUI/plugin/test/package、`site/`、`workers/`
  已删除，public-readiness 会阻止其回归。
- 内置工具 24 个；CLI 与 Guard 均支持 linux/darwin/windows × amd64/arm64、`CGO_ENABLED=0`。

## 4. 上游冻结点

所有本地镜像在冻结时均 clean，并执行 `fetch --prune --tags`、`pull --ff-only`。本机镜像只是便利缓存；
丢失后按 manifest URL/branch 重新 clone，Git 内的 lock 和审计才是权威。

| 项目 | reviewed SHA | 决策角色 |
|---|---|---|
| DeepSeek Reasonix | `2301e24827bf62c7584f34c4f541c432dd4f6e0b` | 唯一一级主源码上游；DeepSeek 原生与主 runtime |
| Hermes | `a7d7c02cb6db071eced4ac82e24f878588619600` | 三级 Gateway/错误/运维机制参考；Provider/selector/perf/provenance 信号已分类 |
| Codex | `678157acaa819d5510adfe359abb5d0392cfe461` | 二级战略；GPT/Responses、协议、插件、Hook/LSP/CDP/App-Server |
| MiMo Code | `ec413adeccfcb65ccb63a708bb6136644ea13c79` | 三级设计/Skill 体验参考；checkpointed learning state 候选 |
| Impeccable | `e4ab5e24bdf5321b72163d2fbcbe6fa985c848ba` | 品牌设计语言参考 |
| Scream Code | `22a2adaf8a459ab6bcfda028cc74b4c9b7e5f11f` | 三级 Goal/TUI/可靠性机制参考 |
| AgentArk | `63985cf819d1760f50f2a5c0dc11d82815e74623` | 安全架构参考 |
| Claude Code | `015170d3fd84fb57ef4685a64b673fadd0690dc1` | 二级战略；Claude/Messages、Thinking、工具/视觉/缓存、插件 |
| Kimi Code | `df6899553962d1764c9f4c3bec1b63c811cb425e` | 三级 Desktop Shell/headless/文件语义参考 |
| Grok Build | `ba76b0a683fa52e4e60685017b85905451be17bc` | 三级安全/终端/ACP 机制参考；Stop/session/permission 信号进入后续路线 |
| awesome-design-md | `664b3e78fd1a298ba11973822da988483256d4b4` | 设计资料参考 |

本批末尾另审查 Hermes `7a43ab04..34e66a0d` 与 Impeccable `8967edc9..e4ab5e24`：前者采用
PowerShell 5.1 纯 ASCII installer 棘轮，并修复全局凭据 `.env` 的 UTF-16 安全读取/成功保存后 UTF-8
规范化；UTF-32、损坏编码、嵌入 NUL 与截断 UTF-16 均拒绝写回并保留原字节。空白值 quoting、`key_env`
和动态 home 在 Reames 已有等价边界，MCP timeout/OOM 为 Python 非同构，增量 Markdown lexer 进入
Desktop 性能候选；后者留下 inset-shadow stripe 设计信号，并将单规则/规范文件集合的窄化豁免、未知
策略参数 fail closed 和生成副本同步保留为治理信号。以上最终 SHA 已写入锁文件；详见
`audits/2026-07-19-upstream-hermes-impeccable-delta.md`。

提交前又审查 Codex `312caf17..0fb559f0`、Hermes `34e66a0d..614dc194` 与 Scream
`5b1a9922..4938d517`：Codex 的 dynamic-tool/code-mode inline audio、modality fallback 与 paginated
App-Server metadata/summary/full legacy views、双向 cursor/live-turn 合并进入 P9 明确合同；Hermes 的 per-tool
订阅和 profile cache invalidation、Scream 的 SGR/ConPTY/大输入确认只作为需 benchmark 的机制信号。最终 SHA
已写入锁文件；详见 `audits/2026-07-19-codex-hermes-scream-precommit-delta.md`。

Reasonix `3637d0f0..40ef98de` 共 5 个非 merge 提交、49 文件、`+1658/-410`；完整逐提交结论和机器账本：

- `docs/audits/2026-07-18-reasonix-3637d0f-40ef98d.md`
- `docs/upstreams/reviews/reasonix-generation-3637d0f-40ef98d.json`
- `docs/upstreams/reviews/reasonix-current.json`

全参考冻结和 Grok intake：

- `docs/audits/2026-07-18-upstream-reference-freeze.md`
- `docs/audits/2026-07-18-grok-build-reference-intake.md`

P7 新增区间和机器账本：

- `docs/audits/2026-07-19-p7-upstream-gateway-watchdog.md`
- `docs/upstreams/reviews/reasonix-generation-40ef98d-2335d0d.json`
- `docs/upstreams/reviews/reasonix-current.json`

本批最新 Reasonix 与全参考再冻结：

- `docs/audits/2026-07-19-reasonix-2335d0d-a46fc6f.md`
- `docs/upstreams/reviews/reasonix-generation-2335d0d-a46fc6f.json`
- `docs/audits/2026-07-19-upstream-a46fc6f-reference-delta.md`
- `docs/audits/2026-07-19-reasonix-a46fc6f-65fcd46.md`
- `docs/upstreams/reviews/reasonix-generation-a46fc6f-65fcd46.json`
- `docs/audits/2026-07-19-reasonix-65fcd46-8bb0e54.md`
- `docs/upstreams/reviews/reasonix-generation-65fcd46-8bb0e54.json`
- `docs/audits/2026-07-20-reasonix-8bb0e54-2301e24.md`
- `docs/upstreams/reviews/reasonix-generation-8bb0e54-2301e24.json`
- `docs/audits/2026-07-20-upstream-strategic-reference-delta.md`
- `docs/audits/2026-07-20-codex-hermes-late-delta.md`

## 5. P7 本批代码变化

Reasonix `40ef98de` 的适用部分已按 Reames 状态机重构：

- TUI 捕获鼠标时，右键优先复制活动 transcript selection；无 selection 且 composer 可见时粘贴文本；
- SSH 环境不读取远端主机剪贴板冒充用户本地终端剪贴板，并显示三语提示；
- 右键文本重新进入统一 `tea.PasteMsg`，继续复用文件引用、长文本折叠、completion 和 repaint；
- assistant 回答增加稳定的 `Reames` identity/two-cell gutter；live 与 resume 使用同一投影；
- reasoning → answer、answer → usage receipt 增加语义间距，直接回答不增加首行空白。

Hermes 的 Windows BOM 信号已转成 Reames 修复：`internal/cron.Open` 接受 UTF-8 BOM，下一次成功保存
自动写回无 BOM JSON。Hermes 最终 `4c96172d` 的 CDP 双栈/端口占用修复因 Reames 没有同构
browser-connect runtime 而明确不适用。

上游追踪方面，Reasonix、Hermes、Codex、MiMo、Scream Code、AgentArk、Kimi Code、Grok Build 已启用
路径级 `diff=true`；以后只比较 lock → latest，仍然只自动发现/建单，不自动 merge/cherry-pick。

本批新增 `internal/systemdnotify`，无 CGO/libsystemd 依赖：

- `gateway install --watchdog-sec 60s` 在 Linux 渲染 `Type=notify`、`NotifyAccess=main`、
  `WatchdogSec=60s`；默认仍为 `Type=simple`，非 Linux/非 install/小于 2 秒 fail closed；
- Gateway recovery preflight 和至少一个 adapter 启动后才发送 `READY=1`；
- systemd 提供 watchdog 环境且至少一个 adapter 为 running/degraded 时发送 `WATCHDOG=1`，全部
  closed/error 后发送 unhealthy status 并停止心跳；
- SIGINT/SIGTERM 发送 `STOPPING=1`，Gateway stop 以 30 秒为上限；
- filesystem 与 Linux abstract Unix datagram、`WATCHDOG_PID`、报文清洗、unit 渲染和完整生命周期均有测试。

Codex `b8b61bc6` 的 compressed rollout inventory 已做代码级审查；Reames 当前没有 compressed session
或 SQLite thread inventory，因此不引入 zstd，但未来若压缩会话必须保留 logical path、plain sibling 优先、
corruption 与 temp-file 的独立诊断语义。Reasonix `2335d0df` Fleet 没有整体移植，因为 Reames writer child 已使用独立 worktree、显式交付事务、
durable effect journal 和整树预算；Reasonix named profile/字体只保留为 UX 候选。Hermes 的最终 Electron
full-load/resize zoom 修复缺少 Wails/WebView2 同构证据，只作为 native resize/跨显示器/recovery smoke
信号；其 Provider
stream/multimodal 信号转入 P8；MiMo/Kimi 无同构缺口，Scream Goal wizard、Provider/多代理帮助入口为
P8/P9 体验候选，品牌 welcome/Think badge 不复制。

## 5.1 P8 已交付状态

P8 仓库内实现与本地交付门槛已关闭，`a58f7691` 对应 CI `29663429224` 8/8、CodeQL
`29663429258` 3/3 全绿：

- `kind = "openai"` 新增显式 `api_mode = "responses"`；空值保持 Chat Completions，不能按模型名猜测；
- OpenAI Responses 已覆盖 instructions/input、GPT reasoning summary、文本、并行 function call/output、
  vision data URL、usage/cache/reasoning tokens、typed failed/incomplete、cancel/reconnect/interruption；同时
  保留向后兼容 include 并保存 opaque `reasoning.encrypted_content`（当前 `store=false` API 默认也返回），
  写入 `ReasoningBlocks` 供工具续轮回放，
  transcript DTO 与 `/export` 有不泄漏回归；
- Anthropic Messages 已补 effort 与 thinking 解耦、typed SSE error、缺失 `message_stop`/未闭合 block
  fail closed，以及 signed thinking + opaque `redacted_thinking` 原序持久化/回放；模型级 thinking override
  让 Haiku 4.5 省略不兼容 adaptive/effort，Sonnet/Opus 保持 adaptive；
- `ProviderEntry.api_mode` 已贯通 TOML、boot、Desktop 读写；第一方 OpenAI/Anthropic 推荐预设已加入；
- OpenAI 官方预设已按 2026-07-19 公共 API 文档开放 `gpt-5.6-sol`、`gpt-5.6-terra`、
  `gpt-5.6-luna` 与 `gpt-5.4`；前三者有 1.05M context、vision、普通 function calling 和
  none/low/medium/high/xhigh/max effort。Codex catalog 的 `code_mode_only` 只描述其产品 runtime，
  P9 仍须补 freeform/code-mode、Responses Lite/WebSocket、PTC、显式缓存、persisted reasoning/pro、
  hosted tools、multi-agent 与 App-Server，不能把 P8 function tools 冒充 Codex 产品 parity；
- Hermes 最新空响应分类修复已完成同构吸收：包含 `max_tokens` 的 empty-response advisory 不再让
  `provider.ClassifyError` 建议 context compaction，真实 `max_tokens > context_window` 仍保持该分类门禁；
  当前 Agent 压缩仍由 usage 驱动，尚未消费 `ShouldCompact`，不得写成已复现的运行时事故；
- Codex `0fb559f0` 前的 Realtime V3 初始历史项已做代码级审查，保留 128 项/8192 token 的会话播种边界；
  最新 inline audio 与 paginated legacy-view 语义进入 P9 WebSocket/code-mode/App-Server，不混入 P8 HTTP Responses；Hermes `7a43ab04` 的 Discord durable
  recovery cursor/final-delivery gate 进入 M6，`/model --once` 进入 P9，computer-use 验证后前台升级和
  dead-driver reconnect 进入 P10；
- 权威审计为 `audits/2026-07-19-p8-native-gpt-claude-provider-parity.md`。真实 OpenAI/Anthropic
  key、账单、缓存计费和公网模型可用性仍是 `external-blocked`。

## 5.2 M6 durable channel recovery 当前批

- CLI foreground Gateway 与 Desktop bot runtime 共用
  `<Reames Agent home>/bot/delivery-ledger.json`；schema v2 的 0600 原子文件保存入站身份、opaque cursor、
  状态，以及成功 turn 的最终文本 obligation；不保存入站正文/附件/tool/raw payload/raw error；
- host `AdapterBinding` 覆盖适配器自报 connection/domain 后才做访问控制、路由、认领和 checkpoint；
- 入站消息必须先持久 claim；重复 delivered/processing 跨重启抑制，冷启动遗留 processing 转 interrupted
  后可重试，损坏/超限/身份不一致/写失败 fail closed；
- 同一远端频道使用单调 sequence，只推进连续 delivered 前缀；群聊不同用户的后完成消息不能越过前面的
  failed 消息；默认 4096 records/channels、4 MiB 文件和每次启动全局 200 条扫描上限；
- 每个 outbound obligation 最多 1 MiB/512 个纯文本分片；长生命周期 OS 文件锁阻止 CLI Gateway 与
  Desktop bot 并发成为 writer。render 成功后先持久化 `pending`，每个分片 Send 前持久化 `attempting`，
  ACK 后推进 `next_chunk`，最后 ACK 与全部 constituent claims/cursor 原子结算；
- 冷启动先恢复 obligation，再用剩余全局扫描预算执行 `RecoveryAdapter`。纯 pending 无警告；attempting/
  failed 会给第一个恢复分片添加可见“可能重复”。重复 inbound 直接恢复原答复，不创建 Controller、不调用
  Provider；日志、status 和 metrics 只投影计数；
- slash/pairing/拒绝/steer 等同步回复仍按最终 ack 结算；成功
  collect/debounce、queue-cap summarize/drop 会把全部消息 claim 与 media 带入后续 turn；成功
  `/stop`、`/new`、`/reset`、`/use`、`/attach` 或 interrupt ack 会关闭全部被明确取消的 active/pending
  claims，避免用户明确停止或切换的任务在重启后重放；
- 微信 iLink 的原生 `get_updates_buf` 已改为最终投递结算后 0600 原子提交；failed settlement、磁盘失败与
  restart 从旧 buffer 重放。Telegram 同样在最终投递后推进 offset。飞书/QQ 仍没有已证明的 history/resume
  API；四类实时 adapter 的满队列现在背压而不静默 drop。当前仍不能把平台保留窗口冒充完全离线漏消息恢复；
- control `/status`、IM `/status` 与 metrics 只返回统计计数。权威审计：
  `audits/2026-07-19-m6-durable-channel-recovery.md` 与
  `audits/2026-07-20-m6-outbound-final-response-obligation.md` 与
  `audits/2026-07-20-m6-weixin-polling-desktop-backpressure.md`。

## 5.3 Reasonix 最新可靠性与 Telegram 扩展

- Reasonix `2335d0df..a46fc6f` 的 21 个提交/16 个非 merge/106 个文件已代码级审查；采用
  test-state isolation、Windows batch Hook、有界 session save lock/shutdown recovery、每模型
  context window、Mermaid/旧 WebKit/线性脱敏、Windows-safe session filename 和最大化 resize cursor；
- 取消轮次 partial display、PRIMARY/tmux 中键粘贴和 conversation width 有明确延后；远程 crash
  upload/遥测与当前 TanStack 版本不存在的私有 API 明确不采用；
- Telegram 使用 `update_id` 作为 durable identity、`message_id` 作为 reply，`getMe` 后启动 long poll，
  每次请求有 deadline、指数退避和可取消 Stop；localhost E2E 已证明首次 send 502 时 offset 保持 0，
  第二次成功并 durable commit 后推进到 43，token 不进入 ledger；
- Hermes 最新 outbound delivery obligation 信号已按 Reames 单一 Go runtime 完成采用：不复制 Python/
  Electron gateway，而是在现有 ledger/render/Gateway 中实现有界、0600、身份绑定且对 mid-send 歧义可见
  标记的 obligation 恢复；
- Codex 本轮无新增；Claude 只有 changelog/feed，未从说明推断协议变化。MiMo BM25 Skill search、Scream
  25 项 bug audit、Kimi 多实例 Web/分片 read model/stat-lstat 仅作为 P9/P10 或可靠性候选。
- 最终补扫又发现 Reasonix `a46fc6f..65fcd465` 5 个提交/141 文件：本批采用 LongCat-2.0 1,048,576
  context window 与窄迁移、Linux WebKit/JavaScriptCore `SA_ONSTACK` 启动修复、会话图片/PDF 分页和后端
  原子多文件导出；pane opacity 因 Theme Pack v1 schema 已关闭而延后，Remote SSH 进入 P11，不整套复制。
- Hermes `3a6e40b2..36f2a966` 只登记 search/extract 能力分离和设置 deep-link 机制；Kimi
  `a3e773f9..df689955` 仅新增远端 thinking-effort telemetry，Reames 不建设自有遥测服务，因此不采用。
- Scream `c6b24f60..22a2adaf` 的 component-scoped animation/footer render 只作为 TUI 性能机制信号；
  Reames React/Wails 没有同构的全屏 `requestRender` 热路径。
- Reasonix 最终 `65fcd465..8bb0e549` 修复设置刷新覆盖活动 Theme Pack。Reames 已由统一
  `themePackRuntime.effectiveStyle/apply` 等价覆盖，本批新增显式回归，证明刷新配置不替换活动包，清包后恢复
  最新配置基底；生产代码无需复制 helper。

## 6. 本批本地验证

当前 M6/P9 批次在正式提交前已直接从工作树重跑并通过：

- Root build/vet/全 `internal/...` 测试，Desktop build/vet/全测试；
- Frontend 完整 `test:all`、production build 和 bundle budget：entry JS 641,587 B、localized initial
  1,003,986 B、browser mock 983,252 B，全部在预算内；
- linux/darwin/windows × amd64/arm64 的 CLI 与 Guard，共 12 个 `CGO_ENABLED=0` 目标；
- Gateway credential-free clean-node smoke：真实构建二进制、隔离 home、localhost Provider、持久会话与
  feedback 生命周期，`status=passed`；
- scripts 全发现 151 项测试通过、2 项按平台跳过；deploy/release/docs/public、安装器、Desktop
  artifact/candidate/native/interaction/accessibility/recovery/plugin lifecycle 合同和 Issue reconciliation 全部通过；
- 最终绑定 SHA 的 `--deep` 为 11/11、`changed_count=0`；
- 上游：Codex `678157ac`、Hermes `a7d7c02c` 与 MiMo `ec413ade` 逐提交/逐文件审查；最终接受强制使用绑定完整 SHA 的
  `--accept-revision`，未绑定 `--accept`/`--accept-all`/`--update-lock` 已禁用；
- Kimi：Anthropic provider 定向测试证明官方 host/model family 识别、无签名原生 thinking block 续轮回放、
  lookalike host 拒绝和 Claude signature 边界；provider/config 全组通过；
- 高风险定向 race 覆盖微信/飞书/QQ/Telegram，以及 Desktop emitter/tab sink；
- `BenchmarkAsyncRuntimeEmitterCoalescedBacklog` 在 Windows amd64 独立 5 轮中位数约 `1.3 µs/op`，只证明 Go 队列合并开销，
  不冒充原生 WebView frame pacing。

提交前不得把旧 detached SHA、早期 clean clone、插件 smoke 或历史 run ID 当作本批证据。正式提交后必须从该
提交建立新的 `F:\reames-agent-clean-verify`，重跑 Root、Desktop、Frontend、公开清洁与上游治理门槛；只有
clean clone 通过且最终 push SHA 的 CI/CodeQL 全绿，才可关闭本批公开交付门槛。

远端完成声明必须使用最终 push 提交对应的 CI/CodeQL。为避免仅写回 run ID 又触发一次 CI，本文件不
硬编码本批 run ID；新会话使用：

```powershell
gh run list --commit (git rev-parse HEAD) --limit 20
```

## 7. 外部依赖和未关闭边界

以下不能用 mock、localhost 或测试密钥冒充完成：

- 生产 registry HTTPS endpoint、不同人员见证的离线 root/targets threshold ceremony、HSM/等价托管、
  freshness monitor、真实轮换与 compromise drill；
- 声明 builder identity/SLSA level 时的独立 DSSE/SLSA policy verifier；
- 干净 Linux 云节点上的 linger-enabled logout/reboot 与 Gateway recovery/system service 实启；
- 真实 OpenAI/Anthropic/DeepSeek Provider 和飞书/QQ/微信/Telegram 的文本、审批、取消、恢复回环；
- 真实 systemd watchdog kill/restart、IM 远端掉线/重连和第一方 CDP 对真实登录态浏览器的控制；
- 公开签名 release、Windows/macOS signing/notarization 与真实升级失败/断电点演练；
- NVDA/Narrator 实际听感和 Windows High Contrast 人工验收。

这些是 `external-blocked`，不是仓库失败。没有真实 API key、IM 应用或云服务器时，继续完成仓库内
合同、fixture、fail-closed 和 redaction，但不得把它们写成生产证据。

## 8. 新会话启动顺序

```powershell
Set-Location F:\reames-agent
git status --short --branch
git branch --show-current
git log -1 --oneline --decorate
git fetch origin --prune
git rev-list --left-right --count main...origin/main
python scripts/check_upstreams.py --out-dir artifacts/upstream-watch
gh run list --commit (git rev-parse HEAD) --limit 20
```

判断规则：

1. 工作树应干净，当前分支应为 `main`，`main...origin/main` 应为 `0 0`；否则先审查，不 reset/丢弃。
2. Upstream Watch 若无新提交，不重开 P6/P7；若有新提交，只审 lock → latest。
3. 本批 CI/CodeQL 若失败，先在同一批修复，不用碎片 push 消耗 CI。
4. 电脑清理后若 `F:\code-reference` 丢失，按 `docs/upstreams/upstreams.json` 重建；不要从旧聊天猜 SHA。
5. 远端全绿且用户未提供外部环境时，M6 飞书/QQ 历史分页、微信/Telegram 真实保留窗口、真实掉线和云节点
   证据保持等待；仓库内继续逐渠道审批/取消/reconnect fixture 与 P9 App-Server/headless，
   再进入 P10，不降低真实 API、真实 IM、systemd reboot 或浏览器登录态证据门槛。

## 9. Git 与清洁约束

- `artifacts/`、`bin/` 构建产物、Desktop `dist` 生成内容不提交；只提交权威审计和机器账本。
- 大批删除只用显式路径和预览；不执行宽泛 `git clean -fdX`。
- 不使用 `git reset --hard`、`git checkout --` 丢弃未知改动。
- 提交前运行 `git diff --check`、`git diff --cached --check`；push 后核对 CI/CodeQL。
- 当前仓库只维护 `main`；不要为会话交接额外制造长期分支。
