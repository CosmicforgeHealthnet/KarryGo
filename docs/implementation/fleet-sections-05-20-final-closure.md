# Fleet Sections 5-20: closure report

Date 2026-11-13. Branch `staging`. Service `services/fleet-partner-service`.
Companion documents: requirements matrix, API/database audit, integration
audit and implementation log (`fleet-sections-05-20-*.md`).

## 1. Executive summary

This session closed 19 audit rows in Sections 7-11, 17 and 18 with tested
code, and left the rest of Sections 5-20 as recorded in the matrix. **Sections
5-20 are not fully closed.** The remaining work is mostly cross-service
(Dispatch/Taxi provisioning contracts, marketplace offer producers, Wallet
staging secret) plus about a dozen tenant-side APIs and the frontend wiring
of the new routes.

What changed:
- **Organization structure (7, 8):** branches, departments and organization
  settings now have APIs on top of repositories that existed but were never
  routed.
- **Access administration (9):** staff edit, custom roles, and role bindings
  with branch/department scope and expiry, with escalation, self-change and
  owner-role guards.
- **Managed providers (10):** edit, reactivate (gates re-checked), remove
  (refused while live work exists; identity kept), capabilities, policies
  (decline right cannot be turned off), decline history.
- **Safety (11.13, 18.11):** marketplace unsafe-vehicle declines now start
  the inspection / work-order / asset-block workflow; a decline on a vehicle
  Fleet does not manage can no longer stall the sweep.
- **Workforce (17):** shift templates (DST-gap aware, overnight, idempotent
  generation) and training enrolments (migration 000089, reversible).

35 routes, 1 migration, 4 new PostgreSQL-backed tests, no new service and
no new external connection. Nothing was renamed or removed.

## 2. Requirement totals (matrix rows)

CLOSED = implemented and tested this session. IMPLEMENTED (audit) = per the
earlier audit, not re-verified this session.

| Section | CLOSED | IMPLEMENTED (audit) | PARTIAL | MISSING | Other | Status |
|---|---|---|---|---|---|---|
| 5 Repository and conventions | 0 | 8 | 4 | 0 | | NOT CLOSED |
| 6 API and data conventions | 0 | 11 | 3 | 0 | | NOT CLOSED |
| 7 Multi-tenancy | 3 | 10 | 0 | 0 | | IMPLEMENTED - LIVE CERTIFICATION BLOCKED (not deployed-verified) |
| 8 Auth, onboarding, lifecycle | 2 | 10 | 1 | 0 | 1 superseded, 1 external, 1 pending live | NOT CLOSED (8.15 teams/cost centres/locations) |
| 9 Users, roles, approvals | 5 | 8 | 0 | 2 | | NOT CLOSED (approval policies, delegations) |
| 10 Managed providers | 5 | 7 | 2 | 2 | | BLOCKED - EXTERNAL DEPENDENCY (Dispatch/Taxi contracts) + gaps |
| 11 Accept / decline | 1 | 12 | 0 | 0 | | IMPLEMENTED - LIVE CERTIFICATION BLOCKED (provider routes need caller allowlist on staging) |
| 12 Marketplace and trips | 0 | 5 | 2 | 0 | 1 external | BLOCKED - EXTERNAL DEPENDENCY (offer producers) |
| 13 Wallet and sub-wallets | 0 | 9 | 1 | 0 | 1 external | BLOCKED - EXTERNAL DEPENDENCY (Wallet secret on staging) |
| 14 CRM and contracts | 0 | 10 | 0 | 1 | | NOT CLOSED (leads) |
| 15 Orders, shipments | 0 | 9 | 1 | 1 | | NOT CLOSED |
| 16 Dispatch and tracking | 0 | 12 | 1 | 1 | | NOT CLOSED |
| 17 Workforce | 2 | 12 | 1 | 1 | | NOT CLOSED (provider self-view) |
| 18 Assets and maintenance | 1 | 10 | 0 | 1 | | NOT CLOSED (meter readings, tyres, defects) |
| 19 Compliance and safety | 0 | 11 | 1 | 1 | | NOT CLOSED |
| 20 Specialized logistics | 0 | 11 | 2 | 1 | | NOT CLOSED. Source text found (spec 20 and 50); not a source blocker. |

