package mobilityrepositories

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	mobilitymodels "cosmicforge/logistics/services/customer-service/internal/features/mobility_context/models"
	"cosmicforge/logistics/shared/go/apperrors"
)

// ErrVersionConflict means the optimistic expected version did not match.
var ErrVersionConflict = errors.New("mobility context version conflict")

// ErrRequestKeyReused means an idempotency key was replayed with a different body.
var ErrRequestKeyReused = errors.New("idempotency key reused with a different request")

type PostgresRepository struct{ db *pgxpool.Pool }

func NewPostgresRepository(db *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{db: db}
}

const preferenceColumns = `customer_id::text, accessibility_required, wheelchair_required, child_seat_preference,
	pet_travel_preference, luggage_default, preferred_passenger_count, communication_preferences,
	safety_share_default, special_assistance_notes, version, created_at, updated_at`

func scanPreferences(row pgx.Row) (mobilitymodels.MobilityPreferences, error) {
	var p mobilitymodels.MobilityPreferences
	var communication []byte
	err := row.Scan(&p.CustomerID, &p.AccessibilityRequired, &p.WheelchairRequired, &p.ChildSeatPreference,
		&p.PetTravelPreference, &p.LuggageDefault, &p.PreferredPassengerCount, &communication,
		&p.SafetyShareDefault, &p.SpecialAssistanceNotes, &p.Version, &p.CreatedAt, &p.UpdatedAt)
	p.CommunicationPreferences = json.RawMessage(communication)
	return p, err
}

func (r *PostgresRepository) GetPreferences(ctx context.Context, customerID string) (mobilitymodels.MobilityPreferences, bool, error) {
	p, err := scanPreferences(r.db.QueryRow(ctx, `SELECT `+preferenceColumns+` FROM customer_mobility_preferences WHERE customer_id=$1`, customerID))
	if errors.Is(err, pgx.ErrNoRows) {
		return mobilitymodels.MobilityPreferences{}, false, nil
	}
	return p, err == nil, err
}

// SavePreferences creates the row when expectedVersion is 0 and otherwise
// performs a compare-and-set on version. Any mismatch is ErrVersionConflict.
func (r *PostgresRepository) SavePreferences(ctx context.Context, p mobilitymodels.MobilityPreferences, expectedVersion int64) (mobilitymodels.MobilityPreferences, error) {
	args := []any{p.CustomerID, p.AccessibilityRequired, p.WheelchairRequired, p.ChildSeatPreference,
		p.PetTravelPreference, p.LuggageDefault, p.PreferredPassengerCount, []byte(p.CommunicationPreferences),
		p.SafetyShareDefault, p.SpecialAssistanceNotes}
	var row pgx.Row
	if expectedVersion == 0 {
		row = r.db.QueryRow(ctx, `
			INSERT INTO customer_mobility_preferences (customer_id, accessibility_required, wheelchair_required,
				child_seat_preference, pet_travel_preference, luggage_default, preferred_passenger_count,
				communication_preferences, safety_share_default, special_assistance_notes)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
			ON CONFLICT (customer_id) DO NOTHING
			RETURNING `+preferenceColumns, args...)
	} else {
		row = r.db.QueryRow(ctx, `
			UPDATE customer_mobility_preferences SET accessibility_required=$2, wheelchair_required=$3,
				child_seat_preference=$4, pet_travel_preference=$5, luggage_default=$6,
				preferred_passenger_count=$7, communication_preferences=$8, safety_share_default=$9,
				special_assistance_notes=$10, version=version+1, updated_at=NOW()
			WHERE customer_id=$1 AND version=$11
			RETURNING `+preferenceColumns, append(args, expectedVersion)...)
	}
	saved, err := scanPreferences(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return mobilitymodels.MobilityPreferences{}, ErrVersionConflict
	}
	return saved, err
}

const guestColumns = `id::text, customer_id::text, name, phone, relationship_label, accessibility_notes,
	mobility_requirements_json, active, version, created_at, updated_at`

func scanGuest(row pgx.Row) (mobilitymodels.GuestTraveller, error) {
	var g mobilitymodels.GuestTraveller
	var requirements []byte
	err := row.Scan(&g.ID, &g.CustomerID, &g.Name, &g.Phone, &g.RelationshipLabel, &g.AccessibilityNotes,
		&requirements, &g.Active, &g.Version, &g.CreatedAt, &g.UpdatedAt)
	if len(requirements) > 0 {
		g.MobilityRequirementsJSON = json.RawMessage(requirements)
	}
	return g, err
}

