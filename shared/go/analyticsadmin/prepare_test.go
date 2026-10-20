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
	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/serviceauth"
)

func TestAnalyticsProducerPreparationReturnsOnlyDraftNativeBinding(t *testing.T) {
	key := bytes.Repeat([]byte{85}, 32)
	owner := "driver-taxi-service"
	kind := "mission-success:provider_id"
	binding := admincontext.RecordBinding{Fingerprint: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", SourceContract: "analytics.cohort-lineage.v1:" + kind, Canonical: adminpolicy.Scope{Product: "karryam"}}
	good, _ := json.Marshal(map[string]any{"state": "DRAFT_PENDING_OWNER_APPROVAL", "binding": binding})
	for _, tc := range []struct {
		name   string
		raw    []byte
		status int
		want   error
	}{
		{"draft", good, 200, nil},
		{"invented approval", bytes.ReplaceAll(good, []byte("DRAFT_PENDING_OWNER_APPROVAL"), []byte("APPROVED")), 200, ErrInvalid},
		{"invented zone", bytes.ReplaceAll(good, []byte(`"zone":""`), []byte(`"zone":"gps-zone"`)), 200, ErrInvalid},
		{"wrong contract", bytes.ReplaceAll(good, []byte("mission-success:provider_id"), []byte("mission-success:branch_id")), 200, ErrInvalid},
		{"raw payload", append(append([]byte{}, good[:len(good)-1]...), []byte(`,"payload":{}}`)...), 200, ErrInvalid},
		{"bad hash", bytes.ReplaceAll(good, []byte(binding.Fingerprint), []byte("bad")), 200, ErrInvalid},
		{"unavailable", nil, 503, ErrUnavailable},
		{"denied", nil, 403, ErrDenied},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != "GET" || r.URL.Path != SourceContextPath+owner+"/bindings/"+kind+"/native-event" || r.URL.Query().Get("purpose") != "TEST_ONLY_REVIEW" {
					t.Error("not fixed preparation request")
				}
				if _, err := serviceauth.NewVerifier(serviceauth.Secrets{ProducerPublisherCaller: key}, 30*time.Second).VerifyRequest(r); err != nil {
					t.Error(err)
				}
				w.WriteHeader(tc.status)
				w.Write(tc.raw)
			}))
			defer server.Close()
			client, err := NewPublisherClient(server.URL, owner, key)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			out, err := client.PrepareBinding(context.Background(), kind, "native-event", "TEST_ONLY_REVIEW")
			if !errors.Is(err, tc.want) {
				t.Fatal(err)
			}
			if err == nil && out != binding {
				t.Fatal(out)
			}
			if calls != 1 {
				t.Fatal("retry", calls)
			}
			for _, ref := range []string{"../other", "has/slash", "%2f", "?tenant=other"} {
				if _, err := client.PrepareBinding(context.Background(), kind, ref, "TEST_ONLY_REVIEW"); !errors.Is(err, ErrInvalid) || calls != 1 {
					t.Fatal("invalid reference reached source", ref)
				}
			}
		})
	}
}
