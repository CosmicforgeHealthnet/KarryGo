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

const contractColumns = `id, organization_id, COALESCE(branch_id, ''), customer_id,
	COALESCE(quotation_id, ''), contract_number, effective_from, effective_to, renewal_terms,
	service_scope, commercial_snapshot, termination_terms, status, current_version,
	COALESCE(active_version_id, ''), COALESCE(approval_request_id, ''),
	COALESCE(supersedes_id, ''), version, created_by, updated_by, created_at, updated_at,
	activated_at, terminated_at`

func scanContract(row pgx.Row) (models.Contract, error) {
	var contract models.Contract
	var renewal, scope, commercial, termination []byte
	err := row.Scan(&contract.ID, &contract.OrganizationID, &contract.BranchID,
		&contract.CustomerID, &contract.QuotationID, &contract.ContractNumber,
		&contract.EffectiveFrom, &contract.EffectiveTo, &renewal, &scope, &commercial,
		&termination, &contract.Status, &contract.CurrentVersion, &contract.ActiveVersionID,
		&contract.ApprovalRequestID, &contract.SupersedesID, &contract.Version,
		&contract.CreatedBy, &contract.UpdatedBy, &contract.CreatedAt, &contract.UpdatedAt,
		&contract.ActivatedAt, &contract.TerminatedAt)
	if err != nil {
		return contract, scanErr(err)
	}
	contract.RenewalTerms = decodeJSONMap(renewal)
	contract.ServiceScope = decodeJSONMap(scope)
	contract.CommercialSnapshot = decodeJSONMap(commercial)
	contract.TerminationTerms = decodeJSONMap(termination)
	return contract, nil
}

// NextContractNumber mints a stable, organization-scoped contract number.
func (r *Repository) NextContractNumber(ctx context.Context, q database.Querier,
	organizationID string, now time.Time) (string, error) {
	year := now.UTC().Year()
	var sequence int64
	err := r.q(q).QueryRow(ctx, `
		SELECT count(*) + 1 FROM fleet_partner_contracts
		WHERE organization_id = $1 AND date_part('year', created_at) = $2`,
		organizationID, year).Scan(&sequence)
	if err != nil {
		return "", classify(err, "next contract number")
	}
	return fmt.Sprintf("CTR-%d-%06d", year, sequence), nil
}

// CreateContract inserts a draft contract.
func (r *Repository) CreateContract(ctx context.Context, q database.Querier,
	contract models.Contract) (models.Contract, error) {
	renewal, err := encodeJSON(contract.RenewalTerms)
	if err != nil {
		return models.Contract{}, err
	}
	scope, err := encodeJSON(contract.ServiceScope)
	if err != nil {
		return models.Contract{}, err
	}
	commercial, err := encodeJSON(contract.CommercialSnapshot)
	if err != nil {
		return models.Contract{}, err
	}
	termination, err := encodeJSON(contract.TerminationTerms)
	if err != nil {
		return models.Contract{}, err
	}
	row := r.q(q).QueryRow(ctx, `
		INSERT INTO fleet_partner_contracts
			(id, organization_id, branch_id, customer_id, quotation_id, contract_number,
			 effective_from, effective_to, renewal_terms, service_scope, commercial_snapshot,
			 termination_terms, status, supersedes_id, created_by, updated_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $15)
		RETURNING `+contractColumns,
		contract.ID, contract.OrganizationID, nullable(contract.BranchID),
		contract.CustomerID, nullable(contract.QuotationID), contract.ContractNumber,
		contract.EffectiveFrom, contract.EffectiveTo, renewal, scope, commercial, termination,
		contract.Status, nullable(contract.SupersedesID), contract.CreatedBy)
	created, err := scanContract(row)
	if err != nil {
		return created, classify(err, "create contract")
	}
	return created, nil
}

// GetContract reads one contract inside the organization.
func (r *Repository) GetContract(ctx context.Context, q database.Querier,
	organizationID, contractID string) (models.Contract, error) {
	return scanContract(r.q(q).QueryRow(ctx, `SELECT `+contractColumns+`
		FROM fleet_partner_contracts WHERE organization_id = $1 AND id = $2`,
		organizationID, contractID))
}

// LockContract reads one contract FOR UPDATE.
func (r *Repository) LockContract(ctx context.Context, q database.Querier,
	organizationID, contractID string) (models.Contract, error) {
	return scanContract(r.q(q).QueryRow(ctx, `SELECT `+contractColumns+`
		FROM fleet_partner_contracts WHERE organization_id = $1 AND id = $2 FOR UPDATE`,
		organizationID, contractID))
}

