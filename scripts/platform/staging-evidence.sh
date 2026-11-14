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

services=("$@")
[ "${#services[@]}" -gt 0 ] || mapfile -t services < <(all_services)

echo "EVIDENCE host=$(hostname) at=$(date -u +%Y-%m-%dT%H:%M:%SZ) account=$(id -un)"

for key in "${services[@]}"; do
  load_service "$key" || { echo "SERVICE $key unknown"; continue; }
  echo "== SERVICE $key"

  # Settings file: owner and mode only (never contents).
  if [ -e "$SVC_ENV_PATH" ]; then
    owner_mode="$(stat -c '%U:%G %a' "$SVC_ENV_PATH" 2>/dev/null || echo unreadable)"
    readable=no; [ -r "$SVC_ENV_PATH" ] && readable=yes
    echo "SETTINGS file=$SVC_ENV_PATH owner_mode=$owner_mode readable_by_deploy=$readable"
  else
    echo "SETTINGS file=$SVC_ENV_PATH missing"
  fi

  # Running containers for this service (api/app, worker, migrate helpers).
  base="${SVC_CONTAINER}"
  for c in $(docker ps -a --format '{{.Names}}' | grep -E "^${base%-service}(-|$)|^${base}$" | sort -u); do
    img="$(docker inspect -f '{{.Image}}' "$c" 2>/dev/null)"
    rev="$(docker image inspect -f '{{index .Config.Labels "org.opencontainers.image.revision"}}' "$img" 2>/dev/null)"
    state="$(docker inspect -f '{{.State.Status}}' "$c" 2>/dev/null)"
    health="$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}-{{end}}' "$c" 2>/dev/null)"
    started="$(docker inspect -f '{{.State.StartedAt}}' "$c" 2>/dev/null)"
    echo "CONTAINER name=$c state=$state health=$health revision=${rev:-unknown} started=$started"
  done
  release="$(current_image 2>/dev/null || true)"
  echo "RELEASE image=${release:-none}"

  # Migration bookkeeping in the service's own database container.
  db="karrygo-${SVC_HOME%-service}-db"
  if [ "$(docker inspect -f '{{.State.Running}}' "$db" 2>/dev/null)" = true ]; then
    docker exec "$db" sh -c '
      psql -X -A -t -U "$POSTGRES_USER" -d "${POSTGRES_DB:-$POSTGRES_USER}" -v ON_ERROR_STOP=1 <<SQL 2>/dev/null
SELECT format($$MIGRATIONS table=%s.%s$$, table_schema, table_name)
  FROM information_schema.tables
 WHERE table_name ILIKE $$%migration%$$ AND table_schema NOT IN ($$pg_catalog$$,$$information_schema$$);
SQL
    ' 2>/dev/null
    # Latest applied entry per common bookkeeping table (version or name column).
    docker exec "$db" sh -c '
      for t in schema_migrations local_schema_migrations fleet_partner_schema_migrations goose_db_version; do
        psql -X -A -t -U "$POSTGRES_USER" -d "${POSTGRES_DB:-$POSTGRES_USER}" -c "SELECT 1 FROM $t LIMIT 1" >/dev/null 2>&1 || continue
        col=$(psql -X -A -t -U "$POSTGRES_USER" -d "${POSTGRES_DB:-$POSTGRES_USER}" -c "SELECT column_name FROM information_schema.columns WHERE table_name = '"'"'$t'"'"' AND column_name IN ('"'"'version'"'"','"'"'name'"'"','"'"'filename'"'"','"'"'version_id'"'"') ORDER BY 1 LIMIT 1")
        [ -n "$col" ] || continue
        latest=$(psql -X -A -t -U "$POSTGRES_USER" -d "${POSTGRES_DB:-$POSTGRES_USER}" -c "SELECT max($col::text) FROM $t")
        count=$(psql -X -A -t -U "$POSTGRES_USER" -d "${POSTGRES_DB:-$POSTGRES_USER}" -c "SELECT count(*) FROM $t")
        echo "MIGRATION_HEAD table=$t latest=$latest applied=$count"
      done
    ' 2>/dev/null
  else
    echo "DATABASE container=$db not-running-or-shared"
  fi
done

echo "== DONE"
