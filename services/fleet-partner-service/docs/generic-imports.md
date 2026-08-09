# Generic import framework (Section 25C)

`internal/features/bulkimports` now serves two engines on the same two
tables (`fleet_partner_bulk_imports`, `fleet_partner_bulk_import_rows`, plus
the shared immutable `fleet_partner_bulk_import_attempts`):

- **`engine='LEGACY'`** — the original order-only bulk-import flow
  (`POST /fleet-partner/v1/orders/imports` and siblings). Untouched: every
  method that serves it (`Create`, `RunWorkerPass`, `Claim`, `Cancel`,
  `Replay`, `InsertRow`, `ListRows`) is unmodified from before this section,
  and the column default (`engine='LEGACY'`) means every row that existed
  before this migration stays exactly what it always was.
- **`engine='GENERIC'`** — the new Section 25C adapter-based framework,
  additive alongside it.

The two engines are kept disjoint by the `engine` column, not by hoping
status-string vocabularies never collide (`VALIDATING`, `FAILED`, `CANCELLED`
are meaningful in both). Every generic-engine repository method filters
`engine='GENERIC'` explicitly; the one legacy method that could otherwise
have scooped up a generic-engine row (`Claim`, the worker's polling query)
gained `AND engine='LEGACY'`. The legacy usecase methods (`Get`, `List`,
`Cancel`, `Replay`) additionally reject a resolved row whose `engine` is not
`LEGACY`, and the generic equivalents are scoped to `engine='GENERIC'` in
their own SQL — so an id from one engine is never reachable through the
other's routes, even though both share one id namespace.

## Architecture

Generic import code never writes a domain table itself. Each import type is
served by a registered `Adapter` (`internal/features/bulkimports/usecases`
`generic.go`):

```go
type Adapter interface {
    Type() string
    SchemaVersion() int
    RequiredColumns() []string
    OptionalColumns() []string
    ValidateRow(raw map[string]string) RowOutcome
    Commit(ctx context.Context, t tenant.Context, correlationID string, data map[string]any) (subjectReference string, err error)
}
```

The real adapters (`internal/app/imports_adapters.go`) call the owning
domain's own usecase exactly as its own HTTP handler would — `Commit` for
`WORKFORCE_PROFILE` calls `workforce.Service.CreateProfile`, `ASSET` calls
`assets.Service.CreateAsset`, `WAREHOUSE_INVENTORY` calls
`warehouse.Service.Receipt`, `PRIVATE_ORDER` reuses the exact same
`OrdersPort` adapter the legacy engine already uses. Nothing bypasses that
domain's own permissions, eligibility or safety rules, because nothing
re-implements them — the generic engine only calls in.

## Job lifecycle

```
UPLOADED → VALIDATION_PENDING → VALIDATING → VALIDATED → COMMIT_PENDING → COMMITTING → COMMITTED
                                     │                                         │
                                     └→ VALIDATION_FAILED                      ├→ PARTIALLY_COMMITTED
                                                                                └→ FAILED
Any non-terminal status → CANCELLED.
FAILED (whole-job) retries with backoff until the attempt ceiling, then → DEAD_LETTER → (operator replay) → VALIDATION_PENDING or COMMIT_PENDING.
```

- **Dry-run vs commit** are separate commands, always: validating a
  `DRY_RUN`-mode job runs the identical validation path a `COMMIT`-mode job
  uses, but `POST .../commit` is refused outright for `DRY_RUN` — there is no
  code path from `DRY_RUN` to a committed record.
- **Commit requires a successful `VALIDATED` state.** A job cannot be
  committed straight from `UPLOADED`.
- **Partial success is explicit**: `PARTIALLY_COMMITTED` when some rows
  committed and some failed; `COMMITTED` when every attempted row committed
  (rows invalid at validation are not "attempted" — they were never eligible
  to commit); `FAILED` when a commit pass attempted at least one row and
  none of them committed.
- **Row correction / versioning**: a corrected replacement row is a new row
  record (`row_version` incremented, `replaces_row_id` pointing at the
  original), never an in-place edit — the original row and its attempt
  history are preserved. `row_reference` (reused from the legacy schema) is
  `"<externalReference>#v<rowVersion>"`, unique per version;
  `external_reference` carries the stable identity shared across every
  version of the same logical row, and `ListLatestGenericRows` (`DISTINCT ON`)
  is what a commit pass actually acts on.
- **Row-level retry** (`POST .../retry`, `POST .../rows/:rowId/retry`) re-arms
  `FAILED` rows to `VALID` and requeues the job to `COMMIT_PENDING` — this
  works whether the job landed in `PARTIALLY_COMMITTED` or whole-job `FAILED`
  (every row failed), since both are exactly what a retry exists to recover.
- **Whole-job retry-with-backoff** (`ClaimForRetry`, distinct from the
  row-level retry above) is worker-driven: a `FAILED` job (validation or
  commit path threw before completing, e.g. Media unavailable) is
  automatically reclaimed once its backoff window elapses, re-entering
  whichever phase it was last attempting, until the attempt ceiling sends it
  to `DEAD_LETTER`.
- **Corrected-row submission** (`POST .../rows/:rowId/corrections`) creates a
  new immutable row version for an `INVALID` or `FAILED` row — the original
  is never edited. The submitted data is validated synchronously through the
  same `Adapter.ValidateRow` every other row in the job used (no separate
  validation code path for corrections); the new version lands `VALID` or
  `INVALID` based on that outcome. A `COMMITTED` row can never be corrected.
  If the correction is valid, the job is requeued straight to
  `COMMIT_PENDING` for the next commit pass — including from
  `VALIDATION_FAILED` (a job whose only row(s) were invalid), skipping a
  redundant full re-validation, since the one thing that changed was already
  validated by the correction call itself.

## Routes

`/fleet-partner/v1/imports*`, additive alongside the unchanged
`/fleet-partner/v1/orders/imports*` compatibility routes (same aggregate,
`engine='LEGACY'`, never touched):

| Method | Path | Permission |
| --- | --- | --- |
| `GET` | `/imports` | `imports.read` |
| `POST` | `/imports` | `imports.manage` (Idempotency-Key) |
| `GET` | `/imports/:id` | `imports.read` |
| `GET` | `/imports/:id/rows` | `imports.read` |
| `POST` | `/imports/:id/validate` | `imports.validate` (Idempotency-Key) |
| `POST` | `/imports/:id/commit` | `imports.commit` (Idempotency-Key) |
| `POST` | `/imports/:id/rows/:rowId/corrections` | `imports.retry` (Idempotency-Key + expectedVersion) |
| `POST` | `/imports/:id/retry` | `imports.retry` (Idempotency-Key) |
| `POST` | `/imports/:id/rows/:rowId/retry` | `imports.retry` (Idempotency-Key) |
| `POST` | `/imports/:id/cancel` | `imports.manage` (Idempotency-Key + expectedVersion) |
| `POST` | `/imports/:id/replay` | `imports.retry` (Idempotency-Key) |

`imports.read`/`.manage` are the pre-existing legacy permissions (unchanged
grants). `imports.validate`/`.commit`/`.retry` are new (migration
`000056_generic_imports`), granted to the owner and operations-manager roles.

## Import types

`PRIVATE_ORDER`, `WORKFORCE_PROFILE`, `ASSET`, `WAREHOUSE_INVENTORY` — all
four registered in `internal/app/bulkimports_wiring.go`. Requesting any other
value is rejected by the `import_type` CHECK constraint before it reaches
application code; requesting a recognized-but-unregistered type (none exist
in this deployment, but the check exists for forward-compatibility) is
refused as "recognized but not available", the same pattern the export
framework uses.

Each adapter's expected CSV columns:

| Type | Required | Optional |
| --- | --- | --- |
| `PRIVATE_ORDER` | rowReference, customerId, serviceCatalogItemId, timezone | customerReference, contractId, priority, specialInstructions |
| `WORKFORCE_PROFILE` | membershipId, employeeNumber, relationshipType, role | supervisorMembershipId, startDate |
| `ASSET` | assetCode, assetType | manufacturer, model, year, capacity, capacityUnit, ownershipType, registrationReference, platformVehicleId |
| `WAREHOUSE_INVENTORY` | warehouseId, locationId, ownerType, ownerReference, sku, quantity | batchReference, lotReference, serialReference, unit, condition, expiryAt |

`WORKFORCE_PROFILE` requires an existing organization membership id — it
attaches a workforce profile to a membership that must already exist; it does
not create a person from scratch. `WAREHOUSE_INVENTORY`'s adapter passes the
row's own `row_reference` as `warehouse.Receipt`'s `CorrelationID`, which
makes Receipt itself idempotent per row version: a retried commit of the same
row version never double-receives stock, while a corrected row (new version,
new `row_reference`) is free to apply its own corrected quantity.

A duplicate-key domain error (employee number, asset code) from
`workforce.CreateProfile`/`assets.CreateAsset` — neither of which classifies
it itself — is mapped to a safe row-level conflict by the adapter
(`asDuplicateConflict` in `imports_adapters.go`), never the raw
constraint-violation text.

## Workers

Three passes, `internal/features/bulkimports/usecases/generic_worker.go`,
registered in `internal/app/section15_workers.go` (both the API and the
dedicated worker binary): `RunValidationPass`, `RunCommitPass`,
`RunRetryPass`. All three share one dispatcher (`processClaimed`, switching
on the claimed status: `VALIDATING` → validation logic, `COMMITTING` →
commit logic) — they differ only in which query claims the batch. Every
worker uses `SKIP LOCKED`, a leased owner + heartbeat, a bounded batch,
per-attempt immutable records (`fleet_partner_bulk_import_attempts`, the
same table and append-only trigger the legacy worker already uses), and
graceful shutdown via context cancellation (the same ticker-loop helper
`section15_workers.go` already uses for every other Section 15 worker).

## Configuration

Every write and worker flag defaults to `false`.

| Variable | Default |
| --- | --- |
| `IMPORTS_ENABLED` | `false` |
| `IMPORTS_WRITES_ENABLED` | `false` |
| `IMPORTS_VALIDATION_WORKER_ENABLED` | `false` |
| `IMPORTS_COMMIT_WORKER_ENABLED` | `false` |
| `IMPORTS_RETRY_WORKER_ENABLED` | `false` |

The existing legacy flags (`BULK_IMPORTS_ENABLED` etc.) are unchanged and
independent — enabling generic imports does not enable or require the
legacy order-import flow, and vice versa.

## Schema

Migration `000056_generic_imports` (reversible), purely additive on top of
`000031_bulk_order_imports`:

- `fleet_partner_bulk_imports` gains `import_type`, `engine`, `mode`,
  `schema_version`, `file_fingerprint`, `external_reference`,
  `committed_rows`, `validation_version`; the `status` CHECK is widened
  (dropped and re-added under a new name, discovered dynamically rather than
  by a guessed constraint name) to add the generic vocabulary alongside every
  original value.
- `fleet_partner_bulk_import_rows` gains `external_reference`,
  `schema_version`, `commit_status`, `generated_subject_reference`,
  `row_version`, `replaces_row_id`; its `status` CHECK is widened the same
  way to add `PENDING`/`COMMITTED`/`SKIPPED` alongside the original four
  values.
- Three new permissions (`imports.validate`, `.commit`, `.retry`).

No backfill statement is needed: every column's `DEFAULT` (`import_type`
defaults to `'PRIVATE_ORDER'`, `engine` to `'LEGACY'`) is applied by Postgres
to pre-existing rows the moment `ADD COLUMN` runs.

## Known limitations

- File format is CSV-only for the generic engine (the legacy engine also
  accepts JSON; the generic parser does not yet).
- The HTTP response bodies for both the legacy and generic import routes
  serialize the raw `models.Import`/`models.Row` Go structs with no `json`
  tags (pre-existing legacy convention, matched rather than silently
  changed) — unlike documents/exports/search, which use dedicated View
  types. Wire field names are therefore the exported Go names (`"ID"`, not
  `"id"`).
- Cross-route idempotency between the compatibility (`/orders/imports`) and
  generic (`/imports`) surfaces is not unified — the legacy routes do not use
  the shared `Idempotency-Key`/`idempotency.Store` mechanism at all (they
  rely solely on the `(organization_id, media_file_id, request_fingerprint)`
  unique constraint), so there is no shared identity to unify against.
- Error reports (`WriteErrorReport`) reuse the same `MediaPort` the legacy
  engine already uses; format is a JSON body of `{externalReference, code}`
  pairs, never row data.
