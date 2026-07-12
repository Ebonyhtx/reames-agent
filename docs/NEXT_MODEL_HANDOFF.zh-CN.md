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
- M3 进行中：性能、模态焦点、显示缩放、主题对比度以及三平台启动 readiness 已形成分层证据。partial fix commit `0cdfef1` 的普通 CI `29211082959` 为 8/8、CodeQL `29211082955` 为 3/3；candidate run `29211086907` 的 Linux/macOS 与 Windows native startup 均通过，但 interaction 再次证明 tab 条目恢复后 controller 仍可能 `ready=false`。当前未提交前端批次在首次 history hydrate 前等待权威 controller readiness，并完成本地 production 原生闭环。
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
- 该批 production 数据：entry JS 624,233 B、initial JS 1,213,450 B、initial CSS 611,424 B、largest JS 704,186 B、initial files 5；当前源码 Windows production Wails 冷/warm 稳定响应均为 1.516 秒。

commit `fb37db1` 已推送；普通 CI run `29209093041` 为 8/8、CodeQL run `29209093033` 为 3/3。Windows installer candidate 仍是手动 workflow，不能用普通 CI 替代。

## 当前 M3 候选批次关键证据

```text
python -m unittest scripts.test_smoke_desktop_candidate   PASS (14)
python -m unittest scripts.test_smoke_desktop_native      PASS (21)
python scripts/check_release_contracts.py                 PASS
Linux native candidate runner                             PASS (4.538s first / 5.567s stable)
macOS native candidate runner                             PASS (0.575s first / 1.872s stable)
Windows hosted startup candidate                          PASS (5.032s first / 6.032s stable; warm 2.000s stable)
Windows hosted interaction recovery                       FAIL (partial-fix controller-ready race)
完整 root/Desktop/frontend/docs 门禁                      PASS
```

当前远端 `main` 为 `0cdfef1`。Linux 证据包含隔离状态与可见 X11 窗口；macOS 只声明隔离状态 readiness。Windows 启动预算连续两次在 hosted runner 通过，不能因同一 job 后续 interaction 失败而回退该结论。最新 frontend readiness 修复的本地 production Wails 已通过 19 请求、五类失败恢复、停止和同会话重启恢复，hosted installer 复核待下一次集中 push 后执行。

## 下一执行顺序

1. 完整验证并提交 frontend controller-ready hydrate 门槛与更新后的竞态合同/审计；保持受保护文件排除。
2. 集中 push 一次后，只手动复跑一次三平台 `Desktop candidate` 并读取两个 Windows JSON；启动三平台应保持通过，interaction 必须在 hosted installer 上恢复原会话/工作区/用户与助手消息。
3. 后续补 Windows High Contrast/屏幕阅读器原生抽查；M3 主体验充分后再进入干净云节点 CLI + Gateway + feedback 与真实飞书回环。

## 长期未关闭项

- M3 原生 Desktop 日用化、可访问性和启动/bundle 性能门槛。
- 干净 Linux/云节点的 CLI、Gateway、feedback、日志、备份、升级回滚。
- 真实飞书/Lark 文本、审批、取消与恢复回环。
- plugin 权限 manifest、内容完整性和安装预览。
- 生产签名、notarization、provenance 与 updater 信任链，保持 `external-blocked`。

长期 GOAL 尚未完成。即使本批全绿，也只能声明该批验收完成，不能声明整个项目完成。

## 当前未提交 Desktop restart recovery 批次

后端 `ListTabs()` 门闩已在 `0cdfef1` 交付，但远端证明 tab 条目恢复不等于 controller ready。当前 frontend `syncActiveTabFromBackend()` 会先显示 restored tab shell，再以 30 秒有界轮询等待 `ListTabs.ready`/`startupErr`，刷新 metadata/sessionPath 后才读取 history；ready event 与轮询并发仍只 hydrate 一次，等待后重新执行 tab guard，其他 fork 路径保留 6 秒门槛。ready-event/missed-ready/tab-switch/new-session 定向测试 6/8/61/15 全通过；最新 production Wails SHA-256 `E744BD7705C71962873ED56BD10775AB7412ED517B51843DBBDD8C29808F2305` 已完成 19 请求原生闭环，重启前后 session path 一致、临时 HOME 清理成功、边界变化为 0。frontend typecheck/test:all/build 与 docs/public/release 合同已通过；当前 bundle 为 entry 624,409 B、initial JS 1,213,626 B、initial CSS 611,424 B、largest JS 704,186 B，预算内。commit/push 与 hosted candidate 复核待完成，受保护文件继续排除。
