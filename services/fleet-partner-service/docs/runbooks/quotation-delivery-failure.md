# Runbook: quotation delivery failure

## Symptom

A quotation is `ISSUED` but the customer says they never received the link.
`crm_notification_retry_backlog` is rising, or `crm_worker_dead_letters` is
non-zero.

## What is guaranteed

The quotation **is** issued. Its immutable version exists and the customer
access token is valid. Delivery is a separate, durable operation: a notification
outage never rolls back a commercial commitment.

Nothing here is lost. The intent is a row.

## Triage

1. Find the operation:

   ```
   GET /fleet-partner/v1/crm/notification-operations?subjectId=<quotationId>
   ```

   Read `status`, `attempts`, `safeErrorCode` and `nextAttemptAt`.

2. Interpret the status:

   | Status | Meaning | Action |
   | --- | --- | --- |
   | `PENDING` / `FAILED_RETRYABLE` | still queued | wait for the next pass |
   | `UNKNOWN` | the send may have landed | do **not** resend blindly; see below |
   | `DEAD_LETTER` | attempts exhausted, or permanent | see below |
   | `DELIVERED` | notification-service accepted it | the problem is downstream |

3. Check the worker is actually running: `CRM_NOTIFICATION_WORKER_ENABLED=true`,
   `CRM_ENABLED=true`, and neither `FLEET_CRM_KILL_SWITCH` nor
   `FLEET_INTEGRATIONS_KILL_SWITCH` engaged. If notification-service is
   unconfigured the queue is deliberately left untouched rather than drained
   into a fake success.

## Status `UNKNOWN`

A send whose fate could not be determined. It may have arrived.

Do not manually resend. The worker retries with backoff, and the operation
carries an idempotency key that notification-service honours, so a retry cannot
duplicate a delivery that already happened. Manually creating a *new* operation
would.

## Status `DEAD_LETTER`

Read `safeErrorCode`:

- `recipient_unresolved` — the contact reference no longer resolves. Contacts are
  archived, never deleted, so this should not happen. Escalate as a data defect.
- `recipient_not_contactable` — the contact is archived, inactive, has withdrawn
  consent, or has no address. This is **correct behaviour**. Fix the contact
  record, then replay.
- `notification_unconfigured` / `notification_unavailable` — a downstream
  outage. Repair it first, then replay.

Replay preserves identity:

```
POST /fleet-partner/v1/crm/notification-operations/{operationId}/replay
```

Same row, same idempotency key, same payload. Never mint a new operation for the
same event.

## Delivering out of band

If the customer needs the quotation now and delivery cannot be repaired: the raw
access token was returned exactly once, in the issue response, and **cannot be
retrieved again**. There is no endpoint that produces it, and only a keyed digest
is stored.

To give the customer a working link, **supersede** the quotation with a
replacement and issue that. Superseding revokes the old token and preserves the
original issued version and its history.

## Escalation

If `crm_worker_dead_letters` is rising steadily rather than sitting flat, the
problem is systemic — bad configuration, an expired service secret, or
notification-service rejecting the whole event type — rather than per-message.
Stop replaying individual operations and fix the configuration first.
