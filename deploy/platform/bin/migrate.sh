#!/bin/sh
# Applies a service's database migrations exactly once each, in order.
#
# Used only by services with no migration runner of their own (migrate=script
# in services.tsv: api-gateway, dispatch, payment-wallet). It runs INSIDE the
# service's image (services/Dockerfile copies it to /srv/app/migrate.sh), where
# the migration files and psql already are. deploy-service.sh calls it with:
#   docker compose run --rm -e MIGRATE_DB_URL_VAR=<setting name> --entrypoint /srv/app/migrate.sh app
#
# Modes:
#   migrate.sh                     apply every file not yet recorded
#   migrate.sh --status            list applied and pending files, change nothing
#   migrate.sh --baseline          record every file as applied WITHOUT running it
#   migrate.sh --baseline --until <file>
#                                  record files up to and including <file> only
#
# --baseline is for a database whose tables were created some other way (by
# hand, or by an older deploy). It tells this script "these are already done".
# Only use it after checking the database really has those tables.
#
# A record of every applied file is kept in the karrygo_deploy_migrations table.
# Files are ordered by their leading number (so 001_x.sql comes before
# 000004_y.up.sql, matching the services' own runners); *.down.sql and
# *_down.sql files are never applied.
set -eu

MIGRATIONS_DIR="${MIGRATIONS_DIR:-/srv/app/migrations}"
VAR="${MIGRATE_DB_URL_VAR:?set MIGRATE_DB_URL_VAR to the name of the database-address setting}"
eval "DB_URL=\${$VAR:-}"
[ -n "$DB_URL" ] || { echo "migrate.sh: the setting $VAR is empty or missing in the settings file" >&2; exit 1; }

MODE=apply
UNTIL=""
while [ $# -gt 0 ]; do
  case "$1" in
    --status) MODE=status ;;
    --baseline) MODE=baseline ;;
    --until) shift; UNTIL="${1:?--until needs a file name}" ;;
    *) echo "migrate.sh: unknown option $1" >&2; exit 2 ;;
  esac
  shift
done

psql_q() { psql "$DB_URL" -v ON_ERROR_STOP=1 -X -q -tA "$@"; }

psql_q -c "CREATE TABLE IF NOT EXISTS karrygo_deploy_migrations (
  filename   TEXT PRIMARY KEY,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  how        TEXT NOT NULL
);" >/dev/null

# Migration files in order: by leading number, then by name.
ordered_files() {
  ls -1 "$MIGRATIONS_DIR" 2>/dev/null \
    | grep -E '\.sql$' \
    | grep -vE '(\.down\.sql|_down\.sql)$' \
    | awk 'match($0, /^[0-9]+/) { printf "%d\t%s\n", substr($0, RSTART, RLENGTH) + 0, $0; next } { printf "999999999\t%s\n", $0 }' \
    | sort -n -s -k1,1 \
    | cut -f2
}

# Queries are piped in (not passed with -c) because psql only substitutes
# :'variables' in input it reads, which also keeps file names safely quoted.
is_applied() {
  [ "$(echo "SELECT 1 FROM karrygo_deploy_migrations WHERE filename = :'f';" | psql_q -v f="$1" 2>/dev/null || true)" = "1" ]
}

record() { # file how
  echo "INSERT INTO karrygo_deploy_migrations (filename, how) VALUES (:'f', :'h') ON CONFLICT DO NOTHING;" \
    | psql_q -v f="$1" -v h="$2" >/dev/null
}

files="$(ordered_files)"
[ -n "$files" ] || { echo "migrate.sh: no migration files in $MIGRATIONS_DIR - nothing to do"; exit 0; }

if [ -n "$UNTIL" ] && ! echo "$files" | grep -qxF "$UNTIL"; then
  echo "migrate.sh: --until file '$UNTIL' is not in $MIGRATIONS_DIR" >&2
  exit 2
fi

applied=0 pending=0
for f in $files; do
  if is_applied "$f"; then
    [ "$MODE" = status ] && echo "  applied  $f"
    applied=$((applied + 1))
  else
    case "$MODE" in
      status)
        echo "  PENDING  $f"
        pending=$((pending + 1)) ;;
      baseline)
        record "$f" baseline
        echo "  recorded $f (baseline, not run)"
        applied=$((applied + 1)) ;;
      apply)
        echo "  applying $f"
        psql "$DB_URL" -v ON_ERROR_STOP=1 -X -q -f "$MIGRATIONS_DIR/$f"
        record "$f" applied
        applied=$((applied + 1)) ;;
    esac
  fi
  if [ -n "$UNTIL" ] && [ "$f" = "$UNTIL" ]; then
    break
  fi
done

case "$MODE" in
  status)   echo "migrate.sh: $applied applied, $pending pending" ;;
  baseline) echo "migrate.sh: baseline complete ($applied files recorded)" ;;
  apply)    echo "migrate.sh: database is up to date ($applied files)" ;;
esac
