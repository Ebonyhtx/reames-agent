# Reames Agent 发展计划

> 状态：当前唯一执行路线
>
> 更新：2026-07-15
>
> 规划方式：先关闭真实用户闭环，再扩展能力面

## 北极星

让用户能够把一个真实任务交给 Reames Agent，并在 Desktop、CLI 或远程入口中安全地观察、控制、恢复和验证整个执行过程。

“全能 Agent”不是工具数量的堆积，而是以下能力的乘积：

```text
可靠运行 × 可控副作用 × 持久状态 × 可扩展能力 × 多入口一致性
```

## 优先级规则

1. 阻断真实任务闭环的问题高于新增功能。
2. 安全、数据丢失和提示词污染问题高于体验优化。
3. 共用 runtime/control 能力高于单端特例。
4. 可测试的纵向闭环高于横向铺设空接口。
5. 上游变化先形成 Issue 和采用建议，再进入本计划。

## 路线总览

| 阶段 | 目标 | 完成门槛 | 状态 |
|---|---|---|---|
| M0 基线可信 | 每次提交可构建、可测试、可发布复现 | 核心/Desktop/前端 CI 全绿；干净 clone 验证；无损品牌资产 | 已完成 |
| M1 真实任务闭环 | 用真实模型在原生 Desktop 完成一次受控改码任务 | 流式输出、审批、文件变更、停止/恢复、会话恢复均有证据 | 已完成 |
| M2 统一控制面 | 四个入口共享命令、事件、错误和权限语义 | 建立依赖守卫；消除关键入口对 runtime 内部的绕行 | 已完成 |
| M3 桌面日用化 | Desktop 达到稳定、清晰、低噪音的主产品体验 | 核心路径点击测试；设置/审批/变更/恢复完整；体积与启动性能达标 | 已完成 |
| M4 Agent 可靠性 | 长任务能计划、分解、验证、压缩和恢复 | Goal/Plan/子任务/证据/检查点形成统一状态机和失败恢复测试 | 已完成 |
| M5 扩展生态 | Skill、Hook、MCP、插件可发现、安装、授权和诊断 | 包格式稳定；安装/升级/禁用/权限/故障隔离 E2E | 进行中 |
| M6 远程与多渠道 | Gateway service、Server/Web 可安全承载同一任务闭环 | 独立后台 gateway、跨平台 service manager、鉴权、租约、重连、渠道 envelope、部署和运维验证 | 进行中 |
| M7 通用工作能力 | 从编程扩展到研究、文档、数据和自动化 | 每类至少一个端到端工作流；复用同一权限和证据模型 | 待开始 |

## M0：基线可信

当前批次：

- [x] 核心、Desktop、前端测试与构建恢复为绿色。
- [x] CI 纳入 nested Desktop module 和前端 production build。
- [x] 修复损坏的 PNG/ICNS/ICO 与残留字标。
- [x] 建立官方上游追踪、分类、Issue 生命周期和人工接受机制。
- [x] 收敛当前文档入口，删除失真路线图和外来文档。
- [x] 推送当前改动并观察远端 CI（run `28998792475`：5 个 jobs 全部通过）。
- [x] 在干净 clone 中验证 root build/vet/关键契约、Desktop 全测和前端 frozen-lockfile build。
- [x] 隔离继承自上游、仍绑定 `esengine`/R2/npm/Homebrew 的生产发布 workflow。
- [x] 触发并核验无发布权限的六目标 CLI candidate workflow（run `29000371845`，工件 60,041,869 bytes）。
- [x] 建立三平台 native Desktop candidate 打包（run `29015844761`：Linux、Windows、macOS 全部通过）。
- [x] 下载三平台 Desktop candidate artifacts 并完成内容级 smoke（Windows portable zip 含 update helper；Linux tar/deb、macOS zip/dmg 结构符合预期）。
- [x] 完成 Windows portable Desktop candidate 启动冒烟（隔离 `REAMES_AGENT_HOME`，启动 12 秒未崩溃）。
- [x] 完成 Linux/macOS Desktop candidate 安装/启动冒烟（run `29070605386`：Linux 安装 `.deb` 后通过 Xvfb 可见窗口验证；macOS 挂载 `.dmg`、复制并校验 universal `.app` 后启动 12 秒；两端状态边界均无变化）。
- [x] 完成 Windows NSIS candidate 安装/启动/卸载冒烟（run `29070966084`：静默安装实际 installer、验证 HKCU 注册与 update helper、运行安装后二进制 12 秒并静默卸载，文件和注册清理通过）。
- [x] 将 updater 迁移到当前 GitHub 仓库，并默认关闭无项目自有服务的遥测、指标和崩溃上传。
- [x] 将 Docker、compose、systemd、部署命令和 `serve.token_env` 纳入 CI 部署契约检查。
- [x] 建立公开仓库前门禁：公开说明、所有权、许可证/NOTICE、生产发布禁用和遗留 worker 手动部署隔离。
- [x] 公开仓库后恢复 CodeQL workflow，覆盖 Go、JavaScript/TypeScript 和 GitHub Actions，并完成远端全绿核验。
- [x] 明确版本号来源、变更日志和发布签名策略，并纳入 CI 发布契约检查。
- [x] 为安装器补显式 release artifact 模式和 `SHA256SUMS` 校验 dry-run 契约，但默认仍保持 source 构建，避免 pre-stable 阶段误导为稳定发布（见 `audits/2026-07-09-installer-release-mode.md`）。
- [x] Desktop 支持 `--home <path>` / `--home=<path>` 命令行参数，在 NewApp 前设置 `REAMES_AGENT_HOME`，实现不同 home 的独立 single-instance 锁和状态隔离。
- [x] 新增 Windows 原生 Desktop 启动 smoke 脚本（`scripts/smoke_desktop_native.py`），支持隔离 home、响应性观察、状态围栏检查和 JSON 证据输出。该基础 smoke 保持进程级；frameless 截图通道的 `0x80004002` 初始阻断及后续 UIA 解法分别见 `audits/2026-07-10-windows-native-smoke-attempt.md` 与 `audits/2026-07-10-windows-native-interaction-smoke.md`。
- [x] 最终核验远端普通 CI run `29072070070` 的 8 个 jobs 全部通过，CodeQL run `29072070100` 的 Go、JavaScript/TypeScript、Actions 全部通过；三平台原生安装 candidate run `29070966084` 全部通过，M0 的可构建、可测试、可安装复现门槛关闭。
- [x] Desktop 的“暂时跳过密钥配置”改为持久化显式选择；显式 `--home` / `REAMES_AGENT_HOME` 同时隔离 WebView2 user data，避免 localStorage、cookie 或缓存跨 home 复用。

