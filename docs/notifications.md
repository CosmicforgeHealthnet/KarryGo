# Notifications

`notification-service` is the central service for all platform notifications. It
owns delivery across four channels — `push` (FCM), `email` (SMTP), `websocket`
(realtime), and `in_app` (durable feed) — plus templates, recipient preferences,
device registry, retry/dead-letter, and idempotency. Other services never send
push/email/websocket directly; they call notification-service.

## Sending notifications from a backend service

Use the shared client `shared/go/notifications`. Every call is signed with the
service HMAC secret (`shared/go/serviceauth`).

1. Add config for the notification base URL and HMAC secret, e.g.
   `HAULING_NOTIFICATION_URL` / `HAULING_NOTIFICATION_SECRET`,
   `PAYMENT_WALLET_NOTIFICATION_URL` / `PAYMENT_WALLET_NOTIFICATION_SECRET`.
2. The secret must match an entry in notification-service's
   `NOTIFICATION_SERVICE_SECRETS` (`servicename=secret,...`), keyed by the
   `ServiceName` the sender signs with.
3. Wrap `notifications.Client` in a small per-feature notifier (see
   `services/driver-hauling-service/internal/features/booking/clients/notification_client.go`
   and the equivalent in `payment-wallet-service`). When URL/secret are empty the
   wrapper is a **no-op**, so local dev without notification-service still works.
4. **Fire-and-forget.** A failed notification must never roll back or block the
   underlying booking/payment. Log the error, don't return it.

```go
notifier.Send(ctx, notifications.Request{
    IDempotencyKey: notifications.IdempotencyKey("driver-hauling-service", notifications.EventBookingMatched, bookingID),
    SourceService:  "driver-hauling-service",
    EventType:      notifications.EventBookingMatched,
    Recipient:      notifications.Recipient{Type: notifications.RecipientProvider, ID: providerID},
    TemplateKey:    notifications.EventBookingMatched, // resolves a seeded template
    TemplateData:   map[string]interface{}{"booking_id": bookingID},
})
```

- `IdempotencyKey` is deterministic (`source:event:entity`) via
  `notifications.IdempotencyKey`, so retries and webhook redeliveries dedupe.
- Prefer the `Event*` and `Recipient*` constants over string literals.
- `TemplateKey == event_type`. If no template is seeded, inline `Title`/`Body`
  are used instead.

## Recipient types

- `notifications.RecipientCustomer` (`"customer"`) — `recipient.id` is the
  customer id (token subject).
- `notifications.RecipientProvider` (`"provider"`) — `recipient.id` is the
  provider id.

## Event catalog

| Event type | Recipient | Source service |
|---|---|---|
| `booking.matched` | provider | driver-hauling-service |
| `booking.accepted` | customer | driver-hauling-service |
| `booking.unmatched` | customer | driver-hauling-service |
| `cargo.picked_up` | customer | driver-hauling-service |
| `cargo.delivered` | customer | driver-hauling-service |
| `booking.completed` | customer | driver-hauling-service |
| `booking.cancelled` | provider | driver-hauling-service |
| `booking.cancelled_by_provider` | customer | driver-hauling-service |
| `payment.topup_success` | customer | payment-wallet-service |
| `payment.success` | customer | payment-wallet-service |
| `payment.failed` | customer | payment-wallet-service |
| `withdrawal.completed` | provider | payment-wallet-service |
| `withdrawal.failed` | provider | payment-wallet-service |
| `withdrawal.reversed` | provider | payment-wallet-service |
| `customer.auth.otp` | customer | customer-service (email, inline) |

Taxi and dispatch services will follow the same pattern once their booking
usecases exist.

## Templates

Templates live in `notification_templates (key, locale, title, body,
default_channels)`, keyed by `(event_type, locale)`, with `{{placeholder}}`
interpolation from `template_data`. Seed/refresh them with:

```bash
cd services/notification-service && go run ./cmd/seed
```

The seed (`seeds/dev_templates.sql`) is an idempotent upsert — safe to re-run
after editing copy.

## App → notification-service (proxy flow)

Apps hold a customer/provider **bearer** token, not the HMAC secret, so they
never call notification-service directly. The owning service brokers it:

- customer app → `customer-service` `GET/POST /api/v1/customer/notifications...`
- provider app → `driver-hauling-service` `GET/POST /api/v1/hauling/provider/notifications...`

Each owning service exposes three bearer-protected endpoints (recipient is always
the token subject, so a user only sees their own notifications):

| Method | Path (suffix) | Purpose |
|---|---|---|
| `GET` | `/notifications` | List the recipient's recent notifications (feed) |
| `POST` | `/notifications/realtime-token` | Mint a short-lived websocket token |
| `POST` | `/notifications/devices` | Register an FCM device token for push |

