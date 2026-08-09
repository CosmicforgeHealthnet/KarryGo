# Section 29 — Reliability, Observability and Operational Readiness

Scope: `services/fleet-partner-service`. This document is the reference for
everything Section 29 added or verified this session. Disaster recovery
(RPO/RTO, backup/restore procedure, replay/rebuild) has its own document:
[`docs/section29-disaster-recovery.md`](section29-disaster-recovery.md).

## 1. Structured logging

All logs are structured JSON, emitted by `internal/observability.Log`
(pre-existing) — every call site added this session uses it; no call site
anywhere in this service falls back to unstructured text logging in
production (the one gap found — `shared/go/httpx.Recovery`'s plain-text
panic log — was replaced by a service-local `middleware.Recovery` that logs
through the same structured path; see §1.4).

### 1.1 Fields

| Field | Present on | Source |
|---|---|---|
| `timestamp`, `level`, `service`, `environment` | every log line | `observability.Log` |
| `requestId` | HTTP requests | `httpx.RequestID` middleware |
| `correlationId` | HTTP requests, jobs, events | `middleware.CorrelationID`; threaded onto `jobs.Job.CorrelationID` / `events.Envelope.CorrelationID` |
| `traceId`, `spanId` | HTTP requests, external client calls, jobs, outbox publish, stream consume | `internal/platform/tracing` (new this session) |
| `organizationId` | tenant-scoped requests/jobs/events | resolved tenant context |
| `branchId` | requests carrying a `:branchId` path param | `middleware.RequestLogger` |
| `actorType`, `actorId` | requests with a resolved tenant or service caller | tenant context / `ServiceCallerKey` |
| `serviceCallerId` | internal HMAC-authenticated requests | `middleware.ServiceCallerKey` |
| `method`, `normalizedRoute` (+`route` alias) | HTTP requests | `c.FullPath()` — a route **template**, never a raw path with an embedded ID |
| `statusCode` (+`status` alias) | HTTP requests | response writer |
| `durationMs` | HTTP requests, jobs, spans | measured |
| `safeErrorClass`, `safeErrorCode` | failed requests/jobs/events/spans | bounded classification, never a raw driver/library error string |
| `retryAttempt` | external-client retry attempts | `platformclients.Client`'s retry loop |
| `jobId`, `jobType`, `attempt`, `leaseOwner` | every job lifecycle log line | `jobs.Worker.logJob` (new this session) |
| `eventId`, `eventType`, `aggregateType`, `aggregateId` | outbox publish / stream consume | `events.OutboxWorker` / `events.StreamConsumer` (new this session) |

### 1.2 What is never logged

Raw request/response bodies, job/event payloads, tokens, cookies, signed
URLs, or exact GPS coordinates. Sensitive-field redaction is
`internal/platform/redact` (pre-existing, Section 28), reused unchanged by
every new log call site this session added — `observability.Log` itself
redacts any field whose key `redact.IsSensitiveKey` flags, so a call site
cannot accidentally leak a secret by adding one more field to its `fields`
map.

### 1.3 Route/label cardinality

HTTP: `c.FullPath()` (gin's own route template, e.g. `/jobs/:jobId`, never
the raw path). External clients: `normalizePathTemplate`
(`internal/platformclients/base.go`, new this session) replaces every
ID-shaped path segment (this service's own opaque-ID shape, UUIDs, long
hex/numeric segments) with `{id}` before it is ever used as a metric label
or span name — this closes a real, pre-existing cardinality bug: the
client-call metric's `path` label previously carried the raw,
ID-interpolated path verbatim (confirmed live: `/objects/{id}/content` was
previously `/objects/med_xyz.../content`, one label series per media
object). Proven by `TestReferenceIdentifiersArePathEscaped` and
`TestMetricsInstrumented` (both passing).

### 1.4 Panic recovery

