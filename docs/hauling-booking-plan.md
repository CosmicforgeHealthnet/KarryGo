# Truck Hauling Booking System — Implementation Plan

This document is the implementation guide for the full truck hauling booking feature: from
provider availability management through customer booking and live trip lifecycle, across
`driver-hauling-service`, `apps/customer`, and `apps/truck_provider`.

---

## 1. Overview and Core Constraint

The core constraint is: **a customer cannot initiate a truck booking if no provider is
currently available.** This drives the architecture. Provider availability is the gate.

The system has two parallel sides:

- **Provider side**: Truck providers authenticate, register their truck, and toggle
  themselves online. When online, they are matchable. The app listens for incoming booking
  requests and accepts or rejects within a time window.

- **Customer side**: The customer taps "Find a Truck", the app checks provider availability
  in real time, then walks through pickup/dropoff/cargo details, sees a fare estimate,
  confirms, and waits while the system finds and matches a provider. Once matched, the
  customer tracks the live trip.

---

## 2. Booking State Machine

Every haulage booking moves through exactly these states:

```
pending_match
    ↓ (provider found and notified)
awaiting_acceptance
    ↓ (provider accepts)         → [timeout / reject] → pending_match (retry) or unmatched
accepted
    ↓ (provider confirms pickup)
en_route_pickup
    ↓ (provider arrives)
arrived_at_pickup
    ↓ (cargo loaded, provider departs)
en_route_delivery
    ↓ (provider arrives at destination)
arrived_at_destination
    ↓ (delivery confirmed by provider)
delivered
    ↓ (customer confirms / auto-confirmed after timeout)
completed
```

Cancellation is allowed by the customer up to `accepted` state. Providers can cancel up to
`en_route_pickup`. Cancellation after these windows triggers a penalty/partial charge
workflow (deferred to v2).

---

## 3. Backend — driver-hauling-service

The service is a scaffold with only `cmd/main.go` and `config/config.go`. Everything
below is net-new. Follow the exact feature-first layout from `CLAUDE.md` and mirror the
customer-service patterns.

### 3.1 Config additions (`internal/config/config.go`)

Add alongside the existing fields:

```go
Migration          bool
CustomerSecret     string   // HMAC secret for verifying customer bearer tokens
NotificationURL    string   // notification-service base URL
NotificationSecret string   // HMAC secret for notification-service
PaymentURL         string   // payment-wallet-service base URL
PaymentSecret      string   // HMAC secret for payment-wallet-service
BookingMatchTimeout int     // seconds provider has to accept (default 30)
ProviderOnlineTTL  int     // seconds a provider stays online without heartbeat (default 7200)
```

Env vars: `HAULING_MIGRATION`, `HAULING_CUSTOMER_TOKEN_SECRET`,
`HAULING_NOTIFICATION_URL`, `HAULING_NOTIFICATION_SECRET`, `HAULING_PAYMENT_URL`,
`HAULING_PAYMENT_SECRET`.

Add `getEnvBool` helper (same as notification-service pattern).

### 3.2 Migration (`services/driver-hauling-service/migrations/001_hauling_core.sql`)

