# Cloud-kitchen workspace

Entry: More ? Cloud kitchen. Bulk packages, corporate plans and kitchen defaults use existing public cloud-kitchen routes; capacity and linked production work use the existing production workspace. No backend endpoint or business rule was added.

## Flows

- Kitchen settings: choose a branch with an active CLOUD_KITCHEN capability, read backend defaults/version 0, then save threshold, lead time, reservation hold, horizon and an actual branch/capability capacity-pool reference with the current version.
- Bulk packages: create, detail/edit DRAFT or ACTIVE configuration, archive, actual branch catalogue item/variant selection, component removal and non-overlapping quantity bands. Band edits/archives carry both actual package and band versions; component changes also update the package version.
- Capacity preview: positive quantity and optional production timestamps. Display matched band, lead-time satisfaction, required workload and the server capacity decision. This read creates no reservation, order or payment.
- Corporate plans: existing active package for the actual branch, corporate profile reference, branch timezone, recurrence/ISO weekdays, local time, effective range, quantity, duration, lead time and horizon. Create remains DRAFT. Edit is available only for DRAFT/SUSPENDED; activate, suspend, resume and archive follow the backend transition map.
- Date exceptions: SKIP, RESCHEDULE, QUANTITY_OVERRIDE or PACKAGE_OVERRIDE, using calendar dates. They affect subsequent worker materialization, rather than silently rewriting an already-created occurrence. The endpoint has a success-only response and no public exception-list route; the UI does not invent one.
- Occurrences: read actual state, window, quantities, errors and attempt/replay evidence. Retry capacity conflicts, replay dead letters, reschedule PLANNED/CAPACITY_CONFLICT, and request cancellation with a bounded reason. The server owns started-production restrictions and linked reservation/job cancellation. Genuine linked jobs/reservations open the existing screens.

## References and contract boundaries

The authenticated `/cloud-kitchen/corporate-references` bridge explicitly maps `corporate_profile_ref` to the CRM corporate profile ID exposed as `corporate_ref`. It requires kitchen management and CRM corporate-read access for the chosen branch, returns only active eligible relationships, and supplies labels with offset pagination. New or changed plan references are checked at the handler and locked/rechecked in the write transaction. Unchanged historical references may be retained when editing an existing plan; they are labeled as saved rather than presented as verified active relationships.

`/cloud-kitchen/context/{branchId}` returns the actual business currency under kitchen-read scope. Pricing uses decimal inputs for currencies whose two-digit convention the app supports; other codes retain explicit integer minor-unit entry. Amounts are not converted between currencies. The package page can edit already saved logistics keys with a type-preserving full-screen editor, review and the existing versioned/recoverable package update. Maps are edited recursively; arrays/null remain read-only and preserved.

The backend still declares no nested logistics vocabulary or operational consumers of those keys. Creating NEW typed logistics configuration remains undefined; the app does not invent delivery modes, destinations, fees or worker behavior. This is the remaining portion of tracking item 1. No migration or new logistics execution has been added.

List routes return arrays and do not support invented pagination/filter query parameters. Status chips filter the loaded collection locally. References come from actual public branch, catalogue and capacity reads; denied reads block editing rather than providing placeholder reference IDs. Server scope/permissions remain authoritative.

## Recovery, Mock and validation

Writes persist a mode/vendor/staff-scoped command with frozen fields, versions, references and idempotency key before sending. Retry reuses the accepted command. DELETE component operations retain their required version body. A saved unconfirmed command blocks another write. Successful date-exception responses without data are acknowledged only for that exact endpoint.

Mock adds an active cloud-kitchen capability, a shared production capacity pool, package/components/band, corporate plan and sample planned/conflict/dead-letter occurrences. Capacity preview uses the shared Mock production model; overlap, reference scope and version checks remain explicit. Sample dates are fixtures rather than certification of an IANA recurrence worker. New plan activation does not run a background materializer. Retry/replay returns an occurrence to PLANNED; it does not invent a successful reservation. No real orders or payments are created.

Focused validation: test/cloud_kitchen_workspace_test.dart covers settings first-write versions, package/band CAS and overlap refusal, duplicate-safe creation, capacity preview, plan transitions, success-only exceptions and occurrence controls. Phone-width theme validation is tracked in VALIDATION.md. Actual deployed permissions, timezones, recurrence workers, capacity reservation conflicts and final device verification remain pending.

Sources: services/vendor-service/internal/features/cloud_kitchen/http.go and service.go; production capacity/authorization handlers; sections10-11-production.openapi.json and embedded generated route contracts.

11 October 2026 focused check: shared Mock CRM reference/currency/plan validation test passed; Vendor cloud-kitchen and cmd packages compile. Contract verification covers 742 routes. Deployment and database/device checks remain item 5.
