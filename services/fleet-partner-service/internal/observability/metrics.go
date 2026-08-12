// Package observability provides the service's metrics registry and
// structured logging helpers.
//
// The repository has no existing metrics library; this registry is a small,
// dependency-free counter/gauge/duration store rendered in a Prometheus-style
// text exposition format at the protected /metrics endpoint. Labels must be
// low-cardinality: route templates, methods, statuses and service names —
// never raw IDs, organization names or personal information.
package observability

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// Metrics is a concurrency-safe in-memory metrics registry.
type Metrics struct {
	mu        sync.Mutex
	counters  map[string]float64
	gauges    map[string]float64
	durations map[string]*durationSummary
}

type durationSummary struct {
	count uint64
	sum   time.Duration
}

// NewMetrics creates an empty registry.
func NewMetrics() *Metrics {
	return &Metrics{
		counters:  map[string]float64{},
		gauges:    map[string]float64{},
		durations: map[string]*durationSummary{},
	}
}

// key renders `name{k1="v1",k2="v2"}` with sorted label keys.
func key(name string, labels map[string]string) string {
	if len(labels) == 0 {
		return name
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%q", k, labels[k]))
	}
	return name + "{" + strings.Join(parts, ",") + "}"
}

// IncCounter adds delta to a counter.
func (m *Metrics) IncCounter(name string, labels map[string]string, delta float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.counters[key(name, labels)] += delta
}

// SetGauge sets a gauge to value.
func (m *Metrics) SetGauge(name string, labels map[string]string, value float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.gauges[key(name, labels)] = value
}

// AddGauge adjusts a gauge by delta (for active-request style gauges).
func (m *Metrics) AddGauge(name string, labels map[string]string, delta float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.gauges[key(name, labels)] += delta
}

// ObserveDuration records one observation of a latency metric.
func (m *Metrics) ObserveDuration(name string, labels map[string]string, d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := key(name, labels)
	summary, ok := m.durations[k]
	if !ok {
		summary = &durationSummary{}
		m.durations[k] = summary
	}
	summary.count++
	summary.sum += d
}

// CounterValue returns the current value of a counter (for tests).
func (m *Metrics) CounterValue(name string, labels map[string]string) float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.counters[key(name, labels)]
}

// Render produces the text exposition of every metric, sorted for stability.
func (m *Metrics) Render() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	var b strings.Builder
	lines := make([]string, 0, len(m.counters)+len(m.gauges)+2*len(m.durations))
	for k, v := range m.counters {
		lines = append(lines, fmt.Sprintf("%s %g", k, v))
	}
	for k, v := range m.gauges {
		lines = append(lines, fmt.Sprintf("%s %g", k, v))
	}
	for k, s := range m.durations {
		name, labels := splitKey(k)
		lines = append(lines, fmt.Sprintf("%s_seconds_count%s %d", name, labels, s.count))
		lines = append(lines, fmt.Sprintf("%s_seconds_sum%s %g", name, labels, s.sum.Seconds()))
	}
	sort.Strings(lines)
	for _, line := range lines {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

func splitKey(k string) (name string, labels string) {
	if idx := strings.IndexByte(k, '{'); idx >= 0 {
		return k[:idx], k[idx:]
	}
	return k, ""
}
