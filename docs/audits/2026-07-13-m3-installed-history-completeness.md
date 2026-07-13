# M3 installed history completeness follow-up

## Trigger

Desktop candidate `29244551372` ran commit `96c0fe2` after ordinary CI
`29244107302` (8/8) and CodeQL `29244107307` (3/3) had passed.

- Linux installed candidate: passed.
- macOS installed candidate: passed.
- Windows native cold/warm startup: passed.
- Windows interaction persisted the baseline turn, completed 19 Provider
  requests, all five failure/recovery scenarios, approval denial, tool timeout,
  and stop cleanup.
- The canonical `.events.jsonl` contained 11,402 bytes while the compatibility
  `.jsonl` checkpoint was 0 bytes; `boundary_changes=[]`.
- Restart still did not expose the baseline user and assistant messages through
  UI Automation. The strict accessibility step therefore did not run.

This contradicts any completion claim based only on local production evidence.

## Remaining uncovered race class

The previous fallback treated a controller projection as complete as soon as it
contained one visible user turn. A partial restored projection could therefore
win over a canonical event log with more turns. Independently, the frontend's
cache predicate treated any item, including a startup warning, as a reusable
transcript and could skip a later authoritative history read.

This batch closes both false-completeness gates:

1. `HistoryPageForTab` and `HistoryForTab` compare visible user-turn counts and
   use the pinned disk projection when it contains more complete history.
2. `hasReusableCachedTranscript` requires actual user or assistant content and
   still requires the expected session path when one is known.
3. Go regression coverage uses a 0-byte checkpoint, a two-turn canonical event
   log, and a one-turn controller projection.
4. Frontend regression coverage proves a startup notice is not reusable while a
   matching persisted user transcript is reusable.

## Local production evidence

The rebuilt Windows production Wails executable is 49,844,224 bytes with
SHA-256
`C4FE251037C0ADD87CA194DC79171AF54674D214C6850DC37FBA6DFD5824EB64`.

- Native cold/warm startup reached stable responsiveness in 2.015/1.516 seconds
  with `boundary_changes=[]` and no errors.
- The interaction/restart smoke completed 19 localhost Provider requests, all
  five failure/recovery scenarios, approval denial, tool timeout, Stop cleanup,
  and recovery of the initial user/assistant messages from the same session
  path. It reported `outcome=true`, `boundary_changes=[]`, and no errors.
- The strict accessibility smoke passed using UI Automation InvokePattern only.
  Skip-to-composer focus, Settings dialog semantics, background isolation,
  dialog focus, and opener focus restoration all passed with
  `boundary_changes=[]` and no errors.

The complete pre-push gates also pass: `go build ./...`, `go vet ./...`, all
`internal/...` tests, Desktop Go tests/vet, frontend `test:all` and production
build/bundle budgets, documentation/deployment contracts, and diff checks. This
closes the source-production reproduction, but does not replace installed
artifact evidence. M3 remains open until a new installed Windows candidate
proves both interaction recovery and strict accessibility.

## Fourth installed-candidate result

The batch landed as `16136c8` plus the documentation-index repair `1d115ba`.
CI `29248016550` passed 8/8 and CodeQL `29248016498` passed 3/3. Desktop
candidate `29248420722` then reproduced the same installed-only failure:

- Linux and macOS installed candidates passed.
- Windows native cold/warm startup passed for installer SHA-256
  `5FEAC84616D5B0172CABFA2775780DFFABB5941AB9F80C4307526C5D382FAEAA`.
- Windows interaction completed 19 Provider requests, every failure/recovery
  scenario, Stop, message persistence, and `boundary_changes=[]`.
- The canonical event log was 11,401 bytes, the compatibility checkpoint was
  0 bytes, and restart still did not expose the baseline user/assistant pair.
  Strict accessibility therefore did not run.

The old evidence schema assigned `recovered_session_path` only after the UI
message wait, so its empty value did not prove the tab path was absent. It left
the failure ambiguous between tab/session restore, disk history, backend
projection, frontend hydration, and UI rendering.

## Authoritative ready refresh follow-up

This follow-up closes the two remaining false-completeness paths and makes a
future failure actionable:

1. Startup still preloads pinned history while the controller is not ready, but
   performs one authoritative history refresh after readiness. Only an actual
   live turn may suppress that refresh; an arbitrary cached transcript may not.
2. Backend history uses the canonical event log when it has more visible
   messages even if its visible user-turn count ties the controller projection.
3. React regression coverage starts with a partial preload, covers both the
   ready-event and readiness-polling paths, and proves the authoritative
   user/assistant pair replaces or refreshes it exactly once. Go coverage
   proves both more-turn and tied-turn/more-message disk fallbacks.
4. The Windows interaction report now captures the restart tab scope, workspace
   match and session path before waiting for transcript text, verifies the
   baseline pair directly on disk, and independently reports composer,
   onboarding, marker, assistant and UIA element presence.

The rebuilt production Wails executable is 48,052,736 bytes with SHA-256
`127D824CB7602257662719FEC4C33ED57BEFD8E187B0E57046497E6BD91A9382`.
Cold/warm stable responsiveness was 1.515/1.500 seconds. The complete 19-request
interaction/restart passed with the same initial/recovered session path; every
new disk/UIA recovery field was true except the expected
`restart_onboarding_present=false`. Strict InvokePattern accessibility also
passed with `boundary_changes=[]` and no errors. Installed-candidate evidence is
still required before closing M3.

