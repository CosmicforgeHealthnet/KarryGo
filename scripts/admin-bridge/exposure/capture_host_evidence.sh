#!/usr/bin/env bash
# Section 29 host exposure evidence capture (READ-ONLY).
#
# NOT EXECUTED BY THE AUTHORING CHANGE. This script was written and syntax-
# checked only; it has never been run on real infrastructure from the
# repository. An operator runs it ON the staging host (as root, so process
# names and firewall tables are visible):
#
#   sudo bash capture_host_evidence.sh [--out-dir /var/tmp/karrygo-evidence]
#
# Every command it runs only READS state: ss, docker ps (names and published
# ports only -- never environment, labels or inspect output), nft list,
# iptables -S, ufw status. It changes nothing, restarts nothing and prints no
# secrets. Output goes to a new timestamped directory with a SHA-256 manifest.
set -euo pipefail

base="/var/tmp/karrygo-exposure-evidence"
while [ $# -gt 0 ]; do
  case "$1" in
    --out-dir) base="${2:-}"; shift ;;
    -h|--help) sed -n '2,16p' "$0"; exit 0 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
  shift
done
[ -n "$base" ] || { echo "--out-dir must not be empty" >&2; exit 2; }

stamp="$(date -u +%Y%m%dT%H%M%SZ)"
dir="$base/$stamp"
umask 077
mkdir -p "$dir"

capture() { # file command...
  local file="$1"; shift
  if command -v "$1" >/dev/null 2>&1; then
    { echo "# $*"; "$@" 2>&1 || echo "# exit status $?"; } >"$dir/$file"
  else
    echo "# $1 not installed on this host" >"$dir/$file"
  fi
}

capture listeners.txt ss -lntup
capture docker-published-ports.txt docker ps --format '{{.Names}}\t{{.Image}}\t{{.Ports}}'
capture nft-ruleset.txt nft list ruleset
capture iptables-docker-user.txt iptables -S DOCKER-USER
capture ip6tables-docker-user.txt ip6tables -S DOCKER-USER
capture ufw-status.txt ufw status verbose
capture interfaces.txt ip -brief address
{
  echo "captured_at=$stamp"
  echo "hostname=$(hostname 2>/dev/null || echo unknown)"
  echo "uid=$(id -u)"
} >"$dir/context.txt"

# Quick reading aid: listeners on a wildcard address for a protected port.
{
  echo "# wildcard listeners on protected ports (should be empty; see README-admin-exposure.md)"
  grep -E '(0\.0\.0\.0|\*|\[::\]):(4100|4101|4103|8080|8110|81(0[1-9]|1[0-6])|5432|6379)\b' "$dir/listeners.txt" || true
} >"$dir/summary.txt"

( cd "$dir" && sha256sum ./*.txt >MANIFEST.sha256 )
echo "evidence written to $dir"
