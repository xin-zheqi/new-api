"""Safety contracts for independent log history and application recovery."""

import contextlib
import io
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest import mock

import runner


APP_ID = "a" * 64
LOG_ID = "b" * 64
UNKNOWN_ID = "c" * 64


class LogArchiveTests(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name)
        self.env = {
            "BACKUP_STATE_DIR": str(self.root / "state"),
            "BACKUP_CACHE_DIR": str(self.root / "cache"),
            "BACKUP_SOURCE_DIR": str(self.root / "source"),
            "BACKUP_REPOSITORY": str(self.root / "repository"),
            "BACKUP_PASSWORD_FILE": str(self.root / "password"),
            "BACKUP_DB_PASSWORD": "fixture-database-password",
            "BACKUP_MIN_FREE_MB": "1",
            "BACKUP_RETRIES": "0",
            "BACKUP_HOST": "archive-fixture",
            "BACKUP_TAG": "application-fixture",
        }
        (self.root / "password").write_text("fixture-repository-password\n")
        for name in ("data", "logs", "config"):
            directory = self.root / "source" / name
            directory.mkdir(parents=True)
            (directory / "fixture.txt").write_text(name)
        self.config = runner.Config(self.env)
        self.config.prepare()
        self.work = self.config.state / "job-archived"
        self.receipts = {
            APP_ID: self.receipt(APP_ID, [self.config.tag, runner.APP_ROLE]),
            LOG_ID: self.receipt(LOG_ID, [self.config.tag, runner.LOG_ROLE], [str(self.config.source / "logs")]),
            UNKNOWN_ID: self.receipt(UNKNOWN_ID, [self.config.tag]),
        }
        self.calls = []
        runner.STOPPING = False


    def receipt(self, snapshot_id, tags, paths=None):
        return {"id": snapshot_id, "hostname": self.config.host, "tags": tags,
                "paths": paths if paths is not None else [str(self.work), str(self.config.source)]}


    def repository(self, _stage, args):
        self.calls.append(list(args))
        if args[0] == "snapshots":
            query = args[-1]
            if query in self.receipts:
                return json.dumps([self.receipts[query]])
            return json.dumps(list(self.receipts.values()))
        if args[0] == "restore":
            if args[1] == APP_ID:
                # Informational manifests must not couple application recovery
                # to the existence or readability of a log archive.
                target = Path(args[args.index("--target") + 1])
                manifest = target / "state" / "job-export" / "manifest.json"
                manifest.parent.mkdir(parents=True)
                manifest.write_text("not JSON")
            return "verified"
        if args[0] == "forget":
            return "retention verified"
        self.fail("Unexpected repository operation: " + args[0])


    def run_cli(self, args):
        with mock.patch.dict(os.environ, self.env, clear=True), \
                mock.patch.object(runner.Job, "restic", side_effect=self.repository), \
                contextlib.redirect_stdout(io.StringIO()) as output:
            result = runner.main(args)
        return result, output.getvalue()


    def export_command(self, _stage, args, env=None, monitor=True):
        if args[0] == "pg_dump" and args[1] == "--version":
            return "pg_dump (PostgreSQL) 15.20\n"
        if args[0] == "restic":
            return "restic 0.18.1\n"
        if args[0] == "rclone":
            return "rclone v1.71.2\n"
        if args[0] == "psql":
            return "150020\n"
        if args[0] == "pg_dump":
            Path(args[args.index("--file") + 1]).write_bytes(b"database-export")
            return ""
        if args[0] == "pg_restore":
            return "verified TOC"
        self.fail("Unexpected local operation: " + args[0])


    def test_reserved_role_names_cannot_be_used_as_the_base_tag(self):
        for role in (runner.APP_ROLE, runner.LOG_ROLE):
            with self.subTest(role=role), self.assertRaises(runner.BackupError):
                runner.Config(self.env | {"BACKUP_TAG": role})


    def test_receipt_rejects_wrong_prefix_foreign_host_and_foreign_base_tag(self):
        cases = [
            self.receipt(UNKNOWN_ID, [self.config.tag, runner.APP_ROLE]),
            self.receipt(APP_ID, [self.config.tag + "-another", runner.APP_ROLE]),
            self.receipt(APP_ID, [self.config.tag, runner.APP_ROLE]) | {"hostname": "another-host"},
        ]
        for receipt in cases:
            with self.subTest(receipt=receipt), \
                    mock.patch.object(runner.Job, "restic", return_value=json.dumps([receipt])), \
                    self.assertRaises(runner.BackupError):
                runner.Job(self.config).snapshot_receipt(APP_ID[:8])


    def test_destination_rejects_live_directories_repository_and_existing_targets(self):
        existing = self.root / "already-present"
        existing.mkdir()
        alias = self.root / "source-alias"
        alias.symlink_to(self.config.source, target_is_directory=True)
        for destination in ("relative", str(existing), str(alias), str(alias / "new"),
                            str(self.config.source / "new"), str(self.config.state / "new"),
                            str(self.config.cache / "new"), str(self.root / "repository" / "new")):
            with self.subTest(destination=destination), self.assertRaises(runner.BackupError):
                runner.Job(self.config).restore_destination(destination)
        safe = self.root / "safe-restore"
        self.assertEqual(runner.Job(self.config).restore_destination(str(safe)), safe)
        self.assertFalse(safe.exists(), "Preflight must not create a target before snapshots are verified")


    def test_retention_filters_new_applications_and_preserves_log_role(self):
        runner.write_status(self.config, {"state": "success", "identity": self.config.identity(),
                                         "started_at": runner.utcnow(), "success_at": runner.utcnow()})
        result, _ = self.run_cli(["retention", "--apply"])
        self.assertEqual(result, 0)
        forget = next(args for args in self.calls if args[0] == "forget")
        self.assertEqual(set(forget[forget.index("--tag") + 1].split(",")), {self.config.tag, runner.APP_ROLE})
        self.assertEqual(forget[forget.index("--keep-tag") + 1], runner.LOG_ROLE)


    def test_successful_log_archive_is_preserved_when_application_backup_fails(self):
        job = runner.Job(self.config)
        with mock.patch.object(job, "command", side_effect=self.export_command), \
                mock.patch.object(job, "ensure_repository", return_value="d" * 64), \
                mock.patch.object(job, "backup_snapshot", side_effect=[
                    (LOG_ID, {"snapshot_id": LOG_ID}), runner.BackupError("Application upload failed"),
                ]) as snapshots, mock.patch.object(job, "restic") as repository, \
                contextlib.redirect_stdout(io.StringIO()), self.assertRaises(runner.BackupError):
            job.backup()
        self.assertEqual(job.logs_snapshot_id, LOG_ID)
        self.assertEqual(snapshots.call_count, 2)
        self.assertFalse(any(call.args[1][0] == "forget" for call in repository.call_args_list))
        self.assertEqual(list(self.config.state.glob("job-*")), [])


    def test_missing_log_directory_does_not_delete_history_or_block_application_backup(self):
        logs = self.config.source / "logs"
        (logs / "fixture.txt").unlink()
        logs.rmdir()
        manifests = []

        def snapshot(_stage, paths, tags, exclude=()):
            self.assertIn(runner.APP_ROLE, tags)
            self.assertNotIn(runner.LOG_ROLE, tags)
            manifest = Path(paths[0]) / "manifest.json"
            manifests.append(json.loads(manifest.read_text()))
            return APP_ID, {"snapshot_id": APP_ID}

        job = runner.Job(self.config)
        with mock.patch.object(job, "command", side_effect=self.export_command), \
                mock.patch.object(job, "ensure_repository", return_value="d" * 64), \
                mock.patch.object(job, "backup_snapshot", side_effect=snapshot), \
                mock.patch.object(job, "restic") as repository, contextlib.redirect_stdout(io.StringIO()):
            self.assertEqual(job.backup(), APP_ID)
        self.assertEqual(len(manifests), 1)
        self.assertTrue(manifests[0]["logs"]["missing"])
        self.assertFalse(manifests[0]["logs"]["snapshot_id"])
        self.assertFalse(any(call.args[1][0] == "forget" for call in repository.call_args_list))


    def test_unreadable_log_directory_is_not_reported_as_missing_or_success(self):
        logs = self.config.source / "logs"
        original_stat = Path.stat

        def denied_stat(path, *args, **kwargs):
            if path == logs:
                raise PermissionError("fixture access denied")
            return original_stat(path, *args, **kwargs)

        with mock.patch.object(Path, "stat", side_effect=denied_stat, autospec=True), \
                mock.patch.object(runner.Job, "command", side_effect=self.export_command), \
                mock.patch.object(runner.Job, "ensure_repository", return_value="d" * 64), \
                mock.patch.object(runner.Job, "backup_snapshot") as snapshots, \
                contextlib.redirect_stdout(io.StringIO()), self.assertRaises(runner.BackupError):
            runner.perform_backup(self.config)
        self.assertEqual(runner.read_status(self.config)["state"], "failed")
        snapshots.assert_not_called()


    def test_application_restore_is_independent_of_log_archives_and_manifest_contents(self):
        self.receipts.pop(LOG_ID)
        target = self.root / "application-only"
        result, _ = self.run_cli(["restore-files", APP_ID, str(target)])
        self.assertEqual(result, 0)
        restores = [args for args in self.calls if args[0] == "restore"]
        self.assertEqual(len(restores), 1)
        self.assertEqual(restores[0][1], APP_ID)
        self.assertEqual(restores[0][restores[0].index("--target") + 1], str(target))
        self.assertIn("--verify", restores[0])
        self.assertNotIn("--delete", restores[0])
        self.assertNotIn("--include", restores[0])
        self.assertEqual([args[-1] for args in self.calls if args[0] == "snapshots"], [APP_ID])
        self.assertFalse(any(args[0] == "dump" for args in self.calls))
        self.assertEqual((target / "state" / "job-export" / "manifest.json").read_text(), "not JSON")

    def test_restore_rejects_cross_role_unknown_and_dual_role_snapshots_before_writing(self):
        cases = [
            ("restore-files", [self.config.tag, runner.LOG_ROLE]),
            ("restore-logs", [self.config.tag, runner.APP_ROLE]),
            ("restore-files", [self.config.tag]),
            ("restore-logs", [self.config.tag]),
            ("restore-files", [self.config.tag, runner.APP_ROLE, runner.LOG_ROLE]),
            ("restore-logs", [self.config.tag, runner.APP_ROLE, runner.LOG_ROLE]),
        ]
        for index, (command, tags) in enumerate(cases):
            self.calls.clear()
            self.receipts[UNKNOWN_ID] = self.receipt(UNKNOWN_ID, tags, [str(self.config.source / "logs")])
            target = self.root / ("rejected-role-" + str(index))
            with self.subTest(command=command, tags=tags), self.assertRaises(runner.BackupError):
                self.run_cli([command, UNKNOWN_ID, str(target)])
            self.assertFalse(target.exists())
            self.assertFalse(any(args[0] == "restore" for args in self.calls))

    def test_log_restore_only_includes_the_verified_logs_directory(self):
        target = self.root / "logs-only"
        result, _ = self.run_cli(["restore-logs", LOG_ID, str(target)])
        self.assertEqual(result, 0)
        restores = [args for args in self.calls if args[0] == "restore"]
        self.assertEqual(len(restores), 1)
        self.assertEqual(restores[0][1], LOG_ID)
        self.assertEqual(restores[0][restores[0].index("--include") + 1], str(self.config.source / "logs"))
        self.assertIn("--verify", restores[0])
        self.assertNotIn("--delete", restores[0])

    def test_log_restore_rejects_invalid_or_mixed_source_paths_before_writing(self):
        paths = [
            [str(self.config.source / "data")],
            [str(self.config.source / "logs"), str(self.config.source / "config")],
            ["relative/logs"],
            [str(self.config.source) + "/data/../logs"],
        ]
        for index, invalid in enumerate(paths):
            self.calls.clear()
            self.receipts[LOG_ID]["paths"] = invalid
            target = self.root / ("invalid-log-path-" + str(index))
            with self.subTest(paths=invalid), self.assertRaises(runner.BackupError):
                self.run_cli(["restore-logs", LOG_ID, str(target)])
            self.assertFalse(target.exists())
            self.assertFalse(any(args[0] == "restore" for args in self.calls))

    def test_snapshot_lists_exclude_unknown_and_dual_roles_and_keep_recovery_separate(self):
        ambiguous = "d" * 64
        self.receipts[ambiguous] = self.receipt(ambiguous, [self.config.tag, runner.APP_ROLE, runner.LOG_ROLE])
        result, output = self.run_cli(["snapshots"])
        self.assertEqual(result, 0)
        self.assertEqual({item["id"] for item in json.loads(output)}, {APP_ID})
        result, output = self.run_cli(["log-snapshots"])
        self.assertEqual(result, 0)
        self.assertEqual({item["id"] for item in json.loads(output)}, {LOG_ID})



if __name__ == "__main__":
    unittest.main()
