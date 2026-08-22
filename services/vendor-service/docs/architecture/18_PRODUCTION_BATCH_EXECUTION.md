# Section 16C — Production Batch Execution

Section 16C implements operational production execution without creating a
second production-capacity engine or a second Inventory ledger.

## Authority

- Sections 10/11: capacity pools, lines, slots, capacity reservations and
  `production_jobs`.
- Section 14: physical Inventory balances, lots, reservations and movements.
- Section 16A: Ingredient and Packaging master data.
- Section 16B: Recipe/BOM definitions and immutable versions.
- Section 16C: production-batch orchestration.
- Section 16D: cost/COGS analytics — not implemented here.

## Lifecycle

`DRAFT -> PLANNED -> IN_PROGRESS -> COMPLETED`

Terminal exception states:

- `CANCELLED`
- `FAILED`

Planning freezes the selected RecipeVersion and exact scaled material
requirements. Later Recipe changes cannot alter a historical batch.

## Inventory reservation

All required `INVENTORY_CONTROLLED` Ingredient components are aggregated and
submitted to the Section 14 `ALL_OR_NOTHING` reservation engine.

Packaging participates only when both:

1. it is inventory-controlled; and
2. its branch `production_consumption_point` is `PRODUCTION`.

`FULFILMENT` packaging is not deducted by Section 16C.

Optional Ingredient components are frozen but are not automatically reserved.
They require an explicit top-up before actual controlled use.

## Actual consumption and waste

Actual use is recorded separately from Recipe expected waste.

Expected waste remains a planning assumption.

Actual waste is immutable operational evidence and is posted to the Section 14
Inventory ledger as `WASTE` when the production batch is finalized.

Actual controlled use cannot exceed reserved quantity. The API returns a
top-up-required conflict instead of silently overdrawing stock.

## Prepared stock

Production policies support:

- `ON_DEMAND`
- `BATCH_STOCK`

`ON_DEMAND` consumes controlled raw materials but creates no prepared-stock
balance.

`BATCH_STOCK` requires an active branch-owned generic Inventory item of type
`PRODUCTION_COMPONENT`. Batch completion creates exactly one prepared-stock
Inventory receipt/output.

`inventory_production_postings` is the exactly-once boundary. Retrying batch
completion cannot consume the same raw stock twice or create prepared output
twice.

## Cancellation

Before physical Inventory finalization, cancellation releases held/confirmed
Section 14 reservations.

After physical Inventory posting, cancellation does not restore consumed stock.
An explicit Inventory correction workflow is required.

## Financial boundary

Section 16C contains no:

- supplier cost calculation;
- COGS calculation;
- Wallet operation;
- payment;
- accounting journal;
- supplier settlement.

Those remain outside Section 16C.