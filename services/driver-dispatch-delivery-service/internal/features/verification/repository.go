package verification

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	ListSteps(ctx context.Context, providerID string) ([]VerificationStep, error)
	GetStep(ctx context.Context, providerID string, step Step) (VerificationStep, bool, error)
	ListStepDocuments(ctx context.Context, providerID string, step Step) ([]VerificationDocument, error)
	GetLastFaceCheck(ctx context.Context, providerID string) (FaceCheck, bool, error)
	GetProviderVerificationState(ctx context.Context, providerID string) (ProviderVerificationState, bool, error)
	InitializeStepsForProvider(ctx context.Context, providerID string) (InitializationResult, error)
	InsertDocument(ctx context.Context, input DocumentInput) error
	GetLatestIdentityGovtIDDocument(ctx context.Context, providerID string) (VerificationDocument, bool, error)
	UpdateStepSubmitted(ctx context.Context, providerID string, step Step) (VerificationStep, error)
	UpdateVehicleStepApproved(ctx context.Context, providerID string, bikeID string) (VerificationStep, bool, error)
	UpdateVehicleStepRejected(ctx context.Context, providerID string, reason string) (VerificationStep, bool, error)
	UpdateProviderProfilePhoto(ctx context.Context, providerID string, photoURL string) error
	UpdateProviderVerificationStatus(ctx context.Context, providerID string, status string) error
	CheckRequiredStepsApproved(ctx context.Context, providerID string) (bool, error)
	HasFullyApprovedAudit(ctx context.Context, providerID string) (bool, error)
	InsertAudit(ctx context.Context, input AuditInput) error
	InsertFaceCheck(ctx context.Context, input FaceCheckInput) (FaceCheck, error)
	UpdateFaceCheckResult(ctx context.Context, input FaceCheckResultInput) (FaceCheck, error)
	AdminApproveStep(ctx context.Context, providerID string, step Step, reviewerID string) (VerificationStep, error)
	AdminRejectStep(ctx context.Context, providerID string, step Step, reviewerID string, reason string) (VerificationStep, error)
	UpdateLicenceExpiry(ctx context.Context, providerID string, expiryDate time.Time) error
}

type PostgresRepository struct {
	db *pgxpool.Pool
}

func NewPostgresRepository(db *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) ListSteps(ctx context.Context, providerID string) ([]VerificationStep, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, provider_id::text, step, is_optional, is_auto_confirmed,
			confirm_method, status, submitted_at, reviewed_at, reviewer_id::text,
			rejection_reason, licence_expiry_date, updated_at
		FROM verification_steps
		WHERE provider_id = $1
		ORDER BY CASE step
			WHEN 'identity' THEN 1
			WHEN 'licence' THEN 2
			WHEN 'vehicle' THEN 3
			WHEN 'face' THEN 4
			WHEN 'guarantor' THEN 5
			WHEN 'emergency' THEN 6
			ELSE 99
		END
	`, providerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var steps []VerificationStep
	for rows.Next() {
		step, err := scanStep(rows)
		if err != nil {
			return nil, err
		}
		steps = append(steps, step)
	}
	return steps, rows.Err()
}

func (r *PostgresRepository) GetStep(ctx context.Context, providerID string, step Step) (VerificationStep, bool, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id::text, provider_id::text, step, is_optional, is_auto_confirmed,
			confirm_method, status, submitted_at, reviewed_at, reviewer_id::text,
			rejection_reason, licence_expiry_date, updated_at
		FROM verification_steps
		WHERE provider_id = $1 AND step = $2
	`, providerID, step)

	result, err := scanStep(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return VerificationStep{}, false, nil
	}
	return result, err == nil, err
}

