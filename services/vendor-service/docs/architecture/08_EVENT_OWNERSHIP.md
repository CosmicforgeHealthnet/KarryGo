# 08 — Event Ownership

## Why a service-local package, not a shared one

### Section 7 event family

Section 7 adds onboarding, representative, document, verification-projection, bank-profile and `vendor.business.activated` events. Activation emits exactly once in the transaction that first activates the graph. Verification produces `vendor.verification.case_created` and `vendor.verification.status_changed`; the latter is delivered from its leased outbox to Vendor. Inbound verification events are source/event deduped and source-version guarded.

`shared/go/events` is 22 lines of plain string constants, imported nowhere except one fallback constant in `shared/go/notifications`. It is not an outbox, publisher, or envelope abstraction, and no service in the platform (including the most mature, `fleet-partner-service`) builds its real event platform on top of it. Four different tiers of the outbox/inbox pattern coexist in the platform today, none of them shared (see [00](00_REPOSITORY_AUDIT_BASELINE.md)). Vendor Service's `internal/events` is therefore its own package, deliberately modeled on `payment-wallet-service`'s shape (envelope, DB-backed outbox with `ClaimBatch`/lease/exponential-backoff/dead-letter/`Replay`, `DisabledPublisher` default, `HTTPPublisher` using the shared HMAC contract) — judged the cleanest generic version among the four tiers, and small enough to own locally without pulling in Redis Streams infrastructure this phase does not need.

## What is built

- `Envelope` — event id, event type, schema version, aggregate type/id/version, vendor id (nullable — some events are platform-scoped), correlation id, causation id, occurred-at, producer.
- `OutboxRepository` — `Append` (same transaction as the domain mutation, via a narrow `querier` interface satisfied by both `*pgxpool.Pool` and `pgx.Tx`), `ClaimBatch` (`FOR UPDATE SKIP LOCKED`, so concurrent workers never double-claim), `MarkPublished`, `MarkFailed` (bounded exponential backoff, dead-letters after `DefaultOutboxMaxAttempts=10`), `RecoverExpiredLeases` (crash recovery — an owner that died mid-publish releases its lease back to the queue), `Replay` (operator recovery from `DEAD_LETTERED`), `PendingCount`.
- `InboxRepository` — `Claim(sourceService, eventID, eventType)`, a primary-key insert on `(source_service, event_id)`; the first claim succeeds, every redelivery returns `claimed=false` so the caller's handler is a safe no-op on retry.
- `HTTPPublisher` — signs with `shared/go/serviceauth` as `"vendor-service"`, delivers to one destination; a `Worker` (ticker + `RunPass`) drains the outbox on an interval.
- Migration `000002_events_outbox_inbox` creates `vendor_outbox` and `vendor_inbox_processed_events`.

## What is deliberately not built yet

- **No routing table for multiple destinations, still.** `Publisher` is a single interface; today's `HTTPPublisher` targets one fixed URL. **Update (Section 5):** a real producer now exists — `staffusecases.StaffService`/`RoleService` append `vendor.staff.created/.suspended/.reactivated/.roles_changed` and `vendor.role.changed` to the outbox on every mutation (via the `staffEventAdapter` in `cmd/adapters.go`), proven live by the Section 5 integration tests. There is still no destination/consumer wired (the outbox worker remains unstarted), so these accumulate as `PENDING` — correct per this document's original design (durable, never dropped, never faked), just no longer hypothetical. When a second event-type family needs a *different* destination than the first, revisit the one-Worker-per-destination-vs-routing-Publisher decision then.
- **No event-type registry, still — now with one real producer to eventually register.** `fleet-partner-service`'s `internal/events/registry.go` pattern remains premature for 5 event types; revisit once Section 5's events plus a second section's worth exist together.
- **No live-transport verification.** The outbox/inbox/idempotency schema and Go code are verified against real Postgres (migration apply/rollback/reapply — see [12](12_MIGRATION_OWNERSHIP.md)); no end-to-end HTTP delivery has been exercised, since there is no destination service registered to receive vendor-service events yet.
- **The outbox worker is wired but not started.** `cmd/main.go` constructs `outboxRepo` and an `events.NewWorker(outboxRepo, events.DisabledPublisher{}, ...)` but does not `go outboxWorker.Run(ctx)` — starting a worker that can only ever fail (no destination configured) would just generate noise. It is started once the first feature both produces an event and has somewhere real to send it.

## Section 6 event family

Section 6 appends versioned business/brand/capability/branch lifecycle events through `DomainPublisher` in the same transaction as aggregate changes. Event families are `vendor.business.created|updated|status_changed`, `vendor.capability.changed`, `vendor.brand.changed`, `vendor.branch.created|updated|status_changed`, and `vendor.branch.capability_changed`. No `vendor.business.activated` event is emitted and no consumer/destination is fabricated.

## Correlation, causation, and idempotency

Every event carries a correlation id (ties together every event produced by one logical workflow, e.g. one checkout) and an optional causation id (the specific event/command that directly produced this one). Every outbound delivery carries `Idempotency-Key: <event-id>` so a redelivered event is a safe no-op on the receiving service's own inbox — the same convention `payment-wallet-service`'s `HTTPPublisher` already uses against `fleet-partner-service`.

## Section 8A — CRM events

### Consumed (inbound, root-level `POST /internal/vendor/v1/crm/events`)

| Event type | Owning authority | Gating flag |
|---|---|---|
| `CUSTOMER_RELATIONSHIP_OBSERVED` | customer-service | `VENDOR_CRM_CUSTOMER_PROJECTION_ENABLED` |
| `CUSTOMER_IDENTITY_LINKED` | customer-service | `VENDOR_CRM_CUSTOMER_PROJECTION_ENABLED` |
| `CUSTOMER_IDENTITY_MERGED` | customer-service | `VENDOR_CRM_CUSTOMER_PROJECTION_ENABLED` |
| `CUSTOMER_CONSENT_CHANGED` | customer-service | `VENDOR_CRM_CONSENT_INTEGRATION_ENABLED` |
| `CUSTOMER_PRIVACY_CHANGED` | customer-service | `VENDOR_CRM_PRIVACY_INTEGRATION_ENABLED` |
| `CRM_ORDER_FACT_CHANGED` | the authoritative vendor commerce order publisher (does not exist yet) | `VENDOR_CRM_CUSTOMER_PROJECTION_ENABLED` |

Authority is per event type, not per service: a caller that owns one event type still cannot send another. The signed caller identity wins over any `source_service` the body claims. Protection is HMAC v2 + single-use nonce + `(source_service, event_id)` inbox dedupe with a payload fingerprint, so an identical redelivery is a no-op while the same event id carrying different data is an integrity conflict rather than a silent overwrite. Ordering is settled by `source_version`, never arrival time.

All six are currently dormant: no service publishes them. The contracts exist so the owning services have something concrete to implement against.

### Produced (outbox)

`vendor.crm.profile.created`, `vendor.crm.profile.archived`, `vendor.crm.profile.restored`, `vendor.crm.profile.linked`, `vendor.crm.privacy_state.changed`.

Payloads are minimal and carry no personal data: profile reference, business reference, state, subject type, source version, correlation id. No consumer exists yet, so they accumulate durably as `PENDING` — never silently dropped — matching the Section 5/6/7 precedent.