// ContractFilter is the allowlisted contract list query.
type ContractFilter struct {
	Status        string
	CustomerID    string
	BranchID      string
	CreatedAfter  *time.Time
	CreatedBefore *time.Time
	Cursor        cursor.Cursor
	HasCursor     bool
	Limit         int
}

// ListContracts returns a tenant-scoped page, newest first.
func (r *Repository) ListContracts(ctx context.Context, organizationID string,
	branchIDs []string, filter ContractFilter) ([]models.Contract, error) {
	args := []any{organizationID, filter.Status, filter.CustomerID, filter.BranchID}
	where := `organization_id = $1
		AND ($2 = '' OR status = $2)
		AND ($3 = '' OR customer_id = $3)
		AND ($4 = '' OR branch_id = $4)`
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

	rows, err := r.db.Query(ctx, `SELECT `+contractColumns+`
		FROM fleet_partner_contracts WHERE `+where+` AND `+clause+`
		ORDER BY created_at DESC, id DESC LIMIT $`+itoa(len(args)), args...)
	if err != nil {
		return nil, classify(err, "list contracts")
	}
	defer rows.Close()
	out := make([]models.Contract, 0)
	for rows.Next() {
		value, scanError := scanContract(rows)
		if scanError != nil {
			return nil, scanError
		}
		out = append(out, value)
	}
	return out, rows.Err()
}

// ContractUpdate carries the mutable draft contract fields.
type ContractUpdate struct {
	EffectiveFrom      *time.Time
	EffectiveTo        *time.Time
	RenewalTerms       models.JSONMap
	ServiceScope       models.JSONMap
	CommercialSnapshot models.JSONMap
	TerminationTerms   models.JSONMap
	QuotationID        *string
	UpdatedBy          string
}

// UpdateContract applies a compare-and-swap update to a draft contract.
func (r *Repository) UpdateContract(ctx context.Context, q database.Querier,
	organizationID, contractID string, expectedVersion int64,
	update ContractUpdate) (models.Contract, error) {
	encode := func(value models.JSONMap) (any, error) {
		if value == nil {
			return nil, nil
		}
		return encodeJSON(value)
	}
	renewal, err := encode(update.RenewalTerms)
	if err != nil {
		return models.Contract{}, err
	}
	scope, err := encode(update.ServiceScope)
	if err != nil {
		return models.Contract{}, err
	}
	commercial, err := encode(update.CommercialSnapshot)
	if err != nil {
		return models.Contract{}, err
	}
	termination, err := encode(update.TerminationTerms)
	if err != nil {
		return models.Contract{}, err
	}
	row := r.q(q).QueryRow(ctx, `
		UPDATE fleet_partner_contracts SET
			effective_from = COALESCE($4, effective_from),
			effective_to = COALESCE($5, effective_to),
			renewal_terms = COALESCE($6::jsonb, renewal_terms),
			service_scope = COALESCE($7::jsonb, service_scope),
			commercial_snapshot = COALESCE($8::jsonb, commercial_snapshot),
			termination_terms = COALESCE($9::jsonb, termination_terms),
			quotation_id = COALESCE($10, quotation_id),
			updated_by = $11, version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND id = $2 AND version = $3 AND status = 'DRAFT'
		RETURNING `+contractColumns,
		organizationID, contractID, expectedVersion, update.EffectiveFrom, update.EffectiveTo,
		renewal, scope, commercial, termination, update.QuotationID, update.UpdatedBy)
	updated, err := scanContract(row)
	if err != nil {
		return updated, r.classifyContractWrite(ctx, q, organizationID, contractID, err,
			"update contract")
	}
	return updated, nil
}

// ContractTransition carries a controlled contract status change.
type ContractTransition struct {
	FromStatus        string
	ToStatus          string
	ApprovalRequestID string
	ActiveVersionID   string
	EffectiveTo       *time.Time
	MarkActivated     bool
	MarkTerminated    bool
	BumpVersion       bool
	Actor             string
}