func (r *PostgresRepository) ListStepDocuments(ctx context.Context, providerID string, step Step) ([]VerificationDocument, error) {
	rows, err := r.db.Query(ctx, `
		SELECT DISTINCT ON (d.document_type)
			d.id::text, d.step_id::text, d.provider_id::text, d.document_type,
			d.file_url, d.file_size, d.mime_type, d.uploaded_at
		FROM verification_documents d
		JOIN verification_steps s ON s.id = d.step_id
		WHERE d.provider_id = $1
			AND s.step = $2
		ORDER BY d.document_type, d.uploaded_at DESC, d.id DESC
	`, providerID, step)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var documents []VerificationDocument
	for rows.Next() {
		doc, err := scanDocument(rows)
		if err != nil {
			return nil, err
		}
		documents = append(documents, doc)
	}
	return documents, rows.Err()
}

func (r *PostgresRepository) GetLastFaceCheck(ctx context.Context, providerID string) (FaceCheck, bool, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id::text, provider_id::text, step_id::text, selfie_url, id_doc_url,
			match_score::float8, result, provider_used, error_message, checked_at
		FROM face_checks
		WHERE provider_id = $1
		ORDER BY checked_at DESC, id DESC
		LIMIT 1
	`, providerID)
	check, err := scanFaceCheck(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return FaceCheck{}, false, nil
	}
	return check, err == nil, err
}

func (r *PostgresRepository) GetProviderVerificationState(ctx context.Context, providerID string) (ProviderVerificationState, bool, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id::text, verification_status, is_active
		FROM providers
		WHERE id = $1
	`, providerID)

	state, err := scanProviderVerificationState(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return ProviderVerificationState{}, false, nil
	}
	return state, err == nil, err
}

func (r *PostgresRepository) InitializeStepsForProvider(ctx context.Context, providerID string) (InitializationResult, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return InitializationResult{}, err
	}
	defer tx.Rollback(ctx)

	result, err := initializeStepsTx(ctx, tx, providerID)
	if err != nil {
		return InitializationResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return InitializationResult{}, err
	}
	return result, nil
}

func (r *PostgresRepository) InsertDocument(ctx context.Context, input DocumentInput) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO verification_documents (
			step_id, provider_id, document_type, file_url, file_size, mime_type
		)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, input.StepID, input.ProviderID, input.DocumentType, input.FileURL, input.FileSize, input.MimeType)
	return err
}

func (r *PostgresRepository) GetLatestIdentityGovtIDDocument(ctx context.Context, providerID string) (VerificationDocument, bool, error) {
	row := r.db.QueryRow(ctx, `
		SELECT d.id::text, d.step_id::text, d.provider_id::text, d.document_type,
			d.file_url, d.file_size, d.mime_type, d.uploaded_at
		FROM verification_documents d
		JOIN verification_steps s ON s.id = d.step_id
		WHERE d.provider_id = $1
			AND s.step = 'identity'
			AND d.document_type = 'govt_id'
		ORDER BY d.uploaded_at DESC, d.id DESC
		LIMIT 1
	`, providerID)
	doc, err := scanDocument(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return VerificationDocument{}, false, nil
	}
	return doc, err == nil, err
}

func (r *PostgresRepository) UpdateStepSubmitted(ctx context.Context, providerID string, step Step) (VerificationStep, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE verification_steps
		SET status = 'submitted',
			submitted_at = now(),
			rejection_reason = NULL,
			updated_at = now()
		WHERE provider_id = $1 AND step = $2
		RETURNING id::text, provider_id::text, step, is_optional, is_auto_confirmed,
			confirm_method, status, submitted_at, reviewed_at, reviewer_id::text,
			rejection_reason, licence_expiry_date, updated_at
	`, providerID, step)
	return scanStep(row)
}

func (r *PostgresRepository) UpdateVehicleStepApproved(ctx context.Context, providerID string, _ string) (VerificationStep, bool, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE verification_steps
		SET status = 'approved',
			confirm_method = 'auto',
			reviewed_at = now(),
			rejection_reason = NULL,
			updated_at = now()
		WHERE provider_id = $1 AND step = 'vehicle'
		RETURNING id::text, provider_id::text, step, is_optional, is_auto_confirmed,
			confirm_method, status, submitted_at, reviewed_at, reviewer_id::text,
			rejection_reason, licence_expiry_date, updated_at
	`, providerID)
	step, err := scanStep(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return VerificationStep{}, false, nil
	}
	return step, err == nil, err
}

