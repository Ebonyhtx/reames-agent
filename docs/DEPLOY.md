# Reames Agent — 部署指南

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

## IM 通道连接

```bash
# 启动飞书 bot
reames-agent bot start --channels feishu

# 启动微信 bot
reames-agent bot start --channels weixin

# 启动多个平台
reames-agent bot start --channels feishu,weixin,qq
```
