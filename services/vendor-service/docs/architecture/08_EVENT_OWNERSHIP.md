# 08 — Event Ownership

## Section 8H events

Section 8H emits four aggregate events through the existing transactional outbox: `vendor.crm.intelligence.materialized`, `vendor.crm.intelligence.failed`, and for exports `vendor.crm.export.requested`, `.completed`, `.failed` and `.expired`.

They are **scope-level and count-level, never row-level**. An intelligence event carries the business reference, optional branch, model key and version, generation, source instant and the availability counts. An export event carries the job reference, type, format, scope, status and row count. None carries a customer list, a score per customer, a filter document, a download token or an artifact location — an event bus is the last place a customer export should be reconstructable from, and one audit row per customer score would be a second copy of the score set nobody asked for.

No consumer exists yet, so — as with Sections 5-8G — these accumulate durably as `PENDING` rather than being silently dropped. Analytics Service is the natural future consumer of the aggregate intelligence events; that would not move CRM authority out of Vendor Service, and no cross-service database access is involved either way.

## Section 8G events

Section 8G emits Vendor-owned domain events through the existing transactional outbox, in the same transaction as the mutation that caused them: `vendor.crm.corporate.created`, `.updated`, `.status.changed`, `.archived`, `.scope.changed`, and the `contact`, `membership`, `location` and `cost_center` `created`/`updated`/`archived` families.

Each carries **references and the shape of the change**, never contents: the corporate reference, the child reference, the new status or role, and the version. No event carries an organization's contact list, a customer profile reference's contact details, or anything that would make the bus a customer directory. A bulk change emits one event describing it, not one per row.

No consumer exists yet, so — as with Sections 5-8F — these accumulate durably as `PENDING` rather than being silently dropped. Section 8G consumes nothing new: corporate contact privacy and identity merge arrive through the existing Section 8A ingress, which is why this section registers no internal route.

## Section 8E produced events

`vendor.crm.campaign.created`, `vendor.crm.campaign.updated`, `vendor.crm.campaign.archived`, and `vendor.crm.campaign.prepared`. All are appended to the transactional outbox inside the same transaction as the mutation they describe.

Payloads carry the campaign's identity, scope, channels, definition version and hash, and — for `prepared` — the snapshot reference, the target and control counts, and `send_time_recheck_required: true`. They never carry the audience's membership. A target list on an event bus would be a customer export nobody asked for, and a test rejects a customer reference appearing in any campaign event payload.

**Events Section 8E deliberately does not emit:** `campaign.sent`, `notification.sent`, `sms.sent`, `email.sent`, or anything else describing a delivery. This section prepares; Section 8F executes, and only Section 8F may say something was sent. A preparation that fails is recorded on the snapshot row and in the audit log rather than published, because a failed internal rebuild is an operational fact rather than a domain event other services need.

## Section 8D produced events

Section 8D appends `vendor.crm.loyalty.program.updated`, `vendor.crm.loyalty.account.created`, `vendor.crm.loyalty.points.credited`, `vendor.crm.loyalty.points.debited`, `vendor.crm.loyalty.points.expired`, `vendor.crm.loyalty.adjustment.applied`, `vendor.crm.loyalty.entry.reversed`, and `vendor.crm.loyalty.account.merged` to Vendor's transactional outbox. Payloads carry business, profile, account and entry references, the entry type, the signed delta, the resulting balance, the programme version and the occurrence instant — never the operator's free-text reason, the command key, the producer's source reference, or any canonical PII.

Section 8D emits no `payment.*`, `wallet.*`, `refund.*`, `settlement.*` or `payout.*` event, and consumes no new external event. A points movement is not a financial movement, and a consumer that treated one as the other would be wrong; nothing on this bus invites that reading.

## Section 8C produced events

Section 8C appends `vendor.crm.interaction.created`, `vendor.crm.interaction.corrected`, `vendor.crm.task.created`, `vendor.crm.task.updated`, `vendor.crm.task.started`, `vendor.crm.task.reassigned`, `vendor.crm.task.completed`, and `vendor.crm.task.cancelled` to Vendor's transactional outbox. Payloads carry identifiers, controlled state, version, and scope metadata only—never note bodies, task descriptions, or canonical PII. It consumes no new external event and creates no notification event.

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

