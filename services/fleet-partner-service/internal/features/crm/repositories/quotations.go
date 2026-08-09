package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/cursor"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

const quotationColumns = `id, organization_id, COALESCE(branch_id, ''), customer_id,
	COALESCE(opportunity_id, ''), quotation_number, currency, valid_from, valid_until,
	service_scope, terms, internal_notes, status, current_version,
	COALESCE(issued_version_id, ''), COALESCE(accepted_version_id, ''),
	COALESCE(approval_request_id, ''), COALESCE(superseded_by_id, ''), total_amount_minor,
	version, created_by, updated_by, created_at, updated_at, issued_at, resolved_at`

func scanQuotation(row pgx.Row) (models.Quotation, error) {
	var quotation models.Quotation
	var scope, terms []byte
	err := row.Scan(&quotation.ID, &quotation.OrganizationID, &quotation.BranchID,
		&quotation.CustomerID, &quotation.OpportunityID, &quotation.QuotationNumber,
		&quotation.Currency, &quotation.ValidFrom, &quotation.ValidUntil, &scope, &terms,
		&quotation.InternalNotes, &quotation.Status, &quotation.CurrentVersion,
		&quotation.IssuedVersionID, &quotation.AcceptedVersionID, &quotation.ApprovalRequestID,
		&quotation.SupersededByID, &quotation.TotalAmountMinor, &quotation.Version,
		&quotation.CreatedBy, &quotation.UpdatedBy, &quotation.CreatedAt, &quotation.UpdatedAt,
		&quotation.IssuedAt, &quotation.ResolvedAt)
	if err != nil {
		return quotation, scanErr(err)
	}
	quotation.ServiceScope = decodeJSONMap(scope)
	quotation.Terms = decodeJSONMap(terms)
	return quotation, nil
}

// NextQuotationNumber mints a stable, organization-scoped business number.
//
// The number is derived from a per-organization, per-year count rather than a
// database sequence, so it exposes no global row count and one tenant cannot
// infer another's volume from it. The unique index on
// (organization_id, quotation_number) is the real arbiter: on a collision the
// caller retries, which is why this takes the count under the caller's
// transaction rather than trusting a cached value.
func (r *Repository) NextQuotationNumber(ctx context.Context, q database.Querier,
	organizationID string, now time.Time) (string, error) {
	year := now.UTC().Year()
	var sequence int64
	err := r.q(q).QueryRow(ctx, `
		SELECT count(*) + 1 FROM fleet_partner_quotations
		WHERE organization_id = $1 AND date_part('year', created_at) = $2`,
		organizationID, year).Scan(&sequence)
	if err != nil {
		return "", classify(err, "next quotation number")
	}
	return fmt.Sprintf("QTN-%d-%06d", year, sequence), nil
}

// CreateQuotation inserts a draft quotation.
func (r *Repository) CreateQuotation(ctx context.Context, q database.Querier,
	quotation models.Quotation) (models.Quotation, error) {
	scope, err := encodeJSON(quotation.ServiceScope)
	if err != nil {
		return models.Quotation{}, err
	}
	terms, err := encodeJSON(quotation.Terms)
	if err != nil {
		return models.Quotation{}, err
	}
	row := r.q(q).QueryRow(ctx, `
		INSERT INTO fleet_partner_quotations
			(id, organization_id, branch_id, customer_id, opportunity_id, quotation_number,
			 currency, valid_from, valid_until, service_scope, terms, internal_notes,
			 status, created_by, updated_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $14)
		RETURNING `+quotationColumns,
		quotation.ID, quotation.OrganizationID, nullable(quotation.BranchID),
		quotation.CustomerID, nullable(quotation.OpportunityID), quotation.QuotationNumber,
		quotation.Currency, quotation.ValidFrom, quotation.ValidUntil, scope, terms,
		quotation.InternalNotes, quotation.Status, quotation.CreatedBy)
	created, err := scanQuotation(row)
	if err != nil {
		return created, classify(err, "create quotation")
	}
	return created, nil
}

// GetQuotation reads one quotation inside the organization.
func (r *Repository) GetQuotation(ctx context.Context, q database.Querier,
	organizationID, quotationID string) (models.Quotation, error) {
	return scanQuotation(r.q(q).QueryRow(ctx, `SELECT `+quotationColumns+`
		FROM fleet_partner_quotations WHERE organization_id = $1 AND id = $2`,
		organizationID, quotationID))
}

// LockQuotation reads one quotation FOR UPDATE.
func (r *Repository) LockQuotation(ctx context.Context, q database.Querier,
	organizationID, quotationID string) (models.Quotation, error) {
	return scanQuotation(r.q(q).QueryRow(ctx, `SELECT `+quotationColumns+`
		FROM fleet_partner_quotations WHERE organization_id = $1 AND id = $2 FOR UPDATE`,
		organizationID, quotationID))
}

// QuotationFilter is the allowlisted quotation list query.
type QuotationFilter struct {
	Status        string
	CustomerID    string
	OpportunityID string
	BranchID      string
	CreatedAfter  *time.Time
	CreatedBefore *time.Time
	Cursor        cursor.Cursor
	HasCursor     bool
	Limit         int
}

