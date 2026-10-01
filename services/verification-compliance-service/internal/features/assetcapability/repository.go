package assetcapability

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository owns every statement this feature issues.
//
// It takes a pgx.Tx on the write paths rather than opening its own transaction,
// because the invariant that matters most here is that a decision, the case
// status it moves, the certification it produces and the outbox row that tells
// the authority about it all land together or not at all. A certification that
// exists with no event to deliver it is an asset that is eligible in this
// service and pending in the one that actually dispatches work.
type Repository struct{ db *pgxpool.Pool }

// NewRepository builds a repository over the service pool.
func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

// Pool exposes the pool for callers that must open their own transaction.
func (r *Repository) Pool() *pgxpool.Pool { return r.db }

const caseColumns = `id::text, authority, provider_id, asset_id, capability_code, product_code,
	requested_value_json, evidence_refs, document_refs, COALESCE(inspection_id::text,''),
	status, policy_version, source_version, reason_code, expires_at, correlation_id,
	observed_at, created_at, updated_at`

func scanCase(row pgx.Row) (Case, error) {
	var c Case
	var requested, evidence, documents []byte
	err := row.Scan(&c.CaseRef, &c.Authority, &c.ProviderID, &c.AssetID, &c.CapabilityCode,
		&c.ProductCode, &requested, &evidence, &documents, &c.InspectionRef, &c.Status,
		&c.PolicyVersion, &c.SourceVersion, &c.ReasonCode, &c.ExpiresAt, &c.CorrelationID,
		&c.ObservedAt, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, ErrNotFound
	}
	if err != nil {
		return c, err
	}
	_ = json.Unmarshal(requested, &c.RequestedValue)
	c.EvidenceRefs = decodeStrings(evidence)
	c.DocumentRefs = decodeStrings(documents)
	return c, nil
}

func decodeStrings(raw []byte) []string {
	out := []string{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out)
	}
	if out == nil {
		out = []string{}
	}
	return out
}

func encodeJSON(value any) []byte {
	if value == nil {
		return []byte(`{}`)
	}
	raw, err := json.Marshal(value)
	if err != nil || len(raw) == 0 {
		return []byte(`{}`)
	}
	return raw
}

func encodeStrings(values []string) []byte {
	if values == nil {
		values = []string{}
	}
	raw, _ := json.Marshal(values)
	return raw
}

// CreateCaseInput is everything needed to open a case.
type CreateCaseInput struct {
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
	Fingerprint    string
}

// CreateCase inserts a case, returning the existing row when the same
// idempotency key carries the same request.
//
// A different request under the same key is a conflict rather than a second
// case: two different certification questions sharing one key would make the
// retry that answered first decide which question was asked.
func (r *Repository) CreateCase(ctx context.Context, tx pgx.Tx, in CreateCaseInput) (Case, error) {
	var inspection *string
	if in.InspectionRef != "" {
		inspection = &in.InspectionRef
	}
	created, err := scanCase(tx.QueryRow(ctx, `
		INSERT INTO asset_capability_cases(
			authority, provider_id, asset_id, capability_code, product_code,
			requested_value_json, evidence_refs, document_refs, inspection_id,
			policy_version, expires_at, correlation_id, idempotency_key, request_fingerprint)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9::uuid,$10,$11,$12,$13,$14)
		ON CONFLICT (idempotency_key) DO NOTHING
		RETURNING `+caseColumns,
		in.Authority, in.ProviderID, in.AssetID, in.CapabilityCode, in.ProductCode,
		encodeJSON(in.RequestedValue), encodeStrings(in.EvidenceRefs), encodeStrings(in.DocumentRefs),
		inspection, in.PolicyVersion, in.ExpiresAt, in.CorrelationID,
		in.IdempotencyKey, in.Fingerprint))
	if err == nil {
		return created, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return Case{}, err
	}
	// The key already exists. Replay it only when it asked the same question.
	var fingerprint string
	existing, err := scanCaseWithFingerprint(tx.QueryRow(ctx,
		`SELECT `+caseColumns+`, request_fingerprint FROM asset_capability_cases WHERE idempotency_key=$1`,
		in.IdempotencyKey), &fingerprint)
	if err != nil {
		return Case{}, err
	}
	if fingerprint != in.Fingerprint {
		return Case{}, ErrConflict
	}
	return existing, nil
}

