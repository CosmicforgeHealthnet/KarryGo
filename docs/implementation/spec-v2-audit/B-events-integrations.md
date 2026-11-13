# Fleet backend vs. Corporate Logistics Spec v2.0: Sections 26, 27, 60, 61, 62

This is a read-only audit of `services/fleet-partner-service` and the counterpart services. No repository file was changed. Paths are relative to `services/` unless they start with `shared/`, `web/` or `apps/`. `fps/` means `fleet-partner-service/`. All flags shown are the code defaults from `fps/internal/config/config.go` unless stated otherwise.

Status values:
- **IMPLEMENTED**: both sides are real and the shapes match.
- **PARTIAL**: one side is missing, a shape or name mismatches, or the work is done by a different mechanism.
- **MISSING**: nothing exists.
- **EXTERNAL**: owned by another service.
- **N/A**: not applicable.

## Summary

| Section | Implemented | Partial | Missing | Notes |
|---|---|---|---|---|
| 26 Events, contracts, data sync (28 rows) | 1 | 15 | 12 | The outbox and inbox work. Fleet's Redis stream `fleet-partner:events` has **no external consumer**, and no service publishes onto it. Only 6 of the 13 "produced" events are emitted under their exact name, and no other service consumes any of them. Of the 13 "consumed" events, only `media.asset.status_changed.v1` has a real producer with a matching path. **`platform.request.offered.v1` never creates an opportunity: no production code creates request projections or offers.** |
| 27 Jobs, leases, scheduling (20 rows) | 8 | 8 | 4 | The durable queue, attempts, heartbeat, owner-checked completion and shutdown release are real. **The `scheduled_tasks` runner is never started**, so the scheduled-task rows and "run-now" do nothing. The reassignment coordinator only raises alerts. Report and analytics workers are absent. |
| 60 Cross-service contracts (16 rows) | 3 | 10 | 3 | Hauling and Taxi execution contracts match Fleet's client. **The Dispatch execution contract does not match**: snake_case vs camelCase bodies, a wrong eligibility route, no trip-cancel route, and reservation looked up by request ID. **Organization verification is a dead end**: status values disagree (`APPROVED` vs `VERIFIED`), there is no read-back, no event consumer, and nothing calls the organization `TryActivate`. Provider identity resolution exists only in Hauling. |
| 61 Event architecture and catalogue (14 rows) | 0 | 11 | 3 | Event names follow 26.2-style `x.y.v1` names, not the 61 catalogue. The envelope lacks `causationId`, `actor`, `requestId`, `traceparent` and `metadata`. Branch, user-role, rescue, warehouse-receipt, putaway, pick, report and automation-execution events are not produced. Unsupported-schema events are acknowledged and dropped instead of dead-lettered. |
| 62 Durable workers and recovery (29 rows) | 9 | 13 | 7 | Most workers exist behind default-off flags. Offer expiry, wallet-entry projection reconcile, fuel anomaly, report generation, analytics snapshot and real automation evaluation are missing. Several workers start only in `cmd/worker`, not in the API binary's in-process worker. |

### The ten most consequential findings

1. **Marketplace opportunities can never appear in production.** `ProjectionRepository.Upsert` (`fps/internal/features/marketplace/repositories/projections.go:50`) and `OfferRepository.Create`/`CreateRecipient` (`offers.go:36,176`) have no production caller. Only `internal/integration/*_test.go` seeds them. `IngestEvent` for `platform.request.offered.v1` looks up an existing projection and returns `IGNORED / projection_not_found` when there is none (`usecases/event_ingestion.go:202-208`). So Dispatch's real `platform.request.offered.v1` delivery is acknowledged and dropped, and the whole "organization accepts request, then assigns" chain (26.2/26.3/61.3) has no entry point.
2. **Organization verification and activation can never complete** (the known issue, confirmed):
   - Fleet's activation gate and durable eligibility both require `status == "APPROVED"` (`fps/internal/features/organizations/usecases/submission_activation.go:358`, `fps/internal/features/eligibility/usecases/checks.go:172`).
   - `verification-compliance-service` only accepts `PENDING|VERIFIED|REJECTED|SUSPENDED` (`verification-compliance-service/internal/features/organizations/http.go:157`).
   - Fleet's local status only ever moves to `SUBMITTED`, `PENDING_DISPATCH` or `UNKNOWN_OUTCOME` (`submission_activation.go:214-233`).
   - `VerificationClient.ReadOrganizationStatus` (`fps/internal/platformclients/verification.go:41`) has no caller, and no worker reconciles `UNKNOWN_OUTCOME`/`PENDING_DISPATCH`.
   - Verification writes `organization.verification.status_changed.v1` to its outbox (`http.go:206`), but its only outbox worker delivers vendor callbacks (`verification-compliance-service/internal/features/vendors/callbacks.go:46-81`). Fleet registers no consumer for that event.
   - `organizations/usecases.(*Service).TryActivate` (`submission_activation.go:405`) has no caller at all. The only `TryActivate` calls are the provider-membership one (`providers/usecases/lifecycle.go:78`, `reconciler.go:131`).
   - Nobody calls Verification's `POST .../submissions/:id/decisions` (no admin-backoffice caller either).
3. **Fleet's produced events go nowhere.** The outbox worker publishes to Redis stream `fleet-partner:events` (`fps/internal/events/redis_publisher.go:16`, `cmd/main.go:141-153`). No other service reads that stream (repo-wide grep for `fleet-partner:events` finds only Fleet), and no service consumes any 26.2 event name. The only consumers are Fleet's own webhook fan-out (`fps/internal/features/integrations/worker.go:123-128`) and Fleet's own Section-26 stream consumer, which only handles *consumed* types.
4. **Dispatch execution contract mismatch (60.3).** The fleet `DispatchExecution` client (`fps/internal/platformclients/dispatch_execution.go`) speaks the snake_case dialect that Hauling and Taxi implement:
   - Dispatch's `/api/v1/internal/marketplace` expects camelCase `requestId`/`expectedRequestVersion` and returns `reservationId` (`driver-dispatch-delivery-service/internal/features/marketplace/http.go:219-226`, `service.go:148`). Fleet sends `source_request_id` and reads `reservation_id`.
   - Fleet calls `GET /eligibility?…`, but Dispatch only has `POST /eligibility/providers/validate`.
   - Fleet calls `POST /trips/:id/cancel`, which Dispatch does not have.
   - Fleet calls `GET /reservations/{sourceRequestId}?idempotency_key=`, but Dispatch keys that route on the reservation ID and has `/reservations/by-idempotency/:key` instead.
   - `ReadRequest`/`ReadTrip` decode snake_case but Dispatch returns camelCase.
   - Only `POST /executions` and `GET /executions/by-idempotency/:key` match.
5. **Taxi publishes no marketplace or trip events to Fleet.** It sends only completed-work (`driver-taxi-service/internal/outbox/completedwork.go:24`). Fleet's registry also names Taxi's producer `taxi-service` (`fps/internal/events/registry.go:220-227`), while Taxi signs as `driver-taxi-service` (`driver-taxi-service/internal/outbox/publisher.go:356`).
6. **No producer exists anywhere** for `provider.availability.changed.v1`, `provider.verification.changed.v1`, `vehicle.verification.changed.v1`, `wallet.entry.posted.v1` or `support.case.changed.v1`. Fleet built consumers and reconciliation authorities for them. Wallet sends `finance.*`/`wallet.*` events to a different endpoint, `/finance/events`.
7. **The scheduled-task runner is never constructed outside tests.** `scheduler.Runner` is built only in `fps/internal/integration/scheduler_section27_test.go:236`. The tenant and operator `/scheduled-tasks/*` routes (including `run-now` and `enable`) edit rows that nothing executes.
8. **The offer-expiry worker is missing.** `OfferRepository.ExpireDue` (`offers.go:140`) exists but is not in the maintenance sweep map (`fps/cmd/main.go:160-166`) and has no other caller.
9. **The support integration is half-wired.**
   - Fleet's `SupportDisputeReferenceClient.ValidateIncident` calls `GET /admin/complaints/:id`, but Support restricts that route to `admin-backoffice-service` (`support-dispute-service/internal/features/support/http/routes.go:66-71`), so Fleet always gets 403.
   - Support never emits case events to Fleet's `/support/events`.
   - Appending messages or evidence to organization cases is not offered.
