# Secure Admin Section 15 — Referral implementation evidence

**Software status: `SECTION_15_BACKEND_FUNCTIONALLY_COMPLETE`.** This covers the source-supported Referral administration boundary on `staging`. It does not enable Headquarters access, deploy services, post money, install production scope mappings or approve an incentive policy.

## Implemented owner behavior

- Referral now exposes nine fixed, privacy-minimized owner resource families (18 list/detail Bridge capabilities): persons, linked accounts, referrals, qualification tracks, reward ledger entries, fraud flags, milestones, Wallet reconciliation mismatches and immutable administrative actions. Raw phone/email/device/bank hashes, referral codes, fraud evidence blobs, free-form review notes and Wallet request payloads are not disclosed.
- Eight fixed owner actions reuse Referral's real state machines: freeze/clear/block person, freeze/clear/disqualify referral, resolve fraud flag and reverse reward. There is no generic status or balance setter. Blocking/disqualification may request paid-reward take-back only through the existing negative Referral ledger entry and Wallet posting workflow.
- Every mutable person, referral, reward and flag now has a monotonic owner version. The owner command transaction locks and compares that version, applies exactly one native transition, records the existing Referral audit action and commits one immutable command receipt. Same-identity concurrent retries return the first receipt; stale or conflicting identities fail closed.
- Clear, block, disqualify, fraud resolution and reward reversal require the existing two-person approval flow bound to the exact intent digest. Self-approval is rejected. Referral-native staff grants are checked independently of delegated HQ policy; migration `000007` installs no grant.
- Every read and command requires a current signed Referral owner context for the exact subject and exact product, market, zone, organization and tenant. Referral does not own canonical geography or tenant relationships, so no production mapping is inferred from actor type, customer identity or request parameters. Missing, expired, revoked or contradictory context denies access.
- The old HMAC-only Referral operator routes remain available only behind `REFERRAL_LEGACY_ADMIN_ENABLED`; the default is `false`. The new owner boundary uses separate resource, command-write and command-readback credentials, durable nonces, HMAC v2, signed policy, immutable disclosure audit and Bridge mTLS/delegated authorization.

## Contracts, flags and migration

The private capability matrix and OpenAPI contain 24 additive Section 15 capabilities: 18 resource list/detail operations and six command/approval/readback operations. The complete private matrix contains 388 capabilities at this checkpoint. No Referral Admin route was added to the public Gateway.

All activation switches default off. Source switches are `REFERRAL_ADMIN_RESOURCE_READ_ENABLED`, `REFERRAL_ADMIN_CONTEXT_PUBLISH_ENABLED`, `REFERRAL_ADMIN_COMMAND_WRITES_ENABLED` and `REFERRAL_ADMIN_COMMAND_READBACK_ENABLED`. Bridge switches are `ADMIN_REFERRAL_RESOURCE_READ_ENABLED`, `ADMIN_REFERRAL_COMMAND_WRITES_ENABLED`, `ADMIN_REFERRAL_COMMAND_READBACK_ENABLED`, `ADMIN_REFERRAL_WRITES_ENABLED` and global `ADMIN_REMOTE_WRITES_ENABLED`. Enabling any switch supplies no grant, policy, context, credential or approval.

Migration `000007_secure_admin_boundary` adds owner versions, Referral-native staff grants, signed owner-context persistence, replay records, immutable read evidence and the immutable command ledger. It seeds no staff, access scope, reward amount, policy value, secret or production authorization. Rollback refuses to remove recorded evidence.

## Verification evidence

Disposable PostgreSQL tests read and bind all nine source dictionaries against actual Referral tables. The end-to-end owner-command test imports an independently signed test-only context, uses native staff grants and signed governance policy, proves concurrent identical submission has one effect, rejects stale versions and cross-tenant scope, requires independent approval for clearing, rejects self-approval and proves the decision ledger is immutable. The full Referral integration suite continues to cover attribution, deduplication, fraud, milestones, customer/provider rewards, reversals, Wallet posting/retry/reconciliation and signed internal transport. Bridge/shared tests cover fixed profiles, HMAC v2, mTLS, delegated scope, default-off gates, two-person approvals, original-identity readback and generated contract consistency.

## Explicit blocked dependencies

- `BLOCKED_DOMAIN` — payment-wallet currently does not publish `finance.job.settlement_reversed.v1`. Referral can consume it, but cannot claim complete post-settlement job reversal propagation until Wallet emits that exact authoritative event after a committed reversal.
- `BLOCKED_DOMAIN` — no owner state machine exists for referral appeals, incentive campaigns, promotions, seller/customer retention offers or ranking/leaderboard publication. Existing flag resolution and account/referral review are not represented as appeal or campaign decisions.
- `BLOCKED_POLICY` — production person-to-market/zone/organization/tenant contexts, staff grants, reward/incentive rules, disclosure policy and live approval policy are not supplied. No mapping or threshold is fabricated.
- `BLOCKED_INFRA` — live credentials, private network enrollment, migration execution, runtime activation, deployment and Wallet event rollout remain separate release work.
