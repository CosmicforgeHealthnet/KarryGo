// Package observability instruments Dispatch using the shared repository
// registry. Label values are closed vocabularies, never entity identifiers.
package observability

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	platform "cosmicforge/logistics/shared/go/observability"
)

var Metrics = platform.NewMetrics()

func init() {
	for _, worker := range []string{"assignment", "handover", "recovery", "route", "outbox", "relay", "wallet_hold", "wallet_settlement", "collector"} {
		Metrics.SetGauge("dispatch_worker_last_run_timestamp", map[string]string{"worker": worker}, 0)
		for _, result := range []string{"success", "timeout_unknown", "cancelled", "rejected_or_failed"} {
			Metrics.IncCounter("dispatch_worker_runs_total", map[string]string{"worker": worker, "outcome": result}, 0)
		}
	}
}

func OutboxOutcome(target, outcome string) {
	Metrics.IncCounter("dispatch_event_delivery_total", map[string]string{"target": Bounded(target, "vendor-service", "analytics-service", "notification-service", "fleet-partner-service"), "outcome": Bounded(outcome, "ACCEPTED", "DUPLICATE", "REJECTED", "UNKNOWN", "RETRYABLE")}, 1)
}

func Bounded(value string, allowed ...string) string {
	for _, candidate := range allowed {
		if value == candidate {
			return value
		}
	}
	return "other"
}

func Outcome(err error) string {
	if err == nil {
		return "success"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout_unknown"
	}
	var timeout net.Error
	if errors.As(err, &timeout) && timeout.Timeout() {
		return "timeout_unknown"
	}
	if errors.Is(err, context.Canceled) {
		return "cancelled"
	}
	return "rejected_or_failed"
}

func Handover(operation, stage, method string, err error) {
	labels := map[string]string{"stage": Bounded(stage, "PICKUP", "DELIVERY", "PROVIDER_TRANSFER", "RETURN"), "outcome": Outcome(err)}
	if operation == "verification" {
		labels["method"] = Bounded(method, "OTP", "QR")
	} else {
		labels["reason"] = "requested"
	}
	Metrics.IncCounter("dispatch_handover_"+Bounded(operation, "verification", "resend", "issue")+"_total", labels, 1)
}

func Worker(name string, started time.Time, err error) {
	name = Bounded(name, "assignment", "handover", "recovery", "route", "outbox", "relay", "wallet_hold", "wallet_settlement", "collector")
	labels := map[string]string{"worker": name, "outcome": Outcome(err)}
	Metrics.IncCounter("dispatch_worker_runs_total", labels, 1)
	Metrics.SetGauge("dispatch_worker_last_run_timestamp", map[string]string{"worker": name}, float64(time.Now().Unix()))
	Metrics.ObserveDuration("dispatch_worker_processing", labels, time.Since(started))
}

func WorkerRunning(name string) func() {
	name = Bounded(name, "assignment", "handover", "recovery", "route", "outbox", "relay", "wallet_hold", "wallet_settlement", "collector")
	labels := map[string]string{"worker": name}
	Metrics.AddGauge("dispatch_workers_running", labels, 1)
	var once sync.Once
	return func() { once.Do(func() { Metrics.AddGauge("dispatch_workers_running", labels, -1) }) }
}

// DependencyTransport wraps the existing HTTP transport without changing
// authentication, retries, deadlines or response handling.
type DependencyTransport struct {
	Base  http.RoundTripper
	Hosts map[string]string
}

func (t DependencyTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	name := Bounded(t.Hosts[r.URL.Host], "wallet", "notification", "media", "vendor", "fleet", "analytics", "routing")
	start := time.Now()
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	response, err := base.RoundTrip(r)
	result := Outcome(err)
	if err == nil && response != nil && response.StatusCode >= 400 {
		result = "remote_error"
	}
	labels := map[string]string{"dependency": name, "outcome": result}
	Metrics.IncCounter("dispatch_dependency_requests_total", labels, 1)
	Metrics.ObserveDuration("dispatch_dependency_latency", labels, time.Since(start))
	return response, err
}
func InstallDependencyTransport(endpoints map[string]string) {
	hosts := map[string]string{}
	for name, endpoint := range endpoints {
		if u, err := url.Parse(strings.TrimSpace(endpoint)); err == nil && u.Host != "" {
			hosts[u.Host] = name
		}
	}
	http.DefaultTransport = DependencyTransport{Base: http.DefaultTransport, Hosts: hosts}
}