10. **Notification integration covers only communication messages and escalations** (`fps/internal/features/engagement/communications.go:254`, `workers.go:305`). Assignment, invitation-to-provider (outside the auth email path) and alert notifications are not emitted. No realtime audience is registered.

---

## Section 26 - Events, service contracts and data synchronization

### 26.1 Envelope

| Item | Status | Evidence / gap |
|---|---|---|
| Versioned envelope through the transactional outbox, inbox dedupe by event ID | PARTIAL | `fps/internal/events/envelope.go:20-32` has eventId, eventType, schemaVersion, aggregateType/Id/Version, organizationId, correlationId, occurredAt, producer and payload. **`causationId` is missing.** `outbox.go:73-99` appends inside the caller's transaction, and the inbox is keyed (consumer, eventId) with a payload hash (`inbox.go:53`). Inbound producers (Dispatch, Hauling) use a different envelope with `sourceService` and `payloadHash` (`marketplace/usecases/event_ingestion.go:81-94`), so there are two envelope dialects. |

### 26.2 Core produced events (Fleet produces, others consume)

Fleet-side evidence is the `Publish(...)` call site. Nobody consumes these: `fleet-partner:events` has no external reader (see finding 3), so each row is at best PARTIAL.

| Event | Fleet produces? | Counterpart consumes? | Status |
|---|---|---|---|
| organization.created.v1 | Yes: `organizations/usecases/onboarding_service.go` | No. Wallet, verification, notification and analytics do not consume it; Wallet only has its own `wallet.organization.created.v1`. | PARTIAL |
| organization.status_changed.v1 | **No.** Transitions emit specific types instead (`organization.activated/suspended/under_review/changes_required/closed/closure_requested.v1`). Registry only. | No | MISSING (renamed) |
| organization.provider.provisioned.v1 | **No** (registry only) | No | MISSING |
| organization.provider.membership_changed.v1 | **No.** Emits `organization.provider.invited/association_accepted/association_declined/activated/suspended/reactivated/ending/removed.v1` (`providers/usecases/lifecycle.go`, `provider_service.go`). | No. Execution services learn about membership through Fleet's internal provider reads (`/internal/fleet-partner/v1/providers/:id/memberships`) and the restriction API. | MISSING (renamed) |
| organization.request.accepted.v1 | Yes: `marketplace/usecases/dispatcher.go` | No (source reservation is a synchronous call instead) | PARTIAL |
| organization.assignment.created.v1 | Yes: `marketplace/usecases/assign.go` | No. No provider notification; provider apps poll. | PARTIAL |
| organization.assignment.accepted.v1 | Yes: `assignment_service.go` | No. Execution is created by a synchronous `POST /executions`. | PARTIAL |
| organization.assignment.declined.v1 | Yes: `assignment_service.go` | No | PARTIAL |
| organization.assignment.expired.v1 | Yes: `assign.go` (expiry) | No | PARTIAL |
| organization.asset.blocked.v1 | **No.** Emits `asset.blocked.v1` (`assets/service.go:527`, `operations.go:662`, `safety_declines.go:92`). | No. Synchronous `/internal/restrictions/apply` exists for Dispatch and Hauling only (`fps/internal/app/wiring.go:397-400`). | MISSING (renamed) |
| corporate.order.confirmed.v1 | **No.** Emits `organization.orders.order.confirmed.v1` (`orders/usecases/service.go:789`). | No | MISSING (renamed) |
| corporate.shipment.status_changed.v1 | **No.** Only `organization.shipments.shipment.created.v1` and `shipment.status_projected.v1`. | No | MISSING |
| corporate.invoice.issued.v1 | **No.** Only `invoice.created.v1` and `invoice.updated.v1`. | No | MISSING |

Side effect: `publicIntegrationEvents` (`fps/internal/events/registry.go:260-270`) offers organization webhooks five event types that are never produced: `organization.status_changed`, `membership_changed`, `corporate.order.confirmed`, `corporate.shipment.status_changed` and `corporate.invoice.issued`. Subscribers to those never receive anything.

### 26.3 Core consumed events (others produce, Fleet consumes)

Fleet has two ingestion paths:
- **HTTP**: `POST /internal/fleet-partner/v1/events` (`marketplace/http/event_handlers.go:26`). It needs `FLEET_MARKETPLACE_EVENT_INGESTION_ENABLED=false` set to true, plus `FLEET_MARKETPLACE_ALLOWED_CALLERS=""` populated.
- **Redis**: the Section-26 stream consumer (`app/section26_workers.go:21-57`). It needs `FLEET_SYNCHRONIZATION_ENABLED=false` set to true and reads `fleet-partner:events`, which only Fleet writes. In practice this path is dead.

| Event | Producer side | Fleet side | Status |
|---|---|---|---|
| platform.request.offered.v1 | Dispatch: yes (`driver-dispatch-delivery-service/internal/features/request/events_outbox.go:20`, delivered by HTTP to `/internal/fleet-partner/v1/events`, `DISPATCH_EVENT_DELIVERY_ENABLED=false`/`DISPATCH_MARKETPLACE_OUTBOX_ENABLED=false`). Hauling: **no**. Taxi: **no**. | Accepted, but it only updates an existing projection. **No code creates one** (finding 1), so the result is always `IGNORED projection_not_found`. | PARTIAL (functionally MISSING) |
| platform.request.cancelled.v1 | Dispatch yes. Hauling and Taxi no. | Handled (`event_ingestion.go:347-351`, cancels offer/assignment) for existing projections | PARTIAL |
| provider.availability.changed.v1 | **No producer** in any service | Registry + sync authority (Dispatch/Hauling authority lookup, `app/section26_adapters.go:100-118`) | MISSING (producer) |
| provider.verification.changed.v1 | **No producer** (Verification emits only `organization.verification.*` and `verification.vendor_case.changed.v1`) | Registry + authority | MISSING (producer) |
| trip.created / status_changed / completed.v1 | Dispatch yes (`marketplace/events.go:23-25`). Hauling yes (`fleetauthority/http.go:352,938-942`, `HAULING_FLEET_INTEGRATION_ENABLED=false`, `HAULING_FLEET_EVENT_URL`). **Taxi no.** | `trip_projection_ingestion.go` projects them, but trips resolve their organization through the (never created) request projection or the private-execution mapping. Works for **private** execution (payload `fleetExecutionOperationId`). | PARTIAL |
| vehicle.verification.changed.v1 | **No producer** | Registry + authority | MISSING (producer) |
| wallet.entry.posted.v1 | **No producer.** Wallet emits `finance.*`/`wallet.*` to `/internal/fleet-partner/v1/finance/events` (`payment-wallet-service/internal/events/http_publisher.go:28`, `PAYMENT_WALLET_FINANCE_OUTBOX_ENABLED=false`). | Section-26 authority calls `ReadProjection("WALLET_ENTRY")`, which Wallet's `/operations/:type/:id` does not support (`payment-wallet-service/internal/features/finance/http/handler.go:508-587`), so reconciliation always fails. The finance `/finance/events` path is the real consumer (`fps/internal/features/finance/ingestion.go:132-149`). | MISSING (as specified); alternative path PARTIAL |
| settlement.created / completed.v1 | Not under these names. Wallet emits `finance.settlement.processing/succeeded/failed.v1` and `wallet.settlement.status_changed.v1`. | Consumed through `/finance/events` (projection type SETTLEMENT); the 26 names are never seen | PARTIAL |
| support.case.changed.v1 | **No producer.** support-dispute-service never posts to Fleet. | `/internal/.../support/events` route (`engagement/http.go:255`, `SUPPORT_PROJECTIONS_ENABLED=false`) + polling reconcile `SECTION23_SUPPORT_RECONCILE` | PARTIAL |
| media.asset.status_changed.v1 | Media yes (`media-file-service/internal/events/envelope.go:56`, `http_publisher.go:26` to `/internal/fleet-partner/v1/documents/media-events`) | `documents/http/internal.go:41` (`FLEET_DOCUMENTS_EVENT_INGESTION_ENABLED=false`) | IMPLEMENTED |

