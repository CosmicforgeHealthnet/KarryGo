# Vendor Service Documentation

## Section 8I — final CRM certification

Section 8I is the closing certification pass over Sections 5–8H. It adds no CRM
feature, no public route and no migration. What it adds is the evidence that the
whole thing works as one backend rather than as eight sections that each pass
their own tests.

**Two production defects were found and fixed.**

The first was an authority-surface defect. `POST /internal/staff/provision-owner`
— the route that creates the first Owner of a vendor business — was wired under
the public vendor base at `/api/v1/vendor/internal/...`, while its own OpenAPI
specification, the Section 5 contract package and the route register had always
described it at the engine root. Section 7 established the rule that internal
authority ingress must not sit behind the path the gateway forwards; this route
was missed in that pass. The wiring now matches the documentation.

The second was a transaction-scope defect in Section 8F. The notification
handoff ran a synchronous HMAC call to `notification-service` while holding
three row locks — the delivery, the run, and the customer profile. That last one
is the shared privacy fence, so a slow or hanging remote call blocked redaction
for that customer and pinned a database connection for the round trip. The
handoff now reserves the delivery, commits, calls, and records the outcome in a
second transaction. Contact history is deliberately still written under the
profile lock, because the frequency cap counts those rows — moving it out broke
the accepted concurrency test, which is exactly what that test exists for.

**One global contract guard was added.** `internal/crmcontract` builds the real
production router from the same `Register*` helpers `cmd/main.go` calls and holds
it against every OpenAPI specification and the route register in both
directions. The per-section contract packages each pinned their own surface;
nothing pinned the whole of it, which is the gap that lets a live undocumented
route hide. The accepted count — **194** — is now derived from code rather than
counted from prose.

**Cross-module certification.** `section8i_certification_test.go` asks one
question of the entire CRM: after a redaction, can any module still act on this
person? Segmentation, intelligence, corporate contacts and exports each answer
independently, and three anti-resurrection paths — a new tag, a rebuild, and a
late order fact arriving from the authority — are each proved unable to bring
the relationship back.

## Section 8H backend

Section 8H adds Vendor-owned CRM intelligence and secure exports: a versioned code-defined model registry, deterministic RFM V1 and Churn V1, score availability and freshness states, explainability, generation-safe materialization, dependency-scoped invalidation, a Customer 360 intelligence block, operational analytics, eight new Section 8B segment fields, and a step-up protected export pipeline over six closed datasets. Public surface `/api/v1/vendor/crm/intelligence` and `/api/v1/vendor/crm/exports` (10 operations, no internal routes). Migrations `000024`-`000025`. Contract: `openapi/section8h-crm-intelligence-exports.openapi.json`.

**No machine learning, and no probability.** Both models are heuristics computed in Go from commercial facts this service was given. There is no model artefact, no training data, no float anywhere in the schema and no generated prose. A churn risk of 70 ranks a relationship for follow-up; it is not a 70% chance of anything, and the OpenAPI contract has a test that fails if it ever starts claiming otherwise.

**A score is not permission.** A relationship can be the most valuable in the book and still be one this vendor may not contact. Consent, privacy, marketing suppression, frequency and quiet hours are decided by Sections 8A, 8E and 8F, are not consulted to produce a score, and are not weakened by one. A real-Postgres test drives a high-churn customer who denied marketing consent through the segment engine and proves they are not selected.

**Unavailable is not zero.** A relationship with no observed commercial facts gets a `NOT_AVAILABLE` score row with a reason code and every numeric column NULL — a database CHECK makes a row asserting otherwise unstorable. The segment fields compile score predicates so a missing score yields SQL NULL, which means `churn.risk_score GT 70` cannot be satisfied by absence and `rfm.composite_score LT 5` cannot be satisfied by a relationship nobody ever scored.

**A failed rebuild never empties a dashboard.** The new generation is built alongside the live one and swapped in one transaction that also re-certifies against privacy; a failure before the swap leaves the previous scores exactly as they were and the scope reports `STALE` with its last-good as-of intact.

