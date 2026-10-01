#!/usr/bin/env bash
set -euo pipefail

# Bootstrap local infrastructure for pricing-service: Postgres on 5451.
#
# Pricing needs no Redis. A cache may later accelerate reads of immutable policy
# rows, but commercial truth lives only in Postgres, so the service must run
# correctly without one.
#
# Migrations are applied by the service's own tracked runner, so re-running this
# script is a no-op. All ports/paths/bins are overridable via environment
# variables.

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=scripts/bootstrap_services/_common.sh
source "$(dirname "${BASH_SOURCE[0]}")/_common.sh"

LOG_LABEL="pricing-local"

PG_PORT="${PG_PORT:-5451}"
PG_DATA_DIR="${PG_DATA_DIR:-$KARRYGO_DATA_DIR/postgres-pricing}"

ROLE_NAME="${ROLE_NAME:-cosmicforge_logistics}"
ROLE_PASSWORD="${ROLE_PASSWORD:-cosmicforge_logistics}"
DB_NAME="${DB_NAME:-pricing_service}"
SERVICE_DIR="$ROOT_DIR/services/pricing-service"

ensure_role_and_db() {
  log "ensuring pricing role and database exist"
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
  log "applying pricing-service migrations (tracked)"
  (cd "$SERVICE_DIR" && PRICING_DATABASE_URL="postgres://${ROLE_NAME}:${ROLE_PASSWORD}@localhost:${PG_PORT}/${DB_NAME}?sslmode=disable" go run ./cmd/migrate)
}

main() {
  cd "$ROOT_DIR"
  start_postgres
  ensure_role_and_db
  apply_migrations

  log "ready. start the service with:"
  printf 'cd %s/services/pricing-service && go run ./cmd\n' "$ROOT_DIR"
}

main "$@"
