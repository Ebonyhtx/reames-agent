# Codex / Hermes 提交前最新增量代码审计

> 日期：2026-07-20
>
> 范围：Codex `3e2f79727a4e..7844386e3de0`，Hermes `299e409f15aa..1b17015f7a8d`
>
> 层级：Codex 为二级战略代码上游；Hermes 为三级机制参考

## 1. 审查方法

两个本地镜像均执行 `fetch --prune --tags`、`pull --ff-only`，确认 `main...origin/main` 无差异后，逐提交、
逐文件检查源码、测试和提交说明。随后运行 `check_upstreams.py --deep` 取得完整 diff 证据。早期逐项
`--accept` 暴露了审查与写锁之间的移动 HEAD/TOCTOU：Codex 在审查后继续前进，旧命令会静默接受新 HEAD。
本批因此新增 `--accept-revision ID=FULL_SHA`，禁用未绑定 SHA 的 `--accept`、`--accept-all` 和
`--update-lock`；最终只用人工审过的完整 SHA 写锁，远端不一致时 fail closed。

## 2. Codex `3e2f7972..7844386e`

本区间 7 个非 merge 提交、28 个变化文件、`+1041/-304`；前 6 个位于 Rust TUI，最后一个位于
headless exec/App-Server 路径：

| 提交 | 代码级结论 | Reames 决策 |
|---|---|---|
| `aa982319` Speed up TUI Markdown layout | 表格宽度批量收缩、styled spans 一次 flatten、URL 跨 span 识别、OSC hyperlink 单向重映射 | 已有/非同构。Reames 表格按列比例一次计算，不做按字符缩减；`x/ansi` 负责 ANSI/CJK wrapping，链接显示为文本和目标而非 Codex 的 OSC hyperlink range。保留未来 terminal hyperlink 性能信号，不复制 Rust 数据结构。 |
| `74bfbda9` Keep incremental rendering with visualization context | 只有真正出现 visualization directive 时才全量重绘；仅存在 context 不再破坏稳定前缀 | 延后到 P9/P10。Reames CLI 当前没有 Codex inline visualization context；Desktop Mermaid 是独立 React 组件，不会把 context 注入 Bubble Tea Markdown renderer。未来第一方 visualization/CDP surface 必须保留“无 directive 时稳定前缀”合同。 |
| `854a82db` Track TUI command completion separately from output | App-Server output delta 不能作为 command 完成标志；duration/result 才结束，interrupted 时保留 partial output | 已有等价并进入 P9 App-Server 合同。Reames 使用独立 `event.ToolProgress` 与 `event.ToolResult`，progress 只更新运行卡，result 才结算；中断轮次 partial tool card 为 `LocalOnly`，不会进入 Provider。现有 CLI 和 interrupted recovery 测试覆盖该边界。 |
| `d0516cfe` Avoid buffering replay-irrelevant thread notifications | raw response item、MCP progress、realtime audio/transcript 和 command/process delta 不进入 per-thread replay buffer，但仍更新 turn/approval 状态 | P9 合同采用。Reames 目前从 canonical transcript 恢复，没有 Codex 同构的 App-Server raw replay store；Serve 慢订阅者已有有界 drop。未来 P9 replay 只能保存恢复所需 canonical 事件，不得持久缓存 raw audio/progress/output delta。Desktop `asyncRuntimeEmitter` 是另一条无界 live queue，需独立 benchmark/backpressure，不能冒充已有等价。 |
| `6a54efb7` Cache finalized Markdown history rendering | 按宽度、主题和终端颜色缓存 finalized Markdown；含 visualization directive 时因文件状态可变而绕过缓存 | 已有等价/性能信号。Reames CLI finalized Markdown 已在提交时形成 ANSI transcript，resize 只做 ANSI wrapping；Desktop Markdown/Assistant 组件使用 React memo 与 `useMemo`。Mermaid/未来动态 visualization 仍不得套用静态缓存。 |
| `c86b1be3` Avoid cloning file changes in TUI diff rendering | Codex 改为消费或借用 `DiffSummary` 的 path/`FileChange`，避免为排序和两种渲染路径深克隆内容 | 当前非同构。Reames CLI 每个 tool 只携带一个 `event.FileDiff`，Go string/value 传递不复制 diff backing bytes，也没有 `HashMap<PathBuf, FileChange>` 行对象克隆；Desktop 直接从单个 wire diff 生成虚拟化行。保留为 P9 大 diff benchmark 信号，不复制 Rust lifetime 结构。 |
| `7844386e` Backfill completion items only for the active exec turn | child `turn/completed` 可与 primary turn 共用事件流；只有先匹配 primary thread+turn 的完成事件才允许 `thread/read` backfill | 当前已有隔离，P9 强制采用。Reames `subSinkFor` 只转发 child tool/usage，过滤 child `TurnDone`，顶层 `Controller.runGuarded` 才产生唯一最终完成事件，因此当前不会由 child completion 触发 history 读取。未来 App-Server/多 Agent event stream 必须在任何 completion backfill、结算或 unsubscribe 前同时绑定 thread ID 与 turn ID，并补 child-first fixture。 |

