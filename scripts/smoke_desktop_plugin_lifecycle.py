#!/usr/bin/env python3
"""Windows native Wails plugin lifecycle smoke using UI Automation.

The smoke runs a real Desktop executable against an isolated home, drives the
plugin settings surface through stable AutomationIds, and verifies each UI
transition against the persisted plugin state. It uses a local synthetic plugin
and the existing localhost provider fixture; no real credential or user state is
read.
"""

from __future__ import annotations

import argparse
import json
import sys
from dataclasses import asdict, dataclass, field
from datetime import datetime, timezone
from pathlib import Path
from typing import Callable

try:
    from scripts import smoke_desktop_interaction as interaction
    from scripts import smoke_desktop_native as native
    from scripts.windows_uia import WindowsUIAutomation, wait_for_window
except ModuleNotFoundError:  # direct ``python scripts/...`` execution
    import smoke_desktop_interaction as interaction  # type: ignore[no-redef]
    import smoke_desktop_native as native  # type: ignore[no-redef]
    from windows_uia import WindowsUIAutomation, wait_for_window  # type: ignore[no-redef]


SCHEMA_VERSION = 1
PLUGIN_NAME = "native-smoke-plugin"
PLUGIN_PERMISSION = "skills.load"
STATE_FILENAME = "plugin-packages.json"
SETTINGS_OPEN_ID = "settings-open"
SETTINGS_DIALOG_ID = "settings-dialog"
PLUGIN_TAB_ID = "settings-tab-plugins"
PLUGIN_PAGE_ID = "plugin-settings-page"
SOURCE_INPUT_ID = "plugin-install-local-source"
INSTALL_PREVIEW_ID = "plugin-install-preview"
INSTALL_APPLY_ID = "plugin-install-apply"
INSTALL_PLAN_ID = "plugin-install-plan"
OPERATION_ERROR_ID = "plugin-operation-error"


def utc_now() -> str:
    return datetime.now(timezone.utc).isoformat()


@dataclass
class PluginLifecycleSmokeResult:
    schema_version: int = SCHEMA_VERSION
    platform: str = sys.platform
    started_at: str = field(default_factory=utc_now)
    finished_at: str = ""
    outcome: str = "unknown"
    failure_kind: str | None = None
    artifact_name: str = ""
    artifact_sha256: str = ""
    artifact_size: int = 0
    executable_name: str = ""
    executable_sha256: str = ""
    executable_size: int = 0
    home_dir: str = ""
    workspace_dir: str = ""
    source_dir: str = ""
    settings_opened: bool = False
    plugin_tab_opened: bool = False
    stale_plan_rejected: bool = False
    installed_disabled: bool = False
    installed_version: str = ""
    installed_digest: str = ""
    enable_review_visible: bool = False
    enabled_exact_grants: bool = False
    update_planned: bool = False
    updated_version: str = ""
    updated_digest: str = ""
    update_changed_generation: bool = False
    rollback_planned: bool = False
    rolled_back_version: str = ""
    rolled_back_digest: str = ""
    rollback_restored_digest: bool = False
    doctor_invoked: bool = False
    remove_planned: bool = False
    removed: bool = False
    install_root_removed: bool = False
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


def fail(result: PluginLifecycleSmokeResult, kind: str, message: str) -> None:
    if result.failure_kind is None:
        result.failure_kind = kind
    result.outcome = "failed"
    result.errors.append(message)


def write_plugin_source(root: Path, version: str, marker: str) -> None:
    skill_dir = root / "skills" / PLUGIN_NAME
    skill_dir.mkdir(parents=True, exist_ok=True)
    manifest = {
        "schemaVersion": 1,
        "name": PLUGIN_NAME,
        "version": version,
        "description": "Native Wails plugin lifecycle fixture",
        "skills": ["skills"],
        "permissions": [PLUGIN_PERMISSION],
    }
    (root / "reames-agent-plugin.json").write_text(
        json.dumps(manifest, separators=(",", ":")) + "\n", encoding="utf-8"
    )
    (skill_dir / "SKILL.md").write_text(
        "---\n"
        f"name: {PLUGIN_NAME}\n"
        "description: Native plugin lifecycle fixture\n"
        "---\n"
        f"{marker}\n",
        encoding="utf-8",
    )


def read_plugin_state(home: Path) -> dict[str, object]:
    path = home / STATE_FILENAME
    if not path.is_file():
        return {"version": 2, "plugins": []}
    raw = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(raw, dict) or not isinstance(raw.get("plugins"), list):
        raise ValueError("invalid plugin state shape")
    return raw


