# Provider app handoff: Commerce delivery

## Reliability and replacement status

Critical Booking/Trip notifications now retry from durable intents. A queued or unknown send is not delivered; clients must render current authorized domain state rather than count notifications. No OTP/QR secret is included in generic lifecycle events.

The existing assignment inbox may return `purpose=PROVIDER_REPLACEMENT` and `incident_ref`. Accepting such an offer returns the existing Trip and `REPLACEMENT_READY`: it does not transfer custody, authorize completion, or start a second Trip. Continue through the established provider-transfer challenge/scans/evidence flow. Recovery exhaustion requires Operations review; clients cannot force a provider or financial outcome. No frontend code was implemented in this closure.

Commerce jobs appear in the existing public Dispatch marketplace. They use the
same offer ranking and acceptance flow as other public jobs; the provider app
must not label them as company/Fleet private assignments.

## Job card

Show the mission reference, service class (`FOOD` or `SHOP`), route-aware
distance/time, pickup count, promised-by time, package count, total weight and
bounded handling flags. Show one ordered card per logical pickup with:

- sequence and Vendor/branch display;
- address and ready-from/ready-to window;
- parcel count and parcel references;
- stop manifest summary and handling warnings;
- pickup state and handover state.

Two Vendors at the same coordinates are still two cards. The app may visually
group the location, but must require both independent handovers and must never
merge their manifests.

## Pickup flow

Only the next stop may be actioned. On arrival, POST the provider operation and
current mission version to the stop `arrive` route. Dispatch returns the
short-lived pickup credential for the Vendor handover. The Vendor-facing side
submits that credential through the internal verification route; the provider
then completes the stop. Completion before verification, an expired code,
too many attempts, a replay with changed data, or an out-of-sequence stop is a
fail-closed error.

Treat idempotent replays as success and replace local state with the returned
canonical mission. Never invent or cache a new parcel identifier.

## Receiver and proof

After the final pickup, the existing Dispatch trip starts with the established
delivery credential. Customer verification-code and provider trip completion
flows remain authoritative. Recipient confirmation/proof belongs to the
existing trip domain; Commerce does not create a second proof or OTP system.

If the receiver is unavailable, use the Commerce mission command and display
the returned canonical state. Operations may then reschedule or require a
return. Do not mark delivered locally and do not ask Vendor Service to decide
the runtime state.

## Failure and replacement

Before any pickup, replacement returns the job to normal matching. After
custody, replacement is transfer-safe: show `TRANSFER_REQUIRED` until the
canonical state changes and retain the exact parcel list. Return jobs show the
original parcel references and the recorded return leg. Dispatch event
delivery drives customer/vendor projections; the provider app must not infer
refunds, Vendor payable or settlement from a runtime failure.
