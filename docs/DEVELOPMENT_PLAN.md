# Reames Agent 发展计划

> 状态：当前唯一执行路线
>
> 更新：2026-07-13
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
| M3 桌面日用化 | Desktop 达到稳定、清晰、低噪音的主产品体验 | 核心路径点击测试；设置/审批/变更/恢复完整；体积与启动性能达标 | 进行中 |
| M4 Agent 可靠性 | 长任务能计划、分解、验证、压缩和恢复 | Goal/Plan/子任务/证据/检查点形成统一状态机和失败恢复测试 | 待开始 |
| M5 扩展生态 | Skill、Hook、MCP、插件可发现、安装、授权和诊断 | 包格式稳定；安装/升级/禁用/权限/故障隔离 E2E | 待开始 |
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
- [x] 建立 Windows 原生 Desktop 冷启动硬门槛：本地源码 production smoke 要求 8 秒内达到连续三次响应；托管 runner 的首次安装候选依据 11.531 秒首次响应实测采用 15 秒门槛与 20 秒观察窗，两层不得互相替代。当前源码 production Wails 实测首次可见/响应 1.016 秒、稳定响应 2.016 秒，隔离 HOME 边界无泄漏（见同一审计）。
- [x] 可访问性首批：统一真正模态层的初始焦点、Tab/Shift+Tab 围栏、嵌套顶层判定、退出动画后 opener 恢复和 `aria-modal`/读屏关联；命令面板补 combobox/listbox active-descendant 合同，设置/历史/图片/首次引导/快捷键帮助共用同一生命周期（见 `audits/2026-07-13-m3-modal-focus-accessibility.md`）。
- [x] Windows 显示缩放闭环：连续滑动按最后选择串行合并写入，Go 偏好使用原子替换并拒绝非有限值；设置页区分启动已应用/保存中/待重启，提供立即重启与失败回滚，组件和真实浏览器覆盖 100% → 105% → 100% 状态（见 `audits/2026-07-13-m3-display-zoom-persistence.md`）。
- [x] 主题对比度与焦点纵向合同：六套视觉风格同时覆盖深/浅色、普通/创作模式的小文本、状态色、主按钮与焦点指示器，自动浅色必须与显式浅色一致；补 forced-colors 焦点规则、局部画布焦点环重算和入口重挂载后的语义焦点恢复，并用真实浏览器切换 Graphite/Carbon/Amber 及创作模式核验最终计算值（见 `audits/2026-07-13-m3-theme-contrast.md`）。
- [x] Windows warm relaunch 门槛：native smoke schema v3 在冷启动关闭后复用同一隔离 HOME/WebView2 profile 启动第二个真实进程，独立记录可见/响应/稳定时间、预算、早退和清理；托管安装器 candidate 强制冷启动 15 秒与 warm 6 秒预算，本地源码 production 仍保持冷启动 8 秒，当前源码两轮稳定响应均为 1.516 秒（见 `audits/2026-07-13-m3-windows-warm-startup.md`）。
- [x] Linux/macOS 启动预算：candidate smoke schema v2 要求隔离 HOME 的 Desktop 状态连续三次就绪且不泄漏默认状态，Linux 同时要求最终仍有可见窗口；run `29209723618` 的 Linux 首次状态/窗口就绪为 4.538 秒、稳定就绪 5.567 秒，macOS 首次状态就绪 0.575 秒、稳定就绪 1.872 秒，均通过 10 秒门槛。macOS 证据只声明状态 readiness，不冒充窗口可见性（见 `audits/2026-07-13-m3-linux-macos-startup-readiness.md`）。
- [x] Desktop 重启恢复竞态：后端首次 `ListTabs` 等待 `tabsRestored` 门闩，前端在 restored controller `ready=true` 前不读取 history，避免空 tab 或空 transcript 快照成为整轮最终状态；补两层竞态合同、canonical event log 半行容错与 UIA Send fallback，当前 production Wails 已完成 19 请求、五类失败恢复、停止和原会话/工作区/消息重启恢复的原生闭环。hosted candidate 复核仍单独记录（见 `audits/2026-07-13-m3-desktop-restart-restore-race.md`）。
- [ ] 性能后续：继续评估 locale、主工作流与 CSS 拆分，不为数字牺牲首屏可用性。

UI 改动必须同时提供组件测试和一次真实浏览器或 Wails 点击验证。

## M4：Agent 可靠性

- 统一 Goal、Plan、Task、证据账本和 Checkpoint 的状态关系。
- 对循环、无进展、预算耗尽和上下文压缩建立确定性策略。
- 子任务必须有输入边界、并发预算、取消传播和结果归并。
- 记忆检索必须可解释、可关闭、可删除，并避免把动态状态写入稳定前缀。
- “完成”需要证据，不只依赖模型自报。

## M5：扩展生态

- 固化 plugin package manifest、版本和权限声明。
- 提供安装来源信任、校验、升级预览、回滚和禁用。
- 统一 Skill、Hook 与 MCP 的发现、诊断和桌面管理体验。
- 插件故障不得拖垮主 Agent；敏感能力默认最小权限。
- 参考 Claude Code/Codex 的生态机制，但保持 Reames 自己的稳定契约。

## M6：远程与多渠道

- 服务器 CLI/TUI：单二进制安装、SSH/tmux 交互、`run` 一次性任务、服务器用户级 `REAMES_AGENT_HOME` 与真实 API key。
- Gateway service：独立后台进程承载飞书/微信/QQ/Telegram 等社交通道；已补 Linux systemd、Windows Scheduled Task、macOS launchd 的生命周期命令骨架、`REAMES_AGENT_HOME` service 绑定、前台 `gateway run --home` 调试入口、防 `serve`/`bot start` 回归的生命周期契约，以及隔离 home 下真实 CLI 的 headless Gateway smoke（见 `audits/2026-07-09-gateway-home-binding.md`、`audits/2026-07-09-gateway-service-lifecycle.md`、`audits/2026-07-10-headless-gateway-smoke.md`），下一步做干净机器实战验证、真实 IM 渠道回环和 setup 向导。
- Server/Web：作为可选远程控制面，提供鉴权、CSRF/Origin、租约、SSE/WS 重连、速率限制和审计。
- 部署：Docker、systemd、反向代理、健康检查、备份和升级回滚。
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
推进 M3 原生 Desktop 日用化、可访问性与性能门槛
→ 在干净云节点完成 CLI + Gateway + feedback 运维闭环
→ 使用真实飞书凭据完成文本/审批/取消/恢复回环
```
