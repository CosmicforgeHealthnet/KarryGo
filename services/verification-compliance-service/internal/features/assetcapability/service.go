package assetcapability

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Service holds Section 12's business rules.
//
// Everything that decides WHETHER something may happen lives here; the HTTP
// layer decides only how to say so. That split matters most on the decision
// path, where four separate refusals (wrong caller, disabled feature, stale
// version, unverified provider) would otherwise be scattered across a handler
// and become impossible to test as a set.
type Service struct {
	repo     *Repository
	evidence *EvidenceValidator
	enabled  bool
	now      func() time.Time
}

// Options configures a Service.
type Options struct {
	Repository *Repository

	// Evidence validates cited media. Nil means no Media integration is
	// configured, and every citation is refused rather than waved through.
	Evidence *EvidenceValidator

	// Enabled is ASSET_CAPABILITY_CERTIFICATION_ENABLED. When false every
	// WRITE path refuses. Reads stay open deliberately: an operational service
	// asking "is this asset certified?" during a rollout pause must still get
	// the true stored answer, because the alternative is that a disabled flag
	// makes certification-required work look unrestricted (Section 5.16).
	Enabled bool

	Now func() time.Time
}

// NewService builds the feature service.
func NewService(options Options) *Service {
	now := options.Now
	if now == nil {
		now = time.Now
	}
	return &Service{
		repo:     options.Repository,
		evidence: options.Evidence,
		enabled:  options.Enabled,
		now:      now,
	}
}

// Enabled reports whether write paths are active.
func (s *Service) Enabled() bool { return s != nil && s.enabled }

func (s *Service) writable() error {
	if !s.Enabled() {
		return ErrDisabled
	}
	return nil
}

