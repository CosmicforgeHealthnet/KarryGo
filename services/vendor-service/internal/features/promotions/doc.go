// Package promotions is the reserved feature boundary for vendor and
// platform promotions, including delivery-fee subsidies whose funding source
// (customer/vendor/platform/combined) must remain attributable through to
// settlement — see docs/architecture/05_PAYMENT_ALLOCATION_MODEL.md,
// "Delivery Promotion Funding". A promotion here computes a discount and its
// funding split; it never directly moves money (internal/integrations/wallet
// does that). Not implemented in this architecture phase.
package promotions
