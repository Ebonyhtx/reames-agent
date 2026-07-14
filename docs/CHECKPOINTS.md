# Design: Checkpoints & Rewind

Updated: 2026-07-14.

Status: **Phase 1 + 2 and the M4 crash-recovery hardening are implemented**.
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
- **Tracks only previewable edit-tool changes** — `write_file`, `edit_file`,
  `multi_edit`, `delete_range`, `delete_symbol`, `notebook_edit`, `move_file`, and
  `apply_patch`. Multi-file previews are collected and snapshotted as one gate;
  `move_file` records source deletion plus destination creation.
  `bash` side effects are **not** tracked (no way to know what a shell command
  touched), exactly as Claude Code. Risky bash is already permission-gated.
- Full pre-edit content snapshots (simple; storage bounded by retention, below).
- Persisted checkpoint records use `0600` and their directory uses `0700`, because
  snapshots may contain source credentials or other workspace-private bytes.

An optional **git-backed mode** (v1's `auto-git-rollback`) is a lower-priority
future mode for users who want git-level safety; it is explicitly out of scope
for the implemented snapshot phases.

## Anchors & capture

- **One checkpoint per orchestrated turn.** A checkpoint opens through the
  shared turn orchestrator before its user-role message is appended. Visible
  user turns appear in rewind pickers; synthetic Goal/Plan continuations use
  hidden checkpoints so crash recovery only undoes that continuation's effects
  and never crosses the preceding committed visible turn.
- **Pre-edit snapshot.** In `agent.(*Agent).executeOne`, before running a tool
  whose `ReadOnly()` is false and which implements `tool.Previewer` or
  `tool.MultiPreviewer`, collect every `diff.Change{Path, Kind, OldText}` and
  record all snapshots before the tool runs. Any preview or snapshot persistence
  failure blocks the whole writer.
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
    Path     string        `json:"path"`
    Content  *string       `json:"content"` // nil -> file did not exist at the anchor
    Encoding *fileenc.Kind `json:"encoding,omitempty"`
    Mode     *uint32       `json:"mode,omitempty"`
}

type Checkpoint struct {
    Turn             int             `json:"turn"`
    Time             time.Time       `json:"time"`
    Prompt           string          `json:"prompt"`
    MsgIndex         int             `json:"msgIndex"` // exact transcript boundary
    Synthetic        bool            `json:"synthetic,omitempty"`
    TranscriptDigest string          `json:"transcriptDigest,omitempty"`
    Runtime          json.RawMessage `json:"runtime,omitempty"` // Goal/Plan/Todo projection at turn start
    Files            []FileSnap      `json:"files"`
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
- **Durable turn commit** — branch metadata binds an in-flight turn to its exact
  checkpoint. A successful turn persists transcript and runtime, writes their
  commit anchor, then clears the marker. Resume preserves workspace changes only
  when both resources match; otherwise it restores checkpoint workspace/runtime
  and removes the partial transcript tail.
- **Durable rewind journal** — conversation/both rewind records `prepared`,
  publishes transcript/runtime/workspace, advances to `resources_applied`, then
  retires checkpoints and clears the journal. Resume and the next turn replay an
  unfinished phase; Compact/New/Fork/Branch/Switch/Summarize/Rewind and other
  content-preserving rotations use the same preflight. Checkpoints cannot be
  retired while prepared recovery still needs their file snapshots.
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
  rejected. Preflight, delete, write and rollback reuse one workspace `os.Root`,
  so a component replaced after validation cannot redirect writes outside the
  root. A failed operation rolls back earlier writes and the possibly partially
  written failing target from captured bytes/mode.
- **Conversation**: write durable rewind intent, truncate `Session.Messages` to
  just before turn `turn`'s user
  message only when the live prefix matches `TranscriptDigest`. Non-empty invalid
  or future-version checkpoint runtime fails before transcript mutation; valid
  runtime replaces Goal/Plan/Todo, while a truly missing legacy runtime clears
  future Goal/Plan state. The rewritten transcript/runtime is persisted before a
  success event.
- **Both**: code + conversation under the same durable rewind journal. This is a
  recoverable cross-resource protocol, not a claim that all files change in one
  filesystem transaction.

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
- **Known recovery limits**: `AtomicWriteFile` uses a sibling temp file, fsync,
  atomic replace and parent-directory/write-through persistence; cross-device or
  filter-driver rename failures now fail closed and never fall back to in-place
  copy. The turn and rewind journals provide logical recovery across separate
  transcript/runtime/checkpoint/workspace resources. Previewable built-in
  writers and restore use handle-relative resolve-beneath I/O, but `bash`, MCP,
  external APIs and background opaque side effects remain untracked and are not
  exactly-once. ACLs, xattrs and hard-link identity are not restored.

## Phasing

1. **Phase 1**: snapshot store + `executeOne` capture seam + `Controller.Rewind`
   (code/conversation/both) + CLI picker (Esc-Esc + `/rewind`).
2. **Phase 2**: desktop hover-rewind UI; "fork from here"; "summarize from/up to
   here"; optional git-backed mode.

## Open questions

- Default retention window and whether to expose it in `[checkpoints]` config.
- Content-addressed dedup vs one-file-per-snapshot.
