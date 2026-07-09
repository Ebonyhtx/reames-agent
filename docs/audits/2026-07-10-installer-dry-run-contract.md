# Installer dry-run contract audit

## Scope

This batch strengthens the one-command server/Gateway installation surface:

- `scripts/install.sh`
- `scripts/install.ps1`
- `.github/workflows/ci.yml`
- `scripts/check_deploy_contracts.py`
- `scripts/verify-baseline.ps1`
- `docs/DEPLOY.md`

## Contract

Installer dry-runs must stay safe and informative:

1. They do not write to the host.
2. They show the exact `gateway install --home ...` command that will bind the
   Gateway service to the selected `REAMES_AGENT_HOME`.
3. They show `<Reames Agent home>/.env` as the provider/bot credential source.
4. They state that service definitions pin `REAMES_AGENT_HOME` but do not embed
   secret values.
5. Release-mode dry-runs continue to show the selected platform artifact and
   `SHA256SUMS` verification step.

## Automated coverage

- `python -m unittest scripts.test_installers -v`
  - Unix Gateway dry-run home binding and credential-boundary note.
  - Unix release artifact dry-run checksum contract on Linux/macOS.
  - PowerShell Gateway dry-run home binding and credential-boundary note.
  - PowerShell release artifact dry-run checksum contract.
- `scripts/check_deploy_contracts.py`
  - Text-level guard that both installers keep the `.env` and secret-boundary
    wording.
- CI `deployment-contracts`
  - Runs the installer dry-run tests in both Bash and PowerShell surfaces.

## Remaining evidence gap

This is still a dry-run contract. A clean Linux server smoke with a real binary,
real `REAMES_AGENT_HOME`, and at least one configured Gateway channel remains
required before the server deployment story can be called complete.
