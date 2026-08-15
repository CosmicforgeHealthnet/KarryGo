KARRYGO VENDOR SERVICE + COMMERCE LOGISTICS

SECTION 8 — BUILT-IN VENDOR CRM

FINAL ORIGINAL-SPEC COMPLETION
SECTIONS 8.1–8.6 + FINAL ACCEPTANCE

PRODUCTION BACKEND IMPLEMENTATION TASK

======================================================================
0. MISSION
======================================================================

THIS IS A DEVELOPMENT TASK.

The objective is to FINISH SECTION 8.

Do not stop after:

- auditing
- describing remaining gaps
- creating schema
- writing TODOs
- adding interfaces
- adding inert tables
- preparing recommendations

Implement every remaining CRM-side requirement from the original Section 8
specification end-to-end.

When this task is complete, Section 8 may be reported COMPLETE only if every
remaining CRM-side requirement is genuinely implemented, tested, integrated
and documented.

External source authorities may remain unavailable.

Missing external producers DO NOT make the Vendor CRM consumer implementation
incomplete if:

- the consumer contract exists
- source authentication exists
- facts can be ingested
- projections work
- privacy/merge work
- Customer 360 works
- segmentation works where required
- tasks/triggers work when authoritative facts exist
- source absence fails closed as NOT_AVAILABLE
- no source truth is fabricated

DO NOT implement external domains merely to make CRM appear complete.

======================================================================
1. CLAUDE CONTEXT / USAGE-SAVING MODE
======================================================================

Follow root CLAUDE.md.

Historical session history exists at:

docs/SESSION_LOG.md

DO NOT read SESSION_LOG.md unless an unresolved architecture question genuinely
cannot be answered from:

- current code
- current migrations
- current documentation
- this specification

Usage rules:

- No subagents.
- No broad repository rediscovery.
- Read only files relevant to the current phase.
- Use targeted grep/search.
- Avoid repeatedly opening large files.
- Keep commentary concise.
- Do not browse the web.
- Do not perform unrelated research.
- Do not refactor unrelated working code.
- Reuse existing accepted Section 8 patterns.
- Use targeted tests during development.
- Redirect long test output to files.
- Inspect failures/tails instead of printing massive successful output.
- Run the full grouped regression only once at final certification.
- Stop when Section 8 reaches the acceptance gate.

This task is large.

Work sequentially through the phases below.

DO NOT restart architecture discovery after each phase.

======================================================================
2. REPOSITORY
======================================================================

Repository:

C:\KarryGoPush

Primary service:

services/vendor-service

Expected branch:

staging

Work directly in the existing working tree.

DO NOT:

- create a branch
- create a worktree
- clone
- switch branches
- commit
- stage
- merge
- cherry-pick
- rebase
- push
- reset
- stash
- clean
- delete unrelated files
- delete untracked files

Dirty working tree is expected.

Preserve all accepted work.

Leave everything uncommitted.

======================================================================
3. CONCURRENT-WRITER SAFETY
======================================================================

Before modifying code:

verify no other coding agent is currently writing to this same worktree.

If another implementation process is actively modifying Vendor Service:

STOP.

Report:

SECTION_8_FINAL_BLOCKED_CONCURRENT_WRITER

Do not allocate migrations concurrently.

======================================================================
4. AUTHORITATIVE CURRENT BASELINE
======================================================================

Expected baseline from the latest accepted completion report:

Branch:
staging

HEAD:
10ba439...

Vendor routes:
196

Vendor migration tip:
000027

Segment field registry:
30

Media DATA_EXPORT:
AVAILABLE

Section 8I:
GO

Loyalty tiers:
IMPLEMENTED

Section 9:
NOT STARTED

BUT:

derive all values from the actual repository.

Do not hard-code assumptions.

Record:

PRE_SECTION8_FINAL_ROUTE_COUNT

PRE_SECTION8_FINAL_MIGRATION_TIP

PRE_SECTION8_FINAL_SEGMENT_REGISTRY_COUNT

PRE_SECTION8_FINAL_HEAD

======================================================================
5. IMPORTANT EXISTING WORK
======================================================================

DO NOT rebuild accepted implementations.

Already accepted:

8A
CRM Foundation
Customer 360 foundation
Consent/privacy projections
commercial fact foundation
merge/redaction protections

8B
Tags
Advanced Segmentation Engine
safe rule DSL
materialized generations
dependency invalidation

8C
Interactions
Timeline
CRM Tasks

8D
Loyalty ledger
accounts
entries
lots
consumption
expiry
reversal
merge

8E
Campaign Intelligence
audience preparation
experiments

8F
Campaign execution
Notification integration
send-time privacy/consent enforcement

8G
Corporate Customer CRM

8H
CRM Intelligence
RFM
Churn heuristic
secure exports

8I
global router/OpenAPI drift guard
cross-module privacy certification
Notification handoff transaction correction
full regression certification

Later completion:
LOYALTY TIERS implemented and tested.

Preserve these.

Do not rewrite them unless required to integrate one of the remaining original
Section 8 requirements or a real defect is discovered.

======================================================================
6. PREVIOUS FINAL-COMPLETION REPORT — REMAINING FUNCTIONAL GAPS
======================================================================

The previous completion attempt reported these as NOT IMPLEMENTED:

1. full order_count/commercial relationship completion
2. frequently ordered categories
3. frequently ordered items
4. refund/complaint/service-recovery projection
5. related order/RFQ/support references on interactions/tasks
6. automatic CRM task trigger engine
7. customer value bands
8. corporate commercial projections
9. corporate RFQ projections
10. quote-conversion projection
11. relationship signals:
   - repeat
   - lapsed
   - high-value
   - at-risk
   - bulk-capable
   - corporate
   - recurring
12. authoritative event-ingress extensions
13. Customer 360 completion for these domains
14. segment registry/dependency expansion where required
15. privacy/merge/concurrency coverage for all new capabilities

Some schema for these may already exist in migrations 000026/000027.

DO NOT duplicate existing tables.

Audit and activate what already exists.

======================================================================
7. DEFINITION OF "SECTION 8 COMPLETE"
======================================================================

Section 8 is COMPLETE only when the Vendor CRM can safely support:

8.1 Customer 360
8.2 Tags and segmentation
8.3 Interactions and tasks
8.4 Loyalty and customer value
8.5 Corporate CRM
8.6 relationship intelligence/signals

including the original Section 8 requirements.

External producer absence may result in:

NOT_AVAILABLE

but not:

NOT_IMPLEMENTED.

There must be working code behind the CRM side.

======================================================================
8. AUTHORITY PRINCIPLE
======================================================================

Vendor CRM owns:

- Vendor-local commercial relationship projections
- CRM tags/segments
- interactions/tasks
- loyalty recognition
- customer value classification
- relationship signals
- corporate relationship projections
- Customer 360 presentation
- CRM automation decisions
- safe marketing targeting inputs

Vendor CRM DOES NOT own:

- canonical customer identity
- customer canonical PII
- Orders
- Checkout
- catalogue truth
- Wallet money movement
- formal refund transaction authority
- formal Support/Dispute lifecycle
- RFQ authority
- Notification transport
- logistics authority

NO CROSS-SERVICE DATABASE ACCESS.

======================================================================
9. SOURCE-AVAILABILITY PRINCIPLE
======================================================================

Every source-dependent projection must distinguish:

NOT_AVAILABLE

