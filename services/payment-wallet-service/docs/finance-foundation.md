# Financial foundation (WP7)

Payment Wallet is authoritative for organization wallets, provider earning accounts, double-entry journals, subscriptions/invoices/payments, commission rules and allocations, holds, settlements, refunds, reversals and statements. All amounts are signed 64-bit integer minor units paired with an ISO currency; no API accepts a client-calculated balance.

## Private API and security

Private finance routes are mounted below `/api/v1/payment-wallet/internal/finance` only when `PAYMENT_WALLET_FINANCE_PRIVATE_API_ENABLED=true`. Requests use the shared HMAC signature, timestamp freshness check, caller allowlist and a unique `X-Service-Nonce`. Commands carry payload version 1, organization, idempotency key, fingerprint inputs, correlation/causation IDs and expected version where state already exists. Finance RBAC is database-backed; settlement/refund approval additionally enforces versioned separation-of-duty policies. Private reads also require finance view permission and return safe projections rather than ledger accounts or balances.

The contract provides wallet and earning-account ensure/read, plan and commission-rule configuration, subscription invoice creation, authoritative payment confirmation, earning allocation, hold create/release, settlement and refund request/approval/read-back, operation replay, reconciliation and statement generation. Direct ledger mutation is not exposed.

## Ledger and workflows

Database constraints and append-only triggers enforce balanced same-currency journals and immutable posted journals/lines. Corrections use compensating reversals. Atomic wallet/earning projections are updated under row locks and expected versions. First-use account creation uses transaction advisory locks to avoid orphan accounts under concurrency.

Subscription invoices are blocked when a plan has no configured price. Allocations store the exact commission-rule version and split snapshot so the gross amount can be reproduced. Holds move balances atomically and unverified incident/dispute references require an expiry. Refund requests reference an original posted journal and enforce the cumulative refundable ceiling.

Every state change writes history, a redacted append-only audit entry and a versioned transactional-outbox envelope in the same commit.

## Workers and runbook

`PAYMENT_WALLET_FINANCE_WORKERS_ENABLED=false` by default. When enabled and the execution kill switch is off, maintenance and money workers use bounded batches, database leases, lease recovery, exponential backoff, retry limits, dead letter and operator replay. A payout/refund timeout is `UNKNOWN`; the next pass performs authoritative read-back using the same external reference and never initiates a duplicate. Successful status, journal, projection, history, audit and event commit atomically.

The independently flagged outbox worker (`PAYMENT_WALLET_FINANCE_OUTBOX_ENABLED`) delivers the same event ID and envelope to Fleet over HMAC HTTP, treating 2xx and duplicate 409 as success. Target URL and secret have no defaults. Configure batch size, interval, lease and max attempts with the corresponding `PAYMENT_WALLET_FINANCE_OUTBOX_*` variables.

Migration `000004_finance_foundation` is the complete additive schema and has a matching down migration. The PostgreSQL integration suite performs fresh apply, replay, rollback and reapply and refuses any database whose name is not explicitly test-specific.

## Durable reversal contract (WP8)

Migration `000006_reversal_contract_and_job_payments` adds `finance_reversals`, the authoritative aggregate for undoing a posted entry, and `finance_job_payments`, the authoritative record that a unit of dispatched work was paid for.

A reversal moves through `REQUESTED` → `PENDING_APPROVAL` → `APPROVED` → `PROCESSING` → (`UNKNOWN` → `RECONCILING`) → `COMPLETED`, with `FAILED_RETRYABLE`, `FAILED_PERMANENT`, `REJECTED` and `DEAD_LETTER` as the other terminal or recoverable states. The row records the reversal ID, organization, original journal, reason code, integer minor amount, ISO currency, request fingerprint, idempotency key, expected version, external operation reference, attempts, retry schedule, correlation and causation IDs, creating actor, approval reference with policy version, and the resulting compensating journal.

The original entry is immutable. Completion posts a compensating journal in the same transaction that marks the reversal complete, so a reversal can never claim success without the entry that makes it true. A full-value reversal is the exact mirror and takes the ledger's single-reversal index; a partial reversal is linked by causation and its lines are scaled by largest remainder within each side, which is what keeps debits and credits equal after rounding.

`LedgerRepository.ReversibleMinor` caps a request at the original's debit total less live refunds and live reversals. Rejected and permanently failed attempts release their claim. `PostCompensating` chooses the full or partial path and refuses anything above the original total.

Idempotency is per organization and key. An identical repeat returns the original row; a different fingerprint under the same key is a conflict. A cross-tenant original journal is refused before any state exists. Approval runs through the versioned `REVERSAL_APPROVAL` policy with self-approval disabled, so the requester cannot approve their own reversal.

Execution reuses the existing financial worker. A gateway timeout is persisted as `UNKNOWN` and the next pass reads the outcome back instead of executing again; an abandoned lease is recovered to `UNKNOWN` for the same reason. A compensating journal already posted under the reversal's idempotency key resolves as success, and `resulting_journal_id` is uniquely indexed, so a reversal cannot execute twice. Reconciliation raises `REVERSAL_UNKNOWN_OUTCOME` for reversals that stay ambiguous and `REVERSAL_COMPLETED_WITHOUT_JOURNAL` for a state that must never occur; it reports and does not repair.

Permissions added: `finance.reversal.request` (operator, admin), `finance.reversal.approve` (approver, admin) and `finance.job_payment.view` (viewer, operator, admin).

## Authoritative job payments (WP8)

`finance_job_payments` is searchable by dispatch request reference, canonical trip reference, partner booking reference and payment reference — identifiers other services already own. Each is uniquely indexed per organization, so a lookup resolves to one record or to none.

`GET /internal/finance/job-payments/lookup` requires `finance.job_payment.view` and returns the payment verification ID, organization, request/trip reference, integer minor amount, ISO currency, payment status, source version, paid timestamp, refund and reversal state, authoritative operation reference and a derived settled flag. It returns no balance, journal line, bank or card detail and no provider secret. Unknown reference, unsupported reference type and another tenant's record are indistinguishable `404`s.

`POST /internal/finance/job-payments` requires `finance.payment.confirm`, so a service that consumes payment verification can never mint one. The wallet payment flow records and advances these rows through the finance service; amount and currency are never rewritten, because a payment whose value changed is a different payment.