验收命令：

```powershell
go build ./...
go vet ./...
go test ./internal/... -count=1 -timeout 300s
Push-Location desktop; go test . -count=1 -timeout 300s; Pop-Location
Push-Location desktop/frontend; corepack pnpm test:all; corepack pnpm build; Pop-Location
```

## M1：真实任务闭环

按以下顺序推进：

1. [x] 使用真实 API Key 完成最小文本对话，记录 Provider、缓存和使用量证据（见 `audits/2026-07-09-real-provider.md`）。
2. [x] 在原生 Wails 中完成新建会话、选择工作区、发送和停止（截图无关的 Windows UIA 驱动通过 `InvokePattern`、`ValuePattern`、稳定 AutomationId 和焦点窗口消息操作真实 WebView，`SendInput` 仅作回退；隔离 home 内的 loopback OpenAI 兼容端点收到 marker，用户/助手消息进入 canonical 事件账本；跨 Git Bash/PowerShell 的 Python 长命令出现 Stop 并被取消，见 `audits/2026-07-10-windows-native-interaction-smoke.md`）。
3. [x] 执行一次需要文件写入的任务，验证审批、补丁预览、落盘和回退（自动化锁定真实 `write_file`：审批请求 diff、ToolDispatch diff、磁盘写入、RewindCode 删除，见 `audits/2026-07-09-m1-file-write-loop.md`）。
4. [x] 关闭并重启应用，验证会话、待处理状态和工作区恢复（前端/后端自动化覆盖 pending approval replay、tab/workspace/pinned session/history；Windows 原生 smoke 进一步关闭并重启同一安装后二进制，确认同一项目、同一 session path、用户 marker 和 assistant response 均恢复，见 `audits/2026-07-09-m1-reconnect-recovery.md`、`audits/2026-07-10-windows-native-interaction-smoke.md`）。
5. [x] 对失败场景补自动化：安装形态共用的 Windows UIA smoke 在 production Wails 中连续验证合成无效 key 的 401 warning、429 可见重试与自动恢复、4 次流中断后的 warning 和部分输出持久化、真实 `write_file` 原生审批拒绝且不落盘、真实 `bash` 1 秒超时错误卡；每类均验证 Stop/Send 归零与后续成功 turn，原有停止和重启恢复合同继续通过（见 `audits/2026-07-09-m1-failure-contracts.md`、`audits/2026-07-09-desktop-m1-failure-display.md`、`audits/2026-07-11-windows-native-failure-recovery.md`）。

真实密钥不得写入仓库、测试日志或截图。没有可用密钥时，先完成可自动化的原生桥接与失败路径，密钥 E2E 保持显式阻塞。

## M2：统一控制面

