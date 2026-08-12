package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

// ExecutionStartRepository stores durable execution-start work.
type ExecutionStartRepository struct {
	db *pgxpool.Pool
}

// NewExecutionStartRepository builds the repository.
func NewExecutionStartRepository(db *pgxpool.Pool) *ExecutionStartRepository {
	return &ExecutionStartRepository{db: db}
}

const executionStartColumns = `id, organization_id, assignment_id, provider_response_id,
	request_projection_id, source_service, idempotency_key, request_fingerprint, status,
	attempt_count, next_attempt_at, lease_owner, lease_expires_at,
	dispatch_response_snapshot, trip_id, last_error, version, created_at, updated_at, completed_at`

func scanExecutionStart(row pgx.Row) (*models.ExecutionStartOperation, error) {
	var op models.ExecutionStartOperation
	err := row.Scan(&op.ID, &op.OrganizationID, &op.AssignmentID, &op.ProviderResponseID,
		&op.RequestProjectionID, &op.SourceService, &op.IdempotencyKey, &op.RequestFingerprint,
		&op.Status, &op.AttemptCount, &op.NextAttemptAt, &op.LeaseOwner, &op.LeaseExpiresAt,
		&op.DispatchResponseSnapshot, &op.TripID, &op.LastError, &op.Version,
		&op.CreatedAt, &op.UpdatedAt, &op.CompletedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("marketplace: scan execution start: %w", err)
	}
	return &op, nil
}

// Create queues execution-start work.
//
// This runs in the SAME transaction as the provider's acceptance. If it were
// a follow-up write, a crash between the two would leave a provider who had
// accepted with no work queued and no trip ever created.
func (r *ExecutionStartRepository) Create(ctx context.Context, q database.Querier,
	op *models.ExecutionStartOperation) error {
	if op.ID == "" {
		op.ID = ids.New("exs")
	}
	_, err := q.Exec(ctx, `
		INSERT INTO fleet_partner_execution_start_operations
			(id, organization_id, assignment_id, provider_response_id, request_projection_id,
			 source_service, idempotency_key, request_fingerprint, status, attempt_count,
			 next_attempt_at, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'PENDING', 0, now(), 1, now(), now())
		ON CONFLICT (assignment_id) DO NOTHING`,
		op.ID, op.OrganizationID, op.AssignmentID, op.ProviderResponseID,
		op.RequestProjectionID, op.SourceService, op.IdempotencyKey, op.RequestFingerprint)
	if err != nil {
		return fmt.Errorf("marketplace: create execution start: %w", err)
	}
	return nil
}

// GetByAssignment loads the operation for an assignment.
func (r *ExecutionStartRepository) GetByAssignment(ctx context.Context, assignmentID string) (*models.ExecutionStartOperation, error) {
	return scanExecutionStart(r.db.QueryRow(ctx,
		`SELECT `+executionStartColumns+` FROM fleet_partner_execution_start_operations
		 WHERE assignment_id = $1`, assignmentID))
}

// Claim leases due operations for one worker.
//
// FOR UPDATE SKIP LOCKED lets several worker replicas run without blocking on
// each other, and the lease means a worker that dies mid-flight releases its
// work by timeout rather than holding it forever.
func (r *ExecutionStartRepository) Claim(ctx context.Context, q database.Querier,
	owner string, lease time.Duration, limit int) ([]models.ExecutionStartOperation, error) {
	return r.ClaimEnabled(ctx, q, owner, lease, limit, []string{models.SourceDispatch, models.SourceTaxi, models.SourceHauling})
}

