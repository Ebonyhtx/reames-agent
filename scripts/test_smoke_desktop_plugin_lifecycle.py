from __future__ import annotations

import json
import tempfile
import unittest
from pathlib import Path

from scripts import smoke_desktop_plugin_lifecycle as smoke


class DesktopPluginLifecycleSmokeTests(unittest.TestCase):
    def test_evidence_schema_covers_native_lifecycle_and_cleanup(self) -> None:
        evidence = smoke.PluginLifecycleSmokeResult().to_dict()
        for field in (
            "stale_plan_rejected",
            "installed_disabled",
            "enable_review_visible",
            "enabled_exact_grants",
            "update_planned",
            "updated_version",
            "update_changed_generation",
            "rollback_planned",
            "rolled_back_version",
            "rollback_restored_digest",
            "doctor_invoked",
            "remove_planned",
            "removed",
            "install_root_removed",
            "uia_actions",
            "cleanup_ok",
            "boundary_changes",
        ):
            self.assertIn(field, evidence)
        self.assertEqual(evidence["schema_version"], smoke.SCHEMA_VERSION)
        for field in ("artifact_name", "artifact_sha256", "artifact_size"):
            self.assertIn(field, evidence)

    def test_plugin_source_is_native_schema_v1_with_exact_permissions(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            smoke.write_plugin_source(root, "1.2.3", "fixture marker")
            manifest = json.loads(
                (root / "reames-agent-plugin.json").read_text(encoding="utf-8")
            )
            self.assertEqual(manifest["schemaVersion"], 1)
            self.assertEqual(manifest["name"], smoke.PLUGIN_NAME)
            self.assertEqual(manifest["version"], "1.2.3")
            self.assertEqual(manifest["permissions"], [smoke.PLUGIN_PERMISSION])
            skill = root / "skills" / smoke.PLUGIN_NAME / "SKILL.md"
            self.assertIn("fixture marker", skill.read_text(encoding="utf-8"))

    def test_plugin_record_reads_only_the_named_package(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            home = Path(raw)
            (home / smoke.STATE_FILENAME).write_text(
                json.dumps(
                    {
                        "version": 2,
                        "plugins": [
                            {"name": "other", "version": "9.0.0"},
                            {
                                "name": smoke.PLUGIN_NAME,
                                "version": "1.0.0",
                                "enabled": False,
                            },
                        ],
                    }
                ),
                encoding="utf-8",
            )
            record = smoke.plugin_record(home)
            self.assertIsNotNone(record)
            self.assertEqual(record["version"], "1.0.0")  # type: ignore[index]

    def test_missing_state_is_an_empty_v2_state(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            self.assertEqual(
                smoke.read_plugin_state(Path(raw)), {"version": 2, "plugins": []}
            )


if __name__ == "__main__":
    unittest.main()
