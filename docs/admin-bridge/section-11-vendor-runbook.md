# Secure Admin Section 11 — Vendor runbook

This document describes the software boundary, not authorization to deploy or activate it. Work is confined to staging. Do not alter running services, production data, public Gateway or Partner cutover.

## Development verification

The current Vendor-owned software target is `SECTION_11_BACKEND_FUNCTIONALLY_COMPLETE`. This runbook does not authorize production activation.

Run `go run ./tools/vendor-test-runner` from `services/vendor-service` using a disposable PostgreSQL database authorized for clone/create/drop and disposable Redis. Set `VENDOR_SERVICE_TEST_DATABASE_URL`, `VENDOR_REDIS_ADDR`, `VENDOR_SERVICE_TEST_EVIDENCE_DIR` and `VENDOR_SERVICE_TEST_WIRE_DIR`. The runner creates a migrated template and a distinct database per test-bearing package. The runner remaps legacy test DSN aliases to that package's database and fails on skipped mandatory database cases. The separate five-process stack certification is explicitly reported.

Then run `scripts/admin-bridge/vendor_testgate.py` against the evidence directory and `scripts/admin-bridge/vendor_wire_contract.py` against native wire capture. Generate/validate private contracts with the existing Admin contract tools. Use the existing workflow commands for Bridge, Media and all seven required acceptance workflows. Do not substitute a shared-state Vendor `go test ./...` run for canonical certification.

## Disabled enrollment boundary

`VENDOR_CUSTOMER_STANDING_ENFORCED=false` by default. Enforcing new-booking standing requires a dedicated `VENDOR_CUSTOMER_STANDING_SECRET` matching Customer's dedicated `CUSTOMER_VENDOR_STANDING_SECRET`, caller `vendor-service`, scope `account_standing`, current signed responses and private service routing. Credentials cannot share application or other service audiences. Outage/staleness denies new purchases; committed-order idempotent readback remains available. The switch is not permission to interrupt existing fulfillment or refunds.

All `VENDOR_ADMIN_*_ENABLED` switches and `ADMIN_VENDOR_WRITES_ENABLED` default false. Bridge remote/write controls remain independently required. The owner requires signed current policy, enrolled workload identity, delegated human capability and exact source scope, plus an active native Vendor staff assignment and native permission for the same business/branch. Branch-only native assignments never authorize merchant-wide access. No enrollment is auto-created by a context publication or a successful test.

Vendor resource, context-publication, case, case-publication, command-write and command-readback HMAC keys must be distinct from each other, Customer standing, inbound service keys and application credentials. Load only the channel configuration actually enabled. Never use test keys or manufacture market/zone/organization/tenant mappings.

Media requires independently approved source enrollment, modern object/case/subject binding and its existing disclosure policy. A legacy asset ID is not an object mapping. Public clients cannot access the private Vendor Admin routes through the Gateway.

## Migration and recovery constraints

Migrations 133 and 134 are additive. Only disposable databases were migrated during development. Future normal owner rollout must first demonstrate a real, restorable backup and an independently reviewed migration plan; this runbook does not execute either.

Migration 133 records immutable context, disclosure, command and case history. Do not truncate, delete or cascade this evidence during merchant cleanup. Its down migration is permitted only when those histories are empty. Migration 134 records administrative processing of an already customer-approved substitution; `NOT_RECORDED` means no administrative processing marker and says nothing about ordinary customer processing. Its down migration refuses any recorded processing evidence.

Keep all Admin switches off through any future migration verification. A service rollback must preserve the new evidence and schema if populated; use a compatible binary or forward repair. Never force a down migration or erase a ledger to make rollback succeed. Ordinary booking and native fulfillment remain source-owned.

For an interrupted command, use the same operation identity and the private original-action readback. `UNKNOWN_OUTCOME` is not proof that a mutation failed. Never generate a replacement identity to retry stock, payment, refund or delivery effects. On audit persistence failure the native transaction rolls back; on readback outage retain uncertainty. Customer-approved substitution reconciliation uses the original deterministic inventory replacement identity and the same native transaction.

Substitution proposals must carry a future deadline from the versioned customer-approved policy flow. The recovery worker expires due cases and proposals idempotently; an approval at or after the deadline also persists `EXPIRED` before refusing the action. Do not extend a proposal by editing its deadline or reuse its identity for another candidate. `PENDING_LIABILITY` is a live Wallet-owned obligation: keep the decision and allocation at their pending states and recover only through the same Wallet idempotency key.

For new orders, the commit transaction rechecks merchant, branch and listing state and, once Vendor onboarding has started, the current native capability/Verification reducer. Do not bypass this check for an old approved quote. Existing committed orders use idempotent readback and continue their permitted fulfillment lifecycle.

Requests for refunds, verification, custody changes or cross-market closure cannot be converted into generic Admin mutations. Escalation records must remain visibly separate from actual owner results. Existing paid orders, financial allocations and mission execution must not be cancelled by a booking hold or merchant pause.

External-owner follow-up remains separate: Section 13 owns Wallet adjudication/execution, Section 14 owns any future canonical global market/zone closure, and Section 19 owns authoritative legacy Media association. Dispatch, Taxi and Hauling retain mission and custody authority. Their unavailable commands do not authorize a Vendor-side loop, database write or synthetic success.
