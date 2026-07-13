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

- 本批起点 HEAD：`7757602 feat: persist goal verification references`；该提交与本批 writer persistence 在本审计时均未 push。最近远端基线 `73c155f` 的 CI run `29289973158` 8/8、CodeQL run `29289973160` 3/3 全绿。
- M0、M1、M2、M3 已按路线图关闭。最近安装器级 M3 证据：commit `68218d6`，CI `29262192635` 8/8、CodeQL `29262193090` 3/3；candidate `29262541971` 的 Linux/macOS installed 与 Windows native/interaction/accessibility 通过。
- M6 的 Gateway setup、credential-free preflight、WSL systemd 登录会话生命周期、备份/恢复和 updater rollback 已有确定性本地证据；真实 linger logout/reboot、干净云节点、真实飞书和公开签名 release 仍是 `external-blocked`。
- M4 进行中。会话运行态恢复、共享委派预算、writable effects 已在远端交付；durable evidence 已本地提交，当前未提交批次关闭 previewable writer persistence 门禁。两批均不等于整个 M4 完成。

## 当前未提交 M4 批次

核心变化：

- Checkpoint `Begin/Snapshot` 现在返回错误；allocation/record 失败不开放 active turn，Snapshot 失败回滚内存 `seen/Files`，可在存储恢复后真正重试。
- Agent pre-edit 与 ancestor effects callback 改为可失败门禁；Preview、checkpoint 或祖先 callback 失败均返回 blocked tool result，writer 不执行。
- Root previewable writer 还要求 runtime sidecar 与当前 session in-flight marker 成功；marker 必须由当前 turn 成功写入并匹配 session、message boundary 与 `preserveUser`，旧 marker 不能误放行。整个门禁和 autosave recovery/session rebind 共用 `snapshotMu`，避免资源落到不同 branch。
- 真实 builtin `write_file` 已覆盖 checkpoint 目录异常、runtime sidecar 故障和 in-flight marker 故障；marker 故障还预置了旧 turn marker，三类场景目标文件均未产生。
- 持久 checkpoint 在模拟 writer 部分写入、进程消失后由新 Store 恢复原字节。该证据不等于自动恢复或 transcript/runtime/workspace 断电原子性。
- `bash`/opaque writer 的静态目标未知，后台 Task durable journal、handle-relative 路径安全与跨资源事务仍未关闭。

审计：`docs/audits/2026-07-14-m4-writer-persistence-gate.md`。

主要修改文件：

```text
internal/agent/{agent.go,subagent_effects.go,subagent_effects_test.go}
internal/checkpoint/{checkpoint.go,checkpoint_test.go}
internal/control/{approval_e2e_test.go,checkpoint.go,checkpoint_test.go,controller.go,goal.go}
docs/ARCHITECTURE.md
docs/PROJECT.md
docs/DEVELOPMENT_PLAN.md
docs/THREAT_MODEL.md
docs/GUIDE.md
docs/GUIDE.zh-CN.md
docs/SPEC.md
docs/DOCS_INDEX.md
docs/NEXT_MODEL_HANDOFF.zh-CN.md
docs/audits/2026-07-14-m4-writer-persistence-gate.md
```

## 当前本地证据

当前未提交 writer persistence 批次已通过完整本地门禁：

```text
go build ./... && go vet ./...
go test ./internal/... -count=1 -timeout 600s
go test -race ./internal/agent ./internal/evidence ./internal/checkpoint ./internal/control ./internal/board -count=1 -timeout 900s
Desktop go vet/test
前端 corepack pnpm test:all/build
111 个 Python 测试（2 skipped）、Node/工具/docs/public/deploy/release 合同
六目标 CGO_ENABLED=0 交叉编译（系统 TEMP）
scripts/verify-baseline.ps1 -SkipDesktop -SkipFrontendHint（系统 TEMP 报告）
Go 品牌残留计数 0
git diff --check
```

stale-marker 修正后已重跑 root build/vet/internal/race，并在提交前复验受影响的 Desktop、交叉编译和综合 baseline；前端与 Python/Node 合同对应目录在修正后无代码变化。两批当前远端 CI/CodeQL 均待集中 push 后验证。

## 下一执行顺序

1. 显式暂存并提交 writer persistence 批次，不纳入 `.agents/`、`artifacts/` 或参考 gap map。
2. 将 `7757602` 与 writer persistence 两个实质提交集中 push，一次性守候并修复 CI/CodeQL。
3. 完成后台 Task crash-resume、compaction 与记忆检索统一恢复。
4. 继续处理 handle-relative 路径安全与跨资源断电事务，不把进程内回滚描述为 crash-safe。
5. 外部环境可用时并行关闭干净云节点、真实飞书和签名 release；不可用时保持 `external-blocked`，不要停止本地 M4。

长期 GOAL 尚未完成。本批全绿只允许声明 previewable writer persistence 子项验收完成，不能声明 M4 或整个项目完成。
