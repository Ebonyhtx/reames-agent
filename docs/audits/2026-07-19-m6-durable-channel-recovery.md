# M6 durable channel recovery audit

Date: 2026-07-19

Scope: repository-side Gateway delivery recovery after the Hermes
`862b1b37..7a43ab04` Discord recovery signal. This audit does not claim a real
Feishu, QQ, Weixin, or Telegram history replay.

## Outcome

The transport-independent Gateway now has a durable inbound delivery state
machine shared by foreground CLI and Desktop hosts:

- production hosts use `<Reames Agent home>/bot/delivery-ledger.json`; the file
  is written atomically with mode `0600`;
- an inbound message is durably claimed before allowlist callbacks, commands,
  queueing, Controller creation, or Agent execution; corrupt, oversized,
  identity-mismatched, full, or unwritable ledger state fails closed;
- the record retains the original remote session identity, message ID, opaque
  recovery cursor, state, attempt count, and timestamps. It never stores message
  text, media URLs, raw platform payload, model output, tool data, credentials,
  or raw error strings;
- live duplicate delivery and duplicate history replay are suppressed across
  process restarts. A claim left `processing` by a stopped process becomes
  `interrupted` on cold open and can be reclaimed;
- adapter-supplied connection/domain metadata cannot select another connection:
  the host `AdapterBinding` is authoritative before access checks, routing,
  dedupe, and checkpointing;
- recovery cursors are per remote delivery channel, not per group member Agent
  session. Each channel receives a durable monotonic sequence, and the cursor
  advances only across a contiguous prefix of successfully delivered records.
  A later group-user response therefore cannot skip an earlier failed response;
- default limits are 4,096 message records, 4,096 channel sequences, a 4 MiB
  ledger file, and 200 recovered messages across all adapters per startup;
- optional `RecoveryAdapter.RecoverMissed` receives privacy-sensitive
  checkpoints in process memory and must return strictly-after-checkpoint,
  oldest-first messages. Missing identities/cursors and adapter over-return are
  rejected; scan errors are reported as degraded startup evidence without
  committing a cursor;
- normal Agent turns commit only after `RunTurn` succeeds and at least one final
  assistant text chunk is sent successfully. Final render sends now pass through
  the Gateway send tracker, so adapter health, outbound echo suppression, and
  delivery failure share one result;
- synchronous slash commands, access denial, pairing guidance, queue rejection,
  and successfully steered guidance commit after their final acknowledgement.
  Follow-up and interrupt-queued work remains claimed until its later turn is
  actually delivered. Collect/debounce merges, queue-cap summarize/drop, and
  later turns carry every constituent durable claim and media reference into
  the final-delivery transaction, rather than orphaning all but the first
  message ID. A successful `/stop`, `/new`, `/reset`, `/use`, `/attach`, or
  interrupt acknowledgement closes the explicitly canceled active/pending
  claims before the command cursor, so an intentional cancellation is not
  replayed as unfinished work;
- `/status`, authenticated control `/status`, and Prometheus metrics expose only
  aggregate record/checkpoint/retry counts. They do not expose ledger paths,
  remote IDs, message IDs, or cursor values.

## Failure and ordering evidence

The new tests cover:

- claim-before-delivery, same-process duplicate suppression, final commit, and
  cross-restart duplicate suppression;
- cold-start conversion of an abandoned processing claim and retry attempt
  accounting;
- failed delivery leaving the channel cursor unchanged and raw bearer material
  absent from the ledger;
- corrupt identity/state rejection;
- two group-user sessions completing out of order while the shared channel
  checkpoint waits for the contiguous successful prefix;
- a real `BotGateway.runTurn` with successful and failed final adapter sends;
- cancellation committing the canceled inbound identity before the `/stop`
  command cursor, including already queued claims;
- collect/debounce merged claims and media, queue-cap dropped/summarized claim
  carry-forward, and interrupt reporting of superseded pending claims;
- optional adapter startup replay, checkpoint handoff, global scan truncation,
  and degraded-but-live behavior when the history API is unavailable;
- race tests for `internal/bot` and `internal/botruntime`.

## Honest boundary

None of the current built-in Feishu, QQ, or Weixin adapters implements
`RecoveryAdapter` yet. They immediately benefit from durable live-event
claim/dedupe and final-delivery accounting, but a process that was completely
offline can only recover missed events after that platform adapter gains a real
history/resume API implementation. Real platform credentials, disconnects,
history pagination, reconnect, and final remote delivery remain external/adapter
evidence.

This mechanism is at-least-once, not exactly-once. If one response chunk reaches
the IM service and a later chunk fails, the cursor deliberately remains behind;
a retry may duplicate the earlier chunk. Likewise, external tool/API side
effects cannot be rolled back by the delivery ledger. Exactly-once claims are
forbidden until a platform supplies an idempotent outbound key or transaction
with equivalent evidence.

## Verification commands

```text
go build ./...
go vet ./...
go test ./internal/... -count=1 -timeout 300s
go test -race ./internal/bot ./internal/botruntime ./internal/control ./internal/plugin ./internal/config -count=1 -timeout 600s
(cd desktop && go build ./... && go vet ./... && go test ./... -count=1 -timeout 300s)
(cd desktop/frontend && corepack pnpm test:all && corepack pnpm build && corepack pnpm smoke:plugin-browser)
```

Root/Desktop/frontend full validation, the real Chrome plugin smoke, five-package
race coverage, documentation/public/deploy/release contracts, and six-target CLI
plus six-target Guard builds passed locally. An independent `--no-hardlinks`
clean clone also passed Root/Desktop/frontend validation from an initially empty
`node_modules`, the real Chrome plugin lifecycle smoke, Gateway clean-node smoke,
all governance and installer contracts, and an 11-repository upstream scan with
`changed_count=0`; Go and pnpm caches were isolated on the F drive. The final
batch now requires only the pushed commit's remote CI/CodeQL before this
repository tranche is closed.
