# Reames Agent — 部署指南

## 推荐形态：CLI + 独立 Gateway

如果目标是“像 Hermes 一样把 Agent 部署到云服务器，然后随时 SSH 上去使用，同时飞书/微信/QQ/Telegram 在后台常驻”，部署形态应分成两个互不干扰的入口：

1. **CLI/TUI**：用户 SSH 到服务器后直接运行 `reames-agent` 或 `reames-agent run`。
2. **Gateway service**：后台服务独立接收社交通道消息，前台调试命令是 `reames-agent gateway run --home "$REAMES_AGENT_HOME" --channels feishu`（`reames-agent bot start --home "$REAMES_AGENT_HOME" --channels feishu` 保持兼容），后台服务生命周期命令是 `reames-agent gateway install/start/status`。

这种形态和本机 CLI 最接近：

- `ssh` 进入服务器后运行 `reames-agent`，得到交互式 CLI/TUI；
- 用 `reames-agent run "..."` 执行一次性任务；
- 用 `tmux` / `screen` 保持长任务不断线；
- gateway 在 systemd / Windows Scheduled Task / launchd 等后台服务中运行，不占用 CLI 终端；
- provider key 保存在该服务器用户的 `<Reames Agent home>/.env`；
- `serve` 是后续可选 Web/API 控制面，不是 CLI 或 gateway 的前置条件。

### 0. 一键安装脚本

公开稳定 release 还未开启前，官方安装脚本采用源码构建方式，需要目标机器已有 Git 和 Go 1.25+：

```bash
curl -fsSL https://raw.githubusercontent.com/Ebonyhtx/reames-agent/main/scripts/install.sh | bash
curl -fsSL https://raw.githubusercontent.com/Ebonyhtx/reames-agent/main/scripts/install.sh | bash -s -- --home "$HOME/.reames-agent" --gateway --channels feishu --gateway-dir /srv/reames-work
```

Windows PowerShell：

```powershell
powershell -ExecutionPolicy Bypass -c "iex (irm https://raw.githubusercontent.com/Ebonyhtx/reames-agent/main/scripts/install.ps1)"
powershell -ExecutionPolicy Bypass -File scripts\install.ps1 -AgentHome "$env:APPDATA\reames-agent" -Gateway -Channels feishu -GatewayDir F:\reames-work
```

所有会安装后台 gateway 的路径都建议先使用 dry-run：

```bash
scripts/install.sh --dry-run --home "$HOME/.reames-agent" --gateway --channels feishu
```

```powershell
.\scripts\install.ps1 -DryRun -AgentHome "$env:APPDATA\reames-agent" -Gateway -Channels feishu
```

安装器 dry-run 会打印将要执行的 `gateway install --home ...` 命令、Gateway
凭据来源 `<Reames Agent home>/.env`，并声明服务定义只绑定
`REAMES_AGENT_HOME`、不嵌入 secret 值。
仓库里的 `python scripts/smoke_gateway_headless.py` 会用隔离 home 和真实 CLI
二进制验证 `gateway setup --dry-run` 零落盘、正式 setup 原子写入和幂等重跑，
再验证 `gateway doctor --home` 与 `gateway install --dry-run --home`。烟测不手写
Gateway TOML、不创建 synthetic `.env`，doctor 应准确报告 secret 环境变量未设置，
因此适合作为无真实 IM secret 的服务器部署预检；加上
`--out artifacts/headless-gateway-smoke.json` 可保存机器可读的部署预检证据。

Linux 上还可运行 `python scripts/smoke_gateway_service_linux.py --binary /absolute/path/to/reames-agent --out artifacts/gateway-systemd-user-smoke.json`，用随机本地 Feishu webhook challenge 验证真实 systemd user service 的安装、同名重装、status、restart、stop/start、journal 和 uninstall。该 smoke 不需要真实 Provider/IM 凭据，但必须在已经运行的 systemd user manager 中执行；报告中的 `linger_state` 和 `external_blocked` 必须保留，登录会话内通过不等于 SSH 断开或云主机重启后仍常驻。

