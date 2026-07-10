from __future__ import annotations

import hashlib
import subprocess
import tempfile
import unittest
from pathlib import Path
from unittest import mock

from scripts import smoke_desktop_native as smoke


class FakeClock:
    def __init__(self) -> None:
        self.now = 0.0

    def monotonic(self) -> float:
        return self.now

    def sleep(self, seconds: float) -> None:
        self.now += seconds


class FakeProcess:
    def __init__(self, exit_after_polls: int | None = None, exit_code: int = 0) -> None:
        self.pid = 1234
        self.exit_after_polls = exit_after_polls
        self.exit_code = exit_code
        self.polls = 0

    def poll(self) -> int | None:
        self.polls += 1
        if self.exit_after_polls is not None and self.polls >= self.exit_after_polls:
            return self.exit_code
        return None


class DesktopNativeSmokeTests(unittest.TestCase):
    def test_evidence_schema_contains_required_fields(self) -> None:
        evidence = smoke.SmokeResult().to_dict()
        for field in (
            "schema_version",
            "platform",
            "artifact_sha256",
            "executable_sha256",
            "observation_seconds",
            "responding",
            "cleanup_method",
            "cleanup_ok",
            "temp_cleaned",
            "boundary_changes",
        ):
            self.assertIn(field, evidence)
        self.assertEqual(evidence["schema_version"], smoke.SCHEMA_VERSION)

    def test_sha256_file(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            path = Path(raw) / "desktop.exe"
            path.write_bytes(b"desktop candidate")
            self.assertEqual(
                smoke.sha256_file(path),
                hashlib.sha256(b"desktop candidate").hexdigest().upper(),
            )

    def test_missing_artifact_is_startup_failure_on_windows(self) -> None:
        if smoke.sys.platform != "win32":
            self.skipTest("Windows-only native smoke")
        with tempfile.TemporaryDirectory() as raw:
            exe = Path(raw) / "desktop.exe"
            exe.write_bytes(b"candidate")
            result = smoke.run_smoke(str(exe), artifact_path="missing-installer.exe")
        self.assertEqual(result.failure_kind, "startup-failure")
        self.assertIn("artifact not found", result.errors[0])

    def test_observation_duration_contract(self) -> None:
        self.assertEqual(smoke.validate_observation_seconds(12), 12)
        for invalid in (0, 9, 301):
            with self.assertRaises(ValueError):
                smoke.validate_observation_seconds(invalid)

    def test_managed_home_cleans_by_default(self) -> None:
        with tempfile.TemporaryDirectory() as parent:
            with smoke.managed_smoke_home(False, Path(parent)) as home:
                (home / "desktop-tabs.json").write_text("{}", encoding="utf-8")
                saved = home
            self.assertFalse(saved.exists())

    def test_managed_home_keep_temp(self) -> None:
        with tempfile.TemporaryDirectory() as parent:
            with smoke.managed_smoke_home(True, Path(parent)) as home:
                saved = home
            self.assertTrue(saved.exists())

    def test_remove_tree_retries_transient_webview_lock(self) -> None:
        path = Path("locked-webview-fixture")
        attempts: list[Path] = []
        sleeps: list[float] = []

        def remove(candidate: Path) -> None:
            attempts.append(candidate)
            if len(attempts) < 3:
                raise PermissionError("WebView2 still closing")

        with mock.patch.object(smoke.shutil, "rmtree", side_effect=remove):
            smoke.remove_tree_with_retries(
                path, retry_seconds=(0.1, 0.2), sleeper=sleeps.append
            )

        self.assertEqual(attempts, [path, path, path])
        self.assertEqual(sleeps, [0.1, 0.2])

    def test_prepare_smoke_home_disables_updates_and_quits_on_close(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            home = Path(raw) / "home"
            smoke.prepare_smoke_home(home)
            config = (home / "config.toml").read_text(encoding="utf-8")
            self.assertIn('close_behavior = "quit"', config)
            self.assertIn("check_updates = false", config)
            self.assertIn("onboarding_dismissed = true", config)
            self.assertIn('language = "en"', config)
            self.assertNotIn("key", config.lower())

    def test_boundary_roots_include_default_webview2_store(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            old_appdata = smoke.os.environ.get("APPDATA")
            old_localappdata = smoke.os.environ.get("LOCALAPPDATA")
            try:
                smoke.os.environ["APPDATA"] = str(Path(raw) / "roaming")
                smoke.os.environ["LOCALAPPDATA"] = str(Path(raw) / "local")
                roots = smoke.default_boundary_roots(
                    Path(raw) / "isolated", "candidate.exe"
                )
            finally:
                if old_appdata is None:
                    smoke.os.environ.pop("APPDATA", None)
                else:
                    smoke.os.environ["APPDATA"] = old_appdata
                if old_localappdata is None:
                    smoke.os.environ.pop("LOCALAPPDATA", None)
                else:
                    smoke.os.environ["LOCALAPPDATA"] = old_localappdata
            self.assertEqual(
                roots["WEBVIEW2"], (Path(raw) / "roaming" / "candidate.exe").resolve()
            )

    def test_snapshot_reports_only_metadata_changes(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            before = smoke.snapshot_roots({"APPDATA": root})
            (root / "desktop-tabs.json").write_text("{}", encoding="utf-8")
            after = smoke.snapshot_roots({"APPDATA": root})
            self.assertEqual(
                smoke.changed_snapshot(before, after),
                ["added <APPDATA>/desktop-tabs.json"],
            )

    def test_observe_process_requires_consecutive_and_final_response(self) -> None:
        clock = FakeClock()
        proc = FakeProcess()
        responses = iter([False, True, True, True, True, True])

        def responder(_pid: int) -> tuple[bool, int]:
            return next(responses, True), 1

        result = smoke.observe_process(
            proc,
            3,
            responder=responder,
            clock=clock.monotonic,
            sleeper=clock.sleep,
        )
        self.assertTrue(result.responding)
        self.assertGreaterEqual(result.max_consecutive_responses, 3)
        self.assertTrue(result.final_check_responsive)

    def test_observe_process_classifies_early_exit(self) -> None:
        clock = FakeClock()
        proc = FakeProcess(exit_after_polls=2, exit_code=17)
        result = smoke.observe_process(
            proc,
            3,
            responder=lambda _pid: (True, 1),
            clock=clock.monotonic,
            sleeper=clock.sleep,
        )
        self.assertEqual(result.early_exit_code, 17)
        self.assertFalse(result.responding)

    def test_missing_executable_is_startup_failure(self) -> None:
        result = smoke.run_smoke("definitely-missing.exe")
        expected = "unsupported-platform" if smoke.sys.platform != "win32" else "startup-failure"
        self.assertEqual(result.failure_kind, expected)
        self.assertEqual(result.outcome, "failed")

    def test_cleanup_process_accepts_already_exited_process(self) -> None:
        class ExitedProcess:
            def poll(self) -> int:
                return 0

        errors: list[str] = []
        ok, method = smoke.cleanup_process(ExitedProcess(), errors)  # type: ignore[arg-type]
        self.assertTrue(ok)
        self.assertEqual(method, "already-exited")
        self.assertEqual(errors, [])

    def test_cleanup_process_records_graceful_window_close(self) -> None:
        class GracefulProcess:
            pid = 1234

            def __init__(self) -> None:
                self.exited = False

            def poll(self) -> int | None:
                return 0 if self.exited else None

            def wait(self, timeout: int) -> int:
                self.exited = True
                return 0

        proc = GracefulProcess()
        errors: list[str] = []
        original = smoke.request_window_close
        smoke.request_window_close = lambda _pid: 1
        try:
            ok, method = smoke.cleanup_process(proc, errors)  # type: ignore[arg-type]
        finally:
            smoke.request_window_close = original
        self.assertTrue(ok)
        self.assertEqual(method, "wm-close")
        self.assertEqual(errors, [])

    def test_cleanup_process_records_terminate_fallback(self) -> None:
        class TerminatedProcess:
            pid = 1234

            def __init__(self) -> None:
                self.terminated = False
                self.waits = 0

            def poll(self) -> int | None:
                return 1 if self.terminated else None

            def wait(self, timeout: int) -> int:
                self.waits += 1
                if not self.terminated:
                    raise subprocess.TimeoutExpired("desktop.exe", timeout)
                return 1

            def terminate(self) -> None:
                self.terminated = True

        proc = TerminatedProcess()
        errors: list[str] = []
        original = smoke.request_window_close
        smoke.request_window_close = lambda _pid: 1
        try:
            ok, method = smoke.cleanup_process(proc, errors)  # type: ignore[arg-type]
        finally:
            smoke.request_window_close = original
        self.assertTrue(ok)
        self.assertEqual(method, "terminate")
        self.assertEqual(proc.waits, 2)
        self.assertEqual(errors, [])


if __name__ == "__main__":
    unittest.main()
