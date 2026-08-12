# Foundation Audit — fleet-partner-service

Audit performed before any code was written, per the Corporate Logistics Backend
Implementation Specification (Sections 3–7 foundation task).

Date: 2026-07-25
Auditor: automated implementation session (Claude Code)

---

## 1. Confirmed repository facts

### 1.1 Git state at start

- Branch: `staging`
- Commit: `7320aaaeb3bb7427b1ffb15c003bc6389d549c1f` ("Add partner OTP signup onboarding with SMTP email")
- Working tree: clean (`git status --short` empty)

### 1.2 Workspace

- `go.work` (go 1.26.3) lists 12 services plus `./shared/go`:
  `admin-backoffice-service`, `analytics-service`, `api-gateway`, `customer-service`,
  `driver-dispatch-delivery-service`, `driver-hauling-service`, `driver-taxi-service`,
  `media-file-service`, `notification-service`, `payment-wallet-service`,
  `support-dispute-service`, `verification-compliance-service`.
- All 12 services referenced by the specification exist under exactly those names.
- `fleet-partner-service` did **not** exist in any form (no directory, no `go.work`
  entry, no stray code). Grep for "fleet" matched only incidental content
  (driver profile vehicle fields, seed data, a README word).

### 1.3 Module naming

- Services: `cosmicforge/logistics/services/<service-name>`
- Shared library: `cosmicforge/logistics/shared/go`

### 1.4 HTTP framework and service bootstrap

- Router: `github.com/gin-gonic/gin` v1.12.0 everywhere.
- `shared/go/serviceapp.Run(Options)` is the common bootstrap: mounts `/health`,
  `/ready`, `<base>/meta`, standard middleware chain
  (`httpx.RequestID`, `httpx.Logger`, `httpx.Recovery`, `httpx.ErrorHandler`),
  graceful shutdown on SIGINT/SIGTERM with 10s timeout.
- `serviceapp.Run` hardcodes: readiness pings Redis whenever `REDIS_ADDR` is set,
  `/ready` dials a **new** Redis client per request, no `/metrics`, no worker
  concept, `/health` and `/ready` response shapes use `{"success":true,...}`.

### 1.5 Configuration pattern

- `internal/config/config.go` per service with `Load()` reading env vars via
  `getEnv`/`getEnvInt`/`getEnvBool` helpers; `godotenv.Load()` in `main`.
- Env names are prefixed with the service, e.g. `CUSTOMER_DATABASE_URL`,
  `CUSTOMER_REDIS_ADDR`; common ones unprefixed: `APP_ENV`, `HTTP_ADDR`, `MIGRATION`.
- Existing services use permissive development defaults (including default
  secrets). This conflicts with the corporate spec's "no permissive
  authentication fallbacks"; see §4 Conflicts.

### 1.6 PostgreSQL pattern

- `github.com/jackc/pgx/v5` + `pgxpool`. `internal/database/postgres.go`
  `NewPool(ctx, url)` with hardcoded pool limits (MaxConns 10 etc.).
- One dedicated database per service (distinct ports/URLs in dev compose).

### 1.7 Redis pattern

- `github.com/redis/go-redis/v9` via `shared/go/redisx.NewClient` (ping-on-create).

### 1.8 Migrations

- Plain SQL files in `migrations/`, applied in lexicographic order by
  `internal/database/migrate.go` when `MIGRATION=true` at startup.
- Two naming conventions coexist:
  - `customer-service`: single files `001_x.sql` (idempotent `IF NOT EXISTS` DDL).
  - `driver-dispatch-delivery-service`: paired `00000N_x.up.sql` / `.down.sql`.
- **No migration tracking table** exists in the customer-service runner
  (migrations must be idempotent). No shared migration runner library.

### 1.9 Authentication

- User auth: `shared/go/auth` — hand-rolled HS256 JWT (`TokenSigner`), claims
  `{sub, role, service, session_id, typ, iat, exp}`; `BearerMiddleware(signer, role, service)`
  enforces token type/role/service and stores claims in gin context.
- Service-to-service auth: `shared/go/serviceauth` — HMAC-SHA256 signed requests
  with `X-Service-Name`, `X-Service-Timestamp`, `X-Service-Signature` headers,
  per-caller secrets parsed from `name=secret,...` env strings, 5-minute skew.

### 1.10 Request ID / logging / error envelope

- `httpx.RequestID()`: `X-Request-ID` header in/out, uuid fallback.
- **No correlation-ID convention exists anywhere.** (New in this service.)
- `shared/go/logging`: printf-style colored console logger (Success/Error/Notice/Fatal).
  No structured JSON logging exists.
- Error envelope (shared `httpx.RespondError`):
  `{"success":false,"error":{"code","message","request_id","fields","details"}}`
  with codes from `shared/go/apperrors`.
- Success responses are ad-hoc `gin.H{"success":true, ...}` per handler; there
  is no shared success envelope helper.

