package orders

import (
	"testing"
	"time"
)

func etaFixture() ETAInput {
	now := time.Date(2026, 8, 25, 10, 0, 0, 0, time.UTC)
	readyFrom, readyTo := now.Add(10*time.Minute), now.Add(20*time.Minute)
	promise := now.Add(2 * time.Hour)
	return ETAInput{
		ParentOrderID: "po_1", FulfilmentGroupID: "fg_1", MissionSlotRef: "mission_1",
		PlanVersion: 3, SourceVersion: 7, SourceEventID: "event_1", SourceAuthority: "DISPATCH",
		SourceOccurredAt: now, Trigger: TriggerMissionCreated, Mode: "BALANCED", Domain: "FOOD",
		PromisedLatest: &promise, ReadyFrom: &readyFrom, ReadyTo: &readyTo,
		RouteSeconds: 1800, RouteProvider: "dispatch-route", RouteEvidenceVersion: "route_4",
		RouteObservedAt: now.Add(-time.Minute), PickupStopCount: 2,
	}
}

func TestConfidenceETAProducesVersionedExplainableRange(t *testing.T) {
	in := etaFixture()
	got, err := CalculateConfidenceETA(in)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != ETAActive || got.EarliestAt == nil || got.LatestAt == nil || !got.LatestAt.After(*got.EarliestAt) {
		t.Fatalf("expected active non-exact range, got %+v", got)
	}
	if got.PolicyVersion != ETAPolicyVersion || len(got.InputFingerprint) != 64 {
		t.Fatalf("missing explainability fields: %+v", got)
	}
	want := map[string]bool{"FOOD_PREPARATION": false, "PROVIDER_TO_PICKUP": false, "PICKUP_MANIFEST_HANDLING": false, "ADDITIONAL_VENDOR_STOPS": false, "RECEIVER_ROAD_ROUTE": false}
	for _, component := range got.Components {
		if _, ok := want[component.Kind]; ok {
			want[component.Kind] = true
		}
	}
	for kind, found := range want {
		if !found {
			t.Errorf("missing ETA component %s: %+v", kind, got.Components)
		}
	}
	if got.Confidence != ETAConfidenceLow { // provider search still contributes uncertainty
		t.Fatalf("pre-assignment confidence = %s, want LOW", got.Confidence)
	}
	if got.DelayState != ETAOnTime {
		t.Fatalf("delay state = %s, want ON_TIME", got.DelayState)
	}
}

func TestConfidenceETAShopAndOperationalConfidenceTransitions(t *testing.T) {
	in := etaFixture()
	in.Domain = "SHOP"
	in.ProviderAssigned = true
	in.ProviderRef = "provider_1"
	in.SourceEventID = "event_assigned"
	assigned, err := CalculateConfidenceETA(in)
	if err != nil {
		t.Fatal(err)
	}
	if assigned.Components[0].Kind != "SHOP_PICKING" || assigned.Confidence != ETAConfidenceMedium {
		t.Fatalf("assigned Shop ETA = %+v", assigned)
	}

	in.ActualReady, in.PickedUp = true, true
	in.Trigger, in.SourceEventID, in.SourceVersion = TriggerPickupCompleted, "event_pickup", 8
	pickedUp, err := CalculateConfidenceETA(in)
	if err != nil {
		t.Fatal(err)
	}
	if pickedUp.Confidence != ETAConfidenceHigh || pickedUp.Components[0].Source != "VENDOR_ACTUAL_READY" {
		t.Fatalf("pickup confidence/readiness = %+v", pickedUp)
	}

	in.OperationalException = true
	in.Trigger, in.SourceEventID, in.SourceVersion = TriggerBreakdown, "event_breakdown", 9
	broken, err := CalculateConfidenceETA(in)
	if err != nil {
		t.Fatal(err)
	}
	if broken.Confidence != ETAConfidenceLow {
		t.Fatalf("exception confidence = %s, want LOW", broken.Confidence)
	}

	in.OperationalException = false
	in.Trigger, in.SourceEventID = TriggerRouteDeviation, "event_stale_route"
	in.RouteObservedAt = in.SourceOccurredAt.Add(-ETARouteFreshFor - time.Second)
	stale, err := CalculateConfidenceETA(in)
	if err != nil {
		t.Fatal(err)
	}
	if stale.Confidence != ETAConfidenceLow {
		t.Fatalf("stale-route confidence = %s, want LOW", stale.Confidence)
	}
}