Additional Dispatch/Hauling trip events Fleet accepts but the spec does not list: `trip.cancelled`, `proof.completed`, `trip.recipient_confirmed`, `trip.delivery_failed`, `platform.request.updated/reserved/reservation_released/expired`.

### 26.4 Reconciliation

| Item | Status | Evidence / gap |
|---|---|---|
| Every external projection has a periodic reconciliation worker | PARTIAL | Marketplace request/trip reconciliation exists (`marketplace/usecases/reconciliation_worker.go`, `FLEET_MARKETPLACE_RECONCILIATION_ENABLED=false`), but it reads Dispatch with mismatched shapes (finding 4). The Section-26 synchronization worker only reconciles *opened cases*, never a periodic sweep (`synchronization/service.go:255`, `FLEET_SYNCHRONIZATION_WORKER_ENABLED=false`). There is no organization-verification read-back. Finance projections, documents, notifications and support have their own reconcilers (default off). |

---

## Section 27 - Background jobs, leases and scheduled processing

### Entities

| Entity | Status | Evidence |
|---|---|---|
| outbox_events | IMPLEMENTED | `fleet_partner_outbox`; `events/outbox.go` (attempts, published_at, DEAD_LETTERED after 10, `publisher.go:39`) |
| consumer_inbox | IMPLEMENTED | `fleet_partner_inbox`; `events/inbox.go` (consumer + event_id, payload hash, PROCESSING/PROCESSED/FAILED) |
| jobs | IMPLEMENTED | `fleet_partner_jobs`; `jobs/jobs.go:121-235` (priority, scheduled_at, dedupe, max_attempts) |
| job_attempts | IMPLEMENTED | `jobs.go:293` `RecordAttempt` (`fleet_partner_job_attempts`) |
| scheduled_tasks | PARTIAL | Table `migrations/000064_section27_scheduled_tasks.up.sql:7` and repository `internal/scheduler/scheduler.go` (ClaimDue SKIP LOCKED, RunNow). **`scheduler.Runner` is never started** outside `internal/integration/scheduler_section27_test.go:236`. Recurring work instead runs on ad-hoc goroutine tickers. |

### 27.1 Lease pattern

| Rule | Status | Evidence / gap |
|---|---|---|
| Recover expired CLAIMED/RUNNING leases in a separate short transaction | PARTIAL | Recovery is folded into the claim statement (`jobs.go:200-205`, `OR (status IN (CLAIMED,RUNNING) AND lease_expires_at < now())`) rather than a separate transaction. Execution-start and marketplace reconciliation do have separate recovery (`RecoverExpiredLeases`). |
| Claim with FOR UPDATE SKIP LOCKED | IMPLEMENTED | `jobs.go:208`, `outbox.go:112-113`, sagas, synchronization cases |
| Lease owner + expiry; only owner heartbeats/completes | IMPLEMENTED | `jobs.go:257-290,313-355` (`lease_owner = $owner`) |
| Graceful shutdown stops claiming, heartbeats, releases safe leases | IMPLEMENTED | `jobs/worker.go:198-222` (`ReleaseForShutdown`) |
| Unknown external outcomes require read-back before retry | PARTIAL | Wallet, finance, payroll, execution start, notification and provider saga read back (e.g. `financeops/payroll.go:435-447`). **Organization verification does not** (`submission_activation.go:214-224` marks UNKNOWN_OUTCOME and nothing resolves it). Claims do not filter `attempts < max_attempts` (spec 62.1). |

### 27.2 Required workers

| Worker | Status | Where / flag | Gap |
|---|---|---|---|
| Outbox publisher | PARTIAL | `events.OutboxWorker`, both binaries (`cmd/main.go:184`, `cmd/worker/main.go:138`). Redis stream when Redis is configured, else `DisabledPublisher`. Stopped only by `FLEET_INTEGRATIONS_KILL_SWITCH`. | Ordered by `available_at`, not aggregate/version (`outbox.go:112`). No external consumer (finding 3). `cmd/main.go:204` logs "transport: disabled adapter" even when Redis is live. |
| Assignment expiry | IMPLEMENTED | `app.StartAssignmentExpiryWorker`, `ASSIGNMENT_EXPIRY_WORKER_ENABLED=false` | - |
| Reassignment coordinator | PARTIAL | `StartReassignmentStalenessWorker`, `REASSIGNMENT_WORKER_ENABLED=false`; `FLEET_REASSIGNMENT_ENABLED` / `FLEET_AUTOMATIC_REASSIGNMENT_ENABLED` = false | Alerts only. Next-candidate drafts happen only when automatic reassignment is on. |
| Provider provisioning reconciliation | IMPLEMENTED | `app.RunProviderReconciler` (`reconciliation_workers.go:73`) on both binaries; always on unless kill switch | In-process ticker; saga claim via SKIP LOCKED (`providers/repositories/sagas.go:215`). Only Hauling has an identity resolver (see 60). |
| Marketplace projection reconciliation | PARTIAL | `StartMarketplaceReconciliation`, `FLEET_MARKETPLACE_RECONCILIATION_ENABLED=false` | Dispatch read-back decodes the wrong shape (finding 4). |
| Document expiry | IMPLEMENTED | `StartDocumentWorkers`, `FLEET_DOCUMENTS_EXPIRY_WORKER_ENABLED=false` | - |
| Preventive maintenance scheduler | IMPLEMENTED | `assets` job `section18.maintenance` (`assets/operations.go:613-640` creates work orders), `SECTION18_WORKERS_ENABLED=false` | - |
| Recurring order generator | IMPLEMENTED | `StartSection15Workers`, `RECURRING_ORDERS_WORKER_ENABLED=false` | - |
| Notification dispatcher | PARTIAL | `SECTION23_NOTIFICATION_SUBMIT/RECONCILE` (`engagement/workers.go:32-33`), `SECTION23_WORKERS_ENABLED=false` | Only communication-message and escalation intents. No assignment, alert or invitation intents. |
| Bulk import processor | IMPLEMENTED | Section 15 bulk/import validation/commit/retry, `BULK_IMPORTS_WORKER_ENABLED`, `IMPORTS_*_WORKER_ENABLED` (all false) | - |
| Export generator | IMPLEMENTED | `StartSection25Workers`, `EXPORTS_WORKER_ENABLED=false` | - |
| SLA evaluator | PARTIAL | `SECTION23_SLA_GENERATE` (`SECTION23_WORKERS_ENABLED`) + CRM `SLA_EVALUATION_WORKER_ENABLED` / `SLA_ESCALATION_WORKER_ENABLED` | CRM workers start **only in `cmd/worker`** (`cmd/worker/main.go:186`), not in the API's in-process worker. |
| Automation runner | PARTIAL (stub) | `AUTOMATION_RUNNER_WORKER_ENABLED=false`; `automation/section27_runner.go` | Always dead-letters "automation domain not implemented". |
| Data retention worker | PARTIAL | `DATA_RETENTION_WORKER_ENABLED=false` (report-only, `FLEET_DATA_RETENTION_DRY_RUN=true`) + `FLEET_PARTNER_RETENTION_EXECUTION_ENABLED=false` (admin-triggered job, `retentionexec/admin.go:168`) | Not a daily anonymizer. |

---

## Section 60 - Cross-service integration contracts

### Common requirements

