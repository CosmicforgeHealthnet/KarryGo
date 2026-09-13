# Sections 44–45 — Event Catalogue and Durable Workers

## Closure result

Sections 44 and 45 reuse the existing Vendor event, outbox, inbox, job and
domain-row worker systems. They introduce no second event bus, state machine,
route or migration.

The machine artifacts are `SECTION44_EVENT_PARITY.json` and
`SECTION45_WORKER_CATALOGUE.json`. There are 39 canonical event contracts: 33
active, four dependency-gated and two future-disabled. There are 27 worker
requirements: 17 active mappings, seven dependency-gated, two not applicable
because the accepted implementation is event/read-time driven, and one
future-disabled.

ROUTES ADDED: 0  
ROUTES CHANGED: 0  
ROUTES REMOVED: 0  
MIGRATIONS ADDED: 0  
HISTORICAL MIGRATIONS CHANGED: 0  
FRONTEND CHANGED: NO

## Infrastructure audit

| Capability | Existing implementation | Result |
|---|---|---|
| Envelope | Opaque event ID, type, schema version, aggregate/version, tenant, correlation, causation, UTC occurrence and producer | PASS |
| Publication | `DomainPublisher.Publish` appends through the mutation's `pgx.Tx` | PASS; quote lock, ParentOrder and Wallet-confirmed payment gaps closed |
| Outbox | Bounded `SKIP LOCKED` claim, owner lease, expired-lease recovery, exponential retry, dead letter, replay and low-cardinality backlog/dead-letter age snapshot | PASS |
| Inbox | Source/event uniqueness, fail-closed catalogue version gate, type/fingerprint conflict detection and transactional claim | PASS; unsupported/future versions and late-fingerprint edge case closed |
| Worker coordination | `vendor_job_controls`, renewable heartbeat, bounded attempts/backoff, dead letter and replay | PASS; common secret redactor and low-cardinality health snapshot applied |
| Domain-row workers | Purpose-built queues/generations with leases, version fences and atomic promotion | PASS |
| Unknown outcome | Stable operation ID followed by authoritative read-back | PASS where integrated; otherwise gated |
| Delivery semantics | Effectively-once through durable identity, dedupe and state guards | PASS; no distributed exactly-once claim |

Payloads contain opaque references, versions, controlled codes and bounded
commercial facts. They exclude customer/receiver contact details, full
addresses, OTP/QR material, tokens, PAN/CVV, credentials, HMAC secrets, private
keys and secret-bearing media URLs. Outbox and persisted worker failures use the
same bounded redactor.

## Section 44 event closure matrix

`MAPPED` means an accepted repository-native event expresses the fact; it does
not create a duplicate canonical event.

