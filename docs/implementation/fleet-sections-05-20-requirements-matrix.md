# Fleet Sections 5-20: requirements matrix

Snapshot 2026-11-13, branch `staging`. Spec: KarryGo Corporate Logistics Specification v2.0 (Part 1 Sections 5-20; detail rows from Part 2 Sections 36-50 where the audit added them).

**How to read this.** Rows and their initial status come from the 2026-11-13 audit (`spec-v2-audit/A-foundations.md`, `F-sections-8-25.md`), which traced each route to its handler, use case and SQL. Rows changed in this session were re-verified against PostgreSQL 16 through the real router or use cases and carry a commit and a test. Rows **not** changed this session keep the audit status and are marked "not re-verified"; the audit is evidence for them, not a fresh certification.

Final status values: **CLOSED** (implemented and tested this session), **IMPLEMENTED (audit)** (implemented per audit, not re-verified), **PARTIAL**, **MISSING**, **EXTERNAL** (owned by another service).

Granularity note: the spec has more atomic requirements than these rows (Part 2 detail tables, Section 57 schema, Section 66 tests). This matrix tracks at the audit's row level; finer items are listed under each section's gap text.


## Section 5

| ID | Requirement | Initial status | Final status | Action / gap | Commit | Test |
|---|---|---|---|---|---|---|
| FLEET-5a | `cmd/main.go`, `cmd/worker/main.go` | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-5b | `internal/{config,database,middleware,platformclients,events,jobs,observability}` | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-5c | Feature packages: organizations, staff, accesscontrol, approvals, providers, assignments, marketplace, wallets, customers, contracts, servicecatalog, orders, shipments, dispatch, routes, tracking, workforce, assets, inspections, maintenance, fuel, compliance, safety, proof, warehouse, finance, communications, reports, automation | PARTIAL | PARTIAL |  | | |
| FLEET-5d | `migrations/`, `docs/`, `.env.example`, `go.mod/go.sum` | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-5e | Each feature uses models/repositories/usecases/http (+clients) | PARTIAL | PARTIAL |  | | |
| FLEET-5f | No feature imports another feature's HTTP package | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-5g | shared/go only for generic code | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-5.1a | Handlers depend on use-case interfaces | PARTIAL | PARTIAL |  | | |
| FLEET-5.1b | Use cases depend on repository/client interfaces | PARTIAL | PARTIAL |  | | |
| FLEET-5.1c | Adapters wired at composition root | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-5.1d | Domain models free of DB/HTTP/SDK | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-5.1e | Cross-feature collaboration through interfaces/events | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |

## Section 6

| ID | Requirement | Initial status | Final status | Action / gap | Commit | Test |
|---|---|---|---|---|---|---|
| FLEET-6a | JSON; multipart only for upload handoff | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-6b | Bearer for users; service auth for internal | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-6c | UTC ISO-8601; local timezone stored separately | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-6d | Money = amountMinor + ISO currency; no float64 arithmetic | PARTIAL | PARTIAL |  | | |
| FLEET-6e | Opaque prefixed IDs; no sequential IDs | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-6f | Cursor pagination; page/limit only for compatibility | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-6g | Allowlisted filters/sorts; reject unknown | PARTIAL | PARTIAL |  | | |
| FLEET-6h | Success `{"data","meta"}` | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-6i | Error `{"error":{code,message,details,requestId}}` | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-6j | Idempotency-Key required on financial, provisioning, import and assignment actions | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-6k | Version + If-Match/expectedVersion | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-6l | Audit: actor, org, previous/new summary, request ID | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-6.1 | Standard Money object | PARTIAL | PARTIAL |  | | |
| FLEET-6.2 | Pricing deferral | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |

## Section 7

