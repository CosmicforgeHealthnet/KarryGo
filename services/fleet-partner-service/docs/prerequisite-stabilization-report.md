# Prerequisite Stabilization Report

Tracks the work moving the repository from **NO-GO** to **READY FOR SECTIONS 13–15**.

## WP0 — Worktree inventory and backup

| Field | Value |
| --- | --- |
| Branch | `staging` |
| Starting commit | `7320aaaeb3bb7427b1ffb15c003bc6389d549c1f` |
| Tracked modified files | 5 |
| Untracked files | 246 |
| Total preserved | **251** |
| Backup location | `C:\KarryGoPush-backup-20260726-213208` |

The audit's reported counts were verified rather than assumed; both matched exactly.

### Tracked modifications (5)

```
go.work
services/driver-dispatch-delivery-service/.env.example
services/driver-dispatch-delivery-service/cmd/main.go
services/driver-dispatch-delivery-service/internal/config/config.go
services/driver-dispatch-delivery-service/internal/router/router.go
```

### Untracked distribution

| Area | Files |
| --- | --- |
| `services/fleet-partner-service` | 215 |
| `services/driver-dispatch-delivery-service` | 35 |
| `go.work` (tracked, modified) | 1 |

**No unrelated service has any change.** Taxi, hauling, wallet, customer,
media, notification, verification and analytics are all untouched.

### Backup method

Every tracked-modified and untracked file was copied outside the repository,
preserving relative paths, with a full inventory at `INVENTORY.txt`. Verified
251 inventory entries → 251 files copied.

`git clean` was not used. Nothing was staged, committed, reset or restored.

## WP1 — Dispatch schema mismatch: `trips.package_type`

### Root cause: stale test harness, not a schema defect

`migrations/000029_add_package_fields_to_trips.up.sql` adds `package_type`,
and the column is present in the real chain. The failure was in the test:

`internal/features/trip/repository_integration_test.go` built an isolated
schema and applied a **hand-picked subset** of migrations —
000004, 000019, 000021, 000022, 000023, 000024 — omitting 000029 entirely.

The test was therefore asserting against a schema **no deployment ever runs**.
Classification, per the work package's own list: *a migration not applied by
the test harness*. Not a missing migration, not a stale query, not a
production defect.

### Fix

The harness now globs and sorts the real `*.up.sql` chain and applies it in
order, plus an explicit assertion that `trips.package_type` exists before the
test proceeds. A hand-maintained subset silently stops covering whatever a
later migration adds — the same self-maintenance failure previously found in
the fleet integration TRUNCATE list. Deriving the list means a new migration is
covered the moment it lands.

No migration was rewritten. No expected field was removed from the test.

### Second finding: the legacy chain is not blindly re-appliable

A no-op second apply of the whole chain fails at
`000014_create_bikes.up.sql`. At least **eight tracked legacy migrations**
(000014, 000015, 000016, 000021, 000022, 000023, 000024, 000025) use bare
`CREATE TABLE` without `IF NOT EXISTS`.

This is pre-existing and by design: the chain is applied once per version by a
versioned runner, not re-executed. Blind re-application was the wrong test for
it. The three new marketplace migrations (000031/000032/000033) **are** fully
idempotent and were each verified apply → no-op → rollback → reapply
individually.

No legacy migration was modified, per the instruction not to rewrite applied
migrations.

### WP1 proof

| Check | Result |
| --- | --- |
| Fresh apply of full chain into a clean schema | PASS |
| `trips.package_type` present after chain | PASS |
| `000029` rollback removes the column | PASS (1 → 0) |
| `000029` reapply restores it | PASS (0 → 1) |
| `TestPostgresRepositoryTripFoundation` | **PASS** (was skipping) |
| Full dispatch suite, run twice | **1152 pass / 0 fail / 0 skip** |
| Customer booking + independent-provider regressions | green within the above |

### Third finding: a silently skipping test

`TestPostgresRepositoryTripFoundation` reads
`DISPATCH_TRIP_TEST_DATABASE_URL` or `TEST_DATABASE_URL` — not the
`DISPATCH_TEST_DATABASE_URL` used elsewhere. It had been skipping silently
rather than failing, which is why the schema drift went unnoticed.

Separately, `acceptance_to_canonical_trip_harness_test.go` appeared to be
incomplete scaffolding — it skipped unconditionally, seemingly waiting for a
test that did not exist — and was removed on that basis.

**That judgement was wrong, and WP2 caught it.** The file is the dispatch half
of a two-process end-to-end test, and `TestAcceptanceToOneCanonicalTrip` does
exist, in the fleet integration package. Removing it broke that test. It was
restored byte-identical from the WP0 backup during WP2. See the WP2 section.

## WP2 — Full fleet and dispatch PostgreSQL suites

### Environment

| Variable | Value |
| --- | --- |
| `FLEET_PARTNER_TEST_DATABASE_URL` | `karrygo_fleet_partner_test` on :5446 |
| `DISPATCH_TEST_DATABASE_URL` | `karrygo_dispatch_test` on :5447 |

Both are read by the fleet harness: the second is required by the
cross-service end-to-end test. Isolation verified — distinct ports, distinct
databases, and zero dispatch tables present in the fleet database.

### Baseline

329 s, **207 pass / 1 fail / 0 skip**. Skips were counted from verbose output;
a non-verbose run cannot display them and its skip count means nothing.

### Root cause 1 — a regression I introduced in WP1

`TestAcceptanceToOneCanonicalTrip` failed with
`dispatch helper exited before readiness`.

The fleet end-to-end test spawns the dispatch half as a child process:

```go
exec.CommandContext(childCtx, "go", "test", "-count=1",
    "-run", "^TestAcceptanceToCanonicalTripDispatchHarness$", "./internal/router")
```

That harness is exactly the file WP1 removed as "dead scaffolding". It was not
scaffolding — it is the dispatch process of a two-process test, and its
unconditional-looking skip is a guard on
`KARRYGO_CROSS_SERVICE_DISPATCH_HELPER`, which the fleet test sets when it
launches the child.

Restored byte-identical from the WP0 backup. The end-to-end test passes again.

**Lesson recorded:** a test that always skips standalone is not necessarily
dead. Before deleting one, find its caller.

### Root cause 2 — per-test migration re-application

No hang, no lock wait, no leaked goroutine, no pool exhaustion. The
`internal/integration` package accounted for 305 s of the 329 s, spread evenly
across 103 tests at roughly 3 s each; the slowest single test was 5.76 s.

`testPool` re-applied all 11 migrations on every call. They are idempotent, so
this was correct but wasteful — it changed no outcome. Isolation comes from
the per-test TRUNCATE and catalogue re-seed, which are retained unchanged.

Applied `sync.Once` so migrations run once per test binary.

**329 s → 208 s / 191 s (about 40 % faster) with no assertion weakened, no
test skipped and no coverage removed.**

### Migration verification

The fleet chain is 11 up migrations, each with a matching down file, no
duplicate numbers. Fresh apply verified. As recorded under WP1, the dispatch
legacy chain is versioned rather than blindly re-appliable; the fleet chain
applies cleanly and the test schema matches repository expectations, proven by
the suite passing.

### Results

| Run | Result | Runtime |
| --- | --- | --- |
| Fleet run 1 | **208 pass / 0 fail / 0 skip** | 208 s |
| Fleet run 2 | **208 pass / 0 fail / 0 skip** | 191 s |
| Dispatch regression | **1152 pass / 0 fail / 1 skip** | — |

Idle-in-transaction sessions after the runs: **0** — no leaked database state.

The single dispatch skip is `TestAcceptanceToCanonicalTripDispatchHarness`,
the helper process described above. Skipping when run standalone is its
correct behaviour, and its coverage *is* exercised — through the fleet
end-to-end test, which passes. It is not a missing-coverage skip.

`go vet` and `go build` pass for both services.

## WP3 — Durable dispatch marketplace outbox delivery (partial)

