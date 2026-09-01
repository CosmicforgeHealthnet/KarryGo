# Dispatch security, observability and certification

## Baseline and pre-change control inventory

2026-08-31; staging; HEAD `2e46bcd5b557f4ae559668d519993f84401dd5f0`. Dirty work preserved; index empty. Migration tips: Dispatch 000051, Vendor 000109, Analytics 002. No rollout flags enabled.

This initial inventory records inspection, not a claim of executed certification. Final results follow after implementation/testing.

| Requirement | Existing implementation/files | Existing evidence | Before repair |
|---|---|---|---|
| 17.1 secure OTP | securehandover/crypto.go: crypto/rand challenge + HMAC PRF, keyed hash, constant-time compare | crypto_test.go; secure_handover_concurrency_test.go | PASS (inspection; DB gated) |
| 17.2 purpose/key separation | securehandover namespace vs auth OTP; HANDOVER_CODE_SECRET vs OTPSecret | crypto_test.go; config tests | PARTIAL: reject equal/development keys outside development |
| 17.3 QR binding/replay | securehandover repository stage/context/manifest/version/expiry | crypto/contract tests; PostgreSQL lifecycle tests | PASS (inspection; DB gated) |
| 17.4 distributed limits | securehandover Redis atomic Lua; DB attempts/cooldown | ratelimit_test.go | PARTIAL: central issue and per-context limits need complete coverage |
| 17.5 secret logging | shared/httpx and Dispatch logger; auth usecase | source search | MISSING: development auth logs raw OTP/contact; shared panic recovery can dump request/panic |
| 17.6 safe errors | shared/apperrors and HTTP envelopes | handler tests | PARTIAL: review dependency error/log paths |
| 17.7 receiver privacy | scoped Trip/booking reads, minimized lifecycle adapters | request and Trip tests | PARTIAL: pre-acceptance detail exposes receiver contact |
| 17.8 managed financial privacy | Fleet membership-summary visibility policy; company-specific views | providercompat/marketplace tests | MISSING on generic provider Trip/response path |
| 17.9 emergency exception | deliveryrecovery.RecordEmergencyRecovery, separate append-only action and HMAC admin route | recovery migration/contract tests | PASS state guard/audit; does not mark a challenge consumed |
| 17.10 object authorization | authenticated Customer/Provider plus repository predicates; HMAC source families | request/trip/assignment/planning/securehandover/router tests | PASS inspected paths; final negative matrix pending |
| 17.11 Partner secrets | Gateway owns key; Dispatch takes signed service context | shared serviceauth/partner contract tests | PASS inspected boundary |
| 17.12 media relation | router media_attachment_authorizer; securehandover capture/evidence checks | cross-mission Media tests | PASS inspected boundary |
| 17.13 Wallet authority | shared/walletclient; opaque reconciliation | booking settlement/hold tests | PASS inspected boundary |
| 17.14 private serialization | Challenge and Trip credential fields json:"-" | TestChallengeAndOutboxRepresentationsDoNotLeakSecrets | PASS inspected boundary |
| 18 metrics | canonical outbox/incident backlog logs; Fleet dependency-free registry | Fleet observability tests | PARTIAL: outcome/duration/queue/handover/dependency coverage absent |
| 18 structured safe logs | shared/logging, httpx; Fleet redaction pattern | logging tests/source scan | PARTIAL: body/panic/contact gaps above |
| 19 tests | existing Dispatch 4–16 suite; canonical env-gated PG helpers | baseline test inventory | PARTIAL: add only uncovered negative/telemetry cases and audit skips |

Canonical PostgreSQL environment is required for database execution; no substitute database is authorized.

## Final outcome and evidence limits

Sections 17–19 implementation is complete in this working tree; PostgreSQL and live deployment certification are **not** complete. Security, privacy and durable observability certification are PARTIAL until the database-backed controls execute. Available non-database tests pass after the specific regression correction described below. No Section 20 work was started.

The first inventory above was provisional. Further inspection found three additional real gaps: legacy Trip/Commerce OTP plaintext persistence, an unkeyed secret-bearing idempotency fingerprint, and missing expected-version validation for emergency recovery. These were corrected, not reclassified away.

Evidence vocabulary: **executed** means the test body ran successfully in this session; **DB gated** means the test compiled/discovered but its database body did not execute; **inspection** does not substitute for runtime certification. Local HTTP mocks/miniredis are contract evidence, not live service or distributed deployment certification.

### Repository and change boundaries

- Branch: `staging`; HEAD: `2e46bcd5b557f4ae559668d519993f84401dd5f0`.
- Initial status: 141 dirty/untracked entries; no staged changes. Existing work preserved.
- Dispatch migration tip: 000051 → 000052. Vendor 000109 and Analytics 002 unchanged.
- Production changes: Dispatch, shared HTTP/logging/metrics, and Fleet's metrics compatibility alias. No Vendor, Analytics, Gateway, Wallet, Media or Notification production changes.
- No HTTP routes added or removed. Emergency recovery request now requires `expected_trip_version`; its existing OpenAPI contract and route register were updated.
- Existing scoped Commerce router test enumerates 20 operations; this is **not** a claimed global Dispatch route count.
- New production directories: `shared/go/observability`, `services/driver-dispatch-delivery-service/internal/observability`. The shared registry is the existing Fleet implementation promoted for reuse, not a second metrics framework.
- No branch/index/commit/push operations; no old SQL migration altered by this session.

## Section 17 final control matrix

Paths below are relative to the Dispatch service unless prefixed `shared/`, `Gateway`, `Vendor` or `Fleet`. Feature package names expand to `internal/features/<package>`.

