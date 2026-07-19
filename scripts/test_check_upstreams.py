import json
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

    def test_primary_review_areas_require_record(self):
        with self.assertRaisesRegex(ValueError, "review_record"):
            watch.validate_manifest(
                {
                    "upstreams": [
                        {
                            "id": "reasonix",
                            "name": "Reasonix",
                            "repo": "https://github.com/esengine/DeepSeek-Reasonix.git",
                            "branch": "main-v2",
                            "required_review_areas": ["agent-runtime"],
                        }
                    ]
                }
            )

    def test_primary_review_areas_require_kebab_case(self):
        with self.assertRaisesRegex(ValueError, "kebab-case"):
            watch.validate_manifest(
                {
                    "upstreams": [
                        {
                            "id": "reasonix",
                            "name": "Reasonix",
                            "repo": "https://github.com/esengine/DeepSeek-Reasonix.git",
                            "branch": "main-v2",
                            "required_review_areas": ["Agent Runtime"],
                            "review_record": "docs/upstreams/reviews/reasonix-current.json",
                        }
                    ]
                }
            )

    def test_repository_manifest_tracks_official_grok_build(self):
        manifest = json.loads(watch.DEFAULT_MANIFEST.read_text(encoding="utf-8-sig"))
        grok = next(up for up in manifest["upstreams"] if up["id"] == "grok-build")

        self.assertEqual("https://github.com/xai-org/grok-build.git", grok["repo"])
        self.assertEqual("main", grok["branch"])
        self.assertEqual("security-interaction-reference", grok["importance"])
        self.assertEqual(r"F:\code-reference\Grok-Build", grok["local_reference"])
        self.assertTrue(grok["diff"])

    def test_code_mechanism_references_keep_path_level_diffs(self):
        manifest = json.loads(watch.DEFAULT_MANIFEST.read_text(encoding="utf-8-sig"))
        by_id = {up["id"]: up for up in manifest["upstreams"]}

        for upstream_id in ("reasonix", "hermes", "codex", "mimo", "scream-code", "agentark", "claude-code", "kimi-code", "grok-build"):
            self.assertTrue(by_id[upstream_id]["diff"], upstream_id)

    def test_codex_and_claude_are_strategic_code_upstreams(self):
        manifest = json.loads(watch.DEFAULT_MANIFEST.read_text(encoding="utf-8-sig"))
        by_id = {up["id"]: up for up in manifest["upstreams"]}

        for upstream_id in ("codex", "claude-code"):
            self.assertEqual("strategic-code-upstream", by_id[upstream_id]["importance"])
            self.assertTrue(by_id[upstream_id]["diff"])


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

    @mock.patch.object(watch, "diff_changed_files")
    @mock.patch.object(watch, "git_ls_remote")
    def test_strategic_code_upstream_always_requires_code_review(self, ls_remote, changed_files):
        ls_remote.return_value = {"refs/heads/main": "c" * 40}
        changed_files.return_value = [{"status": "M", "path": "CHANGELOG.md"}]
        upstream = {
            **self.upstream,
            "id": "codex",
            "name": "OpenAI Codex",
            "branch": "main",
            "importance": "strategic-code-upstream",
        }

        result = watch.analyze_upstream(upstream, self.lock)

        self.assertEqual("review-required", result["decision"])
        self.assertIn("code-level capability review", result["recommendation"])

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
            "c" * 40,
        )
        self.assertEqual(accepted["baseline"], "a" * 40)
        self.assertEqual(accepted["reviewed"], "c" * 40)

    def test_accept_revision_requires_full_sha(self):
        with self.assertRaisesRegex(ValueError, "FULL_40_CHARACTER_GIT_SHA"):
            watch.parse_accept_revisions(["codex=abc123"])

    def test_accept_revision_rejects_conflicting_duplicates(self):
        with self.assertRaisesRegex(ValueError, "conflicting accepted revisions"):
            watch.parse_accept_revisions([f"codex={'a' * 40}", f"codex={'b' * 40}"])

    def test_accept_revision_normalizes_sha(self):
        self.assertEqual(
            {"codex": "a" * 40},
            watch.parse_accept_revisions([f"codex={'A' * 40}"]),
        )

    def test_unbound_acceptance_options_are_disabled(self):
        legacy_options = [(["codex"], False, False), ([], True, False), ([], False, True)]
        for legacy_ids, accept_all, update_lock in legacy_options:
            with self.subTest(legacy_ids=legacy_ids, accept_all=accept_all, update_lock=update_lock):
                with self.assertRaisesRegex(ValueError, "unbound upstream acceptance is disabled"):
                    watch.acceptance_revisions(legacy_ids, accept_all, update_lock, [])

    def test_accept_rejects_remote_head_different_from_reviewed_sha(self):
        with self.assertRaisesRegex(ValueError, "does not match reviewed revision"):
            watch.accepted_lock_entry(
                {"id": "codex", "branch": "main", "latest": "b" * 40},
                self.lock,
                "c" * 40,
            )

    def test_primary_base_without_complete_coverage_stays_review_required(self):
        upstream = {
            **self.upstream,
            "required_review_areas": ["agent-runtime", "work-modes"],
            "review_record": "missing-review.json",
        }
        with mock.patch.object(watch, "git_ls_remote", return_value={"refs/heads/main-v2": "b" * 40}):
            result = watch.analyze_upstream(upstream, self.lock)
        self.assertFalse(result["changed"])
        self.assertEqual(result["decision"], "review-required")
        self.assertEqual(result["coverage"]["status"], "incomplete")

    def test_review_coverage_requires_every_area_and_matching_revision(self):
        with tempfile.TemporaryDirectory(dir=watch.ROOT) as tmp:
            record = Path(tmp) / "review.json"
            record.write_text(
                '{"upstream":"reasonix","baseline":"base","reviewed":"head",'
                '"areas":{"agent-runtime":{"status":"complete","evidence":["runtime diff reviewed"]},'
                '"work-modes":{"status":"pending"}}}',
                encoding="utf-8",
            )
            relative = record.relative_to(watch.ROOT).as_posix()
            up = {
                "id": "reasonix",
                "required_review_areas": ["agent-runtime", "work-modes"],
                "review_record": relative,
            }
            incomplete = watch.review_coverage(up, "base", "head")
            self.assertEqual(incomplete["missing"], ["work-modes"])
            payload = record.read_text(encoding="utf-8").replace('"pending"', '"complete"')
            record.write_text(payload, encoding="utf-8")
            missing_evidence = watch.review_coverage(up, "base", "head")
            self.assertEqual(missing_evidence["missing"], ["work-modes"])
            self.assertIn("require non-empty evidence", missing_evidence["error"])
            record.write_text(
                payload.replace(
                    '"work-modes":{"status":"complete"}',
                    '"work-modes":{"status":"complete","evidence":["mode commits and tests reviewed"]}',
                ),
                encoding="utf-8",
            )
            complete = watch.review_coverage(up, "base", "head")
            self.assertEqual(complete["status"], "complete")

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
