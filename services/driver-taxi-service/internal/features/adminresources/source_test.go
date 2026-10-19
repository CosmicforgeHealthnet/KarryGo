package adminresources

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

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
	if _, err := db.Exec(ctx, `INSERT INTO taxi_providers(id,first_name) VALUES('11111111-1111-4111-8111-111111111111','PRIVATE_NAME'); INSERT INTO provider_operational_state(provider_id,state) VALUES('11111111-1111-4111-8111-111111111111','OFFLINE')`); err != nil {
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
