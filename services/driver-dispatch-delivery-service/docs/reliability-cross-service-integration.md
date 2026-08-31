# Dispatch Sections 15–16 Reliability and Integration Report

Historical implementation report: its PARTIAL findings below are preserved as the pre-repair audit. The subsequent [gap-closure report](reliability-integration-gap-closure.md) records the repairs, final test results and remaining PostgreSQL certification blocker. Do not use this historical status as the current implementation status.

Audit date: 2026-08-31. Current working tree, not a released/committed revision.

## Overall Status

| Gate | Result |
|---|---|
| IMPLEMENTATION | PARTIAL |
| SECTION 15 | PARTIAL |
| SECTION 16 | PARTIAL |
| EVENT / OUTBOX CERTIFICATION | PARTIAL: local controls tested; PostgreSQL and all downstream consumers not certified |
| WORKER RELIABILITY CERTIFICATION | PARTIAL: available tests pass; database races skipped |
| CROSS-SERVICE CONTRACT CERTIFICATION | PARTIAL |
| NON-DATABASE CERTIFICATION | PASS for executed tests, vet and build; not a claim of complete integration |
| POSTGRESQL CERTIFICATION | BLOCKED_BY_TEST_INFRASTRUCTURE |
| NEW WORKERS CREATED | 2 implementations: recovery scheduler/return retry; abandoned booking hold reconciliation |
| EXISTING WORKERS REUSED | 8 worker families listed below; an additional Notification-target instance reuses the existing outbox worker |
| NEW EVENTS CREATED | 1: `dispatch.delivery_recovery.activated.v1` |
| EXISTING EVENTS REUSED | 60 distinct versioned event names retained in the audited production packages; this is a catalogue count, not proof all consumers exist |
| NEW MIGRATION | `000050_dispatch_reliability_reconciliation` |

## 1. Baseline audit

- Branch: `staging`; HEAD: `2e46bcd5b557f4ae559668d519993f84401dd5f0`.
- Existing substantial dirty/untracked Sections 4–14, Vendor, Hauling and shared work was retained. Nothing staged, committed, pushed, pulled, reset, stashed or cleaned.
- Migration tip before this work: `000049_booking_serviceability_idempotency`; after: `000050_dispatch_reliability_reconciliation`. Earlier migration files were not edited by this task.
- Canonical database variable `DISPATCH_TEST_DATABASE_URL` is unset. No substitute database, connection string or Docker volume was created.
- Disk checked before build/test; approximately 65 GB free.
- Canonical outbox: `dispatch_marketplace_outbox`, with `dispatch_marketplace_outbox_deliveries`. Existing Fleet relay: `dispatch_fleet_relay_operations`.
- Reused shared Wallet, Notification, Media, service authentication, existing Fleet adapters, CommerceMission and routing/serviceability authorities.
- Existing feature defaults remain off for sequential assignment, queue, multi-stop missions and protected operational capabilities. No rollout was globally enabled.

## 2. Outbox architecture

State-changing repositories append events inside their PostgreSQL transaction. The envelope contains event ID/type, schema/source, aggregate identity/version, occurrence/correlation/causation, payload and payload hash. Target-specific delivery claims use `FOR UPDATE SKIP LOCKED`; lease expiry recovers abandoned claims. Attempts, capped exponential backoff, dead-letter state and existing operator replay remain authoritative.

This pass adds a unique **claim epoch** to outbox lease ownership. Worker success/failure updates are fenced by that epoch, so a timed-out old worker cannot revert a newer acknowledged delivery. Existing unleased repository helpers remain for compatibility; production `OutboxWorker` passes the claimed epoch. Maintenance failures are logged without payload/credential material.

Nested payload validation now normalizes typed maps/slices and checks case/underscore variants. Migration 000050 adds a recursive JSONB constraint excluding OTP/QR values and hashes, payment/card credentials and direct receiver contact fields. Applying that constraint to historical data is deliberately **not certified** without PostgreSQL. No second outbox/event bus was created.

