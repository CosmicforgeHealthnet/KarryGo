# Section 18 — Vendor Order Lifecycle

Section 18 extends the Section 17 commerce aggregates. `commerce_vendor_order_lines`
and allocations remain immutable historical evidence. Picking writes to
`commerce_vendor_order_line_outcomes`; substitutions write a frozen replacement
snapshot to `commerce_substitution_proposals` and require the authenticated
customer's versioned decision. A cheaper replacement creates a Section 17
commercial adjustment; a more expensive replacement requires explicit
commercial-change approval.

Vendor Own Delivery can advance only through the authenticated vendor action
matrix. Platform-delivered orders can reach pickup, delivering and delivered
only through a Dispatch/Hauling event matching the frozen mission reference.
The event inbox deduplicates event ids, rejects foreign missions, and ignores
older source versions so delivery state never regresses. Wallet remains the
authority for `REFUND_PENDING`, `REFUNDED` and `PARTIALLY_REFUNDED` outcomes.
