# VPS-only Notification SMTP cutover

This procedure supersedes the previous GitHub-secret/SSH SMTP activation path.
The Resend key already exists on the VPS at
`/opt/karrygo/notification-service/.secrets/resend-smtp-key`; do not recreate it,
copy it to GitHub, place it in a request, or print it. Its SMTP authentication
and the current healthy legacy service are owner-confirmed facts until an
authorized live inspection independently confirms them.

## Trust and data flow

1. The existing manual staging workflow publishes `smtp-request.json` using
   `actions/upload-artifact`. It contains a SHA, run ID, operation, nonce and
   restore/backup run identifiers, never application credentials or executable
   code. Only the Notification service is allowed.
2. A fixed, root-owned VPS systemd worker polls GitHub using an outbound HTTPS
   connection. It has no listening port and is not a GitHub runner. It never
   downloads/executes scripts from a request or updates its installed code.
3. The worker verifies artifact digest/provenance, current staging SHA, original
   dispatch actor, active first-attempt run, exact workflow path, actual reviewer
   protection and independent approval, initiator permissions/allowlist, all
   seven existing acceptance workflows, and the existing actual-restore proof.
   `gate.py` remains authoritative. Partner/Gateway/Dispatch/Admin holds remain.
4. Only after reauthorization does the worker read the VPS key and patch SMTP.
   It checks `APP_ENV=staging` in both the file and container. It uses protected
   backups and existing `apply-env.sh notification-service`; only the existing
   application container is recreated, on the recorded existing image.
5. A separate least-privilege VPS credential publishes a GitHub deployment
   **receipt** (`task=karrygo-smtp`, `production_environment=false`). The Actions
   job has only read permissions and waits for the configured independent
   receipt identity, exact run/nonce/request digest and a success status. A
   queued request or timeout is never reported as completed activation.

The SMTP/certification job receives no SSH keys, SMTP credentials or audit token.
Its built-in `GITHUB_TOKEN` reads GitHub request/receipt metadata only. Existing
image-release paths for other services are outside this cutover and retain their
original authorization requirements; no legacy credential is silently removed.

## One-time authorized VPS administration

No restricted executor or audit/restore evidence path has been confirmed live.
Repository `server-setup.sh` provisions general deploy SSH; that is not the
outgoing-only executor. First inspect existing automation using the approved VPS
administration channel. Do not grant a coding agent or Actions runner shell
access to bootstrap around the release gate.

Read-only preflight (run on the VPS using authorized administration):

```bash
python3 /path/to/reviewed-checkout/deploy/platform/bin/audit-smtp-prerequisites.py
systemctl list-unit-files --no-pager | rg 'karrygo.*(smtp|release|deploy)'
```

The preflight emits paths, ownership/permission booleans, staging/health booleans
and candidate audit/restore filenames. It never prints env values, keys or backup
contents. Investigate candidates privately; proposed paths below are not claims
that credentials or restore evidence already exist.

If no suitable executor exists, install the reviewed source through authorized
VPS administration. This installer refuses to overwrite an existing executor;
an upgrade requires explicit administrative review. It does not start the timer,
change SMTP, restart Notification, or provision any credential:

```bash
sudo bash /path/to/reviewed-checkout/deploy/platform/bin/install-smtp-worker.sh \
  /path/to/reviewed-checkout <approved-current-staging-sha> <vps-receipt-identity>
```

Before writing anything, the installer independently requires a healthy container
and `APP_ENV=staging` in both the file and running container. It installs the
fixed worker into `/usr/local/lib/karrygo-smtp`, root-owned and
not writable by `deploy`, pins `approved-revision`, and installs only the new
worker's systemd units. The existing `deploy` Docker access is used locally;
GitHub gets no Docker/socket or filesystem access. It does not install a public
endpoint, self-hosted Actions runner or new SSH key. A later source SHA requires
a separately reviewed administrative update to the root-owned code/pin.

Privately locate/reuse existing authorized credentials where available, or
provision the missing **release** credentials through the authorized VPS vault
procedure. No values belong in chat, GitHub Secrets, artifacts or command argv:

| VPS-only path | Required scope |
| --- | --- |
| `/opt/karrygo/notification-service/.secrets/resend-smtp-key` | Existing authenticated Resend sending key; preserve it |
| `/opt/karrygo/platform/.secrets/release-audit-token` | Read-only repository metadata, Actions/artifacts, environments/approvals and secret **names** required by the existing gate; no source writes or approval authority |
| `/opt/karrygo/platform/.secrets/release-receipt-token` | Canonical repository Deployments write/Metadata read only; no Actions, source, secrets or environment write permission |

Credential files must be regular, non-symlink files owned by `deploy`, mode 600,
inside protected directories (700 or appropriately protected 750). Keep any
actual database backups/restore data on the VPS. The receipt writer must use a
dedicated identity distinct from the initiator and `github-actions[bot]`; an
appropriately scoped existing service identity or GitHub App installation token
may be used. Token renewal remains a protected VPS administration responsibility.
Do not repurpose a developer's broad CLI token or broaden the read-only audit
credential merely to publish receipts.

Set the non-secret staging environment variable `STAGING_SMTP_RECEIPT_LOGIN` to
the exact identity installed in the worker's root-owned `receipt-identity` file.
In GitHub: **KarryGo → Settings → Environments → staging → Environment variables**.
No application-secret setting is needed in GitHub. The existing gate also
rejects inherited legacy `VPS_*` release credentials; resolve their ownership
and retirement through authorized release-security administration, preserving
other consumers. Do not just delete shared credentials to pass a check.

