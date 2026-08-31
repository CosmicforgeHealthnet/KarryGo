package dispatchcommerce

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/serviceauth"
)

func TestClientUsesCanonicalPathAndHMACV2(t *testing.T) {
	secret := []byte("dispatch-commerce-test-secret")
	verifier := serviceauth.NewVerifier(serviceauth.Secrets{SignatureCaller: secret}, time.Minute)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != APIBase+CapabilityPath {
			t.Errorf("path=%q", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if r.Header.Get(serviceauth.HeaderVersion) != serviceauth.SignatureVersion {
			t.Errorf("signature version=%q", r.Header.Get(serviceauth.HeaderVersion))
		}
		if caller, err := verifier.VerifyRequest(r); err != nil || caller != SignatureCaller {
			t.Errorf("HMAC verification caller=%q err=%v", caller, err)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(Envelope[MotorcycleCapability]{Success: true, Data: MotorcycleCapability{Class: "MOTORCYCLE", Version: 1}})
	}))
	defer server.Close()

	client := NewClient(ClientOptions{BaseURL: server.URL, Secret: string(secret)})
	capability, err := client.Capability(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if capability.Class != "MOTORCYCLE" || capability.Version != 1 {
		t.Fatalf("capability=%+v", capability)
	}
}
