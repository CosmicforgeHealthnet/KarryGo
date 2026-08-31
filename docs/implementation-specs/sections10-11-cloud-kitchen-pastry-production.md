KARRYGO — VENDOR SERVICE

MASSIVE ADVANCED IMPLEMENTATION
SECTIONS 10 + 11

SECTION 10 — CLOUD KITCHEN DOMAIN
SECTION 11 — PASTRY, BAKERY, CAKE AND SNACK DOMAIN

MODE:
DEVELOPMENT-DRIVEN
PRODUCTION-GRADE
ADVANCED OPERATIONAL IMPLEMENTATION
USAGE-SAVING

REPOSITORY:
C:\KarryGoPush

INTENDED BRANCH:
staging

======================================================================
0. EXECUTION DIRECTIVE
======================================================================

Implement Sections 10 and 11 together as one coherent production-grade backend development task.

SECTION 10:
CLOUD KITCHEN DOMAIN

SECTION 11:
PASTRY, BAKERY, CAKE AND SNACK DOMAIN

These two domains share production-capacity, scheduling, reservation and operational-calendar concepts.

Therefore:

BUILD ONE ADVANCED SHARED PRODUCTION ENGINE

and allow:

CLOUD_KITCHEN
and
PASTRY_BAKERY

to consume that shared engine through separate capacity pools and domain-specific workflows.

DO NOT create:

one capacity system for Cloud Kitchen

and another incompatible capacity system for Pastry.

At the same time:

PASTRY_BAKERY MUST REMAIN A FIRST-CLASS CAPABILITY.

Do not force pastry/cake semantics into Restaurant semantics merely because Section 9 Food primitives already exist.

This is NOT:

- a planning-only task
- a design review
- a gap-analysis-only task
- a documentation-only task
- a prototype
- frontend implementation

You must actually implement:

- database migrations
- production domain models
- repositories
- services
- concurrency control
- workers
- HTTP management APIs
- internal integration seams where justified
- IAM
- audit
- transactional outbox
- idempotency
- optimistic concurrency
- security
- tests
- OpenAPI
- route contracts
- documentation
- final Sections 10+11 certification

DO NOT stop after writing a plan.

Start implementation immediately after the fast baseline check.

======================================================================
1. USAGE-SAVING MODE
======================================================================

Use implementation time efficiently.

- Do not use subagents.
- Do not browse the web.
- Do not perform a whole-repository audit.
- Read this prompt once and treat it as authoritative.
- Re-read only specific sections if needed.
- Trust accepted Sections 5–9.
- Do not broadly recertify CRM.
- Do not broadly re-audit Food Section 9.
- Inspect only relevant Vendor Service packages and shared infrastructure.
- Reuse existing patterns.
- Reuse existing dependencies.
- Do not introduce unnecessary dependencies.
- Do not perform unrelated refactors.
- Keep progress commentary concise.
- Use targeted tests throughout development.
- Run the broad Sections 10+11 final gate only once after stabilization.
- Use isolated PostgreSQL databases/groups if the known shared-test DB contention appears.
- Do not start Section 12.
- Stop when Sections 10 and 11 meet the acceptance criteria.

======================================================================
2. GIT SAFETY — ABSOLUTE
======================================================================

Work in:

C:\KarryGoPush

Run only a fast baseline:

git branch --show-current
git status --short
git log -1 --oneline

DO NOT:

- commit
- stage
- push
- pull
- merge
- rebase
- cherry-pick
- create branch
- switch branch
- create worktree
- stash
- reset
- clean
- delete unrelated files
- restore unrelated files

The dirty working tree is authoritative.

Preserve all existing work.

Leave everything:

UNCOMMITTED
UNSTAGED

======================================================================
3. ACCEPTED BASELINE
======================================================================

Treat Sections 5 through 9 as accepted.

Known accepted state:

SECTION 9A:
GO

SECTION 9B:
GO

SECTION 9C:
GO

SECTION 9:
GO

Known route baseline after Section 9:

262 Vendor routes

Known migration tip:

000032_vendor_section9b_permissions

Section 9 provided:

- food catalogue
- menus
- sections
- items
- variants
- modifier groups
- modifiers
- media refs
- dietary/allergen metadata
- schedules
- sold-out/availability
- branch overrides
- immutable price history
- effective pricing
- effective sellability
- prep profiles
- logistics profiles
- catalogue revisions
- internal Food discovery contract

However:

THE CURRENT WORKING TREE IS AUTHORITATIVE.

Before allocation derive:

PRE_10_11_ROUTE_COUNT
PRE_10_11_MIGRATION_TIP

Do not blindly assume 262 / 000032 if newer changes exist.

======================================================================
4. TARGET ARCHITECTURE
======================================================================

Implement:

                         VENDOR BUSINESS
                               |
                             BRANCH
                               |
                  SHARED PRODUCTION PLATFORM
                               |
        +----------------------+----------------------+
        |                                             |
  CLOUD_KITCHEN                                  PASTRY_BAKERY
        |                                             |
 bulk packages                                  pastry products
 bulk capacity                                  pastry options
 production plans                               custom requests
 corporate plans                                custom quotes
 recurring production                           milestones
 production jobs                                cake snapshots
        |                                             |
        +----------------------+----------------------+
                               |
                       CAPACITY ENGINE
                               |
               capacity pools / rules / slots
                               |
            holds / confirmed reservations
                               |
                    production calendar

Shared infrastructure must support:

- branch-wide capacity
- capability-specific capacity
- production-line capacity
- item-family capacity
- standard pastry capacity
- custom cake capacity
- future extensibility

======================================================================
5. STRICT AUTHORITY BOUNDARIES
======================================================================

Vendor Service owns:

- production configuration
- production calendars
- production lines
- capacity rules
- production slots
- blackouts
- capacity reservations
- capacity utilization truth
- Cloud Kitchen bulk package configuration
- corporate production plans
- recurring occurrence materialization
- pastry products
- pastry options
- cake specifications
- custom cake quotes
- quote revisions
- Vendor-side accepted snapshots
- production workflow/milestones
- production jobs
- change/cancellation eligibility
- Vendor logistics handling metadata
- Vendor production readiness

Vendor Service does NOT own:

- canonical Customer identity
- canonical customer PII
- canonical Order authority
- payment
- Wallet
- settlement
- refunds
- canonical Inventory stock
- stock decrement
- ingredient stock
- delivery provider assignment
- route planning
- delivery fee
- logistics OTP/QR

Do not access another service database.

Do not create cross-service foreign keys.

======================================================================
6. ADVANCED SHARED PRODUCTION FOUNDATION
======================================================================

Implement an advanced shared production domain.

Core entities/concepts must include:

- production_capacity_pools
- capacity_rules
- production_lines
- production_slot_templates
- production_slots
- production_blackouts
- capacity_reservations
- reservation_allocations
- production_jobs
- production_job_events or equivalent history
- production_calendar revisions/state
- production capacity utilization queries

Use repository-consistent names.

Do not create unnecessary duplicate tables when one clean normalized model can serve the requirement.

======================================================================
7. PRODUCTION CAPACITY POOLS
======================================================================

Introduce explicit capacity pools.

A capacity pool represents one constrained production resource.

Examples:

CLOUD_KITCHEN:
HOT_MEALS_LINE_1
HOT_MEALS_LINE_2
BULK_PACKING
COLD_PREP

PASTRY:
STANDARD_BAKERY
CUSTOM_CAKES
DECORATION
CHILLING
PACKING

Capacity pools may be scoped:

BRANCH
CAPABILITY
PRODUCTION_LINE
ITEM_FAMILY
CUSTOM_CLASS

Fields should include conceptually:

id
business_id
branch_id
capability
name
code
scope_type
scope_ref
unit_type
status
version
created_at
updated_at
archived_at

Unit types should be controlled.

Examples:

MEAL
ITEM
TRAY
PACKAGE
PRODUCTION_UNIT

Do not use arbitrary float quantities.

======================================================================
8. PRODUCTION LINES
======================================================================

Implement reusable production-line definitions.

Examples:

Hot Kitchen Line A
Hot Kitchen Line B
Pastry Oven Line
Cake Decoration Line
Cold Room
Packaging Line

Support:

- business
- branch
- capability
- name/code
- production class
- status
- optional associated capacity pool
- operating calendar
- version
- timestamps

Production lines are configuration/resources.

Do NOT model individual staff members or machines unless required by an existing accepted specification.

======================================================================
9. CAPACITY RULES
======================================================================

Required original entity:

capacity_rules

Support:

- maximum quantity/work units
- effective window
- recurrence
- applicable capability
- capacity pool
- production line
- item family
- effective dates
- priority/specificity
- version
- lifecycle

Rules may overlap.

Resolution must be deterministic.

More specific limits must NOT accidentally bypass broader constraints.

Example:

branch max:
1000 meals

Cloud Kitchen max:
800 meals

Hot line max:
400 meals

Large order uses hot line:
all relevant constraints apply.

======================================================================
10. MULTI-POOL RESERVATIONS
======================================================================

Advanced orders/products may consume multiple capacity pools.

Example custom cake:

1 CUSTOM_CAKE production unit

+
1 DECORATION unit

+
1 CHILLING capacity slot

Support reservation allocation across multiple pools.

A reservation must succeed atomically across ALL required pools or fail entirely.

No partial reservation leakage.

This is critical.

======================================================================
11. PRODUCTION SLOT TEMPLATES
======================================================================

Support reusable slot templates.

Examples:

Weekdays:
08:00–10:00
10:00–12:00
12:00–14:00

Weekend:
09:00–13:00

Allow operational teams to generate/manage production windows without manually creating every slot forever.

Use:

- branch timezone
- weekdays
- local start/end
- capability/pool
- effective date range

Do not build arbitrary cron if unnecessary.

======================================================================
12. PRODUCTION SLOTS
======================================================================

Original required entity:

production_slots

Support:

OPEN
CLOSED
BLACKOUT
ARCHIVED

and conceptually:

start_at
end_at
capacity pool
production line
capability
optional capacity override
label
version

Use authoritative branch IANA timezone.

Persist timestamps consistently.

======================================================================
13. SLOT SETUP / CLEANUP BUFFERS
======================================================================

Add advanced setup/cleanup buffer support.

Example:

production:
10:00–11:00

cleanup:
15 minutes

next incompatible job cannot consume conflicting resource until 11:15.

Support bounded:

setup_buffer_minutes
cleanup_buffer_minutes

where relevant at:

pool
production line
package/product profile

Avoid overly complex scheduling algorithms.

Use deterministic effective reservation windows.

======================================================================
14. BLACKOUTS / EXCEPTION CALENDAR
======================================================================

Support blackout windows.

Reasons may include:

MAINTENANCE
STAFF_SHORTAGE
PRIVATE_PRODUCTION
HOLIDAY
EQUIPMENT_FAILURE
CLOSURE
OTHER

Use controlled reason code.

Blackouts can apply to:

branch
capability
pool
production line

Existing reservations must remain visible.

New conflicting reservations must fail after blackout commit.

======================================================================
15. CAPACITY RESERVATIONS
======================================================================

Original required entity:

capacity_reservations

Support:

HELD
CONFIRMED
RELEASED
CANCELLED
EXPIRED

Define exactly which consume capacity.

Expected:

HELD
CONFIRMED

consume capacity while valid.

Reservation fields include:

business
branch
capability
reservation window
quantity/workload
source_type
source_ref
status
hold_expires_at
priority if supported
version
timestamps

======================================================================
16. RESERVATION ALLOCATIONS
======================================================================

For multi-pool reservations store allocation detail.

Example:

reservation:
CUSTOM_CAKE-123

allocations:

CUSTOM_CAKE_POOL:
2 units

DECORATION_POOL:
1 unit

CHILLING_POOL:
1 slot

This allows capacity accounting to remain explainable.

======================================================================
17. SOFT HOLDS
======================================================================

Support temporary capacity holds.

Useful for:

- custom cake quote
- pending corporate confirmation
- future Order/RFQ integration

Hold must have:

expires_at

and MUST stop consuming capacity after expiry even if a cleanup worker has not run.

Correctness may not depend on worker timing.

======================================================================
18. CONFIRMED RESERVATIONS
======================================================================

Confirmed reservations consume capacity until:

- completed/released according to workflow
- cancelled
- production window passes according to defined rules

Do not silently auto-release future reservation evidence required for history.

======================================================================
19. RESERVATION RESCHEDULING
======================================================================

Advanced requirement:

support safe reservation rescheduling.

Rescheduling must:

1. check target capacity
2. obtain appropriate concurrency fences
3. reserve target
4. release/reassign old allocation
5. commit atomically

Never release old capacity first and then discover target capacity is unavailable.

Provide:

RESCHEDULE_CONFLICT
or equivalent error.

======================================================================
20. PARTIAL RELEASE
======================================================================

Support partial release where operationally valid.

Example:

corporate production originally:
100 meals

customer reduces quantity:
80

release:
20 units

Do not mutate history invisibly.

Record allocation adjustment/revision.

Do not allow release below already consumed/started production where business rules forbid it.

======================================================================
21. CAPACITY WAITLIST / REQUEST QUEUE
======================================================================

Advanced optional operational feature:

when requested capacity is unavailable, allow creation of a non-consuming capacity request/waitlist entry where product semantics permit.

This must NOT be treated as a confirmed reservation.

States conceptually:

WAITING
OFFERED
CONVERTED
CANCELLED
EXPIRED

Do not auto-convert without transactional recheck.

If this introduces excessive complexity relative to current architecture, implement only if it can remain clean and bounded.

======================================================================
22. CAPACITY CONCURRENCY — ABSOLUTE REQUIREMENT
======================================================================

Overbooking must be impossible under concurrent successful reservations.

Do NOT use an unsafe:

SELECT remaining
then INSERT

without locking.

Use an appropriate PostgreSQL strategy:

- row locking
- deterministic advisory locks
- serializable transaction + retry
- atomic capacity bucket accounting

Choose the safest repository-compatible implementation.

All applicable pools must be locked in deterministic order to reduce deadlock risk.

Example:

capacity:
100

simultaneous:
60
60

result:

only one may succeed.

======================================================================
23. DEADLOCK SAFETY
======================================================================

When a reservation uses multiple pools:

always acquire resource locks in deterministic order.

Example:

sort capacity pool IDs before locking.

Add deterministic tests for reversed allocation order.

Do not introduce preventable lock-order deadlocks.

======================================================================
24. CAPACITY PREVIEW
======================================================================

Implement preview.

Return conceptually:

requested quantity
requested interval

applicable pools:
- pool ref
- maximum
- reserved
- held
- remaining

available
blocking_rule
blocking_pool
blackout
lead_time_satisfied
evaluated_at

Preview does not guarantee reservation.

Actual mutation rechecks capacity.

======================================================================
25. CAPACITY UTILIZATION
======================================================================

Add operational read endpoints for capacity utilization.

Examples:

by:
day
slot
pool
capability

Return:

total capacity
held
confirmed
remaining
utilization percentage

Percentage should be computed safely from integer quantities.

Do not create financial analytics.

======================================================================
26. PRODUCTION JOBS
======================================================================

Add Vendor-owned production jobs/work records.

This is NOT the canonical customer Order.

A production job represents operational work tied to:

capacity reservation
corporate occurrence
custom cake
bulk package operation
future Order/RFQ source ref

States conceptually:

PLANNED
QUEUED
IN_PROGRESS
BLOCKED
READY
COMPLETED
CANCELLED

Use exact repository-style state vocabulary.

Production jobs allow kitchen staff to manage actual production without Vendor becoming Order authority.

======================================================================
27. PRODUCTION PRIORITY
======================================================================

Support bounded operational priority.

Example:

NORMAL
HIGH
URGENT

Do not create arbitrary numerical scheduler manipulation.

Priority must not bypass capacity.

======================================================================
28. PRODUCTION READINESS
======================================================================

Track:

scheduled_start
scheduled_end
actual_start
ready_at
completed_at

where appropriate.

Do not fabricate delivery ETA.

Production readiness is Vendor production truth only.

======================================================================
29. CLOUD_KITCHEN CAPABILITY
======================================================================

Cloud Kitchen requires:

CLOUD_KITCHEN

enabled on the branch.

It may coexist with:

RESTAURANT
FAST_FOOD
PASTRY_BAKERY

Do not treat every Restaurant as Cloud Kitchen.

======================================================================
30. SECTION 9 FOOD REUSE FOR CLOUD KITCHEN
======================================================================

Cloud Kitchen may reuse:

food menus
sections
items
variants
modifiers
media
pricing
dietary/allergen
prep
logistics metadata

But extend eligibility only where Section 10 specifically requires it.

Do not globally loosen every Section 9 capability gate.

Implement capability-aware reuse.

======================================================================
31. CLOUD KITCHEN SETTINGS
======================================================================

Support Cloud Kitchen operational settings.

At minimum:

bulk_order_threshold
minimum_lead_time_minutes
default_capacity_pool_ref
reservation_hold_duration
corporate_reservation_horizon_days

Apply sane limits.

Version settings.

Audit changes.

======================================================================
32. LEAD-TIME POLICY HIERARCHY
======================================================================

Support lead time at multiple levels where useful:

Cloud Kitchen default
→ package override
→ corporate-plan override
→ special/custom production requirement

Most specific configured rule wins.

Do not allow zero lead time to bypass production capacity/blackout.

======================================================================
33. BULK PACKAGES
======================================================================

Original entity:

bulk_packages

Support:

name
description
media refs where useful
capability
branch
status
minimum quantity
maximum quantity if applicable
production pool/class
lead time
logistics profile
version

Package composition should reference Section 9 food items.

Do not duplicate item records.

======================================================================
34. BULK PACKAGE COMPOSITION
======================================================================

Represent package composition explicitly.

Example:

Office Lunch:

1 Rice Meal
1 Protein Choice
1 Drink

or quantity-based composition.

Support:

item_ref
default quantity
optional variant/configuration constraints
position

Do not turn this into Order line items.

======================================================================
35. BULK PACKAGE VERSIONS
======================================================================

Advanced:

version bulk package composition and commercial configuration.

Future reservations/Orders must preserve which package definition applied.

Do not overwrite historical package composition silently.

======================================================================
36. QUANTITY BANDS
======================================================================

Support bands.

Example:

20–49
50–99
100–199
200+

Each band may define:

unit price
package price
lead time
capacity weight/profile

Rules:

no overlap
proper boundaries
one deterministic matching band

Support open-ended final band.

======================================================================
37. BULK PRICE HISTORY
======================================================================

Bulk package/band pricing must be immutable-version compatible.

Use integer minor units.

Do not recompute historical commercial truth from current bands.

Reuse Section 9 price-history principles where possible.

======================================================================
38. BULK RESERVATION PREVIEW
======================================================================

Allow Cloud Kitchen management to preview:

package
quantity
matched band
lead time
required pools
capacity
estimated production window

No payment.

No Order.

======================================================================
39. CORPORATE PLANS
======================================================================

Original entity:

corporate_plans

Reuse Section 8G corporate profile refs.

Do not copy PII.

A corporate plan supports:

corporate_profile_ref
branch
CLOUD_KITCHEN
name
package/menu configuration
quantity
schedule
production time
delivery-service metadata if applicable
lead time
capacity pools
activation state
effective date range
reservation horizon
version

======================================================================
40. CORPORATE PLAN STATES
======================================================================

Closed lifecycle conceptually:

DRAFT
ACTIVE
SUSPENDED
ARCHIVED

Do not generate occurrences from suspended/archived plans.

======================================================================
41. CORPORATE RECURRENCE
======================================================================

Support:

DAILY
WEEKLY
SELECTED_WEEKDAYS

Optional:

EVERY_N_WEEKS

if cleanly implementable.

Do not create arbitrary cron.

Use branch IANA timezone.

DST-safe.

======================================================================
42. CORPORATE EXCEPTION DATES
======================================================================

Advanced feature:

allow explicit occurrence exceptions.

Examples:

SKIP
RESCHEDULE
QUANTITY_OVERRIDE
PACKAGE_OVERRIDE

for one occurrence date.

Do not mutate the whole recurring plan for one exceptional day.

======================================================================
43. CORPORATE PAUSE / RESUME
======================================================================

Suspending a corporate plan must stop generation of new future occurrences.

Define behavior for already confirmed reservations.

Do not silently destroy them.

Allow controlled cancellation/release where authorized.

======================================================================
44. CORPORATE MATERIALIZATION
======================================================================

Worker creates future reservations within bounded horizon.

Must support:

leases
heartbeat
retry
dead-letter/replay
deterministic occurrence key
capacity checks
blackouts
lead time
idempotency

Do not materialize years of occurrences.

======================================================================
45. CORPORATE OCCURRENCE STATES
======================================================================

Track each occurrence independently.

Conceptually:

PLANNED
RESERVED
CAPACITY_CONFLICT
SKIPPED
CANCELLED
COMPLETED

Do not fabricate customer Order status.

======================================================================
46. CAPACITY CONFLICT MANAGEMENT
======================================================================

If occurrence cannot reserve:

record:

CAPACITY_CONFLICT

with reason.

Expose operational management visibility.

Allow retry/reschedule after capacity changes.

Do not overbook.

======================================================================
47. PASTRY_BAKERY — FIRST-CLASS DOMAIN
======================================================================

