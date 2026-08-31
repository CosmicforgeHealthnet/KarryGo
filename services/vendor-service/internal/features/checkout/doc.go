// Package checkout is Section 17B: checkout validation and Inventory
// reservation.
//
// It turns an ACTIVE Section 17A cart into a validated checkout candidate and
// nothing more. There is no quote, no quote lock, no logistics price, no
// payment, no ParentOrder and no VendorOrder anywhere in this package; those
// are later Section 17 responsibilities and a route, column or code path that
// implied otherwise would be a boundary violation, not a shortcut.
//
// # What "authoritative" means here
//
// Nothing the cart, the search projection or the client says about price,
// availability or catalogue state is trusted. Every line is re-resolved
// against the owning authority at validation time:
//
//   - Food, through the Section 9 Food authority, which already answers
//     business, branch, capability, menu, service window, section, item and
//     operational state as one decision and delegates INVENTORY_LINKED
//     production availability to its own fail-closed seam.
//   - Shop, through the Section 12 Shop catalogue authority for listing,
//     variant, sell unit, measurement and price.
//   - Stock, through the Section 14 generic Inventory authority, which
//     Section 14's certification made the stock authority for this service.
//     Shop's own Section 12B inventory surface remains live and unchanged;
//     Section 17B does not read or write it.
//
// The cart snapshot is kept only so that a material difference can be
// reported. Section 17B never silently re-prices a cart and never creates the
// final immutable quote; that is Section 17D's decision to make.
//
// # Reservation
//
// Shop groups reserve real physical stock through Section 14
// (source type CHECKOUT), never by writing an inventory table. Food reserves
// nothing: its current production architecture exposes availability, not a
// reservation interface, so reserving raw Shop stock on its behalf would be
// inventing a contract that does not exist.
//
// A Shop cart may span several Vendor groups. Section 14 owns one transaction
// per reservation, so cross-group atomicity is achieved by deterministic
// compensation rather than by one database transaction: the durable group row
// and its reservation source reference are committed BEFORE the Inventory
// call, so a failure or a crash always leaves a key that can find and release
// whatever was actually created. The end state is always either every
// required group RESERVED, or validation FAILED with no hold left behind.
//
// # Boundary
//
// This package imports Section 17A's cart package for the cart model and the
// two cart-owned status transitions, which is the same shape Shop already
// uses against Inventory. Food, Shop and Inventory are reached only through
// the narrow ports in ports.go, whose adapters live in the composition root.
package checkout
