# M2 版本化命令控制面审计

日期：2026-07-11

## 结论

本批关闭 M2 的版本化 command DTO 纵向路径，但不声明 M2 完成。`control.Command` / `CommandResult` 已稳定表达异步提交、取消、审批和状态查询；Desktop、CLI、Serve、Bot、ACP 的对应生产入口通过 `CommandControl` 执行。剩余 event/展示 DTO、会话、装配、设置边界和 prompt metadata/cache 前缀仍按路线图继续。

## 合同

- envelope 固定 `version=1`，kind 为 `submit`、`cancel`、`approval`、`status`；submit 与 approval 使用判别 payload，cancel/status 禁止 payload。
- `CommandError` 使用 `invalid_version`、`invalid_kind`、`invalid_payload`、`forbidden`、`busy` 稳定 code，并带可选 field；无效命令在运行时副作用前拒绝，并发第二个 submit 以 HTTP 409 明确拒绝而不是静默丢弃后伪报 accepted。
- `CommandScope` 由 Go 传输适配器选择，不进入 JSON：Serve/Bot/ACP 使用 `remote`，Desktop/CLI 使用 `trusted`，纯 prompt 重试使用 `user_turn`。因此远端客户端不能在请求中把自己提升为 trusted shell/本地 metadata 调用方。
- `CommandResult.Accepted` 是异步 dispatch 的权威确认；附带 `RuntimeStatus` 是 dispatch 后快照，不伪装成 turn 已经完成。
- 同步 `RunTurn` 继续服务拥有 context、等待结果和 turn 生命周期的 CLI/Bot/ACP；没有为了表面统一而把它错误改造成异步确认。

## 传输与兼容

- Serve 新增 `POST /command`，成功和失败都返回结构化 `CommandResult`；未知字段、版本、payload 和远端 transcript metadata 均有合同测试。
- WebSocket 新增 `method=command`。旧 `submit` / `cancel` / `approve` method 和旧 HTTP 端点保留，但全部映射到同一 remote dispatcher。
- 旧 WebSocket `submit` 此前直接调用 trusted `Submit`，可让 `!cmd` 绕过 HTTP 的 shell 禁止；现在与 HTTP 使用相同 remote policy，并有“runner 未收到 shell”的真实连接测试。
- 新测试首次执行真实 `/ws` 握手时发现日志 `responseWriter` 隐藏 `http.Hijacker`，导致公开路由实际返回 500。本批转发 `Hijack` 后以 Gorilla WebSocket 对 `httptest.Server` 完成真实握手、status、legacy 拒绝和 versioned submit 回环。

## 前端迁移

- Desktop bound submit/display/edit/user-turn、Cancel、Approve 和常规 runtime status 读取使用 command port；工作区只读、绑定、topic index 等前端职责保持不变。
- CLI 的取消、审批与运行/取消状态读取使用 command port。
- Bot 的远端取消、审批与运行状态读取使用 remote command scope。
- ACP 的审批回调与 config rebuild 状态检查使用 remote command scope；ACP `session/cancel` 仍取消 service 持有的同步 turn context，这是协议拥有的生命周期而非绕过。
- Serve 的 legacy 与 versioned HTTP/WS 入口共用 dispatcher；`/model`、`/effort` 仍由持有 controller rebuild 权限的 Server adapter 原子执行。

## 自动化证据

新增或扩展的关键覆盖：

- `internal/control/command_dto_test.go`：精确 JSON、所有 scope 路由、四类操作、结构化错误和无副作用拒绝。
- `internal/serve/serve_test.go`：真实 submit → running status → cancel → idle；未知版本/字段、远端 metadata 和 shell 拒绝；真实 WebSocket 握手、旧协议隔离及新协议提交。
- Desktop/Bot 测试桩显式实现新 port，防止嵌入 nil interface 意外掩盖生产调用。

本批提交前门禁全部通过：

```text
go build ./...
go vet ./...
go test ./internal/... -count=1 -timeout 10m
desktop/go test . -count=1                     # 170.678s final rerun
desktop/frontend/corepack pnpm test:all
desktop/frontend/corepack pnpm build
python -m unittest scripts.test_smoke_desktop_interaction scripts.test_smoke_desktop_native scripts.test_check_upstreams -v
node scripts/test_upstream_watch_issue.mjs
go test ./internal/tool -run TestBuiltinToolContractDocumentation -v -count=1
.\scripts\verify-baseline.ps1 -SkipFrontendHint
git diff --check
```

前端 build 继续报告既有的大 chunk/ineffective dynamic import 警告但成功；这属于 M3 性能债务。远端 CI 尚需集中 push 后确认。

## 证据边界与未完成项

本审计证明本地确定性 command 协议、跨入口适配、真实 localhost WebSocket 和已有前端测试回归。它不证明公网 Origin/限流/大 frame 防护、真实 IM、干净云节点、生产签名或剩余 M2 event/会话/装配边界。上述事项继续保持未完成或 `external-blocked`，不得由本批测试替代。