未来开启稳定 GitHub Release 后，安装器已经预留“预构建产物优先”的显式路径，但默认仍保持源码构建，避免在 pre-stable 阶段给用户制造已经发布稳定二进制的错觉：

```bash
scripts/install.sh --binary-source release --version v0.1.0 --home "$HOME/.reames-agent"
```

```powershell
.\scripts\install.ps1 -BinarySource release -Version v0.1.0 -AgentHome "$env:APPDATA\reames-agent"
```

release 模式只信任当前 Reames 仓库的 GitHub Release artifact 命名：`reames-agent-linux-amd64.tar.gz`、`reames-agent-darwin-arm64.tar.gz`、`reames-agent-windows-amd64.zip` 等，并会下载 `SHA256SUMS` 做校验。没有正式 release 时不要使用该模式；服务器部署继续使用默认 source 模式或手动上传已验证的候选二进制。

### 1. 创建低权限用户

```bash
sudo useradd --create-home --shell /bin/bash reames
sudo install -d -o reames -g reames /opt/reames-agent/bin
```

### 2. 安装二进制

从本机上传已构建的 Linux 二进制：

```bash
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/reames-agent-linux-amd64 ./cmd/reames-agent
scp bin/reames-agent-linux-amd64 user@server:/tmp/reames-agent
ssh user@server "sudo install -m 755 /tmp/reames-agent /opt/reames-agent/bin/reames-agent"
ssh user@server "sudo ln -sf /opt/reames-agent/bin/reames-agent /usr/local/bin/reames-agent"
```

或在服务器上从源码构建：

```bash
git clone https://github.com/Ebonyhtx/reames-agent.git
cd reames-agent
go build -o /tmp/reames-agent ./cmd/reames-agent
sudo install -m 755 /tmp/reames-agent /opt/reames-agent/bin/reames-agent
sudo ln -sf /opt/reames-agent/bin/reames-agent /usr/local/bin/reames-agent
```

### 3. 配置服务器用户的 Agent home 和 API key

```bash
sudo -iu reames
export REAMES_AGENT_HOME="$HOME/.reames-agent"
mkdir -p "$REAMES_AGENT_HOME"
install -m 600 /dev/null "$REAMES_AGENT_HOME/.env"
printf '%s\n' 'DEEPSEEK_API_KEY=replace-with-your-key' >> "$REAMES_AGENT_HOME/.env"
reames-agent doctor
```

也可以运行 `reames-agent setup` 交互式写入 provider 配置和凭据。setup 完成后
会打印 Gateway preflight 提示：先执行
`reames-agent gateway doctor --deep --home "$REAMES_AGENT_HOME"` 检查配置、凭据
env、访问控制和连接记录，再执行
`reames-agent gateway install --dry-run --home "$REAMES_AGENT_HOME"` 审阅后台服务
计划。

`<Reames Agent home>/.env` 是 provider key 的运行时来源。不要把真实 key 写入项目仓库、项目 `.env`、systemd unit 或 shell 历史可见的位置。长期使用时，可以把 `export REAMES_AGENT_HOME="$HOME/.reames-agent"` 写入该用户的 shell profile。

源码检出还可以先运行 credential-free 运维预检：

```bash
python scripts/smoke_gateway_headless.py --out artifacts/headless-gateway-smoke.json
```

它用实际 CLI 二进制和隔离 home 验证 Gateway 配置/诊断/service plan、localhost
Provider 一次性任务与会话落盘，以及 feedback 提交/聚合/脱敏/本地草稿。报告中的
`external_blocked` 仍需在真实云节点、Provider 和 IM 应用中补证；该命令不会创建
synthetic `.env`，也不会启动或发布外部服务。

### 4. 像本机一样使用 CLI

```bash
sudo -iu reames
cd /srv/projects/your-repo
reames-agent
reames-agent run "审查这个项目并给出风险"
echo "修复失败测试并说明验证结果" | reames-agent run
```

