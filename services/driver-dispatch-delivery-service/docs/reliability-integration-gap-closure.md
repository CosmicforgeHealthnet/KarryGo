# Dispatch Sections 15–16 Gap Closure Report

Verification date: 2026-08-31. This report covers the current uncommitted working tree, not a deployed release. It supersedes the remaining-gap status in [the original implementation report](reliability-cross-service-integration.md), whose historical findings are retained.

## Overall Status

| Gate | Result |
|---|---|
| IMPLEMENTATION | COMPLETE |
| SECTION 15 | COMPLETE |
| SECTION 16 | COMPLETE |
| EVENT / OUTBOX CERTIFICATION | PASS — available contract/unit/static evidence; PostgreSQL execution blocked |
| WORKER RELIABILITY CERTIFICATION | PASS — available tests/code; database races not certified |
| CROSS-SERVICE CERTIFICATION | PASS — affected contracts/packages; not a live deployed-service certification |
| NON-DATABASE CERTIFICATION | PASS |
| POSTGRESQL CERTIFICATION | BLOCKED_BY_TEST_INFRASTRUCTURE |
| NEW WORKERS | 1 new target instance of the existing outbox worker: Analytics; incident reconciliation extends the existing recovery loop |
| NEW EVENTS | 0 new event names |
| NEW MIGRATIONS | Dispatch 000051; Vendor 000109; Analytics 002 |

## 1. Baseline

- Repository: `C:/KarryGoPush`; branch: `staging`.
- HEAD before/after: `2e46bcd5b557f4ae559668d519993f84401dd5f0`.
- Substantial pre-existing dirty/untracked Dispatch, Vendor, Hauling, shared and documentation work preserved. This is not a clean release checkout.
- Starting migration tips: Dispatch `000050_dispatch_reliability_reconciliation`; Vendor `000108_vendor_audit_append_only`; Analytics `001_commerce_analytics_reporting`.
- No staging, commit, push, pull, branch switch, reset, stash, clean or database creation performed. Index remains empty.
- Canonical Dispatch, Vendor and both Analytics test database variables are unset. No substitute DSN was invented.
- Disk checked before broad tests/build: approximately 59.5 GB free at the final regression check.
- Existing feature flags/kill switches remain safe-default off. Section 17 was not started.

## 2. Previous gaps A–E

| Gap | Status | Concrete repair / evidence |
|---|---|---|
| A: Vendor event consumer | FIXED | Approved shared lifecycle contract, producer transaction-time source projection, minimizing outbox adapter, existing Vendor receiver/storage/read model extended; strict source/parcel scope, event-ID dedup and stale version tests |
| B: Analytics delivery | FIXED | Existing route events flow through canonical outbox to a configured Analytics target; strict minimized metric DTO, caller/source binding, canonical ingest replay and daily metric view |
| C: Booking notification durability | FIXED | Additive PostgreSQL triggers persist existing lifecycle notification intent with Booking/Trip status mutation; existing worker/Notification transport reused; duplicate direct callbacks suppressed |
| D: Replacement recovery | FIXED | Existing incident recovery sweep, fenced leases, persisted bounded attempts/deadline, original assignment discovery/Fleet validation/inbox, one replacement winner per incident; accepted replacement does not create another Trip or move custody |
| E: PostgreSQL certification | BLOCKED | Tests written and compiled, but canonical database variables are absent; migration replay, atomicity and race tests skipped honestly |

## 3. Vendor event compatibility

Approved consumed names:

- `dispatch.delivery_plan.created.v1`
- `dispatch.delivery_plan.superseded.v1`
- `dispatch.mission.assigned.v1`
- `dispatch.pickup.verified.v1`
- `dispatch.receiver.verified.v1`
- `dispatch.return.completed.v1`
- `dispatch.return.started.v1`
- `dispatch.delivery_recovery.selected.v1`
- `dispatch.provider_replacement.started.v1`

The existing canonical Dispatch outbox captures a minimized `delivery_projection` through actual Group/Mission/manifest relations. Recovery fan-out uses another target-specific row in the SAME outbox, not another event bus. Snapshot capture is in the originating transaction, not an unstable HTTP-time read-back.

The target adapter forwards only source reference/version, group reference, and mission reference/version/state/parcel references. Vendor independently matches source reference to its previously reserved DISPATCH platform-mission operation and validates every parcel against that operation's plan slot. Merely providing an arbitrary business/source ID does not grant access.

