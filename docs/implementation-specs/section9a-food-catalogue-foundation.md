KARRYGO — VENDOR SERVICE
SECTION 9A — FOOD CATALOGUE FOUNDATION
FULL BACKEND IMPLEMENTATION PROMPT

MODE: DEVELOPMENT-DRIVEN
TARGET: PRODUCTION-GRADE BACKEND IMPLEMENTATION
SCOPE: SECTION 9A ONLY
REPOSITORY: C:\KarryGoPush
INTENDED BRANCH: staging

======================================================================
0. EXECUTION DIRECTIVE
======================================================================

You are implementing Section 9A of the KarryGo Vendor Service.

This is NOT a design-only task.
This is NOT an architecture review.
This is NOT a gap-analysis-only task.

You must inspect the minimum relevant existing implementation, make the required code changes, create migrations, implement repositories/services/HTTP routes/IAM/audit/outbox behavior, add tests, update OpenAPI and required documentation, run the required validation, fix failures, and finish with a concise implementation report.

DO NOT stop after planning.

DO NOT ask for confirmation for normal implementation decisions that can be safely resolved from:
- the existing repository conventions,
- the accepted architecture,
- this prompt.

If a genuinely blocking conflict exists between this prompt and an established repository contract, preserve the established contract where possible and report the conflict.

Do not broaden the task into Section 9B, Section 9C, Section 10, Orders, Inventory, Dispatch, or frontend work.

======================================================================
1. USAGE-SAVING / FAST IMPLEMENTATION MODE
======================================================================

Work efficiently.

- Do not use subagents.
- Do not perform a full-repository audit.
- Do not repeatedly inspect already-accepted Sections 5–8.
- Inspect only files required to understand current Vendor Service patterns.
- Do not browse the web.
- Do not research external frameworks unnecessarily.
- Reuse existing repository patterns and dependencies.
- Do not introduce unnecessary dependencies.
- Do not refactor unrelated code.
- Do not rewrite accepted implementations simply for stylistic consistency.
- Do not run the full repository test suite during development.
- Run targeted tests while implementing.
- Run Section 9A/vendor-service validation once implementation stabilizes.
- Update OpenAPI/docs after functionality stabilizes.
- Keep progress commentary short.
- Stop when Section 9A acceptance requirements are satisfied.

Use the existing PostgreSQL/Redis test setup where available.

Avoid recreating test infrastructure unnecessarily.

======================================================================
2. GIT / WORKTREE SAFETY — MANDATORY
======================================================================

Work directly in:

C:\KarryGoPush

The intended branch is:

staging

Before modifying code perform only a QUICK verification:

git branch --show-current
git status --short
git log -1 --oneline

Do NOT:

- create a branch
- create a worktree
- switch branches
- commit
- stage files
- merge
- cherry-pick
- rebase
- push
- pull
- reset
- stash
- clean
- discard existing local work
- delete unrelated files

The working tree may already be dirty.

Preserve all unrelated changes.

Leave all Section 9A work UNCOMMITTED and UNSTAGED.

Do not touch unrelated root-level untracked files.

======================================================================
3. ACCEPTED BASELINE
======================================================================

Treat Sections 5 through 8I as accepted.

Do not re-audit them.

Known accepted 8I baseline before any later local work was approximately:

- Vendor routes: 194
- Vendor migration tip:
  000025_vendor_section8h_permissions
- CRM registry: 30
- Section 8I: GO
- CRM regression: green
- Media DATA_EXPORT support: AVAILABLE

HOWEVER:

The working tree is authoritative.

There may be newer accepted Section 8 completion work after 8I.

Therefore DO NOT blindly assume:

- route count is still 194
- migration tip is still 000025

Derive the actual current baseline from the repository before allocating migrations or calculating route deltas.

Do not redo or damage Section 8.

======================================================================
4. TARGET SECTION
======================================================================

SECTION 9 — RESTAURANT AND FAST-FOOD DOMAIN

This task is ONLY:

SECTION 9A — FOOD CATALOGUE FOUNDATION

9A answers:

    WHAT DOES THIS VENDOR / BRANCH SELL?

It builds the management-side authoritative catalogue foundation.

Section 9B will later answer:

    CAN THIS ITEM BE SOLD NOW, AT THIS BRANCH, AT THIS TIME,
    FOR WHAT EFFECTIVE PRICE, WITH WHAT PREPARATION/LOGISTICS PROFILE?

Section 9C will later provide:

    READ-OPTIMIZED DISCOVERY / INTERNAL CONSUMPTION +
    FINAL SECTION 9 HARDENING.

Do not implement 9B or 9C ahead of schedule.

======================================================================
5. AUTHORITATIVE DOMAIN BOUNDARIES
======================================================================

Vendor Service owns:

- vendor businesses
- vendor branches
- vendor capabilities
- food catalogue configuration
- menus
- menu sections
- food items
- variants
- modifier groups
- modifiers
- item/modifier relationships

Vendor Service does NOT own:

- payment execution
- wallet settlement
- customer identity
- customer canonical PII
- media bytes/storage
- delivery provider dispatch
- logistics OTP/QR authority
- inventory stock truth
- order authority
- customer discovery gateway
- customer consent authority
- refund authority

Do not access another service's database.

Do not introduce cross-service database foreign keys.

Cross-service entities must be referenced only through opaque IDs/contracts.

======================================================================
6. SECTION 9A DOMAIN MODEL
======================================================================

Implement the actual food catalogue hierarchy:

FOOD MENU
    ↓
MENU SECTION
    ↓
FOOD ITEM
    ↓
    ├── VARIANTS
    │
    └── MODIFIER GROUPS
            ↓
         MODIFIERS

Required logical entities:

1. food_menus
2. food_menu_sections
3. food_items
4. food_item_variants
5. modifier_groups
6. modifiers
7. food item ↔ modifier group relationships

You may use repository-consistent physical table naming such as:

food_modifier_groups
food_modifiers
food_item_modifier_groups

if necessary to prevent generic table-name collisions.

The API/domain vocabulary should remain clear.

Additional supporting tables are allowed where justified, for example:

- ordered item media references

Do not create speculative tables unrelated to 9A.

