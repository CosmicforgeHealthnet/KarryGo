# Taxi application verification — 15 October 2026

Core booking, parcel, wallet settlement/refund and server notification flows passed against real local HTTP services and PostgreSQL. This is **not full release certification**: Android end-to-end operation, external Paystack checkout/webhooks, and background push delivery remain unverified. An insufficient-funds error presentation defect was reproduced.

## Automated results

| Scope | Passing checks | Failed | Skipped tests |
|---|---:|---:|---:|
| Taxi backend | 587 | 0 | 0 |
| Payment/wallet backend, including bounded load | 219 | 0 | 0 |
| Customer backend | 91 | 0 | 0 |
| Notification backend | 19 | 0 | 0 |
| Driver Flutter tests | 19 | 0 | 0 |
| Customer Flutter tests | 73 | 0 | 0 |
| **Total** | **1,008** | **0** | **0** |

Go counts include named parent tests and subtests. Packages with no test files are excluded. Driver `flutter analyze` and `git diff --check` passed. The customer tests emitted mocked HTTP/image warnings but all final assertions passed.

Coverage includes authentication/session handling, matching/leases, availability/compliance, ride transitions, pickup/delivery verification, cancellations, scheduling, recovery, location telemetry, reviews, earnings, customer booking APIs/controllers, wallet models/withdrawal controllers, ledger integrity, refunds, HMAC/replay protection, notification retries/dead letters, and journey sharing/history. These suites mix real-database integration tests and unit tests with fakes; they do not all call external providers.

## Actual local HTTP walkthroughs

Temporary taxi :8202, wallet :8205 and notification :8206 used separate databases and Redis namespaces. Customer tokens were synthetic, with test-only signing secrets; the existing driver's production/local-development trip was not modified. A balanced synthetic ₦100,000 wallet deposit funded the scenarios. No real money was charged.

- Fare estimate returned ₦978.50 for the sample Comfort ride, QA Yaba to QA Lagos destination.
- Creation returned 201; an identical idempotency-key retry returned the same booking. Changing the payload under that key returned 409.
- A real online seeded driver received the offer. Acceptance, heading to pickup and arrival returned 200.
- Starting before pickup verification returned 409; a wrong pickup code was refused; the correct code unlocked start.
- Both customer and driver cancellation during an ongoing trip returned 409.
- Completion returned 200, booking became `completed`/`paid`, and the wallet intent became `completed` for 97,850 kobo.
- Wallet balance moved from 10,000,000 to 9,902,150 kobo, exactly the fare. Repeat completion returned 409. All ledger transactions balanced.
- Cancelling another accepted ride restored the pre-booking wallet balance exactly.
- Parcel creation preserved receiver, description and fragile fields. Completion before delivery verification returned 409; incorrect handover code returned 400; correct code and subsequent completion returned 200 with payment marked paid.
- Offer rejection returned 200; the customer could then cancel the unassigned request.
- An unfunded customer could create a request, consistent with the design charging at driver acceptance. Acceptance was blocked, no debit was posted, and the booking ultimately became `unmatched`/`unpaid`. **The error returned to the driver was misleading; see below.**

### Notifications

Ride lifecycle rows reached the notification outbox and were acknowledged. The synthetic customer's lifecycle/payment/refund messages had in-app delivery records marked sent. Push was skipped because these test recipients had no registered devices.

A separate real WebSocket client completed HTTP 101 upgrade, received `qa.realtime.test`, and verified two sends with the same idempotency key returned one message ID. A server-side `websocket=sent` record alone was not used as proof of client receipt.

The isolated run explicitly enabled `TAXI_RECOVERY_NOTIFICATIONS_ENABLED=true` and disabled external email/push senders. That flag is absent in the existing local taxi .env and defaults false; the current local durable notification publisher therefore does not have the same configuration as this successful test. Notification aggregate message status remains `queued` by the service's documented v1 behavior; per-channel delivery records carry delivery status.

## Findings and unverified areas

1. **Insufficient funds shown as service unavailable.** Wallet returned 409 on `pay-from-wallet`; Taxi returned 503 with generic “Something went wrong. Please try again.” Booking acceptance remained blocked and no money moved. Reproduction: create a wallet booking for an unfunded customer, allow matching, accept as the driver. `internal/features/booking/usecases/matching.go:528` wraps the hold error as unavailable. Preserve a meaningful domain error while keeping transport failures distinct.
2. **Driver background push is not implemented in this client.** No `firebase_messaging` dependency/token registration was found in `apps/taxi_provider`; offers use foreground polling. Firebase server credentials exist locally, but that does not establish device delivery. Permission handling, registration, foreground/background/killed-app notifications and notification taps need implementation/device validation.
3. **External Paystack unavailable from this test environment.** The configured key has the `sk_test_` prefix. A test initialization attempt returned non-JSON; a read-only bank API probe returned HTTP 403, `text/plain`, `error code: 1010`. This does not prove the key is invalid. Checkout success, external callback/webhook arrival, bank payout and provider-side reversal are not certified. Local wallet settlement and refund behavior passed independently.
4. **Email unconfigured.** Existing notification configuration selects SMTP but host/password are empty. No real email or SMS receipt was tested.
5. **Native Android walkthrough incomplete.** Wireless device remained connected and driver process was running. The signed-out dark login screen rendered; a login-start request from the device reached the isolated API with 201. The foreground switched to another app before OTP completion, so further interaction stopped. The original `8102 → 8102` ADB tunnel was restored. Native booking completion, camera QR scanning, background location, offline/reconnect, and process-death recovery were not exercised in this run. API and widget results do not substitute for these checks.
6. Maps routing, iOS, production traffic scale, and unrelated non-taxi products were outside this run. Payment load numbers below are local database benchmarks, not a production capacity claim.

