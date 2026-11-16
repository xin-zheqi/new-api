#!/usr/bin/env python3
"""Online PostgreSQL/file backups. This process never controls application containers."""

import argparse
import contextlib
import datetime as dt
import fcntl
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import signal
import subprocess
import sys
import tempfile
import time
from zoneinfo import ZoneInfo, ZoneInfoNotFoundError


class BackupError(Exception):
    """Messages are deliberately safe to persist: never include subprocess stderr."""

    def __init__(self, message, retryable=False, exit_code=None):
        super().__init__(message)
        self.retryable = retryable
        self.exit_code = exit_code


STOPPING = False
MB = 1024 * 1024


def stop(_signum, _frame):
    global STOPPING
    STOPPING = True


def utcnow():
    return dt.datetime.now(dt.timezone.utc).isoformat()


def integer(env, name, default, minimum=1, maximum=2**31 - 1):
    try:
        value = int(env.get(name, str(default)))
    except ValueError:
        raise BackupError(f"Invalid {name}") from None
    if not minimum <= value <= maximum:
        raise BackupError(f"Invalid {name}")
    return value


def secret(env, name, default_file=""):
    filename = env.get(name + "_FILE", default_file)
    if filename:
        try:
            value = Path(filename).read_text().rstrip("\r\n")
        except OSError:
            raise BackupError(f"Cannot read {name}_FILE") from None
    else:
        value = env.get(name, "")
    if not value:
        raise BackupError(f"Missing {name} or {name}_FILE")
    return value


