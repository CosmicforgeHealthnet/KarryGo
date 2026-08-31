// Package food is the Restaurant/Fast-Food catalogue domain: the
// management-side authoritative answer to "what does this vendor branch sell".
//
// Section 9A implements the structure — menus, sections, items, variants,
// modifier groups, modifiers and item/modifier-group relationships — together
// with tenant and branch isolation, deterministic ordering, integer minor-unit
// pricing, optimistic concurrency, transactional audit and outbox.
//
// What this package deliberately does NOT do:
//
//   - Section 9B owns menu schedules, sold-out/effective availability,
//     branch-level price and preparation overrides, effective-price resolution
//     and catalogue price history. Nothing here pretends to answer "can this be
//     sold right now, at what effective price".
//   - Section 9C owns the read-only internal discovery/consumption contract;
//     the public frontend consumer remains outside this service. Every route
//     in this package's management handler is behind the ordinary staff
//     session, while the separate internal handler uses HMAC v2.
//   - Orders, inventory, logistics and money are other services' authority.
//     A catalogue price is commercial configuration, not settlement.
//
// Modifier groups are business-level reusable definitions rather than
// per-branch copies, so "Add extras" is written once. Branch reach is decided
// on the item a group is attached to; a branch-scoped actor may attach and
// reorder groups on their own branch's items but may not edit the shared
// definitions themselves.
//
// Gated by COMMERCE_FOOD_ENABLED (route registration) and
// VENDOR_FOOD_CATALOGUE_WRITES_ENABLED plus COMMERCE_GLOBAL_KILL_SWITCH
// (mutations), matching the Section 8 rollout convention.
package food
