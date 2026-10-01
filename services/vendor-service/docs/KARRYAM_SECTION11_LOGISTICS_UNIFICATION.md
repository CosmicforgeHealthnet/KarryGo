# KARRYAM Section 11 — Vendor / Commerce logistics unification

> Naming note. "Sections 10/11" in `SECTION10_11_HANDOFF.md` refers to Vendor
> Service's own internal section numbering (production, cloud kitchen,
> pastry). This document is about the **KARRYAM platform** Section 11, which is
> a different thing entirely: making Vendor treat DISPATCH, TAXI_GOODS and
> HAULING as first-class logistics fulfilment authorities under one contract.

## What this changed, and what it deliberately did not

Vendor already created missions against all three authorities. It already had
per-authority feature flags, HMAC-signed clients, an idempotent mission ref, a
recovery pass and an HMAC-protected event ingress that accepted all three
callers. None of that was rebuilt.

What it did not have was one vocabulary. Four features each defined their own
authority enum, and they agreed only by luck. Status handling compared raw
strings from three services with three different vocabularies, so a service
reporting its real state was silently filed as stale. Errors from three client
packages reached commerce as three unrelated shapes. And capability was
Vendor's own assumption rather than the authority's declaration.

Pricing is untouched. No fare, quote, estimate or pricing algorithm was
modified; capability answers what can be carried, never what it costs.

## The canonical layer

`internal/integrations/logistics` owns the vocabulary and imports no Vendor
feature, so every feature can depend on it without a cycle.

| File | Owns |
| --- | --- |
| `authority.go` | The canonical authority values and the accepted aliases. |
| `status.go` | The normalized execution status and each authority's mapping. |
| `errors.go` | The Vendor-safe failure categories and their classification. |
| `adapter.go` | The capability adapter interface, registry, gate and readiness. |
| `mission.go` | The conceptual mission intent and the execution identity. |

### Authority vocabulary (VND-002)

The canonical values are the shared platform ones:
`DISPATCH`, `TAXI`, `HAULING`, `VENDOR_OWN_DELIVERY`.

Two aliases are accepted on input and normalized on the way out:

- `TAXI_GOODS` — the product name for the Taxi goods authority. Every service,
  signature allowlist and persisted row already spells it `TAXI`; renaming it
  would be a cross-service data and contract migration for no behavioural
  gain, so the product name is accepted and the stored name is kept.
- `VENDOR` — what `fulfilmentgroup` persists for vendor-owned delivery, where
  `fulfilment` spells the same thing `VENDOR_OWN_DELIVERY`.

`TestEveryVendorAuthorityEnumSpellsTheSameAuthorities` and
`TestTheThreePlatformAuthoritiesAgreeAcrossEveryEnum` keep the feature-local
enums pinned to these values. The custody-source enum's `VENDOR` and
`PLATFORM` are deliberately outside the authority vocabulary: custody records
who observed a fact, which is a wider question than who is executing.

### Normalized execution status (VND-020)

Dispatch has nineteen lifecycle states, Taxi Goods fifteen and Hauling eleven.
All of them map into one commerce state, and the authority's own value is
still what gets stored — the normalization decides what happens, it does not
replace what was reported.

`PENDING`, `ASSIGNING`, `ASSIGNED`, `PROVIDER_EN_ROUTE`, `ARRIVED_PICKUP`,
`PICKUP_VERIFIED`, `IN_TRANSIT`, `ARRIVED_DELIVERY`, `DELIVERY_VERIFIED`,
`COMPLETED`, `CANCELLED`, `FAILED`, `RECONCILIATION_REQUIRED`.

An unmapped value is `StatusUnknown` and advances nothing. Guessing is how a
projection silently skips a custody transfer.

States carry a rank, so a repeated event advances nothing and a stale one
cannot walk a mission backwards. Terminal states are never overwritten, which
is what makes a duplicated completion callback safe.

#### What changed on the existing ingress, and what did not

The four collapsed event names every authority sends today — `PICKED_UP`,
`DELIVERING`, `DELIVERED`, `DELIVERY_FAILED` — normalize to exactly the states
the aggregate already moved to, so current traffic behaves identically. What is
new is that a richer spelling is now understood rather than filed as stale: a
Hauling `en_route_delivery` or a Taxi `AT_DROPOFF` advances the order instead of
being silently dropped.

Two deliberate consequences follow. A rich status that arrives genuinely out of
order now raises a conflict the caller retries, where it used to be swallowed
as stale — that is the correct handling, and no authority sends those spellings
on this boundary yet. And signal, timing and failure events are untouched:
they return before the status update, so normalizing their names could not and
does not move an order.

### Failure categories (VND-021)

`NOT_SERVICEABLE`, `CAPABILITY_MISMATCH`, `PROVIDER_UNAVAILABLE`,
`ASSIGNMENT_TIMEOUT`, `EXECUTION_CREATION_CONFLICT`, `CREDENTIAL_FAILURE`,
`DEPENDENCY_UNAVAILABLE`, `RECONCILIATION_REQUIRED`.

