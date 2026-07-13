from __future__ import annotations

import dataclasses
import inspect
import sys
import unittest
from unittest import mock

from scripts import smoke_desktop_accessibility as accessibility
from scripts.windows_uia import ElementInfo


def element(
    automation_id: str,
    *,
    role: str,
    control_type: int,
    aria_properties: str = "",
    focusable: bool = True,
    offscreen: bool = False,
) -> ElementInfo:
    return ElementInfo(
        index=0,
        name=automation_id,
        automation_id=automation_id,
        control_type=control_type,
        enabled=True,
        localized_control_type="localized",
        has_keyboard_focus=False,
        is_keyboard_focusable=focusable,
        is_offscreen=offscreen,
        aria_role=role,
        aria_properties=aria_properties,
    )


class DesktopAccessibilitySmokeTests(unittest.TestCase):
    def test_result_schema_contains_native_accessibility_evidence(self) -> None:
        fields = {
            field.name
            for field in dataclasses.fields(accessibility.AccessibilitySmokeResult)
        }
        self.assertTrue(
            {
                "initial_elements",
                "dialog_element",
                "close_element",
                "restored_opener",
                "background_absent_while_modal",
                "strict_invoke_only",
                "boundary_changes",
                "cleanup_ok",
            }.issubset(fields)
        )

    def test_background_ids_cover_every_surface_hidden_by_settings(self) -> None:
        self.assertEqual(
            set(accessibility.BACKGROUND_IDS),
            {
                "app-main",
                "transcript-log",
                "transcript-announcer",
                "skip-to-composer",
                "composer-input",
                "settings-open",
            },
        )

    def test_require_element_accepts_matching_native_contract(self) -> None:
        accessibility._require_element(
            element(
                "settings-open",
                role="button",
                control_type=accessibility.UIA_BUTTON_CONTROL_TYPE,
            ),
            role="button",
            control_type=accessibility.UIA_BUTTON_CONTROL_TYPE,
            focusable=True,
            on_screen=True,
        )

    def test_require_element_rejects_role_and_visibility_drift(self) -> None:
        with self.assertRaisesRegex(RuntimeError, "role"):
            accessibility._require_element(
                element("settings-dialog", role="group", control_type=50033),
                role="dialog",
            )
        with self.assertRaisesRegex(RuntimeError, "on-screen"):
            accessibility._require_element(
                element(
                    "settings-open",
                    role="button",
                    control_type=accessibility.UIA_BUTTON_CONTROL_TYPE,
                    offscreen=True,
                ),
                on_screen=True,
            )

    def test_idle_log_accepts_webview_omitted_busy_false(self) -> None:
        accessibility._require_idle_log_properties("atomic=false;live=off;readonly=true")
        accessibility._require_idle_log_properties("live=off;busy=false")

    def test_idle_log_rejects_live_or_busy_drift(self) -> None:
        with self.assertRaisesRegex(RuntimeError, "live=off"):
            accessibility._require_idle_log_properties("live=polite;busy=false")
        with self.assertRaisesRegex(RuntimeError, "busy=true"):
            accessibility._require_idle_log_properties("live=off;busy=true")

    def test_dialog_accepts_webview_omitted_modal_and_rejects_false(self) -> None:
        accessibility._require_dialog_properties("readonly=true")
        accessibility._require_dialog_properties("modal=true;readonly=true")
        with self.assertRaisesRegex(RuntimeError, "modal=false"):
            accessibility._require_dialog_properties("modal=false;readonly=true")

    def test_wait_for_idle_log_outlasts_hydration(self) -> None:
        busy = element(
            "transcript-log",
            role="log",
            control_type=50026,
            aria_properties="live=off;busy=true",
        )
        idle = element(
            "transcript-log",
            role="log",
            control_type=50026,
            aria_properties="live=off",
        )
        uia = mock.Mock()
        uia.element_info.side_effect = [busy, idle]
        with mock.patch.object(accessibility.time, "sleep"):
            self.assertIs(accessibility._wait_for_idle_log(uia, 2), idle)
        self.assertEqual(uia.element_info.call_count, 2)

    def test_smoke_source_uses_only_strict_invoke_api(self) -> None:
        source = inspect.getsource(accessibility.run_smoke)
        self.assertEqual(source.count("uia.invoke_pattern("), 3)
        self.assertNotIn("uia.invoke(", source)
        self.assertNotIn("SetCursorPos", source)

    def test_non_windows_returns_before_reading_inputs(self) -> None:
        with mock.patch.object(sys, "platform", "linux"):
            result = accessibility.run_smoke("missing.exe")
        self.assertEqual(result.outcome, "failed")
        self.assertEqual(result.failure_kind, "unsupported-platform")
        self.assertEqual(result.executable_name, "")


if __name__ == "__main__":
    unittest.main()
