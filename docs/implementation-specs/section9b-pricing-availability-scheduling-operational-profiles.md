KARRYGO — VENDOR SERVICE
SECTION 9B — PRICING, AVAILABILITY, SCHEDULING & OPERATIONAL PROFILES
FULL BACKEND IMPLEMENTATION PROMPT

MODE: DEVELOPMENT-DRIVEN
TARGET: PRODUCTION-GRADE BACKEND IMPLEMENTATION
SCOPE: SECTION 9B ONLY
REPOSITORY: C:\KarryGoPush
INTENDED BRANCH: staging

======================================================================
0. EXECUTION DIRECTIVE
======================================================================

You are implementing Section 9B of the KarryGo Vendor Service.

This is a DEVELOPMENT task.

This is NOT:
- a design-only task
- a review-only task
- a gap-analysis-only task
- a documentation-only task
- an architecture discussion

You must:

1. perform a fast targeted baseline check;
2. inspect only the relevant accepted 9A and shared Vendor patterns;
3. create required migrations;
4. implement the Section 9B domain;
5. update existing 9A price mutation paths where necessary;
6. implement pricing history/versioning;
7. implement scheduling;
8. implement operational availability;
9. implement branch item overrides;
10. implement effective price resolution;
11. implement effective availability resolution;
12. implement preparation profiles;
13. implement logistics profiles;
14. implement the inventory/ingredient availability seam without fabricating Inventory authority;
15. implement immediate catalogue-change/invalidation foundations;
16. implement IAM/audit/outbox/concurrency;
17. implement the required HTTP management surface;
18. add production-grade tests;
19. update OpenAPI and documentation;
20. run final Section 9B validation;
21. fix failures caused by Section 9B;
22. return the required final report.

DO NOT stop after planning.

DO NOT spend the session explaining the specification back to the user.

Begin implementation after the fast baseline check.

Do not ask for confirmation for ordinary implementation decisions that can be resolved safely from:

- this specification;
- the current repository;
- accepted KarryGo architecture;
- existing Vendor Service patterns.

If an actual conflict exists between this specification and an established accepted repository contract, preserve the established contract where possible and document the conflict.

======================================================================
1. USAGE-SAVING IMPLEMENTATION MODE
======================================================================

Use development time efficiently.

- Do not use subagents.
- Do not perform a broad repository audit.
- Do not browse the web.
- Do not repeatedly reread the entire specification.
- Read this specification once and treat it as the authoritative task contract.
- Reopen only specific relevant sections if needed.
- Inspect only files directly relevant to Section 9B.
- Trust accepted Sections 5–8 and accepted Section 9A.
- Do not re-certify CRM.
- Do not reimplement Section 9A.
- Do not refactor unrelated working code.
- Reuse current architecture and dependencies.
- Do not introduce unnecessary dependencies.
- Keep progress updates concise.
- Use targeted tests during development.
- Do not repeatedly run the entire Vendor Service test suite.
- Run the required broader Section 9B gate once implementation stabilizes.
- Update OpenAPI/docs after behavior stabilizes.
- Do not broaden scope.
- Do not start 9C.
- Do not start Section 10.
- Stop when Section 9B acceptance requirements are satisfied.

Reuse existing PostgreSQL and Redis test infrastructure.

Avoid repeatedly recreating infrastructure.

======================================================================
2. GIT / WORKTREE SAFETY — MANDATORY
======================================================================

Work directly in:

C:\KarryGoPush

Intended branch:

staging

Perform only this quick Git verification before implementation:

git branch --show-current
git status --short
git log -1 --oneline

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
- delete unrelated files
- restore unrelated changes
- discard dirty working-tree work

The repository is expected to be dirty.

Preserve all unrelated local work.

Leave Section 9B changes:

UNCOMMITTED
UNSTAGED

======================================================================
3. ACCEPTED BASELINE
======================================================================

Treat Sections 5–8 and Section 9A as accepted.

Do not broadly re-audit them.

Accepted Section 9A report:

SECTION 9A FOOD CATALOGUE FOUNDATION: GO

Known accepted baseline from the completion report:

- branch: staging
- HEAD: 10ba439
- Vendor routes after 9A: 235
- Section 9A routes: 39 public / 0 internal
- migration tip: 000030_vendor_section9a_permissions

Section 9A migrations:

000029_vendor_food_catalogue
000030_vendor_section9a_permissions

Section 9A tables:

- food_menus
- food_menu_sections
- food_items
- food_item_media
- food_item_variants
- food_modifier_groups
- food_modifiers
- food_item_modifier_groups

Section 9A supports:

- RESTAURANT
- FAST_FOOD

Section 9A established:

- integer minor-unit prices
- item base price
- variant absolute price override / inheritance
- modifier non-negative price deltas
- menu/section/item ordering
- opaque media references
- dietary/allergen metadata
- menu/item lifecycle
- optimistic concurrency
- transactional audit
- transactional outbox
- tenant/branch isolation
- nested-IDOR protection
- IAM
- route/OpenAPI parity

HOWEVER:

The CURRENT WORKING TREE is authoritative.

Quickly derive:

PRE_9B_ROUTE_COUNT
PRE_9B_MIGRATION_TIP

Do not blindly assume 235 / 000030 if later accepted work is already present.

Do not modify accepted 9A behavior unless required to implement 9B correctly.

======================================================================
4. SECTION 9B PURPOSE
======================================================================

Section 9A answered:

    WHAT DOES THIS BRANCH SELL?

Section 9B answers:

    CAN THIS ITEM BE SOLD NOW,
    AT THIS BRANCH,
    AT THIS TIME,
    AT WHAT EFFECTIVE CATALOGUE PRICE,
    WITH WHAT PREPARATION REQUIREMENTS,
    AND WITH WHAT LOGISTICS REQUIREMENTS?

Section 9B turns the static food catalogue into an operational catalogue.

Section 9B owns:

1. menu schedules;
2. temporal availability evaluation;
3. sold-out / temporary operational availability;
4. branch item overrides;
5. price history and price-version integrity;
6. effective catalogue price calculation;
7. effective availability/sellability calculation;
8. preparation-time profiles;
9. logistics profiles;
10. optional inventory/ingredient availability integration seam;
11. catalogue revision/invalidation foundations.

Section 9B DOES NOT create customer-facing discovery.

That is Section 9C.

======================================================================
5. AUTHORITATIVE DOMAIN BOUNDARIES
======================================================================

Vendor Service remains authoritative for:

- food catalogue configuration
- current catalogue prices
- catalogue pricing history
- menu schedules
- branch item operational overrides
- preparation configuration
- food handling/logistics metadata
- effective catalogue pricing calculation
- effective catalogue sellability calculation
- availability configuration
- catalogue revision/change truth

Vendor Service is NOT authoritative for:

- customer identity
- customer PII
- cart
- checkout
- Orders
- order snapshots
- payments
- Wallet
- provider assignment
- delivery routing
- logistics OTP/QR
- actual inventory stock
- ingredient stock
- inventory reservations
- refunds
- promotions
- dynamic pricing
- customer discovery gateway

Do not access another service database.

Do not create cross-service database foreign keys.

Use opaque external references only.

======================================================================
6. EXISTING SECTION 9A MODEL MUST REMAIN VALID
======================================================================

Do not redesign Section 9A.

Build additively on:

Menu
  ↓
Section
  ↓
Item
  ├── Variants
  └── Modifier Groups
         ↓
      Modifiers

Preserve the accepted Section 9A hierarchy.

Any required changes to existing 9A mutation code must be surgical.

The most important required extension is:

PRICE MUTATIONS CREATED BY 9A ROUTES MUST NOW PARTICIPATE IN THE
SECTION 9B PRICE-HISTORY SYSTEM.

Do not create separate shadow pricing routes that allow price changes without history.

======================================================================
7. SECTION 9B LOGICAL ENTITIES
======================================================================

Implement the logical concepts required for Section 9B.

Expected logical entities include:

1. menu_schedules
2. branch_item_overrides
3. immutable price history/version records
4. food preparation profiles
5. food logistics profiles
6. item inventory/availability linkage configuration
7. catalogue revision/invalidation state

Physical table names should follow repository conventions.

Examples MAY include:

food_menu_schedules

food_branch_item_overrides

food_item_price_versions
food_variant_price_versions
food_modifier_price_versions
food_branch_item_price_versions

food_item_prep_profiles

food_item_logistics_profiles

food_item_inventory_links

food_catalogue_scope_revisions

These are conceptual names.

Do not blindly use them if existing naming patterns suggest better names.

Do not create speculative tables.

======================================================================
8. MIGRATION ALLOCATION
======================================================================

Determine the REAL current migration tip before creating files.

If the accepted tip remains:

000030_vendor_section9a_permissions

likely next migrations are:

000031_vendor_food_operations
000032_vendor_section9b_permissions

Use the next free numbers if newer migrations exist.

Do not renumber existing migrations.

