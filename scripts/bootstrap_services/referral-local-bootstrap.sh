#!/usr/bin/env bash
set -euo pipefail

# Bootstrap local infrastructure for referral-service: Postgres on 5450 and
# Redis on 6391 (Redis only rate-limits the public code check). Migrations are
# applied by the service's own tracked runner, so re-running is a no-op. All
# ports/paths/bins are overridable via environment variables.

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=scripts/bootstrap_services/_common.sh
source "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

LOG_LABEL="referral-local"

PG_PORT="${PG_PORT:-5450}"
PG_DATA_DIR="${PG_DATA_DIR:-$KARRYGO_DATA_DIR/postgres-referral}"
REDIS_PORT="${REDIS_PORT:-6391}"
REDIS_DATA_DIR="${REDIS_DATA_DIR:-$KARRYGO_DATA_DIR/redis-${REDIS_PORT}}"

ROLE_NAME="${ROLE_NAME:-cosmicforge_logistics}"
ROLE_PASSWORD="${ROLE_PASSWORD:-cosmicforge_logistics}"
DB_NAME="${DB_NAME:-referral_service}"
SERVICE_DIR="$ROOT_DIR/services/referral-service"

ensure_role_and_db() {
  log "ensuring referral role and database exist"
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
  log "applying referral-service migrations (tracked)"
  (cd "$SERVICE_DIR" && REFERRAL_DATABASE_URL="postgres://${ROLE_NAME}:${ROLE_PASSWORD}@localhost:${PG_PORT}/${DB_NAME}?sslmode=disable" go run ./cmd/migrate)
}

main() {
  cd "$ROOT_DIR"
  start_redis
  start_postgres
  ensure_role_and_db
  apply_migrations

  log "ready. start the service with:"
  printf 'cd %s/services/referral-service && go run ./cmd\n' "$ROOT_DIR"
}

main "$@"
