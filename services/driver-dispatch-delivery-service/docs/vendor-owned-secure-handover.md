# Vendor-owned secure handover authority

## Decision

Dispatch owns the secure custody state for `VENDOR_OWN_DELIVERY` without
turning vendor riders into marketplace providers or creating synthetic Trips.
Vendor Commerce continues to own commercial orchestration and its business
mission projection. Fleet private execution was not suitable because its
canonical actors are organization marketplace providers and vehicles, while a
vendor rider is a vendor staff member.

## Storage and lifecycle

Migration `000055_vendor_owned_secure_handover` adds a distinct mission binding,
logical challenge and idempotent operation store in the Dispatch database.
Each binding freezes mission, fulfilment, business, branch, provider, receiver,
manifest/version and exact parcel session scope. Pickup success consumes the
pickup logical challenge and creates the receiver's delivery challenge.
Delivery success commits the canonical terminal custody state.

Only HMAC-SHA256 keyed OTP and QR digests are stored. The OTP and 256-bit opaque
QR token are generated independently by `shared/go/handover`, coexist as two
interfaces to one logical challenge, and are never returned by an API. A narrow
Notification command receives plaintext only for delivery to the bound vendor
staff member or receiver. Generic outbox payload constraints forbid secrets.

Verification, attempts, resend, reassignment, receiver change and cancellation
lock the mission/challenge rows. A partial unique index enforces one active
challenge per mission/stage. Successful verification consumes both interfaces;
resend and binding changes invalidate both before replacement.

## Security boundary

The internal API is registered only when
`DISPATCH_VENDOR_OWNED_HANDOVER_ENABLED=true`. It uses service-auth HMAC v2,
PostgreSQL nonce replay protection and the exact `vendor-service` caller
allowlist. Vendor never supplies an authority selected by a browser: it derives
the binding from its frozen fulfilment plan and assigned mission.

Offline final handover is disallowed. Read-back by mission and stage is the
only recovery for a lost mutation response.

## Certification

The PostgreSQL suite covers OTP-vs-QR, OTP-vs-OTP, QR-vs-QR, duplicate relay,
resend-vs-verification, reassignment-vs-verification,
receiver-change-vs-verification, cancellation-vs-consumption, expiry, attempt
lockout, wrong provider/mission, split-mission isolation, mutual invalidation
and schema plaintext absence.
