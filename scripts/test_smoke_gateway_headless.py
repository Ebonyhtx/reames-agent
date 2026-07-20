from __future__ import annotations

import json
import tempfile
import unittest
from pathlib import Path

from scripts import smoke_gateway_headless as smoke


class HeadlessGatewaySmokeTests(unittest.TestCase):
    def test_sse_fixture_is_openai_compatible_and_complete(self) -> None:
        body = smoke.sse_text_response("hello").decode("utf-8")
        self.assertIn('"content":"hello"', body)
        self.assertIn('"finish_reason":"stop"', body)
        self.assertTrue(body.endswith("data: [DONE]\n\n"))

    def test_latest_user_text_uses_last_user_message(self) -> None:
        payload = {
            "messages": [
                {"role": "user", "content": "first"},
                {"role": "assistant", "content": "answer"},
                {"role": "user", "content": "last"},
            ]
        }
        self.assertEqual(smoke.latest_user_text(payload), "last")

    def test_append_loopback_provider_keeps_secret_out_of_config(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / "config.toml"
            path.write_text('default_model = "deepseek-flash"\n', encoding="utf-8")
            smoke.append_loopback_provider(path, "http://127.0.0.1:1234/v1")
            body = path.read_text(encoding="utf-8")
            self.assertIn('name        = "headless-smoke"', body)
            self.assertIn('api_key_env = "REAMES_HEADLESS_SMOKE_API_KEY"', body)
            self.assertNotIn(smoke.LOOPBACK_KEY, body)
            with self.assertRaises(AssertionError):
                smoke.append_loopback_provider(path, "http://127.0.0.1:1234/v1")

    def test_fixture_response_payload_can_be_parsed(self) -> None:
        events = [
            line.removeprefix("data: ")
            for line in smoke.sse_text_response("ok").decode("utf-8").splitlines()
            if line.startswith("data: {")
        ]
        parsed = [json.loads(event) for event in events]
        self.assertEqual(parsed[0]["choices"][0]["delta"]["content"], "ok")
        self.assertEqual(parsed[-1]["usage"]["total_tokens"], 13)

    def test_sensitive_value_guard_rejects_fixture_secrets(self) -> None:
        smoke.assert_sensitive_values_absent("redacted evidence only")
        for forbidden in (smoke.FEEDBACK_EMAIL, smoke.FEEDBACK_SECRET, smoke.LOOPBACK_KEY):
            with self.assertRaises(AssertionError):
                smoke.assert_sensitive_values_absent(f"leak: {forbidden}")

    def test_sensitive_value_tree_guard_scans_all_persisted_files(self) -> None:
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            nested = root / "evidence" / "report.json"
            nested.parent.mkdir(parents=True)
            nested.write_text('{"status":"passed"}', encoding="utf-8")
            smoke.assert_sensitive_values_absent_from_tree(root)
            nested.write_text(smoke.FEEDBACK_SECRET, encoding="utf-8")
            with self.assertRaisesRegex(AssertionError, "report.json"):
                smoke.assert_sensitive_values_absent_from_tree(root)

    def test_report_guard_rejects_null_evidence_section(self) -> None:
        report = {
            section: {"verified": True}
            for section in (
                "setup",
                "recovery_preflight",
                "doctor",
                "service_plan",
                "foreground_run",
                "cli_run",
                "feedback",
            )
        }
        smoke.assert_report_sections_complete(report)
        report["feedback"] = None
        with self.assertRaisesRegex(AssertionError, "feedback"):
            smoke.assert_report_sections_complete(report)


if __name__ == "__main__":
    unittest.main()
