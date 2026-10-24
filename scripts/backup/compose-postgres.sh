#!/usr/bin/env bash
# New API: PostgreSQL Compose backups encrypted by Restic.
# This tool never stops the application or imports data into a database.
set -Eeuo pipefail
umask 077

die() { printf 'ERROR: %s\n' "$*" >&2; exit 1; }
usage() {
  printf 'Usage: bash %s backup|check|snapshots CONFIG\n' "$0"
  printf '       bash %s restore-files CONFIG SNAPSHOT_ID NEW_DIRECTORY\n' "$0"
  exit 2
}

[[ $# -ge 2 ]] || usage
action=$1
config=$2
case "$action" in backup|check|snapshots|restore-files) ;; *) usage ;; esac
[[ -f "$config" ]] || die "Config file does not exist: $config"
# CONFIG is a trusted shell file, like a deployment script. Store it outside
# the repository with mode 600; never source a file supplied by a customer.
# shellcheck source=/dev/null
source "$config"
: "${RESTIC_REPOSITORY:?Set RESTIC_REPOSITORY in CONFIG}"
: "${RESTIC_PASSWORD_FILE:?Set RESTIC_PASSWORD_FILE in CONFIG}"
[[ -s "$RESTIC_PASSWORD_FILE" ]] || die 'Restic password file is missing or empty'
export RESTIC_REPOSITORY RESTIC_PASSWORD_FILE
command -v restic >/dev/null || die 'Install restic first'

