# Reames Agent — 文档索引

## 入口文档

| 文档 | 用途 |
|---|---|
| `README.md` | 项目介绍、快速开始 |
| `README.zh-CN.md` | 中文版项目介绍 |
| `AGENTS.md` | AI 编程助手工作指南（给接手的人看） |
| `CONTRIBUTING.md` | 开发者贡献指南 |

## 架构与设计

| 文档 | 用途 |
|---|---|
| `docs/ARCHITECTURE.md` | 架构权威文档（模块图、隔离规则、缓存约束） |
| `docs/PRODUCT_ROADMAP.md` | 产品路线图和阶段定义 |
| `docs/FUTURE_PLAN.md` | v0.1.0→v1.0.0 详细路线图 |
| `docs/REFERENCE_PORTING_ROADMAP.md` | 9 个参考项目的移植分析 |
| `docs/P0_VERIFICATION_REPORT.md` | P0 移植项源码对照验证 |

## 部署

| 文档 | 用途 |
|---|---|
| `docs/DEPLOY.md` | Docker/systemd/nginx/SSH 四种部署方式 |
| `Dockerfile` | Docker 镜像构建 |
| `docker-compose.yml` | 一键部署 |
| `deploy/` | systemd + nginx 配置文件 |

## 工具系统

| 文档 | 用途 |
|---|---|
| `docs/TOOL_CONTRACT.md` | 内置工具清单（英文） |
| `docs/TOOL_CONTRACT.zh-CN.md` | 内置工具清单（中文） |

## 执行计划

| 文档 | 状态 |
|---|---|
| `docs/plans/PHASE1_REBRAND.md` | ✅ 完成 |
| `docs/plans/PHASE2_CLOUD_DEPLOY.md` | ✅ 完成 |
| `docs/plans/PHASE3_IM_GATEWAY.md` | ✅ 完成 |
| `docs/plans/PHASE4_BRAND_VISUAL.md` | ✅ 完成 |
| `docs/plans/PHASE5_CONTRACT_MIGRATION.md` | ✅ 完成 |
| `docs/plans/PHASE6_VERIFICATION.md` | ✅ 完成 |

## 新增模块速查

| 包 | 功能 | 行数 |
|---|---|---|
| `internal/crypto/` | AES-256-GCM + Argon2id | ~145 |
| `internal/trust/` | HTML清洗 + 输出信封 + 密钥脱敏 | ~70 |
| `internal/cron/` | 持久化定时任务 | ~200 |
| `internal/board/` | 统一工作台状态投影 | ~97 |
| `internal/provider/classify.go` | 错误分类器(12种故障原因) | ~100 |
| `internal/cli/toolstatus.go` | 工具状态格式化(28 emoji+verb) | ~80 |
| `internal/lsp/` | LSP Delta基线 | ~70 |
| `internal/tool/builtin/websearch.go` | web_search 工具 | ~174 |
| `internal/tool/builtin/applypatch.go` | apply_patch 工具 | ~175 |
| `internal/tool/builtin/cronjob.go` | cronjob 工具 | ~120 |
| `internal/bot/telegram/` | Telegram 适配器 | ~180 |
| `internal/control/pending_snapshot.go` | Pending Prompt 快照 | ~71 |
| `internal/pluginpkg/registry.go` | Plugin 注册表 | ~90 |
| `internal/event/event.go` | +4 种事件类型 | ~11 |
| `internal/hook/hook.go` | Hook glob 匹配 | ~30 |
| `internal/config/credentials.go` | 加密凭证存储 | ~60 |
