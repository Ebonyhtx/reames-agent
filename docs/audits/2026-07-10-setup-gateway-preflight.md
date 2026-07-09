# Setup Gateway preflight audit

## Scope

This batch links first-run setup to the server/Gateway deployment path:

- `internal/cli/cli.go`
- `internal/cli/cli_test.go`
- `docs/DEPLOY.md`
- `scripts/check_deploy_contracts.py`

## Contract

After `reames-agent setup` or non-interactive default config creation, the CLI
prints two copyable next steps:

1. `reames-agent gateway doctor --deep --home <Reames Agent home>`
2. `reames-agent gateway install --dry-run --home <Reames Agent home> --channels feishu --dir <workspace>`

This keeps setup aligned with the Hermes-like server shape: configure first,
diagnose the same `REAMES_AGENT_HOME`, then review the background service plan
before installing it.

## Evidence

- `internal/cli/cli_test.go`
  - `TestWriteDefaultConfigPrintsGatewayPreflight` asserts both next-step
    commands include the selected `REAMES_AGENT_HOME`.
- `scripts/check_deploy_contracts.py`
  - requires the deploy guide to retain the setup-to-Gateway preflight wording.

## Boundary

The hint does not install a service, start the gateway, or collect secrets. It
only points the user to existing read-only diagnostics and dry-run planning.
