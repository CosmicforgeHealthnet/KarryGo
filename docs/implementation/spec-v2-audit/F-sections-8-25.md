# Fleet backend vs. KarryGo Corporate Logistics Specification v2.0 - Sections 8-25

Read-only audit of `services/fleet-partner-service` (committed HEAD on 2026-11-13; see Baseline). No repository file was changed.

**Method.** Ground truth for mounted routes was obtained by building the real router (`app.BuildComponents` + `Registrars` + the `cmd/main.go` internal registrars) in a throwaway copy of the service under the scratchpad and dumping `gin.Engine.Routes()`; this was compared with `internal/platform/riskregistry/registry.go`. For each route the handler and usecase were opened to confirm real persistence (SQL in a transaction with audit/outbox) rather than a stub. Flags and defaults come from `internal/config/config.go`. Requirements are taken from the summary Sections 8-25 plus the binding detail Sections 37-56/59 where they add endpoints (the "(detail: Sec. N)" rows). Paths below omit the `/fleet-partner/v1` prefix; "internal" means `/internal/fleet-partner/v1` (HMAC). `f/` = `internal/features/`.

Status meaning: IMPLEMENTED = mounted route + real handler/usecase/persistence (may be behind a default-off flag; flag shown); PARTIAL = route exists but a key sub-capability is missing, unmounted or unreachable; MISSING = no route/implementation; EXTERNAL = owned by another service per spec.

**Baseline.** This audit reflects committed `HEAD` 2f6932f. The fleet backend was last changed in d3406aa (14:46 UTC). While the audit was running, at about 16:31 UTC, another session began uncommitted working-tree edits that add `GET /staff/:staffId`, `GET /staff/invitations`, `POST /staff/invitations/:invitationId/resend` and `DELETE /staff/invitations/:invitationId` (`organizations/http/handlers.go`, `riskregistry`, objectauth). If those edits land, row 9.3 becomes IMPLEMENTED and row 9.4 becomes PARTIAL (read only, no PATCH). They were not evaluated here.

## 1. Summary

### 1.1 Counts per section

| Sec | Title | IMPLEMENTED | PARTIAL | MISSING | EXTERNAL | Rows |
|---|---|---|---|---|---|---|
| 8 | Authentication, company onboarding and lifecycle | 10 | 2 | 3 | 1 | 16 |
| 9 | Company users, roles, permissions and approvals | 8 | 1 | 6 | 0 | 15 |
| 10 | Organization-managed provider accounts | 7 | 1 | 8 | 0 | 16 |
| 11 | Request acceptance, assignment, provider accept/decline | 12 | 1 | 0 | 0 | 13 |
| 12 | Existing marketplace request and trip integration | 5 | 2 | 0 | 1 | 8 |
| 13 | Company wallet and provider sub-wallets | 9 | 1 | 0 | 1 | 11 |
| 14 | CRM, sales, contracts and SLAs | 10 | 0 | 1 | 0 | 11 |
| 15 | Service catalogue, orders, shipments, packages | 9 | 1 | 1 | 0 | 11 |
| 16 | Dispatch, routes, live control and tracking | 12 | 1 | 1 | 0 | 14 |
| 17 | Workforce, shifts, training and performance | 12 | 1 | 3 | 0 | 16 |
| 18 | Fleet, assets, inspections, maintenance and fuel | 10 | 1 | 1 | 0 | 12 |
| 19 | Compliance, safety, incidents and proof | 11 | 1 | 1 | 0 | 13 |
| 20 | Specialized logistics modules | 11 | 2 | 1 | 0 | 14 |
| 21 | Warehouse, fulfilment, yard, dock and terminal | 10 | 0 | 1 | 0 | 11 |
| 22 | Finance, billing, payroll, expenses and claims | 17 | 0 | 2 | 0 | 19 |
| 23 | Communication, support, quality and SLA | 8 | 0 | 1 | 0 | 9 |
| 24 | Analytics, reporting, AI and workflow automation | 2 | 1 | 11 | 0 | 14 |
| 25 | Files, documents, search, imports and exports | 9 | 0 | 0 | 0 | 9 |
| **Total** | | **172** | **16** | **41** | **3** | **232** |

### 1.2 Most important findings

1. **Organization activation cannot complete through the API.** Activation requires an active default branch (`f/organizations/usecases/submission_activation.go:373-384`), but nothing creates a branch: no `/branches` route and no production caller of `BranchRepository.Create`. Because `middleware.RequireOperationalOrganization` blocks every non-onboarding write while the organization is not ACTIVE, every operational module depends on this gap (unless branches are seeded directly in the database).
2. **Section 9 access administration is read-only beyond invitations.** You can list staff, roles and permissions, invite staff, and suspend or reactivate them. You cannot edit staff, create or edit custom roles, grant or revoke role bindings, resend or cancel invitations, manage approval policies, or delegate approvals. Repository and usecase code exists for several of these (`UpdateStaff`, `ReplaceRoleBindings`, `CreateRole`/`UpdateRole`, `ResendInvitation`) but none of it is routed. All bindings are organization-scoped.
3. **Managed providers (Sec. 10).** Only list, create, get and suspend are routed. Reactivate, remove, PATCH, capabilities, policies, resend-login, availability and declines are missing (`ChangeStatus` supports reactivate and remove, but the routes are not mounted). `POST /providers` can only succeed for HAULING: the DISPATCH and TAXI provider contracts are never wired, so they always fail closed (`platformclients/contracts.go:375-384`; only `app/wiring.go:271` sets a real contract).
4. **Section 17 workforce is substantially real** (shifts with overlap and leave checks, check-in/out, leave approval, qualifications with verification, training courses and completion). Gaps: performance snapshots are computed only by a default-off worker and only from private assignments, so marketplace responses are ignored. Shift templates, training enrollments and provider self-view are missing.
5. **Section 22 finance is substantially real** (invoices, credit notes, receivables with a payment-event projection, expenses, budgets, claims, compensation rules, payroll calculate/approve/post/reverse with an outcome-unknown read-back to Wallet, subcontractor settlements, provider statements). Missing: profitability and the receivables ageing view. Payroll posting needs `FINANCE_POSTING_ENABLED` plus a configured Wallet adapter.
6. **Section 24 is essentially unbuilt.** Reports, automation (a job type that always fails permanently), recommendations and every AI capability are missing. Only exports, candidate scoring and a `/dashboard` aggregate exist.
7. **Provider-facing rights depend on configuration.** Provider accept/decline routes (assignments and delegated offers) are mounted only when `FLEET_MARKETPLACE_ALLOWED_CALLERS` is non-empty, and its default is empty. Offer ingestion (`FLEET_MARKETPLACE_EVENT_INGESTION_ENABLED`) and assignment expiry/reassignment workers also default off. Provider self-service sub-wallet routes exist in code and in riskregistry but are never mounted (`RegisterProviderRoutes` is never called).
8. **Almost every module is default-off.** 174 of the 179 boolean flags default to `false` (Section 3 of this report). On a default deployment, Sections 13-25 answer 404 on reads or 403 on writes, by design.
9. **The riskregistry is not complete, contrary to its header and CI claim.** 36 mounted routes have no registry entry, and 21 registry entries are not mounted by default (Section 4).
10. **The self-reported coverage file (`docs/master-spec-release1-coverage.json`) marks Sections 37, 38, 39 and 52-55 "PASS".** That is contradicted by findings 1-3 and 6, and by the missing detailed endpoints listed below. It does honestly mark AI/automation and Taxi as deferred.

### 1.3 All MISSING / PARTIAL items

| Row | Status | Requirement |
|---|---|---|
| 8.3 | MISSING | POST /auth/login (password login for org users) |
| 8.10 | MISSING | Create headquarters/default branch (stage 5) |
| 8.11 | PARTIAL | Configure timezone, currency, territories, initial policies |
| 8.12 | PARTIAL | Activate only after checks + wallet provisioning |
| 8.15 | MISSING | Branches / locations / departments / teams / cost-centres APIs (37.3, 59.2) |
| 9.3 | MISSING | Invitation resend / cancel / list (`/users/invitations/{id}/resend`) |
| 9.4 | MISSING | GET/PATCH /staff/{id} (branch, department, status) |
| 9.7 | MISSING | POST /roles, PATCH /roles/{id} (custom roles, version check) |
| 9.8 | MISSING | POST/DELETE /role-bindings, PUT /users/{id}/roles |
| 9.9 | PARTIAL | Branch/department-scoped role bindings |
| 9.12 | MISSING | Approval policies CRUD (/approval-policies) |
| 9.13 | MISSING | Approval delegations (/approval-delegations) |
| 10.2 | PARTIAL | POST /providers (resolve-or-provision in owning provider service) |
| 10.4 | MISSING | PATCH /providers/{id} |
| 10.6 | MISSING | POST /providers/{id}/reactivate |
| 10.7 | MISSING | POST /providers/{id}/remove (end membership) |
| 10.8 | MISSING | POST /providers/{id}/resend-login |
| 10.9 | MISSING | PUT /providers/{id}/capabilities |
| 10.10 | MISSING | PUT /providers/{id}/policies |
| 10.11 | MISSING | GET /providers/{id}/availability |
| 10.12 | MISSING | GET /providers/{id}/declines, /branches, /vehicles, /service-roles (39.3) |
| 11.13 | PARTIAL | UNSAFE_VEHICLE/VEHICLE_DEFECT decline -> safety/inspection workflow |
| 12.2 | PARTIAL | Execution creation in source service after provider acceptance |
| 12.3 | PARTIAL | Trip lifecycle projection list/detail (`/trips`) |
| 13.7 | PARTIAL | Provider self-service (own sub-wallet, statement, withdrawal) |
| 14.10 | MISSING | Leads + conversion (`/leads`, `/leads/{id}/convert`, 43.3/59.6) |
| 15.3 | MISSING | PATCH /orders/{id} |
| 15.10 | PARTIAL | Private-order execution through existing provider service after provider acceptance |
| 16.12 | PARTIAL | Geofence events (derived; `GET /geofence-events`) |
| 16.14 | MISSING | Rescue cases (`/rescue-cases`, resolve) |
| 17.10 | PARTIAL | GET /performance/providers/{id} (explainable) |
| 17.13 | MISSING | Shift templates (`/shift-templates`, 46.3) |
| 17.14 | MISSING | Training enrollments (`/training/enrollments`, `/complete`, 46.3/59.4) |
| 17.15 | MISSING | Provider self-view of schedule, qualifications, training, performance (46.2) |
| 18.11 | PARTIAL | UNSAFE_VEHICLE decline -> inspection/work-order draft |
| 18.12 | MISSING | Unassign provider, meter readings, tyres, defects resource, asset costs/history (47.3/48.3) |
| 19.12 | PARTIAL | POST /custody-events |
| 19.13 | MISSING | Incident triage/investigations, proof verify, compliance checks (49.3) |
| 20.3 | PARTIAL | Taxi/passenger records |
| 20.13 | PARTIAL | Disabled modules must not expose routes (20.1) |
| 20.14 | MISSING | Capability schemas / order specialized-data endpoints (50.3) |
| 21.11 | MISSING | Zones/bins, warehouse receipts with lines, claimable leased tasks, pick waves/tasks, packing units (51.3) |
| 22.5 | MISSING | Receivables ageing view (52.3 `/receivables/ageing`) |
| 22.17 | MISSING | Profitability (`/profitability`, 52.3) |
| 23.8 | MISSING | Operational exceptions (`/exceptions`, escalate, resolve; 53.3) |
| 24.1 | PARTIAL | Executive dashboard (`/dashboards/executive`, branch filters, freshness) |
| 24.2 | MISSING | Operations report |
| 24.3 | MISSING | Fleet report |
| 24.4 | MISSING | Workforce report |
| 24.5 | MISSING | Customer reports |
| 24.8 | MISSING | Delay prediction |
| 24.9 | MISSING | Predictive maintenance |
| 24.10 | MISSING | Fraud/anomaly detection |
| 24.11 | MISSING | Document assistance |
| 24.12 | MISSING | Natural-language assistant |
| 24.13 | MISSING | GET/POST /automation/rules, GET /automation/runs |
| 24.14 | MISSING | GET /recommendations, accept, dismiss |