**Exports release nothing until they are complete and still lawful.** Generation, privacy re-certification, private upload and promotion happen in that order; `READY` has a database CHECK requiring the artifact, its checksum, its size, a completion instant and an expiry; and access is decided live per request against a fresh step-up, returning a token that outlives neither the request nor the store's own short window. There is no URL column anywhere.

## Section 8G backend

Section 8G adds the Vendor-owned corporate customer CRM: corporate accounts with a vendor-local `CORP-` reference, a relationship lifecycle, business or branch-set scope, contacts that reference Section 8A customer relationships, memberships, the organization's own locations, cost-centre labels, an account manager, and corporate interactions and tasks that reuse the Section 8C engine. Public surface `/api/v1/vendor/crm/corporate-customers` (41 operations, no internal routes). Migrations `000022`-`000023`. Contract: `openapi/section8g-crm-corporate-customers.openapi.json`.

**Section 8G records who the vendor deals with; it sells nothing.** There is no corporate order, invoice, credit line, budget, contract, quotation, wallet, catalogue entitlement or approval anywhere in it — no route, no permission, no schema, no column and no state. A cost centre is a label with no budget, limit, currency or balance, and `payment-wallet-service` remains the only financial authority. Three separate tests (migration, OpenAPI contract, live router) reject a commerce or finance surface structurally rather than by convention.

**A contact is a reference, not an address book entry.** Corporate contacts point at Section 8A customer relationships and carry a non-identifying display label resolved at read time; no email address, phone number or postal address is duplicated here or returned by this section. The only address in Section 8G is the customer organization's own site record.

**Privacy and identity merge reach corporate contacts through the existing Section 8A port.** `corporateContactPrivacyHandler` registers on the same `PrivacyHandler` registry Sections 8B and 8C use, so redaction suppresses the link and deactivates memberships, and an identity merge moves contacts to the surviving relationship — resolving a collision by archiving the duplicate, moving its memberships and promoting the survivor when the source was primary. `ApplyPrivacy` was not modified.

**Corporate activity reuses Section 8C rather than duplicating it.** `crm_interactions` and `crm_tasks` gained a nullable corporate subject under a `CHECK` that makes a subject-less or double-subject row unstorable. A second task state machine would have been two places for `OPEN`/`IN_PROGRESS`/`COMPLETED`/`CANCELLED` to disagree; a second revision model would have been two definitions of immutable history. Every corporate activity route requires the Section 8C permission **and** `vendor.crm.corporate.read`.

## Section 8E backend

Section 8E adds the Vendor-owned CRM campaign intelligence and audience engine: campaign definitions, immutable campaign versions, segment-backed audience selection, privacy-safe audience preview, durable audience preparation, immutable audience snapshots, campaign targets, snapshot-time consent evidence per channel, deterministic experiment cohorts, and frequency and quiet-hour policy definition. Public surface `/api/v1/vendor/crm/campaigns` (10 operations, no internal routes). Migrations `000018`-`000019`. Contract: `openapi/section8e-crm-campaign-intelligence.openapi.json`.

**Section 8E prepares; Section 8F executes.** This section answers who a campaign should target and freezes that answer. It cannot answer how or when a message is delivered: there is no scheduling code, no send endpoint, no notification handoff, no template, no provider client and no destination column anywhere in it. `notification-service` remains the sole delivery authority and is unchanged.

**A prepared audience is not permission to contact anybody.** Consent is evaluated twice: once here, recorded as historical evidence on each target channel, and again by Section 8F immediately before any delivery handoff, against current consent. Snapshot evidence is never rewritten when a customer later changes their mind — they are protected by a mandatory send-time recheck, not by history being edited. `send_time_recheck_required` is pinned true by a database check, so a row that claimed otherwise cannot be stored.

**Consent fails closed, and there is nothing to fail open into.** Only an explicit, fresh, unexpired ALLOW produces an eligible channel. DENY, UNKNOWN, a missing projection, a lapsed ALLOW and a stale projection are all ineligible, and with no consent authority integrated at all — which is the platform's state today — every channel is ineligible with reason `CONSENT_SOURCE_UNAVAILABLE`. Segment membership is not consent, and a control-cohort assignment is not an exemption.