from:

AVAILABLE WITH ZERO OBSERVED FACTS.

Never fabricate:

0 orders

0 complaints

0 RFQs

false recurring

false bulk

false corporate

because the producer is absent.

Unknown is not false.

======================================================================
10. PHASE A — COMMERCIAL FACT COMPLETION
======================================================================

Finish the original 8.1 commercial model.

Required Customer 360 commercial output:

state

order_count

completed_order_count

net/total_spend_minor according to existing canonical definition

average_order_value_minor

first_order_at

last_order_at

last_branch_ref

source_as_of

Do not create duplicate spend measures if the existing Section 8A field already
represents canonical net spend.

======================================================================
11. ORDER_COUNT
======================================================================

Implement actual:

order_count

distinct from:

completed_order_count.

order_count:

number of authoritative commercial orders observed for the Vendor relationship
under the accepted commercial-fact contract.

completed_order_count:

qualifying completed orders only.

A cancelled order may increment order_count but not completed_order_count.

Test this explicitly.

======================================================================
12. AVERAGE ORDER VALUE
======================================================================

AOV:

canonical net completed commercial spend
/
completed_order_count

Use integer minor units.

No floating point.

If denominator is zero:

return NOT_AVAILABLE/null according to established response semantics.

Do not fabricate zero.

======================================================================
13. ORDER FACT CONTRACT
======================================================================

Extend the existing Section 8A authoritative Order fact contract.

Do not create a parallel commercial engine.

Required safe facts should support, where authoritative:

source_service

event_id

source_version

business_ref

CRM/customer relationship reference

order_ref

branch_ref

commercial outcome

currency

permitted commercial amount

occurred_at

completed_at

classification

line-level CRM projection facts.

Do not copy entire Order aggregates.

======================================================================
14. ORDER LINE RELATIONSHIP FACTS
======================================================================

Support bounded line facts necessary for CRM affinity.

At minimum:

order_ref

item_ref

category_ref where provided by authoritative Order snapshot

quantity

net_line_amount_minor where safely useful

No:

item description
media
current price
catalogue body

CRM stores opaque commercial references only.

======================================================================
15. CATEGORY AFFINITY
======================================================================

Implement frequently ordered categories.

Use existing schema if already present.

Otherwise add a proper derived projection.

Required logical data:

business

profile/customer relationship

branch scope

category_ref

completed_order_count

quantity_count where source supports it

net_spend_minor where useful

last_ordered_at

source_as_of

Projection must be rebuildable from authoritative facts.

======================================================================
16. ITEM AFFINITY
======================================================================

Implement frequently ordered items.

Required logical data:

business

profile

branch scope

item_ref

completed_order_count

quantity_count

net_spend_minor if permitted

last_ordered_at

source_as_of

No copied product PII/content.

======================================================================
17. AFFINITY RANKING
======================================================================

Ranking must be deterministic.

Preferred:

completed_order_count DESC

quantity_count DESC

last_ordered_at DESC

stable ref ASC

or an equivalent documented deterministic rule.

Customer 360:

top_categories

top_items

Default bounded top-N:

5 each.

Do not expose entire purchase history through Customer 360.

======================================================================
18. AFFINITY SOURCE UNAVAILABLE
======================================================================

Order producer unavailable:

affinities.state = NOT_AVAILABLE

not:

top_items = []
top_categories = []

as if authoritative observation exists.

======================================================================
19. SERVICE-RECOVERY FACTS
======================================================================

Implement authoritative CRM-safe service-recovery fact consumption.

Do NOT build Support/Dispute authority.

Support fact must carry only bounded relationship-level information.

Possible fields:

source_service

event_id

source_version

business_ref

profile/customer subject

branch_ref optional

support_case_ref

order_ref optional

case state

outcome code

refund classification

occurred_at

resolved_at

No complaint narratives.

No chat transcripts.

No attachments.

No customer PII.

======================================================================
20. SERVICE-RECOVERY PROJECTION
======================================================================

Implement Customer 360 service-recovery projection.

Required where source provides truth:

state

refund_count

partial_refund_count where meaningful

complaint_count

unresolved_complaint_count

resolved_complaint_count

latest_support_case_ref

latest_outcome_code

latest_refund_ref where appropriate

latest_refund_at

last_service_recovery_at

source_as_of

Use existing schema if already created.

======================================================================
21. SUPPORT AUTHORITY AUDIT
======================================================================

Perform only a TARGETED inspection of support-dispute-service.

Classify:

SUPPORT_RECOVERY_FACT_PRODUCER:

AVAILABLE
PARTIAL
NOT_PRESENT

If an existing authoritative Support domain can emit the CRM fact with a tiny
additive outbox change:

a minimal producer addition is allowed.

Do not implement Support/Dispute product scope.

If producer work is substantial:

leave producer NOT_PRESENT.

Vendor consumer must still be fully implemented.

======================================================================
22. ORDER PRODUCER AUDIT
======================================================================

Perform only a targeted inspection of the current Commerce/Order implementation.

Classify:

VENDOR_COMMERCE_ORDER_PUBLISHER:

AVAILABLE
PARTIAL
NOT_PRESENT

Do not implement Orders in Section 8.

If absent:

CRM remains complete on consumer side and reports NOT_AVAILABLE.

======================================================================
23. AUTHORITATIVE EVENT INGRESS
======================================================================

Reuse:

/internal/vendor/v1/crm/events

where this is the accepted shared CRM event boundary.

Extend its closed event-type registry.

Do not create unnecessary new internal routes.

Required security remains:

HMAC v2

caller allowlist

timestamp freshness

nonce

durable replay protection

event fingerprint

source version

tenant validation.

======================================================================
24. EVENT-TYPE AUTHORIZATION
======================================================================

Caller identity must be authorized PER EVENT TYPE.

Examples:

Order facts:
only approved commerce authority.

Support facts:
only support-dispute-service.

RFQ facts:
only approved RFQ authority.

Customer merge/privacy:
existing approved authority.

Notification Service must not submit Order facts.

Media Service must not submit Support facts.

Test spoofing.

======================================================================
25. DEDUPE / CORRECTION
======================================================================

Same event ID + same fingerprint:

safe replay.

Same event ID + different fingerprint:

conflict.

Version N+1 before N:

N must never overwrite N+1.

Corrections require increasing source version.

All projections must converge under replay.

======================================================================
26. RECOMPUTE, NOT FRAGILE COUNTERS
======================================================================

Authoritative facts are source input.

Derived commercial/affinity/recovery projections should be rebuildable.

Avoid irreversible:

counter++
counter--

when correction/replay/merge can invalidate them.

Required convergence under:

duplicates

corrections

refunds

reopens

out-of-order delivery

identity merge.

======================================================================
27. PHASE B — INTERACTION/TASK CONTEXT
======================================================================

Finish original 8.3 relationship context.

Extend 8C interactions/tasks with safe optional contextual references.

At minimum where applicable:

related_order_ref

related_rfq_ref

related_support_case_ref

Existing:

customer subject
corporate subject

must remain valid.

Do not store external aggregate bodies.

======================================================================
28. RELATED REF VALIDATION
======================================================================

Related refs are opaque references.

Validate:

- bounded format
- no secrets
- no PII
- subject/business scope
- exactly accepted contextual combinations

A manually supplied related ref is a reference, not proof that CRM owns the
external entity.

Automatic triggers should use authoritative source facts.

