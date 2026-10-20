package adminpolicy_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/adminpolicy/testfixture"
)

func TestMediaCasePolicyNeedsExplicitSignedValuesWithoutProductionDefaults(t *testing.T) {
	scope := adminpolicy.Scope{Product: "karryam", Market: "M1", Zone: "Z1", Organization: "O1", Tenant: "O1"}
	f := testfixture.New(t, scope, "media-file-service", "media.case.content.read", "media.admin-case-content.v1")
	if _, err := f.Engine.MediaDisclosure(time.Now()); !errors.Is(err, adminpolicy.ErrBlocked) {
		t.Fatal("no signed media values became defaults")
	}
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		value *adminpolicy.MediaDisclosure
		valid bool
	}{
		{"explicit TEST_ONLY", &adminpolicy.MediaDisclosure{MaxGrantSeconds: 60, SourceCapabilities: []string{"dispatch.media.case.read"}}, true},
		{"missing lifetime", &adminpolicy.MediaDisclosure{SourceCapabilities: []string{"dispatch.media.case.read"}}, false},
		{"missing source capabilities", &adminpolicy.MediaDisclosure{MaxGrantSeconds: 60}, false},
		{"duplicate source capability", &adminpolicy.MediaDisclosure{MaxGrantSeconds: 60, SourceCapabilities: []string{"dispatch.media.case.read", "dispatch.media.case.read"}}, false},
		{"unbounded lifetime", &adminpolicy.MediaDisclosure{MaxGrantSeconds: 86401, SourceCapabilities: []string{"dispatch.media.case.read"}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := f.Document
			doc.Media = tc.value
			raw, _ := json.Marshal(doc)
			envelope, _ := json.Marshal(adminpolicy.Envelope{Document: raw, KeyID: "TEST_ONLY_KEY", Signature: base64.RawURLEncoding.EncodeToString(ed25519.Sign(key, append([]byte(adminpolicy.SigningDomain), raw...)))})
			engine, err := adminpolicy.Verify(envelope, map[string]ed25519.PublicKey{"TEST_ONLY_KEY": pub}, adminpolicy.TestOnly, time.Now())
			if tc.valid != (err == nil) {
				t.Fatal(err)
			}
			if tc.valid {
				p, err := engine.MediaDisclosure(time.Now())
				if err != nil {
					t.Fatal(err)
				}
				p.SourceCapabilities[0] = "forged"
				again, _ := engine.MediaDisclosure(time.Now())
				if again.SourceCapabilities[0] == "forged" {
					t.Fatal("caller mutated signed media policy")
				}
				if _, err := adminpolicy.Verify(envelope, map[string]ed25519.PublicKey{"TEST_ONLY_KEY": pub}, adminpolicy.Production, time.Now()); err == nil {
					t.Fatal("TEST_ONLY media values reached production")
				}
			}
		})
	}
}
