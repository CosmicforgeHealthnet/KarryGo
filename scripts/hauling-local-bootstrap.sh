#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PG_PORT="${PG_PORT:-5436}"
REDIS_PORT="${REDIS_PORT:-6383}"
PG_DATA_DIR="${PG_DATA_DIR:-/tmp/postgres-hauling}"
REDIS_DATA_DIR="${REDIS_DATA_DIR:-/tmp/redis-6383}"
PG_BIN="${PG_BIN:-$(command -v postgres >/dev/null 2>&1 && dirname "$(command -v postgres)" || echo /opt/homebrew/opt/postgresql@16/bin)}"
REDIS_BIN="${REDIS_BIN:-$(command -v redis-server)}"
PSQL_BIN="${PSQL_BIN:-$(command -v psql)}"
INITDB_BIN="${INITDB_BIN:-$(command -v initdb)}"
PG_CTL_BIN="${PG_CTL_BIN:-$(command -v pg_ctl)}"
CREATEDB_BIN="${CREATEDB_BIN:-$(command -v createdb)}"

ROLE_NAME="cosmicforge_logistics"
ROLE_PASSWORD="cosmicforge_logistics"
DB_NAME="hauling_service"

log() {
  printf '\033[1;36m[hauling-local]\033[0m %s\n' "$*"
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
  log "ensuring hauling role and database exist"
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

apply_migrations() {
  log "applying hauling migrations"
  local dsn="postgres://${ROLE_NAME}:${ROLE_PASSWORD}@localhost:${PG_PORT}/${DB_NAME}?sslmode=disable"
  "$PSQL_BIN" "$dsn" -f "$ROOT_DIR/services/driver-hauling-service/migrations/001_hauling_core.sql"
  "$PSQL_BIN" "$dsn" -f "$ROOT_DIR/services/driver-hauling-service/migrations/002_schema_updates.sql"
  "$PSQL_BIN" "$dsn" -f "$ROOT_DIR/services/driver-hauling-service/migrations/003_provider_onboarding.sql"
  "$PSQL_BIN" "$dsn" -f "$ROOT_DIR/services/driver-hauling-service/migrations/004_package_info_and_reviews.sql"
}

main() {
  cd "$ROOT_DIR"
  start_redis
  start_postgres
  ensure_role_and_db
  apply_migrations

  log "ready. start the service with:"
  printf 'cd %s/services/driver-hauling-service && go run ./cmd\n' "$ROOT_DIR"
}

main "$@"
