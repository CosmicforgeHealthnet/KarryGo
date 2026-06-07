package availability

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAvailabilityFeatureFilesExist(t *testing.T) {
	required := []string{
		"handler.go",
		"service.go",
		"repository.go",
		"model.go",
		"subscribers.go",
	}
	for _, name := range required {
		t.Run(name, func(t *testing.T) {
			if _, err := os.Stat(filepath.Join(".", name)); err != nil {
				t.Fatalf("availability feature file %s is missing: %v", name, err)
			}
		})
	}
}

func TestAvailabilityRedisKeys(t *testing.T) {
	providerID := "11111111-1111-1111-1111-111111111111"
	if ProviderStatusKey(providerID) != "avail:status:"+providerID {
		t.Fatalf("status key mismatch: %s", ProviderStatusKey(providerID))
	}
	if ProviderLocationKey(providerID) != "avail:location:"+providerID {
		t.Fatalf("location key mismatch: %s", ProviderLocationKey(providerID))
	}
	if ProviderLocationChannel(providerID) != "avail:loc:chan:"+providerID {
		t.Fatalf("location channel mismatch: %s", ProviderLocationChannel(providerID))
	}
	if OnlineProvidersGeoKey != "avail:geo:online" {
		t.Fatalf("geo key = %s, want avail:geo:online", OnlineProvidersGeoKey)
	}
}
