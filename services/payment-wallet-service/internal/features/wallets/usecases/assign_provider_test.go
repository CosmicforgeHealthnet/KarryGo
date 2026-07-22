package walletusecases

import (
	"context"
	"testing"

	"cosmicforge/logistics/shared/go/apperrors"
)

func TestAssignPaymentProviderValidation(t *testing.T) {
	service := NewWalletService(Options{})

	cases := []struct {
		name         string
		reference    string
		providerType string
		providerID   string
	}{
		{name: "missing reference", reference: "", providerType: "dispatch", providerID: "provider-1"},
		{name: "missing provider type", reference: "pi_123", providerType: "", providerID: "provider-1"},
		{name: "missing provider id", reference: "pi_123", providerType: "dispatch", providerID: ""},
		{name: "all missing", reference: " ", providerType: " ", providerID: " "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := service.AssignPaymentProvider(context.Background(), tc.reference, tc.providerType, tc.providerID)
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}
			appErr := apperrors.From(err)
			if appErr == nil || appErr.Code != apperrors.CodeValidationFailed {
				t.Fatalf("expected validation error, got %v", err)
			}
		})
	}
}
