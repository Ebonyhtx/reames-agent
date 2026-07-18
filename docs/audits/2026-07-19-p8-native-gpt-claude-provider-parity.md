# P8 原生 GPT / Claude Provider parity 审计（2026-07-19）

## 结论

P8 的仓库内闭环已经从“兼容聊天端点”升级为两条显式第一方协议：OpenAI 使用
Responses API，Anthropic 使用 Messages API。二者继续复用同一个 `provider.Provider`、
`boot.NewProvider`、Controller 和 Agent loop；CLI、Desktop、Serve、Gateway 不建立 GPT/Claude
专用运行时。没有真实 API key 时，完成声明限于 localhost SSE fixture、请求 wire、错误、取消、
中断、缓存与会话回放合同；真实鉴权、计费、供应商网络行为仍为 `external-blocked`。

## 战略上游证据

| 上游 | 冻结 SHA | 本批使用的代码级证据 | 决策 |
| --- | --- | --- | --- |
| OpenAI Codex | `312caf176a8f` | `codex-rs/codex-api/src/common.rs` 的 `ResponsesApiRequest`；`codex-api/src/sse/responses.rs` 的 item/event、usage、failed/incomplete 与 EOF 语义；`protocol/src/models.rs` 的 message/function_call/function_call_output/reasoning/image wire；`models-manager/models.json` 的 `gpt-5.6-sol`、图像、并行工具与 effort 能力；`b8b61bc6..35eaf3ff` 的 TUI 增量和 `35eaf3ff..312caf17` 的 Realtime V3 初始历史项均已逐文件复核 | 采用原生 `/responses` transport、encrypted reasoning replay 和可由 Reames 抽象承载的合同；WebSocket/Responses Lite、freeform code-mode tool、App-Server 属于 P9，不混入 Provider 基础层；最新 Realtime 提交不改变 P8 HTTP wire |
| Claude Code | `07dcb0e13580` | 仓库不发布 CLI 核心实现，代码级可审部分是 plugin/example/schema；`CHANGELOG.md` 明确 Sonnet 5 默认、Opus 4.8 xhigh/ultracode 产品档位、effort 继承和 `redacted_thinking` 修复；Messages wire 继续由 Reames 的原生 fixture 证明 | 不根据 release note 猜造 Sonnet 5 API ID；采用已证实的 Messages/effort/redacted block 语义，精确新模型 ID 等官方可验证源或 `/models` 证据 |