## Fifth installed-candidate result

Commit `977050c` passed CI `29252169252` (8/8) and CodeQL `29252169424`
(3/3). Desktop candidate `29252663874` then produced the first unambiguous
installed failure boundary:

- Linux and macOS installed candidates passed.
- Windows native cold/warm startup passed in 7.031/2.000 seconds for installer
  SHA-256 `8DD48FC2B4AD73ECE9ABC67D3669CD54A9471A0C3D545F5020626A304A9B3133`.
- Windows interaction completed 19 Provider requests, all five recovery
  scenarios, Stop, persistence and `boundary_changes=[]`.
- Restart recovered the same project tab, workspace and session path. The disk
  contained both the baseline user and assistant messages; UIA exposed the
  composer, 223 elements and the user marker, but not the baseline assistant.
  Onboarding was correctly absent. Strict accessibility therefore did not run.

This rules out tab identity, workspace identity, session selection and durable
storage. It also proves that the earlier empty `recovered_session_path` was only
an evidence-ordering problem.

## In-flight preload replacement follow-up

One reproducible uncovered race was in the frontend request classifier. The pre-ready
`historyOnly` preload did not set `preserveCachedHistory`, so it was tracked as
authoritative. If `agent:ready` arrived while that request was still pending,
the ready refresh joined the pre-ready request instead of replacing it. A
partial result could therefore become the only applied startup history.

This follow-up:

1. Marks the pre-ready preload as a cache-preserving, replaceable request. A
   ready-time authoritative request now starts a new session-load sequence.
2. Uses the existing sequence gate to discard a late partial preload, so it
   cannot overwrite the complete ready-time projection.
3. Changes the ready-event regression to fire while the first history request
   is unresolved, then proves the second request supplies the assistant and the
   late user-only result is ignored. The polling regression remains independent.
4. Strengthens the Go canonical-log regression by persisting user and assistant
   in separate snapshots, matching real autosave append cadence.
5. Treats `SetForegroundWindow` as best-effort in the UIA helper while retaining
   hard UIA `SetFocus` and `HasKeyboardFocus` verification; Windows foreground
   policy can reject activation even when the target element can be focused.

The rebuilt production Wails executable is 48,052,736 bytes with SHA-256
`889986ABB11E97FDEDBFFC48700600503E6984F866E3E774B2FE751993583F24`.
Cold/warm stable responsiveness was 1.515/1.516 seconds. The complete 19-request
interaction/restart passed with the same session path and every disk/UIA
recovery field true except the expected `restart_onboarding_present=false`;
strict InvokePattern accessibility also passed. Both smokes reported
`boundary_changes=[]` and no errors. A new installed candidate is still
required before M3 can close.

## Sixth installed-candidate result

The follow-up landed as commit `bb13da3`. CI `29256586177` passed 8/8 and
CodeQL `29256588974` passed 3/3. Desktop candidate `29257178248` did not change
the installed interaction boundary:

- Linux and macOS installed candidates passed.
- Windows native cold/warm startup passed in 13.047/2.016 seconds for installer
  SHA-256 `2531E828C0A3464DF3CE9BD220889BA37128EA30E7DD5228746BF290E2F58A22`
  and installed executable SHA-256
  `58C530024C87B2DA34C745EE2709997CF736FDFB0DF2A4ACD0CE27944DD41D0D`.
- Windows interaction completed 19 Provider requests, all five recovery
  scenarios, Stop and persistence with `boundary_changes=[]`.
- Restart recovered the same project tab, workspace and session path. Disk
  contained the baseline user and assistant; UIA exposed the composer and user
  marker but not the baseline assistant. Strict accessibility therefore did
  not run.

The preload replacement remains useful race hardening with direct regression
coverage. The unchanged installed boundary proves it was not sufficient and
does not support calling it the confirmed root cause.

## Viewport-aware UIA follow-up

Transcript messages use `content-visibility: auto`. After restart the
transcript restores at the bottom, so the first assistant response may remain
outside the rendered accessibility subtree until the viewport moves to it.
The user marker alone is insufficient evidence that the first message element
is rendered because the same text can also appear in topic UI.

The existing `QuestionJumpBar` exposes localized accessible buttons for
`Jump to question 1`, `跳转到问题 1`, and `跳轉到問題 1`. The interaction smoke
now:

1. Records user-marker and assistant visibility before navigation for
   diagnostics.
2. Invokes the first-question button through strict UIA InvokePattern.
3. Waits for the baseline assistant after the transcript has navigated to the
   first turn, while retaining the independent disk, project, workspace,
   session, composer and onboarding assertions.

The 48,052,736-byte local production Wails executable with SHA-256
`889986ABB11E97FDEDBFFC48700600503E6984F866E3E774B2FE751993583F24`
completed the strict revised smoke in 52.1 seconds. It made 19 Provider
requests, verified all recovery scenarios and Stop, retained the same session
path, and reported `boundary_changes=[]` with no errors. Before navigation both
the user marker and assistant were present in the UIA tree but both reported
`is_offscreen=true`. After strict InvokePattern activated the first-question
control, both reported `is_offscreen=false`; the final composite recovery gate,
which also requires the disk pair, composer and absent onboarding, passed.
This locally reproduces the offscreen condition, but only a new installed
runner can prove the previously failing candidate. M3 remains open until
Windows installed interaction and the subsequent strict accessibility step
both pass.
