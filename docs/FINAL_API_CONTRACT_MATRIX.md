# ART-002 — API and DTO conformance

Phase baseline `3007d6d42b60bf256578cf2688fe9e330a4a2798`.

Section 20 is an audit. The provider surfaces were built in Sections 8–10 and
the credential and capability contracts in Sections 15–18; nothing here was
rebuilt. Every route below was read out of the registration code, and the ones
marked LIVE answered on a booted binary.

## 1. API-001 … API-015

| ID | Outcome | Evidence |
|---|---|---|
| API-001 corporate session projection | ALREADY SATISFIED | Each service projects corporate context through its own `internal/apiresponse`: Taxi `middleware.go` sets `KeyManaged` from `corporate.IsManagedProvider`, Dispatch `privacy.go` and Hauling `middleware.go` do the equivalent. Fleet entities are never serialized; the projection carries managed flag, membership status, organization context, service eligibility, operating mode and the visibility policy. |
| API-002 organization offers | ALREADY SATISFIED | Identical four routes in all three: `GET /organization-offers`, `GET /organization-offers/:offerId`, `POST …/accept`, `POST …/decline`. Taxi in `fleetprovider/http.go`, Dispatch in `marketplace/provider_http.go`, Hauling through the shared `fleetproviderwork` facade. The LIST reads are the reconnect-recovery path; notification is never the only discovery mechanism. |
| API-003 company assignments | ALREADY SATISFIED | Same four shapes at `/company-assignments`. Certified LIVE on Hauling: the LIST route is mounted and refuses an anonymous caller (401) rather than 404. |
| API-004 assignment verification | ALREADY SATISFIED | `providercompat.AuthorizeManagedOperation` is the single fail-closed gate: service category, authenticated provider, membership status, organization binding and assignment-provider match. Client-supplied organization and membership are hints, never authority — certified by `TestClientSuppliedOrganizationCannotBeSpoofed` and `TestClientSuppliedMembershipCannotBeSpoofed`. |
| API-005 completion projection | ALREADY SATISFIED | All three submit `providercompat.CompletedWorkRequest`. There is no service-specific corporate completion DTO; the twelve-pairing spoofing matrix runs against the one contract. |
| API-006 financial projection | ALREADY SATISFIED | `providercompat.FinancialVisibilityPolicy` decides per field class. `ClassOrganizationOnly` and `ClassPlatformOnly` are `AlwaysConcealed()` — unreachable under any policy. Concealed fields are **absent**, not returned with a flag. |
| API-007 commerce execution | ALREADY SATISFIED | Vendor reads capability from the authority (`logisticscapability`), freezes the authority, and never names a provider — enforced by the Section 11 reflective structural test over every mission request type. Provider identity is late-bound through the custody chain. |
| API-008 DTO allowlists | ALREADY SATISFIED | Provider responses are built by the per-service `apiresponse` projections from explicit field sets. No repository row or ORM entity is serialized to a provider. |
| API-009 typed enums | ALREADY SATISFIED | Status, operating mode, authority, membership state, credential purpose, custody state, reconciliation state and financial visibility are all typed: `logisticscapability.Authority`, `custodychain.CredentialPurpose`/`Party`/`Operation`, `providercompat.MembershipStatus`/`FinancialFieldClass`, and the database CHECK constraints behind each. |
| API-010 versioning | ALREADY SATISFIED | Optimistic version on the mutable write workflows: assignments and offers (`expectedVersion` on accept/decline), compensation plans and assignments, organization settings, reconciliation operations, Vendor missions and parcels. Immutable read models do not carry a decorative version. |
| API-011 timestamps | ALREADY SATISFIED | Internal contracts are UTC and it is enforced, not merely documented: Fleet's completed-work ingestion rejects a non-UTC `completedAt` outright (`"completedAt must be UTC"`). Storage is `timestamptz` throughout. |
| API-012 money | ALREADY SATISFIED | Integer minor units end to end — `*_minor` / `*_kobo` `BIGINT` columns and `int64` in Go. No float appears in any monetary path. Currency is explicit on every monetary contract and CHECK-constrained to `^[A-Z]{3}$` on the settlement instruction. |
| API-013 error envelope | ALREADY SATISFIED | `shared/go/httpx` renders `{success, error:{code, message, retryable, request_id, fields, details}}`. The cause chain is `json:"-"` and never serialized; a 5xx logs only status and code, so an upstream body carrying a credential cannot reach either the response or the log. |
| API-014 internal contract ownership | ALREADY SATISFIED | Every internal route carries an explicit caller allowlist, a versioned path (`/internal/<domain>/v1/...`), typed request and response, an HTTP client timeout, classified retry semantics (`providercompat.FailureKind.Retryable`, `SubmitOutcome.Retryable`) and an idempotency key on every state-changing call. See §2. |
| API-015 contract tests | ALREADY SATISFIED | Contract tests exist for each critical contract; see §3. |

