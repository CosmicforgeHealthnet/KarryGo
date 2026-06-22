# Karry Go Architecture

This document is a handoff guide for continuing development in this monorepo. It describes the current architecture, service boundaries, data flow, persistence, API conventions, and business rules that should remain stable as features are added.

## System Overview

Karry Go is a logistics platform with multiple client apps and a Go microservice backend. The repository is organized as a monorepo so shared contracts, UI packages, and service helpers can evolve together.

The product surface is split into:

- Customer mobile app for signing in, onboarding, booking, delivery, hauling, and account flows.
- Provider mobile apps for taxi, dispatch delivery, and truck/haulage providers.
- Admin web app for operations, moderation, dashboards, and monitoring.
- Go services for business ownership, platform capabilities, auth/session handling, payments, notifications, media, and analytics.

There is intentionally no central identity service. User-facing services own their own auth entry and persistence, while common token, OTP, middleware, and service-auth helpers live in `shared/go`.

## Folder Structure

```text
apps/
  admin/                         Next.js admin/backoffice app
  customer/                      Flutter customer app
  dispatch_provider/             Flutter dispatch provider app scaffold
  taxi_provider/                 Flutter taxi provider app scaffold
  truck_provider/                Flutter truck provider app scaffold

packages/
  api_core/                      Shared Dart API config, error envelopes, ApiException
  ui_kit/                        Shared Flutter UI components and theme helpers

services/
  api-gateway/                   Public edge/routing service scaffold
  customer-service/              Customer auth, profile, saved locations, preferences
  driver-taxi-service/           Taxi provider and ride lifecycle scaffold
  driver-dispatch-delivery-service/ Dispatch rider and package delivery scaffold
  driver-hauling-service/        Truck provider and haulage workflow scaffold
  payment-wallet-service/        Wallets, ledger, payments, withdrawals, refunds
  notification-service/          Push, email, websocket, in-app notifications
  media-file-service/            Uploads, file metadata, Firebase Storage integration
  support-dispute-service/       Complaints, disputes, evidence, resolutions scaffold
  verification-compliance-service/ ID, license, vehicle document verification scaffold
  admin-backoffice-service/      Admin dashboards, moderation, user actions scaffold
  analytics-service/             Reporting and metrics scaffold
  Dockerfile                     Shared service build image

shared/go/
  apperrors/                     Standard domain error codes and HTTP status mapping
  auth/                          OTP, JWT-like HMAC tokens, bearer middleware, sessions
  httpx/                         Gin request ID, recovery, error envelope middleware
  serviceapp/                    Common Gin service bootstrap, health, ready, meta
  serviceauth/                   HMAC service-to-service request signing
  notifications/                 Notification-service client and request schema
  mediaclient/                   Media-file-service client
  walletclient/                  Payment-wallet-service client
  phonenumber/                   Nigerian phone normalization
  redisx/, cache/, pagination/, validation/, events/

infra/
  docker-compose.yml             Local Postgres, Redis, and service runtime wiring

docs/
  microservices-architecture.md  Original service-boundary note
  architecture.md                This handoff document
```

Go services follow the same feature-first shape when implemented:

```text
services/<service>/internal/
  config/                        Environment loading
  database/                      Postgres/Redis setup helpers
  features/<feature>/
    http/                        DTOs, route registration, handlers, response envelopes
    usecases/                    Business workflows and transactions
    models/                      Domain models and pure validation
    repositories/                Persistence interfaces and Postgres/Redis implementations
    clients/                     External service/provider adapters
  migrations/                    Service-owned SQL migrations
```

## Modules And Services

### Client Apps

| App | Stack | Purpose |
|---|---|---|
| `apps/customer` | Flutter | Customer onboarding, OTP auth, profile setup screens, home/dashboard. Uses `packages/api_core` and `packages/ui_kit`. |
| `apps/taxi_provider` | Flutter | Taxi provider app scaffold. |
| `apps/dispatch_provider` | Flutter | Dispatch rider/provider app scaffold. |
| `apps/truck_provider` | Flutter | Truck/haulage provider app scaffold. |
| `apps/admin` | Next.js | Admin web app scaffold with shared app layout and error component. |

