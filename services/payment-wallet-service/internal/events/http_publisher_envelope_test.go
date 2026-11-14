package events

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Fleet's finance/events route answers with its standard envelopes
// after FLEET-6h. The publisher classifies by HTTP status alone, so the
// recorded envelope and the legacy bare body must classify identically.
func TestPublishClassifiesFleetEnvelopeAndLegacyBodies(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  int
		body    string
		wantErr bool
	}{
		{"recorded envelope applied", http.StatusOK, `{"data":{"status":"ACCEPTED"},"meta":{"requestId":"r"}}`, false},
		{"legacy bare applied", http.StatusOK, `{"status":"ACCEPTED"}`, false},
		{"recorded envelope conflict", http.StatusConflict,
			`{"error":{"code":"payload_hash_mismatch","message":"x","retryable":false,"details":[],"requestId":"r"}}`, false},
		{"recorded envelope invalid", http.StatusUnprocessableEntity,
			`{"error":{"code":"invalid_envelope","message":"x","retryable":false,"details":[],"requestId":"r"}}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			publisher, err := NewHTTPPublisher(server.URL, "", "payment-wallet-service", []byte("secret"), time.Second)
			if err != nil {
				t.Fatalf("NewHTTPPublisher: %v", err)
			}
			err = publisher.Publish(context.Background(), OutboxRecord{ID: "evt_1", Payload: []byte(`{}`)})
			if (err != nil) != tc.wantErr {
				t.Fatalf("Publish error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}
