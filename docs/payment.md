# Payments & Wallets

`payment-wallet-service` is the single source of truth for money on the platform —
wallets, a double-entry ledger, payment intents, Paystack charges, job escrow,
provider earnings, withdrawals, and refunds. No other service stores balances or
talks to Paystack directly; they hold/settle money by calling this service. All
amounts are **kobo integers** (1 NGN = 100 kobo). Default currency `NGN`.

API base path: `/api/v1/payment-wallet` (local port `8105`). Every response uses the
platform envelope `{ "success": true, "data": { } }`.

Full endpoint reference: [`../services/payment-wallet-service/README.md`](../services/payment-wallet-service/README.md).
Local setup + Postman collection: [`../services/payment-wallet-service/docs/README.md`](../services/payment-wallet-service/docs/README.md).

## Who consumes what

There are three consumer classes, each with its own auth. **Pick the row that
matches who you are**, then read that section below.

| Consumer | Auth | Surface |
|---|---|---|
| Customer app | Customer **bearer** token (`role=customer`, `service=customer`) | `/wallets/*`, `/topups` |
| Provider app (taxi/dispatch/hauling) | Provider **bearer** token (`service` = `taxi`\|`dispatch`\|`hauling`) | `/provider/*` |
| Backend service (taxi/dispatch/hauling/admin) | Service **HMAC** (`shared/go/serviceauth`) | `/internal/*` |
| Paystack | Webhook signature (`x-paystack-signature`) | `/webhooks/paystack` |

Apps hold a bearer token, never an HMAC secret — so apps never touch `/internal/*`.
Charging a customer for a booking and paying out a provider are driven by the
**owning booking service** over HMAC, not by the app.

## Customer app flow (bearer)

`customer_id` is always taken from the token subject — never sent in the body.

1. **Top up a wallet.** `POST /topups` with `{ amount_kobo, currency, customer_email,
   idempotency_key }` → returns a `PaymentIntent` with an `authorization_url`. Open
   that URL in a Paystack checkout (webview/browser).
2. **Confirm.** After checkout, either rely on the Paystack webhook (server-side
   credit) or call `POST /topups/:reference/verify` to force a verification.
3. **Read balance.** `GET /wallets/me` → `{ available_kobo, escrow_kobo, pending_kobo }`.
   `available_kobo` is spendable; `escrow_kobo` is held against active jobs;
   `pending_kobo` is reserved by in-flight withdrawals/refunds.
4. **History.** `GET /wallets/me/transactions?limit=50`. `side` is from the wallet
   owner's perspective (`credit` = inflow, `debit` = outflow).

The app does **not** debit the wallet for a booking itself — the booking service
does that over HMAC (see "Backend service flow"). The app only tops up and reads.

## Provider app flow (bearer)

`provider_type`/`provider_id` are derived from the token; never sent in the body.

1. **See earnings.** `GET /provider/earnings` → wallet summary (same shape as
   `/wallets/me`). Settled job payouts land here as `available_kobo`.
2. **Register a payout account (once).**
   - `POST /provider/bank-accounts/resolve` `{ account_number, bank_code }` →
     `{ account_name }` so the user can confirm the name before saving.
   - `POST /provider/bank-accounts` `{ bank_code, bank_name, account_number, currency }`
     → persists a `ProviderBankAccount` (creates a Paystack transfer recipient).
3. **Withdraw.** `POST /provider/withdrawals` `{ bank_account_id, amount_kobo,
   currency, idempotency_key }` → `Withdrawal`. Enforces min/max limits and available
   balance; if `PAYMENT_WALLET_REQUIRE_MANUAL_PAYOUTS=true`, it lands `pending` for
   admin approval before Paystack transfer.

## Backend service flow (HMAC) — the core money path