### Audit before changing code

| Component | State found |
| --- | --- |
| `PublishEvent`, `ClaimOutbox` (`FOR UPDATE SKIP LOCKED`), `RecoverExpiredLeases`, `MarkPublished`, `MarkDeliveryFailed`, `RecordDeliveryAttempt`, `CountOutboxByStatus` | already present |
| `GetOutboxByID`, `ReplayDeadLetterEvent`, `ListOutboxByAggregate` | missing |
| Outbox statuses (`PENDING`, `IN_FLIGHT`, `PUBLISHED`, `FAILED`, `DEAD_LETTER`) | already cover the required meanings — no duplicate state system introduced |
| Delivery worker | none existed |
| Event types emitted | **3 of 11** |

### Built

- `marketplace/events.go` — the 11 canonical event types, the wire `Envelope`,
  and `PublishMarketplaceEvent`, which rejects unknown types and restricted
  payload fields at publish time. A nil querier is a hard error, not a
  fallback: an event committed independently of its cause is the split the
  outbox exists to prevent.
- `marketplace/outbox_worker.go` — `HTTPEventDeliverer` (HMAC-signed) with
  five-way outcome classification, plus `UnconfiguredDeliverer` as a distinct
  type so tests can assert by type that no fallback replaced a live client.
- `marketplace/outbox_delivery_worker.go` — `OutboxWorker`. The claim commits
  before any HTTP call; no transaction is open while waiting on another
  service.

### A duplicate method, written and then removed

The first cut added `MarkOutboxUnknown` beside the existing
`MarkDeliveryFailed`, commented as "distinct" because the retry preserves the
event id and payload hash. **The two had identical SQL and identical
signatures**, and that property was equally true of `MarkDeliveryFailed` since
neither touches those columns. The comment asserted a distinction the code did
not implement — worse than no comment, because it tells the next reader that
ambiguous delivery is handled specially when it is not.

Deleted. Treating UNKNOWN like RETRYABLE is correct, for a reason that belongs
at the call site: the row keeps its identity, so the retry is a *redelivery*,
and fleet's inbox dedupes on (producer, eventId) with payload-hash
enforcement. WP4's `TestIngestEventRedeliveryIsDuplicate` proves it.

### Reachability of the three added methods

| Method | Callers | Tests | Status |
| --- | --- | --- | --- |
| `ReplayDeadLetterEvent` | 0 | 0 | correct but unreachable — no operator entry point |
| `GetOutboxByID` | 0 | 0 | speculative |
| `ListOutboxByAggregate` | 0 | 0 | speculative |

Compiled but unproven. Not delivered capability.

### Not done

The 8 missing emission sites, dispatch worker start-up wiring, and the 6
dispatch flags.

## WP4 — Authenticated fleet event ingestion (complete)

Fleet already had a consumer inbox with payload-hash enforcement
(`internal/events/inbox.go`). Reused, not duplicated.

- `usecases/event_ingestion.go` — envelope validation, payload-hash
  verification against delivered bytes, inbox dedupe namespaced per producing
  service, handlers for all 11 types. Inbox claim + projection change + inbox
  completion are **one transaction**.
- `http/event_handlers.go` — `POST /internal/fleet-partner/v1/events` behind
  HMAC auth plus a per-route caller allowlist. `sourceService` comes from the
  **verified signature**, never the body, so a caller cannot attribute events
  to another service.
- `cmd/marketplace_routes.go` — production wiring.

**A real gap this closed:** the marketplace internal routes were mounted only
in the end-to-end test, never in `cmd/main.go`. Production had no
acceptance-verification, provider-read or provider-response route at all. The
passing E2E test had been masking that the composition root never wired them.

Response contract: `200` ACCEPTED/DUPLICATE/IGNORED, `400` permanent
rejection, `403` caller not allowed, `500` transient. The producer classifies
retry-versus-dead-letter from this alone, so a transient failure must never
surface as 400. Only the code is returned; echoing a message would put source
payload fragments into another service's error columns.

## WP5 — Source-version and gap detection (partial)

`ProjectionRepository.ApplySourceEvent` implements three outcomes: **equal or
lower** is ignored (applying it would move the projection backwards); **exactly
one greater** applies; **more than one greater** applies the newer state but
marks `GAP_DETECTED`, which blocks state-changing commands. A projection
already gapped stays gapped until reconciliation — one in-order event does not
prove the earlier missing ones arrived.

Lock is `FOR NO KEY UPDATE`, the same rule established in WP1/WP2: FK child
inserts take an implicit `FOR KEY SHARE` that deadlocks against `FOR UPDATE`.

**Not done:** reconciliation read-back client, reconciliation worker, STALE
sweep, fleet outbox emission.

## Programme-wide audit (WP3–WP14 scope)

This materially changes what "finish the remaining work packages" means.

### Fleet: 6 of 30 feature areas implemented

| Area | Non-test lines | State |
| --- | --- | --- |
| `marketplace` | 7348 | implemented |
| `organizations` | 3706 | implemented |
| `auth` | 2665 | implemented |
| `providers` | 1852 | implemented |
| `staff` | 1784 | implemented |
| `accesscontrol` | 158 | thin — deny-by-default authorizer only |
| **24 other areas** | **8 each** | **reserved-boundary stubs: package clause and doc comment, no code** |

The stubs include `assets` (vehicle linking), `workforce` (shifts), `reports`
(dashboards), `finance`/`wallets` (subscription), `contracts`, `compliance`,
`approvals`, `proof`, `orders`, `shipments`, `tracking`, `safety`, `dispatch`.

### Payment-wallet: 1 of 6 features implemented

`wallets` is real (3614 lines). `payments`, `provider_earnings`, `refunds`,
`settlements`, `withdrawals` are **empty directory scaffolding** — the
`clients/ http/ models/ repositories/ usecases/` folders exist with no `.go`
files. Migrations are `001_wallet_ledger`, `002_provider_transaction_pins`,
`003_bank_account_default`; there is **no schema** for settlements, earnings,
refunds, withdrawals or subscription invoices.

### Consequence

WP7 alone needs a new schema plus five subsystems from zero with double-entry
accounting, idempotency and reconciliation. WP3–WP5 needs roughly six further
Fleet subsystems from zero. This is not a finishing pass over near-complete
code. **NO-GO.**

## Why the remaining WP3 emission work was not attempted

The 8 missing events must be written in the same transaction as the state
change they describe. Those transitions live in
`internal/features/request/repository.go` and
`internal/features/trip/repository.go` — legacy tracked packages whose methods
execute **directly against the pool**, with no transaction to join and no
`Querier` parameter to accept one.

Emitting from there means threading a transaction through legacy repository
signatures and their callers, beneath 1208 passing dispatch tests. That is a
refactor, not an insertion. Starting it without budget to finish and verify it
would leave the tree worse than the green baseline. **This is the resume
point.**

## Repository sync during this work

Between passes the tree was committed and merged with `origin/staging` by
another process — not by this session, which has never committed or staged.
History gained `8dc5ac5 wip: preserve all local implementation before
repository sync`, `812856e merge: integrate origin staging with local
implementation`, `7e8da87`.

All **code** from this session survived. **This report did not** — the merge
restored an older copy, discarding the WP3/WP4/WP5 sections, which are
rewritten above. Anyone reading a doc in this repo should check it against the
code rather than assume the merge preserved it.

Both suites were re-run against the merged tree, because the pre-merge
baseline no longer proved anything.

| Suite | Result |
| --- | --- |
| Fleet, full, verbose | **225 pass / 0 fail / 0 skip** |
| Dispatch, full, verbose | **1208 pass / 0 fail / 1 skip** (merge added 56 tests, all passing) |
| Payment-wallet | all packages ok |
| `go build` / `go vet`, all three services | pass |

## WP3 completed — the eight missing emission sites

