# Vendor frontend coverage

Reviewed against the current 728 public Vendor operations in 29 contract modules on 9 October 2026. Operation totals describe the API foundation, not completed screens. A generated API/controller does not constitute an implemented user journey.

## Existing app journeys

| Area | Public operations | App coverage |
|---|---:|---|
| Auth | 7 | Phone/email, OTP, resend, vendor choice, session restoration, sign-out and recovery |
| Onboarding | 21 | Business setup, document upload, verification, banking, security and activation states |
| Business | 32 | Profile, brands, branches, hours, capabilities and preferences; public Draft activation limitations remain |
| Access | 13 | Staff, role assignments, permission catalogue and supported high-risk changes |
| Food catalogue | 61 | Menu/section/item/variant/modifier configuration, pricing previews and supported advanced controls |
| Shop catalogue | 100 | Products, variants, branch listings, classifications, policies and supported advanced controls |
| Inventory | 27 | Stock tracking, receiving/reversal, transfers/arrivals, quarantine, reorder rules, adjustments, stocktakes, movements and alerts |
| Orders | 25 | Detail, supported lifecycle actions, picking, quality and substitutions; actual-quantity finalization needs a readable measurement version |
| Handover | 2 | Platform pickup observation and credential confirmation |
| Finance | 6 | Balance, transactions, withdrawal review, verification, submission, tracking and uncertain-outcome recovery |
| Readiness | 8 | Order preparation, active branch types, estimates, category activity, ready/delay/block, history and version conflicts |
| Delivery | 23 | Mission management, eligible rider assignment, rider availability/progress, verified delivery/return, core and advanced branch policy, and fallback recovery; Live access boundaries documented in readiness-delivery.md |
| Scheduled commerce | 11 | Scheduled and recurring list/detail, cancellation, recurring editing/status and occurrences; creation/RFQ discovery remains pending in scheduled-commerce.md |
| Promotions | 7 | List/detail, eight offer types, vendor-funded create/edit, lifecycle, coupons, usage and durable identical retry; advanced audience references and Live limitations in promotions.md |
| Taxonomy | 10 | Published facets/nodes and category detail, per-facet mapping replacement, assigned tag/claim status and dietary definitions; vendor tag/claim writes are not exposed. See taxonomy.md |
| Orchestration | 3 | Delivery plan, decision history and permitted authority overrides with audit-based uncertain-outcome recovery; see orchestration.md |
| Recipes | 14 | Catalogue-target creation, draft versions/yield, ingredient and packaging replacement, validation, activation/retirement, scaling and archive; see recipes.md |
| Production cost | 5 | Expected/actual immutable batch snapshots, comparison and paginated history; see production-costs.md |
| Operational cost | 7 | Ingredient/packaging policy create/edit/archive, component evidence and highest-cost materials; see production-costs.md |
| Recipe cost | 5 | Current/version preview, latest snapshot, history and immutable snapshot creation; missing costs remain unavailable |
| Production | 54 | Jobs/queues, station steps, ready handoff, capacity resources/calendars, reservations and alerts; see production-operations.md |
| Production stock policies | 2 | Recipe-target branch policy, on-demand/batch-stock model and prepared-output conversion; see production-batches.md |
| Production batches | 13 | Active-version creation, requirement preview, plan/start, actual consumption, waste, reservation top-ups, completion/output, cancellation and inventory evidence |
| Production master | 20 | Ingredient/packaging list/detail/create/edit/archive, conversions and branch configurations using generic inventory references; see production-materials.md |
| Purchasing | 56 | Suppliers, categories, contacts, terms/products, orders/approvals, inspection/posting/reversal, discrepancies, policies, existing document references, cost evidence, alerts and performance; see purchasing.md |
| Support | 2 | Order issue reporting, review, status list and projection details, with durable identical retry and reconciliation |

These areas have app journeys, but this table does not claim every operation is exposed or deployed-server verified. Their detailed contract boundaries are documented in orders.md, catalogue.md, inventory.md, finance.md, settings.md and support.md.

## API foundation present; vendor journey coverage still missing

| Module | Public operations |
|---|---:|
| CRM | 121 |
| Cloud kitchen | 30 |
| Pastry | 43 |

Business-type, branch and server permission checks must determine availability. A business should not see a workspace merely because an API file exists. Route-by-route read/command/state reviews are required before implementing these workspaces.

## Remaining work order

1. Orchestration decisions/authority overrides and advanced delivery policy editors are implemented. Verify Live rider branch discovery and handover authority. Preparation, rider assignment/actions, packing/custody/returns and fallback workspaces are implemented; remaining boundaries are documented in readiness-delivery.md.
2. Taxonomy and promotions are implemented using the existing public surfaces. Vendor tag/claim editing is not exposed by the backend. Scheduled/recurring management and scoped reads are implemented. Creation, corporate setup and RFQ reference discovery remain separate integration work; see scheduled-commerce.md.
3. Production costs, policies, operations, purchasing and production foundations are implemented; deployed Live verification remains pending. Keep Shop receiving separate from production ingredient stock.
4. CRM and specialist cloud-kitchen/pastry workflows.
5. Final integration checks across supported business types, permissions, session expiry, conflicts, interrupted writes, small phones and light/dark modes.

## Live verification boundary

Mock exercises the UI without server calls. Controlled HTTP checks validate exact public routes, payloads and recovery behavior. An actual deployed Live run still requires an explicit Vendor origin, an activated test business and its test credentials; controlled transport checks do not prove deployment configuration, real OTP delivery, Media availability, payout providers or logistics events.

