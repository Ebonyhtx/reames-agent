# M4 后台 Task、Compaction 与记忆统一恢复审计

日期：2026-07-14

状态：实现、故障注入、完整本地门禁和六目标 `CGO_ENABLED=0` 交叉编译通过；远端 CI/CodeQL 待本批集中 push 后验证。本文关闭后台 Task transcript/job tombstone、compaction continuation 与 memory recall 的统一恢复子项，不声明 arbitrary shell exactly-once、durable child effect journal、跨资源断电事务或整个 M4 完成。

## 问题

原 `jobs.Manager` 只在 job 结束时写 metadata，进程崩溃会留下无法识别的 `.log`；`SubagentStore` 虽会把 stale running ref 改成 `interrupted`，却禁止继续，而且 transcript 只在 completed/failed 时保存。Subagent compaction 只改内存 session，崩溃后既无法知道最后 durable provider/tool boundary，也无法把 job tombstone 与 `continue_from` 关联。记忆检索已有 BM25 与归档能力，但缺少和恢复链路一起证明“解释字段存在、可关闭、删除后不命中、动态结果不进入稳定 system prefix”的统一证据。

## 参考取舍

| 参考 | 审计版本 | 吸收 | 未吸收 |
|---|---|---|---|
| DeepSeek Reasonix | `0e0cb63c712e` | 保留持久 subagent identity、`continue_from`、启动时 stale-running→interrupted tombstone 与 cache-first compaction 主路径 | 不保留“interrupted 永久不可继续”和“只在终态保存 transcript”的限制 |
| Codex CLI | `dc23c7bcc8fd` | rollout-backed child thread、持久 metadata 与显式 resume 的分离思路 | 不复制 Rust agent graph/tree 自动恢复，也不自动恢复或重放崩溃中的工具 |
| Hermes | `88a58ff1355e` | 长任务增量 checkpoint 后再声明完成、恢复时只跳过已确认完成工作 | 不采用 checkpoint 写失败后继续执行的 best-effort 语义；recoverable job metadata 失败必须不启动 |
| Reames Lite | `1230f781cf10` | 用户控制的 task 状态机、原子 progress/task 快照与恢复后继续的产品契约 | 不迁回 Python runtime、raw transcript export 或由状态百分比推断工具副作用 |

引用说明机制来源；完成证据来自本仓库实现与测试。

## 已实现不变量

1. `Agent.Options.SessionSync` 只为 isolated/persisted subagent 注入。Agent 在初始 user message、steer、stream recovery、assistant tool-call envelope、tool results、readiness retry、budget nudge、final 和 compaction rewrite 后调用；callback 失败会在下一 Provider/tool round 前停止。
2. Assistant tool-call envelope 必须在 `Execute` 前保存。崩溃中的工具不会由框架自动重放；恢复 transcript 只表示“该调用已请求”，不证明副作用是否发生。
3. `SubagentStore.SaveRunning` 先把 metadata 发布为 running，再保存 transcript；终态只在 transcript 保存后发布。任一中途崩溃都会保守地落入 stale running/interrupted，而不会让新工作挂在旧 completed 状态下。
4. `PrepareContinue` 只允许 compatible completed/interrupted ref，仍拒绝 running/failed，并继续验证 parent owner、workspace、persona、tool scope/schema、model 与 effort。Interrupted continuation 注入恢复上下文，要求重读 workspace/外部状态后再决定是否重复工作。
5. `jobs.Manager.StartRecoverableForSession` 只有 job log 和 running metadata 成功落盘后才启动 goroutine；metadata 携带 subagent ref。冷加载 running metadata 时写回 `interrupted`、关闭 done channel、保留 partial log、生成 `continue_from` 指引并加入下一 turn 的 background completion note。
6. Subagent compaction rewrite 经同一 SessionSync 落盘；running→startup cleanup→interrupted→PrepareContinue 后仍能读取 compacted digest。
7. Memory BM25 tool result显式包含 score、path、snippet；零值 store 返回 unavailable，归档删除后 active search 不再命中。命中只作为 tool result 进入后续 request，两个 Provider round 的 system message 字节保持不变。

## 回归覆盖

- `internal/agent/session_sync_test.go`：首个 Provider 前持久化失败、tool envelope→Execute→tool result 顺序、manual compaction rewrite 同步。
- `internal/agent/task_recovery_test.go`：真实后台 task 在 writer Execute 阻塞时从磁盘读到 tool-call envelope、interrupted 显式续跑恢复上下文、compacted transcript 跨冷启动续接。
- `internal/agent/subagent_store_test.go`：running 拒绝、startup interrupted 清理、durable transcript 继续、failed 保持拒绝、identity/owner 约束。
- `internal/jobs/artifacts_test.go`：running metadata 先于 goroutine、artifact 失败不启动、running 冷加载为 interrupted、partial log/ref/completion note 与 sequence 恢复。
- `internal/agent/memory_recovery_test.go`：score/path/snippet、删除、关闭与 system prefix 不变。

本批已执行：

```text
go build ./...
go vet ./...
go test ./internal/... -count=1 -timeout 600s
go test -race ./internal/agent ./internal/jobs ./internal/memory ./internal/tool/builtin -count=1 -timeout 900s
Desktop: go vet ./...
Desktop: go test . -count=1 -timeout 600s
Frontend: corepack pnpm test:all
Frontend: corepack pnpm build
python -m unittest discover -s scripts -p "test_*.py" -v
node scripts/test_upstream_watch_issue.mjs
python scripts/check_docs_contracts.py
python scripts/check_public_readiness.py
python scripts/check_deploy_contracts.py
python scripts/check_release_contracts.py
go test ./internal/tool -run TestBuiltinToolContractDocumentation -v
scripts/verify-baseline.ps1 -SkipDesktop -SkipFrontendHint
linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64, windows/arm64: CGO_ENABLED=0 go build
git diff --check
```

结果：root internal、受影响 race 包、Desktop、Frontend、工具/文档/公开/部署/发布合同和综合 baseline 均通过；Python 为 111 passed、2 skipped；六个发布目标均 exit 0。交叉编译产物写入系统临时目录，不纳入仓库。

## 未关闭边界

- 该协议不提供 arbitrary shell/MCP/外部 API exactly-once。崩溃中的工具可能未执行、部分执行或已完成，续跑必须重新核验真实状态。
- Subagent transcript metadata 与 job metadata 是两个原子文件，不是单一跨文件事务；两者均在副作用前写入，但极端断电仍可能产生只有 subagent ref 的 orphan interrupted artifact。Parent tool envelope 会保留为未完成边界，清理不会自动重放。
- Child effects/evidence/budget 仍为进程内桥接；恢复 transcript 不等于恢复 parent evidence，跨 continuation 的 root 项目检查必须重跑。
- `AtomicWriteFile`/`ReplaceFile` 的 Windows cross-device fallback、handle-relative no-reparse 写入、transcript/runtime/workspace 跨资源事务仍未关闭。
- 本批冷启动证据通过新 Manager/Store 和 persisted running fixture 模拟进程消失；不是 OS 强杀、断电或真实外部服务演练。远端 CI/CodeQL 仍需在集中 push 后补录。
