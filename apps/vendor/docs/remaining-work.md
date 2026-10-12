# Remaining vendor frontend work

Verified backlog clarification: 11 October 2026. This tracker distinguishes missing app screens, backend integration limitations and deployment/final verification. Completed features below must not be presented as new modules to build. This is a targeted workflow review, not certification that all 740 client operations have complete user journeys or are deployed.

## Fixed remaining list ? source check, 11 October 2026

These numbers are the current tracking IDs. Keep completed rows marked complete rather than renumbering the rest. Cloud kitchen was not completed before work moved to Media. This list replaces the earlier inconsistent grouping; it does not identify new feature modules.

| No. | Remaining work | Completion boundary |
| --- | --- | --- |
| 1 | Cloud-kitchen corporate selection, currency context and logistics editing | PARTIAL: verified CRM corporate selection, actual business currency and saved logistics editing are implemented. NEW typed logistics fields remain undefined by the backend. |
| 2 | Order evidence | IMPLEMENTED: picking reads include stored measurements/their independent versions and saved quality observations. The app reads both and records actual quantities with the measurement version. Live verification remains item 5. |
| 3 | Setup gaps | IMPLEMENTED: draft branch details open the existing business activation review; rider setup discovers eligible branches through delivery permission and creates its first availability with version 0. Live verification remains item 5. |
| 4 | Current staff access | IMPLEMENTED: More > My access reads the authenticated staff member's current assignment grants, branch/business scope and high-risk flags. Live verification remains item 5. |
| 5 | Final deployment and Live/phone verification | Deploy changed services, apply migrations, provision/configure Media storage/scanner, verify actual permissions/workers/recovery, and check phone layouts and both themes. Review legacy-file preview compatibility; the new Media bridge previews its own bound uploads. |

Existing unresolved backend definition: pastry creation of NEW nested production/logistics requirements has no defined field schema. Resolve that schema before building those inputs; do not invent fields. This is the same pastry boundary from the earlier list, not a newly discovered module. Verified reference selection and editing existing saved requirement maps are implemented.

Implemented work remains implemented: CRM exports, business-wide customer insights, pastry reference selection/saved-map editing, and product/brand/pastry media upload/access code. Media deployment and real scanner/storage verification belong to item 5. See media.md for the exact integration boundary.

Item 1 implementation update: corporate references/currency/saved-map editing are implemented; new typed logistics creation still needs a backend field definition. Item 2 is now implemented. Item 3 is now implemented. Item 4 is now implemented. Item 5 is next. This is a source-based check of the agreed backlog, not certification that every one of the 740 operations or deployed journeys is complete.

Source anchors checked: cloud_kitchen_workspace_screen.dart/controller versus cloud_kitchen/http.go/service.go; order_detail_screen.dart and shop_picking_operations.go (stored measurement/quality evidence implemented); current_access_screen.dart and staff CurrentAccess handler (own grants/scope); rider_workspace_controller.dart and rider_branches.go (delivery-scoped branch discovery implemented); branch_settings_screen.dart/vendor routes; Vendor media screen/handler/scanner and media.md.

## Intended vendor journeys

