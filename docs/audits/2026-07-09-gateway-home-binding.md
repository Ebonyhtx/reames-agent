# Gateway service home binding audit

> Date: 2026-07-09  
> Scope: `reames-agent gateway install/start/status` service lifecycle and server deployment consistency

## Conclusion

The Hermes-like cloud/server shape requires the foreground CLI and the background gateway daemon to use the same Reames Agent home. Otherwise a server can appear correctly configured in SSH/TUI while the installed gateway service silently starts with a different config and credential store.

This batch adds an explicit gateway service home binding:

- `reames-agent gateway install --home PATH`
- default `--home` value from the current `REAMES_AGENT_HOME` environment variable when set
- Linux systemd plan: `Environment=REAMES_AGENT_HOME="..."`
- macOS launchd plan: `EnvironmentVariables.REAMES_AGENT_HOME`
- Windows Scheduled Task plan: `cmd.exe /C set "REAMES_AGENT_HOME=..." && reames-agent gateway run ...`
- dry-run output documents `<Reames Agent home>/.env` as the provider/bot secret source and states that service definitions do not embed secret values.

## Why this matters

`<Reames Agent home>/.env` is the authoritative runtime source for provider credentials and bot secrets. The normal foreground CLI can inherit `REAMES_AGENT_HOME` from the user's shell, but OS service managers do not reliably inherit that interactive environment.

For cloud deployment, this caused a real product risk:

```text
SSH shell / tmux
└─ REAMES_AGENT_HOME=/home/reames/.reames-agent
   └─ reames-agent run works

Gateway service
└─ launched by systemd/launchd/Scheduled Task
   └─ may fall back to a different default home unless the service plan pins it
```

The fix makes the service plan self-contained and auditable before it touches the host.

## Evidence

Automated coverage:

- `internal/gatewayservice/service_test.go`
  - Linux systemd unit renders `Environment=REAMES_AGENT_HOME=...`
  - macOS launchd plist renders `EnvironmentVariables`
  - Windows Scheduled Task command sets `REAMES_AGENT_HOME` before running the gateway
  - formatted service plans show the selected credentials `.env` path while keeping provider/bot secret values out of service definitions
- `internal/cli/cli_test.go`
  - `gateway help` documents `--home PATH`
  - `gateway install --dry-run` picks up current `REAMES_AGENT_HOME`
  - dry-run output documents that secrets remain in `<Reames Agent home>/.env`
- `internal/cli/bot_test.go`
  - gateway help documents foreground and background entrypoints
- `scripts/check_deploy_contracts.py`
  - deployment docs must keep `--home "$REAMES_AGENT_HOME"` in the gateway service examples
  - source-build installers must pass the selected Reames Agent home into `gateway install`

Manual boundary:

- This is still service-plan validation, not a clean Linux/macOS/Windows host install smoke.
- It does not prove a real Feishu connection or a real provider call through the background daemon.
- Those remain M6/C1 validation work.

## Operational guidance

Recommended server flow:

```bash
export REAMES_AGENT_HOME="$HOME/.reames-agent"
scripts/install.sh --home "$REAMES_AGENT_HOME" --gateway --channels feishu --gateway-dir /srv/project
reames-agent gateway install --dry-run --home "$REAMES_AGENT_HOME" --channels feishu --dir /srv/project
reames-agent gateway install --start-now --home "$REAMES_AGENT_HOME" --channels feishu --dir /srv/project
```

If `REAMES_AGENT_HOME` is not set, Reames Agent still uses its platform default home. Passing `--home` is recommended for cloud deployments because it makes the service definition explicit and easier to audit.
