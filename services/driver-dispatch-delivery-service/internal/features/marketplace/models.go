// Package marketplace implements the organization-managed marketplace
// integration: reservation of canonical dispatch requests for an organization,
// provider and vehicle eligibility, and creation of exactly one canonical trip
// once the assigned provider has explicitly accepted.
//
// Dispatch stays authoritative for the canonical request and trip. It never
// decides whether a provider accepts — that decision is owned by
// fleet-partner-service and is verified over HTTP before any trip is created.
package marketplace

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

// Reservation statuses.
const (
	ReservationPending       = "PENDING"
	ReservationReserved      = "RESERVED"
	ReservationReleased      = "RELEASED"
	ReservationRejected      = "REJECTED"
	ReservationCancelled     = "CANCELLED"
	ReservationExpired       = "EXPIRED"
	ReservationReconcileNeed = "RECONCILIATION_REQUIRED"
)

// Acceptance sources.
const (
	AcceptanceOrganizationDispatcher = "ORGANIZATION_DISPATCHER"
	AcceptanceProviderDirect         = "PROVIDER_DIRECT"
)

// Execution modes on the canonical trip.
const (
	ExecutionIndependentProvider = "INDEPENDENT_PROVIDER"
	ExecutionOrganizationManaged = "ORGANIZATION_MANAGED"
)

// Broadcast reservation states mirrored onto request_broadcasts.
const (
	BroadcastReservationNone     = "NONE"
	BroadcastReservationPending  = "PENDING"
	BroadcastReservationReserved = "RESERVED"
	BroadcastReservationReleased = "RELEASED"
	BroadcastReservationExecuted = "EXECUTED"
)

// Command operation types and statuses.
const (
	CommandReserveRequest     = "RESERVE_REQUEST"
	CommandReleaseReservation = "RELEASE_RESERVATION"
	CommandCreateExecution    = "CREATE_EXECUTION"

	CommandPending   = "PENDING"
	CommandConfirmed = "CONFIRMED"
	CommandRejected  = "REJECTED"
	CommandUnknown   = "UNKNOWN_OUTCOME"
	CommandFailed    = "FAILED"
)

// Release reasons accepted from fleet.
var releaseReasons = map[string]bool{
	"ORGANIZATION_CANCELLED": true,
	"ASSIGNMENT_CANCELLED":   true,
	"ASSIGNMENT_EXPIRED":     true,
	"NO_ELIGIBLE_PROVIDER":   true,
	"REQUEST_CANCELLED":      true,
	"PLATFORM_RECOVERY":      true,
}

// ValidReleaseReason reports whether the reason is in the closed catalogue.
func ValidReleaseReason(reason string) bool { return releaseReasons[reason] }

// assignableBroadcastStatuses are the canonical statuses from which an
// organization may still claim a request.
var assignableBroadcastStatuses = map[string]bool{
	"broadcasting": true,
}

// BroadcastAssignable reports whether a broadcast may still be reserved.
func BroadcastAssignable(status string) bool { return assignableBroadcastStatuses[status] }

// Reservation is an organization's claim on a canonical dispatch request.
type Reservation struct {
	ID                       string
	RequestID                string
	OrganizationID           string
	Status                   string
	AcceptanceSource         string
	AcceptingProviderID      *string
	OrganizationMembershipID *string
	SourceRequestVersion     int64
	IdempotencyKey           string
	RequestFingerprint       string
	ReservedAt               *time.Time
	ReleasedAt               *time.Time
	ReleaseReason            *string
	Version                  int64
	CreatedAt                time.Time
	UpdatedAt                time.Time
}

// Live reports whether the reservation still holds the request.
func (r Reservation) Live() bool {
	return r.Status == ReservationPending || r.Status == ReservationReserved
}

// Releasable reports whether the reservation may still be released.
//
// A reservation whose execution has already been created is deliberately not
// releasable here: releasing it would orphan a live canonical trip. That case
// must go through trip cancellation instead.
func (r Reservation) Releasable() bool { return r.Live() }

// Broadcast is the slice of the canonical request this feature needs.
type Broadcast struct {
	ID                     string
	BookingID              string
	Status                 string
	ReservationStatus      string
	ReservedOrganizationID *string
	AcceptedByProviderID   *string
	SourceVersion          int64
	ExpiresAt              time.Time
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

// Claimable reports whether the broadcast may be reserved right now.
func (b Broadcast) Claimable(now time.Time) bool {
	return BroadcastAssignable(b.Status) &&
		b.ReservationStatus == BroadcastReservationNone &&
		now.Before(b.ExpiresAt)
}

// CommandOperation is a durable record of a cross-service command, so a
// replay can be answered from storage rather than executed twice.
type CommandOperation struct {
	ID                 string
	OperationType      string
	RequestID          *string
	TripID             *string
	FleetAssignmentID  *string
	IdempotencyKey     string
	RequestFingerprint string
	Status             string
	ResponseSnapshot   []byte
	ErrorCode          *string
	AttemptCount       int
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// Execution is the organization-managed canonical trip result.
type Execution struct {
	TripID         string
	RequestID      string
	BroadcastID    string
	BookingID      string
	Status         string
	RequestVersion int64
	TripVersion    int64
	CreatedAt      time.Time
}

// Fingerprint is the deterministic hash of a command's meaningful input.
//
// It is what distinguishes a safe replay (same key, same input → return the
// stored result) from an idempotency conflict (same key, different input →
// refuse). Field order is fixed by marshalling a struct, never a map.
func Fingerprint(parts any) string {
	encoded, err := json.Marshal(parts)
	if err != nil {
		// A fingerprint that cannot be computed must never collide with a
		// real one; return a value that will not match any stored record.
		return "unfingerprintable"
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

// PayloadHash hashes an event payload for delivery-integrity checking.
func PayloadHash(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}
