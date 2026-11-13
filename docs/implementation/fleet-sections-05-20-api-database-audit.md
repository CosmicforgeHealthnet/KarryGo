# Fleet Sections 5-20: API and database reconciliation

Snapshot 2026-11-13, branch `staging`. Companion to
`fleet-sections-05-20-requirements-matrix.md`. Prior full route inventory:
`spec-v2-audit/section59-endpoints.tsv` and `spec-v2-audit/E-api-and-detail-sections.md`
(250 Section 59 endpoints: 90 exact, 72 under an equivalent path, 30 partial, 58 missing at audit time).

## 1. Path convention (unchanged)

Fleet keeps `/fleet-partner/v1` (tenant) and `/internal/fleet-partner/v1`
(HMAC service) instead of the spec's `/api/v1/corporate-logistics` (ADR-003).
Every new route below follows the existing convention. No route was renamed.

## 2. Routes added this session (35)

All are tenant routes behind the existing bearer session, tenant middleware,
default-deny authorizer and object-authorization registry. Writes are
audited and emit outbox events. Each is classified in
`internal/platform/riskregistry` and covered by
`internal/section28contract` (route coverage and no-stale-entry tests).

| Method | Path | Permission | Spec | Reuse decision |
|---|---|---|---|---|
| GET | /branches | organization.read | 7, 37.3 | EXTEND: `BranchRepository.List` existed, unrouted |
| POST | /branches | organization.update | 7, 8 stage 5 | EXTEND: `BranchRepository.Create` existed |
| GET | /branches/:branchId | organization.read | 7 | EXTEND |
| PATCH | /branches/:branchId | organization.update | 7 | CREATE `BranchRepository.Update` |
| POST | /branches/:branchId/archive | organization.update | 7 | EXTEND: `Archive` existed |
| GET | /departments | organization.read | 7 | CREATE `DepartmentRepository.List` |
| POST | /departments | organization.update | 7 | EXTEND: `Create` existed |
| GET | /departments/:departmentId | organization.read | 7 | EXTEND |
| PATCH | /departments/:departmentId | organization.update | 7 | CREATE `Update` (cycle check) |
| POST | /departments/:departmentId/archive | organization.update | 7 | CREATE `Archive` |
| GET | /organization-settings | organization.read | 7, 8 stage 8 | EXTEND: `SettingsRepository.Get` |
| PATCH | /organization-settings | organization.update | 7, 8 | EXTEND: `SettingsRepository.Update` |
| PATCH | /staff/:staffId | staff.manage | 9.4 | CONNECT: `UpdateStaff` use case existed, unrouted |
| GET | /staff/:staffId/role-bindings | roles.read | 9.8 | CONNECT: `ListBindings` |
| PUT | /staff/:staffId/roles | roles.manage | 9.8, 9.9 | REPAIR+CONNECT: `ReplaceRoleBindings` was org-scope only |
| POST | /roles | roles.manage | 9.7 | CONNECT: `RoleRepository.CreateRole` |
| GET | /roles/:roleId | roles.read | 9.7 | CONNECT: `GetRole` |
| PATCH | /roles/:roleId | roles.manage | 9.7 | CONNECT: `UpdateRole` |
| POST | /roles/:roleId/archive | roles.manage | 9.7 | CONNECT: `ArchiveRole` |
| PATCH | /providers/:membershipId | providers.manage | 10.4 | CREATE (membership columns existed) |
| POST | /providers/:membershipId/reactivate | providers.manage | 10.6 | CONNECT: `ChangeStatus` supported it |
| POST | /providers/:membershipId/remove | providers.remove | 10.7 | CONNECT: `ChangeStatus` + live-work guard |
| GET | /providers/:membershipId/capabilities | providers.read | 10.9 | CONNECT: `ListCapabilities` |
| PUT | /providers/:membershipId/capabilities | providers.manage_capabilities | 10.9 | CONNECT: `ReplaceCapabilities` |
| GET | /providers/:membershipId/policies | providers.read | 10.10 | CONNECT: `GetPolicy` |
| PUT | /providers/:membershipId/policies | providers.manage_policies | 10.10 | CONNECT: `UpsertPolicy` |
| GET | /providers/:membershipId/declines | providers.read | 10.12, 39.3 | CREATE (reads existing response tables) |
| GET | /shift-templates | shifts.read | 17, 46.3 | CREATE |
| POST | /shift-templates | shifts.manage | 46.3 | CREATE |
| GET | /shift-templates/:templateId | shifts.read | 46.3 | CREATE |
| PATCH | /shift-templates/:templateId | shifts.manage | 46.3 | CREATE |
| POST | /shift-templates/:templateId/generate | shifts.manage | 46.2 | CREATE (writes existing `fleet_partner_shifts`) |
| GET | /training/enrollments | training.read | 46.3, 59.4 | CREATE |
| POST | /training/enrollments | training.manage | 46.3 | CREATE |
| POST | /training/enrollments/:enrollmentId/complete | training.record_completion | 46.3 | CREATE, completes through existing `CompleteCourse` |
| POST | /training/enrollments/:enrollmentId/cancel | training.manage | 46.3 | CREATE |

