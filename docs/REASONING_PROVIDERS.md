# Reasoning controls by provider

Reames Agent exposes a single `/effort` knob (and the per-provider `effort` /
`thinking` config fields), but OpenAI-compatible backends disagree on *how*
chain-of-thought is requested on the wire. The `openai` provider adapts the
request shape per backend; this table is the reference for which protocol each
known backend uses and which parameters it honours or ignores.

## First-party native protocols

| Provider | Config | Reasoning control | `/effort` levels | Notes |
|----------|--------|-------------------|------------------|-------|
| OpenAI Responses | `kind = "openai"`, `api_mode = "responses"`, `reasoning_protocol = "openai"` | `reasoning.effort` + `reasoning.summary = "auto"` | `auto`, `none`, `low`, `medium`, `high`, `xhigh`; `max` where the selected model declares it | The official GPT-5.6 models expose `max`; legacy config value `ultra` maps to wire `max` but is not advertised because Codex ultra also carries automatic-delegation semantics. Responses uses item/event streaming and keeps a backwards-compatible `reasoning.encrypted_content` include for `store=false` tool-turn replay (current OpenAI APIs return it by default). Opaque Data is not displayed or exported. |
| Anthropic Messages | `kind = "anthropic"`, normally `thinking = "adaptive"` | `thinking` and independent `output_config.effort` | `auto`, `low`, `medium`, `high`, `xhigh`, `max` | Signed thinking and opaque `redacted_thinking` blocks are persisted in original order for tool-use replay. Compatible gateways that use `enabled`/`disabled` keep their binary vocabulary. |

The official Anthropic preset includes Sonnet 4.6, Opus 4.8, and Haiku 4.5. Only Opus exposes
`xhigh`/`max`. Haiku 4.5 is a legacy-thinking model in the public Claude Code plugin sources, so its
model override explicitly omits adaptive thinking and disables effort while Sonnet/Opus inherit the provider-level
adaptive wire.

The official OpenAI preset includes GPT-5.6 Sol, Terra, Luna, and GPT-5.4 over Responses. The GPT-5.6
models use a 1.05M context window, default to medium effort, expose `max`, and accept `original` image detail;
GPT-5.4 omits `max` and defaults to `none`. This is native public API support, not a claim that Reames already
implements Codex freeform/code-mode, Responses Lite, programmatic tool calling, multi-agent, hosted tools,
persisted-reasoning controls, pro mode, or App-Server semantics.

`api_mode` is explicit. An existing `openai` provider with an empty value remains
on `chat_completions`; Reames Agent does not infer Responses from a GPT model name.

## Auto-detected backends

These are recognised by base URL (see `internal/provider/openai/host.go`) and
get a tailored request shape automatically — no extra config needed.

| Provider | Base URL | Reasoning control | `/effort` levels | Notes |
|----------|----------|-------------------|------------------|-------|
| DeepSeek | `api.deepseek.com`, `*.deepseek.com` | `thinking.type` + `reasoning_effort` (depth) | `auto`, `disabled`, `high`, `max` | Thinking on by default; `disabled` turns it off via `thinking.type=disabled`. |
| MiniMax M3 | `api.minimaxi.com`, `*.minimaxi.com` | `thinking.type` (`adaptive`\|`disabled`) | `auto`, `adaptive`, `disabled` | No depth scale; `reasoning_effort` is omitted. |
| Zhipu GLM | `open.bigmodel.cn` / `*.bigmodel.cn`, `api.z.ai` / `*.z.ai` | `thinking.type` (`enabled`\|`disabled`) | `auto`, `enabled`, `disabled` | **`reasoning_effort` is silently ignored** by the endpoint, so reasoning is driven purely through `thinking.type`. |

## Everything else (Chat Completions `reasoning_effort`)

Any other OpenAI-compatible backend falls through to the standard
`reasoning_effort` scale (`low`\|`medium`\|`high`). Surveyed popular providers
that need **no special handling** because they already follow this convention:

Qwen (`dashscope.aliyuncs.com`), Moonshot/Kimi (`api.moonshot.cn`), Yi
(`api.01.ai`), SiliconFlow (`api.siliconflow.cn`), Stepfun (`api.stepfun.com`),
Groq (`api.groq.com`), Together (`api.together.xyz`), OpenRouter
(`openrouter.ai`), Perplexity (`api.perplexity.ai`), xAI (`api.x.ai`).

For a backend that uses a binary `thinking.type` toggle but is **not**
auto-detected, set the vendor-agnostic `thinking` field on the provider entry:

```toml
[[providers]]
name        = "my-glm-proxy"
kind        = "openai"
base_url    = "https://my-gateway.example.com/v1"
model       = "glm-4.6"
api_key_env = "MY_API_KEY"
thinking    = "disabled"   # enabled | disabled — emits thinking.type
```

## Troubleshooting

If a model keeps thinking when you asked it not to (or vice versa):

1. Check the table above — a backend may **ignore** the parameter you set
   (e.g. Zhipu ignores `reasoning_effort`; use `thinking`/`/effort` instead).
2. If the backend isn't auto-detected, set the explicit `thinking` field.
3. If the backend uses a non-OpenAI protocol entirely (e.g. Baidu Wenxin), the
   `openai` kind cannot drive its thinking mode — that needs a dedicated
   provider kind.

Distinguishing "provider ignores the field" from a Reames Agent bug starts here:
the request shape Reames Agent emits is fixed per the table, so a mismatch between
the table and observed behaviour is the provider's, not Reames Agent's.