// Fingerprint is the stable digest of a request's meaningful content.
//
// It answers a different question from the idempotency key: the key asks "is
// this the same operation?", the fingerprint asks "is it still asking for the
// same thing?". A retry whose content changed is a conflict, not a duplicate.
func Fingerprint(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		raw = []byte(fmt.Sprintf("%v", value))
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// ── Cases ───────────────────────────────────────────────────────────────────

// OpenCaseRequest is one authority's request to certify its own asset.
type OpenCaseRequest struct {
	Caller         string
	Authority      Authority
	ProviderID     string
	AssetID        string
	CapabilityCode string
	ProductCode    string
	RequestedValue any
	EvidenceRefs   []string
	DocumentRefs   []string
	InspectionRef  string
	PolicyVersion  string
	ExpiresAt      *time.Time
	CorrelationID  string
	IdempotencyKey string
}

// CaseView is a case plus the certification it currently corresponds to.
type CaseView struct {
	Case          Case           `json:"case"`
	Certification *Certification `json:"certification,omitempty"`
}

// OpenCase records a capability case and moves the asset's certification into
// review where that is legitimate.
//
// The caller check is the load-bearing part: only the service that owns an
// asset may raise a question about it. Letting Fleet or Vendor mint a case
// would let a consumer of certification manufacture the certification it wants
// to consume, which is the whole authority inversion Section 3 forbids.
func (s *Service) OpenCase(ctx context.Context, request OpenCaseRequest) (CaseView, error) {
	if err := s.writable(); err != nil {
		return CaseView{}, err
	}
	if request.Caller != request.Authority.OwningService() {
		return CaseView{}, fmt.Errorf("%w: only %s may open a %s asset case",
			ErrForbidden, request.Authority.OwningService(), request.Authority)
	}
	if strings.TrimSpace(request.IdempotencyKey) == "" {
		return CaseView{}, fmt.Errorf("%w: an idempotency key is required", ErrValidation)
	}
	if strings.TrimSpace(request.ProviderID) == "" || strings.TrimSpace(request.AssetID) == "" {
		return CaseView{}, fmt.Errorf("%w: provider and asset references are required", ErrValidation)
	}
	if !ValidCode(request.CapabilityCode) {
		return CaseView{}, fmt.Errorf("%w: capability_code is not a valid code", ErrValidation)
	}
	if !ValidCode(request.ProductCode) {
		return CaseView{}, fmt.Errorf("%w: product_code is not a valid code", ErrValidation)
	}

	binding := EvidenceBinding{Authority: request.Authority, ProviderID: request.ProviderID, AssetID: request.AssetID}
	if err := s.evidence.Validate(ctx, binding, request.EvidenceRefs); err != nil {
		return CaseView{}, err
	}

	fingerprint := Fingerprint(map[string]any{
		"authority": request.Authority, "provider_id": request.ProviderID,
		"asset_id": request.AssetID, "capability_code": request.CapabilityCode,
		"product_code": request.ProductCode, "requested_value": request.RequestedValue,
		"evidence_refs": request.EvidenceRefs, "document_refs": request.DocumentRefs,
		"inspection_ref": request.InspectionRef, "policy_version": request.PolicyVersion,
	})

	tx, err := s.repo.Pool().Begin(ctx)
	if err != nil {
		return CaseView{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if err = s.checkSupportingDocuments(ctx, tx, binding, request.DocumentRefs); err != nil {
		return CaseView{}, err
	}
	if err = s.checkInspection(ctx, tx, binding, request.InspectionRef); err != nil {
		return CaseView{}, err
	}

	opened, err := s.repo.CreateCase(ctx, tx, CreateCaseInput{
		Authority: request.Authority, ProviderID: request.ProviderID, AssetID: request.AssetID,
		CapabilityCode: request.CapabilityCode, ProductCode: request.ProductCode,
		RequestedValue: request.RequestedValue, EvidenceRefs: request.EvidenceRefs,
		DocumentRefs: request.DocumentRefs, InspectionRef: request.InspectionRef,
		PolicyVersion: request.PolicyVersion, ExpiresAt: request.ExpiresAt,
		CorrelationID: request.CorrelationID, IdempotencyKey: request.IdempotencyKey,
		Fingerprint: fingerprint,
	})
	if err != nil {
		return CaseView{}, err
	}

	certification, moved, err := s.repo.OpenPendingCertification(ctx, tx, opened)
	if err != nil {
		return CaseView{}, err
	}
	if moved {
		// A capability fact with no consumer endpoint: the three driver
		// ingresses accept eligibility decisions, not "under review". It is
		// still recorded, because an auditor asking when review started needs
		// an answer that is not "we did not write it down".
		if err = s.repo.PublishEvent(ctx, tx, certification.CertificationID,
			EventCapabilityStatusChanged, certification.SourceVersion, "",
			s.capabilityPayload(opened, certification)); err != nil {
			return CaseView{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return CaseView{}, err
	}
	return CaseView{Case: opened, Certification: &certification}, nil
}

// ReadCase returns a case, its latest decision and its certification.
type CaseDetail struct {
	Case           Case             `json:"case"`
	Decisions      []DecisionRecord `json:"decisions"`
	Certifications []Certification  `json:"certifications"`
}

// ReadCase is the authoritative read-back (Section 5.9).
//
// It stays available while the feature flag is off, because read-back is how a
// caller that lost a response discovers what actually happened, and a disabled
// flag must not turn a recorded decision into an unanswerable question.
func (s *Service) ReadCase(ctx context.Context, caller, caseRef string) (CaseDetail, error) {
	record, err := s.repo.GetCase(ctx, caseRef)
	if err != nil {
		return CaseDetail{}, err
	}
	if caller != record.Authority.OwningService() && caller != AdminService {
		return CaseDetail{}, fmt.Errorf("%w: this caller may not read %s cases", ErrForbidden, record.Authority)
	}
	decisions, err := s.repo.ListDecisions(ctx, caseRef)
	if err != nil {
		return CaseDetail{}, err
	}
	certifications, err := s.repo.ListCertifications(ctx, record.Authority, record.AssetID)
	if err != nil {
		return CaseDetail{}, err
	}
	return CaseDetail{Case: record, Decisions: decisions, Certifications: certifications}, nil
}

// ── Decisions ───────────────────────────────────────────────────────────────

// DecideRequest is an administrator's formal conclusion on a case.
type DecideRequest struct {
	Caller          string
	CaseRef         string
	Decision        Decision
	VerifiedValue   any
	ReasonCode      string
	Reviewer        string
	EvidenceRefs    []string
	DocumentRefs    []string
	PolicyVersion   string
	EffectiveAt     *time.Time
	ExpiresAt       *time.Time
	CorrelationID   string
	ExpectedVersion int64
	IdempotencyKey  string
}

// DecisionView is what a decision produced.
type DecisionView struct {
	Case          Case           `json:"case"`
	Decision      DecisionRecord `json:"decision"`
	Certification Certification  `json:"certification"`
}

// Decide records one formal decision and projects it onto the certification.
//
// Case status, the append-only decision, the certification and the outbox row
// that carries it to the owning authority all move in one transaction. Split
// across two, the interesting failure is the one where the certification
// commits and the event does not: the asset would be eligible here and pending
// in the only service that can actually dispatch it, and nothing would ever
// notice the disagreement.
func (s *Service) Decide(ctx context.Context, request DecideRequest) (DecisionView, []byte, error) {
	if err := s.writable(); err != nil {
		return DecisionView{}, nil, err
	}
	if request.Caller != AdminService {
		return DecisionView{}, nil, fmt.Errorf("%w: only %s may record a certification decision",
			ErrForbidden, AdminService)
	}
	if !request.Decision.Valid() {
		return DecisionView{}, nil, fmt.Errorf("%w: %q is not a decision", ErrValidation, request.Decision)
	}
	if strings.TrimSpace(request.Reviewer) == "" {
		return DecisionView{}, nil, fmt.Errorf("%w: a reviewer identity is required", ErrValidation)
	}
	if strings.TrimSpace(request.IdempotencyKey) == "" {
		return DecisionView{}, nil, fmt.Errorf("%w: an idempotency key is required", ErrValidation)
	}
	if request.ExpectedVersion < 1 {
		return DecisionView{}, nil, fmt.Errorf("%w: expected_source_version is required", ErrValidation)
	}
	// A decision that withdraws or refuses eligibility must say why. An
	// unexplained suspension is one an operator cannot act on and a provider
	// cannot appeal.
	if request.Decision != DecisionVerified && strings.TrimSpace(request.ReasonCode) == "" {
		return DecisionView{}, nil, fmt.Errorf("%w: %s requires a reason code", ErrValidation, request.Decision)
	}

	fingerprint := Fingerprint(map[string]any{
		"case_ref": request.CaseRef, "decision": request.Decision,
		"verified_value": request.VerifiedValue, "reason_code": request.ReasonCode,
		"reviewer": request.Reviewer, "evidence_refs": request.EvidenceRefs,
		"document_refs": request.DocumentRefs, "policy_version": request.PolicyVersion,
		"effective_at": request.EffectiveAt, "expires_at": request.ExpiresAt,
		"expected_source_version": request.ExpectedVersion,
	})

	tx, err := s.repo.Pool().Begin(ctx)
	if err != nil {
		return DecisionView{}, nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	cached, found, err := s.repo.LoadCommand(ctx, tx, "asset_capability_decision",
		request.IdempotencyKey, fingerprint)
	if err != nil {
		return DecisionView{}, nil, err
	}
	if found {
		return DecisionView{}, cached, nil
	}

	record, err := s.repo.LockCase(ctx, tx, request.CaseRef)
	if err != nil {
		return DecisionView{}, nil, err
	}
	if record.SourceVersion != request.ExpectedVersion {
		return DecisionView{}, nil, fmt.Errorf("%w: case is at version %d", ErrVersion, record.SourceVersion)
	}

	binding := EvidenceBinding{Authority: record.Authority, ProviderID: record.ProviderID, AssetID: record.AssetID}
	if err = s.evidence.Validate(ctx, binding, request.EvidenceRefs); err != nil {
		return DecisionView{}, nil, err
	}
	if err = s.checkSupportingDocuments(ctx, tx, binding, request.DocumentRefs); err != nil {
		return DecisionView{}, nil, err
	}

	// Admission requires a verified operator. A certified vehicle driven by a
	// provider nobody has verified is a certified vehicle nobody should be
	// dispatching, and this is the only place that can still refuse it.
	if request.Decision == DecisionVerified {
		if err = s.requireVerifiedProvider(ctx, tx, record); err != nil {
			return DecisionView{}, nil, err
		}
	}

	updated, err := s.repo.AdvanceCase(ctx, tx, request.CaseRef, request.ExpectedVersion,
		request.Decision.CaseStatus(), request.ReasonCode, request.ExpiresAt)
	if err != nil {
		return DecisionView{}, nil, err
	}

	decision, err := s.repo.AppendDecision(ctx, tx, DecisionRecord{
		CaseRef: record.CaseRef, DecisionVersion: updated.SourceVersion,
		Decision: request.Decision, VerifiedValue: request.VerifiedValue,
		ReasonCode: request.ReasonCode, Reviewer: request.Reviewer,
		EvidenceRefs: request.EvidenceRefs, DocumentRefs: request.DocumentRefs,
		PolicyVersion: request.PolicyVersion, EffectiveAt: request.EffectiveAt,
		ExpiresAt: request.ExpiresAt, CorrelationID: request.CorrelationID,
	})
	if err != nil {
		return DecisionView{}, nil, err
	}

	status := request.Decision.CertificationStatus()
	policyVersion := request.PolicyVersion
	if policyVersion == "" {
		policyVersion = record.PolicyVersion
	}
	// The decision's own identity is the certification reference. It is stable,
	// already unique, and points a reader straight at the reviewed conclusion
	// the eligibility rests on rather than at a number invented for the purpose.
	certificationRef := ""
	if status == CertEligible {
		certificationRef = decision.DecisionRef
	}
	certification, err := s.repo.ApplyDecision(ctx, tx, ApplyDecisionInput{
		Case: updated, Status: status, CertificationRef: certificationRef,
		PolicyVersion: policyVersion, ReasonCode: request.ReasonCode,
		EffectiveAt: request.EffectiveAt, ExpiresAt: request.ExpiresAt,
		DecisionRef: decision.DecisionRef,
	})
	if err != nil {
		return DecisionView{}, nil, err
	}

	if err = s.repo.PublishEvent(ctx, tx, certification.CertificationID,
		EventServiceTierStatusChanged, certification.SourceVersion, certification.Authority,
		s.certificationPayload(certification, decision, request.VerifiedValue)); err != nil {
		return DecisionView{}, nil, err
	}

	view := DecisionView{Case: updated, Decision: decision, Certification: certification}
	body, err := json.Marshal(map[string]any{"success": true, "data": view})
	if err != nil {
		return DecisionView{}, nil, err
	}
	if err = s.repo.SaveCommand(ctx, tx, "asset_capability_decision",
		request.IdempotencyKey, fingerprint, body); err != nil {
		return DecisionView{}, nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return DecisionView{}, nil, err
	}
	return view, body, nil
}

func (s *Service) requireVerifiedProvider(ctx context.Context, tx pgx.Tx, record Case) error {
	provider, err := s.repo.GetProviderVerification(ctx, tx, record.Authority, record.ProviderID)
	if err != nil {
		if err == ErrNotFound {
			return fmt.Errorf("%w: %s", ErrEvidence, ReasonProviderNotVerified)
		}
		return err
	}
	if !provider.CurrentlyVerified(s.now()) {
		return fmt.Errorf("%w: %s", ErrEvidence, ReasonProviderNotVerified)
	}
	return nil
}

// checkSupportingDocuments verifies that every cited document exists, belongs
// to this asset, and is currently valid.
//
// This is the whole of a document's contribution to certification (Section
// 5.7): a reviewer may lean on it, and a reviewer may not lean on an expired or
// rejected one. What deliberately does not exist anywhere is the inverse --
// there is no rule, table or code path by which holding valid documents
// produces an eligible certification. A roadworthiness certificate says the
// vehicle was roadworthy; only a reviewed decision says it is a Comfort car.
func (s *Service) checkSupportingDocuments(ctx context.Context, tx pgx.Tx,
	binding EvidenceBinding, refs []string) error {
	now := s.now()
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			return fmt.Errorf("%w: %s", ErrEvidence, ReasonDocumentNotFound)
		}
		document, err := s.repo.GetDocument(ctx, tx, ref)
		if err != nil {
			if err == ErrNotFound {
				return fmt.Errorf("%w: %s (%s)", ErrEvidence, ReasonDocumentNotFound, ref)
			}
			return err
		}
		if document.Authority != binding.Authority || document.AssetID != binding.AssetID {
			return fmt.Errorf("%w: %s (%s)", ErrEvidence, ReasonDocumentWrongAsset, ref)
		}
		if !document.CurrentlyValid(now) {
			return fmt.Errorf("%w: %s (%s)", ErrEvidence, ReasonDocumentNotValid, ref)
		}
	}
	return nil
}

func (s *Service) checkInspection(ctx context.Context, tx pgx.Tx, binding EvidenceBinding, ref string) error {
	if strings.TrimSpace(ref) == "" {
		return nil
	}
	inspection, err := s.repo.GetInspection(ctx, tx, ref)
	if err != nil {
		if err == ErrNotFound {
			return fmt.Errorf("%w: %s", ErrEvidence, ReasonInspectionWrongAsset)
		}
		return err
	}
	if inspection.Authority != binding.Authority || inspection.AssetID != binding.AssetID {
		return fmt.Errorf("%w: %s", ErrEvidence, ReasonInspectionWrongAsset)
	}
	return nil
}

// ── Certifications ──────────────────────────────────────────────────────────

// ReadCertifications is the authoritative certification set for one asset
// (Section 5.11).
//
// Consumers are the three driver services and Admin. It stays readable while
// the feature flag is off, so a rollout pause cannot make a
// certification-gated product silently look ungated.
func (s *Service) ReadCertifications(ctx context.Context, caller string,
	authority Authority, assetID string) ([]Certification, error) {
	if caller != authority.OwningService() && caller != AdminService {
		return nil, fmt.Errorf("%w: this caller may not read %s certifications", ErrForbidden, authority)
	}
	if strings.TrimSpace(assetID) == "" {
		return nil, fmt.Errorf("%w: an asset reference is required", ErrValidation)
	}
	return s.repo.ListCertifications(ctx, authority, assetID)
}

// ── Inspections ─────────────────────────────────────────────────────────────

// RecordInspectionRequest is one authority's record of a check it performed.
type RecordInspectionRequest struct {
	Caller         string
	Authority      Authority
	ProviderID     string
	AssetID        string
	InspectionType InspectionType
	Outcome        InspectionOutcome
	EvidenceRefs   []string
	InspectedAt    *time.Time
	ExpiresAt      *time.Time
	InspectorRef   string
	ReasonCode     string
	CorrelationID  string
	IdempotencyKey string
}

// RecordInspection stores an inspection and publishes it as a platform fact.
func (s *Service) RecordInspection(ctx context.Context, request RecordInspectionRequest) (Inspection, error) {
	if err := s.writable(); err != nil {
		return Inspection{}, err
	}
	if request.Caller != request.Authority.OwningService() && request.Caller != AdminService {
		return Inspection{}, fmt.Errorf("%w: this caller may not record %s inspections",
			ErrForbidden, request.Authority)
	}
	if !request.InspectionType.Valid() {
		return Inspection{}, fmt.Errorf("%w: %q is not an inspection type", ErrValidation, request.InspectionType)
	}
	if request.Outcome == "" {
		request.Outcome = InspectionPending
	}
	if !request.Outcome.Valid() {
		return Inspection{}, fmt.Errorf("%w: %q is not an inspection outcome", ErrValidation, request.Outcome)
	}
	if strings.TrimSpace(request.AssetID) == "" {
		return Inspection{}, fmt.Errorf("%w: an asset reference is required", ErrValidation)
	}
	if strings.TrimSpace(request.IdempotencyKey) == "" {
		return Inspection{}, fmt.Errorf("%w: an idempotency key is required", ErrValidation)
	}

	binding := EvidenceBinding{Authority: request.Authority, ProviderID: request.ProviderID, AssetID: request.AssetID}
	if err := s.evidence.Validate(ctx, binding, request.EvidenceRefs); err != nil {
		return Inspection{}, err
	}

	fingerprint := Fingerprint(map[string]any{
		"authority": request.Authority, "provider_id": request.ProviderID,
		"asset_id": request.AssetID, "inspection_type": request.InspectionType,
		"outcome": request.Outcome, "evidence_refs": request.EvidenceRefs,
		"inspected_at": request.InspectedAt, "expires_at": request.ExpiresAt,
		"inspector_ref": request.InspectorRef,
	})

	tx, err := s.repo.Pool().Begin(ctx)
	if err != nil {
		return Inspection{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	inspection, err := s.repo.RecordInspection(ctx, tx, Inspection{
		Authority: request.Authority, ProviderID: request.ProviderID, AssetID: request.AssetID,
		InspectionType: request.InspectionType, Outcome: request.Outcome,
		EvidenceRefs: request.EvidenceRefs, InspectedAt: request.InspectedAt,
		ExpiresAt: request.ExpiresAt, InspectorRef: request.InspectorRef,
		ReasonCode: request.ReasonCode, CorrelationID: request.CorrelationID,
	}, request.IdempotencyKey, fingerprint)
	if err != nil {
		return Inspection{}, err
	}
	if err = s.repo.PublishEvent(ctx, tx, inspection.InspectionRef, EventInspectionStatusChanged,
		inspection.SourceVersion, "", s.inspectionPayload(inspection)); err != nil {
		return Inspection{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Inspection{}, err
	}
	return inspection, nil
}

// ReadInspection returns one inspection.
func (s *Service) ReadInspection(ctx context.Context, caller, inspectionRef string) (Inspection, error) {
	inspection, err := s.repo.GetInspection(ctx, nil, inspectionRef)
	if err != nil {
		return Inspection{}, err
	}
	if caller != inspection.Authority.OwningService() && caller != AdminService {
		return Inspection{}, fmt.Errorf("%w: this caller may not read %s inspections",
			ErrForbidden, inspection.Authority)
	}
	return inspection, nil
}

// ── Vehicle documents ───────────────────────────────────────────────────────

// RecordDocumentRequest registers a document reference against an asset.
type RecordDocumentRequest struct {
	Caller         string
	Authority      Authority
	ProviderID     string
	AssetID        string
	DocumentType   DocumentType
	MediaAssetID   string
	DocumentNumber string
	Status         DocumentStatus
	IssuedAt       *time.Time
	ExpiresAt      *time.Time
	ReasonCode     string
	CorrelationID  string
	IdempotencyKey string
}

// RecordDocument stores a document reference after binding its media object.
//
// The owning authority may register a document as PENDING; only Admin may
// register or later mark one VALID, because declaring a document genuine is a
// review decision and an authority attesting to its own paperwork is not a
// check.
func (s *Service) RecordDocument(ctx context.Context, request RecordDocumentRequest) (VehicleDocument, error) {
	if err := s.writable(); err != nil {
		return VehicleDocument{}, err
	}
	isOwner := request.Caller == request.Authority.OwningService()
	isAdmin := request.Caller == AdminService
	if !isOwner && !isAdmin {
		return VehicleDocument{}, fmt.Errorf("%w: this caller may not register %s documents",
			ErrForbidden, request.Authority)
	}
	if request.Status == "" {
		request.Status = DocumentPending
	}
	if !request.Status.Valid() {
		return VehicleDocument{}, fmt.Errorf("%w: %q is not a document status", ErrValidation, request.Status)
	}
	if request.Status != DocumentPending && !isAdmin {
		return VehicleDocument{}, fmt.Errorf("%w: only %s may register a document as %s",
			ErrForbidden, AdminService, request.Status)
	}
	if !request.DocumentType.Valid() {
		return VehicleDocument{}, fmt.Errorf("%w: %q is not a document type", ErrValidation, request.DocumentType)
	}
	if strings.TrimSpace(request.ProviderID) == "" || strings.TrimSpace(request.AssetID) == "" {
		return VehicleDocument{}, fmt.Errorf("%w: provider and asset references are required", ErrValidation)
	}
	if strings.TrimSpace(request.IdempotencyKey) == "" {
		return VehicleDocument{}, fmt.Errorf("%w: an idempotency key is required", ErrValidation)
	}

	binding := EvidenceBinding{Authority: request.Authority, ProviderID: request.ProviderID, AssetID: request.AssetID}
	if err := s.evidence.Validate(ctx, binding, []string{request.MediaAssetID}); err != nil {
		return VehicleDocument{}, err
	}

	fingerprint := Fingerprint(map[string]any{
		"authority": request.Authority, "provider_id": request.ProviderID,
		"asset_id": request.AssetID, "document_type": request.DocumentType,
		"media_asset_id": request.MediaAssetID, "document_number": request.DocumentNumber,
		"status": request.Status, "issued_at": request.IssuedAt, "expires_at": request.ExpiresAt,
	})

	return s.repo.RecordDocument(ctx, VehicleDocument{
		Authority: request.Authority, ProviderID: request.ProviderID, AssetID: request.AssetID,
		DocumentType: request.DocumentType, MediaAssetID: request.MediaAssetID,
		DocumentNumber: request.DocumentNumber, Status: request.Status,
		IssuedAt: request.IssuedAt, ExpiresAt: request.ExpiresAt,
		ReasonCode: request.ReasonCode, CorrelationID: request.CorrelationID,
	}, request.IdempotencyKey, fingerprint)
}

// SetDocumentStatusRequest moves a document's validity.
type SetDocumentStatusRequest struct {
	Caller          string
	DocumentRef     string
	Status          DocumentStatus
	ReasonCode      string
	ExpectedVersion int64
}

// SetDocumentStatus records an administrator's verdict on a document.
func (s *Service) SetDocumentStatus(ctx context.Context, request SetDocumentStatusRequest) (VehicleDocument, error) {
	if err := s.writable(); err != nil {
		return VehicleDocument{}, err
	}
	if request.Caller != AdminService {
		return VehicleDocument{}, fmt.Errorf("%w: only %s may rule on a document", ErrForbidden, AdminService)
	}
	if !request.Status.Valid() {
		return VehicleDocument{}, fmt.Errorf("%w: %q is not a document status", ErrValidation, request.Status)
	}
	if request.Status != DocumentValid && strings.TrimSpace(request.ReasonCode) == "" {
		return VehicleDocument{}, fmt.Errorf("%w: %s requires a reason code", ErrValidation, request.Status)
	}
	if request.ExpectedVersion < 1 {
		return VehicleDocument{}, fmt.Errorf("%w: expected_source_version is required", ErrValidation)
	}
	return s.repo.SetDocumentStatus(ctx, request.DocumentRef, request.ExpectedVersion,
		request.Status, request.ReasonCode)
}

// ReadDocuments lists the documents held for one asset.
func (s *Service) ReadDocuments(ctx context.Context, caller string,
	authority Authority, assetID string) ([]VehicleDocument, error) {
	if caller != authority.OwningService() && caller != AdminService {
		return nil, fmt.Errorf("%w: this caller may not read %s documents", ErrForbidden, authority)
	}
	return s.repo.ListDocuments(ctx, authority, assetID)
}

// ── Provider verification ───────────────────────────────────────────────────

// ReportProviderRequest records a provider's verification standing.
type ReportProviderRequest struct {
	Caller        string
	Authority     Authority
	ProviderID    string
	Status        ProviderStatus
	ReasonCode    string
	SourceVersion int64
	EffectiveAt   *time.Time
	ExpiresAt     *time.Time
	CorrelationID string
}

// ReportProviderVerification records a provider's standing with one authority.
//
// Only Admin may declare a provider VERIFIED. The owning authority may report
// the states that restrict a provider (suspension, rejection, expiry) because
// those are facts it discovers first and delaying them would leave an
// unfit provider certified.
func (s *Service) ReportProviderVerification(ctx context.Context, request ReportProviderRequest) (ProviderVerification, error) {
	if err := s.writable(); err != nil {
		return ProviderVerification{}, err
	}
	isOwner := request.Caller == request.Authority.OwningService()
	isAdmin := request.Caller == AdminService
	if !isOwner && !isAdmin {
		return ProviderVerification{}, fmt.Errorf("%w: this caller may not report %s providers",
			ErrForbidden, request.Authority)
	}
	if !request.Status.Valid() {
		return ProviderVerification{}, fmt.Errorf("%w: %q is not a provider status", ErrValidation, request.Status)
	}
	if request.Status == ProviderVerified && !isAdmin {
		return ProviderVerification{}, fmt.Errorf("%w: only %s may verify a provider", ErrForbidden, AdminService)
	}
	if strings.TrimSpace(request.ProviderID) == "" {
		return ProviderVerification{}, fmt.Errorf("%w: a provider reference is required", ErrValidation)
	}
	if request.SourceVersion < 1 {
		return ProviderVerification{}, fmt.Errorf("%w: source_version must be positive", ErrValidation)
	}
	return s.repo.UpsertProviderVerification(ctx, ProviderVerification{
		Authority: request.Authority, ProviderID: request.ProviderID, Status: request.Status,
		ReasonCode: request.ReasonCode, SourceVersion: request.SourceVersion,
		EffectiveAt: request.EffectiveAt, ExpiresAt: request.ExpiresAt,
		CorrelationID: request.CorrelationID,
	})
}

// ReadProviderVerification returns a provider's standing with one authority.
func (s *Service) ReadProviderVerification(ctx context.Context, caller string,
	authority Authority, providerID string) (ProviderVerification, error) {
	if caller != authority.OwningService() && caller != AdminService {
		return ProviderVerification{}, fmt.Errorf("%w: this caller may not read %s providers",
			ErrForbidden, authority)
	}
	return s.repo.GetProviderVerification(ctx, nil, authority, providerID)
}

// ── Event payloads ──────────────────────────────────────────────────────────

// These carry no customer data and no money. A certification is a statement
// about a vehicle and a product, and the moment a passenger or a fare appears
// in one, every downstream consumer inherits data it has no reason to hold.

func (s *Service) capabilityPayload(record Case, certification Certification) map[string]any {
	return map[string]any{
		"schema_version":  SchemaVersion,
		"authority":       string(record.Authority),
		"provider_id":     record.ProviderID,
		"asset_id":        record.AssetID,
		"capability_code": record.CapabilityCode,
		"product_code":    record.ProductCode,
		"status":          string(certification.Status),
		"policy_version":  certification.PolicyVersion,
		"source_version":  certification.SourceVersion,
		"reason_code":     certification.ReasonCode,
		"case_ref":        record.CaseRef,
		"occurred_at":     s.now().UTC(),
		"correlation_id":  record.CorrelationID,
	}
}

func (s *Service) certificationPayload(certification Certification, decision DecisionRecord, verified any) map[string]any {
	payload := map[string]any{
		"schema_version":    SchemaVersion,
		"authority":         string(certification.Authority),
		"provider_id":       certification.ProviderID,
		"asset_id":          certification.AssetID,
		"product_code":      certification.ProductCode,
		"capability_code":   certification.CapabilityCode,
		"status":            string(certification.Status),
		"decision":          string(decision.Decision),
		"policy_version":    certification.PolicyVersion,
		"source_version":    certification.SourceVersion,
		"certification_ref": certification.CertificationRef,
		"reason_code":       certification.ReasonCode,
		"case_ref":          certification.CaseRef,
		"occurred_at":       s.now().UTC(),
		"correlation_id":    decision.CorrelationID,
		"causation_id":      decision.DecisionRef,
	}
	if certification.EffectiveAt != nil {
		payload["effective_at"] = certification.EffectiveAt.UTC()
	}
	if certification.ExpiresAt != nil {
		payload["expires_at"] = certification.ExpiresAt.UTC()
	}
	if verified != nil {
		payload["verified_facts"] = verified
	}
	return payload
}

func (s *Service) inspectionPayload(inspection Inspection) map[string]any {
	payload := map[string]any{
		"schema_version":  SchemaVersion,
		"authority":       string(inspection.Authority),
		"provider_id":     inspection.ProviderID,
		"asset_id":        inspection.AssetID,
		"inspection_ref":  inspection.InspectionRef,
		"inspection_type": string(inspection.InspectionType),
		"outcome":         string(inspection.Outcome),
		"source_version":  inspection.SourceVersion,
		"reason_code":     inspection.ReasonCode,
		"occurred_at":     s.now().UTC(),
		"correlation_id":  inspection.CorrelationID,
	}
	if inspection.ExpiresAt != nil {
		payload["expires_at"] = inspection.ExpiresAt.UTC()
	}
	return payload
}

// expiredCertificationPayload is the event an expiry sweep publishes.
//
// An expiry is published as an explicit status change rather than left to the
// consumer's own clock. Two services independently deciding when a
// certification lapsed is two services that will eventually disagree, and the
// one that decides late is the one still dispatching work against it.
func (s *Service) expiredCertificationPayload(certification Certification) map[string]any {
	payload := map[string]any{
		"schema_version":    SchemaVersion,
		"authority":         string(certification.Authority),
		"provider_id":       certification.ProviderID,
		"asset_id":          certification.AssetID,
		"product_code":      certification.ProductCode,
		"capability_code":   certification.CapabilityCode,
		"status":            string(certification.Status),
		"decision":          string(DecisionExpired),
		"policy_version":    certification.PolicyVersion,
		"source_version":    certification.SourceVersion,
		"certification_ref": certification.CertificationRef,
		"reason_code":       certification.ReasonCode,
		"case_ref":          certification.CaseRef,
		"occurred_at":       s.now().UTC(),
	}
	if certification.ExpiresAt != nil {
		payload["expires_at"] = certification.ExpiresAt.UTC()
	}
	return payload
}
