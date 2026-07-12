# 下一位模型接手交接文档

日期：2026-07-13

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
artifacts/
docs/audits/2026-07-09-reference-feature-gap-map.md
```

只使用显式 `git add -- <paths>`，禁止 `git add .` 或 `git add -A`。

## 当前项目状态

- M0 已关闭：普通 CI、CodeQL、六目标 CLI candidate、三平台 Desktop candidate 和原生安装 smoke 均有历史远端证据。
- M1 已关闭：真实 Provider、原生会话/工作区/停止、文件审批/落盘/回退、重启恢复，以及 401/429/断流/权限拒绝/工具超时均有分层证据。
- M2 已关闭：依赖棘轮 allowlist 已归零，结构化错误、版本化 command/event/display DTO、prompt metadata、会话持久化、Desktop/ACP/CLI 装配和终端渲染边界均已收口；完整本地门禁及远端 CI/CodeQL 已通过。
- M3 进行中：性能首批 commit `e147854` 已远端全绿；入口 chunk 下降 43.6%，Windows production Wails 稳定响应实测 2.016 秒。当前候选批次已统一真正模态层的焦点/键盘/读屏语义，并关闭 Windows 显示缩放的持久化与重启交互缺口。
- M6 进行中：Gateway service、headless smoke 和 feedback 本地闭环已具备；真实 IM 与干净云节点仍缺外部证据。

唯一执行顺序以 `docs/DEVELOPMENT_PLAN.md` 为准。

## 最新已交付 M3 性能批次

- 关闭态与次级界面按真实打开状态拆包，真实 `dist` 强制 entry、initial JS/CSS、最大 asset 和请求数预算。
- Windows native smoke schema v2 记录首次可见/响应/稳定响应，candidate 固定 8 秒硬预算；本地稳定响应 2.016 秒。
- 最终 production 数据：entry JS 621,270 B、initial JS 1,209,699 B、initial CSS 607,374 B、largest JS 704,186 B、initial files 5。

详见 `docs/audits/2026-07-12-m3-desktop-bundle-budget.md`。commit `e147854` 已推送；CI run `29196957695` 为 8/8、CodeQL run `29196957665` 为 3/3。

## 当前 M3 候选批次关键证据

```text
go build ./...                                           PASS
go vet ./...                                             PASS
go test ./internal/... -count=1 -timeout 10m             PASS
desktop/go test ./... -count=1 -timeout 10m              PASS (206.7s wall time)
desktop/frontend/corepack pnpm test:all                   PASS
desktop/frontend/corepack pnpm build                      PASS (bundle budget enforced)
Python Desktop/upstream/installer contracts               PASS (71 tests, 2 platform skips)
docs/public/deploy/release/tool contracts                  PASS
Wails v2.12.0 production Windows build                    PASS
Windows native startup budget (8s)                        PASS (stable 2.016s)
six-target CGO_ENABLED=0 cross-compile                    PASS
.\scripts\verify-baseline.ps1 -SkipFrontendHint           PASS
python scripts/check_upstreams.py                          PASS (changed_count=9)
```

该批改变 Wails UI 与 Windows native smoke schema，但未额外触发三平台 Desktop candidate；上一批 production Windows schema v3 interaction candidate 已全绿。当前远端 `main` 为 `e147854`，普通 CI run `29196957695` 为 8/8、CodeQL run `29196957665` 为 3/3。

## 下一执行顺序

1. 显式暂存当前 M3 焦点与显示缩放候选，集中提交并单次 push；守候普通 CI 与 CodeQL，不为纯证据另做 push。
2. 后续补原生键盘/读屏抽查、对比度及热启动、Linux/macOS candidate 预算。
3. M3 形成足够完整的主产品体验后，再进入干净云节点 CLI + Gateway + feedback 运维闭环与真实飞书回环。

## 长期未关闭项

- M3 原生 Desktop 日用化、可访问性和启动/bundle 性能门槛。
- 干净 Linux/云节点的 CLI、Gateway、feedback、日志、备份、升级回滚。
- 真实飞书/Lark 文本、审批、取消与恢复回环。
- plugin 权限 manifest、内容完整性和安装预览。
- 生产签名、notarization、provenance 与 updater 信任链，保持 `external-blocked`。

长期 GOAL 尚未完成。即使本批全绿，也只能声明该批验收完成，不能声明整个项目完成。

## 当前未提交 M3 可访问性与显示缩放批次

新增共享 `useDialogFocus`，覆盖初始焦点、Tab 围栏、嵌套顶层、延迟挂载和退出动画后二次 opener 恢复；Settings/History/Command Palette/Shortcuts/Image Viewer/Onboarding 已接入。命令面板补 combobox/listbox/active-descendant，并修复按需加载导致退出动画丢失及 opener 捕获过晚。Windows 显示缩放新增最后选择胜出的串行合并写、Go 原子持久化、待重启状态、立即重启和失败回滚；真实浏览器完成 100% → 105% → 100% 回环且日志为 0。完整前端、Desktop、根模块和文档门禁均通过；production 数据为 entry JS 623,888 B、initial JS 1,213,105 B、initial CSS 607,569 B、largest JS 704,186 B、initial files 5。commit、单次 push 与远端证据待完成；受保护文件继续排除。
