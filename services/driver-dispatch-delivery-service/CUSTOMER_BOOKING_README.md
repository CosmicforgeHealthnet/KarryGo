# Customer Dispatch Booking — Implementation Notes

This document explains the customer-facing booking feature added to
`driver-dispatch-delivery-service` on 2026-07-09, why each decision was made,
and exactly what changed in existing code. It is written against the
"Dispatch Customer API Documentation" audit doc, which marked every
customer-facing capability as `GAP, NOT FOUND`. This work closes those gaps.

Related changes were also made in `payment-wallet-service` and
`shared/go/walletclient` (see section 5), and the customer Flutter app gained a
full Bike Delivery flow (`apps/customer/lib/features/dispatch/`) that consumes
this API.

---

## 1. Why

The audit doc identified (section 12 "Gaps and Required Work"):

- No customer create-booking endpoint (bookings only entered via the
  api-gateway **partner** B2B API publishing `booking.dispatch.created`).
- No owner of dispatch booking persistence — the booking existed only as a
  JSONB payload inside `request_broadcasts`, and as a `trips` row *after* a
  rider accepted. Nothing customer-queryable.
- No customer status/tracking/cancel/review endpoints; customer JWTs were not
  accepted anywhere.
- No payment intent creation connected to the customer flow.

The implementation follows the audit doc's own rules:

- Customer JWT auth (customer-service tokens), **not** rider tokens, **not**
  partner API keys.
- Payment secured **before** matching starts.
- `source_service = "dispatch-delivery-service"` and
  `source_reference = booking_id` kept aligned with what trip completion
  already passes to `CompleteJob` and what provider cancel passes to
  `RequestRefund`.
- Customer flow kept fully separate from the partner API gateway flow — the
  partner event path is untouched and still works.

### 1.1 "But an event method already existed — why build all this?"

A fair question, worth answering head-on, because the `booking.dispatch.created`
event pipeline you built **is** real, working, and fully reused here. The
short answer: an inbound event is the *entry into matching*, not a customer
booking system. Two things were missing around it.

**First, nothing customer-side could publish it.** The only publisher was the
api-gateway **partner** feature — a B2B surface authenticated with partner API
keys. The customer app can't use that entrance, and the audit doc's own rules
forbid it ("Do not use partner API keys for the customer app", "Keep customer
flow separate from partner API gateway flow"). The doc's own search confirmed
the hole: *"booking.dispatch.created publisher in customer-service: NOT
FOUND."* So even the entry half of the flow had no customer-facing on-ramp.

**Second, an event is fire-and-forget — it gives the customer nothing back.**
Even with a publisher in place, the customer would have been blind after the
publish:

- No booking record the customer owns or can query. The event's payload only
  survives as the JSONB blob inside `request_broadcasts` (matching machinery,
  not queryable per-customer), and a `trips` row exists only *after* a rider
  accepts — an unmatched or never-accepted booking leaves no customer-visible
  trace at all.
- No status: every read endpoint (`/provider/requests`, `/provider/trips/...`)
  is rider-token-only.
- No cancel: `booking.dispatch.cancelled` had no customer-facing publisher
  either.
- No money: nothing created a payment intent, so the settlement
  (`CompleteJob`) and refund calls already wired into the trip service had
  nothing to ever settle or refund against.

**What this feature does is put a customer front door in front of your
pipeline — not replace it.** `CreateBooking` authenticates the customer, holds
the fare, persists the booking record, and then invokes **the exact same
machinery the event feeds**: it calls `requestService.StartBroadcast(...)`
directly — the same function `HandleBookingDispatchCreatedPayload` calls when
a partner event arrives (see 2.3 for why in-process instead of pub/sub).
Broadcast, inbox, radius expansion, accept locks, trip creation, trip
lifecycle, and your wallet settlement/refund calls are all untouched and now
actually exercised end-to-end. Cancellations likewise flow through the
existing `booking.dispatch.cancelled` semantics, and the booking feature
*subscribes* to your outbound events (`request.accepted`,
`request.no_provider_found`, `trip.completed`, `trip.cancelled`) rather than
duplicating any of that logic.

So: kitchen and conveyor belt were real and are still doing the work — this
change adds the dining room, the cashier, and the order-status board that an
inbound event alone could never provide.

---

## 2. Architecture

### 2.1 New feature package

`internal/features/booking/` (flat layout, same conventions as `trip/` and
`request/`):

| File | Responsibility |
|---|---|
| `model.go` | `Booking`, wire statuses, `DerivedState.WireStatus()`, DTOs |
| `pricing.go` | Thin delegate to `shared/go/dispatchpricing` — the single source of truth also used by the api-gateway partner delivery API, so the two surfaces cannot drift |
| `repository.go` | `dispatch_bookings` persistence + the derived-state JOIN |
| `service.go` | All business rules (create/cancel/review/location + event handlers) |
| `payments.go` | `walletclient` adapter (hold / assign provider / refund) |
| `events.go` | Publishes `booking.dispatch.cancelled` (parity with partner flow) |
| `subscribers.go` | Redis pub/sub consumers (section 6) |
| `trip_adapter.go` | Synchronous customer trip-cancel (section 2.4) |
| `handler.go` | Gin handlers + route registration |
| `service_test.go` | Table-driven tests with fakes (same style as `trip/`) |

Migrations: `000031_create_dispatch_bookings.{up,down}.sql`,
`000032_add_review_fields_to_ratings.{up,down}.sql` (adds
`ratings.recommends_provider BOOLEAN NULL`; customer reviews reuse the
existing `ratings` table — the unique `idx_ratings_booking` index already
enforces one review per booking).

### 2.2 Booking persistence + derived status

`dispatch_bookings` owns the **customer's booking record** (the audit doc's
"owner of dispatch booking persistence"). Its persisted `status` column is
deliberately **coarse** (`searching | accepted | completed | cancelled |
unmatched`) and used for payment bookkeeping and terminal guards.

