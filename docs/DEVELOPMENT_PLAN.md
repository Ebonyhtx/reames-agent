# Reames Agent 发展计划

> 状态：当前唯一执行路线
>
> 更新：2026-07-19
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
| M5 扩展生态 | Skill、Hook、MCP、插件可发现、安装、授权和诊断 | 包格式稳定；安装/升级/禁用/权限/故障隔离 E2E | 仓库内已完成；运营项 external-blocked |
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
- [x] 将 updater 迁移到当前 GitHub 仓库；当时先默认关闭无项目自有服务的遥测、指标和崩溃上传，P4 已进一步永久删除这些远端上传路径。
- [x] 将 Docker、compose、systemd、部署命令和 `serve.token_env` 纳入 CI 部署契约检查。
- [x] 建立公开仓库前门禁：公开说明、所有权、许可证/NOTICE、生产发布禁用和遗留 worker 手动部署隔离。
- [x] 在完成运行引用、发布依赖和替代实现审计后，删除隔离的 Hermes/Python runtime、Electron/TUI、旧 plugins/tests/package 元数据及 `site/`、`workers/`；建立 public-readiness legacy-tree/品牌棘轮，参考实现只保留于 Git 历史和外部参考仓库。
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
- [x] M4 持续加固：现有 writer-capable `task`/Skill/Subagent 使用独立 `reames/subagent-*` branch/worktree 与跨进程 workspace/ref 锁；workspace built-in 重绑到 child root，非 Git writer fail closed，只读委派不分配 worktree。取消/崩溃保留 Git-derived manifest，lost/orphaned 可诊断；移入回收站保留、永久删除先清理 worktree。父会话通过统一 Controller preview/apply/merge/rollback/reject，CLI/Serve/Desktop 只投影同一 DTO。source mutation 前持久化 acceptance intent；断电后可证明未变更或已完成 merge 才自动恢复，ambiguous apply 标记 `acceptance_interrupted` 并拒绝覆盖。Windows long path、junction、隐藏 Git 进程、嵌套 child、dirty source 与真实跨进程锁已有回归（见 `audits/2026-07-17-m4-writer-worktree-isolation.md`）。

会话恢复第一批证据见 `audits/2026-07-14-m4-session-runtime-recovery.md`，共享委派预算见 `audits/2026-07-14-m4-delegation-budget.md`，可写子代理归并见 `audits/2026-07-14-m4-writable-subagent-effects.md`，跨 continuation 最小证据引用见 `audits/2026-07-14-m4-durable-evidence.md`，writer 持久化门禁见 `audits/2026-07-14-m4-writer-persistence-gate.md`，后台 Task/compaction/memory 统一恢复见 `audits/2026-07-14-m4-task-compaction-memory-recovery.md`，rooted writer 与 durable child journal 见 `audits/2026-07-14-m4-rooted-writers-child-effects.md`，跨资源收官见 `audits/2026-07-14-m4-cross-resource-recovery-transaction.md`，writer worktree 隔离与父会话交付见 `audits/2026-07-17-m4-writer-worktree-isolation.md`。M4 已按路线图门槛关闭；opaque side effect、完整跨进程 evidence/委派账本、ambiguous apply 的自动归因和文件 metadata 保真仍是明确限制，不得反向扩大完成声明。

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
- [x] 建立只读生产 registry 运维审计与确定性轮换/泄露演练：必须显式提供带外 root，重放
  连续 root 的旧/新双阈值，强制四角色独立 canonical key、root/targets 2-of-3、受限到期
  窗口，并验证 timestamp/snapshot/targets、hash-prefixed index 与全部 attestation 字节；
  成功 JSON 仍强制列出人员 quorum、HSM、公开 endpoint/monitor 和 DSSE/SLSA policy 等
  外部证据，不把合成密钥测试冒充生产仪式。
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
- [x] 为用户配置和会话注入的 MCP 建立 identity-bound trust receipt 与 launcher lock：receipt
  必须绑定 transport、真实 executable/hash 或规范化 HTTPS endpoint、args、env/header 键名、
  配置来源及 tool schema/read-only/destructive 指纹；identity/capability drift 自动撤销 reader
  authority 并要求 fresh-human 复核。`npx`/`uvx`/Git 等可变 launcher 必须固定 exact
  version/content；cache、lazy connect、plan mode、read-only subagent 与普通执行共用同一评估，
  不能继续仅凭 `trusted_read_only_tools` 的 raw name 跨配置变化授权。实现使用宿主本地
  `mcp-security.json`、跨进程锁与原子写；legacy raw-name list 只在首次 live handshake 迁移一次，
  UI/审批不再改写配置。身份漂移在启动子进程/网络前阻断，Desktop 只允许显式 reverify；
  capability drift 只撤销变化工具，新增工具不自动授权。`destructiveHint` 永远要求 fresh-human，
  Auto/YOLO/Guardian/记忆规则不能代答。共享 Desktop Host 的重连会同步刷新兄弟 tab registry，
  避免保留已关闭 client 的 stale adapter。详细证据见
  `audits/2026-07-17-m5-mcp-identity-trust.md`。
