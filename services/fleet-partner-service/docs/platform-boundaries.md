# Platform Boundaries

`fleet-partner-service` is an **additive organization control plane**. It
integrates with existing KarryGo microservices and never becomes a competing
source of truth.

## Database architecture (binding rule)

**Every KarryGo microservice owns and uses its own dedicated database** —
`customer-service`, `driver-dispatch-delivery-service`, `driver-taxi-service`,
`driver-hauling-service`, `payment-wallet-service`, `notification-service`,
`media-file-service`, `verification-compliance-service`,
`support-dispute-service`, `analytics-service`, `admin-backoffice-service`,
`api-gateway` and `fleet-partner-service`.

`fleet-partner-service` persists **only** to its own database, reached
through the single connection string `FLEET_PARTNER_DATABASE_URL`. There is
no second business-database URL in this service's configuration, and an
architecture test fails the build if a foreign name such as
`CUSTOMER_DATABASE_URL` or `WALLET_DATABASE_URL` ever appears here.

### `shared/go` is generic utilities only

`shared/go` may hold reusable, domain-free helpers — pool constructors,
transaction and migration helpers, ID generators, request/correlation-ID
middleware, service-authentication helpers, HTTP client utilities, logging.
This service currently reuses `httpx` (request ID, recovery), `auth` (HS256
bearer verification), `serviceauth` (HMAC service signing/verification),
`redisx` (client dial), `apperrors` and `logging`.

**A shared helper is never a route to another service's database.** Any
shared database utility may only ever be handed this service's own
connection string:

```go
pool, err := database.NewPool(ctx, cfg.DatabaseURL /* FLEET_PARTNER_DATABASE_URL */, opts)
```

None of the shared packages this service imports opens a database connection
of its own. Fleet-partner-specific concerns — tenant context, organization
repository filters, corporate audit semantics, projection metadata, feature
flags, migrations, corporate service clients — deliberately stay local and
are **not** promoted to `shared/go`.

## Authority map

| Domain | Authoritative service | This service's responsibility |
| --- | --- | --- |
| Customer identity and profile | `customer-service` | Immutable references + organization-owned external-customer records only |
| Dispatch rider identity and execution | `driver-dispatch-delivery-service` | Provision/link managed riders via approved contracts; consume trip events |
| Taxi driver identity and execution | `driver-taxi-service` | Provision/link managed taxi providers; consume ride lifecycle events |
| Truck/hauling provider identity and execution | `driver-hauling-service` | Provision/link managed hauling providers; consume haulage lifecycle events |
| Payments, wallets, settlements, withdrawals | `payment-wallet-service` | Request org wallets/sub-wallets; store safe projections; **no local wallet ledger** |
| Notifications and realtime delivery | `notification-service` | Notification intents, org templates and preferences |
| Files and evidence | `media-file-service` | File IDs, metadata references, subject ownership, purpose |
| Verification outcomes | `verification-compliance-service` + provider service | Organization-sponsored verification path; consume status changes |
| Support and platform disputes | `support-dispute-service` | Organization context + organization-facing projections |
| Platform analytics | `analytics-service` | Publish corporate events; consume aggregated metrics |
| KarryGo internal administration | `admin-backoffice-service` | Protected internal review APIs/events; **never** in the corporate web API |
| External third-party APIs | `api-gateway` Partner API | Completely separate auth, routes, clients, billing (architecture-tested) |

## Source-of-truth rules

For any record owned by another service, this service stores only
`sourceref.SourceReference` projection metadata:

- owning service name (one of the eleven above);
- immutable external resource ID (opaque; foreign formats never assumed);
- source version and last source event ID where relevant;
- last synchronization timestamp and explicit freshness status.

Hard rules:

- Locally cached display fields are projections only.
- Never authorize an action solely from a stale cached field.
- Never calculate a settlement from a projected balance.
- Never perform a canonical trip transition locally.
- Never locally overwrite a verification result owned by another service.

## No direct database access

This service never imports another service's database package, connects to
another service's database, queries/writes another service's tables, uses
cross-service SQL joins, reuses another service's migrations, or creates
database-level foreign keys to external records.

All cross-service interaction uses authenticated internal APIs
(`internal/platformclients`), versioned durable events (outbox → transport →
inbox), reconciliation jobs, and typed client interfaces.

Enforced by architecture tests in `internal/architecture_test.go`:

- no `cosmicforge/logistics/services/<other>` import anywhere;
- no Partner API/api-gateway import;
- no foreign `*_DATABASE_URL` in configuration;
- migration files contain no dblink/FDW mechanisms (migrations test).

## Why wallet and trip data stay projections

`payment-wallet-service` owns every wallet balance, ledger entry, settlement
and withdrawal; the driver services own every canonical trip transition. A
balance or trip status copied into this database is a **read model with a
timestamp**, not authority:

- a settlement is never computed from a projected balance — the authoritative
  service is asked;
- a canonical trip transition is never written locally — trip events are
  consumed and projected;
- a stale or unsynced projection is surfaced as `STALE`/`UNSYNCED` with its
  `syncedAt`, and out-of-order source versions are detected and dropped
  rather than overwriting newer state;
- reconciliation jobs re-read authority; they never invent missing state.

This keeps a single writer per fact and makes divergence detectable instead
of silently authoritative.

## Pricing deferral

This service may store external pricing references and approved/contract/
booking amount snapshots as currency-safe Money values. It must not calculate
marketplace fares, commissions or subscription charges, and contains no
default or placeholder commercial percentages. Any future pricing interface
method that is not configured returns an explicit unsupported/not-configured
result — never zero.
