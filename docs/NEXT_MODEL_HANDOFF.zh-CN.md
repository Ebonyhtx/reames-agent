# 下一位模型接手交接文档

日期：2026-07-14

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
- M3 已关闭：commit `68218d6` 的普通 CI `29262192635` 8/8、CodeQL `29262193090` 3/3；candidate `29262541971` 的 Linux/macOS installed 与 Windows native/interaction/strict accessibility 全部通过。Windows interaction 完成 19 请求、五类恢复、停止和同 project/workspace/session 恢复，问题导航后 user/assistant 均 present + onscreen，`recovery_verified=true`。
- M6 进行中：Gateway service、headless smoke、feedback 和四渠道 `gateway setup` 确定性闭环已具备；WSL2 真实 systemd user manager 已通过带空格路径的 install、同名重装、status、restart、stop/start、journal、webhook readiness 和 uninstall，但 `Linger=no`，真实 IM、logout/reboot 与干净云节点仍缺外部证据。当前工作树又补齐 Linux user-scope install 事务、敏感 home/state backup/verify/restore，以及带 `.previous` 和 `--rollback` 的 CLI updater；尚未 commit/push，必须以本页后述本地证据和最终 CI 为准。

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

## 最新 M3 + 当前 M6 批次

- M3：`bb13da3` 的 pre-ready history replacement 保留为竞态硬化；`68218d6` 使 smoke 与 Transcript 的 `content-visibility: auto` 契约一致，分别记录 UIA presence/onscreen，严格 InvokePattern 调用英文/简中/繁中的“问题 1”导航，并要求首轮 user/assistant 同时 onscreen；最终综合门禁还要求磁盘双消息、同 session、composer 和 onboarding 缺失。
- M6：新增 `internal/gatewaysetup` 和 `reames-agent gateway setup`，覆盖 Feishu/Lark、QQ、Weixin、workspace/model/connection ID、secret-env-only、显式 access、`--reset-access`、redacted dry-run、严格 TOML、原子幂等写入。
- M6 Linux：修复 uninstall 的 disable → delete → daemon-reload 顺序、systemd 指令编码和绝对路径门禁；unit 使用 crash-safe 原子写，`install --start-now` 会显式 enable/restart/is-active。同名重装后旧 webhook 必须失效、新 webhook 必须生效。
- M6 Linux 安装事务：仅 user-scope install 会在执行前快照旧 unit bytes/mode 和 enabled/active，写入后用 `systemd-analyze --user verify`，并对写入、reload、enable、restart、is-active、取消与回滚失败做分层恢复。旧定义写回或恢复 reload 失败时 fail closed，不再触碰未知 manager 状态；macOS/Windows/system scope/uninstall 没有同等保证。
- M6 备份恢复：新增 `reames-agent backup create --offline`、`backup verify`、`backup restore --dry-run|--offline`，覆盖 home/state 分根、已知凭据排除、source identity、manifest/ZIP/path/size 门禁、仅不存在的新目标、权限收紧与多根进程内回滚。归档仍敏感，内嵌 hash 只证明自洽，Windows 依赖目标目录 ACL，跨根没有 durable crash journal。
- 发布：修复 CLI updater 的官方仓库、`reames-agent-<os>-<arch>` 精确资产名和 Windows `reames-agent.exe` 包内名称，并纳入 release contract；当前进一步实际执行 staged/installed `version`、保留 `<executable>.previous`、自动恢复失败发布，并支持同目录互斥锁保护的 `upgrade --rollback`。命令只提示 `gateway restart`，不会静默重启服务。
- CI 稳定性：hosted Windows 暴露 Fork/Rewind workspace prompt 测试的 1 秒 turn 等待上限和 stub controller 清理缺口；测试现为替换 controller 注册 `Close`，并仅在慢 runner 失败路径上放宽到 10 秒。
- 测试：四渠道、状态保留、byte-for-byte 幂等、损坏配置和误传 secret fail closed、setup → doctor → service dry-run；ready-event/missed-ready/pinned page；完整前端与本地 production Wails native/accessibility/interaction。