- [x] 使用真实公开插件 `obra/superpowers@d72560e462a74e10d161b7f993d5fc3282bfa1e2`
  的固定 revision，在隔离 Windows home 中完成 copy install、精确授权、完整性验证和原生
  sandbox SessionStart E2E；来源仍明确标记 `github-https-unsigned`。
- [x] 在提交 `13016c6` 上完成 TUF client 的干净 clone 验证；随后提交 `9295f8b` 收口只读
  registry 运维审计、确定性轮换/泄露演练和 Node.js 24 workflow 迁移。最终 clean clone 的
  root、Desktop、前端与合同全绿且构建后 tracked 工作树干净；普通 CI `29510215514` 的
  8 个 jobs 与 CodeQL `29510215449` 的 Go、JavaScript/TypeScript、Actions 3 个 jobs 全绿。
- [x] CI、CodeQL 支撑步骤、candidate、Upstream Watch 与遗留 deploy workflow 的官方
  JavaScript actions 已统一迁移到 Node.js 24 majors；public-readiness 合同拒绝旧 major、
  未知 ref 和未经审计的 commit pin，最新远端日志未再出现 Node.js 20 弃用告警。

第一批机制取自 Codex 的同盘 staging/失败恢复与本地/远端版本分离、Claude Code 的
显式安装同意和固定 revision 优先；Reasonix 保持工程底座，Reames Lite 只保留轻量
发现/启停体验而不沿用其弱供应链模型。详细证据和边界见
`audits/2026-07-14-m5-plugin-lifecycle-trust.md` 和
`audits/2026-07-15-m5-plugin-process-isolation.md`、
 `audits/2026-07-16-m5-tuf-plugin-registry.md` 和
  `audits/2026-07-16-m5-registry-operations-audit.md`；2026-07-17 的 Reasonix 再同步、凭据与
  schema 安全采用及 identity/worktree 缺口见
  `audits/2026-07-17-reasonix-upstream-sync.md`；其中 P0 已由
  `audits/2026-07-17-m5-mcp-identity-trust.md` 关闭。

## P2：离线 Guard 与 Safe Mode（跨 M3/M6 的恢复加固）

- [x] 建立不装载 Provider、MCP、插件、Hook 或普通 Agent runtime 的 credential-free Guard；
  CLI 在 config/i18n/boot 前提前分派，Windows/macOS/Linux Desktop 安装入口默认经过 Guard。
- [x] 建立带 PID ownership 的 `starting/ready/healthy/clean-exit/failed` 启动账本、五分钟三次
  crash-loop 阈值和 30 秒健康观察期；shutdown 不会在 DOM ready 前错误认证健康。
- [x] 将 updater 扩展为完整安装单元 transaction：Desktop、Guard、launcher/helper 与原先缺失文件
  一并备份；自动回滚必须匹配 startup version、pending `toVersion`、transaction identity、同安装目录
  和所有 SHA-256。staging/swap 补偿不能证明版本一致时报告 mixed install 并 fail closed。
- [x] Safe Mode 不读取或迁移用户/项目 TOML、dotenv，不恢复旧 tab/session，不装配 Provider、
  Controller 或普通 Agent loop，并禁用 Skill、Hook、MCP、插件、Bot、LSP、状态栏命令、更新检查、
  planner、Guardian、subagent、Memory Compiler、heartbeat、本地 pending 诊断归档与 recovery GC；
  普通模式合同不变。
- [x] 配置恢复提供五份 SHA-256 健康快照、损坏 bytes 隔离、显式 restore/undo；插件全量禁用、
  Desktop 派生 tabs/projects/window/zoom rebuild 都是显式 Guard 操作。
