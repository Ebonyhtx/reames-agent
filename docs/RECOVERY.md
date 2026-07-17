# Recovery, Guard, and Safe Mode

Reames Agent ships a credential-free recovery path that runs before the normal
Agent runtime. It is intended for broken configuration, repeated Desktop startup
failure, incomplete self-update, corrupted extension state, and derived Desktop
state that prevents the main UI from opening.

The recovery path does not initialize a Provider, read API keys, connect MCP,
load plugins or Hooks, start Bot/Gateway channels, start LSP, or enter the normal
Agent loop. It reuses `internal/repair` as the single durable recovery model;
CLI, Serve, Desktop, Gateway, and the packaged Guard only project that model.

## Entry points

Packaged Desktop installations start through the sibling Guard executable:

- Windows: `Reames Agent.exe` is the GUI-subsystem Guard launcher; the release
  unit also contains `reames-agent-guard.exe` and `reames-agent-desktop.exe`.
- macOS: the app bundle's `CFBundleExecutable` is `reames-agent-guard`; the
  Wails executable remains a sibling inside `Contents/MacOS`.
- Linux: the `.desktop` entry and packaged service launch
  `reames-agent-guard launch`; archives and `.deb` packages include both Guard
  and Desktop.

The main CLI dispatches `guard` before config, i18n, boot, or runtime setup, so
source and CLI-only installations can use the same commands:

```text
reames-agent guard check --json
reames-agent guard repair
reames-agent guard launch --safe-mode
reames-agent guard rollback
reames-agent guard snapshots --json
reames-agent guard restore --snapshot SNAPSHOT_ID
reames-agent guard undo
reames-agent guard rebuild --target tabs|projects|window|zoom|all
reames-agent guard disable-plugins
```

The standalone binary accepts the same subcommands:

```text
reames-agent-guard check --json
reames-agent-guard launch --safe-mode --detach
```

`--app` may name only `reames-agent-desktop[.exe]` in the Guard's own resolved
installation directory. Symlink resolution, basename checks, and the same-dir
constraint prevent Guard from becoming a general executable launcher.

## Startup ledger and crash-loop decision

The bounded startup ledger records these phases:

```text
starting -> ready -> healthy -> clean-exit
                \-> failed
```

- Desktop records `starting` before normal initialization.
- Wails DOM readiness records `ready`.
- A 30-second observation period records `healthy` and confirms a pending
  update only after the UI stayed alive.
- An orderly shutdown records `clean-exit`; shutdown before DOM readiness does
  not falsely certify the startup.
- Three incomplete startups inside five minutes recommend Safe Mode. A live PID
  owner is never replaced or counted as a dead attempt.

The ledger and pending-update state use OS-level cross-process locks and atomic
writes. A healthy or clean state resets the bounded failure count.

## Automatic rollback evidence

Guard automatically changes installed binaries only when all of the following
are true:

1. the startup ledger proves the bounded crash-loop threshold;
2. a syntactically and structurally valid pending-update transaction exists;
3. the failed startup version equals the transaction's `toVersion`;
4. the pending target belongs to the current Guard installation;
5. every non-missing file in the complete release-unit backup passes its
   recorded SHA-256 check;
6. the transaction still has the same version and creation identity while the
   rollback lock is held.

The release unit includes Desktop, Guard, launcher/helper siblings, and files
that were absent before the update. Rollback stages every backup before any
swap, compensates an interrupted swap, removes files introduced by the failed
release, and reports `mixedInstall` when compensation cannot prove a uniform
installed version. A mixed or unattributed installation fails closed and must
be replaced from a verified release.

Windows update-helper failures write a durable apply-failure marker before
relaunching Guard. A missing Windows helper fails the automatic update instead
of starting an unattributed installer. Linux records an apply-failure marker if
Guard replacement lands but Desktop replacement fails. macOS retains the
complete previous `.app` bundle until the replacement passes the health window.
Only one pending update may exist; a later update cannot overwrite probationary
or failed rollback evidence.

If crash attribution, executable provenance, target directory, transaction
identity, or a backup hash is ambiguous, Guard does not mutate binaries. It
selects Safe Mode or asks the operator to reinstall a verified release.

## Safe Mode boundary

Safe Mode is requested with `REAMES_AGENT_SAFE_MODE=1` or
`guard launch --safe-mode`. It uses built-in recovery defaults and intentionally
does not read or migrate user/project TOML or dotenv files.

Disabled in Safe Mode:

- user/project Skills, Hooks, MCP, plugin packages, host extra plugins, and the
  shared plugin host;
- Bot/Gateway channels, LSP, status-line commands, update checks, heartbeat,
  local pending-diagnostic archival, and recovery GC;
- planner, Guardian, subagents, and Memory Compiler;
- restoration of existing Desktop tabs or sessions.

