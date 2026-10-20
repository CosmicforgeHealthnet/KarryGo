package analyticsadmin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/admincontext"
	"cosmicforge/logistics/shared/go/serviceauth"
)

func TestAnalyticsProducerPublisherClientUsesFixedOwnerAndNoAutomaticRetry(t *testing.T) {
	owner := "driver-taxi-service"
	key := bytes.Repeat([]byte{81}, 32)
	raw, _ := json.Marshal(admincontext.Envelope{Document: json.RawMessage(`{"owner":"driver-taxi-service"}`), KeyID: "test-key", Signature: "TEST_ONLY_PLACEHOLDER"})
	for _, tc := range []struct {
		name, body string
		status     int
		want       error
	}{
		{"recorded", `{"status":"RECORDED"}`, 200, nil},
		{"raw metadata", `{"status":"RECORDED","members":[]}`, 200, ErrInvalid},
		{"duplicate", `{"status":"RECORDED","status":"RECORDED"}`, 200, ErrInvalid},
		{"missing receipt", `{}`, 200, ErrInvalid},
		{"case alias", `{"Status":"RECORDED"}`, 200, ErrInvalid},
		{"denied", "", 403, ErrDenied},
		{"unknown outcome", "", 503, ErrUnavailable},
		{"redirect", "", 302, ErrUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Path != SourceContextPath+owner || r.Method != "POST" || r.URL.RawQuery != "" {
					t.Error("not fixed owner publication path")
				}
				if _, err := serviceauth.NewVerifier(serviceauth.Secrets{ProducerPublisherCaller: key}, 30*time.Second).VerifyRequest(r); err != nil {
					t.Error(err)
				}
				w.WriteHeader(tc.status)
				w.Write([]byte(tc.body))
			}))
			defer server.Close()
			client, err := NewPublisherClient(server.URL, owner, key)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			if err := client.Publish(context.Background(), raw); !errors.Is(err, tc.want) {
				t.Fatal(err)
			}
			if calls != 1 {
				t.Fatal("automatic publication retry", calls)
			}
			if err := client.Publish(context.Background(), bytes.ReplaceAll(raw, []byte(owner), []byte("driver-hauling-service"))); !errors.Is(err, ErrInvalid) || calls != 1 {
				t.Fatal("cross-owner request sent", err, calls)
			}
			if err := client.Publish(context.Background(), bytes.Repeat([]byte{'x'}, (32<<10)+1)); !errors.Is(err, ErrInvalid) || calls != 1 {
				t.Fatal("oversized request sent")
			}
		})
	}
	if _, err := NewPublisherClient("http://localhost", "invented-owner", key); err == nil {
		t.Fatal("invented owner")
	}
}
