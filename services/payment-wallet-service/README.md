# Payment & Wallet Service

Owns wallets, payments, refunds, provider earnings, withdrawals, and fleet
settlement. This service should stay isolated because it handles financial data.

All money values are **kobo integers** (1 NGN = 100 kobo). Default currency is `NGN`.

API base path: `/api/v1/payment-wallet` (local port `8105`).

Every response uses the platform envelope:

```json
{ "success": true, "data": { } }
```

```json
{ "success": false, "error": { "code": "validation_failed", "message": "…", "request_id": "…", "fields": [] } }
```

---

## Who calls what

| Caller | Auth mode | Endpoints |
|--------|-----------|-----------|
| Customer app | Customer bearer token (`role=customer`, `service=customer`) | `/wallets/*`, `/topups` |
| Provider apps (taxi / dispatch / hauling) | Provider bearer token (`service` = `taxi`\|`dispatch`\|`hauling`) | `/provider/*` |
| Other backend services | HMAC service auth (`shared/go/serviceauth`) | `/internal/*` |
| Paystack | Webhook signature (`x-paystack-signature`) | `/webhooks/paystack` |

Provider bearer auth accepts **any non-empty role** as long as the token verifies
against the configured per-service secret and `claims.service` matches one of
`taxi` / `dispatch` / `hauling`. Provider apps use service-specific role strings
(e.g. hauling mints `role=truck_provider`). The signing secret + service binding
are the real gate; see `internal/features/wallets/http/auth.go`.

> **Secret alignment:** each provider service must sign its access tokens with the
> same secret the wallet service has under `PAYMENT_WALLET_PROVIDER_ACCESS_TOKEN_SECRETS`
> for that service key. Mismatched secrets → `403` on every provider call. The dev
> default for `hauling` mirrors `HAULING_PROVIDER_TOKEN_SECRET`.

---

## Customer endpoints (customer bearer)

`customer_id` is always taken from the token subject — never the body.

### `GET /wallets/me`
Wallet summary for the authenticated customer.
```json
{ "owner_type": "customer", "owner_id": "…", "currency": "NGN",
  "available_kobo": 250000, "escrow_kobo": 0, "pending_kobo": 0 }
```
`available_kobo` is spendable; `escrow_kobo` is held against active jobs;
`pending_kobo` is reserved by in-flight withdrawals/refunds/settlements.

### `GET /wallets/me/transactions?limit=50`
```json
{ "transactions": [
  { "reference": "…", "transaction_type": "paystack_charge_success",
    "side": "credit", "amount_kobo": 100000, "currency": "NGN",
    "memo": "Wallet top-up", "created_at": "2026-06-21T10:00:00Z" } ] }
```
`side` is from the wallet owner's perspective (`credit` = inflow, `debit` = outflow).

### `POST /topups`
Body: `{ "amount_kobo": 100000, "currency": "NGN", "customer_email": "…", "idempotency_key": "…" }`
Returns a `PaymentIntent` with `authorization_url` for the Paystack checkout. `201`.

---

## Provider endpoints (provider bearer)

`provider_type`/`provider_id` are derived from the token; never sent in the body.

- `GET /provider/earnings` → wallet summary (same shape as `/wallets/me`).
- `POST /provider/bank-accounts/resolve` — body `{ account_number, bank_code }` → `{ account_number, account_name, bank_id }`.
- `POST /provider/bank-accounts` — body `{ bank_code, bank_name, account_number, currency }` → `ProviderBankAccount`. `201`.
- `POST /provider/withdrawals` — body `{ bank_account_id, amount_kobo, currency, idempotency_key }` → `Withdrawal`. `201`. Enforces min/max withdrawal limits and Paystack balance.

---

## Internal endpoints (HMAC service auth)

Use the shared client in `shared/go/walletclient` — do not hand-roll requests.
All money-moving calls require an `idempotency_key`; replays return the original result.

Commerce allocation refunds are component commands, not arbitrary gross
amounts. They preserve customer, Platform and other-approved funding outputs;
identify escrow or released-beneficiary recovery; and include frozen
merchandise/delivery/revenue/authorization reductions. Wallet validates
conservation and cumulative allocation-local caps before posting one balanced
ledger transaction. An insufficient released-party balance returns
`PENDING_LIABILITY` without claiming that a refund completed.

