# Customer V4 Backend Certification

Final programme report for Karryam Customer Platform Backend V4 (§27, §28 B00–B24, §§29–32). The authority is the saved V4 specification (`karryam_backend_v4.txt`). Customer V4 numbering is separate from Vendor's historical section numbering.

Detailed §§9–26 evidence stays in `docs/customer-sections9-11-report.md`, §14 in `services/vendor-service/docs/IMPLEMENTATION_STATUS.md`, and §§15–18 in the same Vendor status file. This report references them rather than repeating them. It records the programme gates and the work done in this final pass.

**Verdict: BACKEND REPOSITORY IMPLEMENTATION COMPLETE / STAGING / RELEASE CERTIFICATION PENDING.** "BACKEND PROGRAMME DONE" is **not** supportable under V4 §32.2, which requires B00–B24 closed, including a real B23 staging certification.

## Checkpoint

- **Last completed gate:** local closure of B21 and B22, and the strengthened B24 bundle (2026-09-29, second pass; see "Local operational closure").
- **In progress:** none.
- **Next exact action (external):** provision staging access, then run B23 (see "B23 blockers").

## Certification levels (kept separate)

| Level | Status | Meaning |
|---|---|---|
| LOCAL | **PASS** | Built, vetted and tested on this worktree against disposable PostgreSQL 18 (127.0.0.1:55432) and Redis (127.0.0.1:56379). |
| STAGING | **BLOCKED** | No staging credentials, hosts, deployed builds or monitoring access are available. Nothing here is claimed as staging evidence. |
| RELEASE | **PENDING** | A contract bundle was prepared locally but is not frozen on an immutable build: HEAD plus a dirty, uncommitted worktree, with no commit authorized. |

## Programme matrix

| Item | Status | Basis |
|---|---|---|
| **§27 Observability** | **PASS (local)** | Existing registries reused; gaps closed; structured access log. See §27 below. |
| B00 Re-audit and contract freeze | PASS | Frozen baseline manifest below; route/OpenAPI parity 0 drift. |
| B01 Shared contract and authority vocabulary | PASS | `shared/go/events` envelope and `MovementSummary`; `shared/go/journeyshare` projection; the Vendor logistics vocabulary consistency test. `shared/go` suite green. |
| B02 Customer identity/roles | PASS | Trusted contacts, mobility preferences, guest travellers and saved locations. **This pass adds** the `guest_traveller` context scope and freezing it into the Taxi booking (E2E-02). |
| B03 Movement publisher audit | PASS | §22: all five products publish; per-authority reconcilers. |
| B04 Activity projection | PASS | §22: dedupe, version guard, reconciliation and purchase-row grouping on PostgreSQL. |
| B05 Taxi safe live location | PASS (local) | N07/N08 on the running stack; 5-minute load run with 0 unexpected results; stale/unavailable states. |
| B06 Share Journey core | PASS (local) | N01–N06 lifecycle, token hashing, expiry worker. |
| B07 Mobility share adapters | PASS (local) | N08–N10 running-stack E2E. |
| B08 Commerce share adapter | PASS (local) | N11 for all four executors, with no financial leakage. |
| B09 Secure customer media upload | PASS (local) | N12 single-use grant, with no service credential in the Customer app. |
| B10 Notification/deep-link | PASS (local) | The route-intent catalogue covers every V4 §16.1 family. **This pass adds** the missing support-case deep link. |
| B11 Support/evidence context | PASS (local) | §17 order-scoped cases, MediaRef verdict binding, unknown-outcome read-back. |
| B12 Food public contract | PASS (local) | Discovery/catalogue/preview parity; the packed-weight policy fails closed. |
| B13 Shop public contract | PASS (local) | Discovery/product/inventory parity; variable measurement plans from a safe estimate. |
| B14 Cart/checkout/order | PASS (local) | §14 lifecycle E2Es, and a non-committing delivery preview that is invalidated by material changes. |
| B15 Substitution/variable measurement | PASS (local) | Shop substitution and measurement lifecycle E2Es. |
| B16 Commerce logistics/handover | PASS (local) | Capability ladder, escalation-only, no provider selection, distinct credential authorities. |
| B17 Group/RFQ/recurring | PASS (local) | **This pass adds** a flag-on PostgreSQL E2E and **fixes a defect** it found. |
| B18 CRM/loyalty/consent | PASS (local) | Only customer-safe contracts are mounted publicly (route audit shows no staff/internal exposure). |
| B19 Wallet/refund | PASS (local) | §15 unknown-outcome, refund and multi-vendor tests. Wallet is a test double in the Vendor E2Es; there is no Wallet black-box on staging. |
| B20 Ingress/flags/config | PASS (local) | 0 HMAC/admin routes publicly reachable; flags default off and fail closed. |
| B21 Security/observability/recovery | **PASS LOCAL** | Repeatable backup/restore tooling with a 7-service drill; 29 alert rules and a dashboard, validated; §25/§26/§27 evidence still green. Staging restore and deployed alerts are B23 work. |
| B22 Cross-service contract/E2E suite | **PASS LOCAL — HOSTED CI EXECUTION PENDING** | CI workflow definition complete and validated locally; every step it runs passes locally with 0 skips. Hosted execution needs a push. |
| B23 Staging certification | **BLOCKED — EXTERNAL STAGING ACCESS/CONFIG REQUIRED** | See "B23 blockers". |
| B24 Release contract freeze | **PREPARED LOCALLY — FINAL FREEZE PENDING IMMUTABLE BUILD + B23** | Machine-readable manifest `docs/customer-v4-release-manifest.json`, with `LOCAL_RELEASE_CANDIDATE_DIGEST`. |

