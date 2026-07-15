# M4 跨资源恢复事务收官审计

日期：2026-07-14

状态：实现、故障注入与完整本地门禁均通过；集中 push 后的 CI/CodeQL 待远端核验。

## 审计目标

关闭 M4 最后一个显式路线图项：previewable writer active turn 以及用户 Conversation/RewindBoth 在 transcript、runtime sidecar、checkpoint 和 workspace 之间的断电恢复窗口。

本审计不声明 `bash`、MCP、外部 API、后台 opaque side effect exactly-once，也不声明 ACL/xattr/硬链接身份恢复或完整 evidence/委派预算跨进程持久化。

## Active turn 协议

每个 shared turn orchestrator 回合在 user-role message 追加前创建 checkpoint。Visible user turn 写入用户可见 boundary；Goal continuation、plan-approved execution 等 synthetic turn 写入 `Synthetic=true` 的隐藏 checkpoint，不出现在 rewind picker，也不会让一次 synthetic 失败跨过前一个已提交 visible turn。

Branch meta 的 `InFlightTurnMeta` 持久化：

- `StartMessageIndex` / `PreserveUser`
- `CheckpointTurn`
- 成功提交后的 `CommitMessageCount` / `CommitTranscriptDigest`

成功顺序：

```text
persist transcript
-> persist runtime sidecar
-> publish commit anchor in branch meta
-> clear in-flight marker
```

Resume 遇到 marker 时同时比较 transcript prefix 和 runtime sidecar anchor。两者都匹配 commit anchor 才视为“结果已提交、仅 marker 未清”；否则从 `CheckpointTurn` 恢复 workspace 和 Goal/Plan/Todo/evidence projection，并按 visible/synthetic 语义删除 partial transcript。当前进程内的 cancel、非流中断 runner error 和 commit persistence error 复用相同恢复函数。

`provider.StreamInterruptedError` 是一个更窄的已产出边界：Provider 已发送可见输出，Agent
已经耗尽有界 tail recovery，前置同步 tool call 也已经完成。此时 Controller 按上述成功
顺序提交当前 partial transcript/runtime 并清 marker，让用户可见的“部分响应已保留”和
Continue 操作与持久状态一致；若这次提交任一步失败，则仍进入同一个 fail-closed 回滚。

Previewable writer 仍在执行前要求 checkpoint record、runtime sidecar 和当前 marker 全部持久化；因此进程退出时要么没有 writer effect，要么存在精确 checkpoint 可回滚。无 session path 的内存 Controller 只保留兼容行为，不获得跨重启保证。

## Rewind roll-forward 协议

`BranchMeta.Rewind` 使用两个阶段：

1. `prepared`：保存 turn、conversation boundary、prefix digest、checkpoint runtime、code scope 和唯一 started-at identity。此阶段 checkpoint 必须仍存在。
2. `resources_applied`：只有目标 transcript、runtime 和可选 workspace restore 全部成功后才能发布。

随后 Controller 持久化 checkpoint retirement tombstone，再清 rewind journal。恢复规则：

- `prepared`：要求 combined rewind 的 checkpoint 仍在，幂等重放 transcript/runtime/workspace，再发布 barrier。
- `resources_applied` 且 checkpoint 尚在：幂等重放全部资源并退休 checkpoint。
- `resources_applied` 且 checkpoint 已退休：从 journal 重发 transcript/runtime，信任 barrier 已持久化的 workspace 完成事实，再清 journal。
- 任一步失败：保留 journal；Resume、新模型 turn 和 Compact/New/Fork/Branch/Switch/Summarize/Rewind 等保留内容的 rotation 操作前都会再次清算，禁止在未恢复状态上继续产生 Agent side effect。

Checkpoint retirement 只能发生在 `resources_applied` 之后，避免删除 prepared 恢复仍需的 file snapshots。每个 checkpoint 文件的物理删除仍只是 GC；`.state.json` tombstone 是恢复权威。

## 单文件持久化加固

`fileutil.AtomicWriteFile` 现要求 sibling temp、write、fsync、chmod、atomic replace。Windows 使用 `MoveFileEx` 的 replace-existing + write-through；Unix rename 后 fsync 父目录。Cross-device/`ERROR_NOT_SAME_DEVICE` 立即 fail closed，不再降级为会撕裂目标的原地 copy。`AtomicWriteRootFile` 在 rooted rename 后同步目标目录。

Session append event 从 best-effort append 改为同步文件；首次创建 event log 时同步父目录。Branch meta 临时文件在 replace 前执行 `Sync()`。这些保证仍依赖底层文件系统/设备正确实现 flush 语义。

## 故障注入与恢复证据

新增/扩展测试覆盖：

- 未提交 writer 进程退出后同时恢复 workspace、transcript 和 runtime；
- transcript/runtime commit 已完成但 marker clear 前退出，重启保留完整结果；
- 流中断耗尽恢复后提交 partial transcript/runtime 并清 marker；注入该提交失败时仅保留
  visible user prompt、runtime anchor 重新匹配回滚后的 transcript；
