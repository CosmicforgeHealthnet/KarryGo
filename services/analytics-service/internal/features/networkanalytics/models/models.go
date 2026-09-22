// Package models holds the Section 13 network analytics types.
package models

import (
	"time"

	"cosmicforge/logistics/shared/go/networkanalytics"
)

// Ingest outcomes.
const (
	StatusProcessed = "PROCESSED"
	StatusDuplicate = "DUPLICATE"
	StatusStale     = "STALE"
	StatusIgnored   = "IGNORED"
)

// Event is one network event, already validated and minimized.
type Event struct {
	EventID          string
	EventName        string
	SourceService    string
	SourceEntityType string
	SourceEntityID   string
	SourceVersion    int64
	OccurredAt       time.Time
	CorrelationID    string
	Payload          map[string]any
	PayloadHash      string

	// Authority and Kind are empty when the name is not one analytics
	// projects.
	Authority string
	Kind      networkanalytics.Kind
}

// MinimumSample is the least evidence an advisory score is computed from.
// Below it the answer is neutral: five jobs say almost nothing about a
// provider, and a score built on them would steer work on noise.
const MinimumSample = 5

// Window is a closed date range for admin reads.
type Window struct {
	Start time.Time
	End   time.Time
}

// MissionSuccessDay is one authority-day of mission outcomes.
type MissionSuccessDay struct {
	Authority         string    `json:"authority"`
	Date              time.Time `json:"date"`
	Completed         int64     `json:"completed"`
	Failed            int64     `json:"failed"`
	Cancelled         int64     `json:"cancelled"`
	GuaranteeBreaches int64     `json:"guarantee_breaches"`
	SuccessRate       float64   `json:"success_rate"`
}

// RecoveryDay is one authority-day of recovery performance.
type RecoveryDay struct {
	Authority                string    `json:"authority"`
	Date                     time.Time `json:"date"`
	Started                  int64     `json:"started"`
	Replaced                 int64     `json:"replaced"`
	Resolved                 int64     `json:"resolved"`
	Failed                   int64     `json:"failed"`
	AverageResolutionSeconds float64   `json:"average_resolution_seconds"`
}

// MismatchDay is one authority-day-reason of capability mismatches.
type MismatchDay struct {
	Authority  string    `json:"authority"`
	Date       time.Time `json:"date"`
	ReasonCode string    `json:"reason_code"`
	Mismatches int64     `json:"mismatches"`
}

// DemandDay is one authority-zone-day of demand.
type DemandDay struct {
	Authority string    `json:"authority"`
	Zone      string    `json:"zone"`
	Date      time.Time `json:"date"`
	Requests  int64     `json:"requests"`
	Unmatched int64     `json:"unmatched"`
}

// ProviderTotals are a provider's counts over a window.
type ProviderTotals struct {
	Completed, Failed, Cancelled, Arrivals, Declines int64
	SafetyDeclines                                   int64
	RecoveriesAsOriginal, RecoveriesAsReplacement    int64
	DataAsOf                                         time.Time
}

// ReadinessTotals are a branch's readiness counts over a window.
type ReadinessTotals struct {
	ReadyCount, OnTimeCount, LateCount     int64
	LatenessSecondsTotal, PrepSecondsTotal int64
	PrepCount                              int64
	DataAsOf                               time.Time
}

// DeclineDay is one authority-day-reason of provider declines.
type DeclineDay struct {
	Authority  string    `json:"authority"`
	Date       time.Time `json:"date"`
	ReasonCode string    `json:"reason_code"`
	Safety     bool      `json:"safety"`
	Declines   int64     `json:"declines"`
}

// Supply is the latest supply report for a zone.
type Supply struct {
	AvailableProviders int64
	BusyProviders      int64
	ObservedAt         time.Time
}
