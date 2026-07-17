# P2 Offline Guard、Safe Mode 与 Reasonix 增量审计

日期：2026-07-17
范围：Reasonix `099879592742ddeb25b312347b4c37316e8b76f9` →
`c966d0279629f814731cd39171b1a725ae1ab489`，以及 Hermes、Codex、MiMo、Claude Code、
Kimi Code、Scream Code 中与启动恢复、会话可靠性、worktree/桌面交付直接相关的新增信号。

## 结论

P2 的仓库内实现已形成单一恢复闭环：独立 Guard 在普通 runtime 之前运行，Safe Mode 不读取
用户/项目配置或凭据，不启动 Provider、MCP、插件、Hook、Bot、LSP、planner、Guardian、
subagent 或 Memory Compiler；crash-loop、配置快照、pending update 和安装单元回滚共享
`internal/repair`，CLI/Serve/Desktop/Gateway 只投影同一报告。二进制自动回滚必须同时满足
版本归因、安装目录、transaction identity 与全量备份哈希，任何歧义 fail closed。

Reasonix 本轮还暴露了两个与 Guard 不同但同属交付可靠性的缺口：错误 HTTP body 半开会冻结
重试；大 session 在每次切换时重复快照及 stale activation 会造成卡顿或内容表象丢失。本批同步
吸收了有界 error-body read、verified snapshot fast path、损坏 event-log 尾部取证 sidecar 和
last-click-wins navigation epoch。

## Reasonix 五提交分类

| 上游提交 | 机制 | Reames 决策 |
|---|---|---|
| `1bd5f04d` | CLI composer、响应式状态栏、主题与本地化 | 不整批移植。属于体验候选，不能高于恢复/发布可靠性；现有 CLI 合同保持不变 |
| `f590a66e` | quick picker 必须返回 pending model-switch `tea.Cmd` | 已等价：`chat_tui.go` 三条 picker 路径均返回 `pendingModelSwitch`，保留现有回归 |
| `8f2c209a` | MCP persistent stdio、trust identity 简化、Windows lock handoff | 不回退 Reames 已关闭的 identity-bound receipt/launcher lock。Reames 使用更严格 workspace/source/executable/schema identity 与跨进程原子 trust state；persistent writer 的放权取舍不直接复制 |
| `dae65e25` | stalled error body、session snapshot fast path、损坏 tail salvage、stale activation fencing | 采用并按 Reames session/control/Desktop 边界适配 |
| `c966d027` | 删除 Memory v5 execution compiler；TOML retired-key 迁移加固 | 不删除 Reames Memory Compiler。Reasonix 的默认 observe 无收益与 compact runaway 结论不等于 Reames 当前产品合同；Safe Mode 已明确禁用它。后续只有在 Reames 自身 provider-visible 收益、loop 风险与 cache benchmark 证据支持时才调整。其 multiline-safe retired-key 扫描仅适用于本轮删除迁移，Reames 未新增对应 destructive migration |

## 采用实现

### Offline Guard 和更新恢复

- `internal/repair`：五分钟三次失败的进程所有权启动账本，`starting/ready/healthy/clean-exit/failed`
  阶段与 30 秒健康观察期；Windows/Unix 跨进程锁。
- 五份 SHA-256 健康配置快照；损坏 TOML 隔离、恢复和 undo transaction。
- pending update 保存完整安装单元：原有文件 bytes/hash 与原先缺失文件；回滚先全量 staging，
  再 swap，失败执行补偿并报告 `mixedInstall`。
- Windows helper 名称与安装包统一，helper 缺失时自动更新 fail closed；installer failure marker、
  Linux partial-apply marker、macOS 完整 `.app` backup。已有 pending 未清算时禁止覆盖回滚证据。
- `cmd/reames-agent-guard` 和 CLI early dispatch 支持 check/repair/launch/rollback/snapshots/restore/
  undo/rebuild/disable-plugins。
- `--app` 仅接受同安装目录固定 Desktop basename；真实 Guard/假 Desktop 子进程测试验证 Safe Mode
  env、退出码透传和目录拒绝。

### Safe Mode

- `REAMES_AGENT_SAFE_MODE=1` 只装载内建 recovery defaults，不读取或迁移用户/项目 TOML、dotenv。
- Desktop 只建立 recovery-only shell；`boot.Build` 拒绝 Provider、Controller、工具 registry 与普通
  Agent 装配。禁止 Skill、Hook、MCP、plugin package/host、Bot、LSP、状态栏命令、更新检查、遥测、
  metrics、planner、Guardian、subagent、Memory Compiler、heartbeat、启动 ping、flush/GC 与旧
  tab/session 恢复。
- 普通模式装配保持既有行为；Safe Mode 不提供自治执行或权限放宽。

### 统一投影和系统服务

- `control.Controller.RecoveryStatus()`、`SessionAPI.RecoveryControl`、Serve `GET /api/recovery`、
  Desktop `GetRecoveryStatus()` 共用 `repair.Report`。
- `gateway recovery-status` 提供 credential-free CLI 投影；`gateway run` 在加载 config/Provider/
  plugin/channel 前执行同一 preflight，因此 systemd、launchd、Windows Scheduled Task 不需要第二套
  service recovery state。
- 插件全量禁用复用 `pluginpkg.DisableAll` 既有跨进程锁和原子状态发布。

### Reasonix session/provider 可靠性