## 2. Critical internal contract matrix

| Contract | Server | Caller | Auth | Route | Request | Response | Idempotency | Version | Retry |
|---|---|---|---|---|---|---|---|---|---|
| Corporate context | Fleet | Taxi, Dispatch, Hauling | HMAC v2 + allowlist | `GET /internal/fleet-partner/v1/corporate-context` | service + providerId | `providercompat.CorporateContext` | read-only | context `Version` | retryable only on `FLEET_DEPENDENCY_UNAVAILABLE` |
| Offers / assignments relay | Fleet | Taxi, Dispatch, Hauling | HMAC v2 + allowlist | `/internal/fleet-partner/v1/company-assignments/…`, `…/organization-offers/…` | typed decision | typed assignment/offer | `clientResponseId` | `expectedVersion` | bounded, conflict is terminal |
| Private assignments relay | Fleet | Hauling | HMAC v2 + allowlist | `/internal/fleet-partner/v1/private-assignments/:id/{accept,decline}` | typed decision | typed assignment | `Idempotency-Key` | `expectedVersion` | bounded |
| Private execution | Taxi, Dispatch, Hauling | Fleet | HMAC v2 + allowlist | `/internal/private-executions` | typed execution | typed outcome | operation ref | source version | bounded |
| Completed work | Fleet | Taxi, Dispatch, Hauling | HMAC v2 + `callerOwnsSource` | `POST /internal/fleet-partner/v1/completed-work` | `CompletedWorkRequest` | `CompletedWorkAck` | `idempotencyKey` | `sourceVersion` | `SubmitOutcome.Retryable()` |
| Managed settlement | Wallet | Fleet only | HMAC v2 + caller narrowing | `POST /internal/finance/v1/managed-settlements` | instruction | posting result | `(instructionId, instructionVersion)` + key | instruction version | read-back on ambiguity |
| Settlement read-back | Wallet | Taxi, Dispatch, Hauling, Fleet | HMAC v2 + allowlist | `GET …/managed-settlements/by-job/{service}/{job}` | path only | `managedsettlement.Outcome` | read-only | — | failure is `Unknown`, never "unsettled" |
| Logistics capability | Dispatch, Taxi Goods, Hauling | Vendor only | HMAC v2 + nonce + allowlist | `GET <api>/internal/commerce/logistics-capability[/serviceability]` | requirement | `logisticscapability.Capability` | read-only | capability `Version` | none needed |
| Commerce mission | Dispatch, Taxi Goods, Hauling | Vendor only | HMAC v2 + allowlist | `/internal/commerce/missions` | `commercemission.Request` | `Mission` | operation ref + fingerprint | source version | bounded |
| Commerce handover | Dispatch, Taxi Goods, Hauling | Vendor only | HMAC v2 + allowlist | `GET/POST <api>/internal/commerce/missions/:ref/handover/…` | credential confirmation | `Observation` | `Idempotency-Key` | `SourceVersion` | read-back on `ErrUnknown` |
| Media authorization | Media | execution services, Vendor | HMAC v2 + `owner_service` binding | `POST /objects/:id/download-authorizations` | typed grant request | signed, expiring grant | idempotency key | object version | bounded |
| Verification result | Verification | Vendor, execution services | HMAC v2 + nonce + freshness | `POST /internal/vendor/v1/verification-events` | typed event | ack | event id + source version | source version | durable, dead-lettered |
| Support / safety references | Support | execution services (identity), apps | HMAC v2 (internal), bearer (apps) | `/internal/<service>/internal/providers/:id`, `/provider`, `/customer` | typed lookup | minimal identity | operation ref | — | best-effort, never blocking |
| Operator outbox repair | Dispatch | Fleet, admin console | HMAC v2 + allowlist | `GET/POST /api/v1/internal/dispatch/v1/operations/outbox[/:id/requeue]` | filter / path | operator rows | event identity preserved | aggregate version preserved | requeue is redelivery |