| Requirement | Status | Evidence / gap |
|---|---|---|
| X-Service-Name, X-Request-ID, X-Correlation-ID, trace context, organization, signed identity | IMPLEMENTED | `fps/internal/platformclients/base.go:431-462` sets `X-Organization-ID`, `X-Request-ID`, `X-Correlation-ID`, `Idempotency-Key`, `X-Causation-ID`, `traceparent` and `X-Service-Nonce`, and signs with HMAC (`serviceauth.SignRequest`, which sets `X-Service-Name`). No mTLS. |
| Retries only for idempotent operations; unknown write goes to reconciliation | IMPLEMENTED | `base.go:343` retries GET only, `DisableRetries` on execution create, `OutcomeUnknown` classification. |
| Per-service timeouts (1.5s/3s read/write etc.) | PARTIAL | A single `FLEET_PARTNER_HTTP_CLIENT_TIMEOUT=10s` and `DISPATCH_SERVICE_TIMEOUT_SECONDS=10` (`config.go:565,850`). No read/write split. |
| 60.1 internal command envelope (commandId, expiresAt, actor, resourceVersion) | PARTIAL | Plain JSON bodies + `Idempotency-Key` header. No commandId, expiresAt or actor envelope. Receivers persist idempotency (Dispatch, Hauling, Taxi, Wallet, Verification command tables). |
| 60.5 failure classification | IMPLEMENTED | Outcomes validation, unauthenticated, forbidden, not_found, conflict, rate_limited, transient, permanent and unknown_outcome (`base.go:68-77`), plus a circuit breaker. A schema/version "quarantine" is missing (see 61.9). |

### Per-service contracts

| Service | Fleet side | Counterpart side | Status | Flags/config needed |
|---|---|---|---|---|
| **driver-dispatch-delivery-service**: reserve, release, eligibility, execute, read, cancel | `platformclients/dispatch_execution.go` (base `/api/v1/internal/marketplace`), always wired (`app/marketplace_wiring.go:28-36`) | `marketplace/http.go:62-76`. **Mismatches:** body casing (`source_request_id` vs `requestId`), response casing (`reservation_id` vs `reservationId`, `request_id`/`trip_id` vs `requestId`/`tripId`), `GET /eligibility` missing (only `POST /eligibility/providers/validate`, `/vehicles/validate`), `POST /trips/:id/cancel` missing, reservation read/release keyed by reservationId not sourceRequestId. Execution create and read-by-idempotency match. | PARTIAL (reservation path broken) | Fleet: `DISPATCH_SERVICE_URL/SECRET`, `FLEET_DISPATCH_EXECUTION_START_ENABLED=false`. Dispatch: `DISPATCH_MARKETPLACE_INTERNAL_API_ENABLED`, `DISPATCH_ORGANIZATION_RESERVATION_ENABLED`, `DISPATCH_ORGANIZATION_EXECUTION_ENABLED` (all false) |
| Dispatch: provider identity resolve/create (60.2) | Provider contract for DISPATCH is `UnconfiguredProvider` (`contracts.go:375-384`; only Hauling wired at `app/wiring.go:265`) | No resolve/sponsored endpoint | MISSING | - |
| Dispatch: provider/vehicle authority, restrictions | `vehicle_authority.go:102`, `restrictions.go` | `/api/v1/internal/authority/{providers,vehicles}/:id` (`authority/authority.go:426-427`), `/api/v1/internal/restrictions/{apply,clear,operations/:id}` | IMPLEMENTED | Fleet `FLEET_PARTNER_*` client config for dispatch |
| **driver-taxi-service**: reserve/execute/read/cancel | `taxi_execution.go` (base `/api/v1/taxi/internal/marketplace`) | `managedexec/http.go:55-64`, same snake/camel dialect as Fleet expects | IMPLEMENTED (gated) | `FLEET_TAXI_INTEGRATION_ENABLED=false` + taxi client; Taxi `TAXI_FLEET_MANAGED_EXECUTION_ENABLED=false`, `TaxiFleetExecutionCallers`. Note: `cmd/worker` execution-start worker never builds a Taxi adapter (`cmd/worker/execution_start_worker.go:38-53`). |
| Taxi: provider identity, vehicle authority, restrictions | Unconfigured | No resolve, authority or restrictions endpoints | MISSING | - |
| Taxi: events (ride.*, trip.*, availability.*) | Registry expects producer `taxi-service` | Taxi publishes only completed-work to Fleet and signs as `driver-taxi-service` | MISSING | - |
| **driver-hauling-service**: reserve/eligibility/execute/read/cancel/transition | `hauling_execution.go` (base `/api/v1/hauling/internal/marketplace`) | `fleetauthority/http.go:36-58`, matching dialect | IMPLEMENTED | Fleet `FLEET_HAULING_INTEGRATION_ENABLED=false` + client; Hauling `HAULING_FLEET_INTEGRATION_ENABLED=false`, `HAULING_FLEET_ALLOWED_CALLERS`, `HAULING_FLEET_EVENT_URL/SECRET` |
| Hauling: provider identity resolve/sponsor (60.2) | `hauling_provider.go:38-51` | `POST /providers/resolve` returns `MATCHED/NOT_FOUND/CONFLICT` (`fleetauthority/http.go:440-470`), not `no_match/single_match/split_match/ambiguous_match`. Requires both email and phone. A non-`ErrNoRows` DB error is treated as no-match. | PARTIAL | as above |
| Hauling: events | HTTP `/events` ingestion | Emits `platform.request.reserved`, `trip.created/status_changed/completed` only. **No `platform.request.offered` or `cancelled`.** | PARTIAL | as above |
| **payment-wallet-service**: provision org wallet, provider sub-wallet, allocate/reverse/hold/release, read balance, resolve operation | `payment_wallet.go` (base `/api/v1/payment-wallet/internal/finance`) | `finance/http/handler.go:61-94`, `provider_operations.go:16-24`; read-back `/operations/:type/:id` and `/operations/:type/by-idempotency` (incl. PROVIDER_ALLOCATION). Path naming differs from spec `/internal/v1/wallet-operations`, but the semantics (operationId plus idempotency, query until terminal) are met. `WALLET_ENTRY` read type is unsupported. | PARTIAL (functionally IMPLEMENTED for operations) | Fleet `FLEET_PARTNER_PAYMENT_WALLET_SERVICE_*`, `FLEET_FINANCE_INTEGRATION_ENABLED`, `PROVIDER_SUBWALLET/ALLOCATION/WITHDRAWAL_ENABLED`, `FINANCE_POSTING_ENABLED` (false). Wallet `PAYMENT_WALLET_FINANCE_PRIVATE_API_ENABLED=false`, `PAYMENT_WALLET_FINANCE_ALLOWED_CALLERS` (includes Fleet), `PAYMENT_WALLET_FINANCE_OUTBOX_ENABLED=false` + `..._TARGET_URL`. Fleet `FLEET_FINANCE_EVENT_INGESTION_ENABLED=false`, `FLEET_FINANCE_ALLOWED_CALLERS=payment-wallet-service`. |
| Wallet events (wallet.*, ledger_entry.*, settlement.*, withdrawal.*, allocation.*) | `/finance/events` maps by substring (`finance/ingestion.go:132-149`) | Wallet emits `finance.*` + `wallet.*` | PARTIAL | `wallet.provider_allocation.*`, `wallet.provider_subwallet.*`, `wallet.withdrawal.*` and `wallet.managed_settlement.*` map to no projection type (`""`). There is no ledger_entry event. |
| **notification-service**: send invitation/assignment/alert; register realtime; read delivery | `engagement` adapter `POST /api/v1/notifications/send`, `GET /messages/by-idempotency/:key` (`app/section22_23_adapters.go:221-254`); auth OTP/invitation email via `shared/go/notifications` (`auth/clients/otp_sender.go`); CRM notifier (`crm_wiring.go:175`) | `messages/http/routes.go:19-24` (`/send`, `/messages/by-idempotency/:key`, `/realtime/token`) | PARTIAL | No assignment or alert notifications; no realtime audience registration. Fleet `FLEET_PARTNER_NOTIFICATION_SERVICE_BASE_URL/SECRET`, `COMMUNICATIONS_ENABLED`, `NOTIFICATION_RULES_ENABLED`, `SECTION23_WORKERS_ENABLED` (false); notification-service `NOTIFICATION_SERVICE_SECRETS` must include `fleet-partner-service` (no default). |
| **media-file-service**: validate file, signed URL, metadata/hash, retention/legal hold; events | `media_file.go` (base `/api/v1/media-files/internal/media`) | `mediaobjects/http/private.go:117-130`; event `media.asset.status_changed.v1` to `/documents/media-events` | IMPLEMENTED | `FLEET_PARTNER_MEDIA_FILE_SERVICE_*`, `MEDIA_FILE_PRIVATE_ALLOWED_CALLERS=fleet-partner-service` (default), `FLEET_DOCUMENTS_EVENT_INGESTION_ENABLED=false`, `FLEET_DOCUMENTS_ALLOWED_CALLERS=media-file-service` |
| **verification-compliance-service**: submit company/provider/asset evidence, read status, events | `verification.go` submit + read (read unused) | `organizations/http.go:24-28` submit/read/decide; status vocabulary `PENDING/VERIFIED/REJECTED/SUSPENDED`; emits `organization.verification.status_changed.v1` into an outbox that is **never delivered** | PARTIAL (organization activation impossible) | **Mismatch:** Fleet expects `APPROVED`/`CHANGES_REQUIRED`/`UNDER_REVIEW`. **Missing:** Fleet read-back worker, Fleet event consumer, a caller of organization `TryActivate`, provider/asset verification contracts, and an operator path that calls `/decisions`. Config: `FLEET_PARTNER_VERIFICATION_COMPLIANCE_SERVICE_*`, `VERIFICATION_COMPLIANCE_ALLOWED_CALLERS` (includes Fleet). |
| **support-dispute-service**: open case, append message/evidence, link refs, query; events | `supportCaseAdapter` create/read `/admin/organization-cases[/:id]` (`section22_23_adapters.go:258-296`); `SupportDisputeReferenceClient.ValidateIncident` `GET /admin/complaints/:id` | `routes.go:66-71`: organization-cases allowed for Fleet; **`/admin/complaints/:id` allowed only for admin-backoffice, so Fleet gets 403**. No append-message or evidence route for org cases. No events to Fleet. | PARTIAL | `FLEET_PARTNER_SUPPORT_DISPUTE_SERVICE_*`, `SUPPORT_PROJECTIONS_ENABLED=false`, `SUPPORT_DISPUTE_SERVICE_SECRETS` (includes Fleet) |
| **analytics-service**: publish curated events, request reports; events | **None.** Client config exists (`config.go:911`) but no caller. | Ingest only `POST /internal/analytics/v1/commerce-events`; network read endpoints | MISSING | `FLEET_PARTNER_ANALYTICS_SERVICE_*` |
| **api-gateway**: route corporate web/API, edge rate limits, separation from Partner API | Fleet serves `/fleet-partner/v1`, org API clients separate | `api-gateway/internal/features/routing/models/routes.go:83` routes `/fleet-partner/`; internal paths excluded (`:130`) | IMPLEMENTED | Gateway upstream config |
| **admin-backoffice-service**: review package, platform decision, emergency restriction/lift, audit export | `adminresources` read sources (`adminresources/source.go`) + `admininvestigations` commands (OPEN/RECORD_FINDING/CLOSE) via `shared/go/adminowner`/`admincommand`, `FLEET_PARTNER_ADMIN_COMMAND_*` default off | admin-backoffice lists Fleet as `POLICY_REQUIRED` (`hqops/service.go:347`). No organization-approval, emergency-restriction or audit-export command. | PARTIAL | `FLEET_PARTNER_ADMIN_COMMAND_*` |