- [x] `control.Controller.RecoveryStatus()`、Serve `/api/recovery`、Desktop `GetRecoveryStatus()`、
  Guard check 与 `gateway recovery-status` 共用同一 `repair.Report`。`gateway run` 在加载 config、
  Provider、plugin 和 channel 前执行同一 preflight，systemd/launchd/Scheduled Task 不另建状态机。
- [x] 吸收 Reasonix `dae65e25` 的直接可靠性修复：stalled error-body read deadline、verified snapshot
  no-op、损坏 event-log tail salvage 和 Desktop last-click-wins navigation epoch；明确不盲目跟随
  `c966d027` 删除 Reames Memory Compiler。
- [x] 补充跨进程 pending commit/rollback 竞争、Windows installer failure marker/relaunch 注入、真实
  Guard/假 Desktop 进程、打包、Safe Mode、Frontend queue、Gateway preflight 与 shell syntax 测试。

仓库内机制和本地证据见 `audits/2026-07-17-p2-offline-guard-safe-mode.md`；三平台真实签名安装包的
升级失败、crash-loop 自动回滚、Safe Mode UI 与系统重启演练仍需 candidate/外部证据，不能由单元测试冒充。

## P3：Desktop Recovery Center 与发布资格闭环

- [x] 在普通 Desktop 与 recovery-only Safe Mode 中按需加载同一个 Recovery Center，不装配第二套
  Controller、Provider 或 Agent runtime；Safe Mode 只显示恢复壳，不能进入会话、设置或工作区面板。
- [x] 建立 `repair.ActionRequest/ActionResult` 和 `repair.ExecuteAction`，由 Controller/Desktop 只转发
  同一受控动作：全局/项目配置修复、快照恢复、精确 undo、已验证 pending update 回滚、派生状态
  quarantine/rebuild、插件全量禁用。身份相关动作必须携带上一份报告中的 transaction identity，
  stale UI 不能修改更新后的状态。
- [x] Desktop 状态和动作结果在 Go/Wails 边界统一做路径、home/workspace、安装目录和密钥脱敏；前端
  使用请求序号保证刷新/并发操作最后操作优先，确认、执行、错误、撤销与成功反馈留在延迟 chunk。
- [x] 补三语文案、键盘/语义、bridge mock、Go/Frontend 合同和并发测试，并保持首启 JS/CSS 预算；
  Recovery Center 不进入主入口 bundle。
- [x] 新增三平台安装态 recovery smoke 并纳入 Desktop candidate：强制启动 recovery-only Safe Mode，
  保持损坏 `config.toml` 与合成 `.env` 字节不变，执行配置 repair → exact undo，隔离
  tabs/projects/window/zoom，最后要求 Guard 无 error finding 且默认用户状态边界零变化。
- [x] 最新本地 Windows Wails Desktop + Guard 的真实 recovery smoke 通过；脚本单测、发布合同和
  Windows 本地打包验证通过。
- [x] 修复后的集中提交 `89a8b2b` 已通过普通 CI `29610566790` 8/8、CodeQL `29610566725` 3/3
  和 Desktop candidate `29610593446` 的 Linux/macOS/Windows 全部 jobs；Recovery DTO 空数组保持数组，
  三平台 recovery smoke 已不再被 Windows `null` 回归阻断。
- [ ] 真实签名/notarization 安装包、公开 release 的实际升级失败、任意断电点回滚保持
  `external-blocked`，不能用未签名 candidate 或 fault injection 替代。

## P4：Reasonix 代际差距与运行模式闭环

当前增量追踪会对 `reviewed SHA -> main-v2 HEAD` 执行代码级 diff，但这不足以证明从早期 Reasonix
基座演进而来的 Reames 已系统吸收官方当前代际。P4 先建立完整三方基线，再交付确认缺失的核心机制：

1. **全量源码与产品基线**：定位 Reames 与 Reasonix 的共同/导入基线，比较“早期基座、官方当前
   `main-v2`/稳定 tag、Reames HEAD”；同时清点官方活跃 release/feature/fix 分支，防止只追踪默认
   分支而漏掉已发布 Desktop 机制。报告必须覆盖 Agent loop、Controller、Provider、工具、权限、
   Session/Compaction、Desktop/CLI/ACP、性能、测试、发布与全部 bug-fix 提交，不以 README 或 release
   note 代替源码判断。