## 3. Contract test coverage (API-015)

| Contract | Test |
|---|---|
| Fleet corporate context | `shared/go/providercompat` client + `VerifyManagedContext` suites; `membership_test.go` |
| Assignments / offers | `fleetproviderwork` facade tests; Taxi `fleetprovider/enforcement_test.go`; Hauling `route_collision_test.go` (both families mount distinctly) |
| Private execution | Fleet `private_execution_worker_test.go`; Hauling `privateexecution` suite |
| Completed work | Fleet `sections8_9_scenario_matrix_test.go`; `section20_spoofing_matrix_test.go` (12 wrong pairings) |
| Managed settlement | Wallet `managed_settlement_test.go` (15 DB-backed certifications) + `managed_settlement_load_test.go` |
| Wallet read-back | `shared/go/managedsettlement` tests; live HMAC probe from the previous phase |
| Logistics capability | `shared/go/logisticscapability` contract + roundtrip tests; per-authority declaration tests; certified LIVE on Hauling and Dispatch |
| Commerce handover | `shared/go/custodychain` direction matrix; Vendor `integrations/handover/direction_test.go`; per-authority secure-handover suites |
| Media authorization | media-file-service `mediaobjects` suites (grant issue, expiry, deletion blocked under retention and legal hold) |
| Verification result | verification-compliance `internal/features/vendors/integration_test.go`; Vendor Section 7 callback suite |
| Support / safety | support-dispute `handler_test.go` and repository integration suite |
| Outbox metrics config | `shared/go/outboxmetrics/sources_test.go` — each service's compiled configuration is executed against its real table |

## 4. Provider-facing surface matrix

| Surface | Taxi | Dispatch | Hauling |
|---|---|---|---|
| Corporate context on session | yes (`apiresponse` middleware) | yes (`apiresponse/privacy.go`) | yes (`apiresponse` middleware) |
| Organization offers list / detail / accept / decline | yes | yes | yes (shared facade) |
| Company assignments list / detail / accept / decline | yes | yes | yes (shared facade) |
| Private-assignment relay | — | — | yes, at `/provider/private-assignments` |
| Canonical completed-work submission | yes | yes | yes |
| Financial projection under policy | yes | yes | yes |
| Commerce pickup credential (provider-held) | yes (Taxi Goods) | yes | yes |
| Direct pickup credential | customer-held, provider-verified | provider-held, customer-verified | provider-held, customer-verified |
| Delivery credential (receiver-held) | yes | yes | yes |
| Operator outbox inspect / requeue | `/internal/fleet/completed-work` | `/internal/dispatch/v1/operations/outbox` | `/internal/fleet/completed-work` |
| Outbox + reconciliation metrics | `<api>/outbox-metrics` | service `observability` registry | `<api>/metrics` |
