# Reames Agent 文档索引

文档按“当前事实优先”组织。过时方案直接删除，由 Git 历史承担归档职责。

## 新接手者

按顺序阅读：

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

## 实现记录

以下文档描述已实现机制或保留审计证据，不决定当前优先级：

| 文档/目录 | 用途 |
|---|---|
| `docs/superpowers/specs/` | AutoResearch 已实现设计 |
| `docs/superpowers/audits/` | AutoResearch 验证记录 |
| [接管审计](audits/2026-07-09-takeover.md) | 初次代码与测试审计 |
| [首次上游审查](audits/2026-07-09-upstream-review.md) | Upstream Watch 建立时的差异快照 |
| [真实 Provider 最小验证](audits/2026-07-09-real-provider.md) | DeepSeek 鉴权、响应、用量与缓存命中证据 |
| [Desktop M1 桥接审计](audits/2026-07-09-desktop-m1-bridge.md) | Desktop Submit/Cancel 多工作区桥接自动化证据 |
| [Desktop M1 frontend workspace loop audit](audits/2026-07-09-desktop-m1-frontend-workspace-loop.md) | 前端 workspace 选择、发送、切换和停止的 tab 绑定回归证据 |
| [M1 文件写入闭环审计](audits/2026-07-09-m1-file-write-loop.md) | 真实 `write_file` 审批、补丁预览、落盘与 RewindCode 回退证据 |
| [M1 重连与恢复审计](audits/2026-07-09-m1-reconnect-recovery.md) | pending approval replay、Desktop tab 事件恢复和 pending snapshot diff 诊断证据 |
| [M1 失败场景合同审计](audits/2026-07-09-m1-failure-contracts.md) | provider 鉴权失败、审批超时阻塞写入和运行态复位证据 |
| [Gateway service home binding audit](audits/2026-07-09-gateway-home-binding.md) | 后台 gateway service 与 CLI 共用 `REAMES_AGENT_HOME` 的部署一致性证据 |
| [Gateway service lifecycle contract audit](audits/2026-07-09-gateway-service-lifecycle.md) | 跨平台后台 gateway 生命周期命令、CLI/Gateway 隔离和防 `serve`/`bot start` 回归证据 |
| [Installer release artifact mode audit](audits/2026-07-09-installer-release-mode.md) | 安装器默认源码构建、显式 release artifact 模式与 SHA256SUMS 校验证据 |
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