func (r *PostgresRepository) ListGuests(ctx context.Context, customerID string) ([]mobilitymodels.GuestTraveller, error) {
	rows, err := r.db.Query(ctx, `SELECT `+guestColumns+` FROM customer_guest_travellers WHERE customer_id=$1 AND active ORDER BY created_at, id`, customerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]mobilitymodels.GuestTraveller, 0)
	for rows.Next() {
		g, err := scanGuest(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// GetGuest is scoped by customer: another customer's id reads as not found.
func (r *PostgresRepository) GetGuest(ctx context.Context, customerID, id string) (mobilitymodels.GuestTraveller, error) {
	g, err := scanGuest(r.db.QueryRow(ctx, `SELECT `+guestColumns+` FROM customer_guest_travellers WHERE id=$1 AND customer_id=$2 AND active`, id, customerID))
	if errors.Is(err, pgx.ErrNoRows) {
		return g, apperrors.NotFound("Traveller not found.", nil)
	}
	return g, err
}

// CreateGuest is idempotent per (customer, request key): a replay with the
// same fingerprint returns the original row, a different body is rejected.
func (r *PostgresRepository) CreateGuest(ctx context.Context, g mobilitymodels.GuestTraveller, requestKey, fingerprint string) (mobilitymodels.GuestTraveller, bool, error) {
	var key, fp any
	if requestKey != "" {
		key, fp = requestKey, fingerprint
	}
	created, err := scanGuest(r.db.QueryRow(ctx, `
		INSERT INTO customer_guest_travellers (customer_id, name, phone, relationship_label, accessibility_notes,
			mobility_requirements_json, client_request_key, request_fingerprint)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (customer_id, client_request_key) WHERE client_request_key IS NOT NULL DO NOTHING
		RETURNING `+guestColumns,
		g.CustomerID, g.Name, g.Phone, g.RelationshipLabel, g.AccessibilityNotes, nullableJSON(g.MobilityRequirementsJSON), key, fp))
	if err == nil {
		return created, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) || requestKey == "" {
		return created, false, err
	}
	var existingFingerprint string
	existing, err := scanGuestWithFingerprint(r.db.QueryRow(ctx, `SELECT `+guestColumns+`, COALESCE(request_fingerprint,'') FROM customer_guest_travellers WHERE customer_id=$1 AND client_request_key=$2`, g.CustomerID, requestKey), &existingFingerprint)
	if err != nil {
		return existing, false, err
	}
	if existingFingerprint != fingerprint {
		return mobilitymodels.GuestTraveller{}, false, ErrRequestKeyReused
	}
	return existing, true, nil
}

func scanGuestWithFingerprint(row pgx.Row, fingerprint *string) (mobilitymodels.GuestTraveller, error) {
	var g mobilitymodels.GuestTraveller
	var requirements []byte
	err := row.Scan(&g.ID, &g.CustomerID, &g.Name, &g.Phone, &g.RelationshipLabel, &g.AccessibilityNotes,
		&requirements, &g.Active, &g.Version, &g.CreatedAt, &g.UpdatedAt, fingerprint)
	if len(requirements) > 0 {
		g.MobilityRequirementsJSON = json.RawMessage(requirements)
	}
	return g, err
}

func (r *PostgresRepository) UpdateGuest(ctx context.Context, g mobilitymodels.GuestTraveller, expectedVersion int64) (mobilitymodels.GuestTraveller, error) {
	updated, err := scanGuest(r.db.QueryRow(ctx, `
		UPDATE customer_guest_travellers SET name=$3, phone=$4, relationship_label=$5, accessibility_notes=$6,
			mobility_requirements_json=$7, version=version+1, updated_at=NOW()
		WHERE id=$1 AND customer_id=$2 AND active AND version=$8
		RETURNING `+guestColumns,
		g.ID, g.CustomerID, g.Name, g.Phone, g.RelationshipLabel, g.AccessibilityNotes, nullableJSON(g.MobilityRequirementsJSON), expectedVersion))
	if !errors.Is(err, pgx.ErrNoRows) {
		return updated, err
	}
	if _, getErr := r.GetGuest(ctx, g.CustomerID, g.ID); getErr != nil {
		return mobilitymodels.GuestTraveller{}, getErr
	}
	return mobilitymodels.GuestTraveller{}, ErrVersionConflict
}

// DeactivateGuest soft-deletes. Repeating it is a no-op success; a foreign
// or unknown id is not found.
func (r *PostgresRepository) DeactivateGuest(ctx context.Context, customerID, id string) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE customer_guest_travellers
		   SET active=FALSE, version=CASE WHEN active THEN version+1 ELSE version END,
		       updated_at=CASE WHEN active THEN NOW() ELSE updated_at END
		 WHERE id=$1 AND customer_id=$2`, id, customerID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return apperrors.NotFound("Traveller not found.", nil)
	}
	return nil
}

func nullableJSON(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	return []byte(raw)
}
