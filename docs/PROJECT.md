# Reames Agent 项目说明

> 状态：当前产品方向的权威说明
>
> 更新：2026-07-20

## 一句话定位

Reames Agent 是一个以 DeepSeek Reasonix 为工程底座、面向本地与远程工作的多平台通用 Agent。编程助手是第一条成熟主线，长期能力覆盖研究、文件与数据处理、自动化、记忆、插件和多渠道协作。

## 产品形态

同一套 Agent runtime 支撑四个入口：

| 入口 | 定位 |
|---|---|
| Desktop | 主产品；完整会话、工作区、审批、变更、设置和恢复体验 |
| CLI | 终端中的高效编程与自动化入口 |
| Server/Web | 本机、局域网和云端的 HTTP/SSE 服务 |
| Gateway | 飞书、QQ、微信、Telegram 等消息渠道 |

入口只负责交互和传输。会话、Agent loop、模型、工具、权限和事件语义应逐步收敛到 `internal/control` 后面的统一运行边界。

可选云端部署形态见 [CLOUD_AGENT_PLAN.md](CLOUD_AGENT_PLAN.md)：目标是在用户主动部署的节点上同时支持 SSH/CLI、HTTP/SSE、飞书等 IM 通道和后台上游研究 Worker；它不是本地产品、开发或 CI 的依赖，也不承载 Reames 自有遥测。反馈与脱敏诊断默认只在本地落盘，由用户显式导出。

## 项目来源