## Test repairs made

Only test files were changed during this verification; no production payment/booking behavior was changed.

- Taxi migration-020 rollback test now uses a transaction and rolls back its DDL, preserving newer notification rules for subsequent tests. The earlier run had overwritten migration 021's function.
- Taxi movement reconciliation test starts its cursor at the test's database timestamp, excluding pre-existing fixtures from other packages.
- Wallet notification-outbox test applies owning migrations so it also works on a fresh database rather than depending on another package running first.
- Customer onboarding assertion now checks the existing `Book a Taxi` label instead of obsolete `Car Ride`.

Initial failures were retained in temporary logs; the table reports final reruns. The taxi test database's function was restored before the final full-suite run.

## Bounded payment load

- Settlement posting: 400 requests, concurrency 16, 0 errors; measured 1,171.5 requests/s, p95 29.885 ms, p99 42.035 ms.
- Duplicate settlement storm: 400 requests, concurrency 16, 0 errors; p95 4.626 ms, p99 13.906 ms.

## Reproduction and evidence

Logs and the synthetic HTTP scripts are under `/private/tmp/karryam-qa-1015/`. These are temporary local evidence, not committed credentials or release artifacts. Machine-readable final counts are in `2026-10-15-test-summary.json` beside this report.

Use disposable databases only: payment integration tests truncate their fixture tables. Initialize taxi/customer owning migrations before their full suites. For backend suites, run `go test -json -p 1 -count=1 ./...` from each service with:

- Taxi: `TAXI_TEST_DATABASE_URL`, `TAXI_TEST_REDIS_ADDR`, `JOURNEY_TEST_REDIS_ADDR`.
- Payment: `PAYMENT_WALLET_TEST_DATABASE_URL`, `WALLET_TEST_DATABASE_URL`, `WALLET_LOAD_TEST=1`.
- Customer: `CUSTOMER_TEST_DATABASE_URL`, `CLOSURE_MIGRATION_DATABASE_URL`, `JOURNEY_TEST_REDIS_ADDR`.
- Notification: `NOTIFICATION_TEST_DATABASE_URL`.

Run `flutter test --reporter json` in both `apps/taxi_provider` and `apps/customer`; run `flutter analyze` in `apps/taxi_provider`.

Test databases are named `karryam_*_test_1015` and `karryam_*_e2e_test_1015` on local PostgreSQL :5434. They are retained for inspection. Normal app services on :8102/:8105/:8106 remain running; temporary API processes and test Redis are stopped after verification.

## Follow-up fixes (same day)

**Finding 1 — fixed.** `AcceptBooking` now classifies a failed escrow hold (`holdFailureError` in `services/driver-taxi-service/internal/features/booking/usecases/payments.go`). A definite Wallet refusal (a 4xx `walletclient.RequestError`, e.g. insufficient balance) returns **409 `payment_failed`** with "The passenger's wallet payment was declined, so this ride can't be accepted." It is not retryable. Transport failures, Wallet 5xx, 408 and 429 stay a retryable 503. Before the fix, `apperrors.From` converted every non-`apperrors` error to the generic "Something went wrong" message, so the classification was never reached. The booking stays `awaiting_acceptance`/`unpaid` and no refund is issued. New tests: two unit tests, plus `TestAcceptBooking_WalletRefusalReachesTheDriverAsPaymentFailed` against `karryam_taxi_test_1015`. The driver app already shows the server's `message`. The taxi process on :8102 still runs the old binary, so restart it to pick the fix up.

**Finding 2 — fixed for Android.** The taxi driver app was never registered in Firebase. It is now registered as `com.cosmicforge.logistics.taxiprovider` (`1:461709902538:android:d923f7811bf94fdf876a33`), and `google-services.json`, a hand-maintained `lib/firebase_options.dart` and the google-services gradle plugin are in place. `firebase_core`/`firebase_messaging` match truck_provider. `FirebasePush` (`lib/features/notifications/data/firebase_push.dart`) does nothing when Firebase is unavailable. The app shell registers the device after sign-in (`ProviderApi.registerDevice` → `POST /api/v1/taxi/provider/notifications/devices`) and again on token rotation. A foreground push, or tapping a tray notification, refreshes offers immediately. The backend already sent `ride.matched` with a push channel, so no server change was needed. On-device check (JDY-LX2): the Android 13 permission prompt appeared after sign-in, the `notification_devices` row was created for the driver, a signed `ride.matched` sent with the app backgrounded showed "New trip request" in the tray, and tapping it opened the app. Not covered: iOS (not registered, no APNs), killed-app delivery after a force-stop, and a push produced by a real match rather than a direct send. `flutter analyze` is clean and `flutter test` passes 20/20 (including a new registration contract test).

Still open: findings 3–5 (Paystack reachability, SMTP config, full native walkthrough). Finding 1 has a related follow-up: after a definite refusal the matcher still offers the ride to further drivers until the search window ends, and each of them hits the same 409. Ending the search early with a `payment_failed` outcome for the passenger would be a behaviour change and needs a product decision.
