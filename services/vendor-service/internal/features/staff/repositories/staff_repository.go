// Package staffrepositories implements Postgres persistence for Staff IAM.
//
// Tenant isolation lives HERE, not only in the HTTP layer (Section 5's own
// non-negotiable rule 17/24): every query that reads or writes a staff/role/
// assignment row is scoped by vendor_id as part of the WHERE clause itself —
// never "load by id, then check vendor_id in Go." A row belonging to another
// vendor is indistinguishable from a nonexistent row at the SQL level, which
// is what makes it safe against both direct-object-reference and
// enumeration attacks (see docs/architecture/09_SECURITY_AND_AUTHORITY_MODEL.md).
package staffrepositories

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	staffmodels "cosmicforge/logistics/services/vendor-service/internal/features/staff/models"
)

var ErrNotFound = errors.New("staffrepositories: not found")
var ErrVersionConflict = errors.New("staffrepositories: version conflict")

type StaffRepository struct {
	db *pgxpool.Pool
}

func NewStaffRepository(db *pgxpool.Pool) *StaffRepository {
	return &StaffRepository{db: db}
}

const staffColumns = `id, vendor_id, user_id, status, suspended_at, suspended_reason, created_at, updated_at, version,
	(SELECT COALESCE(u.phone, '') FROM vendor_users u WHERE u.id = vendor_staff.user_id),
	(SELECT COALESCE(u.email, '') FROM vendor_users u WHERE u.id = vendor_staff.user_id)`

func (r *StaffRepository) Create(ctx context.Context, s staffmodels.Staff) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO vendor_staff (id, vendor_id, user_id, status)
		VALUES ($1,$2,$3,$4)`, s.ID, s.VendorID, s.UserID, s.Status)
	return err
}

// GetByIDForVendor loads a staff row scoped to vendor_id. Returns ErrNotFound
// both when the id doesn't exist AND when it belongs to a different vendor —
// deliberately the same outcome for both, so a caller can never distinguish
// "wrong tenant" from "doesn't exist" through this repository's behavior.
func (r *StaffRepository) GetByIDForVendor(ctx context.Context, vendorID, id string) (staffmodels.Staff, error) {
	row := r.db.QueryRow(ctx, `SELECT `+staffColumns+` FROM vendor_staff WHERE id = $1 AND vendor_id = $2`, id, vendorID)
	s, err := scanStaff(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return staffmodels.Staff{}, fmt.Errorf("staff %s: %w", id, ErrNotFound)
	}
	return s, err
}

// GetByID loads a staff row with no vendor filter — used only by the
// authentication/session-resolution path (which must first discover which
// vendor a session belongs to) and by internal provisioning. Every other
// caller must use GetByIDForVendor.
func (r *StaffRepository) GetByID(ctx context.Context, id string) (staffmodels.Staff, error) {
	row := r.db.QueryRow(ctx, `SELECT `+staffColumns+` FROM vendor_staff WHERE id = $1`, id)
	s, err := scanStaff(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return staffmodels.Staff{}, fmt.Errorf("staff %s: %w", id, ErrNotFound)
	}
	return s, err
}

// GetByVendorAndUser finds an existing staff record for a user at a specific
// vendor, if any — used both to enforce "one staff row per (vendor, user)"
// and to resolve which vendor a freshly-verified login identity should act
// as when they have exactly one.
func (r *StaffRepository) GetByVendorAndUser(ctx context.Context, vendorID, userID string) (staffmodels.Staff, error) {
	row := r.db.QueryRow(ctx, `SELECT `+staffColumns+` FROM vendor_staff WHERE vendor_id = $1 AND user_id = $2`, vendorID, userID)
	s, err := scanStaff(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return staffmodels.Staff{}, fmt.Errorf("staff for vendor %s user %s: %w", vendorID, userID, ErrNotFound)
	}
	return s, err
}

// ListByUser returns every vendor a given login identity has active staff
// access to — used at login to decide whether a vendor selection prompt is
// needed (see internal/features/auth/usecases).
func (r *StaffRepository) ListByUser(ctx context.Context, userID string) ([]staffmodels.Staff, error) {
	rows, err := r.db.Query(ctx, `SELECT `+staffColumns+` FROM vendor_staff WHERE user_id = $1 AND status = $2 ORDER BY created_at`,
		userID, staffmodels.StaffStatusActive)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []staffmodels.Staff
	for rows.Next() {
		s, err := scanStaff(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *StaffRepository) ListByVendor(ctx context.Context, vendorID string) ([]staffmodels.Staff, error) {
	rows, err := r.db.Query(ctx, `SELECT `+staffColumns+` FROM vendor_staff WHERE vendor_id = $1 ORDER BY created_at`, vendorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []staffmodels.Staff
	for rows.Next() {
		s, err := scanStaff(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// SetStatus is a version-guarded transition (optimistic concurrency): a
// stale expectedVersion is refused as ErrVersionConflict rather than
// silently overwriting a concurrent change (e.g. two admins suspending the
// same staff member at once).
func (r *StaffRepository) SetStatus(ctx context.Context, vendorID, id, status string, reason *string, expectedVersion int64) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE vendor_staff SET
			status = $1,
			suspended_at = CASE WHEN $1 = 'suspended' THEN now() ELSE NULL END,
			suspended_reason = CASE WHEN $1 = 'suspended' THEN $2 ELSE NULL END,
			updated_at = now(),
			version = version + 1
		WHERE id = $3 AND vendor_id = $4 AND version = $5`,
		status, reason, id, vendorID, expectedVersion)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrVersionConflict
	}
	return nil
}

func scanStaff(row pgx.Row) (staffmodels.Staff, error) {
	var s staffmodels.Staff
	err := row.Scan(&s.ID, &s.VendorID, &s.UserID, &s.Status, &s.SuspendedAt, &s.SuspendedReason, &s.CreatedAt, &s.UpdatedAt, &s.Version, &s.Phone, &s.Email)
	return s, err
}
