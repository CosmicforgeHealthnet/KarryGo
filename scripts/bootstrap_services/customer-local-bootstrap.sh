#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=scripts/bootstrap_services/_common.sh
source "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

LOG_LABEL="customer-local"

PG_PORT="${PG_PORT:-5433}"
REDIS_PORT="${REDIS_PORT:-6380}"
PG_DATA_DIR="${PG_DATA_DIR:-$KARRYGO_DATA_DIR/postgres-customer}"
REDIS_DATA_DIR="${REDIS_DATA_DIR:-$KARRYGO_DATA_DIR/redis-6380}"

ROLE_NAME="cosmicforge_logistics"
ROLE_PASSWORD="cosmicforge_logistics"
DB_NAME="customer_service"

ensure_customer_role_and_db() {
  log "ensuring customer role and database exist"
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
  log "applying customer migrations"
  local url="postgres://${ROLE_NAME}:${ROLE_PASSWORD}@localhost:${PG_PORT}/${DB_NAME}?sslmode=disable"
  local migration
  for migration in "$ROOT_DIR/services/customer-service/migrations/"*.sql; do
    log "  -> $(basename "$migration")"
    "$PSQL_BIN" "$url" -f "$migration"
  done
}

main() {
  cd "$ROOT_DIR"
  start_redis
  start_postgres
  ensure_customer_role_and_db
  apply_migrations

  log "ready. start the service with:"
  printf 'cd %s/services/customer-service && MIGRATION=true go run ./cmd\n' "$ROOT_DIR"
}

main "$@"
