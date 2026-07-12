from __future__ import annotations

import hashlib
import os
import tempfile
import unittest
from pathlib import Path

from scripts import smoke_desktop_candidate as smoke


class FakeClock:
    def __init__(self) -> None:
        self.now = 0.0

    def monotonic(self) -> float:
        return self.now

    def sleep(self, seconds: float) -> None:
        self.now += seconds


class FakeProcess:
    pid = 1234

    def __init__(self, exit_after: int | None = None) -> None:
        self.exit_after = exit_after
        self.polls = 0

    def poll(self) -> int | None:
        self.polls += 1
        if self.exit_after is not None and self.polls >= self.exit_after:
            return 17
        return None


class CandidateSmokeTests(unittest.TestCase):
    def test_evidence_schema_contains_install_and_boundary_fields(self) -> None:
        evidence = smoke.CandidateSmokeResult().to_dict()
        for field in (
            "artifact_sha256",
            "executable_sha256",
            "startup_budget_seconds",
            "first_state_ready_seconds",
            "first_visible_seconds",
            "stable_ready_seconds",
            "startup_budget_met",
            "ready",
            "readiness_kind",
            "window_required",
            "window_observed",
            "cleanup_method",
            "temp_cleaned",
            "boundary_changes",
        ):
            self.assertIn(field, evidence)

    def test_sha256_file(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            path = Path(raw) / "candidate"
            path.write_bytes(b"native candidate")
            self.assertEqual(
                smoke.sha256_file(path),
                hashlib.sha256(b"native candidate").hexdigest().upper(),
            )

    def test_prepare_home_disables_updates_and_quits(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            home = Path(raw) / "home"
            smoke.prepare_smoke_home(home)
            config = (home / "config.toml").read_text(encoding="utf-8")
            self.assertIn('close_behavior = "quit"', config)
            self.assertIn("check_updates = false", config)
            self.assertIn("onboarding_dismissed = true", config)
            self.assertIn('language = "en"', config)
            self.assertNotIn("key", config.lower())
            self.assertFalse(smoke.desktop_state_ready(home))
            (home / "desktop-tabs.json").write_text("{}", encoding="utf-8")
            self.assertTrue(smoke.desktop_state_ready(home))

    def test_snapshot_reports_metadata_only(self) -> None:
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            before = smoke.snapshot_roots({"DEFAULT_HOME": root})
            (root / "desktop-tabs.json").write_text("{}", encoding="utf-8")
            changes = smoke.changed_snapshot(
                before, smoke.snapshot_roots({"DEFAULT_HOME": root})
            )
            self.assertEqual(changes, ["added <DEFAULT_HOME>/desktop-tabs.json"])

    def test_boundary_roots_cover_home_cache_and_legacy_support(self) -> None:
        roots = smoke.default_boundary_roots("darwin")
        self.assertEqual(
            set(roots), {"DEFAULT_HOME", "DEFAULT_CACHE", "LEGACY_SUPPORT"}
        )
        self.assertEqual(
            roots["DEFAULT_CACHE"].parts[-3:],
            ("Library", "Caches", "reames-agent"),
        )

        old_cache = os.environ.get("XDG_CACHE_HOME")
        old_config = os.environ.get("XDG_CONFIG_HOME")
        try:
            os.environ["XDG_CACHE_HOME"] = "/tmp/reames-cache"
            os.environ["XDG_CONFIG_HOME"] = "/tmp/reames-config"
            linux = smoke.default_boundary_roots("linux")
        finally:
            if old_cache is None:
                os.environ.pop("XDG_CACHE_HOME", None)
            else:
                os.environ["XDG_CACHE_HOME"] = old_cache
            if old_config is None:
                os.environ.pop("XDG_CONFIG_HOME", None)
            else:
                os.environ["XDG_CONFIG_HOME"] = old_config
        self.assertEqual(linux["DEFAULT_CACHE"].parts[-2:], ("reames-cache", "reames-agent"))
        self.assertEqual(linux["LEGACY_SUPPORT"].parts[-2:], ("reames-config", "reames-agent"))

    def test_observe_linux_requires_repeated_visible_window(self) -> None:
        clock = FakeClock()
        process = FakeProcess()
        observations = iter([[], ["10"], ["10"], ["10"], ["10"]])
        result = smoke.observe_process(
            process,
            3,
            window_probe=lambda _pid: next(observations, ["10"]),
            clock=clock.monotonic,
            sleeper=clock.sleep,
        )
        self.assertIsNone(result.early_exit_code)
        self.assertGreaterEqual(result.window_checks, smoke.REQUIRED_WINDOW_CHECKS)
        self.assertEqual(result.max_visible_windows, 1)
        self.assertTrue(result.ready)

    def test_observe_records_state_window_and_stable_readiness(self) -> None:
        clock = FakeClock()
        states = iter([False, False, True, True, True, True])
        windows = iter([[], ["10"], ["10"], ["10"], ["10"], ["10"]])
        result = smoke.observe_process(
            FakeProcess(),
            3,
            window_probe=lambda _pid: next(windows, ["10"]),
            state_probe=lambda: next(states, True),
            clock=clock.monotonic,
            sleeper=clock.sleep,
        )
        self.assertEqual(result.first_visible_seconds, 0.5)
        self.assertEqual(result.first_state_ready_seconds, 1.0)
        self.assertEqual(result.stable_ready_seconds, 2.0)
        self.assertTrue(result.ready)

    def test_observe_resets_readiness_after_window_disappears(self) -> None:
        clock = FakeClock()
        windows = iter([["10"], ["10"], [], ["10"], ["10"], ["10"]])
        result = smoke.observe_process(
            FakeProcess(),
            3,
            window_probe=lambda _pid: next(windows, ["10"]),
            state_probe=lambda: True,
            clock=clock.monotonic,
            sleeper=clock.sleep,
        )
        self.assertEqual(result.stable_ready_seconds, 2.5)
        self.assertEqual(result.max_consecutive_ready_checks, 3)
        self.assertTrue(result.ready)

    def test_observe_macos_state_only_readiness(self) -> None:
        clock = FakeClock()
        states = iter([False, True, True, True, True])
        result = smoke.observe_process(
            FakeProcess(),
            3,
            state_probe=lambda: next(states, True),
            clock=clock.monotonic,
            sleeper=clock.sleep,
        )
        self.assertEqual(result.first_state_ready_seconds, 0.5)
        self.assertIsNone(result.first_visible_seconds)
        self.assertEqual(result.stable_ready_seconds, 1.5)
        self.assertTrue(result.ready)

    def test_observe_classifies_early_exit(self) -> None:
        clock = FakeClock()
        result = smoke.observe_process(
            FakeProcess(exit_after=2),
            3,
            None,
            clock=clock.monotonic,
            sleeper=clock.sleep,
        )
        self.assertEqual(result.early_exit_code, 17)

    def test_validation_bounds(self) -> None:
        self.assertEqual(smoke.validate_observation_seconds(12), 12)
        for invalid in (0, 9, 301):
            with self.assertRaises(ValueError):
                smoke.validate_observation_seconds(invalid)
        self.assertEqual(smoke.validate_startup_budget_seconds(10), 10)
        for invalid in (0, 0.5, 61):
            with self.assertRaises(ValueError):
                smoke.validate_startup_budget_seconds(invalid)

    def test_classification_requires_stable_readiness_and_budget(self) -> None:
        ready = smoke.Observation(
            final_ready=True,
            max_consecutive_ready_checks=3,
            stable_ready_seconds=2.0,
        )
        self.assertIsNone(smoke.classify_startup_observation(ready, 10, "darwin"))
        self.assertEqual(
            smoke.classify_startup_observation(ready, 1, "darwin")[0],
            "startup-budget",
        )
        self.assertEqual(
            smoke.classify_startup_observation(smoke.Observation(), 10, "linux")[0],
            "startup-not-ready",
        )

    def test_platform_mismatch_fails_before_reading_inputs(self) -> None:
        requested = "darwin" if smoke.host_platform() != "darwin" else "linux"
        result = smoke.run_smoke("missing-artifact", "missing-exe", requested)
        self.assertEqual(result.failure_kind, "platform-mismatch")

    def test_cleanup_accepts_already_exited_process(self) -> None:
        class ExitedProcess:
            def poll(self) -> int:
                return 0

        ok, method = smoke.cleanup_process(ExitedProcess(), "darwin", [])  # type: ignore[arg-type]
        self.assertTrue(ok)
        self.assertEqual(method, "already-exited")


if __name__ == "__main__":
    unittest.main()
