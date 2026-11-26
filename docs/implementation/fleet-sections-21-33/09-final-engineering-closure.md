# Fleet Sections 21-33: final engineering closure

Independent re-audit performed after implementation, against the code on `staging`, not against the
plan. Every count below was re-derived from the repository.

## Exact counts

| Item | Count |
|---|---|
| Commits on `staging` for this work | 6 (`a1fc7371`, `0c1cbab2`, `421b70a2`, `19607d23`, `ae9c641e`, `dc1569ed`) |
| New tenant routes | 68 (warehouse 24, finance 7, exceptions/SLA 8, preferences 3, analytics 20, governance 6) |
| New migrations | 5 (`000100`-`000104`), all reversible, head `000104_privacy_requests` |
| New tables | 22 (7 + 2 + 3 + 8 + 2), plus 1 new column on payroll periods |
| New permissions | 28 (7 + 4 + 5 + 9 + 3) |
| New durable job kinds | 5 (`REPORT_RUN_GENERATE`, `AUTOMATION_EVALUATE`, `AUTOMATION_RULE_EXECUTE`, `ANALYTICS_HOUSEKEEPING`, `SECTION23_EXCEPTION_SLA`) plus warehouse lease release |
| Section 26.2 core events now produced | 7 |
| New OpenAPI files | 2 (plus 3 updated) |
| New integration test journeys | 4; new unit tests 5 groups; new contract suite 1 |
| Full fleet suite (local, real PostgreSQL + Redis) | 100 packages with tests: all pass after 2 fixes (see 06) |
| Full fleet suite (CI `fleet-mandatory`, `ae9c641e` and `dc1569ed`) | pass |
| Staging revision at closure | `dc1569ed`, api and worker healthy, migration head `000104` (via readiness gate) |

## Remaining limitations (honest list)

1. **Short pick** is recorded and holds the order, but the shortfall is not replanned automatically.
2. **Admin session management** (`/security/sessions`) is not built; it is a platform-admin surface.
3. **Audit events** are served by the existing `/audit-logs`; no `/audit-events` alias.
4. **Recommendations are rules, not machine learning**: explainable and versioned, as the spec allows.
5. **New features are off on staging** until the owner turns their switches on.

## Owner actions (cannot be done from code)

| # | Action | Why it is needed |
|---|---|---|
| A1 | Turn on the Section 21-28 switches in the staging fleet settings file (`OPERATIONAL_EXCEPTIONS_ENABLED`, `NOTIFICATION_PREFERENCES_ENABLED`, `ANALYTICS_REPORTS_ENABLED`, `FEATURE_AI_RECOMMENDATIONS`, `FEATURE_AUTOMATION`, `SECTION24_WORKERS_ENABLED`, `PRIVACY_REQUESTS_ENABLED`, `SECURITY_EVENTS_ENABLED`, plus the existing warehouse/fulfilment/yard flags) | Settings files are server-side and owner-controlled. |
| A2 | Run a live staging walk-through with two real staff accounts (one to propose, one to approve) | Separation-of-duties flows cannot be exercised by one person; live certification needs real users. |
| A3 | Patch the shared runtime image: `services/Dockerfile` uses `alpine:3.22` with libssl3/libcrypto3 `3.5.9-r0`; Trivy now reports HIGH CVE-2026-124709 (fixed in `3.5.9-r2`). This fails the *Admin bridge* scan and affects every image built from that Dockerfile, fleet included. | A shared-Dockerfile change redeploys every service on staging; not done unasked within a fleet-only mission. |

| A4 | Upgrade `github.com/gorilla/websocket` in driver-dispatch-delivery-service (and hauling, which pins the same v1.5.3): govulncheck now reports GO-2026-6278, published after the last green run on 2026-11-20. The only fixed version is an unreleased upstream commit (`v1.5.4-0.20240701034025-d67f41855da4`). | It fails the *govulncheck* step of `static-and-contract`, so *Section 31 mandatory acceptance* is red. Dispatch is outside this mission and swapping its live-tracking websocket library for an unreleased commit needs that service owner's review. |

## Release gates (Section 33) on `dc1569ed`

| Workflow | Result | Cause if failing |
|---|---|---|
| Deploy staging (automatic) | success | |
| Section 31 mandatory acceptance: `fleet-mandatory` (full fleet suite) | success | |
| Section 31 mandatory acceptance: `static-and-contract` | **failure** | govulncheck GO-2026-6278 in Dispatch (A4). The formatting failure on `ae9c641e` is fixed in `dc1569ed`. |
| Admin bridge Sections 2-5 | **failure** | Trivy HIGH CVE-2026-124709 in the shared runtime image (A3) |
| Deployment safety, Fleet auth v2, Vendor 46-48, Customer V4, Admin 16-38, Admin owner closure, Partner Web split, Email branding | success | |

The two red gates are new vulnerability-database findings in code this work did not change. No gate was
weakened or skipped, and no allowlist was added to get either gate green.

## Verdicts

These are three separate questions and are answered separately.

| Verdict | Answer | Reason |
|---|---|---|
| **SECTIONS_21_33_ENGINEERING_COMPLETE** | **YES, with limitations 1-4 above** | Every in-scope requirement is implemented and tested except the named partial (1) and not-built item (2). |
| **SECTIONS_21_33_STAGING_INTEGRATION_COMPLETE** | **NO** | The code is deployed to staging and healthy at migration head `000104`, but the features are switched off there and have not been exercised on staging (A1, A2). |
| **SECTIONS_21_33_LIVE_E2E_CERTIFIED** | **NO** | No live end-to-end run with real users has happened (A2). |