The **customer-facing status is always derived on read** with one query:

```sql
dispatch_bookings b
LEFT JOIN request_broadcasts rb ON rb.booking_id = b.id
LEFT JOIN trips t             ON t.booking_id  = b.id
```

Precedence (see `DerivedState.WireStatus()` in `model.go`):

| Source state | Wire status |
|---|---|
| `t.status = assigned` | `accepted` |
| `t.status = en_route_pickup` | `en_route_pickup` |
| `t.status = arrived_pickup` | `arrived_at_pickup` |
| `t.status = in_progress` | `in_transit` |
| `t.status = proof_submitted` | `delivered` |
| `t.status = completed` | `completed` |
| `t.status IN (cancelled, failed)` | `cancelled` |
| `b.status = unmatched` | `unmatched` (**checked before** cancelled_at — unmatched rows also carry a cancelled_at; this ordering was a live-tested bug fix) |
| `b.cancelled_at` set or `b.status = cancelled` | `cancelled` |
| `rb.status = cancelled` | `cancelled` |
| `rb.status IN (no_provider_found, expired)` | `unmatched` |
| `rb.status = accepted` (trip row not created yet) | `accepted` |
| otherwise | `searching` |

Why derive instead of caching: `en_route_pickup` is set by a **silent** DB
transition in the trip service (`HandleProviderLocationUpdated` publishes no
event), so no event-driven cache can be complete. Same DB, one LEFT JOIN,
always fresh.

### 2.3 Matching entry: direct call, not pub/sub

`CreateBooking` calls `requestService.StartBroadcast(...)` **in-process**
instead of publishing `booking.dispatch.created`:

- `StartBroadcast` is already idempotent by `booking_id`, so this is safe.
- A direct call gives **synchronous error semantics**: if broadcasting fails,
  the create endpoint immediately refunds the wallet hold
  (`refund-booking-<id>`) and returns an error. With pub/sub, a dropped
  message would strand a *paid* booking in `searching` forever.
- The partner path (gateway publishes the event, `request/subscribers.go`
  consumes it) is **unchanged** and continues to work in parallel.

### 2.4 Customer cancel: synchronous trip cancel

`POST /bookings/:id/cancel` behaviour by state:

- **Pre-accept (searching):** `CancelBroadcastForBooking` (idempotent, no-ops
  if a rider accepted meanwhile) → **re-read** derived state; if an accept
  raced in, return 409 instead of refunding with the pre-accept key →
  otherwise refund `refund-booking-<id>` and publish
  `booking.dispatch.cancelled` for observers.
- **Rider assigned (assigned / en_route_pickup / arrived_pickup):** calls
  `trip.Service.HandleBookingDispatchCancelled` **synchronously** via
  `TripCancelAdapter` (the exact same handler the event subscriber runs), then
  **verifies the trip actually flipped to cancelled** before refunding
  `refund-<tripID>`. The trip-side guard no-ops for `in_progress`, so if the
  rider started the trip mid-cancel, the endpoint returns 409 and **no refund
  fires** — this closes the refund-while-delivering race that a
  publish-and-hope design would have.
- **in_transit / delivered / terminal:** 409.

### 2.5 Availability + live location

- `GET /availability?lat&lng` counts riders via the availability feature's
  Redis geo store (`RedisLiveStore.GetNearby`), radius from
  `DISPATCH_AVAILABILITY_RADIUS_KM` (default 10). Coordinates are required —
  dispatch availability is geo-scoped, unlike hauling's global count.
- `GET /bookings/:id/location` returns the assigned rider's last GPS fix from
  the live store (`GetLocation`). 404 when no rider is assigned or no recent
  fix — the customer app maps 404 to "no fix" and keeps polling.

---

## 3. Endpoint reference

Base: `/api/v1/customer/dispatch`. Auth: `Authorization: Bearer
<customer access token>` verified with `shared/go/auth.BearerMiddleware`
(role `customer`, service `customer`). The signing secret is
`DISPATCH_CUSTOMER_TOKEN_SECRET` and **must equal customer-service's
`CUSTOMER_ACCESS_TOKEN_SECRET`** (dev default
`development-customer-access-token-secret`).

Envelope: `{"success": true, "message": "...", "data": {...}}` via the local
`apiresponse` helper; errors via `httpx` / `apperrors` as everywhere else.

| Method | Path | Auth | Purpose |
|---|---|---|---|
| POST | `/bookings/estimate` | none | Fare quote (no side effects) |
| GET | `/availability?lat&lng` | customer | Nearby online rider count |
| POST | `/bookings` | customer | Create booking (holds wallet, starts matching) |
| GET | `/bookings?limit&offset` | customer | List own bookings (`{"bookings": [...]}`) |
| GET | `/bookings/:id` | customer | Booking with derived status |
| POST | `/bookings/:id/cancel` | customer | Cancel (body `{"reason": "..."}` optional) |
| POST | `/bookings/:id/review` | customer | Rate rider (writes to `ratings`) |
| GET | `/bookings/:id/review` | customer | Own review (404 when none) |
| GET | `/bookings/:id/location` | customer | Assigned rider live GPS |
| GET | `/providers/:id` | customer | Rider public profile (delegates to `profile.Service.GetPublicProfile`) |

Notes: paths and the POST-cancel verb follow the audit doc's proposed
contract. Ownership is enforced on every `:id` route (foreign bookings return
404, not 403, to avoid leaking existence).

### Create booking

```http
POST /api/v1/customer/dispatch/bookings
Authorization: Bearer <customer-token>

{
  "pickup":  {"lat": 6.5244, "lng": 3.3792, "address": "Surulere, Lagos"},
  "dropoff": {"lat": 6.6018, "lng": 3.3515, "address": "Ikeja, Lagos"},
  "receiver_name": "Ada Obi",
  "receiver_phone": "+2348012345678",
  "package_desc": "Documents",
  "package_weight": 1.5,
  "package_type": "parcel",
  "package_size": "small",
  "is_fragile": false,
  "service_tier": "standard",
  "payment_method": "wallet"
}
```

Server recomputes distance + fare (client fare is never trusted). Validation:
coordinates in range and not (0,0), addresses/receiver/package_desc required,
`package_size` in small|medium|large, `service_tier` in standard|express,
`payment_method` must be `wallet` (the only supported method for now — the
CHECK constraint on the table matches). Response is the booking view with
`status: "searching"`, `fare_amount_kobo`, `payment_status: "held"`.

### Estimate