After credential metadata, root ownership, installed code and policy are reviewed,
enable only the worker:

```bash
sudo systemctl enable --now karrygo-smtp-worker.timer
```

This starts no cutover by itself. Requests still require every release gate.
The worker writes mode-600 run receipts under
`/opt/karrygo/notification-service/.smtp-requests` and will never replay a claimed
run, including after a crash. Inspect/recover uncertain operations through
authorized administration and dispatch a fresh approved run; never delete a
claim to force a retry. Disable this timer to stop consuming future requests.

## Independent approval, CI and actual restore certification

In **Settings → Environments → staging**, configure explicit individual required
reviewers, prevent self-review, disallow admin bypass and retain the exact
`staging` branch policy. The initiator must remain on the existing
`STAGING_RELEASE_INITIATORS` allowlist with maintain/admin permission. This agent
cannot nominate an independent reviewer or approve its own deployment.

All seven workflows named in `scripts/release/gate.py:ACCEPTANCE` must finish
successfully, with their required jobs successful, for the exact current SHA.
The original staging restore certification remains mandatory and valid for only
24 hours. SMTP authentication is not restore evidence.

For real Notification backup/restore proof:

- Locate the actual Notification database, an actual staging backup, its
  manifest and operator-owned validation SELECTs. Known conventions are
  `/opt/karrygo/backups/<timestamp>/` and
  `/etc/karrygo/restore-validation/<actual-db-container>.json`; these are not proof
  that the files exist. The read-only preflight locates additional candidates.
- Use the existing scoped backup/restore procedure. Do not run unscoped
  `backup.sh` with no arguments for this task: it backs up unrelated services
  and performs retention cleanup. If a fresh backup is necessary, authorized
  administration must take only the actual Notification source backup and
  record its validation with the existing `scripts/release/restore.py record`.
- Verify that real backup with `scripts/release/restore.py verify --archive ...
  --manifest ... --output ...`. It restores into a fresh network-isolated
  disposable container and validates hashes, database major version and named
  read-only assertions. Never restore clean SQL into a live database.
- Build the existing schema-1 release proof from the actual resulting evidence:
  exact current SHA, environment `staging`, actual completion time/digests,
  matching database versions, actual protected backup location and validation
  summary under `services.notification-service`. An operator must truthfully
  attest the real restore and live runtime/consumer compatibility. Do not invent
  hashes, successful commands, timestamps or attestations. Do not upload backup
  data or settings; only the reviewed non-secret proof is a workflow input.

Dispatch the existing `certify-staging-restore.yml` with that exact SHA, service
`notification-service` and the genuine proof. Its Notification-only job uses the
same outgoing VPS request/receipt path; after independent authorization it
uploads the existing `staging-restore-evidence` artifact. If this workflow is not
registered in GitHub (404), an authorized repository administrator must register
it through the approved default-branch workflow process. This task does not
modify `main` or bypass registration/approval policy.

## Cutover and rollback

Dispatch existing `deploy-platform.yml` on **staging** with:

- `release_sha`: exact approved current staging SHA;
- `services`: `notification-service`;
- `restore_run_id`: actual successful approved certification for that SHA;
- `operation`: `smtp-activate` (`smtp-inspect` and `smtp-rollback` are also supported);
- for manual rollback only, `backup_run_id`: original activation run ID.

The worker authenticates to `smtp.resend.com:465` using TLS and the existing
local key. It preserves mode-600 `before.env` and `manifest.json` in the mode-700
`.smtp-backups/<activation-run-id>` directory, then atomically changes only:

```dotenv
NOTIFICATION_EMAIL_PROVIDER=smtp
NOTIFICATION_SMTP_HOST=smtp.resend.com
NOTIFICATION_SMTP_PORT=465
NOTIFICATION_SMTP_TLS_MODE=tls
NOTIFICATION_SMTP_TIMEOUT_SECONDS=15
NOTIFICATION_SMTP_USERNAME=resend
NOTIFICATION_EMAIL_FROM="Karryam <no-reply@karryam.com>"
```

`NOTIFICATION_SMTP_PASSWORD` is read locally from the existing key file. Ownership
and unrelated env bytes are preserved; after `apply-env.sh` the actual image,
health, SMTP fields and every unrelated loaded setting are checked. TLS/auth is
checked again after recreation. A configuration, health or SMTP failure restores
the original environment and recreates/verifies the previous configuration.
Keep the backup through acceptance. Manual rollback refuses intervening env
changes or a different image. Failed/uncertain restore requires protected local
inspection; no false success receipt is published. A GitHub receipt outage does
not undo an otherwise healthy service; retain the local receipt and inspect it
through authorized administration rather than guessing activation state.

## Real acceptance and boundaries

After the VPS success receipt, use a dedicated approved staging test account and
the existing owner test inbox. Verify actual inbox From `no-reply@karryam.com`,
receipt headers, valid OTP completion, invalid/expired-code rejection and resend
behavior. Do not use a debug code or a placeholder exposed by an inbox tool.
Legacy `noreply@cosmicforgetech.com` receipt is a failed sender check. HTTP 201/202,
a queued request, SMTP authentication and even a success receipt alone do not
prove inbox delivery or completed authentication.

Notification's `sent` state means SMTP handoff, not inbox confirmation. Existing
retry idempotency code is retained; real provider behavior still needs acceptance.
Partner Web remains independently restricted and is neither inspected nor changed
by this worker. No DNS, Resend domain/webhook, Centralized Admin, corporate Mail,
production or unrelated application service is changed by this procedure.
