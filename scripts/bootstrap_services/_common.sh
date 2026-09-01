#!/usr/bin/env bash
# Shared prelude for the *-local-bootstrap.sh scripts. Sourced, never executed.
#
# Provides the bits every bootstrap needs identically -- data-dir base, binary
# discovery, locale, logging, and the Redis/Postgres start helpers -- so a fix
# lands in all of them at once instead of one script at a time.
#
# A sourcing script is expected to set LOG_LABEL (and optionally LOG_COLOR)
# before its first log call, and PG_PORT / PG_DATA_DIR (plus REDIS_PORT /
# REDIS_DATA_DIR if it uses Redis). Everything here honours pre-set values, so
# callers can still override any of it from the environment.

# Where local Postgres clusters and Redis dumps live.
#
# Deliberately NOT /tmp: macOS periodically sweeps /tmp, deleting files that
# have not been accessed for a few days but leaving the directory tree behind.
# That silently guts an idle cluster -- PG_VERSION and global/pg_control
# disappear while the directory survives -- and the next bootstrap then fails
# with "initdb: directory ... exists but is not empty".
KARRYGO_DATA_DIR="${KARRYGO_DATA_DIR:-$HOME/.karrygo/data}"

PG_BIN="${PG_BIN:-$(command -v postgres >/dev/null 2>&1 && dirname "$(command -v postgres)" || echo /opt/homebrew/opt/postgresql@16/bin)}"
PSQL_BIN="${PSQL_BIN:-$(command -v psql || true)}"
INITDB_BIN="${INITDB_BIN:-$(command -v initdb || true)}"
PG_CTL_BIN="${PG_CTL_BIN:-$(command -v pg_ctl || true)}"
CREATEDB_BIN="${CREATEDB_BIN:-$(command -v createdb || true)}"
# Postgres-only bootstraps (media, support-dispute) never call start_redis, so a
# missing redis-server must not abort them at discovery time.
REDIS_BIN="${REDIS_BIN:-$(command -v redis-server || true)}"

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
  printf '\033[%sm[%s]\033[0m %s\n' "${LOG_COLOR:-1;36}" "${LOG_LABEL:-bootstrap}" "$*"
}

ensure_dir() {
  mkdir -p "$1"
}

start_redis() {
  if lsof -nP -iTCP:"$REDIS_PORT" -sTCP:LISTEN >/dev/null 2>&1; then
    log "redis already listening on ${REDIS_PORT}"
    return
  fi

  if [ -z "$REDIS_BIN" ]; then
    log "redis-server not found on PATH (set REDIS_BIN to override)"
    return 1
  fi

  ensure_dir "$REDIS_DATA_DIR"
  log "starting redis on ${REDIS_PORT}"
  "$REDIS_BIN" --port "$REDIS_PORT" --dir "$REDIS_DATA_DIR" --daemonize yes
}

# empty | healthy | corrupt
cluster_state() {
  local dir="$1"

  if [ ! -d "$dir" ] || [ -z "$(ls -A "$dir" 2>/dev/null)" ]; then
    echo empty
  elif [ -f "$dir/PG_VERSION" ] && [ -f "$dir/global/pg_control" ]; then
    echo healthy
  else
    echo corrupt
  fi
}

# Only ever called for a directory that already failed the integrity check.
discard_corrupt_cluster() {
  local dir="$1"

  case "$dir" in
    ""|/|"$HOME"|"$HOME"/)
      log "refusing to delete '${dir}' -- set PG_DATA_DIR to a dedicated directory"
      return 1
      ;;
  esac

  log "found a partial/corrupt postgres cluster in ${dir}"
  log "  (missing PG_VERSION or global/pg_control -- unrecoverable, discarding it)"
  rm -rf "$dir"
}

start_postgres() {
  if lsof -nP -iTCP:"$PG_PORT" -sTCP:LISTEN >/dev/null 2>&1; then
    log "postgres already listening on ${PG_PORT}"
    return
  fi

  if [ "$(cluster_state "$PG_DATA_DIR")" = "corrupt" ]; then
    discard_corrupt_cluster "$PG_DATA_DIR"
  fi

  ensure_dir "$PG_DATA_DIR"

  if [ "$(cluster_state "$PG_DATA_DIR")" != "healthy" ]; then
    log "initializing postgres cluster in ${PG_DATA_DIR}"
    log "  locale ${PG_LOCALE}"
    "$INITDB_BIN" -D "$PG_DATA_DIR" --encoding=UTF8 --locale="$PG_LOCALE" >/dev/null
  fi

  log "starting postgres on ${PG_PORT}"
  "$PG_CTL_BIN" -D "$PG_DATA_DIR" -o "-p $PG_PORT" -l "$PG_DATA_DIR/postgres.log" start
}

# Echo the applyable migrations in <dir>, newline-separated, in numeric order
# with the .down.sql rollbacks filtered out.
#
# Both details matter. A bare *.sql glob also matches the down files and tries
# to drop what the up file just created; and glob order is lexicographic, so a
# directory mixing widths (001_x.sql alongside 000004_y.up.sql -- payment-wallet
# does exactly this) applies 000004 before 001. `sort -V` orders the numeric
# prefixes the way the services' own Go migration runners do.
list_migrations() {
  local dir="$1"
  ls "$dir"/*.sql 2>/dev/null | grep -v '\.down\.sql$' | sort -V
}
