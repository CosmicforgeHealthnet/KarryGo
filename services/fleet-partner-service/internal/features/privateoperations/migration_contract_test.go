package privateoperations

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPrivateMigrationDoesNotWidenMarketplaceSourceService(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(file), "..", "..", "..")
	privateMigration, err := os.ReadFile(filepath.Join(root, "migrations", "000032_private_assignments_execution.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	lower := strings.ToLower(string(privateMigration))
	if strings.Contains(lower, "alter table fleet_partner_marketplace") || strings.Contains(lower, "organization_private_order')") {
		t.Fatal("private migration widens or alters marketplace source_service")
	}
	marketplaceMigration, err := os.ReadFile(filepath.Join(root, "migrations", "000009_marketplace_and_assignments.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(marketplaceMigration), "source_service IN ('DISPATCH', 'TAXI', 'HAULING')") {
		t.Fatal("marketplace source_service constraint changed")
	}
}