The schema migration should contain Section 9B persistence.

The permission migration should contain new Section 9B IAM permissions and mappings if new permissions are actually necessary.

Migrations must:

- apply to the accepted Section 9A schema
- replay from a fresh database
- preserve Section 9A data
- preserve Sections 5–8
- avoid destructive schema changes
- include necessary constraints
- include necessary indexes
- support rollback according to repository conventions
- not touch other service schemas

======================================================================
9. MENU SCHEDULES
======================================================================

Implement menu scheduling.

Original requirement:

menu_schedules:
breakfast / lunch / dinner / seasonal availability

A menu may have zero or more schedules.

Schedules represent periods during which an ACTIVE menu is operationally available.

Do not confuse:

MENU ADMINISTRATIVE STATUS

with:

MENU SCHEDULE AVAILABILITY.

An ARCHIVED/PAUSED/DRAFT menu cannot become sellable merely because a schedule matches.

--------------------------------------------------
9.1 SCHEDULE MODEL
--------------------------------------------------

Inspect whether the repository already defines a schedule/time-window pattern.

Reuse it if appropriate.

Otherwise support a bounded model that can express:

- recurring weekly schedule
- breakfast
- lunch
- dinner
- custom time window
- seasonal/date-limited availability
- weekday selection
- local start time
- local end time
- optional effective start date
- optional effective end date
- lifecycle/archive state
- deterministic version
- label/type
- timestamps

Recommended closed schedule label vocabulary if no accepted one exists:

BREAKFAST
LUNCH
DINNER
SEASONAL
CUSTOM

The label is descriptive.

Availability is determined by actual configured time/date fields.

Do not hard-code arbitrary breakfast/lunch/dinner clock times.

--------------------------------------------------
9.2 TIMEZONE
--------------------------------------------------

Scheduling MUST use an authoritative timezone.

First inspect whether VendorBranch already has an IANA timezone.

If it does:

reuse it.

Do NOT duplicate timezone truth on every schedule.

If the branch does not currently have an authoritative timezone:

implement the smallest consistent Vendor-owned branch timezone mechanism necessary for schedule correctness.

Use:

IANA timezone identifiers

for example:

Africa/Lagos
Europe/Rome

Do NOT use only fixed UTC offsets.

Do NOT build a timezone service.

Invalid timezone identifiers must be rejected.

--------------------------------------------------
9.3 DST
--------------------------------------------------

Schedule evaluation must work across daylight-saving transitions.

Use Go's timezone database/time APIs.

Do not write manual DST arithmetic.

Evaluate the supplied instant by converting it into the branch timezone and comparing local:

- date
- weekday
- clock time

Tests must include a DST-observing timezone.

--------------------------------------------------
9.4 OVERNIGHT WINDOWS
--------------------------------------------------

Support reasonable restaurant schedules crossing midnight.

Example:

22:00 → 02:00

Do not silently interpret this as invalid unless the existing product specification explicitly prohibits overnight availability.

Define deterministic weekday semantics.

A recommended rule:

an overnight window belongs to the day on which the window starts.

Example:

Friday 22:00 → Saturday 02:00

is the Friday schedule window.

Test this.

--------------------------------------------------
9.5 NO-SCHEDULE BEHAVIOR
--------------------------------------------------

Define deterministic semantics.

Unless an existing authoritative specification says otherwise:

- ACTIVE menu
- zero active schedules

means:

NO SCHEDULE RESTRICTION / always schedule-eligible.

Once at least one active schedule exists:

the supplied time must match at least one active schedule.

Do not create hidden default time windows.

--------------------------------------------------
9.6 SCHEDULE VALIDATION
--------------------------------------------------

Validate:

- timezone
- weekday values
- local clock format
- date bounds
- effective_start_date <= effective_end_date
- non-empty applicable days where recurrence requires them
- lifecycle
- ownership
- menu status
- expected version

Overlapping schedules MAY represent a union of availability windows.

Do not introduce unnecessary conflict resolution unless the repository/master specification requires non-overlap.

======================================================================
10. TEMPORARY AVAILABILITY / SOLD-OUT STATE
======================================================================

Section 9B must support immediate operational availability changes.

At minimum support the product concepts:

AVAILABLE
SOLD_OUT
PAUSED / TEMPORARILY_UNAVAILABLE

First inspect whether exact vocabulary exists.

Use it if present.

If no accepted vocabulary exists, use a minimal closed operational state such as:

AVAILABLE
SOLD_OUT
PAUSED

This operational state is DIFFERENT from the Section 9A item administrative lifecycle.

An item cannot become sellable through operational AVAILABLE if its administrative state is:

DRAFT
PAUSED
ARCHIVED

Administrative safety always wins.

--------------------------------------------------
10.1 OPERATIONAL STATE DATA
--------------------------------------------------

Support:

- status
- optional bounded reason code/text according to repository convention
- optional effective_until / restore_at
- expected_version
- actor
- timestamps

Do not require an expiry for SOLD_OUT or PAUSED.

An indefinite sold-out state must be possible.

When effective_until passes:

the resolver must deterministically treat the temporary override as expired.

Whether a worker physically clears expired state is optional; correctness must not depend on a worker running exactly on time.

--------------------------------------------------
10.2 IMMEDIATE EFFECT
--------------------------------------------------

A successful sold-out/paused mutation must affect authoritative sellability immediately after transaction commit.

Do not use eventual network delivery as the only source of truth.

The same transaction must update the authoritative operational state and relevant catalogue revision/invalidation marker.

======================================================================
11. BRANCH ITEM OVERRIDES
======================================================================

Implement:

branch_item_overrides

Original Section 9 requirement:

branch price / availability / prep overrides

Preserve the accepted 9A branch/menu/item ownership model.

If the current 9A item is already tied to one branch, do NOT redesign the item into a global business item merely to justify this table.

The override remains an operational configuration layer for the owning branch.

Support independent optional override dimensions:

1. price
2. availability
3. prep profile

Do not force all override fields to be present simultaneously.

The API must support clearing an individual override and returning to inheritance.

--------------------------------------------------
11.1 PRICE OVERRIDE
--------------------------------------------------

Support an optional:

base_price_override_minor

or repository-equivalent representation.

NULL / absent means:

inherit the item's normal base price.

Use integer minor units.

Reject negative values.

Respect safe upper bounds.

--------------------------------------------------
11.2 AVAILABILITY OVERRIDE
--------------------------------------------------

Support the operational availability concepts from Section 10.

An availability override must NOT bypass:

- inactive business
- inactive branch
- missing/inactive branch capability
- inactive menu
- schedule restriction
- archived section
- administrative inactive item

--------------------------------------------------
11.3 PREP OVERRIDE
--------------------------------------------------

Allow the branch override layer to override the relevant preparation-time values.

NULL means:

inherit the item's default preparation profile.

Do not duplicate full unrelated profile data unnecessarily if field-level override semantics are sufficient.

======================================================================
12. PRICE HISTORY / VERSIONING
======================================================================

This is a CORE Section 9B requirement.

Original requirement:

Price changes must be versioned enough to preserve order snapshots.
Historical orders must never recompute from the current menu price.

Orders do not exist yet.

DO NOT implement Orders.

Instead implement immutable catalogue price history so future Order authority can capture exact price truth.

Price-bearing entities currently include:

- food_items.base_price_minor
- food_item_variants.price_override_minor
- food_modifiers.price_delta_minor

Section 9B introduces:

- branch item price override

Every price-bearing mutation must create immutable history/version evidence.

--------------------------------------------------
12.1 REQUIRED HISTORY
--------------------------------------------------

At minimum version:

ITEM BASE PRICE

VARIANT PRICE OVERRIDE

MODIFIER PRICE DELTA

BRANCH ITEM PRICE OVERRIDE

Do not allow a production mutation path to modify these fields without recording the corresponding price version/history.

--------------------------------------------------
12.2 IMMUTABILITY
--------------------------------------------------

Historical price records are append-only.

Do not UPDATE historical price rows to change old price truth.

Do not DELETE historical price truth through normal management APIs.

Corrections create a new version.

--------------------------------------------------
12.3 HISTORY FIELDS
--------------------------------------------------

Every history/version record should preserve enough information to identify:

- vendor business
- branch where applicable
- subject type
- subject reference
- previous/current pricing state as appropriate
- amount in integer minor units
- inheritance/null semantics
- currency context
- monotonic version/revision
- actor/source where safe
- recorded_at
- effective_at where needed
- correlation/idempotency context according to existing patterns

Use the repository's established audit/outbox correlation mechanisms where possible.

--------------------------------------------------
12.4 BASELINE BACKFILL
--------------------------------------------------

Existing Section 9A rows already contain prices.

Migration must establish a price-history baseline for existing 9A catalogue records.

DO NOT pretend to know historical changes that occurred before 9B.

Use explicit baseline semantics.

For example:

BASELINE_IMPORT

or repository-equivalent metadata.