======================================================================
7. CAPABILITY ELIGIBILITY
======================================================================

Section 9A is the Restaurant/Fast-Food catalogue domain.

Minimum authoritative capabilities for this section:

- RESTAURANT
- FAST_FOOD

A branch must have the corresponding capability enabled before an active catalogue can be published for that capability.

Do NOT silently expand Section 9A to:

- CLOUD_KITCHEN
- PASTRY_BAKERY
- SHOP_GROCERY
- MARKET
- WHOLESALE

unless an existing authoritative KarryGo specification in the repository explicitly assigns those capabilities to this Section 9 implementation.

The schema must remain extensible so future sections can support additional food-related capabilities without destructive migration.

Reject non-food capabilities in this Section 9A surface.

======================================================================
8. MULTI-TENANT / BRANCH ISOLATION
======================================================================

Tenant isolation is mandatory.

Every catalogue aggregate must belong to a Vendor business.

Branch-specific entities must be bound to the correct branch and business.

Prevent all nested-IDOR cases.

Examples that MUST fail:

- Business A accesses Business B's menu.
- Branch A edits Branch B's menu.
- Menu section belongs to Menu A but request addresses it through Menu B.
- Item belongs to Section A but request addresses it through Section B.
- Variant belongs to Item A but is mutated through Item B.
- Modifier belongs to Group A but is mutated through Group B.
- Modifier group from Business A is attached to Business B's item.
- Branch-scoped actor mutates another branch.

Prefer composite foreign keys / ownership constraints where consistent with existing Vendor Service database design.

Do not rely only on HTTP-layer checks.

Repository/service-layer ownership validation is mandatory.

======================================================================
9. FOOD MENUS
======================================================================

Implement food menus.

At minimum a menu requires:

- id
- vendor_business_id
- branch_id
- capability
- name
- optional description if repository/product convention supports it
- lifecycle/status
- deterministic version/concurrency field
- created_at
- updated_at
- archived_at where applicable

A menu belongs to:

one business
+
one branch
+
one food capability

A branch may have multiple menus where product requirements allow it.

Examples might eventually include:

- Main Menu
- Breakfast Menu
- Dinner Menu

Do NOT implement time scheduling in 9A.

That belongs to 9B.

--------------------------------------------------
9.1 MENU STATUS
--------------------------------------------------

First inspect whether the repository/master specification already defines the exact food-menu status vocabulary.

Use it if present.

If there is no authoritative vocabulary, use the minimal closed lifecycle:

DRAFT
ACTIVE
PAUSED
ARCHIVED

Semantics:

DRAFT
- editable
- not considered published

ACTIVE
- published management state
- branch/business/capability prerequisites must be satisfied

PAUSED
- intentionally unavailable as a menu
- scheduling/effective availability still belongs to 9B

ARCHIVED
- terminal administrative state
- retained for historical references
- not destructive deletion

ARCHIVED objects must not be reactivated unless an existing repository lifecycle pattern explicitly allows it.

Do not use arbitrary free-text status values.

--------------------------------------------------
9.2 MENU ACTIVATION
--------------------------------------------------

Catalogue preparation should be possible before the complete business is operational where this is consistent with onboarding.

However activation/publishing must enforce the real Section 6/7 state prerequisites.

Before ACTIVE, validate at minimum:

- correct vendor business
- correct branch
- branch capability exists
- capability is RESTAURANT or FAST_FOOD
- capability is operational/enabled according to existing Vendor rules
- business/branch state satisfies existing activation requirements

Do not create a second onboarding authority.

Reuse existing Section 6/7 business/branch/capability status APIs/repositories.

======================================================================
10. MENU SECTIONS
======================================================================

Implement ordered sections/groups inside a menu.

Examples:

- Starters
- Main Meals
- Burgers
- Drinks
- Desserts

Required fields include:

- id
- business ownership
- menu_id
- name
- optional description
- position / sort order
- status/archive marker where required
- version
- timestamps

Section ordering must be deterministic.

Support transactional reorder.

Reordering must:

- verify all section IDs belong to the addressed menu
- reject duplicates
- reject foreign IDs
- preserve deterministic ordering
- be atomic
- handle concurrent updates safely

Do not allow an item to be silently orphaned when a section is archived.

Use repository-consistent archive semantics.

======================================================================
11. FOOD ITEMS
======================================================================

Implement food items.

At minimum support:

- id
- business ownership
- branch/menu/section relationship as appropriate
- name
- description
- base price in integer minor units
- media references
- dietary metadata
- allergen metadata
- lifecycle/status
- deterministic ordering within section
- row/entity version
- created_at
- updated_at
- archived_at where appropriate

Do not use floating-point money.

Never use float32/float64 for prices.

Use integer minor units:

price_minor

Example:

₦2,500.00 where NGN minor-unit policy applies must be represented according to the platform's established money convention.

Follow Payment/Wallet/Vendor existing money conventions.

Do not create an alternative money representation.

--------------------------------------------------
11.1 CURRENCY
--------------------------------------------------

Inspect whether VendorBusiness already has an authoritative currency.

If yes:

reuse it.

Do not duplicate conflicting currency truth.

If no existing authoritative business currency exists, use the minimum repository-consistent ISO-4217 currency representation necessary for catalogue price integrity.

Do NOT build:

- FX conversion
- multi-currency checkout
- dynamic currency conversion

--------------------------------------------------
11.2 ITEM LIFECYCLE
--------------------------------------------------

Use an existing authoritative vocabulary if one exists.

Otherwise use a bounded lifecycle consistent with:

DRAFT
ACTIVE
PAUSED
ARCHIVED

Do not confuse Section 9A's administrative PAUSED state with Section 9B's temporary sold-out/effective availability system.

A future SOLD_OUT condition belongs to 9B.

--------------------------------------------------
11.3 ITEM ORDERING
--------------------------------------------------

Items must have deterministic positions within a section.

Provide atomic reorder behavior.

Reject:

- duplicate IDs
- IDs from another section
- cross-business IDs
- archived items where invalid

Use optimistic concurrency consistent with the existing service.

======================================================================
12. ITEM MEDIA
======================================================================

Items may reference media.

