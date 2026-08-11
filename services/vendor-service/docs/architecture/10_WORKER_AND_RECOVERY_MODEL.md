# 10 — Worker and Recovery Model

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
