# Reames Agent — 部署指南

## 推荐形态：CLI + 独立 Gateway

如果目标是“像 Hermes 一样把 Agent 部署到云服务器，然后随时 SSH 上去使用，同时飞书/微信/QQ/Telegram 在后台常驻”，部署形态应分成两个互不干扰的入口：

1. **CLI/TUI**：用户 SSH 到服务器后直接运行 `reames-agent` 或 `reames-agent run`。
2. **Gateway service**：后台服务独立接收社交通道消息，前台调试命令是 `reames-agent gateway run --channels feishu`（`reames-agent bot start --channels feishu` 保持兼容），后台服务生命周期命令是 `reames-agent gateway install/start/status`。

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

`<Reames Agent home>/.env` 是 provider key 的运行时来源。不要把真实 key 写入项目仓库、项目 `.env`、systemd unit 或 shell 历史可见的位置。长期使用时，可以把 `export REAMES_AGENT_HOME="$HOME/.reames-agent"` 写入该用户的 shell profile。

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
reames-agent gateway run --channels feishu
reames-agent bot start --channels feishu
```

它适合调试、tmux 或临时运行；`bot start` 是兼容入口，长期推荐使用 `gateway run`。后台服务管理面当前已提供用户级 systemd / launchd / Windows Scheduled Task 命令；生产部署前建议先用 `--dry-run` 审阅计划：

```bash
reames-agent gateway run                    # 前台运行，适合调试/Docker/Termux
reames-agent gateway doctor --deep          # 只读检查配置、凭据 env、访问控制和连接记录
reames-agent gateway install --dry-run --home "$REAMES_AGENT_HOME" --channels feishu --dir /srv/project
reames-agent gateway install --start-now --home "$REAMES_AGENT_HOME"    # 安装并启动后台服务
reames-agent gateway status                 # 查看后台服务和平台连接状态
reames-agent gateway restart                # 重启服务，不影响用户 CLI 终端
reames-agent gateway uninstall              # 卸载后台服务
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
- 日志写入 Reames Agent home；
- 与 CLI、Desktop、serve 共用配置、凭据、会话和权限模型，但进程互相隔离。

实现前的临时服务器方案：

```bash
sudo -iu reames
tmux new -s reames-gateway
REAMES_AGENT_HOME="$HOME/.reames-agent" reames-agent gateway run --channels feishu
```

正式命令会按平台选择 service manager：Linux 使用 user systemd，macOS 使用 launchd，Windows 使用 Scheduled Task。`--scope system` 只渲染计划并要求管理员/root 手动确认，避免误改系统级服务。

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
reames-agent gateway run --channels feishu

# 前台启动微信 gateway
reames-agent gateway run --channels weixin

# 前台启动多个平台
reames-agent gateway run --channels feishu,weixin,qq

# 只读诊断配置、凭据 env、访问控制和连接记录
reames-agent gateway doctor --deep

# 后台常驻服务，先 dry-run 审阅计划
reames-agent gateway install --dry-run --home "$REAMES_AGENT_HOME" --channels feishu --dir /srv/project
reames-agent gateway install --start-now --home "$REAMES_AGENT_HOME" --channels feishu --dir /srv/project
reames-agent gateway status
```

`reames-agent bot start` 仍可作为旧命名兼容入口。推荐形态是：SSH/CLI/TUI 用于交互式任务，`gateway run` 用于前台调试，`gateway install/start/status` 管理独立后台 Gateway service。这样社交通道进程不会占用或阻塞用户 CLI、Desktop 或可选的 `serve` 入口。

当前仍需在干净 Linux 服务器上补一次完整实战验证：安装二进制、配置 `REAMES_AGENT_HOME` 与真实 provider key、安装 gateway service、检查日志、发送一次真实渠道消息并完成 `/status` 或 `/current` 往返。
