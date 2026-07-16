import subprocess
import tempfile
import unittest
from pathlib import Path
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

    @mock.patch.object(watch, "comparison_evidence")
    @mock.patch.object(watch, "git_ls_remote")
    def test_deep_analysis_is_attached(self, ls_remote, comparison):
        ls_remote.return_value = {"refs/heads/main-v2": "c" * 40}
        comparison.return_value = ([], {"commits": "abc change", "diff": "+change"})
        result = watch.analyze_upstream(self.upstream, self.lock, deep=True)
        self.assertEqual(result["deep"]["commits"], "abc change")
        comparison.assert_called_once_with(self.upstream["repo"], "main-v2", "b" * 40, "c" * 40, deep=True)

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

    def test_gateway_service_paths_are_gateway_area(self):
        self.assertEqual(watch.classify_path("internal/gatewayservice/service.go"), "gateway")

    @mock.patch.object(watch.subprocess, "run")
    def test_command_output_is_decoded_as_utf8(self, run):
        run.return_value = subprocess.CompletedProcess([], 0, stdout="中英提交\n", stderr="")

        result = watch.run(["git", "log"])

        self.assertEqual("中英提交\n", result.stdout)
        self.assertEqual("utf-8", run.call_args.kwargs["encoding"])
        self.assertEqual("replace", run.call_args.kwargs["errors"])

    @mock.patch.object(watch, "fetch_comparison_refs", return_value="")
    @mock.patch.object(watch, "run")
    def test_deep_diff_collects_bounded_evidence(self, run, _fetch):
        run.side_effect = [
            subprocess.CompletedProcess([], 0, stdout="M\ta.txt\n", stderr=""),
            subprocess.CompletedProcess([], 0, stdout="abc change\n", stderr=""),
            subprocess.CompletedProcess([], 0, stdout="diff --git a/a b/a\n+x\n", stderr=""),
        ]
        result = watch.deep_diff_content("https://github.com/example/repo.git", "main", "a" * 40, "b" * 40)
        self.assertEqual(result["commits"], "abc change")
        self.assertIn("diff --git", result["diff"])

    @mock.patch.object(watch, "run")
    def test_fetch_comparison_refs_fetches_missing_base_sha(self, run):
        ok = lambda: subprocess.CompletedProcess([], 0, stdout="", stderr="")
        missing = subprocess.CompletedProcess([], 1, stdout="", stderr="missing")
        run.side_effect = [ok(), ok(), ok(), missing, ok(), ok()]
        error = watch.fetch_comparison_refs(Path("tmp"), "https://github.com/example/repo.git", "main", "a" * 40, "b" * 40)
        self.assertEqual("", error)
        self.assertEqual(["git", "fetch", "--no-tags", "--depth=1", "origin", "a" * 40], run.call_args_list[4].args[0])

    def test_deep_diff_against_local_git_repository(self):
        with tempfile.TemporaryDirectory() as tmp:
            repo = Path(tmp) / "source"
            repo.mkdir()

            def git(*args):
                return subprocess.run(
                    ["git", *args], cwd=repo, check=True, text=True,
                    stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                ).stdout.strip()

            git("init", "-q", "-b", "main")
            git("config", "user.email", "test@example.invalid")
            git("config", "user.name", "Upstream Test")
            (repo / "sample.txt").write_text("base\n", encoding="utf-8")
            git("add", "sample.txt")
            git("commit", "-q", "-m", "base")
            base = git("rev-parse", "HEAD")
            (repo / "sample.txt").write_text("base\nhead\n", encoding="utf-8")
            git("commit", "-qam", "head change")
            head = git("rev-parse", "HEAD")

            result = watch.deep_diff_content(str(repo), "main", base, head)

        self.assertIn("head change", result.get("commits", ""))
        self.assertIn("+head", result.get("diff", ""))


if __name__ == "__main__":
    unittest.main()
