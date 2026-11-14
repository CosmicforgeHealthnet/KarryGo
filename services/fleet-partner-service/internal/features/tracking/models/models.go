// Package models holds the Section 16 tracking-projection domain: the
// projection of an authoritative Dispatch/Hauling trip position, the inbound
// event and the freshness rule. Fleet never becomes a GPS authority.
package models

import (
	"strings"
	"time"
)

// Freshness states of a tracking projection.
const (
	Fresh   = "FRESH"
	Aging   = "AGING"
	Stale   = "STALE"
	Unknown = "UNKNOWN"
	Gapped  = "GAPPED"
)

// JobTrackingReconcile asks the worker to reconcile a detected gap.
const JobTrackingReconcile = "section16.tracking.reconcile"

type TrackingProjection struct {
	OrganizationID          string     `json:"organizationId"`
	SourceService           string     `json:"sourceService"`
	CanonicalTripReference  string     `json:"canonicalTripReference"`
	AssignmentID            string     `json:"assignmentId,omitempty"`
	RoutePlanID             string     `json:"routePlanId,omitempty"`
	ProviderID              string     `json:"providerId,omitempty"`
	VehicleOrTruckReference string     `json:"vehicleOrTruckReference,omitempty"`
	Latitude                *float64   `json:"latitude,omitempty"`
	Longitude               *float64   `json:"longitude,omitempty"`
	Heading                 *float64   `json:"heading,omitempty"`
	Speed                   *float64   `json:"speed,omitempty"`
	Accuracy                *float64   `json:"accuracy,omitempty"`
	OperationalStatus       string     `json:"operationalStatus"`
	SourceTimestamp         time.Time  `json:"sourceTimestamp"`
	ReceivedAt              time.Time  `json:"receivedAt"`
	SourceVersion           int64      `json:"sourceVersion"`
	FreshnessStatus         string     `json:"freshnessStatus"`
	GapStatus               string     `json:"gapStatus"`
	LastReconciledAt        *time.Time `json:"lastReconciledAt,omitempty"`
}

// Freshness classifies how live a position fix is at now.
func Freshness(source, now time.Time, freshAfter, staleAfter time.Duration, gapped bool) string {
	if gapped {
		return Gapped
	}
	if source.IsZero() {
		return Unknown
	}
	age := now.Sub(source)
	if age <= freshAfter {
		return Fresh
	}
	if age <= staleAfter {
		return Aging
	}
	return Stale
}

type TrackingEvent struct {
	EventID, OrganizationID, SourceService, CanonicalTripReference, AssignmentID, RoutePlanID, ProviderID, VehicleOrTruckReference, OperationalStatus string
	Latitude, Longitude, Heading, Speed, Accuracy                                                                                                     *float64
	SourceTimestamp                                                                                                                                   time.Time
	SourceVersion, ReconcilesThroughVersion                                                                                                           int64
}

// StaleProjection identifies a projection the maintenance sweep marked STALE.
type StaleProjection struct {
	OrganizationID, SourceService, CanonicalTripReference, RoutePlanID, AssignmentID string
}

// ProjectionKey is the stable identity of a projection, used as the cursor
// tie-breaker: a projection has no surrogate id.
func ProjectionKey(sourceService, canonicalTripReference string) string {
	return sourceService + "|" + canonicalTripReference
}

// SplitProjectionKey reverses ProjectionKey. Source service names never
// contain "|".
func SplitProjectionKey(key string) (sourceService, canonicalTripReference string, ok bool) {
	i := strings.Index(key, "|")
	if i < 0 {
		return "", "", false
	}
	return key[:i], key[i+1:], true
}
