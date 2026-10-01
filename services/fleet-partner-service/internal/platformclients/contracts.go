package platformclients

import (
	"context"
	"errors"
	"time"
)

// The contracts below describe capabilities this service needs from
// authoritative services for Sections 8–10.
//
// AUDIT FINDING (docs/auth-onboarding-iam-provider-audit.md §1.5): none of
// these endpoints exists in the repository today. Rather than invent live
// endpoint paths, each capability is a typed interface with an explicitly
// unconfigured adapter that returns ErrNotConfigured, plus a reconciliation
// job that re-drives the operation once a real adapter is wired. No
// authoritative service's database is ever read as a fallback.

// ---- Verification (verification-compliance-service) ----

// VerificationSubmission is an organization verification request.
type VerificationSubmission struct {
	OrganizationID  string
	SubmissionID    string
	Snapshot        map[string]any
	DocumentFileIDs []string
	IdempotencyKey  string
}

// VerificationOutcome is the authoritative verification state.
type VerificationOutcome struct {
	ExternalReference string
	Status            string
	RestrictionStatus string
	SourceVersion     int64
	Issues            []VerificationIssue
}

// VerificationIssue is one review finding.
type VerificationIssue struct {
	Code              string
	FieldReference    string
	DocumentReference string
	Remediation       string
}

// VerificationContract is the organization verification capability.
type VerificationContract interface {
	SubmitOrganization(ctx context.Context, meta CallMeta, submission VerificationSubmission) (VerificationOutcome, error)
	// ReadOrganizationStatus is the read-back used after a timeout, so an
	// unknown outcome is resolved by asking rather than by assuming.
	ReadOrganizationStatus(ctx context.Context, meta CallMeta, organizationID string, submissionID string) (VerificationOutcome, error)
	Configured() bool
}

// ---- Wallet (payment-wallet-service) ----

// WalletProvisionRequest asks for an organization wallet or provider
// sub-wallet. It carries no amount: this service never moves money.
type WalletProvisionRequest struct {
	OrganizationID string
	// ProviderMembershipID is set only for provider sub-wallets.
	ProviderMembershipID string
	Currency             string
	IdempotencyKey       string
}

// WalletAccount is the authoritative wallet identity projection. It
// deliberately carries no balance.
type WalletAccount struct {
	AccountID     string
	Status        string
	SourceVersion int64
}

// WalletContract is the wallet provisioning capability.
type WalletContract interface {
	EnsureOrganizationWallet(ctx context.Context, meta CallMeta, request WalletProvisionRequest) (WalletAccount, error)
	EnsureProviderSubWallet(ctx context.Context, meta CallMeta, request WalletProvisionRequest) (WalletAccount, error)
	ReadAccount(ctx context.Context, meta CallMeta, accountID string) (WalletAccount, error)
	// DisableForClosure transitions wallet access during organization closure
	// or provider offboarding, per the wallet service's closure contract.
	DisableForClosure(ctx context.Context, meta CallMeta, accountID string, reason string) error
	Configured() bool
}

// ---- Media (media-file-service) ----

// FileMetadata is the authoritative description of an uploaded file.
type FileMetadata struct {
	FileID        string
	OwnerType     string
	OwnerID       string
	Purpose       string
	UploadState   string
	ScanState     string
	ContentType   string
	SizeBytes     int64
	SourceVersion int64
}

// Ready reports whether the file may be attached to a business record.
func (f FileMetadata) Ready() bool {
	return f.UploadState == "COMPLETED" &&
		(f.ScanState == "PENDING" || f.ScanState == "CLEAN" || f.ScanState == "NOT_REQUIRED")
}

// MediaContract is the file metadata and ownership capability.
// MediaContract is the file metadata and ownership capability.
type MediaContract interface {
	GetFile(ctx context.Context, meta CallMeta, fileID string) (FileMetadata, error)
	AuthorizeUpload(ctx context.Context, meta CallMeta, request MediaUploadRequest) (MediaUploadGrant, error)
	CompleteUpload(ctx context.Context, meta CallMeta, grant MediaUploadGrant,
		declaredMIME string, checksum string, content []byte) (MediaObjectState, error)
	Configured() bool
}

