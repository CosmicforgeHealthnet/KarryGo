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

## Not done in this session

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
