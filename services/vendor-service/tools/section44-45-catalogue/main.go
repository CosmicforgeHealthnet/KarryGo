package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"cosmicforge/logistics/services/vendor-service/internal/events"
	"cosmicforge/logistics/services/vendor-service/internal/jobs"
)

type eventArtifact struct {
	Section                       string                  `json:"section"`
	CanonicalEvents               []events.CatalogueEvent `json:"canonicalEvents"`
	ImplementedEmitters           map[string][]string     `json:"implementedEmitters"`
	Producers                     map[string]string       `json:"producers"`
	Versions                      map[string]int          `json:"versions"`
	ActiveConsumers               map[string][]string     `json:"activeConsumers"`
	DependencyGatedConsumers      map[string][]string     `json:"dependencyGatedConsumers"`
	FutureDisabledEvents          []string                `json:"futureDisabledEvents"`
	MissingDocumentedEvents       []string                `json:"missingDocumentedEvents"`
	UndocumentedImplementedEvents []string                `json:"undocumentedImplementedEvents"`
	ProducerMismatches            []string                `json:"producerMismatches"`
	DuplicateCanonicalEvents      []string                `json:"duplicateCanonicalEvents"`
}

type workerArtifact struct {
	Section      string                 `json:"section"`
	WorkerCount  int                    `json:"workerCount"`
	StatusCounts map[string]int         `json:"statusCounts"`
	Workers      []jobs.CatalogueWorker `json:"workers"`
}

func main() {
	write := flag.Bool("write", false, "write deterministic catalogue artifacts")
	flag.Parse()

	eventCatalogue := events.Section44Catalogue()
	if err := events.ValidateSection44Catalogue(eventCatalogue); err != nil {
		fatal(err)
	}
	workerCatalogue := jobs.Section45Catalogue()
	if err := jobs.ValidateSection45Catalogue(workerCatalogue); err != nil {
		fatal(err)
	}

	implemented := map[string][]string{}
	producers := map[string]string{}
	versions := map[string]int{}
	activeConsumers := map[string][]string{}
	gatedConsumers := map[string][]string{}
	var future []string
	for _, event := range eventCatalogue {
		producers[event.Name] = event.Producer
		versions[event.Name] = event.Version
		if len(event.Aliases) > 0 {
			implemented[event.Name] = event.Aliases
		}
		if len(event.ActiveConsumers) > 0 {
			activeConsumers[event.Name] = event.ActiveConsumers
		}
		if len(event.DependencyGatedConsumers) > 0 {
			gatedConsumers[event.Name] = event.DependencyGatedConsumers
		}
		if event.Status == events.CatalogueFutureDisabled {
			future = append(future, event.Name)
		}
	}
	slices.Sort(future)

	eventBytes := mustJSON(eventArtifact{
		Section:                       "44 — EVENT CATALOGUE",
		CanonicalEvents:               eventCatalogue,
		ImplementedEmitters:           implemented,
		Producers:                     producers,
		Versions:                      versions,
		ActiveConsumers:               activeConsumers,
		DependencyGatedConsumers:      gatedConsumers,
		FutureDisabledEvents:          future,
		MissingDocumentedEvents:       []string{},
		UndocumentedImplementedEvents: []string{},
		ProducerMismatches:            []string{},
		DuplicateCanonicalEvents:      []string{},
	})

	statusCounts := map[string]int{}
	for _, worker := range workerCatalogue {
		statusCounts[worker.Status]++
	}
	workerBytes := mustJSON(workerArtifact{
		Section:      "45 — DURABLE WORKERS AND SCHEDULED JOBS",
		WorkerCount:  len(workerCatalogue),
		StatusCounts: statusCounts,
		Workers:      workerCatalogue,
	})

	artifacts := map[string][]byte{
		filepath.Join("docs", "SECTION44_EVENT_PARITY.json"):     eventBytes,
		filepath.Join("docs", "SECTION45_WORKER_CATALOGUE.json"): workerBytes,
	}
	for path, content := range artifacts {
		if *write {
			if err := os.WriteFile(path, content, 0o644); err != nil {
				fatal(err)
			}
			continue
		}
		current, err := os.ReadFile(path)
		if err != nil {
			fatal(fmt.Errorf("%s: %w", path, err))
		}
		if !bytes.Equal(current, content) {
			fatal(fmt.Errorf("%s is stale; run with -write", path))
		}
	}

	fmt.Printf("SECTION44_45_CATALOGUE_PASS events=%d workers=%d futureEvents=%d\n",
		len(eventCatalogue), len(workerCatalogue), len(future))
}

func mustJSON(value any) []byte {
	content, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		fatal(err)
	}
	return append(content, '\n')
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
