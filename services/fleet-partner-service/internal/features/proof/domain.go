package proof

import (
	"encoding/json"
	"time"
)

type Flags struct{ Policies, Records, Workers, KillSwitch bool }
type Policy struct {
	ID                 string          `json:"id"`
	OrganizationID     string          `json:"organizationId"`
	PolicyScope        string          `json:"policyScope"`
	ScopeReference     string          `json:"scopeReference"`
	ServiceType        string          `json:"serviceType"`
	StopType           string          `json:"stopType"`
	RequiredProofTypes json.RawMessage `json:"requiredProofTypes"`
	MinimumCount       int             `json:"minimumCount"`
	LocationRequired   bool            `json:"locationRequired"`
	TimeWindowRules    json.RawMessage `json:"timeWindowRules"`
	FailurePolicy      string          `json:"failurePolicy"`
	Status             string          `json:"status"`
	Version            int64           `json:"version"`
	CreatedBy          string          `json:"createdBy"`
	CreatedAt          time.Time       `json:"createdAt"`
	UpdatedAt          time.Time       `json:"updatedAt"`
}
type PolicySnapshot struct {
	ID             string          `json:"id"`
	OrganizationID string          `json:"organizationId"`
	PolicyID       string          `json:"policyId"`
	PolicyVersion  int64           `json:"policyVersion"`
	Snapshot       json.RawMessage `json:"snapshot"`
	PublishedBy    string          `json:"publishedBy"`
	PublishedAt    time.Time       `json:"publishedAt"`
}
type Record struct {
	ID, OrganizationID, PolicySnapshotReference, ProofType, OrderID, ShipmentID, PackageID, RouteStopID, AssignmentID, CanonicalTripReference, ActorType, ActorReference, MediaFileID, ValueHash string
	SafeValueMetadata                                                                                                                                                                            json.RawMessage
	Latitude, Longitude, Accuracy                                                                                                                                                                *float64
	SourceTimestamp, ReceivedAt                                                                                                                                                                  time.Time
	Source, VerificationStatus                                                                                                                                                                   string
	SourceVersion                                                                                                                                                                                int64
	IdempotencyKey                                                                                                                                                                               string
	Version                                                                                                                                                                                      int64
	CreatedAt                                                                                                                                                                                    time.Time
}
type RecordInput struct {
	PolicySnapshotReference string          `json:"policySnapshotReference"`
	ProofType               string          `json:"proofType"`
	OrderID                 string          `json:"orderId"`
	ShipmentID              string          `json:"shipmentId"`
	PackageID               string          `json:"packageId"`
	RouteStopID             string          `json:"routeStopId"`
	AssignmentID            string          `json:"assignmentId"`
	CanonicalTripReference  string          `json:"canonicalTripReference"`
	ActorType               string          `json:"actorType"`
	ActorReference          string          `json:"actorReference"`
	MediaFileID             string          `json:"mediaFileId"`
	Value                   string          `json:"value"`
	SafeValueMetadata       json.RawMessage `json:"safeValueMetadata"`
	Latitude                *float64        `json:"latitude"`
	Longitude               *float64        `json:"longitude"`
	Accuracy                *float64        `json:"accuracy"`
	SourceTimestamp         time.Time       `json:"sourceTimestamp"`
	Source                  string          `json:"source"`
	SourceVersion           int64           `json:"sourceVersion"`
	IdempotencyKey          string          `json:"idempotencyKey"`
}
type Verification struct {
	ID                 string          `json:"id"`
	OrganizationID     string          `json:"organizationId"`
	ProofRecordID      string          `json:"proofRecordId"`
	Status             string          `json:"status"`
	AuthorityType      string          `json:"authorityType"`
	AuthorityReference string          `json:"authorityReference"`
	SourceVersion      int64           `json:"sourceVersion"`
	ReasonCodes        json.RawMessage `json:"reasonCodes"`
	VerifiedAt         time.Time       `json:"verifiedAt"`
}