| Control | Implementation / authority | Concrete evidence | Result |
|---|---|---|---|
| 17.1 cryptographic OTP | securehandover/crypto.go; existing shared handover verification | TestCredentialCipherProducesBoundIndependentCredentials; TestLegacyCompatibilityStoresOpaqueReferenceNotOTP | Executed PASS; DB storage guard gated |
| 17.2 auth/handover key separation | config/config.go rejects identical keys, short handover keys and development/debug production configuration | TestHandoverKeyMustBeSeparateAndProductionSafe | Executed PASS |
| 17.3 QR context / single use | keyed opaque QR; repository binds stage, source, Trip, provider, recipient and immutable manifest version; migration047 guards terminality | TestContextKeyBindsStageManifestAndVersion; OTP/QR consumption and identity-change PostgreSQL tests | Unit PASS; DB gated |
| 17.4 atomic abuse limits | existing RedisRateLimiter Lua; issue/verify/resend principal and context dimensions | TestRedisRateLimiterIsSharedBoundedAndWindowed; TestRedisConcurrentAttemptsHaveOneSharedAllowance; TestMissingLimiterAndDisabledQRFailClosed | Miniredis PASS; deployed Redis not tested |
| 17.5 no secret logs | shared/logging/structured.go; auth usecase removes OTP/contact logs; HTTP recovery no longer dumps panic/request | TestSafeFieldsRedactsNestedSecretsAndOpaqueObjects; TestMiddlewareNeverLogsCredentialQueryBodyHeaderPanicOrCause | Executed PASS |
| 17.6 safe HTTP errors | existing envelope/code authority; raw cause omitted; route template and safe correlation only | HTTP middleware security test and corrected auth/http suite | Executed PASS |
| 17.7 stage-scoped receiver access | request list/detail/push blank receiver identity/contact before acceptance; apiresponse/privacy.go blanks terminal receiver contact and standalone proof contact | request phase6fgh/phase6ijk, Trip phase7cde; TestProviderViewsConcealManagedFinanceAndTerminalContact | Executed PASS |
| 17.8 managed financial concealment | existing Fleet membership-summary read-back before provider mutation; generic financial fields null, not zero | TestProviderPrivacyAuthorityFailurePrecedesDomainAction; privacy response test; existing marketplace provider reader tests | Unit/contract PASS; managed-mode rollout and live Fleet dependency remain |
| 17.9 emergency action | existing admin HMAC route + actor/reason + expected Trip version; immutable emergency action, no fabricated challenge consumption | TestEmergencyRequiresActorReasonAndExpectedVersionBeforeMutation; TestEmergencyRecoveryVersionAuditDoesNotVerifyHandoverPostgres | Unit PASS; DB audit/version proof gated |
| 17.10 object authorization | existing Customer/Provider auth + scoped repositories; source-authorized internal families | TestActorAuthorizationIsObjectScoped; ownership tests in booking/request/trip; authority/migration negatives | Non-DB PASS; DB predicate/race execution gated |
| 17.11 Partner secrets | Gateway owns API keys/auth; Dispatch signed source boundary; no Partner OTP API added | Gateway TestAPIKeyNotEchoedInResponses, missing-key and route tests; shared/serviceauth | Executed contract PASS; live Gateway not tested |
| 17.12 Media relation | router/media_attachment_authorizer.go + Media authority read-back; capture/mission/parcel context | TestMediaAttachmentAuthorizerMissionAndParcelBinding; TestOperationalEvidenceMediaAuthorizationFailsClosed; TestProviderTransferUsesSameMediaAuthorizationBoundary | Executed mock-contract PASS |
| 17.13 payment authority | existing shared/walletclient; booking hold and Trip settlement read-back; no Dispatch ledger | TestSettlementWorkerRecoversTimeoutWithoutNewIdentity; TestUnknownHoldDoesNotStartMatchingOrClaimFailure | Executed mock-contract PASS; financial DB races gated |
| 17.14 secret serialization/storage | secret DTO fields json:-; keyed secret-command fingerprint; opaque legacy handover references; migration052 CHECKs | TestChallengeAndOutboxRepresentationsDoNotLeakSecrets; TestSecretTransportIntentJSONExcludesCredentials; TestSecretBearingIdempotencyFingerprintIsKeyed; migration shape | Executed non-DB PASS; migration runtime gated |

### OTP, QR and migration compatibility

- Random references use `crypto/rand`: 18 random bytes, base64url encoded; not timestamp/math/rand codes.
- OTP uses existing HMAC-SHA256 PRF over random challenge reference and version, rendered as six decimal digits. Stored verification is keyed HMAC with constant-time `hmac.Equal`.
- Namespace is `dispatch-secure-handover-v1`; separate purposes include `otp-value`, `qr-token`, `credential-hash:OTP/QR`, `recipient`, and `request-fingerprint`. Authentication OTP uses a separately configured key.
- QR is opaque reference + version + 24-byte truncated HMAC; **never OTP or receiver PII**. Server lookup binds the remaining mission/provider/recipient/manifest/stage context.
- New challenge TTL defaults to 900 seconds; legacy compatibility TTL stays 15 minutes. Resend cooldown defaults to 45 seconds. Existing maximum attempts and database expiry/lock controls remain authoritative.
- Consumption, resend supersession and identity invalidation are transactional. A replay of the same idempotent operation is a read-back, not another consumption.
- Secret-bearing command fingerprints are keyed: a public hash of a six-digit-code request would otherwise enable offline guessing.
- Legacy Trip and Commerce flows now store a `handover_<opaque-reference>`, derive the code only after the existing authorized credential lookup, and retain existing keyed verification hashes. They do not create another challenge engine.
- Generic lifecycle notifications and operation read models do not receive raw codes. The dedicated, authenticated Notification handover transport transiently receives code/QR for delivery; that exception is not a generic event.
- Reassignment/receiver mutation invalidates active challenges through the existing database trigger. Exact race execution remains blocked, not silently certified.

### Rate-limit dimensions and failure behavior

| Operation | Server dimensions | Allowance / existing control |
|---|---|---|
| Issue pickup | provider | 10/minute |
| Central challenge issue | booking, intended recipient, source type/ref | 20/minute per dimension |
| Verify | customer or provider; source caller/mission; challenge | 20/minute principal; 30/minute source/challenge |
| Resend | authenticated actor; authorized booking/recipient/source | 5/10 minutes per dimension plus 45-second default persisted cooldown |
| Incorrect credential | authoritative challenge | Existing persisted attempt ceiling and lock |
| Auth | existing auth OTP limiter | Preserved; not replaced by handover limiter |

Redis operations are atomic and shared across clients. Missing/outage limiter returns controlled unavailable, not allow. Exceeded budget is 429; feature/dependency unavailable is 503. Miniredis concurrent/window tests executed; live Redis multi-instance certification was not performed.