```sql
-- Truck providers
CREATE TABLE IF NOT EXISTS truck_providers (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    phone            TEXT UNIQUE,
    email            TEXT UNIQUE,
    first_name       TEXT NOT NULL DEFAULT '',
    last_name        TEXT NOT NULL DEFAULT '',
    profile_photo_url TEXT,
    photo_asset_id   TEXT,
    status           TEXT NOT NULL DEFAULT 'active',   -- active | suspended
    onboarding_status TEXT NOT NULL DEFAULT 'profile_required',
    rating           NUMERIC(3,2) DEFAULT 5.00,
    total_trips      INT NOT NULL DEFAULT 0,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Provider auth sessions (mirrors customer_sessions)
CREATE TABLE IF NOT EXISTS provider_sessions (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id      UUID NOT NULL REFERENCES truck_providers(id) ON DELETE CASCADE,
    refresh_token_hash TEXT NOT NULL,
    device_id        TEXT NOT NULL DEFAULT '',
    user_agent       TEXT NOT NULL DEFAULT '',
    ip_address       TEXT NOT NULL DEFAULT '',
    expires_at       TIMESTAMPTZ NOT NULL,
    revoked_at       TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Trucks registered by providers
CREATE TABLE IF NOT EXISTS trucks (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider_id      UUID NOT NULL REFERENCES truck_providers(id) ON DELETE CASCADE,
    truck_type       TEXT NOT NULL,   -- flatbed | container | tipper | van | refrigerated
    capacity_kg      INT NOT NULL,
    plate_number     TEXT NOT NULL UNIQUE,
    year             INT,
    make             TEXT,
    model            TEXT,
    color            TEXT,
    status           TEXT NOT NULL DEFAULT 'active',  -- active | inactive
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Haulage bookings
CREATE TABLE IF NOT EXISTS haulage_bookings (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id           UUID NOT NULL,
    provider_id           UUID,
    truck_id              UUID,

    -- Pickup
    pickup_address        TEXT NOT NULL,
    pickup_lat            DOUBLE PRECISION NOT NULL,
    pickup_lng            DOUBLE PRECISION NOT NULL,

    -- Dropoff
    dropoff_address       TEXT NOT NULL,
    dropoff_lat           DOUBLE PRECISION NOT NULL,
    dropoff_lng           DOUBLE PRECISION NOT NULL,

    -- Cargo
    cargo_type            TEXT NOT NULL,   -- furniture | equipment | construction | food | general
    cargo_weight_kg       INT NOT NULL,
    cargo_description     TEXT NOT NULL DEFAULT '',
    requires_helpers      BOOLEAN NOT NULL DEFAULT FALSE,
    helper_count          INT NOT NULL DEFAULT 0,

    -- Pricing (kobo)
    distance_km           NUMERIC(8,2),
    fare_estimate_kobo    BIGINT,
    fare_final_kobo       BIGINT,

    -- Payment
    payment_intent_id     TEXT,

    -- Status
    status                TEXT NOT NULL DEFAULT 'pending_match',
    cancel_reason         TEXT,
    cancelled_by          TEXT,   -- customer | provider | system

    -- Timestamps
    matched_at            TIMESTAMPTZ,
    accepted_at           TIMESTAMPTZ,
    picked_up_at          TIMESTAMPTZ,
    delivered_at          TIMESTAMPTZ,
    completed_at          TIMESTAMPTZ,
    cancelled_at          TIMESTAMPTZ,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Booking events for audit
CREATE TABLE IF NOT EXISTS booking_events (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    booking_id   UUID NOT NULL REFERENCES haulage_bookings(id) ON DELETE CASCADE,
    event_type   TEXT NOT NULL,
    actor_type   TEXT NOT NULL,   -- customer | provider | system
    actor_id     TEXT NOT NULL,
    metadata     JSONB,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_haulage_bookings_customer ON haulage_bookings(customer_id);
CREATE INDEX IF NOT EXISTS idx_haulage_bookings_provider ON haulage_bookings(provider_id);
CREATE INDEX IF NOT EXISTS idx_haulage_bookings_status ON haulage_bookings(status);
CREATE INDEX IF NOT EXISTS idx_booking_events_booking ON booking_events(booking_id);
CREATE INDEX IF NOT EXISTS idx_trucks_provider ON trucks(provider_id);
CREATE INDEX IF NOT EXISTS idx_provider_sessions_provider ON provider_sessions(provider_id);
```

### 3.3 Feature: provider_auth

Path: `internal/features/provider_auth/`

Mirror `customer-service/internal/features/auth` exactly, with these differences:
- Token role claim: `truck_provider` instead of `customer`
- Redis key namespace: `hauling:provider:auth:otp:<identifier>`
- No email-only auth in v1 — phone is required

**Endpoints (all public, no bearer required):**

| Method | Path | Notes |
|--------|------|-------|
| `POST` | `/api/v1/hauling/provider/auth/start` | Start OTP by phone |
| `POST` | `/api/v1/hauling/provider/auth/verify` | Verify OTP → tokens |
| `POST` | `/api/v1/hauling/provider/auth/refresh` | Rotate refresh session |
| `POST` | `/api/v1/hauling/provider/auth/logout` | Revoke session |