Vendor reuses `commerce_platform_operational_events` and adds an optional projection on `commerce_platform_mission_refs`. Event-ID replay with an altered fingerprint conflicts. Lower source/mission versions do not regress the projection; conflicting groups at the same source version fail closed. Existing authorized fulfilment-group reads expose the optional projection. It is operational evidence, not order/payment/settlement authority.

Existing `commerce.*` / `COMMERCE_MISSION` transport and consumer path remain unchanged. Assignment candidates, offer internals and handover lifecycle internals are NOT automatically approved for Vendor delivery. Unsupported dispatch events and missing source binding are visible rejected/dead-letter outcomes, not successful deliveries.

Deployment constraint: the new source must correlate to an existing Vendor-reserved platform mission. Unrelated/unreserved sources fail closed rather than fabricating a Vendor order. Undelivered PENDING/FAILED pre-adapter rows get an explicit current-state snapshot in migration 000051; published history is not rewritten and dead-letter replay remains operator-controlled.

## 4. Analytics integration

Path:

`route/alert mutation -> dispatch_marketplace_outbox -> existing OutboxWorker (Analytics target) -> POST /internal/analytics/v1/commerce-events -> analytics_commerce_events -> analytics_dispatch_route_daily_metrics`.

The four existing route-created/versioned/rerouted/alert-opened events are reused. Alert insertion itself now creates an intent keyed to the actual alert ID, covering scheduler-created stale alerts and avoiding collisions between different alerts on one route version.

New route intents contain only route version, controlled state/alert type/severity, policy version and optional coarse distance/time buckets (5 km / 5 minutes). The adapter also minimizes older queued envelopes. Analytics replaces the operational aggregate identity with event identity and does not ingest provider/customer IDs, coordinates, full polylines, addresses, handover data or payment information.

Canonical ingest rejects a changed payload/event fingerprint on replay. Metrics live in a daily aggregate VIEW over canonical deduplicated events, not a second store or reliability-scoring engine. Existing Section 39 reliability/privacy decisions are unchanged.

Existing HMAC transport, target timeout, bounded attempts/backoff, lease recovery/fencing and dead letters apply. No HTTP call to Analytics occurs on GPS ingestion. Delivery interruption leaves durable backlog; it does not block route/Trip progression.

Configuration: `DISPATCH_ANALYTICS_EVENT_DELIVERY_ENABLED=false`, `DISPATCH_ANALYTICS_URL`, `DISPATCH_ANALYTICS_SECRET`; existing event worker interval/batch/lease/retry settings and external timeout reused. Analytics must allow/configure the Dispatch caller using its existing service-auth configuration. Deploy consumers/migrations before enabling delivery.

## 5. Booking notification durability

All generic entries below use Notification Service as transport/template authority. Stable key is the existing `notifications.IdempotencyKey(sourceService,eventType,bookingID+":"+recipientID)`, preserving upgrade/retry identity. Unique outbox aggregate/event identity coalesces corresponding Booking/Trip callbacks.

| Domain transition | Existing notification | Durable? | Message identity | Transport owner |
|---|---|---|---|---|
| Booking accepted / Trip assigned | delivery.accepted | Yes, same transaction | booking + customer + event | Notification |
| Booking unmatched | delivery.unmatched | Yes | booking + customer + event | Notification |
| Trip en_route_pickup | delivery.en_route_pickup | Yes | booking + customer + event | Notification |
| Trip arrived_pickup | delivery.arrived_at_pickup | Yes | booking + customer + event | Notification |
| Trip arrived_pickup | delivery.pickup_verification_required | Yes; no challenge secret | booking + customer + event | Notification |
| Trip in_progress | package.picked_up | Yes | booking + customer + event | Notification |
| Trip proof_submitted | package.delivered | Yes; existing proof semantics retained | booking + customer + event | Notification |
| Booking/Trip completed | delivery.completed | Yes | booking + customer + event | Notification |
| Customer cancellation | delivery.cancelled | Yes; assigned provider recipient | booking + provider + event | Notification |
| Provider cancellation | delivery.cancelled_by_provider | Yes; no false customer-cancellation duplicate | booking + customer + event | Notification |
| Legacy delivery code ready | Existing secret-bearing code notification | Existing narrowly scoped authority path, NOT generic outbox | Existing code contract | Notification |
| Secure handover initial/resend | Existing challenge notification intent | Existing durable, context/version-bound authority | challenge/version/recipient | Notification |
| Created/payment-pending/matching without historical callback | No new notification invented | Not applicable | Not applicable | Not applicable |