func scanCaseWithFingerprint(row pgx.Row, fingerprint *string) (Case, error) {
	var c Case
	var requested, evidence, documents []byte
	err := row.Scan(&c.CaseRef, &c.Authority, &c.ProviderID, &c.AssetID, &c.CapabilityCode,
		&c.ProductCode, &requested, &evidence, &documents, &c.InspectionRef, &c.Status,
		&c.PolicyVersion, &c.SourceVersion, &c.ReasonCode, &c.ExpiresAt, &c.CorrelationID,
		&c.ObservedAt, &c.CreatedAt, &c.UpdatedAt, fingerprint)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, ErrNotFound
	}
	if err != nil {
		return c, err
	}
	_ = json.Unmarshal(requested, &c.RequestedValue)
	c.EvidenceRefs = decodeStrings(evidence)
	c.DocumentRefs = decodeStrings(documents)
	return c, nil
}

// GetCase reads one case by reference.
func (r *Repository) GetCase(ctx context.Context, caseRef string) (Case, error) {
	return scanCase(r.db.QueryRow(ctx,
		`SELECT `+caseColumns+` FROM asset_capability_cases WHERE id::text=$1`, caseRef))
}

// LockCase reads a case inside a transaction and holds it for update, so a
// concurrent decision on the same case serializes rather than interleaves.
func (r *Repository) LockCase(ctx context.Context, tx pgx.Tx, caseRef string) (Case, error) {
	return scanCase(tx.QueryRow(ctx,
		`SELECT `+caseColumns+` FROM asset_capability_cases WHERE id::text=$1 FOR UPDATE`, caseRef))
}

// AdvanceCase moves a case to the status a decision implies, guarding on the
// version the reviewer believed they were acting on.
func (r *Repository) AdvanceCase(ctx context.Context, tx pgx.Tx, caseRef string,
	expectedVersion int64, status CaseStatus, reasonCode string, expiresAt *time.Time) (Case, error) {
	updated, err := scanCase(tx.QueryRow(ctx, `
		UPDATE asset_capability_cases
		   SET status=$2, reason_code=$3, expires_at=COALESCE($4, expires_at),
		       source_version=source_version+1, observed_at=now(), updated_at=now()
		 WHERE id::text=$1 AND source_version=$5
		RETURNING `+caseColumns,
		caseRef, status, reasonCode, expiresAt, expectedVersion))
	if errors.Is(err, ErrNotFound) {
		return Case{}, ErrVersion
	}
	return updated, err
}

// AppendDecision writes one immutable decision row.
//
// The reference slices are normalised to empty rather than left nil so the
// decision serialises as [] like every other record here. A field that is
// sometimes null and sometimes [] makes a consumer write two code paths for one
// meaning.
func (r *Repository) AppendDecision(ctx context.Context, tx pgx.Tx, record DecisionRecord) (DecisionRecord, error) {
	if record.EvidenceRefs == nil {
		record.EvidenceRefs = []string{}
	}
	if record.DocumentRefs == nil {
		record.DocumentRefs = []string{}
	}
	err := tx.QueryRow(ctx, `
		INSERT INTO asset_capability_decisions(
			case_id, decision_version, decision, verified_value_json, reason_code, reviewer,
			evidence_refs, document_refs, policy_version, effective_at, expires_at, correlation_id)
		VALUES($1::uuid,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		RETURNING id::text, decided_at`,
		record.CaseRef, record.DecisionVersion, record.Decision, encodeJSON(record.VerifiedValue),
		record.ReasonCode, record.Reviewer, encodeStrings(record.EvidenceRefs),
		encodeStrings(record.DocumentRefs), record.PolicyVersion, record.EffectiveAt,
		record.ExpiresAt, record.CorrelationID).Scan(&record.DecisionRef, &record.DecidedAt)
	if isUniqueViolation(err) {
		// Two reviewers raced on the same case version. The version guard on
		// the case row already resolves who won; this is the same collision
		// seen from the decision side.
		return DecisionRecord{}, ErrVersion
	}
	return record, err
}

