# Fleet backend vs. Spec v2.0 - Section 59 API catalogue and detail Sections 37-55

Read-only audit of `services/fleet-partner-service` at HEAD `9e3f4ba` (2026-11-13). No repository file was changed. Paths omit the `/fleet-partner/v1` prefix; "internal" = `/internal/fleet-partner/v1` (HMAC). `f/` = `services/fleet-partner-service/internal/features/`.

Inputs reused: `F-sections-8-25.md` (Sections 8-25), `(audit working file)` (669 routes from `gin.Routes()` at 2f6932f) plus the four staff routes added in d83e81e (`GET /staff/:staffId`, `GET /staff/invitations`, `POST /staff/invitations/:invitationId/resend`, `DELETE /staff/invitations/:invitationId`, `f/organizations/http/handlers.go:89-96`). A `git diff 2f6932f..HEAD` of the service confirms no other route was added. Commit b547b37 adds an automatic Head Office branch (migration `000087_default_head_office_branch`), but still no branch API.

Classification for Section 59: **EXACT** = same method and path mounted; **EQUIVALENT** = the same capability exists under another path/method (route named); **PARTIAL** = a related route exists but covers only part of the capability (named, with the gap); **MISSING** = no route and no reachable implementation. Every `N` row of `s59.tsv` was re-checked against the mounted routes and the handler/usecase code. Status values for Sections 37-55: IMPLEMENTED / PARTIAL / MISSING / EXTERNAL.

## Summary

| Area | EXACT | EQUIVALENT | PARTIAL | MISSING | Total |
|---|---|---|---|---|---|
| 59.2 Organization, structure and settings | 1 | 3 | 2 | 17 | 23 |
| 59.3 Users, roles and approvals | 2 | 8 | 0 | 9 | 19 |
| 59.4 Managed providers and workforce | 8 | 5 | 4 | 8 | 25 |
| 59.5 Marketplace, assignment and live trips | 3 | 10 | 5 | 0 | 18 |
| 59.6 Commercial, orders and routing | 28 | 8 | 3 | 5 | 44 |
| 59.7 Assets, maintenance, compliance and safety | 18 | 16 | 3 | 4 | 41 |
| 59.8 Warehouse, yard and dock | 9 | 7 | 6 | 6 | 28 |
| 59.9 Finance, wallets and reporting | 18 | 5 | 2 | 6 | 31 |
| 59.10 Communication, integrations and governance | 3 | 10 | 5 | 3 | 21 |
| **Section 59 total** | **90** | **72** | **30** | **58** | **250** |

The capability is present (EXACT + EQUIVALENT) for 162 of 250 endpoints (65%), partly present for 30 (12%) and absent for 58 (23%). The 160 `N` rows in `s59.tsv` break down as 72 EQUIVALENT, 30 PARTIAL and 58 MISSING, so most of the gap there was naming drift.

| Detail Sections 37-55 (requirements not covered by the Sections 8-25 audit) | IMPLEMENTED | PARTIAL | MISSING | Total |
|---|---|---|---|---|
| 37-42 Organization, access, providers, marketplace, trips, wallet | 9 | 10 | 9 | 28 |
| 43-46 CRM, orders, dispatch, workforce | 7 | 5 | 7 | 19 |
| 47-49 Assets, maintenance, compliance | 6 | 13 | 3 | 22 |
| 50-55 Specialized, warehouse, finance, comms, integrations | 10 | 4 | 4 | 18 |
| **Total** | **32** | **32** | **23** | **87** |

### Key findings

1. **Organization structure is the largest hole (59.2: 17 of 23 MISSING).** Branches, locations, departments, teams, cost centres and organization settings have no API. Branch and department repositories exist (`f/organizations/repositories/branches.go:28-111`, `departments.go:29-86`) but are unrouted. Teams, cost centres and operating locations have no table. Since b547b37 every company gets an active HQ branch, which unblocks activation, but a company still cannot add a second branch.
2. **Access administration beyond staff (59.3).** Staff list/invite/resend/cancel/get/suspend/reactivate now exist. Still missing: custom roles (create/read/update), role assignment (`PUT /users/{id}/roles`), approval policies and approval delegations. The delegations table exists (`migrations/000006_approvals.up.sql:116`) with no code. Approval policy money thresholds (`threshold_amount_minor`, 000006:17) are never read, so per-user financial limits (38) are not enforced.
3. **Provider membership lifecycle (59.4).** Only create/list/get/suspend exist. PATCH, activate (automatic only), reactivate, end, branch reassignment, service-role removal, availability and per-provider declines are missing. "End" is the most consequential gap: there is no offboarding of active work, future shifts, asset assignments or sub-wallet access (`f/providers/usecases/lifecycle.go:244-305` only revokes eligibility). Dispatch and Taxi still cannot create managed providers (fail-closed `UnconfiguredProvider`, `internal/platformclients/contracts.go:375-384`; only Hauling is wired, `internal/app/wiring.go:271`).
4. **Marketplace (59.5) has no MISSING endpoints.** Everything exists under `/marketplace/*` or as provider relays. Gaps are partial: `/trips` reads tracking projections, while the richer `fleet_partner_trip_projections` table (source service, request id, membership, settlement organization; `migrations/000010:8-36`) has no read route. Rescue exists only as asset breakdowns. Control-tower exceptions are operational alerts.
5. **Order integrity rules from Section 44 are missing.** There is no atomic order + stops + shipment + package creation (`f/orders/http/handlers.go:341-353`), no pickup-before-delivery validation (stop types defined at `f/orders/models/models.go:192-204`, never checked), no order PATCH, and no order approval. `MarkAwaitingApproval` has no caller (`f/orders/repositories/repository.go:262`). A manifest closes without load-scan reconciliation of missing, extra or damaged items (`f/manifests/usecases/service.go:240-262`).
6. **Corrections to the earlier audit.** Warehouse zones and bins exist as typed storage locations (`location_type` ZONE/BIN/..., `migrations/000046:18`). Inventory adjustments exist as `ADJUSTMENT` movements (`f/warehouse/service.go:502`). Section 21.11 is therefore PARTIAL, not MISSING: receipts with lines, claimable put-away and pick tasks, pick waves and packing units are what is absent.
7. **New detail-level defects.**
   - Fuel records have no amount or currency (`f/assets/domain.go` FuelRecord), and there is no anomaly flagging.
   - Asset operational status has 6 of the 10 required values, with no `reserved` (`migrations/000040`).
   - Asset retire/archive checks only active asset assignments, not trips, routes, work orders or custody (`f/assets/service.go:186-206`).
   - The allocation type list lacks TIP (`migrations/000022:112-115`).
   - Sensitive exports require no step-up (`/exports` risk class is used only for rate limiting).
   - Providers have no way to file safety reports (corporate-only routes).
   - Capability-scoped organization suspension does not exist (capability statuses lack SUSPENDED, `migrations/000045:7-9`).
   - There is no customer `platform_customer_id`.
   - `WorkOrder.started_at` is never set (`f/assets/operations.go:198`).
8. **Frontend.** Of the backend modules with working APIs, these have no real page (mock or TODO stubs, or nothing): wallet/sub-wallets, compliance/safety/proof, documents/imports/exports/search, communications/support/quality/SLA/escalations, private orders/shipments/packages/manifests/service catalogue (16 TODO stubs), approvals, audit, sessions, security settings, legal holds, retention, integrations, jobs/sync. The truck and taxi provider apps have no corporate offer/assignment UI, even though their backends now relay those calls.

## 1. Section 59 - Complete API catalogue

### 1.1 Truly MISSING endpoints (58)

