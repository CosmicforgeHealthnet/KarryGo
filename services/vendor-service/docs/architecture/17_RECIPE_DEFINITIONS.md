# Section 16B — Recipes/BOM, Versioning, Yield and Expected Waste

Section 16B defines how Food, Pastry and supported Cloud Kitchen products are produced.

## Authority boundaries

Catalogue authority remains:

- Food
- Pastry/Bakery
- Cloud Kitchen

Ingredient/Packaging master authority remains Section 16A.

Physical stock authority remains Section 14 Inventory.

Section 16B never creates Inventory movements, reservations, consumption, production batches, actual waste or COGS.

## Typed targets

Supported recipe targets are:

- FOOD_ITEM
  - target_ref = food_items.id
  - optional target_variant_ref = food_item_variants.id attached to the item

- PASTRY_PRODUCT
  - target_ref = pastry_products.id
  - optional target_variant_ref = pastry_options.id attached to the product through its option-group configuration

- CLOUD_KITCHEN_PACKAGE
  - target_ref = cloud_kitchen_packages.id
  - no variant ref

Target business and branch are resolved server-side.

## Recipe identity and versions

Recipe is stable identity.

Material composition belongs to RecipeVersion.

Statuses:

- DRAFT
- ACTIVE
- RETIRED

Only one ACTIVE version may exist per Recipe.

Activation locks the Recipe identity and performs:

old ACTIVE -> RETIRED
new DRAFT -> ACTIVE

inside one PostgreSQL transaction.

The database also has a partial unique index preventing two ACTIVE versions.

Once ACTIVE or RETIRED, material definition cannot be changed in place.

## Components

Ingredient requirements use exact rational quantities:

quantity_numerator / quantity_denominator

The recipe stores the entered unit.

At activation, Section 16B resolves the quantity into the Section 16A Ingredient default usage basis where an EXACT_AUTHORITATIVE conversion exists.

For INVENTORY_CONTROLLED Ingredients:

- active Ingredient required
- branch Inventory mapping required
- production eligibility required
- exact authoritative unit conversion required where recipe unit differs from stock/usage basis

For UNTRACKED Ingredients:

- entered quantity remains valid
- exact normalized quantity is recorded when resolvable
- unresolved display/local units do not pretend to be authoritative Inventory arithmetic

## Packaging

Packaging requirements use the same exact rational model.

Section 16B only defines the requirement.

Section 16C decides when/how operational production consumes or reserves inventory-controlled Packaging.

## Yield

Yield is exact:

expected_output_numerator / expected_output_denominator

Example:

Recipe yield = 10 PORTIONS
requested production = 50 PORTIONS

multiplier = 50 / 10 = 5

Every Ingredient and Packaging requirement scales by exactly 5.

No floating-point quantity arithmetic is used.

## Expected waste

Expected waste is a planning assumption in basis points.

Example:

500 bps = 5%

When a component expects 5% loss:

gross planned input =
usable quantity * 10000 / 9500

Component waste overrides the RecipeVersion general expected-waste assumption.

Expected waste is NOT actual waste.

Actual consumption and waste belong to Section 16C.

## Substitutions

Recipe substitutions are deferred.

The current Food architecture contains dietary/allergen claims that a substitution system must respect.

Section 16B therefore does not implement weak substitutions, automatic guessing or AI-selected Ingredient alternatives.

A later explicit substitution policy can be introduced only with claim/allergen compatibility rules.

## Future phases

Section 16B does not implement:

- production batches
- Inventory consumption
- actual yield
- actual waste
- COGS
- Wallet/accounting