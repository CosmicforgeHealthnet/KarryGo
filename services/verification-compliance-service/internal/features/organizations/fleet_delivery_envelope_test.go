package organizations

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Fleet's organization-verification/events route answers with its standard
// envelopes after FLEET-6h. The worker classifies by HTTP status alone, so
// both the recorded envelope and the legacy bare body must classify the same.
func TestFleetDeliveryClassifiesEnvelopeAndLegacyBodies(t *testing.T) {
	for _, tc := range []struct {
		name      string
		status    int
		body      string
		wantErr   bool
		permanent bool
	}{
		{"recorded envelope accepted", http.StatusOK,
			`{"data":{"status":"ACCEPTED","eventId":"e1","activated":true},"meta":{"requestId":"r"}}`, false, false},
		{"legacy bare accepted", http.StatusOK, `{"status":"ACCEPTED","eventId":"e1","activated":true}`, false, false},
		{"recorded envelope rejection", http.StatusBadRequest,
			`{"error":{"code":"submission_not_found","message":"The event was rejected.","retryable":false,"details":[],"requestId":"r"}}`, true, true},
		{"legacy bare rejection", http.StatusBadRequest, `{"status":"REJECTED","code":"submission_not_found"}`, true, true},
		{"recorded envelope transient", http.StatusInternalServerError,
			`{"error":{"code":"ingest_failed","message":"x","retryable":true,"details":[],"requestId":"r"}}`, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			worker := NewFleetDeliveryWorker(nil, server.URL, "secret", time.Second)
			err := worker.deliver(context.Background(), fleetRecord{ID: "e1", EventType: StatusChangedEventType,
				EventVersion: 1, OccurredAt: time.Now(), OrganizationID: "org_1", Payload: []byte(`{"status":"APPROVED"}`)})
			if (err != nil) != tc.wantErr {
				t.Fatalf("deliver error = %v, wantErr %v", err, tc.wantErr)
			}
			if errors.Is(err, errPermanent) != tc.permanent {
				t.Fatalf("permanent = %v, want %v (err %v)", errors.Is(err, errPermanent), tc.permanent, err)
			}
		})
	}
}
