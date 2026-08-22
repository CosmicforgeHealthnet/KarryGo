# Vendor Service Documentation

## Section 18 — Vendor Order Lifecycle

Section 18 extends the Section 17 ParentOrder/VendorOrder authority. Food and
Shop orders use explicit operational states; purchased line snapshots remain
immutable while Shop picking outcomes and substitution proposals are mutable,
versioned evidence. Customer decisions are customer-identity scoped. Rejection
refunds remain isolated per VendorOrder, and Dispatch/Hauling events are
deduplicated and monotonic. Vendor Own Delivery uses authenticated vendor
actions and never creates a platform logistics mission.

## Running the tests

**The canonical full integration regression is:**

```
go run ./tools/vendor-test-runner
```

(or `scripts/test-vendor-isolated.ps1`, which is a thin wrapper around it.)

That runner creates one PostgreSQL database per test package, cloned from a
freshly migrated template, and runs each package against its own copy.

### Why `go test ./...` is not the certification gate

`go test ./...` runs every package against the single database named by
`VENDOR_SERVICE_TEST_DATABASE_URL`. The integration packages in this service
mutate shared, migration-seeded and platform-wide state, so they contaminate one
another when they share a database. This is an architectural property of the
suite, not a flake, and it has been observed directly:

- one package truncated `shop_measure_definitions`, removing the 43 rows a
  migration seeds, after which `taxonomy` and `shop` failed with
  "Measure definition not found" and a foreign-key violation;
- another package changed role-permission rows, after which
  `section13security`'s IAM assertions failed because `role_owner` and
  `role_general_manager` appeared to hold platform capabilities;
- concurrency tests that observe PostgreSQL lock state were satisfied, or
  starved, by backends belonging to entirely different packages.

None of those were defects in the packages that failed. Running the same
packages against isolated databases passes.

`go test ./...` and `go test ./internal/features/<pkg>/` remain useful while
working on a single package, and are fine for the packages that hold no mutable
integration state (the `*contract` OpenAPI packages, `internal/config`,
`internal`). They are not the authoritative full-service gate, and a green or
red `go test ./...` should not be reported as certification either way.

### Requirements

- `VENDOR_SERVICE_TEST_DATABASE_URL` - a PostgreSQL superuser-capable role, on a
  local host, pointing at a non-system database. The runner refuses remote hosts
  unless `VENDOR_SERVICE_TEST_ALLOW_REMOTE_ADMIN=1` is set.
- `VENDOR_SERVICE_TEST_REDIS_ADDR` - for the packages that use Redis.

Packages skip cleanly when these are unset, so `go test ./...` without a
database still compiles and runs the pure-unit tests.


## Section 17B — Checkout validation and Inventory reservation

Section 17B turns an ACTIVE Section 17A cart into a validated checkout
candidate: three customer routes under `/api/v1/commerce/checkout-sessions`,
migration `000076`, contract
`docs/openapi/section17b-commerce-checkout.openapi.json`.

**It validates and it holds stock. It does not sell anything.** There is no
quote, no quote lock, no logistics price, no payment, no ParentOrder and no
VendorOrder in this section — not a route, not a column, not a code path. The
contract package refuses such a path structurally rather than by convention,
because "we will add it properly later" is how a checkout ends up charging
money nobody designed.

**Nothing the cart says is trusted.** The cart's price and availability
snapshots are evidence of what the customer last saw. Every line is re-asked at
checkout time: Food through the Section 9 authority (which already answers
business, branch, capability, menu, service window, item, operational state and
the fail-closed `INVENTORY_LINKED` seam as one decision), Shop through the
Section 12 catalogue, and stock through Section 14. Search is discovery only
and is not consulted here at all.

**Section 14 is the stock authority, and only Section 14.** Shop's Section 12B
inventory surface stays live and unchanged; Section 17B neither reads nor
writes it, and takes only Shop's catalogue-side sellability reasons. Asking two
ledgers the same question is how a checkout ends up holding stock one of them
never had. The practical consequence: finish the Section 14C compatibility
backfill for a branch before enabling checkout for it, or its Shop lines will
fail closed.

**Multi-vendor atomicity is compensation, not one transaction.** Section 14
owns one transaction per reservation. Each checkout group and the reservation
source reference it will use are committed *before* the first Inventory call,
so after that commit there is always a durable key naming every reservation
that might exist — which is what makes compensation, crash recovery and
idempotent retry all possible. The end state is every required group RESERVED,
or FAILED with no hold left behind.

**The reservation TTL is configuration.** `COMMERCE_CHECKOUT_RESERVATION_TTL_SECONDS`
is required and bounded; the service refuses to start on a value outside
`[60, 86400]`, and nothing in the code path assumes one. Lapsed and orphaned
holds are released by `COMMERCE_CHECKOUT_RECOVERY_ENABLED`, and Section 14's
own reservation-expiry worker is the backstop underneath that.

Full detail, including the reservation-basis fail-closed rule and the
concurrency gates, is in `docs/IMPLEMENTATION_STATUS.md`.

### Running the Section 17B tests

The checkout integration tests live in `cmd` so that they drive the real
production adapters, and they need a migrated database:

```
VENDOR_SERVICE_TEST_DATABASE_URL=... go test ./cmd/ -run TestSection17B
```

The pure-unit money and grouping tests need nothing:

```
go test ./internal/features/checkout/ ./internal/section17bcontract/
```

## Section 8I â€” final CRM certification

Section 8I is the closing certification pass over Sections 5â€“8H. It adds no CRM
feature, no public route and no migration. What it adds is the evidence that the
whole thing works as one backend rather than as eight sections that each pass
their own tests.

**Two production defects were found and fixed.**

The first was an authority-surface defect. `POST /internal/staff/provision-owner`
â€” the route that creates the first Owner of a vendor business â€” was wired under
the public vendor base at `/api/v1/vendor/internal/...`, while its own OpenAPI
specification, the Section 5 contract package and the route register had always
described it at the engine root. Section 7 established the rule that internal
authority ingress must not sit behind the path the gateway forwards; this route
was missed in that pass. The wiring now matches the documentation.

The second was a transaction-scope defect in Section 8F. The notification
handoff ran a synchronous HMAC call to `notification-service` while holding
three row locks â€” the delivery, the run, and the customer profile. That last one
is the shared privacy fence, so a slow or hanging remote call blocked redaction
for that customer and pinned a database connection for the round trip. The
handoff now reserves the delivery, commits, calls, and records the outcome in a
second transaction. Contact history is deliberately still written under the
profile lock, because the frequency cap counts those rows â€” moving it out broke
the accepted concurrency test, which is exactly what that test exists for.

**One global contract guard was added.** `internal/crmcontract` builds the real
production router from the same `Register*` helpers `cmd/main.go` calls and holds
it against every OpenAPI specification and the route register in both
directions. The per-section contract packages each pinned their own surface;
nothing pinned the whole of it, which is the gap that lets a live undocumented
route hide. The accepted count â€” **194** â€” is now derived from code rather than
counted from prose.

**Cross-module certification.** `section8i_certification_test.go` asks one
question of the entire CRM: after a redaction, can any module still act on this
person? Segmentation, intelligence, corporate contacts and exports each answer
independently, and three anti-resurrection paths â€” a new tag, a rebuild, and a
late order fact arriving from the authority â€” are each proved unable to bring
the relationship back.

## Section 8H backend

Section 8H adds Vendor-owned CRM intelligence and secure exports: a versioned code-defined model registry, deterministic RFM V1 and Churn V1, score availability and freshness states, explainability, generation-safe materialization, dependency-scoped invalidation, a Customer 360 intelligence block, operational analytics, eight new Section 8B segment fields, and a step-up protected export pipeline over six closed datasets. Public surface `/api/v1/vendor/crm/intelligence` and `/api/v1/vendor/crm/exports` (10 operations, no internal routes). Migrations `000024`-`000025`. Contract: `openapi/section8h-crm-intelligence-exports.openapi.json`.

**No machine learning, and no probability.** Both models are heuristics computed in Go from commercial facts this service was given. There is no model artefact, no training data, no float anywhere in the schema and no generated prose. A churn risk of 70 ranks a relationship for follow-up; it is not a 70% chance of anything, and the OpenAPI contract has a test that fails if it ever starts claiming otherwise.

**A score is not permission.** A relationship can be the most valuable in the book and still be one this vendor may not contact. Consent, privacy, marketing suppression, frequency and quiet hours are decided by Sections 8A, 8E and 8F, are not consulted to produce a score, and are not weakened by one. A real-Postgres test drives a high-churn customer who denied marketing consent through the segment engine and proves they are not selected.

**Unavailable is not zero.** A relationship with no observed commercial facts gets a `NOT_AVAILABLE` score row with a reason code and every numeric column NULL â€” a database CHECK makes a row asserting otherwise unstorable. The segment fields compile score predicates so a missing score yields SQL NULL, which means `churn.risk_score GT 70` cannot be satisfied by absence and `rfm.composite_score LT 5` cannot be satisfied by a relationship nobody ever scored.

**A failed rebuild never empties a dashboard.** The new generation is built alongside the live one and swapped in one transaction that also re-certifies against privacy; a failure before the swap leaves the previous scores exactly as they were and the scope reports `STALE` with its last-good as-of intact.

**Exports release nothing until they are complete and still lawful.** Generation, privacy re-certification, private upload and promotion happen in that order; `READY` has a database CHECK requiring the artifact, its checksum, its size, a completion instant and an expiry; and access is decided live per request against a fresh step-up, returning a token that outlives neither the request nor the store's own short window. There is no URL column anywhere.

## Section 8G backend

Section 8G adds the Vendor-owned corporate customer CRM: corporate accounts with a vendor-local `CORP-` reference, a relationship lifecycle, business or branch-set scope, contacts that reference Section 8A customer relationships, memberships, the organization's own locations, cost-centre labels, an account manager, and corporate interactions and tasks that reuse the Section 8C engine. Public surface `/api/v1/vendor/crm/corporate-customers` (41 operations, no internal routes). Migrations `000022`-`000023`. Contract: `openapi/section8g-crm-corporate-customers.openapi.json`.

**Section 8G records who the vendor deals with; it sells nothing.** There is no corporate order, invoice, credit line, budget, contract, quotation, wallet, catalogue entitlement or approval anywhere in it â€” no route, no permission, no schema, no column and no state. A cost centre is a label with no budget, limit, currency or balance, and `payment-wallet-service` remains the only financial authority. Three separate tests (migration, OpenAPI contract, live router) reject a commerce or finance surface structurally rather than by convention.