======================================================================
29. INTERACTION REVISION SUPPORT
======================================================================

Ensure interaction revision/history handling includes contextual refs correctly.

No revision may rewrite historical evidence incorrectly.

Existing append/revision semantics remain intact.

======================================================================
30. TASK HISTORY
======================================================================

Task history must preserve contextual refs needed to explain why a task exists.

Automatic task source should be distinguishable from staff-created task.

Do not expose internal source payload.

======================================================================
31. PHASE C — AUTOMATIC CRM TASK TRIGGER ENGINE
======================================================================

Implement the remaining automatic follow-up capability.

Required trigger families from the original Section 8 requirements:

UNRESOLVED_COMPLAINT

REJECTED_RFQ

HIGH_VALUE_RECOVERY

LAPSED_CUSTOMER

CORPORATE_LEAD

No AI.

No LLM.

No probabilistic automation.

Deterministic rules only.

======================================================================
32. TRIGGER ARCHITECTURE
======================================================================

Use durable deterministic evaluation.

The engine must survive:

process restart

duplicate source events

projection rebuild

worker retry

source correction

merge.

Do not rely on process-memory state.

Use existing worker/lease/retry/dead-letter infrastructure.

Do not create a second worker framework.

======================================================================
33. TRIGGER IDEMPOTENCY
======================================================================

A qualifying fact must not create duplicate open tasks on replay.

Use deterministic trigger identity.

Conceptually:

business
customer/corporate subject
trigger type
source fact/reference
trigger generation

Unique enough to prevent duplicate task creation.

======================================================================
34. TRIGGER LIFECYCLE
======================================================================

Support at least:

qualification

task creation

dedupe

resolution/no-longer-qualified handling

re-arm where appropriate.

Do not automatically delete historical tasks.

If source correction removes qualification:

existing task history remains evidence.

Future open-task behavior must be deterministic and documented.

======================================================================
35. UNRESOLVED COMPLAINT TRIGGER
======================================================================

When authoritative Support projection states an unresolved qualifying complaint:

create appropriate CRM follow-up task.

Do not require complaint narrative.

Task may reference:

support_case_ref

order_ref where available.

If Support source unavailable:

trigger state NOT_AVAILABLE.

No task fabricated.

======================================================================
36. REJECTED RFQ TRIGGER
======================================================================

When authoritative RFQ fact reports a rejected RFQ:

create a CRM follow-up task according to deterministic rule/configuration.

Task references:

rfq_ref

corporate/customer subject as appropriate.

No RFQ producer:

no fabricated trigger.

======================================================================
37. HIGH-VALUE RECOVERY TRIGGER
======================================================================

Trigger only when BOTH are authoritative/current:

customer classified as high-value

AND

qualifying negative service-recovery/refund outcome occurs.

Do not infer high value from one raw payment amount if Customer Value engine is
not current.

======================================================================
38. LAPSED CUSTOMER TRIGGER
======================================================================

Lapsed is time-dependent.

Use the relationship-signal engine defined later.

When:

LAPSED transitions false/unavailable -> true

and source freshness is sufficient:

create one follow-up task per configured qualification generation.

Because time can pass without a source event:

use the existing durable worker/scheduler infrastructure for bounded lapsed
evaluation.

No busy polling.

======================================================================
39. CORPORATE LEAD TRIGGER
======================================================================

Use existing 8G corporate lifecycle/pipeline semantics.

Do NOT invent a second corporate lifecycle.

Inspect the accepted Corporate CRM state model.

Map the existing lead/prospect state into the trigger.

If no existing lead state exists but the already-created completion schema
provides an explicit pipeline classification:

use that.

Trigger one deterministic follow-up task.

======================================================================
40. TRIGGER CONFIGURATION
======================================================================

Prefer business-scoped configuration if previous inert schema already includes
it.

If configuration does not exist:

introduce a minimal configuration model.

At minimum:

enabled

trigger type

optional due/offset policy

optional lapsed threshold reference

version

No arbitrary executable expressions.

Do not build a second general-purpose rules engine.

======================================================================
41. TRIGGER IAM
======================================================================

Use existing CRM task permissions for generated task visibility.

If trigger configuration needs management:

add the smallest coherent permission.

Suggested only if necessary:

vendor.crm.automation.read

vendor.crm.automation.manage

Do not grant automatically to every role.

Owner/GM:
manage

CRM/Support:
appropriate manage/read

Marketing:
read or limited according to current policy

Analyst/Auditor:
read only

Branch Manager:
branch-scoped only

======================================================================
42. PHASE D — CUSTOMER VALUE BANDS
======================================================================

Loyalty tiers already exist.

Do NOT rebuild them.

Now implement the separate original requirement:

CUSTOMER VALUE BAND.

Value band is commercial relationship classification.

It is NOT loyalty points.

It is NOT Wallet value.

It is NOT a marketing entitlement.

======================================================================
43. VALUE BAND MODEL
======================================================================

First inspect schema already created in 000026/000027.

Use it if valid.

Preferred architecture:

business-configured ordered bands.

Each band:

ref/id

business

name/label

rank

minimum qualifying commercial threshold(s)

status

version

created_at

archived_at where appropriate

Do not hard-code arbitrary marketing labels if configuration already allows
business-defined names.

======================================================================
44. VALUE BAND INPUTS
======================================================================

Use deterministic CRM-owned observed commercial metrics.

Allowed examples:

net spend

completed order count

possibly existing RFM/customer-commercial metrics

depending on previously designed schema.

Do not use:

Wallet balance

unsettled payment

credit score

ML prediction.

======================================================================
45. VALUE BAND AVAILABILITY
======================================================================

Commercial source unavailable:

value band:

NOT_AVAILABLE.

Commercial source current but customer does not qualify for configured band:

NO_BAND.

Qualified:

CURRENT.

Same honesty semantics as loyalty tier completion.

======================================================================
46. VALUE BAND STANDING
======================================================================

Prefer derived standing from current projection rather than stale stored truth.

Threshold change should be reflected deterministically.

If materialized for performance:

generation/version must protect stale results.

No background job should be necessary if read-time evaluation is bounded.

======================================================================
47. VALUE BAND CONCURRENCY
======================================================================

Business configuration replacement must be:

atomic

version checked

tenant-scoped

deterministic.

Use advisory lock or established configuration-lock pattern if appropriate.

No duplicate ranks/ambiguous thresholds.

======================================================================
48. CUSTOMER 360 VALUE OUTPUT
======================================================================

Customer 360 should expose:

customer_value:
  state
  band_ref
  band_name if safe/configured
  rank
  next_band where useful and safe
  evaluated_at/source_as_of

Do not confuse:

value band

with:

loyalty tier.

Expose separately.

======================================================================
49. PHASE E — CORPORATE COMMERCIAL CRM
======================================================================

Finish original 8.5.

8G already owns Corporate Customer CRM.

Extend it.

Do not create a second corporate CRM aggregate.

Required commercial projection support:

corporate order activity

bulk activity

recurring activity

RFQ history

quote conversion

commercial source availability

service relationship context.

======================================================================
50. CORPORATE ORDER PROJECTION
======================================================================

Use authoritative Order facts that identify a corporate relationship where the
future source supports it.

Required safe aggregates where source provides truth:

state

order_count

completed_order_count

bulk_order_count

recurring_order_count

net_spend_minor if permitted

first_order_at

last_order_at

