package profilerepositories

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	profilemodels "cosmicforge/logistics/services/customer-service/internal/features/profile/models"
	"cosmicforge/logistics/shared/go/apperrors"
)

var (
	// ErrContactVersionConflict means the expected version did not match.
	ErrContactVersionConflict = errors.New("trusted contact version conflict")
	// ErrContactRequestKeyReused means an idempotency key was reused with a different body.
	ErrContactRequestKeyReused = errors.New("idempotency key reused with a different request")
)

// TrustedContactRepository reads and writes the same emergency_contacts rows
// that CustomerRepository's emergency-contact methods use. It is a separate
// interface only so existing CustomerRepository fakes stay valid.
type TrustedContactRepository interface {
	ListTrustedContacts(ctx context.Context, customerID string) ([]profilemodels.TrustedContact, error)
	CreateTrustedContact(ctx context.Context, contact profilemodels.TrustedContact, requestKey, fingerprint string) (profilemodels.TrustedContact, bool, error)
	UpdateTrustedContact(ctx context.Context, contact profilemodels.TrustedContact, expectedVersion int64) (profilemodels.TrustedContact, error)
	DeactivateTrustedContact(ctx context.Context, customerID, id string) error
}

const trustedContactColumns = `id::text, customer_id::text, name, phone, NULLIF(relationship, ''),
	emergency_alerts_enabled, trip_sharing_enabled, active, version, created_at, updated_at`

func scanTrustedContact(row pgx.Row, extra ...any) (profilemodels.TrustedContact, error) {
	var t profilemodels.TrustedContact
	dest := append([]any{&t.ID, &t.CustomerID, &t.Name, &t.Phone, &t.RelationshipLabel,
		&t.EmergencyAlertsEnabled, &t.TripSharingEnabled, &t.Active, &t.Version, &t.CreatedAt, &t.UpdatedAt}, extra...)
	return t, row.Scan(dest...)
}

func (r *PostgresCustomerRepository) ListTrustedContacts(ctx context.Context, customerID string) ([]profilemodels.TrustedContact, error) {
	rows, err := r.db.Query(ctx, `SELECT `+trustedContactColumns+` FROM emergency_contacts WHERE customer_id=$1 AND active ORDER BY created_at, id`, customerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]profilemodels.TrustedContact, 0)
	for rows.Next() {
		contact, err := scanTrustedContact(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, contact)
	}
	return out, rows.Err()
}

// CreateTrustedContact is idempotent per (customer, request key).
func (r *PostgresCustomerRepository) CreateTrustedContact(ctx context.Context, t profilemodels.TrustedContact, requestKey, fingerprint string) (profilemodels.TrustedContact, bool, error) {
	var key, fp any
	if requestKey != "" {
		key, fp = requestKey, fingerprint
	}
	relationship := ""
	if t.RelationshipLabel != nil {
		relationship = *t.RelationshipLabel
	}
	created, err := scanTrustedContact(r.db.QueryRow(ctx, `
		INSERT INTO emergency_contacts (customer_id, name, phone, relationship, emergency_alerts_enabled,
			trip_sharing_enabled, client_request_key, request_fingerprint)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (customer_id, client_request_key) WHERE client_request_key IS NOT NULL DO NOTHING
		RETURNING `+trustedContactColumns,
		t.CustomerID, t.Name, t.Phone, relationship, t.EmergencyAlertsEnabled, t.TripSharingEnabled, key, fp))
	if err == nil || !errors.Is(err, pgx.ErrNoRows) || requestKey == "" {
		return created, false, err
	}
	var existingFingerprint string
	existing, err := scanTrustedContact(r.db.QueryRow(ctx, `SELECT `+trustedContactColumns+`, COALESCE(request_fingerprint,'') FROM emergency_contacts WHERE customer_id=$1 AND client_request_key=$2`, t.CustomerID, requestKey), &existingFingerprint)
	if err != nil {
		return existing, false, err
	}
	if existingFingerprint != fingerprint {
		return profilemodels.TrustedContact{}, false, ErrContactRequestKeyReused
	}
	return existing, true, nil
}

func (r *PostgresCustomerRepository) UpdateTrustedContact(ctx context.Context, t profilemodels.TrustedContact, expectedVersion int64) (profilemodels.TrustedContact, error) {
	relationship := ""
	if t.RelationshipLabel != nil {
		relationship = *t.RelationshipLabel
	}
	updated, err := scanTrustedContact(r.db.QueryRow(ctx, `
		UPDATE emergency_contacts SET name=$3, phone=$4, relationship=$5, emergency_alerts_enabled=$6,
			trip_sharing_enabled=$7, version=version+1, updated_at=NOW()
		WHERE id=$1 AND customer_id=$2 AND active AND version=$8
		RETURNING `+trustedContactColumns,
		t.ID, t.CustomerID, t.Name, t.Phone, relationship, t.EmergencyAlertsEnabled, t.TripSharingEnabled, expectedVersion))
	if !errors.Is(err, pgx.ErrNoRows) {
		return updated, err
	}
	var exists bool
	if err := r.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM emergency_contacts WHERE id=$1 AND customer_id=$2 AND active)`, t.ID, t.CustomerID).Scan(&exists); err != nil {
		return profilemodels.TrustedContact{}, err
	}
	if !exists {
		return profilemodels.TrustedContact{}, apperrors.NotFound("Contact not found.", nil)
	}
	return profilemodels.TrustedContact{}, ErrContactVersionConflict
}

// DeactivateTrustedContact soft-deletes so historical safety references
// stay resolvable. The legacy /emergency-contacts DELETE keeps its original
// hard-delete behaviour.
func (r *PostgresCustomerRepository) DeactivateTrustedContact(ctx context.Context, customerID, id string) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE emergency_contacts
		   SET active=FALSE, trip_sharing_enabled=FALSE, emergency_alerts_enabled=FALSE,
		       version=CASE WHEN active THEN version+1 ELSE version END,
		       updated_at=CASE WHEN active THEN NOW() ELSE updated_at END
		 WHERE id=$1 AND customer_id=$2`, id, customerID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return apperrors.NotFound("Contact not found.", nil)
	}
	return nil
}
