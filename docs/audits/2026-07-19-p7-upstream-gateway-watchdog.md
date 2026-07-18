# 2026-07-19 P7 上游增量与 Gateway systemd watchdog 审计

> 主上游区间：DeepSeek Reasonix `40ef98de..2335d0df`
>
> 战略/参考增量：OpenAI Codex、Hermes、MiMo Code、Scream Code、Kimi Code
>
> 目标：只吸收能增强 Reames 单一 Go/Wails runtime 的机制，并把真实外部证据与仓库内证据分开。

## 结论

本轮审查了 6 个发生变化的仓库。Reasonix 是一级主源码上游；Codex 与 Claude Code 是二级战略代码
上游（Codex 本轮有新 SHA，Claude Code 无变化）；其余项目只提供机制信号。

| 项目 | 区间 | 提交/非 merge | 文件 | 变更量 | Reames 决策 |
|---|---|---:|---:|---:|---|
| Reasonix | `40ef98de..2335d0df` | 2/2 | 56 | `+4595/-205` | 不整体复制 Fleet；区域字体和 named profile 留作体验候选 |
| OpenAI Codex | `56395bdd..b8b61bc6` | 1/1 | 3 | `+210/-10` | 压缩 rollout 诊断不适用于当前无压缩 Reames session；保留逻辑路径回归信号 |
| Hermes | `4c96172d..581e92e4` | 38/37 | 98 | `+9984/-656` | 采用无 libsystemd 的通知、健康驱动 watchdog 和有界停止；Electron zoom 修复不直接移植 |
| MiMo Code | `72e9002e..d48888f7` | 2/1 | 6 | `+1/-1` | 仅内置技能目录重命名，无 Reames 缺口 |
| Scream Code | `6474e33a..53fa61b2` | 4/4 | 46 | `+413/-1515` | Goal wizard/Provider/多代理帮助为 UX 候选；WolfPack 加固由现有预算/权限/worktree/effects 覆盖 |
| Kimi Code | `3086e470..4f3c7240` | 1/1 | 15 | `+846/-110` | Web 命令前台默认；Reames `serve`/`gateway run` 已有等价进程语义 |

Reasonix 的机器账本是
[`reasonix-generation-40ef98d-2335d0d.json`](../upstreams/reviews/reasonix-generation-40ef98d-2335d0d.json)。

## Reasonix 两笔增量

### `5eac2d79`：区域字体

Reasonix 为 UI、正文和代码三类区域增加独立字体、大小、行高和字距设置。Reames P5 刚关闭受控
Theme Pack，当前主题 schema 故意只承载语义颜色、有限 recipe 和本地 raster scene；把任意字体名称或
CSS 字体来源塞进主题包会扩大跨平台字体发现、缺失字体回退、可访问性和供应链边界。因此本轮不直接
移植。后续若真实用户需要，应该建立独立的本机字体偏好合同，并验证 Windows/macOS/Linux 回退、
缩放、CJK overflow、代码等宽和 Safe Mode，而不是修改 Theme Pack v1。

### `2335d0df`：profile-aware parallel subagent Fleet

Reasonix 新增 named profile、并发 scheduler、`write_paths` claim 和路径约束工具。Reames 不整体采用，
原因不是功能不重要，而是两边的副作用边界不同：

- Reames writer child 使用独立 Git branch/worktree、workspace lease 和显式交付事务；
- durable child effect journal 在写前保存 intent，按 parent session/workspace/tool-call/cursor 归并；
- 整棵委派树共享并发、step、token、duration 和 cancellation 预算；
- Plan Mode、permission、sandbox、checkpoint 与 root verification 不允许 child 旁路。

Reasonix 的同工作区 `write_paths` 锁不能替代上述隔离。named profile 发现、可视化 fleet 状态和更清晰的
角色选择仍是后续 UX 候选；若采用，必须装配到既有 `internal/boot`、Controller 和 worktree delivery，
不能建立第二套 scheduler/effects 状态机。

## Hermes Gateway 信号与采用

Hermes 本区间同时包含 Python/Electron billing、Discord 重连、Bedrock/streaming、安装器和桌面状态等
大量非同构变化。Reames 本轮直接吸收与 M6 生命周期同构的最小机制：

- Linux systemd 安装可选 `--watchdog-sec DURATION`；默认 `0` 保持 `Type=simple`，启用后渲染
  `Type=notify`、`NotifyAccess=main` 和稳定秒/毫秒 `WatchdogSec=`；
- 只在 shared recovery preflight 完成、Gateway 启动成功且至少一个非禁用 adapter 为
  `running`/`degraded` 后发送 `READY=1`；
- watchdog cadence 由 systemd 的 `WATCHDOG_USEC` 决定，`WATCHDOG_PID` 不匹配只禁用心跳，不阻断
  readiness/stopping；
- 所有已配置 adapter 都进入 `closed`/`error` 时发送 unhealthy `STATUS=` 并停止 `WATCHDOG=1`，让
  service manager 按 watchdog 策略恢复；
- SIGINT/SIGTERM 先发送 `STOPPING=1`，再执行 30 秒有界 `Gateway.Stop()`；超时后主进程返回，避免
  service manager 永久卡在 stopping；
- `internal/systemdnotify` 纯 Go、无 CGO/libsystemd，覆盖 filesystem 和 Linux abstract Unix datagram。

该健康信号证明的是 Reames 进程主循环仍在调度、且至少一个 adapter 仍处于可用状态投影；它不证明
真实 IM 远端、DNS、OAuth/token、websocket 或平台事件回环仍健康。真实渠道 liveness 仍需外部回环证据，
不能由 localhost fixture 或 `running` 状态替代。

