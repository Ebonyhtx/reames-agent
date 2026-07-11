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
- M2 进行中：依赖棘轮、结构化错误、CLI 会话恢复、版本化 command/event/display DTO、prompt metadata、主要会话持久化和设置边界已关闭；剩余 Desktop app/tab、CLI/ACP composition root 与专用渲染边界待继续收口。
- M6 进行中：Gateway service、headless smoke 和 feedback 本地闭环已具备；真实 IM 与干净云节点仍缺外部证据。

唯一执行顺序以 `docs/DEVELOPMENT_PLAN.md` 为准。

## 当前 M2 批次

详见 `docs/audits/2026-07-11-m2-prompt-settings-boundary.md`（上一批会话适配器见 `docs/audits/2026-07-11-m2-session-adapter-boundary.md`）：

- Desktop 删除自有 prompt helper，control 统一处理 carried history、loaded legacy transcript、system prompt 刷新和磁盘 rewrite baseline；相同 prompt 的 rebind wire bytes 保持不变。
- Memory suggestions 与 ACP metadata title 改用展示安全 `control.TranscriptMessage`，不再从 system、合成恢复指令、compose 控制块或 referenced-context payload 生成本地持久化内容。
- Settings 使用 opaque `SessionHistorySnapshot` 和 `RegisteredProviderKinds`；Serve 的后台标题 provider 迁入 `boot.SessionTitleGenerator`，其用量不污染主会话事件流。
- 本批再删除七条 `agent/provider` 生产依赖边；累计收缩十九条。Serve、Bot 和 ACP service 已无受守卫 runtime 直连。

## 本批关键本地证据

```text
go build ./...                                      PASS
go vet ./...                                        PASS
go test ./internal/... -count=1 -timeout 10m        PASS
desktop/go test ./... -count=1 -timeout 10m         PASS (191.4s wall time)
desktop/frontend/corepack pnpm test:all              PASS
desktop/frontend/corepack pnpm build                 PASS (既有 chunk/dynamic-import 警告)
Python Desktop/upstream/installer contracts          PASS (69 tests, 2 platform skips)
upstream issue + builtin/docs/public/deploy/release  PASS
six-target CGO_ENABLED=0 cross-compile               PASS
.\scripts\verify-baseline.ps1 -SkipFrontendHint      PASS
```

本批不改变 Wails UI 或安装工件，未重复触发 Desktop candidate；上一批 production Windows schema v3 candidate 已全绿。当前远端 `main` 为 `4beae18`，普通 CI run `29144669944` 与 CodeQL run `29144669930` 均为 `Success`；本批尚未 push，因此没有对应远端证据。

## 下一执行顺序

1. 显式暂存当前 prompt/settings/ACP metadata 批次，排除 `.agents/` 与 `docs/audits/2026-07-09-reference-feature-gap-map.md`；形成单个提交、单次 push，并守候普通 CI 与 CodeQL 全绿。
2. 继续关闭 Desktop app/tabs 中可独立验收的 session lease/list/meta/display 边界，再处理 CLI/ACP composition root 与 CLI 专用渲染；不要为了清空 allowlist 制造反向依赖。
3. M2 达到当前里程碑门槛后，进入干净云节点 CLI + Gateway + feedback 运维闭环与真实飞书回环。

## 长期未关闭项

- M2 剩余 Desktop app/tabs、CLI/ACP composition root 与 CLI 专用渲染边界收口。
- 干净 Linux/云节点的 CLI、Gateway、feedback、日志、备份、升级回滚。
- 真实飞书/Lark 文本、审批、取消与恢复回环。
- plugin 权限 manifest、内容完整性和安装预览。
- 生产签名、notarization、provenance 与 updater 信任链，保持 `external-blocked`。

长期 GOAL 尚未完成。即使本批全绿，也只能声明该批验收完成，不能声明整个项目完成。

## 当前未提交批次验证

prompt/settings/ACP metadata 批次已通过 root build/vet/internal 全测、Desktop 全测、前端 `test:all`/build、Python/Node/工具/文档/公开/部署/发布合同、六目标交叉编译及 `verify-baseline.ps1`。工作树仍保留该批生产代码、测试和文档，必须显式暂存并集中提交；受保护文件继续排除。