### Privacy and authorization views

| View / route family | Authority and allowed scope | Minimized / concealed |
|---|---|---|
| Customer booking/handover | authenticated owner, repository booking/customer match | Own authorized credential only; another customer's booking/credential denied |
| Provider inbox / request detail | authenticated candidate/offer scope | Receiver name/phone blank before acceptance; no secret material |
| Provider active Trip | assigned provider | Operational pickup/dropoff/contact necessary for fulfilment, not full customer profile |
| Provider terminal Trip / proof | same authorized provider read | Receiver name/phone/email blank; proof is not a contact directory |
| Organization-managed provider | Fleet membership-summary via existing authenticated client | Generic fare/price/commission/payable/margin/settlement/earnings/allocation fields null; authoritative stored amounts unchanged |
| Fleet/private | organization/member and existing provider eligibility boundaries | Existing company-specific visibility preserved; no duplicate membership domain |
| Internal Commerce / Vendor | HMAC v2 allowed caller + source/mission ownership | Opaque references and minimized lifecycle data; no arbitrary Trip/OTP read |
| Partner/Gateway | Gateway API key authority → existing Dispatch internal source contract | API key never forwarded into Dispatch domain data or generic response |
| Admin/emergency | allowlisted admin service, operator, reason, expected version and idempotency | Separate auditable exception; no ability to assert OTP verified |
| Analytics | minimized event adapter, existing ingest authority | No receiver secrets/contact or raw private-media URL |

`PROVIDER_ORGANIZATION_MODE_ENABLED=false` preserves legacy independent-provider rollout. Managed-provider traffic requires that existing mode to be enabled with the Fleet membership reader configured. Once enabled, reader failure or unknown provider mode fails closed before a domain mutation. This report does **not** claim that a deployment admitting managed traffic while disabling its authority integration is safe.

### Emergency recovery

The existing trusted admin/internal route remains the only entry point. Body role/admin booleans have no authority. An actor reference, bounded reason/note, Idempotency-Key and positive `expected_trip_version` are mandatory. Repository locking compares current Trip version; duplicate same-operation results read back before stale-version checks. The emergency audit action never mutates a handover challenge to CONSUMED or fabricates ordinary delivery evidence. PostgreSQL test checks stale version rejection, replay, audit DELETE rejection and zero challenge consumption.

## Section 18 metric inventory

The dependency-free registry formerly used by Fleet is shared; Fleet preserves its public types through an alias. Dispatch samples its **own** durable state every `DISPATCH_METRICS_INTERVAL_SECONDS` (default/minimum 30 seconds) with a five-second collection deadline, and emits structured metric snapshots through existing logging. No public metrics route or second event system was added.

| Semantic metric | Actual family | Bounded labels / meaning | Evidence |
|---|---|---|---|
| Assignment offers | dispatch_assignment_offer_total | strategy, outcome, service_area | Durable current-status count, including legacy broadcast inbox |
| Offer response latency | dispatch_offer_latency_seconds_mean; dispatch_offer_latency_seconds_bucket | strategy/outcome or outcome/le; 10,30,60,90,300,+Inf seconds | Durable response-time distribution |
| Queue depth / wait | dispatch_queue_depth; dispatch_queue_wait_seconds | outcome, service_tier, service_area | Actual active Queue rows; oldest wait, zero buckets |
| Handover verification | dispatch_handover_verification_total | stage, method, outcome | In-process counter |
| Handover resend | dispatch_handover_resend_total | stage, reason=requested, outcome | In-process counter |
| Delivery attempts | dispatch_delivery_attempt_total | outcome, recovery_action | Durable current-state count |
| Route alerts | dispatch_route_alert_total | type, severity, service_area | Durable alert count |
| Replacement | dispatch_replacement_total | reason, outcome | Durable incident recovery count |
| Outbox backlog / lag | dispatch_event_outbox_depth; dispatch_event_outbox_lag_seconds | target, outcome | Pending/failed/in-flight/dead-letter/published; oldest pending age |
| Outbox retries / delivery | dispatch_event_outbox_retry_total; dispatch_event_delivery_total | target, outcome | Durable attempts; live accepted/duplicate/rejected/unknown/retryable outcomes |
| Booking notification backlog | dispatch_booking_notification_backlog; dispatch_booking_notification_oldest_seconds | target, outcome | Existing durable notification intents |
| Reconciliation | dispatch_reconciliation_backlog; dispatch_reconciliation_oldest_seconds | worker, outcome | Wallet hold/settlement, handover notification, replacement, scheduled recovery |
| Exhaustion / retry totals | dispatch_reconciliation_exhausted; dispatch_reconciliation_attempts_total | worker, outcome | Manual-review/dead-letter and durable attempt totals |
| Workers | dispatch_workers_running; dispatch_worker_last_run_timestamp; dispatch_worker_runs_total; dispatch_worker_processing_seconds_count / _seconds_sum | worker, outcome where relevant | Start/stop-once, heartbeat, outcome and duration |
| Dependencies | dispatch_dependency_requests_total; dispatch_dependency_latency_seconds_count / _seconds_sum | dependency, outcome | Existing HTTP transport; Wallet/Notification/Media/Vendor/Fleet/Analytics/routing |

Evidence: `TestMetricsBoundLabelsAndRecordFailuresConcurrently`, `TestDependencyMetricsDoNotInspectURLBodyOrHeaders`, `TestDurableMetricsCoverSemanticBacklogsWithBoundedLabels`, `TestWorkerRunningStopsExactlyOnce`, shared registry snapshot/concurrency tests: executed PASS. `TestDurableTelemetryEmptyBacklogsPostgres`: DB gated.

Important semantics:
- Persisted-state families are gauges even where existing semantic names end in `_total`; they are not monotonic event counters. Historic row deletion/retention can reduce them.
- Latency buckets include rows with a recorded response timestamp, not invented timestamps for older incomplete rows.
- This model has no stable normalized service-area identifier on all sampled authorities. `service_area=unpartitioned` is one explicit bounded bucket; no exact coordinates, inferred region, provider ID or entity ID is used. Geographic breakdown is not claimed.
- Gauge snapshots replace old samples to remove stale series. Failed queries preserve previous completed samples and report collector failure; heartbeat/outcome exposes staleness.
- Dependency timing measures the existing HTTP round trip to response headers, not application-level financial success or full body-processing time.
- No OTP/QR/hash, raw URL, phone/email, SQL or unbounded exception text appears in metric labels.