## 3. Event catalogue

Every listed name exists in the current production tree. The retained catalogue consists of 61 unique versioned names in the audited feature packages, including the one new event. Grouping below avoids implying a consumer exists just because an event is stored.

| Actual events / family | Trigger | Transactional persistence | Consumer / readiness | Payload boundary |
|---|---|---|---|---|
| `platform.request.offered.v1`, `updated`, `reserved`, `reservation_released`, `expired`, `cancelled` | Marketplace request/reservation transitions | Existing outbox | Existing Fleet contract/worker | Operational references, no commercial pricing |
| `trip.created.v1`, `trip.status_changed.v1`, `trip.completed.v1`, `trip.cancelled.v1`, `trip.recipient_confirmed.v1`, `trip.delivery_failed.v1`, `proof.completed.v1` | Canonical execution/proof transitions | Existing outbox | Existing Fleet projections; legacy Redis notifications are separate | Minimized execution facts |
| `commerce.mission.created.v1`, `commerce.mission.changed.v1`, `commerce.stop.changed.v1`, `commerce.parcel.changed.v1`, `commerce.evidence.recorded.v1`, `commerce.exception.reported.v1`, `commerce.return.started.v1`, `commerce.return.completed.v1`, `commerce.provider_transfer.started.v1`, `commerce.provider_transfer.completed.v1` | Existing CommerceMission transitions | Existing outbox | Vendor accepts the established `COMMERCE_MISSION` envelope | Source/mission/parcel correlation, no ledger authority |
| `dispatch.assignment.started.v1`, `dispatch.offer.created.v1`, `dispatch.offer.declined.v1`, `dispatch.offer.expired.v1`, `dispatch.mission.assigned.v1`, `dispatch.mission.no_provider.v1`, `dispatch.mission.queued.v1`, `dispatch.queue.exhausted.v1`, `dispatch.queue.option_selected.v1`, `dispatch.queue.requote_required.v1` | Assignment/queue changes | Existing outbox | Stored with Vendor target; **new `dispatch.*` consumer contract is not implemented by the current Vendor CommerceMission receiver** | References/status, not caller-authoritative money |
| `dispatch.delivery_plan.created.v1`, `dispatch.delivery_plan.superseded.v1` | Plan lifecycle | Existing outbox | Same new-domain Vendor consumer dependency | Versioned plan references |
| `dispatch.handover.challenge_issued.v1`, `dispatch.handover.resent.v1`, `dispatch.evidence.accepted.v1`, `dispatch.pickup.verified.v1`, `dispatch.receiver.verified.v1`, `dispatch.return.completed.v1`, `dispatch.provider_transfer.verified.v1` | Handover/evidence | Existing outbox | New-domain consumer dependency; secret delivery uses a separate constrained Notification request | Generic events never transport challenge secrets/hashes |
| `dispatch.delivery_attempt.started.v1`, `dispatch.receiver_unavailable.recorded.v1`, `dispatch.delivery_recovery.selected.v1`, `dispatch.delivery_recovery.activated.v1`, `dispatch.delivery.retry_scheduled.v1`, `dispatch.delivery.rescheduled.v1`, `dispatch.delivery.alternative_receiver_selected.v1`, `dispatch.return.started.v1`, `dispatch.hub_hold.started.v1`, `dispatch.delivery.operations_review_requested.v1`, `dispatch.delivery_incident.reported.v1`, `dispatch.provider_replacement.started.v1`, `dispatch.hub_hold.checkpoint_recorded.v1`, `dispatch.hub_hold.released.v1`, `dispatch.emergency_recovery.controlled.v1` | Recovery changes | Existing outbox | New adapter feeds existing Notification send API; service-owned recipient read-back | Fixed safe title/body and opaque navigation data, not original event payload |
| `dispatch.route.created.v1`, `dispatch.route.versioned.v1`, `dispatch.route.rerouted.v1`, `dispatch.route_alert.opened.v1` | Route/alert evaluation | Existing outbox | Stored with Analytics target; **no compatible target worker/consumer is wired** | Route/version/alert facts, not a new analytics authority |