`middleware.Recovery` (new this session, `internal/middleware/middleware.go`)
replaces `shared/go/httpx.Recovery` in this service's own router
(`internal/app/app.go`) only — the shared package itself is unchanged
(other services still use it as-is). Logs `panic_recovered` as structured
JSON with `requestId`/`traceId`/`route`/`safeErrorClass:"panic"` and a
length-capped stringification of the recovered value, then responds with
the same safe 500 envelope `httpx.RespondError` always has.

### 1.5 Worker and event logging

`jobs.Worker.execute` (extended this session) logs `job_started`,
`job_tenant_mismatch`, `job_no_handler`, `job_shutdown_released`,
`job_failed`, `job_succeeded` — every one carrying `jobId`/`jobType`/
`attempt`/`leaseOwner`, never the job payload.

`events.OutboxWorker.publishOne` and `events.StreamConsumer.process`
(extended this session) log `outbox_published` / `outbox_publish_failed` /
`outbox_publisher_unconfigured`, and `stream_consumer_processed` /
`_duplicate` / `_handler_error` / `_contract_error` / `_payload_conflict` /
`_inbox_error` / `_decode_error` respectively — every one carrying
`eventId`/`eventType`/`aggregateType`/`aggregateId`/`correlationId`, never
the event payload.

### 1.6 Tests

`internal/middleware` (JSON validity, required fields, redaction — all
pre-existing and re-run passing this session), plus the new logging output
visually and mechanically confirmed throughout this session's own test runs
(every `go test -v` invocation shown in the final report emits and is
readable as one-JSON-object-per-line).

## 2. Metrics

Registry: `internal/observability.Metrics` (pre-existing, in-memory,
Prometheus-text-exposition `Render()`, protected `/fleet-partner/v1/metrics`
route gated by `FLEET_PARTNER_METRICS_AUTH_TOKEN`). No second metrics
framework was introduced.

| Category | Metric | Added |
|---|---|---|
| HTTP | `fleet_partner_http_requests_total{method,route,status}`, `fleet_partner_http_request_duration{method,route}`, `fleet_partner_http_active_requests` | pre-existing |
| DB pool | `fleet_partner_db_pool_{total,acquired,idle,max}_conns`, `_new_conns_total`, `_acquire_count_total`, `_acquire_duration_seconds`, `_canceled_acquire_count_total`, `_empty_acquire_count_total` | **new** (`internal/observability.ObservePoolStats`, wired into both binaries) |
| Outbox/events | `fleet_partner_outbox_{backlog,stale_backlog}`, `_oldest_pending_age_seconds` (**new**), `_published_total`, `_publish_failures_total`, `_publisher_unconfigured_total`, `_claim_errors_total`; `fleet_partner_stream_consumer_*` | pre-existing + one new gauge |
| Jobs | `fleet_partner_jobs{status}`, `_claims_total`, `_retries_total`, `_succeeded_total`, `_lease_lost_total`, `_shutdown_released_total`, `_tenant_mismatch_total` | pre-existing |
| External clients | `fleet_partner_client_calls_total{service,method,path,outcome}` (path now the **bounded template**, not the raw path — see §1.3), `fleet_partner_client_call_duration{service}`, `fleet_partner_client_retry_attempts_total{service,operation}` (**new**) | extended |
| Circuit breaker | `fleet_partner_circuit_breaker_state{dependency}` (0=CLOSED,1=HALF_OPEN,2=OPEN) | **new** |
| Spans | `fleet_partner_span_duration{span}`, `fleet_partner_span_errors_total{span}` | **new** |
| Business (pre-existing, unaffected) | `fleet_partner_provider_allocation_outcomes_total`, `fleet_partner_wallet_unknown_outcomes_total`, `fleet_partner_provider_reconciliation_total`, and others per-feature | unchanged |

**Bounded-label discipline**: every label above is a fixed-cardinality
category (service name, HTTP method, route/path *template*, status class,
outcome enum, dependency name, span name) — never an organization ID, user
ID, order ID, or any other unbounded value. This is enforced by
construction (no call site anywhere in this session's changes passes a raw
ID as a label) and spot-checked live via `metrics.Render()` output during
testing.

