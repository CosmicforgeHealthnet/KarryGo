#!/usr/bin/env bash
# Saves a running container's settings into a private settings file.
#
#   export-container-env.sh <container> <output-file>
#
# Some KarryGo containers were started by hand with their settings typed into
# the start command, so those settings exist nowhere except inside the running
# container. This writes them to <output-file> (mode 600) so the service can be
# recreated exactly. If the container also reads a ".env" file mounted from the
# server, its lines are included too (values given on the command line win).
#
# It never prints any value: only the number of settings and their names.
# It refuses to overwrite an existing file.
source "$(dirname "$0")/lib.sh"

[ $# -eq 2 ] || die "usage: export-container-env.sh <container> <output-file>"
container="$1"; out="$2"
docker inspect "$container" >/dev/null 2>&1 || die "no container named $container"
[ -e "$out" ] && die "$out already exists; move it away first (nothing was changed)"

umask 077
tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT

# Settings the image itself sets (PATH and friends) are not service settings.
image_env="$(docker image inspect -f '{{range .Config.Env}}{{println .}}{{end}}' "$(docker inspect -f '{{.Image}}' "$container")" | cut -d= -f1)"

# 1. Settings given on the command line / via --env-file.
docker inspect -f '{{range .Config.Env}}{{println .}}{{end}}' "$container" \
  | grep -vE '^$' \
  | while IFS= read -r line; do
      key="${line%%=*}"
      echo "$image_env" | grep -qxF "$key" && continue
      [ "$key" = "HOSTNAME" ] && continue
      printf '%s\n' "$line"
    done >"$tmp"

# 2. A mounted .env file, for settings not already set above.
dotenv="$(docker inspect -f '{{range .Mounts}}{{if eq .Type "bind"}}{{.Source}}|{{.Destination}}{{println}}{{end}}{{end}}' "$container" \
  | awk -F'|' '$2 ~ /\/\.env$/ { print $1; exit }')"
if [ -n "$dotenv" ] && [ -f "$dotenv" ]; then
  while IFS= read -r line || [ -n "$line" ]; do
    case "$line" in ''|\#*) continue ;; esac
    line="${line#export }"
    key="${line%%=*}"
    grep -q "^${key}=" "$tmp" || printf '%s\n' "$line" >>"$tmp"
  done <"$dotenv"
fi

{
  echo "# Settings exported from container $container on $(date -u +%Y-%m-%dT%H:%MZ)."
  echo "# Private: keep this file mode 600 and never commit it."
  cat "$tmp"
} >"$out"
chmod 600 "$out"

log "wrote $(grep -cvE '^(#|$)' "$out") settings to $out (mode 600). Names:"
grep -vE '^(#|$)' "$out" | cut -d= -f1 | sort | tr '\n' ' '; echo
[ -n "$dotenv" ] && log "included the mounted settings file $dotenv"
exit 0
