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


APP_ROLE = "new-api:application:v2"
LOG_ROLE = "new-api:logs:v1"


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
    parser.add_argument("--storage-provider", choices=("alibaba", "minio", "s3-alibaba"), default="alibaba")
    args = parser.parse_args()
    prefix = "newapi-backup-it-" + secrets.token_hex(5)
    root = Path(__file__).resolve().parents[2] / ".local-tests" / prefix
    source = root / "source"
    restore = root / "restore"
    for directory in (source / "data", source / "logs", source / "config", restore):
        directory.mkdir(parents=True)
    marker = "private-fixture-" + secrets.token_hex(24)
    (source / "data" / "attachment.txt").write_text("附件内容 " + marker, encoding="utf-8")
    removed_file = source / "data" / "removed-after-first.txt"
    removed_file.write_text("present only in the first snapshot", encoding="utf-8")
    (source / "logs" / "application.log").write_text("test log\n", encoding="utf-8")
    (source / "config" / "deploy.env").write_text("SESSION_SECRET=" + marker, encoding="utf-8")
    password_file = root / "restic-password"
    password_file.write_text(secrets.token_urlsafe(32), encoding="ascii")
    pg_password = secrets.token_urlsafe(24)
    # Keep this positional mc argument from being parsed as an option.
    oss_secret = "backup-" + secrets.token_urlsafe(24)
    oss_admin_secret = secrets.token_urlsafe(24)
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
    report = {"image": args.image, "storage_provider": args.storage_provider, "workspace": str(root), "checks": []}

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
        "RCLONE_CONFIG_OSS_NO_CHECK_BUCKET": "true",
        "RESTIC_REPOSITORY": "rclone:oss:backups/newapi", "RESTIC_PASSWORD_FILE": "/run/secrets/restic-password",
        "RESTIC_CACHE_DIR": "/cache",
    }
    admin_env = {"RCLONE_CONFIG_OSS_ACCESS_KEY_ID": "backup-admin",
                 "RCLONE_CONFIG_OSS_SECRET_ACCESS_KEY": oss_admin_secret}

    def command(*cli, overrides=None, entrypoint=None, ok=True, detached=False,
                state_volume=None, cache_volume=None):
        effective = rclone_env if entrypoint == "restic" or entrypoint == "rclone" else env
        effective = {**effective, **(overrides or {})}
        if args.storage_provider != "alibaba" and entrypoint not in ("restic", "rclone"):
            # Run the same safety contracts through the generic S3 configuration.
            # Raw inspection keeps its own alias but reads the identical objects.
            effective["BACKUP_REPOSITORY"] = effective["BACKUP_REPOSITORY"].replace("rclone:oss:", "rclone:s3:", 1)
            for name in ("ENDPOINT", "ACCESS_KEY_ID", "ACCESS_KEY_SECRET"):
                effective["BACKUP_S3_" + name] = effective.pop("BACKUP_OSS_" + name)
            effective.pop("BACKUP_ALLOW_INSECURE_OSS", None)
            effective.pop("RCLONE_CONFIG_OSS_FORCE_PATH_STYLE", None)
            effective.update({"BACKUP_S3_PROVIDER": "Minio" if args.storage_provider == "minio" else "Alibaba", "BACKUP_S3_REGION": "us-east-1",
                              "BACKUP_S3_FORCE_PATH_STYLE": "true", "BACKUP_ALLOW_INSECURE_S3": "true"})
        name = prefix + "-job-" + secrets.token_hex(3)
        containers.append(name)
        options = ["run", *( ["-d", "--init"] if detached else ["--rm"]), "--name", name,
                   "--label", "newapi.backup.test=" + prefix,
                   "--network", network, "--read-only", "--cap-drop", "ALL",
                   "--security-opt", "no-new-privileges:true", "--tmpfs", "/tmp:size=64m,mode=1777",
                   "--memory", "512m", "--cpus", "0.5"]
        if effective.get("BACKUP_TEMP_DIR") == "/restic-tmp":
            options += ["--tmpfs", "/restic-tmp:size=256m,mode=0700"]
        for key, value in effective.items():
            options += ["-e", key + "=" + value]
        for mount in (
            f"type=bind,src={source},dst=/source,readonly",
            f"type=bind,src={restore},dst=/restore",
            f"type=bind,src={password_file},dst=/run/secrets/restic-password,readonly",
            f"type=volume,src={state_volume or state},dst=/state",
            f"type=volume,src={cache_volume or cache},dst=/cache",
        ):
            options += ["--mount", mount]
        if entrypoint:
            options += ["--entrypoint", entrypoint]
        return docker(*options, args.image, *cli, ok=ok)

    def daemon_smoke(mode, *, overrides=None, repository=None, scenario=None,
                     state_volume=None, cache_volume=None):
        scope = scenario or mode
        now = dt.datetime.now(dt.timezone.utc)
        daily_times = [(now + dt.timedelta(hours=hours)).strftime("%H:%M") for hours in (2, 4)]
        settings = {"TZ": "UTC", "BACKUP_SCHEDULE_MODE": mode,
                    "BACKUP_TIMES": ",".join(daily_times), "BACKUP_INTERVAL": "1m"}
        settings.update(overrides or {})
        command("run", overrides=settings, detached=True, state_volume=state_volume, cache_volume=cache_volume)
        name = containers[-1]
        wait_for(lambda: '"state": "waiting"' in docker("logs", name).stdout, scope + " daemon initial backup")
        logs = docker("logs", name).stdout
        report.setdefault("scheduler", {})[scope] = {"logs": logs}
        records = [json.loads(line) for line in logs.splitlines() if line.startswith("{")]
        receipts = [item for item in records if item.get("state") == "success"]
        waiting = [item for item in records if item.get("state") == "waiting"]
        checked(scope + ": startup creates exactly one successful backup", len(receipts) == 1)
        checked(scope + ": daemon enters configured schedule mode", len(waiting) == 1 and waiting[0]["mode"] == mode)
        next_at = dt.datetime.fromisoformat(waiting[0]["next_run_at"])
        checked(scope + ": next backup is scheduled in the future", next_at > dt.datetime.now(dt.timezone.utc))
        if mode == "daily":
            checked(scope + ": selects next point from multiple configured times", next_at.strftime("%H:%M") == daily_times[0])
        else:
            finished_at = dt.datetime.fromisoformat(receipts[0]["finished_at"])
            checked(scope + ": schedule follows completed backup", 60 <= (next_at - finished_at).total_seconds() < 65)
        docker("exec", name, "python3", "/opt/backup/runner.py", "health")
        checked(scope + ": daemon health succeeds while waiting")
        checked(scope + ": scheduled backup exists in repository",
                receipts[0]["snapshot_id"] in {item["id"] for item in snapshots(repository)})
        docker("stop", "--time", "10", name)
        checked(scope + ": SIGTERM during waiting exits successfully",
                docker("inspect", "--format", "{{.State.ExitCode}}", name).stdout.strip() == "0")
        docker("rm", name)
        checked(scope + ": stopped daemon leaves no container", docker("inspect", name, ok=False).returncode != 0)
        report["scheduler"][scope] = {"receipt": receipts[0], "waiting": waiting[0]}
        return receipts[0]["snapshot_id"]

    def sql(database, text):
        return docker("exec", "-i", pg, "psql", "-X", "-v", "ON_ERROR_STOP=1", "-U", "backup", "-d", database,
                      "--no-align", "--tuples-only", input_text=text).stdout.strip()

    def repository_snapshots(repository=None):
        overrides = {"RESTIC_REPOSITORY": repository} if repository else None
        return json.loads(command("snapshots", "--json", entrypoint="restic", overrides=overrides).stdout)

    def snapshots(repository=None):
        return [item for item in repository_snapshots(repository)
                if APP_ROLE in item.get("tags", []) and LOG_ROLE not in item.get("tags", [])]

    def logs_snapshots(repository=None):
        return [item for item in repository_snapshots(repository) if LOG_ROLE in item.get("tags", [])]

    def backup(**settings):
        output = command("backup", **settings).stdout
        records = [json.loads(line) for line in output.splitlines() if line.startswith("{")]
        receipt = next(item for item in records if item.get("state") == "success")
        return receipt["snapshot_id"]

    def repository_id(repository=None):
        overrides = {"RESTIC_REPOSITORY": repository} if repository else None
        return json.loads(command("cat", "config", entrypoint="restic", overrides=overrides).stdout)["id"]

    def new_state(suffix):
        pair = (prefix + "-" + suffix + "-state", prefix + "-" + suffix + "-cache")
        for volume in pair:
            docker("volume", "create", "--label", "newapi.backup.test=" + prefix, volume)
            volumes.append(volume)
        return {"state_volume": pair[0], "cache_volume": pair[1]}

    def raw_snapshot(host, tag, timestamp=None, *, repository=None):
        options = ["backup", "--json", "--host", host, "--tag", tag,
                   "--tag", APP_ROLE, "--exclude", "/source/logs"]
        if timestamp:
            options += ["--time", timestamp]
        overrides = {"RESTIC_REPOSITORY": repository} if repository else None
        output = command(*options, "/source", entrypoint="restic", overrides=overrides).stdout
        records = [json.loads(line) for line in output.splitlines() if line.startswith("{")]
        snapshot_prefix = next(item["snapshot_id"] for item in records if item.get("message_type") == "summary")
        return next(item["id"] for item in snapshots(repository) if item["id"].startswith(snapshot_prefix))

    def restore_and_compare(snapshot, dirname, expected_files, expected_rows):
        command("restore-files", snapshot, "/restore/" + dirname)
        target = restore / dirname
        manifests = list(target.rglob("manifest.json"))
        checked(dirname + ": exactly one manifest", len(manifests) == 1)
        manifest = json.loads(manifests[0].read_text(encoding="utf-8"))
        checked(dirname + ": application manifest records the separate log capture",
                manifest["format"] == 2 and manifest["source"] == "/source"
                and manifest["logs"]["path"] == "/source/logs" and not manifest["logs"]["missing"]
                and manifest["logs"]["snapshot_id"] in {item["id"] for item in logs_snapshots()})
        checked(dirname + ": explicit online consistency", manifest["consistency"] == "online-uncoordinated")
        checked(dirname + ": both databases exported", {item["name"] for item in manifest["databases"]} == {"app", "logdb"})
        application_files = {name: digest for name, digest in expected_files.items() if not name.startswith("logs/")}
        checked(dirname + ": application restore excludes log files",
                files_digest(target / "source") == application_files and not (target / "source" / "logs").exists())
        command("restore-logs", manifest["logs"]["snapshot_id"], "/restore/" + dirname + "-logs")
        expected_logs = {name.removeprefix("logs/"): digest for name, digest in expected_files.items() if name.startswith("logs/")}
        checked(dirname + ": independently restored log files match their capture",
                files_digest(restore / (dirname + "-logs") / "source" / "logs") == expected_logs
                and not (restore / (dirname + "-logs") / "source" / "data").exists())
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
               "--network-alias", "backups." + minio, "--network-alias", "absent." + minio,
               "--user", "0", "-e", "MINIO_ROOT_USER=backup-admin",
               "-e", "MINIO_ROOT_PASSWORD=" + oss_admin_secret, "-e", "MINIO_DOMAIN=" + minio,
               "--mount", f"type=volume,src={objects},dst=/bitnami/minio/data",
               args.minio_image)
        wait_for(lambda: docker("exec", pg, "pg_isready", "-h", "127.0.0.1", "-U", "backup", ok=False).returncode == 0, "PostgreSQL")
        wait_for(lambda: command("mkdir", "oss:backups", entrypoint="rclone", ok=False,
                                 overrides={**admin_env, "RCLONE_CONFIG_OSS_NO_CHECK_BUCKET": "false"}).returncode == 0,
                 "MinIO bucket")
        # Match a pre-created OSS bucket with an object-scoped RAM user. The
        # backup account must never need permission to create a bucket.
        policy = {"Version": "2012-10-17", "Statement": [
            {"Effect": "Allow", "Action": ["s3:GetBucketLocation", "s3:ListBucket", "s3:ListBucketMultipartUploads"],
             "Resource": ["arn:aws:s3:::backups"]},
            {"Effect": "Allow", "Action": ["s3:GetObject", "s3:PutObject", "s3:DeleteObject",
                                             "s3:AbortMultipartUpload", "s3:ListMultipartUploadParts"],
             "Resource": ["arn:aws:s3:::backups/*"]},
            {"Effect": "Deny", "Action": ["s3:CreateBucket"], "Resource": ["arn:aws:s3:::*"]},
        ]}
        docker("exec", "-i", minio, "/bin/sh", "-c", "cat > /tmp/backup-policy.json",
               input_text=json.dumps(policy))
        # Bitnami configures its own mc as the minio user during startup. Our
        # root-owned test client must not replace that client's config file.
        mc = ["exec", "--env", "MC_HOST_local=http://backup-admin:" + oss_admin_secret + "@127.0.0.1:9000",
              minio, "/opt/bitnami/minio-client/bin/mc", "--config-dir", "/tmp/backup-test-mc"]
        docker(*mc, "admin", "user", "add", "local", "backup-test", oss_secret)
        docker(*mc, "admin", "policy", "create", "local", "backup-objects", "/tmp/backup-policy.json")
        docker(*mc, "admin", "policy", "attach", "local", "backup-objects", "--user", "backup-test")
        result = command("init", entrypoint="restic", ok=False,
                         overrides={"RESTIC_REPOSITORY": "rclone:oss:backups/bucket-check-regression",
                                    "RCLONE_CONFIG_OSS_NO_CHECK_BUCKET": "false"})
        checked("backup account cannot initialize when rclone attempts to create the existing bucket",
                result.returncode != 0 and "accessdenied" in (result.stdout + result.stderr).lower())
        checked("denied bucket creation leaves the repository prefix empty",
                json.loads(command("lsjson", "oss:backups/bucket-check-regression", "--recursive", entrypoint="rclone").stdout) == [])
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
        # Commit a real encrypted repository, then simulate losing only init's
        # reply. Read-back must authenticate it without another init attempt.
        lost_reply_repository = "rclone:oss:backups/init-response-lost"
        lost_reply_state = new_state("init-response-lost")
        lost_reply_code = """
import json
import runner

config = runner.Config()
config.prepare()
job = runner.Job(config)
original_command = job.command
init_calls = 0

def lose_init_reply(stage, args, env=None, monitor=True):
    global init_calls
    if args[0] == "restic" and args[-1] == "init":
        init_calls += 1
        original_command(stage, args, env, monitor)
        raise runner.BackupError("simulated lost initialization reply", retryable=True, exit_code=1)
    return original_command(stage, args, env, monitor)

job.command = lose_init_reply
with runner.job_lock(config):
    repository_id = job.ensure_repository()
assert init_calls == 1, "initialization was repeated after a lost reply"
print(json.dumps({"verified_repository_id": repository_id, "init_calls": init_calls}))
"""
        result = command("-c", lost_reply_code, entrypoint="python3",
                         overrides={"BACKUP_REPOSITORY": lost_reply_repository, "BACKUP_RETRIES": "2",
                                    "BACKUP_RETRY_DELAY_SECONDS": "1"}, **lost_reply_state)
        verified = json.loads(result.stdout.splitlines()[-1])
        checked("lost initialization reply is accepted only after reading the real repository",
                verified["init_calls"] == 1 and verified["verified_repository_id"] == repository_id(lost_reply_repository))
        recovered_snapshot = backup(overrides={"BACKUP_REPOSITORY": lost_reply_repository}, **lost_reply_state)
        checked("repository recovered after a lost init reply accepts a real backup",
                {item["id"] for item in snapshots(lost_reply_repository)} == {recovered_snapshot})
        # Incompressible data queues real encrypted packs beyond the 64 MiB
        # /tmp limit; staging must use the disk-backed cache volume instead.
        capacity_file = source / "data" / "pack-capacity.bin"
        capacity_file.write_bytes(hashlib.shake_256(b"backup-pack-capacity-regression").digest(160 * 1024 * 1024))
        capacity_hash = hashlib.sha256(capacity_file.read_bytes()).hexdigest()
        capacity_repository = "rclone:oss:backups/pack-capacity"
        capacity_state = new_state("pack-capacity")
        capacity_settings = {"BACKUP_REPOSITORY": capacity_repository, "BACKUP_MAX_CACHE_MB": "256", "BACKUP_BW_LIMIT": "8M"}
        try:
            capacity_snapshot = backup(overrides=capacity_settings, **capacity_state)
            checked("160 MiB backup succeeds with 64 MiB tmpfs", capacity_snapshot in {item["id"] for item in snapshots(capacity_repository)})
            command("restore-files", capacity_snapshot, "/restore/pack-capacity", overrides=capacity_settings, **capacity_state)
            checked("large pack staging preserves restored file bytes",
                    hashlib.sha256((restore / "pack-capacity" / "source" / "data" / capacity_file.name).read_bytes()).hexdigest() == capacity_hash)
            memory_repository = "rclone:oss:backups/memory-pack-capacity"
            memory_state = new_state("memory-pack-capacity")
            memory_settings = {**capacity_settings, "BACKUP_REPOSITORY": memory_repository,
                               "BACKUP_TEMP_DIR": "/restic-tmp", "RESTIC_PROGRESS_FPS": "60"}
            memory_snapshot = backup(overrides=memory_settings, **memory_state)
            checked("160 MiB backup succeeds with a dedicated memory staging mount",
                    memory_snapshot in {item["id"] for item in snapshots(memory_repository)})
            command("restore-files", memory_snapshot, "/restore/memory-pack-capacity", overrides=memory_settings, **memory_state)
            checked("memory pack staging preserves restored file bytes",
                    hashlib.sha256((restore / "memory-pack-capacity" / "source" / "data" / capacity_file.name).read_bytes()).hexdigest() == capacity_hash)
        finally:
            capacity_file.unlink()
        first = backup()
        original_repository_id = repository_id()
        checked("first backup automatically initializes an empty repository", bool(original_repository_id))
        command("init")
        command("init")
        checked("repeated initialization preserves the existing repository identity", repository_id() == original_repository_id)
        checked("successful application backup has exactly one application snapshot", len(snapshots()) == 1)
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
        # Parent reuse must still scan the current tree and record deletions.
        removed_file.unlink()
        sql("app", "INSERT INTO ledger VALUES(2,'second',0.0000,decode('abcd','hex'));")
        second_files = files_digest(source)
        second_rows = {**initial_rows, "app": sql("app", query)}
        second = backup()
        second_snapshots = snapshots()
        checked("changed backup retains first snapshot", {item["id"] for item in second_snapshots} == {first, second})
        checked("changed backup reuses the first snapshot despite a different export directory",
                next(item for item in second_snapshots if item["id"] == second).get("parent") == first)
        # Exact file-tree comparison also rejects resurrecting removed_file.
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

        # Updating a node, or recreating its local state/cache, must keep the
        # authenticated existing repository and all of its recovery points.
        before_redeploy = {item["id"] for item in snapshots()}
        fresh = new_state("fresh-existing")
        fresh_snapshot = backup(**fresh)
        checked("new node without local state opens the existing repository",
                repository_id() == original_repository_id)
        checked("new node preserves all preexisting snapshots",
                {item["id"] for item in snapshots()} == before_redeploy | {fresh_snapshot})

        auto_repository = "rclone:oss:backups/auto-created"
        auto_settings = {"BACKUP_REPOSITORY": auto_repository}
        auto_state = new_state("auto-created")
        auto_first = daemon_smoke("daily", overrides=auto_settings, repository=auto_repository,
                                  scenario="auto-init first deployment", **auto_state)
        auto_repository_id = repository_id(auto_repository)
        checked("first deployment creates its own repository identity",
                auto_repository_id != original_repository_id)
        auto_second = daemon_smoke("daily", overrides=auto_settings, repository=auto_repository,
                                   scenario="auto-init redeployment", **auto_state)
        checked("redeployment keeps the repository identity", repository_id(auto_repository) == auto_repository_id)
        checked("redeployment preserves the previous recovery point",
                {item["id"] for item in snapshots(auto_repository)} == {auto_first, auto_second})

        # No previous repository pin exists in this state directory. This proves
        # the remote nonempty guard works independently of local history.
        guard_state = new_state("empty-guard")
        manual_repository = "rclone:oss:backups/manual-only"
        result = command("backup", overrides={"BACKUP_REPOSITORY": manual_repository,
                                               "BACKUP_AUTO_INIT": "false"}, ok=False, **guard_state)
        checked("automatic initialization can be explicitly disabled", result.returncode != 0)
        checked("disabled initialization leaves the empty prefix untouched",
                json.loads(command("lsjson", "oss:backups/manual-only", "--recursive", entrypoint="rclone").stdout) == [])
        inspect_settings = {"BACKUP_REPOSITORY": "rclone:oss:backups/inspect-only"}
        for operation in ("snapshots", "check"):
            checked(operation + " never initializes a missing repository",
                    command(operation, overrides=inspect_settings, ok=False, **guard_state).returncode != 0)
        checked("restore never initializes a missing repository",
                command("restore-files", first, "/restore/inspect-only", overrides=inspect_settings,
                        ok=False, **guard_state).returncode != 0)
        checked("read operations leave the missing repository and restore destination untouched",
                json.loads(command("lsjson", "oss:backups/inspect-only", "--recursive", entrypoint="rclone").stdout) == []
                and not (restore / "inspect-only").exists())
        command("copyto", "/opt/backup/Dockerfile", "oss:backups/damaged/keep.txt", entrypoint="rclone")
        evidence = command("cat", "oss:backups/damaged/keep.txt", entrypoint="rclone").stdout
        result = command("backup", overrides={"BACKUP_REPOSITORY": "rclone:oss:backups/damaged"},
                         ok=False, **guard_state)
        checked("a nonempty prefix without repository config is not initialized", result.returncode != 0)
        damaged_files = json.loads(command("lsjson", "oss:backups/damaged", "--recursive", entrypoint="rclone").stdout)
        checked("nonempty guard preserves the original object without adding repository files",
                {item["Path"] for item in damaged_files} == {"keep.txt"}
                and command("cat", "oss:backups/damaged/keep.txt", entrypoint="rclone").stdout == evidence)

        result = command("backup", overrides={"BACKUP_REPOSITORY": "rclone:oss:backups/typo-empty"},
                         ok=False, **auto_state)
        checked("a deployed node refuses to silently initialize a changed empty repository", result.returncode != 0)
        checked("repository location guard leaves the new prefix untouched",
                json.loads(command("lsjson", "oss:backups/typo-empty", "--recursive", entrypoint="rclone").stdout) == [])

        # Corrupt only the dedicated auto-init fixture, then restore its original
        # config object. Neither the main repository nor any source files change.
        command("copyto", "oss:backups/auto-created/config", "/restore/auto-config.backup", entrypoint="rclone")
        before_config_loss = json.loads(command("lsjson", "oss:backups/auto-created", "--recursive", entrypoint="rclone").stdout)
        expected_paths = {item["Path"] for item in before_config_loss} - {"config"}
        command("deletefile", "oss:backups/auto-created/config", entrypoint="rclone")
        result = command("backup", overrides=auto_settings, ok=False, **auto_state)
        checked("known repository config loss is reported instead of initialized", result.returncode != 0)
        result = command("backup", overrides=auto_settings, ok=False, **guard_state)
        checked("fresh local state also refuses the damaged nonempty repository", result.returncode != 0)
        after_config_loss = json.loads(command("lsjson", "oss:backups/auto-created", "--recursive", entrypoint="rclone").stdout)
        checked("missing-config failures preserve repository objects",
                {item["Path"] for item in after_config_loss} == expected_paths)
        command("copyto", "/restore/auto-config.backup", "oss:backups/auto-created/config", entrypoint="rclone")
        checked("restoring the original config recovers the original repository identity",
                repository_id(auto_repository) == auto_repository_id)
        auto_recovered = backup(overrides=auto_settings, **auto_state)
        checked("recovered repository remains writable without losing earlier snapshots",
                {item["id"] for item in snapshots(auto_repository)} == {auto_first, auto_second, auto_recovered})
        command("check", "--read-data", entrypoint="restic", overrides={"RESTIC_REPOSITORY": auto_repository})
        checked("repository data remains intact after initialization guard failures")

        buckets_before = json.loads(command("lsjson", "oss:", "--dirs-only", entrypoint="rclone", overrides=admin_env).stdout)
        result = command("backup", overrides={"BACKUP_REPOSITORY": "rclone:oss:absent/newapi",
                                               "BACKUP_TIMEOUT_SECONDS": "20"}, ok=False, **guard_state)
        checked("missing bucket fails instead of being automatically created", result.returncode != 0)
        buckets_after = json.loads(command("lsjson", "oss:", "--dirs-only", entrypoint="rclone", overrides=admin_env).stdout)
        checked("automatic initialization does not create a bucket",
                {item["Name"] for item in buckets_after} == {item["Name"] for item in buckets_before})
        command("init", overrides={"BACKUP_REPOSITORY": manual_repository, "BACKUP_AUTO_INIT": "false"}, **guard_state)
        manual_id = repository_id(manual_repository)
        command("init", overrides={"BACKUP_REPOSITORY": manual_repository, "BACKUP_AUTO_INIT": "false"}, **guard_state)
        checked("manual initialization remains available and idempotent when automatic init is disabled",
                repository_id(manual_repository) == manual_id and snapshots(manual_repository) == [])
        checked("initialization and redeployment tests leave application files unchanged",
                files_digest(source) == second_files)
        report["auto_initialization"] = {"original_repository_id": original_repository_id,
                                          "new_repository_id": auto_repository_id,
                                          "snapshot_ids": [auto_first, auto_second, auto_recovered]}
        report["snapshot_ids"] = [historical, second, third, foreign]

        # A separate repository keeps ordinary retention fixtures independent of
        # permanent log history. Each view must restore a whole point in time,
        # never concatenate file versions by name or resurrect deleted files.
        log_repository = "rclone:oss:backups/log-history"
        log_state = new_state("log-history")
        log_settings = {"BACKUP_REPOSITORY": log_repository}
        log_directory = source / "logs"
        original_log = (log_directory / "application.log").read_bytes()
        log_cases = {}

        def archive_log_case(name):
            expected = files_digest(log_directory) if log_directory.exists() else None
            output = command("backup", overrides=log_settings, **log_state).stdout
            records = [json.loads(line) for line in output.splitlines() if line.startswith("{")]
            main_snapshot = next(item["snapshot_id"] for item in records if item.get("state") == "success")
            if expected is None:
                checked(name + ": missing log directory is explicit", any(item.get("state") == "logs_missing" for item in records))
                log_cases[name] = {"snapshot_id": None, "expected": None, "main_snapshot": main_snapshot}
                return None
            archived = next(item for item in records if item.get("state") == "logs_archived")
            log_cases[name] = {"snapshot_id": archived["snapshot_id"], "expected": expected, "main_snapshot": main_snapshot}
            return archived

        try:
            (log_directory / "application.log").unlink()
            active_log = log_directory / "audit.log"
            log_bytes = hashlib.shake_256(b"permanent-log-deduplication-regression").digest(16 * 1024 * 1024)
            active_log.write_bytes(log_bytes)
            archive_log_case("initial")
            with active_log.open("ab") as stream:
                stream.write(hashlib.shake_256(b"appended-log-content").digest(64 * 1024))
            appended = archive_log_case("appended")
            checked("appended log reuses existing encrypted content blocks",
                    0 <= appended["data_added"] < len(log_bytes)
                    and appended["total_bytes_processed"] == len(log_bytes) + 64 * 1024)
            active_log.rename(log_directory / "audit.log.1")
            active_log.write_bytes(b"new active log after rotation\n")
            archive_log_case("rotated")
            active_log.write_bytes(b"truncated\n")
            archive_log_case("truncated")
            active_log.unlink()
            active_log.write_bytes(b"replacement file with the same name\n")
            archive_log_case("recreated")
            active_log.unlink()
            archive_log_case("deleted")
            (log_directory / "audit.log.1").unlink()
            archive_log_case("empty")
            before_missing = {item["id"] for item in logs_snapshots(log_repository)}
            log_directory.rmdir()
            archive_log_case("missing")
            checked("missing log directory does not erase or fabricate a log snapshot",
                    {item["id"] for item in logs_snapshots(log_repository)} == before_missing)
            command("restore-files", log_cases["missing"]["main_snapshot"], "/restore/log-history-missing",
                    overrides=log_settings, **log_state)
            missing_manifest = json.loads(next((restore / "log-history-missing").rglob("manifest.json")).read_text(encoding="utf-8"))
            checked("ordinary restore preserves an explicitly missing log directory",
                    missing_manifest["logs"]["missing"] and missing_manifest["logs"]["snapshot_id"] is None
                    and not (restore / "log-history-missing" / "source" / "logs").exists())

            # A database failure must not discard an already archived log view.
            log_directory.mkdir()
            active_log.write_bytes(b"archived before a database authentication failure\n")
            before_failure = {item["id"] for item in snapshots(log_repository)}
            result = command("backup", overrides={**log_settings, "BACKUP_DB_PASSWORD": "wrong-log-fixture-password"},
                             ok=False, **log_state)
            records = [json.loads(line) for line in result.stdout.splitlines() if line.startswith("{")]
            failed_archive = next(item for item in records if item.get("state") == "logs_archived")
            log_cases["database-failed"] = {"snapshot_id": failed_archive["snapshot_id"], "expected": files_digest(log_directory)}
            checked("database failure preserves independently archived logs without a false application receipt",
                    result.returncode != 0 and not any(item.get("state") == "success" for item in records)
                    and failed_archive["snapshot_id"] in {item["id"] for item in logs_snapshots(log_repository)}
                    and {item["id"] for item in snapshots(log_repository)} == before_failure)
            checked("archived logs do not make a failed database backup healthy",
                    command("health", overrides=log_settings, ok=False, **log_state).returncode != 0)
            archive_log_case("recovered")

            raw_snapshot("integration", "new-api-test", "2020-01-01 00:00:00", repository=log_repository)
            log_history_ids = {item["id"] for item in logs_snapshots(log_repository)}
            main_ids = {item["id"] for item in snapshots(log_repository)}
            command("retention", "--apply", overrides={**log_settings, "BACKUP_KEEP_DAILY": "2",
                                                        "BACKUP_KEEP_WEEKLY": "0", "BACKUP_KEEP_MONTHLY": "0"}, **log_state)
            checked("ordinary retention prunes application snapshots while preserving every log view",
                    {item["id"] for item in logs_snapshots(log_repository)} == log_history_ids
                    and len(snapshots(log_repository)) < len(main_ids))
            listed_logs = json.loads(command("log-snapshots", overrides=log_settings, **log_state).stdout)
            checked("log history listing exposes only archived log recovery points",
                    log_history_ids == {item["id"] for item in listed_logs})
            for name, case in log_cases.items():
                if case["snapshot_id"] is None:
                    continue
                destination = "/restore/log-history-" + name
                command("restore-logs", case["snapshot_id"], destination, overrides=log_settings, **log_state)
                restored_logs = restore / ("log-history-" + name) / "source" / "logs"
                checked(name + ": historical log files restore exactly after pruning",
                        restored_logs.is_dir() and files_digest(restored_logs) == case["expected"])
            checked("application restore rejects a log-only snapshot",
                    command("restore-files", log_cases["initial"]["snapshot_id"], "/restore/log-role-rejected",
                            overrides=log_settings, ok=False, **log_state).returncode != 0
                    and not (restore / "log-role-rejected").exists())
            checked("log restore rejects an application snapshot",
                    command("restore-logs", log_cases["recovered"]["main_snapshot"], "/restore/app-role-rejected",
                            overrides=log_settings, ok=False, **log_state).returncode != 0
                    and not (restore / "app-role-rejected").exists())
            command("check", overrides=log_settings, **log_state)
            checked("permanent log history remains intact after ordinary retention and pruning")
            report["log_history"] = {"repository": log_repository, "log_snapshot_ids": sorted(log_history_ids),
                                     "cases": list(log_cases)}
        finally:
            log_directory.mkdir(exist_ok=True)
            for path in log_directory.iterdir():
                path.unlink()
            (log_directory / "application.log").write_bytes(original_log)
        checked("log history regression restores the original application source tree", files_digest(source) == second_files)
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
