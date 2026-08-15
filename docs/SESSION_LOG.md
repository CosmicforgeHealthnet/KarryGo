# CLAUDE.md

## Project Overview

Karry Go / Cosmicforge Logistics is a multi-application logistics platform in a monorepo.

High-level architecture:

- Flutter mobile apps for customers and providers.
- Next.js admin web app.
- Go microservice backend.
- Shared Go platform utilities under `shared/go`.
- Shared Flutter/Dart packages under `packages`.
- Local infrastructure under `infra/docker-compose.yml`.

There is no standalone identity service. User-facing services own their own auth entry and persistence. Common auth, OTP, token, HTTP, service-auth, notification, media, wallet, validation, Redis, and pagination helpers live in `shared/go`.

Primary architecture reference: `docs/architecture.md`. Older service-boundary reference: `docs/microservices-architecture.md`.

## System Structure

### Frontend Apps

- `apps/customer`: Flutter customer app. Most complete client flow: onboarding, phone/email OTP auth, session storage, profile setup screens, home screen.
- `apps/taxi_provider`: Flutter taxi provider app scaffold.
- `apps/dispatch_provider`: Flutter dispatch provider app scaffold.
- `apps/truck_provider`: Flutter truck/haulage provider app scaffold.
- `apps/admin`: Next.js admin/backoffice app scaffold.

### Shared Frontend Packages

- `packages/api_core`: Dart API config, URI building, API error envelope parsing, `ApiException`.
- `packages/ui_kit`: Reusable Flutter UI components and theme helpers.

### Backend Services

- `services/api-gateway`: Public edge/routing service scaffold.
- `services/customer-service`: Customer auth, refresh sessions, profile, saved locations, preferences, request history.
- `services/driver-taxi-service`: Taxi providers, cars, ride bookings, matching, trip lifecycle scaffold.
- `services/driver-dispatch-delivery-service`: Dispatch riders, bikes, package delivery bookings, matching, proof of delivery scaffold.
- `services/driver-hauling-service`: Truck providers, trucks, haulage bookings, matching, cargo workflow scaffold.
- `services/payment-wallet-service`: Wallet accounts, double-entry ledger, payment intents, Paystack integration, withdrawals, refunds.
- `services/notification-service`: Push, email, websocket, in-app notifications, delivery attempts, retry handling.
- `services/media-file-service`: Internal file uploads, media metadata, Firebase Storage integration.
- `services/support-dispute-service`: Complaints, disputes, evidence, resolutions scaffold.
- `services/verification-compliance-service`: Provider ID, license, vehicle document verification scaffold.
- `services/admin-backoffice-service`: Admin dashboards, moderation, user actions scaffold.
- `services/analytics-service`: Reporting and read models scaffold.
- `services/vendor-service`: Vendor Service / Commerce Logistics backend — Sections 5–7 now implement staff auth/IAM, business/brand/branch capabilities, onboarding/KYB projections, representative/document references, masked bank onboarding, verification reconciliation and policy activation. CRM/catalogue/commerce/fulfilment remain future sections. See `services/vendor-service/docs/`.
- `services/fleet-partner-service`: Corporate Logistics Backend — organizations, staff/IAM, RBAC, approvals, managed providers, marketplace assignments, trip projections, wallet finance integration, CRM/contracts/quotations, documents, search, exports, imports, and a real event platform (Redis Streams outbox/inbox). By far the largest and most mature backend service (54+ feature directories) but was never added to this file's own service listing by the sessions that built it — see `services/fleet-partner-service/docs/` and `services/fleet-partner-service/README.md` for its own documentation, not this file's session log below (which only tracked the consumer/driver apps until the vendor-service architecture session added this line).

### Shared Backend Libraries

- `shared/go/apperrors`: Standard app errors and HTTP status mapping.
- `shared/go/httpx`: Gin request ID, recovery, and error envelope middleware.
- `shared/go/serviceapp`: Standard service bootstrap with `/health`, `/ready`, and `/meta`.
- `shared/go/auth`: OTP, token signing/verification, bearer middleware, session helpers.
- `shared/go/serviceauth`: HMAC service-to-service request signing.
- `shared/go/notifications`: Notification-service request/client schema.
- `shared/go/mediaclient`: Media-file-service client.
- `shared/go/walletclient`: Payment-wallet-service client.
- `shared/go/phonenumber`: Nigerian phone number normalization.
- `shared/go/redisx`, `cache`, `pagination`, `validation`, `events`: Shared platform helpers.

### Standard Go Service Layout

Use this structure for implemented service features:

```text
services/<service>/internal/
  config/
  database/
  features/<feature>/
    http/
    usecases/
    models/
    repositories/
    clients/
  migrations/
```

Handlers parse DTOs and call usecases. Usecases own business workflow. Models own domain rules and pure validation. Repositories own persistence interfaces/implementations. Clients own external/internal service adapters.

## Architecture Rules

### Service Boundaries

- Do not import `internal` packages from another service.
- Do not share business logic by copying it between services.
- Put reusable cross-service logic in `shared/go`, but only when it is genuinely generic.
- Each service owns its own database schema and migrations.
- Do not create cross-service database foreign keys or joins.
- Cross-service references should use IDs and service-owned APIs/events/read models.

### Business Ownership

- Customer auth/profile/preferences/history belong in `customer-service`.
- Taxi booking, matching, and trip lifecycle belong in `driver-taxi-service`.
- Package delivery booking, rider matching, delivery lifecycle, and proof of delivery belong in `driver-dispatch-delivery-service`.
- Truck haulage booking, truck matching, and cargo workflow belong in `driver-hauling-service`.
- Payments, wallets, ledger, withdrawals, refunds, and Paystack integration belong in `payment-wallet-service`.
- Notifications belong in `notification-service`.
- File uploads and media metadata belong in `media-file-service`.
- Verification/compliance, support/disputes, admin operations, and analytics stay in their respective services.

### Communication Rules

- Client apps should ultimately communicate through `api-gateway`. Current local customer app may directly target `customer-service` via `CUSTOMER_API_BASE_URL` until gateway routing is completed.
- Internal service-to-service HTTP calls must use defined clients/contracts and HMAC service auth from `shared/go/serviceauth`, not end-user bearer tokens.
- User-facing protected endpoints use bearer auth from `shared/go/auth`.
- Do not bypass service APIs by reading another service database.
- Use Redis streams/events for asynchronous platform workflows where appropriate, especially notifications.

### API Gateway Rules

- Treat `api-gateway` as the future public backend entry point for customer, provider, and admin apps.
- Do not put domain business logic in the gateway.
- Gateway responsibilities should stay limited to routing, edge checks, auth pass-through/validation where required, rate limiting, and request shaping.

## Frontend Rules

- Keep each app under `apps/` independent.
- Put reusable Flutter UI in `packages/ui_kit`.
- Put reusable Dart API/error primitives in `packages/api_core`.
- Do not duplicate shared UI components across Flutter apps.
- Keep app-specific state/controllers/screens inside the app.
- The customer app auth flow is controller-driven through `CustomerAuthController`.
- Customer app stores sessions through `CustomerSessionStore`; do not bypass it for auth state.
- API responses should be parsed using `packages/api_core` envelope/error conventions.
- Maintain existing Figma-style widget conventions in `apps/customer/lib/shared/widgets/figma_customer_widgets.dart` unless replacing the design system intentionally.
- Profile setup in the customer app is partly frontend-only; add backend profile update APIs before assuming profile fields are persisted.

## Backend Rules

### Common Service Rules

- Start services with `shared/go/serviceapp.Run` unless there is a strong reason not to.
- Every service should expose `GET /health`, `GET /ready`, and `GET <api-base>/meta`.
- Use `shared/go/httpx` middleware for request IDs, panic recovery, and error envelopes.
- Use `shared/go/apperrors` for all API-facing errors.
- Keep handlers thin. Do not put business rules in HTTP handlers.
- Keep business workflows in `usecases`.
- Keep database code in `repositories`.
- Keep external provider integrations in `clients`.
- Add tests at the usecase/model level for business rules and handler level for contract behavior.

### API Response Rules

Success envelope:

```json
{ "success": true, "data": {} }
```

Error envelope:

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

### Customer Auth Rules

- Customer auth accepts exactly one identifier: `phone` or `email`.
- Phone numbers must normalize through `shared/go/phonenumber` to Nigerian `+234...` format.
- Emails must be trimmed and lowercased before challenge storage or customer lookup.
- OTP codes are numeric and default to six digits.
- OTP hashes bind the OTP secret, challenge ID, typed identifier key, and code.
- OTP challenges and rate limits are stored in customer Redis by typed identifier.
- Customer records may have phone, email, or both; at least one identifier is required.
- Refresh tokens are opaque values prefixed by refresh session ID and stored only as HMAC hashes.
- Refresh rotates sessions; old refresh sessions are revoked.
- Access tokens must include subject, role, service, session ID, token type, issued-at, and expiry claims.
- `onboarding_status == "profile_required"` means the app must continue profile setup before normal home.

### Notification Rules

- Notification requests require idempotency key, source service, event type, recipient type/id, and either template key or inline title/body.
- Supported channels are `push`, `email`, `websocket`, and `in_app`.
- HTTP notification endpoints require service HMAC auth.
- Email and push providers fall back to logging senders when provider config is missing.

### Payment/Wallet Rules

- Financial data must remain isolated in `payment-wallet-service`.
- Use kobo integer amounts for money.
- Ledger entries must use positive amounts and valid sides: `debit` or `credit`.
- Preserve idempotency for payment intents, withdrawals, refunds, and source-reference operations.
- Paystack webhook events must be processed idempotently.

### Media/File Rules

- Media uploads are internal-service-only.
- `owner_service` must match the authenticated service.
- Supported purposes include profile photo, document file, proof image, and signature.
- Media metadata must include owner, purpose, checksum, storage path, public URL, status, and timestamps.

## Database Structure

Service-owned migrations currently exist for:

- `services/customer-service/migrations`: `customers`, `customer_sessions`, `customer_auth_events`.
- `services/notification-service/migrations`: notification messages, deliveries, attempts, templates, preferences, devices.
- `services/payment-wallet-service/migrations`: wallet accounts, ledger transactions/entries, payment intents, Paystack events, provider bank accounts, withdrawals, refunds, idempotency keys.
- `services/media-file-service/migrations`: `media_assets`.

Docker Compose also provisions Postgres databases for scaffolded operational/platform services. Add schemas only in the owning service.

## Development Rules For AI

- Always inspect existing files before coding. Use `rg`/`find` to locate patterns.
- Prefer small, scoped changes over broad refactors.
- Do not rename services, folders, API bases, or shared package names unless explicitly requested.
- Do not move business logic across service boundaries.
- Do not edit generated/build artifacts such as `.next`, `node_modules`, Flutter generated plugin files, or platform build outputs unless specifically required.
- Do not overwrite unrelated dirty worktree changes.
- Do not add new dependencies if an existing local package/helper already solves the need.
- Do not invent a new architecture pattern when an existing service already shows the pattern.
- Update tests when changing business rules, API contracts, persistence, auth, payments, or notification behavior.
- Keep migrations service-local and backward-aware.
- Keep secrets and credentials out of committed docs/code. Existing local credential files should not be expanded or copied.
- Use ASCII in docs/code unless the file already requires non-ASCII.

## Workflow Instructions

Before implementing a feature:

1. Read `docs/architecture.md`.
2. Read this `CLAUDE.md`.
3. Identify the owning app/service/package.
4. Inspect existing implementation in the owning area.
5. Inspect relevant shared helpers in `shared/go` or `packages`.
6. Confirm API and persistence contracts.
7. Implement the smallest safe change.
8. Add or update focused tests.
9. Run formatting and relevant checks.

Preferred backend feature order:

1. Domain model and usecase tests.
2. Repository/client interfaces.
3. Migration and repository implementation.
4. HTTP DTOs, handlers, routes.
5. Config and `cmd/main.go` wiring.
6. Shared client updates if another service needs to call it.
7. Integration/handler tests.

Preferred frontend feature order:

1. Existing screen/controller/API inspection.
2. API model/client changes.
3. State/controller changes.
4. UI changes using existing shared widgets/style.
5. Widget/unit tests.
6. `flutter analyze` and `flutter test`.

Common checks:

```bash
go test ./...
flutter analyze
flutter test
npm run build
```

Run checks from the relevant module directory when a repo-wide command is not appropriate.

## Local Development

### With Docker

Run all local services and dependencies:

```bash
docker compose -f infra/docker-compose.yml up --build
```

### Without Docker

The services can also run directly on the host. In that mode, the developer must provide Postgres and Redis locally.

Required local tools:

- Go matching the service `go.mod` / `go.work`.
- Flutter for apps under `apps/`.
- Node/npm for `apps/admin`.
- Postgres for services with migrations.
- Redis for customer OTPs, notification streams, and other Redis-backed services.

Default local service config expects the same ports as Docker Compose. Either run local Postgres/Redis on those ports or override the service environment variables.

Default local databases:

- `customer_service`
- `taxi_service`
- `dispatch_delivery_service`
- `hauling_service`
- `payment_wallet_service`
- `notification_service`
- `support_dispute_service`
- `verification_compliance_service`
- `media_file_service`
- `admin_backoffice_service`
- `analytics_service`
- `vendor_service` (default local port `5445`; not yet provisioned by Docker Compose — `infra/docker-compose.yml` referenced above does not currently exist in this repository, see the vendor-service architecture session note below)

Default local Postgres credentials used by service fallbacks are usually:

```text
user: cosmicforge_logistics
password: cosmicforge_logistics
```

Run a Go service directly:

```bash
cd services/customer-service
go run ./cmd
```

Use the same pattern for other Go services:

```bash
cd services/notification-service && go run ./cmd
cd services/payment-wallet-service && go run ./cmd
cd services/media-file-service && go run ./cmd
```

Set environment variables before running a service when local ports or credentials differ from defaults. Prefer each service `.env.example` when present.

Examples:

```bash
export CUSTOMER_DATABASE_URL='postgres://cosmicforge_logistics:cosmicforge_logistics@localhost:5433/customer_service?sslmode=disable'
export CUSTOMER_REDIS_ADDR='localhost:6380'
cd services/customer-service
go run ./cmd
```

Run the customer app against a direct local customer service:

```bash
cd apps/customer
flutter pub get
flutter run --dart-define=CUSTOMER_API_BASE_URL=http://localhost:8101/api/v1/customer
```

Run the admin web app:

```bash
cd apps/admin
npm install
npm run dev
```

Important local ports:

- API gateway: `8080`
- Customer service: `8101`
- Driver taxi service: `8102`
- Dispatch delivery service: `8103`
- Hauling service: `8104`
- Payment-wallet service: `8105`
- Notification service: `8106`
- Support-dispute service: `8107`
- Verification-compliance service: `8108`
- Media-file service: `8109`
- Admin-backoffice service: `8110`
- Analytics service: `8111`
- Vendor service: `8112`

## Session Progress Log

### 2026-06-19 — Migration env-flag wiring

**Completed:**
- `media-file-service` — migration setup was already fully in place (`internal/database/migrate.go`, `config.Migration` via `MIGRATION` env bool, `if cfg.Migration` block in `cmd/main.go`). No changes needed.
- `notification-service` — was missing all three pieces. Added:
  - `services/notification-service/internal/database/migrate.go` (identical pattern to customer-service)
  - `Migration bool` field + `getEnvBool("MIGRATION", false)` in `internal/config/config.go`
  - `getEnvBool` helper function in config
  - Migration block (`if cfg.Migration { ... }`) in `cmd/main.go`
  - `logConnectivity` + `logSuccess/logError/logNotice/logFatal` helpers in `cmd/main.go`
  - Config log line at startup showing `migration=<bool>`

**Migration pattern (consistent across services):**
- Set `MIGRATION=true` in the service `.env` (or environment) to run migrations on startup.
- Default is `false` — server starts without running migrations.
- Migration reads all `.sql` files from `migrations/` in sorted order and applies them.

### 2026-06-19 — Direct profile photo upload (no proxy)

**Architecture:** Customer app uploads directly to media-file-service → gets public URL → sends URL to customer-service. No proxy through customer-service.

**Completed (customer-service Go):**
- `services/customer-service/internal/features/profile/usecases/profile_service.go` — added `SavePhotoURLInput` struct + `SaveProfilePhotoURL()` method
- `services/customer-service/internal/features/profile/http/handler.go` — added `SavePhotoURL` handler (accepts JSON `{photo_url, asset_id}`)
- `services/customer-service/internal/features/profile/http/routes.go` — registered `PUT /profile/photo-url` route

**Completed (Flutter customer app):**
- `apps/customer/pubspec.yaml` — added `image_picker: ^1.2.2`, `http_parser: ^4.1.2`
- `apps/customer/lib/core/config/customer_app_config.dart` — added `mediaFileApiBaseUrl` and `mediaFileServiceToken` fields (`--dart-define`)
- `apps/customer/lib/features/media/` — new reusable module:
  - `models/media_upload_result.dart` — `{id, url}` result model
  - `data/media_file_api.dart` — raw multipart HTTP client for media-file-service
  - `data/media_upload_service.dart` — `pickAndUpload(source, purpose, ownerId)` facade; call with different `purpose` for future upload types
- `apps/customer/lib/features/auth/state/customer_auth_controller.dart` — replaced `hasProfilePhoto: bool` with `profilePhotoUrl/profilePhotoAssetId`; added `uploadProfilePhoto(source)` method; `finishProfileSetup()` now async and calls `PUT /profile/photo-url`
- `apps/customer/lib/features/auth/data/customer_auth_api.dart` — added PUT method support + `saveProfilePhotoUrl()`
- `apps/customer/lib/features/profile_setup/ui/photo_upload_screen.dart` — real image picker + upload + `Image.network` preview
- `apps/customer/lib/features/profile_setup/ui/all_set_screen.dart` — `finishProfileSetup()` wrapped in lambda (now async)
- `apps/customer/lib/app/customer_app.dart` — wired `MediaFileApi` + `MediaUploadService` into `_buildController()`
- `apps/customer/test/widget_test.dart` — added `_FakeMediaUploadService` test double; all controller instantiations updated; mock handles `PUT /profile/photo-url`
- iOS `Info.plist` + Android `AndroidManifest.xml` — camera/photo permissions added

**To run the app with direct upload:**
```bash
flutter run \
  --dart-define=CUSTOMER_API_BASE_URL=http://localhost:8101/api/v1/customer \
  --dart-define=MEDIA_FILE_API_BASE_URL=http://localhost:8109/api/v1/media-files \
  --dart-define=MEDIA_FILE_SERVICE_TOKEN=development-media-token
```

**Media service token config** (`services/media-file-service/.env`):
```
MEDIA_FILE_SERVICE_TOKENS=customer-service=development-media-token
```

**Security note:** `MEDIA_FILE_SERVICE_TOKEN` via `--dart-define` is embedded in the binary. Acceptable for dev/internal MVP. For production, switch to signed upload URLs or customer JWT support on the media service.

**Future uploads** (documents, proof images): call `MediaUploadService.pickAndUpload()` with a different `purpose` string — no new code required.

### 2026-06-19 — Customer app UI redesign to match Figma

**Architecture changes:**
- `CustomerHomeScreen` converted to `StatefulWidget` with a 4-tab `BottomNavigationBar` (Home, Trips, Alerts, Profile).
- Profile tab embeds `CustomerProfileScreen` directly in the `IndexedStack` — no longer a Navigator push.
- `CustomerProfileScreen` restructured with a `Profile | Wallet` `TabBar` at top.
- `CustomerWalletScreen` gained `embedded: bool` param — hides AppBar when embedded in the Profile tab.
- `customer_app.dart` always initialises `_authApi`, `_supportApi`, `_walletApi` via `_initApis()` even when a test controller is injected, so `CustomerHomeScreen` always receives valid API instances.
- `CustomerAuthController._setAuthenticatedSession` restored to route `profile_required` sessions to `serviceChoice` (was accidentally commented out in previous session, breaking the flow test).

**Files changed:**
- `apps/customer/lib/features/home/ui/customer_home_screen.dart` — complete rewrite: `StatefulWidget`, `BottomNavigationBar` with 4 tabs, `_HomeTab` with map placeholder + service-selection bottom panel, `_TripsTab`, `_NotificationsTab` placeholders.
- `apps/customer/lib/features/profile/ui/customer_profile_screen.dart` — complete rewrite: `Profile | Wallet` `TabController`, centered avatar + name card, `_ProfileTab` with menu items (Saved Locations, Emergency Contact, Support, Notifications), `_ProfileEditScreen` with green header, `CustomerWalletScreen` embedded as second tab.
- `apps/customer/lib/features/wallet/ui/customer_wallet_screen.dart` — added `embedded` flag; AppBar hidden when embedded.
- `apps/customer/lib/app/customer_app.dart` — added `_initApis()` helper; always init APIs regardless of whether a custom controller is provided.
- `apps/customer/lib/features/auth/state/customer_auth_controller.dart` — restored `requiresProfile → serviceChoice` routing in `_setAuthenticatedSession`.
- `apps/customer/test/widget_test.dart` — updated assertions: `'Book a ride'` → `'Car Ride'`, `'Welcome, Ada Okafor'` → `'What do you want to do?'`.

**Home tab design (Figma-matched):**
- Map grid placeholder as full-screen background.
- Top bar: search pill + profile icon button.
- Bottom panel (white, rounded top corners): handle, "5% off first booking" banner, "What do you want to do?" heading, three selectable service rows (Car Ride / Bike Delivery / Truck Hauling), CTA button that changes label per selection.

**Profile tab design (Figma-matched):**
- `Profile | Wallet` pill-style tab switcher.
- Profile tab: centered avatar + name + "Customer" subtitle + phone, "Edit Profile" outlined button, menu list with chevron rows.
- Wallet tab: existing `CustomerWalletScreen` embedded without its own AppBar.
- Edit profile: green header with avatar, form fields (first name, last name, phone read-only, email read-only).

**To run:**
```bash
flutter run \
  --dart-define=CUSTOMER_API_BASE_URL=http://localhost:8101/api/v1/customer \
  --dart-define=MEDIA_FILE_API_BASE_URL=http://localhost:8109/api/v1/media-files \
  --dart-define=MEDIA_FILE_SERVICE_TOKEN=development-media-token \
  --dart-define=SUPPORT_API_BASE_URL=http://localhost:8107/api/v1/support-disputes \
  --dart-define=WALLET_API_BASE_URL=http://localhost:8105/api/v1/payment-wallet
```

### 2026-06-20 — Truck hauling booking system (full stack)

**Architecture:** Customer cannot book a truck unless at least one truck provider is online (Redis SCARD gate). Async goroutine matches bookings to providers via haversine proximity + Redis SETNX lock per provider.

**Booking state machine:** `pending_match` → `awaiting_acceptance` → `accepted` → `en_route_pickup` → `arrived_at_pickup` → `picked_up` → `en_route_delivery` → `delivered` → `completed` (terminals: `cancelled`, `unmatched`).

**Fare formula:** Base ₦5,000 + ₦250/km + 10% weight surcharge if >500 kg + ₦2,000/helper. All stored in kobo integers.

---

#### Backend — `services/driver-hauling-service`

**Module path:** `cosmicforge/logistics/services/hauling-service`

**Config env vars (prefix `HAULING_`):**
- `HAULING_HTTP_ADDR` (default `:8104`)
- `HAULING_DATABASE_URL`
- `HAULING_REDIS_ADDR` / `HAULING_REDIS_PASSWORD` / `HAULING_REDIS_DB`
- `HAULING_PROVIDER_TOKEN_SECRET` / `HAULING_PROVIDER_REFRESH_SECRET` / `HAULING_PROVIDER_OTP_SECRET`
- `HAULING_CUSTOMER_TOKEN_SECRET` — must match `CUSTOMER_TOKEN_SECRET` in customer-service
- `HAULING_BOOKING_MATCH_TIMEOUT` (seconds, default 25)
- `HAULING_PROVIDER_ONLINE_TTL` (seconds, default 90)
- `MIGRATION=true` to run migrations on startup

**Files created:**
- `services/driver-hauling-service/migrations/001_hauling_core.sql` — tables: `truck_providers`, `provider_sessions`, `trucks`, `haulage_bookings`, `booking_events`
- `services/driver-hauling-service/internal/config/config.go` — full config with all secrets + tuning params
- `services/driver-hauling-service/internal/database/postgres.go` — `NewPool()` (MaxConns=10)
- `services/driver-hauling-service/internal/database/migrate.go` — `ApplyMigrations()` sorted SQL files
- `services/driver-hauling-service/go.mod` — added gin, pgx/v5, redis, uuid, godotenv

**`provider_auth` feature** (`internal/features/provider_auth/`):
- Phone-only OTP auth (no email). Normalizes via `shared/go/phonenumber`.
- Token role=`truck_provider`, service=`hauling`.
- Redis key pattern: `hauling:provider:auth:otp:{type}:{value}`, rate: `hauling:provider:auth:otp-rate:{key}`
- Routes: `POST /provider/auth/start|verify|refresh|logout`, `GET /provider/me`

**`provider_profile` feature** (`internal/features/provider_profile/`):
- Profile CRUD + truck management (create/list/update).
- Valid truck types: `flatbed`, `container`, `tipper`, `van`, `refrigerated`.
- `CountActiveByProvider` is used by availability gate — provider must have ≥1 active truck to go online.
- `onboarding_status` flips `profile_required` → `complete` on first profile update.
- Routes: `GET/PUT /provider/profile`, `POST/GET /provider/trucks`, `GET/PUT /provider/trucks/:id`

**`provider_availability` feature** (`internal/features/provider_availability/`):
- Redis keys: online set `hauling:providers:online`, status `hauling:provider:status:{id}`, match lock `hauling:provider:matching:{id}`
- `SetOnline` requires `CountActiveByProvider > 0` (truck gate).
- `GetOnlineProviders` auto-cleans stale set entries (key expired but SMEMBERS still returns them).
- `AcquireMatchLock` = `SETNX` with TTL; `ReleaseMatchLock` = `DEL`.
- Customer endpoint `GET /customer/availability` returns `{available: bool, count: int64}`.
- Provider heartbeat `POST /provider/availability/heartbeat` refreshes lat/lng + TTL.
- Routes: Provider: `PUT /provider/availability`, `POST /provider/availability/heartbeat`, `GET /provider/availability`; Customer: `GET /customer/availability`

