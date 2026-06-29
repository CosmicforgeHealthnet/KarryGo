package verification

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVerificationFeatureFolderHasRequiredFiles(t *testing.T) {
	required := []string{
		"handler.go",
		"service.go",
		"repository.go",
		"model.go",
		"smileidentity.go",
		"subscribers.go",
	}

	for _, name := range required {
		t.Run(name, func(t *testing.T) {
			if _, err := os.Stat(filepath.Join(".", name)); err != nil {
				t.Fatalf("required verification file %s is missing: %v", name, err)
			}
		})
	}
}