| # | Sub | Method | Spec path | Note |
|---|---|---|---|---|
| 1 | 59.2 | GET | `/branches` | BranchRepository Create/List/Archive (repositories/branches.go:28-111) unrouted; HQ branch auto-created by migration 000087/onboarding only |
| 2 | 59.2 | POST | `/branches` | BranchRepository Create/List/Archive (repositories/branches.go:28-111) unrouted; HQ branch auto-created by migration 000087/onboarding only |
| 3 | 59.2 | GET | `/branches/{branchId}` | BranchRepository.GetByID unrouted |
| 4 | 59.2 | PATCH | `/branches/{branchId}` | no update method |
| 5 | 59.2 | POST | `/branches/{branchId}/activate` |  |
| 6 | 59.2 | POST | `/branches/{branchId}/deactivate` | BranchRepository.Archive unrouted |
| 7 | 59.2 | GET | `/locations` | no operating_locations table; only warehouse locations and CRM customer locations |
| 8 | 59.2 | POST | `/locations` | no operating_locations table; only warehouse locations and CRM customer locations |
| 9 | 59.2 | GET | `/departments` | DepartmentRepository Create/GetByID only, unrouted |
| 10 | 59.2 | POST | `/departments` | DepartmentRepository Create/GetByID only, unrouted |
| 11 | 59.2 | PATCH | `/departments` | DepartmentRepository Create/GetByID only, unrouted |
| 12 | 59.2 | GET | `/teams` | no table |
| 13 | 59.2 | POST | `/teams` | no table |
| 14 | 59.2 | PATCH | `/teams` | no table |
| 15 | 59.2 | GET | `/cost-centres` | no table (departments.cost_centre_id column only) |
| 16 | 59.2 | POST | `/cost-centres` | no table (departments.cost_centre_id column only) |
| 17 | 59.2 | PATCH | `/cost-centres` | no table (departments.cost_centre_id column only) |
| 18 | 59.3 | POST | `/roles` | RoleRepository.CreateRole (staff/repositories/roles.go:130) unrouted |
| 19 | 59.3 | GET | `/roles/{id}` | RoleRepository.GetRole (staff/repositories/roles.go:85) exists, no route (GET /roles list only) |
| 20 | 59.3 | PATCH | `/roles/{id}` | RoleRepository.UpdateRole unrouted |
| 21 | 59.3 | PUT | `/users/{id}/roles` | ReplaceRoleBindings usecase (staff/usecases/membership_flows.go:161) unrouted |
| 22 | 59.3 | GET | `/approval-policies` | policies auto-created from templates (approvals/repositories/approvals.go) |
| 23 | 59.3 | POST | `/approval-policies` | policies auto-created from templates (approvals/repositories/approvals.go) |
| 24 | 59.3 | GET | `/approval-policies/{id}` |  |
| 25 | 59.3 | PATCH | `/approval-policies/{id}` |  |
| 26 | 59.3 | POST | `/approval-delegations` | table fleet_partner_approval_delegations (migration 000006:116) has no code |
| 27 | 59.4 | PATCH | `/providers/{id}` |  |
| 28 | 59.4 | POST | `/providers/{id}/activate` | activation is automatic: providers/usecases/lifecycle.go:189 TryActivate after association + sub-wallet |
| 29 | 59.4 | POST | `/providers/{id}/reactivate` | ChangeStatus (lifecycle.go:244) supports it, unrouted; member-layer POST /managed-members/:memberId/reinstate is a different aggregate |
| 30 | 59.4 | POST | `/providers/{id}/end` | ChangeStatus REMOVED unrouted; no offboarding of shifts/assets/sub-wallet |
| 31 | 59.4 | DELETE | `/providers/{id}/service-roles` | no link removal route |
| 32 | 59.4 | POST | `/providers/{id}/branches` | membership carries one branch_id; no reassignment |
| 33 | 59.4 | GET | `/providers/{id}/availability` | no projection route; ProviderAuthority lookup (platformclients/provider_authority.go) used only inside eligibility |
| 34 | 59.4 | POST | `/training-enrolments` | no enrolment aggregate |
| 35 | 59.6 | GET | `/leads` | opportunity stage LEAD exists (crm/models StageLead) but an opportunity requires an existing customer (crm/usecases/opportunities.go:74) |
| 36 | 59.6 | POST | `/leads` | opportunity stage LEAD exists (crm/models StageLead) but an opportunity requires an existing customer (crm/usecases/opportunities.go:74) |
| 37 | 59.6 | POST | `/leads/{id}/convert` |  |
| 38 | 59.6 | PATCH | `/orders/{id}` |  |
| 39 | 59.6 | POST | `/orders/{id}/approve` | AWAITING_APPROVAL path exists only in repository (orders/repositories/repository.go:262 MarkAwaitingApproval, no caller) |
| 40 | 59.7 | POST | `/assets/{id}/reserve` | no RESERVED operational status (migration 000040 CHECK: AVAILABLE,ASSIGNED,IN_USE,BLOCKED,UNDER_MAINTENANCE,OUT_OF_SERVICE) |
| 41 | 59.7 | GET | `/defects` | defects stored only as inspection defect_summary JSON |
| 42 | 59.7 | POST | `/defects` | defects stored only as inspection defect_summary JSON |
| 43 | 59.7 | POST | `/defects/{id}/resolve` |  |
| 44 | 59.8 | GET | `/warehouse-receipts` | receipts are one-shot stock postings; no receipt list |
| 45 | 59.8 | POST | `/warehouse-receipts/{id}/arrive` |  |
| 46 | 59.8 | POST | `/warehouse-receipts/{id}/receive` | no expected/received/damaged/quarantined reconciliation |
| 47 | 59.8 | POST | `/putaway-tasks/{id}/claim` | no claimable leased tasks |
| 48 | 59.8 | GET | `/pick-waves` |  |
| 49 | 59.8 | POST | `/pick-waves` |  |
| 50 | 59.9 | POST | `/invoices/{id}/submit` | PENDING_APPROVAL accepted but nothing sets it; /approve goes straight from DRAFT (financeops/invoices.go:204) |
| 51 | 59.9 | GET | `/dashboard/finance` |  |
| 52 | 59.9 | GET | `/reports` | f/reports is doc.go only |
| 53 | 59.9 | POST | `/reports` | f/reports is doc.go only |
| 54 | 59.9 | POST | `/reports/{id}/run` |  |
| 55 | 59.9 | GET | `/report-runs/{id}` |  |
| 56 | 59.10 | GET | `/automation-rules` | AUTOMATION_RULE_EXECUTE job always fails permanent |
| 57 | 59.10 | POST | `/automation-rules` | AUTOMATION_RULE_EXECUTE job always fails permanent |
| 58 | 59.10 | POST | `/automation-rules/{id}/activate` |  |

### 1.2 PARTIAL endpoints (30) - related route exists, capability incomplete

| Sub | Method | Spec path | Closest route | Gap |
|---|---|---|---|---|
| 59.2 | GET | `/organization-settings` | GET /fleet-partner/operating-mode | only operating mode + financial visibility; assignment timeout, approval thresholds, provider visibility, retention, flags (models.Settings) not readable |
| 59.2 | PATCH | `/organization-settings` | PUT /fleet-partner/operating-mode | SettingsRepository.Update exists (repositories/settings.go:124) but no route |
| 59.4 | POST | `/providers/{id}/service-roles` | POST /managed-members/:memberId/links | member-layer service link (FLEET_MULTI_SERVICE_PROVIDER_LINKS_ENABLED), not the membership aggregate |
| 59.4 | GET | `/providers/{id}/declines` | GET /marketplace/assignments?status=DECLINED | no membership filter (marketplace/http/handlers.go:82-90) |
| 59.4 | POST | `/training-enrolments/{id}/complete` | POST /training/courses/:courseId/complete | completion recorded directly against course + membership (workforce/service.go:717) |
| 59.4 | GET | `/provider-performance` | GET /performance/providers/:membershipId | per-provider only; no date/branch filters; snapshots from private assignments only |
| 59.5 | GET | `/trips` | GET /tracking | reads tracking projections; fleet_partner_trip_projections (migration 000010) has no list route |
| 59.5 | GET | `/trips/{id}` | GET /tracking/:canonicalTripReference | no trip_projection_history read |
| 59.5 | GET | `/control-tower/exceptions` | GET /alerts | operational alerts only |
| 59.5 | POST | `/rescue-cases` | POST /breakdowns | asset breakdown carries rescue_status/replacement_asset_id (assets/domain.go Breakdown); no rescue-case aggregate for provider unavailability/delay/cargo transfer |
| 59.5 | POST | `/rescue-cases/{id}/resolve` | PATCH /breakdowns/:breakdownId |  |
| 59.6 | POST | `/contracts/{id}/amendments` | POST /contracts/:contractId/renew | renewal creates a new version; mid-term amendment of an active contract not supported (UpdateContract draft-only, crm/usecases/contracts.go:224) |
| 59.6 | POST | `/orders/{id}/request-execution` | POST /private-assignments (+ /private-executions/:executionId) | assignment-led private execution; dispatch/hauling only, taxi not wired |
| 59.6 | POST | `/shipments/{id}/exceptions` | POST /packages/:packageId/transition | package-level exceptional states with reasonCode (packages/usecases/service.go:196); no shipment exception record |
| 59.7 | POST | `/inspections/{id}/items` | POST /inspections/:inspectionId/complete | item answers submitted only at completion |
| 59.7 | POST | `/incidents/{id}/triage` | PATCH /incidents/:incidentId | severity/status edit; no triage command or immediateActions record |
| 59.7 | POST | `/incidents/{id}/investigations` | PATCH /incidents/:incidentId (status INVESTIGATING) | no investigation record with lead/scope |
| 59.8 | POST | `/inventory-items` | POST /inventory/receipts | items are created by receipts; no item-master/track-rules |
| 59.8 | POST | `/warehouse-receipts` | POST /inventory/receipts | no expected-receipt document/lines |
| 59.8 | POST | `/putaway-tasks/{id}/complete` | POST /inventory/movements (movementType=PUT_AWAY) | full-item move only (warehouse/service.go:510) |
| 59.8 | POST | `/pick-tasks/{id}/complete` | POST /fulfilment-orders/:fulfilmentOrderId/pick | order-level pick; row-locked reservation (warehouse/operations.go:196-202) |
| 59.8 | POST | `/packing-units` | POST /fulfilment-orders/:fulfilmentOrderId/pack | no packing-unit resource |
| 59.8 | POST | `/yard-visits/{id}/gate-events` | POST /yard-visits/:yardVisitId/check-in | /check-out | /release | fixed transitions; no generic gate/parking/loading events |
| 59.9 | POST | `/allocations/{id}/cancel` | POST /providers/:membershipId/sub-wallet/allocations/:allocationId/reject | approver rejection; no requester cancel |
| 59.9 | GET | `/dashboard/operations` | GET /dashboard | no date/branch/service filters; not in riskregistry |
| 59.10 | POST | `/support-cases/{id}/messages` | POST /communications/threads (subjectType=SUPPORT_CASE) + /messages | internal org thread; SupportPort has no message call to support-dispute-service (engagement/domain.go:72) |
| 59.10 | GET | `/privacy-settings` | GET /retention-policies, GET /security-settings | no privacy-settings aggregate |
| 59.10 | PATCH | `/privacy-settings` | PUT /retention-policies/:class, PUT /security-settings |  |
| 59.10 | GET | `/security/sessions` | GET /sessions | caller own sessions only (auth/http/handlers.go:413-415), not organization-wide |
| 59.10 | POST | `/security/sessions/{id}/revoke` | DELETE /sessions/:sessionId | own sessions only |

### 1.3 Full Section 59 classification (250 rows)

EXACT rows are `Y` rows in `s59.tsv` (method and path mounted as written, path parameters normalized). Machine-readable copy: `section59-endpoints.tsv`.

#### 59.2 Organization, structure and settings