class Config:
    def __init__(self, env=None):
        self.env = dict(os.environ if env is None else env)
        self.state = Path(self.env.get("BACKUP_STATE_DIR", "/state")).resolve()
        self.cache = Path(self.env.get("BACKUP_CACHE_DIR", "/cache")).resolve()
        self.source = Path(self.env.get("BACKUP_SOURCE_DIR", "/source")).resolve()
        for path in (self.state, self.cache):
            if path == self.source or path in self.source.parents or self.source in path.parents:
                raise BackupError("Backup writable directories must be outside sources")
        if self.state == self.cache or self.state in self.cache.parents or self.cache in self.state.parents:
            raise BackupError("State and cache directories must be separate")
        self.timeout = integer(self.env, "BACKUP_TIMEOUT_SECONDS", 7200, maximum=86400)
        self.retries = integer(self.env, "BACKUP_RETRIES", 2, minimum=0, maximum=5)
        self.retry_delay = integer(self.env, "BACKUP_RETRY_DELAY_SECONDS", 10, maximum=300)
        self.min_free = integer(self.env, "BACKUP_MIN_FREE_MB", 1024) * MB
        self.max_dump = integer(self.env, "BACKUP_MAX_DUMP_MB", 4096) * MB
        self.max_cache = integer(self.env, "BACKUP_MAX_CACHE_MB", 512) * MB
        self.host = self.env.get("BACKUP_HOST", "newapi")
        self.tag = self.env.get("BACKUP_TAG", "new-api")
        if not self.host or not self.tag or "," in self.tag:
            raise BackupError("BACKUP_HOST and a single BACKUP_TAG are required")
        try:
            self.zone = ZoneInfo(self.env.get("TZ", "Asia/Shanghai"))
        except (ValueError, ZoneInfoNotFoundError):
            raise BackupError("Invalid TZ") from None
        self.schedule_mode = self.env.get("BACKUP_SCHEDULE_MODE", "daily").strip().lower()
        self.times = ()
        self.interval = 0
        if self.schedule_mode == "daily":
            # Keep existing BACKUP_TIME deployments working; the plural setting wins.
            explicit_times = self.env.get("BACKUP_TIMES", "").strip()
            times = explicit_times or self.env.get("BACKUP_TIME", "03:30")
            pattern = r"[0-9]{1,2}:[0-9]{2}" if explicit_times else r"[0-9]{1,2}:[0-9]{1,2}"
            values = []
            for value in times.split(","):
                value = value.strip()
                if not re.fullmatch(pattern, value):
                    raise BackupError("Invalid BACKUP_TIMES/BACKUP_TIME (expected comma-separated HH:MM)")
                try:
                    hour, minute = map(int, value.split(":"))
                    values.append(dt.time(hour, minute))
                except ValueError:
                    raise BackupError("Invalid BACKUP_TIMES/BACKUP_TIME (expected comma-separated HH:MM)") from None
            self.times = tuple(sorted(set(values)))
        elif self.schedule_mode == "interval":
            value = self.env.get("BACKUP_INTERVAL", "6h").strip().lower()
            match = re.fullmatch(r"([0-9]+)([mhd])", value)
            if not match:
                raise BackupError("Invalid BACKUP_INTERVAL (expected e.g. 30m, 6h or 1d)")
            self.interval = int(match.group(1)) * {"m": 60, "h": 3600, "d": 86400}[match.group(2)]
            if not 60 <= self.interval <= 30 * 86400:
                raise BackupError("BACKUP_INTERVAL must be between 1m and 30d")
        else:
            raise BackupError("BACKUP_SCHEDULE_MODE must be daily or interval")

        # A long interval must not turn an on-schedule backup unhealthy before it is due.
        # Existing explicit thresholds are still honored; daily keeps its 30-hour default.
        if self.env.get("BACKUP_HEALTH_MAX_AGE_HOURS", "").strip():
            self.health_age = integer(self.env, "BACKUP_HEALTH_MAX_AGE_HOURS", 30, maximum=8760) * 3600
        else:
            self.health_age = max(30 * 3600, self.interval + self.timeout + 3600)
        auto_init = self.env.get("BACKUP_AUTO_INIT", "true").strip().lower()
        if auto_init not in ("true", "false"):
            raise BackupError("BACKUP_AUTO_INIT must be true or false")
        self.auto_init = auto_init == "true"

    def prepare(self):
        for path in (self.state, self.cache):
            path.mkdir(mode=0o700, parents=True, exist_ok=True)

    def repository_env(self):
        env = dict(self.env)
        repository = env.get("BACKUP_REPOSITORY", "")
        if repository.startswith("rclone:oss:"):
            if not re.fullmatch(r"rclone:oss:[a-z0-9][a-z0-9.-]+/[^\s:]+", repository):
                raise BackupError("BACKUP_REPOSITORY requires an OSS bucket and dedicated prefix")
            endpoint = env.get("BACKUP_OSS_ENDPOINT", "")
            if endpoint.startswith("http://"):
                if env.get("BACKUP_ALLOW_INSECURE_OSS") != "true":
                    raise BackupError("HTTP OSS endpoint is disabled; use HTTPS")
                endpoint = endpoint.removeprefix("http://")
            if not re.fullmatch(r"(?:https://)?[a-zA-Z0-9.-]+(?::[0-9]+)?", endpoint):
                raise BackupError("Invalid BACKUP_OSS_ENDPOINT")
            if env.get("BACKUP_OSS_ENDPOINT", "").startswith("http://"):
                endpoint = "http://" + endpoint
            env.update({
                "RCLONE_CONFIG_OSS_TYPE": "s3",
                "RCLONE_CONFIG_OSS_PROVIDER": "Alibaba",
                "RCLONE_CONFIG_OSS_ENDPOINT": endpoint,
                "RCLONE_CONFIG_OSS_ACCESS_KEY_ID": secret(env, "BACKUP_OSS_ACCESS_KEY_ID"),
                "RCLONE_CONFIG_OSS_SECRET_ACCESS_KEY": secret(env, "BACKUP_OSS_ACCESS_KEY_SECRET"),
                "RCLONE_CONFIG_OSS_UPLOAD_CONCURRENCY": "2",
                "RCLONE_CONFIG": "/dev/null",
                "RCLONE_BWLIMIT": env.get("BACKUP_BW_LIMIT", "8M"),
                "RCLONE_RETRIES": "1",
                "RCLONE_LOW_LEVEL_RETRIES": "2",
                "RCLONE_CONTIMEOUT": "15s",
                "RCLONE_TIMEOUT": "60s",
            })
        elif not Path(repository).is_absolute():
            raise BackupError("BACKUP_REPOSITORY must be rclone:oss:bucket/prefix or an absolute local path")
        else:
            repo_path = Path(repository).resolve()
            for directory in (self.state, self.cache, self.source):
                if repo_path == directory or directory in repo_path.parents or repo_path in directory.parents:
                    raise BackupError("Local repository must be separate from state, cache and sources")
        password_file = env.get("BACKUP_PASSWORD_FILE", "/run/secrets/restic-password")
        try:
            if not Path(password_file).read_bytes().strip():
                raise BackupError("Empty BACKUP_PASSWORD_FILE")
        except OSError:
            raise BackupError("Cannot read BACKUP_PASSWORD_FILE") from None
        env.update({
            "RESTIC_REPOSITORY": repository,
            "RESTIC_PASSWORD_FILE": password_file,
            "RESTIC_CACHE_DIR": str(self.cache),
            "GOMAXPROCS": env.get("GOMAXPROCS", "2"),
        })
        # Prevent inherited Restic options from overriding the declared repository/password.
        env.pop("RESTIC_PASSWORD", None)
        env.pop("RESTIC_PASSWORD_COMMAND", None)
        env.pop("RESTIC_REPOSITORY_FILE", None)
        return env

    def identity(self):
        value = [self.env.get("BACKUP_REPOSITORY", ""), self.env.get("BACKUP_OSS_ENDPOINT", "").lower(),
                 self.host, self.tag, self.env.get("BACKUP_DB_HOST", "postgres"),
                 self.env.get("BACKUP_DB_PORT", "5432"), self.env.get("BACKUP_DB_USER", "root"),
                 self.env.get("BACKUP_DATABASES", "new-api"), str(self.source)]
        return hashlib.sha256(json.dumps(value).encode()).hexdigest()

    def database_env(self):
        env = dict(self.env)
        password_file = env.get("BACKUP_DB_PASSWORD_FILE", "")
        env["PGPASSWORD"] = secret(env, "BACKUP_DB_PASSWORD", password_file)
        env["PGHOST"] = env.get("BACKUP_DB_HOST", "postgres")
        env["PGPORT"] = str(integer(env, "BACKUP_DB_PORT", 5432, maximum=65535))
        env["PGUSER"] = env.get("BACKUP_DB_USER", "root")
        env["PGCONNECT_TIMEOUT"] = "15"
        env["PGAPPNAME"] = "new-api-backup"
        env["PGSSLMODE"] = env.get("BACKUP_DB_SSLMODE", "prefer")
        if env["PGSSLMODE"] not in ("disable", "allow", "prefer", "require", "verify-ca", "verify-full"):
            raise BackupError("Invalid BACKUP_DB_SSLMODE")
        # A role with BYPASSRLS/superuser is required for a full export of RLS tables.
        env["PGOPTIONS"] = "-c statement_timeout=0 -c row_security=off"
        return env


