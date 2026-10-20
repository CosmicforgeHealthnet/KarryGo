package adminresources

import (
	"context"
	"cosmicforge/logistics/shared/go/admincontext"
	contextfixture "cosmicforge/logistics/shared/go/admincontext/testfixture"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/adminowner"
	fixture "cosmicforge/logistics/shared/go/adminowner/testfixture"
	"github.com/jackc/pgx/v5"
)

func TestActualOwnerMigrationsAndSourceDictionary(t *testing.T) {
	db := fixture.Database(t, "../../../migrations")
	ctx := context.Background()
	source := Source()
	// Every primary projection must compile against the actual migrated owner
	// schema. An unknown record remains absent; it never becomes a global record.
	for kind, spec := range source.Specs {
		t.Run(kind, func(t *testing.T) {
			tx, err := db.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			if _, err := source.Read(ctx, tx, kind, "00000000-0000-4000-8000-000000000000", false); !errors.Is(err, pgx.ErrNoRows) {
				t.Fatalf("absent source record: %v", err)
			}
			if spec.DimensionQuery != "" {
				var raw []byte
				if err := tx.QueryRow(ctx, spec.DimensionQuery, "00000000-0000-4000-8000-000000000000").Scan(&raw); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
	if _, err := db.Exec(ctx, `INSERT INTO truck_providers(id,first_name) VALUES('11111111-1111-4111-8111-111111111111','PRIVATE_NAME'); INSERT INTO hauling_provider_availability(provider_id,state,changed_by) VALUES('11111111-1111-4111-8111-111111111111','UNAVAILABLE','test')`); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	resource, err := source.Read(ctx, tx, "providers", "11111111-1111-4111-8111-111111111111", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := adminowner.ValidateResource(resource, source.Specs["providers"].Definition); err != nil {
		t.Fatal(err)
	}
	if resource.SourceVersion != nil {
		t.Fatal("incomplete lifecycle counter presented as resource version")
	}
	raw, _ := json.Marshal(resource)
	if strings.Contains(string(raw), "PRIVATE_") {
		t.Fatal("source private fields leaked")
	}
	binding, err := source.Binding(ctx, tx, "providers", resource.Reference)
	if err != nil {
		t.Fatal(err)
	}
	if binding.Canonical.Organization != "" || binding.Canonical.Tenant != "" || binding.Canonical.Market != "" || binding.Canonical.Zone != "" {
		t.Fatal("provider geography/tenant inferred without source authority")
	}
}

func TestHaulingPlanningTenantCannotUseSettlementOrConflictingOwner(t *testing.T) {
	db := fixture.Database(t, "../../../migrations")
	ctx := context.Background()
	const ref = "22222222-2222-4222-8222-222222222222"
	if _, err := db.Exec(ctx, `INSERT INTO haulage_bookings(id,customer_id,pickup_address,pickup_lat,pickup_lng,dropoff_address,dropoff_lat,dropoff_lng,cargo_type,cargo_weight_kg,corporate_organization_id,settlement_organization_id) VALUES($1,gen_random_uuid(),'PRIVATE_PICKUP',0,0,'PRIVATE_DESTINATION',0,0,'general',50,'planning-org','aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa')`, ref); err != nil {
		t.Fatal(err)
	}
	source := Source()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := source.Binding(ctx, tx, "missions", ref)
	item, readErr := source.Read(ctx, tx, "missions", ref, false)
	tx.Rollback(ctx)
	if err != nil || readErr != nil {
		t.Fatal(err, readErr)
	}
	if binding.Canonical.Organization != "planning-org" || binding.Canonical.Tenant != "planning-org" {
		t.Fatal("settlement identity used as planning authorization")
	}
	if err := adminowner.ValidateResource(item, source.Specs["missions"].Definition); err != nil {
		t.Fatal(err)
	}
	issuer := contextfixture.New(t, Owner)
	store := admincontext.Store{DB: db, Authority: issuer.Authority, Reader: source}
	scope := adminpolicy.Scope{Product: "karryam", Market: "M1", Zone: "Z1", Organization: "another-org", Tenant: "another-org"}
	a := issuer.Approval("hauling-test-context", "missions", ref, "hauling.missions.read", binding, scope)
	if err := store.Import(ctx, issuer.Sign(t, a), time.Now()); !errors.Is(err, admincontext.ErrUnmapped) {
		t.Fatalf("even a valid signature cannot override canonical planning tenant: %v", err)
	}
	a.Scope.Organization = "planning-org"
	a.Scope.Tenant = "planning-org"
	if err := store.Import(ctx, issuer.Sign(t, a), time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `UPDATE haulage_bookings SET winner_organization_id='bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb' WHERE id=$1`, ref); err != nil {
		t.Fatal(err)
	}
	tx, err = db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := store.Resolve(ctx, tx, "missions", ref, "hauling.missions.read", a.Scope, time.Now()); !errors.Is(err, admincontext.ErrUnmapped) {
		t.Fatalf("conflicting canonical owners accepted: %v", err)
	}
}
