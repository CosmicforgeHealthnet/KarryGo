#!/usr/bin/env bash
set -euo pipefail

# Bootstrap local infrastructure for support-dispute-service.
# Postgres-only (this service does not use Redis). Mirrors the other
# bootstrap scripts (hauling/notification/etc.). All ports/paths/bins are
# overridable via environment variables.

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
PG_PORT="${PG_PORT:-5439}"
PG_DATA_DIR="${PG_DATA_DIR:-/tmp/postgres-support-dispute}"
PG_BIN="${PG_BIN:-$(command -v postgres >/dev/null 2>&1 && dirname "$(command -v postgres)" || echo /opt/homebrew/opt/postgresql@16/bin)}"
PSQL_BIN="${PSQL_BIN:-$(command -v psql)}"
INITDB_BIN="${INITDB_BIN:-$(command -v initdb)}"
PG_CTL_BIN="${PG_CTL_BIN:-$(command -v pg_ctl)}"
CREATEDB_BIN="${CREATEDB_BIN:-$(command -v createdb)}"

ROLE_NAME="${ROLE_NAME:-cosmicforge_logistics}"
ROLE_PASSWORD="${ROLE_PASSWORD:-cosmicforge_logistics}"
DB_NAME="${DB_NAME:-support_dispute_service}"

SERVICE_DIR="$ROOT_DIR/services/support-dispute-service"

log() {
  printf '\033[1;36m[support-dispute-local]\033[0m %s\n' "$*"
}

ensure_dir() {
  mkdir -p "$1"
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
  log "ensuring support-dispute role and database exist"
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
  log "applying support-dispute migrations"
  local dsn="postgres://${ROLE_NAME}:${ROLE_PASSWORD}@localhost:${PG_PORT}/${DB_NAME}?sslmode=disable"
  # Apply every .sql file in migrations/ in sorted order so new migrations are
  # picked up automatically. ON_ERROR_STOP makes a bad migration fail loudly.
  for migration in "$SERVICE_DIR"/migrations/*.sql; do
    log "  -> $(basename "$migration")"
    "$PSQL_BIN" "$dsn" -v ON_ERROR_STOP=1 -f "$migration"
  done
}

seed_help_articles() {
  local seed="$SERVICE_DIR/seeds/help_articles.sql"
  if [ -f "$seed" ]; then
    log "seeding help/FAQ articles (idempotent)"
    local dsn="postgres://${ROLE_NAME}:${ROLE_PASSWORD}@localhost:${PG_PORT}/${DB_NAME}?sslmode=disable"
    "$PSQL_BIN" "$dsn" -v ON_ERROR_STOP=1 -f "$seed"
  fi
}

main() {
  cd "$ROOT_DIR"
  start_postgres
  ensure_role_and_db
  apply_migrations
  seed_help_articles

  log "ready. start the service with:"
  printf 'cd %s/services/support-dispute-service && go run ./cmd\n' "$ROOT_DIR"
  log "(or with migrations handled by the service: MIGRATION=true go run ./cmd)"
}

main "$@"
