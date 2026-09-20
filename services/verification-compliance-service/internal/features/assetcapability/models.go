// Package assetcapability is Section 12: the formal certification authority for
// operational assets.
//
// The distinction this package exists to hold is between an asset's EXISTENCE,
// which belongs to the driver service that owns it, and an asset's ADMISSION to
// a service product, which is a reviewed decision. driver-taxi-service knows a
// vehicle exists, what class it was registered as, and who drives it. It does
// not, and must not, get to decide that the vehicle is a Comfort vehicle:
// that is a claim about a physical condition somebody has to have checked.
//
// So the flow is one-directional. The owning authority OPENS a case about its
// own asset. An administrator DECIDES. Verification then pushes the decision
// back into the authority's own product-eligibility projection, which is where
// matching actually reads from. Verification never matches, never assigns, and
// never holds the asset's operational profile.
//
// Two vocabularies meet here and are deliberately not merged. Verification's
// decisions are VERIFIED/REJECTED/SUSPENDED/EXPIRED -- the language of review.
// The operational projections speak PENDING/ELIGIBLE/SUSPENDED/EXPIRED/REJECTED
// -- the language of dispatchability. The mapping happens once, in this package
// (VERIFIED -> ELIGIBLE), rather than three times in three driver services where
// it could drift.
package assetcapability

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Authority is a driver service that owns assets.
type Authority string

const (
	AuthorityTaxi     Authority = "TAXI"
	AuthorityDispatch Authority = "DISPATCH"
	AuthorityHauling  Authority = "HAULING"
)

// Authorities lists every authority in a stable order, so callers and tests
// never depend on map iteration.
func Authorities() []Authority {
	return []Authority{AuthorityTaxi, AuthorityDispatch, AuthorityHauling}
}

// ParseAuthority resolves a wire value to an authority.
func ParseAuthority(value string) (Authority, error) {
	switch Authority(strings.ToUpper(strings.TrimSpace(value))) {
	case AuthorityTaxi:
		return AuthorityTaxi, nil
	case AuthorityDispatch:
		return AuthorityDispatch, nil
	case AuthorityHauling:
		return AuthorityHauling, nil
	}
	return "", fmt.Errorf("%w: %q is not an asset authority", ErrValidation, value)
}

// OwningService is the service name that may open cases about this authority's
// assets.
//
// This mapping is the whole of the "only the owning authority may create a
// case" rule (Section 5.8). It lives here as data rather than in a handler
// conditional so the rule is one table a reader can check against, and so
// adding an authority cannot accidentally leave the check behind.
func (a Authority) OwningService() string {
	switch a {
	case AuthorityTaxi:
		return "driver-taxi-service"
	case AuthorityDispatch:
		return "driver-dispatch-delivery-service"
	case AuthorityHauling:
		return "driver-hauling-service"
	}
	return ""
}

// AssetFieldName is the name the authority's own certification ingress uses for
// the asset id. The three services each named it after their own vehicle, and
// renaming any of them would be a breaking change to a shipped contract for no
// behavioural gain, so the adapter carries the difference instead.
func (a Authority) AssetFieldName() string {
	switch a {
	case AuthorityTaxi:
		return "vehicle_id"
	case AuthorityDispatch:
		return "bike_id"
	case AuthorityHauling:
		return "truck_id"
	}
	return "asset_id"
}

// AdminService is the only caller permitted to record a formal decision.
const AdminService = "admin-backoffice-service"

// ServiceName is this service's own signing identity.
const ServiceName = "verification-compliance-service"

// CaseStatus is the review lifecycle of a capability case. It reuses the exact
// vocabulary the vendor verification cases already use, so this service has one
// case vocabulary rather than two that nearly agree.
type CaseStatus string

