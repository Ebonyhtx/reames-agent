# 下一位模型接手交接文档

日期：2026-07-14

仓库：`F:\reames-agent`

分支：`main`

本页只记录当前接手边界。工作树、`docs/PROJECT.md`、`docs/DEVELOPMENT_PLAN.md` 和最新远端 CI 比本文更权威。

## 用户目标与工作节奏

持续大步骤推进到高可信可交付状态。参考 DeepSeek Reasonix、`F:\code-reference` 与 `F:\Reames-Lite` 时只吸收适用机制；每批同步代码、测试、文档和证据，充分本地验证后集中 commit/push，避免碎片 push 重复消耗 CI。

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

- 当前批开始 HEAD：`799eb6d feat: add recoverable gateway operations`，当时与 `origin/main` 一致。
- M0、M1、M2、M3 已按路线图关闭。最近安装器级 M3 证据：commit `68218d6`，CI `29262192635` 8/8、CodeQL `29262193090` 3/3；candidate `29262541971` 的 Linux/macOS installed 与 Windows native/interaction/accessibility 通过。
- M6 的 Gateway setup、credential-free preflight、WSL systemd 登录会话生命周期、备份/恢复和 updater rollback 已有确定性本地证据；真实 linger logout/reboot、干净云节点、真实飞书和公开签名 release 仍是 `external-blocked`。
- M4 已改为进行中。当前未提交批次关闭 Goal/Plan/Todo/Checkpoint 会话运行态恢复的第一批，不等于整个 M4 完成。

## 当前未提交 M4 批次

核心变化：

- 删除非 strict Goal 的第二次 completion override；宿主不再伪造 `goal-final` Todo 完成。所有模式都经过 canonical Todo、project checks 和 evidence gate；strict 额外要求实际宿主 self-check turn。
- v2 runtime sidecar 持久化 Goal、PlanMode、canonical Todo、message count、transcript digest、continuation/blocker/intercept/idle/self-check counters 和 monotonic revision；每个完整 turn、history rewrite 与 Goal transition 原子刷新，相同路径的进程内 revision 检查/替换串行化。
- v2 Todo 按 transcript anchor 选择来源：完全相等直接恢复 sidecar Todo，append-only extension 从 sidecar Todo 基础重放成功后缀事件，rewrite/divergence 保留 transcript rebuild；compaction 已移除基础 `todo_write` 也不会丢 Todo，无 digest 的旧 v2 才使用 count 回退。
- Resume/Switch/Branch/Fork/Rewind 整体替换运行态；root/child 分支不串写，新建/清空会话清除 Goal 和 PlanMode。
- Checkpoint 保存 turn-start runtime 与 transcript prefix digest；`RewindBoth`、Fork 和 Summarize 对 stale/同长度 divergence、legacy 无 digest及负边界 fail closed，Rewind/Fork 还拒绝无效 runtime；headless `Controller.Run` 也生成 checkpoint 并推进 Goal FSM。
- 文件恢复预检全部路径，拒绝 workspace escape、symlink/reparse 和非普通文件；path/case/hard-link alias 共享 earliest bytes，中途失败反向回滚已成功文件和可能部分写入的失败目标 bytes/mode。truncate tombstone 先持久化，物理删除失败不会在重启时复活，turn ID 不复用；Branch/Fork post-save 失败清理 transcript、runtime、meta、event 和 lock artifacts。
- `AtomicWriteFile` 的 Windows cross-device fallback、RestoreCode 路径复查后的 TOCTOU、跨 transcript/runtime/workspace 断电窗口、ACL/xattr/硬链接身份恢复均明确保留为残余风险。
- Board 只把 running Goal 标为 active，并读取当前 turn evidence 摘要，不把摘要声明为 durable proof。

审计：`docs/audits/2026-07-14-m4-session-runtime-recovery.md`。

主要修改文件：

```text
internal/agent/{agent.go,session.go}
internal/board/board.go
internal/checkpoint/checkpoint.go
internal/control/{goal.go,goal_state_dto.go,turn_orchestrator.go,controller.go,checkpoint.go,port.go}
internal/evidence/evidence.go
对应 *_test.go
docs/PROJECT.md
docs/DEVELOPMENT_PLAN.md
docs/ARCHITECTURE.md
docs/THREAT_MODEL.md
docs/GOAL_ENFORCEMENT.zh-CN.md
docs/CHECKPOINTS.md
docs/DOCS_INDEX.md
docs/NEXT_MODEL_HANDOFF.zh-CN.md
docs/audits/2026-07-14-m4-session-runtime-recovery.md
scripts/verify-baseline.ps1
CHANGELOG.md
```

## 当前本地证据

本轮新增 hardening 后已通过：

```text
go build ./...
go vet ./...
go test ./internal/... -count=1 -timeout 600s
go test -race ./internal/control ./internal/agent ./internal/checkpoint ./internal/evidence ./internal/board -count=1 -timeout 900s
Push-Location desktop; go test . -count=1 -timeout 300s; Pop-Location
Push-Location desktop/frontend; corepack pnpm test:all; corepack pnpm build; Pop-Location
python -m unittest discover -s scripts -p "test_*.py" -v  # 111 passed, 2 skipped
node scripts/test_upstream_watch_issue.mjs
python scripts/check_docs_contracts.py
python scripts/check_public_readiness.py
python scripts/check_deploy_contracts.py
python scripts/check_release_contracts.py
go test ./internal/tool -run TestBuiltinToolContractDocumentation -count=1 -v
# linux/windows/darwin x amd64/arm64, CGO_ENABLED=0: all built
.\scripts\verify-baseline.ps1 -SkipDesktop -SkipFrontendHint
```

`verify-baseline.ps1` 的实际 schema v2 headless Gateway 报告位于系统 TEMP，覆盖隔离 home、setup/doctor/service-plan、localhost Provider、session persistence 与 feedback redaction，并保持真实 Provider/IM/installed service manager 为 `external_blocked`；默认不写受保护 `artifacts/`。本地门禁完成后只显式暂存本批路径，集中 commit/push，再等待新 CI/CodeQL；历史 run 不能证明当前批。

## 下一执行顺序

1. 收敛当前 M4 diff、完成全量门禁、集中 commit/push 并修复远端 CI/CodeQL。
2. 为 `task` / `parallel_tasks` 建立父子共享并发、token/time/step 预算、root/child cancellation 和原子记账。
3. 结构化归并 writable 子代理的 checkpoint/evidence，覆盖 partial failure/cancel。
4. 建立跨 continuation 循环检测、durable evidence 最小引用、持久化 fail-closed 和后台 Task crash-resume。
5. 外部环境可用时并行关闭干净云节点、真实飞书和签名 release；不可用时保持 `external-blocked`，不要停止本地 M4。

长期 GOAL 尚未完成。本批全绿只允许声明第一批验收完成，不能声明 M4 或整个项目完成。