- [x] 用 Go AST 依赖守卫冻结 Desktop、CLI、Serve、Bot 和 ACP 对 `agent/provider/tool` 的现有直连，禁止依赖面继续增长（见 `audits/2026-07-10-control-boundary-ratchet.md`）。
- [x] 关闭结构化错误纵向路径：`ErrorInfo` 的 code/category/message/retryable 穿过共享 event wire，Desktop 按 code 本地化、按 category 决定严重度与动作；认证错误直达模型设置、可重试错误可重试或续接、取消不再伪装成失败，同时保留旧 `err` 兼容（见 `audits/2026-07-11-m2-error-session-control.md`）。
- [x] 定义版本化稳定 command DTO：`control.Command` / `CommandResult` 以判别 payload 表达提交、取消、审批和状态，协议错误使用稳定 code；调用方不能从 JSON 选择 trusted scope，并发第二个 submit 以 `busy`/HTTP 409 明确拒绝而非静默丢弃。Desktop、CLI、Serve、Bot、ACP 的对应生产入口已迁移，HTTP `POST /command` 与 WebSocket `method=command` 提供新合同，旧 HTTP/WS 入口保持兼容并共用远端策略（见 `audits/2026-07-11-m2-command-control.md`）。
- [x] 定义版本化稳定 event/display DTO：`eventwire` envelope 固定 `version=1`，补齐 `source`、独立 `cache_updated` diagnostics 与 session cache counters；Desktop reducer 实际消费缓存更新。新增 `control.TranscriptMessage` 作为展示安全的会话投影，Serve history 与 ACP replay 不再接收 `provider.Message`，隐藏 system、合成恢复指令、compose 控制块和 referenced-context payload（见 `audits/2026-07-11-m2-event-transcript-metadata.md`）。
- [x] 收口会话、装配、设置和终端渲染边界；稳定 control API 承载列表/顺序/用户消息/恢复/lease/cleanup/trash/rebuild/copy/topic-binding、原子 session-meta mutation、opaque loaded/history handoff 与展示安全 transcript。Provider 注册、review subagent 和 session title 由 boot 拥有，CLI ANSI 输出由 termrender 拥有，MCP 名称合同独立于 tool registry；同步 `RunTurn` 只保留给拥有 turn 生命周期的 CLI/Bot/ACP（见既有 M2 审计与 `audits/2026-07-12-m2-transport-boundary-closeout.md`）。
- [x] 保持 provider prompt 与 UI/渠道 metadata 分离：Agent/Coordinator 在 Provider interface 前剥离 citations/edit/original 等本地展示字段，OpenAI/Anthropic wire bytes 与缓存前缀回归证明本地 metadata 不改变请求；Gateway 仅保留显式参与者标签，不把 connection/domain/chat/user/operator/message ID 注入 prompt（见 `audits/2026-07-11-m2-event-transcript-metadata.md`）。
- [x] 按可验证纵向路径将 transport allowlist 收缩至空；Desktop、CLI、Serve、Bot 与 ACP 的生产文件均无 `agent/provider/tool` 直连，累计删除四十条受守卫 import，AST 门禁禁止重新引入。

## M3：桌面日用化

