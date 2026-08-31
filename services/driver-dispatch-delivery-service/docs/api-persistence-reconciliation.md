# Dispatch API and persistence reconciliation

This document reconciles the roadmap API/persistence responsibilities with the canonical implementation delivered by Dispatch Sections 4–12. It does not establish another Booking, Trip, mission, Wallet, Media, notification, handover, recovery, or route authority.

## Authority flow

Customer Booking remains the customer commercial request. Delivery Group and Delivery Mission are the multi-vendor planning authority. `dispatch_mission_execution_links` correlates an executable mission to the canonical Booking and Trip. The existing Commerce Mission remains the internal commerce execution/compatibility contract. Booking/Request/Trip remains rider execution authority. Wallet owns financial state, Media owns bytes and attachment authorization, and notification-service owns delivery transport.

## API mapping

| Responsibility | Target semantic path | Canonical route | Status | Authority / rollout |
|---|---|---|---|---|
| Customer serviceability | `POST /customer/dispatch/serviceability` | `POST /api/v1/customer/dispatch/serviceability` | exact | customer JWT; serviceability/package flags; idempotency fingerprint |
| Customer booking create | `POST /customer/dispatch/bookings` | `POST /api/v1/customer/dispatch/bookings` | extended | customer JWT; protected rollout requires owned current quote and `Idempotency-Key` |
| Customer booking read | `GET /customer/dispatch/bookings/:id` | `GET /api/v1/customer/dispatch/bookings/:id` | extended | owner scope; additive secret-free operations projection |
| Customer pickup verify | same | `POST /api/v1/customer/dispatch/bookings/:id/pickup/verify` | exact | owner + current manifest/challenge/version |
| Handover resend | same | `POST /api/v1/customer/dispatch/bookings/:id/handover/resend` | exact | owner; safe status/masked destination only |
| Receiver change | dedicated target | `POST /api/v1/customer/dispatch/bookings/:id/delivery-option` with `ALTERNATIVE_RECEIVER` | semantic | owner + recovery policy/cutoff/version/idempotency |
| Delivery option | same | `POST /api/v1/customer/dispatch/bookings/:id/delivery-option` | exact | owner + server allowed-action set |
| Customer location | same | `GET /api/v1/customer/dispatch/bookings/:id/location` plus `GET .../:id/route-status` | composed | owner; live location and immutable route freshness remain deliberately separated |
| Provider mission read | `GET /provider/trips/:id/mission` | `GET /api/v1/provider/trips/:id` | extended semantic | assigned provider; canonical Trip gains stops/manifest/parcels/actions/handover/recovery/route projection |
| Pickup credential | same | `GET /api/v1/provider/trips/:id/stops/:stopId/pickup-credential` | exact | assigned provider/current stop; no hashes or delivery secret |
| Parcel scan | same | `POST /api/v1/provider/trips/:id/stops/:stopId/parcels/scan` | exact | assigned provider + current manifest + idempotency |
| Evidence session | same | `POST /api/v1/provider/trips/:id/stops/:stopId/evidence-session` | exact | assigned provider + stage/manifest gate |
| Package discrepancy | dedicated target | `POST /api/v1/provider/commerce-missions/:missionRef/exceptions` with `PACKAGE_MISMATCH` | semantic | mission-assigned provider; server policy, evidence ref, no fare authority |
| Delivery verify | same | `POST /api/v1/provider/trips/:id/delivery/verify` | exact | assigned provider submits receiver-held credential |
| Failed delivery | legacy single command | canonical delivery-attempt start/contact/fail routes; legacy `/trips/:id/fail` retained | stronger semantic + legacy | assigned provider + server grace/contact/recovery policy |
| Breakdown | same | `POST /api/v1/provider/trips/:id/breakdown` | exact | assigned provider + recovery flag/idempotency |
| Provider transfer verify | dedicated target | commerce mission transfer start/complete plus `PROVIDER_TRANSFER` challenge | semantic | old/replacement provider identity and manifest binding |
| Recipient confirmation | same | `GET /api/v1/provider/trips/:id/recipient-confirmation` | legacy compatibility | assigned provider; secure handover remains stronger authority |
| Internal quote | same | `POST /api/v1/internal/commerce/dispatch/quotes` | exact | allowlisted Vendor HMAC v2 + nonce + idempotency |
| Internal mission create | mission target | `POST /api/v1/internal/dispatch/v1/delivery-plans` | semantic | allowlisted Vendor HMAC v2; source/version/fingerprint; no duplicate mission engine |
| Internal read by source | mission target | `GET /api/v1/internal/dispatch/v1/delivery-plans/by-source/:sourceRef?source_version=N` | new gap route | allowlisted Vendor HMAC v2; source type fixed to `VENDOR_COMMERCE` |
| Internal read by ID | mission target | `GET /api/v1/internal/dispatch/v1/delivery-plans/:groupRef` | semantic | allowlisted Vendor HMAC v2 |
| Internal cancel | same | existing Commerce mission cancellation route | semantic | HMAC source ownership, cutoff/version/idempotency, Wallet projection only |
| Internal pickup verify | stop-scoped target | `POST /api/v1/internal/commerce/dispatch/missions/:id/stops/:stopId/pickup/verify` | stronger exact context | allowlisted HMAC v2 + nonce |
| Internal resend | same | `POST /api/v1/internal/commerce/dispatch/missions/:id/handover/resend` | exact | allowlisted HMAC v2 + nonce |

