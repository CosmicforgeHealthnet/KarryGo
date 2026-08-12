package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/cursor"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

const opportunityColumns = `id, organization_id, COALESCE(branch_id, ''), customer_id, title,
	COALESCE(description, ''), stage, expected_value_amount_minor,
	COALESCE(expected_value_currency, ''), probability_basis_points, expected_close_date,
	service_capabilities, COALESCE(account_owner_user_id, ''), COALESCE(next_action, ''),
	next_action_at, COALESCE(loss_reason, ''), COALESCE(hold_reason, ''), version,
	created_by, updated_by, created_at, updated_at, closed_at`

func scanOpportunity(row pgx.Row) (models.Opportunity, error) {
	var o models.Opportunity
	var capabilities []byte
	err := row.Scan(&o.ID, &o.OrganizationID, &o.BranchID, &o.CustomerID, &o.Title,
		&o.Description, &o.Stage, &o.ExpectedValueAmountMinor, &o.ExpectedValueCurrency,
		&o.ProbabilityBasisPoints, &o.ExpectedCloseDate, &capabilities, &o.AccountOwnerUserID,
		&o.NextAction, &o.NextActionAt, &o.LossReason, &o.HoldReason, &o.Version,
		&o.CreatedBy, &o.UpdatedBy, &o.CreatedAt, &o.UpdatedAt, &o.ClosedAt)
	if err != nil {
		return o, scanErr(err)
	}
	o.ServiceCapabilities = decodeStringSlice(capabilities)
	return o, nil
}

// CreateOpportunity inserts a sales opportunity.
func (r *Repository) CreateOpportunity(ctx context.Context, q database.Querier,
	o models.Opportunity) (models.Opportunity, error) {
	capabilities, err := encodeJSONArray(o.ServiceCapabilities)
	if err != nil {
		return models.Opportunity{}, err
	}
	row := r.q(q).QueryRow(ctx, `
		INSERT INTO fleet_partner_sales_opportunities
			(id, organization_id, branch_id, customer_id, title, description, stage,
			 expected_value_amount_minor, expected_value_currency, probability_basis_points,
			 expected_close_date, service_capabilities, account_owner_user_id, next_action,
			 next_action_at, created_by, updated_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $16)
		RETURNING `+opportunityColumns,
		o.ID, o.OrganizationID, nullable(o.BranchID), o.CustomerID, o.Title,
		nullable(o.Description), o.Stage, o.ExpectedValueAmountMinor,
		nullable(o.ExpectedValueCurrency), o.ProbabilityBasisPoints, o.ExpectedCloseDate,
		capabilities, nullable(o.AccountOwnerUserID), nullable(o.NextAction), o.NextActionAt,
		o.CreatedBy)
	created, err := scanOpportunity(row)
	if err != nil {
		return created, classify(err, "create opportunity")
	}
	return created, nil
}

// GetOpportunity reads one opportunity inside the organization.
func (r *Repository) GetOpportunity(ctx context.Context, q database.Querier,
	organizationID, opportunityID string) (models.Opportunity, error) {
	return scanOpportunity(r.q(q).QueryRow(ctx, `SELECT `+opportunityColumns+`
		FROM fleet_partner_sales_opportunities WHERE organization_id = $1 AND id = $2`,
		organizationID, opportunityID))
}

// LockOpportunity reads one opportunity FOR UPDATE.
func (r *Repository) LockOpportunity(ctx context.Context, q database.Querier,
	organizationID, opportunityID string) (models.Opportunity, error) {
	return scanOpportunity(r.q(q).QueryRow(ctx, `SELECT `+opportunityColumns+`
		FROM fleet_partner_sales_opportunities WHERE organization_id = $1 AND id = $2
		FOR UPDATE`, organizationID, opportunityID))
}

// OpportunityFilter is the allowlisted opportunity list query.
type OpportunityFilter struct {
	Stage              string
	CustomerID         string
	BranchID           string
	AccountOwnerUserID string
	CreatedAfter       *time.Time
	CreatedBefore      *time.Time
	Cursor             cursor.Cursor
	HasCursor          bool
	Limit              int
}