candidate `29262541971` 的 installer SHA-256 为 `2BDAA4E9FC5E87CD498A9E528D49F480B8277B7D9B4514081EF11E2C674D6C19`，installed executable SHA-256 为 `927FEF13D22B0F609DDC72FA35D0BF07451CAC6402BA2CBEBA38456E8D8010F1`；Windows native 7.031/2.016 秒通过。interaction 跳转前 marker present 但 offscreen、assistant absent，调用问题 1 后双消息 present + onscreen；strict accessibility 的 skip focus、dialog、背景隔离、dialog focus、opener focus 和 strict invoke 均通过。三份 smoke 均无边界变化和 errors。

## 下一执行顺序

1. 在干净 Linux/云节点以 linger-enabled 低权限用户完成 CLI + gateway setup/doctor/service + feedback、logout/reboot，并用新命令完成备份恢复、独立 SHA256 比对、公开签名 release 升级/回滚与 Gateway restart 实启闭环。
2. 使用真实飞书凭据完成文本、审批、取消与恢复回环；凭据不可用时保持 `external-blocked` 并推进不受阻项。
3. 进入 M4，统一 Goal/Plan/Task/证据/Checkpoint 状态机及失败恢复测试。

## 长期未关闭项

- NVDA/Narrator 实际听感与 Windows High Contrast 人工验证（不由自动 UIA smoke 替代）。
- 干净 Linux/云节点的 CLI、Gateway、feedback、日志、logout/reboot，以及新备份/升级命令的真实运维演练。
- 真实飞书/Lark 文本、审批、取消与恢复回环。
- plugin 权限 manifest、内容完整性和安装预览。
- 生产签名、notarization、provenance 与 updater 信任链，保持 `external-blocked`。

长期 GOAL 尚未完成。即使本批全绿，也只能声明该批验收完成，不能声明整个项目完成。

## 当前证据边界

新 Wails executable 为 48,052,736 B，SHA-256 `889986ABB11E97FDEDBFFC48700600503E6984F866E3E774B2FE751993583F24`；native cold/warm 稳定响应 1.515/1.516 秒，strict accessibility 3/3 InvokePattern。最新 interaction 用 52.1 秒完成 19 请求、五类失败恢复、停止和同 session path 重启；跳转前 user/assistant 均存在 UIA 树但均 offscreen，调用问题 1 后均 onscreen，`recovery_verified=true`，边界变化和 errors 为空。

上述本地源码 production 证据现由 `68218d6` 的 installed candidate `29262541971` 补齐：Linux/macOS installed 通过，Windows native/interaction/strict accessibility 全链路通过。M6 WSL systemd smoke 的 binary SHA-256 为 `B7AD96B6B0C2C3B10978C31D2CFA637583938E274C61C9508CF38A8A32419315`，初始/重装/restart/start PID 为 `420/465/505/546`，最终 `LoadState=not-found`、`errors=[]`；其 `Linger=no`，不得外推为 logout/reboot 证据。NVDA/Narrator、Windows High Contrast、真实云节点、真实飞书和签名安装器仍分别是手动、远端或 `external-blocked` 证据。受保护的 `.agents/`、`artifacts/`、`docs/audits/2026-07-09-reference-feature-gap-map.md` 禁止暂存。

本批未提交工作树的本地证据包括：`go build ./...`、`go vet ./...`、`go test ./internal/... -count=1 -timeout 300s`，Gateway/Home Backup race，CLI updater 实际编译并执行候选 `version`，以及 Linux/macOS/Windows amd64/arm64 的 CGO-disabled 编译。最终提交前仍须重跑文档/部署/release 合同和 Desktop Go suite；push 后必须等待新的 CI/CodeQL，不能沿用上述历史 run 证明本批。
