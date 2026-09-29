# Customer backend Sections 9–11 audit

Starting branch: `staging`. Starting HEAD and fetched `origin/staging`:
`5b2050a896942ddb4d191a3a90cb4af49342bd02`. Working tree clean.
Scope: backend only. Historical migrations and pricing remain untouched.

## Gap register (before implementation)

| Area | Verified existing implementation | Required extension |
|---|---|---|
| Customer authentication/profile/locations | `customer-service/internal/features/auth`, `profile`, `saved_locations`; mounted in `cmd/main.go` | Regression verification, no replacement |
| Trusted contacts | `profile/{models,repositories,usecases,http}/trusted_contacts.go` | Regression verification |
| Mobility and guests | `mobility_context`; migration 007; versioned bearer routes | Regression verification; Taxi already has passenger requirement snapshots |
| Notification | `notifications/http`, backend Notification client | Preserve boundary |
| Activity | `movement_history`, migrations 008/009, shared versioned events, grouping ingress | Audit reconciliation across authorities; do not create another projection |
| Taxi movement | `internal/movement`, booking transaction writer, Taxi outbox, reconciler in main | Reuse |
| Dispatch/Hauling movement | existing sweep publishers and shared movement outbox | Verify recovery and sweep completeness |
| Commerce grouping | existing fulfilment/grouping projections; logistics own movement | Prompt drift: Vendor is a grouping publisher, not a logistics movement authority |
| Share sessions | No journey-sharing implementation found across services/shared/deploy | Create persistent policy aggregate and safe read adapters |
| Taxi customer tracking | Existing provider telemetry only | Add safe booking-bound read; no new location table |
| Ingress | Gateway rejects internal path segments | Extend regression cases for N08–N11 |

## PROVIDER_API_DEPENDENCY_MATRIX

| Requirement/domain | Owner | Existing API/use case and source | Auth | Status | Consumer/change | Verification |
|---|---|---|---|---|---|---|
| Ride identity | Taxi | `provider_auth`, `provider_profile`, identity internal lookup | provider bearer / internal HMAC | REUSE | Taxi-only safe display projection | Safe DTO tests |
| Ride assignment/vehicle | Taxi | `booking/repositories/postgres_booking_repository.go`; `taxi_bookings.provider_id/vehicle_id`, provider vehicles | provider/customer bearer | REUSE | N07/N08 current assignment | Assignment/ownership tests |
| Ride lifecycle | Taxi | `booking/http/routes.go`: accept, en-route-pickup, arrived, verify-pickup, start, complete, cancel | provider bearer | REUSE | Share eligibility and progress | Existing lifecycle tests |
| Ride active location | Taxi | POST `/api/v1/taxi/provider/location/events`; `booking/usecases/telemetry.go`, `booking/telemetry/repository.go`; `taxi_trip_location_points` | provider bearer | EXTEND | Latest validated booking/current-provider point read for N07/N08 | Telemetry and safe read tests |
| Ride share read | Taxi | No safe Customer Service share read exists | HMAC v2, customer-service only | CREATE | N08 safe adapter over owning booking/telemetry | HMAC/privacy tests |
| Send lifecycle/assignment | Dispatch | `trip/service.go`, `trip/repository.go`, canonical trip and assignment | provider/customer bearer | REUSE | N09 safe trip adapter | Existing trip tests + adapter tests |
| Send location | Dispatch | `booking.Service.GetLiveLocation`, availability live store, existing customer location/socket | customer/provider bearer | EXTEND | N09 must prove trip binding; global points cannot be silently treated as trip-specific | Binding test required |
| Truck lifecycle/assignment | Hauling | `booking/usecases/booking_service.go`, haulage booking repository | provider/customer bearer | REUSE | N10 safe booking adapter | Existing booking tests |
| Truck location | Hauling | `GetBookingLocation`, provider availability heartbeat | customer/provider bearer | EXTEND | N10 must prove trip binding, suppress unbound global telemetry | Binding test required |
| Commerce mission scope | Vendor | `fulfilmentgroup.PlatformMissionRef`, `commerce_order_missions`, logistics mission integrations | vendor/customer bearer, HMAC | EXTEND | N11 ownership/mission-safe logistics read-through | Mission-scoping integration required |
| Fleet/private fields | Fleet | Existing provider/fleet authority surfaces | provider/HMAC | NOT REQUIRED | No fleet financial or private fields in sharing | Serializer privacy tests |
| Credentials/custody | Taxi/Dispatch/Hauling | Existing passenger/parcel/Goods/handover use cases | bounded existing auth | REUSE | Excluded entirely from share DTO | Credential absence tests |

## Validation prerequisites

Go 1.26.3 is available. Docker CLI exists, but the Linux Docker daemon was not
running at audit time. Database integration, migration apply/rollback, real
service E2E, and database contention tests must be reported as unverified unless
a real database becomes available. Skipped integration tests are not certification.

## Execution follow-through

Docker was started after the audit. Real PostgreSQL/Redis tests, migration probes, source adapter tests and regression results are recorded in docs/customer-sections9-11-report.md. The audit's initial infrastructure limitation is historical, not a claim that the subsequent database suites were skipped.

