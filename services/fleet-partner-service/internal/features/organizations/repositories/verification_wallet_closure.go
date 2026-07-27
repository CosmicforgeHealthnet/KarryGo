package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/concurrency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

// ---- Verification ----

// CreateVerificationRequest stores an immutable submission snapshot.
//
// The unique (organization_id, submission_version) constraint makes duplicate
// submissions of the same version fail rather than overwrite history.
func (r *OnboardingRepository) CreateVerificationRequest(ctx context.Context, q database.Querier, organizationID string, request *models.VerificationRequest) error {
	if request.ID == "" {
		request.ID = ids.New("ver")
	}
	request.OrganizationID = organizationID
	snapshot, err := json.Marshal(orEmptyMap(request.SubmissionSnapshot))
	if err != nil {
		return fmt.Errorf("organizations: marshal submission snapshot: %w", err)
	}
	_, err = q.Exec(ctx, `
		INSERT INTO fleet_partner_organization_verification_requests
			(id, organization_id, submission_version, submission_snapshot, status, submitted_by,
			 request_id, correlation_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now(), now())`,
		request.ID, organizationID, request.SubmissionVersion, snapshot,
		models.VerificationStatusPendingDispatch, request.SubmittedBy,
		request.RequestID, request.CorrelationID)
	if err != nil {
		return fmt.Errorf("organizations: create verification request: %w", err)
	}
	return nil
}

// NextSubmissionVersion returns the next submission version for the org.
func (r *OnboardingRepository) NextSubmissionVersion(ctx context.Context, q database.Querier, organizationID string) (int, error) {
	var current *int
	err := q.QueryRow(ctx, `
		SELECT max(submission_version) FROM fleet_partner_organization_verification_requests
		WHERE organization_id = $1`, organizationID).Scan(&current)
	if err != nil {
		return 0, fmt.Errorf("organizations: next submission version: %w", err)
	}
	if current == nil {
		return 1, nil
	}
	return *current + 1, nil
}

// LatestVerificationRequest returns the most recent submission.
func (r *OnboardingRepository) LatestVerificationRequest(ctx context.Context, organizationID string) (*models.VerificationRequest, error) {
	var request models.VerificationRequest
	var snapshot []byte
	err := r.db.QueryRow(ctx, `
		SELECT id, organization_id, submission_version, submission_snapshot, status,
			external_reference, source_service, source_version, last_event_id, synced_at,
			submitted_by, request_id, correlation_id, created_at, updated_at
		FROM fleet_partner_organization_verification_requests
		WHERE organization_id = $1 ORDER BY submission_version DESC LIMIT 1`, organizationID).
		Scan(&request.ID, &request.OrganizationID, &request.SubmissionVersion, &snapshot,
			&request.Status, &request.ExternalReference, &request.SourceService,
			&request.SourceVersion, &request.LastEventID, &request.SyncedAt,
			&request.SubmittedBy, &request.RequestID, &request.CorrelationID,
			&request.CreatedAt, &request.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("organizations: scan verification request: %w", err)
	}
	_ = json.Unmarshal(snapshot, &request.SubmissionSnapshot)
	return &request, nil
}

// UpdateVerificationStatus projects an externally-owned verification outcome.
// Older source versions are ignored so out-of-order events cannot regress it.
func (r *OnboardingRepository) UpdateVerificationStatus(ctx context.Context, q database.Querier, organizationID string, requestID string, status string, externalReference string, sourceVersion int64, eventID string) error {
	var ref *string
	if externalReference != "" {
		ref = &externalReference
	}
	_, err := q.Exec(ctx, `
		UPDATE fleet_partner_organization_verification_requests SET
			status = $3,
			external_reference = COALESCE($4, external_reference),
			source_version = GREATEST(source_version, $5),
			last_event_id = $6, synced_at = now(), updated_at = now()
		WHERE organization_id = $1 AND id = $2 AND source_version <= $5`,
		organizationID, requestID, status, ref, sourceVersion, eventID)
	if err != nil {
		return fmt.Errorf("organizations: update verification status: %w", err)
	}
	return nil
}

// AppendVerificationIssues records append-only review findings.
func (r *OnboardingRepository) AppendVerificationIssues(ctx context.Context, q database.Querier, organizationID string, requestID string, issues []models.VerificationIssue) error {
	for _, issue := range issues {
		if _, err := q.Exec(ctx, `
			INSERT INTO fleet_partner_organization_verification_issues
				(id, organization_id, verification_request_id, issue_code, field_reference,
				 document_reference, remediation, raised_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, now())`,
			ids.New("vis"), organizationID, requestID, issue.IssueCode,
			issue.FieldReference, issue.DocumentReference, issue.Remediation); err != nil {
			return fmt.Errorf("organizations: append verification issue: %w", err)
		}
	}
	return nil
}

