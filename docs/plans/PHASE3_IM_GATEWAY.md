# Phase 3: IM 通道扩展 — Hermes 风格 Gateway

> 状态：⏳ 待执行
> 参考：F:\code-reference\Hermes（20+ 平台 Gateway 架构）

## 3.1 通用 Gateway 接口

### 改造步骤

1. **创建 `internal/gateway/`**
   - `platform.go` — `Platform` 接口：
     ```go
     type Platform interface {
         Name() string
         Start(ctx context.Context) error
         Stop() error
         Send(ctx context.Context, msg Message) error
         Capabilities() Capabilities
     }
     ```
   - `gateway.go` — 统一网关：平台注册表、消息路由、会话配对、入站/出站队列、速率限制
   - `router.go` — 消息路由规则：文本→Agent turn，卡片按钮→审批回复，文件→artifact

2. **消息格式统一**
   - `Message` struct：平台无关的消息格式
   - 富内容支持：文本、图片、文件、卡片、审批按钮
   - 回复线程：`reply_to` 字段

### 验收标准

| # | 验收项 | 方法 |
|---|---|---|
| A1 | Platform 接口可注册/注销 | 单元测试 |
| A2 | 消息路由正确 | 文本消息路由到 Agent，按钮消息路由到审批 |
| A3 | 会话配对正确 | 同一 IM 用户始终对同一 Agent session |

## 3.2 平台适配器 — Phase A（重构现有）

### 改造步骤

1. **飞书** — 从 `internal/bot/feishu/` 重构
   - 保留 Lark SDK 集成
   - 卡片消息、审批按钮双向同步
   - 进度通知推送

2. **微信** — 从 `internal/bot/weixin/` 重构
   - 公众号消息接收和回复
   - 文本命令解析

3. **QQ** — 从 `internal/bot/qq/` 重构
   - WebSocket 网关连接
   - 消息收发

### 验收标准

| # | 验收项 | 方法 |
|---|---|---|
| A1 | 飞书消息收发 | 发送消息→Agent 回复→飞书收到回复 |
| A2 | 飞书审批卡片 | Agent 请求审批→飞书卡片→用户点击→Agent 收到决定 |
| A3 | 微信文本命令 | `/status` → Agent 返回状态 |
| A4 | QQ 消息收发 | 同飞书 |

## 3.3 平台适配器 — Phase B（高优先级新增）

### 改造步骤

1. **Telegram** — Go Telegram Bot API
2. **Discord** — DiscordGo
3. **Slack** — Slack Go SDK
4. **DingTalk** — 钉钉开放平台 SDK
5. **WeCom** — 企业微信 SDK

每个适配器实现相同的 `Platform` 接口。

### 验收标准

| # | 验收项 | 方法 |
|---|---|---|
| A1 | 每个平台收发正常 | 发送消息→Agent 回复→平台收到回复 |
| A2 | 每个平台富内容 | 图片/文件/卡片均可收发 |
| A3 | 启动命令正确 | `reames-agent gateway start --channels telegram,discord` |

## 3.4 安全门控

### 改造步骤

1. **入站安全**
   - 每条入站消息携带：平台名 + 用户 ID + 信任等级
   - 信任等级：verified / known / unknown
   - unknown 用户需配对确认后才能交互

2. **出站安全**
   - 文件写入/bash 等敏感操作需额外审批
   - 速率限制：每平台每用户 QPS 上限
   - 内容过滤：敏感信息脱敏

### 验收标准

| # | 验收项 | 方法 |
|---|---|---|
| A1 | unknown 用户被拦截 | IM 新用户发消息 → 要求配对 |
| A2 | 敏感操作加审批 | Agent 执行 `write_file` → IM 收到审批卡片 |
| A3 | 速率限制生效 | 短时间内大量消息 → 429 限流 |

## 总回归检查

```bash
go build ./...                                     # 编译
go test ./internal/gateway/... -count=1            # Gateway 测试
go test ./internal/bot/... -count=1                # Bot 测试（迁移后）
go test ./internal/... -count=1 -timeout 300s      # 全量
grep -rn 'reasonix' --include='*.go' -l | grep -v 'reames-agent' | wc -l  # 应为 0
```
