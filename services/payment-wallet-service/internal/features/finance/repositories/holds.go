package repositories

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/payment-wallet-service/internal/database"
	"cosmicforge/logistics/services/payment-wallet-service/internal/features/finance/models"
	"cosmicforge/logistics/services/payment-wallet-service/internal/platform/ids"
)

// HoldRepository owns fund holds.
type HoldRepository struct {
	db database.Querier
}

// NewHoldRepository wires the repository.
func NewHoldRepository(db database.Querier) *HoldRepository {
	return &HoldRepository{db: db}
}

const holdColumns = `id, organization_id, subject_type, COALESCE(wallet_id, ''),
	COALESCE(earning_account_id, ''), reason_code, reason, amount_minor, currency,
	authority_service, authoritative_reference, reference_verified, state, expires_at,
	released_at, COALESCE(journal_id, ''), COALESCE(release_journal_id, ''),
	idempotency_key, request_fingerprint, correlation_id, causation_id,
	version, created_at, updated_at`

func scanHold(row pgx.Row) (*models.Hold, error) {
	var hold models.Hold
	err := row.Scan(&hold.ID, &hold.OrganizationID, &hold.SubjectType, &hold.WalletID,
		&hold.EarningAccountID, &hold.ReasonCode, &hold.Reason, &hold.AmountMinor,
		&hold.Currency, &hold.AuthorityService, &hold.AuthoritativeReference,
		&hold.ReferenceVerified, &hold.State, &hold.ExpiresAt, &hold.ReleasedAt,
		&hold.JournalID, &hold.ReleaseJournalID, &hold.IdempotencyKey,
		&hold.RequestFingerprint, &hold.CorrelationID, &hold.CausationID,
		&hold.Version, &hold.CreatedAt, &hold.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("finance: scan hold: %w", err)
	}
	return &hold, nil
}

// Create inserts a hold.
//
// The table's CHECK constraint refuses an unverified authoritative reference
// without an expiry, so an unconfirmed dispute cannot freeze money forever.
func (r *HoldRepository) Create(ctx context.Context, q database.Querier, hold *models.Hold) error {
	if q == nil {
		q = r.db
	}
	if hold.ID == "" {
		hold.ID = ids.New(ids.PrefixHold)
	}
	if hold.State == "" {
		hold.State = models.HoldProvisional
	}
	err := q.QueryRow(ctx, `
		INSERT INTO finance_holds
			(id, organization_id, subject_type, wallet_id, earning_account_id, reason_code,
			 reason, amount_minor, currency, authority_service, authoritative_reference,
			 reference_verified, state, expires_at, journal_id, idempotency_key,
			 request_fingerprint, correlation_id, causation_id, version, created_at, updated_at)
		VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), $6, $7, $8, $9, $10, $11, $12,
		        $13, $14, NULLIF($15, ''), $16, $17, $18, $19, 1, now(), now())
		RETURNING version, created_at, updated_at`,
		hold.ID, hold.OrganizationID, hold.SubjectType, hold.WalletID, hold.EarningAccountID,
		hold.ReasonCode, hold.Reason, hold.AmountMinor, hold.Currency, hold.AuthorityService,
		hold.AuthoritativeReference, hold.ReferenceVerified, hold.State, hold.ExpiresAt,
		hold.JournalID, hold.IdempotencyKey, hold.RequestFingerprint, hold.CorrelationID,
		hold.CausationID).
		Scan(&hold.Version, &hold.CreatedAt, &hold.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return fmt.Errorf("finance: create hold: %w", err)
	}
	return nil
}

// Get loads a hold inside a tenant scope.
func (r *HoldRepository) Get(ctx context.Context, q database.Querier,
	organizationID string, id string) (*models.Hold, error) {
	if q == nil {
		q = r.db
	}
	return scanHold(q.QueryRow(ctx,
		`SELECT `+holdColumns+` FROM finance_holds
		 WHERE id = $1 AND ($2 = '' OR organization_id = $2)`, id, organizationID))
}

// Lock loads a hold FOR UPDATE.
func (r *HoldRepository) Lock(ctx context.Context, q database.Querier, id string) (*models.Hold, error) {
	return scanHold(q.QueryRow(ctx,
		`SELECT `+holdColumns+` FROM finance_holds WHERE id = $1 FOR UPDATE`, id))
}

