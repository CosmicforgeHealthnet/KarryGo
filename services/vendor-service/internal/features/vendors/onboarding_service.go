package vendors

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"
)

var opaqueMediaRefPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]{7,511}$`)

type RepresentativeInput struct {
	Capacity         string
	FirstName        string
	LastName         string
	Email            string
	Phone            string
	OwnershipPercent *float64
	Authorized       bool
	ExpectedVersion  int64
	StepUpGrantID    string
}

func scanRepresentative(row pgx.Row) (Representative, error) {
	var r Representative
	err := row.Scan(&r.ID, &r.BusinessID, &r.StaffRef, &r.Capacity, &r.FirstName, &r.LastName, &r.Email, &r.Phone, &r.OwnershipPercent, &r.Authorized, &r.Active, &r.CreatedAt, &r.UpdatedAt, &r.Version)
	return r, err
}

const representativeCols = `id,business_id,staff_ref,capacity,first_name,last_name,email,phone,ownership_percent,authorized,active,created_at,updated_at,version`

func (s *Service) GetRepresentative(ctx context.Context, actor Actor) (Representative, error) {
	if err := s.require(ctx, actor, "vendor.onboarding.read", nil); err != nil {
		return Representative{}, err
	}
	r, err := scanRepresentative(s.db.QueryRow(ctx, `SELECT `+representativeCols+` FROM vendor_authorized_representatives WHERE business_id=$1 AND active`, actor.VendorID))
	return r, mapDB(err, "Representative")
}

func validateRepresentative(in RepresentativeInput) error {
	violations := []apperrors.FieldViolation{}
	if in.Capacity != "OWNER" && in.Capacity != "DIRECTOR" && in.Capacity != "AUTHORIZED_SIGNATORY" {
		violations = append(violations, apperrors.FieldViolation{Field: "capacity", Message: "Capacity must be OWNER, DIRECTOR or AUTHORIZED_SIGNATORY."})
	}
	if len(strings.TrimSpace(in.FirstName)) < 1 || len(strings.TrimSpace(in.FirstName)) > 100 {
		violations = append(violations, apperrors.FieldViolation{Field: "first_name", Message: "First name is required and cannot exceed 100 characters."})
	}
	if len(strings.TrimSpace(in.LastName)) < 1 || len(strings.TrimSpace(in.LastName)) > 100 {
		violations = append(violations, apperrors.FieldViolation{Field: "last_name", Message: "Last name is required and cannot exceed 100 characters."})
	}
	if _, err := mail.ParseAddress(strings.TrimSpace(in.Email)); err != nil {
		violations = append(violations, apperrors.FieldViolation{Field: "email", Message: "Email is invalid."})
	}
	if len(strings.TrimSpace(in.Phone)) < 7 || len(strings.TrimSpace(in.Phone)) > 32 {
		violations = append(violations, apperrors.FieldViolation{Field: "phone", Message: "Phone is invalid."})
	}
	if in.OwnershipPercent != nil && (*in.OwnershipPercent < 0 || *in.OwnershipPercent > 100) {
		violations = append(violations, apperrors.FieldViolation{Field: "ownership_percent", Message: "Ownership percent must be from 0 to 100."})
	}
	if len(violations) > 0 {
		return validation(violations...)
	}
	return nil
}

func (s *Service) PutRepresentative(ctx context.Context, actor Actor, in RepresentativeInput) (Representative, error) {
	if err := s.require(ctx, actor, "vendor.onboarding.manage", nil); err != nil {
		return Representative{}, err
	}
	if err := validateRepresentative(in); err != nil {
		return Representative{}, err
	}
	var current Representative
	current, currentErr := scanRepresentative(s.db.QueryRow(ctx, `SELECT `+representativeCols+` FROM vendor_authorized_representatives WHERE business_id=$1 AND active`, actor.VendorID))
	if currentErr != nil && !errors.Is(currentErr, pgx.ErrNoRows) {
		return Representative{}, currentErr
	}
	if currentErr == nil {
		var identityVerified bool
		_ = s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM vendor_verification_projections WHERE business_id=$1 AND dimension='IDENTITY' AND status='VERIFIED' AND (expires_at IS NULL OR expires_at>now()))`, actor.VendorID).Scan(&identityVerified)
		if identityVerified {
			if s.stepUp == nil || s.stepUp.ConsumeGrant(ctx, in.StepUpGrantID, actor.SessionID, actor.StaffID, actor.VendorID, "onboarding.representative_replace") != nil {
				return Representative{}, apperrors.Forbidden("Step-up authentication is required to replace a verified representative.", nil)
			}
		}
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Representative{}, err
	}
	defer tx.Rollback(ctx)
	var out Representative
	if errors.Is(currentErr, pgx.ErrNoRows) {
		if in.ExpectedVersion != 0 {
			return Representative{}, conflict(nil)
		}
		out, err = scanRepresentative(tx.QueryRow(ctx, `INSERT INTO vendor_authorized_representatives(id,business_id,staff_ref,capacity,first_name,last_name,email,phone,ownership_percent,authorized) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING `+representativeCols,
			newID("vr_"), actor.VendorID, actor.StaffID, in.Capacity, strings.TrimSpace(in.FirstName), strings.TrimSpace(in.LastName), strings.ToLower(strings.TrimSpace(in.Email)), strings.TrimSpace(in.Phone), in.OwnershipPercent, in.Authorized))
	} else {
		out, err = scanRepresentative(tx.QueryRow(ctx, `UPDATE vendor_authorized_representatives SET capacity=$1,first_name=$2,last_name=$3,email=$4,phone=$5,ownership_percent=$6,authorized=$7,updated_at=now(),version=version+1 WHERE business_id=$8 AND id=$9 AND version=$10 RETURNING `+representativeCols,
			in.Capacity, strings.TrimSpace(in.FirstName), strings.TrimSpace(in.LastName), strings.ToLower(strings.TrimSpace(in.Email)), strings.TrimSpace(in.Phone), in.OwnershipPercent, in.Authorized, actor.VendorID, current.ID, in.ExpectedVersion))
		if errors.Is(err, pgx.ErrNoRows) {
			return Representative{}, conflict(err)
		}
	}
	if err != nil {
		return Representative{}, mapDB(err, "Representative")
	}
	if currentErr == nil {
		if err = s.invalidateProjectionTx(ctx, tx, actor.VendorID, DimensionIdentity, "REPRESENTATIVE_CHANGED", actor); err != nil {
			return Representative{}, err
		}
	}
	if err = s.publish(ctx, tx, "vendor.onboarding.representative_changed", "vendor_representative", out.ID, out.Version, actor, safeRepresentative(out)); err != nil {
		return Representative{}, err
	}
	if err = s.auditTx(ctx, tx, actor, "vendor.onboarding.representative_changed", "vendor_representative", out.ID, "", safeRepresentativePtr(currentErr, current), safeRepresentative(out), ""); err != nil {
		return Representative{}, err
	}
	if err = s.ensureOnboardingTx(ctx, tx, actor.VendorID); err != nil {
		return Representative{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Representative{}, err
	}
	return out, nil
}

func safeRepresentativePtr(err error, r Representative) any {
	if err != nil {
		return nil
	}
	return safeRepresentative(r)
}
func safeRepresentative(r Representative) map[string]any {
	return map[string]any{"id": r.ID, "business_id": r.BusinessID, "capacity": r.Capacity, "authorized": r.Authorized, "active": r.Active, "version": r.Version}
}

