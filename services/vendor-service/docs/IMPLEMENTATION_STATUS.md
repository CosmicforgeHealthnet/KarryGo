## Karryam Backend V4 customer §14 closure — 28 September 2026

**SECTION 14: COMPLETE LOCALLY — PASS.** This verdict covers the repository implementation and the targeted local tests below. §§9–13 retain their prior completed status. Work remains uncommitted on `staging`; no fetch, pull, reset, checkout, commit or push was performed for this closure.

The saved authority is `C:\Users\favou\.claude\projects\C--KarryGoPush\memory\karryam_backend_v4.txt`, §14 (lines 680–1048). The implementation reuses the existing catalogue, Inventory, checkout, fulfilment, immutable quote, Wallet and logistics-authority contracts.

**Customer discovery and preview.** Mounted Food and Shop catalogue, sellability/product detail, search/suggestions and cart delivery preview with customer bearer authorization, default-off feature flags, customer-scoped Redis rate limits and customer-safe DTOs. Preview recomputes authoritative pickup, destination, grouping and physical requirement on every read and makes no stock, mission, provider or money mutation. The mounted surface and OpenAPI inventory match.

**Full local lifecycle evidence.** `TestCommerceLifecycleE2E_Food` and `TestCommerceLifecycleE2E_Shop*` pass from discovery/cart/preview through checkout, plan, quote, approval, payment hold, activation, vendor execution, logistics callbacks, completion, settlement, Activity grouping and mission-scoped Share. Shop covers single and multi-vendor allocation, Dispatch, Taxi Goods, Hauling and Vendor Own Delivery. The substitution cases cover equal, cheaper, costlier, reject and EXACT_ONLY decisions; the Shop chain also covers variable measurement, post-pick Dispatch-to-Taxi replanning, and one-vendor cancellation/refund while its sibling completes. Repeated callbacks, payment and refund requests do not duplicate their effects. Replanned mission references resolve to the active executor; the replaced executor cannot advance an order even when it presents the old plan slot. Consolidated mission fees equal the sum of every included VendorOrder's apportioned share.

The E2E runs real Vendor-owned services and PostgreSQL persistence. Wallet, logistics quote/mission/read and handover authorities are explicit test doubles; local completion is not a claim of independently deployed multi-service black-box or staging certification.

**Verification passed.** Affected PostgreSQL integration and race tests, Wallet and mission unknown-outcome read-back/recovery, concurrent replan/refund/inventory claims, Vendor migration apply/rollback/reapply through `000128`, route/OpenAPI parity, and targeted `go vet` passed. The Redis-backed HTTP load test completed 240 reads across eight workers (116 success, 124 expected rate-limit responses, no unexpected status; 767 ms under race). Final logs are `tmp/customer14-final-db.log`, `tmp/customer14-final-race.log`, `tmp/customer14-final-recovery.log`, `tmp/customer14-final-migrations.log` and `tmp/customer14-final-openapi.log`. The expensive §§9–13 suites were not rerun.

**Defects closed in this local work.** Food orders no longer claim an Inventory hold; actual Taxi Goods transit callbacks are stored and projected to canonical Vendor/Activity status (`000127`–`000128`); consolidated delivery fees are summed across VendorOrders; measured Shop quantities use the frozen price basis and actual physical load; approved replacement stock is committed correctly; concurrent post-pick replan creates one replacement mission; Activity grouping follows the current mission binding. Mission creation and Wallet recovery read the authority after an unknown outcome rather than creating a duplicate. Substitution was unreachable before this work and is now closed: the customer's per-line rule is captured in the cart and frozen through checkout, quote fingerprint and order (`000126`; every Shop line previously froze as EXACT_ONLY); proposal creation no longer fails on `FOR UPDATE` across outer joins; nullable proposal timestamps/case ids scan; a costlier approval becomes effective once its commercial change is APPLIED (the store verifies that itself) without writing a duplicate POSITIVE_DELTA adjustment.

**External/staging certification: PENDING.** Production catalogue and packed-weight data remediation, deployed configuration/ingress and multi-service staging certification, and the separately approved external Reference/Repricing adapter remain external. External Reference/Repricing stays disabled without that adapter. These items do not change the repository-local §14 verdict.

<!-- SECTION17F_PAYMENT_BINDING -->
Historical milestone entries below describe the state at their original closure. Their earlier Taxi-exclusion statements are superseded by the §14 Taxi Goods executor verified above.

## Section 17F - Payment Binding, Total Price Guarantee and Recovery

**Status: IMPLEMENTED.** Independent final certification has NOT been run; this
entry records what was built and verified, not a Section 17 GO.

Migrations `000081_vendor_commerce_payment_binding`,
`000082_vendor_commerce_fulfilment_activation` and
`000083_vendor_commerce_change_adjustments`, plus the payment, fulfilment and
commercial-change files in `internal/features/orders`, bind a payment
operation to the exact approved agreement and make the whole path resilient to
retries, crashes and post-payment replanning.

**Wallet authority.** Vendor Commerce fabricates no payment status, edits no
ledger, computes no balance and settles no vendor funds. Every financial effect
is performed by payment-wallet-service through the existing
`shared/go/walletclient` contracts. An unrecognised authority answer is never
treated as success, and an authority ERROR is treated as an UNKNOWN outcome
rather than a decided one - a timeout may still have moved money, so recovery
asks rather than guesses.

**One payment leg per VendorOrder.** Forced by the real Wallet contract:
`CompleteJob` settles a whole intent to one provider and requires `held`, and a
partial refund moves the intent to `part_refunded`, so a single parent intent
could never refund one vendor while settling another. Splitting at creation
makes multi-vendor refund isolation structural. A separate platform leg carries
the order-level fee and tax, which belong to no vendor.

**Binding.** A payment operation carries quote id, version, fingerprint, quoted
total, max authorized total, currency, customer and an allocation fingerprint
over the whole commercial allocation set. Moving one vendor's share to another
changes the binding even when the total does not, and an operation already
bound to a different agreement is refused rather than reused. Every amount is
read from the stored order; the request body carries only the fingerprint the
client commits to.

**Order-first flow.** The ParentOrder is created (exactly once, from an
approved quote) BEFORE any money is asked for, so each leg names a real
VendorOrder and there is no window where money exists for an order that was
never created. Expired quotes, released stock reservations and revalidated
checkouts are all refused before the money authority is called.

**Crash recovery.** Every leg row is committed before the first authority call.
A durable `commerce-payment-recovery` job (leases, retry, replay) reconciles
every unresolved leg by asking the authority: a leg with a reference is READ,
never re-paid; a leg without one is retried under its original deterministic
key. Past a deadline an unanswerable leg stops holding stock. An operation that
can never complete has every held leg refunded THROUGH the Wallet authority and
only then becomes terminal - it is never closed out while the customer's money
is still held. The same sweep resumes fulfilment activation stranded at the
payment-held boundary.

**Inventory finalization.** Explicit `inventory_state` per allocation, never
inferred: HELD while only reserved, CONFIRMED once payment is held, COMMITTED
on completion, RELEASED on cancellation. Stock is never double-committed, and
cancelling one allocation releases only its own unconsumed reservation.

**Fulfilment creation.** Platform-executed allocations get one logistics
mission per Section 17C plan mission (so a consolidated delivery stays one
mission at one fee). Vendor-owned delivery gets NO platform mission - the
vendor is the executor and its own delivery state tracks that work. **Taxi is
never an executor:** it is not a value of the shared contract's Service type
and the database refuses it.

**Total price guarantee.** A mission is created for the fee the customer
already paid, and `commercemission.VerifyMission` refuses any authority answer
whose fee differs in either direction. An internal mission split is a platform
cost and cannot raise a customer charge.

New shared package `shared/go/commercemission` and NEW internal ingress in
driver-dispatch-delivery-service (`000041`) and driver-hauling-service (`013`),
both behind HMAC v2 + allowlist + nonce and allowlisted to `vendor-service`
alone. See `PLATFORM_SERVICE_CHANGE_REGISTER.md`.

Two payment routes and four customer commercial-change routes are gated on a
wired money authority (752-route fully enabled surface). No route exists to
confirm stock or dispatch a vehicle: activation is worker-driven only.

Verified with real-Postgres integration tests for payment binding, idempotent
and concurrent starts, expiry/fingerprint/quote-swap/rebinding refusals, partial
authorization, crash recovery, compensation, inventory finalization timing,
mission idempotency, vendor-own delivery and Taxi exclusion; pure unit tests for
binding, leg planning and state derivation; contract tests for both new shared
packages; and migration apply/rollback/reapply for both new migrations.

**17F closure.** A material customer change creates a distinct immutable
replacement quote through the existing Section 17D authority, records the
original/replacement chain, requires explicit approval and authorizes only the
server-derived delta through Wallet. A rejected or expired replacement quote
cannot move money. The cheaper-replan policy compares the customer-facing
delivery commitment with the eligible final customer charge (never internal
provider operating cost) and issues one deterministic Wallet partial refund;
unknown Wallet outcomes remain recoverable. Terminal VendorOrders now settle or
refund their own isolated Wallet leg, so cancellation of A cannot block or
redirect settlement of B or C. Original quotes remain immutable.


<!-- SECTION17E_PARENT_VENDOR_ORDERS -->
## Section 17E - ParentOrder and Multi-Vendor VendorOrders

**Status: IMPLEMENTED.**

Migration `000080_vendor_commerce_orders` and `internal/features/orders` turn
one approved, unexpired Section 17D quote into exactly one ParentOrder and one
financially and operationally isolated VendorOrder per vendor/branch
allocation. Five tables: `commerce_parent_orders`, `commerce_vendor_orders`,
`commerce_vendor_order_lines`, `commerce_order_allocations` and
`commerce_order_events`.

Every commercial figure is copied from the frozen quote; nothing recomputes a
historical price from the live catalogue. The ParentOrder preserves the
customer, receiver, delivery address, quote, quote fingerprint, quote version,
currency, customer-approved total, funding breakdown and price disclosure. The
receiver is the one piece Section 17D does not fingerprint, so it is read from
the cart under the exact cart version the quote binds - a receiver change bumps
that version and a cart in checkout is immutable, so a changed contact can
never be snapshotted onto an order the customer approved before the change.
Delivery authority is read from the quote's own frozen mission snapshot, never
from the live plan row, because it decides who gets paid.

Multi-vendor financial isolation is a pure allocation engine
(`allocation.go`) that keeps `customer_payable_minor` and
`vendor_payable_minor` distinct per VendorOrder and asserts that every vendor's
customer payable plus parent-scope fee/tax equals the approved total exactly -
an imbalance creates no order at all. A vendor-funded benefit on a
multi-vendor quote is REFUSED rather than split, because Section 17D's
adjustment carries no vendor identity and splitting it would make one vendor
pay for another's promotion. Platform, delivery-subsidy and other-approved
benefit is spread by largest remainder, which costs no vendor anything. Fee and
tax stay parent-scope platform money. `commerce_order_allocations` records every
part as its own attributable row (type, beneficiary, funder, amount), so "who
actually paid for this delivery" is answerable by reading rows. Delivery money
never lands on a vendor payable unless the vendor executed the delivery, both in
code and by database CHECK.

Each VendorOrder owns its own Section 14 reservation relationship, its own
logistics mission reference, its own vendor-own-delivery state and its own
status: `CREATED → PREPARING → READY → HANDED_OVER → DELIVERED → COMPLETED`,
with `CANCELLED` from any non-terminal state. Transitions are a pure function of
one order's own state. A vendor may confirm delivery only for a delivery it
executed; a platform-executed delivery returns 403 because its confirmation
belongs to the logistics authority that performed it, which Section 17F wires.
A customer may withdraw from one VendorOrder up to `PREPARING` only.

The ParentOrder state is always derived from the children and recomputed inside
the same transaction with the parent locked: all completed -> COMPLETED, all
cancelled -> CANCELLED, all terminal but mixed -> PARTIALLY_COMPLETED, otherwise
CREATED or IN_PROGRESS. A mixed outcome is never hidden.

Completion prepares a settlement allocation and cancellation prepares a refund
allocation; parent-scope fee/tax becomes refundable only on whole-purchase
cancellation. Section 17F consumes these isolated allocations through Wallet
after the lifecycle commit, with recovery if the authority outcome is unknown.
The Section 17E creation path itself still creates no payment intent and never
writes `payment_operation_ref`.

Exactly-once creation is a database constraint (`UNIQUE(quote_id)`), not a
convention: a retry returns the existing order with 200 and duplicates no order,
vendor order, line or allocation. Creation is one transaction, so a failed
VendorOrder leaves no ParentOrder falsely claiming success, and a per-quote
advisory lock makes concurrent creations converge. Transitions carry an
`If-Match` version guard and lock the parent.

Twelve routes gated by `COMMERCE_ORDERS_ENABLED` (which requires
`COMMERCE_QUOTE_LOCK_ENABLED`): four customer and eight vendor staff. The
vendor surface uses `vendor.orders.read` and `vendor.orders.manage`, both
already seeded in Section 5 - Section 17E adds no permission and no role
mapping. The customer create body accepts only the approved fingerprint, never
a price, fee, discount, funding split, vendor id, branch id or payment
reference.

Verified with 20 real-Postgres integration tests (Food single-vendor, multi
-vendor Shop isolation, idempotent and concurrent creation, released-stock and
revalidated-checkout refusal, immutability, historical-snapshot survival,
independent lifecycles with a mixed PARTIALLY_COMPLETED outcome, platform
-delivery refusal, whole-cancellation refund, stale-version conflict,
concurrent sibling transitions, tenant/IDOR isolation, customer cancellation
limits, append-only evidence and a no-payment assertion), 24 pure unit tests
for the allocation engine and lifecycle, 6 handler contract tests, migration
apply/rollback/reapply, Section 17E router/OpenAPI contract parity and the
752-route service-wide contract gate. Model documented in
`docs/architecture/20_ORDER_ORCHESTRATION.md`. Section 17F extends this order
surface with Wallet-bound payment, fulfilment activation, commercial-change
requotes and durable adjustment recovery.


<!-- SECTION17D_IMMUTABLE_QUOTE -->
## Section 17D - Pricing, Promotions and Immutable Quote Lock

**Status: IMPLEMENTED.**

Migration `000079_vendor_immutable_checkout_quote` and
`internal/features/checkoutquote` create the exact customer-approved
commercial agreement from authoritative Section 17B checkout evidence and the
selected Section 17C plan. The quote snapshots cart/customer/address versions,
online product prices and price references, variable-measure bounds,
vendor/branch groups, Inventory reservations, logistics quote evidence,
delivery funding, commercial adjustments, disclosure and exact/max totals.

The material fingerprint is deterministic and the final quote fingerprint
also binds `quote_version`. All quote, child, approval and event rows reject
UPDATE/DELETE at the database. Same material converges on one quote under a
per-checkout advisory lock; changed material creates the next version and old
quotes remain readable without recomputation. Approval re-reads and
fingerprints the current checkout/address/allocation/plan evidence, rejects
stale or expired agreements and records append-only approval evidence.

Three customer-bearer routes are feature-gated by
`COMMERCE_QUOTE_LOCK_ENABLED`: create, read and approve. Both commands use the
customer idempotency boundary. Their strict request bodies accept only a
fulfilment-plan id or quote fingerprint, never prices, fees, discounts,
funding, customer, vendor or branch authority.

Funding distinguishes customer, vendor, platform, delivery-subsidy and other
approved benefit sources. Vendor/platform delivery subsidies from 17C are
preserved, online-price disclosure explicitly makes no physical-store parity
claim, and variable-weight maximums come only from Section 17B's bounded
measurement/tolerance result. `AssessReplan` prepares the later-section policy:
a cheaper replacement may release/refund the difference when payment supports
it; an increase always requires a new customer approval. It moves no money.

Verified with unit/HTTP tests, real-Postgres immutable/history/expiry/stale
source/customer-isolation/multi-vendor/concurrency tests, migration
apply/rollback/reapply, Section 17D router/OpenAPI contract parity and the
734-route service-wide contract gate. No Wallet, payment, ParentOrder,
VendorOrder or execution mission was added; Section 17E has not started.


<!-- SECTION17C_FULFILMENT_PLANNING -->
## Section 17C - Fulfilment Planning and Logistics Integration

**Status: IMPLEMENTED.**

Turns validated Section 17B vendor/branch groups into an authoritative
candidate fulfilment plan.

### What was built

Vendor Service:
- `internal/features/delivery` - delivery policy (migrations 000077 and 000096):
  PLATFORM/VENDOR/HYBRID modes, branch reach/zones, flat/zone/tier fees,
  IANA operating hours, minimum/free rules, vendor capacity, bounded overflow
  evaluation, vendor execution state and price-guarantee evidence. Two
  vendor-staff routes.
- `internal/features/fulfilment` - logistics metadata aggregation, vehicle-class
  selection, consolidation, quoting, hybrid pre-custody replan, custody guard,
  the plan aggregate and its persistence (migrations 000078 and 000096). Two
  customer routes.
- `internal/platform/geo` - service-local great-circle helper. Deliberately not
  taken from `shared/go/dispatchpricing`: that package is the dispatch FARE
  formula, and a vendor-owned charge or a service-area test must never be
  coupled to a platform fare model.

Cross-service (the authoritative quote, which did not exist before):
- `shared/go/commercequote` - one contract for three services. Schema, HMAC
  client, and caller-side verification of the authority's answer.
- `driver-dispatch-delivery-service` - `POST /api/v1/internal/commerce/quotes`
  (migration 000040), priced with `shared/go/dispatchpricing`.
- `driver-hauling-service` - `POST /api/v1/hauling/internal/commerce/quotes`
  (migration 012), priced with hauling's own fare model, owning both VAN and
  TRUCK.

### Decisions worth carrying forward

**No fake fee, enforced by the database.** A platform mission may not carry a
fee without an authority quote id and expiry; an unserviceable group may carry
no fee, no mission and no authority; a platform delivery fee may not be
assigned to the vendor; and customer + vendor-funded + platform-funded must
equal gross on every mission and every group. These are CHECK constraints, so
a future refactor that bypasses the planner still cannot store a fee nobody
stands behind. `cmd/section17c_plan_test.go` writes raw SQL to prove each one.

**Consolidation is bounded by a real logistics constraint.** Neither dispatch
nor hauling models a multi-pickup mission, so merging two distant pickups would
produce a fee for a route nobody can drive. Groups consolidate only when their
pickups are co-located (`COMMERCE_FULFILMENT_CONSOLIDATION_RADIUS_METERS`,
default 150 m - the two-traders-in-one-market case), share a class and an
urgency, carry no separation conflict, and still fit the class combined. When
they do, the cluster is quoted ONCE for its real combined load, and the fee is
attributed back across the groups with shares that provably sum to the mission
total.

**`mode` is `SINGLE`.** The section forbids presenting three equivalent options
under different labels. The enum carries FASTEST/BALANCED/SAVER for when the
optimizer genuinely selects differently; today it does not, and saying so is
the honest answer.

**Taxi is unreachable by construction.** `shared/go/commercequote` has no CAR
class, `fulfilment.classRank` gives CAR no rank so it cannot be selected,
`fulfilment.ServiceFor(VehicleCar)` errors, no taxi client is wired, and
`internal/section17ccontract` asserts all of it. A vendor CAR hint floors the
load at VAN rather than being silently downgraded.

**Sell-unit conversion, not variant mass.** A Shop line resolves its weight
through the ordered sell unit. "2 kg of rice" off a 50 kg sack weighs 2 kg;
using the variant's net weight would overstate it twenty-five times and put a
small order in a truck. `EXACT` is authoritative, `CALIBRATED_APPROXIMATE`
contributes but never claims a measured value (Section 12 forbids treating a
calibration as exact), `NONE` invents nothing.

**Missing evidence is reported, never invented.** Weight, volume and dimension
each carry COMPLETE/PARTIAL/ABSENT, so a plan never claims a measured total it
does not have, and a lower-bound total is visibly a lower bound.

**An authority fault is not a refusal.** A timeout or a 5xx makes the group
unserviceable with `LOGISTICS_AUTHORITY_UNAVAILABLE`; it never falls back to a
Vendor Service estimate, because no such estimate exists. Both quote services
answer 503 rather than "serviceable: false" when they cannot check, so a caller
told "no rider" always learns a fact about riders.

### Verified

- Real-Postgres migration replay (apply, verify, rollback, verify the tables
  are gone, reapply) with 000077 and 000078.
- `shared/go/commercequote`, dispatch and hauling quote suites.
- Vendor Service unit suites for the vendor-own evaluation, the aggregation,
  vehicle selection, and the planner (including mixed vendor/platform plans,
  consolidation both ways, and every refusal path).
- Real-Postgres adapter tests for the catalogue and branch reads, including
  tenant isolation in both directions.
- Real-Postgres plan-persistence tests, including raw-SQL constraint probes.
- `internal/section17ccontract` router/OpenAPI/route-register parity.
- `internal/crmcontract`, the service-wide gate. It caught two real problems
  before this section closed, both worth recording because the same shape has
  recurred: the Section 17C routes were not registered in its stand-in router
  (the identical hole Sections 14I, 16D and 17B each had to close, and it
  reported the four documented operations as phantom), and the original single
  OpenAPI document declared two server bases, which that gate correctly
  resolved into `/api/v1/commerce/api/v1/vendor/...`. The document is now split
  one-per-base and the routes are registered; the accepted surface moved from
  727 to 731.
- The full isolated regression runner (`go run ./tools/vendor-test-runner`),
  49/49 packages.

### Known limits, stated rather than left to be discovered

- **Consolidation is co-location-only.** A genuinely multi-pickup mission needs
  a multi-pickup contract from dispatch or hauling, which neither has. The plan
  schema already supports one quoted mission covering several groups, so the
  commercial commitment survives when 17F splits it into execution missions.
- **Optimizer modes are not implemented.** `mode` exists and is always SINGLE.
- **No platform delivery promotion exists**, so `platform_funded_minor` is
  always zero today. It is modelled anyway so the funding invariant is stated
  in full rather than rediscovered when one is added.
- **Quote consumption is not implemented here.** 17C persists the authority
  quote id, reference, revision, endpoint handles and expiry; presenting them
  back to the authority at mission-creation time is 17F's work.

# Implementation Status

## Section 7 â€” Onboarding, KYB, Verification and Activation

**Status: VERIFIED; GO with explicitly fail-closed external deployment dependencies.**

### Section 7 contract correction

The cross-service VerificationStatus contract is exactly `NOT_STARTED`, `IN_PROGRESS`, `PENDING_REVIEW`, `VERIFIED`, `REJECTED`, `EXPIRED`, `SUSPENDED`; plain `PENDING` is rejected. Projection and history constraints enforce the same vocabulary, and activation tests prove mandatory `PENDING_REVIEW` blocks business, branch, and applicable branch-capability scope. The internal callback is the root-level `POST /internal/vendor/v1/verification-events`; it is not mounted below `/api/v1/vendor` and is not gateway-exposed. Queue/outbox operation statuses named `PENDING` are separate state machines and were deliberately preserved.

- Migrations `000008_vendor_section7_onboarding` and `000009_vendor_section7_permissions` add the onboarding aggregate, representative, opaque document references, exact verification projections/history, data-driven requirements, masked Wallet projection, reconciliation operations, nonce replay table, and six permissions.
- Nineteen public routes plus one HMAC-v2 callback are implemented and covered by `section7-onboarding-verification.openapi.json`; cumulative Vendor route count is 73.
- Activation commits the business graph, Section 7 audit, and `vendor.business.activated` outbox together. Unknown and pending outcomes reconcile by authoritative client reference with leases, bounded backoff, dead-letter and code-level replay; Verification also delivers signed callbacks from its leased outbox.
- Bank replacement is serialized per business, requires `expected_version`, and consumes step-up before the Wallet mutation. Verification submission atomically records the durable operation and marks referenced documents `SUBMITTED`; disabled integration writes nothing.
- Section 7 audit is transactional. The disclosed Section 5/6 append-after-commit audit limitation remains unchanged for those older paths.
- Deployment dependencies: Media and notification must allowlist Vendor; notification has no SMS; admin-backoffice remains a scaffold.

