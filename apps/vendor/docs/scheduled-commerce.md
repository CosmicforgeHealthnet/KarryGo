# Scheduled-order management

Implemented on 10 October 2026. Navigation: Orders ? Scheduled orders ? Scheduled or Recurring ? select a record. Mock remains the default; Live uses the same generated contracts without fallback.

## Backend support added

Four existing-data reads under /api/v1/vendor/commerce/section21:

- GET /scheduled-orders and GET /scheduled-orders/{id}.
- GET /recurring-schedules and GET /recurring-schedules/{scheduleId}.

Reads require vendor.orders.read and apply authenticated vendor plus branch scope in SQL. Lists accept branch_id, limit (default 50, maximum 100) and offset, with items/pagination. Cross-scope list filters are refused; detail queries cannot reveal another vendor or branch's record. The existing COMMERCE_BULK_RFQ_ENABLED flag still controls mounting. No migrations, payment controls or new lifecycle states were added.

Occurrence reads now use the same read authorization. Recurring updates check permission on both the existing and requested branch. Existing scheduling commands still require orders.manage. Exported Go response field casing is preserved. The embedded request models and OpenAPI bodies now describe actual ScheduledOrderInput/RecurringScheduleInput fields instead of empty objects. Scoped regeneration also updates manifest additions/counts.

## App journeys

Scheduled: paginated list, upcoming/past/cancelled/all filtering of loaded records, delivery window, production/expected ready times, capacity status, linked commerce order and reviewed cancellation.

Recurring: active/paused/all filtering, meal time/timezone/weekdays/headcount, dates, advance horizon, occurrence history, editable meal timing/headcount/days/horizon/dates, reviewed pause/resume/end/cancel. Edits preserve actual requester/corporate/branch references, destination, menu rotation, capacity requirements, allowance and vehicle class. They send the current version in If-Match. Status changes send expected_version. Terminal schedules are read-only. Date edits follow the current backend validation, including the current start-date requirement.

Full screens handle lists/details/editing; focused sheets review consequential changes; animated feedback reports success/errors. First reads use skeletons, while command spinners are confined to saving. Colours adapt to light/dark mode and distinguish cancelled/paused/active states. Stale or uncertain writes block further edits until authoritative refresh; they are never blindly retried with a fresh version. A schedule cancellation is not proof of a commerce refund. Occurrences/proposals are not proof of customer approval/payment.

## Remaining integration boundaries

Existing-schedule management and creation from a saved corporate-plan context are implemented; this is not every Section21 operation. Standalone scheduled creation requires a genuine customer-approved checkout quote and customer/capacity context. First-time corporate schedule creation requires genuine corporate/requester discovery and destination/menu/capacity configuration. Vendor RFQ quotation needs actual RFQ discovery. These creation/discovery journeys remain pending; the app does not expose raw-reference entry forms or manufacture identifiers to disguise them as complete.

Customer cart/quote approval/payment, customer RFQ acceptance/conversion and group-organizer/participant actions remain excluded. Workers generate occurrences/proposals; there is no frontend Run job action.

Mock contains scoped schedule/occurrence fixtures and versioned management commands. It does not simulate customer checkout, paid orders or production workers. Existing occurrence reads return an array, capped at 100 by the backend; they are not a paginated full-history contract.

## Validation

Four focused Flutter tests passed: list pagination/cancellation, recurring edits/status/version conflicts with metadata preservation, accepted-write refresh recovery, and 360?800 dark layouts. Focused Go scheduling authorization/input tests and production route inventory/documentation checks passed. Contract verification passed with 728 generated Vendor operations. A deployed Live scheduling run and database-backed scope integration remain outstanding.

## Corporate schedule creation - 11 October 2026

Navigation: Orders > Scheduled orders > Recurring > New meal schedule. Select a scoped existing corporate meal plan, read its authoritative detail, review its saved corporate account/destination/menu/capacity settings, and set the new meal quantity, weekdays, meal time, planning horizon and dates. Required capacity configurations get an explicit start/end window editor. Pool allocations are preserved; quantities are not silently scaled with headcount. The real createRecurringSchedule POST is used in both modes. No backend changes were needed for this flow.

