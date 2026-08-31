// Package internal hosts vendor-service's cross-cutting architecture tests —
// static guardrails that hold from the very first commit, before any feature
// has real logic, so the module-boundary rule is never violated even once.
package internal

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestFeaturesDoNotImportOtherFeatureHTTPOrRepositories enforces the module
// boundary from docs/architecture/03_VENDOR_SERVICE_MODULE_MAP.md and
// CLAUDE.md's "Do not import internal packages from another service" spirit,
// applied at the feature level within this service: no
// internal/features/X package may import internal/features/Y/http or
// internal/features/Y/repositories for a different Y. Cross-feature
// dependencies must go through a narrow port interface defined in the
// consuming feature's usecases package, with the real adapter wired in a
// composition root — mirroring fleet-partner-service's proven convention
// (see docs/architecture/00_REPOSITORY_AUDIT_BASELINE.md). Importing another
// feature's models package is fine; only http and repositories are
// restricted.
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

// TestCloudKitchenHasOneCanonicalDirectory prevents the old unseparated
// spelling from silently returning after the implementation was consolidated
// under the repository's canonical cloud_kitchen boundary.
func TestCloudKitchenHasOneCanonicalDirectory(t *testing.T) {
	root := findFeaturesRoot(t)

	canonical := filepath.Join(root, "cloud_kitchen")
	obsolete := filepath.Join(root, "cloudkitchen")

	if info, err := os.Stat(canonical); err != nil || !info.IsDir() {
		t.Fatalf("canonical Cloud Kitchen directory missing: %s", canonical)
	}
	if _, err := os.Stat(obsolete); !os.IsNotExist(err) {
		t.Fatalf("obsolete Cloud Kitchen directory must not exist: %s", obsolete)
	}

	var packages []string
	err := filepath.Walk(canonical, func(path string, info os.FileInfo, err error) error {
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
		for _, line := range strings.Split(string(content), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "package ") {
				packages = append(packages, strings.TrimSpace(strings.TrimPrefix(line, "package ")))
				break
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("inspect canonical Cloud Kitchen package: %v", err)
	}
	if len(packages) == 0 {
		t.Fatal("canonical Cloud Kitchen directory contains no Go package")
	}
	for _, pkg := range packages {
		if pkg != "cloudkitchen" {
			t.Fatalf("canonical Cloud Kitchen package=%q; want cloudkitchen", pkg)
		}
	}
}
