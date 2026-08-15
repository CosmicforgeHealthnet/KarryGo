SECTION 8 — FINAL CERTIFICATION ONLY

The Section 8 final implementation is already complete.

DO NOT implement new features.
DO NOT redesign anything.
DO NOT start Section 9.
DO NOT perform another broad architecture audit.

Current expected state:

- branch: staging
- baseline HEAD: 10ba4390eee95a54a76bdafc2520ab2d3325464f
- final Section 8 migration: 000028_vendor_crm_section8_final_completion
- segment registry: 40
- previous route count before final completion: 196
- no frontend work
- no Section 9 work

Authoritative specification:

docs/implementation-specs/section8-final-completion.md

Follow CLAUDE.md.

This task is ONLY to close the remaining FINAL ACCEPTANCE evidence.

============================================================
1. FIRST — SHORT STATE CHECK
============================================================

Derive from production code:

- current Vendor migration tip
- current Vendor route count
- current segment registry count
- git status
- files modified by the Section 8 final completion

Do not modify implementation unless a certification test exposes a real defect.

============================================================
2. GLOBAL API CONTRACT CERTIFICATION
============================================================

Run the accepted Section 8I global drift/contract guard against the actual
production router.

Prove bidirectionally:

production router -> OpenAPI
OpenAPI -> production router
API_ROUTE_REGISTER -> production router

Report:

- final route count
- public route count added by final completion
- internal route count added by final completion
- undocumented routes
- phantom routes

Expected route count may remain 196, but DERIVE it rather than assuming.

Do not weaken contract tests.

============================================================
3. FINAL GROUPED REGRESSION
============================================================

Run the full accepted grouped-regression strategy ONCE.

Use separate/dedicated databases where needed and execute groups serially where
DDL contention is possible.

At minimum certify:

GROUP A
Section 8 final-completion targeted tests

GROUP B
entire internal/features/crm package

GROUP C
all other vendor-service packages

GROUP D
migrations + router/contracts

GROUP E
external service packages ONLY if actually modified

Use real PostgreSQL and Redis for gated tests.

Long output must be redirected to files.

Inspect failures/tails instead of dumping full logs.

Report for every group:

packages
duration where useful
failures
skips

Acceptance requires:

0 failures
0 unintended skips

============================================================
4. CONCURRENCY CERTIFICATION
============================================================

Confirm the deterministic PostgreSQL concurrency tests required by the final
Section 8 implementation actually run and pass, including representative:

- Order correction vs correction
- Order fact vs redaction
- Support fact vs redaction
- RFQ fact vs redaction
- merge vs Order fact
- merge vs Support fact
- merge vs RFQ fact
- Customer Value config replace vs replace
- lapsed evaluation vs fresh Order fact
- trigger evaluation vs task completion
- signal rebuild vs privacy change

Do NOT create sleep-only tests.

If any required race test is genuinely absent, add the minimum deterministic
test necessary, run it, then continue certification.

============================================================
5. PRIVACY / MERGE FINAL CERTIFICATION
============================================================

Prove:

redaction prevents actionable resurrection from late:
- Order facts
- Support facts
- RFQ facts
- signal rebuild
- automatic trigger evaluation

Merge must dedupe/rebuild:
- Orders
- Support
- RFQ
- affinities
- customer value
- signals
- tasks/context

Confirm exports and segmentation exclude redacted relationships.

============================================================
6. SEGMENTATION CERTIFICATION
============================================================

Derive final field registry.

Expected:
40

Verify each newly added final field:

- validates
- has correct type
- handles NOT_AVAILABLE correctly
- respects branch scope
- materializes safely
- invalidates on the correct dependency
- excludes redacted profiles

Unknown must never silently become false/zero.

============================================================
7. CAMPAIGN SAFETY REGRESSION
============================================================

Prove new intelligence cannot bypass campaign consent.

Example:

HIGH_VALUE = true
AT_RISK = true
segment matches
marketing consent/suppression denies

=> campaign handoff remains blocked.

Preserve the accepted 8F/8I handoff transaction architecture.

============================================================
8. LOYALTY REGRESSION
============================================================

Preserve accepted loyalty/tier behavior.

Specifically verify:

- credit
- reversal
- merge
- tier configuration change

Tier standing must still use unreversed qualifying credit entries rather than a
stale lifetime-credit counter.

============================================================
9. MIGRATION CERTIFICATION
============================================================

Run:

fresh migration 000001 -> 000028

upgrade from 000027 -> 000028

rollback 000028

prove previous Section 8 schema remains intact

reapply 000028

Report all results.

============================================================
10. BUILD QUALITY
============================================================

From vendor-service run:

go build ./...
go vet ./...
gofmt -l .
git diff --check

Acceptance:

build PASS
vet PASS
gofmt reports no changed Go file needing formatting
git diff --check PASS

If another service was actually modified, run the appropriate equivalent there.

============================================================
11. ORIGINAL SECTION 8 REQUIREMENT MATRIX
============================================================

Take the ORIGINAL Section 8 requirements and map every requirement to:

- implementation location
- test evidence
- status
- external dependency where applicable

At minimum explicitly certify:

8.1 Customer 360
- order count
- completed orders
- spend
- AOV
- first/last order
- last branch
- frequent categories
- frequent items
- refund/complaint/service recovery
- corporate/group/bulk relationship context

8.2 Tags & Segmentation

8.3 Interactions & Tasks
- Order ref
- RFQ ref
- Support ref
- unresolved complaint trigger
- rejected RFQ trigger
- high-value recovery trigger
- lapsed trigger
- corporate lead trigger

8.4 Loyalty & Customer Value
- loyalty ledger
- loyalty tiers
- customer value bands

8.5 Corporate CRM
- corporate commercial activity
- recurring/bulk
- RFQ history
- quote conversion
- service notes
- task/pipeline integration

8.6 Relationship Signals
- repeat
- lapsed
- high-value
- at-risk
- bulk-capable
- corporate
- recurring

The final matrix must contain:

REAL_GAP = 0

for CRM-owned scope.

External producer absence is NOT a real CRM code gap when the complete consumer
path exists and fails closed as NOT_AVAILABLE.

============================================================
12. DOCUMENTATION RECONCILIATION
============================================================

Ensure these reflect the ACTUAL final state:

README.md
IMPLEMENTATION_STATUS.md
PLATFORM_SERVICE_CHANGE_REGISTER.md
API_ROUTE_REGISTER.md
UI_UX_REQUIREMENTS.md
FRONTEND_REQUIREMENTS.md
relevant OpenAPI
SECTION8_FINAL_COMPLETION_REPORT.md

Do not rewrite unrelated documentation.

============================================================
13. FINAL REPORT
============================================================

Update:

services/vendor-service/docs/SECTION8_FINAL_COMPLETION_REPORT.md

and return a concise final report containing:

SECTION 8 FINAL ACCEPTANCE

OVERALL VERDICT:
GO / NO-GO

SECTION 8 CRM BACKEND IMPLEMENTATION COMPLETE:
YES / NO

FULL LIVE DATA ACTIVATION READY:
YES / NO

FINAL MIGRATION TIP:

FINAL ROUTE COUNT:
New public:
New internal:

FINAL SEGMENT REGISTRY:

REGRESSION:
CRM:
Other Vendor packages:
Migrations:
Contracts:
Concurrency:
Failures:
Unintended skips:

BUILD:
go build:
go vet:
gofmt:
git diff --check:

PRIVACY:
Redaction:
Anti-resurrection:
Merge:
Segments:
Exports:
Campaign safety:

ORIGINAL SECTION 8:
8.1:
8.2:
8.3:
8.4:
8.5:
8.6:

REAL CRM-OWNED GAPS:
NONE / list

EXTERNAL DEPENDENCIES:
[list with AVAILABLE / PARTIAL / NOT_PRESENT]

SECTION 9 STARTED:
NO

FRONTEND TOUCHED:
NO

CONFIRM:
No commit.
No stage.
No merge.
No branch created.
No branch switched.

If and only if all CRM-owned requirements and certification gates pass, report:

SECTION 8 CRM BACKEND: GO
SECTION 8 IMPLEMENTATION COMPLETE: YES

External dependencies may still make:

FULL LIVE DATA ACTIVATION READY: NO

Then STOP.

DO NOT START SECTION 9.