import json
import tempfile
import unittest
from pathlib import Path
from unittest import mock

from scripts import gen_upstream_issue_drafts as drafts


class UpstreamIssueDraftTests(unittest.TestCase):
    def test_safe_slug_cannot_traverse_output_directory(self):
        self.assertEqual("evil", drafts.safe_slug("../evil/.."))
        with self.assertRaises(ValueError):
            drafts.safe_slug("../..")

    def test_generate_issue_draft_includes_deep_evidence(self):
        text = drafts.generate_issue_draft({
            "name": "Reasonix",
            "repo": "https://github.com/esengine/DeepSeek-Reasonix.git",
            "branch": "main-v2",
            "decision": "review-required",
            "risk": "high",
            "reviewed": "a" * 40,
            "latest": "b" * 40,
            "recommendation": "Review manually.",
            "areas": {"agent-runtime": 2},
            "files": [{"status": "M", "path": "internal/agent/agent.go"}],
            "deep": {"commits": "abc change", "diff": "+change"},
            "coverage": {"status": "incomplete", "record": "docs/review.json", "missing": ["work-modes"]},
        })
        self.assertIn("## Commits", text)
        self.assertIn("abc change", text)
        self.assertIn("internal/agent/agent.go", text)
        self.assertIn("work-modes", text)

    def test_main_writes_only_attention_drafts(self):
        report = {
            "upstreams": [
                {"id": "changed", "name": "Changed", "repo": "r", "branch": "main", "changed": True},
                {"id": "coverage", "name": "Coverage", "repo": "r", "branch": "main", "changed": False, "coverage": {"status": "incomplete"}},
                {"id": "clean", "name": "Clean", "repo": "r", "branch": "main", "changed": False},
            ]
        }
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            report_path = root / "report.json"
            out = root / "drafts"
            report_path.write_text(json.dumps(report), encoding="utf-8")
            with mock.patch.object(drafts, "DEFAULT_REPORT", report_path), mock.patch.object(drafts, "DRAFT_DIR", out):
                self.assertEqual(drafts.main(), 0)
            self.assertTrue((out / "changed.md").exists())
            self.assertTrue((out / "coverage.md").exists())
            self.assertFalse((out / "clean.md").exists())


if __name__ == "__main__":
    unittest.main()
