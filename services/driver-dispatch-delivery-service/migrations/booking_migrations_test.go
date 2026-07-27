package migrations_test

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBookingMigrationFilesExist(t *testing.T) {
	for _, name := range []string{
		"000031_create_dispatch_bookings.up.sql", "000031_create_dispatch_bookings.down.sql",
		"000032_add_review_fields_to_ratings.up.sql", "000032_add_review_fields_to_ratings.down.sql",
	} {
		if _, err := os.Stat(filepath.Join(".", name)); err != nil {
			t.Fatalf("migration %s is missing: %v", name, err)
		}
	}
}
