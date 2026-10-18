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
	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/google/uuid"
)

func TestTaxiReadDelegationAndOwnerBoundary(t *testing.T) {
	secret := []byte(strings.Repeat("t", 32))
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if _, err := serviceauth.NewVerifier(serviceauth.Secrets{adminread.Caller: secret}, time.Minute).VerifyRequest(r); err != nil {
			t.Error(err)
			w.WriteHeader(401)
			return
		}
		if r.Header.Get("Authorization") != "" || r.Header.Get("X-HQ-Key-ID") != "" {
			t.Error("HQ credentials leaked to owner")
		}
		if r.URL.Path != adminread.TaxiPath || r.URL.Query().Get("staff") != "staff-1" {
			t.Error("wrong fixed owner or delegated identity")
		}
		q, err := adminread.Parse(r.URL.Query(), time.Now())
		if err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		json.NewEncoder(w).Encode(adminread.TaxiSnapshot{ContractVersion: "1.0", Scope: q.Scope, AsOf: time.Now().UTC(), Freshness: "LIVE", Coverage: "PARTIAL_COVERAGE", Items: []adminread.Ride{{ID: uuid.NewString(), Status: "completed", Mode: "standard", SourceContext: "FLEET_ASSIGNMENT", CreatedAt: q.Start.Add(time.Hour), UpdatedAt: q.Start.Add(time.Hour)}}, Events: []adminread.Event{}})
	}))
	defer srv.Close()
	client, err := clients.NewTaxi(srv.URL, secret)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	q := adminread.Query{Scope: adminread.Scope{Market: "NG", Zone: "Z1", Organization: "org-1"}, Start: time.Now().UTC().Add(-24 * time.Hour).Truncate(time.Second), End: time.Now().UTC().Truncate(time.Second), Limit: 10, Purpose: "case-1"}
	values := q.Values()
	values.Del("staff")
	path := "/taxi/rides?" + values.Encode()
	for _, name := range []string{"allowed", "zone_denied", "zone_missing", "role_denied", "permission_denied", "market_denied", "organization_denied", "actor_injection", "audit_unavailable", "disabled"} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			f.claims.Zones = []string{"Z1"}
			if err := f.b.ConfigureTaxi(client, []string{"staff"}); err != nil {
				t.Fatal(err)
			}
			target := path
			want := 200
			switch name {
			case "zone_denied":
				f.claims.Zones = []string{"Z2"}
				want = 403
			case "zone_missing":
				f.claims.Zones = nil
				want = 403
			case "role_denied":
				f.claims.Roles = []string{"other"}
				want = 403
			case "permission_denied":
				f.claims.Permissions = []string{"bridge.health.read"}
				want = 403
			case "market_denied":
				f.claims.Markets = []string{"M2"}
				want = 403
			case "organization_denied":
				f.claims.Organizations = []string{"org-2"}
				want = 403
			case "actor_injection":
				target += "&staff=forged"
				want = 400
			case "audit_unavailable":
				f.repo.auditUnavailable = true
				want = 503
			case "disabled":
				f.b.taxi = nil
				want = 503
			}
			before := calls.Load()
			w := invoke(f.b, f.request(t, "GET", target, nil))
			if w.Code != want {
				t.Fatalf("status=%d want=%d %s", w.Code, want, w.Body)
			}
			if want != 200 && strings.Contains(w.Body.String(), `"items"`) {
				t.Fatal("failed request leaked data")
			}
			if name != "allowed" && name != "audit_unavailable" && before != calls.Load() {
				t.Fatal("denied request reached owner")
			}
			if name == "allowed" && (len(f.repo.audits) != 1 || f.repo.audits[0].Zone != "Z1") {
				t.Fatal("zone audit missing")
			}
		})
	}
}
