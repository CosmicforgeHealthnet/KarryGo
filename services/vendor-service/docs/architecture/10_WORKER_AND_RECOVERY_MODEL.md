# 10 — Worker and Recovery Model

## Section 8I — the campaign handoff transaction model

The Section 8F dispatcher previously performed its notification handoff inside
the transaction that claimed and revalidated the delivery. Section 8I split it:

- **TX1** claims the delivery with `SKIP LOCKED`, locks the run and the customer
  profile, revalidates consent, privacy, merge state, service recovery, quiet
  hours and the frequency cap, records the consent evidence, writes the contact
  history row, reserves the delivery by pushing `next_attempt_at` beyond the
  handoff window, and commits. Every lock is released here.
- **NETWORK** calls notification-service with the delivery's stable
  `NotificationRequestRef`.
- **TX2** re-locks only the delivery and records the outcome, guarded so that a
  cancellation committed during the call wins and the result is discarded.

The reservation window (`HandoffReservation`, default two minutes) is what stops
a second worker adopting an in-flight delivery; it must exceed the Notification
client timeout. A process that dies between TX1 and TX2 leaves the delivery
claimable again once the window lapses, and the retry reuses the same
idempotency key — so the remote side deduplicates rather than sending twice.
That stable identity is the property the whole split rests on.

Contact history is written in TX1 rather than TX2 on purpose: the frequency cap
counts those rows, so recording them after the lock is released would let two
concurrent deliveries for one customer both pass the cap.

## Section 8H workers

Two durable workers and one sweeper, all reusing the accepted lease pattern: claim with `FOR UPDATE SKIP LOCKED`, renewable lease, bounded attempts, exponential backoff, dead-letter and a code-level replay hook.

- **`crm-intelligence-materializer`** builds one generation alongside the live one and swaps atomically. The swap transaction re-certifies against privacy — locking profile rows in stable order, the same lock the privacy and merge handlers take — and recomputes the availability counts from the survivors rather than from what the build believed. A crash before commit leaves the previous generation live; a run that exhausts its attempts dead-letters while the scope reports `STALE` with its last-good pointer intact. A rebuild is never allowed to make a dashboard empty.
- **`crm-export-generator`** streams rows, re-certifies privacy, uploads privately and only then promotes. Its idempotency key is the export job, so a retried generation reuses one artifact instead of orphaning objects in the store. A cancellation that commits mid-run wins, and the bytes the worker produced are deleted rather than left behind.
- **`crm-export-retention`** expires artifacts whose retention has elapsed, independently of the generator, because an overdue artifact must stop being downloadable even if generation is wedged. Expiry is also evaluated on read, so a download attempt cannot race the sweeper and find a live artifact past its expiry.

**Both clocks are the database's.** The artifact expiry and the privacy fence are written and compared by PostgreSQL, not by the service. Deriving either from the service clock — which an early draft did, and the tests caught — would let a host whose clock differs slightly publish artifacts past their retention and make every export look like it raced a redaction.

## Section 8G workers

**Section 8G adds no worker, and needs none.** Every multi-row consequence happens inside the transaction that causes it: archiving an account cancels its open tasks there, archiving a contact deactivates its memberships there, and a redaction or identity merge suppresses or moves corporate rows inside the Section 8A privacy transaction. Nothing is left for a sweeper to reconcile, so there is no state in which a half-applied change is waiting to be noticed.

Correctness under concurrency comes from row-level fences rather than background repair. Account mutations take the account row; contact and membership changes additionally take the Section 8A profile row, which is the same lock privacy actions and identity merge take. That shared discipline is what makes both commit orderings safe: a redaction that lands first is seen by the contact write, and one that lands second suppresses the contact it finds. Six deterministic lock-ordered tests pin those races, with no sleep-based timing anywhere.

## Section 8E — the campaign audience preparer

`crm-campaign-preparer` claims due snapshots under a lease with `SKIP LOCKED`, renews it with a heartbeat while the audience is written, retries with bounded backoff, and dead-letters after the configured ceiling. It reuses the Section 8B materializer's shape rather than introducing a second worker system, and it prepares audiences and nothing else — it never contacts notification-service, because Section 8E does not deliver.

The snapshot row is its own durable job. One table rather than two means the lease, the retry state and the audience being built cannot disagree about which run they belong to, and a crashed worker simply lets its lease lapse for another to adopt. A partially written snapshot is safe to restart because target ids are derived from `(snapshot, profile)` rather than generated randomly, so a re-run reproduces exactly the same rows instead of duplicating them.

