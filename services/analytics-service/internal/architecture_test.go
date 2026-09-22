package internal

// Section 13 guards: analytics is a sink. It reads nothing from another
// service, writes nothing but its own read models, and decides nothing.

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func sources(t *testing.T) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		content, err := os.ReadFile(path)
		files[path] = string(content)
		return err
	})
	if err != nil || len(files) < 5 {
		t.Fatalf("walk: %d files, %v", len(files), err)
	}
	return files
}

// No client, no outbound call: analytics cannot assign a provider, change a
// verification or move a booking because it cannot reach anything that does.
func TestAnalyticsMakesNoOutboundCalls(t *testing.T) {
	banned := []string{"shared/go/walletclient", "shared/go/providercompat", "shared/go/fleetproviderwork",
		"shared/go/commercemission", "shared/go/capabilityfit", "shared/go/verificationclient",
		"shared/go/mediaclient", "shared/go/notifications"}
	for path, content := range sources(t) {
		file, err := parser.ParseFile(token.NewFileSet(), path, content, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, spec := range file.Imports {
			imported, _ := strconv.Unquote(spec.Path.Value)
			for _, pkg := range banned {
				if strings.HasSuffix(imported, pkg) {
					t.Errorf("%s imports %s", path, imported)
				}
			}
		}
		if strings.Contains(content, "http.NewRequest") || strings.Contains(content, "http.Client{") {
			t.Errorf("%s makes an outbound HTTP call", path)
		}
	}
}

var ownTables = regexp.MustCompile(`^(analytics_|vendor_branch_readiness_stats|provider_quality_snapshots|zone_supply_snapshots|zone_demand_daily|mission_success_daily|recovery_performance|capability_mismatch_metrics|decline_reason_metrics|schema_migrations)`)

// Every write lands in analytics' own read models.
func TestAnalyticsWritesOnlyItsOwnTables(t *testing.T) {
	write := regexp.MustCompile(`(?i)\b(insert\s+into|update|delete\s+from)\s+([a-z_]+)`)
	for path, content := range sources(t) {
		for _, match := range write.FindAllStringSubmatch(content, -1) {
			table := strings.ToLower(match[2])
			if table == "set" || ownTables.MatchString(table) {
				continue
			}
			t.Errorf("%s writes %s, which analytics does not own", path, table)
		}
	}
}
