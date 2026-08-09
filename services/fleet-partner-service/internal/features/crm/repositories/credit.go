package repositories

import (
	"context"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/cursor"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

const creditProfileColumns = `id, organization_id, customer_id, credit_limit_amount_minor,
	currency, payment_terms_days, hold_status, COALESCE(hold_reason, ''), risk_status_snapshot,
	COALESCE(approved_by, ''), COALESCE(approval_request_id, ''), effective_from, effective_to,
	version, created_by, updated_by, created_at, updated_at`

func scanCreditProfile(row pgx.Row) (models.CreditProfile, error) {
	var profile models.CreditProfile
	err := row.Scan(&profile.ID, &profile.OrganizationID, &profile.CustomerID,
		&profile.CreditLimitAmountMinor, &profile.Currency, &profile.PaymentTermsDays,
		&profile.HoldStatus, &profile.HoldReason, &profile.RiskStatusSnapshot,
		&profile.ApprovedBy, &profile.ApprovalRequestID, &profile.EffectiveFrom,
		&profile.EffectiveTo, &profile.Version, &profile.CreatedBy, &profile.UpdatedBy,
		&profile.CreatedAt, &profile.UpdatedAt)
	if err != nil {
		return profile, scanErr(err)
	}
	return profile, nil
}

// UpsertCreditProfile creates or replaces a customer's credit policy.
//
// The credit profile is policy, not accounting. There is no receivables
// balance here, no invoice ageing and no derived available credit: Fleet does
// not own what a customer owes, and a second set of books would be a second
// set of wrong answers.
func (r *Repository) UpsertCreditProfile(ctx context.Context, q database.Querier,
	profile models.CreditProfile, expectedVersion int64,
	requireVersion bool) (models.CreditProfile, error) {
	if !requireVersion {
		row := r.q(q).QueryRow(ctx, `
			INSERT INTO fleet_partner_customer_credit_profiles
				(id, organization_id, customer_id, credit_limit_amount_minor, currency,
				 payment_terms_days, hold_status, hold_reason, risk_status_snapshot,
				 approved_by, approval_request_id, effective_from, created_by, updated_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, now(), $12, $12)
			RETURNING `+creditProfileColumns,
			profile.ID, profile.OrganizationID, profile.CustomerID,
			profile.CreditLimitAmountMinor, profile.Currency, profile.PaymentTermsDays,
			profile.HoldStatus, nullable(profile.HoldReason), profile.RiskStatusSnapshot,
			nullable(profile.ApprovedBy), nullable(profile.ApprovalRequestID),
			profile.CreatedBy)
		created, err := scanCreditProfile(row)
		if err != nil {
			return created, classify(err, "create credit profile")
		}
		return created, nil
	}

	row := r.q(q).QueryRow(ctx, `
		UPDATE fleet_partner_customer_credit_profiles SET
			credit_limit_amount_minor = $4,
			currency = $5,
			payment_terms_days = $6,
			risk_status_snapshot = $7,
			approved_by = COALESCE(NULLIF($8, ''), approved_by),
			approval_request_id = COALESCE(NULLIF($9, ''), approval_request_id),
			updated_by = $10, version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND customer_id = $2 AND version = $3
		RETURNING `+creditProfileColumns,
		profile.OrganizationID, profile.CustomerID, expectedVersion,
		profile.CreditLimitAmountMinor, profile.Currency, profile.PaymentTermsDays,
		profile.RiskStatusSnapshot, profile.ApprovedBy, profile.ApprovalRequestID,
		profile.UpdatedBy)
	updated, err := scanCreditProfile(row)
	if err != nil {
		return updated, r.classifyCreditWrite(ctx, q, profile.OrganizationID,
			profile.CustomerID, err, "update credit profile")
	}
	return updated, nil
}

// GetCreditProfile reads a customer's credit policy.
func (r *Repository) GetCreditProfile(ctx context.Context, q database.Querier,
	organizationID, customerID string) (models.CreditProfile, error) {
	return scanCreditProfile(r.q(q).QueryRow(ctx, `SELECT `+creditProfileColumns+`
		FROM fleet_partner_customer_credit_profiles
		WHERE organization_id = $1 AND customer_id = $2`, organizationID, customerID))
}

// LockCreditProfile reads a credit profile FOR UPDATE.
func (r *Repository) LockCreditProfile(ctx context.Context, q database.Querier,
	organizationID, customerID string) (models.CreditProfile, error) {
	return scanCreditProfile(r.q(q).QueryRow(ctx, `SELECT `+creditProfileColumns+`
		FROM fleet_partner_customer_credit_profiles
		WHERE organization_id = $1 AND customer_id = $2 FOR UPDATE`,
		organizationID, customerID))
}

// SetCreditHold changes a customer's hold status.
//
// The expected current hold status is in the WHERE clause, so a hold and a
// release racing each other resolve to one winner rather than to a customer
// whose hold was released by a request that never saw it applied.
func (r *Repository) SetCreditHold(ctx context.Context, q database.Querier,
	organizationID, customerID string, expectedVersion int64,
	fromHold, toHold, reason, approvalRequestID, actor string) (models.CreditProfile, error) {
	row := r.q(q).QueryRow(ctx, `
		UPDATE fleet_partner_customer_credit_profiles SET
			hold_status = $5,
			hold_reason = CASE WHEN $6 = '' THEN NULL ELSE $6 END,
			approval_request_id = COALESCE(NULLIF($7, ''), approval_request_id),
			updated_by = $8, version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND customer_id = $2 AND version = $3 AND hold_status = $4
		RETURNING `+creditProfileColumns,
		organizationID, customerID, expectedVersion, fromHold, toHold, reason,
		approvalRequestID, actor)
	updated, err := scanCreditProfile(row)
	if err != nil {
		return updated, r.classifyCreditWrite(ctx, q, organizationID, customerID, err,
			"set credit hold")
	}
	return updated, nil
}

// SyncCreditRiskSnapshot mirrors a customer risk change onto the credit profile.
//
// A RESTRICTED customer's credit profile is forced to RESTRICTED. Leaving the
// two to disagree would mean a risk decision that never reached the gate that
// actually blocks contract activation.
func (r *Repository) SyncCreditRiskSnapshot(ctx context.Context, q database.Querier,
	organizationID, customerID, riskStatus string, forceRestrict bool) error {
	_, err := r.q(q).Exec(ctx, `
		UPDATE fleet_partner_customer_credit_profiles SET
			risk_status_snapshot = $3,
			hold_status = CASE WHEN $4 THEN 'RESTRICTED' ELSE hold_status END,
			hold_reason = CASE WHEN $4 THEN 'customer_risk_restricted' ELSE hold_reason END,
			version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND customer_id = $2`,
		organizationID, customerID, riskStatus, forceRestrict)
	return classify(err, "sync credit risk snapshot")
}

func (r *Repository) classifyCreditWrite(ctx context.Context, q database.Querier,
	organizationID, customerID string, err error, operation string) error {
	classified := classify(err, operation)
	if classified != ErrNotFound {
		return classified
	}
	if _, getErr := r.GetCreditProfile(ctx, q, organizationID, customerID); getErr == nil {
		return ErrVersionConflict
	}
	return ErrNotFound
}

// AppendCreditHistory writes one immutable credit policy change.
func (r *Repository) AppendCreditHistory(ctx context.Context, q database.Querier,
	entry models.CreditHistory) error {
	if entry.ID == "" {
		entry.ID = ids.New(PrefixHistory)
	}
	_, err := r.q(q).Exec(ctx, `
		INSERT INTO fleet_partner_customer_credit_history
			(id, organization_id, credit_profile_id, customer_id, previous_hold_status,
			 new_hold_status, previous_limit_minor, new_limit_minor, reason_code, actor_type,
			 actor_id, approval_request_id, correlation_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		entry.ID, entry.OrganizationID, entry.CreditProfileID, entry.CustomerID,
		entry.PreviousHoldStatus, entry.NewHoldStatus, entry.PreviousLimitMinor,
		entry.NewLimitMinor, entry.ReasonCode, entry.ActorType, entry.ActorID,
		nullable(entry.ApprovalRequestID), entry.CorrelationID)
	return classify(err, "append credit history")
}

// ListCreditHistory returns a page of credit history, newest first.
func (r *Repository) ListCreditHistory(ctx context.Context, organizationID,
	creditProfileID string, page cursor.Cursor, hasCursor bool,
	limit int) ([]models.CreditHistory, error) {
	args := []any{organizationID, creditProfileID}
	where := `organization_id = $1 AND credit_profile_id = $2`
	where, args = pageWhere(where, args, page, hasCursor, "occurred_at")
	args = append(args, cursor.ClampLimit(limit))

	rows, err := r.db.Query(ctx, `
		SELECT id, organization_id, credit_profile_id, customer_id, previous_hold_status,
		       new_hold_status, previous_limit_minor, new_limit_minor, reason_code,
		       actor_type, actor_id, COALESCE(approval_request_id, ''), correlation_id,
		       occurred_at
		FROM fleet_partner_customer_credit_history WHERE `+where+`
		ORDER BY occurred_at DESC, id DESC LIMIT $`+itoa(len(args)), args...)
	if err != nil {
		return nil, classify(err, "list credit history")
	}
	defer rows.Close()
	out := make([]models.CreditHistory, 0)
	for rows.Next() {
		var entry models.CreditHistory
		if scanError := rows.Scan(&entry.ID, &entry.OrganizationID, &entry.CreditProfileID,
			&entry.CustomerID, &entry.PreviousHoldStatus, &entry.NewHoldStatus,
			&entry.PreviousLimitMinor, &entry.NewLimitMinor, &entry.ReasonCode,
			&entry.ActorType, &entry.ActorID, &entry.ApprovalRequestID, &entry.CorrelationID,
			&entry.OccurredAt); scanError != nil {
			return nil, scanError
		}
		out = append(out, entry)
	}
	return out, rows.Err()
}

// CountCreditHolds reports the number of blocking holds for metrics.
func (r *Repository) CountCreditHolds(ctx context.Context) (int64, int64, error) {
	var profiles, holds int64
	err := r.db.QueryRow(ctx, `
		SELECT count(*), count(*) FILTER (WHERE hold_status IN ('ON_HOLD', 'RESTRICTED'))
		FROM fleet_partner_customer_credit_profiles`).Scan(&profiles, &holds)
	if err != nil {
		return 0, 0, classify(err, "count credit holds")
	}
	return profiles, holds, nil
}