### Logs, errors and worker recovery visibility

Existing logging now supports structured safe fields: request/correlation, event reference, operation/worker, attempt, controlled outcome/error code and status where present. UUID request IDs remain readable; custom request IDs still echo to clients for compatibility but are hashed for logs. HTTP logs use route templates, not raw paths/query strings. Panic and application-cause values are not serialized.

Sensitive key names are recursively redacted; opaque structs/errors/Stringers are not blindly printed. This is defense in depth for audited call sites, not a claim that arbitrary strings under misleading keys can be automatically identified. Dependency errors log controlled categories; existing client/state authorities retain reconciliation semantics.

Assignment, handover/notification, recovery, route, outbox delivery, relay, Wallet hold/settlement and collector workers expose running state, last-run time, outcomes and processing time. Backlog queries expose pending, unknown, retries and exhaustion without per-event full-table scans.

## Section 19 coverage matrix

E = existing test; N = new this session. PG = canonical PostgreSQL required. The result column deliberately does not turn skipped database cases into PASS.

| Requirement | Test / package | Result / environment |
|---|---|---|
| Group/Mission/Parcel/Challenge legal and illegal transitions | E deliveryfoundation.TestStateTransitionsAndCompletionInvariant | PASS, none |
| Offer transitions/ranking/decline/expiry/eligibility/exhaustion | E assignment/policy_test.go, operation_readback_test.go; migration hard-race tests | Unit PASS; PG gated |
| Attempt/recovery/route state rules | E deliveryrecovery/policy_test.go; routeintegrity policy/offline tests; recovery concurrency tests | Unit PASS; PG gated |
| Serviceable/unserviceable/restricted/prohibited/degraded/vehicle/weight | E serviceability; missionplanning.TestSection6RouteAuthorityFailClosedAndDegradedPolicy, planner policy tests | Unit PASS; serviceability repository PG gated |
| Quote expiry/fingerprint/owner | E booking.TestProtectedCreateBindsQuoteAndReplaysOneBooking; serviceability and commercequote tests | Unit PASS; DB constraints gated |
| Issue / OTP / QR / resend / expiry / lock | E securehandover crypto/ratelimit; N security controls and expiry/attempt/resend-vs-verify PG tests | Non-DB PASS; challenge transactions PG gated |
| Invalidate / reassign / receiver / manifest / transfer / return | E identity-change, operational transfer, delivery-recovery tests; N reassignment-vs-verification race and adversarial manifest tests | Unit/context PASS; PG gated |
| Exact manifest, split, wrong stop, custody uniqueness | E missionplanning and securehandover context tests; TestPlannedMissionCanonicalExecutionManifestVersionAndCustody; TestAssignmentAndManifestHardRaceConstraints | Unit PASS; PG gated |
| Receiver unavailable, grace, contacts, retry/reschedule/alternate receiver | E recovery policy and TestDeliveryRecoveryActionRaceAndAppendOnlyContact | Unit PASS; PG gated |
| Return / hub / breakdown / replacement / exhaustion | E TestReturnRouteReusesSection6ZoneAndRoadAuthorities; TestConfiguredHubAuthorityFailsClosed; TestHubCheckpointRequiresExactManifestScan; replacement recovery tests | Unit PASS; PG gated |
| Emergency authorization / audit / no false verification | N emergency unit and PostgreSQL tests | Unit PASS; PG gated |
| Migration up/down/reapply/existing-row compatibility | E migration chain tests; N migration052 shape and active-legacy drain test | Shape PASS; PG gated |
| Exclusive offer / canonical Booking-Trip | E TestAssignmentAndManifestHardRaceConstraints; TestPlannedMissionCanonicalExecutionManifestVersionAndCustody | PG gated; mock concurrent accept PASS is not DB proof |
| OTP and QR race | E TestSecureHandoverOTPAndQRRaceConsumesOnce | PG gated |
| Resend vs verify | N TestSecureHandoverResendVersusVerifyRacePostgres | PG gated |
| Reassignment vs verify | N TestHandoverReassignmentVersusVerificationRacePostgres | PG gated; either ordering valid, old credential rejected after reassignment commit |
| Active custody race | E assignment/planning hard constraint and operational manifest tests | PG gated |
| Domain/outbox rollback and dedupe | E Trip/request event tests, lifecycle notification intent, Vendor projection transaction tests | PG gated |
| Idempotency/fingerprint | E booking/assignment/offline/manifest; N keyed credential fingerprint | Unit PASS; PG uniqueness gated |
| Legacy Customer flags-off booking | E booking.TestCreateBookingHappyPath and existing request/Trip/router tests | Local service/HTTP PASS; not live customer deployment |
| Partner create/read/status/webhook/auth | E Gateway partner/http, partner/webhooks, partnerauth | Local contract PASS; no Gateway production edit |
| Fleet/private acceptance / eligibility / concealment | E provider reader/responder/private tests; N apiresponse privacy; Fleet TestAcceptanceToOneCanonicalTrip | Mock/unit PASS; canonical cross-service PG harness skipped |
| Vendor create unknown outcome/read-back/cancel/lifecycle | E Vendor cmd/dispatch_delivery_adapter_test.go; orders/dispatch_lifecycle_events_test.go | Targeted local contract result recorded below; PG lifecycle test gated |
| Notification secret/minimal intent | E securehandover notification tests, booking.TestLifecycleNotificationTimeoutRetriesSameIdentityAndMinimizes | PASS, mock transport |
| Cross-mission/parcel Media | E router Media binding tests | PASS, authenticated mock read-back |
| Wallet lost response / same operation / reconciliation | E booking hold/settlement, Trip settlement worker and shared/walletclient | PASS, mocks; ledger/reconciliation races PG gated |
| Weak-network scan/evidence/challenge/completion/offline replay | E operational manifest/evidence and route offline tests; keyed operation test | Unit PASS; persisted exactly-once PG gated |
| Secret-free logs/events/metrics/API | N logging/httpx/privacy/metric tests; E challenge DTO/outbox contract | PASS, none/miniredis |
| Safe default and representative enabled features | E config, assignment, planning, handover, recovery, route and notification policy tests | PASS; no global deployment flags changed |