The direct Booking notification client is put into durable-lifecycle mode in composition. It no longer duplicates generic sends, but retains the existing trusted code-delivery exception. No raw OTP/QR or challenge hash enters the new generic intent. When Notification is unavailable, intent remains pending/unknown and retries the same key; absence of a message acknowledgement is not reported as delivery success.

## 6. Replacement recovery

The persisted `dispatch_delivery_incidents` row is the logical replacement operation. Recovery applies to a recoverable REASSIGNING Mission linked to the canonical Trip; it does not invent a new execution model for an unrelated legacy Trip.

The existing recovery maintenance loop:

1. Stops terminal/cancelled mission/group/Trip work and cancels outstanding incident offers.
2. Uses `FOR UPDATE SKIP LOCKED` and a unique lease epoch to claim due incidents.
3. Increments persisted attempts on claim, so repeated crashes consume a bounded budget.
4. Reads back READY/live accepted/offered state before provider rediscovery.
5. Reuses current candidate discovery/ranking/eligibility and authoritative Fleet membership. Organization-managed work never falls back to marketplace on Fleet failure.
6. Creates at most one live incident offer in the existing assignment table/provider inbox.
7. Accepts into `REPLACEMENT_READY` referencing the SAME Trip; original accepted assignment history remains intact.
8. Uses fenced completion, bounded backoff and live-offer expiry to avoid burning attempts while awaiting provider response.
9. Moves exhausted/deadline-expired work to `MANUAL_REVIEW`; unresolved external authority failures are UNKNOWN, not success.

The technical persisted recovery deadline defaults to 30 minutes; retry limit/backoff/lease come from existing centralized external-operation configuration. The worker does not manufacture an Admin identity: ADMIN_ASSISTED work still requires existing authorized operator selection and otherwise reaches controlled manual recovery.

New unique constraints cover one active incident per Trip and one accepted replacement per incident. Existing one-live-offer-per-Mission protection remains. Acceptance revalidates eligibility, version and deadline. Old provider/Trip custody is unchanged; actual transfer still requires existing challenge/scans/evidence. No second Trip or Wallet movement is created by the sweep.

## 7. Outbox certification

Canonical tables remain `dispatch_marketplace_outbox` and `dispatch_marketplace_outbox_deliveries`. Booking/Trip and alert triggers, Vendor projection capture and recovery fan-out are transaction-bound. Rollback must remove corresponding intents; dedicated PostgreSQL tests are present but NOT executed.

Existing worker leases, unique claim epochs, fenced acknowledgements, attempt records, bounded exponential retry, dead letters and operator replay are reused. Unknown outcome retains the same immutable event/message identity. Consumer replay conflicts are explicit. No new generic event system exists.

Fixed-cardinality operational counters are emitted through existing worker logging:

- vendor_event_delivery_backlog / failures
- analytics_event_delivery_backlog / failures
- booking_notification_intent_backlog / oldest_age
- replacement_recovery_backlog / attempts / exhausted

Backlog includes PENDING/FAILED/IN_FLIGHT; delivery failure gauges count current DEAD_LETTER rows, not a fictitious cumulative failure counter. Oldest age is seconds. No customer/Trip/provider IDs become metric labels. Database-backed counter execution is included in the infrastructure limitation.

## 8. Worker inventory

| Existing/final worker | Status and responsibility |
|---|---|
| Marketplace OutboxWorker — Fleet target | Reused; durable existing Fleet events |
| Marketplace OutboxWorker — Vendor target | Extended with approved lifecycle adapter; existing commerce delivery flag |
| Marketplace OutboxWorker — Notification target | Extended with Booking lifecycle adapter alongside recovery notifications; requires Notification config, no longer depends on recovery-feature enablement |
| Marketplace OutboxWorker — Analytics target | One new configured target instance, disabled by default |
| Marketplace RelayWorker | Reused Fleet decision/read-back authority |
| Request Asynq workers | Reused broadcast/request/push task ownership |
| Assignment expiry/queue worker | Reused; replacement-offer expiry is kept inside incident recovery, not generic no-provider terminalization |
| Secure handover expiry/notification worker | Reused durable trusted challenge delivery/expiry |
| Route integrity stale worker | Reused; inserted alerts now have durable Analytics intent |
| Trip auto-complete/settlement recovery | Reused canonical physical completion and Wallet acknowledgement |
| CommerceMission recovery worker | Reused legacy compatibility |
| Delivery recovery worker | Existing scheduled activation/return assignment plus new incident sweep |
| Booking hold reconciliation | Reused original Wallet intent/read-back and compensation |