**A contact is a reference, not an address book entry.** Corporate contacts point at Section 8A customer relationships and carry a non-identifying display label resolved at read time; no email address, phone number or postal address is duplicated here or returned by this section. The only address in Section 8G is the customer organization's own site record.

**Privacy and identity merge reach corporate contacts through the existing Section 8A port.** `corporateContactPrivacyHandler` registers on the same `PrivacyHandler` registry Sections 8B and 8C use, so redaction suppresses the link and deactivates memberships, and an identity merge moves contacts to the surviving relationship â€” resolving a collision by archiving the duplicate, moving its memberships and promoting the survivor when the source was primary. `ApplyPrivacy` was not modified.

**Corporate activity reuses Section 8C rather than duplicating it.** `crm_interactions` and `crm_tasks` gained a nullable corporate subject under a `CHECK` that makes a subject-less or double-subject row unstorable. A second task state machine would have been two places for `OPEN`/`IN_PROGRESS`/`COMPLETED`/`CANCELLED` to disagree; a second revision model would have been two definitions of immutable history. Every corporate activity route requires the Section 8C permission **and** `vendor.crm.corporate.read`.

## Section 8E backend

Section 8E adds the Vendor-owned CRM campaign intelligence and audience engine: campaign definitions, immutable campaign versions, segment-backed audience selection, privacy-safe audience preview, durable audience preparation, immutable audience snapshots, campaign targets, snapshot-time consent evidence per channel, deterministic experiment cohorts, and frequency and quiet-hour policy definition. Public surface `/api/v1/vendor/crm/campaigns` (10 operations, no internal routes). Migrations `000018`-`000019`. Contract: `openapi/section8e-crm-campaign-intelligence.openapi.json`.

**Section 8E prepares; Section 8F executes.** This section answers who a campaign should target and freezes that answer. It cannot answer how or when a message is delivered: there is no scheduling code, no send endpoint, no notification handoff, no template, no provider client and no destination column anywhere in it. `notification-service` remains the sole delivery authority and is unchanged.

**A prepared audience is not permission to contact anybody.** Consent is evaluated twice: once here, recorded as historical evidence on each target channel, and again by Section 8F immediately before any delivery handoff, against current consent. Snapshot evidence is never rewritten when a customer later changes their mind â€” they are protected by a mandatory send-time recheck, not by history being edited. `send_time_recheck_required` is pinned true by a database check, so a row that claimed otherwise cannot be stored.

**Consent fails closed, and there is nothing to fail open into.** Only an explicit, fresh, unexpired ALLOW produces an eligible channel. DENY, UNKNOWN, a missing projection, a lapsed ALLOW and a stale projection are all ineligible, and with no consent authority integrated at all â€” which is the platform's state today â€” every channel is ineligible with reason `CONSENT_SOURCE_UNAVAILABLE`. Segment membership is not consent, and a control-cohort assignment is not an exemption.

**Suppressed relationships get no row at all.** A privacy- or service-recovery-suppressed customer appears only in the snapshot's aggregate counts, never in `crm_campaign_targets`. That is deliberately stronger than storing them with a suppressed state: a customer who must not be contacted is absent from the only table Section 8F will read, so no future query mistake can reach them.

**Audience comes only from Section 8B.** There is no second rule language, no raw SQL parameter and no customer-list upload, so every branch, privacy and consent wall the segment engine already built applies here without being rebuilt â€” and, more importantly, without being subtly rebuilt differently. A preparation pins the exact segment generation it reads, which is what stops two segment rebuilds being mixed into one audience.

## Section 8D backend

Section 8D adds the Vendor-owned CRM loyalty foundation: a per-business programme configuration, lazily created customer loyalty accounts, an append-only points ledger, point lots with per-issuance expiry, recorded lot consumption, a durable expiry sweeper, bounded manual adjustments, and full reversal. Public surface `/api/v1/vendor/crm/loyalty` and `/crm/customers/{profileId}/loyalty` (6 operations, no internal routes). Migrations `000016`-`000017`. Contract: `openapi/section8d-crm-loyalty.openapi.json`.

**Points are not money.** This is a points ledger, not a second financial authority. There is no currency column, no minor-unit amount, no wallet reference and no conversion path anywhere in Section 8D; `payment-wallet-service` remains the sole owner of money, and Section 8D emits no `payment.*`, `wallet.*`, `refund.*`, `settlement.*` or `payout.*` event. A schema test and a contract test both reject a monetary-looking column or field structurally.

**No earn or redeem endpoint.** The earning and redemption engines are real and tested, but they are service-internal primitives (`CreditPoints`, `DebitPoints`) waiting for a trusted producer. Publishing either as a vendor-callable endpoint would let any authorised staff member name their own points total. The only vendor-callable movements are a manual adjustment â€” bounded by the programme's own limit, gated behind a dedicated high-risk permission, and requiring an audited reason â€” and a full reversal. There is deliberately no earning formula anywhere: the Vendor Order domain that would define what an order is worth does not exist yet, so nothing here converts money to points.

**Expiry is stamped once and never rewritten.** Each credit issues a point lot carrying the expiry policy as it stood at issue time. Changing the programme's window later affects future credits only, and an identity merge carries each lot's original expiry across unchanged â€” a merge never buys a customer a fresh window, and a policy change never retroactively voids points they already hold.

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

