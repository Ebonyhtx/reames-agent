import importlib.util
import json
import subprocess
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SPEC = importlib.util.spec_from_file_location("audit_reasonix_generation", ROOT / "scripts" / "audit_reasonix_generation.py")
audit = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
SPEC.loader.exec_module(audit)


class ReasonixGenerationAuditTests(unittest.TestCase):
    def test_parser_enumerates_fix_perf_and_areas(self):
        raw = "\n".join(
            [
                "@@COMMIT@@a\t2026-01-01T00:00:00Z\tfix(provider): repair stream",
                "internal/provider/openai.go",
                "@@COMMIT@@b\t2026-01-02T00:00:00Z\tfeat(desktop): add panel",
                "desktop/frontend/src/App.tsx",
            ]
        )
        commits = audit.parse_commit_log(raw)
        self.assertEqual(len(commits), 2)
        self.assertTrue(commits[0]["fix_perf"])
        self.assertIn("provider-cache", commits[0]["areas"])
        self.assertFalse(commits[1]["fix_perf"])
        self.assertIn("desktop-ui-accessibility", commits[1]["areas"])
        self.assertIsNone(audit.FIX_PERF_RE.match("fix(provider) missing colon"))

    def test_tiny_repo_inventory_is_deterministic_and_complete(self):
        with tempfile.TemporaryDirectory() as tmp:
            repo = Path(tmp)
            subprocess.run(["git", "init", "-q", str(repo)], check=True)
            subprocess.run(["git", "-C", str(repo), "config", "user.email", "audit@example.invalid"], check=True)
            subprocess.run(["git", "-C", str(repo), "config", "user.name", "Audit"], check=True)
            (repo / "README.md").write_text("base\n", encoding="utf-8")
            subprocess.run(["git", "-C", str(repo), "add", "README.md"], check=True)
            subprocess.run(["git", "-C", str(repo), "commit", "-q", "-m", "base"], check=True)
            baseline = subprocess.check_output(["git", "-C", str(repo), "rev-parse", "HEAD"], text=True).strip()
            (repo / "provider.go").write_text("package provider\n", encoding="utf-8")
            subprocess.run(["git", "-C", str(repo), "add", "provider.go"], check=True)
            subprocess.run(["git", "-C", str(repo), "commit", "-q", "-m", "fix(provider): add guard"], check=True)
            reviewed = subprocess.check_output(["git", "-C", str(repo), "rev-parse", "HEAD"], text=True).strip()

            payload = audit.build_inventory(repo, baseline, reviewed)
            self.assertEqual(payload["counts"]["all_commits"], 1)
            self.assertEqual(payload["counts"]["non_merge_commits"], 1)
            self.assertEqual(payload["counts"]["fix_perf_commits"], 1)
            self.assertEqual(payload["commits"][0]["sha"], reviewed)
            json.dumps(payload, ensure_ascii=False)


if __name__ == "__main__":
    unittest.main()