Pastry/Bakery must use:

PASTRY_BAKERY

capability.

It is not a Restaurant subtype.

It may share:

branch
Media
pricing primitives
production infrastructure
logistics metadata

but its catalogue/workflow remains first-class.

======================================================================
48. PASTRY PRODUCTS
======================================================================

Original entity:

pastry_products

Support product types:

CAKE
CUPCAKE
BREAD
DOUGHNUT
PIE
COOKIE
SMALL_CHOPS
SNACK
BOX
TRAY
PARTY_PACK
OTHER

Use bounded enum.

Fields include:

name
description
base_price_minor
currency
media refs
status
customizable
production_class
capacity_pool refs
prep profile
logistics profile
version

======================================================================
49. PASTRY CATALOGUE ORDERING
======================================================================

Add deterministic grouping/category/position semantics where necessary so the future consumer can render products predictably.

Do not build frontend discovery in this section unless an existing Vendor internal catalogue read contract can be safely extended.

======================================================================
50. PASTRY OPTION GROUPS
======================================================================

Original entity:

pastry_option_groups

Support:

FLAVOUR
SIZE
WEIGHT
LAYERS
FILLING
ICING
SERVING_COUNT
PACKAGING
SHAPE
COLOUR_SCHEME
DECORATION_STYLE

Do not treat all option values as arbitrary free text.

======================================================================
51. PASTRY OPTIONS
======================================================================

Options support:

name
code
position
price delta/override where appropriate
production workload impact
capacity class impact where appropriate
status
version

No negative pricing unless a future promotions engine explicitly owns it.

======================================================================
52. OPTION SELECTION RULES
======================================================================

Support:

min_select
max_select

and single-choice where required.

Examples:

SIZE:
exactly 1

FLAVOUR:
1–2

PACKAGING:
exactly 1

Validate combinations.

======================================================================
53. PRODUCT OPTION ATTACHMENTS
======================================================================

Option groups may be reusable across products.

Implement proper relationships.

Do not duplicate definitions.

Tenant isolation mandatory.

======================================================================
54. STANDARD PASTRY PRODUCTION
======================================================================

Standard pastry items can use batch-oriented production.

Advanced concept:

pastry_batches or shared production jobs with batch metadata.

Support:

planned quantity
produced quantity
waste quantity if product-safe
production line
slot
capacity reservation
status

Do not implement Inventory stock.

Finished quantity is production evidence, not inventory authority.

======================================================================
55. CUSTOM CAKE REQUESTS
======================================================================

Original entity:

custom_cake_requests

Fields include:

relationship/customer ref
corporate ref where applicable

occasion
required_at
serving_count
theme
colours
inscription
notes
reference media
selected options
production complexity
logistics requirements
status
version

No canonical PII duplication.

======================================================================
56. REQUEST LIFECYCLE
======================================================================

Use a precise closed lifecycle.

Recommended where no accepted vocabulary exists:

DRAFT
SUBMITTED
UNDER_REVIEW
QUOTE_PENDING
QUOTED
ACCEPTED_PENDING_ORDER
DECLINED
CANCELLED
EXPIRED
CONVERTED

CONVERTED requires a real external Order ref.

Do not falsely claim conversion otherwise.

======================================================================
57. CUSTOM CAKE SPECIFICATION VERSIONS
======================================================================

Every meaningful customer/vendor specification change creates immutable version evidence.

Version:

occasion
date/time
servings
theme
colours
inscription
options
media refs
notes
production requirements
logistics requirements

Never mutate what an old quote priced.

======================================================================
58. SPECIFICATION DIFF
======================================================================

Advanced feature:

provide a safe diff between cake specification versions.

Examples:

servings:
50 → 70

theme:
Blue → Gold

required date:
changed

reference media:
added

Do not generate verbose arbitrary diff code if existing deterministic JSON diff pattern exists; otherwise implement bounded field-level differences.

Useful for vendor approval and quote revision.

======================================================================
59. CUSTOM CAKE CHANGE REQUESTS
======================================================================

Advanced:

track requested changes explicitly rather than silently editing accepted specifications.

Concept:

cake_change_requests

Fields:

request ref
from specification version
requested changes
reason
status
requested_at
decision
resulting specification version

Useful after quote/acceptance.

Do not modify accepted immutable snapshot.

======================================================================
60. CUSTOM CAKE QUOTES
======================================================================

Original entity:

custom_cake_quotes

Support:

request
specification_version
quote revision
amount_minor
currency
expiry
vendor terms
capacity hold
production timing
status
version

No float money.

======================================================================
61. QUOTE STATES
======================================================================

Use deterministic lifecycle conceptually:

DRAFT
ISSUED
SUPERSEDED
ACCEPTED
DECLINED
EXPIRED
CANCELLED

Only one current active issued revision.

======================================================================
62. QUOTE REVISIONING
======================================================================

Never overwrite old quote commercial truth.

Revision:

1
2
3
...

Each revision binds to exact cake specification version.

======================================================================
63. QUOTE NEGOTIATION
======================================================================

Advanced Vendor-side feature:

support quote revision/negotiation history without implementing chat.

Record:

revision
reason code
safe notes
commercial changes
spec version

Do not copy customer PII.

======================================================================
64. QUOTE APPROVAL POLICY
======================================================================

Advanced:

allow configurable internal approval threshold for high-value quotes if compatible with IAM.

Example:

quotes above configured amount require Owner/GM approval before issuance.

Do not implement arbitrary approval workflow engine.

Keep bounded:

approval_required
approved_by
approved_at

Pricing authority must remain separate from production milestone authority.

======================================================================
65. QUOTE EXPIRY
======================================================================

Acceptance always rechecks expiry.

Do not depend solely on worker.

Expired quote cannot be newly accepted.

======================================================================
66. QUOTE CAPACITY HOLD
======================================================================

Quote may reserve capacity via shared HELD reservation.

Hold expires.

On quote revision:

old hold may need release/replacement atomically.

Do not double-hold capacity.

======================================================================
67. QUOTE ACCEPTANCE
======================================================================

On acceptance atomically establish Vendor-side truth:

- accepted quote revision
- exact accepted cake specification version
- immutable accepted specification snapshot
- confirmed capacity reservation
- production job/work shell where appropriate
- order-conversion intent
- audit
- outbox

No external network call inside locked capacity TX.

======================================================================
68. IMMUTABLE ACCEPTED CAKE SNAPSHOT
======================================================================

Store immutable snapshot including:

request ref
spec version
quote ref/revision

occasion
required_at
servings
theme
colours
inscription
options
media refs

amount_minor
currency
vendor terms

production requirements
capacity reservation
production pools

fragile
upright_required
temperature_class
cold_chain_required
package characteristics

accepted_at

Do not later recompute from mutable product/options.

======================================================================
69. ORDER CONVERSION BOUNDARY
======================================================================

If canonical Order authority is absent:

ORDER_AUTHORITY = NOT_PRESENT

CUSTOM_CAKE_ORDER_CONVERSION_SUPPORT =
PENDING_EXTERNAL_DOMAIN

Vendor publishes durable:

order_conversion.requested

or repository-equivalent outbox event.

Do not fabricate:

order
order item
paid status
payment transaction

