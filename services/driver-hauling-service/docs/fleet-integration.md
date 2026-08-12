# Fleet / Truck Hauling production contract

Truck Hauling remains the source of truth for truck providers, trucks, availability, organization links, reservations, executions, lifecycle status and proof references. Fleet stores only opaque identifiers and projections. Wallet remains the sole finance authority; Notification remains the delivery authority; Verification remains the compliance authority.

## Private API

The API is mounted below `/api/v1/hauling/internal/marketplace`. Every request requires shared HMAC signature version 2, which binds the caller identity, method, request target, timestamp, nonce and body hash. The timestamp must be within five minutes, `X-Service-Nonce` is single-use, and the verified caller must be allowlisted. Writes additionally require correlation and causation IDs plus `Idempotency-Key`; commands bind that key to a request fingerprint and reject conflicting reuse.

The surface provides reservation create/read/release, provider and truck eligibility, execution create/read-back, request/trip read-back, cancellation, guarded lifecycle transitions, and provider/truck authority lookup. Lifecycle ordering is enforced for pickup, loading, loaded, delivery travel, unloading, proof-backed delivery, completion, failure and cancellation. Expected source versions prevent stale writes.

Fleet private orders use the dedicated machine-readable contract at
`docs/openapi/private-execution.openapi.json`, mounted below
`/api/v1/hauling/internal/private-executions`. It preserves opaque Fleet order,
shipment, assignment and operation references while Hauling remains the sole
owner of the booking/trip lifecycle. The provider app reads and responds through
`/api/v1/hauling/provider/company-assignments/*` using its normal provider
bearer token; Hauling derives provider identity and signs the private Fleet
decision contract with HMAC v2.

Provider and truck links provide create/request, read and expected-version
transitions for `REQUESTED`, `ACTIVE`, `SUSPENDED`, `REVOKED` and `REJECTED`.
Transitions support controlled organization/branch reassignment, preserve an
immutable history and append a transactional outbox event. Reads include the
organization predicate so a foreign tenant receives the same not-found result
as an unknown opaque link.

Transition commands persist their idempotency key, request fingerprint and
response. An identical retry returns the original response without advancing
the source version or duplicating history/outbox; changed reuse returns a
conflict. Fleet resolves the current live provider link through the
tenant-scoped `provider-links/by-provider/:providerId` read before a controlled
suspension/offboarding revocation; an HTTP request ID is never treated as an
association ID.

## Events and recovery

State changes append `hauling_domain_outbox` in the same transaction. The relay leases rows with `FOR UPDATE SKIP LOCKED`, signs the Fleet event request, retries with bounded backoff, and marks delivery only after a 2xx acknowledgement. Fleet ingests event IDs idempotently and reconciles uncertain command outcomes through the request/trip read endpoints.

Booking notifications are appended to the shared Redis request stream consumed
by Notification service. A Notification outage leaves durable pending work;
Hauling never returns a successful no-op delivery. Wallet/card flows refuse an
unconfigured Wallet. Ambiguous Wallet holds and settlements perform signed
read-back; unresolved outcomes are persisted as `unknown`. Cash collection is
an explicit `HAULING_CASH_PAYMENTS_ENABLED` mode, false by default, and is never
recorded as Wallet-paid or settled.

## Enablement and rollback

Both sides default disabled. Hauling requires `HAULING_FLEET_INTEGRATION_ENABLED=true`, its kill switch off, an inbound caller allowlist and HMAC map, the outbox worker, a Fleet event URL/secret, Wallet URL/secret, and Notification URL/secret. Fleet requires `FLEET_HAULING_INTEGRATION_ENABLED=true`, its global kill switch off, and a Hauling URL/secret. Taxi enablement is rejected because no production Taxi contract exists.

The Hauling kill switch rejects every non-read HTTP method before legacy or
private handlers and prevents outbox, auto-complete and match-recovery workers
from starting. It does not delete pending rows, release leases as success or
convert an unknown result into success.

Use a dedicated `HAULING_TEST_DATABASE_URL`; never point it at Fleet or another
service database. Apply migrations in filename order, verify the tracked replay,
apply the latest down migration, and reapply before staging enablement.

Rollback order: set either kill switch, stop the outbox worker after in-flight leases expire, verify no execution-start operations remain unknown, then deploy code or schema rollback. Never delete undelivered outbox rows or fabricate Fleet projections.

## Operational checks

- Alert on undelivered outbox age, lease churn, retry count, nonce replay rejection, version conflict rate and UNKNOWN reconciliation age.
- Verify Wallet and Notification readiness before enabling Hauling.
- Confirm Fleet allowlists `driver-hauling-service` for event ingestion and Hauling allowlists only `fleet-partner-service` for commands.
- Exercise reserve/read-back, execution/read-back, proof-backed completion and kill-switch behavior in staging before rollout.
- For dead-letter replay, repair the downstream/configuration cause, clear only
  the dead-letter marker through the approved operator procedure, retain attempt
  history, and allow the leased worker to retry the same event ID.
- During an incident, activate both Fleet and Hauling kill switches, inspect
  unknown commands by idempotency key, use authoritative read-back, and never
  retry a timed-out mutation as a fresh command.

## Verification evidence

The Hauling PostgreSQL lifecycle test covers real Gin handlers, shared HMAC v2,
caller allowlisting, timestamp expiry, missing/changed/replayed nonce rejection,
cross-tenant not-found, provider/truck link creation, idempotency conflict,
transition replay, suspension/reactivation/revocation, reassignment, history
and outbox. Fleet's black-box test builds and launches the real Hauling binary
and drives the same authority through Fleet's production client. Final Hauling
full-suite runs pass twice (30.6s and 30.3s); the final Fleet runs including the
black-box path pass twice (167.2s and 140.6s), with no mandatory database skip.