2. **模式体系闭环**：审查并按 Reames 单一 Controller 重构 Reasonix 的两个正交轴——普通/计划/目标
   协作方式，以及 economy/balanced/delivery 工作模式。Reames 已有 Plan/Goal、tab-local token economy
   和原子 Controller rebuild 基础，但不得把这些近似机制直接声明为等价；补齐显式稳定 DTO、CLI、
   Desktop、ACP 与需要的远程入口、会话持久化、活动 work 拒绝切换、cache/schema 稳定性和 E2E。
3. **Bug-fix parity map 与吸收批次**：每个官方修复标记为已等价、需移植、需按 Reames 架构重写、
   不适用或明确拒绝；优先落地数据丢失、安全、重复调用、stale state、崩溃、性能与可访问性修复，
   同步回归测试和用户可见文档。只有矩阵有源码/测试证据时才能更新 reviewed 结论。

P4 不采用整分支 merge，也不为了“版本号一致”恢复 Reasonix 品牌、生产 endpoint 或第二套 runtime。
Theme Pack 是已知候选，但必须在完整代际审计后重新排序。

- [x] 永久删除 Desktop 匿名启动、聚合 metrics 与 crash/performance HTTP 上传代码、配置和设置；
  crash/performance/bot 诊断改为显式本地保存或复制，panic/卡顿 pending 证据在下次启动归档到本机。
  本地 session token、成本、cache、read-file 与 Memory v5 计数不受影响。
- [x] 建立可重复的完整代际账本：`scripts/audit_reasonix_generation.py` 枚举 `07c65c2..3637d0f0`
  的 672 个提交、498 个非 merge 提交、314 个精确 fix/perf 提交和 993 个变化文件，并记录每个提交的
  SHA、subject、路径与审计区域；活跃 tag/ref 同时进入证据。完整判断见
  `audits/2026-07-18-reasonix-generation-parity.md`。
- [x] 关闭 economy/balanced/delivery 工作模式跨入口合同：Desktop 三态 UI 与 tab/session 持久化；
  `reames-agent`、`run`、`serve`、`acp` 的 `--profile`；TUI `/work-mode`；ACP `work_mode` config option；Serve rebuild 保持 profile；
  Bot 明确默认 balanced。Delivery 与 balanced 工具 schema 相同，只增加稳定 system contract，并复用
  Todo、`complete_step`、project checks、checkpoint、evidence、权限和沙箱。
- [x] 完成 15 个 required review areas 的源码/测试证据并把 `reasonix-current.json` 更新为完整覆盖；
  `d3cfa5c2` reasoning-only stop 已吸收，Theme Pack 进入 P5，Memory v5 删除、trust 简化和生产 release
  workflow 均记录明确产品/安全分歧，不用“版本一致”替代判断。

## P5：受控 Theme Pack

Reasonix `7f00d2c2` 的 Theme Pack V2 提供了值得吸收的安全和体验机制，但其品牌资产、图片、发布
体系、marketplace 与整体 1.36 万行实现不属于 Reames。P5 按三层推进：

- [x] **安全主题契约与原子存储**：不可执行、版本化 manifest；semantic token allowlist；ZIP bomb、
  path traversal、symlink、Windows 名称、文件数量/尺寸/像素限制；内容寻址资产；导入/替换/删除的原子
  事务与故障注入。替换 active 用户包同时更新 pack 与 state digest，中断矩阵按发布点回滚或前滚。
- [x] **延迟加载 Appearance/Gallery**：`select != apply`；实时预览可撤销且 crash/relaunch 回到已提交主题；
  Safe Mode 不读取 Store、不提供主题资产并强制 Graphite；启动只恢复 active pack，不枚举 Gallery；
  对比度、字体和 bundle budget 继续受硬合同保护。
- [x] **Reames 原创官方资产与 provenance**：`reames-dawn`、`reames-workshop` 与用户 pack 分区，内嵌
  JPEG 的提示词、生成记录、尺寸、digest 和 MIT license 可检查；官方 ID 不可导入覆盖、替换或删除，
  不连接 Reasonix marketplace 或继承发布 endpoint。
