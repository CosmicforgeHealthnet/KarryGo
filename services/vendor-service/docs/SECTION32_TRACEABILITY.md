# Section 32 — approved two-stage OTP/QR integration

## Final availability

| Authority | Mission | OTP/QR status | Challenge owner |
|---|---|---|---|
| Dispatch | MOTORCYCLE | AVAILABLE | Dispatch `securehandover` |
| Hauling | VAN/TRUCK | AVAILABLE | Hauling secure-handover repository |
| Taxi Goods | CAR | AVAILABLE | Taxi Goods `goods` |
| Vendor-owned | `VendorMission` | AVAILABLE | Dispatch `vendorownedhandover` |

`PENDING_EXTERNAL_DEPENDENCY = NONE`.

The repository audit found that Vendor owns the business mission and vendor
staff assignment, but no non-Vendor service owned its secure custody boundary.
Fleet private execution was rejected because it requires marketplace provider
and vehicle identities; a vendor rider is expressly a Vendor staff identity.
Dispatch was extended with the smallest distinct VendorMission custody
capability. It mirrors immutable mission, tenant/branch, provider, receiver,
manifest and parcel-session scope without querying Vendor's database.

## Authority and security

Dispatch migration `000055_vendor_owned_secure_handover` owns mission custody,
independent OTP and opaque random QR generation, keyed hashes, expiry, attempt
and resend ceilings, single consumption, provider/receiver rotation,
cancellation fencing, idempotent operations and transactional outbox events.
PostgreSQL row locks serialize verification, resend, reassignment, receiver
change and cancellation. Notification Service receives the narrow
secret-delivery command after challenge commit; generic outbox events contain
no secret or receiver contact.

Vendor remains Commerce orchestrator, tenant/branch authorization boundary,
relay and projection. It stores only opaque observations in
`commerce_handover_observations`; additive migration
`000115_section32_vendor_owned_handover_projection` adds the mutually exclusive
VendorMission foreign key without adding challenge state. Vendor rider completion is projected only
after Dispatch returns or reads back canonical consumed state.

RAW OTP IN VENDOR DB: NO

RAW QR IN VENDOR DB: NO

OTP HASH IN VENDOR DB: NO

QR HASH IN VENDOR DB: NO

VENDOR CHALLENGE ENGINE: NO

VENDOR-OWNED AUTHORITY RAW OTP STORAGE: NO

VENDOR-OWNED AUTHORITY RAW QR STORAGE: NO

## Authentication and rollout

All Vendor-to-authority routes use HMAC v2 with method/path/body binding,
timestamp freshness and PostgreSQL nonce replay protection. The exact caller
allowlist is `vendor-service`. Vendor's public pickup relay remains protected
by staff bearer authentication, business ownership, branch scope and
`vendor.orders.manage`.

All four Vendor relay flags default false, including
`VENDOR_OWNED_HANDOVER_RELAY_ENABLED=false`. Dispatch authority registration
also defaults false via `DISPATCH_VENDOR_OWNED_HANDOVER_ENABLED=false`.

Unknown mutation outcomes are reconciled through stable mission/stage
read-back. Neither the normalized adapter nor the rider adapter repeats a
semantic custody transition after a lost response.

FRONTEND CODE CHANGED: NO
