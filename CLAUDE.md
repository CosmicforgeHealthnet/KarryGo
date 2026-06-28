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
