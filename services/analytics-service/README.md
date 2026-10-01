# Analytics Service

Dispatch route-integrity metrics use the existing `POST /internal/analytics/v1/commerce-events` endpoint. The consumer accepts only the four approved route/alert event names and a minimized schema; the authenticated caller must match the event source. Migration `002_dispatch_route_metrics.sql` adds a daily aggregate view over the existing deduplicated event store, not another event bus or operational authority.

Deploy the consumer/migration and configure the existing Dispatch caller secret/allowlist before enabling `DISPATCH_ANALYTICS_EVENT_DELIVERY_ENABLED` in Dispatch. Delivery remains asynchronous and disabled by default; no GPS request depends on Analytics availability. Current runtime PostgreSQL certification is blocked by missing canonical test DB variables. See the [Dispatch closure report](../driver-dispatch-delivery-service/docs/reliability-integration-gap-closure.md) for exact tests, privacy limits and rollback guidance.

Analytics Service owns cross-platform analytics ingestion, operational reporting read models, reliability calculation snapshots and privacy-safe marketplace aggregates.

Commerce analytics are downstream of authoritative operational services. They do not accept orders, mutate inventory, decide refunds, assign missions, adjudicate support cases or move Wallet ledger funds.

Section 39 endpoints:

- `POST /internal/analytics/v1/commerce-events`: HMAC v2 internal ingestion for committed commerce events.
- `GET /api/v1/analytics/commerce/vendor-reports`: vendor operational decision-support report with explicit window and data-as-of.
- `GET /api/v1/analytics/commerce/vendor-reliability`: versioned reliability score. This is not verification status, trust status, star rating or popularity.

Order Guarantee is modeled only as future-disabled evidence readiness. There is no public guarantee claim route and no Wallet compensation path.

## Abuse protection

Vendor report and reliability reads use the shared atomic Redis rate limiter
after HMAC caller and business-scope authorization. Internal event ingestion is
not placed in the public dashboard bucket. Configure Redis with
`ANALYTICS_REDIS_ADDR`, `ANALYTICS_REDIS_PASSWORD` and `ANALYTICS_REDIS_DB`;
configure the operation policy with
`ANALYTICS_READ_RATE_LIMIT_REDIS_PREFIX`, `ANALYTICS_READ_RATE_MAX` and
`ANALYTICS_READ_RATE_WINDOW_SECONDS`. The conservative technical default is 60
authorized reads per caller/business/branch per 60 seconds and returns HTTP
429 when exceeded.

## Network dashboards and ADMIN_NETWORK_PUBLIC_UI_DEPENDENCY

The Section 13 dashboards live under `/api/v1/analytics/network/*`:
`mission-success`, `mission-outcomes` (per authority and product: committed,
assigned, completed, cancelled, recovered, recovery failed, guarantee
breaches), `recovery`, `capability-mismatches`, `declines`, `demand`,
`demand-forecast`, `supply`, `provider-quality/:authority/:providerId` and
`readiness/branches/:branchId`. Every one of them requires HMAC v2 service
auth from exactly `admin-backoffice-service` (`ANALYTICS_ADMIN_CALLER`).
There is no end-user bearer path, and no Admin authentication platform was
built for this section.

The API gateway deliberately does not route analytics-service
(`api-gateway/internal/features/routing/models/routes.go`), and
`TestAnalyticsIsNeverReachableAtTheEdge` pins that every analytics path,
admin and internal alike, is a 404 at the edge.

**ADMIN_NETWORK_PUBLIC_UI_DEPENDENCY (open).** `admin-backoffice-service` is
still a bare scaffold: it has no operator authentication, no RBAC and no
client for these dashboards. Until it gains an authenticated operator
session and signs its calls to this service, the dashboards can only be
read by a service holding the admin HMAC secret (for example from
operations tooling). Exposing them to an operator UI is Admin work, not
Analytics work: the backoffice authenticates the human, then calls these
routes as itself. Nothing here needs to change for that.

Supply and demand are published by Taxi, Dispatch and Haulage as
counts per authority, market, zone and product (`shared/go/networkanalytics`),
never with a provider, vehicle or customer identity or an exact position.
An unknown count is stored as NULL and reported as `UNKNOWN`, never as zero.
The demand forecast (`same_slot_mean_v1`) averages the same 15-minute slot
over the previous four weeks, carries `data_as_of`, method and model version,
and stays neutral (`INSUFFICIENT_DATA`, not available) with fewer than three
observed slots.