======================================================================
70. FUTURE ORDER CONFIRMATION
======================================================================

If useful, implement a secured internal callback/confirmation seam for a future authoritative Order service only if this fits current architecture.

It may associate:

conversion_request_ref
external_order_ref

after authoritative confirmation.

Do NOT fake a consumer.

Use HMAC v2 and strict caller allowlist if implemented.

======================================================================
71. PRODUCTION MILESTONES
======================================================================

Original milestones:

DESIGN_APPROVED
BASE_PREPARED
DECORATION
CHILLING
PACKED
READY

Support additional bounded milestones if clearly useful:

PRODUCTION_STARTED
QUALITY_CHECK

but do not overcomplicate.

======================================================================
72. MILESTONE DEPENDENCIES
======================================================================

Define valid sequencing.

Example:

DESIGN_APPROVED
→ BASE_PREPARED
→ DECORATION
→ CHILLING
→ QUALITY_CHECK
→ PACKED
→ READY

Some products can skip non-applicable milestones according to explicit workflow configuration.

Do not permit arbitrary backwards transitions.

======================================================================
73. MILESTONE HISTORY
======================================================================

Never overwrite milestone history.

Record:

milestone
actor
timestamp
safe note
version
correlation

======================================================================
74. PRODUCTION RISK FLAGS
======================================================================

Advanced operational feature:

support bounded production risk flags.

Examples:

LATE_START
CAPACITY_AT_RISK
CUSTOMER_CHANGE_PENDING
MATERIAL_DEPENDENCY
QUALITY_REVIEW_REQUIRED

These are operational markers only.

Do not invent Inventory truth.

======================================================================
75. CHANGE CUTOFF POLICY
======================================================================

Support:

minutes_before_required_at
and/or
disallowed_after_milestone

Example:

no design change:
< 24h before required time

no structural change:
after BASE_PREPARED

no inscription change:
after DECORATION

Keep rules structured and bounded.

======================================================================
76. CHANGE ELIGIBILITY
======================================================================

Provide service method/API:

Can this specification still be changed?

Return:

allowed
reason_code
cutoff_at
blocking_milestone
evaluated_at

Do not rely on frontend.

======================================================================
77. CANCELLATION POLICY
======================================================================

Cancellation eligibility based on:

required time
production state
configured cancellation policy

Return conceptually:

ALLOWED
RESTRICTED
REQUIRES_REVIEW

No refund amount calculation.

No Wallet calls.

======================================================================
78. CANCELLATION POLICY TIERS
======================================================================

Advanced:

support bounded cancellation stages.

Example:

before production:
ALLOWED

after BASE_PREPARED:
REQUIRES_REVIEW

after DECORATION:
RESTRICTED

Do not create financial refund rules here.

======================================================================
79. PASTRY CAPACITY
======================================================================

Original requirement:

pastry_capacity

Implement using shared pools.

Support separate classes:

STANDARD
CUSTOM

Example:

STANDARD:
200 items/day

CUSTOM:
8 cake production units/day

Do not build a second reservation engine.

======================================================================
80. COMPLEXITY / WORKLOAD UNITS
======================================================================

Custom cake options may affect workload.

Use bounded integer workload units.

Example:

basic cake:
1

multi-tier:
2

complex fondant:
3

No arbitrary formula execution.

Capacity rules operate on workload units where configured.

======================================================================
81. LOGISTICS METADATA
======================================================================

Reuse Section 9 logistics vocabulary.

Support pastry requirements:

fragile
upright_required
temperature class
cold-chain requirement
package size
weight
spill risk where relevant
vehicle suitability hints

Do not assign actual vehicle.

======================================================================
82. LOGISTICS SNAPSHOT
======================================================================

Freeze logistics requirements into accepted cake snapshot.

Future logistics must use immutable purchased specification where needed, not current product configuration.

======================================================================
83. CLOUD KITCHEN LOGISTICS
======================================================================

Bulk packages may define:

hot handling
cold handling
tray count
package size
large-load hint
vehicle suitability metadata

Still no provider assignment.

======================================================================
84. READINESS / PICKUP WINDOW
======================================================================

Advanced Vendor-side production feature:

allow production jobs to expose:

expected_ready_window_start
expected_ready_window_end

This is a production readiness window only.

Do NOT call it delivery ETA.

Do not include travel time.

======================================================================
85. IAM
======================================================================

Reuse Section 5 IAM.

Separate permission areas:

PRODUCTION CAPACITY

CLOUD KITCHEN COMMERCIAL

CORPORATE PLAN MANAGEMENT

PASTRY CATALOGUE

CUSTOM CAKE REQUEST

CUSTOM CAKE QUOTE

PASTRY PRODUCTION

Use repository naming style.

======================================================================
86. ROLE MATRIX
======================================================================

Owner:
full

General Manager:
business-wide according to scope

Branch Manager:
branch-limited management

Kitchen/Pastry role:
production jobs
milestones
capacity operational visibility
availability

Kitchen/Pastry must NOT automatically control high-value quote pricing.

CRM/Support:
may view/coordinate customer request where appropriate, without production-admin powers

Finance:
no production authority by default

Marketing:
no production/capacity authority

Analyst/Auditor:
read-only

deny by default.

======================================================================
87. HIGH-VALUE QUOTE SECURITY
======================================================================

If quote approval threshold is implemented:

only configured commercial approvers may approve.

Do not grant approval solely based on production role.

Use audit.

======================================================================
88. STEP-UP
======================================================================

Do not require step-up for routine:

milestone update
slot update
capacity operation

unless accepted IAM policy requires it.

If high-value quote approval qualifies as sensitive under current security architecture, use established step-up rather than inventing a new mechanism.

======================================================================
89. IDEMPOTENCY
======================================================================

Use existing Vendor idempotency.

Critical idempotent commands:

capacity reservation
reservation release
reservation reschedule
corporate occurrence materialization
quote acceptance
quote revision where command semantics require
milestone change where retry possible

Repeated command must not duplicate state/evidence.

======================================================================
90. OPTIMISTIC CONCURRENCY
======================================================================

Use expected-version for:

capacity pools
capacity rules
production lines
slots
Cloud Kitchen settings
bulk packages
corporate plans
pastry products
option groups
custom requests
quotes
production jobs
change requests

Do not silently overwrite.

======================================================================
91. AUDIT
======================================================================

Audit all material mutations.

Include:

capacity changes
slot changes
blackouts
reservation actions
reschedules
partial releases
bulk package edits
corporate plans
exceptions
pastry products
custom specs
quotes
approvals
acceptance
milestones
cutoff decisions/actions
cancellation decisions/actions

Do not log PII or secrets.

======================================================================
92. TRANSACTIONAL OUTBOX
======================================================================

Use existing outbox.

State + outbox commit together.

Events may include:

production.capacity.changed
production.slot.changed
production.reservation.created
production.reservation.confirmed
production.reservation.rescheduled
production.reservation.released
production.job.changed

cloud_kitchen.bulk_package.changed
cloud_kitchen.corporate_plan.changed
cloud_kitchen.occurrence.reserved
cloud_kitchen.occurrence.capacity_conflict