| Event | Producer | Before | After | Consumers | Evidence |
|---|---|---|---|---|---|
| `vendor.business.activated` | Vendor | ACTIVE | ACTIVE | gated admin/analytics/notification | exact outbox emitter |
| `vendor.branch.status_changed` | Vendor | ACTIVE | ACTIVE | gated search/analytics | exact outbox emitter |
| `vendor.catalog.item_changed` | Vendor | domain-specific | ACTIVE/MAPPED | gated search/analytics | Food/Shop/Pastry aliases |
| `vendor.inventory.stock_changed` | Vendor Inventory | domain-specific | ACTIVE/MAPPED | gated search/analytics | inventory aliases |
| `commerce.checkout.paid` | Vendor after Wallet confirmation | MISSING | ACTIVE | gated analytics | emitted only on `HELD` transition |
| `commerce.order.created` | Vendor ParentOrder | domain record only | ACTIVE | gated analytics | transactional ParentOrder outbox append |
| `vendor.order.accepted` | Vendor | mature alias | ACTIVE/MAPPED | gated notification/analytics | VendorOrder transition event |
| `vendor.order.rejected` | Vendor | mature alias | ACTIVE/MAPPED | gated notification/analytics | VendorOrder transition event |
| `vendor.order.ready` | Vendor | mature aliases | ACTIVE/MAPPED | gated notification/analytics | ready/ready-for-pickup aliases |
| `commerce.fulfilment.plan_created` | Vendor | mature alias | ACTIVE/MAPPED | gated notification | plan activation v1 |
| `commerce.fulfilment.plan_replanned` | Vendor | versioned alias | ACTIVE/MAPPED | gated notification | later plan activation version |
| `commerce.mission.requested` | Vendor orchestration | mature alias | ACTIVE/MAPPED | none claimed | mission reference transaction |
| `delivery.provider_assigned` | Logistics owner | inbound vocabulary | ACTIVE/MAPPED | Vendor projection | assignment aliases + inbox |
| `delivery.pickup.verified` | Custody owner | inbound vocabulary | ACTIVE/MAPPED | Vendor projection | custody aliases + inbox |
| `delivery.parcel.picked_up` | Custody owner | inbound vocabulary | ACTIVE/MAPPED | Vendor projection | parcel alias + inbox |
| `delivery.receiver.verified` | Custody owner | inbound vocabulary | ACTIVE/MAPPED | Vendor projection | receiver alias + inbox |
| `delivery.completed` | Logistics owner | inbound vocabulary | ACTIVE/MAPPED | Vendor projection | terminal aliases + inbox |
| `delivery.failed` | Logistics owner | inbound vocabulary | ACTIVE/MAPPED | Vendor projection | failure aliases + inbox |
| `delivery.returned` | Custody owner | inbound vocabulary | ACTIVE/MAPPED | Vendor projection | return aliases + inbox |
| `commerce.refund.completed` | Payment Wallet | mature aliases | ACTIVE/MAPPED | Vendor projection | Wallet refund status + order projection |
| `commerce.vendor_settlement.completed` | Payment Wallet | mature alias | ACTIVE/MAPPED | Vendor projection | Wallet settlement status; no Vendor money emitter |
| `commerce.quote.locked` | Vendor | domain record only | ACTIVE | none claimed | transactional approval outbox append |
| `commerce.fulfilment.option_selected` | Vendor | plan activation | ACTIVE/MAPPED | none claimed | selected plan binding |
| `commerce.allocation.created` | Vendor | allocation transaction | ACTIVE/MAPPED | none claimed | Parent/Vendor order events |
| `commerce.substitution.proposed` | Vendor | mature alias | ACTIVE/MAPPED | gated notification | proposal event |
| `commerce.substitution.resolved` | Vendor | mature aliases | ACTIVE/MAPPED | gated notification | approved/rejected aliases |
| `commerce.variable_weight.finalized` | Vendor | mature alias | ACTIVE/MAPPED | none claimed | measurement finalization event |
| `commerce.optimization.proposal_created` | Vendor | gated | DEPENDENCY_GATED | optimizer gate | no active emitter claimed |
| `commerce.recurring_basket.generated` | Vendor | occurrence alias | ACTIVE/MAPPED | none claimed | one canonical entry + uniqueness |
| `commerce.rfq.quote_submitted` | Vendor | ACTIVE | ACTIVE | gated analytics | exact emitter |
| `commerce.rfq.quote_revised` | Vendor | revision aliases | ACTIVE/MAPPED | gated analytics | counter/quote aliases |
| `commerce.rfq.quote_accepted` | Vendor | mature alias | ACTIVE/MAPPED | gated analytics | RFQ accepted alias |
| `vendor.verification.badge_changed` | Vendor after Verification projection | mature alias | ACTIVE/MAPPED | none claimed | versioned authority projection |
| `vendor.reliability.updated` | Vendor projection | gated | DEPENDENCY_GATED | intelligence gate | no active emitter claimed |
| `vendor.intelligence.snapshot_updated` | Vendor projection | gated | DEPENDENCY_GATED | intelligence gate | no active emitter claimed |
| `commerce.budget_basket.proposal_created` | Vendor | gated | DEPENDENCY_GATED | optimizer gate | no active emitter claimed |
| `commerce.recurring_basket.approved` | Vendor | schedule alias | ACTIVE/MAPPED | none claimed | durable approval alias |
| `commerce.order_guarantee.evaluated` | Future | FUTURE | FUTURE_DISABLED | none | no emitter/table/route |
| `commerce.order_guarantee.compensation_issued` | Future/Wallet | FUTURE | FUTURE_DISABLED | none | no emitter or Wallet call |

## Section 45 worker closure matrix