case "$action" in
  check)
    [[ $# -eq 2 ]] || usage
    # Full read verifies the stored backup data, not just repository metadata.
    exec restic check --read-data
    ;;
  snapshots)
    [[ $# -eq 2 ]] || usage
    exec restic snapshots --tag "${BACKUP_TAG:?Set BACKUP_TAG}"
    ;;
  restore-files)
    [[ $# -eq 4 ]] || usage
    [[ "$3" =~ ^[0-9a-f]{8,64}$ ]] || die 'Supply an explicit snapshot ID, not latest'
    destination=$4
    [[ "$destination" == /* && "$destination" != / ]] || die 'Use an absolute destination directory'
    [[ ! -e "$destination" && ! -L "$destination" ]] || die 'Restore destination must not already exist'
    mkdir -m 700 -- "$destination"
    # Restore to a separate directory. Database imports and Redis replacement
    # require a reviewed recovery procedure and are intentionally not automated.
    exec restic restore "$3" --target "$destination" --verify
    ;;
esac

[[ $# -eq 2 ]] || usage
: "${DEPLOY_DIR:?Set DEPLOY_DIR}"
: "${STATE_DIR:?Set STATE_DIR outside DEPLOY_DIR}"
: "${BACKUP_HOST:?Set a stable BACKUP_HOST for this deployment}"
: "${BACKUP_TAG:?Set a stable BACKUP_TAG for this backup job}"
POSTGRES_SERVICE=${POSTGRES_SERVICE:-postgres}
REDIS_SERVICE=${REDIS_SERVICE:-redis}
BACKUP_REDIS=${BACKUP_REDIS:-false}
[[ "$BACKUP_REDIS" == true || "$BACKUP_REDIS" == false ]] || die 'BACKUP_REDIS must be true or false'
declare -p COMPOSE_FILES DATABASES BACKUP_PATHS >/dev/null 2>&1 || die 'Set COMPOSE_FILES, DATABASES and BACKUP_PATHS arrays'
(( ${#COMPOSE_FILES[@]} > 0 && ${#DATABASES[@]} > 0 && ${#BACKUP_PATHS[@]} > 0 )) || die 'Backup source arrays must not be empty'
for tool in docker flock realpath mktemp sha256sum; do
  command -v "$tool" >/dev/null || die "Install $tool first"
done
[[ -d "$DEPLOY_DIR" && "$STATE_DIR" == /* ]] || die 'DEPLOY_DIR must exist and STATE_DIR must be absolute'
mkdir -p -m 700 -- "$STATE_DIR"
STATE_DIR=$(realpath -e -- "$STATE_DIR")
DEPLOY_DIR=$(realpath -e -- "$DEPLOY_DIR")
compose=(docker compose --project-directory "$DEPLOY_DIR")
for file in "${COMPOSE_FILES[@]}"; do
  [[ "$file" == /* && -f "$file" ]] || die "Missing absolute Compose file: $file"
  compose+=(-f "$file")
done
[[ -z ${COMPOSE_PROJECT_NAME:-} ]] || compose+=(-p "$COMPOSE_PROJECT_NAME")
for db in "${DATABASES[@]}"; do
  [[ "$db" =~ ^[a-zA-Z0-9_][a-zA-Z0-9_-]*$ ]] || die 'DATABASES must contain database names, not connection strings or paths'
done
for source_path in "${BACKUP_PATHS[@]}"; do
  [[ "$source_path" == /* && -e "$source_path" ]] || die "Missing absolute backup source: $source_path"
  resolved=$(realpath -e -- "$source_path")
  [[ "$resolved" != / && "$STATE_DIR" != "$resolved" && "$STATE_DIR" != "$resolved/"* ]] || die 'STATE_DIR must not be inside a backup source'
done
# Lock this job before creating any plaintext dump or touching Redis.
exec 9>"$STATE_DIR/job.lock"
flock -n 9 || die 'Another backup is already running for this STATE_DIR'
work=$(mktemp -d "$STATE_DIR/run.XXXXXXXX")
redis_temp=''
cleanup() {
  local result=$?
  trap - EXIT
  if [[ -n "$redis_temp" ]]; then
    "${compose[@]}" exec -T "$REDIS_SERVICE" rm -f -- "$redis_temp" >/dev/null 2>&1 || true
  fi
  # work is created by mktemp under the validated private state directory.
  rm -rf -- "$work"
  exit "$result"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

printf 'Online backup: database, files and Redis are not one atomic snapshot.\n'
{
  printf 'format=new-api-compose-backup-v1\nconsistency=online-uncoordinated\n'
  printf 'started_utc=%s\nhost=%s\ntag=%s\n' "$(date -u +%FT%TZ)" "$BACKUP_HOST" "$BACKUP_TAG"
  printf 'postgres_service=%s\nredis_snapshot=%s\n' "$POSTGRES_SERVICE" "$BACKUP_REDIS"
  printf 'database=%s\n' "${DATABASES[@]}"
  printf 'source=%s\n' "${BACKUP_PATHS[@]}"
} >"$work/manifest.txt"

for db in "${DATABASES[@]}"; do
  # Password stays in the container environment, never in command arguments.
  "${compose[@]}" exec -T "$POSTGRES_SERVICE" sh -eu -c '
    export PGPASSWORD="${POSTGRES_PASSWORD:-}"
    exec pg_dump --username "${POSTGRES_USER:-postgres}" --format=custom --dbname "$1"
  ' sh "$db" >"$work/$db.dump"
  [[ -s "$work/$db.dump" ]] || die "Empty database dump: $db"
  "${compose[@]}" exec -T "$POSTGRES_SERVICE" pg_restore --list <"$work/$db.dump" >"$work/$db.toc.txt"
done

if [[ "$BACKUP_REDIS" == true ]]; then
  redis_temp="/tmp/new-api-backup-$(basename "$work").rdb"
  "${compose[@]}" exec -T "$REDIS_SERVICE" sh -eu -c '
    export REDISCLI_AUTH="${REDIS_PASSWORD:?REDIS_PASSWORD is required}"
    exec redis-cli --rdb "$1"
  ' sh "$redis_temp"
  "${compose[@]}" exec -T "$REDIS_SERVICE" redis-check-rdb "$redis_temp"
  "${compose[@]}" cp "$REDIS_SERVICE:$redis_temp" "$work/redis.rdb"
fi

# Record the immutable image IDs without exporting container environment secrets.
"${compose[@]}" images -q >"$work/image-ids.txt"
while IFS= read -r image_id; do
  [[ -n "$image_id" ]] || continue
  docker image inspect --format '{{.Id}} {{json .RepoTags}} {{json .RepoDigests}}' "$image_id"
done <"$work/image-ids.txt" >"$work/images.txt"
printf 'exports_finished_utc=%s\n' "$(date -u +%FT%TZ)" >>"$work/manifest.txt"
(cd "$work" && sha256sum -- *.dump manifest.txt images.txt >SHA256SUMS)
if [[ "$BACKUP_REDIS" == true ]]; then
  (cd "$work" && sha256sum -- redis.rdb >>SHA256SUMS)
fi
# A nonzero restic exit (including unreadable source files) is a failed backup.
restic backup --host "$BACKUP_HOST" --tag "$BACKUP_TAG" "$work" "${BACKUP_PATHS[@]}"
printf 'Backup completed. Record the snapshot ID shown above.\n'
