# M6 Gateway、备份与升级恢复事务审计

日期：2026-07-14

范围：Linux user-scope Gateway 安装失败恢复、Reames home/state 便携备份与新目标恢复、CLI 二进制升级/回滚。本文记录当前实现与证据边界，不替代 `DEVELOPMENT_PLAN.md` 的优先级。

## 结论

本批关闭了三条可由本地确定性验证的恢复链路：

1. Linux user-scope `gateway install` 在变更前快照旧 unit bytes/mode 和 enabled/active 状态，写入后执行 `systemd-analyze --user verify`；前向失败后使用不继承取消的限时上下文回滚。
2. `backup create/verify/restore` 支持 home/state 分根、已知凭据排除、归档自洽验证、仅恢复到不存在的新目标，以及多根后续提交失败时的进程内回滚。
3. CLI updater 实际执行 staged binary 的 `version`，发布后再次检查，保留 `<executable>.previous`；`upgrade --rollback` 交换 current/previous，并与升级共享同目录互斥锁。

这些机制达到“失败可见、常规进程内失败可恢复”的本地门槛，但没有 durable crash journal，不构成断电/强杀原子性声明。

## Gateway 安装边界

- 事务入口仅为 Linux、user scope、install；macOS launchd、Windows Scheduled Task、uninstall 和 system scope 仍使用原有顺序应用或人工计划。
- 已存在 unit 只接受可精确恢复的 `enabled|disabled` 与 `active|inactive`；`static` 等状态在写入前拒绝。
- 新 unit 或旧 unit 写入、verify、daemon-reload、enable、restart、is-active 失败都会进入回滚。
- 旧 unit 写回失败后不再执行 daemon-reload/enable/disable/restart/stop；错误明确标记 degraded 并要求 manual repair。
- 旧 unit 写回成功但 rollback daemon-reload 失败时同样 fail closed，不继续恢复 manager 状态。
- fresh install 无法删除新 unit 时跳过最终 daemon-reload，并报告 degraded/manual repair。
- rollback 使用 `context.WithoutCancel` 派生的 15 秒上下文；原请求取消不阻止恢复，但超时后仍可能需要人工修复。

故障注入覆盖 fresh/existing unit、四种 enabled/active 组合、取消、定义写回失败、恢复 reload 失败、fresh remove/disable 失败和输出保留。

## Backup 格式与安全边界

- manifest schema v1 记录 root、portable relative path、type、owner-relevant mode、size、mtime 和逐文件 SHA-256；manifest 采用严格字段和 EOF 校验。
- create 在扫描和写入两个阶段复验源文件身份与内容，拒绝 symlink、special file、session lease、路径替换以及大小/哈希漂移。
- 排除 `.env`、`credentials`、`credentials.enc`、`weixin/accounts`、`bot/pairing.json`、cache、pending、tmp、lock 和 lease 文件；OS keyring 不导出。
- 排除已知凭据不代表归档无 secret：session、memory 和 custom config 仍可能包含用户粘贴内容，归档始终按敏感数据处理。
- verify 使用同一个已打开 archive file 计算初始/最终 digest，拒绝 ZIP traversal、absolute/UNC/backslash、Windows device name、case 或 Unicode NFC collision、file/descendant 冲突、额外 entry、zip bomb 比例和 payload 篡改。
- embedded manifest hash 只证明自洽；恢复前仍须比对单独保存或签名的 archive SHA-256。
- restore 只接受不存在且祖先不是 symlink 的新目标；每个 root 在同一 parent 下 staging，文件先写入/哈希/fsync，再以 no-replace rename 发布。
- 多根后续 publish 或 parent sync 失败会在进程内逆序回滚；没有跨根 crash journal，强杀可能留下 `.reames-restore-*` 或空 parent。
- Unix 归档权限为 `0600`，恢复文件只保留 owner bits；Windows 机密性仍依赖目标目录 ACL，当前没有自动 DACL 收紧证据。
- `--offline` 只是用户确认所有 CLI/Desktop/serve/bot/Gateway/cron writer 已停止，不是全局进程锁。

## Updater 恢复边界

- 下载资产继续使用官方仓库、精确 GoReleaser 资产名和 SHA256SUMS；这不是签名或 provenance。
- staged binary 必须在 10 秒内输出且只输出 `reames-agent vX.Y.Z` 形式的预期 canonical semver；测试会实际编译并执行候选程序，不只测试字符串解析。
- publish 前保存原 current；成功后 immediate predecessor 位于 `<executable>.previous`，更旧 previous 仅在整个事务成功后删除。
- publish、post-install health check 或旧 previous 清理失败时恢复更新前的 current/previous 组合；恢复本身失败会明确标记 degraded/manual repair。
- `upgrade --rollback` 先验证 previous，再交换 current/previous；previous publish、health check 和最终 predecessor retention 失败均有原状态恢复测试。
- 同目录 advisory lock 阻止两个 Reames updater/rollback 并发；它不防同用户恶意进程忽略锁或替换祖先目录。
- updater 不静默重启 Gateway，只输出 `reames-agent gateway restart` 指引。

## 本地证据

本批工作树执行并通过：

```text
go build ./...
go vet ./...
go test ./internal/... -count=1 -timeout 300s
go test -race ./internal/homebackup/... ./internal/gatewayservice/... -count=1
go test ./internal/cli/... -count=1
```

另对 `linux|darwin|windows` × `amd64|arm64` 以 `CGO_ENABLED=0` 完成 CLI 测试二进制编译。最终 commit 前还需执行 Desktop、文档/部署/release 合同和 diff 检查；push 后的新 CI/CodeQL 才是本批远端证据。

## 未关闭证据

- 干净 Linux 云节点上 linger-enabled 用户的 logout/reboot 常驻。
- 真实备份 archive 的异机恢复、独立可信 SHA-256 比对和强杀后的人工恢复演练。
- 公开签名 release 在 Linux/macOS/Windows 上的真实 self-replace、Gateway restart 与 `upgrade --rollback`。
- macOS/Windows Gateway install 的同等级事务回滚。
- 真实 Provider 与飞书/QQ/微信文本、审批、取消和恢复回环。
- Windows 自动 owner-only DACL、签名、notarization 和 provenance。