## 9. Cross-service matrix

| Authority | Result / scope |
|---|---|
| Wallet | REUSED; sole ledger, original hold/settlement/refund identities unchanged; shared client and Dispatch tests pass; financial PostgreSQL/live-service certification blocked |
| Notification | REUSED transport; new safe lifecycle adapter tested; Notification service production code not modified or fully re-regressed |
| Media | REUSED existing signed relation authorization; no byte/access authority moved |
| API Gateway | REUSED existing internal caller/source boundary; no public partner authentication added |
| Vendor | CHANGED compatible consumer/read model; affected tests pass, PostgreSQL consumer runtime test skipped |
| Fleet | REUSED membership/provider adapter on recovery path; no marketplace fallback for private assignment |
| Support | REUSED formal resolution/evidence authority; no new adjudication or ledger behavior |
| Admin | REUSED authenticated assisted/operational path; worker never forges privileged actor |
| Analytics | CHANGED approved async ingest/view; contract/privacy/source tests pass, PostgreSQL replay test skipped |

These are code/contract and available test results. They do not assert that unconfigured remote deployments were contacted or certified.

## 10. Events

New names: NONE (0). Existing four route events, nine approved Vendor lifecycle events and ten existing Booking notification names were reused. The earlier `dispatch.delivery_recovery.activated.v1` addition belongs to the previous implementation pass and is not counted again.

## 11. Files changed

Paths below are relative to `C:/KarryGoPush`. “Modified” means modified by this closure on top of the dirty baseline, not necessarily tracked in HEAD. Unrelated prior dirty files are intentionally not attributed to this pass.

### Created

- `shared/go/commerceanalytics/dispatch_routes.go` — Strict minimized route metrics contract.
- `shared/go/commerceanalytics/dispatch_routes_test.go` — Route authority/privacy tests.
- `shared/go/dispatchcommerce/lifecycle.go` — Approved Vendor delivery lifecycle projection contract.
- `services/analytics-service/migrations/002_dispatch_route_metrics.sql` — Daily aggregate view over canonical analytics events.
- `services/analytics-service/internal/features/commerceanalytics/repositories/dispatch_routes_integration_test.go` — Canonical PostgreSQL replay/aggregate test.
- `services/analytics-service/internal/features/commerceanalytics/http/dispatch_routes_test.go` — Wire normalization and signer/source confusion tests.
- `services/driver-dispatch-delivery-service/internal/features/routeintegrity/analytics_delivery.go` — Minimized target-specific Analytics adapter.
- `services/driver-dispatch-delivery-service/internal/features/routeintegrity/analytics_delivery_test.go` — Minimization and timeout retry identity tests.
- `services/driver-dispatch-delivery-service/internal/router/analytics_delivery.go` — Analytics target worker composition.
- `services/driver-dispatch-delivery-service/internal/features/booking/notification_delivery.go` — Booking lifecycle Notification contract adapter.
- `services/driver-dispatch-delivery-service/internal/features/booking/notification_delivery_test.go` — Notification allowlist/privacy/retry tests.
- `services/driver-dispatch-delivery-service/internal/features/assignment/replacement_recovery.go` — Incident-scoped offers/read-back/acceptance on canonical assignment authority.
- `services/driver-dispatch-delivery-service/internal/features/assignment/replacement_recovery_test.go` — Replacement read-back and Fleet policy tests.
- `services/driver-dispatch-delivery-service/internal/features/deliveryrecovery/incident_reconciliation.go` — Durable incident sweep/claims/exhaustion.
- `services/driver-dispatch-delivery-service/internal/features/marketplace/vendor_lifecycle_delivery.go` — Vendor allowlist/minimizing transport adapter.
- `services/driver-dispatch-delivery-service/internal/features/marketplace/vendor_lifecycle_delivery_test.go` — New contract and legacy compatibility tests.
- `services/driver-dispatch-delivery-service/internal/features/marketplace/integration_metrics.go` — Fixed-cardinality backlog/age/failure counters.
- `services/driver-dispatch-delivery-service/migrations/000051_lifecycle_delivery_replacement_recovery.up.sql` — Atomic intents/projections and incident recovery constraints.
- `services/driver-dispatch-delivery-service/migrations/000051_lifecycle_delivery_replacement_recovery.down.sql` — Retention-safe rollback guards.
- `services/driver-dispatch-delivery-service/migrations/lifecycle_delivery_recovery_test.go` — Migration shape and PostgreSQL transactional/concurrency scenarios.
- `services/vendor-service/migrations/000109_dispatch_lifecycle_projection.up.sql` — Extend canonical operational event/projection storage.
- `services/vendor-service/migrations/000109_dispatch_lifecycle_projection.down.sql` — Reject rollback that would erase consumed history.
- `services/vendor-service/internal/features/orders/dispatch_lifecycle_events.go` — Source/parcel authorization, deduplication and stale-safe projection.
- `services/vendor-service/internal/features/orders/dispatch_lifecycle_events_test.go` — Lifecycle contract, duplicate/stale and privacy tests.
- `services/vendor-service/internal/features/orders/dispatch_lifecycle_integration_test.go` — Canonical PostgreSQL duplicate/stale/foreign parcel test.
- `services/driver-dispatch-delivery-service/docs/reliability-integration-gap-closure.md` — Final 17-part evidence report and deployment/certification limitations.