**Suppressed relationships get no row at all.** A privacy- or service-recovery-suppressed customer appears only in the snapshot's aggregate counts, never in `crm_campaign_targets`. That is deliberately stronger than storing them with a suppressed state: a customer who must not be contacted is absent from the only table Section 8F will read, so no future query mistake can reach them.

**Audience comes only from Section 8B.** There is no second rule language, no raw SQL parameter and no customer-list upload, so every branch, privacy and consent wall the segment engine already built applies here without being rebuilt — and, more importantly, without being subtly rebuilt differently. A preparation pins the exact segment generation it reads, which is what stops two segment rebuilds being mixed into one audience.

## Section 8D backend

Section 8D adds the Vendor-owned CRM loyalty foundation: a per-business programme configuration, lazily created customer loyalty accounts, an append-only points ledger, point lots with per-issuance expiry, recorded lot consumption, a durable expiry sweeper, bounded manual adjustments, and full reversal. Public surface `/api/v1/vendor/crm/loyalty` and `/crm/customers/{profileId}/loyalty` (6 operations, no internal routes). Migrations `000016`-`000017`. Contract: `openapi/section8d-crm-loyalty.openapi.json`.

**Points are not money.** This is a points ledger, not a second financial authority. There is no currency column, no minor-unit amount, no wallet reference and no conversion path anywhere in Section 8D; `payment-wallet-service` remains the sole owner of money, and Section 8D emits no `payment.*`, `wallet.*`, `refund.*`, `settlement.*` or `payout.*` event. A schema test and a contract test both reject a monetary-looking column or field structurally.

**No earn or redeem endpoint.** The earning and redemption engines are real and tested, but they are service-internal primitives (`CreditPoints`, `DebitPoints`) waiting for a trusted producer. Publishing either as a vendor-callable endpoint would let any authorised staff member name their own points total. The only vendor-callable movements are a manual adjustment — bounded by the programme's own limit, gated behind a dedicated high-risk permission, and requiring an audited reason — and a full reversal. There is deliberately no earning formula anywhere: the Vendor Order domain that would define what an order is worth does not exist yet, so nothing here converts money to points.

**Expiry is stamped once and never rewritten.** Each credit issues a point lot carrying the expiry policy as it stood at issue time. Changing the programme's window later affects future credits only, and an identity merge carries each lot's original expiry across unchanged — a merge never buys a customer a fresh window, and a policy change never retroactively voids points they already hold.

**A stalled sweeper cannot make dead points spendable.** Available balance always excludes lots past their expiry, whether or not the background pass has settled them, and every balance-changing command settles due expiry before it acts. The sweeper needs no queue table and no lease: a lot with points left and a past expiry *is* the durable job, and it stays outstanding until a committed transaction settles it. That gives crash recovery and multi-worker safety from the schema rather than from bookkeeping, and turning the feature off defers the work instead of discarding it.

**The ledger is append-only and replayable.** No posted entry is ever edited or deleted; corrections are compensating `REVERSAL` entries that reference their original, and exactly one can exist per entry (a unique index, not a read-then-write check). Each entry carries a per-account monotonic sequence and the balance that resulted, so history replays exactly, and a database constraint holds `current_points = lifetime_credited - lifetime_debited`. Reversing a credit refuses when its lot has been touched, rather than silently confiscating points from an unrelated issuance; reversing a debit restores the exact points to the exact lots that funded it, then immediately expires anything already past its window.

## Section 8C backend

Section 8C adds Vendor-owned CRM interactions and staff follow-up tasks. Interactions use controlled type, direction and outcome vocabularies, plain-text notes, explicit business/branch visibility, optimistic correction, and append-only revisions. Tasks use a four-state lifecycle, priority, eligible staff assignment, UTC due dates, derived overdue state, optimistic concurrency, and immutable history. Both domains are tenant/branch scoped, use the Section 8A privacy and identity-merge fence, write audit and outbox records transactionally, and add bounded recent-activity blocks to the existing Customer 360 response.

The surface is 16 public operations and no internal operations under `/api/v1/vendor/crm`; migrations are `000014`-`000015`, and the machine-readable contract is `openapi/section8c-crm-interactions-tasks.openapi.json`. This is relationship workflow only: it does not create support cases, send notifications, store canonical customer PII, or implement Sections 8D and later.