The record should mean:

"This was the current known price when price-history authority was introduced."

Do not falsely claim it was the original creation price if that cannot be proven.

--------------------------------------------------
12.5 PRICE VERSION REFERENCES
--------------------------------------------------

Design the resolver so future Order snapshots can preserve exact price evidence.

A resolved price should expose enough immutable identifiers to later snapshot:

item_ref
item_version
item_price_version_ref

variant_ref
variant_version
variant_price_version_ref

modifier refs
modifier versions
modifier_price_version_refs

branch_override_ref/version where relevant

currency

catalogue/pricing revision

resolved_at

Do not create an Order table.

======================================================================
13. EFFECTIVE PRICE RESOLVER
======================================================================

Implement a deterministic application/domain-level effective-price resolver.

It must calculate the authoritative catalogue price for:

Business
+
Branch
+
Item
+
optional Variant
+
selected Modifiers
+
time/context where relevant

This is NOT Wallet pricing.

This is NOT checkout.

This is NOT payment execution.

This is catalogue price composition.

--------------------------------------------------
13.1 BASE PRICE PRECEDENCE
--------------------------------------------------

Use deterministic precedence.

Unless the current specification requires another rule, use:

IF selected variant has explicit absolute price override:
    selected variant price wins as the base unit price
ELSE IF branch item base-price override exists:
    branch override base price
ELSE:
    item base price

Then add valid selected modifier deltas.

This means:

a variant with NULL/inherit price inherits the branch-effective base price.

A variant with an explicit absolute price does not silently inherit the branch base override.

Document and test this exact behavior.

--------------------------------------------------
13.2 MODIFIER PRICE
--------------------------------------------------

Add selected modifiers' current price deltas.

Only modifiers:

- belonging to modifier groups attached to the item
- active/not archived
- valid under selection constraints

may participate.

Reject:

- foreign modifier
- modifier from unattached group
- duplicate modifier selection where duplicates are not allowed
- selection count below min_select
- selection count above max_select
- archived modifier
- archived group
- cross-tenant modifier

Use safe integer arithmetic.

Detect overflow.

The final catalogue price must never overflow or become negative.

--------------------------------------------------
13.3 REQUIRED MODIFIER GROUPS
--------------------------------------------------

When resolving a complete configured item price for future ordering purposes:

min_select > 0

must be enforced.

Do not price an invalid configured selection as though it were orderable.

If management requires a simple base-price preview separate from configured resolution, keep that function semantically distinct.

--------------------------------------------------
13.4 RESOLVER OUTPUT
--------------------------------------------------

Return a structured breakdown, not merely one number.

At minimum conceptually:

currency

item_ref

variant_ref if selected

base_source:
ITEM
BRANCH_OVERRIDE
VARIANT_OVERRIDE

base_price_minor

modifier components:
- modifier_ref
- price_delta_minor
- price_version_ref

modifier_total_minor

effective_unit_price_minor

item_price_version_ref

variant_price_version_ref where relevant

branch_override_price_version_ref where relevant

catalogue_revision

resolved_at

Do not expose internal database fields unnecessarily.

======================================================================
14. EFFECTIVE AVAILABILITY / SELLABILITY RESOLVER
======================================================================

Implement a deterministic effective-availability resolver.

Central question:

CAN BRANCH X SELL ITEM Y AT TIME T?

The resolver must fail closed.

Use the accepted authoritative layers in this order.

Conceptually:

1. Vendor business operational status
2. Branch operational status
3. Branch capability enabled
4. Capability is RESTAURANT / FAST_FOOD
5. Menu administrative state
6. Menu schedule
7. Menu section state
8. Item administrative state
9. Branch/temporary availability override
10. Optional inventory-linked availability
11. final sellability result

An override lower in the chain MUST NOT bypass a failure higher in the chain.

Examples:

- availability=AVAILABLE cannot resurrect an ARCHIVED item
- schedule cannot activate a PAUSED menu
- inventory says available cannot activate an inactive branch
- branch override cannot bypass missing capability

--------------------------------------------------
14.1 EXPLAINABLE RESULT
--------------------------------------------------

Return a structured result containing:

sellable: true/false

reason_code

optional reason details safe for management/API

evaluated_at

branch timezone

matched_schedule_ref if relevant

availability override ref/version if relevant

catalogue_revision

inventory availability status if linked

Do not make clients reproduce complex precedence logic.

--------------------------------------------------
14.2 REASON CODES
--------------------------------------------------

Use a bounded closed reason vocabulary.

Examples conceptually:

SELLABLE
BUSINESS_INACTIVE
BRANCH_INACTIVE
CAPABILITY_INACTIVE
MENU_NOT_ACTIVE
OUTSIDE_MENU_SCHEDULE
SECTION_INACTIVE
ITEM_NOT_ACTIVE
ITEM_SOLD_OUT
ITEM_PAUSED
INVENTORY_SOURCE_UNAVAILABLE
INVENTORY_NOT_AVAILABLE

Use repository naming conventions.

Do not use arbitrary reason text as authoritative program logic.

======================================================================
15. PREPARATION-TIME PROFILE
======================================================================

Implement item preparation profiles.

Original Section 9 requirement:

prep-time contributes ETA/provider dispatch timing

Vendor Service owns food preparation configuration.

Vendor Service does NOT own travel ETA or dispatch timing.

At minimum support a bounded preparation estimate.

Recommended minimal shape:

prep_min_minutes
prep_max_minutes

Rules:

prep_min_minutes >= 0

prep_max_minutes >= prep_min_minutes

Apply sane upper bounds appropriate for food/catalogue operations.

Do not allow absurd integer values.

A branch item override may override these values.

--------------------------------------------------
15.1 RESOLVED PREP PROFILE
--------------------------------------------------

Implement a resolver that returns:

effective prep_min_minutes
effective prep_max_minutes

source:
ITEM_DEFAULT
BRANCH_OVERRIDE

This becomes a future input into:

- order ETA
- kitchen timing
- delivery-provider timing

Do NOT implement those future systems here.

--------------------------------------------------
15.2 NO FAKE ETA
--------------------------------------------------

Do not calculate:

customer delivery ETA
travel time
driver arrival ETA
dispatch time

Section 9B provides preparation inputs only.

======================================================================
16. LOGISTICS PROFILE
======================================================================

Implement a Vendor-owned item logistics/handling profile.

Purpose:

describe the physical handling characteristics of the food item for future logistics selection.

This is METADATA / INPUT.

It does NOT assign a provider.

Do not call Dispatch, Taxi or Hauling.

--------------------------------------------------
16.1 LOGISTICS PROFILE CONTENT
--------------------------------------------------

Use bounded product-safe fields.

At minimum the design should support concepts such as:

- temperature / handling class
- spill risk
- fragility
- package size class
- optional approximate weight
- vehicle suitability hints

Use a controlled vocabulary.

Recommended concepts if none already exist:

temperature_class:
AMBIENT
HOT
COLD
FROZEN

package_size_class:
SMALL
MEDIUM
LARGE

boolean:
spill_risk
fragile

vehicle suitability HINTS may include platform vehicle classes if consistent with established logistics vocabulary.

For example:

MOTORCYCLE
CAR
VAN
TRUCK

These are suitability hints only.

Do not select a provider or service.

Reuse canonical platform vehicle/service codes if they already exist.

Do not create conflicting logistics vocabulary.

--------------------------------------------------
16.2 WEIGHT
--------------------------------------------------

If approximate weight is supported:

use integer grams or the platform's established canonical unit.

Do not use arbitrary mixed units.

Apply sane bounds.

--------------------------------------------------
16.3 LOGISTICS AUTHORITY
--------------------------------------------------

The logistics profile may later be consumed by:

Dispatch
Taxi
Hauling
Order fulfilment

but none of those integrations are implemented in 9B.

======================================================================
17. INVENTORY / INGREDIENT AVAILABILITY SEAM
======================================================================

Original Section 9 requirement:

ingredient-linked availability is optional per item but must be supported by design.

Inventory authority does NOT currently exist for this flow.

Do not create Inventory Service functionality inside Vendor Service.

Do not invent stock quantities.

Do not create stock decrement logic.

Do not fabricate ingredient availability.

--------------------------------------------------
17.1 MODES
--------------------------------------------------

Support a bounded availability-source mode.

Recommended:

MANUAL
INVENTORY_LINKED

MANUAL:

Section 9B operational availability controls the item.

INVENTORY_LINKED:

effective availability additionally depends on an external Inventory authority.

If the actual Inventory contract does not exist:

INVENTORY_LINKED MUST FAIL CLOSED.

Do not pretend the inventory source is available.

--------------------------------------------------
17.2 EXTERNAL REFERENCE
--------------------------------------------------

If configuration needs an external reference, store only an opaque inventory/recipe/catalogue reference.

Prefer a high-level reference such as:

inventory_recipe_ref

or repository-equivalent abstraction.

