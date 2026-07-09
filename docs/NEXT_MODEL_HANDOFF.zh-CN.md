# 下一位模型接手交接文档

日期：2026-07-10  
仓库：`F:\reames-agent`  
远端：`https://github.com/Ebonyhtx/reames-agent.git`  
当前主线：`main`

这份文档用于让新的大模型在没有完整聊天上下文的情况下接手本项目。请把它当作“会话状态快照 + 工作协议”，但仍以当前文件、Git 状态、CI 状态和真实命令输出为最终事实。

## 1. 用户的真实目标

用户希望我持续接手并推进 Reames Agent，在 2026-07-10 前尽可能达到高可信可交付状态。目标不是做一个局部 demo，而是逐步把项目推进成一个“全能 Agent”：

- 桌面、CLI、服务器、IM Gateway 多入口一致；
- 支持真实 API 与真实任务闭环；
- 能部署到国内云服务器，通过 SSH/CLI 和飞书等渠道随时交互；
- 自动追踪 DeepSeek Reasonix 上游和参考项目更新，但升级必须由人或接手机器审查判断；
- 遥测/反馈/BUG 汇总进入自托管闭环，用于后续自我迭代；
- 文档要持续清洁、同步路线图和实现状态；
- CI 要保持可用；
- 改动要成批推进，不要小步频繁 push 浪费 CI。

用户偏好中文沟通。汇报要实事求是，不要把“局部通过”说成“完美完成”。

## 2. 绝对不要踩的边界

当前工作区里有一份未跟踪文档：

```text
docs/audits/2026-07-09-reference-feature-gap-map.md
```

这是用户在侧边会话开的参考项目差距审计任务，还在审计文档可靠性。不要修改、不要格式化、不要暂存、不要提交它。上一位模型已多次确认只让它保持 `??` 未跟踪状态。

除非用户明确解除这个限制，否则任何 `git add .`、批量暂存或文档清理都必须避开它。

## 3. 项目源流和参考路径

项目基座：

- 主源码上游：`esengine/DeepSeek-Reasonix` 的 `main-v2`
- 本地上游参考：`F:\code-reference\DeepSeek-Reasonix`
- 其他机制参考：`F:\code-reference`
- 旧项目前身：`F:\Reames-Lite`

参考项目治理规则见：

- `docs/REFERENCE_GOVERNANCE.md`
- `docs/upstreams/README.md`
- `docs/DEVELOPMENT_PLAN.md`
- `docs/CLOUD_AGENT_PLAN.md`

重要原则：DeepSeek Reasonix 是源码上游；其他项目只吸收机制和体验，不直接盲目搬运。上游或参考项目更新后，应先研究、产出报告或草稿 issue，再由用户或接手机器判断是否升级。

## 4. 当前仓库状态快照

截至本交接文档编写前，最近主线提交为：

```text
9b8bcff feat: submit local feedback from cli
976baac feat: add feedback maintenance CLI
622cd47 feat: generate local feedback maintenance drafts
54aa56a feat: collect self-hosted feedback locally
f25ee6b fix: allow gateway smoke build on CI
7fa5d97 feat: bind foreground gateway to selected home
bfb0ade test: upload headless gateway smoke evidence
8dcb651 feat: print gateway preflight after setup
5baff3b test: smoke headless gateway deployment
eced451 feat: bind gateway doctor to selected home
890c5a5 feat: expose gateway doctor alias
235dfb6 test: cover installer dry-run contracts
```

远端 `9b8bcff` 的 GitHub Actions 状态已核验：

- CI：success
- CodeQL：success

但新接手者仍应先运行：

```powershell
git status --short
git log -5 --oneline
```

确认当前事实，没有假设上一轮状态仍然有效。

## 5. 最近完成的关键能力

### 5.1 Gateway / 云端部署入口

已经围绕 Hermes 式服务器部署推进了一批能力：

