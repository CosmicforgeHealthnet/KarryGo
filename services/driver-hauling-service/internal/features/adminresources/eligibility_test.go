package adminresources

import (
	"context"
	"cosmicforge/logistics/services/hauling-service/internal/features/truckcapability"
	"cosmicforge/logistics/shared/go/admincontext"
	contextfixture "cosmicforge/logistics/shared/go/admincontext/testfixture"
	"cosmicforge/logistics/shared/go/adminowner"
	fixture "cosmicforge/logistics/shared/go/adminowner/testfixture"
	"cosmicforge/logistics/shared/go/adminpolicy"
	policyfixture "cosmicforge/logistics/shared/go/adminpolicy/testfixture"
	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/serviceauth"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestHaulingProductEligibilityRequiresCanonicalLinksAndNativeWindowSemantics(t *testing.T) {
	db := fixture.Database(t, "../../../migrations")
	ctx := context.Background()
	provider, truck, org, otherOrg := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := db.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO truck_providers(id,first_name)VALUES($1,'PRIVATE_NAME')`, provider)
	exec(`INSERT INTO trucks(id,provider_id,truck_type,capacity_kg,inspection_ref,plate_number)VALUES($1,$2,'flatbed',2000,'https://private.invalid/inspection','PRIVATE_PLATE')`, truck, provider)
	for _, organization := range []string{org, otherOrg} {
		state := "ACTIVE"
		if organization == otherOrg {
			state = "REVOKED"
		}
		exec(`INSERT INTO hauling_organization_provider_links(organization_id,provider_id,membership_id,state)VALUES($1,$2,$3,$4)`, organization, provider, uuid.NewString(), state)
		exec(`INSERT INTO hauling_organization_truck_links(organization_id,truck_id,state)VALUES($1,$2,$3)`, organization, truck, state)
	}
	nativeRepo := truckcapability.NewRepository(db)
	past := time.Now().UTC().Add(-time.Hour)
	future := time.Now().UTC().Add(time.Hour)
	native := truckcapability.ProductEligibility{TruckID: truck, ProviderID: provider, ProductCode: "HAUL_GENERAL", Status: truckcapability.EligibilityEligible, PolicyVersion: "P1", CertificationRef: "PRIVATE_CERTIFICATION", ReasonCode: "PRIVATE_REASON", EffectiveAt: &past, ExpiresAt: &future, SourceVersion: 7}
	stored, applied, err := nativeRepo.UpsertEligibility(ctx, native)
	if err != nil || !applied {
		t.Fatal(err)
	}
	ref := stored.ID + ":" + org
	otherRef := stored.ID + ":" + otherOrg
	source := Source()
	definition := source.Specs["product-eligibility"].Definition
	if definition.SourceContract != "hauling.admin-product-eligibility.v1" {
		t.Fatal("new disclosure reused old source contract")
	}
	read := func(tx pgx.Tx) adminowner.Resource {
		t.Helper()
		item, err := source.Read(ctx, tx, "product-eligibility", ref, true)
		if err != nil {
			t.Fatal(err)
		}
		if err := adminowner.ValidateResource(item, definition, true); err != nil {
			t.Fatal(err)
		}
		return item
	}
	tx, err := db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := source.Binding(ctx, tx, "product-eligibility", ref)
	if err != nil {
		t.Fatal(err)
	}
	item := read(tx)
	tx.Rollback(ctx)
	if binding.Canonical.Organization != org || binding.Canonical.Tenant != org || binding.Canonical.Market != "" || binding.Canonical.Zone != "" {
		t.Fatal("fabricated scope", binding.Canonical)
	}
	if item.SourceVersion != nil || len(item.Counters) != 1 || item.Counters[0].Value == nil || *item.Counters[0].Value != 7 {
		t.Fatal("callback counter became full source version", item)
	}
	raw, _ := json.Marshal(item)
	if strings.Contains(string(raw), "PRIVATE_") || strings.Contains(string(raw), "private.invalid") {
		t.Fatal("private inspection/certification reference leaked")
	}
	issuer := contextfixture.New(t, Owner)
	store := &admincontext.Store{DB: db, Reader: source, Authority: issuer.Authority}
	scope := adminpolicy.Scope{Product: "karryam", Market: "M1", Zone: "Z1", Organization: org, Tenant: org}
	allocation := issuer.Approval("eligibility-allocation", "product-eligibility", ref, definition.Capability, binding, scope)
	bad := allocation
	bad.Scope.Tenant = otherOrg
	if err := store.Import(ctx, issuer.Sign(t, bad), time.Now()); err == nil {
		t.Fatal("signed context overrode native linked tenant")
	}
	if err := store.Import(ctx, issuer.Sign(t, allocation), time.Now()); err != nil {
		t.Fatal(err)
	}
	// Current formal admission is measured with the owning domain's UsableAt,
	// including stale stored ELIGIBLE rows whose expiry sweeper has not run.
	for _, name := range []string{"valid", "expired", "future", "suspended", "open-bounds"} {
		t.Run(name, func(t *testing.T) {
			observed := native
			switch name {
			case "expired":
				expires := time.Now().Add(-time.Minute)
				observed.ExpiresAt = &expires
			case "future":
				effective := time.Now().Add(30 * time.Minute)
				observed.EffectiveAt = &effective
			case "suspended":
				observed.Status = truckcapability.EligibilitySuspended
			case "open-bounds":
				observed.EffectiveAt = nil
				observed.ExpiresAt = nil
			}
			tx, err := db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			if _, err := tx.Exec(ctx, `UPDATE hauling_truck_product_eligibility SET status=$2,effective_at=$3,expires_at=$4 WHERE id=$1`, stored.ID, string(observed.Status), observed.EffectiveAt, observed.ExpiresAt); err != nil {
				t.Fatal(err)
			}
			item := read(tx)
			flagFound := false
			for _, dimension := range item.Dimensions {
				if dimension.Name == "record_window_usable" {
					flagFound = true
					if dimension.Flag == nil || *dimension.Flag != observed.UsableAt(time.Now().UTC()) {
						t.Fatal("native eligibility window semantics diverged", name)
					}
				}
			}
			if !flagFound {
				t.Fatal("missing native temporal observation")
			}
		})
	}
	// A historical organization link cannot authorize another tenant's view of
	// this globally admitted asset. Native uniqueness permits one active owner.
	tx, err = db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.Read(ctx, tx, "product-eligibility", otherRef, true); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("historical organization link exposed current product admission", err)
	}
	if _, err := store.Resolve(ctx, tx, "product-eligibility", otherRef, definition.Capability, scope, time.Now()); !errors.Is(err, admincontext.ErrUnmapped) {
		t.Fatal("cross-organization reference accepted", err)
	}
	tx.Rollback(ctx)
	profile, _ := adminowner.ForOwner(Owner)
	policy := policyfixture.New(t, scope, Owner, definition.Capability, definition.SourceContract)
	key := []byte(strings.Repeat("e", 32))
	handler := &adminowner.Handler{Owner: Owner, Path: profile.Path, DB: db, Source: source, Contexts: store, Policy: policy.Engine, Secret: key, Enabled: true, Definitions: profile.Definitions}
	router := gin.New()
	handler.Register(router)
	q := adminread.Query{Scope: adminread.Scope{Market: scope.Market, Zone: scope.Zone, Organization: scope.Organization, Tenant: scope.Tenant}, Staff: "staff-1", Role: "staff", Purpose: "case-1", PolicyID: policy.Engine.ID(), PolicyVersion: policy.Engine.Version(), PolicyChecksum: policy.Engine.Checksum(), Start: time.Now().Add(-time.Hour), End: time.Now().Add(time.Second), Limit: 10}
	request := httptest.NewRequest("GET", profile.Path+"/resources/product-eligibility/"+ref+"?"+q.Values().Encode(), nil)
	if err := serviceauth.SignRequest(request, adminread.Caller, key, nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != 200 || strings.Contains(response.Body.String(), "PRIVATE_") {
		t.Fatal(response.Code, response.Body.String())
	}
	if dir := os.Getenv("ADMIN_OWNER_CUSTODY_CAPTURE_DIR"); dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "hauling-product-eligibility.json"), response.Body.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"truck-revoked", "provider-revoked", "membership-replaced", "truck-owner-mismatch"} {
		t.Run(name, func(t *testing.T) {
			tx, err := db.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			switch name {
			case "truck-revoked":
				_, err = tx.Exec(ctx, `UPDATE hauling_organization_truck_links SET state='REVOKED',version=version+1 WHERE organization_id=$1 AND truck_id=$2`, org, truck)
			case "provider-revoked":
				_, err = tx.Exec(ctx, `UPDATE hauling_organization_provider_links SET state='REVOKED',version=version+1 WHERE organization_id=$1 AND provider_id=$2`, org, provider)
			case "membership-replaced":
				_, err = tx.Exec(ctx, `UPDATE hauling_organization_provider_links SET membership_id=gen_random_uuid(),version=version+1 WHERE organization_id=$1 AND provider_id=$2`, org, provider)
			case "truck-owner-mismatch":
				otherProvider := uuid.NewString()
				if _, err := tx.Exec(ctx, `INSERT INTO truck_providers(id)VALUES($1)`, otherProvider); err != nil {
					t.Fatal(err)
				}
				_, err = tx.Exec(ctx, `UPDATE trucks SET provider_id=$2 WHERE id=$1`, truck, otherProvider)
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := store.Resolve(ctx, tx, "product-eligibility", ref, definition.Capability, scope, time.Now()); !errors.Is(err, admincontext.ErrUnmapped) {
				t.Fatal("old allocation survived changed native membership", name, err)
			}
		})
	}
	// Preserve the owner's monotonic callback fence under concurrent stale input.
	native.Status = truckcapability.EligibilitySuspended
	native.SourceVersion = 20
	if _, applied, err := nativeRepo.UpsertEligibility(ctx, native); err != nil || !applied {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	failures := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			stale := native
			stale.Status = truckcapability.EligibilityEligible
			stale.SourceVersion = 8
			row, applied, err := nativeRepo.UpsertEligibility(ctx, stale)
			if err != nil || applied || row.Status != truckcapability.EligibilitySuspended {
				failures <- errors.New("stale callback resurrected suspended product admission")
			}
		}()
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	tx, err = db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	current := read(tx)
	tx.Rollback(ctx)
	if current.Status == nil || *current.Status != "SUSPENDED" || current.SourceVersion != nil {
		t.Fatal("native suspension/readback lost", current)
	}
}