---

## Section 61 - Event architecture and complete event catalogue

### Envelope and processing rules

| Rule | Status | Evidence / gap |
|---|---|---|
| 61 envelope (schemaVersion, nested aggregate, actor, requestId, causationId, traceparent, data, metadata) | PARTIAL | Fleet envelope (`events/envelope.go:20-32`) has a flat aggregate and `payload` (not `data`). **Missing: actor, requestId, causationId, traceparent, metadata.classification/retentionClass.** Type names carry `.v1` (26.2 style) rather than 61 names (`assignment.accepted` + schemaVersion). |
| Durable inbox keyed by consumer + eventId; idempotent handlers | IMPLEMENTED | `inbox.go`; per-producer consumer names (`event_ingestion.go:142`, `stream_consumer.go:237`) |
| Per-aggregate ordering, gap detection, reconcile rather than guess | PARTIAL | Marketplace and synchronization detect gaps by sourceVersion (`synchronization/service.go:93-137`, `projections.go:388`). The outbox does not publish in aggregate order (`outbox.go:112`). Several emitters pass a constant aggregateVersion (e.g. `assets/safety_declines.go:92` uses `1`). |
| 61.9 producers validate against schema / contract tests | PARTIAL | `registry.ValidateEnvelope` (strict decode, forbidden fields) applies to *consumed* stream events. `section26contract/asyncapi_test.go` checks the registry and envelope. Produced events outside the 26 registry (most of the catalogue) have no schema. |
| 61.9 unsupported schema goes to dead-letter + alert + stop | PARTIAL | The stream consumer acknowledges and drops contract violations with a metric (`stream_consumer.go:225-230`). HTTP ingestion returns 400 so the *producer* dead-letters. Synchronization has its own dead-letter table. |
| 61.9 PII minimization | IMPLEMENTED | Forbidden-field lists (`registry.go:273-276`); Dispatch/Hauling have their own deny-lists |
| 61.9 outbox retention > replay window; backfill=true | PARTIAL | Replay tool `POST /operations/outbox/replay` (`backgroundops/http/internal.go:76`). No `backfill` flag in the envelope; no outbox retention policy found. |

### Catalogue coverage (Fleet as producer, unless noted)

Fleet's emitted names are listed. None of them is consumed by another service (finding 3), so "counterpart" is "none" throughout except where noted.