MEDIA FILE SERVICE remains the authority for:

- upload
- bytes
- MIME validation
- storage
- scanning
- visibility
- access
- retention
- deletion

Vendor Service stores only opaque media references.

Do not:

- store image bytes
- store base64 media
- create a competing upload system
- manipulate Media Service storage directly
- access Media Service tables

Support multiple item media references if consistent with the product requirements.

If multiple media are supported, preserve:

- deterministic ordering
- primary/cover designation where needed
- duplicate prevention

If ownership validation through an existing Media private API is already established, reuse it.

Do not expand Media Service unless a concrete 9A requirement cannot be satisfied without a minimal contract change.

If Media support necessary for this exact use case is absent:

document the dependency cleanly rather than fabricating authority.

======================================================================
13. DIETARY / ALLERGEN METADATA
======================================================================

Food items must support dietary and allergen metadata.

Do not turn this into a medical/nutrition subsystem.

Do not fabricate health certification.

Do not make unsupported claims that an item is medically safe.

Treat metadata as vendor-provided catalogue information.

First inspect whether KarryGo already defines controlled vocabularies.

If yes:

reuse them.

If no:

use a bounded and extensible code-based design rather than arbitrary unbounded blobs.

Requirements:

- validation
- maximum number of codes
- normalized casing
- no duplicate codes
- bounded code length
- no arbitrary executable/query content

Document that dietary/allergen metadata is declarative vendor-supplied information.

Do NOT build:

- nutritional calculation
- calorie computation
- medical allergy recommendation
- ingredient inventory
- ingredient depletion

======================================================================
14. ITEM VARIANTS
======================================================================

Implement food item variants.

Examples:

Burger:
- Single
- Double

Pizza:
- Small
- Medium
- Large

Required fields include:

- id
- business ownership
- item_id
- name
- position
- price override or repository-consistent pricing representation
- lifecycle/archive state
- version
- timestamps

The original Section 9 requirement calls for:

size / portion / price overrides

Do not use floating-point pricing.

A clean model is:

price_override_minor nullable

where:

NULL
= inherit base item price

non-NULL
= variant-specific price

If the established platform pricing model uses a different deterministic representation, follow it.

Do not implement full effective-pricing history/resolution in 9A.

That belongs to 9B.

But the 9A data model must not block later price versioning.

Validate:

- non-negative prices
- bounded names
- deterministic order
- unique constraints where appropriate
- ownership
- archived parent
- concurrency

Do not allow invalid or foreign variants to mutate another item.

======================================================================
15. MODIFIER GROUPS
======================================================================

Implement modifier groups.

Examples:

"Choose your sauce"

"Add extras"

"Choose drink"

"Choose toppings"

Required data:

- id
- vendor_business_id
- name
- selection constraints
- version
- lifecycle/archive state
- timestamps

A modifier group must define deterministic selection constraints.

At minimum:

min_select
max_select

Rules:

min_select >= 0

max_select >= 1 where group is usable

max_select >= min_select

Required groups can be derived from:

min_select > 0

Avoid storing conflicting concepts such as:

required = false
min_select = 1

unless existing repository conventions demand an explicit required flag and enforce consistency.

Do not allow negative or unbounded selection values.

Set sane product limits.

Do not build arbitrary expression/rule engines for modifiers.

======================================================================
16. MODIFIERS
======================================================================

Implement modifiers under modifier groups.

Examples:

Group:
Add extras

Modifiers:
- Extra cheese +₦500
- Bacon +₦800
- Extra sauce +₦200

Required fields:

- id
- business ownership
- modifier_group_id
- name
- price_delta_minor
- position
- lifecycle/archive state
- version
- timestamps

Price delta must be integer minor units.

Do not use float.

A modifier price delta MAY be zero.

Unless the existing specification explicitly supports discounts through modifiers, reject negative modifier deltas.

Promotions/discounts belong elsewhere.

Do not use modifiers as a hidden promotion system.

======================================================================
17. ITEM ↔ MODIFIER GROUP RELATIONSHIPS
======================================================================

Support attaching modifier groups to food items.

This must be a proper relationship.

Do not duplicate modifier-group definitions into every food item.

The relation should support deterministic display/order position.

Requirements:

- same business
- correct tenant
- item exists
- modifier group exists
- no duplicate attachment
- archived groups cannot be newly attached
- archived items cannot accept new attachments
- detach operation must remove only the relationship
- do not delete the modifier group when detaching from one item
- atomic reorder of attached groups

If modifier groups are intended to be branch-specific under current architecture, enforce branch compatibility.

If they are business-level reusable groups, enforce business ownership and allow reuse only within that business.

Choose based on the established repository/domain architecture.

Document the decision.

======================================================================
18. PRICE INTEGRITY BOUNDARY
======================================================================

Section 9 overall requires price changes to be versioned enough that historical orders never recompute from current menu price.

BUT:

Full price history/effective-price resolution belongs to Section 9B.

9A must establish the correct foundation without pretending Orders exist.

For 9A:

- current base price is authoritative catalogue truth
- current variant override is authoritative catalogue truth
- modifier price delta is current catalogue truth
- mutations use entity/version concurrency
- price mutations are audited
- price mutations produce the existing transactional outbox events where applicable

Do NOT create:

- Order tables
- Order line tables
- cart pricing
- immutable order snapshots
- checkout pricing
- payment records

Do not claim historical Order pricing is implemented in 9A.

Section 9B will implement proper catalogue price-version/history and effective-price resolution needed for future Order snapshots.

Design 9A so 9B can add this additively.

======================================================================
19. DO NOT IMPLEMENT SECTION 9B
======================================================================

The following are explicitly OUT OF SCOPE for this task:

- menu schedules
- breakfast/lunch/dinner schedules
- seasonal schedules
- timezone schedule evaluation
- sold-out state engine
- temporary item availability
- branch-specific price overrides
- branch-specific prep overrides
- branch item overrides
- effective price resolver
- effective availability resolver
- price history engine
- price snapshot resolver
- preparation-time profile
- logistics profile
- inventory-linked availability
- ingredient-linked availability
- inventory reconciliation
- discovery invalidation

Do not partially invent these.