**Branch scope reaches into the rule and Customer 360.** A branch-scoped segment reasons over that branch's slice of every commercial fact rather than the business total, and every count â€” preview, tag membership, segment membership â€” is branch-filtered. Customer 360 returns only tags and segment classifications owned by the caller's permitted branches; business-wide and other-branch classifications are withheld because the label itself is a disclosure.

## Section 8A backend

Section 8A adds the CRM foundation: vendor-local customer relationship profiles, a privacy-safe Customer 360, projected marketing consent, projected privacy state with a working redaction engine, an authoritative identity-link/merge path, and the commercial-projection foundation. Public surface `/api/v1/vendor/crm/customers` (six operations); root-level authority ingress `POST /internal/vendor/v1/crm/events`. Migrations `000010`-`000011`. Contract: `openapi/section8a-crm-foundation.openapi.json`.

**Authority boundary, which is the whole point of this section.** Vendor Service is not a customer master. customer-service owns canonical identity, and (once they exist) consent and privacy decisions. CRM stores opaque subject references, never canonical email, phone, address or date of birth, and never a hash of a customer reference used as a substitute identity. There is no vendor-facing route that creates a customer, sets consent, sets privacy state or merges identities â€” a contract test enforces that structurally.

**Fail-closed marketing.** An absent, unknown, expired or stale consent decision is never permission. Privacy outranks consent: a redacted, restricted or marketing-suppressed subject is ineligible regardless of any stored ALLOW. No permission exists â€” at any role, including Owner â€” that can override either.

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

If a document has no impact from a given section, record `NO CHANGE â€” <reason>` in that document rather than leaving it silently untouched.

## Maintained documents

| Document | Purpose |
|---|---|
| [UI_UX_REQUIREMENTS.md](UI_UX_REQUIREMENTS.md) | Plain-language UI/UX requirements generated from backend/product implementation, for the future Vendor Flutter app team. No frontend code. |
| [FRONTEND_REQUIREMENTS.md](FRONTEND_REQUIREMENTS.md) | Plain-language backend-contract handoff (routes, auth, fields, validation, events) for the future frontend team. No frontend code. |
| [PLATFORM_SERVICE_CHANGE_REGISTER.md](PLATFORM_SERVICE_CHANGE_REGISTER.md) | Everything Vendor Service implementation requires outside `services/vendor-service` â€” config changes, migrations, routes, events on other services. |
| [API_ROUTE_REGISTER.md](API_ROUTE_REGISTER.md) | Human-readable master catalogue of every Vendor Commerce route across every affected service. `openapi/` is the machine-readable authority once real routes exist; this is the human-readable index. |
| [IMPLEMENTATION_STATUS.md](IMPLEMENTATION_STATUS.md) | Per-section implementation tracker: scope, status, code paths, migrations, routes, events, workers, tests, affected services, documentation updated, remaining gaps, acceptance result. |
| [architecture/](architecture/) | Technical backend architecture documents (service boundaries, domain ownership, module map, integrations, payment allocation, fulfilment model, OTP/QR dependency status, event ownership, security model, worker/recovery model, feature flags, migration ownership). Read these before implementing any section. |
| [architecture/00_REPOSITORY_AUDIT_BASELINE.md](architecture/00_REPOSITORY_AUDIT_BASELINE.md) | The factual repository audit this whole documentation set is built on â€” read first. |
| `openapi/` | Machine-readable API specs for implemented Sections 5 through 8E; bidirectional route-parity tests prevent undocumented or phantom routes. |

## Section 6 delivery

Vendor business, brand, capability, and branch management is implemented in `internal/features/vendors`, migrations `000006`-`000007`, and `openapi/section6-vendor-business.openapi.json`. Section 7 onboarding builds on that graph; Section 8A projects CRM relationships onto it; Section 8B adds tags and segmentation; Section 8C adds interactions and tasks; Section 8D adds the loyalty points ledger; Section 8E adds campaign intelligence and the audience engine; Section 8F adds campaign execution, Section 8G corporate CRM, and Section 8H intelligence and secure exports, all certified in Section 8I. Catalogues and commerce remain out of scope. The UI handoff is documentation-only; no frontend source was changed.

## Reading order for a new implementation section

1. `architecture/00_REPOSITORY_AUDIT_BASELINE.md` â€” what actually exists today, verified.
2. `architecture/01_SERVICE_BOUNDARIES.md` and `02_DOMAIN_OWNERSHIP.md` â€” what you may and may not own.
3. `architecture/03_VENDOR_SERVICE_MODULE_MAP.md` â€” where your code goes.
4. Whichever of `04`â€“`12` is relevant to the section (integrations, payment, fulfilment, OTP/QR, events, security, workers, flags, migrations).
5. `IMPLEMENTATION_STATUS.md` â€” what has already shipped, so you don't redo or contradict it.
6. `PLATFORM_SERVICE_CHANGE_REGISTER.md` â€” check whether your section depends on an already-tracked external change.

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

## Section 9A â€” food catalogue foundation

Section 9A implements the Restaurant/Fast-Food catalogue in
`internal/features/food` (`models.go`, `service.go`, `menus.go`, `items.go`,
`modifiers.go`, `http.go`), migrations `000029`-`000030`, and
`openapi/section9a-food-catalogue.openapi.json`, pinned by
`internal/section9acontract`.