Do NOT model the external inventory database.

Do NOT cross-service FK.

--------------------------------------------------
17.3 SOURCE STATUS
--------------------------------------------------

Expose/document source support status.

Conceptually:

INVENTORY_AVAILABILITY_SUPPORT = NOT_PRESENT

until an actual authoritative publisher/service exists.

An INVENTORY_LINKED item with no available source should resolve:

sellable = false

reason:
INVENTORY_SOURCE_UNAVAILABLE

This is fail-closed behavior.

--------------------------------------------------
17.4 DO NOT INVENT CALLBACKS
--------------------------------------------------

Do not build a fake Inventory callback producer.

Do not add internal event ingestion unless an authoritative existing Inventory contract exists in the repository and is directly reusable.

Document the future dependency instead.

======================================================================
18. CATALOGUE REVISION / INVALIDATION FOUNDATION
======================================================================

Section 9B must establish an immediate authoritative change signal for data that affects future discovery.

Section 9C will build the actual discovery/consumption API.

Do NOT build customer discovery now.

But after any relevant committed change, future discovery must be able to determine that its previous view is stale.

--------------------------------------------------
18.1 REVISION MODEL
--------------------------------------------------

First inspect whether Vendor Service already has a suitable revision/generation/invalidation pattern.

Reuse it if available.

Otherwise implement a minimal monotonic catalogue revision scoped appropriately, for example:

business + branch + capability

A revision must increase transactionally when sellability/pricing/discovery-visible state changes.

Examples:

- menu activation/pause/archive
- schedule changes
- section/item administrative changes if not already covered
- item price changes
- variant price changes
- modifier price changes
- modifier attachment changes
- operational sold-out/pause/restore
- branch price override
- prep/logistics profile changes if discovery exposes them later
- inventory source mode changes

Do not create a cache in 9B merely to invalidate it.

The revision is authoritative invalidation/change evidence for 9C.

--------------------------------------------------
18.2 TRANSACTIONAL GUARANTEE
--------------------------------------------------

State mutation + revision increment + audit + required outbox must be atomic where applicable.

Do not commit state and then asynchronously attempt to bump a revision.

--------------------------------------------------
18.3 NO LOST UPDATES
--------------------------------------------------

Concurrent mutations must not produce duplicate/non-monotonic revision state.

Use database-safe increment semantics.

Test concurrency.

======================================================================
19. UPDATE EXISTING 9A MUTATION PATHS
======================================================================

Section 9B requires surgical changes to existing Section 9A price-changing operations.

Find every production path capable of changing:

food_items.base_price_minor

food_item_variants.price_override_minor

food_modifiers.price_delta_minor

Ensure each now:

1. validates as before;
2. performs optimistic concurrency as before;
3. appends immutable price history;
4. increments relevant catalogue revision;
5. writes audit;
6. writes outbox where required;
7. commits atomically.

Do not rewrite unrelated 9A logic.

Also ensure Section 9A lifecycle mutations that affect future discovery correctly bump the new catalogue revision where necessary.

Examples:

menu activate/pause/archive
section archive
item archive
modifier-group relation changes

Do this surgically.

======================================================================
20. HTTP MANAGEMENT API
======================================================================

Section 9B is primarily a vendor-management/operational API.

Use:

/api/v1/vendor/food/...

Do NOT create customer-facing discovery routes.

Do NOT create Section 9C's internal service-facing discovery contract.

Follow current Vendor route conventions.

The following are logical route families.

Adapt small route details to existing accepted conventions.

Do not add endpoints merely to reach a route-count target.

--------------------------------------------------
20.1 MENU SCHEDULES
--------------------------------------------------

POST
/api/v1/vendor/food/menus/{menuId}/schedules

GET
/api/v1/vendor/food/menus/{menuId}/schedules

GET
/api/v1/vendor/food/menus/{menuId}/schedules/{scheduleId}

PATCH
/api/v1/vendor/food/menus/{menuId}/schedules/{scheduleId}

POST
/api/v1/vendor/food/menus/{menuId}/schedules/{scheduleId}/archive

--------------------------------------------------
20.2 ITEM OPERATIONAL AVAILABILITY
--------------------------------------------------

Provide a clean management contract.

For example:

GET
/api/v1/vendor/food/items/{itemId}/availability

PUT/PATCH
/api/v1/vendor/food/items/{itemId}/availability

or semantically equivalent action routes.

The API must support:

- AVAILABLE
- SOLD_OUT
- PAUSED
- optional restore/effective-until

Do not create multiple redundant ways to mutate the same state.

--------------------------------------------------
20.3 BRANCH ITEM OVERRIDE
--------------------------------------------------

GET
/api/v1/vendor/food/items/{itemId}/branch-override

PUT/PATCH
/api/v1/vendor/food/items/{itemId}/branch-override

DELETE or explicit clear action
/api/v1/vendor/food/items/{itemId}/branch-override

If individual field clear semantics are needed, design them deterministically.

Do not use zero as an ambiguous "clear" value for price.

NULL / explicit clear semantics must distinguish:

price = 0

from:

inherit/no override.

--------------------------------------------------
20.4 PRICE HISTORY
--------------------------------------------------

Expose a bounded read-management view of price history.

Prefer an item-aggregate history endpoint rather than many unnecessary route families.

For example:

GET
/api/v1/vendor/food/items/{itemId}/pricing/history

It may include relevant:

- item price history
- variant history
- attached modifier history
- branch override price history

with bounded pagination/filtering.

Do not leak unrelated tenant data.

--------------------------------------------------
20.5 EFFECTIVE PRICE PREVIEW
--------------------------------------------------

Provide an authenticated Vendor management preview for the resolver.

For example:

POST
/api/v1/vendor/food/items/{itemId}/pricing/resolve

Input may include:

- variant_ref
- selected modifier_refs
- evaluation time if relevant

This is NOT customer checkout.

This is a Vendor management/resolution endpoint.

--------------------------------------------------
20.6 EFFECTIVE AVAILABILITY PREVIEW
--------------------------------------------------

Provide a management endpoint such as:

GET
/api/v1/vendor/food/items/{itemId}/availability/effective

with optional:

at=<timestamp>

or repository-equivalent request structure.

It must use the authoritative resolver.

--------------------------------------------------
20.7 PREP PROFILE
--------------------------------------------------

GET
/api/v1/vendor/food/items/{itemId}/prep-profile

PUT/PATCH
/api/v1/vendor/food/items/{itemId}/prep-profile

DELETE/clear if supported by semantics

--------------------------------------------------
20.8 LOGISTICS PROFILE
--------------------------------------------------

GET
/api/v1/vendor/food/items/{itemId}/logistics-profile

PUT/PATCH
/api/v1/vendor/food/items/{itemId}/logistics-profile

DELETE/clear where applicable

--------------------------------------------------
20.9 INVENTORY AVAILABILITY CONFIGURATION
--------------------------------------------------

GET
/api/v1/vendor/food/items/{itemId}/availability-source

PUT/PATCH
/api/v1/vendor/food/items/{itemId}/availability-source

DELETE/clear where appropriate

This configures the seam only.

It does not expose inventory stock.

--------------------------------------------------
20.10 INTERNAL ROUTES
--------------------------------------------------

Do not add an internal customer/discovery API in 9B.

The following belong to 9C:

/internal/vendor/v1/.../catalogue
/internal/vendor/v1/.../sellability
/internal/vendor/v1/.../pricing/resolve

unless the repository's accepted architecture has already explicitly assigned a necessary internal contract to 9B.

Default expected new 9B internal route count:

0

Do not force this number if a genuine accepted dependency requires otherwise.

======================================================================
21. IAM / AUTHORIZATION
======================================================================

Reuse Section 5 IAM.

Do not create a second permission engine.

Reuse Section 9A permissions where they correctly fit.

Add new permissions only where necessary to separate operational and pricing authority.

Use existing naming conventions.

Conceptually the policy should distinguish:

READ

CATALOGUE/OPERATIONS MANAGEMENT

PRICING MANAGEMENT

AVAILABILITY MANAGEMENT

Do not force literal permission names if repository convention differs.

--------------------------------------------------
21.1 EXPECTED ROLE INTENT
--------------------------------------------------

OWNER / ACCOUNT CONTROLLER

- full access

GENERAL MANAGER

- business-level management according to existing policy

BRANCH MANAGER

- manage Section 9B configuration only for authorized branches

KITCHEN / PASTRY OPERATIONAL ROLE

May manage, where role semantics allow:

- sold-out
- temporary availability
- preparation-time profile

Should NOT automatically receive broad business-wide pricing authority merely because it operates the kitchen.

MARKETING

No automatic price-management authority unless existing accepted role policy explicitly grants it.

FINANCE / SETTLEMENT

Do not assume food catalogue price-management authority merely because it is a finance role.

ANALYST / AUDITOR

- read-only

OTHER ROLES

- deny by default unless explicitly mapped.

