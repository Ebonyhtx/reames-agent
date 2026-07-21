# P9 App-Server persistent thread lifecycle audit

Date: 2026-07-21

## Scope

This slice adds persistent `thread/fork`, `thread/archive`,
`thread/unarchive`, `thread/rollback`, and archived-list filtering to the
existing stdio App-Server. It reuses `control.Controller`, canonical session
storage, checkpoint rewind, writer leases, and removal guards. It adds no second
Agent loop, rollout database, or remote listener.

The wire and behavior were compared with the Codex App-Server v2 schema and
README at audited upstream commit `eceb3eeaf3a68d732596fd8c0e8a6807f9166770`.
Unsupported override fields remain strict errors instead of silent fallback.

## Adopted behavior

- Fork copies full canonical history or an inclusive `lastTurnId` prefix into a
  new loaded persistent thread, leaves the source Controller unchanged, returns
  copied turns, records stable `forkedFromId`, and responds before
  `thread/started`.
- Archive unloads an idle thread and moves all owned session artifacts into a
  manifest-backed `.archive` bundle. `thread/list` excludes these by default and
  selects them with `archived: true`.
- Unarchive preflights every live destination and restores the whole bundle.
  Archive and restore reverse already completed moves when any later move fails.
- A conflict-recovery thread's origin and active transcripts share one archive
  transaction and cannot be split into a dangling redirect.
- Rollback validates `numTurns`, resolves loaded or stored threads, and uses
  `RewindConversation`; it never restores workspace files.

## Safety boundaries

| Risk | Control |
|---|---|
| mutate an active conversation | source session state plus Controller rotation gate reject active/closing threads |
| background writer races archive | runtime status rejects pending prompts/jobs; writer lease is released only after shutdown snapshot, then removal guard owns save/lease locks |
| partial archive or restore | durable manifest is written first; completed artifact moves are tracked and reversed in reverse order on failure |
| recovery redirect loses one half | origin and active transcripts are acquired in canonical lock order and moved in one bundle |
| fork published without metadata/runtime | new session cleanup runs unless sidecar, lease, runtime load, map registration, and response object all succeed |
| rollback unexpectedly changes files | transport calls conversation-only rewind; tests use a controller double that rejects every other scope |
| archived thread silently appears live | live and archived scans are separate; archive filtering is explicit |

The archive transaction protects process concurrency and ordinary operation
failures. It does not claim power-loss atomicity across multiple filesystem
renames. Corrupt or manually edited manifests fail closed.

## Evidence in this worktree

- `go test ./internal/control ./internal/appserver -count=1`
- archive/unarchive round trip with canonical sidecars and checkpoint directory;
- injected archive and restore move failures with reverse rollback;
- recovery origin/active bundle round trip;
- wire-level fork/rollback/archive/list/unarchive round trip and stable ancestry;
- active-turn rejection before persistence mutation.

Full repository, race, Desktop/Frontend, cross-target, clean-clone, CI and
CodeQL gates remain pending for the larger P9 delivery batch. This audit does
not mark P9 complete.

## Explicit non-parity

Paginated history, compact/review/settings, dynamic-tool registration, MCP
status/login, image/audio/realtime, WebSocket/Responses Lite, ephemeral threads,
and multi-agent App-Server projection remain unsupported. Upstream marks
`thread/rollback` deprecated; Reames keeps only the bounded compatibility method
described above.