pastry.product.changed
pastry.custom_request.submitted
pastry.specification.revised
pastry.quote.issued
pastry.quote.revised
pastry.quote.accepted
pastry.production.milestone_changed
pastry.change_requested

order_conversion.requested

Do not create meaningless event noise.

======================================================================
93. NO NETWORK INSIDE CAPACITY TRANSACTION
======================================================================

Absolutely no synchronous external call while capacity locks are held.

Correct flow:

BEGIN

validate
lock capacity resources
reserve/release
update aggregate
audit
outbox

COMMIT

external processing later

======================================================================
94. WORKER — CORPORATE OCCURRENCES
======================================================================

Implement robust worker:

lease
heartbeat
bounded batch
retry
dead-letter
replay
idempotency

Two workers must not duplicate occurrence.

======================================================================
95. WORKER — EXPIRED HOLDS
======================================================================

Optional cleanup worker may transition expired holds.

But effective capacity calculation MUST ignore expired holds based on authoritative time even before worker cleanup.

======================================================================
96. WORKER — QUOTE EXPIRY
======================================================================

Optional worker may reconcile expired quote state.

Acceptance still checks expiry transactionally.

======================================================================
97. WORKER — PRODUCTION ALERTS
======================================================================

Advanced internal operational worker may generate durable events/tasks for:

production starting soon
capacity conflict
late production job
custom cake change awaiting review

Reuse existing architecture.

Do not send notifications directly if Notification ownership lies elsewhere.

Generate outbox/event/task only.

======================================================================
98. HTTP — SHARED PRODUCTION
======================================================================

Build coherent management routes.

Likely:

/api/v1/vendor/production/capacity-pools

/api/v1/vendor/production/capacity-rules

/api/v1/vendor/production/lines

/api/v1/vendor/production/slot-templates

/api/v1/vendor/production/slots

/api/v1/vendor/production/blackouts

/api/v1/vendor/production/capacity/preview

/api/v1/vendor/production/capacity/utilization

/api/v1/vendor/production/reservations

/api/v1/vendor/production/reservations/{id}

/api/v1/vendor/production/reservations/{id}/confirm

/api/v1/vendor/production/reservations/{id}/release

/api/v1/vendor/production/reservations/{id}/reschedule

/api/v1/vendor/production/jobs

/api/v1/vendor/production/jobs/{id}

/api/v1/vendor/production/jobs/{id}/start

/api/v1/vendor/production/jobs/{id}/ready

/api/v1/vendor/production/jobs/{id}/complete

Adapt to existing route conventions.

Do not create vanity endpoints.

======================================================================
99. HTTP — CLOUD KITCHEN
======================================================================

Likely:

/api/v1/vendor/cloud-kitchen/settings

/api/v1/vendor/cloud-kitchen/bulk-packages

/api/v1/vendor/cloud-kitchen/bulk-packages/{id}

/api/v1/vendor/cloud-kitchen/bulk-packages/{id}/bands

/api/v1/vendor/cloud-kitchen/bulk-packages/{id}/capacity-preview

/api/v1/vendor/cloud-kitchen/corporate-plans

/api/v1/vendor/cloud-kitchen/corporate-plans/{id}

/api/v1/vendor/cloud-kitchen/corporate-plans/{id}/activate

/api/v1/vendor/cloud-kitchen/corporate-plans/{id}/suspend

/api/v1/vendor/cloud-kitchen/corporate-plans/{id}/archive

/api/v1/vendor/cloud-kitchen/corporate-plans/{id}/occurrences

/api/v1/vendor/cloud-kitchen/corporate-plans/{id}/exceptions

======================================================================
100. HTTP — PASTRY
======================================================================

Likely:

/api/v1/vendor/pastry/products

/api/v1/vendor/pastry/products/{id}

/api/v1/vendor/pastry/option-groups

/api/v1/vendor/pastry/option-groups/{id}

/api/v1/vendor/pastry/option-groups/{id}/options

/api/v1/vendor/pastry/products/{id}/option-groups

/api/v1/vendor/pastry/custom-requests

/api/v1/vendor/pastry/custom-requests/{id}

/api/v1/vendor/pastry/custom-requests/{id}/submit

/api/v1/vendor/pastry/custom-requests/{id}/specifications

/api/v1/vendor/pastry/custom-requests/{id}/changes

/api/v1/vendor/pastry/custom-requests/{id}/quotes

/api/v1/vendor/pastry/quotes/{id}

/api/v1/vendor/pastry/quotes/{id}/revise

/api/v1/vendor/pastry/quotes/{id}/approve

/api/v1/vendor/pastry/quotes/{id}/accept

/api/v1/vendor/pastry/quotes/{id}/decline

/api/v1/vendor/pastry/custom-requests/{id}/milestones

/api/v1/vendor/pastry/custom-requests/{id}/change-eligibility

/api/v1/vendor/pastry/custom-requests/{id}/cancellation-eligibility

Use actual architecture to finalize route shapes.

======================================================================
101. INTERNAL INTEGRATION CONTRACT
======================================================================

Do not fabricate consumer services.

If useful, expose only minimal HMAC v2 internal contracts for future:

Order/RFQ capacity reservation
Order conversion confirmation
production readiness lookup

Only if architecture genuinely benefits.

Internal contract ≠ external service implemented.

Document consumer support accurately.

======================================================================
102. PRIVACY
======================================================================

No canonical customer PII.

Use:

CRM relationship refs
corporate profile refs
customer refs

Custom cake notes/theme may contain user-supplied content, so:

- bound lengths
- treat as untrusted text
- do not copy into logs
- do not use for executable/query content

======================================================================
103. DATABASE CONSTRAINTS
======================================================================

Use PostgreSQL constraints for:

positive capacity
positive quantities
valid unit
valid status
valid capability
end > start
quote expiry
non-negative money
unique occurrence
unique quote revision
unique specification revision
tenant composite ownership
valid milestone ordering support
capacity hold timestamps

Defense in depth.

======================================================================
104. INDEXING
======================================================================

Indexes must support:

capacity by pool/window
active reservations by time
reservation source
slot by branch/time
blackouts
production jobs by state/time
corporate occurrences
bulk package branch
pastry products
custom request
spec versions
quotes
milestones
hold expiry
production readiness

Do not over-index.

======================================================================
105. CAPACITY CONCURRENCY TESTS
======================================================================

Mandatory:

capacity 100

concurrent:
60 + 60

exactly one success.

Also:

40 + 60
both succeed.

100 + 1
second denied.

Multiple independent pools:
no false conflict.

Multi-pool reservation:
one exhausted pool causes complete rollback.

Lock order:
reversed requested allocations do not deadlock.

Blackout vs reservation race.

Release vs reservation race.

Reschedule vs competing reservation.

Expired hold vs reservation.

======================================================================
106. QUOTE CONCURRENCY TESTS
======================================================================

Two simultaneous accepts:

one accepted snapshot
one confirmed reservation
one conversion intent
one state transition

No duplicates.

======================================================================
107. CORPORATE WORKER CONCURRENCY
======================================================================

Two worker instances:

same plan occurrence

must create:

one occurrence
one reservation

not duplicates.

======================================================================
108. PRODUCTION JOB TESTS
======================================================================

Test:

planned
start
blocked
resume if implemented
ready
complete
cancel

