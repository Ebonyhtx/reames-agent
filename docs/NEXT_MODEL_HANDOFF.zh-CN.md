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
- M2 进行中：依赖棘轮、结构化错误、CLI 会话恢复、版本化 command/event/display DTO、prompt metadata、主要会话持久化、设置、Desktop history 与 app/tabs session-store 边界已关闭；剩余 Desktop main/CLI composition root 与 CLI 专用渲染边界待继续收口。
- M6 进行中：Gateway service、headless smoke 和 feedback 本地闭环已具备；真实 IM 与干净云节点仍缺外部证据。

唯一执行顺序以 `docs/DEVELOPMENT_PLAN.md` 为准。

## 最近已交付 M2 批次

详见 `docs/audits/2026-07-11-m2-desktop-transcript-boundary.md`（上一批 prompt/settings 见 `docs/audits/2026-07-11-m2-prompt-settings-boundary.md`）：

- Desktop history、分页、checkpoint、tool/todo replay、planner sidecar 与 legacy preview 改用展示安全 `control.TranscriptMessage`，原消息 `Index` 直接保持 checkpoint 关联。
- `DisplayKey` 保持旧 SHA-256 sidecar key 兼容，安全 `ReplayText` 排除 referenced-context、synthetic、steer 和 Memory Compiler contract；两者均为 `json:"-"` 本地字段。
- model/effort/token-mode rebuild 改用 opaque `SessionHistorySnapshot`；event preview/citation/tool 也不再声明 provider DTO。
- 本批删除 Desktop app/tabs 两条 `provider` 生产依赖边，累计收缩二十一条；Serve、Bot、ACP service 无 runtime 直连，Desktop app/tabs 无 provider 直连。

## 最新已交付批次

- Desktop app 的 session listing/order/user-message/time/topic-binding/conflict/cleanup 改用稳定 control DTO；`app.go` 已无 `agent/provider/tool` 直连。
- 新增 opaque `control.LoadedSession` 与 `control.SessionLease`；tab preload/rebind 与 lease transfer/reclaim 不再暴露 agent session/message/lease/error/writer 类型。
- 依赖棘轮累计收缩二十八条，剩余 Desktop 直连集中在 tabs branch/index/migration 元数据。

详见 `docs/audits/2026-07-11-m2-desktop-session-store-boundary.md`。commit `dc93f2a` 已推送；本地全量门禁通过，CI run `29192648091` 为 8/8、CodeQL run `29192648051` 为 3/3。

## 上一已交付批次关键证据

```text
go build ./...                                      PASS
go vet ./...                                        PASS
go test ./internal/... -count=1 -timeout 10m        PASS
desktop/go test ./... -count=1 -timeout 10m         PASS (169.9s wall time)
desktop/frontend/corepack pnpm test:all              PASS
desktop/frontend/corepack pnpm build                 PASS (既有 chunk/dynamic-import 警告)
Python Desktop/upstream/installer contracts          PASS (69 tests, 2 platform skips)
upstream issue + builtin/docs/public/deploy/release  PASS
six-target CGO_ENABLED=0 cross-compile               PASS
.\scripts\verify-baseline.ps1 -SkipFrontendHint      PASS
```

本批不改变 Wails UI 或安装工件，未重复触发 Desktop candidate；上一批 production Windows schema v3 candidate 已全绿。当前远端 `main` 为 `b68e872`，普通 CI run `29158669525` 为 8/8、CodeQL run `29158669534` 为 3/3。

## 下一执行顺序

1. 继续关闭 Desktop main 或 CLI composition root，再处理其余 CLI 专用渲染边界，不为了清空 allowlist 制造反向依赖。
2. 积累足够大的实现/测试/文档批次后再统一跑全量门禁、提交和单次 push。
3. M2 达到当前里程碑门槛后，进入干净云节点 CLI + Gateway + feedback 运维闭环与真实飞书回环。

## 长期未关闭项

- M2 剩余 Desktop main/CLI composition root 与其余 CLI 专用渲染边界收口。
- 干净 Linux/云节点的 CLI、Gateway、feedback、日志、备份、升级回滚。
- 真实飞书/Lark 文本、审批、取消与恢复回环。
- plugin 权限 manifest、内容完整性和安装预览。
- 生产签名、notarization、provenance 与 updater 信任链，保持 `external-blocked`。

长期 GOAL 尚未完成。即使本批全绿，也只能声明该批验收完成，不能声明整个项目完成。

## 当前未提交批次

Desktop tabs session-store/control DTO/原子 meta mutation 批次已形成；root build/vet/internal、Desktop 全测、前端 test/build、CI-scoped Python 69 项（2 platform skips）、Node/文档/公开/部署/发布/工具合同、baseline、上游报告与六目标交叉编译均通过。commit、单次 push 和远端 CI/CodeQL 尚待完成。既有 `dc93f2a` 远端证据已合并进本批文档；受保护文件继续排除。
