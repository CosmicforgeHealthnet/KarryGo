// Package models defines Section 15 manifests and closure snapshots.
package models

import "time"

const (
	StatusDraft      = "DRAFT"
	StatusOpen       = "OPEN"
	StatusClosed     = "CLOSED"
	StatusDispatched = "DISPATCHED"
	StatusCompleted  = "COMPLETED"
	StatusCancelled  = "CANCELLED"
)

var transitions = map[string]map[string]bool{StatusDraft: {StatusOpen: true, StatusCancelled: true}, StatusOpen: {StatusClosed: true, StatusCancelled: true}, StatusClosed: {StatusDispatched: true, StatusCancelled: true}, StatusDispatched: {StatusCompleted: true}, StatusCompleted: {}, StatusCancelled: {}}

func CanTransition(from, to string) bool { return transitions[from][to] }
func Editable(status string) bool        { return status == StatusDraft || status == StatusOpen }

type Manifest struct {
	ID, OrganizationID, BranchID, ManifestReference, RouteReference, FutureRoutePlanID, VehicleReference, ProviderMembershipID, SealNumber, Status string
	Version                                                                                                                                        int64
	CreatedBy, UpdatedBy                                                                                                                           string
	CreatedAt, UpdatedAt                                                                                                                           time.Time
	ClosedAt, DispatchedAt, CompletedAt, CancelledAt                                                                                               *time.Time
}
type Item struct {
	ID, OrganizationID, ManifestID, ItemType, ItemID, ShipmentID, AddedBy string
	CreatedAt                                                             time.Time
}
type Snapshot struct {
	ID, OrganizationID, ManifestID string
	ManifestVersion                int64
	Snapshot                       map[string]any
	RequestFingerprint, CreatedBy  string
	CreatedAt                      time.Time
}
