package serviceauth

import (
	"bytes"
	"net/http"
	"strconv"
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

func TestVerifyRejectsTamperedNonceCallerTimestampMethodAndPath(t *testing.T) {
	body := []byte(`{"hello":"world"}`)
	now := time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)
	newSigned := func() *http.Request {
		req, err := http.NewRequest(http.MethodPost, "http://service/api/v1/private?b=2&a=1", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set(HeaderNonce, "fixed-single-use-nonce")
		if err := SignRequest(req, "fleet-partner-service", []byte("secret"), body, now); err != nil {
			t.Fatal(err)
		}
		return req
	}
	verifier := NewVerifier(ParseSecrets("fleet-partner-service=secret,other-service=secret"), 5*time.Minute)
	verifier.now = func() time.Time { return now }
	for name, mutate := range map[string]func(*http.Request){
		"nonce":  func(r *http.Request) { r.Header.Set(HeaderNonce, "replacement-nonce") },
		"caller": func(r *http.Request) { r.Header.Set(HeaderServiceName, "other-service") },
		"timestamp": func(r *http.Request) {
			r.Header.Set(HeaderTimestamp, strconv.FormatInt(now.Add(time.Second).Unix(), 10))
		},
		"method": func(r *http.Request) { r.Method = http.MethodPut },
		"path":   func(r *http.Request) { r.URL.Path = "/api/v1/other" },
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := verifier.VerifyRequest(mutateAndReturn(newSigned(), mutate)); err == nil {
				t.Fatal("tampered request verified")
			}
		})
	}
}

func TestVerifyRequiresNonceAndVersion(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "http://service/private", nil)
	if err := SignRequest(req, "fleet-partner-service", []byte("secret"), nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	verifier := NewVerifier(ParseSecrets("fleet-partner-service=secret"), 5*time.Minute)
	req.Header.Del(HeaderNonce)
	if _, err := verifier.VerifyRequest(req); err == nil {
		t.Fatal("missing nonce verified")
	}
	req, _ = http.NewRequest(http.MethodGet, "http://service/private", nil)
	if err := SignRequest(req, "fleet-partner-service", []byte("secret"), nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	req.Header.Del(HeaderVersion)
	if _, err := verifier.VerifyRequest(req); err == nil {
		t.Fatal("missing version verified")
	}
}

func mutateAndReturn(req *http.Request, mutate func(*http.Request)) *http.Request {
	mutate(req)
	return req
}

type ioNopCloser struct {
	*bytes.Reader
}

func (c ioNopCloser) Close() error {
	return nil
}