last_branch_ref where appropriate

source_as_of.

No raw invoice/payment data.

======================================================================
51. CORPORATE RFQ FACT CONTRACT
======================================================================

Implement CRM-side versioned RFQ fact ingestion.

Conceptual facts:

source_service

event_id

source_version

business

corporate_profile

rfq_ref

branch where applicable

status/outcome

quote outcome

created_at

resolved_at

No RFQ body.

No attachments.

No private quote document.

No monetary settlement authority.

======================================================================
52. RFQ SOURCE STATUS
======================================================================

Audit only enough to classify:

RFQ_FACT_PRODUCER:

AVAILABLE
PARTIAL
NOT_PRESENT

Do not implement an RFQ service just to feed CRM.

Consumer remains functional and fail-closed.

======================================================================
53. CORPORATE RFQ PROJECTION
======================================================================

Customer/corporate CRM must derive where source exists:

rfq_count

open_rfq_count

accepted_quote_count

rejected_quote_count

expired/no-decision count where authoritative

last_rfq_at

last_rfq_ref

last_rfq_outcome

source_as_of

Do not fabricate zero when source absent.

======================================================================
54. QUOTE CONVERSION
======================================================================

Implement deterministic quote conversion.

Use explicitly documented denominator.

Preferred:

accepted_quote_count
/
eligible resolved quote/RFQ count

Do not include open RFQs in denominator.

Represent using integer basis points or rational counts.

No floating-point percentage persistence.

When denominator zero:

NOT_AVAILABLE.

======================================================================
55. CORPORATE SERVICE NOTES
======================================================================

Existing 8C interactions are the service-note authority inside CRM.

Do not create a duplicate `corporate_notes` table.

Corporate interactions/tasks must remain available through 8G/8C integration.

Where helpful, Customer 360/corporate timeline may include bounded references to
commercial/RFQ context.

======================================================================
56. CORPORATE BRANCH SECURITY
======================================================================

Corporate relationship can span branches according to 8G scope.

Branch-scoped actors must not infer unauthorized branch commercial activity.

Filter before aggregation.

Business-wide corporate actor may see permitted business-wide projection.

======================================================================
57. PHASE F — RELATIONSHIP SIGNAL ENGINE
======================================================================

Implement all seven original Section 8.6 relationship signals:

REPEAT

LAPSED

HIGH_VALUE

AT_RISK

BULK_CAPABLE

CORPORATE

RECURRING

Signals must be:

deterministic

explainable

availability-aware

privacy-safe

segmentable

versioned/as-of aware.

======================================================================
58. SIGNAL OUTPUT CONTRACT
======================================================================

Each signal should conceptually expose:

state

value when known

reason_code(s)

evaluated_at

source_as_of

version/generation where appropriate.

States should distinguish:

NOT_AVAILABLE

CURRENT

possibly STALE

according to existing intelligence conventions.

Do not represent unknown as false.

======================================================================
59. REPEAT SIGNAL
======================================================================

Derived from authoritative completed commercial orders.

Default rule if no previous schema says otherwise:

completed_order_count >= configurable repeat threshold.

Recommended default:

2

Use bounded business configuration if schema supports it.

Explainable reason:

COMPLETED_ORDER_THRESHOLD_MET.

======================================================================
60. LAPSED SIGNAL
======================================================================

Requires:

authoritative commercial source current

last qualifying completed order

business-configured lapsed duration.

Example:

last_order_at older than N days.

Do not use service process current time without consistent business timezone/as-of
semantics.

Persist/evaluate using UTC instant.

Scheduler should refresh time-dependent transitions.

======================================================================
61. HIGH_VALUE SIGNAL
======================================================================

Use Customer Value classification.

Preferred:

TRUE when customer is in one of business-configured high-value band ranks.

Do not create a second high-value calculation independent of Customer Value.

If Customer Value NOT_AVAILABLE:

HIGH_VALUE = NOT_AVAILABLE.

======================================================================
62. AT_RISK SIGNAL
======================================================================

Reuse accepted CHURN_V1 intelligence.

Do not create a second churn model.

Use deterministic configurable threshold over the existing 0–100 heuristic.

Example default if no existing config:

70

but use existing schema/configuration if already defined.

CHURN not current:

AT_RISK = NOT_AVAILABLE.

No probability language.

======================================================================
63. BULK_CAPABLE SIGNAL
======================================================================

Do NOT infer bulk capability merely from high spend.

TRUE only from authoritative evidence such as:

- observed order classified BULK
- corporate commercial configuration explicitly allowing bulk
- another approved authoritative bulk classification

depending on actual existing schema.

No source:

NOT_AVAILABLE.

Reason codes must explain evidence.

======================================================================
64. CORPORATE SIGNAL
======================================================================

TRUE when CRM relationship is authoritatively linked to an active permitted
corporate relationship from 8G.

FALSE only when CRM has sufficient authoritative relationship state to know
there is no current corporate link.

Otherwise:

NOT_AVAILABLE.

Do not heuristically identify companies from email/domain/name.

======================================================================
65. RECURRING SIGNAL
======================================================================

Do NOT infer recurring merely because multiple orders look similar.

TRUE only when authoritative commerce facts classify/link recurring commerce
or an approved corporate recurring relationship exists.

No source:

NOT_AVAILABLE.

Future recurring basket intelligence may consume Order authority later.

No heuristic recurring-basket guess in Section 8.

======================================================================
66. RELATIONSHIP SIGNAL STORAGE
======================================================================

Use already-created inert schema if present.

Otherwise choose the simplest robust model:

materialized current signal row per:

business
profile
signal_type
scope

with:

state
value
reason codes
generation/version
as_of

or an equivalent normalized structure.

Do not create seven unrelated tables unless justified.

======================================================================
67. SIGNAL RECOMPUTATION
======================================================================

Signal changes must be driven by dependencies.

Examples:

Order fact:
repeat
lapsed
bulk
recurring

Customer Value:
high-value

Churn:
at-risk

Corporate relationship:
corporate

RFQ/support changes:
task triggers, but not arbitrary signals unless defined.

Recompute only affected dependencies.

Do not rebuild all customers for every event.

======================================================================
68. TIME-DEPENDENT SIGNAL WORKER
======================================================================

LAPSED changes as time passes.

Use existing worker infrastructure.

Requirements:

leases

heartbeat

bounded batch

retry

dead-letter/replay where existing infrastructure provides it

SKIP LOCKED or established equivalent

no duplicate evaluation effects

No new worker framework.

======================================================================
69. SIGNAL PRIVACY
======================================================================

Redacted relationship:

signals non-actionable/not exposed.

Late facts:

must not resurrect signals/presentation.

Suppressed marketing customer:

signals may remain internally valid if lawful, but campaign eligibility still
obeys consent/suppression.

Do not delete lawful commercial evidence merely because marketing is suppressed.

======================================================================
70. PHASE G — SEGMENT ENGINE EXPANSION
======================================================================

Extend the accepted Section 8B field registry only for fully implemented safe
fields.

Do not expose schema-only fields.

Derive actual registry count.

Potential new segment fields include, where type system supports them:

commercial.order_count

service_recovery.complaint_count

service_recovery.unresolved_complaint_count

customer_value.rank

relationship.repeat

relationship.lapsed

relationship.high_value

relationship.at_risk

relationship.bulk_capable

relationship.corporate

relationship.recurring

Use exact repository naming conventions.

