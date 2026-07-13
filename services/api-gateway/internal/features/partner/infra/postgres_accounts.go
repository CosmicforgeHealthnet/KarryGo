package infra

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/api-gateway/internal/features/partneraccount"
)

// AccountRepo is the Postgres implementation of partneraccount.AccountRepository.
type AccountRepo struct {
	pool *pgxpool.Pool
}

// NewAccountRepo constructs an AccountRepo.
func NewAccountRepo(pool *pgxpool.Pool) *AccountRepo { return &AccountRepo{pool: pool} }

// accountColumns includes business_phone, state, city, business_logo_url,
// added by migration 004. All four are nullable, matching Account's pointer
// fields for them.
const accountColumns = `id, business_name, business_email, business_phone, state, city,
	business_logo_url, status, plan, created_at`

func (r *AccountRepo) CreateAccount(ctx context.Context, params partneraccount.CreateAccountParams) (partneraccount.Account, error) {
	// Profile fields (phone/state/city/logo) are not collected at registration
	// (see Register in service.go) - they start NULL and are filled in later via
	// UpdateProfile from the Settings > Profile edit form.
	const q = `INSERT INTO partner_accounts (business_name, business_email, password_hash, status, plan)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING ` + accountColumns
	var a partneraccount.Account
	err := r.pool.QueryRow(ctx, q, params.BusinessName, params.BusinessEmail, params.PasswordHash, params.Status, params.Plan).
		Scan(&a.ID, &a.BusinessName, &a.BusinessEmail, &a.BusinessPhone, &a.State, &a.City, &a.BusinessLogoURL,
			&a.Status, &a.Plan, &a.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return partneraccount.Account{}, partneraccount.ErrEmailTaken
		}
		return partneraccount.Account{}, err
	}
	return a, nil
}

func (r *AccountRepo) FindByEmail(ctx context.Context, email string) (*partneraccount.Account, string, error) {
	const q = `SELECT ` + accountColumns + `, password_hash
		FROM partner_accounts WHERE business_email = $1`
	var a partneraccount.Account
	var passwordHash string
	err := r.pool.QueryRow(ctx, q, email).
		Scan(&a.ID, &a.BusinessName, &a.BusinessEmail, &a.BusinessPhone, &a.State, &a.City, &a.BusinessLogoURL,
			&a.Status, &a.Plan, &a.CreatedAt, &passwordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	return &a, passwordHash, nil
}

func (r *AccountRepo) FindByID(ctx context.Context, id string) (*partneraccount.Account, error) {
	if _, err := uuid.Parse(id); err != nil {
		return nil, nil
	}
	const q = `SELECT ` + accountColumns + ` FROM partner_accounts WHERE id = $1`
	var a partneraccount.Account
	err := r.pool.QueryRow(ctx, q, id).
		Scan(&a.ID, &a.BusinessName, &a.BusinessEmail, &a.BusinessPhone, &a.State, &a.City, &a.BusinessLogoURL,
			&a.Status, &a.Plan, &a.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *AccountRepo) UpdatePlan(ctx context.Context, id, plan string) (bool, error) {
	if _, err := uuid.Parse(id); err != nil {
		return false, nil
	}
	ct, err := r.pool.Exec(ctx, `UPDATE partner_accounts SET plan = $2 WHERE id = $1`, id, plan)
	if err != nil {
		return false, err
	}
	return ct.RowsAffected() > 0, nil
}

func (r *AccountRepo) UpdateStatus(ctx context.Context, id, status string) (bool, error) {
	if _, err := uuid.Parse(id); err != nil {
		return false, nil
	}
	ct, err := r.pool.Exec(ctx, `UPDATE partner_accounts SET status = $2 WHERE id = $1`, id, status)
	if err != nil {
		return false, err
	}
	return ct.RowsAffected() > 0, nil
}

// UpdateProfile applies a partial update: only fields where the caller passed a
// non-nil pointer are included in the SQL SET clause, so untouched fields on
// the Settings > Profile form are never overwritten with NULL. If params has no
// non-nil fields at all, this is a no-op that just returns the current row.
func (r *AccountRepo) UpdateProfile(ctx context.Context, id string, params partneraccount.UpdateProfileParams) (*partneraccount.Account, bool, error) {
	if _, err := uuid.Parse(id); err != nil {
		return nil, false, nil
	}

	var sets []string
	args := []any{id}
	add := func(col string, val *string) {
		if val == nil {
			return
		}
		args = append(args, *val)
		sets = append(sets, col+" = $"+strconv.Itoa(len(args)))
	}
	add("business_name", params.BusinessName)
	add("business_phone", params.BusinessPhone)
	add("state", params.State)
	add("city", params.City)
	add("business_logo_url", params.BusinessLogoURL)

	if len(sets) == 0 {
		a, err := r.FindByID(ctx, id)
		if err != nil {
			return nil, false, err
		}
		if a == nil {
			return nil, false, nil
		}
		return a, true, nil
	}

	q := `UPDATE partner_accounts SET ` + strings.Join(sets, ", ") + ` WHERE id = $1
		RETURNING ` + accountColumns

	var a partneraccount.Account
	err := r.pool.QueryRow(ctx, q, args...).Scan(
		&a.ID, &a.BusinessName, &a.BusinessEmail, &a.BusinessPhone, &a.State, &a.City, &a.BusinessLogoURL,
		&a.Status, &a.Plan, &a.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &a, true, nil
}
