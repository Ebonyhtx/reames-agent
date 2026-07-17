# Reames Agent 源流与参考项目治理

> 状态：当前项目来源、上游跟踪和参考吸收的权威说明
> 更新：2026-07-17

## 1. 项目源流

Reames Agent 的来源关系不是“多个项目平级拼装”，而是清晰的四层结构：

```text
esengine/DeepSeek-Reasonix (main-v2)
        │ 主源码底座、持续跟踪的 primary upstream
        ▼
Reames Agent
        │ 独立产品、独立品牌、独立路线
        ├── F:\code-reference\*：机制、体验和测试思想的参考库
        └── F:\Reames-Lite：项目前身、产品契约和历史经验
```

### DeepSeek Reasonix

- 仓库：`https://github.com/esengine/DeepSeek-Reasonix`
- 本地镜像：`F:\code-reference\DeepSeek-Reasonix`
- 跟踪分支：`main-v2`
- Reames Agent 初始导入基线：`07c65c22`
- 角色：Go Agent runtime、provider/cache、tool、Desktop、Serve、插件与安全机制的源码底座。

Reasonix 是持续跟进的主上游，不是一次性参考。但 Reames Agent 已有品牌、产品和架构改造，不能直接合并上游主分支；应按功能批次审查和移植。

### `F:\code-reference`

这些仓库是“机制采矿场”，不是 vendor 目录：

- 可以吸收算法、交互模式、协议、测试案例和小型独立机制；
- 不整套复制另一个项目的 runtime、UI 或依赖体系；
- 每次吸收都要先证明 Reames Agent 当前确有缺口；
- 实现应落入 Reames Agent 的统一控制面和测试体系。

初始迁移中曾临时保留的 Hermes/Python/Electron/TUI/插件/测试快照已于
2026-07-17 从当前树删除；Git 历史负责追溯，`F:\code-reference\Hermes` 负责后续机制研究。
任何参考项目都不得再以整套 runtime、UI、依赖体系或测试树形式回到主分支。

### Reames Lite

`F:\Reames-Lite` 是新项目的前身。旧项目在桌面产品化时遇到工程瓶颈，因此转向以 Reasonix 的 Go/Wails 底座建立新项目。

它继续提供：

- 公共客户端边界和传输隔离思想；
- cache-first、metadata 不污染 prompt 的约束；
- 压缩、记忆、事件和工具契约；
- 中文产品经验以及旧实现踩坑记录。

它不再承担：

- 新项目主线开发；
- 桌面 Shell 或 runtime 底座；
- Provider/cache 的直接实现来源。

## 2. 产品北极星

Reames Agent 的长期目标是一个**以编程能力为最先成熟核心的全能 Agent**。

“全能”不等于把所有参考项目功能堆进同一个进程，而是通过一个可组合的 Agent 内核覆盖：

- 软件开发：理解、修改、验证和交付代码；
- 研究与知识工作：检索、阅读、归纳、证据追踪；
- 文件与数据工作：本地文件、结构化数据和多媒体上下文；
- 自动化：后台任务、Goal Loop、定时任务和长期执行；
- 记忆：项目记忆、用户偏好和可控的长期知识；
- 多入口协作：Desktop、CLI、Web、API 和 IM 渠道；
- 能力扩展：Tool、MCP、Plugin、Skill、Hook、LSP；
- 安全治理：权限、沙箱、密钥保护、审计、检查点和恢复。

所有能力必须汇入同一套会话、事件、权限和证据模型。若一个新入口需要复制 Agent loop，它就不符合“全能 Agent”的架构目标。

## 3. 参考项目职责

| 项目 | 主要吸收方向 |
|---|---|
| DeepSeek Reasonix | 主底座；runtime、provider/cache、Desktop、Serve、安全修复 |
| Hermes | 多渠道、远程部署、消息 envelope、错误分类与渠道运维 |
| OpenAI Codex | App Server、审批、线程/任务控制、Hook、LSP 与可靠消息 |
| MiMo Code | 设计 token、OKLCH、Hook 热更新和任务编排 |
| Impeccable | 设计规则、反模式检查和跨平台设计约束 |
| Scream Code | Goal Loop、Storm Breaker、主题与频道会话纪律 |
| AgentArk | Intent、安全边界、密钥、沙箱与 replay gate |
| Claude Code | Plugin/Skill/Hook 生态及市场体验 |
| Kimi Code | 桌面 Shell、浏览器通知、TUI 与 Provider 错误体验 |
| Reames Lite | 前身契约、cache/metadata 约束、压缩、记忆和中文体验 |

## 4. 上游更新规则

1. 参考仓库可先执行 `git pull --ff-only`；工作树不干净时不得覆盖本地研究。
2. 运行：

   ```powershell
   python scripts/check_upstreams.py --out-dir artifacts/upstream-watch
   ```

   完整运维、Issue 生命周期和接受版本流程见 `docs/upstreams/README.md`。

3. Reasonix 差异按以下顺序审查：
   - security / secret / sandbox；
   - provider / cache / stream；
   - agent / control / persistence；
   - Desktop bridge / recovery；
   - UI 与文档。
4. 其他项目只形成候选机制，不自动产生移植任务。
5. 每个移植批次必须记录：来源提交、缺口、Reames 适配、测试和缓存影响。
6. 不使用批量 merge 覆盖 Reames 的品牌、中文体验、公共边界和产品方向。

## 5. 2026-07-09 上游快照

