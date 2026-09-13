package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/concurrency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

// OnboardingRepository stores every onboarding-related table. Every method is
// organization scoped.
type OnboardingRepository struct {
	db database.Querier
}

// NewOnboardingRepository wires the repository.
func NewOnboardingRepository(db database.Querier) *OnboardingRepository {
	return &OnboardingRepository{db: db}
}

// ---- Profile ----

// UpsertProfile creates or compare-and-swap updates the business profile.
func (r *OnboardingRepository) UpsertProfile(ctx context.Context, q database.Querier, organizationID string, profile *models.Profile, expectedVersion int64) error {
	registered, err := json.Marshal(orEmptyMap(profile.RegisteredAddress))
	if err != nil {
		return fmt.Errorf("organizations: marshal registered address: %w", err)
	}
	operating, err := json.Marshal(orEmptyMap(profile.OperatingAddress))
	if err != nil {
		return fmt.Errorf("organizations: marshal operating address: %w", err)
	}

	if expectedVersion == 0 {
		_, err = q.Exec(ctx, `
			INSERT INTO fleet_partner_organization_profiles
				(organization_id, business_type, incorporation_date, tax_reference, registered_address,
				 operating_address, business_email, business_phone, website, business_description,
				 version, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 1, now(), now())
			ON CONFLICT (organization_id) DO NOTHING`,
			organizationID, profile.BusinessType, profile.IncorporationDate, profile.TaxReference,
			registered, operating, profile.BusinessEmail, profile.BusinessPhone,
			profile.Website, profile.BusinessDescription)
		if err != nil {
			return fmt.Errorf("organizations: insert profile: %w", err)
		}
		return nil
	}

	tag, err := q.Exec(ctx, `
		UPDATE fleet_partner_organization_profiles SET
			business_type = $2, incorporation_date = $3, tax_reference = $4,
			registered_address = $5, operating_address = $6, business_email = $7,
			business_phone = $8, website = $9, business_description = $10,
			version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND version = $11`,
		organizationID, profile.BusinessType, profile.IncorporationDate, profile.TaxReference,
		registered, operating, profile.BusinessEmail, profile.BusinessPhone,
		profile.Website, profile.BusinessDescription, expectedVersion)
	if err != nil {
		return fmt.Errorf("organizations: update profile: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return concurrency.ErrVersionMismatch
	}
	return nil
}

// GetProfile loads the business profile.
func (r *OnboardingRepository) GetProfile(ctx context.Context, organizationID string) (*models.Profile, error) {
	var profile models.Profile
	var registered, operating []byte
	err := r.db.QueryRow(ctx, `
		SELECT organization_id, business_type, incorporation_date, tax_reference, registered_address,
			operating_address, business_email, business_phone, website, business_description,
			version, created_at, updated_at
		FROM fleet_partner_organization_profiles WHERE organization_id = $1`, organizationID).
		Scan(&profile.OrganizationID, &profile.BusinessType, &profile.IncorporationDate,
			&profile.TaxReference, &registered, &operating, &profile.BusinessEmail,
			&profile.BusinessPhone, &profile.Website, &profile.BusinessDescription,
			&profile.Version, &profile.CreatedAt, &profile.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("organizations: scan profile: %w", err)
	}
	_ = json.Unmarshal(registered, &profile.RegisteredAddress)
	_ = json.Unmarshal(operating, &profile.OperatingAddress)
	return &profile, nil
}

// ---- Representatives ----

// CreateRepresentative inserts an authorized representative.
func (r *OnboardingRepository) CreateRepresentative(ctx context.Context, q database.Querier, organizationID string, rep *models.Representative) error {
	if rep.ID == "" {
		rep.ID = ids.New("rep")
	}
	rep.OrganizationID = organizationID
	rep.Version = 1
	_, err := q.Exec(ctx, `
		INSERT INTO fleet_partner_organization_authorized_representatives
			(id, organization_id, full_name, role_title, email, phone, authority_type,
			 authority_document_file_id, identity_reference, is_onboarding_owner, status,
			 effective_from, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'ACTIVE', now(), 1, now(), now())`,
		rep.ID, organizationID, rep.FullName, rep.RoleTitle, rep.Email, rep.Phone,
		rep.AuthorityType, rep.AuthorityDocumentFileID, rep.IdentityReference, rep.IsOnboardingOwner)
	if err != nil {
		return fmt.Errorf("organizations: create representative: %w", err)
	}
	return nil
}

// ListRepresentatives returns the organization's representatives.
func (r *OnboardingRepository) ListRepresentatives(ctx context.Context, organizationID string) ([]models.Representative, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, organization_id, full_name, role_title, email, phone, authority_type,
			authority_document_file_id, identity_reference, is_onboarding_owner, status,
			effective_from, effective_to, version, created_at, updated_at
		FROM fleet_partner_organization_authorized_representatives
		WHERE organization_id = $1 ORDER BY created_at, id`, organizationID)
	if err != nil {
		return nil, fmt.Errorf("organizations: list representatives: %w", err)
	}
	defer rows.Close()

	var reps []models.Representative
	for rows.Next() {
		var rep models.Representative
		if err := rows.Scan(&rep.ID, &rep.OrganizationID, &rep.FullName, &rep.RoleTitle,
			&rep.Email, &rep.Phone, &rep.AuthorityType, &rep.AuthorityDocumentFileID,
			&rep.IdentityReference, &rep.IsOnboardingOwner, &rep.Status, &rep.EffectiveFrom,
			&rep.EffectiveTo, &rep.Version, &rep.CreatedAt, &rep.UpdatedAt); err != nil {
			return nil, fmt.Errorf("organizations: scan representative: %w", err)
		}
		reps = append(reps, rep)
	}
	return reps, rows.Err()
}

// UpdateRepresentative applies a compare-and-swap update.
func (r *OnboardingRepository) UpdateRepresentative(ctx context.Context, q database.Querier, organizationID string, repID string, rep *models.Representative, expectedVersion int64) error {
	tag, err := q.Exec(ctx, `
		UPDATE fleet_partner_organization_authorized_representatives SET
			full_name = $3, role_title = $4, email = $5, phone = $6, authority_type = $7,
			authority_document_file_id = $8, status = $9, effective_to = $10,
			version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND id = $2 AND version = $11`,
		organizationID, repID, rep.FullName, rep.RoleTitle, rep.Email, rep.Phone,
		rep.AuthorityType, rep.AuthorityDocumentFileID, rep.Status, rep.EffectiveTo, expectedVersion)
	if err != nil {
		return fmt.Errorf("organizations: update representative: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return concurrency.ErrVersionMismatch
	}
	return nil
}

// ---- Stages ----

// UpsertStage records progress for one onboarding stage.
func (r *OnboardingRepository) UpsertStage(ctx context.Context, q database.Querier, organizationID string, stage string, status string, missing []string, blocking []string) error {
	missingJSON, err := json.Marshal(orEmptySlice(missing))
	if err != nil {
		return fmt.Errorf("organizations: marshal missing fields: %w", err)
	}
	blockingJSON, err := json.Marshal(orEmptySlice(blocking))
	if err != nil {
		return fmt.Errorf("organizations: marshal blocking issues: %w", err)
	}
	_, err = q.Exec(ctx, `
		INSERT INTO fleet_partner_organization_onboarding_stages
			(organization_id, stage, status, missing_fields, blocking_issues, completed_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, CASE WHEN $3 = 'COMPLETE' THEN now() ELSE NULL END, now())
		ON CONFLICT (organization_id, stage) DO UPDATE SET
			status = EXCLUDED.status,
			missing_fields = EXCLUDED.missing_fields,
			blocking_issues = EXCLUDED.blocking_issues,
			completed_at = CASE WHEN EXCLUDED.status = 'COMPLETE'
				THEN COALESCE(fleet_partner_organization_onboarding_stages.completed_at, now())
				ELSE NULL END,
			updated_at = now()`,
		organizationID, stage, status, missingJSON, blockingJSON)
	if err != nil {
		return fmt.Errorf("organizations: upsert stage: %w", err)
	}
	return nil
}

// ListStages returns every stage's progress.
func (r *OnboardingRepository) ListStages(ctx context.Context, organizationID string) ([]models.OnboardingStage, error) {
	rows, err := r.db.Query(ctx, `
		SELECT organization_id, stage, status, missing_fields, blocking_issues, completed_at, updated_at
		FROM fleet_partner_organization_onboarding_stages
		WHERE organization_id = $1`, organizationID)
	if err != nil {
		return nil, fmt.Errorf("organizations: list stages: %w", err)
	}
	defer rows.Close()

	stored := map[string]models.OnboardingStage{}
	for rows.Next() {
		var stage models.OnboardingStage
		var missing, blocking []byte
		if err := rows.Scan(&stage.OrganizationID, &stage.Stage, &stage.Status, &missing,
			&blocking, &stage.CompletedAt, &stage.UpdatedAt); err != nil {
			return nil, fmt.Errorf("organizations: scan stage: %w", err)
		}
		_ = json.Unmarshal(missing, &stage.MissingFields)
		_ = json.Unmarshal(blocking, &stage.BlockingIssues)
		stored[stage.Stage] = stage
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Return every stage in canonical order, defaulting to NOT_STARTED.
	result := make([]models.OnboardingStage, 0, len(models.AllStages))
	for _, name := range models.AllStages {
		if stage, ok := stored[name]; ok {
			result = append(result, stage)
			continue
		}
		result = append(result, models.OnboardingStage{
			OrganizationID: organizationID, Stage: name,
			Status: models.StageStatusNotStarted, MissingFields: []string{}, BlockingIssues: []string{},
		})
	}
	return result, nil
}

// ---- Capabilities ----

// ReplaceCapabilities sets the organization's requested capabilities.
func (r *OnboardingRepository) ReplaceCapabilities(ctx context.Context, q database.Querier, organizationID string, capabilities []string) error {
	if _, err := q.Exec(ctx, `
		UPDATE fleet_partner_organization_capabilities SET status = 'WITHDRAWN', updated_at = now()
		WHERE organization_id = $1 AND status IN ('REQUESTED', 'APPROVED') AND capability <> ALL($2::text[])`,
		organizationID, orEmptySlice(capabilities)); err != nil {
		return fmt.Errorf("organizations: withdraw capabilities: %w", err)
	}
	for _, capability := range capabilities {
		if _, err := q.Exec(ctx, `
			INSERT INTO fleet_partner_organization_capabilities
				(organization_id, capability, status, requires_review, version, created_at, updated_at)
			VALUES ($1, $2, 'REQUESTED', $3, 1, now(), now())
			ON CONFLICT (organization_id, capability) DO UPDATE SET
				status = CASE WHEN fleet_partner_organization_capabilities.status = 'APPROVED'
					THEN 'APPROVED' ELSE 'REQUESTED' END,
				version = fleet_partner_organization_capabilities.version + 1,
				updated_at = now()`,
			organizationID, capability, models.CapabilitiesRequiringReview[capability]); err != nil {
			return fmt.Errorf("organizations: upsert capability: %w", err)
		}
	}
	return nil
}

// ListCapabilities returns the organization's capabilities.
func (r *OnboardingRepository) ListCapabilities(ctx context.Context, organizationID string) (map[string]string, error) {
	rows, err := r.db.Query(ctx, `
		SELECT capability, status FROM fleet_partner_organization_capabilities
		WHERE organization_id = $1 AND status <> 'WITHDRAWN' ORDER BY capability`, organizationID)
	if err != nil {
		return nil, fmt.Errorf("organizations: list capabilities: %w", err)
	}
	defer rows.Close()

	capabilities := map[string]string{}
	for rows.Next() {
		var capability, status string
		if err := rows.Scan(&capability, &status); err != nil {
			return nil, fmt.Errorf("organizations: scan capability: %w", err)
		}
		capabilities[capability] = status
	}
	return capabilities, rows.Err()
}

// ApproveCapabilities marks reviewed capabilities approved (used when
// verification approves the organization).
func (r *OnboardingRepository) ApproveCapabilities(ctx context.Context, q database.Querier, organizationID string) error {
	_, err := q.Exec(ctx, `
		UPDATE fleet_partner_organization_capabilities SET status = 'APPROVED', version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND status = 'REQUESTED'`, organizationID)
	if err != nil {
		return fmt.Errorf("organizations: approve capabilities: %w", err)
	}
	return nil
}

// ---- Documents ----

// AttachDocument records a media-file-service file reference.
func (r *OnboardingRepository) AttachDocument(ctx context.Context, q database.Querier, organizationID string, document *models.Document) error {
	if document.ID == "" {
		document.ID = ids.New("odoc")
	}
	document.OrganizationID = organizationID
	if document.Status == "" {
		document.Status = models.DocumentStatusAttached
	}
	_, err := q.Exec(ctx, `
		INSERT INTO fleet_partner_organization_documents
			(id, organization_id, document_type, file_id, status, issued_at, expires_at,
			 source_service, source_version, synced_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'media-file-service', $8, now(), now(), now())`,
		document.ID, organizationID, document.DocumentType, document.FileID, document.Status,
		document.IssuedAt, document.ExpiresAt, document.SourceVersion)
	if err != nil {
		return fmt.Errorf("organizations: attach document: %w", err)
	}
	return nil
}

// ListDocuments returns attached documents.
// ListDocuments returns attached documents.
func (r *OnboardingRepository) ListDocuments(ctx context.Context, organizationID string) ([]models.Document, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, organization_id, document_type, file_id, verification_reference, status,
			issued_at, expires_at, source_service, source_version, last_event_id, synced_at,
			created_at, updated_at
		FROM fleet_partner_organization_documents
		WHERE organization_id = $1 AND status <> 'REMOVED'
		ORDER BY created_at, id`, organizationID)
	if err != nil {
		return nil, fmt.Errorf("organizations: list documents: %w", err)
	}
	defer rows.Close()

	var documents []models.Document
	for rows.Next() {
		var document models.Document
		if err := rows.Scan(&document.ID, &document.OrganizationID, &document.DocumentType,
			&document.FileID, &document.VerificationReference, &document.Status,
			&document.IssuedAt, &document.ExpiresAt, &document.SourceService,
			&document.SourceVersion, &document.LastEventID, &document.SyncedAt,
			&document.CreatedAt, &document.UpdatedAt); err != nil {
			return nil, fmt.Errorf("organizations: scan document: %w", err)
		}
		documents = append(documents, document)
	}
	return documents, rows.Err()
}

