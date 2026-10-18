package adminread

import (
	"context"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/adminpolicy/testfixture"
	contract "cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOwnerPolicyCannotInventCanonicalTenant(t *testing.T) {
	db := ownerDB(t)
	now := time.Now().UTC().Truncate(time.Second)
	id := seedRide(t, db, "M1", "Z1", "O1", now.Add(-time.Hour))
	secret := []byte(strings.Repeat("p", 32))
	// Even a test-approved grant for a different tenant cannot override the
	// actual Fleet-origin organization resolved by Taxi's authoritative join.
	policy := testfixture.NewTaxi(t, adminpolicy.Scope{Product: "karryam", Market: "M1", Zone: "Z1", Organization: "O1", Tenant: "O2"})
	q := contract.Query{Scope: contract.Scope{Market: "M1", Zone: "Z1", Organization: "O1", Tenant: "O2"}, Staff: "staff-1", Role: "staff", Purpose: "case-1", Start: now.Add(-24 * time.Hour), End: now, Limit: 10, PolicyID: policy.Engine.ID(), PolicyVersion: policy.Engine.Version(), PolicyChecksum: policy.Engine.Checksum()}
	call := func(engine *adminpolicy.Engine, want int) {
		t.Helper()
		root := gin.New()
		Register(root, db, secret, true, engine)
		srv := httptest.NewServer(root)
		defer srv.Close()
		r, _ := http.NewRequest("GET", srv.URL+contract.TaxiPath+"/"+id+"?"+q.Values().Encode(), nil)
		if err := serviceauth.SignRequest(r, contract.Caller, secret, nil, time.Now()); err != nil {
			t.Fatal(err)
		}
		res, err := srv.Client().Do(r)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != want || strings.Contains(string(raw), `"items"`) {
			t.Fatalf("tenant/policy denial=%d %s", res.StatusCode, raw)
		}
	}
	call(policy.Engine, 403)
	call(nil, 503)
	var audits int
	if err := db.QueryRow(context.Background(), `SELECT count(*) FROM taxi_admin_read_audit`).Scan(&audits); err != nil || audits != 0 {
		t.Fatalf("unauthorized read acknowledged as authorized: %d %v", audits, err)
	}
}
