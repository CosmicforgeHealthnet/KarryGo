# ART-003 — Security and authorization

Phase baseline `9ebad225ea9ec25397778a4dc8d3ff842f2938a3`.
No secret value appears in this document or in any certification output.

**Phase update — one deliberate authorization change.**
`support-dispute-service` was added to Fleet's
`PROVIDER_MEMBERSHIP_ALLOWED_CALLERS` so a provider-raised safety case can
record which organization it happened under. It is a **read** of the same
corporate-context contract the three execution services already call, over the
same HMAC v2 signing, with no write access and no Fleet database access — the
existing SEC-018 statement that Support touches no execution-service table is
unchanged. It is recorded here rather than left in a diff because widening an
allowlist is a security decision, not a wiring detail. Support still holds
exactly one Fleet capability, and the resolver is inert until a deployment
supplies the URL and secret.

## 1. SEC-001 … SEC-019

| ID | Outcome | Evidence |
|---|---|---|
| SEC-001 fail closed | ALREADY SATISFIED | `providercompat.AuthorizeManagedOperation` never returns a usable context with an error and never reports "independent" because Fleet was unreachable: an unclassified read failure becomes retryable `FLEET_DEPENDENCY_UNAVAILABLE`, so the caller answers 503. `TestFleetOutageIsNotAMembershipRefusal`. |
| SEC-002 caller authentication | ALREADY SATISFIED | Every internal route sits behind `serviceauth` HMAC v2. There is no universal bypass: each route group builds its own verifier from the secrets it needs. Certified LIVE — an unsigned read of Hauling's capability is 401, a provider token on an internal route is 401. |
| SEC-003 source ownership | ALREADY SATISFIED | `callerOwnsSource` on completed-work ingestion. Twelve wrong pairings refused, including Vendor and Wallet impersonating an execution service and Fleet claiming its own job (`TestCrossServiceCompletedWorkSpoofingIsRefused`), with zero rows written on a refusal. |
| SEC-004 provider object authorization | ALREADY SATISFIED | Provider reads and writes are scoped to the authenticated provider at the repository query, not filtered afterwards: `GetByIDForProvider`, `ReadMissionForProvider`, `lockProviderMission`, `GetByIDForCustomer`. Handover credentials are additionally bound to the assigned provider inside the keyed hash. |
| SEC-005 organization authorization | ALREADY SATISFIED | Organization comes from the authoritative membership, never the request body. `TestClientSuppliedOrganizationCannotBeSpoofed` submits a real provider's completion under a foreign organization and it is refused with nothing written. |
| SEC-006 financial authorization | ALREADY SATISFIED | Section 14's policy is the only gate, and the organization and platform classes are unreachable under every policy. Closed on four alternate routes: Wallet provider surface (no settlement route exists on it), notifications (`notifications.Validate` refuses a provider-addressed payload carrying a concealed class), error payloads (cause chain never serialized), and **logs** — see SEC-007. |
| SEC-007 log redaction | **DONE this phase** | `logging.SafeFields` already redacted by secret-sounding key and by non-scalar type. Neither caught corporate economics: `organizationShareMinor` is an ordinary name holding an ordinary integer. A redactor hook now lets `providercompat` register the financial registry, so every service's logs conceal `ClassOrganizationOnly` and `ClassPlatformOnly` while keeping ids, outcomes and the provider's own figures. Four tests including nesting and spelling variants. |
| SEC-008 error redaction | ALREADY SATISFIED | `apperrors.Error.Cause` is `json:"-"`; `httpx.RespondError` logs only status and code for a 5xx. An upstream body carrying a token cannot reach a client or a log line. |
| SEC-009 least privilege | ALREADY SATISFIED | See §2. Each internal group builds a verifier holding only the secrets it needs, then narrows again with a per-operation allowlist. Certified LIVE: Fleet is refused Vendor's commerce capability (403) and Vendor is refused Dispatch's outbox (403), although both are legitimate callers of other routes on the same services. |
| SEC-010 secret management | ALREADY SATISFIED | Every secret is read from the environment (`*_SERVICE_SECRETS`, `*_TOKEN_SECRET`, `HANDOVER_CODE_SECRET`, `*_METRICS_AUTH_TOKEN`). Development defaults are clearly-labelled development strings, not production material. No secret value is printed by any certification tool in this phase. |
| SEC-011 rotation readiness | ALREADY SATISFIED (documented) | `serviceauth.ParseSecrets` takes a comma-separated `caller=secret` map and the verifier accepts any entry, so one caller can hold two secrets simultaneously. Rotation is therefore overlap-capable per caller: add the new secret to the callee's map, restart the callee, switch the caller's signing secret, restart the caller, remove the old entry. Signing is single-secret per caller, so the caller's own restart is the cut-over point. Procedure in `FINAL_DEPLOYMENT_READINESS_CHECKLIST.md`. |
| SEC-012 credential rate limiting | ALREADY SATISFIED | Every verification surface is bounded, and bounded recoverably. Per challenge: 5 verification attempts then LOCKED, 5 regenerations, 45s resend cooldown — a ceiling of 25 guesses per booking against a six-digit code. Dispatch and Taxi additionally hold a shared Redis window (`securehandover.RedisRateLimiter`, `booking/verification.RateLimiter`). No path locks a legitimate completion permanently: regeneration always exists, and the operator recovery path exists beyond that. |
| SEC-013 one-time credentials | ALREADY SATISFIED | Single ACTIVE challenge per context enforced by partial unique index; terminal states cannot be reactivated (database trigger); the keyed hash binds service, stage, challenge ref, version, booking, provider, owner and expiry, so a credential is mission- and purpose-scoped by construction. `custodychain` records `SingleUse` for every purpose and a test fails if one becomes reusable. |
| SEC-014 handover fail-closed | ALREADY SATISFIED | An unavailable credential dependency returns `OTP_DEPENDENCY_UNAVAILABLE` (503). There is no branch that completes a protected handover without verification; Vendor's `handover.Unavailable` authority cannot manufacture one or fall back to local verification. |
| SEC-015 audit history | ALREADY SATISFIED | Append-only history for operating mode and settings (`version` + settings history), membership (`fleet_partner_organization_provider_memberships` + history), compensation plan and rule versions, compensation assignments, payroll approval, settlement instructions and reversals (compensating journals, never deletions), and operator repair (`requeue_count`, `last_requeued_at`, `last_requeued_by`). |
| SEC-016 admin actions | ALREADY SATISFIED | `fleet_partner_finance_operations` and the operations tables carry `requested_by_actor_type`, `requested_by_actor_id`, operation type, payload, `expected_version`, correlation and causation. Dispatch requeues record the acting service. |
| SEC-017 media security | ALREADY SATISFIED | Retrieval requires a signed, expiring download authorization scoped to the object and the calling service; `owner_service` must match the authenticated caller. Deletion is blocked under retention or legal hold and the block is recorded with its reason. |
| SEC-018 support / safety | ALREADY SATISFIED | support-dispute-service writes only `complaints`, `disputes`, `complaint_events`, `complaint_evidence`, `support_chat_messages`, `support_case_outbox`. It has no statement touching any trip, booking or mission table in any execution service. |
| SEC-019 security test matrix | ALREADY SATISFIED | See §3. |

