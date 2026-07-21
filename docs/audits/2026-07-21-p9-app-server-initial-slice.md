# P9 App-Server initial vertical slice audit

Date: 2026-07-21

## Scope and source

The implementation was reviewed against OpenAI Codex commit
`678157acaa819d5510adfe359abb5d0392cfe461`, including the App-Server README,
protocol v1/v2 schema, initialize processor, message processor and lifecycle
fixtures. Reames adopts the transport and lifecycle contracts that fit its
existing Controller; it does not copy the Rust runtime, SQLite rollout store or
unsupported product surfaces.

Production entrypoint: `reames-agent app-server`. Composition remains
`internal/cli -> boot.Build -> control.Controller -> agent/provider/tools`.
`internal/appserver` owns only wire validation, connection concurrency, thread
leases/subscriptions, event projection and stable App-Server metadata.

## Implemented contract

- stdio JSONL only; no listener, WebSocket or remote attach;
- 8 MiB frame limit, 64 in-flight requests, serialized writer and cancellable
  blocking-reader shutdown;
- App-Server wire without `jsonrpc`, strict top-level frame/params/enum checks;
- initialize/initialized plus thread start, resume, list, loaded-list, read,
  name, unsubscribe and turn start, steer, interrupt;
- response-before-stream ordering for thread start/name update/turn start and
  interrupt-before-completion;
- server-initiated approval and Ask, conservative disconnect/cancel fallback,
  no persistent grant for fresh-human tools;
- completion, steering and interruption bound to primary thread plus turn;
  child completion cannot settle the primary turn;
- canonical `control.TranscriptMessage` replay only; progress/raw response/media
  events are live-only;
- versioned 0600 `<session>.jsonl.appserver.json` metadata preserving a stable
  thread id across conflict-recovery transcript moves, with lease/sidecar
  rollback and session deletion ownership; unused empty-thread metadata is
  removed when no parent transcript was ever persisted;
- Ctrl+C and SIGTERM cancel the blocking stdio reader, settle active work and
  run the same bounded shutdown snapshot path.

## Threat model

| Threat | Control |
|---|---|
| stdout log or secret corrupts protocol | CLI reserves stdout for JSONL; diagnostics use stderr; initialize fixture loads no Provider key |
| oversized or malformed local client input | 8 MiB line cap; one strict JSON object; unknown/mixed fields rejected |
| request flood or writer interleaving | 64-request semaphore and one encoded-write mutex |
| two processes write one transcript | session writer lease; resume fails closed when held |
| recovery changes public thread identity | stable metadata redirects origin to one validated sibling active transcript |
| partial metadata publication | origin/recovery sidecars and lease restore from snapshots on failure |
| child/old turn completes or interrupts current turn | simultaneous primary thread and turn identity match |
| reconnect replays secrets/transient output | replay derives only display-safe canonical transcript |
| headless approval becomes silent permanent authority | server request required; fresh-human persistent choice omitted and ignored |
| cancellation hangs on stdin | context cancellation closes closable readers and releases pending approval/Ask requests |
| response understates execution authority | unconfined shell reports `dangerFullAccess`; enforced mode projects configured roots and network access |

The protocol is local-process trust, not an authentication boundary. A process
that can start App-Server and access the user's Reames home has the same local
authority as other Reames frontends, still constrained by permission, sandbox,
checkpoint, Hook and evidence policy.

## Explicit non-parity

At this initial-slice checkpoint, thread fork/archive/unarchive/rollback were
unsupported; they are implemented by the follow-up lifecycle slice documented
in `2026-07-21-p9-app-server-thread-lifecycle.md`. Paginated history,
compact/review, settings mutation, dynamic-tool registration, MCP status/login
methods, image or audio input, realtime conversation, WebSocket/Responses Lite
and multi-agent App-Server projection remain unsupported. `ephemeral: true`, unknown
`historyMode` and unsupported official override fields fail before Controller
mutation. These remain active P9 work and are not represented as complete.

## Evidence

Focused evidence completed for this slice:

```text
go test ./internal/appserver ./internal/store ./internal/cli -count=1
go test -race ./internal/appserver ./internal/control ./internal/agent -count=1
go vet ./internal/appserver ./internal/cli ./internal/store
```

Fixtures cover lifecycle/event order, approval and Ask settlement, child-first
completion isolation, canonical replay, interrupt identity, strict history and
override rejection, explicit-name versus preview, stable resume metadata,
recovery lease movement/rollback, malformed transport frames, blocked-reader
cancellation and CLI stdout isolation. Full repository, frontend, cross-target
and clean-clone evidence is required before this batch is pushed and will be
recorded with the final batch verification.

Full pre-commit repository evidence also passed:

```text
go build ./...
go vet ./...
go test ./internal/... -count=1 -timeout 300s
go test -race ./internal/appserver ./internal/control ./internal/agent -count=1
(desktop) go test . -count=1 -timeout 300s
(desktop/frontend) corepack pnpm test:all
(desktop/frontend) corepack pnpm build
python -m unittest discover scripts -p "test_*.py" -v  # 155 passed, 2 skipped
python scripts/check_public_readiness.py
python scripts/check_release_contracts.py
node scripts/test_upstream_watch_issue.mjs
go test ./internal/tool -run TestBuiltinToolContractDocumentation -v
```

Six `CGO_ENABLED=0` targets (`windows`, `linux`, `darwin` x `amd64`, `arm64`)
built both `cmd/reames-agent` and `cmd/reames-agent-guard`: 12/12 artifacts.
The live `check_upstreams.py` remote probe exceeded its ten-minute outer bound
and was terminated; its deterministic unit suite and issue-reconciliation
contract passed. This network timeout is not represented as a successful remote
watch or as new upstream-review evidence.