Unqualified suffixes in the first row expand under `platform.request.` and retain `.v1`. No new event name is claimed for challenge expiry or Wallet acknowledgement: those changes currently use their domain persistence/projections.

## 4. Events added / fixed

- `dispatch.delivery_recovery.activated.v1`: commits with a due scheduled action becoming READY. Its aggregate is the recovery-action ID/version 1, avoiding collisions between multiple scheduled actions on one Trip.
- Generic event secrets are blocked recursively, including typed nested payloads and obvious casing variants.
- Existing recovery events previously had a Notification target but no send-contract adapter/worker. They now reuse the canonical outbox and accepted Notification API.
- Existing Vendor/Analytics incompatibilities are explicitly retained as outstanding integration work, not relabelled as successful delivery.

## 5. Worker inventory

| Worker family | Responsibility / claiming | Retry and recovery | Gate |
|---|---|---|---|
| Assignment `RunWorker` (reused) | Offer expiry, queue retries, accepted assignment Trip recovery | Existing domain locking plus leased queue Wallet reconciliation | Assignment flags and kill switch |
| Secure handover `RunExpiryWorker` (reused) | Challenge expiry and pending Notification intents | Durable REQUESTED/FAILED intent, lease epochs, attempts/backoff/dead-letter; expired/changed contexts do not send secrets | Handover enabled; approved notifier/channels |
| Route integrity `RunStaleWorker` (reused) | Stale location/route evaluation | Existing persisted alert dedupe; logged failures; GPS ingestion remains separate | Route-integrity policy |
| Marketplace `OutboxWorker` (reused) | Fleet, Commerce and now recovery Notification targets | Leases, fenced completion, attempt history/backoff/dead-letter/replay | Target configuration / existing delivery flags |
| Marketplace `RelayWorker` (reused) | Fleet provider decisions | Durable original operation key, outcome-unknown reconciliation and replay | Fleet integration |
| Request Asynq worker (reused) | Existing broadcast/request jobs and push tasks | Existing Redis/Asynq retry ownership | Existing bootstrap |
| Trip `RunAutoCompleteWorker` (reused) | Proof-submitted completion plus completed-Trip settlement recovery | Completed Trip is durable intent; source-keyed Wallet/Commerce read-back, leases/backoff/dead-letter | Existing bootstrap; settlement adapter |
| CommerceMission recovery worker (reused) | Existing reservation/execution compatibility | Existing source/idempotency read-back | Commerce mission flag |
| Delivery recovery `RunWorker` (new implementation) | Due schedule activation and return assignment recovery | Locks action/Trip; closes prior attempt without inventing arrival; claimed return retry with unique epoch | Delivery recovery; assignment kill switch respected |
| Booking hold reconciliation (new implementation) | Abandoned/unknown **customer** hold read-back and compensation | Excludes DeliveryMission/CommerceMission bookings; original intent/refund keys; never calls PayFromWallet | Existing Wallet configuration; centralized retry settings |

Config additions: `DISPATCH_RECOVERY_WORKER_INTERVAL_SECONDS=10`, `DISPATCH_EXTERNAL_OPERATION_RETRY_LIMIT=10`, `DISPATCH_EXTERNAL_OPERATION_BACKOFF_BASE_SECONDS=5`, `DISPATCH_EXTERNAL_OPERATION_BACKOFF_MAX_SECONDS=900`, `DISPATCH_EXTERNAL_OPERATION_LEASE_SECONDS=60`, `DISPATCH_EXTERNAL_SERVICE_TIMEOUT_SECONDS=10`. These are technical defaults, not legal/business policy. Abandoned-hold age derives from two external-call timeouts plus the lease. Terminal unknown/dead-letter rows require operational investigation; they do not mean financial failure.