- [x] **用户与维护者文档**：公开 JSON Schema 与 Go allowlist 有回归测试；`THEME_PACKS.md` 说明导入、
  创建、限额、恢复和 Safe Mode；设计审计记录跨进程并发边界。当前只有受单实例保护的 Desktop 写 Store，
  若未来向 CLI/Serve 开放写入口，必须先增加跨进程锁。

P5 已关闭：`b4815ba9` 交付受控 Theme Pack，`7396faf4` 修复 installed recovery smoke 的 detached
进程树竞态；最终 CI `29635818559` 8/8、CodeQL `29635818555` 3/3、Desktop candidate
`29635823162` 的 Linux/macOS/Windows 全绿。主题功能不能替代上游核心 bug-fix parity。真实签名、
notarization、公开主题 registry
或 marketplace 不属于 P5 依赖并保持 `external-blocked`。

## P6：上游最新版代码级冻结与 CLI 增量收口

- [x] 11 个上游/参考仓库均执行 `fetch --prune --tags` 与 `pull --ff-only`，干净镜像和精确 SHA 固定在
  `docs/upstreams/upstreams.lock.json`；接受过程逐项列出项目，没有使用 `--accept-all`。
- [x] 完成 Reasonix `3637d0f0..40ef98de` 5 个提交/49 文件的逐提交、逐文件和 required-area 审查，
  同步生成可机器校验的 generation 账本并把 `reasonix-current.json` 推进到 `40ef98de`。
- [x] 适配 CLI 鼠标接管下的右键文本粘贴、活动 transcript selection 复制优先、SSH 剪贴板边界、统一
  `tea.PasteMsg` 路径、assistant identity/gutter 和 reasoning/answer/usage 语义间距；live/resume 使用同一投影。
- [x] 吸收 Hermes 暴露的 Windows UTF-8 BOM 兼容信号：`cron.json` 可读取 BOM，后续保存自动愈合为无 BOM。
- [x] 对 Hermes、Codex、MiMo、Scream Code、Claude Code 的新增源码完成采用/已有等价/延后/拒绝分类；
  Hermes 最终 `4c96172d` 的 CDP 双栈/端口占用修复因 Reames 无 browser-connect runtime 而明确不适用。
- [x] Reasonix、Hermes、Codex、MiMo、Scream Code、AgentArk、Kimi Code、Grok Build 开启路径级
  `diff=true`，以后只审新 lock → latest，不重复打开本冻结点以前的提交。

## P7：Reasonix Fleet 增量与 Gateway watchdog 收口

- [x] 对 Reasonix `40ef98de..2335d0df` 的区域字体和 profile-aware parallel Fleet 完成逐提交、逐文件
  审查并生成机器账本；不以共享工作区 `write_paths` 锁替代 Reames writer worktree、delivery transaction、
  durable effects 和整树预算，named profile/字体偏好只保留为显式 UX 候选。
- [x] 对 Codex `56395bdd..b8b61bc6` 的压缩 rollout inventory 完成战略代码审查；Reames 当前无压缩
  session/SQLite thread inventory，因此不引入 zstd，但固定 logical-path/plain-sibling/corruption/temp-file 回归信号。
- [x] 对 Hermes `4c96172d..581e92e4` 的 38 个提交完成代码级分类；采用与 Reames M6 同构的纯 Go
  systemd notification、watchdog 和 bounded shutdown，不复制 Python/Electron/billing runtime。
- [x] Linux `gateway install` 增加 opt-in `--watchdog-sec`；启用后使用 `Type=notify`、`NotifyAccess=main`
  和 `WatchdogSec=`，默认关闭时继续使用 `Type=simple`。非 Linux、非 install、负值和小于 2 秒均 fail closed。
- [x] `gateway run` 只在 recovery preflight 和至少一个 adapter 启动后发送 `READY=1`；仅在至少一个
  adapter 为 running/degraded 时发送 `WATCHDOG=1`，全部 closed/error 后停止心跳；退出发送
  `STOPPING=1` 并以 30 秒上限停止 Gateway。
- [x] 聚焦测试覆盖 filesystem/abstract Unix datagram、`WATCHDOG_PID`、通知报文、unit 渲染、完整
  `READY → WATCHDOG/unhealthy → STOPPING` 生命周期和有界停止。真实 systemd kill/restart、logout/reboot
  与远端渠道 liveness 仍需干净节点和真实 IM，保持 external-blocked。