### Modified

- `shared/go/commerceanalytics/commerceanalytics.go` — Route event validation/allowlist.
- `services/analytics-service/internal/features/commerceanalytics/http/routes.go` — Dispatch wire adapter and authenticated source binding.
- `services/analytics-service/internal/features/commerceanalytics/repositories/postgres.go` — Route replay conflict detection; separate aggregate semantics.
- `services/analytics-service/docs/openapi/commerce-analytics-reporting-reliability.openapi.json` — Document expanded ingest contract; no new route.
- `services/driver-dispatch-delivery-service/internal/features/routeintegrity/repository.go` — Minimize route events before persistence; alert intent trigger integration.
- `services/driver-dispatch-delivery-service/internal/config/config.go` — Analytics target flag/URL/secret.
- `services/driver-dispatch-delivery-service/internal/router/router.go` — Compose target adapters and recovery Fleet authority.
- `services/driver-dispatch-delivery-service/cmd/main.go` — Enable durable legacy lifecycle notification mode.
- `services/driver-dispatch-delivery-service/internal/features/booking/clients/notification_client.go` — Suppress duplicate direct lifecycle sends; preserve trusted code notification.
- `services/driver-dispatch-delivery-service/internal/features/assignment/repository.go` — Replacement-aware offer reads/accept/decline/expiry and authority context.
- `services/driver-dispatch-delivery-service/internal/features/assignment/models.go` — Optional incident/purpose provider-offer metadata.
- `services/driver-dispatch-delivery-service/internal/features/assignment/operation_readback.go` — Keep initial assignment read-back distinct from replacement offers.
- `services/driver-dispatch-delivery-service/internal/features/deliveryrecovery/service.go` — Persist incident intent; use sweep rather than post-commit direct assignment.
- `services/driver-dispatch-delivery-service/internal/features/deliveryrecovery/adapters.go` — Reuse assignment recovery contract.
- `services/driver-dispatch-delivery-service/internal/features/marketplace/outbox_delivery_worker.go` — Emit fixed-cardinality integration metrics.
- `services/vendor-service/internal/features/orders/dispatch_commerce_events.go` — Route approved dispatch.* to additive consumer; retain commerce.*.
- `services/vendor-service/internal/features/fulfilmentgroup/mission.go` — Expose authorized optional Dispatch domain projection.
- `services/vendor-service/internal/features/fulfilmentgroup/store.go` — Read projection through existing scoped repository.
- `services/vendor-service/docs/openapi/dispatch-commerce-events.openapi.json` — Document additive event contract.
- `services/driver-dispatch-delivery-service/README.md` — Link final closure evidence.
- `services/driver-dispatch-delivery-service/docs/API_ROUTE_REGISTER.md` — Zero-route-change reconciliation and additive inbox/consumer fields.
- `services/driver-dispatch-delivery-service/docs/openapi/assignment-queue-multi-vendor-planning.openapi.json` — Optional replacement inbox fields and acceptance semantics; existing routes unchanged.
- `services/driver-dispatch-delivery-service/docs/reliability-cross-service-integration.md` — Preserve historical gaps; link superseding closure.
- `services/driver-dispatch-delivery-service/docs/commerce-delivery-provider-handoff.md` — Durable notifications/replacement-ready UI guidance.
- `services/analytics-service/README.md` — Document Dispatch metrics ingest and deployment prerequisites.

