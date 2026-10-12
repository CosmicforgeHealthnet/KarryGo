# Delivery decisions and authority overrides

Open More > Vendor deliveries > delivery > Packing & custody > Delivery decisions.

The workspace reads the existing fulfilment-group orchestration and decision history. It displays server authority, product code, pickup timing, transport requirements, execution state and audit events. Overrides select DISPATCH, TAXI or HAULING, with an optional product code and required reason. No provider selection, pricing or dispatch controls are added.

The public routes are GET /api/v1/vendor/fulfilment/groups/{groupId}/orchestration, GET the same path plus /decisions, and POST the same path plus /authority-override. POST contains only authority, product_code and reason.

Reads require vendor.orchestration.read and actual branch access. Backend handlers now scope pickup evidence to the signed-in vendor and permitted branches. Overrides require vendor.orchestration.override, ownership of every pickup and access to every affected branch. Existing execution, mission references and terminal states prevent overrides. The service remains responsible for authority eligibility.

The app refreshes before submission and refuses a changed review version. The POST contract has no client version or idempotency guarantee. Before submitting, the app saves a recovery journal. After an uncertain response it only reads history; it never resends automatically. A new matching OVERRIDDEN audit event confirms acceptance and clears the journal. If no matching event exists, the outcome remains unresolved and controls stay blocked pending confirmation. This does not eliminate the read-to-submit race inherent in the public contract.

Mock defaults include committed groups and an editable fourth group. Mock records audit events but does not simulate logistics serviceability or worker execution. Live needs the updated vendor backend, enabled orchestration and appropriate permissions. Missing plans, disabled service and denied access are explicit states. No deployed or database-backed integration is certified by local checks.