// TransitionContract moves a contract to a new status.
//
// As with quotations, both version and current status are guarded in SQL, so a
// simultaneous activate and terminate resolve to one winner rather than to an
// active contract that somebody has already been told is terminated.
func (r *Repository) TransitionContract(ctx context.Context, q database.Querier,
	organizationID, contractID string, expectedVersion int64,
	transition ContractTransition) (models.Contract, error) {
	row := r.q(q).QueryRow(ctx, `
		UPDATE fleet_partner_contracts SET
			status = $5,
			approval_request_id = CASE WHEN $6 = '' THEN approval_request_id ELSE $6 END,
			active_version_id = CASE WHEN $7 = '' THEN active_version_id ELSE $7 END,
			effective_to = COALESCE($8, effective_to),
			current_version = CASE WHEN $9 THEN current_version + 1 ELSE current_version END,
			activated_at = CASE WHEN $10 THEN now() ELSE activated_at END,
			terminated_at = CASE WHEN $11 THEN now() ELSE terminated_at END,
			updated_by = $12, version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND id = $2 AND version = $3 AND status = $4
		RETURNING `+contractColumns,
		organizationID, contractID, expectedVersion, transition.FromStatus,
		transition.ToStatus, transition.ApprovalRequestID, transition.ActiveVersionID,
		transition.EffectiveTo, transition.BumpVersion, transition.MarkActivated,
		transition.MarkTerminated, transition.Actor)
	updated, err := scanContract(row)
	if err != nil {
		return updated, r.classifyContractWrite(ctx, q, organizationID, contractID, err,
			"transition contract")
	}
	return updated, nil
}

func (r *Repository) classifyContractWrite(ctx context.Context, q database.Querier,
	organizationID, contractID string, err error, operation string) error {
	classified := classify(err, operation)
	if classified != ErrNotFound {
		return classified
	}
	if _, getErr := r.GetContract(ctx, q, organizationID, contractID); getErr == nil {
		return ErrVersionConflict
	}
	return ErrNotFound
}

// HasActiveContract reports whether the customer already holds an active
// contract, excluding one contract ID.
func (r *Repository) HasActiveContract(ctx context.Context, q database.Querier,
	organizationID, customerID, excludeContractID string) (bool, error) {
	var total int
	err := r.q(q).QueryRow(ctx, `
		SELECT count(*) FROM fleet_partner_contracts
		WHERE organization_id = $1 AND customer_id = $2 AND status = 'ACTIVE' AND id <> $3`,
		organizationID, customerID, excludeContractID).Scan(&total)
	if err != nil {
		return false, classify(err, "check active contract")
	}
	return total > 0, nil
}

// ---------------------------------------------------------------------------
// Immutable contract versions
// ---------------------------------------------------------------------------

const contractVersionColumns = `id, organization_id, contract_id, version_number,
	customer_snapshot, service_scope_snapshot, commercial_snapshot, effective_from,
	effective_to, renewal_terms, termination_terms, document_references,
	COALESCE(approval_request_id, ''), signatory_references, content_fingerprint,
	activated_by, activated_at`

func scanContractVersion(row pgx.Row) (models.ContractVersion, error) {
	var version models.ContractVersion
	var customer, scope, commercial, renewal, termination, documents, signatories []byte
	err := row.Scan(&version.ID, &version.OrganizationID, &version.ContractID,
		&version.VersionNumber, &customer, &scope, &commercial, &version.EffectiveFrom,
		&version.EffectiveTo, &renewal, &termination, &documents, &version.ApprovalRequestID,
		&signatories, &version.ContentFingerprint, &version.ActivatedBy, &version.ActivatedAt)
	if err != nil {
		return version, scanErr(err)
	}
	version.CustomerSnapshot = decodeJSONMap(customer)
	version.ServiceScopeSnapshot = decodeJSONMap(scope)
	version.CommercialSnapshot = decodeJSONMap(commercial)
	version.RenewalTerms = decodeJSONMap(renewal)
	version.TerminationTerms = decodeJSONMap(termination)
	version.DocumentReferences = decodeStringSlice(documents)
	version.SignatoryReferences = decodeStringSlice(signatories)
	return version, nil
}

