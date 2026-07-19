# M6 Telegram 持久长轮询与最终投递审计

日期：2026-07-19

## 结论

Telegram 已从占位配置升级为正式 `bot.Adapter`，并接入 CLI Gateway、Desktop
bot runtime、配置、配对、访问控制、路由、诊断、真实测试发送和统一投递账本。
仓库内可以证明的 durable polling 闭环已经关闭；真实 Telegram Bot token、真实
私聊/群聊、审批/取消和公网掉线恢复仍保持 `external-blocked`，没有用测试 token
或 localhost fixture 冒充生产证据。

## 配置与凭据边界

- 正式平台标识为 `bot.PlatformTelegram`，连接使用
  `provider = "telegram"`、`domain = "telegram"`。
- `gateway setup --channel telegram --token-env TELEGRAM_BOT_TOKEN` 只保存环境变量名，
  不接受或持久化 token 值。
- 默认远端为 `https://api.telegram.org`；非 localhost 的 HTTP 地址 fail closed。
- Telegram token env 已进入统一 credential isolation 集合，子进程、Hook、插件和
  日志不会因普通环境继承获得该值。
- adapter 错误、Gateway 账本和测试诊断不会包含 token、带 token 的 URL、底层
  transport error 或原始响应体。
- Desktop 不宣称一键创建 Telegram Bot；连接仍通过 BotFather + CLI/config 创建。
  Desktop 只提供连接展示、token env 编辑、诊断和真实测试发送。

## 长轮询状态机

- 启动时先调用 `getMe` 验证 token 和远端协议。
- 每个 `getUpdates` 请求有 deadline；失败采用有上限的指数退避。
- `Stop` 可取消阻塞轮询；停止后允许重新启动，但并发重复 `Start` 被拒绝。
- Telegram `update_id` 是 durable delivery identity 与单调 sequence；原生
  `message_id` 单独保留用于回复，不混用为去重游标。
- 自发消息先完成 durable claim，claim 持久化失败会触发重试退避，不会永久卡住
  polling loop，也不会热循环占用 CPU/磁盘。

## 最终投递门禁

Telegram 实现 `DeliverySettlementAdapter`：

1. `update_id` 到达后先写入统一 delivery ledger；
2. Agent turn、审批/取消或同步命令处理完成；
3. 最终 `sendMessage` 成功；
4. delivery ledger 的 delivered 状态原子提交；
5. 只有以上全部成功后，下次 `getUpdates` 的 offset 才推进。

如果最终发送失败，offset 保持不变，同一 update 会重投。进程重启后若 ledger 已
记录 delivered，Gateway 不再次运行 Agent，但会确认远端 update，使 Telegram
offset 越过该消息。该语义是 at-least-once；发送分片中途失败时可能重复已发送
分片，系统选择不丢最终答复而不是静默推进游标。

## localhost 纵向证据

真实 HTTP fixture 覆盖：

- 第一次 `sendMessage` 返回 502；
- polling offset 保持 `0`；
- 同一 `update_id=42` 重投；
- 第二次发送成功；
- ledger durable commit 后 offset 推进到 `43`；
- 回复引用原生 `message_id=7`；
- token 未进入 ledger、日志或错误。

相关测试位于：

- `internal/bot/telegram/telegram_test.go`
- `internal/bot/gateway_test.go`
- `internal/botruntime/runtime_test.go`
- `internal/gatewaysetup/setup_test.go`
- `internal/cli/bot_test.go`
- `desktop/bot_runtime_app_test.go`
- `desktop/settings_app_test.go`

## 已执行验证

```text
go test ./internal/bot/telegram
go test ./internal/bot
go test ./internal/botruntime ./internal/config ./internal/gatewaysetup ./internal/cli
cd desktop && go test . -run 'TestProviderModelOverridesPreservePerModelContextWindow|TestBot'
cd desktop/frontend && corepack pnpm test:typecheck
cd desktop/frontend && corepack pnpm exec tsx src/__tests__/provider-model-refresh.test.ts
```

以上聚焦验证均通过。最终交付仍以本批完整 root/Desktop/frontend/race/交叉编译/
clean-clone 和远端 CI/CodeQL 结果为准。

## 未关闭的外部证据

- 真实 Telegram Bot token 和 BotFather 创建流程；
- 真实私聊、群聊、mention、审批、取消、Ask 和媒体回环；
- 公网超时、429、Telegram 服务波动、长时间掉线和进程/节点重启；
- 真实 Gateway service logout/reboot 常驻与 watchdog kill/restart。

这些项目需要真实外部主体和网络环境，继续标记 `external-blocked`。
