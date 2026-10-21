package http

import (
	"cosmicforge/logistics/services/api-gateway/internal/features/routing/models"
	"testing"
	"time"
)

func TestForgedHQHeadersAreStrippedFromPublicRoutes(t *testing.T) {
	f := newFixture(t, time.Second)
	headers := map[string]string{"X-HQ-Key-ID": "forged", "X-Admin-Actor": "forged", "X-Delegated-Actor": "forged", "X-Service-Name": "admin-backoffice-service", "X-Gateway-Scope": "ADMIN"}
	if w := f.do("GET", "/api/v1/customer/me", nil, headers); w.Code != 200 {
		t.Fatal("public route regressed", w.Code)
	}
	r, _ := f.upstreams[models.UpstreamCustomer].last()
	for name := range headers {
		if name == "X-Gateway-Scope" {
			continue
		}
		if r.Header.Get(name) != "" {
			t.Errorf("forged header forwarded: %s", name)
		}
	}
	if r.Header.Get("X-Gateway-Scope") != "CUSTOMER" {
		t.Fatal("scope forged")
	}
}