The previous pass recorded these as blocked, on the reading that emitting from
`request/repository.go` and `trip/repository.go` meant threading a transaction
down through legacy signatures and their callers.

**That reading was wrong, and it is worth saying why.** The requirement is that
the event commits with the state change. It is *not* that the transaction comes
from the caller. Most of these methods already open a transaction of their own
(`CancelTripTx`, `CompleteTripTx`, `SubmitProofTx`, `MarkArrived`,
`MarkTripStarted`, `TransitionTripStatus`, `MarkBroadcastAccepted`); the rest
were single guarded statements that could be wrapped locally. Emission went
inside those transactions with **no signature changed and no caller touched**.

### How the two packages reach the outbox

`request` and `trip` each declare a one-method `EventEmitter` interface and
hold an optional implementation. `marketplace.LegacyEventEmitter` satisfies
both. The interface is written in terms of `pgx.Tx` rather than a package-local
`Querier`, because Go interface satisfaction is by *exact type identity*: a
method taking `marketplace.Querier` would not satisfy an interface requiring
`trip.Querier` even though the two are structurally identical. `pgx.Tx` is the
one named type all three packages already share.

A nil emitter emits nothing. That is what every pre-existing caller gets, and
it is why the untouched suite still describes unchanged behaviour.

### Source versions

Both aggregates already had a `source_version` column. Each emission bumps it
by one inside the same transaction and publishes that number. Consumers
gap-detect on consecutive versions, so this is not cosmetic — a hole here makes
fleet mark the projection `GAP_DETECTED` and stop applying. Asserted directly
in `TestTripLifecycleEmitsOrderedCanonicalEvents`.

### Mapping decision

`MarkBroadcastNoProviderFound` publishes `platform.request.expired.v1` with
`reason: "no_provider_found"` rather than a twelfth event type. To a managing
organization the operational fact is identical — the request is no longer on
offer — and a new type would oblige every consumer to learn a distinction that
changes nothing they do.

### Production wiring

`cmd/main.go` builds one emitter and attaches it to both repositories, gated on
`DISPATCH_MARKETPLACE_OUTBOX_ENABLED`. The delivery worker starts from
`startOutboxWorker` behind a new, independent `DISPATCH_EVENT_DELIVERY_ENABLED`
plus six tuning values, so event delivery and the provider-decision relay can
be turned on separately.

**All 11 canonical event types are now emitted.**

## WP5 — reconciliation worker and stale sweep

The read-back client was already present (`DispatchExecution.ReadRequest` /
`ReadTrip`, against dispatch's existing `GET /requests/:id` and
`GET /trips/:id`), as was every repository method. What was missing was the
worker joining them, and it is now in
`marketplace/usecases/reconciliation_worker.go`.

`ProjectionRepository.ReconcileFromSource` was added. It deliberately does
**not** follow the one-greater rule `ApplySourceEvent` enforces: that rule
protects against unseen history in an event stream, whereas a read-back is the
source answering directly about present state — skipping the hole is the point
of calling it. Versions still only ever move **forward**, so a lagging read
replica cannot drag a projection backwards and re-open the gap.

Every read failure defers rather than resolves, including "not found". A
replica that has not caught up is indistinguishable from a deleted aggregate,
and one of those two readings is destructive.

### A production defect this uncovered

`Options.Trips` was **never wired** — the only construction site, in
`buildMarketplaceInternalRoutes`, omitted it, so it was nil in every
deployment. It had gone unnoticed because the one production caller,
`execution.go:198`'s `OpenReconciliation`, takes its querier as an argument and
so never dereferenced the nil receiver. `MarkStaleTrips`, which uses the
repository's own pool, would have panicked the moment a sweep ran. Service
construction is now a single shared `newMarketplaceService`, with `Trips` and
`Commands` wired.

### Still incomplete in WP5

`applyProjectionChange` updates the **request** projection on trip events but
never upserts `fleet_partner_trip_projections`. That table is therefore still
unpopulated, so trip-aggregate reconciliation has nothing to find in practice.
The request path is complete and tested; the trip path is written and compiles
but is not reachable until ingestion populates trip projections.

## Verification

Run against the live WP2 PostgreSQL containers (dispatch :5447, fleet :5446),
not skipped.

| Suite | Result |
| --- | --- |
| Dispatch, full, verbose | **1217 pass / 0 fail / 1 skip** (1208 baseline + 9 new) |
| Fleet, full, verbose | **230 pass / 0 fail / 0 skip** (225 baseline + 5 new) |
| Payment-wallet | all packages ok |
| `go build` / `go vet`, all three services | pass |

New coverage: 9 dispatch emission tests (ordering, version consecutiveness,
rejected transitions staying silent, terminal transitions emitting exactly
once, no customer commercial data in payloads, disabled path silent) and 5
fleet reconciliation tests (repair, fail-closed on outage, fail-closed when
unconfigured, no backwards movement, stale sweep).

The data-minimization tests assert on emitted **bytes**, not on the payload
builders, so a field added later is caught rather than assumed safe.

## WP5 completed — trip projection lifecycle

### The defect that had to be fixed first

`applyEvent` resolved every event to the **request** projection and applied
`event.AggregateVersion` to its `source_version`. Dispatch emits trip and proof
events on the **TRIP** aggregate, whose version counts events against one trip;
the request's counts events against one broadcast. Two independent sequences
were being fed into one counter.

The consequence was not subtle. A trip at version 1 arriving against a request
projection at version 4 read as a stale replay and was dropped; a trip at
version 9 against a request at version 2 read as a six-event hole and marked
the projection `GAP_DETECTED`. Contiguity — requirement 2 — was unimplementable
until this was separated.

`applyEvent` now routes on `tripAggregateEvents`. Trip and proof events go to
`applyTripEvent`; request events keep the existing path unchanged.

An existing test encoded the bug: `TestIngestEventStaleVersionIgnored`
sequenced `trip.completed.v1` at version 5 against a request event at version
3, which only read as a sequence while the counters were shared. It now uses
two request-aggregate events, and trip-side staleness is covered separately
against the trip's own sequence.

### Event mappings

| Event | Trip projection | Request projection |
| --- | --- | --- |
| `trip.created.v1` | insert, status from payload | `platform_trip_id` set, status `EXECUTION_CREATED` |
| `trip.status_changed.v1` | advance status | status `IN_PROGRESS` |
| `trip.completed.v1` | status, `completed_at` from payload | status `COMPLETED` (gated) |
| `trip.cancelled.v1` | status, `cancelled_at` from payload | status `CANCELLED` + `cancelOpenWork` (gated) |
| `proof.completed.v1` | version advance + history row only | untouched |

Proof is evidence, not a lifecycle transition: it advances the trip's version
and records history, but never moves the request projection and never releases
anything.

`completed_at` and `cancelled_at` are only ever taken from the source payload,
falling back to the envelope's `occurredAt` — never inferred from elapsed time
or from local assignment state.

### Six outcomes, not three

`ApplyTripSourceEvent` returns `CREATED`, `APPLIED`, `DUPLICATE`, `STALE`,
`CONFLICT` or `GAP`. The two that did not exist before:

- **DUPLICATE** — same version, same payload hash. Idempotent no-op. This is a
  redelivery under a *different event id*, which the inbox cannot dedupe
  because it keys on (producer, eventId); only the projection can.
- **CONFLICT** — same version, **different** payload hash. Two events claim one
  version and we cannot tell which is right, so the projection is marked
  `GAP_DETECTED` and a read-back decides. Overwriting would silently pick a
  winner. Detecting this required storing `last_payload_hash`, added in
  migration 000012.

A projection opened at a version above 1 starts `GAP_DETECTED`: it skipped
history fleet never saw.

### Fail-closed gates (requirement 14)

`blocksCapacityRelease` is deliberately stricter than the existing
`models.BlocksStateChangingCommands`, which admits `STALE`. A stale projection
is fine to read and to advance, but it is not adequate grounds for standing
down an offer or settling money: source silence is equally consistent with
delivery having stopped, and the two readings have opposite consequences. It
blocks on `GAP_DETECTED`, `UNKNOWN`, `STALE` and `RECONCILING`.