## 2. Per-section tables

## Section 8 - Authentication, company onboarding and lifecycle (detail: Sec. 37, 59.2)

| # | Requirement | Status | Evidence (route -> handler/usecase) | Gate (default) |
|---|---|---|---|---|
| 8.1 | POST /auth/phone/start (start owner challenge) | IMPLEMENTED | Different path: `POST /auth/start`, `/auth/sign-up/start`, `/auth/sign-in/start`, `/auth/resend` -> f/auth/http/handlers.go, f/auth/usecases/auth_service.go (email+phone+emailed OTP) | none (public) |
| 8.2 | POST /auth/phone/verify | IMPLEMENTED | `POST /auth/verify` -> f/auth/usecases/auth_service.go | none |
| 8.3 | POST /auth/login (password login for org users) | MISSING | No route. Service is deliberately passwordless (app.go comment: "never accept a password field") | - |
| 8.4 | POST /auth/refresh, POST /auth/logout | IMPLEMENTED | f/auth/http/handlers.go; sessions in f/auth/repositories/sessions.go | none |
| 8.5 | POST /organizations (draft) | IMPLEMENTED | f/organizations/http/handlers.go createOrganization -> usecases/onboarding_service.go (org + owner membership + default settings + stages, audit, outbox) | none |
| 8.6 | PATCH /organizations/{id} (legal/operating profile) | IMPLEMENTED | Method differs: `PUT /organizations/:organizationId`, `PUT .../profile`, `PUT .../capabilities`, `POST/PUT .../representatives`, `PUT .../stages/:stage` | none |
| 8.7 | POST /organizations/{id}/documents (business documents via media-file-service) | IMPLEMENTED | `POST .../documents`, `.../documents/upload-authorization`, `.../upload-content`, `GET .../documents`, `.../preview`, `DELETE .../documents/:documentId`; media is EXTERNAL (media-file-service client must be configured) | client config |
| 8.8 | POST /organizations/{id}/submit -> verification-compliance-service | IMPLEMENTED | `POST .../submit`, `GET .../verification` -> usecases/submission_activation.go; decision itself is EXTERNAL | verification client config |
| 8.9 | GET /organizations/{id}/onboarding-status | IMPLEMENTED | handlers.go onboardingStatus | none |
| 8.10 | Create headquarters/default branch (stage 5) | MISSING | No route and no production code path calls `BranchRepository.Create` (f/organizations/repositories/branches.go:28; only caller-free). Profile "HEADQUARTERS" stage only stores an address. Activation gate `active_default_branch` (usecases/submission_activation.go:373-384) therefore can never pass through the API | - |
| 8.11 | Configure timezone, currency, territories, initial policies | PARTIAL | Timezone/currency captured at draft; settings row created with defaults (onboarding_service.go:364); no settings API (`/organization-settings` absent). Operating mode settable via `GET/PUT /fleet-partner/operating-mode` (f/fleetpartner/http.go) | none |
| 8.12 | Activate only after checks + wallet provisioning | PARTIAL | `TryActivate` (submission_activation.go:405) real, `GET/POST .../wallet-provisioning[/retry]`; but blocked in practice by 8.10. Wallet provisioning is EXTERNAL (payment-wallet-service) | wallet client config |
| 8.13 | Lifecycle DRAFT->...->ACTIVE<->SUSPENDED->CLOSED | EXTERNAL | Transition matrix in usecases/lifecycle.go; review/suspend decisions come from verification/admin services, no corporate route | - |
| 8.14 | POST /organizations/{id}/close-request + controlled closure | IMPLEMENTED | `close-request`, `closure/assess\|approve\|reject\|execute`, `closure-status` -> usecases/closure.go | none |
| 8.15 | Branches / locations / departments / teams / cost-centres APIs (37.3, 59.2) | MISSING | Repos exist (branches.go, departments.go) but no routes; teams, cost-centres, locations absent | - |
| 8.16 | Sessions list/revoke, step-up (56.3) | IMPLEMENTED | `GET /sessions`, `DELETE /sessions/:sessionId`, `POST /auth/step-up/start\|verify`, `POST /auth/select-organization` | none |

## Section 9 - Company users, roles, permissions and approvals (detail: Sec. 38, 59.3)

| # | Requirement | Status | Evidence | Gate (default) |
|---|---|---|---|---|
| 9.1 | GET /staff | IMPLEMENTED | f/organizations/http/handlers.go:88 listStaff -> f/staff/usecases/membership_flows.go ListStaff | none |
| 9.2 | POST /staff/invitations (single-use hashed, expiring) | IMPLEMENTED | handlers.go:89 -> f/staff/usecases/staff_service.go Invite/AcceptByToken; roles+branches captured on invite | none (email delivery via notification-service = EXTERNAL) |
| 9.3 | Invitation resend / cancel / list (`/users/invitations/{id}/resend`) | MISSING | Usecases `ResendInvitation`, `CancelInvitation`, `ListInvitations` exist (staff_service.go:367-411) but are not routed | - |
| 9.4 | GET/PATCH /staff/{id} (branch, department, status) | MISSING | `GetStaff`, `UpdateStaff` usecases exist (membership_flows.go:26,49) but no route | - |
| 9.5 | POST /staff/{id}/suspend, /reactivate | IMPLEMENTED | handlers.go:90-91 -> ChangeStaffStatus (role-version bump) | none |
| 9.6 | GET /roles, GET /permissions | IMPLEMENTED | handlers.go:85-86 -> f/staff/repositories/roles.go ListRoles/ListPermissions | none |
| 9.7 | POST /roles, PATCH /roles/{id} (custom roles, version check) | MISSING | `RoleRepository.CreateRole/UpdateRole/ArchiveRole` exist (roles.go:130-169) but no usecase or route | - |
| 9.8 | POST/DELETE /role-bindings, PUT /users/{id}/roles | MISSING | `ReplaceRoleBindings` usecase exists (membership_flows.go:161) but no route; bindings are only created at invitation acceptance | - |
| 9.9 | Branch/department-scoped role bindings | PARTIAL | Authorizer accepts `Target{BranchID}` (f/accesscontrol); every binding created in code uses `ScopeOrganization` (staff_service.go:219, membership_flows.go:201) | - |
| 9.10 | GET /approvals (pending + history), detail, decisions, history | IMPLEMENTED | Path differs: `/approvals/requests`, `/:approvalId`, `/decisions`, `/history` -> f/approvals/http/handlers.go, f/approvals/usecases/service.go | none |
| 9.11 | POST /approvals/{id}/approve, /reject (different approver, quorum) | IMPLEMENTED | `/approvals/requests/:approvalId/approve\|reject\|cancel\|execute`, `POST /approvals/requests`; self-approval + quorum enforced in usecases/service.go Decide | none |
| 9.12 | Approval policies CRUD (/approval-policies) | MISSING | Policies are auto-created from templates (repositories/approvals.go:75-93); no API | - |
| 9.13 | Approval delegations (/approval-delegations) | MISSING | No table use, route or usecase found | - |
| 9.14 | Step-up for high-risk actions | IMPLEMENTED | `/auth/step-up/*`; consumed by legal hold, security settings, integrations, retention (app/wiring.go stepUpAdapter) | none |
| 9.15 | Default-deny, role-version invalidation of cached decisions | IMPLEMENTED | f/accesscontrol authorizer; tenant middleware role version | none |

## Section 10 - Organization-managed provider accounts (detail: Sec. 39, 59.4)

| # | Requirement | Status | Evidence | Gate (default) |
|---|---|---|---|---|
| 10.1 | GET /providers | IMPLEMENTED | f/organizations/http/handlers.go:93 -> f/providers repositories | none |
| 10.2 | POST /providers (resolve-or-provision in owning provider service) | PARTIAL | handlers.go:94 -> f/providers/usecases/provider_service.go CreateProvider. Only HAULING has a live contract (app/wiring.go:271, requires FLEET_HAULING_INTEGRATION_ENABLED + client). DISPATCH and TAXI always resolve to `UnconfiguredProvider` (platformclients/contracts.go:375-384) and fail closed | FLEET_HAULING_INTEGRATION_ENABLED (false) |
| 10.3 | GET /providers/{id} (+history, provisioning-status) | IMPLEMENTED | handlers.go:95-98 | none |
| 10.4 | PATCH /providers/{id} | MISSING | no route | - |
| 10.5 | POST /providers/{id}/suspend | IMPLEMENTED | handlers.go:96 -> usecases/lifecycle.go ChangeStatus (revokes eligibility in owning service) | none |
| 10.6 | POST /providers/{id}/reactivate | MISSING | `ChangeStatus` supports it but only SUSPENDED is routed. Member-layer `POST /managed-members/:memberId/reinstate` exists (separate aggregate, f/managedmembers) | FLEET_MULTI_SERVICE_PROVIDER_LINKS_ENABLED (false) |
| 10.7 | POST /providers/{id}/remove (end membership) | MISSING | `ChangeStatus` supports REMOVED; not routed | - |
| 10.8 | POST /providers/{id}/resend-login | MISSING | no route | - |
| 10.9 | PUT /providers/{id}/capabilities | MISSING | no route | - |
| 10.10 | PUT /providers/{id}/policies | MISSING | no route | - |
| 10.11 | GET /providers/{id}/availability | MISSING | no route (tracking projection is per trip, not per provider) | - |
| 10.12 | GET /providers/{id}/declines, /branches, /vehicles, /service-roles (39.3) | MISSING | no routes (asset->provider assignment exists only as `POST /assets/:assetId/assignments`) | - |
| 10.13 | Mandatory sub-wallet on activation | IMPLEMENTED | usecases/lifecycle.go EnsureSubWallet/TryActivate; wallet is EXTERNAL | wallet client config |
| 10.14 | Provisioning saga recovery | IMPLEMENTED | f/providers/usecases/reconciler.go | worker |
| 10.15 | Multi-service member layer (managed-members, managed-assets, consent, links, suspend/reinstate) | IMPLEMENTED | f/managedmembers/http.go, service.go (NOT in riskregistry) | FLEET_MULTI_SERVICE_PROVIDER_LINKS_ENABLED (false) |
| 10.16 | Provider membership summary / corporate context for execution services | IMPLEMENTED | internal `GET /providers/:providerId/services/:providerService/membership-summary`, `/providers/:providerId/corporate-context` (not in riskregistry) | PROVIDER_MEMBERSHIP_ALLOWED_CALLERS (default: dispatch, taxi, hauling, support-dispute) |

