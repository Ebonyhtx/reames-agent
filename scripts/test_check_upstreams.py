import subprocess
import unittest
from unittest import mock

from scripts import check_upstreams as watch


class ManifestValidationTests(unittest.TestCase):
    def test_accepts_official_github_repositories(self):
        watch.validate_manifest(
            {
                "upstreams": [
                    {
                        "id": "reasonix",
                        "name": "Reasonix",
                        "repo": "https://github.com/esengine/DeepSeek-Reasonix.git",
                        "branch": "main-v2",
                    }
                ]
            }
        )

    def test_rejects_duplicate_ids(self):
        upstream = {
            "id": "same",
            "name": "One",
            "repo": "https://github.com/example/one.git",
            "branch": "main",
        }
        with self.assertRaisesRegex(ValueError, "duplicate upstream id"):
            watch.validate_manifest({"upstreams": [upstream, {**upstream, "name": "Two"}]})

    def test_rejects_non_github_repository(self):
        with self.assertRaisesRegex(ValueError, "official HTTPS GitHub"):
            watch.validate_manifest(
                {
                    "upstreams": [
                        {
                            "id": "bad",
                            "name": "Bad",
                            "repo": "https://git.example.com/owner/repo.git",
                            "branch": "main",
                        }
                    ]
                }
            )


class AnalysisTests(unittest.TestCase):
    def setUp(self):
        self.upstream = {
            "id": "reasonix",
            "name": "Reasonix",
            "repo": "https://github.com/esengine/DeepSeek-Reasonix.git",
            "branch": "main-v2",
            "importance": "primary-base",
            "policy": "advisory-report",
            "tag_patterns": [],
            "diff": True,
        }
        self.lock = {"baseline": "a" * 40, "reviewed": "b" * 40}

    @mock.patch.object(watch, "diff_changed_files")
    @mock.patch.object(watch, "git_ls_remote")
    def test_primary_security_change_requires_review(self, ls_remote, changed_files):
        ls_remote.return_value = {"refs/heads/main-v2": "c" * 40}
        changed_files.return_value = [{"status": "A", "path": "internal/secrets/redact.go"}]

        result = watch.analyze_upstream(self.upstream, self.lock)

        self.assertTrue(result["changed"])
        self.assertEqual(result["risk"], "high")
        self.assertEqual(result["decision"], "review-required")
        self.assertEqual(result["baseline"], "a" * 40)
        self.assertEqual(result["reviewed"], "b" * 40)

    @mock.patch.object(watch, "git_ls_remote", side_effect=subprocess.CalledProcessError(2, ["git"]))
    def test_one_failed_upstream_becomes_report_data(self, _ls_remote):
        result = watch.analyze_upstream(self.upstream, self.lock)

        self.assertEqual(result["decision"], "check-failed")
        self.assertEqual(result["risk"], "unknown")
        self.assertTrue(result["error"])

    def test_accept_preserves_source_baseline(self):
        accepted = watch.accepted_lock_entry(
            {"id": "reasonix", "branch": "main-v2", "latest": "c" * 40},
            self.lock,
        )
        self.assertEqual(accepted["baseline"], "a" * 40)
        self.assertEqual(accepted["reviewed"], "c" * 40)

    def test_report_fingerprint_ignores_generation_time(self):
        upstreams = [
            {
                "id": "reasonix",
                "reviewed": "a",
                "latest": "b",
                "decision": "review-required",
                "error": "",
            }
        ]
        self.assertEqual(watch.report_fingerprint(upstreams), watch.report_fingerprint(upstreams))
        changed = [{**upstreams[0], "latest": "c"}]
        self.assertNotEqual(watch.report_fingerprint(upstreams), watch.report_fingerprint(changed))


if __name__ == "__main__":
    unittest.main()