A completion or cancellation arriving on a blocked projection opens a
reconciliation case instead of releasing capacity. Proved by
`TestTerminalEventOnGappedProjectionDoesNotReleaseCapacity`.

### Association

The request link is read back from the **stored projection**, not from each
event. Only `trip.created.v1` carries `requestId`; relying on the payload meant
every later event appeared unassociated and completion silently stopped
reaching the request projection. Found by test, not by review.

Tenant attribution never comes from the event body. An existing trip projection
is the strongest answer, then the request projection; a trip matching neither
is ignored rather than assigned an invented organization.

### Reconciliation lifecycle

- **Case creation** now has two sources: ingest-time gaps and conflicts, plus a
  sweep (`openTripReconciliationCases`) over `STALE`/`GAP_DETECTED`/`UNKNOWN`
  projections. Without the sweep, only trips an event happened to flag were
  ever examined — a projection that simply stopped receiving events, which is
  the failure mode reconciliation exists for, was never looked at.
- **Leases.** `ClaimTripReconciliationLease` marks a case in flight *without*
  spending an attempt. Claiming through `ResolveReconciliation` charged every
  case twice per pass — once to claim, once to defer — silently halving every
  retry budget. `ClaimReconciliationCases` now claims `OPEN` only;
  `RecoverExpiredReconciliationLeases` returns abandoned cases, leaving
  `attempts` untouched because a crash is not a judgement.
- **Dead letter** is `FAILED_PERMANENT`, listed by `ListDeadLetteredCases`.
  `ReplayReconciliationCase` is the operator entry point and resets `attempts`
  to zero — a replay that kept its count would dead-letter again on first retry.
- **Not-found is not destructive.** A source reporting no such trip is
  indistinguishable from a lagging read replica, and one reading is
  irreversible. It retries and dead-letters rather than terminating anything.
- **Repair** moves versions forward only, and repairs the associated request
  projection for terminal outcomes so the two views cannot contradict.

### Migration 000012

`last_payload_hash` on trip projections; `lease_owner` / `lease_expires_at` on
reconciliation cases; a **partial unique index** on
`(source_service, aggregate_type, aggregate_source_id)` where status is live,
so repeated sweeps cannot enqueue duplicate repairs for one aggregate
(`OpenReconciliation` absorbs the conflict); plus freshness, source-identity
and request-association indexes. Reversible.

### Production wiring

`newMarketplaceService` is the single construction site for both the HTTP
surface and the worker, wiring request projections, trip projections,
reconciliation cases, offers, assignments, execution starts, commands and the
dispatch read-back client. `startMarketplaceReconciliation` passes interval,
batch size, lease, retry limit, backoff base/max and sweep limit from
configuration; it is registered in `main.go`'s worker WaitGroup, so shutdown
cancels the context and waits for the in-flight pass rather than killing it
mid-transaction.

Seven new `FLEET_MARKETPLACE_RECONCILE_*` values. The gate,
`FLEET_MARKETPLACE_RECONCILIATION_ENABLED`, remains **false** by default and no
example secret was populated.

### Test results

Run against the live WP2 containers (fleet :5446, dispatch :5447).

| Suite | Result |
| --- | --- |
| Fleet, full, verbose | **250 pass / 0 fail / 0 skip** |
| Dispatch, full, verbose | **1217 pass / 0 fail / 1 skip** |
| Payment-wallet | all packages ok |
| `gofmt` / `go vet` / `go build`, all three services | clean |

The single dispatch skip is `TestAcceptanceToCanonicalTripDispatchHarness`, a
helper started only by the fleet-side `TestAcceptanceToOneCanonicalTrip`, which
passes. It is not missing coverage.

20 new fleet tests: creation, status progression, completion, cancellation,
proof, identical duplicate, changed-payload conflict, older version, missing
version, request-to-trip association, data minimization asserted on the stored
row, full transaction rollback including the inbox entry, sweep case creation,
repair, retryable failure, not-found, dead letter, lease recovery, operator
replay and the fail-closed capacity gate.

## WP6 — Fleet prerequisite contracts (complete)

WP6 is complete. All five previously missing contracts have production
implementations, reversible migrations, composition-root wiring and
PostgreSQL-backed tests.

### WP6.1 — Assignment acknowledgement

Fleet now records provider acknowledgement independently from acceptance.
The assignment keeps acknowledgement state, timestamp and actor, while the
immutable command record retains organization/provider ownership, expected
version, idempotency key, request fingerprint and correlation metadata. A
successful command updates acknowledgement and writes history, audit and
`organization.assignment.acknowledged.v1` to the outbox in one transaction.
Assignment status is unchanged, and accept/decline remain available.

Production route:

`POST /internal/fleet-partner/v1/assignments/:assignmentId/provider-acknowledge`

It uses the same HMAC-authenticated caller allowlist and provider-service
identity binding as provider accept/decline. The provider must own the
assignment and pass live eligibility. Identical retries replay; cross-resource
or changed-input key reuse is rejected. Organization policy
`require_assignment_acknowledgement` defaults false and, when enabled, blocks
departure without overriding decline rights.

### WP6.2 — Incident and dispute references

Fleet stores tenant-scoped opaque references only; Support/Dispute remains the
case authority. References associate organization work with assignments,
request/trip projections, platform trips, providers and vehicles. Incident IDs
are checked through the signed Support client; unavailable configured
validation fails closed. Support currently has no dispute-by-ID endpoint, so
dispute pointers are explicitly `UNVERIFIED`, authorize no operational or
financial consequence, and carry a warning.

Tenant/RBAC routes:

- `POST /fleet-partner/v1/safety/references`
- `GET /fleet-partner/v1/safety/references`
- `GET /fleet-partner/v1/safety/references/:referenceId`
- `GET /fleet-partner/v1/safety/references/:referenceId/history`

Writes require `safety.references.manage` and `Idempotency-Key`; reads require
`safety.references.read`. Tenant/association ownership, fingerprint conflicts,
append-only history, audit and transactional outbox insertion are enforced.
Fleet emits `organization.safety_reference.recorded.v1`, stating only that a
pointer was recorded, not that Fleet owns or adjudicated the case.

### WP6.3 — Recipient confirmation

Dispatch is authoritative. Provider-authenticated, provider-scoped routes:

- `POST /api/v1/provider/trips/:id/recipient-confirmation`
- `GET /api/v1/provider/trips/:id/recipient-confirmation`

The versioned/idempotent write stores a random opaque recipient reference,
verification method, expected-recipient match verdict, optional proof pointer
and correlation/causation metadata. It atomically advances the trip version,
appends history and emits `trip.recipient_confirmed.v1`. Recipient contact is
used only for server-side comparison and is neither stored in this record nor
published. Fleet consumes an independently versioned trip projection.

### WP6.4 — Failed delivery

Dispatch is authoritative and uses the existing terminal `failed` trip state:

`POST /api/v1/provider/trips/:id/fail`

The provider-scoped command is versioned and idempotent. Server code owns the
reason classification, retry eligibility and recipient-unavailable verdict.
Operator notes remain private. The transition, evidence, history and
`trip.delivery_failed.v1` event commit atomically. Fleet projects failure but
does not infer completion, cancellation, capacity release or settlement.
Gapped/unknown/stale projection state remains fail-closed and reconciles.

### WP6.5 — Private contract security and production wiring

WP6 adds no Fleet-to-Dispatch command. Dispatch-to-Fleet outcome delivery
reuses the durable `POST /internal/fleet-partner/v1/events` contract: HMAC
service authentication, timestamp freshness, caller allowlist, verified source
identity, payload hashing, inbox replay protection and event-id idempotency.
Provider acknowledgement uses the same signed private boundary as
accept/decline. Fleet's Support lookup uses the shared signed client and fails
closed when configured validation is unavailable.

