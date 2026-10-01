package handover

import (
	"context"
	"cosmicforge/logistics/shared/go/serviceauth"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestTaxiTransportUsesCanonicalReadBackAndSanitizes(t *testing.T) {
	_, c := fixture()
	posts, reads := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		caller, err := serviceauth.NewVerifier(serviceauth.Secrets{"vendor-service": []byte("test-service-secret")}, time.Minute).VerifyRequest(r)
		if err != nil || caller != "vendor-service" {
			t.Error("invalid HMAC", err)
			w.WriteHeader(401)
			return
		}
		if r.Header.Get("X-Correlation-ID") != "request-a" {
			t.Error("missing correlation")
		}
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			posts++
			if !strings.HasSuffix(r.URL.Path, "/pickup/verify") || r.Header.Get("Idempotency-Key") != c.OperationRef {
				t.Error("wrong canonical command")
			}
			var in map[string]string
			_ = json.NewDecoder(r.Body).Decode(&in)
			if in["credential"] != c.Credential || in["challenge_ref"] != c.HandoverRef {
				t.Error("wrong credential relay")
			}
			_, _ = w.Write([]byte(`{"success":true,"data":{"mission_ref":"physical-mission-a","status":"PICKED_UP","extra_secret":"must-not-escape"}}`))
			return
		}
		reads++
		_, _ = w.Write([]byte(`{"success":true,"data":{"mission_ref":"physical-mission-a","handover_ref":"opaque-a","stage":"PICKUP","status":"CONSUMED","source_version":2,"otp_hash":"must-not-escape"}}`))
	}))
	defer server.Close()
	a := &Taxi{Origin: server.URL, Secret: []byte("test-service-secret"), Enabled: true}
	o, err := (Router{"TAXI": a}).ConfirmPickup(context.Background(), c)
	if err != nil || posts != 1 || reads != 1 || o.Status != "CONSUMED" {
		t.Fatalf("%+v %v posts=%d reads=%d", o, err, posts, reads)
	}
	raw, _ := json.Marshal(o)
	if strings.Contains(string(raw), "must-not-escape") {
		t.Fatal("upstream secret leaked")
	}
}

func TestTaxiWrongAuthorityNeverSends(t *testing.T) {
	_, c := fixture()
	c.Authority = "DISPATCH"
	if _, err := (&Taxi{Enabled: true}).ConfirmPickup(context.Background(), c); err != ErrInvalid {
		t.Fatal(err)
	}
}
