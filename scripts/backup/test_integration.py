#!/usr/bin/env python3
"""Exercise backup/restore against disposable PostgreSQL and MinIO containers.

Requires Docker CLI and a built backup image. No host ports, production endpoints,
or Docker socket are exposed to the backup container. Only resources with this
run's random names are removed. Test artifacts stay in ignored .local-tests/.
"""

import argparse
import datetime as dt
import hashlib
import json
import secrets
import subprocess
import sys
import time
from pathlib import Path


def docker(*args, input_text=None, ok=True, timeout=240):
    result = subprocess.run(
        ["docker", *map(str, args)], input=input_text, capture_output=True,
        text=True, encoding="utf-8", errors="replace", timeout=timeout,
    )
    if ok and result.returncode:
        raise RuntimeError(f"Docker operation failed: {result.stdout}\n{result.stderr}")
    return result


def files_digest(directory):
    return {
        path.relative_to(directory).as_posix(): hashlib.sha256(path.read_bytes()).hexdigest()
        for path in directory.rglob("*") if path.is_file()
    }


def wait_for(check, description):
    deadline = time.monotonic() + 90
    while time.monotonic() < deadline:
        if check():
            return
        time.sleep(1)
    raise RuntimeError(f"Timed out waiting for {description}")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--image", default="newapi-backup:test")
    parser.add_argument("--postgres-image", default="postgres:15")
    parser.add_argument("--minio-image", default="bitnamilegacy/minio:2024.6.13")
    args = parser.parse_args()
    prefix = "newapi-backup-it-" + secrets.token_hex(5)
    root = Path(__file__).resolve().parents[2] / ".local-tests" / prefix
    source = root / "source"
    restore = root / "restore"
    for directory in (source / "data", source / "logs", source / "config", restore):
        directory.mkdir(parents=True)
    marker = "private-fixture-" + secrets.token_hex(24)
    (source / "data" / "attachment.txt").write_text("附件内容 " + marker, encoding="utf-8")
    (source / "logs" / "application.log").write_text("test log\n", encoding="utf-8")
    (source / "config" / "deploy.env").write_text("SESSION_SECRET=" + marker, encoding="utf-8")
    password_file = root / "restic-password"
    password_file.write_text(secrets.token_urlsafe(32), encoding="ascii")
    pg_password = secrets.token_urlsafe(24)
    oss_secret = secrets.token_urlsafe(24)
    network = prefix + "-network"
    pg = prefix + "-postgres"
    minio = prefix + "-minio"
    state = prefix + "-state"
    cache = prefix + "-cache"
    pg_data = prefix + "-pgdata"
    objects = prefix + "-objects"
    containers = []
    volumes = []
    network_created = False
    report = {"image": args.image, "workspace": str(root), "checks": []}

    def checked(name, condition=True):
        if not condition:
            raise AssertionError(name)
        report["checks"].append(name)
        print("PASS " + name, flush=True)

    env = {
        "BACKUP_DB_HOST": pg, "BACKUP_DB_USER": "backup", "BACKUP_DB_PASSWORD": pg_password,
        "BACKUP_DATABASES": "app,logdb", "BACKUP_DB_SSLMODE": "disable",
        "BACKUP_REPOSITORY": "rclone:oss:backups/newapi", "BACKUP_PASSWORD_FILE": "/run/secrets/restic-password",
        "BACKUP_OSS_ENDPOINT": f"http://{minio}:9000", "BACKUP_ALLOW_INSECURE_OSS": "true",
        "BACKUP_OSS_ACCESS_KEY_ID": "backup-test", "BACKUP_OSS_ACCESS_KEY_SECRET": oss_secret,
        "RCLONE_CONFIG_OSS_FORCE_PATH_STYLE": "true", "BACKUP_HOST": "integration", "BACKUP_TAG": "new-api-test",
        "BACKUP_MIN_FREE_MB": "1", "BACKUP_MAX_DUMP_MB": "64", "BACKUP_MAX_CACHE_MB": "64",
        "BACKUP_TIMEOUT_SECONDS": "180", "BACKUP_RETRIES": "0", "BACKUP_BW_LIMIT": "0",
    }
    rclone_env = {
        "RCLONE_CONFIG": "/dev/null", "RCLONE_CONFIG_OSS_TYPE": "s3", "RCLONE_CONFIG_OSS_PROVIDER": "Other",
        "RCLONE_CONFIG_OSS_ENDPOINT": f"http://{minio}:9000", "RCLONE_CONFIG_OSS_ACCESS_KEY_ID": "backup-test",
        "RCLONE_CONFIG_OSS_SECRET_ACCESS_KEY": oss_secret, "RCLONE_CONFIG_OSS_FORCE_PATH_STYLE": "true",
        "RESTIC_REPOSITORY": "rclone:oss:backups/newapi", "RESTIC_PASSWORD_FILE": "/run/secrets/restic-password",
        "RESTIC_CACHE_DIR": "/cache",
    }

    def command(*cli, overrides=None, entrypoint=None, ok=True, detached=False):
        effective = rclone_env if entrypoint == "restic" or entrypoint == "rclone" else env
        effective = {**effective, **(overrides or {})}
        name = prefix + "-job-" + secrets.token_hex(3)
        containers.append(name)
        options = ["run", *( ["-d", "--init"] if detached else ["--rm"]), "--name", name,
                   "--label", "newapi.backup.test=" + prefix,
                   "--network", network, "--read-only", "--cap-drop", "ALL",
                   "--security-opt", "no-new-privileges:true", "--tmpfs", "/tmp:size=64m,mode=1777",
                   "--memory", "512m", "--cpus", "0.5"]
        for key, value in effective.items():
            options += ["-e", key + "=" + value]
        for mount in (
            f"type=bind,src={source},dst=/source,readonly",
            f"type=bind,src={restore},dst=/restore",
            f"type=bind,src={password_file},dst=/run/secrets/restic-password,readonly",
            f"type=volume,src={state},dst=/state", f"type=volume,src={cache},dst=/cache",
        ):
            options += ["--mount", mount]
        if entrypoint:
            options += ["--entrypoint", entrypoint]
        return docker(*options, args.image, *cli, ok=ok)

    def daemon_smoke(mode):
        now = dt.datetime.now(dt.timezone.utc)
        daily_times = [(now + dt.timedelta(hours=hours)).strftime("%H:%M") for hours in (2, 4)]
        settings = {"TZ": "UTC", "BACKUP_SCHEDULE_MODE": mode,
                    "BACKUP_TIMES": ",".join(daily_times), "BACKUP_INTERVAL": "1m"}
        command("run", overrides=settings, detached=True)
        name = containers[-1]
        wait_for(lambda: '"state": "waiting"' in docker("logs", name).stdout, mode + " daemon initial backup")
        logs = docker("logs", name).stdout
        records = [json.loads(line) for line in logs.splitlines() if line.startswith("{")]
        receipts = [item for item in records if item.get("state") == "success"]
        waiting = [item for item in records if item.get("state") == "waiting"]
        checked(mode + ": startup creates exactly one successful backup", len(receipts) == 1)
        checked(mode + ": daemon enters configured schedule mode", len(waiting) == 1 and waiting[0]["mode"] == mode)
        next_at = dt.datetime.fromisoformat(waiting[0]["next_run_at"])
        checked(mode + ": next backup is scheduled in the future", next_at > dt.datetime.now(dt.timezone.utc))
        if mode == "daily":
            checked("daily: selects next point from multiple configured times", next_at.strftime("%H:%M") == daily_times[0])
        else:
            finished_at = dt.datetime.fromisoformat(receipts[0]["finished_at"])
            checked("interval: schedule follows completed backup", 60 <= (next_at - finished_at).total_seconds() < 65)
        docker("exec", name, "python3", "/opt/backup/runner.py", "health")
        checked(mode + ": daemon health succeeds while waiting")
        checked(mode + ": scheduled backup exists in repository",
                receipts[0]["snapshot_id"] in {item["id"] for item in snapshots()})
        docker("stop", "--time", "10", name)
        checked(mode + ": SIGTERM during waiting exits successfully",
                docker("inspect", "--format", "{{.State.ExitCode}}", name).stdout.strip() == "0")
        docker("rm", name)
        checked(mode + ": stopped daemon leaves no container", docker("inspect", name, ok=False).returncode != 0)
        report.setdefault("scheduler", {})[mode] = {"receipt": receipts[0], "waiting": waiting[0]}

    def sql(database, text):
        return docker("exec", "-i", pg, "psql", "-X", "-v", "ON_ERROR_STOP=1", "-U", "backup", "-d", database,
                      "--no-align", "--tuples-only", input_text=text).stdout.strip()

    def snapshots():
        return json.loads(command("snapshots", "--json", entrypoint="restic").stdout)

    def backup():
        output = command("backup").stdout
        records = [json.loads(line) for line in output.splitlines() if line.startswith("{")]
        receipt = next(item for item in records if item.get("state") == "success")
        return receipt["snapshot_id"]

    def raw_snapshot(host, tag, timestamp=None):
        options = ["backup", "--json", "--host", host, "--tag", tag]
        if timestamp:
            options += ["--time", timestamp]
        output = command(*options, "/source", entrypoint="restic").stdout
        records = [json.loads(line) for line in output.splitlines() if line.startswith("{")]
        snapshot_prefix = next(item["snapshot_id"] for item in records if item.get("message_type") == "summary")
        return next(item["id"] for item in snapshots() if item["id"].startswith(snapshot_prefix))

    def restore_and_compare(snapshot, dirname, expected_files, expected_rows):
        command("restore-files", snapshot, "/restore/" + dirname)
        target = restore / dirname
        manifests = list(target.rglob("manifest.json"))
        checked(dirname + ": exactly one manifest", len(manifests) == 1)
        manifest = json.loads(manifests[0].read_text(encoding="utf-8"))
        checked(dirname + ": explicit online consistency", manifest["consistency"] == "online-uncoordinated")
        checked(dirname + ": both databases exported", {item["name"] for item in manifest["databases"]} == {"app", "logdb"})
        checked(dirname + ": restored source contents", files_digest(target / "source") == expected_files)
        for item in manifest["databases"]:
            dump = manifests[0].parent / item["file"]
            checked(dirname + ": " + item["name"] + " checksum",
                    hashlib.sha256(dump.read_bytes()).hexdigest() == item["sha256"])
            database = dirname.replace("-", "_") + "_" + item["name"]
            sql("postgres", 'CREATE DATABASE "' + database + '";')
            docker("cp", dump, pg + ":/tmp/restore.dump")
            docker("exec", pg, "pg_restore", "--no-password", "--exit-on-error", "--no-owner", "--no-acl",
                   "-U", "backup", "--dbname", database, "/tmp/restore.dump")
            query = "SELECT id,note,balance,encode(payload,'hex') FROM ledger ORDER BY id;" if item["name"] == "app" else "SELECT message FROM usage;"
            checked(dirname + ": " + item["name"] + " actual database restore",
                    sql(database, query) == expected_rows[item["name"]])
            if item["name"] == "app":
                checked(dirname + ": restored table indexes",
                        sql(database, "SELECT indexname FROM pg_indexes WHERE tablename='ledger' ORDER BY indexname;")
                        == "ledger_note\nledger_pkey")

    try:
        docker("image", "inspect", args.image)
        docker("network", "create", "--label", "newapi.backup.test=" + prefix, network)
        network_created = True
        for volume in (state, cache, pg_data, objects):
            docker("volume", "create", "--label", "newapi.backup.test=" + prefix, volume)
            volumes.append(volume)
        containers.append(pg)
        docker("run", "-d", "--name", pg, "--label", "newapi.backup.test=" + prefix, "--network", network, "-e", "POSTGRES_USER=backup", "-e",
               "POSTGRES_PASSWORD=" + pg_password, "--mount", f"type=volume,src={pg_data},dst=/var/lib/postgresql/data",
               args.postgres_image)
        containers.append(minio)
        # rclone's Alibaba provider uses virtual-host bucket addressing. Give the
        # isolated MinIO server that DNS alias/domain to exercise the same form.
        docker("run", "-d", "--name", minio, "--label", "newapi.backup.test=" + prefix, "--network", network,
               "--network-alias", "backups." + minio, "--user", "0", "-e", "MINIO_ROOT_USER=backup-test",
               "-e", "MINIO_ROOT_PASSWORD=" + oss_secret, "-e", "MINIO_DOMAIN=" + minio,
               "--mount", f"type=volume,src={objects},dst=/bitnami/minio/data",
               args.minio_image)
        wait_for(lambda: docker("exec", pg, "pg_isready", "-h", "127.0.0.1", "-U", "backup", ok=False).returncode == 0, "PostgreSQL")
        wait_for(lambda: command("mkdir", "oss:backups", entrypoint="rclone", ok=False).returncode == 0, "MinIO bucket")
        sql("postgres", "CREATE DATABASE app; CREATE DATABASE logdb;")
        sql("app", "CREATE TABLE ledger(id integer PRIMARY KEY, note text NOT NULL, balance numeric(18,4) CHECK(balance>=0), payload bytea); "
            "INSERT INTO ledger VALUES(1,'中文附件',123.4567,decode('0001ff','hex')); CREATE INDEX ledger_note ON ledger(note);")
        sql("logdb", "CREATE TABLE usage(message text); INSERT INTO usage VALUES('usage log');")
        query = "SELECT id,note,balance,encode(payload,'hex') FROM ledger ORDER BY id;"
        initial_rows = {"app": sql("app", query), "logdb": sql("logdb", "SELECT message FROM usage;")}
        initial_files = files_digest(source)
        report["versions"] = {
            "postgres": sql("postgres", "SHOW server_version;"),
            "pg_dump": command("--version", entrypoint="pg_dump").stdout.strip(),
            "restic": command("version", entrypoint="restic").stdout.strip(),
            "rclone": command("version", entrypoint="rclone").stdout.splitlines()[0],
        }
        initialization = command("init", ok=False)
        if initialization.returncode:
            diagnostic = command("init", entrypoint="restic", overrides={"RCLONE_CONFIG_OSS_PROVIDER": "Alibaba"}, ok=False)
            raise RuntimeError("Runner initialization failed: " + initialization.stderr + "\nRaw isolated backend diagnostic: "
                               + diagnostic.stdout + diagnostic.stderr)
        first = backup()
        checked("successful backup has exactly one snapshot", len(snapshots()) == 1)
        command("health")
        checked("health succeeds after backup")
        command("check")
        checked("full encrypted repository integrity check")
        checked("backup did not change source files", files_digest(source) == initial_files)
        restore_and_compare(first, "first", initial_files, initial_rows)
        # MinIO persists encrypted Restic objects. Fixture config/data plaintext must
        # not appear anywhere in those object files, including metadata sidecars.
        inspect_code = (
            "from pathlib import Path; import sys; needle=sys.argv[1].encode(); "
            "assert not any(needle in p.read_bytes() for p in Path('/objects').rglob('*') if p.is_file())"
        )
        docker("run", "--rm", "--entrypoint", "python3", "--mount", f"type=volume,src={objects},dst=/objects,readonly",
               args.image, "-c", inspect_code, marker)
        checked("object storage contains no fixture secret plaintext")
        (source / "data" / "attachment.txt").write_text("second version", encoding="utf-8")
        sql("app", "INSERT INTO ledger VALUES(2,'second',0.0000,decode('abcd','hex'));")
        second_files = files_digest(source)
        second_rows = {**initial_rows, "app": sql("app", query)}
        second = backup()
        checked("changed backup retains first snapshot", {item["id"] for item in snapshots()} == {first, second})
        restore_and_compare(second, "second", second_files, second_rows)
        result = command("backup", overrides={"BACKUP_DB_PASSWORD": "wrong-local-password"}, ok=False)
        checked("database authentication failure fails backup", result.returncode != 0)
        checked("database credential is not echoed", "wrong-local-password" not in result.stdout + result.stderr)
        checked("failed backup marks health unhealthy", command("health", ok=False).returncode != 0)
        checked("database failure preserves previous snapshots", {item["id"] for item in snapshots()} == {first, second})
        wrong_oss_secret = "wrong-local-oss-" + secrets.token_hex(12)
        failure_started = time.monotonic()
        result = command("backup", overrides={"BACKUP_OSS_ACCESS_KEY_SECRET": wrong_oss_secret,
                                               "BACKUP_TIMEOUT_SECONDS": "20"}, ok=False)
        failure_duration = time.monotonic() - failure_started
        failed_container = containers[-1]
        checked("OSS authentication failure fails backup", result.returncode != 0)
        checked("OSS credential is not echoed", wrong_oss_secret not in result.stdout + result.stderr)
        checked("OSS failure respects overall timeout and termination grace", failure_duration < 50)
        checked("failed OSS job leaves no running container", docker("inspect", failed_container, ok=False).returncode != 0)
        checked("OSS failure marks health unhealthy", command("health", ok=False).returncode != 0)
        report["oss_failure"] = {"duration_seconds": round(failure_duration, 2), "exit_code": result.returncode,
                                 "output": result.stdout + result.stderr}
        checked("OSS failure preserves previous snapshots", {item["id"] for item in snapshots()} == {first, second})
        correct_password = password_file.read_text(encoding="ascii")
        wrong_restic_password = "wrong-local-restic-" + secrets.token_hex(12)
        try:
            password_file.write_text(wrong_restic_password, encoding="ascii")
            result = command("backup", ok=False)
        finally:
            password_file.write_text(correct_password, encoding="ascii")
        checked("wrong repository password fails backup", result.returncode != 0)
        checked("repository password is not echoed", wrong_restic_password not in result.stdout + result.stderr)
        checked("wrong password preserves previous snapshots", {item["id"] for item in snapshots()} == {first, second})
        result = command("backup", overrides={"BACKUP_MIN_FREE_MB": "2147483647"}, ok=False)
        checked("insufficient disk rejects backup", result.returncode != 0)
        checked("disk failure preserves previous snapshots", len(snapshots()) == 2)
        for snapshot, destination in (("latest", "/restore/latest"), (first, "/restore/first"), (first, "/source/new")):
            checked("restore guard rejects " + destination, command("restore-files", snapshot, destination, ok=False).returncode != 0)
        command("retention")
        checked("retention preview does not delete snapshots", len(snapshots()) == 2)
        checked("retention apply rejected after failed backup", command("retention", "--apply", ok=False).returncode != 0)
        checked("test source data remains unchanged", files_digest(source) == second_files)
        third = backup()
        # Fill two calendar-day buckets: when only one day exists, Restic also
        # keeps the oldest snapshot to maximize history, which is intended.
        historical = raw_snapshot("integration", "new-api-test", "2020-01-01 00:00:00")
        foreign = raw_snapshot("another-deployment", "another-tag")
        checked("retention fixture spans two days and includes a foreign snapshot", len(snapshots()) == 5)
        report["before_retention"] = snapshots()
        report["retention_output"] = command("retention", "--apply", overrides={"BACKUP_KEEP_DAILY": "2", "BACKUP_KEEP_WEEKLY": "0", "BACKUP_KEEP_MONTHLY": "0"}).stdout
        report["after_retention"] = snapshots()
        checked("retention apply deletes old owned snapshot and preserves foreign snapshot",
                {item["id"] for item in report["after_retention"]} == {historical, second, third, foreign})
        command("check")
        checked("full integrity check after retention prune")
        restore_and_compare(third, "retained", second_files, second_rows)
        checked("health remains successful after retention", command("health").returncode == 0)
        daemon_smoke("daily")
        daemon_smoke("interval")
        checked("scheduled backups did not modify source files", files_digest(source) == second_files)
        report["snapshot_ids"] = [historical, second, third, foreign]
        report["result"] = "passed"
    except Exception as error:
        report["result"] = "failed"
        report["error"] = str(error)
        raise
    finally:
        for container in reversed(containers):
            label = docker("inspect", "--format", '{{index .Config.Labels "newapi.backup.test"}}', container, ok=False)
            if label.returncode == 0 and label.stdout.strip() == prefix:
                docker("rm", "-f", container, ok=False)
        for volume in reversed(volumes):
            docker("volume", "rm", volume, ok=False)
        if network_created:
            docker("network", "rm", network, ok=False)
        (root / "report.json").write_text(json.dumps(report, indent=2, ensure_ascii=False), encoding="utf-8")
        print("Report: " + str(root / "report.json"), flush=True)
    return 0


if __name__ == "__main__":
    sys.exit(main())
