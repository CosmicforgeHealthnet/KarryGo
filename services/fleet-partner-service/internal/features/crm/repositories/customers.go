package repositories

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/cursor"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

const customerColumns = `id, organization_id, COALESCE(branch_id, ''), customer_type, legal_name,
	display_name, normalized_name, duplicate_fingerprint, COALESCE(registration_number, ''),
	COALESCE(tax_reference, ''), COALESCE(marketplace_customer_id, ''), status, risk_status,
	COALESCE(account_owner_user_id, ''), default_currency, COALESCE(payment_terms_reference, ''),
	COALESCE(credit_profile_id, ''), version, created_by, updated_by, created_at, updated_at,
	archived_at`

func scanCustomer(row pgx.Row) (models.Customer, error) {
	var c models.Customer
	err := row.Scan(&c.ID, &c.OrganizationID, &c.BranchID, &c.CustomerType, &c.LegalName,
		&c.DisplayName, &c.NormalizedName, &c.DuplicateFingerprint, &c.RegistrationNumber,
		&c.TaxReference, &c.MarketplaceCustomerID, &c.Status, &c.RiskStatus,
		&c.AccountOwnerUserID, &c.DefaultCurrency, &c.PaymentTermsReference,
		&c.CreditProfileID, &c.Version, &c.CreatedBy, &c.UpdatedBy, &c.CreatedAt,
		&c.UpdatedAt, &c.ArchivedAt)
	if err != nil {
		return c, scanErr(err)
	}
	return c, nil
}

// CustomerFilter is the allowlisted customer list query.
//
// There is no free-form filter field and no raw sort input: every value here
// is either an enum the domain recognises or a parameterized search term, so a
// filter cannot become a query fragment.
type CustomerFilter struct {
	Status             string
	RiskStatus         string
	CustomerType       string
	BranchID           string
	AccountOwnerUserID string
	CreatedAfter       *time.Time
	CreatedBefore      *time.Time
	Search             string
	Cursor             cursor.Cursor
	HasCursor          bool
	Limit              int
}

// CreateCustomer inserts a customer.
func (r *Repository) CreateCustomer(ctx context.Context, q database.Querier,
	c models.Customer) (models.Customer, error) {
	row := r.q(q).QueryRow(ctx, `
		INSERT INTO fleet_partner_company_customers
			(id, organization_id, branch_id, customer_type, legal_name, display_name,
			 normalized_name, duplicate_fingerprint, registration_number, tax_reference,
			 marketplace_customer_id, status, risk_status, account_owner_user_id,
			 default_currency, payment_terms_reference, created_by, updated_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $17)
		RETURNING `+customerColumns,
		c.ID, c.OrganizationID, nullable(c.BranchID), c.CustomerType, c.LegalName, c.DisplayName,
		c.NormalizedName, c.DuplicateFingerprint, nullable(c.RegistrationNumber),
		nullable(c.TaxReference), nullable(c.MarketplaceCustomerID), c.Status, c.RiskStatus,
		nullable(c.AccountOwnerUserID), c.DefaultCurrency, nullable(c.PaymentTermsReference),
		c.CreatedBy)
	created, err := scanCustomer(row)
	if err != nil {
		return created, classify(err, "create customer")
	}
	return created, nil
}

// GetCustomer reads one customer inside the organization.
func (r *Repository) GetCustomer(ctx context.Context, q database.Querier,
	organizationID, customerID string) (models.Customer, error) {
	return scanCustomer(r.q(q).QueryRow(ctx, `SELECT `+customerColumns+`
		FROM fleet_partner_company_customers WHERE organization_id = $1 AND id = $2`,
		organizationID, customerID))
}

