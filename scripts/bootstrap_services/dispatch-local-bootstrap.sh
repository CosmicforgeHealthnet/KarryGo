#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
PG_PORT="${PG_PORT:-5435}"
REDIS_PORT="${REDIS_PORT:-6382}"
PG_DATA_DIR="${PG_DATA_DIR:-/tmp/postgres-dispatch}"
REDIS_DATA_DIR="${REDIS_DATA_DIR:-/tmp/redis-6382}"
PG_BIN="${PG_BIN:-$(command -v postgres >/dev/null 2>&1 && dirname "$(command -v postgres)" || echo /opt/homebrew/opt/postgresql@16/bin)}"
REDIS_BIN="${REDIS_BIN:-$(command -v redis-server)}"
PSQL_BIN="${PSQL_BIN:-$(command -v psql)}"
INITDB_BIN="${INITDB_BIN:-$(command -v initdb)}"
PG_CTL_BIN="${PG_CTL_BIN:-$(command -v pg_ctl)}"
CREATEDB_BIN="${CREATEDB_BIN:-$(command -v createdb)}"

ROLE_NAME="cosmicforge_logistics"
ROLE_PASSWORD="cosmicforge_logistics"
DB_NAME="dispatch_delivery_service"
MIG_DIR="$ROOT_DIR/services/driver-dispatch-delivery-service/migrations"

log() {
  printf '\033[1;36m[dispatch-local]\033[0m %s\n' "$*"
}

ensure_dir() {
  mkdir -p "$1"
}

start_redis() {
  if lsof -nP -iTCP:"$REDIS_PORT" -sTCP:LISTEN >/dev/null 2>&1; then
    log "redis already listening on ${REDIS_PORT}"
    return
  fi

  ensure_dir "$REDIS_DATA_DIR"
  log "starting redis on ${REDIS_PORT}"
  "$REDIS_BIN" --port "$REDIS_PORT" --dir "$REDIS_DATA_DIR" --daemonize yes
}

start_postgres() {
  if lsof -nP -iTCP:"$PG_PORT" -sTCP:LISTEN >/dev/null 2>&1; then
    log "postgres already listening on ${PG_PORT}"
    return
  fi

  ensure_dir "$PG_DATA_DIR"

  if [ ! -f "$PG_DATA_DIR/PG_VERSION" ]; then
    log "initializing postgres cluster in ${PG_DATA_DIR}"
    "$INITDB_BIN" -D "$PG_DATA_DIR" >/dev/null
  fi

  log "starting postgres on ${PG_PORT}"
  "$PG_CTL_BIN" -D "$PG_DATA_DIR" -o "-p $PG_PORT" -l "$PG_DATA_DIR/postgres.log" start
}

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