## Section 11 - Request acceptance, assignment, provider accept/decline (detail: Sec. 40, 59.5)

| # | Requirement | Status | Evidence | Gate (default) |
|---|---|---|---|---|
| 11.1 | Operating modes CENTRALIZED/DELEGATED/HYBRID | IMPLEMENTED | `GET/PUT /fleet-partner/operating-mode` -> f/fleetpartner (not in riskregistry) | FLEET_OPERATING_MODE_ENFORCEMENT_ENABLED (true) |
| 11.2 | Dispatcher accepts request for org (`/requests/{id}/accept-for-organization`) | IMPLEMENTED | Path differs: `POST /marketplace/offers/:offerId/accept` -> f/marketplace/http/manifest.go + handlers.go -> usecases/offer_response.go (not in riskregistry) | flags hard-wired on (app/marketplace_wiring.go:100) |
| 11.3 | Dispatcher declines opportunity | IMPLEMENTED | `POST /marketplace/offers/:offerId/decline` | - |
| 11.4 | Eligible candidates | IMPLEMENTED | `GET /marketplace/requests/:projectionId/candidates` | - |
| 11.5 | POST /requests/{id}/assign | IMPLEMENTED | `POST /marketplace/requests/:projectionId/assign` -> usecases/assign.go | - |
| 11.6 | GET /assignments, /assignments/{id} | IMPLEMENTED | `GET /marketplace/assignments[/:assignmentId][/history]` | - |
| 11.7 | POST /assignments/{id}/cancel, /reassign | IMPLEMENTED | `POST /marketplace/assignments/:id/cancel\|reassign`; reassignment writes off unless flag | FLEET_REASSIGNMENT_ENABLED (false) |
| 11.8 | Provider accepts/declines assignment (`/assignments/{id}/accept\|decline`) | IMPLEMENTED | Internal relay only: `/internal/fleet-partner/v1/assignments/:assignmentId/provider-accept\|provider-decline\|provider-acknowledge` -> marketplace/http/provider_response_handlers.go. Routes are not mounted at all when the caller allowlist is empty (cmd/marketplace_routes.go:39) | FLEET_MARKETPLACE_ALLOWED_CALLERS ("" -> not mounted) |
| 11.9 | Provider accepts/declines delegated offer (`/marketplace-offers/{id}/accept\|decline`) | IMPLEMENTED | Internal: `/marketplace-offers/:offerId/provider-accept\|provider-decline`, provider reads `/providers/:providerId/offers` | same as 11.8 |
| 11.10 | Decline reason catalogue, mandatory reason, OTHER free text | IMPLEMENTED | f/marketplace/models/decline.go | - |
| 11.11 | Response deadline -> EXPIRED -> REASSIGNMENT_REQUIRED; dispatcher notification / auto-reassign | IMPLEMENTED | app/reconciliation_workers.go:141,181 | ASSIGNMENT_EXPIRY_WORKER_ENABLED (false), REASSIGNMENT_WORKER_ENABLED (false), FLEET_AUTOMATIC_REASSIGNMENT_ENABLED (false) |
| 11.12 | Race protection / idempotency / 409 assignment_already_resolved | IMPLEMENTED | CAS + idempotency in marketplace usecases/repositories | - |
| 11.13 | UNSAFE_VEHICLE/VEHICLE_DEFECT decline -> safety/inspection workflow | PARTIAL | f/assets/safety_declines.go reads only `fleet_partner_private_assignments`; marketplace assignment declines are not consumed | SECTION18_WORKERS_ENABLED (false) |

## Section 12 - Existing marketplace request and trip integration (detail: Sec. 41)

| # | Requirement | Status | Evidence | Gate (default) |
|---|---|---|---|---|
| 12.1 | Organization projection of source opportunities (canonical refs) | IMPLEMENTED | internal `POST /events` -> marketplace/http/event_handlers.go -> usecases trip_projection_ingestion.go | FLEET_MARKETPLACE_EVENT_INGESTION_ENABLED (false) + allowed callers ("") |
| 12.2 | Execution creation in source service after provider acceptance | PARTIAL | usecases/execution.go; Dispatch needs FLEET_DISPATCH_EXECUTION_START_ENABLED + DISPATCH_SERVICE_URL, Hauling needs FLEET_HAULING_INTEGRATION_ENABLED, Taxi FLEET_TAXI_INTEGRATION_ENABLED; canonical trip owned by EXTERNAL services | all three (false) |
| 12.3 | Trip lifecycle projection list/detail (`/trips`) | PARTIAL | No `/trips`; `GET /tracking`, `/tracking/:canonicalTripReference` (f/dispatch/control.go) + internal `POST /tracking/events` | TRACKING_PROJECTIONS_ENABLED (false) |
| 12.4 | Projection gap detection / reconciliation | IMPLEMENTED | app/reconciliation_workers.go, marketplace reconciliation | FLEET_MARKETPLACE_RECONCILIATION_ENABLED (false) |
| 12.5 | Provider-facing data minimization (no fare/commission) | IMPLEMENTED | internal provider read routes; docs/provider-data-minimization.md | FLEET_TYPED_FINANCIAL_VISIBILITY_ENABLED (true) |
| 12.6 | Settlement projection display | IMPLEMENTED | `/finance/projections`, `/finance/operations` (f/finance) + internal `/finance/events` | FLEET_FINANCE_EVENT_INGESTION_ENABLED (false); FLEET_FINANCE_INTEGRATION_ENABLED (false) |
| 12.7 | Completed-work ingestion from execution services | IMPLEMENTED | internal `POST /completed-work` -> f/fleetpartner (not in riskregistry) | FLEET_COMPLETED_WORK_PROJECTION_ENABLED (false) |
| 12.8 | Ledger/settlement execution | EXTERNAL | payment-wallet-service | - |

## Section 13 - Company wallet and provider sub-wallets (detail: Sec. 42)

| # | Requirement | Status | Evidence | Gate (default) |
|---|---|---|---|---|
| 13.1 | GET /wallet | IMPLEMENTED | f/companywallet/http/handlers.go -> usecases/service.go GetWallet | ORGANIZATION_WALLET_READ_ENABLED (false) |
| 13.2 | GET /wallet/transactions (+/:transactionId) | IMPLEMENTED | same | ORGANIZATION_WALLET_READ_ENABLED (false) |
| 13.3 | GET /providers/{id}/sub-wallet | IMPLEMENTED | usecases/service.go GetSubWallet | PROVIDER_SUBWALLET_ENABLED (false) |
| 13.4 | POST /providers/{id}/sub-wallet/allocations (+list, approve, reject, replay) | IMPLEMENTED | usecases/allocations.go | PROVIDER_ALLOCATION_ENABLED (false) |
| 13.5 | GET /providers/{id}/sub-wallet/statement | IMPLEMENTED | usecases/statement.go | PROVIDER_SUBWALLET_ENABLED (false) |
| 13.6 | POST /providers/{id}/sub-wallet/withdrawals (+approve, reject, replay) | IMPLEMENTED | usecases/withdrawals.go | PROVIDER_WITHDRAWAL_ENABLED (false) |
| 13.7 | Provider self-service (own sub-wallet, statement, withdrawal) | PARTIAL | Handlers exist (`/provider/sub-wallet*`, handlers.go:88-98) and are listed in riskregistry, but `RegisterProviderRoutes` is never called -> not mounted | - |
| 13.8 | Withdrawal policy GET/PUT /wallet/policy | IMPLEMENTED | usecases/service.go Get/SetPolicy | - |
| 13.9 | Projection refresh / reconciliation | IMPLEMENTED | usecases/workers.go | WALLET_PROJECTION_REFRESH_ENABLED (false), WALLET_RECONCILIATION_ENABLED (false) |
| 13.10 | Ledger, balances, transfers | EXTERNAL | payment-wallet-service (fail-closed `UnconfiguredWallet` when not configured) | client config; FLEET_COMPANY_WALLET_KILL_SWITCH (false) |
| 13.11 | Managed settlement instruction to Wallet | IMPLEMENTED | f/financeops/managed_settlement.go, f/fleetpartner | FLEET_SETTLEMENT_INSTRUCTION_ENABLED (false), FLEET_COMPENSATION_RESOLVER_ENABLED (false) |

## Section 14 - CRM, sales, contracts and SLAs (detail: Sec. 43)

| # | Requirement | Status | Evidence | Gate (default) |
|---|---|---|---|---|
| 14.1 | GET/POST /customers, GET/PATCH /customers/{id} (+status, risk-status, archive, history) | IMPLEMENTED | f/crm/http/manifest.go -> f/crm/usecases/customers.go | CRM_ENABLED, CRM_CUSTOMER_WRITES_ENABLED (false) |
| 14.2 | POST /customers/{id}/contacts (+list/patch/archive/make-primary) | IMPLEMENTED | crm/usecases/contacts.go | CRM_CONTACT_WRITES_ENABLED (false) |
| 14.3 | POST /customers/{id}/locations (+list/patch/archive) | IMPLEMENTED | crm/usecases/locations.go | CRM_LOCATION_WRITES_ENABLED (false) |
| 14.4 | GET/POST /sales/opportunities (+hold/resume/won/lost/history) | IMPLEMENTED | crm/usecases/opportunities.go | CRM_OPPORTUNITIES_ENABLED (false) |
| 14.5 | GET/POST /quotations, submit, accept (+issue/reject/cancel/supersede/versions, secure customer link) | IMPLEMENTED | crm/usecases/quotations.go, quotation_issue.go, quotation_customer.go; public `/fleet-partner/public/v1/quotations/:id/view\|accept\|reject` | QUOTATIONS_ENABLED, QUOTATION_ISSUE_ENABLED, QUOTATION_ACCEPTANCE_ENABLED (false) |
| 14.6 | Quotation acceptance creates order draft | IMPLEMENTED | `POST /orders/from-quotation/:acceptedVersionId` (f/orders) | ORDERS_* (false) |
| 14.7 | GET/POST /contracts, activate (+submit/suspend/resume/renew/terminate/versions/documents) | IMPLEMENTED | crm/usecases/contracts.go, contract_documents.go | CONTRACTS_ENABLED, CONTRACT_ACTIVATION_ENABLED (false) |
| 14.8 | GET/POST /service-level-agreements (+activate/suspend/terminate/observations) | IMPLEMENTED | crm/usecases/slas.go; internal `POST /crm/sla-observations` | SLA_ENABLED (false) |
| 14.9 | Customer credit profiles (hold/release) | IMPLEMENTED | `GET/PUT /customers/:id/credit-profile`, `/hold`, `/release` -> crm/usecases/credit.go | CUSTOMER_CREDIT_ENABLED (false) |
| 14.10 | Leads + conversion (`/leads`, `/leads/{id}/convert`, 43.3/59.6) | MISSING | no route | - |
| 14.11 | Pricing neutrality (manual lines, no formula) | IMPLEMENTED | quotation line items | - |

