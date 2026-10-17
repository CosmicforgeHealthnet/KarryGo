#!/usr/bin/env bash
# One-screen overview of every KarryGo service on this server.
#
#   status.sh
#
# Shows each service's container state, health, memory use and deployed version.
source "$(dirname "$0")/lib.sh"

printf '%-34s %-11s %-10s %-10s %s\n' SERVICE STATE HEALTH MEMORY VERSION
for key in $(all_services); do
  load_service "$key"
  # docker inspect prints an empty line for a missing container, so test the
  # output rather than relying on "|| echo".
  state="$(docker inspect -f '{{.State.Status}}' "$SVC_CONTAINER" 2>/dev/null | head -1 || true)"
  if [ -z "$state" ]; then
    printf '%-34s %-11s %-10s %-10s %s\n' "$key" "not-deployed" - - -
    continue
  fi
  health="$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}-{{end}}' "$SVC_CONTAINER" 2>/dev/null | head -1)"
  mem="$(docker stats --no-stream --format '{{.MemUsage}}' "$SVC_CONTAINER" 2>/dev/null | cut -d/ -f1 | head -1)"
  version="$(current_image || true)"
  version="${version##*:}"
  printf '%-34s %-11s %-10s %-10s %s\n' "$key" "$state" "$health" "${mem:--}" "${version:0:12}"
done
echo
echo "Server memory:"; free -h | sed -n '1,2p'
echo "Disk:"; df -h / | tail -1
