# M4 会话运行态与 Checkpoint 恢复审计

日期：2026-07-14

状态：M4 第一批已由远端 CI/CodeQL 验证；本文不声明整个 M4 完成。

## 问题

旧 Goal 运行态只在部分终态写入 sidecar，PlanMode、canonical Todo、completion intercept、blocker streak、idle/self-check 和 message boundary 没有统一恢复。非 strict Goal 还能用第二次 `[goal:complete]` 越过未完成 Todo，并由宿主生成 `goal-final` 事件强制完成列表。Branch/Switch/Rewind 因此可能把来源分支的 Goal/Plan 叠加到目标会话；`RewindBoth` 也可能先改文件，再发现 conversation boundary 已因 compaction 失效。

Checkpoint 文件恢复原先逐文件直接写入：后续文件失败会留下前面文件已回退的半完成状态；lexical workspace 检查不能阻止 symlink/reparse escape；JSON 持久化也不是 crash-safe 原子替换。

## 参考取舍

本批按 `REFERENCE_GOVERNANCE.md` 做机制级吸收：

| 参考 | 审计版本 | 吸收 | 未吸收 |
|---|---|---|---|
| Codex CLI | `dc23c7b` | revision/CAS 思路、root/child cancellation 与预算账本方向 | 本批没有声称已完成 Task 共享预算或完整 CAS 冲突 API |
| Scream Code | `e807673` | 分层 loop guard：blocker、idle、turn limit 独立计数 | 不复制其产品 UI 或 prompt 文案 |
| AgentArk | `4249c32` | fail-closed completion 与安全边界思路 | 不引入独立 verifier 服务 |
| DeepSeek Reasonix | `0e0cb63` | 保留现有 Go/Wails checkpoint 与 Goal 主路径 | 不沿用普通 `os.WriteFile` 恢复或模型自报即完成 |
| Reames Lite | `1230f78` | 旧契约中的恢复/证据边界作为兼容参考 | 不回迁 Python runtime/provider/cache |

Verifier/grader 自身出错不会被当作 PASS。引用只证明设计来源，不代替 Reames 本地行为测试。

## 已实现不变量

1. 所有 Goal 模式都必须通过 canonical Todo 和 project checks；第二次或更多 `[goal:complete]` 不能覆盖门控，也不能修改 Todo。
2. strict completion 必须来自实际宿主发起的 self-check turn。待自检状态 crash/resume 后，普通 completion 不能跳过自检。
3. 相同 blocker 的三次审计、50-turn limit 和两次 idle 检测跨 crash/resume 保留。
4. v2 sidecar 包含 Goal/Plan/Todo、continuation counters、message count、transcript digest 和 revision，并在每个完整 turn、history rewrite 与 Goal transition 后用 `AtomicWriteFile` 刷新。相同路径的进程内 revision 检查与替换处于同一临界区；transport 的 session lease 提供跨进程单写者边界。Windows cross-device fallback 的原地复制窗口不被描述为无条件 crash-safe。
5. 新 v2 Todo 在 transcript digest 与 message boundary 完全相等时直接恢复；append-only extension 从 sidecar Todo 基础重放后缀的成功 Todo 事件，compaction 已移除基础 `todo_write` 时仍保留任务。rewrite/divergence 则保留 transcript rebuild，压缩前更大的旧 count 不能覆盖压缩后又前进的 transcript；无 digest 的旧 v2 保留 count 兼容回退。
6. Resume/Switch/Branch/Fork/Rewind 使用 replace-style runtime；新建/清空会话以及没有 sidecar 的目标不会继承来源 Goal/Plan。
7. Checkpoint 在 visible turn 开始前锚定 transcript boundary、prefix digest 和 runtime projection。Conversation rewind 同时恢复 Goal/Plan/Todo，headless `Controller.Run` 也进入相同 orchestrator/checkpoint/Goal FSM 生命周期。
8. `RewindBoth`、Fork 和 Summarize 对 stale/compacted 或同长度 divergent transcript、legacy 无 digest及负边界 fail closed；Rewind/Fork 还会在非空 runtime 无效或来自未来格式时失败关闭。Branch/Fork 在 transcript 已保存而 runtime/meta 失败时清理全部 session artifacts 和未持有 lock。
9. 文件恢复在应用前验证全部路径和当前 disk state，拒绝 escape、symlink/reparse 与非普通文件；规范化 lexical/Windows case aliases，现存硬链接以 `os.SameFile` 共享 earliest bytes；中途失败反向恢复已应用文件和可能部分写入的当前失败目标，相对路径 Unix executable/zero mode 按 workspace root 随快照恢复。Checkpoint truncate 先持久化 tombstone，物理删除只是 GC，失败后重启不会复活未来 turn；turn ID 保持单调。已有 manifest 损坏时旧 checkpoint 整体 fail closed，下一 turn 先退休旧 ID 区间再修复 manifest。
10. Board 只把 `status=running` 的 Goal 标为 active；展示真实当前 turn evidence 摘要，但不把摘要升级为 durable proof。