- 主流程：会话、项目、输入、附件、工具进度、审批、变更和结果。
- 设置中心：模型/密钥、权限、MCP/插件、记忆、网络、外观和更新。
- 原生体验：窗口生命周期、快捷键、拖放、文件选择、通知和自动更新。
- 可访问性：键盘导航、焦点、对比度、缩放和屏幕阅读语义。
- [x] 性能首批：关闭态/次级界面按真实打开状态拆包，构建后强制 entry、初始 JS/CSS、最大 chunk 与请求数预算；入口 chunk 从 1,103,017 B 降至 621,270 B，初始 JS 从 1,342,548 B 降至 1,209,699 B（见 `audits/2026-07-12-m3-desktop-bundle-budget.md`）。
- [x] 建立 Windows 原生 Desktop 冷启动硬门槛：本地源码 production smoke 要求 8 秒内达到连续三次响应；托管 runner 的首次安装候选依据 11.531 秒首次响应实测采用 15 秒门槛与 20 秒观察窗，两层不得互相替代。已交付性能批的本地 production Wails 冷启动首次可见/响应 1.015 秒、稳定响应 2.015 秒；同 HOME warm relaunch 首次可见/响应 0.516 秒、稳定响应 1.516 秒，隔离 HOME 边界无泄漏（见 `audits/2026-07-13-m3-main-graph-css-split.md`）。
- [x] 可访问性首批：统一真正模态层的初始焦点、Tab/Shift+Tab 围栏、嵌套顶层判定、退出动画后 opener 恢复和 `aria-modal`/读屏关联；命令面板补 combobox/listbox active-descendant 合同，设置/历史/图片/首次引导/快捷键帮助共用同一生命周期（见 `audits/2026-07-13-m3-modal-focus-accessibility.md`）。
- [x] Windows 显示缩放闭环：连续滑动按最后选择串行合并写入，Go 偏好使用原子替换并拒绝非有限值；设置页区分启动已应用/保存中/待重启，提供立即重启与失败回滚，组件和真实浏览器覆盖 100% → 105% → 100% 状态（见 `audits/2026-07-13-m3-display-zoom-persistence.md`）。
- [x] 主题对比度与焦点纵向合同：六套视觉风格同时覆盖深/浅色、普通/创作模式的小文本、状态色、主按钮与焦点指示器，自动浅色必须与显式浅色一致；补 forced-colors 焦点规则、局部画布焦点环重算和入口重挂载后的语义焦点恢复，并用真实浏览器切换 Graphite/Carbon/Amber 及创作模式核验最终计算值（见 `audits/2026-07-13-m3-theme-contrast.md`）。
- [x] Windows warm relaunch 门槛：native smoke schema v3 在冷启动关闭后复用同一隔离 HOME/WebView2 profile 启动第二个真实进程，独立记录可见/响应/稳定时间、预算、早退和清理；托管安装器 candidate 强制冷启动 15 秒与 warm 6 秒预算，本地源码 production 仍保持冷启动 8 秒，对应历史本地审计的 cold/warm 稳定响应为 1.516/1.500 秒（见 `audits/2026-07-13-m3-windows-warm-startup.md` 与 `audits/2026-07-13-m3-lazy-locale-budget.md`）。
- [x] Linux/macOS 启动预算：candidate smoke schema v2 要求隔离 HOME 的 Desktop 状态连续三次就绪且不泄漏默认状态，Linux 同时要求最终仍有可见窗口；run `29209723618` 的 Linux 首次状态/窗口就绪为 4.538 秒、稳定就绪 5.567 秒，macOS 首次状态就绪 0.575 秒、稳定就绪 1.872 秒，均通过 10 秒门槛。macOS 证据只声明状态 readiness，不冒充窗口可见性（见 `audits/2026-07-13-m3-linux-macos-startup-readiness.md`）。
- [x] Desktop 重启恢复基础竞态：后端首次 `ListTabs` 等待 `tabsRestored` 门闩，并补 canonical event log 半行容错与 UIA Send fallback。该阶段曾把首次 history read 延后到 restored controller `ready=true`，candidate `29211681563` 的 Windows installer 因当时 controller 在 30 秒内就绪而完成 19 请求、五类失败恢复、停止和原会话/工作区/消息重启恢复；后续更慢 runner 暴露的 transcript 可见性回归由下方安装器级收口项继续跟踪（见 `audits/2026-07-13-m3-desktop-restart-restore-race.md`）。
- [x] 多语言首启拆分：英文保留同步兜底，简中/繁中各自按需加载；首个 React frame 前先从轻量本地 bridge 读取权威保存语言，auto 才使用 OS，只预取最终离线 chunk；运行期切换保持旧词典直至新词典完整到达。production base initial JS 从 1,213,626 B 降至 984,616 B，最坏本地化首启为 1,100,036 B，并由 manifest 递归依赖图、双 locale chunk 数量与 1,150,000 B 本地化预算共同守卫（见 `audits/2026-07-13-m3-lazy-locale-budget.md`）。
- [x] 性能后续：浏览器开发 mock 改为首次使用时动态加载，VirtualMenu/TanStack 虚拟化实现移出首启图，设置中心约 120 KB 源码 CSS 由延迟 `SettingsPanelRoute` 装载；真实 manifest 递归预算分别约束 base、本地化、browser mock、VirtualMenu 和 Settings 首次使用图，并禁止延迟路由被静态提升。production base initial JS 从 984,616 B 降至 865,678 B（-12.1%），initial CSS 从 611,424 B 降至 511,305 B（-16.4%），首屏交互与延迟界面样式合同保持不变。该批已由 commit `7d07c89` 交付；普通 CI `29216174519` 8/8、CodeQL `29216174514` 3/3（见 `audits/2026-07-13-m3-main-graph-css-split.md`）。
- [x] 可访问性后续：七个真正模态层使用稳定 dialog ID 和共享租约栈，在嵌套、退出动画、快速重开及动态 portal 场景中只隔离顶层模态之外的辅助技术树并精确恢复原属性；租约继承直接父控件与原始 opener 链，模态替换和 StrictMode effect 重放后仍能恢复焦点。History Escape 只关闭顶层，Settings 显式恢复 opener，非阻断 PromptShelf 保持可交互。Transcript 提供非逐 token 的日志语义和最终答复单次状态通知，历史预览不重复 ID/announcer；Windows production Wails 以严格 InvokePattern 验证跳转焦点、设置 dialog、背景树隔离和 opener 恢复（见 `audits/2026-07-13-m3-modal-isolation-transcript-uia.md`）。candidate `29262541971` 已在 installed Windows 上实际执行 strict accessibility，skip/composer focus、Settings dialog、背景隔离、dialog focus、opener focus 恢复与 strict InvokePattern 均通过；NVDA/Narrator 听感与 Windows High Contrast 仍需手动或外部证据。
- [x] 安装器级 M3 收口：commit `68218d6` 的 CI `29262192635` 8/8、CodeQL `29262193090` 3/3；candidate `29262541971` 的 Linux/macOS installed 与 Windows native/interaction/accessibility 全部通过。Windows installer SHA-256 为 `2BDAA4E9FC5E87CD498A9E528D49F480B8277B7D9B4514081EF11E2C674D6C19`，executable SHA-256 为 `927FEF13D22B0F609DDC72FA35D0BF07451CAC6402BA2CBEBA38456E8D8010F1`，cold/warm 稳定响应 7.031/2.016 秒。interaction 完成 19 请求、五类失败恢复、停止、磁盘双消息和同 project/workspace/session；跳转前 marker present 但 offscreen、assistant 不在 UIA 且 assistant onscreen 标志为 false，严格 InvokePattern 调用问题 1 后 user/assistant 均 present + onscreen，`recovery_verified=true`。strict accessibility 随后实际执行并全部通过；三份 smoke 均为 `boundary_changes=[]`、无 errors（见 `audits/2026-07-13-m3-installed-history-completeness.md`）。

UI 改动必须同时提供组件测试和一次真实浏览器或 Wails 点击验证。

## M4：Agent 可靠性

