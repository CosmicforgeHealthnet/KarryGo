# Karryam root-domain Resend cutover (staging-first)

Status: **implementation handoff; NOT executed**. This document adds no provider key, DNS change, deployment, or mail traffic. Scope: migrate KarryGo automated OTP/transactional senders to `no-reply@karryam.com` using **one** verified Resend domain, while CosmicForge Admin independently owns human-managed mailboxes.

## Source-of-truth architecture

- **KarryGo notification-service stays the first-party delivery authority for OTP and platform notifications.** Customer, Vendor, Dispatch, Taxi and Hauling already use `shared/go/notifications` / the Notification service. Do **not** send login OTPs through CosmicForge Admin's inbox service; Admin availability must not gate authentication.
- `services/notification-service/cmd/main.go` selects `smtp` using `NOTIFICATION_EMAIL_PROVIDER=smtp`. The existing `SMTPEmailSender` accepts implicit TLS (`tls`) and From display names; no new Resend-specific Go adapter is necessary for an initial authenticated SMTP pilot.
- **Exception:** `services/partner-web-service/internal/features/partneraccount/smtp_otp_sender.go` independently sends partner signup OTPs. Configure its separate `PARTNER_SMTP_*` settings too. Do not assume the Notification-service settings cover it.
- **CosmicForge Admin Mail (different repository):** `CosmicforgeHealthnet/centralize-admin`, `staging`, `services/central-admin-api/src/modules/mail/`. Its Resend API/webhook/worker is responsible for `info@karryam.com`, `support@karryam.com`, `marketing@karryam.com`, `partnership@karryam.com`, staff addresses and incoming replies. One domain does not imply one application or one API key.
- The landing-page `web/karryam-landing/src/app/api/waitlist/resend/route.ts` is a 501 placeholder; do not treat it as an active sending flow.

## New sender and provider settings

Use a dedicated, least-privileged **Resend sending API key for KarryGo Notification** (stored only in the Notification service's protected deployment secret source). Resend's SMTP credentials are host `smtp.resend.com`, port `465` (implicit TLS), username `resend`, password = that API key. A separate sending key may be used by Partner Web. Never commit, log, or paste keys into reports.

In `/opt/karrygo/notification-service/notification.service.env` on the appropriate host, change **only** the relevant values:

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

In the existing Partner Web protected environment, configure its own SMTP sender (only if the service is deployed):

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

## Free-plan verified-domain slot and DNS

1. Record the current Resend domain inventory, sending identities, activity, and DNS records. Identify the exact previous automated From addresses and any other consumers of `no-reply.karryam.com`.
2. Prepare the KarryGo/Partner Web configuration and a test recipient **without activating a new From address yet**. Ensure an operator can quickly restore known-safe operation or keep the flows disabled in staging.
3. Because the Free plan is full, retire `no-reply.karryam.com` in Resend **only after owner approval**, accepting that mail from its old domain will stop until `karryam.com` is verified. No real users exist yet, but this is still an outage boundary for testing or internal recipients.
4. Add **`karryam.com` as a single domain** in Resend, with Sending and Receiving capability selected as required. Put **Resend-provided exact SPF/return-path, DKIM, and inbound MX records** in Hostinger, validate DMARC, and wait for confirmed sending verification. Do not confuse the sending return-path MX with the root inbound MX.
5. Hold off on the **root MX receiving cutover** until CosmicForge Admin's signed webhook, database migrations, scanner, encrypted storage, separate worker and mailbox routing have passed live staging tests. A verified sending domain does not mean inbound Admin mail is live.
6. `no-reply@karryam.com` is a **SYSTEM/outbound-only identity**. It should not be treated as a staff inbox. Plan an explicit unknown-recipient/reply policy for inbound messages addressed to it (do not silently create a catch-all mailbox).

All addresses `no-reply@karryam.com`, `support@karryam.com`, `info@karryam.com`, `marketing@karryam.com`, `partnership@karryam.com` are **local parts under one domain**, not separately verified domains.

## Acceptance checks (staging only; owner-controlled recipients)

- [ ] Resend shows `karryam.com` sending verified; DNS checks and root MX plan are recorded separately.
- [ ] A controlled Notification-service email is accepted by Resend SMTP and received in an external test inbox; headers show **Karryam <no-reply@karryam.com>**, without old-domain identifiers.
- [ ] Customer email sign-in OTP, Vendor registration/sign-in/resend and step-up OTP, Dispatch email OTP/verification, Taxi and Hauling email OTP, and Partner Web signup OTP pass or explicitly document their current flags/dependencies. Never enable debug-OTP in production.
- [ ] Deliberately break SMTP authentication and TLS in an isolated test: delivery is **not** marked as successfully delivered; retry/dead-letter/error behaviour is observed without logging codes.
- [ ] Control free-tier quota exhaustion, recipient blocking, rate limiting and provider failures. Separate transactional OTP capacity from campaign volume (the Resend team quota is shared).
- [ ] CosmicForge Admin can create distinct `SHARED` inboxes for the four departments, assign members/sender grants, receive signed `email.received` events, fetch content and attachments, and reply from each identity.
- [ ] With the same Resend team/webhook, KarryGo's unrelated outgoing delivery events do not produce false Admin mailbox delivery states or leak a HealthNet event into Karryam Mail.
- [ ] Store results as test evidence; do not mark production email ready from a queued (HTTP 202) response alone.

## Known gaps; avoid accidental coupling

1. **SMTP acceptance is not Gmail inbox delivery.** KarryGo's existing SMTP adapter currently records the literal provider message ID `smtp`, not a unique Resend email ID. The current `SENT` state signifies the SMTP handoff, not delivery confirmation. Correlating individual Resend delivery/bounce webhooks back to a KarryGo notification requires separate design and testing (e.g. an API adapter returning provider IDs); do not pretend Admin's existing webhook supplies it automatically.
2. **Single Free-tier webhook.** CosmicForge Admin's existing Resend webhook may see email events from other services on the same account. Its processing must match only its own provider IDs/organizations and reject unsafe cross-brand assumptions. The optional Admin `mail.delivery_status@1` integration contract is an **event-reporting boundary**, not an instruction to route OTP sending through Admin.
3. **No Reply-To in the current Notification SMTP email template.** A user replying to a no-reply sender will not automatically reach Support. Reply-capable product communications should use a later approved Reply-To workflow (or be sent using an authorized corporate mailbox), not simply assume inbound replies arrive in `support@karryam.com`.
4. **Out-of-band Partner Web SMTP.** Centralize later only as a separately reviewed change; the first cutover can safely use the Partner Web's existing SMTP interface.

## Rollback / stop conditions

If provider verification fails, TLS fails, sender identity is rejected, OTP mail is not received, or the backend sends duplicate test mail, leave live sending/receiving gates OFF and investigate. Restoring the old domain may require recreating its verification and DNS records; deleting it is **not** an instantly reversible toggle. Do not modify CosmicForge Admin or Resend production settings as part of this documentation branch.

Official SMTP reference: https://resend.com/docs/send-with-smtp

Related: `deploy/notification-service/SMTP_STAGING.md`, `services/notification-service/README.md`, CosmicForge Admin's `docs/cosmicforge-mail-staging-e2e-acceptance.md`.
