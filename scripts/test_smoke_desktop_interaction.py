from __future__ import annotations

import ctypes
import json
import sys
import tempfile
import unittest
import urllib.request
from pathlib import Path

from scripts import smoke_desktop_interaction as smoke
from scripts import windows_uia


class DesktopInteractionSmokeTests(unittest.TestCase):
    def test_evidence_schema_contains_interaction_and_recovery_fields(self) -> None:
        evidence = smoke.InteractionSmokeResult().to_dict()
        for field in (
            "schema_version",
            "onboarding_absent",
            "workspace_selected",
            "message_persisted",
            "provider_bound_to_loopback",
            "provider_received_marker",
            "assistant_response_persisted",
            "stop_completed",
            "recovery_verified",
            "uia_actions",
            "boundary_changes",
            "cleanup_ok",
        ):
            self.assertIn(field, evidence)
        self.assertEqual(evidence["schema_version"], smoke.SCHEMA_VERSION)

    def test_prepare_fixture_is_keyless_and_preseeds_project(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            home, workspace = smoke.prepare_fixture(
                Path(raw), "http://127.0.0.1:45678/v1"
            )
            config = (home / "config.toml").read_text(encoding="utf-8")
            self.assertIn("onboarding_dismissed = true", config)
            self.assertIn('language = "en"', config)
            self.assertIn('default_model = "native-smoke/native-smoke-model"', config)
            self.assertIn('base_url = "http://127.0.0.1:45678/v1"', config)
            self.assertNotIn("api_key_env", config)
            projects = json.loads(
                (home / "desktop-projects.json").read_text(encoding="utf-8")
            )
            self.assertEqual(projects["projects"][0]["title"], smoke.WORKSPACE_TITLE)
            self.assertTrue(smoke.same_path(projects["projects"][0]["root"], workspace))
            self.assertTrue((workspace / "README.md").is_file())

    def test_active_tab_selects_declared_active_entry(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            home = Path(raw)
            (home / "desktop-tabs.json").write_text(
                json.dumps(
                    {
                        "tabs": [
                            {"id": "first", "scope": "global"},
                            {"id": "selected", "scope": "project"},
                        ],
                        "activeTab": "selected",
                    }
                ),
                encoding="utf-8",
            )
            self.assertEqual(smoke.active_tab(home), {"id": "selected", "scope": "project"})

    def test_durable_session_message_check_reads_legacy_checkpoint(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            path = Path(raw) / "session.jsonl"
            path.write_text('{"role":"user","content":"marker-123"}\n', encoding="utf-8")
            self.assertTrue(
                smoke.durable_session_has_message(str(path), "user", "marker-123")
            )
            self.assertFalse(
                smoke.durable_session_has_message(str(path), "user", "marker-456")
            )

    def test_durable_session_message_check_replays_canonical_event_log(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            path = Path(raw) / "session.jsonl"
            path.write_text("", encoding="utf-8")
            path.with_name("session.events.jsonl").write_text(
                json.dumps(
                    {
                        "schema_version": 1,
                        "type": "replace",
                        "messages": [
                            {"role": "user", "content": "marker-123"},
                            {"role": "assistant", "content": "response-123"},
                        ],
                    }
                )
                + "\n",
                encoding="utf-8",
            )
            self.assertTrue(
                smoke.durable_session_has_message(str(path), "user", "marker-123")
            )
            self.assertTrue(
                smoke.durable_session_has_message(
                    str(path), "assistant", "response-123"
                )
            )

    def test_durable_session_message_check_applies_append_chain(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            path = Path(raw) / "session.jsonl"
            path.write_text("", encoding="utf-8")
            records = [
                {
                    "schema_version": 1,
                    "type": "replace",
                    "messages": [{"role": "system", "content": "system"}],
                },
                {
                    "schema_version": 1,
                    "type": "append",
                    "message_index": 1,
                    "messages": [{"role": "user", "content": "marker-123"}],
                },
            ]
            path.with_name("session.events.jsonl").write_text(
                "".join(json.dumps(record) + "\n" for record in records),
                encoding="utf-8",
            )
            self.assertTrue(
                smoke.durable_session_has_message(str(path), "user", "marker-123")
            )

    def test_loopback_provider_is_keyless_and_records_request(self) -> None:
        opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
        with smoke.local_openai_server() as (base_url, requests):
            request = urllib.request.Request(
                f"{base_url}/chat/completions",
                data=json.dumps(
                    {
                        "model": smoke.LOOPBACK_MODEL,
                        "stream": True,
                        "messages": [{"role": "user", "content": "marker-123"}],
                    }
                ).encode("utf-8"),
                headers={"Content-Type": "application/json"},
                method="POST",
            )
            with opener.open(request, timeout=5) as response:
                body = response.read().decode("utf-8")
        self.assertIn(smoke.LOOPBACK_RESPONSE, body)
        self.assertIn("data: [DONE]", body)
        self.assertEqual(len(requests), 1)
        self.assertIn("marker-123", json.dumps(requests))

    def test_timeout_contract(self) -> None:
        self.assertEqual(smoke.validate_timeout(30), 30)
        for invalid in (0, 9, 181):
            with self.assertRaises(ValueError):
                smoke.validate_timeout(invalid)

    def test_long_running_command_is_shell_portable(self) -> None:
        self.assertIn("python -c", smoke.LONG_RUNNING_COMMAND)
        self.assertNotIn("Start-Sleep", smoke.LONG_RUNNING_COMMAND)

    def test_uia_labels_cover_english_and_chinese(self) -> None:
        self.assertEqual(ctypes.sizeof(windows_uia.GUID), 16)
        self.assertIn("New session", windows_uia.NEW_SESSION_NAMES)
        self.assertIn("新建会话", windows_uia.NEW_SESSION_NAMES)
        self.assertIn("Stop", windows_uia.STOP_NAMES)
        self.assertIn("停止", windows_uia.STOP_NAMES)

    def test_composer_controls_expose_stable_automation_ids(self) -> None:
        composer = (
            Path(__file__).parents[1]
            / "desktop"
            / "frontend"
            / "src"
            / "components"
            / "Composer.tsx"
        ).read_text(encoding="utf-8")
        self.assertIn(f'id="{smoke.COMPOSER_AUTOMATION_ID}"', composer)
        self.assertIn(f'id="{smoke.SEND_AUTOMATION_ID}"', composer)
        self.assertIn(f'id="{smoke.STOP_AUTOMATION_ID}"', composer)

    @unittest.skipIf(sys.platform == "win32", "non-Windows classification")
    def test_run_smoke_rejects_non_windows_before_inputs(self) -> None:
        result = smoke.run_smoke("missing.exe")
        self.assertEqual(result.failure_kind, "unsupported-platform")
        self.assertEqual(result.outcome, "failed")


if __name__ == "__main__":
    unittest.main()