// ListOpportunities returns a tenant-scoped page, newest first.
func (r *Repository) ListOpportunities(ctx context.Context, organizationID string,
	branchIDs []string, filter OpportunityFilter) ([]models.Opportunity, error) {
	args := []any{organizationID, filter.Stage, filter.CustomerID, filter.BranchID,
		filter.AccountOwnerUserID}
	where := `organization_id = $1
		AND ($2 = '' OR stage = $2)
		AND ($3 = '' OR customer_id = $3)
		AND ($4 = '' OR branch_id = $4)
		AND ($5 = '' OR account_owner_user_id = $5)`
	if filter.CreatedAfter != nil {
		args = append(args, *filter.CreatedAfter)
		where += fmt.Sprintf(" AND created_at >= $%d", len(args))
	}
	if filter.CreatedBefore != nil {
		args = append(args, *filter.CreatedBefore)
		where += fmt.Sprintf(" AND created_at <= $%d", len(args))
	}
	where, args = pageWhere(where, args, filter.Cursor, filter.HasCursor, "created_at")
	clause, args := branchClause(branchIDs, "branch_id", args)
	args = append(args, cursor.ClampLimit(filter.Limit))

	rows, err := r.db.Query(ctx, `SELECT `+opportunityColumns+`
		FROM fleet_partner_sales_opportunities WHERE `+where+` AND `+clause+`
		ORDER BY created_at DESC, id DESC LIMIT $`+itoa(len(args)), args...)
	if err != nil {
		return nil, classify(err, "list opportunities")
	}
	defer rows.Close()
	out := make([]models.Opportunity, 0)
	for rows.Next() {
		value, scanError := scanOpportunity(rows)
		if scanError != nil {
			return nil, scanError
		}
		out = append(out, value)
	}
	return out, rows.Err()
}

// OpportunityUpdate carries the mutable opportunity fields.
type OpportunityUpdate struct {
	Title                    *string
	Description              *string
	ExpectedValueAmountMinor *int64
	ExpectedValueCurrency    *string
	ClearExpectedValue       bool
	ProbabilityBasisPoints   *int
	ExpectedCloseDate        *time.Time
	ServiceCapabilities      []string
	AccountOwnerUserID       *string
	NextAction               *string
	NextActionAt             *time.Time
	UpdatedBy                string
}

// UpdateOpportunity applies a compare-and-swap update to a non-terminal
// opportunity.
func (r *Repository) UpdateOpportunity(ctx context.Context, q database.Querier,
	organizationID, opportunityID string, expectedVersion int64,
	update OpportunityUpdate) (models.Opportunity, error) {
	var capabilities any
	if update.ServiceCapabilities != nil {
		encoded, err := encodeJSONArray(update.ServiceCapabilities)
		if err != nil {
			return models.Opportunity{}, err
		}
		capabilities = encoded
	}
	row := r.q(q).QueryRow(ctx, `
		UPDATE fleet_partner_sales_opportunities SET
			title = COALESCE($4, title),
			description = COALESCE($5, description),
			expected_value_amount_minor = CASE WHEN $6 THEN NULL
			                                   ELSE COALESCE($7, expected_value_amount_minor) END,
			expected_value_currency = CASE WHEN $6 THEN NULL
			                               ELSE COALESCE($8::char(3), expected_value_currency) END,
			probability_basis_points = COALESCE($9, probability_basis_points),
			expected_close_date = COALESCE($10, expected_close_date),
			service_capabilities = COALESCE($11::jsonb, service_capabilities),
			account_owner_user_id = COALESCE($12, account_owner_user_id),
			next_action = COALESCE($13, next_action),
			next_action_at = COALESCE($14, next_action_at),
			updated_by = $15, version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND id = $2 AND version = $3
		  AND stage NOT IN ('WON', 'LOST')
		RETURNING `+opportunityColumns,
		organizationID, opportunityID, expectedVersion,
		update.Title, update.Description, update.ClearExpectedValue,
		update.ExpectedValueAmountMinor, update.ExpectedValueCurrency,
		update.ProbabilityBasisPoints, update.ExpectedCloseDate, capabilities,
		update.AccountOwnerUserID, update.NextAction, update.NextActionAt, update.UpdatedBy)
	updated, err := scanOpportunity(row)
	if err != nil {
		return updated, r.classifyOpportunityWrite(ctx, q, organizationID, opportunityID, err,
			"update opportunity")
	}
	return updated, nil
}

// TransitionOpportunity moves an opportunity to a new stage.
//
// The `stage = $4` guard is what makes two concurrent transitions resolve to
// one winner: the loser's expected stage no longer matches and it is refused
// rather than overwriting the winner's outcome.
func (r *Repository) TransitionOpportunity(ctx context.Context, q database.Querier,
	organizationID, opportunityID string, expectedVersion int64, fromStage, toStage,
	lossReason, holdReason, actor string) (models.Opportunity, error) {
	terminal := models.TerminalStage(toStage)
	row := r.q(q).QueryRow(ctx, `
		UPDATE fleet_partner_sales_opportunities SET
			stage = $5,
			loss_reason = CASE WHEN $6 = '' THEN loss_reason ELSE $6 END,
			hold_reason = CASE WHEN $7 = '' THEN hold_reason ELSE $7 END,
			closed_at = CASE WHEN $8 THEN now() ELSE closed_at END,
			updated_by = $9, version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND id = $2 AND version = $3 AND stage = $4
		RETURNING `+opportunityColumns,
		organizationID, opportunityID, expectedVersion, fromStage, toStage,
		lossReason, holdReason, terminal, actor)
	updated, err := scanOpportunity(row)
	if err != nil {
		return updated, r.classifyOpportunityWrite(ctx, q, organizationID, opportunityID, err,
			"transition opportunity")
	}
	return updated, nil
}