const (
	CaseNotStarted    CaseStatus = "NOT_STARTED"
	CaseInProgress    CaseStatus = "IN_PROGRESS"
	CasePendingReview CaseStatus = "PENDING_REVIEW"
	CaseVerified      CaseStatus = "VERIFIED"
	CaseRejected      CaseStatus = "REJECTED"
	CaseExpired       CaseStatus = "EXPIRED"
	CaseSuspended     CaseStatus = "SUSPENDED"
)

// Decision is a reviewer's formal conclusion.
type Decision string

const (
	DecisionVerified  Decision = "VERIFIED"
	DecisionRejected  Decision = "REJECTED"
	DecisionSuspended Decision = "SUSPENDED"
	DecisionExpired   Decision = "EXPIRED"
)

// Valid reports whether a decision is one this service recognises.
func (d Decision) Valid() bool {
	switch d {
	case DecisionVerified, DecisionRejected, DecisionSuspended, DecisionExpired:
		return true
	}
	return false
}

// CaseStatus projects a decision onto the case lifecycle.
func (d Decision) CaseStatus() CaseStatus {
	switch d {
	case DecisionVerified:
		return CaseVerified
	case DecisionRejected:
		return CaseRejected
	case DecisionSuspended:
		return CaseSuspended
	case DecisionExpired:
		return CaseExpired
	}
	return CasePendingReview
}

// CertificationStatus is the operational projections' vocabulary: what the
// three driver services store and what their matchers read.
type CertificationStatus string

const (
	CertPending   CertificationStatus = "PENDING"
	CertEligible  CertificationStatus = "ELIGIBLE"
	CertSuspended CertificationStatus = "SUSPENDED"
	CertExpired   CertificationStatus = "EXPIRED"
	CertRejected  CertificationStatus = "REJECTED"
)

// CertificationStatus maps a review decision onto dispatchability.
//
// This single line is the documented VERIFIED -> ELIGIBLE mapping Section 5.15
// asks for. Everything downstream of it speaks the operational vocabulary.
func (d Decision) CertificationStatus() CertificationStatus {
	switch d {
	case DecisionVerified:
		return CertEligible
	case DecisionRejected:
		return CertRejected
	case DecisionSuspended:
		return CertSuspended
	case DecisionExpired:
		return CertExpired
	}
	return CertPending
}

// TaxiDecisionWord is the word driver-taxi-service's ingress expects.
//
// Taxi speaks CERTIFIED/REVOKED/... where Dispatch and Hauling speak the
// eligibility statuses directly. An empty result means "do not deliver this
// status to Taxi", which is the honest answer for PENDING: Taxi's ingress has
// no PENDING and inventing one would silently admit a vehicle under review.
func (c CertificationStatus) TaxiDecisionWord() string {
	switch c {
	case CertEligible:
		return "CERTIFIED"
	case CertSuspended:
		return "SUSPENDED"
	case CertExpired:
		return "EXPIRED"
	case CertRejected:
		return "REJECTED"
	}
	return ""
}

// ProviderStatus is the provider's own verification standing with one authority.
type ProviderStatus string

const (
	ProviderPending   ProviderStatus = "PENDING"
	ProviderVerified  ProviderStatus = "VERIFIED"
	ProviderRejected  ProviderStatus = "REJECTED"
	ProviderSuspended ProviderStatus = "SUSPENDED"
	ProviderExpired   ProviderStatus = "EXPIRED"
)

// Valid reports whether a provider status is recognised.
func (p ProviderStatus) Valid() bool {
	switch p {
	case ProviderPending, ProviderVerified, ProviderRejected, ProviderSuspended, ProviderExpired:
		return true
	}
	return false
}

// DocumentType is the kind of evidence a vehicle document carries.
type DocumentType string

const (
	DocRegistration     DocumentType = "REGISTRATION"
	DocInsurance        DocumentType = "INSURANCE"
	DocRoadworthiness   DocumentType = "ROADWORTHINESS"
	DocPermit           DocumentType = "PERMIT"
	DocInspectionReport DocumentType = "INSPECTION_REPORT"
	DocOther            DocumentType = "OTHER"
)

