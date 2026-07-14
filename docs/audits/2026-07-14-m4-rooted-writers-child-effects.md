# M4 Rooted Writer 与 Durable Child Effects 审计

日期：2026-07-14

状态：本地实现与受影响包验证完成；等待本批集中 push 后的远端 CI/CodeQL 证据。

## 审计目标

本批关闭 `docs/DEVELOPMENT_PLAN.md` 中两个高风险恢复窗口：

1. Previewable built-in writer 与 checkpoint restore 从“路径预检后按路径写入”迁移到 handle-relative resolve-beneath I/O。
2. Writable child effect 从 turn/process 内存桥接扩展到可崩溃恢复、可防重放的 durable journal。

不在本批完成声明内：`bash`、MCP、外部 API 的任意副作用 exactly-once，ACL/xattr/硬链接身份恢复，以及 transcript/runtime/checkpoint/workspace 的单一跨资源断电事务。

## Rooted writer 边界

`internal/fileutil/rootwrite.go` 提供 `AtomicWriteRootFile`：同目录临时文件、write、fsync、chmod 和 root-relative rename；拒绝 `..` escape 和 symlink/reparse escape，不提供 cross-device 非原子降级。

`internal/tool/builtin/rooted_target.go` 把每个目标绑定为打开的 `os.Root`、relative path、原始 UI/checkpoint path 和 canonical identity。以下生产 writer 已迁移：

- `write_file`
- `edit_file`
- `multi_edit`
- `delete_range`
- `delete_symbol`
- `notebook_edit`
- `move_file`
- `apply_patch`

`move_file` 对 source delete 与 destination create 同时提供 preview；跨 root move 使用 rooted copy + source remove。`apply_patch` 支持 create/update/delete、dry-run、完整 preflight、逐文件原子替换和后续失败回滚；隐式 rename 被拒绝并要求调用 `move_file`。

Agent 通过 `tool.MultiPreviewer` 在任何写入前收集并 checkpoint 全部 change。`checkpoint.Store.RestoreCode` 打开单一 workspace `os.Root`，预检、删除、写入和失败回滚均复用该 handle。
Checkpoint record/state 以 `0600` 持久化并创建 `0700` 目录，避免完整 pre-edit bytes 被同机其他用户读取。

## Durable child effect 协议

每个顶层持久 `task` 或 writer-capable `run_skill` ref 最多创建一个 `*.effects.json` sidecar。嵌套 delegation 继承顶层 journal，避免生成只能由 child transcript 解释、root 无法验证的锚点。

Sidecar 合同：

- schema version 1；权限 `0600`；最多 256 个 retained event，序列化上限 1 MiB；
- 超限时把丢弃前缀折叠为 `compactedThrough` 与保守 `compactedMutation`；
- 只保存 tool/call、success、redacted command、paths、read/write、mutation、depth、parent session/call 与 sequence；
- 不保存 child model text、reasoning、tool output、args、Todo、step 或 provider message；
- command 先经过 `trust.RedactSecrets`，path/command/总文件大小均有硬上限。

Previewable child writer 的时序：

```text
preview all changes
→ persist ancestor checkpoints
→ append durable mutation intent
→ execute writer
→ append durable result receipt
→ publish structured receipt to ancestor ledgers
```

Intent 持久化失败会在 writer 执行前拒绝。结果持久化失败时，先前 intent 仍留下保守 mutation boundary；系统不声称外部副作用 exactly-once。

Parent runtime sidecar 保存 `{ref,journalID,sequence}` cursor。恢复只扫描 parent session 所有者匹配的 journal，并校验 workspace、owner kind 和 parent `task`/`run_skill` call anchor。未确认 child mutation 清空旧 root check refs；child-only bash 不恢复为 root proof。已确认 sequence 幂等跳过，因此 parent compaction 移除旧 delegation call 后不会重复清空更晚的 root check。Journal replacement、sequence rollback、stale anchor、branch crossing 和 corrupt metadata/journal 均 fail closed。

## 自动化证据

新增或扩展测试覆盖：

- root atomic create/replace、escape/symlink 拒绝与临时文件清理；
- writer symlink escape 和验证后组件替换；
- `apply_patch` create/update/delete、dry-run、context mismatch、rollback、workspace/session escape 与 rename 拒绝；
- multi-file preview 全量 checkpoint，任一路径快照失败阻断整个 writer；
- `move_file` rewind 同时恢复 source 并删除 destination；
- checkpoint root-relative/case/hard-link alias 去重、canonical record 校验、durable truncate tombstone，以及 post-preflight symlink swap（含 root 内目录交换）不触碰或误写非目标文件；
- checkpoint record/state 与目录的 `0600`/`0700` 权限合同；
- child intent crash/restart、duplicate/replay、cursor、parent compaction、stale anchor、branch crossing、corrupt meta/journal、secret redaction、bounded compaction；
- journal 持久化失败时 child writer 零落盘；
- background task 跨 turn 后由 sidecar 恢复 mutation boundary；
- writer-capable `run_skill` 使用 `run_skill` parent anchor；
- child-only project check 只满足当前 turn，不进入 durable root verification。

当前已通过：

```text
go build ./...
go vet ./...
go test ./internal/... -count=1 -timeout 600s
go test -race ./internal/agent ./internal/control ./internal/checkpoint \
  ./internal/fileutil ./internal/tool/builtin -count=1 -timeout 900s
Push-Location desktop; go vet ./...; go test ./... -count=1 -timeout 600s; Pop-Location
Push-Location desktop/frontend; corepack pnpm test:all; corepack pnpm build; Pop-Location
python -m unittest discover -s scripts -p "test_*.py" -v  # 114 passed, 2 skipped
scripts/verify-baseline.ps1 -SkipDesktop -SkipFrontendHint -OutputDir <TEMP>
go test ./internal/tool -run TestBuiltinToolContractDocumentation -v
six-target CGO_ENABLED=0 cross-build (linux/darwin/windows x amd64/arm64)
Go brand residual files: 0
git diff --check
```

## 参考治理

- Go `os.Root` 是本批 resolve-beneath 实现基础；没有复制参考仓库 runtime。
- Reames Lite 的工具执行 pipeline 与 metadata-only receipt 纪律用于约束“prepare → execute → receipt”顺序和 sidecar 数据最小化。
- Reasonix/Codex 仅作为 session/tool lifecycle 对照；实现继续落在 Reames `agent`、`control`、`tool`、`checkpoint` 统一边界，未引入平行 orchestration。

## 剩余风险

- `AtomicWriteFile` 在 Windows cross-device/filter-driver fallback 下可能原地复制；effects/meta/transcript/runtime 各自原子不等于跨文件断电事务。
- Rooted writer 只覆盖可预览 built-in file tools；shell、MCP 和 external API 必须重新读取真实状态，不能自动重放。
- Checkpoint 恢复 bytes/mode，不恢复 ACL、xattr 或硬链接身份。
- Journal 恢复的是 mutation boundary 和 cursor，不是完整跨进程 evidence ledger；root 项目检查仍须由 root tool call 重新执行并锚定。