长任务建议放进 tmux：

```bash
sudo -iu reames
tmux new -s reames
cd /srv/projects/your-repo
reames-agent
```

断开 SSH 后用 `tmux attach -t reames` 回来。这个模式最接近桌面电脑上的 CLI 体验。

### 5. 一次性后台任务

对不需要交互的维护任务，可以用 `systemd-run` 临时托管：

```bash
sudo systemd-run \
  --uid=reames \
  --working-directory=/srv/projects/your-repo \
  --setenv=REAMES_AGENT_HOME=/home/reames/.reames-agent \
  --unit=reames-agent-once \
  /usr/local/bin/reames-agent run "生成本周项目维护报告"

journalctl -u reames-agent-once -f
```

这比把 `serve` 当作唯一云端入口更符合“服务器上的 CLI Agent”定位。

### 6. 社交通道 Gateway 后台服务

当前 Reames 已有前台运行入口：

```bash
reames-agent gateway run --home "$REAMES_AGENT_HOME" --channels feishu
reames-agent bot start --home "$REAMES_AGENT_HOME" --channels feishu
```

它适合调试、tmux 或临时运行；`bot start` 是兼容入口，长期推荐使用 `gateway run`。后台服务管理面当前已提供用户级 systemd / launchd / Windows Scheduled Task 命令；生产部署前建议先用 `--dry-run` 审阅计划：

```bash
reames-agent gateway run --home "$REAMES_AGENT_HOME"  # 前台运行，适合调试/Docker/Termux
reames-agent gateway doctor --deep --home "$REAMES_AGENT_HOME"  # 只读检查配置、凭据 env、访问控制和连接记录
reames-agent gateway recovery-status --json --home "$REAMES_AGENT_HOME" --root /srv/project
reames-agent gateway install --dry-run --home "$REAMES_AGENT_HOME" --channels feishu --dir /srv/project
reames-agent gateway install --start-now --home "$REAMES_AGENT_HOME"    # 安装并启动后台服务
reames-agent gateway status                 # 查看 service-manager 状态
reames-agent gateway restart                # 重启服务，不影响用户 CLI 终端
reames-agent gateway uninstall              # 卸载后台服务
journalctl --user -u reames-agent-gateway.service -f  # Linux 实时日志
```

`gateway install --dry-run` 会在计划里显示绑定的 `REAMES_AGENT_HOME` 和
`<Reames Agent home>/.env` 凭据来源。真实 API key、飞书/QQ/微信 secret 等
仍保存在这个 `.env` 文件里；systemd unit、launchd plist 和 Windows
Scheduled Task 不会嵌入 secret 值。

后台 gateway 的职责：

- 独立进程常驻，不占用用户 SSH CLI；
- 每个平台 adapter 独立连接和重连；
- 每个 chat/user/thread 映射到自己的 Agent session；
- 支持审批、取消、状态、恢复和 allowlist；
- 前台运行时日志写入 stderr；service-manager 部署时由 journald、launchd 或 Scheduled Task 对应日志设施接管；
- 与 CLI、Desktop、serve 共用配置、凭据、会话和权限模型，但进程互相隔离。

实现前的临时服务器方案：

```bash
sudo -iu reames
tmux new -s reames-gateway
reames-agent gateway run --home "$HOME/.reames-agent" --channels feishu
```

正式命令会按平台选择 service manager：Linux 使用 user systemd，macOS 使用 launchd，Windows 使用 Scheduled Task。`--scope system` 只渲染计划并要求管理员/root 手动确认，避免误改系统级服务。

Linux user-scope 的 `gateway install` 会在写入后先执行 `systemd-analyze --user verify`，并在变更前快照旧 unit 的 bytes/mode 与 enabled/active 状态。写入、reload、enable、restart 或 is-active 失败时会尝试恢复快照；如果旧 unit 写回或恢复后的 daemon-reload 失败，会 fail closed、停止后续 manager 操作并明确要求人工修复。该自动回滚范围只覆盖 Linux user-scope install，不适用于 macOS launchd、Windows Scheduled Task、uninstall 或 `--scope system`。

