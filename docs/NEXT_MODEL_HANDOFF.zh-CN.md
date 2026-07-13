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
- M3 进行中：可访问性批次已由 `827e0b4` 交付，普通 CI `29229513429` 8/8、CodeQL `29229513359` 3/3。candidate `29229871453` 的 Linux/macOS 与 Windows native 通过，但 Windows interaction 两次在同一重启 transcript 可见性断点失败；当前未提交 pinned-history 预载修复已通过完整本地 production native/accessibility/interaction，仍需新 commit 的 installed candidate。
- M6 进行中：Gateway service、headless smoke、feedback 和本批四渠道 `gateway setup` 确定性闭环已具备；真实 IM 与干净云节点仍缺外部证据。

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

## 当前 M3 + M6 批次

- M3：恢复 tab 在 controller `ready=false` 时先从 pinned session event log 预载历史，仍锁定发送；ready 后复用缓存并补 ancillary，关闭托管 runner 慢启动导致的空 transcript 窗口。
- M6：新增 `internal/gatewaysetup` 和 `reames-agent gateway setup`，覆盖 Feishu/Lark、QQ、Weixin、workspace/model/connection ID、secret-env-only、显式 access、`--reset-access`、redacted dry-run、严格 TOML、原子幂等写入。
- 测试：四渠道、状态保留、byte-for-byte 幂等、损坏配置和误传 secret fail closed、setup → doctor → service dry-run；ready-event/missed-ready/pinned page；完整前端与本地 production Wails native/accessibility/interaction。

candidate `29229871453` attempt 1/2 的 Windows interaction 均失败；本批 pinned-history 修复即使完成本地门禁和普通 CI，也不能把源码 Wails 证据冒充 installed candidate。

## 下一执行顺序

1. 守候本批集中 push 后的普通 CI/CodeQL，不为纯证据另做碎片提交。
2. 对新 commit 运行 Desktop candidate，必须让 Windows installed interaction 越过两次失败断点，并实际执行 strict accessibility step。
3. 远端关闭后进入干净云节点 CLI + gateway setup/doctor/service + feedback，再做真实飞书回环。

## 长期未关闭项

- M3 原生 Desktop 日用化、可访问性和启动/bundle 性能门槛。
- 干净 Linux/云节点的 CLI、Gateway、feedback、日志、备份、升级回滚。
- 真实飞书/Lark 文本、审批、取消与恢复回环。
- plugin 权限 manifest、内容完整性和安装预览。
- 生产签名、notarization、provenance 与 updater 信任链，保持 `external-blocked`。

长期 GOAL 尚未完成。即使本批全绿，也只能声明该批验收完成，不能声明整个项目完成。

## 当前证据边界

新 production bundle：entry 628,743 B、base initial JS 869,545 B、最坏本地化 984,999 B、initial CSS 511,305 B，预算通过。Wails executable 48,050,688 B，SHA-256 `A4D22842BB5C107AA1E9F6829947046338FBD15826AADF035AFCDD0234F4E8A0`；native cold/warm 0.5/1.5 秒，strict accessibility 3/3 InvokePattern，interaction 19 请求与五类失败恢复、停止和重启恢复通过，边界变化和 errors 为空。

这些仍是本地源码 production 证据。新 commit 的安装器 candidate、NVDA/Narrator、Windows High Contrast、真实云节点、真实飞书和签名安装器仍分别是远端或 `external-blocked` 证据。受保护的 `.agents/`、`artifacts/`、`docs/audits/2026-07-09-reference-feature-gap-map.md` 禁止暂存。