## Section 15 - Service catalogue, orders, shipments, packages (detail: Sec. 44)

| # | Requirement | Status | Evidence | Gate (default) |
|---|---|---|---|---|
| 15.1 | GET/POST /service-catalog, GET/PATCH /{id} (+activate/deactivate/archive) | IMPLEMENTED | f/servicecatalog | SERVICE_CATALOG_ENABLED, SERVICE_CATALOG_WRITES_ENABLED (false) |
| 15.2 | GET/POST /orders, GET /orders/{id} (+stops, history) | IMPLEMENTED | f/orders/http/manifest.go -> orders/usecases/service.go | ORDERS_ENABLED, ORDERS_WRITES_ENABLED (false) |
| 15.3 | PATCH /orders/{id} | MISSING | no route | - |
| 15.4 | POST /orders/{id}/confirm, /cancel | IMPLEMENTED | orders manifest | ORDERS_WRITES_ENABLED (false) |
| 15.5 | GET/POST /shipments, GET/PATCH /{id} (+cargo-items) | IMPLEMENTED | f/shipments | SHIPMENTS_ENABLED, SHIPMENTS_WRITES_ENABLED (false) |
| 15.6 | POST /shipments/{id}/packages; POST /packages/{id}/scan (+transition, custody, history) | IMPLEMENTED | f/packages/usecases/service.go | PACKAGES_ENABLED, PACKAGES_WRITES_ENABLED, PACKAGE_SCANNING_ENABLED (false) |
| 15.7 | GET/POST /manifests, POST /{id}/close (+items, seal-amendments, cancel, patch) | IMPLEMENTED | f/manifests | MANIFESTS_ENABLED, MANIFESTS_WRITES_ENABLED (false) |
| 15.8 | POST /orders/imports, GET /orders/imports/{id} | IMPLEMENTED | f/bulkimports | BULK_IMPORTS_ENABLED, _WRITES_, _WORKER_ (false) |
| 15.9 | GET/POST /recurring-orders (+activate/deactivate/replay) | IMPLEMENTED | f/recurringorders | RECURRING_ORDERS_ENABLED, _WRITES_, _WORKER_ (false) |
| 15.10 | Private-order execution through existing provider service after provider acceptance | PARTIAL | `/private-assignments*`, `/private-executions*`, internal private accept/decline (f/privateoperations); dispatch/hauling adapters gated; taxi not wired | PRIVATE_ORDERS_ENABLED, PRIVATE_ASSIGNMENTS_WRITES_ENABLED, PRIVATE_DISPATCH_EXECUTION_ENABLED, PRIVATE_HAULING_EXECUTION_ENABLED, PRIVATE_TAXI_EXECUTION_ENABLED, PRIVATE_EXECUTION_WORKER_ENABLED (all false) |
| 15.11 | Order state machine incl. AWAITING_APPROVAL/PLANNED/ASSIGNED/PARTIALLY_COMPLETED | IMPLEMENTED | f/orders/models/models.go:74-83 | - |

## Section 16 - Dispatch, routes, live control and tracking (detail: Sec. 45)

| # | Requirement | Status | Evidence | Gate (default) |
|---|---|---|---|---|
| 16.1 | GET /dispatch/board | IMPLEMENTED | f/dispatch/http.go:51 -> control.go Board | CONTROL_TOWER_ENABLED (false) |
| 16.2 | GET /dispatch/unassigned | IMPLEMENTED | control.go Unassigned | DISPATCH_PLANNING_ENABLED (false) |
| 16.3 | GET /dispatch/candidates (explainable) | IMPLEMENTED | service.go GenerateCandidates | DISPATCH_PLANNING_ENABLED (false) |
| 16.4 | POST /dispatch/plans (+list/get/patch/approve/publish/cancel) | IMPLEMENTED | service.go CreatePlan.. | DISPATCH_PLANNING_ENABLED (false) |
| 16.5 | POST /routes, PATCH /routes/{id} (+stops CRUD, approve, cancel) | IMPLEMENTED | service.go CreateRoute/EditRoute/AddStop | ROUTE_PLANNING_ENABLED (false) |
| 16.6 | POST /routes/{id}/optimize (provider-neutral job) | IMPLEMENTED | service.go Optimize + workers.go optimization submit/read-back; optimizer itself EXTERNAL (optimizer_client.go) | ROUTE_OPTIMIZATION_ENABLED (false) |
| 16.7 | POST /routes/{id}/publish (creates assignments needing accept/decline) | IMPLEMENTED | service.go PublishRoute | ROUTE_PUBLICATION_ENABLED (false) |
| 16.8 | Re-optimize remaining work (`/routes/{id}/replan`) | IMPLEMENTED | service.go Replan | ROUTE_PLANNING_ENABLED (false) |
| 16.9 | GET /control-tower | IMPLEMENTED | control.go ControlTower | CONTROL_TOWER_ENABLED (false) |
| 16.10 | Tracking projections (last-known, freshness) | IMPLEMENTED | `GET /tracking[/:ref]`, internal `POST /tracking/events` | TRACKING_PROJECTIONS_ENABLED (false) |
| 16.11 | GET/POST /geofences (+patch, activate/deactivate) | IMPLEMENTED | control.go | GEOFENCES_ENABLED (false) |
| 16.12 | Geofence events (derived; `GET /geofence-events`) | PARTIAL | Derived by worker into fleet_partner_geofence_events (workers.go:172-245); no read route | FLEET_SECTION16_WORKERS_ENABLED (false) |
| 16.13 | GET /alerts, acknowledge, resolve | IMPLEMENTED | control.go | OPERATIONAL_ALERTS_ENABLED (false) |
| 16.14 | Rescue cases (`/rescue-cases`, resolve) | MISSING | no route/aggregate (breakdowns exist under Section 18) | - |

## Section 17 - Workforce, shifts, training and performance (detail: Sec. 46, 59.4)

| # | Requirement | Status | Evidence | Gate (default) |
|---|---|---|---|---|
| 17.1 | Workforce profiles (employment relationship, supervisor) | IMPLEMENTED | `GET/POST /workforce/profiles`, `GET/PATCH /:profileId`, `/archive` -> f/workforce/http.go, service.go CreateProfile/UpdateProfile | WORKFORCE_ENABLED (false) |
| 17.2 | GET/POST /shifts (+GET/PATCH /{id}, publish, cancel) | IMPLEMENTED | service.go CreateShift/UpdateShift (timezone stored, version CAS, history) | SHIFTS_ENABLED (false) |
| 17.3 | POST /shifts/{id}/assignments | IMPLEMENTED | service.go AssignShift: row lock, overlap + approved-leave conflict check, capacity | SHIFTS_ENABLED (false) |
| 17.4 | POST /shift-assignments/{id}/check-in, /check-out (+GET, cancel) | IMPLEMENTED | service.go AssignmentAction | SHIFTS_ENABLED (false) |
| 17.5 | GET/POST /leave (+GET, approve, reject, cancel) | IMPLEMENTED | service.go CreateLeave/LeaveAction (approval checks shift conflicts) | LEAVE_ENABLED (false) |
| 17.6 | GET/POST /qualifications (+GET/PATCH) | IMPLEMENTED | service.go CreateQualification/UpdateQualification | QUALIFICATIONS_ENABLED (false) |
| 17.7 | POST /providers/{id}/qualifications (+list, verify, reject, revoke) | IMPLEMENTED | service.go AddProviderQualification/QualificationAction (evidence via media) | QUALIFICATIONS_ENABLED (false) |
| 17.8 | GET/POST /training/courses (+GET/PATCH) | IMPLEMENTED | service.go CreateCourse/UpdateCourse | TRAINING_ENABLED (false) |
| 17.9 | POST /training/courses/{id}/complete | IMPLEMENTED | service.go CompleteCourse (score, certificate media, expiry) | TRAINING_ENABLED (false) |
| 17.10 | GET /performance/providers/{id} (explainable) | PARTIAL | Read-only list of snapshots (service.go ListPerformance). Snapshots are produced only by the maintenance worker (workers.go GenerateDuePerformance) and computed only from `fleet_partner_private_assignments` (service.go:845) - marketplace assignment responses are ignored | PERFORMANCE_SNAPSHOTS_ENABLED (false) + SECTION17_WORKERS_ENABLED (false) |
| 17.11 | Leave/qualification/training/shift feed assignment eligibility | IMPLEMENTED | workforce.Evaluate used by dispatch (app/section16_wiring.go:72) and private ops (app/privateoperations_wiring.go:190) | - |
| 17.12 | Decline metrics distinguish categories, no automatic discipline | IMPLEMENTED | service.go declineCategory; no penalty path found | - |
| 17.13 | Shift templates (`/shift-templates`, 46.3) | MISSING | no route/table use | - |
| 17.14 | Training enrollments (`/training/enrollments`, `/complete`, 46.3/59.4) | MISSING | completion recorded directly against course; no enrollment/assignment | - |
| 17.15 | Provider self-view of schedule, qualifications, training, performance (46.2) | MISSING | no provider-facing or internal route | - |
| 17.16 | Qualification-expiry / maintenance worker | IMPLEMENTED | f/workforce/workers.go | SECTION17_WORKERS_ENABLED (false) |

## Section 18 - Fleet, assets, inspections, maintenance and fuel (detail: Sec. 47-48)

