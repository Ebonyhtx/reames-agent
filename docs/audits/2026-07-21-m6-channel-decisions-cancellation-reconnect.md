# M6 渠道审批、取消与重连审计

日期：2026-07-21

## 范围

本批只关闭可以由仓库代码、确定性 fixture 和本地 HTTP/SDK harness 证明的 M6
Gateway 缺口：远端审批/问答身份、`/stop` 取消事务、四渠道连接状态投影和在线
重连恢复。真实飞书、QQ、微信、Telegram 应用、凭据、平台保留窗口和公网回环
继续保持 `external-blocked`。

## 决策身份与 ACK

- Controller 对未知、重复或过期 approval/ask 返回稳定 `not_found`，不再静默假 ACK；
  原有 `Approve`/`AnswerQuestion` 兼容入口保留。
- Gateway 为每次进程生成新的随机 epoch，远端 card/文本只携带一次性
  `approval-<epoch>-<sequence>` 或 `ask-<epoch>-<sequence>` token。token 映射到
  Controller 内部 ID，成功或过期后立即消费，Gateway 重启不会复用旧 token。
- 远端 token 不进入 Provider prompt、账本、日志或状态接口。重复点击得到过期提示，
  不会再次触发审批或回答。

## 取消事务

- `/stop` 取消当前 Controller turn，并清除 active session 的等待队列；`/new`、`/reset`、
  `/use`、`/attach` 使用同一被取消 claim 结算路径。
- ACK 成功时，旧 active/pending claims 先 durable 标记为 delivered，当前命令 claim
  由外层 inbound dispatcher 只结算一次；所有 constituent cursor 按连续 sequence
  推进。这样不会因为命令内部和外层重复结算而重复提交平台 offset/ACK。
- ACK 失败时，旧 claims 和当前命令都保持 failed/retryable，checkpoint 不推进；下一次
  claim 可以重新接管同一消息。settlement adapter 收到的每个 message identity 在成功或失败
  路径都恰好一次。
- ACK 已成功但 canceled-session durable commit 失败时，错误会传播给外层，当前命令不会被误标
  delivered；账本恢复并重启后旧任务与命令都可重新 claim，避免“用户看到停止、旧任务却静默复活”。
- Feishu、QQ、微信、Telegram 使用同一 table-driven Gateway 合同 fixture，覆盖 active
  cancel、queued clear、成功连续 checkpoint、ACK/commit 失败可 retry 和无 duplicate settlement。

## 连接状态与 watchdog

- 四个 first-party adapter 统一上报 `connecting`、`running`、`reconnecting`、`closed`，
  reason 只允许固定 host-defined 标识。未知 reason 在 Gateway 边界收敛为
  `connection_error`，不会投影原始 URL、SDK error 或 credential。
- Gateway `/status` 增加 bounded connection state 和 reconnect count；`/metrics` 增加
  `reamesAgent_bot_adapter_reconnects_total`。`reconnecting`/`connecting` 不算 watchdog
  active，恢复 `running` 后重新健康。
- CLI watchdog 在不健康期间停止发送 heartbeat 但保持 ticker，适配器恢复后继续喂
  `WATCHDOG=1`；Gateway 拒绝重复 `Start`，避免重复 connection watcher。
- 飞书 webhook 在报告 `running` 前同步完成 `net.Listen`；端口占用直接返回启动错误，
  不制造虚假的 ready 状态。
- QQ 在拥有完整 `session_id + seq` 时使用官方 `Resume`，收到 `RESUMED` 后恢复 running；
  `INVALID_SESSION` 清空状态并回落 Identify。该证据只覆盖在线 WebSocket session resume，
  不声明关机期间历史补扫。

## 自动证据

相关测试：

- `internal/control/command_dto_test.go`
- `internal/bot/decision_token_test.go`
- `internal/bot/cancellation_contract_test.go`
- `internal/bot/connection_state_test.go`
- `internal/bot/feishu/feishu_test.go`
- `internal/bot/qq/gateway_test.go`
- `internal/bot/telegram/telegram_test.go`
- `internal/bot/weixin/weixin_test.go`
- `internal/cli/bot_test.go`

提交前已通过 Root build/vet/完整 internal tests、Desktop build/vet/完整 tests、Frontend
`test:all`/production build/bundle budget、相关包 race、关键故障路径 `-count=20`、155 项 scripts、
`verify-baseline.ps1`、credential-free Gateway smoke，以及 CLI/Guard 的 12 个 `CGO_ENABLED=0`
交叉目标。仍需以正式 commit 后的 clean clone 和该最终 HEAD 的远端 CI/CodeQL 闭环；本地 fixture
不替代真实 IM 证据。

首个 push 的 CI 其余七个 job 全部通过，但 Windows `Desktop Go` 在无断言失败时被 workflow 的
`go test` 全局 300 秒闸门终止；日志显示总套件到 `300.068s`，本地与 clean clone 同套分别约
174–194 秒。修复只把该 job 对齐到项目正式 600 秒门槛，不跳过测试或放宽任何断言；最终完成声明
仍以修复 HEAD 自身的 CI/CodeQL 为准。

## 未关闭边界

- 真实四渠道文本、审批、取消、掉线、重连、平台保留窗口和进程/节点重启；
- 飞书/QQ 历史分页或离线 resume API；QQ Resume 不能证明关机期间补扫；
- 真实 systemd watchdog kill/restart、macOS/Windows service 节点 lifecycle；
- 平台 ACK 与本地 durable commit 之间的 at-least-once 歧义，仍可能保守重发。