## Section 8B backend

Section 8B adds vendor-defined customer tags and the segmentation engine: a safe JSON rule language over a closed field registry, static and dynamic segments, immutable definition versions, privacy-safe preview, and durable materialization with an atomic generation swap. Public surface `/api/v1/vendor/crm/tags`, `/crm/segments` and `/crm/segment-fields` (21 operations, no internal routes). Migrations `000012`-`000013`. Contract: `openapi/section8b-crm-tags-segmentation.openapi.json`.

**Audiences, not permission.** Section 8B builds audiences and sends nothing. There is no route, permission or worker here that contacts a customer, and a contract test rejects a campaign- or send-shaped route structurally. Segment membership is never proof of consent: whatever section eventually sends must re-check consent and privacy at send time.

**No SQL reaches the database from a vendor.** A rule can only name a field published by `GET /crm/segment-fields`. Field keys, operators and values are validated against a closed registry and reach Postgres only as bind parameters; the SQL for each field is hand-written Go in one file. There is no free-text filter, no user-chosen sort column and no escape hatch anywhere in the surface.

**Unknown is not zero.** A condition over data this deployment has never observed is undecidable, not false: it selects nobody, and a negation of it selects nobody either. Deployment-level unavailability is returned in `unavailable_fields` with a warning. Row-level commercial `NOT_AVAILABLE` is a safe no-match with no fabricated number or timestamp; it does not currently emit a per-row `FIELD_NOT_AVAILABLE` reason. That is what stops "we have no order data for this customer" from being read as "this customer spent nothing".

**A failed rebuild never empties an audience.** A new generation is built alongside the live one and swapped in a single transaction; a failure before the swap leaves the previous membership intact and the segment honestly reports `STALE`. Final certification locks every profile represented by the new generation, re-evaluates the complete rule, and updates the generation pointer and count before releasing those locks. Privacy and identity merge use the same profile locks, so whichever commits second must observe or immediately remove the first transaction's result. The worker renews its lease, retries with backoff, dead-letters permanent failures, and exposes a code-level replay hook. Input changes mark and durably enqueue only dependent segments in the same transaction.

**Branch scope reaches into the rule and Customer 360.** A branch-scoped segment reasons over that branch's slice of every commercial fact rather than the business total, and every count — preview, tag membership, segment membership — is branch-filtered. Customer 360 returns only tags and segment classifications owned by the caller's permitted branches; business-wide and other-branch classifications are withheld because the label itself is a disclosure.

## Section 8A backend

Section 8A adds the CRM foundation: vendor-local customer relationship profiles, a privacy-safe Customer 360, projected marketing consent, projected privacy state with a working redaction engine, an authoritative identity-link/merge path, and the commercial-projection foundation. Public surface `/api/v1/vendor/crm/customers` (six operations); root-level authority ingress `POST /internal/vendor/v1/crm/events`. Migrations `000010`-`000011`. Contract: `openapi/section8a-crm-foundation.openapi.json`.

**Authority boundary, which is the whole point of this section.** Vendor Service is not a customer master. customer-service owns canonical identity, and (once they exist) consent and privacy decisions. CRM stores opaque subject references, never canonical email, phone, address or date of birth, and never a hash of a customer reference used as a substitute identity. There is no vendor-facing route that creates a customer, sets consent, sets privacy state or merges identities — a contract test enforces that structurally.

**Fail-closed marketing.** An absent, unknown, expired or stale consent decision is never permission. Privacy outranks consent: a redacted, restricted or marketing-suppressed subject is ineligible regardless of any stored ALLOW. No permission exists — at any role, including Owner — that can override either.

**Honest projections.** With no commerce authority integrated, commercial data reports `NOT_AVAILABLE` rather than a spend of zero. Tags/segments and interactions/tasks report `AVAILABLE` only when their respective feature flags are enabled; Sections 8D-8H remain `NOT_IMPLEMENTED`. A fresh production deployment legitimately holds zero CRM profiles.

