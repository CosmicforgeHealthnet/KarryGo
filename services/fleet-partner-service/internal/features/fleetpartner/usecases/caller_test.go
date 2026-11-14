package usecases

import (
	"testing"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/fleetpartner/models"
)

func TestCallerOwnsSource(t *testing.T) {
	if !callerOwnsSource("driver-dispatch-delivery-service", models.ServiceDispatch) {
		t.Fatal("dispatch caller rejected for dispatch source")
	}
	if callerOwnsSource("driver-dispatch-delivery-service", models.ServiceTaxi) {
		t.Fatal("dispatch caller accepted for taxi source")
	}
}