// ---- Provider services (dispatch / taxi / hauling) ----

// ProviderResolveRequest resolves an identity by BOTH normalized contacts.
type ProviderResolveRequest struct {
	EmailNormalized string
	PhoneE164       string
}

// ProviderResolution is the outcome of a resolve call.
type ProviderResolution struct {
	// Outcome is MATCHED, NOT_FOUND or CONFLICT. CONFLICT means email and
	// phone identify different providers and must never be auto-merged.
	Outcome         string
	ProviderID      string
	PlatformStatus  string
	ManagedByOrgID  string
	ServiceCategory string
	SourceVersion   int64
}

// Provider resolve outcomes.
const (
	ProviderResolveMatched  = "MATCHED"
	ProviderResolveNotFound = "NOT_FOUND"
	ProviderResolveConflict = "CONFLICT"
)

// ProviderProvisionRequest asks the owning service to create an
// organization-sponsored provider identity.
type ProviderProvisionRequest struct {
	OrganizationID  string
	LegalName       string
	EmailNormalized string
	PhoneE164       string
	ServiceCategory string
	// OnboardingMode and ProviderMode are fixed by the platform contract.
	OnboardingMode string
	ProviderMode   string
	IdempotencyKey string
}

// ProviderIdentity is the authoritative provider identity projection.
type ProviderIdentity struct {
	ProviderID     string
	PlatformStatus string
	SourceVersion  int64
}

// ProviderAssociationRequest creates an organization association that the
// provider must accept or decline themselves.
type ProviderAssociationRequest struct {
	OrganizationID   string
	OrganizationName string
	ProviderID       string
	MembershipID     string
	RelationshipType string
	IdempotencyKey   string
}

// ProviderAssociation is the authoritative association state.
type ProviderAssociation struct {
	AssociationID string
	// Status is AWAITING_PROVIDER, ACCEPTED, DECLINED, EXPIRED or CANCELLED.
	Status        string
	SourceVersion int64
	DecidedAt     time.Time
}

// Association statuses.
const (
	AssociationAwaitingProvider = "AWAITING_PROVIDER"
	AssociationAccepted         = "ACCEPTED"
	AssociationDeclined         = "DECLINED"
	AssociationExpired          = "EXPIRED"
	AssociationCancelled        = "CANCELLED"
)

// ProviderContract is the capability set of one owning provider service.
//
// Provider authentication is NOT part of this contract: providers sign in
// through their owning service's existing passwordless email+phone OTP flow.
// This service never issues a provider credential.
type ProviderContract interface {
	Resolve(ctx context.Context, meta CallMeta, request ProviderResolveRequest) (ProviderResolution, error)
	ProvisionSponsored(ctx context.Context, meta CallMeta, request ProviderProvisionRequest) (ProviderIdentity, error)
	ReadProvider(ctx context.Context, meta CallMeta, providerID string) (ProviderIdentity, error)
	CreateAssociation(ctx context.Context, meta CallMeta, request ProviderAssociationRequest) (ProviderAssociation, error)
	ReadAssociation(ctx context.Context, meta CallMeta, associationID string) (ProviderAssociation, error)
	// RevokeOrganizationEligibility removes the provider from an
	// organization's matching eligibility during suspension or offboarding.
	RevokeOrganizationEligibility(ctx context.Context, meta CallMeta, providerID string, organizationID string, reason string) error
	Configured() bool
}

// HaulingLink is the data-minimized projection of an authoritative provider
// or truck relationship.
type HaulingLink struct {
	LinkID         string `json:"linkId"`
	OrganizationID string `json:"organizationId"`
	BranchID       string `json:"branchId"`
	MembershipID   string `json:"membershipId"`
	ProviderID     string `json:"providerId"`
	TruckID        string `json:"truckId"`
	Status         string `json:"status"`
	SourceVersion  int64  `json:"sourceVersion"`
}

type HaulingLinkRequest struct {
	OrganizationID string
	BranchID       string
	MembershipID   string
	ProviderID     string
	TruckID        string
	IdempotencyKey string
}

