# referral-service

The referral programme's rules engine: every customer and driver gets a code,
sign-ups with a code become referrals, and paid jobs earn the referrer rewards
while the referrer is active. It **never holds money** — it decides, and asks
`payment-wallet-service` to credit or take back referral balances.

- HTTP: `:8114`, API base `/api/v1/referrals`, internal routes under
  `/internal/referral/v1`.
- Local Postgres `5450` (`referral_service`), optional Redis `6391`.
- Module `cosmicforge/logistics/services/referral-service`.

## How it learns things

It never reads another service's database. Each service announces facts
through a transactional outbox and a signed relay (`shared/go/outbox`,
`shared/go/referralcontract`):

| Event | Producer | Meaning |
|---|---|---|
| `identity.customer.registered.v1` | customer-service | A customer account was created (or backfilled), with hashed phone/email/device and any referral code. |
| `identity.provider.registered.v1` | hauling, dispatch, taxi | The same for a driver account. |
| `identity.provider.onboarding_completed.v1` | hauling, dispatch, taxi | A driver became fully approved (approval sweep in each service). |
| `finance.job.settled.v1` | payment-wallet-service | A ride, delivery or haul was paid out, with its platform fee and completion time. |
| `finance.job.settlement_reversed.v1` | payment-wallet-service | A paid-out job was refunded or reversed. **No producer yet**: the wallet cannot refund after payout today. |

Each event type is pinned to the service that owns the fact (a taxi service
cannot report a hauling sign-up or a paid job). Delivery is at-least-once;
every effect is keyed by business identity (job, referral, person), never by
event id, and is deduplicated three times: the inbox, the reward ledger's
unique idempotency key, and the wallet's own key.

Identifiers arrive only as keyed hashes (`REFERRAL_IDENTITY_PEPPER`, the same
in every producer). Accounts with the same phone (then email) hash are one
person with one code.

## The rules, briefly

- The reward follows the **referred** person's account type.
- **Customer referral:** the referred customer gets a ₦200 welcome credit and a
  first-booking credit (spend-only). Their first paid booking qualifies the
  referral; the referrer earns ₦500 if they were active when it happened.
- **Milestones:** the referrer's 5th qualified customer referral pays a
  free-delivery credit (spend-only), the 10th pays ₦7,500 and the Power User
  badge. Each is granted once, ever; only referrals in good standing count.
  The money is activity-gated like any referral income; the badge is not.
- **Driver referral:** commission on the driver's first 5 paid jobs is held;
  once the driver is approved and has done 5 jobs, the referral qualifies, the
  held commission is released, the driver gets a ₦500 welcome credit, and a
  90-day window opens. Jobs *finished* inside the window pay the referrer 10%
  of the platform's fee.
- **Active referrer:** 5 paid bookings (as a customer) or 10 paid jobs (across
  driver accounts) in the 30 days before the job finished. Judged per job, an
  hour after it arrives (late events count). Inactive means that reward is
  lost, not deferred.
- **Refunds:** a refunded job takes back exactly what it earned (a partial
  refund takes back the commission on the refunded part of the fee). A fully
  refunded qualifying booking un-qualifies the customer referral until the
  next genuine booking; a qualified driver track keeps its window. Take-backs
  reach the wallet only after the credit they undo, and may push a balance
  below zero. Every 3rd reversed job on one referral puts it under review.
- **Fraud checks:** self-referral, a second code for someone already referred,
  phone/email conflicts, a sign-up on the referrer's device (or on the device
  of another person they referred — the referral is held), a job between the
  referral's two people (earns nothing), and a bank account used by more than
  one person (cash-outs stop until reviewed).

All amounts and thresholds are settings (see `.env.example`).

## HTTP

| Route | Who | Purpose |
|---|---|---|
| `GET /api/v1/referrals/codes/:code` | anyone (rate limited) | Is this code valid? Sign-up screens only; reveals nothing about the owner. |
| `GET /api/v1/referrals/me` | a customer or driver login token | My code, share link, standing, qualified referrals and badge. |
| `POST /internal/referral/v1/events` | the producers above, HMAC-signed | Receive relayed events. |
| `POST /internal/referral/v1/withdrawal-eligibility` | payment-wallet-service | May this driver cash out referral earnings now? Records the destination bank account's hash. |
| `GET /internal/referral/v1/activity/:actorType/:actorId` | admin callers | Paid jobs in the activity window. |
| `GET /internal/referral/v1/admin/accounts/:actorType/:actorId` | admin callers | Everything about the person behind an account. |
| `GET /internal/referral/v1/admin/flags?status=open` | admin callers | Review flags. |
| `POST /internal/referral/v1/admin/flags/:id/resolve` | admin callers | `{"outcome": "cleared" \| "upheld"}` |
| `POST /internal/referral/v1/admin/persons/:id/{freeze,clear,block}` | admin callers | Hold, release or remove a person. |
| `POST /internal/referral/v1/admin/referrals/:id/{freeze,clear,disqualify}` | admin callers | Hold, release or invalidate a referral. |
| `POST /internal/referral/v1/admin/rewards/:id/reverse` | admin callers | Take back one reward. |
| `GET /internal/referral/v1/admin/reconciliation` | admin callers | Accounts whose postings disagree with the wallet. |

Every admin action needs `performed_by` and `reason` in its JSON body, is
recorded in `referral_admin_actions`, and never moves money directly.
`disqualify` and `block` accept `"reverse_paid": true` to also take back
rewards already paid. All internal routes require an HMAC service signature
with a fresh nonce.

## Background workers

| Worker | Default interval | Job |
|---|---|---|
| reward-decisions | 30 s | Decide "was the referrer active?" once each reward's hour is up. |
| wallet-posting | 10 s | Post approved rewards and take-backs to the wallet (retries with backoff). |
| window-expiry | 1 h | Mark ended 90-day driver windows. |
| wallet-reconciliation | 24 h | Compare every account's postings with the wallet's ledger. |
| nonce-prune | 10 min | Forget expired request nonces. |

All state is in Postgres; any number of replicas can run them.

## Running locally

```bash
bash scripts/bootstrap_services/referral-local-bootstrap.sh
```

```bash
cd services/referral-service && go run ./cmd
```

Copy `.env.example` to `.env` first and fill in the secrets. Producers switch
their feeds on with their own `*_REFERRAL_FEED_ENABLED` (or, for the wallet,
`PAYMENT_WALLET_REFERRAL_OUTBOX_ENABLED`) settings, and existing accounts get
codes by running each service's `go run ./cmd/referral-backfill` once.

## Tests

The PostgreSQL suite skips unless `REFERRAL_TEST_DATABASE_URL` names a
database whose name contains `test`. It truncates every table, and packages
share one database, so run it serially:

```bash
REFERRAL_TEST_DATABASE_URL='postgres://user:pass@localhost:5432/referral_test?sslmode=disable' go test -p 1 ./...
```

## Not built yet

- App screens (referral dashboard, sharing, wallet display), leaderboards and
  prize campaigns, and an admin console (the admin API is here).
- A producer for `finance.job.settlement_reversed.v1`.
- Customer cash-out of referral earnings (needs customer identity checks).
- Vendor shop orders do not count as paid jobs.