Repositories/services are non-nil in production composition roots; routes are
registered; event delivery and reconciliation workers start only when enabled;
all new flags and policies default off or fail closed.

### Migrations

- Fleet `000012_trip_projection_lifecycle` (WP5 prerequisite consumed by WP6)
- Fleet `000013_assignment_acknowledgements`
- Fleet `000014_safety_references`
- Fleet `000015_delivery_outcome_projections`
- Dispatch `000034_recipient_confirmations`
- Dispatch `000035_delivery_failures`

Every migration has a matching down migration; shape and full-chain tests pass.

### Implementation files

Fleet acknowledgement/projection code is in
`marketplace/usecases/acknowledgement.go`,
`marketplace/usecases/trip_projection_ingestion.go`,
`marketplace/repositories/trip_delivery_outcomes.go`, and the existing
marketplace models, repositories, HTTP handlers and ingestion code; production
wiring is in `cmd/marketplace_routes.go`. Safety references are under
`internal/features/safety/{models,repositories,usecases,http}` with
`internal/platformclients/support_reference.go`, wired by
`internal/app/wiring.go`. New acknowledgement, safety-reference and
delivery-outcome integration tests provide PostgreSQL/HTTP coverage.

Dispatch outcomes are in
`internal/features/trip/delivery_outcome_{model,repository,service}.go`, wired
through the trip handler/repository and `cmd/main.go`; canonical delivery uses
the marketplace event catalogue/outbox. Migration and PostgreSQL coverage is
in `delivery_outcome_migrations_test.go` and
`delivery_outcome_integration_test.go`.

### Final verification (2026-07-27)

Run against separate live PostgreSQL containers on Fleet :5446 and Dispatch
:5447, with every legacy database-test variable populated:

| Check | Result |
| --- | --- |
| Fleet full suite | **309 pass / 0 fail / 0 skip** |
| Dispatch full suite | **1231 pass / 0 fail / 1 skip** |
| Dispatch focused WP6 PostgreSQL tests | **11 pass / 0 fail / 0 skip** |
| Fleet focused WP6 PostgreSQL tests | **all matched tests pass / 0 fail / 0 skip** |
| Fleet vet / build | pass / pass |
| Dispatch vet / build | pass / pass |
| Payment-wallet regression / vet / build | pass / pass / pass |
| `gofmt` / `git diff --check` | clean |

The only skip is
`TestAcceptanceToCanonicalTripDispatchHarness`, intentionally skipped when run
standalone. Fleet's `TestAcceptanceToOneCanonicalTrip` launches that helper and
passes in the full Fleet run. No mandatory database test skipped.

## Remaining packages

WP3, WP4, WP5 and **WP6 complete**. The next unfinished requirement is **WP7
— payment-wallet financial foundation**. Initial uncommitted WP7 files are
present and compile, but WP7 is not claimed complete or included in WP6.

## WP7 - Payment Wallet financial foundation (2026-07-27)

Payment Wallet now owns organization wallets, opaque provider earning accounts, immutable double-entry journals, configured subscription billing, versioned commission/earning allocation, Fleet fees, holds, settlement and refund workflows, compensating reversals, reconciliation and statements. Migration `000004_finance_foundation` supplies the schema, constraints, append-only triggers, finance RBAC, approval policies, nonce replay table, leases, retry/dead-letter state, audit/history and transactional outbox.

The private finance API uses shared HMAC authentication, caller allowlisting, timestamp freshness and one-time nonces. Mutations enforce tenant scope, permission, state, expected version, idempotency/fingerprint and separation of duties. Private reads are permission checked and omit balances and ledger identity from Fleet-facing projections.

Money workers implement settlement/refund execution, timeout-to-UNKNOWN, authoritative read-back, atomic journal/status/event commits, backoff, lease recovery and dead-letter/replay. Maintenance covers invoice expiry, subscription renewal/transitions, bounded hold expiry, reconciliation and nonce retention. The independently gated outbox worker delivers versioned events to Fleet over HMAC HTTP with stable event IDs.

PostgreSQL coverage includes journal invariants; wallets/earning accounts and concurrent first creation; subscriptions/invoices/payment authorization; versioned allocation; bounded holds; separation of duties; settlement/refund UNKNOWN read-back and exactly-once posting; reconciliation; HMAC/replay; and outbox retry/dead-letter/replay. Migration fresh apply, replay, rollback and reapply pass on the separate Wallet test database. Detailed contracts and runbook are in `services/payment-wallet-service/docs/finance-foundation.md`.

**WP7 complete. WP8 Fleet-to-Wallet integration is next.**

## WP8 checkpoint - Fleet-to-Wallet integration (2026-07-27)

Implemented so far: the production HMAC Wallet client (including nonce and correlation/causation propagation), real wallet/earning-account provisioning, migration `000016_wallet_finance_integration`, durable versioned outbound operations, idempotency/fingerprint conflict protection, leases/recovery/retry/dead-letter/replay, UNKNOWN-to-idempotency-read-back processing, safe projection tables, Wallet event inbox dedupe/payload conflict/older-version/gap handling, and default-off worker/event-ingestion wiring. PostgreSQL tests cover operation idempotency, lease recovery, dead-letter/replay, timeout read-back without resubmission, and event duplicate/conflict/gap behavior.

WP8 remains in progress. Fleet migration `000017_finance_projection_reconciliation` now provides durable stale/gap/unknown reconciliation state, leases, recovery, bounded retry, dead letter, replay, and immutable history. The production worker performs identity/version-checked authoritative Wallet read-back; queue and outcome changes carry audit and outbox records. Wallet's generic private reader now supports tenant-scoped wallet, earning-account, subscription, invoice/payment, allocation, hold, settlement, refund, and compensating-reversal projections.

Current-only subscription and job-finance guards are implemented. Wallet paid-invoice events create a separate opaque `PAYMENT_VERIFICATION` projection. Earning creation requires current payment, current Dispatch completion, no failure/cancellation, and recipient confirmation when policy requires it. PostgreSQL tests cover gap repair, history, active/stale subscriptions, successful earning enqueue, and failed-delivery suppression. Fleet migration 17 replay/rollback/reapply passes.

The next exact WP8 work is Wallet's durable reversal-request/status command and authoritative job-payment lookup contract, followed by Fleet RBAC orchestration routes and the full cross-service HTTP/HMAC suite. WP9 has not started.

## WP8 complete - Fleet-to-Wallet integration (2026-07-28)

The three blockers named at the previous checkpoint are closed.

**Wallet durable reversal contract.** Migration `000006_reversal_contract_and_job_payments` adds `finance_reversals` with the full state set (`REQUESTED`, `PENDING_APPROVAL`, `APPROVED`, `PROCESSING`, `UNKNOWN`, `COMPLETED`, `FAILED_RETRYABLE`, `FAILED_PERMANENT`, `REJECTED`, `RECONCILING`, `DEAD_LETTER`), the reversal identity, tenant, original journal reference, reason code, integer minor amount, ISO currency, request fingerprint, idempotency key, expected version, external operation reference, attempt count, retry schedule, correlation/causation IDs, creating actor, approval reference and policy version, and the resulting compensating journal. The original posted entry is never edited. A full-value reversal posts the exact mirror and claims the ledger's single-reversal index; a partial reversal is linked by causation and scaled by largest remainder within each side so the compensating journal still balances exactly. Requests are capped at the reversible amount (original debit total less live refunds and live reversals); duplicates return the original result; conflicting fingerprints are rejected; cross-tenant journals are refused; the versioned `REVERSAL_APPROVAL` policy forbids self-approval. Execution, timeout-to-UNKNOWN, authoritative read-back, retry, dead-letter and replay run inside the existing Wallet financial worker — no second operation framework was added. Reconciliation reports stuck and journal-less reversals rather than repairing them. Every transition commits history, audit and outbox atomically.

