# Preparation and vendor delivery

## Navigation

Orders ? open an order ? Preparation and ready time. More ? Vendor deliveries ? View delivery. Details use full screens; preparation changes and failure reporting use focused sheets followed by review and animated feedback.

Mock remains the default. Both screens capture the current VendorApp and use the same generated APIs in Mock and Live. Mock data belongs to the selected business and resets with the mock client. No Live read silently falls back to Mock.

## Preparation: eight routes

GET readiness and history; POST start, estimate, ready-soon, ready, delay and block. Start requires the actual order parent/branch and a selected active business type read from that branch's capabilities. The client never equates FOOD with RESTAURANT or manufactures a readiness record after a generic 404: start is offered only for READINESS_NOT_FOUND. Unmounted/disabled routes show the server error instead.

Readiness has its own readiness_version and state machine. Advance commands send that version as If-Match; start has no version header, matching the handler. Category-specific substates mirror substates.go. Delay/block require a reason. Estimate moves to PREPARING and is offered only where the server transition table permits it. Ready clears activity detail. A failed/uncertain write blocks further changes until refresh. First-ready history survives corrections. Readiness updates do not change commerce order lifecycle status.

The service owns estimate bounds and predictions. Mock applies the default estimate bounds, records transitions and preserves first-ready time; it does not simulate historical prediction confidence or the orchestration event pipeline.

## Delivery navigation

- More ? Vendor deliveries ? View delivery: mission details, eligible rider assignment, Packing & parcels and Platform fallback.
- More ? My rider workspace: assigned missions, availability and rider progress.
- Packing & parcels: unit readiness, parcel packing, manifest editing, custody history and returns. Verify rider pickup appears for the linked assigned mission when it is ready for pickup; the order must also be ready.
- More ? Branches ? open a branch ? Delivery policy.

## Assignment and verified rider actions

Assignment reads active staff and their actual rider role assignments, including branch scope. The server checks rider availability. Rider actions follow the backend stage table and send the canonical mission version. Counter pickup uses the existing order handover verification; completing rider pickup then confirms custody. Delivery and return require an issued evidence reference and its OTP or QR credential. QR values are entered manually; camera scanning is not implemented. Arbitrary proof references are never submitted.

Writes persist their original request and idempotency key before submission. Uncertain retries reuse that request. OTP/QR credentials are never persisted: secure retries require re-entry of the original credential. Mock and Live recovery storage are isolated. Server permissions, feature flags and verification authority remain authoritative.

## Packing, custody and returns

Workspaces use actual mission group, unit, parcel and order identifiers. Packing selects real order lines and quantities. Manifest edits preserve existing metadata; sealed manifests are read-only. Unit readiness, parcel custody events, group plans and return records are separate backend projections. Creating a parcel return does not fabricate a completed physical return.

## Policy and fallback

The branch policy editor covers enabled state, delivery mode, currency, core fees, limits and ETA settings. Its Advanced delivery settings screen edits service zones, consecutive distance tiers, IANA operating timezone, local-clock delivery windows (including overnight periods), supported vehicle classes, policy rider availability and hybrid fallback triggers. Individual rules use sheets; Use these settings changes the local draft, and Review policy submits one versioned PUT after confirmation. Platform-funded configuration remains read-only because the public vendor route cannot authorize platform spending.

Distance ranges include their lower bound and exclude their upper bound; zero upper distance means unbounded and is allowed only on the final distance tier. Clearing vehicle classes removes the policy's class restriction; clearing delivery windows removes the policy's hour restriction; clearing fallback triggers permits all supported bounded reasons in Hybrid mode. None of these settings expands the branch service area or authorizes dispatch. Zones use references understood by the existing location authority; the app does not invent polygon or map-boundary APIs.

Optional package/radius limits can now be cleared explicitly. Mock supplies no-typing rule defaults and replays accepted policy updates with their original idempotency key after a lost response. Live uses the same existing public contract, with server timezone validation and actual eligibility/funding authority. Mock does not simulate the complete location, timezone database, dispatch or pricing pipeline.

Fallback submits a supported reason and tracks the actual operation reference. Recovery queries a pending operation before repeating a write. Financial transitions belong to the server; the app does not synthesize balance movements.

## Mock fixtures and Live boundaries

Mock includes a dual-role demo account, eligible rider candidates, packing fixtures and preverified pickup authority. These fixtures make the paths testable without claiming to simulate the complete Dispatch or financial pipeline. Live never substitutes these fixtures.

A rider account without branches.read cannot discover branches to create its first availability record through the current branch selector. Live verification also requires issued handover references and configured verification authority. These backend access/setup boundaries are surfaced rather than bypassed.

Orchestration decisions and permitted authority overrides are implemented; see orchestration.md. Variable-quantity finalization remains separate work. This increment does not complete the entire frontend.

Validation: 15 focused delivery/fulfilment/policy/fallback and existing mission tests passed, including controlled HTTP retry recovery and phone-sized dark layouts. An earlier impacted settings/rider/mission run passed 28 tests. A deployed Live end-to-end run remains outstanding.

Advanced policy increment: six focused advanced/policy/fallback checks passed, including identical recovery after reopening, explicit restriction clearing, invalid-range refusal before submission, no-typing Mock zone creation and 360×800 dark layout. Analysis of the changed files passed with no issues. No backend route or DTO change was needed.
