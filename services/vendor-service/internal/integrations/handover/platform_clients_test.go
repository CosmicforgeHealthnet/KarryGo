package handover

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/serviceauth"
)

func TestDispatchTransportRelaysExactScopeAndReadsBack(t *testing.T) {
	b := Binding{Authority: "DISPATCH", MissionRef: "mission-a", FulfilmentRef: "local-a", StopRef: "stop-a", ManifestRef: "plan-a-stop-a", ManifestVersion: 4}
	c := Confirmation{Binding: b, HandoverRef: "opaque-a", Method: "QR", Credential: "presented-only", OperationRef: "op-a", CorrelationID: "corr-a"}
	posts, reads := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		caller, err := serviceauth.NewVerifier(serviceauth.Secrets{"vendor-service": []byte("secret-a")}, time.Minute).VerifyRequest(r)
		if err != nil || caller != "vendor-service" {
			t.Errorf("HMAC: %v %q", err, caller)
			w.WriteHeader(401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			posts++
			if r.URL.Path != "/api/v1/internal/commerce/dispatch/missions/mission-a/stops/stop-a/pickup/verify" || r.Header.Get("Idempotency-Key") != "op-a" {
				t.Errorf("wrong command %s", r.URL.Path)
			}
			var in map[string]any
			_ = json.NewDecoder(r.Body).Decode(&in)
			if in["credential"] != "presented-only" || in["manifest_ref"] != b.ManifestRef {
				t.Errorf("wrong relay: %#v", in)
			}
			_, _ = w.Write([]byte(`{"success":true,"data":{"challenge_ref":"opaque-a","otp_hash":"must-not-escape"}}`))
			return
		}
		reads++
		_, _ = w.Write([]byte(`{"success":true,"data":{"mission_ref":"mission-a","handover_ref":"opaque-a","stage":"PICKUP","status":"CONSUMED","source_version":2,"qr_hash":"must-not-escape"}}`))
	}))
	defer server.Close()
	o, err := (Router{"DISPATCH": &Dispatch{Origin: server.URL, Secret: []byte("secret-a"), Enabled: true}}).ConfirmPickup(context.Background(), c)
	if err != nil || posts != 1 || reads != 1 || o.Status != "CONSUMED" {
		t.Fatalf("%+v %v posts=%d reads=%d", o, err, posts, reads)
	}
	raw, _ := json.Marshal(o)
	if strings.Contains(string(raw), "must-not-escape") || strings.Contains(string(raw), c.Credential) {
		t.Fatal("secret escaped")
	}
}

func TestHaulingTransportUsesCanonicalReadBack(t *testing.T) {
	b := Binding{Authority: "HAULING", MissionRef: "haul-a", FulfilmentRef: "local-a"}
	c := Confirmation{Binding: b, HandoverRef: "opaque-h", Method: "OTP", Credential: "presented-only", OperationRef: "op-h", CorrelationID: "corr-h"}
	posts, reads := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		caller, err := serviceauth.NewVerifier(serviceauth.Secrets{"vendor-service": []byte("secret-h")}, time.Minute).VerifyRequest(r)
		if err != nil || caller != "vendor-service" {
			t.Errorf("HMAC: %v", err)
			w.WriteHeader(401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			posts++
			if r.URL.Path != "/api/v1/hauling/internal/commerce/missions/haul-a/handover/PICKUP/confirm" {
				t.Error(r.URL.Path)
			}
			_, _ = w.Write([]byte(`{"success":true,"data":{"status":"CONSUMED"}}`))
			return
		}
		reads++
		_, _ = w.Write([]byte(`{"success":true,"data":{"mission_ref":"haul-a","handover_ref":"opaque-h","stage":"PICKUP","status":"CONSUMED","source_version":19}}`))
	}))
	defer server.Close()
	o, err := (Router{"HAULING": &Hauling{Origin: server.URL, Secret: []byte("secret-h"), Enabled: true}}).ConfirmPickup(context.Background(), c)
	if err != nil || posts != 1 || reads != 1 || o.SourceVersion != 19 {
		t.Fatalf("%+v %v posts=%d reads=%d", o, err, posts, reads)
	}
}
