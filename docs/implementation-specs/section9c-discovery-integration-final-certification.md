KARRYGO — VENDOR SERVICE
SECTION 9C — FOOD DISCOVERY, SERVICE INTEGRATION, HARDENING & FINAL SECTION 9 CERTIFICATION
FULL BACKEND IMPLEMENTATION PROMPT

MODE: DEVELOPMENT-DRIVEN
TARGET: PRODUCTION-GRADE BACKEND IMPLEMENTATION
SCOPE: SECTION 9C ONLY + FINAL SECTION 9 CERTIFICATION
REPOSITORY: C:\KarryGoPush
INTENDED BRANCH: staging

======================================================================
0. EXECUTION DIRECTIVE
======================================================================

You are implementing Section 9C of the KarryGo Vendor Service.

Section 9A and Section 9B are already implemented and accepted.

This task completes Section 9 by building the safe read/consumption contract for the Restaurant/Fast-Food catalogue, hardening the complete Section 9 implementation, and certifying Sections 9A–9C together.

This is NOT:

- a design-only task
- a gap-analysis-only task
- a documentation-only task
- a frontend task
- an Order implementation
- a Customer app implementation
- an Inventory implementation
- a Wallet implementation
- a logistics execution implementation

You must:

1. perform a fast baseline verification;
2. inspect only the accepted Section 9A/9B implementation and relevant shared Vendor patterns;
3. implement a read-optimized service-facing food catalogue/discovery contract;
4. implement authoritative sellability consumption;
5. implement authoritative price-resolution consumption;
6. implement bounded search/filtering needed for Food discovery;
7. implement catalogue freshness/revision semantics;
8. secure internal service-facing routes;
9. ensure no customer/client needs to reproduce pricing or sellability business rules;
10. validate performance/query behavior at realistic catalogue sizes;
11. harden tenant/branch/capability boundaries;
12. harden revision/invalidation behavior;
13. test concurrency/freshness;
14. update OpenAPI/contracts/docs;
15. run final Section 9A+9B+9C certification;
16. fix Section 9 defects uncovered by final certification;
17. return the required final report.

Do NOT stop after planning.

Do NOT summarize this specification back to the user.

After the required fast baseline check, begin implementation immediately.

======================================================================
1. USAGE-SAVING / DEVELOPMENT-DRIVEN MODE
======================================================================

Use implementation time efficiently.

- Do not use subagents.
- Do not perform a broad repository audit.
- Do not browse the web.
- Read this specification once and treat it as the task contract.
- Re-read only specific sections if necessary.
- Trust accepted Sections 5–8.
- Trust accepted Section 9A and 9B except where 9C directly integrates them.
- Do not broadly re-audit CRM.
- Do not refactor unrelated packages.
- Inspect only files directly relevant to Section 9.
- Reuse existing dependencies and architecture.
- Do not introduce unnecessary libraries.
- Use targeted tests during development.
- Do not repeatedly run the full repository suite.
- Run final Section 9 certification once near completion.
- Keep progress updates concise.
- Do not broaden scope.
- Do not start Section 10.
- Stop when Section 9C and final Section 9 acceptance criteria are satisfied.

Reuse existing PostgreSQL/Redis infrastructure.

If broad tests have known shared-database contention, use isolated deterministic databases/groups rather than repeatedly rerunning a known-contentious shared suite.

======================================================================
2. GIT / WORKTREE SAFETY
======================================================================

Work directly in:

C:\KarryGoPush

Intended branch:

staging

Perform only:

git branch --show-current
git status --short
git log -1 --oneline

before implementation.

DO NOT:

- create a branch
- create a worktree
- switch branches
- commit
- stage
- merge
- cherry-pick
- rebase
- push
- pull
- reset
- stash
- clean
- discard unrelated dirty changes
- delete unrelated files

Leave all Section 9C work:

UNCOMMITTED
UNSTAGED

The current dirty working tree is authoritative.

======================================================================
3. ACCEPTED SECTION 9 BASELINE
======================================================================

Treat these accepted reports as baseline.

SECTION 9A — FOOD CATALOGUE FOUNDATION:
GO

SECTION 9B — PRICING, AVAILABILITY, SCHEDULING & OPERATIONAL PROFILES:
GO

Known accepted baseline after 9B:

Vendor routes:
257

Section 9A added:
39 public
0 internal

Section 9B added:
22 public
0 internal

Section 9 routes added so far:
61

Pre-Section-9 Vendor routes:
196

Migration tip:
000032_vendor_section9b_permissions

Section 9 migrations:

000029_vendor_food_catalogue
000030_vendor_section9a_permissions
000031_vendor_food_operations
000032_vendor_section9b_permissions

Known 9B support status:

INVENTORY_AVAILABILITY_SUPPORT = NOT_PRESENT

HOWEVER:

the CURRENT working tree is authoritative.

Quickly derive:

PRE_9C_ROUTE_COUNT
PRE_9C_MIGRATION_TIP

before adding anything.

Do not blindly use 257 / 000032 if newer accepted work exists.

======================================================================
4. SECTION 9C PURPOSE
======================================================================

9A answered:

WHAT DOES THE VENDOR SELL?

9B answered:

CAN THIS ITEM BE SOLD NOW,
AT WHAT EFFECTIVE PRICE,
WITH WHAT PREPARATION AND LOGISTICS METADATA?

9C answers:

HOW CAN THE REST OF KARRYGO SAFELY AND EFFICIENTLY CONSUME THAT TRUTH?

Section 9C creates the authoritative service-facing Food catalogue consumption contract and performs final Section 9 hardening/certification.

The central principles are:

- Vendor remains Food catalogue authority.
- Consumers do not reproduce Vendor business rules.
- Consumers do not need to join 9A/9B tables themselves.
- Consumers receive effective, explainable, revision-aware results.
- Discovery reads must not bypass sellability.
- Pricing reads must not bypass immutable price-version truth.
- Stale catalogue consumers can detect change.
- No external service is allowed database access to Vendor tables.

======================================================================
5. WHAT 9C IS NOT
======================================================================

Section 9C DOES NOT implement:

- Customer app UI
- customer authentication
- shopping cart
- checkout
- orders
- restaurant order acceptance
- payment
- Wallet
- vendor settlement
- inventory stock
- ingredient stock
- stock reservations
- promotions
- coupons
- dynamic pricing
- rider assignment
- Dispatch execution
- Taxi execution
- Hauling execution
- logistics OTP/QR
- delivery fees
- travel ETA
- customer recommendation AI
- machine-learning ranking
- frontend search screens

Those belong to later domains.

Do not let "discovery" turn into "ordering".

======================================================================
6. AUTHORITY BOUNDARY
======================================================================

Vendor Service owns:

- catalogue definitions
- current item/variant/modifier configuration
- effective catalogue pricing
- price history references
- operational availability
- menu schedules
- branch overrides
- prep profile
- logistics profile
- inventory-source configuration
- catalogue revision
- Food sellability truth

Consumers may ask Vendor:

- what menus are discoverable for this branch?
- what sections are discoverable?
- what items are currently sellable?
- what valid variants/modifiers exist?
- what is the current effective catalogue price?
- is this item sellable at time T?
- what preparation metadata applies?
- what logistics handling metadata applies?
- what catalogue revision produced this response?

Consumers must NOT query Vendor tables directly.

======================================================================
7. SERVICE-FACING API MODEL
======================================================================

Section 9C should primarily expose INTERNAL service-facing APIs.

Use the accepted internal Vendor API root:

/internal/vendor/v1/...

