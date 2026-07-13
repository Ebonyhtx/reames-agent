from __future__ import annotations

import platform
import unittest
from dataclasses import asdict
from pathlib import Path

from scripts import smoke_gateway_service_linux as smoke


class GatewayServiceLinuxSmokeTests(unittest.TestCase):
    def test_evidence_schema_covers_full_lifecycle_and_boundaries(self) -> None:
        evidence = asdict(smoke.GatewayServiceSmokeResult())
        for field in (
            "systemd_manager_state",
            "linger_state",
            "logout_persistence_verified",
            "config_written",
            "config_mode",
            "unit_written",
            "unit_verified",
            "unit_verify_output_empty",
            "initial_active",
            "initial_pid",
            "initial_webhook_verified",
            "reinstall_completed",
            "reinstall_config_mode",
            "reinstall_pid",
            "reinstall_pid_changed",
            "reinstall_unit_updated",
            "reinstall_old_webhook_unreachable",
            "reinstall_new_webhook_verified",
            "status_completed",
            "restart_active",
            "restart_pid_changed",
            "restart_webhook_verified",
            "stop_inactive",
            "stop_webhook_unreachable",
            "start_active",
            "start_pid_changed",
            "start_webhook_verified",
            "journal_has_start_banner",
            "token_absent_from_outputs",
            "token_absent_from_unit",
            "uninstall_completed",
            "unit_removed",
            "unit_load_state_after_uninstall",
            "external_blocked",
            "errors",
        ):
            self.assertIn(field, evidence)

    def test_parse_systemctl_show_ignores_non_properties(self) -> None:
        state = smoke.parse_systemctl_show(
            "LoadState=loaded\nActiveState=active\nnoise\nMainPID=42\n"
        )
        self.assertEqual(
            state,
            {"LoadState": "loaded", "ActiveState": "active", "MainPID": "42"},
        )

    def test_secret_guard_rejects_exact_fixture_value(self) -> None:
        with self.assertRaisesRegex(AssertionError, "credential leaked"):
            smoke.assert_secret_absent("synthetic-secret", ["x synthetic-secret y"])
        smoke.assert_secret_absent("synthetic-secret", ["secret env name only"])

    def test_webhook_fixture_has_no_real_credentials(self) -> None:
        config = smoke.webhook_config("random-token", 31415)
        self.assertIn('mode = "webhook"', config)
        self.assertIn('verification_token = "random-token"', config)
        self.assertIn("webhook_port = 31415", config)
        self.assertIn("[bot.pairing]", config)
        self.assertNotIn("app_id", config)
        self.assertNotIn("app_secret", config)

    def test_source_uses_real_cli_for_every_lifecycle_action(self) -> None:
        source = Path(smoke.__file__).read_text(encoding="utf-8")
        for action in ("status", "restart", "stop", "start", "uninstall"):
            self.assertIn(f'gateway_args(binary, "{action}", service_name)', source)
        self.assertEqual(
            source.count('gateway_args(binary, "install", service_name)'), 2
        )
        self.assertIn('"systemctl", "--user"', source)
        self.assertIn('"systemd-analyze", "--user", "verify"', source)
        self.assertIn('"loginctl",', source)
        self.assertIn('"journalctl", "--user"', source)
        self.assertIn("verify_webhook(", source)
        self.assertIn("webhook_unreachable(", source)
        self.assertIn('result.unit_load_state_after_uninstall != "not-found"', source)

    @unittest.skipIf(platform.system() == "Linux", "non-Linux classification")
    def test_non_linux_rejects_before_reading_binary(self) -> None:
        result = smoke.run_smoke(Path("missing-binary"))
        self.assertEqual(result.status, "failed")
        self.assertIn("Linux is required", result.errors[0])


if __name__ == "__main__":
    unittest.main()