The customer app currently owns the most complete frontend flow. Its auth controller checks local secure session state, refreshes expired tokens, calls `/me`, then routes through onboarding, OTP verification, profile setup, or home.

### Backend Services

| Service | API base | Port | Primary responsibility |
|---|---:|---:|---|
| `api-gateway` | `/api/v1/gateway` or configured base | `8080` | Public client entry point and routing layer scaffold. |
| `customer-service` | `/api/v1/customer` | `8101` | Customer auth, refresh sessions, customer profile, saved locations, preferences. |
| `driver-taxi-service` | `/api/v1/taxi` | `8102` | Taxi providers, cars, ride bookings, matching, trip lifecycle. |
| `driver-dispatch-delivery-service` | `/api/v1/dispatch-delivery` | `8103` | Dispatch riders, bikes, package bookings, matching, proof of delivery. |
| `driver-hauling-service` | `/api/v1/hauling` | `8104` | Truck providers, trucks, haulage bookings, matching, cargo workflow. |
| `payment-wallet-service` | `/api/v1/payments` | `8105` | Wallet accounts, double-entry ledger, payment intents, Paystack integration, withdrawals, refunds. |
| `notification-service` | `/api/v1/notifications` | `8106` | Notification request intake, delivery fanout, email/push/websocket/in-app. |
| `support-dispute-service` | `/api/v1/support-dispute` | `8107` | Complaints, disputes, evidence, resolutions scaffold. |
| `verification-compliance-service` | `/api/v1/verification-compliance` | `8108` | Provider ID, license, vehicle document verification scaffold. |
| `media-file-service` | `/api/v1/media-files` | `8109` | Authenticated internal file uploads and metadata. |
| `admin-backoffice-service` | `/api/v1/admin-backoffice` | `8110` | Admin dashboards, moderation, user actions scaffold. |
| `analytics-service` | `/api/v1/analytics` | `8111` | Analytics and read models scaffold. |

All services started through `shared/go/serviceapp` expose:

- `GET /health`
- `GET /ready`
- `GET <api-base>/meta`

## Data Flow

### Customer Auth Flow

1. Customer app opens and `CustomerAuthController.initialize()` reads secure local session storage.
2. If no session exists, user enters phone or email and calls `POST /api/v1/customer/auth/start`.
3. `customer-service` normalizes the identifier:
   - phone through Nigerian phone normalization, e.g. `080...` to `+234...`
   - email by trimming and lowercasing
4. Service generates a numeric OTP, hashes it with the OTP secret, challenge ID, and typed identifier key, and stores the challenge in customer Redis.
5. Phone OTPs are logged in local mode. Email OTPs can be sent through `notification-service` when `CUSTOMER_NOTIFICATION_BASE_URL` and `CUSTOMER_NOTIFICATION_SECRET` are configured.
6. User submits code to `POST /api/v1/customer/auth/verify`.
7. Service verifies OTP, creates or loads the customer by phone/email, creates a refresh session in Postgres, signs an access token, and returns tokens plus customer profile.
8. App stores session locally, then routes to profile setup if `onboarding_status == "profile_required"` or home if complete.
9. Expired access tokens are rotated through `POST /api/v1/customer/auth/refresh`; logout revokes the refresh session.

### Notification Flow

1. Internal services submit `notifications.Request` either through signed HTTP `POST /api/v1/notifications/send` or Redis Stream `notification:requests`.
2. Notification service validates request shape, resolves templates/default channels, persists a `notification_messages` row, and creates one `notification_deliveries` row per channel.
3. Delivery workers process queued/retryable deliveries and call channel providers:
   - Firebase Cloud Messaging for push when configured, otherwise logging sender.
   - SMTP for email when configured, otherwise logging sender.
   - WebSocket hub for live delivery.
   - In-app persistence for durable user-visible notifications.
4. Attempts and provider metadata are recorded for retry/audit.

### Wallet And Payment Flow

