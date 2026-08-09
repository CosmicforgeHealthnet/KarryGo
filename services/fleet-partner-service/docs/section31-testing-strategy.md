# Section 31 testing strategy

Section 31 is the release-candidate acceptance gate for the Corporate Logistics backend. It reuses the executable suites delivered in Sections 13–30 and adds test infrastructure only where the final audit found a genuine gap. The machine-readable source of truth is [`section31-test-coverage.json`](section31-test-coverage.json); `internal/section31contract` rejects an omitted mandatory case, a fake test name, stale path, non-mandatory suite, or non-PASS result.

## Test levels and dependencies

| Level | Purpose | Mandatory dependencies |
|---|---|---|
| Unit | State machines, policies, validators, DTO visibility, scheduling, money and error/retry classification | None |
| PostgreSQL integration | Tenant and branch scope, transactions, races, leases, immutable records, inventory, finance, audit, retention and migration replay | PostgreSQL 16 |
| Contract | Internal clients, OpenAPI/AsyncAPI, produced and consumed events, service authentication and provider compatibility | Repository fixtures; PostgreSQL where the contract performs replay |
| End to end | Corporate Fleet plus Dispatch, Hauling, Wallet, Verification, Media, Support and provider-owned execution boundaries | PostgreSQL, Redis and synthetic local service adapters/processes |
| Failure injection | Timeouts, unavailable services, duplicate/delayed messages, stale versions, lost responses, unknown outcomes, rollback, cancellation and lease loss | PostgreSQL and Redis where relevant |
| Security | Revocation, HMAC scopes, rate limits, IDOR, redaction, file authorization, audit chain, legal hold and retention | PostgreSQL and Redis |
| Performance | Repeatable local capacity smoke profiles and query plans | PostgreSQL 16 |
| Migration | Fresh replay, historical upgrade, latest rollback/reapply, policy lint and compatibility fixtures | Disposable PostgreSQL 16 databases |

All data is synthetic. No production credential, personal data, signed URL, unrestricted database dump or secret environment value is a test artifact.

## Mandatory suite topology

The 19 suites are tenancy/authorization; organization onboarding; provider provisioning; acceptance/decline; safety decline; assignment/reassignment; pricing visibility; wallet; orders/shipments; dispatch/routes; fleet/maintenance; compliance/safety/proof; warehouse; finance; events; jobs/leases; security; performance; and migration. Each suite owns an exact mandatory-case list and executable evidence in the JSON matrix. Existing tests remain the primary evidence; Section 31 does not duplicate them under new names.

The final corporate scenario is composed from the real organization, provider, customer/order/shipment/package, marketplace, execution, proof, finance, event, audit, search and export integration suites. Independent and organization-managed provider compatibility is proven by membership-summary golden fixtures, provider-safe serialization, canonical accept/decline/execution tests and both provider Flutter parsers.

## Failure injection and concurrency

Unknown external mutation outcomes are never converted into success. Wallet, execution, Media, Support, restriction and finance tests inject lost/ambiguous responses and require authoritative read-back before any retry. Event tests inject duplicates, lower/same/gapped versions, delayed consumption and unavailable analytics. Worker tests inject cancellation, heartbeat/lease loss, restart and permanent failure.

Real PostgreSQL transactions cover acceptance conflicts, provider capacity, assignment expiry, route publication/replan, warehouse stock changes, invoice/reversal, legal hold/retention, job completion/recovery, scheduled occurrence uniqueness, membership scope, organization closure, export expiry and projection reconciliation. Tests use isolated records and bounded process parallelism; no retry wrapper is permitted around a failing test.

## Determinism and skip policy

Fleet release-candidate tests run with `-parallel 4 -p 1`. Other service suites run with `-p 1`. Mandatory CI provisions PostgreSQL and Redis and then streams `go test -json` through `cmd/section31-testgate`. Any unexpected `Action=skip` fails the job. A narrowly named harness-only skip may be allowlisted only when the parent executable test runs the same scenario; dependency-unavailable skips are never allowed.

## Performance acceptance

Capacity tests record synthetic dataset size, p50/p95 (and p99 where meaningful), throughput, `EXPLAIN` plan evidence, pool pressure and process memory where measurable. The profiles cover provider/tenant-scale lookup, dispatch-board projections, scan-heavy paths, exports, event/outbox backlog and jobs/leasing. Results are development-machine smoke evidence, not a production SLA. Guards are deliberately generous enough to detect a gross query or locking regression without encoding workstation speed as a business promise.

## Migration acceptance

`internal/section30contract` remains the migration authority: it performs empty-schema replay, latest rollback/reapply, multi-step recent-range replay and historical-data upgrade; validates numbering, pairs, destructive operations, backfill metadata, index safety, enum/check evolution and PostgreSQL minimum version; then exercises legacy/provider-app compatibility fixtures. Applied migration files are immutable.

## CI topology

`.github/workflows/section31-mandatory.yml` has five blocking lanes: static/contract, Fleet PostgreSQL+Redis acceptance, all other Go modules, Dispatch/Truck provider Flutter apps, and repository security scans. The acceptance job requires every lane to succeed. Failure output is limited to JSON test output and container status; secrets, environment dumps and database dumps are excluded.

Mandatory gates are formatting of changed Go files, `go vet`, `go build`, Section 31 coverage contract, all tests, PostgreSQL and Redis integration, service contracts, tenant/security, migrations, provider compatibility, failure injection, capacity smoke, private-key/token scan, untracked `.env` guard, sensitive-log scan and `git diff --check`.
