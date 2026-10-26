package campaignclients

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/serviceauth"
)

func TestClientSignsAsNotificationServiceAndParsesContract(t *testing.T) {
	secret := []byte("customer-audience-secret-0123456789abcdef")
	verifier := serviceauth.NewVerifier(serviceauth.Secrets{"notification-service": secret}, time.Minute)
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		caller, err := verifier.VerifyRequest(r)
		if err != nil || caller != "notification-service" || r.Header.Get(serviceauth.HeaderNonce) == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		seen = append(seen, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)
		switch r.URL.Path {
		case AudiencePath:
			_, _ = io.WriteString(w, `{"success":true,"data":{"channel":"push","market":"NG-LAG","as_of":"2026-10-26T00:00:00Z","members":[{"customer_id":"11111111-1111-4111-8111-111111111111","consent_version":2}],"next_cursor":""}}`)
		case CheckPath:
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), `"channel":"push"`) {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			_, _ = io.WriteString(w, `{"success":true,"data":{"channel":"push","as_of":"2026-10-26T00:00:00Z","results":[{"customer_id":"11111111-1111-4111-8111-111111111111","eligible":true,"reason":"OPTED_IN","consent_version":2,"market":"NG-LAG"}]}}`)
		}
	}))
	defer srv.Close()
	c := &CustomerAudienceClient{BaseURL: srv.URL, Secret: secret}
	page, err := c.Audience(context.Background(), "push", "NG-LAG", "", 500)
	if err != nil || len(page.Members) != 1 || page.Members[0].ConsentVersion != 2 {
		t.Fatalf("audience: %+v %v", page, err)
	}
	res, err := c.Check(context.Background(), "push", []string{"11111111-1111-4111-8111-111111111111"})
	if err != nil || !res.Results[0].Eligible {
		t.Fatalf("check: %+v %v", res, err)
	}
	if len(seen) != 2 || !strings.Contains(seen[0], "market=NG-LAG") {
		t.Fatalf("requests: %v", seen)
	}
	// A mismatching answer, a refusal or a missing configuration is never
	// read as consent.
	if _, err := c.Audience(context.Background(), "email", "NG-LAG", "", 500); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("mismatched answer: %v", err)
	}
	bad := &CustomerAudienceClient{BaseURL: srv.URL, Secret: []byte("wrong-secret-wrong-secret-wrong-secret")}
	if _, err := bad.Check(context.Background(), "push", []string{"x"}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("refused request: %v", err)
	}
	if _, err := (&CustomerAudienceClient{}).Audience(context.Background(), "push", "NG-LAG", "", 1); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("unconfigured: %v", err)
	}
}