This is how a booking service (taxi/dispatch/**hauling**) charges a customer and pays
a provider. **Do not hand-roll HTTP** — use the shared client
[`shared/go/walletclient`](../shared/go/walletclient/walletclient.go), which signs
every request with `shared/go/serviceauth` (HMAC-SHA256 over
`METHOD\nrequestURI\ntimestamp\nsha256hex(body)`, headers `X-Service-Name` /
`X-Service-Timestamp` / `X-Service-Signature`).

### Wiring (per consuming service)

1. Add config for the wallet base URL + HMAC secret, e.g.
   `HAULING_WALLET_URL` / `HAULING_WALLET_SECRET`.
2. The secret must match an entry in payment-wallet-service's
   `PAYMENT_WALLET_SERVICE_SECRETS` (`servicename=secret,...`), keyed by the
   `ServiceName` the sender signs with (e.g. `hauling-service`).
3. Build the client once and reuse it:

```go
client := walletclient.Client{
    BaseURL:     cfg.WalletURL,        // http://payment-wallet-service:8105
    ServiceName: "hauling-service",    // must match PAYMENT_WALLET_SERVICE_SECRETS key
    Secret:      []byte(cfg.WalletSecret),
}
```

### The payment lifecycle

```
booking confirmed
        │  CreatePaymentIntent (payment_method = "wallet" | "paystack")
        ▼
  PaymentIntent (status: requires_payment)
        │  wallet:    PayFromWallet(intentID)   ── holds funds: customer_available → job_escrow
        │  paystack:  customer pays via authorization_url; webhook credits + holds
        ▼
  status: held  (funds in escrow, job in progress)
        │  CompleteJob(sourceService, sourceReference)  ── settles: job_escrow → provider_payable (− platform fee)
        ▼
  status: completed  (provider earnings now withdrawable)

  cancellation / dispute:
        │  RequestRefund(paymentReference, amount, reason)  ── job_escrow|provider → customer/Paystack
        ▼
  status: refunded | partially_refunded
```

Mapped to `walletclient` calls:

| Step | Call | Effect |
|---|---|---|
| Create the charge | `CreatePaymentIntent(ctx, PaymentIntentRequest{...})` | Creates intent for `(source_service, source_reference)`. `payment_method` = `wallet` or `paystack`. |
| Hold from wallet | `PayFromWallet(ctx, intentID, idempotencyKey)` | Moves `customer_available → job_escrow`. (Paystack intents hold via the webhook instead.) |
| Settle to provider | `CompleteJob(ctx, sourceService, sourceReference)` | Moves `job_escrow → provider_payable`, minus `PAYMENT_WALLET_PLATFORM_FEE_BPS`. |
| Refund | `RequestRefund(ctx, RefundRequest{...})` | Reverses to the customer / Paystack. |
| Inspect | `GetPayment(ctx, reference)` | Read current intent state. |

### Idempotency — non-negotiable

Every money-moving call carries an `idempotency_key`; replays return the original
result instead of double-charging. Make the key **deterministic** from the entity so
retries and goroutine re-runs dedupe — e.g. `"hauling:pay:" + bookingID`,
`"hauling:refund:" + bookingID`. `(source_service, source_reference)` also uniquely
identifies a payment intent, so `CreatePaymentIntent` for the same booking is safe to
retry.

### Don't roll back a booking on a wallet error — but money is different

Unlike notifications (fire-and-forget), the wallet call **is** the business outcome
for a paid booking: if `PayFromWallet` fails, the booking should not proceed as paid.
Surface the error to the caller and keep the booking in an unpaid/failed state — do
not silently swallow it. The idempotency key makes a later retry safe.

## Webhook flow (Paystack → service)

`POST /webhooks/paystack` is verified by **HMAC-SHA512 over the raw request body**
using the Paystack secret (`x-paystack-signature`). This route must stay
**un-proxied and un-rewritten** — any middleware that re-encodes the body breaks
verification. Events are de-duplicated by event key and processed idempotently, so
Paystack redeliveries are safe. `charge.success` credits the customer wallet (and
holds escrow for a booking intent); transfer/refund events advance withdrawal/refund
state.

## Canonical enums

**Payment status:** `pending`, `requires_payment`, `held`, `completed`, `refunded`,
`partially_refunded`, `failed`.

**Withdrawal status:** `pending`, `processing`, `requires_otp`, `paid`, `failed`,
`reversed`.

**Refund status:** `pending`, `processing`, `processed`, `failed`.

**Ledger account types:** `paystack_receivable`, `paystack_balance`,
`customer_available`, `job_escrow`, `provider_payable`, `withdrawal_pending`,
`refund_pending`, `platform_revenue`.

Source of truth: `services/payment-wallet-service/internal/features/wallets/models/models.go`.

## Notifications

Payment events emit notifications via `notification-service` (see
[`notifications.md`](notifications.md)) — `payment.topup_success`, `payment.success`,
`payment.failed`, `withdrawal.completed`, `withdrawal.failed`, `withdrawal.reversed`.
These are fire-and-forget and never block or roll back the financial operation.

## Adding a new consuming service (taxi / dispatch)

When `driver-taxi-service` / `driver-dispatch-delivery-service` gain bookings, wiring
them into payments is the same mechanical steps `driver-hauling-service` already uses:

1. **Config** — add `TAXI_WALLET_URL` + `TAXI_WALLET_SECRET` to the service config.
2. **Register the secret** — add the service to payment-wallet-service's
   `PAYMENT_WALLET_SERVICE_SECRETS`, e.g.
   `...,driver-taxi-service=development-payment-wallet-service-secret`, and set the
   same value as `TAXI_WALLET_SECRET` in the taxi `.env`.
3. **Client** — build a `walletclient.Client` with `ServiceName: "driver-taxi-service"`
   and inject it into the ride usecase.
4. **Emit at transitions** — call `CreatePaymentIntent` / `PayFromWallet` when a ride
   is confirmed/started, `CompleteJob` on completion, `RequestRefund` on cancellation,
   each with a deterministic idempotency key.
5. **Postman/docs** — clone the `Internal (HMAC)` requests and set `service_name` to the
   new sender; the pre-request signing script is unchanged.

Nothing in `shared/go/walletclient`, the HMAC signing, the ledger, or the Paystack
integration needs to change — you are only repeating the per-service wiring.
