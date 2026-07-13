# Reames Agent 威胁模型

> 状态：描述当前代码边界，不把路线图当作已实现能力
> 更新：2026-07-13

## 范围与假设

Reames Agent 是单用户本地/自托管 Agent。模型输出、网页/工具返回、IM 消息和插件内容均视为不可信；本机用户显式安装的 MCP、Hook、LSP 和 shell 命令属于高权限扩展。`yolo`、关闭 sandbox、`serve.auth = "none"` 或 `allow_all` 都会主动降低防护，不能作为默认安全证据。

主要边界：

```text
本机用户 / Desktop / CLI
          │
          ▼
Controller ── Agent loop ── Provider API
    │              │
    │              ├─ built-in tools / shell sandbox
    │              └─ MCP / Hook / LSP 子进程
    ├─ session / checkpoint / credential state
    └─ Serve / Gateway ── 浏览器或 IM 用户
```

## 控制现状

| 领域 | 状态 | 当前实现 | 尚未覆盖或限制 |
|---|---|---|---|
| Provider 凭据 | 部分实现 | Provider key 名写入配置，值从 Reames Agent 全局 `.env` 或进程环境解析；项目 `.env` 不作为 Provider key 来源；Unix 写入权限收紧为 `0600`；有可选 AES-256-GCM 存储原语 | 进程环境仍是有效来源；加密文件使用机器属性派生密钥，不等同 OS keyring/硬件保护；日志与第三方错误的脱敏是 best-effort，需持续回归 |
| 工具审批 | 已实现核心路径 | `internal/permission` 按工具、subject、只读属性和 allow/ask/deny 规则决策；文件写入审批可携带 diff，拒绝/超时路径有不落盘测试 | `yolo`/显式 allow 会绕过交互；所有新工具、远程入口和扩展必须持续做集成覆盖，不能只依赖工具自报 `readOnly` |
| Shell 隔离 | 部分实现 | `sandbox.mode = enforce` 时使用 Linux bubblewrap、macOS Seatbelt、Windows AppContainer/低完整性 token 与 Job Object；后端不可用时 enforce 模式 fail closed | sandbox 可被配置为 off，零值也不隔离；平台能力不完全等价；MCP、Hook、LSP 不是自动置于同一 shell sandbox 中 |
| 不可信内容 | 部分实现 | 有 untrusted envelope、HTML 文本化和常见 token 正则脱敏；system prompt/tool schema 采用稳定前缀约束 | 不能保证模型不受 prompt injection 影响；正则无法识别所有私有凭据；工具/插件/网页内容仍必须依赖权限边界限制副作用 |
| Prompt/展示数据边界 | 已实现核心路径 | `MessagesForRequest` 在 Provider interface 前剥离 citation/edit/original 等本地 metadata；OpenAI/Anthropic wire-byte 与 Agent cache-prefix 回归覆盖该纪律。`control.TranscriptMessage` 隐藏 system、合成恢复指令、compose 控制块和 referenced-context payload；Serve/ACP history、ACP metadata title、Desktop memory suggestions/history/pagination/planner sidecar 均消费安全投影。Desktop sidecar correlation hash 与安全 replay text 标记为 `json:"-"`，不跨远程 transport；rebuild 通过 opaque history snapshot 刷新已有 system prompt，同时保留 legacy system-less transcript | 新增 Provider、传输适配器或本地 metadata 字段时仍必须加入剥离/投影合同；模型本来就需要看到的用户正文和工具结果不属于该隐藏边界 |
| HTTP Serve | 部分实现 | 支持 `none`/token/password，token 常量时间比较、密码 session HMAC、登录速率限制、JSON-only POST CSRF guard、默认无 CORS、显式单 origin CORS；版本化 command 校验与服务端 `remote` scope 阻止客户端选择 trusted submit，旧 WS submit 也不再绕过 `!shell` 限制；history 使用展示安全 transcript 而不输出 system/注入上下文；真实 WebSocket 握手有回归测试 | WebSocket `CheckOrigin` 当前放行并依赖外层鉴权；`auth=none` 依赖 loopback/same-origin 部署假设；请求体、WS frame 和全局请求速率限制仍需系统化审计 |
| IM Gateway | 部分实现 | 用户/群 allowlist、admin/approver 角色、operator 身份检查和各渠道传输适配已存在；connection/domain/chat/user/operator/message ID 只用于路由，不进入 Provider prompt。`gateway setup` 只接受常规大写 secret 环境变量名，新连接必须显式 pairing/名单/角色或有意 `allow_all`，损坏配置与缺失 access 均 fail closed；dry-run 不落盘，正式更新原子且幂等。Linux systemd user service 已用随机 token 的 loopback webhook challenge 验证 install/reinstall/restart/stop/start/uninstall，token 不进入 unit、命令输出或报告；Linux user-scope install 还会验证 unit、快照旧定义与 enabled/active 状态，并在定义恢复失败时停止后续 manager 操作 | CLI 无法从字符串本身证明一个大写值绝不是误传 secret，运维仍须使用命名明确的环境变量；当前没有通用飞书 webhook HMAC/重放验证实现；install 事务不覆盖 system scope、macOS launchd、Windows Scheduled Task 或 uninstall；WSL 证据为 `Linger=no` 的登录会话内生命周期，不证明 logout/reboot 常驻；真实飞书/QQ/微信回环需要外部应用凭据与网络环境，未验证前不得声明完成 |
| 插件与 Hook | 部分实现 | 插件路径/名称/manifest 基础校验、启停状态、MCP 启动/调用超时、项目 Hook trust gate 和 Hook 超时已存在 | manifest 尚无被安装器执行的权限声明、兼容版本、内容哈希或签名验证；“用户安装即信任”仍是主要边界 |
| 状态与恢复 | 部分实现 | session JSONL、lease/recovery、checkpoint/rewind、版本化 Goal sidecar 和 Todo 恢复均有测试；CLI/Bot/Serve/ACP/Desktop 的列表/恢复/跨进程 lease/cleanup/trash/recovery GC 通过 control persistence 边界复用同一语义。`backup create/verify/restore` 支持 home/state 分根、已知凭据排除、逐文件哈希、源文件身份复验、portable path 拒绝、仅新目标恢复和后续根失败时的进程内回滚 | 备份仍可能含 session/memory/config 中的 secret；内嵌 hash 只证明自洽；`--offline` 是人工确认而非全局锁；Windows 保护依赖目标目录 ACL；跨根恢复没有 durable crash journal，强杀后可能留下 staging/空 parent；并非所有 sidecar 都使用同一种原子写协议 |
| 构建与发布 | 部分实现 | Go 依赖哈希、六目标 candidate、SHA256SUMS、三平台 Desktop candidate、CodeQL 和发布契约检查已建立；CLI updater 已锁定官方仓库和精确资产名，实际执行候选/安装后 `version`，保留 `.previous`，并以同目录锁保护自动恢复和 `upgrade --rollback` | updater 事务没有 durable crash journal；生产发布仍禁用；公开 release 实际升级/失败回滚、CLI/Windows/macOS 工件签名、notarization、provenance attestation 和可信 updater 发布链未完成 |