The 9A schema may provide clean future extension points.

======================================================================
20. DO NOT IMPLEMENT SECTION 9C
======================================================================

Do not implement:

- customer-facing discovery
- public catalogue discovery
- customer app routes
- API Gateway discovery integration
- full catalogue projection endpoint
- internal pricing resolution API
- internal sellability API
- search indexing
- recommendation systems
- customer personalization
- discovery caching infrastructure

Section 9C owns consumption/discovery integration.

9A is the management-side authoritative catalogue foundation.

======================================================================
21. IAM / AUTHORIZATION
======================================================================

Reuse the accepted Section 5 IAM framework.

Do not create a parallel authorization system.

Add the minimum Section 9A permissions using existing permission naming conventions.

At minimum the authorization model must distinguish:

READ
from
MANAGE

and, if repository conventions support it cleanly:

PUBLISH/ACTIVATE

Examples conceptually:

vendor.food.catalogue.read
vendor.food.catalogue.manage
vendor.food.catalogue.publish

Do NOT force these literal names if established permission naming follows another exact pattern.

Use the repository's canonical naming style.

Expected role behavior:

Owner / Account Controller:
full business access

General Manager:
business-level management according to existing role policy

Branch Manager:
manage only authorized branch catalogue

Kitchen / Pastry:
appropriate food catalogue operational access where role semantics permit

Analyst / Auditor:
read-only

Other roles:
deny unless explicitly mapped

Deny by default.

Branch scope must remain enforced even if the actor has a broad role family.

Do not weaken accepted IAM.

======================================================================
22. STEP-UP AUTH
======================================================================

Do NOT require step-up authentication for ordinary menu/item catalogue management unless the existing IAM policy explicitly requires it.

These are not equivalent to:

- bank changes
- owner transfer
- sensitive export
- high-risk IAM mutation

Do not overuse step-up.

======================================================================
23. HTTP API
======================================================================

Use the existing Vendor public-management base:

/api/v1/vendor/...

Use the existing authentication middleware.

Follow established handler/error/pagination/idempotency conventions.

Recommended logical surface:

--------------------------------------------------
MENUS
--------------------------------------------------

POST
/api/v1/vendor/food/menus

GET
/api/v1/vendor/food/menus

GET
/api/v1/vendor/food/menus/{menuId}

PATCH
/api/v1/vendor/food/menus/{menuId}

POST
/api/v1/vendor/food/menus/{menuId}/activate

POST
/api/v1/vendor/food/menus/{menuId}/pause

POST
/api/v1/vendor/food/menus/{menuId}/archive

--------------------------------------------------
SECTIONS
--------------------------------------------------

POST
/api/v1/vendor/food/menus/{menuId}/sections

GET
/api/v1/vendor/food/menus/{menuId}/sections

PATCH
/api/v1/vendor/food/menus/{menuId}/sections/{sectionId}

POST
/api/v1/vendor/food/menus/{menuId}/sections/{sectionId}/archive

POST
/api/v1/vendor/food/menus/{menuId}/sections/reorder

--------------------------------------------------
ITEMS
--------------------------------------------------

POST
/api/v1/vendor/food/menus/{menuId}/sections/{sectionId}/items

GET
/api/v1/vendor/food/menus/{menuId}/sections/{sectionId}/items

GET
/api/v1/vendor/food/items/{itemId}

PATCH
/api/v1/vendor/food/items/{itemId}

POST
/api/v1/vendor/food/items/{itemId}/archive

POST
/api/v1/vendor/food/menus/{menuId}/sections/{sectionId}/items/reorder

--------------------------------------------------
ITEM MEDIA
--------------------------------------------------

Provide a repository-consistent endpoint for replacing/managing ordered
opaque media references.

For example:

PUT
/api/v1/vendor/food/items/{itemId}/media

Do not create raw media-upload routes.

--------------------------------------------------
VARIANTS
--------------------------------------------------

POST
/api/v1/vendor/food/items/{itemId}/variants

GET
/api/v1/vendor/food/items/{itemId}/variants

PATCH
/api/v1/vendor/food/items/{itemId}/variants/{variantId}

POST
/api/v1/vendor/food/items/{itemId}/variants/{variantId}/archive

POST
/api/v1/vendor/food/items/{itemId}/variants/reorder

--------------------------------------------------
MODIFIER GROUPS
--------------------------------------------------

POST
/api/v1/vendor/food/modifier-groups

GET
/api/v1/vendor/food/modifier-groups

GET
/api/v1/vendor/food/modifier-groups/{groupId}

PATCH
/api/v1/vendor/food/modifier-groups/{groupId}

POST
/api/v1/vendor/food/modifier-groups/{groupId}/archive

--------------------------------------------------
ITEM / MODIFIER GROUP RELATIONSHIPS
--------------------------------------------------

POST or PUT using the repository's established relationship semantics:

/api/v1/vendor/food/items/{itemId}/modifier-groups/{groupId}

DELETE:

/api/v1/vendor/food/items/{itemId}/modifier-groups/{groupId}

GET:

/api/v1/vendor/food/items/{itemId}/modifier-groups

POST:

/api/v1/vendor/food/items/{itemId}/modifier-groups/reorder

--------------------------------------------------
MODIFIERS
--------------------------------------------------

POST
/api/v1/vendor/food/modifier-groups/{groupId}/modifiers

GET
/api/v1/vendor/food/modifier-groups/{groupId}/modifiers

PATCH
/api/v1/vendor/food/modifier-groups/{groupId}/modifiers/{modifierId}

POST
/api/v1/vendor/food/modifier-groups/{groupId}/modifiers/{modifierId}/archive

POST
/api/v1/vendor/food/modifier-groups/{groupId}/modifiers/reorder

--------------------------------------------------

These route shapes are a target logical contract.

Before implementation, inspect existing Vendor route conventions.

Small adaptations are allowed for consistency.

Do not add endpoints just to reach a route-count target.

Route quality > route quantity.

======================================================================
24. API BEHAVIOR
======================================================================

Use consistent:

- request DTOs
- response DTOs
- validation
- error envelopes
- pagination
- filtering
- idempotency
- authorization
- optimistic concurrency
- audit
- transactional outbox

