# 下一位模型接手交接文档

日期：2026-07-11
仓库：`F:\reames-agent`
工作分支：`agent/full-delivery-program`

本页只记录当前接手边界。代码、`git status`、`docs/PROJECT.md`、`docs/DEVELOPMENT_PLAN.md` 和最新远端 CI 结果优先级更高。

## 用户目标与节奏

持续把 Reames Agent 推进到高可信可交付状态；参考 DeepSeek Reasonix、`F:\code-reference` 和 `F:\Reames-Lite` 吸收机制，但不盲目复制。每批同步代码、测试和文档，完成大批本地验证后再集中 commit/push，避免碎片 push 反复消耗 CI。

结论必须区分：单元/合同测试、localhost 模拟、原生 Desktop、远端 candidate、真实 Provider/IM/云服务证据。不得用其中一层替代另一层的完成声明。

## 受保护文件

以下为用户或其他会话的未跟踪文件，禁止修改、暂存或提交：

```text
.agents/BULK_EXECUTION_BRIEF.zh-CN.md
.agents/BULK_EXECUTION_STATUS.zh-CN.md
.agents/FULL_DELIVERY_MASTER_PLAN.zh-CN.md
docs/audits/2026-07-09-reference-feature-gap-map.md
```

只使用显式 `git add -- <paths>`，禁止 `git add .` / `git add -A`。

## 当前项目状态

- M0 已关闭：普通 CI、CodeQL、六目标 CLI candidate、三平台 Desktop candidate 和原生安装 smoke 有历史远端证据。
- M1 进行中：真实 Provider、原生新建/发送/停止、文件审批/落盘/回退、重启恢复已有证据；仍缺原生 Wails 失败提示与恢复 smoke。
- M2 进行中：入口依赖棘轮已建立；本批让结构化 `ErrorInfo` 穿过共享 wire，但 command DTO 和前端 category 路由未完成。
- M6 进行中：Gateway service、headless smoke 和 feedback 本地闭环已具备；真实 IM 与干净云节点仍缺外部证据。

路线唯一来源是 `docs/DEVELOPMENT_PLAN.md`。

## 最近批次

外部 Agent 的 23 个提交已完成真实性验收。详细保留、撤回和测试证据见：

```text
docs/audits/2026-07-11-external-agent-batch-acceptance.md
```

本批关键结果：

- 撤回伪前端/Bot/Evidence/cache 测试、未接入的飞书 HMAC、半成品 plugin 安全字段、重复浅层脚本和冲突 NOTICE。
- 修复 GoalState v1 丢 Todo、未来版本绕过、非原子生产写入。
- 让 ErrorInfo 进入 event wire，保留旧 `err`；修复 `generate`/`author` 被误分类的宽泛字符串匹配。
- 补强 Provider harness 请求记录/上限/脚本验证；Provider 真实验证不再保存错误正文。
- 修复 upstream `--deep` 的直接执行崩溃与 base/head 获取，加入真实本地 Git diff 测试和 issue draft 路径清洗。
- 更新威胁模型、项目事实、路线图和 CI Python 合同。

## 已通过验证

```text
go build ./...
go vet ./...
go test ./internal/... -count=1 -timeout 300s
go test -race（本批并发/取消/请求记录子集）
desktop/go test . -count=1 -timeout 300s
desktop/frontend/corepack pnpm test:all
desktop/frontend/corepack pnpm build
文档、public readiness、deploy、release 合同
installer/artifact/native smoke 合同
upstream/provider verifier/issue draft Python 测试
upstream issue Node 测试与真实远端扫描
6 目标 CGO_ENABLED=0 交叉编译
scripts/verify-baseline.ps1 -SkipFrontendHint（含 headless Gateway smoke）
```

真实远端 upstream 扫描结果为 `changed_count=9`；报告写入临时目录后已清理，没有接受或修改 lock。前端 build 仍有既有大 chunk 警告，属于 M3 性能债务。

## 尚未关闭

1. 用 loopback 失败脚本驱动安装后的 Windows Wails，验证 401/429/断流/权限拒绝/工具超时的可见提示、重试和运行态恢复。
2. 完成 ErrorInfo 前端 category 路由，并继续按 submit/cancel/approval/session/status 收缩 control DTO。
3. 在干净 Linux/云节点验证 CLI、Gateway、feedback、日志、备份与升级回滚。
4. 使用真实飞书/Lark 凭据完成文本、审批、取消和恢复回环。
5. 设计并实现 plugin 权限 manifest、内容完整性和安装预览；当前没有签名/权限 enforcement。
6. 生产发布签名、notarization、provenance 和 updater 信任链保持外部阻塞。

## 提交与推送

提交前：

1. `git status --short` 只允许本批跟踪改动和上述受保护未跟踪文件。
2. 显式暂存本批路径，检查 `git diff --cached --stat`、`git diff --cached --check`。
3. 本批集中提交并只 push 一次；push 后观察普通 CI 和 CodeQL。涉及 Desktop candidate workflow 时才手动触发 candidate。

长期 GOAL 尚未完成。即使本批全绿，也只能声明该批验收完成，不能声明整个项目完成。
