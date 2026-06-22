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
