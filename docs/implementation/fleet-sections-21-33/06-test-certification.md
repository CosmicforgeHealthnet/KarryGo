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
