# Generic export framework (Section 25D)

Asynchronous, permissioned data export. A caller requests an export of a
registered dataset type; a durable worker generates a CSV file through a
typed adapter and stores it at Media File Service; the caller re-authorizes
a short-lived download once it is ready.

Generic export code never writes a domain table directly and never builds
unrestricted SQL against another feature's schema — every dataset is served
by a registered `Exporter` adapter that reads through its own domain's
existing tenant- and branch-scoped read path, exactly as an ordinary list
call would.

## Status

Only the **ORDERS** dataset adapter is registered (`internal/app/exports_wiring.go`
`ordersExporter`). The other datasets the wider Section 25D scope names —
shipments, providers/workforce, assets/maintenance, compliance/incidents,
invoices/receivables, warehouse inventory, fulfilment orders, support-case
projections — are not yet wired. Requesting one of those types is refused
with a clear "recognized but not available" error rather than silently
accepted and never generated, because the export_type CHECK constraint (and
`models.ValidExportType`) recognize only `ORDERS` today; widening either
requires an additive migration, matching how document categories and owner
types are already widened in this service.

## Job lifecycle

```
PENDING → RUNNING → READY → EXPIRED
             │         │
             ├→ FAILED (retryable, exponential backoff)
             │     │
             │     └→ DEAD_LETTER → (operator replay) → PENDING
             │
             └→ (any non-terminal state) → CANCELLED
```

- **PENDING**: created, waiting for a worker.
- **RUNNING**: claimed by a worker (`FOR UPDATE SKIP LOCKED`, leased, heartbeat during long steps).
- **READY**: file generated and stored at Media; downloadable until `expires_at`.
- **FAILED**: one attempt failed; retried automatically once its backoff window elapses.
- **DEAD_LETTER**: exhausted its attempt ceiling; never auto-claimed again — only `POST /exports/:id/replay` (an operator action) resets it to PENDING with a clean attempt count.
- **EXPIRED**: a READY job whose `expires_at` has passed; the expiry worker sweeps these on a timer.
- **CANCELLED**: stopped by the caller before completion; version-guarded like every other mutation in this service.

Every worker attempt — success or failure — is recorded in the append-only
`fleet_partner_export_attempts` table (enforced by
`fleet_partner_forbid_mutation()`, the same trigger bulk imports uses).

## Routes

All tenant-scoped under `/fleet-partner/v1`, three separate permissions
(mirroring documents): reading a job's status, requesting/cancelling one, and
obtaining the file itself are different capabilities.

| Method | Path | Permission |
| --- | --- | --- |
| `GET` | `/exports` | `exports.read` |
| `POST` | `/exports` | `exports.manage` (Idempotency-Key required) |
| `GET` | `/exports/:id` | `exports.read` |
| `POST` | `/exports/:id/download-url` | `exports.download` |
| `POST` | `/exports/:id/cancel` | `exports.manage` (Idempotency-Key + expectedVersion) |
| `POST` | `/exports/:id/replay` | `exports.manage` (Idempotency-Key) |

`exports.read`/`exports.manage`/`exports.download` are granted to the
organization owner and the operations manager role
(migration `000055_exports`).

## Media output

The generated CSV is uploaded through the same `platformclients.MediaAuthority`
contract documents and bulk imports already use
(`internal/app/exports_wiring.go` `exportsMediaAdapter`). Fleet stores only
the returned `media_file_id` — never the bytes, never a permanent or signed
URL. `POST /exports/:id/download-url` issues a fresh, short-lived grant each
time; nothing is persisted or logged. A `READY` job past its `expires_at`
answers "expired" rather than serving a stale reference.

## Workers

`internal/app/section25_workers.go` `StartSection25Workers`, registered in
both the API binary (behind `FLEET_PARTNER_WORKER_ENABLED`) and the dedicated
worker binary, so a split deployment cannot end up with generation running
but expiry not, or vice versa:

- **Generation** (`Service.RunGenerationPass`): claims a bounded batch (`SKIP LOCKED`,
  leased, heartbeat), reads rows through the registered exporter (bounded by
  `FLEET_EXPORTS_MAX_ROWS`), renders CSV, uploads to Media, marks `READY`.
- **Expiry** (`Service.RunExpiryPass`): sweeps `READY` jobs past `expires_at` to `EXPIRED`.

## Configuration

Every write and worker flag defaults to `false`.

| Variable | Default |
| --- | --- |
| `EXPORTS_ENABLED` | `false` |
| `EXPORTS_WRITES_ENABLED` | `false` |
| `EXPORTS_WORKER_ENABLED` | `false` |
| `FLEET_EXPORTS_KILL_SWITCH` | `false` |
| `FLEET_EXPORTS_MAX_ROWS` | `50000` |
| `FLEET_EXPORTS_RETENTION_HOURS` | `168` (7 days) |

## Schema

Migration `000055_exports` (reversible):

- `fleet_partner_export_jobs` — one row per export request: type, format,
  filter snapshot, status, media reference, row count, expiry, lease/attempt
  bookkeeping, optimistic `version`.
- `fleet_partner_export_attempts` — immutable per-attempt outcome history.

## Known limitations

- Only the `ORDERS` dataset is wired; the remaining datasets named in the
  broader Section 25D spec are unbuilt.
- No dead-letter/reconciliation **operator listing** route yet (replay exists
  per-job via `POST /exports/:id/replay`; there is no `GET` surface that lists
  every dead-lettered job across the organization the way documents'
  reconciliation-cases listing does).
- Format is CSV-only; the `format` column and `models.ValidFormat` are
  structured to add a second format without a schema change, but nothing
  else does yet.