Never expose internal database models directly if existing architecture separates DTOs/domain models.

List endpoints must be bounded.

Use sensible default/max pagination consistent with Vendor Service.

Avoid N+1 queries.

======================================================================
25. IDEMPOTENCY
======================================================================

Use the existing Vendor Service idempotency architecture.

At minimum protect appropriate create/state-changing operations from accidental duplicate execution.

Follow established header/request conventions.

Do not invent a second idempotency mechanism.

Idempotent retries must not create duplicate:

- menus
- sections
- items
- variants
- modifier groups
- modifiers
- relationships

where an existing idempotency contract applies.

======================================================================
26. OPTIMISTIC CONCURRENCY
======================================================================

All mutable catalogue aggregates must participate in the established concurrency model.

Use the repository's accepted expected-version mechanism.

Do not silently accept stale mutations.

Test stale writes.

Important concurrency targets:

- concurrent menu edits
- concurrent item edits
- concurrent reorders
- concurrent modifier edits
- attach same modifier group twice
- archive while another mutation occurs

Exactly one deterministic result should win where required.

Do not create deadlock-prone locking.

======================================================================
27. REORDER OPERATIONS
======================================================================

Reordering is a first-class correctness requirement.

For:

- menu sections
- items
- variants
- item modifier groups
- modifiers

the reorder operation must:

1. run transactionally
2. validate ownership
3. reject duplicate IDs
4. reject foreign IDs
5. reject unexpected IDs according to the chosen API semantics
6. preserve deterministic contiguous or stable ordering
7. use concurrency/version fences
8. audit the mutation
9. emit required outbox event(s)

Avoid N individual transactions.

Use a single atomic reorder operation.

======================================================================
28. ARCHIVE / DELETE POLICY
======================================================================

Prefer archive/tombstone semantics for catalogue entities.

Do not hard-delete catalogue entities that may later be referenced by:

- orders
- historical snapshots
- analytics
- audit
- CRM
- dispute records

Future references must remain resolvable.

Hard deletion is acceptable only for pure relationship rows where removal has no historical authority implication, for example detaching an item/modifier-group relationship, provided audit/outbox requirements are preserved.

Do not cascade-delete an entire historical catalogue graph.

======================================================================
29. DATABASE DESIGN
======================================================================

Use PostgreSQL.

Follow existing migration conventions.

Use:

- UUID/reference conventions already established by Vendor Service
- timestamps consistent with repository convention
- tenant ownership columns
- composite ownership constraints where appropriate
- proper foreign keys
- CHECK constraints for bounded state where repository conventions use them
- unique indexes
- query-supporting indexes
- deterministic versions
- no floating money

Important indexes should cover at minimum common access patterns such as:

- business + branch
- business + menu
- menu + position
- section + position
- item + active/archive state
- item variants + position
- business + modifier group
- modifier group + position
- item/modifier relationship

Do not over-index blindly.

======================================================================
30. MIGRATIONS
======================================================================

Determine the actual current migration tip.

Do NOT blindly choose 000026.

If the current accepted tip remains 000025, likely create:

000026_vendor_food_catalogue
000027_vendor_section9a_permissions

If newer migrations already exist, allocate the next free numbers.

Use repository naming conventions.

Migration 1 should contain Section 9A schema.

Migration 2 should contain Section 9A IAM permissions/role mappings if that matches existing architecture.

Migrations must:

- apply cleanly on the current schema
- apply on a fresh/replayed Vendor database
- preserve all previous data
- not modify another service's schema
- avoid destructive changes

Do not renumber existing migrations.

======================================================================
31. AUDIT
======================================================================

Every meaningful mutation must participate in the existing Vendor audit system.

Examples:

- menu created/updated/activated/paused/archived
- section created/updated/reordered/archived
- item created/updated/reordered/archived
- item media changed
- variant created/updated/reordered/archived
- modifier group created/updated/archived
- modifier created/updated/reordered/archived
- modifier-group attached/detached/reordered

Audit records must preserve:

- actor
- vendor business
- branch where applicable
- action
- subject/entity reference
- relevant safe metadata
- timestamp

Do not log secrets.

Do not log customer PII.

For new 9A code, use the strongest current transactional audit pattern available.

Do not copy known legacy append-after-commit weaknesses into new code if the current accepted infrastructure supports same-transaction persistence.

======================================================================
32. TRANSACTIONAL OUTBOX
======================================================================

Use the accepted Vendor transactional outbox.

Catalogue mutations that other domains may eventually consume should generate durable domain events.

Do not publish external network events synchronously inside an open transaction.

Persist state + outbox atomically.

Potential event vocabulary should follow repository conventions.

Conceptually:

food.menu.created
food.menu.updated
food.menu.activated
food.menu.paused
food.menu.archived

food.section.created
food.section.updated
food.section.reordered
food.section.archived

food.item.created
food.item.updated
food.item.archived
food.item.reordered

food.variant.created
food.variant.updated
food.variant.archived

food.modifier_group.created
food.modifier_group.updated
food.modifier_group.archived

food.modifier.created
food.modifier.updated
food.modifier.archived

Do not generate unnecessary event explosion if repository conventions use aggregate-level events instead.

Use stable event schemas/versioning consistent with existing outbox.

======================================================================
33. DOMAIN VALIDATION
======================================================================

Implement bounded validation.

At minimum:

- names non-empty after trimming
- descriptions bounded
- identifiers valid
- price >= 0
- price within safe integer bounds
- no float money
- capability valid
- min/max modifier selections valid
- duplicate associations rejected
- duplicate reorder IDs rejected
- invalid foreign parent IDs rejected
- archived parent cannot accept invalid new children
- unsupported lifecycle transitions rejected
- branch/business ownership enforced
- dietary/allergen metadata bounded
- media reference count bounded
- media references unique where appropriate

Use existing validation/error conventions.

Do not create arbitrary database errors as user-facing API responses.

======================================================================
34. SECURITY
======================================================================

Section 9A must include security tests.

Test:

- missing authentication
- wrong tenant
- wrong branch
- wrong capability
- nested-IDOR
- archived entities
- invalid expected version
- replay/idempotency where applicable
- malformed IDs
- excessive payload sizes
- duplicate relation abuse
- reorder foreign IDs
- branch manager scope
- analyst write denial

Do not introduce raw SQL composed from user-provided strings.

Use parameterized queries.

Do not leak existence of another tenant's object unnecessarily.

Prefer the service's established not-found/forbidden behavior.

======================================================================
35. BUSINESS / BRANCH STATUS
======================================================================

Do not duplicate lifecycle truth.

Use Section 6/7 existing business and branch state.

Important distinction:

A vendor may need to prepare its catalogue while still onboarding.

Therefore:

creation/editing of DRAFT catalogue data may be allowed before full activation where existing product semantics permit.

But publishing ACTIVE catalogue must enforce activation prerequisites.

Do not accidentally block catalogue preparation solely because the vendor has not gone live.

Do not accidentally publish a catalogue for an unverified/non-operational branch.

======================================================================
36. FEATURE FLAG
======================================================================

Inspect the existing feature-flag architecture.

Add Section 9A's feature control only if this is the established pattern.

Conceptually:

VENDOR_FOOD_CATALOGUE_ENABLED

Do not introduce an environment-variable pattern inconsistent with the service.

Routes should remain deterministic for OpenAPI/route parity.

Avoid environment-dependent route registration if existing architecture instead gates behavior at service/handler level.

Default production behavior should be safe.

Document activation requirements.

======================================================================
37. ERROR MODEL
======================================================================

Reuse existing Vendor errors.

Add Section 9A domain errors only as required.

Examples conceptually:

FOOD_CAPABILITY_NOT_SUPPORTED
MENU_NOT_FOUND
MENU_NOT_EDITABLE
MENU_ALREADY_ARCHIVED
SECTION_NOT_FOUND
ITEM_NOT_FOUND
VARIANT_NOT_FOUND
MODIFIER_GROUP_NOT_FOUND
MODIFIER_NOT_FOUND
INVALID_SELECTION_LIMITS
INVALID_PRICE
STALE_VERSION
INVALID_REORDER
DUPLICATE_RELATIONSHIP

Do not create inconsistent HTTP status mappings.

Follow existing API error conventions.

======================================================================
38. REPOSITORY / SERVICE LAYERS
======================================================================

Follow current Vendor feature package architecture.

Do not put all logic into HTTP handlers.

Expected separation should follow existing patterns, conceptually:

domain/models
repository
service/application
HTTP handlers
routes
errors
contracts/DTOs
tests

Business rules belong outside raw route wiring.

Repositories should enforce tenant-aware queries.

Avoid giant god files where existing architecture uses smaller components.

Do not refactor the whole Vendor service architecture.

======================================================================
39. TESTING STRATEGY
======================================================================

This task is development-driven.

Run targeted tests while building.

Do NOT repeatedly run the full Vendor CRM suite.

Do NOT run the full KarryGo repository regression for 9A unless a shared-core change makes it necessary.

Section 9A tests should use real PostgreSQL/Redis infrastructure where the existing suite requires it.

======================================================================
40. REQUIRED TEST COVERAGE
======================================================================

At minimum implement tests for:

--------------------------------------------------
MIGRATIONS
--------------------------------------------------

- migration applies
- required tables exist
- constraints exist
- indexes exist where critical
- permission seed applies
- fresh schema replay succeeds

--------------------------------------------------
MENU
--------------------------------------------------

- create
- list
- read
- update
- activate
- pause
- archive
- invalid lifecycle transition
- wrong branch
- wrong tenant
- wrong capability
- stale version
- duplicate idempotent create retry

--------------------------------------------------
MENU SECTION
--------------------------------------------------

- create
- list
- update
- archive
- reorder
- duplicate reorder ID
- foreign section in reorder
- cross-menu nested IDOR
- concurrency

--------------------------------------------------
ITEM
--------------------------------------------------

- create
- list
- get
- update
- archive
- reorder
- integer price handling
- zero price if valid
- negative price rejection
- overflow/bounds rejection
- cross-tenant rejection
- cross-section rejection
- stale update
- invalid status
- media metadata validation
- dietary/allergen validation

--------------------------------------------------
VARIANT
--------------------------------------------------

- create
- list
- update
- archive
- reorder
- inherit base price
- explicit price override
- negative override rejected
- cross-item nested IDOR

--------------------------------------------------
MODIFIER GROUP
--------------------------------------------------

- create
- list
- get
- update
- archive
- min/max validation
- invalid min > max
- invalid negative min
- ownership isolation

--------------------------------------------------
MODIFIER
--------------------------------------------------

- create
- list
- update
- archive
- reorder
- zero delta
- positive delta
- negative delta rejected unless spec explicitly supports it
- cross-group nested IDOR

--------------------------------------------------
ITEM ↔ MODIFIER GROUP
--------------------------------------------------

- attach
- duplicate attach
- detach
- list
- reorder
- cross-business attach rejected
- archived group attach rejected
- archived item attach rejected

--------------------------------------------------
IAM
--------------------------------------------------

- Owner allowed
- General Manager according to policy
- Branch Manager limited to authorized branch
- Kitchen role according to policy
- Analyst/Auditor read-only
- unauthorized role denied
- missing auth denied

--------------------------------------------------
AUDIT
--------------------------------------------------

- meaningful mutations produce correct audit evidence
- no secrets/PII

--------------------------------------------------
OUTBOX
--------------------------------------------------

- state and outbox commit together
- rollback prevents orphan event
- expected event type/version

--------------------------------------------------
CONCURRENCY
--------------------------------------------------

Use deterministic tests where appropriate for:

- simultaneous item updates
- simultaneous reorder
- duplicate relation attach
- archive/update race

Do not rely on flaky sleep-based timing.

======================================================================
41. PERFORMANCE / SCALE
======================================================================

No huge load benchmark is required in 9A.

But verify the design works for realistic catalogue sizes.

Use a moderate fixture such as:

- several menus
- dozens of sections
- hundreds of items
- multiple variants per item
- multiple modifier groups
- multiple modifiers per group

Check for obvious N+1 problems.

List APIs must paginate or be safely bounded where appropriate.

