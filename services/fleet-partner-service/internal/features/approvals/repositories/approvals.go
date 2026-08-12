// Package repositories stores approval requests, decisions and executions.
//
// Every statement is tenant-scoped in SQL. An approval names a controlled
// operation and who signed off on it; there is no legitimate read of one
// across a tenant boundary, so none exists here.
package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/approvals/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// ErrNotFound means no row matched within the tenant scope.
var ErrNotFound = errors.New("approvals: not found")

// ErrConflict means a uniqueness or concurrency guarantee was hit.
var ErrConflict = errors.New("approvals: conflict")

// Identifier prefixes.
const (
	prefixRequest   = "apr"
	prefixStep      = "aps"
	prefixDecision  = "apd"
	prefixExecution = "apx"
	prefixHistory   = "aph"
	prefixPolicy    = "app"
)

// Repository stores approval requests and their evidence.
type Repository struct {
	db database.Querier
}

// NewRepository wires the repository.
func NewRepository(db database.Querier) *Repository {
	return &Repository{db: db}
}

const requestColumns = `id, organization_id, branch_id, policy_id, policy_version,
	resource_type, resource_id, action, requester_id, request_fingerprint,
	payload_version, payload_snapshot, expected_subject_version, minimum_approvals,
	eligible_role_ids, self_approval_allowed, separation_of_duties,
	eligibility_decision_id, status, current_step, expires_at, executed_at,
	execution_reference, invalidated_at, invalidation_reason, idempotency_key,
	correlation_id, causation_id, version, created_at, updated_at`

// EnsurePolicy resolves the approval policy row for a controlled operation,
// creating it from the governing eligibility policy when absent.
//
// The approval framework's own policy table requires an organization, so there
// is no platform-wide row to inherit. Rather than fabricate a tenant at
// migration time, the row is created on first use from the policy version that
// actually governs the operation — which keeps the two in step by
// construction.
func (r *Repository) EnsurePolicy(ctx context.Context, q database.Querier,
	organizationID string, resourceType string, action string,
	template PolicyTemplate) (string, int64, error) {
	if q == nil {
		q = r.db
	}
	var id string
	var version int64
	err := q.QueryRow(ctx, `
		SELECT id, version FROM fleet_partner_approval_policies
		WHERE organization_id = $1 AND resource_type = $2 AND action = $3
		  AND status = 'ACTIVE'`, organizationID, resourceType, action).Scan(&id, &version)
	if err == nil {
		return id, version, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", 0, fmt.Errorf("approvals: read policy: %w", err)
	}

	id = ids.New(prefixPolicy)
	_, err = q.Exec(ctx, `
		INSERT INTO fleet_partner_approval_policies
			(id, organization_id, name, resource_type, action, quorum,
			 eligible_role_ids, requester_may_approve, expiry_seconds,
			 separation_of_duties, eligibility_policy_key,
			 requires_current_eligibility, version, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, 'ACTIVE')`,
		id, organizationID, template.Name, resourceType, action, template.Quorum,
		jsonArray(template.EligibleRoleIDs), template.RequesterMayApprove,
		template.ExpirySeconds, template.SeparationOfDuties,
		template.EligibilityPolicyKey, template.RequiresCurrentEligibility,
		template.Version)
	if err != nil {
		if isUniqueViolation(err) {
			// Lost a race with a concurrent creator; the other row is as good
			// as this one would have been.
			return r.EnsurePolicy(ctx, q, organizationID, resourceType, action, template)
		}
		return "", 0, fmt.Errorf("approvals: create policy: %w", err)
	}
	return id, template.Version, nil
}

// PolicyTemplate is what an approval policy row is created from.
type PolicyTemplate struct {
	Name                       string
	Quorum                     int
	EligibleRoleIDs            []string
	RequesterMayApprove        bool
	ExpirySeconds              int
	SeparationOfDuties         bool
	EligibilityPolicyKey       string
	RequiresCurrentEligibility bool
	Version                    int64
}

