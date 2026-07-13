# Design: Checkpoints & Rewind

Status: **Phase 1 + 2 and the first M4 recovery hardening batch are implemented**.
The shared controller supports code/conversation/both rewind, fork-from-here and
summarize from/up to here for CLI and Desktop. Conversation rewrites now require
a transcript-prefix digest and a valid turn-start runtime projection; checkpoint
truncation is restart-durable even when physical garbage collection fails. An
optional git-backed mode remains a lower-priority follow-up.

This document describes rewind snapshots. For the autonomous-run rule about when
the agent should pause and ask the user, see
[`TASK_CONTRACT.md`](TASK_CONTRACT.md).

## Goal

Let a user rewind a session to a previous point and restore **code**,
**conversation**, or **both** — without touching their git history. Aligned with
Claude Code's rewind (Esc-Esc / `/rewind`), driven identically from the CLI and
the desktop.

## Mechanism: file snapshots, not git

Like Claude Code (and v1's `checkpoints.ts`), checkpoints are **file snapshots**,
independent of git:

- **Zero git pollution** — never commits, stages, or touches `.git/`. Works in a
  non-git directory.
- **Tracks only previewable edit-tool changes** — `write_file` / `edit_file` / `multi_edit`.
  File moves via `move_file` follow the same workspace permission boundary, but
  are not yet represented in checkpoint previews.
  `bash` side effects are **not** tracked (no way to know what a shell command
  touched), exactly as Claude Code. Risky bash is already permission-gated.
- Full pre-edit content snapshots (simple; storage bounded by retention, below).

An optional **git-backed mode** (v1's `auto-git-rollback`) is a possible Phase 2
for users who want git-level safety; it is explicitly out of scope here.

## Anchors & capture

- **One checkpoint per visible user turn.** A checkpoint opens through the
  shared turn orchestrator before the user message is appended. Interactive
  sends and headless `Controller.Run` use the same lifecycle; synthetic Goal or
  approved-plan continuations remain attached to that visible checkpoint.
- **Pre-edit snapshot.** In `agent.(*Agent).executeOne`, before running a tool
  whose `ReadOnly()` is false and which implements `tool.Previewer`, call
  `Preview(args)` → `diff.Change{Path, Kind, OldText}` and record a snapshot of
  that file into the active checkpoint. `tool.Previewer` already exists and the
  file-writers implement it, so this is one centralized seam — no per-tool code.
  - Dedup uses normalized workspace identities. Lexical aliases and Windows
    case aliases keep the first snapshot; existing hard-link aliases all restore
    from the same earliest bytes (hard-link identity itself is not recreated).
  - `Kind == create` (file did not exist) → store `Content = nil` so a restore
    *deletes* it. `modify`/`delete` → store `OldText`.
  - `bash` has no `Previewer`, so it is naturally excluded — matching the
    "edit-tools only" contract.

## Data model

```go
type FileSnap struct {
    Path     string
    Content  *string // nil -> file did not exist at the anchor
    Encoding *encoding.Kind
    Mode     *uint32
}

type Checkpoint struct {
    Turn             int
    Time             time.Time
    Prompt           string
    MsgIndex         int             // exact transcript boundary
    TranscriptDigest string          // digest of the prefix at MsgIndex
    Runtime          json.RawMessage // Goal/Plan/Todo projection at turn start
    Files            []FileSnap
}
```

## Storage

- **Sidecar to the session**, under `config.SessionDir()`: `<session-id>.ckpt/`
  with one `turn-<id>.json` per checkpoint and a small `.state.json` allocation /
  truncate manifest. It remains separate from the message JSONL.
- **Persists across sessions** — resuming a session re-loads its checkpoints, so
  code rewind works after a restart. Conversation rewind/fork additionally
  requires a modern checkpoint digest; legacy integer-only boundaries fail
  closed because same-length transcript divergence cannot be proven safe.
- **Restart-durable truncation** — rewind writes an atomic tombstone range before
  treating future checkpoints as retired. Physical `turn-*.json` deletion is
  garbage collection only; stale files are filtered after restart. Turn IDs use
  a durable monotonic watermark and are never reused after rewind. An existing
  but corrupt manifest fails closed: no old checkpoint is loaded, and the next
  turn retires the full prior ID range before healing the manifest.
- **Retention**: prune with the session (default ~30 days, configurable), to bound
  disk from full-content snapshots.

## Controller API (the one seam both frontends drive)

Checkpoints live on `control.Controller`, beside `SetPlanMode` / `Compact` /
`NewSession`, so the terminal TUI, the desktop webview, and the HTTP/SSE server
drive rewind identically and none re-implement it.

```go
type RewindScope int // Code | Conversation | Both

func (c *Controller) Checkpoints() []CheckpointMeta      // for the picker
func (c *Controller) Rewind(turn int, scope RewindScope) error
```

- **Code**: for every checkpoint from `turn` to the latest, take the earliest
  `FileSnap` per normalized file identity and restore each recorded alias to that
  content (delete if `nil`). All targets are preflighted before the first write:
  workspace escape, symlink/reparse traversal and non-regular targets are
  rejected. A failed operation rolls back earlier writes and the possibly
  partially written failing target from captured bytes/mode.
- **Conversation**: truncate `Session.Messages` to just before turn `turn`'s user
  message only when the live prefix matches `TranscriptDigest`. Non-empty invalid
  or future-version checkpoint runtime fails before transcript mutation; valid
  runtime replaces Goal/Plan/Todo, while a truly missing legacy runtime clears
  future Goal/Plan state. The rewritten transcript/runtime is persisted before a
  success event.
- **Both**: code + conversation.

A `Rewound` event (or reuse of a history-replace event) lets every frontend
re-render uniformly.

## CLI UX (aligned with Claude Code)

- **`Esc Esc`** with an empty composer, or **`/rewind`**, opens a picker listing
  each user turn (time + which files it changed). `chat_tui` already tracks the
  double-Esc timing.
- Select a turn → sub-menu: **`[code+conversation] [conversation] [code] [cancel]`**.
- On a conversation/both restore, the selected prompt is prefilled into the
  composer.

## Desktop UX (aligned with the VS Code extension)

- Each user message in the transcript gets a hover **rewind** control → menu:
  **rewind code / rewind conversation / both / fork-from-here**.
- It calls the same `controller.Rewind` over the Wails binding; the controller's
  event stream pushes the restored state and React re-renders. No rewind logic in
  the frontend.

## Non-goals & edge cases

- **bash / external side effects** (`rm`, `mv`, DB writes, deploys) are not
  tracked — rewind cannot undo them (Claude Code parity).
- **External edits between turns**: a snapshot holds the file's turn-start
  content, so restoring overwrites edits made outside reames-agent in the meantime.
- **Deletions**: an edit-tool deletion is restorable (snapshot has the content); a
  `bash rm` is not.
- **Large files**: full snapshots — retention cleanup bounds disk; revisit dedup
  (content-addressed snapshots) if it becomes a problem.
- **Known atomicity limits**: sidecars use `fileutil.AtomicWriteFile`, whose
  Windows cross-device/filter-driver fallback performs an in-place copy and can
  tear on process or power loss. Transcript/runtime/workspace are separate
  resources and do not form one crash transaction. Restore path checks also have
  a check-to-use window until handle-relative no-reparse/resolve-beneath writes
  are implemented. ACLs, xattrs and hard-link identity are not restored.

## Phasing

1. **Phase 1**: snapshot store + `executeOne` capture seam + `Controller.Rewind`
   (code/conversation/both) + CLI picker (Esc-Esc + `/rewind`).
2. **Phase 2**: desktop hover-rewind UI; "fork from here"; "summarize from/up to
   here"; optional git-backed mode.

## Open questions

- Default retention window and whether to expose it in `[checkpoints]` config.
- Content-addressed dedup vs one-file-per-snapshot.
- Handle-relative no-reparse restore and a durable multi-resource rewind journal.
