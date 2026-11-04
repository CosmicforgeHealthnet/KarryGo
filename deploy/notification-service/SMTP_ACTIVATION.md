# KarryGo Notification SMTP — simple STAGING activation

**Current procedure.** An authorized VPS operator runs the existing protected
SMTP transaction directly. No new GitHub App, audit token, receipt token, hosted
worker, timer, webhook, database restore-certification workflow or new Resend key
is needed merely to **configure SMTP for Notification on staging**.

This is *not* a production release procedure. All production rollouts and other
KarryGo services keep their own authorization and release policies. Do not
touch Partner Web, Dispatch, Gateway, Admin Mail, shared Resend webhook or DNS.

## What was independently checked (4 November 2026)

- VPS staging Notification container `karrygo-notification-service` was
  running and Docker reported `healthy`; `APP_ENV=staging` was confirmed
  in the container and private env file.
- Protected sending-only Resend key exists at
  `/opt/karrygo/notification-service/.secrets/resend-smtp-key`.
  Keep it in place. Do not print, copy or rotate it for this activation.
- A read-only KarryGo checkout exists at `/opt/karrygo-smtp-source` on
  the VPS. Update that checkout to the reviewed staging commit before using
  the new command; a GitHub push alone does not reconfigure the container.
- GitHub CI passed for the previous staging commit. **Check CI again for the
  final merged change** before the operator-run activation.
- There is no evidence of successful live SMTP delivery/OTP acceptance yet.

## Step 1 — update only the reviewed source checkout (no running-service change)

The VPS has already confirmed read-only GitHub access under `deploy`.
Use the `staging` branch, not `main`.

```bash
sudo -u deploy -H git -C /opt/karrygo-smtp-source pull --ff-only origin staging
sudo -u deploy -H git -C /opt/karrygo-smtp-source rev-parse HEAD
```

Confirm this commit matches the reviewed, CI-passing staging head.
If the pull fails or the checkout has unexpected modifications, stop.

## Step 2 — inspect prerequisites (read-only)

```bash
sudo -u deploy -H python3 /opt/karrygo-smtp-source/scripts/release/smtp_local.py inspect
```

Proceed only if `ready_for_activation` is `true`, the environment file and
container both indicate staging, and the running container is healthy.
The inspect command reports booleans; it never prints secrets.

## Step 3 — activate SMTP on the existing service image

Only the authorized VPS operator, after reviewing the service/environment,
runs the following:

```bash
sudo -u deploy -H python3 /opt/karrygo-smtp-source/scripts/release/smtp_local.py activate --confirm STAGING_NOTIFICATION_ONLY
```

The command reuses `scripts/release/smtp_runtime.py`:

1. Refuses to run as root or a user other than `deploy`.
2. Checks staging in the private file **and** running container.
3. Verifies protected owner-only local key, Docker health and current release image.
4. Authenticates to `smtp.resend.com:465` with verified TLS **before** changes.
5. Creates a mode-700 protected, uniquely identified backup of the current env.
6. Updates only the Notification SMTP/provider/sender variables, preserving all
   unrelated config and the **same image**.
7. Recreates **only** the Notification app/worker via existing
   `/opt/karrygo/platform/bin/apply-env.sh notification-service`.
8. Verifies health, image identity, loaded SMTP settings and unrelated env;
   automatically restores the previous configuration if application fails.
9. Prints a numeric backup ID for a possible manual rollback, not any secret.

Target sender is `Karryam <no-reply@karryam.com>`. No other sender, Resend
domain, webhook or service is changed.

If the command fails or the container is unhealthy, **stop and inspect**.
A rollback can itself fail; never assume recovery without a healthy-container
check. Do not rerun blindly or remove protected backups.

## Step 4 — real staging acceptance

Using an approved staging test account and inbox:

- Confirm a real email arrives with `From: Karryam <no-reply@karryam.com>`.
- Test registration/login OTP, wrong code, expired code and resend behavior
  for the relevant caller flows; preserve current caller-specific debug flags
  until the test plan explicitly handles them.
- Confirm logs/queue behavior without exposing SMTP credentials or OTP values.
- A `202 QUEUED` response and SMTP handoff are **not** inbox delivery.
- Record operational acceptance and only then plan a separate production rollout.

## Manual rollback

Use the numeric **activation backup ID printed by Step 3**:

```bash
sudo -u deploy -H python3 /opt/karrygo-smtp-source/scripts/release/smtp_local.py rollback --backup-id <ACTIVATION_BACKUP_ID> --confirm STAGING_NOTIFICATION_ONLY
```

Rollback requires the same immutable container image and unchanged config hash,
preserves a before-rollback copy and checks that the previous container env and
healthy status have been restored. It never modifies application databases.

## Scope and previous machinery

The previous `smtp-activate` GitHub request/receipt worker is **not part of
this staging-only operator procedure**. Its code and other protected deployment
workflows are retained for compatibility but require no installation or setup
to use this path. Do not disable existing repository protections or delete
shared `VPS_*` credentials merely to activate SMTP.

The root-domain Resend key, Admin Mail configuration, Partner Web, DNS and the
shared webhook are not changed. Production is not changed.