### 1.11 Health / readiness / metrics

- `/health`, `/ready` from `serviceapp.Run` (see 1.4).
- **No `/metrics` endpoint exists in any service.** No prometheus (or any
  metrics library) anywhere in the module graph. `analytics-service` merely
  lists the string "metrics" as a capability.

### 1.12 Events / outbox / inbox

- `shared/go/events` contains only event-name string constants.
- **No event broker, no transport, no transactional outbox, no consumer inbox
  exists anywhere in the repository.**

### 1.13 Background jobs / workers

- **No durable job queue, no lease/heartbeat primitives, and no separate worker
  binaries exist.** All services are a single HTTP process (`cmd/main.go`).
- No `FOR UPDATE SKIP LOCKED` usage anywhere.

### 1.14 Internal service-client conventions

- `shared/go/walletclient`, `shared/go/mediaclient`, `shared/go/notifications`:
  struct clients with `BaseURL`, `ServiceName`, `Secret`, optional `HTTPClient`;
  requests signed with `serviceauth.SignRequest`; empty `BaseURL` treated as
  "not configured" with graceful fallback; tests use `httptest.Server`.
- No retry/circuit-breaker/error-classification layer exists.

### 1.15 Partner API location

- Third-party Partner API lives in `api-gateway`
  (`internal/features/partner`, `partneraccount`, `partnerauth`) with its own
  auth and billing. Completely separate from this service; nothing imported.
- `api-gateway/internal/pkg/pricing` holds marketplace pricing. Not imported here.

### 1.16 Local environment

- Go `1.26.3 windows/amd64` toolchain available.
- `infra/docker-compose.yml` is fully commented out (per-service postgres/redis
  templates on ports 5433+/6380+). No compose stack is running.
- A PostgreSQL server is listening on `localhost:5432` (belongs to an unrelated
  project); ports 5433/6380/6379 are closed. Docker daemon availability could
  not be verified in this session (tool permission denied).

---

## 2. Missing infrastructure (to be created by this task)

- Protected `/metrics` endpoint and any metrics registry.
- Structured (field-based) logging for HTTP requests and workers.
- Correlation-ID convention.
- Transactional outbox, consumer inbox, durable jobs, leases, worker binary.
- Cursor pagination (shared `pagination` is page/limit only).
- Idempotency-key storage, optimistic-concurrency helpers, append-only audit,
  tenant context, tenant-aware repository helpers, Money type, opaque
  prefixed IDs, filter allowlisting, standard success envelope.
- Downstream error classification and retry policy for service clients.

All of the above are implemented **inside** `fleet-partner-service` (not in
`shared/go`) because nothing else consumes them yet (spec §8 binding rules).

---

## 3. Assumptions

1. Corporate web users will receive HS256 access tokens compatible with
   `shared/go/auth` claims, with `role="organization_user"` and
   `service="fleet-partner-service"`. Actual token issuance belongs to the
   future staff/IAM work package; this task implements only verification.
2. Organization-membership resolution is an interface
   (`tenant.MembershipResolver`); no membership table exists yet (staff
   management is out of scope), so the default adapter is explicitly
   unconfigured and fails closed on tenant-scoped routes.
3. Event broker transport is not chosen in the repository; the outbox publisher
   ships with a disabled adapter that returns an explicit configuration error
   (events remain durably PENDING; nothing is discarded).
4. Migration style: this service adopts the `up`/`down` paired convention from
   `driver-dispatch-delivery-service`, plus a service-local
   `fleet_partner_schema_migrations` tracking table (the repo has no tracking
   convention; tracking is required for safe replay/rollback tests).
5. Env vars use the repository prefix style (`FLEET_PARTNER_*`) with common
   unprefixed vars `APP_ENV`, `HTTP_ADDR`, `MIGRATION` — mapped from the
   spec's generic names in `docs/configuration.md`.
6. The corporate API uses the spec's `{data, meta}` / `{error:{...}}`
   envelopes (spec §16.8/§16.9). This intentionally differs from the older
   ad-hoc `{"success":...}` shape used by existing services; documented in
   `docs/api-conventions.md`. Error **codes** reuse `shared/go/apperrors` codes.

---

## 4. Conflicts with existing conventions (and resolutions)

| Conflict | Resolution |
| --- | --- |
| `serviceapp.Run` hardcodes readiness/Redis behavior, has no `/metrics`, no worker support | This service builds its own bootstrap in `internal/app`, reusing `httpx` middleware and matching `serviceapp` semantics for `/health`; documented divergence. |
| Existing services default secrets in dev config | This service fails startup on missing mandatory secrets in production env and requires explicit values for auth-critical settings (spec §10, §22). Development still needs explicit values in `.env`. |
| Spec route base `/fleet-partner/v1` vs repo `/api/v1/<name>` | Spec canonical routes win (`/fleet-partner/v1/*`, `/internal/fleet-partner/v1/*`); they are unambiguous and gateway-routable. |
| Spec generic env names vs repo prefixed names | Repo prefix style wins; mapping table in `docs/configuration.md`. |