## 2. Caller → route permission matrix (SEC-009)

Secret values are never shown. "—" means the caller has no route on that service.

| Callee | fleet-partner-service | vendor-service | admin-backoffice-service | api-gateway | operations-service | support-dispute-service | execution services |
|---|---|---|---|---|---|---|---|
| **fleet-partner-service** | — | — | operations, outbox replay, reconciliation cases, worker operations | — | — | — | completed work (own source only), private-execution callbacks, corporate context |
| **payment-wallet-service** | managed settlement post + reverse | commerce funding, delivery recovery plan | operator replay, reconciliation | — | — | dispute reversal | settlement read-back only (own job) |
| **driver-taxi-service** | private execution, restrictions, completed-work inspect | commerce quote, mission, capability, custody | — | — | operations surface, custody | provider identity | — |
| **driver-dispatch-delivery-service** | marketplace internal, authority, restrictions, private execution, operator outbox | commerce quote, mission, capability, multi-stop, handover | operator outbox, admin internal | partner internal | operations, custody | provider identity | — |
| **driver-hauling-service** | fleet authority, restrictions, private execution, completed-work inspect | commerce quote, mission, capability, handover | — | — | operations, custody | provider identity | — |
| **vendor-service** | — | — | admin internal | — | — | commerce support cases | commerce lifecycle ingress |
| **media-file-service** | — | media objects | — | — | — | — | media objects (own `owner_service`) |
| **notification-service** | notifications | notifications | — | — | — | notifications | notifications |