| Method | Spec path | Status | Actual route | Note |
|---|---|---|---|---|
| GET | `/organizations/current` | EXACT | GET /organizations/current |  |
| PATCH | `/organizations/current` | EQUIVALENT | PUT /organizations/:organizationId, PUT /organizations/:organizationId/profile | PUT not JSON Merge Patch; edits allowed only in onboarding statuses |
| POST | `/organizations/current/submit` | EQUIVALENT | POST /organizations/:organizationId/submit |  |
| POST | `/organizations/current/closure-requests` | EQUIVALENT | POST /organizations/:organizationId/close-request | plus closure/assess\|approve\|reject\|execute, closure-status |
| GET | `/organization-settings` | PARTIAL | GET /fleet-partner/operating-mode | only operating mode + financial visibility; assignment timeout, approval thresholds, provider visibility, retention, flags (models.Settings) not readable |
| PATCH | `/organization-settings` | PARTIAL | PUT /fleet-partner/operating-mode | SettingsRepository.Update exists (repositories/settings.go:124) but no route |
| GET | `/branches` | MISSING | - | BranchRepository Create/List/Archive (repositories/branches.go:28-111) unrouted; HQ branch auto-created by migration 000087/onboarding only |
| POST | `/branches` | MISSING | - | BranchRepository Create/List/Archive (repositories/branches.go:28-111) unrouted; HQ branch auto-created by migration 000087/onboarding only |
| GET | `/branches/{branchId}` | MISSING | - | BranchRepository.GetByID unrouted |
| PATCH | `/branches/{branchId}` | MISSING | - | no update method |
| POST | `/branches/{branchId}/activate` | MISSING | - |  |
| POST | `/branches/{branchId}/deactivate` | MISSING | - | BranchRepository.Archive unrouted |
| GET | `/locations` | MISSING | - | no operating_locations table; only warehouse locations and CRM customer locations |
| POST | `/locations` | MISSING | - | no operating_locations table; only warehouse locations and CRM customer locations |
| GET | `/departments` | MISSING | - | DepartmentRepository Create/GetByID only, unrouted |
| POST | `/departments` | MISSING | - | DepartmentRepository Create/GetByID only, unrouted |
| PATCH | `/departments` | MISSING | - | DepartmentRepository Create/GetByID only, unrouted |
| GET | `/teams` | MISSING | - | no table |
| POST | `/teams` | MISSING | - | no table |
| PATCH | `/teams` | MISSING | - | no table |
| GET | `/cost-centres` | MISSING | - | no table (departments.cost_centre_id column only) |
| POST | `/cost-centres` | MISSING | - | no table (departments.cost_centre_id column only) |
| PATCH | `/cost-centres` | MISSING | - | no table (departments.cost_centre_id column only) |

#### 59.3 Users, roles and approvals

| Method | Spec path | Status | Actual route | Note |
|---|---|---|---|---|
| GET | `/users` | EQUIVALENT | GET /staff |  |
| POST | `/users/invitations` | EQUIVALENT | POST /staff/invitations | also GET /staff/invitations, DELETE /staff/invitations/:invitationId |
| POST | `/users/invitations/{id}/resend` | EQUIVALENT | POST /staff/invitations/:invitationId/resend | added d83e81e |
| POST | `/users/{id}/suspend` | EQUIVALENT | POST /staff/:staffId/suspend |  |
| POST | `/users/{id}/reactivate` | EQUIVALENT | POST /staff/:staffId/reactivate |  |
| GET | `/roles` | EXACT | GET /roles |  |
| POST | `/roles` | MISSING | - | RoleRepository.CreateRole (staff/repositories/roles.go:130) unrouted |
| GET | `/roles/{id}` | MISSING | - | RoleRepository.GetRole (staff/repositories/roles.go:85) exists, no route (GET /roles list only) |
| PATCH | `/roles/{id}` | MISSING | - | RoleRepository.UpdateRole unrouted |
| PUT | `/users/{id}/roles` | MISSING | - | ReplaceRoleBindings usecase (staff/usecases/membership_flows.go:161) unrouted |
| GET | `/permissions` | EXACT | GET /permissions |  |
| GET | `/approval-policies` | MISSING | - | policies auto-created from templates (approvals/repositories/approvals.go) |
| POST | `/approval-policies` | MISSING | - | policies auto-created from templates (approvals/repositories/approvals.go) |
| GET | `/approval-policies/{id}` | MISSING | - |  |
| PATCH | `/approval-policies/{id}` | MISSING | - |  |
| GET | `/approvals` | EQUIVALENT | GET /approvals/requests |  |
| GET | `/approvals/{id}` | EQUIVALENT | GET /approvals/requests/:approvalId (+/decisions, /history) |  |
| POST | `/approvals/{id}/decisions` | EQUIVALENT | POST /approvals/requests/:approvalId/approve \| /reject | quorum + self-approval rules in approvals/usecases/service.go:217,1003 |
| POST | `/approval-delegations` | MISSING | - | table fleet_partner_approval_delegations (migration 000006:116) has no code |

#### 59.4 Managed providers and workforce

| Method | Spec path | Status | Actual route | Note |
|---|---|---|---|---|
| GET | `/providers` | EXACT | GET /providers |  |
| POST | `/providers` | EXACT | POST /providers |  |
| GET | `/providers/{id}` | EXACT | GET /providers/:id |  |
| PATCH | `/providers/{id}` | MISSING | - |  |
| POST | `/providers/{id}/activate` | MISSING | - | activation is automatic: providers/usecases/lifecycle.go:189 TryActivate after association + sub-wallet |
| POST | `/providers/{id}/suspend` | EXACT | POST /providers/:id/suspend |  |
| POST | `/providers/{id}/reactivate` | MISSING | - | ChangeStatus (lifecycle.go:244) supports it, unrouted; member-layer POST /managed-members/:memberId/reinstate is a different aggregate |
| POST | `/providers/{id}/end` | MISSING | - | ChangeStatus REMOVED unrouted; no offboarding of shifts/assets/sub-wallet |
| POST | `/providers/{id}/service-roles` | PARTIAL | POST /managed-members/:memberId/links | member-layer service link (FLEET_MULTI_SERVICE_PROVIDER_LINKS_ENABLED), not the membership aggregate |
| DELETE | `/providers/{id}/service-roles` | MISSING | - | no link removal route |
| POST | `/providers/{id}/branches` | MISSING | - | membership carries one branch_id; no reassignment |
| POST | `/providers/{id}/vehicles` | EQUIVALENT | POST /assets/:assetId/assignments | keyed by asset; providerMembershipId,startAt,endAt in body |
| GET | `/providers/{id}/availability` | MISSING | - | no projection route; ProviderAuthority lookup (platformclients/provider_authority.go) used only inside eligibility |
| GET | `/providers/{id}/declines` | PARTIAL | GET /marketplace/assignments?status=DECLINED | no membership filter (marketplace/http/handlers.go:82-90) |
| GET | `/shifts` | EXACT | GET /shifts |  |
| POST | `/shifts` | EXACT | POST /shifts |  |
| PATCH | `/shifts/{id}` | EXACT | PATCH /shifts/:id |  |
| POST | `/shifts/{id}/publish` | EXACT | POST /shifts/:id/publish |  |
| POST | `/leave-requests` | EQUIVALENT | POST /leave |  |
| POST | `/leave-requests/{id}/decisions` | EQUIVALENT | POST /leave/:leaveId/approve \| /reject |  |
| GET | `/training-courses` | EQUIVALENT | GET /training/courses |  |
| POST | `/training-courses` | EQUIVALENT | POST /training/courses | no modules/assessments |
| POST | `/training-enrolments` | MISSING | - | no enrolment aggregate |
| POST | `/training-enrolments/{id}/complete` | PARTIAL | POST /training/courses/:courseId/complete | completion recorded directly against course + membership (workforce/service.go:717) |
| GET | `/provider-performance` | PARTIAL | GET /performance/providers/:membershipId | per-provider only; no date/branch filters; snapshots from private assignments only |

#### 59.5 Marketplace, assignment and live trips

| Method | Spec path | Status | Actual route | Note |
|---|---|---|---|---|
| GET | `/marketplace/offers` | EXACT | GET /marketplace/offers |  |
| GET | `/marketplace/offers/{id}` | EXACT | GET /marketplace/offers/:id |  |
| POST | `/marketplace/offers/{id}/accept` | EXACT | POST /marketplace/offers/:id/accept |  |
| POST | `/marketplace/offers/{id}/reject` | EQUIVALENT | POST /marketplace/offers/:offerId/decline |  |
| GET | `/marketplace/offers/{id}/eligible-providers` | EQUIVALENT | GET /marketplace/requests/:projectionId/candidates | keyed by request projection |
| POST | `/marketplace/offers/{id}/assignments` | EQUIVALENT | POST /marketplace/requests/:projectionId/assign |  |
| GET | `/assignments` | EQUIVALENT | GET /marketplace/assignments |  |
| GET | `/assignments/{id}` | EQUIVALENT | GET /marketplace/assignments/:assignmentId (+/history) |  |
| POST | `/assignments/{id}/accept` | EQUIVALENT | internal POST /internal/fleet-partner/v1/assignments/:assignmentId/provider-accept | provider-only relay via execution services; unmounted when FLEET_MARKETPLACE_ALLOWED_CALLERS is empty |
| POST | `/assignments/{id}/decline` | EQUIVALENT | internal POST /internal/fleet-partner/v1/assignments/:assignmentId/provider-decline | same gating |
| POST | `/assignments/{id}/cancel` | EQUIVALENT | POST /marketplace/assignments/:assignmentId/cancel |  |
| POST | `/assignments/{id}/reassign` | EQUIVALENT | POST /marketplace/assignments/:assignmentId/reassign | writes need FLEET_REASSIGNMENT_ENABLED |
| GET | `/trips` | PARTIAL | GET /tracking | reads tracking projections; fleet_partner_trip_projections (migration 000010) has no list route |
| GET | `/trips/{id}` | PARTIAL | GET /tracking/:canonicalTripReference | no trip_projection_history read |
| GET | `/control-tower/live` | EQUIVALENT | GET /control-tower |  |
| GET | `/control-tower/exceptions` | PARTIAL | GET /alerts | operational alerts only |
| POST | `/rescue-cases` | PARTIAL | POST /breakdowns | asset breakdown carries rescue_status/replacement_asset_id (assets/domain.go Breakdown); no rescue-case aggregate for provider unavailability/delay/cargo transfer |
| POST | `/rescue-cases/{id}/resolve` | PARTIAL | PATCH /breakdowns/:breakdownId |  |