Safe Mode is a diagnostic and repair surface, not a reduced autonomous Agent.
Desktop marks its shell recovery-only and `boot.Build` refuses Provider,
Controller, tool, or Agent assembly. It does not silently grant permissions,
replay tools, start network extensions, or claim that a corrupted installation
is healthy.

## Shared status projection

The same `repair.Report` is available through:

- `control.Controller.RecoveryStatus()`;
- Serve `GET /api/recovery`;
- Desktop `GetRecoveryStatus()`;
- `reames-agent gateway recovery-status [--json] [--home PATH] [--root PATH]`;
- Guard `check` / `diagnose`.

`gateway run` performs the credential-free recovery preflight before loading
configuration, Providers, plugins, or channels. Therefore systemd, launchd, and
Windows Scheduled Task installations use the same preflight without embedding
a second service-specific state machine.

The report contains startup state, configuration checks, pending update,
current/previous binary hashes, session-store readability, plugin-state counts,
and actionable findings. Missing optional files are not automatically errors;
invalid metadata, invalid config, or unreadable plugin state is.

## Desktop Recovery Center

Desktop renders the shared report through a lazy-loaded Recovery Center. In
normal mode it can be opened from the topic bar; when Safe Mode is requested it
becomes the recovery-only shell and the ordinary sidebar, workspace dock,
composer, updater, and session UI remain unavailable.

The UI does not mutate files itself. It sends a bounded `repair.ActionRequest`
through Desktop `RunRecoveryAction`; normal mode delegates to
`control.Controller.RunRecoveryAction`, while Safe Mode calls the same
`repair.ExecuteAction` directly without constructing a Controller or Agent.
Supported operations are:

- repair invalid global or explicitly selected project configuration;
- restore a named healthy configuration snapshot;
- undo the exact last repair transaction;
- roll back only the exact pending update identity shown in the report;
- quarantine/rebuild tabs, projects, window, zoom, or all derived Desktop state;
- disable all managed plugins.

Update rollback and repair undo carry the report-observed version, timestamp, or
transaction ID. The executor rechecks that identity while holding the recovery
action lock, so a stale tab cannot act on newer state. Each result includes a
fresh report. Paths, installation/home/workspace locations, secret-like text,
and action errors are redacted in Go before crossing the Wails boundary. The
frontend uses a monotonic request sequence so late refreshes cannot overwrite a
newer action result.

The installed-candidate recovery smoke launches Guard with an isolated home and
forces recovery-only Safe Mode to DOM-ready. It proves that damaged
`config.toml` and a synthetic `.env` remain byte-identical during startup,
executes config repair followed by exact undo, quarantines tabs/projects/window/
zoom instead of deleting them, requires a final Guard report with no error
finding, and checks that the default user-state boundary did not change. The
latest local Windows Wails Desktop/Guard run passes; Linux/macOS/Windows CI
candidate evidence is generated only after the corresponding workflow runs.

## Operator playbooks

### Desktop repeatedly fails to open

1. Run `reames-agent-guard check --json` and retain the report.
2. Run `reames-agent-guard launch --safe-mode --detach`.
3. If the report identifies invalid config, run `repair`; project config is
   quarantined only with `repair --project`.
4. If derived Desktop state is implicated, use `rebuild --target ...`.
5. Disable all managed plugins only with the explicit `disable-plugins` action.
6. Use `rollback` only for a verified pending update; otherwise reinstall a
   verified release.

### Configuration repair

Healthy global configuration is retained in at most five SHA-256-addressed
snapshots. Repair quarantines invalid bytes instead of overwriting them, may
restore the latest valid global snapshot, and records an undo transaction.
Use `snapshots`, `restore`, and `undo` to make the mutation explicit and
auditable.

### Gateway service refuses to start

```text
reames-agent gateway recovery-status --json --home /absolute/home --root /absolute/workspace
reames-agent gateway doctor --deep --home /absolute/home
```

Repair the reported shared state before restarting the service. Do not bypass
the preflight by copying service definitions or inventing a separate state file.

## Limits and required external evidence

Local tests prove unit, race, fault-injection, cross-process locking, packaging
contracts, cross-compilation, real Guard/child process behavior, and a local
Windows installed-layout recovery smoke. Three-platform candidate smoke proves
the same unsigned CI package contract when the remote workflow passes. Neither
level proves power-loss atomicity below filesystem guarantees, resistance to a
same-host administrator, a publicly signed release chain, notarization, or a
real public-release upgrade/rollback on every OS.

Production claims still require signed release provenance and installed
Windows/macOS/Linux drills, including crash-loop rollback, installer failure,
Safe Mode launch, logout/reboot service persistence, and compromise-response
evidence. Unknown provenance always remains an operator decision.