// ListDecisions returns a case's decision history, newest first.
func (r *Repository) ListDecisions(ctx context.Context, caseRef string) ([]DecisionRecord, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, case_id::text, decision_version, decision, verified_value_json,
		       reason_code, reviewer, evidence_refs, document_refs, policy_version,
		       effective_at, expires_at, correlation_id, decided_at
		  FROM asset_capability_decisions WHERE case_id::text=$1
		 ORDER BY decision_version DESC`, caseRef)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DecisionRecord{}
	for rows.Next() {
		var d DecisionRecord
		var verified, evidence, documents []byte
		if err = rows.Scan(&d.DecisionRef, &d.CaseRef, &d.DecisionVersion, &d.Decision, &verified,
			&d.ReasonCode, &d.Reviewer, &evidence, &documents, &d.PolicyVersion,
			&d.EffectiveAt, &d.ExpiresAt, &d.CorrelationID, &d.DecidedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(verified, &d.VerifiedValue)
		d.EvidenceRefs = decodeStrings(evidence)
		d.DocumentRefs = decodeStrings(documents)
		out = append(out, d)
	}
	return out, rows.Err()
}

const certificationColumns = `id::text, authority, provider_id, asset_id, product_code,
	capability_code, status, policy_version, certification_ref, effective_at, expires_at,
	source_version, reason_code, COALESCE(case_id::text,''), updated_at`

func scanCertification(row pgx.Row) (Certification, error) {
	var c Certification
	err := row.Scan(&c.CertificationID, &c.Authority, &c.ProviderID, &c.AssetID, &c.ProductCode,
		&c.CapabilityCode, &c.Status, &c.PolicyVersion, &c.CertificationRef, &c.EffectiveAt,
		&c.ExpiresAt, &c.SourceVersion, &c.ReasonCode, &c.CaseRef, &c.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, ErrNotFound
	}
	return c, err
}

// OpenPendingCertification records that an asset is under review for a product.
//
// It never downgrades a live ELIGIBLE certification. An asset whose Comfort
// certification is valid until December does not stop being a Comfort vehicle
// because somebody opened next year's renewal case in November. A certification
// that already lapsed (SUSPENDED, EXPIRED, REJECTED) does move back to PENDING,
// which is how a re-inspection visibly restarts the lifecycle.
func (r *Repository) OpenPendingCertification(ctx context.Context, tx pgx.Tx, c Case) (Certification, bool, error) {
	certification, err := scanCertification(tx.QueryRow(ctx, `
		INSERT INTO service_tier_certifications(
			authority, provider_id, asset_id, product_code, capability_code, status,
			policy_version, reason_code, case_id)
		VALUES($1,$2,$3,$4,$5,'PENDING',$6,$7,$8::uuid)
		ON CONFLICT (authority, asset_id, product_code) DO UPDATE
		   SET status='PENDING', capability_code=EXCLUDED.capability_code,
		       policy_version=EXCLUDED.policy_version, reason_code=$9,
		       case_id=EXCLUDED.case_id, certification_ref='',
		       effective_at=NULL, expires_at=NULL,
		       source_version=service_tier_certifications.source_version+1, updated_at=now()
		 WHERE service_tier_certifications.status IN ('SUSPENDED','EXPIRED','REJECTED')
		RETURNING `+certificationColumns,
		c.Authority, c.ProviderID, c.AssetID, c.ProductCode, c.CapabilityCode,
		c.PolicyVersion, ReasonCaseOpened, c.CaseRef, ReasonReinspection))
	if err == nil {
		return certification, true, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return Certification{}, false, err
	}
	// The conflict target matched but the WHERE clause refused the update: an
	// eligible certification is already live and stays untouched.
	existing, readErr := scanCertification(tx.QueryRow(ctx,
		`SELECT `+certificationColumns+` FROM service_tier_certifications
		  WHERE authority=$1 AND asset_id=$2 AND product_code=$3`,
		c.Authority, c.AssetID, c.ProductCode))
	return existing, false, readErr
}

// ApplyDecisionInput carries a decision onto the certification projection.
type ApplyDecisionInput struct {
	Case             Case
	Status           CertificationStatus
	CertificationRef string
	PolicyVersion    string
	ReasonCode       string
	EffectiveAt      *time.Time
	ExpiresAt        *time.Time
	DecisionRef      string
}

// ApplyDecision upserts the certification a decision produces.
//
// source_version is bumped by the database rather than supplied, so two
// decisions arriving at once cannot both claim the same version and leave the
// driver services unable to tell which one is newer.
func (r *Repository) ApplyDecision(ctx context.Context, tx pgx.Tx, in ApplyDecisionInput) (Certification, error) {
	c := in.Case
	return scanCertification(tx.QueryRow(ctx, `
		INSERT INTO service_tier_certifications(
			authority, provider_id, asset_id, product_code, capability_code, status,
			policy_version, certification_ref, effective_at, expires_at, reason_code,
			last_decision_id, case_id)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12::uuid,$13::uuid)
		ON CONFLICT (authority, asset_id, product_code) DO UPDATE
		   SET provider_id=EXCLUDED.provider_id, capability_code=EXCLUDED.capability_code,
		       status=EXCLUDED.status, policy_version=EXCLUDED.policy_version,
		       certification_ref=EXCLUDED.certification_ref, effective_at=EXCLUDED.effective_at,
		       expires_at=EXCLUDED.expires_at, reason_code=EXCLUDED.reason_code,
		       last_decision_id=EXCLUDED.last_decision_id, case_id=EXCLUDED.case_id,
		       source_version=service_tier_certifications.source_version+1, updated_at=now()
		RETURNING `+certificationColumns,
		c.Authority, c.ProviderID, c.AssetID, c.ProductCode, c.CapabilityCode, in.Status,
		in.PolicyVersion, in.CertificationRef, in.EffectiveAt, in.ExpiresAt, in.ReasonCode,
		in.DecisionRef, c.CaseRef))
}

// ListCertifications returns every certification held for one asset.
func (r *Repository) ListCertifications(ctx context.Context, authority Authority, assetID string) ([]Certification, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+certificationColumns+` FROM service_tier_certifications
		  WHERE authority=$1 AND asset_id=$2 ORDER BY product_code`, authority, assetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Certification{}
	for rows.Next() {
		var c Certification
		if err = rows.Scan(&c.CertificationID, &c.Authority, &c.ProviderID, &c.AssetID,
			&c.ProductCode, &c.CapabilityCode, &c.Status, &c.PolicyVersion, &c.CertificationRef,
			&c.EffectiveAt, &c.ExpiresAt, &c.SourceVersion, &c.ReasonCode, &c.CaseRef,
			&c.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// -- Provider verification ----------------------------------------------------

const providerColumns = `authority, provider_id, status, reason_code, source_version,
	effective_at, expires_at, correlation_id, updated_at`

func scanProvider(row pgx.Row) (ProviderVerification, error) {
	var p ProviderVerification
	err := row.Scan(&p.Authority, &p.ProviderID, &p.Status, &p.ReasonCode, &p.SourceVersion,
		&p.EffectiveAt, &p.ExpiresAt, &p.CorrelationID, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, ErrNotFound
	}
	return p, err
}

// GetProviderVerification reads a provider's standing with one authority.
func (r *Repository) GetProviderVerification(ctx context.Context, tx pgx.Tx,
	authority Authority, providerID string) (ProviderVerification, error) {
	query := `SELECT ` + providerColumns + ` FROM provider_verification_records
	           WHERE authority=$1 AND provider_id=$2`
	if tx != nil {
		return scanProvider(tx.QueryRow(ctx, query, authority, providerID))
	}
	return scanProvider(r.db.QueryRow(ctx, query, authority, providerID))
}

// UpsertProviderVerification records a provider's standing, refusing an update
// that is not newer than what is stored.
func (r *Repository) UpsertProviderVerification(ctx context.Context, p ProviderVerification) (ProviderVerification, error) {
	updated, err := scanProvider(r.db.QueryRow(ctx, `
		INSERT INTO provider_verification_records(
			authority, provider_id, status, reason_code, source_version,
			effective_at, expires_at, correlation_id)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (authority, provider_id) DO UPDATE
		   SET status=EXCLUDED.status, reason_code=EXCLUDED.reason_code,
		       source_version=EXCLUDED.source_version, effective_at=EXCLUDED.effective_at,
		       expires_at=EXCLUDED.expires_at, correlation_id=EXCLUDED.correlation_id,
		       updated_at=now()
		 WHERE provider_verification_records.source_version < EXCLUDED.source_version
		RETURNING `+providerColumns,
		p.Authority, p.ProviderID, p.Status, p.ReasonCode, p.SourceVersion,
		p.EffectiveAt, p.ExpiresAt, p.CorrelationID))
	if errors.Is(err, ErrNotFound) {
		// The row exists at an equal or newer version. A stale report must not
		// resurrect a standing that has since been withdrawn.
		return ProviderVerification{}, ErrVersion
	}
	return updated, err
}

// -- Vehicle documents --------------------------------------------------------

const documentColumns = `id::text, authority, provider_id, asset_id, document_type,
	media_asset_id, document_number, status, issued_at, expires_at, reason_code,
	source_version, correlation_id, created_at, updated_at`

func scanDocument(row pgx.Row) (VehicleDocument, error) {
	var d VehicleDocument
	err := row.Scan(&d.DocumentRef, &d.Authority, &d.ProviderID, &d.AssetID, &d.DocumentType,
		&d.MediaAssetID, &d.DocumentNumber, &d.Status, &d.IssuedAt, &d.ExpiresAt,
		&d.ReasonCode, &d.SourceVersion, &d.CorrelationID, &d.CreatedAt, &d.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return d, ErrNotFound
	}
	return d, err
}

// RecordDocument stores a document reference, replaying an identical retry.
func (r *Repository) RecordDocument(ctx context.Context, d VehicleDocument,
	idempotencyKey, fingerprint string) (VehicleDocument, error) {
	created, err := scanDocument(r.db.QueryRow(ctx, `
		INSERT INTO vehicle_documents(
			authority, provider_id, asset_id, document_type, media_asset_id, document_number,
			status, issued_at, expires_at, reason_code, correlation_id,
			idempotency_key, request_fingerprint)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		ON CONFLICT (idempotency_key) DO NOTHING
		RETURNING `+documentColumns,
		d.Authority, d.ProviderID, d.AssetID, d.DocumentType, d.MediaAssetID, d.DocumentNumber,
		d.Status, d.IssuedAt, d.ExpiresAt, d.ReasonCode, d.CorrelationID,
		idempotencyKey, fingerprint))
	if err == nil {
		return created, nil
	}
	if isUniqueViolation(err) {
		// The live-document index refused a second current document of this
		// type. That is a real state conflict, not an idempotency replay.
		return VehicleDocument{}, fmt.Errorf("%w: a current %s document already exists for this asset",
			ErrValidation, d.DocumentType)
	}
	if !errors.Is(err, ErrNotFound) {
		return VehicleDocument{}, err
	}
	var storedFingerprint string
	existing, readErr := scanDocumentWithFingerprint(r.db.QueryRow(ctx,
		`SELECT `+documentColumns+`, request_fingerprint FROM vehicle_documents WHERE idempotency_key=$1`,
		idempotencyKey), &storedFingerprint)
	if readErr != nil {
		return VehicleDocument{}, readErr
	}
	if storedFingerprint != fingerprint {
		return VehicleDocument{}, ErrConflict
	}
	return existing, nil
}

func scanDocumentWithFingerprint(row pgx.Row, fingerprint *string) (VehicleDocument, error) {
	var d VehicleDocument
	err := row.Scan(&d.DocumentRef, &d.Authority, &d.ProviderID, &d.AssetID, &d.DocumentType,
		&d.MediaAssetID, &d.DocumentNumber, &d.Status, &d.IssuedAt, &d.ExpiresAt,
		&d.ReasonCode, &d.SourceVersion, &d.CorrelationID, &d.CreatedAt, &d.UpdatedAt, fingerprint)
	if errors.Is(err, pgx.ErrNoRows) {
		return d, ErrNotFound
	}
	return d, err
}

// GetDocument reads one document reference.
func (r *Repository) GetDocument(ctx context.Context, tx pgx.Tx, documentRef string) (VehicleDocument, error) {
	query := `SELECT ` + documentColumns + ` FROM vehicle_documents WHERE id::text=$1`
	if tx != nil {
		return scanDocument(tx.QueryRow(ctx, query, documentRef))
	}
	return scanDocument(r.db.QueryRow(ctx, query, documentRef))
}

// ListDocuments returns every document reference held for one asset.
func (r *Repository) ListDocuments(ctx context.Context, authority Authority, assetID string) ([]VehicleDocument, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+documentColumns+` FROM vehicle_documents
		  WHERE authority=$1 AND asset_id=$2 ORDER BY document_type, created_at DESC`,
		authority, assetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []VehicleDocument{}
	for rows.Next() {
		var d VehicleDocument
		if err = rows.Scan(&d.DocumentRef, &d.Authority, &d.ProviderID, &d.AssetID, &d.DocumentType,
			&d.MediaAssetID, &d.DocumentNumber, &d.Status, &d.IssuedAt, &d.ExpiresAt,
			&d.ReasonCode, &d.SourceVersion, &d.CorrelationID, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// SetDocumentStatus moves a document to a new validity, guarding on version.
func (r *Repository) SetDocumentStatus(ctx context.Context, documentRef string,
	expectedVersion int64, status DocumentStatus, reasonCode string) (VehicleDocument, error) {
	updated, err := scanDocument(r.db.QueryRow(ctx, `
		UPDATE vehicle_documents
		   SET status=$2, reason_code=$3, source_version=source_version+1,
		       version=version+1, updated_at=now()
		 WHERE id::text=$1 AND source_version=$4
		RETURNING `+documentColumns,
		documentRef, status, reasonCode, expectedVersion))
	if errors.Is(err, ErrNotFound) {
		return VehicleDocument{}, ErrVersion
	}
	return updated, err
}

// -- Inspections --------------------------------------------------------------

const inspectionColumns = `id::text, authority, provider_id, asset_id, inspection_type,
	outcome, evidence_refs, inspected_at, expires_at, inspector_ref, reason_code,
	source_version, correlation_id, created_at, updated_at`

func scanInspection(row pgx.Row) (Inspection, error) {
	var i Inspection
	var evidence []byte
	err := row.Scan(&i.InspectionRef, &i.Authority, &i.ProviderID, &i.AssetID, &i.InspectionType,
		&i.Outcome, &evidence, &i.InspectedAt, &i.ExpiresAt, &i.InspectorRef, &i.ReasonCode,
		&i.SourceVersion, &i.CorrelationID, &i.CreatedAt, &i.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return i, ErrNotFound
	}
	if err != nil {
		return i, err
	}
	i.EvidenceRefs = decodeStrings(evidence)
	return i, nil
}

// RecordInspection stores an inspection, replaying an identical retry.
func (r *Repository) RecordInspection(ctx context.Context, tx pgx.Tx, i Inspection,
	idempotencyKey, fingerprint string) (Inspection, error) {
	created, err := scanInspection(tx.QueryRow(ctx, `
		INSERT INTO asset_inspections(
			authority, provider_id, asset_id, inspection_type, outcome, evidence_refs,
			inspected_at, expires_at, inspector_ref, reason_code, correlation_id,
			idempotency_key, request_fingerprint)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		ON CONFLICT (idempotency_key) DO NOTHING
		RETURNING `+inspectionColumns,
		i.Authority, i.ProviderID, i.AssetID, i.InspectionType, i.Outcome,
		encodeStrings(i.EvidenceRefs), i.InspectedAt, i.ExpiresAt, i.InspectorRef,
		i.ReasonCode, i.CorrelationID, idempotencyKey, fingerprint))
	if err == nil {
		return created, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return Inspection{}, err
	}
	var storedFingerprint string
	existing, readErr := scanInspectionWithFingerprint(tx.QueryRow(ctx,
		`SELECT `+inspectionColumns+`, request_fingerprint FROM asset_inspections WHERE idempotency_key=$1`,
		idempotencyKey), &storedFingerprint)
	if readErr != nil {
		return Inspection{}, readErr
	}
	if storedFingerprint != fingerprint {
		return Inspection{}, ErrConflict
	}
	return existing, nil
}

func scanInspectionWithFingerprint(row pgx.Row, fingerprint *string) (Inspection, error) {
	var i Inspection
	var evidence []byte
	err := row.Scan(&i.InspectionRef, &i.Authority, &i.ProviderID, &i.AssetID, &i.InspectionType,
		&i.Outcome, &evidence, &i.InspectedAt, &i.ExpiresAt, &i.InspectorRef, &i.ReasonCode,
		&i.SourceVersion, &i.CorrelationID, &i.CreatedAt, &i.UpdatedAt, fingerprint)
	if errors.Is(err, pgx.ErrNoRows) {
		return i, ErrNotFound
	}
	if err != nil {
		return i, err
	}
	i.EvidenceRefs = decodeStrings(evidence)
	return i, nil
}

// GetInspection reads one inspection.
func (r *Repository) GetInspection(ctx context.Context, tx pgx.Tx, inspectionRef string) (Inspection, error) {
	query := `SELECT ` + inspectionColumns + ` FROM asset_inspections WHERE id::text=$1`
	if tx != nil {
		return scanInspection(tx.QueryRow(ctx, query, inspectionRef))
	}
	return scanInspection(r.db.QueryRow(ctx, query, inspectionRef))
}

// -- Outbox -------------------------------------------------------------------

// PublishEvent writes one fact to the shared verification outbox, inside the
// caller's transaction.
//
// target is the authority a row must reach, or empty for a fact with no
// consumer endpoint, which is recorded NOT_REQUIRED rather than left pending
// forever.
func (r *Repository) PublishEvent(ctx context.Context, tx pgx.Tx, aggregateID, eventType string,
	eventVersion int64, target Authority, payload map[string]any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	status := DeliveryPending
	if target == "" {
		status = DeliveryNotRequired
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO verification_outbox(
			aggregate_id, event_type, event_version, payload, delivery_status, delivery_target)
		VALUES($1::uuid,$2,$3,$4,$5,$6)
		ON CONFLICT (aggregate_id, event_type, event_version) DO NOTHING`,
		aggregateID, eventType, eventVersion, raw, status, string(target))
	return err
}