### 3.4 Feature: provider_profile

Path: `internal/features/provider_profile/`

**Endpoints (all require provider bearer):**

| Method | Path | Notes |
|--------|------|-------|
| `GET`  | `/provider/me` | Get own profile |
| `PUT`  | `/provider/me` | Update name, photo |
| `POST` | `/provider/trucks` | Register a truck |
| `GET`  | `/provider/trucks` | List own trucks |
| `PUT`  | `/provider/trucks/:id` | Update truck details |
| `GET`  | `/provider/trucks/:id` | Get single truck |

At least one registered active truck is required before the provider can go online.

### 3.5 Feature: provider_availability

Path: `internal/features/provider_availability/`

This is the core gate. Availability is stored **only in Redis** (not Postgres) for fast
lookup and automatic expiry.

**Redis schema:**
```
hauling:providers:online          → Redis Set of provider IDs currently online
hauling:provider:status:<id>      → Hash: {provider_id, truck_id, lat, lng, updated_at} TTL=ProviderOnlineTTL
```

When a provider goes online:
1. Validate provider has at least one active truck
2. `SADD hauling:providers:online <provider_id>`
3. `HSET hauling:provider:status:<id> ...` with `EXPIRE` = `ProviderOnlineTTL`

When a provider goes offline or heartbeat expires:
1. `SREM hauling:providers:online <provider_id>`
2. Delete `hauling:provider:status:<id>`

**Endpoints (provider bearer):**

| Method | Path | Notes |
|--------|------|-------|
| `PUT`  | `/provider/availability` | Body: `{status:"online"\|"offline", lat, lng, truck_id}` |
| `POST` | `/provider/availability/heartbeat` | Extend TTL; body: `{lat, lng}` |
| `GET`  | `/provider/availability` | Current status |

**Customer-facing availability check (customer bearer via shared/go/auth with role=customer):**

| Method | Path | Notes |
|--------|------|-------|
| `GET`  | `/customer/availability` | Returns `{available: bool, count: int}` |

Implementation: `SCARD hauling:providers:online` — if count > 0 return available=true.

### 3.6 Feature: booking

Path: `internal/features/booking/`

**Sub-layers:**

- `models/booking.go` — domain model, state transitions, validation
- `repositories/booking_repository.go` — interface + postgres implementation
- `usecases/booking_service.go` — orchestrates creation, matching, lifecycle
- `http/handler.go`, `http/routes.go`, `http/dto.go`
- `clients/payment_client.go` — wraps `shared/go/walletclient` calls
- `clients/notification_client.go` — wraps `shared/go/notifications` calls

**Customer endpoints (customer bearer):**

| Method | Path | Notes |
|--------|------|-------|
| `POST` | `/customer/bookings` | Create booking, triggers matching |
| `GET`  | `/customer/bookings` | Booking history (paginated) |
| `GET`  | `/customer/bookings/:id` | Booking + current status |
| `PUT`  | `/customer/bookings/:id/cancel` | Cancel (allowed up to `accepted`) |

**Provider endpoints (provider bearer):**

| Method | Path | Notes |
|--------|------|-------|
| `GET`  | `/provider/bookings` | Incoming + active bookings |
| `GET`  | `/provider/bookings/:id` | Single booking details |
| `PUT`  | `/provider/bookings/:id/accept` | Accept a matched booking |
| `PUT`  | `/provider/bookings/:id/reject` | Reject (re-triggers matching) |
| `PUT`  | `/provider/bookings/:id/pickup-confirmed` | Mark cargo picked up |
| `PUT`  | `/provider/bookings/:id/delivered` | Mark delivered (triggers payment completion) |

**Matching algorithm (inside `BookingService.CreateBooking`):**

