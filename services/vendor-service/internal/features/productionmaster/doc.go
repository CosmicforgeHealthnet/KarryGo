package productionmaster

// Package productionmaster owns Section 16A Vendor production master data.
//
// It owns:
//   - ingredient master data
//   - packaging-material master data
//   - exact ingredient/material-specific conversions
//   - branch operational configuration
//   - typed links to Section 14 Inventory
//
// It does not own recipes, BOMs, production batches, stock balances,
// Inventory movements, Purchasing evidence, Wallet/accounting or customer
// orders.
//
// Section 14 Inventory remains the sole physical-stock authority.