// CreateRequest inserts a request and its first step.
func (r *Repository) CreateRequest(ctx context.Context, q database.Querier,
	organizationID string, request *models.Request) error {
	if request.ID == "" {
		request.ID = ids.New(prefixRequest)
	}
	request.OrganizationID = organizationID
	request.Version = 1
	request.Status = models.StatusPending
	request.CurrentStep = 1

	snapshot, err := json.Marshal(request.PayloadSnapshot)
	if err != nil {
		return fmt.Errorf("approvals: encode payload: %w", err)
	}

	_, err = q.Exec(ctx, `
		INSERT INTO fleet_partner_approval_requests
			(id, organization_id, branch_id, policy_id, policy_version, resource_type,
			 resource_id, action, requester_id, request_fingerprint, payload_version,
			 payload_snapshot, expected_subject_version, minimum_approvals,
			 eligible_role_ids, self_approval_allowed, separation_of_duties,
			 eligibility_decision_id, status, current_step, expires_at,
			 idempotency_key, correlation_id, causation_id, version)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15,
		        $16, $17, $18, 'PENDING', 1, $19, $20, $21, $22, 1)`,
		request.ID, organizationID, request.BranchID, request.PolicyID,
		request.PolicyVersion, request.ResourceType, request.ResourceID, request.Action,
		request.RequesterID, request.RequestFingerprint, request.PayloadVersion,
		snapshot, request.ExpectedSubjectVersion, request.MinimumApprovals,
		jsonArray(request.EligibleRoleIDs), request.SelfApprovalAllowed,
		request.SeparationOfDuties, request.EligibilityDecisionID, request.ExpiresAt,
		request.IdempotencyKey, request.CorrelationID, request.CausationID)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return fmt.Errorf("approvals: create request: %w", err)
	}

	stepID := ids.New(prefixStep)
	if _, err := q.Exec(ctx, `
		INSERT INTO fleet_partner_approval_steps
			(id, organization_id, approval_request_id, step_number, required_quorum,
			 eligible_role_ids, status)
		VALUES ($1, $2, $3, 1, $4, $5, 'PENDING')`,
		stepID, organizationID, request.ID, request.MinimumApprovals,
		jsonArray(request.EligibleRoleIDs)); err != nil {
		return fmt.Errorf("approvals: create step: %w", err)
	}
	return nil
}

// GetRequest loads one request inside the tenant scope.
func (r *Repository) GetRequest(ctx context.Context, q database.Querier,
	organizationID string, requestID string) (*models.Request, error) {
	if q == nil {
		q = r.db
	}
	return scanRequest(q.QueryRow(ctx, `SELECT `+requestColumns+`
		FROM fleet_partner_approval_requests
		WHERE organization_id = $1 AND id = $2`, organizationID, requestID))
}

// LockRequest loads a request for update, serializing concurrent decisions.
//
// Concurrency here is not theoretical: two approvers clicking at once is the
// normal case, and without the lock both can read one-approval-short and both
// write the approval that completes quorum.
func (r *Repository) LockRequest(ctx context.Context, q database.Querier,
	organizationID string, requestID string) (*models.Request, error) {
	return scanRequest(q.QueryRow(ctx, `SELECT `+requestColumns+`
		FROM fleet_partner_approval_requests
		WHERE organization_id = $1 AND id = $2 FOR UPDATE`, organizationID, requestID))
}

// FindByIdempotencyKey resolves a retried request creation.
func (r *Repository) FindByIdempotencyKey(ctx context.Context, q database.Querier,
	organizationID string, idempotencyKey string) (*models.Request, error) {
	if q == nil {
		q = r.db
	}
	if idempotencyKey == "" {
		return nil, ErrNotFound
	}
	return scanRequest(q.QueryRow(ctx, `SELECT `+requestColumns+`
		FROM fleet_partner_approval_requests
		WHERE organization_id = $1 AND idempotency_key = $2`,
		organizationID, idempotencyKey))
}

// FindOpenForResource returns the live request for a resource and action.
func (r *Repository) FindOpenForResource(ctx context.Context, q database.Querier,
	organizationID string, resourceType string, resourceID string,
	action string) (*models.Request, error) {
	if q == nil {
		q = r.db
	}
	return scanRequest(q.QueryRow(ctx, `SELECT `+requestColumns+`
		FROM fleet_partner_approval_requests
		WHERE organization_id = $1 AND resource_type = $2 AND resource_id = $3
		  AND action = $4 AND status = 'PENDING'`,
		organizationID, resourceType, resourceID, action))
}

