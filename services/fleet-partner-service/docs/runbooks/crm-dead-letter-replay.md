# Runbook: CRM dead-letter replay

## Symptom

`crm_worker_dead_letters` is non-zero. CRM notifications — quotation issued,
contract activated, credit hold, SLA escalation — are not reaching recipients.

## What a dead letter is

A durable notification operation whose attempt budget is exhausted, or which
failed permanently. The domain change it announces **already happened and is not
in doubt**: the quotation is issued, the contract is activated, the hold is
applied. Only the announcement is outstanding.

Nothing is lost. Each dead letter is a row that keeps its original identity.

## Find them

```
GET /fleet-partner/v1/crm/notification-operations?status=DEAD_LETTER
```

Requires `crm.quotations.read`. The response carries the subject type, subject
ID, intent type, attempt count, `safeErrorCode` and `deadLetterReason`.

Or, for a system-wide count:

```sql
SELECT intent_type, safe_error_code, count(*)
FROM fleet_partner_crm_notification_operations
WHERE status = 'DEAD_LETTER'
GROUP BY intent_type, safe_error_code
ORDER BY count(*) DESC;
```

## Diagnose before replaying

Replaying into an unrepaired downstream just burns the attempt budget again.

| `safeErrorCode` | Meaning | Repair first |
| --- | --- | --- |
| `notification_unconfigured` | no notification client in this deployment | configure notification-service |
| `notification_unavailable` | downstream refused or was unreachable | restore the service |
| `notification_timeout` | ambiguous — recorded as `UNKNOWN`, not failure | usually self-heals; investigate latency |
| `recipient_not_contactable` | the contact is archived, inactive, consent withdrawn, or has no address | fix the **contact record** |
| `recipient_unresolved` | the contact reference does not resolve | escalate as a data defect — contacts are archived, never deleted |

`recipient_not_contactable` is the one that is usually **not** an incident. It is
the system correctly declining to contact somebody who withdrew consent. Confirm
that is the situation before doing anything.

## Replay

```
POST /fleet-partner/v1/crm/notification-operations/{operationId}/replay
```

Requires `crm.quotations.manage`. Returns `202`.

Replay preserves identity: the **same row**, the **same idempotency key**, the
**same payload**. Attempts reset to zero and the row returns to `PENDING` for the
next worker pass.

That identity is the point. An operator repairing an outage replays the original
intent rather than minting a new one, so a message that actually did get through
before the failure was recorded cannot be delivered twice.

**Never** insert a new notification operation by hand for an event that already
has one.

## Order of operations

1. Repair the downstream cause.
2. Confirm the worker is enabled and running:
   `CRM_NOTIFICATION_WORKER_ENABLED=true`, `CRM_ENABLED=true`, kill switches off,
   and a worker process alive.
3. Replay a **single** operation and watch it reach `DELIVERED`.
4. Only then replay the rest.

Replaying a backlog into a still-broken downstream turns a recoverable incident
into a second, larger one.

## Lease recovery

A worker that crashed mid-delivery leaves rows in `PROCESSING` with a lease.
Those are **not** dead letters and need no operator action:
`RecoverExpiredNotificationLeases` runs at the start of every pass and returns
them to the queue once the lease lapses.

`crm_worker_lease_recoveries_total` rising steadily means workers are dying
mid-batch. That is a separate problem — investigate the worker process, not the
queue.

## Ambiguity is never resolved by guessing

An operation in `UNKNOWN` stays retryable. It is not marked delivered to tidy up
a dashboard, and it is not marked failed to force a resend. A send whose fate
could not be determined may have landed; the idempotency key is what makes the
retry safe.
