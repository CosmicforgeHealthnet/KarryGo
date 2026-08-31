package routeintegrity

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/marketplace"
	"cosmicforge/logistics/shared/go/commerceanalytics"
)

// AnalyticsDeliverer also minimizes already-persisted legacy route envelopes.
// It never performs a request from the GPS/domain transaction.
type AnalyticsDeliverer struct{ Transport marketplace.EventDeliverer }

func (d AnalyticsDeliverer) Configured() bool { return d.Transport != nil && d.Transport.Configured() }
func (d AnalyticsDeliverer) Deliver(ctx context.Context, e marketplace.Envelope) (marketplace.DeliveryResult, error) {
	if !commerceanalytics.IsDispatchRouteEvent(e.EventType) {
		return marketplace.DeliveryResult{Outcome: marketplace.DeliveryRejected, Code: "UNSUPPORTED_ROUTE_EVENT"}, nil
	}
	raw, _ := json.Marshal(e.Payload)
	var route ExpectedRoute
	var alert Alert
	var p commerceanalytics.DispatchRouteMetric
	_ = json.Unmarshal(raw, &route)
	_ = json.Unmarshal(raw, &alert)
	_ = json.Unmarshal(raw, &p)
	if p.RouteVersion == 0 {
		p.RouteVersion = route.Version
	}
	if p.RouteVersion == 0 {
		p.RouteVersion = alert.RouteVersion
	}
	if p.State == "" {
		p.State = route.Status
	}
	if route.DistanceMeters > 0 {
		p.DistanceBucketKM = (route.DistanceMeters + 4999) / 5000 * 5
	}
	if route.DurationSeconds > 0 {
		p.DurationBucketMinutes = (route.DurationSeconds + 299) / 300 * 5
	}
	raw, _ = json.Marshal(p)
	if err := commerceanalytics.ValidateDispatchRouteMetric(raw); err != nil {
		return marketplace.DeliveryResult{Outcome: marketplace.DeliveryRejected, Code: "INVALID_ROUTE_METRIC"}, err
	}
	e.Payload = map[string]any{}
	_ = json.Unmarshal(raw, &e.Payload)
	sum := sha256.Sum256(raw)
	e.PayloadHash = hex.EncodeToString(sum[:])
	return d.Transport.Deliver(ctx, e)
}