**Dormant by design.** Consent, privacy, identity-link and order-fact ingestion each sit behind their own flag and are all off, because no service publishes those events yet. The contracts, projections, dedupe, source-version rules, redaction engine and rebuild path are implemented and tested against fixtures; nothing fabricates a publisher.

## Section 7 backend

Section 7 adds onboarding, representative management, Media-owned document references, six-dimension verification projections, Wallet-owned masked bank onboarding, a data-driven checklist, automatic policy activation, signed callbacks, expiry handling, and durable reconciliation. The public surface is `/api/v1/vendor/onboarding`; the root-level internal callback is `/internal/vendor/v1/verification-events`.

**Section 7 contract correction:** VerificationStatus is exactly `NOT_STARTED`, `IN_PROGRESS`, `PENDING_REVIEW`, `VERIFIED`, `REJECTED`, `EXPIRED`, `SUSPENDED`; plain `PENDING` is invalid for verification. Unrelated outbox-delivery and reconciliation-operation states retain their own `PENDING` vocabulary. The callback was moved out of the public `/api/v1/vendor` router group after the route-composition audit found the doubled Vendor prefix accidental.

Vendor actors cannot set verification status. Vendor never stores uploaded bytes or returns full bank/recipient credentials. Bank replacement requires `vendor.bank_profile.manage` plus a single-use `finance.bank_change` step-up grant. Machine-readable routes are in `docs/openapi/section7-onboarding-verification.openapi.json`. Section 8 is not included.

This folder is the mandatory documentation control area for `services/vendor-service` (KarryGo Vendor Service and Commerce Logistics backend). It exists so every future implementation section leaves the codebase and its documentation consistent, per the source-of-truth implementation specification.

## Rule: a section is not complete until documentation is updated

After every implementation section:

1. Update [IMPLEMENTATION_STATUS.md](IMPLEMENTATION_STATUS.md).
2. Update [API_ROUTE_REGISTER.md](API_ROUTE_REGISTER.md) if routes changed.
3. Update [PLATFORM_SERVICE_CHANGE_REGISTER.md](PLATFORM_SERVICE_CHANGE_REGISTER.md) if another service changed (or needs to).
4. Update [FRONTEND_REQUIREMENTS.md](FRONTEND_REQUIREMENTS.md) for any frontend-facing backend behavior.
5. Update [UI_UX_REQUIREMENTS.md](UI_UX_REQUIREMENTS.md) for any new/changed user interaction.
6. Update `openapi/` where applicable.
7. Update service-specific technical docs/runbooks where applicable.

If a document has no impact from a given section, record `NO CHANGE — <reason>` in that document rather than leaving it silently untouched.

## Maintained documents

| Document | Purpose |
|---|---|
| [UI_UX_REQUIREMENTS.md](UI_UX_REQUIREMENTS.md) | Plain-language UI/UX requirements generated from backend/product implementation, for the future Vendor Flutter app team. No frontend code. |
| [FRONTEND_REQUIREMENTS.md](FRONTEND_REQUIREMENTS.md) | Plain-language backend-contract handoff (routes, auth, fields, validation, events) for the future frontend team. No frontend code. |
| [PLATFORM_SERVICE_CHANGE_REGISTER.md](PLATFORM_SERVICE_CHANGE_REGISTER.md) | Everything Vendor Service implementation requires outside `services/vendor-service` — config changes, migrations, routes, events on other services. |
| [API_ROUTE_REGISTER.md](API_ROUTE_REGISTER.md) | Human-readable master catalogue of every Vendor Commerce route across every affected service. `openapi/` is the machine-readable authority once real routes exist; this is the human-readable index. |
| [IMPLEMENTATION_STATUS.md](IMPLEMENTATION_STATUS.md) | Per-section implementation tracker: scope, status, code paths, migrations, routes, events, workers, tests, affected services, documentation updated, remaining gaps, acceptance result. |
| [architecture/](architecture/) | Technical backend architecture documents (service boundaries, domain ownership, module map, integrations, payment allocation, fulfilment model, OTP/QR dependency status, event ownership, security model, worker/recovery model, feature flags, migration ownership). Read these before implementing any section. |
| [architecture/00_REPOSITORY_AUDIT_BASELINE.md](architecture/00_REPOSITORY_AUDIT_BASELINE.md) | The factual repository audit this whole documentation set is built on — read first. |
| `openapi/` | Machine-readable API specs for implemented Sections 5 through 8E; bidirectional route-parity tests prevent undocumented or phantom routes. |

