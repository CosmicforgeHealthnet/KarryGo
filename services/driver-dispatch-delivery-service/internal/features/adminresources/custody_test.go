package adminresources

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/admincontext"
	contextfixture "cosmicforge/logistics/shared/go/admincontext/testfixture"
	"cosmicforge/logistics/shared/go/adminowner"
	fixture "cosmicforge/logistics/shared/go/adminowner/testfixture"
	"cosmicforge/logistics/shared/go/adminpolicy"
	policyfixture "cosmicforge/logistics/shared/go/adminpolicy/testfixture"
	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/gin-gonic/gin"
)

func TestCustodyInventoryUsesActualScopedMultiManifestRecords(t *testing.T) {
	db := fixture.Database(t, "../../../migrations", true)
	ctx := context.Background()
	for _, sql := range []string{
		`INSERT INTO custody_missions(mission_ref,authority,tenant_ref,custody_mode) VALUES('mission-visible','CUSTOMER','tenant-1','MULTI_MANIFEST'),('mission-foreign','VENDOR','tenant-2','MULTI_MANIFEST')`,
		`INSERT INTO custody_mission_state(mission_ref,tenant_ref,status,version,receiver_challenge_ref) VALUES('mission-visible','tenant-1','TRANSFER_REQUIRED',4,'SECRET_CHALLENGE'),('mission-foreign','tenant-2','PICKUP_REQUIRED',1,'FOREIGN_CHALLENGE')`,
		`INSERT INTO custody_mission_stops(mission_ref,stop_ref,stop_type,sequence) VALUES('mission-visible','pickup-1','PICKUP',1),('mission-visible','dropoff-1','DROPOFF',2),('mission-foreign','pickup-foreign','PICKUP',1)`,
		`INSERT INTO custody_stop_manifests(mission_ref,manifest_ref,manifest_version,pickup_stop_ref,source_authority,source_manifest_checksum,package_count,status,current_custodian_ref,operation_ref) VALUES('mission-visible','manifest-visible',3,'pickup-1','CUSTOMER','source-checksum',2,'COLLECTED','provider-2','register-1'),('mission-foreign','manifest-foreign',1,'pickup-foreign','VENDOR','foreign-checksum',1,'REGISTERED','','register-foreign')`,
		`INSERT INTO custody_manifest_packages(mission_ref,manifest_ref,manifest_version,package_ref,package_count,status) VALUES('mission-visible','manifest-visible',3,'package-1',1,'COLLECTED'),('mission-visible','manifest-visible',3,'package-2',1,'COLLECTED'),('mission-foreign','manifest-foreign',1,'FOREIGN_PACKAGE',1,'EXPECTED')`,
		`INSERT INTO custody_manifest_holdings(id,mission_ref,manifest_ref,manifest_version,custodian_ref,acquired_operation_ref,acquired_via) VALUES('holding-1','mission-visible','manifest-visible',3,'provider-2','transfer-1','PROVIDER_TRANSFER')`,
		`INSERT INTO custody_transfer_items(transfer_operation_ref,mission_ref,manifest_ref,manifest_version,from_custodian_ref,to_custodian_ref,package_count) VALUES('transfer-1','mission-visible','manifest-visible',3,'provider-1','provider-2',2)`,
	} {
		if _, err := db.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	source := Source()
	profile, _ := adminowner.ForOwner(Owner)
	definition := profile.Definitions["custody-missions"]
	if definition.SourceContract != "dispatch.admin-custody.v2" {
		t.Fatal("larger disclosure reused a prior v1 approval contract")
	}
	scope := adminpolicy.Scope{Product: "karryam", Market: "M1", Zone: "Z1", Organization: "org-1", Tenant: "tenant-1"}
	policy := policyfixture.New(t, scope, Owner, definition.Capability, definition.SourceContract)
	foreign := scope
	foreign.Tenant = "tenant-2"
	policy.Document.Rules[0].Scopes = append(policy.Document.Rules[0].Scopes, foreign)
	policy.Sign(t)
	issuer := contextfixture.New(t, Owner)
	store := &admincontext.Store{DB: db, Reader: source, Authority: issuer.Authority}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := source.Binding(ctx, tx, "custody-missions", "mission-visible")
	tx.Rollback(ctx)
	if err != nil {
		t.Fatal(err)
	}
	approval := issuer.Approval("test-custody-inventory", "custody-missions", "mission-visible", definition.Capability, binding, scope)
	if err := store.Import(ctx, issuer.Sign(t, approval), time.Now()); err != nil {
		t.Fatal(err)
	}
	secret := []byte(strings.Repeat("s", 32))
	h := &adminowner.Handler{Owner: Owner, Path: profile.Path, DB: db, Source: source, Contexts: store, Policy: policy.Engine, Secret: secret, Enabled: true, Definitions: profile.Definitions}
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	h.Register(engine)
	query := adminread.Query{Scope: adminread.Scope{Market: scope.Market, Zone: scope.Zone, Organization: scope.Organization, Tenant: scope.Tenant}, Staff: "staff-1", Role: "staff", Purpose: "case-1", PolicyID: policy.Engine.ID(), PolicyVersion: policy.Engine.Version(), PolicyChecksum: policy.Engine.Checksum(), Start: time.Now().UTC().Add(-time.Hour).Truncate(time.Second), End: time.Now().UTC().Add(time.Second).Truncate(time.Second), Limit: 1}
	read := func(q adminread.Query, ref string) *httptest.ResponseRecorder {
		t.Helper()
		path := profile.Path + "/resources/custody-missions"
		if ref != "" {
			path += "/" + ref
		}
		req := httptest.NewRequest("GET", path+"?"+q.Values().Encode(), nil)
		if err := serviceauth.SignRequest(req, adminread.Caller, secret, nil, time.Now()); err != nil {
			t.Fatal(err)
		}
		res := httptest.NewRecorder()
		engine.ServeHTTP(res, req)
		return res
	}
	res := read(query, "mission-visible")
	if res.Code != 200 {
		t.Fatalf("scoped inventory: %d %s", res.Code, res.Body.String())
	}
	var snapshot adminowner.Snapshot
	if err := json.Unmarshal(res.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Items) != 1 || snapshot.Items[0].Custody == nil {
		t.Fatal("custody detail missing")
	}
	c := snapshot.Items[0].Custody
	if len(c.Stops) != 2 || len(c.Manifests) != 1 || len(c.Packages) != 2 || len(c.Holdings) != 1 || len(c.Transfers) != 1 || c.Manifests[0].ManifestVersion != 3 || c.Transfers[0].From != "provider-1" || c.Transfers[0].To != "provider-2" || c.Truncated {
		t.Fatalf("canonical inventory changed: %+v", c)
	}
	for _, private := range []string{"SECRET_CHALLENGE", "FOREIGN_CHALLENGE", "FOREIGN_PACKAGE", "foreign-checksum", "manifest-foreign"} {
		if strings.Contains(res.Body.String(), private) {
			t.Fatalf("foreign/private custody data leaked: %s", private)
		}
	}
	if output := os.Getenv("ADMIN_OWNER_CUSTODY_CAPTURE_DIR"); output != "" {
		if err := os.MkdirAll(output, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(output, "custody.json"), res.Body.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if list := read(query, ""); list.Code != 200 || strings.Contains(list.Body.String(), `"custody":`) {
		t.Fatalf("list expanded restricted inventory: %d %s", list.Code, list.Body.String())
	}
	other := query
	other.Scope.Tenant = "tenant-2"
	if denied := read(other, "mission-visible"); denied.Code != 404 {
		t.Fatalf("governance scope replaced source tenant: %d %s", denied.Code, denied.Body.String())
	}
	if denied := read(other, "mission-foreign"); denied.Code != 404 {
		t.Fatalf("unmapped foreign mission disclosed: %d", denied.Code)
	}
	oldApproval := approval
	oldApproval.ID = "test-old-custody-contract"
	oldApproval.SourceContract = "dispatch.admin-resources.v1"
	if err := store.Import(ctx, issuer.Sign(t, oldApproval), time.Now()); err == nil {
		t.Fatal("old source contract authorized larger custody disclosure")
	}
	oldGovernance := policyfixture.New(t, scope, Owner, definition.Capability, "dispatch.admin-resources.v1")
	h.Policy = oldGovernance.Engine
	oldQuery := query
	oldQuery.PolicyID = h.Policy.ID()
	oldQuery.PolicyVersion = h.Policy.Version()
	oldQuery.PolicyChecksum = h.Policy.Checksum()
	if denied := read(oldQuery, "mission-visible"); denied.Code != 403 || strings.Contains(denied.Body.String(), `"custody":`) {
		t.Fatalf("old governance contract authorized inventory: %d %s", denied.Code, denied.Body.String())
	}
	h.Policy = policy.Engine
	// Prove a bounded result rather than silently omitting rows or guessing a
	// complete inventory. IDs/counters come from the real owner's schema.
	if _, err := db.Exec(ctx, `INSERT INTO custody_manifest_packages(mission_ref,manifest_ref,manifest_version,package_ref,package_count) SELECT 'mission-visible','manifest-visible',3,'extra-package-'||n,1 FROM generate_series(1,105) n`); err != nil {
		t.Fatal(err)
	}
	res = read(query, "mission-visible")
	if res.Code != 200 {
		t.Fatal(res.Code, res.Body.String())
	}
	if err := json.Unmarshal(res.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	if !snapshot.Items[0].Custody.Truncated || len(snapshot.Items[0].Custody.Packages) != 100 {
		t.Fatal("custody limit was silent or unbounded")
	}
}

func TestCustodyLegacyInventoryUsesExactFrozenManifestAndOmitsRawParcels(t *testing.T) {
	db := fixture.Database(t, "../../../migrations", true)
	ctx := context.Background()
	for _, sql := range []string{
		`INSERT INTO custody_missions(mission_ref,authority,tenant_ref,custody_mode) VALUES('mission-legacy','CUSTOMER','tenant-legacy','LEGACY_SINGLE')`,
		`INSERT INTO custody_manifests(mission_ref,authority,tenant_ref,business_ref,branch_ref,manifest_ref,manifest_version,pickup_stop_ref,parcel_refs,parcels,package_count,created_at,frozen_at,status,operation_ref,request_fingerprint) VALUES('mission-legacy','CUSTOMER','tenant-legacy','business-legacy','branch-legacy','manifest-legacy',4,'pickup-legacy',ARRAY['parcel-legacy'],'[{"private_note":"PRIVATE_LEGACY_CONTENT"}]'::jsonb,1,now(),now(),'FROZEN','freeze-legacy','fingerprint-legacy')`,
		`INSERT INTO custody_mission_state(mission_ref,tenant_ref,status,version,manifest_ref,manifest_version) VALUES('mission-legacy','tenant-legacy','PICKUP_REQUIRED',2,'manifest-legacy',4)`,
	} {
		if _, err := db.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	source := Source()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	resource, err := source.Read(ctx, tx, "custody-missions", "mission-legacy", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := adminowner.ValidateResource(resource, source.Specs["custody-missions"].Definition, true); err != nil {
		t.Fatal(err)
	}
	legacy := resource.Custody.Legacy
	if legacy == nil || legacy.Reference != "manifest-legacy" || legacy.Version != 4 || len(legacy.ParcelReferences) != 1 || legacy.ParcelReferences[0] != "parcel-legacy" {
		t.Fatal("legacy frozen identity changed")
	}
	raw, _ := json.Marshal(resource)
	if strings.Contains(string(raw), "PRIVATE_LEGACY_CONTENT") || strings.Contains(string(raw), "business-legacy") {
		t.Fatal("legacy raw payload/tenant metadata leaked")
	}
	if _, err := tx.Exec(ctx, `UPDATE custody_mission_state SET manifest_version=5 WHERE mission_ref='mission-legacy'`); err != nil {
		t.Fatal(err)
	}
	if _, err := source.Read(ctx, tx, "custody-missions", "mission-legacy", true); err == nil {
		t.Fatal("stale frozen manifest presented as current custody identity")
	}
}