def plugin_record(home: Path) -> dict[str, object] | None:
    try:
        state = read_plugin_state(home)
    except (OSError, ValueError, json.JSONDecodeError):
        return None
    for plugin in state["plugins"]:
        if isinstance(plugin, dict) and plugin.get("name") == PLUGIN_NAME:
            return plugin
    return None


def wait_for_record(
    home: Path,
    predicate: Callable[[dict[str, object]], bool],
    timeout_seconds: float,
    description: str,
) -> dict[str, object]:
    matched: dict[str, object] | None = None

    def check() -> bool:
        nonlocal matched
        record = plugin_record(home)
        if record is not None and predicate(record):
            matched = record
            return True
        return False

    interaction.wait_until(check, timeout_seconds, description)
    if matched is None:
        raise RuntimeError(description)
    return matched


def wait_for_removed(home: Path, timeout_seconds: float) -> None:
    def removed() -> bool:
        try:
            state = read_plugin_state(home)
        except (OSError, ValueError, json.JSONDecodeError):
            return False
        return not any(
            isinstance(plugin, dict) and plugin.get("name") == PLUGIN_NAME
            for plugin in state["plugins"]
        )

    interaction.wait_until(removed, timeout_seconds, "plugin state did not remove the package")


def run_smoke(
    exe_path: str,
    timeout_seconds: int = 45,
    keep_temp: bool = False,
    artifact_path: str | None = None,
) -> PluginLifecycleSmokeResult:
    result = PluginLifecycleSmokeResult(kept_temp=keep_temp)
    if sys.platform != "win32":
        fail(result, "unsupported-platform", "native plugin lifecycle smoke requires Windows")
        result.finished_at = utc_now()
        return result
    try:
        interaction.validate_timeout(timeout_seconds)
    except ValueError as exc:
        fail(result, "invalid-input", str(exc))
        result.finished_at = utc_now()
        return result

    exe = Path(exe_path).resolve(strict=False)
    result.executable_name = exe.name
    if not exe.is_file():
        fail(result, "startup-failure", f"executable not found: {exe}")
        result.finished_at = utc_now()
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
        fail(result, "startup-failure", f"inspect inputs: {exc}")
        result.finished_at = utc_now()
        return result

    fixture_root: Path | None = None
    proc = None
    uia: WindowsUIAutomation | None = None
    with (
        interaction.local_openai_server() as (provider_base_url, _provider_requests),
        interaction.managed_fixture_root(keep_temp) as root,
    ):
        fixture_root = root
        home, workspace = interaction.prepare_fixture(root, provider_base_url)
        source = workspace / PLUGIN_NAME
        write_plugin_source(source, "1.0.0", "initial content")
        result.home_dir = str(home)
        result.workspace_dir = str(workspace)
        result.source_dir = str(source)
        boundary_roots = native.default_boundary_roots(home, exe.name)
        boundary_before = native.snapshot_roots(boundary_roots)

        try:
            proc = interaction.launch_desktop(exe, home)
            hwnd = wait_for_window(proc.pid, timeout_seconds)
            uia = WindowsUIAutomation(hwnd)
            interaction.wait_until(
                lambda: uia.has(automation_id=interaction.COMPOSER_AUTOMATION_ID),
                timeout_seconds,
                "Desktop composer did not become accessible",
            )
            uia.invoke(automation_id=SETTINGS_OPEN_ID, timeout_seconds=timeout_seconds)
            interaction.wait_until(
                lambda: uia.has(automation_id=SETTINGS_DIALOG_ID),
                timeout_seconds,
                "Settings dialog did not open",
            )
            result.settings_opened = True
            uia.invoke(automation_id=PLUGIN_TAB_ID, timeout_seconds=timeout_seconds)
            interaction.wait_until(
                lambda: uia.has(automation_id=PLUGIN_PAGE_ID),
                timeout_seconds,
                "Plugins settings page did not load",
            )
            result.plugin_tab_opened = True

            uia.type_text(str(source), automation_id=SOURCE_INPUT_ID, timeout_seconds=timeout_seconds)
            uia.wait_enabled(automation_id=INSTALL_PREVIEW_ID, timeout_seconds=timeout_seconds)
            uia.invoke(automation_id=INSTALL_PREVIEW_ID, timeout_seconds=timeout_seconds)
            interaction.wait_until(
                lambda: uia.has(automation_id=INSTALL_PLAN_ID),
                timeout_seconds,
                "Plugin install preview did not render",
            )
            uia.wait_enabled(automation_id=INSTALL_APPLY_ID, timeout_seconds=timeout_seconds)

            write_plugin_source(source, "1.0.0", "content drifted after preview")
            uia.invoke(automation_id=INSTALL_APPLY_ID, timeout_seconds=timeout_seconds)
            interaction.wait_until(
                lambda: uia.has(automation_id=OPERATION_ERROR_ID),
                timeout_seconds,
                "Stale plugin install plan was not rejected in the UI",
            )
            if plugin_record(home) is not None:
                raise RuntimeError("stale plugin plan installed content despite rejection")
            result.stale_plan_rejected = True

            uia.invoke(automation_id=INSTALL_PREVIEW_ID, timeout_seconds=timeout_seconds)
            uia.wait_absent(automation_id=OPERATION_ERROR_ID, timeout_seconds=timeout_seconds)
            uia.wait_enabled(automation_id=INSTALL_APPLY_ID, timeout_seconds=timeout_seconds)
            uia.invoke(automation_id=INSTALL_APPLY_ID, timeout_seconds=timeout_seconds)
            installed = wait_for_record(
                home,
                lambda record: record.get("version") == "1.0.0" and record.get("enabled") is False,
                timeout_seconds,
                "Plugin did not install disabled at version 1.0.0",
            )
            interaction.wait_until(
                lambda: uia.has(automation_id=f"plugin-row-{PLUGIN_NAME}"),
                timeout_seconds,
                "Installed plugin row did not render",
            )
            result.installed_disabled = True
            result.installed_version = str(installed.get("version", ""))
            result.installed_digest = str(installed.get("digest", ""))

            uia.invoke(
                automation_id=f"plugin-{PLUGIN_NAME}-details",
                timeout_seconds=timeout_seconds,
            )
            uia.press_space(
                automation_id=f"plugin-{PLUGIN_NAME}-enabled",
                timeout_seconds=timeout_seconds,
            )
            interaction.wait_until(
                lambda: uia.has(automation_id=f"plugin-{PLUGIN_NAME}-enable-review"),
                timeout_seconds,
                "Plugin authorization review did not render",
            )
            result.enable_review_visible = True
            uia.invoke(
                automation_id=f"plugin-{PLUGIN_NAME}-enable-approve",
                timeout_seconds=timeout_seconds,
            )
            enabled = wait_for_record(
                home,
                lambda record: record.get("enabled") is True
                and record.get("digest") == result.installed_digest
                and record.get("grantedPermissions") == [PLUGIN_PERMISSION],
                timeout_seconds,
                "Plugin exact digest/permission authorization was not persisted",
            )
            result.enabled_exact_grants = bool(enabled)

            write_plugin_source(source, "2.0.0", "updated content")
            update_id = f"plugin-{PLUGIN_NAME}-update"
            uia.invoke(automation_id=update_id, timeout_seconds=timeout_seconds)
            interaction.wait_until(
                lambda: uia.has(automation_id=f"plugin-{PLUGIN_NAME}-update-plan"),
                timeout_seconds,
                "Plugin update plan did not render",
            )
            result.update_planned = True
            uia.invoke(automation_id=update_id, timeout_seconds=timeout_seconds)
            updated = wait_for_record(
                home,
                lambda record: record.get("version") == "2.0.0"
                and record.get("enabled") is True
                and record.get("grantedPermissions") == [PLUGIN_PERMISSION]
                and isinstance(record.get("previous"), dict)
                and record["previous"].get("version") == "1.0.0",  # type: ignore[union-attr]
                timeout_seconds,
                "Plugin update did not publish version 2.0.0 with rollback state",
            )
            result.updated_version = str(updated.get("version", ""))
            result.updated_digest = str(updated.get("digest", ""))
            result.update_changed_generation = bool(
                result.updated_digest
                and result.updated_digest != result.installed_digest
            )

            rollback_id = f"plugin-{PLUGIN_NAME}-rollback"
            interaction.wait_until(
                lambda: uia.has(automation_id=rollback_id),
                timeout_seconds,
                "Plugin rollback action did not appear after update",
            )
            uia.invoke(automation_id=rollback_id, timeout_seconds=timeout_seconds)
            interaction.wait_until(
                lambda: uia.has(automation_id=f"plugin-{PLUGIN_NAME}-rollback-plan"),
                timeout_seconds,
                "Plugin rollback plan did not render",
            )
            result.rollback_planned = True
            uia.invoke(automation_id=rollback_id, timeout_seconds=timeout_seconds)
            rolled_back = wait_for_record(
                home,
                lambda record: record.get("version") == "1.0.0"
                and record.get("enabled") is True
                and record.get("grantedPermissions") == [PLUGIN_PERMISSION]
                and isinstance(record.get("previous"), dict)
                and record["previous"].get("version") == "2.0.0",  # type: ignore[union-attr]
                timeout_seconds,
                "Plugin rollback did not restore version 1.0.0",
            )
            result.rolled_back_version = str(rolled_back.get("version", ""))
            result.rolled_back_digest = str(rolled_back.get("digest", ""))
            result.rollback_restored_digest = bool(
                result.installed_digest
                and result.rolled_back_digest == result.installed_digest
            )

            uia.invoke(
                automation_id=f"plugin-{PLUGIN_NAME}-doctor",
                timeout_seconds=timeout_seconds,
            )
            result.doctor_invoked = True

            uia.wait_enabled(
                automation_id=f"plugin-{PLUGIN_NAME}-remove",
                timeout_seconds=timeout_seconds,
            )
            uia.invoke(
                automation_id=f"plugin-{PLUGIN_NAME}-remove",
                timeout_seconds=timeout_seconds,
            )
            uia.invoke(
                automation_id=f"plugin-{PLUGIN_NAME}-remove-confirm",
                timeout_seconds=timeout_seconds,
            )
            interaction.wait_until(
                lambda: uia.has(automation_id=f"plugin-{PLUGIN_NAME}-remove-plan"),
                timeout_seconds,
                "Plugin removal plan did not render",
            )
            result.remove_planned = True
            uia.invoke(
                automation_id=f"plugin-{PLUGIN_NAME}-remove-apply",
                timeout_seconds=timeout_seconds,
            )
            wait_for_removed(home, timeout_seconds)
            uia.wait_absent(
                automation_id=f"plugin-row-{PLUGIN_NAME}",
                timeout_seconds=timeout_seconds,
            )
            result.removed = True
            install_root = home / "plugins" / PLUGIN_NAME
            interaction.wait_until(
                lambda: not install_root.exists(),
                timeout_seconds,
                "Plugin install root remained after removal",
            )
            result.install_root_removed = True
        except Exception as exc:  # noqa: BLE001 - evidence must retain all failures
            fail(result, "interaction-failure", str(exc))
        finally:
            if uia is not None:
                result.uia_actions.extend(uia.actions)
                uia.close()
            if proc is not None:
                cleanup_ok, method = native.cleanup_process(proc, result.errors)
                result.cleanup_method = method
                result.cleanup_exit_code = proc.poll()
                result.cleanup_ok = cleanup_ok
                if not cleanup_ok:
                    fail(result, "cleanup-failure", "Desktop process did not stop")

        result.home_files = native.list_home_files(home)
        result.boundary_changes = native.changed_snapshot(
            boundary_before, native.snapshot_roots(boundary_roots)
        )
        if result.boundary_changes:
            fail(result, "state-leak", "default user state changed during isolated plugin smoke")
        required = (
            result.settings_opened,
            result.plugin_tab_opened,
            result.stale_plan_rejected,
            result.installed_disabled,
            result.enable_review_visible,
            result.enabled_exact_grants,
            result.update_planned,
            result.updated_version == "2.0.0",
            result.update_changed_generation,
            result.rollback_planned,
            result.rolled_back_version == "1.0.0",
            result.rollback_restored_digest,
            result.doctor_invoked,
            result.remove_planned,
            result.removed,
            result.install_root_removed,
            result.cleanup_ok,
            not result.boundary_changes,
        )
        if result.outcome == "unknown" and all(required):
            result.outcome = "passed"
        elif result.outcome == "unknown":
            fail(result, "incomplete-evidence", "native plugin lifecycle fields are incomplete")

    if fixture_root is not None:
        result.temp_cleaned = not fixture_root.exists()
        if not keep_temp and not result.temp_cleaned:
            fail(result, "cleanup-failure", "temporary plugin fixture was not removed")
    result.finished_at = utc_now()
    return result


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--exe", required=True, help="Path to the Desktop executable")
    parser.add_argument("--artifact", help="Candidate package that installed the executable")
    parser.add_argument("--out", help="Write JSON evidence to this path")
    parser.add_argument(
        "--timeout-seconds",
        type=int,
        default=45,
        help=(
            f"Per-step timeout ({interaction.MIN_TIMEOUT_SECONDS}-"
            f"{interaction.MAX_TIMEOUT_SECONDS} seconds)"
        ),
    )
    parser.add_argument("--keep-temp", action="store_true")
    args = parser.parse_args(argv)
    result = run_smoke(
        args.exe,
        timeout_seconds=args.timeout_seconds,
        keep_temp=args.keep_temp,
        artifact_path=args.artifact,
    )
    evidence = result.to_dict()
    if args.out:
        out = Path(args.out)
        out.parent.mkdir(parents=True, exist_ok=True)
        out.write_text(json.dumps(evidence, indent=2) + "\n", encoding="utf-8")
        print(f"Evidence written to {out}")
    print(json.dumps(evidence, indent=2))
    return 0 if result.outcome == "passed" else 1


if __name__ == "__main__":
    raise SystemExit(main())