### The hierarchy

```
FOOD MENU            one business + one branch + one food capability
  â””â”€ MENU SECTION    ordered groups inside a menu
       â””â”€ FOOD ITEM  ordered items, priced in integer minor units
            â”œâ”€ VARIANTS               size/portion, price override or inherit
            â””â”€ MODIFIER GROUPS  â”€â”€â–º  MODIFIERS   (shared, attached, ordered)
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
key â€” `(branch_id, business_id)`, `(menu_id, business_id)`,
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
delta may be zero but never negative â€” a discount is a promotion, and promotions
are not this domain's authority.

### Lifecycle and archive

Menus and items share one closed lifecycle: `DRAFT â†’ ACTIVE â‡„ PAUSED`, with
`ARCHIVED` terminal. Sections, variants, groups and modifiers are `ACTIVE` or
`ARCHIVED`. A DRAFT catalogue may be prepared while the business is still
onboarding; publishing enforces the existing Section 6/7 business, branch and
capability state rather than creating a second onboarding authority.

Nothing is hard-deleted except the item/modifier-group relationship row, which
carries no historical authority. Archiving a section archives its items, and
archiving an item archives its variants, in the same transaction â€” an entity
left outside the catalogue but still reachable by id is exactly what archive
semantics exist to prevent.

`PAUSED` here is administrative. It is **not** Section 9B's sold-out or
availability state, and the lifecycle vocabulary cannot express one.

### Variants and modifiers

A variant's `price_override_minor` is null to inherit the item's base price and
non-null to override it; `inherits_base_price` is derived for the response so
the two cannot disagree. A modifier group's `min_select > 0` **is** what
"required" means â€” there is no separate boolean, because two columns encoding
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
proves each reference against that service's existing private ownership API â€”
the same contract Section 7 uses for onboarding documents â€” before storing it.
With no media authority configured the write is refused rather than storing an
unverified reference. There is no upload route in this section.

### Dietary and allergen metadata

Bounded, normalised, de-duplicated codes matching `^[A-Z0-9_]{2,40}$`, capped at
16 dietary and 24 allergen entries. This is declarative vendor-supplied
catalogue information: the service computes no nutrition, certifies nothing, and
makes no medical claim. The narrow code shape is deliberate â€” it expresses any
vendor vocabulary while making it impossible to put markup, a query or a URL
into what is displayed as a dietary claim.

### 9A / 9B / 9C boundary

| Question | Owner |
|---|---|
| What does this branch sell? | **9A (this section)** |
| Can it be sold now, at this branch, at this time, for what effective price, with what preparation/logistics profile? | **9B (implemented)** |
| How do customers and other services read it? | 9C â€” internal discovery contract implemented; public frontend remains deferred |

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

## Section 12A â€” Shop / Grocery / Market / Wholesale catalogue

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

## Section 12B â€” Shop inventory, availability and discovery

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

## Section 13 â€” Catalogue, Taxonomy and Search

Section 13 gives Vendor Service the part of the platform that decides what a
customer can find. It has two halves that are easy to confuse and must not be:

**The canonical taxonomy** is the platform's shared vocabulary â€” the versioned,
governed set of facets and nodes every vendor's catalogue is classified against.
Publishing a new version changes how every vendor on the platform is discovered,
which is why authoring it is a platform capability reachable only through a
root-mounted HMAC-v2 internal API, and why no vendor role grants it.

**The search projections** are derived read models over the Food and Shop
catalogues. They are fast because they are denormalised, and they are always
downstream: the Food catalogue (Section 9) and the Shop catalogue and inventory
(Section 12) remain authoritative. A search hit is a pointer back to that
authority, never a promise about it.

### FOOD and SHOP are two systems, not one

Separate namespaces, versions, projection tables, term tables, ranking profiles,
generations, cache key spaces and engines.

The reason is concrete: both catalogues contain "rice". A Jollof Rice dish and a
50kg sack of raw rice match the same text and mean completely different things
to a customer. Merging them produces a result list that is useless in both
directions, and filtering afterwards does not reliably unmerge it. Because the
separation is structural rather than a `WHERE` clause, it cannot be undone by
forgetting one.

### Redis is a cache and only a cache

Every engine reads through to PostgreSQL when the cache misses or is
unreachable. Redis being down makes search slower; it does not make search wrong
or unavailable. The health endpoint reports cache availability **separately**
from search health for exactly this reason â€” an operator who cannot tell the two
apart will fail over a tier that is answering correctly.

Cache keys include the active generation and the cache revision, so a stale
entry is never served: it simply stops matching.

### A search result is not a stock reservation

An `IN_STOCK` Shop result reports what the projection last observed. Checkout
revalidates against authoritative inventory and rejects the purchase if stock
has since gone. Inventory never goes negative because of something search said,
and no route in Section 13 can hold, reserve or consume stock.

### Ranking is bounded, and commercial boost cannot beat relevance

Ranking profiles are vectors of bounded weights. Two of them are commercial, and
both are capped by a non-text ceiling that is itself required to stay strictly
below the weight of an exact name match. That one inequality is what stops a
promoted result outranking a relevant one, and it is enforced by database
constraints, by the engine when it loads a profile, and by the administration
path before a weight is stored.

Sponsored placement is labelled in the response. There is no covert boost, and
there is no advertising system here â€” no campaign, no budget, no auction.

### No customer profiling

Section 13 stores no customer identity and builds no search profile. There is no
search-history table, no behavioural graph, no user embedding, and the
suggestion API has nowhere to put a customer id. Long-term behavioural analytics
remains `analytics-service`'s authority.

### Rebuilds are real

A privileged rebuild request allocates a new generation and a worker streams the
authoritative Food and Shop catalogues into it in bounded batches, while the
previous generation keeps serving every query. Only when the rebuilt document
count matches what the worker reported processing does the generation become
READY, and only then is it activated - atomically, retiring the generation it
was built from.

The rebuild reads the catalogue, never the search projection and never Redis. If
it read the projection, a corrupted projection would rebuild into an identically
corrupted one and the recovery mechanism would be unable to recover anything.

### Where to read more

- [architecture/14_CATALOGUE_TAXONOMY_SEARCH.md](architecture/14_CATALOGUE_TAXONOMY_SEARCH.md) â€” the end-to-end reference for 13Aâ€“13I
- [SECTION13_FINAL_CERTIFICATION_REPORT.md](SECTION13_FINAL_CERTIFICATION_REPORT.md) â€” what was executed and what it proved
- [API_ROUTE_REGISTER.md](API_ROUTE_REGISTER.md) â€” all 60 routes with their authority model
- OpenAPI: `section13-taxonomy`, `section13-taxonomy-admin`, `section13-search`, `section13-search-projection`


## Section 14 â€” Generic Inventory

Section 14 turns `internal/features/inventory` from a reserved boundary into
the Vendor Service's stock authority. It owns inventory item identity and stock
basis, balances, lots, the movement ledger, reservations, goods receiving,
stocktakes, branch transfers, quarantine, reorder rules and alerts.

### Inventory owns stock; Shop still owns the catalogue

The split is deliberate and is enforced by a static architecture test rather
than by convention:

- **Shop** owns products, variants, listings, sell units and catalogue
  measurement configuration.
- **Inventory** owns everything about how much of something exists, where it
  is, and whether it can be sold.

Shop depends on Inventory. Inventory never imports Shop. The Section 12B Shop
inventory API is unchanged and still live: Section 14 bridged to it through
inventory item identity instead of migrating or retiring it, so nothing that
worked before Section 14 stopped working.

### PostgreSQL is the only source of stock truth

Neither the Inventory nor the Shop feature package references Redis anywhere.
Redis is a cache and a job coordination aid elsewhere in this service; it is
not consulted for a balance, a reservation, a lot or a movement, and its
absence cannot corrupt them.

### The ledger is evidence, not a mutable table

Every stock change appends an `inventory_movements` row, and a database trigger
refuses UPDATE and DELETE on that table unconditionally â€” including from code
already holding the authorized-mutation session flag. Corrections arrive as new
movements. Physical stock tables additionally refuse any write that did not
come through an authorized inventory mutation, so no ad-hoc SQL path can invent
stock.

### Reservations are all-or-nothing

A reservation either holds every requested line or none of them. Reservations
are idempotent on `(source_type, source_ref)` together with a request
fingerprint: replaying the same request returns the current durable reservation
state, while reusing the reference with different contents is a conflict rather
than a silent overwrite. Expiry, commit and release settle exactly once even
when they race each other.

### What Section 14 will not do

Section 14 raises a restocking signal; it never creates a purchase order.
Suppliers, purchasing and goods procurement are Section 15. Ingredients,
recipes, production and costing are Section 16. Orders, checkout, payment and
logistics are Section 17 and later. `INVENTORY_LINKED` Food availability fails
closed until Section 16 supplies a real recipe resolver, which is the point of
the mode rather than a gap in it.

### Where to read more

- `docs/openapi/section14-vendor-inventory.openapi.json` â€” the machine-readable
  contract for all 34 routes.
- `docs/architecture/15_GENERIC_INVENTORY.md` â€” the authority model, invariants
  and worker design.
- `docs/API_ROUTE_REGISTER.md` â€” the registered route surface.
- `internal/section14contract` â€” the executable proof that those three agree.


## Section 15 - Suppliers, Purchasing and Goods Receiving

Section 15 answers: who supplies this, at what price, who approved buying it,
what actually turned up, and what was wrong with it.

### Only accepted stock becomes stock

The worked example the whole section is built around:

```
Ordered    100
Delivered   98      (supplier sent 2 short)
Accepted    90
Damaged      5
Rejected     3

