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
- M3 进行中：性能、模态焦点、显示缩放、主题对比度和 Windows cold/warm 启动批次均已远端全绿；最新 commit `fb37db1` 的普通 CI 为 8/8、CodeQL 为 3/3。当前未提交候选为 Linux/macOS candidate 建立隔离状态 readiness 与 10 秒预算，Linux 额外要求持续可见窗口；真实 runner 证据尚未产生。
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

- commit `fb37db1` 建立六主题 × 深浅色 × 普通/Creation 的 305 项对比度/焦点合同，修复 opener 重挂载焦点恢复，并将 Windows 同 HOME warm relaunch 纳入 schema v3 与 6 秒 candidate 门槛。
- 该批 production 数据：entry JS 624,233 B、initial JS 1,213,450 B、initial CSS 611,424 B、largest JS 704,186 B、initial files 5；当前源码 Windows production Wails 冷/warm 稳定响应均为 1.516 秒。

commit `fb37db1` 已推送；普通 CI run `29209093041` 为 8/8、CodeQL run `29209093033` 为 3/3。Windows installer candidate 仍是手动 workflow，不能用普通 CI 替代。

## 当前 M3 候选批次关键证据

```text
python -m unittest scripts.test_smoke_desktop_candidate   PASS (14)
python -m unittest scripts.test_smoke_desktop_native      PASS (21)
python scripts/check_release_contracts.py                 PASS
Linux/macOS native candidate runner                       PENDING
完整 root/Desktop/frontend/docs 门禁                      PASS
```

当前远端 `main` 为 `fb37db1`。本候选没有 push，也没有 Linux/macOS 原生证据；macOS 状态 readiness 不冒充窗口可见性。

## 下一执行顺序

1. 显式暂存 Linux/macOS readiness 候选，集中 commit、单次 push；守候普通 CI/CodeQL。
2. 手动触发一次三平台 `Desktop candidate`，守候 Linux/macOS 10 秒、Windows 8 秒 cold/6 秒 warm 真实 runner 证据；失败时读取原生 JSON 和 job 日志修复。
3. 后续补 Windows High Contrast/屏幕阅读器原生抽查；M3 主体验充分后再进入干净云节点 CLI + Gateway + feedback 与真实飞书回环。

## 长期未关闭项

- M3 原生 Desktop 日用化、可访问性和启动/bundle 性能门槛。
- 干净 Linux/云节点的 CLI、Gateway、feedback、日志、备份、升级回滚。
- 真实飞书/Lark 文本、审批、取消与恢复回环。
- plugin 权限 manifest、内容完整性和安装预览。
- 生产签名、notarization、provenance 与 updater 信任链，保持 `external-blocked`。

长期 GOAL 尚未完成。即使本批全绿，也只能声明该批验收完成，不能声明整个项目完成。

## 当前未提交 M3 Linux/macOS startup readiness 批次

`smoke_desktop_candidate.py` schema v2 新增首次状态就绪、首次可见窗口、连续三次稳定 readiness、最终 readiness、预算和稳定 failure kind。Linux readiness=隔离 Desktop 状态+可见 X11 窗口，macOS readiness=隔离 Desktop 状态；两个 workflow 均固定 10 秒预算与 12 秒完整观察期。14 个 candidate 单测、21 个 Windows native smoke 回归、root/Desktop/frontend 完整门禁和文档/发布合同通过。commit、push 与三平台 candidate runner 证据待完成；受保护文件继续排除。
