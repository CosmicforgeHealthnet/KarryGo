#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
PG_PORT="${PG_PORT:-5434}"
REDIS_PORT="${REDIS_PORT:-6381}"
PG_DATA_DIR="${PG_DATA_DIR:-/tmp/postgres-taxi}"
REDIS_DATA_DIR="${REDIS_DATA_DIR:-/tmp/redis-6381}"
PG_BIN="${PG_BIN:-$(command -v postgres >/dev/null 2>&1 && dirname "$(command -v postgres)" || echo /opt/homebrew/opt/postgresql@16/bin)}"
REDIS_BIN="${REDIS_BIN:-$(command -v redis-server)}"
PSQL_BIN="${PSQL_BIN:-$(command -v psql)}"
INITDB_BIN="${INITDB_BIN:-$(command -v initdb)}"
PG_CTL_BIN="${PG_CTL_BIN:-$(command -v pg_ctl)}"
CREATEDB_BIN="${CREATEDB_BIN:-$(command -v createdb)}"

ROLE_NAME="cosmicforge_logistics"
ROLE_PASSWORD="cosmicforge_logistics"
DB_NAME="taxi_service"

# Both initdb and the postmaster read the locale from the environment. When
# LANG/LC_* are unset -- which is normal in a non-interactive shell -- initdb
# refuses to run and postgres 16 dies at startup with "postmaster became
# multithreaded during startup". Resolve one explicitly and export it rather
# than depending on the caller's environment.
if [ -z "${PG_LOCALE:-}" ]; then
  if locale -a 2>/dev/null | grep -qx "en_US.UTF-8"; then
    PG_LOCALE="en_US.UTF-8"
  else
    PG_LOCALE="C"
  fi
fi
export LC_ALL="$PG_LOCALE"
export LANG="$PG_LOCALE"

log() {
  printf '\033[1;33m[taxi-local]\033[0m %s\n' "$*"
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
    log "  locale ${PG_LOCALE}"
    "$INITDB_BIN" -D "$PG_DATA_DIR" --encoding=UTF8 --locale="$PG_LOCALE" >/dev/null
  fi

  log "starting postgres on ${PG_PORT}"
  "$PG_CTL_BIN" -D "$PG_DATA_DIR" -o "-p $PG_PORT" -l "$PG_DATA_DIR/postgres.log" start
}

ensure_role_and_db() {
  log "ensuring taxi role and database exist"
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
  log "applying taxi migrations"
  local dsn="postgres://${ROLE_NAME}:${ROLE_PASSWORD}@localhost:${PG_PORT}/${DB_NAME}?sslmode=disable"
  local migration
  # Apply every migration in sorted order, skipping the .down.sql rollbacks.
  # This mirrors internal/database/migrate.go so new migrations are picked up
  # automatically instead of needing to be listed here.
  for migration in "$ROOT_DIR/services/driver-taxi-service/migrations"/*.sql; do
    case "$migration" in
      *.down.sql) continue ;;
    esac
    log "  -> $(basename "$migration")"
    "$PSQL_BIN" "$dsn" -f "$migration"
  done
}

seed_dev_drivers() {
  local seed="$ROOT_DIR/services/driver-taxi-service/seeds/dev_seed.sql"
  if [ -f "$seed" ]; then
    log "seeding dev taxi drivers (idempotent)"
    local dsn="postgres://${ROLE_NAME}:${ROLE_PASSWORD}@localhost:${PG_PORT}/${DB_NAME}?sslmode=disable"
    "$PSQL_BIN" "$dsn" -v ON_ERROR_STOP=1 -f "$seed"
  fi
}

main() {
  cd "$ROOT_DIR"
  start_redis
  start_postgres
  ensure_role_and_db
  apply_migrations
  seed_dev_drivers

  log "ready. start the service with:"
  printf 'cd %s/services/driver-taxi-service && go run ./cmd\n' "$ROOT_DIR"
}

main "$@"