func (r *PostgresRepository) UpdateVehicleStepRejected(ctx context.Context, providerID string, reason string) (VerificationStep, bool, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE verification_steps
		SET status = 'rejected',
			rejection_reason = $2,
			reviewed_at = now(),
			updated_at = now()
		WHERE provider_id = $1 AND step = 'vehicle'
		RETURNING id::text, provider_id::text, step, is_optional, is_auto_confirmed,
			confirm_method, status, submitted_at, reviewed_at, reviewer_id::text,
			rejection_reason, licence_expiry_date, updated_at
	`, providerID, reason)
	step, err := scanStep(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return VerificationStep{}, false, nil
	}
	return step, err == nil, err
}

func (r *PostgresRepository) UpdateProviderProfilePhoto(ctx context.Context, providerID string, photoURL string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE providers
		SET profile_photo_url = $2,
			updated_at = now()
		WHERE id = $1
	`, providerID, photoURL)
	return err
}

func (r *PostgresRepository) UpdateProviderVerificationStatus(ctx context.Context, providerID string, status string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE providers
		SET verification_status = $2,
			updated_at = now()
		WHERE id = $1
	`, providerID, status)
	return err
}

func (r *PostgresRepository) CheckRequiredStepsApproved(ctx context.Context, providerID string) (bool, error) {
	// Count non-approved required (non-optional) steps.  If zero remain, all required steps are approved.
	row := r.db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM verification_steps
		WHERE provider_id = $1
			AND is_optional = false
			AND status != 'approved'
	`, providerID)
	var remaining int
	if err := row.Scan(&remaining); err != nil {
		return false, err
	}
	return remaining == 0, nil
}

func (r *PostgresRepository) HasFullyApprovedAudit(ctx context.Context, providerID string) (bool, error) {
	row := r.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM verification_audit
			WHERE provider_id = $1 AND action = 'fully_approved'
		)
	`, providerID)
	var exists bool
	if err := row.Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func (r *PostgresRepository) InsertAudit(ctx context.Context, input AuditInput) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO verification_audit (
			provider_id, step, action, from_status, to_status, performed_by, notes
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, input.ProviderID, input.Step, input.Action, input.FromStatus, input.ToStatus, input.PerformedBy, input.Notes)
	return err
}

func (r *PostgresRepository) InsertFaceCheck(ctx context.Context, input FaceCheckInput) (FaceCheck, error) {
	providerUsed := input.ProviderUsed
	if providerUsed == "" {
		providerUsed = "smile_identity"
	}
	row := r.db.QueryRow(ctx, `
		INSERT INTO face_checks (
			provider_id, step_id, selfie_url, id_doc_url, match_score, result,
			provider_used, error_message
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id::text, provider_id::text, step_id::text, selfie_url, id_doc_url,
			match_score::float8, result, provider_used, error_message, checked_at
	`, input.ProviderID, input.StepID, input.SelfieURL, input.IDDocURL,
		input.MatchScore, input.Result, providerUsed, input.ErrorMessage)
	return scanFaceCheck(row)
}

func (r *PostgresRepository) UpdateFaceCheckResult(ctx context.Context, input FaceCheckResultInput) (FaceCheck, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE face_checks
		SET match_score = $2,
			result = $3,
			error_message = $4,
			checked_at = now()
		WHERE id = $1
		RETURNING id::text, provider_id::text, step_id::text, selfie_url, id_doc_url,
			match_score::float8, result, provider_used, error_message, checked_at
	`, input.ID, input.MatchScore, input.Result, input.ErrorMessage)
	return scanFaceCheck(row)
}

