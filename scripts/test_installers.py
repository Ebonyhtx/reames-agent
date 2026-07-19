"""Installer dry-run contract tests.

These tests keep the server/Gateway one-command installation surface honest
without changing the host. They intentionally exercise the real shell scripts
instead of only grepping source text, because quoting and argument ordering are
part of the deployment contract.
"""

from __future__ import annotations

import os
import platform
import shutil
import subprocess
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]


def run(args: list[str]) -> str:
    proc = subprocess.run(
        args,
        cwd=ROOT,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        check=False,
    )
    if proc.returncode != 0:
        raise AssertionError(
            f"command failed with exit code {proc.returncode}: {args!r}\n{proc.stdout}"
        )
    return proc.stdout


def powershell() -> str | None:
    return shutil.which("pwsh") or shutil.which("powershell")


def unix_bash() -> str | None:
    """Return a real GNU bash, ignoring the unusable Windows WSL app alias."""
    candidates = [shutil.which("bash")]
    if platform.system() == "Windows":
        for root in (os.environ.get("ProgramFiles"), os.environ.get("ProgramFiles(x86)")):
            if root:
                candidates.extend(
                    [
                        str(Path(root) / "Git" / "bin" / "bash.exe"),
                        str(Path(root) / "Git" / "usr" / "bin" / "bash.exe"),
                    ]
                )
    seen: set[str] = set()
    for candidate in candidates:
        if not candidate or candidate in seen or not Path(candidate).is_file():
            continue
        seen.add(candidate)
        try:
            probe = subprocess.run(
                [candidate, "--version"],
                cwd=ROOT,
                stdout=subprocess.PIPE,
                stderr=subprocess.STDOUT,
                check=False,
            )
        except OSError:
            continue
        if probe.returncode == 0 and b"GNU bash" in probe.stdout:
            return candidate
    return None


def powershell_args(script: str, *args: str) -> list[str]:
    exe = powershell()
    if not exe:
        raise unittest.SkipTest("PowerShell is not available")
    base = [exe, "-NoProfile"]
    if Path(exe).name.lower().startswith("powershell"):
        base.extend(["-ExecutionPolicy", "Bypass"])
    return base + ["-File", script, *args]


class InstallerDryRunTests(unittest.TestCase):
    def test_powershell_installer_is_ascii_for_windows_powershell_51(self) -> None:
        data = (ROOT / "scripts" / "install.ps1").read_bytes()
        non_ascii = [(index, value) for index, value in enumerate(data) if value > 0x7F]
        self.assertEqual(
            non_ascii,
            [],
            "install.ps1 must stay pure ASCII: Windows PowerShell 5.1 can "
            "misparse UTF-8-without-BOM punctuation before execution",
        )

    def test_unix_gateway_dry_run_preserves_home_and_credential_boundary(self) -> None:
        bash = unix_bash()
        if not bash:
            self.skipTest("bash is not available")

        run([bash, "-n", "scripts/install.sh"])
        out = run(
            [
                bash,
                "scripts/install.sh",
                "--dry-run",
                "--skip-setup",
                "--home",
                "/home/reames/.reames-agent",
                "--gateway",
                "--channels",
                "feishu",
                "--gateway-dir",
                "/srv/reames-work",
            ]
        )

        self.assertIn("gateway install", out)
        self.assertIn("--home /home/reames/.reames-agent", out)
        self.assertIn("Gateway credential source: /home/reames/.reames-agent/.env", out)
        self.assertIn("do not embed secret values", out)

    @unittest.skipIf(platform.system() == "Windows", "Unix release target is tested on CI Linux")
    def test_unix_release_dry_run_verifies_checksum(self) -> None:
        bash = unix_bash()
        if not bash:
            self.skipTest("bash is not available")

        out = run(
            [
                bash,
                "scripts/install.sh",
                "--dry-run",
                "--skip-setup",
                "--binary-source",
                "release",
                "--version",
                "v0.1.0",
            ]
        )

        self.assertIn("reames-agent-linux-amd64.tar.gz", out)
        self.assertIn("verify SHA256SUMS", out)

    def test_powershell_gateway_dry_run_preserves_home_and_credential_boundary(self) -> None:
        out = run(
            powershell_args(
                "scripts/install.ps1",
                "-DryRun",
                "-SkipSetup",
                "-AgentHome",
                "/home/reames/.reames-agent",
                "-Gateway",
                "-Channels",
                "feishu",
                "-GatewayDir",
                "/srv/reames-work",
            )
        )

        self.assertIn("gateway install", out)
        self.assertIn("--home /home/reames/.reames-agent", out)
        self.assertIn("Gateway credential source: /home/reames/.reames-agent/.env", out)
        self.assertIn("do not embed secret values", out)

    def test_powershell_release_dry_run_verifies_checksum(self) -> None:
        out = run(
            powershell_args(
                "scripts/install.ps1",
                "-DryRun",
                "-SkipSetup",
                "-BinarySource",
                "release",
                "-Version",
                "v0.1.0",
            )
        )

        self.assertIn("reames-agent-windows-amd64.zip", out)
        self.assertIn("verify SHA256SUMS", out)


if __name__ == "__main__":
    unittest.main(verbosity=2)