- [x] Hermes Electron reload/resize zoom、MiMo 技能重命名、Scream `6474e33a..53fa61b2` 的 Goal/WolfPack/
  Provider/多代理帮助与 Kimi Web 前台默认完成
  采用/已有等价/延后/不适用分类；
  详细边界见 `audits/2026-07-19-p7-upstream-gateway-watchdog.md`。

## M6：远程与多渠道

- 服务器 CLI/TUI：单二进制安装、SSH/tmux 交互、`run` 一次性任务、服务器用户级 `REAMES_AGENT_HOME` 与真实 API key。
- Gateway service：独立后台进程承载飞书/微信/QQ/Telegram 等社交通道；已补 Linux systemd、Windows Scheduled Task、macOS launchd 生命周期、`REAMES_AGENT_HOME` service 绑定、`gateway run --home`、headless smoke，以及 `gateway setup` 的四渠道 secret-env-only、fail-closed access、redacted dry-run、原子幂等配置事务。credential-free 预检现用同一实际 CLI 二进制和隔离 home 覆盖 setup → doctor → service-plan、localhost Provider 一次性任务/会话持久化、feedback submit → summary → draft 和敏感值脱敏；新增 `gateway recovery-status`，且 `gateway run` 在普通 runtime 前强制执行共享 Guard preflight。Linux systemd 安装可 opt-in `--watchdog-sec`：preflight 和至少一个 adapter 启动后才 `READY=1`，adapter 状态健康时喂 `WATCHDOG=1`，全部关闭/错误后停止心跳，SIGINT/SIGTERM 发送 `STOPPING=1` 并做 30 秒 bounded shutdown。WSL2 的真实 systemd user manager又覆盖 unit 静态验证、带空格路径、install、同名重装立即生效、status、restart、stop/start、journal、webhook readiness、卸载后 `LoadState=not-found`，并修复 uninstall 顺序与 service unit 编码。Linux user-scope install 现进一步以旧 unit bytes/mode 和 enabled/active 快照做自动事务，`systemd-analyze --user verify`、前向命令、取消和回滚失败均有 fail-closed 故障注入；该保证不外推到 system scope、macOS launchd 或 Windows Scheduled Task（见 `audits/2026-07-10-headless-gateway-smoke.md`、`audits/2026-07-13-m6-gateway-setup.md`、`audits/2026-07-13-m6-clean-node-operations-preflight.md`、`audits/2026-07-13-m6-linux-systemd-lifecycle.md`、`audits/2026-07-14-m6-recovery-transactions.md`、`audits/2026-07-17-p2-offline-guard-safe-mode.md`、`audits/2026-07-19-p7-upstream-gateway-watchdog.md`）。该 WSL 用户 `Linger=no`，下一步仍需在干净 Linux 云节点验证 logout/reboot 常驻、真实 watchdog kill/restart 和新恢复命令实启，再做真实 Provider 与 IM 渠道回环。
- Server/Web：作为可选远程控制面，提供鉴权、CSRF/Origin、租约、SSE/WS 重连、速率限制和审计。
- 部署：Docker、systemd、反向代理和健康检查已有基线；敏感的 home/state `backup create/verify/restore`、仅新目标恢复、候选/安装后健康检查、保留 immediate predecessor 的 updater 与 `upgrade --rollback` 已有本地自动证据。内嵌 manifest 只证明自洽，多根恢复没有 durable crash journal，公开签名 release 和干净云节点实启仍需外部证据。
- 反馈中心：已建立 `internal/feedback` schema、本地 JSONL 账本、`serve` 的 `POST /api/feedback`、`GET /api/feedback/summary` 与 `POST /api/feedback/draft`，以及 SSH 运维可用的 `reames-agent feedback submit|summary|draft --home PATH`，先完成脱敏、去重、本地聚合和维护草稿，再接人工确认后的 Issue 发布。
- Gateway：统一消息 envelope；渠道 metadata 不进入 provider prompt。按 Hermes `862b1b37..7a43ab04`
  的 Discord 断线恢复信号补 durable per-channel cursor、原消息身份、claim/去重账本、全局扫描上限和
  “最终投递成功后才推进 cursor”合同；账本损坏或写失败必须 fail closed，不能把重新连上等同于消息已交付。
- 每个渠道先完成文本 + 审批 + 取消 + 断线补偿/恢复，再扩展媒体与富交互。
- 阿里云等自有服务器形态按 [云端 Agent 计划](CLOUD_AGENT_PLAN.md) 推进，先完成 SSH/CLI 与独立 Gateway service，再按需开启 `serve`，最后承载后台研究任务。

