package bookingusecases

import (
	"context"
	"errors"
	"testing"

	bookingmodels "cosmicforge/logistics/services/hauling-service/internal/features/booking/models"
	bookingrepo "cosmicforge/logistics/services/hauling-service/internal/features/booking/repositories"
	"cosmicforge/logistics/shared/go/custodychain"
)

// directionRepo answers only the commerce question the direction resolver
// asks. Everything else on the repository interface is unused here.
type directionRepo struct {
	bookingrepo.BookingRepository
	metadata *bookingmodels.CommerceOperations
	err      error
}

func (r directionRepo) CommerceProviderMetadata(
	context.Context, string,
) (*bookingmodels.CommerceOperations, error) {
	return r.metadata, r.err
}

// Hauling issues one kind of pickup credential for two flows with different
// verifiers. A Commerce mission is verified by Vendor Service over a signed
// internal call; a direct booking is verified by the customer's own app over
// a bearer token. Describing one as the other would tell a client the wrong
// party is entitled to check the driver.
func TestHaulingPickupDirectionFollowsTheFlow(t *testing.T) {
	commerce := &BookingService{bookings: directionRepo{
		metadata: &bookingmodels.CommerceOperations{SourceType: "VENDOR_COMMERCE"}}}
	purpose, ok := commerce.pickupPurpose(context.Background(), "bkg_1")
	if !ok || purpose != custodychain.PurposeCommercePickup {
		t.Fatalf("commerce pickup resolved to %q (ok=%v)", purpose, ok)
	}

	direct := &BookingService{bookings: directionRepo{metadata: nil}}
	purpose, ok = direct.pickupPurpose(context.Background(), "bkg_2")
	if !ok || purpose != custodychain.PurposeSenderVerifiedPickup {
		t.Fatalf("direct pickup resolved to %q (ok=%v)", purpose, ok)
	}

	// Both are provider-held. Only the verifier differs, which is exactly
	// why they are separate purposes.
	commerceDescriptor, _ := custodychain.DescriptorFor(custodychain.PurposeCommercePickup)
	directDescriptor, _ := custodychain.DescriptorFor(custodychain.PurposeSenderVerifiedPickup)
	if commerceDescriptor.Holder != directDescriptor.Holder {
		t.Fatal("the driver must hold the pickup credential on both flows")
	}
	if commerceDescriptor.Verifier == directDescriptor.Verifier {
		t.Fatal("a customer session can now verify a merchant's mission")
	}
}

// An unreadable commerce lookup states nothing rather than guessing. A wrong
// direction in the payload is worse than an absent one: absent, a client
// asks; wrong, it believes.
func TestHaulingStatesNoDirectionWhenTheFlowIsUnknown(t *testing.T) {
	service := &BookingService{bookings: directionRepo{err: errors.New("database is down")}}
	if _, ok := service.pickupPurpose(context.Background(), "bkg_1"); ok {
		t.Fatal("a direction was invented from a failed lookup")
	}
	view := service.withPickupDirection(context.Background(), "bkg_1", PickupCredentialView{})
	if view.Direction.Purpose != "" {
		t.Fatalf("direction = %+v; want an empty description", view.Direction)
	}
}

func TestHaulingDeliveryDirectionIsReceiverHeld(t *testing.T) {
	view := withDeliveryDirection("bkg_1", VerificationCodeView{})
	if view.Direction.Purpose != custodychain.PurposeDelivery ||
		view.Direction.Holder != custodychain.RoleReceiver ||
		view.Direction.Verifier != custodychain.RoleProvider {
		t.Fatalf("delivery direction = %+v", view.Direction)
	}
	if view.Direction.Authority != "HAULING" || view.Direction.MissionID != "bkg_1" {
		t.Fatalf("delivery direction is unscoped: %+v", view.Direction)
	}
}
