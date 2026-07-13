# M6 Linux systemd user lifecycle audit

## Scope

This batch turns the Linux Gateway service-manager path from a rendered-plan
contract into an installed-process lifecycle with explicit readiness evidence.
It also repairs the CLI release-upgrade contract needed by future server
operations.

Production changes:

- Linux uninstall now executes `disable --now`, deletes the unit, and only then
  runs `daemon-reload`.
- systemd `ExecStart`, `Environment`, and `WorkingDirectory` use directive-aware
  encoding; service installation rejects relative executable, home, and working
  directory paths.
- service definitions are written with the shared crash-safe atomic writer.
- `gateway install --start-now` now performs `daemon-reload`, `enable`,
  `restart`, and `is-active --quiet`, so reinstalling an active service applies
  the new unit immediately instead of leaving the old process running.
- CLI upgrade now reads releases from `Ebonyhtx/reames-agent`, selects the exact
  `reames-agent-<os>-<arch>.(tar.gz|zip)` asset, and extracts the GoReleaser
  Windows binary as `reames-agent.exe`.

## Deterministic fixture

`scripts/smoke_gateway_service_linux.py` uses a real cross-compiled CLI binary,
a real systemd user manager, isolated homes and workspaces whose paths contain
spaces, and a minimal Feishu webhook configuration. The configuration contains
no app ID, app secret, Provider key, or real IM credential. A random high-entropy
verification token and loopback port are created for each configuration.

Readiness is proven by POSTing a Feishu `url_verification` challenge to
`127.0.0.1`; systemd `active` alone is not accepted. The smoke verifies:

- config mode `0600`, unit mode `0644`, and zero-diagnostic
  `systemd-analyze --user verify`;
- initial install, service-manager status, journal start banner, and loopback
  challenge;
- same-name reinstall to a second home/workspace, changed PID, updated unit,
  old endpoint shutdown, and new endpoint readiness;
- explicit restart with another changed PID and endpoint readiness;
- stop with endpoint unreachability, then start with another changed PID and
  restored readiness;
- uninstall, unit deletion, and final `LoadState=not-found`;
- absence of both random tokens from command output, unit content, and report;
- temporary state cleanup.

## Failure-led repair

The first WSL run failed with `LoadState=bad-setting`: systemd treated the Go
quoted `WorkingDirectory="/tmp/..."` value as a non-absolute path. Replacing it
with `\x20` escaping made static verification pass, but a path containing spaces
then failed at runtime with `status=200/CHDIR`; `WorkingDirectory` consumes its
entire right-hand side and does not decode that escape. The final implementation
keeps interior spaces verbatim and only escapes systemd `%` specifiers. The
strict path-with-spaces lifecycle then passed.

## Local evidence

Environment:

- WSL2 Ubuntu, Linux `6.6.87.2-microsoft-standard-WSL2`, x86_64;
- systemd user manager state `running`;
- tested binary size `34,245,235` bytes;
- tested binary SHA-256
  `B7AD96B6B0C2C3B10978C31D2CFA637583938E274C61C9508CF38A8A32419315`.

Final report facts:

- status `passed`, `errors=[]`;
- initial/reinstall/restart/start PIDs `420/465/505/546`;
- initial, reinstall, restart, and start webhook readiness all passed;
- old webhook after reinstall and active webhook after stop were unreachable;
- reinstall unit update, journal banner, status command, and cleanup passed;
- uninstall ended at `LoadState=not-found`.

Targeted gates:

```text
go test ./internal/gatewayservice -count=1
go test ./internal/cli -count=1
python -m unittest scripts.test_smoke_gateway_service_linux -v
python scripts/check_release_contracts.py
python scripts/check_deploy_contracts.py
```

The machine-readable local report is intentionally kept under untracked
`artifacts/` and is not committed.

## Evidence boundary

The WSL user has `Linger=no`. This proves the service-manager lifecycle inside
an active login session, not persistence after SSH/logout or a cloud reboot.
The report therefore keeps the following as `external_blocked`:

- real Provider authentication, usage, and network behavior;
- real IM text, approval, cancel, and recovery round trips;
- logout persistence with a linger-enabled service user;
- a maintained cloud VM lifecycle.

This batch also does not prove a public signed release upgrade. Service install
rollback after a service-manager failure, full home backup/restore, upgrade
rollback, signing, provenance, and cloud reboot drills remain open M6/release
work.
