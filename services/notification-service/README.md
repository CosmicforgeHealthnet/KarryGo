# Notification Service

Owns push, email, in-app notifications, delivery attempts, and retry
handling for async platform updates.

## Current Implementation

This service is the central notification hub for Karry Go. Other services can
submit the same notification request shape through either:

- `POST /api/v1/notifications/send`
- Redis Stream `notification:requests`

The optional `channels` field controls delivery media. If it is omitted, the
service uses template defaults or falls back to `push`, `websocket`, and
`in_app`.

Supported v1 channels:

- `push`: Firebase Cloud Messaging when Firebase env vars are configured;
  otherwise logs locally.
- `email`: SMTP when configured. Missing configuration returns unavailable for email-only requests. Explicit `logging` is development/test only.
- `websocket`: live delivery to connected clients.
- `in_app`: durable notification record.

HTTP service-to-service endpoints use shared HMAC headers from
`shared/go/serviceauth`, not customer/provider bearer tokens.

## Local Bootstrap

Use the helper script from the repo root for a full local setup:

```bash
./scripts/notification-local-bootstrap.sh
```

It starts local Postgres on `5438`, Redis on `6385`, creates the
`notification_service` database if needed, applies the notification migrations,
and prints the command to run the service.

If you prefer to do it manually:

```bash
mkdir -p /tmp/postgres-notification
initdb -D /tmp/postgres-notification
pg_ctl -D /tmp/postgres-notification -o "-p 5438" start

mkdir -p /tmp/redis-6385
redis-server --port 6385 --dir /tmp/redis-6385 --daemonize yes

psql -p 5438 -d template1
```

Inside `psql`:

```sql
CREATE USER cosmicforge_logistics WITH PASSWORD 'cosmicforge_logistics';
CREATE DATABASE notification_service OWNER cosmicforge_logistics;
```

Then apply the migration:

```bash
psql "postgres://cosmicforge_logistics:cosmicforge_logistics@localhost:5438/notification_service?sslmode=disable" \
  -f services/notification-service/migrations/001_notifications.sql
```

## V4 Section 16 customer hints (local status)

The existing signed service request, durable feed, push/email/websocket
delivery and retry machinery remain the notification authority. Navigation
intents are derived from controlled event families and opaque references; a
customer must read current Ride, Send, Truck, Commerce, Share or Wallet state
from the owning service after opening a hint. Production migrations `005`–`008`
seed Ride, Share and financial templates without interpolating business data.
Taxi Ride milestones, Customer Share create/revoke/expire and Wallet status
changes now have committed notification intents with stable retry identity.
Template rendering and email delivery no longer log message bodies, recipient
addresses or rendered credentials.
The shared request validator rejects credential and bank-secret keys, including
nested data, for ordinary events and custody hints. Only the separate corporate
login OTP event retains its explicit legacy exception. Migration `008` replaces
handover template copy that previously interpolated a code.

**Section 16 remains PARTIAL locally.** Dispatch, Hauling, Taxi Goods and Vendor
Own Delivery custody hints now carry references without OTP/QR secrets. Full
receiver-channel binding and the verified email-only secure-read contract are
still required before the affected handover lifecycle can be certified. A
phone-only receiver without an authenticated account or other supported secure
channel must fail closed rather than receiving a fictitious credential notice.
Deployment of signed Notification callers and consumer navigation is still
subject to staging certification.


## SMTP and authentication email

Vendor and Dispatch send email requests to this service over authenticated internal HTTP.
Configure SMTP here, not in the mobile apps or Dispatch service. See
[staging email setup](../../deploy/notification-service/SMTP_STAGING.md).

`NOTIFICATION_SMTP_TLS_MODE=tls` uses implicit TLS (port 465).
`starttls` requires STARTTLS support (usually port 587); it never silently downgrades.
`NOTIFICATION_SMTP_TIMEOUT_SECONDS` defaults to 15 and bounds connection, TLS,
authentication and message submission. Permanent SMTP rejections stop retrying;
transient failures use the existing retry queue. A successful `/send` response
means queued, not inbox delivery. Actual delivery status is recorded by the worker.

Logging email requires an explicit `NOTIFICATION_EMAIL_PROVIDER=logging` and
`APP_ENV=development` or `test`. Staging/production never treat the logging sender
as delivered email. Push configuration and other callers are unchanged.

SMTP messages include an HTML Karryam layout and an unchanged plain-text fallback.
The transparent logo from `web/karryam-landing/public/images/logo.png` is embedded
as an inline MIME image; displaying it does not require a public asset URL.
The layout supports small screens and a dark color scheme in compatible clients.
Vendor authentication and Dispatch authentication events use a prominent code
panel; Vendor step-up events use action-confirmation copy. Other notifications
retain their supplied subject and body inside the shared layout. If an existing
authentication body cannot be recognized, it falls back to the full message
rather than hiding or inventing a code. Expiry wording stays non-numeric because
the current notification contract does not carry the configured OTP lifetime.
Message text is HTML-escaped, and subjects are MIME-encoded.

Generate standalone sample previews without SMTP or real accounts:

```bash
NOTIFICATION_EMAIL_PREVIEW_DIR="$PWD/build/email-previews" \
  go test ./internal/features/messages/clients -run TestEmailVariantsAndPreviews
```

These are browser previews, not certification of rendering in every mail client.
Only Notification needs rebuilding to apply this layout; no database migration,
new API fields, or mobile build is required.
