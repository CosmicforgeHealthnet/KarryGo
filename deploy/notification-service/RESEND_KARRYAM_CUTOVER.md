# Karryam root-domain Resend cutover (staging-first)

Status (28 October 2026): **Resend `karryam.com` sending/receiving verification and Hostinger root inbound MX have been reported complete by the owner.** This document does not repeat or change DNS or Resend setup. **KarryGo SMTP rollout, CosmicForge Admin Mail infrastructure, the shared webhook and live acceptance tests have not been completed.** Scope: staging-first deployment of KarryGo automated OTP/transactional senders on `no-reply@karryam.com` and separate CosmicForge Admin human-managed inboxes.

## Source-of-truth architecture

- **KarryGo notification-service stays the first-party delivery authority for OTP and platform notifications.** Customer, Vendor, Dispatch, Taxi and Hauling already use `shared/go/notifications` / the Notification service. Do **not** send login OTPs through CosmicForge Admin's inbox service; Admin availability must not gate authentication.
- `services/notification-service/cmd/main.go` selects `smtp` using `NOTIFICATION_EMAIL_PROVIDER=smtp`. The existing `SMTPEmailSender` accepts implicit TLS (`tls`) and From display names; no new Resend-specific Go adapter is necessary for an initial authenticated SMTP pilot.
- **Exception:** `services/partner-web-service/internal/features/partneraccount/smtp_otp_sender.go` independently sends partner signup OTPs. Configure its separate `PARTNER_SMTP_*` settings too. Do not assume the Notification-service settings cover it.
- **CosmicForge Admin Mail (different repository):** `CosmicforgeHealthnet/centralize-admin`, `staging`, `services/central-admin-api/src/modules/mail/`. Its Resend API/webhook/worker is responsible for `info@karryam.com`, `support@karryam.com`, `marketing@karryam.com`, `partnership@karryam.com`, staff addresses and incoming replies. One domain does not imply one application or one API key.
- The landing-page `web/karryam-landing/src/app/api/waitlist/resend/route.ts` is a 501 placeholder; do not treat it as an active sending flow.

## New sender and provider settings

**Two Resend keys already exist:** `KarryGo Notifications` (sending-only, restricted to `karryam.com`) and `CosmicForge Admin Mail` (CA API access). Use **the same KarryGo sending-only key** for Notification Service and Partner Web SMTP, separately stored as `NOTIFICATION_SMTP_PASSWORD` and `PARTNER_SMTP_PASSWORD`. Never give either service CA's full-access key. The owner has chosen to reuse the two respective keys in staging and eventual production; isolate application databases, workers, secrets access and test recipients by environment. Resend SMTP: host `smtp.resend.com`, port `465` (implicit TLS), username `resend`. Never create new keys, commit values, log them or paste them in chat.

Configure these values using the **approved GitHub Actions deployment and protected secret source** for the staging Notification Service (do not give a coding agent direct VPS/SSH access). The following are example values, not proof that the deployed environment is configured:

```dotenv
NOTIFICATION_EMAIL_PROVIDER=smtp
NOTIFICATION_SMTP_HOST=smtp.resend.com
NOTIFICATION_SMTP_PORT=465
NOTIFICATION_SMTP_TLS_MODE=tls
NOTIFICATION_SMTP_TIMEOUT_SECONDS=15
NOTIFICATION_SMTP_USERNAME=resend
NOTIFICATION_SMTP_PASSWORD=<secret-in-deployment-vault>
NOTIFICATION_EMAIL_FROM="Karryam <no-reply@karryam.com>"
```

In the staging Partner Web protected environment, if that service is deployed, independently configure its SMTP sender through the approved GitHub deployment flow:

```dotenv
PARTNER_SMTP_HOST=smtp.resend.com
PARTNER_SMTP_PORT=465
PARTNER_SMTP_TLS_MODE=tls
PARTNER_SMTP_USERNAME=resend
PARTNER_SMTP_PASSWORD=<secret-in-deployment-vault>
PARTNER_SMTP_FROM=no-reply@karryam.com
PARTNER_SMTP_FROM_NAME=Karryam
```

These are example environment entries, **not** credentials or a deployment action. Keep DB, Redis, HMAC service caller secrets, push, queue, debug-OTP settings and unrelated config unchanged. Test TLS connectivity from each relevant running container. If the host blocks 465, Resend also supports 587/STARTTLS; set `*_SMTP_TLS_MODE=starttls` explicitly, never `none`.

## Verified Resend domain and DNS — preserve, do not reconfigure