### Exact PostgreSQL inventory (representative required invariants)

All names below are discoverable under Dispatch `migrations`, except separately qualified tests. None of their PostgreSQL bodies executed here.

- Migration chain/rollback: `TestLegacyCredentialIsolationApplyRollbackReapplyPostgres` (N), `TestSecureHandoverMigrationApplyRollbackReapplyAndConstraints`, `TestOperationalMigrationApplyRollbackReapply`, `TestLifecycleMigrationApplyRollbackReapplyPostgres`, `TestReliabilityMigrationApplyRollbackReapplyAndSecretGuard`.
- Offers/manifest constraints: `TestAssignmentAndManifestHardRaceConstraints`.
- Booking/Trip, stable manifest/custody: `TestPlannedMissionCanonicalExecutionManifestVersionAndCustody`.
- OTP/QR one consumption: `TestSecureHandoverOTPAndQRRaceConsumesOnce`.
- Resend/verify: `TestSecureHandoverResendVersusVerifyRacePostgres` (N).
- Reassignment: `TestSecureHandoverIdentityChangeInvalidatesActiveChallenge`; `TestHandoverReassignmentVersusVerificationRacePostgres` (N).
- Wrong customer/manifest/replayed QR: `TestHandoverAdversarialObjectManifestAndReplayPostgres` (N).
- Expired/locked challenge: `TestHandoverExpiryAndAttemptCeilingPostgres` (N), two env-gated subtests.
- Evidence/custody: `TestSecureDeliveryEvidenceGatesCompletionAndIsAppendOnly`, `TestOperationalManifestFreezeAndEvidenceIdempotencyRaces`, `TestOperationalTerminalAndTransferSerializationRaces`.
- Recovery/audit: `TestDeliveryRecoveryActionRaceAndAppendOnlyContact`, `TestEmergencyRecoveryVersionAuditDoesNotVerifyHandoverPostgres` (N).
- Offline/version/idempotency: `TestRouteVersionAndOfflineCommandConcurrency`.
- Notification/domain atomicity: `TestLifecycleNotificationIntentAtomicRollbackAndDedupPostgres`, `TestVendorLifecycleProjectionIsTransactionBoundPostgres`, existing Trip/request event emission tests.
- Reconciliation leases/races: `TestSettlementReconciliationConcurrentClaimAndLateWorker`, `TestOutboxReconciliationLeaseFencesLateFailure`, `TestHandoverNotificationReconciliationClaimRace`, `TestQueueWalletReconciliationClaimRace`, `TestScheduledRecoveryActivationRace`.
- Replacement: `TestIncidentRecoveryClaimReplayAcceptanceAndNoSecondTripPostgres`, `TestReplacementCancelledOrExpiredNeverCreatesOfferPostgres`.
- Empty durable metrics: `TestDurableTelemetryEmptyBacklogsPostgres` (N).
- HMAC replay: authority package `TestAuthorityRejectsAReplay`.
- Live local Fleet/Dispatch HTTP + PG harness: Fleet integration `TestAcceptanceToOneCanonicalTrip`, Dispatch router child `TestAcceptanceToCanonicalTripDispatchHarness`.

### Exact execution record

Commands executed from repository root with command-local `GOCACHE=C:\KarryGoPush\.codex-cache\go-build`. Log files are local regenerable artifacts, not application source.

1. Targeted stabilized Dispatch run:
   `go test -count=1 ./services/driver-dispatch-delivery-service/internal/features/securehandover ./services/driver-dispatch-delivery-service/internal/features/commercemission ./services/driver-dispatch-delivery-service/internal/features/trip ./services/driver-dispatch-delivery-service/internal/features/booking ./services/driver-dispatch-delivery-service/internal/features/request ./services/driver-dispatch-delivery-service/internal/features/deliveryrecovery ./services/driver-dispatch-delivery-service/internal/observability ./services/driver-dispatch-delivery-service/internal/config ./services/driver-dispatch-delivery-service/internal/router ./services/driver-dispatch-delivery-service/migrations`
   All 10 packages PASS; env-gated DB tests skipped. Log `.codex-cache/security-targeted-final.log`.

2. **One full uncached Dispatch regression**:
   `go test -count=1 -json ./services/driver-dispatch-delivery-service/...`
   Exit **1**: raw top-level results 1076 pass, 12 fail, 129 skip across 39 packages (28 pass, one failed, ten no-test packages). All 12 failures were auth/http custom request-ID echo compatibility introduced by UUID-only normalization. Corrected middleware preserves bounded printable caller IDs in responses and hashes non-UUID IDs only for logs. The full command was **not rerun**.
   Log `.codex-cache/security-full-dispatch.jsonl`; stderr empty.

3. Targeted regression correction:
   `go test -count=1 -json ./services/driver-dispatch-delivery-service/internal/features/auth/http ./services/driver-dispatch-delivery-service/internal/router ./shared/go/httpx`
   Exit 0; 61 top-level pass, one subprocess-only harness skip. All 12 prior failures now PASS. Log `.codex-cache/security-regression-correction.jsonl`.
   This is full-suite evidence plus targeted correction, **not a claim that the original full command passed**.

4. Shared/Fleet affected contracts:
   `go test -count=1 -json ./shared/go/httpx ./shared/go/logging ./shared/go/observability ./shared/go/serviceauth ./shared/go/handover ./shared/go/providercompat ./shared/go/walletclient ./shared/go/mediaclient ./shared/go/notifications ./services/fleet-partner-service/internal/observability`
   Exit 0; 30 top-level pass, two Fleet database pool metric tests skip. Log `.codex-cache/security-shared-contracts.jsonl`. HTTPx was also rerun after the compatibility correction.

5. Final targeted collector/recovery/schema check:
   `go test -count=1 ./services/driver-dispatch-delivery-service/internal/observability ./services/driver-dispatch-delivery-service/migrations ./services/driver-dispatch-delivery-service/internal/features/deliveryrecovery ./shared/go/httpx ./services/fleet-partner-service/internal/observability`
   All five packages PASS, PG cases skipped. Log `.codex-cache/security-final-focused.log`.

