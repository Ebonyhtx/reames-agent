from __future__ import annotations

import dataclasses
import unittest

from scripts import windows_uia


def element(
    *,
    automation_id: str = "control",
    focused: bool = False,
) -> windows_uia.ElementInfo:
    return windows_uia.ElementInfo(
        index=0,
        name="Control",
        automation_id=automation_id,
        control_type=50000,
        enabled=True,
        localized_control_type="button",
        has_keyboard_focus=focused,
        is_keyboard_focusable=True,
        is_offscreen=False,
        aria_role="button",
        aria_properties="",
    )


def driver() -> windows_uia.WindowsUIAutomation:
    value = object.__new__(windows_uia.WindowsUIAutomation)
    value.actions = []
    return value


class WindowsUIAutomationContractTests(unittest.TestCase):
    def test_element_info_schema_covers_accessibility_properties(self) -> None:
        names = {field.name for field in dataclasses.fields(windows_uia.ElementInfo)}
        self.assertTrue(
            {
                "localized_control_type",
                "has_keyboard_focus",
                "is_keyboard_focusable",
                "is_offscreen",
                "aria_role",
                "aria_properties",
            }.issubset(names)
        )

    def test_element_info_returns_exact_match(self) -> None:
        value = driver()
        info = element(automation_id="settings-dialog")
        value._find = lambda **_kwargs: {"info": info, "element": object()}  # type: ignore[method-assign]
        self.assertIs(
            value.element_info(automation_id="settings-dialog", timeout_seconds=0.1),
            info,
        )

    def test_wait_focused_uses_current_uia_focus_property(self) -> None:
        value = driver()
        snapshots = [
            [element(automation_id="settings-open", focused=False)],
            [element(automation_id="settings-open", focused=True)],
        ]
        value.refresh = lambda: snapshots.pop(0)  # type: ignore[method-assign]
        focused = value.wait_focused(
            automation_id="settings-open", timeout_seconds=0.2
        )
        self.assertTrue(focused.has_keyboard_focus)

    def test_strict_invoke_never_falls_back_to_bounds(self) -> None:
        value = driver()
        info = element(automation_id="settings-open")
        item = {"info": info, "element": object()}
        value._find = lambda **_kwargs: item  # type: ignore[method-assign]
        value._invoke_pattern_for_item = lambda _item: False  # type: ignore[method-assign]
        value._invoke_by_bounds = lambda _item: self.fail("strict invoke touched bounds")  # type: ignore[method-assign]
        with self.assertRaisesRegex(RuntimeError, "InvokePattern unavailable"):
            value.invoke_pattern(automation_id="settings-open")
        self.assertEqual(value.actions, [])

    def test_strict_invoke_records_only_invoke_pattern(self) -> None:
        value = driver()
        info = element(automation_id="settings-open")
        value._find = lambda **_kwargs: {"info": info, "element": object()}  # type: ignore[method-assign]
        value._invoke_pattern_for_item = lambda _item: True  # type: ignore[method-assign]
        self.assertEqual(value.invoke_pattern(automation_id="settings-open"), "invoke-pattern")
        self.assertEqual(value.actions[0]["action"], "invoke-pattern")

    def test_legacy_invoke_falls_back_only_when_pattern_is_unavailable(self) -> None:
        value = driver()
        info = element()
        item = {"info": info, "element": object()}
        bounds_calls: list[object] = []
        value._find = lambda **_kwargs: item  # type: ignore[method-assign]
        value._invoke_pattern_for_item = lambda _item: False  # type: ignore[method-assign]
        value._invoke_by_bounds = lambda selected: bounds_calls.append(selected)  # type: ignore[method-assign]
        self.assertEqual(value.invoke(automation_id="control"), "uia-bounds-click")
        self.assertEqual(bounds_calls, [item])

    def test_invoke_pattern_failure_does_not_fall_back(self) -> None:
        value = driver()
        info = element()
        value._find = lambda **_kwargs: {"info": info, "element": object()}  # type: ignore[method-assign]

        def fail_pattern(_item: object) -> bool:
            raise OSError("invoke failed")

        value._invoke_pattern_for_item = fail_pattern  # type: ignore[method-assign]
        value._invoke_by_bounds = lambda _item: self.fail("failed InvokePattern touched bounds")  # type: ignore[method-assign]
        with self.assertRaisesRegex(OSError, "invoke failed"):
            value.invoke(automation_id="control")


if __name__ == "__main__":
    unittest.main()