type HaulingLinkTransition struct {
	State           string
	ExpectedVersion int64
	OrganizationID  string
	BranchID        string
	ReasonCode      string
	IdempotencyKey  string
}

type HaulingLinkContract interface {
	CreateProviderLink(context.Context, CallMeta, HaulingLinkRequest) (HaulingLink, error)
	ReadProviderLink(context.Context, CallMeta, string) (HaulingLink, error)
	TransitionProviderLink(context.Context, CallMeta, string, HaulingLinkTransition) (HaulingLink, error)
	CreateTruckLink(context.Context, CallMeta, HaulingLinkRequest) (HaulingLink, error)
	ReadTruckLink(context.Context, CallMeta, string) (HaulingLink, error)
	TransitionTruckLink(context.Context, CallMeta, string, HaulingLinkTransition) (HaulingLink, error)
	Configured() bool
}

// ---- Unconfigured adapters ----

// ErrContractNotImplemented means the authoritative service does not expose
// this capability yet. It is deliberately distinct from a transport failure.
var ErrContractNotImplemented = errors.New("platformclients: contract is not implemented by the authoritative service yet")

func notConfigured(service string) error {
	return &Error{Service: service, Outcome: OutcomeUnknown, Message: "capability is not configured"}
}

// UnconfiguredVerification fails closed for every verification call.
type UnconfiguredVerification struct{ Service string }

func (u UnconfiguredVerification) SubmitOrganization(context.Context, CallMeta, VerificationSubmission) (VerificationOutcome, error) {
	return VerificationOutcome{}, notConfigured(u.Service)
}
func (u UnconfiguredVerification) ReadOrganizationStatus(context.Context, CallMeta, string, string) (VerificationOutcome, error) {
	return VerificationOutcome{}, notConfigured(u.Service)
}
func (u UnconfiguredVerification) Configured() bool { return false }

// UnconfiguredWallet fails closed for every wallet call.
type UnconfiguredWallet struct{ Service string }

func (u UnconfiguredWallet) EnsureOrganizationWallet(context.Context, CallMeta, WalletProvisionRequest) (WalletAccount, error) {
	return WalletAccount{}, notConfigured(u.Service)
}
func (u UnconfiguredWallet) EnsureProviderSubWallet(context.Context, CallMeta, WalletProvisionRequest) (WalletAccount, error) {
	return WalletAccount{}, notConfigured(u.Service)
}
func (u UnconfiguredWallet) ReadAccount(context.Context, CallMeta, string) (WalletAccount, error) {
	return WalletAccount{}, notConfigured(u.Service)
}
func (u UnconfiguredWallet) DisableForClosure(context.Context, CallMeta, string, string) error {
	return notConfigured(u.Service)
}
func (u UnconfiguredWallet) Configured() bool { return false }

// UnconfiguredMedia fails closed for every media call.
// UnconfiguredMedia fails closed for every media call.
type UnconfiguredMedia struct{ Service string }

func (u UnconfiguredMedia) GetFile(context.Context, CallMeta, string) (FileMetadata, error) {
	return FileMetadata{}, notConfigured(u.Service)
}
func (u UnconfiguredMedia) AuthorizeUpload(context.Context, CallMeta, MediaUploadRequest) (MediaUploadGrant, error) {
	return MediaUploadGrant{}, notConfigured(u.Service)
}
func (u UnconfiguredMedia) CompleteUpload(context.Context, CallMeta, MediaUploadGrant, string, string, []byte) (MediaObjectState, error) {
	return MediaObjectState{}, notConfigured(u.Service)
}
func (u UnconfiguredMedia) Configured() bool { return false }

// UnconfiguredProvider fails closed for every provider call.
type UnconfiguredProvider struct{ Service string }

