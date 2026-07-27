# Marketplace Acceptance and Provider Assignments

How a request reaches an organization, how the organization claims it, how a
provider is assigned, and how the provider's decision is recorded.

## 1. Two decisions, never one

A job involves two independent decisions, and the system never collapses them:

1. **The organization decides** whether to take the work (commercial).
2. **The provider decides** whether to perform it (personal and safety).

An organization accepting work does not commit any individual provider. A
provider is only committed by their own explicit response. See
[`universal-accept-decline.md`](universal-accept-decline.md).

## 2. Operating modes

Configured per organization via `settings.operating_mode`:

| Mode | Organization acceptance | Provider direct acceptance |
| --- | --- | --- |
| `CENTRALIZED` | Dispatcher only | No |
| `DELEGATED` | Via a provider accepting an offer | Yes |
| `HYBRID` | Dispatcher | Yes |

Mode controls *who may claim work for the organization*. It never controls
whether a provider may decline.

## 3. Request lifecycle

```
OFFERED ─────► ORGANIZATION_ACCEPTED ─────► ASSIGNMENT_PENDING ─────► ASSIGNED
   │                                              │                      │
   ├──► DECLINED                                  └──► (decline/expiry) ──┘
   ├──► EXPIRED                                          reassignment
   └──► WITHDRAWN                                                        │
                                                                         ▼
                                                              IN_EXECUTION ─► COMPLETED
```

Request state lives in `fleet_partner_marketplace_request_projections`. This
is a **projection** of state owned by the source service, not the source of
truth. It carries `source_version` and a freshness state.

### 3.1 Projections never regress

`Upsert` applies a source event only when `source_version` is strictly
greater than what is stored:

```sql
... ON CONFLICT (...) DO UPDATE SET ...
    WHERE fleet_partner_marketplace_request_projections.source_version
          < EXCLUDED.source_version
```

An out-of-order or duplicated delivery is therefore a no-op rather than a
state regression. The same guard protects trip projections.

### 3.2 Freshness gates state-changing commands

| Freshness | Meaning | State-changing commands |
| --- | --- | --- |
| `CURRENT` | Recently confirmed | Allowed |
| `STALE` | No update within the staleness window | Blocked |
| `GAP_DETECTED` | A version gap was observed | Blocked |
| `UNKNOWN` | Never confirmed | Blocked |

Read operations remain available and are labelled with the freshness state, so
operators see degraded data rather than being shown nothing or, worse, being
allowed to act on a stale view.

## 4. Organization acceptance

`AcceptForOrganization` reserves at the **source service first**, and only
records local acceptance after the source confirms. The ordering matters: if
the local record came first, a lost race at the source would leave the
organization believing it holds work it does not have.

Three outcomes:

| Source outcome | Result |
| --- | --- |
| Confirmed | Local acceptance recorded, request `ORGANIZATION_ACCEPTED` |
| Definite conflict | `request_already_claimed`, nothing recorded |
| Timeout / unknown | `RESERVATION_PENDING`, durable command record, reconciliation |

An uncertain outcome is never guessed. A durable
`fleet_partner_marketplace_command_operations` row holds the idempotency key,
and reconciliation **reads back by that original key** rather than blindly
retrying — a retry could claim a second request.

Acceptance is recorded once and is immutable: `UNIQUE(offer_id)`, an
append-only trigger, and a `CHECK` requiring exactly one actor kind
(dispatcher user or provider, never both, never neither).

## 5. Assignment lifecycle

```
CREATED ─► PENDING_PROVIDER_RESPONSE ─┬─► ACCEPTED ─► READY_FOR_EXECUTION ─► IN_PROGRESS ─► COMPLETED
                                      ├─► DECLINED
                                      ├─► EXPIRED
                                      └─► CANCELLED
```

Transitions are validated against the state machine in `models`, applied with
optimistic concurrency (compare-and-swap on `version`), and appended to
`fleet_partner_assignment_state_history`, which is append-only by trigger.

### 5.1 Creating an assignment

