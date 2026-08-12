package models

import "testing"

func TestPrivateOriginAndExecutionServiceAreSeparateVocabularies(t *testing.T) {
	for _, origin := range []string{"PRIVATE_ORDER", "PRIVATE_SHIPMENT", "ROUTE_STOP"} {
		if !ValidOrigin(origin) {
			t.Errorf("valid origin rejected: %s", origin)
		}
		if ValidExecutionService(origin) {
			t.Errorf("origin accepted as execution service: %s", origin)
		}
	}
	for _, service := range []string{"DISPATCH", "TAXI", "HAULING"} {
		if !ValidExecutionService(service) {
			t.Errorf("valid service rejected: %s", service)
		}
		if ValidOrigin(service) {
			t.Errorf("execution service accepted as origin: %s", service)
		}
	}
}

func TestDeclineReasonsReuseSafetyPolicy(t *testing.T) {
	if !ValidDecline("FATIGUE_OR_REST_REQUIRED") || !SafetyProtected("FATIGUE_OR_REST_REQUIRED") {
		t.Fatal("fatigue must be a valid safety-protected decline")
	}
	if !ValidDecline("OTHER") || !RequiresNote("OTHER") {
		t.Fatal("OTHER must be valid and require a note")
	}
	if ValidDecline("COMPANY_MARGIN_TOO_LOW") {
		t.Fatal("commercial punishment reason must not be accepted")
	}
}