| ID | Requirement | Initial status | Final status | Action / gap | Commit | Test |
|---|---|---|---|---|---|---|
| FLEET-7a | Tenant context from authenticated membership | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-7b | Repository filtering by organization_id plus permitted branch IDs | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-7c | Composite uniqueness with organization_id | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-7d | Ownership checked before a 404/403-safe response | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-7e | Media metadata has org ownership and purpose | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-7f | Every event has organizationId + correlationId | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-7g | Job payload organizationId revalidated on execution | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-7h | Exports tenant-scoped, encrypted at rest, time-limited | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-7.e1 | organizations (status, legal identity, capabilities, country, tz, currency, version) | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-7.e2 | branches (type, address, geofence, hours, status) | PARTIAL | CLOSED | Branch create/edit/archive API; last active branch and branches with departments protected | `c39e5bd` | `TestOrganizationStructureAndAccessAdministration` |
| FLEET-7.e3 | departments (branch, parent, cost centre) | PARTIAL | CLOSED | Department create/edit/archive API; same-org references; cycle refusal | `c39e5bd` | `TestOrganizationStructureAndAccessAdministration` |
| FLEET-7.e4 | organization_settings (assignment timeout, approval thresholds, provider visibility, retention) | PARTIAL | CLOSED | GET/PATCH /organization-settings (timeout, provider visibility, thresholds in minor units); operating mode/retention/flags stay on their governed routes | `c39e5bd` | `TestOrganizationStructureAndAccessAdministration` |
| FLEET-7.e5 | organization_status_history (immutable) | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |

## Section 8

| ID | Requirement | Initial status | Final status | Action / gap | Commit | Test |
|---|---|---|---|---|---|---|
| FLEET-8.1 | POST /auth/phone/start (start owner challenge) | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-8.2 | POST /auth/phone/verify | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-8.3 | POST /auth/login (password login for org users) | MISSING | SUPERSEDED | Not built by decision: Fleet auth is passwordless email OTP (auth v2 closure; spec text conflicts with the newer approved decision). No SMS. | | |
| FLEET-8.4 | POST /auth/refresh, POST /auth/logout | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-8.5 | POST /organizations (draft) | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-8.6 | PATCH /organizations/{id} (legal/operating profile) | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-8.7 | POST /organizations/{id}/documents (business documents via media-file-service) | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-8.8 | POST /organizations/{id}/submit -> verification-compliance-service | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-8.9 | GET /organizations/{id}/onboarding-status | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-8.10 | Create headquarters/default branch (stage 5) | MISSING | CLOSED | Head Office branch created with every company (b547b37, migration 000087); further branches via POST /branches | `c39e5bd` | `TestOrganizationStructureAndAccessAdministration` |
| FLEET-8.11 | Configure timezone, currency, territories, initial policies | PARTIAL | CLOSED | Timezone/currency on organization; settings API added; territories via branches | `c39e5bd` | `TestOrganizationStructureAndAccessAdministration` |
| FLEET-8.12 | Activate only after checks + wallet provisioning | PARTIAL | IMPLEMENTED - LIVE CERTIFICATION PENDING | Verification decisions now applied and approved companies activated (e234978, ADR-006). Not re-verified live this session; wallet gate needs the Fleet-Wallet secret on staging. | `e234978` | |
| FLEET-8.13 | Lifecycle DRAFT->...->ACTIVE<->SUSPENDED->CLOSED | EXTERNAL | EXTERNAL | not re-verified | | |
| FLEET-8.14 | POST /organizations/{id}/close-request + controlled closure | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-8.15 | Branches / locations / departments / teams / cost-centres APIs (37.3, 59.2) | MISSING | PARTIAL | Branches, departments, settings routed. Teams, cost centres and locations entities not built (no tables) | `c39e5bd` | `TestOrganizationStructureAndAccessAdministration` |
| FLEET-8.16 | Sessions list/revoke, step-up (56.3) | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |

## Section 9

