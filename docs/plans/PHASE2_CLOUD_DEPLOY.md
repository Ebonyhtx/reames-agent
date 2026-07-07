# Phase 2: 云服务器部署能力

> 状态：⏳ 待执行
> 目标：Agent 可在云服务器运行，通过 SSH/HTTP/IM 交互，支持 Docker 部署

## 2.1 serve 增强 — 多会话 + WebSocket

### 当前状态
`internal/serve/` 提供 HTTP/SSE 服务，1 server = 1 session，嵌入式 index.html。

### 改造步骤

1. **多会话路由**
   - 新增 `GET /api/sessions` — 列出所有可用会话
   - 新增 `POST /api/sessions` — 创建新会话并返回 session-id
   - URL 路由：`/session/:id` 对应不同会话的 SSE 流
   - Session pool 管理：`SessionPool` struct，LRU 淘汰

2. **WebSocket 升级**
   - 新增 `GET /ws/:id` — WebSocket 端点
   - 双向通信：客户端发送 JSON-RPC，服务端推送事件流
   - 自动回退到 SSE（兼容旧客户端）

3. **健康检查和就绪探针**
   - `GET /health` — 返回 200 + `{"status":"ok"}`
   - `GET /ready` — 等待 provider 连接成功后返回 200

4. **认证增强**
   - 已有 token/password 两种模式，保持不变
   - 新增 `X-API-Key` header 支持（简化 CI/脚本调用）

### 验收标准

| # | 验收项 | 方法 |
|---|---|---|
| A1 | `reames-agent serve` 启动成功 | 浏览器访问 `localhost:8787` 看到 UI |
| A2 | 多会话创建和切换 | 创建 3 个会话，每个独立 SSE 流 |
| A3 | WebSocket 连接 | wscat 连接 `ws://localhost:8787/ws/:id` |
| A4 | `/health` 返回 200 | `curl localhost:8787/health` |
| A5 | token 认证拒绝未授权 | `curl localhost:8787/api/sessions` → 401 |

### 回归测试

```bash
go test ./internal/serve/... -count=1 -v
# 验证：所有现有测试通过，新增测试覆盖多会话/WS/health
```

## 2.2 Docker 化

### 改造步骤

1. **Dockerfile**
   ```dockerfile
   FROM golang:1.25 AS builder
   WORKDIR /src
   COPY . .
   RUN CGO_ENABLED=0 go build -o /reames-agent ./cmd/reames-agent

   FROM gcr.io/distroless/static
   COPY --from=builder /reames-agent /reames-agent
   EXPOSE 8787
   ENTRYPOINT ["/reames-agent", "serve", "--addr", "0.0.0.0:8787"]
   ```

2. **docker-compose.yml**
   - reames-agent 服务 + volume 挂载 `~/.reames-agent/`
   - 可选 PostgreSQL 服务（未来多用户持久化）

3. **docker-compose.prod.yml**
   - 生产配置：`restart: always`、日志驱动、资源限制

### 验收标准

| # | 验收项 | 方法 |
|---|---|---|
| A1 | Docker 构建成功 | `docker build -t reames-agent .` |
| A2 | 容器启动并响应 | `docker run -p 8787:8787 -e DEEPSEEK_API_KEY=xxx reames-agent` |
| A3 | API Key 通过环境变量注入 | 容器内 `curl localhost:8787/health` → 200 |
| A4 | Volume 持久化配置 | 重启容器后 session 仍存在 |

## 2.3 部署清单

### 改造步骤

1. **systemd 服务**
   - `deploy/systemd/reames-agent.service`
   - `Restart=always`、`User=reames`、环境变量注入

2. **Nginx 反向代理**
   - `deploy/nginx/reames-agent.conf`
   - SSL 终端、WebSocket 升级、速率限制

3. **K8s 清单（可选）**
   - `deploy/k8s/deployment.yaml`、`service.yaml`、`configmap.yaml`

### 验收标准

| # | 验收项 | 方法 |
|---|---|---|
| A1 | systemd 服务正常启停 | `systemctl start/stop reames-agent` |
| A2 | Nginx 反向代理 SSL | `https://server-ip` 正常访问 |
| A3 | SSH 无交互运行 | `ssh server 'reames-agent run "echo hello"'` |

## 总回归检查

```bash
# Phase 2 完成后运行
go build ./...                                    # 编译
go test ./internal/... -count=1 -timeout 300s     # 全量测试
docker build -t reames-agent .                    # Docker 构建
docker run --rm reames-agent serve --help          # 验证二进制
```
