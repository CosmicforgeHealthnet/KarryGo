# 03 — Vendor Service Module Map

## Section 8H module map

Section 8H lives in `internal/features/crm` alongside the CRM it extends: `intelligence.go` (model registry and the two pure scoring functions), `intelligence_eval.go` (durable evaluations, the materializer, generation swap, staleness), `intelligence_read.go` (scores, Customer 360 block, analytics), `intelligence_segments.go` (the eight Section 8B fields), `intelligence_privacy.go` (the privacy handler), `intelligence_http.go`, plus `exports.go`, `export_datasets.go`, `exports_service.go`, `exports_worker.go` and `exports_http.go`. The private artifact adapter is `internal/integrations/media/artifacts.go`; the contract package is `internal/section8hcontract`.

**The models are pure functions.** `computeRFM` and `computeChurn` take a value and return a value, so determinism, tie behaviour, band boundaries and the new-customer rule are all unit-testable without a database — which is the point of computing scores in Go rather than in a SQL expression nobody can call twice with the same inputs.

**Two ports, both fail-closed.** `ArtifactStore` and `StepUpConsumer` are interfaces on the service. A nil `ArtifactStore` refuses export creation rather than falling back to a local file; a nil `StepUpConsumer` refuses the guarded operations rather than skipping the check, because an unenforced step-up looks like protection in every diagram while providing none.

**The Section 8A privacy seam was used again, not widened.** `intelligencePrivacyHandler` registers on the same `PrivacyHandler` registry Sections 8B, 8C and 8G use. That is the fourth section to extend Section 8A through its ports rather than by editing it.

**The Section 8B registry was extended, not forked.** Score predicates are compile functions in the existing closed registry. A separate score-segment mechanism would have been a second place for branch scope, privacy exclusion and unavailable-is-not-zero to be implemented — and therefore a second place for them to be implemented differently.

## Section 8G module map

Section 8G lives entirely in `internal/features/crm`, alongside the CRM it extends: `corporate.go` (accounts, lifecycle, branch scope, code allocation, the account fence), `corporate_sites.go` (locations and cost centres), `corporate_contacts.go` (contacts and memberships), `corporate_activity.go` (the Section 8C facade), `corporate_privacy.go` (the privacy/merge handler) and `corporate_http.go` (41 routes). Migrations `000022`-`000023`; contract package `internal/section8gcontract`.

**Section 8C was extended additively rather than copied.** `crm_interactions` and `crm_tasks` now carry a nullable corporate subject under a `CHECK` that permits exactly one subject per row. The Go structs kept `string` fields with SQL `COALESCE`, so no accepted Section 8C call site changed — the alternative, a parallel set of corporate activity tables, would have been two places for the same state machine and the same definition of immutable history to drift apart.

**The Section 8A privacy seam was used again, not widened.** `corporateContactPrivacyHandler` registers on the same `PrivacyHandler` registry Sections 8B and 8C use, so redaction and identity merge reach corporate contacts without `ApplyPrivacy` being touched. That is the third section to extend Section 8A through its ports rather than by editing it, which is the property the port existed to buy.

**No new worker, no new port.** Section 8G has no durable background job: archiving an account cancels its open tasks inside the same transaction, so there is nothing to reconcile later.

## Section 8E modules

Section 8E lives inside `internal/features/crm` alongside Sections 8A-8D rather than in a new feature package, because it composes their types directly: it reads segment membership, evaluates consent through `MarketingEligibility`, and reads Section 8C tasks and interactions for the service-recovery signal. Splitting it out would have meant either duplicating those rules or importing across a feature boundary the architecture test forbids.

- `campaigns.go` — the campaign aggregate, lifecycle, definition validation, immutable versioning, and the audience-source scope rules.
- `campaign_experiments.go` — deterministic cohort assignment. Pure and database-free, so it is unit-tested without Postgres and reproducible by an analyst months later.
- `campaign_audience.go` — the funnel: candidate scan, classification, snapshot-time consent evaluation, preview, the durable preparation worker, the atomic promotion, and the snapshot/target reads.
- `campaign_privacy.go` — the `PrivacyHandler` registration and the `GenerationRetainer` implementation.
- `campaigns_http.go` — the ten public routes, reusing the Section 8B command edge (the same Idempotency-Key store, actor resolution and rate limiter) rather than introducing a fourth convention.

**The Section 8B extension point Section 8E was written for.** `GenerationRetainer` was added in Section 8B with an explicit comment that Section 8E would need it. It is now implemented: a live campaign snapshot pins the exact segment generation it depends on, so segment cleanup cannot release membership out from under a preparation. Using the port as designed also required a small correctness fix in `segment_eval.go` — the retention check and the release now happen under the evaluation row's lock — because without it the two could each conclude the other was not there.

## Section 8C modules

`interactions.go` owns interaction commands/queries and revision history; `tasks.go` owns task queries, assignment, lifecycle, and history; `activity_privacy.go` supplies the Section 8A privacy/merge handlers; `activity_http.go` mounts the 16 public operations. `customer360.go` composes bounded activity summaries. The implementation remains in the existing CRM bounded context and uses the established authorization, idempotency, audit, outbox, feature-flag, and profile-row-fence infrastructure.

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

## Section 8A — `internal/features/crm`