// CreateContractVersion writes the immutable record of what was activated.
//
// There is no update or delete counterpart, and the table's trigger refuses
// both. A change to an active contract must produce a new version, a renewal or
// a replacement contract; it may not restate the one already in force.
func (r *Repository) CreateContractVersion(ctx context.Context, q database.Querier,
	version models.ContractVersion) (models.ContractVersion, error) {
	customer, err := encodeJSON(version.CustomerSnapshot)
	if err != nil {
		return models.ContractVersion{}, err
	}
	scope, err := encodeJSON(version.ServiceScopeSnapshot)
	if err != nil {
		return models.ContractVersion{}, err
	}
	commercial, err := encodeJSON(version.CommercialSnapshot)
	if err != nil {
		return models.ContractVersion{}, err
	}
	renewal, err := encodeJSON(version.RenewalTerms)
	if err != nil {
		return models.ContractVersion{}, err
	}
	termination, err := encodeJSON(version.TerminationTerms)
	if err != nil {
		return models.ContractVersion{}, err
	}
	documents, err := encodeJSONArray(version.DocumentReferences)
	if err != nil {
		return models.ContractVersion{}, err
	}
	signatories, err := encodeJSONArray(version.SignatoryReferences)
	if err != nil {
		return models.ContractVersion{}, err
	}
	row := r.q(q).QueryRow(ctx, `
		INSERT INTO fleet_partner_contract_versions
			(id, organization_id, contract_id, version_number, customer_snapshot,
			 service_scope_snapshot, commercial_snapshot, effective_from, effective_to,
			 renewal_terms, termination_terms, document_references, approval_request_id,
			 signatory_references, content_fingerprint, activated_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		RETURNING `+contractVersionColumns,
		version.ID, version.OrganizationID, version.ContractID, version.VersionNumber,
		customer, scope, commercial, version.EffectiveFrom, version.EffectiveTo, renewal,
		termination, documents, nullable(version.ApprovalRequestID), signatories,
		version.ContentFingerprint, version.ActivatedBy)
	created, err := scanContractVersion(row)
	if err != nil {
		return created, classify(err, "create contract version")
	}
	return created, nil
}

// GetContractVersion reads one immutable contract version.
func (r *Repository) GetContractVersion(ctx context.Context, q database.Querier,
	organizationID, contractID, versionID string) (models.ContractVersion, error) {
	return scanContractVersion(r.q(q).QueryRow(ctx, `SELECT `+contractVersionColumns+`
		FROM fleet_partner_contract_versions
		WHERE organization_id = $1 AND contract_id = $2 AND id = $3`,
		organizationID, contractID, versionID))
}