- `reames-agent gateway doctor [--json] [--deep] [--home PATH]`
- `reames-agent gateway run --home PATH`
- `reames-agent gateway install --dry-run --home PATH --channels ... --dir ...`
- service plan 会绑定 `REAMES_AGENT_HOME`
- service 定义不嵌入 secret 值
- Gateway 凭据来自 `<Reames Agent home>/.env`
- headless Gateway smoke 会构建真实 CLI，在隔离 home 下验证 doctor 和 service dry-run
- CI 上传 headless Gateway smoke 证据 artifact
- `reames-agent setup` 后会提示 Gateway preflight 命令

相关文档/审计：

- `docs/CLOUD_AGENT_PLAN.md`
- `docs/DEPLOY.md`
- `docs/audits/2026-07-09-gateway-home-binding.md`
- `docs/audits/2026-07-09-gateway-service-lifecycle.md`
- `docs/audits/2026-07-10-headless-gateway-smoke.md`
- `docs/audits/2026-07-10-setup-gateway-preflight.md`

### 5.2 自托管反馈闭环

最近几轮重点推进了自托管 telemetry/feedback 的第一阶段。当前已经有：

- `internal/feedback`
  - 稳定 schema
  - 本地 JSONL 账本
  - 邮箱、用户路径、API key、Bearer token、JWT、长 token 等脱敏
  - fingerprint 去重聚合
  - 本地 Markdown 维护草稿生成
- HTTP 控制面：
  - `POST /api/feedback`
  - `GET /api/feedback/summary`
  - `POST /api/feedback/draft`
- SSH/CLI 运维入口：
  - `reames-agent feedback submit --home PATH --message TEXT`
  - `reames-agent feedback summary --home PATH [--json]`
  - `reames-agent feedback draft --home PATH [--json] [--limit N]`

这些入口只写本地账本和本地草稿，不调用第三方服务，不自动创建 GitHub Issue。

示例：

```powershell
reames-agent feedback submit --home "$env:REAMES_AGENT_HOME" --kind feedback --message "飞书发送失败，请检查日志"
reames-agent feedback summary --home "$env:REAMES_AGENT_HOME" --json
reames-agent feedback draft --home "$env:REAMES_AGENT_HOME" --limit 20
```

账本位置：

```text
<Reames Agent home>/feedback/feedback.jsonl
```

草稿位置：

```text
<Reames Agent home>/feedback/drafts/*.md
```

相关文件：

- `internal/feedback/feedback.go`
- `internal/feedback/feedback_test.go`
- `internal/serve/serve.go`
- `internal/serve/serve_test.go`
- `internal/cli/feedback.go`
- `internal/cli/cli_test.go`
- `docs/audits/2026-07-09-telemetry-feedback-boundary.md`
- `docs/audits/2026-07-10-feedback-collector.md`

## 6. 用户对“服务器部署 Agent”的真实需求

用户不是想把桌面端简单搬到服务器，也不是让所有入口都依赖 `serve`。更接近 Hermes 的形态：

```text
Aliyun ECS / 自有服务器
├─ CLI / TUI
│  ├─ reames-agent
│  ├─ reames-agent run
│  └─ SSH / tmux / screen / systemd-run
├─ Gateway daemon
│  ├─ reames-agent gateway run --home "$REAMES_AGENT_HOME" --channels feishu
│  ├─ reames-agent gateway install/start/stop/status
│  └─ 飞书 / Lark / 微信 / QQ / Telegram adapter
├─ serve
│  └─ 可选 HTTP/SSE/Web/API 控制面，不是 CLI 或 Gateway 的前置条件
├─ Desktop
│  └─ 本地电脑可选 UI
├─ upstream research worker
└─ telemetry feedback collector
```

核心原则：

- CLI、Gateway、serve、Desktop 是并列入口；
- Gateway 作为后台服务常驻，不占用用户的 CLI 终端；
- SSH 终端仍然可以像本机一样使用 CLI；
- 所有入口最终复用 `internal/control.Controller` 和相同权限/证据模型；
- 不要为了远程方便绕过审批、沙箱、检查点或密钥脱敏。