1. Customer-facing wallet endpoints use customer bearer auth.
2. Provider-facing wallet endpoints use provider bearer auth with service-specific provider token secrets.
3. Operational services use HMAC service auth for internal payment intents, wallet payments, job completion, refunds, and internal payment lookup.
4. Ledger transactions are idempotent by source reference or explicit idempotency key.
5. Ledger entries must balance debits and credits in the same currency and amount semantics.
6. Paystack integration owns external references, authorization URLs, webhooks, recipient codes, transfers, and refunds.

### Media Upload Flow

1. Internal service calls media-file service with `X-Karrygo-Service` and bearer service token.
2. Request uploads multipart file plus owner metadata and purpose.
3. Upload service validates owner service, purpose, file size, content metadata, and storage config.
4. File bytes go to Firebase Storage or configured storage adapter; metadata is persisted in `media_assets`.
5. Returned URL is treated as permanent public URL for v1.

## API Structure

### Common API Conventions

Success responses generally use:

```json
{
  "success": true,
  "data": {}
}
```

Errors use `shared/go/apperrors` and `shared/go/httpx`:

```json
{
  "success": false,
  "error": {
    "code": "validation_failed",
    "message": "Check your details.",
    "request_id": "request-id",
    "fields": []
  }
}
```

Every request receives an `X-Request-ID`. If the client does not provide one, middleware generates it.

### Active Customer Endpoints

Base: `/api/v1/customer`

| Method | Path | Auth | Notes |
|---|---|---|---|
| `POST` | `/auth/start` | public | Start OTP for exactly one of `phone` or `email`. |
| `POST` | `/auth/verify` | public | Verify OTP and issue access/refresh tokens. |
| `POST` | `/auth/refresh` | public refresh token | Rotate refresh session. |
| `POST` | `/auth/logout` | refresh token payload | Revoke refresh session. |
| `GET` | `/me` | customer bearer | Return authenticated customer profile. |

### Active Payment/Wallet Endpoints

Base: payment-wallet service API base.

| Method | Path | Auth |
|---|---|---|
| `GET` | `/wallets/me` | customer bearer |
| `GET` | `/wallets/me/transactions` | customer bearer |
| `POST` | `/topups` | customer bearer |
| `GET` | `/provider/earnings` | provider bearer |
| `POST` | `/provider/bank-accounts/resolve` | provider bearer |
| `POST` | `/provider/bank-accounts` | provider bearer |
| `POST` | `/provider/withdrawals` | provider bearer |
| `POST` | `/internal/payment-intents` | service HMAC |
| `POST` | `/internal/payment-intents/:id/pay-from-wallet` | service HMAC |
| `POST` | `/internal/jobs/:source_service/:source_reference/complete` | service HMAC |
| `POST` | `/internal/refunds` | service HMAC |
| `GET` | `/internal/payments/:reference` | service HMAC |
| `POST` | `/webhooks/paystack` | Paystack webhook verification logic |

### Active Notification Endpoints

Base: `/api/v1/notifications`

| Method | Path | Auth |
|---|---|---|
| `POST` | `/send` | service HMAC |
| `POST` | `/devices` | service HMAC |
| `POST` | `/realtime/token` | service HMAC |
| `GET` | `/messages` | service HMAC |
| `GET` | `/messages/:id` | service HMAC |
| `GET` | `/ws` | realtime token / websocket flow |

### Active Media Endpoints

Base: `/api/v1/media-files`

| Method | Path | Auth |
|---|---|---|
| `POST` | `/uploads` | service token |
| `GET` | `/files/:id` | service token |
| `GET` | `/files` | service token |
| `DELETE` | `/files/:id` | service token |

## Database Structure

Each service owns its own database. Cross-service data should be referenced by IDs, not foreign keys across service databases.

### Customer Service

Migrations:

- `services/customer-service/migrations/001_customer_auth.sql`
- `services/customer-service/migrations/002_customer_email_auth.sql`

Tables:

- `customers`: customer profile root. Fields include `id`, optional unique `phone`, optional unique `email`, `first_name`, `last_name`, `onboarding_status`, `status`, timestamps. At least one identifier must exist.
- `customer_sessions`: refresh sessions with hashed refresh token, device metadata, expiry, and revocation timestamp.
- `customer_auth_events`: audit table for auth events by customer, phone/email, user agent, and IP.

