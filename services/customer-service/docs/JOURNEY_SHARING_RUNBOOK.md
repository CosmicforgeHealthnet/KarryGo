# Journey sharing rollout and verification

The implementation is disabled by default. Do not enable it based only on
compilation or fixture-based tests. Read the Sections 9–11 report for the
remaining acceptance requirements.

## Configuration

| Service | Setting | Default / purpose |
|---|---|---|
| Customer | `CUSTOMER_SHARE_JOURNEY_ENABLED` | `false`; gates owner commands and public reads |
| Taxi | `TAXI_CUSTOMER_LIVE_LOCATION_ENABLED` | `false`; gates both N07 and N08 |
| Customer | `CUSTOMER_SHARE_JOURNEY_TTL_SECONDS` | 86400; hard expiration |
| Customer | `CUSTOMER_SHARE_COMPLETED_RETENTION_SECONDS` | 900; completion/cancellation observation shortens expiration |
| Customer | `CUSTOMER_SHARE_VIEWER_ORIGIN` | Empty; cross-origin browser reads fail until the exact official viewer origin is set |
| Customer | `CUSTOMER_SHARE_TRUSTED_GATEWAY_CIDRS` | Empty; comma-separated **actual gateway** networks for viewer IP resolution; untrusted forwarding is ignored |
| Taxi | `TAXI_CUSTOMER_LOCATION_FRESH_SECONDS` | 30; source freshness interval |

Configure `CUSTOMER_SHARE_TAXI_URL`, `CUSTOMER_SHARE_DISPATCH_URL`,
`CUSTOMER_SHARE_HAULING_URL`, `CUSTOMER_SHARE_VENDOR_URL` as private bare service
origins, and the corresponding `CUSTOMER_SHARE_<AUTHORITY>_SECRET` values.
For Vendor the authority suffix is `VENDOR`. Do not use the public gateway as
an internal origin: it deliberately rejects internal routes.

The outgoing Customer secret must match the `customer-service` entry in each
owner's `TAXI_SERVICE_SECRETS`, `DISPATCH_SERVICE_SECRETS`,
`HAULING_SERVICE_SECRETS`, or `VENDOR_SERVICE_SECRETS`. Taxi startup refuses
live tracking without that entry. Customer's private metric read requires
`admin-backoffice-service` in `CUSTOMER_SERVICE_SECRETS`.

Vendor's existing Dispatch/Hauling integration URLs, secrets and feature
flags control mission read-through. Its peer calls use `vendor-service`, not
the Customer caller identity. Enable each existing commerce integration only
with its own authoritative capability configuration. Unsupported logistics
authorities fail closed; this adapter does not invent a Taxi Goods or
Vendor Own Delivery location binding.

Keep the existing movement flags and publication settings consistent:
`CUSTOMER_MOVEMENT_EVENTS_CONSUMER_ENABLED`, customer movement reads,
`TAXI_CUSTOMER_MOVEMENT_EVENTS_ENABLED`,
`DISPATCH_CUSTOMER_MOVEMENT_EVENTS_ENABLED`,
`HAULING_CUSTOMER_MOVEMENT_EVENTS_ENABLED`, and
`VENDOR_CUSTOMER_MOVEMENT_GROUPING_ENABLED`. Existing source URLs/secrets
continue to govern publication and repair.

## Source references and capabilities

Taxi uses its passenger booking UUID; Dispatch uses the canonical trip UUID;
Hauling uses its booking UUID; Vendor accepts a scoped mission ID/reference.
Dispatch Activity still uses its booking reference: resolve the existing
booking's trip before creating a SEND share. Never substitute a provider ID.
Food/Shop sharing is per mission, not a whole purchase or sibling missions.

N01 and N05 disclose a new 256-bit capability exactly once. Idempotent replay
returns saved metadata without the plaintext token. If a response is lost,
read owner metadata and explicitly rotate with the current version and a new
idempotency key. Revocation and rotation are checked again after source I/O;
there is no public response cache. Expired/revoked/missing tokens return a
uniform 404. Completion permits a location-free summary until the shortened
expiration.

N06 permits at most 60 reads per token and 120 per viewer IP per minute.
Without trusted gateway CIDRs, the socket peer is the IP bucket; configure
this before gateway-based load acceptance. Malformed forwarding never grants
an arbitrary client-selected IP. Redis failure denies reads rather than
removing rate or nonce protection.

## Database and workers

Apply Customer 010/011, Vendor 000122, Dispatch 000065 and Hauling 025 using
their owning migration runners. Historical migrations were not edited.
Customer's runner replays `.sql` files and does not support paired down files;
its new DDL is repeatable. Restore a database backup rather than discarding
live share/session metadata during rollback. The paired source index and
Vendor checkpoint migrations include explicit down files.

Customer expires up to 200 sessions and observes up to 50 active sources per
sweep. Source calls have a five-second timeout. Hard expiration is checked on
every read even if a worker is behind. Dispatch/Hauling publish committed
state in cyclic keyset pages of 200, and repair source versions in pages of
100. Vendor repairs up to 50 recorded groupings per pass and preserves the
recorded mission order. A historical purchase with no recorded grouping event
needs a verified Vendor plan backfill; the worker does not guess one.

Private Customer metrics expose bounded operation/error/latency categories.
Taxi appends its safe read counters to the existing private metrics scrape.
Worker logs include outcome and repair counts; no capability appears in a
metric name, log path, or ledger. The gateway uses route templates and does
not log the public token path on forwarding errors.

## Reproducing local checks

Use disposable databases, never a shared operational database. Several legacy
Dispatch test packages truncate tables. Run those database packages serially
(`go test -p 1`), and use a clean database with the actual migration runner for
Taxi migration/outbox/reconciliation tests. Do not manually apply historic
DDL and then expect the runner's tracking table to be populated.

Set `CUSTOMER_TEST_DATABASE_URL`, `TAXI_TEST_DATABASE_URL`,
`DISPATCH_TEST_DATABASE_URL`, `HAULING_TEST_DATABASE_URL`,
`VENDOR_SERVICE_TEST_DATABASE_URL`, `MOVEMENT_TEST_DATABASE_URL` and
`JOURNEY_TEST_REDIS_ADDR` to isolated infrastructure for their corresponding
tests. Set only the applicable variables for each package group. The report
lists the executed packages and logs. Fixture-dependent source readers are
explicitly identified in tests and are not a full-service E2E certificate.

Before enabling either new flag, execute complete Ride/Send/Truck/Food/Shop
lifecycles through the running services and gateway, including source
assignment changes, token invalidation, completion, dependency outages,
customer auth and per-IP policy. Run sustained capacity testing with the
deployment's polling rate and latency budget, and a CGO-capable race build.
