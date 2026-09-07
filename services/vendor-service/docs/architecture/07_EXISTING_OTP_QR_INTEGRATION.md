# 07 — Existing OTP/QR and handover integration

## Current classification

`DISPATCH OTP_QR_STATUS = AVAILABLE`

`HAULING OTP_QR_STATUS = AVAILABLE`

`TAXI OTP_QR_STATUS = AVAILABLE`

`VENDOR_OWNED OTP_QR_STATUS = AVAILABLE`

All four physical authorities are connected through Vendor's normalized
handover adapter. The adapter has only read and pickup-confirm operations. It
cannot issue, resend, bind, expire or consume a challenge by itself. Dispatch
requires the frozen stop and manifest binding; Hauling and Taxi resolve the
same canonical Commerce mission reference they created. Every mutation uses
HMAC v2 and canonical read-back before Vendor records an observation.

Hauling now owns a versioned, hash-only challenge with independently generated
numeric OTP and opaque QR credentials. They share TTL, attempt lockout,
resend invalidation and single-use consumption. Neither raw credential is
available through a generic read or Vendor projection.

This removes the `HAULING_SECURE_HANDOVER` dependency blocker for Hauling
Commerce. It does not make Hauling the authority for a Vendor-owned
`VendorMission`, and it does not globally enable Commerce.

## Vendor runtime boundary

Vendor Service defines only the opaque `VendorHandoverAuthority` port. Pickup,
receiver completion and return require a successful result from that port.
Production composition supplies Dispatch's distinct VendorMission custody
adapter when both disabled-by-default rollout flags are enabled. When either
flag or dependency is absent, transitions still fail closed with
`VENDOR_HANDOVER_UNAVAILABLE`.

Vendor Service does not:

- generate an OTP or QR credential;
- store a raw code, hash, expiry, attempt count or resend state;
- accept a client-supplied proof reference as verification;
- reuse a marketplace Dispatch trip identity for a Vendor-owned mission.

The persisted Vendor custody evidence contains only opaque authority and proof
references returned by the approved port. The client request fingerprint is
kept separate from that authority-owned result so an unknown-outcome retry can
read back the original operation without reissuing or re-verifying a code.

## Activation

The secure Vendor Rider flow remains gated by
`VENDOR_OWNED_HANDOVER_RELAY_ENABLED=false` and the Dispatch authority by
`DISPATCH_VENDOR_OWNED_HANDOVER_ENABLED=false`. Custody-moving actions never
degrade to insecure confirmation.
