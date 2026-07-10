# Reames Agent Threat Model

> 状态：当前安全边界说明
> 更新：2026-07-10

## 1. 信任边界

```text
用户工作站 / 服务器
├─ Reames Agent 进程（单用户、单 home）
│  ├─ Agent Loop（prompt → model → tool → result）
│  ├─ Controller（session、approval、checkpoint）
│  ├─ Plugin Host（MCP 子进程、hook 脚本）
│  └─ Sandbox（OS 级隔离：Windows job object、macOS sandbox、Linux landlock）
├─ Provider API（DeepSeek / Anthropic / OpenAI 等外部服务）
├─ IM Gateway（飞书 WebSocket/Webhook、QQ WS、微信 iLink）
└─ Desktop WebView（Wails WebView2 / WebKitGTK）
```

## 2. 凭据安全 (Credential Security)

- Provider API key 仅存储在 `<Reames Agent home>/.env`，不在 config.toml 中
- 运行时不读取 shell 环境变量、项目 `.env` 或系统 keyring 中的 provider key
- 凭据文件以受限权限写入（Unix 0o600, Windows ACL）
- 日志、错误消息、事件流和诊断输出中屏蔽密钥值
- **external-blocked**：真实签名/notarization 需要代码签名证书

## 3. Prompt 注入 (Prompt Injection)

- 系统 prompt 和 tool schema 在会话期间不可变，走缓存优先
- UI/渠道 metadata 永不注入 provider prompt（缓存前缀回归测试保护）
- 用户输入中的控制标记（`[goal:complete]`、`[goal:blocked]`）仅在结构化解析后生效
- 工具输出在追加到对话历史前进行脱敏
- 项目 `.env` 和 `.reames-agent/` 中的 `${VAR}` 展开受 allowlist 限制

## 4. 工具权限

- 所有工具调用经 `internal/permission` 门控（ask/auto/yolo 模式）
- 文件写入：先审批（含 diff 预览），允许后落盘，拒绝/超时不落盘
- Shell 命令：经 `internal/sandbox` 隔离，超时终止，输出大小限制
- 网络请求：经 `internal/netclient` 代理和超时控制
- 审批模式：ask（每次询问）、auto（会话授权）、yolo（跳过，仅开发用）

## 5. 插件供应链

- 插件 manifest 含 TrustLevel（community/verified/signed）、权限声明、兼容范围
- 安装前预览来源、文件、命令、网络和敏感权限
- MCP 子进程有启动超时、tool timeout、输出大小限制和崩溃隔离
- Hook 脚本在 allowlist 环境和超时限制下执行
- 插件故障不得拖垮主 Agent 进程
- **external-blocked**：签名验证需 minisign/SSH 公钥基础设施

## 6. 远程入口

- Serve（HTTP/SSE）：鉴权（token/oauth）、CSRF、Origin 检查、速率限制、租约
- Gateway service：独立进程，凭据仅通过 `REAMES_AGENT_HOME/.env` 引用
- IM 渠道：飞书 webhook 签名验证（HMAC-SHA256 + 时间戳防重放）、消息去重
- 桌面 WebView：仅绑定 localhost，不暴露远程调试端口
- **external-blocked**：真实飞书/QQ/微信回环需应用凭据和公网回调 URL

## 7. 状态与恢复

- 会话状态序列化到 JSONL，版本化管理，原子写入（tmp + rename）
- Checkpoint 保存文件快照，支持 rewind 恢复
- 崩溃恢复通过租约（lease）和 recovery branch 处理并发冲突
- 临时目录在会话结束时清理
- 记忆数据可解释、可关闭、可删除

## 8. 构建与发布

- CGO_ENABLED=0 单二进制，6 目标交叉编译
- go.sum 锁定依赖哈希
- SHA256SUMS + SBOM（Go module graph）用于工件验证
- CI：go vet、CodeQL（Go + JS/TS + Actions）、契约检查
- **external-blocked**：Sigstore/cosign 签名需 OIDC 身份、Apple notarization 需 Developer ID
