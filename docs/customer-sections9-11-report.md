# Customer backend Sections 9–11 implementation report

Verdict: **SECTIONS 9–11 IMPLEMENTATION: COMPLETE LOCALLY. STAGING CERTIFICATION: PENDING EXTERNAL DEPLOYMENT CONFIGURATION.** Not staging-certified and not production-ready: no deployed environment was tested. Both new feature flags remain false by default. No frontend was changed; nothing was deployed, committed or pushed.

## Closure outcome — 2026-09-27 (continuation run, local only)

| ID | Outcome |
|---|---|
| B1 | **LOCAL CODE/CONFIG PASS.** STAGING DEPLOYMENT CERTIFICATION: EXTERNAL PENDING |
| B2 | **LOCAL RUNNING-SERVICE E2E PASS.** STAGING RUNNING-SERVICE E2E: EXTERNAL PENDING (B1) |
| B3 | **RACE PASS. LOCAL SUSTAINED LOAD PASS.** PRODUCTION SLO CERTIFICATION: EXTERNAL PENDING (no SLO supplied) |
| B4 | **PASS** |
| B5 | **PASS** |
| B6 | **PASS** |

### Defects found and fixed in this run

1. **Dispatch hold reconciliation never ran (pre-existing, real).** `claimAbandonedHolds` compared `updated_at < $1-$5::interval`; PostgreSQL typed `$1` as interval and rejected the query on every 10 s tick, so no abandoned wallet hold was ever reconciled. Found from the running stack's log. Fixed with an explicit `timestamptz` cast; new DB test `TestClaimAbandonedHoldsRunsAgainstPostgres` (the query had no coverage).
2. **Journey-share authority reads exhausted sockets under load (real).** `journeyshare.Client` fell back to Go's default transport (2 idle connections per host), so concurrent Customer→authority and Vendor→executor reads opened a TCP connection per request; a 5-minute load run hit Windows ephemeral-port exhaustion (4 N11 failures). Now a shared pooled client (64 idle/host, redirects still refused). Regression test `TestClientReusesConnectionsUnderConcurrency`; the rerun had zero unexpected errors and 65 TIME_WAIT sockets.
3. **Vendor N11 unknown-executor refusal was untested.** Extracted `executorAuthority` (fail-closed mapping) with a unit test; a DB CHECK also makes unknown services unpersistable (confirmed live).
4. Earlier in this closure run (previous agent): duplicate Taxi `/api/v1/taxi/metrics` registration crashed startup — fixed; Customer no longer logs the full database URL.

Validation-harness fixes (not product code): the local-stack script now runs on Windows PowerShell 5.1, supplies the Taxi Goods/movement/debug-OTP fixture settings, trusts Taxi under its canonical caller name `driver-taxi-service` (the old fixture name caused a 403 on movement reconciliation), and runs Dispatch as `development` (its only way to skip mandatory OTP delivery); the isolated-DB script no longer depends on `rg` or dies on native stderr.

### B1 — local code/config

Customer refuses to start with sharing enabled unless every source origin is a bare HTTP(S) origin, every caller secret is set, the viewer origin is exact, TTL/retention are bounded and gateway CIDRs parse; errors name settings, never values (`TestJourneySharingStartupFailsClosedWithoutLeakingSecrets`, new per-field `TestJourneySharingEachRequiredSettingFailsClosed`, 16 cases). Disabled config starts. Taxi refuses live tracking without its `customer-service` HMAC secret and refuses Goods without its media/payment/vendor dependencies (observed live). HMAC caller allowlists, exact CORS, trusted-hop client IP and public rejection of private routes are covered by the contract/parity and gateway suites and were re-exercised on the running stack (wrong caller 403, bearer on private route rejected, foreign owner 404). Real staging values were not available and were not requested.

### B2 — local running-service E2E

Driver: `shared/go/journeyshare/journeysharetest/stacke2e` (black-box HTTP against the five running processes; SQL only places the authority-owned booking/trip/mission rows that booking/matching would create) plus `vendor-service/internal/features/fulfilmentgroup/customer_share_stack_e2e_test.go` for Vendor-owned runtime transitions. Scripts: `docs/validation/sections911-local-stack.ps1`, `sections911-stack-e2e.ps1`, `sections911-stack-outage.ps1`; logs under `docs/validation/sections911-stack-results/`.

| Flow | Result on running stack |
|---|---|
| Ride self | Real provider telemetry ingestion → N07 FRESH; foreign owner 404; N08 caller/bearer boundary; completion terminalizes N07 |
| Ride share | N01 → N06 → live update visible → N02/N03 (no token leak, foreign 404) → N05 rotate (old token 404) → stale-version revoke 409 → N04 revoke (token 404) → completion suppresses location; post-terminal ingestion cannot re-expose it |
| Send share | Real rider OTP login → online → location → N09 FRESH, IDOR 404, wrong caller refused → N06 live update → completion terminal → movement projection observed |
| Truck share | Provider online with truck → heartbeat → N10 → N06 live update → completion terminal → movement projection observed |
| Food/Shop via DISPATCH | Vendor N11 → Dispatch peer read → mission-bound FRESH location |
| Food/Shop via HAULING | Vendor N11 → Hauling peer read → mission-bound FRESH location |
| Food/Shop via TAXI_GOODS | Vendor N11 → Taxi Goods read → `UNAVAILABLE` location while the same customer's passenger ride has fresh telemetry (no passenger fallback) |
| Food via VENDOR_OWN_DELIVERY | N01/N06 through running Customer → Vendor → canonical runtime PLANNED → ASSIGNED → COMPLETED (terminal), location `UNAVAILABLE`, no staff/business/branch leak |
| Projection outage | Taxi stopped mid-share → N06 typed `503 service_unavailable`, no leak → stack restarted → same capability recovers 200 |

Movement: with publishing enabled, Taxi's reconciler republished 4 rides and Customer accepted them; Send/Truck projections appeared automatically. SQL-placed Taxi completions reach Customer only on the reconciler cadence (5 min), as designed.

### B3 — race and local sustained load

Race (Linux CGO container, `-race`, disposable DBs): all 15 changed packages passed with zero skips (`sections911-race-results/race-summary.json`); packages changed afterwards were rerun and passed with no data race: `shared/go/journeyshare/...` (11 pass; 6 skips are the gated stack tests), Vendor `customershare` (3), `fulfilmentgroup` (124), `cmd` grouping/backfill (4), Dispatch `booking` (73).