**A partial audience can never become current.** The new snapshot is built alongside the previous one while it is still `BUILDING` and therefore invisible as a prepared audience; promotion is one transaction that locks every candidate profile in stable id order, re-sweeps for privacy, merge and service-recovery changes that committed mid-build, recomputes the counts from the rows that actually survived, makes the snapshot `CURRENT`, retires the previous one and moves the campaign's pointer. A failure at any point before that transaction leaves the last good audience and the pointer to it exactly as they were — a campaign can report `PREPARATION_FAILED` while still holding a usable prepared audience, which is a far safer answer than an empty one.

Only one preparation per campaign can be in flight, enforced by a partial unique index rather than a process-local guard, so several instances and several concurrent requests all converge on one run.

A dead-lettered preparation is returned to the queue by `ReplayCampaignPreparation`, a code-level operator hook rather than a Vendor-facing endpoint: replaying failed infrastructure work is an operational action, not ordinary CRM authority. A preparation whose pinned segment generation has been released is failed terminally rather than retried, because retrying it could never succeed.

## Section 8D — the loyalty expiry sweeper

The sweeper is a `jobs.Job` ticker like every other Vendor worker, but it deliberately has no queue table and no lease, unlike the Section 8B segment materializer. It does not need them: a point lot with remaining points and a past expiry **is** the outstanding job, and it stays outstanding until a committed transaction settles it. Durability, crash recovery and retry therefore come from the schema rather than from bookkeeping that could itself drift.

Multi-worker safety comes from `FOR UPDATE SKIP LOCKED` on the account row: two workers never claim the same account, and an account currently being debited by a request is skipped rather than blocked — that request settles its own expiry anyway. Each account is settled in its own transaction, so one failure cannot roll back the accounts already swept, and the failed one is simply still due next pass.

The sweeper is an optimisation, not a correctness requirement. Every balance-sensitive command settles due expiry before it acts, and `available_points` always excludes expired lots regardless. A sweeper that has been down for hours delays the bookkeeping without ever letting an expired point be spent, and catches up when it returns. Turning `VENDOR_CRM_LOYALTY_ENABLED` off stands it down and defers the backlog; it does not discard it.

## Section 8C scheduling boundary

Section 8C adds no worker. UTC `due_at` and derived `is_overdue` make tasks reminder-ready, but Vendor does not poll and send reminders because Notification Service owns transport and no current handoff contract was established. Command recovery is provided by the existing idempotency store, optimistic versions, transactional audit/outbox, and deterministic state transitions.

## Convention chosen: ticker + goroutine, not a task-queue dependency

### Section 7 workers

- `verification-reconciliation` leases durable operations, resolves unknown outcomes by client reference, backs off, and dead-letters after eight attempts.
- `verification-expiry` derives `EXPIRED` without changing authoritative `source_version`; effective reads enforce `expires_at` immediately.
- Verification's `CallbackWorker` separately leases `vendor.verification.status_changed` outbox rows, signs Vendor callbacks, retries with bounded backoff, dead-letters after eight attempts, and exposes a code-level replay operation. Callback and polling are complementary recovery paths.

The dominant pattern across the platform is a plain `time.Ticker`-driven goroutine registered directly in `cmd/main.go` (hauling's match-recovery worker, dispatch's push-notification-adjacent workers running alongside its asynq queue). Only `driver-dispatch-delivery-service` and `api-gateway` use a real task-queue library (`hibiken/asynq`), and specifically because they need delayed, independently-retried *single* tasks (e.g. "re-broadcast this specific booking in 30 seconds") — a shape the ticker convention handles poorly. Vendor Service starts with the ticker convention (`internal/jobs.Job`) because no feature exists yet that demonstrably needs per-task delayed retry; if commerce fulfilment planning later needs exactly that shape (e.g. "retry this specific fulfilment unit's mission request in 30s with backoff"), asynq is the proven option to reach for then — not decided speculatively now.

## What exists

