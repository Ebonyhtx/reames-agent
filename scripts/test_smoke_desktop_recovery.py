from __future__ import annotations

import hashlib
import os
import tempfile
import unittest
from pathlib import Path

from scripts import smoke_desktop_recovery as smoke


class FakeClock:
    def __init__(self) -> None:
        self.now = 0.0

    def monotonic(self) -> float:
        return self.now

    def sleep(self, seconds: float) -> None:
        self.now += seconds


class RecoverySmokeTests(unittest.TestCase):
    def test_evidence_schema_covers_recovery_contract(self) -> None:
        evidence = smoke.RecoverySmokeResult().to_dict()
        for field in (
            "guard_sha256",
            "safe_mode_ready",
            "config_unchanged_during_safe_mode",
            "credentials_unchanged_during_safe_mode",
            "config_undo_exact",
            "derived_state_quarantined",
            "final_guard_clean",
            "boundary_changes",
        ):
            self.assertIn(field, evidence)

    def test_sha256_file(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            path = Path(raw) / "guard"
            path.write_bytes(b"installed guard")
            self.assertEqual(
                smoke.sha256_file(path),
                hashlib.sha256(b"installed guard").hexdigest().upper(),
            )

    def test_smoke_environment_forces_all_state_roots(self) -> None:
        old = os.environ.get("REAMES_AGENT_PENDING_HELPER")
        home = Path(tempfile.gettempdir()) / "recovery-home"
        try:
            os.environ["REAMES_AGENT_PENDING_HELPER"] = "rollback"
            env = smoke.smoke_environment(home)
        finally:
            if old is None:
                os.environ.pop("REAMES_AGENT_PENDING_HELPER", None)
            else:
                os.environ["REAMES_AGENT_PENDING_HELPER"] = old
        self.assertEqual(env["REAMES_AGENT_HOME"], str(home))
        self.assertEqual(env["REAMES_AGENT_STATE_HOME"], str(home))
        self.assertEqual(env["REAMES_AGENT_CACHE_HOME"], str(home / "cache"))
        self.assertNotIn("REAMES_AGENT_PENDING_HELPER", env)

    def test_report_summary_drops_paths_and_messages(self) -> None:
        summary = smoke.summarize_report(
            {
                "schemaVersion": 1,
                "config": {
                    "checks": [
                        {
                            "scope": "global",
                            "path": "/secret/home/config.toml",
                            "exists": True,
                            "valid": False,
                            "error": "contains smoke-not-a-real-secret",
                        }
                    ]
                },
                "findings": [
                    {
                        "severity": "error",
                        "code": "config.invalid",
                        "message": "contains smoke-not-a-real-secret",
                    }
                ],
            }
        )
        rendered = str(summary)
        self.assertNotIn("/secret/home", rendered)
        self.assertNotIn("smoke-not-a-real-secret", rendered)
        self.assertIn("error:config.invalid", summary["findingCodes"])

    def test_wait_for_safe_mode_ready_requires_live_owner(self) -> None:
        clock = FakeClock()
        states = iter(
            [
                None,
                {"phase": "starting", "safeMode": True, "pid": 7},
                {"phase": "ready", "safeMode": True, "pid": 7},
            ]
        )
        state = smoke.wait_for_safe_mode_ready(
            Path("ignored"),
            2,
            state_reader=lambda _path: next(states, None),
            alive=lambda pid: pid == 7,
            clock=clock.monotonic,
            sleeper=clock.sleep,
        )
        self.assertEqual(state["phase"], "ready")

    def test_derived_quarantine_requires_exact_bytes(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            home = Path(raw)
            returned: list[str] = []
            for _, (filename, body) in smoke.DERIVED_FIXTURES.items():
                quarantine = home / f"{filename}.reames-rebuild-stamp"
                quarantine.write_bytes(body)
                returned.append(str(quarantine))
            self.assertTrue(smoke.verify_derived_quarantine(home, returned))
            Path(returned[0]).write_bytes(b"tampered")
            self.assertFalse(smoke.verify_derived_quarantine(home, returned))

    def test_platform_mismatch_fails_before_reading_inputs(self) -> None:
        requested = "darwin" if smoke.host_platform() != "darwin" else "linux"
        result = smoke.run_smoke("missing", "missing", "missing", requested)
        self.assertEqual(result.failure_kind, "invalid-input")


if __name__ == "__main__":
    unittest.main()
