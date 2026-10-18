package adminpolicy_test

import (
	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/adminpolicy/testfixture"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestOwnerPolicyIntersectionAndUnmappedReferences(t *testing.T) {
	scope := adminpolicy.Scope{Product: "karryam", Market: "M1", Zone: "Z1", Organization: "O1", Tenant: "O1"}
	f := testfixture.New(t, scope, "customer-service", "customers.read", "customer.scoped-profile.v1")
	actor := adminpolicy.Actor{Product: "karryam", Staff: "staff-1", Roles: []string{"staff"}, Permissions: []string{"customers.read"}, Markets: []string{"M1", "M2"}, Zones: []string{"Z1", "Z2"}, Organizations: []string{"O1", "O2"}, Tenants: []string{"O1", "O2"}}
	facts := adminpolicy.Facts{Owner: "customer-service", Reference: "customer-1", SourceContract: "customer.scoped-profile.v1", Scope: scope}
	for _, name := range []string{"allowed", "product", "permission", "role", "purpose", "market", "zone", "organization", "tenant", "owner", "contract", "missing_mapping", "cross_tuple", "expired"} {
		t.Run(name, func(t *testing.T) {
			a := actor
			s := scope
			source := facts
			purpose := "case-1"
			now := time.Now()
			switch name {
			case "product":
				a.Product = "other"
			case "permission":
				a.Permissions = nil
			case "role":
				a.Roles = []string{"other"}
			case "purpose":
				purpose = "unapproved"
			case "market":
				s.Market = "M2"
			case "zone":
				s.Zone = "Z2"
			case "organization":
				s.Organization = "O2"
			case "tenant":
				s.Tenant = "O2"
			case "owner":
				source.Owner = "fleet-partner-service"
			case "contract":
				source.SourceContract = "unverified-contract"
			case "missing_mapping":
				source.Scope.Market = ""
			case "cross_tuple":
				source.Scope.Organization = "O2"
			case "expired":
				now = f.Document.ExpiresAt
			}
			err := f.Engine.AuthorizeOwner(a, "customers.read", purpose, s, source, now)
			if (err == nil) != (name == "allowed") {
				t.Fatalf("%s: %v", name, err)
			}
		})
	}
	// A returned rule is a copy: consumers cannot expand policy with a map edit.
	r, _ := f.Engine.Rule("customers.read", time.Now())
	r.Scopes[0].Tenant = "O2"
	r.Roles[0] = "other"
	r.CanonicalFields["market"] = "forged"
	if err := f.Engine.AuthorizeOwner(actor, "customers.read", "case-1", scope, facts, time.Now()); err != nil {
		t.Fatal("policy mutated through returned rule", err)
	}
}

func TestDraftRepositoryPolicyAndMissingCanonicalMappingsDeny(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(file), "../../../docs/admin/policies/admin-access-policy-v1.draft.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = adminpolicy.Verify(raw, nil, adminpolicy.Production, time.Now()); !errors.Is(err, adminpolicy.ErrBlocked) {
		t.Fatalf("repository draft enabled access: %v", err)
	}
	for _, owner := range []string{"customer-service", "driver-dispatch-delivery-service", "driver-hauling-service"} {
		t.Run(owner, func(t *testing.T) {
			scope := adminpolicy.Scope{Product: "karryam", Market: "M1", Zone: "Z1", Organization: "O1", Tenant: "O1"}
			f := testfixture.New(t, scope, owner, "entities.read", "test-only-source-contract")
			actor := adminpolicy.Actor{Product: "karryam", Staff: "staff-1", Roles: []string{"staff"}, Permissions: []string{"entities.read"}, Markets: []string{"M1"}, Zones: []string{"Z1"}, Organizations: []string{"O1"}, Tenants: []string{"O1"}}
			facts := adminpolicy.Facts{Owner: owner, Reference: "actual-owner-record", SourceContract: "test-only-source-contract", Scope: adminpolicy.Scope{Product: "karryam", Organization: "O1"}}
			if err = f.Engine.AuthorizeOwner(actor, "entities.read", "case-1", scope, facts, time.Now()); err == nil {
				t.Fatal("policy manufactured missing owner geography/tenant")
			}
		})
	}
}
func TestPolicyApprovalAuthenticityEnvironmentAndDraft(t *testing.T) {
	f := testfixture.New(t, adminpolicy.Scope{Product: "karryam", Market: "M1", Zone: "Z1", Organization: "O1", Tenant: "O1"}, "driver-taxi-service", "taxi.rides.read", "taxi.corporate-rides.v1")
	keys := map[string]ed25519.PublicKey{"test-only-key": f.Public}
	if _, err := adminpolicy.Verify(f.Envelope, keys, adminpolicy.Production, time.Now()); err == nil {
		t.Fatal("test-only approval accepted in production")
	}
	if _, err := adminpolicy.Verify(f.Envelope, nil, adminpolicy.TestOnly, time.Now()); err == nil {
		t.Fatal("unpinned approval accepted")
	}
	var env adminpolicy.Envelope
	json.Unmarshal(f.Envelope, &env)
	var doc adminpolicy.Document
	json.Unmarshal(env.Document, &doc)
	doc.Rules[0].Scopes[0].Tenant = "foreign"
	env.Document, _ = json.Marshal(doc)
	tampered, _ := json.Marshal(env)
	if _, err := adminpolicy.Verify(tampered, keys, adminpolicy.TestOnly, time.Now()); err == nil {
		t.Fatal("tampered approval accepted")
	}
	env.Document = json.RawMessage(`{"schema_version":1,"state":"DRAFT"}`)
	draft, _ := json.Marshal(env)
	if _, err := adminpolicy.Verify(draft, keys, adminpolicy.TestOnly, time.Now()); !errors.Is(err, adminpolicy.ErrBlocked) {
		t.Fatalf("draft enabled: %v", err)
	}
	if _, err := adminpolicy.Verify(nil, keys, adminpolicy.TestOnly, time.Now()); !errors.Is(err, adminpolicy.ErrBlocked) {
		t.Fatal(err)
	}
	var absent *adminpolicy.Engine
	if _, err := absent.Rule("taxi.rides.read", time.Now()); !errors.Is(err, adminpolicy.ErrBlocked) {
		t.Fatal("missing policy failed open")
	}
}