```
1. Check SCARD hauling:providers:online > 0 (gate — fail fast if no one online)
2. Create booking row with status=pending_match
3. Get all online provider IDs from Redis Set
4. For each provider (sorted by last known lat/lng proximity to pickup):
   a. Attempt to atomically claim the provider using a Redis lock:
      SET hauling:provider:matching:<provider_id> <booking_id> NX EX <MatchTimeout>
   b. If lock acquired:
      - Update booking: status=awaiting_acceptance, provider_id=<id>
      - Send push notification to provider via notification-service
      - Start a goroutine timer for MatchTimeout seconds
      - On timeout: release lock, reset booking to pending_match, try next provider
      - On provider accept: cancel timer, advance status to accepted
   c. If lock not acquired (provider already being claimed): skip, try next
5. If no available provider can be locked: mark booking unmatched, notify customer
```

For v1, use simple proximity sort on stored lat/lng from Redis. A proper geospatial
query (Redis GEORADIUS) can replace this in v2.

**Fare estimation (`POST /customer/bookings/estimate`):**

Simple formula for v1:
- Base fare: ₦5,000 (500,000 kobo)
- Per km rate: ₦250/km (25,000 kobo)
- Weight surcharge: +10% if cargo_weight_kg > 500
- Helper fee: ₦2,000 per helper per trip

This is a pure calculation endpoint that returns `{fare_estimate_kobo, distance_km}` without
creating a booking.

**Payment flow:**
- On booking confirmed by provider → call `payment-wallet-service POST /internal/payment-intents`
  with `source_service=hauling-service`, `source_reference=<booking_id>`
- On delivery confirmed → call `POST /internal/jobs/hauling-service/<booking_id>/complete`
- Customer pays from wallet or card via the existing Paystack flow in payment-wallet-service

### 3.7 Migration flag + cmd/main.go wiring

Add `MIGRATION=true` flag support (exact pattern from notification-service).

Wire all features into `cmd/main.go` with:
- Postgres connection pool
- Redis client
- Auth middleware using `shared/go/auth` with role `truck_provider`
- Customer bearer middleware using customer token secret (role `customer`)
- All route groups registered on the `/api/v1/hauling` base

---

## 4. Customer App — Flutter

### 4.1 Config (`customer_app_config.dart`)

Add:
```dart
static String get haulingApiBaseUrl =>
    const String.fromEnvironment('HAULING_API_BASE_URL',
        defaultValue: 'http://localhost:8104/api/v1/hauling');
```

### 4.2 Hauling feature module

```
apps/customer/lib/features/hauling/
  models/
    hauling_models.dart          ← HaulageBooking, HaulageBookingStatus, TruckProvider,
                                    FareEstimate, CargoType, CreateBookingRequest
  data/
    hauling_api.dart             ← HTTP client wrapping all /customer/* hauling endpoints
  state/
    hauling_booking_controller.dart  ← ChangeNotifier state machine
  ui/
    hauling_availability_screen.dart
    hauling_booking_screen.dart
    hauling_confirm_screen.dart
    hauling_searching_screen.dart
    hauling_active_trip_screen.dart
    hauling_complete_screen.dart
```

### 4.3 Booking controller state machine

```dart
enum HaulingBookingStatus {
  idle,
  checkingAvailability,
  unavailable,           // gate: no providers online
  enteringDetails,
  fetchingEstimate,
  confirmingBooking,
  searching,             // polling for provider match
  providerMatched,
  enRoutePickup,
  arrived,
  enRouteDelivery,
  delivered,
  completed,
  cancelled,
  error,
}
```

The controller drives all screens. It polls `GET /customer/bookings/:id` every 5 seconds
while status is `searching`/`awaiting_acceptance`. Once matched it updates to
`providerMatched` and stops fast polling, switching to a slower 15s refresh.

### 4.4 Screen flows

**`HaulingAvailabilityScreen`**
- On mount: calls `GET /customer/availability`
- Shows animated loading indicator ("Checking truck availability...")
- If `available=true` → push `HaulingBookingScreen`
- If `available=false` → show "No trucks available right now" with retry button and option
  to get notified when a truck becomes available

**`HaulingBookingScreen`**
- Pickup location: text field with address autocomplete (Google Places or manual v1)
- Dropoff location: same
- Cargo type selector (Furniture / Equipment / Construction / Food / General)
- Estimated weight (kg) — number input
- Description — optional text field
- "Need helpers?" toggle + helper count stepper
- "Get Estimate" button → calls `POST /customer/bookings/estimate`, shows
  `HaulingConfirmScreen`

