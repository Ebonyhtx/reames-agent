# Gateway service lifecycle contract audit

> Date: 2026-07-09  
> Scope: `reames-agent gateway install/start/stop/restart/status/uninstall`

## Conclusion

The cloud/server gateway is now covered by a stricter service lifecycle contract. The rendered plans must keep the Hermes-like shape the user asked for:

- SSH/terminal CLI remains a normal foreground `reames-agent` / `reames-agent run` experience.
- IM integration runs as an independent `reames-agent gateway run` daemon.
- Background lifecycle commands use the host service manager: systemd, launchd, or Windows Scheduled Task.
- Gateway service plans must not silently regress to `serve` or the legacy foreground `bot start` entrypoint.

## Why this matters

Cloud deployment should not turn `serve` into the hidden prerequisite for every entrypoint. It also should not require the user to keep a bot process occupying the same terminal used for CLI/TUI work. A server install should support both patterns at the same time:

```text
SSH / tmux
└─ reames-agent / reames-agent run

Background service manager
└─ reames-agent gateway run --home "$REAMES_AGENT_HOME" --channels feishu
```

That separation is the key product difference between a convenient server agent and a web-server-only deployment.

## Evidence

Automated coverage:

- `internal/gatewayservice/service_test.go`
  - Linux install plans render `gateway run`, `REAMES_AGENT_HOME`, channels, workspace, model and `Restart=always`.
  - macOS install plans render launchd `ProgramArguments` for `gateway run` and `EnvironmentVariables.REAMES_AGENT_HOME`.
  - Windows install plans render Scheduled Task commands that set `REAMES_AGENT_HOME` before `gateway run`.
  - Cross-platform install plans are checked against legacy regressions: no `bot start`, no `serve`.
  - Lifecycle plans for `status` and `restart` are checked to use platform service managers without rendering file mutations.
- `internal/cli/bot_test.go`
  - `gateway install --dry-run` prints a service lifecycle plan and does not regress to `bot start`.
- `internal/cli/cli_test.go`
  - CLI `gateway install --dry-run` picks up the selected `REAMES_AGENT_HOME`.
- `scripts/check_deploy_contracts.py`
  - Deployment docs and installers must keep CLI/Gateway separation, safe dry-run examples, and `--home "$REAMES_AGENT_HOME"`.

## Remaining validation

This is still contract-level evidence. M6/C1 still needs:

- a clean Linux server install smoke with `scripts/install.sh --gateway --channels feishu`;
- at least one real gateway service start/status/restart/status cycle on Linux systemd;
- Windows Scheduled Task and macOS launchd dry-run plus host smoke;
- a real Feishu/Lark message round trip with approval and cancellation;
- log, evidence ledger and session recovery checks through the gateway process.

Until those are done, this should be treated as a hardened implementation contract, not as completion of cloud gateway deployment.
