# Reames Agent — Reames Lite 合约缺口分析

> 创建：2026-07-08
> 源：F:\Reames-Lite（Python 代码库，仅提取接口契约）

## ReamesClient 方法对照

| Reames Lite (Python) | Reames Agent (Go) | 状态 |
|---|---|---|
| `send_message(content, session_id)` | `Controller.Submit(input)` | ✅ 已有 |
| `send_message_stream(content, session_id)` | SSE `/events` 端点 | ✅ 已有 |
| `create_session(title)` | `POST /new` | ✅ 已有 |
| `resume_session(session_id)` | `POST /resume` | ✅ 已有 |
| `list_sessions(limit)` | `GET /sessions` | ✅ 已有 |
| `end_session(session_id)` | `POST /delete-session` | ✅ 已有 |
| `cancel()` | `POST /cancel` | ✅ 已有 |
| `get_config()` | `config.Load()` | ✅ 已有 |
| `update_config(updates)` | `config.LoadForEdit()` | ✅ 已有 |
| `get_status()` | `GET /status` | ✅ 已有 |
| `approve(id, allow)` | `POST /approve` | ✅ 已有 |
| `toggle_plan_mode()` | `POST /plan` | ✅ 已有 |
| `get_history()` | `GET /history` | ✅ 已有 |
| `get_context()` | `GET /context` | ✅ 已有 |
| `compact()` | `POST /compact` | ✅ 已有 |
| `health()` | `GET /health` | ✅ Phase 2 新增 |
| `ready()` | `GET /ready` | ✅ Phase 2 新增 |

## 结论

**所有 ReamesClient 方法均已覆盖，无 P0 缺口。** Reasonix 的 `control.Controller` + `internal/serve` 已完整实现了原 Reames Lite 的全部公开接口。

## Desktop API 协议

| 端点 | Reames Lite 描述 | Reames Agent | 状态 |
|---|---|---|---|
| `/health` | 健康检查 | `GET /health` | ✅ |
| `/schema` | 运行时 schema | `GET /status` (含使用量和缓存) | ⚠️ 部分（无独立 schema 端点） |
| `/rpc` | 通用 RPC | 多个 POST 端点（submit/cancel/approve/...） | ✅ 等价 |

## 类型映射

| Reames Lite (Python) | Reames Agent (Go) |
|---|---|
| `Message` | `provider.Message` |
| `Session` | session entry in `GET /sessions` |
| `ChatResult` | SSE event stream |
| `Usage` | `provider.Usage` + event stream |
| `ToolCall` | `tool.Call` |