## B00 — frozen baseline manifest

| Item | Value |
|---|---|
| Branch / HEAD | `staging` @ `5b2050a896942ddb4d191a3a90cb4af49342bd02` (2026-09-22, "test(vendor): roll back to 000120 rather than assuming it is last") |
| Worktree | Dirty: 215 modified, 167 untracked (all Customer V4 work, uncommitted) |
| Build identifier | Not immutable: HEAD plus the dirty tree |
| Migration tips (last file / count) | customer `013_movement_commerce_order_authority` / 13; taxi `021_customer_ride_notifications` / 21; dispatch `000067_outbox_guard_restore_safe` / 67 up (73 files; `000067` added in the second pass); hauling `026_receiver_secure_channel` / 28; vendor `000129_receiver_secure_channel` / 129; media `000005` / 5; wallet `000018` / 18; notification `008` / 8; support `008` / 8; verification `000003` / 3 |
| Mounted routes (gin capture) | customer 53, taxi 153, dispatch 196 (rollout gates on), hauling 130, vendor 867 inventory + 5 root (872), wallet 99, notification 11, support 89, media 22, verification 21, api-gateway 47 |
| Route parity (`drift.py`) | 0 undocumented, 0 phantom, 0 security mismatch |
| Public exposure (`classify.py`) | 0 service-credential/admin routes reachable through nginx or the gateway |
| N01–N12 | All mounted at the exact V4 paths and documented (§23) |
| External Reference/Repricing | Not integrated. `EXTERNAL_REFERENCE_REPRICING_ENABLED` defaults off and refuses boot without an approved adapter (`TestCustomerCommerceGates`). No pricing logic was invented. |

Route-capture note: the Vendor capture is the all-flags `crmcontract` inventory (867) merged with the `cmd`-level root routes from the real `main.go`: health, ready, meta, logistics readiness and the new metrics route. A default-flag `main.go` capture mounts only 115 routes and reports flag-gated routes as phantom, so it is not the parity baseline.

## §27 — observability result

No second logging, metrics or monitoring framework was created. The existing `shared/go/observability.Metrics` registry, `journeyshare.Observe`, `outboxmetrics` and `logging.Structured`/`SafeFields` are reused.

| Area | Before | Added |
|---|---|---|
| Structured request logs | Coloured text | `httpx.Logger` emits one `http_request` JSON record: request id, **route template only** (never the raw path or query), method, status, latency, the bounded V4 `outcome` and a validated W3C `trace_id`. |
| Share Journey | Complete | None |
| Taxi live location | Fresh/stale/unavailable, read latency, authorization failures | `taxi_location_ingest_lag` |
| Movement | None | Customer `karry_movement_ingest_total{authority,outcome}` and `karry_movement_ingest_lag`; projection age/rows and 24h rejection gauges; `karry_movement_reconciliation_runs_total` / `_republished_total` from the shared reconciler (Dispatch, Hauling) and Taxi's reconciler |
| Commerce (Vendor) | No scrape | Request outcome/latency by bounded family; stock conflicts, pending substitutions, unresolved missions and payment legs, open refunds and ages, outbox backlog |
| Media | No scrape | Upload sessions, upload/validation failures, orphans |
| Notification | No scrape | Queue oldest age, deliveries, dead letters, websocket sessions |
| Hauling | Registry recorded but never exported | Exported |

Private scrapes are all HMAC-verified and limited to `admin-backoffice-service` and `analytics-service`:
- customer `GET /internal/customer/v1/metrics`
- vendor `GET /internal/vendor/v1/metrics`
- media `/api/v1/media-files/internal/metrics`
- notification `/api/v1/notifications/internal/metrics`
- hauling `/api/v1/hauling/internal/metrics` (its caller allowlist was added this pass)

All are classified internal, with 0 public exposure. Label cardinality is enforced by `LabelKeyIsLowCardinality`. A failing gauge family increments `karry_metrics_collection_errors_total` without blanking the others.

Tests: `TestCollectGaugesReplacesBoundedFamilies`, `TestOutcomeForStatusIsTheV4Vocabulary`, `TestLoggerEmitsStructuredAccessRecord`, `TestTraceIDRejectsMalformedTraceparent`, the Customer movement metrics test, and the per-service gauge SQL tests against migrated schemas (Customer, Vendor, Media, Notification).

**Not done: dashboards and alert rules.** None exist in the repository, and there is no deployed Prometheus/Grafana to load them into. Recommended first alerts, all on metrics that now exist:
- `karry_movement_projection_age_seconds` over 15 min;
- `karry_commerce_payment_legs_unresolved` or `karry_commerce_mission_unresolved` above 0 for 15 min;
- `karry_commerce_refund_oldest_open_age_seconds` over 1 h;
- `karry_notification_dead_letters` increasing;
- `karry_vendor_outbox_oldest_pending_age_seconds` over 5 min;
- any increase of `karry_metrics_collection_errors_total`.

## Defects found and fixed in this pass

