"""Deterministic safety contracts; full PostgreSQL/Restic restore is tested separately."""

import contextlib
import datetime as dt
import hashlib
import io
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest import mock

import runner


SNAPSHOT = "a" * 64
REPOSITORY = "b" * 64


class RunnerTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.env = {
            "BACKUP_STATE_DIR": str(self.root / "state"),
            "BACKUP_CACHE_DIR": str(self.root / "cache"),
            "BACKUP_SOURCE_DIR": str(self.root / "source"),
            "BACKUP_REPOSITORY": str(self.root / "repository"),
            "BACKUP_PASSWORD_FILE": str(self.root / "password"),
            "BACKUP_DB_PASSWORD": "database-test-password",
            "BACKUP_MIN_FREE_MB": "1",
            "BACKUP_RETRIES": "0",
        }
        (self.root / "password").write_text("repository-test-password\n")
        for name in ("data", "logs", "config"):
            directory = self.root / "source" / name
            directory.mkdir(parents=True)
            (directory / "fixture.txt").write_text(name)
        self.config = runner.Config(self.env)
        self.config.prepare()
        runner.STOPPING = False

    def fake_command(self, stage, args, env=None, monitor=True):
        if args[:2] == ["pg_dump", "--version"]:
            return "pg_dump (PostgreSQL) 15.20\n"
        if args[:2] == ["restic", "version"]:
            return "restic 0.18.1 compiled with go\n"
        if args[:2] == ["rclone", "version"]:
            return "rclone v1.71.2\n"
        if args[0] == "restic" and args[-2:] == ["cat", "config"]:
            return json.dumps({"version": 2, "id": REPOSITORY})
        if args[0] == "psql":
            return "150020\n"
        if args[0] == "pg_dump":
            Path(args[args.index("--file") + 1]).write_bytes(b"custom-dump-fixture")
            return ""
        if args[0] == "pg_restore":
            self.assertTrue(Path(args[-1]).is_file())
            return "; readable TOC\n"
        self.fail(f"Unexpected fixture command: {stage}")

    def fake_restic(self, stage, args):
        if args == ["cat", "config"]:
            return json.dumps({"version": 2, "id": REPOSITORY})
        if args[0] == "backup":
            return json.dumps({"message_type": "summary", "snapshot_id": SNAPSHOT[:8]})
        if args[0] == "snapshots":
            return json.dumps([{"id": SNAPSHOT, "hostname": self.config.host, "tags": [self.config.tag]}])
        self.fail(f"Unexpected fixture repository command: {stage}")

    def test_config_rejects_overlapping_writable_sources_and_invalid_limits(self):
        cases = [
            {"BACKUP_STATE_DIR": str(self.config.source / "state")},
            {"BACKUP_CACHE_DIR": str(self.config.state / "cache")},
            {"BACKUP_TIMEOUT_SECONDS": "0"},
            {"BACKUP_RETRIES": "100"},
            {"BACKUP_TIME": "25:00"},
            {"TZ": "Unknown/Zone"},
            {"BACKUP_TAG": "one,two"},
        ]
        for override in cases:
            with self.subTest(override=override), self.assertRaises(runner.BackupError):
                runner.Config(self.env | override)

    def test_repository_rejects_source_and_bucket_root(self):
        for repository in (str(self.config.source / "repo"), "rclone:oss:bucket", "https://user:secret@example.com/repo"):
            with self.subTest(repository=repository), self.assertRaises(runner.BackupError):
                runner.Config(self.env | {"BACKUP_REPOSITORY": repository}).repository_env()

    def test_oss_secret_files_feed_rclone_without_becoming_arguments_or_status(self):
        access_file = self.root / "oss-access"
        key_file = self.root / "oss-secret"
        access_file.write_text("test-access\n")
        key_file.write_text("test-secret\n")
        env = self.env | {
            "BACKUP_REPOSITORY": "rclone:oss:backup-bucket/new-api",
            "BACKUP_OSS_ENDPOINT": "oss-cn-hangzhou.aliyuncs.com",
            "BACKUP_OSS_ACCESS_KEY_ID_FILE": str(access_file),
            "BACKUP_OSS_ACCESS_KEY_SECRET_FILE": str(key_file),
            "RCLONE_CONFIG_OSS_NO_CHECK_BUCKET": "false",
            "RESTIC_PASSWORD": "must-not-override-file",
        }
        config = runner.Config(env)
        result = config.repository_env()
        self.assertEqual(result["RCLONE_CONFIG_OSS_PROVIDER"], "Alibaba")
        self.assertEqual(result["RCLONE_CONFIG_OSS_ACCESS_KEY_ID"], "test-access")
        self.assertEqual(result["RCLONE_CONFIG_OSS_SECRET_ACCESS_KEY"], "test-secret")
        self.assertEqual(result["RCLONE_CONFIG_OSS_NO_CHECK_BUCKET"], "true")
        self.assertNotIn("RESTIC_PASSWORD", result)
        self.assertNotIn("test-secret", config.identity())

    def test_local_lock_prevents_overlapping_repository_operations(self):
        with runner.job_lock(self.config):
            with self.assertRaisesRegex(runner.BackupError, "Another backup"):
                with runner.job_lock(self.config):
                    self.fail("Second job must not acquire lock")
        with runner.job_lock(self.config):
            pass

    def test_restic_temporary_packs_use_guarded_cache_instead_of_small_tmpfs(self):
        config = runner.Config(self.env | {"TMPDIR": "/tmp"})
        env = config.repository_env()
        self.assertEqual(env["TMPDIR"], str(config.cache))
        self.assertEqual(env["RESTIC_CACHE_DIR"], str(config.cache))
        self.assertNotEqual(env["TMPDIR"], str(config.source))

    def test_backup_requires_recovery_configuration(self):
        for path in (self.config.source / "config").iterdir():
            path.unlink()
        with self.assertRaisesRegex(runner.BackupError, "config is empty"):
            runner.Job(self.config).backup()

    def test_database_names_cannot_override_connection_host(self):
        for name in ("host=other dbname=new-api", "postgresql://other/new-api", "new-api\nother"):
            config = runner.Config(self.env | {"BACKUP_DATABASES": name})
            with self.subTest(name=name), mock.patch.object(runner.Job, "command") as command, \
                    self.assertRaisesRegex(runner.BackupError, "not connection strings"):
                runner.Job(config).backup()
            command.assert_not_called()

    def test_stale_owned_dumps_are_cleaned_before_space_check_without_following_symlinks(self):
        stale = self.config.state / "job-stale"
        stale.mkdir()
        (stale / ".backup-owned").touch()
        (stale / "partial.dump").write_bytes(b"partial")
        unowned = self.config.state / "job-unowned"
        unowned.mkdir()
        source_link = self.config.state / "job-source-link"
        source_link.symlink_to(self.config.source, target_is_directory=True)

        def guard():
            self.assertFalse(stale.exists(), "Cleanup must happen before a low-disk guard")
            raise runner.BackupError("Simulated low disk")

        with mock.patch.object(runner.Job, "space_guard", side_effect=guard), self.assertRaisesRegex(runner.BackupError, "low disk"):
            runner.Job(self.config).backup()
        self.assertTrue(unowned.is_dir())
        self.assertTrue((self.config.source / "data/fixture.txt").is_file())
        self.assertTrue(source_link.is_symlink())

    def test_backup_validates_toc_manifest_and_snapshot_before_success(self):
        captured = {}

        def restic(stage, args):
            if args[0] == "backup":
                work = Path(args[-2])
                captured.update(json.loads((work / "manifest.json").read_text()))
            return self.fake_restic(stage, args)

        with mock.patch.object(runner.Job, "command", side_effect=self.fake_command), \
                mock.patch.object(runner.Job, "restic", side_effect=restic), contextlib.redirect_stdout(io.StringIO()):
            runner.perform_backup(self.config)
        status = runner.read_status(self.config)
        self.assertEqual(status["state"], "success")
        self.assertEqual(status["snapshot_id"], SNAPSHOT)
        self.assertEqual(captured["consistency"], "online-uncoordinated")
        self.assertEqual(captured["databases"][0]["sha256"], hashlib.sha256(b"custom-dump-fixture").hexdigest())
        self.assertEqual(list(self.config.state.glob("job-*")), [])
        self.assertEqual((self.config.source / "data/fixture.txt").read_text(), "data")
        self.assertNotIn("database-test-password", json.dumps(captured))

    def test_newer_server_never_produces_incompatible_dump(self):
        def command(stage, args, env=None, monitor=True):
            if args[0] == "psql":
                return "180006\n"
            return self.fake_command(stage, args, env, monitor)

        with mock.patch.object(runner.Job, "command", side_effect=command) as calls, \
                self.assertRaisesRegex(runner.BackupError, "server is newer"):
            runner.Job(self.config).backup()
        self.assertFalse(any(call.args[1][0] == "pg_dump" and "--file" in call.args[1] for call in calls.call_args_list))
        self.assertEqual(list(self.config.state.glob("job-*")), [])

    def test_unreadable_dump_fails_before_any_repository_snapshot(self):
        def command(stage, args, env=None, monitor=True):
            if args[0] == "pg_restore":
                raise runner.BackupError("PostgreSQL dump TOC check failed")
            return self.fake_command(stage, args, env, monitor)

        with mock.patch.object(runner.Job, "command", side_effect=command), \
                mock.patch.object(runner.Job, "restic", side_effect=self.fake_restic) as repository, \
                self.assertRaises(runner.BackupError):
            runner.perform_backup(self.config)
        self.assertFalse(any(call.args[1][0] in ("init", "backup") for call in repository.call_args_list))
        self.assertEqual(runner.read_status(self.config)["state"], "failed")
        self.assertEqual(list(self.config.state.glob("job-*")), [])

    def test_repository_failure_keeps_last_success_but_marks_health_failed(self):
        previous = {"state": "success", "started_at": runner.utcnow(), "success_at": runner.utcnow(),
                    "snapshot_id": SNAPSHOT, "identity": self.config.identity()}
        runner.write_status(self.config, previous)
        with mock.patch.object(runner.Job, "backup", side_effect=runner.BackupError("Encrypted repository backup failed")), \
                self.assertRaises(runner.BackupError):
            runner.perform_backup(self.config)
        status = runner.read_status(self.config)
        self.assertEqual(status["state"], "failed")
        self.assertEqual(status["snapshot_id"], SNAPSHOT)
        self.assertEqual(status["success_at"], previous["success_at"])
        self.assertFalse(runner.health(self.config))

    def test_wrong_snapshot_receipt_is_not_reported_successful(self):
        def restic(stage, args):
            if args[0] == "snapshots":
                return json.dumps([{"id": SNAPSHOT, "hostname": "another-host", "tags": [self.config.tag]}])
            return self.fake_restic(stage, args)

        with mock.patch.object(runner.Job, "command", side_effect=self.fake_command), \
                mock.patch.object(runner.Job, "restic", side_effect=restic), \
                self.assertRaisesRegex(runner.BackupError, "does not match"):
            runner.perform_backup(self.config)
        self.assertEqual(runner.read_status(self.config)["state"], "failed")

    def test_health_rejects_stale_or_different_repository_status(self):
        runner.write_status(self.config, {"state": "success", "started_at": runner.utcnow(),
                                         "success_at": runner.utcnow(), "identity": self.config.identity()})
        self.assertTrue(runner.health(self.config))
        other = runner.Config(self.env | {"BACKUP_HOST": "other-host"})
        self.assertFalse(runner.health(other))
        other_port = runner.Config(self.env | {"BACKUP_DB_PORT": "5433"})
        self.assertFalse(runner.health(other_port))
        other_endpoint = runner.Config(self.env | {"BACKUP_OSS_ENDPOINT": "other-endpoint.aliyuncs.com"})
        self.assertFalse(runner.health(other_endpoint))
        runner.write_status(self.config, {"state": "success", "started_at": "2020-01-01T00:00:00+00:00",
                                         "identity": self.config.identity()})
        self.assertFalse(runner.health(self.config))

    def test_daily_schedule_uses_timezone_and_normalizes_dst_gap(self):
        now = dt.datetime(2026, 11, 16, 0, 0, tzinfo=dt.timezone.utc)
        self.assertEqual(runner.next_run(now, self.config).isoformat(), "2026-11-17T03:30:00+08:00")
        dst = runner.Config(self.env | {"TZ": "America/New_York", "BACKUP_TIME": "02:30"})
        spring = dt.datetime(2026, 3, 8, 6, 0, tzinfo=dt.timezone.utc)
        self.assertEqual(runner.next_run(spring, dst).isoformat(), "2026-03-08T03:30:00-04:00")

    def test_daily_times_preserve_legacy_and_normalize_multiple_points(self):
        self.assertEqual(self.config.schedule_mode, "daily")
        self.assertEqual(self.config.times, (dt.time(3, 30),))
        self.assertEqual(self.config.interval, 0)
        legacy = runner.Config(self.env | {"BACKUP_TIME": "3:30", "BACKUP_TIMES": ""})
        self.assertEqual(legacy.times, (dt.time(3, 30),))
        legacy_minutes = runner.Config(self.env | {"BACKUP_TIME": "3:3"})
        self.assertEqual(legacy_minutes.times, (dt.time(3, 3),))
        multiple = runner.Config(self.env | {"BACKUP_TIME": "invalid-but-overridden",
                                             "BACKUP_TIMES": " 15:30,3:30,03:30,00:00 "})
        self.assertEqual(multiple.times, (dt.time(0), dt.time(3, 30), dt.time(15, 30)))

    def test_daily_schedule_rejects_bad_points_and_unknown_modes(self):
        cases = [
            {"BACKUP_SCHEDULE_MODE": "weekly"},
            {"BACKUP_TIMES": "03:30,"},
            {"BACKUP_TIMES": ",03:30"},
            {"BACKUP_TIMES": "03:30,,15:30"},
            {"BACKUP_TIMES": "24:00"},
            {"BACKUP_TIMES": "03:3"},
            {"BACKUP_TIMES": "03:60"},
            {"BACKUP_TIMES": "03:30:00"},
            {"BACKUP_TIMES": "03:30;15:30"},
        ]
        for override in cases:
            with self.subTest(override=override), self.assertRaises(runner.BackupError):
                runner.Config(self.env | override)

    def test_interval_units_bounds_and_inactive_daily_fields(self):
        cases = [("1m", 60), ("30m", 1800), ("6h", 21600), ("1d", 86400),
                 ("30d", 2592000), (" 2H ", 7200)]
        for value, seconds in cases:
            config = runner.Config(self.env | {"BACKUP_SCHEDULE_MODE": "interval", "BACKUP_INTERVAL": value,
                                               "BACKUP_TIME": "unused-invalid", "BACKUP_TIMES": "also-unused"})
            with self.subTest(value=value):
                self.assertEqual(config.schedule_mode, "interval")
                self.assertEqual(config.interval, seconds)
        # A setting for the inactive mode must not prevent the selected plan running.
        daily = runner.Config(self.env | {"BACKUP_SCHEDULE_MODE": "daily", "BACKUP_INTERVAL": "unused-invalid"})
        self.assertEqual(daily.times, (dt.time(3, 30),))

    def test_interval_rejects_missing_units_nonpositive_and_excessive_duration(self):
        for value in ("", "0m", "-1h", "1.5h", "60s", "3600", "1w", "1h30m", "31d"):
            with self.subTest(value=value), self.assertRaises(runner.BackupError):
                runner.Config(self.env | {"BACKUP_SCHEDULE_MODE": "interval", "BACKUP_INTERVAL": value})

    def test_daily_multiple_points_select_next_future_time_across_midnight(self):
        config = runner.Config(self.env | {"BACKUP_TIMES": "03:30,15:30,23:45"})
        cases = [
            ("2026-11-15T18:00:00+00:00", "2026-11-16T03:30:00+08:00"),
            ("2026-11-16T00:00:00+00:00", "2026-11-16T15:30:00+08:00"),
            ("2026-11-16T07:30:00+00:00", "2026-11-16T23:45:00+08:00"),
            ("2026-11-16T15:46:00+00:00", "2026-11-17T03:30:00+08:00"),
        ]
        for now, expected in cases:
            with self.subTest(now=now):
                self.assertEqual(runner.next_run(dt.datetime.fromisoformat(now), config).isoformat(), expected)
        midnight = runner.Config(self.env | {"BACKUP_TIMES": "00:00,12:00"})
        now = dt.datetime(2026, 11, 16, 15, 59, tzinfo=dt.timezone.utc)
        self.assertEqual(runner.next_run(now, midnight).isoformat(), "2026-11-17T00:00:00+08:00")

    def test_daily_dst_gap_selects_earliest_actual_instant_and_deduplicates_collision(self):
        config = runner.Config(self.env | {"TZ": "America/New_York", "BACKUP_TIMES": "02:30,03:00,03:30"})
        before_gap = dt.datetime(2026, 3, 8, 6, 0, tzinfo=dt.timezone.utc)
        self.assertEqual(runner.next_run(before_gap, config).isoformat(), "2026-03-08T03:00:00-04:00")
        after_first = dt.datetime(2026, 3, 8, 7, 0, tzinfo=dt.timezone.utc)
        self.assertEqual(runner.next_run(after_first, config).isoformat(), "2026-03-08T03:30:00-04:00")
        # 02:30 and 03:30 both normalize to 03:30; neither should replay at that instant.
        after_collision = dt.datetime(2026, 3, 8, 7, 30, tzinfo=dt.timezone.utc)
        self.assertEqual(runner.next_run(after_collision, config).isoformat(), "2026-03-09T02:30:00-04:00")

    def test_daily_dst_fallback_does_not_repeat_a_wall_clock_point(self):
        config = runner.Config(self.env | {"TZ": "America/New_York", "BACKUP_TIMES": "01:30,02:30"})
        cases = [
            ("2026-11-01T05:00:00+00:00", "2026-11-01T01:30:00-04:00"),
            ("2026-11-01T05:30:00+00:00", "2026-11-01T02:30:00-05:00"),
            ("2026-11-01T06:00:00+00:00", "2026-11-01T02:30:00-05:00"),
        ]
        for now, expected in cases:
            with self.subTest(now=now):
                self.assertEqual(runner.next_run(dt.datetime.fromisoformat(now), config).isoformat(), expected)

    def test_interval_is_elapsed_time_across_dst_and_independent_of_timezone(self):
        for zone in ("America/New_York", "Asia/Shanghai", "UTC"):
            config = runner.Config(self.env | {"TZ": zone, "BACKUP_SCHEDULE_MODE": "interval", "BACKUP_INTERVAL": "1h"})
            for timestamp in ("2026-03-08T06:30:00+00:00", "2026-11-01T05:30:00+00:00"):
                now = dt.datetime.fromisoformat(timestamp).astimezone(config.zone)
                with self.subTest(zone=zone, timestamp=timestamp):
                    self.assertEqual(runner.next_run(now, config).timestamp(), now.timestamp() + 3600)

    def test_interval_health_window_scales_to_plan_but_accepts_explicit_override(self):
        self.assertEqual(self.config.health_age, 30 * 3600)
        config = runner.Config(self.env | {"BACKUP_SCHEDULE_MODE": "interval", "BACKUP_INTERVAL": "7d",
                                           "BACKUP_HEALTH_MAX_AGE_HOURS": ""})
        self.assertGreaterEqual(config.health_age, config.interval + config.timeout + 3600)
        self.assertGreaterEqual(config.health_age, 30 * 3600)
        explicit = runner.Config(self.env | {"BACKUP_SCHEDULE_MODE": "interval", "BACKUP_INTERVAL": "7d",
                                             "BACKUP_HEALTH_MAX_AGE_HOURS": "200"})
        self.assertEqual(explicit.health_age, 200 * 3600)

    def test_long_interval_never_extends_initial_without_success_grace(self):
        config = runner.Config(self.env | {"BACKUP_SCHEDULE_MODE": "interval", "BACKUP_INTERVAL": "30d"})
        now = dt.datetime(2026, 11, 16, 12, tzinfo=dt.timezone.utc)
        started = (now - dt.timedelta(hours=31)).isoformat()
        status = {"state": "running", "identity": config.identity(),
                  "first_started_at": started, "started_at": now.isoformat()}
        runner.write_status(config, status)
        with mock.patch.object(runner.dt, "datetime", wraps=dt.datetime) as clock:
            clock.now.return_value = now
            self.assertFalse(runner.health(config), "A failed initial run must not stay healthy for 30 days")
            status["success_at"] = (now - dt.timedelta(days=29)).isoformat()
            runner.write_status(config, status)
            self.assertTrue(runner.health(config), "Successful long-interval plans use their full health window")

    def test_interval_scheduler_runs_immediately_then_waits_from_completion_monotonically(self):
        env = self.env | {"BACKUP_SCHEDULE_MODE": "interval", "BACKUP_INTERVAL": "1m"}
        for fail in (False, True):
            events = []
            elapsed = {"now": 100}
            runner.STOPPING = False

            def backup(_config):
                self.assertNotIn("backup", events, "Overrunning a period must not create immediate catch-up jobs")
                events.append("backup")
                elapsed["now"] += 90
                if fail:
                    raise runner.BackupError("Fixture upload failure")

            def sleep(seconds):
                self.assertGreater(seconds, 0)
                self.assertEqual(elapsed["now"], 190)
                events.append("wait")
                runner.STOPPING = True

            with self.subTest(fail=fail), mock.patch.dict(os.environ, env, clear=True), \
                    mock.patch.object(runner, "perform_backup", side_effect=backup), \
                    mock.patch.object(runner.time, "monotonic", side_effect=lambda: elapsed["now"]), \
                    mock.patch.object(runner.time, "time", side_effect=AssertionError("Interval must not use wall clock")), \
                    mock.patch.object(runner.time, "sleep", side_effect=sleep), \
                    contextlib.redirect_stderr(io.StringIO()), contextlib.redirect_stdout(io.StringIO()):
                self.assertEqual(runner.main(["run"]), 0)
            self.assertEqual(events, ["backup", "wait"])

    def test_daily_scheduler_skips_overrun_points_and_stops_during_wait(self):
        env = self.env | {"TZ": "UTC", "BACKUP_TIMES": "03:30,04:00,12:00"}
        elapsed = {"now": dt.datetime(2026, 11, 16, 3, tzinfo=dt.timezone.utc)}
        events = []
        output = io.StringIO()

        def backup(_config):
            events.append("backup")
            elapsed["now"] = dt.datetime.fromisoformat("2026-11-16T04:05:00+00:00")

        def sleep(seconds):
            self.assertGreater(seconds, 0)
            events.append("wait")
            runner.stop(runner.signal.SIGTERM, None)

        with mock.patch.dict(os.environ, env, clear=True), \
                mock.patch.object(runner, "perform_backup", side_effect=backup), \
                mock.patch.object(runner.dt, "datetime", wraps=dt.datetime) as clock, \
                mock.patch.object(runner.time, "time", side_effect=lambda: elapsed["now"].timestamp()), \
                mock.patch.object(runner.time, "monotonic", side_effect=AssertionError("Daily plans must follow wall time")), \
                mock.patch.object(runner.time, "sleep", side_effect=sleep), contextlib.redirect_stdout(output):
            clock.now.side_effect = lambda *_args, **_kwargs: elapsed["now"]
            self.assertEqual(runner.main(["run"]), 0)
        self.assertEqual(events, ["backup", "wait"])
        waiting = json.loads(output.getvalue())
        self.assertEqual(waiting["mode"], "daily")
        self.assertEqual(waiting["next_run_at"], "2026-11-16T12:00:00+00:00")

    def test_restarts_do_not_renew_health_bootstrap_grace(self):
        old = "2020-01-01T00:00:00+00:00"
        runner.write_status(self.config, {"state": "running", "started_at": old,
                                         "first_started_at": old, "identity": self.config.identity()})

        def interrupted_backup():
            status = runner.read_status(self.config)
            self.assertEqual(status["first_started_at"], old)
            self.assertNotEqual(status["started_at"], old)
            self.assertFalse(runner.health(self.config))
            raise runner.BackupError("Interrupted fixture")

        with mock.patch.object(runner.Job, "backup", side_effect=interrupted_backup), \
                self.assertRaisesRegex(runner.BackupError, "Interrupted fixture"):
            runner.perform_backup(self.config)

    def test_restore_never_overwrites_live_paths_or_accepts_latest(self):
        cases = [
            ["restore-files", "latest", str(self.root / "restore")],
            ["restore-files", SNAPSHOT[:8], "relative-path"],
            ["restore-files", SNAPSHOT[:8], str(self.config.source)],
            ["restore-files", SNAPSHOT[:8], str(self.config.source / "nested")],
            ["restore-files", SNAPSHOT[:8], str(self.config.state / "nested")],
            ["restore-files", SNAPSHOT[:8], str(self.root / "repository/nested")],
        ]
        with mock.patch.dict(os.environ, self.env, clear=True), mock.patch.object(runner.Job, "restic") as repository:
            for arguments in cases:
                with self.subTest(arguments=arguments), self.assertRaises(runner.BackupError):
                    runner.main(arguments)
        repository.assert_not_called()

    def test_restore_uses_explicit_owned_snapshot_and_verifies_files(self):
        calls = []

        def restic(stage, args):
            calls.append(args)
            if args[0] == "snapshots":
                return self.fake_restic(stage, args)
            return "verified"

        destination = self.root / "restore"
        with mock.patch.dict(os.environ, self.env, clear=True), \
                mock.patch.object(runner.Job, "restic", side_effect=restic), contextlib.redirect_stdout(io.StringIO()):
            self.assertEqual(runner.main(["restore-files", SNAPSHOT[:8], str(destination)]), 0)
        self.assertTrue(destination.is_dir())
        self.assertIn("--verify", calls[-1])
        self.assertEqual(calls[-1][1], SNAPSHOT[:8])

    def test_retention_defaults_to_preview_and_groups_varying_dump_paths(self):
        with mock.patch.dict(os.environ, self.env, clear=True), \
                mock.patch.object(runner.Job, "restic", return_value="preview") as repository, \
                contextlib.redirect_stdout(io.StringIO()):
            self.assertEqual(runner.main(["retention"]), 0)
        args = repository.call_args.args[1]
        self.assertIn("--dry-run", args)
        self.assertNotIn("--prune", args)
        self.assertEqual(args[args.index("--group-by") + 1], "host,tags")
        self.assertEqual(args[args.index("--host") + 1], self.config.host)
        self.assertEqual(args[args.index("--tag") + 1], self.config.tag)

    def test_retention_apply_requires_recent_success(self):
        with mock.patch.dict(os.environ, self.env, clear=True), \
                mock.patch.object(runner.Job, "restic") as repository, \
                self.assertRaisesRegex(runner.BackupError, "recent successful"):
            runner.main(["retention", "--apply"])
        repository.assert_not_called()
        runner.write_status(self.config, {"state": "success", "identity": self.config.identity(),
                                         "success_at": runner.utcnow(), "started_at": runner.utcnow()})
        with mock.patch.dict(os.environ, self.env, clear=True), \
                mock.patch.object(runner.Job, "restic", return_value="pruned") as repository, \
                contextlib.redirect_stdout(io.StringIO()):
            self.assertEqual(runner.main(["retention", "--apply"]), 0)
        self.assertIn("--prune", repository.call_args.args[1])

    def test_space_guard_stops_before_filling_disk(self):
        job = runner.Job(self.config)
        low_space = mock.Mock(free=self.config.min_free - 1)
        with mock.patch.object(runner.shutil, "disk_usage", return_value=low_space), \
                self.assertRaisesRegex(runner.BackupError, "Insufficient free space"):
            job.space_guard()

    def test_command_errors_never_include_secret_subprocess_output(self):
        process = mock.Mock(pid=123456, returncode=1)
        process.poll.return_value = 1
        process.wait.return_value = 1
        with mock.patch.object(runner.subprocess, "Popen", return_value=process), \
                mock.patch.object(runner.os, "killpg"), \
                self.assertRaisesRegex(runner.BackupError, r"Export: command failed \(exit 1\)") as caught:
            runner.Job(self.config).command("Export", ["pg_dump"], monitor=False)
        self.assertNotIn("database-test-password", str(caught.exception))
        self.assertEqual(caught.exception.exit_code, 1)

    def test_provider_diagnostics_use_fixed_codes_without_echoing_sensitive_details(self):
        codes = [(code, code) for code in ("BucketAlreadyExists", "BucketAlreadyOwnedByYou", "NoSuchBucket",
                                          "InvalidAccessKeyId", "SignatureDoesNotMatch", "AccessDenied", "UnknownProviderError")]
        codes += [("no space left on device", "NoSpaceLeft"),
                  ("read-only file system", "ReadOnlyFilesystem"),
                  ("too many open files", "TooManyOpenFiles")]
        for detail, code in codes:
            process = mock.Mock(pid=123456, returncode=1)
            process.poll.return_value = 1
            process.wait.return_value = 1

            def launch(*_args, **kwargs):
                kwargs["stderr"].write((detail + " https://private.invalid?secret=test-secret timeout").encode())
                return process

            with self.subTest(code=code), mock.patch.object(runner.subprocess, "Popen", side_effect=launch), \
                    mock.patch.object(runner.os, "killpg"), self.assertRaises(runner.BackupError) as caught:
                runner.Job(self.config).command("Repository initialization", ["restic"], monitor=False)
            message = str(caught.exception)
            self.assertNotIn("test-secret", message)
            self.assertNotIn("private.invalid", message)
            if code == "UnknownProviderError":
                self.assertNotIn(code, message)
                self.assertIn("check connectivity, credentials and permissions", message)
            else:
                self.assertIn(code, message)
                self.assertFalse(caught.exception.retryable)

    def remote_config(self):
        return runner.Config(self.env | {
            "BACKUP_REPOSITORY": "rclone:oss:backup-bucket/new-api",
            "BACKUP_OSS_ENDPOINT": "oss-cn-hangzhou.aliyuncs.com",
            "BACKUP_OSS_ACCESS_KEY_ID": "test-access",
            "BACKUP_OSS_ACCESS_KEY_SECRET": "test-secret",
        })

    def test_auto_init_defaults_on_and_accepts_explicit_opt_out(self):
        self.assertTrue(self.config.auto_init)
        self.assertFalse(runner.Config(self.env | {"BACKUP_AUTO_INIT": "false"}).auto_init)
        self.assertTrue(runner.Config(self.env | {"BACKUP_AUTO_INIT": " TRUE "}).auto_init)
        with self.assertRaises(runner.BackupError):
            runner.Config(self.env | {"BACKUP_AUTO_INIT": "maybe"})

    def test_empty_local_repository_is_initialized_and_verified_once(self):
        self.root.joinpath("repository").mkdir()
        metadata = json.dumps({"version": 2, "id": REPOSITORY})
        with mock.patch.object(runner.Job, "restic", side_effect=[
            runner.BackupError("Repository missing", exit_code=10), "initialized", metadata,
        ]) as repository, contextlib.redirect_stdout(io.StringIO()):
            result = runner.Job(self.config).ensure_repository()
        self.assertEqual(result, REPOSITORY)
        self.assertEqual([call.args[1] for call in repository.call_args_list],
                         [["cat", "config"], ["init"], ["cat", "config"]])
        self.assertEqual(json.loads((self.config.state / "repository.json").read_text())["id"], REPOSITORY)

    def test_existing_repository_redeploy_preserves_id_without_initializing(self):
        with mock.patch.object(runner.Job, "restic", side_effect=self.fake_restic) as repository, \
                contextlib.redirect_stdout(io.StringIO()):
            self.assertEqual(runner.Job(self.config).ensure_repository(), REPOSITORY)
            self.assertEqual(runner.Job(self.config).ensure_repository(), REPOSITORY)
            with mock.patch.dict(os.environ, self.env, clear=True):
                self.assertEqual(runner.main(["init"]), 0)
        self.assertEqual([call.args[1] for call in repository.call_args_list], [["cat", "config"]] * 3)

    def test_fresh_node_accepts_existing_repository_without_local_history(self):
        config = runner.Config(self.env | {"BACKUP_STATE_DIR": str(self.root / "fresh-state"),
                                           "BACKUP_CACHE_DIR": str(self.root / "fresh-cache")})
        config.prepare()
        with mock.patch.object(runner.Job, "restic", side_effect=self.fake_restic) as repository, \
                contextlib.redirect_stdout(io.StringIO()):
            self.assertEqual(runner.Job(config).ensure_repository(), REPOSITORY)
        self.assertEqual([call.args[1] for call in repository.call_args_list], [["cat", "config"]])

    def test_password_permissions_and_network_errors_never_initialize(self):
        for code in (1, 11, 12):
            with self.subTest(exit_code=code), \
                    mock.patch.object(runner.Job, "restic", side_effect=runner.BackupError("Probe failed", exit_code=code)) as repository, \
                    mock.patch.object(runner.Job, "retry_command") as listing, self.assertRaises(runner.BackupError):
                runner.Job(self.config).ensure_repository()
            self.assertEqual([call.args[1] for call in repository.call_args_list], [["cat", "config"]])
            listing.assert_not_called()
        self.assertFalse((self.config.state / "repository.json").exists())

    def test_auto_init_disabled_requires_explicit_init_for_an_empty_destination(self):
        config = runner.Config(self.env | {"BACKUP_AUTO_INIT": "false"})
        missing = runner.BackupError("Repository missing", exit_code=10)
        with mock.patch.object(runner.Job, "restic", side_effect=missing) as repository, self.assertRaises(runner.BackupError):
            runner.Job(config).ensure_repository()
        self.assertEqual([call.args[1] for call in repository.call_args_list], [["cat", "config"]])
        with mock.patch.object(runner.Job, "restic", side_effect=[missing, "initialized", json.dumps({"id": REPOSITORY})]) as repository, \
                contextlib.redirect_stdout(io.StringIO()):
            self.assertEqual(runner.Job(config).ensure_repository(explicit_init=True), REPOSITORY)
        self.assertIn(["init"], [call.args[1] for call in repository.call_args_list])

    def test_missing_config_in_nonempty_local_repository_is_not_recreated(self):
        destination = self.root / "repository"
        destination.mkdir()
        (destination / "retained-pack").write_bytes(b"existing encrypted bytes")
        with mock.patch.object(runner.Job, "restic", side_effect=runner.BackupError("Repository missing", exit_code=10)) as repository, \
                self.assertRaises(runner.BackupError):
            runner.Job(self.config).ensure_repository()
        self.assertEqual((destination / "retained-pack").read_bytes(), b"existing encrypted bytes")
        self.assertEqual([call.args[1] for call in repository.call_args_list], [["cat", "config"]])

    def test_empty_remote_prefix_is_checked_before_initialization(self):
        config = self.remote_config()
        with mock.patch.object(runner.Job, "restic", side_effect=[
            runner.BackupError("Repository missing", exit_code=10), "initialized", json.dumps({"id": REPOSITORY}),
        ]) as repository, mock.patch.object(runner.Job, "retry_command", return_value="[]") as listing, \
                contextlib.redirect_stdout(io.StringIO()):
            self.assertEqual(runner.Job(config).ensure_repository(), REPOSITORY)
        self.assertEqual(listing.call_args.args[1][:3], ["rclone", "lsjson", "oss:backup-bucket/new-api"])
        self.assertIn(["init"], [call.args[1] for call in repository.call_args_list])

    def test_remote_destination_listing_must_positively_prove_emptiness(self):
        config = self.remote_config()
        for listing in ('[{"Path":"data","IsDir":true}]', '{"error":"access denied"}', 'not JSON'):
            with self.subTest(listing=listing), \
                    mock.patch.object(runner.Job, "restic", side_effect=runner.BackupError("Repository missing", exit_code=10)) as repository, \
                    mock.patch.object(runner.Job, "retry_command", return_value=listing), self.assertRaises(runner.BackupError):
                runner.Job(config).ensure_repository()
            self.assertEqual([call.args[1] for call in repository.call_args_list], [["cat", "config"]])
        with mock.patch.object(runner.Job, "restic", side_effect=runner.BackupError("Repository missing", exit_code=10)) as repository, \
                mock.patch.object(runner.Job, "retry_command", side_effect=runner.BackupError("Bucket missing", exit_code=3)), \
                self.assertRaises(runner.BackupError):
            runner.Job(config).ensure_repository()
        self.assertEqual([call.args[1] for call in repository.call_args_list], [["cat", "config"]])

    def test_previously_successful_node_does_not_recreate_a_missing_repository(self):
        previous = {"success_at": runner.utcnow(), "snapshot_id": SNAPSHOT}
        with mock.patch.object(runner.Job, "restic", side_effect=runner.BackupError("Repository missing", exit_code=10)) as repository, \
                self.assertRaises(runner.BackupError):
            runner.Job(self.config, previous_status=previous).ensure_repository()
        self.assertEqual([call.args[1] for call in repository.call_args_list], [["cat", "config"]])

    def test_legacy_success_history_survives_repeated_failed_destination_changes(self):
        # Old releases have status.json but no repository pin. A changed endpoint
        # must not erase that history after the first guarded failure.
        runner.write_status(self.config, {"state": "success", "identity": self.config.identity(),
                                         "started_at": runner.utcnow(), "success_at": runner.utcnow(),
                                         "snapshot_id": SNAPSHOT})
        changed = runner.Config(self.env | {"BACKUP_REPOSITORY": str(self.root / "changed-empty-repository")})
        with mock.patch.object(runner.Job, "restic", side_effect=runner.BackupError("Repository missing", exit_code=10)) as repository, \
                contextlib.redirect_stdout(io.StringIO()):
            for _ in range(2):
                with self.assertRaises(runner.BackupError):
                    runner.perform_backup(changed)
        self.assertEqual([call.args[1] for call in repository.call_args_list], [["cat", "config"], ["cat", "config"]])

    def test_pinned_repository_is_not_recreated_after_its_config_disappears(self):
        with mock.patch.object(runner.Job, "restic", side_effect=self.fake_restic), contextlib.redirect_stdout(io.StringIO()):
            runner.Job(self.config).ensure_repository()
        with mock.patch.object(runner.Job, "restic", side_effect=runner.BackupError("Repository missing", exit_code=10)) as repository, \
                self.assertRaises(runner.BackupError):
            runner.Job(self.config).ensure_repository()
        self.assertEqual([call.args[1] for call in repository.call_args_list], [["cat", "config"]])

    def test_repository_location_alias_accepts_same_verified_id_but_not_a_missing_repo(self):
        with mock.patch.object(runner.Job, "restic", side_effect=self.fake_restic), contextlib.redirect_stdout(io.StringIO()):
            runner.Job(self.config).ensure_repository()
        moved = runner.Config(self.env | {"BACKUP_REPOSITORY": str(self.root / "different-repository")})
        with mock.patch.object(runner.Job, "restic", side_effect=runner.BackupError("Repository missing", exit_code=10)) as repository, \
                self.assertRaises(runner.BackupError):
            runner.Job(moved).ensure_repository()
        self.assertEqual([call.args[1] for call in repository.call_args_list], [["cat", "config"]])
        with mock.patch.object(runner.Job, "restic", side_effect=self.fake_restic) as repository, \
                contextlib.redirect_stdout(io.StringIO()):
            self.assertEqual(runner.Job(moved).ensure_repository(), REPOSITORY)
        self.assertEqual([call.args[1] for call in repository.call_args_list], [["cat", "config"]])

    def test_repository_id_replacement_and_corrupt_pin_fail_closed(self):
        with mock.patch.object(runner.Job, "restic", side_effect=self.fake_restic), contextlib.redirect_stdout(io.StringIO()):
            runner.Job(self.config).ensure_repository()
        with mock.patch.object(runner.Job, "restic", return_value=json.dumps({"id": "c" * 64})) as repository, \
                self.assertRaises(runner.BackupError):
            runner.Job(self.config).ensure_repository()
        self.assertEqual([call.args[1] for call in repository.call_args_list], [["cat", "config"]])
        (self.config.state / "repository.json").write_text('not JSON')
        with mock.patch.object(runner.Job, "restic") as repository, self.assertRaises(runner.BackupError):
            runner.Job(self.config).ensure_repository(explicit_init=True)
        repository.assert_not_called()

    def test_initialization_requires_a_valid_post_creation_repository(self):
        missing = runner.BackupError("Repository missing", exit_code=10)
        for metadata in ('not JSON', '{"id":"short"}', '[]', '{}'):
            with self.subTest(metadata=metadata), \
                    mock.patch.object(runner.Job, "restic", side_effect=[missing, "initialized", metadata]), \
                    self.assertRaises(runner.BackupError):
                runner.Job(self.config).ensure_repository()
            self.assertFalse((self.config.state / "repository.json").exists())

    def test_failed_initialization_does_not_persist_success_or_retry_init(self):
        init_error = runner.BackupError("Initialization uncertain", exit_code=1)
        with mock.patch.object(runner.Job, "restic", side_effect=[
            runner.BackupError("Repository missing", exit_code=10),
            init_error,
            runner.BackupError("Repository still missing", exit_code=10),
        ]) as repository, self.assertRaises(runner.BackupError) as caught:
            runner.Job(self.config).ensure_repository()
        self.assertIs(caught.exception, init_error)
        self.assertEqual([call.args[1] for call in repository.call_args_list],
                         [["cat", "config"], ["init"], ["cat", "config"]])
        self.assertFalse((self.config.state / "repository.json").exists())

    def test_uncertain_initialization_is_accepted_only_after_repository_verification(self):
        for message in ("Config file already exists", "BucketAlreadyExists", "Connection timed out"):
            with self.subTest(message=message), mock.patch.object(runner.Job, "restic", side_effect=[
                runner.BackupError("Repository missing", exit_code=10),
                runner.BackupError(message, exit_code=1), json.dumps({"id": REPOSITORY}),
            ]) as repository, contextlib.redirect_stdout(io.StringIO()) as output:
                self.assertEqual(runner.Job(self.config).ensure_repository(explicit_init=True), REPOSITORY)
            self.assertEqual([call.args[1] for call in repository.call_args_list],
                             [["cat", "config"], ["init"], ["cat", "config"]])
            self.assertFalse(json.loads(output.getvalue())["initialized"])
            self.assertEqual(json.loads((self.config.state / "repository.json").read_text())["id"], REPOSITORY)

    def test_failed_initialization_readback_rejects_wrong_password_and_invalid_metadata(self):
        for result in (runner.BackupError("Wrong password", exit_code=1),
                       runner.BackupError("AccessDenied", exit_code=1), 'not JSON', '{"id":"short"}'):
            with self.subTest(result=result), mock.patch.object(runner.Job, "restic", side_effect=[
                runner.BackupError("Repository missing", exit_code=10),
                runner.BackupError("Config file already exists", exit_code=1), result,
            ]), self.assertRaises(runner.BackupError):
                runner.Job(self.config).ensure_repository()
            self.assertFalse((self.config.state / "repository.json").exists())

    def test_failed_initialization_does_not_probe_after_stop_or_deadline(self):
        for stopping in (False, True):
            job = runner.Job(self.config)
            with self.subTest(stopping=stopping), mock.patch.object(runner, "STOPPING", stopping), \
                    mock.patch.object(runner.time, "monotonic", return_value=job.deadline - 1 if stopping else job.deadline), \
                    mock.patch.object(job, "restic", side_effect=[
                        runner.BackupError("Repository missing", exit_code=10),
                        runner.BackupError("Initialization interrupted", exit_code=1),
                    ]) as repository, self.assertRaises(runner.BackupError):
                job.ensure_repository()
            self.assertEqual([call.args[1] for call in repository.call_args_list], [["cat", "config"], ["init"]])

    def test_initialization_write_is_not_retried_on_uncertain_network_failure(self):
        config = runner.Config(self.env | {"BACKUP_RETRIES": "2"})
        with mock.patch.object(runner.Job, "command", side_effect=runner.BackupError("Timeout", True)) as command, \
                self.assertRaises(runner.BackupError):
            runner.Job(config).restic("Repository initialization", ["init"])
        self.assertEqual(command.call_count, 1)

    def test_timeout_kills_only_owned_process_group_even_if_leader_does_not_exit(self):
        job = runner.Job(self.config)
        job.deadline = 1
        process = mock.Mock(pid=123456)
        process.poll.return_value = None
        process.wait.side_effect = [runner.subprocess.TimeoutExpired("fixture", 5), 0]
        with mock.patch.object(runner.subprocess, "Popen", return_value=process) as launch, \
                mock.patch.object(runner.os, "killpg") as kill, \
                mock.patch.object(runner.time, "monotonic", side_effect=[0, 2]), \
                self.assertRaisesRegex(runner.BackupError, "timed out"):
            job.command("Timeout fixture", ["fixture"], monitor=False)
        self.assertTrue(launch.call_args.kwargs["start_new_session"])
        self.assertEqual(kill.call_args_list, [mock.call(process.pid, runner.signal.SIGTERM),
                                              mock.call(process.pid, runner.signal.SIGKILL)])

    def test_shutdown_kills_owned_child_group_before_returning(self):
        process = mock.Mock(pid=123456)
        process.poll.return_value = None

        def launch(*_args, **_kwargs):
            runner.STOPPING = True
            return process

        with mock.patch.object(runner.subprocess, "Popen", side_effect=launch), \
                mock.patch.object(runner.os, "killpg") as kill, \
                self.assertRaisesRegex(runner.BackupError, "interrupted"):
            runner.Job(self.config).command("Shutdown fixture", ["fixture"], monitor=False)
        self.assertEqual(kill.call_args_list, [mock.call(process.pid, runner.signal.SIGTERM),
                                              mock.call(process.pid, runner.signal.SIGKILL)])

    def test_network_retry_repeats_only_the_failed_repository_operation(self):
        config = runner.Config(self.env | {"BACKUP_RETRIES": "1", "BACKUP_RETRY_DELAY_SECONDS": "1"})
        job = runner.Job(config)
        with mock.patch.object(job, "command", side_effect=[runner.BackupError("Transient network timeout", True), "receipt"]) as command, \
                mock.patch.object(runner.time, "monotonic", side_effect=[0, 0, 1]):
            result = job.restic("Snapshot receipt", ["snapshots", "--json", SNAPSHOT])
        self.assertEqual(result, "receipt")
        self.assertEqual(command.call_count, 2)
        self.assertEqual(command.call_args.args[1][-3:], ["snapshots", "--json", SNAPSHOT])

    def test_configuration_failure_is_not_retried(self):
        config = runner.Config(self.env | {"BACKUP_RETRIES": "2"})
        with mock.patch.object(runner.Job, "command", side_effect=runner.BackupError("Wrong repository password")) as command, \
                self.assertRaisesRegex(runner.BackupError, "Wrong repository password"):
            runner.Job(config).restic("Repository", ["snapshots"])
        self.assertEqual(command.call_count, 1)

    def test_database_startup_retry_does_not_repeat_export(self):
        config = runner.Config(self.env | {"BACKUP_RETRIES": "1", "BACKUP_RETRY_DELAY_SECONDS": "1"})
        psql_attempts = 0
        exports = 0

        def command(stage, args, env=None, monitor=True):
            nonlocal psql_attempts, exports
            if args[0] == "psql":
                psql_attempts += 1
                if psql_attempts == 1:
                    raise runner.BackupError("PostgreSQL readiness: connection refused", True)
            if args[0] == "pg_dump" and "--file" in args:
                exports += 1
            return self.fake_command(stage, args, env, monitor)

        with mock.patch.object(runner.Job, "command", side_effect=command), \
                mock.patch.object(runner.Job, "restic", side_effect=self.fake_restic), \
                mock.patch.object(runner.time, "monotonic", side_effect=[0, 0, 0, 1, 1, 1]):
            self.assertEqual(runner.Job(config).backup(), SNAPSHOT)
        self.assertEqual(psql_attempts, 2)
        self.assertEqual(exports, 1)


if __name__ == "__main__":
    unittest.main()