| Worker | Before | After | Durable | Recovery | Evidence |
|---|---|---|---|---|---|
| Checkout reservation expiry | PASS | ACTIVE | coordinator leases | retry/dead letter/replay | checkout/inventory recovery |
| Menu/product scheduler | read-time | NOT_APPLICABLE | N/A | deterministic re-read | versioned schedule evaluation |
| Stock alerts | PASS | ACTIVE | coordinator leases | convergent rerun | inventory alert workers |
| Inventory expiry | PASS | ACTIVE | row locks + coordinator | due-row retry | lot/reservation expiry |
| Scheduled order activation | PASS | ACTIVE | unique occurrence rows | stale-row recovery | recurring/production materializers |
| Capacity release | PASS | ACTIVE | expiry/state guards | idempotent rerun | pastry/production expiry |
| RFQ expiry | PASS | ACTIVE | durable RFQ rows | state-guarded retry | RFQ lifecycle tests |
| Order acceptance timeout | no canonical policy | DEPENDENCY_GATED | disabled | no fabricated rejection | catalogue gate |
| Substitution timeout | no universal fallback | DEPENDENCY_GATED | disabled | stored policy retained | catalogue gate |
| Fulfilment planner worker | synchronous | DEPENDENCY_GATED | disabled | synchronous idempotency | no duplicate queue |
| Platform mission reconciliation | authority-specific | DEPENDENCY_GATED | disabled | stable IDs retained | authority adapters |
| Handover reconciliation | immediate read-back | DEPENDENCY_GATED | disabled | canonical read-back | no credential ownership |
| Settlement reconciliation | PASS | ACTIVE | coordinator leases | same-operation Wallet read-back | payment recovery |
| CRM segment refresh | PASS | ACTIVE | domain leases/heartbeat | dead letter/replay | atomic generation swap |
| Reliability aggregation | PASS | ACTIVE | domain generation rows | retry/failed generation | versioned intelligence |
| Search index update | PASS | ACTIVE | projection job leases | dead letter/replay | checkpointed activation |
| Quote/price-lock expiry | PASS | ACTIVE | coordinator/domain rows | idempotent expiry | history retained |
| Payment reconciliation | PASS | ACTIVE | coordinator leases | same-operation read-back | UNKNOWN recovery tests |
| Hybrid delivery fallback | command only | DEPENDENCY_GATED | disabled | custody freeze retained | no automatic SLA policy |
| Confidence ETA updater | event-driven | NOT_APPLICABLE | N/A | source-version replay guard | ETA history projection |
| Verification expiry/recheck | PASS | ACTIVE | coordinator/domain rows | authority read-back | expiry/reconciliation tests |
| Variable-weight timeout | no canonical policy | DEPENDENCY_GATED | disabled | financial cap retained | catalogue gate |
| Recurring basket generator | PASS | ACTIVE | coordinator + unique row | replay-safe occurrence | idempotency tests |
| Vendor intelligence aggregation | PASS | ACTIVE | domain generation rows | retry/failed generation | privacy-safe snapshots |
| Search projection rebuild | PASS | ACTIVE | checkpointed leases | dead letter/replay | beside-live atomic activation |
| Budget/optimization expiry | PASS | ACTIVE | coordinator leases | stale acceptance rejection | proposal expiry tests |
| Order Guarantee evaluator | FUTURE | FUTURE_DISABLED | none | none | no worker/table/route/Wallet call |

## Authority boundaries

Payment Wallet alone owns holds, capture, settlement, refund and ledger truth.
`commerce.checkout.paid` is emitted only after the existing Wallet result has
durably produced `PaymentHeld`; `UNKNOWN` never produces it. Refund and
settlement remain Wallet-authoritative projections.

Dispatch, Hauling, Taxi Goods and VendorMission retain execution, assignment,
custody and handover. Vendor consumes/projects those results. OTP/QR values do
not enter the event catalogue. Media, Customer, Verification and Notification
authorities are unchanged. Search and analytics remain projections.

## Certification contract

The validator enforces unique names, positive versions, known producers,
active Vendor emitter mappings, future-disabled non-emission and forbidden
payload-field names. Artifact parity compares the JSON directly with the Go
catalogues and requires all four event gap arrays to be empty. Existing live
PostgreSQL suites cover atomic append/rollback, inbox dedupe, claim exclusivity,
lease recovery, heartbeat, bounded retry, dead letter and replay.

Certification on 2026-09-10 passed the standard test/vet/build gates, changed
Go formatting, deterministic catalogue validation and the unchanged Section 43
OpenAPI regression (867 operations, zero quality changes). The full isolated
runner completed 65/66 before the Section 13 scale fixture exceeded its
10-minute seed context; Section 13 then passed alone on a fresh isolated
database, as did the final changed `events` and `cmd` packages. Effective
isolated coverage is 66/66. Migration replay was not applicable because no
migration changed.

Final command results are recorded in `IMPLEMENTATION_STATUS.md`.