Request: `pickup_lat/lng`, `dropoff_lat/lng`, `package_size`, `service_tier`.
Response: `distance_km`, `fare_estimate_kobo`, `eta`, and a display
`breakdown` (`base_fare_kobo`, `distance_fare_kobo`, `size_surcharge_kobo`,
`express_surcharge_kobo`). The formula (base ₦800 + ₦180/km, +₦300 medium /
+₦700 large, ×1.35 express, rounded to 50 kobo, min ₦1,000) lives in
**`shared/go/dispatchpricing`** and is imported by both this service and the
api-gateway partner delivery API — one place to change, both surfaces follow.

---

## 4. Money flow (wallet escrow)

Decision: **hold at booking creation** (fail fast on low balance), not at
acceptance. Every wallet interaction is idempotency-keyed:

| Step | Wallet call | Idempotency key |
|---|---|---|
| Create | `CreatePaymentIntent` (`source_service=dispatch-delivery-service`, `source_reference=<bookingID>`, `provider_type="dispatch"`) + `PayFromWallet` | `dispatch-hold-<bookingID>` / `dispatch-pay-<bookingID>` |
| Rider accepts | `AssignProvider(paymentRef, "dispatch", riderID)` — **new wallet endpoint**, see 5.1 | idempotent by design |
| Trip completes | existing `CompleteJob("dispatch-delivery-service", bookingID)` in `trip/service.go` — unchanged | — |
| Customer cancel pre-accept / unmatched / broadcast-start failure | `RequestRefund` | `refund-booking-<bookingID>` |
| Customer cancel post-accept, trip-level cancels | `RequestRefund` | `refund-<tripID>` (same key the trip service's provider-cancel auto-refund uses → the wallet dedupes, no double refund) |

Double-refund safety: `refund-booking-*` only ever fires when **no trip
exists** (mutually exclusive with `refund-<tripID>`), and the pre-accept
cancel re-reads state after `CancelBroadcastForBooking` to close the accept
race. Payment bookkeeping (`payment_status` → `refunded`) is flipped **only
after** the wallet confirms (or dedupes) the refund.

`payment_status` lifecycle on `dispatch_bookings`:
`unpaid → held → paid` (settled) or `→ refunded` or `→ failed` (hold failed;
booking is terminal-cancelled with reason `payment_failed`).

Why `provider_type = "dispatch"`: the payment-wallet provider bearer
middleware derives `provider_type` from the token's `service` claim, and
dispatch rider tokens carry `service="dispatch"`. Settling with the same value
means `CompleteJob` credits `provider_payable` under owner
`dispatch:<riderID>` — exactly the ledger the rider app's earnings/withdrawal
endpoints read. (Hauling has a known `truck_provider` vs `hauling` mismatch
here; this feature deliberately avoids repeating it.)

---

## 5. Changes outside this service

### 5.1 payment-wallet-service

- **New internal endpoint** `PUT /api/v1/payment-wallet/internal/payments/:reference/provider`
  (`AssignPaymentProvider`): attaches `provider_type`/`provider_id` to an
  existing intent. Needed because the hold is created **before** a rider
  exists, and `CompleteJob` refuses to settle an intent with empty provider
  details (`postgres_wallet_repository.go` — "Provider details are required
  before settlement."). Idempotent: re-assigning the same provider is a no-op;
  a different provider, or a refunded/failed intent, returns 409.
- **Bug fix in `RequestRefund`**: it resolved intents **only** by intent
  reference (`pi_...`), but this service's existing trip auto-refund passes
  the **booking id** as `payment_reference` — meaning every dispatch refund
  would have failed with not-found once real intents existed. `RequestRefund`
  now falls back to `GetPaymentIntentBySource(source_service,
  source_reference)`; the refund DTO gained the two source fields (the trip
  service already sends them — no change needed on your side).

### 5.2 shared/go/walletclient

- `Client.AssignProvider(ctx, reference, providerType, providerID)` added
  (PUT helper included).

### 5.3 Existing dispatch code (small, additive)

- `request/model.go` + `request/service.go`: `RequestAcceptedEvent` gained
  `customer_id` (populated from the broadcast's booking payload in `Accept`).
  The trip subscriber already parsed `customer_id,omitempty`, so
  `trips.customer_id` stops being empty. Additive — provider app unaffected.
- `internal/router/router.go`: `New(...)` takes a `*booking.Service`;
  registers the customer routes with a `sharedauth.TokenSigner` built from
  `cfg.CustomerTokenSecret`. The booking handler reuses the in-router
  `profileService` for `GET /providers/:id`.
- `cmd/main.go`: builds the booking service (repo + requestService +
  `TripCancelAdapter{tripService}` + `availability.NewRedisLiveStore` +
  wallet adapter + event publisher) and starts its subscribers. Also now calls
  `godotenv.Load()` at startup (same pattern as hauling) so the committed
  `.env` works without manual exports.
- `internal/config/config.go`: `DISPATCH_CUSTOMER_TOKEN_SECRET` (dev default
  matches customer-service) and `DISPATCH_AVAILABILITY_RADIUS_KM` (default 10).
- `.env.example` / new `.env`: the two vars above documented.

**Explicitly NOT changed:** all `/api/v1/provider/*` and `/api/v1/auth/*`
routes and contracts (the dispatch_provider app is untouched), the partner
event pipeline, broadcast/inbox mechanics, trip lifecycle and its wallet
settlement, admin routes.

---

## 6. Event subscriptions (booking feature)

`booking/subscribers.go` consumes four existing topics to keep
`dispatch_bookings` in sync. Partner-created bookings have no
`dispatch_bookings` row, so their events no-op here (guarded UPDATEs affect
0 rows).

| Topic | Handler behaviour |
|---|---|
| `request.accepted` | `SetAccepted` — CAS `WHERE status='searching'` (duplicate/late events can't overwrite the provider) → wallet `AssignProvider` (failure logged, not fatal: settlement is retriable at completion) |
| `request.no_provider_found` | `SetUnmatched` — **strict CAS from `searching`**, so a stray/late event can never clobber or refund an accepted booking → refund `refund-booking-<id>` (exactly once; second event no-ops) |
| `trip.completed` | mark `completed`, `payment_status held → paid` |
| `trip.cancelled` | sync `cancelled`; for provider cancels without admin investigation, re-issue the refund with the **same** `refund-<tripID>` key (dedupes with your auto-refund goroutine, and bookkeeping only flips on confirmed refund) |

The two CAS guards were both validated by a live race test (see section 7) —
`TestCancelAcceptRaceReturnsConflictWithoutRefund`,
`TestCancelTripRaceToInProgressDoesNotRefund`, and
`TestHandleNoProviderFoundNeverClobbersAcceptedBooking` in `service_test.go`
pin them.

---

## 7. How to run and what was verified

```bash
# One-time local setup (Postgres 5435, Redis 6382, all migrations):
bash scripts/bootstrap_services/dispatch-local-bootstrap.sh

cd services/driver-dispatch-delivery-service
go run ./cmd          # :8103, .env auto-loaded

go test ./...         # unit tests incl. the booking feature suite
```

Note on the bootstrap script: this service's migrations are golang-migrate
`.up/.down` files and are **not all re-run-safe** (e.g. `000021_create_trips`
lacks `IF NOT EXISTS`), so the script tracks applied files in a
`local_schema_migrations` marker table and skips them on re-runs.

Verified live against running customer-service (:8101) and
payment-wallet-service (:8105):

1. Estimate matches the partner formula exactly (incl. 50-kobo rounding).
2. Create with empty wallet → 400 `payment_failed` with the wallet's message;
   booking terminal `cancelled/failed`; **no broadcast started**.
3. Create with funded wallet → `searching/held`, balance debited, broadcast
   row `broadcasting`.
4. Cancel pre-accept → `cancelled/refunded`, balance fully restored,
   broadcast `cancelled`.
5. No riders online → 3 broadcast attempts → `no_provider_found` →
   booking `unmatched/refunded` automatically (asynq expiry pipeline,
   end-to-end).
6. Real `request.accepted` event → trip row created **with customer_id**,
   booking `accepted`, payment intent got `provider_type=dispatch` +
   `provider_id` via the new AssignProvider endpoint.
7. `CompleteJob` settled: rider's `provider_payable` credited fare minus
   platform fee (withdrawable via the existing rider wallet surface);
   `trip.completed` flipped the booking to `completed/paid`.
8. Review submit / duplicate 409 / fetch / rider profile alias / list.

Additionally verified **through the real rider endpoints** (no simulation),
using the seeded dev riders (see below): rider OTP login → online through
every availability gate → customer books → request appears in the rider inbox
→ accept → `arrived` / `start` / `proof` (real multipart photo + signature) /
`complete` — with the customer's derived status confirmed at each step
(`accepted → arrived_at_pickup → in_transit → delivered → completed/paid`),
the rider's `provider_payable` credited fare-minus-fee, live GPS tracking
returning the heartbeated fix, and the customer review updating the rider's
public `avg_rating`.

### Dev rider seed

```bash
cd services/driver-dispatch-delivery-service
go run ./cmd/seed     # applies seeds/dev_seed.sql (idempotent, fixed UUIDs)
```

Seeds 5 riders and a small booking/trip history so the customer and rider
apps have something to show without driving a full booking manually. The
bootstrap script also applies the seed automatically.

**Riders:**

| Phone | Name | State | Purpose |
|---|---|---|---|
| `+2348077770001` | Emeka Dispatch | fully verified | ready to go online immediately (motorcycle, `LAG-101-DR`) |
| `+2348077770002` | Bisi Rider | fully verified | ready to go online immediately (dispatch_bike, `LAG-102-DR`) |
| `+2348077770003` | Ngozi Onboarding | mid-onboarding | `verified_to_go_online=false`, no bike yet — go-online fails with `not_verified` |
| `+2348077770004` | Tunde Suspended | suspended | onboarded then suspended — go-online fails with `account_suspended` |
| `+2348077770005` | Chidi Pending | unverified bike | provider verified but the bike is `pending` — go-online fails with `no_verified_vehicle` |

Only riders 1 and 2 belong in `cmd/online`'s `seededRiders` list — 3/4/5 are
deliberately not go-online-ready, to exercise the three distinct
`checkOnlineGate` failure branches.

**Booking/trip history** (synthetic `customer_id`, not a real customer-service
account — it will not show up if you log into the customer app with a real
login, only useful for backend/DB/rider-app testing): 2 completed deliveries
(one on rider 1, one on rider 2) with a full trip lifecycle, delivery proof,
and a review; 1 cancelled booking (customer cancelled pre-match); 1 unmatched
booking (no rider found in time); 1 in-progress booking on rider 1 so the
"Ongoing" bucket / active-trip screens have data too.

**OTP note (differs from hauling):** with `DISPATCH_RIDER_DEBUG_OTP=true` the
login OTP is printed in the dispatch **service logs**
(`development dispatch rider login otp ...`) — it is never in the HTTP
response. The seed includes a reclaim block that renames stray non-seed rows
holding the seed phone/email/plate identifiers, so it is safe even on
databases with prior manual test accounts.

---

## 8. Known gaps / follow-ups

- **No customer realtime channel** — the customer app polls (booking every
  5s, rider location every 8s). If you add a customer-safe websocket or wire
  notification-service, the app already has the fast-path hook.
- **Rider push is still the logging stub** (`LoggingNotificationSender`) —
  pre-existing; riders see requests via inbox polling. Recommended fix: route
  through notification-service (real FCM v1 + device table already exist
  there), the same way hauling notifies truck providers.
- The customer app's client-side search timeout (100s) slightly exceeds the
  full broadcast window (3 × 30s) so the server-side unmatched+refund path
  normally lands first; consider exposing `search_expires_at` on the booking
  response so the app derives it instead of hardcoding.
- On-device visual pass of the Bike Delivery flow is pending a connected
  Android device (the API-level flow is fully verified, including via the
  real rider endpoints).

Resolved since first writing: ~~pricing duplication~~ (now
`shared/go/dispatchpricing`, imported by both surfaces), ~~rider-driven
lifecycle untested~~ (full E2E through the real provider endpoints, above),
plus a fix found by that E2E: customer review inserts now recompute
`providers.avg_rating` in the same transaction, matching the profile
feature's own rating insert.