func (r *OnboardingRepository) ListDocumentsTx(ctx context.Context, q database.Querier, organizationID string) ([]models.Document, error) {
	rows, err := q.Query(ctx, `
		SELECT id, organization_id, document_type, file_id, verification_reference, status,
			issued_at, expires_at, source_service, source_version, last_event_id, synced_at,
			created_at, updated_at
		FROM fleet_partner_organization_documents
		WHERE organization_id = $1 AND status <> 'REMOVED'
		ORDER BY created_at, id`, organizationID)
	if err != nil {
		return nil, fmt.Errorf("organizations: list documents: %w", err)
	}
	defer rows.Close()

	var documents []models.Document
	for rows.Next() {
		var document models.Document
		if err := rows.Scan(&document.ID, &document.OrganizationID, &document.DocumentType,
			&document.FileID, &document.VerificationReference, &document.Status,
			&document.IssuedAt, &document.ExpiresAt, &document.SourceService,
			&document.SourceVersion, &document.LastEventID, &document.SyncedAt,
			&document.CreatedAt, &document.UpdatedAt); err != nil {
			return nil, fmt.Errorf("organizations: scan document: %w", err)
		}
		documents = append(documents, document)
	}
	return documents, rows.Err()
}

// RemoveDocument soft-removes an attached document.
func (r *OnboardingRepository) RemoveDocument(ctx context.Context, q database.Querier, organizationID string, documentID string) error {
	tag, err := q.Exec(ctx, `
		UPDATE fleet_partner_organization_documents SET status = 'REMOVED', updated_at = now()
		WHERE organization_id = $1 AND id = $2 AND status <> 'REMOVED'`, organizationID, documentID)
	if err != nil {
		return fmt.Errorf("organizations: remove document: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateDocumentStatus applies a media-file-service status projection with an
// out-of-order guard: an older source version never overwrites newer state.
func (r *OnboardingRepository) UpdateDocumentStatus(ctx context.Context, q database.Querier, organizationID string, fileID string, status string, sourceVersion int64, eventID string) error {
	_, err := q.Exec(ctx, `
		UPDATE fleet_partner_organization_documents SET
			status = $3, source_version = $4, last_event_id = $5, synced_at = now(), updated_at = now()
		WHERE organization_id = $1 AND file_id = $2 AND source_version <= $4`,
		organizationID, fileID, status, sourceVersion, eventID)
	if err != nil {
		return fmt.Errorf("organizations: update document status: %w", err)
	}
	return nil
}

func orEmptyMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	return value
}

func orEmptySlice(value []string) []string {
	if value == nil {
		return []string{}
	}
	return value
}

// nowPtr is a small helper for optional timestamps.
func nowPtr() *time.Time {
	now := time.Now().UTC()
	return &now
}