| # | Requirement | Status | Evidence | Gate (default) |
|---|---|---|---|---|
| 18.1 | GET/POST /assets, GET/PATCH /assets/{id} | IMPLEMENTED | f/assets/http.go:30 -> service.go | ASSETS_ENABLED (false) |
| 18.2 | POST /assets/{id}/archive (+retire) | IMPLEMENTED | service.go UpdateAsset | ASSETS_ENABLED (false) |
| 18.3 | POST /assets/{id}/assignments | IMPLEMENTED | service.go Assign (overlap check) | ASSET_ASSIGNMENTS_ENABLED (false) |
| 18.4 | Asset documents | IMPLEMENTED | `GET/POST /assets/:id/documents` | ASSETS_ENABLED (false) |
| 18.5 | GET/POST /inspections, /inspection-templates, POST /inspections/{id}/complete (critical fail -> BLOCKED + work order + event) | IMPLEMENTED | service.go CompleteInspection:490-538 | INSPECTIONS_ENABLED (false) |
| 18.6 | GET/POST /maintenance/schedules | IMPLEMENTED | operations.go SaveSchedule | MAINTENANCE_ENABLED (false) |
| 18.7 | GET/POST /maintenance/work-orders, approve, complete, return-to-service | IMPLEMENTED | operations.go UpdateWorkOrder/ReturnToService | MAINTENANCE_ENABLED (false) |
| 18.8 | GET/POST /breakdowns | IMPLEMENTED | operations.go SaveBreakdown | BREAKDOWNS_ENABLED (false) |
| 18.9 | GET/POST /inventory/parts, POST /inventory/stock-movements | IMPLEMENTED | operations.go SavePart/CreateMovement | PARTS_INVENTORY_ENABLED (false) |
| 18.10 | GET/POST /fuel-records (+approve/reject) | IMPLEMENTED | operations.go CreateFuel/FuelAction | FUEL_RECORDS_ENABLED (false) |
| 18.11 | UNSAFE_VEHICLE decline -> inspection/work-order draft | PARTIAL | assets/safety_declines.go (private assignments only, see 11.13) | SECTION18_WORKERS_ENABLED (false) |
| 18.12 | Unassign provider, meter readings, tyres, defects resource, asset costs/history (47.3/48.3) | MISSING | no routes | - |

## Section 19 - Compliance, safety, incidents and proof (detail: Sec. 49)

| # | Requirement | Status | Evidence | Gate (default) |
|---|---|---|---|---|
| 19.1 | GET /compliance/status | IMPLEMENTED | f/compliance/http.go -> service.go ListStatuses | COMPLIANCE_ENABLED (false) |
| 19.2 | GET/POST /compliance/requirements (+patch, activate, deactivate) | IMPLEMENTED | service.go CreateRequirement/UpdateRequirement | COMPLIANCE_ENABLED (false) |
| 19.3 | GET /providers/{id}/compliance, /assets/{id}/compliance | IMPLEMENTED | compliance/http.go | COMPLIANCE_ENABLED (false) |
| 19.4 | Evaluations + restrictions apply/clear | IMPLEMENTED | `POST /compliance/evaluations`, `/compliance/restrictions[/:id/clear]` -> service.go Evaluate/ApplyRestriction/ClearRestriction | COMPLIANCE_ENABLED (false) |
| 19.5 | Restriction propagation to execution services | IMPLEMENTED | service.go ProcessRestriction + workers; enforcement in execution services is EXTERNAL | RESTRICTION_PROPAGATION_ENABLED (false), SECTION19_WORKERS_ENABLED (false) |
| 19.6 | GET/POST /safety/reports (+patch, resolve) | IMPLEMENTED | f/safety/operations.go CreateReport/UpdateReport | SAFETY_REPORTS_ENABLED (false) |
| 19.7 | GET/POST /incidents (+patch) | IMPLEMENTED | operations.go CreateIncident/UpdateIncident | INCIDENTS_ENABLED (false) |
| 19.8 | POST /incidents/{id}/evidence | IMPLEMENTED | operations.go AddEvidence (media validation) | INCIDENTS_ENABLED (false) |
| 19.9 | POST /incidents/{id}/corrective-actions (+complete, verify) | IMPLEMENTED | operations.go CreateCorrectiveAction/Action | INCIDENTS_ENABLED (false) |
| 19.10 | GET/PUT /proof-policies | IMPLEMENTED | f/proof/service.go SavePolicy/ListPolicies | PROOF_POLICIES_ENABLED (false) |
| 19.11 | POST /proof-records (+GET list) | IMPLEMENTED | proof/service.go CreateRecord | PROOF_RECORDS_ENABLED (false) |
| 19.12 | POST /custody-events | PARTIAL | Package-scoped only: `POST/GET /packages/:packageId/custody` (f/packages); no generic custody-event resource | PACKAGES_WRITES_ENABLED (false) |
| 19.13 | Incident triage/investigations, proof verify, compliance checks (49.3) | MISSING | no routes | - |

## Section 20 - Specialized logistics modules (detail: Sec. 50)

| # | Requirement | Status | Evidence | Gate (default) |
|---|---|---|---|---|
| 20.1 | Organization-scoped capability enable/disable/list | IMPLEMENTED | `GET /specialized/capabilities`, `POST .../:code/enable\|disable` -> f/specialized/service.go SetCapability | SPECIALIZED_LOGISTICS_ENABLED (false) |
| 20.2 | Courier/last-mile records | IMPLEMENTED | validation.go SHIPPING_LABEL, DELIVERY_ATTEMPT, REDELIVERY... | COURIER_LAST_MILE_ENABLED (false) |
| 20.3 | Taxi/passenger records | PARTIAL | record types defined, but validation.go:74-76 refuses ("Taxi execution is disabled") | TAXI_PASSENGER_ENABLED (false) |
| 20.4 | Trucking/haulage | IMPLEMENTED | HAULAGE_PROFILE, LOADING_APPOINTMENT | TRUCKING_HAULAGE_ENABLED (false) |
| 20.5 | Heavy/specialized | IMPLEMENTED | HEAVY_PROFILE, PERMIT, ROUTE_SURVEY | HEAVY_SPECIALIZED_ENABLED (false) |
| 20.6 | Freight forwarding / cross-border | IMPLEMENTED | COUNTRY_LEG, CUSTOMS_DOCUMENT, BORDER_EVENT | FREIGHT_FORWARDING_ENABLED (false) |
| 20.7 | Cold chain (+temperature events) | IMPLEMENTED | `POST /specialized/COLD_CHAIN/records/:id/temperature-events` | COLD_CHAIN_ENABLED (false) |
| 20.8 | Medical/sensitive | IMPLEMENTED | MEDICAL_HANDLING, EMERGENCY_EXCEPTION | MEDICAL_SENSITIVE_ENABLED (false) |
| 20.9 | Agricultural/food | IMPLEMENTED | AGRICULTURAL_PROFILE, SPOILAGE_EVENT | AGRICULTURAL_FOOD_ENABLED (false) |
| 20.10 | Moving/relocation | IMPLEMENTED | MOVING_INVENTORY, CONDITION, DAMAGE_RECORD | MOVING_RELOCATION_ENABLED (false) |
| 20.11 | Towing/recovery | IMPLEMENTED | TOWING_RECOVERY | TOWING_RECOVERY_ENABLED (false) |
| 20.12 | Milestones / generic commands on records | IMPLEMENTED | `POST .../records/:id/milestones`, `.../commands/:command` | SPECIALIZED_LOGISTICS_ENABLED (false) |
| 20.13 | Disabled modules must not expose routes (20.1) | PARTIAL | Routes are always mounted; disabled capability answers 404/403 at service layer | - |
| 20.14 | Capability schemas / order specialized-data endpoints (50.3) | MISSING | no `/capability-schemas`, no `/orders/{id}/specialized-data` | - |

## Section 21 - Warehouse, fulfilment, yard, dock and terminal (detail: Sec. 51)

| # | Requirement | Status | Evidence | Gate (default) |
|---|---|---|---|---|
| 21.1 | GET/POST /warehouses (+patch, archive) | IMPLEMENTED | f/warehouse/service.go | WAREHOUSES_ENABLED (false) |
| 21.2 | GET/POST /warehouses/{id}/locations (+patch) | IMPLEMENTED | service.go CreateLocation | WAREHOUSES_ENABLED (false) |
| 21.3 | GET /inventory (+item, movements) | IMPLEMENTED | service.go ListItems/GetItem/Movements | WAREHOUSE_INVENTORY_ENABLED (false) |
| 21.4 | POST /inventory/receipts | IMPLEMENTED | service.go Receipt | WAREHOUSE_INVENTORY_ENABLED (false) |
| 21.5 | POST /inventory/movements | IMPLEMENTED | service.go Move | WAREHOUSE_INVENTORY_ENABLED (false) |
| 21.6 | GET/POST /fulfilment-orders, release (+pick, pack, dispatch, cancel, patch) | IMPLEMENTED | operations.go CreateFulfilment/FulfilmentCommand | FULFILMENT_ENABLED (false) |
| 21.7 | GET/POST /dock-appointments (+check-in, complete) | IMPLEMENTED | operations.go CreateDock/DockCommand | DOCK_OPERATIONS_ENABLED (false) |
| 21.8 | POST /yard-visits/check-in, /{id}/check-out | IMPLEMENTED | Path differs: `POST /yard-visits` + `/:id/check-in\|check-out\|release` -> operations.go YardCommand | YARD_OPERATIONS_ENABLED (false) |
| 21.9 | GET/POST /terminal-transfers (+depart, arrive, reconcile) | IMPLEMENTED | operations.go TransferCommand | TERMINAL_TRANSFERS_ENABLED (false) |
| 21.10 | GET/POST /returns (+receive, decide, complete) | IMPLEMENTED | operations.go ReturnCommand | RETURNS_ENABLED (false) |
| 21.11 | Zones/bins, warehouse receipts with lines, claimable leased tasks, pick waves/tasks, packing units (51.3) | MISSING | not separate resources (pick/pack are fulfilment-order commands) | - |

## Section 22 - Finance, billing, payroll, expenses and claims (detail: Sec. 52)

