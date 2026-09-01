# Delivery recovery and route integrity

These capabilities extend the canonical Dispatch `Booking -> Request -> Trip` execution path. They do not create another Trip, financial ledger, media store, notification transport, routing provider, or handover-secret authority. All six rollout flags default to `false`.

## Delivery recovery

An assigned provider starts a delivery attempt on the canonical Trip. Server time opens the configured grace period. Contact attempts are append-only, bounded to controlled method/outcome values, and at least one contact attempt is required before a receiver-unavailable outcome can be recorded. The server derives allowed recovery actions from `dispatch_delivery_recovery_v1`; the provider does not select the commercial outcome.

The owning customer may select an allowed same-day retry, reschedule, alternative receiver, return-to-sender, approved hub hold, or operations review. Alternative-receiver selection invalidates existing secure-handover challenges before changing the receiver. Additional charges require a matching Wallet payment authorization. Return-to-sender first enforces the Section 6 operating-zone policy, then creates a linked return Delivery Mission with authoritative road-route evidence, an immutable parcel manifest, and starts the mission through the existing assignment/Booking/Trip bridge. An unavailable or incompatible route fails closed. Retries reuse the same return mission and assignment operation.

Hub hold is fail-closed. `DISPATCH_APPROVED_HOLD_HUBS` maps configured hub references to platform authority references. Arrival and release checkpoints require the current provider, an exact scan of every parcel in the active manifest, and Media Service authorization binding the evidence to that exact mission and parcel set. Checkpoints are append-only. A MediaRef or a hub name alone is never authority.

Breakdown/replacement reports preserve the original incident and use the existing sequential-assignment authority. Provider-supplied strategy values are ignored. Emergency recovery is a separate HMAC-v2 Admin Backoffice operation with nonce replay protection, operator identity, reason, immutable audit record, and no ability to mark OTP/QR verification successful.

## Route integrity and weak-network replay

`trip.started` creates version 1 of the expected route using the existing Section 6 route provider. An authorized provider reroute creates a new current version and retains prior route evidence. PostgreSQL permits only the controlled current-to-terminal route status transition; evidence fields and historical rows cannot be updated or deleted.

Provider location events are evaluated against server-owned corridor, wrong-way, stationary, staleness, clock-skew, and accuracy thresholds. Alerts are operational signals only. They do not assign misconduct, financial consequences, or automatic provider punishment. The customer route-status read is ownership-scoped, exposes only the minimum location projection, and explicitly reports stale data.

Offline envelopes bind provider, device ID, device command ID, client timestamp, last-known server version, payload, and `Idempotency-Key`. Same identity plus the same fingerprint replays the stored result; changed semantics conflict. Location fixes may reconcile. Evidence references remain pending Media authorization. Parcel scan, pickup, delivery, and provider transfer commands fail closed until their online manifest/handover authorities validate them. No offline command can verify an OTP/QR, transfer custody, or complete a Trip.

## Rollout configuration

| Variable | Default | Purpose |
|---|---:|---|
| `DISPATCH_DELIVERY_RECOVERY_ENABLED` | `false` | Receiver-unavailable attempt and recovery workflow |
| `DISPATCH_RETURN_WORKFLOW_ENABLED` | `false` | Linked return mission; requires recovery, routing, and sequential assignment |
| `DISPATCH_HUB_HOLD_ENABLED` | `false` | Approved hub-hold flow; requires configured hub authorities |
| `DISPATCH_REPLACEMENT_HANDOVER_ENABLED` | `false` | Breakdown/replacement and controlled emergency recovery |
| `DISPATCH_ROUTE_ALERTS_ENABLED` | `false` | Versioned routes and operational alerts |
| `DISPATCH_OFFLINE_REPLAY_ENABLED` | `false` | Weak-network command ledger; requires route alerts |

Thresholds and time windows are centralized in Dispatch configuration. Enabling a dependent capability without its authority causes startup validation to fail rather than silently degrading security.

## Ownership boundaries

- Dispatch owns Trip execution, Delivery Missions, parcel-manifest relationships, delivery attempts, recovery state, expected routes, and operational alerts.
- payment-wallet-service remains the sole ledger and payment-authorization authority.
- media-file-service remains the byte and media-attachment authority.
- notification-service remains the delivery transport authority.
- securehandover remains the OTP/QR challenge, attempt, expiry, replay, and verification authority.
- Section 6 routing/serviceability remains the route-evidence authority.
- Fleet/private execution and the existing provider-transfer flow are reused and unchanged.

PostgreSQL concurrency tests are gated by `DISPATCH_TEST_DATABASE_URL`. An absent canonical test database must be reported as infrastructure-blocked, never as a pass.
