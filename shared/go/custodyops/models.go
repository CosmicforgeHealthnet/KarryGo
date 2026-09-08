// Package custodyops provides the shared, authority-local contract for frozen
// manifests, typed evidence, delivery recovery, returns, and provider custody
// transfer. Each execution service persists these records in its own database.
package custodyops

import (
	"errors"
	"strings"
	"time"
)

var (
	ErrInvalid       = errors.New("custody operation is invalid")
	ErrNotFound      = errors.New("custody operation was not found")
	ErrConflict      = errors.New("custody operation conflicts with canonical state")
	ErrUnauthorized  = errors.New("custody actor is not authorized")
	ErrPolicy        = errors.New("custody recovery action is not allowed by policy")
	ErrMediaRejected = errors.New("media reference is outside the mission scope")
)

type EvidenceType string

const (
	EvidencePickup           EvidenceType = "PICKUP"
	EvidenceDelivery         EvidenceType = "DELIVERY"
	EvidenceDamage           EvidenceType = "DAMAGE"
	EvidenceDiscrepancy      EvidenceType = "DISCREPANCY"
	EvidenceFailedAttempt    EvidenceType = "FAILED_ATTEMPT"
	EvidenceReturn           EvidenceType = "RETURN"
	EvidenceProviderTransfer EvidenceType = "PROVIDER_TRANSFER"
)

type ExceptionReason string

const (
	ExceptionVendorNotReady      ExceptionReason = "VENDOR_NOT_READY"
	ExceptionPackageMismatch     ExceptionReason = "PACKAGE_MISMATCH"
	ExceptionProhibitedItem      ExceptionReason = "PROHIBITED_OR_RESTRICTED_ITEM"
	ExceptionDamagedPackage      ExceptionReason = "DAMAGED_PACKAGE"
	ExceptionReceiverUnavailable ExceptionReason = "RECEIVER_UNAVAILABLE"
	ExceptionReceiverRefused     ExceptionReason = "RECEIVER_REFUSED"
	ExceptionWrongAddress        ExceptionReason = "WRONG_ADDRESS"
	ExceptionAddressInaccessible ExceptionReason = "ADDRESS_INACCESSIBLE"
	ExceptionProviderBreakdown   ExceptionReason = "PROVIDER_BREAKDOWN"
	ExceptionWeatherOrRoad       ExceptionReason = "WEATHER_OR_ROAD"
	ExceptionSecurityRisk        ExceptionReason = "SECURITY_RISK"
)

type RecoveryAction string

const (
	ActionWait                RecoveryAction = "WAIT"
	ActionReschedule          RecoveryAction = "RESCHEDULE"
	ActionAlternativeReceiver RecoveryAction = "ALTERNATIVE_RECEIVER"
	ActionReturnToSender      RecoveryAction = "RETURN_TO_SENDER"
	ActionReturnToVendor      RecoveryAction = "RETURN_TO_VENDOR"
	ActionControlledHolding   RecoveryAction = "CONTROLLED_HOLDING"
	ActionProviderReplacement RecoveryAction = "PROVIDER_REPLACEMENT"
)

type IncidentReason string

const (
	IncidentBreakdown      IncidentReason = "BREAKDOWN"
	IncidentIllness        IncidentReason = "ILLNESS"
	IncidentSafetyIncident IncidentReason = "SAFETY_INCIDENT"
)

type Parcel struct {
	ParcelRef string `json:"parcel_ref"`
	StopRef   string `json:"stop_ref"`
	Count     int64  `json:"package_count"`
}

type FreezeManifestCommand struct {
	OperationRef         string    `json:"operation_ref"`
	MissionRef           string    `json:"mission_ref"`
	TenantRef            string    `json:"tenant_ref"`
	BusinessRef          string    `json:"business_ref"`
	BranchRef            string    `json:"branch_ref"`
	ManifestRef          string    `json:"manifest_ref"`
	ManifestVersion      int64     `json:"manifest_version"`
	PickupStopRef        string    `json:"pickup_stop_ref"`
	ReceiverChallengeRef string    `json:"receiver_challenge_ref"`
	Parcels              []Parcel  `json:"parcels"`
	CreatedAt            time.Time `json:"created_at"`
}