#### 59.6 Commercial, orders and routing

| Method | Spec path | Status | Actual route | Note |
|---|---|---|---|---|
| GET | `/customers` | EXACT | GET /customers |  |
| POST | `/customers` | EXACT | POST /customers |  |
| GET | `/customers/{id}` | EXACT | GET /customers/:id |  |
| PATCH | `/customers/{id}` | EXACT | PATCH /customers/:id |  |
| POST | `/customers/{id}/contacts` | EXACT | POST /customers/:id/contacts |  |
| POST | `/customers/{id}/credit-holds` | EQUIVALENT | POST /customers/:customerId/credit-profile/hold |  |
| GET | `/leads` | MISSING | - | opportunity stage LEAD exists (crm/models StageLead) but an opportunity requires an existing customer (crm/usecases/opportunities.go:74) |
| POST | `/leads` | MISSING | - | opportunity stage LEAD exists (crm/models StageLead) but an opportunity requires an existing customer (crm/usecases/opportunities.go:74) |
| POST | `/leads/{id}/convert` | MISSING | - |  |
| GET | `/quotations` | EXACT | GET /quotations |  |
| POST | `/quotations` | EXACT | POST /quotations |  |
| POST | `/quotations/{id}/submit` | EXACT | POST /quotations/:id/submit |  |
| POST | `/quotations/{id}/accept` | EXACT | POST /quotations/:id/accept |  |
| GET | `/contracts` | EXACT | GET /contracts |  |
| POST | `/contracts` | EXACT | POST /contracts |  |
| POST | `/contracts/{id}/activate` | EXACT | POST /contracts/:id/activate |  |
| POST | `/contracts/{id}/amendments` | PARTIAL | POST /contracts/:contractId/renew | renewal creates a new version; mid-term amendment of an active contract not supported (UpdateContract draft-only, crm/usecases/contracts.go:224) |
| GET | `/service-level-agreements` | EXACT | GET /service-level-agreements |  |
| POST | `/service-level-agreements` | EXACT | POST /service-level-agreements |  |
| GET | `/orders` | EXACT | GET /orders |  |
| POST | `/orders` | EXACT | POST /orders |  |
| GET | `/orders/{id}` | EXACT | GET /orders/:id |  |
| PATCH | `/orders/{id}` | MISSING | - |  |
| POST | `/orders/{id}/submit` | EQUIVALENT | POST /orders/:orderId/confirm |  |
| POST | `/orders/{id}/approve` | MISSING | - | AWAITING_APPROVAL path exists only in repository (orders/repositories/repository.go:262 MarkAwaitingApproval, no caller) |
| POST | `/orders/{id}/cancel` | EXACT | POST /orders/:id/cancel |  |
| POST | `/orders/{id}/request-execution` | PARTIAL | POST /private-assignments (+ /private-executions/:executionId) | assignment-led private execution; dispatch/hauling only, taxi not wired |
| GET | `/shipments` | EXACT | GET /shipments |  |
| POST | `/shipments` | EXACT | POST /shipments |  |
| GET | `/shipments/{id}` | EXACT | GET /shipments/:id |  |
| PATCH | `/shipments/{id}` | EXACT | PATCH /shipments/:id |  |
| POST | `/shipments/{id}/packages` | EXACT | POST /shipments/:id/packages |  |
| POST | `/shipments/{id}/exceptions` | PARTIAL | POST /packages/:packageId/transition | package-level exceptional states with reasonCode (packages/usecases/service.go:196); no shipment exception record |
| GET | `/manifests` | EXACT | GET /manifests |  |
| POST | `/manifests` | EXACT | POST /manifests |  |
| POST | `/manifests/{id}/close` | EXACT | POST /manifests/:id/close |  |
| GET | `/route-plans` | EQUIVALENT | GET /routes |  |
| POST | `/route-plans` | EQUIVALENT | POST /routes |  |
| POST | `/route-plans/{id}/optimize` | EQUIVALENT | POST /routes/:routeId/optimize |  |
| POST | `/route-plans/{id}/approve` | EQUIVALENT | POST /routes/:routeId/approve |  |
| POST | `/route-plans/{id}/publish` | EQUIVALENT | POST /routes/:routeId/publish |  |
| POST | `/route-plans/{id}/reoptimize` | EQUIVALENT | POST /routes/:routeId/replan |  |
| GET | `/geofences` | EXACT | GET /geofences |  |
| POST | `/geofences` | EXACT | POST /geofences |  |

#### 59.7 Assets, maintenance, compliance and safety

| Method | Spec path | Status | Actual route | Note |
|---|---|---|---|---|
| GET | `/assets` | EXACT | GET /assets |  |
| POST | `/assets` | EXACT | POST /assets |  |
| GET | `/assets/{id}` | EXACT | GET /assets/:id |  |
| PATCH | `/assets/{id}` | EXACT | PATCH /assets/:id |  |
| POST | `/assets/{id}/documents` | EXACT | POST /assets/:id/documents |  |
| POST | `/assets/{id}/reserve` | MISSING | - | no RESERVED operational status (migration 000040 CHECK: AVAILABLE,ASSIGNED,IN_USE,BLOCKED,UNDER_MAINTENANCE,OUT_OF_SERVICE) |
| POST | `/assets/{id}/retire` | EXACT | POST /assets/:id/retire |  |
| GET | `/inspection-templates` | EXACT | GET /inspection-templates |  |
| POST | `/inspection-templates` | EXACT | POST /inspection-templates |  |
| POST | `/inspections` | EXACT | POST /inspections |  |
| POST | `/inspections/{id}/items` | PARTIAL | POST /inspections/:inspectionId/complete | item answers submitted only at completion |
| POST | `/inspections/{id}/complete` | EXACT | POST /inspections/:id/complete |  |
| GET | `/defects` | MISSING | - | defects stored only as inspection defect_summary JSON |
| POST | `/defects` | MISSING | - | defects stored only as inspection defect_summary JSON |
| POST | `/defects/{id}/resolve` | MISSING | - |  |
| GET | `/maintenance-plans` | EQUIVALENT | GET /maintenance/schedules |  |
| POST | `/maintenance-plans` | EQUIVALENT | POST /maintenance/schedules |  |
| GET | `/work-orders` | EQUIVALENT | GET /maintenance/work-orders |  |
| POST | `/work-orders` | EQUIVALENT | POST /maintenance/work-orders |  |
| POST | `/work-orders/{id}/approve` | EQUIVALENT | POST /maintenance/work-orders/:workOrderId/approve |  |
| POST | `/work-orders/{id}/start` | EQUIVALENT | PATCH /maintenance/work-orders/:workOrderId status=IN_PROGRESS | transition APPROVED->IN_PROGRESS (assets/operations.go:167); started_at is not set by the UPDATE (operations.go:198) |
| POST | `/work-orders/{id}/complete` | EQUIVALENT | POST /maintenance/work-orders/:workOrderId/complete |  |
| POST | `/work-orders/{id}/return-to-service` | EQUIVALENT | POST /maintenance/work-orders/:workOrderId/return-to-service |  |
| GET | `/parts` | EQUIVALENT | GET /inventory/parts |  |
| POST | `/parts` | EQUIVALENT | POST /inventory/parts |  |
| POST | `/stock-movements` | EQUIVALENT | POST /inventory/stock-movements |  |
| GET | `/fuel-transactions` | EQUIVALENT | GET /fuel-records |  |
| POST | `/fuel-transactions` | EQUIVALENT | POST /fuel-records | no amount/currency fields (assets/domain.go FuelRecord) |
| POST | `/fuel-transactions/{id}/approve` | EQUIVALENT | POST /fuel-records/:fuelRecordId/approve \| /reject |  |
| GET | `/compliance/requirements` | EXACT | GET /compliance/requirements |  |
| GET | `/compliance/status` | EXACT | GET /compliance/status |  |
| POST | `/compliance/restrictions` | EXACT | POST /compliance/restrictions |  |
| POST | `/compliance/restrictions/{id}/lift` | EQUIVALENT | POST /compliance/restrictions/:restrictionId/clear |  |
| GET | `/incidents` | EXACT | GET /incidents |  |
| POST | `/incidents` | EXACT | POST /incidents |  |
| POST | `/incidents/{id}/triage` | PARTIAL | PATCH /incidents/:incidentId | severity/status edit; no triage command or immediateActions record |
| POST | `/incidents/{id}/investigations` | PARTIAL | PATCH /incidents/:incidentId (status INVESTIGATING) | no investigation record with lead/scope |
| POST | `/corrective-actions` | EQUIVALENT | POST /incidents/:incidentId/corrective-actions |  |
| POST | `/proof-records` | EXACT | POST /proof-records |  |
| GET | `/claims` | EXACT | GET /claims |  |
| POST | `/claims` | EXACT | POST /claims |  |

#### 59.8 Warehouse, yard and dock

