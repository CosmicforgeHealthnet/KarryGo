// Package searchrebuild is the production SearchFullRebuildSource: it
// reconstructs Section 13's Food and Shop search projections from the
// authoritative Vendor domain.
//
// # Why this package sits outside internal/features
//
// A rebuild source is a bridge. To rebuild one Food search document it has to
// read the Food catalogue, the canonical taxonomy, semantic tags, dietary
// claims, branch geography and business verification — five features at once.
// Putting that inside internal/features/searchprojection would make a feature
// package reach across the whole service; putting it inside internal/features/food
// would make the Food feature own Shop rebuilds. Neither is right.
//
// It also cannot go through the owning features' repositories:
// internal/architecture_test.go forbids one feature importing another's
// repositories package, and those repositories expose per-entity reads rather
// than the bulk keyset scans a rebuild needs. So this package reads the
// authoritative tables directly, read-only, from a composition-level package —
// the same shape internal/crmcontract and internal/section13security use to
// compose several features legitimately.
//
// # What "authoritative" means here, and what it forbids
//
// Every value written into a rebuilt document is read from the domain that
// owns it:
//
//   - Food items, menus, prices, availability  -> Section 9 tables
//   - Shop products, variants, listings, units -> Section 12 tables
//   - Inventory balances                        -> Section 12B tables
//   - Canonical classification and vocabulary   -> Section 13A-13C tables
//   - Business verification (trust)             -> Section 7 projections
//   - Branch geography (serviceability)         -> Section 6 tables
//
// It never reads food_search_documents, shop_search_documents, the search term
// tables, or Redis. That is the whole point of a rebuild: if the projection
// were its own source, a corrupted projection would rebuild into an identically
// corrupted projection, and the recovery mechanism would be unable to recover
// anything. The Section 13I hard gate proves this by deleting and corrupting
// the existing projection before rebuilding, then checking the result matches
// the domain rather than the corruption.
//
// # Bounded, restart-safe iteration
//
// Batches are keyset-paginated over an immutable primary key (food_items.id,
// shop_branch_listings.id), never OFFSET. A rebuild of a large mutable
// catalogue with OFFSET silently skips or duplicates rows as the underlying
// data shifts beneath the cursor; a keyset scan cannot. The checkpoint is the
// last id of the previous batch, so a worker that dies mid-rebuild resumes from
// its persisted checkpoint rather than starting over.
//
// Each batch issues a fixed number of bulk queries — every enrichment is an
// `= ANY($1)` over the batch's ids — so the query count per batch is constant
// regardless of batch size. There is no per-document query anywhere in this
// package, and the Section 13I gate asserts that.
//
// # Unknown stays unknown
//
// Where the platform has no authoritative source for a signal, the signal is
// UNKNOWN and is written as UNKNOWN. Reliability is the clearest case: nothing
// in Vendor Service records fulfilment completion or cancellation rates, so
// every rebuilt document carries reliability UNKNOWN with no basis points
// rather than an invented perfect score. Section 13F requires exactly this, and
// inventing a positive default is the specific failure it exists to prevent.
package searchrebuild