Both binaries (`cmd/main.go`, `cmd/worker/main.go`) register the metrics
they own; `observability.NewMetrics()` panics on duplicate *series* only if
misused with conflicting types, which no call site does (proven implicitly
by every test in this session completing without panic).

## 3. Distributed tracing

`internal/platform/tracing` (**new package**, this session). The repository
had no existing tracing library — this follows the exact same
"small, dependency-free, in-repo" precedent `internal/observability` already
set for metrics (see that package's own doc comment). No OTLP exporter
exists; spans are recorded as structured log lines + duration/error metrics
only — this means a missing tracing backend can never block core operation,
because there is no network call in the span-recording path at all.

- **W3C propagation**: `traceparent` parsed/generated per the W3C Trace
  Context spec (`ParseTraceParent`/`Context.TraceParent`); `tracestate`
  forwarded opaquely and length-capped; `baggage` is not forwarded (no
  approved safe keys exist yet — explicitly disclosed, not silently
  dropped).
- **Sampling**: configurable via `FLEET_PARTNER_TRACE_SAMPLE_RATE`
  (0..1, validated at startup); only applies to a NEW root trace — a
  propagated trace always keeps the upstream's sampling decision (head-based
  sampling, per spec).
- **Spans created for**: inbound HTTP requests (`middleware.Tracing` +
  `RequestLogger`), every external client call
  (`platformclients.Client.Exchange`, span name `client.<service>`), every
  durable job execution (`jobs.Worker.execute`, span name `job.<jobType>`),
  outbox publish (`events.OutboxWorker.publishOne`, span `outbox.publish`),
  and stream consumption (`events.StreamConsumer.process`, span
  `stream.consume.<eventType>`).
