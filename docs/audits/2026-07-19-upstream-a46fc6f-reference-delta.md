# Reasonix `a46fc6f` 与战略/机制参考最新增量审计

日期：2026-07-19

## 结论

本轮按三级治理完成最新代码级冻结，并逐项目执行显式 `--accept`，没有使用
`--accept-all`：

| 层级 | 项目 | 接受 SHA | 结论 |
|---|---|---|---|
| 一级主源码上游 | DeepSeek Reasonix | `8bb0e5493a7d` | 后续 LongCat/WebKit/export/Remote SSH 与 Theme Pack refresh 增量均完成代码级审查 |
| 二级战略代码上游 | OpenAI Codex | `0fb559f0f6e2` | 本轮无新增提交 |
| 二级战略代码上游 | Claude Code | `015170d3fd84` | 仅 CHANGELOG/feed；无可公开源码或协议路径变化 |
| 三级机制参考 | Hermes | `36f2a966c7f9` | 只提炼 Gateway、会话、插件、能力投影与运维信号 |
| 三级机制参考 | MiMo Code | `f24ce4eb7341` | Skill BM25 搜索作为 P9 候选 |
| 三级机制参考 | Scream Code | `22a2adaf8a45` | bug audit 与 component-scoped TUI render 作为可靠性/性能信号 |
| 三级机制参考 | Kimi Code | `df6899553962` | 多实例 Web、分片 read model、文件语义与本地 effort 诊断边界作为机制信号 |

其余 Impeccable、AgentArk、Grok Build、awesome-design-md 没有新提交。最终深度 Upstream Watch 在
`2026-07-19T15:28:22Z` 达到 11/11 `changed_count=0`；Reasonix 与 Scream 均逐项接受，没有使用
`--accept-all`。

## Reasonix：一级主源码上游

Reasonix 从 `2335d0df` 到最终 `8bb0e549` 的完整结论和机器账本位于：

- `docs/audits/2026-07-19-reasonix-2335d0d-a46fc6f.md`
- `docs/upstreams/reviews/reasonix-generation-2335d0d-a46fc6f.json`
- `docs/audits/2026-07-19-reasonix-a46fc6f-65fcd46.md`
- `docs/upstreams/reviews/reasonix-generation-a46fc6f-65fcd46.json`
- `docs/audits/2026-07-19-reasonix-65fcd46-8bb0e54.md`
- `docs/upstreams/reviews/reasonix-generation-65fcd46-8bb0e54.json`
- `docs/upstreams/reviews/reasonix-current.json`

本批采用测试用户状态隔离、Windows batch Hook、有界 session save lock 与 shutdown
recovery、每模型 context window、Mermaid/旧 WebKit/线性脱敏可靠性、Windows-safe
session filename，以及 native maximized resize cursor 修复。取消轮次 display buffer、
Linux 中键 PRIMARY/tmux 粘贴和 conversation width 有明确延后原因；远程 crash upload、
遥测和当前 TanStack 版本不存在的私有 API 明确不采用。

## Codex 与 Claude：二级战略代码上游

Codex 保持 `0fb559f0`，因此本轮没有新的 GPT/Responses、App-Server、插件、Hook、
LSP 或 Browser/CDP 代码差异。既有 P8/P9/P10 能力矩阵继续有效，不能因为“无变化”
而把尚未实现的 code-mode、Responses Lite/WebSocket、hosted tools、multi-agent、
App-Server 或第一方 Browser Control 写成已完成。

Claude Code `07dcb0e1..015170d3` 只有 `CHANGELOG.md` 和 `feed.xml`。2.1.215 的产品
信号是 `/verify` 与 `/code-review` 不再被 Claude 自主运行，需用户显式调用。Reames
当前也不会在没有用户/Agent 明确选择时暗中运行同名技能，因此无需代码改动。该区间
没有可公开的 Messages、thinking、cache、tool/vision 或插件 runtime 源码变化，故本轮
只接受发布信号，不从 changelog 反推协议 parity。

## Hermes：Gateway 与 runtime 机制信号

`614dc194..3a6e40b2` 变化很大，但 Hermes 仍是三级机制参考。代码级结论如下：

- Telegram 的 bounded drain 与 cause-agnostic reconnect watchdog 证明“进程仍活着但
  渠道永久失聪”是一类独立故障。Reames 原生 `net/http` 每次 long poll 都有 context
  deadline、指数退避、Stop timeout 和 `CloseIdleConnections`，已覆盖 Hermes 的具体
  httpx pool-close 卡死路径；若未来引入不遵守 context 的自定义 transport，必须新增
  独立 progress watchdog，不能只依赖 systemd `Restart=always`。