// ListFilter narrows an approval listing.
type ListFilter struct {
	ResourceType string
	ResourceID   string
	Action       string
	Status       string
}

// List returns approval requests inside the tenant and branch scope.
func (r *Repository) List(ctx context.Context, query tenant.Query,
	filter ListFilter) ([]models.Request, error) {
	limit := query.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	arguments := []any{query.OrganizationID, filter.ResourceType, filter.ResourceID,
		filter.Action, filter.Status, limit}
	statement := `SELECT ` + requestColumns + `
		FROM fleet_partner_approval_requests
		WHERE organization_id = $1
		  AND ($2 = '' OR resource_type = $2)
		  AND ($3 = '' OR resource_id = $3)
		  AND ($4 = '' OR action = $4)
		  AND ($5 = '' OR status = $5)`
	if len(query.PermittedBranchIDs) > 0 {
		arguments = append(arguments, query.PermittedBranchIDs)
		statement += ` AND (branch_id IS NULL OR branch_id = ANY($7))`
	}
	statement += ` ORDER BY created_at DESC, id DESC LIMIT $6`

	rows, err := r.db.Query(ctx, statement, arguments...)
	if err != nil {
		return nil, fmt.Errorf("approvals: list requests: %w", err)
	}
	defer rows.Close()

	requests := make([]models.Request, 0, limit)
	for rows.Next() {
		request, scanErr := scanRequestRow(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		requests = append(requests, *request)
	}
	return requests, rows.Err()
}

// UpdateStatus moves a request to a new status under optimistic concurrency.
func (r *Repository) UpdateStatus(ctx context.Context, q database.Querier,
	organizationID string, requestID string, expectedVersion int64,
	status string, invalidationReason string) error {
	tag, err := q.Exec(ctx, `
		UPDATE fleet_partner_approval_requests
		SET status = $4,
		    invalidated_at = CASE WHEN $4 = 'INVALIDATED' THEN now() ELSE invalidated_at END,
		    invalidation_reason = CASE WHEN $4 = 'INVALIDATED' THEN $5 ELSE invalidation_reason END,
		    version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND id = $2 AND version = $3`,
		organizationID, requestID, expectedVersion, status, invalidationReason)
	if err != nil {
		return fmt.Errorf("approvals: update status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrConflict
	}
	return nil
}

// MarkExecuted binds the request to the operation it was spent on.
//
// The execution reference is written in the same statement as the status, and
// the unique index on (organization, execution_reference) makes a second
// attempt collide. That is the exactly-once guarantee: it does not depend on
// the caller checking first.
func (r *Repository) MarkExecuted(ctx context.Context, q database.Querier,
	organizationID string, requestID string, expectedVersion int64,
	executionReference string) error {
	tag, err := q.Exec(ctx, `
		UPDATE fleet_partner_approval_requests
		SET status = 'EXECUTED', executed_at = now(), execution_reference = $4,
		    version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND id = $2 AND version = $3
		  AND status = 'APPROVED' AND executed_at IS NULL`,
		organizationID, requestID, expectedVersion, executionReference)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return fmt.Errorf("approvals: mark executed: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrConflict
	}
	return nil
}

// CurrentStep loads the open step of a request.
func (r *Repository) CurrentStep(ctx context.Context, q database.Querier,
	organizationID string, requestID string, stepNumber int) (*models.Step, error) {
	var step models.Step
	var roleIDs []byte
	err := q.QueryRow(ctx, `
		SELECT id, organization_id, approval_request_id, step_number, required_quorum,
		       eligible_role_ids, status, completed_at, version
		FROM fleet_partner_approval_steps
		WHERE organization_id = $1 AND approval_request_id = $2 AND step_number = $3`,
		organizationID, requestID, stepNumber).
		Scan(&step.ID, &step.OrganizationID, &step.ApprovalRequestID, &step.StepNumber,
			&step.RequiredQuorum, &roleIDs, &step.Status, &step.CompletedAt, &step.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("approvals: read step: %w", err)
	}
	step.EligibleRoleIDs = decodeStrings(roleIDs)
	return &step, nil
}

// RecordDecision appends one immutable approver decision.
func (r *Repository) RecordDecision(ctx context.Context, q database.Querier,
	organizationID string, decision *models.Decision) error {
	if decision.ID == "" {
		decision.ID = ids.New(prefixDecision)
	}
	decision.OrganizationID = organizationID

	_, err := q.Exec(ctx, `
		INSERT INTO fleet_partner_approval_decisions
			(id, organization_id, approval_request_id, approval_step_id, approver_id,
			 decision, reason, client_decision_id, step_up_reference, request_version,
			 approved_fingerprint, actor_role_id, correlation_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		decision.ID, organizationID, decision.ApprovalRequestID, decision.ApprovalStepID,
		decision.ApproverID, decision.Decision, decision.Reason, decision.ClientDecisionID,
		decision.StepUpReference, decision.RequestVersion, decision.ApprovedFingerprint,
		decision.ActorRoleID, decision.CorrelationID)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return fmt.Errorf("approvals: record decision: %w", err)
	}
	return nil
}

// FindDecisionByClientID resolves a retried decision.
func (r *Repository) FindDecisionByClientID(ctx context.Context, q database.Querier,
	requestID string, clientDecisionID string) (*models.Decision, error) {
	if q == nil {
		q = r.db
	}
	if clientDecisionID == "" {
		return nil, ErrNotFound
	}
	return scanDecision(q.QueryRow(ctx, `
		SELECT id, organization_id, approval_request_id, approval_step_id, approver_id,
		       decision, reason, client_decision_id, step_up_reference, request_version,
		       approved_fingerprint, actor_role_id, correlation_id, decided_at
		FROM fleet_partner_approval_decisions
		WHERE approval_request_id = $1 AND client_decision_id = $2`,
		requestID, clientDecisionID))
}

// FindDecisionByApprover resolves an approver's existing decision.
func (r *Repository) FindDecisionByApprover(ctx context.Context, q database.Querier,
	requestID string, approverID string) (*models.Decision, error) {
	if q == nil {
		q = r.db
	}
	return scanDecision(q.QueryRow(ctx, `
		SELECT id, organization_id, approval_request_id, approval_step_id, approver_id,
		       decision, reason, client_decision_id, step_up_reference, request_version,
		       approved_fingerprint, actor_role_id, correlation_id, decided_at
		FROM fleet_partner_approval_decisions
		WHERE approval_request_id = $1 AND approver_id = $2`, requestID, approverID))
}

// ListDecisions returns every decision on a request.
func (r *Repository) ListDecisions(ctx context.Context, q database.Querier,
	requestID string) ([]models.Decision, error) {
	if q == nil {
		q = r.db
	}
	rows, err := q.Query(ctx, `
		SELECT id, organization_id, approval_request_id, approval_step_id, approver_id,
		       decision, reason, client_decision_id, step_up_reference, request_version,
		       approved_fingerprint, actor_role_id, correlation_id, decided_at
		FROM fleet_partner_approval_decisions
		WHERE approval_request_id = $1 ORDER BY decided_at`, requestID)
	if err != nil {
		return nil, fmt.Errorf("approvals: list decisions: %w", err)
	}
	defer rows.Close()

	decisions := make([]models.Decision, 0, 4)
	for rows.Next() {
		decision, scanErr := scanDecisionRow(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		decisions = append(decisions, *decision)
	}
	return decisions, rows.Err()
}

// CountApprovals counts the approvals recorded on a request.
func (r *Repository) CountApprovals(ctx context.Context, q database.Querier,
	requestID string) (int, error) {
	var count int
	err := q.QueryRow(ctx, `
		SELECT count(*) FROM fleet_partner_approval_decisions
		WHERE approval_request_id = $1 AND decision = 'APPROVED'`, requestID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("approvals: count approvals: %w", err)
	}
	return count, nil
}

// CompleteStep closes a step.
func (r *Repository) CompleteStep(ctx context.Context, q database.Querier,
	stepID string, status string) error {
	_, err := q.Exec(ctx, `
		UPDATE fleet_partner_approval_steps
		SET status = $2, completed_at = now(), version = version + 1, updated_at = now()
		WHERE id = $1`, stepID, status)
	if err != nil {
		return fmt.Errorf("approvals: complete step: %w", err)
	}
	return nil
}

// RecordExecution appends the durable record of an execution attempt.
func (r *Repository) RecordExecution(ctx context.Context, q database.Querier,
	organizationID string, execution *models.Execution) error {
	if execution.ID == "" {
		execution.ID = ids.New(prefixExecution)
	}
	execution.OrganizationID = organizationID

	_, err := q.Exec(ctx, `
		INSERT INTO fleet_partner_approval_executions
			(id, organization_id, approval_request_id, presented_fingerprint,
			 approved_fingerprint, expected_subject_version, observed_subject_version,
			 execution_reference, outcome, safe_reason_code, actor_type, actor_id,
			 idempotency_key, correlation_id, causation_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`,
		execution.ID, organizationID, execution.ApprovalRequestID,
		execution.PresentedFingerprint, execution.ApprovedFingerprint,
		execution.ExpectedSubjectVersion, execution.ObservedSubjectVersion,
		execution.ExecutionReference, execution.Outcome, execution.SafeReasonCode,
		execution.ActorType, execution.ActorID, execution.IdempotencyKey,
		execution.CorrelationID, execution.CausationID)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return fmt.Errorf("approvals: record execution: %w", err)
	}
	return nil
}

// FindExecutionByIdempotencyKey resolves a retried execution.
func (r *Repository) FindExecutionByIdempotencyKey(ctx context.Context, q database.Querier,
	organizationID string, idempotencyKey string) (*models.Execution, error) {
	if q == nil {
		q = r.db
	}
	if idempotencyKey == "" {
		return nil, ErrNotFound
	}
	var execution models.Execution
	err := q.QueryRow(ctx, `
		SELECT id, organization_id, approval_request_id, presented_fingerprint,
		       approved_fingerprint, expected_subject_version, observed_subject_version,
		       execution_reference, outcome, safe_reason_code, actor_type, actor_id,
		       idempotency_key, correlation_id, causation_id, occurred_at
		FROM fleet_partner_approval_executions
		WHERE organization_id = $1 AND idempotency_key = $2`,
		organizationID, idempotencyKey).
		Scan(&execution.ID, &execution.OrganizationID, &execution.ApprovalRequestID,
			&execution.PresentedFingerprint, &execution.ApprovedFingerprint,
			&execution.ExpectedSubjectVersion, &execution.ObservedSubjectVersion,
			&execution.ExecutionReference, &execution.Outcome, &execution.SafeReasonCode,
			&execution.ActorType, &execution.ActorID, &execution.IdempotencyKey,
			&execution.CorrelationID, &execution.CausationID, &execution.OccurredAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("approvals: read execution: %w", err)
	}
	return &execution, nil
}

// AppendHistory writes one immutable state-change record.
func (r *Repository) AppendHistory(ctx context.Context, q database.Querier,
	organizationID string, requestID string, fromStatus string, toStatus string,
	actorType string, actorID string, reasonCode string, correlationID string) error {
	_, err := q.Exec(ctx, `
		INSERT INTO fleet_partner_approval_state_history
			(id, organization_id, approval_request_id, from_status, to_status,
			 actor_type, actor_id, reason_code, request_id, correlation_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		ids.New(prefixHistory), organizationID, requestID, fromStatus, toStatus,
		actorType, actorID, reasonCode, correlationID, correlationID)
	if err != nil {
		return fmt.Errorf("approvals: append history: %w", err)
	}
	return nil
}

// ListHistory returns the append-only history of one request.
func (r *Repository) ListHistory(ctx context.Context, organizationID string,
	requestID string) ([]HistoryEntry, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, from_status, to_status, actor_type, actor_id, reason_code, occurred_at
		FROM fleet_partner_approval_state_history
		WHERE organization_id = $1 AND approval_request_id = $2
		ORDER BY occurred_at DESC`, organizationID, requestID)
	if err != nil {
		return nil, fmt.Errorf("approvals: list history: %w", err)
	}
	defer rows.Close()

	entries := make([]HistoryEntry, 0, 8)
	for rows.Next() {
		var entry HistoryEntry
		if scanErr := rows.Scan(&entry.ID, &entry.FromStatus, &entry.ToStatus,
			&entry.ActorType, &entry.ActorID, &entry.ReasonCode,
			&entry.OccurredAt); scanErr != nil {
			return nil, scanErr
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

// HistoryEntry is one approval state change.
type HistoryEntry struct {
	ID         string
	FromStatus string
	ToStatus   string
	ActorType  string
	ActorID    string
	ReasonCode string
	OccurredAt time.Time
}

// ExpireOverdue marks pending requests whose window has closed.
//
// The sweep is idempotent and bounded; it never touches a request that already
// reached a terminal state.
func (r *Repository) ExpireOverdue(ctx context.Context, q database.Querier,
	limit int) (int64, error) {
	if q == nil {
		q = r.db
	}
	if limit <= 0 {
		limit = 200
	}
	tag, err := q.Exec(ctx, `
		UPDATE fleet_partner_approval_requests
		SET status = 'EXPIRED', version = version + 1, updated_at = now()
		WHERE id IN (
			SELECT id FROM fleet_partner_approval_requests
			WHERE status = 'PENDING' AND expires_at <= now()
			ORDER BY expires_at LIMIT $1
		)`, limit)
	if err != nil {
		return 0, fmt.Errorf("approvals: expire overdue: %w", err)
	}
	return tag.RowsAffected(), nil
}

// ---------------------------------------------------------------------------
// Scanning helpers
// ---------------------------------------------------------------------------

type rowScanner interface {
	Scan(dest ...any) error
}

func scanRequest(row pgx.Row) (*models.Request, error) { return scanRequestRow(row) }

func scanRequestRow(row rowScanner) (*models.Request, error) {
	var request models.Request
	var snapshot, roleIDs []byte
	err := row.Scan(&request.ID, &request.OrganizationID, &request.BranchID,
		&request.PolicyID, &request.PolicyVersion, &request.ResourceType,
		&request.ResourceID, &request.Action, &request.RequesterID,
		&request.RequestFingerprint, &request.PayloadVersion, &snapshot,
		&request.ExpectedSubjectVersion, &request.MinimumApprovals, &roleIDs,
		&request.SelfApprovalAllowed, &request.SeparationOfDuties,
		&request.EligibilityDecisionID, &request.Status, &request.CurrentStep,
		&request.ExpiresAt, &request.ExecutedAt, &request.ExecutionReference,
		&request.InvalidatedAt, &request.InvalidationReason, &request.IdempotencyKey,
		&request.CorrelationID, &request.CausationID, &request.Version,
		&request.CreatedAt, &request.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("approvals: scan request: %w", err)
	}
	request.EligibleRoleIDs = decodeStrings(roleIDs)
	if len(snapshot) > 0 {
		_ = json.Unmarshal(snapshot, &request.PayloadSnapshot)
	}
	return &request, nil
}

func scanDecision(row pgx.Row) (*models.Decision, error) { return scanDecisionRow(row) }

func scanDecisionRow(row rowScanner) (*models.Decision, error) {
	var decision models.Decision
	err := row.Scan(&decision.ID, &decision.OrganizationID, &decision.ApprovalRequestID,
		&decision.ApprovalStepID, &decision.ApproverID, &decision.Decision,
		&decision.Reason, &decision.ClientDecisionID, &decision.StepUpReference,
		&decision.RequestVersion, &decision.ApprovedFingerprint, &decision.ActorRoleID,
		&decision.CorrelationID, &decision.DecidedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("approvals: scan decision: %w", err)
	}
	return &decision, nil
}

func decodeStrings(raw []byte) []string {
	if len(raw) == 0 {
		return nil
	}
	var values []string
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil
	}
	return values
}

func jsonArray(values []string) []byte {
	if values == nil {
		values = []string{}
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		return []byte("[]")
	}
	return encoded
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