- **Async linking**: a job or event span is NOT a literal continuation of
  the W3C trace that originated it (jobs/events cross a durable-storage
  boundary, not an HTTP call) — it links back via the SAME `correlationId`
  every `Job` and `Envelope` already carried before this package existed
  (see the package's own doc comment for the full reasoning). An operator
  can pivot from an HTTP request's log line to every downstream job/event
  log line for the same `correlationId`.
- **Database/Redis operation spans**: **disclosed scope limitation** — a
  reusable `tracing.StartSpan` helper exists and is applied at the
  transaction/job/event/HTTP/client chokepoints above, but per-repository-
  method instrumentation was not added across this service's ~150
  repository files; that is future incremental work, not done this
  session. DB pool *metrics* (§2) exist regardless and do not depend on
  per-query spans.

### 3.1 Propagation tests

- **API → Wallet/Media/execution clients**: `TestSignedRequestWithTracingHeaders`
  and every other `platformclients` test (all re-run, all passing) confirm
  the `traceparent` header is set on every outbound signed request; the new
  retry/circuit-breaker tests (`internal/platformclients/retry_circuit_test.go`)
  additionally confirm a span/trace ID is produced per attempt.
- **Domain transaction → outbox → Redis consumer**: proven end-to-end this
  session via `TestSection26RealRedisTransactionalDedupDeadLetterAndReplay`
  (real Redis) — its own log output (captured in this session's test run)
  shows a `stream.consume.<eventType>` span with the SAME `correlationId`
  the originating envelope carried.
- **Scheduled task → job worker**: `jobs.Worker.execute`'s `job.<jobType>`
  span, proven by this session's `TestNotificationDegradedMode_...` test,
  whose log output shows the span closing with `safeErrorClass` set on
  failure and absent on success.

## 4. Liveness and readiness

`GET /live` (new) and `GET /health` (pre-existing, unchanged) are byte-for-
byte the same handler — dependency-free by construction (no `ReadyChecks`
are ever wired to it; see `internal/app/app.go`). Proven by
`TestLiveRouteIsDependencyFree` (new): `/ready` reports 503 with Postgres
reported down, `/live` reports 200 regardless, in the same test.

`GET /ready` (pre-existing mechanism, extended this session) evaluates a
list of named `ReadyCheck`s, each reduced to `{dependency, "unavailable"}`
on failure — never a raw error or secret (`TestReadySucceedsAndFails`,
pre-existing, re-verified). Checks, and when each is mandatory:

| Check | Mandatory when |
|---|---|
| `postgres`, `migrations` | always |
| `redis` | `FLEET_PARTNER_REDIS_REQUIRED=true` |
| `retention_expired_export`/`_support_metadata`/`_superseded_projection` | `RETENTION_EXECUTION_ENABLED=true` (Section 28) |
| `risk_registry`, `object_authorization_registry` | always (self-consistency check, Section 28) |
| `service_credential_scoping` | `SERVICE_CREDENTIAL_SCOPING_ENABLED=true` (Section 28) |
| `dispatch_execution_worker` | `DISPATCH_EXECUTION_START_ENABLED=true` (**new**) |
| `hauling_execution_worker` | `HAULING_INTEGRATION_ENABLED=true` (**new**) |
| `audit_persistence` | always (**new**) |
| `job_stale_leases`, `job_dead_letter_backlog` | `FLEET_PARTNER_WORKER_ENABLED=true` (**new**) |
| `outbox_backlog` | `FLEET_PARTNER_WORKER_ENABLED=true` (**new**) |
| `outbox_publisher` | `FLEET_PARTNER_WORKER_ENABLED=true` AND Redis is configured (**new**) |
| `worker` (in-process worker loop) | `FLEET_PARTNER_WORKER_ENABLED=true` (pre-existing) |

A genuine, previously-silent gap this session found and fixed:
`config.ExecutionCapabilities()` ANDs the raw enablement flag with client
configuration, so a deployment that turned `DISPATCH_EXECUTION_START_ENABLED`
on but forgot the client would previously run with the feature silently
disabled and report **ready** regardless. The `dispatch_execution_worker` /
`hauling_execution_worker` checks above close that gap by checking the raw
flag independently.

All 12 new/extended readiness properties are proven against a real database
in `internal/app/section29_readiness_test.go` (all passing, this session).

**cmd/worker/main.go note**: the dedicated worker binary is, by pre-existing
and deliberate design (its own doc comment states this), headless — no HTTP
server, hence no `/ready` of its own. Its readiness is observed through the
API process's `/ready` (both binaries build `Components` through the
identical `BuildComponents`+`Registrars` composition) plus process
supervision and the metrics/backlog endpoints above. This is disclosed as a
pre-existing architectural choice, not a gap introduced or left open this
session.

## 5. External-client deadlines

Every platform client (`internal/platformclients`) is built through
`platformclients.Client`, which:

- Applies `context.WithTimeout(ctx, c.timeout)` **per attempt**, never a
  bare `context.Background()` — a genuine violation found and fixed this
  session: `internal/features/dispatch/optimizer_client.go`'s `Submit`/
  `ReadBack` used `http.NewRequest` (no context at all, equivalent to
  `context.Background()`), unable to respect a caller's cancellation. Fixed
  to `http.NewRequestWithContext`, threading the real job-handler `ctx`
  through `OptimizationPort`'s interface.
- Always closes the response body (`defer resp.Body.Close()`), and bounds
  the read (`MaxResponseBytes`, default 1 MiB).
- Records a timeout as `OutcomeUnknown` (never blindly treated as failure)
  when the deadline was exceeded — see §6.

**Architecture test**: `TestExternalHTTPClientsHaveExplicitTimeout`
(`internal/architecture_test.go`, new) scans every non-test `.go` file
outside `internal/platformclients` itself for a raw `http.Client{...}`
literal lacking an explicit `Timeout` field. Two pre-existing sites
(`internal/app/crm_wiring.go`, `internal/features/dispatch/optimizer_client.go`)
were audited and both already set one explicitly; the test now guards
against a future regression.

Separate connect/TLS/response-header timeouts are not independently
configured (Go's `http.Client.Timeout` bounds the whole round trip
including those phases) — disclosed as the deliberate, simpler choice this
service already made before this session; splitting them further was not
identified as a genuine gap given the whole-request timeout already bounds
every failure mode Section 29 cares about (a hung connect, a hung TLS
handshake, and a hung response all time out the same way).

## 6. Retry policy and unknown-outcome handling

Implemented once, in `platformclients.Client.Exchange` (rewritten this
session), applied uniformly to every client without any of the ~15 concrete
client files needing to change:

- **Classification** reuses the pre-existing `Outcome` enum
  (`OutcomeTransient` retryable; `OutcomeValidation`/`Forbidden`/
  `Unauthenticated`/`NotFound`/`Conflict` never retried;
  `OutcomeUnknown` — a timeout after the request was already sent — never
  blindly retried).
- **Write safety gate**: a write (non-GET/HEAD/OPTIONS) is only
  auto-retried when the caller supplied `CallMeta.IdempotencyKey` — this is
  "writes require idempotency before automatic retry" expressed as a gate
  every call site automatically satisfies or fails safely, rather than a
  rule each of the ~15 clients would otherwise have to remember
  individually.
- **Bounded attempts, exponential backoff, jitter**:
  `FLEET_PARTNER_EXTERNAL_CLIENT_MAX_RETRIES` (default 2),
  `_RETRY_BACKOFF_BASE`/`_MAX` (default 200ms/5s), half-jittered
  exponential growth.
- **Retry-After**: a 429/503 response's `Retry-After` (delta-seconds form)
  overrides the computed backoff for the next attempt, capped at
  `RetryBackoffMax`.
- **Attempt tracing/metrics**: `retryAttempt` structured log field,
  `fleet_partner_client_retry_attempts_total`.
- **No nested retry multiplication**: retry lives in exactly one place
  (`Client.Exchange`); no caller wraps it in a second retry loop.

Tested (`internal/platformclients/retry_circuit_test.go`, new, all
passing): transient-GET-retried-until-success, validation-never-retried,
write-without-idempotency-key-never-retried,
write-with-idempotency-key-is-retried, unknown-outcome-never-auto-retried,
bounded-attempts-exhausted, Retry-After honored (measured ≥900ms from a
server-supplied `Retry-After: 1` against a 1ms base backoff — provably not
just the computed backoff), context-cancellation stops the retry loop.

**Unknown-outcome handling for state-changing calls**: already
comprehensively implemented before this session for Wallet
(`companywallet` allocation/withdrawal workers: durable instruction →
submit-or-read-back, `TestUnknownOutcomeIsResolvedByReadBackNotResubmission`)
and execution (`privateoperations`: `TestPrivateExecutionWorker_UnknownDoesNotBlindlyResubmitAndLeaseOwnership`,
`TestExecutionStartUnknownOutcomeKeepsOriginalIdempotencyKey`) — both
re-verified passing this session. See §9/§10 for the full degraded-mode
detail.

## 7. Circuit breakers

`internal/platform/circuitbreaker` (**new package**), one `Breaker` per
downstream service name (never a shared global breaker), in-memory state
(a restart resets to CLOSED — the deliberate, documented choice; durable
breaker state was judged not worth the persistence cost for a signal that
is safe to lose on restart).

- **States**: CLOSED → OPEN (after `FailureThreshold` classified failures
  within `ObservationWindow`) → HALF_OPEN (after `OpenDuration`, bounded to
  `HalfOpenProbes` concurrent trial calls) → CLOSED (after
  `SuccessThreshold` consecutive HALF_OPEN successes) or back to OPEN (any
  HALF_OPEN failure).
- **Classification**: only `OutcomeTransient`/`OutcomePermanent`/
  `OutcomeUnknown` count as breaker failures — validation, auth, not-found
  and conflict outcomes never do (`Outcome.isBreakerFailure`,
  `platformclients/base.go`).
- **Integration**: wraps the whole retry sequence in
  `platformclients.Client.Exchange` — OPEN refuses immediately, before any
  network attempt.
- **Config**: `FLEET_PARTNER_CIRCUIT_BREAKER_FAILURE_THRESHOLD` (default
  5), `_OBSERVATION_WINDOW`/`_OPEN_DURATION` (default 30s each),
  `_HALF_OPEN_PROBES`/`_SUCCESS_THRESHOLD` (default 2 each) — all
  startup-validated (bounded, non-zero-unbounded).
- **Metrics/logs**: `fleet_partner_circuit_breaker_state{dependency}`
  gauge; `circuit_breaker_transition` structured log line on every state
  change.
- **Readiness integration**: deliberately none — every external dependency
  in this service is already optional/degraded-mode-capable (§9–§12), so no
  breaker gates startup readiness; a breaker being OPEN is observable via
  its own metric/logs, not a readiness failure.

Tested (`circuitbreaker` package, 11 tests; `platformclients` integration,
4 tests — all passing): CLOSED→OPEN at threshold, OPEN refuses without a
network attempt, OPEN→HALF_OPEN after `OpenDuration`, HALF_OPEN bounds
concurrent probes, HALF_OPEN failure re-opens, HALF_OPEN success threshold
closes, failures outside the observation window do not accumulate,
transitions are reported, per-dependency independence (one dependency's
failures never trip another's breaker), and validation/auth errors never
trip it.

## 8–11. Degraded modes

Full detail and evidence in the final Section 29 report; summary:

- **Wallet**: durable-instruction pattern (request written once, a worker
  submits-or-reads-back under the SAME idempotency key), staleness gates
  block a stale projection from funding an allocation, never a fabricated
  balance. Pre-existing, comprehensively tested, re-verified this session.
- **Execution** (Dispatch/Hauling private assignments): local acceptance is
  never recorded before remote confirmation; an unknown outcome requires
  read-back; a cancellation while unknown is itself handled distinctly.
  Pre-existing, comprehensively tested, re-verified this session.
- **Notification**: core domain mutation (e.g., a chat message) always
  succeeds independent of notification-service reachability; the intent is
  durably persisted and submitted via the Section 27 job framework under a
  dedupe key; a classified failure schedules a retry; exhausting
  `max_attempts` dead-letters; an operator can replay; resubmitting the
  identical message never creates a second intent. **New end-to-end test
  this session** (`TestNotificationDegradedMode_CoreMutationPersistsRetriesDeadLettersAndReplays`)
  drives this through the REAL registered job handler via a real
  `*jobs.Worker` pass, not a reimplementation — proves core mutation
  success, intent persistence, transient retry (attempt=1 →
  RETRY_SCHEDULED), permanent dead-letter (attempt=8 → DEAD_LETTERED),
  replay, and duplicate prevention, all against real Postgres.
- **Analytics** (an outbox consumer like any other): the core transaction
  commits and the event is appended to the outbox transactionally
  regardless of whether any consumer is reachable; the backlog is
  observable (`fleet_partner_outbox_backlog`,
  `_oldest_pending_age_seconds`); nothing is ever silently dropped — a
  claimed-but-unpublished event remains a durable row either way. **New
  test this session** (`TestAnalyticsDegradedMode_CoreCommitsBacklogGrowsNoEventLoss`).

## 12. Backup, restore, DR

See [`docs/section29-disaster-recovery.md`](section29-disaster-recovery.md)
for the full procedure, real restore-test evidence, RPO/RTO, and replay/
rebuild test evidence.

## 13. Capacity profiles

`internal/integration/section29_capacity_test.go` (new). Three named,
repeatable, DEVELOPMENT-MACHINE-SCALE profiles (explicitly not a production-
capacity claim — see the file's own doc comment):

| Profile | Size | Result (this session's run) |
|---|---|---|
| Large single tenant | 2,000 durable jobs, one organization | seed 1.53s; list p50 2.28ms, p95 3.13ms; `EXPLAIN` confirms an index scan (`idx_fleet_partner_jobs_organization`), not a sequential scan |
| Multi-tenant | 30 organizations × 50 jobs = 1,500 rows | seed 1.23s; per-tenant list p50 722µs, p95 1.26ms; **zero cross-tenant leakage** across all 30 organizations (asserted per-row, not just per-count) |
| Worker backlog | 500 jobs, batch size 25 | seed 410ms; full drain 1.22s; **410.7 jobs/sec** throughput; exactly 500 executions (no duplicate/lost work under `SKIP LOCKED` claiming) |

**Disclosed scope**: the jobs table is used as the representative
high-volume resource (see the test file's own doc comment for why, given
this session's time budget) rather than seeding all ~15 resources the
mandate names individually (orders, shipments, assignments, invoices,
etc.). The same `organization_id`-led index + cursor-pagination
(`internal/platform/cursor`, `MaxLimit=100`) pattern is enforced uniformly
across this service's schema (confirmed by
`internal/architecture_test.go`'s `TestSQLAlwaysTenantScoped` and by direct
inspection of the jobs/outbox indexes below), so this profile's findings
(index-backed, bounded latency, zero tenant leakage) are representative of
the same query shape used everywhere else, not proof that literally every
other endpoint was individually load-tested.

## 14. Index and pagination findings

- **Pagination**: `internal/platform/cursor` (pre-existing) — opaque
  cursor, `DefaultLimit=25`, `MaxLimit=100` (a compile-time constant, not a
  runtime-configurable value — cannot be misconfigured to "unlimited").
  Stable ordering via a timestamp sort key + ID tie-breaker.
- **Job claims**: `idx_fleet_partner_jobs_claimable` — a PARTIAL index
  (`WHERE status IN ('PENDING','RETRY_SCHEDULED')`) on
  `(priority DESC, scheduled_at)` — exactly the efficient partial-index
  pattern Section 29 calls for.
- **Outbox claims**: `idx_fleet_partner_outbox_pending` — a partial index
  (`WHERE status = 'PENDING'`) on `available_at`.
- **Tenant-scoped listing**: `idx_fleet_partner_jobs_organization` on
  `(organization_id, created_at)`; `idx_fleet_partner_outbox_organization`
  likewise — both confirmed via `EXPLAIN` (§13) to be used at the tested
  row counts.
- **No new index migration was added.** Both indexes exercised by the
  capacity tests already existed and were already sufficient — "add
  migration indexes only where evidence shows they are required; do not
  add indexes blindly" is satisfied by adding none, backed by the `EXPLAIN`
  evidence in §13, rather than speculatively adding one.

## 15. Worker sizing

| Worker | Default batch size | Default lease | Default poll interval | Source |
|---|---|---|---|---|
| Durable jobs (`jobs.Worker`) | 10 (hardcoded default in `Run`) | `FLEET_PARTNER_JOB_LEASE_DURATION` (60s default) | `FLEET_PARTNER_JOB_POLL_INTERVAL` (2s default) | pre-existing |
| Outbox publisher (`events.OutboxWorker`) | 20 | shares job lease duration | `FLEET_PARTNER_OUTBOX_POLL_INTERVAL` (2s default) | pre-existing |
| This session's capacity test | 25 (explicit) | 5s | 20ms | `section29_capacity_test.go` — recommended as a reasonable order-of-magnitude default for a durable-job worker: large enough to amortize the `SKIP LOCKED` claim query, small enough to bound per-claim memory and keep individual job latency low |

Recommended production concurrency: one worker process per
`cmd/worker/main.go` instance; horizontal scaling is via running additional
worker instances, each independently polling — `SKIP LOCKED` already
guarantees no double-claim across any number of concurrent instances
(proven generically by the pre-existing `TestScheduler_ClaimDueIsSkipLockedAcrossConcurrentSchedulers`
and this session's own capacity test's zero-duplicate-execution result).

Graceful shutdown: `jobs.Worker.Run` stops claiming new work on context
cancellation; an in-flight handler that observes cancellation before any
external effect returns `ErrShutdownSafeToRelease`, releasing the lease
immediately without consuming a retry attempt (pre-existing mechanism,
unchanged).

## 16. Operational configuration

All new Section 29 configuration is in `internal/config/config.go`,
validated at startup (`Config.Validate`) — production rejects a zero/
unbounded external-client timeout, an out-of-range retry count, an invalid
circuit-breaker threshold, or a non-positive readiness duration; see the
config test fixture (`internal/config/config_test.go`) for the exact bounds
enforced.

| Setting | Env var | Default |
|---|---|---|
| Trace sample rate | `FLEET_PARTNER_TRACE_SAMPLE_RATE` | 1.0 |
| External client max retries | `FLEET_PARTNER_EXTERNAL_CLIENT_MAX_RETRIES` | 2 |
| External client retry backoff base/max | `FLEET_PARTNER_EXTERNAL_CLIENT_RETRY_BACKOFF_BASE`/`_MAX` | 200ms / 5s |
| Circuit breaker failure threshold | `FLEET_PARTNER_CIRCUIT_BREAKER_FAILURE_THRESHOLD` | 5 |
| Circuit breaker observation window / open duration | `FLEET_PARTNER_CIRCUIT_BREAKER_OBSERVATION_WINDOW`/`_OPEN_DURATION` | 30s / 30s |
| Circuit breaker half-open probes / success threshold | `FLEET_PARTNER_CIRCUIT_BREAKER_HALF_OPEN_PROBES`/`_SUCCESS_THRESHOLD` | 2 / 2 |
| Readiness outbox staleness / backlog critical | `FLEET_PARTNER_READINESS_OUTBOX_STALE_AFTER`/`_BACKLOG_CRITICAL` | 10m / 5000 |
| Readiness job dead-letter critical | `FLEET_PARTNER_READINESS_JOB_DEAD_LETTER_CRITICAL` | 500 |
| Readiness stale-lease threshold | `FLEET_PARTNER_READINESS_STALE_LEASE_THRESHOLD` | 10m |

Alert thresholds: the readiness-critical values above (job dead-letter
count, outbox backlog count/staleness) double as the recommended
alert-before-critical thresholds — an operator's monitoring should page at
some fraction of these (e.g., 50%) before `/ready` itself starts failing.

## 17. Observability security

- Metrics/spans: bounded labels only (§2, §3) — no tenant/user/order ID
  ever used as a label.
- Logs: Section 28 redaction (`internal/platform/redact`) applied uniformly
  by `observability.Log`; no raw job/event payload logged (§1.5).
- Health endpoints: `/live`/`/health`/`/ready` never include a credential or
  raw internal error — `/ready` reduces every failure to
  `{dependency, "unavailable"}` (pre-existing, re-verified).
- Backup logs/artefacts: `backup.sh`/`restore-verify.sh` never print or
  embed the database credential; only the connection string's *presence* is
  checked, never its value.
- Diagnostic/profiling endpoints: this service exposes none (no `pprof`
  route is registered anywhere in `internal/app`), so there is nothing to
  additionally protect or disable.

## 18. Incident procedures, rollout and rollback

Incident command roles and communication procedure are organizational, not
code — see [`docs/section29-disaster-recovery.md`](section29-disaster-recovery.md)
§15 for the explicit disclosure that this repository does not define them.

Rollout: this session's changes are additive (new packages, new optional
config with safe defaults, new optional routes) — no existing route,
config default, or behavior was removed or changed in a way that requires a
coordinated rollout sequence. Rollback: reverting this session's commits
(none of which touch a migration) is a plain code rollback with no data
migration to reverse.
