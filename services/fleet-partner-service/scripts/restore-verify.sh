#!/usr/bin/env bash
# fleet-partner-service PostgreSQL restore verification.
#
# Restores a backup produced by backup.sh into a NEW, disposable database on
# the same PostgreSQL server (never overwriting or touching the source/
# production database), then verifies migration version, critical-table
# presence and row counts, and runs a few smoke queries. Drops the
# disposable database on exit either way.
#
# Usage:
#   FLEET_PARTNER_BACKUP_DATABASE_URL='postgres://user:pass@host:port/db?sslmode=require' \
#     bash services/fleet-partner-service/scripts/restore-verify.sh /path/to/backup-dir/fleet-partner-service-<timestamp>.dump
#
# FLEET_PARTNER_BACKUP_DOCKER_CONTAINER works exactly as in backup.sh: set it
# when pg_dump/pg_restore/psql/createdb/dropdb are not on the host PATH but
# are available inside a running postgres container — in that mode,
# FLEET_PARTNER_BACKUP_DATABASE_URL must be the connection string AS SEEN
# FROM INSIDE THAT CONTAINER (see backup.sh's own note).
set -euo pipefail

DUMP_FILE="${1:-}"
if [ -z "$DUMP_FILE" ] || [ ! -f "$DUMP_FILE" ]; then
  echo "usage: $0 <path-to-dump-file>" >&2
  exit 1
fi
if [ -z "${FLEET_PARTNER_BACKUP_DATABASE_URL:-}" ]; then
  echo "RESTORE FAIL: FLEET_PARTNER_BACKUP_DATABASE_URL is not set (used only to derive host/user/port, never to write to the source database)." >&2
  exit 1
fi

CHECKSUM_FILE="${DUMP_FILE}.sha256"
if [ -f "$CHECKSUM_FILE" ]; then
  echo "== verifying checksum =="
  if command -v sha256sum >/dev/null 2>&1; then
    ( cd "$(dirname "$DUMP_FILE")" && sha256sum -c "$(basename "$CHECKSUM_FILE")" )
  else
    ( cd "$(dirname "$DUMP_FILE")" && shasum -a 256 -c "$(basename "$CHECKSUM_FILE")" )
  fi
  echo "checksum OK"
else
  echo "checksum file not found ($CHECKSUM_FILE) — skipping verification, proceeding with restore" >&2
fi

# All-digits timestamp (no T/Z separators): CREATE DATABASE with an
# unquoted identifier folds to lowercase, and an uppercase-letter-bearing
# name (e.g. from %Y%m%dT%H%M%SZ) would silently create a lowercase database
# that a later mixed-case comparison/URL then fails to find.
DISPOSABLE_DB="fleet_partner_restore_verify_$(date -u +%Y%m%d%H%M%S)_$$"
START_EPOCH=$(date +%s)

run_docker_or_local() {
  local cmd="$1"; shift
  if [ -n "${FLEET_PARTNER_BACKUP_DOCKER_CONTAINER:-}" ]; then
    docker exec -i "$FLEET_PARTNER_BACKUP_DOCKER_CONTAINER" "$cmd" "$@"
  else
    "$cmd" "$@"
  fi
}

cleanup() {
  echo "== dropping disposable database $DISPOSABLE_DB =="
  run_docker_or_local psql -X -d "$FLEET_PARTNER_BACKUP_DATABASE_URL" -c "DROP DATABASE IF EXISTS ${DISPOSABLE_DB}" >/dev/null 2>&1 || true
}
trap cleanup EXIT

echo "== creating disposable database $DISPOSABLE_DB =="
run_docker_or_local psql -X -d "$FLEET_PARTNER_BACKUP_DATABASE_URL" -c "CREATE DATABASE ${DISPOSABLE_DB}"

# Build the disposable database's own connection string by swapping the
# path component of the source URL — never touches the source database.
DISPOSABLE_URL="$(python3 - "$FLEET_PARTNER_BACKUP_DATABASE_URL" "$DISPOSABLE_DB" <<'PYEOF' 2>/dev/null || true
import sys
from urllib.parse import urlsplit, urlunsplit
url, dbname = sys.argv[1], sys.argv[2]
parts = urlsplit(url)
new_path = "/" + dbname
print(urlunsplit((parts.scheme, parts.netloc, new_path, parts.query, parts.fragment)))
PYEOF
)"
if [ -z "$DISPOSABLE_URL" ]; then
  # python3 unavailable: fall back to a plain sed swap of the last path segment.
  DISPOSABLE_URL="$(echo "$FLEET_PARTNER_BACKUP_DATABASE_URL" | sed -E "s#(/[A-Za-z0-9_]+)(\?.*)?\$#/${DISPOSABLE_DB}\2#")"
fi

echo "== restoring into disposable database =="
if [ -n "${FLEET_PARTNER_BACKUP_DOCKER_CONTAINER:-}" ]; then
  docker exec -i "$FLEET_PARTNER_BACKUP_DOCKER_CONTAINER" pg_restore --no-owner --no-privileges -d "$DISPOSABLE_URL" < "$DUMP_FILE"
else
  pg_restore --no-owner --no-privileges -d "$DISPOSABLE_URL" "$DUMP_FILE"
fi

query() {
  run_docker_or_local psql -X -A -t -d "$DISPOSABLE_URL" -c "$1" | tr -d '[:space:]'
}

echo "== verifying migration version =="
RESTORED_VERSION="$(query "SELECT version FROM fleet_partner_schema_migrations ORDER BY version DESC LIMIT 1")"
if [ -z "$RESTORED_VERSION" ]; then
  echo "RESTORE FAIL: no migration version found after restore." >&2
  exit 1
fi
echo "restored schema version: $RESTORED_VERSION"

echo "== verifying critical tables and smoke queries =="
FAIL=0
for TABLE in \
  fleet_partner_organizations fleet_partner_private_orders fleet_partner_private_assignments \
  fleet_partner_documents fleet_partner_outbox fleet_partner_audit_records \
  fleet_partner_jobs fleet_partner_service_credentials; do
  COUNT="$(query "SELECT count(*) FROM ${TABLE}")"
  if [ -z "$COUNT" ]; then
    echo "RESTORE FAIL: table ${TABLE} is not queryable after restore." >&2
    FAIL=1
  else
    echo "  ${TABLE}: ${COUNT} rows"
  fi
done

echo "== verifying append-only audit chain protection survives restore =="
TRIGGER_COUNT="$(query "SELECT count(*) FROM pg_trigger t JOIN pg_class c ON c.oid=t.tgrelid WHERE c.relname='fleet_partner_audit_records' AND NOT t.tgisinternal")"
if [ "${TRIGGER_COUNT:-0}" = "0" ]; then
  echo "RESTORE FAIL: fleet_partner_audit_records has no protective trigger after restore." >&2
  FAIL=1
else
  echo "  audit append-only trigger count: $TRIGGER_COUNT"
fi

END_EPOCH=$(date +%s)
DURATION=$((END_EPOCH - START_EPOCH))
echo "== restore verification duration: ${DURATION}s =="

if [ "$FAIL" != "0" ]; then
  echo "RESTORE FAIL"
  exit 1
fi
echo "RESTORE PASS"