## P8：官方 GPT / Claude Provider parity

- [x] OpenAI Codex 与 Claude Code 提升为二级战略代码上游；从 Codex Responses 源码和 Claude Code
  可公开代码/协议信号建立 capability matrix，不把 release note 本身当完成证据。
- [x] OpenAI 增加显式 `api_mode = "responses"` 的独立 transport；覆盖 instructions/input item、GPT
  reasoning summary、文本、单/并行工具、图像、usage/cache/reasoning tokens、typed failed/incomplete、
  cancel、无输出 reconnect、已输出 interruption 和 clean EOF fail-closed；保留向后兼容 include 并持久化
  opaque `reasoning.encrypted_content`（当前 `store=false` API 默认也会返回），在工具续轮回放且不进入展示/导出。历史空值继续使用
  `chat_completions`，不根据模型名或 URL 猜协议。
- [x] Anthropic Messages 覆盖 thinking、独立 effort、tool use/result、vision、prompt cache、usage 合并、
  typed SSE error、`message_stop`/未闭合 block 中断门禁；signed thinking 与 opaque
  `redacted_thinking` 按原始顺序持久化和回放，后者不进入展示 DTO。
- [x] Anthropic 第一方预设按模型限制协议能力：Sonnet/Opus 使用 adaptive thinking，仅 Opus 暴露
  xhigh/max；Haiku 4.5 通过可持久化的模型级 `thinking=""` + `reasoning_protocol="none"` override
  省略不兼容 wire，并经 TOML/Desktop round-trip 验证。
- [x] CLI/Desktop/Serve/Gateway 继续只通过现有 Controller/boot 装配；Desktop/TOML 保留显式
  `api_mode`，OpenAI/Anthropic 第一方预设可编辑，未建立 GPT/Claude 专用 Agent loop，也未改变稳定
  system prompt/tool schema。
- [x] 审查 Hermes `bf391030..862b1b37` 的空响应误分类修复；Reames 共享 Provider classifier 已保证
  empty-response advisory 即使提到 `max_tokens` 也返回 `ShouldCompact=false`，真实 context-window 溢出
  仍返回压缩建议；当前 Agent 仍按 usage 阈值压缩，未把该合同冒充运行时恢复证据。
- [x] P8 仓库内门槛已通过 Root/Desktop/Frontend 全量、Provider/Agent/plugin/control race、六目标 CLI +
  六目标 Guard、clean clone 与治理合同并关闭；最终公开交付仍以该 push 对应 CI/CodeQL 为准。真实
  OpenAI/Anthropic API 回环继续单独标记 `external-blocked`，不以 localhost fixture 冒充。

## P9：Codex-class extensibility 与 headless 协议

- 以 Codex 最新源码做能力矩阵，审计 Reames 现有 MCP、Plugin Package、Skill、Hook、TUF registry、
  package sandbox 与 M5 lifecycle，逐项标记已有/缺口/不适用，而不是因“已有插件目录”直接宣称 parity。
- 补齐用户需要的发现、安装、精确权限、更新/回滚、禁用/撤销、诊断、版本兼容和会话投影；继续复用
  fresh-human、generation identity、TUF/provenance、OS sandbox 和进程树回收，不接入无治理 marketplace。
- 审计 Codex App-Server/headless 线程、命令、事件、审批和 MCP runtime 语义，只扩展现有
  `internal/control`/event wire；不引入第二套 Agent/runtime 或破坏传输无关边界。
- 将 Codex `ultra` 的自动任务委派作为 Agent/runtime 能力单独验收；P8 只支持已证明的 Responses wire
  effort，不能把 `ultra -> max` 的兼容别名写成自动委派 parity。
- OpenAI 官方预设已按 2026-07-19 公共 API 文档开放 `gpt-5.6-sol`、`gpt-5.6-terra`、
  `gpt-5.6-luna` 与 `gpt-5.4` 的普通 function-tool Responses；Codex catalog 的 `code_mode_only` 是产品
  runtime 选择，不能反推公开 API 不支持 function calling，也不能据此宣称 Reames 已有 Codex 产品 parity。
