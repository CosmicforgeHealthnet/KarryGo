package infra

import (
	"context"
	"errors"

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

func (r *AccountRepo) CreateAccount(ctx context.Context, params partneraccount.CreateAccountParams) (partneraccount.Account, error) {
	const q = `INSERT INTO partner_accounts (business_name, business_email, password_hash, status, plan)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, business_name, business_email, status, plan, created_at`
	var a partneraccount.Account
	err := r.pool.QueryRow(ctx, q, params.BusinessName, params.BusinessEmail, params.PasswordHash, params.Status, params.Plan).
		Scan(&a.ID, &a.BusinessName, &a.BusinessEmail, &a.Status, &a.Plan, &a.CreatedAt)
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
	const q = `SELECT id, business_name, business_email, status, plan, created_at, password_hash
		FROM partner_accounts WHERE business_email = $1`
	var a partneraccount.Account
	var passwordHash string
	err := r.pool.QueryRow(ctx, q, email).
		Scan(&a.ID, &a.BusinessName, &a.BusinessEmail, &a.Status, &a.Plan, &a.CreatedAt, &passwordHash)
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
	const q = `SELECT id, business_name, business_email, status, plan, created_at
		FROM partner_accounts WHERE id = $1`
	var a partneraccount.Account
	err := r.pool.QueryRow(ctx, q, id).
		Scan(&a.ID, &a.BusinessName, &a.BusinessEmail, &a.Status, &a.Plan, &a.CreatedAt)
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