1. **Vendor scheduled orders could never be created without a capacity reservation (real, pre-existing).** `Section21Service.AcceptScheduled` scanned the returned, nullable `capacity_reservation_id` into a plain string, so every scheduled/bulk order with `capacity_status=NOT_REQUIRED` failed after its INSERT and rolled back. Nothing tested the flag-on path. It is now scanned through a pointer, as the replay path already did (`internal/features/orders/commerce_orders.go`). Regression: `TestCustomerV4E2E07ScheduledAndRFQBehindCapabilityFlag`.
2. **Ride for a guest traveller was not implemented (E2E-02 gap).** Taxi had no traveller concept, so a booking "for someone else" could only be booked as the customer.
   - Customer's existing HMAC context endpoint gained an ownership-scoped `guest_traveller` scope (`guest_traveller_id`). It returns name, phone, structured needs and version. Free-text notes and the relationship label are withheld. It is granted to Taxi by default in `CUSTOMER_CONTEXT_CALLER_SCOPES`.
   - Taxi accepts `guest_traveller_id` on create and freezes a `traveller` block into the existing trigger-immutable `requirement_snapshot_json`, so no migration was needed. The traveller's own needs gate matching (source `GUEST_TRAVELLER`), never the customer's profile. An explicit request still wins.
   - An unknown, foreign or unreadable traveller refuses the booking. It is never booked as the customer.
   - Tests: `TestGuestTravellerScopeIsOwnershipScopedAndMinimized`, `TestGuestTravellerScopeWithoutReaderIsUnavailable`, `TestGuestBookingFreezesTravellerAndUsesTheirNeeds`, `TestGuestBookingNeverFallsBackToBookingTheCustomer`, `TestGuestTravellerClientSignsScopesAndParses`, `TestGuestTravellerClientFailsClosed`. The Taxi register OpenAPI was regenerated.
3. **Support-case updates carried no deep link (V4 §16.1 gap).** `support-dispute-service` `support.*` events now derive `SUPPORT_CASE_DETAIL` with the opaque `complaint_id` (`shared/go/notifications/route_intent.go`). The customer-only rule and the URL rejection are unchanged, and the catalogue test was extended.
4. **The Hauling metrics scrape accepted any trusted caller.** It is now restricted to admin-backoffice and analytics, like the other scrapes.
5. Harness only, not product: the route-audit overlay had a corrupted Vendor key (a `\v` escape) and read UTF-8 config files as cp1252. Both are fixed.

**Migrations added in this pass: none.** The only migration from §§22–27 remains customer `013_movement_commerce_order_authority.sql`.

## Verification run in this pass

