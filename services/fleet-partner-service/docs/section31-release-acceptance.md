# Section 31 release acceptance

This document is the durable checklist used by the final release-candidate run. A checked item means the corresponding executable evidence is mapped in `docs/section31-test-coverage.json` and passed in the recorded run; CI repeats the mandatory subset on every pull request and `staging` push.

## Acceptance checklist

- [x] Functional: organization and provider lifecycle; customer/order/shipment; assignment/execution; fleet; warehouse; finance; support; files; search/import/export.
- [x] Security: tenant and branch scope; object and field authorization; revocation; service authentication; rate limits; redaction; append-only audit; legal hold.
- [x] Reliability: retry classification; unknown-outcome read-back; degraded modes; outbox/inbox; jobs and leases; reconciliation.
- [x] Compatibility: fresh/old-data migrations; independent and organization-managed providers; additive provider DTOs; provider Flutter parsers.
- [x] Operations: liveness/readiness; backup/restore and DR smoke; local performance/capacity profiles.
- [x] Every mandatory Section 31 case maps to at least one executable test and the contract discovers that test in source.
- [x] PostgreSQL 16 and Redis 7 are provisioned by mandatory CI; dependency absence cannot silently skip a suite.
- [x] Fleet uses `-parallel 4 -p 1`; other module suites use bounded package concurrency.
- [x] Failure artifacts exclude credentials, raw signed URLs, environment dumps and unrestricted database dumps.
- [x] Formatting, vet, build, secret/private-key, sensitive-log, untracked `.env` and whitespace gates are blocking.

## Release-candidate commands

From each module, the acceptance run uses the command family below. Fleet is the authoritative Section 31 aggregate:

```text
go test ./... -count=1 -parallel 4 -p 1 -timeout=1200s
go vet ./...
go build ./...
```

Dispatch, Hauling, Wallet, Verification, Media, Support and every other Go module use:

```text
go test ./... -count=1 -p 1 -timeout=600s
go vet ./...
go build ./...
```

The Dispatch and Truck/Hauling provider applications use `flutter test`. Focused mandatory runs additionally execute `internal/section31contract`, real Redis tests, tenant/security selectors, migration replay and rollback/reapply, backup/restore smoke, failure-injection/concurrency selectors, capacity profiles and provider compatibility fixtures.

## Release rule

Release is refused for any test/build/vet/scan failure, any unexpected mandatory skip, any staged file, any unmapped requirement, or any unresolved unknown external outcome. Local capacity observations are diagnostic development-machine measurements and are not stated as production SLA evidence.
