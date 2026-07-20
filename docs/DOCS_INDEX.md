# Reames Agent 文档索引

文档按“当前事实优先”组织。过时方案直接删除，由 Git 历史承担归档职责。

## 新接手者

按顺序阅读：

0. [下一位模型接手交接文档](NEXT_MODEL_HANDOFF.zh-CN.md)：当前会话背景、最近提交、工作边界、验证命令和下一步建议。
1. [项目说明](PROJECT.md)：产品定位、边界、原则和当前事实。
2. [发展计划](DEVELOPMENT_PLAN.md)：唯一执行路线、里程碑和验收标准。
3. [架构](ARCHITECTURE.md)：代码结构和运行边界。
4. [贡献指南](../CONTRIBUTING.md) 与 [AGENTS.md](../AGENTS.md)：开发和验证规则。

## 用户文档

| 文档 | 用途 |
|---|---|
| [中文使用指南](GUIDE.zh-CN.md) / [English guide](GUIDE.md) | CLI、配置、会话和功能使用 |
| [中文机器人指南](BOT_GUIDE.zh-CN.md) / [English bot guide](BOT_GUIDE.md) | IM Gateway 配置与使用 |
| [部署指南](DEPLOY.md) | 服务器 CLI、独立 Gateway service、敏感备份/恢复、二进制回滚、Docker、systemd 与 nginx |
| [恢复指南（中文）](RECOVERY.zh-CN.md) / [English](RECOVERY.md) | Offline Guard、crash-loop、Safe Mode、更新回滚和运维操作手册 |
| [受控主题包](THEME_PACKS.md) | Desktop 主题导入、创建、限额、原子恢复、Safe Mode 与官方资产 provenance |
| [云端 Agent 计划](CLOUD_AGENT_PLAN.md) | 可选服务器 CLI、社交通道 Gateway、上游研究 Worker与本地反馈工作流 |
| [配置路径（中文）](CONFIG_PATHS.zh-CN.md) / [English](CONFIG_PATHS.md) | 配置、状态、缓存和迁移路径 |
| [协作方式与工作模式](COLLABORATION_MODES.zh-CN.md) | 普通/计划/目标与经济/均衡/交付两个正交轴 |
| [工具审批模式](TOOL_APPROVAL_MODES.zh-CN.md) | 询问、自动和 Yolo 权限语义 |
| [桌面 Hooks](DESKTOP_HOOKS.zh-CN.md) | Hook 配置、事件和 payload |
| [插件包（中文）](PLUGIN_PACKAGES.zh-CN.md) / [English](PLUGIN_PACKAGES.md) | 插件包安装与管理 |
| [插件 Registry 运维（中文）](PLUGIN_REGISTRY_OPERATIONS.zh-CN.md) / [English](PLUGIN_REGISTRY_OPERATIONS.md) | TUF 仓库结构、角色密钥、发布顺序、轮换与泄露恢复合同 |

## 工程契约

| 文档 | 用途 |
|---|---|
| [工程规格](SPEC.md) | runtime 数据结构、注册表和工程约束 |
| [工具契约（中文）](TOOL_CONTRACT.zh-CN.md) / [English](TOOL_CONTRACT.md) | 内置工具 provider-visible schema |
| [任务契约（中文）](TASK_CONTRACT.zh-CN.md) / [English](TASK_CONTRACT.md) | 长任务输入、暂停和完成语义 |
| [检查点](CHECKPOINTS.md) | 文件快照与回退设计 |
| [会话记忆检索](SESSION_MEMORY_RETRIEVAL.md) | 历史和记忆检索实现 |
| [推理 Provider](REASONING_PROVIDERS.md) | 不同 Provider 的推理参数 |
| [思考语言（中文）](REASONING_LANGUAGE.zh-CN.md) / [English](REASONING_LANGUAGE.md) | 推理语言设置 |
| [Goal 强制执行](GOAL_ENFORCEMENT.zh-CN.md) | Goal 模式预算和执行约束 |

## 项目治理