| # | Requirement | Status | Evidence | Gate (default) |
|---|---|---|---|---|
| 22.1 | GET/POST /invoices (+GET/PATCH /{id}) | IMPLEMENTED | f/financeops/http.go -> invoices.go CreateInvoice/UpdateInvoice | INVOICES_ENABLED (false) |
| 22.2 | POST /invoices/{id}/issue (+approve, cancel) | IMPLEMENTED | invoices.go invoiceCommand | INVOICES_ENABLED (false) |
| 22.3 | POST /invoices/{id}/credit-note | IMPLEMENTED | invoices.go CreditInvoice | INVOICES_ENABLED (false) |
| 22.4 | GET /receivables (+/:id); payment receipt projection | IMPLEMENTED | invoices.go ListReceivables/ApplyPayment; internal `POST /finance/payment-events`; overdue sweep | RECEIVABLES_ENABLED (false) |
| 22.5 | Receivables ageing view (52.3 `/receivables/ageing`) | MISSING | list only, no ageing buckets | - |
| 22.6 | GET/POST /expenses, approve (+submit, reject, reverse, patch) | IMPLEMENTED | expenses.go CreateExpense/ExpenseCommand/reverseExpense | EXPENSES_ENABLED (false) |
| 22.7 | Compensation rules (salary/per-job/commission/allowance) | IMPLEMENTED | `/compensation-rules` CRUD + approve/deactivate -> payroll.go | COMPENSATION_ENABLED (false) |
| 22.8 | GET/POST /payroll-periods | IMPLEMENTED | payroll.go CreatePayrollPeriod | PAYROLL_ENABLED (false) |
| 22.9 | POST /payroll-periods/{id}/calculate | IMPLEMENTED | payroll.go CalculatePayroll | PAYROLL_ENABLED (false) |
| 22.10 | POST /payroll-periods/{id}/approve | IMPLEMENTED | payroll.go ApprovePayroll | PAYROLL_ENABLED (false) |
| 22.11 | POST /payroll-periods/{id}/post (allocations to Wallet) | IMPLEMENTED | payroll.go PostPayroll/executePosting (outcome-unknown read-back); Wallet transfer EXTERNAL (app/section22_23_adapters.go) | PAYROLL_ENABLED + FINANCE_POSTING_ENABLED (false) |
| 22.12 | Payroll reversal | IMPLEMENTED | payroll.go ReversePayroll | PAYROLL_ENABLED (false) |
| 22.13 | GET /providers/{id}/statements | IMPLEMENTED | payroll.go ProviderStatements | PAYROLL_ENABLED (false) |
| 22.14 | Subcontractor settlements (+approve, post) | IMPLEMENTED | payroll.go CreateSettlement/SettlementCommand | SUBCONTRACTOR_SETTLEMENTS_ENABLED (false) |
| 22.15 | GET/POST /budgets (+approve, close, patch) | IMPLEMENTED | expenses.go CreateBudget/BudgetCommand | BUDGETS_ENABLED (false) |
| 22.16 | GET/POST /claims (+patch with status history) | IMPLEMENTED | expenses.go CreateClaim/UpdateClaim (claim_history) | CLAIMS_ENABLED (false) |
| 22.17 | Profitability (`/profitability`, 52.3) | MISSING | no route/model | - |
| 22.18 | Platform settlement/refund/reversal operator surface | IMPLEMENTED | f/finance `/finance/settlements\|refunds\|reversals\|operations\|projections\|reconciliation-cases` (Wallet EXTERNAL) | FLEET_FINANCE_INTEGRATION_ENABLED (false), FLEET_FINANCE_EXECUTION_KILL_SWITCH (false) |
| 22.19 | Finance workers (overdue, posting reconcile, budget projection, claim sync) | IMPLEMENTED | financeops/workers.go | SECTION22_WORKERS_ENABLED (false) |

## Section 23 - Communication, support, quality and SLA (detail: Sec. 53)

| # | Requirement | Status | Evidence | Gate (default) |
|---|---|---|---|---|
| 23.1 | GET/POST /communications/threads (+GET/PATCH, close) | IMPLEMENTED | f/engagement/communications.go | COMMUNICATIONS_ENABLED (false) |
| 23.2 | GET/POST /communications/threads/{id}/messages (via notification-service) | IMPLEMENTED | communications.go SendMessage + workers submit/reconcile; delivery EXTERNAL | COMMUNICATIONS_ENABLED (false) |
| 23.3 | GET/PUT /notification-rules | IMPLEMENTED | communications.go SaveNotificationRule | NOTIFICATION_RULES_ENABLED (false) |
| 23.4 | GET /support/cases, POST /support/cases (platform case with org context) | IMPLEMENTED | support_quality.go CreateSupportCase/ApplySupportEvent; internal `POST /support/events`; case ownership EXTERNAL (support-dispute-service) | SUPPORT_PROJECTIONS_ENABLED (false) |
| 23.5 | GET/POST /quality/cases, /{id}/resolve | IMPLEMENTED | support_quality.go | QUALITY_CASES_ENABLED (false) |
| 23.6 | GET /sla/measurements | IMPLEMENTED | support_quality.go + internal `POST /sla/measurements` | SLA_MEASUREMENTS_ENABLED (false) |
| 23.7 | GET /escalations (+acknowledge, resolve) | IMPLEMENTED | support_quality.go EscalationCommand | ESCALATIONS_ENABLED (false) |
| 23.8 | Operational exceptions (`/exceptions`, escalate, resolve; 53.3) | MISSING | no route (dispatch alerts are the closest) | - |
| 23.9 | Engagement workers | IMPLEMENTED | engagement/workers.go | SECTION23_WORKERS_ENABLED (false) |

## Section 24 - Analytics, reporting, AI and workflow automation (detail: Sec. 54)

| # | Requirement | Status | Evidence | Gate (default) |
|---|---|---|---|---|
| 24.1 | Executive dashboard (`/dashboards/executive`, branch filters, freshness) | PARTIAL | `GET /dashboard` -> f/dashboard/usecases/service.go (real SQL aggregates); path differs, no branch filter, not in riskregistry | none (permission dashboard.read) |
| 24.2 | Operations report | MISSING | f/reports is doc.go only | - |
| 24.3 | Fleet report | MISSING | - | - |
| 24.4 | Workforce report | MISSING | - | - |
| 24.5 | Customer reports | MISSING | - | - |
| 24.6 | Custom exports (async, permission, audit, expiry) | IMPLEMENTED | `/exports` (+cancel, download-url, replay) -> f/exports/usecases/service.go, worker.go | EXPORTS_ENABLED, EXPORTS_WRITES_ENABLED, EXPORTS_WORKER_ENABLED (false) |
| 24.7 | Dispatch recommendation (explainable, advisory) | IMPLEMENTED | `GET /dispatch/candidates` (16.3) | DISPATCH_PLANNING_ENABLED (false) |
| 24.8 | Delay prediction | MISSING | - | - |
| 24.9 | Predictive maintenance | MISSING | only rule-based due checks (assets/domain.go) | - |
| 24.10 | Fraud/anomaly detection | MISSING | - | - |
| 24.11 | Document assistance | MISSING | - | - |
| 24.12 | Natural-language assistant | MISSING | - | - |
| 24.13 | GET/POST /automation/rules, GET /automation/runs | MISSING | f/automation registers job type `AUTOMATION_RULE_EXECUTE` whose handler always fails permanent "automation_domain_not_implemented"; no rules/runs | AUTOMATION_RUNNER_WORKER_ENABLED (false) |
| 24.14 | GET /recommendations, accept, dismiss | MISSING | - | - |

## Section 25 - Files, documents, search, imports and exports (detail: Sec. 55)

| # | Requirement | Status | Evidence | Gate (default) |
|---|---|---|---|---|
| 25.1 | Uploads via media-file-service, file_id references | IMPLEMENTED | `POST /documents/upload-requests`, org document upload routes -> f/documents/usecases/service.go; media EXTERNAL | FLEET_DOCUMENTS_WRITES_ENABLED (false) |
| 25.2 | Document lifecycle (verify/reject/revoke/replace/archive/retention/deletion, history) | IMPLEMENTED | f/documents/usecases (lifecycle.go, verification.go, deletion.go) | FLEET_DOCUMENTS_WRITES_ENABLED (false); FLEET_DOCUMENTS_SELF_VERIFICATION_ALLOWED (false) |
| 25.3 | Short-lived authorized download URLs | IMPLEMENTED | `POST /documents/:id/download-requests`, `/exports/:id/download-url` | FLEET_DOCUMENTS_DOWNLOAD_ENABLED (false) |
| 25.4 | Provider/customer file access excludes confidential docs | IMPLEMENTED | internal `/providers/:providerId/documents*`, `/customers/:customerId/orders/:orderId/documents*` (external_access.go) | FLEET_DOCUMENTS_PROVIDER_ALLOWED_CALLERS (default: the 3 driver services), FLEET_DOCUMENTS_CUSTOMER_ALLOWED_CALLERS (empty) |
| 25.5 | Media event ingestion / reconciliation / expiry / deletion workers | IMPLEMENTED | internal `POST /documents/media-events`; app/document_workers.go | FLEET_DOCUMENTS_EVENT_INGESTION_ENABLED, _RECONCILIATION_, _EXPIRY_WORKER_, _DELETION_WORKER_ (false) |
| 25.6 | Search tenant-scoped, permission-filtered | IMPLEMENTED | `GET /search`, `/search/reindex\|remove`, index-job replay -> f/search/usecases | SEARCH_ENABLED (false) |
| 25.7 | Imports: async, dry-run validation, commit, row corrections/retry | IMPLEMENTED | `/imports*` -> f/bulkimports/usecases/generic.go, generic_worker.go | IMPORTS_ENABLED, IMPORTS_WRITES_ENABLED, IMPORTS_*_WORKER_ENABLED (false) |
| 25.8 | Exports async, auto-expiry | IMPLEMENTED | f/exports | EXPORTS_* (false) |
| 25.9 | Organization API clients + webhooks (55.3) | IMPLEMENTED | f/integrations (rotate/revoke, rotate-secret, deliveries, replay); `/fleet-partner/integrations/v1/context` | FLEET_PARTNER_ORGANIZATION_API_CLIENTS_ENABLED (false), FLEET_PARTNER_WEBHOOKS_ENABLED (false), FLEET_PARTNER_WEBHOOK_DELIVERY_WORKER_ENABLED (false) |

## 3. Feature flags that default to `false` (and what they gate)

All are `getEnvBool(..., false)` in `internal/config/config.go:538-850`. Flags that default to `true`: FLEET_PARTNER_WORKER_ENABLED, FLEET_OPERATING_MODE_ENFORCEMENT_ENABLED, FLEET_TYPED_FINANCIAL_VISIBILITY_ENABLED, FLEET_DATA_RETENTION_DRY_RUN, FLEET_PARTNER_METRICS_ENABLED. Organizations/onboarding, staff, roles/permissions, approvals, managed-provider routes, corporate marketplace routes (accept/assign flags are hard-wired on in `app/marketplace_wiring.go:100-111`), eligibility, documents read surface, audit, sessions and the dashboard have **no** enable flag. A disabled module answers 404 on reads and 403 "Capability is disabled." on writes, for example `f/workforce/service.go:52`.