Do not expose raw unauthenticated Vendor database projections.

Expected logical route family:

/internal/vendor/v1/food/...

Use repository conventions.

Potential routes include:

GET
/internal/vendor/v1/food/branches/{branchId}/catalogue

GET
/internal/vendor/v1/food/branches/{branchId}/menus

GET
/internal/vendor/v1/food/branches/{branchId}/items/{itemId}

GET
/internal/vendor/v1/food/branches/{branchId}/items/{itemId}/sellability

POST
/internal/vendor/v1/food/branches/{branchId}/pricing/resolve

GET or POST
/internal/vendor/v1/food/branches/{branchId}/search

GET
/internal/vendor/v1/food/branches/{branchId}/revision

Do not force this exact count or exact shapes if established Vendor route conventions suggest a cleaner surface.

Aim for a small coherent contract.

Prefer approximately 4–8 meaningful internal operations rather than dozens of consumer routes.

Route quality > route count.

======================================================================
8. INTERNAL AUTHENTICATION — MANDATORY
======================================================================

Reuse the accepted KarryGo internal HMAC v2 architecture.

Internal Food consumption routes must use the existing internal authentication model, including as applicable:

- caller identity
- HMAC v2
- allowlist
- timestamp
- nonce
- replay protection
- request-body hashing where relevant
- tenant/business/branch scope validation

Do not create a second internal authentication system.

Do not mount internal routes under:

/api/v1/vendor

The root must follow the accepted internal routing rule established after Sections 7/8I.

Test that no 9C internal route accidentally appears under the public Vendor gateway base.

======================================================================
9. CALLER AUTHORIZATION
======================================================================

Internal HMAC authentication alone is not enough.

Use caller allowlisting/scoping.

Only approved KarryGo services should be capable of reading the Food discovery contract.

Do not automatically allow every internal service.

If no production Customer/Discovery consumer exists yet:

implement the Vendor-side contract and document the expected future caller configuration.

Do NOT modify another service merely to fabricate a consumer.

Potential future consumers may include:

- Customer Service
- API Gateway
- future Order authority
- future discovery/search service

But only actual approved caller identities should be allowlisted.

======================================================================
10. DISCOVERABLE CATALOGUE
======================================================================

Implement a read-optimized branch Food catalogue view.

The catalogue response must represent EFFECTIVE consumer-visible catalogue truth rather than raw management records.

A consumer should not need to call 20 management routes to reconstruct one menu.

At minimum the projection should be able to return:

BRANCH
  ↓
ACTIVE / DISCOVERABLE MENUS
  ↓
AVAILABLE MENU SECTIONS
  ↓
SELLABLE FOOD ITEMS
       ├── effective price summary
       ├── variants
       ├── valid modifier groups
       │      └── modifiers
       ├── media refs
       ├── dietary metadata
       ├── allergen metadata
       ├── prep profile
       └── logistics metadata

Do not return administrative-only fields unnecessarily.

======================================================================
11. DISCOVERY MUST USE 9B SELLABILITY
======================================================================

Do NOT invent a simplified second sellability algorithm.

Every discovery-visible item must be evaluated using the accepted Section 9B effective availability rules.

The authoritative chain remains conceptually:

business
→ branch
→ capability
→ menu state
→ menu schedule
→ section
→ item
→ operational availability
→ inventory seam
→ sellable

An item that is not sellable must NOT appear as sellable merely because it exists in 9A.

The API may optionally include unavailable items if an explicit management/internal debugging mode exists, but normal discovery must default to consumer-safe sellable truth.

Do not expose an "available=true" flag calculated independently from the 9B resolver.

======================================================================
12. DISCOVERY PRICE
======================================================================

Discovery price must derive from the accepted Section 9B pricing model.

For an item with no selected variant/modifiers, expose a deterministic base/display price.

Do not pretend the display price represents every possible configured selection.

The response should clearly distinguish concepts such as:

- base/effective starting price
- variant-specific prices
- modifier price deltas

Use integer minor units.

Do not use floats.

Include currency.

Include relevant immutable price-version references where appropriate.

Do not make consumer clients reproduce:

branch override
+
variant inheritance
+
modifier pricing

logic.

======================================================================
13. CONFIGURABLE ITEMS
======================================================================

For items with variants/modifier groups, discovery must return enough validated configuration metadata for a consumer to render a valid selection flow.

At minimum include:

VARIANTS

- ref
- name
- position
- active state represented by inclusion
- effective price or price semantics appropriate to the contract

MODIFIER GROUPS

- ref
- name
- position
- min_select
- max_select

MODIFIERS

- ref
- name
- position
- price_delta_minor
- relevant price-version ref

Do not return archived/invalid relationships.

Do not leak modifier groups from another item/business.

======================================================================
14. SEARCH / FILTERING
======================================================================

Implement bounded Food catalogue search/filtering suitable for service consumption.

Do NOT build a separate search engine in Section 9C.

Do NOT introduce Elasticsearch/OpenSearch unless already authoritative and required by the repository, which is not expected.

Prefer PostgreSQL-backed bounded search.

Minimum useful filters should include where supported by the accepted domain:

- capability
- menu
- section
- text query
- dietary code
- allergen exclusion
- sellable-only
- price range
- preparation-time range

Do not add speculative filters unrelated to Section 9.

--------------------------------------------------
14.1 TEXT SEARCH
--------------------------------------------------

Text search may cover appropriate catalogue fields such as:

- item name
- item description

Potentially menu/section names if product-safe.

Use safe parameterized SQL.

Do not concatenate raw SQL from the query.

Do not allow arbitrary SQL operators/fields.

Bound:

- query length
- filter count
- pagination
- result count

--------------------------------------------------
14.2 ALLERGEN FILTERING
--------------------------------------------------

Do not claim medical safety.

An allergen exclusion means:

exclude catalogue items whose vendor-declared allergen metadata contains the requested code.

It does NOT mean:

certified allergen-free.

Document this distinction.

--------------------------------------------------
14.3 DIETARY FILTERING
--------------------------------------------------

Dietary metadata remains vendor-provided catalogue metadata.

Do not create nutritional/medical inference.

======================================================================
15. PAGINATION
======================================================================

Search/list APIs must be bounded.

Use existing Vendor pagination conventions.

Do not permit unbounded catalogue dumps through list/search APIs.

For full branch catalogue snapshot routes, if an intentionally aggregated bounded response is used:

set explicit maximum sizes and fail cleanly if a pathological catalogue exceeds product-safe limits.

Alternatively use pagination/cursors according to existing architecture.

Do not silently truncate without signalling.

======================================================================
16. CATALOGUE REVISION / FRESHNESS
======================================================================

Section 9B introduced transactional catalogue revisions.

Section 9C must make them useful to consumers.

Every discovery/sellability/pricing response must expose appropriate revision/freshness metadata.

At minimum support:

catalogue_revision

and enough scope information to understand what it applies to.

A consumer must be able to determine:

"Has the catalogue changed since revision N?"

Do not make consumers compare timestamps alone.

======================================================================
17. REVISION ENDPOINT
======================================================================

Provide a lightweight revision/freshness check if useful to avoid refetching a full catalogue.

Conceptually:

GET
/internal/vendor/v1/food/branches/{branchId}/revision

Response:

business_ref
branch_ref
capability
catalogue_revision
updated_at

Use actual project conventions.

Do not expose database internals.

======================================================================
18. CONDITIONAL / CHANGE-AWARE READS
======================================================================

Where cleanly compatible with existing HTTP architecture, support a change-aware read mechanism.

Possible approaches include:

- since_revision
- expected/current revision comparison
- HTTP ETag / If-None-Match
- repository-consistent equivalent

Do not force ETag if the current stack does not use it.

The required outcome is:

a consumer can avoid unnecessary large catalogue refreshes when nothing changed.

If:

consumer revision == current revision

the service may return a compact unchanged result according to the chosen contract.

Document exact semantics.

======================================================================
19. REVISION CONSISTENCY
======================================================================

A discovery response must not claim revision N while returning state from a logically later inconsistent snapshot.

Use a database consistency strategy sufficient to provide coherent reads.

Do not over-engineer distributed snapshot isolation.

But ensure:

catalogue data
+
revision metadata

represent a coherent authoritative read.

If reading multiple tables under PostgreSQL transaction isolation, use the appropriate read transaction/ordering.

Test concurrent update vs catalogue read behavior.

======================================================================
20. NO STALE MATERIALIZED TRUTH WITHOUT SAFETY
======================================================================

Prefer reading authoritative Section 9A/9B tables directly unless profiling demonstrates a need for a separate materialized projection.

If a projection table/read model is introduced:

it MUST:

- be derived from authoritative Vendor data
- be rebuildable
- be revision-bound
- never become a second authority
- fail safely if stale
- have deterministic rebuild/update semantics
- include tests for stale-generation behavior

Do not introduce asynchronous eventual-consistency complexity without evidence that it is needed.

9C does not require a materialized view merely for architectural fashion.

======================================================================
21. CACHING
======================================================================

Do not create external distributed caching unless current architecture already provides an appropriate safe pattern and it is genuinely useful.

If Redis is used for catalogue reads:

- DB remains authority
- cache key must include tenant/branch/capability/revision
- stale revision must never masquerade as current truth
- cache failure must not make unavailable items sellable
- invalidation must use the 9B revision model
- cache is optional optimization, not correctness authority

If DB performance is sufficient:

prefer no new cache complexity.

Document the decision.

======================================================================
22. SELLABILITY INTERNAL CONTRACT
======================================================================

Expose the accepted Section 9B sellability resolver through the service-facing contract.

Conceptually:

GET
/internal/vendor/v1/food/branches/{branchId}/items/{itemId}/sellability?at=...

Response should include:

item_ref
business_ref
branch_ref
capability

sellable

reason_code

evaluated_at

branch_timezone

matched_schedule_ref if applicable

operational state/ref if appropriate

inventory support/status

catalogue_revision

Do not expose unnecessary internal tables.

Do not return customer-specific data.

======================================================================
23. PRICING INTERNAL CONTRACT
======================================================================

Expose authoritative catalogue price resolution for future service consumers.

Conceptually:

POST
/internal/vendor/v1/food/branches/{branchId}/pricing/resolve

Input:

item_ref

optional variant_ref

selected_modifier_refs

optional evaluation context required by actual implementation

Do NOT accept client-supplied prices.

Do NOT accept client-supplied currency as truth.

Load all authoritative values.

Response should include:

item_ref

variant_ref if used

base_source

base_price_minor

modifier components

modifier_total_minor

effective_unit_price_minor

currency

item price-version ref

variant price-version ref where applicable

branch override price-version ref where applicable

modifier price-version refs

catalogue_revision

resolved_at

Do not charge money.

Do not create a payment intent.

Do not create an Order.

======================================================================
24. PRICE RESOLUTION MUST VALIDATE SELLABILITY BOUNDARY
======================================================================

Keep price calculation and sellability semantics clear.

A service-facing pricing operation intended for future ordering should not silently produce an "orderable" price for an invalid item configuration.

At minimum validate:

- tenant/business/branch
- item exists
- variant belongs to item
- modifier groups attached
- modifier min/max
- required groups
- duplicate selections
- active/non-archived state

If the existing Section 9B resolver deliberately separates price validity from sellability, preserve that domain separation but expose both clearly.

Do not mislabel a calculated catalogue price as "orderable" if current sellability is false.

======================================================================
25. CATALOGUE SNAPSHOT RESPONSE
======================================================================

Design the branch catalogue response for efficient consumption.

Avoid excessive repeated parent data.

Use repository-consistent DTOs.

Conceptually:

{
  business_ref,
  branch_ref,
  capability,
  catalogue_revision,
  generated_at,
  menus: [...]
}

Each menu:

{
  ref,
  name,
  sections: [...]
}

Each section:

{
  ref,
  name,
  position,
  items: [...]
}

Each item may include:

{
  ref,
  name,
  description,
  media_refs,
  dietary_codes,
  allergen_codes,
  display/base pricing,
  variants,
  modifier_groups,
  prep_profile,
  logistics_profile
}

Only include sellable/currently discoverable resources by default.

Do not include:

- audit records
- internal DB IDs when opaque refs exist
- actor refs
- internal outbox data
- historical price rows
- inventory internals
- archived objects
- management-only lifecycle noise

======================================================================
26. CAPABILITY SCOPING
======================================================================

Section 9 remains scoped to:

RESTAURANT
FAST_FOOD

Do not silently expose:

CLOUD_KITCHEN
PASTRY_BAKERY
SHOP_GROCERY
MARKET
WHOLESALE

through 9C unless an accepted repository specification has explicitly added those domains to Section 9.

The 9C API must reject or fail closed on unsupported capabilities.

Do not make Food discovery accidentally become Shop discovery.

======================================================================
27. MULTI-TENANT SECURITY
======================================================================

9C is a high-value read surface because it aggregates many records.

Tenant isolation is mandatory.

Prevent:

- Business A reading Business B catalogue
- Branch A reading Branch B catalogue
- caller swapping branch refs
- item lookup outside requested branch
- variant from another business
- modifier from another business
- revision lookup outside scope
- search across tenants

Every aggregate query must be scoped correctly.

Do not rely only on post-query filtering.

Use tenant/branch predicates in SQL.

======================================================================
28. NESTED-IDOR HARDENING
======================================================================

Test:

branch catalogue using foreign branch

branch menu using foreign menu

item through foreign branch

sellability through foreign branch

pricing:
- foreign item
- foreign variant
- foreign modifier
- modifier from unattached group

search:
- foreign tenant result leakage

revision:
- foreign branch

Return repository-consistent not-found/forbidden behavior.

Do not leak object existence unnecessarily.

======================================================================
29. INTERNAL REQUEST LIMITS
======================================================================

Internal does not mean unlimited.

Bound:

- search query length
- page size
- modifier refs per price request
- filters per request
- total catalogue response size
- menu/section/item counts where necessary

Reject abusive payload sizes.

Use existing HTTP body limits where available.

======================================================================
30. HMAC REPLAY / NONCE TESTS
======================================================================

For all new internal routes, verify accepted internal security properties.

At minimum:

- missing signature denied
- invalid signature denied
- expired timestamp denied
- unapproved caller denied
- replayed nonce denied where nonce applies
- altered body invalidates signature for POST pricing route
- wrong caller identity denied

Reuse existing security helpers and tests.

Do not weaken HMAC v2.

======================================================================
31. READ-ONLY INTERNAL API
======================================================================

9C's service-facing discovery surface should be read-only.

Do not expose internal mutation endpoints through 9C.

Pricing resolve is computation/read authority, not a mutation.

Search is read-only.

Catalogue retrieval is read-only.

Revision retrieval is read-only.

No new idempotency store is needed for pure reads.

Do not create command records for GET requests merely for symmetry.

======================================================================
32. DATABASE MIGRATIONS
======================================================================