API Gateway remains partner authentication authority, Vendor Service remains commerce source authority, Fleet Partner remains organization authority, and Admin Backoffice remains admin-actor authority. No public Dispatch route trusts body-supplied source, customer, provider, or admin identity.

## Idempotency and versioning

Booking create now has a customer-scoped database uniqueness key and request fingerprint. Serviceability uses customer/key uniqueness plus a fingerprint. Assignment, Queue, execution links, plan/replan, custody, handover, evidence, recovery, route, and offline commands retain their established operation keys/fingerprints and optimistic/source versions. A repeated key with the same semantic request reads back the authoritative result; a changed fingerprint conflicts. HMAC v2 internal mutations bind caller, timestamp, nonce, method, path and payload.

## Response and secret policy

Customer and provider projections are additive and nullable. They contain references, bounded state, safe counters, masked destinations where applicable, and server-allowed actions. They exclude plaintext receiver OTP/QR secrets, secret hashes, encryption material, evidence/media access secrets, provider candidate rankings, unrestricted customer/provider PII, Wallet credentials and ledger internals.

## Persistence mapping

| Responsibility | Canonical table(s) | Migration | Important invariant |
|---|---|---|---|
| Delivery Group | `dispatch_delivery_groups` | 000044 | scoped source identity/version and optimistic version |
| Delivery Mission / execution | `dispatch_delivery_missions`, `dispatch_mission_execution_links` | 000044/000046 | one canonical Booking per mission and source operation |
| Mission Stops | `dispatch_mission_stops`, `dispatch_stop_expected_parcels` | 000044/000045 | stable stop refs and exact expected parcels |
| Parcels | `dispatch_parcels` | 000044/000046 | stable generated KarryGo/scannable uniqueness |
| Manifests/membership | `dispatch_parcel_manifests`, `dispatch_manifest_parcels` | 000044/000046 | immutable history; logical version and one-current uniqueness |
| Active custody | `dispatch_active_custody_manifest_parcels`, `dispatch_manifest_custody_operations` | 000045/000046 | parcel primary key permits one active custody manifest |
| Serviceability quote | `dispatch_serviceability_quotes` | 000044 | customer/key uniqueness, expiry and source type check |
| Assignment offer | `dispatch_assignment_candidates`, `dispatch_assignment_offers`, `dispatch_assignment_audit` | 000045/000046 | one live offer, one winner, append-only audit |
| Queue | `dispatch_queue_entries`, `dispatch_queue_operations` | 000045 | one active entry per mission; versioned server policy/Wallet projection |
| Handover challenge/attempt | `dispatch_handover_challenges`, `dispatch_handover_attempts`, `dispatch_handover_operations` | 000047 | one active context/stage; no plaintext; attempts append-only |
| Evidence | `dispatch_evidence_requirements`, `dispatch_evidence_capture_sessions`, `dispatch_parcel_scan_events`, `dispatch_evidence_refs` | 000047 | one active capture session; scans/evidence append-only |
| Delivery attempt/recovery | `dispatch_delivery_attempts`, `dispatch_delivery_attempt_operations`, `dispatch_delivery_contact_attempts`, `dispatch_delivery_recovery_actions` | 000048 | one open attempt and one active recovery action; contacts append-only |
| Return/hub/incidents | `dispatch_return_workflows`, `dispatch_hub_hold_operations`, `dispatch_hub_hold_checkpoints`, `dispatch_delivery_incidents`, `dispatch_emergency_recovery_actions` | 000048 | lineage retained; current incident uniqueness; immutable checkpoints/actions |
| Provider transfer | existing `commerce_provider_transfers` and challenge/evidence tables | 000043/000047 | mission/provider/manifest identity binding and idempotent state machine |
| Route/tracking/offline | `dispatch_expected_routes`, `dispatch_route_location_fixes`, `dispatch_route_alerts`, `dispatch_device_commands` | 000048 | one current route, unique versions/commands, append-only observations |
| Booking/Trip compatibility | `dispatch_bookings`, `delivery_requests`, `trips`, execution links | existing + 000046/000049 | canonical Trip per Booking; mission correlation; protected create quote/idempotency binding |

## Migration 000049

`000049_booking_serviceability_idempotency` is the only reconciliation migration. It additively binds protected booking creation to `dispatch_serviceability_quotes`, stores the quote version/fingerprint, enforces the quote/customer pair with a composite foreign key, and provides a partial customer-scoped unique create key with request fingerprint. All columns are nullable or safely defaulted for existing legacy rows. Its down migration removes only the new constraint, indexes and columns; it does not remove Booking or quote data. Migrations 000048 and earlier were not changed by this reconciliation.

## Database certification boundary

Static migration-shape and non-database tests run without infrastructure. Apply/rollback/reapply and concurrent constraints use only `DISPATCH_TEST_DATABASE_URL`; they skip rather than inventing a PostgreSQL target when it is absent. Runtime-only claims remain `NOT_RUNTIME_CERTIFIED` until that canonical environment executes the suite.
