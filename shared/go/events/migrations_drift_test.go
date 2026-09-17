package events

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every authority that publishes movement events ships the same emission and
// outbox schema, so the shared emitter and publisher behave identically in
// each service.
func TestServiceMigrationsEmbedCanonicalMovementOutboxSchema(t *testing.T) {
	root := filepath.Join("..", "..", "..", "services")
	if _, err := os.Stat(root); err != nil {
		t.Skip("NOT EXECUTED: services directory is not available beside shared/go")
	}
	for _, direction := range []string{"up", "down"} {
		canonical, err := os.ReadFile(filepath.Join("migrations", "customer_movement_outbox."+direction+".sql"))
		if err != nil {
			t.Fatal(err)
		}
		for _, migration := range []string{
			"driver-hauling-service/migrations/019_customer_movement_outbox",
			"driver-dispatch-delivery-service/migrations/000058_customer_movement_outbox",
		} {
			raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(migration+"."+direction+".sql")))
			if err != nil {
				t.Fatal(err)
			}
			normalize := func(value string) string { return strings.ReplaceAll(value, "\r\n", "\n") }
			if !strings.Contains(normalize(string(raw)), normalize(string(canonical))) {
				t.Errorf("%s.%s.sql drifted from the canonical movement outbox schema", migration, direction)
			}
		}
	}
}