## 3. Security test matrix (SEC-019)

| Attack | Result | Evidence |
|---|---|---|
| Wrong provider (B uses A's assignment) | REFUSED | `MarkAccepted` compare-and-set on provider id; `TestTransitionMustAgreeWithTheRecordedHolder` |
| Wrong provider (B uses A's handover credential) | REFUSED | Credential hash binds `ProviderID`; `TestActorAuthorizationIsObjectScoped` |
| Wrong organization | REFUSED | `TestClientSuppliedOrganizationCannotBeSpoofed` |
| Wrong membership | REFUSED | `TestClientSuppliedMembershipCannotBeSpoofed` |
| Wrong service (Taxi claims Dispatch work) | REFUSED | `TestCrossServiceCompletedWorkSpoofingIsRefused`, 12 pairings |
| Expired assignment | REFUSED | Guarded reset + `ResetToMatching` CAS; `TestMatch_AcceptAtTimeoutBoundaryNotClobbered` |
| Suspended membership, new work | REFUSED | `TestSuspendedExpiredAndTerminatedAreRefusedAndDistinct` |
| Terminated membership, new work | REFUSED | Same; DECLINED / CANCELLED / ENDING / REMOVED all classify TERMINATED |
| Suspended membership, work in custody | ALLOWED TO COMPLETE (documented policy) | `TestSuspensionDuringWorkStillRecordsTheCompletion`; placement enforced by `TestCorporateGateStaysOnAcquisitionPathsOnly` |
| Provider token on internal route | REFUSED 401 | Certified LIVE on Hauling; `TestOperatorOutboxRefusesUnauthorizedCallers` |
| Internal caller on unauthorized route | REFUSED 403 | Certified LIVE: Fleet → Vendor capability, Vendor → Dispatch outbox |
| OTP brute force | BOUNDED | 5 attempts then LOCKED, 5 regenerations, 45s cooldown, plus Redis window on Dispatch and Taxi |
| Credential replay after custody transfer | REFUSED | `TestSecurePickupOTPAndQRShareSingleUseLifecycle`; terminal states cannot be reactivated |
| Credential after provider reassignment | REVOKED | `TestProviderReassignmentBeforePickupRevokesTheOldCredential` (`PROVIDER_REASSIGNED`) |
| Unauthorized media read | REFUSED | Signed grant required, scoped to object and caller service |
| Immutable evidence deletion | REFUSED | `PhysicalDeletionBlocked` under retention or legal hold, with reason recorded |
| Wallet financial leakage | REFUSED | Organization and platform classes `AlwaysConcealed()`; no settlement route on the provider surface (404, certified live) |
| Notification financial leakage | REFUSED | `notifications.Validate` refuses concealed classes to a provider recipient |
| **Log financial leakage** | **REFUSED (closed this phase)** | `providercompat` registers the financial registry with the platform logger |