最终新增 `6f42d6a5`/`036b6595` 让 Electron 在每次 full load 和 resize/move 后重申 zoom，Linux noisy
事件使用 trailing debounce。Reames 使用 Wails/WebView2 窗口创建时的
原生 `ZoomFactor`，修改后显式重启，并没有 Electron `webContents.reload()`/`did-finish-load` listener
生命周期；目前没有 WebView2 resize 丢失缩放的真实证据，因此不制造第二套即时缩放路径。该信号保留给
后续 Windows/macOS/Linux native resize、跨显示器和 renderer recovery smoke；若真实复现，应在 Wails
window/controller 边界修复而不是复制 Electron 事件名。`5f95b251`/`581e92e4` 只更新 contributor map。

Hermes 的 stale stream delta、多模态 interim content 和 Bedrock reasoning-floor 信号转入 P8 Provider
能力矩阵；billing/subscription、Python shutdown runtime、Electron 第二桌面栈和 Nous endpoint 不采用。

## Codex 战略增量

`b8b61bc6` 修复 `doctor thread inventory` 对 `.jsonl.zst` rollout 的识别：数据库仍保存 canonical plain
path，扫描器把压缩 sibling 映射到逻辑路径，plain/compressed 同时存在时优先 plain，损坏 zstd 记录 scan
error 但不误报 DB stale，压缩临时文件不算完成 rollout。

Reames 当前 session、event log 和 doctor bundle 都只支持未压缩 `.jsonl`，没有 `.zst` rollout 或
SQLite thread inventory，因此没有可修复的同构 bug，本轮不引入 zstd 依赖。该提交保留为战略回归信号：
若 P8/P9 或后续存储优化引入压缩 session，必须把物理文件与 canonical logical path 分离，定义
plain/compressed 冲突优先级、临时文件过滤、损坏压缩件与 stale index 的不同错误语义，并同步 doctor、
backup、migration 和 session tool 测试。

## 其他参考增量

- **MiMo Code**：只把内置技能 bundle 从 `mimocode` 重命名为 `mimocode-docs`。Reames 的 Skill/Package
  identity、digest、权限与生命周期独立，不跟随外部目录名。
- **Scream Code**：删除 `/loop` 并扩展 `/goal` 配置向导；后续又重排 responsive welcome、Think badge、
  `/config diy` 自定义 Provider 与 `/model diy` 多代理帮助入口。Reames Goal 已有持久 FSM、Todo/evidence、
  step/token/time 预算与恢复。向导体验可后续采用，但不改变 Goal 完成门槛。WolfPack 的 result aggregation、
  per-child timeout、permission scoping 和状态同步由 Reames 现有委派树预算、child receipt、permission、
  writer worktree 和 durable effects 覆盖。欢迎页品牌 ASCII 和动态 badge 不复制；Provider/多代理发现文案作为
  P8/P9 UX 信号，不建立第二套配置入口。
- **Kimi Code**：`kimi web` 和 TUI `/web` 改为默认前台。Reames `serve`、`gateway run` 本来就是前台
  进程，后台化必须显式交给 service manager，因此无代码缺口。

## 后续 Codex 级能力路线

用户已明确将 Codex 与 Claude Code 提升为仅次于 Reasonix 的二级战略代码上游：未来在 Reames 使用
DeepSeek、GPT 与 Claude 各家的原生模型时，不仅协议原生支持，三家的代码级能力也必须持续跟进。
同时要求具备 Codex 类插件、CDP 和相关扩展能力。
这不是“OpenAI-compatible 能返回文本”即可完成，正式拆为：

1. **P8 Provider parity**：OpenAI Responses API、GPT reasoning/stream/tool/multimodal/usage/error 语义；
   Anthropic Messages/Claude thinking/tool/vision/cache 能力矩阵与确定性 fixture。
2. **P9 Codex-class extensibility**：审计现有 MCP/Plugin/Skill/Hook 与 Codex 能力差距，补发现、安装、
   更新、禁用/撤销、诊断、版本兼容、headless/App-Server 投影，同时保持 M5 TUF/fresh-human/sandbox。
3. **P10 first-party Browser Control**：由 CLI/Desktop/Serve 共用的 CDP 浏览器控制，覆盖 managed browser
   与显式 attach、tab/navigation/DOM/input/screenshot/download、双栈 loopback 和端口协议探测；所有操作
   接入 permission、sandbox、凭据隔离、提示注入防护、evidence 与取消/恢复。

可选 Playwright MCP、`web_search` 和 `web_fetch` 都不能冒充 P10 第一方 Browser Control；ChatGPT
订阅登录也不等于官方 OpenAI API/Responses 支持，除非未来另行建立明确授权和安全合同。

## 验证与外部边界

聚焦测试覆盖 systemd environment 解析、filesystem/abstract datagram、通知报文清洗、unit 渲染与
fail-closed 参数、`READY → WATCHDOG → STOPPING` 生命周期、全 adapter unhealthy 时停止心跳，以及
bounded stop。最终完成声明还要求 Root/Desktop/Frontend 全量、race、脚本与文档合同、六目标、
clean clone 和最终 push 提交对应的 CI/CodeQL。

以下继续保持 `external-blocked`：

- linger-enabled 干净 Linux 节点的 logout/reboot 常驻与真实 watchdog kill/restart；
- 真实 OpenAI/Anthropic API key 和 Provider 回环；
- 真实飞书/QQ/微信应用的文本、审批、取消、恢复与掉线重连；
- 公开 registry、人员/HSM 密钥仪式、生产签名/notarization 和 compromise drill。
