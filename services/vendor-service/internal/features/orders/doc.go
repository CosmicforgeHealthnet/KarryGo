// Package orders is the reserved feature boundary for the parent commerce
// order and its per-vendor child orders: one customer checkout may split into
// multiple vendor orders (see docs/architecture/06_COMMERCE_FULFILMENT_MODEL.md
// and 05_PAYMENT_ALLOCATION_MODEL.md), each independently accepted/rejected
// by its vendor and independently fulfilled. This owns order lifecycle state,
// not payment (internal/integrations/wallet owns money) or delivery execution
// (internal/integrations/dispatch|taxi|hauling own mission execution). Not
// implemented in this architecture phase.
package orders