`gateway run` 在加载 config、Provider、plugin 和 channel 之前执行共享的 credential-free recovery
preflight。systemd、launchd 与 Windows Scheduled Task 都因此复用 Guard 的 `repair.Report`，不会各自
维护恢复状态。若 service 拒绝启动，先运行 `gateway recovery-status` 和 `gateway doctor`；不要通过
复制 unit/plist/task 或新增旁路状态文件跳过预检。

### 6.1 Desktop Offline Guard 与 Safe Mode

打包后的 Windows/macOS/Linux Desktop 默认先经过 `reames-agent-guard`。连续启动失败、配置损坏、
installer failure 或 pending update 可在不读取 API key、不启动普通 Agent runtime 的情况下诊断：

```bash
reames-agent guard check --json
reames-agent guard launch --safe-mode
reames-agent guard repair
reames-agent guard rollback
```

五分钟内三次未完成启动会建议 Safe Mode；新版本在 DOM ready 后还需持续健康 30 秒，才删除回滚
备份并提交 pending update。自动回滚只在失败版本与 `toVersion` 一致、目标属于当前安装目录、
transaction identity 未变化且完整安装单元备份 SHA-256 全部有效时发生。来源或归因不明确时不会
改二进制，而是进入 Safe Mode 或要求从可信 release 重装。已有 pending 未清算时禁止再次准备更新；
Windows helper 缺失时自动更新 fail closed，Linux partial apply 会留下供 Guard 归因的 failure marker。

Safe Mode 不读取/迁移用户或项目 TOML、dotenv，不恢复旧 tab/session；Desktop 只建立 recovery-only
shell，`boot.Build` 拒绝 Provider、Controller、工具与普通 Agent 装配，并禁用 MCP、plugin、Hook、
Bot、LSP、planner、Guardian、subagent、Memory Compiler、更新检查、遥测和 metrics。
详细操作、跨平台打包入口和限制见 [恢复指南](RECOVERY.zh-CN.md)。

### 7. 备份、恢复与二进制回滚

备份前必须停止所有会写入 Reames home/state 的进程，包括 CLI、Desktop、`serve`、`bot`、Gateway 和 cron worker。`--offline` 是运维者对这一事实的显式确认，不是跨进程全局锁：

```bash
reames-agent backup create --offline --out /srv/backups/reames-2026-07-14.zip \
  --home "$REAMES_AGENT_HOME"
reames-agent backup verify /srv/backups/reames-2026-07-14.zip
```

当 `REAMES_AGENT_STATE_HOME` 与 home 不同时，create 和 restore 都应显式传 `--state-home`。归档排除 `.env`、legacy credentials、微信账号、pairing、cache 和 lock/lease 等已知凭据或运行时文件，但 session、memory 和自定义 config 仍可能包含用户粘贴的 secret，因此归档始终按敏感数据保存。Unix 会收紧归档和恢复文件权限；Windows 的实际保护还依赖目标目录 ACL。

`backup verify` 的内嵌 manifest 与逐文件 SHA-256 只证明归档自洽，不证明来源真实性。恢复前必须将命令输出的 archive SHA-256 与单独保存、可信传递的记录或签名比对：

```bash
reames-agent backup restore --dry-run \
  --home /srv/reames-restored \
  /srv/backups/reames-2026-07-14.zip
reames-agent backup restore --offline \
  --home /srv/reames-restored \
  /srv/backups/reames-2026-07-14.zip
```

restore 只接受不存在的 `NEW_PATH`，不支持原地覆盖。分根归档还必须提供一个不存在的 `--state-home NEW_PATH`；恢复后重新配置 Provider 和 IM 凭据。多根提交会在后续 publish/sync 失败时做进程内 best-effort 回滚，但没有 durable crash journal，断电或强杀后不能宣称跨根全局原子；失败或强杀还可能留下 `.reames-restore-*` staging 或仅创建的空 parent，应人工核对后清理。