Known backend gaps remain explicit: variable measurement versions/evidence are not exposed in order reads; saved picking quality completion is not projected; Draft branch activation and general brand-image upload/resolution have no public vendor flow. No client-generated fields or status transitions fill those gaps.

## Reviewed workflow backlog

See [remaining-work.md](remaining-work.md) for intended vendor journeys, capability-specific work, backend dependencies and customer/internal exclusions. The backlog reflects the documented workflow intent; it does not turn each API module into a mandatory separate screen.

Production operations now exposes the workflows for all 54 public Production route patterns: jobs/steps/handoff/delays/history, resource and calendar management, reservations and alerts. See production-operations.md for the reservation-discovery, flexible packaging-evidence, Mock calendar and Live boundaries.

Core CRM now has a workspace at More ? Customers & relationships: customer views, relationship archive/restore, tags, static/dynamic segments, interactions/corrections, tasks/history, loyalty programme/tiers/ledger and corporate accounts with contacts, locations, cost centres, memberships, activity and branch scope. Mock and Live use the existing CRM contract. Campaigns now have a separate workspace (crm-campaigns.md); exports and business-wide intelligence now have dedicated screens; the 121-route foundation count does not mean all routes have UI. See [CRM](crm.md).

Campaign UI covers the public campaign definition, preview/preparation, snapshot/target and run/delivery/history routes. Internal delivery-event callbacks and worker dispatch are excluded from vendor controls. Deployed feature flags, consent/notification integration and final phone verification remain pending.

Cloud kitchen now has a capability-specific workspace at More ? Cloud kitchen. It covers settings, package CRUD/archive, components, quantity bands, capacity preview, recurring corporate plans, lifecycle/date exceptions and occurrence retry/replay/reschedule/cancel. Existing production queue, reservation and capacity screens remain the operational follow-through. Source contracts and remaining reference/worker boundaries are in [Cloud kitchen](cloud-kitchen.md).


## Pastry foundations (10 October 2026)

More > Pastry & bakery connects products, reusable option groups/options, attachment commands, product delivery handling and custom-request/specification draft workflows in Mock and Live. The route and state boundaries, remaining quote/change/production/milestone work and missing attachment discovery are documented in pastry.md. No full pastry or deployed certification claim.


## Pastry quotes, reviews and production (10 October 2026)

Request details now link to Quotes, Design reviews and Production milestones; product details link to Plan production. Existing public routes drive revision, grant-based approval, vendor acceptance/decline, capacity preview/batch creation, specification comparison/decisions and milestone history/transitions. Mock shares production capacity and job records. Acceptance retains the backend order_authority=NOT_PRESENT boundary. See pastry.md for advanced configuration and deployed verification work.

## Pastry configuration - 11 October 2026

Added creation-time request policies, handling and risk flags, design colours/registry option selection/existing Media references, and scoped attachment discovery/removal. Saved opaque nested requirements are preserved without invented fields. Live attachment reads require the updated vendor-service deployment. See pastry.md for contract and integration limits.

## Recurring creation - 11 October 2026

Scheduled orders > Recurring now creates a new meal schedule using an authoritative existing corporate-plan context. Full-screen setup, reviewed creation, required capacity-window editing and durable idempotency recovery use the existing backend POST. First-time corporate context setup, approved-checkout scheduling and vendor RFQ discovery/quotation remain pending; see scheduled-commerce.md.

## Vendor RFQ quotations - 11 October 2026

Added eligible RFQ inbox/details, private branch-scoped revision history, amount/expiry/branch and pool-capacity selection, reviewed submission and durable idempotency recovery. Three missing vendor reads enforce explicit invitation/read-scope checks. Customer acceptance/checkout remain excluded. See scheduled-commerce.md for policy-editor and Live boundaries.

## Approved scheduling - 11 October 2026

Added approved quote discovery/detail, delivery and preparation setup, typed pool-capacity selection, reviewed scheduling and durable recovery. Two missing scoped reads and explicit quote business/branch binding were added. Mock shares production reservation/cancellation state. First-time corporate setup is described below; see scheduled-commerce.md.

## First-time corporate meals - 11 October 2026

Added scoped corporate/approved-food-quote discovery, fixed-menu/address preview, schedule configuration, optional production pool planning, one final confirmation and durable creation recovery. Backend checks CRM/customer identity/privacy/branch binding and immutable checkout evidence; safe replay remains possible after quote expiry. Mock supplies explicit fixture context. No customer payment/approval, CRM location substitution or arbitrary rotating-menu authoring is introduced. Deployment and database/device verification remain pending; see scheduled-commerce.md.


CRM business-wide insights are implemented through the existing overview, RFM, churn and rebuild routes, with scope selection, unavailable reasons, reviewed rebuilds and durable recovery. CRM exports are also implemented. This does not imply deployed worker or permission verification; see crm-insights.md, crm-exports.md and remaining-work.md.


Pastry custom requests now use verified branch-scoped canonical customer and CRM corporate profile selections through an explicit backend reference bridge. Existing saved requirement maps support type-preserving edits without introducing keys. Creation of new nested requirement fields remains a contract boundary; see pastry.md.

Vendor media upload/access is implemented in product details, brand details and pastry design references, including Mock sample photos, target-bound Live grants, private previews, PDF saving and durable upload/attachment recovery. Media has a configurable ClamAV scanner adapter. Deployment/scanner provisioning/migration and physical-phone verification remain pending; see media.md.