本区间没有 OpenAI Responses wire、reasoning、hosted tool、App-Server protocol、插件或 Browser/CDP 的新增，
因此不改变 P8 原生 GPT provider 完成范围，也不能据此宣称 P9/P10 已完成。

## 3. Hermes `299e409f..1b17015f`

本区间 5 个非 merge 提交、18 个变化文件、`+267/-142`：

| 提交 | 代码级结论 | Reames 决策 |
|---|---|---|
| `5f2bfb66` scope cron list to active profile | Hermes 多 profile 聚合 endpoint 默认返回全部 profile，Desktop 必须显式传 scope | 当前非同构。Reames 的 cron store 是单一 `REAMES_AGENT_HOME/cron.json`，没有跨 profile 聚合 endpoint；若未来引入 named profile，list/read/write 必须同时绑定 profile，不能只路由进程。 |
| `60811ced` adaptive thinking for Kimi-family Anthropic endpoints | Kimi/Moonshot Anthropic-compatible wire 支持 `thinking.type=adaptive`、`display=summarized` 与 `output_config.effort`，续轮需要回放无签名 thinking block | 部分已有、补窄缺口。Reames 已发送 adaptive thinking、支持 low/medium/high/xhigh/max 并保存原序 `ReasoningBlocks`，但只回放带 signature 的 thinking。现在仅对官方 Kimi/Moonshot host 或 Kimi family model，允许回放由 Anthropic stream 捕获的无签名 `ReasoningBlock`；通用 `ReasoningContent` 仍不能跨 provider 伪装成原生 wire，Claude 仍严格要求 signature。 |
| `e361c5e2` spaced Windows Git paths | Electron `simple-git` 拒绝带空格的 custom binary，需要对内部可信路径启用其 escape hatch | 非同构。Go `os/exec` 原生接受带空格的绝对 executable path，Reames 也没有 `simple-git` custom-binary validation；不引入不安全 PATH fallback。若以后增加 Git resolver，只能信任进程内解析的系统路径。 |
| `3345b3cd` make Desktop perf `--spawn` work and capture baseline | 通过 package metadata 解析 Vite 8 CLI、隔离 spawn 后等待启动抖动收敛、移除会触发 DNS/embed 的 raw autolink，并记录真实 darwin-arm64 基线 | 采用机制，不复制 Electron harness。Reames 后续 Desktop/CDP 性能门槛必须使用隔离 home/profile、显式 settle、无网络噪声的 fixture，并把平台/设备/连接状态写入基线；当前 `history-performance-benchmark` 仍是合成微基准，不能冒充冷启动/流式帧 pacing 证据。 |
| `1b17015f` tidy session-color pass | await/ternary/docstring/空值等价整理，无行为变化 | 忽略实现。Reames 没有 Hermes 的 Nanostores/project-color 数据流；仅保留“同一 workspace membership 投影驱动所有颜色消费者”的体验信号。 |

Kimi 的 upstream 提交说明包含真实 API 验证，但 Reames 本批没有用户 Kimi key，因此本地证据只证明 request/replay
合同与 Claude 隔离，不把 fixture 写成真实公网回环。

## 4. Reames 实现与验证

实现：

- `internal/provider/anthropic` 增加精确 Kimi/Moonshot endpoint/model-family 识别；
- Kimi 只可回放已由 Anthropic SSE 捕获的无签名 `thinking` block；
- Claude 继续拒绝无签名 thinking；lookalike host 不会误识别；
- Provider 切换时的通用 `ReasoningContent` 不进入 Kimi provider-native history。

已通过：

```text
go test ./internal/provider/anthropic -count=1
go test ./internal/provider/... ./internal/config/... -count=1
python scripts/check_upstreams.py --deep `
  --accept-revision codex=7844386e3de08febd13075eaaaf0e6f9dbe52c58 `
  --accept-revision hermes=1b17015f7a8d0c0d68b1f08aa389538e7fd172e3
```

最终冻结：Codex `7844386e3de08febd13075eaaaf0e6f9dbe52c58`，Hermes
`1b17015f7a8d0c0d68b1f08aa389538e7fd172e3`；接受后 11/11 `changed_count=0`。