| Suite | Result |
|---|---|
| `shared/go` (all packages, Redis-backed journeyshare) | PASS, 0 failures |
| Build + `go vet ./cmd/...` for customer, taxi, dispatch, hauling, vendor, media, notification, support, wallet and api-gateway | PASS |
| Taxi `internal/features/booking/...` with PostgreSQL and Redis (tip-level DB built by Taxi's own migration runner) | 272 PASS, 0 SKIP, 0 FAIL |
| Customer context, mobility, `cmd`, config (PostgreSQL) | PASS |
| Vendor `cmd`: `TestCommerceLifecycleE2E_*` (6), `TestCustomerV4E2E07…`, `TestCartDeliveryPreview…`, `TestCustomerOrderMovement…` (PostgreSQL, tip 000129) | PASS |
| Vendor `internal/features/orders` | PASS |
| Notification `messages/...`, `shared/go/notifications` | PASS |
| Route capture / classify / drift / register rebuild | 0 violations, 0 drift, 0 security mismatch |
| `git diff --check` | Clean |

The expensive §§9–26 suites were not rerun. Shared code touched here (`httpx` logger, `notifications` route intent, `events` reconciler metrics, `observability`) has no test coupling to them, and every service still builds and vets. `-race` was not run: the host has no cgo toolchain. The earlier race evidence is the Linux container run in `sections911-race-results/`.

## §30 — E2E acceptance matrix

| ID | Local | Staging | Evidence |
|---|---|---|---|
| E2E-01 Ride self | PASS_LOCAL | BLOCKED_EXTERNAL | Running-stack Ride self (N07 FRESH, foreign 404, terminal); `TestProviderIngestionToN07AndN08`; Taxi pickup verification suite; movement republish |
| E2E-02 Ride guest | PASS_LOCAL (component + contract) | BLOCKED_EXTERNAL | New guest tests above; the snapshot is trigger-immutable; the share projection carries no traveller data |
| E2E-03 Ride Share | PASS_LOCAL | BLOCKED_EXTERNAL | Running-stack N01→N06→rotate→revoke→terminal |
| E2E-04 Send | PASS_LOCAL | BLOCKED_EXTERNAL | Running-stack Send share; Dispatch secure handover (`TestSecureDeliveryReceiverContactChangeInvalidatesIssuedCredential`, `TestSecureHandoverIdentityChangeInvalidatesActiveChallenge`); account-scoped receiver reads; receivers with no verified channel fail closed (`RECEIVER_SECURE_CHANNEL_REQUIRED`) |
| E2E-05 Truck | PASS_LOCAL | BLOCKED_EXTERNAL | Running-stack Truck share; Hauling lifecycle and handover suites |
| E2E-06 Food restaurant | PASS_LOCAL | BLOCKED_EXTERNAL | `TestCommerceLifecycleE2E_Food` |
| E2E-07 Food custom/scheduled | PASS_LOCAL | BLOCKED_EXTERNAL | `TestCustomerV4E2E07ScheduledAndRFQBehindCapabilityFlag` (new), `TestSection21DisabledFailsClosed` |
| E2E-08 Shop multi-vendor | PASS_LOCAL | BLOCKED_EXTERNAL | `TestCommerceLifecycleE2E_ShopMultiVendorPartialCompletion`, `_ShopPerExecutor` |
| E2E-09 Variable measurement | PASS_LOCAL | BLOCKED_EXTERNAL | `TestCommerceLifecycleE2E_ShopVariableMeasurementReplan` |
| E2E-10 Substitution | PASS_LOCAL | BLOCKED_EXTERNAL | `TestCommerceLifecycleE2E_ShopSubstitutionThroughCompletion` (equal/cheaper/costlier/reject/EXACT_ONLY) |
| E2E-11 Payment unknown | PASS_LOCAL | BLOCKED_EXTERNAL | `TestSection17FWallet_TimeoutIsUnknownAndResolvableOnTheNextSweep`, `TestSection17FPayment_UnknownAuthorityAnswerIsNotSuccess`, `TestStatusFor_UnknownAuthorityAnswerStaysPending` |
| E2E-12 Partial refund/support | PASS_LOCAL | BLOCKED_EXTERNAL | Multi-vendor partial completion/refund E2E; `TestCommerceCaseCreateRaceAndResolutionOrderingPostgres`, `TestCreateCommerceCaseReadsBackUnknownOutcome` |
| E2E-13 Commerce Share | PASS_LOCAL | BLOCKED_EXTERNAL | Running-stack N11 via Dispatch/Hauling/Taxi Goods/Vendor Own; no items, payment or refund fields |
| E2E-14 Notification deep link | PASS_LOCAL | BLOCKED_EXTERNAL | `TestRouteIntentCatalogueUsesOpaqueOwnerReadBack` (now including support), `TestSendPersistsOnlyDerivedRouteIntent`; `read_back:true` with owner reads returning current state |
| E2E-15 Projection outage/recovery | PASS_LOCAL | BLOCKED_EXTERNAL | `TestStackProjectionOutage`, `TestPostgresReconciliationRequeuesMissingVersions`, `TestPostgresCommerceOrderReplayOutOfOrderAndReconciliation` |
| E2E-16 Feature rollback | PASS_LOCAL (flag-off behaviour) | BLOCKED_EXTERNAL (the scenario is defined "during staging") | `TestDomainGateRefusesDisabledDomain` (existing carts stay readable; order/payment routes are not behind the customer domain flag); `TestDisabledFlagsReturnUnavailable`; Share disabled means creation, public read and revoke all return typed 503 with no leak |
| E2E-17 Quantity escalation | PASS_LOCAL | BLOCKED_EXTERNAL | `TestQuantityEscalatesTheSameProduct`, `TestEngine_*Escalates*`, `TestPhysicalReplan_PackageCountEscalates` |
| E2E-18 Mandatory logistics metadata | PASS_LOCAL | BLOCKED_EXTERNAL | `TestDefaultPolicyRequiresPackedWeight`, `TestMissingPackedWeightNeverPlansOnTheSmallestVehicle`, `TestAggregate_MissingWeightIsPartialNotZero` |
| E2E-19 Address/recipient repricing | PASS_LOCAL | BLOCKED_EXTERNAL | `TestCartDeliveryPreviewRecomputesMaterialChangesAndIsNonCommitting`, `TestSection17DQuote_MaterialCustomerChangeInvalidatesApproval`, `TestSection17FChange_AddressChangeCreatesAReplacementQuoteAndLeavesQ1Immutable` |
| E2E-20 No unsafe downgrade | PASS_LOCAL | BLOCKED_EXTERNAL | `TestEngine_HasNoProviderSupplyInput`, `TestPlan_AuthorityRefusalMakesTheGroupUnserviceable`, `TestRequirementEvidence_NeverDescribesASmallerVehicleThanThePlan`, `TestProtectedHandoverCannotDowngrade`, Hauling `TestVehicleClassIsAHardGate`, `TestIncompatibleReplacementIsRefusedAndEscalates` |
| E2E-21 Vendor authority boundary | PASS_LOCAL | BLOCKED_EXTERNAL | `TestNoVendorMissionRequestCarriesProviderSelectionAuthority`, `TestTheProviderSelectionBanIsNotVacuous`, `TestJITBindingNamesNoProvider`, `TestCateringAuthoritySelectionNeverChoosesProvider` |

**E2E: 21/21 PASS LOCAL; 0/21 PASS STAGING.**

Caveats that are honest rather than hidden:
- The Vendor commerce E2Es run real Vendor services and PostgreSQL, but Wallet, logistics quote/mission and handover authorities are explicit test doubles.
- The mobility and share flows (01, 03, 04, 05, 13, 15) ran as real processes on a local stack.
- E2E-02 is component and contract level, not a running stack.

## §29.4 — failure injection

| Failure | Result | Evidence |
|---|---|---|
| Wallet timeout after start | PASS | Marked unknown, then read back by deterministic id; no second charge (`TestSection17FWallet_TimeoutIsUnknownAndResolvableOnTheNextSweep`, `TestSection17FCheaper_WalletTimeoutStaysUnknownThenRecovers`) |
| Logistics mission timeout | PASS | `TestDispatchAdapterRecoversUnknownCreateByStableOperation`, `TestHaulingMissionUnknownCreateReadsBackStableOperationWithoutDuplicate` |
| Notification outage | PASS | `TestCommittedShareSurvivesNotificationOutage`, `TestFCMFailureDoesNotRollbackInbox`; retry then exactly one dead letter (§26) |
| Share authority unavailable | PASS | Running-stack outage: typed 503, no leak, same capability recovers; `TestUnavailableAndMismatchedAuthority` |
| Taxi location stale | PASS | `TestSafeLocationStates` (STALE or UNAVAILABLE, never another provider) |
| Media upload interruption | PASS | `TestUploadAuthorizationIsSingleUse`, `TestExpiredUploadAuthorizationIsRefused`, `TestCustomerGrantRedeemsWithoutServiceCredentialAndCannotReplay`; no service-secret fallback |
| External Reference/Repricing unavailable | PASS | The flag refuses boot without an adapter; current approved pricing is untouched (`TestCustomerCommerceGates`) |
| Movement ingress outage | PASS | The authority outbox retains the event; `TestPostgresReconciliationRequeuesMissingVersions`, `TestStackProjectionOutage` |
| Guest traveller context outage (new) | PASS | Booking refused 503; never booked as the customer (`TestGuestBookingNeverFallsBackToBookingTheCustomer`) |

## §29.5 — security

| Check | Result |
|---|---|
| IDOR across customer IDs, source refs and share IDs | PASS: foreign-owner 404s on N02–N11, Vendor order/change/substitution, Media, Support. New: another customer's guest traveller returns 404, and another customer cannot reuse a scheduled key, open an RFQ or accept one. |
| Share brute force, rate limit, token leakage | PASS: 60 reads/min/token typed 429 under load; digest-only storage; ingress `access_log off`; the app logs the route template only (`TestLoggerEmitsStructuredAccessRecord` asserts no path secret) |
| Wrong HMAC caller, nonce replay, timestamp skew | PASS (`shared/go/serviceauth`, journeyshare with Redis; wrong-caller 403 on every new internal route) |
| OTP/QR never in response/event/log/share | PASS: challenge references only; the notification credential guard |
| Secret scan | PASS with one finding: `secret_scan.py` over 537 distributable files. `apps/truck_provider` compiles the default media token `development-media-token`. This is a mobile app, out of scope, and was reported, not changed. |
| Strict decoding / SQL boundaries | PASS where the service convention is strict (e.g. `TestHandler_TransitionBodyRejectsUnknownFields`); parameterized SQL throughout |

## Local operational closure (second pass, 2026-09-29)

This pass closes the repository-local B21 and B22 gaps and strengthens B24. Everything below is **local** evidence. Nothing here is staging or release certification.

### Tooling added (tracked, portable: Python + Go, Windows and Linux)

| Path | What it is |
|---|---|
| `scripts/customer-v4/pgbackup.py` | Generic PostgreSQL backup/restore/verify. It discovers schema, migration ledger and rows from the catalogue; no table lists. Credentials come only from env vars and are passed as `PGPASSWORD`; nothing is logged. |
| `scripts/customer-v4/cert.py` | One certification entrypoint: `migrate`, `backup-drill`, `gates`, `routes`, `secrets`, `monitoring`, `manifest`, `all` |
| `scripts/customer-v4/routeaudit.py` + `openapigen/` | The §§19–27 route harness moved out of the ignored `tmp/` into tracked form. Overlays are generated at run time (a gin copy patched in a temp workspace; no vendored gin; no hard-coded paths). The gateway table is read from its source. |
| `scripts/customer-v4/secretscan.py` + `secret-scan.reviewed.txt` | Credential-shape gate. Only mobile Firebase/Maps client IDs under `apps/` may be reviewed. |
| `scripts/customer-v4/public-unauthenticated-routes.reviewed.txt` | 31 reviewed open routes plus Vendor's inventory entries; any new unauthenticated public route fails CI |
| `scripts/customer-v4/monitoring_check.py` | Strict PromQL-subset parser plus a metric/label catalogue derived from the Go sources |
| `deploy/monitoring/customer-v4/` | `alerts.rules.yml` (29 alerts), `dashboard.json` (23 panels), `README.md` |
| `.github/workflows/customer-v4-certification.yml` | The CI gate (below) |
| `scripts/customer-v4/test_tooling.py` | 14 self-tests: parser rejections, backup refusals, credential scrubbing, deparse normalization, migration ordering, and workflow validation (structure, no secrets/deploy/push, pinned actions, `bash -n` on every run block) |

### Backup/restore result: PASS LOCAL

`pgbackup.py backup`: `pg_dump -Fc`, a SHA-256 checksum, and metadata recording the build, schema fingerprint, migration ledger, per-table counts and content digests. It fails non-zero if the schema changes mid-dump.

`restore` refuses (exit 2), in each of these cases:
- a checksum mismatch;
- a non-empty target without `--replace --confirm-target <name>`;
- restoring onto the backup's own source;
- any production-looking target without `--allow-production` plus an approved change reference.

It then restores in a single transaction and verifies (exit 4 on mismatch).

**Repeatable drill** (`cert.py backup-drill`), at the current tips built by each service's own runner, with Dispatch and Hauling dev seeds as fixture rows:

| Service (tip) | Tables | Rows | Tables row-digested | Occupied target refused | Source overwrite refused | Tamper refused | Drift detected | Result |
|---|---|---|---|---|---|---|---|---|
| customer (013) | 15 | 0 | 15 | yes | yes | yes | yes | PASS |
| taxi (021) | 52 | 22 | 52 | yes | yes | yes | yes | PASS |
| dispatch (000067) | 112 | 72 | 112 | yes | yes | yes | yes | PASS |
| hauling (026) | 43 | 106 | 43 | yes | yes | yes | yes | PASS |
| vendor (000129) | 407 | 1,036 | 407 | yes | yes | yes | yes | PASS |
| payment-wallet (000018) | 56 | 107 | 56 | yes | yes | yes | yes | PASS |
| media (000005) | 11 | 5 | 11 | yes | yes | yes | yes | PASS |

Staging backup certification is **not** claimed (B23).

### Monitoring/alert definitions result: PASS LOCAL

- **Convention:** the platform exposes Prometheus text; no rule or dashboard files existed before this pass.
- **Alerts (29):**
  - Share: 5
  - Taxi location: 3
  - Movement: 6
  - Commerce: 7
  - Media: 3
  - Notification: 4
  - Platform: 1
- **Dashboard:** 23 panels, 42 targets.
- **Validation:** `monitoring_check.py` confirms that
  - every expression parses under the strict subset;
  - every metric is emitted by the code (70 series), and every label is one that series carries;
  - no label is unbounded;
  - every rule is marked `threshold_status: initial_default` and "pending SRE tuning".

  No production SLO is claimed.
- **promtool:** not installed locally, and not downloaded. CI runs `promtool check rules` (`prom/prometheus:v2.53.2`).
- **One gauge was added** so the share expiry sweep is observable: `karry_journeyshare_overdue_sessions`.

### CI workflow result: CI WORKFLOW DEFINITION COMPLETE LOCALLY; HOSTED CI EXECUTION PENDING SOURCE-CONTROL / CI RUN

The workflow:
- runs on Linux, with PostgreSQL 16 and Redis 7 service containers, `contents: read` permissions, no secrets and no deploy step;
- builds and vets 12 modules;
- runs, via `cert.py`: monitoring with tooling tests, `promtool`, the secret scan, migrations plus the backup drill, the gates, the route audit, and the manifest;
- uploads evidence even when a step fails.

A gate fails on any failing test, **any skipped test**, any package failure, or a `-run` filter that matched nothing.

**Every gate passed locally with 0 skips:**

| Gate | Tests passed |
|---|---|
| shared-go (38 packages) | 379 |
| customer | 83 |
| taxi | 279 |
| dispatch (including `./migrations`) | 159 |
| hauling | 111 |
| vendor-e2e | 59 |
| vendor-packages | 276 |
| vendor-fulfilmentgroup | 112 |
| vendor-orders | 99 |
| wallet (including bounded load) | 163 |
| media | 88 |
| notification | 19 |
| support | 46 |
| **Total** | **1,873** |

Also passing locally:
- Routes: 11 services, 0 public HMAC/admin, 0 unreviewed open routes, drift 0, security mismatch 0, register current.
- Secrets: 0 unreviewed.
- Monitoring: pass.

**Explicit exclusion (not a pass):** `vendor-fulfilmentgroup` `^TestStack`, the running-stack E2E that needs all five services running. It is certified by the §§9–11 running-stack evidence.

### Defects found by the new gates and drill, and fixed

1. **Dispatch databases could not be restored from a standard backup (real, pre-existing).** `dispatch_outbox_payload_is_safe(jsonb)` backs a CHECK constraint on `dispatch_marketplace_outbox` and calls itself recursively without a schema. `pg_restore` runs with an empty `search_path`, so any backup holding outbox rows aborted mid-restore.
   - Fixed by the additive migration `000067_outbox_guard_restore_safe` (`ALTER FUNCTION … SET search_path FROM CURRENT`, with a paired down file). Function body and constraint are unchanged.
   - Regression test `TestOutboxGuardEvaluatesUnderEmptySearchPath`. Verified to fail without the migration, with the exact restore error.
   - A sweep of all 7 V4 schemas found no other constraint- or index-referenced function without a pinned `search_path`.
2. **Wallet `TestRebindMigrationRollsBackCleanly` was broken by this programme's `000018`.** It rolled back "the last" migration assuming that was 000017. It now unwinds later migrations first.
3. **Media `TestMediaGaugeQueriesRunAgainstMigratedSchema` (added in §27) depended on package order.** It now migrates its own database, like the Media integration suite.
4. **Dispatch `TestHandoverNotificationReconciliationClaimRace` was clock-skew sensitive.** It stamped "due" with the database clock but claimed with the host clock. The fixture is now due a minute earlier. This is test-only; production claims a skewed row on the next tick.
5. Harness findings, not product:
   - Taxi's reconcile test and Hauling's share test only passed through shared-database ordering. Every gate package now gets its own fresh database.
   - Dispatch's `test-migrate` omits the three legacy rider-auth files that `dispatch-local-bootstrap.sh` applies first. The drill template follows the bootstrap order.
   - The services mix 3- and 6-digit migration prefixes, so tips are ordered numerically.
   - A Vendor catalogue query exceeded the Windows command line; SQL now goes through stdin.
   - PostgreSQL re-deparses `varchar IN (…)` CHECK constraints after a round trip. Only casts to a closed list of types are normalized, and real changes still fail (unit-tested).

**Migrations added in this pass:** `services/driver-dispatch-delivery-service/migrations/000067_outbox_guard_restore_safe.{up,down}.sql` only.

### B24 manifest and LOCAL_RELEASE_CANDIDATE_DIGEST

`docs/customer-v4-release-manifest.json` (regenerate with `cert.py manifest`) records:
- HEAD and dirty-worktree status;
- `LOCAL_RELEASE_CANDIDATE_DIGEST`;
- migration tips;
- the route inventory;
- 100 OpenAPI fingerprints;
- N01–N12, each verified mounted with the expected authentication;
- 16 flags, all verified to default `false`;
- the event catalogue and deep-link intents;
- service allowlists;
- the metrics catalogue (70 series);
- workers and reconciliation;
- the backup tool version and hash;
- the monitoring definition hashes;
- rollback notes;
- external dependencies;
- the B00–B24 status.

`LOCAL_RELEASE_CANDIDATE_DIGEST` = `533eb2167b8551958ce44c649078c3734954dd652513ed5f86c7296be10ee40e`. It is computed over 4,625 tracked and untracked, non-ignored files under `services/`, `shared/go/`, `deploy/`, `scripts/customer-v4/`, the workflow, `go.work` and `go.work.sum`, with CRLF normalized. It was recomputed twice and matched both times. **It is a local release candidate digest: not a production build ID, not a commit, not an immutable deployed build.**

## B21 — backup/restore

- **Superseded by the second pass:** repeatable tooling now exists (`scripts/customer-v4/pgbackup.py`); see "Local operational closure". The first-pass manual drill below is kept as history.
- **First-pass manual drill:** `pg_dump -Fc` → restore into a fresh database → compare tables/triggers/constraints/indexes, on migration-tip schemas built by each service's own runner. Every result matched exactly:

| Service | Tables | Triggers | Constraints | Indexes | Duration |
|---|---|---|---|---|---|
| Customer (013) | 15 | 0 | 182 | 41 | 6s |
| Taxi (021) | 52 | 9 | 408 | 136 | 3s |
| Dispatch (000066) | 109 | 35 | 875 | 363 | 4s |
| Hauling (026) | 43 | 9 | 338 | 125 | 3s |
| Vendor (000129) | 408 | 75 | 4165 | 1476 | 24s |
| Media | 11 | 3 | 156 | 37 | 2s |
| Notification | 6 | 0 | 123 | 17 | 3s |

This proves the V4 schemas round-trip. It says nothing about staging backups: none has been identified (see `docs/FINAL_BACKUP_RESTORE_EVIDENCE.md`). No RTO is claimed.

## B24 — release bundle (prepared locally, not frozen)

| Artefact | Location |
|---|---|
| Mounted route inventory | `tmp/customer1921-route-audit/*.log` (gin capture), `classification.json` |
| Public customer OpenAPI | `services/*/docs/openapi/customer-v4-route-register.openapi.json` (Customer, Taxi, Dispatch, Hauling, Media); Vendor `docs/openapi/section17*`, `section18*`, `shop-picking-*`, `basket-optimization*`, `scheduled_group_rfq_recurring_commerce*`, `customer-commerce-discovery-preview*`; N01–N12 per §23 |
| Schema versions / migrations | B00 tips table |
| Feature flags (all default **false**) | `CUSTOMER_SHARE_JOURNEY_ENABLED`, `TAXI_CUSTOMER_LIVE_LOCATION_ENABLED`, `CUSTOMER_MEDIA_UPLOAD_SESSION_ENABLED`, `CUSTOMER_FOOD_ENABLED`, `CUSTOMER_SHOP_ENABLED`, `EXTERNAL_REFERENCE_REPRICING_ENABLED`, `CUSTOMER_UNIFIED_MOVEMENT_HISTORY_ENABLED`, `CUSTOMER_MOVEMENT_EVENTS_CONSUMER_ENABLED`, `CUSTOMER_MOBILITY_PREFERENCES_ENABLED`, `CUSTOMER_GUEST_TRAVELLERS_ENABLED`, `CUSTOMER_TRUSTED_CONTACTS_ENABLED`, `VENDOR_CUSTOMER_MOVEMENT_GROUPING_ENABLED`, `TAXI_SERVICE_PRODUCTS_ENABLED`, `TAXI_CUSTOMER_CONTEXT_ENABLED`, `TAXI_CUSTOMER_STANDING_ENFORCED`, `DISPATCH_CUSTOMER_STANDING_ENFORCED`, `HAULING_CUSTOMER_STANDING_ENFORCED`, `COMMERCE_BULK_RFQ_ENABLED`, `DISPATCH_HANDOVER_CHALLENGE_ENABLED` (plus its rollout cohort) |
| Event catalogue | Shared envelope + `MovementSummary` (§22); `commerce.customer_order.movement`; `commerce.scheduled_order.accepted`, `commerce.rfq.*`; notification event families in `shared/go/notifications` |
| Deep-link intents | `RIDE_DETAIL`, `TAXI_GOODS_MISSION`, `DISPATCH_TRIP_DETAIL`, `DISPATCH_MISSION_DETAIL`, `TRUCK_BOOKING_DETAIL`, `COMMERCE_ORDER_DETAIL`, `PAYMENT_DETAIL`, `SHARE_MANAGEMENT`, `SUPPORT_CASE_DETAIL` (always `read_back:true`) |
| Caller allowlists | Customer `CUSTOMER_CONTEXT_CALLER_SCOPES` (Taxi: mobility_preferences, saved_location, guest_traveller, account_standing; Dispatch and Hauling: account_standing; Vendor: saved_location); `CUSTOMER_SHARE_*_SECRET` per authority; movement ingress caller→authority map; metrics scrapes limited to admin-backoffice and analytics |
| Workers | Share expiry and notifications; per-authority movement reconcilers; Vendor purchase-row worker/reconciler; Wallet/refund and mission read-back; notification retry/dead-letter; Dispatch challenge expiry; Media orphan maintenance; Section 21 cutoff notifications and RFQ expiry (flag-gated) |
| Observability catalogue | §27 above |
| External adapters | Reference/Repricing (absent, gated); Wallet/Paystack; Notification providers (FCM/email); Firebase Storage (Media) |
| Build identifier | `5b2050a8` plus a dirty tree. **Not immutable.** Freezing requires a commit and image build, which are not authorized here. |

## §31 — deployment sequencing and rollback

1. Apply additive migrations first: customer through 013; Taxi, Dispatch, Hauling and Vendor through the tips. No V4 migration is destructive. Customer's runner re-applies every file idempotently.
2. Deploy the authority internal endpoints: share adapters, Customer context (now with `guest_traveller`), movement ingress and metrics scrapes. Deploy them with their HMAC secrets and allowlists before enabling any Customer flag.
3. Enable in isolation: movement consumer → Ride live location → Share Journey → Media upload sessions → Food and Shop independently → Section 21 (`COMMERCE_BULK_RFQ_ENABLED`).
   - Guest travellers need `CUSTOMER_GUEST_TRAVELLERS_ENABLED`, `TAXI_SERVICE_PRODUCTS_ENABLED` and `TAXI_CUSTOMER_CONTEXT_ENABLED`, with the Taxi scope granted.
   - External Repricing is a separate, gated release.
4. Rollback is by flag, never by destructive schema rollback. Disabling Food or Shop refuses new carts while existing carts, orders, payments, refunds and support stay readable. Disabling Share stops all share access (typed 503) without leaking. Unknown money or mission states reconcile through the read-back workers before any manual compensation.

## B23 blockers (genuine, external)

1. **No staging access.** VPS host, user and SSH key exist only as GitHub Actions secrets. There is no kube or cloud credential and no staging database URL.
2. **No immutable build.** Nothing is committed or pushed (not authorized), so there is no image digest to deploy or roll back to.
3. **Only three services have deploy workflows** (api-gateway, media-file-service, vendor-service). Customer, Taxi, Dispatch, Hauling, Wallet, Notification and Support have none in this repository.
4. **No monitoring target** (Prometheus/Grafana/alerting) to load the §27 metrics into.
5. **No staging backup** to restore; no RTO or SLO supplied.
6. Maps/Firebase key restriction review and the External Reference/Repricing adapter are external owners' decisions.

Each item needs an operator action; none can be substituted with localhost evidence.

## §32.1 — GO gates

| Gate | Status |
|---|---|
| Staging HEAD re-audited and frozen in B00 | Local HEAD audited and manifested; **not staging** |
| No duplicated authority | PASS (the guest traveller reuses Customer context; metrics reuse existing registries) |
| Movement projection receives all five families and reconciles | PASS local |
| Taxi location ownership/stale/privacy/load | PASS local |
| Share N01–N11 unit/integration/security/E2E | PASS local |
| Dispatch secure handover gated and credential-private | PASS local |
| Food/Shop public contracts, logistics profile, preview; no staff exposure | PASS local |
| Wallet unknown-outcome/refund black-box | Local with a test-double Wallet; **staging black-box not run** |
| Media without a public-client service token | PASS (Customer app); the provider-app finding is recorded |
| Notification deep-link catalogue complete | PASS (support added) |
| Public/internal ingress isolation | PASS local; **deployed network isolation not proven** |
| Backup/restore, monitoring/alerts, credentials, failure injection, bounded load retained | Local: backup/restore tooling and drill, alert rules and dashboard, failure injection and bounded load all retained. Staging backups, deployed alerts and credentials: **BLOCKED (B23)** |
| Reference/Repricing gated; no invented pricing | PASS |

## Remaining non-blocking notes

- **Email-only third-party receivers are refused before custody** (`RECEIVER_SECURE_CHANNEL_REQUIRED`), as are phone-only ones. V4 §12 does not require an email-only receiver channel, and SMS is explicitly not required. Supporting such receivers needs a product-owned mailbox-verification contract. That is a deferred capability, not a V4 gate.
- `.github/workflows/customer-v4-certification.yml` runs the V4 gates; hosted execution is pending a push.
