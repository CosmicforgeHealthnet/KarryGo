package clients

import (
	"bytes"
	"context"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/config"
	"cosmicforge/logistics/shared/go/serviceauth"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestAuthenticatedTypedOwnerClient(t *testing.T) {
	secret := bytes.Repeat([]byte{41}, 32)
	verifier := serviceauth.NewVerifier(serviceauth.Secrets{config.Caller: secret}, time.Minute)
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if _, err := verifier.VerifyRequest(r); err != nil {
			t.Error(err)
			w.WriteHeader(401)
			return
		}
		if r.Method != "GET" || r.URL.Path != "/api/v1/analytics/network/mission-success" || r.URL.Query().Get("authority") != "DISPATCH" || r.Header.Get("X-Correlation-ID") != "test-correlation" {
			t.Error("wrong owner contract")
		}
		_, _ = w.Write([]byte(`{"success":true,"data":[{"authority":"DISPATCH","date":"2026-10-18T00:00:00Z","completed":3,"failed":1,"cancelled":0,"guarantee_breaches":0,"success_rate":0.75,"private_evidence":"STRIP_ME"}]}`))
	}))
	defer srv.Close()
	client, err := NewAnalytics(srv.URL, secret)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	rows, err := client.MissionSuccess(context.Background(), "DISPATCH", 7, "test-correlation")
	if err != nil || len(rows) != 1 || rows[0].Completed != 3 || hits.Load() != 1 {
		t.Fatal(rows, err, hits.Load())
	}
}
func TestOwnerRefusalTimeoutAndRedirectNeverRetry(t *testing.T) {
	for _, scenario := range []string{"refused", "timeout", "redirect", "invalid"} {
		t.Run(scenario, func(t *testing.T) {
			var hits atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits.Add(1)
				switch scenario {
				case "refused":
					w.WriteHeader(403)
				case "timeout":
					time.Sleep(50 * time.Millisecond)
				case "redirect":
					http.Redirect(w, r, "http://unapproved.example", 302)
				case "invalid":
					_, _ = w.Write([]byte(`{"success":true,"data":null}`))
				}
			}))
			defer srv.Close()
			client, err := NewAnalytics(srv.URL, bytes.Repeat([]byte{41}, 32))
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			client.client.Timeout = 10 * time.Millisecond
			if _, err = client.MissionSuccess(context.Background(), "DISPATCH", 7, "test-correlation"); err == nil {
				t.Fatal("owner failure accepted")
			}
			if hits.Load() != 1 {
				t.Fatal("request automatically repeated")
			}
		})
	}
}
func TestClientRejectsArbitraryDestination(t *testing.T) {
	for _, origin := range []string{"https://owner.example/arbitrary/path", "https://user:secret@owner.example", "file:///etc/passwd", "http://owner.example?path=/internal/write"} {
		if _, err := NewAnalytics(origin, bytes.Repeat([]byte{41}, 32)); err == nil {
			t.Fatal("arbitrary proxy configuration accepted")
		}
	}
}