--------------------------------------------------
21.2 SHARED DEFINITIONS
--------------------------------------------------

Section 9A modifier groups are business-level reusable definitions.

Preserve Section 9A's protection against branch actors mutating shared definitions outside their authority.

======================================================================
22. STEP-UP AUTH
======================================================================

Do not require step-up for routine:

- sold-out change
- schedule edit
- prep profile edit
- ordinary menu price edit

unless existing IAM policy explicitly requires it.

Do not weaken step-up for existing sensitive operations.

======================================================================
23. MONEY / CURRENCY
======================================================================

All price values remain integer minor units.

NEVER use:

float32
float64

for monetary values.

Reuse the authoritative Vendor business currency if available.

Do not build:

- FX
- currency conversion
- dynamic pricing
- taxes
- delivery fees
- payment execution

Use overflow-safe arithmetic.

======================================================================
24. BRANCH PRICE RESOLUTION RULE
======================================================================

The price precedence must be deterministic.

Unless an existing authoritative product rule overrides it:

selected variant has explicit price
        ↓
variant absolute price is base

otherwise

branch item base-price override
        ↓
branch override is base

otherwise

item base price
        ↓
item base is base

Then:

+ valid modifier price deltas

Examples:

Item base:
₦5,000

Branch override:
₦5,500

Variant A:
NULL / inherit

Effective base for Variant A:
₦5,500

Variant B:
₦7,000 explicit

Effective base for Variant B:
₦7,000

Do not add the branch override on top of an explicit variant absolute price.

Test this explicitly.

======================================================================
25. SCHEDULE / AVAILABILITY PRECEDENCE
======================================================================

The effective availability evaluation must be deterministic.

A recommended evaluation sequence:

BUSINESS
    ↓
BRANCH
    ↓
CAPABILITY
    ↓
MENU ADMIN STATUS
    ↓
MENU SCHEDULE
    ↓
SECTION STATUS
    ↓
ITEM ADMIN STATUS
    ↓
TEMPORARY/BRANCH AVAILABILITY
    ↓
INVENTORY SOURCE IF LINKED
    ↓
SELLABLE

A lower layer can deny availability.

A lower layer cannot resurrect an upper-layer denial.

Test every important precedence pair.

======================================================================
26. ARCHIVE / HISTORY SAFETY
======================================================================

Do not physically destroy price history.

Do not physically destroy configuration required for future historical order evidence.

Archiving should leave historical identifiers resolvable.

Branch override clearing must preserve price-history evidence.

Profile replacement should preserve audit/history according to repository conventions.

======================================================================
27. IDEMPOTENCY
======================================================================

Reuse Vendor Service's accepted idempotency architecture.

Do not create another idempotency store.

Appropriate mutation routes must be retry-safe.

Examples:

- create schedule
- update availability
- update branch override
- set prep profile
- set logistics profile
- set inventory source

Idempotent retries must not create duplicate:

- schedule rows
- history versions
- outbox events
- audit mutations
- catalogue revision increments

when the established idempotency contract indicates the operation is the same retry.

======================================================================
28. OPTIMISTIC CONCURRENCY
======================================================================

Use the accepted expected-version model.

Test stale mutation handling for:

- schedule edit
- branch override
- availability
- prep profile
- logistics profile
- inventory source config
- existing 9A price edits after history integration

Do not silently overwrite newer changes.

======================================================================
29. TRANSACTIONS
======================================================================

Mutations must keep related authoritative writes atomic.

For a price change, conceptually:

BEGIN
  lock/validate target
  update current price
  insert immutable price version
  write audit
  append outbox
  bump catalogue revision
COMMIT

Do not perform a synchronous external network call while holding important database locks.

Follow the accepted Section 8I principle:

DB mutation
+
durable evidence
+
commit
before external network work.

Section 9B should ideally require no synchronous external network call for the core domain.

======================================================================
30. AUDIT
======================================================================

Audit every meaningful Section 9B mutation.

Examples:

- schedule create/update/archive
- availability change
- sold-out
- restore
- branch price override
- clear override
- prep override
- base prep profile change
- logistics profile change
- inventory source mode change
- price mutation
- price baseline establishment where appropriate

Audit must preserve safe metadata.

Do not log:

- secrets
- customer PII
- raw external credentials

New 9B code should use transactional audit where current infrastructure supports it.

======================================================================
31. TRANSACTIONAL OUTBOX
======================================================================

Use the accepted Vendor outbox.

Relevant mutations should produce durable events appropriate to repository conventions.

Conceptually:

food.menu.schedule.changed
food.item.availability.changed
food.item.price.changed
food.variant.price.changed
food.modifier.price.changed
food.item.branch_override.changed
food.item.prep_profile.changed
food.item.logistics_profile.changed
food.item.availability_source.changed
food.catalogue.revision.changed

Do not create event spam if aggregate-level events are preferred.

Use stable versioned payloads.

State + outbox must commit together.

Do not synchronously call future discovery services.

======================================================================
32. CATALOGUE REVISION ATOMICITY
======================================================================

The revision mechanism must be safe under concurrency.

Test:

N simultaneous relevant mutations

must result in deterministic monotonic revision advancement.

Do not implement:

read current revision
+ 1 in application memory
write

without proper locking/atomic SQL.

Use atomic database increment or equivalent safe semantics.

======================================================================
33. DOMAIN VALIDATION
======================================================================

Apply bounded validation.

At minimum:

SCHEDULE
- valid timezone
- valid days
- valid local times
- valid date range
- valid lifecycle

AVAILABILITY
- valid operational status
- valid restore time
- bounded reason

PRICE
- integer minor units
- >= 0
- safe max
- overflow-safe resolution

PREP
- non-negative
- max >= min
- sane upper bounds

LOGISTICS
- valid enums
- bounded approximate weight
- no arbitrary unsupported vehicle code

INVENTORY SOURCE
- valid mode
- required external reference where mode requires it
- opaque reference bounded
- MANUAL does not require external inventory ref

ALL
- correct business
- correct branch
- correct parent
- not archived where mutation invalid
- expected version
- no cross-tenant IDs

======================================================================
34. SECURITY / IDOR
======================================================================

Add Section 9B security tests.

Must cover:

- missing authentication
- wrong tenant
- wrong branch
- cross-menu schedule access
- cross-item override access
- cross-item profile access
- cross-business inventory ref configuration where validation exists
- branch manager outside branch
- kitchen role attempting unauthorized price mutation
- analyst attempting write
- stale expected_version
- malformed IDs
- oversized payloads
- invalid enum values
- archived item operations
- archived menu schedule operations

Do not leak another tenant's object existence unnecessarily.

Follow existing not-found/forbidden behavior.

======================================================================
35. PRICE HISTORY SECURITY / INTEGRITY
======================================================================

Ensure price history cannot be used as an unrestricted cross-tenant query surface.

Every history read must scope by:

business
+
branch/item ownership

Do not expose internal actor secrets.

Price history must be read-only through ordinary APIs.

No update/delete route for historical price versions.

======================================================================
36. EFFECTIVE RESOLUTION MUST USE DATABASE TRUTH
======================================================================

Do not resolve current price or availability using stale duplicated request values.

Load authoritative current catalogue state.

Do not trust client-supplied:

- current item price
- current variant price
- current modifier delta
- current lifecycle
- current branch state
- current schedule

The client selects references/context.

The Vendor Service resolves authoritative values.

======================================================================
37. NO CUSTOMER DISCOVERY
======================================================================

Section 9B must NOT implement:

- unauthenticated Food browsing
- customer catalogue browsing
- customer app integration
- public discovery search
- customer search indexing
- API Gateway food browse route
- recommendation systems
- location discovery
- restaurant ranking

Section 9C owns safe service-facing/discovery consumption.

======================================================================
38. NO ORDER SYSTEM
======================================================================

Do not create:

orders
order_items
order_modifiers
restaurant order acceptance
cart
checkout
order status
refund logic
order cancellation

Price history exists so future Orders can snapshot immutable pricing.

Do not fabricate that future authority.

When documentation needs to describe it, use:

PENDING_EXTERNAL_DOMAIN — ORDER AUTHORITY

where applicable.

======================================================================
39. NO INVENTORY AUTHORITY
======================================================================

Do not create:

inventory stock tables
ingredient stock levels
stock decrement
stock reservation
low-stock worker
stock reconciliation
expiry stock system

The Section 9B inventory feature is ONLY the availability integration seam.

If no authoritative Inventory provider exists:

INVENTORY_AVAILABILITY_SUPPORT = NOT_PRESENT

and linked mode fails closed.

======================================================================
40. NO PAYMENT / WALLET
======================================================================

Do not call Wallet for catalogue price configuration or price resolution.

Do not implement:

- payment
- escrow
- commission
- payout
- wallet balance
- settlement
- vendor earnings

Catalogue price is commercial configuration.

Wallet remains monetary authority.

======================================================================
41. NO LOGISTICS EXECUTION
======================================================================

