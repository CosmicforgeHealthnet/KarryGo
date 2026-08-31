# Dispatch API route register

Sections 17–19 add/remove **zero HTTP routes**. Existing emergency recovery now requires `expected_trip_version` in addition to authenticated operator identity, reason and idempotency. Pre-acceptance request views blank receiver name/phone; terminal provider views and proof omit unnecessary contact, and organization-managed views conceal financial values according to the Fleet visibility boundary. Secret redisplay remains confined to authorized credential endpoints. See [security-observability-certification.md](security-observability-certification.md) for the migration rollout, compatibility notes, metrics and exact certification evidence.

Sections 15–16 reliability reconciliation and gap closure add **zero HTTP routes** and remove none. Router/OpenAPI operations remain those registered below and by the existing feature contracts. The existing provider assignment inbox optionally identifies `PROVIDER_REPLACEMENT` offers with `incident_ref`; acceptance returns the current Trip and `REPLACEMENT_READY`, not custody transfer. Vendor lifecycle and Analytics delivery use their existing internal ingest routes. See [reliability-integration-gap-closure.md](reliability-integration-gap-closure.md) for contracts and exact certification limitations.

This register covers the additive assignment, queue, and multi-vendor planning capability. The established booking, request, provider-inbox, Trip, Commerce mission, Fleet/private, and operational routes remain registered by their existing packages and are unchanged.

| Method | Path | Authority | Rollout gate |
|---|---|---|---|
| POST | `/api/v1/internal/dispatch/v1/assignments` | Vendor Service HMAC v2 | sequential matching enabled |
| GET | `/api/v1/provider/assignment-offers` | authenticated Dispatch provider | sequential matching enabled |
| POST | `/api/v1/provider/assignment-offers/:offerId/accept` | offer-owning Dispatch provider | sequential matching enabled |
| POST | `/api/v1/provider/assignment-offers/:offerId/decline` | offer-owning Dispatch provider | sequential matching enabled |
| POST | `/api/v1/customer/dispatch/missions/:missionId/queue-options` | Delivery Group-owning customer | sequential matching enabled; queue behavior separately gated |
| POST | `/api/v1/internal/dispatch/v1/partner/missions/:missionId/queue-options` | API Gateway HMAC v2 with matching partner source scope | queue enabled and gateway secret configured |
| POST | `/api/v1/internal/dispatch/v1/admin-assisted-assignments` | Admin Backoffice HMAC v2 | admin-assisted flag and caller secret configured; actor/reason/idempotency required |
| POST | `/api/v1/internal/dispatch/v1/delivery-plans` | Vendor Service HMAC v2 | multi-stop missions enabled |
| GET | `/api/v1/internal/dispatch/v1/delivery-plans/by-source/:sourceRef?source_version=:version` | Vendor Service HMAC v2 with exact `VENDOR_COMMERCE` source/version scope | multi-stop missions enabled |
| GET | `/api/v1/internal/dispatch/v1/delivery-plans/:groupRef` | Vendor Service HMAC v2 | multi-stop missions enabled |
| POST | `/api/v1/internal/dispatch/v1/delivery-plans/:groupRef/replans` | Vendor Service HMAC v2 | multi-stop missions enabled |
| POST | `/api/v1/internal/dispatch/v1/delivery-plans/manifests/:manifestRef/custody/activate` | Vendor Service HMAC v2 with exact source/mission/manifest binding | multi-stop missions enabled; parcel identity is not handover auth |
| POST | `/api/v1/internal/dispatch/v1/delivery-plans/manifests/:manifestRef/custody/release` | Vendor Service HMAC v2 with exact source/mission/manifest binding | multi-stop missions enabled |

Canonical contract: `docs/openapi/assignment-queue-multi-vendor-planning.openapi.json`.

## Customer and provider operational composition

The following established routes are the canonical customer/provider surfaces. They are reused and enriched; no parallel mission API was introduced. Booking creation is quote-bound and idempotent when customer serviceability/package-policy rollout is enabled. Read projections expose operational state only and never challenge hashes, plaintext OTP/QR credentials, unrestricted receiver contact details, internal provider ranking, or financial ledger data.

