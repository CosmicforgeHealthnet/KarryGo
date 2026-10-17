# Added Dispatch provider contracts

These routes are implemented locally; deployment is pending.

## Own-provider Wallet history

`GET /api/v1/payment-wallet/provider/transactions?page=1&limit=20`

Uses the existing provider bearer authentication. The server derives the provider identity from the token, never from query parameters. Page starts at 1; limit is 1-100. Results include only the signed-in provider's `provider_payable` ledger account in the configured default currency, ordered by creation time and ID descending.

Success data: `transactions`, `page`, `limit`, `has_more`. Each transaction contains `reference`, `transaction_type`, `side`, `amount_kobo`, `currency`, `memo`, `created_at`. This is ledger history, not an inferred payout settlement report or unsupported earnings analytics.

## Capture-bound handover image upload

`POST /api/v1/provider/trips/:id/stops/:stopId/evidence-content`

Existing provider bearer authentication is required. Multipart fields: `capture_session_ref` and `photo`. The file must be a detected PNG/JPEG, at most 5 MiB. The server checks assignment, current manifest/version, mission/stop, consumed challenge, active unexpired capture session, and PHOTO/SIGNATURE evidence kind before contacting Media with server credentials.

Success data: `media_ref`. Submit that reference through the existing stop evidence endpoint together with the capture session and actual captured timestamp. Upload alone does not confirm evidence acceptance or complete a handover. Identical active-session content may reuse an existing owned Media asset; evidence mutation retains its existing idempotency contract.

## Provider handover progress projection

The existing authenticated trip mission projection now exposes required photo/signature/parcel-scan flags, required/accepted evidence counts/types and expected/scanned parcel counts. Counts are scoped to the active manifest/version and handover stage, with parcel scans scoped to the stop. No verification credential is added to the read projection.

Database-backed assignment, scope and replay integration checks remain necessary before claiming deployed readiness.

## Assigned Commerce pickup read

`GET /api/v1/provider/commerce-missions/by-trip/:tripId`

Uses the existing dispatch-provider bearer middleware and Commerce rollout registration. Resolves the mission by trip and the authenticated current provider or the specifically assigned replacement provider on their replacement trip, returning 404 for an unassigned mission. A repeatable-read transaction and mission row lock keep assignment and stop/parcel projection consistent.

Success data contains `mission_ref`, `trip_ref`, `state`, `version`, `stops`, `parcels`, and `pickup_actions_allowed`. It excludes quote/plan, internal operation references and verification credentials. Pickup actions are disabled during replacement or terminal mission state. Existing arrive/complete commands remain authoritative and enforce exact stop sequence, version, sender verification, manifest and package count. Their `operation_ref` must equal the `Idempotency-Key` header.

## Commerce custody read additions

The assigned Commerce mission response also includes `returns`, `return_parcel_refs`, `transfer_parcel_refs`, `can_start_transfer` and an optional `transfer`. Return entries expose saved ID, product class, disposition, state and exact parcel references. Transfer status exposes reference, state, OUTGOING/INCOMING role, parcel references, package count and expiry; it does not contain a challenge code. Replacement riders cannot use current-provider pickup or return actions.

`GET /api/v1/provider/commerce-missions/:missionRef/provider-transfers/:transferRef/credential`

Uses existing provider authentication. Resolves only an active, unexpired transfer owned by the outgoing provider, returning the existing typed transfer credential. Responses use `Cache-Control: no-store`. The incoming provider must obtain the code from the outgoing rider and use the existing complete endpoint; only the assigned replacement can accept custody.

Return creation/completion and transfer start/completion continue to use existing provider POST contracts and matching operation/header idempotency references. The app obtains actual device coordinates in Live; only Mock uses fixed demo coordinates. Ordinary mission read results contain no verification credentials.

## Commerce exceptions and photo evidence

The existing assigned mission read now includes `exception_options` (reason code, server-permitted recovery actions, photo requirement), `exceptions` (saved status and recovery choice), and `evidence` summaries (type, parcel references, capture time, attachment count, description). Each history is capped at the latest 100 records. Alternative-receiver authority and commercial fee approval are not exposed as available rider actions.

`POST /api/v1/provider/commerce-missions/:missionRef/evidence-content`