======================================================================
71. SEGMENT TYPES
======================================================================

Use existing safe field types wherever possible:

INTEGER

MONEY_MINOR

BOOLEAN

ENUM

TIMESTAMP

etc.

Do not add raw SQL.

Do not add caller-supplied field names.

Do not allow arbitrary JSON path queries.

If customer-value band name is business-configured and unsafe as ENUM:

segment on:

rank

or stable band ref through a safe reference mechanism.

======================================================================
72. AVAILABILITY IN SEGMENTATION
======================================================================

NOT_AVAILABLE must not silently evaluate as:

0

false

empty.

Predicates over unavailable values must fail closed according to existing rule
engine semantics.

Example:

relationship.at_risk EQ false

must NOT include a customer whose churn source is unavailable.

======================================================================
73. SIGNAL SEGMENT INVALIDATION
======================================================================

Dependency changes must invalidate appropriate dynamic segments.

Examples:

Order fact:
commercial/order_count/repeat/lapsed/bulk/recurring

Support fact:
complaint fields

Customer Value:
value rank/high-value

Churn:
at-risk

Corporate link:
corporate.

Do not invalidate tag-only segments unnecessarily.

======================================================================
74. CURRENT GENERATION SAFETY
======================================================================

Section 8B materialization safety remains mandatory.

Signal/customer-value fields must only be evaluated from current certified state.

No stale intermediate generation becomes campaign audience truth.

======================================================================
75. PHASE H — CUSTOMER 360 FINAL COMPLETION
======================================================================

Final Customer 360 must expose safe bounded blocks for:

commercial

affinities

service_recovery

loyalty

loyalty_tier

customer_value

relationship_signals

interactions/tasks

corporate relationship context

existing intelligence where allowed.

Do not turn Customer 360 into an unbounded database dump.

======================================================================
76. CUSTOMER 360 COMMERCIAL
======================================================================

Required:

state

order_count

completed_order_count

canonical spend

AOV

first order

last order

last branch where permitted

source_as_of.

======================================================================
77. CUSTOMER 360 AFFINITIES
======================================================================

Required:

state

top_categories

top_items

bounded top-N.

No complete order history.

======================================================================
78. CUSTOMER 360 RECOVERY
======================================================================

Required:

state

refund_count

complaint_count

unresolved count

latest outcome

latest case/ref where safe

last recovery date.

No complaint text.

======================================================================
79. CUSTOMER 360 CUSTOMER VALUE
======================================================================

Expose separately from loyalty:

state

current band

rank

next band where useful

source_as_of.

No monetary entitlement implication.

======================================================================
80. CUSTOMER 360 RELATIONSHIP SIGNALS
======================================================================

Expose all seven signals with:

state

value

reason codes

as_of.

Do not expose sensitive raw underlying facts unnecessarily.

======================================================================
81. CORPORATE CUSTOMER 360
======================================================================

Existing corporate read surface should expose appropriate bounded:

commercial summary

RFQ summary

quote conversion

relationship/task context

source availability.

No canonical corporate-contact PII duplication.

======================================================================
82. PHASE I — PRIVACY COMPLETION
======================================================================

Every new capability must integrate with accepted 8A/8I privacy architecture.

Required:

redaction

suppression

merge

anti-resurrection

export exclusion

segment exclusion

campaign protection.

======================================================================
83. REDACTION
======================================================================

After redaction:

commercial relationship may retain lawful opaque evidence where policy allows

but actionable Customer 360 presentation is suppressed

affinities not exposed

recovery not exposed

value not actionable

signals not actionable

dynamic segment membership repaired

exports exclude profile

campaign cannot target.

Late Order/RFQ/Support facts cannot resurrect the customer.

======================================================================
84. MARKETING SUPPRESSION
======================================================================

Marketing suppression does NOT require destroying lawful:

commercial aggregates

interactions

loyalty ledger

corporate relationship

service-recovery evidence

customer-value intelligence.

But:

campaign targeting/handoff must remain suppressed.

Preserve accepted 8I distinction.

======================================================================
85. IDENTITY MERGE
======================================================================

On authoritative S -> T merge:

dedupe commercial Order facts

dedupe Support facts

dedupe RFQ facts

recompute affinities

recompute recovery

recompute Customer Value

recompute signals

transfer/reconcile tasks/interactions according to existing 8C semantics

corporate contacts/relationships remain safe

source becomes non-actionable.

Do NOT add aggregate counters blindly.

======================================================================
86. MERGE COLLISION EXAMPLE
======================================================================

Required test:

S:
Order A
Order B
Support X
RFQ R1

T:
Order B
Order C
Support X
Support Y
RFQ R1
RFQ R2

After merge:

Orders:
A/B/C counted once

Support:
X/Y counted once

RFQ:
R1/R2 counted once

affinity recomputed

value recomputed

signals recomputed

one canonical actionable target.

======================================================================
87. PRIVACY RACES
======================================================================

Use deterministic concurrency tests.

Required races:

Order fact vs REDACT

Support fact vs REDACT

RFQ fact vs REDACT

signal rebuild vs REDACT

task-trigger evaluation vs REDACT

merge vs Order fact

merge vs Support fact

merge vs RFQ fact.

No sleep-only race tests.

======================================================================
88. PHASE J — AUTOMATION / TASK SECURITY
======================================================================

Generated tasks must obey:

tenant scope

branch scope

subject scope

task permission rules

privacy status.

No generated task may expose another branch's Support/RFQ/Order reference.

======================================================================
89. GENERATED TASK CONTENT
======================================================================

Task title/body must be bounded and safe.

Do not copy:

complaint narrative

RFQ body

customer PII

payment information.

Use standardized generated context such as:

"Follow up on unresolved service issue"

with opaque related ref.

======================================================================
90. HUMAN OVERRIDE
======================================================================

Automatic task creation does not replace human CRM workflow.

Staff may:

complete
cancel
reassign

according to existing 8C permissions.

The trigger engine must not repeatedly recreate a completed task for the same
qualification unless a documented re-arm condition occurs.

======================================================================
91. PHASE K — CORPORATE PIPELINE
======================================================================

Original corporate CRM includes lead/commercial follow-up concepts.

Inspect 8G's existing lifecycle.

Do not create a conflicting pipeline vocabulary.

If existing lifecycle already has:

lead
prospect
active
archived

or equivalent:

reuse.

If inert schema from the completion migration already models a corporate
pipeline:

activate it.

No second competing state machine.

======================================================================
92. RFQ / QUOTE TASK CONTEXT
======================================================================

Rejected RFQ trigger must create a corporate/customer task with:

related_rfq_ref

subject

safe trigger reason

due policy

idempotent trigger identity.

No quote document or prices required in task.

======================================================================
93. PHASE L — EXPORTS
======================================================================

Review existing secure exports.

Only add new completion fields to export datasets if:

- appropriate
- privacy-safe
- within existing dataset semantics.

Do not automatically dump:

top item history

Support case refs

RFQ refs

trigger internals

into every export.

If exported:

use bounded, documented columns.

Step-up requirements remain.

======================================================================
94. NO PII
======================================================================

Scan new database/schema/DTO/events/logging.

Vendor CRM must not newly store canonical:

customer name

email

phone

full postal address

government ID

payment credential

Support transcript

RFQ attachment

Order delivery address.

Opaque refs only unless an already-approved safe presentation projection exists.

