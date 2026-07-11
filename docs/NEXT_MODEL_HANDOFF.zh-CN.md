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
- M2 进行中：依赖棘轮、结构化错误、CLI 会话恢复和版本化 command DTO 已关闭；剩余 event/展示 DTO、会话、装配、设置和 prompt/cache 边界待继续收口。
- M6 进行中：Gateway service、headless smoke 和 feedback 本地闭环已具备；真实 IM 与干净云节点仍缺外部证据。

唯一执行顺序以 `docs/DEVELOPMENT_PLAN.md` 为准。

## 当前 M2 批次

详见 `docs/audits/2026-07-11-m2-command-control.md`（上一批原生错误与恢复证据仍见 `docs/audits/2026-07-11-m2-error-session-control.md`）：

- 新增版本化 `control.Command` / `CommandResult`、结构化协议错误和服务端选择的 `CommandScope`，远端 JSON 不能选择 trusted scope。
- Desktop、CLI、Serve、Bot、ACP 的 submit/cancel/approval/status 对应生产入口已迁移；同步 `RunTurn` 为拥有 turn context 的 CLI/Bot/ACP 保留。
- Serve 新增 `POST /command` 和 WebSocket `method=command`，旧 HTTP/WS 入口兼容映射同一 remote dispatcher。
- 修复日志 response writer 未转发 `http.Hijacker` 导致 `/ws` 实际无法握手，以及旧 WS submit 可绕过 HTTP `!shell` 禁止的策略漂移；真实 WebSocket 回归覆盖两者。

## 本批关键本地证据

```text
go build ./...                                      PASS
go vet ./...                                        PASS
go test ./internal/... -count=1 -timeout 10m        PASS
desktop/go test . -count=1                          PASS (170.678s final rerun)
desktop/frontend/corepack pnpm test:all              PASS
desktop/frontend/corepack pnpm build                 PASS (既有 chunk 警告)
Python Desktop/upstream contracts                    PASS (44 tests, 1 skip)
upstream issue + builtin tool contract               PASS
.\scripts\verify-baseline.ps1 -SkipFrontendHint      PASS
真实 localhost WebSocket 握手/command/legacy policy  PASS
```

本批不改变 Wails UI 或安装工件，不重复触发 Desktop candidate；上一批 production Windows schema v3 candidate 已全绿。远端普通 CI/CodeQL 仍需本批集中 push 后确认。

## 下一执行顺序

1. 完成全量本地门禁，显式暂存本批路径，集中提交并只 push 一次；观察普通 CI 与 CodeQL。
2. 审查并收口剩余 event/展示 DTO 与会话/装配边界；随后补 provider prompt 与 UI/渠道 metadata 的缓存前缀回归。
3. 然后进入干净云节点 CLI + Gateway + feedback 运维闭环与真实飞书回环。

## 长期未关闭项

- M2 剩余 event/展示 DTO、会话/装配/设置和 prompt/cache 边界收口。
- 干净 Linux/云节点的 CLI、Gateway、feedback、日志、备份、升级回滚。
- 真实飞书/Lark 文本、审批、取消与恢复回环。
- plugin 权限 manifest、内容完整性和安装预览。
- 生产签名、notarization、provenance 与 updater 信任链，保持 `external-blocked`。

长期 GOAL 尚未完成。即使本批全绿，也只能声明该批验收完成，不能声明整个项目完成。