func (u UnconfiguredProvider) Resolve(context.Context, CallMeta, ProviderResolveRequest) (ProviderResolution, error) {
	return ProviderResolution{}, notConfigured(u.Service)
}
func (u UnconfiguredProvider) ProvisionSponsored(context.Context, CallMeta, ProviderProvisionRequest) (ProviderIdentity, error) {
	return ProviderIdentity{}, notConfigured(u.Service)
}
func (u UnconfiguredProvider) ReadProvider(context.Context, CallMeta, string) (ProviderIdentity, error) {
	return ProviderIdentity{}, notConfigured(u.Service)
}
func (u UnconfiguredProvider) CreateAssociation(context.Context, CallMeta, ProviderAssociationRequest) (ProviderAssociation, error) {
	return ProviderAssociation{}, notConfigured(u.Service)
}
func (u UnconfiguredProvider) ReadAssociation(context.Context, CallMeta, string) (ProviderAssociation, error) {
	return ProviderAssociation{}, notConfigured(u.Service)
}
func (u UnconfiguredProvider) RevokeOrganizationEligibility(context.Context, CallMeta, string, string, string) error {
	return notConfigured(u.Service)
}
func (u UnconfiguredProvider) Configured() bool { return false }

// ContractSet bundles the Section 8–10 contracts.
type ContractSet struct {
	Verification VerificationContract
	Wallet       WalletContract
	Media        MediaContract
	// Providers is keyed by service category (DISPATCH, TAXI, HAULING).
	Providers    map[string]ProviderContract
	HaulingLinks HaulingLinkContract
}

// Service category to owning service mapping.
const (
	CategoryDispatch = "DISPATCH"
	CategoryTaxi     = "TAXI"
	CategoryHauling  = "HAULING"
)

// OwningService maps a service category to its authoritative service name.
func OwningService(category string) (string, bool) {
	switch category {
	case CategoryDispatch:
		return "driver-dispatch-delivery-service", true
	case CategoryTaxi:
		return "driver-taxi-service", true
	case CategoryHauling:
		return "driver-hauling-service", true
	default:
		return "", false
	}
}

// NewUnconfiguredContracts builds the fail-closed contract set used until the
// authoritative endpoints exist.
func NewUnconfiguredContracts() ContractSet {
	return ContractSet{
		Verification: UnconfiguredVerification{Service: "verification-compliance-service"},
		Wallet:       UnconfiguredWallet{Service: "payment-wallet-service"},
		Media:        UnconfiguredMedia{Service: "media-file-service"},
		Providers: map[string]ProviderContract{
			CategoryDispatch: UnconfiguredProvider{Service: "driver-dispatch-delivery-service"},
			CategoryTaxi:     UnconfiguredProvider{Service: "driver-taxi-service"},
			CategoryHauling:  UnconfiguredProvider{Service: "driver-hauling-service"},
		},
		HaulingLinks: UnconfiguredHaulingLinks{},
	}
}

type UnconfiguredHaulingLinks struct{}

func (UnconfiguredHaulingLinks) CreateProviderLink(context.Context, CallMeta, HaulingLinkRequest) (HaulingLink, error) {
	return HaulingLink{}, notConfigured("driver-hauling-service")
}
func (UnconfiguredHaulingLinks) ReadProviderLink(context.Context, CallMeta, string) (HaulingLink, error) {
	return HaulingLink{}, notConfigured("driver-hauling-service")
}
func (UnconfiguredHaulingLinks) TransitionProviderLink(context.Context, CallMeta, string, HaulingLinkTransition) (HaulingLink, error) {
	return HaulingLink{}, notConfigured("driver-hauling-service")
}
func (UnconfiguredHaulingLinks) CreateTruckLink(context.Context, CallMeta, HaulingLinkRequest) (HaulingLink, error) {
	return HaulingLink{}, notConfigured("driver-hauling-service")
}
func (UnconfiguredHaulingLinks) ReadTruckLink(context.Context, CallMeta, string) (HaulingLink, error) {
	return HaulingLink{}, notConfigured("driver-hauling-service")
}
func (UnconfiguredHaulingLinks) TransitionTruckLink(context.Context, CallMeta, string, HaulingLinkTransition) (HaulingLink, error) {
	return HaulingLink{}, notConfigured("driver-hauling-service")
}
func (UnconfiguredHaulingLinks) Configured() bool { return false }