| ID | Requirement | Initial status | Final status | Action / gap | Commit | Test |
|---|---|---|---|---|---|---|
| FLEET-9.1 | GET /staff | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-9.2 | POST /staff/invitations (single-use hashed, expiring) | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-9.3 | Invitation resend / cancel / list (`/users/invitations/{id}/resend`) | MISSING | CLOSED | Invitation list/resend/cancel routed (d83e81e); verified by TestStaffDirectoryAndInvitations | `d83e81e` | `TestStaffDirectoryAndInvitations` |
| FLEET-9.4 | GET/PATCH /staff/{id} (branch, department, status) | MISSING | CLOSED | GET /staff/:id (d83e81e) and PATCH /staff/:id with tenant-checked branch/department ids | `c39e5bd` | `TestOrganizationStructureAndAccessAdministration` |
| FLEET-9.5 | POST /staff/{id}/suspend, /reactivate | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-9.6 | GET /roles, GET /permissions | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-9.7 | POST /roles, PATCH /roles/{id} (custom roles, version check) | MISSING | CLOSED | POST /roles, GET/PATCH /roles/:id, POST /roles/:id/archive; catalogue-validated; escalation guard on role contents; system roles immutable; bound roles cannot be archived | `c39e5bd` | `TestOrganizationStructureAndAccessAdministration` |
| FLEET-9.8 | POST/DELETE /role-bindings, PUT /users/{id}/roles | MISSING | CLOSED | PUT /staff/:id/roles, GET /staff/:id/role-bindings; self-change refused; owner keeps owner role; role version bumped | `c39e5bd` | `TestOrganizationStructureAndAccessAdministration` |
| FLEET-9.9 | Branch/department-scoped role bindings | PARTIAL | CLOSED | Bindings accept BRANCH/DEPARTMENT scope and expiry; scope must belong to the organization | `c39e5bd` | `TestOrganizationStructureAndAccessAdministration` |
| FLEET-9.10 | GET /approvals (pending + history), detail, decisions, history | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-9.11 | POST /approvals/{id}/approve, /reject (different approver, quorum) | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-9.12 | Approval policies CRUD (/approval-policies) | MISSING | MISSING | Policies are auto-created from templates (repositories/approvals.go:75-93); no API | | |
| FLEET-9.13 | Approval delegations (/approval-delegations) | MISSING | MISSING | No table use, route or usecase found | | |
| FLEET-9.14 | Step-up for high-risk actions | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-9.15 | Default-deny, role-version invalidation of cached decisions | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |

## Section 10

