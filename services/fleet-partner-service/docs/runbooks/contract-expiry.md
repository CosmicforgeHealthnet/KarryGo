# Runbook: contract expiry

## Symptom

A contract shows `EXPIRED` unexpectedly, or a contract past its end date is
still `ACTIVE`.

## The worker never terminates a contract early

The expiry sweep claims only contracts whose `effective_to` is **in the past**,
and the transition re-checks the date under a row lock before writing. A contract
with time left on it cannot be swept up.

If a contract expired earlier than expected, the cause is its `effective_to`, not
the worker:

```sql
SELECT status, effective_from, effective_to, activated_at, terminated_at
FROM fleet_partner_contracts WHERE id = '<contractId>';
```

Then read the immutable history, which records who moved it and why:

```
GET /fleet-partner/v1/contracts/{contractId}/history
```

## A contract past its end date is still ACTIVE

Check, in order:

1. `CONTRACT_EXPIRY_WORKER_ENABLED=true`
2. `CRM_ENABLED=true`
3. `FLEET_CRM_KILL_SWITCH` and `FLEET_INTEGRATIONS_KILL_SWITCH` are **off**
4. A worker process is running — either the API with in-process workers, or
   `cmd/worker`
5. `effective_to` is actually set. A contract with no end date never expires;
   that is what an open-ended agreement means.

The sweep is batched (`FLEET_CRM_WORKER_BATCH_SIZE`, default 25) on an interval
(`FLEET_CRM_WORKER_INTERVAL_SECONDS`, default 30). A large backlog drains over
several passes rather than in one.

## Renewal reminders

The reminder worker touches **no contract state**. It records only a durable
notification intent, keyed by the contract ID and its end date, so a failure to
send a reminder cannot change a contract.

If reminders are not arriving, this is a delivery problem — see
[quotation-delivery-failure.md](quotation-delivery-failure.md), which covers the
same queue.

If the end date moves, a fresh reminder is raised: the key includes the date, so
a renewal legitimately re-notifies while a repeated sweep of the same term does
not.

## Expiry is not termination

They are different states and neither substitutes for the other:

- `EXPIRED` — the agreed term ended
- `TERMINATED` — somebody ended it, with a reason and an approval where policy
  requires one

Neither calculates a fee, creates a refund or posts to a wallet. What a party
owes on early termination is settled outside this service.

## What is preserved

Every version and every document survives expiry and termination. Nothing is
deleted, and the activated version stays immutable.