| 文档 | 用途 |
|---|---|
| [参考项目治理](REFERENCE_GOVERNANCE.md) | 主上游、参考项目、许可证和吸收规则 |
| [Upstream Watch](upstreams/README.md) | 自动发现、分类、Issue 生命周期和人工接受版本 |
| [公开仓库前检查清单](PUBLIC_READINESS.md) | 公开前文档、所有权、发布与部署门禁 |
| [发布流程](RELEASING.md) | 分支、canary 和稳定发布 |
| [安全策略](../SECURITY.md) | 信任边界与漏洞报告 |
| [威胁模型](THREAT_MODEL.md) | 当前安全控制、已知缺口与外部阻塞项 |

## 实现记录

以下文档描述已实现机制或保留审计证据，不决定当前优先级：

| 文档/目录 | 用途 |
|---|---|
| `docs/superpowers/specs/` | AutoResearch 已实现设计 |
| `docs/superpowers/audits/` | AutoResearch 验证记录 |
| [接管审计](audits/2026-07-09-takeover.md) | 初次代码与测试审计 |
| [首次上游审查](audits/2026-07-09-upstream-review.md) | Upstream Watch 建立时的差异快照 |
| [Desktop candidate governance audit](audits/2026-07-09-desktop-candidate-governance.md) | 三平台 Desktop candidate workflow、artifact smoke 与发布边界证据 |
| [Legacy tree quarantine audit](audits/2026-07-09-legacy-tree-quarantine.md) | 旧 Python/Hermes 树的短期隔离规则和后续迁移队列 |
| [参考项目功能差异与吸收清单](audits/2026-07-09-reference-feature-gap-map.md) | 2026-07-09 的跨参考项目证据化差异快照；当前优先级仍以发展计划为准 |
| [仓库 legacy tree 清洁收口审计](audits/2026-07-17-repository-cleanup.md) | 删除旧 Hermes/Python/Electron/TUI/worker 树、保留边界与 public-readiness 防回归证据 |
| [Script surface cleanup audit](audits/2026-07-09-script-surface-cleanup.md) | 公开脚本入口清理、旧运行时脚本删除和防回归门禁 |
| [Telemetry and feedback boundary audit](audits/2026-07-09-telemetry-feedback-boundary.md) | 历史默认关闭基线；P4 已永久删除 Desktop 上传并保留本地诊断/反馈边界 |
| [Self-hosted feedback collector audit](audits/2026-07-10-feedback-collector.md) | `internal/feedback`、`serve` 本地反馈收集、脱敏、去重和 JSONL 账本证据 |
| [真实 Provider 最小验证](audits/2026-07-09-real-provider.md) | DeepSeek 鉴权、响应、用量与缓存命中证据 |
| [Desktop M1 桥接审计](audits/2026-07-09-desktop-m1-bridge.md) | Desktop Submit/Cancel 多工作区桥接自动化证据 |
| [Desktop M1 frontend workspace loop audit](audits/2026-07-09-desktop-m1-frontend-workspace-loop.md) | 前端 workspace 选择、发送、切换和停止的 tab 绑定回归证据 |
| [M1 文件写入闭环审计](audits/2026-07-09-m1-file-write-loop.md) | 真实 `write_file` 审批、补丁预览、落盘与 RewindCode 回退证据 |
| [M1 重连与恢复审计](audits/2026-07-09-m1-reconnect-recovery.md) | pending approval replay、Desktop tab 事件恢复和 pending snapshot diff 诊断证据 |
| [M1 失败场景合同审计](audits/2026-07-09-m1-failure-contracts.md) | provider 鉴权失败、审批超时阻塞写入和运行态复位证据 |
| [Desktop M1 frontend failure display audit](audits/2026-07-09-desktop-m1-failure-display.md) | 前端 provider 失败、工具超时和审批拒绝的可见 warn/error 与停止态复位证据 |
| [Windows native Desktop smoke attempt](audits/2026-07-10-windows-native-smoke-attempt.md) | Windows Wails 启动响应与 frameless 截图通道的初始阻断证据 |
| [Windows native Desktop interaction smoke](audits/2026-07-10-windows-native-interaction-smoke.md) | 截图无关 UIA 的新建、工作区、发送、停止、事件账本和重启恢复证据 |
| [Windows 原生 Desktop 失败恢复审计](audits/2026-07-11-windows-native-failure-recovery.md) | 401、429、断流、权限拒绝和工具超时的原生可见提示、运行态复位与后续成功 turn 证据 |
| [M2 结构化错误与会话恢复控制面审计](audits/2026-07-11-m2-error-session-control.md) | `ErrorInfo` 前端 category 动作、原生 UIA 点击与 CLI 会话 DTO/依赖棘轮收缩证据 |
| [M2 版本化命令控制面审计](audits/2026-07-11-m2-command-control.md) | submit/cancel/approval/status 的稳定 DTO、五入口迁移、HTTP/WS 兼容与真实握手安全回归 |
| [M2 事件、转录与 metadata 边界审计](audits/2026-07-11-m2-event-transcript-metadata.md) | `eventwire` v1、展示安全 transcript、Provider/渠道 metadata 隔离与缓存前缀回归 |
| [M2 会话适配器边界审计](audits/2026-07-11-m2-session-adapter-boundary.md) | Bot 会话列表/附着恢复、CLI branch/rename 的 control 迁移与依赖棘轮收缩 |
| [M2 prompt 与设置边界审计](audits/2026-07-11-m2-prompt-settings-boundary.md) | Desktop prompt rebuild、memory suggestions、settings provider view、Serve title 装配与 ACP metadata 安全投影 |
| [M2 Desktop transcript 边界审计](audits/2026-07-11-m2-desktop-transcript-boundary.md) | history/pagination/checkpoint/planner sidecar、安全 replay 与 opaque rebuild snapshot 收口 |
| [M2 CLI/ACP composition 边界审计](audits/2026-07-11-m2-cli-composition-boundary.md) | ACP 死装配删除、session copy control API、MCP 名称合同拆分与依赖棘轮收缩 |
| [M2 Desktop session-store 边界审计](audits/2026-07-11-m2-desktop-session-store-boundary.md) | Desktop app 列表/加载/租约/topic 元数据迁入 control 与 opaque handle |
| [M2 Desktop tabs 会话边界审计](audits/2026-07-12-m2-desktop-tabs-session-boundary.md) | Desktop tabs branch/index/profile/recovery 元数据迁入稳定 control DTO 与原子 mutation |
| [M2 transport 边界收官审计](audits/2026-07-12-m2-transport-boundary-closeout.md) | boot 注册、opaque CLI handoff、termrender、review 装配与 transport allowlist 归零 |
| [M3 Desktop 性能预算审计](audits/2026-07-12-m3-desktop-bundle-budget.md) | 按需界面拆包、真实产物预算、Windows 冷启动硬门槛与交互证据 |
| [M3 模态焦点与可访问性审计](audits/2026-07-13-m3-modal-focus-accessibility.md) | 共享焦点生命周期、Tab 围栏、退出动画恢复、读屏语义与真实浏览器证据 |
| [M3 模态隔离、Transcript 与原生 UIA 可访问性审计](audits/2026-07-13-m3-modal-isolation-transcript-uia.md) | 真正模态背景隔离、嵌套/重开生命周期、最终答复通知语义、严格 InvokePattern 与 Windows production smoke |
| [M3 Windows 显示缩放持久化审计](audits/2026-07-13-m3-display-zoom-persistence.md) | 缩放写入合并、原子持久化、待重启状态、直接重启与浏览器交互证据 |
| [M3 主题对比度与焦点审计](audits/2026-07-13-m3-theme-contrast.md) | 六主题深浅色、普通/创作模式对比度合同、双层焦点环、forced-colors 与重挂载焦点恢复 |
| [M3 Windows warm startup 审计](audits/2026-07-13-m3-windows-warm-startup.md) | 同 HOME/WebView2 profile 原生二次启动、schema v3、6 秒预算、清理与边界证据 |
| [M3 Linux/macOS startup readiness 审计](audits/2026-07-13-m3-linux-macos-startup-readiness.md) | 隔离状态 readiness、Linux 可见窗口、连续稳定采样、10 秒预算与 candidate 证据边界 |
| [M3 Desktop 重启恢复竞态审计](audits/2026-07-13-m3-desktop-restart-restore-race.md) | 首次 tab 快照恢复门闩、canonical event log 并发采样与 Windows UIA 重启恢复证据 |
| [M3 多语言按需加载与预算审计](audits/2026-07-13-m3-lazy-locale-budget.md) | 保存语言优先与 auto 系统语言预取、运行期原子切换、双 locale chunk 合同与本地化首启预算 |
| [M3 主首启图与设置 CSS 拆分审计](audits/2026-07-13-m3-main-graph-css-split.md) | browser mock、VirtualMenu/TanStack、SettingsPanelRoute CSS 延迟加载与递归首次使用图预算 |
| [M3 安装器历史完整性后续审计](audits/2026-07-13-m3-installed-history-completeness.md) | 部分 controller 投影、0 B checkpoint、canonical event log 完整性比较与 production interaction/accessibility 证据边界 |
| [外部 Agent 批次验收](audits/2026-07-11-external-agent-batch-acceptance.md) | 大批提交的保留/撤回范围、真实性修复和全量本地验证 |
| [Control transport boundary ratchet audit](audits/2026-07-10-control-boundary-ratchet.md) | Desktop/CLI/Serve/Bot/ACP 对 runtime 内部直连的精确依赖基线和 CI 收缩棘轮 |
| [Gateway service home binding audit](audits/2026-07-09-gateway-home-binding.md) | 后台 gateway service 与 CLI 共用 `REAMES_AGENT_HOME` 的部署一致性证据 |
| [Gateway service lifecycle contract audit](audits/2026-07-09-gateway-service-lifecycle.md) | 跨平台后台 gateway 生命周期命令、CLI/Gateway 隔离和防 `serve`/`bot start` 回归证据 |
| [Gateway command contract audit](audits/2026-07-10-gateway-command-contract.md) | IM Gateway 平台无关命令解析、`/current` 状态别名和前缀误触发防回归证据 |
| [Installer release artifact mode audit](audits/2026-07-09-installer-release-mode.md) | 安装器默认源码构建、显式 release artifact 模式与 SHA256SUMS 校验证据 |
| [Installer dry-run contract audit](audits/2026-07-10-installer-dry-run-contract.md) | 一键安装器 dry-run 的 Gateway home 绑定、`.env` 凭据来源和 release 校验契约 |
| [Headless CLI、Gateway 与反馈 smoke 审计](audits/2026-07-10-headless-gateway-smoke.md) | 隔离 `REAMES_AGENT_HOME` 下真实 CLI 的 setup/doctor/service plan、localhost Provider 会话与 feedback 本地维护链路 |
| [Setup Gateway preflight audit](audits/2026-07-10-setup-gateway-preflight.md) | `reames-agent setup` 完成后提示 Gateway doctor 和 service dry-run 的交接证据 |
| [M6 Gateway setup 配置闭环审计](audits/2026-07-13-m6-gateway-setup.md) | 四渠道无界面配置、secret-env-only、访问控制、原子幂等更新与部署纵向 fixture 证据 |
| [M6 clean-node 运维预检审计](audits/2026-07-13-m6-clean-node-operations-preflight.md) | schema v2 credential-free 实际二进制预检、全 home 敏感值扫描与真实外部证据非声明 |
| [M6 Linux systemd user lifecycle audit](audits/2026-07-13-m6-linux-systemd-lifecycle.md) | WSL2 真实 user-service 安装、同名重装、restart/stop/start/uninstall、webhook readiness 与升级资产合同证据 |
| [M6 Gateway、备份与升级恢复事务审计](audits/2026-07-14-m6-recovery-transactions.md) | Linux user install fail-closed 回滚、敏感 home/state backup/restore 与 CLI updater previous/rollback 的本地证据和限制 |
| [M4 会话运行态与 Checkpoint 恢复审计](audits/2026-07-14-m4-session-runtime-recovery.md) | Goal completion 门控、v2 Goal/Plan/Todo 恢复、分支/回退一致性、事务式文件恢复及首批边界 |
| [M4 子代理共享委派预算审计](audits/2026-07-14-m4-delegation-budget.md) | 整棵委派树共享并发、step、token、duration 与 cancellation 预算，覆盖嵌套、后台任务和 compaction |
| [M4 可写子代理 Effects 归并审计](audits/2026-07-14-m4-writable-subagent-effects.md) | child read/write/command receipt、父 checkpoint、mutation boundary 与跨 turn 防污染证据 |
| [M4 跨 continuation 最小证据审计](audits/2026-07-14-m4-durable-evidence.md) | writer epoch、项目检查哈希/tool-call 引用、exact-anchor crash-resume 与失效语义 |
| [M4 Writer 持久化门禁审计](audits/2026-07-14-m4-writer-persistence-gate.md) | checkpoint/runtime/in-flight 写失败阻断、stale marker 拒绝、重试回滚、session handoff 串行与进程中断恢复边界 |
| [M4 后台 Task、Compaction 与记忆统一恢复审计](audits/2026-07-14-m4-task-compaction-memory-recovery.md) | subagent 安全边界持久化、interrupted/continue_from、compacted transcript 续接与稳定前缀记忆检索证据 |
| [M4 Rooted Writer 与 Durable Child Effects 审计](audits/2026-07-14-m4-rooted-writers-child-effects.md) | `os.Root` built-in writer/checkpoint restore、multi-file rollback、child mutation intent、journal cursor 与 crash/replay/branch 边界 |
| [M4 跨资源恢复事务收官审计](audits/2026-07-14-m4-cross-resource-recovery-transaction.md) | visible/synthetic turn commit anchor、Rewind 两阶段 journal、断电重放、原子替换 fail-closed 与 M4 完成边界 |
| [M4 可写子代理 Worktree 隔离与交付审计](audits/2026-07-17-m4-writer-worktree-isolation.md) | workspace lease、独立 branch/worktree、取消/崩溃/删除回收、父会话交付事务、Desktop/Serve 投影与真实 Git 证据 |
| [M5 插件生命周期信任审计](audits/2026-07-14-m5-plugin-lifecycle-trust.md) | schema v1、内容寻址 generation、两阶段生命周期、Desktop 审批/原生交互与旧运行时撤销边界 |
| [M5 插件进程隔离与真实第三方 E2E 审计](audits/2026-07-15-m5-plugin-process-isolation.md) | package-owned Hook/MCP 最小环境、隔离后 child env、严格 OS sandbox、运行中撤销、真实 superpowers 固定 revision Windows E2E 与剩余边界 |
| [M5 TUF 插件 Registry 信任链审计](audits/2026-07-16-m5-tuf-plugin-registry.md) | 带外 root、项目抗覆盖配置、TUF 轮换/回滚/冻结、签名 entry/apply 绑定、生命周期证据和运营边界 |
| [M5 Registry 运维审计与轮换演练](audits/2026-07-16-m5-registry-operations-audit.md) | 只读生产策略审计、2-of-3 角色隔离、连续双阈值 root、完整 target 字节与 external-required 边界 |
| [Reasonix 与参考项目增量同步审计](audits/2026-07-17-reasonix-upstream-sync.md) | Reasonix 525 提交分类、MCP schema/凭据/Provider 采用证据、其他参考仓库增量与下一阶段 P0/P1 方向 |
| [Reasonix 全量代际与 bug-fix parity 审计](audits/2026-07-18-reasonix-generation-parity.md) | 从导入基线到当前 main-v2 的完整提交账本、required-area 证据、工作模式与明确产品分歧 |
| [Reasonix 3637d0f0..40ef98de 增量收口审计](audits/2026-07-18-reasonix-3637d0f-40ef98d.md) | 5 提交逐项决策、CLI 右键粘贴/SSH/assistant hierarchy 采用与发布/站点分歧 |
| [全参考上游最新版冻结审计](audits/2026-07-18-upstream-reference-freeze.md) | 11 仓库冻结 SHA、6 组代码级差距、采用/等价/延后/拒绝和 future diff 跟踪 |
| [Grok Build 参考项目首次纳入审计](audits/2026-07-18-grok-build-reference-intake.md) | 官方仓库、许可证/同步模型、权限/沙箱/session/subagent/TUI/ACP 候选与明确不采用边界 |
| [P5 受控 Theme Pack 设计与交付审计](audits/2026-07-18-p5-controlled-theme-pack-design.md) | manifest/schema、恶意包防护、原子 Store、可撤销预览、Safe Mode、原创官方资产与验证边界 |
| [P7 上游增量与 Gateway systemd watchdog 审计](audits/2026-07-19-p7-upstream-gateway-watchdog.md) | Reasonix Fleet/字体、Hermes 生命周期采用、其他参考增量分类、systemd 通知与 P8/P9/P10 方向 |
| [P8 原生 GPT / Claude Provider parity 审计](audits/2026-07-19-p8-native-gpt-claude-provider-parity.md) | OpenAI Responses encrypted reasoning replay、Anthropic Messages ordered redacted thinking、原生预设、fixture 证据与真实 API external-blocked 边界 |
| [M6 持久渠道恢复审计](audits/2026-07-19-m6-durable-channel-recovery.md) | 跨重启入站 claim/dedupe、连续游标、最终发送门禁、受限补扫适配器、隐私状态投影与真实 IM external-blocked 边界 |
| [M6 Telegram 持久长轮询审计](audits/2026-07-19-m6-telegram-durable-polling.md) | 正式 Telegram Adapter、env-only token、long-poll deadline/retry、最终投递后 offset 门禁与 localhost 故障注入 |
| [M6 最终答复 obligation 审计](audits/2026-07-20-m6-outbound-final-response-obligation.md) | 最终文本发送前持久化、ACK 歧义可见恢复、多分片断点、单 writer、隐私投影与不重跑模型证据 |
| [M6 微信持久轮询与 Desktop 背压审计](audits/2026-07-20-m6-weixin-polling-desktop-backpressure.md) | 微信最终投递后提交 poll cursor、三渠道 envelope 背压与 Desktop 有界 live queue 证据 |
| [M6 Linux Gateway 卸载事务审计](audits/2026-07-20-m6-linux-uninstall-transaction.md) | user-scope 卸载快照、absent postcondition、取消/故障回滚、degraded fail-closed 与真实节点边界 |
| [M6 macOS launchd 服务事务审计](audits/2026-07-21-m6-launchd-service-transaction.md) | user-scope plist/manager 快照、同名重装、取消/故障回滚、degraded fail-closed 与真实 macOS 节点边界 |
| [M6 Windows Scheduled Task 服务事务审计](audits/2026-07-21-m6-windows-scheduled-task-transaction.md) | user-scope XML/state 快照、结构化探针、取消/故障回滚、degraded fail-closed 与真实 Windows 节点边界 |
| [Reasonix 2301e248..43993f5a 插件 Skill MCP 绑定审计](audits/2026-07-20-reasonix-2301e24-43993f5.md) | canonical MCP alias、package provenance、runtime Skill binding、权限/Hook/Evidence 身份与 cache/live 边界 |
| [Reasonix 8bb0e549..2301e248 增量审计](audits/2026-07-20-reasonix-8bb0e54-2301e24.md) | Provider env、MCP stdio/lifecycle、中断轮次恢复、WebKit focus 采用及 Remote SSH 延后 |
| [Reasonix 43993f5a..77fd1a47 与 Codex 增量审计](audits/2026-07-21-reasonix-43993f5-77fd1a4-codex-delta.md) | ACP steering、workspace key、cc-switch/Goal、危险宽泛 exec allow fail-closed 与后续路线分类 |
| [Hermes / MiMo / Scream / Kimi / Impeccable 最新增量审计](audits/2026-07-21-upstream-hermes-mimo-scream-kimi-impeccable-delta.md) | 五个参考上游的最新完整 SHA、代码级安全/恢复/Transcript/检索/Hook 分类与 P9/P10/M7 路由 |
| [Codex / Claude / Hermes / Grok 最新增量审计](audits/2026-07-20-upstream-strategic-reference-delta.md) | 二级战略代码审查、Claude 无新增与三级机制信号的明确层级边界 |
| [Codex / Hermes 提交前最新增量审计](audits/2026-07-20-codex-hermes-late-delta.md) | Codex TUI/command lifecycle 三提交代码级分类、Kimi 无签名 thinking 窄回放修复及最新冻结 |
| [Codex / Hermes 最终移动增量审计](audits/2026-07-20-codex-hermes-final-delta.md) | paginated thread name、动态 cell 测量、subagent 请求去重与可信 Desktop benchmark 合同 |
| [Hermes / MiMo 最终移动增量审计](audits/2026-07-20-hermes-mimo-final-delta.md) | custom endpoint 等价、selector/冷启动/首 token 信号、provenance 分歧与 checkpointed Skill state 候选 |
| [Hermes / Impeccable 最新增量审计](audits/2026-07-19-upstream-hermes-impeccable-delta.md) | Hermes installer、dotenv、MCP/Markdown 代码级分类与 Impeccable 设计/治理信号 |
| [Codex / Hermes / Scream 提交前增量审计](audits/2026-07-19-codex-hermes-scream-precommit-delta.md) | Codex audio/code-mode 与 paginated App-Server 线程语义、Hermes Desktop 订阅粒度及 Scream TUI 性能信号 |
| [Reasonix 2335d0df..a46fc6f 增量审计](audits/2026-07-19-reasonix-2335d0d-a46fc6f.md) | 测试隔离、Windows batch/save/session/resize、context window、本地可靠性采用及明确延后/拒绝 |
| [Reasonix 与全参考最新再冻结审计](audits/2026-07-19-upstream-a46fc6f-reference-delta.md) | Reasonix/Codex/Claude 三层治理、Hermes delivery/watchdog、MiMo Skill、Scream bug audit 与 Kimi headless/file 机制分类 |
| [Reasonix a46fc6f..65fcd465 增量审计](audits/2026-07-19-reasonix-a46fc6f-65fcd46.md) | LongCat context、Linux WebKit、可靠会话导出采用，Theme schema 延后与 Remote SSH P11 分类 |
| [Reasonix 65fcd465..8bb0e549 增量审计](audits/2026-07-19-reasonix-65fcd46-8bb0e54.md) | 设置刷新保留活动 Theme Pack、最新配置基底恢复与 Reames 统一 theme runtime 等价证据 |
| [M5 MCP 身份绑定信任审计](audits/2026-07-17-m5-mcp-identity-trust.md) | Reasonix MCP identity receipt、capability drift、launcher exact lock、Desktop reverify 与 destructive fresh-human 闭环 |
| [P2 Offline Guard / Safe Mode 审计](audits/2026-07-17-p2-offline-guard-safe-mode.md) | Guard 启动账本、安装单元回滚、Safe Mode、统一恢复投影及 Reasonix 最新可靠性吸收证据 |
| [P3 Recovery Center 与 Reasonix 方向审计](audits/2026-07-17-p3-recovery-reasonix-direction.md) | Desktop Recovery Center、三平台安装态 smoke、DeepSeek reasoning-only stop、发布棘轮与 P4 代际差距/P5 Theme Pack 方向 |
| [Hermes Gateway 参考审计](audits/2026-07-09-hermes-gateway-reference.md) | Hermes 后台社交通道网关、service manager 和安装机制审计 |
| [安装与部署统一性审计](audits/2026-07-09-install-deploy-governance.md) | Reasonix、Hermes 与 Reames 安装/部署入口统一性审计 |

## 验证入口

```powershell
.\scripts\verify-baseline.ps1
.\scripts\verify-baseline.ps1 -OutputDir "$env:TEMP\reames-agent-baseline-custom"
python scripts/check_public_readiness.py
python scripts/check_release_contracts.py
go test ./internal/... -count=1 -timeout 300s
Push-Location desktop; go test . -count=1 -timeout 300s; Pop-Location
Push-Location desktop/frontend; corepack pnpm test:all; corepack pnpm build; Pop-Location
```

`verify-baseline.ps1` 默认把 Gateway smoke 报告写入系统临时目录
`$env:TEMP\reames-agent-baseline`；只有需要保留或隔离证据时才传
`-OutputDir`，不会默认改动仓库 `artifacts/`。

根目录 `go test ./...` 不覆盖独立的 `desktop/` Go module。