CLI 自升级会先校验 SHA256SUMS，再实际执行候选二进制的 `version`；发布后再次健康检查，并把立即前一版本保留为 `<executable>.previous`。升级和回滚共用同目录互斥锁：

```bash
reames-agent upgrade
reames-agent upgrade --rollback
reames-agent gateway restart
```

发布或安装后健康检查失败会尝试恢复更新前的 current/previous 组合；`--rollback` 会交换二者并保留被替换版本。命令不会静默重启独立 Gateway，所以升级或回滚后应显式执行 `gateway restart`。这些保证有本地实际候选执行与故障注入证据，但没有 durable crash journal，也不等于公开签名 release 已在真实 Linux/macOS/Windows 安装上完成升级/回滚演练。

## Docker 部署（推荐）

```bash
# 构建镜像
docker build -t reames-agent .

# 启动（需设置 API Key）
docker run -d --name reames-agent \
  -p 8787:8787 \
  -v ~/.reames-agent:/root/.reames-agent \
  -e DEEPSEEK_API_KEY=replace-with-your-key \
  -e REAMES_AGENT_SERVE_TOKEN=change-this-long-random-token \
  reames-agent

# 查看日志
docker logs -f reames-agent

# 健康检查
curl http://localhost:8787/health
```

镜像内置健康检查等价于：

```bash
reames-agent serve --health-check
```

默认 Docker 入口会监听容器内 `0.0.0.0:8787`。如果把端口暴露到公网，必须先在配置中启用 `[serve] auth_mode = "token"` 或 `"password"`，或只绑定到可信内网地址。长期部署建议使用 `token_env`，不要把 token 写进配置文件：

```toml
[serve]
auth_mode = "token"
token_env = "REAMES_AGENT_SERVE_TOKEN"
```

## Docker Compose

```bash
# 复制环境变量模板
cp .env.example .env
# 编辑 .env 填入 API Key

# 启动
docker-compose up -d

# 查看状态
docker-compose ps
curl http://localhost:8787/health
```

## 云服务器部署（systemd）

```bash
# 1. 上传二进制
scp bin/reames-agent user@server:/opt/reames-agent/bin/

# 2. 创建配置目录
ssh user@server "mkdir -p /opt/reames-agent/data"

# 3. 设置环境变量；/opt/reames-agent/.env 会由 systemd EnvironmentFile 读取
ssh user@server "install -m 600 /dev/null /opt/reames-agent/.env"
ssh user@server "printf '%s\n' 'DEEPSEEK_API_KEY=replace-with-your-key' 'REAMES_AGENT_SERVE_TOKEN=change-this-long-random-token' >> /opt/reames-agent/.env"

# 4. 安装 systemd 服务
scp deploy/systemd/reames-agent.service user@server:/etc/systemd/system/
ssh user@server "systemctl daemon-reload && systemctl enable --now reames-agent"

# 5. 验证
ssh user@server "curl http://localhost:8787/health"
```

默认 systemd unit 只监听 `127.0.0.1:8787`，避免未配置认证时直接暴露到公网。对外访问应通过同机 Nginx/Caddy 反向代理，并在 Reames Agent 用户配置中启用 `serve.auth_mode`。推荐配置：

```toml
[serve]
auth_mode = "token"
token_env = "REAMES_AGENT_SERVE_TOKEN"
behind_proxy = true
```

## 自托管反馈收集（本地账本）

`serve` 现在提供自托管反馈入口，用于把崩溃、Gateway 诊断、用户反馈或性能问题先汇总到服务器本地账本：

