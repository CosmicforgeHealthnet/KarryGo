# Section 16A — Production Master Data

Section 16A establishes Vendor-owned Ingredient and Packaging Material master data.

## Authority boundaries

Section 16A owns:

- ingredient identity and lifecycle
- packaging-material identity and lifecycle
- MASS / VOLUME / COUNT usage bases
- exact ingredient/material-specific conversions
- non-authoritative display conversions
- branch operational configuration
- links to Section 14 Inventory items

Section 16A does not own:

- recipes/BOMs
- recipe versions
- production batches
- physical stock balances
- Inventory movements
- Purchasing
- Wallet/accounting
- customer Orders
- logistics

Section 14 Inventory remains the sole physical-stock authority.

## Inventory control

Master records support:

- UNTRACKED
- INVENTORY_CONTROLLED

An INVENTORY_CONTROLLED record can exist before a branch Inventory mapping is configured.

However, a branch cannot be marked production-eligible while an inventory-controlled master lacks its valid typed Inventory mapping.

Ingredient mapping requires an ACTIVE Section 14 Inventory item of type INGREDIENT.

Packaging mapping requires an ACTIVE Section 14 Inventory item of type PACKAGING.

## Exact conversions

No floating-point quantity arithmetic is used.

Examples of authoritative exact relationships:

- 1 kg = 1000 g
- 1 L = 1000 ml

Configured conversion uses:

numerator / denominator

and is always scoped to one Vendor master record.

Local units such as Paint, Derica, Mudu, Congo, Tiya, bowl, basket, bunch, carton, sack or jerrycan do not receive universal physical conversions.

A non-standard unit can become authoritative only through an ingredient/material-specific EXACT_AUTHORITATIVE conversion carrying explicit source context.

Otherwise it remains DISPLAY_ONLY.

## Future phases

Recipes, BOM versions, yields, production batches and COGS are not implemented by Section 16A.