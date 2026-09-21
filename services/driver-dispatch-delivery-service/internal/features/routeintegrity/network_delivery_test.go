package routeintegrity

import (
	"context"
	"testing"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/marketplace"
)

// Section 13: network facts go to the network ingest, route metrics to the
// route ingest, and neither is refused for not being the other.
func TestNetworkFactsReachTheNetworkIngest(t *testing.T) {
	route, network := &analyticsTransport{}, &analyticsTransport{}
	d := AnalyticsDeliverer{Transport: route, Network: network}
	for _, event := range []marketplace.Envelope{
		{EventID: "e1", EventType: "dispatch.capability_mismatch.v1", AggregateType: "delivery_mission",
			Payload: map[string]any{"reason_codes": "PARCEL_TOO_HEAVY"}},
		{EventID: "e2", EventType: "trip.completed.v1", AggregateType: "analytics:trip",
			Payload: map[string]any{"providerId": "r1"}},
	} {
		result, err := d.Deliver(context.Background(), event)
		if err != nil || result.Outcome == marketplace.DeliveryRejected {
			t.Fatalf("%s was refused: %+v %v", event.EventType, result, err)
		}
	}
	if len(network.events) != 2 || len(route.events) != 0 {
		t.Fatalf("network=%d route=%d, want 2/0", len(network.events), len(route.events))
	}
	if network.events[1].AggregateType != "trip" {
		t.Fatalf("the analytics copy prefix leaked: %q", network.events[1].AggregateType)
	}
}

func TestANetworkFactWithNoNetworkIngestIsNotSentToTheRouteIngest(t *testing.T) {
	route := &analyticsTransport{}
	result, _ := AnalyticsDeliverer{Transport: route}.Deliver(context.Background(),
		marketplace.Envelope{EventID: "e1", EventType: "dispatch.capability_mismatch.v1", Payload: map[string]any{}})
	if result.Outcome != marketplace.DeliveryRejected || len(route.events) != 0 {
		t.Fatalf("got %+v, route=%d", result, len(route.events))
	}
}
