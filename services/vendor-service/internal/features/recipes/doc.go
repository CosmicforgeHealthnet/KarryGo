// Package recipes is the reserved feature boundary for recipe/ingredient
// definitions that link a catalog/ sellable item to the inventory/ stock it
// consumes on production, so selling an item can decrement raw-ingredient
// stock rather than only finished-item stock. Gated by
// VENDOR_RECIPES_ENABLED. Not implemented in this architecture phase.
package recipes
