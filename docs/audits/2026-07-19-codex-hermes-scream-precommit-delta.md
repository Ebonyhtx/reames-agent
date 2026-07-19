# Codex / Hermes / Scream pre-commit incremental review

Date: 2026-07-19

Reviewed ranges:

- OpenAI Codex: `312caf176a8fd3a5897a3d1fd3ed0a283bd1b5ac..0fb559f0f6e231a88ac02ea002d3ecd248e2b515`
- Hermes: `34e66a0d527a762b128cebf3bd9165cd8d968c06..614dc194ea7d853d39f9e84582ec62156f41a475`
- Scream Code: `5b1a9922e06d87e53a52be964555119a113f576e..4938d5175349768774876efdc0beab8a149ced25`

Reasonix and Claude Code remained unchanged at their reviewed SHAs. Codex is a
tier-two strategic code upstream, so both commits were reviewed as product and
wire behavior, not as release-note claims. Hermes and Scream remain mechanism
references.

## OpenAI Codex

### P9 required capability: dynamic-tool and code-mode audio output

Codex `643de86a19` adds `inputAudio` across dynamic-tool responses, App-Server
events, generated schemas, thread history, MCP conversion, analytics, and the
code-mode `audio()` helper. The implementation is deliberately narrower than a
generic URL player: audio must be an inline `data:` URL, model modality support
is checked before conversion, and unsupported audio becomes an explanatory
text item instead of malformed model input. Tests cover protocol round trips,
invalid URLs, model capability filtering, and MCP audio blocks.

Reames does not yet have Codex freeform/code-mode, dynamic tools, App-Server, or
an audio-capable Provider content contract. ACP explicitly advertises
`audio=false`, and provider model discovery filters specialized audio models.
Adding only an `audio_url` field in this M6 batch would therefore be a false
parity claim. P9 must implement the complete vertical slice: bounded inline
data parsing, explicit model capability, MCP/tool result conversion, safe
history/export projection, wire fixtures, and unsupported-modality fallback.
Remote audio fetches remain forbidden unless a later permission/network design
proves an equivalent boundary.

### P9 App-Server contract: legacy views over paginated thread history

Codex `0fb559f0f6` removes the earlier split where paginated threads could not
serve full-history resume or full item views. It materializes full turns/items
when requested, supports `initialTurnsPage` for a paginated running thread,
merges the live active turn without violating page size/cursors, and preserves
metadata-only `excludeTurns` plus backwards cursors.

Reames Desktop already pages the canonical event log by turn and chooses the
more complete live/disk projection, but it does not expose the Codex App-Server
thread protocol or a paginated thread-store projection. The existing Desktop
behavior is not App-Server parity. P9 must keep metadata, summary, and full
views semantically consistent across legacy/current stores, merge a live turn
without duplicate or lost items, and preserve requested limits and both cursor
directions.

## Hermes

Hermes `e30174fa1` scopes each live tool-diff subscription to one tool-call ID
and replaces keyless React Query invalidation with a correctness-safe denylist
of profile-independent query roots. Reames does not use Nanostores or React
Query, and tool diffs are carried on controller transcript items rather than a
shared `$toolDiffs` map, so the exact patches are non-isomorphic. The mechanism
is retained as a Desktop performance audit signal: streaming one tool must not
re-render unrelated tool rows, and workspace/profile switches must invalidate
only caches whose ownership actually changed. Adoption requires a React
profiler/selector benchmark against the current Zustand/transcript graph.

## Scream Code

Scream `ce3b72b` adds a compact narrow-terminal loading logo. Reames has no
equivalent full-screen Node loading animation, so no code is copied. Scream
`4938d517` upgrades its private TUI dependency for ANSI SGR coalescing and a
Windows ConPTY settle window, then adds confirmation for input longer than
5,000 characters.

Reames uses Bubble Tea rather than that TUI runtime, so the SGR/ConPTY patch is
a benchmark signal only. Reames already preserves large paste/fold/file-ref
semantics; a fixed character gate could interrupt valid source/context input
and is not adopted without token-aware cost, paste/file distinction, keyboard
and accessibility behavior, and tests proving commands entered after a cancel
are not swallowed. It remains a P9/P10 UX candidate, not an M6 requirement.

## Freeze action

The three clean local mirrors were fast-forwarded after review and their exact
SHAs explicitly accepted in `docs/upstreams/upstreams.lock.json`. Acceptance
means reviewed/classified, not copied. Generated `artifacts/` reports are
temporary delivery evidence and are removed after the pushed batch is green.