## Section 6 delivery

Vendor business, brand, capability, and branch management is implemented in `internal/features/vendors`, migrations `000006`-`000007`, and `openapi/section6-vendor-business.openapi.json`. Section 7 onboarding builds on that graph; Section 8A projects CRM relationships onto it; Section 8B adds tags and segmentation; Section 8C adds interactions and tasks; Section 8D adds the loyalty points ledger; Section 8E adds campaign intelligence and the audience engine; Section 8F adds campaign execution, Section 8G corporate CRM, and Section 8H intelligence and secure exports, all certified in Section 8I. Catalogues and commerce remain out of scope. The UI handoff is documentation-only; no frontend source was changed.

## Reading order for a new implementation section

1. `architecture/00_REPOSITORY_AUDIT_BASELINE.md` — what actually exists today, verified.
2. `architecture/01_SERVICE_BOUNDARIES.md` and `02_DOMAIN_OWNERSHIP.md` — what you may and may not own.
3. `architecture/03_VENDOR_SERVICE_MODULE_MAP.md` — where your code goes.
4. Whichever of `04`–`12` is relevant to the section (integrations, payment, fulfilment, OTP/QR, events, security, workers, flags, migrations).
5. `IMPLEMENTATION_STATUS.md` — what has already shipped, so you don't redo or contradict it.
6. `PLATFORM_SERVICE_CHANGE_REGISTER.md` — check whether your section depends on an already-tracked external change.

Section 8F campaign execution is implemented in `internal/features/crm/campaign_execution.go` and `campaign_execution_http.go`. Its Notification boundary is documented in `NOTIFICATION_CONTRACT_8F.md`, and its locking/state model in `architecture/13_CAMPAIGN_EXECUTION_MODEL.md`. OpenAPI lives at `docs/openapi/section8f-campaign-execution.openapi.json`.

Section 8G corporate customer CRM is implemented in `internal/features/crm/corporate.go`, `corporate_sites.go`, `corporate_contacts.go`, `corporate_activity.go`, `corporate_privacy.go` and `corporate_http.go`. OpenAPI lives at `docs/openapi/section8g-crm-corporate-customers.openapi.json`, pinned by `internal/section8gcontract`.

Section 8H CRM intelligence and secure exports are implemented in `internal/features/crm/intelligence*.go`, `exports*.go` and `export_datasets.go`, with the private artifact adapter in `internal/integrations/media/artifacts.go`. OpenAPI lives at `docs/openapi/section8h-crm-intelligence-exports.openapi.json`, pinned by `internal/section8hcontract`.

## Section 8 completion

The original Section 8 CRM requirements are implemented through the final
completion slice. Customer 360, affinity, service recovery, value bands,
relationship signals, corporate projections and bounded follow-up triggers are
active behind their existing feature flags and fail closed when producer facts
are unavailable. Loyalty tiers remain derived from the immutable ledger.

Migration tip `000028`. Vendor surface **196 routes**. Segment registry **40**
fields. No external service was changed by the final-completion slice.

## Section 9A — food catalogue foundation

Section 9A implements the Restaurant/Fast-Food catalogue in
`internal/features/food` (`models.go`, `service.go`, `menus.go`, `items.go`,
`modifiers.go`, `http.go`), migrations `000029`-`000030`, and
`openapi/section9a-food-catalogue.openapi.json`, pinned by
`internal/section9acontract`.

### The hierarchy

```
FOOD MENU            one business + one branch + one food capability
  └─ MENU SECTION    ordered groups inside a menu
       └─ FOOD ITEM  ordered items, priced in integer minor units
            ├─ VARIANTS               size/portion, price override or inherit
            └─ MODIFIER GROUPS  ──►  MODIFIERS   (shared, attached, ordered)
```

`food_menus`, `food_menu_sections`, `food_items`, `food_item_media`,
`food_item_variants`, `food_modifier_groups`, `food_modifiers` and
`food_item_modifier_groups`.