**`booking` feature** (`internal/features/booking/`):
- `CreateBooking`: checks `CountOnline > 0`, calculates fare, inserts booking, spawns `go matchBooking()`.
- `matchBooking` goroutine: sorts online providers by haversine distance → `AcquireMatchLock` → `MarkMatched` → waits `matchTimeout` → checks accepted → if not, releases lock → `ResetToMatching` → tries next → marks `unmatched` if all exhausted.
- `RejectBooking`: immediately releases Redis lock (no wait for timeout), re-triggers `go matchBooking()`.
- `ConfirmDelivery`: marks `delivered`, spawns goroutine to auto-complete after 30 minutes.
- `haversineKm()` uses Earth radius 6371 km.
- Public route: `POST /customer/bookings/estimate` (no auth).
- Customer routes (bearer, role=`customer`, service=`customer`): `POST/GET /customer/bookings`, `GET /customer/bookings/:id`, `PUT /customer/bookings/:id/cancel`
- Provider routes (bearer, role=`truck_provider`, service=`hauling`): `GET/GET /provider/bookings`, `PUT /provider/bookings/:id/accept|reject|pickup-confirmed|delivered|cancel`

**`cmd/main.go`:** Wires all repos/services/routes. Customer tokens verified with a second `TokenSigner` initialized from `cfg.CustomerTokenSecret`. Uses `shared/go/serviceapp.Run`.

**To run hauling-service:**
```bash
cd services/driver-hauling-service
MIGRATION=true go run ./cmd
```

---

#### Customer app — `apps/customer` (hauling feature)

**New files:**
- `lib/features/hauling/models/hauling_models.dart` — `HaulingBookingStatus` enum (with `isActive`, `isSearching`, `isTerminal`, `displayLabel`), `CargoType` enum, `FareEstimate`, `FareBreakdown`, `HaulageBooking`, `AvailabilityResult`
- `lib/features/hauling/data/hauling_api.dart` — HTTP client: `checkAvailability`, `estimateFare`, `createBooking`, `getBooking`, `listBookings`, `cancelBooking`
- `lib/features/hauling/state/hauling_booking_controller.dart` — `HaulingBookingController` (`ChangeNotifier`) with 9-state `HaulingFlowStatus` machine, 5-second polling for booking status updates, `_applyBookingUpdate()` auto-transitions states
- `lib/features/hauling/ui/hauling_flow_screen.dart` — single `HaulingFlowScreen` that switches between 9 sub-views: availability check → unavailable → details form (cargo type chips, weight slider, helper stepper) → confirm/fare breakdown → searching animation (pulsing truck icon) → active trip (step tracker) → delivered → completed → cancelled/unmatched

**Modified files:**
- `lib/core/config/customer_app_config.dart` — added `haulingApiBaseUrl` + `HAULING_API_BASE_URL` dart-define (default `http://192.168.1.138:8104/api/v1/hauling`)
- `lib/app/customer_app.dart` — imports `HaulingApi` + `HaulingBookingController`; `_initApis()` now creates both; `_haulingController` injected into `CustomerHomeScreen`
- `lib/features/home/ui/customer_home_screen.dart` — `CustomerHomeScreen` + `_HomeTab` accept `haulingController`; "Find a Truck" CTA pushes `HaulingFlowScreen` as full-screen dialog; Car/Bike show "Coming soon" snackbar

**To run customer app with hauling:**
```bash
cd apps/customer
flutter run \
  --dart-define=CUSTOMER_API_BASE_URL=http://localhost:8101/api/v1/customer \
  --dart-define=MEDIA_FILE_API_BASE_URL=http://localhost:8109/api/v1/media-files \
  --dart-define=MEDIA_FILE_SERVICE_TOKEN=development-media-token \
  --dart-define=SUPPORT_API_BASE_URL=http://localhost:8107/api/v1/support-disputes \
  --dart-define=WALLET_API_BASE_URL=http://localhost:8105/api/v1/payment-wallet \
  --dart-define=HAULING_API_BASE_URL=http://localhost:8104/api/v1/hauling
```

---

#### Truck provider app — `apps/truck_provider` (full build from scaffold)

**Added to `pubspec.yaml`:** `http: ^1.4.0`, `shared_preferences: ^2.5.3`

**New files created:**

`lib/main.dart` — replaced counter scaffold with `TruckProviderApp` entry point.

`lib/core/config/provider_app_config.dart` — single config field `haulingApiBaseUrl` (`HAULING_API_BASE_URL` dart-define).

`lib/features/auth/models/provider_auth_models.dart` — `ProviderSession`, `TruckProvider`, `OtpChallenge`, `ProviderBooking` (used by both auth and home features).

`lib/features/auth/data/provider_auth_api.dart` — HTTP client for `/provider/auth/start|verify|refresh|logout` and `GET /provider/me`.

`lib/features/auth/data/provider_session_store.dart` — `SharedPrefsProviderSessionStore` persists access token, refresh token, provider fields.

`lib/features/auth/state/provider_auth_controller.dart` — `ProviderAuthController` with `ProviderAuthStatus` (checking → phoneEntry → otpVerification → authenticated). `initialize()` tries to refresh saved session on startup.

`lib/features/home/data/provider_api.dart` — `ProviderApi` HTTP client: `setOnline`, `setOffline`, `heartbeat`, `listBookings`, `acceptBooking`, `rejectBooking`, `confirmPickup`, `confirmDelivery`.

`lib/features/home/state/provider_home_controller.dart` — `ProviderHomeController`:
- Online/offline toggle → `setOnline`/`setOffline` API calls.
- 30-second heartbeat `Timer.periodic` when online.
- 4-second booking poll: detects `awaiting_acceptance` → shows incoming request; detects active statuses → shows active trip screen.
- 30-second countdown `Timer.periodic` for incoming request; auto-rejects on expiry.
- `acceptBooking` / `rejectBooking` / `confirmPickup` / `confirmDelivery` — call API + transition state.

`lib/features/auth/ui/phone_entry_screen.dart` — phone number entry + submit.

`lib/features/auth/ui/otp_screen.dart` — 6-digit OTP input + verify. Shows `debugOtp` in dev.

`lib/features/home/ui/provider_home_screen.dart` — three screens rendered by `ProviderHomeStatus`:
- **Dashboard** (`_DashboardScreen`): green online/offline toggle card with switch, stats (total trips, rating), recent history list.
- **Incoming request** (`_IncomingRequestScreen`): circular countdown timer (30s), booking details card (pickup/dropoff, cargo type/weight, distance, fare), Accept (green) / Decline (red) buttons.
- **Active trip** (`_ActiveTripScreen`): status badge, route card, action button changes by status ("Confirm cargo picked up" or "Confirm delivery").

`lib/app/truck_provider_app.dart` — `TruckProviderApp` wires all controllers + APIs, routes on `ProviderAuthStatus`, splash screen.

**To run truck provider app:**
```bash
cd apps/truck_provider
flutter pub get
flutter run --dart-define=HAULING_API_BASE_URL=http://localhost:8104/api/v1/hauling
```

### 2026-06-21 — Widget decomposition, hauling .env, hauling bootstrap script

#### Widget decomposition — all four monolithic UI files split into per-view/per-widget files

**Strategy:**
- `views/` — full-screen state views (one per flow state)
- `tabs/` — tab content widgets embedded in a parent screen
- `screens/` — named push-navigation screens
- `widgets/` — shared components used across 2+ view files (public classes); components used in only one file stay private (`_` prefix) inside their new file
- Main screen files rewritten as thin `AnimatedBuilder`/`switch` routers importing the split files

**Customer app — hauling flow** (`apps/customer/lib/features/hauling/ui/`):
- `hauling_flow_screen.dart` 1,224 → 62 lines (thin router)
- `widgets/hauling_flow_helpers.dart` — `haulingFlowScaffold`, `haulingSectionLabel`, `haulingFareRow` (public free functions)
- `widgets/hauling_route_point.dart` — `HaulingRoutePoint` (shared by confirm + active-trip views)
- `views/hauling_availability_check_view.dart` — calls `controller.startHaulingFlow()` in `initState`
- `views/hauling_details_view.dart` — address fields, cargo type chips, weight slider, helper stepper
- `views/hauling_confirm_view.dart` — fare breakdown + confirm button
- `views/hauling_searching_view.dart` — pulsing truck icon animation
- `views/hauling_active_trip_view.dart` — step tracker
- `views/hauling_unavailable_view.dart`, `hauling_delivered_view.dart`, `hauling_completed_view.dart`, `hauling_cancelled_view.dart`, `hauling_error_view.dart`

**Customer app — home screen** (`apps/customer/lib/features/home/ui/`):
- `customer_home_screen.dart` 738 → 94 lines (thin router)
- `tabs/customer_home_tab.dart` — map placeholder + service selection panel
- `tabs/customer_trips_tab.dart`, `tabs/customer_notifications_tab.dart`
- `widgets/customer_home_bottom_nav.dart` — `CustomerHomeBottomNav`

**Customer app — profile screen** (`apps/customer/lib/features/profile/ui/`):
- `customer_profile_screen.dart` 1,029 → 252 lines (kept entry + tab scaffold)
- `customer_profile_tab.dart` — avatar card, menu list, logout/delete-account; exports `CustomerProfileAvatar`, `CustomerProfileMenuItem`, `CustomerProfileDivider`
- `customer_profile_edit_screen.dart` — full edit screen with photo upload, name fields, language dropdown

**Truck provider app — home screen** (`apps/truck_provider/lib/features/home/ui/`):
- `provider_home_screen.dart` 623 → 46 lines (thin router)
- `widgets/provider_app_colors.dart` — shared color constants (`kProviderGreen`, etc.)
- `widgets/provider_shared_widgets.dart` — `ProviderRequestRow`, `ProviderChipBadge`
- `screens/provider_dashboard_screen.dart` — online/offline toggle, stats, history
- `screens/provider_incoming_request_screen.dart` — 30s countdown, accept/reject
- `screens/provider_active_trip_screen.dart` — status badge, action button

**Post-decomposition analysis:** `flutter analyze` — customer app 0 errors, truck provider app 0 errors (1 pre-existing deprecated `activeColor` info, not introduced).

---

#### Hauling service local dev config

- `services/driver-hauling-service/.env` — active local config. Key values:
  - `HAULING_MIGRATION=true`
  - `HAULING_DATABASE_URL=postgres://cosmicforge_logistics:cosmicforge_logistics@localhost:5436/hauling_service?sslmode=disable`
  - `HAULING_REDIS_ADDR=localhost:6383`
  - `HAULING_CUSTOMER_TOKEN_SECRET=development-customer-access-token-secret` (must match customer-service `CUSTOMER_ACCESS_TOKEN_SECRET`)
  - `HAULING_OTP_DEBUG=true`
- `services/driver-hauling-service/.env.example` — committed template (`HAULING_MIGRATION=false`, `HAULING_OTP_DEBUG=false`)

---

#### Hauling local bootstrap script

- `scripts/hauling-local-bootstrap.sh` — follows same pattern as `customer-local-bootstrap.sh` and `notification-local-bootstrap.sh`
  - Starts Redis on port `6383` (daemonized, data dir `/tmp/redis-6383`)
  - Starts Postgres on port `5436` (data dir `/tmp/postgres-hauling`), initializes cluster if needed
  - Creates role `cosmicforge_logistics` and database `hauling_service` if missing
  - Applies `services/driver-hauling-service/migrations/001_hauling_core.sql`
  - All paths/ports overridable via env vars: `PG_PORT`, `REDIS_PORT`, `PG_DATA_DIR`, `REDIS_DATA_DIR`, `PG_BIN`, `REDIS_BIN`

**To bootstrap and run hauling service locally:**
```bash
bash scripts/hauling-local-bootstrap.sh
cd services/driver-hauling-service && go run ./cmd
```

### 2026-06-21 — Hauling dev seeder

**What is seeded (idempotent — safe to re-run):**

Providers (3) with `onboarding_status='complete'` and `status='active'` — log in via OTP using these phone numbers (requires `HAULING_OTP_DEBUG=true`):

| Phone           | Name           | Trucks                     |
|-----------------|----------------|---------------------------|
| `+2348011111001` | Emeka Okonkwo  | flatbed (10 t) + container (20 t) |
| `+2348011111002` | Biodun Adeyemi | tipper (8 t) + van (2 t) |
| `+2348011111003` | Chidi Eze      | refrigerated (5 t) + flatbed (12 t) |

Trucks (6): one active truck per entry above; providers can go online immediately after logging in.

Completed bookings (3): one per provider, with full audit trail (`booking_events`). Covers Lagos routes: Surulere→VI, Ikeja→Lekki, Apapa→Ogba. Fares match the v1 formula exactly.

**Files:**
- `services/driver-hauling-service/seeds/dev_seed.sql` — SQL with `ON CONFLICT DO NOTHING`; uses fixed UUIDs so re-runs are safe
- `services/driver-hauling-service/cmd/seed/main.go` — thin Go binary: reads `.env`, connects to DB, applies the SQL file, prints summary

**To seed (run from the service directory):**
```bash
cd services/driver-hauling-service
go run ./cmd/seed
```

Requires the database to be running and migrations already applied. Run the bootstrap script first if starting from scratch:
```bash
bash scripts/hauling-local-bootstrap.sh
cd services/driver-hauling-service && go run ./cmd/seed && go run ./cmd
```

### 2026-06-21 — Customer app: onboarding perms, places error surfacing, real home map

**Three reported issues fixed in `apps/customer`:**

1. **Onboarding location/notification "do nothing"** (`lib/features/onboarding/ui/onboarding_screen.dart`)
   - Root cause: when a permission is already granted/permanently-denied, `Permission.x.request()` returns instantly with no system dialog, so the button looked inert.
   - Fix: check `.status` first; if already granted, show a confirmation snackbar and advance; handle permanently-denied explicitly; added `_busy` flag + `isLoading` on the primary button; after `openAppSettings()` re-check status and advance if now granted. Added `_requestNotification()` helper.

2. **No location autocomplete suggestions** (`lib/features/hauling/data/places_api.dart` + `views/hauling_location_entry_view.dart`)
   - Root cause class: errors were silently swallowed (`catch (_)` + `if (status != 200) return []`), so a `REQUEST_DENIED`/billing/quota failure showed as "no results". Note: the default key in `customer_app_config.dart` works server-side (verified via curl — autocomplete + details both return `OK`), so on-device failures point to Google Cloud key restrictions (Places API not enabled / app restriction blocking the Web Service).
   - Fix: `PlacesApi.autocomplete` now throws `PlacesApiException(status, message)` on non-OK; `getLatLng` logs + returns null. The location entry view catches it and renders a red inline error (`_suggestionsError`) with friendly text per status (REQUEST_DENIED, OVER_QUERY_LIMIT, NETWORK_ERROR, MISSING_KEY).

3. **Home screen real map to match mockup** (`lib/features/home/ui/tabs/customer_home_tab.dart` + new `widgets/customer_home_map.dart`)
   - Replaced the painted grid `_MapPlaceholder`/`_MapGridPainter` (deleted) with a live `google_maps_flutter` `GoogleMap` centered on Lagos, 12 deterministic scattered vehicle markers, `myLocationEnabled` once location is granted.
   - Added a custom **green** "locate me" FAB (`_LocateButton`) floating above the service panel (native button disabled). Uses new `geolocator` dep to `getCurrentPosition` + `animateCamera`; requests permission on tap; snackbars on denied/service-off/failure.
   - Added `geolocator` to `pubspec.yaml` (iOS `NSLocationWhenInUse...` and Android `ACCESS_FINE/COARSE_LOCATION` already present).

**Verification:** `flutter analyze` — no new errors/warnings in changed files (only pre-existing infos elsewhere). `flutter test` — all 13 tests pass.

### 2026-06-21 — Hauling flow redesign (Figma UX reorder + package info + reviews)

**What changed:** The customer app's truck hauling booking flow was reordered to match the Figma mockup. Previously the flow started with location entry; now it follows: Details → Package Info → Location → Tier → Payment → Search → Trip → Review.

**Backend changes (`services/driver-hauling-service`):**

- `migrations/004_package_info_and_reviews.sql` — adds 6 columns to `haulage_bookings` (`weight_category`, `receiver_name`, `receiver_phone`, `package_content`, `package_size`, `is_fragile`); creates `booking_reviews` table (rating 1-5, review_text, recommends_driver, booking/customer/provider FKs).
- `booking/models/booking.go` — 6 new fields on `Booking`/`PublicBooking`; new `BookingReview`/`PublicBookingReview` types with `Public()` method.
- `booking/repositories/booking_repository.go` — updated `bookingSelectCols`, `scanBooking`, `Create` INSERT for 6 new columns; added `CreateReview`/`GetReviewByBooking` methods.
- `booking/http/dto.go` — 6 new fields on `createBookingRequest`; new `submitReviewRequest` DTO.
- `booking/usecases/booking_service.go` — 6 new fields on `CreateBookingInput`; new `SubmitReviewInput` + `SubmitReview()` method (validates rating 1-5, checks booking ownership + delivered/completed status, creates review, auto-completes booking if still delivered).
- `booking/http/handler.go` — updated `CreateBooking` to pass 6 new fields; new `SubmitReview` handler.
- `booking/http/routes.go` — new `POST /customer/bookings/:id/review` route (customer bearer auth).

**Flutter changes (`apps/customer`):**

- `hauling_models.dart` — 6 new fields on `HaulageBooking`; new `BookingReview` model.
- `hauling_api.dart` — 6 new params on `createBooking()`; new `submitReview()` method.
- `hauling_booking_controller.dart` — `HaulingFlowStatus` enum reordered: `idle → details → packageInfo → locationEntry → checkingAvailability → unavailable → tierSelection → payment → paymentProcessing → searching → activeTrip → delivered → review → completed → cancelled → error`. Removed `confirm` status. Added state fields: `receiverName`, `receiverPhone`, `packageContent`, `packageSize`, `isFragile`, `reviewRating`, `reviewText`, `recommendsDriver`. New methods: `proceedFromDetailsToPackageInfo()`, `proceedFromPackageInfoToLocation()`, `proceedFromTierToPayment()`, `backToPackageInfo()`, `submitReview()`, `skipReview()`, setters for all new fields. `_applyBookingUpdate`: `delivered` → `review` (not `delivered` view). `startHaulingFlow()` → `details` (not `locationEntry`).
- New `hauling_package_info_view.dart` — receiver name/phone fields, package content/size fields, fragile toggle.
- New `hauling_review_view.dart` — "You have arrived!" screen, driver card, 5-star rating, review text, recommend driver pills, submit/skip.
- `hauling_flow_screen.dart` — router updated: `idle`/`details` → details view, `packageInfo` → package info view, `delivered`/`review` → review view. Removed `confirm`/`availability_check`/`delivered` view imports.
- `hauling_details_view.dart` — back goes to `Navigator.pop()` (first screen), continue → `proceedFromDetailsToPackageInfo`.
- `hauling_location_entry_view.dart` — back → `backToPackageInfo()`.
- `hauling_tier_selection_view.dart` — "Select Truck" → `proceedFromTierToPayment()`.

**Bootstrap:** `scripts/hauling-local-bootstrap.sh` — added `004_package_info_and_reviews.sql`.

**Verification:** `go build ./...` — clean. `flutter analyze` — 0 errors. `flutter test` — all 17 tests pass.

### 2026-06-21 — Hauling location entry UI polish

**File:** `apps/customer/lib/features/hauling/ui/views/hauling_location_entry_view.dart`

**What changed:** Replaced the two separate `_LocationField` rows (each with a plain 10×10 colored dot) with a single grouped `_LocationInputCard` widget that matches the Figma mockup style.

- Pickup indicator: `Icons.radio_button_checked` (green `CustomerFigmaColors.primary`).
- Dropoff indicator: `Icons.location_on` (orange).
- A `CustomPaint` dotted vertical line (`_DottedLinePainter`) connects the two icons in the left icon column.
- Both text fields share one card container with a `Divider` between them and no visible border on the inputs themselves — border is on the outer card.
- Loading spinners replace each icon while the Places API resolves lat/lng for the selected suggestion.
- The `_LocationField` class was deleted; replaced entirely by `_LocationInputCard` + `_DottedLinePainter`.

### 2026-06-22 — Truck provider home dashboard rebuild (Figma-matched)

**What changed:** Completely rebuilt the truck provider app's home screens to match the new Figma mockups (screens 2286-2292). The old single-screen design (online toggle + stats + list) was replaced with a 5-tab bottom navigation shell with dedicated Earnings dashboard, Requests list, and full request detail + assign-truck flow.

**Architecture changes:**

- `ProviderHomeStatus` simplified: `dashboard` + `activeTrip` (removed `incomingRequest` and `completing`).
- `ProviderHomeState` replaces `incomingBooking`/`requestExpiresAt` with `pendingRequests: List<ProviderBooking>` so multiple concurrent requests are visible.
- `acceptBooking(bookingId)` and `rejectBooking(bookingId)` now take a booking ID parameter (previously read from single `incomingBooking`).
- Added `trucks`, `trucksLoading`, `selectedTruckId` fields to state.
- Added `loadTrucks()` and `setSelectedTruck()` methods.
- Removed countdown timer (30s countdown was per-booking; backend handles unmatching after timeout).

**New/changed files:**