| Flag(s) (default false) | Gates |
|---|---|
| FLEET_HAULING_INTEGRATION_ENABLED, FLEET_TAXI_INTEGRATION_ENABLED, FLEET_DISPATCH_EXECUTION_START_ENABLED | Live calls to hauling, taxi and dispatch for marketplace execution start. Hauling also gates managed-provider provisioning (10.2). |
| FLEET_MARKETPLACE_EVENT_INGESTION_ENABLED (+ FLEET_MARKETPLACE_ALLOWED_CALLERS, default "") | Offer/trip projection ingestion (12.1). An empty caller list leaves all provider accept/decline/read internal routes unmounted (11.8-11.9). |
| FLEET_MARKETPLACE_RECONCILIATION_ENABLED | Projection gap reconciliation and stale sweep (12.4) |
| ASSIGNMENT_EXPIRY_WORKER_ENABLED, REASSIGNMENT_WORKER_ENABLED, FLEET_REASSIGNMENT_ENABLED, FLEET_AUTOMATIC_REASSIGNMENT_ENABLED | Assignment timeout to EXPIRED, reassignment command and automatic reassignment (11.7, 11.11) |
| AUTOMATION_RUNNER_WORKER_ENABLED | Automation job type, which is a permanent-failure stub (24.13) |
| DATA_RETENTION_WORKER_ENABLED, FLEET_PARTNER_RETENTION_EXECUTION_ENABLED | Retention sweeps and executable retention runs |
| FLEET_FINANCE_INTEGRATION_ENABLED, FLEET_FINANCE_EVENT_INGESTION_ENABLED | Finance operator settlements/refunds/reversals, Wallet event ingestion (12.6, 22.18) |
| FLEET_DOCUMENTS_WRITES_ENABLED, _DOWNLOAD_ENABLED, _EVENT_INGESTION_ENABLED, _SELF_VERIFICATION_ALLOWED, _RECONCILIATION_ENABLED, _EXPIRY_WORKER_ENABLED, _DELETION_WORKER_ENABLED | Document uploads/lifecycle, downloads, media events, workers (25.1-25.5) |
| ORGANIZATION_WALLET_READ_ENABLED, PROVIDER_SUBWALLET_ENABLED, PROVIDER_ALLOCATION_ENABLED, PROVIDER_WITHDRAWAL_ENABLED, WALLET_PROJECTION_REFRESH_ENABLED, WALLET_RECONCILIATION_ENABLED | Section 13 wallet facade (13.1-13.9) |
| CRM_ENABLED, CRM_CUSTOMER_WRITES_ENABLED, CRM_CONTACT_WRITES_ENABLED, CRM_LOCATION_WRITES_ENABLED, CRM_OPPORTUNITIES_ENABLED, QUOTATIONS_ENABLED, QUOTATION_ISSUE_ENABLED, QUOTATION_ACCEPTANCE_ENABLED, CONTRACTS_ENABLED, CONTRACT_ACTIVATION_ENABLED, SLA_ENABLED, CUSTOMER_CREDIT_ENABLED | Section 14 CRM (14.1-14.9) |
| CRM_NOTIFICATION_WORKER_ENABLED, QUOTATION_EXPIRY_WORKER_ENABLED, CONTRACT_EXPIRY_WORKER_ENABLED, CONTRACT_RENEWAL_WORKER_ENABLED, SLA_EVALUATION_WORKER_ENABLED, SLA_ESCALATION_WORKER_ENABLED | CRM workers |
| SERVICE_CATALOG_ENABLED, SERVICE_CATALOG_WRITES_ENABLED, ORDERS_ENABLED, ORDERS_WRITES_ENABLED, SHIPMENTS_ENABLED, SHIPMENTS_WRITES_ENABLED, PACKAGES_ENABLED, PACKAGES_WRITES_ENABLED, PACKAGE_SCANNING_ENABLED, MANIFESTS_ENABLED, MANIFESTS_WRITES_ENABLED, RECURRING_ORDERS_ENABLED, RECURRING_ORDERS_WRITES_ENABLED, RECURRING_ORDERS_WORKER_ENABLED | Section 15 (15.1-15.9), custody (19.12) |
| BULK_IMPORTS_ENABLED, BULK_IMPORTS_WRITES_ENABLED, BULK_IMPORTS_WORKER_ENABLED, IMPORTS_ENABLED, IMPORTS_WRITES_ENABLED, IMPORTS_VALIDATION_WORKER_ENABLED, IMPORTS_COMMIT_WORKER_ENABLED, IMPORTS_RETRY_WORKER_ENABLED | Order imports (15.8) and generic imports (25.7) |
| EXPORTS_ENABLED, EXPORTS_WRITES_ENABLED, EXPORTS_WORKER_ENABLED | Exports (24.6, 25.8) |
| PRIVATE_ORDERS_ENABLED, PRIVATE_ASSIGNMENTS_WRITES_ENABLED, PRIVATE_ASSIGNMENTS_WORKER_ENABLED, PRIVATE_DISPATCH_EXECUTION_ENABLED, PRIVATE_HAULING_EXECUTION_ENABLED, PRIVATE_TAXI_EXECUTION_ENABLED, PRIVATE_EXECUTION_WORKER_ENABLED | Private assignments and execution (15.10) |
| FLEET_MULTI_SERVICE_PROVIDER_LINKS_ENABLED | managed-members / managed-assets layer (10.6, 10.15) |
| DISPATCH_PLANNING_ENABLED, ROUTE_PLANNING_ENABLED, ROUTE_OPTIMIZATION_ENABLED, ROUTE_PUBLICATION_ENABLED, CONTROL_TOWER_ENABLED, TRACKING_PROJECTIONS_ENABLED, GEOFENCES_ENABLED, OPERATIONAL_ALERTS_ENABLED, FLEET_SECTION16_WORKERS_ENABLED | Section 16 (16.1-16.13), trip tracking (12.3), candidate scoring (24.7) |
| WORKFORCE_ENABLED, SHIFTS_ENABLED, LEAVE_ENABLED, QUALIFICATIONS_ENABLED, TRAINING_ENABLED, PERFORMANCE_SNAPSHOTS_ENABLED, SECTION17_WORKERS_ENABLED | Section 17 (17.1-17.16). Performance needs both of the last two. |
| ASSETS_ENABLED, ASSET_ASSIGNMENTS_ENABLED, INSPECTIONS_ENABLED, MAINTENANCE_ENABLED, BREAKDOWNS_ENABLED, PARTS_INVENTORY_ENABLED, FUEL_RECORDS_ENABLED, SECTION18_WORKERS_ENABLED | Section 18 (18.1-18.11), safety-decline consumer (11.13) |
| COMPLIANCE_ENABLED, SAFETY_REPORTS_ENABLED, INCIDENTS_ENABLED, PROOF_POLICIES_ENABLED, PROOF_RECORDS_ENABLED, RESTRICTION_PROPAGATION_ENABLED, SECTION19_WORKERS_ENABLED | Section 19 (19.1-19.11) |
| SPECIALIZED_LOGISTICS_ENABLED, COURIER_LAST_MILE_ENABLED, TAXI_PASSENGER_ENABLED, TRUCKING_HAULAGE_ENABLED, HEAVY_SPECIALIZED_ENABLED, FREIGHT_FORWARDING_ENABLED, COLD_CHAIN_ENABLED, MEDICAL_SENSITIVE_ENABLED, AGRICULTURAL_FOOD_ENABLED, MOVING_RELOCATION_ENABLED, TOWING_RECOVERY_ENABLED, SECTION20_WORKERS_ENABLED | Section 20 (20.1-20.12) |
| WAREHOUSES_ENABLED, WAREHOUSE_INVENTORY_ENABLED, FULFILMENT_ENABLED, DOCK_OPERATIONS_ENABLED, YARD_OPERATIONS_ENABLED, TERMINAL_TRANSFERS_ENABLED, RETURNS_ENABLED, SECTION21_WORKERS_ENABLED | Section 21 (21.1-21.10) |
| INVOICES_ENABLED, RECEIVABLES_ENABLED, EXPENSES_ENABLED, BUDGETS_ENABLED, COMPENSATION_ENABLED, PAYROLL_ENABLED, SUBCONTRACTOR_SETTLEMENTS_ENABLED, CLAIMS_ENABLED, FINANCE_POSTING_ENABLED, SECTION22_WORKERS_ENABLED | Section 22 (22.1-22.19). FINANCE_POSTING_ENABLED is needed for payroll/settlement/expense posting to Wallet. |
| COMMUNICATIONS_ENABLED, NOTIFICATION_RULES_ENABLED, SUPPORT_PROJECTIONS_ENABLED, QUALITY_CASES_ENABLED, SLA_MEASUREMENTS_ENABLED, ESCALATIONS_ENABLED, SECTION23_WORKERS_ENABLED | Section 23 (23.1-23.9) |
| FLEET_COMPENSATION_RESOLVER_ENABLED, FLEET_COMPLETED_WORK_PROJECTION_ENABLED, FLEET_SETTLEMENT_INSTRUCTION_ENABLED | Managed-work compensation and settlement (12.7, 13.11) |
| SEARCH_ENABLED | Search (25.6) |
| FLEET_SYNCHRONIZATION_ENABLED, FLEET_SYNCHRONIZATION_WORKER_ENABLED | Section 26 synchronization projections, dead letters, reconciliation |
| FLEET_PARTNER_ORGANIZATION_API_CLIENTS_ENABLED, FLEET_PARTNER_WEBHOOKS_ENABLED, FLEET_PARTNER_WEBHOOK_DELIVERY_WORKER_ENABLED, FLEET_PARTNER_WEBHOOK_ALLOW_HTTP | Organization API clients and webhooks (25.9) |
| FLEET_PARTNER_SERVICE_CREDENTIAL_SCOPING_ENABLED, FLEET_PARTNER_REDIS_REQUIRED, MIGRATION | Platform (scoped HMAC credentials, Redis requirement, migrate on start) |
| Kill switches (all default false = not engaged): FLEET_INTEGRATIONS_KILL_SWITCH, FLEET_FINANCE_EXECUTION_KILL_SWITCH, FLEET_DOCUMENTS_KILL_SWITCH, FLEET_COMPANY_WALLET_KILL_SWITCH, FLEET_CRM_KILL_SWITCH, FLEET_ORDERS_/SHIPMENTS_/PACKAGES_/MANIFESTS_/RECURRING_ORDERS_/BULK_IMPORTS_/EXPORTS_/PRIVATE_OPERATIONS_/SYNCHRONIZATION_KILL_SWITCH | Emergency disable |

Besides the flags, many integrations stay fail-closed until a client URL and secret are configured: payment-wallet, media-file, verification-compliance, driver-hauling/taxi/dispatch, notification, support-dispute, and the route optimizer.

## 4. riskregistry vs. the routes actually mounted

`registry.go` claims that `TestRouteRiskRegistry_CoversEveryMountedRoute` guarantees completeness. That test enumerates a hand-picked set of feature manifests (`internal/section28contract/route_coverage_test.go:84-415`), not the real router. It skips the corporate marketplace, managedmembers, dashboard and fleetpartner tenant routes and several internal handlers.

