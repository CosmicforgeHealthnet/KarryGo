# Fleet Sections 5-20: implementation log

Branch `staging`. Newest last. Each package was audited against current code
first (not only against the earlier audit), then built on the existing
repositories and use cases, then tested against PostgreSQL 16.

## Session baseline (2026-11-13)

- `staging` at `fc9c814`. Spec: v2.0 docx (Sections 5-20 incl. Section 20 at
  spec lines 1159-1191 and Part 2 Section 50; Section 20 was available, not a
  source blocker).
- Full `fleet-partner-service` suite against a fresh PostgreSQL 16 database:
  all packages pass once migrations are applied (`go run ./cmd/test-migrate`;
  without that step `internal/app` and `managedmembers` fail on missing
  tables, which is test setup, not a defect).
- Re-checked against code before building: staff detail and invitation
  management, Head Office branch and ADR-006 activation had already landed
  after the audit; their rows were updated accordingly.

## Package A/B: organization structure and access administration (`c39e5bd`)

Built: branches, departments, organization settings, staff edit, custom
roles, branch/department-scoped and expiring role bindings (35 routes in
total across this log; see the API audit).

Decisions:
- Reused every existing repository; added only `BranchRepository.Update`,
  `CountActive`, and department `List/Update/Archive`.
- Settings PATCH changes only assignment timeout, provider visibility and
  approval thresholds. Operating mode, retention and feature flags keep their
  own governed routes, so this route cannot bypass them.
- Escalation guard applies to the contents of a role, not only to grants:
  otherwise "create a role with everything, bind it to a colleague" escalates.
- Staff cannot change their own roles; an owner always keeps the owner role
  (ownership only changes through a transfer).
- A role still bound to anyone cannot be archived; the last active branch
  cannot be deactivated or archived; a branch with departments, or a
  department with children, cannot be archived.
- Structure and access writes are allowed before activation (spec 8 stages 5
  and 8 are pre-activation, like staff invitations).

Tests: `TestOrganizationStructureAndAccessAdministration` (production router,
two companies, cross-tenant 404s, audit rows), section 28 contract tests.

## Package C: managed-provider administration (`1b3d869`)

Built: PATCH, reactivate, remove, capabilities, policies, declines.
Repaired: the safety-decline reconciler ignored marketplace declines and
could wedge on a vehicle Fleet does not manage.

Decisions:
- Remove is refused while the provider holds live marketplace or private work.
  ACTIVE/SUSPENDED go through ENDING to REMOVED; invited/pending memberships
  are CANCELLED. The provider's platform identity is never touched.
- Reactivation re-checks activation gates except "pending setup".
- Capabilities cannot widen the provider's service category.
- A policy that tries to disable the decline right is refused (not silently
  corrected); the DB CHECK remains the last line.
- Decline history returns reason codes only; free text stays with safety review.

Tests: `TestManagedProviderAdministration`, extended
`TestSection17And18EndToEnd` (marketplace unsafe-vehicle decline blocks the
asset, creates inspection + work order; unmanaged vehicle recorded and skipped).

## Section 17: shift templates and training enrolments (`dda4aa0`)

Migration 000089 (reversible). Generation from a template skips local days
whose start or end is in a DST gap and reports them; overnight shifts end the
next day; re-generation never duplicates a day (unique template/day index).
Enrolments are one-open-per-course and complete through the existing course
completion, so evidence and certificate expiry are kept.

Tests: `TestShiftTemplatesAndTrainingEnrollments` (Europe/Rome 2026-03-29
spring-forward), latest-migration replay updated, route catalogue count updated.

## Not done in the first session (superseded: see Phase 2 below)

Listed per row in the matrix. The larger items:
- Dispatch and Taxi provider resolve-or-provision contracts (cross-service).
- Marketplace offer producers in the execution services and staging enablement.
- Approval policies and delegations (9.12, 9.13).
- Leads (14.10), PATCH /orders (15.3), rescue cases and geofence-event read
  (16.12, 16.14), provider self-view (17.15), meter readings/tyres/defects
  (18.12), investigations/proof verification/compliance checks (19.13),
  capability schemas (20.14).
- Frontend: none of the new routes is yet wired into `web/fleet`. The
  existing roles, staff and provider pages are unchanged.
- Fleet-Wallet staging secret (infrastructure owner).


## Phase 2 (2026-11-13 to 2026-11-14): closing Sections 5-20

Everything in the "not done" list above is now built, except the structural
items recorded under Sections 5 and 6 in the matrix. Each line names the
commit; the matrix names the test for every row.

