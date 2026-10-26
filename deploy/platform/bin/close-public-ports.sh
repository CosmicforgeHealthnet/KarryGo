#!/usr/bin/env bash
# Blocks internet access to KarryGo container ports that are published on all
# addresses, without restarting anything.
#
#   close-public-ports.sh              DRY RUN (default): print the rules, change nothing
#   close-public-ports.sh --apply      block them now and keep them blocked after reboots (root)
#   close-public-ports.sh --check      show whether the block is in place (root)
#   close-public-ports.sh --undo       remove the block (root)
#   close-public-ports.sh --list-ports print the protected host ports, one per line
#
# Why this is needed: Docker writes its own firewall rules, which run BEFORE
# Ubuntu's firewall (ufw). A port published as "4100:8080" is therefore open to
# the whole internet even though ufw only allows 22, 80 and 443. Docker reserves
# the DOCKER-USER chain for exactly this kind of rule.
#
# Traffic from the server itself (nginx -> 127.0.0.1:4100) is not affected,
# because only packets arriving on the public network interface are dropped.
# Outbound traffic is not affected either: replies to connections the server or
# a container opened are conntrack direction REPLY, and only ORIGINAL-direction
# packets are matched. SSH (22), HTTP (80) and HTTPS (443) are never in the list
# (the script refuses to run if they ever are).
# Once a service is adopted under its compose file it is published on
# 127.0.0.1 only and this block becomes a second layer of safety. Host-native
# listeners (not Docker) are governed by ufw's INPUT policy, not by this chain.
#
# Idempotent: every rule is checked (-C) before it is inserted, and --undo
# removes every copy. Section 29 candidate: authoring this file applies nothing.
set -euo pipefail

# Host ports (or first:last ranges) that must never be reachable from the
# internet. Section 20/29 additions: 8110 (private admin bridge listener), the
# Go service ports 8101-8116, the per-service PostgreSQL ports 5432-5451 and
# Redis ports 6379-6391 used by the local/staging layout, and 8080 (gateway's
# container port, should it ever be published directly).
PORTS=(4100 4101 4103 8080 8110 8101:8116 5432:5451 6379:6391)
NEVER_BLOCK=(22 80 443)
COMMENT="karrygo-close-public-ports"
UNIT=/etc/systemd/system/karrygo-close-public-ports.service

guard_ports() {
  local spec low high never
  for spec in "${PORTS[@]}"; do
    low="${spec%%:*}"; high="${spec##*:}"
    [[ "$low" =~ ^[0-9]+$ && "$high" =~ ^[0-9]+$ && "$low" -le "$high" ]] || { echo "invalid port spec $spec" >&2; exit 2; }
    for never in "${NEVER_BLOCK[@]}"; do
      if [ "$never" -ge "$low" ] && [ "$never" -le "$high" ]; then
        echo "refusing: $spec would block management/web port $never" >&2; exit 2
      fi
    done
  done
}
guard_ports

mode="${1:-}"
case "$mode" in
  ""|--dry-run|--apply|--apply-only|--check|--undo|--list-ports) ;;
  *) echo "usage: $0 [--dry-run|--apply|--check|--undo|--list-ports]" >&2; exit 2 ;;
esac

if [ "$mode" = --list-ports ]; then
  printf '%s\n' "${PORTS[@]}"
  exit 0
fi

detect_ext_if() {
  command -v ip >/dev/null 2>&1 || return 0
  ip route get 1.1.1.1 2>/dev/null | awk '{ for (i = 1; i <= NF; i++) if ($i == "dev") { print $(i + 1); exit } }'
}

if [ "$mode" = "" ] || [ "$mode" = --dry-run ]; then
  ext_if="$(detect_ext_if)"
  ext_if="${ext_if:-<public-interface>}"
  echo "DRY RUN: nothing is changed. Re-run with --apply (as root) to insert these rules."
  for p in "${PORTS[@]}"; do
    echo "iptables -I DOCKER-USER -i $ext_if -p tcp -m conntrack --ctorigdstport $p --ctdir ORIGINAL -m comment --comment $COMMENT -j DROP"
  done
  echo "(the same rules are mirrored to ip6tables when Docker manages an IPv6 DOCKER-USER chain)"
  echo "never blocked: ${NEVER_BLOCK[*]} and all outbound traffic"
  exit 0
fi

[ "$(id -u)" = 0 ] || { echo "run as root (sudo)" >&2; exit 1; }

ext_if="$(detect_ext_if)"
[ -n "$ext_if" ] || { echo "could not find the public network interface" >&2; exit 1; }

# After Docker's address translation the packet's port is the CONTAINER port,
# so match on the ORIGINAL destination port that the visitor used. A spec may
# be a single port or a first:last range (conntrack accepts both).
rule_args() { # port or first:last
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

case "$mode" in
  --apply) apply; install_unit ;;
  --apply-only) apply ;;
  --check) check ;;
  --undo) undo ;;
esac