| ID | Requirement | Initial status | Final status | Action / gap | Commit | Test |
|---|---|---|---|---|---|---|
| FLEET-10.1 | GET /providers | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-10.2 | POST /providers (resolve-or-provision in owning provider service) | PARTIAL | PARTIAL | handlers.go:94 -> f/providers/usecases/provider_service.go CreateProvider. Only HAULING has a live contract (app/wiring.go:271, requires FLE | | |
| FLEET-10.3 | GET /providers/{id} (+history, provisioning-status) | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-10.4 | PATCH /providers/{id} | MISSING | CLOSED | PATCH /providers/:id (home branch must be own active branch; relationship/employment) | `1b3d869` | `TestManagedProviderAdministration` |
| FLEET-10.5 | POST /providers/{id}/suspend | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-10.6 | POST /providers/{id}/reactivate | MISSING | CLOSED | POST /providers/:id/reactivate; sub-wallet, platform-status and organization gates re-checked | `1b3d869` | `TestManagedProviderAdministration` |
| FLEET-10.7 | POST /providers/{id}/remove (end membership) | MISSING | CLOSED | POST /providers/:id/remove; refused while live work exists; ENDING->REMOVED; platform identity kept | `1b3d869` | `TestManagedProviderAdministration` |
| FLEET-10.8 | POST /providers/{id}/resend-login | MISSING | MISSING | no route | | |
| FLEET-10.9 | PUT /providers/{id}/capabilities | MISSING | CLOSED | GET/PUT /providers/:id/capabilities; cannot widen the service category | `1b3d869` | `TestManagedProviderAdministration` |
| FLEET-10.10 | PUT /providers/{id}/policies | MISSING | CLOSED | GET/PUT /providers/:id/policies; decline right cannot be disabled (refused, and DB CHECK) | `1b3d869` | `TestManagedProviderAdministration` |
| FLEET-10.11 | GET /providers/{id}/availability | MISSING | MISSING | no route (tracking projection is per trip, not per provider) | | |
| FLEET-10.12 | GET /providers/{id}/declines, /branches, /vehicles, /service-roles (39.3) | MISSING | PARTIAL | GET /providers/:id/declines added (reason codes only, safety-flagged). /branches, /vehicles, /service-roles not built | `1b3d869` | `TestManagedProviderAdministration` |
| FLEET-10.13 | Mandatory sub-wallet on activation | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-10.14 | Provisioning saga recovery | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-10.15 | Multi-service member layer (managed-members, managed-assets, consent, links, suspend/reinstate) | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-10.16 | Provider membership summary / corporate context for execution services | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |

## Section 11

| ID | Requirement | Initial status | Final status | Action / gap | Commit | Test |
|---|---|---|---|---|---|---|
| FLEET-11.1 | Operating modes CENTRALIZED/DELEGATED/HYBRID | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-11.2 | Dispatcher accepts request for org (`/requests/{id}/accept-for-organization`) | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-11.3 | Dispatcher declines opportunity | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-11.4 | Eligible candidates | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-11.5 | POST /requests/{id}/assign | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-11.6 | GET /assignments, /assignments/{id} | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-11.7 | POST /assignments/{id}/cancel, /reassign | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-11.8 | Provider accepts/declines assignment (`/assignments/{id}/accept\/decline`) | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-11.9 | Provider accepts/declines delegated offer (`/marketplace-offers/{id}/accept\/decline`) | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-11.10 | Decline reason catalogue, mandatory reason, OTHER free text | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-11.11 | Response deadline -> EXPIRED -> REASSIGNMENT_REQUIRED; dispatcher notification / auto-reassign | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-11.12 | Race protection / idempotency / 409 assignment_already_resolved | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-11.13 | UNSAFE_VEHICLE/VEHICLE_DEFECT decline -> safety/inspection workflow | PARTIAL | CLOSED | Safety-decline reconciler consumes marketplace UNSAFE_VEHICLE/VEHICLE_DEFECT declines: inspection, work order, asset block; unmanaged vehicles recorded, never wedge the sweep | `1b3d869` | `TestSection17And18EndToEnd` |

## Section 12

| ID | Requirement | Initial status | Final status | Action / gap | Commit | Test |
|---|---|---|---|---|---|---|
| FLEET-12.1 | Organization projection of source opportunities (canonical refs) | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-12.2 | Execution creation in source service after provider acceptance | PARTIAL | PARTIAL | usecases/execution.go; Dispatch needs FLEET_DISPATCH_EXECUTION_START_ENABLED + DISPATCH_SERVICE_URL, Hauling needs FLEET_HAULING_INTEGRATION | | |
| FLEET-12.3 | Trip lifecycle projection list/detail (`/trips`) | PARTIAL | PARTIAL | No `/trips`; `GET /tracking`, `/tracking/:canonicalTripReference` (f/dispatch/control.go) + internal `POST /tracking/events` | | |
| FLEET-12.4 | Projection gap detection / reconciliation | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-12.5 | Provider-facing data minimization (no fare/commission) | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-12.6 | Settlement projection display | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-12.7 | Completed-work ingestion from execution services | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-12.8 | Ledger/settlement execution | EXTERNAL | EXTERNAL | not re-verified | | |

## Section 13

| ID | Requirement | Initial status | Final status | Action / gap | Commit | Test |
|---|---|---|---|---|---|---|
| FLEET-13.1 | GET /wallet | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-13.2 | GET /wallet/transactions (+/:transactionId) | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-13.3 | GET /providers/{id}/sub-wallet | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-13.4 | POST /providers/{id}/sub-wallet/allocations (+list, approve, reject, replay) | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-13.5 | GET /providers/{id}/sub-wallet/statement | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-13.6 | POST /providers/{id}/sub-wallet/withdrawals (+approve, reject, replay) | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-13.7 | Provider self-service (own sub-wallet, statement, withdrawal) | PARTIAL | PARTIAL | Handlers exist (`/provider/sub-wallet*`, handlers.go:88-98) and are listed in riskregistry, but `RegisterProviderRoutes` is never called ->  | | |
| FLEET-13.8 | Withdrawal policy GET/PUT /wallet/policy | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-13.9 | Projection refresh / reconciliation | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-13.10 | Ledger, balances, transfers | EXTERNAL | EXTERNAL | not re-verified | | |
| FLEET-13.11 | Managed settlement instruction to Wallet | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |

## Section 14

| ID | Requirement | Initial status | Final status | Action / gap | Commit | Test |
|---|---|---|---|---|---|---|
| FLEET-14.1 | GET/POST /customers, GET/PATCH /customers/{id} (+status, risk-status, archive, history) | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-14.2 | POST /customers/{id}/contacts (+list/patch/archive/make-primary) | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-14.3 | POST /customers/{id}/locations (+list/patch/archive) | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-14.4 | GET/POST /sales/opportunities (+hold/resume/won/lost/history) | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-14.5 | GET/POST /quotations, submit, accept (+issue/reject/cancel/supersede/versions, secure customer link) | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-14.6 | Quotation acceptance creates order draft | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-14.7 | GET/POST /contracts, activate (+submit/suspend/resume/renew/terminate/versions/documents) | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-14.8 | GET/POST /service-level-agreements (+activate/suspend/terminate/observations) | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-14.9 | Customer credit profiles (hold/release) | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-14.10 | Leads + conversion (`/leads`, `/leads/{id}/convert`, 43.3/59.6) | MISSING | MISSING | no route | | |
| FLEET-14.11 | Pricing neutrality (manual lines, no formula) | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |

## Section 15

| ID | Requirement | Initial status | Final status | Action / gap | Commit | Test |
|---|---|---|---|---|---|---|
| FLEET-15.1 | GET/POST /service-catalog, GET/PATCH /{id} (+activate/deactivate/archive) | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-15.2 | GET/POST /orders, GET /orders/{id} (+stops, history) | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-15.3 | PATCH /orders/{id} | MISSING | MISSING | no route | | |
| FLEET-15.4 | POST /orders/{id}/confirm, /cancel | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-15.5 | GET/POST /shipments, GET/PATCH /{id} (+cargo-items) | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-15.6 | POST /shipments/{id}/packages; POST /packages/{id}/scan (+transition, custody, history) | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-15.7 | GET/POST /manifests, POST /{id}/close (+items, seal-amendments, cancel, patch) | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-15.8 | POST /orders/imports, GET /orders/imports/{id} | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-15.9 | GET/POST /recurring-orders (+activate/deactivate/replay) | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-15.10 | Private-order execution through existing provider service after provider acceptance | PARTIAL | PARTIAL | `/private-assignments*`, `/private-executions*`, internal private accept/decline (f/privateoperations); dispatch/hauling adapters gated; tax | | |
| FLEET-15.11 | Order state machine incl. AWAITING_APPROVAL/PLANNED/ASSIGNED/PARTIALLY_COMPLETED | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |

## Section 16

| ID | Requirement | Initial status | Final status | Action / gap | Commit | Test |
|---|---|---|---|---|---|---|
| FLEET-16.1 | GET /dispatch/board | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-16.2 | GET /dispatch/unassigned | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-16.3 | GET /dispatch/candidates (explainable) | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-16.4 | POST /dispatch/plans (+list/get/patch/approve/publish/cancel) | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-16.5 | POST /routes, PATCH /routes/{id} (+stops CRUD, approve, cancel) | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-16.6 | POST /routes/{id}/optimize (provider-neutral job) | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-16.7 | POST /routes/{id}/publish (creates assignments needing accept/decline) | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-16.8 | Re-optimize remaining work (`/routes/{id}/replan`) | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-16.9 | GET /control-tower | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-16.10 | Tracking projections (last-known, freshness) | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-16.11 | GET/POST /geofences (+patch, activate/deactivate) | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-16.12 | Geofence events (derived; `GET /geofence-events`) | PARTIAL | PARTIAL | Derived by worker into fleet_partner_geofence_events (workers.go:172-245); no read route | | |
| FLEET-16.13 | GET /alerts, acknowledge, resolve | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-16.14 | Rescue cases (`/rescue-cases`, resolve) | MISSING | MISSING | no route/aggregate (breakdowns exist under Section 18) | | |

## Section 17

| ID | Requirement | Initial status | Final status | Action / gap | Commit | Test |
|---|---|---|---|---|---|---|
| FLEET-17.1 | Workforce profiles (employment relationship, supervisor) | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-17.2 | GET/POST /shifts (+GET/PATCH /{id}, publish, cancel) | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-17.3 | POST /shifts/{id}/assignments | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-17.4 | POST /shift-assignments/{id}/check-in, /check-out (+GET, cancel) | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-17.5 | GET/POST /leave (+GET, approve, reject, cancel) | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-17.6 | GET/POST /qualifications (+GET/PATCH) | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-17.7 | POST /providers/{id}/qualifications (+list, verify, reject, revoke) | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-17.8 | GET/POST /training/courses (+GET/PATCH) | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-17.9 | POST /training/courses/{id}/complete | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-17.10 | GET /performance/providers/{id} (explainable) | PARTIAL | PARTIAL | Read-only list of snapshots (service.go ListPerformance). Snapshots are produced only by the maintenance worker (workers.go GenerateDuePerfo | | |
| FLEET-17.11 | Leave/qualification/training/shift feed assignment eligibility | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-17.12 | Decline metrics distinguish categories, no automatic discipline | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-17.13 | Shift templates (`/shift-templates`, 46.3) | MISSING | CLOSED | Migration 000089; /shift-templates CRUD + generate (DST gap skipped and reported, overnight, idempotent per day) | `dda4aa0` | `TestShiftTemplatesAndTrainingEnrollments` |
| FLEET-17.14 | Training enrollments (`/training/enrollments`, `/complete`, 46.3/59.4) | MISSING | CLOSED | /training/enrollments list/create/complete/cancel; one open per person per course; completion via existing course completion | `dda4aa0` | `TestShiftTemplatesAndTrainingEnrollments` |
| FLEET-17.15 | Provider self-view of schedule, qualifications, training, performance (46.2) | MISSING | MISSING | no provider-facing or internal route | | |
| FLEET-17.16 | Qualification-expiry / maintenance worker | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |

## Section 18

| ID | Requirement | Initial status | Final status | Action / gap | Commit | Test |
|---|---|---|---|---|---|---|
| FLEET-18.1 | GET/POST /assets, GET/PATCH /assets/{id} | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-18.2 | POST /assets/{id}/archive (+retire) | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-18.3 | POST /assets/{id}/assignments | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-18.4 | Asset documents | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-18.5 | GET/POST /inspections, /inspection-templates, POST /inspections/{id}/complete (critical fail -> BLOCKED + work order + event) | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-18.6 | GET/POST /maintenance/schedules | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-18.7 | GET/POST /maintenance/work-orders, approve, complete, return-to-service | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-18.8 | GET/POST /breakdowns | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-18.9 | GET/POST /inventory/parts, POST /inventory/stock-movements | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-18.10 | GET/POST /fuel-records (+approve/reject) | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-18.11 | UNSAFE_VEHICLE decline -> inspection/work-order draft | PARTIAL | CLOSED | Same change as 11.13 | `1b3d869` | `TestSection17And18EndToEnd` |
| FLEET-18.12 | Unassign provider, meter readings, tyres, defects resource, asset costs/history (47.3/48.3) | MISSING | MISSING | no routes | | |

## Section 19

| ID | Requirement | Initial status | Final status | Action / gap | Commit | Test |
|---|---|---|---|---|---|---|
| FLEET-19.1 | GET /compliance/status | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-19.2 | GET/POST /compliance/requirements (+patch, activate, deactivate) | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-19.3 | GET /providers/{id}/compliance, /assets/{id}/compliance | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-19.4 | Evaluations + restrictions apply/clear | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-19.5 | Restriction propagation to execution services | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-19.6 | GET/POST /safety/reports (+patch, resolve) | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-19.7 | GET/POST /incidents (+patch) | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-19.8 | POST /incidents/{id}/evidence | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-19.9 | POST /incidents/{id}/corrective-actions (+complete, verify) | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-19.10 | GET/PUT /proof-policies | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-19.11 | POST /proof-records (+GET list) | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-19.12 | POST /custody-events | PARTIAL | PARTIAL | Package-scoped only: `POST/GET /packages/:packageId/custody` (f/packages); no generic custody-event resource | | |
| FLEET-19.13 | Incident triage/investigations, proof verify, compliance checks (49.3) | MISSING | MISSING | no routes | | |

## Section 20

| ID | Requirement | Initial status | Final status | Action / gap | Commit | Test |
|---|---|---|---|---|---|---|
| FLEET-20.1 | Organization-scoped capability enable/disable/list | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-20.2 | Courier/last-mile records | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-20.3 | Taxi/passenger records | PARTIAL | PARTIAL | record types defined, but validation.go:74-76 refuses ("Taxi execution is disabled") | | |
| FLEET-20.4 | Trucking/haulage | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-20.5 | Heavy/specialized | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-20.6 | Freight forwarding / cross-border | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-20.7 | Cold chain (+temperature events) | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-20.8 | Medical/sensitive | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-20.9 | Agricultural/food | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-20.10 | Moving/relocation | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-20.11 | Towing/recovery | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-20.12 | Milestones / generic commands on records | IMPLEMENTED | IMPLEMENTED (audit) | not re-verified | | |
| FLEET-20.13 | Disabled modules must not expose routes (20.1) | PARTIAL | PARTIAL | Routes are always mounted; disabled capability answers 404/403 at service layer | | |
| FLEET-20.14 | Capability schemas / order specialized-data endpoints (50.3) | MISSING | MISSING | no `/capability-schemas`, no `/orders/{id}/specialized-data` | | |
