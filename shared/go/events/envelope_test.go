package events

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/serviceauth"
)

func summary() MovementSummary {
	return MovementSummary{CustomerID: "c1", Authority: MovementAuthorityTaxi, SourceRef: "b1", MovementType: MovementTypePassenger, State: MovementStateAssigned}
}

func TestMovementEnvelopeValidation(t *testing.T) {
	envelope, err := NewMovementEnvelope("e1", TaxiBookingAssigned, "driver-taxi-service", "taxi_booking", 2, time.Now(), "corr", "", summary())
	if err != nil {
		t.Fatal(err)
	}
	if envelope.SourceEntityID != "b1" || envelope.SchemaVersion != MovementSchemaVersion {
		t.Fatalf("envelope = %+v", envelope)
	}
	bad := envelope
	bad.SourceVersion = 0
	if bad.Validate() == nil {
		t.Fatal("source_version 0 must be invalid")
	}
	s := summary()
	s.Authority = "VENDOR"
	if _, err := NewMovementEnvelope("e2", TaxiBookingAssigned, "x", "y", 1, time.Now(), "", "", s); err == nil {
		t.Fatal("unknown authority must be invalid")
	}
	seq, count := 3, 2
	s = summary()
	s.MissionSequence, s.MissionCount = &seq, &count
	if s.Validate() == nil {
		t.Fatal("mission sequence beyond count must be invalid")
	}
	if !IsTerminalMovementState(MovementStateCompleted) || IsTerminalMovementState(MovementStateInProgress) {
		t.Fatal("terminal classification is wrong")
	}
}

func TestHTTPDelivererSignsAndPosts(t *testing.T) {
	verifier := serviceauth.NewVerifier(serviceauth.Secrets{"driver-taxi-service": []byte("secret")}, time.Minute)
	var received Envelope
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := verifier.VerifyRequest(r); err != nil || r.URL.Path != CustomerMovementIngressPath {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &received)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	envelope, _ := NewMovementEnvelope("e1", TaxiBookingCreated, "driver-taxi-service", "taxi_booking", 1, time.Now(), "", "", summary())
	deliverer := HTTPDeliverer{BaseURL: server.URL, Path: CustomerMovementIngressPath, ServiceName: "driver-taxi-service", Secret: []byte("secret")}
	if err := deliverer.Deliver(context.Background(), envelope); err != nil {
		t.Fatal(err)
	}
	if received.EventID != "e1" {
		t.Fatalf("received = %+v", received)
	}
	deliverer.Secret = []byte("wrong")
	if err := deliverer.Deliver(context.Background(), envelope); err == nil {
		t.Fatal("rejected delivery must return an error so the outbox retries")
	}
}