| Method | Spec path | Status | Actual route | Note |
|---|---|---|---|---|
| GET | `/warehouses` | EXACT | GET /warehouses |  |
| POST | `/warehouses` | EXACT | POST /warehouses |  |
| GET | `/warehouses/{id}` | EXACT | GET /warehouses/:id |  |
| PATCH | `/warehouses/{id}` | EXACT | PATCH /warehouses/:id |  |
| GET | `/warehouse-zones` | EQUIVALENT | GET /warehouses/:warehouseId/locations (location_type=ZONE) | migration 000046 location_type CHECK |
| POST | `/warehouse-zones` | EQUIVALENT | POST /warehouses/:warehouseId/locations (location_type=ZONE) | migration 000046 location_type CHECK |
| GET | `/storage-bins` | EQUIVALENT | GET /warehouses/:warehouseId/locations (location_type=BIN) |  |
| POST | `/storage-bins` | EQUIVALENT | POST /warehouses/:warehouseId/locations (location_type=BIN) |  |
| GET | `/inventory-items` | EQUIVALENT | GET /inventory |  |
| POST | `/inventory-items` | PARTIAL | POST /inventory/receipts | items are created by receipts; no item-master/track-rules |
| GET | `/warehouse-stock` | EQUIVALENT | GET /inventory | stock identity warehouse/location/sku/batch/lot/serial (warehouse/service.go:391) |
| POST | `/inventory-adjustments` | EQUIVALENT | POST /inventory/movements (movementType=ADJUSTMENT) | warehouse/service.go:502 |
| GET | `/warehouse-receipts` | MISSING | - | receipts are one-shot stock postings; no receipt list |
| POST | `/warehouse-receipts` | PARTIAL | POST /inventory/receipts | no expected-receipt document/lines |
| POST | `/warehouse-receipts/{id}/arrive` | MISSING | - |  |
| POST | `/warehouse-receipts/{id}/receive` | MISSING | - | no expected/received/damaged/quarantined reconciliation |
| POST | `/putaway-tasks/{id}/claim` | MISSING | - | no claimable leased tasks |
| POST | `/putaway-tasks/{id}/complete` | PARTIAL | POST /inventory/movements (movementType=PUT_AWAY) | full-item move only (warehouse/service.go:510) |
| GET | `/pick-waves` | MISSING | - |  |
| POST | `/pick-waves` | MISSING | - |  |
| POST | `/pick-tasks/{id}/complete` | PARTIAL | POST /fulfilment-orders/:fulfilmentOrderId/pick | order-level pick; row-locked reservation (warehouse/operations.go:196-202) |
| POST | `/packing-units` | PARTIAL | POST /fulfilment-orders/:fulfilmentOrderId/pack | no packing-unit resource |
| GET | `/dock-appointments` | EXACT | GET /dock-appointments |  |
| POST | `/dock-appointments` | EXACT | POST /dock-appointments |  |
| POST | `/dock-appointments/{id}/check-in` | EXACT | POST /dock-appointments/:id/check-in |  |
| GET | `/yard-visits` | EXACT | GET /yard-visits |  |
| POST | `/yard-visits` | EXACT | POST /yard-visits |  |
| POST | `/yard-visits/{id}/gate-events` | PARTIAL | POST /yard-visits/:yardVisitId/check-in \| /check-out \| /release | fixed transitions; no generic gate/parking/loading events |

#### 59.9 Finance, wallets and reporting

| Method | Spec path | Status | Actual route | Note |
|---|---|---|---|---|
| GET | `/invoices` | EXACT | GET /invoices |  |
| POST | `/invoices` | EXACT | POST /invoices |  |
| GET | `/invoices/{id}` | EXACT | GET /invoices/:id |  |
| POST | `/invoices/{id}/submit` | MISSING | - | PENDING_APPROVAL accepted but nothing sets it; /approve goes straight from DRAFT (financeops/invoices.go:204) |
| POST | `/invoices/{id}/issue` | EXACT | POST /invoices/:id/issue |  |
| POST | `/credit-notes` | EQUIVALENT | POST /invoices/:invoiceId/credit-note |  |
| GET | `/expenses` | EXACT | GET /expenses |  |
| POST | `/expenses` | EXACT | POST /expenses |  |
| POST | `/expenses/{id}/submit` | EXACT | POST /expenses/:id/submit |  |
| POST | `/expenses/{id}/decisions` | EQUIVALENT | POST /expenses/:expenseId/approve \| /reject |  |
| GET | `/payroll-periods` | EXACT | GET /payroll-periods |  |
| POST | `/payroll-periods` | EXACT | POST /payroll-periods |  |
| POST | `/payroll-periods/{id}/calculate` | EXACT | POST /payroll-periods/:id/calculate |  |
| POST | `/payroll-periods/{id}/approve` | EXACT | POST /payroll-periods/:id/approve |  |
| POST | `/payroll-periods/{id}/allocate` | EQUIVALENT | POST /payroll-periods/:periodId/post | needs FINANCE_POSTING_ENABLED + wallet |
| GET | `/budgets` | EXACT | GET /budgets |  |
| POST | `/budgets` | EXACT | POST /budgets |  |
| GET | `/wallet` | EXACT | GET /wallet |  |
| GET | `/wallet/transactions` | EXACT | GET /wallet/transactions |  |
| GET | `/providers/{id}/sub-wallet` | EXACT | GET /providers/:id/sub-wallet |  |
| GET | `/providers/{id}/statements` | EXACT | GET /providers/:id/statements |  |
| POST | `/providers/{id}/allocations` | EQUIVALENT | POST /providers/:membershipId/sub-wallet/allocations |  |
| POST | `/allocations/{id}/approve` | EQUIVALENT | POST /providers/:membershipId/sub-wallet/allocations/:allocationId/approve |  |
| POST | `/allocations/{id}/cancel` | PARTIAL | POST /providers/:membershipId/sub-wallet/allocations/:allocationId/reject | approver rejection; no requester cancel |
| GET | `/dashboard/operations` | PARTIAL | GET /dashboard | no date/branch/service filters; not in riskregistry |
| GET | `/dashboard/finance` | MISSING | - |  |
| GET | `/reports` | MISSING | - | f/reports is doc.go only |
| POST | `/reports` | MISSING | - | f/reports is doc.go only |
| POST | `/reports/{id}/run` | MISSING | - |  |
| GET | `/report-runs/{id}` | MISSING | - |  |
| POST | `/exports` | EXACT | POST /exports |  |

#### 59.10 Communication, integrations and governance

| Method | Spec path | Status | Actual route | Note |
|---|---|---|---|---|
| GET | `/support-cases` | EQUIVALENT | GET /support/cases |  |
| POST | `/support-cases` | EQUIVALENT | POST /support/cases |  |
| POST | `/support-cases/{id}/messages` | PARTIAL | POST /communications/threads (subjectType=SUPPORT_CASE) + /messages | internal org thread; SupportPort has no message call to support-dispute-service (engagement/domain.go:72) |
| GET | `/notification-rules` | EXACT | GET /notification-rules |  |
| POST | `/notification-rules` | EQUIVALENT | PUT /notification-rules |  |
| GET | `/automation-rules` | MISSING | - | AUTOMATION_RULE_EXECUTE job always fails permanent |
| POST | `/automation-rules` | MISSING | - | AUTOMATION_RULE_EXECUTE job always fails permanent |
| POST | `/automation-rules/{id}/activate` | MISSING | - |  |
| GET | `/api-clients` | EQUIVALENT | GET /integrations/api-clients |  |
| POST | `/api-clients` | EQUIVALENT | POST /integrations/api-clients |  |
| POST | `/api-clients/{id}/rotate` | EQUIVALENT | POST /integrations/api-clients/:id/rotate |  |
| GET | `/webhook-subscriptions` | EQUIVALENT | GET /integrations/webhooks |  |
| POST | `/webhook-subscriptions` | EQUIVALENT | POST /integrations/webhooks |  |
| POST | `/webhook-deliveries/{id}/replay` | EQUIVALENT | POST /integrations/webhook-deliveries/:id/replay |  |
| POST | `/imports` | EXACT | POST /imports |  |
| GET | `/imports/{id}` | EXACT | GET /imports/:id |  |
| GET | `/audit-events` | EQUIVALENT | GET /audit-logs |  |
| GET | `/privacy-settings` | PARTIAL | GET /retention-policies, GET /security-settings | no privacy-settings aggregate |
| PATCH | `/privacy-settings` | PARTIAL | PUT /retention-policies/:class, PUT /security-settings |  |
| GET | `/security/sessions` | PARTIAL | GET /sessions | caller own sessions only (auth/http/handlers.go:413-415), not organization-wide |
| POST | `/security/sessions/{id}/revoke` | PARTIAL | DELETE /sessions/:sessionId | own sessions only |
## 2. Sections 37-55 - requirements stated only in the detail sections

The table lists requirements from Sections 37-55 (business rules, aggregates, sub-resources, fields) that the Sections 8-25 audit (`F-sections-8-25.md`) did not evaluate. Endpoints already covered by Section 59 above or by that audit are not repeated; where a 37-55 API row differs from Section 59, it appears here.

### 37-42 Organization, access, providers, marketplace, trips, wallet

