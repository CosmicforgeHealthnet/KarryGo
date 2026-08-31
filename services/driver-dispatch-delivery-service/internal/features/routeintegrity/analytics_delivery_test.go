package routeintegrity

import (
	"context"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/marketplace"
	"cosmicforge/logistics/shared/go/commerceanalytics"
	"encoding/json"
	"strings"
	"testing"
)

type analyticsTransport struct{ events []marketplace.Envelope }

func (t *analyticsTransport) Configured() bool { return true }
func (t *analyticsTransport) Deliver(_ context.Context, e marketplace.Envelope) (marketplace.DeliveryResult, error) {
	t.events = append(t.events, e)
	return marketplace.DeliveryResult{Outcome: marketplace.DeliveryUnknown}, nil
}
func TestAnalyticsDeliveryMinimizesHistoricalPayloadAndPreservesRetryIdentity(t *testing.T) {
	transport := &analyticsTransport{}
	d := AnalyticsDeliverer{Transport: transport}
	e := marketplace.Envelope{EventID: "event", EventType: "dispatch.route.versioned.v1", Payload: map[string]any{"version": 2, "status": "CURRENT", "policy_version": PolicyVersion, "distance_meters": 1201, "duration_seconds": 450, "ordered_points": []any{map[string]any{"latitude": 6.51, "longitude": 3.44}}, "provider_id": "private", "otp": "secret"}}
	for i := 0; i < 2; i++ {
		r, err := d.Deliver(context.Background(), e)
		if err != nil || r.Outcome != marketplace.DeliveryUnknown {
			t.Fatal(r, err)
		}
	}
	raw, _ := json.Marshal(transport.events[0].Payload)
	if err := commerceanalytics.ValidateDispatchRouteMetric(raw); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"latitude", "longitude", "provider_id", "otp", "private", "secret"} {
		if strings.Contains(string(raw), bad) {
			t.Fatal("leak", bad)
		}
	}
	if transport.events[0].EventID != transport.events[1].EventID || transport.events[0].PayloadHash != transport.events[1].PayloadHash {
		t.Fatal("unstable delivery")
	}
}
