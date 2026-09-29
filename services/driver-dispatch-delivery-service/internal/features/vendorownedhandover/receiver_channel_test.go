package vendorownedhandover

import (
	"context"
	"errors"
	"testing"
)

func TestPhoneOnlyReceiverFailsBeforeVendorOwnedChallenge(t *testing.T) {
	s := NewService(&Repository{}, Policy{Enabled: true})
	_, err := s.Bind(context.Background(), MissionBinding{MissionRef: "mission", ReceiverPhone: "+390001234567"})
	if !errors.Is(err, ErrReceiverSecureChannelRequired) {
		t.Fatalf("phone-only receiver returned %v", err)
	}
}