## 6. Reconciliation

- **Wallet hold:** unknown error stops matching without claiming payment failed/refunded. Recovery reuses `dispatch-hold-{booking}` to resolve the existing intent, then GETs authoritative payment state. It never repeats a debit. Held funds use `refund-booking-{booking}` compensation. Pending/settled/unavailable read-back remains unresolved rather than guessed. Source/customer/amount/currency are validated.
- **Wallet completion:** physical completion remains committed. `HandleTripCompleted` marks payment paid only after Wallet acknowledgement; existing Commerce allocation release is selected for Commerce bookings. The completed Trip row survives a crash before any external call and is swept independently of Redis pub/sub.
- **Queue financial release:** retained Booking financial authority, durable original operation ref, leased retries and bounded failure projection. No ledger in Dispatch.
- **Notification:** initial/resend intent commits before send; failures retry the same challenge/version key. Context, provider, recipient, source and manifest are re-read before secret delivery. Generic recovery notifications reuse the event ID and derive the recipient from Dispatch rows.
- **Return assignment:** the recovery action's original key is reused. Removing a freshly generated queue deadline from the adapter stabilizes its fingerprint. `ReadStart` resolves a committed assignment before checking new candidate availability.
- **Fleet:** existing durable relay and authoritative response lookup retained. Vendor CommerceMission source read-back remains the compatibility authority.

## 7. Cross-service matrix

| Service | Dispatch owns | Remote owns | Auth / idempotency / read-back | Status |
|---|---|---|---|---|
| Wallet | Execution evidence, references and acknowledgement projection | Holds, ledger, refunds, settlement/earnings | Existing signed shared client; source/key dedupe, payment GET, idempotent completion | Adapter/unit PASS; PostgreSQL/end-to-end pending |
| Notification | Minimal intent, authorized recipient context | Durable acceptance and channel transport/retries | Existing signed client; challenge/version or event-ID key | Recovery adapters PASS in tests; deployment delivery untested |
| Media | Mission/Parcel/evidence relationship | Bytes, metadata/access grants | Existing authenticated mission-bound authorization before evidence persistence | Reused; no byte service duplicated |
| API Gateway | Source-scoped execution commands/read-back | Partner identity/API credentials | Existing internal HMAC allowlist and source/version checks | Reused; no new public Partner auth |
| Vendor | Delivery execution and CommerceMission compatibility | Cart/catalogue/stock/order/payable/commercial decisions | Existing CommerceMission source read-back and event receiver | Established contract reused; new `dispatch.*` event consumer BLOCKED_BY_CROSS_SERVICE_DEPENDENCY |
| Fleet | Execution/provider safety | Organization membership/assignment authority | Existing authenticated authority + durable relay | Reused; DB tests skipped |
| Support | Operational exceptions/evidence references | Formal disputes/resolution | Existing Commerce support boundary; no new direct Section 15–16 adjudication integration | Future/direct contract only where no route exists |
| Admin | Controlled internal operations/read/replay | Operator identity/scopes and UI | Existing admin/internal HMAC and audit | Reused; no operator frontend |
| Analytics | Minimized durable execution facts | Long-term read models/reporting | Existing Commerce analytics authority not replaced | Route-integrity stream consumer/adapter not wired |

## 8. Wallet

No Dispatch ledger, balance calculation, earnings allocation or invented refund authority was added. Financial timeouts are not confirmations. `WalletPaymentClient.Refund` now requires `processed`; completion requires the correct source pair and `completed`. Unknown hold read-back never calls `PayFromWallet`. Historical completed Trips are eligible for idempotent settlement catch-up. Real Wallet/PostgreSQL exactly-once execution is not certified here.

## 9. Notification

