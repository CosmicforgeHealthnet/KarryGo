# Events and Jobs

## Event envelope

`internal/events.NewEnvelope` produces:

```json
{
  "eventId": "evt_…", "eventType": "corporate.…", "schemaVersion": 1,
  "aggregateType": "organization", "aggregateId": "org_…", "aggregateVersion": 3,
  "organizationId": "org_…", "correlationId": "…",
  "occurredAt": "2026-07-25T00:00:00Z", "producer": "fleet-partner-service",
  "payload": { }
}
```

Organization-owned events require `organizationId`; platform-scoped events
(no organization) must pass an explicit flag and be documented here.
Currently documented platform-scoped events: none.

## Transactional outbox (`fleet_partner_outbox`)

- `Append` is called with the same transaction as the domain mutation, so an
  event exists if and only if the mutation committed (verified by
  `TestOutboxTransactionAtomicity`).
- The publisher worker claims due `PENDING` rows with
  `FOR UPDATE SKIP LOCKED` under a lease (`lease_owner`,
  `lease_expires_at`); expired leases are reclaimable.
- Publication is retried with exponential backoff (capped 300s); failures are
  stored redacted (`last_error_redacted`); after `max attempts` (default 10)
  events move to `DEAD_LETTERED`. Published events remain in the table per
  retention policy.
- **No transport exists in the repository yet.** The wired publisher is
  `DisabledPublisher`, which returns `ErrPublisherNotConfigured`; the worker
  treats that as a configuration state — the lease lapses, no attempt is
  consumed, nothing is discarded, and
  `fleet_partner_outbox_publisher_unconfigured_total` counts the condition.
  Integrating a real broker means implementing the one-method `Publisher`
  interface and wiring it in `cmd`. Verified against a live database
  (`TestOutboxWorkerDisabledPublisherKeepsEventsPending`): after the worker
  runs, the event is still `PENDING` with `attempts = 0` and
  `published_at` null.

## Consumer inbox (`fleet_partner_inbox`)

Primary key `(consumer_name, event_id)` deduplicates deliveries:

- first delivery inserts `PROCESSING`; completion (`MarkProcessed`) happens in
  the same transaction as the business effect — an event is never marked
  successful before its local transaction commits;
- duplicate delivery of a `PROCESSED` event returns `ErrDuplicateEvent`
  (acknowledge, skip);
- the same event ID with a different payload hash returns
  `ErrPayloadMismatch` — treated as a contract/security error, never processed;
- failed processing is recorded (`FAILED` + `error_code`) and remains
  recoverable — a redelivery re-claims the row;
- concurrent duplicate deliveries serialize on the row lock; exactly one
  processes (verified by `TestInboxDeduplication`);
- out-of-order handling can layer on source versions later.

## Durable jobs (`fleet_partner_jobs`)

Statuses: `PENDING → CLAIMED → RUNNING → SUCCEEDED | RETRY_SCHEDULED →
… → DEAD_LETTERED`, plus `FAILED` (reserved), `CANCELLED`.

- **Enqueue** (optionally in a domain transaction) with an
  organization-scoped `dedupe_key`: an active duplicate within the same
  organization returns the existing job ID (partial unique index on
  `COALESCE(organization_id,''), job_type, dedupe_key`). Two organizations
  may hold the same key independently.
- **Claim**: batch `FOR UPDATE SKIP LOCKED`, leasing with
  `lease_owner`/`lease_expires_at` and incrementing `attempts`. Expired
  leases from crashed workers are reclaimed by the same query.
- **Heartbeat / complete / fail / dead-letter are owner-only** — a worker
  that lost its lease gets `ErrNotLeaseOwner` and must stop.
- **Retry**: exponential backoff (capped 600s) until `max_attempts`, then
  `DEAD_LETTERED`. Errors stored redacted.
- **Tenant revalidation**: before execution the worker compares the payload's
  `organizationId` with the job row; a mismatch dead-letters with a
  `security:` reason and increments
  `fleet_partner_job_tenant_mismatch_total`.
- **Shutdown**: context cancellation stops the claim loop; in-flight handlers
  finish under their lease or the lease expires and another worker recovers
  the job. Handlers must therefore be idempotent.

## Degraded modes

| Condition | Behaviour |
| --- | --- |
| Event transport unconfigured | Events accumulate durably as PENDING; metric counts the condition; nothing lost |
| Redis unavailable (optional) | Startup continues; features needing Redis degrade; readiness unaffected unless required |
| Downstream service down | Client calls classify as transient/permanent; only transient retries; readiness unaffected |
| Database unavailable | Startup fails fast; at runtime `/ready` fails, `/health` stays up |

## Metrics

Outbox: backlog gauge, published/failure/claim-error/unconfigured counters.
Jobs: per-status gauges, claims, retries, tenant mismatches, lease-lost,
succeeded counters. Worker readiness feeds `/ready` when running in-process.

---

## Sections 8–10 events

All are `.v1` schema-versioned corporate events carrying organizationId,
correlationId, aggregate identity/version, event ID, occurredAt and producer.

**Produced.** Identity and session: `corporate_identity.created`,
`corporate_identity.contact_verified`, `corporate_session.created`,
`corporate_session.revoked`. Organization: `organization.created`,
`.updated`, `.contact_verified`, `.business_submitted`, `.under_review`,
`.changes_required`, `.activated`, `.suspended`, `.reactivated`,
`.closure_requested`, `.closed`. Staff and IAM: `organization_user.invited`,
`.activated`, `.suspended`, `.reactivated`, `.ended`, `role.created`,
`role.updated`, `role_binding.created`, `role_binding.revoked`. Approvals:
`approval.requested`, `.step_completed`, `.approved`, `.rejected`,
`.expired`. Providers: `organization.provider.invited`,
`.association_accepted`, `.association_declined`, `.activated`,
`.suspended`, `.reactivated`, `.ending`, `.removed`,
`.capabilities_changed`, `.policies_changed`.

**Consumed** (through the inbox, with source-version checks):
`media.asset.status_changed`, `organization.verification.status_changed`,
`provider.verification.changed`, `provider.identity.status_changed`,
`provider.organization_association.changed`,
`wallet.organization_account.created`, `wallet.provider_subwallet.created`,
`wallet.account.status_changed`, `notification.delivery.status_changed`.

### Never in a payload

OTP codes, invitation tokens, access or refresh tokens, identity documents,
full financial account details. Invitation events carry a masked email and a
reference — never the token that was emailed.

## Sections 8–10 workers

| Worker | Does |
| --- | --- |
| Maintenance sweeps | expire OTP challenges, sessions, staff and provider invitations |
| Provider reconciler | re-drives incomplete provisioning sagas, resolving unknown outcomes by read-back |
| Outbox publisher | unchanged (disabled transport adapter until a broker exists) |
| Durable job worker | unchanged (leases, heartbeats, tenant revalidation) |

Every sweep is a bounded, idempotent `UPDATE` that only expires records whose
deadline genuinely passed — no worker completes an operation on a timeout
assumption. Unknown external write outcomes are always resolved by reading
back before any retry, so a repeat cannot duplicate a provider identity or a
wallet account.