// Valid reports whether a document type is recognised.
func (d DocumentType) Valid() bool {
	switch d {
	case DocRegistration, DocInsurance, DocRoadworthiness, DocPermit, DocInspectionReport, DocOther:
		return true
	}
	return false
}

// DocumentStatus is a document's validity.
//
// VALID means the document itself checks out. It emphatically does NOT mean the
// asset is admitted to anything: see CapabilityFromDocuments below.
type DocumentStatus string

const (
	DocumentPending    DocumentStatus = "PENDING"
	DocumentValid      DocumentStatus = "VALID"
	DocumentRejected   DocumentStatus = "REJECTED"
	DocumentExpired    DocumentStatus = "EXPIRED"
	DocumentSuperseded DocumentStatus = "SUPERSEDED"
)

// Valid reports whether a document status is recognised.
func (d DocumentStatus) Valid() bool {
	switch d {
	case DocumentPending, DocumentValid, DocumentRejected, DocumentExpired, DocumentSuperseded:
		return true
	}
	return false
}

// InspectionType and InspectionOutcome describe a physical or documentary check.
type (
	InspectionType    string
	InspectionOutcome string
)

const (
	InspectionPhysical InspectionType = "PHYSICAL"
	InspectionDocument InspectionType = "DOCUMENT"
	InspectionRemote   InspectionType = "REMOTE"
	InspectionPeriodic InspectionType = "PERIODIC"
	InspectionIncident InspectionType = "INCIDENT"

	InspectionPending InspectionOutcome = "PENDING"
	InspectionPassed  InspectionOutcome = "PASSED"
	InspectionFailed  InspectionOutcome = "FAILED"
	InspectionExpired InspectionOutcome = "EXPIRED"
)

// Valid reports whether an inspection type is recognised.
func (i InspectionType) Valid() bool {
	switch i {
	case InspectionPhysical, InspectionDocument, InspectionRemote, InspectionPeriodic, InspectionIncident:
		return true
	}
	return false
}

// Valid reports whether an inspection outcome is recognised.
func (i InspectionOutcome) Valid() bool {
	switch i {
	case InspectionPending, InspectionPassed, InspectionFailed, InspectionExpired:
		return true
	}
	return false
}

// Event types published to the outbox (Section 5.12).
const (
	EventCapabilityStatusChanged  = "asset.capability.status_changed"
	EventServiceTierStatusChanged = "asset.service_tier.status_changed"
	EventInspectionStatusChanged  = "asset.inspection.status_changed"
)

// SchemaVersion is the published event schema version.
const SchemaVersion = 1

// Outbox delivery states. NOT_REQUIRED is a real terminal state for a recorded
// fact with no consumer endpoint, so an honest outbox is never permanently
// backlogged by events nobody subscribes to.
const (
	DeliveryPending     = "PENDING"
	DeliveryNotRequired = "NOT_REQUIRED"
)

// Stable reason codes (Appendix B). Codes rather than sentences, so an operator
// surface, a driver service and a test can all match on the same value.
const (
	ReasonProviderNotVerified  = "PROVIDER_NOT_VERIFIED"
	ReasonEvidenceNotFound     = "EVIDENCE_NOT_FOUND"
	ReasonEvidenceNotActive    = "EVIDENCE_NOT_ACTIVE"
	ReasonEvidenceWrongOwner   = "EVIDENCE_WRONG_OWNER"
	ReasonEvidenceWrongPurpose = "EVIDENCE_WRONG_PURPOSE"
	ReasonDocumentNotFound     = "SUPPORTING_DOCUMENT_NOT_FOUND"
	ReasonDocumentNotValid     = "SUPPORTING_DOCUMENT_NOT_VALID"
	ReasonDocumentWrongAsset   = "SUPPORTING_DOCUMENT_WRONG_ASSET"
	ReasonInspectionWrongAsset = "INSPECTION_WRONG_ASSET"
	ReasonCertificationExpired = "CERTIFICATION_EXPIRED"
	ReasonInspectionExpired    = "INSPECTION_EXPIRED"
	ReasonDocumentExpired      = "DOCUMENT_EXPIRED"
	ReasonProviderExpired      = "PROVIDER_VERIFICATION_EXPIRED"
	ReasonCaseOpened           = "CASE_OPENED"
	ReasonReinspection         = "REINSPECTION_OPENED"
)