@contextlib.contextmanager
def job_lock(config):
    config.prepare()
    with (config.state / "job.lock").open("a") as lock:
        try:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            raise BackupError("Another backup operation is running") from None
        try:
            yield
        finally:
            fcntl.flock(lock, fcntl.LOCK_UN)


def read_status(config):
    try:
        result = json.loads((config.state / "status.json").read_text())
        return result if isinstance(result, dict) else {}
    except (OSError, ValueError):
        return {}


def write_status(config, status):
    path = config.state / "status.json.part"
    with path.open("w") as output:
        json.dump(status, output, ensure_ascii=False)
        output.flush()
        os.fsync(output.fileno())
    path.replace(config.state / "status.json")


def directory_bytes(directory):
    size = 0
    for root, _dirs, files in os.walk(directory, followlinks=False):
        for name in files:
            path = Path(root) / name
            try:
                if not path.is_symlink():
                    size += path.stat().st_size
            except FileNotFoundError:
                continue
    return size


class Job:
    def __init__(self, config, previous_status=None):
        self.config = config
        self.deadline = time.monotonic() + config.timeout
        self.work = None
        self.previous_status = read_status(config) if previous_status is None else previous_status

    def space_guard(self):
        for directory in (self.config.state, self.config.cache):
            if shutil.disk_usage(directory).free < self.config.min_free:
                raise BackupError("Insufficient free space; backup stopped before exhausting disk")
        if self.work and directory_bytes(self.work) > self.config.max_dump:
            raise BackupError("Database export exceeded BACKUP_MAX_DUMP_MB")
        if directory_bytes(self.config.cache) > self.config.max_cache:
            raise BackupError("Restic cache exceeded BACKUP_MAX_CACHE_MB")

    def command(self, stage, args, env=None, monitor=True):
        """Own a process group so timeout/shutdown cannot leave pg_dump/rclone children."""
        if STOPPING:
            raise BackupError("Backup interrupted")
        if time.monotonic() >= self.deadline:
            raise BackupError("Backup operation timed out")
        if monitor:
            self.space_guard()
        with tempfile.TemporaryFile(dir=self.config.state) as output, tempfile.TemporaryFile(dir=self.config.state) as errors:
            try:
                process = subprocess.Popen(args, env=env, stdout=output,
                                           stderr=errors, start_new_session=True)
            except OSError:
                raise BackupError(f"{stage}: executable unavailable") from None
            try:
                while process.poll() is None:
                    if STOPPING:
                        raise BackupError("Backup interrupted")
                    if time.monotonic() >= self.deadline:
                        raise BackupError(f"{stage}: timed out")
                    if monitor:
                        self.space_guard()
                    if output.tell() > 16 * MB or errors.tell() > MB:
                        raise BackupError(f"{stage}: excessive command output")
                    time.sleep(0.5)
                if process.returncode:
                    # Classify privately; never expose provider errors, URLs or credentials.
                    errors.seek(0)
                    detail = errors.read(65536).lower()
                    permanent = any(marker in detail for marker in (
                        b"accessdenied", b"access denied", b"invalidaccesskeyid", b"signaturedoesnotmatch",
                        b"wrong password", b"permission denied", b"password authentication failed",
                        b"repository does not exist", b"no such file", b"status code: 403", b"status code: 401"))
                    transient = any(marker in detail for marker in (
                        b"timeout", b"timed out", b"connection reset", b"connection refused", b"unexpected eof",
                        b"temporary failure", b"broken pipe", b"no such host", b"503 service unavailable",
                        b"database system is starting up", b"database system is in recovery mode",
                        b"not yet accepting connections",
                        b"502 bad gateway", b"status code: 500", b"status code: 502", b"status code: 503",
                        b"status code: 504", b"429 too many requests"))
                    network_exit = process.returncode == 1 or (args[0] == "psql" and process.returncode == 2)
                    retryable = not permanent and (process.returncode == 11 or (network_exit and transient))
                    raise BackupError(f"{stage}: command failed (exit {process.returncode}); check connectivity, credentials and permissions",
                                      retryable, exit_code=process.returncode)
                output.seek(0)
                result = output.read(16 * MB + 1)
                if len(result) > 16 * MB:
                    raise BackupError(f"{stage}: excessive command output")
                return result.decode("utf-8", errors="replace")
            finally:
                # Also signal the group if the leader exited but left child processes behind.
                try:
                    os.killpg(process.pid, signal.SIGTERM)
                except ProcessLookupError:
                    pass
                try:
                    process.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    pass
                try:
                    os.killpg(process.pid, signal.SIGKILL)
                except ProcessLookupError:
                    pass
                process.wait()

    def retry_command(self, stage, args, env):
        """Retry transient connectivity/lock failures without repeating earlier stages."""
        for attempt in range(self.config.retries + 1):
            try:
                return self.command(stage, args, env)
            except BackupError as error:
                if not error.retryable or STOPPING or attempt == self.config.retries or time.monotonic() >= self.deadline:
                    raise
                delay_end = min(self.deadline, time.monotonic() + self.config.retry_delay * (attempt + 1))
                while time.monotonic() < delay_end and not STOPPING:
                    time.sleep(min(0.5, max(0, delay_end - time.monotonic())))

    def restic(self, stage, args):
        return self.retry_command(stage, ["restic", "--retry-lock", "30s", *args], self.config.repository_env())

    def ensure_repository(self, explicit_init=False):
        """Open existing repositories; bootstrap only a positively empty destination.

        The caller holds the local job lock. A brand-new shared repository must have
        one bootstrap node: upstream Restic init is not atomic across separate hosts.
        """
        config = self.config
        env = config.repository_env()
        repository = env["RESTIC_REPOSITORY"]
        location = hashlib.sha256(json.dumps([
            repository, config.env.get("BACKUP_OSS_ENDPOINT", "").lower()
        ]).encode()).hexdigest()
        pin_file = config.state / "repository.json"
        try:
            pin = json.loads(pin_file.read_text())
            if (not isinstance(pin, dict) or not re.fullmatch(r"[0-9a-f]{64}", pin.get("id", ""))
                    or not re.fullmatch(r"[0-9a-f]{64}", pin.get("location", ""))):
                raise ValueError
        except FileNotFoundError:
            pin = None
        except (OSError, ValueError, TypeError):
            raise BackupError("Saved repository identity is unreadable; inspect the backup state directory") from None
        initialized = False
        try:
            output = self.restic("Repository access check", ["cat", "config"])
        except BackupError as error:
            # Only Restic's explicit missing-repository code permits an emptiness
            # check. Password, permission and network errors never trigger init.
            if error.exit_code != 10:
                raise
            if not explicit_init:
                if not config.auto_init:
                    raise BackupError("Repository is not initialized; run init or enable BACKUP_AUTO_INIT") from None
                if pin or self.previous_status.get("repository_used") or self.previous_status.get("success_at"):
                    raise BackupError("A previously used repository is missing; refusing automatic recreation") from None
            if repository.startswith("rclone:oss:"):
                listing = self.retry_command("Repository destination check", [
                    "rclone", "lsjson", repository.removeprefix("rclone:"),
                    "--max-depth", "1", "--no-mimetype", "--no-modtime"
                ], env)
                try:
                    entries = json.loads(listing)
                except ValueError:
                    raise BackupError("Cannot verify whether the repository destination is empty") from None
                if not isinstance(entries, list) or entries:
                    raise BackupError("Repository destination contains data; refusing initialization")
            else:
                path = Path(repository)
                try:
                    if path.exists() and (not path.is_dir() or next(path.iterdir(), None) is not None):
                        raise BackupError("Repository destination contains data; refusing initialization")
                except OSError:
                    raise BackupError("Cannot verify whether the repository destination is empty") from None
            self.restic("Repository initialization", ["init"])
            # Verify both the password and the created repository before recording it.
            output = self.restic("Repository access check", ["cat", "config"])
            initialized = True

        try:
            metadata = json.loads(output)
            repository_id = metadata["id"]
            if not re.fullmatch(r"[0-9a-f]{64}", repository_id):
                raise ValueError
        except (ValueError, KeyError, TypeError):
            raise BackupError("Repository identity could not be verified") from None
        # Endpoint aliases or a copied repository can legitimately change location;
        # accepting the same verified ID supports redeployment without losing history.
        if pin and pin["id"] != repository_id and not explicit_init:
            raise BackupError("Repository identity changed; verify the destination and run init explicitly")
        receipt = {"location": location, "id": repository_id}
        part = pin_file.with_suffix(".json.part")
        with part.open("w") as stream:
            json.dump(receipt, stream)
            stream.flush()
            os.fsync(stream.fileno())
        part.replace(pin_file)
        print(json.dumps({"state": "repository_ready", "repository_id": repository_id,
                          "initialized": initialized}), flush=True)
        return repository_id

    def clear_cache(self):
        # The dedicated cache is disposable. Never touch application files or repository data.
        if directory_bytes(self.config.cache) > self.config.max_cache:
            for path in self.config.cache.iterdir():
                if path.is_symlink() or path.is_file():
                    path.unlink()
                elif path.is_dir():
                    shutil.rmtree(path)

    def backup(self):
        config = self.config
        # Recover only our marked temporary directories, including after SIGKILL/OOM.
        # This must precede the space check: a stale dump may itself have filled staging.
        for path in config.state.glob("job-*"):
            marker = path / ".backup-owned"
            if path.is_dir() and not path.is_symlink() and marker.is_file() and not marker.is_symlink():
                shutil.rmtree(path)
        self.clear_cache()
        self.space_guard()
        for name in ("data", "logs", "config"):
            if not (config.source / name).is_dir():
                raise BackupError(f"Required source directory missing: {name}")
        if not any(path.is_file() for path in (config.source / "config").rglob("*")):
            raise BackupError("Source config is empty; mount recovery configuration read-only")
        databases = [name.strip() for name in config.env.get("BACKUP_DATABASES", "new-api").split(",")]
        if not all(databases) or len(set(databases)) != len(databases):
            raise BackupError("BACKUP_DATABASES must contain unique nonempty database names")
        if any("=" in name or name.startswith(("postgres://", "postgresql://")) or
               any(ord(char) < 32 or ord(char) == 127 for char in name) for name in databases):
            raise BackupError("BACKUP_DATABASES must contain database names, not connection strings")
        database_env = config.database_env()
        self.ensure_repository()
        versions = {}
        for tool in ("pg_dump", "restic", "rclone"):
            versions[tool] = self.command(f"{tool} version", [tool, "version" if tool != "pg_dump" else "--version"]).splitlines()[0]
        match = re.search(r"(\d+)(?:\.\d+)", versions["pg_dump"])
        if not match:
            raise BackupError("Cannot determine PostgreSQL client version")
        client_major = int(match.group(1))
        manifest = {"format": 1, "consistency": "online-uncoordinated", "started_at": utcnow(),
                    "tools": versions, "databases": []}
        self.work = Path(tempfile.mkdtemp(prefix="job-", dir=config.state))
        (self.work / ".backup-owned").touch(mode=0o600)
        try:
            for index, database in enumerate(databases):
                server = self.retry_command("PostgreSQL readiness/version", ["psql", "--no-password", "--no-psqlrc", "--tuples-only", "--no-align", "--dbname", database, "--command", "SHOW server_version_num"], database_env).strip()
                if not server.isdigit() or int(server) // 10000 > client_major:
                    raise BackupError("PostgreSQL server is newer than the backup client; use a matching backup image")
                part = self.work / f"database-{index:03d}.dump.part"
                dump = part.with_suffix("")
                started = utcnow()
                self.command("PostgreSQL export", ["pg_dump", "--no-password", "--format=custom", "--compress=1", "--lock-wait-timeout=10s", "--file", str(part), "--dbname", database], database_env)
                self.command("PostgreSQL dump TOC check", ["pg_restore", "--list", str(part)])
                part.replace(dump)
                checksum = hashlib.sha256()
                with dump.open("rb") as stream:
                    while chunk := stream.read(MB):
                        if STOPPING or time.monotonic() >= self.deadline:
                            raise BackupError("Backup interrupted or timed out while checking export")
                        checksum.update(chunk)
                digest = checksum.hexdigest()
                manifest["databases"].append({"name": database, "file": dump.name, "sha256": digest,
                                              "bytes": dump.stat().st_size, "started_at": started,
                                              "finished_at": utcnow(), "server_version_num": int(server)})
            manifest["dump_finished_at"] = utcnow()
            (self.work / "manifest.json").write_text(json.dumps(manifest, ensure_ascii=False, indent=2))
            output = self.restic("Encrypted repository backup", ["backup", "--json", "--host", config.host, "--tag", config.tag,
                                 str(self.work), str(config.source)])
            snapshots = []
            for line in output.splitlines():
                try:
                    item = json.loads(line)
                except ValueError:
                    continue
                if item.get("message_type") == "summary" and item.get("snapshot_id"):
                    snapshots.append(item["snapshot_id"])
            if len(snapshots) != 1 or not re.fullmatch(r"[0-9a-f]{8,64}", snapshots[0]):
                raise BackupError("Backup completed without a unique snapshot receipt")
            receipt = json.loads(self.restic("Snapshot receipt verification", ["snapshots", "--json", snapshots[0]]))
            if len(receipt) != 1 or not receipt[0]["id"].startswith(snapshots[0]) or receipt[0].get("hostname") != config.host or config.tag not in receipt[0].get("tags", []):
                raise BackupError("Snapshot receipt does not match this backup")
            return receipt[0]["id"]
        finally:
            shutil.rmtree(self.work)
            self.work = None
            self.clear_cache()


