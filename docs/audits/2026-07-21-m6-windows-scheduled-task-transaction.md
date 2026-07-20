# M6 Windows Scheduled Task Gateway service transaction

日期：2026-07-21

## 已实现

`internal/gatewayservice.Apply` 的 Windows user-scope install/uninstall 不再走无回滚的 generic plan：

- mutation 前用 Windows PowerShell `ScheduledTasks` 模块精确定位 `TaskPath`/`TaskName`，通过 schema-v1 JSON
  返回 exists/enabled/running，并用 `Export-ScheduledTask` 保存完整 XML；不解析 `schtasks /FO LIST /V` 的本地化文本；
- 同名 `install --start-now` 在旧任务 running 时先执行 `/End`，再执行可审计 `schtasks /Create` 与 `/Run`，
  避免“running”后置条件仍由旧进程满足；随后用结构化探针验证 task exists+enabled，
  `--start-now` 时还必须 running；
- uninstall 在任务不存在时幂等成功，删除后用同一结构化探针验证 task absent；
- create/delete、取消、前向命令或 postcondition 失败均进入独立 15 秒恢复上下文，不继承已取消的 forward context；
- rollback 先幂等移除可能部分写入的新任务，再把快照 XML 以内存 base64 传给 `Register-ScheduledTask -Xml`，
  恢复 enabled/running；running+disabled 的旧状态按 enable→start→disable 顺序恢复；
- 恢复命令或最终状态验证失败时返回 degraded/manual-repair，不用原始 forward error 掩盖恢复失败；
- 成功 probe 只在 `Result.Outputs` 记录 exists/enabled/running 摘要，不回显完整 XML；前向和恢复命令输出仍保留。
  rollback error 也不拼接 PowerShell args，防止恢复失败时把携带精确 XML 的 base64 回显；不使用
  `ExecutionPolicy Bypass`。

## 故障注入与宿主探针

`service_windows_transaction_test.go` 覆盖：

- schema-v1 JSON 与完整 XML 解码，且公开输出不泄漏 XML；
- 已存在的 running+disabled 任务先 `/End` 再替换，新任务启动失败后恢复精确 XML 与原状态；
- fresh install 在 forward context 取消后使用未取消上下文删除 partial task；
- uninstall postcondition 失败后恢复 XML、enabled 与 running；
- rollback register 失败时返回 degraded/manual-repair、保留 forward/rollback 输出且错误不泄漏 XML base64；
- absent uninstall 幂等且不执行 mutation。

当前 Windows 宿主还实际执行了只读 `ScheduledTasks` 模块探针，目标为不存在的
`\\ReamesAgent\\__reames_transaction_probe_absent__`，返回：

```json
{"schema":1,"exists":false,"enabled":false,"running":false,"xml":""}
```

这只证明模块、路径参数和结构化 JSON 在当前宿主可用，不是管理员级 install/uninstall 演练。

## 未外推

- system scope 仍只提供 dry-run/manual administrator plan；自动事务只覆盖现有允许执行的 user scope。
- 当前没有在干净 Windows 节点对真实任务执行 install/reinstall/start/stop/uninstall、登录后启动、注销、重启、
  ACL 或断电点演练；这些仍需外部节点证据。
- Task Scheduler XML 中的 principal、trigger 与 settings 由原生 Export/Register round-trip 承担；如果恢复命令因账户、
  凭据或策略限制失败，系统明确进入 degraded/manual-repair，而不是声称已恢复。
