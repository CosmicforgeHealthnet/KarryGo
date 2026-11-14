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
	ID                   string     `json:"id"`
	OrganizationID       string     `json:"organizationId"`
	BranchID             string     `json:"branchId"`
	ManifestReference    string     `json:"manifestReference"`
	RouteReference       string     `json:"routeReference"`
	FutureRoutePlanID    string     `json:"futureRoutePlanId"`
	VehicleReference     string     `json:"vehicleReference"`
	ProviderMembershipID string     `json:"providerMembershipId"`
	SealNumber           string     `json:"sealNumber"`
	Status               string     `json:"status"`
	Version              int64      `json:"version"`
	CreatedBy            string     `json:"createdBy"`
	UpdatedBy            string     `json:"updatedBy"`
	CreatedAt            time.Time  `json:"createdAt"`
	UpdatedAt            time.Time  `json:"updatedAt"`
	ClosedAt             *time.Time `json:"closedAt"`
	DispatchedAt         *time.Time `json:"dispatchedAt"`
	CompletedAt          *time.Time `json:"completedAt"`
	CancelledAt          *time.Time `json:"cancelledAt"`
}
type Item struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organizationId"`
	ManifestID     string    `json:"manifestId"`
	ItemType       string    `json:"itemType"`
	ItemID         string    `json:"itemId"`
	ShipmentID     string    `json:"shipmentId"`
	AddedBy        string    `json:"addedBy"`
	CreatedAt      time.Time `json:"createdAt"`
}
type Snapshot struct {
	ID                 string         `json:"id"`
	OrganizationID     string         `json:"organizationId"`
	ManifestID         string         `json:"manifestId"`
	ManifestVersion    int64          `json:"manifestVersion"`
	Snapshot           map[string]any `json:"snapshot"`
	RequestFingerprint string         `json:"requestFingerprint"`
	CreatedBy          string         `json:"createdBy"`
	CreatedAt          time.Time      `json:"createdAt"`
}
