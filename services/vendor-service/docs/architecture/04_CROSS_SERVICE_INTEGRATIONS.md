# 04 — Cross-Service Integrations

Detailed per-service contracts live as package docs next to their client code — `internal/integrations/{wallet,verification,media,notification,support,admin,analytics,dispatch,taxi,hauling,handover}/doc.go` — so the contract is discoverable from the code that will implement it, not only from a document that can drift. This file is the index and the shared calling convention every one of those integrations must follow.

## Index

| Package | Destination | Current destination state | Status |
|---|---|---|---|
| `wallet` | `payment-wallet-service` | Vendor bank resolve/register/read-back added on the existing authority | **AVAILABLE** when HMAC deployment secret is configured |
| `verification` | `verification-compliance-service` | Six-dimension Vendor case/create/read/decision plus durable callbacks added | **AVAILABLE** when both services' HMAC/callback deployment config is present |
| `media` | `media-file-service` | Private object authority with tenant/owner-scoped read-back exists | **AVAILABLE**; Section 7 reuses read-back, deployment allowlisting required |
| `notification` | `notification-service` | Real | Contract documented, PENDING_EXTERNAL_DEPENDENCY: none |
| `support` | `support-dispute-service` | Real, but no vendor caller group yet | Contract documented, real work is PLATFORM_SERVICE_CHANGE_REGISTER (future) |
| `admin` | `admin-backoffice-service` | Pure scaffold | **NOT_PRESENT**; manual-review dependency remains explicit |
| `analytics` | `analytics-service` | Pure scaffold | Contract documented (outbound events only), nothing to call yet |
| `dispatch` | `driver-dispatch-delivery-service` | Real | Contract documented; custody confirmation depends on `handover` |
| `taxi` | `driver-taxi-service` | Pure scaffold, no goods-delivery concept | Contract documented as future GOODS_DELIVERY boundary only |
| `hauling` | `driver-hauling-service` | Real | Contract documented; custody confirmation depends on `handover` |
| `handover` | *(none — platform-wide gap)* | Does not exist anywhere in the platform | **PENDING_EXTERNAL_DEPENDENCY — LOGISTICS OTP/QR**, see [07](07_EXISTING_OTP_QR_INTEGRATION.md) |

## Section 7 contract correction

Vendor and Verification share one exact VerificationStatus vocabulary: `NOT_STARTED`, `IN_PROGRESS`, `PENDING_REVIEW`, `VERIFIED`, `REJECTED`, `EXPIRED`, `SUSPENDED`. No translation layer accepts plain `PENDING`. Verification callbacks target the direct-service root path `/internal/vendor/v1/verification-events`, consistent with the platform's `/internal/<domain>/v1` namespace, and remain absent from API Gateway.

## Shared calling convention (reused, not reinvented)

Every integration in this folder, once implemented, must use the platform's existing standards rather than a Vendor-Service-specific variant:

- **HMAC service auth**: `shared/go/serviceauth.SignRequest` / `NewVerifier`, signing as `"vendor-service"`. Destination services allowlist `vendor-service` in their own `<PREFIX>_SERVICE_SECRETS` — a config change on the receiving service, tracked in [PLATFORM_SERVICE_CHANGE_REGISTER.md](../PLATFORM_SERVICE_CHANGE_REGISTER.md) per integration.
- **Timestamp/skew validation**: inherited from `serviceauth.Verifier`'s default (5-minute skew), matching every other service.
- **Nonce/replay protection**: `shared/go/serviceauth` signs a nonce but does not consume it. Section 7 callback and Verification case routes therefore claim the nonce in their owning database; duplicates are rejected inside the signature freshness window.
- **Idempotency-Key**: `internal/platform/idempotency`, required on every outbound state-changing call (payment intents, mission creation, verification submissions).
- **Request fingerprinting**: `idempotency.Fingerprint(method, path, body)`, same as the key store's own conflict-detection.
- **Correlation ID / causation ID**: carried on `internal/events.Envelope` for every event; also propagated as request headers on synchronous internal calls once a feature needs distributed tracing across a multi-service workflow (e.g. checkout → wallet hold → fulfilment dispatch).
- **`expectedVersion` / optimistic concurrency**: any command that mutates a versioned aggregate (an order, a fulfilment plan) must accept and check `expectedVersion` against the stored version, following the version-guarded-`UPDATE`-returns-zero-rows convention already proven across the platform (hauling's `ResetToMatching`/`MarkAccepted` compare-and-set guards, fleet-partner-service's `repositories.ErrVersionConflict`). Not built yet — no versioned aggregate exists in this architecture phase.
- **Authoritative read-back after an unknown outcome**: if an outbound call's result is ambiguous (timeout, 5xx after the effect may already have happened), the integration must resolve state by reading the destination's own record via its own reference (payment intent by `source_service`+`source_reference`, mission by mission reference) before retrying or reporting failure — never assume success or failure from the transport error alone. Stated explicitly per integration in each `doc.go` (see `dispatch`/`hauling`).

## Section 6 result

No cross-service call was introduced. Media references are opaque and locally format-checked; there is no invented strong-validation call. Verification decisions remain external and non-writable by vendor actors. All Section 6 consistency rules are local same-database constraints or transaction checks.

## Section 7 preflight result

The formal classifications and exact allowed external changes were recorded in `PLATFORM_SERVICE_CHANGE_REGISTER.md` before external-service implementation. Verification and bank capabilities are partial and may be extended only at their owning services. Media reference read-back is available through `media-file-service` (the initial wrong-directory audit was corrected before any media edit); admin review remains absent. Notification delivery is partial (no SMS and Vendor caller deployment allowlisting outstanding). The API gateway has no general Vendor routing layer, so direct public Vendor routes remain the repository-consistent surface.

## Section 7 implemented result

Concrete HMAC clients now live in `internal/integrations/{verification,wallet,media}`. Verification owns cases, decisions, versions, history and callback outbox; Vendor combines signed callbacks with client-reference polling. Wallet owns resolution and the full bank record; Vendor stores a masked projection only. Media remains unchanged except for deployment allowlisting and provides tenant/owner-scoped object read-back. Admin review and notification delivery remain honest external dependencies.

## What remains outside Section 7

No commerce payment, settlement, catalogue, CRM, fulfilment, logistics, or logistics OTP/QR integration was introduced.

## Section 12B internal consumption contract

Section 12B introduces no outbound cross-service mutation. Vendor Service owns
Shop inventory and exposes availability/discovery plus reservation transitions
at `/internal/vendor/v1/shop`. These routes reuse HMAC v2, require an allowlisted
caller, bind the signature to the exact branch path, consume each nonce and
fail closed when the allowlist is empty. Catalogue and inventory revisions let
consumers detect stale snapshots.

The caller supplies an opaque source type/reference for reservation
idempotency; Vendor Service does not infer or create an Order. No Wallet,
payment, supplier, Dispatch, Taxi or Hauling call is made. Deployment must
configure approved callers and shared secrets before the internal surface is
usable; no public/mobile client may hold those credentials.