// ListContractVersions returns every version of a contract, newest first.
func (r *Repository) ListContractVersions(ctx context.Context, organizationID,
	contractID string, limit int) ([]models.ContractVersion, error) {
	rows, err := r.db.Query(ctx, `SELECT `+contractVersionColumns+`
		FROM fleet_partner_contract_versions
		WHERE organization_id = $1 AND contract_id = $2
		ORDER BY version_number DESC LIMIT $3`,
		organizationID, contractID, cursor.ClampLimit(limit))
	if err != nil {
		return nil, classify(err, "list contract versions")
	}
	defer rows.Close()
	out := make([]models.ContractVersion, 0)
	for rows.Next() {
		value, scanError := scanContractVersion(rows)
		if scanError != nil {
			return nil, scanError
		}
		out = append(out, value)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Contract documents
// ---------------------------------------------------------------------------

const contractDocumentColumns = `id, organization_id, contract_id,
	COALESCE(contract_version_id, ''), media_object_id, document_purpose, is_mandatory,
	status_projection, readiness_state, retention_reference, source_service, source_version,
	last_event_id, synced_at, version, created_by, created_at, updated_at`

// AttachContractDocument records a Media object reference against a contract.
func (r *Repository) AttachContractDocument(ctx context.Context, q database.Querier,
	doc models.DocumentReference) (models.DocumentReference, error) {
	row := r.q(q).QueryRow(ctx, `
		INSERT INTO fleet_partner_contract_documents
			(id, organization_id, contract_id, contract_version_id, media_object_id,
			 document_purpose, is_mandatory, status_projection, readiness_state,
			 retention_reference, source_version, last_event_id, synced_at, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, now(), $13)
		ON CONFLICT (organization_id, contract_id, media_object_id, document_purpose)
		DO UPDATE SET status_projection = EXCLUDED.status_projection,
		              readiness_state = EXCLUDED.readiness_state,
		              source_version = GREATEST(
		                  fleet_partner_contract_documents.source_version,
		                  EXCLUDED.source_version),
		              synced_at = now(), version = fleet_partner_contract_documents.version + 1,
		              updated_at = now()
		RETURNING `+contractDocumentColumns,
		doc.ID, doc.OrganizationID, doc.SubjectID, nullable(doc.SubjectVersionID),
		doc.MediaObjectID, doc.DocumentPurpose, doc.IsMandatory, doc.StatusProjection,
		doc.ReadinessState, doc.RetentionReference, doc.SourceVersion, doc.LastEventID,
		doc.CreatedBy)
	created, err := scanDocument(row)
	if err != nil {
		return created, classify(err, "attach contract document")
	}
	return created, nil
}

// GetContractDocument reads one contract document reference.
func (r *Repository) GetContractDocument(ctx context.Context, q database.Querier,
	organizationID, contractID, documentID string) (models.DocumentReference, error) {
	return scanDocument(r.q(q).QueryRow(ctx, `SELECT `+contractDocumentColumns+`
		FROM fleet_partner_contract_documents
		WHERE organization_id = $1 AND contract_id = $2 AND id = $3`,
		organizationID, contractID, documentID))
}

// ListContractDocuments returns every document reference for a contract.
func (r *Repository) ListContractDocuments(ctx context.Context, q database.Querier,
	organizationID, contractID string) ([]models.DocumentReference, error) {
	rows, err := r.q(q).Query(ctx, `SELECT `+contractDocumentColumns+`
		FROM fleet_partner_contract_documents
		WHERE organization_id = $1 AND contract_id = $2
		ORDER BY created_at ASC, id ASC`, organizationID, contractID)
	if err != nil {
		return nil, classify(err, "list contract documents")
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

// UpdateContractDocumentProjection refreshes the local Media projection.
//
// The source version is monotonic: an older Media event can never overwrite a
// newer one, so out-of-order delivery degrades into "we already knew that"
// rather than into a document silently reverting to safe.
func (r *Repository) UpdateContractDocumentProjection(ctx context.Context, q database.Querier,
	organizationID, documentID, statusProjection, readinessState, lastEventID string,
	sourceVersion int64) error {
	_, err := r.q(q).Exec(ctx, `
		UPDATE fleet_partner_contract_documents SET
			status_projection = $3, readiness_state = $4,
			last_event_id = COALESCE(NULLIF($5, ''), last_event_id),
			source_version = GREATEST(source_version, $6),
			synced_at = now(), version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND id = $2 AND source_version <= $6`,
		organizationID, documentID, statusProjection, readinessState, lastEventID,
		sourceVersion)
	return classify(err, "update contract document projection")
}

// RemoveContractDocument detaches a document reference.
//
// This removes Fleet's pointer. It does not delete the object: physical
// deletion belongs to media-file-service's retention workflow, and a CRM
// operator detaching a draft must not be able to destroy evidence.
func (r *Repository) RemoveContractDocument(ctx context.Context, q database.Querier,
	organizationID, contractID, documentID string) error {
	tag, err := r.q(q).Exec(ctx, `
		DELETE FROM fleet_partner_contract_documents
		WHERE organization_id = $1 AND contract_id = $2 AND id = $3
		  AND contract_version_id IS NULL`,
		organizationID, contractID, documentID)
	if err != nil {
		return classify(err, "remove contract document")
	}
	if tag.RowsAffected() == 0 {
		// Either it is gone, or it is bound to an activated version and is now
		// part of an immutable record.
		return ErrNotFound
	}
	return nil
}

// BindContractDocumentsToVersion freezes the document set onto a version.
func (r *Repository) BindContractDocumentsToVersion(ctx context.Context, q database.Querier,
	organizationID, contractID, versionID string) error {
	_, err := r.q(q).Exec(ctx, `
		UPDATE fleet_partner_contract_documents
		SET contract_version_id = $3, version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND contract_id = $2 AND contract_version_id IS NULL`,
		organizationID, contractID, versionID)
	return classify(err, "bind contract documents")
}

// RequireMandatoryContractDocumentsReady refuses when any mandatory contract
// document is not confirmed safe and ready.
func (r *Repository) RequireMandatoryContractDocumentsReady(ctx context.Context,
	q database.Querier, organizationID, contractID string) error {
	var blocking int
	err := r.q(q).QueryRow(ctx, `
		SELECT count(*) FROM fleet_partner_contract_documents
		WHERE organization_id = $1 AND contract_id = $2 AND is_mandatory
		  AND NOT (status_projection = 'READY' AND readiness_state = 'READY')`,
		organizationID, contractID).Scan(&blocking)
	if err != nil {
		return classify(err, "check contract documents")
	}
	if blocking > 0 {
		return ErrDocumentNotReady
	}
	return nil
}

// ---------------------------------------------------------------------------
// Contract history and worker support
// ---------------------------------------------------------------------------

// AppendContractHistory writes one immutable contract status change.
func (r *Repository) AppendContractHistory(ctx context.Context, q database.Querier,
	entry models.ContractHistory) error {
	if entry.ID == "" {
		entry.ID = ids.New(PrefixHistory)
	}
	_, err := r.q(q).Exec(ctx, `
		INSERT INTO fleet_partner_contract_history
			(id, organization_id, contract_id, previous_status, new_status, reason_code,
			 actor_type, actor_id, correlation_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		entry.ID, entry.OrganizationID, entry.ContractID, entry.PreviousStatus,
		entry.NewStatus, entry.ReasonCode, entry.ActorType, entry.ActorID,
		entry.CorrelationID)
	return classify(err, "append contract history")
}

// ListContractHistory returns a page of contract history, newest first.
func (r *Repository) ListContractHistory(ctx context.Context, organizationID,
	contractID string, page cursor.Cursor, hasCursor bool,
	limit int) ([]models.ContractHistory, error) {
	args := []any{organizationID, contractID}
	where := `organization_id = $1 AND contract_id = $2`
	where, args = pageWhere(where, args, page, hasCursor, "occurred_at")
	args = append(args, cursor.ClampLimit(limit))

	rows, err := r.db.Query(ctx, `
		SELECT id, organization_id, contract_id, previous_status, new_status, reason_code,
		       actor_type, actor_id, correlation_id, occurred_at
		FROM fleet_partner_contract_history WHERE `+where+`
		ORDER BY occurred_at DESC, id DESC LIMIT $`+itoa(len(args)), args...)
	if err != nil {
		return nil, classify(err, "list contract history")
	}
	defer rows.Close()
	out := make([]models.ContractHistory, 0)
	for rows.Next() {
		var entry models.ContractHistory
		if scanError := rows.Scan(&entry.ID, &entry.OrganizationID, &entry.ContractID,
			&entry.PreviousStatus, &entry.NewStatus, &entry.ReasonCode, &entry.ActorType,
			&entry.ActorID, &entry.CorrelationID, &entry.OccurredAt); scanError != nil {
			return nil, scanError
		}
		out = append(out, entry)
	}
	return out, rows.Err()
}

// ClaimExpiringContracts returns contracts whose effective_to has passed.
//
// Expiry never terminates early: the predicate requires the end date to be in
// the past, so a contract with time left cannot be swept up by this query.
func (r *Repository) ClaimExpiringContracts(ctx context.Context,
	limit int) ([]models.Contract, error) {
	rows, err := r.db.Query(ctx, `SELECT `+contractColumns+`
		FROM fleet_partner_contracts
		WHERE status IN ('ACTIVE', 'SUSPENDED', 'RENEWAL_PENDING')
		  AND effective_to IS NOT NULL AND effective_to <= now()
		ORDER BY effective_to ASC, id ASC LIMIT $1`, cursor.ClampLimit(limit))
	if err != nil {
		return nil, classify(err, "claim expiring contracts")
	}
	defer rows.Close()
	out := make([]models.Contract, 0)
	for rows.Next() {
		value, scanError := scanContract(rows)
		if scanError != nil {
			return nil, scanError
		}
		out = append(out, value)
	}
	return out, rows.Err()
}

// ClaimRenewalDueContracts returns active contracts inside the renewal window.
func (r *Repository) ClaimRenewalDueContracts(ctx context.Context, window time.Duration,
	limit int) ([]models.Contract, error) {
	rows, err := r.db.Query(ctx, `SELECT `+contractColumns+`
		FROM fleet_partner_contracts
		WHERE status = 'ACTIVE' AND effective_to IS NOT NULL
		  AND effective_to > now() AND effective_to <= now() + $1::interval
		ORDER BY effective_to ASC, id ASC LIMIT $2`,
		fmt.Sprintf("%d seconds", int64(window.Seconds())), cursor.ClampLimit(limit))
	if err != nil {
		return nil, classify(err, "claim renewal due contracts")
	}
	defer rows.Close()
	out := make([]models.Contract, 0)
	for rows.Next() {
		value, scanError := scanContract(rows)
		if scanError != nil {
			return nil, scanError
		}
		out = append(out, value)
	}
	return out, rows.Err()
}

// CountContractsByStatus reports per-status contract totals for metrics.
func (r *Repository) CountContractsByStatus(ctx context.Context) (map[string]int64, error) {
	rows, err := r.db.Query(ctx, `SELECT status, count(*)
		FROM fleet_partner_contracts GROUP BY status`)
	if err != nil {
		return nil, classify(err, "count contracts")
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
