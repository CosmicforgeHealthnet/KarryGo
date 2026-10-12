package media

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOnboardingUploadGrantUsesPrivateAuthorityAndPublicOrigin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/media-files/internal/media/upload-authorizations" || r.Method != "POST" {
			t.Errorf("unexpected authority request %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("X-Organization-ID") != "business-a" || r.Header.Get("Idempotency-Key") != "business-a:command-a" || r.Header.Get("X-Request-Fingerprint") == "" {
			t.Error("missing tenant/idempotency binding")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["ownerType"] != "VENDOR_BUSINESS" || body["ownerReference"] != "business-a" || body["visibility"] != "PRIVATE" || body["uploadPurpose"] != "COMPLIANCE_DOCUMENT" || body["retentionClass"] != "COMPLIANCE" {
			t.Error("incorrect private document ownership")
		}
		json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"uploadAuthorizationId": "grant-a", "mediaObjectId": "object-a", "uploadToken": "token-a", "expiresAt": time.Now().Add(time.Minute), "maxSizeBytes": 100, "allowedMimeTypes": []string{"application/pdf"}}})
	}))
	defer server.Close()
	client := NewClient(server.URL, "test-service-secret", time.Second).WithPublicURL("https://media.example.com")
	grant, err := client.AuthorizeOnboardingUpload(context.Background(), "business-a", "BUSINESS", "application/pdf", 100, "command-a")
	if err != nil {
		t.Fatal(err)
	}
	if grant.URL != "https://media.example.com/api/v1/media-files/vendor-uploads/object-a/content" || grant.MediaRef != "object-a" || grant.Token != "token-a" {
		t.Fatalf("incorrect grant %#v", grant)
	}
}
func TestOnboardingUploadRequiresPublicOrigin(t *testing.T) {
	for _, origin := range []string{"", "https://user:pass@media.example.com", "https://media.example.com/internal", "https://media.example.com?secret=x", "file:///tmp/media"} {
		_, err := NewClient("http://127.0.0.1:1", "secret", time.Second).WithPublicURL(origin).AuthorizeOnboardingUpload(context.Background(), "business-a", "BUSINESS", "application/pdf", 100, "command-a")
		if err == nil {
			t.Fatalf("accepted %q", origin)
		}
	}
}
