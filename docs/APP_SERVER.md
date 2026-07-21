# App-Server integration

`reames-agent app-server` exposes Reames Agent to a local editor or automation
client over newline-delimited JSON on stdin/stdout. The wire follows the
Codex-class App-Server shape but intentionally implements a smaller, documented
surface. It does not open a network listener.

## Start

```bash
reames-agent app-server
reames-agent app-server --model openai/gpt-5.6-sol --profile delivery
```

`--listen stdio://` is the only supported endpoint; `--stdio` is an alias.
Diagnostics go to stderr and stdout is reserved for protocol frames. Each frame
must be one JSON object of at most 8 MiB. The wire omits the `jsonrpc` member.

```json
{"id":1,"method":"initialize","params":{"clientInfo":{"name":"my-editor","version":"1.0"}}}
{"method":"initialized","params":{}}
{"id":2,"method":"thread/start","params":{"cwd":"/absolute/project","historyMode":"legacy"}}
```

The connection becomes usable when `initialize` succeeds; the conventional
`initialized` notification is accepted as an acknowledgement. Repeating
`initialize` fails. Initialization itself does not load Provider credentials;
creating or resuming a thread assembles the normal Reames runtime and therefore
uses the same configuration and credential requirements as other frontends.

## Method matrix

| Direction | Method | Status |
|---|---|---|
| client to server | `initialize`, `initialized` | supported |
| client to server | `thread/start`, `thread/resume` | persistent legacy history only |
| client to server | `thread/list`, `thread/loaded/list`, `thread/read` | supported; bounded cursor pagination |
| client to server | `thread/name/set`, `thread/unsubscribe` | supported |
| client to server | `turn/start`, `turn/steer`, `turn/interrupt` | text input only; one active turn per thread |
| server to client | `thread/started`, thread/turn/item status and text/reasoning deltas | supported live projection |
| server to client | command/file approval, `item/tool/requestUserInput` | supported server requests |
| client to server | fork/archive/unarchive/rollback/compact/review/settings/dynamic-tool methods | unsupported |
| transport | WebSocket / TCP / HTTP | unsupported |
| content | image, local image, skill, mention, audio and realtime input/output | unsupported |
| history | `historyMode: "paginated"` | rejected before runtime mutation |

Unknown fields and enum values are rejected rather than ignored. In particular,
unsupported sandbox, approval, model-runtime, MCP, or environment overrides do
not silently fall back to defaults.

The open-thread response reports the effective shell posture conservatively:
an unconfined shell is `dangerFullAccess`; an enforced shell reports
`workspaceWrite`, its configured write roots and sandbox network access.

`thread/list` follows Reames session-history semantics and omits a newly opened
thread until it has at least one user turn. `thread/loaded/list` includes loaded
empty threads.

## Runtime and safety

Every thread is driven by the same `control.Controller` used by CLI, Serve and
Desktop. App-Server adds no second Agent loop. A session writer lease prevents a
thread from being resumed by two local runtimes at once. `turn/steer`,
`turn/interrupt`, completion settlement and subscription decisions bind both the
primary thread id and turn id, so child-agent activity cannot complete a parent
turn.

Tool approval and Ask are server-initiated requests. A disconnect, cancellation,
invalid response or missing response resolves conservatively. Tools requiring a
fresh human decision cannot gain a persistent session grant; the existing
sandbox-escape session decision remains explicit. The Controller's normal
permission, sandbox, Hook, checkpoint and evidence policies still apply.

Replay is rebuilt only from the canonical Reames transcript. Raw Provider items,
tool progress, process output deltas and realtime media are not written to a
second replay log. A versioned 0600 sidecar keeps the App-Server thread id stable
when conflict recovery moves the active transcript; metadata and writer-lease
updates roll back together on failure.

This first slice is not full Codex App-Server parity. See the
[development plan](DEVELOPMENT_PLAN.md#p9codex-class-extensibility-与-headless-协议)
for the remaining P9 work.