func TestConfidenceETADelayThresholdAndDedupe(t *testing.T) {
	in := etaFixture()
	promise := in.SourceOccurredAt.Add(30 * time.Minute)
	in.PromisedLatest = &promise
	first, err := CalculateConfidenceETA(in)
	if err != nil {
		t.Fatal(err)
	}
	if first.DelayState != ETADelayed || !first.NotifyDelay {
		t.Fatalf("first meaningful delay = %+v", first)
	}

	in.PreviousDelay = ETADelayed
	in.PreviousLatest = first.LatestAt
	in.SourceEventID, in.SourceVersion = "event_tiny_move", 8
	tiny, err := CalculateConfidenceETA(in)
	if err != nil {
		t.Fatal(err)
	}
	if tiny.NotifyDelay {
		t.Fatal("same delayed range must not produce notification spam")
	}

	previous := first.LatestAt.Add(-ETADelayNotifyMovement)
	in.PreviousLatest, in.SourceEventID, in.SourceVersion = &previous, "event_material_move", 9
	material, err := CalculateConfidenceETA(in)
	if err != nil {
		t.Fatal(err)
	}
	if !material.NotifyDelay {
		t.Fatal("material range movement while delayed should notify")
	}
}

func TestConfidenceETATerminalAndReturnStates(t *testing.T) {
	for state, want := range map[string]ETAStatus{"DELIVERED": ETAActual, "CANCELLED": ETAInvalidated, "RETURNING": ETAReturning} {
		in := etaFixture()
		in.MissionState = state
		in.SourceEventID = "event_" + state
		got, err := CalculateConfidenceETA(in)
		if err != nil {
			t.Fatalf("%s: %v", state, err)
		}
		if got.Status != want {
			t.Errorf("%s status = %s, want %s", state, got.Status, want)
		}
		if state == "DELIVERED" && (got.ActualAt == nil || !got.ActualAt.Equal(in.SourceOccurredAt)) {
			t.Errorf("delivery actual timestamp not preserved: %+v", got)
		}
		if state != "DELIVERED" && (got.EarliestAt != nil || got.LatestAt != nil) {
			t.Errorf("%s retained normal arrival range: %+v", state, got)
		}
	}
}

func TestConfidenceETAFingerprintIsSemanticAndSplitMissionsRemainIndependent(t *testing.T) {
	base := etaFixture()
	a, err := CalculateConfidenceETA(base)
	if err != nil {
		t.Fatal(err)
	}
	retry := base
	retry.SourceEventID = "retry_event"
	b, err := CalculateConfidenceETA(retry)
	if err != nil {
		t.Fatal(err)
	}
	if a.InputFingerprint != b.InputFingerprint {
		t.Fatal("transport retry identity changed the semantic fingerprint")
	}
	secondMission := base
	secondMission.MissionSlotRef = "mission_2"
	secondMission.RouteSeconds = 3600
	c, err := CalculateConfidenceETA(secondMission)
	if err != nil {
		t.Fatal(err)
	}
	if c.LatestAt.Equal(*a.LatestAt) {
		t.Fatal("split missions with distinct route evidence collapsed into one ETA")
	}
}

func TestConfidenceETARejectsMissingRouteAuthority(t *testing.T) {
	for name, mutate := range map[string]func(*ETAInput){
		"route duration":   func(in *ETAInput) { in.RouteSeconds = 0 },
		"route provider":   func(in *ETAInput) { in.RouteProvider = "" },
		"route version":    func(in *ETAInput) { in.RouteEvidenceVersion = "" },
		"source authority": func(in *ETAInput) { in.SourceAuthority = "" },
	} {
		t.Run(name, func(t *testing.T) {
			in := etaFixture()
			mutate(&in)
			if _, err := CalculateConfidenceETA(in); err == nil {
				t.Fatal("expected fail-closed validation")
			}
		})
	}
}

func TestScheduledCutoffUsesAuthoritativeUTCPrecedence(t *testing.T) {
	desired := time.Date(2026, 10, 25, 2, 30, 0, 0, time.FixedZone("CEST", 2*60*60))
	window := desired.Add(-2 * time.Hour)
	ready := desired.Add(-3 * time.Hour)
	if got := ScheduledCutoffAt(desired, nil, nil); !got.Equal(desired.UTC().Add(-time.Hour)) || got.Location() != time.UTC {
		t.Fatalf("desired fallback cutoff = %v", got)
	}
	if got := ScheduledCutoffAt(desired, &window, nil); !got.Equal(window.UTC()) {
		t.Fatalf("window cutoff = %v", got)
	}
	if got := ScheduledCutoffAt(desired, &window, &ready); !got.Equal(ready.UTC()) {
		t.Fatalf("production cutoff = %v", got)
	}
}
