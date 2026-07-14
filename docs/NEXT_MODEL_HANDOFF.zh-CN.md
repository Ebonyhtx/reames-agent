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

- 当前 HEAD 与 `origin/main` 均为 `1962fef feat: recover interrupted background tasks`。
- 该远端基线的 CI run `29296154559` 与 CodeQL run `29296154523` 均为 `success`。
- M0、M1、M2、M3 已按路线图关闭。M6 已有本地 Gateway、备份/恢复和 updater rollback 确定性证据；真实 linger logout/reboot、干净云节点、真实飞书和公开签名 release 仍是 `external-blocked`。
- M4 进行中。会话运行态恢复、共享委派预算、writable effects、durable evidence、writer persistence 与后台 Task/compaction/memory 已远端交付；当前未提交批次关闭 rooted built-in writer/checkpoint 和 durable child effect journal 两个子项，但不等于整个 M4 完成。

## 当前未提交 M4 批次

核心变化：

- 新增 `fileutil.AtomicWriteRootFile` 和 rooted target：previewable built-in writer 的 read/stat/temp-write/fsync/chmod/rename/remove 均通过打开的 `os.Root` + 相对路径完成，拒绝路径逃逸和 symlink/reparse 组件替换。
- `write_file`、`edit_file`、`multi_edit`、`delete_range`、`delete_symbol`、`notebook_edit`、`move_file`、`apply_patch` 全部迁移；旧 path-based writer helper 已删除。
- `tool.MultiPreviewer` 让 Agent 在 multi-file writer 执行前快照所有 change；`move_file` 预览 source delete + destination create，父 checkpoint 可同时恢复两端。
- `apply_patch` 现在支持 validated create/update/delete unified diff、完整 preflight、dry-run、逐文件原子替换和后续失败回滚；隐式 rename 被拒绝并要求使用 `move_file`。
- `checkpoint.Store.RestoreCode` 的预检、删除、写入与失败回滚复用单一 workspace `os.Root`，post-preflight symlink swap 测试证明 root 外文件不受影响。
- 顶层持久 `task`/writer-capable `run_skill` 使用 `*.effects.json` v1 sidecar：0600、最多 256 retained events/1 MiB，按序列化字节保守压缩；不保存 model text、tool output、args/Todo/step，command 使用现有 secret redaction。
- Previewable child writer 在执行前先写 mutation intent；journal 写失败则不执行。结果 receipt 在祖先 ledger 发布前落盘。Runtime sidecar 保存 `{ref,journalID,sequence}` cursor，恢复校验 parent session/workspace、`task`/`run_skill` call anchor 和 journal identity。
- Child mutation 会清空旧 root checks；child-only bash 仍不成为跨 continuation root proof。已确认 cursor 可在 parent transcript compaction 后幂等跳过；损坏、stale anchor、替换、重复重放、分支 crossing 和后台跨 turn 均有测试。

本批允许显式暂存的完整文件清单（受保护路径不在其中）：

