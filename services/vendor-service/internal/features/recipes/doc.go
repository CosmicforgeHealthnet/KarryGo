package recipes

// Package recipes owns Vendor recipe/BOM production definitions.
//
// Section 16B owns:
//   - stable Recipe identity
//   - immutable RecipeVersion history
//   - Ingredient requirements
//   - Packaging requirements
//   - exact normalized planning quantities
//   - expected yield
//   - expected waste assumptions
//   - transactional version activation
//
// It does not own:
//   - Food/Pastry/Cloud catalogue entities
//   - Ingredient or Packaging master data
//   - physical Inventory
//   - production batches
//   - actual consumption/waste
//   - COGS
//   - Purchasing
//   - Wallet/accounting
//
// Food, Pastry and Cloud Kitchen remain catalogue authorities.
// Section 16A productionmaster remains Ingredient/Packaging authority.
// Section 14 Inventory remains physical-stock authority.
//
// Substitutions are deliberately deferred. Proper substitution requires
// dietary/allergen/customer-claim policy integration; Section 16B does not
// create a weak or AI-guessed substitution system.
