package supporthttp

import (
	"testing"

	supportmodels "cosmicforge/logistics/services/support-dispute-service/internal/features/support/models"
)

func TestComplainantTypeFromRole(t *testing.T) {
	cases := map[string]supportmodels.ComplainantType{
		"customer":         supportmodels.ComplainantCustomer,
		"truck_provider":   supportmodels.ComplainantHaulingProvider,
		"hauling":          supportmodels.ComplainantHaulingProvider,
		"taxi_provider":    supportmodels.ComplainantTaxiProvider,
		"dispatch_provider": supportmodels.ComplainantDispatchProvider,
		"unknown":          supportmodels.ComplainantCustomer,
	}
	for role, want := range cases {
		if got := complainantTypeFromRole(role); got != want {
			t.Errorf("complainantTypeFromRole(%q) = %q, want %q", role, got, want)
		}
	}
}