======================================================================
95. NO MONETARY AUTHORITY
======================================================================

Customer Value, RFQ projection, recovery and signals must never create:

wallet balances

credits

financial reward

settlement

refund transaction

payment transaction.

Loyalty remains points recognition only.

Customer Value is classification only.

======================================================================
96. NO AI / ML
======================================================================

Do not implement:

LLM
embeddings
vector search
ML scoring
predictive churn model
recommendation model.

CHURN_V1 remains the accepted deterministic heuristic.

Signals remain deterministic.

======================================================================
97. SCHEMA / MIGRATION POLICY
======================================================================

Actual current Vendor migration tip must be derived.

Expected:

000027

but do not assume.

Inspect 000026/000027 because the previous report says remaining schema may
already exist there.

DO NOT duplicate tables.

DO NOT edit accepted migrations merely to add runtime implementation.

If new schema is necessary:

use next sequential additive migration.

Possible conceptual migration:

<next>_vendor_crm_section8_final_completion

If new IAM permissions are needed:

<next+1>_vendor_section8_final_permissions

Do not create empty migrations.

======================================================================
98. MIGRATION QUALITY
======================================================================

Use:

primary keys

composite tenant FKs

source identity uniqueness

source version checks

non-negative counters/money

state CHECK constraints

indexes based on query shapes.

Down migration must remove only new additions.

Do not damage accepted 8A–8I schema.

======================================================================
99. OUTBOX / INBOX
======================================================================

Reuse accepted:

transactional inbox

transactional outbox

worker framework.

No synchronous external network calls while holding privacy/profile locks.

The 8I Notification transaction defect must not reappear.

======================================================================
100. EVENT PAYLOAD SAFETY
======================================================================

CRM events must be low-detail.

Do not publish complete:

purchase histories

Support histories

RFQ histories

customer value evidence

PII.

Use:

aggregate ref
business
branch
version
changed field family
as_of.

======================================================================
101. HTTP ROUTE PRINCIPLE
======================================================================

Prefer extending existing routes.

Likely:

Customer 360 existing read

corporate existing reads

interaction/task existing routes

CRM internal event ingress existing route.

Do not add route families simply because tables exist.

New public routes are allowed only where configuration management genuinely
needs them, such as:

customer-value band configuration

automation trigger configuration

if no appropriate existing route exists.

Derive actual route count.

======================================================================
102. ROUTER / OPENAPI GUARD
======================================================================

8I global drift guard must remain green.

Any new route:

must exist in production router

OpenAPI

API_ROUTE_REGISTER.

Any documented route:

must exist in production router.

Do not weaken the guard.

======================================================================
103. SEGMENT REGISTRY COUNT
======================================================================

Expected pre-count:

30.

Do not target an arbitrary final number.

Add only fully implemented safe fields.

Report:

PRE

ADDED

POST

and exact field names.

======================================================================
104. IAM
======================================================================

Reuse existing CRM permissions wherever possible.

Potential new permissions only if required:

vendor.crm.value.read

vendor.crm.value.manage

vendor.crm.automation.read

vendor.crm.automation.manage

vendor.crm.corporate.commercial.read

But first inspect current permission catalogue.

Do not introduce duplicates/synonyms.

======================================================================
105. ROLE MAPPINGS
======================================================================

Use accepted role families.

Owner:
appropriate full CRM management

General Manager:
appropriate full CRM management

CRM/Support:
CRM operational management

Marketing:
safe segmentation/campaign/value visibility where appropriate;
no Support sensitive detail

Branch Manager:
branch-scoped only

Analyst/Auditor:
read only

Finance:
no automatic CRM mutation authority

IT Support:
no business-decision authority merely for being technical.

No role-name authorization.

======================================================================
106. BRANCH SCOPE
======================================================================

Branch actors must not infer other branches through:

order count

spend

affinities

complaints

refunds

last branch

Customer Value

relationship signals

automatic tasks

RFQ activity.

FILTER BEFORE AGGREGATE.

Do not aggregate business-wide then hide a field afterward.

======================================================================
107. TENANT SCOPE
======================================================================

Every new table/repository query must protect:

business tenancy.

Test cross-business access using:

real IDs from another business

nested IDs

event-ingress spoofing

merge scenarios.

======================================================================
108. PERFORMANCE
======================================================================

Avoid N+1.

Customer 360 should use bounded/set-based queries.

Do not:

query one row per item affinity

query one row per category affinity

query one Support fact at a time

query each relationship signal independently if one bounded query works.

======================================================================
109. MODERATE PERFORMANCE FIXTURE
======================================================================

Use a moderate fixture only.

Suggested:

1,000 CRM profiles

5,000 commercial facts

15,000 compact line facts

1,000 Support facts

500 RFQ facts

multiple branches

multiple value configurations

relationship signals

trigger jobs/tasks.

Do not claim production throughput from this fixture.

======================================================================
110. PHASE M — TEST MATRIX: COMMERCIAL
======================================================================

Required:

first order

completed order

cancelled order

order_count distinct from completed count

multiple orders

duplicate event

same event/different body

source correction

stale version

refund correction

AOV

first/last order

last branch

branch scope

cross currency according to existing policy

redaction

merge

rebuild.

======================================================================
111. TEST MATRIX: AFFINITY
======================================================================

Required:

repeated item

multiple categories

quantity aggregation

net-spend aggregation if implemented

deterministic ranking

ties

top-N

branch scope

business scope

replay

correction

refund

merge dedupe

redaction

source unavailable.

======================================================================
112. TEST MATRIX: SERVICE RECOVERY
======================================================================

Required:

first complaint

unresolved

resolved

reopen where vocabulary supports it

refund-linked case

multiple cases

latest outcome

duplicate event

source correction

stale event

branch scope

tenant isolation

merge

redaction

source unavailable.

======================================================================
113. TEST MATRIX: INTERACTION/TASK REFS
======================================================================

Required:

related order

related RFQ

related Support case

revision/history

wrong subject

wrong business

branch escape

oversized/invalid ref

no external body stored.

======================================================================
114. TEST MATRIX: TRIGGERS
======================================================================

Each trigger family:

qualifies

creates exactly one task

duplicate input does not duplicate

correction behaves correctly

completed task not recreated improperly

privacy blocks task creation

branch scope correct

merge safe.

Test:

UNRESOLVED_COMPLAINT

REJECTED_RFQ

HIGH_VALUE_RECOVERY

LAPSED_CUSTOMER

CORPORATE_LEAD.

======================================================================
115. TEST MATRIX: CUSTOMER VALUE
======================================================================

Required:

no source -> NOT_AVAILABLE

source + no qualifying band -> NO_BAND

band qualification

boundary threshold

next band

configuration replacement

stale version conflict

duplicate rank/threshold rejection

commercial correction changes band

merge changes band

redaction

branch view.

======================================================================
116. TEST MATRIX: CORPORATE COMMERCIAL
======================================================================

Required:

corporate order fact

bulk fact

recurring fact

RFQ create/update

accepted quote

rejected quote

conversion denominator

zero denominator

stale source version

duplicate

branch scope

corporate archive/suppression behavior

merge/contact privacy.

======================================================================
117. TEST MATRIX: RELATIONSHIP SIGNALS
======================================================================

Required:

REPEAT true/false/unavailable

LAPSED true/false/unavailable

HIGH_VALUE true/false/unavailable

AT_RISK true/false/unavailable

