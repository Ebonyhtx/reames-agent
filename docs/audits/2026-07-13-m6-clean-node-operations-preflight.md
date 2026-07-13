# M6 clean-node operations preflight

## Scope

`scripts/smoke_gateway_headless.py` report schema v2 extends the existing real
binary Gateway smoke without introducing credentials or another CI job. Against
one isolated `REAMES_AGENT_HOME`, it now verifies:

- redacted, atomic, idempotent `gateway setup`;
- deep `gateway doctor` and a secret-free service-manager plan;
- explicit `gateway run --home` precedence over ambient state;
- an OpenAI-compatible localhost Provider turn through `reames-agent run`;
- metrics and canonical session persistence for that turn;
- two duplicate Gateway feedback submissions, one aggregate group, and a local
  maintenance draft;
- absence of email/key-shaped fixture values from command output, ledger, draft,
  config, every other persisted home file, and report evidence.

`scripts/test_smoke_gateway_headless.py` covers the fixture protocol, provider
config secret boundary, marker extraction, and leak guard. The existing
`deployment-contracts` CI job runs those tests and the full smoke, then uploads
the unchanged `artifacts/headless-gateway-smoke.json` path.

## Local evidence

The schema-v2 Windows preflight passed with exactly one localhost Provider
request, two session files containing the task/response, persisted metrics, two
feedback records grouped into one duplicate cluster, and one persisted
maintenance draft. The final whole-home and report scan found no fixture email,
API-key-shaped feedback value, or synthetic Provider key. Contract checks and
seven Python unit tests also passed.

## Explicit non-claims

The report records these as `external_blocked` rather than completed:

- real Provider API round trip;
- real IM text/approval/cancel/recovery round trip;
- installed service-manager start/status/restart cycle.

Ubuntu CI after the eventual batch push is the clean Linux credential-free
preflight. It still does not substitute for a maintained cloud VM, a real API
key, a real IM application, or service-manager lifecycle evidence.