The app then opens a websocket to notification-service
`GET /api/v1/notifications/ws?token=<realtime_token>` for live delivery, and
registers its FCM token (via the proxy) so background push works. These proxy
routes are skipped automatically when the owning service has no notification
base URL configured.

## Push (FCM)

The server-side FCM HTTP v1 sender is implemented
(`services/notification-service/.../firebase_push_sender.go`). It activates when
`FIREBASE_PROJECT_ID` and `GOOGLE_APPLICATION_CREDENTIALS` are set; otherwise it
falls back to a logging sender. Apps must register a device token (proxy
`POST .../notifications/devices`) for push to reach a device.

### Client-side FCM — remaining manual step

The app-side device-registration plumbing is in place but **decoupled from
Firebase** so the apps build without the Firebase project config. To enable real
device tokens:

1. Run `flutterfire configure` in `apps/customer` and `apps/truck_provider`
   (adds `firebase_options.dart`, `google-services.json`,
   `GoogleService-Info.plist`, and the Gradle/iOS plugin wiring).
2. Add `firebase_core` + `firebase_messaging` to each app's `pubspec.yaml` and
   `Firebase.initializeApp(...)` in `main.dart`.
3. Provide a token source to `PushRegistrationService`
   (`apps/customer/lib/features/notifications/data/push_registration_service.dart`):
   request notification permission, then `FirebaseMessaging.instance.getToken()`,
   and re-register on `onTokenRefresh`. Inject the service into
   `NotificationController` (the `pushRegistration` parameter).

Until then, in-app feed + websocket realtime work fully; only background OS push
is pending the Firebase project config.

## Adding a new provider service (taxi / dispatch)

`driver-taxi-service` and `driver-dispatch-delivery-service` are currently empty
scaffolds — no auth, no bookings, no provider tokens — so they neither send nor
receive notifications yet. When they are built, exposing the notification system
to them is the **same mechanical wiring** `driver-hauling-service` already uses.
Using taxi as the worked example (substitute `dispatch` / its own ports/secrets):

### A. Sending side (once ride/delivery bookings exist)

1. **Event constants** — add the events to
   [`shared/go/notifications/notifications.go`](../shared/go/notifications/notifications.go),
   e.g. `EventRideMatched = "ride.matched"`, `ride.accepted`, `ride.arrived`,
   `ride.completed`, `ride.cancelled`.
2. **Config** — add `TAXI_NOTIFICATION_URL` + `TAXI_NOTIFICATION_SECRET` to the
   service config (mirror hauling's
   [`internal/config/config.go`](../services/driver-hauling-service/internal/config/config.go)).
3. **Notifier wrapper** — copy
   [`booking/clients/notification_client.go`](../services/driver-hauling-service/internal/features/booking/clients/notification_client.go)
   into the taxi feature, with one method per event. Keep it fire-and-forget
   (log on error, never fail the booking) and no-op when URL/secret are empty.
4. **Emit at transitions** — inject the notifier into the ride usecase and call it
   after each state change, exactly as hauling's
   [`booking_service.go`](../services/driver-hauling-service/internal/features/booking/usecases/booking_service.go)
   does. Build the idempotency key with `notifications.IdempotencyKey`.
5. **Register the sender secret** — add taxi to notification-service's
   `NOTIFICATION_SERVICE_SECRETS`, e.g.
   `...,driver-taxi-service=development-taxi-notification-secret`, and set the same
   value as `TAXI_NOTIFICATION_SECRET` in the taxi `.env`.
6. **Templates** — add rows for the new event types to
   [`seeds/dev_templates.sql`](../services/notification-service/seeds/dev_templates.sql)
   and re-run `go run ./cmd/seed`.

### B. Receiving side (once taxi provider auth exists)

7. **Proxy feature** — copy
   [`internal/features/notifications/http/`](../services/driver-hauling-service/internal/features/notifications/http/)
   into the taxi service, swapping `ProviderRole` / `ProviderService` for taxi's
   auth constants, and register the route in `cmd/main.go`. That gives the taxi
   provider app the same three bearer endpoints (`/provider/notifications`,
   `/provider/notifications/realtime-token`, `/provider/notifications/devices`).
8. **Provider app** — reuse the
   [`provider_notification_api.dart`](../apps/truck_provider/lib/features/notifications/data/provider_notification_api.dart)
   + [`provider_realtime_listener.dart`](../apps/truck_provider/lib/features/notifications/data/provider_realtime_listener.dart)
   pattern in the taxi provider app, pointed at the taxi API base.

### C. Postman / docs

9. Add a "Taxi Provider Proxy" folder to the collection (clone the hauling proxy
   folder, point it at the taxi base URL/port), and a `-service=taxi` branch to the
   devtoken helper so its bearer tokens are signed with the taxi provider secret.

Nothing in `shared/go/notifications`, the HMAC signing, the template engine, or the
proxy contract needs to change — you are only repeating the per-service wiring.
