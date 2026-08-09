# Company wallets and provider sub-wallets (Section 13)

## Ownership

`payment-wallet-service` is the sole authority for balances, journals, holds
and execution. `fleet-partner-service` owns references, instructions, policy,
approval workflow and audit.

Fleet has **no ledger**, **no authoritative balance**, and **no accounts
receivable**. Every balance-shaped value Fleet holds is a projection that
carries a freshness state, and no projection is ever sufficient on its own to
authorize money movement. `TestFleetOwnsNoLocalLedgerOrBalance` asserts this
against the live schema, so a future migration that adds a balance column fails
the suite rather than passing review.

| Concern | Owner |
| --- | --- |
| Wallet accounts, balances, journals, holds | payment-wallet-service |
| Provider sub-wallet (earning account) | payment-wallet-service |
| Allocation and withdrawal execution | payment-wallet-service |
| Wallet references and safe projections | fleet-partner-service |
| Allocation instructions and business reasons | fleet-partner-service |
| Withdrawal policy and company approval | fleet-partner-service |
| Bank account details | payout provider — neither service stores them |

## Provider sub-wallets reuse the earning account

Section 13 does **not** introduce a second provider financial account. The
existing `finance_earning_accounts` row already carries the currency, status,
ledger link and version a sub-wallet needs. Migration
`000007_provider_subwallet_operations` adds the corporate ownership Section 13
requires — organization wallet, provider identity, organization membership and
service category — to that same row, with a partial unique index enforcing one
active account per membership, service category and currency.

Two provider account models would mean two answers to "how much is owed to this
provider", and the balance would follow whichever one a given code path
happened to read.

## Link states

Fleet's link state is separate from the authoritative account status:

```
NOT_REQUESTED → REQUESTED → PROVISIONING → ACTIVE
                                  ↓
     SUSPENDED · RESTRICTED · CLOSED · DISABLED
                                  ↓
        UNKNOWN · STALE · RECONCILING  (ask again)
                                  ↓
      FAILED_RETRYABLE · FAILED_PERMANENT
```

`UNKNOWN`, `STALE` and `RECONCILING` are deliberately distinct from the
`FAILED_*` states. They mean *we do not currently know*, and every sensitive
gate treats not knowing as a refusal. An ACTIVE link whose local copy is not
`CURRENT` authorizes nothing: the account may have been restricted since it was
copied, and the freshness state exists precisely because Fleet cannot tell.

Before refusing, the gate attempts one authoritative read-back
(`requireOperationalWallet`, `requireOperationalSubWallet`), so a merely stale
copy is repaired rather than blocking the operation permanently. When the
read-back also fails, the caller gets `wallet_projection_stale` (503) — "ask
again", not "you may not".

## Settlement separation

```
Customer payment
    ↓
KarryGo marketplace settlement
    ↓
Organization master wallet        ← the customer's money stops here
    ↓
Company-authorized provider allocation   ← a separate, later decision
    ↓
Provider sub-wallet
```

A customer settlement credits the organization wallet and never posts to a
provider sub-wallet. A provider allocation is a distinct Wallet operation with
its own journal type (`PROVIDER_ALLOCATION`), its own idempotency key and its
own approval trail.

This is not a naming convention — it is what makes the provider statement safe.
If the two were one movement, a provider could read their own statement and
reconstruct what the customer paid and what the company kept.

`TestProviderAllocationIsASeparateMovementFromSettlement` asserts on the
postings: the settlement journal must contain zero lines against any provider
ledger account.

## Reads

| Route | Permission |
| --- | --- |
| `GET /fleet-partner/v1/wallet` | `wallet.read` |
| `GET /fleet-partner/v1/wallet/transactions` | `wallet.transactions.read` |
| `GET /fleet-partner/v1/wallet/transactions/{transactionId}` | `wallet.transactions.read` |
| `GET /fleet-partner/v1/wallet/policy` | `wallet.read` |
| `PUT /fleet-partner/v1/wallet/policy` | `wallet.policies.manage` |
| `GET /fleet-partner/v1/providers/{membershipId}/sub-wallet` | `wallet.provider_subwallet.read` |
| `GET /fleet-partner/v1/providers/{membershipId}/sub-wallet/statement` | `wallet.provider_subwallet.read` |

`GET /wallet?authoritative=true` asks payment-wallet-service directly and
refreshes the link. The default is the local projection, which reports its own
freshness rather than pretending to be current.

Balances are pointers in the response DTO. When the authoritative read fails
the field is **absent**, not zero: a zero balance is a claim, and Fleet is not
entitled to make it.

`fleet_partner_wallet_transaction_projections` is display-only. Nothing sums
it. A balance is asked of payment-wallet-service, never reconstructed from
these rows, because a projection missing one event produces a number that looks
exactly as authoritative as a correct one.

## Configuration

| Variable | Default | Effect |
| --- | --- | --- |
| `ORGANIZATION_WALLET_READ_ENABLED` | `false` | Wallet read surface |
| `PROVIDER_SUBWALLET_ENABLED` | `false` | Sub-wallet reads and statements |
| `PROVIDER_ALLOCATION_ENABLED` | `false` | Allocation writes and execution |
| `PROVIDER_WITHDRAWAL_ENABLED` | `false` | Withdrawal writes and execution |
| `WALLET_PROJECTION_REFRESH_ENABLED` | `false` | Projection refresh worker |
| `WALLET_RECONCILIATION_ENABLED` | `false` | Reconciliation worker |
| `FLEET_COMPANY_WALLET_KILL_SWITCH` | `false` | Overrides every write and worker |
| `FLEET_COMPANY_WALLET_STALE_SECONDS` | `900` | Projection staleness window |

Every write and every worker defaults false. Startup validation refuses a
configuration where a write flag is on without a configured Wallet client, and
refuses `PROVIDER_ALLOCATION_ENABLED` or `PROVIDER_WITHDRAWAL_ENABLED` without
`PROVIDER_SUBWALLET_ENABLED` — a write surface whose result cannot be read is
incoherent.

The global `FLEET_INTEGRATIONS_KILL_SWITCH` and the feature kill switch both
override every flag. That is what makes them usable during an incident.

## Section 15

Section 15 service-catalogue, order, shipment, package, and manifest aggregates
now exist in Fleet. The wallet allocation business-reference type constraint
still accepts only `TRIP`, `ASSIGNMENT`, `PAYROLL_REFERENCE`,
`EXPENSE_REFERENCE` and `MANUAL_APPROVED_REFERENCE`. `PRIVATE_ORDER` and
`SHIPMENT` remain refused by the wallet service and its database CHECK; Section
15 does not silently broaden financial authority.

Taxi remains disabled and fail-closed. Hauling remains production-backed and
was not changed by this work package.
