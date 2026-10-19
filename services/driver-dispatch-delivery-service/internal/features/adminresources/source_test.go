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
	db := fixture.Database(t, "../../../migrations", true)
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
	if _, err := db.Exec(ctx, `INSERT INTO providers(id,phone,full_name) VALUES('11111111-1111-4111-8111-111111111111','PRIVATE_PHONE','PRIVATE_NAME'); INSERT INTO provider_availability(provider_id,status) VALUES('11111111-1111-4111-8111-111111111111','offline')`); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	resource, err := source.Read(ctx, tx, "riders", "11111111-1111-4111-8111-111111111111", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := adminowner.ValidateResource(resource, source.Specs["riders"].Definition); err != nil {
		t.Fatal(err)
	}
	if resource.SourceVersion != nil {
		t.Fatal("incomplete lifecycle counter presented as resource version")
	}
	raw, _ := json.Marshal(resource)
	if strings.Contains(string(raw), "PRIVATE_") {
		t.Fatal("source private fields leaked")
	}
	binding, err := source.Binding(ctx, tx, "riders", resource.Reference)
	if err != nil {
		t.Fatal(err)
	}
	if binding.Canonical.Organization != "" || binding.Canonical.Tenant != "" || binding.Canonical.Market != "" || binding.Canonical.Zone != "" {
		t.Fatal("provider geography/tenant inferred without source authority")
	}
}

func TestCustodyAuthorizationBindsAuthorityModeAndCanonicalTenant(t *testing.T) {
	db := fixture.Database(t, "../../../migrations", true)
	ctx := context.Background()
	const ref = "mission-owner-boundary"
	if _, err := db.Exec(ctx, `INSERT INTO custody_missions(mission_ref,authority,tenant_ref,custody_mode) VALUES($1,'CUSTOMER','tenant-1','MULTI_MANIFEST')`, ref); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO custody_mission_state(mission_ref,tenant_ref,status,version) VALUES($1,'tenant-1','PICKUP_REQUIRED',1)`, ref); err != nil {
		t.Fatal(err)
	}
	source := Source()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := source.Binding(ctx, tx, "custody-missions", ref)
	resource, readErr := source.Read(ctx, tx, "custody-missions", ref, false)
	tx.Rollback(ctx)
	if err != nil || readErr != nil {
		t.Fatal(err, readErr)
	}
	if err := adminowner.ValidateResource(resource, source.Specs["custody-missions"].Definition); err != nil {
		t.Fatal(err)
	}
	issuer := contextfixture.New(t, Owner)
	store := admincontext.Store{DB: db, Reader: source, Authority: issuer.Authority}
	scope := adminpolicy.Scope{Product: "karryam", Market: "M1", Zone: "Z1", Organization: "org-1", Tenant: "tenant-1"}
	approval := issuer.Approval("custody-owner-test", "custody-missions", ref, "dispatch.custody.read", binding, scope)
	if err := store.Import(ctx, issuer.Sign(t, approval), time.Now()); err != nil {
		t.Fatal(err)
	}
	for _, change := range []string{"authority", "custody-mode", "tenant"} {
		t.Run(change, func(t *testing.T) {
			tx, err := db.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			var sql string
			switch change {
			case "authority":
				sql = `UPDATE custody_missions SET authority='ANOTHER_SOURCE' WHERE mission_ref=$1`
			case "custody-mode":
				sql = `UPDATE custody_missions SET custody_mode='LEGACY_SINGLE' WHERE mission_ref=$1`
			case "tenant":
				sql = `UPDATE custody_mission_state SET tenant_ref='tenant-2' WHERE mission_ref=$1`
			}
			if _, err := tx.Exec(ctx, sql, ref); err != nil {
				t.Fatal(err)
			}
			if _, err := store.Resolve(ctx, tx, "custody-missions", ref, "dispatch.custody.read", scope, time.Now()); !errors.Is(err, admincontext.ErrUnmapped) {
				t.Fatalf("custody authorization survived changed source binding: %v", err)
			}
		})
	}
}