## 3. Reused vs created

Reused: branch, department, settings, role, binding, member, provider
membership, capability and policy repositories; `UpdateStaff`,
`ReplaceRoleBindings`, `ChangeStatus`, `EvaluateActivation`, `CompleteCourse`;
the authorizer's escalation guard; the safety-decline inbox and work-order
pipeline; the outbox publisher and audit recorder.
Repaired: role bindings were organization-scope only; the safety-decline
reconciler ignored marketplace work and could wedge.
Created: branch update, department list/update/archive, role-admin and
provider-admin use cases, shift templates and enrolments.
Consolidated: nothing needed consolidation; no duplicate routes or tables
were found for the work done.

## 4. Verification evidence

- PostgreSQL 16 (local cluster), fresh database, migrations applied with
  `go run ./cmd/test-migrate`, then `go test -p 1 -count=1 ./...` over the
  whole service: 78 packages pass; the two failures in that run were
  assertions my change had to update (workforce route count 37 -> 46 and
  the Section 17 OpenAPI document). Both were fixed and re-run green
  (`internal/features/workforce`, `internal/section17contract`).
- New tests: `TestOrganizationStructureAndAccessAdministration` (production
  router), `TestManagedProviderAdministration`,
  `TestShiftTemplatesAndTrainingEnrollments`, extended
  `TestSection17And18EndToEnd`; migration replay of 000089 in
  `section30contract`; route classification in `section28contract`.
- `go build ./...`, `go vet ./...` clean.

Not run: `-race` (not part of this session's runs), the browser suites, and
any live staging journey.

## 5. Security notes

- Default-deny permissions on every new route; tenant scope from the session
  only; cross-tenant reads and writes return 404 (tested).
- Privilege escalation through role contents is refused; staff cannot change
  their own roles; owners keep the owner role.
- The provider decline right cannot be disabled (refused in the use case,
  CHECK constraint in the database).
- Provider decline history exposes reason codes, never free text.
- Settings PATCH cannot change operating mode, retention or feature flags.

## 6. Deployment

Pushing `staging` triggers `Deploy staging (automatic)`, which builds and
deploys changed services, including `fleet-partner-service`. Migration
000089 runs on service start where `MIGRATION=true` is set for staging.
The deploy outcome and running image revision must be read from the
workflow run and the server; this report does not claim them. See the
session summary for the run status at the time of writing.

## 7. Remaining blockers

| Blocker | Owner | Done so far | Next action |
|---|---|---|---|
| Dispatch and Taxi resolve-or-provision contracts | dispatch / taxi service teams | Fleet side fails closed; Hauling works | Add internal endpoints in those services, wire in `app/wiring.go` |
| Marketplace offer producers | dispatch / taxi / hauling | Fleet ingestion exists, off by default | Producers + enable `FLEET_MARKETPLACE_EVENT_INGESTION_ENABLED` and allowlist on staging |
| Fleet-Wallet trust on staging | platform / infra | Code complete; staging waiver for activation | Set `FLEET_PARTNER_PAYMENT_WALLET_SERVICE_SECRET` and Wallet trust entry |
| Provider accept/decline routes on staging | platform / infra | Mounted only when `FLEET_MARKETPLACE_ALLOWED_CALLERS` is set | Set the allowlist for the provider services |
| Frontend for new routes | web/fleet | none | Wire branches, departments, settings, custom roles, provider admin, templates and enrolments into existing pages without redesign |
| Remaining tenant APIs | Fleet | listed in matrix | 9.12/9.13, 14.10, 15.3, 16.12/16.14, 17.15, 18.12, 19.13, 20.14 |

## 8. Verdict

Partial progress, honestly scoped: 19 rows closed with tests; Sections 5-20
as a whole remain open. No section is marked CLOSED - VERIFIED, because no
section has every row closed and live-certified on staging.