Redis:

- OTP challenges keyed by typed identifier, e.g. `customer:auth:otp:phone:+234...` or `customer:auth:otp:email:ada@example.com`.
- OTP rate limits keyed by the same typed identifier.

### Notification Service

Migration: `services/notification-service/migrations/001_notifications.sql`

Tables:

- `notification_messages`: canonical notification request, recipient, channels, template/inline body, priority, status.
- `notification_deliveries`: per-channel delivery rows with status, attempts, provider metadata, retry schedule.
- `notification_delivery_attempts`: append-only delivery attempt audit.
- `notification_templates`: localized templates and default channel sets.
- `notification_preferences`: recipient-level channel preferences.
- `notification_devices`: push device tokens by recipient and app/platform.

Redis:

- `notification:requests`
- `notification:deliveries`
- `notification:dead_letters`

### Payment/Wallet Service

Migration: `services/payment-wallet-service/migrations/001_wallet_ledger.sql`

Tables:

- `wallet_accounts`: owner-scoped accounts by owner type/id, account type, currency, normal balance.
- `ledger_transactions`: immutable transaction headers with reference, type, status, source reference, idempotency key, metadata.
- `ledger_entries`: debit/credit entries linked to transactions.
- `payment_intents`: Paystack/customer payment lifecycle and platform fee data.
- `paystack_webhook_events`: webhook idempotency and processing audit.
- `provider_bank_accounts`: provider payout recipients.
- `withdrawals`: provider transfer requests and status.
- `refunds`: refund records against payment intents.
- `idempotency_keys`: reusable idempotent response cache by key/scope/actor.

### Media/File Service

Migration: `services/media-file-service/migrations/001_media_assets.sql`

Table:

- `media_assets`: storage metadata including owner service/id, purpose, original filename, content type, size, checksum, bucket/path, public URL, status, metadata, uploader, and delete marker.

### Scaffolded Service Databases

Docker Compose provisions Postgres databases for taxi, dispatch delivery, hauling, support/dispute, verification/compliance, admin backoffice, and analytics. Their feature directories are present, but schemas and full handlers should be added as those domains are implemented.

## Key Design Patterns

- Feature-first Go services: keep HTTP, usecases, models, repositories, and clients inside the owning feature.
- Thin handlers: parse DTOs, call usecases, return envelopes. Business rules belong in usecases/models.
- Repository interfaces: usecases depend on interfaces; Postgres/Redis implementations live in repositories.
- Adapter clients: external providers and internal services are accessed through clients, not directly from handlers.
- Shared middleware: all services use request ID, recovery, and error envelope middleware from `shared/go/httpx`.
- Service bootstrap: `shared/go/serviceapp.Run` standardizes health, readiness, meta routes, Gin setup, graceful shutdown, and API base grouping.
- Service-owned persistence: no cross-service database joins. Communicate with IDs, service clients, events, or read models.
- HMAC service auth: internal service-to-service calls use `shared/go/serviceauth`, not end-user bearer tokens.
- Customer/provider bearer auth: user tokens are HMAC-signed claims with subject, role, service, session ID, type, issue time, and expiry.
- Idempotency where money or notifications are involved: wallet/payment operations and notification sends must preserve idempotent semantics.

## Dependencies Between Components

### Frontend Dependencies

- Flutter apps depend on:
  - `packages/api_core` for API URL building and error parsing.
  - `packages/ui_kit` for reusable widgets/theme.
  - `http` for API calls.
  - `flutter_secure_storage` for customer session storage.
- Customer app talks directly to `customer-service` in local development through `CUSTOMER_API_BASE_URL`. Long term, it should go through `api-gateway`.
- Admin app is a Next.js scaffold with local component and app-route structure.

### Backend Dependencies

- All Go services depend on `shared/go`.
- Customer service depends on:
  - Postgres for customers and refresh sessions.
  - Redis for OTP challenges and rate limits.
  - Notification service for email OTP delivery when configured.
- Notification service depends on:
  - Postgres for message/delivery persistence.
  - Redis streams for async request and delivery queues.
  - Firebase credentials for push when configured.
  - SMTP config for email when configured.