| ID | Requirement (spec) | Status | Evidence |
|---|---|---|---|
| D37.1 | Statuses draft, submitted, under_review, changes_required, active, suspended, closing, closed; domain-only transitions (37.2) | PARTIAL | `f/organizations/usecases/lifecycle.go:19-55` matrix DRAFT, CONTACT_VERIFIED, BUSINESS_SUBMITTED, UNDER_REVIEW, CHANGES_REQUIRED, ACTIVE, SUSPENDED, CLOSED, ARCHIVED. No CLOSING state; closure is a separate closure-request record (`usecases/closure.go`) |
| D37.2 | Branch/department/etc. carry organization_id; cross-organization parent references rejected (37.2) | PARTIAL | Repository checks exist: `repositories/departments.go:43-63` (branch and parent must be in tenant), `branches.go:144` existsInTenant. No API reaches them; no teams/cost-centres/locations tables |
| D37.3 | Active marketplace capability required before receiving KarryGo requests for that service (37.2) | IMPLEMENTED | `f/eligibility/repositories/state.go:371-383` requires capability `status='APPROVED'` |
| D37.4 | Suspension may be organization-wide or capability-scoped (37.2) | MISSING | Capability statuses REQUESTED/APPROVED/REJECTED/WITHDRAWN/DISABLED/ENABLED (`migrations/000045_specialized_logistics.up.sql:7-9`), no SUSPENDED; org suspension is whole-org only (`lifecycle.go:46-52`); capabilities editable only during onboarding (`usecases/onboarding_service.go:880-889`) |
| D37.5 | Closure blocked by unresolved marketplace work, pending wallet command, open payroll, legal hold, required export (37.2) | IMPLEMENTED | `usecases/closure.go:19-37` blocker codes incl. active requests/assignments/trips, wallet, allocations, withdrawals, payroll, legal hold, pending exports; UNKNOWN keeps it blocked (`closure.go:111-168`) |
| D37.6 | Settings invariant provider_decline_required=true (37.5) | IMPLEMENTED | Enforced on the per-membership work policy: `migrations/000007_managed_providers.up.sql:117-118` CHECK, `f/providers/repositories/providers.go:274-304`. Not part of organization settings |
| D37.7 | Events branch.created/updated/archived (37.4) | MISSING | No branch commands; only the migration-seeded HQ branch (000087) |
| D38.1 | System role templates immutable; custom roles from global catalogue (38.2) | PARTIAL | Immutability enforced (`f/staff/repositories/roles.go:17-18` ErrSystemRoleImmutable, `is_system`); custom role create/update unrouted (59.3) |
| D38.2 | Sequential or quorum approval; requester cannot self-approve; separation of duties (38.2) | IMPLEMENTED | `f/approvals/usecases/service.go:185-219` (quorum, SelfApprovalAllowed), `:999-1010` assertMayDecide |
| D38.3 | Approval expiry (approval.expired) (38.4) | PARTIAL | Expiry enforced at decision time (`service.go:992`, `:1143` ApprovalExpired); no expiry sweep/event found |
| D38.4 | Financial limits on delegated authority (38 intro); policy money thresholds | MISSING | `threshold_amount_minor`/`threshold_currency` columns (`migrations/000006_approvals.up.sql:15-18`) never read by any Go code |
| D38.5 | Delegations time-bounded, scoped, cannot exceed delegator authority (38.2) | MISSING | Table `fleet_partner_approval_delegations` (000006:116-134) with no repository/usecase/route |
| D39.1 | Split identity (phone and email resolve to different providers) fails atomically (39.2) | IMPLEMENTED | `f/providers/usecases/provider_service.go:216-222` ProviderIdentityConflict, saga marked BLOCKED |
| D39.2 | Link existing identity; sponsored create only when none exists; no repeated onboarding (39.2) | PARTIAL | Logic real (`provider_service.go:224-262`, `ORGANIZATION_SPONSORED`), live only for HAULING (contracts for DISPATCH/TAXI fail closed) |
| D39.3 | Multiple organization memberships where conflict rules permit (39.2) | PARTIAL | Hard rule: at most one live membership per provider per service (`provider_service.go:334-347`); no configurable conflict policy |
| D39.4 | Ending membership handles active work, future shifts, asset assignment, sub-wallet access (39.2) | MISSING | No end command; `ChangeStatus` (`lifecycle.go:244-305`) only revokes eligibility in the owning service |
| D39.5 | provider_branch_assignments (multi-branch, primary flag) (39.1, 59.4) | PARTIAL | Single `HomeBranchID` on membership (`f/providers/models/models.go:110`) |
| D39.6 | provider_compensation_profiles (39.1) | IMPLEMENTED | `/compensation-rules`, `POST /fleet-partner/compensation-plans`, `POST /fleet-partner/memberships/:membershipId/compensation-assignments` |
| D40.1 | Provider acceptance rechecks deadline, membership, shift, qualifications, asset, conflicts, source availability (40.2) | IMPLEMENTED | `f/marketplace/usecases/assignment_service.go:216`, `:610-640` recheckEligibility (membership + source) |
| D41.1 | Trip projection stores source_service, trip_id, request_id, executing_provider_id, membership_id, settlement_organization_id (41.2) | PARTIAL | Stored in `fleet_partner_trip_projections` (`migrations/000010:8-36`) with history table (:49); no tenant read route (`/tracking` reads a different projection) |
| D41.2 | High-volume telemetry separated from transactional tables (41.2) | IMPLEMENTED | Separate `fleet_partner_tracking_event_inbox` / `fleet_partner_tracking_projections` (`migrations/000035:2-34`), last-known only |
| D41.3 | `GET /geofence-events` (41.3) | MISSING | Events derived into `fleet_partner_geofence_events` by worker; no read route (already noted in 16.12 as PARTIAL) |
| D42.1 | Allocation types salary, per-job, bonus, tip, expense allowance, reimbursement (42.2) | PARTIAL | CHECK list SALARY, WAGE, PER_JOB, ALLOWANCE, REIMBURSEMENT, BONUS, ADJUSTMENT_CREDIT, APPROVED_DEDUCTION, REVERSAL (`migrations/000022:112-115`); no TIP |
| D42.2 | Duplicate source payment rejected (42.2) | IMPLEMENTED | Unique business-purpose index (`migrations/000022:170-180`) |
| D42.3 | `GET /allocations` (organization-wide) (42.3) | PARTIAL | Only per provider: `GET /providers/:membershipId/sub-wallet/allocations` |
| D42.4 | `GET /wallet/reconciliation-cases` (42.3) | MISSING | Company wallet keeps `reconciliation_state` columns only (`f/companywallet/repositories/withdrawals.go:22`); no case resource |
| D42.5 | Offboarding: pending allocation and remaining balance follow controlled workflow (42.2) | MISSING | Depends on D39.4 |
| D42.6 | Provider sees only own sub-wallet/statement (42.2) | MISSING | Provider self-service routes exist in code but are never mounted (`RegisterProviderRoutes` unused; prior 13.7) |

### 43-46 CRM, orders, dispatch, workforce

| ID | Requirement (spec) | Status | Evidence |
|---|---|---|---|
| D43.1 | Optional `platform_customer_id` reference only (43.2) | MISSING | No such column in `migrations/000023_company_crm.up.sql` |
| D43.2 | Quotation and contract versions immutable after approval/issue (43.2) | IMPLEMENTED | Triggers `trg_fleet_crm_quotation_versions_immutable` (000023:296), `trg_fleet_crm_contract_versions_immutable` (:455), acceptances (:354) |
| D43.3 | Amendments create a new version with effective dates (43.2; `POST /contracts/{id}/versions`) | PARTIAL | `RenewContract` creates a successor (`f/crm/usecases/contracts.go:864`); `UpdateContract` is draft-only (`:224`) - no mid-term amendment |
| D43.4 | Credit holds block order submission (43.2) | IMPLEMENTED | `f/orders/usecases/service.go:328-334` CustomerCreditHold |
| D43.5 | SLAs versioned, metric-specific (43.2; `/slas`) | PARTIAL | Metric-specific with one active per customer/contract/metric (000023:545); row version only, no SLA version history |
| D43.6 | Leads separate from opportunities, convertible to customers (43.1/43.3) | MISSING | Opportunity stage LEAD exists but requires an existing customer (`f/crm/usecases/opportunities.go:74`) |
| D44.1 | Order, stops, first shipment and packages created atomically (44.2) | MISSING | `createOrderRequest` has no stops/shipments/packages (`f/orders/http/handlers.go:341-353`) |
| D44.2 | Stop sequence and pickup-before-delivery validated (44.2) | MISSING | Stop types declared (`f/orders/models/models.go:192-204`), no ordering validation anywhere |
| D44.3 | Package unique organization code/barcode (44.2) | IMPLEMENTED | UNIQUE(org, package_code/barcode_value/qr_value) (`migrations/000028:33-35`) |
| D44.4 | Custody events append-only; location derived (44.2) | IMPLEMENTED | `trg_fleet_custody_events_append_only` (000028:112), package scans/history triggers (:61, :85) |
| D44.5 | Manifest load/unload scan reconciliation; cannot close with missing/extra/damaged (44.2; `/manifests/{id}/load`) | MISSING | Close checks only non-empty items, item conflicts and provider/vehicle authority (`f/manifests/usecases/service.go:240-262`) |
| D44.6 | Order approval step (AWAITING_APPROVAL) | MISSING | Status in matrix (`models.go:94-95`) but `MarkAwaitingApproval` has no caller (`repositories/repository.go:262`) |
| D45.1 | Route plans: multiple pickups/deliveries, capacities, time windows, breaks, vehicle restrictions, return-to-depot (45.2) | PARTIAL | Stops with time windows, service duration, DEPOT_START/END (`migrations/000034:58-90`); capacities/breaks/vehicle rules only as free `constraint_snapshot` JSON (:43); route stop types must be PICKUP/DELIVERY/WAREHOUSE_TRANSFER/RETURN/DEPOT (`f/dispatch/service.go:370`) |
| D45.2 | Optimization input, output, provider and operator changes retained (45.2) | IMPLEMENTED | `migrations/000037_route_optimization.up.sql` input_snapshot, result_snapshot, provider_reference, constraint_violations |
| D45.3 | Re-optimization records why the sequence changed (45.2) | IMPLEMENTED | `f/dispatch/service.go:612-651` Replan with reason and replacement route |
| D45.4 | Geofence events dedupe and dwell thresholds (45.2) | IMPLEMENTED | `migrations/000036_geofences_alerts.up.sql:1,15` (duplicate suppression, DWELL) |
| D45.5 | Candidate execution covers all three services | PARTIAL | `fleet_partner_assignment_candidates.execution_service IN('DISPATCH','HAULING')` (`migrations/000034`) - no TAXI |
| D46.1 | Overnight/DST shifts valid only after explicit ambiguity resolution (46.2) | MISSING | Shifts store UTC instants + timezone (`f/workforce/service.go:225`); no ambiguity detection |
| D46.2 | Courses and assessments versioned; completion evidence + certificate expiry (46.2) | PARTIAL | Expiry and certificate media kept (`f/workforce/domain.go:122-135`); course has only row version, no modules/assessments, completion not bound to a course version (`:107-121`) |

