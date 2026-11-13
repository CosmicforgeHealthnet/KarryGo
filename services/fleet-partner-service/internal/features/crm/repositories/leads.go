package repositories

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/cursor"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

// PrefixLead is the sales lead identifier prefix.
const PrefixLead = "led"

const leadColumns = `id, organization_id, COALESCE(branch_id, ''), source, company_name,
	contact_snapshot, status, COALESCE(owner_user_id, ''), score, COALESCE(next_action, ''),
	next_action_at, COALESCE(notes, ''), COALESCE(disqualification_reason, ''),
	COALESCE(converted_customer_id, ''), COALESCE(converted_opportunity_id, ''), converted_at,
	COALESCE(converted_by, ''), COALESCE(conversion_idempotency_key, ''),
	COALESCE(create_idempotency_key, ''), COALESCE(create_fingerprint, ''), version,
	created_by, updated_by, created_at, updated_at`

func scanLead(row pgx.Row) (models.Lead, error) {
	var l models.Lead
	var contact []byte
	err := row.Scan(&l.ID, &l.OrganizationID, &l.BranchID, &l.Source, &l.CompanyName, &contact,
		&l.Status, &l.OwnerUserID, &l.Score, &l.NextAction, &l.NextActionAt, &l.Notes,
		&l.DisqualificationReason, &l.ConvertedCustomerID, &l.ConvertedOpportunityID,
		&l.ConvertedAt, &l.ConvertedBy, &l.ConversionIdempotencyKey, &l.CreateIdempotencyKey,
		&l.CreateFingerprint, &l.Version, &l.CreatedBy, &l.UpdatedBy, &l.CreatedAt, &l.UpdatedAt)
	if err != nil {
		return l, scanErr(err)
	}
	l.ContactSnapshot = map[string]any(decodeJSONMap(contact))
	return l, nil
}

func encodeLeadContact(contact map[string]any) ([]byte, error) {
	if contact == nil {
		contact = map[string]any{}
	}
	return encodeJSON(contact)
}

// CreateLead inserts a sales lead. A unique (organization_id,
// create_idempotency_key) index makes a retried create collide instead of
// duplicating; the caller resolves that collision by reading the original.
func (r *Repository) CreateLead(ctx context.Context, q database.Querier, l models.Lead) (models.Lead, error) {
	contact, err := encodeLeadContact(l.ContactSnapshot)
	if err != nil {
		return models.Lead{}, err
	}
	row := r.q(q).QueryRow(ctx, `
		INSERT INTO fleet_partner_sales_leads
			(id, organization_id, branch_id, source, company_name, contact_snapshot, status,
			 owner_user_id, score, next_action, next_action_at, notes, create_idempotency_key,
			 create_fingerprint, created_by, updated_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $15)
		RETURNING `+leadColumns,
		l.ID, l.OrganizationID, nullable(l.BranchID), l.Source, l.CompanyName, contact, l.Status,
		nullable(l.OwnerUserID), l.Score, nullable(l.NextAction), l.NextActionAt, nullable(l.Notes),
		nullable(l.CreateIdempotencyKey), nullable(l.CreateFingerprint), l.CreatedBy)
	created, err := scanLead(row)
	if err != nil {
		return created, classify(err, "create lead")
	}
	return created, nil
}

// GetLead reads one lead inside the organization.
func (r *Repository) GetLead(ctx context.Context, q database.Querier, organizationID, leadID string) (models.Lead, error) {
	return scanLead(r.q(q).QueryRow(ctx, `SELECT `+leadColumns+`
		FROM fleet_partner_sales_leads WHERE organization_id = $1 AND id = $2`,
		organizationID, leadID))
}

// GetLeadByCreateKey finds the lead a previous create under the same
// Idempotency-Key produced.
func (r *Repository) GetLeadByCreateKey(ctx context.Context, q database.Querier,
	organizationID, key string) (models.Lead, error) {
	return scanLead(r.q(q).QueryRow(ctx, `SELECT `+leadColumns+`
		FROM fleet_partner_sales_leads WHERE organization_id = $1 AND create_idempotency_key = $2`,
		organizationID, key))
}

// LockLead reads one lead FOR UPDATE. Every lead mutation, conversion
// included, goes through this lock, which is what makes conversion happen
// at most once under concurrency.
func (r *Repository) LockLead(ctx context.Context, q database.Querier, organizationID, leadID string) (models.Lead, error) {
	return scanLead(r.q(q).QueryRow(ctx, `SELECT `+leadColumns+`
		FROM fleet_partner_sales_leads WHERE organization_id = $1 AND id = $2 FOR UPDATE`,
		organizationID, leadID))
}

