package networkanalytics

// Section 25 acceptance scenario 17: an Analytics outage cannot stop an
// otherwise eligible match.
//
// Two facts make that true. The client answers every failure with a neutral
// signal (TestEveryFailureIsANeutralAnswer), and no operational service calls
// the advisory reads from a matching, assignment or commit path. This test
// pins the second: any new call site has to be added here deliberately, where
// a reviewer sees that it must tolerate the neutral answer.

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// advisoryReadCallers lists the non-analytics files allowed to call the
// advisory reads. It is empty: Analytics is observed, never consulted, on
// operational paths today.
var advisoryReadCallers = map[string]bool{}

var advisoryRead = regexp.MustCompile(`\.(ProviderQuality|SupplySnapshot|BranchReadiness|DemandForecast)\(ctx`)

func TestNoOperationalPathDependsOnAnalyticsAvailability(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	services := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", "services"))
	if _, err := os.Stat(services); err != nil {
		t.Skipf("services tree not available from %s", services)
	}
	scanned := 0
	err := filepath.WalkDir(services, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if name := entry.Name(); name == "analytics-service" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		scanned++
		content := string(raw)
		if !strings.Contains(content, "shared/go/networkanalytics") || !advisoryRead.MatchString(content) {
			return nil
		}
		rel, _ := filepath.Rel(services, path)
		if !advisoryReadCallers[filepath.ToSlash(rel)] {
			t.Errorf("%s consults Analytics; it must tolerate the neutral answer and be listed in advisoryReadCallers", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if scanned == 0 {
		t.Fatal("scanned no service sources; the boundary check proved nothing")
	}
}

// The boundary check is only as good as its pattern: prove it recognises a
// call to each advisory read.
func TestAdvisoryReadPatternRecognisesEveryRead(t *testing.T) {
	for _, call := range []string{
		"quality := client.ProviderQuality(ctx, authority, id)",
		"supply := c.SupplySnapshot(ctx, AuthorityTaxi, zone)",
		"ready := analytics.BranchReadiness(ctx, branchID)",
		"forecast := s.analytics.DemandForecast(ctx, a, z, p)",
	} {
		if !advisoryRead.MatchString(call) {
			t.Errorf("pattern misses %q", call)
		}
	}
}