- [x] Promotions: list/detail, all eight offer types, vendor-funded configuration, lifecycle, coupon creation and usage; advanced reference discovery and Live verification boundaries are in promotions.md.
- [x] Advanced branch delivery policy editors: zones, distance pricing, timezone/windows, vehicle classes, rider availability and fallback triggers; preserve server delivery/funding authority.
- [x] Orchestration decision/history visibility and explicitly permitted authority overrides; no frontend provider dispatch.
- [x] Taxonomy browsing and catalogue mapping; assigned tags and dietary claims are read-only on the vendor API. No platform taxonomy administration.
- [x] Purchasing: suppliers/products, purchase orders, approvals, receipt inspection/posting/reversal, discrepancies, policies, documents by existing Media reference, costs and alerts; see purchasing.md for integration boundaries.
- [x] Production foundations: ingredient and packaging list/detail/create/edit/archive, with Mock fixtures and Live requests.
- [x] Production foundations: conversion list/create/edit/archive status and branch configuration/inventory mapping, with actual generic inventory references. See production-materials.md for Live boundaries.
- [x] Recipes, draft versions, ingredient/packaging replacement, validation, activation/retirement, scaling and recipe-cost preview/history/snapshots; see recipes.md for scope and Live boundaries.
- [x] Production stock policies and batches: target-scoped policy, active-version creation, requirement preview, plan/start, consumption, waste, top-up reservations, completion/output, cancellation and reservation evidence; see production-batches.md.
- [x] Production operations first stage: ordered job queue/detail/history, manual creation, queue/start/block/resume/cancel, station steps, delay reports and capacity preview/utilization; see production-operations.md.
- [x] Production operations: ready packaging evidence/handoff, lines, pool/rule management, slots/templates, blackouts, reservations and operational alerts; see production-operations.md for contract and Live boundaries.
- [x] Production/operational costs: material valuation policies, expected/actual batch snapshots, variance, saved component evidence/history and highest-cost subjects. The mounted API does not expose generic expense entries or allocations; see production-costs.md.
- [x] Core CRM: relationships, customer views, tags/segments, interactions/tasks, loyalty and corporate accounts; see crm.md for implemented flows and integration boundaries.
- [x] Business-wide CRM insights: scope selection, RFM/churn coverage and freshness, distributions, unavailable reasons and reviewed rebuilds with durable retry; see crm-insights.md.
- [x] CRM exports: supported datasets/formats, scoped filters, status/detail, cancellation, fresh OTP file saving and creation recovery; see crm-exports.md.
- [x] CRM marketing campaigns: definitions, audience preview/preparation, snapshots/targets, immediate/scheduled runs, pause/resume/cancel and delivery/history; see crm-campaigns.md for Mock and deployed integration boundaries.
- [x] Cloud kitchen: branch settings, bulk packages/components/bands, capacity preview, corporate plan configuration/lifecycle, date exceptions and occurrence recovery/cancellation; see cloud-kitchen.md for Live boundaries.
- [x] Pastry foundations: products/options, option-group attachment, delivery handling, custom requests, direct specification history/revisions, submission and eligibility-aware cancellation; see pastry.md.
- [x] Pastry quotes/revisions, OTP approval, vendor accept/decline, conversion response evidence, design comparison/decisions/history, standard production capacity/batch creation and milestone transitions/history; see pastry.md.
- [x] Pastry supported design/policy/risk configuration and scoped attachment discovery/removal; see pastry.md.
- [x] Pastry verified customer/corporate selection, saved nested-map editing and reference-file upload/access; see pastry.md and media.md.
- [ ] Pastry new nested requirement creation schema; see pastry.md.
- [x] Existing scheduled-order list/detail/cancellation and recurring list/detail/edit/status/occurrences.
- [x] New recurring schedules from an existing scoped corporate-plan context, with review and durable duplicate-safe recovery; see scheduled-commerce.md.
- [x] Vendor RFQ inbox/details, private quote revisions, quotation creation and durable recovery; see scheduled-commerce.md.
- [x] Approved-quote scheduling creation with scoped customer/branch discovery, delivery setup, shared-capacity reservation and durable recovery; see scheduled-commerce.md.
- [x] Recurring worker capacity windows follow each occurrence's local date; database date boundaries and daylight-saving transitions are handled explicitly.
- [x] First-time corporate recurring setup with verified customer/destination context and backend-defined fixed-menu evidence; see scheduled-commerce.md for deployment and rotating-menu boundaries.
- [ ] Actual deployed Live integration and final access/state/recovery/responsiveness/theme checks.

Some entries belong inside existing Orders, Catalogue or More screens rather than additional tabs. Show capability-specific work only for eligible branches/businesses, and respect actual staff scope. Generic inventory has a separate contract in the backend guide; audit its coverage separately before claiming the Shop stock UI covers production inventory.

## Known dependencies and backend gaps

- [ ] Cloud-kitchen NEW typed logistics field definitions remain absent. Corporate profile discovery, actual currency context and saved-map editing are implemented; deployed worker/capacity verification remains item 5.

- [ ] Deployed CRM intelligence/export permission, scope, worker and privacy verification. Overview, distributions and rebuild screens are implemented; this does not certify every CRM route or production behavior.

- [x] Catalogue upload/access integration; see media.md for deployment and legacy-access boundaries.
- [x] Actual measured-quantity version/evidence reads and recording. Financial finalization remains server-owned.
- [x] Readable saved picking-quality status and observations.
- [x] Draft branch activation review through the existing onboarding readiness/request-activation flow.
- [x] Brand-image and pastry-reference upload/access integration; see media.md.
- [x] First availability setup for rider accounts without ordinary branch-list access.
- [ ] Deployed corporate setup join/privacy/permission verification. Fixed-menu setup, approved-quote discovery and RFQs are implemented and await deployment. Arbitrary menu rotation is not exposed by the fixed-menu contract.
- [x] Current effective staff access discovery through authenticated self-only grants, separate from the permissions catalogue.

## Excluded from vendor controls

Customer discovery/cart/checkout/quote approval/payment, group organizer/participant actions, customer RFQ acceptance/conversion and recurring basket purchasing. Internal workers, service-credential operations, platform/operator administration and independent payment/dispatch authority are also excluded.

Analytics/reporting is identified by the guide as an Analytics Service integration, not assumed to be an existing vendor-service workspace. Review that separate integration before including it as mandatory remaining vendor backend work.

## Source evidence

- services/vendor-service/docs/frontend/VENDOR_FRONTEND_GUIDE.md: screen map and flows 3, 4, 8, 9, 10 and 11.
- services/vendor-service/docs/SECTION21_TRACEABILITY.md: requirement ownership and route/event boundaries.
- services/vendor-service/internal/features/orders/commerce_orders_http.go: actual mounted Vendor and Customer routes.
- services/vendor-service/internal/features/orders/commerce_orders.go and recurring_feeding.go: authorization, checkout/capacity, schedule state and worker boundaries.
- services/vendor-service/internal/features/orders/recurring_feeding_test.go: validation/occurrence identity tests, not Live certification.