BULK_CAPABLE true/false/unavailable

CORPORATE true/false/unavailable

RECURRING true/false/unavailable

reason codes

source_as_of

dependency invalidation

rebuild

redaction

merge.

======================================================================
118. TIME-BASED TESTS
======================================================================

Use injected clock/test clock where available.

Do not use long sleeps.

Lapsed worker tests must deterministically move clock.

Timezone/DST must not change elapsed-duration semantics.

Store UTC timestamps.

======================================================================
119. CONCURRENCY TESTS
======================================================================

Use real PostgreSQL locks/barriers.

Representative required races:

Order correction vs Order correction

Order fact vs redaction

Support fact vs redaction

RFQ fact vs redaction

merge vs Order update

merge vs Support update

merge vs RFQ update

Customer Value config replace vs replace

lapsed worker vs fresh Order fact

trigger evaluation vs task completion

signal rebuild vs privacy change.

No sleep-only synchronization.

======================================================================
120. SEGMENT TESTS
======================================================================

For each new segment field:

validation

type checking

rule evaluation

NOT_AVAILABLE behavior

branch scope

dynamic materialization

dependency invalidation

privacy exclusion

generation swap.

No raw SQL.

======================================================================
121. CAMPAIGN SAFETY REGRESSION
======================================================================

Test:

high-value = true

at-risk = true

segment match = true

BUT

marketing consent/suppression denies

=> campaign handoff remains blocked.

New intelligence never bypasses 8E/8F consent.

======================================================================
122. LOYALTY REGRESSION
======================================================================

Existing loyalty/tier behavior must stay green.

Specifically preserve fix:

standing is based on unreversed qualifying credit entries

not stale lifetime credited counter.

Test:

credit
reversal
merge
tier configuration change.

======================================================================
123. 8I REGRESSION
======================================================================

Preserve:

internal-route root mounting

global route drift guard

privacy anti-resurrection

Notification reserve -> commit -> network -> record transaction model

frequency-cap fence

secure exports

no CRM PII.

======================================================================
124. MIGRATION TESTS
======================================================================

Required:

fresh 000001 -> final

upgrade from current pre-task tip -> final

rollback new completion migration(s)

prove accepted Section 8 remains intact

reapply.

No migration number collision.

======================================================================
125. TARGETED DEVELOPMENT TESTING
======================================================================

During implementation:

run only affected packages.

Do not run the entire 10+ minute CRM suite after every edit.

Redirect output.

PowerShell pattern:

go test <actual-target-package> -count=1 *> C:\KarryGoPush\.tmp-section8-target.log

Inspect:

Get-Content C:\KarryGoPush\.tmp-section8-target.log -Tail 80

Failures:

Select-String `
  -Path C:\KarryGoPush\.tmp-section8-target.log `
  -Pattern "FAIL|panic:|expected|actual" `
  -Context 3,5

Replace <actual-target-package> with real Go package path.

Do NOT type angle-bracket placeholders literally into PowerShell.

======================================================================
126. FINAL GROUPED REGRESSION
======================================================================

RUN ONLY ONCE after implementation stabilizes.

The full CRM package is known to take many minutes.

Use dedicated databases / serial groups as accepted in 8I.

Suggested:

GROUP A:
new final-completion tests

GROUP B:
internal/features/crm complete package

GROUP C:
other Vendor packages

GROUP D:
migrations/contracts/global router

GROUP E:
support-dispute-service only if modified

GROUP F:
other external service only if actually modified.

Use real PostgreSQL/Redis for gated tests.

Required:

0 failures

0 unintended skips.

======================================================================
127. BUILD QUALITY
======================================================================

Vendor Service:

go build ./...

go vet ./...

gofmt -l .

git diff --check

If another service was modified:

run appropriate build/vet/fmt/tests there too.

======================================================================
128. DOCUMENTATION
======================================================================

Update mandatory Vendor docs:

services/vendor-service/docs/README.md

services/vendor-service/docs/UI_UX_REQUIREMENTS.md

services/vendor-service/docs/FRONTEND_REQUIREMENTS.md

services/vendor-service/docs/PLATFORM_SERVICE_CHANGE_REGISTER.md

services/vendor-service/docs/API_ROUTE_REGISTER.md

services/vendor-service/docs/IMPLEMENTATION_STATUS.md

Relevant OpenAPI specs.

Relevant architecture docs only where architecture actually changed.

Do not rewrite unrelated historical material.

======================================================================
129. UI/UX REQUIREMENTS
======================================================================

NO FRONTEND IMPLEMENTATION.

Document future UI states for:

commercial metrics

affinities

service recovery

related refs

automatic tasks

customer value

loyalty tier separately

corporate commercial summary

RFQ/quote conversion

relationship signals

NOT_AVAILABLE

NO_BAND

CURRENT

STALE where applicable.

======================================================================
130. FRONTEND REQUIREMENTS
======================================================================

Document exact DTO behavior:

availability states

null semantics

branch scope

safe refs

Customer 360 blocks

task trigger indicators

Customer Value

relationship signals

corporate commercial/RFQ summaries.

No frontend code.

======================================================================
131. IMPLEMENTATION STATUS
======================================================================

At completion, document each original subsection individually:

8.1 Customer 360

8.2 Tags & Segmentation

8.3 Interactions & Tasks

8.4 Loyalty & Customer Value

8.5 Corporate CRM

8.6 Relationship Signals

Each must state:

IMPLEMENTED

or:

NOT IMPLEMENTED

Do NOT say "complete" if any CRM-side requirement is still inert/schema-only.

External producer absence should be separately recorded:

IMPLEMENTED — SOURCE NOT_PRESENT

if consumer implementation is complete.

======================================================================
132. EXTERNAL DEPENDENCY MATRIX
======================================================================

Final report/docs must classify:

VENDOR_COMMERCE_ORDER_PUBLISHER

SUPPORT_RECOVERY_FACT_PRODUCER

RFQ_FACT_PRODUCER

CUSTOMER_CONSENT_AUTHORITY

CUSTOMER_PRIVACY_EVENT_AUTHORITY

CUSTOMER_IDENTITY_MERGE_AUTHORITY

PII_FREE_CUSTOMER_PRESENTATION

NOTIFICATION_DELIVERY_EVENT_PRODUCER

PRODUCTION_SMS_CHANNEL

PRIVATE_EXPORT_ARTIFACT_SUPPORT

Use:

AVAILABLE

PARTIAL

NOT_PRESENT

Do not hide unavailable dependencies.

======================================================================
133. RELEASE-BLOCKING DEFECTS
======================================================================

SECTION 8 FINAL = NO-GO if any remain:

- schema-only original requirement
- fake source data
- unknown represented as zero/false
- duplicate Order counts under replay
- affinity corruption under correction
- complaint double counting
- RFQ double counting
- trigger duplicate tasks
- trigger privacy bypass
- lapsed worker race
- customer value stale classification
- high-value calculation contradicting Customer Value
- corporate cross-branch leak
- signal unknown evaluated as false in segments
- privacy resurrection
- merge double count
- campaign consent bypass
- cross-tenant access
- customer PII duplication
- monetary authority duplicated
- raw SQL segment escape
- route/OpenAPI drift
- migration rollback corruption
- unintended skipped tests
- build/vet/fmt failure.

======================================================================
134. ACCEPTABLE EXTERNAL BLOCKERS
======================================================================

These do NOT make CRM-side Section 8 incomplete if consumer side is fully
implemented and tested:

Order publisher NOT_PRESENT

Support producer NOT_PRESENT

RFQ producer NOT_PRESENT

Notification delivery callback producer NOT_PRESENT

production SMS NOT_PRESENT

authoritative consent source NOT_PRESENT.

They do mean:

FULL LIVE DATA ACTIVATION READY = NO.

But they may coexist with:

SECTION 8 CRM IMPLEMENTATION COMPLETE = YES.

======================================================================
135. FINAL ORIGINAL-SPEC AUDIT
======================================================================

Before declaring GO:

take the ORIGINAL Section 8 requirements and map every sentence/bullet to:

implementation

test

or legitimate external dependency.

Do not rely only on the completion checklist above.

Generate an explicit final matrix:

Requirement
Implementation location
Test evidence
Status
External dependency if any.

There must be:

0 REAL_GAP

for Section 8 CRM-owned scope.

======================================================================
136. FINAL REPORT FORMAT
======================================================================

Return:

SECTION 8 FINAL CRM COMPLETION + ACCEPTANCE REPORT

OVERALL VERDICT:
GO / NO-GO

SECTION 8 CRM BACKEND IMPLEMENTATION COMPLETE:
YES / NO

FULL LIVE DATA ACTIVATION READY:
YES / NO

REPOSITORY:
Branch:
Starting HEAD:
Ending HEAD:
Working tree:
Committed:
Staged:
Merged:
Branch created:
Branch switched:

BASELINE:
Pre routes:
Pre migration tip:
Pre segment registry:
Media DATA_EXPORT:

FINAL COUNTS:
Post routes:
New public:
New internal:
Final migration tip:
Final segment registry:

ORIGINAL SECTION 8 MATRIX:

8.1 CUSTOMER 360:
Commercial order count:
Completed order count:
Spend:
AOV:
First/last order:
Last branch:
Category affinity:
Item affinity:
Service recovery:
Corporate/group/bulk relationship:
Status:

8.2 TAGS & SEGMENTATION:
Tags:
Static segments:
Dynamic segments:
New final fields:
Availability semantics:
Dependency invalidation:
Status:

8.3 INTERACTIONS & TASKS:
Interactions:
Tasks:
Order refs:
RFQ refs:
Support refs:
Unresolved complaint trigger:
Rejected RFQ trigger:
High-value recovery trigger:
Lapsed trigger:
Corporate lead trigger:
Status:

8.4 LOYALTY & CUSTOMER VALUE:
Loyalty ledger:
Loyalty tiers:
Customer value bands:
Value standing:
Status:

8.5 CORPORATE CRM:
Corporate profile:
Contacts:
Commercial projection:
Recurring/bulk:
RFQ history:
Quote conversion:
Service notes:
Pipeline/task integration:
Status:

8.6 RELATIONSHIP SIGNALS:
Repeat:
Lapsed:
High-value:
At-risk:
Bulk-capable:
Corporate:
Recurring:
Future recurring-basket boundary:
Status:

COMMERCIAL FACTS:
Event contract:
Order producer:
Replay:
Versioning:
Correction:
Availability:
Result:

AFFINITIES:
Category:
Item:
Ranking:
Top-N:
Branch scope:
Merge:
Privacy:
Result:

SERVICE RECOVERY:
Producer:
Fact contract:
Projection:
Customer360:
Merge:
Privacy:
Result:

RELATED REFS:
Order:
RFQ:
Support:
History:
Result:

AUTOMATIC TASK ENGINE:
Worker:
Trigger dedupe:
Complaint:
Rejected RFQ:
High-value recovery:
Lapsed:
Corporate lead:
Privacy:
Result:

CUSTOMER VALUE:
Config:
Standing:
Next band:
Availability:
Concurrency:
Merge:
Result:

CORPORATE COMMERCIAL:
Order:
Bulk:
Recurring:
RFQ:
Quote conversion:
Availability:
Branch security:
Result:

RELATIONSHIP SIGNALS:
Storage:
Reasons:
Availability:
Recompute:
Worker:
Privacy:
Result:

SEGMENTATION:
Registry before:
Fields added:
Registry after:
Exact added fields:
Unknown semantics:
Invalidation:
Result:

CUSTOMER 360:
Commercial:
Affinity:
Recovery:
Loyalty:
Tier:
Value:
Signals:
Corporate:
PII:
Result:

INTERNAL EVENT INGRESS:
Route:
Event types:
Caller authorization:
HMAC:
Nonce:
Dedupe:
Fingerprint:
Source versions:
Result:

PRIVACY:
Redaction:
Anti-resurrection:
Suppression:
Export:
Segments:
Campaign:
Race tests:
Result:

MERGE:
Order:
Support:
RFQ:
Affinity:
Value:
Signals:
Tasks:
Race tests:
Result:

IAM:
Permissions added:
Mappings:
Branch scope:
Analyst:
Marketing:
Result:

ROUTES:
Pre:
New public:
New internal:
Post:
Exact new routes:

MIGRATIONS:
New migrations:
Fresh replay:
Upgrade:
Rollback:
Reapply:
Result:

EXTERNAL SERVICES:
Order:
Support:
RFQ:
Customer:
Notification:
Media:
Wallet:
Dispatch:
Taxi:
Hauling:
Changes actually made:

OPENAPI / CONTRACTS:
Specs updated:
Global router parity:
Route register parity:
Undocumented routes:
Phantom routes:
Result:

TESTS:
Commercial:
Affinity:
Recovery:
Interactions/tasks:
Automation:
Customer value:
Corporate:
Signals:
Segments:
Customer360:
Privacy:
Merge:
Concurrency:
Migrations:
Full CRM regression:
Other Vendor regression:
External services:
Failures:
Skips:
Result:

BUILD QUALITY:
go build:
go vet:
gofmt:
git diff --check:
Result:

PERFORMANCE:
Fixture:
Customer360:
Affinity:
Signals:
Trigger worker:
N+1:
Result:

DOCUMENTATION UPDATED:
[list]

EXTERNAL DEPENDENCY MATRIX:
[list status]

REAL SECTION 8 CRM GAPS:
NONE / list

SECTION 9 STARTED:
NO

FRONTEND TOUCHED:
NO

AI/ML IMPLEMENTED:
NO

PRE-EXISTING UNRELATED WORK PRESERVED:
YES / NO

FINAL VERDICT:

SECTION 8 CRM BACKEND:
GO / NO-GO

SECTION 8 IMPLEMENTATION COMPLETE:
YES / NO

FULL PRODUCTION ACTIVATION:
READY / BLOCKED BY EXTERNAL DEPENDENCIES

CONFIRM:
No commit.
No stage.
No merge.
No branch created.
No branch switch.
No Section 9 implementation.
No frontend implementation.

======================================================================
137. HARD STOP
======================================================================

DO NOT START SECTION 9.

After the final Section 8 report:

STOP.

If any CRM-owned original requirement remains incomplete:

report NO-GO and list the exact remaining implementation work.

Do not hide gaps.

If all CRM-owned original requirements are implemented and all certification
passes:

report:

SECTION 8 CRM BACKEND: GO
SECTION 8 IMPLEMENTATION COMPLETE: YES

External source dependencies may still prevent:

FULL LIVE DATA ACTIVATION READY.

Leave all changes uncommitted.

Wait for explicit authorization before Section 9.