// LockCustomer reads one customer FOR UPDATE.
//
// Every state-changing customer operation goes through this rather than
// GetCustomer, so two concurrent status changes serialize on the row instead of
// both reading the old status and both deciding they may proceed.
func (r *Repository) LockCustomer(ctx context.Context, q database.Querier,
	organizationID, customerID string) (models.Customer, error) {
	return scanCustomer(r.q(q).QueryRow(ctx, `SELECT `+customerColumns+`
		FROM fleet_partner_company_customers WHERE organization_id = $1 AND id = $2
		FOR UPDATE`, organizationID, customerID))
}

// ListCustomers returns a tenant-scoped page, newest first.
func (r *Repository) ListCustomers(ctx context.Context, organizationID string,
	branchIDs []string, filter CustomerFilter) ([]models.Customer, error) {
	args := []any{organizationID, filter.Status, filter.RiskStatus, filter.CustomerType,
		filter.AccountOwnerUserID, filter.BranchID}
	where := `organization_id = $1
		AND ($2 = '' OR status = $2)
		AND ($3 = '' OR risk_status = $3)
		AND ($4 = '' OR customer_type = $4)
		AND ($5 = '' OR account_owner_user_id = $5)
		AND ($6 = '' OR branch_id = $6)`

	if filter.CreatedAfter != nil {
		args = append(args, *filter.CreatedAfter)
		where += fmt.Sprintf(" AND created_at >= $%d", len(args))
	}
	if filter.CreatedBefore != nil {
		args = append(args, *filter.CreatedBefore)
		where += fmt.Sprintf(" AND created_at <= $%d", len(args))
	}
	if search := strings.TrimSpace(filter.Search); search != "" {
		// Parameterized and organization scoped. The pattern is built here
		// rather than interpolated, and the LIKE metacharacters in the caller's
		// text are escaped, so a search for "100%" is a search and not a
		// wildcard.
		args = append(args, "%"+escapeLike(models.NormalizeName(search))+"%")
		where += fmt.Sprintf(" AND normalized_name LIKE $%d ESCAPE '\\'", len(args))
	}

	where, args = pageWhere(where, args, filter.Cursor, filter.HasCursor, "created_at")
	clause, args := branchClause(branchIDs, "branch_id", args)
	args = append(args, cursor.ClampLimit(filter.Limit))

	rows, err := r.db.Query(ctx, `SELECT `+customerColumns+`
		FROM fleet_partner_company_customers
		WHERE `+where+` AND `+clause+`
		ORDER BY created_at DESC, id DESC LIMIT $`+itoa(len(args)), args...)
	if err != nil {
		return nil, classify(err, "list customers")
	}
	defer rows.Close()
	out := make([]models.Customer, 0)
	for rows.Next() {
		value, scanError := scanCustomer(rows)
		if scanError != nil {
			return nil, scanError
		}
		out = append(out, value)
	}
	return out, rows.Err()
}

func escapeLike(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(value)
}

// CustomerUpdate carries the mutable customer fields.
//
// Nil means "leave alone" rather than "clear". A PATCH that omits a field must
// not silently erase it.
type CustomerUpdate struct {
	BranchID              *string
	LegalName             *string
	DisplayName           *string
	NormalizedName        *string
	DuplicateFingerprint  *string
	RegistrationNumber    *string
	TaxReference          *string
	MarketplaceCustomerID *string
	AccountOwnerUserID    *string
	DefaultCurrency       *string
	PaymentTermsReference *string
	CreditProfileID       *string
	UpdatedBy             string
}