- Payment-wallet service depends on:
  - Postgres for wallet ledger and payment state.
  - Paystack for payment/transfer/refund provider operations.
  - Customer/provider token secrets and service HMAC secrets for endpoint auth.
- Media-file service depends on:
  - Postgres for media metadata.
  - Firebase Storage config/credentials for object storage.
  - Service token config for internal caller auth.

## Important Business Logic Rules

### Auth And Sessions

- Exactly one auth identifier should be submitted for customer OTP start/verify: `phone` or `email`, never both.
- Phone identifiers must normalize to Nigerian `+234...` format.
- Email identifiers must be trimmed and lowercased before challenge storage or customer lookup.
- OTP codes are numeric and default to six digits.
- OTP hashes bind together secret, challenge ID, typed identifier key, and code.
- OTP challenges expire and failed attempts are capped.
- OTP start requests are rate-limited per typed identifier.
- Refresh tokens are opaque values prefixed by refresh session ID and stored only as HMAC hashes.
- Refresh rotates the session: old refresh session is revoked and a new refresh/access token pair is issued.
- Access tokens must include subject, role, service, session ID, token type, issued-at, and expiry claims.

### Customer Profile

- Customer records can be created on first verified sign-in.
- A customer may have phone, email, or both; at least one identifier is required.
- `onboarding_status == "profile_required"` means the app should continue profile setup before normal authenticated home.
- The current profile setup flow in the customer app is partly frontend-side; add profile update APIs before relying on those fields as persisted business data.

### Notifications

- Notification requests must include idempotency key, source service, event type, recipient type/id, and either a template key or inline title/body.
- Supported v1 channels are `push`, `email`, `websocket`, and `in_app`.
- If channels are omitted, template defaults are used; otherwise fallback channels are push, websocket, and in-app.
- HTTP notification endpoints require service HMAC auth.
- Email and push providers fall back to logging senders when provider configuration is missing.

### Wallets And Payments

- Financial data stays isolated in `payment-wallet-service`.
- Wallet account uniqueness is owner type/id + account type + currency.
- Ledger entries must use positive kobo amounts and valid sides: `debit` or `credit`.
- Payment intent uniqueness is source service + source reference.
- Idempotency keys must be honored for payment, withdrawal, and refund flows.
- Paystack webhook events are stored by unique event key before processing to prevent duplicate side effects.

### Media Files

- Upload callers must authenticate as an internal service.
- `owner_service` must match the authenticated service.
- Supported media purposes are profile photo, document file, proof image, and signature.
- Files are recorded with checksum, storage path, public URL, and owner metadata.
- v1 public URLs are expected to be permanently readable.

### Service Boundaries

- Taxi bookings and matching belong in `driver-taxi-service`.
- Package delivery bookings, rider matching, and proof of delivery belong in `driver-dispatch-delivery-service`.
- Truck haulage bookings, truck matching, and cargo workflow belong in `driver-hauling-service`.
- Shared platform concerns like payments, notifications, media, verification, support, admin, and analytics stay in their platform services.
- Avoid moving profile fields into auth-only packages; auth can use profile repositories, but profile remains its own feature.

## Local Development

Run all local services and dependencies:

```bash
docker compose -f infra/docker-compose.yml up --build
```

Useful checks:

```bash
go test ./...
flutter analyze
flutter test
npm run build
```

Common local URLs:

- API gateway: `http://localhost:8080`
- Customer service: `http://localhost:8101/api/v1/customer`
- Payment-wallet service: `http://localhost:8105`
- Notification service: `http://localhost:8106/api/v1/notifications`
- Media-file service: `http://localhost:8109/api/v1/media-files`

When adding a new feature, prefer this sequence:

1. Add or update domain model and usecase tests.
2. Define repository/client interfaces at the usecase boundary.
3. Add persistence migrations and repository implementation.
4. Add HTTP DTOs/routes/handlers with standard envelopes.
5. Wire service config and `cmd/main.go`.
6. Update frontend API models/controllers if client-facing.
7. Run focused tests first, then broader service/app checks.