func (r *Repository) classifyOpportunityWrite(ctx context.Context, q database.Querier,
	organizationID, opportunityID string, err error, operation string) error {
	classified := classify(err, operation)
	if classified != ErrNotFound {
		return classified
	}
	if _, getErr := r.GetOpportunity(ctx, q, organizationID, opportunityID); getErr == nil {
		return ErrVersionConflict
	}
	return ErrNotFound
}

// AppendOpportunityHistory writes one immutable stage change.
func (r *Repository) AppendOpportunityHistory(ctx context.Context, q database.Querier,
	entry models.OpportunityHistory) error {
	if entry.ID == "" {
		entry.ID = ids.New(PrefixHistory)
	}
	_, err := r.q(q).Exec(ctx, `
		INSERT INTO fleet_partner_sales_opportunity_history
			(id, organization_id, opportunity_id, previous_stage, new_stage, reason_code,
			 actor_type, actor_id, correlation_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		entry.ID, entry.OrganizationID, entry.OpportunityID, entry.PreviousStage,
		entry.NewStage, entry.ReasonCode, entry.ActorType, entry.ActorID, entry.CorrelationID)
	return classify(err, "append opportunity history")
}

// ListOpportunityHistory returns a page of stage changes, newest first.
func (r *Repository) ListOpportunityHistory(ctx context.Context, organizationID,
	opportunityID string, page cursor.Cursor, hasCursor bool,
	limit int) ([]models.OpportunityHistory, error) {
	args := []any{organizationID, opportunityID}
	where := `organization_id = $1 AND opportunity_id = $2`
	where, args = pageWhere(where, args, page, hasCursor, "occurred_at")
	args = append(args, cursor.ClampLimit(limit))

	rows, err := r.db.Query(ctx, `
		SELECT id, organization_id, opportunity_id, previous_stage, new_stage, reason_code,
		       actor_type, actor_id, correlation_id, occurred_at
		FROM fleet_partner_sales_opportunity_history WHERE `+where+`
		ORDER BY occurred_at DESC, id DESC LIMIT $`+itoa(len(args)), args...)
	if err != nil {
		return nil, classify(err, "list opportunity history")
	}
	defer rows.Close()
	out := make([]models.OpportunityHistory, 0)
	for rows.Next() {
		var entry models.OpportunityHistory
		if scanError := rows.Scan(&entry.ID, &entry.OrganizationID, &entry.OpportunityID,
			&entry.PreviousStage, &entry.NewStage, &entry.ReasonCode, &entry.ActorType,
			&entry.ActorID, &entry.CorrelationID, &entry.OccurredAt); scanError != nil {
			return nil, scanError
		}
		out = append(out, entry)
	}
	return out, rows.Err()
}

// OpportunityPriorStages lists the distinct stages an opportunity has occupied.
//
// Resuming from ON_HOLD is only permitted into a stage the opportunity
// genuinely reached, so this is read from the immutable history rather than
// inferred from the current stage.
func (r *Repository) OpportunityPriorStages(ctx context.Context, q database.Querier,
	organizationID, opportunityID string) ([]string, error) {
	rows, err := r.q(q).Query(ctx, `
		SELECT DISTINCT previous_stage FROM fleet_partner_sales_opportunity_history
		WHERE organization_id = $1 AND opportunity_id = $2`,
		organizationID, opportunityID)
	if err != nil {
		return nil, classify(err, "opportunity prior stages")
	}
	defer rows.Close()
	out := make([]string, 0)
	for rows.Next() {
		var stage string
		if scanError := rows.Scan(&stage); scanError != nil {
			return nil, scanError
		}
		out = append(out, stage)
	}
	return out, rows.Err()
}

// CountOpportunitiesByStage reports per-stage totals for metrics.
func (r *Repository) CountOpportunitiesByStage(ctx context.Context) (map[string]int64, error) {
	rows, err := r.db.Query(ctx, `SELECT stage, count(*)
		FROM fleet_partner_sales_opportunities GROUP BY stage`)
	if err != nil {
		return nil, classify(err, "count opportunities")
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var stage string
		var total int64
		if scanError := rows.Scan(&stage, &total); scanError != nil {
			return nil, scanError
		}
		out[stage] = total
	}
	return out, rows.Err()
}