// ListVerificationIssues returns issues for a submission.
func (r *OnboardingRepository) ListVerificationIssues(ctx context.Context, organizationID string, requestID string) ([]models.VerificationIssue, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, organization_id, verification_request_id, issue_code, field_reference,
			document_reference, remediation, raised_at
		FROM fleet_partner_organization_verification_issues
		WHERE organization_id = $1 AND verification_request_id = $2
		ORDER BY raised_at, id`, organizationID, requestID)
	if err != nil {
		return nil, fmt.Errorf("organizations: list verification issues: %w", err)
	}
	defer rows.Close()

	var issues []models.VerificationIssue
	for rows.Next() {
		var issue models.VerificationIssue
		if err := rows.Scan(&issue.ID, &issue.OrganizationID, &issue.VerificationRequestID,
			&issue.IssueCode, &issue.FieldReference, &issue.DocumentReference,
			&issue.Remediation, &issue.RaisedAt); err != nil {
			return nil, fmt.Errorf("organizations: scan verification issue: %w", err)
		}
		issues = append(issues, issue)
	}
	return issues, rows.Err()
}

// ---- Wallet provisioning ----

// EnsureWalletProvisioning creates the provisioning row if absent.
func (r *OnboardingRepository) EnsureWalletProvisioning(ctx context.Context, q database.Querier, organizationID string, idempotencyKey string) error {
	_, err := q.Exec(ctx, `
		INSERT INTO fleet_partner_organization_wallet_provisioning
			(organization_id, status, idempotency_key, version, created_at, updated_at)
		VALUES ($1, 'REQUESTED', $2, 1, now(), now())
		ON CONFLICT (organization_id) DO NOTHING`, organizationID, idempotencyKey)
	if err != nil {
		return fmt.Errorf("organizations: ensure wallet provisioning: %w", err)
	}
	return nil
}

// GetWalletProvisioning loads the wallet provisioning projection.
func (r *OnboardingRepository) GetWalletProvisioning(ctx context.Context, organizationID string) (*models.WalletProvisioning, error) {
	var wallet models.WalletProvisioning
	err := r.db.QueryRow(ctx, `
		SELECT organization_id, status, wallet_account_id, idempotency_key, attempts,
			last_error_code, next_attempt_at, source_service, source_version, last_event_id,
			synced_at, version, created_at, updated_at
		FROM fleet_partner_organization_wallet_provisioning WHERE organization_id = $1`, organizationID).
		Scan(&wallet.OrganizationID, &wallet.Status, &wallet.WalletAccountID, &wallet.IdempotencyKey,
			&wallet.Attempts, &wallet.LastErrorCode, &wallet.NextAttemptAt, &wallet.SourceService,
			&wallet.SourceVersion, &wallet.LastEventID, &wallet.SyncedAt, &wallet.Version,
			&wallet.CreatedAt, &wallet.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("organizations: scan wallet provisioning: %w", err)
	}
	return &wallet, nil
}

// SetWalletStatus updates the projection. The wallet account ID is only ever
// stored from a confirmed response or reconciliation — never invented.
func (r *OnboardingRepository) SetWalletStatus(ctx context.Context, q database.Querier, organizationID string, status string, walletAccountID string, errorCode string, nextAttemptAt *time.Time) error {
	var accountID, code *string
	if walletAccountID != "" {
		accountID = &walletAccountID
	}
	if errorCode != "" {
		code = &errorCode
	}
	_, err := q.Exec(ctx, `
		UPDATE fleet_partner_organization_wallet_provisioning SET
			status = $2,
			wallet_account_id = COALESCE($3, wallet_account_id),
			last_error_code = $4,
			next_attempt_at = $5,
			attempts = attempts + 1,
			synced_at = now(),
			version = version + 1,
			updated_at = now()
		WHERE organization_id = $1`, organizationID, status, accountID, code, nextAttemptAt)
	if err != nil {
		return fmt.Errorf("organizations: set wallet status: %w", err)
	}
	return nil
}

// ---- Closure ----

// CreateClosureRequest opens a closure workflow. The partial unique index
// makes a second open request fail rather than duplicate.
func (r *OnboardingRepository) CreateClosureRequest(ctx context.Context, q database.Querier, organizationID string, request *models.ClosureRequest) error {
	if request.ID == "" {
		request.ID = ids.New("clo")
	}
	request.OrganizationID = organizationID
	request.Version = 1
	_, err := q.Exec(ctx, `
		INSERT INTO fleet_partner_organization_closure_requests
			(id, organization_id, status, reason_code, reason_text, requested_by,
			 request_id, correlation_id, version, created_at, updated_at)
		VALUES ($1, $2, 'REQUESTED', $3, $4, $5, $6, $7, 1, now(), now())`,
		request.ID, organizationID, request.ReasonCode, request.ReasonText,
		request.RequestedBy, request.RequestID, request.CorrelationID)
	if err != nil {
		return fmt.Errorf("organizations: create closure request: %w", err)
	}
	return nil
}

// GetOpenClosureRequest returns the organization's open closure request.
func (r *OnboardingRepository) GetOpenClosureRequest(ctx context.Context, organizationID string) (*models.ClosureRequest, error) {
	var request models.ClosureRequest
	err := r.db.QueryRow(ctx, `
		SELECT id, organization_id, status, reason_code, reason_text, requested_by,
			approval_request_id, assessed_at, completed_at, version, request_id, correlation_id,
			created_at, updated_at
		FROM fleet_partner_organization_closure_requests
		WHERE organization_id = $1
		  AND status IN ('REQUESTED','ASSESSING','BLOCKED','APPROVAL_REQUIRED','APPROVED','EXECUTING')
		ORDER BY created_at DESC LIMIT 1`, organizationID).
		Scan(&request.ID, &request.OrganizationID, &request.Status, &request.ReasonCode,
			&request.ReasonText, &request.RequestedBy, &request.ApprovalRequestID,
			&request.AssessedAt, &request.CompletedAt, &request.Version,
			&request.RequestID, &request.CorrelationID, &request.CreatedAt, &request.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("organizations: scan closure request: %w", err)
	}
	return &request, nil
}

// UpdateClosureStatus applies a compare-and-swap closure status change.
func (r *OnboardingRepository) UpdateClosureStatus(ctx context.Context, q database.Querier, organizationID string, closureID string, status string, expectedVersion int64) error {
	tag, err := q.Exec(ctx, `
		UPDATE fleet_partner_organization_closure_requests SET
			status = $3,
			assessed_at = CASE WHEN $3 IN ('BLOCKED','APPROVAL_REQUIRED','APPROVED') THEN now() ELSE assessed_at END,
			completed_at = CASE WHEN $3 = 'COMPLETED' THEN now() ELSE completed_at END,
			version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND id = $2 AND version = $4`,
		organizationID, closureID, status, expectedVersion)
	if err != nil {
		return fmt.Errorf("organizations: update closure status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return concurrency.ErrVersionMismatch
	}
	return nil
}

// ReplaceClosureBlockers records the current blocker assessment.
func (r *OnboardingRepository) ReplaceClosureBlockers(ctx context.Context, q database.Querier, organizationID string, closureID string, blockers []models.ClosureBlocker) error {
	if _, err := q.Exec(ctx, `
		DELETE FROM fleet_partner_organization_closure_blockers
		WHERE organization_id = $1 AND closure_request_id = $2`, organizationID, closureID); err != nil {
		return fmt.Errorf("organizations: clear closure blockers: %w", err)
	}
	for _, blocker := range blockers {
		if _, err := q.Exec(ctx, `
			INSERT INTO fleet_partner_organization_closure_blockers
				(id, organization_id, closure_request_id, blocker_code, owning_service, detail,
				 state, checked_at, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, now(), now(), now())`,
			ids.New("blk"), organizationID, closureID, blocker.BlockerCode,
			blocker.OwningService, blocker.Detail, blocker.State); err != nil {
			return fmt.Errorf("organizations: insert closure blocker: %w", err)
		}
	}
	return nil
}

// ListClosureBlockers returns the recorded blockers.
func (r *OnboardingRepository) ListClosureBlockers(ctx context.Context, organizationID string, closureID string) ([]models.ClosureBlocker, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, organization_id, closure_request_id, blocker_code, owning_service, detail, state, checked_at
		FROM fleet_partner_organization_closure_blockers
		WHERE organization_id = $1 AND closure_request_id = $2
		ORDER BY blocker_code`, organizationID, closureID)
	if err != nil {
		return nil, fmt.Errorf("organizations: list closure blockers: %w", err)
	}
	defer rows.Close()

	var blockers []models.ClosureBlocker
	for rows.Next() {
		var blocker models.ClosureBlocker
		if err := rows.Scan(&blocker.ID, &blocker.OrganizationID, &blocker.ClosureRequestID,
			&blocker.BlockerCode, &blocker.OwningService, &blocker.Detail,
			&blocker.State, &blocker.CheckedAt); err != nil {
			return nil, fmt.Errorf("organizations: scan closure blocker: %w", err)
		}
		blockers = append(blockers, blocker)
	}
	return blockers, rows.Err()
}
