package http

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/privateoperations/models"
)

func TestProviderViewAlwaysOffersAcceptAndDeclineWhilePending(t *testing.T) {
	view := providerDTO(models.Assignment{ID: "pas_1", ProviderID: "provider_1", Status: models.StatusPending, ResponseDeadline: time.Now(), Version: 1})
	if len(view.AllowedActions) != 2 || view.AllowedActions[0] != "ACCEPT" || view.AllowedActions[1] != "DECLINE" {
		t.Fatalf("allowed actions = %v", view.AllowedActions)
	}
}

func TestProviderViewContainsNoCommercialFields(t *testing.T) {
	raw, err := json.Marshal(providerDTO(models.Assignment{ID: "pas_1", Status: models.StatusPending}))
	if err != nil {
		t.Fatal(err)
	}
	lower := strings.ToLower(string(raw))
	for _, forbidden := range []string{"fare", "quotation", "contract", "settlement", "commission", "margin", "customer", "phone"} {
		if strings.Contains(lower, forbidden) {
			t.Errorf("provider DTO leaks %s: %s", forbidden, raw)
		}
	}
}