Sustained load (`TestStackSustainedLoad`, running stack, 5 min, 16 concurrent readers round-robin over N06–N11, concurrent Taxi ingestion + Dispatch ping + Hauling heartbeat writes):

| Path | Requests | OK | Typed 429 | Unexpected | p50 | p95 | p99 | Max |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| N06 | 69,727 | 300 | 69,427 | 0 | 5.2 ms | 11.6 ms | 21.0 ms | 215 ms |
| N07 | 69,727 | 69,727 | 0 | 0 | 4.3 ms | 9.5 ms | 15.5 ms | 222 ms |
| N08 | 69,727 | 69,727 | 0 | 0 | 6.4 ms | 13.5 ms | 20.8 ms | 221 ms |
| N09 | 69,727 | 69,727 | 0 | 0 | 12.0 ms | 23.8 ms | 36.6 ms | 234 ms |
| N10 | 69,727 | 69,727 | 0 | 0 | 12.1 ms | 24.0 ms | 37.3 ms | 239 ms |
| N11 | 69,727 | 69,727 | 0 | 0 | 19.9 ms | 38.2 ms | 59.9 ms | 247 ms |

418,362 reads, 0 unexpected. N06 served exactly its 60 reads/minute per-token budget (300 in 5 min) and typed 429s otherwise. Provider writes: 3,474, 0 errors (1,009 typed Dispatch ping-throttle 429s at the driver's 4/s cadence). Mid-load: 39 DB connections across five databases, 0 lock waits; Redis 57 clients, 0 blocked, 0 rejected, ~1.3k ops/s; service working sets stable (17–39 MB, no growth trend), thread/handle counts flat. **LOCAL LOAD VALIDATED / PRODUCTION SLO NOT CERTIFIED** — single host, no deployment polling budget supplied.

### B4 — PASS

All four executors verified on the running stack (table above) and by owning-service DB tests: canonical mission and owning authority, ownership (foreign owner 404), exact caller (wrong caller refused), nonce replay, mission-bound location only where the executor has authoritative GPS, `UNAVAILABLE` otherwise (Taxi Goods, Vendor Own Delivery), no global-provider or passenger fallback, terminal suppression, source version ≥ 1, per-authority feature gates. Vendor N11 fails closed for an unknown executor (code path unit-tested; DB CHECK rejects persisting one).

### B5 — PASS

`BackfillBatch` reconstructs grouping only from the persisted ACTIVE plan's slot sequence and actual mission bindings under canonical parent ownership; anything else is `UNRECOVERABLE` — never inferred from timestamps or UUID order. Tests on a disposable Vendor DB (`sections911-stack-results/B5-backfill.log`):

- dry run writes nothing; canonical mission order preserved; completed-run replay repairs 0;
- bounded: 133 parents in 3 batches of ≤ 50; the cursor is committed per batch and a fresh publisher (restart) resumes from it; every parent classified exactly once;
- three concurrent runs: exactly one `REPAIRED`, the others `ALREADY_CORRECT`; exactly one grouping event;
- a pre-existing checkpoint is untouched (version/timestamp unchanged, no event);
- no mission created/recreated, no platform binding added, no parent or vendor-order lifecycle change, no non-grouping outbox event.

Counts (concurrent run): eligible 1, repaired 1, already_correct 14, unrecoverable 24, failed 0. Genuinely unrecoverable historical purchases are identified, not invented.

### B6 — PASS

One final authoritative pass (`sections911-isolated-db.ps1 -RunPrefix c4`, evidence `docs/validation/sections911-results-c4/`): every package of Customer, Taxi, Dispatch, Hauling, Vendor and `shared/go`, each on its own disposable clone of a template built by that service's actual migration runner, packages serialized. A package whose tests all skip is reported as SKIPPED, never PASS; packages with no test files are counted separately.

| Service | Packages pass | No tests | Tests pass | Fail | Skipped |
|---|---:|---:|---:|---:|---:|
| Customer | 19 | 17 | 70 | 0 | 0 |
| Taxi | 33 | 18 | 557 | 0 | 0 |
| Dispatch | 36 | 12 | 1,731 | 0 | 1 |
| Hauling | 28 | 16 | 359 | 0 | 0 |
| Vendor | 71 | 32 | 2,597 + reruns | 0 after reruns | 8 |
| shared/go | 38 | 5 | 510 | 0 after rerun | 6 (gated stack E2E) |

All three first-pass failures were classified test/harness issues and fixed, then rerun (`sections911-results-c4r/`):

- Vendor `internal/database` `TestCapabilityVocabularyMigrationRollbackAndReapply` assumed 000121 was the last migration; with this work's 000122/000123 its `RollbackLastMigration` undid 000123 instead. It now unwinds later migrations first, matching the fix the previous commit made to its sibling test. The whole package then passes (a second rerun hit two 60 s rollback-chain timeouts under host load; the full package passed cleanly on the next run, 458 s).
- Vendor `cart` Section 17A certifications refuse databases whose names lack `test`; the harness now names clones `<prefix>_test_<service>_<n>`. 18/18 pass.
- `shared/go/custodyops` needs a service-owned custody schema; run against a clone of the migrated Dispatch template with `CUSTODY_TEST_DATABASE_URL`: 21/21 pass.

No product regression was found.

## Final closure work — 2026-09-27

Re-audit before edits: branch `staging`, HEAD `5b2050a896942ddb4d191a3a90cb4af49342bd02`; existing Sections 9–11 tracked and untracked changes preserved. `git diff --check` passed (Git reported normal Windows line-ending conversion warnings). Audit and implementation report inspected. No reset, revert, commit or push.

| Blocker | Closure gap at start | Required evidence/action |
|---|---|---|
| B1 | Deployment-owned staging values unavailable in supplied request | Obtain config/access without inventing secrets/origins; certify real staging ingress/connectivity |
| B2 | Source/peer parity missing; final E2E used component fixtures | Mounted contract checks and actual stack lifecycle evidence |
| B3 | Windows CGO disabled; only short load samples | CGO-capable race run; sustained observations; formal SLO acceptance belongs to Product/SRE |
| B4 | Taxi Goods and Vendor own executor reads absent | Distinct Goods mission read and canonical Vendor runtime routing; no global/passenger GPS fallback |
| B5 | Only recorded grouping events repaired | Canonical persisted sequence reconstruction or explicit UNRECOVERABLE classification; bounded resumable dry-run/backfill |
| B6 | Destructive shared DB fixtures invalidated broad parallel execution | Dedicated disposable databases and serialized packages with actual owning migration runners; preserve fail/skip evidence |

## 1. Starting branch and HEAD

`staging`, `5b2050a896942ddb4d191a3a90cb4af49342bd02`. `git fetch origin staging` completed before editing; fetched `origin/staging` matched HEAD. The initial working tree was clean. The pre-implementation audit is in `docs/customer-sections9-11-audit.md`.

## 2. Ending branch and HEAD

`staging`, the same HEAD. Changes are reviewable working-tree modifications and new files; no commit was created.

## 3. Files changed

The complete generated inventory is appended below. Changes are limited to Customer, Taxi, Dispatch, Hauling, Vendor, gateway regression tests, shared backend helpers, migrations and documentation.

## 4. Migrations added

| Owner | Migration | Purpose |
|---|---|---|
| Customer | `010_journey_share_sessions.sql` | Digest-only sessions, bounded permissions, owner/source indexes, active-session uniqueness, optimistic version, metadata-only idempotency ledger, expiry/observation indexes |
| Customer | `012_movement_commerce_product.sql` | Vendor-owned FOOD/SHOP grouping classification |
| Dispatch | `000065_customer_movement_reconcile_index.up/down.sql` | Indexed bounded replay lookup |
| Hauling | `025_customer_movement_reconcile_index.up/down.sql` | Indexed bounded replay lookup |
| Vendor | `000122_customer_grouping_emissions.up/down.sql` | Durable monotonic grouping fingerprint/version checkpoint and replay index |

Historical migrations were not edited. Customer's existing runner has no paired-down convention: its new SQL was applied repeatedly and tested with transactional rollback/reapply in a disposable schema. The new Vendor paired migration passed up/down/up in a disposable transaction. Dispatch/Hauling indexes applied successfully; destructive rollback of live data was not performed.

## 5. Existing APIs/use cases reused

Customer auth, profile, saved locations, trusted contacts, mobility context, guests, notification boundaries and the single existing Activity projection remain canonical. Taxi reuses the existing booking-bound telemetry ingestion and current booking/provider/vehicle records. Dispatch reuses the existing availability location update; Hauling reuses the existing provider heartbeat. The added observers retain only a latest point bound by the owning service to its current journey and assignment epoch. No ingestion route, provider assignment authority, pricing logic, credentials store or second Activity projection was added.

## 6. PROVIDER_API_DEPENDENCY_MATRIX

| Domain / dependency | Owner | Verified existing implementation | Decision | Change / proof |
|---|---|---|---|---|
| Ride provider identity | Taxi | `provider_auth`, `provider_profile`, `taxi_providers` | REUSE | First name only; no phone, identity documents or private IDs |
| Ride assignment / vehicle | Taxi | `booking` repositories; `taxi_bookings.provider_id/vehicle_id` | REUSE | Single SQL snapshot; active assigned vehicle bound to the assigned provider |
| Ride lifecycle | Taxi | Existing accept/start/complete/cancel booking use cases | REUSE | Native status determines eligibility/terminal behavior |
| Ride telemetry | Taxi | POST `/api/v1/taxi/provider/location/events`, `booking/usecases/telemetry.go`, `taxi_trip_location_points` | EXTEND | Existing ingestion exercised against PostgreSQL; latest validated current-provider point only |
| Ride customer/share read | Taxi | No bounded safe sharing read existed | CREATE | N07/N08 use the same safe reader, feature gate and ownership rules |
| Send lifecycle / assignment | Dispatch | `trip` service/repository and current trip/provider binding | REUSE | Native trip status and owner assertion |
| Send position | Dispatch | Existing booking live read and availability update/store | EXTEND | A global position does not prove a trip binding; new observer records a latest bound point after accepted ingestion |
| Send safe internal read | Dispatch | No suitable Customer-only sharing DTO existed | CREATE | N09, HMAC v2, existing PostgreSQL nonce guard, current-binding recheck |
| Truck lifecycle / assignment | Hauling | `booking/usecases/booking_service.go`, haulage booking repository | REUSE | Native booking/current provider/truck |
| Truck position | Hauling | Existing booking location and provider heartbeat | EXTEND | Latest bound point; no arbitrary provider/global position exposed |
| Truck safe internal read | Hauling | No suitable Customer-only sharing DTO existed | CREATE | N10, existing HMAC/PostgreSQL replay boundary, current-binding recheck |
| Food/Shop mission ownership | Vendor | `commerce_order_missions`, `commerce_parent_orders`, existing logistics integrations | EXTEND | N11 scopes one accepted mission and checks both mission and parent owner |
| Commerce logistics tracking | Dispatch / Hauling | Existing mission-to-booking/trip bindings | CREATE scoped read | Vendor-only mission read-through, not a new assignment/lifecycle API |
| Credential/custody domains | Existing provider services | Passenger, parcel, Goods and handover use cases | REUSE, excluded | No OTP, QR, unlock, payment or custody material appears in sharing DTOs |
| Dispatch assigned bike | Dispatch | No canonical bike ID in the trip row used by this contract | OMIT | Never selects a provider's arbitrary primary bike |
| Other commerce executors | Taxi Goods / Vendor Own Delivery | Separate authority and credential domains | PARTIAL: B4 | No passenger/global GPS fallback; missing scoped binding fails closed |

The audit file records the source paths examined before implementation. New internal reads were added only where the required safe/mission-scoped contract was absent.

## 7. Provider APIs created because no suitable API existed

N07 and N08–N11 are read-only safe projections over their owning service's canonical records. Two additional Vendor-only commerce reads resolve the existing Dispatch/Hauling mission binding before invoking the same safe journey reader. No new provider location ingestion endpoint was created.

## 8. Public routes added

| ID | Method and path | Boundary |
|---|---|---|
| N01 | POST `/api/v1/customer/journey-shares` | Customer bearer; required idempotency key |
| N02 | GET `/api/v1/customer/journey-shares` | Customer bearer; bounded tenant-scoped listing/filters |
| N03 | GET `/api/v1/customer/journey-shares/{shareId}` | Customer bearer; owner metadata only |
| N04 | POST `/api/v1/customer/journey-shares/{shareId}/revoke` | Customer bearer; expected version and idempotency key |
| N05 | POST `/api/v1/customer/journey-shares/{shareId}/rotate` | Customer bearer; expected version and idempotency key |
| N06 | GET `/api/v1/customer/public/journey-shares/{token}` | Capability only; Redis token/IP limits, exact configured CORS origin |
| N07 | GET `/api/v1/taxi/customer/bookings/{id}/location` | Customer bearer; passenger booking owner only |

N01/N05 disclose a newly generated 32-byte random token once. Replays disclose metadata, never a persisted plaintext token. A lost capability is recovered by an explicit new rotation with the current version.

## 9. Internal routes added

| ID | GET path | Exact caller |
|---|---|---|
| N08 | `/internal/taxi/v1/customer-share/bookings/{sourceRef}` | `customer-service` |
| N09 | `/internal/dispatch/v1/customer-share/trips/{sourceRef}` | `customer-service` |
| N10 | `/internal/hauling/v1/customer-share/bookings/{sourceRef}` | `customer-service` |
| N11 | `/internal/vendor/v1/customer-share/missions/{sourceRef}` | `customer-service` |
| Peer | `/internal/dispatch/v1/commerce-share/missions/{sourceRef}` | `vendor-service` |
| Peer | `/internal/hauling/v1/commerce-share/missions/{sourceRef}` | `vendor-service` |
| Metrics | `/internal/customer/v1/journey-share-metrics` | `admin-backoffice-service` |

All new private reads use HMAC v2 plus nonce replay protection. `customer_id` is a signed query assertion and is checked against canonical ownership. Gateway tests confirm the private paths are never forwarded publicly. Taxi's N08 remains unmounted when the existing internal verifier is absent; enabling live tracking now requires the Customer caller secret at startup.

## 10. Existing routes changed

No existing public route was removed or renamed. Existing provider location/heartbeat use cases gained a bounded observation callback after their accepted update. Customer Activity retains existing service-specific product codes while RIDE/SEND/TRUCK filters recognize the product family. Vendor alone supplies FOOD/SHOP classification. Grouping application now preserves classification across newer execution events and refuses grouping ownership reassignment.

The existing private Customer movement-state read now returns only grouping versions for authorized grouping callers; it cannot expose Taxi/Dispatch/Hauling state to Vendor. The existing private Taxi metrics scrape includes safe tracking counters.

## 11. OpenAPI and contract documentation

Created Customer journey-sharing and private-metrics specifications; Taxi customer-location and share-projection specifications; Dispatch/Hauling safe projection specifications including the two peer reads; Vendor mission projection specification. Updated Vendor's route register, parity artifact and structural contract counts to 858 mounted routes / 868 declarations.

Customer's six sharing routes pass mounted-versus-OpenAPI parity. Vendor's existing whole-router inventory, reference/security/operation-ID checks pass. The other source adapters are exercised through their real registrars/boundaries in integration tests; an aggregate mounted-versus-spec gate for every added source/peer operation is still required under B2.

## 12. Feature flags/configuration

New default-false flags: `CUSTOMER_SHARE_JOURNEY_ENABLED`, `TAXI_CUSTOMER_LIVE_LOCATION_ENABLED`. Taxi's flag gates both N07 and N08. Additional Customer settings configure source origins/secrets, hard TTL, completed retention, official viewer origin and trusted gateway CIDRs. Taxi configures the freshness interval and rejects enabled tracking without its `customer-service` HMAC secret. Existing movement, commerce integration and grouping flags remain authoritative. See `services/customer-service/docs/JOURNEY_SHARING_RUNBOOK.md` for exact settings and source reference semantics.

## 13. Event publishers changed

Dispatch/Hauling now cycle through committed booking IDs in bounded keyset pages, avoiding the old timestamp-window first-page recovery gap. Existing fingerprints/outboxes still control idempotent emission and monotonic source versions. Vendor grouping now uses a transactional fingerprint/version checkpoint instead of Unix-second versions, and its delivery adapter unwraps the Vendor envelope into the shared grouping envelope Customer actually accepts. Vendor never publishes operational lifecycle state.

Taxi's existing transactional movement writer/outbox was reused and verified separately against a clean database. N07/N08 report a monotonic booking version from the existing source/movement versions without inventing a new business version.

## 14. Reconciliation jobs

Added bounded Dispatch/Hauling outbox-to-Customer version repair. Missing/behind projections are requeued with a new event ID and the same business version; already pending or caught-up entries are not replayed. Their lookup indexes were added in owning migrations.

Added Vendor grouping repair/backfill: it uses the latest recorded grouping, verifies canonical parent ownership/domain, preserves recorded mission order, creates only a missing checkpoint, and requeues missing projection versions without advancing business state. Checkpoint locking prevents stale/concurrent repair. Historical purchases with no recorded grouping event are handled by the canonical-plan backfill (`BackfillBatch`, migration 000123) or classified `UNRECOVERABLE` (B5 PASS). Taxi's existing reconciler remains canonical and its missing-versus-current test passed on an isolated clean database.

## 15. Workers changed

Customer adds bounded expiration/completion observation, even if nobody opens a link. Expiration is enforced synchronously on every public read. Source completion/cancellation produces a short-lived, location-free summary. Dispatch/Hauling repair and Vendor grouping repair run under existing owning feature flags. Source HTTP calls have bounded timeouts, and worker errors are logged by safe outcome category.

## 16. Tests added/extended

- Token entropy, digest-only serialization, permission/authority validation, privacy and terminal suppression.
- Customer PostgreSQL lifecycle, idempotency replay/conflict, owner isolation, optimistic versions, concurrent revoke, rotate invalidation, completion/expiry sweep and feature gate.
- Customer HTTP bearer/spoofing, metadata privacy, CORS, cache/index/referrer headers, Redis rate limit, commands and mounted contract parity.
- Taxi real telemetry ingestion through the existing booking use case, N07/N08, signed source read, ownership, source route presence/absence, reassignment, FRESH/STALE/UNAVAILABLE, terminal and flag behavior; concurrent ingestion during polling.
- Dispatch/Hauling PostgreSQL/Redis bound-point adapters, HMAC/replay boundary, ownership, reassignment, secret absence and durable publisher output.
- Vendor real parent/mission scope for FOOD and SHOP, signed source boundary, per-authority gate and safe DTO; logistics reader is an explicit fixture.
- Customer grouping order independence, ownership immutability, classification retention, grouping-only version reads and all-five-product filters.
- Real shared movement delivery/retry and missing-version repair.
- Vendor real transactional grouping versions, replay idempotency, correct signed HTTP envelope and preserved-version repair.
- Gateway rejection of all new internal/peer paths; trusted-forwarding spoof rejection.

## 17. Tests executed

Full regression command covered `customer-service/...`, `driver-taxi-service/...`, `driver-dispatch-delivery-service/...`, `driver-hauling-service/...`, `vendor-service/...`, `api-gateway/...`, and `shared/go/...`. Customer's database suite was enabled. Database-dependent suites requiring other infrastructure were separately selected; a skipped test is not certification.

Executed all new feature adapter suites with PostgreSQL and Redis, Customer movement integration, shared movement integration, Vendor grouping transaction/delivery/repair integration, Taxi migration/reconciliation/outbox/observer integration on a separate clean database, and the initially interfering Dispatch authority/marketplace/private-execution packages serially. Final focused checks covered changed HTTP/config/registrars and Vendor's whole OpenAPI inventory.

Evidence logs in the workspace: `sections911-all-regression-tests.log`, `sections911-final-feature-db-tests.log`, `sections911-final-taxi-location-tests.log`, `sections911-final-grouping-db-tests.log`, `sections911-final-verification-tests.log`, `sections911-taxi-regression-db-tests.log`, `sections911-dispatch-isolated-regression-tests.log`, `sections911-movement-db-tests.log`, `sections911-migration-tests.log`, `sections911-grouping-migration-tests.log`. Logs are ignored local test artifacts, not source changes.

## 18. Results

The full default regression run passed **241 package suites**; other packages had no tests. New selected feature/adapter suites passed with actual PostgreSQL/Redis. Customer movement and all-five filters passed. Shared delivery/reconciliation and Vendor grouping delivery/reconciliation passed. Taxi clean-runner migration replay, existing reconciliation, outbox, recovery/rollback and network observer checks passed. Dispatch's three interfering packages passed serially. `git diff --check` passed.

An initial broad database run was invalidated by shared destructive fixtures and untracked manually applied Taxi DDL: it produced foreign-key/count/deadlock/migration conflicts. Those outcomes were retained in local logs, not silently counted as passes. The affected Taxi packages passed with its actual migration runner on a separate clean database; the Dispatch failures passed after serial isolation. The concurrent broad authority run was stopped once interference was established. A full serial database run of every legacy package has not been certified (B6).

`go test -race` cannot run on the Windows host (CGO disabled); it was executed in a Linux CGO container instead and passed (see Closure outcome, B3).

## 19. Security findings

Verified: no token in session or command persistence; one-time new token disclosure; owner-only metadata/commands; strict JSON rejects spoofed customer fields; optimistic mutation guards; signed owner assertions; exact new private caller allowlists and replay protection; no public private-route forwarding; no public owner/source IDs or provider/credential/payment secrets; immediate old-token invalidation after rotation/revoke, including recheck after dependency I/O; exact-origin CORS; no-store/noindex/no-referrer; token/IP rate limiting; trusted-hop resolution rejects spoofed forwarding; current assignment suppression; terminal location suppression; ETA stays null without authority evidence.

Taxi passenger sharing excludes direct parcel/Goods bookings. Dispatch/Hauling never fall back to an unrelated global provider point. A missing Taxi route is omitted rather than fabricating `(0,0)` endpoints. An unproven Dispatch bike is omitted. Full live deployment security/E2E certification remains B1/B2/B3/B4; no claim is made that fixtures prove it.

## 20. Performance/load results

Local bounded load used eight concurrent readers:

| Path | Requests | p50 | p95 | Maximum | Evidence/limit |
|---|---:|---:|---:|---:|---|
| N06 | 40 | 5.65 ms | 35.92 ms | 40.84 ms | Real session DB/Redis/HTTP; explicit source fixture |
| N07 | 200 | 1.59 ms | 4.62 ms | 35.57 ms | Real Taxi DB safe read, bearer, concurrent existing ingestion writes |
| N08 | 200 | 3.26 ms | 5.51 ms | 13.18 ms | Real Taxi DB, HTTP HMAC and Redis replay boundary |

The token's sixty-read minute budget was enforced; subsequent requests returned 429. `EXPLAIN ANALYZE` for an unmatched digest used `journey_share_sessions_token_hash_key` with 0.055 ms execution on this small local dataset. These were superseded by the 5-minute running-stack load in the Closure outcome (B3), which covers N06–N11 including multi-hop commerce. No production SLO is asserted.

## 21. External dependencies

Private source origins, matching caller secrets, official viewer origin, actual gateway CIDRs, reliable Redis/PostgreSQL, existing movement/grouping feature/configuration, deployment telemetry and real lifecycle actors. None was guessed or enabled in a deployment. Local disposable PostgreSQL/Redis containers remain available for review on host ports 55432/56379; no service daemon or frontend was deployed.

## 22. Operational blockers and exact next actions

Only external, deployment-owned items remain:

| ID | Remaining item | Owner | Next action |
|---|---|---|---|
| B1 | Real staging source origins, caller secrets, official viewer origin and gateway CIDRs | Platform/SRE + Customer/provider owners | Supply values; startup validation fails closed until complete; verify private connectivity and forwarded-IP behavior; keep flags false until acceptance |
| B2 | Running-service E2E on deployed staging | QA | Re-run `sections911-stack-e2e.ps1`-equivalent flows against staging once B1 is configured |
| B3 | Production polling budget / SLO | Product/SRE | Agree the budget; compare against the local figures above on production-like infrastructure |

## 23. Section 9 status

**COMPLETE LOCALLY.** Customer regression and isolated-DB suites green; one Activity projection handles all five product filters; durable source/grouping publication, version protection, bounded repair and canonical historical grouping backfill (B5) implemented and tested; movement projection observed end to end on the running stack. Staging certification pending B1.

## 24. Section 10 status

**COMPLETE LOCALLY.** N01–N06 lifecycle (create, list, get, rotate, revoke, expiry/completion, CORS, rate limits, cache headers) verified on the running stack for Ride, Send, Truck and Food/Shop via all four executors, plus projection outage/recovery and 5-minute sustained load. Staging certification pending B1.

## 25. Section 11 status

**COMPLETE LOCALLY.** N07/N08 verified with real telemetry ingestion, ownership, caller boundary, live updates, terminal suppression and under sustained concurrent reads/writes; race-clean. Staging certification pending B1.

## 26. Explicit verdict

**SECTIONS 9–11 IMPLEMENTATION: COMPLETE LOCALLY. STAGING CERTIFICATION: PENDING EXTERNAL DEPLOYMENT CONFIGURATION.** Not staging-certified and not production-ready: only local disposable infrastructure was exercised. No mock, skipped suite or disabled flag is counted as a pass.

## Complete changed-file inventory

<!-- generated inventory follows -->
- docs/customer-sections9-11-audit.md
- docs/customer-sections9-11-report.md
- services/api-gateway/internal/features/routing/http/edge_test.go
- services/customer-service/cmd/main.go
- services/customer-service/docs/JOURNEY_SHARING_RUNBOOK.md
- services/customer-service/docs/openapi/journey-sharing-metrics.openapi.json
- services/customer-service/docs/openapi/journey-sharing.openapi.json
- services/customer-service/internal/config/config.go
- services/customer-service/internal/features/journey_sharing/http/routes_integration_test.go
- services/customer-service/internal/features/journey_sharing/http/routes.go
- services/customer-service/internal/features/journey_sharing/models/session_test.go
- services/customer-service/internal/features/journey_sharing/models/session.go
- services/customer-service/internal/features/journey_sharing/repositories/postgres.go
- services/customer-service/internal/features/journey_sharing/usecases/service_integration_test.go
- services/customer-service/internal/features/journey_sharing/usecases/service.go
- services/customer-service/internal/features/journey_sharing/workers/expiry.go
- services/customer-service/internal/features/movement_history/http/routes.go
- services/customer-service/internal/features/movement_history/repositories/grouping_repository.go
- services/customer-service/internal/features/movement_history/repositories/postgres_integration_test.go
- services/customer-service/internal/features/movement_history/repositories/postgres_repository.go
- services/customer-service/internal/features/movement_history/usecases/service.go
- services/customer-service/migrations/010_journey_share_sessions.sql
- services/customer-service/migrations/012_movement_commerce_product.sql
- services/driver-dispatch-delivery-service/cmd/main.go
- services/driver-dispatch-delivery-service/docs/openapi/customer-share-projection.openapi.json
- services/driver-dispatch-delivery-service/internal/features/availability/service.go
- services/driver-dispatch-delivery-service/internal/features/customershare/service_integration_test.go
- services/driver-dispatch-delivery-service/internal/features/customershare/service.go
- services/driver-dispatch-delivery-service/internal/features/movement/movement.go
- services/driver-dispatch-delivery-service/internal/router/router.go
- services/driver-dispatch-delivery-service/migrations/000065_customer_movement_reconcile_index.down.sql
- services/driver-dispatch-delivery-service/migrations/000065_customer_movement_reconcile_index.up.sql
- services/driver-hauling-service/cmd/main.go
- services/driver-hauling-service/docs/openapi/customer-share-projection.openapi.json
- services/driver-hauling-service/internal/features/customershare/service_integration_test.go
- services/driver-hauling-service/internal/features/customershare/service.go
- services/driver-hauling-service/internal/features/movement/movement.go
- services/driver-hauling-service/internal/features/provider_availability/usecases/availability_service.go
- services/driver-hauling-service/migrations/025_customer_movement_reconcile_index.down.sql
- services/driver-hauling-service/migrations/025_customer_movement_reconcile_index.up.sql
- services/driver-taxi-service/cmd/main.go
- services/driver-taxi-service/docs/openapi/customer-live-location.openapi.json
- services/driver-taxi-service/docs/openapi/customer-share-projection.openapi.json
- services/driver-taxi-service/internal/config/config.go
- services/driver-taxi-service/internal/features/customertracking/http.go
- services/driver-taxi-service/internal/features/customertracking/service_integration_test.go
- services/driver-taxi-service/internal/features/customertracking/service.go
- services/vendor-service/cmd/customer_grouping_integration_test.go
- services/vendor-service/cmd/customer_grouping_reconcile.go
- services/vendor-service/cmd/customer_grouping.go
- services/vendor-service/cmd/main.go
- services/vendor-service/docs/API_ROUTE_REGISTER.md
- services/vendor-service/docs/openapi/customer-share-mission-projection.openapi.json
- services/vendor-service/docs/SECTION43_ROUTE_PARITY.json
- services/vendor-service/internal/crmcontract/inventory_test.go
- services/vendor-service/internal/crmcontract/section43_contract_test.go
- services/vendor-service/internal/features/customershare/service_integration_test.go
- services/vendor-service/internal/features/customershare/service.go
- services/vendor-service/internal/features/orders/fulfilment.go
- services/vendor-service/migrations/000122_customer_grouping_emissions.down.sql
- services/vendor-service/migrations/000122_customer_grouping_emissions.up.sql
- shared/go/events/movement_reconcile_integration_test.go
- shared/go/events/movement_reconcile.go
- shared/go/events/movement.go
- shared/go/journeyshare/boundary.go
- shared/go/journeyshare/client_ip.go
- shared/go/journeyshare/http.go
- shared/go/journeyshare/journeysharetest/load.go
- shared/go/journeyshare/metrics.go
- shared/go/journeyshare/points.go
- shared/go/journeyshare/projection_test.go
- shared/go/journeyshare/projection.go

Closure-run additions:

- docs/validation/sections911-isolated-db.ps1
- docs/validation/sections911-local-stack.ps1
- docs/validation/sections911-race.ps1
- docs/validation/sections911-stack-e2e.ps1
- docs/validation/sections911-stack-outage.ps1
- services/customer-service/internal/config/journey_share.go
- services/customer-service/internal/config/journey_share_test.go
- services/customer-service/internal/database/closure_integration_test.go
- services/driver-dispatch-delivery-service/internal/features/booking/hold_reconciliation.go
- services/driver-dispatch-delivery-service/internal/features/booking/hold_reconciliation_integration_test.go
- services/driver-dispatch-delivery-service/internal/features/customershare/contracts_test.go
- services/driver-dispatch-delivery-service/internal/features/customershare/http.go
- services/driver-hauling-service/internal/features/customershare/contracts_test.go
- services/driver-hauling-service/internal/features/customershare/http.go
- services/driver-taxi-service/docs/openapi/commerce-share-mission.openapi.json
- services/driver-taxi-service/internal/features/customertracking/contracts_test.go
- services/driver-taxi-service/internal/features/goods/customer_share.go
- services/driver-taxi-service/internal/features/goods/customer_share_contract_test.go
- services/driver-taxi-service/internal/features/goods/customer_share_integration_test.go
- services/vendor-service/cmd/customer_grouping_backfill.go
- services/vendor-service/cmd/customer_grouping_backfill_integration_test.go
- services/vendor-service/internal/config/config.go
- services/vendor-service/internal/database/closure_integration_test.go
- services/vendor-service/internal/features/customershare/contracts_test.go
- services/vendor-service/internal/features/customershare/executor_test.go
- services/vendor-service/internal/features/customershare/http.go
- services/vendor-service/internal/features/customershare/service_integration_test.go
- services/vendor-service/internal/features/fulfilmentgroup/customer_share_integration_test.go
- services/vendor-service/internal/features/fulfilmentgroup/customer_share_stack_e2e_test.go
- services/vendor-service/internal/features/fulfilmentgroup/vendor_delivery_runtime_concurrency_integration_test.go
- services/vendor-service/migrations/000123_customer_grouping_backfill.down.sql
- services/vendor-service/migrations/000123_customer_grouping_backfill.up.sql
- shared/go/journeyshare/client_pool_test.go
- shared/go/journeyshare/journeysharetest/contracts.go
- shared/go/journeyshare/journeysharetest/stacke2e/stack_e2e_test.go

## Customer V4 §§19–21 closure — 2026-09-28 (local only)

**§19 SECTION 19: COMPLETE LOCALLY — PASS.** Verification Compliance stays the decision authority. Vendor's public trust projection (`internal/publictrust`) is bounded to 5 minutes, refreshed on completed-verification reconciliation, recovers a lost suspension callback, and removes or downgrades trust on expiry, suspension or rejection. Stale verification events are rejected and tenants are isolated. Food/Shop discovery DTOs carry trust only, never case documents or notes, and ratings are never shown as verification. Evidence: `tmp/customer1921-trust.log` (`TestCustomerV4PublicTrustLifecycle` and the Section 7 security matrix PASS against PostgreSQL).

**§20 SECTION 20: COMPLETE LOCALLY — PASS.** Route audit of every route each service actually registers (`tmp/customer1921-route-audit/`, gin registration overlay; `classify.py` resolves each route through `deploy/customer-public-ingress.locations.conf` and then the api-gateway table):

| Service | Routes | Service-credential (HMAC) | HMAC publicly reachable |
|---|---|---|---|
| customer-service | 52 | 8 | 0 |
| driver-taxi-service | 153 | 70 | 0 |
| driver-dispatch-delivery-service | 166 | 64 | 0 |
| driver-hauling-service | 129 | 55 | 0 |
| payment-wallet-service | 99 | 80 | 0 |
| vendor-service | 867 | 81 | 0 |
| notification-service | 10 | 6 | 0 |
| support-dispute-service | 89 | 22 | 0 |
| media-file-service | 21 | 11 | 0 |
| verification-compliance-service | 21 | 18 | 0 |
| api-gateway (own) | 47 | 24 | 0 |

- No `/internal/...`, HMAC or admin route is reachable through either public layer.
- Vendor keeps its direct `/api/v1/vendor/` and `/api/v1/commerce/` ingress. Every Vendor route the contract capture showed without middleware is mounted under `RequireAuth` (`cmd/main.go:1319`) or the bearer-guarded customer discovery group. Only vendor auth entry is intentionally open.
- Share public read (`/api/v1/customer/public/journey-shares/{token}`) is intentionally public: no access log, `no-store`, `no-referrer`, `noindex`, and it is rate limited by the owning service.
- Other open routes are intentional: auth entry, fare estimates, public catalogues, the rate-limited public rider profile, and support FAQs. The Paystack webhook verifies its signature in-handler, and the tracking and notification WebSockets authenticate in-handler.
- **Defect fixed:** `/api/v1/taxi/metrics` (booking counts, no auth by design, "network placement is the control") was publicly routed. `/api/v1/<svc>/metrics` and `/outbox-metrics` are now refused by the gateway `IsInternalPath` and the nginx include. Gateway edge tests were extended and pass.
- **Explained drift:** customer-service's own `/api/v1/customer/notifications` proxy is shadowed by Dispatch's equivalent customer-bearer proxy, which the gateway table names as owner. Partner-API routes belong to the B2B gateway host and are outside the customer vhost.
- Wrong caller, expired timestamp, bad signature and nonce replay (409) PASS (`shared/go/serviceauth`, `journeyshare` with Redis). Vendor/cart route/OpenAPI parity PASS.
- **STAGING PENDING:** deployed LB/DNS/TLS/network isolation (public-internet rejection of internal routes) is not claimed locally.

**§21 SECTION 21: COMPLETE LOCALLY — PASS.** Tips: customer `012`, taxi `021`, dispatch `000066`, hauling `026`, vendor `000129`, media `000005`, wallet `000018`, notification `008`, support `008`, verification `000003`.

| V4 row | Result |
|---|---|
| customer journey_share_sessions (NEW REQUIRED) | Implemented in `010_journey_share_sessions.sql`: `token_hash` only; unique active per (customer, authority, source_ref); indexes on expiry, customer+created_at, and authority+source_ref |
| customer movement product/grouping (VERIFY FIRST) | Implemented in `012_movement_commerce_product.sql` (renumbered from a duplicate `011`; this runner re-applies every file, so renaming is safe; re-applied twice on a fresh DB) |
| taxi customer location projection (NEW/VERIFY) | Not required: derived from existing `taxi_trip_location_points` |
| taxi movement outbox (VERIFY FIRST) | Reused (existing outbox/target routing) |
| dispatch secure handover (REUSE) | Reused; the earlier §16 gap repair `000066_receiver_secure_channel` is the only addition |
| hauling share-safe persistence (VERIFY FIRST) | Share read served from existing models; `025`/`026` are earlier-section additions |
| vendor (INTEGRATION FIRST) | `000122`–`000129` from §§9–18 only |
| media upload authorization (VERIFY/NEW) | Reused: `media_upload_authorizations` (`000002`) |

**Migrations added in §§19–21: none.**

*Register update (§22, 2026-09-28):* customer-service `013_movement_commerce_order_authority.sql` widens the projection CHECKs for the `VENDOR_COMMERCE` / `COMMERCE_ORDER` purchase row (proven gap; see §§22–26 below). Re-runnable; applied twice on a fresh database with no duplicate constraints.

## Customer V4 §§22–26 closure — 2026-09-28 (local only)

**SECTION 22: COMPLETE LOCALLY — PASS.**

*Envelope.* `shared/go/events.Envelope` plus `MovementSummary` carry the whole V4 logical envelope: `event_id` (UUID), bounded `event_name`, real `source_service`, UTC `occurred_at`, `source_entity_id` = `source_ref` (opaque), monotonic `source_version`, and in the payload `customer_id`, `authority`, `product_code`, `parent_context_ref`. The payload is the typed summary only, with no free-form data. `product_code` carries the execution variant, and Customer filters RIDE/SEND/TRUCK by authority family and FOOD/SHOP by Vendor product.

*Coverage gaps fixed.*
1. Fine lifecycle steps (en route, custody started, in transit, drop-off arrived, return) collapsed into coarse states and produced no event. There is now an optional bounded `stage` (15 plus 2 commerce values). Each authority reports only stages its own state machine records:
   - Taxi Ride and Taxi Goods: from status.
   - Dispatch: trip status, plus drop-off arrival from an issued DELIVERY handover challenge and RETURNING/RETURNED from `dispatch_return_workflows`.
   - Hauling: from status. Loaded = `picked_up`; drop-off arrival and delivery are one authority step.
   Mature event names are unchanged. A stage change is a new version.
2. Food/Shop purchases reached Activity only through executor missions. A vendor-owned delivery, or a purchase cancelled or refunded before any platform mission, never appeared. Vendor now publishes one `VENDOR_COMMERCE`/`COMMERCE_ORDER` purchase row per ParentOrder:
   - Published in the same transaction as creation and every parent state change, through the existing outbox and its own worker to the existing movement ingress.
   - `source_version` is the ParentOrder's own version.
   - Mission rows join it by `parent_context_ref`.
   - Gated by `VENDOR_CUSTOMER_MOVEMENT_GROUPING_ENABLED` (no new flag). Customer's caller map adds `vendor-service=VENDOR_COMMERCE`.
   - Reconciliation compares Customer's projected version (`movement-state?authority=VENDOR_COMMERCE`) and requeues current state without touching the order.

*Coverage.* RIDE, SEND (including drop-off and return), TRUCK and FOOD/SHOP (every ParentOrder state) have distinct published views, each covered by a per-authority test.

*Projection rules proven on real PostgreSQL:*
- Event-id dedupe: a replayed v5 is a DUPLICATE.
- Version guard: v4 arriving after v5 is STALE.
- Missing v7: reconciliation sees the projection behind and republishes.
- A deleted projection row is rebuilt without touching the authority.
- 20 concurrent random-order deliveries keep the newest.
- A purchase row groups with its missions.
- The Dispatch sweep publishes drop-off arrival without credential or receiver data.

Grouping replacement, replan, stale-executor refusal and partial or mixed outcomes are covered by the §14 lifecycle E2E, which passes with purchase rows enabled.

**SECTION 23: COMPLETE LOCALLY — PASS.**
- N01–N12 are all mounted and documented at the exact V4 paths. N12 is the §18 secure upload session, with Media's one-use `customer-uploads/{id}/content` redemption.
- Mechanized parity (`tmp/customer1921-route-audit/drift.py` over captured routes) for Customer, Taxi/Dispatch/Hauling customer and share families, Vendor commerce and Media customer upload: **0 undocumented, 0 phantom, 0 security mismatches**.
- The 8 apparent Dispatch phantoms were flag-gated routes. Re-capturing with those gates on mounts them (196 routes, still 0 HMAC exposure).
- 86 long-standing customer routes had no OpenAPI. They are now registered in `docs/openapi/customer-v4-route-register.openapi.json` for Customer, Taxi, Dispatch, Hauling and Media. Each is derived from the gin registration and handler source by `tmp/customer1921-route-audit/openapigen`: request DTO fields, query, headers and security. No fields are hand-authored and no `required` claims are made.
- 27 existing customer operations in 5 Vendor/Dispatch documents declared no security. They now declare `CustomerBearer`; staff and provider operations are unchanged.

**SECTION 24: COMPLETE LOCALLY — PASS.**

| Flag | Owner | Default | Enforcement proven |
|---|---|---|---|
| `CUSTOMER_SHARE_JOURNEY_ENABLED` | customer | false | startup validation; disabled gate test (PostgreSQL) |
| `TAXI_CUSTOMER_LIVE_LOCATION_ENABLED` | taxi | false | valid customer refused `503 no-store` (new test) |
| `CUSTOMER_MEDIA_UPLOAD_SESSION_ENABLED` | customer | false | **new**. Refused server-side with no upstream calls; startup fails closed naming (never echoing) missing Media/Support/secret config |
| `CUSTOMER_FOOD_ENABLED` / `CUSTOMER_SHOP_ENABLED` | vendor | false | domain gate plus kill switch tests |
| `EXTERNAL_REFERENCE_REPRICING_ENABLED` | vendor | false | refused at boot without an adapter |

Dispatch handover, evidence and QR dependencies fail closed at startup (observed: challenge without a rollout cohort, hub-hold without approved hubs). The Taxi Goods gate and Vendor credential requirements are enforced.

**SECTION 25: COMPLETE LOCALLY — PASS.**
- **IDOR, location minimization and HMAC**: pass on real PostgreSQL and Redis for Taxi (N07/N08, with load), Dispatch, Hauling and Vendor share. Also covered: Share, Media, Vendor order/change/substitution foreign-customer checks, and wrong-caller, expired, bad-signature and nonce-replay rejection.
- **Secret scan** (`secret_scan.py`, 537 distributable files): only development literals in `.env.example`, and client-restricted Firebase/Maps keys in mobile binaries. Finding for the app owner (not changed here, mobile out of scope): `apps/truck_provider` compiles a default media service token `development-media-token`. The Customer app is clean.
- **Share token**: entropy, digest-only storage, no-store/no-referrer/noindex headers, rotation and revocation tests pass.
- **OTP/QR**: excluded from movement payloads, notifications and challenge representations.
- **Media**: scope, single-use, forged, expired and purpose checks pass.
- **Payment integrity**: requests carry no amounts; approvals charge once, including under concurrency.
- **OCC and replay**: pass.
- **PII logging**: the canonical `logging.SafeFields` redactor now also redacts `account_number`, `nuban` and `bvn` without over-redacting account ids.

**SECTION 26: COMPLETE LOCALLY — PASS.**
- Share expiry sweep (bounded, observes completed sources, idempotent).
- Per-authority movement reconciliation (Taxi, shared `MovementReconciler` for Dispatch/Hauling, Vendor grouping plus purchase row).
- Vendor Wallet and refund unknown-outcome read-back, and mission read-back for Dispatch, Hauling and Taxi Goods, with JIT adoption.
- Notification retry with growing backoff, then exactly one dead-letter, with the business message kept (new test).
- Dispatch challenge expiry refuses stale credentials with no custody move, the sweep is idempotent, and re-issue works (new PostgreSQL test).
- Media orphan cleanup: leased maintenance expires grants and never-completed uploads, and deletion respects retention.

All reused; no new worker framework. The purchase-row worker and reconciler are bounded, keyset-scanned and flag-gated. Replica duplicates are harmless under Customer's inbox and version guard.

**Migrations added in §§22–26:** customer-service `013_movement_commerce_order_authority.sql` only.

**STAGING PENDING:** deployed network isolation, cloud secrets and allowlists, production SLOs, Maps/Firebase key restriction review, and the external Reference/Repricing adapter.