Implemented as a flat feature package following the Section 6/7 `vendors` convention: `models.go` (pure domain rules and invariants), `service.go` (tenancy- and scope-safe reads, lifecycle commands, shared transactional helpers), `consent.go`, `privacy.go` (redaction engine plus the handler registry), `commercial.go` (order-fact projection and rebuild), `identity.go` (link/merge), `ingestion.go` (profile resolution, anti-resurrection, inbox claim), `customer360.go` (composed read model), `events.go` (event registry, authority checks, handlers), `http.go` and `internal_http.go`.

Cross-feature dependencies go through ports, per the boundary rule enforced by `internal/architecture_test.go`: `ScopeResolver` (over the Section 5 authorization evaluator), `AuditRecorder` and `EventPublisher`, each adapted in `cmd/adapters.go`. The CRM package imports no other feature's `http` or `repositories`.

Two extension points exist so later CRM packages do not require rewrites: `PrivacyRegistry` (8B–8H register their own redaction handlers) and the modular Customer 360 composition (each block comes from its own reader, rather than one service importing every future repository).

## Section 8B — segmentation inside `internal/features/crm`

Section 8B extends the same flat CRM package rather than opening a second one: tags, segments and audiences are the same bounded context as relationships, consent and privacy, and splitting them would have forced a port between two halves of one domain.

Files: `tags.go` (tag domain, naming rules, bulk membership), `segment_rules.go` (the rule AST, validation, canonicalisation, hashing, deterministic explanations), `segment_registry.go` (the closed field/operator registry and the per-field SQL compilers), `segments.go` (definitions, immutable versions, scope rules), `segment_membership.go` (static membership, manual exclusions, member reads), `segment_eval.go` (preview, materialization, the worker, invalidation), `segment_privacy.go` (the Section 8A privacy/merge handlers for the two new domains), `segments_http.go`.

**The registry is the architectural boundary that matters here.** Vendor input reaches the database only through `SegmentFieldRegistry`, whose every field carries a hand-written Go compile function. Nothing derives a SQL identifier from input, and values are always bind parameters. A new field is a code change with a test, not a configuration entry — which is the point: the set of things a vendor can ask about is a deliberate, reviewable list.

**Both Section 8A extension points were used as designed.** Section 8B registered tag/segment handlers and a Customer 360 composer; Section 8C now registers interaction/task handlers and composes bounded recent activity through the same seams. Sections 8D-8H can follow that pattern without replacing the privacy dispatcher or Customer 360 endpoint.

A third seam was added for the section after next: `GenerationRetainer` decides whether a superseded membership generation may be cleaned up. Section 8B's implementation always allows it; Section 8E implements it so a campaign can pin the generation it launched against.

## Section 9A — `features/food` is no longer reserved

`internal/features/food` now carries the Restaurant/Fast-Food catalogue:

```
features/food/
  doc.go          # scope, and the 9A/9B/9C boundary it must not cross
  models.go       # domain types, response projections, bounded validation
  service.go      # ports, flags, errors, ownership loaders, ordering primitives
  menus.go        # menus and menu sections
  items.go        # items, item media, variants
  modifiers.go    # modifier groups, modifiers, item attachments
  http.go         # handler, command edge, route registration
```

It follows the same shape as `features/crm`: one package, small files by
aggregate, no repository/usecase split, ports for everything outside the feature
(`ScopeResolver`, `AuditRecorder`, `EventPublisher`, `MediaAuthority`) with the
real adapters wired in `cmd/adapters.go`. It imports no other feature's `http`
or `repositories` package, which `internal/architecture_test.go` enforces.

`cloud_kitchen/` and `pastry/` are implemented by Sections 10/11. Section 12A
implements `shop/` independently of `food/`; shared business ownership never
implies shared catalogue authority. `catalog/` remains a boundary marker, not a
cross-domain repository abstraction.

`internal/features/shop` is split by aggregate and commercial responsibility:

```
features/shop/
  models.go           # controlled vocabularies and domain records
  service.go          # IAM, capability, audit/outbox and revision primitives
  operatingmodels.go  # branch operating-model configuration
  categories.go       # bounded hierarchical taxonomy
  measurements.go     # definitions, aliases and product/branch calibration
  products.go         # products, variants, identifiers and packaging
  sellunits.go        # fractions, exact/calibrated conversions and MOQ
  listings.go         # branch listings, price precedence/history and tiers
  fresh.go            # controlled product-applicable fresh attributes
  views.go            # transport projections
  http.go             # authenticated, idempotent management routes
  inventory*.go       # stock, lots, reservations, operations and projections
  inventory_http.go   # authenticated management and internal HMAC routes
  inventory_bulk*.go  # validated bulk rows, leases, retries and dead-lettering
```

List hydration is page-batched for product media/models, variant handling
flags and fresh options. Platform-measure aliases are tenant-scoped, while a
branch calibration may only be attached at the branch listing sell-unit edge;
the business-wide sell unit never absorbs branch-specific conversion truth.

Section 12B adds inventory authority inside `features/shop` because stock bases
are defined by Shop listings/variants and share one transaction/revision
boundary with their availability projection. It does not import Food, Cloud
Kitchen or Pastry repositories. Public inventory management and HMAC-v2
discovery/reservation transports call the same scoped service methods; neither
transport owns business rules. Order, Wallet and logistics execution remain
outside the package.
