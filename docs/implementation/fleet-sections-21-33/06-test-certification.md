# Fleet Sections 21-33: test certification

## Environment

- Disposable PostgreSQL 18 on `127.0.0.1:55433` (databases `fleet_partner_test`, `dispatch_test`), disposable
  Redis on `127.0.0.1:56379`. The developer's own `localhost:5432` database was never touched.
- Migrations applied with `go run ./cmd/test-migrate` (fleet and dispatch).
- Command: `go test ./... -count=1 -parallel 4 -p 1` in `services/fleet-partner-service`
  (the same shape as the `fleet-mandatory` CI job).

## Result

Full run on 2026-11-26: **98 packages passed, 2 failed, 184 without test files**. The integration package
alone ran 530 s against real PostgreSQL and Redis, including the Dispatch end-to-end test.

The two failures were real and were fixed, then the packages re-run green:

| Package | Failure | Fix |
|---|---|---|
| `internal` | FLEET-5.1b: analytics and governance use cases held `*repositories.Repository` directly | Added `Repository` interfaces in each `usecases` package with a compile-time assertion. |
| `internal/section28contract` | Sensitive-column inventory stale (new `cost_allocations.amount_minor`) | Regenerated, reviewed: classified FINANCIAL, approved plaintext (a business amount, not a payment credential). |

No test was skipped, weakened or deleted to get green. One stub test was removed together with the
stub it tested (`automation/section27_runner_test.go`); its behaviour is now covered by real job tests.

## New tests for Sections 21-33

| Test | Kind | Proves |
|---|---|---|
| `TestSection51WarehouseJourney` | integration (real DB) | Receipt lifecycle, discrepancy blocking, leased tasks, concurrent wave release (WH-003), short pick, packing limit, gate ordering, tenant isolation |
| `TestSection52FinanceAnalysis` | integration | Payroll fingerprint refusal, invoice submit, ageing, profitability snapshot immutability, cost allocation |
| `TestSection53ExceptionEscalationJourney` | integration | Exception dedupe, SLA escalation by the real worker, notification failure, preferences |
| `TestSection24AnalyticsRecommendationsAutomationAndGovernance` | integration | Dashboards and filters, report runs with frozen scope, recommendations decide/expire, automation SoD, exactly-once execution, loop guard, privacy requests with legal hold and SoD, security events |
| `TestAgeingBucketsAndCurrencyIsolation`, `TestProfitabilityMarginAndSplitEven` | unit | Finance arithmetic in exact minor units |
| `TestNarrowChannels` | unit | Mandatory notification channels cannot be suppressed |
| payroll hash and canonical-event unit tests | unit | Hash stability; Section 26.2 event derivation |
| `section24_33contract` | contract | OpenAPI parity for every new route; protected-action catalogue |
| `section28contract` route coverage, `section30contract` tip `000104`, migration replay/rollback | contract | Registries, migration chain, reversibility |

## Not executed here

- `-race`: needs cgo, and this Windows host has no C compiler. CI (Linux) does not run `-race` for fleet either.
- Live end-to-end on staging with real users: see 07 and 09.

---

# Final-closure test evidence (2026-11-27)

## Local (disposable PostgreSQL 18 on 127.0.0.1:55433, Redis on 127.0.0.1:56379)

| Command (from the service directory) | Result |
|---|---|
| fleet: `go test ./... -count=1 -parallel 4 -p 1` | 100 packages pass, 0 fail (integration package 279 s) |
| fleet: `go vet ./...`, gofmt on every changed file, `git diff --check` | clean |
| fleet: `TestSection51WarehouseJourney` with the old PICK guard restored | **fails** as the defect predicts (order-level pick returns 200 and marks the line picked while its task is open), then passes with the fix |
| fleet: `TestSection24AnalyticsRecommendationsAutomationAndGovernance` | pass: report_run.ready.v1 once; session list/revoke: tenant isolation, branch-scope refusal, reason required, idempotent repeat, switched-family session in another organization untouched, one event, one audit entry with the reason |
| fleet: `TestAuditEventsAliasServesTheAuditLogList` | pass: `/audit-events` byte-identical to `/audit-logs`, other tenant excluded, 403 without `audit.read` on both |
| fleet: `TestSection28LegalHold_*` | pass, incl. one applied and one released event |
| fleet: section28/24_33/30 contracts, masterrelease, platform registries, middleware, app | pass; migration replay/rollback through `000105` |
| media-file-service: `go test ./...`, `govulncheck ./...` | pass; 0 vulnerabilities |
| driver-dispatch-delivery-service: `go test ./... -p 1`, `govulncheck ./...` | pass (all 15 websocket tests: auth before upgrade, upgrade, initial frame, forwarding, multiple clients, offline, disconnect/reconnect, admin); 0 vulnerabilities |
| driver-hauling-service: `go test ./... -p 1`, `govulncheck ./...`, `TestTrackSocket_*` (x3) | pass; 0 vulnerabilities; new socket test: 401/403/404 before upgrade, 101 upgrade, offline first frame, relay of a published fix, close, reconnect, unsubscribe |
| all 21 Go modules: `govulncheck ./...` (v1.6.0) before the fixes | 3 affected: Dispatch, Hauling (GO-2026-6278), media (GO-2026-6974); fleet and the rest clean |

## CI on the final code commit `befb4c33`

| Workflow | Result | Run |
|---|---|---|
| Section 31 mandatory acceptance (incl. govulncheck over every module and the full fleet suite) | success | `45949687541` |
| Admin bridge Sections 2-5 (incl. Trivy image scan) | success | `45949687535` |
| Customer V4 backend certification (14 gates, 0 skipped; Hauling 149 passed) | success | `45949687656` |
| Deployment safety acceptance | success | `45949687746` |
| Partner Web split / Vendor 46-48 / Admin 16-38 / Admin owner closure / Email branding (Fleet authentication v2 passed on `d5409a21`; its path filter did not trigger on `befb4c33`) | success | `45949687524`, `45949687647`, `45949687521`, `45949687717`, `45949687579` |
| Deploy staging (automatic) | success | `45949687555` |
| Fleet staging capabilities | success | `45949687696` |

On the intermediate commit `d5409a21` every gate except Customer V4 certification was already green; that
certification failed only because the new Hauling socket test skipped (the gate counts a skip as a
failure). `befb4c33` points the test at the Redis the gate provides.

## Still not executed

- `-race` (no C compiler on this Windows host; CI does not run it for these modules either).
- Authenticated live journeys on staging (see 07 and 09).