// FindByIdempotencyKey resolves a replayed hold request.
func (r *HoldRepository) FindByIdempotencyKey(ctx context.Context, q database.Querier,
	organizationID string, key string) (*models.Hold, error) {
	if q == nil {
		q = r.db
	}
	if key == "" {
		return nil, ErrNotFound
	}
	return scanHold(q.QueryRow(ctx,
		`SELECT `+holdColumns+` FROM finance_holds
		 WHERE organization_id = $1 AND idempotency_key = $2`, organizationID, key))
}

// Transition moves a hold under an expected version, optionally attaching the
// releasing journal.
func (r *HoldRepository) Transition(ctx context.Context, q database.Querier, id string,
	expectedVersion int64, state string, releaseJournalID string) error {
	tag, err := q.Exec(ctx, `
		UPDATE finance_holds SET
			state = $3,
			release_journal_id = COALESCE(NULLIF($4, ''), release_journal_id),
			released_at = CASE WHEN $3 IN ('RELEASED', 'EXPIRED', 'CANCELLED')
			                   THEN now() ELSE released_at END,
			version = version + 1,
			updated_at = now()
		WHERE id = $1 AND version = $2 AND state IN ('PROVISIONAL', 'ACTIVE')`,
		id, expectedVersion, state, releaseJournalID)
	if err != nil {
		return fmt.Errorf("finance: transition hold: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrVersionMismatch
	}
	return nil
}

// Confirm promotes a provisional hold to active once its reference is
// verified.
func (r *HoldRepository) Confirm(ctx context.Context, q database.Querier, id string,
	expectedVersion int64) error {
	tag, err := q.Exec(ctx, `
		UPDATE finance_holds SET
			state = 'ACTIVE', reference_verified = true, version = version + 1, updated_at = now()
		WHERE id = $1 AND version = $2 AND state = 'PROVISIONAL'`, id, expectedVersion)
	if err != nil {
		return fmt.Errorf("finance: confirm hold: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrVersionMismatch
	}
	return nil
}

// ExpiredHolds returns live holds whose expiry has passed.
func (r *HoldRepository) ExpiredHolds(ctx context.Context, q database.Querier,
	now time.Time, limit int) ([]models.Hold, error) {
	if q == nil {
		q = r.db
	}
	if limit <= 0 {
		limit = 50
	}
	rows, err := q.Query(ctx, `
		SELECT `+holdColumns+` FROM finance_holds
		WHERE state IN ('PROVISIONAL', 'ACTIVE') AND expires_at IS NOT NULL AND expires_at <= $1
		ORDER BY expires_at LIMIT $2`, now, limit)
	if err != nil {
		return nil, fmt.Errorf("finance: list expired holds: %w", err)
	}
	defer rows.Close()
	return collectHolds(rows)
}

// ListForSubject returns the holds attached to a wallet or earning account.
func (r *HoldRepository) ListForSubject(ctx context.Context, q database.Querier,
	organizationID string, subjectType string, subjectID string) ([]models.Hold, error) {
	if q == nil {
		q = r.db
	}
	rows, err := q.Query(ctx, `
		SELECT `+holdColumns+` FROM finance_holds
		WHERE organization_id = $1 AND subject_type = $2
		  AND COALESCE(wallet_id, earning_account_id) = $3
		ORDER BY created_at DESC LIMIT 200`, organizationID, subjectType, subjectID)
	if err != nil {
		return nil, fmt.Errorf("finance: list holds: %w", err)
	}
	defer rows.Close()
	return collectHolds(rows)
}

func collectHolds(rows pgx.Rows) ([]models.Hold, error) {
	var holds []models.Hold
	for rows.Next() {
		var hold models.Hold
		if err := rows.Scan(&hold.ID, &hold.OrganizationID, &hold.SubjectType, &hold.WalletID,
			&hold.EarningAccountID, &hold.ReasonCode, &hold.Reason, &hold.AmountMinor,
			&hold.Currency, &hold.AuthorityService, &hold.AuthoritativeReference,
			&hold.ReferenceVerified, &hold.State, &hold.ExpiresAt, &hold.ReleasedAt,
			&hold.JournalID, &hold.ReleaseJournalID, &hold.IdempotencyKey,
			&hold.RequestFingerprint, &hold.CorrelationID, &hold.CausationID,
			&hold.Version, &hold.CreatedAt, &hold.UpdatedAt); err != nil {
			return nil, fmt.Errorf("finance: scan hold: %w", err)
		}
		holds = append(holds, hold)
	}
	return holds, rows.Err()
}
