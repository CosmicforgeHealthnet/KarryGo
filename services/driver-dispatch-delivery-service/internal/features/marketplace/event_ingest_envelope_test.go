package marketplace

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Recorded Fleet /internal/fleet-partner/v1/events bodies after FLEET-6h:
// standard envelopes plus the deprecated top-level aliases.
const (
	recordedFleetDuplicate = `{"data":{"status":"DUPLICATE","eventId":"evt_1","code":""},` +
		`"meta":{"requestId":"r1"},"status":"DUPLICATE","eventId":"evt_1","code":""}`
	recordedFleetRetryableRejection = `{"error":{"code":"aggregate_version_gap","message":"The event was rejected.",` +
		`"retryable":false,"details":[],"requestId":"r2"},"status":"REJECTED","code":"aggregate_version_gap"}`
	recordedFleetPermanentRejection = `{"error":{"code":"invalid_envelope","message":"The event was rejected.",` +
		`"retryable":false,"details":[],"requestId":"r3"},"status":"REJECTED","code":"invalid_envelope"}`
)

func deliverAgainst(t *testing.T, status int, body string) DeliveryResult {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()
	deliverer, live := NewHTTPEventDeliverer(DelivererConfig{BaseURL: server.URL,
		ServiceName: "driver-dispatch-delivery-service", Secret: "secret", Timeout: time.Second})
	if !live {
		t.Fatal("deliverer should be live")
	}
	result, err := deliverer.Deliver(context.Background(), Envelope{EventID: "evt_1", EventType: "t",
		SchemaVersion: 1, Payload: map[string]any{}})
	if err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	return result
}

func TestDeliverClassifiesRecordedFleetEnvelopes(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  int
		body    string
		outcome DeliveryOutcome
		code    string
	}{
		{"envelope duplicate", http.StatusOK, recordedFleetDuplicate, DeliveryDuplicate, ""},
		{"envelope only duplicate", http.StatusOK, `{"data":{"status":"DUPLICATE","code":"x"},"meta":{}}`, DeliveryDuplicate, "x"},
		{"envelope accepted", http.StatusOK, `{"data":{"status":"ACCEPTED","eventId":"evt_1","code":""},"meta":{}}`, DeliveryAccepted, ""},
		{"envelope retryable rejection", http.StatusBadRequest, recordedFleetRetryableRejection, DeliveryRetryable, "aggregate_version_gap"},
		{"error-only retryable rejection", http.StatusBadRequest, `{"error":{"code":"aggregate_version_gap"}}`, DeliveryRetryable, "aggregate_version_gap"},
		{"envelope permanent rejection", http.StatusBadRequest, recordedFleetPermanentRejection, DeliveryRejected, "invalid_envelope"},
		{"legacy bare duplicate", http.StatusOK, `{"status":"DUPLICATE","eventId":"evt_1","code":""}`, DeliveryDuplicate, ""},
		{"legacy bare rejection", http.StatusBadRequest, `{"status":"REJECTED","code":"aggregate_version_gap"}`, DeliveryRetryable, "aggregate_version_gap"},
		{"legacy bare server error", http.StatusInternalServerError, `{"status":"ERROR","code":"ingest_failed"}`, DeliveryRetryable, "ingest_failed"},
		{"envelope server error", http.StatusInternalServerError, `{"error":{"code":"ingest_failed","retryable":true}}`, DeliveryRetryable, "ingest_failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := deliverAgainst(t, tc.status, tc.body)
			if got.Outcome != tc.outcome || got.Code != tc.code {
				t.Fatalf("got %s/%q, want %s/%q", got.Outcome, got.Code, tc.outcome, tc.code)
			}
		})
	}
}