**Mounted but not in riskregistry (36):**

- `GET /dashboard`
- `GET /fleet-partner/operating-mode`
- `GET /fleet-partner/public/v1/quotations/:quotationId/view`
- `GET /internal/providers/:providerId/corporate-context`
- `GET /internal/providers/:providerId/services/:providerService/membership-summary`
- `GET /managed-assets`
- `GET /managed-members`
- `GET /managed-members/:memberId`
- `GET /marketplace/assignments`
- `GET /marketplace/assignments/:assignmentId`
- `GET /marketplace/assignments/:assignmentId/history`
- `GET /marketplace/metrics`
- `GET /marketplace/offers`
- `GET /marketplace/offers/:offerId`
- `GET /marketplace/requests/:projectionId/candidates`
- `POST /fleet-partner/compensation-plans`
- `POST /fleet-partner/memberships/:membershipId/compensation-assignments`
- `POST /fleet-partner/public/v1/quotations/:quotationId/accept`
- `POST /fleet-partner/public/v1/quotations/:quotationId/reject`
- `POST /internal/completed-work`
- `POST /internal/finance/payment-events`
- `POST /internal/sla/measurements`
- `POST /internal/support/events`
- `POST /managed-assets`
- `POST /managed-assets/:linkId/status`
- `POST /managed-members`
- `POST /managed-members/:memberId/consent`
- `POST /managed-members/:memberId/links`
- `POST /managed-members/:memberId/reinstate`
- `POST /managed-members/:memberId/suspend`
- `POST /marketplace/assignments/:assignmentId/cancel`
- `POST /marketplace/assignments/:assignmentId/reassign`
- `POST /marketplace/offers/:offerId/accept`
- `POST /marketplace/offers/:offerId/decline`
- `POST /marketplace/requests/:projectionId/assign`
- `PUT /fleet-partner/operating-mode`

(The three `/fleet-partner/public/v1/quotations/...` routes are registered under the wrong prefix as `GET /fleet-partner/v1/quotations/:quotationId/view`, and accept/reject are not registered at all.)

**In riskregistry but not mounted by default (21):**

- `GET /internal/admin/authorization-contexts/bindings/:kind/:id`
- `GET /internal/admin/resources/:kind`
- `GET /internal/admin/resources/:kind/:id`
- `GET /internal/operations/backlogs`
- `GET /internal/worker-operations/jobs`
- `GET /internal/worker-operations/jobs/:jobId`
- `GET /internal/worker-operations/jobs/:jobId/attempts`
- `GET /internal/worker-operations/scheduled-tasks`
- `GET /internal/worker-operations/scheduled-tasks/:taskId`
- `GET /internal/worker-operations/scheduled-tasks/:taskId/runs`
- `GET /provider/sub-wallet`
- `GET /provider/sub-wallet/statement`
- `GET /quotations/:quotationId/view`
- `POST /internal/admin/authorization-contexts`
- `POST /internal/operations/outbox/replay`
- `POST /internal/worker-operations/jobs/:jobId/cancel`
- `POST /internal/worker-operations/jobs/:jobId/retry`
- `POST /internal/worker-operations/scheduled-tasks/:taskId/disable`
- `POST /internal/worker-operations/scheduled-tasks/:taskId/enable`
- `POST /internal/worker-operations/scheduled-tasks/:taskId/run-now`
- `POST /provider/sub-wallet/withdrawals`

The `/provider/sub-wallet*` routes are never mounted in any configuration. The internal admin, worker-operations and outbox routes mount only when their runtime admin configuration or `WORKER_OPERATIONS_ALLOWED_CALLERS` is set. Full route lists: `(audit working file)` (669 mounted, from `gin.Routes()`) and `(audit working file)` (650).

## 5. Appendix A catalogue vs. routes

The Appendix A table (spec lines 1893-2316) expands to 223 method+path pairs, counting "GET/POST" as two. Matching against mounted routes plus riskregistry, with path-parameter names normalized, gives **170 exact matches and 53 without a match**. Of the 53, 27 have a functionally equivalent route under a different path or method (rename, `/marketplace/*` prefix, internal relay, `PUT` instead of `PATCH`), and **26 have no equivalent at all** (marked "none").

| Catalogue group | Endpoint | Purpose | Closest implemented route |
|---|---|---|---|
| Authentication and sessions | POST /auth/phone/start | Start authentication challenge | `POST /auth/start`, `/auth/sign-in/start`, `/auth/sign-up/start` (renamed; email+phone OTP) |
| Authentication and sessions | POST /auth/phone/verify | Verify challenge | `POST /auth/verify` (renamed) |
| Authentication and sessions | POST /auth/login | Password login | none - password login not implemented (passwordless by design) |
| Organizations and structure | PATCH /organizations/{id} | Update profile | `PUT /organizations/:organizationId` (+ `PUT .../profile`) - method differs |
| Organizations and structure | GET /branches | Manage branches/sites | none (repository only, no route) |
| Organizations and structure | POST /branches | Manage branches/sites | none - no code path creates a branch |
| Organizations and structure | GET /departments | Manage departments | none (repository only) |
| Organizations and structure | POST /departments | Manage departments | none |
| Organizations and structure | GET /organization-settings | Manage settings | none (settings only via `/fleet-partner/operating-mode`) |
| Organizations and structure | PUT /organization-settings | Manage settings | none |
| Staff and access | POST /staff | List/invite staff | `POST /staff/invitations` (equivalent) |
| Staff and access | GET /staff/{id} | Read/update staff | none (GetStaff usecase unrouted) |
| Staff and access | PATCH /staff/{id} | Read/update staff | none (UpdateStaff usecase unrouted) |
| Staff and access | POST /roles | Manage roles | none (repo CreateRole unrouted) |
| Staff and access | GET /role-bindings | Manage grants | none |
| Staff and access | POST /role-bindings | Manage grants | none (ReplaceRoleBindings usecase unrouted) |
| Staff and access | GET /approvals | List approvals | `GET /approvals/requests` (renamed) |
| Staff and access | POST /approvals/{id}/approve | Approve | `POST /approvals/requests/:approvalId/approve` (renamed) |
| Staff and access | POST /approvals/{id}/reject | Reject | `POST /approvals/requests/:approvalId/reject` (renamed) |
| Managed providers | PATCH /providers/{id} | Read/update membership | none |
| Managed providers | POST /providers/{id}/reactivate | Reactivate | none (member-layer `/managed-members/:memberId/reinstate` only) |
| Managed providers | POST /providers/{id}/remove | Remove from company | none |
| Managed providers | PUT /providers/{id}/capabilities | Set capabilities | none |
| Managed providers | PUT /providers/{id}/policies | Set work policy | none |
| Managed providers | POST /providers/{id}/resend-login | Resend login | none |
| Managed providers | GET /providers/{id}/availability | Read projection | none |
| Marketplace and assignments | GET /requests | List organization opportunities/projections | `GET /marketplace/offers` (renamed; not in riskregistry) |
| Marketplace and assignments | GET /requests/{id} | Request detail | `GET /marketplace/offers/:offerId` |
| Marketplace and assignments | POST /requests/{id}/accept-for-organization | Company accepts | `POST /marketplace/offers/:offerId/accept` |
| Marketplace and assignments | POST /requests/{id}/decline-for-organization | Company declines opportunity | `POST /marketplace/offers/:offerId/decline` |
| Marketplace and assignments | GET /requests/{id}/candidates | Eligible candidates | `GET /marketplace/requests/:projectionId/candidates` |
| Marketplace and assignments | POST /requests/{id}/assign | Create assignment | `POST /marketplace/requests/:projectionId/assign` |
| Marketplace and assignments | GET /assignments | List assignments | `GET /marketplace/assignments` |
| Marketplace and assignments | GET /assignments/{id} | Assignment detail | `GET /marketplace/assignments/:assignmentId` |
| Marketplace and assignments | POST /assignments/{id}/accept | Provider accepts | internal only: `/internal/fleet-partner/v1/assignments/:assignmentId/provider-accept` (not mounted unless FLEET_MARKETPLACE_ALLOWED_CALLERS set) |
| Marketplace and assignments | POST /assignments/{id}/decline | Provider declines | internal only: `.../assignments/:assignmentId/provider-decline` (same gating) |
| Marketplace and assignments | POST /assignments/{id}/reassign | Reassign | `POST /marketplace/assignments/:assignmentId/reassign` |
| Marketplace and assignments | POST /assignments/{id}/cancel | Cancel | `POST /marketplace/assignments/:assignmentId/cancel` |
| Marketplace and assignments | GET /marketplace-offers | Provider offers | internal only: `GET /internal/fleet-partner/v1/providers/:providerId/offers` |
| Marketplace and assignments | POST /marketplace-offers/{id}/accept | Provider accepts offer | internal only: `.../marketplace-offers/:offerId/provider-accept` |
| Marketplace and assignments | POST /marketplace-offers/{id}/decline | Provider declines offer | internal only: `.../marketplace-offers/:offerId/provider-decline` |
| Trips and control tower | GET /trips | List trip projections | partial: `GET /tracking` |
| Trips and control tower | GET /trips/{id} | Trip detail projection | partial: `GET /tracking/:canonicalTripReference` |
| Orders and shipments | PATCH /orders/{id} | Order detail | none |
| Compliance, safety and proof | POST /custody-events | Custody event | partial: `POST /packages/:packageId/custody` (package-scoped) |
| Warehouse and terminal | POST /yard-visits/check-in | Gate check-in | `POST /yard-visits` + `POST /yard-visits/:yardVisitId/check-in` |
| Communication, support and reporting | GET /dashboards/executive | Dashboard | partial: `GET /dashboard` |
| Communication, support and reporting | GET /reports/operations | Operations report | none |
| Communication, support and reporting | GET /reports/fleet | Fleet report | none |
| Communication, support and reporting | GET /reports/workforce | Workforce report | none |
| Communication, support and reporting | GET /automation/rules | Automation | none |
| Communication, support and reporting | POST /automation/rules | Automation | none |
| Communication, support and reporting | GET /recommendations | Recommendations | none (closest: `GET /dispatch/candidates`) |

Per-row data: `(audit working file)`.

**Section 59, the binding HTTP catalogue (supplementary).** The same normalized match against Section 59's tables gives 90 of 250 method+path pairs mounted. Most of the shortfall is naming drift: `/users` vs `/staff`, `/leave-requests` vs `/leave`, `/payroll/periods` vs `/payroll-periods`, `/marketplace/offers/{id}/reject` vs `/decline`, `/restrictions` vs `/compliance/restrictions`, and so on. The rest are the genuinely missing resources listed above. Per-row data: `(audit working file)`.