No new production package directories, framework, service, or roadmap-number production Go files were introduced. Test output remains under the existing local `.codex-cache`; it is not project code.

## 12. Migrations

| Service | Before | After | New schema / rollback |
|---|---|---|---|
| Dispatch | 000050 | 000051_lifecycle_delivery_replacement_recovery | Existing-table triggers, incident claim/deadline/result fields, scoped assignment winner constraints; down removes additions but refuses if replacement-offer history would be destroyed |
| Vendor | 000108 | 000109_dispatch_lifecycle_projection | Optional authorized projection and expanded existing event CHECK; down refuses when consumed dispatch lifecycle history exists |
| Analytics | 001 | 002_dispatch_route_metrics | Aggregate VIEW over existing event table; existing Analytics migrator is forward-only; deliberate rollback is DROP VIEW analytics_dispatch_route_daily_metrics, retaining canonical events |

Dispatch migration 000050 and every earlier migration were not edited by this pass. Vendor 000108 and Analytics 001 were not edited. No history was deleted.

Deployment precheck: the new one-active-recovery index deliberately does not silently delete duplicate pre-existing REASSIGNING incidents. Such data must be reviewed before migration. This and real SQL execution remain part of canonical database certification.

Static Dispatch migration-shape test: PASS. Apply/rollback/reapply and runtime constraints: BLOCKED_BY_TEST_INFRASTRUCTURE. Do not deploy code without applying required migrations first.

## 13. Security

- Generic adapters explicitly allowlist schemas; no OTP/QR secret, challenge hash, payment/card credential, Partner credential, raw location or unnecessary receiver contact data is forwarded.
- Payload integrity hashes remain normal envelope metadata; they are not handover-secret hashes.
- Vendor validates source/slot/parcel ownership from its own records and rejects changed replay fingerprints.
- Analytics binds event source to authenticated caller and accepts only controlled minimal route metrics.
- Existing HMAC v2 client, allowlist, timeout/replay conventions retained; no new auth system.
- Replacement cannot assign the old provider, bypass Fleet eligibility, create a second Trip or move custody automatically.
- Logs/counters use controlled outcomes and fixed dimensions, not payload bodies or secrets.
- Database race/nonce guarantees are not marked runtime-certified without execution.

## 14. Backward compatibility

Booking -> Request -> Trip, broadcast matching, provider inbox/accept/decline, Trip lifecycle, proof/recipient confirmation/failed delivery, Wallet, location, commercequote, existing CommerceMission and Fleet/private paths remain on their existing authorities.

Deliberate changes: critical notification transport becomes durable; inbox offers can optionally expose `purpose=PROVIDER_REPLACEMENT` and `incident_ref`; replacement accept returns the existing Trip with `REPLACEMENT_READY`, not permission to complete the old provider's Trip. Existing apps' ordinary offers do not require these optional fields.

Routes added/removed: ZERO. Vendor's existing Dispatch event contract still has 1 operation; Analytics still has 3 domain operations; the Dispatch assignment/planning contract retains 13 operations. All three edited OpenAPI documents parse, and Dispatch routing/contract tests pass.

## 15. Test results

All commands below were executed from `C:/KarryGoPush`, with `GOCACHE=C:/KarryGoPush/.codex-cache/go-build`; long output redirected to cache logs.

### Targeted Dispatch / workers / reconciliation

```powershell
go test -count=1 ./services/driver-dispatch-delivery-service/internal/features/assignment ./services/driver-dispatch-delivery-service/internal/features/booking/... ./services/driver-dispatch-delivery-service/internal/features/trip ./services/driver-dispatch-delivery-service/internal/features/securehandover ./services/driver-dispatch-delivery-service/internal/features/routeintegrity ./services/driver-dispatch-delivery-service/internal/features/deliveryrecovery ./services/driver-dispatch-delivery-service/internal/features/marketplace ./services/driver-dispatch-delivery-service/internal/router ./services/driver-dispatch-delivery-service/migrations
```

