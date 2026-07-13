"""Windows Desktop accessibility smoke using strict UI Automation patterns.

The smoke launches a production Wails executable with an isolated synthetic
home, inspects native UIA roles/properties, and proves that a real modal removes
its background from the accessibility tree. It never reads a real API key or
falls back to screenshots, coordinates, or injected mouse input.
"""

from __future__ import annotations

import argparse
import json
import subprocess
import sys
import time
from dataclasses import asdict, dataclass, field
from pathlib import Path

try:
    from scripts import smoke_desktop_interaction as interaction
    from scripts import smoke_desktop_native as native
except ModuleNotFoundError:  # direct ``python scripts/...`` execution
    import smoke_desktop_interaction as interaction  # type: ignore[no-redef]
    import smoke_desktop_native as native  # type: ignore[no-redef]


SCHEMA_VERSION = 1
APP_MAIN_ID = "app-main"
TRANSCRIPT_LOG_ID = "transcript-log"
TRANSCRIPT_ANNOUNCER_ID = "transcript-announcer"
SKIP_LINK_ID = "skip-to-composer"
COMPOSER_ID = "composer-input"
SETTINGS_OPEN_ID = "settings-open"
SETTINGS_DIALOG_ID = "settings-dialog"
SETTINGS_CLOSE_ID = "settings-modal-close"

UIA_BUTTON_CONTROL_TYPE = 50000
UIA_EDIT_CONTROL_TYPE = 50004
UIA_HYPERLINK_CONTROL_TYPE = 50005

BACKGROUND_IDS = (
    APP_MAIN_ID,
    TRANSCRIPT_LOG_ID,
    TRANSCRIPT_ANNOUNCER_ID,
    SKIP_LINK_ID,
    COMPOSER_ID,
    SETTINGS_OPEN_ID,
)


@dataclass
class AccessibilitySmokeResult:
    schema_version: int = SCHEMA_VERSION
    platform: str = sys.platform
    started_at: str = field(default_factory=interaction.utc_now)
    finished_at: str = ""
    outcome: str = "unknown"
    failure_kind: str | None = None
    executable_name: str = ""
    executable_sha256: str = ""
    executable_size: int = 0
    artifact_name: str = ""
    artifact_sha256: str = ""
    artifact_size: int = 0
    home_dir: str = ""
    workspace_dir: str = ""
    initial_elements: dict[str, dict[str, object]] = field(default_factory=dict)
    dialog_element: dict[str, object] = field(default_factory=dict)
    close_element: dict[str, object] = field(default_factory=dict)
    restored_opener: dict[str, object] = field(default_factory=dict)
    skip_focused_composer: bool = False
    background_absent_while_modal: bool = False
    dialog_focus_verified: bool = False
    opener_focus_restored: bool = False
    strict_invoke_only: bool = False
    uia_actions: list[dict[str, object]] = field(default_factory=list)
    cleanup_method: str = ""
    cleanup_exit_code: int | None = None
    cleanup_ok: bool = False
    temp_cleaned: bool = False
    kept_temp: bool = False
    home_files: list[str] = field(default_factory=list)
    boundary_changes: list[str] = field(default_factory=list)
    errors: list[str] = field(default_factory=list)

    def to_dict(self) -> dict[str, object]:
        return asdict(self)


def _fail(result: AccessibilitySmokeResult, kind: str, message: str) -> None:
    if result.failure_kind is None:
        result.failure_kind = kind
    result.outcome = "failed"
    result.errors.append(message)


