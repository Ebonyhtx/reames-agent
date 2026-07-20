# M6 Linux Gateway 卸载事务审计

日期：2026-07-20

范围：Linux user-scope `gateway uninstall` 的快照、后置验证、取消恢复和故障回滚。本文不扩大到
system scope、macOS launchd、Windows Scheduled Task 或真实云节点常驻证据。

## 问题

此前 Linux `gateway install` 已具备 unit bytes/mode 与 enabled/active 状态快照，但 `uninstall` 仍按
`disable --now -> 删除 unit -> daemon-reload` 顺序直接执行。以下任一步失败都可能留下半卸载状态：

- `disable --now` 已停止或禁用服务后返回错误；
- unit 删除失败，但旧服务没有恢复为原来的 enabled/active 状态；
- unit 已删除而 `daemon-reload` 失败，磁盘定义与 systemd manager 视图不一致；
- 调用上下文在卸载中途取消，后续恢复继承取消而无法运行；
- 命令表面成功，但 manager 仍报告残留 enablement 或 active 状态。

## 实现

`internal/gatewayservice` 现在把 Linux user-scope install 和 uninstall 都送入显式事务入口：

1. mutation 前以 `lstat` 拒绝 symlink/非普通 unit，读取并保留原 bytes 与 mode；
2. 用 `systemctl --user is-enabled` 和 `is-active` 只接受可精确恢复的
   enabled/disabled × active/inactive 状态；
3. unit 与 manager 均干净缺失时，uninstall 幂等返回；unit 缺失但 manager 有孤儿状态时仍 fail closed；
4. 正向执行 `disable --now`、删除 unit、`daemon-reload`；
5. 正向命令完成后再次以“unit 不存在”语义探测 manager，不能只凭命令 exit code 声明完成；
6. disable、删除、reload、后置探测或调用取消失败时，使用
   `context.WithoutCancel` 派生的 15 秒恢复上下文；
7. unit 已删除时先原子写回旧 bytes/mode，再成功 `daemon-reload`，最后恢复原 enabled/active；
8. 旧 unit 写回或恢复 reload 失败时停止后续 manager mutation，并返回
   `degraded; manual repair is required`，避免在 manager 看不到可靠定义时继续 enable/restart。

事务输出只保存 service-manager 命令的诊断文本；service 定义仍只包含 `REAMES_AGENT_HOME` 路径和
Gateway 参数，不嵌入 Provider/IM secret。

## 故障注入证据

`internal/gatewayservice/service_test.go` 新增并覆盖：

- 成功卸载后 unit 消失且执行第二次 absent-state probe；
- unit 与 manager 已缺失时不执行 delete 或 mutation 命令；
- 正向 reload 失败后恢复旧 bytes/mode、enablement 与 runtime；
- delete 失败时不做无意义 reload，但恢复 enabled/active；
- postcondition 发现 manager 残留时恢复完整旧状态；
- 旧 unit 写回失败时停止 manager 恢复并报告 degraded/manual repair；
- forward context 取消后 rollback 使用新的未取消上下文；
- `applyWithDeps` 的真实 Linux 路由确实进入 uninstall transaction，而不是退回 generic plan。
- Linux service smoke schema 在首次 `LoadState=not-found` 后再次执行真实 CLI uninstall，要求第二次卸载
  仍报告完成；取得可启动的 systemd user manager 后，这会把幂等合同纳入纵向证据。

同批也补强了 credential-free headless 纵向预检。`scripts/smoke_gateway_headless.py` 使用本批实际构建
二进制执行 `gateway recovery-status --json`：健康配置必须返回 schema v1、global config valid 和零
findings；fixture 随后临时写入损坏 TOML，要求 exit code 1 并投影 `config.invalid`，最后逐字节恢复原
config。机器报告新增非空 `recovery_preflight` section，并证明 app ID 与 synthetic Provider key 未进入
输出或持久证据。该链路验证共享 `repair.Report` 的 CLI 投影与 fail-closed 行为，不需要真实凭据，也不
替代真实 service-manager 启动。

目标包普通测试、race 与新增测试 `-count=20` 均通过；同一工作树的 Root build/vet/全 `internal/...`、
Desktop build/vet/全测试、Frontend `test:all`/production build/bundle budget、155 项 scripts tests、
credential-free Gateway smoke 以及 linux/darwin/windows × amd64/arm64 的 CLI/Guard 12 个
`CGO_ENABLED=0` 目标也已通过。代码提交 `a6d6fd07136453041c275e40f4f8e2b4f9bca04f` 的完整 clean clone、
CI `29754127548` 8/8 与 CodeQL `29754135162` 3/3 随后全部通过。本审计所在的证据闭环提交仍以自身
HEAD 的远端结果为最终门槛，不能复用旧 SHA 的绿色状态。

## 证据边界

- 当前事务只覆盖 Linux user scope；`--scope system` 继续只渲染计划并要求 root 人工确认。
- macOS launchd 与 Windows Scheduled Task 仍使用顺序应用，没有同等级快照/回滚保证。
- 本轮尝试重新启动本机 WSL2 systemd user manager 时，宿主返回 `0x800705aa`（系统资源不足），因此没有
  把本轮实现写成新的真实 manager smoke；历史 WSL 生命周期证据仍有效，但不替代当前提交的 clean-node 复验。
- linger-enabled logout/reboot、watchdog kill/restart、真实 Provider/IM 回环仍需外部环境，保持
  `external-blocked`。
