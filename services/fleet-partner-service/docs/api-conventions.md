# API Conventions

## Route classes and authentication

| Class | Path | Authentication |
| --- | --- | --- |
| Corporate web API | `/fleet-partner/v1/*` | `Authorization: Bearer <access-token>` — HS256 token (shared `auth` package) with role `organization_user`, service `fleet-partner-service` |
| Internal service API | `/internal/fleet-partner/v1/*` | HMAC service signature (shared `serviceauth`): `X-Service-Name`, `X-Service-Timestamp`, `X-Service-Signature`. User bearer tokens are rejected. |
| Health | `/health` | none (liveness only, no dependency checks) |
| Readiness | `/ready` | none (checks PostgreSQL, migration state, required Redis, worker readiness; failures are reported without credentials or stack traces) |
| Metrics | `/metrics` | `Authorization: Bearer <FLEET_PARTNER_METRICS_AUTH_TOKEN>`; constant-time compare; fails closed when unconfigured |

## Content type

`application/json` only. `multipart/form-data` is reserved for a future
direct-upload handoff and is not accepted by any foundation route. Mutating
requests with other content types receive `415`.

## Envelopes

Success (`internal/platform/respond`):

```json
{"data": {}, "meta": {"requestId": "..."}}
```

Lists add `meta.nextCursor` (null/omitted on the last page). Errors:

```json
{"error": {"code": "validation_failed", "message": "…", "details": [], "requestId": "…"}}
```

Codes come from `shared/go/apperrors` (`bad_request`, `validation_failed`,
`unauthorized`, `forbidden`, `not_found`, `conflict`, `rate_limited`,
`service_unavailable`, `internal_error`). Messages are safe and public; no
stack traces, SQL, credentials or downstream response bodies.

> Divergence note: older services use an ad-hoc `{"success": ...}` shape.
> The corporate API standardizes on the envelopes above per the corporate
> specification; error codes remain shared.

## Request and correlation IDs

`X-Request-ID` (shared `httpx`) and `X-Correlation-ID` (this service) are
accepted inbound, generated when absent, echoed on every response, included
in every envelope/log line and propagated on every downstream call.

## Dates

Timestamps are stored UTC (`timestamptz`) and serialized RFC 3339. Business
scheduling never uses server-local time; IANA timezones are stored explicitly
(`organizations.timezone`, `branches.timezone`) and **validated on write**
with `time.LoadLocation` — the tzdata database is embedded (`time/tzdata`) so
validation behaves identically on Windows, Linux and slim containers.
`Local` and empty values are rejected. Time-sensitive use cases accept
injected clocks for testing.

## Money

```json
{"amountMinor": 700000, "currency": "NGN"}
```

`amountMinor` is `int64`; the currency is an explicit validated three-letter
code, never inferred from country. Arithmetic and comparison across
currencies fail with `ErrCurrencyMismatch`; no floating point is used
(architecture-tested); no FX conversion exists.

## Identifiers

Locally owned resources use opaque prefixed IDs (`org_`, `brn_`, `dep_`,
`evt_`, `job_`, …) generated from UUIDv4 — never sequential, never database
serials. Expected prefixes are validated on input. External services' IDs are
opaque strings; their format is never assumed.

## Pagination

Cursor pagination (`internal/platform/cursor`) for high-volume resources:
opaque base64url tokens over a stable sort key plus an ID tie-breaker; limits
clamped to 100; malformed cursors are rejected (not silently reset).
Page/limit exists only where backward compatibility requires it.

## Filtering

Allowlisted filters and sorts (`internal/platform/filters`). Unknown filters,
unknown sort fields and malformed values are rejected with structured
validation details. Client values are only ever bound as SQL parameters.
Tenant filtering is applied by repositories independently of user filters.

## Idempotency

`Idempotency-Key` + normalized request fingerprint, organization-scoped per
endpoint (`internal/platform/idempotency`). First use claims the key
(concurrent claims settle on the primary key — exactly one winner); completed
success responses replay; the same key with a different request conflicts;
in-progress reuse conflicts; entries expire after 24h. Authentication
failures (401/403), rate limits and 5xx are never cached.

## Optimistic concurrency

Aggregates carry `version`. Mutations supply the expected version via
`If-Match` or `expectedVersion` (`internal/platform/concurrency`);
repositories update with compare-and-swap (`WHERE … AND version = $n`) and a
mismatch surfaces as `409 conflict`. Direct unrestricted status updates do
not exist.

## Audit

Append-only audit records (`internal/platform/audit`) capture organization,
branch, actor, action, resource, redacted before/after summaries, reason,
request/correlation IDs, timestamp and safe network metadata — written in the
same transaction as the domain mutation. UPDATE/DELETE are blocked by a
database trigger.