6. Final added DB-test discovery:
   `go test -count=1 -v -run 'Test(LegacyCredentialIsolation|SecureHandoverResend|HandoverAdversarial|HandoverReassignment|EmergencyRecoveryVersion|DurableTelemetryEmpty|HandoverExpiry)' ./services/driver-dispatch-delivery-service/migrations`
   Exit 0; migration shape executed PASS; six top-level PostgreSQL cases and two expiry/attempt subtests skipped. Log `.codex-cache/security-final-postgres-discovery.log`. The expiry/attempt parent reports PASS despite skipped children; **not DB certification**.

7. Fleet cross-service harness:
   `go test -count=1 -v -run '^TestAcceptanceToOneCanonicalTrip$' ./services/fleet-partner-service/internal/integration`
   Exit 0 but selected test **SKIPPED**, Dispatch DB absent. Log `.codex-cache/security-fleet-blackbox.log`.

8. Gateway Partner contracts:
   `go test -count=1 -json ./services/api-gateway/internal/features/partner/http ./services/api-gateway/internal/features/partner/webhooks ./services/api-gateway/internal/features/partnerauth`
   Exit 0. Log `.codex-cache/security-partner-contracts.jsonl`. These are local contracts, not live Partner certification.

9. Vendor Dispatch adapter/lifecycle contracts:
   `go test -count=1 -json -run '^TestDispatch(Adapter|Lifecycle)' ./services/vendor-service/cmd ./services/vendor-service/internal/features/orders`
   Log `.codex-cache/security-vendor-contracts.jsonl`; final counts recorded in verification addendum below. No broad Vendor/CRM regression.

10. Native race detector:
    `go test -race -count=1 ./services/driver-dispatch-delivery-service/internal/features/securehandover ./services/driver-dispatch-delivery-service/internal/features/assignment ./services/driver-dispatch-delivery-service/internal/observability ./shared/go/observability`
    Exit 2: `-race requires cgo`. Host CGO_ENABLED=0 and no gcc. **BLOCKED_BY_TEST_INFRASTRUCTURE**, not PASS. Log `.codex-cache/security-race.log`.

11. Static:
    `go vet ./services/driver-dispatch-delivery-service/... ./services/fleet-partner-service/... ./shared/go/httpx ./shared/go/logging ./shared/go/observability`
    and `go build` with those same paths: both exit 0. Logs `.codex-cache/security-vet.log`, `security-build.log`.
    `gofmt -l` over the 56 session Go files: no output; newly added race test also formatted. Final diff check below.

### Skip audit and missing environment

Full Dispatch raw top-level skips: authority 19; marketplace 43; privateexecution 6; request 6; restrictions 2; serviceability 1; Trip 15; verification 1; migrations 35; router 1.

- 128 top-level skips require PostgreSQL.
- One top-level router helper is started only by the Fleet integration parent, which was separately invoked and skipped for missing Dispatch DB.
- **Eight additional subtests skipped**: four provider relay decisions, two replacement cancelled/expired cases, two challenge expiry/attempt cases. Their three parent containers report PASS without running the DB assertions. They are not counted as certified security behavior.
- The late-added reassignment/verify race is additionally discoverable and skipped; it was not present in the single full-run counts.
- Shared/Fleet: two PostgreSQL pool-observation tests skipped. No hidden feature-disable skip is presented as security PASS.
- Native race toolchain is a separate blocked command, not a Go test skip.
- No live external smoke test was performed.

Unset canonical variables checked:
`DISPATCH_TEST_DATABASE_URL`,
`DISPATCH_TRIP_TEST_DATABASE_URL`,
`DISPATCH_REQUEST_TEST_DATABASE_URL`,
`DISPATCH_AVAILABILITY_TEST_DATABASE_URL`,
`DISPATCH_PROFILE_TEST_DATABASE_URL`,
`DISPATCH_VEHICLE_TEST_DATABASE_URL`,
`DISPATCH_VERIFICATION_TEST_DATABASE_URL`,
`DISPATCH_VERIFICATION_EVENT_TEST_DATABASE_URL`,
`TEST_DATABASE_URL`,
`FLEET_PARTNER_TEST_DATABASE_URL`,
`VENDOR_SERVICE_TEST_DATABASE_URL`,
`ANALYTICS_SERVICE_TEST_DATABASE_URL`,
`ANALYTICS_TEST_DATABASE_URL`.
`TEST_REDIS_URL` is also unset; only miniredis ran. No substitute database was created or contacted.

### Feature flags / rollout

All current safe-default feature gates remain off: route serviceability/package enforcement/degraded mode; sequential matching/queue/organization assignment/admin-assisted/multi-stop; secure challenge/QR/pickup-delivery evidence gates; recovery/return/hub/replacement; route alerts/offline replay; Vendor/Analytics event-delivery rollout gates. Existing aliases remain. Legacy broadcast stays the default.

Representative enabled-policy tests cover assignment/admin/Fleet membership, consolidation/split/capacity/route fail-closed, handover QR disabled/unavailable limits and recovery/route policies. Database-backed enabled end-to-end tests remain gated. This is not an exhaustive production flag-combination deployment test.

New config is `DISPATCH_METRICS_INTERVAL_SECONDS` (30 seconds default, collector minimum 30). Production requires a distinct non-development handover key and disabled OTP debug. Managed-mode requires existing Fleet URL/caller/secret configuration. SMS remains disabled unless an actual approved transport is deployed; no SMS delivery certification is claimed.

## Migration 000052: necessary credential-storage correction

Files:
- `migrations/000052_legacy_handover_secret_isolation.up.sql`
- `migrations/000052_legacy_handover_secret_isolation.down.sql`

Reason: old Trip pickup/delivery codes, Commerce pickup fields and credential-operation snapshots could retain raw OTP despite the secure challenge table storing only hashes. Application redaction alone did not enforce safe persistence.