```bash
curl -X POST http://127.0.0.1:8787/api/feedback \
  -H "Content-Type: application/json" \
  -H "X-API-Key: $REAMES_AGENT_SERVE_TOKEN" \
  -d '{"kind":"feedback","source":"gateway","label":"feishu","message":"发送失败，请检查日志"}'

curl -H "X-API-Key: $REAMES_AGENT_SERVE_TOKEN" \
  http://127.0.0.1:8787/api/feedback/summary

curl -X POST http://127.0.0.1:8787/api/feedback/draft \
  -H "Content-Type: application/json" \
  -H "X-API-Key: $REAMES_AGENT_SERVE_TOKEN" \
  -d '{"limit":20}'

reames-agent feedback submit --home "$REAMES_AGENT_HOME" \
  --kind feedback \
  --message "飞书发送失败，请检查日志"
reames-agent feedback summary --home "$REAMES_AGENT_HOME"
reames-agent feedback draft --home "$REAMES_AGENT_HOME" --limit 20
```

记录写入 `<Reames Agent home>/feedback/feedback.jsonl`，会先脱敏邮箱、用户路径、API key、Bearer token、JWT 和长 token，再按 fingerprint 聚合重复问题。`POST /api/feedback` 和 `reames-agent feedback submit` 都只写本地账本；`/api/feedback/draft` 和 `reames-agent feedback draft` 都会把聚合结果写成 `<Reames Agent home>/feedback/drafts/*.md` 本地维护草稿。HTTP API 适合 Web/API 控制面；CLI 命令适合 SSH/tmux 运维，不需要启动 `serve`。这些入口不连接第三方服务，也不自动创建 Issue；后续把草稿发到 GitHub Issue 前仍需人工审阅。

## Nginx 反向代理（SSL）

```bash
# 1. 安装 certbot + nginx
ssh user@server "apt install -y nginx certbot python3-certbot-nginx"

# 2. 配置 nginx
scp deploy/nginx/reames-agent.conf user@server:/etc/nginx/sites-available/
ssh user@server "ln -s /etc/nginx/sites-available/reames-agent.conf /etc/nginx/sites-enabled/"

# 3. 获取 SSL 证书
ssh user@server "certbot --nginx -d agent.example.com"

# 4. 重启
ssh user@server "systemctl restart nginx"
```

## SSH 远程使用

部署完成后可通过 SSH 直接调用：

```bash
ssh user@server "reames-agent run '修复 src/auth.go 的空指针问题'"
echo "审查这个 PR" | ssh user@server "reames-agent run"
```

## IM 通道连接（前台调试与后台常驻）

```bash
# 前台启动飞书 gateway，适合调试或 tmux
reames-agent gateway run --home "$REAMES_AGENT_HOME" --channels feishu

# 前台启动微信 gateway
reames-agent gateway run --home "$REAMES_AGENT_HOME" --channels weixin

# 前台启动多个平台
reames-agent gateway run --home "$REAMES_AGENT_HOME" --channels feishu,weixin,qq

# 只读诊断配置、凭据 env、访问控制和连接记录
reames-agent gateway doctor --deep --home "$REAMES_AGENT_HOME"

# 后台常驻服务，先 dry-run 审阅计划
reames-agent gateway install --dry-run --home "$REAMES_AGENT_HOME" --channels feishu --dir /srv/project
reames-agent gateway install --start-now --home "$REAMES_AGENT_HOME" --channels feishu --dir /srv/project
reames-agent gateway status
```

`reames-agent bot start` 仍可作为旧命名兼容入口。推荐形态是：SSH/CLI/TUI 用于交互式任务，`gateway run` 用于前台调试，`gateway install/start/status` 管理独立后台 Gateway service。这样社交通道进程不会占用或阻塞用户 CLI、Desktop 或可选的 `serve` 入口。

WSL2 已完成登录会话内的 credential-free systemd user service 实启、同名重装、status/restart/stop/start、journal、webhook readiness 和卸载验证，但测试用户 `Linger=no`。当前仍需在干净 Linux 服务器上以 linger-enabled 低权限用户补 logout/reboot 常驻、真实 provider key、feedback 运维产物、备份恢复和升级回滚演练，再发送一次真实渠道消息并完成 `/status` 或 `/current` 往返。credential-free smoke 不能替代这些外部证据。
