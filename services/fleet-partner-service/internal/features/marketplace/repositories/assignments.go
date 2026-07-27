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
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/concurrency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// ErrAssignmentConflict means another live assignment already exists for the
// request, or a reservation is already held by another assignment.
var ErrAssignmentConflict = errors.New("marketplace: assignment conflict")

// ErrInvalidTransition means the assignment state machine forbids the move.
var ErrInvalidTransition = errors.New("marketplace: invalid assignment transition")

// ErrResponseExists means the assignment already carries a provider response.
var ErrResponseExists = errors.New("marketplace: assignment already has a provider response")

// AssignmentRepository stores assignments, responses, history and reservations.
type AssignmentRepository struct {
	db database.Querier
}

// NewAssignmentRepository wires the repository.
func NewAssignmentRepository(db database.Querier) *AssignmentRepository {
	return &AssignmentRepository{db: db}
}

const assignmentColumns = `id, organization_id, request_projection_id, offer_id, membership_id,
	executing_provider_id, source_service, asset_reference, assigned_by_type, assigned_by_id,
	status, response_deadline, accepted_at, declined_at, expired_at, cancelled_at,
	ready_for_execution_at, started_at, completed_at, root_assignment_id, replaces_assignment_id,
	reassignment_attempt, reassignment_reason, execution_start_status, source_execution_id,
	version, created_at, updated_at`

// Create inserts an assignment.
//
// The partial unique index on request_projection_id enforces one live
// assignment per request: a second concurrent dispatcher loses here rather
// than creating a duplicate provider decision.
func (r *AssignmentRepository) Create(ctx context.Context, q database.Querier, organizationID string, assignment *models.Assignment) error {
	if assignment.ID == "" {
		assignment.ID = ids.New("asg")
	}
	assignment.OrganizationID = organizationID
	assignment.Version = 1
	if assignment.Status == "" {
		assignment.Status = models.AssignmentCreated
	}
	if assignment.ExecutionStartStatus == "" {
		assignment.ExecutionStartStatus = models.ExecutionNotRequested
	}
	if assignment.RootAssignmentID == nil {
		assignment.RootAssignmentID = &assignment.ID
	}

	_, err := q.Exec(ctx, `
		INSERT INTO fleet_partner_provider_assignments
			(id, organization_id, request_projection_id, offer_id, membership_id,
			 executing_provider_id, source_service, asset_reference, assigned_by_type,
			 assigned_by_id, status, response_deadline, root_assignment_id,
			 replaces_assignment_id, reassignment_attempt, reassignment_reason,
			 execution_start_status, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, 1, now(), now())`,
		assignment.ID, organizationID, assignment.RequestProjectionID, assignment.OfferID,
		assignment.MembershipID, assignment.ExecutingProviderID, assignment.SourceService,
		assignment.AssetReference, assignment.AssignedByType, assignment.AssignedByID,
		assignment.Status, assignment.ResponseDeadline, assignment.RootAssignmentID,
		assignment.ReplacesAssignmentID, assignment.ReassignmentAttempt,
		assignment.ReassignmentReason, assignment.ExecutionStartStatus)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrAssignmentConflict
		}
		return fmt.Errorf("marketplace: create assignment: %w", err)
	}
	return nil
}

// GetByID loads an assignment inside the organization scope.
func (r *AssignmentRepository) GetByID(ctx context.Context, organizationID string, assignmentID string) (*models.Assignment, error) {
	row := r.db.QueryRow(ctx, `SELECT `+assignmentColumns+`
		FROM fleet_partner_provider_assignments
		WHERE organization_id = $1 AND id = $2`, organizationID, assignmentID)
	return scanAssignment(row)
}