### Deployment detection and permissions
- `deploy/platform/bin/changed-services.py`: select services by imported
  shared packages; `go.work`/`go.work.sum` select nothing. Per-service
  preflight skips (never loosens) unreadable settings files and fails the run
  with the owner action. Pairing drops settings that depend on a blocked
  service (`e1a57b6`, `31dbd64`, `6b5e412`).

### Fleet-owned work
- Approval policies and delegations, cost centres, teams, operating
  locations (migration 000090); approval decisions require step-up and record
  the verified grant (`728590f`, `d111e76`); mandatory step-up on high-risk
  actions with the web client completing it (`513bb60`).
- Provider administration, invitations resend (000094), provisioning
  read-back (000095); provider lifecycle writes require If-Match (`bb2d944`).
- CRM leads (000091); orders lifecycle; rescue cases and asset lifecycle
  (000092); custody subjects, incident investigations, compliance checks
  (000093); per-record cold-chain replay (000096, `6b6b3d3`).
- Branch scope applied by permission (`accesscontrol.RequireScoped`) across
  23 features; NULL-safe branch guards in 11 list queries with a static guard
  (`d4e3a79`).
- Section 6 conventions: list paging validation (`0331419`), unknown query
  parameters refused (`a177684`), cursor pagination for staff/provider
  history/managed lists (`651b08e`), standard envelope (`6bffac1`), UTC
  timestamps (`12d7de8`), If-Match required on structure/staff/role/settings
  writes (`82e1de2`) and corporate assignment cancel (`33f7405`), money never
  through float64 with a static guard (`4639c19`), camelCase response keys
  for 87 structs with a static guard (`aa4cfb9`, web `56cee23`).
- Organization lifecycle: verification suspension is lifted by a later
  VERIFIED decision (`8b76b80`); wallet-gated activation reads the wallet back
  instead of re-provisioning (`45b31f7`).
- Inbox: concurrent duplicate deliveries are duplicates, not insert errors
  (`c164b86`).

### Cross-service (internal Karryam services)
- Fleet <-> Wallet settlement identity: Wallet resolves the canonical
  settlement to the payment the execution service created (Wallet 000023,
  `6940fe3`); Dispatch reports the booking as the payment reference
  (`450891b`); Fleet carries it (000097, `ee0caba`); stuck-job reconciliation
  certified (`9668ae8`). Provider withdrawals read back by idempotency key
  (`1b036af`).
- Dispatch: organization trip cancel `POST /api/v1/internal/marketplace/trips/:tripId/cancel`
  (`5810346`); test harness applies the rider-auth schema (`501909d`).
- Fleet trip cancel `POST /marketplace/trips/:tripId/cancel` through the
  source service with durable command and read-back reconciliation
  (`929e87d`).
- Taxi restriction authority (Taxi 031) with enforcement at eligibility,
  reservation, confirmation and private execution (`43ca5fd`); Fleet
  propagates to Taxi (000098, `f75f86a`).
- Section 11: operating-mode owner grants (000099), accept checks
  expectedVersion, candidates honour branch scope, expiry raises one
  dispatcher alert per assignment (`01c6f11`).

### Frontend (no redesign; existing components and styles)
- Wave 2 (`40ce7b4`): leads, orders, shipments, custody, rescue, asset care,
  approval policies, structure units, incidents, proof, compliance checks,
  provider administration, trip lifecycle.
- Wave 3 (`523a186`, `d1be3cc`): service catalogue, manifests, order imports,
  recurring orders, order execution, asset and provider compliance,
  capabilities, control-tower exceptions, maintenance schedules, sessions,
  approvals inbox and detail. No `TODO: implement` page or mock data remains
  in Sections 5-20 scope (integrations, retention and closure pages belong to
  other sections).

### Tests added (PostgreSQL-backed unless noted)
Row tests for Sections 8-20 (`TestRow8_*` ... `TestRow20_*`), Section 11
marketplace rows, trip cancel (Fleet and Dispatch), Taxi restriction
authority and propagation, wallet-gated activation, role-version cache
invalidation, NULL branch scope, settlement identity (Wallet), and static
guards for branch scope, money and JSON tags. The Sections 14-20 wave alone
found and fixed 26 defects (listed in its commits `57676e4`..`b0d37c8`).

### Final runs
- Fleet `c164b86`: full suite on a fresh database migrated through 000099,
  84 packages ok, 0 failures. Concurrency tests also pass under `-race`.
- Payment-wallet 11, Dispatch 42, Taxi 45, Hauling 37, shared/go 57 packages
  ok on the merged tree.
- web/fleet: `tsc` clean, 43/43 tests, production build, CI lint set.
