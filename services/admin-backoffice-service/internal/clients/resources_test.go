package clients

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/adminowner"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/serviceauth"
)

func TestOwnerResourcesWireContract(t *testing.T) {
	secret := []byte(strings.Repeat("s", 32))
	for _, profile := range adminowner.Profiles() {
		for kind, definition := range profile.Definitions {
			t.Run(profile.Domain+"/"+kind, func(t *testing.T) {
				var mode atomic.Value
				mode.Store("valid")
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if _, err := serviceauth.NewVerifier(serviceauth.Secrets{adminread.Caller: secret}, time.Minute).VerifyRequest(r); err != nil {
						t.Error(err)
						w.WriteHeader(401)
						return
					}
					if r.URL.Path != profile.Path+"/resources/"+kind || r.Header.Get("Authorization") != "" {
						t.Error("unexpected source path or credentials")
					}
					q, err := adminread.Parse(r.URL.Query(), time.Now())
					if err != nil {
						t.Error(err)
						w.WriteHeader(400)
						return
					}
					item := adminowner.Resource{Reference: "record-1", Kind: kind, SnapshotETag: strings.Repeat("a", 64), CreatedAt: q.Start.Add(time.Minute), UpdatedAt: q.Start.Add(time.Minute), Dimensions: []adminowner.Dimension{}, Counters: []adminowner.Counter{}, References: []adminowner.Reference{}, Timeline: []adminowner.TimelineEvent{}}
					for _, name := range definition.Dimensions {
						item.Dimensions = append(item.Dimensions, adminowner.Dimension{Name: name})
					}
					for _, name := range definition.Counters {
						item.Counters = append(item.Counters, adminowner.Counter{Name: name})
					}
					out := adminowner.Snapshot{ContractVersion: adminowner.ContractVersion, Owner: profile.Owner, Kind: kind, Scope: adminpolicy.Scope{Product: "karryam", Market: q.Scope.Market, Zone: q.Scope.Zone, Organization: q.Scope.Organization, Tenant: q.Scope.Tenant}, PolicyID: q.PolicyID, PolicyVersion: q.PolicyVersion, PolicyChecksum: q.PolicyChecksum, AsOf: time.Now().UTC(), Coverage: "CONTEXT_SCOPED_PARTIAL", Items: []adminowner.Resource{item}}
					switch mode.Load().(string) {
					case "tenant":
						out.Scope.Tenant = "another-tenant"
					case "policy":
						out.PolicyVersion++
					case "stale":
						out.AsOf = out.AsOf.Add(-time.Hour)
					case "version":
						v := int64(0)
						out.Items[0].SourceVersion = &v
					case "duplicate":
						out.Items = append(out.Items, item)
					case "unbounded":
						w.Write([]byte(strings.Repeat("x", 1048577)))
						return
					case "redirect":
						w.Header().Set("Location", "http://127.0.0.1:1/private")
						w.WriteHeader(302)
						return
					case "extra":
						raw, _ := json.Marshal(out)
						w.Write(append(raw[:len(raw)-1], []byte(`,"secret":"PRIVATE"}`)...))
						return
					}
					json.NewEncoder(w).Encode(out)
				}))
				defer srv.Close()
				client, err := NewResources(profile.Owner, srv.URL, secret)
				if err != nil {
					t.Fatal(err)
				}
				defer client.Close()
				q := adminread.Query{Scope: adminread.Scope{Market: "M1", Zone: "Z1", Organization: "O1", Tenant: "O1"}, Staff: "staff-1", Role: "staff", Purpose: "case-1", PolicyID: "policy-1", PolicyVersion: 1, PolicyChecksum: strings.Repeat("a", 64), Start: time.Now().UTC().Add(-time.Hour), End: time.Now().UTC(), Limit: 10}
				for _, name := range []string{"valid", "tenant", "policy", "stale", "version", "duplicate", "extra", "unbounded", "redirect"} {
					mode.Store(name)
					_, err := client.Read(context.Background(), q, kind, "")
					if (err == nil) != (name == "valid") {
						t.Fatalf("%s: %v", name, err)
					}
				}
				for _, ref := range []string{"../record", "record/secret", ".."} {
					if _, err := client.Read(context.Background(), q, kind, ref); err == nil {
						t.Fatal("unsafe source path accepted")
					}
				}
			})
		}
	}
}