// ClaimEnabled leases only operations whose durable source identity is
// currently enabled. Disabled sources remain pending and consume no attempts.
func (r *ExecutionStartRepository) ClaimEnabled(ctx context.Context, q database.Querier,
	owner string, lease time.Duration, limit int, enabledSources []string) ([]models.ExecutionStartOperation, error) {
	if len(enabledSources) == 0 {
		return nil, nil
	}
	rows, err := q.Query(ctx, `
		WITH claimed AS (
			SELECT id, status AS previous_status FROM fleet_partner_execution_start_operations
			WHERE status IN ('PENDING', 'RETRY_SCHEDULED', 'UNKNOWN_OUTCOME')
			  AND source_service = ANY($4)
			  AND next_attempt_at <= now()
			ORDER BY next_attempt_at
			FOR UPDATE SKIP LOCKED
			LIMIT $3
		)
		UPDATE fleet_partner_execution_start_operations o SET
			status = 'CLAIMED',
			lease_owner = $1,
			lease_expires_at = now() + make_interval(secs => $2),
			version = o.version + 1,
			updated_at = now()
		FROM claimed
		WHERE o.id = claimed.id
		RETURNING `+prefixed(executionStartColumns, "o")+`, claimed.previous_status`, owner, lease.Seconds(), limit, enabledSources)
	if err != nil {
		return nil, fmt.Errorf("marketplace: claim execution start: %w", err)
	}
	defer rows.Close()

	var claimed []models.ExecutionStartOperation
	for rows.Next() {
		var op models.ExecutionStartOperation
		if err := rows.Scan(&op.ID, &op.OrganizationID, &op.AssignmentID, &op.ProviderResponseID,
			&op.RequestProjectionID, &op.SourceService, &op.IdempotencyKey, &op.RequestFingerprint,
			&op.Status, &op.AttemptCount, &op.NextAttemptAt, &op.LeaseOwner, &op.LeaseExpiresAt,
			&op.DispatchResponseSnapshot, &op.TripID, &op.LastError, &op.Version,
			&op.CreatedAt, &op.UpdatedAt, &op.CompletedAt, &op.Status); err != nil {
			return nil, fmt.Errorf("marketplace: scan claimed execution start: %w", err)
		}
		claimed = append(claimed, op)
	}
	return claimed, rows.Err()
}

// RecoverExpiredLeases returns abandoned work to the queue.
func (r *ExecutionStartRepository) RecoverExpiredLeases(ctx context.Context, q database.Querier) (int64, error) {
	if q == nil {
		q = r.db
	}
	tag, err := q.Exec(ctx, `
		UPDATE fleet_partner_execution_start_operations SET
			status = CASE status
				WHEN 'EXECUTION_START_PENDING' THEN 'UNKNOWN_OUTCOME'
				ELSE 'RETRY_SCHEDULED'
			END,
			lease_owner = NULL,
			lease_expires_at = NULL,
			updated_at = now()
		WHERE status IN ('CLAIMED', 'EXECUTION_START_PENDING')
		  AND lease_expires_at IS NOT NULL
		  AND lease_expires_at < now()`)
	if err != nil {
		return 0, fmt.Errorf("marketplace: recover execution leases: %w", err)
	}
	return tag.RowsAffected(), nil
}

// SetStatus records progress without losing the idempotency key.
func (r *ExecutionStartRepository) SetStatus(ctx context.Context, q database.Querier,
	id string, status string) error {
	if q == nil {
		q = r.db
	}
	_, err := q.Exec(ctx, `
		UPDATE fleet_partner_execution_start_operations SET
			status = $2, version = version + 1, updated_at = now()
		WHERE id = $1`, id, status)
	if err != nil {
		return fmt.Errorf("marketplace: set execution start status: %w", err)
	}
	return nil
}

// Confirm records a successful canonical execution.
func (r *ExecutionStartRepository) Confirm(ctx context.Context, q database.Querier,
	id string, tripID string, snapshot any) error {
	if q == nil {
		q = r.db
	}
	var encoded []byte
	if snapshot != nil {
		var err error
		encoded, err = json.Marshal(snapshot)
		if err != nil {
			return fmt.Errorf("marketplace: encode dispatch snapshot: %w", err)
		}
	}
	_, err := q.Exec(ctx, `
		UPDATE fleet_partner_execution_start_operations SET
			status = 'CONFIRMED', trip_id = $2, dispatch_response_snapshot = $3,
			lease_owner = NULL, lease_expires_at = NULL, last_error = NULL,
			completed_at = now(), version = version + 1, updated_at = now()
		WHERE id = $1`, id, tripID, encoded)
	if err != nil {
		return fmt.Errorf("marketplace: confirm execution start: %w", err)
	}
	return nil
}