UP:
1. Rejects application while active unverified legacy numeric Trip/Commerce/transfer credentials remain.
2. After those drain, erases obsolete historical raw credential fields only; preserves Trip, mission, transfer, challenge hashes and non-secret operation history.
3. Adds reference-only CHECK constraints on Trip pickup/delivery columns, Commerce stop handover_code and targeted Commerce operation snapshot fields.

DOWN drops only these new guards. It does not delete tables or restore erased secrets. All earlier SQL files remain untouched by this session.

Required staged deployment: arrange reference-writing binaries before the credential-isolation migration, allow active old credentials to drain, then apply 052. If deployment automatically applies migrations before startup, coordinate a staged release/migration procedure; this report does not invent a bypass flag. Do not clear active OTPs to force migration success. Preserve the handover key during rollout; key rotation needs its existing operational plan.

Shape test PASS. Apply/rollback/reapply and existing-row compatibility test are written but **BLOCKED_BY_TEST_INFRASTRUCTURE**. No database cleanup/update was executed in this session.

## Frontend and operations handoff (documentation only)

- Treat blank pre-acceptance/terminal receiver fields as intentional minimization.
- Concealed managed-provider money is null, not a zero balance/earning.
- Use existing authorized credential endpoints; generic lifecycle events are not code delivery.
- Handle 429 cooldown/budget responses and 503 dependency/disabled-feature responses safely.
- Emergency callers send current expected_trip_version and handle version conflict; they cannot assert handover verified.
- Media evidence must retain authorized mission/parcel/session binding.
- Consume metrics with documented gauge semantics and collector freshness; unpartitioned is not a real geographic zone.
- No frontend implementation or new public route.

## Remaining work classification

**Implementation gaps:** none identified in repaired Sections 17–19 scope.

**Certification gaps:** canonical PostgreSQL migration, IDOR, challenge/custody/offer/reconciliation/outbox races and empty durable telemetry queries; native race toolchain; live Gateway/Fleet/Vendor/Wallet/Media/Notification/Analytics/routing smoke tests. Full Dispatch's original failure is retained in the record; all exposed failures passed targeted correction, not a second full run.

**Deployment dependencies:** staged migration052 and legacy-code drain; production handover key separation; authoritative Fleet visibility mode for managed traffic; configured Redis and approved notification transports; log/metric collection and alerting. No invented legal retention, KMS or SMS claim.

Implementation readiness for the next roadmap section does not authorize production rollout or equate to database/live-service certification.

## Final verification addendum

- Gateway Partner contracts: **71 top-level tests PASS**, no skips.
- Vendor Dispatch adapter/lifecycle contracts: **5 PASS, 1 PostgreSQL SKIP** (`TestDispatchLifecycleDuplicateStaleAndForeignParcelPostgres`).
- Final `go vet` and `go build`: exit 0 for Dispatch, Fleet, shared/httpx, shared/logging and shared/observability.
- Formatting: all 56 session Go files formatted, including final added DB race test.
- `git diff --check`: exit 0. Git emitted existing LF/CRLF normalization warnings; no whitespace-error gate failure.
- Final branch/HEAD unchanged; index empty; 174 status entries versus baseline 141. All baseline status entries remain present. This is a filename/status preservation check, not a claim that intended edited files are byte-identical.
- Final report distinguishes raw Go parent-container results from executed assertions. There is no claim of 1,088 independently executed security tests.
- No code cleanup, Git mutation, database mutation, live external deployment or Section 20 implementation was performed.

## Session file manifest

This list identifies files touched by **this** hardening session, not all unrelated pre-existing dirty work. Some listed files already existed as uncommitted implementation and were amended in place.