---

## 5. Blockers (session 1 — all resolved in session 2, see §7)

1. ~~**No live PostgreSQL for this service in this session.**~~ Resolved: a
   disposable container was provisioned and the full suite ran (§7).
2. **No event broker exists.** Outbox publishing runs against a disabled
   adapter until a transport is chosen (see Deferred). This is a deferred
   integration, not a foundation blocker.

---

## 6. Deferred integrations

- Live event-broker adapter for the outbox publisher.
- Real business methods on all 11 platform service clients (only foundational
  contract surface implemented).
- Organization membership store + resolver (staff/IAM work package).
- Verification, wallet, notification, media, support, analytics business flows.
- Tracing exporter (request/correlation IDs propagate regardless).
- Rate limiting backend (interface only).

---

## 7. Session 2 — completion and validation audit

Second pass performed to complete and validate Sections 3–7 against a real
database. This section supersedes any conflicting statement above.

### 7.1 Verified starting state

- Branch `staging`, commit `7320aaaeb3bb7427b1ffb15c003bc6389d549c1f`
  (unchanged; nothing had been committed).
- `go env`: `GOWORK=C:\KarryGoPush\go.work`, `GOPATH=C:\Users\favou\go`,
  `GOOS=windows`, `GOARCH=amd64`, Go `1.26.3`.
- All session-1 implementation files were present and did build.
- `go.work` correctly listed `./services/fleet-partner-service`.

### 7.2 Corrections to the session-1 report

| Session-1 claim | Verified reality |
| --- | --- |
| "PostgreSQL integration tests skipped — no database" | **Corrected.** A disposable `postgres:16-alpine` container was provisioned and **all 27 integration tests ran and passed with zero skips.** |
| "`go work sync` modified dependency files across several services" | **Confirmed and reverted.** 23 unrelated `go.mod`/`go.sum` files plus `go.work.sum` were restored to `HEAD`. Only `go.work` (one `use` line) remains modified. The churn was pure workspace reconciliation to pgx v5.10.0 — a version `api-gateway` already required at `HEAD`, so nothing in this service needed it. |
| "No live migration replay completed" | **Corrected.** Fresh apply, no-op re-apply, rollback, re-apply and mid-run failure rollback all verified against the real database. |
| Job dedupe uniqueness | **Defect found and fixed.** The partial unique index was `(job_type, dedupe_key)` — global, so two organizations could not hold the same dedupe key. Now `(COALESCE(organization_id,''), job_type, dedupe_key)`, with a regression test. |
| Branch listing "enforces permitted branch IDs" | **Defect found and fixed.** `PermittedBranchIDs` was carried in `tenant.Query` but never applied in SQL. Now enforced in the `WHERE` clause with a test. |
| "Timezone must be a valid IANA timezone" | **Gap found and fixed.** Documented but unenforced. `ValidateTimezone`/`ValidateCurrency` now run on organization and branch writes (with `time/tzdata` embedded so validation works on Windows and slim containers). |
| Organization settings | **Gap found and fixed.** The table existed with no repository. `SettingsRepository` added with timeout-bound and feature-flag-namespace validation plus compare-and-swap updates. |
| Suspended membership | **Gap found and fixed.** Only missing membership failed closed. `Membership.Status` is now checked; non-`ACTIVE` membership is refused (403). |

### 7.3 Environment facts (session 2)

- **Docker**: available (server 29.1.3) after starting Docker Desktop.
- **Local PostgreSQL on :5432** belongs to an unrelated project — deliberately
  **not** used. Test database credentials were **not** taken from any existing
  service secret; a fresh random password was generated for the throwaway
  container.
- **Redis**: not running locally; it is optional for this service, and the
  degraded path is covered by unit tests (`internal/platform/cache`).
- **No repository test-database harness exists** — `infra/docker-compose.yml`
  is entirely commented out and `scripts/` holds only psql shells for other
  services. A service-local harness (`scripts/ci.sh`) was therefore added.

### 7.4 Boundary re-verification

- No file in this service imports another service's package (architecture
  test `TestNoOtherServiceInternalsImported`).
- No foreign database DSN name appears in configuration
  (`TestNoForeignServiceDSNsInConfig`); the only connection string is
  `FLEET_PARTNER_DATABASE_URL`.
- Migrations contain no `dblink`, foreign data wrapper or cross-schema
  reference; a live catalogue query confirms every foreign key targets a
  `fleet_partner_*` table.
- `shared/go` is used only for generic helpers (gin middleware, HS256 token
  auth, HMAC service auth, redis dial, apperrors, logging). None of them
  opens a database connection — the pool is constructed locally from this
  service's own URL.

### 7.5 Remaining gaps

None for Sections 3–7. The only outstanding items are the deferred
integrations in §6, which are explicitly out of scope for this work package.