func (r *PostgresRepository) AdminApproveStep(ctx context.Context, providerID string, step Step, reviewerID string) (VerificationStep, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE verification_steps
		SET status = 'approved',
			reviewed_at = now(),
			reviewer_id = $3,
			confirm_method = 'manual',
			rejection_reason = NULL,
			updated_at = now()
		WHERE provider_id = $1 AND step = $2
		RETURNING id::text, provider_id::text, step, is_optional, is_auto_confirmed,
			confirm_method, status, submitted_at, reviewed_at, reviewer_id::text,
			rejection_reason, licence_expiry_date, updated_at
	`, providerID, step, reviewerID)
	return scanStep(row)
}

func (r *PostgresRepository) AdminRejectStep(ctx context.Context, providerID string, step Step, reviewerID string, reason string) (VerificationStep, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE verification_steps
		SET status = 'rejected',
			reviewed_at = now(),
			reviewer_id = $3,
			rejection_reason = $4,
			updated_at = now()
		WHERE provider_id = $1 AND step = $2
		RETURNING id::text, provider_id::text, step, is_optional, is_auto_confirmed,
			confirm_method, status, submitted_at, reviewed_at, reviewer_id::text,
			rejection_reason, licence_expiry_date, updated_at
	`, providerID, step, reviewerID, reason)
	return scanStep(row)
}

func (r *PostgresRepository) UpdateLicenceExpiry(ctx context.Context, providerID string, expiryDate time.Time) error {
	_, err := r.db.Exec(ctx, `
		UPDATE verification_steps
		SET licence_expiry_date = $2,
			updated_at           = now()
		WHERE provider_id = $1 AND step = 'licence'
	`, providerID, expiryDate)
	return err
}

type stepDefinition struct {
	step            Step
	isOptional      bool
	isAutoConfirmed bool
	confirmMethod   *ConfirmMethod
	status          StepStatus
}

func defaultStepDefinitions() []stepDefinition {
	auto := ConfirmAuto
	return []stepDefinition{
		{step: StepIdentity, status: StatusPending},
		{step: StepLicence, isOptional: true, status: StatusPending},
		{step: StepVehicle, status: StatusPending},
		{step: StepFace, status: StatusPending},
		{step: StepGuarantor, isAutoConfirmed: true, confirmMethod: &auto, status: StatusApproved},
		{step: StepEmergency, isAutoConfirmed: true, confirmMethod: &auto, status: StatusApproved},
	}
}

