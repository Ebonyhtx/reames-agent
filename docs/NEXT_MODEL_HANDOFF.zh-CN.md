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
- M3 进行中：性能、模态焦点、显示缩放、主题对比度、三平台启动 readiness 与 Desktop 重启恢复竞态已形成分层证据。commits `0cdfef1`、`9d31735` 关闭后端 tab restore 与前端 controller-ready 两层门槛；普通 CI `29211678562` 为 8/8、CodeQL `29211678591` 为 3/3，candidate run `29211681563` 三平台全部成功，Windows interaction `recovery_verified=true`。当前本地候选又将简中/繁中词典移出基础首启图，并保留首帧语言正确性。
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

当前远端 `main` 为 `9d31735`。Linux 证据包含隔离状态与可见 X11 窗口；macOS 只声明隔离状态 readiness。Windows installer 的 native 与 interaction 两份 JSON 均通过，重启前后 session path 一致、清理成功、边界变化为 0、errors 为空。

## 下一执行顺序

1. 显式暂存并集中提交当前 locale 性能批次，push 一次后守候普通 CI 与 CodeQL；随后继续评估主流程与 CSS 初始拆分。
2. 后续补 Windows High Contrast/屏幕阅读器原生抽查；M3 主体验充分后再进入干净云节点 CLI + Gateway + feedback 与真实飞书回环。

## 长期未关闭项

- M3 原生 Desktop 日用化、可访问性和启动/bundle 性能门槛。
- 干净 Linux/云节点的 CLI、Gateway、feedback、日志、备份、升级回滚。
- 真实飞书/Lark 文本、审批、取消与恢复回环。
- plugin 权限 manifest、内容完整性和安装预览。
- 生产签名、notarization、provenance 与 updater 信任链，保持 `external-blocked`。

长期 GOAL 尚未完成。即使本批全绿，也只能声明该批验收完成，不能声明整个项目完成。

## 当前未提交证据与下一批边界

远端重启恢复完成证据已与本批文档合并，尚未单独 push。当前 locale 候选 production bundle 为 entry 623,577 B、base initial JS 984,616 B、含 manifest 递归依赖的最坏本地化首启 1,100,036 B、最大 locale 115,420 B、initial CSS 611,424 B、largest JS 704,186 B；相对拆分前 base initial 减少 229,010 B，最坏语言首启减少 113,590 B。启动前先读取轻量 `DesktopStartupSettings` 的权威保存语言，只在其为空时才读取 legacy 偏好；组件合同覆盖 OS=zh-TW、保存偏好=zh 时只加载简中，并在恢复 auto 后按 OS 加载繁中。完整 frontend、Go、Desktop Go、Python、docs/release/deploy/tool 门禁已通过；Windows production Wails SHA-256 为 `7CE473389D54DA662299DB03BE80FD571D95E7AA3A1E671FC8855866E67D0783`，冷/warm 稳定响应 1.516/1.500 秒，最终 UIA interaction 为 19 请求、五类失败恢复、停止和 `recovery_verified=true`。下一步只需完整 diff 复核、显式暂存、一次 commit/push 并守候远端；受保护文件继续排除。
