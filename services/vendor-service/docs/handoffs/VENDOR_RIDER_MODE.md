# Vendor Rider Mode frontend handoff

This is a frontend contract only. No Vendor Rider frontend is implemented in
this repository.

Use the authenticated Vendor staff session. Never ask for or send a Dispatch
provider identity. The Rider can set only `OFFLINE` or `AVAILABLE`; `BUSY` and
`SUSPENDED` are authority-owned states. Send `If-Match` and `Idempotency-Key`
for every write and treat a replayed success as the same operation.

The Rider surface provides availability, mission list, current mission, a
single assigned mission and controlled actions. Render the ordered stages;
do not expose an arbitrary status editor. Mission and Parcel identifiers from
another Rider, branch or business resolve as not found.

Pickup, receiver completion and return are secure-handover actions. The current
Vendor-compatible handover authority is dependency-gated, so hide or disable
those controls unless capability discovery explicitly enables them. A 503
`VENDOR_HANDOVER_UNAVAILABLE` is not proof of pickup or delivery and must never
be converted into an optimistic local success. Do not collect, store or log a
raw OTP/QR credential in the Vendor UI.

Partial pickup may return the mission at the pickup stage while selected
Parcels move into custody. Refresh the mission and Parcel list after every
action. Receiver-unavailable is a distinct state and requires a bounded reason;
return uses the same Parcel identities and an authoritative return handover.

Hybrid fallback is an operator action, not a Rider shortcut. Show its durable
state and linkage to the new plan/PlatformMissionRef. Once any Parcel is in
Vendor Rider custody, clean fallback is forbidden; direct the operator to the
supported return/support path. Cross-provider transfer remains dependency-gated.