Reorder operations must not perform pathological query loops.

If a query is important and non-trivial, inspect the query/index relationship.

Do not spend the task doing broad performance research.

======================================================================
42. OPENAPI
======================================================================

After code stabilizes:

Update the appropriate Vendor OpenAPI specification(s).

Every production Section 9A route must be documented.

Every documented Section 9A route must exist.

Document:

- authentication
- permissions
- request fields
- response fields
- enums
- pagination
- idempotency where applicable
- expected-version concurrency
- errors
- money minor-unit semantics
- archive behavior
- capability restrictions

Run route/OpenAPI parity tests.

Do not leave undocumented production routes.

======================================================================
43. API ROUTE REGISTER
======================================================================

Update:

API_ROUTE_REGISTER.md

Use the repository's canonical route syntax consistently.

Record:

- method
- path
- public/internal
- auth
- permission
- Section 9A ownership

Use the global route parity guard established in Section 8I if it is applicable to these routes.

Do not weaken that guard.

======================================================================
44. IMPLEMENTATION STATUS
======================================================================

Update:

IMPLEMENTATION_STATUS.md

Mark only Section 9A as implemented when all acceptance criteria are met.

Do NOT mark:

9B complete
9C complete
Section 9 complete

Expected status conceptually:

9A FOOD CATALOGUE FOUNDATION — IMPLEMENTED / TESTED

9B PRICING, AVAILABILITY, SCHEDULING & OPERATIONAL PROFILES — NOT STARTED

9C DISCOVERY, INTEGRATION & FINAL SECTION 9 ACCEPTANCE — NOT STARTED

======================================================================
45. PLATFORM SERVICE CHANGE REGISTER
======================================================================

Update:

PLATFORM_SERVICE_CHANGE_REGISTER.md

Record only real external-service impacts.

Expected 9A should ideally require:

NO external service production changes

unless Media reference validation requires a minimal documented existing integration.

Do not invent requirements for:

- Customer Service
- Wallet
- Dispatch
- Taxi
- Hauling
- Inventory
- Orders

Document future dependencies separately rather than implementing them.

======================================================================
46. README / ARCHITECTURE DOCUMENTATION
======================================================================

Update the relevant Vendor Service documentation once the implementation is stable.

At minimum reconcile:

README.md

and relevant architecture docs.

Document:

- catalogue hierarchy
- capability boundary
- tenant ownership
- money representation
- menu lifecycle
- archive behavior
- variants
- modifier semantics
- media reference ownership
- 9A/9B/9C boundary

Do not create excessive duplicate documentation.

======================================================================
47. UI / FRONTEND REQUIREMENT DOCUMENTATION
======================================================================

This is BACKEND ONLY.

Do NOT implement frontend.

However update the existing required backend handoff docs where Section 9A creates UI/API implications:

UI_UX_REQUIREMENTS.md
FRONTEND_REQUIREMENTS.md

Document backend-derived requirements such as:

- menu management
- section ordering
- item editor
- variant editor
- modifier-group editor
- drag/reorder semantics
- status actions
- concurrency conflict handling
- media reference selection
- price minor-unit/API representation
- capability restrictions

Do not design or code frontend screens.

======================================================================
48. NO CUSTOMER DISCOVERY YET
======================================================================

A management GET route is not the same as customer discovery.

Do not claim Section 9A creates customer-facing Food discovery.

The customer app integration belongs later.

Section 9C will establish the safe read/consumption contract.

======================================================================
49. NO ORDERS
======================================================================

Do not create:

orders
order_items
cart
checkout
order price snapshot
restaurant acceptance flow
order SLA
refunds
order cancellations

Section 9A is catalogue only.

If a test or interface requires an Order authority, stop that path and document:

PENDING_EXTERNAL_DOMAIN — ORDER AUTHORITY

Do not fabricate an Order Service.

======================================================================
50. NO INVENTORY
======================================================================

Do not implement:

ingredient stock
stock decrement
stock reservation
stock reconciliation
low-stock alerts
ingredient recipes
automatic sold-out due to stock

Section 9B will add the inventory availability seam.

A later inventory domain will own actual stock.

======================================================================
51. NO LOGISTICS / DISPATCH
======================================================================

Do not:

- assign riders
- choose vehicle
- calculate delivery fee
- call Dispatch
- call Taxi
- call Hauling
- create logistics OTP/QR
- create delivery mission

Food item's logistics profile belongs to 9B.

Actual provider assignment belongs to logistics services.

======================================================================
52. NO WALLET / PAYMENT
======================================================================

Do not call Wallet merely because items have prices.

Catalogue prices are commercial configuration, not monetary settlement.

Wallet remains sole authority for money movement.

Do not create:

- wallet balance
- settlement
- payment intent
- escrow
- commission calculation
- vendor payout

======================================================================
53. EVENTUAL ORDER SNAPSHOT COMPATIBILITY
======================================================================

Design identifiers and catalogue data so a future Order domain can snapshot:

item_ref
item_version
variant_ref
selected modifiers
base_price_minor
variant pricing
modifier deltas
currency
pricing version
captured_at

Do not implement that snapshot in 9A.

Simply avoid designs that make future immutable snapshots impossible.

======================================================================
54. CODE QUALITY
======================================================================

After implementation stabilizes run:

- gofmt on touched Go files
- targeted go test
- appropriate Vendor Service build
- go vet for affected service/packages
- git diff --check

Do not format unrelated files.

Do not mass-rewrite go.mod/go.sum unless a real dependency requires it.

Avoid adding a new dependency when standard library/current dependencies suffice.

======================================================================
55. DEVELOPMENT ORDER
======================================================================

Follow this implementation order to avoid wasting time:

PHASE A — QUICK BASELINE
- branch
- status
- current migration tip
- current route count
- locate relevant Section 6/7/IAM/outbox/audit patterns
- locate Vendor HTTP route registration
- locate OpenAPI/route-register patterns

Do not audit entire CRM.

PHASE B — SCHEMA
- food catalogue migration
- indexes
- constraints
- permissions migration

PHASE C — DOMAIN + REPOSITORY
Implement:

- menus
- sections
- items
- variants
- modifier groups
- modifiers
- item/modifier relationships