| Method | Path | Authority | Purpose |
|---|---|---|---|
| POST | `/api/v1/customer/dispatch/serviceability` | authenticated customer | Persist an exact route/package admission and fare snapshot |
| POST | `/api/v1/customer/dispatch/bookings` | authenticated customer + `Idempotency-Key` | Create one booking bound to the same customer's current serviceability quote |
| GET | `/api/v1/customer/dispatch/bookings/:id` | booking-owning customer | Read the booking plus additive group/mission/manifest, assignment, handover-state, evidence-summary, recovery and route projections |
| GET | `/api/v1/customer/dispatch/bookings/:id/location` | booking-owning customer | Read minimized current provider location from the established location surface |
| GET | `/api/v1/customer/dispatch/bookings/:id/route-status` | booking-owning customer | Read immutable expected-route version and freshness/deviation state |
| POST | `/api/v1/customer/dispatch/bookings/:id/pickup/verify` | booking-owning customer | Verify provider-held pickup credential |
| POST | `/api/v1/customer/dispatch/bookings/:id/handover/resend` | booking-owning customer | Rotate an owned receiver/customer challenge without returning its secret |
| POST | `/api/v1/customer/dispatch/bookings/:id/delivery-option` | booking-owning customer | Select redelivery, pickup, alternative receiver or cancellation policy; alternative receiver is the canonical receiver-change command |
| GET | `/api/v1/provider/trips/:id` | assigned provider | Read the canonical Trip plus additive stops, current manifest/parcels, safe handover state, allowed actions, recovery and route projections |
| GET | `/api/v1/provider/trips/:id/stops/:stopId/pickup-credential` | assigned provider | Read the provider-held pickup credential for the exact stop/manifest context |
| POST | `/api/v1/provider/trips/:id/stops/:stopId/parcels/scan` | assigned provider | Append an expected-parcel scan |
| POST | `/api/v1/provider/trips/:id/stops/:stopId/evidence-session` | assigned provider | Obtain a mission/stop/purpose-bound evidence capture session |
| POST | `/api/v1/provider/trips/:id/delivery/verify` | assigned provider | Verify receiver-held delivery credential |
| POST | `/api/v1/provider/commerce-missions/:missionRef/exceptions` | mission-assigned provider | Record a structured exception; `PACKAGE_MISMATCH` is the canonical discrepancy command |
| POST | `/api/v1/provider/trips/:id/delivery-attempts/:attemptId/fail` | attempt-owning provider | Record policy-valid receiver-unavailable failure |
| POST | `/api/v1/provider/trips/:id/breakdown` | assigned provider | Record breakdown and enter controlled recovery |
| POST | `/api/v1/provider/commerce-missions/:missionRef/provider-transfers/start` | assigned provider | Start provider transfer under the canonical Commerce mission authority |
| POST | `/api/v1/provider/commerce-missions/:missionRef/provider-transfers/:transferRef/complete` | replacement provider in the bound challenge context | Complete provider transfer without creating a second Trip authority |
| POST | `/api/v1/provider/trips/:id/recipient-confirmation` | assigned provider | Preserve the established recipient-confirmation compatibility contract |

Canonical customer/provider contract: `docs/openapi/customer-provider-dispatch-operations.openapi.json`. The location and route-status endpoints intentionally remain two composed reads because they have different freshness and authorization semantics.

## Secure handover and chain of custody

All routes below are absent unless `DISPATCH_HANDOVER_CHALLENGE_ENABLED=true`. Provider credentials are available only to the currently assigned provider. Customer and internal callers can verify or resend but cannot retrieve plaintext secrets.

