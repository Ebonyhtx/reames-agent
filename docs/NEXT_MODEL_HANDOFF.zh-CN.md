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
- M3 进行中：性能、模态焦点、显示缩放、主题对比度、三平台启动 readiness 与 Desktop 重启恢复竞态已形成分层证据。commits `0cdfef1`、`9d31735` 关闭后端 tab restore 与前端 controller-ready 两层门槛；candidate run `29211681563` 三平台全部成功，Windows interaction `recovery_verified=true`。多语言按需加载已在 `bbdddde` 交付，普通 CI `29214262280` 为 8/8、CodeQL `29214262276` 为 3/3。当前未提交候选继续将 browser dev mock、VirtualMenu/TanStack 和设置中心 CSS 移出基础首启图，并新增递归首次使用图预算；frontend、Go、Python、合同、production Wails、原生启动与 19 请求交互的完整本地验收均已通过。
- M6 进行中：Gateway service、headless smoke 和 feedback 本地闭环已具备；真实 IM 与干净云节点仍缺外部证据。

唯一执行顺序以 `docs/DEVELOPMENT_PLAN.md` 为准。

## 最新已交付 M3 批次

- 关闭态与次级界面按真实打开状态拆包，真实 `dist` 强制 entry、initial JS/CSS、最大 asset 和请求数预算。
- Windows native smoke schema v2 记录首次可见/响应/稳定响应，本地源码 production 固定 8 秒硬预算；本地稳定响应 2.016 秒。后续 hosted installer candidate 依据真实 runner 证据独立采用 15 秒门槛。
- 最终 production 数据：entry JS 621,270 B、initial JS 1,209,699 B、initial CSS 607,374 B、largest JS 704,186 B、initial files 5。

详见 `docs/audits/2026-07-12-m3-desktop-bundle-budget.md`。commit `e147854` 已推送；CI run `29196957695` 为 8/8、CodeQL run `29196957665` 为 3/3。

- commit `9d8368c` 统一 Settings/History/Command Palette/Shortcuts/Image Viewer/Onboarding 的模态焦点生命周期，补 combobox/listbox 读屏合同与退出动画后 opener 恢复。
- 同一批次关闭 Windows 显示缩放最后选择胜出、Go 原子持久化、待重启状态、立即重启和失败回滚。
- 该批 production 数据：entry JS 623,888 B、initial JS 1,213,105 B、initial CSS 607,569 B、largest JS 704,186 B、initial files 5。

commit `9d8368c` 已推送；普通 CI run `29203366720` 为 8/8、CodeQL run `29203366703` 为 3/3。

- commit `fb37db1` 建立六主题 × 深浅色 × 普通/Creation 的 305 项对比度/焦点合同，修复 opener 重挂载焦点恢复，并将 Windows 同 HOME warm relaunch 纳入 schema v3 与 6 秒 candidate 门槛。
- 该批 production 数据：entry JS 624,233 B、initial JS 1,213,450 B、initial CSS 611,424 B、largest JS 704,186 B、initial files 5；该批源码 Windows production Wails 冷/warm 稳定响应均为 1.516 秒。

commit `fb37db1` 已推送；普通 CI run `29209093041` 为 8/8、CodeQL run `29209093033` 为 3/3。Windows installer candidate 仍是手动 workflow，不能用普通 CI 替代。

- commit `bbdddde` 将简中/繁中词典移出基础首启图，首个 React frame 前读取权威保存语言，auto 才检测 OS，并用递归 locale 图和恰好两个 locale chunk 守卫首启。
- 该批 production 数据：entry JS 623,577 B、base initial JS 984,616 B、最坏本地化首启 1,100,036 B、initial CSS 611,424 B、largest JS 704,186 B。

commit `bbdddde` 已推送；普通 CI run `29214262280` 为 8/8、CodeQL run `29214262276` 为 3/3。详见 `docs/audits/2026-07-13-m3-lazy-locale-budget.md`。

## 当前 M3 候选批次关键证据

```text
python -m unittest scripts.test_smoke_desktop_candidate   PASS (14)
python -m unittest scripts.test_smoke_desktop_native      PASS (21)
python scripts/check_release_contracts.py                 PASS
Linux native candidate runner                             PASS (4.538s first / 5.567s stable)
macOS native candidate runner                             PASS (0.575s first / 1.872s stable)
Windows hosted startup candidate                          PASS (11.016s first / 12.016s stable; warm 2.000s stable)
Windows hosted interaction recovery                       PASS (19 requests / recovery_verified=true)
完整 root/Desktop/frontend/docs 门禁                      PASS
```