Do not:

- choose actual driver/rider
- dispatch provider
- calculate travel route
- call Dispatch
- call Taxi
- call Hauling
- create delivery mission
- calculate delivery fee
- generate logistics OTP/QR

Logistics profile = input metadata only.

======================================================================
42. NO PROMOTIONS / DYNAMIC PRICING
======================================================================

Do not implement:

- coupons
- discount engine
- scheduled promotions
- surge pricing
- AI pricing
- campaign prices
- BOGO
- delivery promotions

Modifier deltas remain non-negative unless an accepted specification explicitly changed this.

======================================================================
43. PRICE SNAPSHOT COMPATIBILITY
======================================================================

Future Order authority must be capable of recording something equivalent to:

item_ref
item_version
item_price_version_ref

variant_ref
variant_version
variant_price_version_ref

selected modifiers:
  modifier_ref
  modifier_version
  modifier_price_version_ref
  price_delta_minor

branch_override_ref/version

base_price_minor
modifier_total_minor
effective_unit_price_minor

currency

catalogue_revision
resolved_at
captured_at

Section 9B should make this possible.

Do not persist Order snapshots in Vendor Service.

======================================================================
44. PREP / LOGISTICS RESOLVER BOUNDARY
======================================================================

The effective operational resolver may provide:

effective prep range
logistics metadata
sellability
price

But do NOT combine these into a customer delivery promise.

For example:

allowed:
prep_min_minutes = 15
prep_max_minutes = 25

not allowed:
customer_delivery_eta = 37 minutes

Travel/dispatch information is outside Vendor authority.

======================================================================
45. FEATURE FLAGS
======================================================================

Inspect existing Vendor rollout/feature-flag architecture.

Add Section 9B gating only if consistent with established design.

Conceptually:

VENDOR_FOOD_OPERATIONS_ENABLED

or extensions to the accepted food feature flag.

Do not create environment-dependent route registration if this breaks route/OpenAPI parity.

Prefer stable route registration with behavior gating where this is the current architecture.

Document activation.

======================================================================
46. HTTP ERROR MODEL
======================================================================

Reuse established Vendor errors.

Add bounded Section 9B domain errors only as necessary.

Conceptual examples:

MENU_SCHEDULE_NOT_FOUND
INVALID_MENU_SCHEDULE
OUTSIDE_MENU_SCHEDULE

INVALID_AVAILABILITY_STATE

INVALID_PRICE_OVERRIDE
PRICE_HISTORY_NOT_FOUND
INVALID_PRICE_SELECTION
INVALID_MODIFIER_SELECTION

PREP_PROFILE_NOT_FOUND
INVALID_PREP_PROFILE

LOGISTICS_PROFILE_NOT_FOUND
INVALID_LOGISTICS_PROFILE

INVENTORY_SOURCE_UNAVAILABLE

STALE_VERSION
INVALID_REORDER
CAPABILITY_INACTIVE

Do not create inconsistent status mappings.

======================================================================
47. REPOSITORY / SERVICE ARCHITECTURE
======================================================================

Follow existing Vendor feature package patterns.

Do not place all logic in HTTP handlers.

The complex resolver logic must live in domain/application/service code where it can be directly tested.

Repository queries must be tenant-aware.

Avoid N+1 query behavior when resolving:

item
+ variant
+ modifier groups
+ selected modifiers
+ branch override
+ schedule
+ profiles

Use bounded/selective loading.

Do not refactor unrelated Vendor packages.

======================================================================
48. TESTING STRATEGY
======================================================================

Use targeted development tests first.

Do NOT repeatedly run:

go test ./...

throughout implementation.

Suggested sequence:

1. migration tests
2. schedule tests
3. pricing/history tests
4. availability tests
5. profile tests
6. IAM/security tests
7. concurrency tests
8. HTTP contract tests
9. final Section 9B service gate

Use real PostgreSQL/Redis where existing tests require them.

Use deterministic concurrency tests.

Do not use flaky long sleeps.

======================================================================
49. REQUIRED TEST COVERAGE — MIGRATIONS
======================================================================

Test:

- migration applies from accepted 9A tip
- tables exist
- constraints exist
- indexes exist
- permission seeds apply
- baseline price history generated correctly
- fresh migration replay
- rollback/reapply according to repository conventions
- Sections 5–9A remain intact

======================================================================
50. REQUIRED TEST COVERAGE — SCHEDULES
======================================================================

Test:

- create
- list
- get
- update
- archive
- expected version
- wrong menu
- wrong branch
- wrong tenant
- invalid timezone
- invalid weekday
- invalid date range
- breakfast-style window
- lunch-style window
- dinner-style window
- seasonal/date-limited window
- CUSTOM
- no schedules => unrestricted schedule eligibility
- schedules exist => one must match
- overlapping schedules behave deterministically
- overnight 22:00 → 02:00
- weekday rollover semantics
- DST-observing timezone
- archived schedule ignored

======================================================================
51. REQUIRED TEST COVERAGE — OPERATIONAL AVAILABILITY
======================================================================

Test:

- default available/inherit state
- mark SOLD_OUT
- sellability becomes false immediately after commit
- restore AVAILABLE
- PAUSED
- optional restore time
- expired temporary state
- invalid status
- stale version
- archived item
- wrong branch
- wrong tenant
- idempotent retry

======================================================================
52. REQUIRED TEST COVERAGE — BRANCH OVERRIDES
======================================================================

Test:

- create/set override
- read
- update
- clear individual price override
- clear availability override
- clear prep override
- delete/clear aggregate override where supported
- zero price is distinguishable from no override
- inherited price
- explicit branch price
- branch ownership
- stale version
- archive safety
- concurrency

======================================================================
53. REQUIRED TEST COVERAGE — PRICE HISTORY
======================================================================

Test:

- baseline records created from 9A prices
- baseline records identified as baseline, not fabricated historical change
- item base price mutation appends history
- variant override mutation appends history
- variant clear-to-inherit appends history
- modifier delta mutation appends history
- branch price override appends history
- branch override clear appends history
- old version remains immutable
- no update/delete route for history
- idempotent retry does not double-record same mutation
- rollback does not leave orphan history
- cross-tenant history access refused
- pagination/bounds
- correct currency
- correct version references

======================================================================
54. REQUIRED TEST COVERAGE — EFFECTIVE PRICE
======================================================================

Test:

ITEM BASE only

ITEM BASE + modifiers

BRANCH OVERRIDE + inherited variant

EXPLICIT VARIANT OVERRIDE beats branch base override

VARIANT + modifiers

zero-priced item

zero-priced modifier

required modifier group missing => reject

too few selections => reject

too many selections => reject

foreign modifier => reject

modifier from unattached group => reject

duplicate modifier => reject

archived modifier => reject

archived group => reject

cross-tenant modifier => reject

integer overflow => reject safely

structured version references returned

catalogue revision returned

currency returned

======================================================================
55. REQUIRED TEST COVERAGE — EFFECTIVE AVAILABILITY
======================================================================

Test each layer:

business inactive
branch inactive
capability inactive/missing
menu DRAFT
menu PAUSED
menu ARCHIVED
outside menu schedule
inside menu schedule
section archived
item DRAFT
item PAUSED
item ARCHIVED
item SOLD_OUT
item operational PAUSED
item AVAILABLE
temporary state expired
inventory-linked with source absent
manual availability

Also test precedence:

AVAILABLE override cannot resurrect archived item.

Inventory available simulation, if a test seam exists, cannot resurrect inactive menu.

Schedule match cannot resurrect inactive branch.

Final reason code must identify the relevant denial.

======================================================================
56. REQUIRED TEST COVERAGE — PREP PROFILES
======================================================================

Test:

- create/set
- get
- update
- clear
- min = 0 if allowed
- max >= min
- max < min rejected
- negative rejected
- excessive values rejected
- branch override precedence
- inheritance after clear
- stale version
- wrong tenant
- wrong branch

======================================================================
57. REQUIRED TEST COVERAGE — LOGISTICS PROFILES
======================================================================

Test:

- create/set
- get
- update
- clear
- valid temperature classes
- invalid class rejected
- valid package size
- invalid size rejected
- spill risk
- fragility
- approximate weight bounds
- vehicle suitability codes
- unknown vehicle code rejected
- wrong tenant
- stale version

Assert no Dispatch/Taxi/Hauling call is made.

======================================================================
58. REQUIRED TEST COVERAGE — INVENTORY SEAM
======================================================================

Test:

MANUAL mode

INVENTORY_LINKED mode

linked mode requires the appropriate opaque source reference if the design requires it

invalid mode rejected

source not configured/present:
sellability false
reason INVENTORY_SOURCE_UNAVAILABLE

MANUAL mode:
does not depend on inventory

No inventory stock table is created.

No fake inventory callback is exposed.

======================================================================
59. REQUIRED TEST COVERAGE — IAM
======================================================================

