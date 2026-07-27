# Architecture

## Deployment position

```
Customer App / Provider Apps / Corporate Web
                     |
                 API Gateway
                     |
          fleet-partner-service
             |       |       |
             |       |       +--> analytics-service
             |       +----------> payment-wallet-service
             +------------------> dispatch / taxi / hauling services
             +------------------> notification / media / verification / support
```

## Runtime components

1. **HTTP API process** (`cmd/main.go`): gin router assembled in
   `internal/app`, graceful shutdown, optional in-process workers
   (`FLEET_PARTNER_WORKER_ENABLED`).
2. **Worker process** (`cmd/worker/main.go`): durable-job worker + outbox
   publisher without HTTP, for deployments that separate the concerns.
3. **Dedicated PostgreSQL** (`internal/database`): pgx pool with explicit
   limits, startup ping, transaction helper, migration runner with a
   `fleet_partner_schema_migrations` tracking table. The pool is built from
   `FLEET_PARTNER_DATABASE_URL` only — this service never opens a connection
   to another service's database, and no shared helper is allowed to do so on
   its behalf (see [platform-boundaries.md](platform-boundaries.md)).
4. **Redis** (`internal/config` + shared `redisx`): optional, bounded
   operational use only (caching, locks, counters — always with TTL); never a
   source of truth; degraded mode when unavailable and not required.
5. **Transactional outbox / consumer inbox / durable jobs** — see
   [events-and-jobs.md](events-and-jobs.md).
6. **Platform service clients** (`internal/platformclients`): typed
   interfaces, signed requests, explicit timeouts, error classification
   (retry only on classified transient failures), metrics instrumentation,
   explicit not-configured adapters. Remote availability is never a liveness
   requirement.
7. **Observability** (`internal/observability`): dependency-free metrics
   registry rendered at protected `/metrics`; structured JSON request logs
   with redaction. Request and correlation IDs propagate everywhere;
   downstream calls carry `X-Request-ID` and `X-Correlation-ID`. Circuit
   breaker state is observable through per-outcome client metrics
   (`fleet_partner_client_calls_total{outcome=...}`).

## Dependency direction (enforced)

```
HTTP handler → use-case interface → repository / service-client interfaces → infrastructure adapters
```

- Handlers depend on use-case interfaces; adapters are wired in `cmd`/`app`.
- Domain models import no SQL drivers, HTTP routers or SDKs.
- One feature never imports another feature's `http` or `repositories`
  package; collaboration uses interfaces or domain events.
- Fleet-specific tenant/organization/projection logic stays in this service —
  it is not moved into `shared/go`.

All of the above are covered by `internal/architecture_test.go`.

## Bootstrap divergence from `serviceapp.Run`

Existing services use `shared/go/serviceapp.Run`. This service assembles its
own bootstrap because the shared one cannot express: a protected `/metrics`
endpoint, database/migration readiness, worker readiness, configured (rather
than implicit) Redis readiness, and spec route classes. The shared `httpx`
middleware, `auth`, `serviceauth`, `redisx` and `logging` packages are reused.

## Startup sequence

1. Load `.env`, validate configuration — **fail fast**, never printing secrets.
2. Connect dedicated PostgreSQL (ping) — fail fast.
3. Apply migrations when `MIGRATION=true`.
4. Connect Redis if configured (fatal only when `FLEET_PARTNER_REDIS_REQUIRED`).
5. Build metrics, platform clients, workers (optional), router.
6. Serve; on SIGINT/SIGTERM stop accepting requests, drain HTTP within
   `FLEET_PARTNER_SHUTDOWN_TIMEOUT`, stop worker claim loops, wait for
   workers, close pools.