当前远端 `main` 为 `bbdddde`。Linux 证据包含隔离状态与可见 X11 窗口；macOS 只声明隔离状态 readiness。Windows installer 的 native 与 interaction 两份 JSON 均通过，重启前后 session path 一致、清理成功、边界变化为 0、errors 为空。这些历史 candidate 证据与 `bbdddde` 的普通 CI/CodeQL 均不能替代当前未提交性能批次的原生和远端验收。

## 下一执行顺序

1. 只显式暂存当前主首启图批次文件，集中 commit/push 一次并守候普通 CI 与 CodeQL；远端全部通过前不得声明本批已交付。
2. 下一批补模态背景 `inert`/`aria-hidden` 隔离、稳定 dialog ID、transcript `role=log` 与 Windows UIA 可访问性 smoke；High Contrast/屏幕阅读器结论仍保留手动或外部证据边界。
3. M3 主体验充分后再进入干净云节点 CLI + Gateway + feedback 与真实飞书回环。

## 长期未关闭项

- M3 原生 Desktop 日用化、可访问性和启动/bundle 性能门槛。
- 干净 Linux/云节点的 CLI、Gateway、feedback、日志、备份、升级回滚。
- 真实飞书/Lark 文本、审批、取消与恢复回环。
- plugin 权限 manifest、内容完整性和安装预览。
- 生产签名、notarization、provenance 与 updater 信任链，保持 `external-blocked`。

长期 GOAL 尚未完成。即使本批全绿，也只能声明该批验收完成，不能声明整个项目完成。

## 当前未提交证据与下一批边界

当前未提交性能批次把 browser dev mock 改为首次使用时动态加载，把 `@tanstack/react-virtual` 留在 `VirtualMenuImpl` 延迟 chunk，并由新的 lazy `SettingsPanelRoute.tsx` 装载设置中心约 120 KB 源码 CSS，使直接导入 SettingsPanel 的组件测试不依赖 CSS loader。共享 responsive `.drawer--wide` 与 App BotDetail 规则仍留在 `styles.css`，避免异步 CSS 到达顺序改变级联。production bundle 为 entry 624,876 B、base initial JS 865,678 B（5 files）、最坏本地化首启 981,098 B、initial CSS 511,305 B、largest JS 704,186 B；相对已交付 `bbdddde`，base 与 localized initial 均减少 118,938 B，CSS 减少 100,119 B，entry 因共享模块重分组增加 1,299 B。首次使用图为 browser mock 960,568 B、VirtualMenu 890,931 B、Settings 1,053,773 B JS + 611,477 B CSS，均受递归 manifest 硬预算保护。

完整本地证据：`pnpm test:all` 与 `pnpm build` 在 `SettingsPanelRoute` 修复后通过；Go build/vet/internal tests、Desktop `go test .`、Python 脚本套件（79 tests、2 skipped）、upstream issue 合同（3）、docs/public/release/deploy/tool contracts 与 Wails production build 全部通过。localhost 已证明 browser mock 启动、Settings 延迟 CSS 最终布局和 VirtualMenu 键盘交互，console 无 warning/error。

最终 Windows production 可执行文件为 48,045,568 B，SHA-256 `0F5E5BB8BCC4F7605D387F84D69079F54192A2E67C1F288056BBFBC8B6A12CE3`。native startup smoke 的 cold 首次可见/响应 1.015 秒、稳定 2.015 秒（8 秒预算），warm 首次可见/响应 0.516 秒、稳定 1.516 秒（6 秒预算），清理、临时目录和状态边界均干净。interaction smoke 完成 19 次 loopback provider 请求，覆盖成功、invalid key、rate limit、stream interruption/retry、permission denial/write blocked、tool timeout、stop/recovery、持久化，以及 production Wails 中 Settings lazy route 的真实打开/关闭；boundary/errors 为空且清理成功。

当前批次本地验证已完整闭合，只剩显式暂存、集中 commit/push 与本批远端 CI/CodeQL。当前改动尚未 commit/push，不能借用 `bbdddde` 的远端结果声明本批已交付。详见 `docs/audits/2026-07-13-m3-main-graph-css-split.md`。

下一批可访问性方向是模态背景隔离、稳定 dialog ID、transcript 日志语义和 Windows UIA 合同；Windows High Contrast/屏幕阅读器实际行为保持手动或外部证据边界。受保护文件继续排除，只显式暂存本批路径。
