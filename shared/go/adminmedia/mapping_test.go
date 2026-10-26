package adminmedia

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/google/uuid"
)

func TestLegacyMappingClientRejectsForgedOrMismatchedMappings(t *testing.T) {
	key := []byte(strings.Repeat("k", 32))
	legacy := uuid.NewString()
	good := LegacyMapping{ContractVersion: LegacyMappingContract, LegacyAssetID: legacy, MediaObjectID: "mob_1", AttestationDigest: strings.Repeat("a", 64)}
	for _, tc := range []struct {
		name   string
		status int
		body   LegacyMapping
		want   error
	}{
		{"valid", 200, good, nil},
		{"other legacy asset", 200, LegacyMapping{ContractVersion: LegacyMappingContract, LegacyAssetID: uuid.NewString(), MediaObjectID: "mob_1", AttestationDigest: good.AttestationDigest}, ErrInvalid},
		{"forged digest", 200, LegacyMapping{ContractVersion: LegacyMappingContract, LegacyAssetID: legacy, MediaObjectID: "mob_1", AttestationDigest: "not-a-digest"}, ErrInvalid},
		{"uppercase digest alias", 200, LegacyMapping{ContractVersion: LegacyMappingContract, LegacyAssetID: legacy, MediaObjectID: "mob_1", AttestationDigest: strings.Repeat("A", 64)}, ErrInvalid},
		{"url as object id", 200, LegacyMapping{ContractVersion: LegacyMappingContract, LegacyAssetID: legacy, MediaObjectID: "https://x/y?z", AttestationDigest: good.AttestationDigest}, ErrInvalid},
		{"contract", 200, LegacyMapping{ContractVersion: "v0", LegacyAssetID: legacy, MediaObjectID: "mob_1", AttestationDigest: good.AttestationDigest}, ErrInvalid},
		{"unadopted", 404, LegacyMapping{}, ErrNotFound},
		{"denied", 403, LegacyMapping{}, ErrDenied},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" || r.URL.Path != LegacyMappingPath || r.URL.RawQuery != "" {
					t.Error("not the fixed mapping path")
				}
				if _, err := serviceauth.NewVerifier(serviceauth.Secrets{Caller: key}, 30*time.Second).VerifyRequest(r); err != nil {
					t.Error(err)
				}
				w.WriteHeader(tc.status)
				if tc.status == 200 {
					_ = json.NewEncoder(w).Encode(tc.body)
				}
			}))
			defer s.Close()
			c, err := NewClient(s.URL, key)
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			out, err := c.LegacyMapping(context.Background(), legacy)
			if !errors.Is(err, tc.want) || err == nil && out != good {
				t.Fatal(tc.name, out, err)
			}
		})
	}
	c, err := NewClient("http://127.0.0.1:1", key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.LegacyMapping(context.Background(), "../escape"); !errors.Is(err, ErrInvalid) {
		t.Fatal("invalid legacy id sent")
	}
}
