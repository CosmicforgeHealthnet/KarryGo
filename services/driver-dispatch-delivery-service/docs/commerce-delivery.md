# Commerce dispatch delivery

Dispatch is the canonical authority for route-aware motorcycle Commerce
quotes and runtime delivery missions. Vendor Service supplies immutable
commercial plan evidence and retains only `PlatformMissionRef` projections;
it never owns provider assignment, trip state, custody, route evidence or the
Dispatch mission identifier.

## Contract and authority

The machine contract is
`docs/openapi/commerce-dispatch-delivery.openapi.json`. Internal routes use
HMAC v2, a five-minute timestamp window, a durable single-use nonce and the
exact `vendor-service` caller allowlist. Mutation keys are also domain
operation references. Reusing a key with a different request fingerprint is a
conflict; ambiguous responses are resolved through operation or mission
read-back before any retry.

Quotes call the configured route provider for ordered leg evidence. A missing
or failed route authority is unserviceable; straight-line distance is never a
price authority. `FASTEST`, `BALANCED` and `SAVER` are explicit modes. Quote
creation neither creates a booking nor reserves a provider.

Mission creation binds all of these immutable values:

- fulfilment plan reference, version, fingerprint and mission slot;
- quote id, revision and request fingerprint;
- ordered logical pickup stops and their ready windows;
- receiver location, service class and mode;
- Vendor parcel references, immutable manifests and handling requirements;
- motorcycle capability version and the already-approved funding reference.

Foreign, expired or mismatched evidence fails closed. One mission produces one
normal public-marketplace booking and is paid as one mission. It is never
turned into a Fleet private order and no Commerce-only provider pool exists.

## Multi-pickup and custody

Pickup order is persisted and deterministic. Co-located Vendors may share a
physical stop reference, but keep separate logical stops, manifests, parcel
sets and handover challenges. A provider cannot complete stop N+1 before N.
Parcel identity remains the opaque Vendor-issued reference throughout pickup,
provider replacement, transfer, delivery, return and event replay.

Pickup completion confirms the exact Parcel set, package count and manifest
version for that stop. Actual custody handover freezes both stop and Parcel
manifest bindings. Damage and discrepancy therefore append operational
evidence rather than rewriting the pickup record.

Provider eligibility reuses the normal Dispatch marketplace and its current
provider, registration, insurance, availability, organization and vehicle
checks. Mission-level capacity and handling constraints are checked against
the current motorcycle capability before creation and again before assignment.

## Recovery and lifecycle

No current provider is a durable queued state, not an immediate terminal
failure. The recovery worker uses persisted attempts, bounded exponential
backoff and `promised_by` as the SLA ceiling. Each retry, assignment,
exhaustion, replacement, receiver-unavailable, reschedule, return and
completion advances a source version and writes lifecycle evidence through the
transactional outbox.

Before custody, provider replacement creates another marketplace attempt.
After custody the old provider remains custodian until a replacement completes
the provider-bound transfer challenge; a plain provider-id reassignment is
forbidden. The transfer preserves every Parcel reference and manifest version,
closes and opens immutable carrier-history entries, invalidates the old trip
challenge and regenerates the receiver challenge.
Receiver unavailability can be rescheduled or converted into a return leg;
neither action rewrites the original delivery history.

Dispatch publishes targeted mission, stop and parcel events to Vendor Service.
Delivery is at least once. Vendor validates the source, schema, payload hash,
mission operation and parcel references against its own projection, then
ignores duplicate or stale versions. Dispatch supplies failure evidence but
does not decide or execute customer refunds and does not mutate Vendor payable.

## Configuration and operations

Required for the capability:

- `VENDOR_SERVICE_SECRET` on Dispatch equals `VENDOR_DISPATCH_SECRET` on Vendor;
- `DISPATCH_ROUTE_SERVICE_URL` points to the road-route authority;
- `HANDOVER_CODE_SECRET` is stable across Dispatch replicas;
- `VENDOR_SERVICE_URL` and `DISPATCH_COMMERCE_EVENT_DELIVERY_ENABLED=true`
  enable lifecycle delivery;
- Vendor enables `DISPATCH_COMMERCE_INTEGRATION_ENABLED` and, after migration
  rollout, `DISPATCH_MULTI_PICKUP_SUPPORTED`.

Keep event delivery disabled until Vendor's callback is deployed. Monitor
pending/failed targeted outbox rows, queue age, assignment attempts,
`NO_PROVIDER_QUEUED`, `SLA_EXHAUSTED`, route-provider failures and handover
failure codes. Do not manually replay a mutation; read its stable operation
first.
