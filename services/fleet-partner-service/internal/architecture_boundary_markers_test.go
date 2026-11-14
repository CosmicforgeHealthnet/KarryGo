package internal_test

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// boundaryMarkers are the spec-named Section 5 packages whose production
// implementation lives in another, already layered feature (FLEET-5c). Each
// is a documented boundary, not a second implementation: moving the code
// would only rename stable packages that registries and other services
// reference. The guard keeps the mapping true.
var boundaryMarkers = map[string][]string{
	"assignments": {"marketplace", "privateoperations"},
	"contracts":   {"crm"},
	"customers":   {"crm"},
	"wallets":     {"companywallet"},
	"reports":     {"exports", "dashboard"},
}

var featureRef = regexp.MustCompile(`features/([a-z0-9_]+)`)

// TestBoundaryMarkersPointAtRealLayeredFeatures fails when a marker gains
// code (it would become a second implementation), stops naming its
// implementing features, or names a feature that does not exist or is not
// layered into models/repositories/usecases/http.
func TestBoundaryMarkersPointAtRealLayeredFeatures(t *testing.T) {
	featuresDir := filepath.Join(serviceRoot, "internal", "features")
	for marker, implementers := range boundaryMarkers {
		dir := filepath.Join(featuresDir, marker)
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("marker %s: %v", marker, err)
		}
		if len(entries) != 1 || entries[0].Name() != "doc.go" {
			t.Errorf("marker %s must contain only doc.go (found %d entries); code belongs in its implementing feature", marker, len(entries))
		}
		doc, err := os.ReadFile(filepath.Join(dir, "doc.go"))
		if err != nil {
			t.Fatalf("marker %s doc: %v", marker, err)
		}
		named := map[string]bool{}
		for _, m := range featureRef.FindAllStringSubmatch(string(doc), -1) {
			named[m[1]] = true
		}
		for _, impl := range implementers {
			if !named[impl] {
				t.Errorf("marker %s doc.go does not name its implementation features/%s", marker, impl)
			}
			for _, layer := range []string{"http"} {
				if _, err := os.Stat(filepath.Join(featuresDir, impl, layer)); err != nil {
					t.Errorf("marker %s names features/%s, which has no %s layer", marker, impl, layer)
				}
			}
			if _, err := os.Stat(filepath.Join(featuresDir, impl, "usecases")); err != nil {
				t.Errorf("marker %s names features/%s, which has no usecases layer", marker, impl)
			}
		}
	}
}
