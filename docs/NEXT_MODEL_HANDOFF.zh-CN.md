# 下一位模型接手交接文档

日期：2026-07-11

仓库：`F:\reames-agent`

工作分支：`main`

本页只记录当前接手边界。代码、`git status`、`docs/PROJECT.md`、`docs/DEVELOPMENT_PLAN.md` 和最新远端 CI 结果优先级更高。

## 用户目标与节奏

持续把 Reames Agent 推进到高可信可交付状态；参考 DeepSeek Reasonix、`F:\code-reference` 和 `F:\Reames-Lite` 吸收机制，但不盲目复制。每批同步代码、测试和文档，形成足够大的本地成果后再集中 commit/push，避免碎片 push 重复消耗 CI。

结论必须区分单元/合同测试、localhost 模拟、原生 Desktop、远端 candidate 与真实 Provider/IM/云服务证据，不能用其中一层替代另一层。

## 受保护文件

以下是用户或其他会话的未跟踪文件，禁止修改、暂存或提交：

```text
.agents/
docs/audits/2026-07-09-reference-feature-gap-map.md
```

只使用显式 `git add -- <paths>`，禁止 `git add .` 或 `git add -A`。

## 当前项目状态

- M0 已关闭：普通 CI、CodeQL、六目标 CLI candidate、三平台 Desktop candidate 和原生安装 smoke 均有历史远端证据。
- M1 已关闭：真实 Provider、原生会话/工作区/停止、文件审批/落盘/回退、重启恢复，以及 401/429/断流/权限拒绝/工具超时均有分层证据。
- M2 进行中：依赖棘轮已建立；结构化错误路径和 CLI 会话恢复 control 边界已关闭；提交、取消、审批、状态与剩余 command/event DTO 待继续收口。
- M6 进行中：Gateway service、headless smoke 和 feedback 本地闭环已具备；真实 IM 与干净云节点仍缺外部证据。

唯一执行顺序以 `docs/DEVELOPMENT_PLAN.md` 为准。

## 当前 M2 批次

详见 `docs/audits/2026-07-11-m2-error-session-control.md`：

- Desktop 真正消费共享 `ErrorInfo`：按 code 本地化，按 category 决定严重度与动作；认证错误打开模型设置，可重试错误重试/续接，用户取消不再显示为失败。
- Windows production Wails UIA 已真实点击认证设置和流中断续接；schema v3 本地证据通过，19 次 Provider 请求，所有原 M1 场景、Stop、重启恢复继续通过。
- 新增稳定 `control.SessionInfo`、`control.ListSessions` 和事务式 `ResumeSessionPath`；CLI 两份 `/resume` 实现移除 `internal/agent` 直连，依赖棘轮缩减两条边。

## 本批关键本地证据

```text
production Wails executable:
desktop/build/bin/reames-agent-desktop.exe
size: 47,906,304 bytes
SHA-256: 175B6BA8D5E18027108830FC87F32415F94A9BF50E3EDE3AB6256E1242E6FD09

native UIA evidence:
artifacts/desktop-windows-interaction-smoke-m2-local.json
schema_version: 3
outcome: passed
auth_settings_opened: true
stream_retry_invoked: true
provider_requests: 19
errors: []
boundary_changes: []
```

`artifacts/` 是本地生成证据，不提交。push 后需手动触发 Desktop candidate，核验安装后的 Windows schema v3 证据。

## 下一执行顺序

1. 显式暂存本批路径，集中提交并只 push 一次；观察普通 CI 与 CodeQL。
2. 手动触发 Desktop candidate，下载并核验 Windows 安装后 schema v3 证据。
3. 继续按提交 → 取消 → 审批 → 状态纵向收缩 control 边界与 command/event DTO。
4. 然后进入干净云节点 CLI + Gateway + feedback 运维闭环与真实飞书回环。

## 长期未关闭项

- M2 剩余 control/DTO 收口。
- 干净 Linux/云节点的 CLI、Gateway、feedback、日志、备份、升级回滚。
- 真实飞书/Lark 文本、审批、取消与恢复回环。
- plugin 权限 manifest、内容完整性和安装预览。
- 生产签名、notarization、provenance 与 updater 信任链，保持 `external-blocked`。

长期 GOAL 尚未完成。即使本批全绿，也只能声明该批验收完成，不能声明整个项目完成。
