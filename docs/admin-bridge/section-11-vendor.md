# Secure Admin Section 11 — Vendor implementation evidence

This is Secure Admin Section 11, not the Vendor service's older internal section numbering. It extends the existing private Bridge and native Vendor engines. Sections 6–10 and all staging history are preserved. Nothing in this work deploys services, installs production grants or enables Admin access.

## Implemented behavior

- New purchases use the signed, fresh Customer standing contract, with a dedicated Vendor audience/key and a default-off enforcement switch. Canonical order creation checks standing before writes or side effects. Previously committed orders return their original idempotent result during a later hold. RFQ/scheduled creation uses the authoritative quote customer; uncertain contractual recurring renewals acquire no invented eligibility exception.
- The private Vendor source exposes 37 fixed, allowlisted resource kinds against actual merchant, brand, branch, verification-reference, Food, Shop, inventory, production, kitchen, pastry, allocation, fulfillment, substitution, incident and delivery records. Branch hours and commercial directory labels are available without exposing contacts, free-text case material or private URLs.
- Stock reads use the canonical expiry-aware Inventory availability view. Operational eligibility uses Vendor's existing verification reducer, including expired evidence. A historical verification reference does not become a new verification decision.
- Parent order references are scoped to the authorized child allocation. Sibling merchant details are withheld. Delivery links retain the selected native authority; external-owner observations are explicitly projections, not live Dispatch/Taxi/Hauling truth. Vendor delivery state remains Vendor-owned.
- Seventeen named command bindings reuse native state machines: merchant/branch pause and reviewed reopening, Food/Shop takedown and reviewed restoration, production block/resume, delay reporting, capacity rescheduling, approved-substitution inventory reconciliation, and investigation/escalation/finding/appeal/closure transitions. Case results explicitly mean a recorded owner case transition, not a completed refund, verification or logistics decision.
- Customer-approved substitution processing uses the existing replacement identity and actual Inventory transaction. It neither supplies customer consent nor changes approved prices/quantities. Its administrative processing marker is independent of whether ordinary customer processing previously occurred.
- Canonical new-booking creation also serializes against actual merchant/branch/listing restrictions. A quote approved before a pause cannot create a new booking afterward; an existing committed booking is unaffected.
- Native Vendor IAM is checked in addition to signed delegated scope, current source allocation and policy. Branch grants do not cover merchant-wide data. Mutation checks hold native permission/assignment locks through commit. No role, staff assignment, organization/tenant relationship or geographic authority is created by this integration.
- Private Media proof uses actual Vendor case/business/branch membership. Modern case-bound Media objects use the existing grant, safety, quarantine, revocation and immutable disclosure machinery. Closed cases withhold proof. Legacy asset identifiers are never aliased to modern object identifiers.

## Fixed contracts and ownership

Vendor adds 74 fixed list/detail Bridge paths and the existing six command orchestration shapes for the Vendor command family. Native registration adds seven resource/context/case route templates and three command/readback/history templates. Generated private contracts include Vendor-specific resource and command schemas; existing owner wire schemas remain compatible. Capability discovery describes prerequisites; it grants no access.

Vendor owns merchant, catalogue, inventory, production, order and fulfillment transitions. Verification retains formal verification authority, Wallet retains financial authority, delivery owners retain mission/custody authority, and Media retains disclosure authority. No generic HTTP proxy, state setter, refund calculator, custody override or public Admin route was added.

Migrations `000133_secure_admin_owner_boundary` and `000134_admin_approved_substitution_processing` add native signed context/audit/command/case persistence and administrative substitution-processing evidence. They were applied only to disposable test databases. Populated immutable evidence prevents destructive rollback; see the Section 11 runbook.

## Evidence and acceptance

The canonical Vendor runner uses a separate migrated PostgreSQL database per package and Redis. It emits JSON evidence and fails on failed/skipped owner tests. Dedicated gates require the new source-behavior cases and validate actual signed native directory, command-receipt and case-proof responses against private contracts.

Owner scenarios cover native branch authorization and cross-tenant denial, stale versions, duplicate/concurrent actions, immutable audit failure rollback, catalogue stock/price preservation, verification expiry, capacity conflicts, booking holds and committed-order readback, authoritative allocation relationships, real Inventory replacement transaction rollback, expiry-aware stock and signed price differences. Existing native multi-vendor lifecycle, split-refund caps, logistics replay and migration regressions remain in the full runner.

Additional suites cover Bridge mTLS/HMAC/delegation/approval/readback, existing-owner regressions, public Gateway denial, Media grant/content/revocation and immutable auditing. Native Vendor proof and the actual Media content service are independently tested; external owner/service-process doubles are not represented as deployed end-to-end certification. Local Windows testing does not claim race instrumentation; Linux acceptance supplies race tests.

`TestStackOwnDeliveryShare` is separately identified as external running-stack certification requiring five local processes. It is not silently counted as a passing isolated database test. No deployment or live-stack certification was performed.

Exact pushed commit, required seven workflow results and final test totals belong in the closure report after acceptance finishes. Earlier passing staging runs do not certify a later commit.

## Explicit unavailable authority and activation gates

- No canonical Vendor-owned contract establishes global market/zone closure or cross-merchant geographic administration. Branch-level operations do not invent it.
- HQ financial adjudication, independent refund/return-fee calculation, forced delivery/custody transitions and arbitrary logistics replay are unavailable. The supported interface records and escalates owner-scoped requests and exposes real owner result references; it does not claim those requests completed a financial or delivery decision. Native existing financial/logistics engines retain their current contracts and tests.
- Legacy evidence adoption needs an authoritative legacy-to-modern object mapping. The new proof boundary supports explicitly bound modern objects only.
- Live signed source scope, source-native staff enrollment, high-risk approvals, policy revisions, dedicated credentials and Media disclosure/lifetime decisions remain owner-controlled governance prerequisites. No production values are supplied. All new privileged flags remain false; absence or ambiguity fails closed.

These unavailable owner decisions must not be described as implemented commands. Section 11 cannot be represented as unrestricted HQ financial/geographic administration merely because its feasible native operations and software boundaries pass tests.
