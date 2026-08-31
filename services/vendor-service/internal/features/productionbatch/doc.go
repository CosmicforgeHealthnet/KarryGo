// Package productionbatch implements Section 16C operational production
// execution.
//
// It intentionally does not own:
//   - production capacity pools, slots or scheduling,
//   - Recipe definitions,
//   - Ingredient / Packaging master data,
//   - physical Inventory balances,
//   - purchasing cost,
//   - COGS,
//   - Wallet or accounting.
//
// Existing Sections 10/11 remain production-capacity authority.
// Section 14 remains physical-stock authority.
package productionbatch