## 7. 当前仍未完成的关键缺口

不要把当前项目说成“完美”。以下仍是明确缺口：

1. 真实 IM 回环
   - 飞书/Lark 真实 bot 凭据、真实消息进入、回复、审批、取消、恢复尚需端到端实测。

2. 真实云服务器部署证据
   - 仍缺一台干净 Linux/阿里云 ECS 上的实际安装、`REAMES_AGENT_HOME`、真实 API、Gateway service、CLI run、日志、备份验证。

3. Desktop 完整高可信
   - Windows portable 已有 smoke，但 Linux/macOS Desktop candidate 安装/启动 smoke 仍在路线图中。
   - 原生 Desktop 交互、真实 API 长任务、失败恢复还需继续扩展实证。

4. Feedback 用户入口
   - 后端和 CLI 已有本地 submit/summary/draft。
   - 仍缺 Desktop/Gateway 侧用户可见“发送前预览摘要 + opt-in”入口。

5. GitHub Issue 草稿发布
   - 当前只生成本地 Markdown 草稿。
   - 后续需要“人工确认后发布 Issue”的流程，禁止自动外发。

6. 上游研究 Worker
   - 已有 Upstream Watch 发现/契约能力。
   - 仍需从“发现更新”升级到“自动研究差异、生成报告、建议补丁/issue，但不自动合并”。

7. 参考项目差距审计
   - `docs/audits/2026-07-09-reference-feature-gap-map.md` 由侧边会话审计，当前接手者不要动。

## 8. 推荐下一步路线

优先继续 M6/C4，因为它和用户最近需求最贴近：

1. 做一个真实云节点部署 smoke/runbook
   - 目标：干净 Linux 节点或本地模拟 Linux 流程能执行安装、配置 home、CLI run、Gateway doctor、feedback submit/summary/draft。
   - 可先增强现有脚本，再补文档，不要伪造真实云证据。

2. 给 feedback 增加人工 Issue 草稿发布前的本地审核流程
   - 可以先做 `reames-agent feedback issue-draft` 或类似命令，只生成 GitHub Issue JSON/Markdown，不调用 API。
   - 真正调用 GitHub API 必须有显式人工确认或用户授权。

3. 接 Desktop/Gateway 的 feedback submit
   - Desktop crash/metrics 已有客户端侧基础；把它接到本地/serve schema 前，要保留 opt-in 和预览。
   - Gateway 失败可以结构化写入 feedback ledger，但不能泄漏消息正文/密钥。

4. 做真实 Feishu Gateway 回环准备
   - 不要硬编码凭据。
   - 文档写清飞书配置、`.env`、`gateway doctor --deep --home`、前台 `gateway run`、service install/start。
   - 如果没有真实凭据，就明确停在“待用户提供真实 bot 凭据/服务器回调配置”。

5. 上游研究 Worker
   - 先读 `internal/autoresearch`、`docs/upstreams/README.md`、`scripts/check_upstreams.py`。
   - 目标是“研究报告/issue 草稿”，不是自动升级。

## 9. 验证命令

常用本地验证：

```powershell
go build ./...
go vet ./...
go test ./internal/... -count=1 -timeout 300s
python scripts\check_docs_contracts.py
python scripts\check_deploy_contracts.py
python scripts\check_public_readiness.py
python scripts\check_release_contracts.py
git diff --check
```

Desktop 独立模块：

```powershell
Push-Location desktop
go test . -count=1 -timeout 300s
Pop-Location

Push-Location desktop/frontend
corepack pnpm test:all
corepack pnpm build
Pop-Location
```

Gateway/部署聚焦：

```powershell
go test ./internal/cli ./internal/gatewayservice ./internal/bot ./internal/botruntime -count=1 -timeout 300s
python scripts\check_deploy_contracts.py
python scripts\smoke_gateway_headless.py --out artifacts\headless-gateway-smoke.json
Remove-Item -LiteralPath artifacts\headless-gateway-smoke.json -Force
```

Feedback 聚焦：

