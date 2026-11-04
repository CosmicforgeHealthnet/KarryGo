# Staging Notification SMTP configuration through GitHub

Use the existing **Deploy platform (explicit approved release)** workflow,
`.github/workflows/deploy-platform.yml`, dispatched at `staging`. No new SSH
access, unprotected deployment workflow, Resend domain, API adapter, DNS write,
webhook change or corporate mailbox setup is involved.

## Prerequisites and credential provenance

The existing `scripts/release/gate.py` remains authoritative. It requires the
current exact staging SHA, all seven acceptance workflows with successful jobs,
an approved actual staging restore certification for Notification at that SHA
(less than 24 hours old), a maintain/admin initiator on the existing allowlist,
and a separate configured individual reviewer. Staging must prevent self-review
and admin bypass and allow only the exact staging branch.

Existing environment-only secrets remain mandatory: `RELEASE_AUDIT_TOKEN`,
`STAGING_DEPLOY_HOST`, `STAGING_DEPLOY_USER`, `STAGING_DEPLOY_SSH_KEY` and
`STAGING_DEPLOY_KNOWN_HOSTS`. The gate intentionally rejects inherited legacy
`VPS_*` and release credentials. Do not delete/move credentials or alter release
protection without the separately authorized release-security procedure.

For activation only, securely provision the **existing KarryGo Notifications**
sending-only key into the protected staging environment secret
`KARRYGO_NOTIFICATIONS_RESEND_SMTP_PASSWORD`. Confirm its name, sending access
and restriction to the existing `karryam.com` domain in the existing provider
account/vault. The key format alone cannot prove access scope. Never supply the
Centralized Admin key, generate a replacement when the existing key is available,
paste a key into a workflow input/chat, or print it. This workflow cannot retrieve
encrypted GitHub secret values for an operator. Inspection/rollback do not require
this SMTP secret. No repository/organization copy of this secret is allowed.

## Operations

Supply `release_sha=<current full staging SHA>`,
`services=notification-service`, `restore_run_id=<actual approved certification>`
and one of these operations. Do not fabricate restore evidence or substitute
a disposable test restore for the existing operator certification.

| Operation | Behavior |
| --- | --- |
| `deploy` (existing default) | Existing immutable-image release; no SMTP configuration mutation |
| `smtp-inspect` | Protected host/container inspection; outputs booleans for presence, staging, health, matching settings and password presence; no secret values or sends |
| `smtp-activate` | Authenticates to Resend over certificate-verified TLS on the host, backs up and patches Notification env, calls existing apply-env, checks actual loaded values and health; restores previous config on failure |
| `smtp-rollback` | Also supply `backup_run_id=<original activation run>`; restores protected backup using apply-env; refuses intervening env changes or a changed running image |

The SMTP job cannot select Partner, Dispatch, Gateway, Admin, another service,
`all`, another branch or production. It shares `staging-release-runtime`
concurrency with existing deployments and takes a nonblocking host directory
lock. Authorization is checked again immediately before SSH. SSH pins existing
known hosts and uses batch mode. Only schema-checked boolean facts and numeric
run IDs are printed; raw host output, errors and application logs are suppressed.
The SMTP password travels on encrypted stdin, never process argv or evidence.

## Exact environment delta

The only edited file is
`/opt/karrygo/notification-service/notification.service.env`:

```dotenv
NOTIFICATION_EMAIL_PROVIDER=smtp
NOTIFICATION_SMTP_HOST=smtp.resend.com
NOTIFICATION_SMTP_PORT=465
NOTIFICATION_SMTP_TLS_MODE=tls
NOTIFICATION_SMTP_TIMEOUT_SECONDS=15
NOTIFICATION_SMTP_USERNAME=resend
NOTIFICATION_SMTP_PASSWORD=<protected existing KarryGo sending key>
NOTIFICATION_EMAIL_FROM="Karryam <no-reply@karryam.com>"
```

Unrelated env bytes, ownership and mode 600 are preserved. Symlinks, duplicate
target assignments, non-staging envs/containers, unhealthy current containers,
insecure parent permissions and mismatched release/current images fail closed.
Backups are owner-only files under the mode-700 directory
`/opt/karrygo/notification-service/.smtp-backups/<activation-run-id>/`.
`before.env` and `manifest.json` stay on the host; neither is uploaded to Actions.
Keep them for rollback. `before-rollback.env` protects the state before a manual
rollback attempt. A failed automatic restore leaves the backup intact and the
job failed; use the approved recovery procedure, not a broad restart.

