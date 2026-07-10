#!/usr/bin/env python3
"""SBOM and build provenance contract verifier.

Checks repository supply-chain integrity artifacts and build
provenance documentation. Actual signing/notarization requires
real certificates (external-blocked).
"""

from __future__ import annotations

from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]


def read(path: str) -> str:
    return (ROOT / path).read_text(encoding="utf-8")


def require(condition: bool, message: str, failures: list[str]) -> None:
    if not condition:
        failures.append(message)


def check_sbom_contract(failures: list[str]) -> None:
    require((ROOT / "go.mod").exists(), "go.mod must exist for Go module SBOM", failures)
    require((ROOT / "desktop" / "go.mod").exists(), "desktop/go.mod must exist for nested module SBOM", failures)

    notice = ROOT / "NOTICE"
    if notice.exists():
        content = notice.read_text(encoding="utf-8")
        require("MIT" in content or "Apache" in content or "BSD" in content,
                "NOTICE must document licenses", failures)
    else:
        failures.append("NOTICE file is missing")


def check_go_module_hashes(failures: list[str]) -> None:
    require((ROOT / "go.sum").exists(), "go.sum must exist for dependency verification", failures)
    require((ROOT / "desktop" / "go.sum").exists(), "desktop/go.sum must exist for nested module verification", failures)


def check_provenance_docs(failures: list[str]) -> None:
    releasing = read("docs/RELEASING.md")
    require("SHA256SUMS" in releasing, "RELEASING.md must document SHA256SUMS", failures)
    require("provenance" in releasing.lower() or "SLSA" in releasing,
            "RELEASING.md must mention build provenance", failures)


def check_ci_security(failures: list[str]) -> None:
    ci = read(".github/workflows/ci.yml")
    require("go vet" in ci, "CI must run go vet", failures)
    codeql_path = ROOT / ".github/workflows/codeql.yml"
    if codeql_path.exists():
        require("CodeQL" in codeql_path.read_text(encoding="utf-8"),
                "CodeQL workflow must exist", failures)
    else:
        failures.append("CodeQL workflow is missing")


def check_threat_model(failures: list[str]) -> None:
    tm = ROOT / "docs" / "THREAT_MODEL.md"
    if tm.exists():
        content = tm.read_text(encoding="utf-8")
        for token in ["credential", "prompt injection", "sandbox", "tool", "plugin"]:
            require(token in content.lower(), f"THREAT_MODEL.md must cover {token}", failures)
    else:
        failures.append("docs/THREAT_MODEL.md is missing — should cover credential handling, prompt injection, tool permissions, plugin supply chain")


def main() -> int:
    failures: list[str] = []
    check_sbom_contract(failures)
    check_go_module_hashes(failures)
    check_provenance_docs(failures)
    check_ci_security(failures)
    check_threat_model(failures)

    if failures:
        print("SBOM/Provenance contract check failed:")
        for f in failures:
            print(f"  - {f}")
        return 1
    print("SBOM/Provenance contract check passed.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