本次已安全更新所有干净参考仓库。Reames Lite 存在未跟踪调研文件，保留现场，未执行 pull。

Reasonix 从初始跟踪点之后新增 133 个提交，值得优先审查的候选包括：

- tool output、历史会话和诊断包的密钥脱敏；
- 敏感环境变量过滤与敏感文件保护；
- Agent 配置文件写入的强制人工审批；
- ACP 客户端文件系统/终端能力；
- 工具工作区约束与客户端 I/O；
- Desktop 滚动意图、恢复副本、上下文窗口环和审批交互。

具体差异报告位于 `artifacts/upstream-watch/upstream-report.md`。

## 6. 2026-07-17 上游再同步

Reasonix `main-v2` 已审查至 `099879592742ddeb25b312347b4c37316e8b76f9`，相对原 reviewed
点累计 664 个提交（490 个非 merge）。本轮直接采用 MCP schema 隔离、保存凭据的环境/读取
保护、credential-free cache identity、Anthropic usage 合并、MiMo schema dialect，以及身份漂移
失败的类型化 re-verification 投影；Reames
适配、测试、拒绝项和后续门槛见
[`audits/2026-07-17-reasonix-upstream-sync.md`](audits/2026-07-17-reasonix-upstream-sync.md)。

本轮同时安全快进并审查 Hermes、Codex、MiMo、Impeccable、Scream Code、AgentArk、Claude Code
和 Kimi Code 的干净本地参考仓库。只形成三个新的路线图信号：

1. Reasonix 的 MCP identity receipt/launcher lock 已按 Reames control/security 边界完成 M5 P0，
   见 [`audits/2026-07-17-m5-mcp-identity-trust.md`](audits/2026-07-17-m5-mcp-identity-trust.md)；
2. Reasonix + MiMo 的 workspace lease/worktree 已按 Reames Controller、durable child journal 与 Windows 路径边界关闭 M4 持续加固 P1，见 `audits/2026-07-17-m4-writer-worktree-isolation.md`；
3. Reasonix offline Guard/Safe Mode P2 已按“复用现有恢复事务、不建立第二套 Agent/runtime”关闭，
   见 `audits/2026-07-17-p2-offline-guard-safe-mode.md`。

其他 UI、主题、发布、遥测、Python/Electron runtime 和参考项目依赖体系不进入 Reames 主树。
正式 reviewed SHA 保存在 `docs/upstreams/upstreams.lock.json`；`artifacts/upstream-watch/` 仍只
是本地分析输出，不提交。

同日最终 deep scan 将 Hermes 补审至 `bd0021233734`。其中 stored-id rotation 只适用于其压缩后
fork SessionDB continuation 的模型；PTY attach-token session scope 只适用于其 keep-alive registry；
basic-auth deny-list 修复只适用于其可插拔 dashboard auth provider；`_HERMES_GATEWAY` 清理只适用于
dashboard 在 gateway 进程内再拉起 gateway action 的递归模型。
Codex 补审至 `315195492c80`。
Hermes 的统一 worktree dialog 仅补强现有 P1 交付 UX；其 transcript 优化在 Reames hot/warm/cold
分页与长历史 benchmark 中已有等价或更强机制，compute host 不进入主树。Reames compaction 原地
重写同一 session path，且 session path recovery 已有 metadata/hydration 对账，因此不复制 Hermes 的
stored-id route/selection atom。Reames serve 的 controller/session 切换已有 `bindMu` + lease，password
auth 是编译内置 `authGate`，因此也不复制 Hermes 的 PTY registry key 或 plugin deny-list 修复。Codex 的 SQLite
migration writer-slot、Bedrock custom transport、approval payload structs、memory provenance 与
thread originator 变化均完成分类：除通用回归信号外，没有证据支持在本批新增产品路线或依赖。

## 7. 2026-07-17 P2 补审

Reasonix 已从 `099879592742` 补审到 `c966d0279629`。直接采用 `dae65e25` 的 stalled error-body
deadline、verified snapshot fast path、damaged event-log salvage 和 Desktop last-click-wins；
`f590a66e` 的 model picker 问题在 Reames 已有等价修复。`8f2c209a` 不用于放宽已关闭的
identity-bound MCP trust，`1bd5f04d` 保留为体验候选，`c966d027` 删除 Memory Compiler 的
产品决策不直接跟随。

参考项目按本批相关性补审至 Hermes `ef9e0c98f5c2`、Codex `e0ac6d0ec9ee`、MiMo
`28a0ced5e8e9`、Claude Code `67f390c9a0b1`、Kimi Code `7d393b56fb32` 与 Scream Code
`b37627ad8e3b`。Codex installed-app runtime projection 保留为 P3 Recovery Center 的只读运行态
投影参考；后续 realtime handoff、executor capability discovery、multi-exec workspace isolation 与测试
环境隔离只作为 token-economy/P3 和回归信号，Reames 已有父控制面/worktree 边界，不复制第二套
thread/exec-server 模型。Hermes 后续 auxiliary runtime turn/context 隔离、fallback 同步和 FTS 自愈
不适用于 Reames 当前 Go runtime/非 FTS 存储；Scream Code 新增仅为 README 下载链接。以上均无本批
代码采用项，MiMo 与 Kimi 的既有信号也已完成适用性分类。

本次明确使用逐项 `--accept`，没有使用 `--accept-all`。权威 SHA 以
`docs/upstreams/upstreams.lock.json` 为准，详细采用与拒绝理由见 P2 审计。
