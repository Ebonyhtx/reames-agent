# Upstream Watch

Reames Agent is not a blind fork, but it must keep tracking important upstream
reference projects. This directory records the advisory-only upstream watch
configuration.

## Files

| File | Purpose |
|---|---|
| `upstreams.json` | Reference repositories, branches, tag patterns, local reference paths, and tracking policy. |
| `upstreams.lock.json` | Current pinned refs used for comparison. Updating this file means “we have reviewed this upstream point”. |

## Command

```powershell
python scripts/check_upstreams.py --out-dir artifacts/upstream-watch
```

Outputs:

- `artifacts/upstream-watch/upstream-report.md`
- `artifacts/upstream-watch/upstream-report.json`

The report is advisory. It must not auto-merge upstream code into `main`.

## Policy

1. Reasonix is the primary base. Its runtime/provider/cache/security changes are high-risk and require human review.
2. Other reference projects are signal sources. We may copy ideas, UX patterns, tests, or small mechanisms, but not wholesale code.
3. A lock update means the upstream point has been reviewed or intentionally deferred.
4. Product UI changes must still satisfy the Reames Agent authority and execution plan.
