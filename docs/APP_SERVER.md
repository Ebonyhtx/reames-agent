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
| client to server | `thread/fork` | persistent fork; optional inclusive `lastTurnId` |
| client to server | `thread/archive`, `thread/unarchive` | transactional local archive for idle threads |
| client to server | `thread/rollback` | deprecated compatibility method; conversation history only |
| client to server | `thread/list`, `thread/loaded/list`, `thread/read` | supported; bounded cursor pagination |
| client to server | `thread/name/set`, `thread/unsubscribe` | supported |
| client to server | `turn/start`, `turn/steer`, `turn/interrupt` | text input only; one active turn per thread |
| server to client | `thread/started`, thread/turn/item status and text/reasoning deltas | supported live projection |
| server to client | command/file approval, `item/tool/requestUserInput` | supported server requests |
| client to server | compact/review/settings/dynamic-tool methods | unsupported |
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
empty threads. Archived threads are excluded by default and selected with
`{"archived":true}`.

`thread/fork` copies the canonical persisted history into a new independently
leased thread without switching the source Controller. `lastTurnId`, when set,
is inclusive. The source must be idle; unsupported runtime overrides and
`ephemeral: true` fail before the fork is published. The response contains the
copied turns and `forkedFromId`, then `thread/started` is emitted.

`thread/archive` stops and unloads an idle runtime, then moves the transcript,
canonical event log, metadata, checkpoints, jobs, Guardian state, owned
sub-agent records, and any origin/active recovery pair as one rollback-capable
bundle under the session store's `.archive` directory. A live turn, pending
prompt, background job, writer lease, destination collision, or failed artifact
move rejects the operation. `thread/unarchive` preflights every live target and
rolls back partial restores. Responses are written before `thread/archived` or
`thread/unarchived` notifications.

`thread/rollback` accepts `numTurns >= 1` and is intentionally conversation-only:
it uses the Controller's durable rewind transaction and checkpoint transcript
anchor, but never restores workspace files. Clients remain responsible for file
reversion. This method follows the upstream deprecation and is retained only as
a bounded compatibility surface.

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
second replay log. A versioned 0600 sidecar keeps the App-Server thread id and
fork ancestry stable when conflict recovery moves the active transcript;
metadata, archive bundles and writer-lease updates roll back together on
failure.

This first slice is not full Codex App-Server parity. See the
[development plan](DEVELOPMENT_PLAN.md#p9codex-class-extensibility-与-headless-协议)
for the remaining P9 work.
