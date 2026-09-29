package cart

import "testing"

func TestReceiverAccountBindingValidation(t *testing.T) {
	bound, err := ValidateReceiver(Receiver{Name: "Recipient", Phone: "+390001234567", CustomerID: "11111111-1111-4111-8111-111111111111"})
	if err != nil || bound.CustomerID == "" {
		t.Fatalf("valid receiver account rejected: %+v %v", bound, err)
	}
	if _, err = ValidateReceiver(Receiver{Name: "Recipient", Phone: "+390001234567", CustomerID: "not-an-account-id"}); err != ErrInvalidReceiver {
		t.Fatalf("invalid account identifier accepted: %v", err)
	}
	phoneOnly, err := ValidateReceiver(Receiver{Name: "Recipient", Phone: "+390001234567"})
	if err != nil || phoneOnly.CustomerID != "" {
		t.Fatalf("phone-only contact incorrectly acquired account identity: %+v %v", phoneOnly, err)
	}
}
