# Headless Gateway smoke audit

## Scope

This batch adds a real-binary smoke for the server/Gateway deployment shape:

- `scripts/smoke_gateway_headless.py`
- `.github/workflows/ci.yml`
- `scripts/verify-baseline.ps1`
- `scripts/check_deploy_contracts.py`
- `docs/DEPLOY.md`

## Contract

The smoke creates an isolated `REAMES_AGENT_HOME`, writes a minimal Gateway
configuration and credentials `.env`, then runs the actual CLI binary through:

1. `reames-agent gateway doctor --json --deep --home <home>`
2. `reames-agent gateway install --dry-run --home <home> --channels feishu`
3. `reames-agent gateway run --home <selected-home> --channels feishu`

It asserts:

- the selected home is the diagnosed home;
- credentials are resolved from the selected home `.env`;
- the rendered service plan pins `REAMES_AGENT_HOME`;
- the rendered plan documents that service definitions do not embed secret
  values;
- the foreground `gateway run --home` entrypoint reads the selected home even
  when an ambient `REAMES_AGENT_HOME` points somewhere else;
- the dummy secret value never appears in output.

## Evidence

- Local/CI command:

  ```bash
  python scripts/smoke_gateway_headless.py --out artifacts/headless-gateway-smoke.json
  ```

- Baseline command:

  ```powershell
  .\scripts\verify-baseline.ps1
  ```

- CI:
  - `deployment-contracts` runs the smoke on Ubuntu with Go 1.25.
  - `deployment-contracts` uploads `artifacts/headless-gateway-smoke.json`.

## Remaining evidence gap

This still stops before starting a real long-lived service or sending a real
Feishu/Lark/Weixin/QQ message. It is a reproducible preflight smoke for the
Hermes-like server shape; a real cloud VM + real channel round trip remains the
next higher-confidence proof.