Invalid transitions refused.

Production state must not become delivery state.

======================================================================
109. CLOUD KITCHEN TESTS
======================================================================

Test:

capability
settings
lead time
bulk threshold
package
composition
bands
band boundaries
price history
capacity preview
capacity reservation
calendar
corporate recurrence
exception skip
exception reschedule
pause/resume
capacity conflict
future materialization
duplicate prevention
DST

======================================================================
110. PASTRY TESTS
======================================================================

Test:

products
types
media refs
option groups
options
attachments
selection rules
capacity classes
standard production
custom request
spec revisions
spec diff
change request
quotes
quote revisions
approval
expiry
capacity hold
acceptance
snapshot
milestones
risk flags
change cutoff
cancellation eligibility
logistics snapshot

======================================================================
111. ORDER ABSENCE TEST
======================================================================

If no Order authority:

accepting quote MUST NOT create:

order
paid order
payment
Wallet transaction

Must produce:

accepted snapshot
confirmed reservation
conversion intent

======================================================================
112. WALLET ABSENCE TEST
======================================================================

Assert no Wallet call for:

quote
bulk package
corporate plan
reservation
acceptance

======================================================================
113. LOGISTICS BOUNDARY TEST
======================================================================

Persist/expose:

fragile
upright
cold
weight
size
vehicle hints

Assert no:

Dispatch
Taxi
Hauling

execution.

======================================================================
114. SECURITY TESTS
======================================================================

Test:

cross-business
cross-branch
cross-capability
foreign pool
foreign line
foreign slot
foreign reservation
foreign package
foreign corporate plan
foreign corporate profile relation
foreign pastry product
foreign request
foreign spec
foreign quote
foreign milestone
foreign media ref where supported

No tenant leakage.

======================================================================
115. IAM TESTS
======================================================================

Owner:
full

GM:
scoped full management

Branch Manager:
own branch only

Kitchen/Pastry:
production operations but no unauthorized quote pricing

CRM/Support:
only allowed customer-workflow permissions

Finance:
no production authority by default

Analyst/Auditor:
read-only

Missing auth:
denied

======================================================================
116. PERFORMANCE
======================================================================

Moderate realistic fixture.

Include:

several branches
multiple capacity pools
multiple lines
hundreds of reservations
many production slots
dozens of corporate plans
future occurrences
hundreds of pastry products
custom requests
quotes
milestones

Check:

capacity calculation
calendar
worker batching
quote reads
occurrence reads
production queues

No obvious N+1.

======================================================================
117. MIGRATIONS
======================================================================

Determine actual current tip.

If still 000032, next free starts at:

000033

but derive actual state.

Prefer coherent migrations, perhaps:

shared production foundation

Cloud Kitchen

Pastry/Bakery

permissions

Do not force this split if a cleaner repository-consistent structure exists.

Do not create excessive tiny migrations.

======================================================================
118. OPENAPI
======================================================================

Prefer separate contracts:

section10-cloud-kitchen.openapi.json

section11-pastry-bakery.openapi.json

and shared production contract if project structure justifies it.

Every production route documented.

Every documented route exists.

======================================================================
119. GLOBAL ROUTE CONTRACT
======================================================================

Preserve Section 8I global route drift guard.

Add all new routes.

No internal route under public root.

No undocumented route.

No phantom route.

======================================================================
120. API ROUTE REGISTER
======================================================================

Update:

services/vendor-service/docs/API_ROUTE_REGISTER.md

Classify:

SHARED PRODUCTION

SECTION 10

SECTION 11

clearly.

======================================================================
121. IMPLEMENTATION STATUS
======================================================================

Update:

IMPLEMENTATION_STATUS.md

On successful completion:

SECTION 10 — CLOUD KITCHEN
IMPLEMENTED + TESTED + SECURITY-CERTIFIED

SECTION 11 — PASTRY/BAKERY
IMPLEMENTED + TESTED + SECURITY-CERTIFIED

Section 12:
NOT STARTED

======================================================================
122. PLATFORM CHANGE REGISTER
======================================================================

Document actual truth.

Expected if current architecture unchanged:

ORDER_AUTHORITY = NOT_PRESENT

CUSTOM_CAKE_ORDER_CONVERSION_SUPPORT =
PENDING_EXTERNAL_DOMAIN

INVENTORY_AUTHORITY =
NOT_PRESENT / OUTSIDE CURRENT SCOPE

MEDIA =
existing support reused

WALLET =
no change

LOGISTICS_EXECUTION =
not implemented

CUSTOMER =
no canonical PII copied

CRM =
existing corporate/relationship refs reused

======================================================================
123. UI / FRONTEND HANDOFF
======================================================================

Backend only.

Update handoff docs with backend capabilities for future UI:

Cloud Kitchen:

capacity dashboard
utilization
production calendar
slot template editor
blackouts
reservation board
production jobs
bulk package builder
quantity bands
corporate plans
recurrence exceptions
capacity conflicts

Pastry:

catalogue
options
cake request builder
spec version history
spec diff
quote revisions
quote approval
capacity hold
milestones
production risk
change eligibility
cancellation eligibility
fragile/upright/cold handling

Do not implement UI.

======================================================================
124. ADVANCED OPERATIONAL SAFETY
======================================================================

Protect against:

duplicate corporate reservation
duplicate quote acceptance
duplicate capacity consumption
capacity overbooking
stale quote acceptance
stale spec quote
quote against superseded specification
hold expiry race
reschedule race
blackout race
cross-pool deadlock
stale milestone mutation
late change after cutoff
cancellation after restricted milestone

Add regression tests.

======================================================================
125. FINAL CERTIFICATION ORDER
======================================================================

PHASE A
fast baseline

PHASE B
shared production schema

PHASE C
capacity engine

STOP and prove capacity concurrency before proceeding.

PHASE D
production lines / calendar / reservations

PHASE E
Cloud Kitchen settings + bulk

PHASE F
corporate recurrence + workers

PHASE G
pastry catalogue/options

PHASE H
custom cake specs/change history

PHASE I
quotes + holds + acceptance

PHASE J
production jobs/milestones/cutoffs

PHASE K
IAM/security

PHASE L
OpenAPI/docs

PHASE M
final combined certification

======================================================================
126. ABSOLUTELY DO NOT IMPLEMENT
======================================================================

Do NOT build:

frontend
Section 12
customer mobile UI
cart
checkout
Order tables
fake RFQ authority
payment
Wallet movement
settlement
refund execution
Inventory stock
ingredient stock
stock reservation
Dispatch mission
Taxi mission
Hauling mission
delivery OTP/QR
delivery routing
delivery fee
travel ETA
dynamic pricing
coupon engine
promotion engine
AI scheduling
AI pricing
AI cake design
unbounded workflow/rules engine

======================================================================
127. SECTION 10 FINAL ACCEPTANCE
======================================================================

Section 10 GO requires at minimum:

- CLOUD_KITCHEN first-class capability
- Section 9 catalogue reuse
- production capacity pools
- production lines
- capacity rules
- slots
- blackouts
- concurrency-safe reservations
- multi-pool constraints
- preview
- utilization
- holds
- confirmation/release
- rescheduling
- lead time
- bulk packages
- quantity bands
- immutable commercial versioning
- corporate plans
- recurrence
- exception dates
- future materialization
- capacity conflicts
- production jobs
- IAM
- audit
- outbox
- worker safety
- IDOR
- concurrency
- OpenAPI
- no fake Orders/Wallet/logistics

