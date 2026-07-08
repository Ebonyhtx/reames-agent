# Reames Agent — 文档索引

> 新接手者先读本页，再读两份权威文档。历史文档只作证据库，方向以“权威接手文档 + 执行计划”为准。

## 必读入口

| 文档 | 用途 |
|---|---|
| `docs/REAMES_AGENT_AUTHORITY.md` | **权威接手文档 v2**：确认 `F:\reames-agent` 是新主项目，说明 Reasonix 底座、桌面 Agent 优先级、Reames Lite/其他参考项目分别保留什么。 |
| `docs/REAMES_AGENT_EXECUTION_PLAN.md` | **唯一执行计划 v2**：按当前真实路线推进桌面 UI、公共边界、legacy 清理、云部署和 Gateway 融合。 |
| `README.md` / `README.zh-CN.md` | 项目介绍与快速开始。 |
| `AGENTS.md` | AI 编程助手工作指南。 |
| `CONTRIBUTING.md` | 开发者贡献指南。 |

## 架构与设计

| 文档 | 用途 |
|---|---|
| `docs/ARCHITECTURE.md` | 当前仓库架构总览、模块边界和技术约束。 |
| `docs/PRODUCT_ROADMAP.md` | 产品路线图和阶段定义。 |
| `docs/FUTURE_PLAN.md` | v0.1.0 到 v1.0.0 的详细路线。 |
| `docs/REFERENCE_PORTING_ROADMAP.md` | 参考项目迁移分析。 |
| `docs/P0_VERIFICATION_REPORT.md` | P0 移植项源码对照验证。 |

## 部署

| 文档/目录 | 用途 |
|---|---|
| `docs/DEPLOY.md` | Docker、systemd、nginx、SSH 部署指南。 |
| `Dockerfile` | Docker 镜像构建。 |
| `docker-compose.yml` | 一键部署编排。 |
| `deploy/` | systemd/nginx 等配置文件。 |

## 工具系统

| 文档 | 用途 |
|---|---|
| `docs/TOOL_CONTRACT.md` | 内置工具清单与契约（英文）。 |
| `docs/TOOL_CONTRACT.zh-CN.md` | 内置工具清单与契约（中文）。 |

## 当前基线验证

优先使用：

```powershell
.\scripts\verify-baseline.ps1
```

该脚本覆盖 root Go CLI/provider/agent 关键测试，以及 nested `desktop/` 模块的关键桌面基线测试。注意：root `go test ./...` 不会覆盖 `desktop/` nested module。

## 执行计划归档

历史 phase 文档保留为证据和审计记录，不再作为新的方向入口：

| 文档 | 状态 |
|---|---|
| `docs/plans/PHASE1_REBRAND.md` | 已完成/历史记录 |
| `docs/plans/PHASE2_CLOUD_DEPLOY.md` | 已完成/历史记录 |
| `docs/plans/PHASE3_IM_GATEWAY.md` | 已完成/历史记录 |
| `docs/plans/PHASE4_BRAND_VISUAL.md` | 已完成/历史记录 |
| `docs/plans/PHASE5_CONTRACT_MIGRATION.md` | 已完成/历史记录 |
| `docs/plans/PHASE6_VERIFICATION.md` | 已完成/历史记录 |