```powershell
go test ./internal/cli ./internal/feedback ./internal/serve -count=1 -timeout 300s
```

上游追踪：

```powershell
python -m unittest scripts.test_check_upstreams -v
node scripts/test_upstream_watch_issue.mjs
python scripts/check_upstreams.py --out-dir artifacts/upstream-watch
```

## 10. GitHub Actions 查询方式

当前环境中 `gh` 可能不可用，但用户已配置 `GITHUB_TOKEN` 到 Windows User env。可用 PowerShell 调 GitHub REST：

```powershell
$token=[Environment]::GetEnvironmentVariable('GITHUB_TOKEN','User')
$headers=@{
  'Accept'='application/vnd.github+json'
  'X-GitHub-Api-Version'='2022-11-28'
  'User-Agent'='reames-agent-codex'
}
if($token){$headers['Authorization']='Bearer '+$token}
$r=Invoke-WebRequest -UseBasicParsing -Headers $headers -Uri 'https://api.github.com/repos/Ebonyhtx/reames-agent/actions/runs?per_page=8'
($r.Content|ConvertFrom-Json).workflow_runs |
  Select-Object name,status,conclusion,head_sha,html_url,created_at |
  ConvertTo-Json -Depth 4
```

查看某个 run 的 job：

```powershell
$run = '<run-id>'
$r=Invoke-WebRequest -UseBasicParsing -Headers $headers -Uri "https://api.github.com/repos/Ebonyhtx/reames-agent/actions/runs/$run/jobs?per_page=50"
($r.Content|ConvertFrom-Json).jobs |
  Select-Object name,status,conclusion,started_at,completed_at,html_url |
  ConvertTo-Json -Depth 4
```

## 11. 工作方式建议

- 先读代码再改，尤其是 `internal/` 下 package comment。
- 新功能尽量加在 `control` 或共享 runtime 层，不要让 CLI/serve/Desktop 各自实现不同 Agent 行为。
- 新模块必须有 `_test.go` 或覆盖到现有测试。
- 每批改动同步文档、测试、提交和 push。
- 不要小改一次 push 一次；按用户要求，尽量攒成有意义的大步后再验证/push。
- 不要提交二进制或 `artifacts/` 生成物。
- 不要用 `git reset --hard` 或破坏用户未提交改动。
- 当前工作区可能有用户/侧边会话文件；暂存时显式列文件，不要 `git add .`。
- 公开仓库已开启，公开前准备已完成；后续仍要保持 public readiness 门禁。

## 12. 新接手者启动清单

新模型接手后建议按顺序执行：

```powershell
git status --short
git log -8 --oneline
python scripts\check_public_readiness.py
python scripts\check_docs_contracts.py
```

然后阅读：

1. `docs/PROJECT.md`
2. `docs/DEVELOPMENT_PLAN.md`
3. `docs/CLOUD_AGENT_PLAN.md`
4. `docs/DEPLOY.md`
5. `docs/REFERENCE_GOVERNANCE.md`
6. `docs/upstreams/README.md`
7. `docs/audits/2026-07-10-feedback-collector.md`
8. `docs/audits/2026-07-10-headless-gateway-smoke.md`

最后根据当前用户最新指令选择一个大步推进。若用户没有新方向，推荐从“云端真实部署 smoke + feedback/Gateway 运维闭环”继续。

## 13. 不要误读的点

- “自动检测上游/参考项目更新”不是自动升级。正确形态是自动发现、自动研究、自动产出可审查报告/Issue/草稿补丁，再由用户或接手机器判断是否升级。
- “服务器部署 Agent”不是把桌面端搬上服务器，也不是所有入口都依赖 `serve`。正确形态是 CLI、Gateway、serve、Desktop 并列。
- “遥测反馈”默认不是第三方 telemetry。当前路线是自托管、本地账本、脱敏、人工确认后再外发。
- “完美”必须靠证据，不靠口号。完成声明要引用测试、真实交互、CI、发布 artifact 或运行日志。

