// Package internal hosts referral-service's architecture guardrails.
package internal

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestFeaturesDoNotImportOtherFeatureHTTPOrRepositories keeps feature
// boundaries honest from the first commit: no internal/features/X package may
// import internal/features/Y/http or internal/features/Y/repositories. Cross-
// feature needs go through a port interface wired in cmd/.
func TestFeaturesDoNotImportOtherFeatureHTTPOrRepositories(t *testing.T) {
	root := findFeaturesRoot(t)

	importRe := regexp.MustCompile(`internal/features/([A-Za-z0-9_]+)/(http|repositories)"`)

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}

		ownFeature := ownFeatureName(root, path)
		if ownFeature == "" {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		for _, match := range importRe.FindAllStringSubmatch(string(content), -1) {
			importedFeature := match[1]
			if importedFeature != ownFeature {
				t.Errorf("%s: feature %q imports internal/features/%s/%s — cross-feature http/repositories imports are forbidden; use a port interface instead",
					path, ownFeature, importedFeature, match[2])
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk features: %v", err)
	}
}

func ownFeatureName(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return ""
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) < 2 {
		return ""
	}
	return parts[0]
}

func findFeaturesRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	root := filepath.Join(wd, "features")
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("internal/features directory not found at %s: %v", root, err)
	}
	return root
}
