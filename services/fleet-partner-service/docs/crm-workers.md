# CRM workers (Section 14)

## What runs

| Worker | Flag (default false) | What it does |
| --- | --- | --- |
| Quotation expiry | `QUOTATION_EXPIRY_WORKER_ENABLED` | `ISSUED`/`VIEWED` past `valid_until` → `EXPIRED` |
| Contract expiry | `CONTRACT_EXPIRY_WORKER_ENABLED` | past `effective_to` → `EXPIRED` |
| Contract renewal reminder | `CONTRACT_RENEWAL_WORKER_ENABLED` | notification intent inside the renewal window |
| SLA evaluation | `SLA_EVALUATION_WORKER_ENABLED` | resolves `NOT_EVALUATED` observations |
| SLA escalation | `SLA_ESCALATION_WORKER_ENABLED` | notification and support intents for `MISSED` |
| CRM notification delivery | `CRM_NOTIFICATION_WORKER_ENABLED` | drains the durable notification queue |

All six are driven by one loop (`Service.RunWorkers`) so a deployment cannot end
up with the expiry sweep running and the notification drain not — the shape of
failure where quotations quietly expire and nobody is told.

`FLEET_CRM_KILL_SWITCH` and `FLEET_INTEGRATIONS_KILL_SWITCH` stop every worker
regardless of the individual flags. Nothing starts unless at least one worker is
enabled: an idle ticker that never does anything is a goroutine and a log line
pretending to be a capability.

The loop is wired in **both** binaries — `cmd/main.go` (in-process) and
`cmd/worker/main.go` (dedicated) — through the same `app.StartCRMWorkers` helper
the API uses, so a split deployment cannot end up with different gates.

## Tuning

| Variable | Default |
| --- | --- |
| `FLEET_CRM_WORKER_INTERVAL_SECONDS` | 30 |
| `FLEET_CRM_WORKER_BATCH_SIZE` | 25 |
| `FLEET_CRM_WORKER_MAX_ATTEMPTS` | 8 |

## Shared guarantees

- **Server time is the authority.** A client timestamp never expires a quotation
  or a contract.
- **Every claim is re-checked under a row lock.** The claim query is an index
  scan for candidates; the transition re-reads the status and the date and does
  nothing if they moved. An acceptance landing between the claim and the write
  wins.
- **Failures are isolated per row.** One quotation failing does not stop the
  batch; the rest are independent and the failure is retried on the next pass.
- **Failures are isolated per worker.** A Media outage stalling the contract
  sweep must not stop quotations from expiring, because an expired quotation
  that stays open is an offer a customer can still accept.
- **Shutdown is graceful.** The loop returns on cancellation between passes, and
  a pass in flight finishes its batch. Anything leased and not completed is
  recovered by the next process through lease expiry, so a restart mid-batch
  loses nothing.
- **No HTTP call happens inside an open transaction.**

## Quotation expiry

Claims `ISSUED` and `VIEWED` quotations whose `valid_until` has passed, using the
partial index `idx_fleet_crm_quotations_expiry` so the sweep is an index scan.

It **never** expires an `ACCEPTED`, `REJECTED`, `CANCELLED` or `SUPERSEDED`
quotation: the claim excludes them and the transition re-checks under the lock.
A resolved quotation cannot expire out from under its resolution.

Each expiry appends history, writes audit and outbox, and records a durable
notification intent — all in one transaction.

## Contract expiry and renewal

Expiry **never terminates early**: the claim requires `effective_to` to be in the
past and the transition re-checks it under the lock. A contract with time left
cannot be swept up.

The renewal reminder worker touches **no contract state at all**. It produces
only a notification intent, keyed by the contract ID *and its end date*, so a
renewal that moves the date raises a fresh reminder while a repeated sweep of the
same term does not. A reminder failure therefore cannot change a contract.

## SLA evaluation

Observations are append only, so a resolved outcome is recorded as a **new row at
the next source version** rather than by editing the one that arrived. The
immutable record of what a source said stands.

An observation whose metric or unit cannot be interpreted becomes `UNKNOWN`; one
with no measured value becomes `INSUFFICIENT_DATA`. Neither is ever turned into
`MET`.

## SLA escalation

Produces a notification intent, and a support-case intent **only where a real
contract exists** — raising one against no agreement produces noise nobody can
action.

It creates no penalty, no customer credit, no provider deduction, no refund and
no wallet journal.

## Notification delivery

The queue table is `fleet_partner_crm_notification_operations`. A row is written
inside the domain transaction that caused it; the delivery is not. An outage at
notification-service must not roll back a commercial commitment already made to
a customer.

`UNIQUE (organization_id, idempotency_key)` means the same domain event can never
produce two deliveries.

### Leasing

`ClaimNotifications` takes a lease with `FOR UPDATE SKIP LOCKED` and an expiry.
A second worker never picks up work in flight, and a worker that crashed after
claiming has its rows returned to the pool once the lease lapses rather than
leaving them stuck forever. `RecoverExpiredNotificationLeases` runs at the start
of every pass.

### Outcomes

| Outcome | Effect |
| --- | --- |
| delivered | `DELIVERED` with the provider's reference |
| unknown | stays `UNKNOWN`, retried after backoff |
| retryable | `FAILED_RETRYABLE`, or `DEAD_LETTER` once attempts are exhausted |
| permanent | `DEAD_LETTER` immediately |

**`UNKNOWN` is a first-class outcome.** A send whose fate could not be
determined may well have landed. Recording it as failed invites a duplicate;
recording it as delivered hides a customer who was never told. It stays
retryable so the next pass can read back.

A timeout maps to `UNKNOWN`, not to failure.

### No fabricated success

If notification-service is unconfigured the queue is **left exactly as it is**
rather than drained into a fake success: an operator must be able to see that
nothing was sent. There is no no-op adapter that reports delivery.

### Recipients

The recipient is an opaque contact reference. The address is resolved at
**delivery time from the live contact record**, never from the queued payload, so
a customer who withdraws consent between the intent being recorded and the
worker running is not contacted.

A contact that cannot be resolved, or is not contactable, is a **permanent**
failure rather than an endless retry that hides the problem.

The queued payload carries no address, no name and no line items.

## Dead letter and replay

```
GET  /fleet-partner/v1/crm/notification-operations?status=DEAD_LETTER
POST /fleet-partner/v1/crm/notification-operations/{operationId}/replay
```

Replay preserves the **identity**: the same row, the same idempotency key, the
same payload. An operator repairing a downstream outage replays the original
intent rather than minting a new one that could deliver twice. Attempts reset to
zero and the row returns to `PENDING`.

See [runbooks/crm-dead-letter-replay.md](runbooks/crm-dead-letter-replay.md).

## Metrics

`RefreshMetrics` runs at the end of every pass and publishes the gauges listed in
[crm-events.md](crm-events.md). Labels are statuses, stages, outcomes and worker
names only — never an organization or record ID.

Key operational gauges:

- `crm_notification_retry_backlog`
- `crm_worker_dead_letters`
- `sla_unknown_outcomes_total`
- `contracts_expiring_total`