- [x] 第一批统一 Goal/Plan/Todo/Checkpoint 的会话恢复投影：v2 sidecar 持久化 continuation、blocker、intercept、idle、strict self-check、PlanMode、canonical Todo、message count、transcript digest 与 monotonic revision；每个完整 turn、history rewrite 和 Goal transition 刷新，同进程相同路径的 revision 检查/替换串行化，旧 revision 不覆盖新状态；headless `Controller.Run` 复用同一 orchestrator 生命周期。
- [x] 删除非 strict 的二次 completion override 和宿主伪造 `goal-final` Todo 完成；所有模式均经过 Todo/project checks/evidence gate，strict 只额外要求实际宿主 self-check turn，被 PromptSubmit hook 拦截的 synthetic turn 不计为自检并会重新排队。
- [x] Resume/Switch/Branch/Fork/Rewind 使用 replace-style runtime 恢复；相等 anchor 直接恢复 Todo，append-only extension 从 sidecar 基础重放后缀事件，compaction/rewrite divergence 不接受旧投影；新建/清空会话不继承 Goal 或 PlanMode，Branch/Fork post-save 失败清除全部 partial artifacts。
- [x] Checkpoint runtime projection 随 turn-start prefix digest 持久化；conversation rewind/Fork/Summarize 对同长度 divergence、legacy 无 digest和负边界 fail closed，rewind/Fork 还拒绝无效 runtime。代码恢复拒绝工作区逃逸与 symlink/reparse，规范化 path/case/hard-link alias，中途失败回滚已成功及可能部分写入的目标；durable truncate tombstone 阻止删除失败后复活并保持 turn ID 单调，损坏 manifest 不接纳任何旧 checkpoint 并在下一 turn 安全修复。该批当时保留的原子替换与跨资源窗口由 M4 最终批次关闭。
- [x] Board 只把 `status=running` 的 Goal 标为 active，并展示 Controller 当前 turn 的真实 evidence 摘要；明确该摘要不是 durable proof。
- [x] 为 `task`、`read_only_task`、`parallel_tasks` 和 subagent skill 建立每棵委派树共享的并发、token、time、step 与 root/child cancellation 账本：默认最多 3 个活跃 Provider round、聚合 100 round，token/time 可选；并发槽只覆盖 Provider stream，避免父代理等待嵌套 child 时占槽死锁。前台树继承 parent turn，后台 `task` 由 session/job context 托管；预算字段仅由用户级配置控制。
- [x] 将 writable 子代理的文件变化、checkpoint 与 evidence 结构化归并到父任务：prompt 不可见的 effects bridge 只传播结构化 read/write/command receipt、父 tool-call provenance 和 pre-edit callback，不传播 child transcript/模型正文/工具输出；成功 child write 与其后的验证可满足父级当前 turn project checks，失败或取消的 previewed writer 形成不可信 mutation boundary 并要求后置验证，父 checkpoint 可回退 child 创建/修改。后台 child 跨 turn 继续运行，但 evidence generation 与 turn-scoped checkpoint callback 会拒绝把旧 turn 的迟到效果记入新 turn。
- [x] 建立跨 continuation 的成功读取/验证循环和可恢复的最小 durable evidence 引用：runtime sidecar 只保存项目检查命令哈希与 root bash tool-call ID，不保存命令正文或输出；writer/mutation attempt 开启新验证 epoch 并清空旧引用，同一检查的最新失败覆盖旧成功。恢复只在 transcript count/digest 精确相等时进行，并要求引用解析到最新 writer 之后该检查最新一次成功 tool result；append/divergence、丢失/失败引用和新 Goal 均 fail closed。child-only bash 仍只作为当前 turn 证据，跨 continuation 必须由 parent/root 重新核验（见 `audits/2026-07-14-m4-durable-evidence.md`）。
- [x] 将 checkpoint/runtime 持久化失败向 previewable writer fail closed 传播：turn allocation、checkpoint record、runtime sidecar 或 in-flight marker 任一失败均在工具执行前拒绝；marker 还必须匹配当前 session、message boundary 与 `preserveUser`，旧 turn marker 不得误放行。Snapshot 失败回滚内存 `seen/Files`，可安全重试，晚到 child callback 以错误拒绝而非静默丢弃。root 写前门禁与 autosave recovery/session rebind 共用 handoff 锁；持久快照在模拟 writer 部分写入并进程消失后可由新 Store 恢复原字节。无静态 preview 的 `bash` 和后台 child journal 不在该批声明内（见 `audits/2026-07-14-m4-writer-persistence-gate.md`）。
- [x] 完成后台 Task crash-resume、上下文压缩与记忆检索的统一恢复：持久 subagent 在首个 Provider round、assistant tool-call envelope、tool result、retry/nudge/final 与 compaction rewrite 边界同步 transcript；recoverable job 只在 running metadata 成功落盘后启动，冷启动把遗留 running 转为可读取的 `interrupted` tombstone 并给出 `continue_from`，不自动重放可能有副作用的工具。显式续跑会收到重新核验 workspace 的恢复上下文；compacted transcript 可经 running→interrupted→continue 完整恢复。BM25 记忆命中保留 score/path/snippet，空 store 可关闭、归档删除后不再命中，动态结果只进入 tool result 而不改变稳定 system prefix（见 `audits/2026-07-14-m4-task-compaction-memory-recovery.md`）。
- [x] 将 previewable built-in writer 与 checkpoint restore 收口到 handle-relative resolve-beneath I/O：`write_file`、`edit_file`、`multi_edit`、`delete_range`、`delete_symbol`、`notebook_edit`、`move_file` 和 `apply_patch` 先把目标绑定为 `os.Root` + 相对路径，再通过同一 handle 完成 read/stat/temp-write/fsync/chmod/rename/remove；symlink/reparse escape 与组件替换测试证明外部文件不被触碰。`move_file` 预览 source delete + destination create，multi-file `apply_patch` 先全量 preflight、逐文件原子替换并在中途失败时回滚，Agent 会在执行前快照全部 preview change。Checkpoint restore 的预检、删除、写入和回滚复用单一 workspace `os.Root`。该声明不覆盖 shell/MCP/external API，也不恢复 ACL/xattr 或硬链接身份。
- [x] 建立 durable child effect journal：每个顶层持久 `task` 或 writer-capable `run_skill` ref 使用 0600、有版本、1 MiB/256-event 有界 sidecar；previewable child writer 在执行前先持久化 mutation intent，失败则不执行，结果 receipt 先落盘再发布给祖先。Sidecar 不保存 child model text、tool output、参数/Todo/step，命令字段先做现有 secret redaction；parent session、workspace、delegation tool-call、journal ID/sequence 与 runtime cursor 共同防止跨分支、替换和重复重放污染。崩溃恢复的 child mutation 会清空旧 root checks，已确认 cursor 不会在父 transcript compaction 后重复失效较新的 root 验证；child-only bash 仍不能成为跨 continuation 的 root proof。损坏/不匹配 journal 每进程首次 fail closed 并要求 root 重验（见 `audits/2026-07-14-m4-rooted-writers-child-effects.md`）。
- [x] 关闭 previewable writer active-turn 与用户 Rewind 的 transcript/runtime/checkpoint/workspace 跨资源断电恢复窗口：每个 orchestrated turn（含隐藏 synthetic continuation）分配独立 checkpoint，in-flight marker 绑定 checkpoint turn；成功路径依次持久化 transcript、runtime 和 commit anchor 后清 marker，冷启动只有在 transcript/runtime 同时匹配 anchor 时保留 workspace，否则回滚 workspace/runtime/partial transcript。Provider 已产生输出且耗尽 tail recovery 的显式流中断会提交可继续的部分 transcript/runtime 边界；其提交失败和其他 runner error 仍 fail closed 回滚，避免 Desktop 声称保留部分响应而持久层将其删除。Conversation/RewindBoth 使用 `prepared -> resources_applied -> checkpoint retirement -> clear` 的 durable sidecar journal，任一步断电都可重放；新 turn 以及 Compact/New/Fork/Branch/Switch/Summarize/Rewind 等保留内容的 rotation 操作前都会强制清算遗留事务。Transcript append event 现同步文件和首次目录项；`AtomicWriteFile` 不再原地复制，跨设备 rename 直接 fail closed，Windows 使用 write-through replace，Unix/Rooted writer 同步父目录。该完成声明不覆盖 `bash`、MCP、外部 API、后台 opaque side effect exactly-once，也不恢复 ACL/xattr/硬链接身份（见 `audits/2026-07-14-m4-cross-resource-recovery-transaction.md`）。