`POST /fleet-partner/v1/requests/{requestId}/assign`

1. Check freshness and that the organization holds the request.
2. Check membership eligibility locally, then at the source when enabled.
3. Lock the request projection; verify the expected version.
4. Create the assignment — `uq_fleet_partner_assignment_current` guarantees at
   most one live assignment per request.
5. Reserve provider capacity, and asset capacity when an asset is named —
   `uq_fleet_partner_reservation_held` prevents double-booking.
6. Transition to `PENDING_PROVIDER_RESPONSE` with a deadline.
7. Move the request to `ASSIGNMENT_PENDING`.
8. Write audit and outbox rows in the same transaction.

Notification delivery is deliberately **outside** the commit requirement: a
provider whose device is offline can fetch pending work on reconnect until the
deadline. Making delivery a precondition would let a push failure block
dispatch.

### 5.2 The provider responds

Accept and decline both:

- Check for a replay by `clientResponseId` and return the original outcome
- Lock the assignment **proving ownership in SQL**, not in Go — a provider
  cannot lock another provider's assignment
- Write exactly one immutable response row
- Transition state, append history, write audit and outbox

Accept additionally re-checks eligibility **at response time**, because
conditions may have changed since assignment, and only then requests canonical
execution at the source — outside the transaction, guarded by a durable
command record.

Decline additionally releases reservations, creates typed follow-up actions
for the reason given, and opens a reassignment case, flagged for human review
when the reason is safety-protected.

### 5.3 Expiry never accepts

The expiry worker claims overdue rows with `FOR UPDATE SKIP LOCKED`,
re-checks the deadline and status under lock (a provider may have answered
between scan and update), then moves the assignment to `EXPIRED`, releases
capacity and opens a reassignment case.

### 5.4 Reassignment preserves history

A replacement assignment is a new row linked by `rootAssignmentId` and
`replacesAssignmentId`, with an incremented attempt counter. The original is
never rewritten: the previous provider's response stays exactly as they gave
it. The new provider receives a fresh Accept/Decline choice.

An assignment that is already accepted or executing cannot be silently
replaced — that would strip a provider of committed work. It must be cancelled
through the trip-cancellation path first.

### 5.5 Cancellation

Cancellation is refused once canonical execution exists (`sourceExecutionId`
set, or status `IN_PROGRESS`); it must go through the source service's trip
cancellation contract. A live trip is never cancelled locally without source
confirmation.

## 6. Concurrency guarantees

| Race | Guard | Test |
| --- | --- | --- |
| Two dispatchers assign the same request | `uq_fleet_partner_assignment_current` | `TestOneLiveAssignmentPerRequest` |
| Same provider double-booked | `uq_fleet_partner_reservation_held` | `TestProviderCapacityCannotBeDoubleBooked` |
| Provider taps accept and decline repeatedly | `UNIQUE(assignment_id)` + row lock | `TestConcurrentProviderResponsesSingleWinner` |
| Stale client state | version CAS | `TestAssignmentStateMachineRejectsIllegalMoves` |
| Duplicate source event | `source_version` guard | `TestTripProjectionIgnoresOlderAndDuplicateEvents` |
| Replayed command | `UNIQUE(operation_type, idempotency_key)` | `TestCommandIdempotencyAndConflict` |
| Cross-tenant read | organization predicate in every query | `TestMarketplaceTenantIsolation` |

## 7. Service category status

| Category | Status |
| --- | --- |
| Dispatch | Fully implemented and end-to-end validated against `driver-dispatch-delivery-service` |
| Taxi | Fleet integration contract ready; live execution disabled by feature flag pending taxi-service implementation |
| Hauling | See the final report for the exact repository-verified status |

Taxi and hauling use the `UnimplementedExecution` adapter, which **fails
closed**: it refuses execution rather than pretending success. Enabling those
categories requires a real adapter, not a flag flip.

See also [`service-contracts.md`](service-contracts.md),
[`events-and-jobs.md`](events-and-jobs.md), and
[`provider-data-minimization.md`](provider-data-minimization.md).