The existing `/opt/karrygo/platform/bin/apply-env.sh notification-service`
recreates only its application container. No images are built/pulled by the SMTP
operation, no migrations run, and Redis/database volumes are untouched. Deploy
the SMTP code fixes first through the normal image release when required.

## Partner Web and caller configuration

The repository register/Compose declares
`/opt/karrygo/partner-web-service/partner.env`; this is **not proof of its deployed
path or that the split service is running**. SMTP inspection inventories the
container and recognizes the registered Compose env path only when supported by
its actual Compose label/file. The existing release gate explicitly forbids
Partner deployment because the legacy Gateway must first stop partner billing
and webhook workers. This task does not remove that hold or recreate Partner.

After its separately approved split/cutover, use its approved configuration path
to apply these values with the same protected KarryGo sending key:

```dotenv
PARTNER_SMTP_HOST=smtp.resend.com
PARTNER_SMTP_PORT=465
PARTNER_SMTP_TLS_MODE=tls
PARTNER_SMTP_USERNAME=resend
PARTNER_SMTP_PASSWORD=<protected existing KarryGo sending key>
PARTNER_SMTP_FROM=no-reply@karryam.com
PARTNER_SMTP_FROM_NAME=Karryam
```

Customer, Vendor, Dispatch, Taxi and Hauling need their own existing Notification
URL/HMAC secret and corresponding `NOTIFICATION_SERVICE_SECRETS` entries. Missing
caller credentials cannot be repaired by installing an SMTP key. Inspect them
without printing values. Preserve all debug/test-login flags; acceptance must
use genuine email OTP delivery with debug bypass inactive. Never enable bypasses
to make a test pass. The Dispatch legacy cPanel adapter remains source-only:
current main selects the Notification client or an unavailable client.

## Acceptance and honest evidence

The host SMTP TLS/auth probe sends nothing. Container health and matching env
are prerequisite checks, not container egress or inbox proof. Existing SMTP
`sent` means SMTP handoff; provider ID `smtp` is not a Resend tracking ID.

For dedicated staging accounts, test Customer signup/login/resend/verification,
Vendor registration/login/step-up, Dispatch/Taxi/Hauling email auth, Partner signup
after its hold is cleared, and an existing non-OTP transaction. Record owning
service acceptance, persisted message/delivery identifiers, queue attempt/status,
provider evidence, inbox receipt/headers and completed OTP auth. Test wrong and
expired codes, resend invalidation and failure behavior; inspect logs using
redacted checks without publishing codes or recipients. No actual recipient
address or inbox access means these tests remain pending. Do not send customer
traffic or deliberately break shared live SMTP to test failures; use isolated
fake SMTP/provider fault tests.

Notification retries use a per-delivery hashed `Resend-Idempotency-Key` and stable
Date/MIME/Message-ID; distinct resend messages have distinct delivery IDs. Resend
supports this SMTP header ([official SMTP reference](https://resend.com/docs/send-with-smtp)).
Real provider deduplication remains an external acceptance test; retention limits,
changed binaries/templates during retries and ambiguous handoffs prevent a claim
of indefinite exactly-once delivery. Partner no longer deletes an accepted
challenge just because the server fails QUIT after accepting DATA.

## Production procedure (not authorized for execution)

First finish all staging acceptance above and clear the independent Partner hold.
Read-only inventory the actual production topology/env paths, health, image
digests, SMTP settings/key provenance and caller HMAC wiring using the approved
production inspection mechanism. Production was not inspected by this staging
workflow, which deliberately refuses production.

Obtain separate production release authorization and the existing production
approval/restore evidence; no automatic production path is added here. Reuse the
root domain, the same approved sending-only key and the exact SMTP values above.
Back up each actual production env with owner-only permissions, deploy approved
immutable images, change only SMTP settings and recreate only each approved app
with its existing environment-application mechanism. Verify health, loaded config,
TLS/auth and controlled inbox/OTP acceptance. Do not enable debug OTP, alter
corporate Mail, change DNS or touch the shared webhook. On failure, restore each
protected env backup and prior approved image through the existing rollback
procedure, verify health and retain evidence. Never roll an old full env over
intervening unrelated changes.
