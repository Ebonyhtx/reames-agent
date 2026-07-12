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
- M3 进行中：性能首批 commit `e147854` 与可访问性/显示缩放 commit `9d8368c` 均已远端全绿；入口 chunk 下降 43.6%。当前未提交候选已把六主题深浅色、普通/Creation 对比度与焦点环变成可执行合同，修复设置入口重挂载后的焦点恢复，并把 Windows 同 HOME warm relaunch 纳入 6 秒原生预算；当前冷/warm 稳定响应均实测 1.516 秒。
- M6 进行中：Gateway service、headless smoke 和 feedback 本地闭环已具备；真实 IM 与干净云节点仍缺外部证据。

唯一执行顺序以 `docs/DEVELOPMENT_PLAN.md` 为准。

## 最新已交付 M3 批次

- 关闭态与次级界面按真实打开状态拆包，真实 `dist` 强制 entry、initial JS/CSS、最大 asset 和请求数预算。
- Windows native smoke schema v2 记录首次可见/响应/稳定响应，candidate 固定 8 秒硬预算；本地稳定响应 2.016 秒。
- 最终 production 数据：entry JS 621,270 B、initial JS 1,209,699 B、initial CSS 607,374 B、largest JS 704,186 B、initial files 5。

详见 `docs/audits/2026-07-12-m3-desktop-bundle-budget.md`。commit `e147854` 已推送；CI run `29196957695` 为 8/8、CodeQL run `29196957665` 为 3/3。

- commit `9d8368c` 统一 Settings/History/Command Palette/Shortcuts/Image Viewer/Onboarding 的模态焦点生命周期，补 combobox/listbox 读屏合同与退出动画后 opener 恢复。
- 同一批次关闭 Windows 显示缩放最后选择胜出、Go 原子持久化、待重启状态、立即重启和失败回滚。
- 该批 production 数据：entry JS 623,888 B、initial JS 1,213,105 B、initial CSS 607,569 B、largest JS 704,186 B、initial files 5。

commit `9d8368c` 已推送；普通 CI run `29203366720` 为 8/8、CodeQL run `29203366703` 为 3/3。

## 当前 M3 候选批次关键证据

```text
desktop/frontend/corepack pnpm test:theme-contrast        PASS (305)
desktop/frontend/corepack pnpm test:dialog-focus          PASS (10 hook + 2 palette)
desktop/frontend/corepack pnpm typecheck/test:all/build   PASS
in-app Browser Graphite/Carbon/Amber + Creation           PASS (0 warning/error)
python -m unittest scripts.test_smoke_desktop_native -v   PASS (21)
python scripts/check_release_contracts.py                 PASS
Wails v2.12.0 production Windows build                    PASS (48.7s)
native cold 8s / same-home warm 6s startup budgets        PASS (1.516s / 1.516s)
go build/vet/test ./internal/...                          PASS
desktop/go test ./...                                    PASS
docs/public/release contracts                            PASS
```

当前远端 `main` 为 `9d8368c`。本候选没有 push，也没有远端证据；Vite dev + mock bridge 的浏览器交互不冒充 Wails、Windows High Contrast 或屏幕阅读器证据。

## 下一执行顺序

1. 显式暂存当前主题对比度 + Windows warm relaunch 大批次，commit、单次 push，并守候普通 CI/CodeQL。
2. Windows installer candidate 是手动 workflow；按发布节奏触发后再把 8 秒冷/6 秒 warm runner 结果记为远端 candidate 证据，普通 push 不冒充该层。
3. 后续补 Windows High Contrast/屏幕阅读器抽查与 Linux/macOS candidate 启动预算；M3 主体验充分后再进入干净云节点 CLI + Gateway + feedback 与真实飞书回环。

## 长期未关闭项

- M3 原生 Desktop 日用化、可访问性和启动/bundle 性能门槛。
- 干净 Linux/云节点的 CLI、Gateway、feedback、日志、备份、升级回滚。
- 真实飞书/Lark 文本、审批、取消与恢复回环。
- plugin 权限 manifest、内容完整性和安装预览。
- 生产签名、notarization、provenance 与 updater 信任链，保持 `external-blocked`。

长期 GOAL 尚未完成。即使本批全绿，也只能声明该批验收完成，不能声明整个项目完成。

## 当前未提交 M3 主题对比度、重挂载焦点与 warm startup 批次

新增直接解析生产 `styles.css` 的对比度合同，覆盖六主题 × 深浅色 × 普通/Creation 两种模式的小文本、状态色、diff、主按钮与焦点指示器，并强制显式/自动浅色一致、双层焦点环和 forced-colors 规则。初版暴露 48 个根主题失败，扩展后继续抓到 Graphite 自动浅色 Creation 主按钮文字、旧高特异性焦点环以及 Creation 局部背景求值问题；当前 305/305 通过。`useDialogFocus` 新增稳定语义键，在设置内切换工作台/创作导致 opener 重挂载后仍恢复到新设置按钮。真实浏览器完成 Graphite/Carbon/Amber、Amber Creation 与工作台重挂载回环，恢复自动主题 + Graphite + 工作台后日志为 0。native smoke schema v3 在冷启动关闭后复用同一隔离 HOME/WebView2 profile 启动第二个进程，candidate 冻结 8 秒冷启动和 6 秒 warm 预算；当前源码 production Wails 两轮稳定响应均为 1.516 秒，边界无泄漏并完成清理。完整前端、Desktop、根模块与文档/发布门禁均通过；production 数据为 entry JS 624,233 B、initial JS 1,213,450 B、initial CSS 611,424 B、largest JS 704,186 B、initial files 5。commit 与远端证据待完成；受保护文件继续排除。
