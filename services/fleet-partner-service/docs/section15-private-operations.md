# Section 15 private operations

Section 15 is implemented as organization-owned aggregates in the Fleet
database. It does not widen the marketplace `source_service` vocabulary and it
does not make Fleet authoritative for canonical trips, provider identity,
vehicle identity, wallet ledgers, document bytes, or customer identity.

## Data and migrations

Migrations `000025` through `000033` add service catalogue, private orders and
stops, shipments/cargo, packages/scans/custody, manifests/snapshots, recurring
templates/occurrences, bulk imports/rows, and private assignments/execution
operations. All changes are additive and have paired down migrations.

Immutable history, decision, scan/custody, snapshot, occurrence-attempt,
import-attempt, and execution-attempt records use append-only database
triggers. Command state changes use expected versions. Creation and other
replay-sensitive commands use the shared idempotency store.

## Routes and authorization

Corporate routes are mounted at `/fleet-partner/v1` behind bearer
authentication, tenant resolution, branch scope, and semantic permissions.
Provider Accept and Decline are mounted only at
`/internal/fleet-partner/v1/private-assignments/*` behind HMAC v2 service
authentication. The caller must own the assignment's execution service and
must assert the selected provider. Provider responses omit commercial and
unnecessary customer data. Provider apps reach those routes through the
authoritative Dispatch or Hauling provider bearer surface; the internal HMAC
routes are not public provider endpoints.

The machine-readable contract is
`docs/openapi/section15-private-operations.openapi.json`. Its parity test reads
the same declarative manifests used by route registration and checks method,
path, operation ID, authentication class, permission, feature flag,
idempotency, and expected-version metadata in both directions.

## Workers

The API and dedicated worker binaries use the same Section 15 worker starter.
Recurring occurrence generation, bulk validation/commit, and assignment expiry
start only when their individual flags are true and the global integration kill
switch is false. Claims are bounded and use PostgreSQL `SKIP LOCKED` leases;
attempt records contain only safe error codes.

Private acceptance creates a durable execution operation but never a canonical
trip. The leased submission worker calls production Dispatch or Hauling HMAC
adapters, performs stable-reference read-back after ambiguous outcomes, and
converges cancellation through the authority service. Authoritative events are
processed with the inbox, source-version projection, assignment/shipment/order
history, audit and outbox in one transaction. Version gaps and conflicts create
leased reconciliation cases with heartbeat, bounded retry, dead-letter and
permissioned replay. Taxi is always disabled. All production execution flags
remain false by default. No no-op adapter or simulated success is used.

## Feature flags

All new writes and workers default false. Important gates include:

- `PACKAGE_SCANNING_ENABLED`, `MANIFESTS_ENABLED`,
  `RECURRING_ORDERS_ENABLED`, and `BULK_IMPORTS_ENABLED`.
- `PRIVATE_ORDERS_ENABLED` and `PRIVATE_ASSIGNMENTS_WRITES_ENABLED`.
- `PRIVATE_DISPATCH_EXECUTION_ENABLED` and
  `PRIVATE_HAULING_EXECUTION_ENABLED`.
- `PRIVATE_TAXI_EXECUTION_ENABLED` is documented as false and cannot enable a
  Taxi adapter.
- `FLEET_INTEGRATIONS_KILL_SWITCH` and the domain kill switches override new
  writes and worker claims. Decline of an already-issued assignment remains a
  safety action and is not treated as an external execution write.

## Operations and rollback

Before enabling a worker, apply migrations, verify `/ready`, confirm the
authority client base URL and HMAC secret, and run the disposable-PostgreSQL
contract suite. Disable the individual write/worker flag first during an
incident; use the global kill switch when multiple integrations are unsafe.
Unknown external write outcomes must be read back before any retry.

Rollback is flag-first. Stop claims, wait for leases to expire, drain or retain
the outbox, and run paired down migrations only after confirming no retained
Section 15 data is required. Append-only audit/history records should normally
be retained rather than rolled back.

## Explicit boundary

Section 16 remains deferred. Section 15 does not expose a dispatch board,
candidate recommendations, route plans, optimization, control tower, complete
tracking projections, geofences, or operational-alert APIs. It also does not
expose a generic Partner API or a production Taxi operation.