## Section 8B events

### Consumed

**None.** Section 8B reasons entirely over data Vendor Service already owns — its own tags and the Section 8A projections — so it registers no event type and adds no caller to the authority allowlist. It has no internal route at all.

### Produced (outbox)

`vendor.crm.tag.created`, `vendor.crm.tag.updated`, `vendor.crm.tag.archived`, `vendor.crm.segment.created`, `vendor.crm.segment.updated`, `vendor.crm.segment.activated`, `vendor.crm.segment.archived`, `vendor.crm.segment.materialized`.

No event is emitted per membership row. A tag applied to five hundred customers produces one event describing the batch, not five hundred describing customers.

`vendor.crm.segment.materialized` carries the audience's **identity** and never its contents: segment reference, definition version, definition hash, generation, member count and the evaluation instant. That is exactly what a future campaign service needs to reference an exact audience state, and exactly not a customer list on an event bus.

As with Sections 5-8A, no consumer exists yet, so these accumulate durably as `PENDING` rather than being silently dropped.

## Section 9A events

### Consumed (inbox)

**None.** Section 9A registers no internal route and consumes no event. Nothing
outside this service produces a menu, and the read contract other services will
eventually use belongs to Section 9C — deliberately, so that section can design
it rather than inherit an accidental one.

### Produced (outbox)

`food.menu.created`, `food.menu.updated`, `food.menu.activated`,
`food.menu.paused`, `food.menu.archived`, `food.section.created`,
`food.section.updated`, `food.section.reordered`, `food.section.archived`,
`food.item.created`, `food.item.updated`, `food.item.reordered`,
`food.item.archived`, `food.item.media_changed`,
`food.item.modifier_group_attached`, `food.item.modifier_group_detached`,
`food.item.modifier_groups_reordered`, `food.variant.created`,
`food.variant.updated`, `food.variant.reordered`, `food.variant.archived`,
`food.modifier_group.created`, `food.modifier_group.updated`,
`food.modifier_group.archived`, `food.modifier.created`,
`food.modifier.updated`, `food.modifier.reordered`, `food.modifier.archived`.

All at schema version 1, appended in the same transaction as the mutation that
produced them, so state and event commit together or neither does.

Section 9B extends the same outbox rule with schedule, availability, branch
override, prep-profile, logistics-profile, availability-source and price-change
events. Section 9C adds read-only internal discovery and does not emit a new
event stream; consumers read the committed state, price references and
catalogue revision through the HMAC-protected contract.

A **reorder emits one aggregate-level event** naming the parent and the new
order, not one event per moved row: a section reordered among twelve is one fact
about the menu, not twelve facts about sections. Archiving a section or a
modifier group likewise emits one event carrying what it cascaded to, rather
than one per affected child.

No event carries a price the reader could mistake for money movement, no
customer reference and no media bytes — the media event carries the opaque
reference list and nothing else.

As with Sections 5-8, no consumer exists yet, so these accumulate durably as
`PENDING` rather than being silently dropped.

## Section 12 Shop inventory events

Section 12A catalogue mutations and Section 12B inventory mutations append
their audit evidence and outbox envelope in the same PostgreSQL transaction as
state. Inventory event families cover stock-basis configuration, receipt
posting/reversal, reservation transitions, movements/adjustments, quarantine,
transfer, stocktake, policy, fulfilment measurement, reorder rules, alert
lifecycle, bulk completion and backorder allocation. Expiry reconciliation
emits the corresponding movement/event only when an unencumbered lot is
actually expired from stock.

Events carry stable aggregate/source references, branch scope, version and
bounded operational evidence. They do not carry customer identity, payment
authority, an Order, delivery promise or media bytes. Discovery is read-only
and emits no event merely because a consumer reads a projection. No external
consumer was fabricated; envelopes remain in the existing durable outbox until
a real destination is configured.