type txExecutor interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func initializeStepsTx(ctx context.Context, tx txExecutor, providerID string) (InitializationResult, error) {
	var result InitializationResult
	now := time.Now().UTC()
	for _, def := range defaultStepDefinitions() {
		var confirmMethod any
		var reviewedAt any
		if def.confirmMethod != nil {
			confirmMethod = string(*def.confirmMethod)
			reviewedAt = now
		}

		var stepID string
		err := tx.QueryRow(ctx, `
			INSERT INTO verification_steps (
				provider_id, step, is_optional, is_auto_confirmed,
				confirm_method, status, reviewed_at
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (provider_id, step) DO NOTHING
			RETURNING id::text
		`, providerID, def.step, def.isOptional, def.isAutoConfirmed, confirmMethod, def.status, reviewedAt).Scan(&stepID)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return InitializationResult{}, fmt.Errorf("insert verification step %s: %w", def.step, err)
		}
		result.InsertedSteps++

		if def.isAutoConfirmed {
			notes := "Auto-confirmed from completed profile onboarding."
			tag, err := tx.Exec(ctx, `
				INSERT INTO verification_audit (
					provider_id, step, action, from_status, to_status, performed_by, notes
				)
				SELECT $1, $2, $3, $4, $5, NULL, $6
				WHERE NOT EXISTS (
					SELECT 1
					FROM verification_audit
					WHERE provider_id = $1
						AND step = $2
						AND action = $3
						AND from_status = $4
						AND to_status = $5
				)
			`, providerID, def.step, AuditActionAutoConfirmed, StatusPending, StatusApproved, notes)
			if err != nil {
				return InitializationResult{}, fmt.Errorf("insert auto-confirm audit %s: %w", def.step, err)
			}
			result.AutoConfirmedAuditRows += int(tag.RowsAffected())
		}
	}
	return result, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanStep(row scanner) (VerificationStep, error) {
	var step VerificationStep
	var rawStep string
	var rawStatus string
	var confirmMethod pgtype.Text
	var submittedAt pgtype.Timestamptz
	var reviewedAt pgtype.Timestamptz
	var reviewerID pgtype.Text
	var rejectionReason pgtype.Text
	var licenceExpiryDate pgtype.Date

	err := row.Scan(
		&step.ID,
		&step.ProviderID,
		&rawStep,
		&step.IsOptional,
		&step.IsAutoConfirmed,
		&confirmMethod,
		&rawStatus,
		&submittedAt,
		&reviewedAt,
		&reviewerID,
		&rejectionReason,
		&licenceExpiryDate,
		&step.UpdatedAt,
	)
	if err != nil {
		return VerificationStep{}, err
	}

	step.Step = Step(rawStep)
	step.Status = StepStatus(rawStatus)
	if confirmMethod.Valid {
		method := ConfirmMethod(confirmMethod.String)
		step.ConfirmMethod = &method
	}
	if submittedAt.Valid {
		value := submittedAt.Time
		step.SubmittedAt = &value
	}
	if reviewedAt.Valid {
		value := reviewedAt.Time
		step.ReviewedAt = &value
	}
	if reviewerID.Valid {
		value := reviewerID.String
		step.ReviewerID = &value
	}
	if rejectionReason.Valid {
		value := rejectionReason.String
		step.RejectionReason = &value
	}
	if licenceExpiryDate.Valid {
		t := licenceExpiryDate.Time
		step.LicenceExpiryDate = &t
	}
	return step, nil
}

func scanProviderVerificationState(row scanner) (ProviderVerificationState, error) {
	var state ProviderVerificationState
	if err := row.Scan(&state.ProviderID, &state.VerificationStatus, &state.IsActive); err != nil {
		return ProviderVerificationState{}, err
	}
	return state, nil
}

func scanDocument(row scanner) (VerificationDocument, error) {
	var doc VerificationDocument
	var fileSize pgtype.Int4
	var mimeType pgtype.Text

	if err := row.Scan(
		&doc.ID,
		&doc.StepID,
		&doc.ProviderID,
		&doc.DocumentType,
		&doc.FileURL,
		&fileSize,
		&mimeType,
		&doc.UploadedAt,
	); err != nil {
		return VerificationDocument{}, err
	}
	if fileSize.Valid {
		value := int(fileSize.Int32)
		doc.FileSize = &value
	}
	if mimeType.Valid {
		value := mimeType.String
		doc.MimeType = &value
	}
	return doc, nil
}

func scanFaceCheck(row scanner) (FaceCheck, error) {
	var check FaceCheck
	var matchScore pgtype.Float8
	var result pgtype.Text
	var errorMessage pgtype.Text
	var checkedAt pgtype.Timestamptz

	if err := row.Scan(
		&check.ID,
		&check.ProviderID,
		&check.StepID,
		&check.SelfieURL,
		&check.IDDocURL,
		&matchScore,
		&result,
		&check.ProviderUsed,
		&errorMessage,
		&checkedAt,
	); err != nil {
		return FaceCheck{}, err
	}
	if matchScore.Valid {
		value := matchScore.Float64
		check.MatchScore = &value
	}
	if result.Valid {
		value := result.String
		check.Result = &value
	}
	if errorMessage.Valid {
		value := errorMessage.String
		check.ErrorMessage = &value
	}
	if checkedAt.Valid {
		value := checkedAt.Time
		check.CheckedAt = &value
	}
	return check, nil
}
