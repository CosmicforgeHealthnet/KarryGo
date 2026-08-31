# Commerce settlement and refund traceability

## Authority and accounting

- `payment-wallet-service` remains the sole ledger and balance authority. Vendor Service persists immutable policy evidence and projections only.
- One ParentOrder Commerce Payment Intent and its frozen VendorOrder allocations are reused. No second charge, payment engine, refund ledger, or settlement engine was introduced.
- Wallet migration `000011_commerce_refund_recovery_accounting` records allocation-local customer/funder outputs, escrow or released-beneficiary recovery sources, platform-loss funding, component reductions, and unused-authorization release.
- Allocation, item, delivery and difference refunds use immutable economic idempotency keys. Wallet row locks, component caps and allocation-local counters prevent sibling consumption and cumulative over-refund.

## Settlement ownership

- VendorOrder completion releases Vendor merchandise and, only for Vendor-owned delivery, the Vendor delivery component.
- Platform delivery never credits a Vendor delivery payable. Dispatch or Hauling completes its own commerce mission and calls Wallet's existing provider-delivery release with the assigned provider identity. Wallet binds the authenticated logistics caller to its provider type and resolves the immutable allocation fingerprint internally, so logistics never receives the Vendor-only payment-intent projection.
- Platform revenue remains a separate Wallet component. Vendor-, Platform- and other-approved funding attribution remains frozen through release and reversal.

## Deterministic refund policy

`internal/features/orders/refund_policy.go` evaluates the order's stored `commerce-refund-policy.v1` snapshot for Vendor rejection/cancellation, Shop AUTO_REFUND, cheaper replacement, variable-measure finalization, cheaper fulfilment replan, and bounded delivery-failure classifications. It returns economic instructions, never ledger entries.

- Pre-preparation Vendor rejection/cancellation refunds only that VendorOrder allocation. Post-preparation value consumption routes to Support review.
- Shop AUTO_REFUND uses the frozen line and funding snapshot, releases only that Inventory item hold through the existing Inventory authority, and leaves sibling lines untouched.
- Cheaper replacement, variable-measure and replan refunds are exact differences. Customer refunds never include a sponsor contribution the customer did not pay.
- Delivery failure is explicitly one of delivery-only, goods-and-delivery, customer-caused/no automatic refund, replan-required/no refund, or ambiguous/support review. Prepared Food does not inherit generic Shop return behavior.
- Provider-preserving customer protection is an explicit Platform expense; Vendor Service never manufactures customer money or debits a sibling allocation.

## Durability, projection and recovery

Vendor migration `000099_commerce_refund_policy_decisions` stores immutable trigger, policy/version, allocation scope, component evidence, support/Wallet references and projection completion. The existing payment-recovery worker:

1. reconstructs a missing obligation after a crash from persisted unavailable-line, variable-finalization or delivery-failure evidence;
2. releases exact AUTO_REFUND Inventory stock before requesting money movement;
3. reads back or retries Wallet with the same immutable key;
4. resumes unresolved Support handoff and variable financial projection; and
5. projects `REFUND_PENDING`, `UNKNOWN`, `PARTIALLY_REFUNDED` or `REFUNDED` only from Wallet state.

Order lifecycle and financial state remain separate. A cancelled/rejected order does not become operationally resurrected when its refund changes state.

## Security and surface

- No new Vendor or customer route was added; the fully enabled Vendor route count remains 816.
- Existing Vendor/customer tenant-scoped routes remain the only entry points. No arbitrary amount or allocation identifier is accepted by an automatic-refund API.
- Wallet and Support calls use existing HMAC v2 service authentication. Support receives bounded references/reason codes and no money instruction; creating a case does not move money.
- Dispatch and Hauling own provider identity and completion. Customer Service and Taxi are unchanged.

## Verification status

Unit, contract, migration-shape, HMAC-client and affected package tests cover policy classification, funding conservation, overflow rejection, allocation-local recovery, provider preservation, exact support handoff and service-owned provider settlement. PostgreSQL migration replay and race/conservation cases require `VENDOR_SERVICE_TEST_DATABASE_URL` and `PAYMENT_WALLET_TEST_DATABASE_URL`; certification must report them as infrastructure-blocked when those variables are unavailable.
