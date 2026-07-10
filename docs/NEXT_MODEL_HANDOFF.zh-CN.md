# 下一位模型接手交接文档

日期：2026-07-11
仓库：`F:\reames-agent`
工作分支：`main`

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
- M1 已关闭：真实 Provider、原生新建/发送/停止、文件审批/落盘/回退、重启恢复，以及 401/429/断流/权限拒绝/工具超时的原生提示和恢复均有分层证据。
- M2 进行中：入口依赖棘轮已建立；本批让结构化 `ErrorInfo` 穿过共享 wire，但 command DTO 和前端 category 路由未完成。
- M6 进行中：Gateway service、headless smoke 和 feedback 本地闭环已具备；真实 IM 与干净云节点仍缺外部证据。

路线唯一来源是 `docs/DEVELOPMENT_PLAN.md`。

## 最近批次

本批关闭 M1 最后的原生失败恢复缺口，详见 `docs/audits/2026-07-11-windows-native-failure-recovery.md`：

- loopback OpenAI fixture 按 turn 脚本化 401、429、流截断、`write_file` tool call 和 `bash` tool call，同时继续承担成功 turn。
- Desktop warning 使用结构化 `ErrorInfo.code` 暴露稳定 alert AutomationId；approval 和失败 tool card 增加稳定 UIA 标识。
- 修复 retry 状态被后台 `phase/notice` 立即清除的问题；原生 UIA 实际观察到 `retrying (1/10)…`。
- production Wails 在同一会话完成五类失败、每类后续成功 turn、长命令 Stop 和重启恢复；19 次 Provider 请求，边界变化与错误为空。

## 已通过验证

```text
go build ./...
go vet ./...
go test ./internal/... -count=1 -timeout 10m
desktop/go test ./... -count=1 -timeout 10m
desktop/frontend/corepack pnpm test:all
desktop/frontend/corepack pnpm build
python -m unittest scripts.test_smoke_desktop_interaction scripts.test_smoke_desktop_native -v
go test ./internal/tool -run TestBuiltinToolContractDocumentation -v -count=1
python scripts/smoke_desktop_interaction.py --exe desktop/build/bin/reames-agent-desktop.exe --artifact desktop/build/bin/reames-agent-desktop.exe --out artifacts/desktop-windows-interaction-smoke-local.json --timeout-seconds 45
```

最终本地 Wails 证据对应 SHA-256 `9011DD5E634F601D275BE893B743335A234116F420AD4B29D8305B2832814D1F`。前端 build 仍有既有大 chunk 警告，属于 M3 性能债务。本批 push 前远端状态仍是 `f0cd4a5` 的普通 CI 8/8 与 CodeQL 3/3 通过；新提交必须重新观察。

## 尚未关闭

1. 完成 ErrorInfo 前端 category 路由，并继续按 submit/cancel/approval/session/status 收缩 control DTO。
2. 在干净 Linux/云节点验证 CLI、Gateway、feedback、日志、备份与升级回滚。
3. 使用真实飞书/Lark 凭据完成文本、审批、取消和恢复回环。
4. 设计并实现 plugin 权限 manifest、内容完整性和安装预览；当前没有签名/权限 enforcement。
5. 生产发布签名、notarization、provenance 和 updater 信任链保持 external-blocked。

## 提交与推送

提交前：

1. `git status --short` 只允许本批跟踪改动和上述受保护未跟踪文件。
2. 显式暂存本批路径，检查 `git diff --cached --stat`、`git diff --cached --check`。
3. 本批集中提交并只 push 一次；push 后观察普通 CI 和 CodeQL。涉及 Desktop candidate workflow 时才手动触发 candidate。

长期 GOAL 尚未完成。即使本批全绿，也只能声明该批验收完成，不能声明整个项目完成。