- `internal/jobs.Job` — `{Name, Interval, Pass func(context.Context) error}`, `Run(ctx, onError)` ticks `Pass` until `ctx` is cancelled, on-error is caller-supplied (transient failures never kill the ticker, matching hauling's `RunMatchRecoveryWorker` — a job stopping permanently on one bad tick would be worse than logging and retrying).
- `internal/events.Worker` — the outbox delivery loop, same ticker shape, `RunPass` exposed separately so a test or an operator "drain now" endpoint can call it directly without waiting for the ticker.
- `Section7Metrics` — low-cardinality counters for submissions, transitions, duplicate/stale callbacks, activation evaluations/blocking dimensions, bank failures/changes and expiry. The expiry worker emits a JSON snapshot to structured logs; labels never contain Vendor ids, document refs, bank data or PII.

## Recovery guarantees already built into the outbox (verified against real Postgres)

- **Lease-based crash recovery**: `ClaimBatch` sets `lease_expires_at`; `RecoverExpiredLeases` reclaims any record whose owner died mid-publish before its lease expired, so a crashed worker process never permanently strands events.
- **Bounded exponential backoff, then dead-letter**: `MarkFailed` schedules the next attempt at `LEAST(300, 2^attempts)` seconds and dead-letters after `DefaultOutboxMaxAttempts=10`, so a persistently-failing destination stops being retried forever but is never silently dropped.
- **Operator replay**: `Replay` returns a `DEAD_LETTERED` record to `PENDING` for a fresh attempt cycle.
- **Migration replay is itself recovery-tested**: `internal/database/migrate_test.go` (gated on `VENDOR_SERVICE_TEST_DATABASE_URL`) applies all migrations, verifies schema state, rolls back the last migration and verifies its down file actually undid the change (not just that the tracker forgot it), then reapplies and verifies consistency — proving the query-driven `RollbackLastMigration` never assumes a fixed migration count. Run live against a throwaway database during this architecture phase; result: pass (apply → verify identity row + idempotency table exist → rollback → verify idempotency table dropped and `MigrationsCurrent` correctly reports it missing → reapply → verify consistent again).

## What is deliberately not built yet

- The generic Vendor domain-outbox worker is still not started because Section 5/6/7 domain events have no registered multi-destination consumer. This does not apply to the two Section 7 Vendor workers or Verification's dedicated callback worker, which are started when their integrations are enabled.
- No stranded-record sweep beyond the generic outbox lease mechanism exists yet (compare to hauling's `RunMatchRecoveryWorker`, which re-adopts strand bookings after a specific business-domain timeout) — a business-specific recovery worker (e.g. "re-plan a fulfilment unit stuck in `pending_match` past its search window") is written when `internal/features/fulfilment` is implemented, following the same pattern.
- No dead-letter operator HTTP route exists yet (mirrors `fleet-partner-service`'s own honestly-disclosed gap here — cross-tenant/platform-wide operator routes need an operator-auth class that doesn't exist yet either; deferred together).

## Section 8B — the segment materializer

The first Vendor Service worker whose failure mode is a *wrong answer* rather than a delayed one, which shapes its design.

- **Claim and heartbeat**: `UPDATE ... WHERE id = (SELECT ... FOR UPDATE SKIP LOCKED)` claims a lease under a process-unique worker id. A heartbeat renews that lease while the generation is built, so a healthy long run cannot be adopted by a second instance merely because the initial TTL elapsed.
- **Recovery**: a worker that dies simply lets its lease lapse, and another adopts the run. Rows written by the dead attempt are discarded on restart, because the generation being written is never the live one.
- **Retry, dead-letter, and replay**: bounded attempts with linear backoff, then `DEAD_LETTER`, so a permanently broken definition stops consuming capacity without being silently forgotten. The code-level `ReplayMaterialization` operator hook returns only a dead-lettered run to `QUEUED`; there is no public "force rebuild the database" route.
- **Fenced atomic generation swap**: the new membership is built alongside the previous one. Before making it live, the swap transaction locks every candidate profile in stable id order, then re-evaluates the complete compiled rule while those locks are held. Privacy actions and identity merge take the same profile-row locks, so either commit ordering is safe: a privacy/merge commit that wins first is seen by certification, while one that follows removes or transfers membership after the swap. The pointer, count and evaluation state change in that same transaction. A crash before commit leaves the last good audience exactly as it was.
- **Failure is visible, not silent**: the segment reports `STALE` or `FAILED` with an error code while still serving the last successful membership. An empty audience would read as "nobody qualifies"; only "this is out of date" is recoverable by the person looking at it.
- **Retention**: superseded generations are released after a successful swap, subject to the `GenerationRetainer` port so a later section can pin one.
- **Invalidation** is dependency-scoped and durable: each stored definition version records the registry fields its rule reads, so tag, commercial, consent and profile-creation changes restake only dependent segments. Marking `STALE` and inserting/reusing the queued evaluation happen in the caller's transaction. Restriction and marketing suppression invalidate only the affected eligibility dependencies and immediately remove the relationship from those current audiences with count repair, without changing consent decisions. Redaction removes current membership immediately; archive/restore and identity merge restake every dynamic segment in that tenant because their effects are implicit in every audience.

Batch size, lease TTL, attempt ceiling, backoff and the preview count budget are all configuration rather than magic numbers, because they are the knobs an operator reaches for when a large tenant makes evaluation expensive.

## Sections 10/11 production workers

Cloud Kitchen occurrence materialization is bounded to the plan horizon and
uses `(plan_id, occurrence_key)` as its durable uniqueness boundary. The
closure schema adds a renewable worker lease, attempt/replay counters,
last-error and dead-letter timestamps; capacity conflicts remain visible as
`CAPACITY_CONFLICT` and can be retried or rescheduled by an authorised manager.
Successful occurrences link to a Vendor production job. No worker creates an
Order or calls an external fulfilment service.
