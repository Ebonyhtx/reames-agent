# App-Server 集成

`reames-agent app-server` 通过 stdin/stdout 上的逐行 JSON，把 Reames Agent 暴露给本地编辑器或自动化
客户端。wire 采用 Codex-class App-Server 形状，但只实现本文明确列出的较小能力面，不打开网络监听端口。

## 启动

```bash
reames-agent app-server
reames-agent app-server --model openai/gpt-5.6-sol --profile delivery
```

`--listen stdio://` 是唯一支持的 endpoint，`--stdio` 是别名。诊断只写 stderr，stdout 只写协议帧。
每帧必须是单个 JSON object，最大 8 MiB；wire 不带 `jsonrpc` 字段。

```json
{"id":1,"method":"initialize","params":{"clientInfo":{"name":"my-editor","version":"1.0"}}}
{"method":"initialized","params":{}}
{"id":2,"method":"thread/start","params":{"cwd":"F:\\project","historyMode":"legacy"}}
```

`initialize` 成功后连接即可使用；惯例中的 `initialized` notification 会作为确认被接受。重复 initialize
会失败。initialize 本身不加载 Provider 凭据；创建或恢复 thread 时才装配普通 Reames runtime，因此与其他
前端使用相同配置和凭据要求。

## 方法矩阵

| 方向 | 方法 | 状态 |
|---|---|---|
| client -> server | `initialize`、`initialized` | 支持 |
| client -> server | `thread/start`、`thread/resume` | 只支持持久化 legacy history |
| client -> server | `thread/list`、`thread/loaded/list`、`thread/read` | 支持有界 cursor 分页 |
| client -> server | `thread/name/set`、`thread/unsubscribe` | 支持 |
| client -> server | `turn/start`、`turn/steer`、`turn/interrupt` | 只支持文本；每 thread 最多一个 active turn |
| server -> client | `thread/started`、thread/turn/item 状态与文本/推理 delta | 支持 live 投影 |
| server -> client | command/file approval、`item/tool/requestUserInput` | 支持 server request |
| client -> server | fork/archive/unarchive/rollback/compact/review/settings/dynamic-tool | 不支持 |
| transport | WebSocket / TCP / HTTP | 不支持 |
| content | image、local image、skill、mention、audio、realtime 输入输出 | 不支持 |
| history | `historyMode: "paginated"` | 在装配 runtime 前拒绝 |

未知字段和 enum 会被拒绝，不会静默忽略。尤其是尚未支持的 sandbox、approval、模型 runtime、MCP 或
environment override 不会悄悄退回默认值。

打开 thread 的响应会保守报告实际 shell posture：未隔离 shell 是 `dangerFullAccess`；enforce 模式才报告
`workspaceWrite`、配置后的 write roots 与 sandbox network access。

`thread/list` 沿用 Reames 会话历史语义：新 thread 至少产生一个 user turn 后才进入列表；
`thread/loaded/list` 会包含仍在内存中的空 thread。

## Runtime 与安全边界

每个 thread 都由 CLI、Serve、Desktop 共用的 `control.Controller` 驱动，App-Server 不增加第二套 Agent
loop。session writer lease 防止两个本地 runtime 同时恢复并写入同一 thread。`turn/steer`、
`turn/interrupt`、最终结算和订阅决策同时绑定 primary thread ID 与 turn ID，因此 child agent 活动不能
完成 parent turn。

工具审批与 Ask 使用 server-initiated request。断线、取消、无效或缺失响应均保守处理。需要 fresh-human
决策的工具不能取得持久 session grant；sandbox escape 保留现有、显式的 session 决策。Controller 原有的
permission、sandbox、Hook、checkpoint 和 evidence 规则继续生效。

replay 只从 canonical Reames transcript 重建；raw Provider item、tool progress、process output delta 和
realtime media 不写入第二份 replay log。版本化 0600 sidecar 在冲突恢复切换 active transcript 后保持
App-Server thread ID 稳定；metadata 与 writer lease 更新失败时一起回滚。

这只是首批纵向闭环，不代表完整 Codex App-Server parity。剩余 P9 工作以
[发展计划](DEVELOPMENT_PLAN.md#p9codex-class-extensibility-与-headless-协议)为准。
