// Package handover is the reserved abstraction boundary for the platform's
// secure two-stage pickup/delivery custody-confirmation credential (OTP or
// QR), consumed by the dispatch, taxi, and hauling integrations
// (internal/integrations/dispatch, /taxi, /hauling) whenever a mission
// involves a vendor pickup or a customer delivery handover.
//
// # PENDING_EXTERNAL_DEPENDENCY — LOGISTICS OTP/QR
//
// Audit verdict (see docs/architecture/07_EXISTING_OTP_QR_INTEGRATION.md and
// docs/architecture/00_REPOSITORY_AUDIT_BASELINE.md): OTP_QR_STATUS = NOT_PRESENT.
// No service in the platform today generates a one-time code or QR token,
// issues it to a sender/receiver, and verifies a returned code/scan against
// that server-issued value with expiry/resend/invalidation semantics.
// Delivery confirmation today (driver-dispatch-delivery-service's
// SubmitProof/ConfirmRecipient, driver-hauling-service's ConfirmDelivery) is
// evidence capture — a photo, a signature, and a static receiver-phone string
// match — not a challenge-response credential. This is a real, disclosed gap
// in the wider platform, not something vendor-service invents a workaround
// for and not something vendor-service is authorized to build a duplicate
// implementation of.
//
// This package therefore defines ONLY the shape vendor-service's commerce
// integrations will call once the authoritative logistics OTP/QR system
// exists, per the two-stage flow the specification requires:
//
//	STAGE 1  Vendor/Sender -> Pickup Provider -> pickup OTP/QR confirmation -> custody transferred
//	STAGE 2  Delivery Provider -> Receiver -> delivery OTP/QR confirmation -> delivery completed
//
// Planned contract (not implemented; will be defined precisely once the
// owning logistics service — most likely driver-dispatch-delivery-service,
// given it already has the closest analog, ConfirmRecipient/WP6.3 — builds a
// real challenge-response credential):
//   - IssuePickupChallenge(mission, vendorHandoverSessionRef) — one challenge
//     per vendor pickup handover session, not one per parcel; every parcel in
//     that pickup stays bound to the session's manifest.
//   - IssueDeliveryChallenge(mission, receiverRef) — one challenge per
//     mission/manifest for a single-provider delivery to one receiver; a
//     split-provider mission requires a separate challenge per receiver
//     handover.
//   - Reassigning the mission's provider invalidates any challenge tied to
//     the previous provider; changing the declared receiver invalidates the
//     previous receiver's challenge; a resend invalidates the prior
//     credential.
//   - This credential is a one-time custody credential, entirely separate
//     from any login/authentication OTP (shared/go/auth) issued by
//     customer-service, driver-hauling-service, or
//     driver-dispatch-delivery-service.
//
// Recorded in docs/PLATFORM_SERVICE_CHANGE_REGISTER.md with status
// PENDING_EXTERNAL_DEPENDENCY. This absence blocks only the final activation
// of platform-delivery handover confirmation for vendor commerce orders; it
// does not block vendor onboarding, catalogues, CRM, inventory, purchasing,
// commerce checkout, or fulfilment-plan architecture, none of which depend on
// this package.
package handover
