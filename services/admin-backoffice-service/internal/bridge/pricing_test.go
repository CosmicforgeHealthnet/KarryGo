package bridge

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cosmicforge/logistics/services/admin-backoffice-service/internal/clients"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/config"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/adminpolicy/testfixture"
	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/serviceauth"
)

func TestPricingAdminBridgeUsesFixedSignedRoutesAndDeniesUnsupportedActions(t *testing.T) {
	secret := []byte(strings.Repeat("p", 32))
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if _, err := serviceauth.NewVerifier(serviceauth.Secrets{"admin-backoffice-service": secret}, time.Minute).VerifyRequest(r); err != nil {
			t.Error(err)
			w.WriteHeader(401)
			return
		}
		if r.URL.Path != "/internal/pricing/v1/admin/policy-versions/version-1/approve" || r.Header.Get("X-Idempotency-Key") != "approval-command-1" {
			t.Fatalf("unexpected owner request %s %s", r.Method, r.URL.String())
		}
		if r.URL.Query().Get("admin_market") != "NG" || r.URL.Query().Get("admin_zone") != "Z1" || r.URL.Query().Get("admin_organization") != "org-1" || r.URL.Query().Get("admin_tenant") != "org-1" {
			t.Fatalf("delegated five-scope context was not bound to the owner request: %s", r.URL.RawQuery)
		}
		var payload map[string]any
		if json.NewDecoder(r.Body).Decode(&payload) != nil || payload["performed_by"] != "staff-1" || payload["reason"] != "CASE-123" {
			t.Fatalf("delegated staff and purpose were not injected: %#v", payload)
		}
		json.NewEncoder(w).Encode(map[string]any{"status": "APPROVED", "version_id": "version-1"})
	}))
	defer server.Close()
	client, err := clients.NewPricing(server.URL, secret)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	scope := adminpolicy.Scope{Product: "karryam", Market: "NG", Zone: "Z1", Organization: "org-1", Tenant: "org-1"}
	policy := testfixture.New(t, scope, clients.PricingOwner, "pricing.policy.approve", "pricing.admin.v1")
	query := adminread.Query{Scope: adminread.Scope{Market: scope.Market, Zone: scope.Zone, Organization: scope.Organization, Tenant: scope.Tenant}, Purpose: "CASE-123", Start: time.Now().UTC().Add(-time.Hour).Truncate(time.Second), End: time.Now().UTC().Truncate(time.Second), Limit: 10}.Values()
	for _, key := range []string{"staff", "role", "policy_id", "policy_version", "policy_checksum"} {
		query.Del(key)
	}
	body := []byte(`{"idempotency_key":"approval-command-1","expected_body_hash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`)
	for _, test := range []struct {
		name       string
		permission string
		flags      config.DomainFlags
		configured bool
		want       int
		call       bool
	}{
		{"allowed", "pricing.policy.approve", config.DomainFlags{RemoteWrites: true, PricingWrites: true}, true, 200, true},
		{"domain flag off", "pricing.policy.approve", config.DomainFlags{RemoteWrites: true}, true, 503, false},
		{"permission denied", "pricing.policy.read", config.DomainFlags{RemoteWrites: true, PricingWrites: true}, true, 403, false},
		{"unconfigured", "pricing.policy.approve", config.DomainFlags{RemoteWrites: true, PricingWrites: true}, false, 503, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newFixture(t)
			f.claims.Permissions = []string{test.permission}
			f.claims.Zones, f.claims.Tenants = []string{"Z1"}, []string{"org-1"}
			f.b.ConfigureDomainFlags(test.flags)
			if test.configured {
				if err := f.b.ConfigurePricing(client, policy.Engine, true, true); err != nil {
					t.Fatal(err)
				}
			}
			before := calls.Load()
			response := invoke(f.b, f.request(t, http.MethodPost, "/pricing/policy-versions/version-1/approve?"+query.Encode(), body))
			if response.Code != test.want {
				t.Fatalf("status %d: %s", response.Code, response.Body.String())
			}
			if (calls.Load() != before) != test.call {
				t.Fatal("owner call did not match authorization decision")
			}
		})
	}
}