def perform_backup(config):
    with job_lock(config):
        previous = read_status(config)
        started = utcnow()
        status = {"state": "running", "started_at": started, "first_started_at": started, "identity": config.identity()}
        # Preserve this across configuration changes, including upgrades from versions
        # without repository.json. A failed attempt must not erase the bootstrap guard.
        status["repository_used"] = bool(previous.get("repository_used") or previous.get("success_at"))
        if previous.get("identity") == config.identity():
            status["first_started_at"] = previous.get("first_started_at", previous.get("started_at", started))
            for field in ("success_at", "snapshot_id"):
                if field in previous:
                    status[field] = previous[field]
        write_status(config, status)
        job = Job(config, previous_status=previous)
        try:
            # Network/lock retries are local to each Restic operation. Never re-export the
            # database merely because an upload or its final receipt temporarily failed.
            snapshot = job.backup()
            status.update(state="success", finished_at=utcnow(), success_at=utcnow(), snapshot_id=snapshot,
                          repository_used=True)
            write_status(config, status)
            print(json.dumps({"state": "success", "snapshot_id": snapshot, "finished_at": status["finished_at"]}), flush=True)
        except Exception as error:
            message = str(error) if isinstance(error, BackupError) else "Backup failed; inspect local configuration and permissions"
            status.update(state="failed", finished_at=utcnow(), error=message)
            write_status(config, status)
            raise BackupError(message) from None


