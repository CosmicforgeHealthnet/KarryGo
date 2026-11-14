package outbox

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func deliverMarketplaceAgainst(t *testing.T, status int, body string) error {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()
	destination := FleetMarketplaceEventDestination{BaseURL: server.URL, ServiceName: "driver-taxi-service",
		Secret: []byte("secret"), Enabled: true}
	return destination.Deliver(context.Background(), Record{ID: "evt_1", AggregateType: "booking",
		AggregateID: "b1", EventType: "taxi.request.created.v1", Payload: []byte(`{"sourceVersion":1}`),
		OccurredAt: time.Now()})
}

// Recorded Fleet /events bodies after FLEET-6h (envelope plus deprecated
// aliases), the envelope alone, and the legacy bare body.
func TestFleetMarketplaceDeliveryReadsEnvelopeAndLegacyRejections(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"recorded envelope with aliases", `{"error":{"code":"unknown_event_type","message":"The event was rejected.","retryable":false,"details":[],"requestId":"r"},"status":"REJECTED","code":"unknown_event_type"}`},
		{"envelope only", `{"error":{"code":"unknown_event_type"}}`},
		{"legacy bare", `{"status":"REJECTED","code":"unknown_event_type"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := deliverMarketplaceAgainst(t, http.StatusBadRequest, tc.body)
			var terminal *TerminalDeliveryError
			if !errors.As(err, &terminal) || terminal.Code != "unknown_event_type" {
				t.Fatalf("want terminal unknown_event_type, got %#v", err)
			}
		})
	}
}

func TestFleetMarketplaceDeliveryAcceptsEnvelopeSuccess(t *testing.T) {
	body := `{"data":{"status":"ACCEPTED","eventId":"evt_1","code":""},"meta":{"requestId":"r"},"status":"ACCEPTED","eventId":"evt_1","code":""}`
	if err := deliverMarketplaceAgainst(t, http.StatusOK, body); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
}