Inventory receives +90.  Not 98. Not 100.
```

Damaged, rejected, short and wrong-item quantities become discrepancy evidence.
They never become sellable stock. Generic Inventory (Section 14) remains the
sole stock authority; purchasing hands it the accepted quantity and nothing
else.

### Receiving stock does not move money

Purchasing records what things cost, in an append-only
`vendor_purchase_cost_history` the database refuses to update or delete. That
is commercial evidence, not a payment. Receiving a delivery creates no wallet
transaction, ledger entry, payout or payment intent, and a certification gate
greps the source to keep it that way.

### An approved order is a commitment

A supplier invoicing above the approved amount does not rewrite the order. The
approved total stands, a `COST_VARIANCE` discrepancy records expected, actual
and variance, the configured approval policy applies, and the immutable cost
record carries what was actually paid.

### Supplier performance tells the truth about what it does not know

A supplier nobody has received from does not have a perfect delivery record; it
has no delivery record. Every rate carries its own denominator and a
`metric_status`, and reports `INSUFFICIENT_HISTORY` with a null value rather
than a flattering zero.

### Where to read more

- `docs/openapi/section15-vendor-purchasing.openapi.json` - all 56 routes.
- `docs/architecture/16_PURCHASING.md` - the model, lifecycles and boundaries.
- `internal/section15contract` - the executable proof that the router, the
  specification and the route register agree.

<!-- SECTION 16B RECIPES -->
## Section 16B — Recipe definitions

Vendor Service recipe definitions now support versioned Food/Pastry/Cloud Kitchen production definitions without moving catalogue or Inventory authority into Recipes.

<!-- SECTION16C_IMPLEMENTATION -->
## Section 16C — Production Batch Execution

Backend implementation includes production-batch lifecycle, immutable RecipeVersion
requirement snapshots, Section 14 all-or-nothing Inventory reservation, explicit
top-ups, actual consumption/waste evidence, production-time Packaging consumption,
ON_DEMAND/BATCH_STOCK operation and exactly-once prepared-stock output.

Sections 10/11 remain the production-capacity/job authority. Section 14 remains
the sole physical-stock ledger. Section 16D COGS is not implemented by 16C.

## Section 16D - Operational cost, COGS and production cost variance

Section 16D answers one question: what did it cost us to make this, based on
what we actually paid for the inputs. It is OPERATIONAL ANALYTICS. There is no
ledger, no journal, no settlement and no Wallet effect anywhere in it, and a
production variance never becomes an automatic financial adjustment or a change
to a selling price.

Backend support:

- **Ingredient and packaging cost policies** - business-level defaults with
  optional per-branch overrides, versioned and audited. Packaging is costed
  through its own subject type; it is never modelled as an ingredient.
- **Three explicit cost strategies** - latest accepted purchase cost, bounded
  quantity-weighted average purchase cost (default: 10 most recent accepted
  observations), and manual configured cost. Manual cost is consulted only when
  the active policy selects it, so it can never quietly outrank real evidence.
- **Section 15 as the sole purchase-cost authority** - read through a narrow
  port over `vendor_purchase_cost_history`. No second purchasing ledger exists.
  A REVERSAL cancels its matching OBSERVED evidence, so reversed receipts leave
  the cost basis.
- **Recipe estimated COGS** - per-component breakdown, packaging contribution,
  expected-waste contribution, and cost per serving and per output unit.
- **Expected waste applied exactly once** - Section 16B stores NET quantities,
  so recipe COGS grosses up; Section 16C freezes waste-inclusive quantities, so
  production expected cost does not gross up again.
- **Immutable cost snapshots** - historical evidence records every component,
  the quantity used, the unit cost applied and the receipt it came from, so a
  figure from months ago can be re-derived line by line. Enforced by database
  trigger, not only by the service.
- **Production expected cost, actual cost and variance** - calculated from the
  batch's own frozen and measured evidence, never from a newer recipe version.
- **Completeness is visible** - when no authoritative cost resolves, the result
  is INCOMPLETE or UNKNOWN with a per-component reason. Absence never becomes a
  fabricated zero, and no FX rate is ever invented.
- **Cost privacy** - `vendor.cost.read`, `vendor.production.cost.read` and
  `vendor.cost.policy.manage` are separate from recipe and production
  permissions. Kitchen and pastry staff keep every production permission and
  receive no cost visibility at all.

Exact money: integer minor units with exact rational quantities, rounded half
away from zero once per component. No floating point.


<!-- SECTION17A_CUSTOMER_CART -->
## Section 17A - Customer Cart Foundation

Section 17A implements customer-owned Food and Shop carts under
`/api/v1/commerce`.

Customer identity comes only from the authenticated Customer Service bearer
subject. Food carts are restricted to one Vendor business and one branch. Shop
carts may contain multiple Vendor businesses and branches and expose grouping
for later checkout conversion.

Cart entries retain catalogue, price, revision and measurement evidence for
later authoritative checkout revalidation. Saved Customer Service addresses are
resolved through the internal HMAC contract and stored as immutable cart
snapshots. Optional receiver information represents a third-party recipient,
not another Customer account.

Commands reuse customer-scoped Idempotency-Key handling. Existing-cart mutations
use optimistic concurrency through If-Match. Cart mutation evidence and
commerce.cart.* events are committed transactionally using the existing
vendor_outbox.

Section 17A does not own checkout, inventory checkout reservation, payments,
platform delivery pricing, logistics execution, ParentOrder or VendorOrder.


<!-- SECTION17C_FULFILMENT_PLANNING -->
## Section 17C - Fulfilment planning and logistics integration

Section 17C turns validated Section 17B vendor/branch groups into an
authoritative candidate fulfilment plan under `/api/v1/commerce`, plus a
Vendor Own Delivery policy surface under `/api/v1/vendor`.

For each group it aggregates the real logistics metadata the catalogue holds
(weight, volume, dimensions, package count, handling and temperature
requirements, urgency), selects the smallest vehicle class that can actually
carry it, and then either uses the vendor's own delivery policy or asks the
owning logistics authority for a real quote.

**Vendor Service does not price platform deliveries.** Every platform fee comes
verbatim from the owning service's own quote endpoint - a contract that did not
exist before this section and was added to driver-dispatch-delivery-service and
driver-hauling-service as part of it (see
`docs/PLATFORM_SERVICE_CHANGE_REGISTER.md`). The only fee calculated here is a
Vendor Own Delivery fee, because the vendor both performs and prices that
delivery.

**Taxi is not used and is not required.** driver-taxi-service has no production
goods-delivery capability. The shared quote contract has no CAR class, the
class-to-service mapping refuses one, and no taxi client is wired.

**Nothing is silently dropped.** Every group appears in the plan whether or not
it can be delivered, and a refused group names an explicit reason the customer
can be shown.

**Nothing here is an authorization.** Section 17C plans and quotes. It creates
no mission, assigns no provider, and moves no money; 17D locks the
customer-protected total.

### Running the Section 17C surface

It is off by default. `COMMERCE_FULFILMENT_PLANNING_ENABLED=true` requires
`COMMERCE_CHECKOUT_ENABLED=true` and at least one delivery authority
(`VENDOR_OWN_DELIVERY_ENABLED`, `DISPATCH_COMMERCE_INTEGRATION_ENABLED` or
`HAULING_COMMERCE_INTEGRATION_ENABLED`) - otherwise it would be a surface whose
only possible answer is "no delivery option available", and config validation
refuses to start.

Platform quotes additionally need the far side configured:

```
# vendor-service (BARE ORIGINS - the client appends each service's API base)
VENDOR_DISPATCH_URL=http://localhost:8103
VENDOR_DISPATCH_SECRET=<shared>
VENDOR_HAULING_URL=http://localhost:8104
VENDOR_HAULING_SECRET=<shared>

