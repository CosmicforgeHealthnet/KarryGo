package adminresources

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cosmicforge/logistics/services/hauling-service/internal/features/restrictions"
	"cosmicforge/logistics/shared/go/admincontext"
	contextfixture "cosmicforge/logistics/shared/go/admincontext/testfixture"
	"cosmicforge/logistics/shared/go/adminowner"
	fixture "cosmicforge/logistics/shared/go/adminowner/testfixture"
	"cosmicforge/logistics/shared/go/adminpolicy"
	policyfixture "cosmicforge/logistics/shared/go/adminpolicy/testfixture"
	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

func TestHaulingRestrictionReadsRequireActualSubjectsAndCurrentCanonicalLinks(t *testing.T) {
	db := fixture.Database(t, "../../../migrations")
	ctx := context.Background()
	const provider = "11111111-1111-4111-8111-111111111111"
	const truck = "22222222-2222-4222-8222-222222222222"
	const org = "33333333-3333-4333-8333-333333333333"
	const otherOrg = "44444444-4444-4444-8444-444444444444"
	const member = "55555555-5555-4555-8555-555555555555"
	if _, err := db.Exec(ctx, `INSERT INTO truck_providers(id,first_name) VALUES($1,'PRIVATE_NAME')`, provider); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO trucks(id,provider_id,truck_type,capacity_kg,plate_number) VALUES($1,$2,'flatbed',2000,'PRIVATE_PLATE')`, truck, provider); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO hauling_organization_provider_links(organization_id,provider_id,membership_id,state) VALUES($1,$2,$3,'ACTIVE')`, org, provider, member); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO hauling_organization_truck_links(organization_id,truck_id,state) VALUES($1,$2,'ACTIVE')`, org, truck); err != nil {
		t.Fatal(err)
	}
	native := restrictions.New(db)
	source := Source()
	def := source.Specs["restrictions"].Definition
	if def.SourceContract != "hauling.admin-authority-restriction.v1" {
		t.Fatal("new restriction disclosure reused broad approval")
	}
	scope := adminpolicy.Scope{Product: "karryam", Market: "M1", Zone: "Z1", Organization: org, Tenant: org}
	issuer := contextfixture.New(t, Owner)
	store := &admincontext.Store{DB: db, Reader: source, Authority: issuer.Authority}
	refs := map[string]string{}
	commands := map[string]restrictions.Command{}
	for typ, subject := range map[string]string{"PROVIDER": provider, "ASSET": truck} {
		command := restrictions.Command{OperationReference: "TEST_ONLY_APPLY_" + typ, OrganizationID: org, SubjectType: typ, SubjectReference: subject, ReasonCode: "SAFETY_BLOCK", SourceReference: "PRIVATE_SOURCE_CASE", SourceVersion: 9}
		operation, err := native.Apply(ctx, command)
		if err != nil {
			t.Fatal(err)
		}
		refs[typ] = operation.RestrictionID
		commands[typ] = command
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		binding, err := source.Binding(ctx, tx, "restrictions", operation.RestrictionID)
		if err != nil {
			t.Fatal(err)
		}
		resource, err := source.Read(ctx, tx, "restrictions", operation.RestrictionID, true)
		tx.Rollback(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if binding.Canonical.Organization != org || binding.Canonical.Tenant != org || resource.SourceVersion == nil || *resource.SourceVersion != 1 || resource.Status == nil || *resource.Status != "APPLIED" {
			t.Fatal("native restriction facts lost", resource)
		}
		if err := adminowner.ValidateResource(resource, def, true); err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(resource)
		if strings.Contains(string(raw), "PRIVATE_") {
			t.Fatal("restriction provenance/private profile leaked")
		}
		a := issuer.Approval("TEST_ONLY_RESTRICTION_"+typ, "restrictions", operation.RestrictionID, def.Capability, binding, scope)
		old := a
		old.SourceContract = "hauling.admin-resources.v1"
		if err := store.Import(ctx, issuer.Sign(t, old), time.Now()); err == nil {
			t.Fatal("broad approval enabled restriction view")
		}
		wrong := a
		wrong.Scope.Tenant = otherOrg
		if err := store.Import(ctx, issuer.Sign(t, wrong), time.Now()); err == nil {
			t.Fatal("caller tenant overrode native planning organization")
		}
		if err := store.Import(ctx, issuer.Sign(t, a), time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	policy := policyfixture.New(t, scope, Owner, def.Capability, def.SourceContract)
	profile, _ := adminowner.ForOwner(Owner)
	key := []byte(strings.Repeat("r", 32))
	h := &adminowner.Handler{Owner: Owner, Path: profile.Path, DB: db, Source: source, Contexts: store, Policy: policy.Engine, Secret: key, Enabled: true, Definitions: profile.Definitions}
	engine := gin.New()
	h.Register(engine)
	q := adminread.Query{Scope: adminread.Scope{Market: scope.Market, Zone: scope.Zone, Organization: scope.Organization, Tenant: scope.Tenant}, Staff: "staff-1", Role: "staff", Purpose: "case-1", PolicyID: policy.Engine.ID(), PolicyVersion: policy.Engine.Version(), PolicyChecksum: policy.Engine.Checksum(), Start: time.Now().UTC().Add(-time.Hour), End: time.Now().UTC().Add(time.Second), Limit: 10}
	request := httptest.NewRequest("GET", profile.Path+"/resources/restrictions/"+refs["ASSET"]+"?"+q.Values().Encode(), nil)
	if err := serviceauth.SignRequest(request, adminread.Caller, key, nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	if response.Code != 200 || strings.Contains(response.Body.String(), "PRIVATE_") {
		t.Fatal("signed restriction read", response.Code, response.Body.String())
	}
	if dir := os.Getenv("ADMIN_OWNER_CUSTODY_CAPTURE_DIR"); dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "hauling-restriction.json"), response.Body.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
	}
	denied := func(tx pgx.Tx, ref string) {
		t.Helper()
		if _, err := source.Read(ctx, tx, "restrictions", ref, true); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatal("unmapped/foreign subject disclosed", err)
		}
		if _, err := store.Resolve(ctx, tx, "restrictions", ref, def.Capability, scope, time.Now()); !errors.Is(err, admincontext.ErrUnmapped) {
			t.Fatal("old context survived changed subject/tenant", err)
		}
	}
	for _, name := range []string{"provider link revoked", "truck link revoked", "membership replaced", "foreign organization"} {
		t.Run(name, func(t *testing.T) {
			tx, err := db.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			switch name {
			case "provider link revoked":
				_, err = tx.Exec(ctx, `UPDATE hauling_organization_provider_links SET state='REVOKED',version=version+1 WHERE provider_id=$1`, provider)
			case "truck link revoked":
				_, err = tx.Exec(ctx, `UPDATE hauling_organization_truck_links SET state='REVOKED',version=version+1 WHERE truck_id=$1`, truck)
			case "membership replaced":
				_, err = tx.Exec(ctx, `UPDATE hauling_organization_provider_links SET membership_id=gen_random_uuid(),version=version+1 WHERE provider_id=$1`, provider)
			case "foreign organization":
				_, err = tx.Exec(ctx, `UPDATE hauling_authority_restrictions SET organization_id=$2 WHERE id=$1`, refs["ASSET"], otherOrg)
			}
			if err != nil {
				t.Fatal(err)
			}
			if name == "membership replaced" {
				if _, err := store.Resolve(ctx, tx, "restrictions", refs["ASSET"], def.Capability, scope, time.Now()); !errors.Is(err, admincontext.ErrUnmapped) {
					t.Fatal("old allocation followed replaced membership", err)
				}
			} else {
				denied(tx, refs["ASSET"])
			}
			if name == "provider link revoked" {
				denied(tx, refs["PROVIDER"])
			}
		})
	}
	for _, command := range []restrictions.Command{
		{OperationReference: "TEST_ONLY_UNMAPPED", OrganizationID: org, SubjectType: "PROVIDER", SubjectReference: "historical-unmapped-provider", ReasonCode: "SAFETY_BLOCK", SourceReference: "PRIVATE_SOURCE", SourceVersion: 1},
		{OperationReference: "TEST_ONLY_EXTERNAL", OrganizationID: org, SubjectType: "SHIPMENT", SubjectReference: truck, ReasonCode: "SAFETY_BLOCK", SourceReference: "PRIVATE_SOURCE", SourceVersion: 1},
	} {
		op, err := native.Apply(ctx, command)
		if err != nil {
			t.Fatal(err)
		}
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		denied(tx, op.RestrictionID)
		tx.Rollback(ctx)
	}
	// An observed native clear is not a permission/membership mutation. It keeps
	// the same authorization binding and advances the actual restriction revision.
	clear := commands["ASSET"]
	clear.OperationReference = "TEST_ONLY_CLEAR"
	clear.SourceVersion = 10
	if _, err := native.Clear(ctx, clear); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	r, err := source.Read(ctx, tx, "restrictions", refs["ASSET"], true)
	if err != nil || r.Status == nil || *r.Status != "CLEARED" || r.SourceVersion == nil || *r.SourceVersion != 2 {
		t.Fatal("native clear not observed", err)
	}
	if _, err := store.Resolve(ctx, tx, "restrictions", refs["ASSET"], def.Capability, scope, time.Now()); err != nil {
		t.Fatal("native state change invalidated unchanged authorization graph", err)
	}
}
