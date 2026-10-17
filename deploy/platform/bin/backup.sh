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
[ -f "$CONF" ] && . "$CONF"

[ "$(id -u)" = 0 ] || { echo "run as root (sudo)" >&2; exit 1; }
log() { printf '[%s] %s\n' "$(date -u +%H:%M:%S)" "$*"; }

db_containers() {
  docker ps --format '{{.Names}}' | grep -E '^karrygo-.*-(db|postgres)$' || true
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

  for c in $(db_containers); do
    local user; user="$(container_env "$c" POSTGRES_USER)"; user="${user:-postgres}"
    log "database $c"
    docker exec "$c" pg_dumpall -U "$user" | gzip -9 >"$dir/$c.sql.gz"
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
  local scratch="karrygo-restore-test-$$"
  log "restoring $file into a throwaway container ($scratch)"
  docker run -d --name "$scratch" -e POSTGRES_PASSWORD=restore-test postgres:16-alpine >/dev/null
  trap 'docker rm -f "$scratch" >/dev/null 2>&1' EXIT
  until docker exec "$scratch" pg_isready -U postgres >/dev/null 2>&1; do sleep 2; done
  gunzip -c "$file" | docker exec -i "$scratch" psql -U postgres -q >/dev/null 2>"/tmp/$scratch.err" || true
  log "databases and table counts in the restored copy:"
  for db in $(docker exec "$scratch" psql -U postgres -tAc "SELECT datname FROM pg_database WHERE datname NOT IN ('postgres','template0','template1')"); do
    echo "  $db: $(docker exec "$scratch" psql -U postgres -d "$db" -tAc "SELECT count(*) FROM information_schema.tables WHERE table_schema NOT IN ('pg_catalog','information_schema')") tables"
  done
  log "restore test finished; the throwaway container is removed"
}

case "${1:-}" in
  "") take_backup ;;
  --install-cron)
    install -m 0755 "$(readlink -f "$0")" /usr/local/sbin/karrygo-backup
    echo "30 2 * * * root /usr/local/sbin/karrygo-backup >>/var/log/karrygo-backup.log 2>&1" >/etc/cron.d/karrygo-backup
    echo "Nightly backup installed (02:30). Log: /var/log/karrygo-backup.log" ;;
  --test-restore) test_restore "${2:?which database container? e.g. karrygo-customer-db}" ;;
  *) echo "usage: $0 [--install-cron | --test-restore <db-container>]" >&2; exit 2 ;;
esac
