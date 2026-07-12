# 下一位模型接手交接文档

日期：2026-07-12

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
- M2 本地候选完成：依赖棘轮 allowlist 已归零，结构化错误、版本化 command/event/display DTO、prompt metadata、会话持久化、Desktop/ACP/CLI 装配和终端渲染边界均已收口；完整本地门禁已通过，远端证据待补齐。
- M6 进行中：Gateway service、headless smoke 和 feedback 本地闭环已具备；真实 IM 与干净云节点仍缺外部证据。

唯一执行顺序以 `docs/DEVELOPMENT_PLAN.md` 为准。

## 最近已交付 M2 批次

详见 `docs/audits/2026-07-11-m2-desktop-transcript-boundary.md`（上一批 prompt/settings 见 `docs/audits/2026-07-11-m2-prompt-settings-boundary.md`）：

- Desktop history、分页、checkpoint、tool/todo replay、planner sidecar 与 legacy preview 改用展示安全 `control.TranscriptMessage`，原消息 `Index` 直接保持 checkpoint 关联。
- `DisplayKey` 保持旧 SHA-256 sidecar key 兼容，安全 `ReplayText` 排除 referenced-context、synthetic、steer 和 Memory Compiler contract；两者均为 `json:"-"` 本地字段。
- model/effort/token-mode rebuild 改用 opaque `SessionHistorySnapshot`；event preview/citation/tool 也不再声明 provider DTO。
- 本批删除 Desktop app/tabs 两条 `provider` 生产依赖边，累计收缩二十一条；Serve、Bot、ACP service 无 runtime 直连，Desktop app/tabs 无 provider 直连。

## 最新已交付批次

- 新增最小稳定 `control.SessionMeta` 与 control-owned 原子 mutation，隐藏 revision/digest/writer/in-flight/listing 等持久层字段；错误 mutation 不落盘。
- Desktop tabs 的 listing/order/index/migration/topic/profile/recovery/cleanup 路径全部改走 control，`boot.Options` 恢复回调也不再暴露 `agent.BranchMeta`。
- `desktop/app.go` 与 `desktop/tabs.go` 均无 `agent/provider/tool` 生产直连；依赖棘轮累计收缩二十九条。

详见 `docs/audits/2026-07-12-m2-desktop-tabs-session-boundary.md`。主 commit `9effae6` 与文档索引修复 `cc92f67` 已推送；本地全量门禁通过，CI run `29193741370` 为 8/8、CodeQL run `29193741351` 为 3/3。

## 最新批次关键证据

```text
go build ./...                                      PASS
go vet ./...                                        PASS
go test ./internal/... -count=1 -timeout 10m        PASS
desktop/go test ./... -count=1 -timeout 10m         PASS (211.3s wall time)
desktop/frontend/corepack pnpm test:all              PASS
desktop/frontend/corepack pnpm build                 PASS (既有 chunk/dynamic-import 警告)
Python Desktop/upstream/installer contracts          PASS (69 tests, 2 platform skips)
upstream issue + builtin/docs/public/deploy/release  PASS
six-target CGO_ENABLED=0 cross-compile               PASS
.\scripts\verify-baseline.ps1 -SkipFrontendHint      PASS
```

本批不改变 Wails UI 或安装工件，未重复触发 Desktop candidate；上一批 production Windows schema v3 candidate 已全绿。当前远端 `main` 为 `cc92f67`，普通 CI run `29193741370` 为 8/8、CodeQL run `29193741351` 为 3/3。

## 下一执行顺序

1. 显式暂存当前 M2 transport 收官批次，集中提交、单次 push 并守候远端 CI/CodeQL。
2. 远端全绿后进入 M3 原生 Desktop 日用化、可访问性和启动/bundle 性能门槛。
3. 随后进入干净云节点 CLI + Gateway + feedback 运维闭环与真实飞书回环。

## 长期未关闭项

- M2 仅剩当前候选批次的远端证据。
- 干净 Linux/云节点的 CLI、Gateway、feedback、日志、备份、升级回滚。
- 真实飞书/Lark 文本、审批、取消与恢复回环。
- plugin 权限 manifest、内容完整性和安装预览。
- 生产签名、notarization、provenance 与 updater 信任链，保持 `external-blocked`。

长期 GOAL 尚未完成。即使本批全绿，也只能声明该批验收完成，不能声明整个项目完成。

## 当前未提交批次

M2 transport 收官代码、测试、架构与审计批次已形成；root build/vet/internal、Desktop 全测、前端 test/build、CI-scoped Python 69 项（2 platform skips）、Node/文档/公开/部署/发布/工具合同、baseline、上游报告、空 allowlist 与六目标交叉编译均通过。commit、单次 push 和远端 CI/CodeQL 尚待完成；上一批 `9effae6`/`cc92f67` 的远端成功证据已并入本批文档，受保护文件继续排除。