**`HaulingConfirmScreen`**
- Shows route summary: pickup → dropoff, distance
- Fare breakdown: base + per-km + helpers
- Truck type required (system auto-selects; v2 lets customer choose)
- "Pay from Wallet" / "Pay on Delivery" toggle (v1: wallet only)
- "Confirm Booking" button → calls `POST /customer/bookings`, navigates to
  `HaulingSearchingScreen`

**`HaulingSearchingScreen`**
- Full-screen animated state: pulsing truck icon, "Finding your truck provider..."
- Shows booking reference ID
- Polls booking status every 5s
- On `accepted` → pushes `HaulingActiveTripScreen`
- Cancel button triggers `PUT /customer/bookings/:id/cancel`
- Timeout after 5 minutes → shows "No provider accepted. Try again."

**`HaulingActiveTripScreen`**
- Provider card: name, photo, rating, phone number (tap to call)
- Truck details: type, plate number
- Map placeholder (real Google Maps in v2) showing pickup → dropoff route
- Status chip that updates: "Heading to pickup" → "Arrived at pickup" → "Cargo loaded,
  en route" → "Arriving at destination"
- Status is polled every 15s

**`HaulingCompleteScreen`**
- Fare paid, trip summary
- Star rating for provider (1-5)
- Optional comment
- "Done" returns to home

### 4.5 Home screen wiring

In `_HomeTabState._HomeTab`, the "Find a Truck" CTA `onPressed` should:
```dart
onPressed: () {
  if (_selectedService == 2) {  // Truck / Hauling
    Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => HaulingAvailabilityScreen(
          controller: haulingController,
        ),
      ),
    );
  }
}
```

`HaulingBookingController` is constructed in `customer_app.dart` alongside the existing
controllers and passed down.

### 4.6 Trips tab

Update `_TripsTab` to:
1. Call `GET /customer/bookings` on mount
2. Show list of past haulage bookings with status chips
3. Tap → `HaulingActiveTripScreen` (if active) or a read-only summary screen (if completed)

### 4.7 App routes additions

```dart
static const haulingAvailability = 'hauling-availability';
static const haulingBooking      = 'hauling-booking';
static const haulingConfirm      = 'hauling-confirm';
static const haulingSearching    = 'hauling-searching';
static const haulingActiveTrip   = 'hauling-active-trip';
static const haulingComplete     = 'hauling-complete';
```

---

## 5. Truck Provider App — apps/truck_provider

The app is a default Flutter scaffold (counter demo). It needs to be built from scratch
following the customer app's conventions: `packages/api_core`, `packages/ui_kit`, the same
controller-driven architecture.

### 5.1 Structure

```
apps/truck_provider/lib/
  core/
    config/truck_provider_app_config.dart
  app/
    truck_provider_app.dart
    app_routes.dart
  features/
    auth/
      models/provider_auth_models.dart
      data/provider_auth_api.dart
      data/provider_session_store.dart
      state/provider_auth_controller.dart
      ui/
        splash_screen.dart
        phone_entry_screen.dart
        otp_verification_screen.dart
    onboarding/
      ui/
        profile_setup_screen.dart
        truck_setup_screen.dart
        onboarding_complete_screen.dart
    home/
      state/provider_home_controller.dart
      ui/
        provider_home_screen.dart     ← online/offline toggle, earnings snapshot, tabs
    booking/
      models/booking_models.dart
      data/booking_api.dart
      state/booking_controller.dart
      ui/
        incoming_request_screen.dart  ← countdown timer, accept/reject
        active_trip_screen.dart       ← pickup → delivery steps
        trip_complete_screen.dart
    availability/
      data/availability_api.dart
```

### 5.2 Auth flow

Identical pattern to customer app: phone → OTP → token pair stored in
`flutter_secure_storage`. Token role is `truck_provider`.

### 5.3 Provider home screen

Tab structure:
- **Home tab**: Online/Offline toggle (large, prominent), current earnings today, active
  booking card if one exists
- **Trips tab**: History of completed bookings + earnings per trip
- **Earnings tab**: Weekly/monthly breakdown from `GET /provider/earnings` on
  payment-wallet-service
