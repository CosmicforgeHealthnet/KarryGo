#!/usr/bin/env bash
# fleet-partner-service PostgreSQL backup.
#
# Produces a pg_dump custom-format (-Fc) backup, a sha256 checksum file, and
# a metadata JSON (schema version, row counts for critical tables, service
# name, timestamp) alongside it. No credential is ever embedded in the
# backup file name or metadata; the connection is read entirely from
# FLEET_PARTNER_BACKUP_DATABASE_URL (never printed).
#
# Usage:
#   FLEET_PARTNER_BACKUP_DATABASE_URL='postgres://user:pass@host:port/db?sslmode=require' \
#     bash services/fleet-partner-service/scripts/backup.sh /path/to/backup-dir
#
# If pg_dump is not on PATH (this repository's own dev/CI containers do not
# install PostgreSQL client tools on the host), set
# FLEET_PARTNER_BACKUP_DOCKER_CONTAINER to the name of a running postgres
# container, and every pg_dump/psql invocation below runs via `docker exec`
# against it instead — in that mode, FLEET_PARTNER_BACKUP_DATABASE_URL must
# be the connection string AS SEEN FROM INSIDE THAT CONTAINER (typically
# host "localhost" and PostgreSQL's own default port 5432, NOT the
# docker-compose host-side port mapping you would use to connect from
# outside).
#
# Encryption/off-host destination and retention/rotation are operator
# concerns layered on top of this script's OUTPUT (a single backup set in
# --destination) — see docs/section29-reliability-observability.md's Backup
# and Restore section for exactly what is and is not automated here.
set -euo pipefail

DESTINATION="${1:-}"
if [ -z "$DESTINATION" ]; then
  echo "usage: $0 <destination-directory>" >&2
  exit 1
fi
if [ -z "${FLEET_PARTNER_BACKUP_DATABASE_URL:-}" ]; then
  echo "BACKUP FAIL: FLEET_PARTNER_BACKUP_DATABASE_URL is not set." >&2
  exit 1
fi

mkdir -p "$DESTINATION"
# Restricted access: the destination and every file this script writes
# should only be readable by the operator/service account running it.
chmod 700 "$DESTINATION" 2>/dev/null || true

TIMESTAMP="$(date -u +%Y%m%dT%H%M%SZ)"
BASENAME="fleet-partner-service-${TIMESTAMP}"
DUMP_FILE="${DESTINATION}/${BASENAME}.dump"
META_FILE="${DESTINATION}/${BASENAME}.meta.json"
CHECKSUM_FILE="${DESTINATION}/${BASENAME}.dump.sha256"

run_pg_dump() {
  if [ -n "${FLEET_PARTNER_BACKUP_DOCKER_CONTAINER:-}" ]; then
    docker exec -e PGCONNECT_TIMEOUT=10 "$FLEET_PARTNER_BACKUP_DOCKER_CONTAINER" \
      pg_dump -Fc --no-owner --no-privileges -d "$FLEET_PARTNER_BACKUP_DATABASE_URL" > "$DUMP_FILE"
  else
    pg_dump -Fc --no-owner --no-privileges -d "$FLEET_PARTNER_BACKUP_DATABASE_URL" > "$DUMP_FILE"
  fi
}

run_psql_scalar() {
  # $1 = SQL. Never echoes the connection string.
  if [ -n "${FLEET_PARTNER_BACKUP_DOCKER_CONTAINER:-}" ]; then
    docker exec "$FLEET_PARTNER_BACKUP_DOCKER_CONTAINER" psql -X -A -t -d "$FLEET_PARTNER_BACKUP_DATABASE_URL" -c "$1"
  else
    psql -X -A -t -d "$FLEET_PARTNER_BACKUP_DATABASE_URL" -c "$1"
  fi
}

echo "== creating backup =="
run_pg_dump
echo "backup written: $DUMP_FILE"

echo "== checksum =="
if command -v sha256sum >/dev/null 2>&1; then
  ( cd "$DESTINATION" && sha256sum "$(basename "$DUMP_FILE")" > "$(basename "$CHECKSUM_FILE")" )
else
  ( cd "$DESTINATION" && shasum -a 256 "$(basename "$DUMP_FILE")" > "$(basename "$CHECKSUM_FILE")" )
fi
echo "checksum written: $CHECKSUM_FILE"

echo "== schema/version metadata =="
SCHEMA_VERSION="$(run_psql_scalar "SELECT version FROM fleet_partner_schema_migrations ORDER BY version DESC LIMIT 1" | tr -d '[:space:]')"
ORG_COUNT="$(run_psql_scalar "SELECT count(*) FROM fleet_partner_organizations" | tr -d '[:space:]')"
AUDIT_COUNT="$(run_psql_scalar "SELECT count(*) FROM fleet_partner_audit_records" | tr -d '[:space:]')"
JOB_COUNT="$(run_psql_scalar "SELECT count(*) FROM fleet_partner_jobs" | tr -d '[:space:]')"

cat > "$META_FILE" <<EOF
{
  "service": "fleet-partner-service",
  "createdAtUtc": "${TIMESTAMP}",
  "schemaVersion": "${SCHEMA_VERSION}",
  "format": "postgresql-custom",
  "rowCounts": {
    "fleet_partner_organizations": ${ORG_COUNT:-0},
    "fleet_partner_audit_records": ${AUDIT_COUNT:-0},
    "fleet_partner_jobs": ${JOB_COUNT:-0}
  }
}
EOF
echo "metadata written: $META_FILE"
echo "BACKUP PASS"
