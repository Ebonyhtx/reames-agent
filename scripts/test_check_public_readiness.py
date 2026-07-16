import tempfile
import unittest
from pathlib import Path
from unittest import mock

from scripts import check_public_readiness as readiness


class WorkflowActionRuntimeTests(unittest.TestCase):
    def test_rejects_node20_action_majors(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            workflows = root / ".github" / "workflows"
            workflows.mkdir(parents=True)
            (workflows / "ci.yml").write_text(
                "steps:\n"
                "  - uses: actions/checkout@v4\n"
                "  - uses: actions/setup-node@v4\n"
                "  - uses: actions/upload-artifact@v4\n",
                encoding="utf-8",
            )
            (workflows / "legacy.yaml").write_text(
                "steps:\n"
                "  - uses: actions/github-script@v7\n"
                "  - uses: actions/checkout@1111111111111111111111111111111111111111\n",
                encoding="utf-8",
            )
            failures: list[str] = []
            with mock.patch.object(readiness, "ROOT", root):
                readiness.check_workflow_action_runtimes(failures)
            self.assertEqual(5, len(failures), failures)
            self.assertEqual(4, sum("Node 24 baseline" in failure for failure in failures))
            self.assertTrue(any("unaudited" in failure for failure in failures))

    def test_accepts_node24_majors_and_immutable_pins(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            workflows = root / ".github" / "workflows"
            workflows.mkdir(parents=True)
            (workflows / "ci.yml").write_text(
                "steps:\n"
                "  - uses: actions/checkout@v7\n"
                "  - uses: actions/setup-go@v7\n"
                "  - uses: actions/setup-python@v6\n"
                "  - uses: actions/setup-node@v7\n"
                "  - uses: actions/upload-artifact@v7\n"
                "  - uses: actions/github-script@v9\n"
                "  - uses: pnpm/action-setup@v6\n"
                "  - uses: actions/checkout@9c091bb21b7c1c1d1991bb908d89e4e9dddfe3e0\n",
                encoding="utf-8",
            )
            failures: list[str] = []
            with mock.patch.object(readiness, "ROOT", root):
                readiness.check_workflow_action_runtimes(failures)
            self.assertEqual([], failures)


class LegacyTreeAndBrandTests(unittest.TestCase):
    def test_rejects_removed_legacy_paths(self):
        for rel in [
            "agent/runtime.py",
            "apps/desktop/package.json",
            "site/package.json",
            "workers/forum/wrangler.toml",
            "pyproject.toml",
            "REAMES_AGENT.md",
            ".github/workflows/pages.yml",
        ]:
            self.assertIsNotNone(readiness.legacy_path_failure(rel), rel)

        for rel in [
            "internal/agent/agent.go",
            "desktop/frontend/package.json",
            "docs/PROJECT.md",
            ".github/workflows/ci.yml",
        ]:
            self.assertIsNone(readiness.legacy_path_failure(rel), rel)

    def test_reasonix_filename_compatibility_is_narrow(self):
        self.assertEqual(
            [],
            readiness.brand_failures_for_text(
                "internal/memory/doc.go",
                'var names = []string{"AGENTS.md", "REASONIX.md", "REASONIX.local.md"}',
            ),
        )
        failures = readiness.brand_failures_for_text(
            "internal/i18n/messages_en.go",
            'const label = "Reasonix Agent"',
        )
        self.assertEqual(1, len(failures), failures)

    def test_rejects_hermes_runtime_brand_in_active_surfaces(self):
        failures = readiness.brand_failures_for_text(
            "desktop/app.go",
            'const home = "HERMES_HOME" // Hermes Agent by Nous Research',
        )
        self.assertEqual(3, len(failures), failures)

        self.assertEqual(
            [],
            readiness.brand_failures_for_text(
                "docs/audits/2026-07-17-repository-cleanup.md",
                "Historical Hermes Agent source was removed.",
            ),
        )


if __name__ == "__main__":
    unittest.main()
