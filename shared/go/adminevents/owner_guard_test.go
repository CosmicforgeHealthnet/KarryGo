package adminevents

import (
	"bytes"
	"context"
	"cosmicforge/logistics/shared/go/outbox"
	"cosmicforge/logistics/shared/go/serviceauth"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestOwnerExportGuardRunsBeforeSigningAndPreservesPayload(t *testing.T) {
	key := bytes.Repeat([]byte("t"), 32)
	var calls atomic.Int64
	received := make(chan []byte, 2)
	verifier := serviceauth.NewVerifier(serviceauth.Secrets{"support-dispute-service": key}, time.Minute)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if caller, err := verifier.VerifyRequest(r); err != nil || caller != "support-dispute-service" {
			t.Error("invalid workload signature")
		}
		body, _ := io.ReadAll(r.Body)
		received <- body
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	inner, err := outbox.NewHTTPPublisher(server.URL, IngressPath, "support-dispute-service", key, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	record := outbox.Record{ID: "test-event", Payload: json.RawMessage(`{"event_id":"test-event","scope":{"tenant":"test"}}`)}
	original := append([]byte(nil), record.Payload...)
	denied := errors.New("allocation revoked")
	publisher := rawPublisher{inner: inner, guard: func(context.Context, outbox.Record) error { return denied }}
	if err = publisher.Publish(context.Background(), record); !errors.Is(err, denied) || calls.Load() != 0 {
		t.Fatal("denied event reached publisher")
	}
	publisher.guard = func(context.Context, outbox.Record) error { return nil }
	if err = publisher.Publish(context.Background(), record); err != nil || calls.Load() != 1 || !bytes.Equal(<-received, original) || !bytes.Equal(record.Payload, original) {
		t.Fatalf("approved bytes changed or publish failed: %v", err)
	}
	publisher.guard = nil
	if err = publisher.Publish(context.Background(), record); err != nil || calls.Load() != 2 {
		t.Fatal("legacy owners changed")
	}
}
