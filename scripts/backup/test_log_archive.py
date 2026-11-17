"""Safety contracts for permanent log history and paired application restores."""

import contextlib
import io
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest import mock

import runner


APP_ROLE = "new-api:application:v2"
LOG_ROLE = "new-api:logs:v1"
APP_ID = "a" * 64
LOG_ID = "b" * 64
LEGACY_ID = "c" * 64


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
            APP_ID: self.receipt(APP_ID, [self.config.tag, APP_ROLE]),
            LOG_ID: self.receipt(LOG_ID, [self.config.tag, LOG_ROLE], [str(self.config.source / "logs")]),
            LEGACY_ID: self.receipt(LEGACY_ID, [self.config.tag]),
        }
        self.manifest = {
            "format": 2,
            "source": str(self.config.source),
            "consistency": "online-uncoordinated",
            "started_at": runner.utcnow(),
            "databases": [{"name": "app", "file": "database-000.dump", "bytes": 15,
                           "sha256": "d" * 64, "server_version_num": 150020}],
            "logs": {"snapshot_id": LOG_ID, "path": str(self.config.source / "logs"),
                     "captured_at": runner.utcnow(), "missing": False},
        }
        self.calls = []
        self.dump_manifests = {}
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
        if args[0] == "dump":
            manifest = self.dump_manifests.get(args[-1], self.manifest)
            if isinstance(manifest, Exception):
                raise manifest
            return json.dumps(manifest)
        if args[0] in ("restore", "forget"):
            return "verified"
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
        for role in (APP_ROLE, LOG_ROLE):
            with self.subTest(role=role), self.assertRaises(runner.BackupError):
                runner.Config(self.env | {"BACKUP_TAG": role})

    def test_receipt_rejects_wrong_prefix_foreign_host_and_foreign_base_tag(self):
        cases = [
            self.receipt(LEGACY_ID, [self.config.tag, APP_ROLE]),
            self.receipt(APP_ID, [self.config.tag + "-another", APP_ROLE]),
            self.receipt(APP_ID, [self.config.tag, APP_ROLE]) | {"hostname": "another-host"},
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

    def test_paired_restore_uses_manifest_log_id_and_one_new_target(self):
        # Paths may arrive in either order; the first path is not necessarily a job directory.
        self.receipts[APP_ID]["paths"].reverse()
        target = self.root / "paired-restore"
        result, _ = self.run_cli(["restore-files", APP_ID, str(target)])
        self.assertEqual(result, 0)
        restores = [args for args in self.calls if args[0] == "restore"]
        self.assertEqual([args[1] for args in restores], [APP_ID, LOG_ID])
        for args in restores:
            self.assertEqual(args[args.index("--target") + 1], str(target))
            self.assertIn("--verify", args)
            self.assertNotIn("--delete", args)
        self.assertEqual(restores[1][restores[1].index("--include") + 1], str(self.config.source / "logs"))

    def test_paired_restore_identifies_job_named_source_after_state_directory_migration(self):
        old_export = Path("/previous-state/job-export")
        old_source = Path("/previous-source/job-project")
        self.manifest["source"] = str(old_source)
        self.manifest["logs"]["path"] = str(old_source / "logs")
        self.receipts[LOG_ID]["paths"] = [str(old_source / "logs")]
        self.dump_manifests = {
            str(old_export / "manifest.json"): self.manifest,
            str(old_source / "manifest.json"): runner.BackupError("No archive manifest at source root"),
        }
        for index, paths in enumerate(([str(old_export), str(old_source)], [str(old_source), str(old_export)])):
            self.calls.clear()
            self.receipts[APP_ID]["paths"] = paths
            result, _ = self.run_cli(["restore-files", APP_ID, str(self.root / ("migrated-" + str(index)))])
            self.assertEqual(result, 0)
            restores = [args for args in self.calls if args[0] == "restore"]
            self.assertEqual([args[1] for args in restores], [APP_ID, LOG_ID])
            self.assertEqual(restores[1][restores[1].index("--include") + 1], str(old_source / "logs"))

    def test_ambiguous_valid_application_manifests_are_rejected_before_restore(self):
        export = Path("/old-state/job-export")
        source = Path("/old-source/job-project")
        self.receipts[APP_ID]["paths"] = [str(export), str(source)]
        self.dump_manifests = {
            str(export / "manifest.json"): self.manifest | {
                "source": str(source), "logs": self.manifest["logs"] | {"path": str(source / "logs")}},
            str(source / "manifest.json"): self.manifest | {
                "source": str(export), "logs": self.manifest["logs"] | {"path": str(export / "logs")}},
        }
        target = self.root / "ambiguous-restore"
        with self.assertRaises(runner.BackupError):
            self.run_cli(["restore-files", APP_ID, str(target)])
        self.assertFalse(target.exists())
        self.assertFalse(any(args[0] == "restore" for args in self.calls))

    def test_legacy_log_restore_identifies_a_job_named_source_from_its_old_manifest(self):
        export = Path("/legacy-state/job-export")
        source = Path("/legacy-source/job-project")
        self.receipts[LEGACY_ID]["paths"] = [str(source), str(export)]
        self.dump_manifests = {
            str(export / "manifest.json"): {"format": 1, "consistency": "online-uncoordinated",
                                           "databases": self.manifest["databases"]},
            str(source / "manifest.json"): runner.BackupError("No archive manifest at source root"),
        }
        result, _ = self.run_cli(["restore-logs", LEGACY_ID, str(self.root / "legacy-job-logs")])
        self.assertEqual(result, 0)
        restore = next(args for args in self.calls if args[0] == "restore")
        self.assertEqual(restore[restore.index("--include") + 1], str(source / "logs"))

    def test_paired_restore_rejects_invalid_log_identity_role_or_path_before_writing(self):
        cases = [
            self.receipt(LOG_ID, [self.config.tag, APP_ROLE], [str(self.config.source / "logs")]),
            self.receipt(LOG_ID, [self.config.tag, LOG_ROLE, APP_ROLE], [str(self.config.source / "logs")]),
            self.receipt(LOG_ID, [self.config.tag, LOG_ROLE], [str(self.config.source / "data")]),
            self.receipt(LOG_ID, [self.config.tag, LOG_ROLE], [str(self.config.source / "logs"), str(self.config.source / "config")]),
            self.receipt(LOG_ID, [self.config.tag, LOG_ROLE], [str(self.config.source / "logs")]) | {"hostname": "foreign"},
        ]
        for index, receipt in enumerate(cases):
            self.calls.clear()
            self.receipts[LOG_ID] = receipt
            target = self.root / ("rejected-pair-" + str(index))
            with self.subTest(receipt=receipt), self.assertRaises(runner.BackupError):
                self.run_cli(["restore-files", APP_ID, str(target)])
            self.assertFalse(target.exists())
            self.assertFalse(any(args[0] == "restore" for args in self.calls))

    def test_invalid_new_manifest_never_falls_back_to_legacy_restore(self):
        valid = self.manifest
        cases = [
            valid | {"format": 1},
            {key: value for key, value in valid.items() if key != "logs"},
            valid | {"logs": valid["logs"] | {"snapshot_id": "latest"}},
            valid | {"logs": valid["logs"] | {"path": str(self.config.source / "data")}},
        ]
        for index, manifest in enumerate(cases):
            self.calls.clear()
            self.manifest = manifest
            target = self.root / ("bad-manifest-" + str(index))
            with self.subTest(manifest=manifest), self.assertRaises(runner.BackupError):
                self.run_cli(["restore-files", APP_ID, str(target)])
            self.assertFalse(target.exists())
            self.assertFalse(any(args[0] == "restore" for args in self.calls))

    def test_legacy_restore_stays_independent_of_a_new_manifest(self):
        target = self.root / "legacy-restore"
        result, _ = self.run_cli(["restore-files", LEGACY_ID, str(target)])
        self.assertEqual(result, 0)
        self.assertFalse(any(args[0] == "dump" for args in self.calls))
        self.assertEqual([args[1] for args in self.calls if args[0] == "restore"], [LEGACY_ID])

    def test_application_restore_rejects_log_only_and_ambiguous_roles(self):
        for tags in ([self.config.tag, LOG_ROLE], [self.config.tag, LOG_ROLE, APP_ROLE]):
            self.calls.clear()
            self.receipts[LOG_ID]["tags"] = tags
            target = self.root / ("wrong-role-" + str(len(tags)))
            with self.subTest(tags=tags), self.assertRaises(runner.BackupError):
                self.run_cli(["restore-files", LOG_ID, str(target)])
            self.assertFalse(target.exists())
            self.assertFalse(any(args[0] == "restore" for args in self.calls))

    def test_log_restore_limits_new_and_legacy_snapshots_to_their_log_path(self):
        for snapshot_id in (LOG_ID, LEGACY_ID):
            self.calls.clear()
            target = self.root / ("logs-" + snapshot_id[:8])
            result, _ = self.run_cli(["restore-logs", snapshot_id, str(target)])
            self.assertEqual(result, 0)
            restores = [args for args in self.calls if args[0] == "restore"]
            self.assertEqual(len(restores), 1)
            self.assertEqual(restores[0][1], snapshot_id)
            self.assertEqual(restores[0][restores[0].index("--include") + 1], str(self.config.source / "logs"))
            self.assertNotIn("--delete", restores[0])
        self.calls.clear()
        with self.assertRaises(runner.BackupError):
            self.run_cli(["restore-logs", APP_ID, str(self.root / "not-a-log-archive")])
        self.assertFalse(any(args[0] == "restore" for args in self.calls))

    def test_retention_filters_new_applications_and_preserves_log_role(self):
        runner.write_status(self.config, {"state": "success", "identity": self.config.identity(),
                                         "started_at": runner.utcnow(), "success_at": runner.utcnow()})
        result, _ = self.run_cli(["retention", "--apply"])
        self.assertEqual(result, 0)
        forget = next(args for args in self.calls if args[0] == "forget")
        self.assertEqual(set(forget[forget.index("--tag") + 1].split(",")), {self.config.tag, APP_ROLE})
        self.assertEqual(forget[forget.index("--keep-tag") + 1], LOG_ROLE)

    def test_snapshot_lists_separate_application_backups_and_discover_legacy_log_history(self):
        result, output = self.run_cli(["snapshots"])
        self.assertEqual(result, 0)
        self.assertEqual({item["id"] for item in json.loads(output)}, {APP_ID, LEGACY_ID})
        result, output = self.run_cli(["log-snapshots"])
        self.assertEqual(result, 0)
        archives = {item["id"]: item for item in json.loads(output)}
        self.assertEqual(set(archives), {LOG_ID, LEGACY_ID})
        self.assertFalse(archives[LOG_ID]["legacy"])
        self.assertTrue(archives[LEGACY_ID]["legacy"])

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
            self.assertIn(APP_ROLE, tags)
            self.assertNotIn(LOG_ROLE, tags)
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


if __name__ == "__main__":
    unittest.main()