// Sentinel errors. Handlers map these to status codes; nothing below the HTTP
// layer knows what a status code is.
var (
	ErrValidation = errors.New("assetcapability: validation failed")
	ErrForbidden  = errors.New("assetcapability: caller is not permitted")
	ErrNotFound   = errors.New("assetcapability: not found")
	ErrConflict   = errors.New("assetcapability: idempotency conflict")
	ErrVersion    = errors.New("assetcapability: version conflict")
	ErrDisabled   = errors.New("assetcapability: certification is disabled")
	// ErrEvidence covers every way supporting evidence fails to bind: a missing
	// media asset, a revoked one, one owned by a different asset, or a cited
	// document that is not currently valid. They share an error because they
	// share a consequence -- the decision is refused, not retried.
	ErrEvidence = errors.New("assetcapability: supporting evidence is not acceptable")
)

// codePattern is the shared shape of a capability or product code. The three
// driver services already enforce it on their own product codes, so accepting a
// looser value here would only produce a certification no projection can store.
var codePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{1,63}$`)

// ValidCode reports whether a capability or product code is well formed.
func ValidCode(value string) bool { return codePattern.MatchString(value) }

// Case is one request to certify an asset for a service product.
type Case struct {
	CaseRef        string     `json:"case_ref"`
	Authority      Authority  `json:"authority"`
	ProviderID     string     `json:"provider_id"`
	AssetID        string     `json:"asset_id"`
	CapabilityCode string     `json:"capability_code"`
	ProductCode    string     `json:"product_code"`
	RequestedValue any        `json:"requested_value,omitempty"`
	EvidenceRefs   []string   `json:"evidence_refs"`
	DocumentRefs   []string   `json:"document_refs"`
	InspectionRef  string     `json:"inspection_ref,omitempty"`
	Status         CaseStatus `json:"status"`
	PolicyVersion  string     `json:"policy_version,omitempty"`
	SourceVersion  int64      `json:"source_version"`
	ReasonCode     string     `json:"reason_code,omitempty"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
	CorrelationID  string     `json:"correlation_id,omitempty"`
	ObservedAt     time.Time  `json:"observed_at"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// DecisionRecord is one append-only reviewer conclusion.
type DecisionRecord struct {
	DecisionRef     string     `json:"decision_ref"`
	CaseRef         string     `json:"case_ref"`
	DecisionVersion int64      `json:"decision_version"`
	Decision        Decision   `json:"decision"`
	VerifiedValue   any        `json:"verified_value,omitempty"`
	ReasonCode      string     `json:"reason_code,omitempty"`
	Reviewer        string     `json:"reviewer"`
	EvidenceRefs    []string   `json:"evidence_refs"`
	DocumentRefs    []string   `json:"document_refs"`
	PolicyVersion   string     `json:"policy_version,omitempty"`
	EffectiveAt     *time.Time `json:"effective_at,omitempty"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
	CorrelationID   string     `json:"correlation_id,omitempty"`
	DecidedAt       time.Time  `json:"decided_at"`
}

