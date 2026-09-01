#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=scripts/bootstrap_services/_common.sh
source "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

LOG_LABEL="dispatch-local"

PG_PORT="${PG_PORT:-5435}"
REDIS_PORT="${REDIS_PORT:-6382}"
PG_DATA_DIR="${PG_DATA_DIR:-$KARRYGO_DATA_DIR/postgres-dispatch}"
REDIS_DATA_DIR="${REDIS_DATA_DIR:-$KARRYGO_DATA_DIR/redis-6382}"

ROLE_NAME="cosmicforge_logistics"
ROLE_PASSWORD="cosmicforge_logistics"
DB_NAME="dispatch_delivery_service"
MIG_DIR="$ROOT_DIR/services/driver-dispatch-delivery-service/migrations"

ensure_role_and_db() {
  log "ensuring dispatch role and database exist"
  if ! "$PSQL_BIN" -p "$PG_PORT" -d template1 -tAc "SELECT 1 FROM pg_roles WHERE rolname = '${ROLE_NAME}'" | grep -q 1; then
    log "creating role ${ROLE_NAME}"
    "$PSQL_BIN" -p "$PG_PORT" -d template1 -v ON_ERROR_STOP=1 -c "CREATE ROLE ${ROLE_NAME} LOGIN PASSWORD '${ROLE_PASSWORD}' CREATEDB;"
  else
    log "role ${ROLE_NAME} already exists"
  fi

  if ! "$PSQL_BIN" -p "$PG_PORT" -d template1 -tAc "SELECT 1 FROM pg_database WHERE datname = '${DB_NAME}'" | grep -q 1; then
    log "creating database ${DB_NAME}"
    "$CREATEDB_BIN" -p "$PG_PORT" -O "$ROLE_NAME" "$DB_NAME"
  else
    log "database ${DB_NAME} already exists"
  fi
}

# The dispatch service migrations are golang-migrate style .up/.down files and
# are NOT all re-run-safe (e.g. 000021_create_trips.up.sql lacks IF NOT EXISTS),
# so applied files are tracked in a local marker table and skipped on re-runs.
apply_migrations() {
  log "applying dispatch migrations"
  local dsn="postgres://${ROLE_NAME}:${ROLE_PASSWORD}@localhost:${PG_PORT}/${DB_NAME}?sslmode=disable"

  "$PSQL_BIN" "$dsn" -v ON_ERROR_STOP=1 -q -c \
    "CREATE TABLE IF NOT EXISTS local_schema_migrations (filename TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now());"

  apply_file() {
    local file="$1"
    local name
    name="$(basename "$file")"
    if "$PSQL_BIN" "$dsn" -tAc "SELECT 1 FROM local_schema_migrations WHERE filename = '${name}'" | grep -q 1; then
      log "skipping ${name} (already applied)"
      return
    fi
    log "applying ${name}"
    "$PSQL_BIN" "$dsn" -v ON_ERROR_STOP=1 -q -f "$file"
    "$PSQL_BIN" "$dsn" -v ON_ERROR_STOP=1 -q -c \
      "INSERT INTO local_schema_migrations (filename) VALUES ('${name}') ON CONFLICT DO NOTHING;"
  }

  # Legacy dispatch-rider auth migrations first, then the numbered .up files.
  for f in \
    "$MIG_DIR"/001_dispatch_rider_auth.sql \
    "$MIG_DIR"/002_dispatch_rider_auth_alter.sql \
    "$MIG_DIR"/003_dispatch_rider_auth_signup.sql; do
    [ -f "$f" ] && apply_file "$f"
  done
  for f in "$MIG_DIR"/0000*.up.sql; do
    apply_file "$f"
  done
}

seed_dev_riders() {
  local seed="$ROOT_DIR/services/driver-dispatch-delivery-service/seeds/dev_seed.sql"
  if [ -f "$seed" ]; then
    log "seeding dev dispatch riders (idempotent)"
    local dsn="postgres://${ROLE_NAME}:${ROLE_PASSWORD}@localhost:${PG_PORT}/${DB_NAME}?sslmode=disable"
    "$PSQL_BIN" "$dsn" -v ON_ERROR_STOP=1 -q -f "$seed"
  fi
}

main() {
  cd "$ROOT_DIR"
  start_redis
  start_postgres
  ensure_role_and_db
  apply_migrations
  seed_dev_riders

  log "ready. start the service with:"
  printf 'cd %s/services/driver-dispatch-delivery-service && go run ./cmd\n' "$ROOT_DIR"
}

main "$@"