func scanDocument(row pgx.Row) (OnboardingDocument, error) {
	var d OnboardingDocument
	err := row.Scan(&d.ID, &d.BusinessID, &d.RepresentativeID, &d.BranchID, &d.BrandID, &d.Dimension, &d.MediaRef, &d.DocumentType, &d.Status, &d.CreatedBy, &d.CreatedAt, &d.UpdatedAt, &d.Version)
	return d, err
}

const documentCols = `id,business_id,COALESCE(representative_id,''),COALESCE(branch_id,''),COALESCE(brand_id,''),dimension,media_ref,document_type,status,created_by,created_at,updated_at,version`

func (s *Service) ListDocuments(ctx context.Context, actor Actor) ([]OnboardingDocument, error) {
	if err := s.require(ctx, actor, "vendor.onboarding.read", nil); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `SELECT `+documentCols+` FROM vendor_onboarding_documents WHERE business_id=$1 ORDER BY created_at,id`, actor.VendorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []OnboardingDocument{}
	for rows.Next() {
		d, e := scanDocument(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

type DocumentInput struct {
	RepresentativeID, BranchID, BrandID string
	Dimension                           VerificationDimension
	MediaRef, DocumentType              string
	ExpectedVersion                     int64
}

func validateDocument(in DocumentInput) error {
	v := []apperrors.FieldViolation{}
	if !validDimension(in.Dimension) {
		v = append(v, apperrors.FieldViolation{Field: "dimension", Message: "Unsupported verification dimension."})
	}
	if !opaqueMediaRefPattern.MatchString(strings.TrimSpace(in.MediaRef)) {
		v = append(v, apperrors.FieldViolation{Field: "media_ref", Message: "Media reference format is invalid."})
	}
	if len(strings.TrimSpace(in.DocumentType)) < 2 || len(strings.TrimSpace(in.DocumentType)) > 80 {
		v = append(v, apperrors.FieldViolation{Field: "document_type", Message: "Document type must contain 2 to 80 characters."})
	}
	if len(v) > 0 {
		return validation(v...)
	}
	return nil
}

func (s *Service) CreateDocument(ctx context.Context, actor Actor, in DocumentInput) (OnboardingDocument, error) {
	if err := s.require(ctx, actor, "vendor.onboarding.manage", branchPtr(in.BranchID)); err != nil {
		return OnboardingDocument{}, err
	}
	if err := validateDocument(in); err != nil {
		return OnboardingDocument{}, err
	}
	if in.ExpectedVersion != 0 {
		return OnboardingDocument{}, conflict(nil)
	}
	if err := s.validateMediaReference(ctx, actor.VendorID, in.MediaRef); err != nil {
		return OnboardingDocument{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return OnboardingDocument{}, err
	}
	defer tx.Rollback(ctx)
	d, err := scanDocument(tx.QueryRow(ctx, `INSERT INTO vendor_onboarding_documents(id,business_id,representative_id,branch_id,brand_id,dimension,media_ref,document_type,created_by) VALUES($1,$2,NULLIF($3,''),NULLIF($4,''),NULLIF($5,''),$6,$7,$8,$9) RETURNING `+documentCols, newID("vdoc_"), actor.VendorID, in.RepresentativeID, in.BranchID, in.BrandID, in.Dimension, strings.TrimSpace(in.MediaRef), strings.TrimSpace(in.DocumentType), actor.StaffID))
	if err != nil {
		return OnboardingDocument{}, mapDB(err, "Document reference")
	}
	if err = s.publish(ctx, tx, "vendor.onboarding.document_attached", "vendor_onboarding_document", d.ID, d.Version, actor, map[string]any{"id": d.ID, "dimension": d.Dimension}); err != nil {
		return OnboardingDocument{}, err
	}
	if err = s.auditTx(ctx, tx, actor, "vendor.onboarding.document_attached", "vendor_onboarding_document", d.ID, in.BranchID, nil, map[string]any{"id": d.ID, "dimension": d.Dimension}, ""); err != nil {
		return OnboardingDocument{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return OnboardingDocument{}, err
	}
	return d, nil
}

func (s *Service) UpdateDocument(ctx context.Context, actor Actor, id string, in DocumentInput) (OnboardingDocument, error) {
	if err := s.require(ctx, actor, "vendor.onboarding.manage", branchPtr(in.BranchID)); err != nil {
		return OnboardingDocument{}, err
	}
	if err := validateDocument(in); err != nil {
		return OnboardingDocument{}, err
	}
	if err := s.validateMediaReference(ctx, actor.VendorID, in.MediaRef); err != nil {
		return OnboardingDocument{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return OnboardingDocument{}, err
	}
	defer tx.Rollback(ctx)
	d, err := scanDocument(tx.QueryRow(ctx, `UPDATE vendor_onboarding_documents SET representative_id=NULLIF($1,''),branch_id=NULLIF($2,''),brand_id=NULLIF($3,''),dimension=$4,media_ref=$5,document_type=$6,updated_at=now(),version=version+1 WHERE id=$7 AND business_id=$8 AND status='DRAFT' AND version=$9 RETURNING `+documentCols, in.RepresentativeID, in.BranchID, in.BrandID, in.Dimension, strings.TrimSpace(in.MediaRef), strings.TrimSpace(in.DocumentType), id, actor.VendorID, in.ExpectedVersion))
	if errors.Is(err, pgx.ErrNoRows) {
		return OnboardingDocument{}, conflict(err)
	}
	if err != nil {
		return OnboardingDocument{}, err
	}
	if err = s.auditTx(ctx, tx, actor, "vendor.onboarding.document_updated", "vendor_onboarding_document", id, in.BranchID, nil, map[string]any{"id": d.ID, "dimension": d.Dimension}, ""); err != nil {
		return OnboardingDocument{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return OnboardingDocument{}, err
	}
	return d, nil
}

func (s *Service) DeleteDocument(ctx context.Context, actor Actor, id string, expected int64) error {
	if err := s.require(ctx, actor, "vendor.onboarding.manage", nil); err != nil {
		return err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `DELETE FROM vendor_onboarding_documents WHERE id=$1 AND business_id=$2 AND status='DRAFT' AND version=$3`, id, actor.VendorID, expected)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return conflict(nil)
	}
	if err = s.auditTx(ctx, tx, actor, "vendor.onboarding.document_removed", "vendor_onboarding_document", id, "", map[string]any{"id": id}, nil, ""); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func branchPtr(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func (s *Service) validateMediaReference(ctx context.Context, businessID, mediaRef string) error {
	if s.media == nil {
		return apperrors.Unavailable("Media ownership validation is not configured.", nil)
	}
	object, err := s.media.GetObject(ctx, businessID, mediaRef)
	if err != nil {
		return apperrors.Unavailable("Media reference could not be validated.", err)
	}
	if object.ID != mediaRef || object.UploadStatus != "COMPLETED" || object.DeletionState != "ACTIVE" || object.SafetyStatus == "UNSAFE" || object.SafetyStatus == "QUARANTINED" {
		return validation(apperrors.FieldViolation{Field: "media_ref", Message: "Media object is not an active completed object owned by this business."})
	}
	return nil
}

func scanProjection(row pgx.Row) (VerificationProjection, error) {
	var p VerificationProjection
	err := row.Scan(&p.ID, &p.BusinessID, &p.Dimension, &p.SubjectType, &p.SubjectRef, &p.SourceService, &p.SourceRef, &p.SourceVersion, &p.Status, &p.ObservedAt, &p.ExpiresAt, &p.ReasonCode, &p.Remediation, &p.UpdatedAt, &p.Version)
	if err == nil && p.ExpiresAt != nil && !p.ExpiresAt.After(time.Now()) && p.Status == VerificationVerified {
		p.Status = VerificationExpired
	}
	return p, err
}

const projectionCols = `id,business_id,dimension,subject_type,subject_ref,source_service,source_ref,source_version,status,observed_at,expires_at,COALESCE(reason_code,''),COALESCE(remediation,''),updated_at,version`

func (s *Service) ListVerifications(ctx context.Context, actor Actor) ([]VerificationProjection, error) {
	if err := s.require(ctx, actor, "vendor.verification.read", nil); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `SELECT `+projectionCols+` FROM vendor_verification_projections WHERE business_id=$1 ORDER BY dimension,subject_ref`, actor.VendorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []VerificationProjection{}
	for rows.Next() {
		p, e := scanProjection(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
func (s *Service) GetVerification(ctx context.Context, actor Actor, dimension VerificationDimension) ([]VerificationProjection, error) {
	if !validDimension(dimension) {
		return nil, validation(apperrors.FieldViolation{Field: "dimension", Message: "Unsupported verification dimension."})
	}
	all, err := s.ListVerifications(ctx, actor)
	if err != nil {
		return nil, err
	}
	out := []VerificationProjection{}
	for _, p := range all {
		if p.Dimension == dimension {
			out = append(out, p)
		}
	}
	return out, nil
}

type SubmitVerificationInput struct {
	Dimension               VerificationDimension
	SubjectType, SubjectRef string
	DocumentRefs            []string
}

func (s *Service) SubmitVerification(ctx context.Context, actor Actor, in SubmitVerificationInput, idempotencyKey string) (VerificationProjection, error) {
	if !validDimension(in.Dimension) {
		return VerificationProjection{}, validation(apperrors.FieldViolation{Field: "dimension", Message: "Unsupported verification dimension."})
	}
	if !validSubject(in.SubjectType) || strings.TrimSpace(in.SubjectRef) == "" {
		return VerificationProjection{}, validation(apperrors.FieldViolation{Field: "subject", Message: "Subject type or reference is invalid."})
	}
	if err := s.assertSubjectScope(ctx, actor.VendorID, in.SubjectType, in.SubjectRef); err != nil {
		return VerificationProjection{}, err
	}
	branchScope, err := s.verificationSubjectBranch(ctx, actor.VendorID, in.SubjectType, in.SubjectRef)
	if err != nil {
		return VerificationProjection{}, err
	}
	if err = s.require(ctx, actor, "vendor.verification.submit", branchPtr(branchScope)); err != nil {
		return VerificationProjection{}, err
	}
	if len(in.DocumentRefs) == 0 {
		return VerificationProjection{}, validation(apperrors.FieldViolation{Field: "document_refs", Message: "At least one onboarding document reference is required."})
	}
	if s.verification == nil {
		return VerificationProjection{}, apperrors.Unavailable("Verification authority is not configured; no submission was recorded.", nil)
	}
	for _, documentID := range in.DocumentRefs {
		var valid bool
		if err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM vendor_onboarding_documents WHERE id=$1 AND business_id=$2 AND dimension=$3 AND status<>'SUPERSEDED')`, documentID, actor.VendorID, in.Dimension).Scan(&valid); err != nil {
			return VerificationProjection{}, err
		}
		if !valid {
			return VerificationProjection{}, apperrors.NotFound("Onboarding document not found.", nil)
		}
	}
	clientRef := "vcase_" + uuid.NewString()
	raw, _ := json.Marshal(in)
	sum := sha256.Sum256(raw)
	fingerprint := hex.EncodeToString(sum[:])
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return VerificationProjection{}, err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `INSERT INTO vendor_verification_operations(id,business_id,dimension,subject_type,subject_ref,client_reference,request_fingerprint,document_refs) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, newID("vop_"), actor.VendorID, in.Dimension, in.SubjectType, in.SubjectRef, clientRef, fingerprint, rawStringSlice(in.DocumentRefs))
	if err != nil {
		return VerificationProjection{}, err
	}
	_, err = tx.Exec(ctx, `UPDATE vendor_onboarding_documents SET status='SUBMITTED',updated_at=now(),version=version+1 WHERE business_id=$1 AND id=ANY($2::text[]) AND status<>'SUPERSEDED'`, actor.VendorID, in.DocumentRefs)
	if err != nil {
		return VerificationProjection{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return VerificationProjection{}, err
	}
	result, callErr := s.verification.CreateCase(ctx, VerificationCaseInput{ClientReference: clientRef, BusinessID: actor.VendorID, Dimension: in.Dimension, SubjectType: in.SubjectType, SubjectRef: in.SubjectRef, DocumentRefs: in.DocumentRefs})
	if callErr != nil {
		_, _ = s.db.Exec(ctx, `UPDATE vendor_verification_operations SET status='UNKNOWN',attempts=1,next_attempt_at=now()+interval '30 seconds',last_error='VERIFICATION_SOURCE_UNAVAILABLE',updated_at=now(),version=version+1 WHERE client_reference=$1`, clientRef)
		return VerificationProjection{}, apperrors.Unavailable("Verification submission outcome is unknown and will be reconciled.", callErr)
	}
	p, err := s.applyProjection(ctx, actor, "verification-compliance-service", result, in.Dimension, in.SubjectType, in.SubjectRef, "")
	if err == nil {
		operationStatus := "PENDING"
		if terminalVerificationStatus(result.Status) {
			operationStatus = "COMPLETED"
		}
		_, _ = s.db.Exec(ctx, `UPDATE vendor_verification_operations SET status=$2,source_ref=$3,attempts=CASE WHEN $2='PENDING' THEN 0 ELSE 1 END,next_attempt_at=CASE WHEN $2='PENDING' THEN now()+interval '1 minute' ELSE next_attempt_at END,updated_at=now(),version=version+1 WHERE client_reference=$1`, clientRef, operationStatus, result.SourceRef)
	}
	return p, err
}

func terminalVerificationStatus(status VerificationState) bool {
	switch status {
	case VerificationVerified, VerificationRejected, VerificationExpired, VerificationSuspended:
		return true
	}
	return false
}

func rawStringSlice(v []string) []byte { b, _ := json.Marshal(v); return b }
func validSubject(v string) bool {
	switch v {
	case "BUSINESS", "REPRESENTATIVE", "BRANCH", "BRAND", "CAPABILITY", "BRANCH_CAPABILITY":
		return true
	}
	return false
}
func (s *Service) verificationSubjectBranch(ctx context.Context, businessID, t, ref string) (string, error) {
	if t == "BRANCH" {
		return ref, nil
	}
	if t != "BRANCH_CAPABILITY" {
		return "", nil
	}
	var branchID string
	err := s.db.QueryRow(ctx, `SELECT branch_id FROM vendor_branch_capabilities WHERE id=$1 AND business_id=$2`, ref, businessID).Scan(&branchID)
	if err != nil {
		return "", mapDB(err, "Branch capability")
	}
	return branchID, nil
}

func (s *Service) assertSubjectScope(ctx context.Context, businessID, typ, ref string) error {
	var ok bool
	switch typ {
	case "BUSINESS":
		ok = ref == businessID
	case "REPRESENTATIVE":
		_ = s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM vendor_authorized_representatives WHERE id=$1 AND business_id=$2 AND active)`, ref, businessID).Scan(&ok)
	case "BRANCH":
		_ = s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM vendor_branches WHERE id=$1 AND business_id=$2)`, ref, businessID).Scan(&ok)
	case "BRAND":
		_ = s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM vendor_brands WHERE id=$1 AND business_id=$2)`, ref, businessID).Scan(&ok)
	case "CAPABILITY":
		_ = s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM vendor_capabilities WHERE id=$1 AND business_id=$2)`, ref, businessID).Scan(&ok)
	case "BRANCH_CAPABILITY":
		_ = s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM vendor_branch_capabilities WHERE id=$1 AND business_id=$2)`, ref, businessID).Scan(&ok)
	}
	if !ok {
		return apperrors.NotFound("Verification subject not found.", nil)
	}
	return nil
}

func (s *Service) applyProjection(ctx context.Context, actor Actor, source string, result VerificationCaseResult, dimension VerificationDimension, subjectType, subjectRef, eventID string) (VerificationProjection, error) {
	if result.SourceRef == "" || result.SourceVersion < 1 || result.ObservedAt.IsZero() {
		return VerificationProjection{}, apperrors.BadRequest("Authoritative verification result is missing source metadata.", nil)
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return VerificationProjection{}, err
	}
	defer tx.Rollback(ctx)
	if eventID != "" {
		tag, e := tx.Exec(ctx, `INSERT INTO vendor_inbox_processed_events(source_service,event_id,event_type,processed_at) VALUES($1,$2,'vendor.verification.status_changed',now()) ON CONFLICT DO NOTHING`, source, eventID)
		if e != nil {
			return VerificationProjection{}, e
		}
		if tag.RowsAffected() == 0 {
			s.metrics.Inc("verification_callback_duplicate_total")
			p, e := scanProjection(tx.QueryRow(ctx, `SELECT `+projectionCols+` FROM vendor_verification_projections WHERE source_service=$1 AND source_ref=$2`, source, result.SourceRef))
			return p, e
		}
	}
	var existingVersion int64
	var existingRef string
	// Serialize the first insert as well as updates for this subject.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, actor.VendorID+":"+string(dimension)+":"+subjectType+":"+subjectRef); err != nil {
		return VerificationProjection{}, err
	}
	var existingObserved time.Time
	e := tx.QueryRow(ctx, `SELECT source_version,source_ref,observed_at FROM vendor_verification_projections WHERE business_id=$1 AND dimension=$2 AND subject_type=$3 AND subject_ref=$4 FOR UPDATE`, actor.VendorID, dimension, subjectType, subjectRef).Scan(&existingVersion, &existingRef, &existingObserved)
	if e != nil && !errors.Is(e, pgx.ErrNoRows) {
		return VerificationProjection{}, e
	}
	if e == nil && existingRef == result.SourceRef && result.SourceVersion < existingVersion {
		s.metrics.Inc("verification_stale_source_rejected_total", "dimension", string(dimension))
		return VerificationProjection{}, apperrors.Conflict("Stale verification source version cannot overwrite the current projection.", nil)
	}
	if e == nil && existingRef != result.SourceRef && !result.ObservedAt.After(existingObserved) {
		return VerificationProjection{}, apperrors.Conflict("An older verification case cannot replace the current projection.", nil)
	}
	if e == nil && existingRef == result.SourceRef && result.SourceVersion == existingVersion {
		// Only a successful authority read-back renews freshness. Replayed
		// callbacks must not keep a lost/superseded decision publicly valid.
		if eventID == "" {
			if _, err = tx.Exec(ctx, `UPDATE vendor_verification_projections SET updated_at=now() WHERE business_id=$1 AND dimension=$2 AND subject_type=$3 AND subject_ref=$4`, actor.VendorID, dimension, subjectType, subjectRef); err != nil {
				return VerificationProjection{}, err
			}
		}
		p, scanErr := scanProjection(tx.QueryRow(ctx, `SELECT `+projectionCols+` FROM vendor_verification_projections WHERE business_id=$1 AND dimension=$2 AND subject_type=$3 AND subject_ref=$4`, actor.VendorID, dimension, subjectType, subjectRef))
		if scanErr != nil {
			return VerificationProjection{}, scanErr
		}
		if commitErr := tx.Commit(ctx); commitErr != nil {
			return VerificationProjection{}, commitErr
		}
		return p, nil
	}
	var p VerificationProjection
	if errors.Is(e, pgx.ErrNoRows) {
		p, err = scanProjection(tx.QueryRow(ctx, `INSERT INTO vendor_verification_projections(id,business_id,dimension,subject_type,subject_ref,source_service,source_ref,source_version,status,observed_at,expires_at,reason_code,remediation) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,NULLIF($12,''),NULLIF($13,'')) RETURNING `+projectionCols, newID("vvp_"), actor.VendorID, dimension, subjectType, subjectRef, source, result.SourceRef, result.SourceVersion, result.Status, result.ObservedAt, result.ExpiresAt, result.ReasonCode, result.Remediation))
	} else {
		p, err = scanProjection(tx.QueryRow(ctx, `UPDATE vendor_verification_projections SET source_service=$1,source_ref=$2,source_version=$3,status=$4,observed_at=$5,expires_at=$6,reason_code=NULLIF($7,''),remediation=NULLIF($8,''),updated_at=now(),version=version+1 WHERE business_id=$9 AND dimension=$10 AND subject_type=$11 AND subject_ref=$12 RETURNING `+projectionCols, source, result.SourceRef, result.SourceVersion, result.Status, result.ObservedAt, result.ExpiresAt, result.ReasonCode, result.Remediation, actor.VendorID, dimension, subjectType, subjectRef))
	}
	if err != nil {
		return VerificationProjection{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO vendor_verification_history(id,projection_id,business_id,source_service,source_ref,source_version,status,observed_at,expires_at,reason_code,remediation) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,NULLIF($10,''),NULLIF($11,''))`, newID("vvh_"), p.ID, actor.VendorID, source, result.SourceRef, result.SourceVersion, result.Status, result.ObservedAt, result.ExpiresAt, result.ReasonCode, result.Remediation)
	if err != nil {
		return VerificationProjection{}, err
	}
	if err = s.publish(ctx, tx, "vendor.verification.projection_changed", "vendor_verification_projection", p.ID, p.Version, actor, p); err != nil {
		return VerificationProjection{}, err
	}
	if err = s.auditTx(ctx, tx, actor, "vendor.verification.projection_changed", "vendor_verification_projection", p.ID, "", nil, map[string]any{"dimension": p.Dimension, "status": p.Status, "source_ref": p.SourceRef, "source_version": p.SourceVersion}, p.ReasonCode); err != nil {
		return VerificationProjection{}, err
	}
	if err = s.evaluateActivationTx(ctx, tx, actor); err != nil {
		return VerificationProjection{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return VerificationProjection{}, err
	}
	s.metrics.Inc("verification_status_transition_total", "dimension", string(dimension), "status", string(result.Status))
	return p, nil
}

func (s *Service) ApplyVerificationEvent(ctx context.Context, source, eventID, businessID string, result VerificationCaseResult, dimension VerificationDimension, subjectType, subjectRef string) (VerificationProjection, error) {
	if source != "verification-compliance-service" {
		return VerificationProjection{}, apperrors.Forbidden("Caller is not allowed to update verification projections.", nil)
	}
	if err := s.assertSubjectScope(ctx, businessID, subjectType, subjectRef); err != nil {
		return VerificationProjection{}, err
	}
	return s.applyProjection(ctx, Actor{VendorID: businessID, StaffID: "verification-compliance-service"}, source, result, dimension, subjectType, subjectRef, eventID)
}

func (s *Service) Readiness(ctx context.Context, actor Actor) (ActivationReadiness, error) {
	if err := s.require(ctx, actor, "vendor.onboarding.read", nil); err != nil {
		return ActivationReadiness{}, err
	}
	result, err := s.readiness(ctx, s.db, actor.VendorID)
	if err == nil {
		s.metrics.Inc("activation_evaluation_total", "ready", fmt.Sprintf("%t", result.Ready))
		for _, blocker := range result.Blockers {
			dimension := string(blocker.Dimension)
			if dimension == "" {
				dimension = "LOCAL"
			}
			s.metrics.Inc("activation_blocked_total", "dimension", dimension)
		}
	}
	return result, err
}

type queryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func (s *Service) readiness(ctx context.Context, q queryer, businessID string) (ActivationReadiness, error) {
	reqs := []RequirementResult{}
	type definition struct{ code, scope, capability, dimension, local string }
	rows, err := q.Query(ctx, `SELECT requirement_code,scope,COALESCE(capability_type,''),COALESCE(dimension,''),COALESCE(local_check,'') FROM vendor_compliance_requirements WHERE active AND mandatory ORDER BY requirement_code`)
	if err != nil {
		return ActivationReadiness{}, err
	}
	definitions := []definition{}
	for rows.Next() {
		var d definition
		if e := rows.Scan(&d.code, &d.scope, &d.capability, &d.dimension, &d.local); e != nil {
			rows.Close()
			return ActivationReadiness{}, e
		}
		definitions = append(definitions, d)
	}
	rows.Close()
	localSatisfied := func(check string) (bool, error) {
		var ok bool
		switch check {
		case "BUSINESS_BASICS":
			err = q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM vendor_businesses WHERE id=$1 AND legal_name<>'' AND currency<>'' AND timezone<>'')`, businessID).Scan(&ok)
		case "AUTHORIZED_REPRESENTATIVE":
			err = q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM vendor_authorized_representatives WHERE business_id=$1 AND active AND authorized)`, businessID).Scan(&ok)
		case "FIRST_BRANCH":
			err = q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM vendor_branches WHERE business_id=$1 AND status<>'CLOSED')`, businessID).Scan(&ok)
		default:
			return false, fmt.Errorf("unsupported compliance local check %s", check)
		}
		return ok, err
	}
	projectionStatus := func(dimension, subjectType, subjectRef string) (string, error) {
		status := "NOT_STARTED"
		e := q.QueryRow(ctx, `SELECT CASE WHEN expires_at IS NOT NULL AND expires_at<=now() AND status='VERIFIED' THEN 'EXPIRED' ELSE status END FROM vendor_verification_projections WHERE business_id=$1 AND dimension=$2 AND ($3='' OR subject_type=$3) AND ($4='' OR subject_ref=$4) ORDER BY observed_at DESC,source_version DESC LIMIT 1`, businessID, dimension, subjectType, subjectRef).Scan(&status)
		if errors.Is(e, pgx.ErrNoRows) {
			return "NOT_STARTED", nil
		}
		return status, e
	}
	appendProjection := func(d definition, subjectType, subjectRef string, cap CapabilityType) error {
		status, e := projectionStatus(d.dimension, subjectType, subjectRef)
		if e != nil {
			return e
		}
		r := RequirementResult{Code: d.code, Scope: d.scope, Dimension: VerificationDimension(d.dimension), CapabilityType: cap, SubjectRef: subjectRef, Satisfied: status == "VERIFIED", Status: status}
		if !r.Satisfied {
			r.Blocker = d.dimension + "_" + status
		}
		reqs = append(reqs, r)
		return nil
	}
	for _, d := range definitions {
		if d.local != "" {
			ok, e := localSatisfied(d.local)
			if e != nil {
				return ActivationReadiness{}, e
			}
			r := RequirementResult{Code: d.code, Scope: d.scope, Satisfied: ok, Status: "SATISFIED"}
			if !ok {
				r.Status = "MISSING"
				r.Blocker = d.code + "_REQUIRED"
			}
			reqs = append(reqs, r)
			continue
		}
		switch d.scope {
		case "BUSINESS":
			subjectType := "BUSINESS"
			subjectRef := businessID
			if d.dimension == "IDENTITY" {
				subjectType = "REPRESENTATIVE"
				subjectRef = ""
			}
			if e := appendProjection(d, subjectType, subjectRef, ""); e != nil {
				return ActivationReadiness{}, e
			}
		case "BRANCH":
			items, e := q.Query(ctx, `SELECT id FROM vendor_branches WHERE business_id=$1 AND status<>'CLOSED' ORDER BY id`, businessID)
			if e != nil {
				return ActivationReadiness{}, e
			}
			ids := []string{}
			for items.Next() {
				var id string
				if e = items.Scan(&id); e != nil {
					items.Close()
					return ActivationReadiness{}, e
				}
				ids = append(ids, id)
			}
			items.Close()
			for _, id := range ids {
				if e = appendProjection(d, "BRANCH", id, ""); e != nil {
					return ActivationReadiness{}, e
				}
			}
		case "BRANCH_CAPABILITY":
			items, e := q.Query(ctx, `SELECT id,capability_type FROM vendor_branch_capabilities WHERE business_id=$1 AND ($2='' OR capability_type=$2) ORDER BY id`, businessID, d.capability)
			if e != nil {
				return ActivationReadiness{}, e
			}
			type scopedCapability struct {
				id  string
				cap CapabilityType
			}
			capabilities := []scopedCapability{}
			for items.Next() {
				var id string
				var cap CapabilityType
				if e = items.Scan(&id, &cap); e != nil {
					items.Close()
					return ActivationReadiness{}, e
				}
				capabilities = append(capabilities, scopedCapability{id: id, cap: cap})
			}
			items.Close()
			for _, item := range capabilities {
				if e = appendProjection(d, "BRANCH_CAPABILITY", item.id, item.cap); e != nil {
					return ActivationReadiness{}, e
				}
			}
		case "CAPABILITY":
			items, e := q.Query(ctx, `SELECT id,capability_type FROM vendor_capabilities WHERE business_id=$1 AND ($2='' OR capability_type=$2) ORDER BY id`, businessID, d.capability)
			if e != nil {
				return ActivationReadiness{}, e
			}
			type scopedCapability struct {
				id  string
				cap CapabilityType
			}
			capabilities := []scopedCapability{}
			for items.Next() {
				var id string
				var cap CapabilityType
				if e = items.Scan(&id, &cap); e != nil {
					items.Close()
					return ActivationReadiness{}, e
				}
				capabilities = append(capabilities, scopedCapability{id: id, cap: cap})
			}
			items.Close()
			for _, item := range capabilities {
				if e = appendProjection(d, "CAPABILITY", item.id, item.cap); e != nil {
					return ActivationReadiness{}, e
				}
			}
		case "BRAND":
			items, e := q.Query(ctx, `SELECT id FROM vendor_brands WHERE business_id=$1 ORDER BY id`, businessID)
			if e != nil {
				return ActivationReadiness{}, e
			}
			ids := []string{}
			for items.Next() {
				var id string
				if e = items.Scan(&id); e != nil {
					items.Close()
					return ActivationReadiness{}, e
				}
				ids = append(ids, id)
			}
			items.Close()
			for _, id := range ids {
				if e = appendProjection(d, "BRAND", id, ""); e != nil {
					return ActivationReadiness{}, e
				}
			}
		default:
			return ActivationReadiness{}, fmt.Errorf("unsupported compliance requirement scope %s", d.scope)
		}
	}
	blockers := []RequirementResult{}
	satisfied := 0
	for _, r := range reqs {
		if r.Satisfied {
			satisfied++
		} else {
			blockers = append(blockers, r)
		}
	}
	progress := 0
	if len(reqs) > 0 {
		progress = satisfied * 100 / len(reqs)
	}
	return ActivationReadiness{Ready: len(blockers) == 0, Progress: progress, Requirements: reqs, Blockers: blockers}, nil
}

func (s *Service) Overview(ctx context.Context, actor Actor) (OnboardingOverview, error) {
	if err := s.require(ctx, actor, "vendor.onboarding.read", nil); err != nil {
		return OnboardingOverview{}, err
	}
	_ = s.ensureOnboarding(ctx, actor.VendorID)
	var out OnboardingOverview
	if err := s.db.QueryRow(ctx, `SELECT status,version FROM vendor_onboarding_profiles WHERE business_id=$1`, actor.VendorID).Scan(&out.Status, &out.Version); err != nil {
		return out, err
	}
	if r, e := s.GetRepresentative(ctx, actor); e == nil {
		out.Representative = &r
	}
	out.Verifications, _ = s.ListVerifications(ctx, Actor{StaffID: actor.StaffID, VendorID: actor.VendorID})
	out.Readiness, _ = s.Readiness(ctx, actor)
	return out, nil
}

func (s *Service) SubmitOnboarding(ctx context.Context, actor Actor) (OnboardingOverview, error) {
	if err := s.require(ctx, actor, "vendor.onboarding.manage", nil); err != nil {
		return OnboardingOverview{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return OnboardingOverview{}, err
	}
	defer tx.Rollback(ctx)
	if err = s.ensureOnboardingTx(ctx, tx, actor.VendorID); err != nil {
		return OnboardingOverview{}, err
	}
	_, err = tx.Exec(ctx, `UPDATE vendor_onboarding_profiles SET status='PENDING_VERIFICATION',submitted_at=COALESCE(submitted_at,now()),updated_at=now(),version=version+1 WHERE business_id=$1 AND status<>'ACTIVATED'`, actor.VendorID)
	if err != nil {
		return OnboardingOverview{}, err
	}
	_, err = tx.Exec(ctx, `UPDATE vendor_businesses SET status='PENDING_VERIFICATION',updated_at=now(),version=version+1 WHERE id=$1 AND status='DRAFT'`, actor.VendorID)
	if err != nil {
		return OnboardingOverview{}, err
	}
	if err = s.publish(ctx, tx, "vendor.onboarding.submitted", "vendor_onboarding", actor.VendorID, 1, actor, map[string]any{"business_id": actor.VendorID}); err != nil {
		return OnboardingOverview{}, err
	}
	if err = s.auditTx(ctx, tx, actor, "vendor.onboarding.submitted", "vendor_onboarding", actor.VendorID, "", nil, map[string]any{"status": "PENDING_VERIFICATION"}, ""); err != nil {
		return OnboardingOverview{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return OnboardingOverview{}, err
	}
	s.metrics.Inc("onboarding_submitted_total")
	return s.Overview(ctx, actor)
}

func (s *Service) RequestActivation(ctx context.Context, actor Actor) (ActivationReadiness, error) {
	if err := s.require(ctx, actor, "vendor.onboarding.manage", nil); err != nil {
		return ActivationReadiness{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return ActivationReadiness{}, err
	}
	defer tx.Rollback(ctx)
	if err = s.evaluateActivationTx(ctx, tx, actor); err != nil {
		return ActivationReadiness{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return ActivationReadiness{}, err
	}
	return s.Readiness(ctx, actor)
}

func (s *Service) evaluateActivationTx(ctx context.Context, tx pgx.Tx, actor Actor) error {
	r, err := s.readiness(ctx, tx, actor.VendorID)
	if err != nil {
		return err
	}
	if err = s.ensureOnboardingTx(ctx, tx, actor.VendorID); err != nil {
		return err
	}
	if !r.Ready {
		_, err = tx.Exec(ctx, `UPDATE vendor_onboarding_profiles SET status=CASE WHEN EXISTS(SELECT 1 FROM vendor_verification_projections WHERE business_id=$1 AND status='REJECTED') THEN 'ACTION_REQUIRED' ELSE 'PENDING_REVIEW' END,updated_at=now(),version=version+1 WHERE business_id=$1 AND status<>'ACTIVATED'`, actor.VendorID)
		return err
	}
	var wasActive bool
	if err = tx.QueryRow(ctx, `SELECT status='ACTIVE' FROM vendor_businesses WHERE id=$1 FOR UPDATE`, actor.VendorID).Scan(&wasActive); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE vendor_businesses SET status='ACTIVE',updated_at=now(),version=version+1 WHERE id=$1 AND status IN ('DRAFT','PENDING_VERIFICATION','PENDING_REVIEW')`, actor.VendorID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE vendor_capabilities SET status='ACTIVE',updated_at=now(),version=version+1 WHERE business_id=$1 AND status IN ('DRAFT','PENDING_REVIEW')`, actor.VendorID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE vendor_branches SET status='ACTIVE',updated_at=now(),version=version+1 WHERE business_id=$1 AND status='DRAFT'`, actor.VendorID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE vendor_branch_capabilities SET status='ACTIVE',updated_at=now(),version=version+1 WHERE business_id=$1 AND status='DRAFT'`, actor.VendorID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE vendor_onboarding_profiles SET status='ACTIVATED',activated_at=COALESCE(activated_at,now()),updated_at=now(),version=version+1 WHERE business_id=$1`, actor.VendorID)
	if err != nil {
		return err
	}
	if !wasActive {
		if err = s.publish(ctx, tx, "vendor.business.activated", "vendor_business", actor.VendorID, 1, actor, map[string]any{"business_id": actor.VendorID, "activated_at": time.Now().UTC()}); err != nil {
			return err
		}
		if err = s.auditTx(ctx, tx, actor, "vendor.business.activated", "vendor_business", actor.VendorID, "", nil, map[string]any{"status": "ACTIVE"}, ""); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) PutBankProfile(ctx context.Context, actor Actor, accountNumber, bankCode, idempotencyKey, stepUpGrantID string, expectedVersion int64) (BankProfile, error) {
	if err := s.require(ctx, actor, "vendor.bank_profile.manage", nil); err != nil {
		return BankProfile{}, err
	}
	if len(strings.TrimSpace(accountNumber)) < 6 || len(strings.TrimSpace(bankCode)) < 2 {
		return BankProfile{}, validation(apperrors.FieldViolation{Field: "bank_profile", Message: "Account number and bank code are required."})
	}
	if s.bank == nil {
		return BankProfile{}, apperrors.Unavailable("Bank resolution authority is not configured.", nil)
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return BankProfile{}, err
	}
	defer tx.Rollback(ctx)
	// Serialize bank replacements per business before consuming the step-up
	// grant or invoking Wallet. This prevents two callers with the same stale
	// expected version from both mutating the authority.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, actor.VendorID); err != nil {
		return BankProfile{}, err
	}
	var currentVersion int64
	if err = tx.QueryRow(ctx, `SELECT COALESCE((SELECT version FROM vendor_bank_profiles WHERE business_id=$1),0)`, actor.VendorID).Scan(&currentVersion); err != nil {
		return BankProfile{}, err
	}
	if currentVersion != expectedVersion {
		return BankProfile{}, conflict(nil)
	}
	if s.stepUp == nil || s.stepUp.ConsumeGrant(ctx, stepUpGrantID, actor.SessionID, actor.StaffID, actor.VendorID, "finance.bank_change") != nil {
		return BankProfile{}, apperrors.Forbidden("Step-up authentication is required for bank profile changes.", nil)
	}
	result, err := s.bank.ResolveAndRegister(ctx, actor.VendorID, strings.TrimSpace(accountNumber), strings.TrimSpace(bankCode), idempotencyKey)
	if err != nil {
		s.metrics.Inc("bank_resolution_failure_total")
		if read, e := s.bank.GetProfile(ctx, actor.VendorID); e == nil {
			result = read
		} else {
			return BankProfile{}, apperrors.Unavailable("Bank profile outcome is unknown; authoritative read-back failed.", err)
		}
	}
	var p BankProfile
	err = tx.QueryRow(ctx, `INSERT INTO vendor_bank_profiles(business_id,external_ref,bank_code,masked_account,resolved_account_name,source_version,status,observed_at) SELECT $1,$2,$3,$4,$5,$6,$7,$8 WHERE $9=0 ON CONFLICT(business_id) DO UPDATE SET external_ref=EXCLUDED.external_ref,bank_code=EXCLUDED.bank_code,masked_account=EXCLUDED.masked_account,resolved_account_name=EXCLUDED.resolved_account_name,source_version=EXCLUDED.source_version,status=EXCLUDED.status,observed_at=EXCLUDED.observed_at,updated_at=now(),version=vendor_bank_profiles.version+1 WHERE EXCLUDED.source_version>vendor_bank_profiles.source_version AND vendor_bank_profiles.version=$9 RETURNING business_id,external_ref,bank_code,masked_account,resolved_account_name,source_version,status,observed_at,updated_at,version`, actor.VendorID, result.ExternalRef, result.BankCode, result.MaskedAccount, result.ResolvedAccountName, result.SourceVersion, result.Status, result.ObservedAt, expectedVersion).Scan(&p.BusinessID, &p.ExternalRef, &p.BankCode, &p.MaskedAccount, &p.ResolvedAccountName, &p.SourceVersion, &p.Status, &p.ObservedAt, &p.UpdatedAt, &p.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return BankProfile{}, conflict(err)
	}
	if err != nil {
		return BankProfile{}, err
	}
	vr := VerificationCaseResult{SourceRef: result.ExternalRef, SourceVersion: result.SourceVersion, Status: result.Status, ObservedAt: result.ObservedAt}
	if _, err = s.upsertProjectionTx(ctx, tx, actor, "payment-wallet-service", vr, DimensionBank, "BUSINESS", actor.VendorID); err != nil {
		return BankProfile{}, err
	}
	if err = s.publish(ctx, tx, "vendor.bank_profile.changed", "vendor_bank_profile", actor.VendorID, p.Version, actor, map[string]any{"external_ref": p.ExternalRef, "bank_code": p.BankCode, "masked_account": p.MaskedAccount, "status": p.Status}); err != nil {
		return BankProfile{}, err
	}
	if err = s.auditTx(ctx, tx, actor, "vendor.bank_profile.changed", "vendor_bank_profile", actor.VendorID, "", nil, map[string]any{"external_ref": p.ExternalRef, "bank_code": p.BankCode, "masked_account": p.MaskedAccount, "status": p.Status}, ""); err != nil {
		return BankProfile{}, err
	}
	if err = s.evaluateActivationTx(ctx, tx, actor); err != nil {
		return BankProfile{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return BankProfile{}, err
	}
	s.metrics.Inc("bank_profile_changed_total", "status", string(p.Status))
	return p, nil
}

func (s *Service) GetBankProfile(ctx context.Context, actor Actor) (BankProfile, error) {
	if err := s.require(ctx, actor, "vendor.bank_profile.read", nil); err != nil {
		return BankProfile{}, err
	}
	var p BankProfile
	err := s.db.QueryRow(ctx, `SELECT business_id,external_ref,bank_code,masked_account,resolved_account_name,source_version,status,observed_at,updated_at,version FROM vendor_bank_profiles WHERE business_id=$1`, actor.VendorID).Scan(&p.BusinessID, &p.ExternalRef, &p.BankCode, &p.MaskedAccount, &p.ResolvedAccountName, &p.SourceVersion, &p.Status, &p.ObservedAt, &p.UpdatedAt, &p.Version)
	return p, mapDB(err, "Bank profile")
}

func (s *Service) upsertProjectionTx(ctx context.Context, tx pgx.Tx, actor Actor, source string, result VerificationCaseResult, dimension VerificationDimension, subjectType, subjectRef string) (VerificationProjection, error) {
	var p VerificationProjection
	err := tx.QueryRow(ctx, `INSERT INTO vendor_verification_projections(id,business_id,dimension,subject_type,subject_ref,source_service,source_ref,source_version,status,observed_at,expires_at,reason_code,remediation) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,NULLIF($12,''),NULLIF($13,'')) ON CONFLICT(business_id,dimension,subject_type,subject_ref) DO UPDATE SET source_service=EXCLUDED.source_service,source_ref=EXCLUDED.source_ref,source_version=EXCLUDED.source_version,status=EXCLUDED.status,observed_at=EXCLUDED.observed_at,expires_at=EXCLUDED.expires_at,reason_code=EXCLUDED.reason_code,remediation=EXCLUDED.remediation,updated_at=now(),version=vendor_verification_projections.version+1 WHERE EXCLUDED.source_ref<>vendor_verification_projections.source_ref OR EXCLUDED.source_version>vendor_verification_projections.source_version RETURNING `+projectionCols, newID("vvp_"), actor.VendorID, dimension, subjectType, subjectRef, source, result.SourceRef, result.SourceVersion, result.Status, result.ObservedAt, result.ExpiresAt, result.ReasonCode, result.Remediation).Scan(&p.ID, &p.BusinessID, &p.Dimension, &p.SubjectType, &p.SubjectRef, &p.SourceService, &p.SourceRef, &p.SourceVersion, &p.Status, &p.ObservedAt, &p.ExpiresAt, &p.ReasonCode, &p.Remediation, &p.UpdatedAt, &p.Version)
	if err != nil {
		return VerificationProjection{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO vendor_verification_history(id,projection_id,business_id,source_service,source_ref,source_version,status,observed_at,expires_at,reason_code,remediation) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,NULLIF($10,''),NULLIF($11,''))`, newID("vvh_"), p.ID, actor.VendorID, source, result.SourceRef, result.SourceVersion, result.Status, result.ObservedAt, result.ExpiresAt, result.ReasonCode, result.Remediation)
	return p, err
}

func (s *Service) invalidateProjectionTx(ctx context.Context, tx pgx.Tx, businessID string, dimension VerificationDimension, reason string, actor Actor) error {
	_, err := tx.Exec(ctx, `WITH changed AS (UPDATE vendor_verification_projections SET status='NOT_STARTED',observed_at=now(),reason_code=$3,remediation='Resubmit this verification after the material profile change.',updated_at=now(),version=version+1 WHERE business_id=$1 AND dimension=$2 AND status<>'NOT_STARTED' RETURNING *) INSERT INTO vendor_verification_history(id,projection_id,business_id,source_service,source_ref,source_version,status,observed_at,expires_at,reason_code,remediation) SELECT 'vvh_'||gen_random_uuid()::text,id,business_id,source_service,source_ref,source_version,status,observed_at,expires_at,reason_code,remediation FROM changed`, businessID, dimension, reason)
	return err
}

func (s *Service) invalidateSubjectProjectionTx(ctx context.Context, tx pgx.Tx, businessID string, dimension VerificationDimension, subjectType, subjectRef, reason string) error {
	_, err := tx.Exec(ctx, `WITH changed AS (
		UPDATE vendor_verification_projections SET status='NOT_STARTED',observed_at=now(),reason_code=$5,
		remediation='Resubmit this verification after the material profile change.',updated_at=now(),version=version+1
		WHERE business_id=$1 AND dimension=$2 AND subject_type=$3 AND subject_ref=$4 AND status<>'NOT_STARTED' RETURNING *)
		INSERT INTO vendor_verification_history(id,projection_id,business_id,source_service,source_ref,source_version,status,observed_at,expires_at,reason_code,remediation)
		SELECT 'vvh_'||gen_random_uuid()::text,id,business_id,source_service,source_ref,source_version,status,observed_at,expires_at,reason_code,remediation FROM changed`, businessID, dimension, subjectType, subjectRef, reason)
	return err
}

func (s *Service) ensureOnboarding(ctx context.Context, businessID string) error {
	_, err := s.db.Exec(ctx, `INSERT INTO vendor_onboarding_profiles(business_id,status) VALUES($1,'NOT_STARTED') ON CONFLICT DO NOTHING`, businessID)
	return err
}
func (s *Service) ensureOnboardingTx(ctx context.Context, tx pgx.Tx, businessID string) error {
	_, err := tx.Exec(ctx, `INSERT INTO vendor_onboarding_profiles(business_id,status) VALUES($1,'NOT_STARTED') ON CONFLICT DO NOTHING`, businessID)
	return err
}

func (s *Service) auditTx(ctx context.Context, tx pgx.Tx, actor Actor, action, aggregate, id, branch string, before, after any, reason string) error {
	beforeJSON, _ := json.Marshal(before)
	afterJSON, _ := json.Marshal(after)
	_, err := tx.Exec(ctx, `INSERT INTO vendor_audit_log(id,vendor_id,actor_type,actor_id,action,aggregate_type,aggregate_id,branch_id,correlation_id,before_data,after_data,reason) VALUES($1,$2,$3,NULLIF($4,''),$5,$6,$7,NULLIF($8,''),NULLIF($9,''),$10,$11,NULLIF($12,''))`, `val_`+uuid.NewString(), actor.VendorID, "staff", actor.StaffID, action, aggregate, id, branch, actor.CorrelationID, nilJSON(before, beforeJSON), nilJSON(after, afterJSON), reason)
	return err
}
func nilJSON(v any, b []byte) any {
	if v == nil {
		return nil
	}
	return b
}

func (s *Service) ExpireVerifications(ctx context.Context, limit int) (int64, error) {
	if limit <= 0 {
		limit = 100
	}
	tag, err := s.db.Exec(ctx, `WITH due AS (SELECT id FROM vendor_verification_projections WHERE status='VERIFIED' AND expires_at<=now() ORDER BY expires_at LIMIT $1 FOR UPDATE SKIP LOCKED), changed AS (UPDATE vendor_verification_projections p SET status='EXPIRED',observed_at=now(),reason_code='VERIFICATION_EXPIRED',remediation='Resubmit verification.',updated_at=now(),version=version+1 FROM due WHERE p.id=due.id RETURNING p.*) INSERT INTO vendor_verification_history(id,projection_id,business_id,source_service,source_ref,source_version,status,observed_at,expires_at,reason_code,remediation) SELECT 'vvh_'||gen_random_uuid()::text,id,business_id,source_service,source_ref,source_version,status,observed_at,expires_at,reason_code,remediation FROM changed`, limit)
	if err != nil {
		return 0, err
	}
	if tag.RowsAffected() > 0 {
		s.metrics.Add("verification_expired_total", float64(tag.RowsAffected()))
	}
	return tag.RowsAffected(), nil
}

func (s *Service) ReconcileVerifications(ctx context.Context, worker string, limit int) error {
	if s.verification == nil {
		return nil
	}
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.Query(ctx, `WITH due AS (SELECT id FROM vendor_verification_operations WHERE status IN ('PENDING','UNKNOWN','FAILED','COMPLETED') AND next_attempt_at<=now() AND (lease_expires_at IS NULL OR lease_expires_at<now()) ORDER BY next_attempt_at LIMIT $1 FOR UPDATE SKIP LOCKED) UPDATE vendor_verification_operations o SET lease_owner=$2,lease_expires_at=now()+interval '30 seconds',attempts=attempts+1,updated_at=now() FROM due WHERE o.id=due.id RETURNING o.id,o.business_id,o.dimension,o.subject_type,o.subject_ref,o.client_reference,o.attempts`, limit, worker)
	if err != nil {
		return err
	}
	type op struct {
		id, business, subjectType, subjectRef, client string
		dimension                                     VerificationDimension
		attempts                                      int
	}
	ops := []op{}
	for rows.Next() {
		var o op
		if e := rows.Scan(&o.id, &o.business, &o.dimension, &o.subjectType, &o.subjectRef, &o.client, &o.attempts); e != nil {
			rows.Close()
			return e
		}
		ops = append(ops, o)
	}
	rows.Close()
	for _, o := range ops {
		result, e := s.verification.GetCaseByClientReference(ctx, o.client)
		if e != nil {
			status := "FAILED"
			if o.attempts >= 8 {
				status = "DEAD_LETTER"
			}
			backoff := time.Duration(1<<min(o.attempts, 10)) * time.Minute
			_, _ = s.db.Exec(ctx, `UPDATE vendor_verification_operations SET status=$2,next_attempt_at=now()+$3::interval,lease_owner=NULL,lease_expires_at=NULL,last_error='VERIFICATION_SOURCE_UNAVAILABLE',updated_at=now(),version=version+1 WHERE id=$1`, o.id, status, fmt.Sprintf("%d seconds", int64(backoff.Seconds())))
			continue
		}
		_, e = s.applyProjection(ctx, Actor{VendorID: o.business, StaffID: "verification-reconciler"}, "verification-compliance-service", result, o.dimension, o.subjectType, o.subjectRef, "")
		if e == nil {
			status := "PENDING"
			if terminalVerificationStatus(result.Status) {
				status = "COMPLETED"
			}
			_, _ = s.db.Exec(ctx, `UPDATE vendor_verification_operations SET status=$2,source_ref=$3,attempts=0,next_attempt_at=now()+interval '1 minute',lease_owner=NULL,lease_expires_at=NULL,last_error=NULL,updated_at=now(),version=version+1 WHERE id=$1`, o.id, status, result.SourceRef)
		} else {
			status := "FAILED"
			if o.attempts >= 8 {
				status = "DEAD_LETTER"
			}
			_, _ = s.db.Exec(ctx, `UPDATE vendor_verification_operations SET status=$2,next_attempt_at=now()+interval '1 minute',lease_owner=NULL,lease_expires_at=NULL,last_error='PROJECTION_APPLY_FAILED',updated_at=now(),version=version+1 WHERE id=$1`, o.id, status)
		}
	}
	return nil
}

// ReplayVerificationOperation returns a dead-lettered operation to the bounded
// reconciliation queue. It is intentionally not exposed as a public Vendor API.
func (s *Service) ReplayVerificationOperation(ctx context.Context, operationID string) error {
	tag, err := s.db.Exec(ctx, `UPDATE vendor_verification_operations SET status='PENDING',attempts=0,next_attempt_at=now(),lease_owner=NULL,lease_expires_at=NULL,last_error=NULL,updated_at=now(),version=version+1 WHERE id=$1 AND status='DEAD_LETTER'`, operationID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return apperrors.NotFound("Dead-lettered verification operation not found.", nil)
	}
	return nil
}
