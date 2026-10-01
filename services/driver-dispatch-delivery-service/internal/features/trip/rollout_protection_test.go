package trip

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type rolloutGate struct {
	required bool
	err      error
}

func (g rolloutGate) Enabled() bool                                        { return false }
func (g rolloutGate) Required(context.Context, string) (bool, error)       { return g.required, g.err }
func (g rolloutGate) PickupReady(context.Context, string) (bool, error)    { return false, nil }
func (g rolloutGate) DeliveryReady(context.Context, string) (bool, error)  { return false, nil }
func (g rolloutGate) EnsureDelivery(context.Context, string, string) error { return nil }
func (g rolloutGate) MarkInTransit(context.Context, string) error          { return nil }

func TestProtectedTripCannotUseLegacyRecipientConfirmationWhenFlagOff(t *testing.T) {
	service := NewService(newFakeRepository(), nil).WithSecureHandover(rolloutGate{required: true})
	_, err := service.ConfirmRecipient(context.Background(), ConfirmRecipientInput{TripID: "11111111-1111-1111-1111-111111111111"})
	if err == nil || !strings.Contains(err.Error(), "secure delivery verification route") {
		t.Fatalf("protected legacy completion was not rejected: %v", err)
	}
}

func TestRolloutPolicyReadFailureFailsClosed(t *testing.T) {
	service := NewService(newFakeRepository(), nil).WithSecureHandover(rolloutGate{err: errors.New("database unavailable")})
	_, err := service.ConfirmRecipient(context.Background(), ConfirmRecipientInput{TripID: "11111111-1111-1111-1111-111111111111"})
	if err == nil || !strings.Contains(err.Error(), "Secure handover policy is temporarily unavailable") {
		t.Fatalf("policy dependency failure did not fail closed: %v", err)
	}
}
