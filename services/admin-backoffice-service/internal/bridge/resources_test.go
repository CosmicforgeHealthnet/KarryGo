package bridge

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cosmicforge/logistics/services/admin-backoffice-service/internal/clients"
	"cosmicforge/logistics/shared/go/adminowner"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/adminpolicy/testfixture"
	"cosmicforge/logistics/shared/go/adminread"
)

func TestOwnerResourcesDelegation(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(404) }))
	defer srv.Close()
	for _, profile := range adminowner.Profiles() {
		for kind, definition := range profile.Definitions {
			t.Run(profile.Domain+"/"+kind, func(t *testing.T) {
				client, err := clients.NewResources(profile.Owner, srv.URL, []byte(strings.Repeat("s", 32)))
				if err != nil {
					t.Fatal(err)
				}
				defer client.Close()
				policy := testfixture.New(t, adminpolicy.Scope{Product: "karryam", Market: "NG", Zone: "Z1", Organization: "org-1", Tenant: "org-1"}, profile.Owner, definition.Capability, definition.SourceContract)
				q := adminread.Query{Scope: adminread.Scope{Market: "NG", Zone: "Z1", Organization: "org-1", Tenant: "org-1"}, Purpose: "case-1", Start: time.Now().UTC().Add(-time.Hour).Truncate(time.Second), End: time.Now().UTC().Truncate(time.Second), Limit: 10}
				values := q.Values()
				for _, key := range []string{"staff", "role", "policy_id", "policy_version", "policy_checksum"} {
					values.Del(key)
				}
				for _, name := range []string{"allowed", "zone", "tenant", "organization", "permission", "role", "policy", "spoofed_actor", "unconfigured"} {
					f := newFixture(t)
					f.claims.Permissions = []string{definition.Capability}
					f.claims.Zones = []string{"Z1"}
					f.claims.Tenants = []string{"org-1"}
					if name != "unconfigured" {
						if err := f.b.ConfigureResources(client, policy.Engine); err != nil {
							t.Fatal(err)
						}
					}
					path := "/" + profile.Domain + "/resources/" + kind + "?" + values.Encode()
					want := 403
					switch name {
					case "allowed":
						want = 404
					case "zone":
						f.claims.Zones = []string{"Z2"}
					case "tenant":
						f.claims.Tenants = []string{"org-2"}
					case "organization":
						f.claims.Organizations = []string{"org-2"}
					case "permission":
						f.claims.Permissions = []string{"bridge.health.read"}
					case "role":
						f.claims.Roles = []string{"other"}
					case "policy":
						f.b.resourcePolicies[profile.Owner] = nil
						want = 503
					case "spoofed_actor":
						path += "&staff=another"
						want = 400
					case "unconfigured":
						want = 503
					}
					before := calls.Load()
					res := invoke(f.b, f.request(t, "GET", path, nil))
					if res.Code != want {
						t.Fatalf("%s: %d %s", name, res.Code, res.Body.String())
					}
					if (calls.Load() != before) != (name == "allowed") {
						t.Fatalf("%s bypassed delegation or failed to call owner", name)
					}
				}
			})
		}
	}
}
