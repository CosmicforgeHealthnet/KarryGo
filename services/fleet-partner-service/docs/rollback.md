# Rollback

## Migration rollback

Migrations are paired `NNNNNN_name.up.sql` / `NNNNNN_name.down.sql` files
tracked in `fleet_partner_schema_migrations`. The runner
(`internal/database`) exposes:

- `ApplyMigrations` — applies unapplied up-files in order, each in its own
  transaction together with its tracking row;
- `RollbackLastMigration` — applies the down-file of the most recently
  applied migration and removes its tracking row, transactionally;
- `MigrationsCurrent` — used by `/ready` to refuse traffic on a
  partially-migrated database.

Operational procedure:

1. Stop workers first (they may hold leases against tables being removed).
2. Run the rollback (invoke `RollbackLastMigration` via a maintenance task or
   apply the down-file manually and delete the tracking row in one
   transaction).
3. Deploy the matching older binary.
4. `/ready` confirms the migration state the binary expects.

Cautions:

- `000001` down-drops the outbox, inbox, jobs, idempotency, audit and
  file-reference tables — **durable events and jobs are lost**. Only roll it
  back on environments where that is acceptable, or export the tables first.
- Down-files are covered by the integration test
  `TestMigrationsReplayAndRollback` (rollback → reapply → current).

## Verifying rollback safely

Never rehearse a rollback against a shared or real database. Use the
disposable test database described in
[local-development.md](local-development.md); the integration test
`TestMigrationsReplayAndRollback` performs fresh apply → no-op re-apply →
rollback → re-apply on every CI run, and
`TestMigrationFailureRollsBack` proves a failing migration leaves no partial
schema and no tracking row behind.

Dispose of the test database afterwards:

```bash
docker rm -f karrygo-fleet-partner-test-db
```

## Binary rollback

Both binaries are stateless outside PostgreSQL/Redis:

1. Deploy the previous image/binary for `cmd` and `cmd/worker` together.
2. Workers stop claiming during shutdown; leases from the old deployment
   expire (`FLEET_PARTNER_JOB_LEASE_DURATION`) and are recovered by the new
   deployment automatically.
3. In-flight idempotency claims either complete or are released by expiry;
   commands are safely retryable by clients.

## Configuration rollback

Configuration is env-only; restore the previous env set and restart. Startup
validation refuses a partially rolled-back configuration (fail fast) rather
than starting degraded.

## Event transport rollback

The outbox publisher adapter is wired in `cmd`. Reverting to the disabled
adapter stops publication **without losing events** — they accumulate as
PENDING and publish once a transport is restored.