EMAIL and IN_APP/PUSH use existing Notification contracts. Dispatch does not implement transport. Handover secret-bearing requests remain isolated from generic events. Generic recovery events now become fixed-content in-app/push requests rather than being posted as incompatible raw envelopes. SMS intent flags remain present; **SMS TRANSPORT ACTIVATION REMAINS A NOTIFICATION-SERVICE DEPLOYMENT DEPENDENCY**. Legacy Booking lifecycle notifiers remain best-effort and are a separate remaining durability gap, not covered by the new recovery adapter.

## 10. Media

Existing shared Media client and `internal/router/media_attachment_authorizer.go` remain authoritative for access/read-back. Dispatch authorizes mission/Parcel/purpose attachment and persists opaque references; Media owns bytes and private access. No Media production code changed.

## 11. API Gateway / Partner

Existing Partner queue/internal execution contracts remain source-scoped and HMAC-protected. API Gateway retains Partner authentication. No routes or Partner API key storage were introduced.

## 12. Vendor

Existing `commerce_missions` compatibility and source/idempotency lookup were reused. The current Vendor receiver (`services/vendor-service/internal/features/orders/dispatch_commerce_events.go`) requires `aggregateType=COMMERCE_MISSION` and a closed `commerce.*` event list. It does **not** accept assignment/planning/handover `dispatch.*` envelopes. Loosening that validation would be unsafe; a real scoped consumer/adapter is still required.

## 13. Fleet

Organization membership, managed-provider authority and provider decisions stay with the existing Fleet/private adapters. Normal marketplace eligibility is unchanged. Relay timeout and connection-reset tests executed; PostgreSQL relay races skipped.

## 14. Support / Dispute

Formal adjudication and refund decisions remain Support/Wallet-owned. Existing operational references are reused. No new direct integration is claimed for operational events merely because they mention support-required states.

## 15. Admin Backoffice

Existing authenticated assignment/replay/operational boundaries are retained. No admin frontend, new role system or force-assignment bypass was added. New domain reconciliation dead letters must be investigated; there is no new unsafe public reset endpoint.

## 16. Analytics

Route events are durably stored with target `analytics-service`, but bootstrap currently has no compatible route-event publisher/consumer adapter. Commerce analytics is a different contract. This remains an integration gap rather than a fabricated Analytics PASS.

## 17. Failure / degradation matrix

| Dependency / failure | Dispatch behavior | Projection / recovery |
|---|---|---|
| Wallet hold response lost | Matching stopped; no new debit during recovery | Unknown cancelled booking, source read-back, safe compensation or unresolved/dead-letter |
| Wallet settlement unavailable | Physical completion preserved | Payment remains held; completed-Trip leased retry; only confirmed Wallet state becomes paid |
| Wallet refund processing/timeout | No claim of refunded money | Existing key retried/read back; unresolved state retained |
| Notification acceptance lost | Same message identity resent | Durable domain/outbox intent; no OTP response fallback |
| Notification unconfigured/channel absent | Dependency remains visible | No fabricated transport success; UNAVAILABLE handover delivery requires configuration/operator recovery |
| Media inaccessible/wrong relation | Reject attachment before persistence | Controlled authorization/dependency error |
| Fleet unknown result | Original command retained | Existing relay read-back/backoff/dead-letter |
| Assignment timeout | Read original operation before discovery | Same assignment/Booking/Trip, no new deadline fingerprint |
| Unsupported Vendor/Analytics event contract | No unsafe schema relaxation | Outstanding downstream integration; not certified delivered |

## 18. Security

No raw OTP, QR secret or their hashes are added to generic outbox payloads. No gateway/payment credentials, Partner API keys or unrestricted receiver profiles are added. Notification recipient authority comes from current Dispatch context, never event-provided `customer_id`. New retries use service-scoped existing clients and bounded HTTP timeouts. New fencing guards stop stale workers overwriting acknowledged results. These are implementation/non-DB test findings, not PostgreSQL race PASS claims.