## 优先风险

1. **远程副作用**：Serve/IM 一旦暴露到非 loopback，鉴权、Origin、CSRF、角色与审批必须同时成立。
2. **扩展供应链**：插件、MCP、Hook 和 LSP 能启动本机进程；在签名和权限 manifest 完成前，只应安装可审计来源。
3. **凭据外泄**：Provider/IM token 可能经错误正文、工具输出、日志或第三方扩展泄露；证据脚本不得保存原始 HTTP 错误正文。
4. **沙箱误解**：权限批准不等于 OS 隔离，sandbox 配置为 off 也不等于安全执行。
5. **恢复一致性**：session、Goal、Todo、checkpoint、lease、备份根和版本二进制跨文件更新时必须防止旧状态复活或终态丢失；进程内回滚不得冒充断电原子性。

## 外部阻塞与可本地推进

没有真实 API key、IM 应用或云服务器时，仍可使用 localhost Provider harness、假凭据、隔离 home、原生安装包和本地反向代理完成确定性合同与失败路径。以下证据必须保持 `external-blocked`，不能用 mock 替代完成声明：

- 真实 Provider 的鉴权、计费/用量和供应商网络行为；
- 真实 IM 平台的身份、回调/WebSocket、审批与重连回环；
- 公网 TLS、反向代理、安全组、linger-enabled logout/reboot 常驻，以及干净云节点上的备份/恢复和公开签名 release 升级/回滚实启；
- Windows/macOS 代码签名、Apple notarization、OIDC provenance 与公开 updater 链。

漏洞报告流程和支持边界见 [SECURITY.md](../SECURITY.md)。发布启用门槛见 [RELEASING.md](RELEASING.md)。
