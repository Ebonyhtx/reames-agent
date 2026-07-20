# M6 微信持久轮询与 Desktop 背压审计

日期：2026-07-20

## 范围

本批只关闭无需真实 IM 凭据即可由 localhost、持久状态和故障注入证明的缺口：

- 微信 iLink `get_updates_buf` 的最终投递结算与跨进程恢复；
- 飞书、QQ、微信入站队列饱和时的 envelope 保留；
- Desktop Wails live event queue 的 delta 合并与有界可取消背压。

飞书/QQ 完全离线期间的真实历史分页、真实微信/Telegram 远端保留窗口、审批/取消公网回环和节点重启
仍需要平台应用、凭据与真实节点，继续标记 `external-blocked`。

## 修复前风险

1. 微信适配器在 `getupdates` HTTP 响应到达时立即替换内存 `syncBuf`。如果 Agent、最终回复或本地账本提交
   随后失败，同一进程下一次轮询仍会越过该批消息。
2. 飞书、QQ 和微信均使用 64 条 channel，但生产路径采用 `select default`；恢复扫描或 Gateway 启动较慢时，
   第 65 条实时事件会在 durable claim 之前静默丢失。
3. Desktop `asyncRuntimeEmitter` 在 WebView/Wails bridge 阻塞时无限追加 slice。长工具输出或 token stream
   会把 UI 卡顿扩大为无界 Go heap 增长。

## 采用机制

### 微信 iLink

- `get_updates_buf` 现在是 adapter-owned durable cursor，不再在 HTTP receipt 时提交；
- 每批有效 inbound message 等待 Gateway 的 `DeliverySettlementAdapter.SettleInbound`；只有所有消息的最终
  回复已发送且 delivery ledger 已提交，才原子写入新的 poll state；
- failed settlement、进程取消或 poll-state 写入失败均保留旧 buffer，下一次请求重放同一批；
- `<Reames Agent user data>/weixin/accounts/<account>.poll-state.json` 使用 schema version、0600 权限和
  `fileutil.AtomicWriteFile`；损坏或未知 schema 在 adapter startup fail closed；
- poll state 只保存 opaque buffer 和最后 update ID，不保存入站正文、用户 ID、附件、模型内容或凭据；
- 手工配置的 `account_id` 若包含路径分隔符、控制字符、Windows device name 或危险尾缀，会稳定映射为
  digest 文件名；账户、context token 与 poll state 三类文件都被限制在 `weixin/accounts`；
- 空轮询且 cursor 未变化时不重复写盘。

微信使用平台原生 long-poll cursor，因此不伪装成另一个历史分页 `RecoveryAdapter`。飞书/QQ 在没有已证明的
平台 resume/history API 前也不实现假的补扫。

### 渠道 envelope 背压

飞书 SDK/webhook、QQ Gateway 和微信 poll publisher 在 64 条队列满时改为等待 Gateway 消费；服务取消会
解除等待。飞书若在 claim 前取消还会回滚内存 event-id 去重，允许平台重试。这样网络回调、WebSocket reader
或 long poll 自然向平台施加背压，不会在 durable claim 前静默丢消息。

### Desktop live event queue

- active backlog 默认最多 2048 个 envelope；
- 同 tab 相邻 `text`、`reasoning` 和同 tool 的 `tool_progress` 在 Go 侧合并；重复的
  `project-tree:changed` 也会合并；
- 队列达到上限后对 producer 施加可取消背压，不丢 `text`、`reasoning`、`tool_progress`，因为异常中断时
  部分回答/工具输出不一定立刻获得完整 Message/ToolResult 回填；
- `TurnDone`、完整 Message/ToolResult、approval、Ask、usage、notice 和生命周期事件同样保持原顺序；
- `Clear` 推进 emitter generation 并广播空间变化；旧 tab/context 的等待 producer 被唤醒后退出，不能把旧
  事件重新排入新代队列。worker dequeue 同样广播空间，避免遗留阻塞 producer。

## 自动证据

- 微信 localhost fixture：最终结算前不发下一次请求、不写 cursor；failed settlement 和磁盘写失败均从旧
  buffer 重放；成功结算后跨 adapter restart 使用已提交 buffer；损坏 state fail closed；
- 飞书、QQ、微信 fixture：队列饱和时 publisher 等待而非返回/drop，消费或 context cancellation 可解除；
- Desktop：delta 合并、2048/自定义小上限、满队列 producer 背压与恢复、cancellation、Clear generation、
  零语义 drop 和顺序 drain；
- Windows amd64 微基准：`BenchmarkAsyncRuntimeEmitterCoalescedBacklog` 每轮 10,000 次、独立 5 轮为
  1.262–1.529 µs/op，中位数约 1.3 µs/op
  （设备相关，只证明实现没有明显锁级退化，不冒充 UI frame pacing）。

完整交付仍以本批最终 root/Desktop/frontend 全量、race、六目标构建、clean clone 和 push 对应 CI/CodeQL
为准。

## 未关闭边界

- 真实微信 iLink 账号的离线窗口、token 过期、平台 cursor 失效和群聊行为；
- 飞书/QQ 平台历史/resume API 与断电期间漏消息补偿；
- 各渠道真实审批、取消、掉线、重连、最终回复和节点重启回环；
- Desktop 原生 WebView 长时间阻塞下的真实 frame pacing/heap profile。