// Fail schedules a retry, or dead-letters an exhausted operation.
//
// permanent=true means the source service gave a definite refusal; retrying it
// would burn the budget on an answer that will never change.
func (r *ExecutionStartRepository) Fail(ctx context.Context, q database.Querier, id string,
	errorCode string, backoff time.Duration, maxAttempts int, permanent bool) error {
	if q == nil {
		q = r.db
	}
	_, err := q.Exec(ctx, `
		UPDATE fleet_partner_execution_start_operations SET
			status = CASE
				WHEN $5 THEN 'REJECTED'
				WHEN attempt_count + 1 >= $4 THEN 'DEAD_LETTER'
				ELSE 'RETRY_SCHEDULED' END,
			attempt_count = attempt_count + 1,
			next_attempt_at = now() + make_interval(secs => $3),
			lease_owner = NULL,
			lease_expires_at = NULL,
			last_error = $2,
			version = version + 1,
			updated_at = now()
		WHERE id = $1`, id, errorCode, backoff.Seconds(), maxAttempts, permanent)
	if err != nil {
		return fmt.Errorf("marketplace: fail execution start: %w", err)
	}
	return nil
}

// MarkUnknown records an ambiguous outcome for read-back resolution.
//
// The operation keeps its original idempotency key, which is what lets the
// next attempt ask "did my earlier command succeed?" rather than issuing a
// new command that could create a second trip.
func (r *ExecutionStartRepository) MarkUnknown(ctx context.Context, q database.Querier,
	id string, errorCode string, backoff time.Duration) error {
	if q == nil {
		q = r.db
	}
	_, err := q.Exec(ctx, `
		UPDATE fleet_partner_execution_start_operations SET
			status = 'UNKNOWN_OUTCOME',
			attempt_count = attempt_count + 1,
			next_attempt_at = now() + make_interval(secs => $3),
			lease_owner = NULL,
			lease_expires_at = NULL,
			last_error = $2,
			version = version + 1,
			updated_at = now()
		WHERE id = $1`, id, errorCode, backoff.Seconds())
	if err != nil {
		return fmt.Errorf("marketplace: mark execution unknown: %w", err)
	}
	return nil
}

// Replay returns a dead-lettered operation to the queue for an operator.
func (r *ExecutionStartRepository) Replay(ctx context.Context, id string) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE fleet_partner_execution_start_operations SET
			status = 'PENDING', attempt_count = 0, next_attempt_at = now(),
			last_error = NULL, version = version + 1, updated_at = now()
		WHERE id = $1 AND status = 'DEAD_LETTER'`, id)
	if err != nil {
		return fmt.Errorf("marketplace: replay execution start: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// CountByStatus supports metrics and smoke tests.
func (r *ExecutionStartRepository) CountByStatus(ctx context.Context, status string) (int, error) {
	var count int
	err := r.db.QueryRow(ctx,
		`SELECT count(*) FROM fleet_partner_execution_start_operations WHERE status = $1`,
		status).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("marketplace: count execution start: %w", err)
	}
	return count, nil
}

// prefixed qualifies a column list with a table alias.
func prefixed(columns string, alias string) string {
	out := ""
	for _, part := range splitColumns(columns) {
		if out != "" {
			out += ", "
		}
		out += alias + "." + part
	}
	return out
}

func splitColumns(columns string) []string {
	var parts []string
	current := ""
	for _, char := range columns {
		switch char {
		case ',':
			parts = append(parts, trimSpace(current))
			current = ""
		case '\n', '\t':
			current += " "
		default:
			current += string(char)
		}
	}
	if trimSpace(current) != "" {
		parts = append(parts, trimSpace(current))
	}
	return parts
}

func trimSpace(value string) string {
	start, end := 0, len(value)
	for start < end && (value[start] == ' ' || value[start] == '\t' || value[start] == '\n') {
		start++
	}
	for end > start && (value[end-1] == ' ' || value[end-1] == '\t' || value[end-1] == '\n') {
		end--
	}
	return value[start:end]
}