type Manifest struct {
	Authority            string    `json:"authority"`
	MissionRef           string    `json:"mission_ref"`
	TenantRef            string    `json:"tenant_ref"`
	BusinessRef          string    `json:"business_ref"`
	BranchRef            string    `json:"branch_ref"`
	ManifestRef          string    `json:"manifest_ref"`
	ManifestVersion      int64     `json:"manifest_version"`
	PickupStopRef        string    `json:"pickup_stop_ref"`
	ReceiverChallengeRef string    `json:"receiver_challenge_ref"`
	ParcelRefs           []string  `json:"parcel_refs"`
	PackageCount         int64     `json:"package_count"`
	CreatedAt            time.Time `json:"created_at"`
	FrozenAt             time.Time `json:"frozen_at"`
	Status               string    `json:"status"`
}

type ConfirmParcelsCommand struct {
	OperationRef    string   `json:"operation_ref"`
	MissionRef      string   `json:"mission_ref"`
	TenantRef       string   `json:"tenant_ref"`
	BusinessRef     string   `json:"business_ref"`
	BranchRef       string   `json:"branch_ref"`
	PickupStopRef   string   `json:"pickup_stop_ref"`
	ManifestRef     string   `json:"manifest_ref"`
	ManifestVersion int64    `json:"manifest_version"`
	ProviderRef     string   `json:"provider_ref"`
	ParcelRefs      []string `json:"parcel_refs"`
	PackageCount    int64    `json:"package_count"`
}

type EvidenceCommand struct {
	OperationRef    string       `json:"operation_ref"`
	MissionRef      string       `json:"mission_ref"`
	TenantRef       string       `json:"tenant_ref"`
	ManifestRef     string       `json:"manifest_ref"`
	ManifestVersion int64        `json:"manifest_version"`
	StopRef         string       `json:"stop_ref,omitempty"`
	ParcelRefs      []string     `json:"parcel_refs,omitempty"`
	EvidenceType    EvidenceType `json:"evidence_type"`
	MediaRef        string       `json:"media_ref"`
	ProviderRef     string       `json:"provider_ref"`
	ActorRef        string       `json:"actor_ref"`
	CapturedAt      time.Time    `json:"captured_at"`
	Latitude        *float64     `json:"latitude,omitempty"`
	Longitude       *float64     `json:"longitude,omitempty"`
	AccessPolicy    string       `json:"access_policy"`
}

type Evidence struct {
	ID string `json:"evidence_id"`
	EvidenceCommand
	Authority string    `json:"authority"`
	CreatedAt time.Time `json:"created_at"`
}

type ExceptionCommand struct {
	OperationRef                    string          `json:"operation_ref"`
	MissionRef                      string          `json:"mission_ref"`
	TenantRef                       string          `json:"tenant_ref"`
	ProviderRef                     string          `json:"provider_ref"`
	ActorRef                        string          `json:"actor_ref"`
	Stage                           string          `json:"stage"`
	Reason                          ExceptionReason `json:"reason_code"`
	RecoveryAction                  RecoveryAction  `json:"recovery_action"`
	ManifestRef                     string          `json:"manifest_ref"`
	ManifestVersion                 int64           `json:"manifest_version"`
	ParcelRefs                      []string        `json:"parcel_refs,omitempty"`
	MediaRef                        string          `json:"media_ref,omitempty"`
	SafeNote                        string          `json:"safe_note,omitempty"`
	OccurredAt                      time.Time       `json:"occurred_at"`
	Latitude                        *float64        `json:"latitude,omitempty"`
	Longitude                       *float64        `json:"longitude,omitempty"`
	ScheduledFor                    *time.Time      `json:"scheduled_for,omitempty"`
	ReceiverRef                     string          `json:"receiver_ref,omitempty"`
	PreviousReceiverChallengeRef    string          `json:"previous_receiver_challenge_ref,omitempty"`
	ReplacementReceiverChallengeRef string          `json:"replacement_receiver_challenge_ref,omitempty"`
	HoldingRef                      string          `json:"holding_ref,omitempty"`
	FeeQuoteRef                     string          `json:"fee_quote_ref,omitempty"`
	FeeApprovalRef                  string          `json:"fee_approval_ref,omitempty"`
}

type ReturnCommand struct {
	OperationRef     string   `json:"operation_ref"`
	MissionRef       string   `json:"mission_ref"`
	TenantRef        string   `json:"tenant_ref"`
	OriginalManifest string   `json:"original_manifest_ref"`
	OriginalVersion  int64    `json:"original_manifest_version"`
	ReturnManifest   string   `json:"return_manifest_ref"`
	ParcelRefs       []string `json:"parcel_refs"`
	ParcelCondition  string   `json:"parcel_condition"`
	DestinationRef   string   `json:"return_destination_ref"`
	ReceiverRef      string   `json:"receiving_actor_ref"`
	Reason           string   `json:"reason_code"`
	CategoryPolicy   string   `json:"category_policy"`
	EvidenceRef      string   `json:"evidence_ref"`
}

