# Section 16 — Dispatch, routes, live control and tracking

## Ownership

Fleet Partner owns corporate planning and organization-scoped projections:
dispatch plans, route plans/stops, candidate snapshots, publication snapshots,
tracking projections, geofences, alerts and their audit/outbox records.

Dispatch and Hauling remain authoritative for canonical requests,
reservations, trips, provider actions, cancellation, completion and live trip
lifecycle. Provider location is accepted only from their signed internal event
surface. Fleet never polls or writes another service database, never creates a
canonical trip during route publication, and never claims stale location is
live. Provider/customer commercial fields are absent from board and control
tower DTOs.

Taxi/passenger stops and publication are fail-closed. Workforce, maintenance,
warehouse, finance extensions and automation remain deferred to later sections.

## API and state

The corporate prefix is `/fleet-partner/v1`. The dispatch board, unassigned
view, candidates, plans, routes/stops, control tower, tracking, geofences and
alerts are declared once in `internal/features/dispatch/http.go`; registration
and the OpenAPI parity test consume that catalogue. Published route snapshots
are immutable. Draft stop deletion normalizes the remaining sequence. Replan
creates a replacement route and omits completed work.

Plan states: `DRAFT`, `READY`, `APPROVED`, `PUBLISHED`, `IN_PROGRESS`,
`COMPLETED`, `CANCELLED`.

Route states: `DRAFT`, `OPTIMIZATION_PENDING`, `OPTIMIZED`, `READY`,
`APPROVED`, `PUBLISHED`, `IN_PROGRESS`, `COMPLETED`,
`PARTIALLY_COMPLETED`, `FAILED`, `CANCELLED`.

Stop, alert and tracking freshness transitions are centralized in
`internal/features/dispatch/domain.go`; repositories cannot accept an arbitrary
HTTP status string.

## Candidates and publication

Candidate scores are deterministic (0–100). Every component names the rule,
points and explanation. Membership, provider status, capability, availability,
vehicle/truck authority, capacity, branch, cargo, safety, reservation and
active-work conflicts are hard blocks. Stale/unknown/gapped location is a
warning and never supplies fictitious proximity. Snapshots carry source
versions, an input fingerprint and a short expiry.

Publication locks the route, checks its expected version, branch, stops,
provider, vehicle/truck, availability and the current candidate evidence. Hard
blocks cannot be overridden. An allowed warning needs
`dispatch.override_warning` and an explicit reason. The same transaction writes
the immutable publication snapshot and creates Section 15 assignments with
`work_origin=ROUTE_STOP` and `execution_service=DISPATCH|HAULING`. Provider
Accept/Decline remains mandatory. No execution operation or canonical trip is
created until provider acceptance.

## Optimization

`OptimizationPort` is provider-neutral. The production adapter submits and
reads back an opaque provider job; tests use a deterministic fake. Inputs are
fingerprinted by route version, stops and constraints. The shared PostgreSQL
job queue supplies deduplication, `SKIP LOCKED`, leases, heartbeats, expired
lease recovery, retry and dead-letter. Unknown submissions enqueue read-back;
stale-version results become reconciliation-required and never overwrite the
route. `ROUTE_OPTIMIZATION_ENABLED` is rejected at startup without provider
configuration. Manual route approval remains available while optimization is
off.

## Tracking, geofences and alerts

`POST /internal/fleet-partner/v1/tracking/events` is HMAC-v2 only and accepts
the two authoritative service names. Source time and receive time are separate.
Event IDs deduplicate delivery; lower versions cannot overwrite; skipped
versions mark `GAPPED`; same-version payload conflicts create reconciliation
records. A later ordinary event cannot silently clear a gap: only an
authoritative event whose `reconcilesThroughVersion` covers its source version
restores `CURRENT` and records `lastReconciledAt`. Freshness thresholds are
configurable and retained coordinates are deleted after the retention window.
Logs and outbox payloads omit coordinates.

Circle and simple polygon geometry is validated without a major GIS dependency.
Self-intersecting polygons are rejected. Stale/gapped locations suppress
high-confidence geofence decisions. Trigger uniqueness prevents duplicate
events and alert storms.

Alerts are unique by an open condition fingerprint. Acknowledgement assigns the
operator; resolution requires a code and reason; terminal alerts cannot be
mutated. SLA escalation writes an event containing a notification intent—actual
delivery remains Notification Service ownership.

## Workers and replay

Section 16 registers optimization submit/read-back, tracking reconciliation,
geofence evaluation, alert generation/escalation and route progress handlers on
the existing durable job worker. The generic worker owns bounded claims,
heartbeat, recovery, classified retry, dead-letter and graceful shutdown.
The periodic Section 16 maintenance sweep applies retention/freshness policy,
creates deduplicated stale/decline/expiry alerts and enqueues route aggregation
from authoritative Section 15 assignment projections in both API-worker and
dedicated-worker deployments.
`alerts.replay` is required to replay a tenant-owned dead-letter. The global
integration kill switch and `FLEET_SECTION16_WORKERS_ENABLED` override all.

## Rollout and rollback

1. Apply additive migrations `000034`–`000037` with all flags false.
2. Enable read projections, then planning, publication and workers separately.
3. Configure and validate Dispatch/Hauling HMAC sources before tracking.
4. Configure an optimization provider before enabling optimization.

For rollback, disable Section 16 writes/workers first, drain in-flight jobs and
retain outbox evidence. Application rollback is safe while additive tables
remain. Schema rollback must run `000037` down through `000034` down in reverse
order and only after confirming no retained operational history is required.

Machine-readable contract:
`docs/openapi/section16-dispatch-routes-control.openapi.json`.