```text
.github/workflows/ci.yml
desktop/sessions.go
docs/ARCHITECTURE.md
docs/CHECKPOINTS.md
docs/DEVELOPMENT_PLAN.md
docs/DOCS_INDEX.md
docs/GOAL_ENFORCEMENT.zh-CN.md
docs/NEXT_MODEL_HANDOFF.zh-CN.md
docs/PROJECT.md
docs/THREAT_MODEL.md
docs/TOOL_CONTRACT.md
docs/TOOL_CONTRACT.zh-CN.md
docs/audits/2026-07-14-m4-session-runtime-recovery.md
docs/audits/2026-07-14-m4-rooted-writers-child-effects.md
internal/agent/agent.go
internal/agent/migrate.go
internal/agent/migrate_test.go
internal/agent/subagent_cleanup_test.go
internal/agent/subagent_effect_journal.go
internal/agent/subagent_effect_journal_test.go
internal/agent/subagent_effects.go
internal/agent/subagent_effects_test.go
internal/agent/subagent_store.go
internal/agent/task.go
internal/boot/boot.go
internal/checkpoint/checkpoint.go
internal/checkpoint/checkpoint_test.go
internal/cli/toolcard.go
internal/config/config.go
internal/config/render.go
internal/control/branches_test.go
internal/control/controller.go
internal/control/controller_test.go
internal/control/goal.go
internal/control/goal_state_dto_test.go
internal/control/session_store.go
internal/control/turn_orchestrator.go
internal/control/turn_orchestrator_test.go
internal/evidence/evidence.go
internal/fileutil/rootwrite.go
internal/fileutil/rootwrite_test.go
internal/permission/permission.go
internal/permission/permission_extra_test.go
internal/tool/builtin/applypatch.go
internal/tool/builtin/applypatch_test.go
internal/tool/builtin/confine.go
internal/tool/builtin/delete_range.go
internal/tool/builtin/delete_symbol.go
internal/tool/builtin/editfile.go
internal/tool/builtin/encoding_helpers.go
internal/tool/builtin/movefile.go
internal/tool/builtin/multiedit.go
internal/tool/builtin/notebookedit.go
internal/tool/builtin/preview.go
internal/tool/builtin/preview_test.go
internal/tool/builtin/rooted_target.go
internal/tool/builtin/rooted_target_test.go
internal/tool/builtin/session_guard_test.go
internal/tool/builtin/workspace.go
internal/tool/builtin/writefile.go
internal/tool/tool.go
scripts/test_verify_baseline.py
scripts/verify-baseline.ps1
```

审计：`docs/audits/2026-07-14-m4-rooted-writers-child-effects.md`。

## 当前本地证据

以下门禁已通过：

```text
go build ./...
go vet ./...
go test ./internal/... -count=1 -timeout 600s
go test -race ./internal/agent ./internal/control ./internal/checkpoint ./internal/fileutil ./internal/tool/builtin -count=1 -timeout 900s
Desktop go vet/test
Frontend corepack pnpm test:all/build
114 个 Python 测试通过（2 skipped），Node/工具/docs/public/deploy/release 合同通过
scripts/verify-baseline.ps1 -SkipDesktop -SkipFrontendHint
六目标 CGO_ENABLED=0 交叉编译全部 exit 0
git diff --check
```

六目标为 linux/amd64、linux/arm64、darwin/amd64、darwin/arm64、windows/amd64、windows/arm64；产物位于系统临时目录。最终工作树上的 root build/vet/internal 全量、五个高风险包 race、Desktop、Frontend、综合 baseline、114 个脚本测试、合同检查和六目标构建均已通过；品牌残留检查为 0。远端 CI/CodeQL 尚待集中 push 后验证。

## 未关闭边界

- arbitrary shell、MCP 和外部 API 没有 exactly-once 语义；pending opaque tool 可能未执行、部分执行或已完成，续跑必须核验真实状态。
- Journal、subagent transcript/meta、parent runtime、checkpoint 与 workspace 是多个原子文件，不是单一跨资源断电事务。
- 完整 child evidence 与委派预算仍非跨进程账本；durable journal 只恢复结构化 effect boundary，跨 continuation 的 root 项目检查必须由 root 重跑。
- `AtomicWriteFile` 的 Windows cross-device/filter-driver fallback 仍可能原地复制；ACL、xattr 和硬链接身份不恢复。
- Rooted I/O 只覆盖 previewable built-in writer 和 checkpoint restore，不覆盖 shell/MCP/external API。

## 下一执行顺序

1. 完成本批文档合同与全量本地门禁，全面检查 diff 和受保护路径。
2. 只显式暂存本批文件，形成一个大提交并集中 push 一次，守候并修复 CI/CodeQL；不要为回填 run ID 制造碎片 push。
3. 转入 M4 剩余核心本地项：评估并关闭 transcript/runtime/checkpoint/workspace 跨资源断电恢复窗口。
4. 外部环境可用时并行关闭干净云节点、真实飞书和公开签名 release；不可用时保持 `external-blocked`，继续其他本地工作。

长期 GOAL 尚未完成。本批全绿只允许声明 rooted built-in writer/checkpoint 与 durable child effect journal 子项验收完成，不能声明任意副作用 exactly-once、整个 M4 或项目完成。
