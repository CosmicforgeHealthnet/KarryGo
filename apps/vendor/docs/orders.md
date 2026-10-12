# Vendor Orders

The Orders tab reads the selected branch's latest 50 orders. Filters apply to that loaded set. Opening an order navigates to a dedicated full screen and reads its canonical detail. It stays open as the order advances, with explicit cancellation and rejection actions in the fixed footer whenever allowed. Closing a changed order reloads the dashboard and queue.

## Lifecycle

Actions follow `services/vendor-service/internal/features/orders/lifecycle.go`. The UI chooses a primary action for food preparation or shop picking and exposes other valid transitions as named action rows. Terminal orders have no lifecycle commands. Platform logistics pickup, handover and delivery transitions are excluded from vendor lifecycle actions; the separate pickup verification route observes the custody authority.

All lifecycle writes send a generated Idempotency-Key and the order version as If-Match. Identical failed-command retries retain the key. A conflict or uncertain transport result requires a fresh read before another action. A successful write with a failed read remains saved, with further actions disabled pending refresh. A 403 disables that operation without guessing role privileges.

## Shop picking

The picking-queue response uses the backend's PascalCase fields. Availability and quality commands use the picking outcome's Version, independently of the order version. Availability computes unavailable units from the ordered count and uses the backend's FULLY_AVAILABLE, PARTIALLY_AVAILABLE or UNAVAILABLE outcomes. Quality uses MET, PARTIAL, NOT_AVAILABLE or SUBSTITUTE. The backend remains the packing authority.

Replacement selection reads active branch listings and their product and variant details. It submits proposed_catalogue_ref and reason; the backend supplies authoritative replacement quantity, price and snapshot. The app does not approve suggestions on behalf of customers. The picker shows up to 50 active listings.

## Pickup verification

When the canonical order supplies a logistics mission reference and is READY, platform pickup verification reads the PICKUP observation. Only an ACTIVE challenge with VENDOR_SENDER as verifier accepts a credential. OTP and opaque QR credentials stay in the sheet's memory, are cleared after submission, and are not persisted. Unknown outcomes require a handover read before another attempt. A CONSUMED observation does not locally advance the order: authenticated logistics events own that progress.

Mock pickup code: `654321`. Mock opaque QR credential: `mock-pickup-credential`. These are separate from the sign-in code.

## Backend gaps

The actual-quantity endpoint requires the variable measurement row's version. Neither the public order detail nor picking queue exposes that measurement version or final measurement evidence. The app therefore does not submit a guessed version for variable-measure finalization. That flow needs backend read support before it can be implemented safely.

The picking queue also does not expose saved quality results. The UI acknowledges a successful quality write but does not invent a persisted quality checklist completion marker.

## Data modes

Mock is the default and persists order mutations within its current runtime. Reconnecting the data source resets its fixtures. Live uses the declared public Vendor APIs without mock fallback. No backend files were changed for this increment.

## Presentation

Order details and replacement browsing are separate screens with slide and fade transitions. Short confirmation, reason, availability, quality and credential tasks use focused sheets. Acceptance and terminal actions have acknowledgement dialogs; smaller updates use floating feedback. Mutation failures offer refresh recovery. Cancellation and rejection acknowledgements retain their ending-state colour. Reduced-motion preferences disable the custom motion. No visual progress implies unseen historical transitions.
