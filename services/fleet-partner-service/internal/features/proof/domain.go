package proof

import (
	"encoding/json"
	"time"
)

type Flags struct{ Policies, Records, Workers, KillSwitch bool }
type Policy struct {
	ID, OrganizationID, PolicyScope, ScopeReference, ServiceType, StopType string
	RequiredProofTypes                                                     json.RawMessage
	MinimumCount                                                           int
	LocationRequired                                                       bool
	TimeWindowRules                                                        json.RawMessage
	FailurePolicy, Status                                                  string
	Version                                                                int64
	CreatedBy                                                              string
	CreatedAt, UpdatedAt                                                   time.Time
}
type PolicySnapshot struct {
	ID, OrganizationID, PolicyID string
	PolicyVersion                int64
	Snapshot                     json.RawMessage
	PublishedBy                  string
	PublishedAt                  time.Time
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
	PolicySnapshotReference, ProofType, OrderID, ShipmentID, PackageID, RouteStopID, AssignmentID, CanonicalTripReference, ActorType, ActorReference, MediaFileID, Value string
	SafeValueMetadata                                                                                                                                                    json.RawMessage
	Latitude, Longitude, Accuracy                                                                                                                                        *float64
	SourceTimestamp                                                                                                                                                      time.Time
	Source                                                                                                                                                               string
	SourceVersion                                                                                                                                                        int64
	IdempotencyKey                                                                                                                                                       string
}
type Verification struct {
	ID, OrganizationID, ProofRecordID, Status, AuthorityType, AuthorityReference string
	SourceVersion                                                                int64
	ReasonCodes                                                                  json.RawMessage
	VerifiedAt                                                                   time.Time
}
