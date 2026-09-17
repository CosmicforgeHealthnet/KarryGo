package custodyops

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every execution service must ship the canonical multi-manifest schema
// verbatim, so Dispatch, Taxi and Hauling share one custody model.
func TestServiceMigrationsEmbedCanonicalMultiManifestSchema(t *testing.T) {
	root := filepath.Join("..", "..", "..", "services")
	if _, err := os.Stat(root); err != nil {
		t.Skip("NOT EXECUTED: services directory is not available beside shared/go")
	}
	for _, direction := range []string{"up", "down"} {
		canonical, err := os.ReadFile(filepath.Join("migrations", "multi_manifest."+direction+".sql"))
		if err != nil {
			t.Fatal(err)
		}
		for _, migration := range []string{
			"driver-taxi-service/migrations/012_custody_multi_manifest",
			"driver-hauling-service/migrations/018_custody_multi_manifest",
			"driver-dispatch-delivery-service/migrations/000057_custody_multi_manifest",
		} {
			raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(migration+"."+direction+".sql")))
			if err != nil {
				t.Fatal(err)
			}
			normalize := func(value string) string { return strings.ReplaceAll(value, "\r\n", "\n") }
			if !strings.Contains(normalize(string(raw)), normalize(string(canonical))) {
				t.Errorf("%s.%s.sql drifted from the canonical shared custody schema", migration, direction)
			}
		}
	}
}
