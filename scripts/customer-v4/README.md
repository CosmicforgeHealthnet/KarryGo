# Customer V4 certification and operations tooling

Tracked, portable tooling for Karryam Customer V4 B21 (operations), B22 (CI) and B24 (release bundle). Python 3.10+ and Go only. Everything also runs on Windows.

| File | Purpose |
|---|---|
| `pgbackup.py` | Generic PostgreSQL backup / restore / verify for every V4 service database |
| `cert.py` | Certification runner: migrations, backup drill, test gates, routes, secrets, monitoring, manifest |
| `routeaudit.py` + `openapigen/` | Route capture, public-exposure classification, OpenAPI drift, route-register freshness |
| `secretscan.py` + `secret-scan.reviewed.txt` | Credential-shape scan of distributable files |
| `monitoring_check.py` | Validates `deploy/monitoring/customer-v4/` against the metrics the code emits |
| `public-unauthenticated-routes.reviewed.txt` | Reviewed public routes that carry no auth middleware; CI fails on any new one |
| `test_tooling.py` | Offline self-tests for all of the above |

## Backup and restore (`pgbackup.py`)

The tool is service-agnostic. It holds no table lists. The schema, the migration ledger and the rows to compare are discovered from the database catalogue at run time. It serves customer, taxi, dispatch, hauling, vendor, payment-wallet and media (and any other service-owned PostgreSQL database).

### Prerequisites

- PostgreSQL client tools (`pg_dump`, `pg_restore`, `psql`) whose major version matches the server. Alternatively, pass `--docker-container <name>` (or set `PGTOOLS_DOCKER_CONTAINER`) to run them inside a container. In that mode the connection URL must be the one valid **from inside** the container.
- Connection strings come **only from environment variables**. The command line names the variable, never the value.
- The password reaches the tools through `PGPASSWORD` in the child environment. Logs and metadata name the database, never the user, password or host. The host is recorded only as a salted fingerprint, used to refuse restoring onto the backup's own source.

### Back up

```bash
export BACKUP_DATABASE_URL='postgres://USER:PASSWORD@HOST:5432/customer_service?sslmode=require'
python scripts/customer-v4/pgbackup.py backup \
  --service customer-service \
  --output-dir /secure/backups/customer-service \
  --expect-migrations-dir services/customer-service/migrations
```

Output:
- `<service>-<UTC timestamp>.dump`: `pg_dump -Fc --no-owner --no-privileges`.
- `.meta.json`, recording:
  - the tool version and service;
  - the build (`--build-id`, `BUILD_ID`, or git HEAD with `+dirty`);
  - the dump's SHA-256 and size;
  - the server version;
  - a schema fingerprint (tables, columns, constraints, indexes, triggers, functions, views, sequences, extensions);
  - each migration ledger and its tip;
  - the row count of every table, and a content digest for every table up to `--digest-row-limit` rows (default 20,000).

The backup fails and deletes its dump if the schema or migration ledger changes while it runs, or if `--expect-migrations-dir` does not match the applied tip. With `--require-quiet` it also fails if any row changes during the dump.

### Restore

```bash
export RESTORE_TARGET_DATABASE_URL='postgres://USER:PASSWORD@HOST:5432/customer_service_restore_20260929?sslmode=require'
python scripts/customer-v4/pgbackup.py restore \
  --metadata /secure/backups/customer-service/customer-service-20260929T040000Z.meta.json \
  --expect-migrations-dir services/customer-service/migrations
```

Safety rules (each refusal exits 2):

- The dump's SHA-256 must match its metadata. An altered or truncated artefact is refused.
- The target must be empty. `--replace` drops and recreates it, and only when `--confirm-target <target database name>` repeats its name.
- Restoring onto the database the backup was taken from is refused. Restore into a new database and cut over.
- A target whose name contains `prod`/`production`/`live`, or a run with `APP_ENV`/`KARRY_ENVIRONMENT=production`, is refused unless `--allow-production` is given **and** `RESTORE_CHANGE_APPROVAL_REF` names an approved change. Production restore is an operator procedure, never an automated one.

The restore runs `pg_restore --exit-on-error --single-transaction`, so a failed restore leaves nothing half-applied. It then verifies:
- the schema fingerprint;
- the object counts;
- the migration ledgers;
- every row count;
- every row digest.

Exit codes: 0 pass, 2 refusal, 3 tool failure, 4 verification mismatch.

`verify --metadata … --url-env VAR` compares any existing database with a backup's metadata without restoring.

CHECK constraints are the one place PostgreSQL re-deparses a definition differently after a round trip (the cast placement moves). For them alone, casts to a closed list of types are removed before fingerprinting. A changed value, column or structure still fails.

### Local drill (repeatable)

`cert.py backup-drill` does the following for every V4 service:
1. Builds a fresh database at the current migration tip with the service's own runner. Dispatch first applies its legacy rider-auth files, in the order `dispatch-local-bootstrap.sh` uses.
2. Loads the dev seed where one exists (Dispatch, Hauling).
3. Backs up.
4. Proves an occupied target, the backup's own source, and a tampered dump are each refused.
5. Replaces the disposable target, restores and verifies.
6. Proves that drift introduced afterwards is detected.

```bash
export CERT_PG_URL='postgres://USER:PASS@127.0.0.1:55432/postgres'           # disposable server, host view
export CERT_PG_TOOLS_URL='postgres://USER:PASS@localhost:5432/postgres'      # same server, as the tools see it
export PGTOOLS_DOCKER_CONTAINER=karry-customer-sections911-postgres          # optional
python scripts/customer-v4/cert.py backup-drill
```

### Staging drill procedure (B23; operator-run, not certified here)

1. Take a backup of each staging service database with `backup`, recording `--build-id` as the deployed image digest.
2. Create an **isolated** database per service on the staging server (never the live one) and run `restore` into it.
3. Boot the service image against the restored database with `MIGRATION=false`, and confirm `/ready`.
4. Record wall-clock duration from backup start to `/ready` as the RTO evidence. No RTO is claimed in this repository.
5. Keep the metadata JSON and the verification report with the B23 evidence.

### Production restrictions

- Never restore over a live database. Restore to a new database and cut over by configuration.
- Dumps contain customer data. Store them encrypted, off-host, with restricted access and a retention policy. Those controls are layered on this tool's output directory and are not implemented here.
- Never pass a connection string on the command line or commit it anywhere.

## Certification runner (`cert.py`)

`python scripts/customer-v4/cert.py all` runs, in order:
1. monitoring validation and tooling self-tests;
2. the secret scan;
3. migrations with each service's own runner;
4. the backup drill;
5. the targeted V4 Go gates;
6. the route audit;
7. the release manifest.

Evidence lands in `tmp/customer-v4-cert/` (git-ignored). A gate fails on any failing test, **any skipped test**, a package build failure, or a `-run` filter that matched nothing. The CI workflow `.github/workflows/customer-v4-certification.yml` calls the same steps.
