// Package vendorownedhandover is Dispatch's authoritative custody boundary for
// vendor-operated deliveries. Vendor Commerce remains the business workflow;
// this package alone owns challenge digests and secure custody transitions.
package vendorownedhandover

import (
	"errors"
	"strings"
	"time"
)

var (
	ErrInvalid  = errors.New("vendor-owned handover request is invalid")
	ErrNotFound = errors.New("vendor-owned handover was not found")
	ErrConflict = errors.New("vendor-owned handover conflicts with canonical state")
	ErrRejected = errors.New("vendor-owned handover credential was rejected")
	ErrDisabled = errors.New("vendor-owned handover is disabled")
)

type Stage string

const (
	Pickup   Stage = "PICKUP"
	Delivery Stage = "DELIVERY"
	Return   Stage = "RETURN"
)

type MissionBinding struct {
	MissionRef      string   `json:"mission_ref"`
	FulfilmentRef   string   `json:"fulfilment_ref"`
	BusinessID      string   `json:"business_id"`
	BranchID        string   `json:"branch_id"`
	ProviderRef     string   `json:"provider_ref"`
	ReceiverRef     string   `json:"receiver_ref"`
	ReceiverPhone   string   `json:"receiver_phone,omitempty"`
	ManifestRef     string   `json:"manifest_ref"`
	ManifestVersion int64    `json:"manifest_version"`
	ParcelRefs      []string `json:"parcel_refs"`
	OperationRef    string   `json:"-"`
	CorrelationID   string   `json:"-"`
}

func (b MissionBinding) valid() bool {
	return strings.TrimSpace(b.MissionRef) != "" && strings.TrimSpace(b.FulfilmentRef) != "" &&
		strings.TrimSpace(b.BusinessID) != "" && strings.TrimSpace(b.BranchID) != "" &&
		strings.TrimSpace(b.ProviderRef) != "" && strings.TrimSpace(b.ReceiverRef) != "" &&
		strings.TrimSpace(b.ManifestRef) != "" && b.ManifestVersion > 0 &&
		strings.TrimSpace(b.OperationRef) != ""
}

type ConfirmCommand struct {
	MissionRef      string `json:"-"`
	Stage           Stage  `json:"-"`
	ChallengeRef    string `json:"challenge_ref"`
	ProviderRef     string `json:"provider_ref"`
	Method          string `json:"method"`
	Credential      string `json:"credential"`
	ManifestRef     string `json:"manifest_ref"`
	ManifestVersion int64  `json:"manifest_version"`
	OperationRef    string `json:"-"`
}

type ChangeCommand struct {
	MissionRef    string `json:"-"`
	ProviderRef   string `json:"provider_ref,omitempty"`
	ReceiverRef   string `json:"receiver_ref,omitempty"`
	ReceiverPhone string `json:"receiver_phone,omitempty"`
	OperationRef  string `json:"-"`
}

type Observation struct {
	MissionRef    string `json:"mission_ref"`
	HandoverRef   string `json:"handover_ref"`
	Stage         Stage  `json:"stage"`
	Status        string `json:"status"`
	SourceVersion int64  `json:"source_version"`
}

type secretIssue struct {
	Observation
	OTP            string
	QR             string
	RecipientRef   string
	RecipientPhone string
	ExpiresAt      time.Time
}

type Policy struct {
	Enabled     bool
	TTL         time.Duration
	Cooldown    time.Duration
	MaxAttempts int
	MaxResends  int
}

func (p Policy) normalized() Policy {
	if p.TTL <= 0 {
		p.TTL = 15 * time.Minute
	}
	if p.Cooldown <= 0 {
		p.Cooldown = 45 * time.Second
	}
	if p.MaxAttempts <= 0 {
		p.MaxAttempts = 5
	}
	if p.MaxResends <= 0 {
		p.MaxResends = 5
	}
	return p
}
