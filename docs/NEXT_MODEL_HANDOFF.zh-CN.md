# Reames Agent 新会话无痛交接

> 日期：2026-07-19
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

持续把 Reames Agent 推进到高可信可交付状态；Reasonix 是一级主源码上游，Codex/Claude Code 是二级
战略代码上游，其他项目只吸收适用机制。
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
- M6 当前批正在关闭 durable channel recovery core：持久 claim/去重、原消息身份、每频道连续前缀 cursor、
  最终发送门禁、全局补扫上限和隐私安全状态投影已实现并通过定向/race；内置渠道的真实历史分页和真实 IM
  掉线回环仍未完成，不能从核心合同外推。
- 后续方向已由用户明确：P8 官方 OpenAI Responses/GPT 与 Claude parity；P9 Codex-class
  Plugin/Skill/Hook/MCP/headless；P10 第一方 CDP Browser Control。现有兼容端点、插件基础或
  `web_search`/`web_fetch`/Playwright MCP 不能冒充这些阶段完成。
- 当前树只保留 Go/Wails 产品；旧 Hermes/Python/Electron/TUI/plugin/test/package、`site/`、`workers/`
  已删除，public-readiness 会阻止其回归。
- 内置工具 24 个；CLI 与 Guard 均支持 linux/darwin/windows × amd64/arm64、`CGO_ENABLED=0`。

## 4. 上游冻结点

所有本地镜像在冻结时均 clean，并执行 `fetch --prune --tags`、`pull --ff-only`。本机镜像只是便利缓存；
丢失后按 manifest URL/branch 重新 clone，Git 内的 lock 和审计才是权威。

| 项目 | reviewed SHA | 决策角色 |
|---|---|---|
| DeepSeek Reasonix | `2335d0df9ea4029108ed965f76c2efff30fe6cf4` | 唯一主源码上游 |
| Hermes | `614dc194ea7d853d39f9e84582ec62156f41a475` | Gateway/错误/运维参考 |
| Codex | `0fb559f0f6e231a88ac02ea002d3ecd248e2b515` | 二级战略；GPT/Responses、协议、插件、Hook/LSP/CDP |
| MiMo Code | `d48888f7b1d22e830ee5c10faf2b9e455f3cd881` | 设计/技能体验参考 |
| Impeccable | `e4ab5e24bdf5321b72163d2fbcbe6fa985c848ba` | 品牌设计语言参考 |
| Scream Code | `4938d5175349768774876efdc0beab8a149ced25` | Goal/TUI/主题机制参考 |
| AgentArk | `63985cf819d1760f50f2a5c0dc11d82815e74623` | 安全架构参考 |
| Claude Code | `07dcb0e13580b21174ff1bf6a7e1d5ead3b61d60` | 二级战略；Claude/Messages、Thinking、工具/视觉/缓存、插件 |
| Kimi Code | `4f3c7240c4adc7c748e536bf578e468c1b5bcd7b` | Desktop Shell/权限文案参考 |
| Grok Build | `7cfcb20d2b50b0d18801a6c0af2e401c0e060894` | 安全/终端/ACP 机制参考；本批采用无歧义 MCP 名称合同 |
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
  `<Reames Agent home>/bot/delivery-ledger.json`；0600 原子文件只保存消息身份、opaque cursor、状态、
  attempt 和时间，不保存正文/附件/model/tool/raw payload/raw error；
- host `AdapterBinding` 覆盖适配器自报 connection/domain 后才做访问控制、路由、认领和 checkpoint；
- 入站消息必须先持久 claim；重复 delivered/processing 跨重启抑制，冷启动遗留 processing 转 interrupted
  后可重试，损坏/超限/身份不一致/写失败 fail closed；
- 同一远端频道使用单调 sequence，只推进连续 delivered 前缀；群聊不同用户的后完成消息不能越过前面的
  failed 消息；默认 4096 records/channels、4 MiB 文件和每次启动全局 200 条补扫上限；
- `runTurn` 成功且至少一个最终文本分片发送成功后才提交；render 发送复用 Gateway send tracker，发送失败
  同时进入 adapter health 和 retry ledger；slash/pairing/拒绝/steer 等同步回复按最终 ack 结算；成功
  collect/debounce、queue-cap summarize/drop 会把全部消息 claim 与 media 带入后续 turn；成功
  `/stop`、`/new`、`/reset`、`/use`、`/attach` 或 interrupt ack 会关闭全部被明确取消的 active/pending
  claims，避免用户明确停止或切换的任务在重启后重放；