- `apps/truck_provider/lib/features/auth/models/provider_auth_models.dart` — added `ProviderTruck` model + `shortId` getter on `ProviderBooking`.
- `apps/truck_provider/lib/features/home/data/provider_api.dart` — added `listTrucks()` (`GET /provider/trucks`).
- `apps/truck_provider/lib/features/home/state/provider_home_controller.dart` — rewritten with new state shape.
- `apps/truck_provider/lib/features/home/ui/provider_home_screen.dart` — rewritten as `StatefulWidget` with `BottomNavigationBar` (5 tabs: Home, Requests, Calendar, Card, Profile). Active trip still takes over the full screen.
- `apps/truck_provider/lib/features/home/ui/screens/provider_dashboard_screen.dart` — rewritten: `SliverAppBar` with dark green gradient + "Earnings" header + online/offline toggle, balance card (Available/Pending/Today's Earnings/Withdraw), stats row (Total Trips, Pending, Rating), "Recent Request" list using `ProviderRequestCard`.
- `apps/truck_provider/lib/features/home/ui/screens/provider_requests_screen.dart` — new, replaces `provider_incoming_request_screen.dart`; full list of pending bookings.
- `apps/truck_provider/lib/features/home/ui/screens/provider_request_detail_screen.dart` — new; shows customer avatar, trip reference/date, quick stats (distance/time/fee), route card, truck haul info, Reject + Assign Driver buttons.
- `apps/truck_provider/lib/features/home/ui/screens/provider_assign_truck_screen.dart` — new; lists provider's active trucks with radio selection; Confirm → `acceptBooking()` → success screen ("Trip Assigned!").
- `apps/truck_provider/lib/features/home/ui/widgets/provider_request_card.dart` — new shared widget; shows avatar, booking ID, distance/time/fare chips, dotted route connector, Reject + Assign Driver buttons.
- Deleted: `provider_incoming_request_screen.dart` (replaced by requests screen + request detail).

**Request flow (new):**

1. Provider goes online from the Home tab → polls every 4s.
2. `awaiting_acceptance` bookings appear as cards on the Home tab ("Recent Request") and Requests tab.
3. Tapping "Assign Driver" → Request Detail screen → Assign Truck screen (loads provider trucks).
4. Provider selects a truck → Confirm → `acceptBooking(bookingId)` → "Trip Assigned!" success screen.
5. Tapping "Reject" from any screen → `rejectBooking(bookingId)`.
6. Once accepted, active trip takes over full screen (existing `ProviderActiveTripScreen` unchanged).

**Verification:** `flutter analyze` — 0 issues. Active trip screen unchanged.

**To run truck provider app:**
```bash
cd apps/truck_provider
flutter run --dart-define=HAULING_API_BASE_URL=http://localhost:8104/api/v1/hauling
```

### 2026-06-22 — Truck provider UI rebuild (map-based, Figma-matched)

**What changed:** Completely rebuilt the truck provider home screens to match the actual Figma mockups (screens 2034–2049, 2119). The previous earnings-dashboard design was replaced with a map-first, bottom-sheet-driven UX.

**Architecture:**
- Home tab = full-screen live `GoogleMap` (same `google_maps_flutter` setup as customer app), with online/offline pill button overlaid top-left and notification bell top-right.
- Offline state: white modal card overlay ("You are Offline!") with "Go Online" button.
- Incoming request: white bottom sheet over map ("Incoming Requests..."), shows first pending request card with direct Accept/Reject buttons.
- Active trip: full-screen stack (map + bottom sheet) with 3 status phases:
  - En route to pickup: "Arriving at pick up" heading, Start Trip disabled.
  - Arrived at pickup: "Arrived at Pickup", Start Trip enabled → calls `confirmPickup`.
  - Trip in progress: "Trip has started..." + progress bar + "End Trip" red pill → navigates to inline completion screen.
- Completion screen: embedded in active trip widget (no Navigator push); shows "You have arrived" + green banner + customer summary + Truck Haul Info + photo-capture + signature pad + "Confirm" button → calls `confirmDelivery`.
- Request Detail (screen 2045): direct Accept/Reject buttons, no assign-truck flow.
- Requests tab (screen 2049): list of pending request cards with direct Accept/Reject.
- Notifications screen (screen 2119): grouped by day, pushed from bell icon.

**Files changed/created:**
- `lib/features/home/ui/widgets/provider_home_map.dart` — new reusable live map widget (vehicle markers, permission-aware, optional route polyline)
- `lib/features/home/ui/screens/provider_dashboard_screen.dart` — rewritten: map home tab with online/offline pill, bell, offline modal, incoming request sheet
- `lib/features/home/ui/widgets/provider_request_card.dart` — renamed `onAssign→onAccept`; `AssignDriverButton` replaced by `AcceptRequestButton` ("Accept Request" green)
- `lib/features/home/ui/screens/provider_requests_screen.dart` — updated: direct accept/reject buttons, card tap → request detail
- `lib/features/home/ui/screens/provider_request_detail_screen.dart` — removed assign-truck nav; Accept/Reject buttons call `acceptBooking`/`rejectBooking` directly
- `lib/features/home/ui/screens/provider_active_trip_screen.dart` — rewritten: map + bottom sheet, 3-phase status UI, inline `_CompletionScreen` with photo + `CustomPaint` signature pad
- `lib/features/home/ui/screens/provider_notifications_screen.dart` — new: grouped notification list (Figma 2119)
- `lib/features/home/ui/provider_home_screen.dart` — updated: notifications pushed from bell, badge count on Requests tab
- `lib/features/home/state/provider_home_controller.dart` — added `cancelActiveTrip()` method
- `lib/features/home/data/provider_api.dart` — added `cancelActiveTrip()` (`PUT /provider/bookings/:id/cancel`)
- **Deleted:** `lib/features/home/ui/screens/provider_assign_truck_screen.dart`

**Verification:** `flutter analyze` — 0 issues. `flutter build apk --debug` — builds successfully.

### 2026-06-22 — Truck provider Profile section (full stack)

**What changed:** Built the entire Profile section of the truck provider app to match the Figma mockups (Profile main, Profile Info edit, Change Phone, Verification & Documents, Face Verification, Truck Information view/edit, Support/Live Chat), and wired it to the hauling-service backend.

**Backend (`services/driver-hauling-service`):**

- `migrations/005_provider_profile_extras.sql` — `truck_providers`: `language`, `driver_license_number`, `license_expiry_year`, `license_expiry_date`. `trucks`: `license_type`, `number_of_axles`, `years_of_experience`, `goods_types TEXT[]`, `has_insurance`.
- `provider_auth/models/provider.go` — `Provider`/`PublicProvider` extended with location/language/service/operation_mode/license/doc-url fields + `created_at`. `GET /provider/profile` (and verify/refresh) now return the full profile.
- `provider_auth/repositories/postgres_provider_repository.go` — extended SELECT/RETURNING columns + `scanProvider`; new `UpdatePhone(id, phone)` repo method (added to `ProviderRepository` interface).
- `provider_profile/models/models.go` — `Truck`/`PublicTruck` extended; `ValidTruckTypes` widened to a superset (adds pickup/box/tanker/trailer/dump/lowbed/crane/other alongside the original slugs).
- `provider_profile/repositories/postgres_profile_repository.go` — rewritten with shared `providerColumns`/`truckColumns`; `UpdateProfile` now persists language/license fields **and** uses `CASE WHEN $x != '' THEN $x ELSE col END` preserve-on-empty semantics for location/operation/service/language/license/guarantor/emergency so partial profile updates don't wipe untouched fields; truck create/update carry the new columns; `UpdatePhone` delegator.
- `provider_profile` http/usecases — DTO/input/handler carry the new profile + truck fields; `normalizeGoods` helper.
- `provider_auth` usecases/http — `ChangePhoneStart`/`ChangePhoneVerify` (reuse OTP infra; bearer-protected; unique-violation → friendly validation error). Routes: `POST /provider/phone/change/start|verify`.

**Frontend (`apps/truck_provider`):** new `lib/features/profile/`:
- `data/provider_profile_api.dart` — getProfile, updateProfile (partial-safe), changePhoneStart/Verify, listTrucks, create/updateTruck.
- `state/provider_profile_controller.dart` — `ProviderProfileController` (load profile+trucks, save profile info, photo, verification, truck; phone-change flow). Pushes name/photo/phone updates back into the auth session via new `ProviderAuthController.applyProviderUpdate`.
- `ui/` — `provider_profile_screen.dart` (2110, replaces the old `_ProfileTab` in the home shell), `provider_profile_info_screen.dart` (2111/2112), `provider_change_phone_screen.dart` (2113 + OTP), `provider_verification_screen.dart` (2133-2141, real document uploads via media-file-service), `provider_face_verification_screen.dart` (2142/2143 + submitted state — visual KYC gate), `provider_truck_info_screen.dart` (2162 + 2163 dialog), `provider_truck_edit_screen.dart` (2164), `provider_support_screen.dart` (Account option-4/5 — live chat UI is local/canned), `provider_coming_soon_screen.dart` (Safety/Security/Privacy/Payments — no mockups yet), `widgets/provider_profile_widgets.dart` (header, fields, primary button, confirm dialog).
- `auth/models/provider_auth_models.dart` — `TruckProvider` + `ProviderTruck` extended; `truckTypeLabel()` + `providerTruckTypeOptions`.

**Notes / scope:** Face Verification scan and Live Chat are faithful UIs without a dedicated backend (no face-match service; real-time chat needs support-dispute-service). The confirmation dialogs use icon-based headers rather than the Figma 3D illustration (asset not bundled). Truck "Capacity" is kept as numeric kg (matching the booking/matching logic) under the Figma label.

**Verification:** backend `go build ./...` + `go vet` + `go test ./...` clean. `flutter analyze` — No issues found.

**Run:**
```bash
# Backend
bash scripts/hauling-local-bootstrap.sh   # applies migrations 001–005
cd services/driver-hauling-service && go run ./cmd/seed  # optional: seed dev providers
cd services/driver-hauling-service && go run ./cmd

# Truck provider app
cd apps/truck_provider
flutter run \
  --dart-define=HAULING_API_BASE_URL=http://localhost:8104/api/v1/hauling \
  --dart-define=MEDIA_FILE_API_BASE_URL=http://localhost:8109/api/v1/media-files \
  --dart-define=MEDIA_FILE_SERVICE_TOKEN=development-media-token
```

`MEDIA_FILE_SERVICE_TOKENS=truck-provider=development-media-token` must be set in `services/media-file-service/.env` for document uploads to succeed.

### 2026-06-22 — Customer app: booking flow fixes + bottom nav icons

**Five issues fixed in `apps/customer` truck hauling flow:**

**1. Long connection time** — added 90-second client-side `Timer` in `HaulingBookingController` (`_startSearchTimeout` / `_stopSearchTimeout`). If the booking is still in `searching` state after 90 s, it auto-cancels with the message "No trucks found nearby. Please try again later." Timer starts when polling starts and is cancelled on any terminal state.

**2. Cancel not working** — the cancel button in `hauling_searching_view.dart` was missing error surfacing. Added a red bordered error container above the Cancel button that shows `state.error`. Cancel button is disabled while `state.isLoading` and shows a spinner. A confirmation bottom sheet (`_confirmCancel`) gates the action.

**3. Paystack broken** — removed Paystack entirely. `hauling_payment_view.dart` now offers Wallet Balance and Cash on Delivery only. "Confirm & Find Truck" calls `confirmPayment()` directly (appropriate for MVP). No `url_launcher` usage remains.

**4. Book a Truck / Choose a Truck UI** — redesigned to match Figma mockups (screens 1733-1735):
- `hauling_location_entry_view.dart` — discount banner redesigned (white card + green checkmark), "Pick-up" / "Drop off (optional)" labels inside `_LocationInputCard`, title 20px, subtitle updated.
- `hauling_tier_selection_view.dart` — full rewrite: route summary row at top (green/orange dots + dashed `CustomPainter` line + pickup/dropoff addresses), `_TierCard` widget with truck PNG (`assets/figma/delivery truck back side view.png`), tier name, fare from background estimate, ⏱ 2 min ETA, "Popular" dark badge on first tier (index 0), radio button, selected card has tinted background + primary border.
- Flow order changed: `startHaulingFlow()` now emits `locationEntry` (first screen = location entry, not details form).
- `checkAvailabilityAndProceed()` fires `_fetchPreviewFare()` after transitioning to `tierSelection` — background estimate (100 kg, 0 helpers) pre-populates tier card prices.

**5. Bottom nav icons** — replaced Material `Icon` widgets with real Figma asset icons in `customer_home_bottom_nav.dart`:
- Home: `assets/figma/House_01.svg` (SvgPicture)
- Trips: `assets/figma/Group 1000004752.svg` (SvgPicture)
- Alerts: `assets/figma/notification_bell.png` (Image.asset with color tint)
- Profile: `assets/figma/user.svg` (SvgPicture)
- Added `flutter_svg: ^2.0.10` to `pubspec.yaml`.
- Selected tab: green pill with white icon + label. Unselected: muted-color icon only.

**Files changed:**
- `apps/customer/pubspec.yaml` — `flutter_svg: ^2.0.10`
- `apps/customer/lib/features/home/ui/widgets/customer_home_bottom_nav.dart` — asset icons
- `apps/customer/lib/features/hauling/state/hauling_booking_controller.dart` — search timeout, flow order fix, preview fare fetch
- `apps/customer/lib/features/hauling/ui/hauling_flow_screen.dart` — router updated
- `apps/customer/lib/features/hauling/ui/views/hauling_location_entry_view.dart` — UI polish
- `apps/customer/lib/features/hauling/ui/views/hauling_tier_selection_view.dart` — full rewrite
- `apps/customer/lib/features/hauling/ui/views/hauling_details_view.dart` — back → `backToTierSelection`
- `apps/customer/lib/features/hauling/ui/views/hauling_package_info_view.dart` — continue → `proceedFromPackageInfoToPayment`
- `apps/customer/lib/features/hauling/ui/views/hauling_payment_view.dart` — Paystack removed, Cash on Delivery added
- `apps/customer/lib/features/hauling/ui/views/hauling_searching_view.dart` — error surfacing, cancel button fixes

**Verification:** `flutter analyze` — 0 errors (12 pre-existing infos in unrelated files).

**To run customer app:**
```bash
cd apps/customer
flutter run \
  --dart-define=CUSTOMER_API_BASE_URL=http://localhost:8101/api/v1/customer \
  --dart-define=MEDIA_FILE_API_BASE_URL=http://localhost:8109/api/v1/media-files \
  --dart-define=MEDIA_FILE_SERVICE_TOKEN=development-media-token \
  --dart-define=SUPPORT_API_BASE_URL=http://localhost:8107/api/v1/support-disputes \
  --dart-define=WALLET_API_BASE_URL=http://localhost:8105/api/v1/payment-wallet \
  --dart-define=HAULING_API_BASE_URL=http://localhost:8104/api/v1/hauling
```

### 2026-06-22 — Customer app: Paystack online payment for truck booking

**What changed:** Added a "Card or Bank Transfer" (Paystack) option to the truck-hauling payment screen. Frontend-only — reuses the existing, working wallet top-up endpoint (`POST /topups` → Paystack checkout → `POST /topups/:reference/verify`). No backend changes.

**Architecture decision:** `payment-wallet-service` exposes Paystack to customers **only** through wallet top-ups; the `payment-intents` / `pay-from-wallet` endpoints are internal (service HMAC auth) and the hauling service is not wired to them. Booking creation in `driver-hauling-service` does **not** debit/escrow anything today (`payment_method` is not even sent to the booking API). So "pay with Paystack" = charge the fare into the customer wallet via the real Paystack checkout, then create the booking. Money is real and lands in the customer's Karry Go wallet.

**Flow:** Payment screen → select "Card or Bank Transfer" → "Pay with Paystack" → `WalletApi.createTopUp(fareKobo)` → in-app `webview_flutter` checkout (`HaulingPaystackCheckoutView`, same return-URL detection as `WalletCheckoutView`) → on return → `verifyTopUp` (4 retries) → create booking → searching. Wallet/Cash paths unchanged (create booking directly).

**Files changed (`apps/customer`):**
- `lib/features/hauling/state/hauling_booking_controller.dart` — added `paystackCheckout` to `HaulingFlowStatus`; added `topUpReference` state field (cleared via existing `clearPaystackUrl`); `confirmPayment()` branches to `_startPaystackCheckout()` when method is `paystack`; new `_startPaystackCheckout()` (calls `createTopUp` with fare + profile email, guards empty fare/email), `onPaystackCheckoutReturned()` (verify + create booking), `cancelPaystackCheckout()`, `_customerEmail()` helper (prefers `profileEmail`, falls back to `email`).
- `lib/features/hauling/ui/views/hauling_paystack_checkout_view.dart` — **new**; WebView checkout host modeled on `wallet/ui/funding/wallet_checkout_view.dart`.
- `lib/features/hauling/ui/hauling_flow_screen.dart` — route `paystackCheckout` → `HaulingPaystackCheckoutView`.
- `lib/features/hauling/ui/views/hauling_payment_view.dart` — new "Card or Bank Transfer" payment row; button label becomes "Pay with Paystack" when selected. (`_canConfirm` already permits any non-wallet method.)

**Reused (no change):** `WalletApi.createTopUp` / `verifyTopUp` (`lib/features/wallet/data/wallet_api.dart`); backend `POST /topups`, `POST /topups/:reference/verify` (customer bearer auth).

**Edge case:** customers without a profile email get an inline error ("Add an email to your profile to pay with card or transfer.") — Paystack requires a valid email.

**Follow-up for true escrow** — DONE, see the next entry. The wallet top-up hack described above was superseded by the server-driven `card-payment` escrow endpoint; the customer app now creates the booking with a `payment_method` and (for card) opens an up-front Paystack intent bound to the booking.

**Verification:** `flutter analyze` — 0 new issues (12 pre-existing infos/warnings unchanged). `flutter test` — all 25 tests pass.

### 2026-06-22 — Hauling escrow: verified wiring + fixed two config bugs

**Context:** True escrow (charge-on-acceptance) was already implemented in the working tree from the earlier "server-driven payment" session (backend `PaymentClient` + `booking/payments/wallet_payment_client.go` over `shared/go/walletclient`, `migrations/006_booking_payment.sql`, `POST /customer/bookings/:id/card-payment`, customer app `initiateCardPayment` + dispatch-before-payment). This pass **verified the whole chain and fixed two config bugs that would have broken it silently at runtime.** No new escrow logic was needed.

**Escrow flow (confirmed wired across every lifecycle path in `booking_service.go`):**
- Booking create → `payment_method` (`wallet|card|cash`), `payment_status=unpaid`.
- Provider accept → `ensurePaymentSecured`: wallet → `HoldFromWallet` (CreatePaymentIntent + PayFromWallet); card → must already be paid up-front (else reject accept); cash → no-op. Blocks `accepted` if the charge fails. Sets `payment_status=held`.
- Auto-complete worker + review-driven completion → `settleOnComplete` → `CompleteJob` (release escrow to provider), `payment_status=paid`.
- Customer cancel, provider cancel, unmatched → `refundIfHeld` → `RequestRefund`, `payment_status=unpaid`.
- `s.payments == nil` (no `HAULING_PAYMENT_URL`/`SECRET`) → every step is a no-op so local dev runs without payment-wallet.

**Bug 1 — doubled base URL (would 404 every internal call).** `shared/go/walletclient` appends the full `/api/v1/payment-wallet/internal/...` path to `Client.BaseURL`, so `BaseURL` must be a **bare origin**. Both `services/driver-hauling-service/.env` and `.env.example` had `HAULING_PAYMENT_URL=http://localhost:8105/api/v1/payment-wallet` → fixed to `http://localhost:8105` (added the "bare origin only" comment, matching the existing `HAULING_NOTIFICATION_URL`).

**Bug 2 — HMAC service-name + secret mismatch (would 403 every internal call).** The hauling payment client signs as `driver-hauling-service` with `HAULING_PAYMENT_SECRET=development-hauling-payment-secret` (its canonical identity — same name notification-service already trusts). But `payment-wallet-service` trusted `hauling-service=development-payment-wallet-service-secret`. Aligned payment-wallet to `driver-hauling-service=development-hauling-payment-secret` in both `services/payment-wallet-service/.env` and the `PAYMENT_WALLET_SERVICE_SECRETS` default in `internal/config/config.go` (matches notification-service's per-caller-secret convention).

**Verification:** hauling + payment-wallet `go build ./... && go vet ./... && go test ./...` clean (matching + notification unit tests pass). Customer app `flutter analyze` — only 1 pre-existing unrelated warning; `flutter test` — all 25 pass.

**To run with escrow active:**
```bash
bash scripts/hauling-local-bootstrap.sh        # applies migrations 001–006
cd services/payment-wallet-service && go run ./cmd   # port 8105
cd services/driver-hauling-service && go run ./cmd   # reads .env: HAULING_PAYMENT_URL=http://localhost:8105
```
Escrow only activates when `HAULING_PAYMENT_URL` + `HAULING_PAYMENT_SECRET` are set; otherwise bookings settle without charging (local dev). Card payment additionally needs the customer to have a profile email (Paystack requirement).

### 2026-06-22 — Hauling booking flow: matching correctness, server-driven payment, customer realtime

**What changed:** Reviewed the customer hauling flow end-to-end and implemented fixes across three tiers. Replaced the client-side "top-up wallet as a proxy for paying" hack with a **server-driven payment model** bound to the booking, fixed real matching-engine eligibility/concurrency bugs, and gave the customer a realtime channel + live driver location.

**Decisions (with user):** all tiers; **charge-on-acceptance**; **card pays up-front** (Paystack intent at booking, completed in the WebView while searching, refunded if unmatched/cancelled); **wallet holds on provider acceptance**; **cash is record-only**; settle to provider via `CompleteJob` on completion.

#### Backend — `services/driver-hauling-service`

- **Matching engine** (`booking/usecases/booking_service.go`): `matchBooking` now loads the booking, filters online providers by a **serviceable radius** (`HAULING_MATCH_MAX_RADIUS_KM`, default 25) and by **truck type + capacity** (via a new `TruckLookup` interface adapted over the truck repo in `cmd/main.go`), and only then dispatches nearest-first. Lock lifecycle fixed: per-provider lock TTL is `matchTimeout + 15s`, released only **after** the accept/timeout decision (kept on accept); `MarkMatched` is now a guarded compare-and-set from `pending_match` only (`booking/repositories`). `RejectBooking` cancels the in-flight match goroutine (`cancelMatch`) before re-dispatching, so only one matcher runs per booking. Availability gate now uses `GetOnlineProviders` (prunes stale entries) instead of raw `CountOnline`, in both `CreateBooking` and `CheckAvailability`.
- **Payment binding** (`PaymentClient` interface + `booking/payments/wallet_payment_client.go` adapter over `shared/go/walletclient`): `AcceptBooking` secures payment (wallet → `PayFromWallet` hold; card must already be held/paid; cash record-only) and **blocks `accepted` if the charge fails**. Completion (`autoCompleteDelivered` + `SubmitReview`) calls `Settle` (`CompleteJob`); cancel/unmatched call `Refund` (`RequestRefund`). New `InitiateCardPayment` usecase + `POST /customer/bookings/:id/card-payment` route returns the Paystack authorization URL. `payment_intent_id` stores the wallet payment **reference** (refunds/lookups resolve by reference). `payments == nil` (no `HAULING_PAYMENT_URL`) disables payment for local dev.
- **Coordinate validation**: `CreateBooking` rejects null-island/out-of-range pickup & dropoff coords.
- **Migration** `006_booking_payment.sql` — `payment_method` (`wallet|card|cash`, default `wallet`), `payment_status` (`unpaid|held|paid|failed`); wired into bootstrap script.
- **Customer realtime**: notification handler parameterized by recipient type (`NewHandlerFor`); added a **customer** notification group (`/customer/notifications/...`, customer bearer) alongside the provider one. New `GET /customer/bookings/:id/location` returns the assigned provider's heartbeat lat/lng.
- **Tests**: `booking/usecases/matching_test.go` — radius exclusion, capacity/type skip, eligible dispatch→unmatched-on-timeout, with a concurrency-safe in-memory repo/store honouring the status guards. Existing notification test updated to the new `Options` constructor + `SetPayment` fake. `go build/vet/test ./...` clean.

#### Frontend — `apps/customer`

- **Dispatch-before-payment** (`hauling_booking_controller.dart`): `confirmPayment` creates the booking immediately with `payment_method` (UI `'paystack'`→`'card'`), starts searching, then for card opens the up-front Paystack checkout via `initiateCardPayment` while the booking searches in the background. Removed the wallet top-up/verify hack (`_startPaystackCheckout`/`onPaystackCheckoutReturned` rewritten; `cancelPaystackCheckout` now cancels the unpaid card booking).
- **Customer realtime + live location**: new `data/customer_realtime_listener.dart` (mirrors the provider's websocket fast-path + reconnect), wired in `customer_app.dart` via a `realtimeListenerFactory`; `_startRealtime`/`_onRealtimeEvent` refresh booking state on push, 5s poll remains the fallback. Driver location polled every 8s on the active trip (`getBookingLocation`) and shown as a blue marker (`hauling_map_widget.dart` `driverLatLng`).
- **Coordinate guard**: location entry "Find Truck" requires resolved (non-(0,0)) pickup & dropoff coords.
- **Polish**: payment view wallet "insufficient" hard block softened to a non-blocking low-balance note (wallet is charged on acceptance now); robust Paystack return-URL parsing (parse URL + match path/`trxref`/`reference`, not substrings); live search countdown in the searching view (`searchTimeout` constant + `searchDeadline`); tier-selection preview fare uses real weight/helpers once chosen.
- New models: `CardPaymentInit`, `RealtimeToken`, `ProviderLocation`.

**Verification:** backend `go build ./... && go vet ./... && go test ./...` clean (new matcher tests pass). `flutter analyze lib` — no new issues. `flutter test` — all 25 tests pass.

**Run note:** wallet/card payment is server-driven and only active when `HAULING_PAYMENT_URL` + `HAULING_PAYMENT_SECRET` are set (point at payment-wallet-service with a shared HMAC secret); otherwise bookings settle without charging (local dev). Customer realtime needs `HAULING_NOTIFICATION_URL`/`SECRET` set (else the routes are skipped and the app falls back to polling).

### 2026-06-22 — Truck provider Earnings/Wallet screen (full stack)

**What:** Built the truck provider **Earnings/Wallet** screen to match the Figma mockup (zip screens 2271 balance-shown / 2272 balance-hidden) and connected it to a new backend endpoint. It is bottom-nav **tab 3** in the provider home shell — previously "Card" → "Coming soon", now "Earnings" (`account_balance_wallet` icon).

**Backend (`services/driver-hauling-service`)** — new `GET /provider/earnings` (provider bearer) on the existing `booking` feature:
- `internal/features/booking/models/earnings.go` — `ProviderEarnings` + `EarningsTransaction` read-model and the pure aggregator `ComputeEarnings(bookings, now)`. Unit-tested in `earnings_test.go`.
- Aggregation: completed trips → `available_balance_kobo`; in-progress (accepted..delivered) → `pending_balance_kobo`; `today_earnings_kobo` + `trips_completed_today` from trips completed today; `hours_online` = 0 (no historical tracking yet). Bookings still `awaiting_acceptance`/`cancelled`/`unmatched` are ignored. Fare per booking = `fare_final_kobo ?? fare_estimate_kobo ?? 0`. Transactions sorted newest-first; title = receiver name (else "Haulage Trip"), subtitle = dropoff (else pickup) address.
- `BookingService.GetProviderEarnings` (usecase) → `ListByProvider` (cap 200) + `ComputeEarnings`. Handler `GetProviderEarnings` + route in `booking/http`.
- **Architecture note:** this is trip-earnings *reporting* over fare data the hauling service already owns (read-only projection) — it does NOT create a ledger. The authoritative provider wallet/ledger stays in `payment-wallet-service` (credited via `CompleteJob` settlement when payments are wired). The displayed `available_balance` is therefore a booking-derived estimate, not the real withdrawable balance.

**Frontend (`apps/truck_provider`)** — new `lib/features/earnings/`:
- `models/earnings_models.dart` — `ProviderEarnings`, `EarningsTransaction`, and `EarningsTransactionGroup.group()` (Today/Yesterday/Last Week/Earlier bucketing).
- `state/provider_earnings_controller.dart` — `ProviderEarningsController` (loads `/provider/earnings`, tracks `balanceHidden`). Takes an `accessToken: () => ...` closure, not the whole auth controller.
- `ui/provider_earnings_screen.dart` — the screen: header + "Earning Summary" pill, green balance card (eye toggle, Pending/Today's sub-balances, amber Dispute badge, Withdraw pill overhanging the bottom-right edge), stats card (Trips Completed Today / Hours Online), "Recent Transactions" + "View All", grouped transaction cards with "Go to Trips". Local `formatNaira` (thousands sep; app has no `intl` dep) + `formatTransactionDate` ("2nd Jan 2025, 12:00:23"). Eye toggle masks **only** the balance card; transaction amounts stay visible (matches 2272).
- `ProviderApi.getEarnings()` + a `_get` Map helper added; `ProviderEarningsController` wired in `truck_provider_app.dart` and passed into `ProviderHomeScreen` (new required `earningsController`).
- Tests: `test/earnings_screen_test.dart` (3 widget tests via `MockClient`: load+render, eye toggle, empty state).

**Still stubbed as "coming soon"** (designs exist in the zip, not yet built): Withdraw flow (2281-2285 + receipt), Earning Summary chart (2273/2274), Transaction Detail (2275-2280), Disputes (2266-2270), View All, Go to Trips.

**Verification:** backend `go build ./... && go vet ./... && go test ./...` clean. `flutter analyze` — No issues found. `flutter test` — earnings tests pass. Screen layout visually confirmed against the mockup via a throwaway golden render.

### 2026-06-22 — Truck provider Withdrawal flow (full stack)

**What:** Built the provider **withdrawal flow** (Figma 2281 → 2282 → 2284/2285 → 2283 → "Withdrawal Receipt"/Home.png), launched from the Earnings screen's "Withdraw" button. Wired to **payment-wallet-service** (the owner of money/withdrawals per the architecture rules), NOT hauling.

**Key discovery:** `payment-wallet-service` already exposes a provider surface under `/api/v1/payment-wallet/provider` (provider bearer auth, accepts the hauling token via service-keyed secret `hauling=…`): `GET /provider/earnings` (real `WalletSummary` balance), `POST /provider/bank-accounts/resolve`, `POST /provider/bank-accounts`, `POST /provider/withdrawals`. The hauling provider token secret matches payment-wallet's `PAYMENT_WALLET_PROVIDER_ACCESS_TOKEN_SECRETS` default, so the truck provider app calls payment-wallet directly (same pattern as the customer top-up flow).

**Backend (`payment-wallet-service`)** — one addition (everything else already existed): `GET /provider/bank-accounts` (list). Repo `ListProviderBankAccounts` + usecase `ListBankAccounts` + handler + route. `go build/vet/test` clean.

**Frontend (`apps/truck_provider`)** — new `lib/features/wallet/`:
- `core/config` — added `PAYMENT_WALLET_API_BASE_URL` (default `http://…:8105/api/v1/payment-wallet`).
- `models/wallet_models.dart` — `WalletBalance`, `ProviderBankAccount`, `ResolvedBankAccount`, `WithdrawalResult`, curated `nigerianBanks` (code+name) list.
- `data/provider_wallet_api.dart` — points at payment-wallet base URL with the provider bearer token: getBalance, listBankAccounts, resolveBankAccount, registerBankAccount, requestWithdrawal.
- `state/provider_withdrawal_controller.dart` — shared multi-step flow state (balance/accounts load, amount keypad → kobo, account select, resolve+register bank, submit withdrawal with idempotency key). Token via `accessToken: () => ...` closure.
- `ui/` — 7 screens: `provider_withdrawal_form_screen` (2281 amount keypad), `provider_withdrawal_confirm_screen` (2282), `provider_bank_accounts_screen` (select/change account), `provider_add_bank_account_screen` (bank dropdown + account number → resolve → register), `provider_transaction_auth_screen` (2284 password / 2285 PIN — **visual gate**, provider auth is OTP-only), `provider_withdrawal_processing_screen` (2283; runs the withdrawal, routes to receipt or error), `provider_withdrawal_receipt_screen` (Home.png; scalloped card + success seal). Shared `widgets/wallet_keypad.dart` (numeric keypad + primary button) and `widgets/wallet_widgets.dart` (flow app bar, amount-to-pay card, bank tile).
- Earnings screen "Withdraw" now launches the flow via an `onWithdraw` hook threaded through `ProviderHomeScreen`; `ProviderWithdrawalController` + `ProviderWalletApi` wired in `truck_provider_app.dart`.
- Shared formatter extracted to `lib/core/format/money_format.dart` (earnings screen updated to use it).
- Tests: `test/withdrawal_flow_test.dart` (7 tests via `MockClient`: keypad/kobo, exceeds-balance gate, load, submit posts to `/provider/withdrawals`, error surfacing, form + confirm render).

**Caveats:** resolve/register(recipient)/withdrawal(transfer) all call Paystack — fully works only with a Paystack secret + funded balance; otherwise the UI surfaces the backend error. PIN/password authorization is a visual gate (no transaction-PIN store yet). Known inconsistency: the withdrawal form shows payment-wallet's real `available_kobo`, while the Earnings screen still shows hauling's booking-derived balance — reconcile later (point the earnings balance card at payment-wallet, keep the trip transaction list from hauling).

**Verification:** payment-wallet `go build/vet/test` clean. `flutter analyze` — No issues. `flutter test` — all 10 tests pass (3 earnings + 7 withdrawal). All 5 key screens visually confirmed against the mockups via a throwaway golden render at 430×932 (the iPhone 14/15 Pro Max design size).

### 2026-06-23 — Truck provider Earning Summary screen (full stack)

**What:** Built the provider **Earning Summary** screen (Figma 2273 / 2274 balance-hidden), reached from the Earnings screen's "Earning Summary" pill (was a "coming soon" snackbar). Lifetime total + a monthly earnings chart + the transaction list.

**Backend (`services/driver-hauling-service`)** — extended the existing `GET /provider/earnings` (no new endpoint): `bookingmodels.ProviderEarnings` gained `total_earnings_kobo` (lifetime completed), `summary_year`, and `monthly_earnings_kobo` (length-12 Jan-Dec series for the current year). `ComputeEarnings` populates them from the same completed bookings it already iterates (over the endpoint's 200-booking cap). `earnings_test.go` updated. `go build/vet/test` clean.

**Frontend (`apps/truck_provider`):**
- `features/earnings/models/earnings_models.dart` — `ProviderEarnings` gained `totalEarningsKobo`, `summaryYear`, `monthlyEarningsKobo`.
- `features/earnings/ui/widgets/earnings_chart.dart` — **new** `EarningsChart` custom painter: smooth Catmull-Rom area-line through 12 monthly points, vertical green gradient fill, dashed gridlines, painted month labels, and a tooltip bubble (with pointer) on the highest-earning month (clamped on-canvas).
- `features/earnings/ui/widgets/earnings_transaction_list.dart` — **new** shared `EarningsTransactionList` + `EarningsTransactionCard` + `formatTransactionDate`, extracted from `provider_earnings_screen.dart` (which now imports them; its private copies removed). `formatNaira` already lives in `lib/core/format/money_format.dart`.
- `features/earnings/ui/provider_earning_summary_screen.dart` — **new** screen: green Total Earnings card (eye toggle reuses the controller's `balanceHidden`), white "Earnings / Monitor and track your earnings." card with a display-only year pill + the chart, then the shared transaction list. Reuses the already-loaded `ProviderEarningsController` (no extra fetch).
- `provider_earnings_screen.dart` — the "Earning Summary" pill now pushes the new screen.
- Tests: `test/earning_summary_test.dart` (2 widget tests: renders total/chart/year, eye toggle masks total).

**Notes:** the chart's highlighted point is the peak month (deterministic), not necessarily the mock's mid-curve point. Year dropdown is display-only (current year). Monthly/total are over the 200 most-recent bookings (the endpoint cap). Still stubbed: Transaction Detail (2275-2280), Disputes (2266-2270), View All, Go to Trips, year filter.

**Verification:** hauling `go build/vet/test` clean. `flutter analyze` — No issues. `flutter test` — all 12 tests pass (3 earnings + 7 withdrawal + 2 summary). Screen + chart visually confirmed against 2273 via a throwaway golden render at 430×932.

### 2026-06-23 — Truck provider Transaction Detail + Disputes flows (full stack)

Built the final two wallet/earnings flows from the zip.

**Transaction Detail (Figma 2275-2280)** — tap any transaction in the Earnings/Summary list.
- `apps/truck_provider/lib/features/earnings/ui/provider_transaction_detail_screen.dart` — status-coloured amount (completed=green / pending=amber / failed=red), a commission breakdown (10% display-only; the earnings balances elsewhere stay gross), and a trip section for trip credits fetched from the existing `GET /provider/bookings/:id` (added `ProviderApi.getBooking` + `ProviderEarningsController.fetchTripDetail`).
- Wired via a new `EarningsTransactionList.onTransactionTap` (earnings + summary screens both pass it). No backend change.

**Disputes (Figma 2266-2270 + transaction-picker Frame)** — reached from the Earnings "Dispute" badge.
- Backend (`services/support-dispute-service`): it was customer-only; added a `/provider` route group using `BearerMiddleware(haulingProviderSecret, "truck_provider", "hauling")` that reuses the existing complaint/chat handlers (config `SUPPORT_DISPUTE_HAULING_PROVIDER_TOKEN_SECRET`, default `development-hauling-provider-token-secret`; `complainantTypeFromRole` now maps `truck_provider`→`hauling_provider`). The complaint domain already supported `hauling_provider` + `booking_reference`. `handler_test.go` added. `go build/vet/test` clean.
- Frontend `apps/truck_provider/lib/features/disputes/`: `provider_support_api.dart` (new `SUPPORT_API_BASE_URL`, default `:8107/api/v1/support-disputes`), `provider_dispute_controller.dart`, and screens: `provider_log_disputes_screen.dart` (feedback list + transaction picker sheet), `provider_select_dispute_type_screen.dart` (→ `POST /provider/complaints`, subject=dispute type, booking_reference=txn id), `provider_dispute_details_screen.dart` (status stepper + processing-record timeline), `provider_dispute_chat_screen.dart` (complaint messages; provider bubbles right/green, admin left/grey). Shared `ui/widgets/dispute_widgets.dart`.
- A dispute = a complaint; complaint.status → Submitted/Processing/Completed stepper. Processing-record notes are placeholder (no per-status history table).

**Notes:** the "Dispute" badge + transaction tap are now live (were "coming soon"). Both new client APIs use the provider's hauling bearer token directly (same pattern as withdrawals/top-ups). Remaining stubs are minor: View All, Go to Trips, year filter, receipt download.

**Verification:** support-dispute `go build/vet/test` clean. `flutter analyze` — No issues. `flutter test` — all 21 tests pass (3 earnings + 7 withdrawal + 2 summary + 3 transaction-detail + 6 dispute). All 6 new screens visually confirmed against the mockups via throwaway golden renders at 430×932.

### 2026-06-24 — Auth→booking review fixes (trip lifecycle, GPS, payment/cancel desync)

**Context:** Walkthrough of customer + truck-provider apps and `driver-hauling-service` surfaced a blocker: **no booking could complete through the UI.** The provider "Start Trip" button gated on `arrived_at_pickup`, a status nothing ever set, so the trip dead-ended at `accepted`. Fixed that plus real GPS, the search-timeout/card-payment desync, and a few cleanup items.

**1. Trip lifecycle (blocker) — full state machine wired.** Designed machine is now real: `accepted → en_route_pickup → arrived_at_pickup → picked_up → en_route_delivery → delivered → completed`. The customer app already rendered all six statuses (no customer change needed) — only the server transitions + provider UI were missing.
- Backend `services/driver-hauling-service`:
  - `booking/repositories/booking_repository.go` — new guarded transitions `MarkEnRoutePickup` (accepted→en_route_pickup), `MarkArrivedAtPickup` (en_route_pickup→arrived_at_pickup), `MarkEnRouteDelivery` (picked_up→en_route_delivery); widened `MarkPickedUp` to also accept `arrived_at_pickup`; widened `CancelByProvider` guard to all in-trip statuses; added the three methods to the `BookingRepository` interface.
  - `booking/usecases/booking_service.go` — `MarkEnRoutePickup`/`MarkArrivedAtPickup`/`MarkEnRouteDelivery` (ownership check → repo → event → customer notify), mirroring `ConfirmPickup`.
  - `booking/clients/notification_client.go` — `NotifyCustomerEnRoutePickup`/`ArrivedPickup`/`EnRouteDelivery`; new event constants in `shared/go/notifications/notifications.go` (`EventDriverEnRoutePickup`/`ArrivedPickup`/`EnRouteDelivery`).
  - `booking/http/handler.go` + `routes.go` — `PUT /provider/bookings/:id/{en-route-pickup,arrived,en-route-delivery}`.
  - New `booking/usecases/lifecycle_test.go` (full happy-path walk + wrong-provider rejection).
- Provider app `apps/truck_provider`:
  - `home/data/provider_api.dart` — `markEnRoutePickup`/`markArrived`/`markEnRouteDelivery`.
  - `home/state/provider_home_controller.dart` — matching controller methods.
  - `home/ui/screens/provider_active_trip_screen.dart` — replaced the dead `isArrived`-gated single button with a status-driven `_TripAction` (`_actionFor`): Start Driving → I've Arrived → Start Trip → Start Delivery → End Trip. `_StartTripButton` now takes a `label`.

**2. Real GPS for provider (was hardcoded Lagos).** `home/state/provider_home_controller.dart` — new `_currentPosition()` helper (Geolocator check/request permission, location-service check, Lagos `_fallbackLat/Lng` on any failure — never blocks going online); used by `goOnline` + the 30s heartbeat (record `(double,double)` tuple). `geolocator` + iOS/Android perms were already present.

**3. Payment & cancel desync.**
- `apps/customer/.../hauling_booking_controller.dart` `_startSearchTimeout` now calls the cancel API server-side (`reason: 'search_timeout'`) instead of only flipping the UI to `cancelled`, so a timed-out booking can't still be accepted/charged. Falls back to local cancel if the server call fails.
- `booking_service.go` `CreateBooking` no longer dispatches card bookings immediately; matching starts from `InitiateCardPayment` (once the customer commits to Paystack checkout), guarded to dispatch once. Clearer provider-facing error when an accept loses the card-payment race.

**4. Cleanup.**
- Provider `auth/ui/phone_entry_screen.dart` — removed the email login path (backend provider auth is phone-only; it had a hardcoded `samuel@karrygo.dev` default that would 4xx). Now phone-only.
- Customer tier card fare relabeled `from ₦…` (indicative — preview uses default weight; tier has no backend fare effect).
- Realtime: no code change — routes only register when `HAULING_NOTIFICATION_URL`/`SECRET` set; apps fall back to polling otherwise (listener already tolerates the 404).

**Verification:** hauling + shared + notification-service `go build/vet/test` clean (new lifecycle test passes). `apps/truck_provider` `flutter analyze` — No issues; `flutter test` — all 21 pass. `apps/customer` `flutter analyze lib` — only pre-existing infos in untouched files; `flutter test` — all 25 pass.

**Known remaining (not addressed):** provider earnings-balance vs withdrawal-balance inconsistency; card escrow still needs a Paystack webhook to flip `payment_status`→held; tiers remain presentation-only; face-verify / live-chat / transaction-PIN are still visual-only.

### 2026-06-26 — Customer Trips screen redesign (Figma) + completed-trip visibility fix

**What changed (`apps/customer`):** Rebuilt the "My Trips" tab to match the Figma (Past / Ongoing / Upcoming / Cancelled tab bar + rich trip cards) and fixed completed trips not appearing.

- `lib/features/home/ui/tabs/customer_trips_tab.dart` — rewritten with a 4-tab `TabController` (Past / Ongoing / Upcoming / Cancelled), Figma header (menu + title + subtitle), per-tab `TabBarView`. Lazily fetches `ProviderSnapshot`s (cached in tab state) to fill driver name/tenure/trip count on cards without an N+1 blocking render. Categorization: Past = `completed`; Ongoing = searching/active/`delivered`; Upcoming = `isUpcoming`; Cancelled = `cancelled`/`unmatched`. Returning from the detail screen re-loads history.
- `lib/features/hauling/ui/widgets/hauling_trip_widgets.dart` — new public `TripCard` (header date + 3-dot, driver row, dotted-connector route, fare + status pill), `TripStatusPill` (dark pill for completed/ongoing, green for upcoming, soft-green for cancelled), `_DottedConnectorPainter`. Kept old `TripStatusChip`/`tripStatusColor`/`formatTripDate`.
- `lib/features/hauling/ui/views/hauling_trip_detail_screen.dart` — rewritten to the Figma "Trip Detail": driver header (avatar + name + truck `color make model` + plate), Trip Completed reference + Date, route+fee card, Receiver/Package Information, fragile note, cancellation reason, and an inline **review section** for completed trips (5-star rating, description, "Do you Recommend this Driver?" Yes/No, Submit Review → read-only after submit). Falls back to "Book again" for cancelled/unmatched. Fetches provider + truck snapshots in `initState`.
- `lib/features/hauling/models/hauling_models.dart` — `HaulageBooking` gained `scheduledAt`, `completedAt` (parsed from API) + `isUpcoming` getter; `HaulingBookingStatus.tripChipLabel` (short Completed/Ongoing…/Upcoming/Cancelled labels).
- `lib/features/hauling/state/hauling_booking_controller.dart` — exposed `api` getter (for lazy provider fetches); added `submitReviewForBooking()` (review an arbitrary booking from the detail screen + refresh history); `loadHistory()` now also runs after review submit/skip and on terminal poll updates, so a just-completed trip surfaces under "Past" immediately.

**Issue #2 root cause:** submitting a review server-side `MarkCompleted`s a `delivered` booking (already in `SubmitReview`), but the Flutter history list was never refreshed after completion, so the trip didn't appear until app restart. Now refreshed in all completion paths. A skipped-review trip stays under "Ongoing" until the 30-min auto-complete worker (then moves to Past) — acceptable.

**Verification:** `flutter analyze lib` — 0 errors (only pre-existing infos/warnings in untouched files). `flutter test` — all 25 pass.

### 2026-06-30 — Support-dispute service: multi-app readiness + hardening (full stack)

**Context:** Reviewed `support-dispute-service` against the product doc. It was well-modeled but only served 2 of 4 apps (customer + hauling provider), had IDOR holes, unvalidated enums (incl. a live `wallet` bug breaking the customer wallet-dispute screen), no admin triage surface, and missing audit/notifications/identity-validation. Implemented all fixes plus value-adds, then **ran it against real Postgres** and fixed two SQL bugs the in-memory unit tests could not catch.

**Backend (`services/support-dispute-service`):**
- **Multi-app routing** — `http/routes.go` registers customer (root) + three provider bearer groups via a `ProviderGroup` list: `/provider` (truck_provider/hauling, existing), `/taxi-provider` (taxi_provider/taxi), `/dispatch-provider` (dispatch_provider/dispatch). `registerCallerRoutes(grp, includeDispute)` shares the surface; only customer gets the `…/dispute` routes. Taxi/dispatch are wired + ready but not exercisable until those driver services issue tokens.
- **Security (IDOR)** — ownership checks added to `AddEvidence`, `ListEvidence`, `EscalateToDispute`, `GetDispute` (usecase takes requester identity; admin passes empty type to bypass). `assertOwnership` helper.
- **Validation** — `migrations/003_add_wallet_service_type.sql` (single-statement `ALTER TYPE … ADD VALUE 'wallet'`); usecase validates service_type/category/status/outcome/respondent_type → `validation_failed` (HTTP 422) instead of raw 500s.
- **Admin triage** — `/admin/complaints` (filter by status/priority/service_type/complainant_type, emergency sorts first), `/admin/complaints/:id`, `/evidence`, `/dispute`, `/events`, `/disputes`, `/refresh-identity`. Repo `ListComplaints`/`ListDisputes` use a dynamic WHERE builder.
- **Doc-gap features** — `complaint_events` audit trail now written (`RecordEvent`) + readable (`/events`); `SupportNotifier` (`clients/notifier.go`, fire-and-forget, no-op when unconfigured) notifies on status change / admin reply / dispute resolution; admin-driven refund hook in `ResolveDispute` via `shared/go/walletclient` (only when outcome favours complainant + payment-wallet configured + admin supplies `refund_source_reference`+`refund_amount_kobo`).
- **Add-ons** — F1 unread counts + `…/messages/read` (uses `support_chat_messages.is_read`); F2 realtime chat proxy feature (`internal/features/notifications/http`, mirrors hauling) for customer + 3 providers; F3 `migrations/004_categories_and_help.sql` (`complaints.category/priority/incident_lat/lng` + `help_articles`) with public `GET /support/categories` + `/support/faqs` and `seeds/help_articles.sql`; F4 `POST /sos` (emergency-priority complaint + reporter ack).
- **Cross-service identity (G)** — `clients/identity.go` resolves complainant/respondent identity from the owning service by `complainant_type`, signing HMAC as `support-dispute-service`; `migrations/005_identity_snapshot.sql` adds `complainant_name/phone` + `respondent_name/phone` snapshot columns. **Soft/non-blocking** — actions succeed even if the owning service is down. New internal endpoints on the owning services: `GET /api/v1/customer/internal/customers/:id` (customer-service `features/identity`) and `GET /api/v1/hauling/internal/providers/:id` (hauling `features/identity`), both behind `serviceauth.Middleware`; each owning service gained a `*_SERVICE_SECRETS` config trusting `support-dispute-service`.

**Two SQL bugs found via smoke test + fixed** (`repositories/repository.go`) — Postgres enum/text parameter-binding errors invisible to the in-memory unit tests:
1. `ListComplaintsByComplainant` unread subquery reused `$1` across an enum (`complainant_type`) and text (`sender_type`) → cast `$1::complainant_type`.
2. `UpdateComplaintStatus` `status = $2` (enum) vs `$2 IN ('resolved','closed')` (text) → cast `$2::complaint_status`.

**Tests:** `usecases/support_service_test.go` (in-memory repo: ownership denial, enum validation, wallet, SOS priority, unread). `repositories/repository_integration_test.go` — **DB-backed regression guard**, env-gated on `SUPPORT_DISPUTE_TEST_DATABASE_URL` (skips in plain `go test ./...`), locks in the two enum-cast fixes + exercises every repo method. `go build/vet/test ./...` green for support-dispute, customer-service, hauling.

**Ops artifacts:**
- `scripts/bootstrap_services/support-dispute-local-bootstrap.sh` — Postgres-only (port 5439, DB `support_dispute_service`), applies all `migrations/*.sql` in order + seeds FAQs. Added `support-dispute` to `bootstrap-all.sh`.
- `docs/support-disputes.postman_collection.json` + `.postman_environment.json` — every endpoint with descriptions; admin folder auto-signs HMAC.
- `README.md` — full endpoint reference + auth model + run instructions. `.env` + `.env.example`.

**Smoke-test result:** bootstrap applied migrations 001–005 + seed; a throwaway Go tester (since deleted) drove 30/30 endpoint checks against a live instance on `:8107` — public/customer/provider/admin groups, wallet complaint, IDOR 403s, unread+mark-read, dispute lifecycle, SOS emergency, admin triage/resolve, auth-negative.

**To run locally:**
```bash
bash scripts/bootstrap_services/support-dispute-local-bootstrap.sh
cd services/support-dispute-service && go run ./cmd     # :8107
# DB-backed regression test:
SUPPORT_DISPUTE_TEST_DATABASE_URL='postgres://cosmicforge_logistics:cosmicforge_logistics@localhost:5439/support_dispute_service?sslmode=disable' \
  go test ./internal/features/support/repositories/ -run Integration -v
```

**Known remaining:** taxi/dispatch provider groups are ready but their backend services don't issue tokens yet; SOS "admin alert" is a reporter ack + top-of-queue priority (notification-service has no admin recipient audience); refund hook needs payment-wallet + Paystack wired; identity/notifications/realtime are config-gated.

### 2026-07-06 — Customer live-trip screens redesigned to Figma (zip 22: 1736/1766, 1772-1775, 1776)

**What changed (`apps/customer` hauling flow):** the Connecting Driver, Driver Assigned / Trip has started, and rating screens were rebuilt to match the Figma mockups exactly.

- **New shared widgets** `lib/features/hauling/ui/widgets/hauling_trip_sheet_widgets.dart`: `HaulingDiscountBanner` (floating "10% off first booking" card), `HaulingDriverProfileBlock` (avatar + name + "Driving with KarryGo . 1 year" green + "Completed Trips . 1,000+" gold + amber stars "5.0 Rating"), `HaulingDriverAvatar`, `HaulingTripStatsRow` (km / ETA / ₦ fare, `formatHaulingFare` = commas + 2 decimals), `HaulingVehicleLine` ("Blue • MAN TGX **EKY232GY**" green plate), `HaulingTripProgressSlider` (green donut → track → pale dot, optional truck png at progress), `HaulingPillButton`, `HaulingFlowBottomNav` (reuses `CustomerHomeBottomNav`, Home selected; any tap pops the flow back to the home shell — booking keeps running, reachable from Trips), plus `haulingEtaMinutes`/`haulingHaversineKm`.
- **`hauling_searching_view.dart` (1736)** — map + banner + sheet: "Connecting Driver…", matched-driver block (spinner placeholder until `awaiting_acceptance` provides a provider), stats row, Pick-up / Add stop (static) / Drop off route section with green connector column, "Edit Destination" tinted pill (confirm sheet → `controller.editDestinationDuringSearch()`: cancels server-side, returns to location entry prefilled), green "Cancel Truck" pill. Removed the visible search countdown (timeout logic unchanged).
- **`hauling_active_trip_view.dart` (1772/1773)** — two phases: pre-pickup "Driver Assigned, arriving in X minute(s)" (ETA from live driver location → pickup haversine; "arriving soon" without a fix) with Call Driver (green) + Cancel Truck (pale `primarySoft`); after pickup "Trip has started." with Share Trip / Report / SOS grey circles (share = clipboard copy + snackbar; SOS = confirm sheet → tel:112; report = coming-soon snackbar). Vehicle line under the subtitle; slider truck position derived from live location between pickup/dropoff.
- **`hauling_review_view.dart` (1776)** — map + white back circle (back = `skipReview`), sheet: "You have arrived", full-bleed green belongings banner, centered avatar/name/vehicle line, Trip Completed (booking id) + Date ("Monday, 22 May 2026") rows, **green** 1-5 stars, grey filled "Enter Description here" field, "Yes, I do!"/"No, I don't!" pills (solid green selected / pale green unselected), Submit Review pill. Skip button removed (back arrow covers it).
- **Model/backend:** `ProviderSnapshot` gained `createdAt` + `tenureLabel` ("1 year") + `tripsLabel` ("1,000+") ; hauling-service `GetPublicProvider` now returns `created_at`.
- Deleted now-unused `hauling_driver_card.dart` (`HaulingDriverCard`/shimmer replaced by the profile block).
- Fixed pre-existing broken hauling-service test compiles (in-progress rejected-providers work): `noopAvailabilityStore`/`fakeStore` gained `MarkRejected`/`GetRejectedProviders`; removed an unused import in `profile_service_test.go`.

**Verification:** hauling `go build/vet/test` green; `flutter analyze` 0 new issues; `flutter test` all 25 pass; all four states deployed to the physical Android device via a throwaway `-t` preview target (since deleted) and screencap-compared against the mockups.

### 2026-07-06 — Booking flow race fixes ("accepted, then asked to accept again")

**Root cause 1 (backend, primary):** the matcher's timeout path in `tryEligibleProviders` was read-then-reset: it re-read the booking after the 30s accept window and, seeing `awaiting_acceptance`, called `ResetToMatching` — whose SQL guard only excluded terminal statuses, so an accept landing between the read and the reset was silently clobbered back to `pending_match` and re-dispatched (often to the same provider).

**Root cause 2 (provider app):** `_pollBookings` had no in-flight/staleness guard, so a 4s poll that started before an accept could return after it and re-populate `pendingRequests` with the just-accepted booking, flipping the UI from active trip back to the incoming-request sheet.

**Backend (`services/driver-hauling-service`):**
- `booking_repository.go` — `ResetToMatching(ctx, id, expectedProviderID)` is now a compare-and-set (`WHERE status='awaiting_acceptance' AND provider_id=$x`); `MarkAccepted(ctx, id, providerID)` guard now also requires `provider_id` (an accept can't land on a booking rematched to someone else). Interface updated.
- `booking_service.go` — timeout path replaced with guarded-reset-then-interpret: try `ResetToMatching` first; if it's refused, re-read — accepted means the provider keeps the lock and matching stops. `AcceptBooking`/`RejectBooking` map a failed guard to `apperrors.Conflict` ("This request has expired or been reassigned." / "This request can no longer be rejected.") via new `isNotFound` helper.
- Tests: `trackingRepo` fakes honour the new guards; new `TestMatch_AcceptAtTimeoutBoundaryNotClobbered` pins the race deterministically via an `acceptDuringResetRepo` wrapper (accept injected between timeout fire and reset).

**Provider app (`apps/truck_provider` `provider_home_controller.dart`):**
- `_pollInFlight` prevents overlapping polls; `_stateGeneration` is bumped by every local mutation (accept/reject/deliver/cancel/offline) and any poll response fetched under an older generation is dropped.
- `acceptBooking`/`rejectBooking` return `Future<bool>`; on failure the booking is removed from `pendingRequests` locally (a genuinely-still-pending request reappears on the next fresh poll) and the error surfaces via the existing error pill. `provider_request_detail_screen.dart` only pops on success.
- Active-trip poll handling: customer-cancelled/unmatched trips now exit to the dashboard **with a visible message** ("This trip was cancelled by the customer."); a booking missing from the poll page no longer kicks the active trip off screen.

**Customer app (`apps/customer`):**
- `hauling_booking_controller.dart` — when a searching booking drops back to `pending_match` (provider timed out/rejected), the stale `providerSnapshot` is cleared so the searching view returns to the "connecting" placeholder instead of showing the old driver.
- `hauling_details_view.dart` — `DropdownButtonFormField` `value:` → `initialValue:` (deprecation).

**Verification:** hauling `go build/vet/test` green (incl. new boundary test); truck_provider `flutter analyze` clean + all 24 tests pass; customer `flutter analyze lib` no new issues + all 25 tests pass.

### 2026-07-06 — End-to-end booking + notification hardening (review-driven, 4 phases)

A full audit of the hauling system (customer app → hauling-service → provider app + notification pipeline) surfaced ~30 issues; all approved fixes implemented and verified. Decisions: block customer cancel after pickup; card payment removed for now (wallet/cash only); FCM wired now (Android).

**Phase 1 — booking correctness (`services/driver-hauling-service`):**
- **Match recovery worker** (`RunMatchRecoveryWorker`, wired in `cmd/main.go`): matchers are in-memory goroutines, so a restart used to strand bookings in `pending_match`/`awaiting_acceptance` forever. The worker (30s tick + on start) re-adopts them via new repo `ListStaleMatching(pendingCutoff, awaitingCutoff)`: resumes matching if the search window (from `created_at`) remains, else `markUnmatched`. Skips bookings with a live matcher (`activeMatches`) and never-initiated legacy card bookings.
- **Timed-out providers are `MarkRejected`** in the matcher timeout branch — previously only explicit rejects were, so the nearest silent provider was re-offered every rescan, eating the whole window. Now each provider is offered at most once per booking.
- **`MarkUnmatched` guarded CAS** (`WHERE status='pending_match'`) replaces the unguarded `UpdateStatus` in `markUnmatched` — a stale deadline can no longer clobber a concurrent accept.
- **Provider cancel** now calls `cancelMatch` + `ReleaseMatchLock` (both leaked when cancelling from `awaiting_acceptance`).
- **Cancel policy**: `CancelByCustomer` guard also excludes `picked_up`/`en_route_delivery`; usecase maps the refused cancel to 409 Conflict ("cargo already in transit… contact support"). Customer UI already hides Cancel after pickup.
- **Card removed at create**: `CreateBooking` rejects `payment_method=card` (validation error); dispatch is unconditional again. `InitiateCardPayment` route remains for legacy rows only.
- Deleted dead `AssignProvider` repo method. New tests: `recovery_test.go` (timeout-rejected, unmatched guard, recovery resume/expire/orphaned-offer/live-matcher-skip, cancel-after-pickup 409, card-rejected).

**Phase 2 — notification pipeline:**
- **Recipient-aware idempotency**: `BookingNotifier.send` keys on `bookingID+":"+recipientID` — previously provider B's `booking.matched` was silently dropped by notification-service's `ON CONFLICT (idempotency_key) DO NOTHING` after provider A was offered the same booking.
- **3 missing templates seeded** (`booking.en_route_pickup`, `booking.arrived_at_pickup`, `booking.en_route_delivery`) in `notification-service/seeds/dev_templates.sql` — these events were silently rejected (no template + no inline title/body). `notification-local-bootstrap.sh` now applies the seed (it previously only ran the migration — root cause of missing templates at runtime).
- **Provider real notification feed**: `ProviderNotificationApi.listNotifications()` (`GET /provider/notifications`), new `ProviderNotificationController` (load on open, pull-to-refresh, live refresh fanned out from the existing home realtime websocket, unread styling via persisted last-seen marker) and `provider_notifications_screen.dart` rewritten from 100% hardcoded fake data to the real feed (same Figma design).
- **Customer unread badge**: `CustomerHomeBottomNav` renders `NotificationController.unreadCount` on the Alerts item; unread is no longer reset by `loadFeed` (only by opening the Alerts tab, which also refreshes the feed).

**Phase 3 — FCM push (Android, end-to-end):**
- Firebase project `logistics-93b9f` (same one media-file-service uses). Both Android apps registered via the Firebase Management REST API (service account at `credentials/firebase.json`; the firebase CLI's app-create timed out, REST worked): customer `1:…:android:23fdca…`, provider `1:…:android:7c2bce…`. `google-services.json` written into each `android/app/`; hand-maintained `lib/firebase_options.dart` in both apps (Android only; iOS deferred — needs APNs + iOS app registration).
- Gradle: `com.google.gms.google-services` 4.4.3 plugin in both apps' `settings.gradle.kts` + `app/build.gradle.kts`. `firebase_core` + `firebase_messaging` added to both pubspecs. Both debug APKs build.
- New `FirebasePush` helper (per app, `features/notifications/data/firebase_push.dart`): failure-tolerant init/token/permission + lazy `tokenRefresh()`/`foregroundMessages()` streams — everything no-ops when Firebase isn't configured, so WS+poll remain the fallback.
- Customer: `PushRegistrationService` finally constructed (with `FirebasePush.deviceToken`) and passed to `NotificationController`, so `start()` registers the device post-auth; token-refresh re-registers; foreground pushes → `loadFeed()` + new `HaulingBookingController.pokeRefresh()`.
- Provider: device registered on login (auth-listener in `truck_provider_app.dart`) + on token refresh via existing `registerDevice`; foreground pushes → `ProviderHomeController.pokeRefresh()` + feed refresh.
- Backend: `notification-service/.env` now sets `FIREBASE_PROJECT_ID` + `GOOGLE_APPLICATION_CREDENTIALS` (push sender switches from logging to real FCM v1).
- **Remaining manual step:** physical-device FCM check (no device attached this session): install both apps, log in, background the provider app, create a booking → tray notification.

**Phase 4 — dead code + UX:**
- Customer: deleted orphaned `hauling_confirm_view.dart`, `hauling_delivered_view.dart`, `hauling_availability_check_view.dart`, `hauling_route_point.dart`; removed unreachable `HaulingFlowStatus.delivered`, dead `backToPayment()`, dead `initiateCardPayment`/`CardPaymentInit`, dead `searchDeadline`; `HaulingCancelledView` now shows `state.error` (the search-timeout "No trucks found nearby…" message finally surfaces); poll-race guard (`_pollInFlight` + `_stateGeneration`, mirroring the provider fix) on booking/driver-location polls with generation bumps on cancel/timeout/reset/review/open; live review screen skips to completed when a review already exists (`getReviewForBooking` check in `_applyBookingUpdate`).
- Provider: **proof of delivery is now real** — migration `007_proof_of_delivery.sql` (`proof_photo_url`, `signature_url` on `haulage_bookings`), `MarkDelivered(id, proofURL, sigURL)` + `ConfirmDelivery(…, proof, sig)` + optional JSON body on `PUT …/delivered`; app renders the signature strokes to PNG (`ui.PictureRecorder`, pad-sized via GlobalKey), uploads photo (`proof_image`) + signature (`signature`) through `MediaUploadService` (new `uploadBytes`; uploads are best-effort — media outage never blocks delivery), sends URLs; photo preview fixed `Image.network`→`Image.file`. Going offline mid-trip keeps the active trip + its poll alive (only availability stops); `restoreOnlineStatus` polls bookings first so an in-progress trip is recovered even when the online TTL lapsed; `_actionFor` unknown status renders a disabled "Updating trip…" instead of re-firing Start Driving; removed hardcoded test phone from `auth_entry_screen.dart`.

**Verification:** hauling + notification-service + shared/go `go build/vet/test` all green; both apps `flutter analyze` clean (customer's 7 pre-existing infos only) + all tests pass (provider 24, customer 25); both debug APKs build with the Firebase gradle wiring. **Live E2E** (bootstraps + seeder + `cmd/online` + both services): (1) 3 silent providers each offered exactly once → all 3 got their own `booking.matched` (recipient-keyed) → `booking.unmatched` to customer; (2) service killed mid-offer → booking stranded `awaiting_acceptance` → restart → `match_recovered` → `unmatched` (window spent); (3) full lifecycle accept→…→picked_up → customer cancel refused HTTP 409 → delivered with proof URLs persisted on the row → review → completed, with all 8 lifecycle notifications stored incl. the 3 previously-dropped ones.

### 2026-07-07 — Customer Saved Locations (full stack, Figma zip 23) + booking integration

**What:** Built the profile "Saved Locations" section to match Figma zip 23 (screens 1777 list, 1778/1782/1786 add-edit form, 1779-1781 map picker) and integrated saved locations into the truck-hauling location entry as one-tap fills alongside the existing current-location action. User decisions: delete via long-press on a list card + a delete icon in the edit form header (nothing added to the visible Figma layouts); booking integration as tappable rows under "Use my current location".

**Backend (`services/customer-service`)** — implemented into the previously-empty `internal/features/saved_locations/` scaffold, mirroring the emergency-contacts pattern:
- `migrations/005_saved_locations.sql` — `saved_locations` (customer_id FK cascade, title, address, latitude/longitude, description, timestamps).
- `models/saved_location.go`, `repositories/postgres_saved_location_repository.go` (List/Create/Update/Delete, all customer-scoped; missing row → NotFound), `usecases/saved_location_service.go` (title+address required, (0,0) coords rejected → validation error with fields), `http/handler.go` + `routes.go`.
- Routes (customer bearer): `GET/POST /saved-locations`, `PUT/DELETE /saved-locations/:id`. List envelope: `{locations: [...]}`. Wired in `cmd/main.go`.
- `usecases/saved_location_service_test.go` — trim/validation/ownership tests. Smoke-tested live on :8101: full CRUD + validation 422 + ownership 404 + auth 401 all pass.

**Customer app (`apps/customer`):**
- `SavedLocation` model + `getSavedLocations`/`addSavedLocation`/`updateSavedLocation`/`deleteSavedLocation` on `CustomerAuthApi`.
- New `lib/features/profile/ui/saved_locations/`: `saved_locations_header.dart` (shared back-circle + title/subtitle header), `saved_locations_screen.dart` (1777: "+ Add New" pill, cards with title/address/chevron, tap→edit, long-press→delete confirm, pull-to-refresh, empty/error states), `saved_location_form_screen.dart` (1778/1782/1786: grey filled fields, read-only Address field with pin icon → map picker, multiline description, Save pill pale-disabled/solid-enabled, delete icon in edit mode), `saved_location_map_picker_screen.dart` (1779-1781: full-screen GoogleMap, back circle + white search pill with green pin, debounced Places autocomplete dropdown, map-tap → reverse geocode, green marker + camera animate, "Confirm Location" pill → pops `SavedLocationPickResult`).
- Wiring: `placesApi` threaded `CustomerHomeScreen` → `CustomerProfileScreen` → `CustomerProfileTab`; "Saved Locations" menu item now pushes the list screen.
- Booking integration: `HaulingBookingController` gained optional `savedLocationsLoader` (wired in `customer_app.dart` via `_authApi` + current session), so all three `HaulingFlowScreen` entry points (home tab, trips "book again", trip detail) get it. `hauling_location_entry_view.dart` loads it in `initState` (failure-silent) and shows a "Saved locations" rows container (bookmark icon + title + address, suggestion-list styling) below "Use my current location" whenever the active field has <2 chars typed; tapping fills the field + controller coords directly (no Places round-trip), so "Find Truck" enables immediately.

**Verification:** customer-service `go build/vet/test` green; `flutter analyze` — only the 7 pre-existing infos; `flutter test` — all 25 pass. Backend CRUD smoke-tested against the live service. List + form layouts checked via throwaway golden renders (since deleted). **On-device screencap comparison still pending — no Android device was attached this session; migration 005 is applied to local Postgres (5433) and the running customer-service on :8101 serves the new routes.**

### 2026-07-09 — Customer dispatch (bike delivery) booking system (full stack)

**What:** Built the customer-facing dispatch booking end-to-end: the dispatch backend had NO customer surface (bookings only came from the api-gateway partner B2B API via the `booking.dispatch.created` Redis event; the service is otherwise provider-only). Implemented the missing customer API on `driver-dispatch-delivery-service` per the developer audit doc's proposed contract, plus the full Bike Delivery flow in the customer app (cloned from hauling with dispatch parameters) and a merged My Trips tab.

**Backend — `services/driver-dispatch-delivery-service` (new `internal/features/booking/`):**
- Routes under `/api/v1/customer/dispatch` (customer bearer via `shared/go/auth`, secret `DISPATCH_CUSTOMER_TOKEN_SECRET` = customer-service access secret): public `POST /bookings/estimate`; authed `GET /availability?lat&lng` (Redis geo nearby count, `DISPATCH_AVAILABILITY_RADIUS_KM` default 10), `POST/GET /bookings`, `GET /bookings/:id`, `POST /bookings/:id/cancel`, `POST+GET /bookings/:id/review` (into existing `ratings` + new `recommends_provider` column), `GET /bookings/:id/location` (availability live store), `GET /providers/:id` (profile public alias).
- Migrations `000031_create_dispatch_bookings` (customer-owned booking record: coarse status searching|accepted|completed|cancelled|unmatched, payment_method/status/ref, package fields, service_tier) + `000032_add_review_fields_to_ratings`.
- **Status is derived on read** via `dispatch_bookings LEFT JOIN request_broadcasts LEFT JOIN trips`: trip status wins (assigned→accepted, en_route_pickup, arrived_pickup→arrived_at_pickup, in_progress→in_transit, proof_submitted→delivered, completed, cancelled/failed→cancelled), then booking terminal (unmatched BEFORE cancelled_at — order matters, unmatched rows carry cancelled_at), then broadcast (broadcasting→searching, no_provider_found/expired→unmatched, accepted-no-trip→accepted).
- **Wallet escrow (charge at creation, fail fast):** create → insert row → `CreatePaymentIntent`+`PayFromWallet` (idem keys `dispatch-hold-/dispatch-pay-<bookingID>`, `SourceService=dispatch-delivery-service`, `SourceReference=bookingID`, `ProviderType="dispatch"` so settlement lands in the ledger the rider app reads) → **direct in-process** `requestSvc.StartBroadcast` (idempotent; failure refunds + fails create; partner pub/sub path untouched). Insufficient balance → 400 `payment_failed` with the wallet's message, booking marked cancelled/failed.
- **Subscribers** (`request.accepted`, `request.no_provider_found`, `trip.completed`, `trip.cancelled`): accept → `SetAccepted` (CAS from searching) + wallet `AssignProvider(paymentRef, "dispatch", riderID)`; no-provider → `SetUnmatched` (strict CAS from searching — a stray event can never clobber/refund an accepted booking) + refund `refund-booking-<id>`; trip completed → completed/paid; trip cancelled → sync + re-issue trip-keyed refund (wallet dedupes with the trip service's own auto-refund).
- **Cancel endpoint:** pre-accept → `CancelBroadcastForBooking` + re-read (accept race → 409) + refund `refund-booking-<id>`; rider assigned → synchronous `trip.HandleBookingDispatchCancelled` via `TripCancelAdapter` then verify the trip actually cancelled before refunding `refund-<tripID>` (trip-side guard no-ops for in_progress → 409, no refund); in transit/delivered/terminal → 409.
- Request feature: `RequestAcceptedEvent` gained `customer_id` (from broadcast payload) so `trips.customer_id` is populated. `cmd/main.go` now loads `.env` via godotenv (hauling pattern). Tests: fake-based table-driven suite (create/cancel matrix incl. races, status derivation, subscriber idempotency, reviews) + migration files test.

**payment-wallet-service + shared/go/walletclient:**
- New idempotent internal endpoint `PUT /internal/payments/:reference/provider` (`AssignPaymentProvider`) — attaches the matched provider to a held intent, because `CompleteJob` refuses settlement without provider details; conflict on reassignment/refunded. `walletclient.AssignProvider` added.
- **Fixed a latent refund bug:** `RequestRefund` resolved intents only by intent reference, but the dispatch trip service passes the booking id — every dispatch refund would have 404'd. Now falls back to `GetPaymentIntentBySource(source_service, source_reference)`; DTO/input gained the source pair (trip service already sends them).

**Customer app — `apps/customer` (new `lib/features/dispatch/`):**
- Clone-and-adapt of the hauling flow: `dispatch_models.dart` (DispatchBookingStatus per wire mapping; PackageSizeOption small/medium/large; ServiceTierOption standard/express; reuses hauling `ProviderSnapshot`/`ProviderLocation` so the shared trip-sheet widgets work unchanged; `riderSnapshotFromPublicProfile` maps the dispatch public profile), `dispatch_api.dart` (list unwraps `{bookings:[]}`; location 404 → unavailable; review 404 → null), `dispatch_booking_controller.dart` (same machine: 5s booking poll, 8s rider location poll, generation guards, saved locations; poll-only — no realtime; 100s search timeout slightly past the 3x30s broadcast window so the server's own unmatched+refund usually lands first).
- Flow: locationEntry (cloned, "Book a Bike Delivery"/"Find a Rider") → packageInfo (desc, optional type, size chips, Standard/Express selector, fragile, receiver) → payment (wallet-only + fare breakdown incl. size/express surcharges + ETA; top-up CTA) → searching/activeTrip/review/completed/cancelled (rider labels, no truck/vehicle line). `DispatchFlowScreen` router; trip detail screen clone.
- Wiring: `dispatchApiBaseUrl` (`DISPATCH_API_BASE_URL`, default `http://192.168.1.138:8103/api/v1/customer/dispatch`); `customer_app.dart` builds `DispatchBookingController` (FCM poke included); home tab case 1 (Bike Delivery) launches the flow. **Trips tab merged**: sealed `_TripEntry` wrapper, both histories loaded/bucketed/sorted together, `DispatchTripCard` with a green "Bike" badge, taps route to the dispatch flow (active) or `DispatchTripDetailScreen` (terminal).
- Tests: `test/features/dispatch/dispatch_api_test.dart` (6 tests: status matrix, create contract body, list unwrap, 404 mappings, profile mapping). All 31 app tests pass; `flutter analyze` clean.

**Ops:** `scripts/bootstrap_services/dispatch-local-bootstrap.sh` (Postgres 5435 / Redis 6382; golang-migrate files aren't re-run-safe so applied files are tracked in a `local_schema_migrations` marker table — verified idempotent), service `.env` (rider secrets, wallet secret `development-payment-wallet-service-secret` matching payment-wallet, customer token secret).

**Live E2E (all passed):** customer OTP token → estimate (fare matches gateway formula exactly incl. rounding) → availability 0 riders → create with empty wallet → 400 payment_failed + booking cancelled/failed → funded wallet (dev ledger credit) → create → held (balance debited) + broadcast row → cancel → refunded (balance restored) + broadcast cancelled → second booking left to expire → 3 broadcast attempts → `no_provider_found` → unmatched/refunded automatically → third booking + seeded rider + real `request.accepted` event → trip row created (with customer_id), booking accepted, **payment intent got provider dispatch/<rider> via AssignProvider** → `CompleteJob` settled (rider provider_payable credited fare minus platform fee — withdrawable via the existing rider wallet endpoints) → trip.completed → booking completed/paid → review submit + duplicate 409 + fetch + rider profile alias + list. Two bugs found by the E2E and fixed with regression tests: unmatched-vs-cancelled derivation order, and the SetUnmatched strict CAS.

**Remaining manual step:** no Android device was attached this session — deploy the customer app to the physical device and screencap the Bike Delivery flow. Rider-app-driven trip lifecycle (accept via the dispatch_provider app UI) also untested end-to-end, though the event pipeline it drives was exercised directly.

**Run:**
```bash
bash scripts/bootstrap_services/dispatch-local-bootstrap.sh
cd services/driver-dispatch-delivery-service && go run ./cmd   # :8103 (.env auto-loaded)
# customer app adds: --dart-define=DISPATCH_API_BASE_URL=http://<host>:8103/api/v1/customer/dispatch
```

### 2026-07-09 — Dispatch follow-ups: shared pricing + rider seed + rider-driven E2E

**Shared pricing (`shared/go/dispatchpricing`):** the dispatch fare formula (base ₦800 + ₦180/km, +₦300 medium/+₦700 large, ×1.35 express, round 50 kobo, min ₦1,000; ETA; haversine; size/tier normalizers) now lives in one shared package with its own pinned tests. `api-gateway/internal/features/partner/delivery/pricing.go` and dispatch `booking/pricing.go` (+ `NormalizePackageSize/Tier` in booking/model.go) are thin delegates — drift between the partner and customer surfaces is now impossible. Behavioral no-op: gateway + dispatch test suites pass unchanged. (Gateway's separate config-driven `internal/pkg/pricing` engine untouched.)

**Dispatch dev rider seed:** new `services/driver-dispatch-delivery-service/seeds/dev_seed.sql` + `cmd/seed/main.go` (hauling pattern). Seeds 2 fully verified riders that pass every online gate: `dispatch_rider_identities` (login requires it), `providers` (verified, `onboarding_complete=true` — required to reach the app Home), `provider_availability` (`verified_to_go_online=true`), verified active bike, approved licence step with future expiry. Riders: `+2348077770001` Emeka Dispatch, `+2348077770002` Bisi Rider. Includes a reclaim block renaming stray non-seed rows holding seed phones/emails/plates (deletes would hit trip FKs). **OTP gotcha (differs from hauling): dispatch prints the debug OTP in the SERVICE LOGS only** (`development dispatch rider login otp ...`), never in the HTTP response. `dispatch-local-bootstrap.sh` now applies the seed automatically. NOTE: dispatch has no `cmd/online` helper (that's hauling-only) — riders go online via the app/API and it works because the seed passes the gates.

**Rider-driven E2E (real provider endpoints, no event simulation) — all passed live:** seeded rider OTP login (code scraped from logs) → `PATCH /provider/availability online` through the real gates → GPS heartbeat → customer availability shows count=1 → customer books (wallet held) → request in rider inbox → accept → customer `accepted` + trip row + wallet intent got `dispatch/<rider>` → `arrived`→`arrived_at_pickup`, `start`→`in_transit`, `proof` (real multipart photo+signature)→`delivered`, `complete`→`completed/paid` + rider `provider_payable` credited fare-minus-fee → live location endpoint returns the heartbeated fix (note: 30s TTL — a stale single push 404s by design) → review submitted.

**Bug found & fixed by the E2E:** `booking/repository.go InsertReview` inserted into `ratings` without recomputing `providers.avg_rating`, so customer reviews never moved the rider's public rating (the profile feature's own rating insert does recompute). Now wrapped in a tx with the same recompute UPDATE; verified live (5 then 4 → avg_rating 4.5); pre-fix rows backfilled in the dev DB.

**Verification:** shared/go + api-gateway + dispatch `go build/vet/test` all green (existing tests unchanged); seed re-run idempotent; bootstrap re-run idempotent. **Still pending: on-device visual pass (no adb device attached again).** CUSTOMER_BOOKING_README.md updated (delegated pricing notes, seed workflow, rider E2E results, refreshed Known Gaps).

### 2026-07-11 — Deferred-items sprint: balance unification, real PIN, real support chat, iOS push prep

Cleared 3 of the 5 deferred work-report items in code (the other 2 are device-blocked manual QA). Face verification explicitly deferred again (user decision).

**A. Provider balance unification (frontend-only).** The Earnings screen balance card now shows the payment-wallet ledger balance — the same figure the withdrawal flow uses. `ProviderEarningsController` takes an optional `walletApi` (wired in `truck_provider_app.dart`, shared instance) and loads hauling earnings + wallet balance in parallel; wallet failure is non-fatal and the card falls back to the hauling booking-derived estimate with the label "Available Balance (estimated)". Pending = wallet `pending_kobo`; "Today's Earnings" + stats + transaction list stay hauling-derived. Earning Summary lifetime total intentionally stays hauling (trip reporting, not a balance). Tests: 2 new widget tests (wallet wins / fallback + estimated label).

**B. Real withdrawal transaction PIN (full stack, payment-wallet-service owns it).**
- Backend: `migrations/002_provider_transaction_pins.sql` (`provider_transaction_pins`: provider_type+provider_id unique, `pin_hash` bcrypt, `failed_attempts`, `locked_until`); repo Get/Create/Update; `usecases/transaction_pin.go` — `TransactionPinStatus` / `SetTransactionPin` (4-6 digits, conflict if set) / `ChangeTransactionPin` / `VerifyTransactionPin` with lockout (5 consecutive failures → 15-min lock; correct PIN refused while locked; success resets counters). `RequestWithdrawal` now requires `pin` (validation error without it; `pin_not_set`/`invalid_pin`/`pin_locked` error codes) — verified BEFORE any bank-account/Paystack work. Routes: `GET /provider/pin/status`, `POST /provider/pin`, `PUT /provider/pin` (provider bearer). `golang.org/x/crypto` promoted to direct dep. Unit suite `transaction_pin_test.go` (fake store; set/verify/lockout/change matrix + withdrawal-requires-pin).
- Flutter (`apps/truck_provider`): `ProviderWalletApi` gained `getPinStatus`/`setPin`/`changePin` (+ `_put` helper); `requestWithdrawal` sends `pin`. `ProviderWithdrawalController`: `pinSet` loaded with balance/accounts, `setEnteredPin`, `setupPin`. `provider_transaction_auth_screen.dart` rewritten as a real gate: pinSet → enter-PIN → withdraw; no PIN → create + confirm (mismatch restarts with error) → save → withdraw; password variant deleted. Wrong-PIN server errors surface on the processing screen's error state which pops back to the auth screen. Tests updated + 2 new widget tests (enter phase, create/confirm mismatch).
- **Live smoke (all passed):** status false → set → status true → duplicate set 409 → change wrong-current `invalid_pin` → change ok → withdrawal without pin 422 (`fields:[pin]`) → 4× wrong pin `invalid_pin` → 5th `pin_locked` → correct pin still `pin_locked`.
- `scripts/bootstrap_services/payment-local-bootstrap.sh` now loops over ALL `migrations/*.sql` (was hardcoded to 001).

**C. Real in-app support chat (provider app ↔ support-dispute-service).** `provider_support_screen.dart` rewritten: the canned local `_ChatMessage` list is gone. "Start Chat" opens `ProviderLiveChatScreen`, which calls `ProviderDisputeController.openSupportChat()` → `POST /provider/support-chat/start` (find-or-create an open "Support Chat" complaint, verified idempotent), then reuses the real complaint-chat: `loadMessages`/`sendMessage`, `markMessagesRead`, 5s poll while open, agent bubbles left/green + provider right/tint, error + retry + empty states. `ProviderProfileScreen`/home shell now thread `disputeController` through. **Live smoke:** provider start → send → admin reply via HMAC `POST /admin/complaints/:id/messages` (signed as `admin-backoffice-service`) → provider sees both messages; re-start reuses the same complaint id.
- Note: agent replies come from the admin surface (no admin UI yet — Postman collection's admin folder auto-signs).

**D. iOS push prep (everything except the Apple account).**
- Registered both iOS apps in Firebase `logistics-93b9f` via the Management REST API (service account `credentials/firebase.json`; throwaway python script): customer `com.cosmicforge.logistics.customer` → `1:461709902538:ios:72471c4f0f275502876a33`, truck provider `com.cosmicforge.logistics.truckprovider` → `1:461709902538:ios:4988156b53cad8e7876a33`.
- `GoogleService-Info.plist` downloaded into both `ios/Runner/` and added to both Xcode projects (pbxproj file ref + Resources build phase, plutil-linted); iOS options added to both hand-maintained `lib/firebase_options.dart` (the iOS case no longer throws); `Runner.entitlements` (aps-environment=development) created + `CODE_SIGN_ENTITLEMENTS` on all Runner configs; `UIBackgroundModes: remote-notification` in both Info.plists; iOS deployment target bumped 13.0 → 15.0 (Podfile `platform :ios, '15.0'` + `IPHONEOS_DEPLOYMENT_TARGET`) because firebase_core requires 15.0. `FirebasePush` needed no change (platform-neutral).
- **Both apps compile: `flutter build ios --no-codesign --debug` ✓** (customer needed one retry after a transient pod-clone network error).
- **Remaining manual steps documented in `docs/ios-push-remaining-steps.md`** — needs an Apple Developer account (not yet purchased): register bundle IDs, create APNs auth key, upload to Firebase Cloud Messaging, set signing team, device test. ~10 min once the account exists.

**Verification:** payment-wallet `go build/vet/test` green; truck_provider `flutter analyze` clean + all 29 tests pass; customer `flutter analyze` 7 pre-existing infos + all 31 tests pass; both iOS builds green; PIN + chat smoke-tested live against running services (stale pre-session payment-wallet process on :8105 was killed and restarted — it predated the PIN routes).

**Still pending (device-blocked, no adb device attached):** (1) dispatch rider app screen-by-screen walkthrough (seeded riders `+2348077770001/2`, OTP in service logs; bootstrap + services ready); (2) on-device visual sign-off of Saved Locations + Bike Delivery screens. Both are ready to run the moment a phone is plugged in.

### 2026-07-16 — Customer Notifications (Alerts) tab redesigned to Figma

**What changed (`apps/customer`):** rebuilt the Alerts tab to the Figma notifications mockup — in-body "Notifications" title + "Manage your Notifications and Activities here." subtitle (AppBar removed), notifications grouped by day (Today / Yesterday / N days ago / `d MMM` beyond a week), each item a white rounded shadow card with a leading green dot (solid `primary` = unread, `primarySoft` = read), message text (dark w600 unread / muted read), and right-aligned 12-hour time ("5:21 PM"). Mirrors the provider app's Figma-2119 screen pattern.

- `lib/features/home/ui/tabs/customer_notifications_tab.dart` — rewritten (grouped card list, pull-to-refresh, empty/loading/error state preserved).
- `lib/features/notifications/state/notification_controller.dart` — per-item unread tracking added: persisted last-seen marker (`customer.notifications.last_seen` in `flutter_secure_storage` — the app has no shared_preferences dep; reads/writes are try/catch best-effort so plugin-less widget tests degrade to "all unread"), `isUnread(item)`, and `openFeed()` (loads cutoff → advances marker → `markAllRead()` → `loadFeed()`). Items newer than the previous Alerts-tab visit render unread for the whole visit.
- `lib/features/home/ui/customer_home_screen.dart` — Alerts nav tap now calls `openFeed()` (replaces `markAllRead()` + `loadFeed()`); the tab's `initState` (IndexedStack, fires once) calls it too.

**Verification:** `flutter analyze` — only the 7 pre-existing infos; `flutter test` — all 31 pass. On-device (adb A7LQCP4914404983): debug APK deployed, logged in via email debug OTP, Alerts tab screencapped and compared against the mock — grouped "Today" cards, times, and read styling all match; injected a fresh notification row directly into notification_service DB (port 5438) → re-opened tab → solid-green unread dot + bold text confirmed, then deleted the test rows.

### 2026-07-17 — Truck provider Safety & Emergency (full stack, Figma-matched)

**What:** Built the Profile → "Safety & Emergency" section in `apps/truck_provider` (was a ProviderComingSoonScreen stub): landing screen (Emergency Contact / Guarantor Information rows), each with a saved-card list view ("+ Add New" pill, name/+234 phone/relationship card) and an add-edit form (Full Name, 🇳🇬 +234 digits-only phone, Relationship Type dropdown — emergency only; Save pill pale-green while disabled). Reuses `PUT /provider/profile` — no new endpoints.

**Backend (`services/driver-hauling-service`)** — the five `truck_providers` columns (`guarantor_name/phone`, `emergency_contact_name/phone/relationship`, from migration 003) were already accepted by `PUT /provider/profile` (preserve-on-empty) but **write-only**. Extended the read path:
- `provider_auth/models/provider.go` — 5 fields on `Provider` + `PublicProvider` (JSON tags match the request DTO names) + `Public()`.
- `provider_auth/repositories/postgres_provider_repository.go` — the 4 inlined identical column lists (UpsertByPhone/UpsertByEmail/GetByID/UpdatePhone) hoisted into a package `providerColumns` const with the 5 columns appended; `scanProvider` extended.
- `provider_profile/repositories/postgres_profile_repository.go` — same 5 columns appended to its `providerColumns` const (covers UpdateProfile/UpdatePhoto RETURNING) + its `scanProvider`.
- Test: `profile_service_test.go` `TestUpdateProfile_SafetyFieldsTrimmedNotNormalized` — guarantor/emergency phones are only trimmed, never phone-normalized, so **the app must send the final `+234...` string itself**.

**Flutter (`apps/truck_provider`):**
- `auth/models/provider_auth_models.dart` — `TruckProvider` +5 fields, `fromJson`/`copyWith`, `hasEmergencyContact`/`hasGuarantor` getters.
- `profile/data/provider_profile_api.dart` — `updateProfile` +5 optional params (empty string = server no-op).
- `profile/state/provider_profile_controller.dart` — `saveEmergencyContact({name, phone, relationship})` / `saveGuarantor({name, phone})` via `_runUpdate` (send current first/last name — backend requires non-empty `first_name`).
- New `profile/ui/widgets/provider_safety_widgets.dart` — `SafetyFieldLabel` (regular-weight labels per mockup), `SafetyPhoneField` (gray field: flag + chevron + "(+234)" + digits-only), `SafetyAddNewPill`, `SafetyContactCard`, `SafetyHelperText`, `safetyFullPhone` (strip leading 0 → `+234...`) / `safetyLocalDigits` helpers.
- New screens: `provider_safety_emergency_screen.dart` (landing), `provider_emergency_contact_screen.dart`, `provider_guarantor_screen.dart` (no relationship field — backend has no guarantor_relationship column; card shows name+phone only, deliberate deviation from the mockup card). List vs form driven by `hasEmergencyContact`/`hasGuarantor` + local `_editing`; back arrow in edit mode returns to the card (pops only from list/first-time form); saved values read from `profileController.profile` via `AnimatedBuilder`.
- `provider_profile_screen.dart` — menu item rewired from coming-soon to `ProviderSafetyEmergencyScreen`.

**Verification:** hauling `go build/vet/test` green (incl. new test). `flutter analyze` — No issues; `flutter test` — all 29 pass. Live smoke on :8104 (restarted service, seeded provider `+2348011111001` via debug OTP): PUT emergency fields → echoed; GET returns them; PUT guarantor-only → GET shows guarantor saved **and** emergency preserved (preserve-on-empty confirmed). **Pending: on-device visual pass (no adb device attached this session).**

### 2026-07-17 — Truck provider Payments & Withdrawals (full stack)

**What:** Built the profile "Payment & Withdrawals" section (was coming-soon) to the Figma mockups: Linked Accounts list with delete, Add Bank Account form ("Enter New Bank Details"), and a persisted **Set Default withdrawal Account** — plus backend support in payment-wallet-service (the money owner). The withdrawal confirm screen's "default account" copy is now backed by a real persisted default.

**Backend (`services/payment-wallet-service`):**
- `migrations/003_bank_account_default.sql` — `provider_bank_accounts.is_default` + partial unique index (one default per provider) + backfill (newest active account per provider becomes default).
- Repo: `SetDefaultProviderBankAccount` (tx clear+set, active-only, NotFound otherwise), `RemoveProviderBankAccount` (**soft-delete** `status='removed'` — withdrawals hold a NOT NULL FK), shared `selectBankAccountSQL()`; `ListProviderBankAccounts` now active-only ordered `is_default DESC, created_at DESC`; `GetProviderBankAccount`/`getBankAccountForUpdate` filter active (removed accounts can't be withdrawn to). Existing `ON CONFLICT` upsert in Create reactivates a re-registered removed account.
- Usecases (`bank_accounts.go`): new narrow `BankAccountStore` interface (mirrors `TransactionPinStore`; `Options.BankStore` defaults to Repository) so rules are unit-testable. `SetDefaultBankAccount`; `DeleteBankAccount` (deleting the default promotes the newest remaining account); `RegisterBankAccount` marks the first/only active account default via `ensureDefaultBankAccount`.
- Routes (provider bearer): `DELETE /provider/bank-accounts/:id`, `PUT /provider/bank-accounts/:id/default`.
- Tests `bank_accounts_test.go` (fake store + fake Paystack): first-default, switch, delete-promotes, re-register-reactivates, NotFound, ownership scoping. `go build/vet/test` green.

**Flutter (`apps/truck_provider`):**
- `wallet_models.dart` `ProviderBankAccount.isDefault`; `provider_wallet_api.dart` `deleteBankAccount`/`setDefaultBankAccount` + `_delete` helper.
- `provider_withdrawal_controller.dart`: default seeding is now `default-flagged ?? first` (`_defaultAccount`); new `loadAccounts()`, `deleteAccount()` (re-fetches so server-side promotion reflects), `makeDefault()` (optimistic flip, rollback on error).
- New `ui/provider_payments_screen.dart` — the Figma screen: header (WalletFlowAppBar gained optional `subtitle`), Linked Accounts cards + red trash (confirm dialog via `showProviderConfirmDialog`), green "Add Bank Account" pill, Set Default section with radio cards; loading/empty/error states.
- `provider_add_bank_account_screen.dart` restyled to "Enter New Bank Details": auto-resolves on bank + 10 digits, read-only Account Name field fills from Paystack resolve (spinner → name + check), single Save pill (pale disabled → solid).
- Wiring: `ProviderProfileScreen` takes `withdrawalController` (threaded from home shell); menu item pushes `ProviderPaymentsScreen`.
- Tests: new `test/payments_screen_test.dart` (5 tests: default preselection, list render, delete → DELETE + refresh, tap → PUT default, add-form auto-resolve + save). `flutter analyze` clean; all 34 tests pass.

**Verification:** migration applied via bootstrap; service restarted on :8105 (stale pre-session process killed); curl smoke with devtoken — set/switch default, list order, delete-promotes, 404 on removed/foreign-provider, all pass. **On-device (adb A7LQCP4914404983):** deployed debug APK, walked Profile → Payment & Withdrawals: empty state, add form, populated list (2 seeded accounts for Emeka), tapped set-default (radio flip + DB row flipped), deleted an account (dialog → soft-removed in DB, snackbar). All screens match the mockups.

**Note:** the two dev accounts seeded for Emeka (`RCP_dev_a/b`) have fake Paystack recipient codes — real transfers against them would fail at Paystack initiate; register real accounts via the app once a Paystack secret is configured.

### 2026-07-19 — Dispatch stack brought to hauling-stack integration parity (4 phases, full stack)

Audit found the dispatch stack far behind hauling: no notification pipeline at all, rider app couldn't learn about new requests (manual pull-to-refresh only), support-dispute broken for riders (secret mismatch + missing identity endpoint), withdrawals 422 (PIN never sent), several stubbed screens. All fixed by cloning the hauling/truck-provider patterns.

**Phase 1 — Notification pipeline (backend + both apps):**
- `driver-dispatch-delivery-service`: new `booking/clients/notification_client.go` (BookingNotifier clone, `sourceService=driver-dispatch-delivery-service`, recipient-aware idempotency `bookingID:recipientID`, noop when unset). Emissions: booking service (accepted/unmatched/completed/cancelled-by-provider/provider-cancelled via `WithNotifier`) + trip service (`CustomerNotifier` iface + `WithNotifier`: en_route_pickup on the auto location-update transition, arrived, picked_up on StartTrip, delivered on SubmitProof). One emitter per event. New `request/notification_client.go` `NotificationServiceSender` replaces the `LoggingNotificationSender` FCM stub (booking.matched, priority high, idempotency on inbox_id; errors returned so asynq retries). New `internal/features/notifications/http` proxy (clone of hauling's `NewHandlerFor`): `/api/v1/provider/notifications` (+realtime-token,+devices) behind sharedauth `dispatch_provider`/`dispatch`, `/api/v1/customer/notifications` behind the customer signer; skipped when `DISPATCH_NOTIFICATION_URL` unset. Config `DISPATCH_NOTIFICATION_URL/SECRET`; notification-service `NOTIFICATION_SERVICE_SECRETS` += `driver-dispatch-delivery-service=development-dispatch-notification-secret`. Dispatch rider tokens verified sharedauth-compatible.
- Customer app: `dispatch_api.dart` `fetchRealtimeToken` (builds against the service ORIGIN — the base URL includes the `/api/v1/customer/dispatch` prefix; `_sendJson` gained `overrideUri`); `DispatchBookingController` gained `realtimeListenerFactory` + `_startRealtime/_onRealtimeEvent` (reuses hauling's `CustomerRealtimeListener`); wired in `customer_app.dart` (the "no customer realtime channel yet" comment is gone).
- Rider app (`apps/dispatch_provider`): registered in Firebase `logistics-93b9f` via Management REST API (appId `1:461709902538:android:b8e207f3a842ebcf876a33`), `google-services.json` + hand-maintained `lib/firebase_options.dart` (Android-only) + gradle google-services 4.4.3 + `firebase_core`/`firebase_messaging`/`web_socket_channel` deps. New `lib/features/notifications/`: `firebase_push.dart` (failure-tolerant clone), `rider_notification_api.dart` (paths `/api/v1/provider/notifications...` on the bare origin), `rider_realtime_listener.dart`, `rider_notification_controller.dart` (unread marker in flutter_secure_storage `dp_notifications_last_seen`), `notifications_screen.dart` (Figma-2119 clone). `RequestsController` gained 4s `startPolling()` with `_pollInFlight`+`_stateGeneration` guards (generation bumped on accept/reject) gated by `shouldPoll: () => availability.isOnline`; `TripsController` gained 6s `startActivePolling()` (skips while `isActionLoading`); both started/stopped by DashboardScreen initState/dispose. App shell: realtime listener + FCM device registration + token-refresh re-register + foreground poke started on entering home, stopped on logout (`_go` hook); home-screen bell now opens the real feed via navigatorKey. Config adds `NOTIFICATION_WS_URL` (default `ws://127.0.0.1:8106/api/v1/notifications/ws` — needs `adb reverse tcp:8106`).

**Phase 2 — Support-dispute for riders:** support-dispute `SUPPORT_DISPUTE_DISPATCH_PROVIDER_TOKEN_SECRET` fixed to `development-dispatch-access-token-secret` (config default + .env/.example) and `SUPPORT_DISPUTE_DISPATCH_SERVICE_URL=http://localhost:8103`. Dispatch gained `internal/features/identity/http` (`GET /api/v1/dispatch/internal/providers/:id` behind serviceauth, resolves name/phone via profile repo `GetProviderByID`; config `DISPATCH_SERVICE_SECRETS`). Rider app: new `lib/features/disputes/` (`rider_support_api.dart` on the `/dispatch-provider` group with `service_type=dispatch`, `dispute_models.dart` with `isMine == 'dispatch_provider'`, `rider_dispute_controller.dart` — disputes reference TRIPS not wallet txns since the app has no txn-history endpoint, shared `dispute_chat_screen.dart` with 5s poll + mark-read). Stubs replaced: `log_dispute_screen.dart` (real trips picker + complaint list + submit), `dispute_detail_screen.dart` (stepper + info + chat), `live_chat_screen.dart` (find-or-create 'Live chat support' complaint → real chat). Profile "Support" and "Security" menu items were dead (`onTap: () {}`) — now wired. Config adds `SUPPORT_API_BASE_URL` (default `:8107/api/v1/support-disputes`, needs `adb reverse tcp:8107`).

**Phase 3 — Withdrawal PIN:** rider `WalletApi` gained `getPinStatus/setPin/changePin` + `pin` in the withdrawal body; `WalletController` gained `pinSet/loadPinStatus/setupPin/changePin`. `transaction_auth_screen.dart` rewritten as the real gate (loading→enter | create→confirm→save-and-withdraw; `pin_not_set` routes to create; `invalid_pin`/`pin_locked` surface inline; password mode deleted). Profile Security → Transaction Pin screens (`transaction_pin_screen`/`create_pin_screen`) wired to the same controller (change flow verifies current pin server-side).

**Phase 4 — Cleanup:** customer dispatch controller null-token now drives `handleAuthFailure()` (was silent dead-end). New dispatch `cmd/online` dev helper (seeded riders d1/d2 online: Postgres verified/online + Redis status/geo near Lagos; `--offline`; gated on `DISPATCH_RIDER_DEBUG_OTP=true`; NOTE: a rider login clears their geo entry and the live-location key has a short TTL — re-run before match tests). Deleted empty scaffolds `internal/features/{proof_of_delivery,bikes,deliveries,riders}` + root `verify_integration.go`; README scope table updated. New `RecoverStrandedBroadcasts` startup sweep (repo `ListStrandedBroadcasts`: status=broadcasting AND expired) closes the crash-between-CreateBroadcast-and-scheduleExpire hole — verified live (re-adopted a real stranded broadcast). Onboarding upload TODOs were STALE comments — the app shell already runs the full identity/face/licence/vehicle/doc upload sequence; comments corrected. Rider tests: `wallet_pin_test.dart` (4), `notifications_feed_test.dart` (3), `dispute_controller_test.dart` (4).

**Verification:** dispatch + support-dispute + notification-service `go build/vet/test` green. Customer 31/31, rider 12/12 tests; both analyzers clean. Rider debug APK builds with Firebase (needed `flutter clean` after an interrupted first build; AGP was downgraded to 8.11.1 for plugin compat). **Live E2E (all passed):** rider proxy feed/realtime-token/device-register 200s → customer books (wallet held, dev ledger credit trick: balanced entries vs `paystack_receivable`, `ledger_entries.id` is bigserial) → rider inbox + `booking.matched` in rider feed (real send, not stub) → accept → customer `booking.accepted` → location→`en_route_pickup`, arrived, start, proof (JPEG magic bytes), complete → customer feed shows all 6 lifecycle events, booking `completed/paid`, settlement landed in rider's payment-wallet (`dispatch:d1...` available_kobo>0) → separate unmatched run delivered `booking.unmatched` → dispute + support-chat filed on `/dispatch-provider` with identity snapshot "Emeka Dispatch" resolved via the new internal endpoint → PIN set/status/422-without-pin/invalid_pin all correct. On-device (adb A7LQCP4914404983): APK installed, Firebase initializes, backend reachable over adb reverse, login screen renders. **Remaining manual:** full on-device visual walkthrough (rider login → online → booking → push in tray) and a real FCM tray-notification check with the app backgrounded.

### 2026-07-19 — Dispatch dev seeder: varied rider states + booking/trip history

**What:** `services/driver-dispatch-delivery-service/seeds/dev_seed.sql` previously only seeded 2 fully-verified riders with zero booking history — much thinner than hauling's seeder. Extended it (mirroring hauling's pattern, adapted to dispatch's derived-on-read booking status and wallet-only payment method) so local testing doesn't require a full mobile-app click-through or hand-crafted curl bookings just to see populated Trips/Earnings/active-trip screens.

**3 new riders in deliberately varied states**, each hitting a distinct `checkOnlineGate` branch (`internal/features/availability/service.go`): `+2348077770003` Ngozi Onboarding (`provider_availability.verified_to_go_online=false`, no bike → `not_verified`), `+2348077770004` Tunde Suspended (`providers.verification_status='suspended'`, otherwise fully onboarded → `account_suspended`), `+2348077770005` Chidi Pending (verified provider, `bikes.verification_status='pending'` → `no_verified_vehicle`). Existing riders 1/2 (Emeka, Bisi) unchanged; the reclaim block extended to cover the 3 new phone/email/plate identifiers.

**Booking/trip history** on a fixed synthetic `customer_id` (`e0000000-...-000000000001`, not a real customer-service account — same approach hauling's seed uses): 2 completed deliveries (rider 1 and rider 2, each with a full `trips`/`trip_state_log`/`delivery_proofs`/`ratings` row set), 1 cancelled booking, 1 unmatched booking, 1 in-progress booking on rider 1 (`trips.status='in_progress'`) for the "Ongoing" bucket. Key schema finding: dispatch's booking status is derived on read (`internal/features/booking/model.go` `DerivedState.WireStatus()` — trip status wins, then booking terminal fields) so **no `request_broadcasts` row is needed** for cancelled/unmatched/completed seed rows, unlike what a naive port of hauling's pattern would assume. `ratings` writes are keyed on `booking_id` (not `trip_id` — that's the separate `customer_ratings_by_provider` table used by the *provider*-rates-*customer* direction); since the app's `avg_rating` update only happens inside the live review-submission transaction, the seed sets `providers.avg_rating`/`total_trips` via an explicit `UPDATE` after the `ratings` inserts.

**Gotcha hit and fixed:** UUIDs must be valid hex — an early draft used `g`/`h`/`i` as leading identifier characters for trips/trip_state_log/ratings, which Postgres rejected (`invalid input syntax for type uuid`). Reassigned to valid hex prefixes (`a` for trips, `c` for trip_state_log, `9` for ratings).

`cmd/online/main.go` unchanged in behavior (still only riders 1/2 — they're the only go-online-ready ones) but gained a comment explaining why 3/4/5 are intentionally excluded.

**Verification:** `go build/vet/test` clean. Bootstrap + seed run 3x in a row — row counts stable (providers=5, bikes=4, dispatch_bookings=5, trips=3, trip_state_log=16, delivery_proofs=2, ratings=2), confirming idempotency. Live-tested against a running instance: OTP login + `PATCH /provider/availability {"status":"online"}` for all 5 riders — 1/2 succeed, 3/4/5 fail with exactly `not_verified` / `account_suspended` / `no_verified_vehicle` respectively (distinct error codes, not a generic failure). `psql` spot-checks confirm all 5 seeded bookings land in the intended state.

### 2026-07-20 — Dispatch login: debug OTP shown on-screen (parity with customer/hauling)

**What:** The dispatch rider app now displays the OTP on the confirmation screen in debug builds, matching the customer + truck-provider apps. Previously dispatch printed the OTP only in the service logs (never in the HTTP response), so testing login on the device meant tailing logs.

**Backend (`services/driver-dispatch-delivery-service`):** `authusecases.StartResult` gained a `DebugOTP` field (Go tag `json:"-"` — never auto-serialized); `Start`/`SignupStart`/`LoginStart` set it only when `u.otpDebug` (gated by `DISPATCH_RIDER_DEBUG_OTP=true`). `StartResponse` DTO (`internal/features/auth/http/dto.go`) gained `debug_otp` with `omitempty`; the three handler call sites copy `result.DebugOTP` through. The old two "OTP never in response" handler tests were split into a debug-disabled variant (still asserts the OTP never appears) + a debug-enabled variant (asserts a 6-digit `debug_otp` is present) via a new `buildSLRouterWithDebug(..., otpDebug)` helper. `go build/vet/test` green; live-verified `POST /auth/login/start` returns `debug_otp` with the flag on.

**Frontend (`apps/dispatch_provider`):** `AuthStartResponse` gained `debugOtp` (parses `debug_otp`); `DispatchAuthController` exposes a `debugOtp` getter set from `loginStart`/`signupStart` success and cleared on logout; `otp_verification_screen.dart` renders the same green code box as the customer app, gated on `kDebugMode && controller.debugOtp != null` (never in release). Also prefilled the login field with seeded rider `+2348077770001` under `kDebugMode` for faster local testing. `flutter analyze` clean; all 12 tests pass.

### 2026-07-20 — Truck-provider Profile redesigned to match the dispatch Profile

**What:** Reworked the truck-hauling provider Profile tab to mirror the dispatch provider Profile design. The two screens were already ~90% identical (same header/subtitle, avatar+name+stat-lines, the same 8 menu destinations, delete card); this ports the dispatch page's distinguishing elements. Confined to one file — `apps/truck_provider/lib/features/profile/ui/provider_profile_screen.dart` — with all navigation/controllers/data wiring preserved.

**Decisions (with user):** stats card is **frontend-only, 2 cells (Trips + Rating)** — the hauling backend exposes only `rating`+`totalTrips` per provider (no `/provider/stats` like dispatch has); and **keep the truck brand palette** (`kProviderGreen 0xFF22A84A` etc.) rather than dispatch's `#4CAF50`, so the tab stays consistent with the app's other tabs.

**Changes (mirroring dispatch's `profile_screen.dart` components, in truck colors):** scaffold bg → `kProviderSurface`; avatar → 100px with a green-tinted initial-letter fallback; new provider-ID row (`KG-XXXXXXXX` derived client-side from the provider UUID, no backend field) with a copy-to-clipboard button; new 2-cell stats card (`_StatsCard`/`_StatCell`/`_StatDivider` — Trips thousands-separated + Rating with an amber star / "No ratings yet"); menu rows restyled from shadowed cards to flat bordered pills (`kProviderBorder`, radius 14, icon 20, label 14/w600). Preserved: 4 constructor params, `initState` load + re-load after `_open`, `AnimatedBuilder`/`RefreshIndicator`, all 8 menu destinations + nested nav, `authController.logout()`, `_DeleteAccountCard`.

**Verification:** `flutter analyze` clean; `flutter test` — all 34 pass. On-device (adb `A7LQCP4914404983`): built + installed + logged in as seeded hauling provider `+2348011111001` (debug OTP shown on-screen) and screencapped the full Profile top-to-bottom — avatar/`KG-A0000000` copy row/`37 Trips | ★ 4.90 Rating` card/8 bordered rows/red Log Out/Delete card all render in the truck green; confirmed Profile Information still navigates.

### 2026-07-20 — Removed the account-type selection screen (customer + truck provider apps)

**What:** Both `apps/customer` (`ServiceChoiceScreen`, "How do you want to use Cosmicforge Logistics?") and `apps/truck_provider` (`AccountTypeScreen`, same copy) showed an account-type picker right after OTP verification, before the rest of onboarding. In both apps the selected value was dead data — never sent to any backend API or persisted, only used to gate the screen's own Continue button. In the truck app specifically, only "Drive, Deliver or Haul" ever proceeded; the other two options showed a blocking "wrong app" dialog — so the screen was a guaranteed dead-end for 2 of its 3 choices in a driver-only app. Removed from both apps; `apps/dispatch_provider` has an equivalent screen too but was left untouched (out of scope of this request).

**Customer app:** deleted `service_choice_screen.dart`. `CustomerAuthController`: removed `CustomerAuthStatus.serviceChoice`, the dead `selectedService` state field, and `selectServiceChoice()`/`goToServiceChoice()`; `_setAuthenticatedSession` now routes `requiresProfile` straight to `profileDetails`. `customer_app.dart`: removed the import + switch case. `profile_details_screen.dart`: dropped the back arrow (nothing to go back to now that it's the first onboarding screen). `app_routes.dart`: removed the now-dead route constant. Updated the one widget test that walked through this screen.

**Truck provider app:** deleted `account_type_screen.dart`. `ProviderAuthController`: removed `ProviderAuthStatus.accountTypeSelection`, the dead `OnboardingFormData.accountType` field, and `selectAccountType()`/`proceedToServiceType()`; `initialize()`/`verifyOtp()` now route `needsOnboarding` straight to `serviceTypeSelection`. `truck_provider_app.dart`: removed the import + switch case. No `step:` renumbering needed — `AccountTypeScreen` never participated in the visible progress bar.

**Verification:** both apps `flutter analyze` clean, no leftover references (`grep` swept for every symbol name in both `lib/` and `test/`). Customer `flutter test` 31/31 pass; truck_provider `flutter test` 34/34 pass unchanged.

### 2026-07-20 — Dispatch seed bug: "fully verified" riders showed Unverified in the app

**What:** User reported the seeded rider prefilled on the dispatch app's login screen (`+2348077770001`, from the earlier debug-OTP-prefill change) showed as not verified after logging in, despite `providers.verification_status='verified'` in the DB. Root cause: the app's verification badge does **not** read `providers.verification_status` — `GET /provider/verification/status` (`internal/features/verification/service.go` `calculateOverallStatus`) computes its own `overall_status` from the `verification_steps` table, requiring all 5 REQUIRED steps (`identity`, `vehicle`, `face`, `guarantor`, `emergency` — `licence` is optional and excluded) to be `approved`. The seed only ever inserted a single `licence` step, so the other 5 were missing entirely → `calculateOverallStatus` fell through to `OverallStatusNotStarted`, which the app's `_StatusBadges` renders as "Unverified" (its default/unmatched case) — completely independent of the correct-looking DB column.

**Fix:** extended `seeds/dev_seed.sql` for riders 1 (Emeka) and 2 (Bisi) — the two riders documented as "fully verified" — to also insert `identity`/`vehicle`/`face`/`guarantor`/`emergency` verification_steps rows as `approved`, matching the shape `internal/features/verification/repository.go`'s `defaultStepDefinitions()`/`initializeStepsTx` would create during real onboarding (guarantor/emergency are `is_auto_confirmed=true, confirm_method='auto'`; identity/vehicle/face are manual-review). Also fixed a stale comment in the seed header claiming the debug OTP is log-only (it's been returned in the HTTP response — see the "debug OTP shown on-screen" entry above — since before this session).

**Verification:** re-ran the seed 2x — idempotent (12 verification_steps rows across both riders, stable). Live-tested against the running service: logged in as both riders via real OTP, `GET /provider/verification/status` now returns `"overall_status": "verified", "completion_percentage": 100` for both (previously would have been `"not_started"`). `go build/vet` clean (SQL-only change, no Go code touched).

### 2026-08-10 — Vendor Service / Commerce Logistics: architecture foundation only (new service scaffolded, no business features)

**Scope:** per an explicit "architecture/foundation only" spec, audited the current repository baseline, then created `services/vendor-service` as a new backend service (Vendor + Commerce Logistics: businesses, catalogues, CRM, inventory, purchasing, recipes, commerce orders, promotions, vendor-owned delivery, fulfilment planning). No business feature logic was implemented — only scaffolding, module boundaries, and documentation, per the task's own explicit restriction.

**Audit findings (see `services/vendor-service/docs/architecture/00_REPOSITORY_AUDIT_BASELINE.md` for full detail):**
- **OTP_QR_STATUS = NOT_PRESENT** — no service anywhere in the platform implements a real one-time-code/QR challenge-response for pickup/delivery custody handover. Dispatch's `SubmitProof`/`ConfirmRecipient` and hauling's `ConfirmDelivery` are all evidence-capture (photo/signature + a static receiver-phone string match), not a generated-and-verified credential. Fleet-partner-service's package `qr_value` is a static per-package barcode scanned by trusted internal devices, and its Section 19 "OTP" proof type just HMAC-hashes a caller-submitted value with no generation/verification step anywhere. Handled per the spec's own Case C: no duplicate implementation built; `internal/integrations/handover` documents the planned two-stage contract only; recorded as `PENDING_EXTERNAL_DEPENDENCY` in the new docs.
- **`fleet-partner-service` was not listed anywhere in this file** despite being the platform's largest, most mature backend service (54+ feature directories) — added a one-line entry above (Backend Services) rather than fully documenting it (out of scope here; see its own `docs/` and `README.md`).
- **`infra/docker-compose.yml` and every `scripts/*bootstrap*.sh` referenced elsewhere in this file's own session log do not exist anywhere in this repository's git history** — confirmed via `git log --all` over those paths. Not recreated (out of scope for a single-service architecture task); disclosed explicitly in the new audit doc rather than silently worked around. No service in the repo has a committed `.env.example` either; vendor-service's own was added anyway since it costs nothing.
- `payment_intents.source_service` and media-file-service's `owner_service` are both free-text at the schema layer — onboarding vendor-service as a caller is a config-only change (service-secret allowlisting) on each, not a migration. `verification-compliance-service` and `admin-backoffice-service`/`analytics-service` remain mostly/fully scaffolds. `api-gateway` has no generic routing layer yet (only a B2B "Partner API," an unrelated concept) — vendor-service exposes its own direct routes for now, matching the existing customer/hauling/dispatch precedent.
- No genuine architecture fork required stopping to ask the user — every ambiguity resolved to "follow the existing, most mature convention" once evidenced (see the audit doc's closing section).

**Built (`services/vendor-service/`):** `cmd/main.go` (wires `serviceapp.Run`, health/ready/meta, config validation, migration-on-flag); `internal/config` (full `VENDOR_*`/`COMMERCE_*` feature-flag surface from the spec, with validation rules — no orphaned flags, no flag without its required downstream credentials); `internal/database` (self-contained migration runner copied from payment-wallet-service's proven pattern — paired `.up/.down.sql`, query-driven rollback, not a hardcoded step count, avoiding a real bug class fleet-partner-service hit repeatedly); `internal/events` (transactional outbox + inbox dedupe + HTTP publisher + worker, modeled on payment-wallet-service's shape, `DisabledPublisher` default); `internal/platform/idempotency` (service-local Idempotency-Key store, adapted from fleet-partner-service's — no shared/go package exists for this anywhere in the platform, confirmed); `internal/audit` (Recorder interface, no schema yet); `internal/jobs` (ticker-based worker convention, matching the platform majority over the dispatch/api-gateway-only asynq pattern); `internal/architecture_test.go` (static guardrail: no `internal/features/X` may import `internal/features/Y/{http,repositories}`, enforced from the very first commit); `internal/integrations/{wallet,verification,media,notification,support,admin,analytics,dispatch,taxi,hauling,handover}` and `internal/features/{auth,vendors,capabilities,brands,branches,staff,crm,food,cloud_kitchen,pastry,shop,catalog,inventory,purchasing,recipes,production,orders,substitutions,promotions,delivery,fulfilment,finance_projection,analytics}` (34 reserved module-boundary packages, `doc.go` only, no logic — per the spec's explicit "do not implement full feature logic yet"); 3 foundation migrations (extension/identity marker, outbox/inbox tables, idempotency-keys table).

**Documentation created:** `services/vendor-service/docs/{README,UI_UX_REQUIREMENTS,FRONTEND_REQUIREMENTS,PLATFORM_SERVICE_CHANGE_REGISTER,API_ROUTE_REGISTER,IMPLEMENTATION_STATUS}.md` and `docs/architecture/00`–`12` (repository audit baseline, service boundaries, domain ownership, module map, cross-service integrations, payment allocation model, commerce fulfilment model, OTP/QR dependency, event ownership, security/authority model, worker/recovery model, feature-flag/rollout model, migration ownership).

**Verification:** `go build ./...`, `go vet ./...`, `gofmt -l .` clean; `go test ./...` all pass (config validation tests, static architecture-boundary test); `internal/database/migrate_test.go`'s real-Postgres migration-replay test (apply → verify → rollback → verify the down file actually dropped the table, not just untracked it → reapply → verify) run live against a throwaway database on the existing `karrygo-fleet-partner-test-db` container and passed, then confirmed to skip cleanly with no test database configured. `go vet` on `customer-service` and `driver-hauling-service` confirmed the `go.work` addition didn't disturb existing modules.

**Explicitly not done, per the task's own scope limits:** no business feature logic (menus, CRM, checkout, fulfilment optimizer, etc.); no routes (0 registered — only the free `/health`/`/ready`/`/meta` every `serviceapp.Run` service gets); no OpenAPI spec (nothing real to document yet); no changes to any other service's code (only documented future config/migration needs in `PLATFORM_SERVICE_CHANGE_REGISTER.md`); no frontend/UI work.

### 2026-08-10 (same day) — Vendor Service Section 5: Authentication, Sessions and Staff IAM — full backend implementation

**Scope:** complete production-oriented backend for passwordless OTP login, refresh sessions, step-up authentication, and Staff IAM (staff provisioning/suspension, roles, permissions, branch-scoped assignments), on top of the architecture foundation from earlier the same day. No Section 6 (Business/Brand/Branch) CRUD, no frontend, per the task's own explicit restrictions.

**Two completely separate security domains, deliberately never sharing a table/key/challenge**: Vendor AUTHENTICATION (this section) vs. the platform's logistics pickup/delivery custody handover, which the architecture-phase audit had already found `NOT_PRESENT` — that finding stands unchanged; nothing in Section 5 touches it.

**Key design decisions (each documented in-repo, not just here):**
- **Access tokens carry minimal claims only** (`shared/go/auth.Claims` unmodified — Subject/Role/Service/SessionID). Full scope (vendor id, branch, effective permissions) is resolved server-side from the session/staff tables on **every** authenticated request via `vendorauthhttp.RequireAuth`, never trusted from the JWT or the request body. This was chosen over extending the shared `Claims` struct (used by every other service) or a permission-version claim (only invalidates at next refresh) — it makes suspension/permission changes take effect on the very next request, proven by `TestAuth_SuspendedStaffCannotRefresh`.
- **OTP challenges and sessions are Postgres-backed, not Redis**, unlike hauling's/dispatch's provider-auth pattern — a deliberate divergence justified by the spec's own migration requirements (challenge/session lookup+expiry indexes) and Section 5's audit/durability needs for a staff-admin surface. Rate limiting still uses the platform's established Redis INCR+EXPIRE idiom (`internal/platform/ratelimit`), matching hauling's provider_auth exactly.
- **No SMS channel exists anywhere in the platform** (`shared/go/notifications`' channels are exactly push/email/websocket/in_app) — a real, previously undocumented gap discovered while wiring OTP delivery. Also discovered: customer-service's existing `LoggingOTPSender` **logs the raw OTP code in debug mode**, which Section 5's own rule 11 ("raw OTP values must never be logged") explicitly forbids — deliberately not replicated. Vendor Service's `OTPNotifier` delivers real email (once `notification-service` allowlists `vendor-service` — not yet done, tracked in `PLATFORM_SERVICE_CHANGE_REGISTER.md`), returns a clear `service_unavailable` for phone, and in debug mode skips real delivery entirely (`DebugOTP` travels only in the API response, never a log line).
- **Authorization enforcement lives in the usecase layer**, diverging from the Foundation phase's HTTP-layer-RBAC plan (which was modeled on fleet-partner-service): delegation checks (can this actor grant this permission) and step-up-grant consumption both need direct repository access and must also protect the internal `ProvisionOwner` path, not just HTTP requests.
- **Privilege escalation prevented structurally**: `delegationCheck` requires the acting staff member to already hold every permission in a role before they can grant it (at a covering branch scope); `vendor.owner.transfer` is hardcoded as never-delegatable to any role. High-risk permission changes (bank-account change, ownership transfer, sensitive data export) require a fresh step-up grant even for the Owner.

**Built:** migrations `000004_vendor_auth_iam` (10 tables: `vendor_users`, `vendor_staff`, `vendor_otp_challenges`, `vendor_sessions`, `vendor_step_up_grants`, `vendor_permissions`, `vendor_roles`, `vendor_role_permissions`, `vendor_staff_role_assignments`, `vendor_audit_log`) and `000005_vendor_default_permissions_and_roles` (35 seeded permissions, 11 seeded system role families — Owner/General Manager/Branch Manager/Kitchen-Pastry/Shop-Inventory/Procurement/CRM-Support/Marketing/Finance/Vendor Rider/Analyst-Auditor — with full role-permission mappings). `internal/features/auth/{models,repositories,usecases,clients,http}` (OTP state machine, sessions, step-up, notification client, HTTP handlers/middleware). `internal/features/staff/{models,repositories,usecases,http}` (staff/roles/permissions/assignments, authorization evaluator, HTTP handlers). `internal/platform/ratelimit` (new), `internal/platform/authctx` (new — shared request-context carrier so `auth` and `staff` never import each other's `http` packages, respecting the Foundation phase's `internal/architecture_test.go` guardrail). `internal/audit/postgres_recorder.go` (real implementation, replacing the Foundation phase's `NoopRecorder`-only state). `cmd/adapters.go` (cross-feature port adapters — composition-root pattern). `cmd/main.go` fully wired: 21 routes (5 public auth, 2 authenticated step-up, 12 authenticated staff/role/permission, 1 internal HMAC-protected provisioning route). `docs/openapi/section5-auth-staff-iam.openapi.json` + `internal/section5contract` route-parity test.

**Tests: 50 functions across 8 packages, all passing** — 11 unit (OTP state machine/session/step-up), 13 real-Postgres auth integration tests (full lifecycle, expired/resend/lockout/concurrent-verify-once, suspended-staff blocks login+refresh, multi-vendor selection without burning the OTP, step-up binding precision, **raw OTP never persisted**, **raw OTP never logged**), 8 real-Postgres staff/IAM integration tests (**tenant isolation/IDOR**, **privilege-escalation prevention**, **branch-scope-escape prevention**, **role-name-alone-does-not-authorize**, **high-risk-requires-step-up**, duplicate-assignment prevention, suspend/reactivate version-conflict+audit), 4 real HTTP contract tests (full login→list-staff through the actual production router), 2 OpenAPI/route-parity tests, extended migration-replay/schema tests.

**A real bug found and fixed by the test suite**: the seeded Branch Manager role initially granted only `vendor.staff.read`, not `.manage` — silently contradicting the spec's "branch staff where permitted." Caught by a test asserting in-branch delegation should succeed; fixed in migration `000005` before it was committed (same session).

**A real test-infrastructure finding, not a product bug**: running `go test ./...` normally (packages parallelized) against one shared external test Postgres causes DDL races (`CREATE TABLE IF NOT EXISTS` is not safe against concurrent execution from multiple processes) and cross-package data leftovers. Every package passes cleanly in isolation; the full suite passes cleanly with `go test -p 1 ./...` (serialized) against one fresh database. Documented in `IMPLEMENTATION_STATUS.md` so a future CI setup doesn't rediscover this the hard way.

**Verification:** `go build ./...`, `go vet ./...`, `gofmt -l .` clean. `go test -p 1 ./...` — 50/50 tests pass, 0 skips, against real Postgres (`karrygo-fleet-partner-test-db`, throwaway databases created/dropped this session) and real Redis (`fleet-partner-redis-test` container).

**Remaining gaps, disclosed (not this section's own code to fix):** `notification-service` allowlist for `vendor-service` not yet added (config-only, tracked in `PLATFORM_SERVICE_CHANGE_REGISTER.md`); phone-number OTP delivery has no real channel anywhere in the platform (a newly-documented platform-wide gap, not Vendor-Service-specific); audit writes are not yet wrapped in the same transaction as their triggering mutation (disclosed in `architecture/09`).

### 2026-08-11 — Vendor Service Section 7: Onboarding, KYB, Verification and Activation

**Scope:** implemented the complete backend onboarding aggregate and activation gate without starting Section 8 or adding frontend code. Vendor Service now owns onboarding progress, the authorized representative, document references, safe verification projections, bank-profile projections, blockers, readiness, and activation orchestration; `verification-compliance-service` remains authoritative for verification decisions, `payment-wallet-service` remains authoritative for resolved bank details, and `media-file-service` remains authoritative for private document objects.

**Built:** Vendor migrations `000008_vendor_section7_onboarding` and `000009_vendor_section7_permissions`; 19 public Vendor endpoints and one HMAC-protected verification callback; six verification dimensions (`IDENTITY`, `BUSINESS`, `BANK`, `LOCATION`, `BRAND`, `FOOD_KITCHEN`) with exact status/source-version handling; document metadata/reference flows that never ingest binary content; expected-version and step-up protected bank replacement; data-driven activation requirements; transactional business activation, audit, and outbox writes; expiry and durable verification-reconciliation workers; low-cardinality Section 7 metrics. Added four internal case/read/decision routes plus a durable callback outbox worker to `verification-compliance-service`, and three safe bank-resolution routes to `payment-wallet-service`. Existing Media private-object HMAC v2 support was reused and documented; Admin, Notification, and Gateway code was intentionally unchanged after the preflight audit.

**Security and recovery:** every Vendor public operation derives tenant and branch scope from the authenticated session; request DTOs cannot set authority-owned statuses, source versions, or business IDs. Verification callbacks require the exact service caller, HMAC v2, timestamp freshness, durable nonce consumption, event-id deduplication, and monotonically increasing source versions. Representative attachment is protected by a composite tenant foreign key. Verification submission fails closed before local mutation when integration is disabled; configured remote ambiguity becomes a durable `UNKNOWN` operation reconciled with leasing, backoff, dead-letter, and replay. Concurrent bank replacement is serialized with a per-business PostgreSQL advisory transaction lock before consuming step-up or calling Wallet.

**Verification:** full real-Postgres/Redis Vendor suite passed with `go test -p 1 ./... -count=1`; the full real-Postgres verification-compliance and payment-wallet suites passed; migration rollback/reapply, OpenAPI route parity, tenant/branch isolation, callback replay, stale/duplicate/expiry handling, activation matrix, and concurrent bank replacement were exercised. Builds and vets passed for all changed Go modules, shared Go tests passed, OpenAPI JSON validated, `git diff --check` passed, and the no-frontend gate passed.

**Remaining dependencies:** production deployment must provide/allowlist the Media, Wallet, Verification callback, and Notification service credentials/routes. The Admin operator review product is still absent, Notification still has no SMS channel, and generic Vendor domain outbox consumers remain future platform work. The older Section 5/6 audit-after-commit limitation remains documented; Section 7 mutations use transactionally atomic audit/outbox writes.

### 2026-08-11 — Section 7 targeted contract correction

**Correction:** audited every Section 7 `PENDING` occurrence before editing. Vendor and Verification already serialized the intended value `PENDING_REVIEW`; the completion report had misstated it. The ambiguous Go identifier was renamed to `VerificationPendingReview`, strict tests now reject plain `PENDING`, and exact seven-value checks were added to both Vendor and Verification history tables. Unrelated outbox-delivery and reconciliation-operation `PENDING` states were preserved.

**Callback route audit:** `/api/v1/vendor/internal/vendor/v1/verification-events` was accidental composition of the public `serviceapp` base and the canonical internal namespace. Added an explicit root registration hook and moved the callback to `POST /internal/vendor/v1/verification-events`, matching `/internal/<domain>/v1`; Verification's signed worker, OpenAPI, route tests, and documentation were synchronized. The route remains absent from API Gateway and protected by HMAC v2, exact caller allowlisting, timestamp freshness, durable nonce consumption, event dedupe, and source-version guards.

**Verification:** fresh real-Postgres/Redis Vendor and fresh real-Postgres Verification suites passed, including Section 7 migration rollback/reapply and direct constraint assertions, `PENDING_REVIEW` callback persistence, activation blocking for business/branch/branch-capability scopes, stale versions, duplicate events, Sections 5/6 regression, OpenAPI parity, vet, build, and shared tests. No frontend or Section 8 work was added.

### 2026-08-11 — Vendor Service Section 8B: Tags & Advanced Segmentation Engine

**Scope:** vendor-defined customer tags plus the segmentation engine — a safe JSON rule DSL over a closed field registry, static and dynamic segments, immutable definition versions, privacy-safe preview, durable materialization with atomic generation swap, manual exclusions, and Customer 360 integration. Built on the accepted Section 8A baseline (verified first: 80 routes, migration tip `000011`, full Sections 5-8A suite green on a fresh database before any code was written). Sections 8C-8I were not started.

**Built:** migrations `000012_vendor_section8b_tags_segments` (7 tables) and `000013_vendor_section8b_permissions` (5 permissions); 21 public routes, 0 internal (cumulative Vendor surface **101**); `internal/features/crm/{tags,segment_rules,segment_registry,segments,segment_membership,segment_eval,segment_privacy,segments_http}.go`; `docs/openapi/section8b-crm-tags-segmentation.openapi.json` + `internal/section8bcontract`; a lease-based materializer worker wired in `cmd/main.go`.

**The load-bearing design decisions:**
- **The rule language is a closed registry, not a parser.** A condition can only name a field defined in `NewSegmentFieldRegistry`, and each field's SQL is a hand-written Go function. Field keys, operators and values never become SQL text; values are always bind parameters. No free-text filter, no user-chosen sort column, no escape hatch. An unknown field and a forbidden field return the same error so a rejection cannot map the schema.
- **Unknown is not zero.** Every commercial condition compiles to `CASE WHEN <observed> THEN (<expr>) ELSE NULL END` with the rule wrapped in `IS TRUE`, so SQL's own three-valued logic makes an unobserved relationship match neither `>= 2` nor `< 2`, and a `NOT` over it does not resurrect it.
- **A failed rebuild never empties an audience.** The new generation is built alongside the live one and swapped in one transaction that also re-sweeps for relationships redacted mid-run; a failure before the swap leaves the previous membership intact and the segment reports `STALE`, not zero.
- **Redaction wins every race, in both commit orderings** (bulk assignment locks profile rows; the swap re-sweeps; the privacy handlers run inside the recording transaction).
- **Section 8A was extended, not modified.** Both new privacy domains register through the existing `PrivacyHandler` port (which also gives them merge handling via `ApplyIdentityLink`), and Customer 360 gained one composition call. `ApplyPrivacy` is untouched.
- **Consent eligibility exists in Go and in SQL, and a real-Postgres test asserts the two agree** over allow/deny/unknown/expired relationships, so drift fails the build rather than mis-targeting customers.

**Two real bugs the tests caught pre-commit:** the canonical rule form did not re-parse (values serialised as arrays, durations as raw seconds) so stored definitions would have failed revalidation at rebuild time; and a bigint parameter first used inside a text concatenation made Postgres infer `text`, breaking the generation insert. The evaluation clock also moved from the database's `now()` to the service clock so runs are reproducible.

**Verification:** `go build ./...`, `go vet ./...` clean; **246 top-level tests, 0 failures, 0 skips** with `go test -p 1 ./... -count=1` against real Postgres + Redis on a freshly created database, including a 1,200-relationship scale suite (batch-boundary correctness, pagination completeness, preview count budget, bulk at the 500 ceiling, a rule at the complexity ceiling, and the real query plan). Migration fresh replay, 8A→8B upgrade, rollback and reapply all pass, with an explicit assertion that an 8B rollback leaves Section 8A's tables and permissions undamaged.

**One pre-existing test legitimately updated:** 8A's `TestCustomer360ReportsFutureModulesAsUnimplemented` asserted `tags`/`segments` report `NOT_IMPLEMENTED`, which stopped being true. It now asserts they report `AVAILABLE` and that 8C-8H still do not. The Section 7/8A migration rollback tests also stopped naming later sections by hand and now walk down to their own first version through a shared helper, so future sections never need to edit them.

**External services: NO CHANGE, all of them.** Segmentation reasons entirely over Vendor-owned data. Notably nothing was added to notification-service: this section builds audiences and sends nothing.

**Inherited dependencies (unchanged from 8A, none new):** no vendor commerce order publisher, so commercial rules match nobody; no consent authority, so eligibility rules match nobody — both fail closed by design. Section 8B's own flags default off, and `VENDOR_CRM_SEGMENTATION_ENABLED` requires `VENDOR_CRM_ENABLED`, which `VENDOR_CRM_SEGMENT_MATERIALIZATION_ENABLED` in turn requires, validated at startup.

### 2026-08-12 — Vendor Service Section 8E: Campaign Intelligence & Audience Engine

**Scope:** the Vendor-owned CRM campaign definition and audience engine — campaign entities, immutable campaign versions, segment-backed audience selection, privacy-safe preview, durable audience preparation, immutable audience snapshots, campaign targets, per-channel snapshot-time consent evidence, deterministic experiment cohorts, and frequency/quiet-hour policy definition. Built on the accepted 5–8D baseline (verified first: 123 routes, migration tip `000017`, segment field registry 22, full suite green before any code was written). Sections 8F–8I were not started.

**The boundary the whole section is built around:** 8E answers WHO a campaign should target and freezes that answer. It cannot answer HOW or WHEN a message is delivered. There is no scheduling code, send endpoint, notification handoff, template, provider client, `scheduled_at`/`sent_at`/`delivered_at` column or delivery event anywhere in it, and three separate tests (migration, OpenAPI contract, live router) reject one structurally rather than by convention. `notification-service` is unchanged.

**Built:** migrations `000018_vendor_crm_campaign_intelligence` (6 tables) and `000019_vendor_section8e_permissions` (5 permissions); 10 public routes, 0 internal (cumulative Vendor surface **133**); `internal/features/crm/{campaigns,campaign_experiments,campaign_audience,campaign_privacy,campaigns_http}.go`; `docs/openapi/section8e-crm-campaign-intelligence.openapi.json` + `internal/section8econtract`; a lease-based audience-preparer worker wired in `cmd/main.go`.

**The load-bearing design decisions:**
- **A prepared audience is not permission to contact anybody.** Consent is evaluated twice — here as historical evidence, and again by 8F against current consent before any handoff. `send_time_recheck_required` carries `CHECK (send_time_recheck_required = true)`, so a row claiming otherwise is *unstorable*, not merely unwritten. A later consent change never rewrites the frozen evidence; the customer is protected by the mandatory recheck, not by history being edited.
- **Suppression is absence, not a state.** Privacy- and service-recovery-suppressed relationships get no target row at all and appear only in aggregate counts, so a customer who must not be contacted is missing from the only table 8F will read. That makes the channel-eligibility vocabulary exactly two values, and there is no state meaning "cleared to send".
- **Consent reuses Section 8A's single `MarketingEligibility` rule** rather than a second copy — including for the "would this relationship be ineligible even with perfect consent?" question that drives structural suppression. Fail-closed is therefore a property of the shared rule, not a convention each section re-implements.
- **Audience comes only from the 8B segment engine.** No second rule language, no SQL parameter, no customer-list upload, so one set of branch/privacy/consent walls rather than two that could disagree. A branch campaign may only use its own branch's segment — the count is the disclosure.
- **The 8B `GenerationRetainer` port, left open in that section explicitly for 8E, is now implemented**, so a segment rebuild cannot delete membership a live campaign preparation depends on.
- **Preparation reuses the 8B materializer's shape** (snapshot row = its own durable job, lease + SKIP LOCKED + heartbeat + bounded retry + dead-letter + replay hook, batch 500) rather than introducing a second worker system. A partial audience can never become current: promotion is one transaction that fences on profile rows, re-sweeps for mid-build privacy/merge/service-recovery changes, and recomputes counts from the survivors.
- **Experiments are integer basis points** totalling exactly 10000, assigned by `sha256(campaign_version, profile_ref, namespace) mod 10000` over canonical cumulative ranges — deterministic, reproducible without a stored lookup, and taking no contact data as input.

**A real correctness fix in accepted Section 8B code.** Using `GenerationRetainer` as designed exposed a genuine race: `cleanupSupersededGenerations` asked the retention question and then deleted, while a campaign could pin a generation in between — each concluding the other was not there. `releaseGeneration` now takes the evaluation row's lock before asking, and campaign enqueue takes the same lock before pinning, so the two serialize. Covered by `TestCampaignConcurrency_SegmentRebuildDoesNotStealAPinnedGeneration`.

**Two accuracy fixes found by the dead-code sweep:** the section's own idempotency-conflict code was documented as `CRM_CAMPAIGN_IDEMPOTENCY_CONFLICT` while the shared command edge actually emits `CRM_IDEMPOTENCY_CONFLICT` (spec and contract test corrected to the real behaviour); and five preview reason-code constants were dead because structural suppression correctly reuses 8A's vocabulary instead (removed, and the OpenAPI enum already described the real behaviour).

**Verification:** `go build ./...`, `go vet ./...`, `gofmt -l .` clean; `go test -p 1 ./... -count=1` green against real Postgres + Redis; migration fresh replay, 8D→8E upgrade, rollback and reapply, with explicit assertions that an 8E rollback leaves Sections 5–8D undamaged. 81 new Section 8E test functions, including six deterministic lock-ordered concurrency races (no sleep-based timing anywhere) and a 1,000-relationship fixture covering batch boundaries, pagination completeness, cohort spread and index coverage.

**Three pre-existing tests legitimately updated:** the 8B, 8C and 8D migration guards asserted that no `vendor.crm.campaign%` permission existed. Campaigns stopped being future work, so they now guard the thing that is still unbuilt — any `%.send%` permission — which is the assertion that actually matters.

**External services: NO CHANGE, all of them.** Nothing outside `services/vendor-service` was touched.

**Inherited dependencies, unchanged and disclosed:** no consent authority publishes to this platform, so with the integration flag off every channel is correctly ineligible and a prepared audience contacts nobody — the section is built to be safe in exactly that state. No vendor commerce order publisher, so commercial segment rules still match nobody. `VENDOR_CAMPAIGN_CONTACT_HISTORY = NOT_PRESENT`, so frequency capping is stored and honestly reported as unevaluable until 8F has delivered something. `EXTERNAL_SERVICE_RECOVERY_SUPPORT = NOT_PRESENT`; suppression uses the Vendor-local 8C signal only (an OPEN/IN_PROGRESS task linked to a SERVICE_FOLLOW_UP interaction) and no support-dispute state is read or fabricated.

### 2026-08-13 — Vendor Service Section 8G: Corporate Customer CRM

**Scope:** the Vendor-owned commercial relationship with organizations — corporate accounts with a vendor-local `CORP-` reference, lifecycle, business/branch-set scope, contacts referencing Section 8A customer relationships, memberships, the organization's own locations, cost-centre labels, an account manager, and corporate interactions/tasks reusing the Section 8C engine. Built on the accepted 5–8F baseline (verified first: 143 routes, migration tip `000021`). Sections 8H–8I were not started.

**The boundary the section is built around:** 8G records WHO a vendor deals with. It cannot express an order, invoice, credit line, budget, contract, quotation, wallet, catalogue entitlement or approval — no route, permission, schema, column or state, and three separate tests (migration, OpenAPI contract, live router) reject one structurally rather than by convention. A cost centre is a label with no budget/limit/currency/balance; `payment-wallet-service` stays the only financial authority.

**Built:** migrations `000022_vendor_crm_corporate_customers` (7 tables + additive 8C subject support) and `000023_vendor_section8g_permissions` (5 permissions); **41 public routes, 0 internal** (cumulative Vendor surface **184**); `internal/features/crm/{corporate,corporate_sites,corporate_contacts,corporate_activity,corporate_privacy,corporate_http}.go`; `docs/openapi/section8g-crm-corporate-customers.openapi.json` + `internal/section8gcontract`.

**The load-bearing design decisions:**
- **Section 8C was extended, not duplicated.** `crm_interactions`/`crm_tasks` gained a nullable corporate subject under a `CHECK` permitting exactly one subject per row; Go structs kept `string` fields with SQL `COALESCE`, so **no accepted 8C call site changed** and the full 8C suite passes unaltered (`ok … 89.9s`). A parallel corporate activity table set would have been two places for `OPEN/IN_PROGRESS/COMPLETED/CANCELLED` and for "immutable history" to drift apart.
- **A contact is a reference, not an address book entry.** Contacts point at 8A relationships and carry a non-identifying `display_label` resolved at read time. No email/phone/address is stored or returned; the only address in the section is the customer organization's own site record. A redacted relationship keeps the link for continuity but is `privacy_suppressed`/`actionable:false` with the label withheld.
- **Privacy and identity merge reach corporate contacts through the existing 8A `PrivacyHandler` port** — the third section to extend 8A through its ports rather than by editing it. `ApplyPrivacy` untouched. Merge resolves collisions by archiving the duplicate, moving its memberships and promoting the survivor when the source was primary.
- **Isolation is structural**: `(id, business_id)` for tenancy and `(id, business_id, corporate_customer_id)` for nested resources, so Postgres refuses a membership naming another account's contact even if handler code were wrong. 26 tenant-composite FKs asserted directly.
- **No worker.** Every multi-row consequence is in the causing transaction (archive cancels open tasks; contact archive deactivates memberships), so nothing is left for a sweeper.
- **Activity permissions are 8C's, reused.** Each corporate activity route requires the 8C grant **and** `vendor.crm.corporate.read`, so an activity grant alone does not reveal which organizations a vendor works with.

**Two contract facts corrected during verification:** the section registers **41** routes, not the 42 the in-code comment claimed (memberships have no get-by-id — the list covers it), and sort/direction are allowlisted **with fallback** (the accepted 8A convention) rather than rejected, so the scale test asserts the safety property — caller text never reaches SQL — instead of an error.

**Verification:** `go build ./...`, `go vet ./...`, `gofmt -l .` clean; `go test -p 1 ./... -count=1` green against real Postgres + Redis on a fresh database. Migration fresh replay, 8F→8G upgrade, rollback and reapply, with explicit assertions that an 8G rollback restores `crm_interactions`/`crm_tasks` to `NOT NULL` and leaves Sections 5–8F undamaged. Six deterministic lock-ordered concurrency races (no sleep-based timing) and a ~100-account/300-contact/500-site scale fixture covering pagination completeness, literal search, and index coverage under `enable_seqscan = off`.

**External services: NO CHANGE, all of them.** Nothing outside `services/vendor-service` was touched.

**Inherited/disclosed dependencies:** no corporate ordering, invoicing or credit domain exists on this platform, so `MEMBERSHIP_ROLE=APPROVER` is a CRM label that authorises nothing — deliberately. Corporate accounts are **not** customer-service records: the organization exists only inside the vendor's tenancy; only its people are platform customers. Vendor commerce order facts remain absent (inherited from 8B), so no corporate spend or value is derived. The `go test -p 1` serialisation caveat from Section 5 still applies to the shared external test database.

### 2026-08-13 — Vendor Service Section 8H: CRM Intelligence, Analytics & Secure Exports

**Scope:** deterministic CRM intelligence (versioned model registry, RFM V1, Churn V1, availability/freshness states, explainability, generation-safe materialization, dependency-scoped invalidation, Customer 360 block, operational analytics, 8 new Section 8B segment fields) plus a step-up protected secure export pipeline over six closed datasets. Built on the accepted 5–8G baseline (verified first: 184 routes, migration tip `000023`, segment registry 22 fields). Section 8I not started.

**The three claims the section refuses to make, each enforced by a test:**
- **No ML, no probability.** Both models are heuristics computed in Go. No model artefact, no training data, **no floating-point column anywhere in the score schema**, no generated prose. A churn risk of 70 is a follow-up rank; the OpenAPI contract test fails if the spec ever describes it as a likelihood.
- **A score is not permission.** Consent/privacy/suppression/frequency/quiet-hours stay owned by 8A/8E/8F and are never consulted to produce a score. A real-Postgres test drives a high-churn customer who denied marketing consent through the segment engine and proves they are not selected.
- **Unavailable is not zero.** A relationship with no commercial facts gets a `NOT_AVAILABLE` row with a reason code and all-NULL numerics — a DB CHECK makes the alternative *unstorable*. Segment score predicates read through a `state='CURRENT'` subquery, so `churn.risk_score GT 70` cannot be satisfied by absence.

**Built:** migrations `000024_vendor_crm_intelligence_exports` (4 tables) + `000025_vendor_section8h_permissions` (6 permissions); **10 public routes, 0 internal** (cumulative Vendor surface **194**); `internal/features/crm/{intelligence,intelligence_eval,intelligence_read,intelligence_segments,intelligence_privacy,intelligence_http,exports,export_datasets,exports_service,exports_worker,exports_http}.go`; `internal/integrations/media/artifacts.go`; `docs/openapi/section8h-crm-intelligence-exports.openapi.json` + `internal/section8hcontract`.

**Models.** RFM V1 = scope-relative quintiles from PostgreSQL `cume_dist` (tie-equal by construction, so identical behaviour always scores identically); composite 3–15. Churn V1 = integer 0–100 from how overdue a relationship is against **its own observed cadence**, plus absolute inactivity and engagement depth; bands 0-24/25-49/50-74/75-100. Both carry a model version + definition hash over a canonical descriptor — changing a threshold *or what the model claims to do* changes its identity, and a queued run stamped with an old hash is SUPERSEDED rather than writing under it. Fewer than 2 orders → `INSUFFICIENT_HISTORY` (never high risk); cohort < 5 → `INSUFFICIENT_COHORT`.

**Segment registry: 22 → exactly 30.** Extended, not forked — a second score-segment mechanism would have been a second place for branch scope, privacy exclusion and unavailable-is-not-zero to diverge.

**Exports.** Six closed datasets × {CSV, JSONL}, versioned per-dataset field allowlists, typed filters (no column/sort/expression/SQL reachable from a client), 100k default / 250k hard ceiling + byte cap, keyset-streamed reads, lease-based generator, cancellation, retention sweeper, full audit + outbox. Privacy exclusion applied **centrally** in `buildExportQuery` so a future dataset cannot forget it. Step-up required on create *and* download, purpose-bound and single-use. `READY` has a DB CHECK requiring artifact+checksum+size+completion+expiry, so a partial file is structurally unreleasable; promotion is a guarded update a concurrent cancel wins.

**One external change, deliberately minimal — `media-file-service`.** Audit: `PRIVATE_EXPORT_ARTIFACT_SUPPORT = PARTIAL`. Its private object contract has everything (HMAC + caller allowlist + single-use nonce + private objects + short-lived download grants + retention/deletion, all config-tunable) but `validation.Detect` is signature-only and refuses sniff-less text — i.e. every CSV/JSONL — and had no data-export purpose. Added a `DATA_EXPORT` purpose and `validation.DetectTextual`, which establishes a textual type **by inspecting the bytes** (valid UTF-8, no NUL, no C0 controls beyond tab/CR/LF, no executable signature) and is reached only for that purpose. The declaration selects which claim to test, never substitutes for it: a PDF/ELF/ZIP/shell script declared as `text/csv` is still refused, and `TestDetect_StillRefusesTextForDocumentPurposes` proves the document contract is untouched. **User chose this option explicitly when asked.**

**Two clock bugs the tests caught pre-commit** (both real, both would have shipped): the artifact expiry and the export privacy fence were computed from the *service* clock and compared against *database*-written values. On any host with slight clock skew that would have published artifacts past their retention and made every export look like it raced a redaction. Both now read the database clock. A third real defect found the same way: `CUSTOMER_SUMMARY` originally lacked the redaction filter — fixed by moving the exclusion into `buildExportQuery` for all profile-bearing datasets.

**Verification:** `go build ./...`, `go vet ./...`, `gofmt -l .`, `git diff --check` clean; `go test -p 1 ./... -count=1` green against real Postgres + Redis on a fresh database. Migration fresh replay, 8G→8H upgrade, rollback and reapply with explicit assertions that Sections 5–8G are undamaged, that no status vocabulary contains `PENDING`, and that an unavailable score carrying a number is refused by the database. media-file-service `go build/vet/test` green including new textual-detection tests.

**Three pre-existing tests legitimately updated:** 8A's and 8B's Customer 360 guards asserted `intelligence` reports `NOT_IMPLEMENTED` — it stopped being true, so they now assert `AVAILABLE` and still guard the modules that genuinely remain unbuilt.

**External services: only media-file-service changed.** customer-service, notification-service, payment-wallet-service, support-dispute-service, analytics-service, api-gateway and every driver service: NO CHANGE.

**Disclosed dependency:** no vendor commerce order publisher exists, so a production deployment will legitimately report `NOT_AVAILABLE` for every score until authoritative order facts arrive. The section is built to be honest in exactly that state rather than to fill it with zeros.

### 2026-08-13 — Vendor Service Section 8I: Final CRM Integration, Hardening & Acceptance

**Scope:** the closing certification pass over Sections 5–8H. No new CRM module, **0 new routes** (public or internal), **0 new migrations**. Vendor surface stays 194, migration tip stays `000025_vendor_section8h_permissions`, segment registry stays 30 fields.

**Two production defects found and fixed:**

1. **Internal authority route mounted under the public base.** `POST /internal/staff/provision-owner` — which creates the first Owner of a vendor business — was wired with the `/api/v1/vendor` group, so it answered at `/api/v1/vendor/internal/staff/provision-owner`. Its own OpenAPI, `internal/section5contract` and `API_ROUTE_REGISTER.md` had **always** specified the engine root; Section 7 set that rule for the verification callback and this route was missed. It was HMAC-protected throughout (defence in depth, not an open door), but the rule exists so a gateway misconfiguration can't become an authority bypass. `RegisterInternalRoutes` now takes the engine and mounts at `/internal`.
2. **Notification handoff inside a DB transaction holding the privacy fence.** Section 8F called `notifier.Send` while holding `FOR UPDATE` on the delivery, the run, **and the customer profile row** — the same lock the privacy and identity-merge handlers take. A hanging notification-service could therefore block a redaction, turning one service's availability problem into another's compliance problem. Restructured to **TX1 reserve → commit → NETWORK → TX2 record**, with a `HandoffReservation` window (default 2m, must exceed the client timeout) stopping a second worker adopting an in-flight delivery. Safe because of the stable `NotificationRequestRef`: a crash-and-retry re-sends with the same idempotency key and the remote side dedupes. `NOTIFICATION_HANDOFF_TRANSACTION_MODEL` moved from **B (synchronous remote call inside DB transaction)** to **A (reserve/commit/call/record)**.

**A useful negative result from fix 2:** moving the contact-history write out of the locked transaction broke the accepted `TestCampaignExecution_ConcurrentFrequencyFenceAllowsOneHandoff` (2 handoffs, want 1) — the frequency cap counts those rows, so writing them after the profile lock released let two deliveries for one customer both pass the cap. The write was returned to the reservation transaction. The cap is now slightly conservative (a failed handoff consumes a slot), which is the right direction for a customer-protection rule.

**Added: `internal/crmcontract`, the global drift guard.** It builds the real production router from the same `Register*` helpers `cmd/main.go` calls (nil dependencies — registration attaches handlers, it never invokes them) and holds it against every OpenAPI spec and the route register **in both directions**. The per-section contract packages each pinned their own surface; nothing pinned the whole, which is the gap a live undocumented route hides in. The 194 figure is now derived from code, not counted from prose. It also caught both defects above.

**Added: cross-module certification** (`section8i_certification_test.go`). One question of the whole CRM: after a redaction, can any module still act on this person? Segmentation, intelligence, corporate contacts and exports each answer independently, plus three anti-resurrection paths (new tag, rebuild, late order fact from the authority). Also marketing-suppression semantics (records survive, contact does not) and two live-schema authority scans — no canonical customer PII anywhere in CRM, no monetary column in loyalty.

**Verification gap from 8H closed.** 8H's broad serial run overran the execution window without reporting. Measured the cause and ran deterministic groups on separate databases: **(A+C)** all 21 non-CRM packages, **(B)** the CRM package alone at **833.7s** — that is what overran — **(G)** media-file-service. All three green, **0 failures, 0 skips**.

**Verification:** `go build ./...`, `go vet ./...`, `gofmt -l .`, `git diff --check` clean for both services; media build/vet/fmt clean and its full suite green.

**External services: NO CHANGE in 8I.** The only cross-service change in the programme remains 8H's minimal media `DATA_EXPORT` addition.

**Final verdict: GO.** CRM backend implementation complete; full production activation waits on external authorities (order publisher, consent authority, notification delivery callbacks, SMS), each of which fails closed and is documented rather than faked.

### 2026-08-14 — Vendor Service Section 8 completion: audit + loyalty tiers (PARTIAL)

**What this was:** an audit of the ORIGINAL Section 8 specification against what Sections 8A-8I actually built, then implementation of the remaining gaps. **It is partial.** One requirement is complete end to end; the rest have schema but no domain code. The status doc states this precisely rather than rounding "tables exist" up to "implemented".

**Gap audit result.** Section 8A already provided most of 8.1's commercial fields (`completed_order_count`, `gross/net_spend_minor`, `average_order_value_minor`, `first/last_completed_order_at`, `last_order_ref`, `last_branch_ref`, `last_capability_type`, plus the `crm_customer_order_facts` fact-set-and-recompute model). Genuinely missing across the original text: an order count distinct from the completed count, category/item affinity, service-recovery projection, corporate commercial + RFQ projections, relationship signals, customer value band, loyalty tiers, and the automatic follow-up trigger engine.

**Migrations `000026` + `000027` (tip now `000027`).** `000026` creates the schema for every one of those: `crm_customer_order_fact_lines`, `crm_customer_category_affinities`, `crm_customer_item_affinities`, `crm_customer_support_facts`, `crm_customer_service_recovery_projections`, `crm_corporate_commercial_projections`, `crm_corporate_rfq_facts`, `crm_corporate_rfq_projections`, `crm_loyalty_tier_definitions`, `crm_customer_value_projections`, `crm_relationship_signals`, `crm_task_trigger_evaluations`, plus `observed_order_count`, order `order_type`/`corporate_customer_id`, and related-ref/trigger-provenance columns on interactions and tasks. Every projection carries an explicit availability state with a CHECK making a numeric column NULL unless the state is CURRENT — so "no producer" can never be stored as "zero". `000027` adds 3 permissions (`vendor.crm.customer_value.read`, `vendor.crm.loyalty.tiers.manage`, `vendor.crm.followup_triggers.read`), not one per projection: everything else reads through Customer 360 and the 8C task grants, which already gate it.

One additive change to an accepted table: `crm_customer_order_facts` gained `UNIQUE (id, business_id)`, because 8A never needed to reference an order fact and the completion's order lines do — tenant-composite FKs are how every other CRM table makes a cross-tenant row unstorable rather than merely unwritten.

**COMPLETE and active: 8.4 loyalty tiers** (2 routes, surface 194 -> 196). A vendor defines an ordered ladder of at most 8 thresholds; standing is **derived at read time** from the 8D ledger and never stored, so a credit, reversal, merge or threshold change lands immediately with no recalculation job. The input is **lifetime credited points net of reversed credit**, not the spendable balance — the original spec allowed either and required the choice be documented: status recognises what a customer has done, so redeeming must not demote them (a test pins both halves). That quantity is computed by summing unreversed credit-type ledger entries, **not** by reading the account's `lifetime_credited_points` column — a correction the test forced, because 8D's ledger is immutable and reversing a credit posts a compensating REVERSAL rather than decrementing the counter, so reading the counter would have let a credit posted in error confer permanent status. Ranks are derived from thresholds so the ladder cannot have two answers to "which is higher"; thresholds must be distinct; NOT_AVAILABLE / NO_TIER / CURRENT are three different answers and none of them is the bottom rung. Recognition only — no discount, entitlement or monetary column, with a live-schema scan test that fails the build if one appears. Replacement is atomic, version-checked under a per-business advisory lock, and archives superseded rungs rather than deleting them.

**NOT complete:** the domain code for affinities, service recovery, corporate commercial/RFQ projections, relationship signals, customer value band and the trigger engine; the CRM event-ingress extensions for order-line/support/RFQ facts; the segment registry expansion (still **30** fields — `completion_segments.go` defines the dependency/source vocabulary they will use); the Customer 360 blocks for those projections; and their privacy/merge/concurrency tests. Those tables are inert: nothing reads or writes them, so nothing reports a fabricated value.

**Two real defects found by the work itself.** The global drift guard from 8I flagged both new routes as undocumented the instant they were registered (which is what it is for). And a tier test caught that a customer with no loyalty account was told NO_TIER but not which rung they were climbing toward — the record answered half the question a CRM user opens it to ask.

**Verification:** `go build`, `go vet`, `gofmt -l` clean; migration replay `000001`-`000027` green, `000026`/`000027` roll back and reapply as a pair; drift guard green at 196 with OpenAPI + route register updated; 10 real-Postgres tier tests pass. **No external service changed.** Nothing committed.
