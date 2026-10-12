# Purchasing workspace

Open More > Purchasing and select an accessible branch. Mock is the default; Live uses the configured Vendor server and signed-in session.

## Implemented flows

- Supplier categories, suppliers, contacts, terms and supplier products, including supported editing and archiving.
- Draft purchase orders with actual supplier-product references, quantity constraints and currency; submit, approval/rejection, ordering, cancellation and closure follow backend states.
- Approval history and policy configuration. Self-approval restrictions and required step-up verification remain enforced. Verification uses purchasing.purchase_order.approve.
- Record deliveries, inspect delivered/accepted/damaged/rejected/short quantities and wrong items, then explicitly post accepted stock. Reversal requires a reason. Inspection does not post stock.
- Discrepancy approval/rejection/resolution, receiving policies, outstanding deliveries, alerts, supplier performance and cost evidence.
- Receipt document attachment through an existing Media reference. This does not provide a new public upload authorization flow.

## Contract and recovery

Requests use existing public purchasing routes, exact backend fields, current versions and supported If-Match headers. Mutations with idempotency support persist the original request before transmission and retry that same body, key and version after reopening. An unresolved saved change pauses other purchasing mutations.

Alert acknowledgement/resolution has no contractual idempotency guarantee: recovery reads the alert status and never automatically repeats the POST. Unknown outcomes remain blocked pending confirmation.

Approval policy reads now include the additive rules_complete indicator. Branch-filtered policy reads hide rules, so the app permits replacement only when the backend confirms full policy visibility. Deploy the accompanying vendor-service change to enable editing existing policies in Live. The server still authorizes every action.

## Integration boundaries

Supplier products reference generic production inventory, not Shop stock. Live product setup requires actual inventory item references; production master setup is the next workspace. Existing Media references must resolve through the deployed integration.

Mock supplies independent ingredient balances and purchasing fixtures for UI exercises. It is not a database transaction, worker, accounting or concurrency simulator. Controlled tests do not certify deployed permissions, OTP delivery, inventory accounting, Media availability or multi-rule approval behavior. Live integration remains pending.
