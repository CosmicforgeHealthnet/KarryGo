# Secure Admin Section 13 — Wallet implementation evidence

**Software status: `SECTION_13_BACKEND_FUNCTIONALLY_COMPLETE`.** This covers the Wallet-owned backend boundary on `staging`. It does not activate Headquarters access, money-moving workers, provider integrations, production policies, or any deployment.

## Implemented owner behavior

- The payment-wallet service exposes fixed, minimized, source-backed reads for customer accounts and integer-kobo balances; payment intents; customer refunds; provider payouts and webhook processing posture; organization wallets; provider earning accounts; ledger accounts, balances, journals and lines; holds; subscriptions, invoices and payments; commission rules and earning allocations; commerce payment snapshots, per-vendor/platform allocations and durable financial operations (including truthful `PENDING_LIABILITY`); settlement batches/items; native refunds and reversals; provider allocations/withdrawals; managed job payments; reconciliation runs/findings; statements; immutable financial audit records; and financial investigation cases.
- Bank account numbers, recipient/provider tokens, webhook bodies, request fingerprints, raw metadata, free-form financial narratives, actor identities, network metadata and external error bodies are outside the allowlist. References are opaque owner identifiers; money is always an integer minor unit with an explicit currency.
- Privileged commands reuse Wallet's native finance RBAC and state machines. Supported operations are wallet restriction/suspension/reactivation, case-scoped capped partial refund and reversal requests, independent refund/reversal approval, destination-version-fenced provider settlement requests and approval, and independent provider-withdrawal approval/rejection. Rejection releases reserved provider funds through the existing earning-account invariant.
- Native Wallet approval policies remain mandatory. A missing policy, revoked permission, self-approval, stale source version, duplicate economic claim, exceeded refundable/reversible amount, unavailable owner, or cross-organization reference fails closed. The Admin boundary never posts ledger entries directly and never marks an external payment, refund, reversal, payout or settlement complete.
- Financial investigations can record and escalate `DISPUTE`, `CHARGEBACK`, fraud, reconciliation and liability findings, but no fabricated adjudication engine was added. Execution remains with Wallet's supported operations and workers; ambiguous provider outcomes retain the existing `UNKNOWN`/`RECONCILING` readback path and original idempotency identity.

## Contracts and migration

The shared owner/command profiles, Bridge capability registry, private OpenAPI, source-owner OpenAPI, standalone schemas, fixtures and checksums include the Wallet contract and are generated deterministically. The owner and Bridge read/write flags are disabled by default. Migration `000022_secure_admin_wallet_boundary` adds only private signed-context, replay, immutable audit/decision and financial-case persistence. It installs no staff grants, approval thresholds, credentials or activation values; its rollback refuses to remove recorded evidence.

## Verification evidence

Disposable PostgreSQL tests prepare every Wallet projection against the real schema and execute owner-backed restriction, investigation, capped refund, reversal, independent approval and authoritative readback. The existing Wallet integration suite continues to cover integer-kobo ledger conservation, allocation/refund caps, idempotency conflicts, concurrent decisions, settlement and withdrawal invariants, external unknown outcomes and recovery. Shared Admin tests cover HMAC v2, replay, signed source context, delegated scope, approval separation and fail-closed feature flags.

## External and domain gates

Live HMAC/mTLS credentials, HQ staff role grants, signed source policies, step-up MFA configuration and approval thresholds remain production governance inputs. There is no separate Wallet-native chargeback adjudication or global arbitrary balance-adjustment contract; those remain `BLOCKED_DOMAIN`. Provider workers and external gateways remain controlled by their existing independent runtime switches. Missing inputs never fall back to a permissive default.
