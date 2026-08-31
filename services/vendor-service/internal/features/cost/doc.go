// Package cost is Section 16D: operational cost for recipes and production.
//
// # WHAT THIS IS
//
// Operational analytics. It answers "what did it cost us to make this, based
// on what we actually paid for the inputs" so a vendor can see a recipe's
// estimated COGS and how a production batch compared to plan.
//
// # WHAT THIS IS NOT
//
// Accounting. There is no ledger here, no journal, no settlement, no supplier
// payment, no tax evidence and no Wallet effect of any kind. Nothing in this
// package writes to a financial system, and nothing in it may be treated as
// accounting truth. A production variance is an operational observation; it
// never becomes an automatic financial adjustment, and it never changes a
// selling price - pricing is a separate authority that does not exist yet.
//
// # AUTHORITY BOUNDARIES
//
// Section 15 owns purchase cost. This package READS
// vendor_purchase_cost_history through PurchasingCostReader and never copies
// it into a second mutable history. Section 16A owns ingredient and packaging
// master data; a cost policy references that identity and never redefines it,
// and packaging is never modelled as an ingredient for costing convenience.
// Section 16B owns recipes and their versions; costing never mutates one.
// Section 16C owns production batches and their frozen requirements; costing
// reads that evidence and never re-derives it from a newer recipe version.
// Section 14 owns inventory; no cost calculation moves stock.
//
// # EXPECTED WASTE - THE ONE THING EASY TO GET WRONG
//
// Section 16B stores NET component quantities. The expected-waste gross-up,
// qty * 10000 / (10000 - waste_bps), is applied by Section 16C when it freezes
// a batch's planned requirements. So:
//
//   - Recipe estimated COGS reads net recipe quantities and MUST apply the
//     gross-up itself, exactly once.
//   - Production expected cost reads frozen batch quantities that ALREADY
//     include it and MUST NOT apply it again.
//
// The two calculations therefore sit on opposite sides of the same rule.
// Applying it in both places double-counts waste; applying it in neither
// understates every recipe. Both directions are pinned by tests.
//
// # COMPLETENESS
//
// Absence of cost is never zero. When no authoritative evidence resolves, the
// component records why (NO_PURCHASE_EVIDENCE, CURRENCY_MISMATCH, ...) and the
// snapshot is INCOMPLETE. A real zero, if purchasing ever recorded one, stays
// distinguishable from a missing one.
//
// # CURRENCY
//
// No FX exists on this platform, so none is invented. Evidence in a currency
// other than the policy's is excluded and reported as CURRENCY_MISMATCH rather
// than converted at a made-up rate.
package cost
