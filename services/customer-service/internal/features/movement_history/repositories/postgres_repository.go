package movementrepositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	movementmodels "cosmicforge/logistics/services/customer-service/internal/features/movement_history/models"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/events"
)

type PostgresRepository struct{ db *pgxpool.Pool }

func NewPostgresRepository(db *pgxpool.Pool) *PostgresRepository { return &PostgresRepository{db: db} }

const columns = `customer_id::text, authority, source_ref, source_version, product_code, movement_type, state,
	origin_summary_json, destination_summary_json, scheduled_at, started_at, completed_at,
	provider_replacement_count, recovery_state, parent_context_type, parent_context_ref,
	fulfilment_plan_ref, fulfilment_group_ref, mission_sequence, mission_count, last_event_id,
	last_event_at, created_at, updated_at`

func scan(row pgx.Row) (movementmodels.Movement, error) {
	var m movementmodels.Movement
	var origin, destination []byte
	err := row.Scan(&m.CustomerID, &m.Authority, &m.SourceRef, &m.SourceVersion, &m.ProductCode, &m.MovementType, &m.State,
		&origin, &destination, &m.ScheduledAt, &m.StartedAt, &m.CompletedAt, &m.ProviderReplacementCount, &m.RecoveryState,
		&m.ParentContextType, &m.ParentContextRef, &m.FulfilmentPlanRef, &m.FulfilmentGroupRef, &m.MissionSequence,
		&m.MissionCount, &m.LastEventID, &m.LastEventAt, &m.CreatedAt, &m.UpdatedAt)
	if len(origin) > 0 {
		m.OriginSummary = json.RawMessage(origin)
	}
	if len(destination) > 0 {
		m.DestinationSummary = json.RawMessage(destination)
	}
	return m, err
}