// LeadFilter is the allowlisted lead list query.
type LeadFilter struct {
	Status        string
	Source        string
	OwnerUserID   string
	BranchID      string
	CreatedAfter  *time.Time
	CreatedBefore *time.Time
	Cursor        cursor.Cursor
	HasCursor     bool
	Limit         int
}

// ListLeads returns a tenant- and branch-scoped page, newest first.
func (r *Repository) ListLeads(ctx context.Context, organizationID string, branchIDs []string,
	filter LeadFilter) ([]models.Lead, error) {
	args := []any{organizationID, filter.Status, filter.Source, filter.OwnerUserID, filter.BranchID}
	where := `organization_id = $1
		AND ($2 = '' OR status = $2)
		AND ($3 = '' OR source = $3)
		AND ($4 = '' OR owner_user_id = $4)
		AND ($5 = '' OR branch_id = $5)`
	if filter.CreatedAfter != nil {
		args = append(args, *filter.CreatedAfter)
		where += " AND created_at >= $" + itoa(len(args))
	}
	if filter.CreatedBefore != nil {
		args = append(args, *filter.CreatedBefore)
		where += " AND created_at <= $" + itoa(len(args))
	}
	where, args = pageWhere(where, args, filter.Cursor, filter.HasCursor, "created_at")
	clause, args := branchClause(branchIDs, "branch_id", args)
	args = append(args, cursor.ClampLimit(filter.Limit))

	rows, err := r.db.Query(ctx, `SELECT `+leadColumns+`
		FROM fleet_partner_sales_leads WHERE `+where+` AND `+clause+`
		ORDER BY created_at DESC, id DESC LIMIT $`+itoa(len(args)), args...)
	if err != nil {
		return nil, classify(err, "list leads")
	}
	defer rows.Close()
	out := make([]models.Lead, 0)
	for rows.Next() {
		value, scanError := scanLead(rows)
		if scanError != nil {
			return nil, scanError
		}
		out = append(out, value)
	}
	return out, rows.Err()
}

// LeadUpdate carries the mutable lead fields. Nil means unchanged.
type LeadUpdate struct {
	CompanyName     *string
	Source          *string
	ContactSnapshot map[string]any
	OwnerUserID     *string
	Score           *int
	ClearScore      bool
	NextAction      *string
	NextActionAt    *time.Time
	Notes           *string
	UpdatedBy       string
}

// UpdateLead applies a compare-and-swap update to a lead that is still open.
func (r *Repository) UpdateLead(ctx context.Context, q database.Querier, organizationID, leadID string,
	expectedVersion int64, update LeadUpdate) (models.Lead, error) {
	var contact any
	if update.ContactSnapshot != nil {
		encoded, err := encodeLeadContact(update.ContactSnapshot)
		if err != nil {
			return models.Lead{}, err
		}
		contact = encoded
	}
	row := r.q(q).QueryRow(ctx, `
		UPDATE fleet_partner_sales_leads SET
			company_name = COALESCE($4, company_name),
			source = COALESCE($5, source),
			contact_snapshot = COALESCE($6::jsonb, contact_snapshot),
			owner_user_id = CASE WHEN $7::text IS NULL THEN owner_user_id
			                     ELSE NULLIF($7::text, '') END,
			score = CASE WHEN $8 THEN NULL ELSE COALESCE($9, score) END,
			next_action = COALESCE($10, next_action),
			next_action_at = COALESCE($11, next_action_at),
			notes = COALESCE($12, notes),
			updated_by = $13, version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND id = $2 AND version = $3
		  AND status NOT IN ('DISQUALIFIED', 'CONVERTED')
		RETURNING `+leadColumns,
		organizationID, leadID, expectedVersion, update.CompanyName, update.Source, contact,
		update.OwnerUserID, update.ClearScore, update.Score, update.NextAction, update.NextActionAt,
		update.Notes, update.UpdatedBy)
	updated, err := scanLead(row)
	if err != nil {
		return updated, r.classifyLeadWrite(ctx, q, organizationID, leadID, err, "update lead")
	}
	return updated, nil
}

// TransitionLead moves an open lead to a new qualification status. The
// `status = $4` guard makes two concurrent transitions resolve to one winner.
func (r *Repository) TransitionLead(ctx context.Context, q database.Querier, organizationID, leadID string,
	expectedVersion int64, fromStatus, toStatus, disqualificationReason, actor string) (models.Lead, error) {
	row := r.q(q).QueryRow(ctx, `
		UPDATE fleet_partner_sales_leads SET
			status = $5,
			disqualification_reason = CASE WHEN $6 = '' THEN disqualification_reason ELSE $6 END,
			updated_by = $7, version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND id = $2 AND version = $3 AND status = $4
		RETURNING `+leadColumns,
		organizationID, leadID, expectedVersion, fromStatus, toStatus, disqualificationReason, actor)
	updated, err := scanLead(row)
	if err != nil {
		return updated, r.classifyLeadWrite(ctx, q, organizationID, leadID, err, "transition lead")
	}
	return updated, nil
}

