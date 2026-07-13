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
