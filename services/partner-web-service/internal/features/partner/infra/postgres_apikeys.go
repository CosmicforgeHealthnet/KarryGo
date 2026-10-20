package infra

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/partner-web-service/internal/features/partneraccount"
	"cosmicforge/logistics/services/partner-web-service/internal/features/partnerauth"
)

// APIKeyRepo is the Postgres implementation of partnerauth.APIKeyRepository AND
// partneraccount.APIKeyStore - both interfaces are satisfied by this one struct
// since they both read/write the same partner_api_keys table.
type APIKeyRepo struct {
	pool *pgxpool.Pool
}

// NewAPIKeyRepo constructs an APIKeyRepo.
func NewAPIKeyRepo(pool *pgxpool.Pool) *APIKeyRepo { return &APIKeyRepo{pool: pool} }

// apiKeyColumns includes description, expires_at, permissions, added by
// migration 005 (the Generate API Key form's description/expiration/
// permissions fields, and what partnerauth.RequirePermission enforces against).
const apiKeyColumns = `id, partner_id, key_hash, key_prefix, environment, label, description,
	expires_at, permissions, status, last_used_at, created_at, revoked_at`

func (r *APIKeyRepo) FindByPrefix(ctx context.Context, prefix string) (*partnerauth.APIKey, error) {
	const q = `SELECT ` + apiKeyColumns + `
		FROM partner_api_keys WHERE key_prefix = $1 ORDER BY created_at DESC LIMIT 1`
	var k partnerauth.APIKey
	err := r.pool.QueryRow(ctx, q, prefix).Scan(
		&k.ID, &k.PartnerID, &k.KeyHash, &k.KeyPrefix, &k.Environment, &k.Label, &k.Description,
		&k.ExpiresAt, &k.Permissions, &k.Status, &k.LastUsedAt, &k.CreatedAt, &k.RevokedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &k, nil
}

func (r *APIKeyRepo) UpdateLastUsed(ctx context.Context, keyID string) error {
	const q = `UPDATE partner_api_keys SET last_used_at = now() WHERE id = $1`
	_, err := r.pool.Exec(ctx, q, keyID)
	return err
}

// --- partneraccount.APIKeyStore (dashboard key management) -------------------

func (r *APIKeyRepo) CreateKey(ctx context.Context, params partneraccount.CreateKeyParams) (string, error) {
	permissions := params.Permissions
	if permissions == nil {
		permissions = []string{}
	}
	const q = `INSERT INTO partner_api_keys
		(partner_id, key_hash, key_prefix, environment, label, description, expires_at, permissions, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING id`
	var id string
	err := r.pool.QueryRow(ctx, q, params.PartnerID, params.KeyHash, params.KeyPrefix,
		params.Environment, params.Label, params.Description, params.ExpiresAt, permissions,
		params.Status).Scan(&id)
	return id, err
}

func (r *APIKeyRepo) ListByPartner(ctx context.Context, partnerID string) ([]partneraccount.KeyView, error) {
	const q = `SELECT id, key_prefix, environment, coalesce(label, ''), coalesce(description, ''),
		expires_at, permissions, status, last_used_at, created_at, revoked_at
		FROM partner_api_keys WHERE partner_id = $1 ORDER BY created_at DESC`
	rows, err := r.pool.Query(ctx, q, partnerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []partneraccount.KeyView
	for rows.Next() {
		var v partneraccount.KeyView
		if err := rows.Scan(&v.ID, &v.Prefix, &v.Environment, &v.Label, &v.Description,
			&v.ExpiresAt, &v.Permissions, &v.Status, &v.LastUsedAt, &v.CreatedAt, &v.RevokedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *APIKeyRepo) RevokeByPartner(ctx context.Context, partnerID, keyID string) (bool, error) {
	if _, err := uuid.Parse(keyID); err != nil {
		return false, nil
	}
	ct, err := r.pool.Exec(ctx,
		`UPDATE partner_api_keys SET status = 'revoked', revoked_at = now() WHERE id = $1 AND partner_id = $2`,
		keyID, partnerID)
	if err != nil {
		return false, err
	}
	return ct.RowsAffected() > 0, nil
}