// MarkLeadConverted records the conversion outcome. The status guard means
// the row can only ever be converted from an open status, once.
func (r *Repository) MarkLeadConverted(ctx context.Context, q database.Querier, organizationID, leadID string,
	expectedVersion int64, fromStatus, customerID, opportunityID, idempotencyKey,
	actor string) (models.Lead, error) {
	row := r.q(q).QueryRow(ctx, `
		UPDATE fleet_partner_sales_leads SET
			status = 'CONVERTED', converted_customer_id = $5, converted_opportunity_id = $6,
			converted_at = now(), converted_by = $7, conversion_idempotency_key = $8,
			updated_by = $7, version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND id = $2 AND version = $3 AND status = $4
		RETURNING `+leadColumns,
		organizationID, leadID, expectedVersion, fromStatus, customerID, nullable(opportunityID),
		actor, nullable(idempotencyKey))
	updated, err := scanLead(row)
	if err != nil {
		return updated, r.classifyLeadWrite(ctx, q, organizationID, leadID, err, "convert lead")
	}
	return updated, nil
}

func (r *Repository) classifyLeadWrite(ctx context.Context, q database.Querier,
	organizationID, leadID string, err error, operation string) error {
	classified := classify(err, operation)
	if classified != ErrNotFound {
		return classified
	}
	if _, getErr := r.GetLead(ctx, q, organizationID, leadID); getErr == nil {
		return ErrVersionConflict
	}
	return ErrNotFound
}

// AppendLeadHistory writes one immutable lead change.
func (r *Repository) AppendLeadHistory(ctx context.Context, q database.Querier, entry models.LeadHistory) error {
	if entry.ID == "" {
		entry.ID = ids.New(PrefixHistory)
	}
	_, err := r.q(q).Exec(ctx, `
		INSERT INTO fleet_partner_sales_lead_history
			(id, organization_id, lead_id, change_type, previous_status, new_status, reason_code,
			 actor_type, actor_id, correlation_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		entry.ID, entry.OrganizationID, entry.LeadID, entry.ChangeType, entry.PreviousStatus,
		entry.NewStatus, entry.ReasonCode, entry.ActorType, entry.ActorID, entry.CorrelationID)
	return classify(err, "append lead history")
}

// ListLeadHistory returns a lead's changes, newest first.
func (r *Repository) ListLeadHistory(ctx context.Context, organizationID, leadID string,
	page cursor.Cursor, hasCursor bool, limit int) ([]models.LeadHistory, error) {
	args := []any{organizationID, leadID}
	where := `organization_id = $1 AND lead_id = $2`
	where, args = pageWhere(where, args, page, hasCursor, "occurred_at")
	args = append(args, cursor.ClampLimit(limit))
	rows, err := r.db.Query(ctx, `
		SELECT id, organization_id, lead_id, change_type, previous_status, new_status, reason_code,
		       actor_type, actor_id, correlation_id, occurred_at
		FROM fleet_partner_sales_lead_history WHERE `+where+`
		ORDER BY occurred_at DESC, id DESC LIMIT $`+itoa(len(args)), args...)
	if err != nil {
		return nil, classify(err, "list lead history")
	}
	defer rows.Close()
	out := make([]models.LeadHistory, 0)
	for rows.Next() {
		var entry models.LeadHistory
		if scanError := rows.Scan(&entry.ID, &entry.OrganizationID, &entry.LeadID, &entry.ChangeType,
			&entry.PreviousStatus, &entry.NewStatus, &entry.ReasonCode, &entry.ActorType,
			&entry.ActorID, &entry.CorrelationID, &entry.OccurredAt); scanError != nil {
			return nil, scanError
		}
		out = append(out, entry)
	}
	return out, rows.Err()
}

// ActiveOrganizationUser reports whether userID is a live (invited or active)
// member of the organization. Lead ownership may only be assigned to staff
// of the same tenant.
func (r *Repository) ActiveOrganizationUser(ctx context.Context, q database.Querier,
	organizationID, userID string) (bool, error) {
	var exists bool
	err := r.q(q).QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_organization_users
		WHERE organization_id = $1 AND id = $2 AND status IN ('INVITED', 'ACTIVE'))`,
		organizationID, userID).Scan(&exists)
	if err != nil {
		return false, classify(err, "check organization user")
	}
	return exists, nil
}