// UpdateCustomer applies a compare-and-swap update.
func (r *Repository) UpdateCustomer(ctx context.Context, q database.Querier,
	organizationID, customerID string, expectedVersion int64,
	update CustomerUpdate) (models.Customer, error) {
	row := r.q(q).QueryRow(ctx, `
		UPDATE fleet_partner_company_customers SET
			branch_id = COALESCE($4, branch_id),
			legal_name = COALESCE($5, legal_name),
			display_name = COALESCE($6, display_name),
			normalized_name = COALESCE($7, normalized_name),
			duplicate_fingerprint = COALESCE($8, duplicate_fingerprint),
			registration_number = COALESCE($9, registration_number),
			tax_reference = COALESCE($10, tax_reference),
			marketplace_customer_id = COALESCE($11, marketplace_customer_id),
			account_owner_user_id = COALESCE($12, account_owner_user_id),
			default_currency = COALESCE($13::char(3), default_currency),
			payment_terms_reference = COALESCE($14, payment_terms_reference),
			credit_profile_id = COALESCE($15, credit_profile_id),
			updated_by = $16, version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND id = $2 AND version = $3 AND archived_at IS NULL
		RETURNING `+customerColumns,
		organizationID, customerID, expectedVersion,
		update.BranchID, update.LegalName, update.DisplayName, update.NormalizedName,
		update.DuplicateFingerprint, update.RegistrationNumber, update.TaxReference,
		update.MarketplaceCustomerID, update.AccountOwnerUserID, update.DefaultCurrency,
		update.PaymentTermsReference, update.CreditProfileID, update.UpdatedBy)
	updated, err := scanCustomer(row)
	if err != nil {
		return updated, r.classifyCustomerWrite(ctx, q, organizationID, customerID, err,
			"update customer")
	}
	return updated, nil
}

// SetCustomerStatus moves a customer to a new status.
func (r *Repository) SetCustomerStatus(ctx context.Context, q database.Querier,
	organizationID, customerID string, expectedVersion int64, status, actor string,
	archive bool) (models.Customer, error) {
	row := r.q(q).QueryRow(ctx, `
		UPDATE fleet_partner_company_customers SET
			status = $4,
			archived_at = CASE WHEN $5 THEN now() ELSE archived_at END,
			updated_by = $6, version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND id = $2 AND version = $3
		RETURNING `+customerColumns,
		organizationID, customerID, expectedVersion, status, archive, actor)
	updated, err := scanCustomer(row)
	if err != nil {
		return updated, r.classifyCustomerWrite(ctx, q, organizationID, customerID, err,
			"set customer status")
	}
	return updated, nil
}

// SetCustomerRisk moves a customer to a new risk status.
func (r *Repository) SetCustomerRisk(ctx context.Context, q database.Querier,
	organizationID, customerID string, expectedVersion int64,
	riskStatus, actor string) (models.Customer, error) {
	row := r.q(q).QueryRow(ctx, `
		UPDATE fleet_partner_company_customers SET
			risk_status = $4, updated_by = $5, version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND id = $2 AND version = $3
		RETURNING `+customerColumns,
		organizationID, customerID, expectedVersion, riskStatus, actor)
	updated, err := scanCustomer(row)
	if err != nil {
		return updated, r.classifyCustomerWrite(ctx, q, organizationID, customerID, err,
			"set customer risk")
	}
	return updated, nil
}

// AttachCreditProfile points a customer at its credit profile.
func (r *Repository) AttachCreditProfile(ctx context.Context, q database.Querier,
	organizationID, customerID, creditProfileID string) error {
	_, err := r.q(q).Exec(ctx, `
		UPDATE fleet_partner_company_customers
		SET credit_profile_id = $3, updated_at = now()
		WHERE organization_id = $1 AND id = $2 AND credit_profile_id IS DISTINCT FROM $3`,
		organizationID, customerID, creditProfileID)
	return classify(err, "attach credit profile")
}

// classifyCustomerWrite distinguishes a version conflict from a missing row.
//
// A compare-and-swap that affects no rows is ambiguous: the row may not exist,
// may belong to another tenant, or may have moved on. The first two must both
// answer not-found; only the third is a conflict, and telling them apart needs
// a second read.
func (r *Repository) classifyCustomerWrite(ctx context.Context, q database.Querier,
	organizationID, customerID string, err error, operation string) error {
	classified := classify(err, operation)
	if classified != ErrNotFound {
		return classified
	}
	if _, getErr := r.GetCustomer(ctx, q, organizationID, customerID); getErr == nil {
		return ErrVersionConflict
	}
	return ErrNotFound
}