def _require_element(
    info: object,
    *,
    role: str | None = None,
    control_type: int | None = None,
    focusable: bool | None = None,
    on_screen: bool | None = None,
) -> None:
    if not getattr(info, "localized_control_type", ""):
        raise RuntimeError(
            f"{getattr(info, 'automation_id', '')!r} has no localized control type"
        )
    if not getattr(info, "enabled", False):
        raise RuntimeError(f"{getattr(info, 'automation_id', '')!r} is disabled")
    if role is not None and getattr(info, "aria_role", "").lower() != role:
        raise RuntimeError(
            f"{getattr(info, 'automation_id', '')!r} role is "
            f"{getattr(info, 'aria_role', '')!r}, expected {role!r}"
        )
    if control_type is not None and getattr(info, "control_type", 0) != control_type:
        raise RuntimeError(
            f"{getattr(info, 'automation_id', '')!r} control type is "
            f"{getattr(info, 'control_type', 0)}, expected {control_type}"
        )
    if focusable is not None and bool(
        getattr(info, "is_keyboard_focusable", False)
    ) != focusable:
        raise RuntimeError(
            f"{getattr(info, 'automation_id', '')!r} keyboard focusable mismatch"
        )
    if on_screen is not None and (not bool(getattr(info, "is_offscreen", False))) != on_screen:
        raise RuntimeError(f"{getattr(info, 'automation_id', '')!r} on-screen mismatch")


def _require_idle_log_properties(properties: str) -> None:
    normalized = properties.lower()
    # WebView2 omits the default false value for aria-busy from UIA even when
    # the DOM attribute is explicit. Busy=true would be a real idle-state
    # regression; omission and busy=false are equivalent here.
    if "live=off" not in normalized or "busy=true" in normalized:
        raise RuntimeError(
            "transcript log ARIA properties must expose live=off and must not expose busy=true: "
            f"{normalized!r}"
        )


def _require_dialog_properties(properties: str) -> None:
    normalized = properties.lower()
    # WebView2 currently omits aria-modal from UIA AriaProperties. The smoke
    # proves modality behaviorally by requiring the background to disappear;
    # an explicitly exposed false value is still a contract failure.
    if "modal=false" in normalized:
        raise RuntimeError(
            f"settings dialog explicitly exposes modal=false: {normalized!r}"
        )


def _wait_for_idle_log(uia: object, timeout_seconds: int) -> object:
    deadline = time.monotonic() + timeout_seconds
    last: object | None = None
    while True:
        remaining = max(1, int(deadline - time.monotonic()))
        last = uia.element_info(
            automation_id=TRANSCRIPT_LOG_ID,
            timeout_seconds=min(timeout_seconds, remaining),
        )
        properties = getattr(last, "aria_properties", "").lower()
        if "live=off" in properties and "busy=true" not in properties:
            _require_idle_log_properties(properties)
            return last
        if time.monotonic() >= deadline:
            _require_idle_log_properties(properties)
        time.sleep(0.1)


def _inspect_initial_elements(uia: object, timeout_seconds: int) -> dict[str, object]:
    elements: dict[str, object] = {}
    for automation_id in BACKGROUND_IDS:
        elements[automation_id] = (
            _wait_for_idle_log(uia, timeout_seconds)
            if automation_id == TRANSCRIPT_LOG_ID
            else uia.element_info(
                automation_id=automation_id, timeout_seconds=timeout_seconds
            )
        )
    _require_element(elements[APP_MAIN_ID], role="main", on_screen=True)
    _require_element(elements[TRANSCRIPT_LOG_ID], role="log", on_screen=True)
    _require_element(elements[TRANSCRIPT_ANNOUNCER_ID], role="status")
    _require_element(
        elements[SKIP_LINK_ID],
        control_type=UIA_HYPERLINK_CONTROL_TYPE,
        focusable=True,
    )
    _require_element(
        elements[COMPOSER_ID],
        control_type=UIA_EDIT_CONTROL_TYPE,
        focusable=True,
        on_screen=True,
    )
    _require_element(
        elements[SETTINGS_OPEN_ID],
        control_type=UIA_BUTTON_CONTROL_TYPE,
        focusable=True,
        on_screen=True,
    )
    _require_idle_log_properties(
        getattr(elements[TRANSCRIPT_LOG_ID], "aria_properties", "")
    )
    announcer_properties = getattr(
        elements[TRANSCRIPT_ANNOUNCER_ID], "aria_properties", ""
    ).lower()
    if "live=polite" not in announcer_properties or "atomic=true" not in announcer_properties:
        raise RuntimeError(
            "transcript announcer ARIA properties do not expose polite/atomic semantics: "
            f"{announcer_properties!r}"
        )
    return elements


