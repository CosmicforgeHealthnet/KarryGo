# Secure Admin Section 14 — Pricing implementation evidence

**Software status: `SECTION_14_BACKEND_FUNCTIONALLY_COMPLETE`.** This status covers the source-supported Pricing administration lifecycle and its private Bridge boundary on `staging`. It does not enable live Pricing administration, production quote cutover, global commerce closure, or any deployment.

## Implemented owner behavior

- The existing Pricing-owned draft, edit, submit, four-eye approve/reject, scheduled activate, rollback, resolve, simulate, rollout and market-freeze state machines remain authoritative. No second policy store or generic proxy was added.
- The private Bridge has 16 fixed Pricing capabilities for policy history/detail, lifecycle commands, resolution, simulation, approval queue, rollout state/control and market freeze/unfreeze. The typed client has a pinned origin, fixed paths, HMAC v2 identity, nonce freshness, bounded JSON responses, no redirects and no forwarded browser credentials.
- Bridge authorization intersects the delegated staff permission and five-part scope with a current signed `pricing-service` policy. The source receives the signed market, zone, organization and tenant scope and rechecks the real policy geography. Market and zone policies must exactly match. Broader country/state policy changes are refused to scoped HQ callers; they remain native Pricing-operator responsibilities.
- Approval and rollout reads are filtered to the delegated market. Freeze/unfreeze and rollout writes require an exact owner market match. Resolve always uses the Bridge-authorized market and zone; a body cannot substitute another geography.
- Pricing mutations require a stable `X-Idempotency-Key`. Same-key/same-payload retries return the first stored HTTP result; key reuse with another payload is rejected. An unresolved persistence failure leaves the command `IN_PROGRESS` and prevents automatic replay.
- Draft update, submit, approve/reject and activation require the 64-character current `body_hash`. Database compare-and-set checks reject stale writes. Active bodies remain immutable, an editor cannot approve their own draft, and activation retries return the original activation rather than creating another.
- Concurrent migration application is serialized with a PostgreSQL advisory lock, preventing multiple service/test processes from racing on the migration ledger.

## Contracts and configuration

The private OpenAPI, standalone schemas, checksums and capability registry now contain 16 additive Pricing capabilities (364 total Bridge capabilities at this checkpoint). Mutation request contracts declare required idempotency and source-hash fields. No public Gateway route was added.

All capability switches remain off by default:

- `ADMIN_PRICING_READ_ENABLED`
- `ADMIN_PRICING_WRITES_ENABLED`
- `ADMIN_REMOTE_WRITES_ENABLED`

Activation additionally requires a fixed `ADMIN_PRICING_ORIGIN`, dedicated `ADMIN_PRICING_SECRET_FILE`, and valid independently signed Admin policy/key files. No production secret, operator, grant, threshold, rate book or policy value is included.

No schema migration was added. The implementation uses the existing `pricing_idempotency_keys`, immutable audit, policy-version, approval, activation, rollout and freeze tables.

## Verification evidence

Real disposable PostgreSQL tests execute the full owner lifecycle, four-eye separation, immutable active revision behavior, stale-source rejection, same-key replay and fingerprint conflict, idempotent activation, concurrent transition fencing, market freeze, scheduled activation, rollback, historical resolution, simulation non-mutation, price-neutral parity and source-scope isolation. The full Pricing service suite passes with no database skips when `PRICING_TEST_DATABASE_URL` is supplied. Bridge tests verify fixed routes, signed transport, delegated actor/purpose injection, default-off write gates, missing permission/policy denial and unsafe target rejection. Generated OpenAPI/schema/checksum validation passes.

## Explicit blocked dependencies

- `BLOCKED_DOMAIN` — Pricing still has no authoritative quote issue, quote bind or quote reconciliation routes. Taxi, Dispatch, Hauling and Vendor quote cutover remains disabled; no quote ID, fare commitment or Wallet allocation is fabricated.
- `BLOCKED_DOMAIN` — Pricing market freeze prevents Pricing changes only. It is not a global commerce shutdown. No source-owned cross-service market-closure contract exists, so no branch iteration or unrelated database mutation was added.
- `BLOCKED_POLICY` — approval expiry duration/revocation policy, live HQ operator grants and production tariff/commission approvals are not supplied. Existing content-bound approval cancellation and four-eye controls remain enforced; missing policy never receives a guessed default.
- `BLOCKED_INFRA` — live HMAC/mTLS enrollment, operator provisioning, networking, migration execution and deployment certification remain separate rollout work.