def next_run(now, config):
    if config.schedule_mode == "interval":
        # Epoch arithmetic preserves elapsed time across DST and timezone changes.
        return dt.datetime.fromtimestamp(now.timestamp() + config.interval, config.zone)
    local = now.astimezone(config.zone)
    date = local.date()
    candidates = []
    for _ in range(2):
        for point in config.times:
            candidate = dt.datetime.combine(date, point, config.zone)
            # Missing spring-forward times move forward; repeated fall-back times use
            # only the first occurrence. Select by instant as normalization can reorder points.
            candidate = dt.datetime.fromtimestamp(candidate.timestamp(), config.zone)
            if candidate.timestamp() > now.timestamp():
                candidates.append(candidate)
        date += dt.timedelta(days=1)
    if candidates:
        return min(candidates, key=lambda candidate: candidate.timestamp())
    raise BackupError("Cannot determine next backup time")


def health(config):
    status = read_status(config)
    if status.get("identity") != config.identity() or status.get("state") == "failed":
        return False
    try:
        timestamp = dt.datetime.fromisoformat(status.get("success_at") or status.get("first_started_at") or status["started_at"])
        age = dt.datetime.now(dt.timezone.utc).timestamp() - timestamp.timestamp()
        # Restarting a never-successful monthly job must not extend its bootstrap grace.
        limit = config.health_age if status.get("success_at") else min(config.health_age, 30 * 3600)
        return 0 <= age <= limit
    except (ValueError, KeyError, TypeError):
        return False


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    for name in ("init", "backup", "run", "snapshots", "check", "health"):
        commands.add_parser(name)
    restore = commands.add_parser("restore-files")
    restore.add_argument("snapshot")
    restore.add_argument("destination")
    retention = commands.add_parser("retention")
    retention.add_argument("--apply", action="store_true", help="Explicitly forget/prune expired snapshots")
    args = parser.parse_args(argv)
    os.umask(0o077)
    config = Config()
    if args.command == "health":
        return 0 if health(config) else 1
    if args.command == "backup":
        perform_backup(config)
        return 0
    if args.command == "run":
        while not STOPPING:
            try:
                perform_backup(config)
            except BackupError as error:
                print(str(error), file=sys.stderr, flush=True)
            if STOPPING:
                break
            scheduled = next_run(dt.datetime.now(dt.timezone.utc), config)
            # Intervals start after this attempt finishes and use monotonic time so
            # clock adjustments cannot shorten/extend them. Daily plans follow wall time.
            clock = time.monotonic if config.schedule_mode == "interval" else time.time
            due = clock() + config.interval if config.schedule_mode == "interval" else scheduled.timestamp()
            print(json.dumps({"state": "waiting", "mode": config.schedule_mode,
                              "next_run_at": scheduled.isoformat()}), flush=True)
            while not STOPPING:
                remaining = due - clock()
                if remaining <= 0:
                    break
                time.sleep(min(1, remaining))
        return 0
    with job_lock(config):
        job = Job(config)
        job.clear_cache()
        if args.command == "init":
            job.ensure_repository(explicit_init=True)
        elif args.command == "snapshots":
            print(job.restic("Snapshot listing", ["snapshots", "--host", config.host, "--tag", config.tag]))
        elif args.command == "check":
            print(job.restic("Repository integrity check", ["check", "--read-data"]))
        elif args.command == "restore-files":
            if not re.fullmatch(r"[0-9a-f]{8,64}", args.snapshot):
                raise BackupError("Restore requires an explicit hexadecimal snapshot ID (not latest)")
            destination = Path(args.destination)
            if not destination.is_absolute() or destination.exists() or destination.is_symlink():
                raise BackupError("Restore destination must be an absolute, nonexisting directory")
            destination = destination.resolve()
            for directory in (config.source, config.state, config.cache):
                if destination == directory or directory in destination.parents or destination in directory.parents:
                    raise BackupError("Restore destination must be separate from live sources/state/cache")
            repository = config.env.get("BACKUP_REPOSITORY", "")
            if Path(repository).is_absolute():
                repo_path = Path(repository).resolve()
                if destination == repo_path or repo_path in destination.parents or destination in repo_path.parents:
                    raise BackupError("Restore destination must be separate from repository")
            receipt = json.loads(job.restic("Restore snapshot verification", ["snapshots", "--json", args.snapshot]))
            if len(receipt) != 1 or receipt[0].get("hostname") != config.host or config.tag not in receipt[0].get("tags", []):
                raise BackupError("Snapshot does not belong to this backup host/tag")
            destination.mkdir(mode=0o700)
            print(job.restic("File restore verification", ["restore", args.snapshot, "--target", str(destination), "--verify"]))
        elif args.command == "retention":
            daily = integer(config.env, "BACKUP_KEEP_DAILY", 7, minimum=2)
            weekly = integer(config.env, "BACKUP_KEEP_WEEKLY", 4, minimum=0)
            monthly = integer(config.env, "BACKUP_KEEP_MONTHLY", 6, minimum=0)
            options = ["forget", "--host", config.host, "--tag", config.tag, "--group-by", "host,tags",
                       "--keep-last", "2", "--keep-daily", str(daily), "--keep-weekly", str(weekly), "--keep-monthly", str(monthly)]
            if args.apply:
                status = read_status(config)
                if status.get("state") != "success" or not health(config):
                    raise BackupError("Retention apply requires a recent successful backup for this repository/host/tag")
                options.append("--prune")
            else:
                options.append("--dry-run")
            print(job.restic("Repository retention", options))
    return 0


if __name__ == "__main__":
    for signum in (signal.SIGTERM, signal.SIGINT):
        signal.signal(signum, stop)
    try:
        sys.exit(main())
    except BackupError as error:
        print(str(error), file=sys.stderr)
        sys.exit(1)
    except Exception:
        # Credentials can appear in exceptions from libraries/subprocesses; never print them.
        print("Backup operation failed; check configuration and permissions", file=sys.stderr)
        sys.exit(1)
