# Headless Gateway smoke audit

## Scope

This batch adds a real-binary smoke for the server/Gateway deployment shape:

- `scripts/smoke_gateway_headless.py`
- `.github/workflows/ci.yml`
- `scripts/verify-baseline.ps1`
- `scripts/check_deploy_contracts.py`
- `docs/DEPLOY.md`

## Contract

The contract was strengthened on 2026-07-13 when the headless setup command
became available; the workflow and report path remain unchanged.

The smoke creates an isolated `REAMES_AGENT_HOME` and runs the actual CLI
binary through:

1. `reames-agent gateway setup --dry-run --home <home> ...`
2. `reames-agent gateway setup --home <home> ...`
3. the same `gateway setup` command a second time;
4. `reames-agent gateway doctor --json --deep --home <home>`;
5. `reames-agent gateway install --dry-run --home <home> --channels feishu`;
6. `reames-agent gateway run --home <selected-home> --channels feishu`.

The setup path intentionally references a secret environment variable that is
not set. It never writes a synthetic credential or hand-authored Gateway TOML.

It asserts:

- the selected home is the diagnosed home;
- setup dry-run does not create `config.toml`;
- setup applies atomically, then an identical rerun reports `unchanged` and
  leaves the config byte-for-byte unchanged;
- no credentials `.env` is created and doctor reports the missing secret
  environment variable by name;
- the rendered service plan pins `REAMES_AGENT_HOME`;
- the rendered plan documents that service definitions do not embed secret
  values;
- the foreground `gateway run --home` entrypoint reads the selected home even
  when an ambient `REAMES_AGENT_HOME` points somewhere else;
- the application identifier remains redacted from command output.

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
Feishu/Lark/Weixin/QQ message. It is a reproducible, credential-free preflight
smoke for the Hermes-like server shape; a real cloud VM + real channel round
trip remains the next higher-confidence proof.
