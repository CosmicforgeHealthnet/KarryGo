package vendors

import (
	"encoding/json"
	"sort"
	"strings"
	"sync"
)

// Section7Metrics is a dependency-free, low-cardinality counter registry.
// Labels are closed vocabularies (dimension/status/ready), never identifiers,
// document references, bank data, or representative PII.
type Section7Metrics struct {
	mu       sync.Mutex
	counters map[string]float64
}

func NewSection7Metrics() *Section7Metrics {
	return &Section7Metrics{counters: map[string]float64{}}
}

func (m *Section7Metrics) Inc(name string, labels ...string) { m.Add(name, 1, labels...) }

func (m *Section7Metrics) Add(name string, value float64, labels ...string) {
	if m == nil {
		return
	}
	parts := make([]string, 0, len(labels)/2)
	for i := 0; i+1 < len(labels); i += 2 {
		parts = append(parts, labels[i]+"="+labels[i+1])
	}
	sort.Strings(parts)
	key := name
	if len(parts) > 0 {
		key += "{" + strings.Join(parts, ",") + "}"
	}
	m.mu.Lock()
	m.counters[key] += value
	m.mu.Unlock()
}

func (m *Section7Metrics) Snapshot() map[string]float64 {
	out := map[string]float64{}
	if m == nil {
		return out
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, value := range m.counters {
		out[key] = value
	}
	return out
}

func (s *Service) Section7MetricsJSON() string {
	raw, _ := json.Marshal(s.metrics.Snapshot())
	return string(raw)
}