// ListQuotations returns a tenant-scoped page, newest first.
func (r *Repository) ListQuotations(ctx context.Context, organizationID string,
	branchIDs []string, filter QuotationFilter) ([]models.Quotation, error) {
	args := []any{organizationID, filter.Status, filter.CustomerID, filter.OpportunityID,
		filter.BranchID}
	where := `organization_id = $1
		AND ($2 = '' OR status = $2)
		AND ($3 = '' OR customer_id = $3)
		AND ($4 = '' OR opportunity_id = $4)
		AND ($5 = '' OR branch_id = $5)`
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

	rows, err := r.db.Query(ctx, `SELECT `+quotationColumns+`
		FROM fleet_partner_quotations WHERE `+where+` AND `+clause+`
		ORDER BY created_at DESC, id DESC LIMIT $`+itoa(len(args)), args...)
	if err != nil {
		return nil, classify(err, "list quotations")
	}
	defer rows.Close()
	out := make([]models.Quotation, 0)
	for rows.Next() {
		value, scanError := scanQuotation(rows)
		if scanError != nil {
			return nil, scanError
		}
		out = append(out, value)
	}
	return out, rows.Err()
}

// QuotationUpdate carries the mutable draft quotation fields.
type QuotationUpdate struct {
	ValidFrom     *time.Time
	ValidUntil    *time.Time
	ServiceScope  models.JSONMap
	Terms         models.JSONMap
	InternalNotes *string
	OpportunityID *string
	UpdatedBy     string
}

// UpdateQuotation applies a compare-and-swap update to a draft quotation.
//
// The `status = 'DRAFT'` guard is in SQL: an issued quotation is a promise
// already made, and no edit path may reach it, including one that raced past
// an in-memory status check.
func (r *Repository) UpdateQuotation(ctx context.Context, q database.Querier,
	organizationID, quotationID string, expectedVersion int64,
	update QuotationUpdate) (models.Quotation, error) {
	var scope, terms any
	if update.ServiceScope != nil {
		encoded, err := encodeJSON(update.ServiceScope)
		if err != nil {
			return models.Quotation{}, err
		}
		scope = encoded
	}
	if update.Terms != nil {
		encoded, err := encodeJSON(update.Terms)
		if err != nil {
			return models.Quotation{}, err
		}
		terms = encoded
	}
	row := r.q(q).QueryRow(ctx, `
		UPDATE fleet_partner_quotations SET
			valid_from = COALESCE($4, valid_from),
			valid_until = COALESCE($5, valid_until),
			service_scope = COALESCE($6::jsonb, service_scope),
			terms = COALESCE($7::jsonb, terms),
			internal_notes = COALESCE($8, internal_notes),
			opportunity_id = COALESCE($9, opportunity_id),
			updated_by = $10, version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND id = $2 AND version = $3 AND status = 'DRAFT'
		RETURNING `+quotationColumns,
		organizationID, quotationID, expectedVersion, update.ValidFrom, update.ValidUntil,
		scope, terms, update.InternalNotes, update.OpportunityID, update.UpdatedBy)
	updated, err := scanQuotation(row)
	if err != nil {
		return updated, r.classifyQuotationWrite(ctx, q, organizationID, quotationID, err,
			"update quotation")
	}
	return updated, nil
}

// QuotationTransition carries a controlled quotation status change.
type QuotationTransition struct {
	FromStatus        string
	ToStatus          string
	ApprovalRequestID string
	IssuedVersionID   string
	AcceptedVersionID string
	SupersededByID    string
	TotalAmountMinor  *int64
	MarkIssued        bool
	MarkResolved      bool
	BumpVersion       bool
	Actor             string
}

// TransitionQuotation moves a quotation to a new status.
//
// Both the expected row version and the expected current status are in the
// WHERE clause. A concurrent accept and expire therefore cannot both succeed:
// whichever commits second finds the status it expected is gone and is
// refused, rather than overwriting a resolution that has already been
// communicated to a customer.
func (r *Repository) TransitionQuotation(ctx context.Context, q database.Querier,
	organizationID, quotationID string, expectedVersion int64,
	transition QuotationTransition) (models.Quotation, error) {
	row := r.q(q).QueryRow(ctx, `
		UPDATE fleet_partner_quotations SET
			status = $5,
			approval_request_id = CASE WHEN $6 = '' THEN approval_request_id ELSE $6 END,
			issued_version_id = CASE WHEN $7 = '' THEN issued_version_id ELSE $7 END,
			accepted_version_id = CASE WHEN $8 = '' THEN accepted_version_id ELSE $8 END,
			superseded_by_id = CASE WHEN $9 = '' THEN superseded_by_id ELSE $9 END,
			total_amount_minor = COALESCE($10, total_amount_minor),
			current_version = CASE WHEN $11 THEN current_version + 1 ELSE current_version END,
			issued_at = CASE WHEN $12 THEN now() ELSE issued_at END,
			resolved_at = CASE WHEN $13 THEN now() ELSE resolved_at END,
			updated_by = $14, version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND id = $2 AND version = $3 AND status = $4
		RETURNING `+quotationColumns,
		organizationID, quotationID, expectedVersion, transition.FromStatus,
		transition.ToStatus, transition.ApprovalRequestID, transition.IssuedVersionID,
		transition.AcceptedVersionID, transition.SupersededByID, transition.TotalAmountMinor,
		transition.BumpVersion, transition.MarkIssued, transition.MarkResolved,
		transition.Actor)
	updated, err := scanQuotation(row)
	if err != nil {
		return updated, r.classifyQuotationWrite(ctx, q, organizationID, quotationID, err,
			"transition quotation")
	}
	return updated, nil
}

