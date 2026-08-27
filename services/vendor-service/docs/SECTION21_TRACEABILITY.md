# Section 21 Traceability

| Requirement | Authority | Schema | Route/event | Verification |
|---|---|---|---|---|
| 21.1 scheduled/bulk acceptance | Orders + Production | `commerce_scheduled_orders` | `/vendor/commerce/section21/scheduled-orders`, `commerce.scheduled_order.accepted` | Orders/Production targeted tests; transactional reservation checks |
| 21.2 group ordering | Orders + CheckoutQuote | `commerce_group_*` | `/commerce/section21/groups/*` | group token, attribution and route parity tests |
| 21.3 RFQ lifecycle | Orders + CheckoutQuote + Production | `commerce_rfqs`, `commerce_rfq_quotes`, `commerce_rfq_quote_revisions`, `commerce_rfq_checkout_bindings` | `/commerce/section21/rfqs/*`, RFQ outbox events | revision immutability, current-revision acceptance, binding digest tests |
| 21.4 counter-offers/deposits/conversion | Orders + existing Wallet adapter | `commerce_rfq_payment_obligations` and immutable policy columns | counter-offer, conversion, status and cancellation routes | deterministic deposit-policy tests; targeted Orders/route/migration tests |
| 21.5 corporate recurring feeding | Orders + Production + durable jobs + Outbox/CRM projection | `commerce_recurring_schedules`, `commerce_recurring_occurrences`, `commerce_recurring_order_proposals` | recurring schedule/status/occurrence routes; recurring outbox events | timezone/weekday/idempotency tests; durable worker wiring and route parity |

Wallet remains the only payment authority. CRM receives recoverable relationship
signals through the existing outbox; it does not own corporate identity,
billing, payment, or order state. Group Orders remain organizer-paid.
