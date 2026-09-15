# FINAL_BACKUP_RESTORE_EVIDENCE

Phase baseline `9ebad225ea9ec25397778a4dc8d3ff842f2938a3`.

## BAK-001 … BAK-006 — all NOT RUN

| ID | Gate | Status |
|---|---|---|
| BAK-001 | Create or identify a staging backup | **NOT RUN — NO ENVIRONMENT** |
| BAK-002 | Restore into an isolated database | **NOT RUN — NO ENVIRONMENT** |
| BAK-003 | Verify schema is at the latest migration | **NOT RUN — NO ENVIRONMENT** |
| BAK-004 | Verify representative records | **NOT RUN — NO ENVIRONMENT** |
| BAK-005 | Boot a service against the restored database | **NOT RUN — NO ENVIRONMENT** |
| BAK-006 | Record restore duration as RTO evidence | **NOT RUN — NO ENVIRONMENT** |

## Why, specifically

This is not "we did not get to it". **No staging PostgreSQL instance is
identified anywhere in this repository**, and no infrastructure-as-code
describes one. `infra/docker-compose.yml` is 382 lines of which 0 are active.
The per-service database URLs live in operator-provisioned env files on the VPS
that, for Vendor at least, have never been created. There is therefore nothing
to back up, no backup configuration to inspect, and no provider console to
inspect it from.

Backing up and restoring the throwaway certification databases created inside
this session would prove that `pg_dump` works. It would say nothing about
staging, and presenting it as BAK evidence would be a fabrication.

**No RTO is stated.** The section says not to invent one where none formally
exists, and none does.

## What the drill will need to cover when it can be run

Recorded now so the drill is designed once, not improvised:

- The restore target must be **isolated**. Restoring over an active staging
  database in the name of testing a restore is the classic way to turn a drill
  into an incident.
- Schema verification must read each service's own tracking table
  (`taxi_schema_migrations`, `hauling_schema_migrations`,
  `fleet_partner_schema_migrations`, `vendor_service_schema_migrations`,
  `schema_migrations` for Wallet), not a count of files on disk.
- Representative records to check, chosen because each is a different failure
  if lost: one organization, one provider membership, one assignment, one
  completed-work record, one settlement instruction, **one Wallet ledger
  journal**, one Vendor mission. The journal matters most — it is the only one
  that represents money already moved.
- Booting a service against the restored database is what catches a restore
  that is technically complete and functionally wrong (missing extension,
  missing role, sequence not advanced).
- Restore duration is the RTO evidence and must be recorded from the drill,
  not estimated.
