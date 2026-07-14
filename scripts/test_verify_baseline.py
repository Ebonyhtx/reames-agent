from __future__ import annotations

import re
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SCRIPT = ROOT / "scripts" / "verify-baseline.ps1"


class VerifyBaselineContractTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.source = SCRIPT.read_text(encoding="utf-8")

    def test_output_directory_is_configurable(self) -> None:
        self.assertRegex(
            self.source,
            re.compile(r"\[string\]\$OutputDir\s*=\s*\"\"", re.MULTILINE),
        )
        self.assertIn("[System.IO.Path]::IsPathRooted($OutputDir)", self.source)

    def test_default_output_directory_uses_system_temp(self) -> None:
        self.assertIn("[System.IO.Path]::GetTempPath()", self.source)
        self.assertIn('"reames-agent-baseline"', self.source)
        self.assertNotIn('Join-Path $RepoRoot "artifacts"', self.source)

    def test_gateway_smoke_uses_configured_output_directory(self) -> None:
        self.assertIn(
            '$gatewaySmokeOutput = Join-Path $OutputDir "headless-gateway-smoke.json"',
            self.source,
        )
        self.assertIn('"--out", $gatewaySmokeOutput', self.source)
        self.assertNotIn("artifacts/headless-gateway-smoke.json", self.source)
        self.assertNotIn("artifacts\\headless-gateway-smoke.json", self.source)


if __name__ == "__main__":
    unittest.main()
