# Section 28 traceability — Dispatch delivery integration

## Authority decisions

| Requirement | Authority / evidence | Status |
|---|---|---|
| Route-aware Commerce quote | Dispatch `commercequote` V1 service + configured route provider | Implemented |
| Canonical mission, stops, attempts, legs | Dispatch `commercemission` + migration `000042` | Implemented |
| Provider marketplace and ranking | Existing Dispatch request/marketplace authority | Reused |
| Immutable commercial plan and parcel identity | Vendor fulfilment-group authority, bound through `shared/go/dispatchcommerce` | Reused |
| Vendor mission view | `PlatformMissionRef` projection only | Implemented |
| Refund and payable decisions | Existing Vendor/Wallet Sections 23–24 | Reused; Dispatch has no authority |
| Pickup/delivery handover | Existing shared handover + Dispatch trip/booking authority | Reused and extended per pickup stop |

## Implementation map

- Shared wire/client: `shared/go/dispatchcommerce`.
- Dispatch route quotes: `internal/features/commercequote/multi_stop*.go` and
  `route_provider.go`.
- Dispatch missions, custody and recovery:
  `internal/features/commercemission/multi_stop*.go`,
  `pickup_handover.go`, `mission_commands.go`, `provider_eligibility.go` and
  `recovery_worker.go`.
- Dispatch schema: `000042_commerce_multistop_delivery`.
- Vendor adapter/projection: `cmd/dispatch_delivery_adapter.go`,
  `cmd/fulfilment_operations.go`, and `internal/features/orders` fulfilment,
  cancellation and Dispatch-event files.
- HTTP contract:
  `services/driver-dispatch-delivery-service/docs/openapi/commerce-dispatch-delivery.openapi.json`.

## Contract facts

- Dispatch routes: 109 before, 122 after; 13 added.
- Vendor routes: 840 before, 841 after; one HMAC-v2 lifecycle callback added.
- Dispatch migration tip: `000041` before, `000042` after.
- Vendor migration tip remains `000103`; no Section 28 Vendor migration.
- No roadmap-number production source filename or production directory exists.

## Certification boundary

Static route/OpenAPI parity, migration shape, domain/unit tests, build, vet,
formatting and diff checks are runnable without PostgreSQL. Canonical migration
replay and database race suites require `DISPATCH_TEST_DATABASE_URL` /
`DISPATCH_DATABASE_URL` and `VENDOR_SERVICE_TEST_DATABASE_URL` /
`VENDOR_DATABASE_URL`. A missing database is reported as infrastructure
blocked, never as a pass.
