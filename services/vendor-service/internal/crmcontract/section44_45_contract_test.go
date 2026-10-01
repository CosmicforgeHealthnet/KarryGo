package crmcontract

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"cosmicforge/logistics/services/vendor-service/internal/events"
	"cosmicforge/logistics/services/vendor-service/internal/jobs"
)

type section44Artifact struct {
	CanonicalEvents               []events.CatalogueEvent `json:"canonicalEvents"`
	MissingDocumentedEvents       []string                `json:"missingDocumentedEvents"`
	UndocumentedImplementedEvents []string                `json:"undocumentedImplementedEvents"`
	ProducerMismatches            []string                `json:"producerMismatches"`
	DuplicateCanonicalEvents      []string                `json:"duplicateCanonicalEvents"`
}

type section45Artifact struct {
	WorkerCount int                    `json:"workerCount"`
	Workers     []jobs.CatalogueWorker `json:"workers"`
}

func TestSection44EventParityArtifactMatchesCode(t *testing.T) {
	raw, err := os.ReadFile("../../docs/SECTION44_EVENT_PARITY.json")
	if err != nil {
		t.Fatal(err)
	}
	var artifact section44Artifact
	if err = json.Unmarshal(raw, &artifact); err != nil {
		t.Fatal(err)
	}
	want := events.Section44Catalogue()
	if err = events.ValidateSection44Catalogue(want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(artifact.CanonicalEvents, want) {
		t.Fatal("SECTION44_EVENT_PARITY.json is stale; run section44-45-catalogue -write")
	}
	if len(artifact.MissingDocumentedEvents) != 0 || len(artifact.UndocumentedImplementedEvents) != 0 ||
		len(artifact.ProducerMismatches) != 0 || len(artifact.DuplicateCanonicalEvents) != 0 {
		t.Fatalf("event parity gaps: %+v", artifact)
	}
}

func TestSection45WorkerCatalogueMatchesCode(t *testing.T) {
	raw, err := os.ReadFile("../../docs/SECTION45_WORKER_CATALOGUE.json")
	if err != nil {
		t.Fatal(err)
	}
	var artifact section45Artifact
	if err = json.Unmarshal(raw, &artifact); err != nil {
		t.Fatal(err)
	}
	want := jobs.Section45Catalogue()
	if err = jobs.ValidateSection45Catalogue(want); err != nil {
		t.Fatal(err)
	}
	if artifact.WorkerCount != len(want) || !reflect.DeepEqual(artifact.Workers, want) {
		t.Fatal("SECTION45_WORKER_CATALOGUE.json is stale; run section44-45-catalogue -write")
	}
}
