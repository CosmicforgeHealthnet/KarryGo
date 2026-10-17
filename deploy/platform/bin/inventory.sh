#!/usr/bin/env bash
# Read-only snapshot of the server. Changes nothing and never prints secrets:
# private settings files are not opened, and anything that looks like a
# password in a compose file is replaced with ***hidden***.
#
#   inventory.sh
#
# Run it before any change, and keep the output with your notes.
set -uo pipefail

section() { printf '\n### %s\n' "$*"; }

section "SERVER"
. /etc/os-release && echo "$PRETTY_NAME"
echo "CPUs: $(nproc)"; free -h; df -h /

section "DOCKER"
docker --version
docker compose version 2>/dev/null || echo "docker compose plugin: NOT installed"
docker-compose --version 2>/dev/null || true

section "CONTAINERS"
docker ps -a --format 'table {{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Ports}}'

section "HOW KARRYGO CONTAINERS WERE STARTED"
for c in $(docker ps -a --format '{{.Names}}' | grep '^karrygo-'); do
  printf '%-48s compose-project=%s\n' "$c" "$(docker inspect -f '{{index .Config.Labels "com.docker.compose.project"}}' "$c")"
done

section "VOLUMES"
docker volume ls

section "/opt/karrygo"
ls -la /opt/karrygo /opt/karrygo/* 2>&1

section "KARRYGO COMPOSE FILES (passwords hidden)"
for f in /opt/karrygo/*/docker-compose*.y*ml /opt/karrygo/platform/compose/*.yml; do
  [ -f "$f" ] || continue
  echo "== $f"
  sed -E 's/((PASSWORD|SECRET|TOKEN|KEY)[A-Za-z_]*["]?[:=][ ]*).*/\1***hidden***/I' "$f"
done

section "DEPLOYED VERSIONS"
for f in /opt/karrygo/*/release.env; do [ -f "$f" ] && echo "$f: $(cat "$f")"; done

section "LISTENING PORTS"
ss -tlnp

section "NGINX"
nginx -v 2>&1; ls /etc/nginx/sites-enabled /etc/nginx/conf.d 2>&1
ls /etc/letsencrypt/live 2>&1

section "FIREWALL"
ufw status 2>&1
iptables -S DOCKER-USER 2>/dev/null | grep karrygo || echo "no KarryGo DOCKER-USER block yet"

section "SWAP"
swapon --show; [ "$(swapon --show | wc -l)" -eq 0 ] && echo "no swap"
exit 0
