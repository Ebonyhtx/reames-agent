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
| [部署指南](DEPLOY.md) | 服务器 CLI、独立 Gateway service、Docker、systemd、nginx 与远程部署 |
| [云端 Agent 计划](CLOUD_AGENT_PLAN.md) | 服务器 CLI、社交通道 Gateway、上游研究 Worker、遥测反馈闭环 |
| [配置路径（中文）](CONFIG_PATHS.zh-CN.md) / [English](CONFIG_PATHS.md) | 配置、状态、缓存和迁移路径 |
| [协作模式](COLLABORATION_MODES.zh-CN.md) | Plan、Goal 与省 token 模式 |
| [工具审批模式](TOOL_APPROVAL_MODES.zh-CN.md) | 询问、自动和 Yolo 权限语义 |
| [桌面 Hooks](DESKTOP_HOOKS.zh-CN.md) | Hook 配置、事件和 payload |
| [插件包（中文）](PLUGIN_PACKAGES.zh-CN.md) / [English](PLUGIN_PACKAGES.md) | 插件包安装与管理 |

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
| [Script surface cleanup audit](audits/2026-07-09-script-surface-cleanup.md) | 公开脚本入口清理、旧运行时脚本删除和防回归门禁 |
| [Telemetry and feedback boundary audit](audits/2026-07-09-telemetry-feedback-boundary.md) | 遥测/反馈默认关闭、自托管边界和公开门禁证据 |
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
| [外部 Agent 批次验收](audits/2026-07-11-external-agent-batch-acceptance.md) | 大批提交的保留/撤回范围、真实性修复和全量本地验证 |
| [Control transport boundary ratchet audit](audits/2026-07-10-control-boundary-ratchet.md) | Desktop/CLI/Serve/Bot/ACP 对 runtime 内部直连的精确依赖基线和 CI 收缩棘轮 |
| [Gateway service home binding audit](audits/2026-07-09-gateway-home-binding.md) | 后台 gateway service 与 CLI 共用 `REAMES_AGENT_HOME` 的部署一致性证据 |
| [Gateway service lifecycle contract audit](audits/2026-07-09-gateway-service-lifecycle.md) | 跨平台后台 gateway 生命周期命令、CLI/Gateway 隔离和防 `serve`/`bot start` 回归证据 |
| [Gateway command contract audit](audits/2026-07-10-gateway-command-contract.md) | IM Gateway 平台无关命令解析、`/current` 状态别名和前缀误触发防回归证据 |
| [Installer release artifact mode audit](audits/2026-07-09-installer-release-mode.md) | 安装器默认源码构建、显式 release artifact 模式与 SHA256SUMS 校验证据 |
| [Installer dry-run contract audit](audits/2026-07-10-installer-dry-run-contract.md) | 一键安装器 dry-run 的 Gateway home 绑定、`.env` 凭据来源和 release 校验契约 |
| [Headless Gateway smoke audit](audits/2026-07-10-headless-gateway-smoke.md) | 隔离 `REAMES_AGENT_HOME` 下真实 CLI 的 `gateway doctor --home` 与 service dry-run 烟测 |
| [Setup Gateway preflight audit](audits/2026-07-10-setup-gateway-preflight.md) | `reames-agent setup` 完成后提示 Gateway doctor 和 service dry-run 的交接证据 |
| [Hermes Gateway 参考审计](audits/2026-07-09-hermes-gateway-reference.md) | Hermes 后台社交通道网关、service manager 和安装机制审计 |
| [安装与部署统一性审计](audits/2026-07-09-install-deploy-governance.md) | Reasonix、Hermes 与 Reames 安装/部署入口统一性审计 |

## 验证入口

```powershell
.\scripts\verify-baseline.ps1
python scripts/check_public_readiness.py
python scripts/check_release_contracts.py
go test ./internal/... -count=1 -timeout 300s
Push-Location desktop; go test . -count=1 -timeout 300s; Pop-Location
Push-Location desktop/frontend; corepack pnpm test:all; corepack pnpm build; Pop-Location
```

根目录 `go test ./...` 不覆盖独立的 `desktop/` Go module。
