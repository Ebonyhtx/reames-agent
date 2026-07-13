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
- M3 进行中：性能、模态焦点、显示缩放、主题对比度、三平台启动 readiness 与 Desktop 重启恢复竞态已形成分层证据。主首启图/CSS 拆分已由 `7d07c89` 交付，普通 CI `29216174519` 为 8/8、CodeQL `29216174514` 为 3/3。当前未提交候选继续关闭真正模态背景隔离、Transcript 最终答复语义和 Windows 严格 UIA accessibility smoke；localhost Browser、production Wails accessibility/native/interaction 与完整 frontend/Python/Go/Desktop/docs 门禁已通过，本批远端结果仍待收口。
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

- commit `7d07c89` 将 browser dev mock、VirtualMenu/TanStack 和设置中心 CSS 移出基础首启图，并用递归 manifest 图守卫 base、本地化和三条首次使用路径。
- 该批 production 数据：entry 624,876 B、base initial JS 865,678 B、最坏本地化 981,098 B、initial CSS 511,305 B；browser mock 960,568 B、VirtualMenu 890,931 B、Settings 1,053,773 B JS + 611,477 B CSS。

commit `7d07c89` 已推送；普通 CI run `29216174519` 为 8/8、CodeQL run `29216174514` 为 3/3。详见 `docs/audits/2026-07-13-m3-main-graph-css-split.md`。

## 当前 M3 候选批次关键证据

```text
python -m unittest scripts.test_smoke_desktop_candidate   PASS (14)
python -m unittest scripts.test_smoke_desktop_native      PASS (21)
python -m unittest scripts.test_smoke_desktop_accessibility PASS (10)
python scripts/check_release_contracts.py                 PASS
Linux native candidate runner                             PASS (4.538s first / 5.567s stable)
macOS native candidate runner                             PASS (0.575s first / 1.872s stable)
Windows hosted startup candidate                          PASS (11.016s first / 12.016s stable; warm 2.000s stable)
Windows hosted interaction recovery                       PASS (19 requests / recovery_verified=true)
Windows source production strict accessibility             PASS (3 InvokePattern actions)
完整 root/Desktop/frontend/docs 门禁                      PASS
```

当前远端 `main` 为 `7d07c89`；普通 CI `29216174519` 8/8、CodeQL `29216174514` 3/3。Linux 证据包含隔离状态与可见 X11 窗口；macOS 只声明隔离状态 readiness。Windows installer 的历史 native/interaction JSON 均通过，但本批新增 accessibility workflow 尚未触发安装器 candidate，不能用本地源码 Wails 或普通 CI 冒充。

## 下一执行顺序

1. 最终审查当前 accessibility 批次 diff，只显式暂存本批路径，集中 commit/push 一次并守候普通 CI 与 CodeQL。
2. 远端全绿后继续 M3 原生 Desktop 日用化；安装器 accessibility candidate、NVDA/Narrator 与 Windows High Contrast 分别保留远端人工/外部证据边界，不为纯证据单独碎片 push。
3. M3 主体验充分后再进入干净云节点 CLI + Gateway + feedback 与真实飞书回环。

## 长期未关闭项

- M3 原生 Desktop 日用化、可访问性和启动/bundle 性能门槛。
- 干净 Linux/云节点的 CLI、Gateway、feedback、日志、备份、升级回滚。
- 真实飞书/Lark 文本、审批、取消与恢复回环。
- plugin 权限 manifest、内容完整性和安装预览。
- 生产签名、notarization、provenance 与 updater 信任链，保持 `external-blocked`。

长期 GOAL 尚未完成。即使本批全绿，也只能声明该批验收完成，不能声明整个项目完成。

## 当前未提交证据与下一批边界

当前未提交 accessibility 批次为七个真正模态层提供稳定 ID：`command-palette-dialog`、`settings-dialog`、`history-dialog`、`shortcuts-cheatsheet-dialog`、`image-viewer-dialog`、`onboarding-dialog`、`heartbeat-dialog`。`useDialogFocus` 以每次打开独立 lease 管理顶层，沿 dialog→body 路径用 `inert`/`aria-hidden` 隔离背景并精确恢复原属性，覆盖嵌套、退出动画、快速重开、动态 portal 和 inert portal 首次聚焦重试；lease 继承直接父控件与原始 opener 链，模态替换或 StrictMode effect 重放后不会把焦点丢到 body。只有顶层响应 Escape，`PromptShelf aria-modal=false` 保持非阻断。

主 Transcript 使用唯一 `transcript-log`、`role=log`、`aria-live=off`、运行/加载 busy 状态；独立 `transcript-announcer` 只在真实运行结束后提交一次最终 assistant 文本。hydration、tab/session 切换、history append、rewind/undo 和 `/clear` generation 不回放历史。History 预览使用唯一 `history-preview-transcript-log` 并禁用第二个 announcer。

Windows UIA 驱动新增 localized control type、focused/focusable、offscreen、ARIA role/properties，并提供严格 `invoke_pattern()`，缺失或调用失败时不允许坐标回退。新 accessibility smoke 以隔离 HOME 验证 `app-main`、log/status、skip→composer、Settings dialog/关闭焦点、六个背景 ID 从 UIA 树消失、关闭后 opener 恢复；candidate Windows job 已接线并上传独立 JSON，但本批未触发安装器 candidate。

当前定向与 production 证据：dialog focus 35 + palette 2、Transcript 54、accessibility smoke 单测 10、`pnpm build` 通过。最终 bundle 为 entry 628,571 B、base initial JS 869,373 B（5 files）、最坏本地化 984,827 B、initial CSS 511,305 B、browser mock 964,263 B、VirtualMenu 894,626 B、Settings 1,057,628 B JS + 611,477 B CSS、largest JS 704,186 B，均在硬预算内。无热更新 localhost Browser 的嵌套隔离、双 Escape、opener 恢复和唯一 Transcript seam 通过，warning/error 为 0。

最终 Windows production 可执行文件为 48,050,176 B，SHA-256 `8E008415A9D331ABFFA63864CD67B5818FFC74F8FD5D8984790C7371F8590CD7`。严格 accessibility smoke 的 3 个动作全部为 InvokePattern，所有断言、清理和状态边界通过；native cold 首次可见/响应 0.500 秒、稳定 1.500 秒，warm 0.500/1.500 秒，满足 8/6 秒预算；interaction smoke 完成 19 请求、五类失败恢复、停止、持久化和重启恢复，`boundary_changes=[]`、`errors=[]`。

当前改动尚未 commit/push，不能借用 `7d07c89` 的远端结果声明本批已交付。完整 frontend/Python/Go/Desktop/docs 门禁已通过；只显式暂存本批路径并集中 push，受保护的 `.agents/`、`artifacts/`、`docs/audits/2026-07-09-reference-feature-gap-map.md` 继续排除。NVDA/Narrator 实际听感、Windows High Contrast、签名安装器和本批三平台 candidate 仍是独立手动或外部证据。详见 `docs/audits/2026-07-13-m3-modal-isolation-transcript-uia.md`。
