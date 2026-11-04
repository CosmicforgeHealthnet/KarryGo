#!/usr/bin/env bash
# One-time AUTHORIZED VPS administration only. Never run from GitHub Actions.
# Installs code/timer, not credentials; does not change/restart Notification.
set -euo pipefail
[ "$(id -u)" = 0 ] || { echo 'Authorized root administration required' >&2; exit 1; }
[ "$#" = 3 ] || { echo 'usage: install-smtp-worker.sh <reviewed-checkout> <approved-staging-sha> <receipt-identity>' >&2; exit 1; }
checkout="$(realpath "$1")"
revision="$2"
identity="$3"
[[ "$revision" =~ ^[0-9a-f]{40}$ ]] || exit 1
[[ "$identity" =~ ^[a-zA-Z0-9_-]+(\[bot\])?$ ]] || exit 1
[ "$identity" != 'github-actions[bot]' ] || exit 1
[ "$(git -C "$checkout" rev-parse HEAD)" = "$revision" ] || exit 1
[ "$(git -C "$checkout" branch --show-current)" = staging ] || exit 1
target=/usr/local/lib/karrygo-smtp
[ ! -e "$target" ] && [ ! -L "$target" ] || { echo 'Existing executor directory: inspect and review an upgrade; refusing overwrite' >&2; exit 1; }
[ "$(realpath /usr/local/lib)" = /usr/local/lib ] || exit 1
for unit in karrygo-smtp-worker.service karrygo-smtp-worker.timer; do
  [ ! -e "/etc/systemd/system/$unit" ] && [ ! -L "/etc/systemd/system/$unit" ] || {
    echo 'Existing worker unit: inspect before installation; refusing overwrite' >&2; exit 1;
  }
done
install -d -o root -g root -m 755 "$target" "$target/deploy" "$target/deploy/platform"
for file in gate.py smtp_activate.py smtp_runtime.py smtp_vps.py; do
  git -C "$checkout" show "$revision:scripts/release/$file" >"$target/$file"
  chmod 644 "$target/$file"
done
git -C "$checkout" show "$revision:deploy/platform/services.tsv" >"$target/deploy/platform/services.tsv"
chmod 644 "$target/deploy/platform/services.tsv"
printf '%s\n' "$revision" >"$target/approved-revision"
printf '%s\n' "$identity" >"$target/receipt-identity"
printf '%s\n' 'import sys' 'sys.path.insert(0, "/usr/local/lib/karrygo-smtp")' 'from smtp_vps import main' \
  'try:' '    main()' 'except Exception:' '    print("Notification SMTP worker blocked; inspect protected prerequisites")' '    raise SystemExit(1)' >"$target/entry.py"
chmod 644 "$target/approved-revision" "$target/receipt-identity" "$target/entry.py"
for unit in karrygo-smtp-worker.service karrygo-smtp-worker.timer; do
  git -C "$checkout" show "$revision:deploy/platform/systemd/$unit" >"/etc/systemd/system/$unit"
  chmod 644 "/etc/systemd/system/$unit"
done
systemctl daemon-reload
echo 'Worker code installed. No SMTP settings changed. Provision/verify VPS-only release credentials and approvals before enabling its timer.'