会话恢复第一批证据见 `audits/2026-07-14-m4-session-runtime-recovery.md`，共享委派预算见 `audits/2026-07-14-m4-delegation-budget.md`，可写子代理归并见 `audits/2026-07-14-m4-writable-subagent-effects.md`，跨 continuation 最小证据引用见 `audits/2026-07-14-m4-durable-evidence.md`，writer 持久化门禁见 `audits/2026-07-14-m4-writer-persistence-gate.md`，后台 Task/compaction/memory 统一恢复见 `audits/2026-07-14-m4-task-compaction-memory-recovery.md`，rooted writer 与 durable child journal 见 `audits/2026-07-14-m4-rooted-writers-child-effects.md`，跨资源收官见 `audits/2026-07-14-m4-cross-resource-recovery-transaction.md`。M4 已按路线图门槛关闭；opaque side effect、完整跨进程 evidence/委派账本和文件 metadata 保真仍是明确限制，不得反向扩大完成声明。

## M5：扩展生态

- [x] 固化原生 plugin manifest schema v1、语义版本和精确权限集合；Codex/Claude
  兼容包按实际能力推导权限，损坏原生 manifest 不回退到兼容格式。
- [x] 复制安装改为 `sha256-tree-v1` 内容寻址的不可变 generation，状态 v2 原子指向
  active/previous；新安装默认禁用，启用绑定精确 digest 与权限，更新权限扩张自动禁用。
- [x] 安装、更新、回滚和卸载接入确定性 preview/planId/apply；CLI 已覆盖真实
  install → enable → update → rollback → remove 生命周期，doctor 会验证完整性。
- [x] 状态 mutation 使用进程内互斥与跨进程文件锁；受管创建/发布/删除使用
  `os.Root` 相对操作，故障注入覆盖 copy/publish/state/rollback/uninstall/cleanup，
  tamper 和 mutable link 漂移在 runtime 前 fail closed。
- [x] Desktop 已自动化覆盖权限摘要、版本/权限差异、planId、update/rollback/remove
  两阶段确认、failed/partial/blocked 状态和显式 enable 授权；apply 缺失或使用旧
  planId 均 fail closed。
