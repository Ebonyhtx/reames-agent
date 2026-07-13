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
