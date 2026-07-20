# M6 macOS launchd Gateway service transaction

日期：2026-07-21

## 已实现

`internal/gatewayservice.Apply` 不再让 macOS install/uninstall 走无回滚的 generic plan：

- mutation 前快照 plist bytes/mode，以及 `launchctl print gui/<uid>/<label>` 的 loaded/running 状态；
- 同名 `install --start-now` 先 bootout 旧 job，再原子写入 plist、bootstrap、kickstart，并验证 loaded+running；
- uninstall 对未加载 plist 不强行 bootout，删除后验证 manager 已缺失；定义和 manager 都不存在时幂等成功；
- command、取消、写入、删除或 postcondition 失败均进入独立 15 秒恢复上下文，不继承已取消的 forward context；
- rollback 先清除可能部分 bootstrap 的新 job，再恢复旧 plist，按快照恢复 loaded/running，并重新 probe；
- definition 恢复、manager 恢复或恢复后状态验证失败时返回 degraded/manual-repair，不用原始 forward error 掩盖恢复失败；
- forward 与 rollback 命令输出都保留在 `Result.Outputs`，但 plist 和命令仍不含 secret 值。

## 故障注入

`service_darwin_transaction_test.go` 覆盖：

- 已存在且 running 的 job 在新 kickstart 失败后恢复旧 bytes/mode 和 running 状态；
- bootstrap 已可能产生副作用且 forward context 被取消时，rollback 使用未取消上下文清理新 job/新 plist；
- uninstall postcondition 失败后恢复 plist、bootstrap、kickstart 和原状态；
- `launchctl print` running/not-found/orphan manager 状态解析。

## 未外推

- 本批是 deterministic fault-injection，不是实际 macOS 主机 launchd 证据；真实 user login/logout、重启、权限和签名 app bundle 仍需 macOS 节点。
- Windows Scheduled Task 的同等级 snapshot/rollback/postcondition 已由本批后续实现，见
  `2026-07-21-m6-windows-scheduled-task-transaction.md`；两者都仍缺真实服务节点生命周期证据。
- system scope 仍只提供 dry-run/manual administrator plan；本事务只覆盖现有允许自动执行的 user scope。