- **Profile tab**: Provider profile, truck details, documents

### 5.4 Online/offline toggle

When provider taps "Go Online":
1. Check they have an active truck (client-side validation from stored profile)
2. Call `PUT /provider/availability` with `{status:"online", lat, lng, truck_id}`
3. On success: start a background timer every 10 minutes to call `POST /provider/availability/heartbeat`
4. Screen shows green "You are Online" state and begins polling for incoming requests

When offline:
1. Call `PUT /provider/availability` with `{status:"offline"}`
2. Cancel heartbeat timer

### 5.5 Incoming booking request screen

When a push notification arrives (or polling detects `awaiting_acceptance` booking):
1. Full-screen overlay: customer's pickup address, dropoff address, cargo type, weight,
   distance, fare amount
2. 30-second countdown timer
3. **Accept** button → `PUT /provider/bookings/:id/accept`
4. **Reject** button → `PUT /provider/bookings/:id/reject`
5. On timeout with no action → system auto-rejects and tries next provider

### 5.6 Active trip screen

Step-by-step confirm buttons:
1. "I've arrived at pickup" → `PUT .../arrived-pickup` (informational, no separate endpoint
   needed — use booking event system)
2. "Cargo loaded, starting delivery" → `PUT .../pickup-confirmed`
3. "I've arrived at destination" → informational
4. "Confirm delivery" → `PUT .../delivered` — requires optional photo upload for
   proof-of-delivery (via media-file-service, purpose=`proof_image`)

---

## 6. Implementation Order

### Sprint 1 — Backend foundation (hauling-service)

1. Migration file (`001_hauling_core.sql`)
2. Config additions + `MIGRATION` flag
3. `provider_auth` feature (OTP auth, session management)
4. `provider_profile` feature (profile CRUD, truck registration)
5. `provider_availability` feature (Redis online/offline, heartbeat)
6. `booking` feature:
   a. Fare estimation endpoint (pure calculation)
   b. `POST /customer/bookings` with synchronous availability gate
   c. Async matching goroutine + Redis lock mechanism
   d. Status polling endpoints
   e. Provider accept/reject endpoints
   f. Trip lifecycle endpoints (pickup-confirmed, delivered)
7. Payment-wallet-service integration (create intent, complete job)
8. Notification integration (provider notified on match, customer notified on accept/complete)
9. Usecase and handler tests

### Sprint 2 — Customer app hauling screens

1. `hauling_models.dart` + `hauling_api.dart`
2. `HaulingBookingController` state machine
3. `HaulingAvailabilityScreen` + `HaulingBookingScreen`
4. `HaulingConfirmScreen` with fare breakdown
5. `HaulingSearchingScreen` with polling
6. `HaulingActiveTripScreen`
7. `HaulingCompleteScreen` with rating
8. Wire CTA in home screen
9. Wire Trips tab to show booking history
10. `flutter analyze` + `flutter test`

### Sprint 3 — Truck provider app

1. Scaffold: `truck_provider_app.dart`, theme (reuse `cosmicforge_logistics_app_theme.dart`
   pattern), routing
2. Auth flow (screens + controller, mirrors customer app)
3. Onboarding: profile setup + truck registration screens
4. Provider home screen with online/offline toggle
5. Incoming request screen with countdown
6. Active trip screen with step confirmations
7. Proof of delivery photo upload
8. Trips history + earnings screens
9. Push notification wiring (FCM device token registration via notification-service)
10. `flutter analyze` + `flutter test`

### Sprint 4 — Integration + hardening

1. End-to-end flow test: provider goes online → customer books → provider accepts →
   provider delivers → payment settles
2. Cancellation flows (customer and provider side)
3. Matching timeout and retry (second provider attempt)
4. Wallet top-up prompt when customer balance is insufficient
5. Docker Compose env updates for hauling service env vars
6. Load test the matching concurrency (multiple simultaneous bookings)

---

## 7. Key Environment Variables

### driver-hauling-service `.env`