Before submitting, save the exact creation body with its IdempotencyKey in a mode/origin/vendor/staff-scoped journal. Lost confirmations restore the frozen body/key and prevent another creation until retry or a definitive refusal. A successful creation opens the new schedule detail. Mock creates an ACTIVE version-1 record and replays the original key without duplication; it does not generate worker occurrences or paid orders.

This flow starts from an existing corporate context. Empty accounts do not get invented requester, destination or menu identifiers: first-time corporate setup remains pending. Scheduled checkout-quote creation and vendor RFQ discovery/quotation remain pending. Fixed recurring capacity-window semantics in the existing worker require deployed validation; local creation does not certify future reservations.

Five focused scheduling tests passed, including lost accepted creation recovery, preserved corporate/menu/destination/capacity context, management reconciliation and the existing small dark-phone layout check. New creation screens still need final physical-device validation.

## Vendor RFQ quotations - 11 October 2026

Navigation: Orders > Scheduled orders > Quotation requests. Open/All filters apply to loaded records; load more for additional pages. Details show actual servings, requested time, budget, deadline and saved destination/menu/packaging/delivery/capacity requirements.

Create a quotation for an active branch with a currency amount, future expiry and optional typed production capacity. Customer-required capacity cannot be switched off by the form. Choose genuine active branch/capability pools, capacity units and start/end times; review in a sheet before submission. Capacity units are not inferred from servings. A quotation is not a capacity reservation, accepted customer order, payment or conversion.

Existing branch quotations use Revise quotation; another branch can create its own quotation. Saved Snapshot is preserved and previous revisions remain visible. Typed vendor/payment/deposit/cancellation policy authoring and optional production-line/item-family editing remain outside this editor because the generic maps lack defined nested fields. Returned policies remain readable.

Exact body, RFQ, branch and body idempotency_key are stored before submission in a mode/origin/vendor/staff-scoped journal. Unknown outcomes block new quotations; retries reuse the frozen original. Known refusals require authoritative refresh. The server checks request state/deadline before replay; closed requests may require verifying readable quote history instead.

Three new vendor reads require orders.read and explicit invitation membership. Quote history additionally enforces staff branch scope and excludes competitor quotes. Deploy the updated vendor service for Live discovery. Branch/production registry permissions are composition dependencies. Mock supplies request/revision fixtures and duplicate-safe submissions, without customer acceptance, reservation, checkout or workers. Approved-quote scheduling is described below; first-time corporate setup remains pending.

Focused RFQ recovery/history, five existing scheduling tests, backend read-access/Section21 checks and contract verification passed. Final new-screen phone/layout and Live database/access checks remain pending.

## Approved-quote scheduling - 11 October 2026

Navigation: Orders > Scheduled orders > Scheduled > Schedule delivery. Select a current customer-approved quote for an authorized business/branch. The quote supplies actual customer, branch, delivery address and own-vendor line references; identifiers are not entered manually. Discovery is paginated and detail is reread before setup.

Full-screen setup handles desired delivery, optional delivery window, production-ready deadline, expected readiness and suggested vehicle. Optional capacity uses genuine active branch pools with one capability, explicit units and a start/end window. A review sheet precedes submission. The backend remains the approval/reservation authority; schedule acceptance does not assert payment, order conversion or dispatch.

Exact body and IdempotencyKey are journalled before submission. Recovery retries the frozen command; another creation is blocked while its outcome is unknown. Success opens the created schedule detail. Mock provides a distinct approved-quote fixture, creates the actual scheduling state, uses shared production capacity and releases it on cancellation. It does not implement customer checkout/approval or workers.

Two vendor reads expose only unexpired approved quote contexts containing the authenticated vendor's scoped branch lines. Other-vendor lines are excluded. The creation service now checks the same vendor/branch binding after authoritative quote lookup. Additional production-read permission is needed when configuring capacity. Live discovery requires deploying the updated vendor service. Backend quote expiry checks still apply before command replay, and final database-backed/deployed scope and device checks remain outstanding.