type ReturnCompletionCommand struct {
	OperationRef   string   `json:"operation_ref"`
	MissionRef     string   `json:"mission_ref"`
	TenantRef      string   `json:"tenant_ref"`
	ProviderRef    string   `json:"provider_ref"`
	ReturnManifest string   `json:"return_manifest_ref"`
	ParcelRefs     []string `json:"parcel_refs"`
	EvidenceRef    string   `json:"evidence_ref"`
}

type IncidentCommand struct {
	OperationRef string         `json:"operation_ref"`
	MissionRef   string         `json:"mission_ref"`
	TenantRef    string         `json:"tenant_ref"`
	ProviderRef  string         `json:"provider_ref"`
	Reason       IncidentReason `json:"reason_code"`
	OccurredAt   time.Time      `json:"occurred_at"`
	Latitude     *float64       `json:"latitude,omitempty"`
	Longitude    *float64       `json:"longitude,omitempty"`
	SafeNote     string         `json:"safe_note,omitempty"`
}

type AssignReplacementCommand struct {
	OperationRef        string   `json:"operation_ref"`
	MissionRef          string   `json:"mission_ref"`
	TenantRef           string   `json:"tenant_ref"`
	OldProviderRef      string   `json:"old_provider_ref"`
	NewProviderRef      string   `json:"new_provider_ref"`
	VehicleClass        string   `json:"vehicle_class"`
	CapacityCount       int64    `json:"capacity_count"`
	Handling            []string `json:"handling_capabilities"`
	ComplianceRef       string   `json:"compliance_ref"`
	AvailabilityRef     string   `json:"availability_ref"`
	ManifestRef         string   `json:"manifest_ref"`
	ManifestVersion     int64    `json:"manifest_version"`
	ReceiverChallenge   string   `json:"receiver_challenge_ref"`
	AllocationPolicyRef string   `json:"allocation_policy_ref,omitempty"`
}

type TransferCommand struct {
	OperationRef            string   `json:"operation_ref"`
	MissionRef              string   `json:"mission_ref"`
	TenantRef               string   `json:"tenant_ref"`
	TransferRef             string   `json:"transfer_ref"`
	OldProviderRef          string   `json:"old_provider_ref"`
	NewProviderRef          string   `json:"new_provider_ref"`
	ManifestRef             string   `json:"manifest_ref"`
	ManifestVersion         int64    `json:"manifest_version"`
	ParcelRefs              []string `json:"parcel_refs"`
	PackageCount            int64    `json:"package_count"`
	EvidenceRef             string   `json:"evidence_ref"`
	Latitude                float64  `json:"latitude"`
	Longitude               float64  `json:"longitude"`
	OldProviderConfirm      bool     `json:"old_provider_confirmed"`
	NewProviderConfirm      bool     `json:"new_provider_confirmed"`
	NewReceiverChallengeRef string   `json:"new_receiver_challenge_ref"`
	OldAuthority            string   `json:"old_authority,omitempty"`
	OldAuthorityMissionRef  string   `json:"old_authority_mission_ref,omitempty"`
	NewAuthority            string   `json:"new_authority,omitempty"`
	NewAuthorityMissionRef  string   `json:"new_authority_mission_ref,omitempty"`
	FinancialAllocationRef  string   `json:"financial_allocation_ref,omitempty"`
	SourceEventRef          string   `json:"source_event_ref,omitempty"`
	SourceVersion           int64    `json:"source_version,omitempty"`
}

type ProviderAllocation struct {
	ProviderRef string `json:"provider_ref"`
	AmountMinor int64  `json:"amount_minor"`
}

type AllocationCommand struct {
	OperationRef       string               `json:"operation_ref"`
	MissionRef         string               `json:"mission_ref"`
	TenantRef          string               `json:"tenant_ref"`
	PolicyRef          string               `json:"allocation_policy_ref"`
	ApprovedTotalMinor int64                `json:"approved_provider_earning_minor"`
	Currency           string               `json:"currency"`
	WalletOperationRef string               `json:"wallet_operation_ref"`
	WalletStatus       string               `json:"wallet_status"`
	Allocations        []ProviderAllocation `json:"allocations"`
}

type Result struct {
	OperationRef string    `json:"operation_ref"`
	MissionRef   string    `json:"mission_ref"`
	State        string    `json:"state"`
	Version      int64     `json:"version"`
	Replayed     bool      `json:"replayed"`
	CreatedAt    time.Time `json:"created_at"`
}

type Policy struct {
	ControlledHoldingRefs map[string]struct{}
}

func trim(value string) string { return strings.TrimSpace(value) }
