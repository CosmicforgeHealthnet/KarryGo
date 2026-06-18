package serviceauth

import (
	"bytes"
	"net/http"
	"testing"
	"time"
)

func TestSignAndVerifyRequest(t *testing.T) {
	body := []byte(`{"hello":"world"}`)
	request, err := http.NewRequest(http.MethodPost, "http://notification-service/api/v1/notifications/send", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}

	now := time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)
	if err := SignRequest(request, "customer-service", []byte("secret"), body, now); err != nil {
		t.Fatalf("SignRequest() error = %v", err)
	}

	verifier := NewVerifier(ParseSecrets("customer-service=secret"), 5*time.Minute)
	verifier.now = func() time.Time { return now }

	serviceName, err := verifier.VerifyRequest(request)
	if err != nil {
		t.Fatalf("VerifyRequest() error = %v", err)
	}
	if serviceName != "customer-service" {
		t.Fatalf("serviceName = %s", serviceName)
	}
}

func TestVerifyRejectsTamperedBody(t *testing.T) {
	body := []byte(`{"hello":"world"}`)
	request, err := http.NewRequest(http.MethodPost, "http://notification-service/api/v1/notifications/send", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}

	now := time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)
	if err := SignRequest(request, "customer-service", []byte("secret"), body, now); err != nil {
		t.Fatalf("SignRequest() error = %v", err)
	}
	request.Body = ioNopCloser{bytes.NewReader([]byte(`{"hello":"changed"}`))}

	verifier := NewVerifier(ParseSecrets("customer-service=secret"), 5*time.Minute)
	verifier.now = func() time.Time { return now }

	if _, err := verifier.VerifyRequest(request); err == nil {
		t.Fatal("expected tampered body to fail")
	}
}

type ioNopCloser struct {
	*bytes.Reader
}

func (c ioNopCloser) Close() error {
	return nil
}
