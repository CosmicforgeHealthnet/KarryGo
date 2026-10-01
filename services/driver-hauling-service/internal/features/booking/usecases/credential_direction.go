package bookingusecases

import (
	"context"
	"time"

	"cosmicforge/logistics/shared/go/custodychain"
)

// Hauling issues the SAME kind of pickup credential for two different flows,
// and they do not have the same verifier.
//
//   - A Commerce mission's pickup is verified by Vendor Service over an
//     HMAC-signed internal call. The merchant is the sender.
//   - A direct booking's pickup is verified by the customer's own app over a
//     bearer token. The customer is the sender.
//
// The driver holds the credential either way, so reading the holder off the
// route is harmless; reading the VERIFIER off it is not, because the two
// verifiers authenticate completely differently. CFC-008 asks for that to be
// stated in the payload rather than inferred, which is what this resolves.

const haulingAuthority = "HAULING"

// pickupPurpose decides which pickup direction a booking is running.
//
// An unreadable commerce lookup returns ok=false rather than guessing. A
// wrong direction in the payload is worse than an absent one: absent, a
// client asks; wrong, it believes.
func (s *BookingService) pickupPurpose(ctx context.Context, bookingID string) (custodychain.CredentialPurpose, bool) {
	reader, ok := s.bookings.(commerceProviderMetadataReader)
	if !ok {
		// A deployment whose repository cannot answer the question says
		// nothing rather than guessing a direction.
		return "", false
	}
	metadata, err := reader.CommerceProviderMetadata(ctx, bookingID)
	if err != nil {
		return "", false
	}
	if metadata != nil {
		return custodychain.PurposeCommercePickup, true
	}
	return custodychain.PurposeSenderVerifiedPickup, true
}

// pickupDirection builds the description for one booking's pickup credential.
func (s *BookingService) pickupDirection(
	ctx context.Context, bookingID string, expiresAt *time.Time,
) (custodychain.Direction, bool) {
	purpose, ok := s.pickupPurpose(ctx, bookingID)
	if !ok {
		return custodychain.Direction{}, false
	}
	return custodychain.DirectionFor(purpose, haulingAuthority, bookingID, expiresAt)
}

// deliveryDirection describes the delivery credential, which has one
// direction on every flow: the receiver holds it, the driver verifies it.
func deliveryDirection(bookingID string, expiresAt *time.Time) (custodychain.Direction, bool) {
	return custodychain.DirectionFor(
		custodychain.PurposeDelivery, haulingAuthority, bookingID, expiresAt)
}

// withPickupDirection stamps the description onto a pickup view at the point
// it leaves the service, so every public entry point answers the same way
// without each one repeating the lookup.
func (s *BookingService) withPickupDirection(
	ctx context.Context, bookingID string, view PickupCredentialView,
) PickupCredentialView {
	if direction, ok := s.pickupDirection(ctx, bookingID, view.ExpiresAt); ok {
		view.Direction = direction
	}
	return view
}

// withDeliveryDirection does the same for the customer-facing delivery view.
func withDeliveryDirection(bookingID string, view VerificationCodeView) VerificationCodeView {
	if direction, ok := deliveryDirection(bookingID, view.ExpiresAt); ok {
		view.Direction = direction
	}
	return view
}
