# M6 outbound final-response obligation 审计

> 日期：2026-07-20
>
> 范围：`internal/bot` 最终答复持久化、平台 ACK 歧义、跨重启恢复、单 writer 与隐私投影
>
> 上游信号：Hermes durable outbound delivery obligation；按 Reames 单一 Go/Wails runtime 重构

## 1. 关闭的缺口

旧 Gateway 只有 inbound durable claim。Agent 成功后，render 会直接向平台发送最终文本，再把 inbound
claim 标为 delivered。若进程在平台发送前或多分片发送中崩溃，账本没有可恢复的答复正文；同一 inbound
重试只能重新运行模型，既浪费 token，也可能再次执行工具并生成不同答案。

本批把 delivery ledger 从 schema v1 升级为 v2，在同一原子状态中加入 outbound final-response
obligation。成功 turn 的最终文本必须先落盘，随后才允许调用 Adapter；重复 inbound 若命中已有 obligation，
直接恢复原答复，不创建 Controller、不调用 Provider。

## 2. 状态机与提交顺序

```text
pending
  └─ persist attempting before Send
       ├─ Send failed/unconfirmed -> failed
       └─ platform ACK
            ├─ more chunks -> persist next_chunk + pending
            └─ last chunk -> one atomic ledger write:
                 remove obligation
                 mark every constituent inbound claim delivered
                 advance each contiguous channel checkpoint / Telegram offset
```

- `pending`：没有发生平台发送尝试；冷启动恢复不显示重复警告。
- `attempting`：发送前状态已落盘，但没有 durable ACK commit；平台可能已收到该分片。
- `failed`：Adapter 没有确认投递；远端是否部分接收仍不能可靠证明。
- `attempting`/`failed` 恢复时，第一个重发分片添加：

  ```text
  ♻️ 这是网关重启后恢复的答复；上次发送结果未能确认，因此可能与已收到的内容重复。
  ```

- 已经持久 ACK 的前置分片不会重发，只从 `next_chunk` 继续。
- 这提供可见的 at-least-once 语义，不声称平台 ACK 与本地磁盘 commit 具备分布式 exactly-once。

## 3. 边界与单 writer

- 每个 obligation 最多 1 MiB、512 个纯文本分片；整个 ledger 仍限 4 MiB、默认 4096 records/channels。
- obligation 只接受同一 connection/domain/chat/chat-type/reply target 的最终文本；media、keyboard、card、
  Approval、Ask 和 progress 不进入恢复正文。
- obligation ID 是 source、message ID、全部 constituent claims、target 和 chunks 的 SHA-256 内容身份；
  修改正文、target 或 claims 后重启会 fail closed。
- CLI foreground Gateway 与 Desktop bot 使用同一路径长生命周期 OS 文件锁；第二 writer 启动失败。
- Windows 使用 `LockFileEx`，Unix 使用 non-blocking `flock`；锁文件权限收紧为 0600，Gateway Stop、启动失败
  和 ledger decode/validation 失败都会释放锁。
- Gateway Stop 先阻止新 turn/delivery、取消 Controller，等待 turn 与 obligation send 退出，再释放 ledger 锁。

## 4. 恢复、取消与扫描预算

- 冷启动先按创建时间恢复 obligation，再用剩余预算调用各 Adapter 的 `RecoveryAdapter`。
- obligation 与历史补扫共用默认 200 条全局启动扫描上限，不能各自无限扫描。
- 重复 inbound 命中 obligation 时直接进入恢复发送；同进程 active-obligation gate 防止同一答复并发双发。
- collect/debounce、queue-cap summarize/drop 产生的 constituent claims 全部随 obligation 保存；最后 ACK 一次
  结算所有 claims 和连续 cursor。
- 明确 `/stop`、`/new`、`/reset`、`/use`、`/attach` 或 interrupt 的取消确认成功时，旧 obligation 随 claim
  结算删除；取消确认失败时 obligation 保留为可恢复状态。

## 5. 隐私边界

跨重启原样恢复必然要求在本机保存最终文本。该正文只写入用户 home 下 0600 ledger，不进行远端上传，也不
进入日志、adapter health、control status、Prometheus metrics 或 Provider prompt。诊断面只返回 obligation
总数、pending 数和 ambiguous 数。

该文件当前没有额外静态加密；同机管理员、账户失陷或不安全备份可读取最终答复。这是明确威胁模型边界，
不能用 0600 冒充磁盘加密。

## 6. 自动证据

专门回归覆盖：

- schema v1 → v2 迁移和第二 writer fail closed；
- Adapter.Send 执行时磁盘已经是 `attempting`；
- pending 恢复无警告，attempting/failed 恢复带“可能重复”；
- 多分片从下一个未确认 chunk 恢复；
- 最后 ACK、合并 claims 和连续 checkpoint 原子结算；
- 平台发送失败与 ACK 本地 commit 失败均不推进 cursor；
- Windows 原子替换若 ledger 目标被破坏为目录，会识别为结构性永久错误并立即 fail closed，不进入共享锁
  retry 梯；Gateway 因 durable claim 失败及时发送 retry settlement。该路径通过 normal×100、race×20；
- duplicate inbound 不创建 Controller、不重跑模型；
- 1 MiB/512 chunk 上限和内容身份损坏 fail closed；
- obligation 与历史恢复共享扫描上限；
- 取消成功删除、取消 ACK 失败保留；
- 日志、status、metrics 不泄漏唯一答复正文；
- Bot package 全量与 race。

本批实现完成时至少执行：

```powershell
go test ./internal/bot -count=1
go test -race ./internal/bot -count=1
```

完整 Root/Desktop/Frontend、跨平台、clean-clone、CI/CodeQL 证据按本批最终交付门禁执行，不由这两个包级
命令外推。

## 7. 仍未关闭

- 飞书、QQ、微信的真实历史分页/resume `RecoveryAdapter`；
- Telegram 在 Bot API long polling 之外的独立历史补扫能力；
- 真实四渠道的掉线、重连、审批、取消、分片和节点重启回环；
- 干净 Linux 云节点 linger-enabled logout/reboot 与真实 watchdog kill/restart。

这些需要真实应用、网络或云节点，继续标记 adapter/external-blocked；localhost fixture 不冒充生产证据。
