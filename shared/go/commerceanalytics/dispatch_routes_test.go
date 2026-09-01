package commerceanalytics

import (
	"encoding/json"
	"testing"
	"time"
)

func TestDispatchRouteEnvelopeHasIndependentAuthorityAndNoPII(t *testing.T) {
	raw, _ := json.Marshal(DispatchRouteMetric{RouteVersion: 1, PolicyVersion: "dispatch_route_integrity_v1", State: "OPEN", AlertType: "STALE_LOCATION", Severity: "HIGH"})
	e := Envelope{EventID: "id", EventType: "dispatch.route_alert.opened.v1", SourceService: "driver-dispatch-delivery-service", SourceAggregate: "dispatch_route_metric", SourceID: "id", SourceVersion: 1, SchemaVersion: 1, OccurredAt: time.Now(), Payload: raw}
	if err := ValidateEnvelope(e); err != nil {
		t.Fatal(err)
	}
	e.SourceService = "vendor-service"
	if ValidateEnvelope(e) == nil {
		t.Fatal("caller confusion")
	}
	e.SourceService = "driver-dispatch-delivery-service"
	e.Payload = json.RawMessage(`{"route_version":1,"policy_version":"dispatch_route_integrity_v1","state":"OPEN","latitude":1}`)
	if ValidateEnvelope(e) == nil {
		t.Fatal("coordinates accepted")
	}
}