Each has a customer-safe sentence, and that sentence is the only text that
leaves the package. The originating error stays reachable through `Unwrap` for
logs, so an internal host, table or stack never reaches a commerce response.

An error that matches no sentinel and no known refusal word becomes
`RECONCILIATION_REQUIRED`, not a failure. An unrecognised error after a create
is exactly the case where an execution may already exist, and assuming failure
there is how a second vehicle gets dispatched. A create timeout is classified
the same way for the same reason.

### Capability and serviceability (VND-005 … VND-009)

Each authority serves the shared contract at
`GET  <api-base>/internal/commerce/logistics-capability` and
`POST <api-base>/internal/commerce/logistics-capability/serviceability`,
behind that service's existing internal-commerce boundary — HMAC v2, caller
allowlist and durable nonce replay protection. It is a separate path from
Taxi's older, Taxi-shaped `/internal/commerce/capabilities`; two contracts on
one path would either collide at registration or answer the wrong shape.

Declared capability is audited, not assumed:

| Authority | Multi-pickup | Max pickups | Dropoffs | Secure pickup/delivery |
| --- | --- | --- | --- | --- |
| Dispatch | yes | 12 | 1 | yes |
| Taxi Goods | no | 1 | 1 | yes |
| Hauling | yes | 10 | 1 | yes |

Two rules in the matcher are worth stating because they are easy to get
backwards:

- An undeclared bound is **unknown and skipped**, never zero capacity. A
  capability document that omits a maximum weight does not mean the authority
  can carry nothing.
- A protected handover can never downgrade. An authority that cannot verify a
  delivery credential is refused a mission that requires one, rather than
  completing it unverified.

### The gate, and where it deliberately does not fail closed

Before a mission is created, the chosen authority is asked whether it can
execute this requirement. A declared incapability becomes a `REFUSED` mission
carrying the authority's own reason code, so commerce records something it can
show a customer instead of retrying a mission that can never succeed.

An authority that cannot be **asked** is treated differently from one that
says no. An unreachable capability endpoint produces no refusal: it is not
evidence the authority cannot carry the load, and failing the order there would
turn a capability-service outage into a failed paid order. Mission creation
remains the real gate.

### Readiness (VND-031)

`GET /internal/vendor/v1/logistics/readiness` reports, per authority: enabled,
configured, reachable, when it was checked, and a reason code. A disabled or
unconfigured authority is reported without being contacted — Vendor has no
business probing an authority it may not send work to. The surface is
HMAC-protected and read-only.

An authority this deployment does not use is not a readiness problem; only an
enabled one that cannot be reached is.

### Feature flags (VND-032)

No new flag names. The existing ones remain the single source of truth, and
config validation already refuses each one without its credentials:

- `DISPATCH_COMMERCE_INTEGRATION_ENABLED` + `VENDOR_DISPATCH_URL`/`_SECRET`
- `TAXI_GOODS_DELIVERY_ENABLED` + `VENDOR_TAXI_URL`/`_SECRET`
- `HAULING_COMMERCE_INTEGRATION_ENABLED` + `VENDOR_HAULING_URL`/`_SECRET`

A disabled authority gets no capability client at all, which is what makes
"never select an authority whose integration is disabled" structural rather
than a runtime check somebody can forget. It is never silently replaced by a
different authority.

## Vendor never selects a provider (VND-003)

Vendor may say "this order uses Hauling". It may not say "use provider 123".

`TestNoVendorMissionRequestCarriesProviderSelectionAuthority` walks
`logistics.MissionIntent`, `orders.MissionRequest`, `commercemission.Request`,
`dispatchcommerce.MissionCreateRequest` and `dispatchcommerce.QuoteRequest`
recursively and fails on any field naming a provider, driver, rider, vehicle,
fleet member or assignee. `TestTheProviderSelectionBanIsNotVacuous` proves the
walker would actually catch one, so the rule cannot quietly stop enforcing
anything.

Provider identity is late-bound: the execution service or Fleet resolves it,
and Vendor persists it when the execution service reports it.

## Vendor is not a second completed-work producer (VND-026)

`TestVendorIsNotASecondCompletedWorkProducer` (in `internal/`) forbids Vendor
from importing `shared/go/providercompat` or `shared/go/fleetproviderwork`, and
from addressing Fleet's completed-work route. The executing service owns
physical completion; if Vendor reported it too, one execution would produce two
completed-work records and a managed provider could be compensated twice.

## Section 12 handoff (VND-027, VND-028)

No financial settlement was implemented here. Two things are recorded for
Section 12 rather than fixed:

- The commerce delivery fee is attributed at quote time, before any provider is
  known. Vendor's fulfilment plan freezes what the customer pays for delivery;
  it does not, and at that point cannot, split that amount between a provider
  and an organization.
- Vendor's Wallet payment intent settles to a single provider per VendorOrder.
  A managed-provider delivery whose compensation must be divided between the
  provider and their organization has no representation in that contract today.

Both are contract-compatibility observations, not defects introduced here.