// Project applies one envelope atomically: inbox dedupe, row lock, pure
// Apply rules, upsert, and outcome record in a single transaction.
func (r *PostgresRepository) Project(ctx context.Context, envelope events.Envelope, incoming movementmodels.Movement) (string, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Serialize every event for one movement so the inbox insert and the
	// projection decision observe a consistent order.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 6606))`, incoming.Authority+"|"+incoming.SourceRef); err != nil {
		return "", err
	}
	var seen bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM customer_movement_event_inbox WHERE event_id=$1)`, envelope.EventID).Scan(&seen); err != nil {
		return "", err
	}
	if seen {
		return movementmodels.OutcomeDuplicate, nil
	}

	var existing *movementmodels.Movement
	current, err := scan(tx.QueryRow(ctx, `SELECT `+columns+` FROM customer_movement_history_projection WHERE authority=$1 AND source_ref=$2 FOR UPDATE`, incoming.Authority, incoming.SourceRef))
	switch {
	case err == nil:
		existing = &current
	case !errors.Is(err, pgx.ErrNoRows):
		return "", err
	}
	next, outcome, err := movementmodels.Apply(existing, incoming)
	if err != nil {
		return "", err
	}
	if outcome == movementmodels.OutcomeApplied {
		_, err = tx.Exec(ctx, `
			INSERT INTO customer_movement_history_projection (customer_id, authority, source_ref, source_version, product_code,
				movement_type, state, origin_summary_json, destination_summary_json, scheduled_at, started_at, completed_at,
				provider_replacement_count, recovery_state, parent_context_type, parent_context_ref, fulfilment_plan_ref,
				fulfilment_group_ref, mission_sequence, mission_count, last_event_id, last_event_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22)
			ON CONFLICT (authority, source_ref) DO UPDATE SET source_version=EXCLUDED.source_version,
				product_code=EXCLUDED.product_code, movement_type=EXCLUDED.movement_type, state=EXCLUDED.state,
				origin_summary_json=EXCLUDED.origin_summary_json, destination_summary_json=EXCLUDED.destination_summary_json,
				scheduled_at=EXCLUDED.scheduled_at, started_at=EXCLUDED.started_at, completed_at=EXCLUDED.completed_at,
				provider_replacement_count=EXCLUDED.provider_replacement_count, recovery_state=EXCLUDED.recovery_state,
				parent_context_type=EXCLUDED.parent_context_type, parent_context_ref=EXCLUDED.parent_context_ref,
				fulfilment_plan_ref=EXCLUDED.fulfilment_plan_ref, fulfilment_group_ref=EXCLUDED.fulfilment_group_ref,
				mission_sequence=EXCLUDED.mission_sequence, mission_count=EXCLUDED.mission_count,
				last_event_id=EXCLUDED.last_event_id, last_event_at=EXCLUDED.last_event_at, updated_at=NOW()`,
			next.CustomerID, next.Authority, next.SourceRef, next.SourceVersion, next.ProductCode, next.MovementType, next.State,
			nullable(next.OriginSummary), nullable(next.DestinationSummary), next.ScheduledAt, next.StartedAt, next.CompletedAt,
			next.ProviderReplacementCount, next.RecoveryState, next.ParentContextType, next.ParentContextRef, next.FulfilmentPlanRef,
			next.FulfilmentGroupRef, next.MissionSequence, next.MissionCount, next.LastEventID, next.LastEventAt)
		if err != nil {
			return "", err
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO customer_movement_event_inbox (event_id, source_service, event_name, authority, source_ref, source_version, outcome) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		envelope.EventID, envelope.SourceService, envelope.EventName, incoming.Authority, incoming.SourceRef, envelope.SourceVersion, outcome); err != nil {
		return "", err
	}
	return outcome, tx.Commit(ctx)
}

// Reject stores a well-formed but refused event for reconciliation.
func (r *PostgresRepository) Reject(ctx context.Context, envelope events.Envelope, reason string) error {
	raw, _ := json.Marshal(envelope)
	_, err := r.db.Exec(ctx, `INSERT INTO customer_movement_event_rejections (event_id, source_service, event_name, reason_code, envelope) VALUES ($1,$2,$3,$4,$5) ON CONFLICT (event_id) DO NOTHING`,
		envelope.EventID, envelope.SourceService, envelope.EventName, reason, raw)
	return err
}

type Filter struct {
	Authority        string
	ProductCode      string
	MovementType     string
	State            string
	From             *time.Time
	To               *time.Time
	ParentContextRef string
	Page             int
	Limit            int
}

func (r *PostgresRepository) List(ctx context.Context, customerID string, filter Filter) ([]movementmodels.Movement, int, error) {
	where := []string{"customer_id=$1"}
	args := []any{customerID}
	add := func(clause string, value any) {
		args = append(args, value)
		where = append(where, fmt.Sprintf(clause, len(args)))
	}
	if filter.Authority != "" {
		add("authority=$%d", filter.Authority)
	}
	if filter.ProductCode != "" {
		add("product_code=$%d", filter.ProductCode)
	}
	if filter.MovementType != "" {
		add("movement_type=$%d", filter.MovementType)
	}
	if filter.State != "" {
		add("state=$%d", filter.State)
	}
	if filter.From != nil {
		add("last_event_at >= $%d", *filter.From)
	}
	if filter.To != nil {
		add("last_event_at < $%d", *filter.To)
	}
	if filter.ParentContextRef != "" {
		add("parent_context_ref=$%d", filter.ParentContextRef)
	}
	clause := strings.Join(where, " AND ")
	var total int
	if err := r.db.QueryRow(ctx, `SELECT count(*) FROM customer_movement_history_projection WHERE `+clause, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, filter.Limit, (filter.Page-1)*filter.Limit)
	rows, err := r.db.Query(ctx, fmt.Sprintf(`SELECT %s FROM customer_movement_history_projection WHERE %s ORDER BY last_event_at DESC, authority, source_ref LIMIT $%d OFFSET $%d`, columns, clause, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]movementmodels.Movement, 0)
	for rows.Next() {
		m, err := scan(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, m)
	}
	return out, total, rows.Err()
}

func (r *PostgresRepository) Get(ctx context.Context, customerID, authority, sourceRef string) (movementmodels.Movement, error) {
	m, err := scan(r.db.QueryRow(ctx, `SELECT `+columns+` FROM customer_movement_history_projection WHERE customer_id=$1 AND authority=$2 AND source_ref=$3`, customerID, authority, sourceRef))
	if errors.Is(err, pgx.ErrNoRows) {
		return m, apperrors.NotFound("Movement not found.", nil)
	}
	return m, err
}

func nullable(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	return []byte(raw)
}
