# 07 — Existing OTP/QR Integration

## OTP_QR_STATUS = NOT_PRESENT

A dedicated read-only audit searched `driver-dispatch-delivery-service`, `driver-hauling-service`, `driver-taxi-service`, `customer-service`, `shared/go`, and `fleet-partner-service` for any implementation of a secure two-stage OTP/QR custody-handover credential, distinct from login/authentication OTP. Full evidence below; verdict is unambiguous: **no service anywhere in the platform generates a one-time code or QR token, issues it to a sender/receiver, and verifies a returned code/scan against that server-issued value with expiry/resend/invalidation semantics.**

## Evidence, per service

**`driver-dispatch-delivery-service`** (the most developed candidate):

- `SubmitProof` (`internal/features/trip/service.go:453`, route `POST /api/v1/provider/trips/:id/proof`) requires a delivery photo, a signature image, `receiver_name`, and `receiver_phone`. The only "verification" is a string comparison: the provider-typed phone number against the phone already on file for the booking (`service.go:512`). No code is ever generated or sent to that number.
- `ConfirmRecipient` / WP6.3 (`internal/features/trip/delivery_outcome_service.go:27`, route `POST /api/v1/provider/trips/:id/recipient-confirmation`) accepts a `verification_method` field whose allowed values include `OTP` alongside `SIGNATURE`, `PHOTO`, `ID_CHECK`, `VERBAL` — but this is a self-declared label the provider picks; the code path is identical regardless of which label is chosen. There is no OTP generation, no SMS dispatch of a code, no expiry/resend/invalidation, and no comparison of a receiver-supplied code against a server-issued one anywhere in this feature. The `recipient_contact` field is compared against the booking's expected receiver and then discarded, never stored, never published as an event — it is the same static receiver phone number `SubmitProof` uses, not a one-time credential. What this feature does contribute genuinely is a well-designed idempotency-key + request-fingerprint + `expected_source_version` optimistic-concurrency pattern worth reusing for other commands — just not an OTP/QR mechanism.
- No pickup-side counterpart exists.

**`driver-hauling-service`**: `ConfirmPickup` involves no sender/vendor interaction of any kind. `ConfirmDelivery` accepts optional `proof_photo_url`/`signature_url` (explicitly best-effort — either may be empty) with no receiver-identity check at all. The only OTP code in this service is provider *login* OTP, unrelated to handover.

**`driver-taxi-service`**: pure directory scaffold, zero implementation of anything.

**`customer-service`**: only login/auth OTP.

**`shared/go`**: `auth` package is login-OTP/token/session helpers only. No `handover`, `custody`, `qrcode`, or `onetimecode`/`pickupcode`/`deliverycode` package exists anywhere in `shared/go`.

**`fleet-partner-service`**: has package/custody *tracking*, not a receiver-facing credential — worth distinguishing precisely since it is the closest-sounding thing in the platform:
- `fleet_partner_packages.qr_value` is a static, permanent, unique-per-package identifier generated once at package creation (functionally a barcode, not a one-time/expiring code), scanned by *pre-registered trusted internal devices* (warehouse/staff scanning through an internal status pipeline `CREATED → RECEIVED → SORTED → ... → DELIVERED`) — not issued to an external sender or receiver.
- `fleet_partner_custody_events` logs actor-to-actor handovers between internal operational actors (staff/warehouse/vehicle references), not a receiver credential.
- Section 19's "Proof" feature includes an `'OTP'` value in its `proof_type` enum, but examining the actual `CreateRecord` logic shows a submitted value is immediately HMAC-hashed and the plaintext discarded for later tamper-detection audit — there is no generation, no dispatch to anyone, and no verification method anywhere in the package. It is a hash-and-store audit field, not a challenge-response system.

## Handling per the task's Case C instruction

Because the capability is genuinely absent (not merely undiscovered), this architecture phase:

1. Does **not** implement a duplicate OTP/QR engine inside Vendor Service.
2. Creates only the Vendor Commerce integration abstraction/boundary: `internal/integrations/handover` (a `doc.go` describing the planned two-stage contract — no routes, no schema, no fake production code).
3. Documents the required approved flow exactly as specified (see [06_COMMERCE_FULFILMENT_MODEL.md](06_COMMERCE_FULFILMENT_MODEL.md), "Multi-vendor pickup and receiver handover rules").
4. Marks the dependency `PENDING_EXTERNAL_DEPENDENCY — LOGISTICS OTP/QR` in [PLATFORM_SERVICE_CHANGE_REGISTER.md](../PLATFORM_SERVICE_CHANGE_REGISTER.md), [IMPLEMENTATION_STATUS.md](../IMPLEMENTATION_STATUS.md), [FRONTEND_REQUIREMENTS.md](../FRONTEND_REQUIREMENTS.md), and [UI_UX_REQUIREMENTS.md](../UI_UX_REQUIREMENTS.md).
5. Does not let the absence block anything else — Vendor Service foundation, vendor authentication, business/branch architecture, catalogue architecture, CRM architecture, inventory architecture, purchasing architecture, order architecture, payment allocation architecture, and fulfilment planning architecture all proceed normally. Only the final activation of *confirmed-handover* platform-delivery fulfilment units depends on this.

## Likely owning service for the eventual real implementation

Not decided by this architecture phase (that decision belongs to whichever team builds the platform-wide capability, likely coordinated across dispatch/hauling/taxi rather than owned unilaterally by one of them), but `driver-dispatch-delivery-service`'s `ConfirmRecipient`/WP6.3 is the closest existing analog — it already has the idempotency/fingerprint/optimistic-concurrency scaffolding a real challenge-response credential would need, it is simply missing the actual code generation/verification step. A future implementation extending that feature (rather than starting over) is the most plausible path, but Vendor Service does not assume this and its `handover` package contract is written against the *behavior* the specification requires, not against any particular existing file.