- **Owner-reported completed:** `karryam.com` is verified in the existing Resend account for sending and receiving (Ireland / `eu-west-1`); the Hostinger root MX `@` → `receive-euw1.forge.rmta.net` (priority `10`) is configured and verified. Sending DKIM/return-path records have also been verified. Recheck provider and DNS status **read-only** before activation; this documentation does not independently validate live DNS.
- **Do not** add another domain, delete `no-reply.karryam.com`, retire an existing verified domain, change the root MX, or replace a Resend webhook. Preserve HealthNet and `marketing.karryam.com` provider identities and existing traffic.
- **One shared Resend webhook:** before making changes, inventory the existing endpoint and subscribed event types in the Resend dashboard. Once the CA backend, database, storage/KMS, ClamAV and worker are ready, configure the **existing** webhook to target `https://stagingca.cosmicforge-healthnet.com/api/mail/v1/webhooks/resend` for approved staging tests. After a separately approved production release and cutover, update that *same* webhook to `https://api.cosmicforge-healthnet.com/api/mail/v1/webhooks/resend`. Do not create a second endpoint or switch it early.
- **Inbound traffic warning:** the verified inbound MX routes domain mail to Resend, but that does **not** make the four Admin mailboxes operational. Until the CA webhook, database and workers are enabled and tested, external incoming mail must not be treated as delivered to the departments.
- `no-reply@karryam.com` is an outbound-only SYSTEM sender reserved for KarryGo. It is not a staff inbox or catch-all. Keep HealthNet inbound receiving disabled on the shared provider account until cross-brand isolation is demonstrated.

All addresses `no-reply@karryam.com`, `support@karryam.com`, `info@karryam.com`, `marketing@karryam.com`, `partnership@karryam.com` are **local parts under one domain**, not separately verified domains.

## Acceptance checks (staging only; owner-controlled recipients)

- [ ] Reconfirm **read-only** that the owner's existing `karryam.com` sending/receiving verification and root MX records are still correct; do not change or recreate DNS.
- [ ] After staging-only protected secrets are installed, a controlled Notification-service email is accepted by Resend SMTP and received in an approved external test inbox; headers show **Karryam <no-reply@karryam.com>**, without old-domain identifiers.
- [ ] Customer email sign-in OTP, Vendor registration/sign-in/resend and step-up OTP, Dispatch email OTP/verification, Taxi and Hauling email OTP, and Partner Web signup OTP pass or explicitly document their current flags/dependencies. Never enable debug-OTP in production.
- [ ] Deliberately break SMTP authentication and TLS in an isolated test: delivery is **not** marked as successfully delivered; retry/dead-letter/error behaviour is observed without logging codes.
- [ ] Control free-tier quota exhaustion, recipient blocking, rate limiting and provider failures. Separate transactional OTP capacity from campaign volume (the Resend team quota is shared).
- [ ] CosmicForge Admin can create distinct `SHARED` inboxes for the four departments, assign members/sender grants, receive signed `email.received` events, fetch content and attachments, and reply from each identity.
- [ ] With the single Resend webhook and staging-only `MAIL_TEST_RECIPIENT_ALLOWLIST` and `MAIL_INBOUND_ACTIVE_ENVIRONMENT=staging`, KarryGo's unrelated events do not enter Admin delivery states and HealthNet/marketing data remains isolated. Verify provider environment tags with a real signed event rather than assuming they are echoed.
- [ ] Store results as test evidence; do not mark production email ready from a queued (HTTP 202) response alone.

## Known gaps; avoid accidental coupling

1. **SMTP acceptance is not Gmail inbox delivery.** KarryGo's existing SMTP adapter currently records the literal provider message ID `smtp`, not a unique Resend email ID. The current `SENT` state signifies the SMTP handoff, not delivery confirmation. Correlating individual Resend delivery/bounce webhooks back to a KarryGo notification requires separate design and testing (e.g. an API adapter returning provider IDs); do not pretend Admin's existing webhook supplies it automatically.
2. **Single Free-tier webhook.** CosmicForge Admin's existing Resend webhook may see email events from other services on the same account. Its processing must match only its own provider IDs/organizations and reject unsafe cross-brand assumptions. The optional Admin `mail.delivery_status@1` integration contract is an **event-reporting boundary**, not an instruction to route OTP sending through Admin.
3. **No Reply-To in the current Notification SMTP email template.** A user replying to a no-reply sender will not automatically reach Support. Reply-capable product communications should use a later approved Reply-To workflow (or be sent using an authorized corporate mailbox), not simply assume inbound replies arrive in `support@karryam.com`.
4. **Out-of-band Partner Web SMTP.** Centralize later only as a separately reviewed change; the first cutover can safely use the Partner Web's existing SMTP interface.

## Rollback / stop conditions

If TLS, sender identity, delivery, webhook classification, or staging API health checks fail, leave CA Mail flags OFF and stop the pilot. Restore only the approved application configuration; **do not delete, recreate or alter the verified Resend domains or root MX** as a rollback. Do not redirect the shared webhook or change production settings without separate cutover approval. Never treat an HTTP 202 or SMTP handoff as inbox delivery.

**Staging activation sequence:** merge and deploy reviewed application code to `staging`; run the separately approved, read-only GitHub Actions Mail inspection; complete staged infrastructure readiness, protected secrets and mailbox provisioning; run controlled end-to-end tests; only then point the existing webhook to the staging endpoint. Keep CA Mail and broadcast flags OFF until explicitly authorized. Nothing in this documentation PR deploys containers or activates email.

Official SMTP reference: https://resend.com/docs/send-with-smtp

Related: `deploy/notification-service/SMTP_STAGING.md`, `services/notification-service/README.md`, CosmicForge Admin's `docs/cosmicforge-mail-staging-e2e-acceptance.md`.