- [DeepSeek Reasonix](https://github.com/esengine/DeepSeek-Reasonix) `main-v2` 是唯一一级主源码上游；
  DeepSeek 原生模型协议、Agent/runtime、Desktop/CLI 和可靠性修复均做代码级持续追踪。
- `F:\Reames-Lite` 是前身，只保留公共边界、缓存纪律、中文体验和接口契约等思想。
- OpenAI Codex 与 Claude Code 是二级战略代码上游：分别跟进 GPT/OpenAI 与 Claude/Anthropic 的原生
  模型协议，以及推理、缓存、工具、多模态、会话、子代理、插件、Hook、MCP、LSP、headless 和
  Browser/CDP 等代码级产品能力；“原生”必须由官方 wire、stream、reasoning/thinking replay、usage/cache、
  tool/vision、错误和 runtime 行为证明，不能由兼容 endpoint 或模型名称代替；仍按 Reames Go/Wails 架构
  重构，不作为可直接拼接的 runtime。
- `F:\code-reference` 中的其余官方项目提供机制和体验参考，不作为可直接拼接的代码集合。
- 详细来源、许可证和升级规则见 [REFERENCE_GOVERNANCE.md](REFERENCE_GOVERNANCE.md)。

## 核心原则

1. **单一运行语义**：Desktop、CLI、Server 和 Gateway 不各自实现 Agent 行为。
2. **缓存稳定**：system prompt 与 tool schema 保持稳定；UI、渠道和诊断状态不得污染模型前缀。
3. **副作用受控**：文件、命令、网络和凭据操作经过权限、沙箱、脱敏与证据记录。
4. **状态可恢复**：会话、检查点、任务和后台作业必须有清晰的持久化与恢复语义。
5. **桌面优先但不绑死桌面**：桌面是主产品，核心能力仍保持传输无关。
6. **上游人工决策**：自动发现、分类和建单；由维护者审查、移植和接受版本。
7. **证据先于完成声明**：构建、测试、真实交互或发布证据齐备后，事项才算完成。
8. **模型与产品能力解耦**：支持 GPT/Claude 不只等于兼容聊天端点；Provider、插件、headless 和浏览器能力
   分别建立明确合同，并继续复用统一 Controller、权限、沙箱和 evidence。

## 当前事实

- Go Agent 内核、CLI、HTTP/SSE、IM Gateway 和 Wails/React Desktop 已有较完整实现。
- 核心、Desktop 和前端已建立本地与远端 CI 基线，并有六目标 CLI candidate、三平台 Desktop candidate 及原生安装 smoke 记录。
- M1 真实任务闭环已关闭：真实 Provider、原生会话/工作区/停止、文件审批/落盘/回退、重启恢复以及五类原生失败恢复均有分层证据。
- 24 个内置工具，具备权限、沙箱、检查点、记忆、技能、插件、定时任务、LSP 和证据账本等模块。
- M3 Desktop 日用化已关闭：关闭态/次级界面与简中/繁中词典按需拆包并受真实产物硬预算保护，模态隔离、Transcript 语义和严格 Windows UIA 可访问性 smoke 已交付。commit `68218d6` 的 CI `29262192635` 8/8、CodeQL `29262193090` 3/3；candidate `29262541971` 的 Linux/macOS installed 与 Windows 全链路均通过。Windows native cold/warm 为 7.031/2.016 秒；interaction 完成 19 请求、五类失败恢复、停止和同 project/workspace/session 恢复，跳转前 marker present 但 offscreen、assistant 不在 UIA，严格 InvokePattern 定位问题 1 后 user/assistant 均 present + onscreen，`recovery_verified=true`；strict accessibility 随后实际执行并通过。三份 smoke 的 `boundary_changes=[]` 且无 errors。自动证据不等于 NVDA/Narrator 实际听感或 Windows High Contrast 人工验证。
- M6 的无界面配置与 credential-free 运维预检已形成纵向链路：`gateway setup` 支持飞书/Lark、QQ、微信和 Telegram，secret 只引用大写环境变量名，访问控制 fail closed，dry-run 零落盘，更新原子、幂等并保留其他连接、route、access 和 session mappings；同一隔离 home 的真实 CLI 二进制覆盖 setup → `gateway recovery-status` 健康 schema/零 findings 与损坏 TOML `config.invalid` fail-closed → doctor → service-plan、localhost Provider 一次性 `run`/会话落盘、反馈脱敏/去重/维护草稿，预检后精确恢复配置且证据不加载凭据值。WSL2 真实 systemd user manager 进一步通过带空格 binary/home/workspace 的 install、同名重装生效、status、restart、stop/start、journal、loopback webhook readiness 与 uninstall `LoadState=not-found`，但 `Linger=no`，不替代 logout/reboot、干净云节点、真实 Provider 或真实 IM 回环证据。
- M6 本地恢复基线已补齐三条确定性链路：Linux user-scope `gateway install` 会先执行 `systemd-analyze --user verify`，并对旧 unit bytes/mode、enabled/active 状态做快照和分层回滚；`gateway uninstall` 同样先快照、在 disable/delete/reload 后验证 manager 已缺失，取消或故障时用独立恢复上下文写回定义并恢复原状态，恢复定义/reload 再失败则 fail closed 要求人工修复；`backup create/verify/restore` 支持 home/state 分根、已知凭据排除、内嵌哈希自洽验证和仅恢复到不存在的新目标；CLI updater 会实际执行候选与安装后 `version` 健康检查、保留 `<executable>.previous`，并支持互斥锁保护的 `upgrade --rollback`。这些证据来自本地故障注入、race/vet 和跨平台编译，不等于跨根崩溃原子性、Windows 目标目录 ACL 保护、macOS/Windows Gateway 事务，也不替代干净云节点或公开签名 release 的实际演练。
- M6 Gateway core 已吸收 Hermes durable channel recovery 信号：CLI/Desktop 共用 Reames home 下的 0600 原子 schema-v2 投递账本，消息先 claim 再进入访问控制/排队/Agent；重复消息跨重启抑制，遗留 processing 冷启动转为可重试。游标按远端频道和单调序列只推进连续成功前缀；collect/debounce 合并、queue-cap summarize/drop、interrupt 与 `/stop|/new|/reset|/use|/attach` 均保留或显式结算全部 constituent claims。成功 turn 的最终文本先保存为最多 1 MiB/512 分片、身份绑定的 outbound obligation；发送每个分片前持久化 `attempting`，ACK 后推进 `next_chunk`，最后 ACK 与全部 inbound claims/cursor 在一次原子写中结算。重启直接恢复原答复而不重跑模型；ACK 与本地 commit 之间的歧义会显示“可能重复”，纯 pending 不误报。同一路径 OS 文件锁阻止 CLI Gateway 与 Desktop bot 双 writer；4 MiB 总账本、损坏/超限/身份漂移/写失败均 fail closed，日志、status、metrics 和 Provider prompt 不含答复正文。Telegram 已成为正式 long-poll Adapter：`getMe`、deadline、退避、Stop、`update_id` durable identity、原生 reply 和最终 commit 后 offset 推进均有 localhost 故障注入。飞书/QQ/微信仍缺真实历史分页 `RecoveryAdapter`，Telegram 也尚无独立历史 API 补扫；完全离线补消息与真实 IM 掉线回环继续保持 adapter/external-blocked。详见 `audits/2026-07-19-m6-durable-channel-recovery.md`、`audits/2026-07-19-m6-telegram-durable-polling.md` 与 `audits/2026-07-20-m6-outbound-final-response-obligation.md`。
- M4 Agent 可靠性已按路线图门槛关闭：所有 Goal completion 统一通过 Todo/project checks，v2 sidecar 持久化 Goal/Plan/Todo、最小 root 项目检查引用和 child journal cursor；委派树共享预算，可写 child receipt/checkpoint 归并给祖先，持久 subagent 在 Provider/tool/compaction 边界保存 transcript 并以 `interrupted` + `continue_from` 显式恢复。Previewable built-in writer/checkpoint restore 使用 `os.Root` resolve-beneath I/O；每个 visible/synthetic turn 都有独立恢复 checkpoint，in-flight commit anchor 让冷启动在“完整提交则保留、否则 workspace/runtime/transcript 一起回滚”之间 fail closed。Conversation/RewindBoth 另有 `prepared -> resources_applied` journal，checkpoint 只在资源提交后退休。`AtomicWriteFile` 已移除 Windows 原地复制降级，跨设备 rename fail closed，并补 write-through/父目录同步。该完成声明严格限于可预览文件 writer 与会话本地资源：完整 evidence/预算仍非跨进程账本，child-only bash 不是 durable root proof，shell/MCP/external API 和后台 opaque side effect 不具备逐文件门禁或 exactly-once，ACL/xattr/硬链接身份也不恢复。
- M5 插件生命周期信任机制已关闭所有可由仓库、clean clone 和 CI/CodeQL 验证的事项：原生 schema v1、精确权限、不可变 generation、默认禁用、`preview/planId/apply`、跨进程状态锁和 `os.Root` 受管路径已有故障注入与完整生命周期测试。Desktop、CLI、Bot、Serve/event wire 和 ACP 共用 fresh-human 结构化审批；generation 变化或禁用会原子阻止新 work 起跑，串行 rebuild，并撤销旧 MCP/Hook/Skill runtime。package-owned Hook/MCP 使用最小环境、独立 state/tmp、严格 OS sandbox、敏感读取阻断和进程树回收；真实 `obra/superpowers@d72560e462a74e10d161b7f993d5fc3282bfa1e2` 已完成 Windows sandbox E2E。commit `13016c6` 加入无默认 endpoint/TOFU 的官方 `go-tuf/v2` registry client，绑定 full commit、canonical tree digest、manifest 权限和 provenance assertion。commit `9295f8b` 又加入显式带外 root 的只读生产策略审计，验证连续 root 旧/新双阈值、角色 key 隔离、到期窗口和完整 metadata/index/attestation 字节，并在成功报告中保留人员仪式、HSM、endpoint/monitor 与 DSSE/SLSA policy 等 `externalRequired`。该提交的 clean clone、本地全量门禁、普通 CI `29510215514` 8/8 与 CodeQL `29510215449` 3/3 全绿，Node.js 24 action majors 的远端日志不再出现 Node.js 20 弃用告警。M5 唯一未关闭项保持 `external-blocked`：直接 GitHub 未签名，以及真实运营公开 registry 的生产 endpoint、人员/HSM 密钥仪式、实际轮换/compromise drill 和独立 DSSE/SLSA policy verifier。package process 允许网络、跨平台硬 CPU/RSS 配额不统一仍是明确安全限制，但不以 mock 冒充生产 registry 证据，也不重新打开已验收的 M5 仓库内合同。
- 2026-07-17 Reasonix 再同步已采用 MCP schema/凭据/Provider 加固，并关闭 identity-bound MCP trust P0、writer worktree P1 与 offline Guard/Safe Mode P2。独立 Guard 在普通 runtime 之前运行，使用五分钟三次失败的进程所有权启动账本、30 秒健康观察期、配置健康快照和完整安装单元 pending transaction；自动回滚必须同时满足 crash 归因、版本、同安装目录与全量备份 SHA-256，歧义或 mixed install fail closed。Safe Mode 不读取用户/项目 TOML 或 dotenv，不启动 Provider、MCP、插件、Hook、Bot、LSP、planner、Guardian、subagent 或 Memory Compiler。CLI/Serve/Desktop/Gateway 共享 `repair.Report`；三平台 Desktop 包均通过 Guard 启动，Gateway service 在加载运行时前执行同一 credential-free preflight。
- P3 Desktop Recovery Center 已完成仓库内、真实 Windows 本地和修复后远端三平台证据：普通模式与 recovery-only Safe Mode 都只投影同一 `repair.Report`，所有修复经 `repair.ExecuteAction -> control.Controller.RunRecoveryAction -> Desktop RunRecoveryAction` 受控执行；支持配置隔离、快照恢复、精确 undo、已验证更新回滚、tabs/projects/window/zoom 派生状态重建和插件全量禁用。Recovery DTO 空数组 `null` 回归已加入 Go/Wails 合同；提交 `89a8b2b` 的 CI `29610566790` 8/8、CodeQL `29610566725` 3/3、Desktop candidate `29610593446` Linux/macOS/Windows 全绿。真实签名/notarization、公开 release 升级失败与断电点回滚保持 `external-blocked`。
- P4 已把 Reasonix 追踪从增量抽样升级为完整代码级代际账本：固定 `07c65c2..3637d0f0`，枚举 672 个提交、498 个非 merge 提交、314 个精确 fix/perf 提交和 993 个变化文件，并将稳定 tag、活跃未合并 ref、15 个 required area 的源码/测试证据写入 `audits/2026-07-18-reasonix-generation-parity.md` 与 `docs/upstreams/reviews/`。`dae65e25` 会话可靠性和 `d3cfa5c2` DeepSeek reasoning-only stop 已吸收；普通/计划/目标与 economy/balanced/delivery 两个正交轴已闭环 Desktop、CLI/TUI、Serve、ACP 和 Bot 默认合同。Reames 自有启动、metrics、crash、performance 远端上传永久删除，只保留本地诊断。`7f00d2c2` Theme Pack V2 进入 P5；Memory v5 删除、trust 简化和生产 release workflow 是有证据的产品/安全分歧，不机械跟随。
- P5 已关闭受控 Theme Pack 交付：`.reames-theme` v1 只允许严格 JSON、语义颜色、有限 recipe 和最多两张本地 raster scene；ZIP/path/symlink/Windows 名称/数量/尺寸/压缩比/像素炸弹、图片 magic/解码与完整 SHA-256 均 fail closed。用户包使用内容寻址 Store、原子 pack/state、可恢复 install/delete journal 和故障注入；替换 active 包会原子推进活动摘要。Desktop Appearance/Gallery 延迟加载，选择、预览和应用分离，预览不落盘且重启撤销；Safe Mode 不读主题 Store、所有 Wails mutation 入口后端拒绝，并强制 Graphite。两套只读官方主题使用 Reames 原创内嵌 JPEG，ID、许可证、生成记录和 digest 可检查，不继承 Reasonix 品牌、图片、marketplace、endpoint 或发布运行时。`b4815ba9` 完成交付，`7396faf4` 修复 recovery smoke 的 detached 进程树竞态；CI `29635818559` 8/8、CodeQL `29635818555` 3/3、Desktop candidate `29635823162` Linux/macOS/Windows 全绿。当前并发写保证限于受单实例保护的 Desktop 进程；未来开放 CLI/Serve 写入口前必须增加跨进程 Store lock。用户与资产文档见 `THEME_PACKS.md`，验收审计见 `audits/2026-07-18-p5-controlled-theme-pack-design.md`。
- Grok Build 已作为第 10 个正式机制参考项目纳入 Upstream Watch：官方 `xai-org/grok-build` `main` 首次锁定 `98c3b24`，本地镜像位于 `F:\code-reference\Grok-Build`。优先研究权限/sandbox、持久 session/subagent、TUI/ACP/headless 和终端恢复；不引入其 Rust 第二 runtime、xAI auth/model endpoint、遥测、在线 memory、managed policy 或 marketplace。其 Plan Mode 明确不拦 shell 重定向及可写 child，因此只作为负面回归信号，不降低 Reames 的 planmode/permission 边界。
- P6 已完成 11 个上游/参考仓库的最新版代码级冻结。Reasonix `3637d0f0..40ef98de` 的 5 个提交和其他发生变化的参考区间均已逐文件分类；Reames 直接吸收 CLI 右键文本粘贴、SSH 剪贴板边界、assistant transcript hierarchy/语义间距，以及 Hermes 暴露的 Windows cron JSON UTF-8 BOM 兼容缺口。站点、品牌、registry UI、生产发布授权、billing/subscription、音频/远程 executor、Rust/Python/Electron 第二 runtime 等均记录为拒绝、不适用或未来候选。11 个 reviewed SHA 已由逐项 `--accept` 固定在 `docs/upstreams/upstreams.lock.json`，代码型参考默认启用路径级 diff；完整结论见 `audits/2026-07-18-reasonix-3637d0f-40ef98d.md` 与 `audits/2026-07-18-upstream-reference-freeze.md`。
- P7 已完成 Reasonix `40ef98de..2335d0df`、Codex `56395bdd..b8b61bc6` 与 Hermes/MiMo/Scream/Kimi
  最新增量的代码级分类。Codex 的压缩 rollout inventory 对当前未压缩 Reames session 不适用，但其
  canonical logical path、plain/compressed sibling、损坏压缩件和临时文件语义已固定为未来回归信号。Reasonix
  Fleet 不替换 Reames 独立 writer worktree、delivery transaction、durable effects 和整树预算；区域字体与
  named profile 保留为 UX 候选。Hermes 的 systemd 生命周期信号已转成纯 Go `internal/systemdnotify`：
  Linux 安装可 opt-in watchdog，Gateway 在 recovery preflight 与 adapter start 后发 `READY=1`，adapter
  健康时发 `WATCHDOG=1`，全部 unhealthy 后停止心跳，退出发 `STOPPING=1` 并做有界停止。真实 systemd
  watchdog restart 和 IM 远端 liveness 仍需外部证据。完整结论见
  `audits/2026-07-19-p7-upstream-gateway-watchdog.md`。
- 用户后续产品方向已固定：P8 补官方 OpenAI Responses/GPT 与 Anthropic Claude 能力矩阵；P9 以 Codex
  审计并补齐插件/Skill/Hook/MCP/headless 能力；P10 实现第一方 CDP Browser Control。当前已有
  OpenAI-compatible Chat Completions、Anthropic Messages 和受治理插件基础，但不能据此宣称 Codex/Claude
  parity；`web_search`、`web_fetch` 与可选 Playwright MCP 也不算内置浏览器控制。
- P8 已完成仓库内原生协议实现，并通过本批全量、race、12 目标交叉编译与 clean-clone 门槛：OpenAI 通过显式
  `api_mode=responses` 支持 instructions/input item、GPT reasoning、单/并行工具、图像、usage/cache、
  typed failed/incomplete 与中断恢复，并请求/回放 opaque `reasoning.encrypted_content` 维持
  `store=false` 工具续轮；该 Data 不进入展示或导出。历史 Provider 默认仍为 Chat Completions。Anthropic Messages 现将
  effort 与 thinking 解耦，缺失 `message_stop` fail closed，并按原始顺序持久化 signed thinking 与 opaque
  `redacted_thinking` 供 tool-use 续轮回放；第一方预设用模型级 override 让 legacy-thinking Haiku 4.5
  省略 adaptive/effort，而 Sonnet/Opus 继承原生 adaptive wire。两条协议共用 Provider/boot/Controller/Agent，Desktop/TOML
  可显式选择且提供 OpenAI/Anthropic 第一方预设；OpenAI 预设按 2026-07-19 公共 API 文档开放
  `gpt-5.6-sol`、`gpt-5.6-terra`、`gpt-5.6-luna` 和 `gpt-5.4` 的原生普通 function-tool Responses。
  Codex catalog 的 `code_mode_only` 是产品 runtime 选择；P9 仍跟进 freeform/code-mode、Responses
  Lite/WebSocket、PTC、persisted reasoning/pro、显式缓存、hosted tools、multi-agent 与 App-Server。真实公网
  API 回环仍为 `external-blocked`；最终公开交付仍要求该 push 对应 CI/CodeQL 全绿。详见
  `audits/2026-07-19-p8-native-gpt-claude-provider-parity.md`。
- Hermes `bf391030..862b1b37` 暴露的空响应分类问题已完成同构修复：Provider 返回的 empty-response
  advisory 即使提到 `max_tokens`，共享 classifier 也返回可重试 server error 和 `ShouldCompact=false`；
  真实 context-window 溢出仍返回压缩建议。当前 Agent 自动压缩由 usage 驱动，尚未消费该分类合同，
  因此这是一项前置加固而非已复现的运行时恢复证据。
- 最新增量已审至 Codex `0fb559f0` 与 Hermes `614dc194`：Realtime V3 初始历史项、dynamic-tool/code-mode
  inline audio、paginated thread 的 metadata/summary/full legacy views 与 live-turn/cursor 合并进入 P9；IM durable
  recovery cursor/final-delivery Gateway core 已在 M6 落地，真实渠道历史分页/掉线回环仍待适配器与外部证据；单轮模型覆盖进入 P9；browser/computer-use 的验证后前台
  升级、按会话审批和 dead-driver 重连进入 P10。它们均未被包装成 P8 已完成能力。
- 参考项目最新增量已按锁文件人工接受：Kimi 的 Auto/YOLO 文案准确性已转化为三语权限契约测试；Hermes 的 session-state 单一投影、profile prewarm 和 best-effort stream fence、Codex 的集中 MCP runtime、MiMo 的 CLI-only 演示脚本路径均只作为架构回归信号，不引入 Python/Electron/Rust 或第二套 runtime。Reasonix 新增的多套生产 release workflow 不继承；Reames 反而增加全 workflow 发布写权限/动作棘轮，继续只允许无 secrets、`contents: read` 的候选构建。外部风险仍是公开 registry 运营、干净云节点 logout/reboot、真实 IM 回环和生产签名链。
- Hermes `7a43ab04..34e66a0d` 与 Impeccable `8967edc9..e4ab5e24` 已完成代码级增量分类：采用 Windows PowerShell 5.1 的 `install.ps1` 纯 ASCII 字节棘轮，并修复 Reames 全局凭据 `.env` 的 UTF-16 安全读取/成功写入后 UTF-8 规范化；UTF-32、损坏编码、嵌入 NUL 与截断 UTF-16 均拒绝写回且保留原字节。Hermes 的空白值 quoting、`key_env` 与动态 home 在 Reames 已有等价边界；Python completed-future timeout/OOM spin 对 Go context MCP 非同构；增量 Markdown lexer 是 `ReactMarkdown` 长回复的真实性能候选，但必须先证明 remark/rehype/GFM/math/Mermaid 的 AST/DOM 等价，不能直接复制；Impeccable inset-shadow stripe 检查仅作为未来非状态装饰的设计信号，其窄化到单规则与规范文件集合的豁免、未知策略参数 fail closed 和生成副本同步则作为治理信号。详见 `audits/2026-07-19-upstream-hermes-impeccable-delta.md`。
- 最新一级上游已代码级审至 Reasonix `43993f5a`：在 LongCat/Linux WebKit/安全会话导出和 Theme Pack
  等价回归基础上，本批继续采用数字开头 Provider env、MCP stdio server request/有界 reply queue、全部
  Desktop MCP/插件 lifecycle admission 与 visible/detached Controller reservation、graceful interrupted-turn
  recovery 和 WebKit shortcut recorder focus；最新插件 Skill 修复又补齐 package-owned MCP provenance、
  raw/visible/portable/Claude-style 名称到唯一 canonical tool 的解析、host-only runtime binding、稳定 Provider
  schema，以及 permission/Plan Mode/Hook/Evidence/子代理统一 canonical identity。Reames 保留 identity-bound
  trust 与冷崩溃 checkpoint rollback，且不虚构当前不存在的 `use_capability` proxy；
  Remote SSH UX/host-key 两项继续进入 P11。Codex 二级战略审至 `678157ac`，Claude 仍为 `015170d3`；
  Hermes `e361c5e2` 的 Kimi adaptive thinking 信号已补齐无签名 provider-native block 的窄回放，cron
  profile/Windows `simple-git` 路径仍只按机制层分类；Codex raw replay retention、finalized Markdown cache
  和 diff cloning 作为 P9 合同/性能信号；最新三提交又固定 paginated explicit name、动态 cell 重测及
  fresh/fork/resume subagent backfill/request 复用边界。Hermes 最终审至 `a7d7c02c`：custom endpoint/目标
  Provider 刷新已有等价，selector race 进入回归合同，cold-start/first-token/唯一调试端口/warm-cache 方法进入
  P9/P10 benchmark；全零 revision fallback 因削弱 provenance 被拒绝。MiMo `ec413ade` 的 checkpointed
  learning Skill 只作未来通用 Skill 状态机制候选；Grok Build `ba76b0a6` 不改变层级。完整结论见
  `audits/2026-07-20-reasonix-8bb0e54-2301e24.md`、
  `audits/2026-07-20-reasonix-2301e24-43993f5.md` 与
  `audits/2026-07-20-upstream-strategic-reference-delta.md`、
  `audits/2026-07-20-codex-hermes-late-delta.md` 与
  `audits/2026-07-20-codex-hermes-final-delta.md`、
  `audits/2026-07-20-hermes-mimo-final-delta.md`。
- 新增 `internal/testenv` 并接入会写状态的 Go/Desktop 测试包，隔离 HOME/USERPROFILE、XDG、AppData、TEMP/TMP 以及 Reames home/state/cache override；本批测试不会再默认把配置、worktree lease 和大量临时小文件写入真实用户目录或 C 盘通用 Temp。
- 继承自早期迁移的 Hermes/Python runtime、Electron/TUI、旧 plugins/tests/package 元数据以及 `site/`、`workers/` 已在完成运行引用和替代实现审计后从当前树删除；参考机制只保留在 Git 历史和 `F:\code-reference`，不得重新整套 vendor。

代码与测试驱动的初始接管结论见 [audits/2026-07-09-takeover.md](audits/2026-07-09-takeover.md)。

## 近期产品标准

桌面端应让用户完成一个完整任务闭环：

```text
创建/恢复会话
→ 选择工作区和模型
→ 提交任务与附件
→ 查看推理、工具进度和文件变更
→ 批准或拒绝副作用
→ 停止、继续、回退或恢复
→ 获得带证据的结果
```

主界面使用中文优先、低噪音的桌面产品语言。工程术语、迁移阶段和上游名称不应进入普通用户主流程。

## 明确不做

- 不自动合并或自动升级上游源码。
- 不用旧 Reames Lite 的 Python provider/cache 覆盖 Reasonix 现有管线。
- 不把桌面端做成 CLI 日志壳或工程状态仪表盘。
- 不为追求“全能”而复制整套参考项目。
- 不在没有行为测试或真实验证时宣称功能已经完成。

## 文档治理

- 产品方向只更新本文。
- 优先级、里程碑和验收只更新 [DEVELOPMENT_PLAN.md](DEVELOPMENT_PLAN.md)。
- 架构事实更新 [ARCHITECTURE.md](ARCHITECTURE.md)。
- 上游来源和接受规则更新 [REFERENCE_GOVERNANCE.md](REFERENCE_GOVERNANCE.md)。
- 一次性调查、审计和验证记录放在 `docs/audits/`，不得冒充当前计划。
- 已被替代或无法反映当前代码的文档直接删除；Git 历史承担归档职责。
