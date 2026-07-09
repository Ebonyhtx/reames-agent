# Installer release artifact mode audit

> Date: 2026-07-09  
> Scope: `scripts/install.sh`, `scripts/install.ps1`, CI deployment contracts

## Conclusion

The Reames installers now have a forward-compatible release artifact path without changing the current safe default.

- Default install mode remains `source`, because stable public releases are not enabled yet.
- Explicit release mode is available for future stable releases:
  - Unix: `scripts/install.sh --binary-source release --version v0.1.0`
  - Windows: `scripts/install.ps1 -BinarySource release -Version v0.1.0`
- Release mode downloads only Reames-owned GitHub Release artifacts and verifies them against `SHA256SUMS`.
- Gateway service installation still receives the same `--home` / `REAMES_AGENT_HOME` binding after either binary source.

## Why this matters

Reasonix already has mature prebuilt CLI release distribution. Reames should eventually absorb that advantage, but it should not pretend to have a stable release channel before the release governance, signing, and updater boundaries are ready.

This keeps the deployment story honest:

```text
today:  source build by default
future: explicit release artifact install + SHA256SUMS verification
later:  signatures / Sigstore / npm / Homebrew after stable release gates
```

## Evidence

Automated and dry-run coverage:

- `scripts/install.sh`
  - `--binary-source source` remains the default.
  - `--binary-source release --version v...` selects `reames-agent-<os>-<arch>.tar.gz`.
  - release mode downloads `SHA256SUMS` and verifies the selected archive.
- `scripts/install.ps1`
  - `-BinarySource source` remains the default.
  - `-BinarySource release -Version v...` selects `reames-agent-windows-<arch>.zip`.
  - release mode downloads `SHA256SUMS` and verifies the selected archive with `Get-FileHash`.
- `.github/workflows/ci.yml`
  - deployment contracts dry-run both source/gateway and release modes.
- `scripts/check_deploy_contracts.py`
  - installer text-level checks require explicit binary source mode, Reames release artifact shape, and `SHA256SUMS` verification.

Local dry-run evidence:

```text
bash scripts/install.sh --dry-run --skip-setup --binary-source release --version v0.1.0
→ reames-agent-linux-amd64.tar.gz
→ verify SHA256SUMS contains reames-agent-linux-amd64.tar.gz

powershell -File scripts\install.ps1 -DryRun -SkipSetup -BinarySource release -Version v0.1.0
→ reames-agent-windows-amd64.zip
→ verify SHA256SUMS contains reames-agent-windows-amd64.zip
```

## Remaining validation

This does not open production publishing. Before making release mode the recommended default, Reames still needs:

- real GitHub Release creation policy owned by this repository;
- signed checksums or Sigstore/cosign keyless signing;
- clean Linux/macOS/Windows artifact install smoke from an actual release;
- npm/Homebrew wrapper governance if those channels are enabled;
- updater trust checks that fail closed to Reames-owned endpoints only.