Test real permission seed.

At minimum:

Owner:
allowed

General Manager:
allowed according to scope

Branch Manager:
allowed within branch

Branch Manager:
denied outside branch

Kitchen operational role:
allowed sold-out/prep according to policy

Kitchen operational role:
denied pricing where policy says pricing is restricted

Analyst/Auditor:
read only

Unauthorized:
denied

Missing auth:
denied

Do not weaken Section 9A IAM.

======================================================================
60. REQUIRED TEST COVERAGE — IDOR
======================================================================

Test:

schedule through foreign menu

availability through foreign item

branch override through foreign item

prep profile through foreign item

logistics profile through foreign item

pricing history through foreign item

price resolver with foreign variant

price resolver with foreign modifier

cross-business relationship

Cross-tenant targets must not leak data.

======================================================================
61. REQUIRED TEST COVERAGE — CONCURRENCY
======================================================================

Use deterministic concurrency tests.

At minimum:

simultaneous item price edits
- one deterministic winner under expected-version semantics

simultaneous branch override edits

simultaneous sold-out/restore race

simultaneous schedule edit

concurrent catalogue revision bumps

idempotent same-key retries

price history:
no duplicate version for one successful mutation

rollback:
no orphan history/outbox/revision

Avoid sleep-based race tests where possible.

======================================================================
62. REQUIRED TEST COVERAGE — AUDIT / OUTBOX / REVISION
======================================================================

For representative mutations verify:

STATE
+
PRICE HISTORY where applicable
+
AUDIT
+
OUTBOX
+
CATALOGUE REVISION

commit together.

On forced rollback:

none should survive partially.

Verify relevant mutation increments revision exactly according to the chosen semantics.

======================================================================
63. PERFORMANCE / QUERY QUALITY
======================================================================

Do not spend the task on broad benchmarking.

Create a moderate realistic fixture.

For example:

- several menus
- many schedules
- dozens of sections
- hundreds of items
- variants
- modifier groups/modifiers
- branch overrides
- profiles

Test resolver/query behavior for obvious N+1 problems.

Effective pricing should not execute one query per selected modifier where a set-based query can be used.

Effective availability should not repeatedly reload the same parent graph unnecessarily.

Inspect important query/index relationships.

======================================================================
64. OPENAPI
======================================================================

After implementation stabilizes:

create/update the Section 9B OpenAPI specification according to current project structure.

Expected file name if consistent with 9A:

services/vendor-service/docs/openapi/section9b-food-operations.openapi.json

Use the actual documentation convention from the repository.

Document:

- all 9B routes
- authentication
- permissions
- schedule enums
- availability enums
- timezone format
- expected-version behavior
- idempotency
- integer minor-unit prices
- branch override null/inheritance semantics
- price-resolution request
- price-resolution breakdown
- availability-resolution reason codes
- prep profile
- logistics profile
- inventory support status
- errors

Run route/OpenAPI parity.

Do not document routes that do not exist.

Do not leave production routes undocumented.

======================================================================
65. GLOBAL ROUTE CONTRACT
======================================================================

Section 8I introduced the global production-router contract guard.

Section 9A extended it.

Preserve it.

Update the route inventory/spec registration as necessary so the entire Vendor route surface remains bidirectionally certified.

Derive:

PRE_9B_ROUTE_COUNT

NEW_9B_PUBLIC_ROUTES

NEW_9B_INTERNAL_ROUTES

POST_9B_ROUTE_COUNT

from the production router.

Do not calculate from comments.

Do not modify tests merely to hide route drift.

======================================================================
66. API ROUTE REGISTER
======================================================================

Update:

services/vendor-service/docs/API_ROUTE_REGISTER.md

Record every actual Section 9B route.

Use consistent canonical path syntax.

Record:

- method
- path
- public/internal
- authentication
- permission
- section ownership

Do not mark 9C routes as present.

======================================================================
67. IMPLEMENTATION STATUS
======================================================================

Update:

services/vendor-service/docs/IMPLEMENTATION_STATUS.md

Expected state after successful implementation:

SECTION 9A — IMPLEMENTED + TESTED

SECTION 9B — IMPLEMENTED + TESTED

SECTION 9C — NOT STARTED

SECTION 9 — NOT YET FINAL-CERTIFIED

Do not mark all of Section 9 complete before 9C.

======================================================================
68. PLATFORM SERVICE CHANGE REGISTER
======================================================================

Update:

services/vendor-service/docs/PLATFORM_SERVICE_CHANGE_REGISTER.md

Record only actual cross-service effects.

Expected:

Media:
no new change unless genuinely required

Wallet:
none

Customer:
none

Order:
NOT_PRESENT / future dependency

Inventory:
NOT_PRESENT for authoritative availability

Dispatch:
none

Taxi:
none

Hauling:
none

Document:

INVENTORY_AVAILABILITY_SUPPORT = NOT_PRESENT

if that is still the real repository state.

Do not fabricate integrations.

======================================================================
69. README / ARCHITECTURE
======================================================================

Update relevant Vendor docs after code stabilizes.

Document:

- 9A vs 9B boundary
- schedule model
- timezone semantics
- overnight windows
- price history
- baseline import semantics
- price precedence
- modifier resolution
- effective availability precedence
- branch overrides
- prep profile
- logistics profile
- inventory fail-closed seam
- catalogue revision/invalidation foundation
- 9C deferral

Avoid duplicating the same material excessively.

======================================================================
70. UI / FRONTEND HANDOFF DOCS
======================================================================

Backend only.

DO NOT implement frontend.

Update:

services/vendor-service/docs/UI_UX_REQUIREMENTS.md

services/vendor-service/docs/FRONTEND_REQUIREMENTS.md

where necessary.

Document backend-derived UI requirements such as:

- schedule editor
- timezone awareness
- overnight schedule representation
- sold-out toggle/state
- optional restore time
- branch price override
- clear/inherit semantics
- price history view
- prep-time editor
- logistics handling metadata
- inventory-linked unavailable state
- effective-price preview
- effective-sellability reason display
- concurrency conflict behavior

Do not design/build actual screens.

======================================================================
71. DEVELOPMENT ORDER
======================================================================

Follow this order.

Do not spend excessive time planning.

PHASE A — FAST BASELINE

- branch/status/HEAD
- real route count
- real migration tip
- locate accepted 9A feature package
- locate audit/outbox/IAM/idempotency patterns
- locate route/OpenAPI global contract integration

Then begin coding.

PHASE B — DATABASE FOUNDATION

Implement:

- menu schedules
- branch item overrides
- immutable price history
- prep profiles
- logistics profiles
- inventory availability configuration
- catalogue revisions
- permissions if required

Run migration tests.

PHASE C — PRICE HISTORY INTEGRATION

Surgically update existing 9A:

- item price mutation
- variant price mutation
- modifier price mutation

to append immutable history + revision atomically.

Run targeted price-history tests.

PHASE D — SCHEDULING

Implement:

- schedule CRUD
- timezone resolution
- weekly/date windows
- overnight windows
- DST-safe evaluation

Run schedule tests.

PHASE E — AVAILABILITY + BRANCH OVERRIDES

Implement:

- operational SOLD_OUT/PAUSED/AVAILABLE
- restore semantics
- branch price override
- prep override
- fail-safe precedence

Run targeted tests.

PHASE F — EFFECTIVE PRICE RESOLVER

Implement:

- item
- variant
- modifiers
- precedence
- constraints
- overflow safety
- immutable version references

Run pricing tests.

PHASE G — EFFECTIVE AVAILABILITY RESOLVER

Implement the full precedence chain and explainable reason codes.

Run availability tests.

PHASE H — PREP / LOGISTICS / INVENTORY SEAM

Implement profiles and fail-closed inventory linkage.

Run targeted tests.

PHASE I — HTTP + IAM

Wire management routes.

Do not start 9C internal discovery routes.

PHASE J — SECURITY / CONCURRENCY

Run:

- IAM
- IDOR
- stale version
- concurrency
- revision
- transaction rollback tests

PHASE K — DOCS / OPENAPI

Update after API stabilizes.

PHASE L — FINAL SECTION 9B GATE

Run final required 9B validation once.

PHASE M — REPORT

Return the required report.

======================================================================
72. FINAL BUILD / QUALITY GATE
======================================================================

After implementation stabilizes run appropriate commands for:

gofmt

go vet

go build

git diff --check

Run Section 9B tests against the real PostgreSQL/Redis test infrastructure as required.

Run route/OpenAPI parity.

Run migration/replay tests.

Do not run unrelated enormous repository suites unless Section 9B changed shared core code requiring them.

If a broader Vendor Service regression is appropriate, run it ONCE at the end.

Do not repeatedly burn usage on already-green CRM certification.

======================================================================
73. THINGS YOU MUST NOT DO
======================================================================

DO NOT:

