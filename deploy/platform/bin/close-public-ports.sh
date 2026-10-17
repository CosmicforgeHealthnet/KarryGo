#!/usr/bin/env bash
# Blocks internet access to KarryGo container ports that are published on all
# addresses, without restarting anything. Run as root.
#
#   close-public-ports.sh            block them now and keep them blocked after reboots
#   close-public-ports.sh --check    show whether the block is in place
#   close-public-ports.sh --undo     remove the block
#
# Why this is needed: Docker writes its own firewall rules, which run BEFORE
# Ubuntu's firewall (ufw). A port published as "4100:8080" is therefore open to
# the whole internet even though ufw only allows 22, 80 and 443. Docker reserves
# the DOCKER-USER chain for exactly this kind of rule.
#
# Traffic from the server itself (nginx -> 127.0.0.1:4100) is not affected,
# because only packets arriving on the public network interface are dropped.
# Once a service is adopted under its compose file it is published on
# 127.0.0.1 only and this block becomes a second layer of safety.
set -euo pipefail

# host port -> what it is
PORTS=(4100 4101 4103)   # api-gateway, media-file-service, dispatch
COMMENT="karrygo-close-public-ports"
UNIT=/etc/systemd/system/karrygo-close-public-ports.service

[ "$(id -u)" = 0 ] || { echo "run as root (sudo)" >&2; exit 1; }

ext_if="$(ip route get 1.1.1.1 | awk '{ for (i = 1; i <= NF; i++) if ($i == "dev") { print $(i + 1); exit } }')"
[ -n "$ext_if" ] || { echo "could not find the public network interface" >&2; exit 1; }

# After Docker's address translation the packet's port is the CONTAINER port,
# so match on the ORIGINAL destination port that the visitor used.
rule_args() { # port
  echo "-i $ext_if -p tcp -m conntrack --ctorigdstport $1 --ctdir ORIGINAL -m comment --comment $COMMENT -j DROP"
}

tables() {
  echo iptables
  # IPv6 only if Docker manages an IPv6 DOCKER-USER chain on this server.
  ip6tables -S DOCKER-USER >/dev/null 2>&1 && echo ip6tables
  return 0
}

apply() {
  for t in $(tables); do
    $t -S DOCKER-USER >/dev/null 2>&1 || { echo "$t: no DOCKER-USER chain (is Docker running?)" >&2; exit 1; }
    for p in "${PORTS[@]}"; do
      # shellcheck disable=SC2046
      $t -C DOCKER-USER $(rule_args "$p") 2>/dev/null || $t -I DOCKER-USER $(rule_args "$p")
    done
  done
  echo "Blocked from the internet on $ext_if: ${PORTS[*]}"
}

undo() {
  for t in $(tables); do
    for p in "${PORTS[@]}"; do
      # shellcheck disable=SC2046
      while $t -C DOCKER-USER $(rule_args "$p") 2>/dev/null; do $t -D DOCKER-USER $(rule_args "$p"); done
    done
  done
  if [ -f "$UNIT" ]; then
    systemctl disable --now karrygo-close-public-ports.service >/dev/null 2>&1 || true
    rm -f "$UNIT"
    systemctl daemon-reload
  fi
  echo "Removed the block for: ${PORTS[*]}"
}

check() {
  for t in $(tables); do
    echo "$t DOCKER-USER rules for KarryGo:"
    $t -S DOCKER-USER | grep "$COMMENT" || echo "  (none)"
  done
  [ -f "$UNIT" ] && echo "Re-applied at boot by: $UNIT" || echo "Not re-applied at boot (no systemd unit)"
}

install_unit() {
  # Docker recreates its chains at boot; this re-adds the rules right after.
  install -m 0755 "$(readlink -f "$0")" /usr/local/sbin/karrygo-close-public-ports
  cat >"$UNIT" <<UNITFILE
[Unit]
Description=Block internet access to KarryGo container ports (DOCKER-USER)
After=docker.service
Requires=docker.service

[Service]
Type=oneshot
ExecStart=/usr/local/sbin/karrygo-close-public-ports --apply-only
RemainAfterExit=yes

[Install]
WantedBy=multi-user.target
UNITFILE
  systemctl daemon-reload
  systemctl enable karrygo-close-public-ports.service >/dev/null
  echo "Will be re-applied automatically after every reboot ($UNIT)"
}

case "${1:-}" in
  "") apply; install_unit ;;
  --apply-only) apply ;;
  --check) check ;;
  --undo) undo ;;
  *) echo "usage: $0 [--check|--undo]" >&2; exit 2 ;;
esac
