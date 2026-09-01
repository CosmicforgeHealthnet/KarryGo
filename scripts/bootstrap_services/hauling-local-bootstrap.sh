#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=scripts/bootstrap_services/_common.sh
source "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

LOG_LABEL="hauling-local"

PG_PORT="${PG_PORT:-5436}"
REDIS_PORT="${REDIS_PORT:-6383}"
PG_DATA_DIR="${PG_DATA_DIR:-$KARRYGO_DATA_DIR/postgres-hauling}"
REDIS_DATA_DIR="${REDIS_DATA_DIR:-$KARRYGO_DATA_DIR/redis-6383}"

ROLE_NAME="cosmicforge_logistics"
ROLE_PASSWORD="cosmicforge_logistics"
DB_NAME="hauling_service"

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
  local migration
  # Mirrors internal/database/migrate.go so new migrations are picked up
  # automatically instead of needing to be listed here.
  while IFS= read -r migration; do
    log "  -> $(basename "$migration")"
    "$PSQL_BIN" "$dsn" -f "$migration"
  done < <(list_migrations "$ROOT_DIR/services/driver-hauling-service/migrations")
}

seed_dev_providers() {
  local seed="$ROOT_DIR/services/driver-hauling-service/seeds/dev_seed.sql"
  if [ -f "$seed" ]; then
    log "seeding dev truck providers (idempotent)"
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
  seed_dev_providers

  log "ready. start the service with:"
  printf 'cd %s/services/driver-hauling-service && go run ./cmd\n' "$ROOT_DIR"
}

main "$@"
