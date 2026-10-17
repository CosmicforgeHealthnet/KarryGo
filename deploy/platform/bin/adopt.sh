#!/usr/bin/env bash
# Moves a service that was started by hand under its compose file, safely.
#
#   adopt.sh <service-key> <image>
#
# Before running it, the service's settings file must exist (use
# export-container-env.sh to capture a hand-started container's settings).
#
# What it does:
#   1. stops each old container and renames it <name>-pre-adoption-<time>,
#      keeping it as an instant fallback (its data volumes are untouched)
#   2. dispatch only: copies the uploaded photos out of the old container onto
#      a permanent volume (they used to live in the container's /tmp)
#   3. deploys the service with deploy-service.sh (health check included)
#   4. if that fails: removes the new containers, renames the old ones back
#      and starts them again
#
# The old containers are left stopped afterwards. Delete them once you are
# happy (the command is printed at the end).
source "$(dirname "$0")/lib.sh"

[ $# -eq 2 ] || die "usage: adopt.sh <service-key> <image>"
load_service "$1"
IMAGE="$2"
check_service_files
stamp="$(date -u +%Y%m%d-%H%M%S)"

# Every container this compose file will create, and the ones that exist now.
names="$(awk '/^    container_name:/ { print $2 }' "$SVC_COMPOSE")"
moved=()

# The old containers are the fallback, so nothing they read may change. If an
# old container mounts the very file the compose file uses for its settings,
# preparing that file (mode 600, owned by deploy) would make it unreadable to
# the old container: a restored fallback would silently start on developer
# defaults. That happened once with notification; refuse instead.
for name in $names; do
  docker inspect "$name" >/dev/null 2>&1 || continue
  if docker inspect -f '{{range .Mounts}}{{.Source}}{{println}}{{end}}' "$name" | grep -qxF "$SVC_ENV_PATH"; then
    die "$name reads $SVC_ENV_PATH through a mount. Use a different settings file name for the compose file (services.tsv + compose/$SVC_KEY.yml) and leave that file untouched."
  fi
done

restore_old() {
  warn "restoring the old containers"
  RELEASE_OVERRIDE=/dev/null IMAGE=unused svc_compose rm -sf >/dev/null 2>&1 || true
  for pair in "${moved[@]}"; do
    old="${pair%%|*}"; backup="${pair##*|}"
    docker rm -f "$old" >/dev/null 2>&1 || true
    docker rename "$backup" "$old"
    docker start "$old" >/dev/null
    log "restored $old"
  done
}

# 1. Move the old containers aside.
for name in $names; do
  if docker inspect "$name" >/dev/null 2>&1; then
    project="$(docker inspect -f '{{index .Config.Labels "com.docker.compose.project"}}' "$name")"
    if [ "$project" = "karrygo-$SVC_KEY" ]; then
      log "$name is already managed by this compose file; nothing to adopt"
      continue
    fi
    backup="$name-pre-adoption-$stamp"
    log "stopping $name and keeping it as $backup"
    docker stop "$name" >/dev/null
    docker rename "$name" "$backup"
    moved+=("$name|$backup")
  fi
done

# 2. Dispatch's uploaded files (avatars, verification documents, trip proofs).
# Copied after the old container is stopped, so no upload can slip in mid-copy.
old_dispatch=""
for pair in "${moved[@]:-}"; do [ "${pair%%|*}" = "$SVC_CONTAINER" ] && old_dispatch="${pair##*|}"; done
if [ "$SVC_KEY" = "driver-dispatch-delivery-service" ] && [ -n "$old_dispatch" ]; then
  log "copying dispatch's uploaded files onto the permanent volume karrygo-dispatch-uploads"
  docker volume create karrygo-dispatch-uploads >/dev/null
  staging="$(mktemp -d)"
  for pair in avatar:avatars verification:verification trip-proof:trip-proofs; do
    src="/tmp/cosmicforge-${pair%%:*}-uploads"
    mkdir -p "$staging/${pair##*:}"
    if docker cp "$old_dispatch:$src/." "$staging/${pair##*:}/" 2>/dev/null; then
      log "  copied $src ($(find "$staging/${pair##*:}" -type f | wc -l) files)"
    else
      log "  $src does not exist in the old container (nothing to copy)"
    fi
  done
  docker image inspect "$IMAGE" >/dev/null 2>&1 || docker pull -q "$IMAGE" >/dev/null
  if ! docker run --rm --user 0 --entrypoint sh \
    -v "$staging:/src:ro" -v karrygo-dispatch-uploads:/data/uploads "$IMAGE" \
    -c 'cp -a /src/. /data/uploads/ && chown -R cosmicforge:cosmicforge /data/uploads'; then
    rm -rf "$staging"
    restore_old
    die "could not copy dispatch's uploaded files; the old container is running again"
  fi
  rm -rf "$staging"
fi

# 3. Deploy under compose.
if "$(dirname "$0")/deploy-service.sh" "$SVC_KEY" "$IMAGE"; then
  log "== $SVC_KEY is adopted and healthy."
  if [ "${#moved[@]}" -gt 0 ]; then
    log "The old containers are stopped but kept. Once you are happy, delete them with:"
    for pair in "${moved[@]}"; do echo "    docker rm ${pair##*|}"; done
  fi
  exit 0
fi

# 4. Put the old ones back.
restore_old
die "adoption of $SVC_KEY failed; the old containers are running again"