def run_smoke(
    exe_path: str,
    timeout_seconds: int = 30,
    keep_temp: bool = False,
    artifact_path: str | None = None,
) -> AccessibilitySmokeResult:
    result = AccessibilitySmokeResult(kept_temp=keep_temp)
    try:
        interaction.validate_timeout(timeout_seconds)
    except ValueError as exc:
        _fail(result, "invalid-arguments", str(exc))
        result.finished_at = interaction.utc_now()
        return result
    if sys.platform != "win32":
        _fail(result, "unsupported-platform", "Desktop accessibility smoke requires Windows")
        result.finished_at = interaction.utc_now()
        return result

    try:
        from scripts.windows_uia import WindowsUIAutomation, wait_for_window
    except ModuleNotFoundError:  # direct ``python scripts/...`` execution
        from windows_uia import WindowsUIAutomation, wait_for_window  # type: ignore[no-redef]

    exe = Path(exe_path).resolve(strict=False)
    result.executable_name = exe.name
    if not exe.is_file():
        _fail(result, "startup-failure", f"executable not found: {exe}")
        result.finished_at = interaction.utc_now()
        return result
    try:
        result.executable_size = exe.stat().st_size
        result.executable_sha256 = native.sha256_file(exe)
        if artifact_path:
            artifact = Path(artifact_path).resolve(strict=False)
            result.artifact_name = artifact.name
            if not artifact.is_file():
                raise FileNotFoundError(f"artifact not found: {artifact}")
            result.artifact_size = artifact.stat().st_size
            result.artifact_sha256 = native.sha256_file(artifact)
    except OSError as exc:
        _fail(result, "startup-failure", f"inspect inputs: {exc}")
        result.finished_at = interaction.utc_now()
        return result

    fixture_root: Path | None = None
    proc: subprocess.Popen | None = None
    uia: WindowsUIAutomation | None = None
    with (
        interaction.local_openai_server() as (provider_base_url, _requests),
        interaction.managed_fixture_root(keep_temp) as root,
    ):
        fixture_root = root
        home, workspace = interaction.prepare_fixture(root, provider_base_url)
        result.home_dir = str(home)
        result.workspace_dir = str(workspace)
        boundary_roots = native.default_boundary_roots(home, exe.name)
        boundary_before = native.snapshot_roots(boundary_roots)

        try:
            proc = interaction.launch_desktop(exe, home)
            hwnd = wait_for_window(proc.pid, timeout_seconds)
            uia = WindowsUIAutomation(hwnd)
            initial = _inspect_initial_elements(uia, timeout_seconds)
            result.initial_elements = {
                key: asdict(value) for key, value in initial.items()
            }

            uia.invoke_pattern(
                automation_id=SKIP_LINK_ID, timeout_seconds=timeout_seconds
            )
            composer = uia.wait_focused(
                automation_id=COMPOSER_ID, timeout_seconds=timeout_seconds
            )
            result.skip_focused_composer = composer.has_keyboard_focus

            uia.invoke_pattern(
                automation_id=SETTINGS_OPEN_ID, timeout_seconds=timeout_seconds
            )
            dialog = uia.element_info(
                automation_id=SETTINGS_DIALOG_ID, timeout_seconds=timeout_seconds
            )
            _require_element(dialog, role="dialog", on_screen=True)
            if not dialog.name:
                raise RuntimeError("settings dialog has no accessible name")
            _require_dialog_properties(dialog.aria_properties)
            result.dialog_element = asdict(dialog)

            close = uia.wait_focused(
                automation_id=SETTINGS_CLOSE_ID, timeout_seconds=timeout_seconds
            )
            _require_element(
                close,
                control_type=UIA_BUTTON_CONTROL_TYPE,
                focusable=True,
                on_screen=True,
            )
            result.close_element = asdict(close)
            result.dialog_focus_verified = close.has_keyboard_focus

            for automation_id in BACKGROUND_IDS:
                uia.wait_absent(
                    automation_id=automation_id, timeout_seconds=timeout_seconds
                )
            result.background_absent_while_modal = True

            uia.invoke_pattern(
                automation_id=SETTINGS_CLOSE_ID, timeout_seconds=timeout_seconds
            )
            uia.wait_absent(
                automation_id=SETTINGS_DIALOG_ID, timeout_seconds=timeout_seconds
            )
            opener = uia.wait_focused(
                automation_id=SETTINGS_OPEN_ID, timeout_seconds=timeout_seconds
            )
            result.restored_opener = asdict(opener)
            result.opener_focus_restored = opener.has_keyboard_focus
            for automation_id in BACKGROUND_IDS:
                uia.element_info(
                    automation_id=automation_id, timeout_seconds=timeout_seconds
                )

            result.uia_actions.extend(uia.actions)
            result.strict_invoke_only = bool(result.uia_actions) and all(
                action.get("action") == "invoke-pattern"
                for action in result.uia_actions
            )
            if not result.strict_invoke_only:
                raise RuntimeError("accessibility smoke used a non-InvokePattern action")
        except Exception as exc:
            _fail(result, "accessibility-failure", str(exc))
        finally:
            if uia is not None:
                if not result.uia_actions:
                    result.uia_actions.extend(uia.actions)
                uia.close()
            if proc is not None:
                result.cleanup_ok, result.cleanup_method = native.cleanup_process(
                    proc, result.errors
                )
                result.cleanup_exit_code = proc.poll()

        result.home_files = native.list_home_files(home)
        result.boundary_changes = native.changed_snapshot(
            boundary_before, native.snapshot_roots(boundary_roots)
        )
        if result.boundary_changes:
            _fail(result, "state-leak", "default user state changed during accessibility smoke")
        if not result.cleanup_ok:
            _fail(result, "cleanup-failure", "Desktop process did not stop")
        if result.outcome == "unknown":
            result.outcome = "passed"

    if fixture_root is not None:
        result.temp_cleaned = not fixture_root.exists()
        if not keep_temp and not result.temp_cleaned:
            _fail(result, "cleanup-failure", "temporary accessibility fixture was not removed")
    result.finished_at = interaction.utc_now()
    return result


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--exe", required=True, help="Path to the Desktop executable")
    parser.add_argument("--artifact", help="Candidate package that installed the executable")
    parser.add_argument("--out", help="Write JSON evidence to this path")
    parser.add_argument(
        "--timeout-seconds",
        type=int,
        default=30,
        help=(
            f"Per-step timeout ({interaction.MIN_TIMEOUT_SECONDS}-"
            f"{interaction.MAX_TIMEOUT_SECONDS} seconds)"
        ),
    )
    parser.add_argument("--keep-temp", action="store_true")
    args = parser.parse_args(argv)
    try:
        interaction.validate_timeout(args.timeout_seconds)
    except ValueError as exc:
        parser.error(str(exc))

    result = run_smoke(
        args.exe,
        timeout_seconds=args.timeout_seconds,
        keep_temp=args.keep_temp,
        artifact_path=args.artifact,
    )
    evidence = result.to_dict()
    if args.out:
        out_path = Path(args.out)
        out_path.parent.mkdir(parents=True, exist_ok=True)
        out_path.write_text(json.dumps(evidence, indent=2) + "\n", encoding="utf-8")
        print(f"Evidence written to {out_path}")
    print(json.dumps(evidence, indent=2))
    return 0 if result.outcome == "passed" else 1


if __name__ == "__main__":
    raise SystemExit(main())