- P9 继续代码级跟进并实现 Codex freeform/code-mode、Responses Lite/WebSocket、hosted tool item、
  programmatic tool calling、显式 prompt caching/写入计费、`reasoning.context`、pro mode、Responses
  multi-agent 与 App-Server/headless 投影；每项必须有 wire fixture、恢复/权限/沙箱/evidence 合同。
- Codex `35eaf3ff..312caf17` 已增加 Realtime V3 `initial_items`：最多 128 个完整 role-bearing 文本项，
  总计和单项均受 8192 估算 token 门槛约束，并只在 Frameless Bidi V3 启用。Reames 将其作为 P9
  Realtime/App-Server 会话播种合同，不回填到 P8 HTTP Responses；Hermes 的 `/model --once` 作为单轮
  root 模型覆盖候选，必须证明不污染持久默认模型、并发会话或后续 turn 后才可采用。

## P10：第一方 Browser Control / CDP

- 建立 CLI/Desktop/Serve 共用的浏览器控制 runtime，支持受管 Chromium 与显式 attach；CDP 连接必须验证
  双栈 loopback、目标协议和端口占用者，不能把普通 TCP listener 当作浏览器就绪。
- 首个纵向闭环覆盖 browser/tab 生命周期、navigate、DOM/query、click/type、screenshot、download、取消和
  session 恢复；后续再扩展复杂 Playwright 语义。
- 输入动作采用“后台投递 → 结果验证 → 仅在结构化 `suspected_noop`/`background_unavailable` 信号后建议
  前台升级”的阶梯；前台焦点变更使用独立、按会话和投递模式隔离的审批，不继承后台 action 授权。
  浏览器/驱动 transport 退出必须清除 started 状态，下一次调用有界重建，不得在失效 session 上挂死。
- 浏览器网络、cookie/登录态、下载和页面文本全部接入 permission、sandbox、credential isolation、
  prompt-injection 标注、evidence 与 redaction。`web_search`、`web_fetch` 或可选 Playwright MCP 不算 P10 完成。

## M7：通用工作能力

在编程闭环稳定后，按共同基础设施复用程度扩展：

1. 深度研究与可追溯引用。
2. 文档、表格、演示文稿和 PDF 工作流。
3. 浏览器与桌面自动化。
4. 数据整理、分析和可视化。
5. 定时任务与长期监控。
6. 团队协作与外部服务连接器。
7. 上游和参考项目研究 Worker：自动发现、研究、建单、草稿分支或草稿 PR，但不自动合并。
8. 本地诊断与主动反馈：脱敏诊断由用户复制/保存，管理员主动反馈聚合为可审计维护草稿；不建设 Reames 自有遥测服务。

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
P1/P2/P3/P4/P5 已关闭；P5 的 CI、CodeQL 与三平台 Desktop candidate 全绿
→ P6 已关闭；P7 已审查 Reasonix `40ef98de..2335d0df`、Codex 战略增量与 4 个机制参考增量，并收口 Gateway systemd
  READY/WATCHDOG/STOPPING、adapter-health gate 和 bounded shutdown；权威 reviewed SHA 以 lock 为准
→ Grok Build `98c3b24` 已纳入机制参考；后续增量重点比较 shell/permission/sandbox、
durable session/subagent、TUI queue/interject、ACP/headless。不得照搬其 Plan Mode 的 shell/subagent 写入缺口，
也不接入 xAI auth、telemetry、online memory、managed policy、marketplace 或 Rust 第二 runtime
→ 当前 M6 继续等待/准备干净 Linux linger-enabled logout/reboot、真实 watchdog kill/restart、Gateway
  recovery-status/system service 实启，以及真实 Provider 与飞书/微信/QQ 文本、审批、取消、恢复回环
→ 无需外部凭据的下一仓库主线按 P8 → P9 → P10：官方 GPT/Claude Provider parity、Codex-class
  插件/headless 能力矩阵、第一方 CDP Browser Control；每条先完成 fixture/权限/沙箱/evidence 再补真实回环
→ 体验候选：历史消息时间、Windows 外部打开器、Subagent profile、workspace 面板偏好按真实用户缺口进入
→ external-blocked：真实运营 registry 的生产 endpoint、人员/HSM 密钥仪式、online custody、
实际轮换/compromise drill，以及声明 provenance 时的独立 DSSE/SLSA policy verifier
→ 并行等待干净云节点、真实飞书和公开签名 release 外部证据，取得环境后关闭 M6 external-blocked 项
```