9C may require ZERO migrations if the accepted 9A/9B schema already supports safe discovery.

Do NOT create migrations just because every previous section had migrations.

If performance/query/index work genuinely requires schema changes:

determine the actual current migration tip and allocate the next numbers.

If baseline remains 000032, possible next number is:

000033_...

But do not create it without need.

Allowable 9C migration reasons include:

- missing critical query index
- revision support gap
- safe read-model structure proven necessary

Not allowable:

- migration solely to mark Section 9C
- duplicating existing truth
- speculative customer tables

Report explicitly:

9C migrations:
NONE

if none are needed.

======================================================================
33. INDEX / QUERY HARDENING
======================================================================

Inspect queries created for:

- branch catalogue
- menu/section/item traversal
- search
- sellability
- price resolution
- revision lookup

Ensure appropriate existing indexes are used.

Add only justified indexes.

Use EXPLAIN / EXPLAIN ANALYZE on representative non-trivial queries where useful.

Do not optimize blindly.

Target realistic catalogue sizes.

======================================================================
34. PERFORMANCE FIXTURE
======================================================================

Create a realistic bounded performance fixture.

For example:

- 1 business
- several branches
- multiple RESTAURANT/FAST_FOOD menus
- dozens of sections
- several hundred food items
- variants
- modifier groups
- modifiers
- schedules
- operational availability records
- branch overrides
- prep/logistics profiles

Test:

- full branch catalogue
- search
- one sellability lookup
- one price resolve

Look for:

- N+1 queries
- pathological per-item queries
- unbounded memory growth
- unnecessary one-query-per-modifier behavior

Do not create an enormous benchmark.

======================================================================
35. AVOID N+1
======================================================================

The branch catalogue endpoint must not perform naïve:

1 menu query
+
N section queries
+
N item queries
+
N variant queries
+
N modifier queries

for every child.

Use set-based/batched repository loading appropriate to current architecture.

Measure/query-count test if current test infrastructure supports it.

Do not sacrifice correctness merely to reduce query count.

======================================================================
36. CONSISTENT ORDERING
======================================================================

Discovery output must respect deterministic ordering established in 9A.

At minimum:

menus
sections
items
variants
modifier groups
modifiers

must return deterministic positions/order.

Do not depend on unspecified PostgreSQL row order.

Use explicit ORDER BY.

======================================================================
37. TIME EVALUATION
======================================================================

Discovery defaults to:

current authoritative server time

unless an explicit evaluation time is supplied on an approved internal preview/test route.

Do not allow arbitrary callers to manipulate production discovery time without clear API semantics.

Sellability must use the branch IANA timezone through the accepted 9B resolver.

Do not reimplement schedule logic in 9C.

======================================================================
38. TEMPORARY AVAILABILITY FRESHNESS
======================================================================

`effective_until` expiry must be reflected correctly even if no worker physically clears the record.

A catalogue fetch after expiry must use the correct effective state.

Do not rely solely on stale stored operational status.

Use the 9B effective resolver semantics.

======================================================================
39. INVENTORY FAIL-CLOSED BEHAVIOR
======================================================================

Current accepted state:

INVENTORY_AVAILABILITY_SUPPORT = NOT_PRESENT

Therefore:

INVENTORY_LINKED items must not be surfaced as sellable through consumer discovery merely because the catalogue exists.

Discovery must honor:

INVENTORY_SOURCE_UNAVAILABLE

or repository-equivalent reason.

Do not fabricate inventory truth.

Do not hide the dependency by returning sellable=true.

======================================================================
40. LOGISTICS PROFILE BOUNDARY
======================================================================

9C may expose Vendor-owned logistics metadata for future consumers.

It must remain metadata.

Possible values may include:

- temperature class
- package size
- spill risk
- fragility
- weight
- vehicle suitability hints

Do not convert hints into:

- assigned delivery service
- assigned vehicle
- provider mission
- route
- price
- delivery SLA

Those are later logistics responsibilities.

======================================================================
41. PREPARATION PROFILE BOUNDARY
======================================================================

9C may expose:

prep_min_minutes
prep_max_minutes

Do NOT expose fabricated:

delivery ETA
driver arrival ETA
customer promise time

The future consumer can combine Vendor prep truth with logistics/travel truth elsewhere.

======================================================================
42. MEDIA
======================================================================

Discovery returns only safe Media references / presentation references already permitted by the accepted Media integration.

Do not store Media bytes.

Do not call Media for every catalogue item if this would create N+1 network behavior.

Reuse the accepted reference/presentation architecture.

If 9A stores opaque refs that require a future caller to resolve Media separately, document that.

Do not expand Media Service unnecessarily.

======================================================================
43. CUSTOMER PII
======================================================================

Food discovery requires ZERO customer PII.

Do not accept:

- customer name
- customer phone
- customer email
- customer address

merely to retrieve catalogue data.

Location-aware restaurant discovery across branches is outside this exact Vendor branch-catalogue contract unless an accepted upstream service provides branch selection.

9C does not become customer identity authority.

======================================================================
44. NO LOCATION/RANKING ENGINE
======================================================================

Do not build:

- nearest restaurant search
- GPS distance
- geospatial ranking
- restaurant recommendation
- popularity ranking
- sponsored ranking

unless those are already accepted explicit Section 9 requirements, which is not expected.

9C assumes the caller has selected or resolved a Vendor branch where necessary.

Future Customer discovery orchestration can decide which branches to query.

======================================================================
45. NO CROSS-BRANCH CART / AGGREGATION
======================================================================

Do not create multi-vendor/multi-branch shopping aggregation.

A branch catalogue response is branch-scoped.

Do not:

- calculate multi-vendor delivery fees
- merge carts
- select riders
- split orders

Those are later commerce/order/logistics responsibilities.

======================================================================
46. API RESPONSE VERSIONING
======================================================================

Internal Food contracts must be versionable.

Use existing:

/internal/vendor/v1/...

versioning architecture.

Avoid response structures that cannot evolve additively.

Use closed enums where business logic requires deterministic semantics.

Document enum values in OpenAPI.

======================================================================
47. ERROR MODEL
======================================================================

Reuse existing Vendor error envelopes.

Add 9C errors only where required.

Conceptual examples:

FOOD_BRANCH_NOT_FOUND
FOOD_CAPABILITY_NOT_SUPPORTED
FOOD_ITEM_NOT_FOUND
FOOD_ITEM_NOT_SELLABLE
INVALID_FOOD_SEARCH
INVALID_MODIFIER_SELECTION
CATALOGUE_TOO_LARGE
CATALOGUE_REVISION_MISMATCH
INVENTORY_SOURCE_UNAVAILABLE

Do not invent inconsistent HTTP semantics.

======================================================================
48. OBSERVABILITY
======================================================================

Use existing logging/metrics conventions.

Log safe operational context such as:

- caller service
- business ref
- branch ref
- operation
- response outcome
- duration

Do not log:

- HMAC secrets
- full signatures
- customer PII
- arbitrary request bodies containing future sensitive data

If metrics architecture exists, expose bounded useful counters/timers according to established patterns.

Do not create an entire observability platform.

======================================================================
49. RESILIENCE
======================================================================

9C core reads should depend primarily on Vendor's own PostgreSQL truth.

Avoid synchronous calls to other services in the hot catalogue read path unless an accepted dependency absolutely requires them.

Current Inventory authority is NOT_PRESENT.

Therefore do not call a fake Inventory endpoint.

Do not turn Food catalogue retrieval into a chain of remote dependencies.

======================================================================
50. NO NETWORK CALL INSIDE DB LOCKED TRANSACTION
======================================================================