| Family | Spec events | Fleet emits | Status |
|---|---|---|---|
| 61.1 Organization | created, submitted, activated, restricted, suspended, closed | `organization.created/business_submitted/under_review/changes_required/activated/suspended/closure_requested/closed/updated/contact_verified.v1` | PARTIAL (no `restricted`; `activated` unreachable, see finding 2) |
| 61.1 Branch | created, updated, activated, deactivated | none | MISSING (no branch API; prior audit 8.x) |
| 61.1 User/roles/approvals | organization_user.*, user_roles.changed, approval.* | `organization_user.invited/activated/suspended/reactivated/ended.v1`, `role_binding.created.v1`, `organization.approval.requested/decided/executed/invalidated.v1` | PARTIAL (no `user_roles.changed`, no step_completed/expired) |
| 61.2 Provider membership, role, branch, asset | provider_membership.*, provider_service_role.changed, provider_branch.changed, provider_asset.changed | `organization.provider.invited/association_accepted/association_declined/activated/suspended/reactivated/ending/removed.v1`, `asset.assigned.v1` | PARTIAL (no service-role or branch events) |
| 61.2 Workforce | shift.*, leave.*, training.*, provider_performance.snapshot_created | `shift.created/published/updated.v1`, `shift_assignment.created.v1`, `leave.created.v1`, `training.course_created/course_updated/completed.v1`, `performance_snapshot.generated.v1`, `workforce_profile.*` | PARTIAL (no shift started/ended/cancelled, leave decided, training enrolled/expired) |
| 61.3 Marketplace and execution | organization_offer.*, assignment.*, trip_projection.*, projection.gap_detected/reconciled, rescue_case.* | `organization.request.accepted/declined.v1`, `organization.offer.provider_accepted/declined.v1`, `organization.assignment.created/accepted/declined/expired/cancelled/acknowledged/reassigned/reassignment_required/execution_requested/execution_started/safety_followup_requested.v1`, `private_assignment.*`, `tracking_projection.updated/gap_detected/reconciled.v1` | PARTIAL (no offer received/reserved/expired, no assignment completed, no trip_projection.* names, **no rescue_case**) |
| 61.4 Commercial and orders | customer.*, lead.*, quotation.*, contract.*, sla.*, transport_order.*, shipment.*, package.*, manifest.* | `organization.crm_customer.*`, `organization.customer_credit.*`, `organization.sales_opportunity.*` (lead equivalent), `organization.quotation.*` (10 states), `organization.contract.*` (8), `organization.sla.*`, `organization.orders.order.created/confirmed/cancelled.v1`, `private_order.status_projected.v1`, `organization.shipments.shipment.created.v1`, `shipment.status_projected.v1`, `package.created/scanned/status_changed/exception_recorded.v1`, `manifest.created/closed.v1`, `custody_event.recorded.v1` | PARTIAL (order lifecycle partial; shipment lifecycle events largely missing; manifest departed/arrived/reconciled missing) |
| 61.5 Routing, assets, maintenance | route_plan.*, geofence.*, asset.*, inspection.*, asset_defect.*, work_order.*, stock.*, fuel_transaction.* | `route_plan.created/updated/optimization_requested/optimized/published/replanned.v1`, `route_stop.*`, `geofence.created/updated/activated/deactivated/triggered.v1`, `asset.created/updated/assigned/blocked/returned_to_service/retired/archived.v1`, `inspection.completed/critical_failure.v1`, `work_order.created/updated/approved/completed.v1`, `stock_movement.recorded.v1`, `inventory.*`, `part.reorder_required.v1`, `fuel_record.created.v1`, `breakdown.*` | PARTIAL (no asset_defect.*, no fuel decided/anomaly, no geofence entered/exited/dwell) |
| 61.6 Compliance, safety, proof | verification.*, operational_restriction.*, incident.*, corrective_action.*, claim.*, proof.* | `restriction.applied/clear_requested/cleared/propagation_*.v1`, `incident.created/updated/evidence_attached.v1`, `corrective_action.created.v1`, `claim.created/updated.v1`, `proof_record.created.v1`, `proof_policy.*`, `compliance_requirement.*`, `compliance_status.evaluated.v1`, `safety_report.*`, `organization.document.*` | PARTIAL (verification.* is consumed-side and absent; no corrective_action overdue/completed; claim states collapsed into `updated`) |
| 61.7 Warehouse and terminal | warehouse_receipt.*, putaway_task.*, pick_wave.*, pick_task.*, packing_unit.*, dock_appointment.*, yard_visit.*, yard.gate_event_recorded | `warehouse.created/updated/archived/location_*.v1`, `inventory.received/moved.v1`, `fulfilment_order.created/updated.v1`, `dock_appointment.created.v1`, `yard_visit.created.v1`, `terminal_transfer.created.v1`, `return.created.v1` | MISSING for receipts, putaway, pick and packing; PARTIAL for dock and yard (created only) |
| 61.8 Finance | invoice.*, expense.*, payroll_period.*, wallet_allocation.*, wallet_projection.* | `invoice.created/updated.v1`, `credit_note.issued.v1`, `receivable.*`, `expense.created/updated/reversed.v1`, `budget.created.v1`, `payroll_period.created/calculated/approved/posted/reversed.v1`, `organization.provider_allocation.requested/decided/confirmed/failed.v1`, `organization.provider_withdrawal.*`, `fleet.finance.operation.*`, `fleet.finance.projection.*`, `subcontractor_settlement.created.v1` | PARTIAL (invoice issued/paid/overdue/voided not distinct; no wallet_projection.reconciliation_failed name) |
| 61.8 Communication and governance | support_case.*, notification_rule.*, report_run.*, export.*, import.*, automation_rule.*, automation_execution.*, api_client.*, webhook_subscription.*, webhook_delivery.* | `support_case_projection.created/updated.v1`, `communication_*`, `notification_rule.created/updated.v1`, `bulk_import.created.v1`, `api_client.created/rotated/revoked.v1`, `webhook.subscription_created/updated/secret_rotated/disabled.v1`, `webhook.delivery_succeeded/failed/replayed.v1`, `escalation.created.v1`, `quality_case.*` | PARTIAL (no report_run, no export.* events, no automation_rule/execution, no webhook dead_lettered) |

---

## Section 62 - Durable workers, schedulers and recovery

The API binary runs workers only with `FLEET_PARTNER_WORKER_ENABLED=true` (the default). `cmd/worker` is a dedicated binary (Dockerfile builds both). Unless stated, a worker starts in both binaries.