// Certification is the current authoritative answer for one asset and product.
type Certification struct {
	CertificationID  string              `json:"certification_id"`
	Authority        Authority           `json:"authority"`
	ProviderID       string              `json:"provider_id,omitempty"`
	AssetID          string              `json:"asset_id"`
	ProductCode      string              `json:"product_code"`
	CapabilityCode   string              `json:"capability_code,omitempty"`
	Status           CertificationStatus `json:"status"`
	PolicyVersion    string              `json:"policy_version,omitempty"`
	CertificationRef string              `json:"certification_ref,omitempty"`
	EffectiveAt      *time.Time          `json:"effective_at,omitempty"`
	ExpiresAt        *time.Time          `json:"expires_at,omitempty"`
	SourceVersion    int64               `json:"source_version"`
	ReasonCode       string              `json:"reason_code,omitempty"`
	CaseRef          string              `json:"case_ref,omitempty"`
	UpdatedAt        time.Time           `json:"updated_at"`
}

// Inspection is a recorded physical or documentary check of an asset.
type Inspection struct {
	InspectionRef  string            `json:"inspection_ref"`
	Authority      Authority         `json:"authority"`
	ProviderID     string            `json:"provider_id,omitempty"`
	AssetID        string            `json:"asset_id"`
	InspectionType InspectionType    `json:"inspection_type"`
	Outcome        InspectionOutcome `json:"outcome"`
	EvidenceRefs   []string          `json:"evidence_refs"`
	InspectedAt    *time.Time        `json:"inspected_at,omitempty"`
	ExpiresAt      *time.Time        `json:"expires_at,omitempty"`
	InspectorRef   string            `json:"inspector_ref,omitempty"`
	ReasonCode     string            `json:"reason_code,omitempty"`
	SourceVersion  int64             `json:"source_version"`
	CorrelationID  string            `json:"correlation_id,omitempty"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
}

// VehicleDocument is a reference to a document held in media-file-service,
// together with the validity Verification has recorded for it.
type VehicleDocument struct {
	DocumentRef    string         `json:"document_ref"`
	Authority      Authority      `json:"authority"`
	ProviderID     string         `json:"provider_id"`
	AssetID        string         `json:"asset_id"`
	DocumentType   DocumentType   `json:"document_type"`
	MediaAssetID   string         `json:"media_asset_id"`
	DocumentNumber string         `json:"document_number,omitempty"`
	Status         DocumentStatus `json:"status"`
	IssuedAt       *time.Time     `json:"issued_at,omitempty"`
	ExpiresAt      *time.Time     `json:"expires_at,omitempty"`
	ReasonCode     string         `json:"reason_code,omitempty"`
	SourceVersion  int64          `json:"source_version"`
	CorrelationID  string         `json:"correlation_id,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

// CurrentlyValid reports whether the document is valid at a point in time.
//
// Note what this does NOT do: it answers a question about the DOCUMENT, and
// there is deliberately no function anywhere in this package that turns a set
// of valid documents into a certification. Section 5.7's rule is structural,
// not advisory -- a valid registration and a valid insurance certificate
// together still say nothing about whether a vehicle's refrigeration holds
// minus eighteen degrees, and the only thing that can say so is a reviewed
// decision that a human recorded.
func (v VehicleDocument) CurrentlyValid(now time.Time) bool {
	if v.Status != DocumentValid {
		return false
	}
	return v.ExpiresAt == nil || v.ExpiresAt.After(now)
}

// ProviderVerification is a provider's standing with one authority.
type ProviderVerification struct {
	Authority     Authority      `json:"authority"`
	ProviderID    string         `json:"provider_id"`
	Status        ProviderStatus `json:"status"`
	ReasonCode    string         `json:"reason_code,omitempty"`
	SourceVersion int64          `json:"source_version"`
	EffectiveAt   *time.Time     `json:"effective_at,omitempty"`
	ExpiresAt     *time.Time     `json:"expires_at,omitempty"`
	CorrelationID string         `json:"correlation_id,omitempty"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

// CurrentlyVerified reports whether the provider may hold eligible assets now.
func (p ProviderVerification) CurrentlyVerified(now time.Time) bool {
	if p.Status != ProviderVerified {
		return false
	}
	return p.ExpiresAt == nil || p.ExpiresAt.After(now)
}