- [x] 插件 MCP owner 在 controller 加载时绑定并随同名用户接管更新；更新、回滚、卸载
  和禁用会用 controller runtime-mutation reservation 与 Desktop work-start gate 原子阻止
  检查后新 turn/rotation 起跑，串行化同步 rebuild、取消旧状态启动但尚未发布的异步
  build，并在所有 live/detached controller 精确撤销旧插件 MCP/Hook、暂停旧 Skill 入口
  直到重建或新会话；MCP connect/owner mutation 串行，同名用户 MCP 不受影响。
- [x] 取得真实 Chromium 与源码 production Wails 的插件安装、授权、更新、回滚、移除
  及 stale plan 失败恢复证据：Chromium 链明确标记 `backend=browser-mock`，原生链使用
  隔离 home、真实 Go 后端和落盘状态，两者不互相冒充；稳定 AutomationId 同时具备
  可访问语义，styled checkbox 通过聚焦后 Space 的标准键盘合同操作。
- [x] 将 `install_source` 的结构化审批计划接入统一 permission/control 流程：planning
  按调用级只读，apply 在执行前重算并绑定精确 `planId`/actions；Controller 强制 fresh-human
  决策，YOLO、Auto、Guardian、Plan 执行窗口、已有 grant 和 headless autonomy 均不能替代。
  Desktop、CLI、Bot、Serve/event wire 与 ACP 消费同一计划字段，pending replay 保留完整计划，
  MCP URL/command/args/env/headers 在展示和持久化前结构化脱敏；不支持结构化审批及 headless
  apply 的宿主在联网、读盘预览或执行前 fail closed。
- [x] commit `e9de895` 的普通 CI `29378573077` 8/8、CodeQL `29378573116` 3/3
  全绿；Desktop candidate `29378899444` 的 Linux/macOS/Windows 全部通过，Windows
  安装后二进制完成 interaction、accessibility、native 和 plugin lifecycle 四条 smoke，
  包括 stale plan、默认禁用、精确授权、更新、回滚、doctor、移除和状态边界检查。
- [x] 建立无默认 endpoint 的 TUF registry 客户端信任链：用户级配置和带外 bootstrap root
  不可被项目 TOML/`.env` 覆盖；官方 `go-tuf/v2` 持久 metadata 验证顺序 root 轮换、过期、
  rollback/freeze/mix-and-match，索引严格绑定 canonical GitHub full commit、树摘要、manifest
  版本/权限与 provenance assertion；apply 重解析并重算，registry/root/provenance/attestation
  证据跨 update/rollback 落盘。CLI 与 Desktop 可搜索、展示和预检 `registry:<name>`；可选
  attestation target 目前只做 TUF 完整性验证，不冒充 DSSE signer/SLSA policy 验证。插件
  state、安装请求与 registry 索引共享可移植 ASCII 名称身份，大小写别名、尾点和 Windows
  设备保留名在内容物化前 fail closed。
- [ ] 建立真实运营且有信任策略的公开 registry：完成生产 endpoint、离线 root/targets
  threshold ceremony、online snapshot/timestamp custody、实际轮换与 compromise drill，并接入
  独立 DSSE/SLSA policy verifier（若要声明 builder identity/SLSA level）。在此之前无默认
  registry；直接 GitHub 来源仍只记为 unsigned HTTPS + commit revision。
- [x] 已安装包拥有的 Hook/MCP 使用最小环境、独立 state/tmp、严格 OS sandbox 和敏感
  read barrier；backend 不可用 fail closed，运行中 Hook 撤销会取消进程树并封住 late-start
  竞态。manifest 环境变量名和值不进入 wrapper argv；隐藏 helper 在隔离边界内恢复并清除
  编码 child env，payload 缺失/损坏或宿主漏注册 dispatch 均 fail closed。Windows helper 已
  实际验证 child env/cwd、越界写拒绝和后代回收；Linux 私有 `/tmp` 后只读重挂不可变
  generation/helper，避免临时隔离 home 遮蔽代码且不扩大写权限；Linux/macOS
  保持平台实现与交叉编译/CI 证据边界，不外推为统一硬 CPU/RSS 配额。
- [x] 使用真实公开插件 `obra/superpowers@d72560e462a74e10d161b7f993d5fc3282bfa1e2`
  的固定 revision，在隔离 Windows home 中完成 copy install、精确授权、完整性验证和原生
  sandbox SessionStart E2E；来源仍明确标记 `github-https-unsigned`。
- [ ] 在最终候选提交上完成干净 clone，并确认关闭 M5 时最新提交的远端 CI/CodeQL 全绿。

第一批机制取自 Codex 的同盘 staging/失败恢复与本地/远端版本分离、Claude Code 的
显式安装同意和固定 revision 优先；Reasonix 保持工程底座，Reames Lite 只保留轻量
发现/启停体验而不沿用其弱供应链模型。详细证据和边界见
`audits/2026-07-14-m5-plugin-lifecycle-trust.md` 和
`audits/2026-07-15-m5-plugin-process-isolation.md`、
`audits/2026-07-16-m5-tuf-plugin-registry.md`。

## M6：远程与多渠道