First-time corporate setup is implemented below with a backend-defined fixed-menu context. CRM contacts are profile relationships and CRM locations are not delivery addresses. Arbitrary rotating-menu authoring remains outside this fixed-menu flow.

Seven focused Flutter checks passed across scheduling creation, management and RFQs, including lost accepted creation recovery, no duplicate reservation, cancellation release and foreign-business refusal. Backend approval/branch projection/read-scope tests and contract verification passed (734 operations).

## Recurring worker date and capacity correction - 11 October 2026

Required capacity windows are templates relative to the schedule's StartsOn calendar date. Each generated occurrence shifts both window endpoints by the calendar-day offset, preserving local clock times in the schedule timezone, allocations and other saved evidence. The occurrence snapshot and the actual production reservation use the same resolved window. Cross-branch capacity and nonexistent daylight-saving clock times fail before inserting the occurrence; the schedule template is not mutated.

Worker start/end boundaries now interpret stored database DATE values by their calendar components and iterate from local midnight, rather than converting UTC midnight or truncating absolute 24-hour periods. This preserves the first and final local schedule dates in timezones west of UTC and across daylight-saving transitions.

Only the focused TestRecurring backend checks were run and passed. Mock does not run the recurring worker. Deploy the service for the corrected Live behavior; database-backed worker verification remains pending. This correction does not complete first-time corporate customer/destination/menu authoring.

## First-time corporate meal setup - 11 October 2026

Navigation: Orders > Scheduled orders > Recurring > New meal schedule > First-time meal setup. Select an eligible corporate account and approved food menu, configure timezone/allowance/vehicle, choose days/time/headcount/date range, optionally select actual production pools and units, and confirm once in the final review sheet. Mock supplies a linked corporate/quote fixture and prefilled example timezone/headcount; Live reads the new backend context endpoint. Existing-plan reuse remains available.

GET /commerce/section21/recurring-setup-contexts requires orders.read plus vendor.crm.corporate.read. The service projects current customer-approved quotes containing only this vendor/branch's FOOD lines. The corporate account must be ACTIVE, visible in the staff's CRM scope and applicable to the delivery branch. It must have an active unsuppressed contact whose active privacy-safe canonical CUSTOMER profile customer_ref matches the quote customer. CRM profile IDs never become RequesterID, and CRM organizational locations never become delivery addresses. Pagination scans quote pages: use next_offset rather than the number of returned corporate contexts; empty pages can still have more quote pages.

The backend defines MenuRotation schema_version=1, mode=FIXED, source_quote_id, quote_version, currency and lines. Lines are exact checkoutquote.LineSnapshot values for the authenticated vendor/branch; DestinationSnapshot is the exact checkoutquote.AddressSnapshot. These are immutable evidence carried unchanged into each occurrence proposal. Headcount does not rescale quoted line quantities or pool allocations. There is no arbitrary weekday rotation editor, new address authority, automatic purchase, customer approval or payment in this flow.

Creation checks the current relationship and rereads the approved quote, rejecting altered menu/address/customer bindings. Reuse of an existing validated immutable plan context is supported after the original quote expires, provided the corporate relationship remains eligible. Typed menu/address/account/customer/branch evidence cannot be replaced through schedule editing. Original body/key are journalled before POST. Replay checks staff manage scope and saved business/branch/corporate/customer binding before returning the saved schedule, even after quote expiry; concurrent key conflicts also check those bindings.

Mock fixtures represent the linked relationship locally; the Mock scheduling service does not run the Live CRM database join or recurring worker. It preserves immutable snapshots, refuses altered new contexts and supports duplicate-safe recovery. Focused creation/recovery/tampering checks, existing recurring workflows, backend projection/scope/large-amount/date checks and contract verification passed. Deploy the updated service for Live setup. Database-backed join/privacy/permission validation and physical-phone verification remain pending.

StartsOn/EndsOn represent calendar dates, not instants: the editor sends UTC-midnight calendar components and the backend validates/stores those components in the selected meal timezone without shifting them to the previous day. Worker execution times remain timezone-aware instants. This also prevents a phone timezone from silently moving a chosen schedule date.