- 遗留 active-turn 自动恢复再次失败时，新模型 turn 在 checkpoint 分配前被阻断且 marker 保留；
- 新 turn 的 checkpoint 分配或 in-flight marker 持久化失败时，模型 runner 在调用前被阻断；marker 失败后刚分配但未武装的 checkpoint 会被退休，不暴露幽灵 rewind 点；
- synthetic checkpoint 重启后仍隐藏，且只恢复自身 `v2 -> v1`，不跨越 visible turn 回到 `v0`；
- rewind 仅写 `prepared` 后退出；
- `resources_applied` 后、checkpoint retirement 前退出；
- checkpoint 已退休、journal clear 前退出；
- public `Controller.Rewind` 的 phase 写入失败与 final clear 失败均留下正确阶段，并可由新 Controller 冷启动收敛；
- branch meta in-flight/commit/rewind round trip 与 listing refresh 不丢 marker；
- cross-device replace fail closed、Windows error classification、atomic/rooted directory sync 路径；
- Desktop stuck topic timing contract 在 loaded Windows runner 下保留单 grace 上界。

本批最终工作树已通过：

```text
go build ./...
go vet ./...
go test ./internal/... -count=1 -timeout 600s
go test -race ./internal/agent ./internal/control ./internal/checkpoint ./internal/fileutil ./internal/tool/builtin -count=1 -timeout 900s
Desktop: go vet ./... && go test ./...
Frontend: corepack pnpm test:all && corepack pnpm build
Python scripts: 114 passed, 2 skipped
Node upstream reconciliation: 3 passed
Tool/docs/public/deploy/release contracts: passed
scripts/verify-baseline.ps1 -SkipDesktop -SkipFrontendHint -OutputDir <system-temp>: passed
linux/darwin/windows x amd64/arm64, CGO_ENABLED=0: all 6 passed
Go brand residue: 0; gofmt -l: empty; git diff --check: passed
```

综合 baseline 实际构建 CLI、运行缓存敏感测试、公开/部署/发布合同、localhost Gateway smoke 和核心包集；报告与六目标二进制均写入系统临时目录，未污染仓库 `artifacts/`。上述是本地证据；远端 CI/CodeQL 必须在本大批集中 push 后另行核验，且不为回填 run ID 制造额外提交。

普通 `go test ./internal/...` 不会仅因开发机存在 `DEEPSEEK_API_KEY` 而访问真实服务；DeepSeek cache probe 现与 ACP 真实测试统一使用 `live` build tag，只有显式 `go test -tags live ...` 才会编译执行。当前真实 cache probe 的网络请求超时，不计入本批通过证据，也不影响确定性门禁。

## 2026-07-15 installed candidate 回归与修复复验

commit `a0c09de` 的普通 CI run `29376470335` 8/8、CodeQL run `29376470342`
3/3 全绿。Desktop candidate run `29376807221` 的 Linux/macOS jobs 通过，Windows
安装、启动 smoke 也通过，但 interaction smoke 在进入新增 plugin lifecycle 前失败：
`stream_interruption.idle_recovered=false`，错误为
`partial stream output was not persisted after disconnect`。这证明 M4 通用事务回滚错误地
删除了 M1 原生合同明确要求保留的 partial assistant response，不是插件 smoke 本身失败。

当前源码修复后重新构建 production Wails：48,678,400 bytes，SHA-256
`939CC9A9172F7BB7586FCA44DDD1925F04F4D3E34C6715B07C50C230511ED934`。同一 Windows
UIA interaction smoke 完成 19 次 localhost Provider 请求，五类失败场景的
`signal_visible`、`idle_recovered`、`followup_succeeded` 全部为 `true`；
`stream_partial_persisted=true`、`stream_retry_invoked=true`、`recovery_verified=true`，
最终 `boundary_changes=[]`、`errors=[]`。证据写在系统临时目录，不进入仓库。该结果是
源码 production Wails 证据，仍需新 commit 的 installed candidate 远端复验，不能冒充
旧 run 已通过。

修复批次重新通过 root build/vet/internal 全测、`internal/control` 全测与 race、Desktop
vet/full test、前端 `test:all`/production build 与 bundle budget、工具/文档/公共发布/部署
合同、119 个 Python 合同测试（2 skipped）、Node upstream reconciliation、实际 upstream
scan 和六目标 `CGO_ENABLED=0` 交叉编译。以上仍是提交前本地证据。

## 完成边界

该协议不是单个跨文件系统原子写，而是使用 durable intent/barrier 让多个原子资源在重启后收敛到一致的 committed 或 rolled-back 状态。它覆盖 Reames 可预览 built-in 文件 writer、session transcript、Goal/Plan/Todo runtime 和 checkpoint workspace restore。

以下仍需显式核验，不能自动重放：

- shell 命令、MCP 工具、远程 API、数据库、部署和后台 opaque job；
- 只存在于 child 当前 turn 的 bash proof；
- ACL、xattr、硬链接身份以及 checkpoint 未记录的外部编辑；
- 无 session lease 的多进程嵌入写入。

在上述边界内，M4 的 Goal/Plan/子任务/证据/检查点状态机和失败恢复测试门槛已关闭；项目长期 GOAL 与 M5/M6/M7 仍未完成。