**Authoritative job-payment lookup.** Migration 6 also adds `finance_job_payments`, keyed by references other services already own — dispatch request, canonical trip, partner booking and payment reference — each uniquely indexed per organization so a lookup is never ambiguous. `GET /internal/finance/job-payments/lookup` returns only the verification ID, organization, request/trip reference, integer minor amount, ISO currency, payment status, source version, paid timestamp, refund/reversal state, operation reference and a derived settled flag. HMAC, caller allowlist, timestamp freshness, nonce replay rejection, SQL-level tenant ownership and `finance.job_payment.view` are enforced; unknown reference, unsupported reference type and another tenant's record all answer `404` identically. Recording requires `finance.payment.confirm`, which Fleet does not hold. Wallet's own payment flow (`PayFromWallet`, `CompleteJob`, Paystack `charge.success`) mirrors intents into the record when the intent metadata carries an organization; a mirror failure never rolls back the payment and simply leaves downstream gates closed.

**Fleet finance RBAC routes.** Migration `000018_finance_operator_access` records the requesting actor and branch on each operation, a branch on each projection, and seeds `finance.operations.read`, `finance.operations.replay`, `finance.reconciliation.read`, `finance.reconciliation.manage`, `finance.settlements.request`, `finance.refunds.request` and `finance.reversals.request`. Fifteen tenant-scoped corporate routes cover listing and reading operations, projections and reconciliation cases, operation history and audit, dead-letter and reconciliation replay, permitted authoritative read-back, and settlement/refund/reversal requests. Deny by default, organization scope from the resolved membership only, object-level ownership in every query, branch filtering, `404` for out-of-scope identifiers, mandatory idempotency keys, expected-version checks, separation of duties on reversal, and audit plus outbox on every action. Suspended staff are rejected by the existing tenant middleware. No balance, ledger identifier or credential is exposed.

**Orchestration.** The job gate now consults Wallet's authoritative lookup rather than Fleet's copy, and fails closed when Fleet's own projection is stale, gapped, unknown, reconciling or ahead of the authoritative version. Unverified, refunded and reversed payments block earnings; failed and cancelled deliveries block earnings and settlement; the recipient-confirmation policy is enforced; duplicate completion resolves to the same operation. Settlement additionally requires no open reconciliation case and a matching expected version. Fleet writes no ledger entry — the integration suite asserts the Fleet schema contains no journal or ledger table.

**Cross-service tests.** Fleet and Wallet are separate Go modules and neither may import the other, so the proof is two suites sharing the production `shared/go/serviceauth` signer and verifier. `cross_service_finance_test.go` (Wallet) drives the real handlers over HTTP against real PostgreSQL for the whole HMAC matrix, the job-payment lookup, and the reversal contract end to end including crash-restart recovery and partial-reversal balancing. `finance_cross_service_test.go` (Fleet) drives the real `WalletClient` over HTTP against a server running the same verifier, and the real operator routes against real PostgreSQL for RBAC, IDOR, branch scope, replay, read-back and every orchestration gate.

### Verification (2026-07-28)

| Check | Result |
| --- | --- |
| `go vet ./services/payment-wallet-service/...` | pass |
| `go build ./services/payment-wallet-service/...` | pass |
| `go test -count=1 -timeout 30m ./services/payment-wallet-service/...` | **62 pass / 0 fail / 0 skip** |
| `go vet ./services/fleet-partner-service/...` | pass |
| `go build ./services/fleet-partner-service/...` | pass |
| `go test -count=1 -timeout 30m ./services/fleet-partner-service/...` | **321 pass / 0 fail / 0 skip** |
| Wallet migration fresh apply / replay / rollback / reapply | pass |
| Fleet migration fresh apply / replay / rollback / reapply | pass |
| `gofmt` on changed files / `git diff --check` | clean |

Both suites were run against the live PostgreSQL containers on Fleet :5446,
Wallet :5448 and Dispatch :5447, with `FLEET_PARTNER_TEST_DATABASE_URL`,
`PAYMENT_WALLET_TEST_DATABASE_URL` and `DISPATCH_TEST_DATABASE_URL` all
populated. `DISPATCH_TEST_DATABASE_URL` is mandatory for the Fleet suite:
without it `TestAcceptanceToOneCanonicalTrip` skips, and that skip is the only
one the Fleet suite can produce.

Line endings: 36 pre-existing Go files in the working tree use CRLF and are
therefore reported by `gofmt -l` regardless of content. That condition predates
this work package; no file's line endings were changed. Every file created in
this work package is `gofmt` clean, and the two edited files that `gofmt`
flagged for real content (`orchestration.go`, `models.go`, `handler.go`,
`service.go`) were formatted.

**WP8 complete. WP9 eligibility and approval enforcement is next.**

## WP9 complete - eligibility and approval (2026-07-28)

**Durable eligibility decisions.** Migration `000019_eligibility_decisions` adds the decision itself (identity, tenant, branch, subject type and opaque subject id, operation, policy id/key/version, one of `ELIGIBLE` / `INELIGIBLE` / `UNKNOWN` / `STALE` / `RECONCILING`, input fingerprint, `checked_at`, `expires_at`, supersession, actor, correlation and causation ids, audit and outbox references), a closed reason vocabulary that decisions foreign-key into, a per-decision table of authoritative sources with their versions and freshness, and append-only history. Tenant, subject, operation, status, expiry and branch indexes are present. `uq_fleet_partner_eligibility_current` is a partial unique index over the live decision per subject and operation, so two concurrent evaluations produce one winner and one conflict rather than two answers a caller could pick between. Triggers make a published policy version and a recorded decision immutable apart from archival and supersession; the integration suite asserts that `UPDATE` and `DELETE` are both refused by the database. Only a current, unexpired `ELIGIBLE` decision authorizes anything, and unavailable, stale, conflicting or inconclusive authoritative data records `UNKNOWN`, `STALE` or `RECONCILING` and blocks.

**The approval framework was extended, not replaced.** Migration `000020_approval_execution_binding` alters the `000006` tables: `INVALIDATED` joins the status set, and requests gain `expected_subject_version`, `payload_version`, `policy_version`, the approval controls copied at request time (`minimum_approvals`, `eligible_role_ids`, `self_approval_allowed`, `separation_of_duties`), the authorizing `eligibility_decision_id`, `execution_reference`, `invalidated_at`, `invalidation_reason`, `branch_id` and `idempotency_key`. Decisions gain `request_version` and `approved_fingerprint` and stay append-only. A new append-only `fleet_partner_approval_executions` table records every attempt with its presented and approved fingerprints, expected and observed subject versions, outcome and safe reason code. `uq_fleet_partner_approval_execution_reference` is the exactly-once guarantee, and a check constraint refuses an `EXECUTED` row carrying no evidence. A changed payload or a moved subject invalidates the approval rather than merely refusing the attempt, so a caller cannot keep trying payloads until one matches; identical retries replay; conflicting decision identifiers are rejected; cross-tenant requests answer `404`; execution and the protected operation commit in one transaction. A refused attempt is recorded in a second committed transaction, because an accountability record written inside the transaction that refuses would be rolled back with it.

**The vehicle authority contract is real.** Dispatch already owned vehicles but exposed only a request-scoped eligibility endpoint that could not answer a general lookup, so `driver-dispatch-delivery-service` now mounts a private authority surface at `/api/v1/internal/authority` with `GET /vehicles/:vehicleId` and `GET /providers/:providerId`. It answers lookup by opaque id, ownership and control, provider association, service type, verification, availability, restriction, per-document present/expired status, active assignment conflict with its stated basis, and a source version. Every route sits behind HMAC service authentication, a caller allowlist and a single-use nonce claimed in `dispatch_service_request_nonces` (migration `000036`), so a captured request cannot be replayed while its signature is still fresh; a missing nonce store answers `503` rather than passing. The response carries verdicts and never the record: the suite asserts plate number, chassis number, brand, model, document URL, name, phone and email are all absent. Unknown and malformed identifiers answer identically. Fleet's `DispatchVehicleAuthorityClient` implements both ports, with no cache and no local fallback.

