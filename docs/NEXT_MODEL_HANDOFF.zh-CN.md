# 下一位模型接手交接文档

日期：2026-07-14

仓库：`F:\reames-agent`

分支：`main`

本页只记录当前接手边界。工作树、`docs/PROJECT.md`、`docs/DEVELOPMENT_PLAN.md` 和远端检查比本文更权威。

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

## 当前基线

- 本 M4 收官批次以 `ce3f0e6 feat: harden rooted writers and child effect recovery` 为父提交；当前提交与远端状态以 `git log`、`git status` 和 GitHub Actions 为准，避免为静态 hash/run ID 反复修改本页。
- 该提交 CI run `29302654132` attempt 2 为 8/8 success；CodeQL run `29302654052` 为 3/3 success。
- 本地只有 `main` 分支。
- M0、M1、M2、M3 已关闭；本批关闭 M4 最后的跨资源 crash-recovery 门槛。M6 的真实 linger logout/reboot、干净云节点、真实 IM 和公开签名 release 仍为 `external-blocked`。

## M4 收官批次

### Active turn 事务

- 每个 orchestrated turn 都分配 checkpoint；visible turn 进入用户 rewind 列表，Goal/Plan synthetic continuation 使用隐藏 checkpoint，只回滚自身 effects。
- Branch meta 的 in-flight marker 记录 checkpoint turn、message boundary 和 `preserveUser`。
- 成功顺序为 transcript 持久化、runtime sidecar 持久化、commit anchor、marker clear。
- 冷启动只有在 transcript 与 runtime 同时匹配 commit anchor 时保留 workspace；否则恢复 checkpoint workspace/runtime 并截断 partial transcript。失败/cancel 在当前进程也走同一回滚。
- checkpoint 分配或 in-flight marker 持久化失败会在模型 runner 前阻断；marker 失败时退休刚创建的空 checkpoint，避免幽灵 rewind 点。

### Rewind 事务

- Conversation/RewindBoth 在 branch sidecar 写 `prepared` intent，包含 turn、prefix anchor、runtime 和 code scope。
- transcript/runtime/workspace 全部发布后写 `resources_applied` barrier；随后才退休 checkpoint 并清 journal。
- Resume、新 turn 以及 Compact/New/Fork/Branch/Switch/Summarize/Rewind 等保留内容的 rotation 操作前都会重放未完成 phase。测试覆盖仅 intent、资源已提交但 checkpoint 未退休、checkpoint 已退休但日志未清、phase 写失败和 final clear 失败。

### 原子持久化

- `AtomicWriteFile` 已删除 Windows cross-device/filter-driver 原地 copy fallback；这类 rename 直接 fail closed。
- Windows replace 使用 `MoveFileEx(REPLACE_EXISTING|WRITE_THROUGH)`；Unix rename 后 fsync 父目录；rooted writer rename 后同步目录。
- Session append event 现执行 fsync，首次创建 event log 后同步父目录；branch meta 临时文件 rename 前先 `Sync()`。
- Desktop 的 stuck topic timing test 已与 delete-session 合同统一，定向连续 20 次通过，用于消除 Windows CI loaded-runner 假失败。

## 当前本地证据

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

综合 baseline 的 Gateway smoke 报告与六目标二进制只写入系统临时目录。以上为本批本地证据，不替代集中 push 后的新 CI/CodeQL；远端状态直接读取 GitHub Actions，不为回填 run ID 单独 push。

DeepSeek cache probe 已与 ACP 真实测试统一置于 `live` build tag；普通全测即使开发机有 `DEEPSEEK_API_KEY` 也不产生真实请求。显式 live probe 本次网络超时，因此没有新增真实 Provider 证据。

## 明确限制

- `bash`、MCP、外部 API 和后台 opaque side effect 没有逐文件 checkpoint 或 exactly-once；恢复后必须读取磁盘/外部系统真实状态。
- 完整 evidence ledger 和委派预算仍非跨进程账本；durable root check 引用与 child mutation cursor 不等于完整 child proof。
- Checkpoint 恢复 bytes/mode，不恢复 ACL、xattr 或硬链接身份。
- 无 session lease 的嵌入方不获得跨进程单写者保证；跨 home/state 根 backup 与 updater 仍有各自的 M6 边界。

## 下一执行顺序

1. 审查 diff 和受保护路径；若本批尚未提交，只显式暂存允许文件，形成一个大提交并集中 push 一次。
2. 守候并修复该提交的新 CI/CodeQL；不要为回填 run ID 单独 push。
3. M4 远端证据闭环后进入 M5 plugin manifest/权限/安装信任/升级回滚；外部环境到位时并行关闭 M6 external-blocked 项。

长期 GOAL 尚未完成。M4 完成声明只覆盖路线图定义的本地 Agent 可靠性边界，不得扩大为任意副作用 exactly-once 或整个项目完成。
