# Vendor-owned and Hybrid delivery runtime

Vendor Rider identity is an active `vendor_staff` record with the restricted
`role_vendor_rider` assignment and branch-scoped
`vendor.delivery.execute_assigned` permission. It is not a Dispatch provider
identity and creates no marketplace-provider earning.

The Vendor fulfilment aggregate remains canonical for `VendorMission`, Rider
availability, assignment history, runtime stage and Parcel custody projection.
One active assignment and one Rider current mission are protected by database
constraints. Commands use optimistic versions, operation fingerprints and
post-lock read-back for simultaneous retries.

Custody-moving actions call the opaque handover authority before the database
transaction. Only authority-returned references are persisted. With the
current `PARTIAL` platform capability, production leaves that port unwired and
pickup, delivery and return fail closed.

Pre-custody Hybrid fallback is a durable saga:

1. Lock the VendorMission and record a unique fallback operation.
2. Quote the replacement through the accepted Dispatch Section 28 adapter.
3. Append a new immutable FulfilmentPlan version and platform slot.
4. Ask Wallet to atomically reclassify the funded delivery obligation.
5. Create/read back the Dispatch mission through the existing authority.
6. Persist the PlatformMissionRef linkage and retire the old VendorMission.

The same CommerceOrder, VendorOrder, FulfilmentGroup, FulfilmentUnit and Parcel
identities survive. A mission lock serializes fallback against pickup; any
existing custody event blocks clean conversion. Post-custody takeover remains
dependency-gated because no cross-authority Vendor-to-Dispatch transfer is
currently approved.

Wallet remains the sole ledger authority. The immutable checkout allocation is
retained as original evidence. Effective payables move from Vendor delivery to
the platform provider, any higher operational cost is funded by the platform,
and customer contribution and original subsidy attribution are not rewritten.
A cheaper replacement fails closed into the existing Section 24 customer
benefit/refund authority rather than creating a second refund engine.