**Checks.** Company eligibility reads lifecycle, verification outcome and its freshness, onboarding approval, restrictions from closure requests and blockers (where `UNKNOWN` blocks exactly like `BLOCKING`), required documents with expiry and rejection, operating branch, service capability and the subscription projection including its reconciliation status. Provider eligibility combines Fleet's own membership facts - status, consent, service category, home branch, live assignment conflict and projection freshness - with the authority's account state, verification, availability, shift, documents and conflict; the service-type check is that the authority which answered is the service Fleet's membership names. Vehicle eligibility is entirely authoritative, with the expected provider taken from Fleet's membership record rather than from the request, plus Fleet's own asset-reference conflict as an independent second source.

**Enforcement is in service logic.** `enforcement.Require` treats a nil gate as a refusal, so a service wired without one fails loudly rather than running ungated. Gated: company activation, provider linking, assignment, reassignment, acceptance and the departure path under `EXECUTION`, settlement, refund, reversal and the wallet orchestration surface, which maps hold release and earning allocation onto `FINANCIAL_RELEASE`, refund and reversal onto `FINANCIAL_REVERSAL`, and everything else onto `PAID_FEATURE`.

**Routes and permissions.** Fifteen tenant-scoped corporate routes cover evaluating and refreshing eligibility, reading decisions and their history, reading and publishing policy versions, and the full approval lifecycle: request, list, read, decisions, history, approve, reject, cancel and execute. Nine permissions separate reading an answer from forcing a re-evaluation from rewriting the policy that decides it, and requesting from deciding from cancelling from executing. Deny by default, branch scope in SQL, `404` rather than `403` for out-of-scope identifiers, and a mandatory `Idempotency-Key` on writes.

Contracts, policies, permissions, events and the runbook are in `services/fleet-partner-service/docs/eligibility-and-approval.md`.

### Verification (2026-07-28)

| Check | Result |
| --- | --- |
| `gofmt -l` on files changed in this work package | clean |
| `go vet ./services/fleet-partner-service/...` | pass |
| `go build ./services/fleet-partner-service/...` | pass |
| `go vet ./services/driver-dispatch-delivery-service/...` | pass |
| `go build ./services/driver-dispatch-delivery-service/...` | pass |
| `go test -count=1 -timeout 30m ./services/driver-dispatch-delivery-service/...` | **1224 pass / 0 fail / 28 skip** |
| `go test -count=1 -timeout 40m ./services/fleet-partner-service/...` | **384 pass / 0 fail / 0 skip** |
| Fleet migration fresh apply / replay / rollback / reapply (19, 20) | pass |
| Dispatch migration apply / replay / rollback / reapply (36) | pass |
| `git diff --check` | clean |

Both suites ran against the live PostgreSQL containers on Fleet :5446 and Dispatch :5447 with `FLEET_PARTNER_TEST_DATABASE_URL` and `DISPATCH_TEST_DATABASE_URL` populated. Fleet's integration package requires both, and the Fleet suite skipped nothing.

Two caveats stated plainly rather than rounded away.

Dispatch's 28 skips are pre-existing legacy tests gated on `TEST_DATABASE_URL` and `DISPATCH_VEHICLE_TEST_DATABASE_URL`, not on `DISPATCH_TEST_DATABASE_URL`. None is a WP9 test: the twenty vehicle-contract tests in `internal/features/authority` all ran. Re-running those 28 with `TEST_DATABASE_URL` pointed at fresh disposable databases passes; they must each get their OWN fresh database, because the legacy harness applies migrations itself and a reused database fails on `CREATE EXTENSION`.

The Fleet suite passed with zero failures on two consecutive full runs. A third run, made only to count results with `-v`, produced one failure in `TestSafetyRouteRequiresIdempotencyKey` - a WP6 test this work package does not touch - which received `403` instead of `400` because a freshly seeded role binding did not yet grant its permission. It did not reproduce in three repeats or in either full run. The symptom matches the intermittent authorization failures seen in the shared-database integration harness before this work package, and its cause is in the harness rather than in the routes: the same test passes consistently in isolation. It is recorded here rather than filtered out, and it is not fixed by WP9.

New WP9 coverage: eligible company, provider and vehicle; suspended, restricted and unverified companies; unknown and stale verification outcomes; missing and expired documents; inactive, stale and reconciling subscriptions; unverified providers; wrong service category; missing consent; expired provider documents; provider and vehicle assignment conflicts; stale provider projections; unavailable and unconfigured authorities; unknown vehicles; ownership, verification, restriction, document and source-version failures; decision expiry, supersession, immutability and concurrent evaluation; policy publication leaving history intact; tenant isolation for decisions and approvals; approval happy path with exactly-once execution; idempotent retries; fingerprint stability under key reordering; self-approval, separation of duties, insufficient quorum, rejection, conflicting decisions and expiry; payload and subject-version invalidation; execution attempt recording and append-only enforcement; HTTP deny-by-default, branch scope, IDOR, permission separation and mandatory idempotency keys; and the dispatch authority's authentication, allowlist, freshness, replay, data-minimization and state-mapping behaviour.

**WP9 complete. WP10 media and document security has not started.**

## WP10 complete - Media and document security (2026-07-28)

Media File Service is now authoritative for private media objects through
`000002_media_object_authority`. It owns opaque object identity, private
storage references, single-use upload authorizations, declared and detected
MIME, byte size, SHA-256 checksum, signature validation, safety/scan state,
short-lived object-bound download grants, retention and physical deletion.
The private API uses the shared HMAC timestamp contract, caller allowlisting
and durable nonce replay protection. Upload completion reads the actual stored
bytes and rejects oversize, incomplete, checksum-mismatched, unsupported,
MIME/signature-mismatched and executable content. An absent scanner stays
`PENDING`; an unavailable scanner becomes `UNKNOWN`; neither is treated as
safe. Unsafe content is quarantined and its bytes are removed. No signed URL,
upload token, storage credential or raw content is persisted or logged.

Fleet migration `000021_fleet_documents` adds tenant-scoped document
references, controlled lifecycle and verification states, opaque Media IDs,
source versions/freshness, append-only history and decisions, replacement and
revocation, expiry, retention/legal hold, deletion requests, reconciliation
cases, leases, retries, dead-letter/replay, audit and transactional outbox.
There is no cross-service foreign key and no URL, token, storage path, file
content, checksum or authoritative MIME column in Fleet. The production Media
client reuses the signed transport for JSON and bounded raw-body calls.

Corporate routes cover upload requests, tenant/branch-scoped reads and lists,
history, verification decisions, replacement, revocation, retention/legal
hold, deletion, private download grants and operator replay. The internal
Media-event route is behind HMAC, timestamp/nonce replay protection and a Media
caller allowlist. Inbox processing rejects changed event identities and
same-version payload conflicts, ignores older versions, opens gaps, and repairs
only through forward-only authoritative read-back.

The default-off expiry, reconciliation and deletion workers are wired in both
the API and dedicated-worker compositions with batch, interval, lease,
recovery, backoff, retry limit, dead letter, replay, kill switch and graceful
shutdown. Document changes invalidate WP9 eligibility decisions. Only a
current, safe, verified, unexpired, unreplaced, unrevoked and undeleted
document with no open reconciliation case satisfies a required-document check.

Four defects found during final PostgreSQL verification were fixed:

- Fleet inbox deduplication had hashed volatile correlation/timestamp envelope
  fields; it now hashes stable event identity plus the business payload.
- The reconciliation fixture simulated an authority older than its own event;
  it now proves repair while preserving forward-only source versions.
- The expiry fixture attempted to verify an already expired document; it now
  verifies first and exercises the real later-expiry transition.