// AppendCustomerHistory writes one immutable customer change record.
func (r *Repository) AppendCustomerHistory(ctx context.Context, q database.Querier,
	entry models.CustomerHistory) error {
	fields, err := encodeJSONArray(entry.ChangedFields)
	if err != nil {
		return err
	}
	if entry.ID == "" {
		entry.ID = ids.New(PrefixHistory)
	}
	_, err = r.q(q).Exec(ctx, `
		INSERT INTO fleet_partner_crm_customer_history
			(id, organization_id, customer_id, change_type, previous_status, new_status,
			 previous_risk_status, new_risk_status, changed_fields, reason_code,
			 approval_request_id, actor_type, actor_id, correlation_id, causation_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`,
		entry.ID, entry.OrganizationID, entry.CustomerID, entry.ChangeType,
		entry.PreviousStatus, entry.NewStatus, entry.PreviousRiskStatus, entry.NewRiskStatus,
		fields, entry.ReasonCode, nullable(entry.ApprovalRequestID), entry.ActorType,
		entry.ActorID, entry.CorrelationID, entry.CausationID)
	return classify(err, "append customer history")
}

// ListCustomerHistory returns a page of customer history, newest first.
func (r *Repository) ListCustomerHistory(ctx context.Context, organizationID, customerID string,
	page cursor.Cursor, hasCursor bool, limit int) ([]models.CustomerHistory, error) {
	args := []any{organizationID, customerID}
	where := `organization_id = $1 AND customer_id = $2`
	where, args = pageWhere(where, args, page, hasCursor, "occurred_at")
	args = append(args, cursor.ClampLimit(limit))

	rows, err := r.db.Query(ctx, `
		SELECT id, organization_id, customer_id, change_type, previous_status, new_status,
		       previous_risk_status, new_risk_status, changed_fields, reason_code,
		       COALESCE(approval_request_id, ''), actor_type, actor_id, correlation_id,
		       causation_id, occurred_at
		FROM fleet_partner_crm_customer_history
		WHERE `+where+`
		ORDER BY occurred_at DESC, id DESC LIMIT $`+itoa(len(args)), args...)
	if err != nil {
		return nil, classify(err, "list customer history")
	}
	defer rows.Close()
	out := make([]models.CustomerHistory, 0)
	for rows.Next() {
		var entry models.CustomerHistory
		var fields []byte
		if scanError := rows.Scan(&entry.ID, &entry.OrganizationID, &entry.CustomerID,
			&entry.ChangeType, &entry.PreviousStatus, &entry.NewStatus,
			&entry.PreviousRiskStatus, &entry.NewRiskStatus, &fields, &entry.ReasonCode,
			&entry.ApprovalRequestID, &entry.ActorType, &entry.ActorID, &entry.CorrelationID,
			&entry.CausationID, &entry.OccurredAt); scanError != nil {
			return nil, scanError
		}
		entry.ChangedFields = decodeStringSlice(fields)
		out = append(out, entry)
	}
	return out, rows.Err()
}

// CountCustomersByStatus reports the per-status customer totals for metrics.
//
// It aggregates across tenants deliberately: the gauge it feeds is labelled by
// status only, because an organization ID as a metric label is an unbounded
// cardinality problem and a tenant-enumeration side channel.
func (r *Repository) CountCustomersByStatus(ctx context.Context) (map[string]int64, error) {
	rows, err := r.db.Query(ctx, `SELECT status, count(*)
		FROM fleet_partner_company_customers GROUP BY status`)
	if err != nil {
		return nil, classify(err, "count customers")
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var status string
		var total int64
		if scanError := rows.Scan(&status, &total); scanError != nil {
			return nil, scanError
		}
		out[status] = total
	}
	return out, rows.Err()
}