// -- Idempotent command cache -------------------------------------------------

// LoadCommand returns a cached response for an identical earlier command.
func (r *Repository) LoadCommand(ctx context.Context, tx pgx.Tx, commandType, key, fingerprint string) ([]byte, bool, error) {
	var body []byte
	err := tx.QueryRow(ctx,
		`SELECT response_body FROM verification_commands
		  WHERE command_type=$1 AND idempotency_key=$2 AND request_fingerprint=$3`,
		commandType, key, fingerprint).Scan(&body)
	if errors.Is(err, pgx.ErrNoRows) {
		// Either no such command, or the same key carrying a different request.
		var exists bool
		if err = tx.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM verification_commands WHERE command_type=$1 AND idempotency_key=$2)`,
			commandType, key).Scan(&exists); err != nil {
			return nil, false, err
		}
		if exists {
			return nil, false, ErrConflict
		}
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return body, true, nil
}

// SaveCommand records a command's response so a retry replays it.
func (r *Repository) SaveCommand(ctx context.Context, tx pgx.Tx, commandType, key, fingerprint string, body []byte) error {
	_, err := tx.Exec(ctx,
		`INSERT INTO verification_commands(command_type, idempotency_key, request_fingerprint, response_body)
		 VALUES($1,$2,$3,$4) ON CONFLICT (command_type, idempotency_key) DO NOTHING`,
		commandType, key, fingerprint, body)
	return err
}

// -- Expiry sweeps ------------------------------------------------------------

// ExpireCertifications moves lapsed certifications to EXPIRED and returns them
// so the caller can publish an explicit event for each.
//
// The returned rows carry their new source_version, which is what makes a
// delivered EXPIRED newer than the ELIGIBLE it replaces. Silently dropping an
// asset out of eligibility without an event is the failure this guards: the
// driver service would keep dispatching against a certification that no longer
// exists here.
func (r *Repository) ExpireCertifications(ctx context.Context, now time.Time, limit int) ([]Certification, error) {
	// The CTE's column is named due_id rather than id on purpose: in an
	// UPDATE ... FROM, RETURNING can see both relations, and two columns called
	// id make every bare reference in the select list ambiguous. Renaming one
	// side is cheaper than qualifying every column of a shared list.
	rows, err := r.db.Query(ctx, `
		WITH due AS (
			SELECT id AS due_id FROM service_tier_certifications
			 WHERE status='ELIGIBLE' AND expires_at IS NOT NULL AND expires_at<=$1
			 ORDER BY expires_at LIMIT $2 FOR UPDATE SKIP LOCKED)
		UPDATE service_tier_certifications
		   SET status='EXPIRED', reason_code=$3,
		       source_version=service_tier_certifications.source_version+1, updated_at=now()
		  FROM due WHERE service_tier_certifications.id=due.due_id
		RETURNING `+certificationColumns, now, limit, ReasonCertificationExpired)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Certification{}
	for rows.Next() {
		var c Certification
		if err = rows.Scan(&c.CertificationID, &c.Authority, &c.ProviderID, &c.AssetID,
			&c.ProductCode, &c.CapabilityCode, &c.Status, &c.PolicyVersion, &c.CertificationRef,
			&c.EffectiveAt, &c.ExpiresAt, &c.SourceVersion, &c.ReasonCode, &c.CaseRef,
			&c.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ExpireInspections moves lapsed inspections to EXPIRED.
func (r *Repository) ExpireInspections(ctx context.Context, now time.Time, limit int) ([]Inspection, error) {
	rows, err := r.db.Query(ctx, `
		WITH due AS (
			SELECT id AS due_id FROM asset_inspections
			 WHERE outcome='PASSED' AND expires_at IS NOT NULL AND expires_at<=$1
			 ORDER BY expires_at LIMIT $2 FOR UPDATE SKIP LOCKED)
		UPDATE asset_inspections
		   SET outcome='EXPIRED', reason_code=$3,
		       source_version=asset_inspections.source_version+1,
		       version=asset_inspections.version+1, updated_at=now()
		  FROM due WHERE asset_inspections.id=due.due_id
		RETURNING `+inspectionColumns, now, limit, ReasonInspectionExpired)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Inspection{}
	for rows.Next() {
		var i Inspection
		var evidence []byte
		if err = rows.Scan(&i.InspectionRef, &i.Authority, &i.ProviderID, &i.AssetID,
			&i.InspectionType, &i.Outcome, &evidence, &i.InspectedAt, &i.ExpiresAt,
			&i.InspectorRef, &i.ReasonCode, &i.SourceVersion, &i.CorrelationID,
			&i.CreatedAt, &i.UpdatedAt); err != nil {
			return nil, err
		}
		i.EvidenceRefs = decodeStrings(evidence)
		out = append(out, i)
	}
	return out, rows.Err()
}

// ExpireDocuments moves lapsed documents to EXPIRED and reports how many.
func (r *Repository) ExpireDocuments(ctx context.Context, now time.Time, limit int) (int64, error) {
	tag, err := r.db.Exec(ctx, `
		WITH due AS (
			SELECT id FROM vehicle_documents
			 WHERE status='VALID' AND expires_at IS NOT NULL AND expires_at<=$1
			 ORDER BY expires_at LIMIT $2 FOR UPDATE SKIP LOCKED)
		UPDATE vehicle_documents d
		   SET status='EXPIRED', reason_code=$3,
		       source_version=d.source_version+1, version=d.version+1, updated_at=now()
		  FROM due WHERE d.id=due.id`, now, limit, ReasonDocumentExpired)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// ExpireProviderVerifications moves lapsed provider standings to EXPIRED.
//
// It deliberately does NOT cascade into that provider's live certifications.
// Withdrawing an asset's eligibility is a decision with operational
// consequences -- work in flight, a driver mid-shift -- and inventing it as a
// side effect of a clock would be policy this section never states. What the
// expiry does guarantee is that no NEW eligible certification can be issued for
// the provider, because the decision path re-checks the standing every time.
func (r *Repository) ExpireProviderVerifications(ctx context.Context, now time.Time, limit int) (int64, error) {
	tag, err := r.db.Exec(ctx, `
		WITH due AS (
			SELECT id FROM provider_verification_records
			 WHERE status='VERIFIED' AND expires_at IS NOT NULL AND expires_at<=$1
			 ORDER BY expires_at LIMIT $2 FOR UPDATE SKIP LOCKED)
		UPDATE provider_verification_records p
		   SET status='EXPIRED', reason_code=$3,
		       source_version=p.source_version+1, updated_at=now()
		  FROM due WHERE p.id=due.id`, now, limit, ReasonProviderExpired)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
