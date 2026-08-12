package migrations_test

import (
	"os"
	"strings"
	"testing"
)

func TestSection25And26CompletionMigrations(t *testing.T) {
	cases := map[string][]string{"000061_section25_search_and_exports_completion.up.sql": {"COMPLIANCE_INCIDENTS", "SUPPORT_CASE_PROJECTIONS", "fleet_partner_search_index_jobs", "fleet_partner_search_index_attempts", "FOR EACH ROW EXECUTE FUNCTION fleet_enqueue_search_index", "visibility"}, "000062_section26_synchronization.up.sql": {"fleet_partner_synchronization_projections", "source_service", "source_version", "gap_conflict_state", "last_reconciled_at", "next_reconciliation_at", "fleet_partner_synchronization_cases", "fleet_partner_synchronization_attempts", "fleet_partner_synchronization_dead_letters", "synchronization.read", "synchronization.reconcile", "synchronization.replay"}}
	for file, required := range cases {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		text := string(raw)
		for _, token := range required {
			if !strings.Contains(text, token) {
				t.Errorf("%s missing %s", file, token)
			}
		}
	}
}
