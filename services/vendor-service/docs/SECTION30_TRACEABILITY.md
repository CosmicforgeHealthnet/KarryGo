# Section 30 — Truck / Hauling Commerce Integration

Status: implementation complete; canonical PostgreSQL replay certification is
environment-gated.

## Authority split

- Vendor Service owns Commerce order orchestration, fulfilment-plan evidence,
  customer-approved quote terms, and opaque platform mission projection.
- `driver-hauling-service` remains the canonical Van/Truck authority for
  truck providers, bookings, matching, custody, OTP/QR handover and trip state.
- Wallet remains the sole financial ledger and settlement authority.
- The existing shared HMAC v2 service-auth boundary is reused; no second auth,
  OTP, Wallet, Media or Hauling service was created.

## Implemented controls

| Requirement | Implementation | Evidence |
|---|---|---|
| Van/Truck quote serviceability | `shared/go/commercequote` + Hauling `commercequote` now accept committed multi-pickup stops and compute the route across all legs | `go test ./shared/go/commercequote ./services/driver-hauling-service/internal/features/commercequote` |
| Mission execution | `shared/go/commercemission` carries operation refs, plan binding, stops, Parcels, manifest version and handling evidence | `go test ./shared/go/commercemission ./services/driver-hauling-service/internal/features/commercemission` |
| Idempotency/fingerprint | Hauling `commerce_missions.operation_ref` is unique; same operation with changed semantic fingerprint returns conflict | `014_commerce_mission_execution_controls`; `TestCreate_RejectsSameOperationWithDifferentFingerprint` |
| Unknown outcome read-back | Vendor adapter reads Hauling by operation ref after transport-unknown create/cancel outcomes | `hauling_commerce_adapter.go` |
| Cancellation | Hauling exposes HMAC-protected operation read and pre-custody cancel; post-custody states fail closed | `commercemission/http.go`, `repository.go` |
| Scheduling | First committed pickup ready window is persisted as booking `scheduled_at`; timezone evidence is retained on mission metadata | `TestCreate_CarriesMultiStopScheduledManifestEvidence` |
| Existing lifecycle reuse | Hauling booking and `BookingService.StartCommerceMatching` remain the execution/matching authority | `CreateMissionWithBooking` transaction |
| Kill switch | `HAULING_COMMERCE_INTEGRATION_ENABLED` and `HAULING_COMMERCE_INTEGRATION_KILL_SWITCH` gate internal commerce exposure/writes | Hauling config/cmd wiring |

## Routes

Hauling commerce internal routes after Section 30:

- `POST /api/v1/hauling/internal/commerce/quotes`
- `POST /api/v1/hauling/internal/commerce/missions`
- `GET /api/v1/hauling/internal/commerce/missions/operations/{operationRef}`
- `POST /api/v1/hauling/internal/commerce/missions/operations/{operationRef}/cancel`

OpenAPI: `services/driver-hauling-service/docs/openapi/commerce-hauling-missions.openapi.json`.

## Deferrals

- PostgreSQL apply/rollback/reapply certification is blocked unless
  `HAULING_SERVICE_TEST_DATABASE_URL`/canonical local DB infrastructure is
  available.
- No Taxi goods delivery, Section 32 wholesale, Section 37, Section 41,
  provider app or frontend implementation was started.
