#!/usr/bin/env bash
# Log in to the VPS over SSH.
#
# Usage:
#   bash scripts/ssh-vps.sh              # interactive login (prompts for password)
#   bash scripts/ssh-vps.sh setup-key    # one-time: install your SSH key, then no password needed
#   bash scripts/ssh-vps.sh 'uptime'     # run a single remote command
#
# Overrides (env): VPS_HOST, VPS_USER, VPS_PORT, VPS_KEY
# Optional non-interactive password: export VPS_PASSWORD=... (requires sshpass).
# Never hardcode the password in this file.
set -euo pipefail

VPS_HOST="${VPS_HOST:-173.249.30.55}"
VPS_USER="${VPS_USER:-root}"
VPS_PORT="${VPS_PORT:-22}"
VPS_KEY="${VPS_KEY:-$HOME/.ssh/id_ed25519}"

TARGET="${VPS_USER}@${VPS_HOST}"
SSH_OPTS=(-p "$VPS_PORT" -o ServerAliveInterval=30 -o ServerAliveCountMax=3)

if [[ "${1:-}" == "setup-key" ]]; then
  if [[ ! -f "$VPS_KEY" ]]; then
    echo "No key at $VPS_KEY; generating one..."
    ssh-keygen -t ed25519 -f "$VPS_KEY" -N "" -C "$(whoami)@$(hostname)"
  fi
  echo "Copying ${VPS_KEY}.pub to $TARGET (you'll be asked for the password once)..."
  ssh-copy-id -i "${VPS_KEY}.pub" -p "$VPS_PORT" "$TARGET"
  echo "Done. Future logins use the key: bash scripts/ssh-vps.sh"
  exit 0
fi

# Use the key if it exists (no password prompt once setup-key has run).
if [[ -f "$VPS_KEY" ]]; then
  SSH_OPTS+=(-i "$VPS_KEY")
fi

if [[ -n "${VPS_PASSWORD:-}" ]]; then
  if ! command -v sshpass >/dev/null 2>&1; then
    echo "VPS_PASSWORD is set but sshpass isn't installed." >&2
    echo "Install it (brew install hudochenkov/sshpass/sshpass) or unset VPS_PASSWORD." >&2
    exit 1
  fi
  # -e reads the password from $SSHPASS, so it never shows up in `ps`.
  export SSHPASS="$VPS_PASSWORD"
  exec sshpass -e ssh "${SSH_OPTS[@]}" "$TARGET" "$@"
fi

exec ssh "${SSH_OPTS[@]}" "$TARGET" "$@"
