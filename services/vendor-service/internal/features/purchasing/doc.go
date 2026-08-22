// Package purchasing owns Vendor-scoped supplier and purchasing authority.
//
// Section 15A owns:
//   - supplier categories
//   - suppliers and contacts
//   - supplier terms
//   - SupplierProduct -> Section 14 Inventory mappings
//   - exact purchase-unit conversion metadata
//
// Section 15B owns:
//   - PurchaseOrder / PurchaseOrderLine
//   - Vendor-configurable ApprovalPolicy / approval rules
//   - PurchaseOrderApproval history
//   - DRAFT -> SUBMITTED -> APPROVED -> ORDERED lifecycle
//   - REJECTED / CANCELLED / CLOSED transitions
//   - PARTIALLY_RECEIVED / RECEIVED PO state for Section 15C to advance
//   - approval-count, branch, self-approval and step-up policy
//   - amount/content-change approval invalidation
//   - immutable PO commercial snapshots
//   - If-Match / optimistic version semantics
//   - durable command idempotency for lifecycle transitions
//
// Purchasing does NOT own:
//   - Inventory balances, lots or movements
//   - customer commerce Orders
//   - supplier invoices or supplier payment settlement
//   - Logistics execution
//
// Section 15C will create goods-receiving authority and use the transaction-
// scoped ApplyReceivingProgressTx seam. A PO becoming PARTIALLY_RECEIVED or
// RECEIVED is not itself evidence that Inventory stock has been posted.
package purchasing