| Method | Path | Authority | Purpose |
|---|---|---|---|
| GET | `/api/v1/provider/trips/:id/stops/:stopId/pickup-credential` | assigned Dispatch provider bearer | Display provider-held pickup OTP/QR |
| POST | `/api/v1/provider/trips/:id/stops/:stopId/parcels/scan` | assigned Dispatch provider bearer | Append an expected-parcel scan |
| POST | `/api/v1/provider/trips/:id/stops/:stopId/evidence-session` | assigned Dispatch provider bearer | Create a post-verification capture session |
| POST | `/api/v1/provider/trips/:id/stops/:stopId/evidence` | assigned Dispatch provider bearer + Media relation authorization | Append a MediaRef evidence record |
| POST | `/api/v1/provider/trips/:id/delivery/verify` | assigned Dispatch provider bearer | Verify receiver-held delivery OTP/QR |
| POST | `/api/v1/provider/trips/:id/return/verify` | assigned Dispatch provider bearer | Verify sender-held return OTP/QR and release return custody |
| POST | `/api/v1/customer/dispatch/bookings/:id/pickup/verify` | owning customer bearer | Verify provider-held pickup OTP/QR |
| POST | `/api/v1/customer/dispatch/bookings/:id/handover/resend` | owning customer bearer | Rotate a receiver/customer-owned challenge |
| POST | `/api/v1/customer/dispatch/bookings/:id/return-credential` | owning customer bearer | Issue a new manifest-bound return-to-sender credential |
| POST | `/api/v1/internal/commerce/dispatch/missions/:id/stops/:stopId/pickup/verify` | allowlisted Vendor/API Gateway HMAC v2 + nonce | Source-scoped sender pickup verification |
| POST | `/api/v1/internal/commerce/dispatch/missions/:id/handover/resend` | allowlisted Vendor/API Gateway HMAC v2 + nonce | Source-scoped challenge rotation |
| POST | `/api/v1/internal/commerce/dispatch/missions/:id/stops/:stopId/return-credential` | allowlisted Vendor/API Gateway HMAC v2 + nonce | Issue a source-scoped return credential for the sender |

Mutations require `Idempotency-Key`. The secret-bearing receiver notification uses the authenticated notification-service contract; generic Dispatch outbox events contain references and lifecycle state only. Canonical contract: `docs/openapi/secure-handover-chain-of-custody.openapi.json`.

## Delivery recovery and route integrity

All routes are additive and absent unless their capability flag is enabled.

| Method | Path | Authority | Purpose |
|---|---|---|---|
| POST | `/api/v1/provider/trips/:id/delivery-attempts` | assigned Dispatch provider bearer | Start server-timed receiver grace period |
| POST | `/api/v1/provider/trips/:id/delivery-attempts/:attemptId/contact` | attempt-owning Dispatch provider bearer | Append a controlled contact attempt |
| POST | `/api/v1/provider/trips/:id/delivery-attempts/:attemptId/fail` | attempt-owning Dispatch provider bearer | Record receiver unavailable after grace/contact policy |
| POST | `/api/v1/customer/dispatch/bookings/:id/delivery-option` | booking-owning customer bearer | Select one server-allowed recovery action |
| POST | `/api/v1/provider/trips/:id/breakdown` | assigned Dispatch provider bearer | Record incident and request safe replacement |
| POST | `/api/v1/provider/trips/:id/hub-hold/checkpoints` | assigned Dispatch provider + manifest scan + Media authorization | Append approved hub arrival/release custody evidence |
| POST | `/api/v1/internal/dispatch/v1/trips/:id/emergency-recovery` | Admin Backoffice HMAC v2 + nonce | Record separate controlled emergency recovery action |
| POST | `/api/v1/provider/trips/:id/reroute` | assigned Dispatch provider bearer | Create an immutable next expected-route version |
| POST | `/api/v1/provider/offline-commands` | authenticated Dispatch provider bearer | Reconcile a fingerprinted weak-network envelope |
| GET | `/api/v1/customer/dispatch/bookings/:id/route-status` | booking-owning customer bearer | Read minimized location plus freshness state |

All mutations require `Idempotency-Key`. Canonical contract: `docs/openapi/delivery-recovery-route-integrity.openapi.json`.