PASS, 10 tested packages. Database-gated tests skipped. Log: `.codex-cache/gap-closure-final-targeted.log`.

### Vendor / Analytics / Notification / shared security and Wallet contracts

```powershell
go test -count=1 ./services/vendor-service/internal/features/orders ./services/vendor-service/internal/features/fulfilmentgroup ./services/analytics-service/internal/features/commerceanalytics/... ./services/analytics-service/internal/database ./shared/go/commerceanalytics ./shared/go/dispatchcommerce ./shared/go/notifications ./shared/go/serviceauth ./shared/go/walletclient
```

PASS, 11 tested packages and one package without tests. Log: `.codex-cache/gap-closure-cross-service.log`. This is not a full Vendor/Notification regression.

Additional final Analytics wire normalization/signed-source confusion tests:

```powershell
go test -count=1 ./services/analytics-service/internal/features/commerceanalytics/... ./services/analytics-service/internal/database
```

PASS, 4 tested packages, one without tests. Log: `.codex-cache/gap-closure-analytics-final.log`.

### Full Dispatch — one final invocation

```powershell
go test -count=1 -json ./services/driver-dispatch-delivery-service/...
```

PASS: 27 tested packages; 11 packages without tests; 1,072 top-level test passes (1,351 including subtests); 124 top-level skips (130 including subtests); 0 failures. Log: `.codex-cache/gap-closure-full-dispatch.jsonl`. Counts come from this execution, not previous reports.

### PostgreSQL / concurrency

BLOCKED_BY_TEST_INFRASTRUCTURE. Required canonical variables were unset: `DISPATCH_TEST_DATABASE_URL`, `VENDOR_SERVICE_TEST_DATABASE_URL`, `ANALYTICS_SERVICE_TEST_DATABASE_URL`, `ANALYTICS_TEST_DATABASE_URL`.

New ready-to-run database tests include:

- TestLifecycleNotificationIntentAtomicRollbackAndDedupPostgres
- TestIncidentRecoveryClaimReplayAcceptanceAndNoSecondTripPostgres
- TestLifecycleMigrationApplyRollbackReapplyPostgres
- TestReplacementCancelledOrExpiredNeverCreatesOfferPostgres
- TestVendorLifecycleProjectionIsTransactionBoundPostgres
- TestDispatchLifecycleDuplicateStaleAndForeignParcelPostgres
- TestDispatchRouteIngestReplayPostgres

Previously written canonical outbox, Wallet, secure notification, schedule, queue and manifest/race tests remain in the full suite. Skipping is not a PostgreSQL PASS. Vendor integration additionally requires the repository's existing plan fixture; its absence is a skip, not fabricated certification.

### Vet / build

```powershell
go vet ./services/driver-dispatch-delivery-service/... ./services/vendor-service/... ./services/analytics-service/... ./shared/go/commerceanalytics ./shared/go/dispatchcommerce
go build ./services/driver-dispatch-delivery-service/... ./services/vendor-service/... ./services/analytics-service/... ./shared/go/commerceanalytics ./shared/go/dispatchcommerce
```

Both PASS (exit 0). Logs: `.codex-cache/gap-closure-vet.log`, `.codex-cache/gap-closure-build.log`.

### Formatting / diff / contracts

`gofmt -l` over all 37 Go files in the closure file manifest: PASS, no unformatted files.
`git diff --check`: PASS, exit 0. Existing Git CRLF conversion warnings are not whitespace failures.
All three edited OpenAPI JSON files parse. No new HTTP route.

## 16. Remaining gaps

NO REMAINING SECTIONS 15–16 IMPLEMENTATION GAPS.

POSTGRESQL RUNTIME CERTIFICATION REMAINS INFRASTRUCTURE-BLOCKED.

Deployment prerequisites are still real: apply new migrations on canonical test infrastructure, run races/rollback tests, configure approved service callers/secrets and enable selected target flags. These were not executed against production.

Frontend handoff is documentation only: display replacement-ready as awaiting verified transfer, preserve current custody, distinguish queued/unknown/delivered notification states, and do not infer payment/refund completion from an operational event.

## 17. Final verdict

SECTIONS 15–16 CLOSED — READY FOR SECTION 17

This is implementation closure with available non-database evidence, not PostgreSQL/runtime rollout certification. Section 17 has not been started.
