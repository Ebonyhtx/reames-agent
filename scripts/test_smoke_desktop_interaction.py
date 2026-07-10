from __future__ import annotations

import ctypes
import json
import sys
import tempfile
import unittest
import urllib.error
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
            "failure_scenarios",
            "stream_partial_persisted",
            "auth_settings_opened",
            "stream_retry_invoked",
            "permission_denied",
            "permission_write_blocked",
            "tool_timeout_error_visible",
            "stop_completed",
            "recovery_verified",
            "uia_actions",
            "boundary_changes",
            "cleanup_ok",
        ):
            self.assertIn(field, evidence)
        self.assertEqual(evidence["schema_version"], smoke.SCHEMA_VERSION)
        self.assertEqual(
            set(evidence["failure_scenarios"]), set(smoke.FAILURE_SCENARIOS)
        )

    def test_prepare_fixture_uses_only_synthetic_loopback_key_and_preseeds_project(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            home, workspace = smoke.prepare_fixture(
                Path(raw), "http://127.0.0.1:45678/v1"
            )
            config = (home / "config.toml").read_text(encoding="utf-8")
            self.assertIn("onboarding_dismissed = true", config)
            self.assertIn('language = "en"', config)
            self.assertIn('default_model = "native-smoke/native-smoke-model"', config)
            self.assertIn('base_url = "http://127.0.0.1:45678/v1"', config)
            self.assertIn(
                f'api_key_env = "{smoke.LOOPBACK_API_KEY_ENV}"', config
            )
            self.assertNotIn(smoke.LOOPBACK_API_KEY, config)
            self.assertIn("bash_timeout_seconds = 1", config)
            self.assertIn('default_tool_approval_mode = "ask"', config)
            self.assertIn('allow = ["Bash(python -c:*)"]', config)
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

    def test_loopback_provider_success_records_request(self) -> None:
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
        self.assertIn(f"{smoke.LOOPBACK_RESPONSE}: marker-123", body)
        self.assertIn("data: [DONE]", body)
        self.assertEqual(len(requests), 1)
        self.assertIn("marker-123", json.dumps(requests))

    def test_loopback_provider_scripts_native_failure_scenarios(self) -> None:
        opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))

        def post(base_url: str, messages: list[dict[str, str]]):
            request = urllib.request.Request(
                f"{base_url}/chat/completions",
                data=json.dumps(
                    {"model": smoke.LOOPBACK_MODEL, "stream": True, "messages": messages}
                ).encode("utf-8"),
                headers={"Content-Type": "application/json"},
                method="POST",
            )
            return opener.open(request, timeout=5)

        with smoke.local_openai_server() as (base_url, requests):
            with self.assertRaises(urllib.error.HTTPError) as auth_error:
                post(base_url, [{"role": "user", "content": smoke.INVALID_KEY_PROMPT}])
            self.assertEqual(auth_error.exception.code, 401)

            with self.assertRaises(urllib.error.HTTPError) as rate_error:
                post(base_url, [{"role": "user", "content": smoke.RATE_LIMIT_PROMPT}])
            self.assertEqual(rate_error.exception.code, 429)
            self.assertEqual(rate_error.exception.headers.get("Retry-After"), "8")
            with post(
                base_url, [{"role": "user", "content": smoke.RATE_LIMIT_PROMPT}]
            ) as response:
                self.assertIn(smoke.LOOPBACK_RESPONSE, response.read().decode("utf-8"))

            with post(
                base_url,
                [{"role": "user", "content": smoke.STREAM_INTERRUPTION_PROMPT}],
            ) as response:
                interrupted = response.read().decode("utf-8")
            self.assertIn(smoke.STREAM_PARTIAL_RESPONSE, interrupted)
            self.assertNotIn("data: [DONE]", interrupted)

            for prompt, call_id, tool_name, final_response in (
                (
                    smoke.PERMISSION_DENIAL_PROMPT,
                    smoke.PERMISSION_TOOL_CALL_ID,
                    "write_file",
                    smoke.PERMISSION_DENIAL_RESPONSE,
                ),
                (
                    smoke.TOOL_TIMEOUT_PROMPT,
                    smoke.TIMEOUT_TOOL_CALL_ID,
                    "bash",
                    smoke.TOOL_TIMEOUT_RESPONSE,
                ),
            ):
                with post(base_url, [{"role": "user", "content": prompt}]) as response:
                    tool_call = response.read().decode("utf-8")
                self.assertIn(call_id, tool_call)
                self.assertIn(f'"name":"{tool_name}"', tool_call)
                with post(
                    base_url,
                    [
                        {"role": "user", "content": prompt},
                        {"role": "tool", "content": "blocked"},
                    ],
                ) as response:
                    final = response.read().decode("utf-8")
                self.assertIn(final_response, final)

        counts = {
            scenario: sum(
                smoke.request_scenario(payload) == scenario for payload in requests
            )
            for scenario in smoke.FAILURE_SCENARIOS
        }
        self.assertEqual(counts["invalid_key"], 1)
        self.assertEqual(counts["rate_limit"], 2)
        self.assertEqual(counts["stream_interruption"], 1)
        self.assertEqual(counts["permission_denial"], 2)
        self.assertEqual(counts["tool_timeout"], 2)

    def test_request_scenario_uses_latest_user_turn(self) -> None:
        payload = {
            "messages": [
                {"role": "user", "content": smoke.INVALID_KEY_PROMPT},
                {"role": "assistant", "content": "failed"},
                {"role": "user", "content": "recovery turn"},
            ]
        }
        self.assertEqual(smoke.request_scenario(payload), "success")
        payload["messages"].append(
            {
                "role": "user",
                "content": "The previous assistant response was interrupted during streaming.",
            }
        )
        self.assertEqual(smoke.request_scenario(payload), "stream_interruption")

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
        self.assertIn('id="composer-runstatus"', composer)

        frontend = Path(__file__).parents[1] / "desktop" / "frontend" / "src"
        runtime_error = (frontend / "components" / "RuntimeErrorNotice.tsx").read_text(
            encoding="utf-8"
        )
        approval = (frontend / "components" / "ApprovalModal.tsx").read_text(
            encoding="utf-8"
        )
        tool_card = (frontend / "components" / "ToolCard.tsx").read_text(
            encoding="utf-8"
        )
        settings = (frontend / "components" / "SettingsPanel.tsx").read_text(
            encoding="utf-8"
        )
        self.assertIn("notice-${item.code}", runtime_error)
        self.assertIn("error-action-settings-${item.code", runtime_error)
        self.assertIn("error-action-retry-${item.code", runtime_error)
        self.assertIn(f'id="{smoke.SETTINGS_MODAL_AUTOMATION_ID}"', settings)
        self.assertIn(f'id="{smoke.SETTINGS_CLOSE_AUTOMATION_ID}"', settings)
        self.assertIn(f'automationId="{smoke.TOOL_APPROVAL_AUTOMATION_ID}"', approval)
        self.assertIn(f'automationId="{smoke.TOOL_DENY_AUTOMATION_ID}"', approval)
        self.assertIn("tool-error-${item.id}", tool_card)

    @unittest.skipIf(sys.platform == "win32", "non-Windows classification")
    def test_run_smoke_rejects_non_windows_before_inputs(self) -> None:
        result = smoke.run_smoke("missing.exe")
        self.assertEqual(result.failure_kind, "unsupported-platform")
        self.assertEqual(result.outcome, "failed")


if __name__ == "__main__":
    unittest.main()