- implement frontend
- start Section 9C
- start Section 10
- redesign Section 9A
- create Orders
- create carts
- create checkout
- create payments
- create Wallet logic
- create Inventory stock authority
- create ingredient stock authority
- decrement stock
- reserve stock
- dispatch riders
- call Taxi
- call Hauling
- generate logistics OTP/QR
- calculate travel ETA
- calculate delivery fee
- create dynamic pricing
- create promotions
- create coupons
- create recommendation AI
- create search indexing
- expose customer discovery
- create customer PII
- access another service database
- use floating-point money
- use fixed UTC offset as schedule timezone authority
- use synchronous external network calls while holding critical DB locks
- overwrite immutable price history
- bypass existing IAM
- introduce unnecessary dependencies
- create branches/worktrees
- commit
- stage
- merge
- push
- stash/reset/clean
- broadly refactor unrelated code
- perform endless repository audits
- repeatedly run the full test suite

======================================================================
74. SECTION 9B ACCEPTANCE CRITERIA
======================================================================

Section 9B is GO only if all of these are satisfied:

1. Menu schedules implemented.
2. Weekly availability supported.
3. Breakfast/lunch/dinner labels supported.
4. Seasonal/date-limited availability supported.
5. Custom schedule supported where required.
6. IANA timezone handling implemented.
7. DST-safe evaluation implemented.
8. Overnight windows handled deterministically.
9. Operational SOLD_OUT supported.
10. Operational PAUSED/temporary unavailable supported.
11. AVAILABLE/restore supported.
12. Optional restore/effective-until supported.
13. Branch item overrides implemented.
14. Branch price override implemented.
15. Branch availability override implemented.
16. Branch prep override implemented.
17. Clear/inherit semantics implemented.
18. Immutable item price history implemented.
19. Immutable variant price history implemented.
20. Immutable modifier price history implemented.
21. Immutable branch override price history implemented.
22. Existing 9A price mutation paths write history.
23. Existing 9A price rows receive explicit baseline history.
24. Historical records are append-only.
25. Effective price resolver implemented.
26. Variant/branch price precedence deterministic.
27. Modifier selection constraints enforced.
28. Effective price uses integer minor units.
29. Overflow handled safely.
30. Price version references returned.
31. Effective availability resolver implemented.
32. Business state included.
33. Branch state included.
34. Capability state included.
35. Menu administrative state included.
36. Menu schedule included.
37. Section state included.
38. Item administrative state included.
39. Operational availability included.
40. Inventory-linked mode included fail-closed.
41. Explainable availability reason codes implemented.
42. Prep profiles implemented.
43. Branch prep inheritance/override implemented.
44. Logistics profiles implemented.
45. Logistics metadata remains non-authoritative hints.
46. Inventory seam implemented without stock authority.
47. Inventory source absence fails closed.
48. Catalogue revision/invalidation foundation implemented.
49. Relevant state changes bump revision transactionally.
50. Audit integrated.
51. Outbox integrated.
52. Idempotency integrated.
53. Optimistic concurrency integrated.
54. Tenant isolation enforced.
55. Branch isolation enforced.
56. Nested-IDOR prevented.
57. IAM role matrix enforced.
58. Kitchen role does not accidentally obtain unrestricted pricing power.
59. Analyst/Auditor remains read-only.
60. Migrations apply from 9A.
61. Fresh migration replay succeeds.
62. Required constraints/indexes present.
63. Schedule tests pass.
64. DST tests pass.
65. Overnight schedule tests pass.
66. Price-history tests pass.
67. Effective-price tests pass.
68. Availability tests pass.
69. Prep-profile tests pass.
70. Logistics-profile tests pass.
71. Inventory fail-closed tests pass.
72. Security tests pass.
73. IAM tests pass.
74. Concurrency tests pass.
75. Transaction atomicity tests pass.
76. Catalogue revision tests pass.
77. HTTP tests pass.
78. OpenAPI parity passes.
79. Global route parity remains green.
80. gofmt clean.
81. go vet passes.
82. go build passes.
83. git diff --check passes.
84. API_ROUTE_REGISTER updated.
85. IMPLEMENTATION_STATUS updated.
86. PLATFORM_SERVICE_CHANGE_REGISTER updated.
87. relevant README/architecture docs updated.
88. UI_UX_REQUIREMENTS updated where required.
89. FRONTEND_REQUIREMENTS updated where required.
90. No frontend code touched.
91. No Order authority fabricated.
92. No Inventory authority fabricated.
93. No Wallet/payment authority fabricated.
94. No logistics execution added.
95. No customer discovery added.
96. Section 9C remains NOT STARTED.
97. Work remains uncommitted.
98. Work remains unstaged.
99. No branch/worktree created.
100. No push performed.

======================================================================
75. FINAL ROUTE / MIGRATION VERIFICATION
======================================================================

At completion derive actual values.

Report:

PRE_9B_ROUTE_COUNT

NEW_9B_PUBLIC_ROUTES

NEW_9B_INTERNAL_ROUTES

POST_9B_ROUTE_COUNT

Report:

PRE_9B_MIGRATION_TIP

NEW_9B_MIGRATIONS

POST_9B_MIGRATION_TIP

Also report:

new tables

price history tables

schedule table

override table

profile tables

inventory seam table/config

catalogue revision mechanism

Do not guess counts.

======================================================================
76. FINAL REPORT FORMAT
======================================================================

Return this exact concise structure:

# SECTION 9B FINAL IMPLEMENTATION REPORT

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

## BASELINE

Pre-9B routes:
Pre-9B migration tip:

Accepted 9A status:

## DATABASE

New migrations:
Post-9B migration tip:

New tables:

Price-history model:
Catalogue-revision model:

## SCHEDULING

Menu schedules:
Timezone:
DST:
Overnight windows:
Seasonal/date bounds:
No-schedule semantics:

## AVAILABILITY

Operational states:
Temporary restore:
Branch availability override:
Effective availability resolver:
Reason codes:
Immediate revision/invalidation:

## PRICING

Item price history:
Variant price history:
Modifier price history:
Branch override price history:
Baseline import:
Effective price resolver:
Price precedence:
Modifier validation:
Version references:
Currency/minor-unit handling:

## OPERATIONAL PROFILES

Prep profile:
Branch prep override:
Logistics profile:
Inventory/ingredient seam:

INVENTORY_AVAILABILITY_SUPPORT:

## IAM / SECURITY

Permissions:
Owner:
General Manager:
Branch Manager:
Kitchen role:
Analyst/Auditor:
Tenant isolation:
Branch isolation:
Nested-IDOR:

## TRANSACTION SAFETY

Idempotency:
Optimistic concurrency:
Audit:
Outbox:
Catalogue revision:
Rollback atomicity:
External network calls inside locked transactions:

## API

New public routes:
New internal routes:
Post-9B total routes:
OpenAPI parity:
Global route parity:

## TESTS

Migration:
Schedule:
DST:
Overnight:
Availability:
Price history:
Effective price:
Effective availability:
Prep profile:
Logistics profile:
Inventory seam:
IAM:
Security/IDOR:
Concurrency:
Revision/invalidation:
PostgreSQL:
Redis:

Failures:
Skips:

## BUILD QUALITY

gofmt:
go vet:
go build:
git diff --check:

## EXTERNAL SERVICE IMPACT

Media:
Wallet:
Customer:
Order:
Inventory:
Dispatch:
Taxi:
Hauling:

## DEFERRED CORRECTLY TO 9C

List exact Section 9C work still unimplemented.

## DOCUMENTATION UPDATED

List files.

## FINAL VERDICT

SECTION 9B PRICING, AVAILABILITY, SCHEDULING & OPERATIONAL PROFILES:

GO / NO-GO

REMAINING 9B CODE DEFECTS:

NONE or list them.

Confirm explicitly:

- No commit
- No stage
- No merge
- No branch created
- No branch switch
- No push
- Section 9C not started
- Section 10 not started

======================================================================
77. START IMPLEMENTATION NOW
======================================================================

Begin with the fast baseline verification.

Do NOT summarize this specification.

Do NOT produce another long implementation plan.

Read only the minimum existing files necessary to understand:

- current Section 9A implementation;
- migration conventions;
- Vendor audit/outbox;
- IAM;
- idempotency;
- route registration;
- OpenAPI/global route parity.

Then immediately begin implementation.

Proceed through:

schema
→ price-history integration
→ schedules
→ operational availability
→ branch overrides
→ pricing resolver
→ availability resolver
→ prep/logistics
→ inventory seam
→ IAM/HTTP
→ targeted tests
→ security/concurrency
→ OpenAPI/docs
→ final gate

Fix Section 9B regressions you introduce.

If you discover a real pre-existing defect that directly blocks Section 9B:

- fix only if narrow and clearly safe;
- add regression coverage;
- report it separately.

Do not perform unrelated cleanup.

Continue until every Section 9B acceptance criterion is satisfied or a genuine blocking external dependency is proven.

START SECTION 9B IMPLEMENTATION NOW.