### Capability boundary

Only `RESTAURANT` and `FAST_FOOD`. `CLOUD_KITCHEN`, `PASTRY_BAKERY`,
`SHOP_GROCERY`, `MARKET` and `WHOLESALE` are real KarryGo capabilities but not
this section's; the schema takes new values by additive CHECK change so a later
section widens this without a destructive migration. A branch must hold the
capability before a menu exists for it.

### Tenant and branch ownership

Every row carries `business_id`, and every parent link is a composite foreign
key — `(branch_id, business_id)`, `(menu_id, business_id)`,
`(section_id, menu_id)`, `(item_id, business_id)`,
`(modifier_group_id, business_id)`. A section addressed through the wrong menu,
a variant mutated through the wrong item and a group borrowed from another
business are therefore unstorable rather than merely refused. Branch scope is
resolved per request from the Section 5 assignment and applied in SQL, so a
branch-scoped caller never receives another branch's rows and cannot learn that
they exist.

### Money

Integer minor units everywhere: `base_price_minor`, `price_override_minor`,
`price_delta_minor`. No floating-point column, no per-item currency. The
business's ISO-4217 currency (Section 6) is resolved for the response, never
copied onto a catalogue row, so there is exactly one currency truth. A modifier
delta may be zero but never negative — a discount is a promotion, and promotions
are not this domain's authority.

### Lifecycle and archive

Menus and items share one closed lifecycle: `DRAFT → ACTIVE ⇄ PAUSED`, with
`ARCHIVED` terminal. Sections, variants, groups and modifiers are `ACTIVE` or
`ARCHIVED`. A DRAFT catalogue may be prepared while the business is still
onboarding; publishing enforces the existing Section 6/7 business, branch and
capability state rather than creating a second onboarding authority.

Nothing is hard-deleted except the item/modifier-group relationship row, which
carries no historical authority. Archiving a section archives its items, and
archiving an item archives its variants, in the same transaction — an entity
left outside the catalogue but still reachable by id is exactly what archive
semantics exist to prevent.

`PAUSED` here is administrative. It is **not** Section 9B's sold-out or
availability state, and the lifecycle vocabulary cannot express one.

### Variants and modifiers

A variant's `price_override_minor` is null to inherit the item's base price and
non-null to override it; `inherits_base_price` is derived for the response so
the two cannot disagree. A modifier group's `min_select > 0` **is** what
"required" means — there is no separate boolean, because two columns encoding
one fact eventually disagree.

Modifier groups are **business-level reusable definitions**, not per-branch
copies: "Add extras" is written once and attached to as many items as the
business likes. Editing a shared definition therefore needs a business-wide
grant; a branch-scoped grant may still read groups and attach, detach and
reorder them on its own branch's items.

### Ordering

Sections, items, variants, modifiers and item attachments each carry a
`position`. Create, archive and reorder all end in one `resequence` routine, so
live rows are always contiguous from zero with archived rows behind them. Each
collection has a deferred unique `(parent, position)` constraint, which is what
lets a single atomic statement permute positions while the committed state stays
provably unique. A reorder must name every live id exactly once; duplicates,
foreign ids and partial lists are refused with the same message, so a reorder
cannot become an oracle for what exists elsewhere.

### Media

Media File Service owns bytes, MIME, safety, retention and access. This service
stores an opaque reference, its display order and which one is the cover, and
proves each reference against that service's existing private ownership API —
the same contract Section 7 uses for onboarding documents — before storing it.
With no media authority configured the write is refused rather than storing an
unverified reference. There is no upload route in this section.

### Dietary and allergen metadata

Bounded, normalised, de-duplicated codes matching `^[A-Z0-9_]{2,40}$`, capped at
16 dietary and 24 allergen entries. This is declarative vendor-supplied
catalogue information: the service computes no nutrition, certifies nothing, and
makes no medical claim. The narrow code shape is deliberate — it expresses any
vendor vocabulary while making it impossible to put markup, a query or a URL
into what is displayed as a dietary claim.

### 9A / 9B / 9C boundary