======================================================================
128. SECTION 11 FINAL ACCEPTANCE
======================================================================

Section 11 GO requires at minimum:

- PASTRY_BAKERY first-class capability
- pastry products
- product types
- options
- reusable option groups
- standard/custom capacity
- custom cake requests
- immutable spec versions
- spec diff
- change requests
- reference media
- quotes
- quote revisioning
- optional approval
- expiry
- shared capacity holds
- concurrency-safe acceptance
- immutable accepted snapshot
- order-conversion intent
- production jobs
- milestones
- milestone history
- production risk
- change cutoff
- cancellation eligibility
- fragile metadata
- upright metadata
- cold-chain metadata
- logistics snapshot
- IAM
- audit
- outbox
- IDOR
- concurrency
- OpenAPI
- no fake Orders/Wallet/logistics

======================================================================
129. FINAL QUALITY GATE
======================================================================

Run once after stabilization:

migrations / fresh replay

shared production tests

Cloud Kitchen tests

corporate worker tests

Pastry tests

custom cake tests

quote tests

capacity concurrency tests

quote concurrency

IAM

IDOR

route/OpenAPI parity

global route contract

PostgreSQL integration

Redis if required

Then:

gofmt
go vet
go build
git diff --check

Use isolated DB test groups if shared DB contention occurs.

Do not burn usage rerunning accepted CRM suites.

======================================================================
130. FINAL REPORT
======================================================================

Return:

# SECTIONS 10 + 11 ADVANCED FINAL IMPLEMENTATION REPORT

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

Accepted Section 9:
Pre-10/11 routes:
Pre-10/11 migration tip:

## SHARED PRODUCTION PLATFORM

Capacity pools:
Capacity scopes:
Production lines:
Slot templates:
Slots:
Blackouts:
Setup/cleanup buffers:
Capacity rules:
Multi-pool reservations:
Holds:
Confirmed reservations:
Partial release:
Rescheduling:
Capacity preview:
Utilization:
Production jobs:
Concurrency model:
Deadlock prevention:
Audit:
Outbox:

## SECTION 10 — CLOUD KITCHEN

Capability:
Food catalogue reuse:
Settings:
Bulk threshold:
Lead time:
Bulk packages:
Package composition:
Quantity bands:
Price/version history:
Capacity integration:
Corporate plans:
Recurrence:
Exception dates:
Pause/resume:
Future materialization:
Occurrence states:
Capacity conflict handling:
Workers:
Production jobs:
Logistics metadata:

## SECTION 11 — PASTRY / BAKERY

Capability:
Products:
Product types:
Option groups:
Options:
Product-option relationships:
Standard production:
Custom cake requests:
Specification versions:
Specification diff:
Change requests:
Reference media:
Quotes:
Quote revisions:
Quote approval:
Quote expiry:
Capacity holds:
Quote acceptance:
Accepted snapshot:
Order-conversion intent:
Production jobs:
Milestones:
Milestone history:
Risk flags:
Change cutoff:
Change eligibility:
Cancellation policy:
Pastry capacity:
Logistics metadata:
Logistics snapshot:

## ORDER / PAYMENT BOUNDARY

ORDER_AUTHORITY:
CUSTOM_CAKE_ORDER_CONVERSION_SUPPORT:
Real Order created:
Payment created:
Wallet called:
Conversion intent:

## DATABASE

New migrations:
Post migration tip:
New tables:
Important constraints:
Important indexes:

## IAM / SECURITY

Owner:
General Manager:
Branch Manager:
Kitchen/Pastry:
CRM/Support:
Finance:
Analyst/Auditor:
Quote pricing authority:
Tenant isolation:
Branch isolation:
Nested-IDOR:

## CONCURRENCY

Capacity 60+60/100:
Multi-pool atomicity:
Lock ordering:
Blackout race:
Release race:
Reschedule race:
Expired hold race:
Quote acceptance race:
Corporate occurrence duplication:
Optimistic concurrency:

## WORKERS

Corporate materialization:
Expired holds:
Quote expiry:
Operational alerts:
Leases:
Heartbeats:
Retries:
Dead-letter/replay:

## API

Pre routes:
Shared production public:
Shared production internal:
Section 10 public:
Section 10 internal:
Section 11 public:
Section 11 internal:
Post routes:
Total routes added:
OpenAPI parity:
Global route parity:

## TESTS

Migration:
Production:
Capacity:
Cloud Kitchen:
Bulk:
Corporate:
Recurrence:
Workers:
Pastry:
Custom Cake:
Quote:
Milestones:
IAM:
Security:
IDOR:
Concurrency:
PostgreSQL:
Redis:
Route/OpenAPI:

Failures:
Skips:

## BUILD QUALITY

gofmt:
go vet:
go build:
git diff --check:
Temporary artifacts:

## EXTERNAL SERVICE IMPACT

Media:
CRM:
Customer:
Order:
Inventory:
Wallet:
Notification:
Dispatch:
Taxi:
Hauling:

## DOCUMENTATION UPDATED

List files.

## FINAL CERTIFICATION

SECTION 10 — CLOUD KITCHEN:
GO / NO-GO

SECTION 11 — PASTRY, BAKERY, CAKE AND SNACK:
GO / NO-GO

REMAINING SECTION 10 CODE DEFECTS:
NONE or list.

REMAINING SECTION 11 CODE DEFECTS:
NONE or list.

EXTERNAL ACTIVATION DEPENDENCIES:
List only real dependencies.

Confirm:

- No commit
- No stage
- No merge
- No branch created
- No branch switch
- No push
- No frontend
- No fake Order authority
- No fake Inventory authority
- No payment/Wallet implementation
- No logistics execution
- Section 12 not started

======================================================================
131. START IMPLEMENTATION NOW
======================================================================

Begin with the fast baseline check.

DO NOT summarize this specification.

DO NOT produce another long plan.

Immediately inspect only:

- Section 9 Food catalogue/pricing/logistics primitives
- Section 6 capability model
- Section 8G corporate CRM refs
- IAM
- audit
- outbox
- idempotency
- worker infrastructure
- serviceauth if internal route is justified
- production router/global route contract

Then IMPLEMENT.

First:

SHARED PRODUCTION CAPACITY ENGINE.

Before moving forward, prove through deterministic PostgreSQL concurrency tests that:

CAPACITY = 100

parallel requests:
60
60

CANNOT both succeed.

Also prove multi-pool atomicity and deterministic lock ordering.

Only once that foundation is green continue:

Cloud Kitchen
→ bulk packages
→ corporate recurrence
→ pastry
→ custom cakes
→ quotes
→ production workflow
→ security
→ final certification.

Do not continue building high-level workflows on an unsafe capacity engine.

If a real existing defect directly blocks Sections 10/11:

fix it narrowly
add regression coverage
report it

Do not perform unrelated cleanup.

Continue until both Sections 10 and 11 are fully implemented, tested and certified or a genuine external authority dependency prevents a specific activation path.

START DEVELOPMENT NOW.