## 19. Backward compatibility

Booking → Request → Trip, broadcast, provider inbox, planning, evidence, provider transfer, tracking, offline replay, Fleet, CommerceMission and Media authorities remain intact. Changes are additive except intentional corrections: timeout is no longer payment failure; physical completion no longer proves paid earnings; unsafe/mismatched notification context cannot receive a secret; stale worker results cannot overwrite a newer acknowledgement. Queue/return/handover flags remain off by default. Full non-DB Dispatch regression passed. Frontend-facing success text now says earnings are confirmed separately by Wallet.

## 20. Test results

Final commands ran from `C:\KarryGoPush`, using the existing `.codex-cache/go-build` cache. Long output was redirected and inspected; temporary logs are not implementation artifacts.

| Gate / exact command or package | Result |
|---|---|
| `go test -count=1` on Dispatch `internal/features/assignment`, `booking`, `trip`, `securehandover`, `deliveryrecovery`, `marketplace`, `routeintegrity`, `internal/config`, `internal/router`, `migrations`, plus `shared/go/walletclient` and `shared/go/notifications` | All 12 packages PASS in focused run |
| `go test -count=1 -json ./services/driver-dispatch-delivery-service/...` | Exit 0; 38 packages: 27 tested PASS, 11 no tests; 1,065 top-level tests PASS (1,344 including subtests); 124 tests SKIP |
| New event security test | Nested OTP/hash/QR and typed/casing variants rejected; safe references allowed |
| New recovery notification test | Timeout UNKNOWN, same event key, authoritative recipient, no payload secret forwarding, unsupported event rejected |
| New handover notification tests | Challenge/version identity stable; changed recipient/provider/manifest/source/invalidation rejected |
| New assignment read-back test | Accepted-operation retry does not rediscover an already-busy winning provider |
| New Wallet hold tests | Real adapter against HTTP stub; no repeated debit; stable refund key; pending/settled state not falsely resolved |
| New settlement tests | Timeout does not mark paid; original Booking identity retained; worker recovers acknowledgement |
| Migration shape | PASS |
| New PostgreSQL tests | Settlement claim/stale-worker race, outbox lease fencing, handover claim race, queue Wallet claim race, scheduled activation race, migration rollback/reapply/secret constraint: all SKIPPED due canonical DB absence |
| Existing Fleet relay, execution, HMAC nonce and domain concurrency tests | Non-DB cases PASS; DB cases among the 124 skips |
| `go vet ./services/driver-dispatch-delivery-service/...` | PASS |
| `go build ./services/driver-dispatch-delivery-service/...` | PASS |
| `gofmt -l` on changed/new Dispatch Go files | PASS after formatting |
| `git diff --check` | PASS |

An earlier full Dispatch run also passed before final follow-up repairs. It is not substituted for the final-tree run above. No other service-wide regression was run: other services' production code was not changed in this task.

## 21. Remaining gaps

1. **New-domain Vendor event consumer:** current `dispatch.*` assignment/planning/handover events are incompatible with the established Vendor CommerceMission receiver. Requires an explicitly scoped consumer/adapter; BLOCKED_BY_CROSS_SERVICE_DEPENDENCY.
2. **Route-integrity Analytics event delivery:** durable events exist, but no compatible target worker/consumer is connected. No downstream analytics ingestion PASS is claimed.
3. **Legacy lifecycle notifications:** Booking's old fire-and-forget notification callbacks have not all been migrated onto transactionally durable intents. The new recovery Notification path does not close this broader legacy gap.
4. **Replacement-incident recovery:** return dispatch now has a durable scheduler; `ReportIncident` replacement still relies on repeating the original command after an external failure. A persisted automatic recovery sweep is not claimed.
5. **Database evidence:** migration apply/rollback/reapply, financial concurrency, nonce, outbox, scheduler/notification claims and replay require canonical PostgreSQL. No runtime DB PASS.
6. Configure service credentials/allowlists and channels in deployment. SMS activation remains Notification-owned. Pending/DEAD_LETTER operation recovery must not be mistaken for a confirmed financial outcome.