func (r *Repository) classifyQuotationWrite(ctx context.Context, q database.Querier,
	organizationID, quotationID string, err error, operation string) error {
	classified := classify(err, operation)
	if classified != ErrNotFound {
		return classified
	}
	if _, getErr := r.GetQuotation(ctx, q, organizationID, quotationID); getErr == nil {
		return ErrVersionConflict
	}
	return ErrNotFound
}

// ---------------------------------------------------------------------------
// Line items
// ---------------------------------------------------------------------------

const lineItemColumns = `id, organization_id, quotation_id, description,
	trim_scale(quantity)::text,
	unit_type, unit_amount_minor, line_amount_minor, currency,
	COALESCE(external_pricing_reference, ''), manual_reason, sequence, version,
	created_at, updated_at`

func scanLineItem(row pgx.Row) (models.LineItem, error) {
	var item models.LineItem
	err := row.Scan(&item.ID, &item.OrganizationID, &item.QuotationID, &item.Description,
		&item.Quantity, &item.UnitType, &item.UnitAmountMinor, &item.LineAmountMinor,
		&item.Currency, &item.ExternalPricingReference, &item.ManualReason, &item.Sequence,
		&item.Version, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return item, scanErr(err)
	}
	return item, nil
}

// ReplaceLineItems rewrites a draft quotation's lines.
//
// Replacing rather than patching keeps the sequence contiguous and the unique
// (quotation_id, sequence) index satisfied without a two-phase renumbering
// dance. It is only ever called for a DRAFT quotation, and issued lines live
// in the immutable version snapshot, not here.
func (r *Repository) ReplaceLineItems(ctx context.Context, q database.Querier,
	organizationID, quotationID string, items []models.LineItem) ([]models.LineItem, error) {
	if _, err := r.q(q).Exec(ctx, `DELETE FROM fleet_partner_quotation_line_items
		WHERE organization_id = $1 AND quotation_id = $2`,
		organizationID, quotationID); err != nil {
		return nil, classify(err, "clear line items")
	}
	out := make([]models.LineItem, 0, len(items))
	for index, item := range items {
		if item.ID == "" {
			item.ID = ids.New(PrefixLineItem)
		}
		row := r.q(q).QueryRow(ctx, `
			INSERT INTO fleet_partner_quotation_line_items
				(id, organization_id, quotation_id, description, quantity, unit_type,
				 unit_amount_minor, line_amount_minor, currency, external_pricing_reference,
				 manual_reason, sequence)
			VALUES ($1, $2, $3, $4, $5::numeric, $6, $7, $8, $9, $10, $11, $12)
			RETURNING `+lineItemColumns,
			item.ID, organizationID, quotationID, item.Description, item.Quantity,
			item.UnitType, item.UnitAmountMinor, item.LineAmountMinor, item.Currency,
			nullable(item.ExternalPricingReference), item.ManualReason, index)
		created, err := scanLineItem(row)
		if err != nil {
			return nil, classify(err, "insert line item")
		}
		out = append(out, created)
	}
	return out, nil
}

