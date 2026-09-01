// Package observability retains Fleet's existing instrumentation API.
package observability

import platform "cosmicforge/logistics/shared/go/observability"

type Metrics = platform.Metrics

func NewMetrics() *Metrics { return platform.NewMetrics() }