Existing dispatch-provider bearer authentication applies. Multipart fields are `photo`, `operation_ref` and a JSON array in `parcel_refs`; the operation reference must equal `Idempotency-Key`. Accepts a decoded JPEG/PNG image header with at most 5 MiB and 40 million pixels. Validates active assignment (current or explicitly assigned replacement) and parcel membership before uploading through the backend Media client. Returned data is `media_ref`, bound to mission/provider/exact parcel scope. Existing same-operation assets can be reused only with identical checksum/scope; inactive assets return a processing message. Upload alone does not save evidence or change custody.

The form then calls existing `exceptions` or `evidence` POST contracts with that reference. Dedicated pickup/delivery/return/transfer evidence still belongs to its original workflow, rather than the generic evidence endpoint. Existing PICKUP_BLOCKED mission state now prevents provider pickup mutations, as well as disabling the corresponding UI controls.

## Approved hub custody

`GET /api/v1/provider/trips/:id/hub-hold`

Uses existing provider authentication, assigned-trip scope and the existing hub-hold rollout policy. Returns the approved `hub_ref`, `state`, active `manifest_ref`, exact `parcel_refs`/`parcel_codes`, and `allowed_stage`. Existing states APPROVED/EN_ROUTE/CUSTODY_PENDING permit ARRIVAL; HELD/PICKUP_PENDING permit RELEASE. Other states expose no mutation action.

`POST /api/v1/provider/trips/:id/hub-hold/evidence-content`

Provider-authenticated multipart upload with `photo`, `manifest_ref`, `operation_ref` and matching `Idempotency-Key`. Resolves the assigned approved hold and active manifest itself, validates JPEG/PNG content/size through the common image guard, and uploads a Media reference bound to that mission and exact parcel set. Reuses only an identical owned operation/checksum/manifest upload. App credentials never include Media service secrets.

The existing `hub-hold/checkpoints` endpoint remains authoritative for exact manifest scan matching, Media attachment authorization and state transition. The new UI requires a photo and full scan set before calling it; upload by itself does not record a checkpoint. Return and transfer workflows use the same existing Commerce evidence upload boundary for their optional photos.


## Offline replay consistency

Uses existing POST /api/v1/provider/offline-commands with authenticated provider identity and stable Idempotency-Key. App queues only LOCATION_FIX and EVIDENCE_REFERENCE; the latter returns PENDING_EVIDENCE_SYNC and requires delivery confirmation. Other custody and verification actions remain online.

Dispatch migration 000069_offline_location_atomic adds an optional unique device-command reference to location fixes. Accepted command recording and location ingestion are atomic; repeated command identities cannot duplicate the fix. Location history ordering and staleness use observation time bounded by receipt time. Apply migration before deploying this backend increment. Database-backed validation remains pending.


## Provider activity filtering

Existing GET /api/v1/provider/trips now supports optional created_from (inclusive) and created_before (exclusive) RFC3339 timestamps and status=active alongside existing concrete statuses. Bounds apply to created_at. Active excludes completed/cancelled/failed, matching the active-trip query. Provider scope and identical filter predicates apply to count and rows, before pagination; ordering uses created_at DESC, id DESC. Existing callers remain compatible. App date shortcuts calculate local boundaries and send UTC timestamps. Backend deployment is required for these filters; no new migration is needed for this increment.


## Supported payout-bank directory

GET /api/v1/payment-wallet/provider/banks uses the existing provider bearer middleware. Response data: {banks:[{name,code,currency}],country:"NG",currency:"NGN"}. Code remains a string. The current directory covers Nigeria/NGN/nuban payouts; it does not assert other-market support. Wallet fetches all cursor pages from Paystack and filters active/non-deleted banks; failure returns unavailable without a stale static or partial fallback. Client performs account-name resolution before saving through existing routes.

Source: [Paystack List Banks](https://paystack.com/docs/api/miscellaneous/#bank-list). Deploy the updated Wallet service before using the directory in Live; no new migration. App Mock directory is explicitly isolated from this endpoint's Live transport.


Provider trip route geometry (15 October)

- Existing provider-scoped active/detail trip responses now include optional route_pickup_polyline and route_dropoff_polyline, sourced from the already stored booking route fields after the trip ownership lookup succeeds. No new routing authority, database migration or client-side routing credential introduced.
- Flutter decodes the stored Google-format polyline with malformed-input/coordinate bounds. Live shows no invented road geometry when a route is absent. The Dispatch service must be deployed for these optional fields; routing configuration must already generate the stored geometry.
- Mock trip creation/state transitions preserve existing endpoint coordinates; demonstration paths and motion are explicitly labelled. The pickup scan simulation is locally guarded by Mock mode and never sends a verification command to Live.