// ListLineItems returns a quotation's lines in stable sequence order.
func (r *Repository) ListLineItems(ctx context.Context, q database.Querier,
	organizationID, quotationID string) ([]models.LineItem, error) {
	rows, err := r.q(q).Query(ctx, `SELECT `+lineItemColumns+`
		FROM fleet_partner_quotation_line_items
		WHERE organization_id = $1 AND quotation_id = $2
		ORDER BY sequence ASC, id ASC`, organizationID, quotationID)
	if err != nil {
		return nil, classify(err, "list line items")
	}
	defer rows.Close()
	out := make([]models.LineItem, 0)
	for rows.Next() {
		value, scanError := scanLineItem(rows)
		if scanError != nil {
			return nil, scanError
		}
		out = append(out, value)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Immutable quotation versions
// ---------------------------------------------------------------------------

const quotationVersionColumns = `id, organization_id, quotation_id, version_number,
	customer_snapshot, contact_snapshot, currency, line_items_snapshot,
	service_scope_snapshot, terms_snapshot, total_amount_minor, valid_from, valid_until,
	document_references, COALESCE(approval_request_id, ''), issued_by, issued_at,
	content_fingerprint`

func scanQuotationVersion(row pgx.Row) (models.QuotationVersion, error) {
	var version models.QuotationVersion
	var customer, contact, lines, scope, terms, documents []byte
	err := row.Scan(&version.ID, &version.OrganizationID, &version.QuotationID,
		&version.VersionNumber, &customer, &contact, &version.Currency, &lines, &scope,
		&terms, &version.TotalAmountMinor, &version.ValidFrom, &version.ValidUntil,
		&documents, &version.ApprovalRequestID, &version.IssuedBy, &version.IssuedAt,
		&version.ContentFingerprint)
	if err != nil {
		return version, scanErr(err)
	}
	version.CustomerSnapshot = decodeJSONMap(customer)
	version.ContactSnapshot = decodeJSONMap(contact)
	version.ServiceScopeSnapshot = decodeJSONMap(scope)
	version.TermsSnapshot = decodeJSONMap(terms)
	version.DocumentReferences = decodeStringSlice(documents)
	if len(lines) > 0 {
		_ = json.Unmarshal(lines, &version.LineItemsSnapshot)
	}
	if version.LineItemsSnapshot == nil {
		version.LineItemsSnapshot = []models.LineItem{}
	}
	return version, nil
}

// CreateQuotationVersion writes the immutable record of what was offered.
//
// There is deliberately no update or delete counterpart: the table carries a
// BEFORE UPDATE OR DELETE trigger that refuses both, so no later edit,
// migration or repair script can restate what a customer was told.
func (r *Repository) CreateQuotationVersion(ctx context.Context, q database.Querier,
	version models.QuotationVersion) (models.QuotationVersion, error) {
	customer, err := encodeJSON(version.CustomerSnapshot)
	if err != nil {
		return models.QuotationVersion{}, err
	}
	contact, err := encodeJSON(version.ContactSnapshot)
	if err != nil {
		return models.QuotationVersion{}, err
	}
	lines, err := encodeJSONArray(version.LineItemsSnapshot)
	if err != nil {
		return models.QuotationVersion{}, err
	}
	scope, err := encodeJSON(version.ServiceScopeSnapshot)
	if err != nil {
		return models.QuotationVersion{}, err
	}
	terms, err := encodeJSON(version.TermsSnapshot)
	if err != nil {
		return models.QuotationVersion{}, err
	}
	documents, err := encodeJSONArray(version.DocumentReferences)
	if err != nil {
		return models.QuotationVersion{}, err
	}
	row := r.q(q).QueryRow(ctx, `
		INSERT INTO fleet_partner_quotation_versions
			(id, organization_id, quotation_id, version_number, customer_snapshot,
			 contact_snapshot, currency, line_items_snapshot, service_scope_snapshot,
			 terms_snapshot, total_amount_minor, valid_from, valid_until, document_references,
			 approval_request_id, issued_by, content_fingerprint)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
		RETURNING `+quotationVersionColumns,
		version.ID, version.OrganizationID, version.QuotationID, version.VersionNumber,
		customer, contact, version.Currency, lines, scope, terms, version.TotalAmountMinor,
		version.ValidFrom, version.ValidUntil, documents, nullable(version.ApprovalRequestID),
		version.IssuedBy, version.ContentFingerprint)
	created, err := scanQuotationVersion(row)
	if err != nil {
		return created, classify(err, "create quotation version")
	}
	return created, nil
}

// GetQuotationVersion reads one immutable version.
func (r *Repository) GetQuotationVersion(ctx context.Context, q database.Querier,
	organizationID, quotationID, versionID string) (models.QuotationVersion, error) {
	return scanQuotationVersion(r.q(q).QueryRow(ctx, `SELECT `+quotationVersionColumns+`
		FROM fleet_partner_quotation_versions
		WHERE organization_id = $1 AND quotation_id = $2 AND id = $3`,
		organizationID, quotationID, versionID))
}

// ListQuotationVersions returns every version of a quotation, newest first.
func (r *Repository) ListQuotationVersions(ctx context.Context, organizationID,
	quotationID string, limit int) ([]models.QuotationVersion, error) {
	rows, err := r.db.Query(ctx, `SELECT `+quotationVersionColumns+`
		FROM fleet_partner_quotation_versions
		WHERE organization_id = $1 AND quotation_id = $2
		ORDER BY version_number DESC LIMIT $3`,
		organizationID, quotationID, cursor.ClampLimit(limit))
	if err != nil {
		return nil, classify(err, "list quotation versions")
	}
	defer rows.Close()
	out := make([]models.QuotationVersion, 0)
	for rows.Next() {
		value, scanError := scanQuotationVersion(rows)
		if scanError != nil {
			return nil, scanError
		}
		out = append(out, value)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Acceptance and conversion intent
// ---------------------------------------------------------------------------

const acceptanceColumns = `id, organization_id, quotation_id, accepted_version_id, actor_type,
	actor_reference, acceptance_method, accepted_at, COALESCE(ip_hash, ''),
	COALESCE(user_agent_hash, ''), COALESCE(evidence_document_reference, ''),
	idempotency_key, request_fingerprint, correlation_id, causation_id, created_at`

func scanAcceptance(row pgx.Row) (models.Acceptance, error) {
	var acceptance models.Acceptance
	err := row.Scan(&acceptance.ID, &acceptance.OrganizationID, &acceptance.QuotationID,
		&acceptance.AcceptedVersionID, &acceptance.ActorType, &acceptance.ActorReference,
		&acceptance.AcceptanceMethod, &acceptance.AcceptedAt, &acceptance.IPHash,
		&acceptance.UserAgentHash, &acceptance.EvidenceDocumentReference,
		&acceptance.IdempotencyKey, &acceptance.RequestFingerprint, &acceptance.CorrelationID,
		&acceptance.CausationID, &acceptance.CreatedAt)
	if err != nil {
		return acceptance, scanErr(err)
	}
	return acceptance, nil
}

// CreateAcceptance writes the immutable acceptance record.
func (r *Repository) CreateAcceptance(ctx context.Context, q database.Querier,
	acceptance models.Acceptance) (models.Acceptance, error) {
	row := r.q(q).QueryRow(ctx, `
		INSERT INTO fleet_partner_quotation_acceptances
			(id, organization_id, quotation_id, accepted_version_id, actor_type,
			 actor_reference, acceptance_method, ip_hash, user_agent_hash,
			 evidence_document_reference, idempotency_key, request_fingerprint,
			 correlation_id, causation_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		RETURNING `+acceptanceColumns,
		acceptance.ID, acceptance.OrganizationID, acceptance.QuotationID,
		acceptance.AcceptedVersionID, acceptance.ActorType, acceptance.ActorReference,
		acceptance.AcceptanceMethod, nullable(acceptance.IPHash),
		nullable(acceptance.UserAgentHash), nullable(acceptance.EvidenceDocumentReference),
		acceptance.IdempotencyKey, acceptance.RequestFingerprint, acceptance.CorrelationID,
		acceptance.CausationID)
	created, err := scanAcceptance(row)
	if err != nil {
		return created, classify(err, "create acceptance")
	}
	return created, nil
}

// GetAcceptance reads a quotation's acceptance, if any.
func (r *Repository) GetAcceptance(ctx context.Context, q database.Querier,
	organizationID, quotationID string) (models.Acceptance, error) {
	return scanAcceptance(r.q(q).QueryRow(ctx, `SELECT `+acceptanceColumns+`
		FROM fleet_partner_quotation_acceptances
		WHERE organization_id = $1 AND quotation_id = $2`, organizationID, quotationID))
}

// FindAcceptanceByKey looks up a replay by idempotency key.
func (r *Repository) FindAcceptanceByKey(ctx context.Context, q database.Querier,
	organizationID, idempotencyKey string) (models.Acceptance, error) {
	return scanAcceptance(r.q(q).QueryRow(ctx, `SELECT `+acceptanceColumns+`
		FROM fleet_partner_quotation_acceptances
		WHERE organization_id = $1 AND idempotency_key = $2`, organizationID, idempotencyKey))
}

const conversionIntentColumns = `id, organization_id, quotation_id, accepted_version_id,
	acceptance_id, conversion_type, status, idempotency_key, fingerprint, correlation_id,
	causation_id, created_at`

func scanConversionIntent(row pgx.Row) (models.ConversionIntent, error) {
	var intent models.ConversionIntent
	err := row.Scan(&intent.ID, &intent.OrganizationID, &intent.QuotationID,
		&intent.AcceptedVersionID, &intent.AcceptanceID, &intent.ConversionType,
		&intent.Status, &intent.IdempotencyKey, &intent.Fingerprint, &intent.CorrelationID,
		&intent.CausationID, &intent.CreatedAt)
	if err != nil {
		return intent, scanErr(err)
	}
	return intent, nil
}

// CreateConversionIntent records that accepted work may later become a private
// order.
//
// This is the entire Section 15 surface of an acceptance. It mints no order ID,
// reserves no capacity, creates no shipment, package, assignment or trip, and
// moves no money. The unique index on accepted_version_id is what guarantees
// "exactly one per accepted version" under concurrency.
func (r *Repository) CreateConversionIntent(ctx context.Context, q database.Querier,
	intent models.ConversionIntent) (models.ConversionIntent, error) {
	row := r.q(q).QueryRow(ctx, `
		INSERT INTO fleet_partner_quotation_conversion_intents
			(id, organization_id, quotation_id, accepted_version_id, acceptance_id,
			 conversion_type, status, idempotency_key, fingerprint, correlation_id,
			 causation_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING `+conversionIntentColumns,
		intent.ID, intent.OrganizationID, intent.QuotationID, intent.AcceptedVersionID,
		intent.AcceptanceID, models.ConversionTypeFuturePrivateOrder,
		models.ConversionStatusPending, intent.IdempotencyKey, intent.Fingerprint,
		intent.CorrelationID, intent.CausationID)
	created, err := scanConversionIntent(row)
	if err != nil {
		return created, classify(err, "create conversion intent")
	}
	return created, nil
}

// GetConversionIntent reads the intent for an accepted version.
func (r *Repository) GetConversionIntent(ctx context.Context, q database.Querier,
	organizationID, acceptedVersionID string) (models.ConversionIntent, error) {
	return scanConversionIntent(r.q(q).QueryRow(ctx, `SELECT `+conversionIntentColumns+`
		FROM fleet_partner_quotation_conversion_intents
		WHERE organization_id = $1 AND accepted_version_id = $2`,
		organizationID, acceptedVersionID))
}

// ConsumeConversionIntent marks a conversion intent consumed by a private
// order, guarded by consumed_at IS NULL so exactly one order can ever claim
// an intent under concurrency. Callers pass the transaction of the order
// creation, so the intent is marked consumed if and only if the order
// actually committed.
//
// This is Section 15 writing to a Section 14 table by design: the conversion
// intent is the durable handoff record Section 14 created specifically for
// this consumption, not a boundary violation.
func (r *Repository) ConsumeConversionIntent(ctx context.Context, q database.Querier,
	organizationID, intentID, privateOrderID string) error {
	tag, err := r.q(q).Exec(ctx, `
		UPDATE fleet_partner_quotation_conversion_intents
		SET consumed_at = now(), private_order_id = $1
		WHERE organization_id = $2 AND id = $3 AND consumed_at IS NULL`,
		privateOrderID, organizationID, intentID)
	if err != nil {
		return fmt.Errorf("crm: consume conversion intent: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ConversionIntentConsumption reads whether an intent has already been
// consumed and, if so, which private order claimed it — the fact that makes
// a repeated conversion attempt an idempotent replay instead of a conflict.
func (r *Repository) ConversionIntentConsumption(ctx context.Context, q database.Querier,
	organizationID, intentID string) (consumed bool, privateOrderID string, err error) {
	var orderID *string
	var consumedAt *time.Time
	scanErr := r.q(q).QueryRow(ctx, `
		SELECT consumed_at, private_order_id
		FROM fleet_partner_quotation_conversion_intents
		WHERE organization_id = $1 AND id = $2`, organizationID, intentID,
	).Scan(&consumedAt, &orderID)
	if scanErr != nil {
		return false, "", classify(scanErr, "read conversion intent consumption")
	}
	if consumedAt == nil {
		return false, "", nil
	}
	if orderID != nil {
		privateOrderID = *orderID
	}
	return true, privateOrderID, nil
}

// ---------------------------------------------------------------------------
// Quotation history
// ---------------------------------------------------------------------------

// AppendQuotationHistory writes one immutable quotation status change.
func (r *Repository) AppendQuotationHistory(ctx context.Context, q database.Querier,
	entry models.QuotationHistory) error {
	if entry.ID == "" {
		entry.ID = ids.New(PrefixHistory)
	}
	_, err := r.q(q).Exec(ctx, `
		INSERT INTO fleet_partner_quotation_history
			(id, organization_id, quotation_id, previous_status, new_status,
			 quotation_version, reason_code, approval_request_id, actor_type, actor_id,
			 correlation_id, causation_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		entry.ID, entry.OrganizationID, entry.QuotationID, entry.PreviousStatus,
		entry.NewStatus, entry.QuotationVersion, entry.ReasonCode,
		nullable(entry.ApprovalRequestID), entry.ActorType, entry.ActorID,
		entry.CorrelationID, entry.CausationID)
	return classify(err, "append quotation history")
}

// ListQuotationHistory returns a page of quotation history, newest first.
func (r *Repository) ListQuotationHistory(ctx context.Context, organizationID,
	quotationID string, page cursor.Cursor, hasCursor bool,
	limit int) ([]models.QuotationHistory, error) {
	args := []any{organizationID, quotationID}
	where := `organization_id = $1 AND quotation_id = $2`
	where, args = pageWhere(where, args, page, hasCursor, "occurred_at")
	args = append(args, cursor.ClampLimit(limit))

	rows, err := r.db.Query(ctx, `
		SELECT id, organization_id, quotation_id, previous_status, new_status,
		       quotation_version, reason_code, COALESCE(approval_request_id, ''), actor_type,
		       actor_id, correlation_id, causation_id, occurred_at
		FROM fleet_partner_quotation_history WHERE `+where+`
		ORDER BY occurred_at DESC, id DESC LIMIT $`+itoa(len(args)), args...)
	if err != nil {
		return nil, classify(err, "list quotation history")
	}
	defer rows.Close()
	out := make([]models.QuotationHistory, 0)
	for rows.Next() {
		var entry models.QuotationHistory
		if scanError := rows.Scan(&entry.ID, &entry.OrganizationID, &entry.QuotationID,
			&entry.PreviousStatus, &entry.NewStatus, &entry.QuotationVersion,
			&entry.ReasonCode, &entry.ApprovalRequestID, &entry.ActorType, &entry.ActorID,
			&entry.CorrelationID, &entry.CausationID, &entry.OccurredAt); scanError != nil {
			return nil, scanError
		}
		out = append(out, entry)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Secure customer access tokens
// ---------------------------------------------------------------------------

const accessTokenColumns = `id, organization_id, quotation_id, quotation_version_id,
	token_hash, scope, single_use, status, COALESCE(issued_to_contact_id, ''), expires_at,
	first_viewed_at, last_viewed_at, view_count, revoked_reason, created_by, version,
	created_at, updated_at`

func scanAccessToken(row pgx.Row) (models.QuotationAccessToken, error) {
	var token models.QuotationAccessToken
	err := row.Scan(&token.ID, &token.OrganizationID, &token.QuotationID,
		&token.QuotationVersionID, &token.TokenHash, &token.Scope, &token.SingleUse,
		&token.Status, &token.IssuedToContactID, &token.ExpiresAt, &token.FirstViewedAt,
		&token.LastViewedAt, &token.ViewCount, &token.RevokedReason, &token.CreatedBy,
		&token.Version, &token.CreatedAt, &token.UpdatedAt)
	if err != nil {
		return token, scanErr(err)
	}
	return token, nil
}

// CreateAccessToken stores the hash of a customer's secure link.
//
// The caller passes a hash. The raw token never reaches this layer, so it
// cannot be logged by a query logger, captured in a slow-query trace, or read
// out of a database backup.
func (r *Repository) CreateAccessToken(ctx context.Context, q database.Querier,
	token models.QuotationAccessToken) (models.QuotationAccessToken, error) {
	row := r.q(q).QueryRow(ctx, `
		INSERT INTO fleet_partner_quotation_access_tokens
			(id, organization_id, quotation_id, quotation_version_id, token_hash, scope,
			 single_use, status, issued_to_contact_id, expires_at, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'ACTIVE', $8, $9, $10)
		RETURNING `+accessTokenColumns,
		token.ID, token.OrganizationID, token.QuotationID, token.QuotationVersionID,
		token.TokenHash, token.Scope, token.SingleUse, nullable(token.IssuedToContactID),
		token.ExpiresAt, token.CreatedBy)
	created, err := scanAccessToken(row)
	if err != nil {
		return created, classify(err, "create access token")
	}
	return created, nil
}

// FindAccessTokenByHash resolves a presented token.
//
// The lookup is by hash alone. The quotation ID in the request is checked
// against what the token says afterwards, never used to find it: a token that
// names a different quotation must fail, and it cannot fail if the query
// trusted the request to locate the row in the first place.
func (r *Repository) FindAccessTokenByHash(ctx context.Context, q database.Querier,
	tokenHash string) (models.QuotationAccessToken, error) {
	return scanAccessToken(r.q(q).QueryRow(ctx, `SELECT `+accessTokenColumns+`
		FROM fleet_partner_quotation_access_tokens WHERE token_hash = $1`, tokenHash))
}

// RecordTokenView marks a view and consumes a single-use token.
func (r *Repository) RecordTokenView(ctx context.Context, q database.Querier,
	tokenID string) error {
	_, err := r.q(q).Exec(ctx, `
		UPDATE fleet_partner_quotation_access_tokens SET
			first_viewed_at = COALESCE(first_viewed_at, now()),
			last_viewed_at = now(),
			view_count = view_count + 1,
			status = CASE WHEN single_use THEN 'CONSUMED' ELSE status END,
			version = version + 1, updated_at = now()
		WHERE id = $1 AND status = 'ACTIVE'`, tokenID)
	return classify(err, "record token view")
}

// RevokeQuotationTokens invalidates every active token for a quotation.
//
// Supersession and cancellation both call this. A customer holding a link to a
// withdrawn offer must not be able to accept it, and revoking the link is the
// only thing that actually stops them.
func (r *Repository) RevokeQuotationTokens(ctx context.Context, q database.Querier,
	organizationID, quotationID, reason string) error {
	_, err := r.q(q).Exec(ctx, `
		UPDATE fleet_partner_quotation_access_tokens SET
			status = 'REVOKED', revoked_reason = $3, version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND quotation_id = $2 AND status = 'ACTIVE'`,
		organizationID, quotationID, reason)
	return classify(err, "revoke quotation tokens")
}

// ---------------------------------------------------------------------------
// Expiry worker support
// ---------------------------------------------------------------------------

// ClaimExpiredQuotations returns delivered quotations past their validity.
//
// Server time decides. The predicate mirrors the partial expiry index, so the
// sweep is an index scan rather than a table scan, and it never returns an
// ACCEPTED, REJECTED, CANCELLED or SUPERSEDED quotation: those are resolved,
// and a resolved quotation cannot expire out from under its resolution.
func (r *Repository) ClaimExpiredQuotations(ctx context.Context,
	limit int) ([]models.Quotation, error) {
	rows, err := r.db.Query(ctx, `SELECT `+quotationColumns+`
		FROM fleet_partner_quotations
		WHERE status IN ('ISSUED', 'VIEWED') AND valid_until <= now()
		ORDER BY valid_until ASC, id ASC LIMIT $1`, cursor.ClampLimit(limit))
	if err != nil {
		return nil, classify(err, "claim expired quotations")
	}
	defer rows.Close()
	out := make([]models.Quotation, 0)
	for rows.Next() {
		value, scanError := scanQuotation(rows)
		if scanError != nil {
			return nil, scanError
		}
		out = append(out, value)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Quotation documents
// ---------------------------------------------------------------------------

const quotationDocumentColumns = `id, organization_id, quotation_id,
	COALESCE(quotation_version_id, ''), media_object_id, document_purpose, is_mandatory,
	status_projection, readiness_state, retention_reference, source_service, source_version,
	last_event_id, synced_at, version, created_by, created_at, updated_at`

func scanDocument(row pgx.Row) (models.DocumentReference, error) {
	var doc models.DocumentReference
	err := row.Scan(&doc.ID, &doc.OrganizationID, &doc.SubjectID, &doc.SubjectVersionID,
		&doc.MediaObjectID, &doc.DocumentPurpose, &doc.IsMandatory, &doc.StatusProjection,
		&doc.ReadinessState, &doc.RetentionReference, &doc.SourceService, &doc.SourceVersion,
		&doc.LastEventID, &doc.SyncedAt, &doc.Version, &doc.CreatedBy, &doc.CreatedAt,
		&doc.UpdatedAt)
	if err != nil {
		return doc, scanErr(err)
	}
	return doc, nil
}

// AttachQuotationDocument records a Media object reference against a quotation.
func (r *Repository) AttachQuotationDocument(ctx context.Context, q database.Querier,
	doc models.DocumentReference) (models.DocumentReference, error) {
	row := r.q(q).QueryRow(ctx, `
		INSERT INTO fleet_partner_quotation_documents
			(id, organization_id, quotation_id, quotation_version_id, media_object_id,
			 document_purpose, is_mandatory, status_projection, readiness_state,
			 retention_reference, source_version, last_event_id, synced_at, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, now(), $13)
		ON CONFLICT (organization_id, quotation_id, media_object_id, document_purpose)
		DO UPDATE SET status_projection = EXCLUDED.status_projection,
		              readiness_state = EXCLUDED.readiness_state,
		              source_version = GREATEST(
		                  fleet_partner_quotation_documents.source_version,
		                  EXCLUDED.source_version),
		              synced_at = now(), version = fleet_partner_quotation_documents.version + 1,
		              updated_at = now()
		RETURNING `+quotationDocumentColumns,
		doc.ID, doc.OrganizationID, doc.SubjectID, nullable(doc.SubjectVersionID),
		doc.MediaObjectID, doc.DocumentPurpose, doc.IsMandatory, doc.StatusProjection,
		doc.ReadinessState, doc.RetentionReference, doc.SourceVersion, doc.LastEventID,
		doc.CreatedBy)
	created, err := scanDocument(row)
	if err != nil {
		return created, classify(err, "attach quotation document")
	}
	return created, nil
}

// ListQuotationDocuments returns every document reference for a quotation.
func (r *Repository) ListQuotationDocuments(ctx context.Context, q database.Querier,
	organizationID, quotationID string) ([]models.DocumentReference, error) {
	rows, err := r.q(q).Query(ctx, `SELECT `+quotationDocumentColumns+`
		FROM fleet_partner_quotation_documents
		WHERE organization_id = $1 AND quotation_id = $2
		ORDER BY created_at ASC, id ASC`, organizationID, quotationID)
	if err != nil {
		return nil, classify(err, "list quotation documents")
	}
	defer rows.Close()
	out := make([]models.DocumentReference, 0)
	for rows.Next() {
		value, scanError := scanDocument(rows)
		if scanError != nil {
			return nil, scanError
		}
		out = append(out, value)
	}
	return out, rows.Err()
}

// BindQuotationDocumentsToVersion freezes the document set onto an issued
// version.
func (r *Repository) BindQuotationDocumentsToVersion(ctx context.Context, q database.Querier,
	organizationID, quotationID, versionID string) error {
	_, err := r.q(q).Exec(ctx, `
		UPDATE fleet_partner_quotation_documents
		SET quotation_version_id = $3, version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND quotation_id = $2 AND quotation_version_id IS NULL`,
		organizationID, quotationID, versionID)
	return classify(err, "bind quotation documents")
}

// ErrDocumentNotReady reports that a mandatory document cannot back a
// commitment.
var ErrDocumentNotReady = errors.New("crm: mandatory document is not ready")

// RequireMandatoryDocumentsReady refuses when any mandatory quotation document
// is not confirmed safe and ready.
//
// UNKNOWN counts as not ready. A document whose safety could not be confirmed
// is treated exactly like one confirmed unsafe, because the alternative is
// sending a customer a file nobody has checked.
func (r *Repository) RequireMandatoryDocumentsReady(ctx context.Context, q database.Querier,
	organizationID, quotationID string) error {
	var blocking int
	err := r.q(q).QueryRow(ctx, `
		SELECT count(*) FROM fleet_partner_quotation_documents
		WHERE organization_id = $1 AND quotation_id = $2 AND is_mandatory
		  AND NOT (status_projection = 'READY' AND readiness_state = 'READY')`,
		organizationID, quotationID).Scan(&blocking)
	if err != nil {
		return classify(err, "check quotation documents")
	}
	if blocking > 0 {
		return ErrDocumentNotReady
	}
	return nil
}
