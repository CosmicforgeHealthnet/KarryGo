# 07 — Existing OTP/QR and handover integration

## Current classification

`OTP_QR_STATUS = PARTIAL`

The platform now has storage-agnostic code generation, hashing, expiry and
attempt primitives in `shared/go/handover`. Dispatch also has an authoritative
handover implementation for its own canonical Commerce missions, including
per-stop pickup verification and receiver verification.

That authority is not currently exposed through an approved contract that can
verify a Vendor-owned `VendorMission`. A Dispatch mission reference cannot be
fabricated merely to obtain its handover flow, because Dispatch and Vendor
missions have different canonical owners.

## Vendor runtime boundary

Vendor Service defines only the opaque `VendorHandoverAuthority` port. Pickup,
receiver completion and return require a successful result from that port.
Production composition supplies no implementation until a platform-approved
VendorMission-compatible authority exists, so these transitions fail closed
with `VENDOR_HANDOVER_UNAVAILABLE`.

Vendor Service does not:

- generate an OTP or QR credential;
- store a raw code, hash, expiry, attempt count or resend state;
- accept a client-supplied proof reference as verification;
- reuse a Dispatch mission identity for a Vendor-owned mission.

The persisted Vendor custody evidence contains only opaque authority and proof
references returned by the approved port. The client request fingerprint is
kept separate from that authority-owned result so an unknown-outcome retry can
read back the original operation without reissuing or re-verifying a code.

## Activation dependency

The secure Vendor Rider UI must remain gated until a compatible authority is
wired. Availability, assignment, navigation, receiver-unavailable handling and
pre-custody Hybrid fallback remain usable; custody-moving Vendor actions do
not degrade to insecure confirmation.
