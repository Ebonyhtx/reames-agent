# Plugin Registry Operations

This runbook defines the operator contract for a Reames Agent plugin registry.
The client mechanism exists, but the project does not operate or preconfigure a
public registry yet. A deployment is trustworthy only after its owners complete
their own key ceremony, repository publishing, monitoring, and incident drills.

## Trust and Repository Shape

Reames Agent follows [The Update Framework specification](https://theupdateframework.github.io/specification/draft/)
through the official [go-tuf/v2 client](https://github.com/theupdateframework/go-tuf).
Each client must receive an initial `root.json` out of band. Do not download the
bootstrap root from the same untrusted URL and call that verification.

The initial supported repository has:

```text
metadata/
  <N>.root.json
  timestamp.json
  <N>.snapshot.json
  <N>.targets.json
targets/
  <sha256>.plugins.json
  attestations/<sha256>.<name>.json       # optional
```

The root must enable consistent snapshots. `plugins.json` is the default signed
index target; a user may configure another clean relative target path. Metadata
and targets may use different HTTPS origins, but each request refuses a redirect
to a different origin. Plain HTTP is accepted only on loopback for tests.

## Read-only Operator Audit

Before publication, audit an assembled repository with a bootstrap root obtained
through a genuinely independent channel:

```text
reames-agent plugin registry audit ./repository \
  --root /offline/reames-registry-root.json \
  --at 2026-07-16T10:00:00Z
```

`--root` is mandatory: the resolved path must be outside the repository, and it
cannot alias any file under `repository/metadata` through a hard link. `--index <target>` selects a non-default index; `--at` pins the UTC
reference time for a reproducible ceremony record. The command is read-only and
does not load, create, or persist private keys.

The JSON report proves the bootstrap self-signature; every contiguous root
rotation under both old and new thresholds; independent canonical keys for all
top-level roles; a minimum 2-of-3 root and targets threshold; bounded expiry
windows; the timestamp/snapshot/targets chain; and the hash-prefixed index plus
every referenced attestation target. The report includes sorted public key IDs
and SHA-256 digests for the final top-level metadata so ceremony records can
bind the exact audited bytes. Any gap, key reuse, weak threshold,
duplicate JSON key, path escape, stale/overlong expiry, or byte mismatch fails
closed. Run it again from a retained bootstrap root after an actual role
rotation or compromise-recovery publication.

Successful output always contains `externalRequired`. A local report is not
evidence of separate human key holders, HSM custody, atomic HTTPS publication,
live freshness alerts, a witnessed compromise drill, or DSSE/SLSA identity and
predicate policy. Preserve the report with the external ceremony record; do
not remove those boundaries from downstream summaries.

## Minimum Key Policy

Use independent keys for the four top-level roles. The following is a deployment
baseline, not a substitute for an organization-specific threat analysis:

| Role | Suggested custody | Suggested threshold | Operational purpose |
|---|---|---:|---|
| root | offline devices held by separate people | 2 of 3 or stronger | delegates and rotates every role |
| targets | offline or protected release ceremony | 2 of 3 for a public registry | authorizes the index and attestations |
| snapshot | isolated online publisher | 1 of 1 | binds one coherent metadata set |
| timestamp | isolated online publisher | 1 of 1 | provides freshness and limits freeze attacks |

Keep root keys offline and geographically/administratively separated. Never put
root or targets private keys in the web server, repository bucket, CI logs, or
the Reames Agent source tree. Give snapshot and timestamp different credentials
and narrowly scoped write access. Record public key IDs, thresholds, owners,
backup locations, creation dates, planned expiry, and revocation state in an
offline ceremony record.

Choose expiries that force monitoring without making routine outages dangerous.
A reasonable starting point is timestamp 24 hours, snapshot 7 days, targets 30
days, and root 365 days. Alert well before every deadline; the client correctly
fails closed after expiry.

## Signed Index Contract

The authenticated target is strict JSON schema version 1:

```json
{
  "schemaVersion": 1,
  "registry": "example-production",
  "updated": "2026-07-16T00:00:00Z",
  "plugins": [{
    "name": "example-plugin",
    "description": "One-line operator-reviewed description",
    "version": "1.2.3",
    "author": "Example",
    "category": "development",
    "source": "https://github.com/example/example-plugin",
    "subpath": "plugin",
    "revision": "0123456789abcdef0123456789abcdef01234567",
    "digest": "sha256-git-tree-v1:<64-lowercase-hex>",
    "permissions": ["skills.load"],
    "provenance": {
      "source": "https://github.com/example/example-plugin",
      "subpath": "plugin",
      "revision": "0123456789abcdef0123456789abcdef01234567",
      "digest": "sha256-git-tree-v1:<same-64-lowercase-hex>",
      "builderId": "https://registry.example/builders/release-v1",
      "attestationTarget": "attestations/example-plugin.dsse.json"
    }
  }]
}
```

The source is initially limited to a canonical
`https://github.com/<owner>/<repository>` URL and a full 40-character Git commit.
It must be anonymously fetchable without prompts: registry materialization ignores
system/global Git configuration, credential helpers, filters, replace refs, and LFS
smudge commands. Private repository credentials are not part of this registry contract.
The index permission set must exactly equal the manifest permission set. The
provenance object must bind the exact source, subpath, revision, and
`sha256-git-tree-v1` digest. Display and path fields are length-bounded and cannot contain
terminal control, bidirectional-formatting, or line-separator characters.

An `attestationTarget` is optional. The client verifies that its bytes match the
TUF target metadata, but does not parse or independently verify a DSSE envelope,
builder identity, SLSA predicate, transparency log, or SLSA level. Consult the
[DSSE specification](https://github.com/secure-systems-lab/dsse) and
[SLSA 1.2 provenance model](https://slsa.dev/spec/v1.2/provenance) before adding
such claims. Until a separate policy verifier exists, call this evidence a
“TUF-authenticated attestation target,” not “SLSA verified.”

## Release and Publish Sequence

For every plugin release:

1. Review the canonical repository and exact full commit. Check out that commit
   in a clean environment and run
   `reames-agent plugin registry digest <checkout> [subpath]`; record its full
   revision and cross-platform `sha256-git-tree-v1` digest. Separately run
   `reames-agent plugin install <checkout-or-subpath> --dry-run` to inspect the
   manifest version and normalized permissions without applying it. That preview's
   `sha256-tree-v1` is the local installed-tree digest, not the registry source digest.
2. Verify manifest name, semantic version, permissions, skills, hooks, MCP
   servers, and requested environment contract. Generate and review any
   attestation separately.
3. Update `plugins.json`; ensure its provenance fields exactly match the entry.
4. Write immutable hash-prefixed target files first.
5. Sign and publish targets metadata, then snapshot metadata, and publish
   `timestamp.json` last. Publishing timestamp last prevents clients from seeing
   a newly advertised snapshot before all referenced bytes are available.
6. From a clean, persistent client home containing only the approved bootstrap
   root, run `plugin registry refresh`, `search`, and `show`; then preview and
   install the release and verify the persisted revision, canonical source and
   installed-tree digests, root
   version, bootstrap-root digest, provenance status, and attestation digest.
7. Retain the complete previous repository generation for rollback diagnosis,
   but never republish lower metadata versions. Correct a bad release with new,
   monotonically increasing metadata and a new reviewed plugin version/revision.

Use atomic object publication or immutable object versions. Repository/CDN cache
rules must not serve an old timestamp beside a new snapshot. Monitor freshness,
HTTP failures, target availability, unexpected root versions, and client refresh
failures.

## Rotation

Root metadata versions are sequential. A routine root rotation must produce
version `N+1`, meet the threshold of the trusted `N` root, and meet the threshold
of the new `N+1` root. Publish every intermediate `<N>.root.json`; the client
will not skip a missing version and caps one refresh at 32 rotations.

To rotate targets, snapshot, or timestamp keys, first publish a properly signed
new root that delegates the new keys and removes compromised/retired keys. Then
publish new targets/snapshot/timestamp metadata in normal order. Exercise this
flow with an isolated persistent cache before production.

Changing a user's bootstrap root is an out-of-band trust reset, not ordinary
rotation. It selects a new cache namespace and discards the continuity learned
under the old root. Do it only after independently authenticating the new root
and documenting why ordinary root rotation is unavailable.

## Compromise and Recovery

- A timestamp or snapshot compromise requires revocation through a new root,
  metadata version advancement, and repository republishing. Assume availability
  may be lost while clients fail closed.
- A targets compromise can authorize malicious plugin entries. Revoke it through
  root, publish corrected higher versions, identify affected revisions/digests,
  and notify users to disable/uninstall them. Sandbox and explicit permissions
  reduce impact but do not make a signed malicious plugin safe.
- If enough uncompromised current root keys remain to meet the current threshold,
  use that quorum to rotate the compromised keys. Otherwise stop the registry;
  recovery requires a separately authenticated bootstrap root and explicit user
  action. Treat clients that installed attacker-authorized code as potentially
  compromised.
- Do not tell users to delete `registry-cache` as routine repair. Deleting it
  removes locally learned rollback state and restarts verification from the
  bootstrap root. Preserve it for incident analysis unless a reviewed recovery
  procedure explicitly requires a reset.

No public endpoint, production private-key ceremony, HSM policy, transparency
monitor, or compromise drill is provided by this repository today. Those remain
external operational evidence, not completed by the client implementation.