| Worker | Status | Implementation / registration | Flag (default) | Gap vs spec |
|---|---|---|---|---|
| outbox-publisher | PARTIAL | `events.OutboxWorker`; `cmd/main.go:184`, `cmd/worker/main.go:138` | Always on (stopped by `FLEET_INTEGRATIONS_KILL_SWITCH`) | No aggregate ordering; no consumer; replay tool exists |
| inbox-consumer-* | PARTIAL | HTTP: marketplace `/events`, finance `/finance/events`, documents `/documents/media-events`, support `/support/events`, tracking `/tracking/events`, completed-work `/completed-work`. Redis: Section-26 consumer | `FLEET_MARKETPLACE_EVENT_INGESTION_ENABLED`, `FLEET_FINANCE_EVENT_INGESTION_ENABLED`, `FLEET_DOCUMENTS_EVENT_INGESTION_ENABLED`, `SUPPORT_PROJECTIONS_ENABLED`, `TRACKING_PROJECTIONS_ENABLED`, `FLEET_COMPLETED_WORK_PROJECTION_ENABLED`, `FLEET_SYNCHRONIZATION_ENABLED` (all false) | The Redis consumer reads only Fleet's own stream. Nobody posts to `/support/events` or `/tracking/events`. Contract violations are dropped, not quarantined. |
| offer-expiry | MISSING | `OfferRepository.ExpireDue` (`offers.go:140`) has no caller | - | Add it to the maintenance sweeps or a worker |
| assignment-expiry | IMPLEMENTED | `StartAssignmentExpiryWorker`, `ExpireAssignments` (`assign.go:223`, SKIP LOCKED claim `assignments.go:337`) | `ASSIGNMENT_EXPIRY_WORKER_ENABLED=false` | - |
| assignment-execution-reconcile | PARTIAL | Execution-start worker (`cmd/worker/execution_start_worker.go`, **`cmd/worker` only**) + marketplace reconciliation (`FLEET_MARKETPLACE_RECONCILIATION_ENABLED=false`) | `FLEET_DISPATCH_EXECUTION_START_ENABLED=false` / Hauling / Taxi flags | Taxi missing from the worker's execution set. Dispatch read-back shape mismatch. |
| provider-sponsorship-saga | PARTIAL | `RunProviderReconciler` (`reconciliation_workers.go:73`) | Always on | Identity resolution only for Hauling. Invitation and compliance steps are not re-driven separately. |
| wallet-operation-reconcile | IMPLEMENTED | Company-wallet workers (`app/companywallet_workers.go`, **`cmd/worker` only**, `cmd/worker/main.go:181`) + finance `Run` (`FLEET_FINANCE_INTEGRATION_ENABLED=false`) + `SECTION22_POSTING_RECONCILE` | `PROVIDER_ALLOCATION_ENABLED`, `PROVIDER_WITHDRAWAL_ENABLED`, `WALLET_RECONCILIATION_ENABLED`, `SECTION22_WORKERS_ENABLED` (false) | No financial kill-switch escalation case beyond `FLEET_COMPANY_WALLET_KILL_SWITCH` |
| wallet-projection-reconcile | PARTIAL | `WALLET_PROJECTION_REFRESH_ENABLED=false` (company-wallet workers, `cmd/worker` only); Section-26 `wallet.entry.posted.v1` authority | false | `WALLET_ENTRY` read unsupported by Wallet |
| trip-projection-reconcile | PARTIAL | Marketplace reconciliation (`MarkStaleTrips`, `reconciliation_worker.go:154-158`) + Section 16 `JobTrackingReconcile` | `FLEET_MARKETPLACE_RECONCILIATION_ENABLED`, `FLEET_SECTION16_WORKERS_ENABLED` (false) | Dispatch-only read-back (`reconciliation_worker.go:256`); shape mismatch |
| route-optimization | IMPLEMENTED | Section 16 `JobOptimizationSubmit/ReadBack` (`dispatch/workers.go:23-24`) | `FLEET_SECTION16_WORKERS_ENABLED=false`, `ROUTE_OPTIMIZATION_ENABLED` | - |
| compliance-expiry | IMPLEMENTED | `section19.compliance_sweep`, `section19.restriction_reconcile` + document expiry | `SECTION19_WORKERS_ENABLED=false`, `FLEET_DOCUMENTS_EXPIRY_WORKER_ENABLED=false` | - |
| maintenance-due | IMPLEMENTED | `section18.maintenance` (`assets/operations.go:613-640`) | `SECTION18_WORKERS_ENABLED=false` | - |
| fuel-anomaly | MISSING | No anomaly evaluation (`fuel_record.created.v1` only) | - | - |
| notification-intent | PARTIAL | `SECTION23_NOTIFICATION_SUBMIT/RECONCILE` | `SECTION23_WORKERS_ENABLED=false` | Narrow intent sources (see 27.2) |
| webhook-delivery | IMPLEMENTED | `integrations` `JobTypeWebhookDelivery` + fan-out coordinator (`integrations/worker.go:88,123`) | `FLEET_PARTNER_WEBHOOKS_ENABLED`, `FLEET_PARTNER_WEBHOOK_DELIVERY_WORKER_ENABLED` (false) | Five subscribable types are never produced (26.2) |
| report-generation | MISSING | Section 24 reports not built | - | - |
| export-generation | IMPLEMENTED | `StartSection25Workers` | `EXPORTS_WORKER_ENABLED=false` | - |
| import-validation | IMPLEMENTED | `RunValidationPass` | `IMPORTS_VALIDATION_WORKER_ENABLED=false` | - |
| import-commit | IMPLEMENTED | `RunCommitPass` | `IMPORTS_COMMIT_WORKER_ENABLED=false` | - |
| payroll-calculation | PARTIAL | Synchronous `financeops` calculate (on-demand, no durable job) | `FINANCE_POSTING_ENABLED=false` | No `periodId:inputHash` job; no invalidation worker |
| payroll-allocation | PARTIAL | Synchronous per-entry `PROVIDER_ALLOCATION` posting with read-back (`financeops/payroll.go:552`) + `SECTION22_POSTING_RECONCILE` | `FINANCE_POSTING_ENABLED`, `SECTION22_WORKERS_ENABLED` (false) | Not a durable "after approval" job |
| invoice-overdue | PARTIAL | `SECTION22_INVOICE_OVERDUE` (`financeops/workers.go:26,67`) | `SECTION22_WORKERS_ENABLED=false` | Interval-based, not "daily at organization local time" |
| sla-monitor | PARTIAL | `SECTION23_SLA_GENERATE` + CRM SLA workers (`cmd/worker` only) | `SECTION23_WORKERS_ENABLED`, `SLA_EVALUATION_WORKER_ENABLED` (false) | - |
| retention-enforcer | PARTIAL | Report-only retention worker + admin-triggered `retentionexec` job | `DATA_RETENTION_WORKER_ENABLED`, `FLEET_PARTNER_RETENTION_EXECUTION_ENABLED` (false) | Not a daily enforcer |
| search-indexer | IMPLEMENTED | `RunIndexingPass` (`section25_workers.go:55`) | `SEARCH_ENABLED=false` | - |
| analytics-snapshot | MISSING | Only `performance_snapshot.generated.v1` from Section 17 maintenance | `SECTION17_WORKERS_ENABLED=false` | No dashboard or profitability snapshots |
| automation-evaluator | PARTIAL (stub) | `automation/section27_runner.go` | `AUTOMATION_RUNNER_WORKER_ENABLED=false` | Always dead-letters |
| (spec 27) reassignment coordinator | PARTIAL | `StartReassignmentStalenessWorker` | `REASSIGNMENT_WORKER_ENABLED=false` | - |
| scheduled-task runner (27 entity) | MISSING (unwired) | `scheduler.Runner` exists but is never started | - | `run-now`/`enable` routes have no effect |

### 62.1 Lease and claim algorithm

| Rule | Status | Evidence / gap |
|---|---|---|
| Separate short recovery transaction | PARTIAL | Combined with claim (`jobs.go:200-205`) |
| Claim filters `attempts < max_attempts`, priority/scheduled ordering | PARTIAL | Orders by priority, scheduled_at (`jobs.go:206`). No attempts filter in claim; dead-lettering happens in `Fail`. |
| No network I/O in claim; commit claim before processing | IMPLEMENTED | `worker.go:99-119` |
| Heartbeat/complete owner-checked | IMPLEMENTED | `jobs.go:264-290` |
| Write external operation IDs before calls; reconcile on restart | PARTIAL | Wallet, finance, execution and saga do this; organization verification does not |
| Redacted error codes | IMPLEMENTED | `jobs.go:609` `redact`, `errorclass.go` |
| Operators: pause job type, retry/dead-letter one job, replay safe event range, inspect correlation | PARTIAL | Retry and cancel jobs, outbox single-record replay, sync dead-letter replay, scheduled-task enable/disable (`backgroundops/http/manifest.go:38-67`, `internal.go:76`). **No pause-by-job-type and no event-range replay.** |

---

## 1. Cross-service work required

### driver-dispatch-delivery-service
- Make `/api/v1/internal/marketplace` match Fleet's contract (or change Fleet's `DispatchExecution` to Dispatch's dialect). Specifically:
  - Accept `source_request_id`, `expected_source_version`, `organization_id`, `acceptance_source`, `accepting_provider_id`, `organization_membership_id` and `idempotency_key` on `POST /reservations`, and return `reservation_id`, `status`, `winner_organization_id`, `source_version`.
  - Add `GET /reservations/:sourceRequestId?idempotency_key=` (or have Fleet use `/requests/:id/reservation` and `/reservations/by-idempotency/:key`).
  - Make release addressable by source request id.
  - Add `GET /eligibility?source_request_id&provider_id&vehicle_reference` returning `eligible, platform_status, reason_code, vehicle_ok, source_version`.
  - Add `POST /trips/:tripId/cancel`.
  - Return snake_case `request_id, booking_id, status, source_version, winner_organization_id, accepted_provider_id, expires_at` and `trip_id, request_id, …, executing_provider_id, completed_at, cancelled_at` from `/requests/:id` and `/trips/:id`.
- Add provider identity resolution and sponsored creation (`POST /providers/resolve`, `POST /providers/sponsored`) with spec dispositions (`no_match | single_match | split_match | ambiguous_match`).
- Emit `provider.availability.changed.v1` and `vehicle.verification.changed.v1` (or the provider-verification equivalent) to Fleet.
- Set the `organizationId` targeting needed for Fleet to create an opportunity on `platform.request.offered.v1`. This also requires the Fleet-side creator (see Fleet below).
- Optionally consume `organization.provider.membership_changed.v1` and `organization.asset.blocked.v1`, or keep relying on the synchronous restriction API.

### driver-taxi-service
- Publish `platform.request.offered/cancelled.v1` and `trip.created/status_changed/completed.v1` to `/internal/fleet-partner/v1/events`. Today it sends only completed-work.
- Add a provider identity resolve/sponsor contract, a provider/vehicle authority (`/internal/authority/{providers,vehicles}/:id`) and a restrictions API (`/internal/restrictions/apply|clear|operations/:id`).
- Fleet side: change the registry producer `taxi-service` to `driver-taxi-service`, and add Taxi to the `cmd/worker` execution-start set.

### driver-hauling-service
- Emit `platform.request.offered.v1` and `platform.request.cancelled.v1` (only `reserved` and trip events today).
- Return spec dispositions from `/providers/resolve`. Distinguish real DB errors from no-match (`fleetauthority/http.go:455-466`). Support single-identifier resolution.
- Emit truck verification and provider availability events.

