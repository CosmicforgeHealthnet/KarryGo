package models

import (
	"encoding/json"
	"testing"
)

func TestRouteAndStopTransitions(t *testing.T) {
	for _, tc := range []struct {
		name    string
		ok, bad bool
	}{{"manual route approval", ValidRouteTransition(RouteDraft, RouteApproved), ValidRouteTransition(RoutePublished, RouteDraft)}, {"route lifecycle", ValidRouteTransition(RoutePublished, RouteInProgress), ValidRouteTransition(RouteCompleted, RouteInProgress)}, {"stop lifecycle", ValidStopTransition(StopArrived, StopServiceStarted), ValidStopTransition(StopCompleted, StopPlanned)}} {
		if !tc.ok || tc.bad {
			t.Errorf("%s ok=%v bad=%v", tc.name, tc.ok, tc.bad)
		}
	}
}
func TestOptimizationFingerprintCoversRouteVersionAndInputs(t *testing.T) {
	a := OptimizationInput{RouteID: "r", RouteVersion: 1, Stops: []RouteStop{{ID: "s1", Sequence: 1}}}
	b := a
	if OptimizationFingerprint(a) != OptimizationFingerprint(a) {
		t.Fatal("fingerprint must be deterministic")
	}
	b.RouteVersion = 2
	if OptimizationFingerprint(a) == OptimizationFingerprint(b) {
		t.Fatal("changed route version must create a new fingerprint")
	}
	b = a
	b.Stops = append([]RouteStop(nil), a.Stops...)
	b.Stops[0].Sequence = 2
	if OptimizationFingerprint(a) == OptimizationFingerprint(b) {
		t.Fatal("changed route input must create a new fingerprint")
	}
	raw, _ := json.Marshal(a)
	if len(raw) == 0 {
		t.Fatal("optimization input must be serializable")
	}
}
