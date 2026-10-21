#!/usr/bin/env bash
# Nightly backup of everything KarryGo needs to come back after a disaster.
# Run as root.
#
#   backup.sh                     take a backup now
#   backup.sh --install-cron      run it every night at 02:30 (server time)
#   backup.sh --test-restore <db-container>
#                                 prove the newest backup of that database works,
#                                 by restoring it into a throwaway container
#
# What is saved, into /opt/karrygo/backups/<date-time>/:
#   - every KarryGo PostgreSQL container (all its databases), compressed
#   - all private settings files under /opt/karrygo, ENCRYPTED with your
#     backup passphrase (stored in /root/.karrygo-backup-passphrase, mode 600)
#   - dispatch's uploaded files (the karrygo-dispatch-uploads volume)
# Backups older than 7 days are deleted.
#
# Optional off-server copy: install rclone, configure a remote (rclone config),
# then put   RCLONE_REMOTE=<remote>:<folder>   in /etc/karrygo-backup.conf.
set -euo pipefail

BACKUP_ROOT=/opt/karrygo/backups
KEEP_DAYS=7
PASSFILE=/root/.karrygo-backup-passphrase
CONF=/etc/karrygo-backup.conf
VALIDATION_ROOT=/etc/karrygo/restore-validation
RESTORE_HELPER="${RESTORE_HELPER:-$(dirname "$(readlink -f "$0")")/../../../scripts/release/restore.py}"
[ -f "$CONF" ] && . "$CONF"

[ "$(id -u)" = 0 ] || { echo "run as root (sudo)" >&2; exit 1; }
log() { printf '[%s] %s\n' "$(date -u +%H:%M:%S)" "$*"; }

db_containers() {
  local names
  names="$(docker ps --format '{{.Names}}')" || return 1
  printf '%s\n' "$names" | grep -E '^karrygo-.*-(db|postgres)$'
}

# Reads a setting from a container's configuration without printing it.
container_env() { # container name
  docker inspect -f '{{range .Config.Env}}{{println .}}{{end}}' "$1" | sed -n "s/^$2=//p" | head -1
}

take_backup() {
  [ -f "$PASSFILE" ] || {
    echo "Missing $PASSFILE. Create it once with your backup passphrase:" >&2
    echo "  sudo sh -c 'umask 077; cat > $PASSFILE'   (type the phrase, Enter, then Ctrl-D)" >&2
    exit 1
  }
  local dir="$BACKUP_ROOT/$(date -u +%Y%m%d-%H%M%S)"
  install -d -m 700 "$dir"

  local containers
  containers="$(db_containers)"
  [ -n "$containers" ] || { echo "no source databases discovered" >&2; exit 1; }
  for c in $containers; do
    local user; user="$(container_env "$c" POSTGRES_USER)"; user="${user:-postgres}"
    log "database $c"
    [ -s "$VALIDATION_ROOT/$c.json" ] || { echo "missing operator validation: $c" >&2; exit 1; }
    # Destructive clean SQL is restored ONLY into a newly created sandbox with
    # a distinct bootstrap role. Never feed it to any existing/live database.
    docker exec "$c" pg_dumpall --clean --if-exists -U "$user" | gzip -9 >"$dir/$c.sql.gz"
    python3 "$RESTORE_HELPER" record --container "$c" --user "$user" \
      --archive "$dir/$c.sql.gz" --manifest "$dir/$c.manifest.json" --queries "$VALIDATION_ROOT/$c.json"
  done

  log "settings files (encrypted)"
  tar -C /opt/karrygo -czf - --exclude=backups --exclude=platform \
      $(cd /opt/karrygo && find . -maxdepth 2 -type f \( -name '*.env' -o -name '*.env.*' -o -name '.env' \) | sed 's|^\./||') \
    | openssl enc -aes-256-cbc -pbkdf2 -salt -pass "file:$PASSFILE" -out "$dir/settings.tar.gz.enc"

  if docker volume inspect karrygo-dispatch-uploads >/dev/null 2>&1; then
    log "dispatch uploads"
    tar -C "$(docker volume inspect -f '{{.Mountpoint}}' karrygo-dispatch-uploads)" -czf "$dir/dispatch-uploads.tar.gz" .
  fi

  chmod 600 "$dir"/*
  log "saved to $dir ($(du -sh "$dir" | cut -f1))"

  find "$BACKUP_ROOT" -mindepth 1 -maxdepth 1 -type d -mtime +"$KEEP_DAYS" -exec rm -rf {} +

  if [ -n "${RCLONE_REMOTE:-}" ] && command -v rclone >/dev/null; then
    log "copying off the server to $RCLONE_REMOTE"
    rclone copy "$dir" "$RCLONE_REMOTE/$(basename "$dir")"
  fi
}

test_restore() { # container
  local c="$1" latest file
  latest="$(ls -1d "$BACKUP_ROOT"/2* 2>/dev/null | tail -1)"
  file="$latest/$c.sql.gz"
  [ -f "$file" ] || { echo "no backup of $c in $latest" >&2; exit 1; }
  python3 "$RESTORE_HELPER" verify --archive "$file" --manifest "$latest/$c.manifest.json" \
    --output "$latest/$c.restore-evidence.json"
  log "strict isolated restore verified; evidence: $latest/$c.restore-evidence.json"
}

case "${1:-}" in
  "") take_backup ;;
  --install-cron)
    # Installed script needs an absolute helper path; never silently fall back
    # to an obsolete copied implementation.
    test -s "$RESTORE_HELPER"
    install -d -m 750 /opt/karrygo/platform/restore-tools
    install -m 0755 "$RESTORE_HELPER" /opt/karrygo/platform/restore-tools/restore.py
    install -m 0755 "$(readlink -f "$0")" /usr/local/sbin/karrygo-backup
    echo "30 2 * * * root RESTORE_HELPER=/opt/karrygo/platform/restore-tools/restore.py /usr/local/sbin/karrygo-backup >>/var/log/karrygo-backup.log 2>&1" >/etc/cron.d/karrygo-backup
    echo "Nightly backup installed (02:30). Log: /var/log/karrygo-backup.log" ;;
  --test-restore) test_restore "${2:?which database container? e.g. karrygo-customer-db}" ;;
  *) echo "usage: $0 [--install-cron | --test-restore <db-container>]" >&2; exit 2 ;;
esac