Duplicates checked before creating: no existing branch, department, settings,
role-admin, provider-admin, template or enrolment routes existed under any
path. `/managed-members/:memberId/reinstate` (multi-service member layer) is a
different aggregate and was left as is.

Pre-activation: the organization-structure and access-administration writes
were added to `middleware.PreActivationWriteRoutes` (spec 8 stages 5 and 8
happen before activation, as do the existing staff invitation routes). The
provider, shift and training writes were **not**: they remain blocked until
the company is ACTIVE.

## 3. Database

### New (migration 000089, reversible)

| Table / change | Spec entity (57.2) | Notes |
|---|---|---|
| `fleet_partner_shift_templates` | provider_shift_templates | weekdays smallint[], HH:MM checks, status ACTIVE/ARCHIVED, version |
| `fleet_partner_shifts.template_id`, `template_local_date` + unique (template_id, template_local_date) | provider_shifts.template_id | one generated shift per template per local day |
| `fleet_partner_training_enrollments` | training_enrollments | course_version snapshot; partial unique open enrolment; completion_id FK to existing completions |

Replay: `section30contract.TestMigrationReplay_LatestMigrationRollbackAndReapply`
now applies, rolls back (tables dropped) and re-applies 000089.

### Reused, no schema change

`fleet_partner_branches`, `fleet_partner_departments`,
`fleet_partner_organization_settings`, `fleet_partner_roles`,
`fleet_partner_role_permissions`, `fleet_partner_role_bindings`
(scope_type/scope_id/expires_at already existed),
`fleet_partner_organization_provider_memberships`,
`fleet_partner_provider_capabilities`, `fleet_partner_provider_policies`
(`provider_decline_required = true` CHECK), `fleet_partner_assignment_responses`,
`fleet_partner_private_assignments`, `fleet_partner_safety_decline_inbox`,
`fleet_partner_work_orders`, `fleet_partner_asset_safety_blocks`.

### Still missing tables (from Section 57 audit, unchanged)

Teams, cost centres, locations, leads, provider availability, rescue
cases, meter readings, tyres, defects, investigations and the other 26
tables listed in `spec-v2-audit/A-foundations.md` 57.x, except the two
created above.

## 4. Remaining API gaps in Sections 5-20

From the matrix (MISSING/PARTIAL rows): approval policies and delegations
(9.12, 9.13); provider resend-login, availability, branches/vehicles/service-roles
(10.8, 10.11, 10.12); dispatch/taxi provider contracts (10.2); execution start
and `/trips` projection (12.2, 12.3); provider self-service wallet routes (13.7);
leads (14.10); PATCH /orders (15.3); private-order execution (15.10);
geofence events read and rescue cases (16.12, 16.14); provider self-view and
performance from marketplace work (17.10, 17.15); meter readings, tyres,
defects, unassign, asset costs (18.12); custody events, investigations, proof
verification, compliance checks (19.12, 19.13); taxi passenger records,
capability schemas (20.3, 20.13, 20.14).