# driver-dispatch-delivery-service
VENDOR_SERVICE_SECRET=<same as VENDOR_DISPATCH_SECRET>

# driver-hauling-service
HAULING_SERVICE_SECRETS=...,vendor-service=<same as VENDOR_HAULING_SECRET>
```

A missing secret on either side means that authority's quote route is not
registered at all and the class is simply unavailable - which the plan reports
honestly rather than papering over.

## Section 17D - Immutable checkout quote lock

Section 17D freezes the current Section 17B/17C commercial evidence into the
exact expiring quote the customer approves. Migration
`000079_vendor_immutable_checkout_quote` stores append-only quote, vendor/group,
line, logistics, adjustment, approval and event evidence; database triggers
reject every UPDATE and DELETE. The quote fingerprint binds all material
inputs, its version and expiry. A changed or expired agreement creates a new
version while the old one remains historical.

The three customer-bearer routes are documented in
`docs/openapi/section17d-immutable-checkout-quote.openapi.json`. Creation and
approval require `Idempotency-Key`; their strict bodies accept no customer
price, fee, funding, vendor or branch override. The response preserves exact
and bounded variable-weight totals, funding attribution, Inventory and
logistics references, and explicit `VENDOR_ONLINE_PRICE` disclosure without a
physical-store parity promise.

Enable with `COMMERCE_QUOTE_LOCK_ENABLED=true` and set
`COMMERCE_QUOTE_TTL_SECONDS` (default 300). Config validation requires both
checkout and fulfilment planning. No Wallet, payment, ParentOrder, VendorOrder
or execution mission is wired by Section 17D itself.

## Section 17E - ParentOrder and multi-vendor VendorOrders

Section 17E turns one approved, unexpired Section 17D quote into exactly one
ParentOrder and one financially and operationally isolated VendorOrder per
vendor/branch allocation. Migration `000080_vendor_commerce_orders` adds the two
aggregate tables plus append-only order lines, an allocation register and a
lifecycle event log; database triggers freeze the three evidence tables while
leaving the aggregates writable, because their status is exactly what a
transition changes.

Creation is exactly-once per approved quote (`UNIQUE(quote_id)`) and atomic:
a retry returns the existing order with 200, and a failed VendorOrder leaves no
ParentOrder claiming success. Each VendorOrder carries only its own money, its
own Section 14 reservation, its own mission reference and its own status, and
the ParentOrder's state is always derived from the children - a purchase where
some vendors completed and others cancelled reports `PARTIALLY_COMPLETED`
rather than either extreme. Completion and cancellation *prepare* settlement
and refund allocations; **no payment operation is created and no money moves.**

Twelve routes are documented in
`docs/openapi/section17e-parent-vendor-orders.openapi.json`: four customer and
eight vendor staff. The vendor surface uses `vendor.orders.read` and
`vendor.orders.manage`, both already seeded in Section 5, so Section 17E adds
no permission. Lifecycle commands require `If-Match`.

Enable with `COMMERCE_ORDERS_ENABLED=true`; config validation requires
`COMMERCE_QUOTE_LOCK_ENABLED`. The full model, including the allocation
attribution rules and the deliberate refusal to split an unattributable
vendor-funded promotion, is in
`docs/architecture/20_ORDER_ORCHESTRATION.md`. Section 17F now also exposes the
Wallet-gated commercial-change workflow: a material customer change creates a
new quote and requires explicit approval, while cheaper eligible delivery
outcomes receive one Wallet refund. Original quotes remain immutable.