### 47-49 Assets, maintenance, compliance

| ID | Requirement (spec) | Status | Evidence |
|---|---|---|---|
| D47.1 | Operational status available, reserved, assigned, in_service, inspection_hold, maintenance_hold, compliance_hold, breakdown, accident_hold, retired (47.2) | PARTIAL | `migrations/000040_assets_assignments_documents.up.sql:6` CHECK AVAILABLE, ASSIGNED, IN_USE, BLOCKED, UNDER_MAINTENANCE, OUT_OF_SERVICE; retired via lifecycle_status |
| D47.2 | Platform vehicle references unique and verified via owning service (47.2) | PARTIAL | Unique only per organization (`uq_fleet_assets_platform_vehicle`, 000040:12); verification read via `internal/platformclients/vehicle_authority.go` at eligibility, not at create |
| D47.3 | Asset with open blocking restriction excluded from eligibility (47.2) | IMPLEMENTED | Asset hard blocks (`f/assets/service.go:555-600`) + restriction check (`f/eligibility/usecases/checks.go:95-100`) |
| D47.4 | Retire/archive requires no active trip, route, work order or custody obligation (47.2) | PARTIAL | Only active asset assignment checked (`f/assets/service.go:186-206`) |
| D47.5 | Assign/unassign provider, costs, meter readings, history (47.3) | PARTIAL | Assign via `POST /assets/:assetId/assignments`; unassign, `/costs`, `/meter-readings`, `/history` absent; status history table exists (000040:27) without a read route |
| D47.6 | Asset ownership/type catalogue (47.2) | IMPLEMENTED | Free-text `asset_type`, `ownership_type` stored (`f/assets/service.go:128`) - no enumeration |
| D48.1 | Inspection templates versioned; inspections bind a template version (48.2) | PARTIAL | Inspections store `template_id` only (`migrations/000041:7-12`) |
| D48.2 | Completed inspections immutable (48.2) | IMPLEMENTED | Completion update guarded `status IN('DRAFT','IN_PROGRESS')` (`f/assets/service.go:490`); results table `fleet_partner_inspection_results` unique per item (000041:13-16) |
| D48.3 | Critical failure creates defects and operational restrictions in the same transaction (48.2) | PARTIAL | Asset BLOCKED + work order + event in one tx (`f/assets/service.go:490-538`); no defect record, no compliance restriction row |
| D48.4 | Maintenance triggers: date, odometer, engine hours, fault code, inspection, manual (48.2) | PARTIAL | DATE/MILEAGE/OPERATING_HOURS/COMBINED (`migrations/000041:25`, `domain.go:96-108`); inspection -> work order and manual work orders exist; fault code absent |
| D48.5 | Return-to-service verifies blocking defects, restrictions and quality checks (48.2) | IMPLEMENTED | `f/assets/operations.go:215-264` (critical inspection follow-up pass, restriction authority) |
| D48.6 | Part/tyre stock movements append-only with reservations preventing negative available (48.2) | PARTIAL | Negative stock refused (`operations.go:457`); no reservations; tyres absent |
| D48.7 | Fuel records include quantity, amount, currency, asset, meter, evidence (48.2) | PARTIAL | Quantity, meter, receipt media present; no amount/currency (`f/assets/domain.go` FuelRecord; `operations.go:503`) |
| D48.8 | Fuel anomaly detection creates review flags (48.2) | MISSING | No anomaly logic; only manual approve/reject |
| D48.9 | `POST /maintenance/work-orders/{id}/start` sets start (48.3) | PARTIAL | Via PATCH status IN_PROGRESS (`operations.go:167`), but `started_at` never written (`operations.go:198`) |
| D49.1 | Global/country requirements plus effective-dated organization overlays; stricter governs (49.2) | PARTIAL | Effective-dated, organization-scoped only (`migrations/000043:2-18`, organization_id NOT NULL); no global layer or precedence |
| D49.2 | Providers may report safety concerns/incidents regardless of company role (49.2) | MISSING | `/safety/reports`, `/incidents` are corporate-only; no internal/provider ingress |
| D49.3 | Evidence and proof immutable; corrections supersede (49.2) | IMPLEMENTED | Append-only triggers on incident history/evidence, proof records/verifications (`migrations/000044:35,44,95,104`) |
| D49.4 | Proof types: OTP, signature, photo, video, barcode, QR, identity, seal, temperature, GPS, timestamp (49.2) | PARTIAL | `migrations/000044:80` lacks VIDEO (barcode/QR via PACKAGE_SCAN, GPS via LOCATION) |
| D49.5 | `POST /proofs/{id}/verify`, `GET /proofs/{id}` (49.3) | PARTIAL | Automatic media verification only (`f/proof/service.go:310`, `workers.go:49-70`); no manual verify/reject, list only |
| D49.6 | Claims reference incidents (49.2) | IMPLEMENTED | `incident_id` FK (`migrations/000049:54`) |
| D49.7 | `GET /corrective-actions`, `GET /restrictions` lists (49.3) | MISSING | Only per-incident create + complete/verify, and `GET /compliance/restrictions/:restrictionId` |

### 50-55 Specialized, warehouse, finance, communications, integrations

| ID | Requirement (spec) | Status | Evidence |
|---|---|---|---|
| D50.1 | `GET/PATCH /capabilities/{code}/settings` (50.3) | PARTIAL | Configuration only set through enable/disable (`f/specialized/service.go:92-108`) |
| D51.1 | Stock identity organization/bin/item/lot/serial (51.2) | IMPLEMENTED | `f/warehouse/service.go:391` lock key includes location, sku, batch, lot, serial |
| D51.2 | Zones and bins (51.3) | IMPLEMENTED | `/warehouses/:warehouseId/locations` with `location_type` ZONE/AISLE/RACK/SHELF/BIN/... (`migrations/000046:18`) - corrects prior 21.11 |
| D51.3 | Receiving records expected/received/damaged/quarantined; discrepancies resolved before completion (51.2) | MISSING | `POST /inventory/receipts` posts stock in one step; no receipt document or lines |
| D51.4 | Put-away/pick tasks claimable and leased (51.2) | MISSING | No task resource (PUT_AWAY only as a movement type, `service.go:510`) |
| D51.5 | Concurrent picking reserves stock in a row-locked transaction (51.2) | IMPLEMENTED | `f/warehouse/operations.go:196-202` FOR UPDATE + reserved_quantity |
| D51.6 | Dock appointments prevent confirmed overlap by dock (51.2) | IMPLEMENTED | `operations.go:365` overlap EXISTS check |
| D51.7 | Yard visits preserve check-in, gate, parking, loading, check-out events (51.2) | PARTIAL | check-in/check-out/release transitions with gate/bay/seal fields (`operations.go:455-480`); no event log for parking/loading |
| D51.8 | Return disposition restock, quarantine, repair, return-to-sender, destroy, claim (51.2) | IMPLEMENTED | `migrations/000047:81` RESTOCK, QUARANTINE, REPAIR, RETURN_TO_OWNER, DISPOSE, INVESTIGATE (no CLAIM value) |
| D52.1 | No multi-currency aggregation without a conversion snapshot (52.2) | IMPLEMENTED | Per-record currency; payroll rules filtered by period currency (`f/financeops/payroll.go:210`) |
| D52.2 | Payroll calculation hash and breakdown retained before approval (52.2) | PARTIAL | Breakdown items with rule_version, calculation_version, source_inputs (`payroll.go:321-348`); no calculation hash |
| D52.3 | Issued invoices immutable; corrections via credit notes (52.2) | IMPLEMENTED | Edit allowed only DRAFT/PENDING_APPROVAL (`f/financeops/invoices.go:160`); credit-note route |
| D53.1 | Threads scoped with participants (53.2) | IMPLEMENTED | `fleet_partner_communication_participants` (`migrations/000051:21`), subject types (`f/engagement/communications.go:18`) |
| D53.2 | Customer notification preferences; `GET/PATCH /notification-preferences` (53.1/53.3) | MISSING | Only organization notification rules (`PUT /notification-rules`) |
| D53.3 | `GET /sla-breaches` (53.3) | PARTIAL | `/sla/measurements` carries breach_status (`migrations/000052:43`) |
| D55.1 | Exports require step-up for sensitive categories (55.2) | MISSING | `/exports` is RiskHighRisk in riskregistry (`internal/platform/riskregistry/registry.go:240`), which only selects a rate limit (`internal/platform/ratelimit/riskclass.go:29`); no step-up in `f/exports` |
| D55.2 | API clients scoped, rotatable, optionally IP-restricted (55.2) | IMPLEMENTED | `f/integrations/service.go:162-182` (scopes, CIDR allowlist) |
| D55.3 | Webhooks HTTPS, SSRF-safe, HMAC, timestamp/replay protection (55.2) | IMPLEMENTED | `f/integrations/security.go:119`, `worker.go:261`, `:293-304` |


## 3. Cross-service work required

Grouped by the service that must change. Fleet-side gaps (unrouted repositories, missing aggregates) are in Sections 1-2 and are not repeated here.

