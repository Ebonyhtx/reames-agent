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
- M3 进行中：性能、模态焦点、显示缩放、主题对比度以及 Windows 本地 cold/warm 启动批次均已远端全绿。commit `de893c0` 又为 Linux/macOS candidate 建立隔离状态 readiness 与 10 秒预算，普通 CI `29209717326` 为 8/8、CodeQL `29209717324` 为 3/3；candidate run `29209723618` 的 Linux/macOS 原生 jobs 已通过，Windows job 暴露托管首次安装观察窗偏短，当前批正在校准并复跑。
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
Windows hosted installer candidate                        FAIL (11.531s first; old 12s observation)
完整 root/Desktop/frontend/docs 门禁                      PASS
```

当前远端 `main` 为 `de893c0`。Linux 证据包含隔离状态与可见 X11 窗口；macOS 只声明隔离状态 readiness。Windows 远端同一 installer 已在本地安装复核，20 秒观察窗、15 秒 cold/6 秒 warm 参数下稳定响应分别为 2.000/1.500 秒，待 hosted runner 复跑。

## 下一执行顺序

1. 将 Windows hosted installer candidate 改为 20 秒观察窗、15 秒 cold/6 秒 warm 预算，保持本地源码默认 8/6 秒；定向门禁通过后集中 commit/push 一次。
2. 只手动复跑一次三平台 `Desktop candidate` 并读取原生 JSON；Windows 必须在 hosted runner 形成连续三次响应和同 HOME warm 证据，Linux/macOS 应保持回归通过。
3. 后续补 Windows High Contrast/屏幕阅读器原生抽查；M3 主体验充分后再进入干净云节点 CLI + Gateway + feedback 与真实飞书回环。

## 长期未关闭项

- M3 原生 Desktop 日用化、可访问性和启动/bundle 性能门槛。
- 干净 Linux/云节点的 CLI、Gateway、feedback、日志、备份、升级回滚。
- 真实飞书/Lark 文本、审批、取消与恢复回环。
- plugin 权限 manifest、内容完整性和安装预览。
- 生产签名、notarization、provenance 与 updater 信任链，保持 `external-blocked`。

长期 GOAL 尚未完成。即使本批全绿，也只能声明该批验收完成，不能声明整个项目完成。

## 当前未提交 Windows candidate 校准批次

`smoke_desktop_native.py` 的本地默认值不变。workflow 单独把首次安装后的 hosted cold 预算从 8 秒调为 15 秒、观察期从 12 秒调为 20 秒，warm 继续保持 6 秒；发布合同同步冻结三项参数。依据是 run `29209723618` 首次响应 11.531 秒但观察窗不足以形成稳定三连，以及同一远端 installer 的本地真实安装复核。完成定向测试、一次 commit/push 和三平台复跑前不得声明 Windows installer candidate 已通过；受保护文件继续排除。
