# Sections 10/11 Backend Handoff

Sections 10 and 11 are backend-complete. No frontend, Order, payment, Wallet, inventory-stock, dispatch, routing, fee, ETA, or delivery-handover authority was added. The machine-readable contract is [sections10-11-production.openapi.json](openapi/sections10-11-production.openapi.json), and the complete route/permission index is [API_ROUTE_REGISTER.md](API_ROUTE_REGISTER.md).

All routes are relative to `/api/v1/vendor`, require a Vendor staff bearer session, and are absent when the production feature is disabled. Reads use `vendor.production.read`, `vendor.pastry.read`, or the quote-read authority; configuration uses `vendor.production.manage`, `vendor.cloud_kitchen.manage`, or `vendor.pastry.manage`; production commands use `vendor.production.operate`; quote pricing uses `vendor.pastry.quote.manage`; quote approval uses `vendor.pastry.quote.approve` plus the established step-up grant.

## Shared production screens

A future capacity dashboard can render pools, production lines, recurrence-aware capacity rules, slot templates/generated slots, blackouts, reservations, utilization, jobs/job history, and durable operational alerts. Capacity preview is read-only. Holds, confirmation, release, partial release, and rescheduling all use the same server-side capacity engine; clients must never calculate availability as authority.

Every versioned mutation sends the latest positive `expected_version` (job transitions use a positive `If-Match`). Corporate occurrence retry, replay, reschedule, and cancellation advance the occurrence version. Treat `409` as reload/reconcile. Reservation or job responses may expose `source_ref` and `source_version`; display them as traceability metadata, not as a customer Order.

## Cloud Kitchen screens

The package builder manages Section 9 food-item/variant references, composition, active/archived quantity bands, integer minor-unit prices, lead time, workload, logistics metadata, and capacity-pool binding. Component, band, and package edits increment and snapshot the package version. A component reference is accepted only when the food item/variant belongs to the same Vendor and package branch.

Corporate-plan screens manage branch-authoritative IANA timezone recurrence, bounded horizon, lead time, exception dates, lifecycle, materialized occurrences, conflicts, retry/reschedule, dead-letter replay, and safe cancellation. Occurrence states are operational production truth. They are not Orders or delivery schedules.

## Pastry/Bakery screens

The catalogue supports product types, reusable option groups, ordered options, price/workload deltas, capacity class, product-option attachment, immutable product definitions, standard-batch capacity preview, and standard production jobs.

The custom-cake flow is:

1. Create a versioned request with opaque customer/corporate references, controlled risk flags, change/cancellation policies, and controlled logistics fields (`fragile`, `upright_required`, `cold_chain_required`, `temperature_class`, `package_size`, `weight_grams`, `spill_risk`, and bounded vehicle hints).
2. Append immutable specifications with bounded reference-media identifiers; use the diff endpoint to compare revisions.
3. Create exactly one active quote revision against the current effective request/spec version and optional shared capacity hold. `approval_required` defaults to `true`; when explicitly false the quote is issued directly as `APPROVED`. Revision inherits the approval policy, supersedes old commercial truth atomically, and does not double-hold capacity.
4. Approve where required, then accept with the latest quote version. Issue, approval, revision, and acceptance reject proposed, declined, stale, or superseded specifications. Acceptance atomically confirms capacity, freezes the quote/spec/logistics/risk/capacity snapshot, creates one production job and milestones, and creates one `PENDING_EXTERNAL_DOMAIN` conversion intent.
5. Track milestone history, risk, change eligibility, and cancellation eligibility. The backend re-evaluates cutoff/policy state in the mutation transaction; UI estimates are never authoritative.

Quote acceptance does not create a real Order. A future Order authority must consume the conversion intent and explicitly complete it before the request can truthfully become `CONVERTED`.

## Retry and refresh behavior

Every state-changing Sections 10/11 route requires an `Idempotency-Key` and uses the shared Vendor replay store. Reuse a key only for an identical method/path/body retry; another fingerprint or an in-progress duplicate returns `409`. Poll bounded lists after mutation unless a future event consumer is added. Operational alerts are durable and can be acknowledged/resolved with optimistic versions; they are not notification-delivery authority.

The corporate materializer, expiry/reconciliation workers, and alert worker use leases or durable dedupe, bounded retries, dead-letter evidence, audit, and transactional outbox records. Clients never run those workers and must not fabricate a successful occurrence, quote, or conversion when the backend reports a conflict.