OpenAI 公共 API 文档是模型可用性与 wire 能力的权威补充源，本批在 2026-07-19 复核了
[`Models`](https://developers.openai.com/api/docs/models)、
[`GPT-5.6 Sol`](https://developers.openai.com/api/docs/models/gpt-5.6-sol)、
[`Terra`](https://developers.openai.com/api/docs/models/gpt-5.6-terra)、
[`Luna`](https://developers.openai.com/api/docs/models/gpt-5.6-luna) 与
[`Model guidance`](https://developers.openai.com/api/docs/guides/latest-model)。三种 GPT-5.6 均明确支持
Responses、streaming、vision、普通 function calling、1.05M context 和 none 至 max effort。因此 Codex
catalog 的 `code_mode_only` 只能作为 Codex 产品 runtime 差分信号，不能用来否定公共 API 的 function tools。

Claude Code 的公开仓库不包含其闭源 CLI 主体，因此“战略代码上游”在这里意味着：对可公开的
plugin/schema/example 做源码 diff，对闭源产品变化只登记为待协议/fixture 复核的信号，不能把 changelog
本身当作 Reames 已完成证据。

## OpenAI Responses 能力矩阵

| 能力 | 状态 | 证据与边界 |
| --- | --- | --- |
| 显式协议选择 | 已完成 | `ProviderEntry.api_mode = "responses"`；空值保持历史 `chat_completions`，不按模型名或 URL 静默切换 |
| 请求输入 | 已完成 | system → `instructions`；user/assistant → message item；tool call/result → `function_call` / `function_call_output`；本地 citation/edit metadata 不改变 wire bytes |
| 参数边界 | 已完成 | Responses 不继承全局 Chat-era `temperature`；保留原生 reasoning effort 与 `max_output_tokens`，reserved `extra_body` 不能覆盖协议核心字段 |
| GPT reasoning | 已完成 | `reasoning.effort` + `summary=auto`；GPT-5.6 暴露 none/low/medium/high/xhigh/max 并默认 medium，GPT-5.4 不暴露 max 且默认 none；旧配置 `ultra` 仅兼容映射 wire `max`，不冒充 Codex 自动委派语义；summary delta 优先，raw reasoning 只作无 summary 回退；保留向后兼容的 `include=["reasoning.encrypted_content"]`（当前 `store=false` 默认也返回），将 opaque item 保存为 `openai_reasoning` block，并在工具续轮先于 function call/result 回放 |
| 官方模型预设 | 已完成 | 暴露 `gpt-5.6-sol`、`gpt-5.6-terra`、`gpt-5.6-luna` 与 `gpt-5.4`，默认 Sol；模型级 override 精确约束 effort/default，公共 context 为 1.05M。这里声明的是公共 Responses/function support，不是 Codex product parity |
| 工具与并行调用 | 已完成 | flat function tool schema、start/delta/done、稳定 call ID、多个并行 item；保留现有工具顺序与 canonical schema |
| 多模态 | 已完成 | vision-gated `input_image` data URL 与 low/high/original detail；空值保持 API auto；文本模型不注入图片字段 |
| usage/cache | 已完成 | input/output/total、cached input、reasoning output tokens 统一映射 `provider.Usage`；GPT-5.6 `cache_write_tokens` 与显式缓存策略/计费保持 P9 明示缺口，不用 miss 估算冒充独立写入证据 |
| 错误与中断 | 已完成 | `response.failed` / `response.incomplete` 为 typed error；`response.completed` 前 clean EOF fail closed；连接在无输出时可 bounded reconnect，已有输出后返回 `StreamInterruptedError`，不重复可见输出/工具调用 |
| 配置与桌面 | 已完成 | boot 传递 `api_mode`；TOML round-trip；Desktop 可选择 Chat Completions / Responses 且保存不丢字段；OpenAI 第一方预设使用 `https://api.openai.com/v1` 与 `/models` |
| 会话与隐私 | 已完成 | Responses reasoning block JSONL round-trip、compaction 估算与 tool-turn replay 有 fixture；opaque encrypted Data 不进入 Controller transcript DTO、Desktop history 或 `/export`，summary 仍可正常显示 |

未纳入 P8：Responses WebSocket/Lite、Codex freeform/code-mode、hosted tool item、programmatic tool
calling、显式 prompt caching 与 `cache_write_tokens`、`reasoning.context`、pro mode、Responses
multi-agent、远端 compaction 和 App-Server thread/event 协议。这些属于持续变化的 OpenAI/Codex 产品
能力，按 P9 统一扩展 Provider、`internal/control`、插件/headless 与权限/沙箱/evidence，不用普通 function
tools 冒充已经具备。

## Anthropic Messages 能力矩阵

| 能力 | 状态 | 证据与边界 |
| --- | --- | --- |
| 原生 Messages | 已完成 | `/v1/messages`、`anthropic-version`、`x-api-key`，兼容网关可显式 Bearer；system 顶层化并保持 user/assistant 交替 |
| thinking 与 effort | 已完成 | adaptive summarized thinking；low/medium/high/xhigh/max；`output_config.effort` 与 thinking 独立；enabled/disabled 兼容网关保持二值协议并在 boot 拒绝无效组合 |
| 签名与 redacted thinking | 已完成 | signed thinking 和 opaque `redacted_thinking` 按原始顺序持久化到 `ReasoningBlocks`，Data 不显示/解释；tool-use 续轮原样回放，legacy session 仍兼容 flattened reasoning/signature |
| 工具与视觉 | 已完成 | `tool_use` / `tool_result`、JSON delta、tool start/done；vision-gated base64 image block |
| prompt cache 与 usage | 已完成 | tools/system/conversation 三处受控 ephemeral breakpoint；合并 input/output/cache-create/cache-read，映射 finish reason |
| 错误与中断 | 已完成 | Anthropic SSE error 保留结构化 type/message；缺失 `message_stop`、未闭合 tool/reasoning block 均 fail closed；已有输出的 clean EOF 标记 `StreamInterruptedError`，交给共享 Agent tail recovery，不静默提交半截响应 |
| 会话恢复 | 已完成 | `ReasoningBlocks` JSONL round-trip、compaction 估算与 PostLLMCall 原文/签名约束有测试；opaque redacted Data 不进入展示 DTO |

第一方 Anthropic 预设固定仓库可核验的 `claude-sonnet-4-6`、`claude-opus-4-8` 与
`claude-haiku-4-5`。仅 Opus 暴露 xhigh/max；公开插件代码把 Haiku 4.5 列为 legacy-thinking 模型，
所以新增模型级 `thinking` override：Haiku 解析后显式省略 adaptive thinking 并关闭 effort，Sonnet/Opus
继续继承 adaptive wire。该 override 可经 TOML、Desktop DTO/保存和 preset clone 往返，不靠模型名在
Provider 内部临时猜测。Claude Code changelog 已宣布 Sonnet 5 默认，但没有足够可靠的公开 API 精确 ID，
因此不猜写；取得官方模型列表或真实 API 证据后再更新。Bedrock/Vertex/Foundry 的不同 wire/auth 不属于
当前 `anthropic` kind，不用兼容网关推断为已支持。

## 同批参考增量

- Hermes `581e92e4..bf391030`：headed Chromium 通过显式配置/环境变量启用，headed browser 跨 turn
  保留并由 idle reaper 最终回收，cloud/CDP 不注入 headed。它是 P10 Browser Control 的 lifecycle
  输入，本批不复制 Python/agent-browser runtime。后续 Electron `webContents` 的 Ctrl/Cmd+滚轮缩放
  统一进入其持久 zoom funnel；Reames 已有 Wails/WebView2 启动时 `ZoomFactor`、持久设置、显示面板和
  Ctrl/Cmd + `+/-/0` 字号快捷键，但没有可无重启热改的同构 WebView2 事件，因此不复制 Electron IPC/
  main-process 实现，保留为 Desktop 原生手势验证候选。后续 `e45d12642` 把 busy submit 的 interrupt
  移出 history lock，并在 session close 时先释放 resume lock 再慢 teardown；Reames Desktop 已在
  `closeTabRuntime` 中先移除/解锁再 teardown，Controller/会话租约也没有 Hermes 的全局 Python
  history/resume lock，因此作为并发回归信号而不复制。Slack/Google Chat 的可配置 working 文案和
  per-tool 状态已被 Reames `internal/bot/render.go` 的 `ToolDispatch/ToolProgress/ToolResult` 简洁投影覆盖；
  缺失 provider 行、Radix dialog wrapper、Electron shift-plus 与 dashboard schema 修复均是其前端栈局部问题。
- Hermes `bf391030..862b1b37`：真实 diff 修复了“空响应提示中包含 `max_tokens`，被错误分类为上下文
  溢出并触发压缩”的问题。Reames 的共享 `provider.ClassifyError` 合同存在同构匹配顺序缺口，已在上下文
  模式前拦截窄化的 empty-response advisory，将其标记为可重试 server error 且 `ShouldCompact=false`；
  同时保留 `max_tokens > context_window` 的真实溢出回归。当前 Agent 自动压缩由 usage 阈值驱动，尚未
  消费该分类合同，因此这是预防未来接线后的错误建议，不冒充已复现的运行时事故；也没有复制 Hermes
  的 Python classifier/runtime。
- Hermes `862b1b37..7a43ab04`：Desktop reconnect 通过 `session.activate`、持久 transcript 与 live
  projection 对账并保留本地 pending turn；Reames Desktop/Controller 同进程且已有 canonical event log、
  session transaction、hydration 与 pending prompt replay，不存在其 REST/RPC 双权威同构缺口。Discord
  增量引入 per-channel durable cursor、原消息身份、claim/去重账本、扫描上限，并只在 streamed final
  delivery 成功后推进 cursor，这是 M6 尚需实现的机制；`/model --once` 是不污染持久默认值的 P9 候选。
  computer-use 的 verify→background/foreground escalation、按 session+delivery-mode 隔离审批以及失效
  driver 自动重建进入 P10 验收，不在本批复制 Python/CUA runtime。
- OpenAI Codex `b8b61bc6..35eaf3ff`：`537e69ab66` 将 TUI 流式 Markdown 改为只重绘最后一个可变
  顶层块，并在 reference definition、inline visualization、宽度或 render mode 改变时回退 canonical
  全量渲染；`028edf8c1e` 修复 effort 快捷键重复发送当前模型。Reames 没有该 reasoning 快捷键事件链，
  显式 `/effort`/Desktop 选择需要重建不可变 Provider，故不存在重复 `UpdateModel` 缺口。Reames CLI
  当前按“已闭合 Markdown 前缀”流式显示，仍会随已闭合前缀增长重复渲染；这是有源码证据的性能候选，
  但必须连同列表、引用定义、表格、代码围栏、宽度变化和 canonical 等价测试一起专批实现，不能在 P8
  尾声用字符串拼接冒充同构修复。`35eaf3ffb0` 进一步只在可见 tail/status 真正变化时请求 redraw；
  Reames 当前已避免未闭合 Markdown 前缀重渲染，但 Bubble Tea 事件仍可能触发无变化 frame，和前述
  增量 renderer 一并进入专门 TUI 性能批次，不在 Provider P8 内混改渲染调度。
- OpenAI Codex `35eaf3ff..312caf17`：仅新增 Realtime V3/Frameless Bidi 的 role-bearing
  `initial_items`，拒绝旧 realtime 版本，并以最多 128 项、单项与总计 8192 估算 token 作为门槛。
  Reames 将这组代码级合同登记到 P9 WebSocket/App-Server；P8 继续保持 HTTP Responses 的显式边界。
- Scream Code `53fa61b2..5b1a9922`：仅 status-panel fixture 补 `wolfpackMode` 与 README 图片尺寸，
  没有 Reames Provider/Goal 代码缺口。
- Grok Build `98c3b243..7cfcb20d`：单次 monorepo 快照同步包含 permission、MCP OAuth、plugin、SSRF、
  session/terminal 大改。唯一立即采用项是其 overlap-aware MCP qualified-name 防歧义：Reames 现在拒绝
  model-visible 名中的第二个/重叠 `__`，并对原始双下划线组件做带 hash 的稳定归一化；TUF/plugin
  digest、SSRF、transient 401 已有等价或更强实现，RFC 9207 OAuth issuer 进入 P9 完整 auth helper，
  不在 P8 拼接半套 OAuth。

以上增量均在完成分类后逐项接受到 `docs/upstreams/upstreams.lock.json`；不使用 `--accept-all`。

## 验证与未关闭证据

仓库内门槛已通过：

- Root `build`/`vet`/`go test ./internal/...`，Desktop 独立 module `build`/`vet`/全测；
- Frontend `test:all`、production build、bundle budget，以及真实 Chrome 插件浏览器 smoke；
- OpenAI、Anthropic、Agent、Plugin、Control 的 race；六平台 CLI 与六平台 Guard 共 12 个
  `CGO_ENABLED=0` 构建；
- upstream、docs/public、deploy/release、installer、Gateway 与 Desktop smoke schema 合同；
- 候选提交对象的独立 `--no-hardlinks` clean clone：Root/Desktop 全量、空 `node_modules`
  安装后的 Frontend 全测/构建，以及 upstream/docs/public/deploy/release 合同。

最终公开交付仍要求最终 push 提交对应的 CI/CodeQL 全绿。fixture 证明协议和失败边界，不证明真实
OpenAI/Anthropic key、模型可用性、账单、缓存计费或区域网络；这些仍保持 `external-blocked`，获得凭据
后用单独的脱敏真实回环审计关闭。