// ListForProvider lists assignments belonging to one provider.
//
// Scoped by executing_provider_id and source service, so a provider can only
// enumerate their own work regardless of which organization issued it.
func (r *AssignmentRepository) ListForProvider(ctx context.Context, executingProviderID string,
	sourceService string, limit int, offset int) ([]models.Assignment, error) {
	rows, err := r.db.Query(ctx, `SELECT `+assignmentColumns+`
		FROM fleet_partner_provider_assignments
		WHERE executing_provider_id = $1 AND source_service = $2
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4`, executingProviderID, sourceService, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("marketplace: list assignments for provider: %w", err)
	}
	defer rows.Close()

	var assignments []models.Assignment
	for rows.Next() {
		assignment, scanErr := scanAssignmentRows(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		assignments = append(assignments, *assignment)
	}
	return assignments, rows.Err()
}

// GetForProvider loads one assignment owned by this provider.
func (r *AssignmentRepository) GetForProvider(ctx context.Context, assignmentID string,
	executingProviderID string, sourceService string) (*models.Assignment, error) {
	row := r.db.QueryRow(ctx, `SELECT `+assignmentColumns+`
		FROM fleet_partner_provider_assignments
		WHERE id = $1 AND executing_provider_id = $2 AND source_service = $3`,
		assignmentID, executingProviderID, sourceService)
	return scanAssignment(row)
}

// GetByIDUnscoped loads an assignment without an organization predicate.
//
// Almost every read in this service is tenant-scoped, and deliberately so.
// This one is not, because the caller is the source execution service asking
// "did the provider on assignment X accept?" — it holds the assignment id from
// its own command and does not know the organization. The organization is
// RETURNED so the caller can bind it against its own reservation; it is never
// accepted as input here. Do not use this for any organization-facing path.
func (r *AssignmentRepository) GetByIDUnscoped(ctx context.Context, assignmentID string) (*models.Assignment, error) {
	row := r.db.QueryRow(ctx, `SELECT `+assignmentColumns+`
		FROM fleet_partner_provider_assignments
		WHERE id = $1`, assignmentID)
	return scanAssignment(row)
}

// LockByID loads an assignment FOR UPDATE. Concurrent accept/decline/expire
// attempts on the same assignment serialize here, so exactly one wins.
func (r *AssignmentRepository) LockByID(ctx context.Context, q database.Querier, assignmentID string) (*models.Assignment, error) {
	row := q.QueryRow(ctx, `SELECT `+assignmentColumns+`
		FROM fleet_partner_provider_assignments WHERE id = $1 FOR UPDATE`, assignmentID)
	return scanAssignment(row)
}

// LockForProvider loads an assignment FOR UPDATE and confirms the responding
// provider owns it. Ownership is checked in SQL rather than trusted from the
// request, so a provider can never respond to another provider's assignment.
func (r *AssignmentRepository) LockForProvider(ctx context.Context, q database.Querier, assignmentID string, membershipID string) (*models.Assignment, error) {
	row := q.QueryRow(ctx, `SELECT `+assignmentColumns+`
		FROM fleet_partner_provider_assignments
		WHERE id = $1 AND membership_id = $2 FOR UPDATE`, assignmentID, membershipID)
	return scanAssignment(row)
}

// List returns assignments for a tenant query, honouring branch scope through
// the provider membership's home branch.
func (r *AssignmentRepository) List(ctx context.Context, query tenant.Query, status string) ([]models.Assignment, error) {
	if query.OrganizationID == "" {
		return nil, fmt.Errorf("marketplace: organization id is required")
	}
	limit := query.Limit
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	branches := orEmptySlice(query.PermittedBranchIDs)
	rows, err := r.db.Query(ctx, `
		SELECT a.id, a.organization_id, a.request_projection_id, a.offer_id, a.membership_id,
			a.executing_provider_id, a.source_service, a.asset_reference, a.assigned_by_type,
			a.assigned_by_id, a.status, a.response_deadline, a.accepted_at, a.declined_at,
			a.expired_at, a.cancelled_at, a.ready_for_execution_at, a.started_at, a.completed_at,
			a.root_assignment_id, a.replaces_assignment_id, a.reassignment_attempt,
			a.reassignment_reason, a.execution_start_status, a.source_execution_id,
			a.version, a.created_at, a.updated_at
		FROM fleet_partner_provider_assignments a
		JOIN fleet_partner_organization_provider_memberships m ON m.id = a.membership_id
		WHERE a.organization_id = $1
		  AND ($2 = '' OR a.status = $2)
		  AND (cardinality($3::text[]) = 0 OR m.home_branch_id IS NULL OR m.home_branch_id = ANY($3::text[]))
		ORDER BY a.created_at DESC, a.id
		LIMIT $4`, query.OrganizationID, status, branches, limit)
	if err != nil {
		return nil, fmt.Errorf("marketplace: list assignments: %w", err)
	}
	defer rows.Close()

	var assignments []models.Assignment
	for rows.Next() {
		assignment, err := scanAssignmentRows(rows)
		if err != nil {
			return nil, err
		}
		assignments = append(assignments, *assignment)
	}
	return assignments, rows.Err()
}

// ListForMembership returns a provider's own assignments.
func (r *AssignmentRepository) ListForMembership(ctx context.Context, organizationID string, membershipID string, limit int) ([]models.Assignment, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	rows, err := r.db.Query(ctx, `SELECT `+assignmentColumns+`
		FROM fleet_partner_provider_assignments
		WHERE organization_id = $1 AND membership_id = $2
		ORDER BY created_at DESC, id LIMIT $3`, organizationID, membershipID, limit)
	if err != nil {
		return nil, fmt.Errorf("marketplace: list provider assignments: %w", err)
	}
	defer rows.Close()

	var assignments []models.Assignment
	for rows.Next() {
		assignment, err := scanAssignmentRows(rows)
		if err != nil {
			return nil, err
		}
		assignments = append(assignments, *assignment)
	}
	return assignments, rows.Err()
}

// Transition applies a validated state-machine move with compare-and-swap and
// appends append-only history.
//
// This is the only path that changes assignment status: it validates the
// transition, checks the expected version, and records who did it.
func (r *AssignmentRepository) Transition(ctx context.Context, q database.Querier, current *models.Assignment, toStatus string, expectedVersion int64, entry models.StateHistoryEntry) error {
	if !models.CanTransition(current.Status, toStatus) {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, current.Status, toStatus)
	}

	tag, err := q.Exec(ctx, `
		UPDATE fleet_partner_provider_assignments SET
			status = $2,
			accepted_at = CASE WHEN $2 = 'ACCEPTED' THEN COALESCE(accepted_at, now()) ELSE accepted_at END,
			declined_at = CASE WHEN $2 = 'DECLINED' THEN now() ELSE declined_at END,
			expired_at = CASE WHEN $2 = 'EXPIRED' THEN now() ELSE expired_at END,
			cancelled_at = CASE WHEN $2 = 'CANCELLED' THEN now() ELSE cancelled_at END,
			ready_for_execution_at = CASE WHEN $2 = 'READY_FOR_EXECUTION' THEN COALESCE(ready_for_execution_at, now()) ELSE ready_for_execution_at END,
			started_at = CASE WHEN $2 = 'IN_PROGRESS' THEN COALESCE(started_at, now()) ELSE started_at END,
			completed_at = CASE WHEN $2 = 'COMPLETED' THEN now() ELSE completed_at END,
			version = version + 1, updated_at = now()
		WHERE id = $1 AND version = $3`,
		current.ID, toStatus, expectedVersion)
	if err != nil {
		return fmt.Errorf("marketplace: transition assignment: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return concurrency.ErrVersionMismatch
	}

	metadata, err := json.Marshal(orEmptyMap(entry.Metadata))
	if err != nil {
		return fmt.Errorf("marketplace: marshal history metadata: %w", err)
	}
	_, err = q.Exec(ctx, `
		INSERT INTO fleet_partner_assignment_state_history
			(id, organization_id, assignment_id, from_status, to_status, actor_type, actor_id,
			 reason_code, metadata, request_id, correlation_id, occurred_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, now())`,
		ids.New("ash"), current.OrganizationID, current.ID, current.Status, toStatus,
		entry.ActorType, entry.ActorID, entry.ReasonCode, metadata,
		entry.RequestID, entry.CorrelationID)
	if err != nil {
		return fmt.Errorf("marketplace: append assignment history: %w", err)
	}
	return nil
}

// SetExecutionStatus records the outcome of the external execution command.
func (r *AssignmentRepository) SetExecutionStatus(ctx context.Context, q database.Querier, assignmentID string, status string, sourceExecutionID string) error {
	var executionID *string
	if sourceExecutionID != "" {
		executionID = &sourceExecutionID
	}
	_, err := q.Exec(ctx, `
		UPDATE fleet_partner_provider_assignments SET
			execution_start_status = $2,
			source_execution_id = COALESCE($3, source_execution_id),
			updated_at = now()
		WHERE id = $1`, assignmentID, status, executionID)
	if err != nil {
		return fmt.Errorf("marketplace: set execution status: %w", err)
	}
	return nil
}

// ListHistory returns the append-only assignment history.
func (r *AssignmentRepository) ListHistory(ctx context.Context, organizationID string, assignmentID string) ([]models.StateHistoryEntry, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, organization_id, assignment_id, from_status, to_status, actor_type, actor_id,
			reason_code, metadata, request_id, correlation_id, occurred_at
		FROM fleet_partner_assignment_state_history
		WHERE organization_id = $1 AND assignment_id = $2
		ORDER BY occurred_at DESC, id`, organizationID, assignmentID)
	if err != nil {
		return nil, fmt.Errorf("marketplace: list assignment history: %w", err)
	}
	defer rows.Close()

	var entries []models.StateHistoryEntry
	for rows.Next() {
		var entry models.StateHistoryEntry
		var metadata []byte
		if err := rows.Scan(&entry.ID, &entry.OrganizationID, &entry.AssignmentID,
			&entry.FromStatus, &entry.ToStatus, &entry.ActorType, &entry.ActorID,
			&entry.ReasonCode, &metadata, &entry.RequestID, &entry.CorrelationID,
			&entry.OccurredAt); err != nil {
			return nil, fmt.Errorf("marketplace: scan history: %w", err)
		}
		_ = json.Unmarshal(metadata, &entry.Metadata)
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

// ClaimExpired leases overdue pending assignments for the expiry worker.
//
// It uses FOR UPDATE SKIP LOCKED and re-checks the deadline in SQL, so an
// assignment is never expired early and two workers never expire the same one.
func (r *AssignmentRepository) ClaimExpired(ctx context.Context, q database.Querier, limit int) ([]models.Assignment, error) {
	rows, err := q.Query(ctx, `SELECT `+assignmentColumns+`
		FROM fleet_partner_provider_assignments
		WHERE status = 'PENDING_PROVIDER_RESPONSE' AND response_deadline < now()
		ORDER BY response_deadline
		LIMIT $1
		FOR UPDATE SKIP LOCKED`, limit)
	if err != nil {
		return nil, fmt.Errorf("marketplace: claim expired assignments: %w", err)
	}
	defer rows.Close()

	var assignments []models.Assignment
	for rows.Next() {
		assignment, err := scanAssignmentRows(rows)
		if err != nil {
			return nil, err
		}
		assignments = append(assignments, *assignment)
	}
	return assignments, rows.Err()
}

// ---- Responses ----

// CreateResponse writes the immutable provider decision.
//
// The unique constraint on assignment_id guarantees exactly one response ever
// exists; a duplicate client_response_id from a retrying mobile client is
// caught by its own unique constraint and reported as a replay.
func (r *AssignmentRepository) CreateResponse(ctx context.Context, q database.Querier, response *models.Response) error {
	if response.ID == "" {
		response.ID = ids.New("ars")
	}
	_, err := q.Exec(ctx, `
		INSERT INTO fleet_partner_assignment_responses
			(id, organization_id, assignment_id, membership_id, response, reason_code, reason_text,
			 safety_related, client_response_id, device_recorded_at, occurred_at, request_id, correlation_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, now(), $11, $12)`,
		response.ID, response.OrganizationID, response.AssignmentID, response.MembershipID,
		response.Response, response.ReasonCode, response.ReasonText, response.SafetyRelated,
		response.ClientResponseID, response.DeviceRecordedAt, response.RequestID, response.CorrelationID)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrResponseExists
		}
		return fmt.Errorf("marketplace: create response: %w", err)
	}

	for _, fileID := range response.EvidenceFileIDs {
		if _, err := q.Exec(ctx, `
			INSERT INTO fleet_partner_assignment_response_evidence
				(id, organization_id, response_id, file_id, created_at)
			VALUES ($1, $2, $3, $4, now()) ON CONFLICT DO NOTHING`,
			ids.New("are"), response.OrganizationID, response.ID, fileID); err != nil {
			return fmt.Errorf("marketplace: attach response evidence: %w", err)
		}
	}
	return nil
}

// GetResponse loads the response for an assignment.
func (r *AssignmentRepository) GetResponse(ctx context.Context, q database.Querier, assignmentID string) (*models.Response, error) {
	if q == nil {
		q = r.db
	}
	var response models.Response
	err := q.QueryRow(ctx, `
		SELECT id, organization_id, assignment_id, membership_id, response, reason_code,
			reason_text, safety_related, client_response_id, device_recorded_at, occurred_at,
			request_id, correlation_id
		FROM fleet_partner_assignment_responses WHERE assignment_id = $1`, assignmentID).
		Scan(&response.ID, &response.OrganizationID, &response.AssignmentID, &response.MembershipID,
			&response.Response, &response.ReasonCode, &response.ReasonText, &response.SafetyRelated,
			&response.ClientResponseID, &response.DeviceRecordedAt, &response.OccurredAt,
			&response.RequestID, &response.CorrelationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("marketplace: scan response: %w", err)
	}
	return &response, nil
}

// FindResponseByClientID resolves a replayed mobile response so a retry
// returns the original result instead of failing.
func (r *AssignmentRepository) FindResponseByClientID(ctx context.Context, membershipID string, clientResponseID string) (*models.Response, error) {
	var response models.Response
	err := r.db.QueryRow(ctx, `
		SELECT id, organization_id, assignment_id, membership_id, response, reason_code,
			reason_text, safety_related, client_response_id, device_recorded_at, occurred_at,
			request_id, correlation_id
		FROM fleet_partner_assignment_responses
		WHERE membership_id = $1 AND client_response_id = $2`, membershipID, clientResponseID).
		Scan(&response.ID, &response.OrganizationID, &response.AssignmentID, &response.MembershipID,
			&response.Response, &response.ReasonCode, &response.ReasonText, &response.SafetyRelated,
			&response.ClientResponseID, &response.DeviceRecordedAt, &response.OccurredAt,
			&response.RequestID, &response.CorrelationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("marketplace: scan response by client id: %w", err)
	}
	return &response, nil
}

// ---- Reservations ----

// HoldReservation reserves provider or asset capacity for an assignment.
// The partial unique index rejects a second live hold on the same resource.
func (r *AssignmentRepository) HoldReservation(ctx context.Context, q database.Querier, organizationID string, assignmentID string, resourceType string, resourceID string) error {
	_, err := q.Exec(ctx, `
		INSERT INTO fleet_partner_assignment_reservations
			(id, organization_id, assignment_id, resource_type, resource_id, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, 'HELD', now(), now())`,
		ids.New("rsv"), organizationID, assignmentID, resourceType, resourceID)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrAssignmentConflict
		}
		return fmt.Errorf("marketplace: hold reservation: %w", err)
	}
	return nil
}

// ReleaseReservations frees every hold for an assignment. Called whenever an
// assignment resolves without execution (decline, expiry, cancellation) so
// capacity is never leaked.
func (r *AssignmentRepository) ReleaseReservations(ctx context.Context, q database.Querier, assignmentID string) error {
	_, err := q.Exec(ctx, `
		UPDATE fleet_partner_assignment_reservations SET
			status = 'RELEASED', released_at = now(), updated_at = now()
		WHERE assignment_id = $1 AND status = 'HELD'`, assignmentID)
	if err != nil {
		return fmt.Errorf("marketplace: release reservations: %w", err)
	}
	return nil
}

// CountHeld reports live holds for a resource; used by eligibility checks.
func (r *AssignmentRepository) CountHeld(ctx context.Context, resourceType string, resourceID string) (int, error) {
	var count int
	err := r.db.QueryRow(ctx, `
		SELECT count(*) FROM fleet_partner_assignment_reservations
		WHERE resource_type = $1 AND resource_id = $2 AND status = 'HELD'`,
		resourceType, resourceID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("marketplace: count reservations: %w", err)
	}
	return count, nil
}

func scanAssignment(row pgx.Row) (*models.Assignment, error) {
	var assignment models.Assignment
	err := row.Scan(&assignment.ID, &assignment.OrganizationID, &assignment.RequestProjectionID,
		&assignment.OfferID, &assignment.MembershipID, &assignment.ExecutingProviderID,
		&assignment.SourceService, &assignment.AssetReference, &assignment.AssignedByType,
		&assignment.AssignedByID, &assignment.Status, &assignment.ResponseDeadline,
		&assignment.AcceptedAt, &assignment.DeclinedAt, &assignment.ExpiredAt,
		&assignment.CancelledAt, &assignment.ReadyForExecutionAt, &assignment.StartedAt,
		&assignment.CompletedAt, &assignment.RootAssignmentID, &assignment.ReplacesAssignmentID,
		&assignment.ReassignmentAttempt, &assignment.ReassignmentReason,
		&assignment.ExecutionStartStatus, &assignment.SourceExecutionID,
		&assignment.Version, &assignment.CreatedAt, &assignment.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("marketplace: scan assignment: %w", err)
	}
	return &assignment, nil
}

func scanAssignmentRows(rows pgx.Rows) (*models.Assignment, error) {
	var assignment models.Assignment
	err := rows.Scan(&assignment.ID, &assignment.OrganizationID, &assignment.RequestProjectionID,
		&assignment.OfferID, &assignment.MembershipID, &assignment.ExecutingProviderID,
		&assignment.SourceService, &assignment.AssetReference, &assignment.AssignedByType,
		&assignment.AssignedByID, &assignment.Status, &assignment.ResponseDeadline,
		&assignment.AcceptedAt, &assignment.DeclinedAt, &assignment.ExpiredAt,
		&assignment.CancelledAt, &assignment.ReadyForExecutionAt, &assignment.StartedAt,
		&assignment.CompletedAt, &assignment.RootAssignmentID, &assignment.ReplacesAssignmentID,
		&assignment.ReassignmentAttempt, &assignment.ReassignmentReason,
		&assignment.ExecutionStartStatus, &assignment.SourceExecutionID,
		&assignment.Version, &assignment.CreatedAt, &assignment.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("marketplace: scan assignment: %w", err)
	}
	return &assignment, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

var _ = time.Now

// CountHeldExcluding reports live holds for a resource, ignoring one
// assignment. Used when re-checking eligibility for the assignment that
// already owns the hold.
func (r *AssignmentRepository) CountHeldExcluding(ctx context.Context, resourceType string, resourceID string, excludeAssignmentID string) (int, error) {
	var count int
	err := r.db.QueryRow(ctx, `
		SELECT count(*) FROM fleet_partner_assignment_reservations
		WHERE resource_type = $1 AND resource_id = $2 AND status = 'HELD'
		  AND ($3 = '' OR assignment_id <> $3)`,
		resourceType, resourceID, excludeAssignmentID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("marketplace: count reservations excluding: %w", err)
	}
	return count, nil
}