- Hermes 新增 outbound final-response obligation ledger：在模型已生成答案但平台 ACK
  前崩溃时，保存原答复并以可见“可能重复”标记恢复，避免重跑整个 turn。Reames 当前
  已有入站 claim、连续 cursor 和最终发送门禁，Telegram 失败时不推进 offset，但仍可能
  在重投时重新执行 Agent；“持久 outbound obligation/不重跑模型”登记为下一项 M6
  durability 缺口，不把现有 inbound ledger 冒充等价。
- per-session turn lease、conversation-scope funnel、compression durable anchor、最终
  tool-call tail 答复持久化和 byte-stable gateway context 都是有效回归信号。Reames
  现有 Controller/session lease、M4 turn transaction、每个 provider 回合先持久化
  assistant tool-call envelope、稳定 prompt/tool schema 已覆盖主要不变量；未来任何跨
  connection 共享同一 session path 的入口仍需并发测试。
- request-local Anthropic client 修复针对 Python SDK 在陌生线程关闭共享 TLS client 后
  误伤 SQLite FD 的运行时形状；Reames 使用 Go `net/http` request context，不共享或从
  watchdog 线程关闭 Anthropic SDK client，因此不复制该实现。
- unknown top-level config warning、统一 credential lifecycle、插件 i18n、性能基线、
  browser setup 和 cron per-job model 都是 P9/P10/配置治理候选。Reames 的 API key
  仍只通过受控 env/credential store 解析，不复制 Hermes 的 `.env + auth.json +
  config.yaml` 三副本模型。

## 其他机制参考

### MiMo Code

新增 `skill_search` 使用 BM25、query coverage、localized alias 和首轮 reminder。Reames
当前将有界 name/description index 固定在稳定 prompt，并按需 `run_skill`；当 Skill 数量
超过现有索引预算时，可在 P9 增加不污染 system prompt 的搜索工具，但必须保持稳定 tool
schema、权限范围和结果上限。本批不为三个提交建立第二套 Skill runtime。

### Scream Code

25 项全项目审计包含 compaction race、read_file 全量缓冲 OOM、FetchURL 无 timeout、
thinking off 被自动恢复、Anthropic 非 adaptive xhigh、后台 timer、symlink-unsafe workDir
比较等。Reames 已有 streaming/windowed `read_file` 与大文件内存测试、网络 request
deadline、P8 thinking/effort override、M4 compaction/turn transaction、context cancellation
和 `os.Root` writer 边界；这些作为现有门禁的独立佐证。大粘贴恢复、TUI committed-prefix
和 CJK/FTS 细节只保留为后续同构复现候选。

后续 `c6b24f60..22a2adaf` 把 thinking/loader/footer 的时间驱动刷新改为 component-scoped render，避免
动画 tick 触发整棵 TUI 重绘。Reames Desktop 使用 React state 和组件级更新，没有同构的
`requestRender` 全屏热路径，因此不复制实现；该不变量保留为 P9/P10 前端性能 benchmark 信号。

### Kimi Code

Kimi 将后台 server/service 树收缩为前台 `kimi web` 多实例 registry、把 read model 分成
16 个 shard，并明确 `stat` 跟随 symlink、`lstat` 检查链接自身。Reames 的 Gateway
service 是正式产品形态，不能因 Kimi 删除 daemon 而回退；多实例 registry 只作为未来
Serve/App-Server 发现机制参考。MiniDB shard 与 JS lock 不适用于当前 Go/Wails 状态；
文件工具继续按安全目的显式选择 follow/no-follow，并由 `os.Root`、confine 和 symlink
测试决定，不能机械统一为 Kimi 的 host-fs 语义。

## 未关闭边界

- outbound final-response obligation 持久恢复尚未实现；
- 取消轮次的只读 partial display history 尚未实现；
- 真实 OpenAI/Anthropic/DeepSeek 公网回环、真实 Telegram/飞书/QQ/微信掉线恢复、
  linger-enabled logout/reboot、生产 registry/signing 仍是 `external-blocked`；
- P9 Codex-class extensibility/headless 与 P10 第一方 Browser/CDP 仍按代码级矩阵推进，
  不能由已有 MCP/Plugin 或 Playwright MCP 冒充完成。