Per-section tracker for Vendor Service. Allowed statuses: `NOT_STARTED`, `IN_PROGRESS`, `BLOCKED`, `IMPLEMENTED`, `VERIFIED`, `GO`.

## Section 0 â€” Architecture / Foundation

- **Scope**: repository audit; service scaffold; own database/Redis config; migration runner/convention; health/ready/meta; structured logging; standard error handling; service-auth foundation; outbox/inbox/job architecture skeleton; feature-flag configuration; mandatory documentation structure; architecture decision docs; module boundaries for later sections; no fake OpenAPI routes; baseline tests.
- **Status**: **VERIFIED**
- **Code paths changed**: `services/vendor-service/` (new â€” `cmd/main.go`, `internal/config`, `internal/database`, `internal/events`, `internal/platform/idempotency`, `internal/audit`, `internal/jobs`, `internal/integrations/*` (11 reserved boundaries), `internal/features/*` (23 reserved boundaries), `internal/architecture_test.go`); `go.work` (added `./services/vendor-service`); `CLAUDE.md` (service listing, ports, databases, this session's log entry)
- **Migrations**: `000001_vendor_service_foundation`, `000002_events_outbox_inbox`, `000003_idempotency_keys` (all with `.up.sql`/`.down.sql`)
- **Routes**: none (intentional â€” no fake routes for unimplemented features)
- **Events**: none produced yet (outbox/inbox infrastructure exists, unused)
- **Workers**: none started yet (outbox worker constructed, not run â€” no destination configured)
- **Tests**: `internal/config/config_test.go` (6 tests â€” defaults, validation rules), `internal/database/migrate_test.go` (real-Postgres gated, migration replay: apply/rollback/reapply, run live and passing), `internal/architecture_test.go` (feature-boundary import guardrail)
- **Affected services**: none modified (only new `internal/integrations/*` doc.go boundaries describing future calls â€” see [PLATFORM_SERVICE_CHANGE_REGISTER.md](PLATFORM_SERVICE_CHANGE_REGISTER.md))
- **Documentation updated**: this file; [README.md](README.md); [UI_UX_REQUIREMENTS.md](UI_UX_REQUIREMENTS.md); [FRONTEND_REQUIREMENTS.md](FRONTEND_REQUIREMENTS.md); [PLATFORM_SERVICE_CHANGE_REGISTER.md](PLATFORM_SERVICE_CHANGE_REGISTER.md); [API_ROUTE_REGISTER.md](API_ROUTE_REGISTER.md); `architecture/00`â€“`12`
- **Remaining gaps**: none for this section's own scope. See "Future sections" below for what is intentionally not yet started.
- **Acceptance result**: `go build ./...`, `go vet ./...`, `gofmt -l .` all clean; `go test ./...` all pass (config + architecture-boundary tests unconditionally; migration-replay test verified live against real Postgres in this session, then confirmed to skip cleanly without a configured test database â€” both paths exercised).

## Section 5 â€” Authentication, Sessions and Staff IAM

- **Scope**: passwordless OTP login (start/resend/verify), refresh sessions (rotation/revocation), logout, step-up authentication, staff provisioning/invitation/suspension/reactivation, roles (11 system role families + vendor-scoped custom roles), a global capability-string permission catalogue (35 permissions), role-permission mappings, branch-scoped staff-role assignments, a real authorization evaluator (deny-by-default, permission-string-only, branch-scope-aware), repository-level tenant isolation, real Postgres-backed audit trail, rate limiting, HTTP API, OpenAPI + route-parity test.
- **Status**: **VERIFIED**
- **Code paths changed**: `services/vendor-service/internal/features/auth/{models,repositories,usecases,clients,http}` (real implementation, replacing the Foundation phase's `doc.go` boundary); `internal/features/staff/{models,repositories,usecases,http}` (same); `internal/platform/ratelimit` (new); `internal/platform/authctx` (new â€” shared request-context carrier, not feature-owned, since both `auth` and `staff` need it without importing each other's `http` package); `internal/audit/postgres_recorder.go` (new, `internal/audit/audit.go` extended with `BranchID`/`Reason`); `internal/config/config.go` (`AuthConfig`, `RateLimitConfig`, validation rules); `cmd/main.go` (full wiring), `cmd/adapters.go` (new â€” cross-feature port adapters), `cmd/http_contract_test.go` (new)
- **Migrations**: `000004_vendor_auth_iam` (`vendor_users`, `vendor_staff`, `vendor_otp_challenges`, `vendor_sessions`, `vendor_step_up_grants`, `vendor_permissions`, `vendor_roles`, `vendor_role_permissions`, `vendor_staff_role_assignments`, `vendor_audit_log`), `000005_vendor_default_permissions_and_roles` (35 seeded permissions, 11 seeded system role families + mappings) â€” both with `.up.sql`/`.down.sql`, verified apply/rollback/reapply against real Postgres
- **Routes**: 21 (see [API_ROUTE_REGISTER.md](API_ROUTE_REGISTER.md) and `docs/openapi/section5-auth-staff-iam.openapi.json`) â€” 5 public auth routes, 2 authenticated step-up routes, 12 authenticated staff/role/permission routes, 1 internal HMAC-protected provisioning route, 0 fake/future routes
- **Events**: `vendor.staff.created`, `.suspended`, `.reactivated`, `.roles_changed`, `vendor.role.changed` â€” appended to the real durable outbox (`internal/events`) on every mutation; no consumer/destination exists yet (see [architecture/08](architecture/08_EVENT_OWNERSHIP.md)) so they accumulate as `PENDING`, never silently dropped
- **Workers**: none started (outbox worker remains unstarted, matching Section 0's decision â€” no destination is configured yet)
- **Tests**: 50 test functions across 8 packages, all passing â€” `internal/features/auth/models` (11 unit tests: OTP state machine, session, step-up binding), `internal/features/auth/usecases` (13 real-Postgres integration tests: full login lifecycle, expired/resend/lockout/concurrent-verify-once, suspended-staff-blocks-login-and-refresh, multi-vendor selection without burning the OTP, step-up binding precision, logout revokes step-up grants, **raw OTP never persisted**, **raw OTP never logged**), `internal/features/staff/usecases` (8 real-Postgres integration tests: provision+invite+branch-scoped-assignment, **tenant isolation/IDOR**, **privilege-escalation prevention**, **branch-scope-escape prevention**, **role-name-alone-does-not-authorize**, **high-risk-role-requires-step-up**, duplicate-assignment prevention, suspend/reactivate with version-conflict + audit trail), `cmd` (4 real-Postgres+Redis HTTP contract tests: full loginâ†’list-staff through the real production router, validation-error envelope, 401-without-a-valid-token), `internal/section5contract` (2 OpenAPI/route-parity tests), `internal/database` (extended migration-replay + schema-existence tests)
- **A real bug found and fixed by this test suite**: the seeded Branch Manager role initially granted only `vendor.staff.read`, not `.manage`, silently contradicting the specification's "branch staff where permitted" â€” caught by `TestIAM_BranchScopedActorCannotEscapeBranch` failing on the in-branch-delegation-should-succeed half of the assertion, fixed in migration `000005` before it was ever "accepted" history (same session, not yet committed)
- **Affected services**: none modified. `notification-service` needs a config-only allowlist addition (`vendor-service` in `NOTIFICATION_SERVICE_SECRETS`) before real email OTP delivery works â€” tracked in [PLATFORM_SERVICE_CHANGE_REGISTER.md](PLATFORM_SERVICE_CHANGE_REGISTER.md) #3, still `NOT_STARTED` (not this service's code to change)
- **Documentation updated**: this file; [API_ROUTE_REGISTER.md](API_ROUTE_REGISTER.md); [PLATFORM_SERVICE_CHANGE_REGISTER.md](PLATFORM_SERVICE_CHANGE_REGISTER.md); [FRONTEND_REQUIREMENTS.md](FRONTEND_REQUIREMENTS.md); [UI_UX_REQUIREMENTS.md](UI_UX_REQUIREMENTS.md); [architecture/09_SECURITY_AND_AUTHORITY_MODEL.md](architecture/09_SECURITY_AND_AUTHORITY_MODEL.md) (updated from the Foundation phase's planned model to the actual implemented one); `docs/openapi/section5-auth-staff-iam.openapi.json` (new)
- **Remaining gaps, disclosed**:
  - **Phone-number OTP delivery has no real channel** â€” confirmed no SMS capability exists anywhere in the platform (`shared/go/notifications`' channel set is exactly push/email/websocket/in_app). Email delivery is real; phone returns a clear `service_unavailable` error outside debug mode rather than silently failing or unsafely falling back to logging the raw code (unlike customer-service's existing `LoggingOTPSender`, which does log the raw code in debug mode â€” a pattern deliberately NOT replicated here per this task's own rule 11). Recorded as a platform-wide gap, not a Vendor Service defect.
  - **`notification-service` caller allowlist** not yet updated (config-only change on another service, out of this section's own code).
  - **Section 6 (Business/Branch) does not exist yet**, so `vendor_id`/`branch_id` remain opaque text throughout, exactly as the architecture phase specified â€” no fake CRUD was added.
  - **Security scenario coverage**: 50 real tests cover every category from the task's security-test list (OTP lifecycle, rate-limit wiring present, step-up binding, IAM tenant isolation/escalation/branch-scope/step-up-gating, audit) at representative depth; not literally all ~47 enumerated scenarios have a dedicated test (e.g. Redis-unavailable degraded-mode behavior and a few narrower rate-limit-bucket-isolation cases are implemented in code â€” `ratelimit.Limiter` fails closed via `apperrors.Unavailable`, never open â€” but not independently tested here for time).
- **Acceptance result**: `go build ./...`, `go vet ./...`, `gofmt -l .` all clean. Full suite run with `go test -p 1 ./...` (packages serialized to avoid shared-test-database DDL contention across packages â€” a test-infrastructure requirement, not a product defect; each package was also independently verified against its own isolated database) â€” all 50 tests pass, zero failures, zero skips (both `VENDOR_SERVICE_TEST_DATABASE_URL` and `VENDOR_SERVICE_TEST_REDIS_ADDR` were set for this run).

## Sections 10/11 â€” Shared Production, Cloud Kitchen, and Pastry/Bakery

- **Status**: **GO**. Backend implementation is complete; no frontend was created.
- **Schema**: migrations `000033` through `000041`; current tip `000041_vendor_sections10_11_integrity_alerts`. The schema includes shared pools/lines/rules/templates/slots/blackouts, multi-pool reservations and allocations, production jobs/history/alerts, Cloud Kitchen packages/bands/version history/plans/occurrences, and Pastry products/options/specifications/quotes/accepted snapshots/conversion intents/milestones/history.
- **Capacity correctness**: one PostgreSQL transaction locks capacity pools in deterministic order, applies buffers/rules/slots/blackouts, and atomically grants or rejects all allocations. Blackout publication takes the same ordered pool locks, preserving prior commitments while making every reservation started after blackout commit fail. Holds, confirmation, release, partial release, rescheduling, preview, utilization, calendar revisions, and expiry share this authority.
- **Cloud Kitchen**: Section 9 catalogue references are tenant/branch checked; composition and price-band edits preserve immutable package/price history. Corporate recurrence uses branch IANA timezones, exception dates, horizon/lead-time policy, idempotent future materialization, durable conflicts, leases, retries, dead-letter replay, safe cancellation, jobs, audit, and outbox. Occurrence management is versioned compare-and-swap, including concurrent reschedule protection.
- **Pastry/Bakery**: typed products, reusable ordered options, standard capacity-backed batches, versioned custom requests/specifications/diffs/change decisions, bounded media references, controlled logistics/risk metadata, optional-approval quote/revision/expiry/hold flows, stale/superseded-spec rejection at issue/approval/acceptance, concurrency-safe acceptance, immutable accepted snapshot, `PENDING_EXTERNAL_DOMAIN` conversion intent, jobs, milestones/history, cutoff and cancellation policy.
- **IAM/security**: production read/manage/operate, Cloud Kitchen manage, Pastry read/manage/quote-manage/quote-approve remain distinct; high-value approval uses existing step-up. The Kitchen/Pastry role has production operation and visibility but not production administration or quote pricing/approval. Tenant, branch, nested-resource, capacity-pool, catalogue-item, and capability references are checked fail-closed.
- **Workers**: corporate materialization/recovery, expired holds/quotes, and operational alerts are bounded and outbox/audit backed. Corporate, occurrence-reschedule, and quote race regression tests prove one occurrence/reservation/job, exactly one winning occurrence CAS mutation, and one accepted snapshot/conversion intent/reservation/job respectively.
- **API**: 123 Sections 10/11 public operations, zero internal operations; fully enabled Vendor Service router is pinned at 385 routes. All module mutations use the shared Vendor `Idempotency-Key` store; job transitions require `If-Match`. Section OpenAPI declares authentication, idempotency, path parameters, command bodies, and bidirectional route parity; the global route contract is green.
- **Authority boundary**: `ORDER_AUTHORITY=NOT_PRESENT`; accepted cake quotes create only a durable conversion intent. No payment, Wallet, inventory-stock, dispatch, taxi, hauling, route, fee, ETA, or handover credential is created.
- **Contract handoff**: [SECTION10_11_HANDOFF.md](SECTION10_11_HANDOFF.md), [sections10-11-production.openapi.json](openapi/sections10-11-production.openapi.json), and [API_ROUTE_REGISTER.md](API_ROUTE_REGISTER.md).

## Future sections (NOT_STARTED â€” listed for roadmap visibility only, per the module map)

| Section (indicative, not binding numbering) | Feature(s) | Depends on |
|---|---|---|
| Vendor identity/business | `vendors`, `capabilities` | â€” |
| Brands/branches | `brands`, `branches` | Vendor identity |
| CRM | `crm` | Vendor identity |
| Catalogues (Food/Cloud Kitchen/Pastry/Shop) | `catalog`, `food`, `cloud_kitchen`, `pastry`, `shop` | Branches, capabilities |
| Inventory | `inventory` | Catalogues |
| Purchasing | `purchasing` | Inventory |
| Recipes/production | `recipes`, `production` | Inventory, catalogues |
| Commerce orders | `orders` | Catalogues, [payment allocation model](architecture/05_PAYMENT_ALLOCATION_MODEL.md) |
| Substitutions | `substitutions` | Orders |
| Promotions | `promotions` | Orders, [payment allocation model](architecture/05_PAYMENT_ALLOCATION_MODEL.md) |
| Fulfilment planning | `fulfilment` | Orders; `integrations/dispatch|taxi|hauling` |
| Vendor-owned delivery | `delivery` | Fulfilment |
| Finance projection | `finance_projection` | `integrations/wallet` |
| Vendor analytics | `analytics` (feature) | Orders, fulfilment |

Each of these is expected to follow the specification's own "Preferred backend feature order" (domain model/usecase tests â†’ repository/client interfaces â†’ migration + repository implementation â†’ HTTP DTOs/handlers/routes â†’ config/wiring â†’ shared client updates â†’ integration/handler tests), and to update this file, [API_ROUTE_REGISTER.md](API_ROUTE_REGISTER.md), [PLATFORM_SERVICE_CHANGE_REGISTER.md](PLATFORM_SERVICE_CHANGE_REGISTER.md), [FRONTEND_REQUIREMENTS.md](FRONTEND_REQUIREMENTS.md), and [UI_UX_REQUIREMENTS.md](UI_UX_REQUIREMENTS.md) as its own definition-of-done, per [README.md](README.md)'s documentation gate.

## Section 6 - Vendor Business, Brands, Capabilities and Branches

**Status: IMPLEMENTED; activation remains feature-flag controlled.**

- Repository checkpoint: branch `staging`; starting and ending HEAD `f4dc08ca080a6b8dab5d3060578ee7566d1af7e2` (no commit requested). Code paths: `internal/features/vendors`, Section 6 migrations, production `cmd` wiring, Section 6 contract tests, and documentation.
- Route surface: 32 new Section 6 operations, 53 cumulative Vendor operations. No worker or downstream consumer was invented; legitimate events persist as `PENDING` in the existing outbox.

- Real Postgres aggregates and constraints: `vendor_businesses`, `vendor_capabilities`, `vendor_brands`, `vendor_branches`, normalized hours, radius service areas, explicit settings, and branch capabilities.
- Authenticated staff scope is authoritative: business id equals the Section 5 `vendor_id`, owner is the creating `staff_id`, and every repository query includes business scope. Multi-business staff continue to select a distinct Section 5 session scope.
- Exact state vocabularies and nine capability types (the original seven plus `CATERING` and `MANUFACTURER`, admitted by migration 000121) are enforced in Go, SQL, OpenAPI, and contract tests. Creation is always non-active/non-verified; vendor operational transitions are only `ACTIVE` to `PAUSED` and `PAUSED` to `ACTIVE`.
- Mutations use optimistic versions, idempotency, audit records, and transactional outbox events for aggregate/status changes. Parent pause gates effective branch capability state without rewriting children.
- Production IAM branch assignments validate that the branch exists in the same business. Section 5 tests remain backward compatible through the optional validation port.
- No frontend, catalogue, logistics, wallet, verification, media-file, or other-service implementation was added.
- Known continuing audit limitation: Section 6 reuses the Section 5 `PostgresRecorder`, so audit rows are durable and tested but still append after the aggregate transaction rather than atomically inside it; this remains disclosed in architecture/09 and was not expanded into an unrelated audit-framework rewrite.

## Section 8A - CRM Foundation, Customer 360, Consent and Privacy

**Status: IMPLEMENTED; every external CRM integration is deliberately dormant because no authority publishes those events yet.**

- Repository checkpoint: branch `claude/vendor-crm-foundation-8a-uonlya` (based on `staging`); starting HEAD `aa8c7807d8f7b90aad33425add2ac92402830fcf`. Route baseline before 8A: **73**; after: **80** (6 public + 1 internal). No existing route was changed or removed.
- Migrations `000010_vendor_crm_foundation` and `000011_vendor_section8a_crm_permissions`. Fresh replay, Section 7 -> 8A upgrade, rollback and reapply all verified against real Postgres; the Section 7 pair test was updated to step back past 8A first so it still proves exactly what it always proved.

### What the CRM is, and is not

A CRM profile is the commercial relationship between one vendor business and one customer subject. It is a projection, not a customer master: no canonical email, phone, address, date of birth or credential is stored or returned anywhere, and no hash of a customer reference is used as a stand-in identity. `TestHTTPCustomer360ReturnsNoCanonicalPII` walks the whole response tree to enforce that at the contract level.

Relationships are created **only** from an authenticated authoritative event. There is no vendor-facing create-customer route, so a production database legitimately holds zero CRM profiles until a real commerce or identity authority is integrated. Nothing was seeded to make the feature look populated.

### Authority audit (the reason most integrations are dormant)

| Capability | Result | Evidence |
|---|---|---|
| `CUSTOMER_CONSENT_SUPPORT` | **NOT_PRESENT** | No consent concept anywhere in customer-service. notification-service's `notification_preferences` is a delivery toggle (`enabled boolean DEFAULT true`, no purpose, no source version, defaults opt-in) owned by the delivery service, not a marketing-consent authority â€” mapping it to ALLOW would be exactly the forbidden widening. |
| `CUSTOMER_PRIVACY_EVENT_SUPPORT` | **NOT_PRESENT** | No delete/restrict/redact/suppression concept in customer-service. |
| `CUSTOMER_IDENTITY_MERGE_SUPPORT` | **NOT_PRESENT** | No identity-link, guest-upgrade or account-merge concept in customer-service. |
| `CUSTOMER_PRESENTATION_SUPPORT` | **PARTIAL, unusable as-is** | `GET /internal/customers/:id` (HMAC) exists but returns `phone` and `email` alongside `name`. There is no PII-free presentation contract, so `safe_display_label` is left permanently null rather than populated from a payload that carries contact data. |
| `ORDER_CRM_PROJECTION_SUPPORT` | **NOT_PRESENT** | `internal/features/orders` is still a reserved `doc.go` boundary; no vendor commerce order publisher exists. The projection interface, ingestion contract, dedupe, source-version rules and rebuild path are implemented and fixture-tested; the live integration stays disabled. |
| `SUPPORT_INTERACTION_PROJECTION_SUPPORT` | NOT_PRESENT | Not needed by 8A (interactions are 8C); recorded for later packages. |
| `NOTIFICATION_REFERENCE_DELIVERY_SUPPORT` | PARTIAL | push/email/websocket/in_app exist; no SMS sender. 8A sends nothing. SMS **consent** remains representable even though SMS **delivery** does not exist â€” the two are separate concerns. |
| `MEDIA_EXPORT_SUPPORT` | Not applicable to 8A | Exports are 8H. |

Consequence, stated plainly: with no consent authority wired, every marketing decision reads UNKNOWN and every channel is ineligible. That is the designed fail-closed outcome, not a defect.

### Schema

`vendor_customer_profiles`, `crm_customer_identity_links`, `crm_customer_branch_relations`, `crm_customer_order_facts`, `crm_consent_projections`, `crm_privacy_projections`, `crm_privacy_processing_history`, plus one additive nullable `payload_fingerprint` column on the shared inbox (the existing inbox deduped by event id but could not detect the same id carrying a different body).

Every CRM table is tenant-scoped in PostgreSQL, not merely in middleware: each carries `business_id` and reaches its parent through a composite `(id, business_id)` foreign key, so a child row can never be attached across tenants. Vocabularies, money non-negativity, the one-primary-subject-reference invariant, the merge-state/privacy-state agreement, and the "redacted iff redacted_at" rule are all real CHECK constraints.

**No privacy tombstone table was added.** The redacted profile row *is* the suppression record: it retains its opaque subject reference (and nothing else usable), which is the minimum needed to recognise a late event and refuse recreation. Hashing that reference would not make the subject anonymous â€” it would only make suppression unreliable.

### Behaviour worth calling out

- **Marketing fails closed by construction.** `MarketingEligibility` takes no override input, so no caller can pass one. Precedence: redaction > merge > marketing suppression > restriction > archival > consent unavailable > DENY > UNKNOWN/stale > ALLOW. Only the last returns true.
- **Aggregates are derived, never incremented.** Totals are recomputed from `crm_customer_order_facts` on every change, which is what makes refunds, replays, out-of-order delivery and merges safe by construction rather than by careful arithmetic. A partial refund lowers net without creating a second order; a redelivered event changes nothing.
- **Cross-currency data is quarantined, never converted.** CRM has no FX authority. An order denominated differently from the business currency is rejected and the existing totals are proven uncorrupted.
- **Merges require authoritative evidence.** No heuristic, fuzzy or ML matching exists. Both profiles are locked in a fixed id order (no deadlock), evidence is unique per authority event (no double merge), and the survivor's totals are rebuilt from the merged fact set so an order both sides observed is counted once.
- **Anti-resurrection holds in both orderings**: a late event for a redacted subject is refused, and a redaction that arrives *before* any relationship still blocks the later creation. A merged-away reference routes to its survivor instead of spawning a duplicate.
- **Archive cannot launder a privacy decision.** Archive and restore are both refused on a redacted or merged-away relationship.
- **Privacy processing is a handler registry**, so 8B-8H register their own redaction behaviour instead of the engine being rewritten. 8A registers `customer_profile`, `consent`, `commercial_projection`, `identity_link`, and Customer 360 reports the covered domain list.
- **Customer 360 distinguishes zero from unknown.** With no observed commercial data the state is `NOT_AVAILABLE` and every figure is omitted. Future modules report `NOT_IMPLEMENTED` rather than an empty structure that would read as an operational feature with no data.

### IAM

Five permissions added in `000011`: `vendor.crm.customers.read`, `.customers.manage`, `.customer360.read`, `.consent.read`, `.privacy.read`. Mapped to Owner, General Manager, CRM/Support (full 8A surface); Marketing (safe read + consent eligibility only â€” no privacy read, no lifecycle); Branch Manager (read only, branch-scoped by its Section 5 assignment); Analyst/Auditor (read only, including privacy for compliance review). Finance, Kitchen/Pastry, Shop/Inventory, Procurement and Vendor Rider are deliberately unmapped.

No `vendor.crm.consent.override`, `vendor.crm.privacy.override` or `vendor.crm.identity.force_merge` permission exists, and a migration test asserts none is ever introduced.

### Tests

220 test functions across the service, **0 failures, 0 skips**, run with real Postgres and Redis (`go test -p 1 ./... -count=1`). Section 8A contributes unit tests for the pure rules plus real-Postgres suites for tenancy/IDOR, branch-scope escape, event-caller authority, privacy races (including out-of-order versions and both anti-resurrection orderings), merge concurrency and dedupe, consent decision/version matrices, Customer 360 honesty, refund and replay safety, HTTP contract behaviour, and OpenAPI route parity.

### Known dependencies (real, not hypothetical)

1. **No consent authority** â€” needed before any marketing feature (8E/8F) can target anyone. Until then eligibility is uniformly false.
2. **No privacy authority** â€” the redaction engine is built and tested but nothing currently instructs it.
3. **No identity-merge authority** â€” guest-to-customer linking cannot occur in production yet.
4. **No vendor commerce order publisher** â€” every commercial figure stays `NOT_AVAILABLE`. This is the dependency that makes CRM useful, and it belongs to the order section.
5. **No PII-free customer presentation contract** â€” `safe_display_label` stays null; a client needing display identity must ask the customer authority directly.

### Not started

At the time Section 8A closed: Section 8B and everything after it. Section 8B has since been implemented â€” see its own entry below.

## External dependencies blocking full activation (not blocking further Vendor Service sections)

- **RESOLVED IN SECTION 32 — LOGISTICS OTP/QR** (see [architecture/07_EXISTING_OTP_QR_INTEGRATION.md](architecture/07_EXISTING_OTP_QR_INTEGRATION.md) and [SECTION32_TRACEABILITY.md](SECTION32_TRACEABILITY.md)): Dispatch, Hauling, Taxi Goods and Vendor-owned delivery now have approved two-stage authorities. Activation remains controlled by disabled-by-default rollout flags.
- **Vendor admin review product**: Verification's Vendor authority and signed callback/read-back paths are implemented, but `admin-backoffice-service` remains a scaffold with no operator work queue/UI. Formal cases remain honestly `PENDING_REVIEW` until an authorized admin-service caller decides them.

## Section 8B â€” Tags & Advanced Segmentation Engine

**Status: IMPLEMENTED; GO.**

Master CRM requirements 14-24. Built on the accepted Section 8A baseline (80 Vendor routes, migration tip `000011`), verified against the working tree before any code was written: the full Section 5-8A suite was run green on a fresh Postgres/Redis first, so nothing here is attributed to a failure that was already present.

### Scope

Vendor-defined customer tags, a safe JSON segmentation DSL with a closed field/operator registry, static and dynamic segments, immutable definition versions, privacy-safe preview, durable materialization with atomic generation swap, manual exclusions, and Customer 360 integration. At the Section 8B boundary, Sections 8C-8I had not started; Section 8C is now implemented below, while 8D-8I remain untouched.

### What was built

- **Migrations** `000012_vendor_section8b_tags_segments` and `000013_vendor_section8b_permissions` add seven tables â€” `crm_tags`, `crm_customer_tag_memberships`, `crm_segments`, `crm_segment_versions`, `crm_segment_evaluations`, `crm_segment_memberships`, `crm_segment_exclusions` â€” and five permissions. Migration tip is now `000013`.
- **21 public routes**, 0 internal. Cumulative Vendor surface: **101**. Documented in `openapi/section8b-crm-tags-segmentation.openapi.json` and pinned by `internal/section8bcontract`; `cmd`'s HTTP contract test independently counts what the production registration helper actually mounts, so the two cannot drift.
- **Code**: `internal/features/crm/segment_rules.go` (DSL, validation, canonicalisation, hashing, explanations), `segment_registry.go` (field/operator registry and the SQL compilers), `segments.go` (definitions and versions), `segment_membership.go` (static membership, exclusions, member reads), `segment_eval.go` (preview, materialization, worker, invalidation), `segment_privacy.go` (privacy and merge handlers), `tags.go`, `segments_http.go`. Two Section 8A files gained an extension point each: `privacy.go` registers the new handlers, `customer360.go` composes the new blocks.

### Design decisions worth recording

**The rule language is the entire attack surface, so it is a closed registry, not a parser.** A condition can only name a field that exists in `NewSegmentFieldRegistry`, and each field's SQL is a hand-written Go function in one file. Field keys, operators and values never become SQL text â€” values travel as bind parameters and identifiers are never derived from input. There is no free-text filter, no user-chosen sort column and no escape hatch anywhere in the surface. A rejection for an unknown field and a rejection for a forbidden one are the same error, so a probe learns nothing about the schema.

**Unknown is not zero, and SQL's three-valued logic enforces it.** Every commercial condition compiles to `CASE WHEN <observed> THEN (<expr>) ELSE NULL END`, and the whole rule is wrapped in `IS TRUE`. A relationship with no observed commercial data therefore matches neither `count >= 2` nor `count < 2`, and a `NOT` over such a condition does not resurrect it. This is native SQL semantics rather than bolted-on filtering, which is why it holds at every nesting level. Deployment-level missing sources are reported in preview `unavailable_fields`; an individual row that lacks observed data is simply `NOT_AVAILABLE` for evaluation and does not expose a per-customer reason.

**Consent eligibility exists in two places and a test proves they agree.** `MarketingEligibility` in Go remains the single definition of the policy; `consentEligibleCompiler` is its set-based twin. A real-Postgres test seeds allow/deny/unknown/expired relationships and asserts the segment answer matches Section 8A's own eligibility answer for every one, so drift between them fails the build rather than quietly mis-targeting customers.

**A failed rebuild never empties an audience.** Materialization writes a new generation alongside the live one and swaps in a single committed transaction. A failure at any point before the swap leaves the previous membership exactly as it was, and the segment reports `STALE` with an error code rather than a count of zero. An empty audience reads as "nobody qualifies"; a stale one reads as "this is out of date" â€” only one of those is recoverable by the person looking at it.

**Privacy and merge win every materialization race, in both orderings.** The swap transaction locks all candidate profile rows in stable id order and re-evaluates the complete compiled rule before changing the live-generation pointer. Privacy actions and identity merge use the same row locks. If privacy/merge commits first, certification excludes the relationship; if swap commits first, the following privacy/merge transaction removes or transfers its membership and repairs counts. A crash during the fenced swap rolls back the pointer and membership changes together.

**Dependency tracking is real, not decorative.** Each stored definition version carries the registry field keys its rule reads. Tag, commercial, consent and profile-creation mutations mark stale and durably queue only the segments that read the changed fields. Restriction and marketing suppression target only marketing-eligibility dependencies and immediately remove now-ineligible current membership. Duplicate invalidations reuse the queued evaluation rather than creating duplicate work.

**Five permissions, not nine.** Bulk tagging is the same authority as tagging; previewing and materializing are the same authority as reading and managing a definition. Splitting them would create ways for a role to be half-granted rather than any real protection. Member reads are separate, because naming the customers in an audience is a strictly larger disclosure than reading its definition and size.

**Branch scope reaches into the rule and every read surface.** A branch-scoped segment compiles its commercial conditions against that branch's relation row rather than the business-wide aggregate, so "two completed orders" means two at that branch. Preview counts, tag member counts, segment member lists and Customer 360 are branch-filtered. In Customer 360 a branch actor sees only branch-scoped classifications for an authorized branch; business-scoped tag and segment classifications are never disclosed, and tag member counts remain omitted.

**Section 8B extends the Section 8A privacy registry without changing its authority model.** Both new domains register through the existing `PrivacyHandler` port, and `PrivacySummary.CoveredDomains` grows automatically. `ApplyPrivacy` now locks the relationship profile row before dispatching handlers so privacy, merge and materialization share one database fence; decision ownership and handler extensibility are unchanged. That is the property 8C-8H depend on.

**Archived relationships are excluded by default but not made unreachable.** The mandatory filter drops them unless the rule explicitly names `profile.status`, so an operational audience over archived relationships stays expressible while the default stays safe.

### Deliberate scope choices

- **Computed traits are a versioned code registry, not a table.** Every registered field declares its key, label/description, type, source, scopes, operators, nullable/branch/privacy behavior, availability predicate, hand-written query compiler, deterministic explanation renderer, and field-contract version. No persisted trait table was added because these values are deterministic functions of Section 8A projections. No `VIP`, `Churn Risk` or `High Value` system tag was invented â€” those are segment logic, and scoring belongs to 8H.
- **No purpose/marketing-intent field on a segment.** It would not change privacy or consent enforcement, so it would be metadata pretending to be a control.
- **Archived tag names are reusable; archived tag identifiers are not.** A new tag with a reused name gets a new id, so historical membership and audit stay unambiguous.
- **`GenerationRetainer` is a port with a no-op default.** Section 8E implements it to pin the generation a campaign launched against; building the seam now is what stops cleanup from deleting an audience a campaign is still reconciling against later.

### Completion-audit fixes

1. **The canonical rule form did not re-parse.** Values were serialised as arrays and durations as raw seconds, so a stored definition failed revalidation at rebuild time â€” silently, long after the rule was accepted. Fixed by making the canonical form emit exactly the shapes the validator accepts, and pinned by a round-trip test over every value type.
2. **Parameter typing in the generation insert.** A bigint parameter was first used inside a text concatenation, so Postgres inferred `text` and pgx could not encode it. Fixed with explicit casts. The evaluation clock also moved from the database's `now()` to the service clock, so a run is reproducible and testable rather than dependent on container time.
3. **Lease renewal and replay were incomplete.** The worker claimed a lease but did not renew it during a long generation build, and a dead-letter had no operator recovery method. Materialization now heartbeats the lease, uses a unique worker identity per process, and exposes the code-level `ReplayMaterialization` hook. Tests prove renewal and dead-letter replay.
4. **Projection invalidation stopped at `STALE`.** Tag changes were marked stale but not durably enqueued, and commercial, consent, archive/restore, and merge changes did not consistently trigger the dependent audience rebuild. Invalidation now marks and queues in the same transaction, with field-level dependency selection where possible and tenant-wide restaking only for implicit profile-state/merge effects.
5. **Customer 360 over-disclosed business classifications to branch actors.** Branch reads now return only authorized branch-scoped tags and segments; business actors retain the full business view. Member counts remain absent from Customer 360 tag summaries.
6. **Profile creation and privacy eligibility changes missed segment invalidation.** New profiles now invalidate only `profile.subject_type`, `profile.status`, and `profile.acquisition_source_type` dependencies in the creation transaction. Restriction and marketing suppression invalidate only channel-eligibility dependencies, immediately remove current ineligible membership, repair counts, and leave consent decisions unchanged.
7. **Generation swap had a privacy/merge race.** The swap now shares profile-row locks with privacy and merge, certifies the pending generation by re-running the complete rule while fenced, and atomically changes membership, counts, evaluation state, and current-generation pointer. Tests cover both commit orderings and crash rollback/recovery.

### Tests

**257 top-level test functions across the service, 0 failures, 0 skips**, run with real Postgres and real Redis (`go test -p 1 ./... -count=1`) on a freshly created database after the targeted correctness pass. Section 8B contributes:

- **Rule DSL (unit)**: structural rejections, operator/type compatibility, every value-type rejection, all six complexity limits plus the ceiling being usable, canonical-hash stability, canonical round-trip, deterministic explanations, and the security set â€” SQL injected through field names, operators and values, unmapped column names, forbidden personal fields proven absent from the registry, and the registry proven not to leak table or column names.
- **Tags (real Postgres)**: normalisation and duplicate collapse, reserved namespaces, control/invisible/markup rejection, branch scope enforcement, cross-branch invisibility, cross-tenant isolation, all-or-nothing bulk semantics, bulk ceiling, archive clearing membership, version conflicts, branch-relationship requirement, and branch-scoped member counts.
- **Segments (real Postgres)**: static and dynamic lifecycles, terminal archive, static-only and dynamic-only refusals, preview counts and samples, the unknown-versus-zero matrix including negation, branch-scoped preview and the business-scope refusal, privacy exclusion, tag- and branch-reference scope enforcement, cross-tenant isolation, business-segment invisibility to branch actors, and version conflicts.
- **Materialization (real Postgres)**: first build, failure preserving the last good audience, dead-lettering and code-level replay, lease exclusivity, heartbeat renewal and adoption after expiry, partial-generation discard, fenced redaction and merge races in both commit orderings, crash rollback and recovery during the fenced swap, exclusion subtraction surviving rebuilds, rule-edit versioning, transactional dependency-scoped invalidation/enqueue (including commercial and profile-creation changes), duplicate-request collapsing, archive dropping membership, branch-scoped figures, a shared evaluation instant, single-generation reads, the disabled-worker behaviour, two concurrent workers producing one audience, generation cleanup, the retention port pinning a generation, and the consent cross-check against Section 8A.
- **Privacy and merge (real Postgres)**: domain registration, immediate removal of tags/membership/exclusions on redaction with dynamic restaking, restriction/suppression eligibility invalidation with immediate membership and count repair while consent decisions remain unchanged, tag and static-membership races in both orderings, tag and static-membership transfer on merge with deduplication, exclusion transfer, dynamic membership recomputed rather than copied, and Customer 360 scope tests proving branch actors never receive business-scoped classifications.
- **Scale (real Postgres, 1,200 relationships)**: materialization across batch boundaries with no loss or duplication, complete non-overlapping pagination, the preview count budget reporting an honest lower bound, a bulk operation at the 500 ceiling, a rule at the documented complexity ceiling evaluating, and the candidate query's real plan.
- **Contract**: bidirectional OpenAPI/route parity, route count, every operation declaring a permission, every command declaring `Idempotency-Key`, no campaign- or later-section-shaped route, exact enum vocabularies, a fully recursive documented rule grammar, and the error-code catalogue.
- **Migration**: fresh replay, 8A-to-8B upgrade, rollback and reapply as a pair, Section 8A schema and permissions proven undamaged by an 8B rollback, exact SQL vocabularies, eleven composite tenant foreign keys, thirteen indexes, and the role-mapping matrix including the roles that must hold nothing.
- **Config**: both flag prerequisites and the safe default.

Customer 360 availability tests advance with each implemented module: tags/segments are available under Section 8B flags, interactions/tasks under Section 8C flags, loyalty under the Section 8D flag, and 8E-8H remain unimplemented. Section 8E deliberately does not change that: campaign targeting history is not added to Customer 360, so `campaigns` still reports `NOT_IMPLEMENTED` there even though the section itself is implemented. The Section 7, 8A, and 8B rollback tests walk down to their own version boundaries rather than assuming they are the migration tip.

### Performance limits (configuration, not capacity claims)

Pagination default 50 / max 200. Bulk batch default 100 / max 500. Rule limits: depth 6, nodes 50, OR branches 25, IN values 100, NOT depth 3, tag references 25. Materializer batch 500, lease 2 minutes, 5 attempts, 30s base backoff. Preview sample default 25 / max 100, count budget 50,000. The scale tests ran against a 1,200-relationship fixture; **no figure from them is a production capacity claim**, and none should be quoted as one.

### Known dependencies

Unchanged from Section 8A, and none new. Segmentation runs entirely over Vendor-owned data, so it needs no external service. What it inherits:

1. **No vendor commerce order publisher** â€” every commercial field reports as unobserved, so commercial rules match nobody. This is the dependency that makes segmentation useful, and it belongs to the order section.
2. **No consent authority** â€” consent fields are unavailable, so a marketing-eligibility rule matches nobody. Fail-closed by design.
3. **No privacy or identity-merge authority** â€” the handlers are built and tested against fixtures; nothing currently instructs them in production.

Section 8B's own flags default off, and `VENDOR_CRM_SEGMENTATION_ENABLED` requires `VENDOR_CRM_ENABLED`, which `VENDOR_CRM_SEGMENT_MATERIALIZATION_ENABLED` in turn requires â€” validated at startup, so a half-enabled configuration is a boot failure rather than a runtime surprise.

### Not started

At the Section 8C boundary, Sections 8D-8I had not started. Section 8D is now implemented below; Sections 8E (campaign intelligence), 8F (campaign execution), 8G (corporate CRM), 8H (intelligence/exports), 8I (final CRM integration), and everything in Section 9 onwards remain not started. No table, route, permission, flag, worker, or event for those sections was created; Section 8C contains no notification delivery, support-ticket authority, frontend, or future commerce domain.

## Section 8C â€” interactions, timeline, and CRM tasks

**Status: IMPLEMENTED and PostgreSQL-verified.** Section 8C extends `internal/features/crm` with Vendor-local relationship interactions, append-only corrections, staff follow-up tasks, immutable task history, privacy/merge handlers, bounded Customer 360 activity, and 16 public HTTP operations. It creates no internal route and changes no external service.

- **Migrations:** `000014_vendor_crm_interactions_tasks` adds `crm_interactions`, `crm_interaction_revisions`, `crm_tasks`, and `crm_task_history`; `000015_vendor_section8c_permissions` adds six permissions and established role mappings. Current tip is `000015`.
- **Interactions:** 12 controlled types, four directions, ten outcomes, `BUSINESS`/`BRANCH` visibility, 200-character subject, 5,000-character plain-text note, and optimistic correction with an immutable full-snapshot revision.
- **Tasks:** `OPEN`, `IN_PROGRESS`, `COMPLETED`, and `CANCELLED`; `LOW`, `NORMAL`, `HIGH`, and `URGENT`; same-tenant active eligible staff assignment; nullable UTC due date; derived `is_overdue`; explicit terminal transitions and optimistic version checks.
- **Privacy and merge:** interaction content becomes display-suppressed after redaction; open/in-progress tasks are cancelled and suppressed; merged records move transactionally to the canonical profile. Creation/correction/task mutation and privacy/merge share the profile-row lock fence.
- **Customer 360:** at most five recent permitted interactions with `has_more`, plus at most five open permitted tasks and an exact scope-safe count. Redacted and merged-source profiles expose no ordinary activity content.
- **Audit/outbox:** create, correction, task create/update/start/reassign/complete/cancel all write transactionally. Events are `vendor.crm.interaction.created`, `vendor.crm.interaction.corrected`, `vendor.crm.task.created`, `vendor.crm.task.updated`, `vendor.crm.task.started`, `vendor.crm.task.reassigned`, `vendor.crm.task.completed`, and `vendor.crm.task.cancelled`; free-text bodies and canonical PII are excluded from event/audit metadata.
- **Flags:** `VENDOR_CRM_INTERACTIONS_ENABLED` and `VENDOR_CRM_TASKS_ENABLED`, both safe-default false and requiring `VENDOR_CRM_ENABLED`; all commands also obey `VENDOR_CRM_WRITES_ENABLED` and `COMMERCE_GLOBAL_KILL_SWITCH`.
- **Rate limits:** the existing fail-closed Redis limiter is applied per business/staff: 60 interaction/task creates and 120 correction/task mutations per minute.
- **Tests:** real PostgreSQL coverage includes tenant/branch/customer/assignee/link/history IDOR, validation, idempotency, revision/history, privacy and merge, Customer 360 disclosure, deterministic concurrency races, migration replay/upgrade/rollback/reapply, OpenAPI parity, and a 300-profile/1,500-interaction/600-task scale fixture.
- **No authority expansion:** no notification delivery, support/dispute workflow, canonical customer PII, cross-service database read, reminder worker, attachment, bulk task mutation, frontend, Section 8D+, or Section 9+ implementation.

Known deployment dependencies remain explicit: Customer Service must eventually publish privacy/merge authority events, Notification Service would own any future reminder delivery, and the future order domain must publish commercial facts. Section 8C itself fabricates none of these integrations.

## Section 8D - loyalty points

**Status: IMPLEMENTED and PostgreSQL-verified.** Section 8D extends `internal/features/crm` with a per-business loyalty programme, lazily created customer accounts, an append-only points ledger, point lots with per-issuance expiry, recorded lot consumption, a durable expiry sweeper, bounded manual adjustments, full reversal, privacy and identity-merge handlers, a Customer 360 loyalty block, and 6 public HTTP operations. It creates no internal route and changes no external service.

- **Migrations:** `000016_vendor_crm_loyalty` adds `crm_loyalty_program_configs`, `crm_loyalty_accounts`, `crm_loyalty_entries`, `crm_loyalty_point_lots`, and `crm_loyalty_lot_consumptions`; `000017_vendor_section8d_permissions` adds four permissions and role mappings. Current tip is `000017`.
- **Points, not money.** No currency, minor-unit amount, wallet reference or conversion path exists anywhere in the schema, the API or the events. `payment-wallet-service` was not touched and remains the sole monetary authority. Both a schema-level test and an OpenAPI contract test reject a monetary-looking column or field, so the boundary fails the build rather than drifting.
- **Programme:** `DISABLED`/`ACTIVE`/`PAUSED`; `NEVER`/`FIXED_DAYS` expiry with a 1-3650 day window; configurable points nouns, redemption minimum and increment, and manual adjustment limit; optimistic concurrency where the first write must send `expected_version: 0` and a concurrent first write loses on the unique constraint rather than silently overwriting.
- **Accounts:** one per business/profile, created lazily on first real movement - a read never creates a row, so `NOT_ENROLLED` is a genuine fact rather than a fabricated zero. Concurrent first credits resolve to one account via the unique constraint.
- **Ledger:** append-only, eight closed entry types, per-account monotonic sequence, `balance_after` on every row, and a database constraint holding `current_points = lifetime_credited - lifetime_debited`. There is no update or delete path; corrections are compensating entries.
- **Lots and FEFO:** every credit issues a lot stamped with the expiry policy as it stood at issue time, never recomputed. Debits consume earliest-expiring first (non-expiring last) and record exactly which lot funded how much, which is what makes a later reversal exact rather than approximate.
- **Expiry:** `available_points` always excludes lots past their expiry whether or not the sweeper has settled them, and every balance-changing command settles due expiry before acting - so a sweeper outage delays bookkeeping without ever letting an expired point be spent. The sweeper needs no queue table and no lease: a lot with points left and a past expiry is itself the durable job, giving crash recovery and multi-worker safety (SKIP LOCKED on the account row) from the schema. Disabling the flag defers the backlog rather than discarding it.
- **Earn and redeem:** implemented as service-internal primitives with durable command-key and source-event deduplication. **No public earn or redeem route exists**, and the contract test rejects one structurally: an endpoint letting a caller name its own points total is a far larger authority than the bounded manual adjustment. No earning formula was invented, because the Vendor Order domain that would define one does not exist.
- **Manual adjustment:** dedicated high-risk permission, positive amount only (direction carries the sign), bounded by the programme's own limit, mandatory 500-character reason recorded in the audit log **only** - never in the ledger row, the outbox payload or any log line. A credit may enrol; a debit may not, and may not overdraw.
- **Reversal:** full only, exactly once per entry (a unique index, not a read-then-write check). Only `EARN`, `REDEEM`, `ADJUSTMENT_CREDIT` and `ADJUSTMENT_DEBIT` are reversible. Reversing a credit refuses when its lot has been touched rather than confiscating points from an unrelated issuance; reversing a debit restores the exact points to the exact funding lots with their original expiry, then immediately expires anything already past its window in the same transaction.
- **Privacy and merge:** redaction makes the account non-actionable and suppresses it from every ordinary read while retaining the ledger, which holds integer movements and vendor-local references with no personal data to erase. Marketing suppression and restriction are explicit no-ops - holding a balance is not marketing consent, and withdrawing consent is not forfeiture. Identity merge posts an auditable `MERGE_OUT`/`MERGE_IN` pair, transfers surviving lots with their original expiry preserved, and leaves the source ledger immutable and non-actionable.
- **Customer 360:** one loyalty block reporting programme status, enrolment, available/lifetime/pending-expiry points and the next expiry, contributed through the same composition point Sections 8B and 8C use. Redacted and merged-source profiles report `NOT_ENROLLED` with no figures.
- **Audit/outbox:** programme update, account creation, credit, debit, expiry, adjustment, reversal and merge all write transactionally. Events are `vendor.crm.loyalty.program.updated`, `.account.created`, `.points.credited`, `.points.debited`, `.points.expired`, `.adjustment.applied`, `.entry.reversed`, and `.account.merged`; free-text reasons, command keys, source references and canonical PII are excluded from every payload.
- **Flags:** `VENDOR_CRM_LOYALTY_ENABLED`, safe-default false and requiring `VENDOR_CRM_ENABLED` (validated at startup). Commands also obey `VENDOR_CRM_WRITES_ENABLED` and `COMMERCE_GLOBAL_KILL_SWITCH`.
- **Segmentation boundary:** the Section 8B field registry remains at exactly 22 fields, guarded by a test. Loyalty is deliberately not segmentable yet; adding a field would silently change what every stored audience means.
- **Tests:** real PostgreSQL coverage includes programme versioning and expiry-policy validation, non-retroactive lot expiry, ledger replay and aggregate agreement, overflow and zero/negative rejection, FEFO and multi-lot consumption, sweeper idempotence and multi-day outage catch-up, adjustment validation/limit/idempotency and reason non-disclosure, the full reversal matrix, tenant/branch/customer/entry/programme IDOR, Customer 360 disclosure, HTTP permission separation and edge idempotency, migration replay/upgrade/rollback/reapply with constraint and index assertions, OpenAPI parity, and eight deterministic barrier-synchronised concurrency races (debit vs debit, first-credit creation, expiry vs debit, double reversal, merge vs credit, merge vs debit, merge vs expiry, redaction vs adjustment) each closing with a ledger/aggregate/lot reconciliation.
- **No authority expansion:** no loyalty tiers, reward catalogue, voucher or coupon engine, gift cards, cashback, wallet conversion, cash redemption, campaign targeting, notification delivery, order integration, internal route, external-service change, frontend, or Section 8E+/Section 9+ implementation.

**Known dependencies, disclosed rather than fabricated.** The Vendor Order domain does not exist, so nothing awards points automatically today; `CreditPoints` and `DebitPoints` are ready for it and deduplicate producer events durably. Expiry-warning notifications would be Section 8E/8F work and require their own Notification Service integration. The pre-existing `go test -p 1 ./...` serialisation caveat documented under Section 5 still applies to the shared external test database.

## Section 8E - campaign intelligence and the audience engine

**Status: IMPLEMENTED and PostgreSQL-verified.** Section 8E extends `internal/features/crm` with campaign definitions, immutable campaign versions, segment-backed audience selection, privacy-safe preview, durable audience preparation, immutable audience snapshots, campaign targets, per-channel snapshot-time consent evidence, deterministic experiment cohorts, frequency and quiet-hour policy definition, a privacy/merge handler, a segment-generation retainer, and 10 public HTTP operations. It creates no internal route and changes no external service.

- **Migrations:** `000018_vendor_crm_campaign_intelligence` adds `crm_campaigns`, `crm_campaign_versions`, `crm_campaign_experiment_variants`, `crm_campaign_audience_snapshots`, `crm_campaign_targets`, and `crm_campaign_target_channels`; `000019_vendor_section8e_permissions` adds five permissions and role mappings. Current tip is `000019`.
- **Prepares, never delivers.** No scheduling code, send endpoint, notification handoff, template, provider client, `scheduled_at`, `sent_at`, `delivered_at` or destination column exists anywhere in the section. The campaign lifecycle is `DRAFT`/`PREPARING`/`PREPARED`/`PREPARATION_FAILED`/`ARCHIVED` with no delivery state, and a migration test rejects one structurally. `notification-service` was not touched.
- **Dual-time consent.** Preparation records what the authority's projection said, per target per channel, as immutable evidence. Section 8F must revalidate current consent before any delivery handoff. `send_time_recheck_required` carries a `CHECK (send_time_recheck_required = true)`, so a row asserting otherwise is unstorable rather than merely unwritten, and a later consent change never rewrites the frozen evidence.
- **Fail-closed eligibility.** Consent is decided by Section 8A's single `MarketingEligibility` rule, not a second copy: DENY, UNKNOWN, a missing projection, a lapsed ALLOW and a stale projection are all ineligible, and with the consent integration disabled every channel is ineligible with `CONSENT_SOURCE_UNAVAILABLE`. A database check additionally refuses any row whose stored decision and stored verdict disagree.
- **Suppression is absence, not a state.** Privacy- and service-recovery-suppressed relationships receive no target row at all and appear only in the snapshot aggregates, so the channel-eligibility vocabulary is exactly two values and there is no state meaning "cleared to send". Structural suppression reuses Section 8A's own reason codes rather than inventing coarser ones.
- **Audience source:** the Section 8B segment engine and nothing else. No rule column, no SQL parameter, no customer-list upload. A `BRANCH` campaign may only use a `BRANCH` segment for its own branch; a `STALE`, `FAILED` or unevaluated dynamic source is refused with `CRM_CAMPAIGN_AUDIENCE_NOT_CURRENT` rather than silently frozen. Exclusions are bounded at 10 segments; static exclusions use generation 0 and dynamic exclusions use only their current successful definition/generation.
- **Immutable versions:** editing a campaign mints a new version and leaves the old one, and every snapshot points at the exact version it was built from, so an edit mid-preparation cannot mix two definitions. The definition hash is order-independent, so reordering channels, exclusions or variants is correctly recognised as no change. A `PREPARED` campaign returns to `DRAFT` on edit rather than inheriting an audience the new definition never described. Scope is immutable after creation.
- **Generation pinning:** the `GenerationRetainer` port Section 8B deliberately left open is now implemented, so a segment rebuild cannot delete the membership a queued or current campaign snapshot depends on. Section 8B's `cleanupSupersededGenerations` was changed to take the evaluation row's lock before asking the retention question, and campaign enqueue takes the same lock before pinning - closing a genuine race in which both could have decided the other was not there.
- **Preparation worker:** the snapshot row is its own durable job (lease + `SKIP LOCKED` + heartbeat + bounded retry + dead-letter + a code-level replay hook), matching the Section 8B materializer rather than introducing a second worker system. A partial audience can never become current: the new snapshot is built alongside the old, promoted in one transaction that re-sweeps under a profile-row fence and recomputes the counts from the surviving rows, and a failure at any point before that leaves the previous `CURRENT` snapshot and the campaign's pointer to it untouched. Batch size 500. Only one preparation per campaign can be in flight, enforced by a partial unique index rather than a process-local guard.
- **Experiments:** integer basis points totalling exactly 10000, at most one control (at most 50%), one to five treatment variants. Assignment is `sha256(campaign_version, profile_ref, namespace) mod 10000` laid over cumulative ranges in a canonical order - deterministic, reproducible without a stored lookup, stable within a campaign version, and taking no contact data as input. A campaign without an experiment produces no cohort at all rather than a synthetic treatment group of everyone.
- **Policies:** frequency caps and quiet hours are validated, versioned and immutable, and enforced nowhere here. `VENDOR_CAMPAIGN_CONTACT_HISTORY = NOT_PRESENT`: nothing has ever been delivered, so a cap evaluated against an empty history would always pass. Frequency therefore reports `NOT_AVAILABLE` with a mandatory recheck, never "eligible". Quiet hours are minute-of-day plus a mandatory IANA zone (`time/tzdata` is embedded so validation works on Windows and slim containers), handle cross-midnight windows, and are evaluated only against an explicit hypothetical instant a caller supplies. No scheduler was written.
- **Service recovery:** the Vendor-local signal is an `OPEN`/`IN_PROGRESS` CRM task linked to a `SERVICE_FOLLOW_UP` interaction - both halves required, so an ordinary open task does not suppress an audience. It is evaluated live on reads rather than stored, because a complaint can be resolved at any moment and a stored flag would be stale. `EXTERNAL_SERVICE_RECOVERY_SUPPORT = NOT_PRESENT`; no support-dispute state is read or fabricated. Section 8C was not modified.
- **Privacy and merge:** registered through the same `PrivacyHandler` port Sections 8B-8D use. Unlike those domains it revokes current actionability rather than deleting evidence: the frozen snapshot stays auditable while the customer stops being contactable the moment the instruction commits. A merge revokes the source rather than moving its targets onto the survivor, leaving exactly one actionable row for the merged identity.
- **Audit/outbox:** created, updated, archived, preparation.started and prepared all write transactionally. Events carry the audience's identity and size and never its membership; a customer reference in a campaign event fails the build.
- **Flags:** `VENDOR_CRM_CAMPAIGNS_ENABLED` and `VENDOR_CRM_CAMPAIGN_PREPARATION_ENABLED`, both safe-default false. Campaigns require `VENDOR_CRM_ENABLED` **and** `VENDOR_CRM_SEGMENTATION_ENABLED` (the audience has no other source); preparation requires campaigns. Freezing preparation never disturbs an audience that is already prepared.
- **Segmentation boundary:** the Section 8B field registry remains at exactly 22 fields. Campaign membership, campaign counts and campaign response are deliberately not segmentable; adding a field would silently change what every stored audience means.
- **Customer 360:** unchanged. Campaign targeting history is not surfaced per customer, so the `campaigns` module still reports `NOT_IMPLEMENTED` - communication history belongs after execution exists.
- **Tests:** real PostgreSQL coverage includes the full consent matrix and fail-closed behaviour, per-channel decisions, dual-time evidence immutability, privacy/merge/service-recovery suppression and their post-snapshot revocation, static and dynamic sources, stale/failed source refusal, exact generation pinning, exclusion precedence, funnel reconciliation on every snapshot, last-good-snapshot preservation and terminal dead-lettering with replay, atomic supersede, edit-during-preparation version binding, tenant/branch/segment/snapshot/target IDOR, HTTP permission separation and edge idempotency, schema- and wire-level PII and delivery-machinery guards, audit and outbox payload inspection, migration replay/upgrade/rollback/reapply with constraint and index assertions, bidirectional OpenAPI parity, six deterministic lock-ordered concurrency races (duplicate prepare, redaction committing first, redaction committing second, merge during preparation, service recovery during preparation, segment rebuild versus a pinned generation), and a 1,000-relationship fixture covering batch boundaries, pagination completeness, cohort spread and index coverage.
- **No authority expansion:** no campaign scheduling, delivery, notification handoff, provider callback, delivery history, send-time enforcement, template, corporate CRM, RFM/churn scoring, sensitive export, internal route, external-service change, frontend, or Section 8F+/Section 9+ implementation.

**Known dependencies, disclosed rather than fabricated.** No consent authority publishes to this platform yet, so with the integration flag off every channel is correctly ineligible and a prepared audience contacts nobody - the section is built to be safe in exactly that state rather than to pretend otherwise. No vendor commerce order publisher exists, so commercial segment rules still match nobody (inherited from Section 8B, unchanged here). Section 8E preview correctly has no contact history; Section 8F now owns the real handoff history used for frequency enforcement. The pre-existing `go test -p 1 ./...` serialisation caveat documented under Section 5 still applies to the shared external test database.

## Section 8F â€” campaign execution and Notification handoff

**Status: IMPLEMENTED and PostgreSQL-verified.** Migrations `000020_vendor_crm_campaign_execution` and `000021_vendor_section8f_permissions` add durable runs, deliveries, append-only transition history, contact history, delivery-event dedupe, four permissions, nine public operations, and one internal HMAC ingress. The execution flag defaults false.

Run creation binds the current `PREPARED` snapshot and exact immutable version. Workers use `SKIP LOCKED`; every handoff rechecks current consent, privacy/identity, service recovery, quiet hours, frequency, cancellation/pause, and channel availability under database fences. Notification receives an opaque customer reference and fixed delivery idempotency key, never destination PII. `HANDED_OFF` is distinct from `DELIVERED`. SMS is suppressed because the real Notification contract has no SMS channel. The current Notification Service has no delivery-event producer, so the secured Vendor receiver is implemented but production callbacks remain a disclosed dependency.

Adversarial tests cover corrected current-generation exclusions, consent withdrawal, privacy after snapshot, quiet-hour boundaries, sequential and concurrent frequency caps, cancellation, kill-switch behavior, Notification timeout retry identity, and event dedupe/out-of-order protection. Migration tests cover fresh replay and 8E-to-8F rollback/reapply. Sections 5â€“8E remain in the full serial regression gate.

## Section 8G â€” corporate customer CRM

**Status: IMPLEMENTED and PostgreSQL-verified.** Migrations `000022_vendor_crm_corporate_customers` and `000023_vendor_section8g_permissions` add seven tables (code counters, profiles, branch scopes, locations, cost centres, contacts, memberships), an additive corporate subject on the Section 8C interaction and task tables, five permissions and 41 public operations. No internal route. The corporate flag defaults false and requires `VENDOR_CRM_ENABLED`.

**What it owns.** The commercial relationship with an organization: identity and vendor-local `CORP-` reference, lifecycle (`PROSPECT`/`ACTIVE`/`ON_HOLD`/`INACTIVE`/`ARCHIVED`), business or branch-set scope, an account manager, contacts referencing Section 8A relationships, memberships, the organization's own sites, and cost-centre labels.

**What it does not own, and cannot express.** No corporate order, invoice, credit, budget, contract, quotation, wallet, catalogue entitlement, approval workflow, RFM/churn score or export. A cost centre carries no budget, limit, currency or balance. The lifecycle contains no commercial state, and the migration test asserts the absence directly rather than trusting review.

**Isolation is structural.** Every child reaches its parent through a composite key: `(id, business_id)` for tenancy and `(id, business_id, corporate_customer_id)` for nested resources, so PostgreSQL â€” not only the HTTP layer â€” refuses a membership pointing at another account's contact, or an interaction naming another vendor's site. Branch-scoped operators see business-wide accounts plus branch-set accounts covering a branch they hold, and cannot widen an account past their own scope.

**Concurrency is proved by lock-ordered races, not by timing.** Six deterministic tests pin contact creation against redaction (both commit orderings), contact creation against identity merge, membership and activity against merge, archive against task creation, complete against cancel, and two simultaneous primary-contact promotions. Each blocks the second transaction on a real row lock and asserts the surviving state, so a passing run is evidence rather than a fast machine.

**Section 8C was extended, not modified.** `customer_profile_id`/`original_profile_id` became nullable under `crm_interaction_subject_ck` and `crm_task_subject_ck`; Go structs kept `string` fields with SQL `COALESCE`, so no accepted Section 8C call site changed and the full 8C suite passes unaltered. The 8G rollback restores both columns to `NOT NULL` and drops the corporate columns, which the migration test asserts.

**Known dependencies, disclosed rather than fabricated.** No corporate ordering, invoicing or credit domain exists on this platform, so `MEMBERSHIP_ROLE=APPROVER` is a CRM label and approves nothing â€” deliberately, because an approval that authorises nothing is safer than one that appears to. Corporate accounts are not customers of `customer-service`: the organization exists only inside this vendor's tenancy, and only its people are platform customers. The pre-existing `go test -p 1 ./...` serialisation caveat documented under Section 5 still applies to the shared external test database.

## Section 8H â€” CRM intelligence, analytics and secure exports

**Status: IMPLEMENTED and PostgreSQL-verified.** Migrations `000024_vendor_crm_intelligence_exports` and `000025_vendor_section8h_permissions` add four tables (evaluations, scope states, customer scores, exports), six permissions and 10 public operations. No internal route. Both feature flags default false.

**The models.** `RFM_V1` scores recency, frequency and monetary as scope-relative quintiles computed from PostgreSQL `cume_dist`, so equal inputs always receive equal scores and a rerun over unchanged facts is byte-identical. `CHURN_V1` is an integer 0-100 heuristic derived from how overdue a relationship is against its own observed purchase cadence, plus absolute inactivity and engagement depth. Both carry a model version and a definition hash taken over a canonical descriptor, so changing a threshold â€” or changing what the model claims to do â€” changes its identity and a queued run stamped with the old one is superseded rather than writing scores under a hash that no longer exists.

**What the models refuse to do.** A relationship with no commercial projection is `SOURCE_NOT_AVAILABLE`. One with a single observed order is `INSUFFICIENT_HISTORY`, never high churn risk â€” the rule that stops a follow-up list filling with people who arrived yesterday. A scored cohort below five members is `INSUFFICIENT_COHORT`, because a quintile over three people reads exactly like a quintile over thousands.

**Generation safety.** Reuses the Section 8B shape: a durable evaluation row is the job, the generation is built alongside the live one, and the swap is a single transaction that re-certifies against privacy and recomputes counts from the survivors. A forced-failure test proves the previous generation stays live and current, and that no row from the failed generation is ever `is_current`.

**Segmentation.** The Section 8B registry grows from 22 fields to exactly 30. Score predicates read through a correlated subquery requiring `state='CURRENT'`, so an unavailable, stale or failed score yields NULL and matches no numeric comparison. Invalidation is dependency-scoped: a churn rebuild leaves an RFM-only segment `CURRENT`.

**Exports.** Six closed datasets, two line-oriented formats, a per-dataset versioned field allowlist, typed filters, a 100k default and 250k hard row ceiling with a byte cap, keyset-streamed reads, a lease-based generator, cancellation, retention sweeping and full audit. Every dataset whose rows are about an individual excludes redacted and merged-away relationships centrally in `buildExportQuery`, so a dataset added later cannot forget. No canonical contact detail, payment credential, notification destination or free-text CRM note appears in any dataset, asserted against the real generated bytes in both formats.

CSV rendering deterministically prefixes a single quote to text cells beginning with `=`, `+`, `-`, `@`, tab or carriage return, neutralising spreadsheet formula interpretation without modifying CRM source data. JSONL preserves native JSON value semantics.

### Section 8H completion report

EXPORT STORAGE:

- Initial support status: `PARTIAL`
- Gap: Media's existing private object contract used magic-signature MIME detection and therefore rejected legitimate sniff-less CSV/JSONL; it also had no closed export purpose.
- Minimal Media File Service change: additive, purpose-scoped private textual artifact support only; no generic media redesign and no export generation moved out of Vendor Service.
- DATA_EXPORT purpose: added to Media's closed purpose registry and constrained private in the database and use case.
- CSV: exactly `.csv` + `text/csv`; bounded UTF-8/control-byte/signature checks and standard-library CSV structural parsing.
- JSONL: exactly `.jsonl` + canonical `application/x-ndjson`; every non-empty line must be one valid JSON object. No additional MIME alias is repository-established or accepted.
- Private-only enforcement: `DATA_EXPORT + PUBLIC` is rejected server-side and is unstorable under Media database checks.
- Safe-text validation: non-empty, valid UTF-8, NUL/C0-control rejection, executable/archive/binary-signature and markup rejection, exact extension/MIME pairing, bounded parsing, checksum and stored-size verification. Existing purpose signature detection is unchanged.
- Max artifact size: **20 MiB (20,971,520 bytes)**, purpose-specific and clamped to Media's global upload ceiling; Vendor uses the same 20 MiB ceiling.
- Content-Disposition: `attachment` with the validated, header-sanitised filename.
- Nosniff: `X-Content-Type-Options: nosniff`; private content also uses `Cache-Control: no-store`.
- Artifact reference: opaque Media object id only. Vendor persists `artifact_ref`, `checksum_sha256`, `artifact_bytes`, `row_count`, `completed_at` and `expires_at`; it persists no path, URL, credentials or bytes.
- Short-lived access: Vendor rechecks actor permission, tenant/branch scope, `READY`, expiry and purpose-bound single-use Step-Up, audits access, then asks Media for its object-specific token/path grant (five-minute default). Nothing permanent is stored.
- Artifact expiry: approximately 24 hours. Vendor supplies the same `retentionUntil` to Media, refuses and marks expired jobs, and invokes Media's existing idempotent deletion semantics on expiry/cancellation/failure cleanup.
- Media migrations: `000004_data_export_artifacts` adds visibility/file-name metadata, private-only checks and the export-retention index; no existing table semantics are weakened.
- Media tests: validator and real-PostgreSQL contract coverage proves private CSV/JSONL success; PUBLIC, wrong extension/MIME, unsupported alias, binary/NUL/control/invalid UTF-8, malformed/oversized content and HTML/SVG/executable bypass refusal; existing-purpose regression; short-lived private attachment access; completed-upload reconciliation; and deletion denial.
- Vendorâ†”Media contract: generated CSV/JSONL is checksum-verified, uploaded as private `DATA_EXPORT` under the stable export id, retained for 24 hours, and reconciled after an ambiguous completion. `READY` occurs only after verified persistence; storage failure remains non-READY; Step-Up gates download; expired artifacts are refused and deleted. Adapter tests also pin Media retention and deletion-idempotency headers.
- Final support status: `AVAILABLE`
- Result: `PRIVATE_EXPORT_ARTIFACT_SUPPORT = AVAILABLE`

EXTERNAL SERVICES:

- media-file-service: `MINIMAL SECTION 8H CHANGE â€” DATA_EXPORT private CSV/JSONL artifact support`
- All other external services: unchanged; no other concrete Section 8H requirement needs a cross-service modification.

**Two clock bugs the tests caught pre-commit.** The artifact expiry and the privacy fence were both computed from the service clock and compared against database-written values; on any host whose clock differs slightly that would have published artifacts past their retention and made every export look like it raced a redaction. Both now read the database clock, which is the one that stamped the values they are compared against.

**Known dependencies, disclosed rather than fabricated.** No vendor commerce order publisher exists, so a production deployment will legitimately report `NOT_AVAILABLE` for every score until authoritative order facts arrive â€” the section is built to be honest in exactly that state rather than to fill it with zeros. `media-file-service` required one minimal change to accept an export artifact (see the change register). The pre-existing `go test -p 1 ./...` serialisation caveat documented under Section 5 still applies to the shared external test database.

## Section 8I â€” final CRM integration, hardening and acceptance

**Status: CERTIFIED.** No new CRM module, no new public or internal route, no new
migration. Vendor surface remains **194 routes**, migration tip remains
`000025_vendor_section8h_permissions`, segment registry remains **30 fields**.

**Defects fixed.**

1. *Internal authority route mounted under the public base.* `staffhttp.RegisterInternalRoutes`
   was called with the `/api/v1/vendor` group, exposing `POST /internal/staff/provision-owner`
   at `/api/v1/vendor/internal/staff/provision-owner`. Its OpenAPI, the Section 5
   contract package and the route register all specified the engine root. The
   registrar now takes the engine and mounts at `/internal`, matching the rule
   Section 7 set for the verification callback. Guarded by
   `TestInternalRoutesAreNeverMountedUnderThePublicBase`.
2. *Notification handoff inside a database transaction.* Section 8F called
   `notifier.Send` while holding the delivery, run and customer-profile row
   locks. The profile lock is the shared privacy fence, so the remote call could
   block redaction. Restructured to reserve-and-commit, call, then record in a
   second transaction, with a `HandoffReservation` window (default 2 minutes,
   comfortably beyond the client timeout) preventing a second worker from
   adopting an in-flight delivery. Duplicate handoff is prevented by the stable
   `NotificationRequestRef` idempotency key the remote side deduplicates on.
   Contact history remains written under the profile lock inside the reservation
   transaction, because the frequency cap counts those rows.

**Verification gap closed.** Section 8H's broad serial run exceeded the execution
window without reporting. Section 8I measured why and ran the suite in
deterministic groups on separate databases: the CRM package alone takes ~834s,
which is what overran. Groups: (A+C) all 21 non-CRM packages, (B) the CRM
package, (G) media-file-service. All three: 0 failures, **0 skips**.

**Added tests.** The global route/OpenAPI/register drift guard
(`internal/crmcontract`), the cross-module redaction and anti-resurrection
certification, the marketing-suppression semantics test, and two live-schema
authority scans proving CRM holds no canonical customer PII and loyalty holds no
monetary column.

**Known dependencies remain external, documented and fail-closed** â€” see the
dependency matrix in `PLATFORM_SERVICE_CHANGE_REGISTER.md`. No production path
fabricates an unavailable external truth.


---

## Section 8 completion â€” remaining original Section 8 requirements

Sections 8A-8I delivered the CRM architecture. This final-completion pass audited the ORIGINAL
Section 8 specification line by line against what was actually built and
completed the remaining CRM-owned runtime slice. Producer-owned facts remain
fail-closed until their authoritative service sends them; that is an
availability state, not a fabricated zero.

**Migrations added:** `000026_vendor_crm_section8_completion`,
`000027_vendor_section8_completion_permissions`, and
`000028_vendor_crm_section8_final_completion` (reversible additive completion
columns, configuration, bounds and trigger lifecycle). Vendor migration tip is
now `000028`.

**Routes added:** 2 public, 0 internal. Vendor surface **194 -> 196**.

### What is COMPLETE and active

**8.4 Loyalty tiers** â€” implemented end to end and active today, with no
external producer required. A vendor defines an ordered ladder of at most 8
thresholds; a customer's standing is DERIVED at read time from the Section 8D
ledger and never stored, so a credit, a reversal, an identity merge or a
threshold change is reflected immediately with no recalculation job and no
second copy of the truth to drift.

Decisions worth recording:

- **The input is lifetime credited points, net of reversed credit** â€” not the
  spendable balance. The original specification permitted either and required
  the choice be made and documented. Status recognises what a customer has done
  over the relationship, so redeeming points must not demote them. A test pins
  this: a customer who earns 1200 and redeems 1000 keeps their rung, and a
  customer whose credit is *reversed* surrenders it.
- **That quantity is computed from the ledger entries, not from the account's
  `lifetime_credited_points` column** â€” a correction a test forced. Section 8D's
  ledger is immutable, so reversing a credit posts a compensating REVERSAL entry
  rather than decrementing the counter; reading the counter would have let a
  credit posted in error confer permanent status, which is exactly what a
  reversal exists to undo. Summing credit-type entries that have not been
  reversed gets all three cases right at once: debits never appear so redeeming
  cannot demote, a reversed credit is excluded, and `MERGE_IN` is included so a
  relationship that absorbed another keeps the standing it earned.
- **Ranks are derived from thresholds, never supplied.** A caller-supplied rank
  could disagree with its own ordering and the ladder would have two answers to
  "which tier is higher".
- **Thresholds must be distinct.** Two rungs at one entry point make membership
  ambiguous, and an ambiguous customer-facing status is worse than no tiers.
- **NOT_AVAILABLE, NO_TIER and CURRENT are three different answers.** No ladder
  configured is not the bottom rung; a configured ladder not yet reached is not
  the bottom rung either.
- **Recognition, not benefits.** No discount, entitlement, multiplier or
  monetary column exists, and a live-schema scan test fails the build if one
  appears. Points are not money; payment-wallet-service remains the only
  monetary authority.
- Replacement is atomic and version-checked under a per-business advisory lock;
  superseded rungs are archived rather than deleted so audit records that
  referenced them still point at something.

### Final-completion runtime status

> The historical paragraph immediately below this heading described the
> pre-handoff state. It is superseded by the completion table and verification
> below: the runtime projections, ingress handlers, Customer 360 blocks and
> privacy/merge repair paths are now active.

`000026`-`000028` create and complete the tables every remaining original Section 8 requirement
needs, with the availability semantics and tenant-composite keys the accepted
sections use. The Go projections, consumers, workers and read models over
them are active behind the existing flags. These tables are not inert: their
nothing reads from them, so no surface reports a fabricated value â€” but neither
facts remain unavailable until their authoritative service sends them; no path
fabricates a value.

| Original requirement | Schema | Domain code | Status |
|---|---|---|---|
| 8.1 order_count distinct from completed | observed order count, order type and line facts | order ingress + commercial rebuild | COMPLETE |
| 8.1 category / item affinity | line facts and affinity projections | rebuildable business/branch rankings | COMPLETE |
| 8.1 / 8.3 service-recovery summary | support facts and recovery projection | authenticated support ingress + read block | COMPLETE |
| 8.3 order / RFQ / support refs on interactions and tasks | bounded related-ref columns | DTOs, revisions and history | COMPLETE |
| 8.3 automatic follow-up trigger engine | trigger configs/evaluations | idempotent system-authored tasks | COMPLETE |
| 8.4 customer value band | value definitions/projection | ordered thresholds and NO_BAND | COMPLETE |
| 8.5 corporate commercial projection | corporate commercial projection | order-type and spend projection | COMPLETE |
| 8.5 corporate RFQ projection and quote conversion | RFQ facts/projection | bounded outcomes + basis points | COMPLETE |
| 8.6 relationship signals (7) | `crm_relationship_signals` | deterministic, availability-aware signals | COMPLETE |

The CRM event-ingress extensions for
order-line, support and RFQ facts; the 40-field segment registry; the Customer
360 completion blocks; and their privacy, merge, concurrency and availability
coverage are included in this final-completion slice. External producer
absence is represented as `NOT_AVAILABLE`, not as a CRM-owned gap.

### Verification

- `go build ./...`, `go vet ./...`, `gofmt -l .` clean.
- Migration replay from `000001` through `000028` passes; `000028` rolls back
  and reapplies cleanly, leaving Sections 5-8I intact.
- The global drift guard (`internal/crmcontract`) passes at 196 routes with
  both new routes documented in OpenAPI and the route register. It caught them
  as undocumented the moment they were registered, which is what it is for.
- 10 real-Postgres loyalty tier tests pass, including the ledger-following,
  redemption-does-not-demote, concurrent-editor, tenant-scope and
  no-monetary-column cases.

One real defect was found and fixed by these tests before commit: a customer
with no loyalty account was told their state was NO_TIER but was not told which
rung they were climbing toward, so the record answered half the question a CRM
user opens it to ask.

## Section 9A â€” Food Catalogue Foundation â€” IMPLEMENTED / TESTED

Section 9A answers one question: **what does this vendor branch sell?** It is
the management-side authoritative catalogue and nothing else.

| Section | Scope | Status |
|---|---|---|
| 9A | Food catalogue foundation â€” menus, sections, items, variants, modifier groups, modifiers, relationships | **IMPLEMENTED / TESTED** |
| 9B | Pricing, availability, scheduling and operational profiles | **IMPLEMENTED / TESTED** |
| 9C | Discovery, integration and final Section 9 acceptance | **IMPLEMENTED / TESTED** |

### Section 9B â€” Pricing, Availability, Scheduling & Operational Profiles

### Section 9C â€” Discovery and integration

Section 9C is read-only service-facing composition. Catalogue/search, revision,
sellability and price-resolution routes are mounted only under
`/internal/vendor/v1/food`; they are never reachable through the public vendor
gateway and no frontend route is fabricated. Search is bounded and parameterized,
branch/capability scoped, and responses carry catalogue revision and generated-at
freshness markers. It reuses the Section 9B sellability and pricing authorities;
no migration or external service change was required. The global route surface is
262 operations (257 pre-9C plus five internal discovery routes).

### Sections 10/11 â€” production foundation and management surface

Migration `000033_vendor_production_foundation` establishes the shared capacity
engine for Cloud Kitchen and Pastry/Bakery. Migrations `000034` through
`000039` now adds worker lease/replay state and immutable change-decision history
after the separated IAM, slot-template fields, Cloud Kitchen packages/plans,
and first-class Pastry/Bakery closure schema. The current
management surface includes bounded slot generation, recurrence materialization,
production jobs, Cloud Kitchen lifecycle APIs, pastry lifecycle APIs, immutable
specification diff/change requests, quote revision/expiry/atomic acceptance,
milestones and conversion intents. The current Vendor route surface is 362
operations. Final domain certification is still pending targeted database
workflow and concurrency coverage.

Section 9B is implemented additively over the 9A catalogue. Migrations
`000031_vendor_food_operations` and `000032_vendor_section9b_permissions` add
timezone-aware schedules, operational availability, branch overrides,
append-only price history, effective price/availability resolution, preparation
and logistics profiles, the fail-closed inventory seam, and catalogue
revisions. Twenty-two authenticated management routes are registered; no
discovery or internal routes are introduced. The isolated real-Postgres food
regression and route/OpenAPI drift guard pass.

### What exists

`internal/features/food` implements the hierarchy â€” menu â†’ section â†’ item â†’
{variants, attached modifier groups â†’ modifiers} â€” over eight tables created by
`000029_vendor_food_catalogue`, with three permissions seeded by
`000030_vendor_section9a_permissions`. Thirty-nine public management routes are
registered under `/api/v1/vendor/food`; **zero internal routes**, because
nothing outside this service produces a menu and Section 9C owns the
read/consumption contract.

Load-bearing decisions, each with a test that fails if it is reversed:

- **Ownership is a database fact.** Composite foreign keys mean a section
  addressed through the wrong menu, a variant mutated through the wrong item, a
  modifier reached through the wrong group and a group borrowed from another
  business are all unstorable, not merely refused by a service check.
- **Money is integer minor units** with a bounded range enforced by CHECK. The
  business's currency (Section 6) is read for the response and never copied onto
  a catalogue row, so nothing can disagree with it.
- **Ordering is deterministic by construction.** Create, archive and reorder all
  end in one `resequence` routine, so live rows are always contiguous from zero
  with archived rows behind them. A deferred unique `(parent, position)`
  constraint per collection means an atomic permutation is legal in flight and
  provably unique at commit.
- **Archive, never delete**, with one exception: the item/modifier-group
  relationship row, which carries no historical authority of its own.
- **Modifier groups are business-level reusable definitions.** A branch-scoped
  grant may read, attach, detach and reorder them on its own branch's items but
  may not edit the shared definitions, because a group edited from one branch
  changes every other branch's items.
- **Publication reads Section 6/7 rather than re-deciding it.** A DRAFT
  catalogue can be prepared while the business is still onboarding; ACTIVE
  requires the business, branch and branch capability to all be ACTIVE.
- **Media stays opaque.** References are proven against Media File Service's
  existing private ownership API before storage; with no media authority
  configured the write is refused rather than storing an unverified reference.

### What is deliberately absent

No schedules, sold-out state, branch price or preparation overrides, effective
price/availability resolver or price history â€” those are 9B, and the schema
takes them additively. No discovery, projection, search or internal consumption
API â€” that is 9C. No orders, cart, checkout, inventory, dispatch or wallet call:
a catalogue price is commercial configuration, not settlement.

The item and menu lifecycles cannot express `SOLD_OUT`; a contract test fails if
one is added, so 9B's availability engine cannot be half-invented here.

### Verification

- `go build ./...`, `go vet ./...` and `gofmt -l` clean; `git diff --check` clean.
- Migrations `000029`/`000030` apply on the Section 8 tip, replay on a fresh
  database, and roll back and reapply without touching Sections 5-8. The
  structural guard asserts the ownership keys, the deferred position
  constraints, the query indexes and the permission seed â€” and that the database
  itself refuses a non-food capability, a negative price, an impossible
  selection range and an archived row without `archived_at`.
- The Section 8I global drift guard (`internal/crmcontract`) passes at **235
  routes** with all 39 new routes documented in
  `section9a-food-catalogue.openapi.json` and `API_ROUTE_REGISTER.md`. It caught
  them as undocumented the moment they were registered, which is what it is for.
- `internal/section9acontract` pins the 39-operation surface in both directions
  and fails if the contract ever describes a floating-point price, restates
  currency, exposes an upload/download or byte field, adds a Section 9B or 9C
  path, or makes a nutritional or medical claim about a dietary code.
- Real-Postgres feature, security, IAM, nested-IDOR, concurrency, HTTP and scale
  tests pass in `internal/features/food`, including: cross-tenant and
  cross-branch refusal on every read and write path, the analyst read-only
  shape, the branch-manager scope, the role/permission matrix asserted against
  the real migration seed, duplicate/foreign/incomplete reorder refusal,
  simultaneous-edit and duplicate-attach races resolving to exactly one winner,
  outbox-and-state committing together with no orphan on rollback, and an
  idempotent create retry that replays rather than duplicating.

### Defect found and fixed during implementation

None in accepted Sections 5-8. One design fault was caught by the tests inside
this section before it could ship: archiving a menu section originally left its
items behind, reachable by id but attached to a section no longer in the
catalogue. Archival now cascades to the items in the same transaction, and
`TestSection_CreateListReorderArchive` fails if it stops doing so.

## Section 12A â€” Advanced Shop / Grocery / Market / Wholesale: IMPLEMENTED

Section 12A implements the Vendor management and commercial foundation only.
The fully enabled production router now contains **443 routes**, including 58
new authenticated `/shop` operations. Migration tip is
`000043_vendor_shop_permissions`.

Implemented scope:

- branch operating models with capability mapping for supermarket, grocery,
  foodstuff dealer, market trader and wholesaler;
- bounded hierarchical taxonomy with tenant-scoped parent keys, ordering,
  depth checks, cycle prevention and optimistic concurrency;
- product masters, variants/SKUs, handling/temperature/physical profiles,
  opaque ordered media, package relationships, identifiers and exact lookup;
- first-class measurement definitions/families, platform and tenant scopes,
  tenant-scoped aliases on both platform and vendor definitions, standard
  integer base units, Nigerian local units, product/variant and optional branch
  calibrations, exact versus calibrated-approximate
  conversions, rational fractions and price portions;
- branch listings, retail/wholesale coexistence, sell-unit selection,
  branch-only calibration assignment without promoting it to business truth,
  deterministic price precedence, MOQ/order multiples, non-overlapping
  quantity bands, overflow guards and append-only price versions;
- product-applicable fresh attributes for produce, tubers, plantain, fish,
  meat and vegetables, plus variable-measurement tolerances in basis points;
- monotonic scoped catalogue revisions, separated IAM grants, expected-version
  writes, shared idempotency, transactional audit and transactional outbox;
- repository-level Shop/Food/Cloud Kitchen/Pastry isolation and SQL ownership
  predicates for business, branch and nested resources, plus database category
  and packaging cycle guards;
- batched product, variant and fresh-option list hydration at the 2,000-product
  / 5,000-variant moderate fixture;
- dedicated strict command/query OpenAPI plus bidirectional Section 12A and
  global route parity.

Part A deliberately introduced none of the inventory authority subsequently
implemented by Section 12B. No Order, Wallet/payment or logistics execution
authority is present in either part.

## Section 12B â€” Shop inventory, availability and discovery: IMPLEMENTED

Section 12B completes the Vendor-owned Shop operational authority. Migration
tip is `000046_vendor_shop_inventory_permissions`; the fully enabled router has
**496 routes**: 42 authenticated inventory/bulk routes and 11 HMAC-v2 internal
discovery/reservation routes added to the 443-route Section 12A baseline.

Implemented scope:

- branch/variant stock bases, non-negative materialized balances, append-only
  movements and monotonic inventory/catalogue revisions;
- draft/post/cancel receipts, lot/batch/expiry evidence, FEFO/FIFO allocation,
  synchronous expired-lot exclusion and reconciliation;
- atomic multi-line reservations with confirm, commit, release, cancel and
  expiry transitions, controlled preorder/backorder policy and allocation;
- controlled adjustments, quarantine/release/disposal, inter-branch transfers
  with partial receipt and damaged/missing resolution, and stocktakes with
  variance posting;
- variable fulfilment measurements, reorder rules, complete alert generation
  and ACK/RESOLVE lifecycle;
- atomic and dry-run bulk catalogue, price, listing and inventory operations
  with leases, bounded retry, dead-letter state and row-level reports;
- repeatable-read discovery projections with retail/wholesale price precedence,
  MOQ/multiple, fresh/handling metadata, sellability, availability and both
  catalogue and inventory revisions;
- fail-closed IAM and branch scope, nested IDOR protection, HMAC caller
  allowlisting/nonces, idempotency fingerprints, transactional audit/outbox,
  concurrency hard gates and 10,000-product/20,000-variant scale coverage.

Authority boundary: Vendor owns catalogue, inventory and availability truth,
not customer Orders, checkout, payment/Wallet settlement, purchasing/suppliers,
or Dispatch/Taxi/Hauling execution. Internal reservation callers must provide
their own source reference; a reservation is not an Order or payment promise.

## Section 13 â€” Catalogue, Taxonomy and Search

**Status: VERIFIED; GO.** Subsections 13Aâ€“13I are implemented, and every
Section 13 hard gate previously deferred was executed in 13I against real
PostgreSQL. See [architecture/14_CATALOGUE_TAXONOMY_SEARCH.md](architecture/14_CATALOGUE_TAXONOMY_SEARCH.md)
for the end-to-end reference and
[SECTION13_FINAL_CERTIFICATION_REPORT.md](SECTION13_FINAL_CERTIFICATION_REPORT.md)
for the evidence.

| Subsection | Scope | Status |
|---|---|---|
| 13A | Canonical taxonomy foundation â€” namespaces, versions, facets, nodes, publication lifecycle, change history, idempotent commands | VERIFIED |
| 13B | Food/Shop seeds and Nigerian vocabulary â€” fingerprinted seed bundles, aliases, synonyms, authority vocabulary, append-only seed evidence | VERIFIED |
| 13C | Mapping, semantic tags, dietary claims and governance â€” typed mappings against published versions, facet-mapping replacement, claim verification policy, governance workflow | VERIFIED |
| 13D | Separate Food/Shop search projections â€” generation-keyed denormalised documents, Unicode-aware normalisation preserving Nigerian terms | VERIFIED |
| 13E | Search engine â€” exact/prefix/full-text/alias/synonym/bounded-typo matching, bounded ranking, filters, facets, suggestions, stable cursor pagination | VERIFIED |
| 13F | Operational signals â€” availability, serviceability, trust, reliability; checkout revalidation; `UNKNOWN` never upgraded | VERIFIED |
| 13G | Redis cache and incremental projection â€” revision-keyed cache identity, event idempotency, stale-event rejection, PostgreSQL fallback | VERIFIED |
| 13H | Rebuild generations, workers and recovery â€” build-alongside, atomic activation, lease-based workers, retry, dead-letter replay | VERIFIED |
| 13I | APIs, IAM/HMAC, security, scale, OpenAPI and final certification | VERIFIED |

### Migrations

Section 13 owns `000047`â€“`000059`. The tip after 13I is **000059**
(`000059_vendor_taxonomy_search_permissions`), which adds the Section 13
permission vocabulary and its role grants. Every migration is paired up/down and
replays cleanly; none contains `CREATE EXTENSION`.

### Routes

Section 13 adds **60 routes**, taking the fully-enabled Vendor surface to
**556**. The count is asserted from production route registration by
`internal/crmcontract`, not from prose.

- 10 authenticated vendor classification routes under `/api/v1/vendor/taxonomy`
- 33 root-mounted platform taxonomy administration routes
- 6 root-mounted internal Food/Shop search routes
- 11 root-mounted ranking and projection administration routes

All 50 internal routes use the existing certified `shared/go/serviceauth`
HMAC-v2 contract with an explicit caller allowlist and durable nonce replay
protection through `vendor_service_nonces`. No new authentication mechanism was
created.

### Three defects found and fixed during 13I

These were genuine, previously-undetected defects in Sections 13D, 13H and the
13D test fixture. They are recorded here rather than quietly repaired because
each represents a class of failure worth remembering.

1. **Projection rejected documents with empty optional vocabulary (13D, product
   defect).** The document builders bound Go `nil` slices to `NOT NULL` array
   columns, so a Food item not yet on a menu, or a Shop variant without a
   barcode, failed projection outright and silently never became discoverable.
   Both are ordinary catalogue states. Fixed in the builders so the invariant
   holds for every backend.

2. **Concurrent generation activation surfaced as an opaque 500 (13H, product
   defect).** Activation runs at `SERIALIZABLE` and snapshots at the
   advisory-lock statement, so the loser of an activation race could be aborted
   with SQLSTATE 40001 instead of returning the intended domain conflict. The
   invariant held â€” only one generation activated â€” but the API reported an
   internal error for a routine race. Serialization failures now map to
   `ErrGenerationActivationConflict`; the isolation level was deliberately left
   at `SERIALIZABLE`.

3. **13D hard gate could not run (test defect).** Its projection reset used
   `TRUNCATE` without `CASCADE` and predated the Section 13E term tables that
   hold a foreign key onto the documents. The gate had been failing at setup.

### Production rebuild closure (Section 13 final)

The previous certification recorded one non-blocking limitation: full-rebuild
workers were not started, because Section 13H's `SearchFullRebuildSource` port
had no concrete implementation. **That gap is now closed.** A privileged rebuild
request progresses on its own from `BUILDING` to `ACTIVE` against the real
authoritative Food and Shop domains.

**Authoritative rebuild source â€” `internal/searchrebuild`.** A new
composition-level package implements `SearchFullRebuildSource` by reading the
domains that own each value: Food items, menus, prices and availability from
Section 9; Shop products, variants, listings, sell units, identifiers and
inventory balances from Section 12; canonical classification, vocabulary,
semantic tags and dietary claims from Sections 13Aâ€“13C; business verification
(trust) from Section 7; branch geography (serviceability) from Section 6.

It never reads `food_search_documents`, `shop_search_documents`, the search term
tables or Redis. That is the property that makes a rebuild a recovery mechanism
rather than a copy: if the projection were its own source, a corrupted
projection would rebuild into an identically corrupted one. The gate proves it
by corrupting the live projection before rebuilding and checking the result
matches the catalogue instead.

The package sits outside `internal/features` because a rebuild bridges five
features at once, and because `internal/architecture_test.go` forbids one
feature importing another's repositories.

**Bounded, restart-safe iteration.** Batches are keyset-paginated over immutable
primary keys (`food_items.id`, `shop_branch_listings.id`), never OFFSET â€” an
OFFSET scan over a mutable catalogue silently skips and duplicates rows as data
shifts beneath a long rebuild. The batch ceiling stays at the certified 200.
Every enrichment is a bulk `= ANY($1)` over the batch, so query count per batch
is constant regardless of batch size; the gate measures this and observed zero
growth between a 1-document batch and a 7-document batch. The checkpoint is the
last id of the previous batch, so a worker that dies resumes where it stopped.

**Workers started.** `cmd/section13.go` builds two `ProjectionWorker`s and
`cmd/main.go` starts them through the existing `jobs.Job` runtime â€” no second
scheduler. One claims `FULL_REBUILD`/`SCOPED_REBUILD` with a two-minute lease
and a claim batch of one; the other claims `GENERATION_VALIDATE_ACTIVATE` with a
30-second lease and a claim batch of five, so a long rebuild holding its lease
never delays activating a generation that has already finished. Both are gated
on `VENDOR_SEARCH_ADMIN_ENABLED`, and standing them down freezes rebuilds
without taking discovery offline.

**Unknown stays unknown.** Reliability has no authoritative source anywhere in
Vendor Service, so every rebuilt document carries reliability `UNKNOWN` with no
basis points rather than an invented perfect score. There is no authoritative
promotion source either, so a rebuild never promotes anything.

### Three further defects found and fixed by the closure gates

1. **A completed build could never become READY (product defect, 13H).**
   `CompleteGenerationBuild` recorded the completion but enqueued nothing, so a
   finished rebuild sat at `BUILDING` forever with no job left to validate it â€”
   the exact symptom the previous limitation described, and it would have
   persisted even with a source implemented. Completion, its evidence and the
   follow-on `GENERATION_VALIDATE_ACTIVATE` job now commit in one transaction,
   so a crash between them cannot strand a generation. The job is deduplicated
   on `(namespace, generation)`.

2. **A signal with unknown reliability could not be stored at all (product
   defect, 13F).** `ApplyOperationalSignals` bound nullable basis-point
   parameters whose only appearances were inside `jsonb_build_object` and
   `COALESCE`, so PostgreSQL could not infer their type and the statement failed
   with "could not determine data type" whenever they were NULL. NULL basis
   points are the *normal* shape for reliability â€” Section 13F requires unknown
   to stay unknown â€” so the one case the section mandates was the one case that
   could not be written. Every parameter in both signal statements is now
   explicitly typed.

3. **Operational signals could not be applied alongside a rebuild.** Signals are
   applied by document id, which only the projector knew how to derive.
   `FoodSearchDocumentID`/`ShopSearchDocumentID` now export that derivation, so
   a caller producing a document and its signals in one pass uses the same
   function rather than reimplementing the hash.

### Closure evidence

| Gate | Result |
|---|---|
| Real Food rebuild end-to-end (domain â†’ BUILDING â†’ READY â†’ ACTIVE, previous RETIRED) | PASS |
| Real Shop rebuild end-to-end | PASS |
| Rebuild reads the domain, not the projection (corruption gate) | PASS |
| Old generation keeps serving while the rebuild runs | PASS |
| Rebuild failure preserves the ACTIVE generation; retry completes | PASS |
| Unvalidated generation can never be activated | PASS |
| Bounded keyset batching, no repeats, checkpoint advances | PASS |
| No per-document query growth | PASS |
| Rebuild runs with no Redis at all | PASS |
| HMAC-v2 rebuild request drives BUILDING â†’ ACTIVE through the real API | PASS |
| Unsigned rebuild request rejected and queues nothing | PASS |
| Scale: 1,507 Food documents rebuilt in bounded batches of â‰¤200 | PASS |
| Rebuild keyset scan is index-capable | PASS |

### Test infrastructure note

`tools/vendor-test-runner` now accepts an optional package-name substring as an
argument, so one gate can be re-run in isolation during development. Each
selected package still gets its own freshly migrated database cloned from the
same template; the full regression is what runs with no argument, and
certification always runs the full regression.
## Section 14A - Inventory Architecture & Generic Domain Foundation

**Status: IMPLEMENTED; certification pending.**

The existing reserved `internal/features/inventory` boundary is now an
implemented generic Inventory application/domain foundation.

Section 14A establishes:

- tenant-scoped Inventory actors
- business/branch scope
- Inventory Item Resolver port
- Inventory Evidence Recorder port
- Inventory Event Publisher port
- deterministic Inventory Clock
- transaction boundary
- generic Inventory feature controls

The certified Section 12 Shop inventory implementation remains operational and
has not been migrated, renamed or redirected.

No generic stock tables, generic Inventory HTTP routes or Food inventory
implementation are introduced by Section 14A.


## Section 14B-14I - Generic Inventory: stock authority, operations and certification

Section 14 is implemented and certified. Generic Inventory is now the stock
authority for the Vendor Service.

| Part | Scope | Status |
|---|---|---|
| 14A | Generic Inventory architecture and domain foundation | IMPLEMENTED |
| 14B | Generic inventory item identity and stock basis | IMPLEMENTED |
| 14C | Shop compatibility bridge | IMPLEMENTED |
| 14D | Core stock, lots and the movement ledger | IMPLEMENTED |
| 14E | All-or-nothing reservations and concurrency | IMPLEMENTED |
| 14F | Stocktakes, transfers, quarantine, reorder and alerts | IMPLEMENTED |
| 14G | Food inventory seam (fail-closed) | IMPLEMENTED |
| 14H | Workers, IAM and internal HMAC contracts | IMPLEMENTED |
| 14I | Scale, security, migration, OpenAPI and certification | CERTIFIED |

Migration range: `000060` - `000066`. Migration tip after Section 14I is
`000066`; Section 14I added no new migration.

### What Inventory owns, and what it does not

Inventory owns generic inventory item identity, stock basis, balances, lots,
movements, reservations, stocktakes, transfers, quarantine, reorder rules,
alerts, inventory revision and expiry state.

Shop remains the owner of products, variants, listings, sell units and
catalogue measurement configuration. The certified Section 12B Shop inventory
surface is unchanged and still live; Section 14 bridges to it through inventory
item identity rather than absorbing it. Generic Inventory does not import Shop
domain or repository code, and the dependency runs one way only: Shop depends
on Inventory.

### Certified invariants

- PostgreSQL is authoritative. Neither the Inventory nor the Shop feature
  package references Redis at all, so Redis being unavailable cannot change,
  invent or lose a balance.
- Available quantity never becomes negative, and quarantined and expired stock
  is excluded from it.
- The movement ledger is append-only. A database trigger refuses every UPDATE
  and DELETE unconditionally, including from a caller already holding the
  authorized-mutation session flag.
- Physical stock tables refuse any write that did not arrive through an
  authorized inventory mutation.
- Reservations are all-or-nothing and idempotent on
  `(source_type, source_ref)` plus a request fingerprint. Replaying a request
  returns the current durable reservation state; the same reference with a
  different fingerprint is a conflict.
- A stale stocktake snapshot cannot overwrite newer stock truth.
- Transfers refuse to move stock between items that do not share a stock
  identity, because the quantity would not mean the same thing at both ends.
- `INVENTORY_LINKED` Food availability fails closed. There is no HYBRID mode,
  and recipe authority remains a Section 16 seam.

### Section 14I certification results

Every gate below ran against real PostgreSQL 16 and real Redis through
`scripts/test-vendor-isolated.ps1` / `tools/vendor-test-runner`, which gives
each test package its own database cloned from a freshly migrated template.

- Full Vendor Service regression: 37/37 packages pass.
- Section 12B Shop regression: passes, including the Shop inventory
  concurrency, security, bulk, discovery and internal-contract suites.
- Migration certification: empty database to tip, rollback of the tip, and
  reapply; Section 14's stock schema survives the rollback and
  `vendor_job_controls` is genuinely dropped rather than merely untracked.
- Movement immutability: certified against the running database, not from
  reading the migration text.
- Tenant, branch and nested-identifier isolation: certified against the real
  service and the real database.
- Concurrency: the Section 14E and 14F PostgreSQL race matrices execute and
  pass; these were pending a test database until Section 14I.
- Scale: 21,000 generic inventory items, 21,000 balances, 63,000 movements,
  6,996 lots, 3,000 reservations, 3,000 reorder rules, 1,800 alerts across 4
  businesses and 12 branches.
- Query plans: all eleven hot paths use index scans at that scale, sub
  millisecond. No index was added, because no plan justified one.
- Route parity: live router = OpenAPI = API route register, enforced by
  `internal/section14contract`.

### Defects found and fixed during Section 14I

1. **Migrations could not apply to an empty database.** `000063` used
   `CREATE OR REPLACE VIEW` to change `inventory_balance_availability` columns
   from numeric to bigint, which PostgreSQL refuses. The chain aborted at
   `000063`, so no fresh environment could ever reach the tip and no
   database-backed Section 14 test could run. Fixed by dropping and recreating
   the view, which is what `000063`'s own down file already did correctly. A
   new migration could not have fixed this, because the chain died before
   reaching it.
2. **Stocktake creation was impossible whenever the snapshot returned rows.**
   `CreateStocktake` executed an INSERT inside an open `rows.Next()` loop on
   the same connection; pgx refuses this with `conn busy`. Fixed by reading the
   snapshot to completion before writing any line. The same interleave pattern
   was then swept for across the whole Inventory package and is now absent.
3. **A stale Section 9B assertion** still expected the pre-Section-14 constant
   `NOT_PRESENT` for the Food inventory seam, which Section 14G had renamed to
   `NOT_SUPPORTED`. The test was updated to the current seam vocabulary.
4. **Two Section 14F transfer fixtures** built a destination stock basis that
   did not share the source's stock identity and had no enabled sell unit. The
   production guards were correct and were left alone; the fixtures were
   corrected.

### Known limitations

- Generic receipt operations (`CreateReceipt`, `PostReceipt`,
  `CancelReceipt`) exist on the generic stock service and are certified at the
  service layer, but are deliberately not exposed as generic public routes.
  Goods receiving reaches HTTP through the Section 12B Shop surface, and no
  duplicate generic route was added purely for symmetry.
- Reorder rules carry an opaque `supplier_ref` only. Supplier and purchase
  order authority belongs to Section 15.
- `RecipeAvailabilityResolver` remains an unimplemented seam. Section 16 owns
  recipe authority.
- Five tracked test files under `internal/database` and most of
  `internal/features/shop` are stored with CRLF line endings, so a bare
  `gofmt -l` on Windows lists them. Their content is correctly formatted; this
  is a checkout artifact, not a formatting defect.


## Section 15 - Suppliers, Purchasing and Goods Receiving

Section 15 is implemented and certified.

| Part | Scope | Status |
|---|---|---|
| 15A | Suppliers, contacts, terms, supplier products | IMPLEMENTED |
| 15B | Purchase orders and approval policies | IMPLEMENTED |
| 15C | Goods receiving, Inventory posting, discrepancies | IMPLEMENTED |
| 15D | Operations, APIs, workers, scale and certification | CERTIFIED |

Migration range: `000067` - `000070`. Migration tip after Section 15D is
`000070_vendor_purchasing_operations`.

Routes: 56, all authenticated under `/api/v1/vendor/purchasing`. No internal
HMAC surface: no other service drives purchasing.

### What Section 15D added

- `vendor_purchasing_alerts` plus the operational indexes the 15D read paths
  need, and two new read permissions
  (`vendor.purchasing.performance.read`, `vendor.purchasing.dashboard.read`).
- Supplier performance derived from purchasing evidence, with honest
  insufficient-history semantics.
- Purchasing dashboards, outstanding-delivery views and operational alerts,
  built as indexed queries rather than projection tables.
- Two durable workers on the existing job coordinator.
- Six operational routes, the Section 15 OpenAPI contract, the route register
  block, and `internal/section15contract`.

### Defects found and fixed during 15D certification

1. **Every goods-receipt reversal failed (P0).** The reversal movement insert
   applied a unary minus to an untyped bind parameter (`-$6`). PostgreSQL
   cannot resolve `- unknown` and refused the statement, and the caller wrapped
   the failure as "reversal unsafe" - so a plumbing fault read as a legitimate
   business refusal. Fixed by casting before negating.
2. **Reversal could not reconcile the purchase order (P0).**
   `reconcilePOReceivingAfterReversalTx` ran a query inside an open
   `rows.Next()` loop on the same transaction; pgx refuses that with
   `conn busy`. Fixed by reading the line set to completion first.
3. **Purchased stock could not be sold (P0).** Goods receiving created an
   inventory lot only for lot- or expiry-tracked items. Generic Inventory
   allocates reservations and adjustments out of lots, so stock received from a
   supplier for a simple item sat on the balance, read as available, and could
   not be reserved or adjusted. Fixed by creating a lot for every posted line,
   matching Section 14's own Receive. Lot *metadata* is still required only for
   genuinely tracked items.
4. **Section 15 was invisible to the service-wide contract.** Its 56 routes
   were live but absent from every specification, from the route register and
   from `internal/crmcontract`'s router - the same hole Section 14I found and
   fixed for Section 14. Closed; the accepted surface moved from 590 to 646.

### Certification results

All against real PostgreSQL 16.

- 26 Section 15 tests pass, including the canonical discrepancy example
  (ordered 100, delivered 98, accepted 90, damaged 5, rejected 3, short 2 ->
  Inventory +90), three-receipt partial receiving, exactly-once confirmation
  under six concurrent racers, lot/expiry pass-through, cost variance leaving
  the approved order untouched, and append-only cost history.
- Cross-vendor isolation across twelve probes, all using identifiers that
  really exist and really belong to another business.
- Branch isolation, including refusing a branch filter outside the caller's
  scope.
- No wallet mutation, checked both at runtime and by grepping the purchasing
  source for wallet and payment client references.
- Scale: 52,000 purchase orders, 156,000 order lines, 39,000 goods receipts,
  26,000 supplier products, 14,000 discrepancies, 9,000 cost observations.
- Query plans: all 15 hot paths on index scans, sub-millisecond.

### Known limitations

- Supplier performance rates need at least three observations before they
  report a value. Below that they are `INSUFFICIENT_HISTORY` with a null value,
  which is deliberate.
- `hours_online`-style operator metrics and a cost-variance approval UI are not
  part of the backend surface.
- Financial settlement remains a separate, future authority. Purchasing records
  cost and never pays anyone.

<!-- SECTION 16B RECIPES -->
## Section 16B

Implemented:

- Recipe master
- RecipeVersion
- Ingredient BOM
- Packaging requirements
- typed Food/Pastry/Cloud Kitchen targets
- exact normalization
- expected yield
- expected waste
- deterministic scale calculations
- transactional activation
- historical immutability
- IAM
- audit/outbox
- public Vendor APIs

Certification remains deferred under the current implementation-first workflow.

<!-- SECTION16C_IMPLEMENTATION -->
## Section 16C implementation

Status: IMPLEMENTED — CERTIFICATION DEFERRED.

Implemented backend:

- ProductionBatch DRAFT / PLANNED / IN_PROGRESS / COMPLETED / CANCELLED / FAILED.
- frozen RecipeVersion Ingredient and Packaging requirements;
- exact Section 16B waste/yield scaling semantics;
- Section 14 atomic controlled-stock reservation;
- explicit Inventory top-up for actual use above the original reservation;
- actual Ingredient / Packaging consumption;
- immutable actual-waste evidence;
- production-time Packaging consumption policy;
- ON_DEMAND / BATCH_STOCK;
- PRODUCTION_COMPONENT prepared-stock output;
- exactly-once Inventory finalization;
- capacity/job reference to the existing Sections 10/11 production engine;
- tenant and branch authorization;
- audit/outbox events;
- command idempotency;
- optimistic version checks.

Not implemented here: Section 16D COGS/cost analytics and final Section 16
certification.

## Section 16D - Operational cost, COGS and production variance

**Status: IMPLEMENTED.** Section 16D is the operational cost layer over
Sections 15/16A/16B/16C, and the phase that carries the final Section 16
certification.

### What was built

`internal/features/cost` (about 3,800 lines): exact money arithmetic, a narrow
Section 15 purchase-cost read port, three cost strategies, cost policy
management with OCC/audit/outbox, the recipe COGS engine, production
expected/actual/variance, immutable snapshot persistence with component-level
evidence, bounded read models, and 17 HTTP routes.

Migration `000074_vendor_operational_cost`: `vendor_cost_policies`,
`vendor_cost_snapshots`, `vendor_cost_snapshot_components`, database-level
snapshot immutability triggers, production-cost uniqueness and command
idempotency constraints, three permissions and their role grants.

### Defects found and fixed during this phase

**The 34 "phantom" Section 16 OpenAPI operations were a test-harness gap, not a
product defect.** `internal/crmcontract`'s `buildProductionRouter` never
registered `productionmaster`, `recipes` or `productionbatch`, although
`cmd/main.go` had registered all three since their own sections. The gate was
therefore measuring a router that was missing Section 16 entirely - the same
class of hole Section 14I had closed for Section 14. Registering those routes
in the stand-in router resolved it; the historical claim should not be carried
forward.

**The migration rollback helper had a hardcoded 64-step cap.** With 74
migrations, `rollbackUntilRemoved` could not reach an early version. It now
derives its bound from the number of applied migrations, so the bug cannot
recur the next time the repository grows - replacing 64 with a larger constant
would only have deferred it.

### Route surface

The accepted Vendor surface moved from 646 to **712**: 646 at the close of
Section 15, plus the 49 Section 16A/16B/16C routes that were live but
unregistered in the gate, plus Section 16D's 17. The figure is measured from
production route registration, never targeted.

### Known limitations, stated rather than hidden

- **No end-to-end purchase-evidence fixture.** The REVERSAL cancellation rule
  is asserted against the real Section 15 table and the SQL predicate, and the
  weighted-average and currency rules are asserted through a reader stub. A
  fixture that drives a full goods-receipt through Section 15's receiving
  pipeline to produce genuine `OBSERVED`/`REVERSAL` rows would be stronger and
  is not present.
- **Recipe and production costing are not yet exercised end-to-end against a
  seeded recipe and batch.** The engines are unit- and schema-tested; a
  fixture that seeds a full 16A/16B/16C chain and asserts a specific COGS
  figure is not present.
- **No scale fixtures or query-plan evidence for the cost paths.** Indexes were
  designed for the documented access patterns and exist in migration 000074,
  but no thousands-scale corpus has been built and no EXPLAIN output has been
  captured for the cost queries.
- **Concurrency is enforced structurally rather than by race test.** The
  uniqueness constraints that make duplicate production cost evidence
  impossible are asserted to exist; a test that races two finalizations is not
  present.

`go test -p 1 ./...` remains unsupported for this service. The canonical full
integration regression is `go run ./tools/vendor-test-runner`; see docs/README.md.

### Section 16D - final certification gates closed

The five gates the previous pass left unevidenced are now closed.

**End-to-end 16A -> 16D.** A synthetic chain seeded through every section's own
constraints - ingredient and packaging master data, branch inventory mapping,
a recipe version composed while DRAFT and then activated, a frozen production
batch, Section 15 purchase evidence on a real purchasing spine - with exact
assertions: recipe COGS 2520, production expected 2520 (asserted NOT to be the
2698 a second waste gross-up would give), production actual 2730, variance
+210, per-unit 252, per-serving 315. Historical immutability proven against a
live price change: a fresh preview moves, the stored snapshot's total,
component unit cost and source evidence reference do not.

**REVERSAL through the real Section 15 pipeline.**
`internal/features/purchasing/section16d_cost_evidence_test.go` orders,
receives, inspects, confirms and then reverses actual goods receipts through
the Section 15 services, and asserts the Section 16D contract against the
result: latest cost falls back from 900 to the surviving 500, the weighted
average moves from 700 to exactly 500 (the reversed observation contributes
neither quantity nor cost), and once every receipt is reversed the reader
reports absence rather than zero. A fully rejected delivery never becomes cost
evidence at all.

**Concurrency.** Six races on real transactions, released from a barrier with
no sleep anywhere: simultaneous recipe snapshot (one row, both callers get the
same id, component evidence intact), duplicate production expected and actual
(exactly one record each, retry converges), policy and manual-cost OCC (exactly
one winner, one version conflict, no lost update, exactly one audit and one
outbox row), and historical reads held stable through 5 concurrent policy
edits. **5 consecutive full passes.**

**Scale and query plans.** A 165,000-row synthetic corpus and twelve captured
plans - see `docs/architecture/19A_OPERATIONAL_COST_QUERY_PLANS.md`.

**One real defect found by the query-plan gate and fixed.** The highest-cost
ingredients read hash-joined its bounded snapshot window against the entire
component table: 36.8ms and growing with total evidence rather than with the
window. An index nested loop measured 4.9ms, so the better plan existed and the
planner was not choosing it; an explicit id array, a LATERAL join and a
MATERIALIZED CTE all failed to move it. Fixed with a covering index added to
migration `000074` (unreleased and local-only at the time) plus splitting the
read into two bounded queries: **24.8ms -> 3.1ms**, now bounded by the window.

**One test defect found and fixed.** `TestSection16DMigrationApplyRollbackReapply`
assumed 000074 was the migration tip. It is not any more - `000075` landed
during this work - so it now rolls back until Section 16D is removed using the
generic helper, the same lesson as the hardcoded 64-step cap.

## Section 17B - Checkout validation and Inventory reservation

**Status: IMPLEMENTED.** Section 17B turns an ACTIVE Section 17A cart into a
validated checkout candidate. It is not a quote, not an authorization and not
an order: there is no quote lock, no logistics price, no payment, no
ParentOrder and no VendorOrder anywhere in it, and the contract package refuses
such a path structurally rather than by convention.

### What was built

`internal/features/checkout` (about 2,700 lines): the checkout session
aggregate, authoritative line revalidation, vendor/branch grouping, the
Section 14 reservation coordinator with deterministic compensation, TTL and
crash recovery, variable-weight money preparation, and three customer routes.

`internal/features/cart/checkout_transitions.go`: the ACTIVE <->
CHECKOUT_IN_PROGRESS transitions. Cart status is cart-owned state, so the
transition lives in the cart feature even though checkout is what drives it.

`internal/features/inventory/source_lookup.go`: two Section 14 reads checkout
needs and Section 14 did not yet expose - `FindReservationBySource`, which is
how a crashed coordinator finds a hold it never recorded the identity of, and
`ResolveShopVariantItem`, which turns a Shop catalogue selection into the
generic Inventory identity a reservation is made against.

`cmd/section17b_adapters.go` and `cmd/section17b.go`: the composition root.
Food, Shop and Inventory are reached only through the narrow ports in
`checkout/ports.go`, so the feature package imports none of them.

Migration `000076_vendor_commerce_checkout`: `commerce_checkout_sessions`,
`commerce_checkout_groups`, `commerce_checkout_lines`,
`commerce_checkout_events`, and two new actions on the Section 17A cart
history vocabulary. Contract:
`docs/openapi/section17b-commerce-checkout.openapi.json`.

### The decisions that shaped it

**Section 14 is the stock authority, and only Section 14.** Section 14's own
certification says so. Shop's Section 12B inventory surface remains live and
unchanged, and Section 17B neither reads nor writes it. That has a practical
consequence worth stating plainly: the two ledgers are still separate during
the compatibility bridge, so a Shop variant that has stock in the Section 12B
tables and none in the generic ones will fail checkout. That is fail-closed,
and it is the correct reading of "generic Inventory is now the stock
authority", but a deployment mid-migration must finish the bridge before
enabling `COMMERCE_CHECKOUT_ENABLED`.

**Shop answers the catalogue; Section 14 answers the stock.** Section 12B's
`ResolveSellability` blends both into one boolean, so the adapter splits them
and drops Shop's stock verdict - asking two ledgers the same question is how a
checkout ends up holding stock one of them never had.

The split is an allowlist of the six answers Section 17B may defer
(`SELLABLE` plus the five stock outcomes), and a refusal of everything else,
rather than a list of catalogue denials to reject. That direction is the
point: Shop owns its reason vocabulary and may grow it, and a denylist would
let a reason code added later fall through and sell. A unit test pins the
deferred set, including an invented future code, so widening it has to be a
deliberate edit.

**Food reserves nothing.** Its production architecture exposes availability,
not a reservation interface: `ResolveSellability` already answers business,
branch, capability, menu, service window, section, item, operational state and
the fail-closed `INVENTORY_LINKED` seam as one decision. Section 17B enforces
that decision - which is the actual change, because Section 17A deliberately
lets an unsellable item sit in a cart. Reserving raw Shop stock on Food's
behalf would have been inventing a contract that does not exist. A database
CHECK makes a Food group carrying a reservation unstorable.

**The reservation basis fails closed rather than guessing.** There are exactly
two answers this platform can defend today: a `VARIABLE_FULFILMENT` basis
reserves in its own stock measure (the same basis an actual fulfilment
measurement is later compared against), and a `SAME_UNIT` basis whose measure
is the sell unit's measure at a whole-unit fraction reserves one-for-one -
which is how the Section 12B sellability engine already compares a requested
quantity. `EXACT_CONVERSION`, a fractional sell unit and a mismatched measure
all refuse. Section 14 explicitly declines to invent conversion values and
Section 12 keeps measurement authority, so a checkout that derived one would be
reserving a number that means something else. A test drives a half-paint sell
unit and asserts the refusal.

**Multi-vendor atomicity is compensation, not one transaction.** Section 14
owns one transaction per reservation, so cross-group atomicity cannot be a
single database transaction without reaching inside it. Instead each group row
and its reservation source reference are committed BEFORE the first Inventory
call. After that commit there is always a durable key naming every reservation
that might exist, which is what makes both compensation and crash recovery
possible - and what makes every retry idempotent, because Section 14 keys
reservations on `(source_type, source_ref)`. The end state is always every
required group RESERVED, or FAILED with no hold left behind.

**A validation refusal is a session, not an error envelope.** The per-line
reason codes are what a customer needs in order to fix the cart, and an error
envelope cannot carry them. Errors are reserved for cart preconditions
(not ACTIVE, empty, no delivery address, stale version) and for an authority
that could not answer at all.

**A price change is reported, never silently applied to a quote.** The
candidate is quoted from the authoritative price, the cart snapshot is kept
beside it, and `price_changed` says so. Creating the final immutable quote is
Section 17D's decision.

**The TTL is configuration, never a constant.** `NewService` refuses to build
without one inside `[1 minute, 24 hours]`, and `Config.Validate` refuses to
start the service on a value outside `[60, 86400]` seconds or on a stranded
grace that is not shorter than the TTL. The `.env.example` default is 600
seconds; nothing in the code path assumes it.

### Concurrency and recovery, proved on real PostgreSQL

Every gate below runs against real PostgreSQL, the real Section 17A cart
service, the real Section 12 Shop catalogue, the real Section 14 Inventory
authority and the real production adapters from `cmd/section17b_adapters.go`.
A checkout that only ever reserved from a fake would prove nothing about the
one property this section exists for.

- **Two customers, three units, both want three.** Released from one barrier:
  exactly one VALIDATED, exactly one FAILED with `STOCK_UNAVAILABLE`,
  available lands on zero, and the balance invariant
  (`reserved + quarantined <= on_hand`, none negative) holds.
- **The same customer runs checkout twice.** The second request returns the
  same session; the stock is held once.
- **Cart mutation during validation.** The authoritative reads happen without
  the cart row lock; the lock path re-asserts the version and refuses anything
  that moved underneath it. Nothing is held for the refused attempt.
- **Stock written down between cart and checkout**, through Section 14's own
  adjustment authority: refused cleanly, no partial hold.
- **Multi-vendor partial failure.** Vendor A succeeds, vendor B cannot; A is
  compensated, both balances are exactly as they started, the cart is ACTIVE.
- **Reservation expiry vs validation.** The TTL is reached by moving an
  injected clock, never by sleeping. A read never reports an out-of-date hold
  as valid, the stock returns, the cart is handed back, and an immediate retry
  produces a new session.
- **The sweep with nobody watching.** No request touches the session; the
  recovery pass still releases the hold, and running it twice changes nothing.
- **A crash mid-coordination.** The session is forced into the exact shape a
  dead process leaves - still VALIDATING, coordination IN_PROGRESS, the hold
  real but its identity never recorded - and recovery finds it from the source
  reference alone, releases it, records what it found, and hands back the cart.
  The mirror case is also pinned: a coordination that started a second ago is
  not swept out from under itself.
- **Self-healing without Section 17B at all.** Section 14's own
  reservation-expiry worker releases a lapsed checkout hold with no checkout
  code involved.
- **Customer isolation**, at both the service and the transport boundary: a
  guessed session id returns 404 to a read and to a cancel, and the owner's
  hold survives the attempt. The wire shape carries no customer identity.

### The service-wide contract gate now sees Section 17

`internal/crmcontract` builds its stand-in router from the same `Register*`
helpers `cmd/main.go` calls and holds it against every OpenAPI document and the
route register in both directions. It registered neither Section 17A's cart nor
Section 17B's checkout, so its figure excluded both and the Section 17
specifications read as 15 phantom operations - the same class of hole Section
14I and Section 16D each had to close before it. Both customer commerce
surfaces are now registered there and the accepted figure moves from **712 to
727**, derived from production route registration rather than targeted. Section
17B additionally pins its own surface in `internal/section17bcontract`, the
established per-section three-way gate.

Closing it required adding the twelve Section 17A cart rows to
`docs/API_ROUTE_REGISTER.md`. Those routes were already live and already
specified; only the register entry was missing.

### One real defect and two behaviours the tests corrected

**A suspended vendor could sell.** Section 17B's required checkout sequence
puts "validate Vendor active" and "validate branch active" ahead of
catalogue, price and stock. Food's resolver answers both as part of one
sellability decision, so the Food path was correct - but Shop's
`requireShopBranch` reads the branch status into a variable and never
inspects it, admits a branch capability in `DRAFT` or `PAUSED`, and never
reads the `vendor_businesses` row at all. Section 17B had inherited that path
and assumed it covered steps 2 and 3. It did not: a vendor set to `SUSPENDED`
with its catalogue rows untouched validated a checkout and held real stock.

Four tests were written first and all four failed against the original
adapter. The Shop adapter now asserts business `ACTIVE`, branch `ACTIVE` and
an `ACTIVE` Shop branch capability itself, before anything commercial is
read, and reports the same `BUSINESS_INACTIVE` / `BRANCH_INACTIVE` /
`CAPABILITY_INACTIVE` vocabulary Food already used - one reason code per
fact, whichever domain the line came from. Section 12's own behaviour is
deliberately unchanged: admitting a draft capability is right for a vendor
still building a catalogue, and wrong only for selling from one.

`markValidated` set the coordination state to `COMPLETE` unconditionally, so a
Food session that never coordinated anything claimed it had finished
coordination it never started. It now only advances a state that was actually
`IN_PROGRESS`.

The OpenAPI document described `Idempotency-Key` as optional. The shared
customer-command middleware requires it on every command on this base, and the
HTTP test found the mismatch; the specification now says required, and a test
asserts the 400.

### Known limitations, stated rather than hidden

- **No scale fixtures or query-plan evidence.** The indexes in migration
  `000076` were designed for the documented access patterns - the one-live-
  session-per-cart guard, the TTL sweep, the stranded-coordination sweep and
  the recovery lookup - and each is a partial index over exactly the predicate
  its query uses. No thousands-scale corpus has been built and no EXPLAIN
  output captured.
- **Food capacity is not consulted, because nothing links a Food item to a
  capacity pool.** Section 10/11 capacity reservation requires an explicit
  pool, window and allocation, none of which is derivable from a Food
  catalogue item for a customer order. Food's `INVENTORY_LINKED` seam is the
  production/availability authority that does exist, and it is enforced.
- **A refused validation still persists a session row.** That is deliberate -
  the per-line reasons are evidence a client can re-read - but nothing prunes
  them, so a hot retry loop accumulates rows. Retention is unaddressed.
- **`EXACT_CONVERSION` Shop stock bases cannot be checked out.** See the
  fail-closed rule above. Supporting them needs a conversion authority that
  does not exist anywhere in this platform today.

### Verification

`go build ./...`, `go vet ./...` and `gofmt` clean. Migration `000076` was
applied, rolled back and reapplied against real PostgreSQL, and its down file
was verified to actually drop what the up file created.

The canonical isolated regression - `go run ./tools/vendor-test-runner`, one
freshly migrated database per test package - was run on the final tree:
**46/46 packages pass, 76 migrations in the template.** That figure includes
the two service-wide gates (`internal/crmcontract` at 727 routes,
`internal/section17bcontract`), the Section 17B `cmd` integration suite, and
every pre-existing section's own suite. It therefore also closes the gap the
Section 17A entry below records: 17A deferred the full runner because 17B was
concurrently in flight, and this run covers both.

`go test -p 1 ./...` remains unsupported for this service. The canonical full
integration regression is `go run ./tools/vendor-test-runner`; see
docs/README.md.


<!-- SECTION17A_CUSTOMER_CART -->
## Section 17A - Customer Cart Foundation

**Status: IMPLEMENTED; GO.**

Certified:
- Customer Service bearer identity ownership
- cross-customer IDOR protection
- Vendor staff/customer principal separation
- Food/Shop cart separation
- Food single-business/single-branch restriction
- Shop multi-business/multi-branch grouping
- catalogue/price/version snapshot evidence
- variable-measure cart metadata
- immutable Customer Service address snapshot
- optional third-party receiver
- customer-scoped idempotency and replay
- optimistic concurrency and stale-version conflict
- transactional cart mutation history
- transactional commerce.cart.* vendor_outbox publication
- transaction rollback when event publication fails
- migration 000075 through dedicated certification database
- exactly 12 customer-commerce operations
- OpenAPI/router parity
- Customer Service, Food, Shop and Inventory targeted regressions

The full Vendor cmd test package and full Vendor regression runner were
intentionally excluded from this closure because Section 17B was concurrently
under active implementation. Section 17A was certified independently so
unfinished Section 17B test files cannot create false Section 17A failures.

## Section 18 - Vendor Order Lifecycle

Implemented on top of Section 17: food and Shop VendorOrder state transitions,
optimistic concurrency, rejection isolation, immutable-line picking outcomes,
customer-scoped substitution proposals and decisions, cheaper-substitution
adjustment evidence, Wallet refund-state projection, and monotonic
Dispatch/Hauling event synchronization. Vendor Own Delivery remains separate
from platform logistics. Corrective migrations
`000085_vendor_order_lifecycle_remediation`,
`000086_inventory_partial_finalization`, and
`000087_vendor_order_event_actor_types` add payment-to-vendor evidence, refund
amount/OCC state, authenticated logistics ingress, authoritative partial stock
finalisation, and the explicit lifecycle event actors emitted by payment and
vendor operations; the Section 18 OpenAPI/register entries describe the live
surface. The canonical isolated Vendor regression is green at 54/54 packages,
including the five formerly blocked Section 17A PostgreSQL tests.

## Section 19 — Shop picking, substitution and variable measurement

**Status: GO.** Migration `000088_shop_picking_substitution_measurement`
provides the picking, substitution, quality, variable-measurement and Wallet
evidence boundaries. Migration `000089_shop_substitution_authority_unification`
keeps the Section 18 proposal authority canonical, and `000090_inventory_atomic_replacement`
adds Inventory-owned atomic replacement evidence. CAP_AND_REFUND variable
finalization, VendorOrder settlement reconciliation, final receipt assertions,
parent-cap race behavior, and the required PostgreSQL behavioral matrix are
covered by the canonical isolated regression.

Final closure gates: migration replay through `000090`, route/OpenAPI/register
parity at 777 routes, canonical Vendor regression 54/54, Customer Service and
Payment Wallet tests, `go vet`, `go build`, gofmt, and `git diff --check`.

## Section 20 - Kitchen, bakery and production operations

Implemented in the existing `production`, `orders`, `productionbatch`,
`pastry`, `cloud_kitchen` and `inventory` authorities. Migration
`000091_vendor_production_operations` plus `000092_vendor_production_timing_projection`
add promised-ready ordering, exactly-once EXTERNAL_ORDER linkage, station steps,
delay evidence, immutable READY packaging evidence, fulfilment/logistics timing,
honest customer ETA reads, and durable ETA notification handoff. Production
queue and step/delay routes are documented in the capability OpenAPI and route
register; the fully enabled Vendor route surface is now 803.
## Section 21 — commerce foundation and advanced RFQ/recurring feeding

Scheduled/bulk evidence, group ordering and core RFQ are implemented behind
`COMMERCE_BULK_RFQ_ENABLED`. Orders remains the commercial conversion authority;
Production owns capacity reservations; CheckoutQuote/Checkout and Wallet own
checkout and money. RFQ revisions now freeze vendor/payment/deposit/
cancellation terms; conversion records Wallet-authoritative obligations and
recovery references. Corporate recurring schedules generate timezone-aware,
idempotent future occurrences/proposals with optional Production reservations.
Group Orders remain organizer-paid only; split participant payment and corporate
credit/invoicing are out of scope.

## Promotions and delivery funding

**Status: IMPLEMENTATION COMPLETE; canonical database certification blocked by missing local test infrastructure.** Vendor migration `000097`
adds campaign, scope, bundle, quantity-tier, coupon, redemption, immutable quote/
order evidence, HYBRID funding authority fields and Wallet-operation projection.
Seven Vendor routes are capability-documented. CheckoutQuote remains the sole
commercial calculator and usage reservation is serialized with quote creation;
order conversion consumes reservations in the same transaction. Expired
abandoned reservations are released by the existing jobs runtime.

Wallet migration `000009` and two HMAC-v2 internal routes implement exact,
idempotent customer/Vendor/Platform delivery funding with platform approval
evidence, authoritative read-back and provider-payable ledger effects. Dispatch
and Hauling are unchanged. The fully enabled Vendor route surface is 810.
All targeted affected tests, route/OpenAPI/register parity, Wallet tests, vet,
build, gofmt and diff checks pass. The canonical 58-package isolated runner
stopped before template creation because `VENDOR_SERVICE_TEST_DATABASE_URL` is
unset and the local Docker engine is unavailable; migration apply/rollback/
reapply is therefore infrastructure-blocked, not reported as a code pass.

## Payment and Wallet extension

**Implementation complete; PostgreSQL certification environment-dependent.**
Wallet migration `000010` adds the one-payment immutable commerce allocation
authority and Vendor payable/withdrawal ownership. Vendor migration `000098`
adds the high-risk withdrawal permission to Owner and Finance roles. The
existing order payment projection now records one Wallet intent across its
isolated VendorOrder legs, validates quote/final/max/allocation fingerprints,
holds the frozen maximum, releases only qualifying beneficiaries, and routes
allocation-specific cancellation and variable-measure refunds through Wallet.

The new `finance_projection` package is a thin Vendor bearer/IAM/step-up façade;
it stores no balance or ledger. Wallet mutations write ledger, immutable
financial operation, append-only audit, and outbox evidence in one transaction.
Capability OpenAPI files and the route/frontend/platform registers are updated.
The fully enabled Vendor route surface is 816 (810 before this extension plus
six Vendor Finance routes).

## Commerce settlement and refund model

**Implementation complete; PostgreSQL certification environment-dependent.**
Vendor migration `000099_commerce_refund_policy_decisions` freezes the policy
version and immutable automatic-refund decision evidence. Wallet migration
`000011_commerce_refund_recovery_accounting` extends the existing Commerce
allocation authority with allocation-local recovery sources and component
reductions; it does not add another ledger or refund engine.

Vendor completion releases merchandise and eligible Vendor-owned delivery.
Dispatch/Hauling completion owns platform-provider release. Exact item,
replacement, variable-measure, fulfilment-replan and classified delivery-
failure outcomes use the existing Wallet allocation operation and read-back.
AUTO_REFUND releases only the affected Inventory item hold. Ambiguous or
post-value-consumption cases use the existing Support organization-case
authority without moving money.

The existing payment recovery job reconstructs missing decisions from frozen
operational evidence, retries stock/support/Wallet work under stable keys, and
resumes Vendor/variable projections after crashes. No public route was added;
the fully enabled Vendor route count remains **816**. Full traceability is in
`SECTION24_TRACEABILITY.md`.

## Fulfilment group, parcel and mission model

**Implementation complete; PostgreSQL certification environment-dependent.**

## Section 30 - Truck / Hauling Commerce integration

**Implementation complete; PostgreSQL certification environment-dependent.**
Vendor Service now uses the existing Hauling authority for VAN/TRUCK commerce
execution with committed fulfilment-plan evidence. Hauling quote requests carry
multi-pickup stops where the fulfilment plan has them; mission creation carries
operation refs, plan binding, Parcel and manifest evidence, handling metadata
and scheduled pickup-window metadata.

Hauling migration `014_commerce_mission_execution_controls` adds operation
fingerprinting, source-versioned read-back and pre-custody cancellation support
to the existing `commerce_missions` lifecycle. Same operation ref with a
different semantic payload fails closed; retries with the same payload return
the existing mission. The Section 30 commerce kill switch is
`HAULING_COMMERCE_INTEGRATION_KILL_SWITCH`; exposure defaults through
`HAULING_COMMERCE_INTEGRATION_ENABLED=true` and still requires the
`vendor-service` entry in `HAULING_SERVICE_SECRETS`.

No Vendor public route was added. Hauling internal commerce routes are
documented in
`services/driver-hauling-service/docs/openapi/commerce-hauling-missions.openapi.json`.
No Taxi goods delivery, Section 32 wholesale, provider app, frontend, duplicate
Wallet, duplicate OTP/QR authority or duplicate Hauling lifecycle was created.

Vendor migration `000100_commerce_fulfilment_group_parcel_mission` adds the
post-order operational fulfilment aggregate in eleven tables: the
FulfilmentGroup rooted on one CommerceOrder, the FulfilmentUnits inside it, the
Parcels that physically move with their manifests, the append-only
FulfilmentPlan versions with their mission slots and persisted stop sequences,
the projections of platform logistics missions, the VendorMissions this service
owns, the parcel custody chain, and return journeys.

The audit that preceded it found the two things that shaped the result. First,
`internal/features/fulfilment` is a **pre-order** package whose plan is bound to
a checkout session; Section 25 is post-order, so the aggregate is a sibling
package (`internal/features/fulfilmentgroup`) rather than an extension, and the
existing package is untouched. Second, migration `000096`'s
`commerce_delivery_execution` and `commerce_delivery_custody_events` had **no Go
code at all** — vendor-owned execution state and custody events were schema
stubs, not capabilities.

Four identities now survive operational change: one group per commerce order,
one unit per vendor order, a parcel that is still the same parcel after replan,
reassignment, mission replacement, transfer and return, and an append-only plan
history where v1 stays readable after v3 is in force. Concurrency is carried by
database constraints rather than handler checks — `UNIQUE (group_id, version)`
means two concurrent replans against v3 produce one v4 and one stale answer.

VendorMission is canonical here and only for vendor-owned delivery; its status
vocabulary **reuses** `delivery.ExecutionState` verbatim, pinned by a test that
compares both the values and the transition table. PlatformMissionRef is a
projection: `external_ref` is opaque, the owning authority is a column, and a
status report is applied only when strictly newer than the one already held.

payment-wallet-service remains the financial authority. A plan version stores a
planned-cost snapshot as evidence; nothing in the feature creates a hold,
settlement or refund, and Section 24's refund policy is unchanged.

13 routes added (fully enabled Vendor surface **816 → 829**). No other service
was modified. Taxi commerce mission execution is **DEFERRED / NOT CURRENTLY
ACTIVE** — modelled in the vocabulary, refused at plan validation, never
fabricated. Full traceability in `SECTION25_TRACEABILITY.md`.

---

## Delivery Requirement and Vehicle Selection Engine — IMPLEMENTED

A deterministic, explainable Release 1 engine. No AI, no ML, no external
optimizer, no scoring model: every answer is a walk down an ordered ladder of
configured thresholds, and every rung records why it was or was not taken.

Owner: `internal/features/fulfilment` (the engine) with durable evidence in
`internal/features/fulfilmentgroup` (the plan aggregate). No second fulfilment
planner was created — the engine produces a decision, and the existing plan
aggregate remains the only thing that versions and activates plans.

The engine answers two questions and keeps them apart. The MINIMUM PHYSICAL
CLASS is a fact about the load, and a car can be the answer. The SELECTED
EXECUTABLE SERVICE is what the platform can actually perform. Collapsing them
would lose the difference between "your order needs a bigger vehicle" and "we
cannot currently run the vehicle your order needs" — different answers to a
customer and different problems to an operator.

Two live defects were found and fixed. `VEHICLE_OVERFLOW` and `SLA_OVERFLOW`
were **unreachable**: the delivery policy evaluator had always accepted a
`RequiredVehicleClass` and an SLA pair, and the planner had never populated
either, so a vendor whose fleet was motorcycles only could be handed a
van-sized load. The cause was ordering — the vendor policy was evaluated before
anything decided what the load needed — and requirement evaluation now runs
first.

Structural safety properties, each pinned by a named test: one load yields one
class and no count, so multi-motorcycle fragmentation is unrepresentable;
neither the capability policy nor the serviceability snapshot has any field for
provider supply, so scarcity can never select a smaller vehicle; a temperature
conflict is refused before any candidate is evaluated, so a bigger van cannot
"resolve" hot-plus-frozen; and vendor hints can only raise a class, never lower
it.

Taxi goods execution is **DEFERRED**. `driver-taxi-service` is a two-file
scaffold with no goods concept, so the car class is modelled and gated three
ways — capability policy, aggregate validation, and a database `CHECK`. The
supporting reason `CAR_PHYSICALLY_SUITABLE_BUT_GOODS_MODE_DISABLED` is recorded
when the engine falls through, so a van is never mistaken for a heavy order.
`driver-taxi-service` was not modified.

Multi-pickup runtime remains **NOT_PRESENT** in dispatch and hauling. The
capability is modelled rather than hardcoded false, so a future runtime is a
configuration change; today a multi-pickup requirement fails closed.

**0 routes added** (Vendor surface unchanged at 829). Evidence rides on the
existing `GET /fulfilment/groups/{groupId}/plans` and `GET .../parcels`
responses rather than growing the surface, and platform vehicle thresholds have
no vendor-editable endpoint by design. One migration,
`000101_delivery_requirement_vehicle_selection`. No other service was modified.
Full traceability in `SECTION26_TRACEABILITY.md`.


## Multi-Vendor Consolidation and Mission Planning

Implemented. Certification against PostgreSQL is **BLOCKED BY TEST
INFRASTRUCTURE** - see the closing note.

### What changed in planning

Planning used to produce one grouping and execute it. It now builds every
alternative it can, prices each one through the real logistics authorities,
compares them under the customer's chosen preference, executes the winner and
keeps the losers with the reason they lost.

The executed plan has the same structure it always had. The alternatives
decide WHICH grouping runs, not what a running grouping looks like.

- **FASTEST / BALANCED / SAVER**, carried on the plan and flowing unchanged
  into `commerce_quotes.selected_fulfilment_mode`. A plan quoted as SAVER
  cannot later be executed as FASTEST against a charge the customer never
  approved, because the mode is part of the quote fingerprint.
- **Both alternatives are quoted before either is compared.** Three quote
  calls for a two-vendor checkout, not one. A detour measured against an
  estimate is not evidence, and the comparison between consolidated and split
  is mostly a comparison between two delivery fees.
- **Mandatory splits are structural.** Capacity, temperature, fragility,
  vendor-declared separation, urgency, ready windows, a promised arrival time
  and runtime capability each refuse a consolidation in EVERY mode. They are
  recorded as `INELIGIBLE`, and the database refuses to store a constraint
  reason on any other status - so a safety limit is unrepresentable as a price
  decision rather than merely not recorded as one.
- **No AI, no weights, no scores.** Each mode is a lexicographic ordering over
  facts the plan already contains, and ties break on the candidate's own
  semantic fingerprint rather than on slice order.

### The multi-stop gate

Neither `driver-dispatch-delivery-service` nor `driver-hauling-service` models
a mission with several separate collection points. Both remain
**NOT_PRESENT**, and neither service was modified.

The planning model handles that honestly rather than working around it:

- A consolidated candidate spanning two pickup LOCATIONS is built, evaluated
  and rejected with `MULTI_STOP_UNSUPPORTED`. No quote is even requested for
  it - asking an authority to price a route it cannot drive spends a real call
  to obtain a number that must be thrown away.
- Several vendors at ONE physical pickup point still consolidate normally. The
  distinction is `pickup_stop_count`, computed by clustering branch
  coordinates within the co-location radius, and getting it wrong in either
  direction is silent: counting members would destroy the co-located
  optimization, and counting one would price undrivable routes.
- The executable split alternative is planned instead. A missing runtime
  capability costs the customer a consolidation, never a delivery.
- The capability is read from configuration
  (`DISPATCH_MULTI_PICKUP_SUPPORTED`, `HAULING_MULTI_PICKUP_SUPPORTED`, both
  defaulting to false). When a runtime gains multi-pickup, the gate opens with
  no code change - proved by a test that flips the flag and watches the same
  candidate become selectable.

### Basket optimization

A new customer capability in `internal/features/basket`: Optimize My Cart,
budget shopping and recurring baskets, sharing one evaluation pipeline.

- **Nothing is fabricated.** Every candidate is a real active listing with an
  authoritative price and authoritative sellable stock. Equivalence comes from
  a manufacturer trade identifier, the catalogue product identity or the
  taxonomy authority - never from two product names sounding alike.
- **Delivery is quoted, not estimated.** Each candidate basket is priced
  through the quote-only fulfilment planner, so the comparison between "three
  cheap shops" and "one slightly dearer shop" is decided by real fees.
- **The cart is never silently mutated.** Optimization returns proposals;
  acceptance is a separate, refusable act that creates a NEW cart version and
  always requires an authoritative requote afterwards.
- **The budget is the DELIVERED total.** A basket whose merchandise fits and
  whose delivered total does not is never reported as fitting.
- **No-solution answers are truthful.** The minimum feasible delivered total,
  the shortfall and the binding constraints, or an explicit
  `no_complete_basket` when a requirement cannot be satisfied at any price -
  which is a different answer from "too expensive" and is kept apart from it.
- **The search is bounded and says so.** `optimality_note` is
  `BEST_VALID_CANDIDATE_UNDER_CONFIGURED_SEARCH`, never "the cheapest basket",
  and `search_truncated` reports when a bound actually bit.

### QUALITY_BALANCED: NOT_AVAILABLE_DUE_TO_DATA

The variant is implemented and gated, and for most requests it correctly does
not appear.

The platform's catalogue carries `QUALITY_GRADE` as a fresh-goods SELECTION
vocabulary - which grades a listing offers - and no ordinal quality score for
anything. A customer's HARD minimum-grade constraint is therefore checkable and
is enforced; RANKING one vendor's listing above another's by quality is not,
and no score was invented to make the variant appear. The response says which
requirement lacked the signal.

### Exactly-once recurrence

One logical generation per scheduled occurrence, resting on a deterministic
occurrence key plus `UNIQUE (template_id, occurrence_key)`.

The key is derived only from the schedule - the local wall-clock moment and its
IANA zone - so a retry, a worker replay, a process restart, a duplicate
scheduler delivery and two concurrent workers all present the same key and
collide in the database. Nothing rests on an in-memory lock, because every one
of those failure modes involves more than one process.

The occurrence is CLAIMED before any shopping happens, so a duplicate loses the
race cheaply rather than after both workers have called logistics authorities.

**Missed occurrences are SKIPPED, not caught up.** A worker down for a day must
not wake and generate a backlog of yesterday's shopping, because a recurring
basket that catches up is one that delivers twice.

### Money and price

Wallet, orders and payments were **NOT_TOUCHED**. Nothing in this capability is
wired to any of them, which is what makes "evaluating a hundred baskets creates
zero payment intents" a property of the composition rather than a promise.

Post-payment price behaviour is unchanged and continues to be Section 24's:
internal replanning cannot raise an approved charge, a cheaper replan uses the
existing refund policy, and a customer-requested change uses the existing
reapproval path. No second financial policy was written.

### Surface and schema

**11 routes added** (Vendor surface 829 -> 840), all customer-facing under the
commerce base, ten of them read-or-propose. The delivery preference travels in
the EXISTING plan request's body rather than as a new route.

Two migrations: `000102_commerce_fulfilment_plan_candidates` and
`000103_commerce_basket_optimization`.

### Certification status

Every non-database gate passes: build, vet, gofmt, the full Vendor test suite,
route/OpenAPI/register parity, and the migration shape tests.

`VENDOR_SERVICE_TEST_DATABASE_URL` is unset and Docker is unavailable in this
environment, so the PostgreSQL-backed gates could not run. They are written and
skip cleanly rather than being absent:

- migration apply/rollback/reapply
- concurrent occurrence claim (16 workers, one generation)
- crash-and-retry occurrence resumption
- concurrent proposal acceptance (one cart version)
- proposal and template ownership isolation
- template optimistic-concurrency conflict
- database refusal of incoherent rows

They live in `internal/features/basket/store_integration_test.go` and run under
`VENDOR_SERVICE_TEST_DATABASE_URL`. A skip is reported as a skip and never as a
pass: "exactly once" asserted by a test that did not run is not an assertion.

Full traceability in `SECTION27_TRACEABILITY.md`.

## Dispatch delivery integration

Implementation complete; canonical PostgreSQL certification is environment
dependent. Dispatch migration `000042_commerce_multistop_delivery` adds the
canonical route-aware multi-pickup mission runtime while Vendor remains an
immutable-plan client and `PlatformMissionRef` projection. The accepted Section
27 planner can now truthfully enable `DISPATCH_MULTI_PICKUP_SUPPORTED` after
deployment instead of failing every multi-location consolidation closed.

Dispatch uses the normal public marketplace, persists ordered logical stops,
independent manifests and parcel custody, queues no-provider missions with
bounded retry/SLA recovery, supports transfer-safe replacement, receiver
unavailability, reschedule and return, and delivers versioned mission/stop/
parcel events through its transactional outbox. Vendor adds one HMAC-v2 event
callback (840 -> 841 routes) and no migration. Dispatch changes from 109 to 122
routes. Full traceability is in root `docs/SECTION28_TRACEABILITY.md`.

## Vendor-owned and Hybrid delivery runtime

Implementation complete, with canonical PostgreSQL certification dependent on
`VENDOR_SERVICE_TEST_DATABASE_URL` and `PAYMENT_WALLET_TEST_DATABASE_URL`.
Vendor migration `000104_vendor_owned_delivery_runtime` adds restricted Rider
availability, assignment/runtime operation history, durable fallback linkage
and effective VendorOrder execution projection. Wallet migration `000012`
adds the sole-ledger funding transition and effective payable projection.

Vendor routes change from 841 to 849. Vendor Rider remains Vendor staff, not a
marketplace provider. VendorMission remains canonical in Vendor; platform
missions remain canonical in Dispatch. Pre-custody fallback preserves all
commercial and Parcel identities, appends a plan version, transitions funding
through Wallet and reuses Dispatch Section 28. Clean post-custody conversion is
hard-blocked.

The Section 31 secure Vendor handover dependency was closed by Section 32:
Dispatch now owns a distinct VendorMission-compatible custody authority.
Custody-moving Vendor actions still return 503 whenever either safe-default
flag or the authority dependency is absent. Full traceability is in
`SECTION32_TRACEABILITY.md`.

## Operational evidence, exceptions, returns and provider transfer

Sections 33–35 are implemented as one extension to the accepted Dispatch
Commerce mission. Dispatch migration `000043` adds versioned/frozen stop and
Parcel manifest bindings, append-only evidence, exception/return operations,
one-open-transfer enforcement and immutable carrier history. Vendor migration
`000105` adds only the authenticated operational-event projection.

Dispatch provider-to-provider post-custody transfer is available through the
existing shared handover authority. Direct provider reassignment after custody
is blocked; old custody remains active until the replacement verifies the
exact manifest/Parcel/package/location challenge. Normal old-trip proof and
completion are frozen during replacement, preserving Wallet's single
completion settlement path without creating a new earning split engine.

Vendor routes remain 849. Dispatch adds seven routes to the existing Commerce
contract. Hauling and Wallet are unchanged. Vendor Rider secure handover stays
partial/dependency-gated and Vendor Rider → platform post-custody transfer stays
not present. PostgreSQL migration replay/race tests are env-gated on
`DISPATCH_TEST_DATABASE_URL`; a missing variable is reported as test
infrastructure blocked. Root traceability: `docs/SECTION33_35_TRACEABILITY.md`.

## Realtime, semantic notifications and confidence ETA

Section 36 backend implementation is complete. Vendor migration `000106`
adds immutable mission-level ETA versions, a concurrency-fenced current
pointer and scheduled-cutoff notification markers. Notification migration
`002` adds source-event/aggregate/version provenance and Notification-owned
Commerce templates. No public route was added: the existing customer ETA read
endpoint now also returns a `missions` array with an earliest/latest range,
controlled confidence, policy version, source freshness and delay state.

Dispatch remains route, mission, provider-assignment and receiver-credential
authority. Production remains Food/Shop readiness authority. Notification
Service remains the only email/push/realtime transport owner; Vendor publishes
bounded semantic intent through its transactional outbox. SMS transport is not
configured and no business service bypass was added. VendorMission secure
handover remains dependency-gated exactly as documented for Section 31.

Canonical migration replay and race certification requires the documented
PostgreSQL test URLs. Capability details are in
`architecture/22_REALTIME_NOTIFICATIONS_CONFIDENCE_ETA.md`; roadmap evidence is
in `SECTION36_TRACEABILITY.md`.

## Analytics, reporting and reliability

Section 39 is implemented as downstream operational analytics. Analytics
Service owns event ingestion, read-model aggregation, windowed vendor reports,
reliability snapshots, privacy-safe demand/price intelligence and the
future-disabled Order Guarantee policy model. Vendor Service was not made a
reporting authority; it only publishes controlled Commerce analytics events
from its transactional outbox when Analytics configuration is present.

Order Guarantee status is `FUTURE_DISABLED`: no claim route, no customer-facing
eligibility, no compensation, and no Wallet guarantee mutation.

Final verification resolved the reliability policy provenance: no approved
pre-Section 39 KarryGo weight policy was found, and no persisted V1 snapshots
were found, so `vendor_reliability_v1` now uses the neutral equal-weight
technical default. Price benchmark privacy requires five independent external
comparable Vendors after excluding the requester and suppresses low-sample
cohorts.

## Security, privacy and compliance

Section 40 is implemented as a hardening/certification pass, not a new
security domain. Existing Vendor RBAC, shared HMAC v2, Wallet, Media, Support,
Analytics, Notification and handover authorities remain authoritative.
Analytics read routes are HMAC/allowlist/scope protected, and Shop Commerce
now rejects pharmacy/medicine products on product create/update, bulk import
and checkout revalidation. Traceability is in `SECTION40_TRACEABILITY.md`.

## Section 29 — Taxi Goods Delivery extension

Implemented behind `TAXI_GOODS_DELIVERY_ENABLED=false` in both Taxi and Vendor.
CAR now maps to the distinct Taxi Goods quote/mission authority. Vendor restores
the exact service, vehicle class, fee, and authority quote from the customer's
frozen Section 17C mission before execution; it never guesses from order domain
or fee and never calls the passenger booking surface. See
`SECTION29_TRACEABILITY.md` and the Taxi Goods OpenAPI contract.

Final gap closure adds explicit idempotent provider-reassignment and receiver-
change workflows, atomic OTP/opaque-QR verification against binding changes,
and a leased Taxi outbox publisher for HMAC-authenticated Vendor projection.
PostgreSQL/Redis integration, migration replay, and repeated concurrency tests
certify these paths while both runtime flags remain false by default.

## Section 32 — approved handover integration

This audit uses the local implementation, including uncommitted Sections 29/30.
Section 29 is closed. Section 32 does not reopen passenger Taxi and Vendor
Commerce does not implement a challenge authority.

| Authority | Mission type | Current finding | Challenge owner / contract |
|---|---|---|---|
| Dispatch | MOTORCYCLE | AVAILABLE / INTEGRATED | `securehandover`; source-scoped pickup verify plus opaque pickup/delivery read-back, HMAC v2, caller allowlist and nonce |
| Hauling | VAN/TRUCK | AVAILABLE / INTEGRATED | booking `SecureHandoverRepository`; Commerce pickup confirm plus opaque pickup/delivery read-back, HMAC v2, caller allowlist and nonce |
| Taxi Goods | CAR | AVAILABLE / INTEGRATED | `goods`; pickup verify plus opaque pickup/delivery read-back, Vendor-only HMAC and durable nonce |
| Vendor-owned | VendorMission | AVAILABLE / INTEGRATED | Dispatch `vendorownedhandover`; distinct mirrored custody binding, OTP/QR authority, HMAC v2 and canonical read-back |

All four authorities are wired through one Vendor relay/projection boundary
and independently disabled-by-default capability flags. Dispatch owns the new
VendorMission challenge/custody state in migration `000055`; Vendor keeps no
credential or credential hash. `PENDING_EXTERNAL_DEPENDENCY = NONE`.

The adapter contract covers canonical read-back after unknown outcomes, wrong
authority/mission rejection, concurrent duplicate confirmation, stale
projection protection and credential-free serialization. Canonical Dispatch,
Hauling and Taxi suites retain OTP/QR mutual consumption, resend, binding,
expiry, attempt and concurrency ownership.

## Sections 33–35 — final authority-local closure

This entry supersedes the earlier Dispatch-only operational-evidence status.
Dispatch, Hauling and Taxi now each persist the shared authority-local custody
contract in their own database. Dispatch also owns VendorMission custody
through `vendorownedhandover`; its former return-unavailable adapter gap is
closed.

The contract covers immutable/versioned exact manifests, typed Media-backed
evidence, structured exceptions and recovery, category-specific explicit
returns, authenticated incidents, completion freeze, capability-safe
replacement, atomic two-provider transfer, receiver challenge regeneration,
cross-authority provenance, bounded policy-backed Wallet allocations, durable
outbox events, HMAC v2 and replay prevention. Vendor adds no custody route,
credential store, physical mission authority, or migration.

Traceability: `docs/SECTION33_35_TRACEABILITY.md` and repository root
`docs/SECTION33_35_TRACEABILITY.md`.

**SECTION 37 — INTENTIONALLY SKIPPED / OUT OF CURRENT SCOPE.** At the time of
the Sections 33-35 closure no Section 41+ work had started; no frontend code
changed.

## Sections 40-42 final security, recovery and schema closure

Sections 40, 41 and 42 are implementation-complete. Additive migration
`000116_sections40_42_security_recovery` adds complete idempotency metadata and
policy-driven operational receiver/location minimization with durable legal
holds, restricted evidence, bounded legacy discovery and a dead-letter-capable
worker. Semantic JSON fingerprints, atomic expired-key reclaim,
transaction-bound fingerprinted inbox claims and credential-safe durable worker
errors close the code gaps found by the repository audit.

No route or frontend was added and no service authority moved. See
`SECTION40_42_TRACEABILITY.md` for the closure and domain-model matrices.

## Section 43 — API contract families

**COMPLETE / RELEASE-CERTIFIED.** The fully enabled production router remains
857 routes before and after this work. Across 48 Vendor OpenAPI files, 867
declarations map to 857 unique production operations; router-to-OpenAPI and
OpenAPI-to-router mismatch counts are both zero. Ten mature cross-section
declaration overlaps are explicitly allowlisted and guarded.

All operations now have globally unique operation IDs, responses, valid local
references, valid security-scheme references, and required declarations for
every path template. Missing idempotency and optimistic-version headers were
aligned with the existing Vendor order, substitution and Section 16 command
behavior. Section 16D cost writes now use their already-injected shared
idempotency store. No route, migration, frontend, or authority was added or
moved. The route/register artifact and full matrix are in
`SECTION43_ROUTE_PARITY.json` and `SECTION43_TRACEABILITY.md`.

Order Guarantee is `FUTURE_DISABLED`. Pharmacy/medicine commerce remains out of
scope and unimplemented. Wallet, Media, Customer preference, Verification,
Dispatch, Hauling, Taxi Goods, VendorMission, and handover/QR authorities remain
preserved.

Certification completed on 2026-09-09:

- `go test ./... -count=1` — PASS.
- isolated PostgreSQL/Redis regression — all 66 packages PASS in aggregate. The
  initial full run passed 65/66 and exposed a certification-only Section 13
  measurement that counted asynchronous database-wide transaction statistics;
  the measurement was scoped to the search pool's traced SQL statements and its
  isolated rerun passed against the same 48,000-row corpus. Production behavior
  was not changed by this correction.
- `go vet ./...` and `go build ./...` — PASS.
- shared `serviceauth` HMAC test suite — PASS.
- Section 43 OpenAPI validation — PASS: 48 documents, 867 operations, 867
  globally unique operation IDs, zero unresolved local references, zero missing
  responses, zero missing required path parameters, and zero invalid security
  scheme references.
- formatter, deterministic parity artifact and Git whitespace checks — PASS.

## Sections 44–45 — event catalogue and durable workers

**COMPLETE / RELEASE-CERTIFIED.** The repository-native envelope,
`vendor_outbox`, transactional inbox,
durable coordinator and domain-row worker patterns remain canonical. There are
39 authority-reviewed event contracts and 27 worker-requirement mappings.

Three real publication gaps were closed without schema changes:
`commerce.quote.locked` now reaches the transactional outbox once per immutable
approval, `commerce.order.created` reaches it once per ParentOrder, and
`commerce.checkout.paid` is emitted only after authoritative Wallet confirmation
changes the payment operation to `HELD`. An `UNKNOWN` Wallet outcome remains
non-paid. Inbox fingerprint matching now rejects a late
fingerprint if the original event identity was claimed without one. Persisted
durable-worker failures now reuse the Section 40 event credential redactor,
including OTP, QR, access/payment/bank credentials, PAN/CVV and service secrets.
The outbox exposes bounded pending/dead-letter/oldest-pending metrics without
event or tenant identifiers.
The durable coordinator now exposes a low-cardinality health/metrics snapshot
covering retained backlog, persistent retry, dead letters, stale leases and
dependency availability; disabled workers remain visible and do not consume or
erase pending work.

No route, migration, frontend, external service, custody code or Section 43
contract was changed. Order Guarantee remains future-disabled. Full matrices
and machine parity are in `SECTION44_45_TRACEABILITY.md`,
`SECTION44_EVENT_PARITY.json` and `SECTION45_WORKER_CATALOGUE.json`.

Certification completed on 2026-09-10:

- Vendor isolated runner: the full pass completed 65/66; the only failure was
  the Section 13 scale fixture exceeding its 10-minute seed context. That
  package passed on its fresh isolated rerun, and the final changed `events`
  and `cmd` packages also passed independently. Effective package result:
  66/66, with the initial timeout retained here rather than hidden.
- `go test ./... -count=1`, `go vet ./...`, and `go build ./...` — PASS.
- Section 44/45 deterministic catalogue validation — PASS: 39 events, 27
  worker requirements, two future-disabled events, zero parity gaps.
- Section 43 OpenAPI regression — PASS: 867 operations, zero quality changes.
- changed-Go-file formatting and `git diff --check` — PASS.
- Migration replay — NOT REQUIRED; no migration was added or changed.
# Final production-readiness closure (2026-09-10)

Repository implementation remains complete. Local `api`, `worker`, and `all` process probes pass after closing a worker-only nil-server panic and enforcing PostgreSQL connectivity at worker startup. Production GO remains blocked because deployed ingress, encrypted backups/restore drill, monitoring/alerts, staging credentials/allowlists, live multi-service black-box flows, failure injection, and bounded staging performance/SLO measurements are unavailable. The authoritative register is `docs/FINAL_PRODUCTION_GO_CERTIFICATION.md`.

## Final DevOps execution (2026-09-11)

The actual backend deployment convention was confirmed from repository and GitHub metadata: immutable GHCR images are deployed over SSH to Docker Compose under `/opt/karrygo` on a VPS. Vendor now has a repository-native deployment bundle following that convention: a Vendor-specific production image, a PostgreSQL-advisory-locked migration binary, distinct API and Worker Compose units, and a workflow enforcing migration -> API readiness -> Worker rollout. The API publishes only on VPS loopback and the Worker publishes no port. Compose validation, image build, and the packaged migration-tip verification (116 records) pass locally.

This does not promote Sections 47/48 or production GO. The VPS shell, existing reverse proxy/TLS/DNS, Vendor environment file, PostgreSQL backup provider, monitoring control plane, and downstream staging credentials/allowlists are unavailable from this task. The exact remaining operator commands and pass conditions are in `deploy/vendor-service/README.md`; the authoritative production decision remains `PRODUCTION GO: BLOCKED`.

## Customer V4 §22 purchase Activity row (2026-09-28, local)

Vendor, as ParentOrder authority, publishes one `VENDOR_COMMERCE`/`COMMERCE_ORDER` movement per ParentOrder (`internal/features/orders/customer_movement.go`). It is appended to the existing outbox in the creation and parent-state-change transactions as `commerce.customer_order.movement`, delivered by `vendor-customer-order-movement` to Customer's movement ingress, and repaired by `customerOrderMovementReconciler` (`cmd/customer_order_movement.go`). `source_version` is the ParentOrder version. Order state only: no amounts, items, provider or credentials. Gated by `VENDOR_CUSTOMER_MOVEMENT_GROUPING_ENABLED`; no Vendor migration. Evidence: `TestCustomerOrderMovementCancelledBeforeDispatchAndReconciled` and the lifecycle E2E with emission enabled. Full record: `docs/customer-sections9-11-report.md` §§22–26 (Customer V4 numbering, not Vendor section numbering).

## Customer V4 Sections 15–18 integration (2026-09-28, local)

**Section 15: COMPLETE LOCALLY — PASS.** Wallet remains financial authority.
Commerce binds approved immutable quote/order fingerprints and deterministic
Wallet operation IDs, reads back UNKNOWN payment/refund outcomes, and preserves
VendorOrder plus platform-leg isolation. Real PostgreSQL tests cover duplicate
and concurrent writes, partial/full and multi-vendor refunds, recovery and
Customer Wallet regressions. External Reference/Repricing remains gated and
unimplemented pending an approved adapter.

**Section 17: COMPLETE LOCALLY — PASS.** Support remains case/resolution
authority. Order-scoped case ownership and child-ref integrity, Media verdict
binding, IDOR denial and deterministic refund separation pass targeted tests.

**Section 18: COMPLETE LOCALLY — PASS for repository scope.** Customer bearer
authorization, Support ownership validation, one-use Media grant redemption,
metadata read-back, MIME/size/expiry/rate limits, OpenAPI and mobile credential
removal pass targeted tests. Production scanner, HMAC configuration and ingress
remain staging items.

**Section 16: PARTIAL locally.** Ride, Send, Truck, Commerce, Share and Wallet
route-intent hints are wired; Taxi Ride, Customer Share and Wallet state
transitions have durable retry. The receiver-channel product decision is now
fixed: a phone-only snapshot without a verified account or other supported
secure channel must fail closed; no SMS or voice integration is authorized.
Dispatch secure handover returns `RECEIVER_SECURE_CHANNEL_REQUIRED` for that
case. Dispatch secure handover, Vendor Own Delivery, Hauling and Taxi Goods
notification producers now send challenge references, not OTP/QR values;
Notification validation rejects custody secrets, and production template
copy no longer interpolates a code. Vendor Own Delivery has a hash-checked
credential reconstruction path. Dispatch, Hauling, Taxi Goods and Vendor Own
Delivery now have account-scoped receiver reads; a third-party receiver account
is explicitly frozen from the Vendor cart into the order and authority mission
instead of being inferred from the sender. Phone-only third-party receiver
bindings fail closed before pickup custody with `RECEIVER_SECURE_CHANNEL_REQUIRED`.
The sender is no longer assumed to be the receiver in Dispatch secure handover.
Hauling, Taxi Goods and Vendor Own PostgreSQL tests confirm that receiver
binding changes invalidate or deny old credential reads. Vendor cart, order,
mission and typed-error propagation have targeted tests; receiver read routes
have bearer-auth and OpenAPI checks. The remaining closure is a transaction-
bound, verified email-only receiver read contract. Notification delivery alone
cannot prove mailbox control, and existing Customer email login creates an
account rather than providing an account-free proof for this transaction.
Until that authority contract and the full affected E2E/race
matrix pass, repository-local §16 is not certified. Staging delivery and
mobile navigation remain separate certification items. No commit or push was
made in this local worktree.
