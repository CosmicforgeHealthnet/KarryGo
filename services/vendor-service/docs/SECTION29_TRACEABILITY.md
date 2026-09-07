# Section 29 — Taxi Goods Delivery Traceability

## Boundary

Vendor Commerce CAR delivery uses the distinct Taxi Goods authority at
`/api/v1/taxi/internal/commerce`. It never calls Taxi's passenger/customer
booking API and never converts an order to `ride_purpose=parcel`.

| Requirement | Authority / implementation |
|---|---|
| CAR quote | Taxi `POST /api/v1/taxi/internal/commerce/quotes`; persisted `taxi_goods_quotes`; Taxi Goods pricing namespace |
| Mission create and recovery | Taxi mission create/read/read-by-operation/cancel routes; Vendor shared client and frozen quote binding |
| Binding changes | Vendor-scoped receiver-change command and operations-only pre-custody provider-reassignment command; both versioned, idempotent, audited, and challenge-invalidating |
| Provider selection | Taxi matching over existing Redis proximity plus durable compliance/cargo filters |
| Passenger/Goods exclusion | Existing `provider_operational_state`; `goods_mission_id`; DB constraint forbids simultaneous booking and goods references |
| Capability and opt-in | `taxi_goods_provider_settings.capability_status` and independent `goods_available`; legacy `accepts_parcels` remains an additional product preference |
| Cargo | Versioned vehicle cargo profile with weight, volume, package, dimension, handling, and verification gates |
| Custody | Distinct pickup and delivery challenge stages; provider-held pickup credential verified by Vendor; receiver-held delivery credential verified by provider |
| Credential security | OTP and opaque QR are independently generated, HMAC-bound, hash-only at rest, alternative single-use credentials; resend rotates both |
| Evidence | Taxi stores Media references only and validates mission ownership through Media File Service |
| Money | Wallet Commerce delivery release only; provider type `taxi`; no passenger debit or Taxi-side customer charge |
| Notification | Notification Service transports receiver delivery credentials |
| Events | Taxi lifecycle mutations append safe events to `taxi_outbox`; a leased `SKIP LOCKED` publisher signs deterministic fan-out deliveries to the existing Vendor ingress, with bounded retry, lease recovery and dead-letter state |
| Runtime gate | Taxi and Vendor `TAXI_GOODS_DELIVERY_ENABLED`, both false by default |

## Shared contracts

`shared/go/commercequote` adds `CAR`. `shared/go/commercemission` adds `TAXI`,
`CAR`, and the Taxi API base. MOTORCYCLE/Dispatch and VAN/TRUCK/Hauling retain
their existing meanings.

## Frontend follow-up

No frontend was modified. A later Taxi Provider UI must add a separately
labelled Goods availability toggle, cargo-profile verification status, privacy-
safe Goods job cards, provider-held pickup OTP/QR display, receiver credential
entry/scanning at delivery, and Media-backed evidence capture. It must not
present Vendor financial allocations or reuse passenger trip-start controls as
goods custody proof.
