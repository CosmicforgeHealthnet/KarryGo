package deliveryrecovery

import (
	"context"
	"testing"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/commercequote"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/serviceability"
	"cosmicforge/logistics/shared/go/dispatchcommerce"
)

type recoveryRoutePlanner struct {
	called bool
	plan   commercequote.RoutePlan
}

func (p *recoveryRoutePlanner) Plan(context.Context, []commercequote.RoutePoint, dispatchcommerce.Mode) (commercequote.RoutePlan, error) {
	p.called = true
	return p.plan, nil
}

type recoveryZoneEvaluator struct {
	pickupInside, dropoffInside, restricted, available bool
}

func (z recoveryZoneEvaluator) Evaluate(_, _ serviceability.Point) (bool, bool, bool, bool) {
	return z.pickupInside, z.dropoffInside, z.restricted, z.available
}

func TestReturnRouteReusesSection6ZoneAndRoadAuthorities(t *testing.T) {
	trip := TripContext{TripID: "trip-1", PickupLat: 6.5, PickupLng: 3.3, DropoffLat: 6.6, DropoffLng: 3.4}
	road := &recoveryRoutePlanner{plan: commercequote.RoutePlan{Provider: "roads", EvidenceVersion: "v9", DistanceMeters: 3200, DurationSeconds: 600}}
	router := NewSection6ReturnRouter(road, recoveryZoneEvaluator{true, true, false, true})
	decision, err := router.ReturnRoute(context.Background(), trip)
	if err != nil || !road.called || decision.EvidenceVersion != "v9" || decision.DistanceMeters != 3200 {
		t.Fatalf("decision=%+v road_called=%t err=%v", decision, road.called, err)
	}

	road.called = false
	router = NewSection6ReturnRouter(road, recoveryZoneEvaluator{true, false, false, true})
	if _, err = router.ReturnRoute(context.Background(), trip); err == nil {
		t.Fatal("out-of-zone return route was accepted")
	}
	if road.called {
		t.Fatal("road provider was called after operating-zone rejection")
	}
}

func TestRecoveryPolicyIsServerOwnedAndReasonBound(t *testing.T) {
	policy := Policy{Enabled: true, ReturnEnabled: true, HubHoldEnabled: true}.normalized()
	actions := allowedActions("RECEIVER_NO_ANSWER", policy)
	for _, required := range []RecoveryAction{RetrySameDay, Reschedule, AlternativeReceiver, ReturnToSender, HubHold, OperationsReview} {
		if !containsAction(actions, required) {
			t.Fatalf("missing %s from %v", required, actions)
		}
	}
	refused := allowedActions("RECEIVER_REFUSED", policy)
	if containsAction(refused, RetrySameDay) || containsAction(refused, AlternativeReceiver) {
		t.Fatalf("unsafe refused actions = %v", refused)
	}
	unsafe := allowedActions("LOCATION_UNSAFE", policy)
	if containsAction(unsafe, RetrySameDay) {
		t.Fatalf("unsafe location may not retry: %v", unsafe)
	}
}

func TestAlternativeReceiverReferenceIsOpaque(t *testing.T) {
	contact := "+2348000000000"
	ref := opaqueReceiverRef(contact)
	if ref == contact || len(ref) < 20 {
		t.Fatalf("receiver ref is not opaque: %q", ref)
	}
	if ref != opaqueReceiverRef(contact) {
		t.Fatal("receiver reference is not deterministic")
	}
}

func TestConfiguredHubAuthorityFailsClosed(t *testing.T) {
	authority := NewConfiguredHubAuthority("hub-lagos=admin-policy-42")
	ref, err := authority.AuthorizeHub(context.Background(), "hub-lagos")
	if err != nil || ref != "admin-policy-42" {
		t.Fatalf("ref=%q err=%v", ref, err)
	}
	if _, err = authority.AuthorizeHub(context.Background(), "caller-invented"); err == nil {
		t.Fatal("unapproved hub was accepted")
	}
}

func TestHubCheckpointRequiresExactManifestScan(t *testing.T) {
	if !sameCodes([]string{"KGP-A", "kgp-b"}, []string{"KGP-B", "KGP-A", "KGP-A"}) {
		t.Fatal("equivalent manifest scans were rejected")
	}
	if sameCodes([]string{"KGP-A", "KGP-B"}, []string{"KGP-A"}) {
		t.Fatal("partial manifest scan was accepted")
	}
}
