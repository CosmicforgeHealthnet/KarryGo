#!/usr/bin/env bash
# One-time server preparation for KarryGo deployments. Run as root. Safe to run
# again: every step checks first and skips what is already done.
#
#   server-setup.sh "<deploy public key>"
#   e.g. server-setup.sh "ssh-ed25519 AAAAC3Nza... github-actions-deploy"
#
# It:
#   1. installs the modern "docker compose" plugin (the GitHub workflows use it)
#   2. adds a 4 GB swap file if the server has none (emergency memory)
#   3. installs fail2ban (blocks repeated password guessing) and automatic
#      security updates
#   4. creates the "deploy" user that GitHub Actions logs in as, and installs
#      its public key
#   5. gives "deploy" ownership of /opt/karrygo so it can read the private
#      settings files (they stay mode 600: owner only)
#
# It deliberately does NOT touch Docker's own configuration (daemon.json):
# changing it restarts Docker, which would restart every container on this
# shared server, including the other products. Log size limits are set per
# service in deploy/platform/compose/common.yml instead.
set -euo pipefail

[ "$(id -u)" = 0 ] || { echo "run as root (sudo)" >&2; exit 1; }
[ $# -eq 1 ] || { echo "usage: $0 \"<deploy public key>\"" >&2; exit 2; }
DEPLOY_PUBKEY="$1"
case "$DEPLOY_PUBKEY" in ssh-ed25519\ *|ssh-rsa\ *|ecdsa-*) ;; *) echo "that does not look like an SSH public key" >&2; exit 2 ;; esac

step() { printf '\n== %s\n' "$*"; }

step "1. docker compose plugin"
if docker compose version >/dev/null 2>&1; then
  echo "already installed: $(docker compose version)"
else
  apt-get update -qq
  # Ubuntu's own package, matching the docker.io engine already installed here.
  DEBIAN_FRONTEND=noninteractive apt-get install -y -qq docker-compose-v2
  docker compose version
fi

step "2. swap"
if [ "$(swapon --show | wc -l)" -gt 0 ]; then
  echo "swap already present:"; swapon --show
else
  fallocate -l 4G /swapfile
  chmod 600 /swapfile
  mkswap /swapfile >/dev/null
  swapon /swapfile
  grep -q '^/swapfile ' /etc/fstab || echo '/swapfile none swap sw 0 0' >>/etc/fstab
  # Prefer real memory; only use swap when memory is genuinely short.
  echo 'vm.swappiness=10' >/etc/sysctl.d/99-karrygo-swap.conf
  sysctl -q -p /etc/sysctl.d/99-karrygo-swap.conf
  echo "added 4 GB swap"; swapon --show
fi

step "3. fail2ban and automatic security updates"
DEBIAN_FRONTEND=noninteractive apt-get install -y -qq fail2ban unattended-upgrades >/dev/null
systemctl enable --now fail2ban >/dev/null
echo "fail2ban: $(systemctl is-active fail2ban); unattended-upgrades installed"

step "4. deploy user"
if id deploy >/dev/null 2>&1; then
  echo "user 'deploy' already exists"
else
  useradd -m -s /bin/bash deploy
  echo "created user 'deploy'"
fi
# Membership of the docker group lets "deploy" manage containers. Be aware this
# is close to root power on the server; that is why the key is only stored in
# GitHub's encrypted secrets and the user has no password.
usermod -aG docker deploy
passwd -l deploy >/dev/null 2>&1 || true
install -d -m 700 -o deploy -g deploy /home/deploy/.ssh
touch /home/deploy/.ssh/authorized_keys
grep -qxF "$DEPLOY_PUBKEY" /home/deploy/.ssh/authorized_keys || echo "$DEPLOY_PUBKEY" >>/home/deploy/.ssh/authorized_keys
chown deploy:deploy /home/deploy/.ssh/authorized_keys
chmod 600 /home/deploy/.ssh/authorized_keys
echo "public key installed for 'deploy'"

step "5. /opt/karrygo ownership"
install -d -m 750 /opt/karrygo /opt/karrygo/platform /opt/karrygo/backups
chown -R deploy:deploy /opt/karrygo
echo "/opt/karrygo now belongs to 'deploy' (file modes unchanged)"

step "done"
echo "Next: add the GitHub secrets VPS_HOST, VPS_USER=deploy and VPS_SSH_KEY (see the guide, section J)."