| File | Purpose |
|---|---|
| `services/driver-dispatch-delivery-service/cmd/main.go` | Wire shared privacy/telemetry into existing composition |
| `services/driver-dispatch-delivery-service/internal/apiresponse/privacy.go` | Stage-scoped receiver minimization and managed-provider financial concealment |
| `services/driver-dispatch-delivery-service/internal/apiresponse/privacy_test.go` | Focused regression/security/compatibility coverage or updated safe-response fixture |
| `services/driver-dispatch-delivery-service/internal/apiresponse/response.go` | Stage-scoped receiver minimization and managed-provider financial concealment |
| `services/driver-dispatch-delivery-service/internal/config/config.go` | Handover key separation/production safety and collector interval |
| `services/driver-dispatch-delivery-service/internal/config/security_test.go` | Focused regression/security/compatibility coverage or updated safe-response fixture |
| `services/driver-dispatch-delivery-service/internal/middleware/logger.go` | Safe structured HTTP access logging |
| `services/driver-dispatch-delivery-service/internal/router/router.go` | Wire shared privacy/telemetry into existing composition |
| `services/driver-dispatch-delivery-service/internal/router/router_response_test.go` | Focused regression/security/compatibility coverage or updated safe-response fixture |
| `services/driver-dispatch-delivery-service/internal/observability/metrics.go` | Bounded metrics, durable collector and dependency/worker instrumentation |
| `services/driver-dispatch-delivery-service/internal/observability/collector.go` | Bounded metrics, durable collector and dependency/worker instrumentation |
| `services/driver-dispatch-delivery-service/internal/observability/metrics_test.go` | Focused regression/security/compatibility coverage or updated safe-response fixture |
| `services/driver-dispatch-delivery-service/internal/features/auth/http/middleware.go` | Privacy authority check before mutation or removal of raw OTP/contact logging |
| `services/driver-dispatch-delivery-service/internal/features/auth/usecases/auth_usecase.go` | Privacy authority check before mutation or removal of raw OTP/contact logging |
| `services/driver-dispatch-delivery-service/internal/features/request/service.go` | Pre-acceptance receiver minimization in list/detail/push |
| `services/driver-dispatch-delivery-service/internal/features/request/model.go` | Pre-acceptance receiver minimization in list/detail/push |
| `services/driver-dispatch-delivery-service/internal/features/request/workers.go` | Pre-acceptance receiver minimization in list/detail/push |
| `services/driver-dispatch-delivery-service/internal/features/request/phase6fgh_test.go` | Focused regression/security/compatibility coverage or updated safe-response fixture |
| `services/driver-dispatch-delivery-service/internal/features/request/phase6ijk_test.go` | Focused regression/security/compatibility coverage or updated safe-response fixture |
| `services/driver-dispatch-delivery-service/internal/features/securehandover/service.go` | Keyed credential fingerprints, fail-closed context limits, safe legacy credential references or secret intent serialization |
| `services/driver-dispatch-delivery-service/internal/features/securehandover/crypto.go` | Keyed credential fingerprints, fail-closed context limits, safe legacy credential references or secret intent serialization |
| `services/driver-dispatch-delivery-service/internal/features/securehandover/compatibility.go` | Keyed credential fingerprints, fail-closed context limits, safe legacy credential references or secret intent serialization |
| `services/driver-dispatch-delivery-service/internal/features/securehandover/compatibility_test.go` | Focused regression/security/compatibility coverage or updated safe-response fixture |
| `services/driver-dispatch-delivery-service/internal/features/securehandover/security_controls_test.go` | Focused regression/security/compatibility coverage or updated safe-response fixture |
| `services/driver-dispatch-delivery-service/internal/features/securehandover/notification.go` | Keyed credential fingerprints, fail-closed context limits, safe legacy credential references or secret intent serialization |
| `services/driver-dispatch-delivery-service/internal/features/assignment/service.go` | Instrument existing worker; preserve durable retry/idempotency authority |
| `services/driver-dispatch-delivery-service/internal/features/deliveryrecovery/service.go` | Expected-version emergency recovery guard and worker telemetry |
| `services/driver-dispatch-delivery-service/internal/features/deliveryrecovery/repository.go` | Expected-version emergency recovery guard and worker telemetry |
| `services/driver-dispatch-delivery-service/internal/features/deliveryrecovery/models.go` | Expected-version emergency recovery guard and worker telemetry |
| `services/driver-dispatch-delivery-service/internal/features/deliveryrecovery/security_test.go` | Focused regression/security/compatibility coverage or updated safe-response fixture |
| `services/driver-dispatch-delivery-service/internal/features/routeintegrity/service.go` | Instrument existing worker; preserve durable retry/idempotency authority |
| `services/driver-dispatch-delivery-service/internal/features/marketplace/outbox_delivery_worker.go` | Instrument existing worker; preserve durable retry/idempotency authority |
| `services/driver-dispatch-delivery-service/internal/features/marketplace/relay_worker.go` | Instrument existing worker; preserve durable retry/idempotency authority |
| `services/driver-dispatch-delivery-service/internal/features/trip/service.go` | Reference-only legacy OTP persistence, authorized redisplay and worker/log safety |
| `services/driver-dispatch-delivery-service/internal/features/trip/verification.go` | Reference-only legacy OTP persistence, authorized redisplay and worker/log safety |
| `services/driver-dispatch-delivery-service/internal/features/trip/settlement_reconciliation.go` | Instrument existing worker; preserve durable retry/idempotency authority |
| `services/driver-dispatch-delivery-service/internal/features/trip/phase7cde_test.go` | Focused regression/security/compatibility coverage or updated safe-response fixture |
| `services/driver-dispatch-delivery-service/internal/features/trip/phase7fgh_test.go` | Focused regression/security/compatibility coverage or updated safe-response fixture |
| `services/driver-dispatch-delivery-service/internal/features/trip/events_outbox_integration_test.go` | Focused regression/security/compatibility coverage or updated safe-response fixture |
| `services/driver-dispatch-delivery-service/internal/features/trip/delivery_outcome_integration_test.go` | Focused regression/security/compatibility coverage or updated safe-response fixture |
| `services/driver-dispatch-delivery-service/internal/features/booking/service.go` | Authorized compatibility code decoding or minimal notification documentation |
| `services/driver-dispatch-delivery-service/internal/features/booking/trip_adapter.go` | Authorized compatibility code decoding or minimal notification documentation |
| `services/driver-dispatch-delivery-service/internal/features/booking/hold_reconciliation.go` | Instrument existing worker; preserve durable retry/idempotency authority |
| `services/driver-dispatch-delivery-service/internal/features/booking/clients/notification_client.go` | Authorized compatibility code decoding or minimal notification documentation |
| `services/driver-dispatch-delivery-service/internal/features/commercemission/pickup_handover.go` | Legacy pickup/transfer opaque credential persistence; authorized redisplay |
| `services/driver-dispatch-delivery-service/internal/features/commercemission/provider_transfer.go` | Legacy pickup/transfer opaque credential persistence; authorized redisplay |
| `services/driver-dispatch-delivery-service/migrations/security_observability_certification_test.go` | Focused regression/security/compatibility coverage or updated safe-response fixture |
| `services/fleet-partner-service/internal/observability/metrics.go` | Compatibility alias to the same shared metrics registry |
| `shared/go/httpx/errors.go` | Safe request correlation, recovery and HTTP logging/error behavior |
| `shared/go/httpx/logger.go` | Safe request correlation, recovery and HTTP logging/error behavior |
| `shared/go/httpx/middleware.go` | Safe request correlation, recovery and HTTP logging/error behavior |
| `shared/go/httpx/security_test.go` | Focused regression/security/compatibility coverage or updated safe-response fixture |
| `shared/go/logging/structured.go` | Recursive structured-log redaction using existing logger |
| `shared/go/logging/structured_test.go` | Focused regression/security/compatibility coverage or updated safe-response fixture |
| `shared/go/observability/metrics.go` | Existing Fleet metrics registry shared for reuse |
| `shared/go/observability/metrics_test.go` | Focused regression/security/compatibility coverage or updated safe-response fixture |
| `services/driver-dispatch-delivery-service/migrations/000052_legacy_handover_secret_isolation.up.sql` | New guarded reference-only credential constraint and obsolete-secret cleanup migration |
| `services/driver-dispatch-delivery-service/migrations/000052_legacy_handover_secret_isolation.down.sql` | Remove only the new constraints, preserve domain history |
| `services/driver-dispatch-delivery-service/docs/openapi/delivery-recovery-route-integrity.openapi.json` | Existing emergency contract requires expected Trip version |
| `services/driver-dispatch-delivery-service/docs/API_ROUTE_REGISTER.md` | Zero-route-change and privacy/emergency contract reconciliation |
| `services/driver-dispatch-delivery-service/docs/security-observability-certification.md` | This consolidated evidence, migration/rollout and frontend handoff |