- 可选 `RecoveryAdapter` 合同和 fake history scan 已有故障注入，但内置飞书/QQ/微信尚未接真实历史 API。
  当前完成的是 durable live-event dedupe/final-delivery core，不是完全离线漏消息恢复；
- control `/status`、IM `/status` 与 metrics 只返回统计计数。权威审计：
  `audits/2026-07-19-m6-durable-channel-recovery.md`。

## 6. 本批本地验证

P8 冻结提交形成前已通过：

- Root：`go build ./...`、`go vet ./...`、`go test ./internal/... -count=1 -timeout 300s`；
- Desktop：`go build ./...`、`go vet ./...`、`go test ./... -count=1 -timeout 300s`；
- Frontend：`corepack pnpm test:all`、`corepack pnpm build`、bundle budget；
- Race：`go test -race ./internal/provider/openai ./internal/provider/anthropic ./internal/agent ./internal/plugin ./internal/control -count=1 -timeout 600s`；
- 上游：21 项治理测试、Node Issue reconciliation、Codex/Hermes 显式逐项目接受，最终 11/11 无变化；
- 治理：docs/public/deploy/release、安装器、Gateway、Desktop candidate/native/interaction/accessibility 合同；
- 浏览器：真实 Chrome 插件安装/启用/更新/回滚/doctor/移除 smoke，无 console/page error；
- 发布形态：六目标 CLI + 六目标 Guard，共 12 个 `CGO_ENABLED=0` 构建。
- clean clone：冻结提交对象通过独立 `--no-hardlinks` clone 的 Root/Desktop 全量、空 `node_modules`
  Frontend install/test/build/bundle budget，以及 Reasonix/upstream/docs/public 合同。

M6 当前批已通过：

- Root `go build ./...`、`go vet ./...`、`go test ./internal/... -count=1 -timeout 300s`；
- Desktop `go build ./...`、`go vet ./...`、`go test ./... -count=1 -timeout 300s`；
- Frontend `corepack pnpm test:all`、production build/bundle budget 与真实 Chrome plugin lifecycle smoke，
  无 console/page error；
- Race：`internal/bot`、`botruntime`、`control`、`plugin`、`config`；
- 六目标 CLI + 六目标 Guard 共 12 个 `CGO_ENABLED=0` 构建；
- durable claim/cold restart/corruption/redaction、跨用户顺序门禁、collect/queue-cap/interrupt 全 claim 结算、
  取消 ack 失败重试、真实 runTurn 最终发送成功/失败、补扫上限、checkpoint handoff 与 adapter scan failure；
- docs/public/deploy/release/installer/upstream 治理合同；`2026-07-19T00:56:24Z` 最终上游检查 11/11 无变化。
- 独立 `--no-hardlinks` clean clone 已从空 `node_modules` 和隔离在 F 盘的 Go/pnpm 缓存完成 Root、Desktop、
  Frontend、真实 Chrome plugin lifecycle、六目标 CLI/Guard、Gateway clean-node smoke，以及 docs/public/
  deploy/release/installer/upstream 全部合同；上游复查仍为 `changed_count=0`。

本批仓库内与 clean-clone 门槛已关闭；尚未完成的是最终 push 提交对应的远端 CI/CodeQL。在远端证据
全绿前不得关闭本批最后公开交付门槛。

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
- 真实 OpenAI/Anthropic Provider 和飞书/QQ/微信的文本、审批、取消、恢复回环；
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
5. 远端全绿且用户未提供外部环境时，M6 渠道历史分页/真实掉线和云节点证据保持等待；仓库内继续 P9，
   再进入 P10，不降低真实 API、真实 IM、systemd reboot 或浏览器登录态证据门槛。

## 9. Git 与清洁约束

- `artifacts/`、`bin/` 构建产物、Desktop `dist` 生成内容不提交；只提交权威审计和机器账本。
- 大批删除只用显式路径和预览；不执行宽泛 `git clean -fdX`。
- 不使用 `git reset --hard`、`git checkout --` 丢弃未知改动。
- 提交前运行 `git diff --check`、`git diff --cached --check`；push 后核对 CI/CodeQL。
- 当前仓库只维护 `main`；不要为会话交接额外制造长期分支。
