# 下一位模型接手交接文档

日期：2026-07-14

仓库：`F:\reames-agent`

分支：`main`

本页只记录当前接手边界。工作树、`docs/PROJECT.md`、`docs/DEVELOPMENT_PLAN.md` 和最新远端 CI 比本文更权威。

## 用户目标与工作节奏

持续大步推进到高可信可交付状态。参考 DeepSeek Reasonix、`F:\code-reference` 和 `F:\Reames-Lite` 时只吸收适用机制；每批同步代码、测试、文档和证据，充分本地验证后集中 commit/push，避免碎片 push 重复消耗 CI。

单元/合同测试、localhost fixture、原生 Desktop、远端 candidate、真实 Provider/IM/云节点证据必须分层表述，不能互相冒充。

## 受保护路径

以下未跟踪路径属于用户或其他会话，禁止修改、暂存或提交：

```text
.agents/
artifacts/
docs/audits/2026-07-09-reference-feature-gap-map.md
```

只使用显式 `git add -- <paths>`，禁止 `git add .` 和 `git add -A`。

## 已交付基线

- 当前 HEAD 与 `origin/main` 均为 `cb6c980 feat: fail closed on writer persistence errors`。
- 该远端基线的 CI run `29293474205` 为 8/8，CodeQL run `29293474123` 为 3/3。
- M0、M1、M2、M3 已按路线图关闭。M6 已有本地 Gateway、备份/恢复和 updater rollback 确定性证据；真实 linger logout/reboot、干净云节点、真实飞书和公开签名 release 仍是 `external-blocked`。
- M4 进行中。会话运行态恢复、共享委派预算、writable effects、durable evidence 与 writer persistence 已远端交付；当前未提交批次关闭后台 Task、compaction 与 memory 统一恢复子项，但不等于整个 M4 完成。

## 当前未提交 M4 批次

核心变化：

- `Agent.Options.SessionSync` 在初始 user、steer/retry/nudge、assistant tool-call envelope、tool result、final 与 compaction rewrite 边界持久化 isolated subagent transcript；同步失败会在进入下一 Provider/tool round 前停止。
- tool-call envelope 必须在工具执行前落盘，恢复框架不自动重放可能有副作用的调用。
- `SubagentStore.SaveRunning` 先发布 running metadata 再保存 transcript；completed 只在 transcript 成功保存后发布。元数据使用 `AtomicWriteFile` fsync 临时文件并原子替换。
- `continue_from` 只接受 compatible completed/interrupted transcript；running、failed 和未知状态均拒绝。interrupted 续跑注入恢复上下文，要求先重验 workspace/外部状态。
- `jobs.Manager.StartRecoverableForSession` 只有 running job metadata 成功落盘后才启动 goroutine；冷启动把遗留 running job 转为 `interrupted` tombstone，保留 partial log、subagent ref、completion note 和继续指引。
- compaction digest 可跨 running → interrupted → continue 恢复；memory 统一测试覆盖 BM25 `score/path/snippet`、空 store 禁用、归档删除后不命中和动态结果不污染稳定 system prefix。

主要文件：

```text
internal/agent/agent.go
internal/agent/task.go
internal/agent/parallel_tasks.go
internal/agent/subagent_store.go
internal/agent/subagent_store_test.go
internal/agent/session_sync_test.go
internal/agent/task_recovery_test.go
internal/agent/memory_recovery_test.go
internal/jobs/jobs.go
internal/jobs/artifacts.go
internal/jobs/artifacts_test.go
internal/jobs/jobs_extra_test.go
internal/tool/builtin/bgjobs.go
```

审计：`docs/audits/2026-07-14-m4-task-compaction-memory-recovery.md`。

## 当前本地证据

以下门禁已通过：

```text
go build ./...
go vet ./...
go test ./internal/... -count=1 -timeout 600s
go test -race ./internal/agent ./internal/jobs ./internal/memory ./internal/tool/builtin -count=1 -timeout 900s
Desktop go vet/test
Frontend corepack pnpm test:all/build
111 个 Python 测试通过（2 skipped），Node/工具/docs/public/deploy/release 合同通过
scripts/verify-baseline.ps1 -SkipDesktop -SkipFrontendHint
六目标 CGO_ENABLED=0 交叉编译全部 exit 0
git diff --check
```

六目标为 linux/amd64、linux/arm64、darwin/amd64、darwin/arm64、windows/amd64、windows/arm64；产物位于系统临时目录。代码最后补强了 job/subagent metadata 的 fsync 写入并保持原 `0600` 权限，随后聚焦测试、root build/vet、文档合同和 diff 检查再次通过；完整 root internal/race、综合 baseline 与六目标构建也在本批最终实现上通过。远端 CI/CodeQL 尚待集中 push 后验证。

## 未关闭边界

- arbitrary shell、MCP 和外部 API 没有 exactly-once 语义；pending tool call 可能未执行、部分执行或已完成，续跑必须核验真实状态。
- subagent transcript metadata 与 job metadata 是两个原子文件，不是单一跨文件事务；极端断电仍可能留下 orphan interrupted ref。
- child effects/evidence/budget 仍含进程内桥接，恢复 transcript 不等于恢复 parent evidence；跨 continuation 的 root 项目检查必须重跑。
- `AtomicWriteFile` 的 Windows cross-device fallback、handle-relative no-reparse 写入、transcript/runtime/workspace 跨资源断电事务仍未关闭。
- 本批冷启动证据是 persisted fixture + 新 Manager/Store，不是 OS 强杀、真实断电或真实外部服务演练。

## 下一执行顺序

1. 全面检查 diff 与受保护路径，显式暂存本批文件并形成一个大提交。
2. 集中 push 一次，守候并修复 CI/CodeQL；不要为了单独回填 run ID 立即制造碎片 push。
3. 转入 M4 剩余高风险项：handle-relative no-reparse/resolve-beneath 路径安全、durable child effect journal、transcript/runtime/workspace 跨资源断电恢复。
4. 外部环境可用时并行关闭干净云节点、真实飞书和公开签名 release；不可用时保持 `external-blocked`，继续本地 M4。

长期 GOAL 尚未完成。本批全绿只允许声明后台 Task/compaction/memory 统一恢复子项验收完成，不能声明整个 M4 或项目完成。