PHASE D — APPLICATION/SERVICE LOGIC
Implement:

- ownership
- capability gates
- lifecycle rules
- price validation
- reorder logic
- archive behavior
- concurrency
- audit
- outbox

PHASE E — HTTP + IAM
Implement routes and permissions.

PHASE F — TARGETED TESTS
Run feature tests while fixing implementation.

PHASE G — SECURITY / CONCURRENCY TESTS
Run IDOR, permissions, versioning and reorder tests.

PHASE H — OPENAPI + DOCS
Only after API behavior stabilizes.

PHASE I — SECTION 9A FINAL GATE
Run final Section 9A validation once.

PHASE J — REPORT
Return concise exact results.

======================================================================
56. THINGS YOU MUST NOT DO
======================================================================

Do NOT:

- implement frontend
- start Section 9B
- start Section 9C
- start Section 10
- build Orders
- build Inventory
- build Checkout
- build Dispatch integration
- build Taxi integration
- build Hauling integration
- build logistics OTP/QR
- build recommendation AI
- build search AI
- build dynamic pricing
- build promotions
- build coupons
- build loyalty changes
- modify CRM unnecessarily
- create a new authentication system
- create a new media system
- create a new monetary authority
- duplicate business/branch authority
- access another service DB
- introduce floating-point money
- invent customer PII
- create branches/worktrees
- commit/stage/push
- perform broad unrelated refactors
- run endless full-repository tests
- spend the session writing plans instead of code

======================================================================
57. ACCEPTANCE CRITERIA
======================================================================

Section 9A is GO only when all of the following are true:

1. Food menus implemented.
2. Menu sections implemented.
3. Food items implemented.
4. Item variants implemented.
5. Modifier groups implemented.
6. Modifiers implemented.
7. Item/modifier-group relationships implemented.
8. Deterministic ordering/reordering implemented.
9. Integer minor-unit pricing enforced.
10. Restaurant/Fast-Food capability boundaries enforced.
11. Tenant isolation enforced.
12. Branch isolation enforced.
13. Nested-IDOR prevented.
14. Menu/item lifecycle implemented.
15. Archive semantics safe for future historical references.
16. Media remains opaque reference only.
17. Dietary/allergen metadata supported safely.
18. IAM permissions implemented.
19. Branch Manager scope enforced.
20. Analyst/Auditor remains read-only.
21. Idempotency follows existing architecture.
22. Optimistic concurrency implemented.
23. Audit integrated.
24. Transactional outbox integrated.
25. Migrations apply cleanly.
26. Fresh migration replay succeeds.
27. Relevant PostgreSQL/Redis tests pass.
28. Security tests pass.
29. Deterministic concurrency tests pass.
30. gofmt clean for touched files.
31. go vet passes for affected service/packages.
32. build passes.
33. git diff --check passes.
34. OpenAPI matches production routes.
35. API_ROUTE_REGISTER updated.
36. IMPLEMENTATION_STATUS updated.
37. PLATFORM_SERVICE_CHANGE_REGISTER updated.
38. relevant README/architecture docs updated.
39. UI_UX_REQUIREMENTS updated where necessary.
40. FRONTEND_REQUIREMENTS updated where necessary.
41. No frontend code touched.
42. No external authority fabricated.
43. No Section 9B functionality falsely marked complete.
44. No Section 9C functionality falsely marked complete.
45. No Git commit/stage/branch/push performed.

======================================================================
58. FINAL ROUTE / MIGRATION VERIFICATION
======================================================================

At completion derive the REAL values from production code.

Do not guess.

Report:

PRE_9A_ROUTE_COUNT
NEW_9A_PUBLIC_ROUTES
NEW_9A_INTERNAL_ROUTES
POST_9A_ROUTE_COUNT

Report:

PRE_9A_MIGRATION_TIP
NEW_9A_MIGRATIONS
POST_9A_MIGRATION_TIP

Also report actual tables created.

Do not optimize implementation around a desired route count.

======================================================================
59. FINAL REPORT FORMAT
======================================================================

Return a concise report in this exact structure:

# SECTION 9A FINAL IMPLEMENTATION REPORT

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

## BASELINE
Pre-9A routes:
Pre-9A migration tip:

## IMPLEMENTATION
Menus:
Menu sections:
Food items:
Variants:
Modifier groups:
Modifiers:
Item/modifier relationships:
Media reference support:
Dietary/allergen support:
Capability enforcement:
Tenant/branch isolation:
IAM:
Audit:
Outbox:
Idempotency:
Concurrency:

## DATABASE
New migrations:
New tables:
Important constraints/indexes:

## API
New public routes:
New internal routes:
Post-9A total routes:
OpenAPI parity:

## TESTS
Targeted functional:
PostgreSQL:
Redis:
IAM/security:
Nested-IDOR:
Concurrency:
Migration/replay:
Route/OpenAPI parity:

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

## DEFERRED CORRECTLY TO 9B
List the exact 9B items that remain unimplemented.

## DEFERRED CORRECTLY TO 9C
List the exact 9C items that remain unimplemented.

## DOCUMENTATION UPDATED
List files.

## FINAL VERDICT
SECTION 9A FOOD CATALOGUE FOUNDATION:
GO / NO-GO

REMAINING 9A CODE DEFECTS:
NONE or list them.

Confirm explicitly:

- No commit
- No stage
- No merge
- No branch created
- No branch switch
- No push
- Section 9B not started
- Section 9C not started

======================================================================
60. START NOW
======================================================================

Begin with the fast baseline check.

Do not spend the session producing another plan.

Inspect only what you need to establish the current implementation pattern and migration/route baseline.

Then immediately implement Section 9A end-to-end.

Continue through code, migrations, tests, security, OpenAPI and documentation until the Section 9A acceptance criteria are satisfied or a genuine blocking dependency is proven.

If a test fails because of your Section 9A changes, fix it.

If you uncover a real pre-existing defect that directly blocks Section 9A:
- fix it only if the correction is narrow and clearly safe;
- add regression coverage;
- report it separately.

Do not expand into unrelated cleanup.

START SECTION 9A IMPLEMENTATION NOW.