- `POST /internal/payment-intents` — `walletclient.CreatePaymentIntent`. Body is `PaymentIntentRequest` (`source_service`, `source_reference`, `customer_id`, `amount_kobo`, `payment_method` = `wallet`\|`paystack`, `idempotency_key`, …). Returns `PaymentIntent`.
- `POST /internal/payment-intents/:id/pay-from-wallet` — `walletclient.PayFromWallet`. Holds funds from the customer wallet into job escrow.
- `POST /internal/jobs/:source_service/:source_reference/complete` — `walletclient.CompleteJob`. Settles escrow to the provider after job completion.
- `POST /internal/refunds` — `walletclient.RequestRefund`. Body `RefundRequest`.
- `GET /internal/payments/:reference` — `walletclient.GetPayment`.

### Vendor bank onboarding (Section 7 only)

These additive HMAC-v2 routes accept only the exact `vendor-service` caller.
They resolve/register a `vendor_business` bank subject in Wallet's existing
Paystack/provider-bank authority; they do not create a Vendor ledger,
settlement, withdrawal, or commerce-money surface.

- `POST /internal/vendor-bank-profiles/resolve` validates a supplied account and
  returns only bank code, masked account, and resolved account name.
- `PUT /internal/vendor-bank-profiles/:businessId` idempotently registers or
  selects the matching account as default. `Idempotency-Key` is mandatory.
- `GET /internal/vendor-bank-profiles/:businessId` performs authoritative
  read-back after an unknown mutation outcome.

Responses never contain the full account number, recipient code, storage
credential, or Paystack secret. The Vendor-facing projection is
`external_ref`, `bank_code`, `masked_account`, `resolved_account_name`,
`source_version`, `status=VERIFIED`, and `observed_at`. See
`docs/openapi/vendor-bank-onboarding.openapi.json`.

Example caller:
```go
client := walletclient.Client{
    BaseURL:     "http://payment-wallet-service:8105",
    ServiceName: "taxi-service",
    Secret:      []byte(os.Getenv("WALLET_SERVICE_SECRET")),
}
intent, err := client.CreatePaymentIntent(ctx, walletclient.PaymentIntentRequest{ /* … */ })
```

---

## Webhook

`POST /webhooks/paystack` — verified by HMAC-SHA512 over the **raw request body**
using the Paystack secret. This route must stay **un-proxied and un-rewritten**:
any middleware that re-encodes the body will break signature verification. Events
are de-duplicated by event key and processed idempotently.

---

## Canonical enums

**Ledger account types:** `paystack_receivable`, `paystack_balance`,
`customer_available`, `job_escrow`, `provider_payable`, `withdrawal_pending`,
`refund_pending`, `platform_revenue`.

**Payment status:** `pending`, `requires_payment`, `held`, `completed`,
`refunded`, `partially_refunded`, `failed`.

**Withdrawal status:** `pending`, `processing`, `requires_otp`, `paid`, `failed`,
`reversed`.

**Refund status:** `pending`, `processing`, `processed`, `failed`.

Source of truth: `internal/features/wallets/models/models.go`.
```

> Feature folders other than `wallets/` (`payments`, `provider_earnings`,
> `refunds`, `settlements`, `withdrawals`) are scaffolding only. The live
> implementation for all of the above currently lives under `wallets/`.

## V4 Section 15/16 customer integration (local)

Wallet remains the sole authority for payment, refund, settlement, balance and
receipt state. Vendor Commerce binds approved fingerprints and deterministic
operation references, reads back ambiguous payment/refund outcomes, and keeps
VendorOrder and platform allocations isolated. Targeted PostgreSQL tests cover
concurrent/idempotent payment, multi-vendor and partial/full refunds, recovery,
and Customer Wallet behavior. No external repricing formula was introduced.

Migration `000018_customer_notification_outbox` writes reference-only payment,
refund and withdrawal notification intents in the same transaction as future
Wallet state transitions. The worker retries Notification outages with the
same Notification idempotency key and dead-letters after bounded attempts;
financial state is never rolled back for notification failure. Existing direct
notifier calls remain a low-latency hint and no longer include amount or
failure reason in payloads. The Notification service's production templates
are in migration `007_customer_wallet_templates.sql`. Deployment must configure
`PAYMENT_WALLET_NOTIFICATION_URL` and `PAYMENT_WALLET_NOTIFICATION_SECRET` to
drain the outbox; without them, committed intents remain pending.