| Question | Owner |
|---|---|
| What does this branch sell? | **9A (this section)** |
| Can it be sold now, at this branch, at this time, for what effective price, with what preparation/logistics profile? | **9B (implemented)** |
| How do customers and other services read it? | 9C — internal discovery contract implemented; public frontend remains deferred |

Feature flags: `COMMERCE_FOOD_ENABLED` registers the surface at all;
`VENDOR_FOOD_CATALOGUE_WRITES_ENABLED` gates mutations;
`COMMERCE_GLOBAL_KILL_SWITCH` overrides both and can only ever withhold a write.
With the feature off no food route is registered, so a disabled catalogue has no
attack surface rather than routes that answer 503 and confirm it exists.

Migration tip `000032` (no 9C migration). Vendor surface **262 routes** (22 new
public in 9B, 5 new internal in 9C). No external service was changed by Section
9C.

Sections 10/11 now have the shared production foundation at migration
`000033`, IAM at `000034`, Cloud Kitchen domain tables at `000035`, Pastry/Bakery
tables at `000036`, slot-template fields at `000037`, pastry closure fields at
`000038`, and worker/change-decision durability at `000039`. The authenticated
management surface is 362 routes, including
capacity/slot templates/jobs, Cloud Kitchen packages and recurring plans, and
Pastry products/options/specifications/quotes. Capacity and quote
acceptance remain Vendor operational truth; no Order, payment, Wallet,
Inventory, Dispatch, Taxi or Hauling authority is fabricated.

## Section 12A — Shop / Grocery / Market / Wholesale catalogue

Section 12A is implemented as authenticated Vendor management under `/shop`.
It adds five branch operating models, hierarchical categories, product masters,
variants/SKUs, explicit packaging relationships and exact barcode/identifier
lookup. Branch listings support retail and wholesale together, selected sell
units, MOQ/order multiples, deterministic price precedence, non-overlapping
quantity bands and append-only integer-minor-unit price history.

Nigerian trade measurement is normalized rather than stored as a unit string.
Platform definitions cover standard gram/kilogram, millilitre/litre and count
units plus local/container/package concepts including Paint, Derica, Mudu,
Congo, Tiya, bowls, baskets, bunches, tubers, bags, cartons and loads. Local
containers have no global weight. Product/variant and optional branch
calibrations carry an explicitly approximate amount and tolerance; exact and
calibrated conversions cannot be confused. Vendor aliases on platform measures
remain tenant-scoped. Branch calibrations attach only to that branch's listing
sell unit and never become business-wide conversion truth. Sell units support bounded rational
fractions such as half Paint and half basket, variable weight/count and
price-defined portions.

The Part A migration tip is `000043_vendor_shop_permissions`; its fully enabled
Vendor router baseline is **443 routes** (58 Section 12A public management
routes added to the 385-route pre-12A surface). `COMMERCE_SHOP_ENABLED`
registers the surface and `VENDOR_SHOP_CATALOGUE_WRITES_ENABLED` gates
mutation; the global commerce kill switch overrides writes.

## Section 12B — Shop inventory, availability and discovery

Section 12B adds the authoritative Vendor-owned inventory ledger beneath the
Section 12A catalogue: stock bases and balances, receipts, lots/batches/expiry,
FEFO/FIFO reservations, adjustments, quarantine, branch transfers, stocktakes,
preorder/backorder policy, variable fulfilment evidence, reorder rules, alerts
and leased bulk operations. Availability and internal Shop discovery resolve
effective retail/wholesale pricing, quantity rules, fresh and handling metadata,
sellability and matching catalogue/inventory revisions in one repeatable-read
snapshot.

The migration tip is `000046_vendor_shop_inventory_permissions`. The fully
enabled router has **496 routes**: 42 Section 12B authenticated operations and
11 HMAC-v2 internal operations added to the 443-route Part A baseline. The
machine-readable contracts are
`docs/openapi/section12b-shop-inventory.openapi.json` and
`docs/openapi/section12-shop-discovery.openapi.json`.

Vendor Service still creates no customer Order, cart, checkout, payment/Wallet
movement, supplier/purchase order, or Dispatch/Taxi/Hauling execution. An
inventory reservation is operational stock authority only.
