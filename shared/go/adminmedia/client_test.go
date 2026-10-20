package adminmedia

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/google/uuid"
)

func TestPrivateMediaCaseClientUsesFixedPathsAndRejectsUnboundOrRawResponses(t *testing.T) {
	key := bytes.Repeat([]byte{89}, 32)
	actor := Actor{Product: "karryam", Staff: "staff-1", ExpiresAt: time.Now().Add(time.Minute)}
	q := Request{Scope: adminpolicy.Scope{Product: "karryam", Market: "M1", Zone: "Z1", Organization: "O1", Tenant: "O1"}, Purpose: "case-1", SourceCapability: "dispatch.media.case.read", CaseOwner: "driver-dispatch-delivery-service", CaseKind: "TEST_ONLY_CASE", CaseReference: "case-1", SubjectType: "BOOKING", SubjectReference: "booking-1", MediaObjectID: "mobj_test"}
	grant := Grant{ContractVersion: Contract, ID: uuid.NewString(), Token: base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{90}, 32)), MediaObjectID: q.MediaObjectID, ExpiresAt: time.Now().Add(30 * time.Second)}
	raw, _ := json.Marshal(grant)
	for _, tc := range []struct {
		name   string
		body   []byte
		status int
		want   error
	}{
		{"valid", raw, 200, nil},
		{"foreign object", bytes.ReplaceAll(raw, []byte("mobj_test"), []byte("mobj_other")), 200, ErrInvalid},
		{"raw storage key", append(append([]byte{}, raw[:len(raw)-1]...), []byte(`,"storage_key":"PRIVATE"}`)...), 200, ErrInvalid},
		{"duplicate token", append(append([]byte{}, raw[:len(raw)-1]...), []byte(`,"token":"forged"}`)...), 200, ErrInvalid},
		{"case alias", append(append([]byte{}, raw[:len(raw)-1]...), []byte(`,"Token":"forged"}`)...), 200, ErrInvalid},
		{"redirect", nil, 302, ErrUnavailable},
		{"denied", nil, 403, ErrDenied},
		{"missing", nil, 404, ErrNotFound},
		{"unavailable", nil, 503, ErrUnavailable},
		{"oversized", bytes.Repeat([]byte{'x'}, MaxBody+1), 200, ErrInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != "POST" || r.URL.Path != GrantPath || r.URL.RawQuery != "" || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
					t.Error("not isolated fixed case path")
				}
				if _, err := serviceauth.NewVerifier(serviceauth.Secrets{Caller: key}, 30*time.Second).VerifyRequest(r); err != nil {
					t.Error(err)
				}
				w.WriteHeader(tc.status)
				w.Write(tc.body)
			}))
			defer s.Close()
			c, err := NewClient(s.URL, key)
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			if _, err := c.Issue(context.Background(), actor, q); !errors.Is(err, tc.want) {
				t.Fatal(err)
			}
			if calls != 1 {
				t.Fatal("automatic retry", calls)
			}
			if _, err := c.Read(context.Background(), actor, q, "../other", grant.Token); !errors.Is(err, ErrInvalid) || calls != 1 {
				t.Fatal("invalid path sent")
			}
		})
	}
	for _, origin := range []string{"https://host/untrusted", "https://user:pass@host", "https://host?target=other", "file:///data"} {
		if _, err := NewClient(origin, key); err == nil {
			t.Fatal("non-bare origin accepted")
		}
	}
}