- The shared Fleet test reset omitted migration 21's permission catalogue;
  WP10 permissions are now restored after every database truncation.

### WP10 verification (2026-07-28)

| Check | Result |
| --- | --- |
| `go vet ./services/media-file-service/...` | pass |
| `go build ./services/media-file-service/...` | pass |
| `go test -count=1 -timeout 30m ./services/media-file-service/...` | **50 top-level tests / 0 fail / 0 mandatory skip** |
| Media PostgreSQL/file-security focused suite | **35 integration tests / pass** |
| `go vet ./services/fleet-partner-service/...` | pass |
| `go build ./services/fleet-partner-service/...` | pass |
| `GOFLAGS=-p=1 go test -count=1 -timeout 40m ./services/fleet-partner-service/...` | **413 top-level tests / 0 fail / 0 skip** |
| Fleet WP10 PostgreSQL focused suite | **35 tests / pass in 323.320s** |
| Media migration fresh apply/replay/rollback/reapply and duplicate scan | pass |
| Fleet migration fresh apply/replay/rollback/reapply and duplicate scan | pass |
| `gofmt` / `git diff --check` | clean |

The full suites ran against separate disposable PostgreSQL containers on Media
:5449, Fleet :5446 and Dispatch :5447. `MEDIA_FILE_TEST_DATABASE_URL`,
`FLEET_PARTNER_TEST_DATABASE_URL` and `DISPATCH_TEST_DATABASE_URL` were set, so
the mandatory database, file-security and canonical-trip tests ran. No WP10
test skipped. New write/worker flags remain false by default and example
secrets are empty. Detailed contracts and runbooks are in
`services/media-file-service/docs/private-media-api.md` and
`services/fleet-partner-service/docs/document-domain.md`.

**WP10 complete. WP3 through WP10 are implemented and verified.**

## WP11-WP12 prerequisite remediation (2026-07-28)

WP11A adds Truck Hauling's authoritative Fleet boundary without moving source
ownership into Fleet. Migration 008 introduces durable/versioned availability,
opaque organization/provider/truck links, single-winner reservations,
idempotency fingerprints, assignment references, truck lifecycle timestamps,
nonce replay records and the transactional outbox. The private API is HMAC
authenticated, caller-allowlisted, nonce protected, expected-version guarded
and default-off. It provides reservation, eligibility, execution/read-back,
request/trip reconciliation, cancellation, provider/truck lookup and ordered
loading/unloading/proof lifecycle operations. Redis is now a matching cache;
PostgreSQL is the availability authority. The leased relay publishes signed
events to Fleet with bounded retries and dead-lettering.

Fleet now has a real Hauling execution and authority adapter using the shared
signed transport. Its existing event inbox, projections, execution-start
operations and reconciliation paths remain provider-neutral and therefore
consume Hauling without a second source of truth. Hauling is enabled only when
its explicit flag, URL and secret are present and the global kill switch is
off.

WP11B leaves Taxi hard-disabled. Configuration rejects any attempt to enable
Taxi because no real Taxi contract exists. Every Taxi execution/provider
adapter method returns an explicit UNKNOWN refusal; it performs no network,
persistence, Wallet or Notification work.

WP12 consolidates default-off integration gates and validates partial
enablement. Hauling requires explicit inbound identities/secrets, Fleet event
URL/secret, Wallet and Notification clients and an enabled outbox worker.
Example secrets are empty. The global Fleet kill switch and Hauling kill switch
override category enablement.

This remediation does not implement or start WP13 or WP14. Current readiness
must be decided only from the final real-database matrix in the remediation
report; historical or partial test results in this document are not a GO.

### Production-wiring correction

- Fleet derives Dispatch, Taxi and Hauling from one capability structure. The
  global integration kill switch wins, Taxi is always false, and Hauling
  requires both its flag and a complete client.
- The execution-start worker claims only enabled durable source operations and
  rechecks the kill switch immediately before an external write.
- Verification, Media onboarding, Wallet, Hauling provider authority and
  Hauling provider/truck link clients are real signed clients when configured.
- Eligibility selects Dispatch or Hauling authority from the membership/source
  rather than treating Dispatch as a global provider authority.
- Shared HMAC signature version 2 binds caller, method, request target,
  timestamp, nonce and body hash. Empty, replayed and altered nonces fail.
- The global kill switch prevents composition of Wallet, Media, Verification,
  Notification and Hauling mutation clients and stops their mutation workers.
- Taxi remains explicitly unconfigured and fail closed.

## Final prerequisite remediation evidence (2026-07-28)

Fleet now wires production Verification, Media, Wallet and Hauling provider,
link and execution clients only when their complete URL/identity/secret
configuration is present. Dispatch and Hauling execution capabilities are
independent; Taxi is always false. The global integration kill switch wins
over both sources, prevents mutation-client composition, stops execution
claims, outbox delivery and provider-provisioning reconciliation, and is
rechecked before an execution worker performs an external write.

The Hauling authority owns provider/truck link IDs and implements tenant-scoped
create/read, provider lookup for controlled revocation, expected-version state
transitions, reassignment, immutable history, idempotency fingerprints and
transactional outbox. A black-box integration test builds and launches the
actual Hauling binary and drives it with Fleet's production HMAC client against
the Hauling PostgreSQL database. The provider-neutral Fleet database suite
continues to prove reservation single-winner behavior, UNKNOWN reconciliation,
leases, dead letters, inbox deduplication, version gaps/conflicts and restart
recovery.

Shared service authentication is signature contract v2. Caller identity,
method, request target/query, timestamp, nonce and body hash are canonicalized
and signed. Missing, replayed or altered nonces fail authentication. Dispatch
access tokens carry unique JTIs; logout stores only a hashed JTI in Redis until
token expiry, validation fails closed on Redis outage, and session refresh
rotation retains the existing superseded-refresh protection.

Hauling notifications use the Redis request stream consumed by Notification;
there is no successful no-op delivery. Wallet absence fails closed. Ambiguous
hold, settlement and refund outcomes use authoritative read-back and remain
`UNKNOWN` when unresolved. Cash mode is explicit and false by default and
never fabricates `PAID` or `SETTLED`.

Final consecutive real-database runs:

| Service | Run 1 | Run 2 | Mandatory skips |
| --- | ---: | ---: | ---: |
| Fleet (including real Fleet-to-Hauling binary test) | pass, 167.2s | pass, 140.6s | 0 |
| Dispatch (`GOFLAGS=-p=1` to isolate shared package databases) | pass, 453.7s | pass, 465.0s | 0 |
| Wallet | pass, 106.6s | pass, 74.8s | 0 |
| Media | pass, 60.4s | pass, 53.2s | 0 |
| Hauling | pass, 30.6s | pass, 30.3s | 0 |
| Verification | pass | pass | 0 |

Notification, Support and shared service-auth suites pass. Vet and build pass
for Fleet, Dispatch, Wallet, Media, Hauling, Verification, Notification,
Support and shared Go modules. Fleet migration 21 is exercised through fresh
apply, replay, rollback and reapply in its PostgreSQL suite. Dispatch migration
37, Hauling migration 009 and Verification migration 000001 were each rolled
back and reapplied explicitly. Wallet and Media migration packages pass in
both full runs.

All required `.env.example` secret, password, token and credential-bearing
database URL values are empty. Staging enablement order is: configure secrets
out of band; deploy migrations; verify read-only authority endpoints; enable
Hauling inbound authority and outbox; enable Fleet Hauling; then enable the
execution worker. Rollback starts by activating both kill switches, waiting
for in-flight leases, and resolving UNKNOWN operations by read-back. Dead
letters retain their event/idempotency identity and are replayed only after the
downstream cause is repaired.

The duplicate-number scan still reports historical Dispatch pairs 000031 and
000032. They predate this remediation and were not renamed because rewriting
already-applied migration identity is explicitly prohibited. New Dispatch
migration 000037 and all other new service-local migrations have unique
numbers and matching down files.
