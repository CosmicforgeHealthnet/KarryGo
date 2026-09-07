// Package handover is the reserved abstraction boundary for the platform's
// secure two-stage pickup/delivery custody-confirmation credential (OTP or
// QR), consumed by the dispatch, taxi, and hauling integrations
// (internal/integrations/dispatch, /taxi, /hauling) whenever a mission
// involves a vendor pickup or a customer delivery handover.
//
// # PLATFORM SECURE OTP/QR — AVAILABLE
//
// Dispatch, Hauling and Taxi Goods each report OTP_QR_STATUS = AVAILABLE and
// remain their own challenge authority. Hauling owns the
// hash-only, expiring, attempt-bounded, versioned pickup/delivery lifecycle;
// OTP and opaque QR are alternative single-use credentials for one challenge.
// Vendor Service does not retrieve either secret and must not build a second
// challenge engine. readiness.go exposes only the safe dependency projection.
//
// This package therefore defines ONLY the shape vendor-service's commerce
// integrations will call once the authoritative logistics OTP/QR system
// exists, per the two-stage flow the specification requires:
//
//	STAGE 1  Vendor/Sender -> Pickup Provider -> pickup OTP/QR confirmation -> custody transferred
//	STAGE 2  Delivery Provider -> Receiver -> delivery OTP/QR confirmation -> delivery completed
//
// The separate Vendor-owned Rider mission port below remains an abstraction;
// Hauling availability does not make Vendor the credential authority:
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
// Hauling Commerce readiness is READY, while the Hauling Commerce feature flag
// remains disabled by default. Other logistics modes retain their own truthful
// capability state.
package handover
