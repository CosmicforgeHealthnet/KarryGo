# 03 — Vendor Service Module Map

## Actual current structure

### Section 7 implementation map

Section 7 extends the business-root package with `onboarding_models.go`, `onboarding_service.go`, and `onboarding_http.go`. Cross-service ports are defined at the feature boundary; concrete HMAC clients live under `internal/integrations/{verification,wallet,media}` and composition-root adapters live in `cmd/adapters.go`. Decisions, bank records and file bytes stay at their owning services.

```text
services/vendor-service/
  cmd/
    main.go                       # serviceapp.Run bootstrap; no feature routes registered yet
  internal/
    architecture_test.go          # cross-feature http/repositories import boundary guardrail
    config/
      config.go                   # env config: db, redis, service secrets, all feature flags
      config_test.go
    database/
      postgres.go                 # pgxpool
      migrate.go                  # self-contained runner: apply/rollback/current, query-driven
      migrate_test.go             # real-Postgres gated (VENDOR_SERVICE_TEST_DATABASE_URL)
    app/                          # reserved: route-assembly composition root, empty until
                                   # the first feature registers routes
    platform/
      idempotency/
        idempotency.go            # service-local Idempotency-Key store
    audit/
      audit.go                    # Recorder interface + NoopRecorder; no schema yet
    events/
      envelope.go                 # event envelope (event id, type, aggregate, correlation/causation)
      outbox.go                   # transactional outbox: append/claim/lease/backoff/dead-letter/replay
      inbox.go                    # inbound-event dedupe
      http_publisher.go           # signed HTTP delivery to one destination
      worker.go                   # outbox delivery loop
    jobs/
      runner.go                   # ticker-driven background job convention
    integrations/                 # one reserved package per external authority (doc.go only)
      wallet/ verification/ media/ notification/ support/ admin/ analytics/
      dispatch/ taxi/ hauling/ handover/
    features/                     # one reserved package per business feature (doc.go only)
      auth/ vendors/ capabilities/ brands/ branches/ staff/ crm/
      food/ cloud_kitchen/ pastry/ shop/ catalog/
      inventory/ purchasing/ recipes/ production/
      orders/ substitutions/ promotions/
      delivery/ fulfilment/
      finance_projection/ analytics/
  migrations/
    000001_vendor_service_foundation.{up,down}.sql
    000002_events_outbox_inbox.{up,down}.sql
    000003_idempotency_keys.{up,down}.sql
  docs/
    README.md, UI_UX_REQUIREMENTS.md, FRONTEND_REQUIREMENTS.md,
    PLATFORM_SERVICE_CHANGE_REGISTER.md, API_ROUTE_REGISTER.md, IMPLEMENTATION_STATUS.md
    architecture/  (this folder)
    openapi/       (empty; populated once real routes exist)
```

This matches the specification's requested layout with one deliberate naming difference: the specification's example tree nests `internal/database/` under `internal/app/` implicitly and lists `audit/`/`events/`/`jobs/` at the `internal/` top level directly, which is exactly what was built — `internal/app/` is reserved specifically for the future route-composition root (mirroring how `cmd/main.go` in every other service wires repositories → usecases → HTTP handlers → `serviceapp.Run`'s `Register` callback), not yet populated because no feature has routes to compose.

## Standard per-feature layout (once a feature is implemented)

Every `internal/features/<feature>/` package follows the repository-wide convention already established by every real service (see CLAUDE.md, "Standard Go Service Layout"):

```text
internal/features/<feature>/
  http/            # DTOs, handlers, route registration — thin, no business rules
  usecases/        # business workflow; defines port interfaces for cross-feature/cross-service calls
  models/          # domain rules and pure validation
  repositories/    # persistence interfaces + Postgres implementation
  clients/         # adapters over internal/integrations/<service> for this feature's specific calls
```

## Section 6 implementation map

Section 6 uses `internal/features/vendors/{models,validation,service,http}.go` as the business-root aggregate. `cmd/main.go` supplies authorization, audit, outbox, idempotency, and feature flags. Staff IAM consumes only the narrow `BranchScopeValidator` port, implemented by the vendor aggregate, so staff does not import vendor repositories or HTTP code.

## Cross-feature dependency rule

No `internal/features/X` package may import `internal/features/Y/http` or `internal/features/Y/repositories` for a different Y. A feature needing another feature's data defines a narrow port interface in its own `usecases` package (e.g. `orders/usecases.CatalogPort`), with the real adapter wired in `internal/app` — the same pattern proven at scale in `fleet-partner-service` (`orders/usecases.CRMPort`/`.CatalogPort`, adapters in `internal/app/orders_adapters.go`) and enforced there by a real Go-level test. Vendor Service adopts the identical rule and the identical enforcement mechanism from day one: `internal/architecture_test.go`'s `TestFeaturesDoNotImportOtherFeatureHTTPOrRepositories` statically scans every file under `internal/features` for a forbidden cross-feature import. Importing another feature's `models` package is fine; only `http` and `repositories` are restricted.
