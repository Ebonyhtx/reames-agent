# M4 Writer 持久化门禁审计

日期：2026-07-14

状态：本地实现、故障注入、进程中断恢复用例与完整本地门禁完成；当前批远端 CI/CodeQL 待集中 push 后补证据。本文只关闭 previewable writer 的写前持久化传播，不声明跨资源断电事务、后台 Task crash-resume 或 M4 完成。

## 问题

原 checkpoint `Begin`、`Snapshot` 和 runtime sidecar `writeState` 在磁盘写失败时只记日志。Agent 的 pre-edit callback 没有错误通道，因此 allocation manifest、turn/runtime record、文件快照或 in-flight marker 丢失后，已获审批的 writer 仍可修改工作区。Snapshot 还会先更新内存 `seen/Files`：若持久化失败，重试可能把未落盘状态误认为已经保存。

## 参考取舍

| 参考 | 审计版本 | 吸收 | 未吸收 |
|---|---|---|---|
| DeepSeek Reasonix | `0e0cb63c712e` | 保留 Previewer → pre-edit callback → checkpoint 的 Go 主路径 | 不沿用 void callback 与日志后继续 writer 的 best-effort 语义 |
| Codex CLI | `dc23c7bcc8fd` | 写前 durable state、恢复时 fail closed 和 turn 生命周期串行思路 | 不复制 Rust rollout/app-server 协议或宣称跨文件原子事务 |
| Reames Lite | `1230f781cf10` | 旧版受控写入、恢复证据与中断可诊断契约 | 不迁回 Python runtime，也不从模型文本推断 checkpoint 成功 |

引用说明机制来源；完成证据来自本仓库实现与测试。

## 已实现不变量

1. `checkpoint.Store.Begin/BeginAnchored/Snapshot/SnapshotForTurn` 返回错误。只有 allocation state 和 turn/runtime record 都成功后才开放 active turn；迟到 child turn、无 active turn、路径逃逸或空路径均拒绝。
2. Snapshot record 失败会删除本次 `seen`、`seenFiles` 与 `Files` 追加；修复存储后同一变化可真正重试。即使底层写入报告失败前产生部分 record，writer 仍不执行。
3. `agent.PreEditHook` 与 ancestor effects callback 可返回错误。Previewer 自身失败、root hook 失败或任一祖先 hook 失败时，Agent 返回 blocked tool result，不调用 writer；只有所有写前条件成功才设置 mutation-attempt 边界并进入 `Execute`。
4. Root writer 依次要求 durable checkpoint、最新 runtime sidecar 和当前 session 的 in-flight turn marker。Controller 只接纳本次成功写入且与当前 session、message boundary、`preserveUser` 完全相等的 marker；旧 turn 遗留 marker 不能在新 marker 写失败时误放行 writer。整个门禁持有与 autosave conflict recovery、Resume/Switch/New/session rebind 相同的 `snapshotMu`，三个资源不会因进程内换 branch 分落到旧、新路径。
5. 持久 checkpoint 在模拟 writer 已部分覆盖目标、进程未执行清理的场景下，由新构造的 Store 加载并恢复原字节。该证据证明 crash 后可回退材料存在，不代表自动回退或断电原子性。

## 回归覆盖

- `internal/checkpoint/checkpoint_test.go`：allocation 写失败无 active turn、record 失败内存/磁盘回滚与成功重试、路径逃逸双侧拒绝、进程重建后恢复部分 writer。
- `internal/agent/subagent_effects_test.go`：root 与 ancestor callback 失败均不执行 writer、不触碰磁盘。
- `internal/control/approval_e2e_test.go`：真实 builtin `write_file` 在 checkpoint、runtime sidecar、in-flight marker 三类故障下注入失败；marker 场景预置旧 turn marker 再令当前写入失败，目标仍不存在且 tool result 标明失败阶段。
- `internal/control/checkpoint_test.go`：晚到 child callback 返回错误，当前 turn snapshot 仍正常。

当前批已执行：

```text
go build ./...
go vet ./...
go test ./internal/... -count=1 -timeout 600s
go test -race ./internal/agent ./internal/evidence ./internal/checkpoint ./internal/control ./internal/board -count=1 -timeout 900s
(desktop) go vet ./... && go test . -count=1 -timeout 600s
(desktop/frontend) corepack pnpm test:all && corepack pnpm build
python -m unittest discover -s scripts -p "test_*.py" -v  # 111 passed, 2 skipped
node scripts/test_upstream_watch_issue.mjs
go test ./internal/tool -run TestBuiltinToolContractDocumentation -v -count=1
python scripts/check_docs_contracts.py
python scripts/check_public_readiness.py
python scripts/check_deploy_contracts.py
python scripts/check_release_contracts.py
六目标 CGO_ENABLED=0 交叉编译（产物写入系统 TEMP）
powershell -File scripts/verify-baseline.ps1 -SkipDesktop -SkipFrontendHint -OutputDir <system-temp>
Go 品牌残留文件计数 0
git diff --check
```

其中 stale-marker 修正后重新执行了 root build/vet/internal 全测与五个恢复核心包 race；提交前又复验 Desktop、六目标交叉编译、baseline、文档/公开合同和差异卫生。前端与 Python/Node 合同对应目录在修正后无代码变化。上述均为本地证据，不能替代待 push commit 的远端 CI/CodeQL。

## 未关闭边界

- `bash` 和不实现 `tool.Previewer` 的外部 writer 无法提供静态目标，仍没有逐文件 checkpoint；权限、沙箱、evidence 与事后磁盘核验继续承担边界。
- 后台 Task 的进程重启、durable child effect journal、compaction 后 child receipt 恢复仍未关闭。
- In-flight marker 用于识别并清理 partial transcript；当前不会在启动时自动回退任意工作区变化，用户仍通过 checkpoint rewind 选择恢复。
- `AtomicWriteFile` Windows fallback、handle-relative no-reparse 写入、ACL/xattr/硬链接身份，以及 transcript/runtime/workspace 跨资源断电事务仍未关闭。
