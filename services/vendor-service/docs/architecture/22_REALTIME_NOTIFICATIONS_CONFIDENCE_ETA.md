# Realtime notifications and confidence ETA

## Ownership

Vendor Order and Fulfilment projections emit semantic events through the
existing Vendor transactional outbox. Dispatch owns routes, Commerce missions,
providers, stop progress and its receiver credential. Production owns Food
preparation and Shop picking readiness. Notification Service owns templates,
preferences, delivery attempts, retry/dead-letter state, device tokens, email,
push, in-app delivery and the existing WebSocket hub.

Business services never select a raw destination or send a transport directly.
They submit an opaque recipient reference, controlled event/template key,
source aggregate/event/version and bounded non-PII data. Notification Service
deduplicates both the request idempotency key and semantic source tuple.

## ETA policy v1

`commerce-confidence-eta.v1` deterministically composes conservative earliest
and latest durations for:

1. Food preparation or Shop picking readiness;
2. provider search/assignment-to-pickup uncertainty;
3. manifest verification and loading at remaining pickup stops;
4. additional multi-Vendor stop handling; and
5. the Dispatch route-evidenced receiver road leg.

No straight-line or frontend-supplied distance can become the final authority.
Missing route provider, evidence version, observation time or positive route
duration fails closed to no new ETA. The accepted FASTEST/BALANCED/SAVER
promise remains the SLA boundary; the prediction never rewrites it.

Confidence is LOW before assignment, MEDIUM after assignment and HIGH only
after custody with fresh route evidence. Stale routes, reassignment and
operational exceptions reduce confidence to LOW. Delivery records an ACTUAL
outcome. Cancellation invalidates normal arrival ETA; return states remain
separate.

## Versioning and realtime

Every material semantic input creates one immutable ETA version per mission.
A deterministic fingerprint suppresses equivalent retries, while the current
pointer is moved in the same transaction as ETA events. Plan/source fences
reject stale same-authority updates and old-provider updates. `commerce.eta.updated`
is an in-app/WebSocket read-model hint. Meaningful delay transitions also emit
`commerce.delivery.delay_detected`; tiny changes do not create push spam.

Customers read current mission ranges through the existing scoped order ETA
route. Vendors receive only ETA attached through VendorOrder/Parcel scope.
History retains component provenance for support and reliability analysis but
is not a public mutation surface.
