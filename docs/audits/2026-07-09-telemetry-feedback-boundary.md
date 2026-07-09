# Telemetry and feedback boundary audit

Date: 2026-07-09

## 背景

用户希望未来云端 Reames Agent 能汇总遥测、反馈和 BUG，并把重复失败转为可执行维护任务。这一方向可行，但它必须是自托管、隐私优先、可审计的闭环，不能继承上游或第三方未知 endpoint。

## 当前实现事实

当前仓库已有两类客户端侧基础：

- `desktop/crash_app.go`：结构化 crash / exception / feedback / performance / bot report；包含用户路径、邮箱、API key、JWT、Bearer token、长 token 等脱敏逻辑。
- `desktop/metrics_app.go`：内容无关的 aggregate metrics 计数；只记录 signal/bucket/count，避免 prompts、文件路径、base URL、密钥和工具输出。

但默认上报是关闭的：

- `crashEndpoint = ""`
- `metricsEndpoint = ""`
- 没有 endpoint 时 `ReportCrash` fail closed。
- metrics 还要求 `desktop.metrics` 用户配置 opt-in，并要求 repository-owned endpoint。

## 本轮治理

新增 public-readiness 门禁：

- 桌面 crash / metrics endpoint 必须默认为空。
- crash reporting 必须在没有项目自有 endpoint 时 fail closed。
- metrics 代码必须保留 owned-endpoint gate 注释。
- `.github/workflows`、`desktop`、`deploy`、`scripts`、README 和常规 docs 中不得引入常见第三方遥测 secret 或 endpoint，例如 `SENTRY_DSN`、`POSTHOG_KEY`、`DATADOG_API_KEY`、`CRASH_ENDPOINT`、`TELEMETRY_ENDPOINT`、`METRICS_ENDPOINT`。

允许例外：

- `docs/CLOUD_AGENT_PLAN.md`：描述未来自托管反馈中心计划。
- `docs/PUBLIC_READINESS.md`：要求暂时不要配置 crash/telemetry secrets。
- `scripts/check_public_readiness.py`：保存门禁本身的 forbidden token 列表。

## 后续实现路线

真正的云端反馈闭环应按以下顺序实现：

1. 定义 `internal/feedback` 的稳定 schema，覆盖 crash、performance、bot diagnostic、user feedback 和 aggregate metrics。
2. 实现 `reames-agent feedback collect` 或 `serve` 子路由，只接受自有 token/本机 loopback/reverse proxy 后鉴权请求。
3. 写入本地 SQLite/JSONL 队列，默认不上传第三方。
4. 建立去重和聚类：错误类型、版本、平台、top frame、工具错误分类。
5. 生成维护 Issue 或本地 task，但不得自动提交源代码或发送对话全文。
6. 在 Desktop/CLI/Gateway 中提供“发送前预览摘要”和 opt-in 开关。

完成声明需要真实端到端证据：一次 crash 或用户反馈可以被收集、脱敏、去重，并转化为可审查维护项。