- 非 2xx response body 读取设置 10 秒硬边界；timer 关闭半开 body，使 retry loop 可恢复。
- 只有当前进程完成并验证 transcript/ledger 一致的 save 才能启用 snapshot no-op fast path；每次命中
  仍以 transcript/event-log 文件戳与 ledger revision/digest 做 O(1) 磁盘 fencing。load-adopted
  baseline、外部磁盘变化、rewrite、normalization repair 或 damaged log 都回到完整保存。
- event log 修复在 truncate 前把原始尾部写入 `*.events.jsonl.damaged`，该 sidecar 不参与 replay，
  但随 session delete/restore/purge/rename/doctor bundle 生命周期处理；sidecar 落盘失败时不截断原日志。
- Desktop navigation 使用 monotonic epoch；新点击在进入串行 queue 时立即使旧 activation 失效，
  stale completion 不切换 active tab、不清理新 surface cache。

## 其他参考项目增量

| 项目与 reviewed SHA | 信号 | 决策 |
|---|---|---|
| Hermes `ef9e0c98f5c2` | 既有 updater/remote boot/transcript 信号；新增 auxiliary runtime 按 turn/context 隔离、fallback 同步、FTS 写路径自愈 | Reames 已有 tab/model epoch、会话账本且不使用 Python auxiliary/SessionDB FTS；无同构采用项 |
| Codex `e0ac6d0ec9ee` | installed-app runtime projection、parent-owned thread 只读、realtime handoff、executor capability discovery、multi-exec workspace isolation、测试环境隔离 | runtime projection/capability discovery 保留为 P3/token-economy 参考；P1 已有更严格父 Controller/worktree/工具重绑，不复制第二套 thread/exec-server 模型；其余作为回归信号 |
| MiMo `28a0ced5e8e9` | empty-step guard 收窄；tool_script serialization/jail/MCP dispatch 改为 opt-in | Reames 不含同构 JS tool_script；不引入第二套 sandbox。空 step 不作为本 P2 启动恢复变更 |
| Claude Code `67f390c9a0b1` | changelog/feed | 无代码采用项 |
| Kimi Code `7d393b56fb32` | FetchURL SSRF/DNS rebinding、Windows workspace identity、mid-turn error、stale attachment queue、移除 Desktop app | Reames web_fetch 已逐 DNS answer 校验、pin direct dial、逐跳 redirect 保护并覆盖 mapped IPv6/CGNAT；session/workspace path 已 Windows case-fold/canonicalize；当前没有 Kimi Web queue/Minidb 架构，不移植。删除第二套 Desktop runtime 的方向与 Reames 一致 |
| Scream Code `b37627ad8e3b` | README 增加桌面下载链接 | 已逐行审查，无 Goal Loop/Storm Breaker 或代码变化，无采用项 |

所有上游均逐项 `--accept` 到 lock 中记录的 reviewed SHA，没有使用 `--accept-all`；最终
Upstream Watch 为 `changed_count=0`。

## 验证证据

本批最终本地通过：

- Root：`go build ./...`、`go vet ./...`、`go test ./internal/... -count=1 -timeout 600s`。
- Desktop：`go build ./...`、`go vet ./...`、`go test ./... -count=1 -timeout 600s`；主包
  204.431 秒通过。
- Frontend：`corepack pnpm test:all`、`corepack pnpm build`，bundle budget 通过。
- Race：`repair`、`guardcmd`、`provider`、`agent`、`control`、`pluginpkg`、`gatewayservice`；Desktop
  Safe Mode/updater/Guard 与 Windows update helper 定向 race 通过。
- 六目标：linux/darwin/windows × amd64/arm64 的 CLI 与 Guard 均以 `CGO_ENABLED=0` 编译通过。
- Python discovery：126 tests，2 skipped；docs/deploy/release/public contracts、Desktop artifact
  contract、工具文档契约、upstream watch 单测/issue reconciliation 均通过。
- `actionlint v1.7.7` 与 Git for Windows `bash -n scripts/desktop-build.sh` 通过；`git diff --check`
  无错误，Frontend `dist` 占位符和本地 upstream artifacts 未污染 tracked 工作树。
- 本地提交候选的 `--no-local` clean clone 从空 Frontend `node_modules` 重新完成 Root
  build/vet/internal 全测、四类合同、Desktop build/vet/full test、Frontend frozen install/
  `test:all`/production build/bundle budget；tracked 文件保持干净。
- Upstream Watch deep scan 后逐项显式接受 Reasonix/Hermes/Codex/MiMo/Claude/Kimi/Scream Code，
  审查截止点 `changed_count=0`，未使用 `--accept-all`。

最终交付声明仍以本批结束时的全量 build/vet/internal/Desktop/frontend/race、六目标 CGO=0、
release/deploy/public/upstream contracts、clean clone 和远端 CI/CodeQL 为准；定向测试不能代替这些门槛。

## 未扩大声明

- 自动回滚只覆盖有合法 pending transaction 且 provenance/attribution 完整的直接前任版本；
  不是任意版本管理器。
- 本地 fault injection 不等于真实 Windows installer、macOS notarized bundle、Linux package manager
  在断电下的全覆盖演练。
- 同机管理员可修改程序、状态或锁文件，不在应用层信任边界内。
- Safe Mode 不恢复未知外部副作用，不提供 MCP/API exactly-once，也不替代可信重装。
- 公开 registry 生产 endpoint、人员/HSM threshold ceremony、实际 rotation/compromise drill、独立
  provenance policy verifier 仍为 `external-blocked`。
