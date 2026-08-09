package repositories

import (
	"context"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/cursor"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

const contactColumns = `id, organization_id, customer_id, full_name, role_title,
	COALESCE(normalized_email, ''), COALESCE(normalized_phone, ''), preferred_channel,
	contact_purpose, is_primary, notification_preferences, consent_status, consent_source,
	consent_recorded_at, status, version, created_by, updated_by, created_at, updated_at,
	archived_at`

func scanContact(row pgx.Row) (models.Contact, error) {
	var c models.Contact
	var preferences []byte
	err := row.Scan(&c.ID, &c.OrganizationID, &c.CustomerID, &c.FullName, &c.RoleTitle,
		&c.NormalizedEmail, &c.NormalizedPhone, &c.PreferredChannel, &c.ContactPurpose,
		&c.IsPrimary, &preferences, &c.ConsentStatus, &c.ConsentSource, &c.ConsentRecordedAt,
		&c.Status, &c.Version, &c.CreatedBy, &c.UpdatedBy, &c.CreatedAt, &c.UpdatedAt,
		&c.ArchivedAt)
	if err != nil {
		return c, scanErr(err)
	}
	c.NotificationPreferences = decodeJSONMap(preferences)
	return c, nil
}

// CreateContact inserts a customer contact.
func (r *Repository) CreateContact(ctx context.Context, q database.Querier,
	c models.Contact) (models.Contact, error) {
	preferences, err := encodeJSON(c.NotificationPreferences)
	if err != nil {
		return models.Contact{}, err
	}
	row := r.q(q).QueryRow(ctx, `
		INSERT INTO fleet_partner_customer_contacts
			(id, organization_id, customer_id, full_name, role_title, normalized_email,
			 normalized_phone, preferred_channel, contact_purpose, is_primary,
			 notification_preferences, consent_status, consent_source, consent_recorded_at,
			 status, created_by, updated_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $16)
		RETURNING `+contactColumns,
		c.ID, c.OrganizationID, c.CustomerID, c.FullName, c.RoleTitle,
		nullable(c.NormalizedEmail), nullable(c.NormalizedPhone), c.PreferredChannel,
		c.ContactPurpose, c.IsPrimary, preferences, c.ConsentStatus, c.ConsentSource,
		c.ConsentRecordedAt, c.Status, c.CreatedBy)
	created, err := scanContact(row)
	if err != nil {
		return created, classify(err, "create contact")
	}
	return created, nil
}

// GetContact reads one contact.
//
// The customer ID is in the WHERE clause, not just checked afterwards: a
// contact ID from another customer must be as invisible as one that does not
// exist.
func (r *Repository) GetContact(ctx context.Context, q database.Querier,
	organizationID, customerID, contactID string) (models.Contact, error) {
	return scanContact(r.q(q).QueryRow(ctx, `SELECT `+contactColumns+`
		FROM fleet_partner_customer_contacts
		WHERE organization_id = $1 AND customer_id = $2 AND id = $3`,
		organizationID, customerID, contactID))
}

// LockContact reads one contact FOR UPDATE.
func (r *Repository) LockContact(ctx context.Context, q database.Querier,
	organizationID, customerID, contactID string) (models.Contact, error) {
	return scanContact(r.q(q).QueryRow(ctx, `SELECT `+contactColumns+`
		FROM fleet_partner_customer_contacts
		WHERE organization_id = $1 AND customer_id = $2 AND id = $3 FOR UPDATE`,
		organizationID, customerID, contactID))
}

// ContactFilter is the allowlisted contact list query.
type ContactFilter struct {
	Status         string
	ContactPurpose string
	Cursor         cursor.Cursor
	HasCursor      bool
	Limit          int
}

// ListContacts returns a page of a customer's contacts, newest first.
func (r *Repository) ListContacts(ctx context.Context, organizationID, customerID string,
	filter ContactFilter) ([]models.Contact, error) {
	args := []any{organizationID, customerID, filter.Status, filter.ContactPurpose}
	where := `organization_id = $1 AND customer_id = $2
		AND ($3 = '' OR status = $3) AND ($4 = '' OR contact_purpose = $4)`
	where, args = pageWhere(where, args, filter.Cursor, filter.HasCursor, "created_at")
	args = append(args, cursor.ClampLimit(filter.Limit))

	rows, err := r.db.Query(ctx, `SELECT `+contactColumns+`
		FROM fleet_partner_customer_contacts WHERE `+where+`
		ORDER BY created_at DESC, id DESC LIMIT $`+itoa(len(args)), args...)
	if err != nil {
		return nil, classify(err, "list contacts")
	}
	defer rows.Close()
	out := make([]models.Contact, 0)
	for rows.Next() {
		value, scanError := scanContact(rows)
		if scanError != nil {
			return nil, scanError
		}
		out = append(out, value)
	}
	return out, rows.Err()
}

// PrimaryContact reads the primary contact for a purpose, if one exists.
func (r *Repository) PrimaryContact(ctx context.Context, q database.Querier,
	organizationID, customerID, purpose string) (models.Contact, error) {
	return scanContact(r.q(q).QueryRow(ctx, `SELECT `+contactColumns+`
		FROM fleet_partner_customer_contacts
		WHERE organization_id = $1 AND customer_id = $2 AND contact_purpose = $3
		  AND is_primary AND archived_at IS NULL`,
		organizationID, customerID, purpose))
}

// ContactUpdate carries the mutable contact fields.
type ContactUpdate struct {
	FullName                *string
	RoleTitle               *string
	NormalizedEmail         *string
	NormalizedPhone         *string
	PreferredChannel        *string
	ContactPurpose          *string
	NotificationPreferences models.JSONMap
	ConsentStatus           *string
	ConsentSource           *string
	Status                  *string
	UpdatedBy               string
}

// UpdateContact applies a compare-and-swap update.
func (r *Repository) UpdateContact(ctx context.Context, q database.Querier,
	organizationID, customerID, contactID string, expectedVersion int64,
	update ContactUpdate) (models.Contact, error) {
	var preferences any
	if update.NotificationPreferences != nil {
		encoded, err := encodeJSON(update.NotificationPreferences)
		if err != nil {
			return models.Contact{}, err
		}
		preferences = encoded
	}
	row := r.q(q).QueryRow(ctx, `
		UPDATE fleet_partner_customer_contacts SET
			full_name = COALESCE($5, full_name),
			role_title = COALESCE($6, role_title),
			normalized_email = COALESCE($7, normalized_email),
			normalized_phone = COALESCE($8, normalized_phone),
			preferred_channel = COALESCE($9, preferred_channel),
			contact_purpose = COALESCE($10, contact_purpose),
			notification_preferences = COALESCE($11::jsonb, notification_preferences),
			consent_status = COALESCE($12, consent_status),
			consent_source = COALESCE($13, consent_source),
			consent_recorded_at = CASE WHEN $12 IS NOT NULL AND $12 <> consent_status
			                           THEN now() ELSE consent_recorded_at END,
			status = COALESCE($14, status),
			updated_by = $15, version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND customer_id = $2 AND id = $3 AND version = $4
		  AND archived_at IS NULL
		RETURNING `+contactColumns,
		organizationID, customerID, contactID, expectedVersion,
		update.FullName, update.RoleTitle, update.NormalizedEmail, update.NormalizedPhone,
		update.PreferredChannel, update.ContactPurpose, preferences, update.ConsentStatus,
		update.ConsentSource, update.Status, update.UpdatedBy)
	updated, err := scanContact(row)
	if err != nil {
		return updated, r.classifyContactWrite(ctx, q, organizationID, customerID, contactID,
			err, "update contact")
	}
	return updated, nil
}

// ArchiveContact closes a contact.
//
// Archiving clears the primary flag in the same statement. Leaving an archived
// contact holding the primary slot would make the partial unique index refuse
// every future primary for that purpose, and the customer would silently lose
// the ability to have one.
func (r *Repository) ArchiveContact(ctx context.Context, q database.Querier,
	organizationID, customerID, contactID string, expectedVersion int64,
	actor string) (models.Contact, error) {
	row := r.q(q).QueryRow(ctx, `
		UPDATE fleet_partner_customer_contacts SET
			status = 'ARCHIVED', is_primary = false, archived_at = now(),
			updated_by = $5, version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND customer_id = $2 AND id = $3 AND version = $4
		  AND archived_at IS NULL
		RETURNING `+contactColumns,
		organizationID, customerID, contactID, expectedVersion, actor)
	updated, err := scanContact(row)
	if err != nil {
		return updated, r.classifyContactWrite(ctx, q, organizationID, customerID, contactID,
			err, "archive contact")
	}
	return updated, nil
}

// DemotePrimaryContacts clears the primary flag for a purpose.
//
// It runs before promoting the new primary inside the same transaction. The
// partial unique index is what actually decides a race between two concurrent
// promotions; this statement is what stops the winner from colliding with the
// incumbent.
func (r *Repository) DemotePrimaryContacts(ctx context.Context, q database.Querier,
	organizationID, customerID, purpose, exceptContactID string) error {
	_, err := r.q(q).Exec(ctx, `
		UPDATE fleet_partner_customer_contacts
		SET is_primary = false, version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND customer_id = $2 AND contact_purpose = $3
		  AND id <> $4 AND is_primary AND archived_at IS NULL`,
		organizationID, customerID, purpose, exceptContactID)
	return classify(err, "demote primary contacts")
}

// PromoteContact makes a contact primary for its purpose.
func (r *Repository) PromoteContact(ctx context.Context, q database.Querier,
	organizationID, customerID, contactID string, expectedVersion int64,
	actor string) (models.Contact, error) {
	row := r.q(q).QueryRow(ctx, `
		UPDATE fleet_partner_customer_contacts SET
			is_primary = true, updated_by = $5, version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND customer_id = $2 AND id = $3 AND version = $4
		  AND archived_at IS NULL AND status = 'ACTIVE'
		RETURNING `+contactColumns,
		organizationID, customerID, contactID, expectedVersion, actor)
	updated, err := scanContact(row)
	if err != nil {
		return updated, r.classifyContactWrite(ctx, q, organizationID, customerID, contactID,
			err, "promote contact")
	}
	return updated, nil
}

func (r *Repository) classifyContactWrite(ctx context.Context, q database.Querier,
	organizationID, customerID, contactID string, err error, operation string) error {
	classified := classify(err, operation)
	if classified != ErrNotFound {
		return classified
	}
	if _, getErr := r.GetContact(ctx, q, organizationID, customerID, contactID); getErr == nil {
		return ErrVersionConflict
	}
	return ErrNotFound
}

// AppendContactHistory writes one immutable contact change record.
func (r *Repository) AppendContactHistory(ctx context.Context, q database.Querier,
	entry models.ContactHistory) error {
	fields, err := encodeJSONArray(entry.ChangedFields)
	if err != nil {
		return err
	}
	if entry.ID == "" {
		entry.ID = ids.New(PrefixHistory)
	}
	_, err = r.q(q).Exec(ctx, `
		INSERT INTO fleet_partner_crm_contact_history
			(id, organization_id, customer_id, contact_id, change_type, previous_status,
			 new_status, changed_fields, reason_code, actor_type, actor_id, correlation_id,
			 causation_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		entry.ID, entry.OrganizationID, entry.CustomerID, entry.ContactID, entry.ChangeType,
		entry.PreviousStatus, entry.NewStatus, fields, entry.ReasonCode, entry.ActorType,
		entry.ActorID, entry.CorrelationID, entry.CausationID)
	return classify(err, "append contact history")
}

// CountContacts reports the total contact count for metrics.
func (r *Repository) CountContacts(ctx context.Context) (int64, error) {
	var total int64
	err := r.db.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_customer_contacts
		WHERE archived_at IS NULL`).Scan(&total)
	return total, classify(err, "count contacts")
}

// GetContactByID reads one contact by ID inside the organization.
//
// It exists for the notification worker, which holds an opaque contact
// reference and no customer ID. The organization is still in the WHERE clause:
// a worker draining a queue is not exempt from tenant scoping.
func (r *Repository) GetContactByID(ctx context.Context, q database.Querier,
	organizationID, contactID string) (models.Contact, error) {
	return scanContact(r.q(q).QueryRow(ctx, `SELECT `+contactColumns+`
		FROM fleet_partner_customer_contacts
		WHERE organization_id = $1 AND id = $2`, organizationID, contactID))
}