Preserve the Section 8I architectural rule.

Do not perform remote network calls while holding critical Vendor row locks or read transactions unnecessarily.

9C should ideally perform no external network writes.

Read-only service-facing APIs should remain simple and bounded.

======================================================================
51. FINAL SECTION 9 SECURITY REVIEW
======================================================================

9C is also the final Section 9 hardening pass.

Perform a targeted review of ONLY Section 9 code for:

- tenant isolation
- branch isolation
- nested-IDOR
- internal/public route separation
- HMAC/replay protection
- pricing trust boundary
- availability trust boundary
- integer money
- SQL injection
- unbounded queries
- unsafe archived record exposure
- stale catalogue behavior
- inventory fail-closed behavior

Do not perform a whole-repository security audit.

Fix concrete Section 9 defects discovered.

Add regression tests for every real defect fixed.

======================================================================
52. FINAL SECTION 9 CONCURRENCY REVIEW
======================================================================

Verify Section 9 interactions under concurrent changes.

Important cases:

CATALOGUE READ vs ITEM PRICE UPDATE

The read must return one coherent valid revision/state, not a fabricated mixed result.

CATALOGUE READ vs SOLD_OUT

After successful sold-out commit, subsequent reads must not continue returning item as sellable due to stale internal truth.

REVISION READ vs MUTATION

Revision must advance monotonically.

SEARCH vs ARCHIVE

Archived item must not reappear due to stale projection.

PRICING RESOLVE vs PRICE MUTATION

Resolver must use a coherent authoritative version set.

Do not overpromise strict serializability if current database isolation does not provide it.

Define and test the actual guarantee.

======================================================================
53. FINAL SECTION 9 PRICE HISTORY CERTIFICATION
======================================================================

As part of final Section 9 certification, confirm targeted integrity of:

- item price history
- variant price history
- modifier price history
- branch override price history
- BASELINE_IMPORT
- append-only protection
- clear-to-inherit versions
- pricing resolver version refs

Do not rerun every 9B test repeatedly.

Run focused final certification.

======================================================================
54. FINAL SECTION 9 SCHEDULE CERTIFICATION
======================================================================

Confirm:

- IANA timezone
- DST
- overnight windows
- no-schedule semantics
- seasonal bounds
- archived schedule exclusion

Do not reimplement schedule logic in 9C.

======================================================================
55. FINAL SECTION 9 AVAILABILITY CERTIFICATION
======================================================================

Confirm:

business
→ branch
→ capability
→ menu
→ schedule
→ section
→ item
→ operational availability
→ inventory seam

and:

lower layers cannot resurrect upper-layer denial.

Ensure 9C consumption uses exactly this truth.

======================================================================
56. FINAL SECTION 9 IAM CERTIFICATION
======================================================================

9A/9B management routes retain their accepted IAM model.

9C internal routes use service authentication, not Vendor staff IAM, unless current architecture explicitly combines them.

Do not accidentally expose internal discovery through staff public authentication alone.

Test separation:

PUBLIC MANAGEMENT ROUTES
=
staff IAM

INTERNAL CONSUMPTION ROUTES
=
internal service auth

Do not blur the two.

======================================================================
57. ROUTE REGISTRATION
======================================================================

Wire all 9C routes into the actual production router.

Do not leave dead handlers.

Do not mount internal routes under the public `/api/v1/vendor` group.

Update the global route inventory contract.

At completion derive:

PRE_9C_ROUTE_COUNT
NEW_9C_PUBLIC_ROUTES
NEW_9C_INTERNAL_ROUTES
POST_9C_ROUTE_COUNT

Do not guess.

Expected 9C likely adds mostly internal routes.

Do not add public routes merely to increase route count.

======================================================================
58. SECTION 9C OPENAPI
======================================================================

Create/update a dedicated 9C OpenAPI contract according to repository convention.

Likely:

services/vendor-service/docs/openapi/section9c-food-discovery.openapi.json

if consistent with 9A/9B.

Use actual project conventions.

Document:

- internal HMAC authentication
- caller requirements
- catalogue route
- search/filter parameters
- sellability result
- pricing request/result
- catalogue revision
- freshness semantics
- enums
- pagination
- integer minor-unit pricing
- allergen/dietary disclaimer semantics
- inventory fail-closed state
- error responses

OpenAPI must match production routes bidirectionally.

======================================================================
59. GLOBAL ROUTE CONTRACT
======================================================================

Preserve the Section 8I global production-router contract guard.

9A and 9B already participate in it.

Add 9C.

The global contract must detect:

- undocumented production routes
- documented phantom routes
- internal route mounted under public root
- method drift
- path drift

Do not weaken the guard to make tests pass.

======================================================================
60. SECTION 9 CONTRACT PACKAGE
======================================================================

If current repository convention uses per-section contract packages, create/update a Section 9C contract guard.

It should pin the exact 9C surface.

Also add a final Section 9 aggregate contract/certification test if useful.

Avoid duplicate test machinery where the global contract already provides the guarantee.

======================================================================
61. API ROUTE REGISTER
======================================================================

Update:

services/vendor-service/docs/API_ROUTE_REGISTER.md

Record actual 9C routes with:

- method
- path
- internal/public
- authentication
- caller/permission model
- Section 9C ownership

Also reconcile Section 9A/9B counts if necessary.

Do not document future Order/Customer routes as implemented.

======================================================================
62. IMPLEMENTATION STATUS
======================================================================

Update:

services/vendor-service/docs/IMPLEMENTATION_STATUS.md

After a successful 9C certification:

SECTION 9A:
IMPLEMENTED + TESTED

SECTION 9B:
IMPLEMENTED + TESTED

SECTION 9C:
IMPLEMENTED + TESTED + SECURITY-CERTIFIED

SECTION 9:
COMPLETE / GO

Only mark Section 9 complete after final certification passes.

======================================================================
63. PLATFORM SERVICE CHANGE REGISTER
======================================================================

Update:

services/vendor-service/docs/PLATFORM_SERVICE_CHANGE_REGISTER.md

Record the actual Vendor-side internal contract created for future consumers.

Do not claim another service has integrated unless it actually has.

Expected statuses may include:

FOOD_DISCOVERY_VENDOR_CONTRACT = AVAILABLE

CUSTOMER_FOOD_DISCOVERY_CONSUMER = NOT_PRESENT
or repository-equivalent if no consumer exists

ORDER_AUTHORITY = NOT_PRESENT

INVENTORY_AVAILABILITY_SUPPORT = NOT_PRESENT

LOGISTICS_EXECUTION_INTEGRATION = NOT_PRESENT

Use actual established status vocabulary.

Do not fabricate completed integrations.

======================================================================
64. README / ARCHITECTURE DOCS
======================================================================

Update relevant Vendor documentation.

Document:

- Section 9 complete architecture
- 9A ownership
- 9B operational truth
- 9C consumption contract
- internal HMAC route boundary
- revision/freshness model
- search/filtering behavior
- sellability contract
- pricing contract
- prep/logistics metadata boundary
- Inventory fail-closed state
- later Order/Customer/logistics dependencies

Keep documentation concise and authoritative.

======================================================================
65. UI / FRONTEND HANDOFF
======================================================================

Backend only.

Do NOT implement frontend.

Update:

services/vendor-service/docs/UI_UX_REQUIREMENTS.md

services/vendor-service/docs/FRONTEND_REQUIREMENTS.md

with backend contract implications such as:

- Food catalogue response structure
- revision/freshness behavior
- unavailable/sold-out semantics
- price minor-unit representation
- configurable item flow
- variant selection
- modifier min/max
- allergen/dietary metadata
- preparation ranges
- inventory unavailable state
- no direct frontend access to internal service routes

Clarify:

frontend/customer apps should consume through the future appropriate Customer/API layer rather than directly possessing internal HMAC credentials.

======================================================================
66. CUSTOMER SERVICE / API GATEWAY
======================================================================

Do NOT automatically modify Customer Service or API Gateway in Section 9C.

First determine whether the authoritative master architecture explicitly requires a consumer-side change inside Section 9.

Default expected behavior:

Vendor exposes the safe internal contract.

Future Customer/API Gateway work consumes it in its proper section.

If no consumer integration currently exists:

document:

PENDING_EXTERNAL_CONSUMER

Do not fabricate a Customer Service implementation just to make 9C appear integrated.

======================================================================
67. NO FRONTEND DIRECT INTERNAL ACCESS
======================================================================

Internal HMAC endpoints must never be documented as directly callable from:

- mobile Customer app
- browser
- Vendor frontend
- public internet client

They are service-to-service contracts.

Frontend requirements must explicitly state that an upstream backend/API layer consumes them.

======================================================================
68. NO ORDERS
======================================================================

Do NOT create:

orders
order_items
order snapshots
order modifier selections
restaurant order queue
accept/decline
checkout
cart
payment

The 9C pricing contract is future-order-compatible but does not create an Order authority.

Document:

ORDER_AUTHORITY = NOT_PRESENT

or existing equivalent.

======================================================================
69. NO INVENTORY AUTHORITY
======================================================================

Do not create:

inventory stock
ingredient stock
recipes as stock authority
stock reservations
stock decrement
availability publisher

The accepted seam remains fail closed.

Inventory integration belongs later when an authoritative provider exists.

======================================================================
70. NO LOGISTICS EXECUTION
======================================================================

Do not call:

Dispatch
Taxi
Hauling

Do not:

- assign provider
- select actual vehicle
- create mission
- calculate route
- create OTP
- create QR
- calculate delivery fee
- calculate delivery SLA

9C may expose logistics metadata only.

======================================================================
71. NO PAYMENT / WALLET
======================================================================

Do not:

- charge customer
- hold funds
- calculate payout
- move Wallet balance
- settle vendor
- calculate platform commission

Catalogue price resolution remains commercial configuration truth.

======================================================================
72. NO PROMOTIONS
======================================================================

Do not add:

- coupon engine
- promotion engine
- scheduled discount
- campaign price
- free delivery
- loyalty discount
- surge

Those are outside Section 9.

======================================================================
73. TESTING STRATEGY
======================================================================

Use targeted tests while developing.

Suggested order:

A. internal auth/route wiring
B. catalogue projection
C. search/filtering
D. pricing consumption
E. sellability consumption
F. revision/freshness
G. tenant/IDOR
H. concurrency/coherent reads
I. performance/N+1
J. OpenAPI/route contract
K. final Section 9 certification

Do not repeatedly run all CRM tests.

======================================================================
74. REQUIRED TESTS — INTERNAL AUTH
======================================================================

Test:

- approved caller accepted
- missing HMAC denied
- invalid signature denied
- expired timestamp denied
- nonce replay denied
- unapproved caller denied
- altered POST pricing body denied
- route root is `/internal/...`
- no 9C route accidentally available under `/api/v1/vendor/internal/...`

======================================================================
75. REQUIRED TESTS — CATALOGUE
======================================================================

Test:

- branch catalogue returns active/sellable menu hierarchy
- deterministic ordering
- paused menu excluded
- archived menu excluded
- outside schedule excluded
- archived section excluded
- archived item excluded
- SOLD_OUT item excluded from sellable discovery
- PAUSED item excluded
- inventory-linked/no authority excluded
- correct media refs
- correct dietary codes
- correct allergen codes
- variants included correctly
- modifier groups included correctly
- modifiers included correctly
- no cross-tenant data
- no management-only internal fields

======================================================================
76. REQUIRED TESTS — SEARCH
======================================================================

Test:

- text search
- bounded query length
- menu filter
- section filter
- capability filter
- dietary code filter
- allergen exclusion
- sellable-only
- price range if implemented
- prep range if implemented
- pagination
- deterministic ordering
- unsupported filters rejected
- SQL injection-style input remains data, not executable SQL
- foreign tenant results never returned

======================================================================
77. REQUIRED TESTS — PRICING CONTRACT
======================================================================

Test:

- item base
- branch override
- inherited variant
- explicit variant
- modifiers
- min/max
- required groups
- duplicate modifier
- unattached modifier
- foreign modifier
- archived modifier
- overflow
- currency
- price-version refs
- catalogue revision
- client-supplied fake price cannot influence result

======================================================================
78. REQUIRED TESTS — SELLABILITY CONTRACT
======================================================================

Test:

business inactive
branch inactive
capability inactive
menu inactive
outside schedule
section inactive
item inactive
SOLD_OUT
PAUSED
inventory source unavailable
sellable case

Verify bounded reason codes.

Verify lower layers cannot resurrect higher-level denial.

======================================================================
79. REQUIRED TESTS — REVISION/FRESHNESS
======================================================================

Test:

- revision returned
- revision changes after relevant mutation
- unchanged catalogue does not spuriously advance due to read
- lightweight revision endpoint
- since-revision/conditional semantics if implemented
- concurrent mutations produce monotonic revision
- catalogue response revision matches coherent response state
- sold-out commit reflected in subsequent read
- price update reflected in subsequent price resolution
- archive reflected in subsequent search/catalogue result

======================================================================
80. REQUIRED TESTS — TENANT / IDOR
======================================================================

Test:

- foreign business
- foreign branch
- foreign menu
- foreign item
- foreign variant
- foreign modifier
- foreign revision
- foreign search results

No cross-tenant leakage.

======================================================================
81. REQUIRED TESTS — PERFORMANCE
======================================================================

Use a moderate fixture.

Verify:

- no obvious N+1 catalogue traversal
- search bounded
- price resolve bounded
- sellability bounded
- reasonable query/index plans

Do not set arbitrary unrealistic microsecond performance promises.

Report measured/observed query behavior rather than marketing claims.

======================================================================
82. REQUIRED TESTS — CONCURRENCY
======================================================================

Deterministically test:

catalogue read vs sold-out mutation

catalogue read vs item archive

price resolve vs concurrent price mutation

revision lookup vs concurrent revision increment

search vs archive

The system must return coherent valid state, not cross-version fabricated state.

Use actual PostgreSQL transaction behavior.

Avoid flaky sleeps.

======================================================================
83. FINAL SECTION 9 MIGRATION CERTIFICATION
======================================================================

Run migration/replay validation covering:

000029
000030
000031
000032

plus any real 9C migration if introduced.

Verify:

- fresh database replay
- accepted Sections 5–8 still migrate
- 9A schema exists
- 9B operational schema exists
- 9C additions exist if any
- constraints remain valid

Do not create fake migration merely for this test.

======================================================================
84. FINAL SECTION 9 CONTRACT CERTIFICATION
======================================================================

Certify:

9A management API
+
9B operational API
+
9C internal consumption API

against:

- production router
- OpenAPI
- API_ROUTE_REGISTER

No undocumented route.

No phantom route.

No internal route on public base.

No method mismatch.

======================================================================
85. FINAL SECTION 9 REGRESSION
======================================================================

Run final Section 9 regression ONCE after implementation stabilizes.

At minimum include:

- all food feature packages
- Section 9A contract tests
- Section 9B contract tests
- Section 9C contract tests
- global route contract
- migrations
- relevant IAM/security
- PostgreSQL/Redis integration where required

Do not rerun the enormous accepted CRM suite unless Section 9C changes a truly shared CRM/core component.

If broad Go package tests contend on a shared database:

use isolated deterministic test databases/groups.

Document harness contention separately from code failures.

Do not hide real failures.

======================================================================
86. BUILD QUALITY
======================================================================

At final gate run appropriate commands for:

gofmt on touched Go files

go vet for vendor-service / affected packages

go build for vendor-service

git diff --check

Also ensure no accidental executable/vendor binary is created/tracked.

Do not format unrelated files.

======================================================================
87. CLEANUP OF TEMPORARY TEST ARTIFACTS
======================================================================

Do not leave temporary Section 9 test logs or generated binaries in the service directory unless intentionally documented/ignored.

Remove only artifacts created by this Section 9C session that are clearly temporary and untracked.

Do NOT delete unrelated existing files.

Do not use `git clean`.

If temporary `.tmp-section9*` logs are created, delete them after results are recorded unless the repository intentionally preserves them.

======================================================================
88. DOCUMENTATION TRUTH
======================================================================

At completion documentation must distinguish:

IMPLEMENTED VENDOR CONTRACT

from:

ACTUAL EXTERNAL CONSUMER INTEGRATION

Do not say:

"Customer Food discovery integrated"

if no Customer/API service has actually consumed the contract.

Correct wording:

"Vendor Food discovery/consumption contract available; external consumer integration pending."

======================================================================
89. SECTION 9 FINAL BOUNDARY MATRIX
======================================================================

Final report must classify:

VENDOR FOOD CATALOGUE AUTHORITY:
AVAILABLE

VENDOR EFFECTIVE PRICING:
AVAILABLE

VENDOR EFFECTIVE SELLABILITY:
AVAILABLE

VENDOR INTERNAL FOOD DISCOVERY CONTRACT:
AVAILABLE

CATALOGUE REVISION/FRESHNESS:
AVAILABLE

INVENTORY_AVAILABILITY_AUTHORITY:
NOT_PRESENT unless repository proves otherwise

ORDER_AUTHORITY:
NOT_PRESENT unless repository proves otherwise

CUSTOMER DISCOVERY CONSUMER:
report actual state

PAYMENT/WALLET EXECUTION:
OUTSIDE SECTION 9

LOGISTICS EXECUTION:
OUTSIDE SECTION 9

Do not fabricate statuses.

======================================================================
90. SECTION 9C ACCEPTANCE CRITERIA
======================================================================

Section 9C is GO only when all are satisfied:

1. Read-optimized Food catalogue contract implemented.
2. Contract is service-facing/internal.
3. Internal HMAC v2 applied.
4. Caller allowlisting applied.
5. Replay protection preserved.
6. Internal routes mounted under correct root.
7. No internal route leaks under public Vendor root.
8. Branch catalogue hierarchy implemented.
9. Only effectively discoverable/sellable catalogue data returned by default.
10. 9B sellability resolver reused.
11. No second sellability algorithm created.
12. Effective catalogue pricing exposed safely.
13. 9B pricing resolver reused.
14. No client-supplied price trusted.
15. Variant configuration exposed.
16. Modifier groups exposed.
17. Modifier min/max semantics exposed.
18. Modifier pricing exposed in minor units.
19. Search implemented if required by this spec.
20. Search bounded.
21. Search SQL safe.
22. Dietary filtering supported.
23. Allergen exclusion supported without medical claims.
24. Pagination/response bounds enforced.
25. Catalogue revision exposed.
26. Freshness/change-aware behavior implemented.
27. Revision is monotonic.
28. Discovery state/revision reads are coherent.
29. Sold-out change visible immediately to subsequent discovery.
30. Archive visible immediately to subsequent discovery.
31. Price change visible immediately to subsequent resolver.
32. Inventory-linked items fail closed while authority absent.
33. Prep profile exposed as prep metadata only.
34. Logistics profile exposed as handling metadata only.
35. No travel ETA fabricated.
36. No logistics service selected.
37. Tenant isolation enforced.
38. Branch isolation enforced.
39. Nested-IDOR blocked.
40. HMAC security tests pass.
41. Search security tests pass.
42. Pricing security tests pass.
43. Sellability tests pass.
44. Concurrency/freshness tests pass.
45. Performance fixture passes without obvious N+1 design.
46. Query/index relationship reviewed.
47. No unnecessary materialized second authority.
48. Cache, if used, remains non-authoritative and revision-safe.
49. No Customer PII introduced.
50. No Order authority introduced.
51. No Inventory authority introduced.
52. No Wallet/payment execution introduced.
53. No logistics execution introduced.
54. No frontend implemented.
55. OpenAPI complete.
56. 9C route contract passes.
57. Global route contract passes.
58. API_ROUTE_REGISTER updated.
59. IMPLEMENTATION_STATUS updated.
60. PLATFORM_SERVICE_CHANGE_REGISTER updated.
61. README/architecture updated.
62. UI_UX_REQUIREMENTS updated where required.
63. FRONTEND_REQUIREMENTS updated where required.
64. Final Section 9 migration replay passes.
65. Final Section 9A/9B/9C regression passes.
66. gofmt clean.
67. go vet passes.
68. go build passes.
69. git diff --check passes.
70. Temporary test artifacts cleaned.
71. Section 10 not started.
72. No Git commit/stage/push/branch/worktree operation performed.

======================================================================
91. FINAL SECTION 9 ACCEPTANCE CRITERIA
======================================================================

SECTION 9 as a whole is GO only if:

SECTION 9A:
GO

SECTION 9B:
GO

SECTION 9C:
GO

AND the complete Restaurant/Fast-Food domain satisfies the original Section 9 requirements:

ENTITIES:

food_menus
food_menu_sections
food_items
food_item_variants
modifier_groups
modifiers
menu_schedules
branch_item_overrides

REQUIREMENTS:

- branch/capability menus
- ordered menu sections
- food item description/media
- integer base prices
- dietary/allergen metadata
- variants
- modifiers
- required/optional min/max modifier choices
- schedules
- branch price/availability/prep overrides
- price changes versioned
- historical price truth preserved for future Order snapshots
- sold-out/paused reflected immediately in discovery
- prep time available as future ETA/dispatch input
- logistics handling profile available
- optional inventory/ingredient availability seam supported by design
- Inventory-linked mode fails closed when external authority absent
- safe service-facing discovery contract exists
- effective pricing can be consumed
- effective sellability can be consumed
- catalogue freshness/revision can be consumed

Do not declare Section 9 GO if one of these is materially missing.

======================================================================
92. MIGRATION / ROUTE REPORTING
======================================================================

At completion derive REAL values.

Report:

PRE_9C_ROUTE_COUNT

NEW_9C_PUBLIC_ROUTES

NEW_9C_INTERNAL_ROUTES

POST_9C_ROUTE_COUNT

SECTION_9_TOTAL_NEW_ROUTES

Report:

PRE_9C_MIGRATION_TIP

NEW_9C_MIGRATIONS

POST_9C_MIGRATION_TIP

If no migration:

NEW_9C_MIGRATIONS:
NONE

Do not invent one.

Also report:

- total Section 9 tables
- internal discovery route count
- final Vendor route count

======================================================================
93. FINAL REPORT FORMAT
======================================================================

Return the following exact structure:

# SECTION 9C + FINAL SECTION 9 IMPLEMENTATION REPORT

OVERALL VERDICT:
GO / NO-GO

## REPOSITORY

Branch:
Starting HEAD:
Ending HEAD:
Committed:
Staged:
Merged:
Branch created:
Branch switched:
Pushed:

## ACCEPTED BASELINE

9A:
9B:
Pre-9C routes:
Pre-9C migration tip:

## SECTION 9C IMPLEMENTATION

Internal Food catalogue contract:
Catalogue hierarchy:
Search/filtering:
Effective sellability:
Effective pricing:
Revision/freshness:
Conditional/change-aware reads:
Media references:
Prep profile:
Logistics profile:
Inventory fail-closed behavior:

## INTERNAL SECURITY

HMAC v2:
Caller allowlist:
Nonce/replay:
Body integrity:
Internal/public root separation:
Tenant isolation:
Branch isolation:
Nested-IDOR:

## DATABASE

New 9C migrations:
Post-9C migration tip:
New tables:
New indexes:
Read-model/projection:
Cache:

If no migration/table/cache was needed, say so explicitly.

## API

Pre-9C routes:
New 9C public routes:
New 9C internal routes:
Post-9C Vendor routes:

Section 9 total routes added:
OpenAPI parity:
Global route parity:

## DISCOVERY CONTRACT

Catalogue endpoint:
Search endpoint:
Sellability endpoint:
Pricing endpoint:
Revision endpoint:

List actual routes, not guessed routes.

## FRESHNESS / CONSISTENCY

Catalogue revision:
Immediate sold-out visibility:
Immediate archive visibility:
Immediate price visibility:
Concurrent read guarantee:
Stale projection protection:

## PERFORMANCE

Fixture size:
Catalogue query behavior:
Search query behavior:
Pricing query behavior:
Sellability query behavior:
N+1 status:
Important indexes/query plans:

## TESTS

Internal auth:
HMAC replay:
Catalogue:
Search:
Pricing:
Sellability:
Revision/freshness:
Tenant/IDOR:
Concurrency:
Performance:
Migration/replay:
Section 9A:
Section 9B:
Section 9C:
Global route/OpenAPI:

PostgreSQL:
Redis:

Failures:
Skips:

## BUILD QUALITY

gofmt:
go vet:
go build:
git diff --check:
Temporary artifacts:

## SECTION 9 DOMAIN MATRIX

Food catalogue authority:
Effective pricing:
Effective sellability:
Internal Food discovery contract:
Catalogue freshness:
Inventory availability authority:
Order authority:
Customer discovery consumer:
Payment/Wallet:
Logistics execution:

## EXTERNAL SERVICE IMPACT

Media:
Customer:
API Gateway:
Order:
Inventory:
Wallet:
Dispatch:
Taxi:
Hauling:

## DEFERRED OUTSIDE SECTION 9

List exact future-domain dependencies, including as applicable:

- customer-facing orchestration
- cart
- checkout
- Order authority
- Inventory authority integration
- payment
- vendor settlement
- logistics execution
- delivery fees
- delivery mission
- promotions

Do NOT label these as incomplete 9C defects if they are correctly outside Section 9.

## DOCUMENTATION UPDATED

List files.

## FINAL SECTION 9 CERTIFICATION

9A:
GO / NO-GO

9B:
GO / NO-GO

9C:
GO / NO-GO

SECTION 9 — RESTAURANT AND FAST-FOOD DOMAIN:
GO / NO-GO

REMAINING SECTION 9 CODE DEFECTS:
NONE or list them.

Confirm explicitly:

- No commit
- No stage
- No merge
- No branch created
- No branch switch
- No push
- No frontend implementation
- No Order authority created
- No Inventory authority created
- No Wallet/payment implementation
- No logistics execution
- Section 10 not started

======================================================================
94. DEVELOPMENT ORDER
======================================================================

Use this order.

PHASE A — FAST BASELINE

Verify:

- branch
- HEAD
- dirty state
- actual route count
- migration tip

Locate ONLY:

- Section 9A food package
- Section 9B food operations package
- internal HMAC v2 registration pattern
- global route contract
- OpenAPI conventions

Then code.

PHASE B — INTERNAL CONTRACT DESIGN IN CODE

Define minimal DTOs/contracts for:

- catalogue
- search
- sellability
- pricing
- revision

Do not spend excessive time writing a separate plan.

PHASE C — REPOSITORY / QUERY LAYER

Implement:

- efficient branch catalogue loading
- bounded search
- revision reads
- required batch loading

Avoid N+1.

PHASE D — APPLICATION/SERVICE

Reuse:

- 9B effective pricing
- 9B effective availability

Build:

- catalogue assembly
- filtering
- freshness semantics

PHASE E — INTERNAL HTTP + HMAC

Wire actual `/internal/vendor/v1/...` routes.

PHASE F — TARGETED FUNCTIONAL TESTS

Get:

- catalogue
- pricing
- sellability
- revision
- search

green.

PHASE G — SECURITY

Run:

- HMAC
- replay
- caller allowlist
- tenant/branch IDOR
- root separation

PHASE H — PERFORMANCE / CONCURRENCY

Run:

- realistic fixture
- N+1 check
- coherent read/freshness tests

PHASE I — OPENAPI / DOCS

Update after behavior stabilizes.

PHASE J — FINAL SECTION 9 CERTIFICATION

Run final 9A+9B+9C Food-domain gate once.

PHASE K — FINAL REPORT

Return only the required report.

======================================================================
95. THINGS YOU MUST NOT DO
======================================================================

DO NOT:

- build frontend
- build Customer app
- build Customer Service unless explicitly required by an existing accepted Section 9 contract
- build cart
- build checkout
- build Orders
- build restaurant order acceptance
- build Wallet logic
- build payments
- build settlements
- build Inventory stock
- build ingredient stock
- build stock reservation
- build delivery assignment
- call Dispatch
- call Taxi
- call Hauling
- build delivery OTP/QR
- build delivery pricing
- build travel ETA
- build dynamic pricing
- build promotions
- build coupons
- build recommendation AI
- build cross-branch ranking
- create customer PII
- access another service DB
- use floating-point money
- expose internal routes publicly
- create a second sellability engine
- create a second pricing engine
- create a second catalogue authority
- create migrations without actual schema need
- create Redis caching merely for appearance
- introduce new search infrastructure unnecessarily
- create branches/worktrees
- commit
- stage
- push
- stash
- reset
- clean
- broadly refactor unrelated code
- repeatedly run the full CRM regression
- start Section 10

======================================================================
96. START NOW
======================================================================

Begin with the fast baseline verification.

Do NOT summarize this document.

Do NOT produce another long plan.

Inspect only enough code to identify:

- accepted Section 9A data model
- accepted Section 9B resolvers/revisions
- current internal HMAC v2 pattern
- current production router
- current global contract tests

Then immediately implement Section 9C.

Continue through:

internal discovery contract
→ efficient catalogue queries
→ search/filtering
→ sellability consumption
→ pricing consumption
→ revision/freshness
→ internal HMAC security
→ IDOR hardening
→ concurrency/freshness
→ performance
→ OpenAPI/docs
→ final Section 9 certification

If you uncover a real defect in 9A or 9B that directly prevents Section 9 from meeting its accepted requirements:

- fix it narrowly;
- add regression coverage;
- report it clearly.

Do not perform unrelated cleanup.

Continue until Section 9C and final Section 9 acceptance criteria are satisfied or a genuine blocking external dependency is proven.

START SECTION 9C IMPLEMENTATION NOW.