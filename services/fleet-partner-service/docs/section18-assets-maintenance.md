# Section 18 - Assets, safety, maintenance, inventory, and fuel

Section 18 is implemented inside `fleet-partner-service`. Its 43 operations are registered from `assets.TenantRoutes` below `/fleet-partner/v1`; `docs/openapi/section18-assets-maintenance.openapi.json` is the machine-readable contract and is parity-tested against that catalogue.

## Security and ownership

- Every operation requires an authenticated corporate actor, trusted organization context, its semantic permission, and branch scope for the referenced asset or inventory part.
- Create and command operations require `Idempotency-Key`; mutable transitions require `expectedVersion` or `If-Match`.
- Direct and list queries independently apply organization and permitted-branch filtering to assets and their schedules, work orders, inspections, breakdowns, fuel records, parts, and movements.
- Fleet owns organization asset operations. Canonical vehicle/trip identity, Media bytes, provider identity, notification delivery, wallets, and financial ledgers remain external authorities.

## API groups

| Group | Operations | State ownership |
| --- | ---: | --- |
| Assets, assignments, documents | 9 | asset lifecycle, current assignment, external Media references |
| Inspection templates and inspections | 8 | server-evaluated safety inspections and immutable answers |
| Maintenance schedules and work orders | 11 | due rules, approval/completion, and return-to-service decisions |
| Breakdowns | 4 | rescue/towing/replacement workflow without canonical trip mutation |
| Parts and stock movements | 6 | immutable movements and locked quantity projection |
| Fuel and energy | 5 | validated usage records and approval lifecycle |

Inspection results are computed from mandatory template answers; callers cannot supply the overall result. A critical failure locks and completes the inspection, blocks the asset, records a safety block, creates one work-order intent, and writes audit/outbox records atomically. Work-order completion alone never clears a safety block. Return to service is a separate permissioned and versioned decision requiring completion evidence and, after a critical inspection, a later passing follow-up inspection.

Maintenance supports date, mileage, operating-hours, and combined due rules. Critical due work blocks an asset and raises a Section 16 operational alert. Unsafe-vehicle or vehicle-defect assignment declines are reconciled idempotently into exactly one inspection, work order, and safety block; the payload explicitly prohibits automatic provider penalty.

Stock movements are immutable, use database decimal columns, update projections under a row lock, and reject negative stock. Reorder conditions raise operational alerts. Fuel validates energy type/unit, prevents odometer or operating-hour regression, validates receipt Media IDs, and explicitly performs no wallet, ledger, or provider-penalty mutation.

## Flags and workers

`ASSETS_ENABLED`, `ASSET_ASSIGNMENTS_ENABLED`, `INSPECTIONS_ENABLED`, `MAINTENANCE_ENABLED`, `BREAKDOWNS_ENABLED`, `PARTS_INVENTORY_ENABLED`, and `FUEL_RECORDS_ENABLED` default to false. `SECTION18_WORKERS_ENABLED` also defaults false and requires the shared worker runtime. The global Fleet kill switch fails closed.

The Section 18 maintenance job uses the shared durable queue with leasing, heartbeat, retry, dead-letter, and replay behavior. It expires documents and assignments, evaluates due schedules, creates work orders idempotently, applies critical safety blocks, emits audit/outbox events, reconciles safety declines, and raises Section 16 alerts.

For rollout, apply migrations first, deploy with flags disabled, enable capabilities incrementally, and enable workers last. Rollback begins by disabling workers and capability flags. Existing blocks and history remain authoritative; migrations `000040` through `000042` have tested down/up paths for controlled schema rollback.