- 服务器 CLI/TUI：单二进制安装、SSH/tmux 交互、`run` 一次性任务、服务器用户级 `REAMES_AGENT_HOME` 与真实 API key。
- Gateway service：独立后台进程承载飞书/微信/QQ/Telegram 等社交通道；已补 Linux systemd、Windows Scheduled Task、macOS launchd 生命周期、`REAMES_AGENT_HOME` service 绑定、`gateway run --home`、headless smoke，以及 `gateway setup` 的四渠道 secret-env-only、fail-closed access、redacted dry-run、原子幂等配置事务。credential-free 预检现用同一实际 CLI 二进制和隔离 home 覆盖 setup → doctor → service-plan、localhost Provider 一次性任务/会话持久化、feedback submit → summary → draft 和敏感值脱敏；WSL2 的真实 systemd user manager 又覆盖 unit 静态验证、带空格路径、install、同名重装立即生效、status、restart、stop/start、journal、webhook readiness、卸载后 `LoadState=not-found`，并修复 uninstall 顺序与 service unit 编码。Linux user-scope install 现进一步以旧 unit bytes/mode 和 enabled/active 快照做自动事务，`systemd-analyze --user verify`、前向命令、取消和回滚失败均有 fail-closed 故障注入；该保证不外推到 system scope、macOS launchd 或 Windows Scheduled Task（见 `audits/2026-07-10-headless-gateway-smoke.md`、`audits/2026-07-13-m6-gateway-setup.md`、`audits/2026-07-13-m6-clean-node-operations-preflight.md`、`audits/2026-07-13-m6-linux-systemd-lifecycle.md`、`audits/2026-07-14-m6-recovery-transactions.md`）。该 WSL 用户 `Linger=no`，下一步仍需在干净 Linux 云节点验证 logout/reboot 常驻和新恢复命令实启，再做真实 Provider 与 IM 渠道回环。
- Server/Web：作为可选远程控制面，提供鉴权、CSRF/Origin、租约、SSE/WS 重连、速率限制和审计。
- 部署：Docker、systemd、反向代理和健康检查已有基线；敏感的 home/state `backup create/verify/restore`、仅新目标恢复、候选/安装后健康检查、保留 immediate predecessor 的 updater 与 `upgrade --rollback` 已有本地自动证据。内嵌 manifest 只证明自洽，多根恢复没有 durable crash journal，公开签名 release 和干净云节点实启仍需外部证据。
- 反馈中心：已建立 `internal/feedback` schema、本地 JSONL 账本、`serve` 的 `POST /api/feedback`、`GET /api/feedback/summary` 与 `POST /api/feedback/draft`，以及 SSH 运维可用的 `reames-agent feedback submit|summary|draft --home PATH`，先完成脱敏、去重、本地聚合和维护草稿，再接人工确认后的 Issue 发布。
- Gateway：统一消息 envelope；渠道 metadata 不进入 provider prompt。
- 每个渠道先完成文本 + 审批 + 取消 + 恢复，再扩展媒体与富交互。
- 阿里云等自有服务器形态按 [云端 Agent 计划](CLOUD_AGENT_PLAN.md) 推进，先完成 SSH/CLI 与独立 Gateway service，再按需开启 `serve`，最后承载后台研究任务。

## M7：通用工作能力

在编程闭环稳定后，按共同基础设施复用程度扩展：

1. 深度研究与可追溯引用。
2. 文档、表格、演示文稿和 PDF 工作流。
3. 浏览器与桌面自动化。
4. 数据整理、分析和可视化。
5. 定时任务与长期监控。
6. 团队协作与外部服务连接器。
7. 上游和参考项目研究 Worker：自动发现、研究、建单、草稿分支或草稿 PR，但不自动合并。
8. 自托管遥测与反馈：崩溃、指标和用户反馈聚合为可审计维护任务，默认不上传源码、密钥或对话全文。

每类能力都必须复用权限、证据、检查点和恢复模型，不能成为绕过核心安全边界的旁路。

## 持续治理轨道

以下工作不单独等待某个里程碑：

- **安全**：密钥、沙箱、网络、供应链和提示注入持续审查。
- **上游**：每日发现，Issue 驱动审查，维护者显式接受版本。
- **质量**：新增模块同步测试；修复回归必须先固化失败用例。
- **性能**：启动、内存、token/cache、前端体积和长会话持续度量。
- **文档**：代码变化同步维护当前文档，Git 历史代替过时文档归档。
- **遗留清理**：仅在“无运行引用、无发布依赖、有替代实现或明确无价值”时删除。

## 每批交付规则

每个批次应当：

1. 只解决一个可描述的用户或工程闭环。
2. 先读取实现和既有测试，再修改。
3. 同步补测试和必要文档。
4. 执行与风险成比例的验证。
5. 使用逻辑清晰的提交；形成一批充分本地验证的成果后集中 push 并观察 CI，避免用碎片提交反复消耗远端资源。
6. 若改变优先级或范围，更新本计划而不是另建路线图。

## 当前下一步

当前执行顺序：

```text
M5 进行中：核心生命周期、installed candidate、统一模型宿主审批、旧 generation 撤销、
package-owned 进程隔离、真实第三方 E2E 与无默认 endpoint 的 TUF 客户端信任链已收口
→ 本批最终候选干净 clone 已收口，集中 push/CI 尚待关闭；真实运营 registry、生产密钥仪式、实际轮换/
compromise drill 和可选 DSSE/SLSA policy verifier 仍需要外部运营主体
→ 并行等待干净云节点、真实飞书和公开签名 release 外部证据，取得环境后关闭 M6 external-blocked 项
```