```env
HTTP_ADDR=:8104
HAULING_DATABASE_URL=postgres://cosmicforge_logistics:cosmicforge_logistics@localhost:5436/hauling_service?sslmode=disable
HAULING_REDIS_ADDR=localhost:6383
HAULING_MIGRATION=false

HAULING_CUSTOMER_TOKEN_SECRET=<same-secret-used-by-customer-service>
HAULING_PROVIDER_TOKEN_SECRET=<new-secret-for-truck-provider-tokens>
HAULING_OTP_SECRET=<new-secret-for-provider-otps>

HAULING_NOTIFICATION_URL=http://localhost:8106/api/v1/notifications
HAULING_NOTIFICATION_SECRET=<hauling-notification-hmac-secret>

HAULING_PAYMENT_URL=http://localhost:8105/api/v1/payments
HAULING_PAYMENT_SECRET=<hauling-payment-hmac-secret>

HAULING_BOOKING_MATCH_TIMEOUT=30
HAULING_PROVIDER_ONLINE_TTL=7200
```

### Customer app dart-defines

```bash
flutter run \
  --dart-define=CUSTOMER_API_BASE_URL=http://localhost:8101/api/v1/customer \
  --dart-define=HAULING_API_BASE_URL=http://localhost:8104/api/v1/hauling \
  --dart-define=MEDIA_FILE_API_BASE_URL=http://localhost:8109/api/v1/media-files \
  --dart-define=MEDIA_FILE_SERVICE_TOKEN=development-media-token \
  --dart-define=SUPPORT_API_BASE_URL=http://localhost:8107/api/v1/support-disputes \
  --dart-define=WALLET_API_BASE_URL=http://localhost:8105/api/v1/payment-wallet
```

### Truck provider app dart-defines

```bash
flutter run \
  --dart-define=HAULING_API_BASE_URL=http://localhost:8104/api/v1/hauling \
  --dart-define=MEDIA_FILE_API_BASE_URL=http://localhost:8109/api/v1/media-files \
  --dart-define=MEDIA_FILE_SERVICE_TOKEN=development-provider-media-token
```

---

## 8. What Is Out of Scope for v1

These are tracked as future work to keep v1 shippable:

- Real-time GPS tracking (requires WebSocket streaming from provider app)
- Google Maps integration (use coordinate-based placeholder in v1)
- Geospatial Redis GEORADIUS matching (use simple linear proximity sort for now)
- Customer truck type preference selection
- Cancellation penalty charges
- Provider ratings aggregation after each trip
- In-app chat between customer and provider
- Multi-stop haulage
- Scheduled/future bookings
- Admin dashboard for hauling operations
- Analytics service integration

---

## 9. Files to Create (Summary)

### driver-hauling-service (net new)

```
services/driver-hauling-service/
  migrations/001_hauling_core.sql
  internal/database/
    migrate.go
    postgres.go
    redis.go
  internal/features/
    provider_auth/http/{handler,routes,dto}.go
    provider_auth/models/provider.go
    provider_auth/repositories/provider_repository.go
    provider_auth/usecases/auth_service.go
    provider_profile/http/{handler,routes,dto}.go
    provider_profile/models/profile.go
    provider_profile/repositories/profile_repository.go
    provider_profile/usecases/profile_service.go
    provider_availability/http/{handler,routes,dto}.go
    provider_availability/repositories/availability_store.go
    provider_availability/usecases/availability_service.go
    booking/http/{handler,routes,dto}.go
    booking/models/booking.go
    booking/repositories/booking_repository.go
    booking/usecases/booking_service.go
    booking/clients/{payment_client,notification_client}.go
```

### apps/customer (additions)

```
apps/customer/lib/features/hauling/
  models/hauling_models.dart
  data/hauling_api.dart
  state/hauling_booking_controller.dart
  ui/hauling_availability_screen.dart
  ui/hauling_booking_screen.dart
  ui/hauling_confirm_screen.dart
  ui/hauling_searching_screen.dart
  ui/hauling_active_trip_screen.dart
  ui/hauling_complete_screen.dart
```

Modified: `app_routes.dart`, `customer_app_config.dart`, `customer_app.dart`,
`customer_home_screen.dart`, `_TripsTab` (in home screen)

### apps/truck_provider (full build)

All files under `apps/truck_provider/lib/` replacing the counter scaffold.
