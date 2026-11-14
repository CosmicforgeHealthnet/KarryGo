#!/usr/bin/env bash
# Read-only staging evidence for the KarryGo platform services.
#
# Run on the staging server as the deploy account, from an unpacked copy of
# deploy/platform (so bin/lib.sh and services.tsv are available):
#
#   bash staging-evidence.sh <platform-dir> [service ...]
#
# It changes nothing and never prints a secret:
#   - settings files are never opened; only their owner and mode are read
#     (stat), which is what the deploy preflight checks;
#   - database queries are read-only SELECTs on migration bookkeeping tables;
#   - container inspection prints image ids, revision labels, state and health.
set -uo pipefail
PLATFORM="$1"; shift
cd "$PLATFORM" || exit 1
# shellcheck disable=SC1091
source bin/lib.sh
# lib.sh enables errexit for deploy scripts. Evidence must report every
# service even when one probe fails, so failures are tolerated per step.
set +e

services=("$@")
[ "${#services[@]}" -gt 0 ] || mapfile -t services < <(all_services)

echo "EVIDENCE host=$(hostname) at=$(date -u +%Y-%m-%dT%H:%M:%SZ) account=$(id -un)"

for key in "${services[@]}"; do
  ( load_service "$key" ) >/dev/null 2>&1 || { echo "SERVICE $key unknown"; continue; }
  load_service "$key"
  echo "== SERVICE $key"

  # Settings file: owner and mode only (never contents).
  if [ -e "$SVC_ENV_PATH" ]; then
    owner_mode="$(stat -c '%U:%G %a' "$SVC_ENV_PATH" 2>/dev/null || echo unreadable)"
    readable=no; [ -r "$SVC_ENV_PATH" ] && readable=yes
    echo "SETTINGS file=$SVC_ENV_PATH owner_mode=$owner_mode readable_by_deploy=$readable"
  else
    echo "SETTINGS file=$SVC_ENV_PATH missing"
  fi

  # Running containers for this service: everything under its karrygo-<home>
  # prefix (api, worker, db, redis, helpers).
  prefix="karrygo-${SVC_HOME%-service}"
  for c in $(docker ps -a --format '{{.Names}}' | grep -E "^${prefix}(-|$)|^${SVC_CONTAINER}$" | sort -u); do
    img="$(docker inspect -f '{{.Image}}' "$c" 2>/dev/null)"
    rev="$(docker image inspect -f '{{index .Config.Labels "org.opencontainers.image.revision"}}' "$img" 2>/dev/null)"
    state="$(docker inspect -f '{{.State.Status}}' "$c" 2>/dev/null)"
    health="$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}-{{end}}' "$c" 2>/dev/null)"
    started="$(docker inspect -f '{{.State.StartedAt}}' "$c" 2>/dev/null)"
    echo "CONTAINER name=$c state=$state health=$health revision=${rev:-unknown} started=$started"
  done
  # Media only: private-object encryption at rest (FLEET-7h.c). Reports the
  # backend, whether keys are configured (yes/no, never the value) and, for
  # the filesystem backend, how many objects start with the encryption
  # header magic (first 4 bytes only; object contents are never read).
  if [ "$key" = media-file-service ] && [ "$(docker inspect -f '{{.State.Running}}' "$SVC_CONTAINER" 2>/dev/null)" = true ]; then
    docker exec "$SVC_CONTAINER" sh -c '
      keys=no; [ -n "${MEDIA_FILE_PRIVATE_ENCRYPTION_KEYS:-}" ] && keys=yes
      if [ -n "${MEDIA_FILE_FIREBASE_BUCKET:-}" ]; then
        echo "MEDIA_AT_REST backend=bucket keys_configured=$keys"
      else
        d="${MEDIA_FILE_PRIVATE_STORAGE_DIR:-/tmp/karrygo-media-private}"
        total=0; sealed=0
        for f in $(find "$d" -type f 2>/dev/null); do
          total=$((total+1))
          [ "$(head -c 4 "$f" | od -An -tx1 | tr -d " \n")" = "894b474d" ] && sealed=$((sealed+1))
        done
        echo "MEDIA_AT_REST backend=filesystem keys_configured=$keys objects=$total encrypted=$sealed legacy_plaintext=$((total-sealed))"
      fi' 2>/dev/null
  fi
  release="$(current_image 2>/dev/null || true)"
  echo "RELEASE image=${release:-none}"

  # Migration bookkeeping in the service's own database container.
  db="karrygo-${SVC_HOME%-service}-db"
  if [ "$(docker inspect -f '{{.State.Running}}' "$db" 2>/dev/null)" = true ]; then
    # One read-only block: for every table whose name contains "migration",
    # report its row count and the greatest value of its first version-like
    # column. SQL travels on stdin so the shell never expands it.
    docker exec -i "$db" sh -c 'psql -X -q -U "$POSTGRES_USER" -d "${POSTGRES_DB:-$POSTGRES_USER}" -v ON_ERROR_STOP=0 -f - 2>&1' <<'SQL' | sed -n 's/^.*NOTICE:  //p'
DO $evidence$
DECLARE t record; col text; latest text; n bigint;
BEGIN
  FOR t IN SELECT table_schema, table_name FROM information_schema.tables
           WHERE table_name ILIKE '%migration%' AND table_schema NOT IN ('pg_catalog','information_schema') LOOP
    SELECT column_name INTO col FROM information_schema.columns
     WHERE table_schema = t.table_schema AND table_name = t.table_name
       AND column_name IN ('version','name','filename','version_id','migration')
     ORDER BY array_position(ARRAY['version','name','filename','version_id','migration'], column_name::text) LIMIT 1;
    EXECUTE format('SELECT count(*) FROM %I.%I', t.table_schema, t.table_name) INTO n;
    IF col IS NOT NULL THEN
      EXECUTE format('SELECT max(%I::text) FROM %I.%I', col, t.table_schema, t.table_name) INTO latest;
    ELSE latest := '-'; END IF;
    RAISE NOTICE 'MIGRATION_HEAD table=%.% latest=% applied=%', t.table_schema, t.table_name, latest, n;
  END LOOP;
END
$evidence$;
SQL
  else
    echo "DATABASE container=$db not-running-or-shared"
  fi
done

echo "== DONE"
