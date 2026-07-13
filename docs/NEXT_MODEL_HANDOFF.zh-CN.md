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

- 当前 HEAD：`73c155f docs: index M4 delivery audits`，与 `origin/main` 一致。远端 CI run `29289973158` 8/8、CodeQL run `29289973160` 3/3 全绿。
- M0、M1、M2、M3 已按路线图关闭。最近安装器级 M3 证据：commit `68218d6`，CI `29262192635` 8/8、CodeQL `29262193090` 3/3；candidate `29262541971` 的 Linux/macOS installed 与 Windows native/interaction/accessibility 通过。
- M6 的 Gateway setup、credential-free preflight、WSL systemd 登录会话生命周期、备份/恢复和 updater rollback 已有确定性本地证据；真实 linger logout/reboot、干净云节点、真实飞书和公开签名 release 仍是 `external-blocked`。
- M4 进行中。已交付 Goal/Plan/Todo/Checkpoint 会话恢复、共享委派预算和 writable effects 三批；当前未提交批次关闭跨 continuation 的最小 durable evidence 引用，不等于整个 M4 完成。

## 当前未提交 M4 批次

核心变化：

- `Receipt` 增加稳定 `ToolCallID`；v2 runtime sidecar 只保存最新 writer epoch 的 `WritePending`、项目检查命令 SHA-256 与 root bash tool-call ID，不保存命令正文、工具输出或模型文本。
- 成功 writer 或带 `MutationAttempt` 的失败/取消 writer 清空旧引用；同一检查的最新失败覆盖旧成功，后续成功才能恢复。Final readiness 优先使用本 turn 最新结果，本 turn 未运行时才读取 durable ref。
- Ledger `Rotate` 在同一锁内返回旧 receipt、清空并推进 generation，避免后台 receipt 落在 snapshot/reset 缝隙。新 Goal 同时清空 durable state 与上一普通 turn ledger，不会重新吸收无关证据。
- 恢复只接受 exact transcript count/digest；引用还必须解析到最新 writer 后该检查最新一次成功 tool result。Append/rewrite/divergence、丢失/失败 ID 或后续 writer 都 fail closed。
- Child writer 会使父旧引用失效；child-only bash 不在 parent transcript，只能满足当前 turn，跨 continuation 需 root 重验。
- Runtime/checkpoint 写失败向 writer fail closed、完整 child effect journal 与后台 Task crash-resume 仍未关闭。

审计：`docs/audits/2026-07-14-m4-durable-evidence.md`。

主要修改文件：

```text
internal/agent/{agent.go,final_readiness_test.go}
internal/control/{goal.go,goal_state_dto.go,turn_orchestrator.go,controller.go,goal_state_dto_test.go}
internal/evidence/evidence.go
internal/evidence/evidence_test.go
docs/PROJECT.md
docs/DEVELOPMENT_PLAN.md
docs/THREAT_MODEL.md
docs/GOAL_ENFORCEMENT.zh-CN.md
docs/GUIDE.md
docs/GUIDE.zh-CN.md
docs/SPEC.md
docs/DOCS_INDEX.md
docs/NEXT_MODEL_HANDOFF.zh-CN.md
docs/audits/2026-07-14-m4-durable-evidence.md
```

## 当前本地证据

当前未提交批次已通过完整本地门禁：

```text
go test ./internal/evidence ./internal/agent ./internal/control -count=1 -timeout 600s
go vet ./internal/evidence ./internal/agent ./internal/control
python scripts/check_docs_contracts.py
python scripts/check_public_readiness.py
git diff --check
```

此外已完成 root build/vet/internal 全测、恢复核心 race、Desktop Go vet/test、前端 `test:all`/production build/bundle budget、Python（111 passed、2 skipped）、Node upstream、builtin tool、docs/public/deploy/release 合同、六目标 `CGO_ENABLED=0` 交叉编译、`verify-baseline.ps1 -SkipDesktop -SkipFrontendHint` 和品牌残留检查（0）。当前批远端 CI/CodeQL 仍待与下一实质提交集中 push 后验证；历史远端 run 不能证明当前未提交批次。

## 下一执行顺序

1. 对当前 durable evidence 批次执行 race 与完整本地门禁，集中 commit；继续积累 writer fail-closed 实质成果后再统一 push，避免为单个本地批次重复消耗 CI。
2. 将 checkpoint/runtime 持久化失败向 writer fail closed 传播，并覆盖进程中断窗口。
3. 完成后台 Task crash-resume、compaction 与记忆检索统一恢复。
4. 继续处理 handle-relative 路径安全与跨资源断电事务，不把进程内回滚描述为 crash-safe。
5. 外部环境可用时并行关闭干净云节点、真实飞书和签名 release；不可用时保持 `external-blocked`，不要停止本地 M4。

长期 GOAL 尚未完成。本批全绿只允许声明 durable evidence 子项验收完成，不能声明 M4 或整个项目完成。