## 回归覆盖

- `internal/control/goal_test.go`：重复 completion、blocked audit、session rotation。
- `internal/agent/session_test.go`：transcript anchor 忽略 leading system prompt 刷新，并区分相等、append-only 与 divergence。
- `internal/control/goal_state_dto_test.go`：v0/v1/v2 兼容、旧 revision、并发 Controller revision 顺序、blocker/intercept/turn/idle/self-check crash-resume。
- `internal/control/controller_test.go`：compaction Todo 恢复、append-only Todo 后缀重放、pre-compaction 大 count 分叉拒绝、每 turn/history rewrite runtime 锚点刷新、headless lifecycle。
- `internal/control/turn_orchestrator_test.go`：PromptSubmit hook 拦截 synthetic strict self-check 时不复用上一轮 completion，并重新排队自检。
- `internal/control/branches_test.go`：root/child Goal 与 PlanMode 隔离，以及 Branch/Fork post-save meta 失败完整清理。
- `internal/control/rewind_e2e_test.go`：Rewind/Fork/Summarize 的 stale/同长度 divergence/legacy/负边界门禁、无效 runtime fail closed，以及 runtime rewind 与冷恢复。
- `internal/checkpoint/checkpoint_test.go`：runtime/digest payload、invalid record load、durable truncate tombstone、单调 turn、symlink escape、路径/硬链接 alias、第二文件及当前失败文件回滚、绝对/相对路径 Unix mode。
- `internal/board/board_test.go`：blocked Goal 和 Controller evidence 投影。

聚焦证据：

```text
go test ./internal/control -count=1 -timeout 300s
go test ./internal/checkpoint ./internal/board ./internal/evidence ./internal/agent -count=1 -timeout 300s
go test -race ./internal/control ./internal/agent ./internal/checkpoint ./internal/evidence ./internal/board -count=1 -timeout 900s
go build ./...
go vet ./...
go test ./internal/... -count=1 -timeout 600s
python scripts/smoke_gateway_headless.py --out "$env:TEMP\reames-agent-headless-gateway-smoke-m4.json"
git diff --check
```

上述命令及其扩展门禁已在本轮新增 hardening 后重新通过：`go build ./...`、`go vet ./...`、`go test ./internal/...`、五个恢复核心包 race、Desktop Go 全测、前端 `pnpm test:all` 与 production build、111 个 Python 测试（2 skipped）、Node upstream 合同、builtin tool 合同、docs/public/deploy/release 合同和六目标 `CGO_ENABLED=0` 交叉编译。修订后的 `verify-baseline.ps1 -SkipDesktop -SkipFrontendHint` 也实际通过，schema v2 headless Gateway 报告写入系统 TEMP 而非仓库 `artifacts/`。commit `f35e0af` 的远端 CI `29285435177` 8/8、CodeQL `29285435130` 3/3 均成功。

## 未关闭边界

- writable 子代理的进程内 effects 归并已在 `2026-07-14-m4-writable-subagent-effects.md` 关闭；跨 turn/crash 的 durable effect journal 仍未实现。
- 跨 continuation 的成功读取/验证循环检测与 durable evidence 最小引用。
- runtime/checkpoint 持久化失败向 writer 的 fail-closed 传播和断电窗口。
- RestoreCode 的 handle-relative no-reparse/resolve-beneath 写入、`RewindBoth` 跨 transcript/runtime/workspace durable journal，以及 ACL/xattr/硬链接身份恢复。
- 后台 Task crash-resume 与 compaction 后恢复。
- 干净云节点、真实 IM、公开签名 release 仍分别需要外部环境，保持 `external-blocked`，不影响继续推进本地 M4。
