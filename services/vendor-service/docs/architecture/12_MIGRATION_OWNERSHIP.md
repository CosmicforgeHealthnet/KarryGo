# 12 — Migration Ownership

## Runner convention chosen, and why

Three migration mechanisms coexist in the platform (see [00](00_REPOSITORY_AUDIT_BASELINE.md)). Vendor Service adopts `payment-wallet-service`'s exact pattern: paired `NNNNNN_name.up.sql`/`NNNNNN_name.down.sql` files, a service-local tracking table (`vendor_service_schema_migrations`), and a self-contained Go runner (`internal/database/migrate.go`) that is:

- **Deploy-safe without an external script** — unlike `driver-dispatch-delivery-service`, which has no in-binary migration runner at all and depends on an external `psql`-driven bootstrap script that its own documentation admits produces files that are "not all re-run-safe."
- **Query-driven for rollback, not count-based** — `RollbackLastMigration` determines the most recently applied migration by querying `vendor_service_schema_migrations` directly, never by a caller-supplied step count. This deliberately avoids a real, repeated bug class in `fleet-partner-service`'s own history: multiple migration-replay tests there hardcoded "roll back the last N migrations," which silently under-rolled-back every time a later session added a new migration on top, and in one case left a shared test database mid-rollback for whatever test ran next in the same process. `ListMigrations` also hard-fails on a duplicate numeric prefix, so two migrations can never silently race for the same slot.

## Sequencing (per the specification's requested domain order)

```text
Foundation                          -> 000001_vendor_service_foundation
Durability (outbox/inbox)           -> 000002_events_outbox_inbox
Durability (idempotency)            -> 000003_idempotency_keys
Authentication and IAM              -> 000004, 000005
Vendor identity/business/branches   -> 000006, 000007
Onboarding/verification/activation  -> 000008, 000009
Branches/staff                      -> (future)
CRM                                 -> (future)
Catalogues                          -> (future)
Inventory                           -> (future)
Purchasing                          -> (future)
Production                          -> (future)
Commerce Orders                     -> (future)
Fulfilment                          -> (future)
Vendor delivery                     -> (future)
Advanced features                   -> (future)
```

No single giant migration exists or is planned; each future domain gets its own additive, sequentially-numbered migration when that feature section is implemented.

## What exists today

- `000001_vendor_service_foundation` — `pgcrypto` extension, a singleton `vendor_service_identity` marker row. No business tables — this is scaffolding to give the runner and its tests something real to exercise, not a business migration.
- `000002_events_outbox_inbox` — `vendor_outbox`, `vendor_inbox_processed_events`.
- `000003_idempotency_keys` — `vendor_idempotency_keys`.
- `000004`-`000005` — authentication/IAM tables and permissions.
- `000006`-`000007` — Vendor business graph and Section 6 permissions.
- `000008_vendor_section7_onboarding` — onboarding aggregate, representative, opaque documents, verification projection/history/requirements, masked bank projection, durable operations and callback nonces.
- `000009_vendor_section7_permissions` — six Section 7 permissions and role mappings.

## Verification performed (not merely asserted)

`internal/database/migrate_test.go` is gated on `VENDOR_SERVICE_TEST_DATABASE_URL` (skips cleanly, like every other service's real-Postgres-gated test, when unset). It was run live against a throwaway database on the repository's existing `karrygo-fleet-partner-test-db` Postgres container during this architecture session (created and dropped as part of verification, no lasting change to that container):

1. `ApplyMigrations` — all three migrations applied; `MigrationsCurrent` confirms.
2. Confirmed `vendor_service_identity` row and `vendor_idempotency_keys` table exist.
3. `RollbackLastMigration` — rolled back `000003`; confirmed `vendor_idempotency_keys` was actually dropped (not just un-tracked) and `MigrationsCurrent` correctly reports it missing.
4. `ApplyMigrations` again — reapplied cleanly; confirmed `vendor_idempotency_keys` exists again and the tracker is consistent.

Result: **pass**. This is the "migration replay" test layer required by the task's test-architecture instruction, exercised for real rather than only described.

## Section 6 migrations

Section 6 adds only vendor-service migrations: `000006_vendor_business_brands_capabilities_branches` and `000007_vendor_section6_permissions`. Both have down migrations. `000006` is additive to Section 5: existing opaque IAM branch assignments are not retroactively foreign-keyed; new production assignments are validated through a service port. Migration replay always discovers the latest version dynamically and therefore covers these additions without hardcoded positioning.

## Existing-service migration changes

Section 7 adds `verification-compliance-service` migration `000002_vendor_verification_cases`, including Vendor cases/history and additive delivery state on its existing outbox. Payment Wallet and Media reuse existing schemas. The change register records ownership, rollback and deployment config.
