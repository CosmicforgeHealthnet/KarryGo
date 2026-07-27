// Package sourceref defines projection metadata for records owned by other
// platform services.
//
// fleet-partner-service never treats a cached copy of another service's data
// as authoritative: it stores the owning service, the immutable external ID
// and synchronization metadata, and treats local display fields as
// projections only (see docs/platform-boundaries.md).
package sourceref

import (
	"fmt"
	"time"
)

// Known authoritative service names.
const (
	ServiceCustomer     = "customer-service"
	ServiceDispatch     = "driver-dispatch-delivery-service"
	ServiceTaxi         = "driver-taxi-service"
	ServiceHauling      = "driver-hauling-service"
	ServicePayment      = "payment-wallet-service"
	ServiceNotification = "notification-service"
	ServiceMedia        = "media-file-service"
	ServiceVerification = "verification-compliance-service"
	ServiceSupport      = "support-dispute-service"
	ServiceAnalytics    = "analytics-service"
	ServiceAdmin        = "admin-backoffice-service"
)

// SyncStatus describes projection freshness.
type SyncStatus string

const (
	SyncStatusFresh    SyncStatus = "FRESH"
	SyncStatusStale    SyncStatus = "STALE"
	SyncStatusUnsynced SyncStatus = "UNSYNCED"
)

// SourceReference points at a record owned by another platform service.
type SourceReference struct {
	SourceService string     `json:"sourceService"`
	ExternalID    string     `json:"externalId"`
	SourceVersion int64      `json:"sourceVersion"`
	LastEventID   string     `json:"lastEventId,omitempty"`
	SyncedAt      time.Time  `json:"syncedAt"`
	SyncStatus    SyncStatus `json:"syncStatus,omitempty"`
}

var knownServices = map[string]bool{
	ServiceCustomer:     true,
	ServiceDispatch:     true,
	ServiceTaxi:         true,
	ServiceHauling:      true,
	ServicePayment:      true,
	ServiceNotification: true,
	ServiceMedia:        true,
	ServiceVerification: true,
	ServiceSupport:      true,
	ServiceAnalytics:    true,
	ServiceAdmin:        true,
}

// Validate checks the reference points at a known service with a non-empty
// external ID.
func (r SourceReference) Validate() error {
	if !knownServices[r.SourceService] {
		return fmt.Errorf("sourceref: unknown authoritative service %q", r.SourceService)
	}
	if r.ExternalID == "" {
		return fmt.Errorf("sourceref: external id is required")
	}
	return nil
}

// Freshness derives the projection state at now: UNSYNCED before any sync,
// STALE once the last sync is older than staleAfter, FRESH otherwise. A
// projection is never treated as authority regardless of freshness.
func (r SourceReference) Freshness(now time.Time, staleAfter time.Duration) SyncStatus {
	if r.SyncedAt.IsZero() {
		return SyncStatusUnsynced
	}
	if now.Sub(r.SyncedAt) > staleAfter {
		return SyncStatusStale
	}
	return SyncStatusFresh
}

// CompareVersion classifies an incoming source version against the stored
// projection: +1 newer (apply), 0 duplicate (ignore), -1 out-of-order
// (detect, never overwrite with older state).
func (r SourceReference) CompareVersion(incoming int64) int {
	switch {
	case incoming > r.SourceVersion:
		return 1
	case incoming == r.SourceVersion:
		return 0
	default:
		return -1
	}
}