**driver-dispatch-delivery-service**
- Implement the managed-provider contract Fleet expects (`ProviderContract` in `internal/platformclients/contracts.go`: Resolve by phone+email, ProvisionSponsored with `ORGANIZATION_SPONSORED` onboarding, ReadProvider, CreateAssociation/ReadAssociation, RevokeOrganizationEligibility). Today only a provider authority read exists (`internal/features/authority/authority.go:426-427`), and Fleet keeps the DISPATCH contract `UnconfiguredProvider` (`contracts.go:375-384`), so `POST /providers` for dispatch always fails closed. Then wire a real client in `internal/app/wiring.go` next to the hauling one (:271).
- Publish the provider availability projection that `GET /providers/{id}/availability` and live control need (41.1 provider_availability_projections). The authority lookup's `Available`/`AvailabilityState` is read per request, not projected.
- Relay provider safety reports and incidents from the rider app to Fleet (49.2).
- Accept membership-end signals: offboarding must close future work and sub-wallet access (39.2/42.2).

**driver-taxi-service**
- Same managed-provider contract as dispatch (resolve/provision/association/revoke); the TAXI contract is unconfigured, and `production_contracts_test.go:35` asserts it stays unconfigured.
- A private-assignment relay. Hauling has `/provider/private-assignments/*` (`driver-hauling-service/internal/features/providerassignments/http.go:118-125`) and dispatch has one (`internal/features/marketplace/fleet_responder.go`); taxi only relays organization offers and company assignments (`internal/features/fleetprovider/http.go:67-75`). Fleet's private taxi execution (`PRIVATE_TAXI_EXECUTION_ENABLED`) and specialized taxi records (`f/specialized` validation refuses taxi) depend on it.
- Execution start and trip events for corporate marketplace work (`FLEET_TAXI_INTEGRATION_ENABLED`), plus candidate support (Fleet candidates exclude TAXI, `migrations/000034`).

**driver-hauling-service**
- Already provides resolve/provision/provider-links, execution, and the provider relays. Still needed: an availability projection feed, safety-report relay, and membership-end handling, as for dispatch.

**payment-wallet-service**
- Provider sub-wallet self-service for providers (Fleet's provider routes are unmounted; the provider apps need either Fleet provider routes behind execution-service relays or a direct wallet surface) (42.2).
- A controlled offboarding workflow for sub-wallet balance, pending allocations and access (42.2).
- A TIP allocation type if added on the Fleet side (Fleet CHECK `migrations/000022:112-115`).
- A reconciliation-case read model for `/wallet/reconciliation-cases` (42.3).

**support-dispute-service**
- A message/attachment API for platform support cases with organization context, so `POST /support-cases/{id}/messages` can reach the platform case instead of an internal Fleet thread (`f/engagement/domain.go:72` SupportPort has only Create).

**notification-service**
- Per-customer notification preferences (53.2) and template approval for automatic customer updates. Fleet only stores organization rules.
- An admin/ops recipient audience, for escalations and SOS-style rescue alerts.

**verification-compliance-service**
- Global/country compliance requirement catalogue that Fleet overlays (49.2), and platform-vehicle uniqueness/verification across organizations (47.2).

**media-file-service**
- No new work identified; Fleet already uses it for documents, proofs, certificates and evidence.

**Route optimizer / analytics (new or external)**
- Route optimization provider (adapter exists, `f/dispatch/optimizer_client.go`, unconfigured by default).
- Report runner, scheduled reports, dashboards and recommendation/AI services (Section 54) do not exist anywhere.

## 4. Frontend work required

### 4.1 web/fleet (Next.js)

Snapshot at HEAD `9e3f4ba`. Method: API paths were extracted from `web/fleet/src/lib/services/*.ts` and from page code, then compared with mounted tenant routes (`(audit working file)`). Page data sources were checked by grepping each module for `MOCK_`/mock-data imports, `setTimeout` and "TODO: implement". 267 mounted tenant routes have no web caller (285 without a literal call, minus 18 reached through dynamic command helpers).

Backend capability that works but has no real page (the page is mock, a TODO stub or absent):

| Module | Backend routes (working) | Current web state |
|---|---|---|
| Company wallet and provider sub-wallets | `/wallet`, `/wallet/transactions[/:id]`, `/wallet/policy`, `/providers/:id/sub-wallet[/statement]`, `.../allocations[/:id/approve\|reject\|replay]`, `.../withdrawals[...]` | `wallet/*` 13 pages, all mock; no nav entry |
| Private operations | `/orders*` (create, confirm, cancel, stops, history, from-quotation, imports), `/recurring-orders*`, `/shipments*` (+cargo-items, packages), `/packages/:id` (scan, transition, custody, history), `/manifests*` (items, seal-amendments, close, cancel), `/service-catalog*`, `/private-assignments*`, `/private-executions*` | `private-ops/*` 16 pages, all "TODO: implement" stubs |
| Compliance, safety, proof | `/compliance/requirements*`, `/compliance/status`, `/compliance/evaluations`, `/compliance/restrictions[/:id/clear]`, `/providers/:id/compliance`, `/assets/:id/compliance`, `/incidents*` (+evidence, corrective actions), `/corrective-actions/:id/complete\|verify`, `/safety/reports*`, `/safety/references*`, `/proof-policies`, `/proof-records` | `compliance/*` 13 mock pages; `fleet/assets/[assetId]/compliance` stub; provider compliance tab mock |
| Documents, imports, exports, search | `/documents*` (lifecycle, verification, deletion, download, reconciliation), `/documents/upload-requests`, `/imports*`, `/exports*`, `/search*` | `documents/*` 13 mock pages |
| Communications, support, quality, SLA | `/communications/threads*`, `/notification-rules`, `/support/cases*`, `/quality/cases*`, `/sla/measurements*`, `/escalations*` | `comms/*` 10 mock pages |
| Approvals | `/approvals/requests*` (list, detail, decisions, history, approve, reject, cancel, execute) | `settings/approvals*` mock |
| Audit and security | `/audit-logs[/:id]`, `/audit/verify-chain`, `/sessions`, `DELETE /sessions/:id`, `/security-settings`, `/legal-holds*`, `/retention-policies*`, `/retention-runs*` | `settings/audit*`, `sessions`, `security`, `legal-holds*`, `retention*` mock or stubs |
| Integrations | `/integrations/api-clients*` (create, rotate, revoke), `/integrations/webhooks*` (rotate-secret, deliveries, replay) | `settings/integrations/*` 12 TODO stubs |
| Organization lifecycle settings | `/organizations/:id/close-request`, `/closure/*`, `/closure-status`; `GET/PUT /fleet-partner/operating-mode`; `GET/PUT /organizations/:id/capabilities` | `settings/closure`, `settings/capabilities`, `settings/policies` stubs; no operating-mode page |
| System operations | `/jobs*`, `/scheduled-tasks*`, `/synchronization/*` | `system/*` 8 mock pages |
| Eligibility | `/eligibility/decisions*`, `/eligibility/policies`, `/eligibility/evaluations` | no page |
| Multi-service members | `/managed-members*`, `/managed-assets*` | no page |
| Compensation plans | `POST /fleet-partner/compensation-plans`, `/fleet-partner/memberships/:id/compensation-assignments` | no page (`finance/compensation-rules` covers rules only) |
| Dispatch planning reads | `/dispatch/board`, `/dispatch/candidates`, `/dispatch/unassigned` | dispatch pages read `/control-tower` instead |
| Warehouse commands | dock `check-in`/`complete`, yard `check-in`/`check-out`/`release`, `PATCH /dock-appointments/:id` | list/detail pages exist, no action buttons call these |
| Finance commands | budgets approve/close, compensation approve/deactivate, expenses submit/approve/reject/reverse, payroll calculate/approve/post/reverse, settlements approve/post | wired in `financeService.ts` via `runCommand`; `/finance/settlements` list is still mock |
| Specialized records | `/specialized/:code/records*` (milestones, commands, cold-chain temperature) | only capability enable/disable is called |

Pages that exist but whose backend is missing (Section 1.1), so they cannot become real until the API exists:
- `settings/branches`, `settings/departments`, `settings/locations` (stubs).
- `settings/approval-policies`, `settings/approvals/delegation`, `settings/roles/[roleId]`, `settings/roles/assignments` (mock).
- `providers/[membershipId]/availability`, `/declines`, `/work-setup` (mock via `src/lib/mock-provider-data`).
- `reports/*` (8 mock pages).
- `wallet/reconciliation`.

### 4.2 Provider apps (`apps/*`, Flutter)

- `apps/dispatch_provider`: lists and accepts/declines organization offers and company assignments (`lib/features/requests/data/requests_api.dart:90-119, 257-262`). Missing:
  - private-assignment UI (the dispatch relay exists);
  - an "employer" view: membership, branch, schedule/shifts, leave, qualifications, training, explainable performance (46.2);
  - own sub-wallet and statement (42.2);
  - safety/incident reporting to the company (49.2);
  - decline reason capture aligned with Fleet's catalogue, if not already sent.
- `apps/truck_provider`: no corporate UI at all. Hauling serves `/provider/company-assignments` (RegisterFleetWork, `driver-hauling-service/cmd/main.go:698`) and `/provider/private-assignments/*`, but the app calls neither. Needs offers/assignments accept-decline, then the same employer, sub-wallet and safety screens as dispatch.
- `apps/taxi_provider`: no corporate UI. Taxi serves `/provider/organization-offers*` and `/provider/company-assignments*` (`driver-taxi-service/internal/features/fleetprovider/http.go:67-75`). Same list as the truck app; private assignments also need the missing taxi backend relay.
- Sponsored onboarding (39.2: "provider does not repeat normal onboarding"): the three apps need an invitation/association-acceptance entry point for organization-sponsored accounts. None was found (`grep` for organization/corporate in `truck_provider` and `taxi_provider` returns only auth models).
