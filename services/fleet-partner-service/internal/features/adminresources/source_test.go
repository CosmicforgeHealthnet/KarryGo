package adminresources

import (
	"context"
	"errors"
	"testing"

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
}
