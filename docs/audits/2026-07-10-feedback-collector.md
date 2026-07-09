# Self-hosted feedback collector audit

## Scope

This batch adds the first concrete C4 feedback-center primitive:

- `internal/feedback`
- `POST /api/feedback`
- `GET /api/feedback/summary`

The goal is a local, self-hosted feedback ledger for cloud/server nodes. It does
not enable third-party telemetry, automatic desktop uploads, or automatic issue
creation.

## Contract

`internal/feedback` accepts structured reports for:

- `crash`
- `exception`
- `feedback`
- `performance`
- `bot`
- `metrics`

Before a record is written, it:

- clips large fields;
- redacts email addresses, user home path segments, key/value secrets, Bearer
  tokens, explicit provider-key shaped tokens, JWTs, environment-secret names,
  long hex strings, and long base64/base64url tokens;
- computes a stable fingerprint for duplicate grouping;
- appends JSONL to `<Reames Agent home>/feedback/feedback.jsonl`.

The `serve` API exposes:

```text
POST /api/feedback
GET  /api/feedback/summary
```

The POST endpoint is protected by the existing `serve` JSON content-type guard
and the existing `serve` authentication middleware when auth is enabled. It does
not bypass token/password auth.

## Evidence

Local tests:

```bash
go test ./internal/feedback ./internal/serve -count=1 -timeout 300s
```

Covered behavior:

- secret-like values are redacted before storage;
- duplicate feedback records share a fingerprint but keep distinct IDs;
- missing feedback ledgers summarize as empty;
- HTTP feedback collection writes the local JSONL ledger;
- HTTP summary output does not leak the submitted secret-like values;
- `text/plain` feedback POSTs are rejected by the serve CSRF/content-type guard.

## Remaining gap

This is the local collector and aggregation layer only. The C4 completion bar
still requires:

- a Desktop/Gateway user-facing submission flow with preview;
- a maintenance-task or GitHub Issue draft generator;
- real cloud-node evidence from a live feedback or crash path;
- operator controls for retention, export, deletion, and rate limiting.
