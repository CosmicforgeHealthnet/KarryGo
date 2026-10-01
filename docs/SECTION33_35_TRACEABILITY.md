# Sections 33–35 — custody, recovery, and transfer closure

## Authority and pre-implementation gap matrix

| Authority | Section 33 | Section 34 | Section 35 | Gap closed here |
|---|---|---|---|---|
| Dispatch MOTORCYCLE | AVAILABLE | AVAILABLE | AVAILABLE | Normalized immutable custody contract and retained mature Dispatch handover/recovery |
| Dispatch Vendor-owned | PARTIAL | PARTIAL | PARTIAL | Added authoritative return handover and shared Dispatch custody/recovery/transfer operations |
| Hauling VAN/TRUCK | PARTIAL | PARTIAL | NOT_PRESENT | Added authority-local manifest, evidence, return, incident, replacement, transfer and allocation records |
| Taxi Goods CAR | PARTIAL | PARTIAL | PARTIAL | Extended Section 29 with the same authority-local safety guarantees |
| Vendor Commerce | orchestration only | projection only | orchestration only | No custody, credential, physical mission, or provider-transfer authority added |

## Section 33 — exact custody and evidence

`shared/go/custodyops` is a shared contract and repository implementation used
inside each execution authority's own database. It freezes a versioned manifest
against mission, tenant, business, branch, pickup stop, exact Parcel IDs and
package count. PostgreSQL immutability triggers reject update/delete after
freeze. Exact stop confirmation rejects cross-mission, cross-tenant,
cross-business, cross-branch, wrong-stop, stale-version, wrong-Parcel and wrong
package-count requests.

The current receiver challenge reference is frozen into the initial custody
state. Replacement must present that exact reference before the transaction
invalidates it; transfer then installs a distinct new receiver challenge.

Evidence is append-only and typed as `PICKUP`, `DELIVERY`, `DAMAGE`,
`DISCREPANCY`, `FAILED_ATTEMPT`, `RETURN`, or `PROVIDER_TRANSFER`. Only opaque
Media File Service references and relationship metadata are stored. Before a
record is accepted, Media read-back verifies active status, allowed owner,
mission, tenant, upload intent, and owner relationship. No media bytes are
stored in Dispatch, Hauling, Taxi, or Vendor.

## Section 34 — lifecycle, exceptions, and returns

The mature service-specific state machines remain authoritative; the custody
state is their normalized Commerce safety layer. It provides closed reason and
action vocabularies, state-safe policy validation, durable operations, and a
transactional outbox. `CONTROLLED_HOLDING` fails closed because no holding
facility is configured. Fee-bearing recovery requires both quote and approval
references and never mutates the frozen original quote.

Returns bind the original manifest/version, explicit return manifest, exact
Parcel subset, condition, destination, receiving actor, reason, category
policy, and RETURN evidence. Prepared food policies (`NO_RESTOCK` or
`DISPOSAL`) are distinct from retail return/inspection/quarantine policies.
Dispatch `vendorownedhandover` now creates and verifies a dedicated RETURN
challenge; Vendor merely relays the transient credential and reads the
canonical Dispatch result.

Alternative-receiver recovery requires an explicit new receiver plus distinct
old/new receiver challenge references. Reschedule records the requested time
and, when credential rotation is needed, requires its replacement reference.

## Section 35 — incidents, replacement, and transfer

Authenticated providers can report `BREAKDOWN`, `ILLNESS`, and
`SAFETY_INCIDENT` in Hauling and Taxi; Dispatch retains its mature provider
incident endpoint. A per-mission transaction lock moves custody to
`REPLACEMENT_REQUIRED`, and database completion guards block normal terminal
completion. Replacement assignment validates distinct providers, vehicle
class, capacity against the frozen package count, handling capability,
compliance, availability, manifest/version, and a replacement receiver
challenge.

Transfer requires a dedicated transfer reference, both provider confirmations,
exact location, manifest/version, exact Parcel set/package count, separately
typed transfer evidence, and a new receiver challenge. Completion atomically
closes the old provider and makes the replacement current. Cross-authority
requests additionally require old/new authority mission references, financial
allocation reference, and source event/version; Vendor never updates an
execution-service column or writes another authority's database.

Provider earning allocation is a policy-backed record only after operational
outcome. Allocations are unique per provider, cannot exceed approved earning,
carry one Wallet operation reference, and represent `APPLIED` or `UNKNOWN`
read-back state. The custody layer cannot debit a customer and cannot invent a
percentage when no policy exists.

## Security and event delivery

Internal routes use existing HMAC v2 method/path/body binding, exact caller
allowlists, freshness checks, correlation/idempotency, and a durable nonce
table. Every canonical mutation and the additional
`RETURN_HANDOVER_REQUIRED`/`PROVIDER_TRANSFER_REQUIRED` facts are written to a
transactional outbox before commit. Payloads contain references, never raw OTP,
QR credentials, authentication secrets, or media bytes.

## Migrations and contracts

- Dispatch: `000056_sections33_35_custody_recovery.up.sql` / `.down.sql`.
- Hauling: `017_sections33_35_custody_recovery.up.sql` / `.down.sql`.
- Taxi: `011_sections33_35_custody_recovery.up.sql` / `.down.sql`.
- Vendor: no migration; it remains relay/projection-only.
- OpenAPI contracts live with Dispatch, Hauling, and Taxi. Vendor deliberately
  has no custody OpenAPI operations.

## Frontend and scope boundary

No Provider, Customer, Vendor, or Admin frontend code changed. Future clients
must provide controlled camera/scan, exception selection, return proof, and
two-provider transfer screens against these backend policies.

**SECTION 37 — INTENTIONALLY SKIPPED / OUT OF CURRENT SCOPE**

No Section 41+ work was started.
