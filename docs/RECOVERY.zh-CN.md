# 恢复、Guard 与安全模式

Reames Agent 提供一条在普通 Agent runtime 之前运行、无需凭据的恢复路径，用于处理
配置损坏、Desktop 连续启动失败、自更新未完成、扩展状态损坏，以及导致主界面无法打开的
Desktop 派生状态。

恢复路径不会初始化 Provider、读取 API key、连接 MCP、加载插件或 Hook、启动 Bot/Gateway、
启动 LSP，也不会进入普通 Agent loop。所有入口都复用 `internal/repair` 这一套持久恢复模型；
CLI、Serve、Desktop、Gateway 和打包后的 Guard 只负责投影，不建立第二套状态机。

## 启动入口

打包后的 Desktop 默认先经过同安装目录的 Guard：

- Windows：`Reames Agent.exe` 是 GUI 子系统 Guard launcher；安装单元同时包含
  `reames-agent-guard.exe` 和 `reames-agent-desktop.exe`。
- macOS：app bundle 的 `CFBundleExecutable` 是 `reames-agent-guard`，Wails 主程序仍是
  `Contents/MacOS` 内的同级文件。
- Linux：`.desktop`、归档和 `.deb` 都包含 Guard 与 Desktop，并通过
  `reames-agent-guard launch` 启动。

主 CLI 在配置、i18n、boot 和 runtime 初始化前提前分派 `guard`：

```text
reames-agent guard check --json
reames-agent guard repair
reames-agent guard launch --safe-mode
reames-agent guard rollback
reames-agent guard snapshots --json
reames-agent guard restore --snapshot SNAPSHOT_ID
reames-agent guard undo
reames-agent guard rebuild --target tabs|projects|window|zoom|all
reames-agent guard disable-plugins
```

独立 `reames-agent-guard` 二进制接受相同子命令。`--app` 只能指向 Guard 自身解析后安装
目录中的 `reames-agent-desktop[.exe]`；符号链接解析、固定 basename 和同目录检查使它不能
退化成任意程序启动器。

## 启动账本与 crash-loop 判定

有界启动账本记录：

```text
starting -> ready -> healthy -> clean-exit
                \-> failed
```

- Desktop 在普通初始化前记录 `starting`；
- Wails DOM ready 后记录 `ready`；
- UI 持续存活 30 秒后记录 `healthy`，此时才确认待定更新；
- 正常退出记录 `clean-exit`，DOM ready 前退出不会错误认证健康；
- 五分钟内三次未完成启动会建议安全模式；仍存活的 PID owner 不会被覆盖或当作失败。

启动账本和 pending update 使用 OS 级跨进程锁与原子写；健康或正常退出会清零有界失败计数。

## 自动回滚证据

只有以下证据同时成立，Guard 才自动修改已安装二进制：

1. 启动账本达到 crash-loop 阈值；
2. pending-update transaction 结构和语义有效；
3. 失败启动版本等于 transaction 的 `toVersion`；
4. 更新目标属于当前 Guard 安装目录；
5. 完整安装单元中所有原有文件的备份 SHA-256 均通过；
6. 持有回滚锁时 transaction 的版本与创建身份仍未变化。

安装单元包含 Desktop、Guard、launcher/helper 等同级文件，也记录升级前不存在的文件。回滚会
先完成全部 staging，再开始 swap；中途失败会执行补偿，并删除失败版本新增的文件。若补偿后
不能证明安装单元版本一致，会报告 `mixedInstall` 并 fail closed，要求从可信发布重新安装。

Windows update helper 在 installer 失败后先写持久 failure marker，再重启 Guard；helper 缺失时
自动更新直接失败，不绕过归因去启动 installer。Linux 若 Guard 已替换而 Desktop 替换失败，会写
apply-failure marker；macOS 在新版本通过健康观察期前保留完整旧 `.app`。同一时间只允许一份
pending update，后续更新不能覆盖仍在观察或失败状态的回滚证据。

只要 crash 归因、二进制来源、目标目录、transaction 身份或备份哈希存在歧义，Guard 就不会
修改二进制，只进入安全模式或要求人工安装可信版本。

## 安全模式边界

通过 `REAMES_AGENT_SAFE_MODE=1` 或 `guard launch --safe-mode` 请求安全模式。安全模式只使用
内建恢复默认值，不读取或迁移用户/项目 TOML 与 dotenv。

安全模式禁用：

- 用户/项目 Skill、Hook、MCP、插件包、宿主 ExtraPlugins 和共享 Plugin Host；
- Bot/Gateway、LSP、状态栏命令、更新检查、遥测、Metrics、Heartbeat、启动 ping、crash/metrics
  flush 和 Recovery GC；
- planner、Guardian、subagent 与 Memory Compiler；
- 现有 Desktop tab/session 恢复。

安全模式是诊断和修复控制面，不是缩小版自治 Agent。Desktop 只建立 recovery-only shell，
`boot.Build` 拒绝 Provider、Controller、工具和普通 Agent 装配。它不会放宽权限、重放工具、
启动网络扩展，也不会把损坏安装冒充为健康状态。

## 统一状态投影

同一份 `repair.Report` 通过以下入口提供：

- `control.Controller.RecoveryStatus()`；
- Serve `GET /api/recovery`；
- Desktop `GetRecoveryStatus()`；
- `reames-agent gateway recovery-status [--json] [--home PATH] [--root PATH]`；
- Guard `check` / `diagnose`。

`gateway run` 会在加载配置、Provider、插件和渠道之前执行 credential-free recovery preflight，
因此 systemd、launchd 与 Windows Scheduled Task 都复用同一恢复状态，不需要 service 专用状态机。

报告包含启动阶段、配置检查、pending update、current/previous 二进制哈希、session store 可读性、
插件状态计数和可操作 finding。可选文件不存在不自动算错误；无效 metadata、损坏配置和不可读插件
状态会阻断普通启动。

## 运维操作手册

### Desktop 连续打不开

1. 运行 `reames-agent-guard check --json` 并保存报告；
2. 运行 `reames-agent-guard launch --safe-mode --detach`；
3. 配置损坏时运行 `repair`；只有显式加 `--project` 才隔离项目配置；
4. 派生 UI 状态异常时使用 `rebuild --target ...`；
5. 只有显式 `disable-plugins` 才会全量禁用受管插件；
6. `rollback` 只用于有可信 pending update 的场景，否则重新安装可信 release。

### 配置修复

系统最多保留五份带 SHA-256 的健康全局配置快照。修复操作先隔离原始损坏 bytes，不原地覆盖；
必要时恢复最新有效全局快照，并记录可 undo 的 transaction。使用 `snapshots`、`restore` 和 `undo`
保持所有变更显式且可审计。

### Gateway service 拒绝启动

```text
reames-agent gateway recovery-status --json --home /absolute/home --root /absolute/workspace
reames-agent gateway doctor --deep --home /absolute/home
```

先修复共享报告中的问题，再重启 service。不要通过复制 service definition 或另建状态文件绕过预检。

## 限制与外部证据

本地测试能证明单元、race、故障注入、跨进程锁、打包合同、交叉编译和真实 Guard/child 进程交互；
不能证明超出文件系统保证的断电原子性、抵抗同机管理员、公开签名发布链、notarization，或三平台
每一种真实安装升级/回滚。

生产声明仍需要带 provenance 的签名发布，以及 Windows/macOS/Linux 安装态演练：crash-loop 自动
回滚、installer failure、安全模式启动、logout/reboot 后 service 常驻和 compromise response。
任何来源不明确的二进制都必须保留为人工决策。
