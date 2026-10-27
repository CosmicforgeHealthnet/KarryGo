# Staging email delivery

> **New Karryam-domain plan:** Use [RESEND_KARRYAM_CUTOVER.md](RESEND_KARRYAM_CUTOVER.md)
> for the Resend SMTP configuration (`no-reply@karryam.com`), free-plan domain-slot
> migration, and independent Partner Web OTP sender. The cPanel settings below
> are historical staging examples; they are **not** the Resend cutover values.


Status: code prepared locally. VPS configuration, deployment and actual delivery
have not been verified. Existing debug settings remain unchanged.

## Notification service

Merge these values into the existing Notification environment, preserving its
database, Redis, Firebase and existing caller configuration:

```dotenv
NOTIFICATION_EMAIL_PROVIDER=smtp
NOTIFICATION_SMTP_HOST=mail.cosmicforgetech.com
NOTIFICATION_SMTP_PORT=465
NOTIFICATION_SMTP_TLS_MODE=tls
NOTIFICATION_SMTP_TIMEOUT_SECONDS=15
NOTIFICATION_SMTP_USERNAME=_mainaccount@cosmicforgetech.com
NOTIFICATION_SMTP_PASSWORD=
NOTIFICATION_EMAIL_FROM=
```

The username above is the authentication login shown by cPanel. Set the sender
to a valid address authorized by the hosting account; confirm it rather than
assuming the system login is the desired public From address. Enter the password
privately on the VPS in its protected environment file.

Append these callers to `NOTIFICATION_SERVICE_SECRETS` while retaining all existing
entries. Use separate generated secrets for the two services:

```text
vendor-service=<vendor-notification-secret>
driver-dispatch-delivery-service=<dispatch-notification-secret>
```

## Vendor

```dotenv
VENDOR_NOTIFICATION_URL=http://karrygo-notification-service:8106
VENDOR_NOTIFICATION_SECRET=<vendor-notification-secret>
```

Keep `VENDOR_AUTH_OTP_DEBUG` unchanged until a controlled real delivery test. It
currently bypasses sending entirely when true; the email test must run against
an instance configured with debug false. Preserve/restore the current value if
the test fails. Only leave debug disabled after successful delivery verification.

## Dispatch

```dotenv
DISPATCH_NOTIFICATION_URL=http://karrygo-notification-service:8106
DISPATCH_NOTIFICATION_SECRET=<dispatch-notification-secret>
```

Dispatch no longer reads `SMTP_HOST`, `SMTP_PORT`, `SMTP_TLS_MODE`, `SMTP_USER`,
`SMTP_PASSWORD` or `SMTP_FROM`. Remove those old credentials from its deployment
configuration after migration. Its email client now queues authentication and
verification emails through Notification. Phone-only flows still require SMS.

Verify the actual Notification container name, `HTTP_ADDR` and Docker network
before applying the example internal URLs above. Rebuild the changed Notification,
Dispatch and Vendor images and recreate them using their existing deployment
configuration, preserving volumes, ports, networks and unrelated settings.

## Delivery verification

1. Send one explicit test email through the authenticated Notification `/send`
   route to a tester-controlled mailbox. A 202/QUEUED response is acceptance only.
2. Confirm the message's delivery record is SENT and the email arrived. Check
   the error/delivery record for an SMTP failure instead of printing credentials.
3. Exercise Vendor registration/login and resend; each new code must arrive and
   only the latest code should verify. Test sensitive-action codes as well.
4. Exercise Dispatch signup/login, resend and verification emails. Confirm
   missing Notification configuration returns a clear failure outside development.
5. After real email delivery succeeds, keep debug OTP disabled. Phone number
   changes that need SMS remain unavailable until an SMS provider is integrated.

The local SMTP tests use only a loopback fake server. They do not prove cPanel
credentials, sender authorization, VPS connectivity, DNS delivery or inbox placement.
