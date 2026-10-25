# Secure Admin Sections 12–15 integration runbook

This runbook is for later, separately authorized rollout work. Code integration into `staging` does not authorize deployment or capability activation.

## Common order

1. Deploy owner migrations with the owner migration role and verify the exact migration head. Never run migrations from the runtime role.
2. Enroll a dedicated owner HMAC secret for `admin-backoffice-service`; do not reuse a browser, quote-consumer or public Gateway credential.
3. Install independently approved signed owner policies and revocation material. Draft repository policy documents contain no access grants.
4. Configure the fixed private owner origin and secret file. Verify private routing, workload identity, clock synchronization and nonce persistence.
5. Enable owner read support, then the matching Bridge read switch. Verify an allowed and a cross-tenant/cross-market denial before considering writes.
6. For writes, enable both the domain switch and `ADMIN_REMOTE_WRITES_ENABLED`. Verify source versions, idempotent retry, audit persistence failure and authoritative readback. Never retry an uncertain command under a new identity.
7. Roll back capability activation by disabling the domain switch first. Preserve immutable evidence, owner records and operation ledgers.

## Section 12 — Fleet

Use Fleet's canonical organization tenant and current signed market/zone allocation. Private manifests require their distinct permission. Investigation commands change only Fleet investigation cases; they cannot forge quotation, approval, assignment, custody or Wallet state.

## Section 13 — Wallet

Apply Wallet migration `000022` only in an authorized window. Confirm finance RBAC, step-up and signed approval policy before enabling any write. A missing threshold blocks execution. Reconcile `UNKNOWN_OUTCOME` using the original financial identity. Never operate the ledger directly.

## Section 14 — Pricing

Reads require `ADMIN_PRICING_READ_ENABLED`, `ADMIN_PRICING_ORIGIN`, `ADMIN_PRICING_SECRET_FILE` and signed policy/key files. Writes additionally require `ADMIN_PRICING_WRITES_ENABLED` and `ADMIN_REMOTE_WRITES_ENABLED`. Use the Pricing `body_hash` as the expected source version and one stable idempotency key per intended mutation. The source accepts scoped HQ actions only for exact market/zone policies; country/state policies require separate native operator authority.

Pricing freeze is not global market closure. Quote issue/bind/reconcile cutover remains unavailable and must stay disabled. Do not set rollout mode to central quote authority until those owner contracts exist and the consuming services have separate certification.

## Section 15 — Referral

Apply Referral migration `000007` only through the normal Referral migration role. It installs no staff grant or scope relationship. Provision Referral-native staff grants separately, then publish an independently signed owner context for each exact subject and five-part scope. Because Referral records have no canonical market/zone/organization/tenant relationship, a missing context is expected to deny access; never derive one from request parameters or account type.

Keep `REFERRAL_LEGACY_ADMIN_ENABLED=false`. Reads require the separate source resource key and matching source/Bridge read switches. Commands require separate write and readback keys, the matching owner/Bridge command switches, `ADMIN_REFERRAL_WRITES_ENABLED` and `ADMIN_REMOTE_WRITES_ENABLED`. Verify an allowed read, foreign-tenant denial, revoked-context denial, immutable audit failure, stale version, self-approval denial, same-key replay and original-identity readback before any rollout.

Reward reversal means Referral recorded a take-back request in its native ledger. It does not mean Wallet posted the financial effect. Reconcile Wallet state using the original reward/reversal references. Do not enable automatic post-settlement job reversal propagation until Wallet publishes the exact `finance.job.settlement_reversed.v1` contract and that path is separately certified.
