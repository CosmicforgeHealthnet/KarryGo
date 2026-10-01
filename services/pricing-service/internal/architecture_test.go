// Package internal hosts pricing-service's architecture guardrails.
//
// These run in ordinary CI with no database, and they exist because the two
// rules below are cheap to break by accident and expensive to discover later.
package internal

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestFeaturesDoNotImportOtherFeatureHTTPOrRepositories keeps feature boundaries
// honest from the first commit: no internal/features/X package may import
// internal/features/Y/http or internal/features/Y/repositories. Cross-feature
// needs go through a port interface wired in cmd/.
func TestFeaturesDoNotImportOtherFeatureHTTPOrRepositories(t *testing.T) {
	root := featuresRoot(t)
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
			if match[1] != ownFeature {
				t.Errorf("%s: feature %q imports internal/features/%s/%s — cross-feature http/repositories imports are forbidden; use a port interface instead",
					path, ownFeature, match[1], match[2])
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk features: %v", err)
	}
}

// TestNoFloatingPointInTheEngineOrMoney is the rule that makes a quote
// reproducible.
//
// A float64 kilometre is why the legacy formulas cannot be replayed exactly: the
// same trip can produce a different fare depending on how the distance happened
// to be represented. The engine therefore works in whole metres, whole seconds
// and whole kobo, with rates as integers over an integer unit and multipliers as
// exact rationals.
//
// canonical is exempt at one point only: coordinates arrive from callers as
// floats, and canonical.Micro converts them to fixed-point integers at the
// boundary. That exemption is asserted below rather than assumed, so it cannot
// quietly widen.
func TestNoFloatingPointInTheEngineOrMoney(t *testing.T) {
	floatRe := regexp.MustCompile(`\bfloat(32|64)\b`)

	// Directories that must contain no floating point at all.
	guarded := []string{
		filepath.Join("platform", "money"),
		"engine", // created by the policy milestone; checked as soon as it exists
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	for _, dir := range guarded {
		absolute := filepath.Join(wd, dir)
		if _, statErr := os.Stat(absolute); statErr != nil {
			continue // not built yet
		}
		err := filepath.Walk(absolute, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || !strings.HasSuffix(path, ".go") {
				return nil
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			// A test may mention a float to prove it is refused.
			if strings.HasSuffix(path, "_test.go") {
				return nil
			}
			for _, line := range strings.Split(string(content), "\n") {
				if trimmed := strings.TrimSpace(line); strings.HasPrefix(trimmed, "//") {
					continue // prose may discuss floats
				}
				if floatRe.MatchString(line) {
					t.Errorf("%s: %s mentions a floating-point type in code (%q). Money is whole kobo, distance whole metres, rates integers over an integer unit, multipliers exact rationals.",
						path, dir, strings.TrimSpace(line))
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", dir, err)
		}
	}
}

// TestCanonicalFloatExemptionIsLimitedToCoordinates pins the single permitted
// float boundary, so a future edit cannot broaden it unnoticed.
func TestCanonicalFloatExemptionIsLimitedToCoordinates(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	source, err := os.ReadFile(filepath.Join(wd, "platform", "canonical", "canonical.go"))
	if err != nil {
		t.Fatalf("read canonical.go: %v", err)
	}

	// Count the function signatures that accept a float. Exactly one is allowed:
	// Micro, which converts degrees to fixed-point microdegrees.
	signatureRe := regexp.MustCompile(`func ([A-Za-z0-9_]+)\([^)]*float64`)
	matches := signatureRe.FindAllStringSubmatch(string(source), -1)
	for _, match := range matches {
		if match[1] != "Micro" {
			t.Errorf("canonical.%s accepts a float64; only Micro may, and it must round to a fixed grid immediately", match[1])
		}
	}
	if len(matches) != 1 {
		t.Errorf("expected exactly one float-accepting function in canonical (Micro), found %d", len(matches))
	}
}

func ownFeatureName(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return ""
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) < 2 {
		return "" // a file directly in features/, such as doc.go
	}
	return parts[0]
}

func featuresRoot(t *testing.T) string {
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