These are not cosmetic report deficiencies. Sections 15–16 must not be called complete merely because available tests pass.

## 22. Final verdict

**SECTIONS 15–16 NOT CLOSED — DO NOT START SECTION 17.**

The repair set is implemented and available tests/build gates pass. Remaining integration/durability gaps are listed above, separately from PostgreSQL certification. No Section 17+ work was started.

## File / migration traceability

All paths below are relative to `services/driver-dispatch-delivery-service/` unless stated otherwise.

Created:

- `migrations/000050_dispatch_reliability_reconciliation.{up,down}.sql`: existing domain retry projections, due indexes and outbox secret constraint; no ledger/new outbox.
- `migrations/reliability_reconciliation_test.go`: shape and canonical env-gated database tests.
- `internal/features/assignment/operation_readback{,_test}.go`: assignment retry lookup before candidate discovery.
- `internal/features/booking/hold_reconciliation{,_test}.go`: abandoned-hold compensation without re-debit.
- `internal/features/booking/settlement_reconciliation_test.go`: authoritative paid projection test.
- `internal/features/trip/settlement_reconciliation{,_test}.go`: durable completed-Trip settlement catch-up.
- `internal/features/deliveryrecovery/notification_delivery{,_test}.go`: canonical outbox-to-Notification adapter and tests.
- `internal/features/marketplace/event_secret_safety_test.go`: generic payload boundary tests.
- `docs/reliability-cross-service-integration.md`: this evidence report and operational/frontend handoff.

Modified (including extending files already untracked at handoff):

- `cmd/main.go`: bounded Wallet client, settlement/hold reconciliation wiring.
- `internal/config/config{,_test}.go`: centralized retry/timeout defaults.
- `internal/router/router.go`: recovery and Notification-target worker wiring; handover notification retry; kill-switch preservation.
- `internal/features/assignment/{models,policy,service,repository}.go`: durable queue financial retry and scoped read-back.
- `internal/features/booking/{payments,service,service_test}.go`: unknown hold classification, confirmed refunds/settlement and regression coverage.
- `internal/features/trip/service.go`: completion/settlement separation and existing-worker reuse.
- `internal/features/securehandover/{models,notification,notification_test,service,repository}.go`: atomic intent, retry, contextual safety and lease fencing.
- `internal/features/deliveryrecovery/{models,service,repository,adapters}.go`: schedule activation, return retry, stable assignment identity.
- `internal/features/marketplace/{events,repository,outbox_delivery_worker}.go`: nested secret validation and stale-worker fencing.
- `internal/features/routeintegrity/{service,subscribers}.go`: visible safe worker errors.
- `README.md`, `docs/API_ROUTE_REGISTER.md`: pointers/status and unchanged route count explanation.

No new production directory, roadmap-numbered Go filename, API route, OpenAPI operation, dependency, remote service implementation or frontend was added. Migration rollback removes new guards/projections only; runtime safety is pending canonical DB tests.

### Frontend / operations handoff (documentation only)

- A Wallet timeout may return unavailable/pending confirmation with booking dispatch stopped. Do not display “payment failed” or “refunded” unless authoritative status says so.
- Completed delivery is distinct from confirmed earnings/settlement. Display existing payment state accordingly.
- READY recovery permits the assigned provider to start the next real arrival attempt; it does not fabricate arrival or restart grace time in advance.
- Notification UNAVAILABLE/FAILED/DEAD_LETTER is not successful delivery. Do not expose challenge hashes or put credentials in generic event/log payloads.
- Generic recovery notifications contain safe navigation identifiers; reload the authorized booking/Trip view for current details.
- No frontend/provider UI or SMS transport was implemented.