### payment-wallet-service
- Support a `WALLET_ENTRY` read type (or a ledger-entry read) for Fleet's `wallet.entry.posted.v1` reconciliation. Alternatively, change Fleet to reconcile through existing types.
- Emit (or alias) `wallet.entry.posted.v1` and `settlement.created/completed.v1`, or Fleet must drop those registry entries in favour of `finance.*`/`wallet.*`.
- Ensure `wallet.provider_allocation.*`, `wallet.provider_subwallet.*`, `wallet.withdrawal.*` and `wallet.managed_settlement.*` are mapped by Fleet's finance ingestion (Fleet change), or are delivered with a type Fleet maps.
- Optional spec alignment: expose `/internal/v1/wallet-operations` + `GET /…/{operationId}` aliases.

### verification-compliance-service
- Deliver `organization.verification.status_changed.v1` (and `submitted`) to Fleet. The outbox worker currently only delivers vendor callbacks (`vendors/callbacks.go`). Add a Fleet delivery target with HMAC.
- Agree on a status vocabulary with Fleet. Either emit `APPROVED/CHANGES_REQUIRED/UNDER_REVIEW`, or Fleet maps `VERIFIED → APPROVED`, `PENDING → UNDER_REVIEW` and `SUSPENDED → restricted`. Add a changes-required state with issues.
- Provide provider and asset (vehicle) verification submission and read contracts and events (`provider.verification.changed.v1`, `vehicle.verification.changed.v1`).
- Provide an operator path (admin-backoffice) that calls `POST /internal/organizations/:org/submissions/:id/decisions`.

### support-dispute-service
- Allow `fleet-partner-service` on a case-existence/read route for incident references (today `/admin/complaints/:id` is admin-only), or point Fleet at `/admin/organization-cases/:id`.
- Add append-message (with `clientMessageId` idempotency) and evidence routes for organization cases.
- Emit `support.case.changed.v1` / `support.message.*` to `/internal/fleet-partner/v1/support/events`.

### notification-service
- Add `fleet-partner-service` to `NOTIFICATION_SERVICE_SECRETS` (no default today).
- Templates for assignment-created, assignment-expiring, provider invitation and operational alert. Fleet must also emit those intents.
- Realtime audience registration for dispatcher boards. Delivery-result events (`notification.delivered/failed`) to Fleet if push-based projection is wanted.

### media-file-service
- No blocking work. Optionally emit discrete `media.uploaded/scanned/rejected/deleted/retention_changed` if the 60 table's names are to be honoured literally.

### analytics-service
- Add a curated-event ingest for Fleet (e.g. `/internal/analytics/v1/fleet-events`) or a generic event ingest. Fleet must add a producer (outbox target).

### admin-backoffice-service
- Organization review package and decision flow (approve, reject, changes-required) that drives Verification `/decisions`.
- Emergency restriction/lift commands against Fleet with actor/approval reference and audit export.

### fleet-partner-service (same-service work these sections imply)
- **Create request projections and offers** from `platform.request.offered.v1`: eligible-organization resolution plus `Projections.Upsert` and `Offers.Create/CreateRecipient` in `applyProjectionChange`.
- Organization verification: a read-back worker for `UNKNOWN_OUTCOME`/`PENDING_DISPATCH`, a consumer for `organization.verification.status_changed.v1`, a status-vocabulary mapping, and a caller of `organizations.TryActivate` (event handler + periodic sweep).
- Emit the 26.2 names (`organization.status_changed`, `provider.provisioned`, `provider.membership_changed`, `organization.asset.blocked`, `corporate.order.confirmed`, `corporate.shipment.status_changed`, `corporate.invoice.issued`), or rename the registry and webhook allowlist to the names actually emitted.
- Deliver outbox events to consumers that exist. Either per-target HTTP delivery (as Dispatch/Hauling/Wallet do), or get counterparts to read `fleet-partner:events`.
- Add `causationId`, `actor`, `requestId`, `traceparent` and `metadata` to the envelope. Order outbox publication by aggregate/version.
- Start `scheduler.Runner`. Wire `Offers.ExpireDue`. Start the execution-start, company-wallet and CRM workers in the API binary as well (or document that `cmd/worker` is mandatory).
- Fix `/main.go:204` log text. Add pause-by-job-type and range replay.

## 2. Frontend work required

### web/fleet (Next.js)
- **System pages are mock-backed.** `src/app/(dashboard)/system/jobs/page.tsx:46-322` ("MOCK_JOBS", "Retrying job … (mock)") and `system/sync/page.tsx:38-162` ("MOCK_SYNC_STATUS"). The same pattern applies to `system/dead-letters`, `system/scheduled-tasks` and `system/reconciliation`. Wire them to:
  - `GET/POST /fleet-partner/v1/jobs[/:id[/attempts|/retry|/cancel]]`
  - `/scheduled-tasks[/:id[/runs|/enable|/disable|/run-now]]`
  - `/synchronization/projections`, `/synchronization/reconciliation-cases[/:id[/reconcile]]`, `/synchronization/dead-letters[/:id[/replay]]`

  Note that scheduled tasks will not run until the backend runner is started.
- **Onboarding/verification page** (`/organizations/{id}/verification`) must show the real verification states once they converge (`UNDER_REVIEW`, `CHANGES_REQUIRED` + issues, `APPROVED`/`REJECTED`, `UNKNOWN_OUTCOME` "reconciling"). It needs an activation-readiness view of the gates from `EvaluateActivation`, which is not exposed as an API today; a backend route is needed.
- **Marketplace pages** (`operations/marketplace/*`, `operations/assignments`) will stay empty until projections and offers are created from source events. Add projection freshness, gap and reconciling badges, and a "source unavailable" state for Taxi/Hauling when their integration flags are off.
- **Webhook subscription UI** (`settings/integrations/webhooks/*`) should offer only event types that are actually emitted, or the backend allowlist must be fixed first.
- **Finance/wallet projection screens** should show `lastReconciledAt` and staleness, and the wallet-operation `UNKNOWN_OUTCOME`/"reconciliation pending" state.

### Provider apps (`apps/*`)
- `apps/dispatch_provider`: already models `organization-offers` and `company-assignments` (`lib/features/requests/data/request_model.dart:14-18,253`). It needs Accept/Decline with reason codes and a response-deadline countdown. The backend side is gated by `DISPATCH_PROVIDER_ORGANIZATION_OFFERS_ENABLED`, `DISPATCH_PROVIDER_COMPANY_ASSIGNMENTS_ENABLED` and Fleet `FLEET_MARKETPLACE_ALLOWED_CALLERS`. Verify it handles 409 "assignment expired or reassigned" and the decline safety-review flow.
- `apps/truck_provider`: **no company-assignment or organization-offer UI.** Hauling exposes `/provider/company-assignments` (shared `fleetproviderwork`) and `/provider/private-assignments/:id/{accept,decline}` (`driver-hauling-service/internal/features/providerassignments/http.go:118-125`). Add list and detail views, Accept/Decline with reason, deadline, and the managed-provider "organization mode" banner.
- `apps/taxi_provider`: **no company-assignment or organization-offer UI.** Taxi exposes `/provider/organization-offers[/:id/accept|decline]` and `/provider/company-assignments[/:id/accept|decline]` (`driver-taxi-service/internal/features/fleetprovider/http.go:67-75`), gated by `TAXI_FLEET_PROVIDER_WORK_ENABLED`. Add the same Accept/Decline UX.
- All three apps need push/in-app notification handling for "new company assignment", "assignment expiring" and "assignment reassigned/cancelled". Backend notification intents for these do not exist yet (Section 60 notification row), so apps currently have to poll.
- Managed providers should see a sub-wallet/allocation view only if `RegisterProviderRoutes` is mounted (prior audit: unmounted). This is not caused by these sections, but it is required for the wallet allocation events in 61.8 to be visible to providers.
