package repositories

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/payment-wallet-service/internal/database"
	"cosmicforge/logistics/services/payment-wallet-service/internal/features/finance/models"
	"cosmicforge/logistics/services/payment-wallet-service/internal/platform/ids"
)

// AccountRepository owns organization wallets and provider earning accounts.
type AccountRepository struct {
	db     database.Querier
	ledger *LedgerRepository
}

// NewAccountRepository wires the repository.
func NewAccountRepository(db database.Querier, ledger *LedgerRepository) *AccountRepository {
	return &AccountRepository{db: db, ledger: ledger}
}

const walletColumns = `id, organization_id, currency, status, available_minor, held_minor,
	pending_settlement_minor, restrictions, ledger_account_id, version, created_at, updated_at`

func scanWallet(row pgx.Row) (*models.Wallet, error) {
	var wallet models.Wallet
	err := row.Scan(&wallet.ID, &wallet.OrganizationID, &wallet.Currency, &wallet.Status,
		&wallet.AvailableMinor, &wallet.HeldMinor, &wallet.PendingSettlementMinor,
		&wallet.Restrictions, &wallet.LedgerAccountID, &wallet.Version,
		&wallet.CreatedAt, &wallet.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("finance: scan wallet: %w", err)
	}
	return &wallet, nil
}

// EnsureWallet returns the organization's wallet for a currency, creating it
// and its ledger account on first use.
func (r *AccountRepository) EnsureWallet(ctx context.Context, q database.Querier,
	organizationID string, currency string) (*models.Wallet, bool, error) {
	if q == nil {
		q = r.db
	}
	// Serialize first creation before allocating the linked ledger account. The
	// use-case passes its transaction, so the lock is released at atomic commit.
	if _, err := q.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, "finance-wallet:"+organizationID+":"+currency); err != nil {
		return nil, false, fmt.Errorf("finance: lock wallet creation: %w", err)
	}
	existing, err := r.GetWallet(ctx, q, organizationID, currency)
	if err == nil {
		return existing, false, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return nil, false, err
	}

	// A wallet is money owed to the organization, so its ledger account has a
	// CREDIT normal balance.
	walletID := ids.New(ids.PrefixWallet)
	account, err := r.ledger.EnsureAccount(ctx, q, organizationID,
		models.OwnerOrganizationWallet, walletID, models.AccountWalletAvailable,
		currency, models.SideCredit)
	if err != nil {
		return nil, false, err
	}

	_, err = q.Exec(ctx, `
		INSERT INTO finance_wallets
			(id, organization_id, currency, status, available_minor, held_minor,
			 pending_settlement_minor, restrictions, ledger_account_id, version, created_at, updated_at)
		VALUES ($1, $2, $3, 'ACTIVE', 0, 0, 0, '[]'::jsonb, $4, 1, now(), now())
		ON CONFLICT (organization_id, currency) DO NOTHING`,
		walletID, organizationID, currency, account.ID)
	if err != nil {
		return nil, false, fmt.Errorf("finance: create wallet: %w", err)
	}

	wallet, err := r.GetWallet(ctx, q, organizationID, currency)
	if err != nil {
		return nil, false, err
	}
	// A concurrent creator may have won; the wallet we return is whichever one
	// the unique constraint accepted.
	return wallet, wallet.ID == walletID, nil
}

// GetWallet loads a wallet by organization and currency.
func (r *AccountRepository) GetWallet(ctx context.Context, q database.Querier,
	organizationID string, currency string) (*models.Wallet, error) {
	if q == nil {
		q = r.db
	}
	return scanWallet(q.QueryRow(ctx,
		`SELECT `+walletColumns+` FROM finance_wallets WHERE organization_id = $1 AND currency = $2`,
		organizationID, currency))
}

// GetWalletByID loads a wallet inside a tenant scope. An empty organizationID
// is a platform-scoped read.
func (r *AccountRepository) GetWalletByID(ctx context.Context, q database.Querier,
	organizationID string, id string) (*models.Wallet, error) {
	if q == nil {
		q = r.db
	}
	return scanWallet(q.QueryRow(ctx,
		`SELECT `+walletColumns+` FROM finance_wallets
		 WHERE id = $1 AND ($2 = '' OR organization_id = $2)`, id, organizationID))
}

// LockWallet loads a wallet FOR UPDATE so a balance movement serializes
// against concurrent writers.
func (r *AccountRepository) LockWallet(ctx context.Context, q database.Querier,
	id string) (*models.Wallet, error) {
	return scanWallet(q.QueryRow(ctx,
		`SELECT `+walletColumns+` FROM finance_wallets WHERE id = $1 FOR UPDATE`, id))
}

// MoveWalletBalances applies signed deltas to the wallet projections under an
// expected version.
//
// Negative results are refused by the CHECK constraints on held and pending
// settlement, and explicitly here for available funds, so an operation can
// never overdraw a wallet by racing another.
func (r *AccountRepository) MoveWalletBalances(ctx context.Context, q database.Querier,
	id string, expectedVersion int64, availableDelta int64, heldDelta int64,
	pendingDelta int64) error {
	tag, err := q.Exec(ctx, `
		UPDATE finance_wallets SET
			available_minor = available_minor + $3,
			held_minor = held_minor + $4,
			pending_settlement_minor = pending_settlement_minor + $5,
			version = version + 1,
			updated_at = now()
		WHERE id = $1 AND version = $2
		  AND available_minor + $3 >= 0
		  AND held_minor + $4 >= 0
		  AND pending_settlement_minor + $5 >= 0`,
		id, expectedVersion, availableDelta, heldDelta, pendingDelta)
	if err != nil {
		return fmt.Errorf("finance: move wallet balances: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrVersionMismatch
	}
	return nil
}

// SetWalletStatus restricts, suspends, reactivates or closes a wallet under an
// expected version.
func (r *AccountRepository) SetWalletStatus(ctx context.Context, q database.Querier,
	id string, expectedVersion int64, status string, restrictions any) error {
	encoded, err := rawJSON(restrictions)
	if err != nil {
		return err
	}
	tag, err := q.Exec(ctx, `
		UPDATE finance_wallets SET
			status = $3, restrictions = $4, version = version + 1, updated_at = now()
		WHERE id = $1 AND version = $2`, id, expectedVersion, status, encoded)
	if err != nil {
		return fmt.Errorf("finance: set wallet status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrVersionMismatch
	}
	return nil
}

const earningAccountColumns = `id, provider_reference, provider_service,
	COALESCE(organization_id, ''), currency, status, payable_minor, held_minor, settled_minor,
	restrictions, payout_destination_ref, ledger_account_id, version, created_at, updated_at`

func scanEarningAccount(row pgx.Row) (*models.EarningAccount, error) {
	var account models.EarningAccount
	err := row.Scan(&account.ID, &account.ProviderReference, &account.ProviderService,
		&account.OrganizationID, &account.Currency, &account.Status, &account.PayableMinor,
		&account.HeldMinor, &account.SettledMinor, &account.Restrictions,
		&account.PayoutDestinationRef, &account.LedgerAccountID, &account.Version,
		&account.CreatedAt, &account.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("finance: scan earning account: %w", err)
	}
	return &account, nil
}

// EnsureEarningAccount returns a provider's earning account, creating it on
// first use.
//
// providerReference is opaque: it is whatever the owning provider service
// calls this provider. No provider identity is duplicated into this service.
func (r *AccountRepository) EnsureEarningAccount(ctx context.Context, q database.Querier,
	providerService string, providerReference string, organizationID string,
	currency string) (*models.EarningAccount, bool, error) {
	if q == nil {
		q = r.db
	}
	if _, err := q.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, "finance-earning:"+providerService+":"+providerReference+":"+currency); err != nil {
		return nil, false, fmt.Errorf("finance: lock earning account creation: %w", err)
	}
	existing, err := r.GetEarningAccount(ctx, q, providerService, providerReference, currency)
	if err == nil {
		return existing, false, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return nil, false, err
	}

	accountID := ids.New(ids.PrefixEarningAccount)
	ledgerAccount, err := r.ledger.EnsureAccount(ctx, q, organizationID,
		models.OwnerProviderEarning, accountID, models.AccountEarningPayable,
		currency, models.SideCredit)
	if err != nil {
		return nil, false, err
	}

	_, err = q.Exec(ctx, `
		INSERT INTO finance_earning_accounts
			(id, provider_reference, provider_service, organization_id, currency, status,
			 payable_minor, held_minor, settled_minor, restrictions, payout_destination_ref,
			 ledger_account_id, version, created_at, updated_at)
		VALUES ($1, $2, $3, NULLIF($4, ''), $5, 'ACTIVE', 0, 0, 0, '[]'::jsonb, '', $6, 1, now(), now())
		ON CONFLICT (provider_service, provider_reference, currency) DO NOTHING`,
		accountID, providerReference, providerService, organizationID, currency, ledgerAccount.ID)
	if err != nil {
		return nil, false, fmt.Errorf("finance: create earning account: %w", err)
	}

	account, err := r.GetEarningAccount(ctx, q, providerService, providerReference, currency)
	if err != nil {
		return nil, false, err
	}
	return account, account.ID == accountID, nil
}

// GetEarningAccount loads an earning account by its provider identity.
func (r *AccountRepository) GetEarningAccount(ctx context.Context, q database.Querier,
	providerService string, providerReference string, currency string) (*models.EarningAccount, error) {
	if q == nil {
		q = r.db
	}
	return scanEarningAccount(q.QueryRow(ctx,
		`SELECT `+earningAccountColumns+` FROM finance_earning_accounts
		 WHERE provider_service = $1 AND provider_reference = $2 AND currency = $3`,
		providerService, providerReference, currency))
}

// GetEarningAccountByID loads an earning account by ID.
func (r *AccountRepository) GetEarningAccountByID(ctx context.Context, q database.Querier,
	id string) (*models.EarningAccount, error) {
	if q == nil {
		q = r.db
	}
	return scanEarningAccount(q.QueryRow(ctx,
		`SELECT `+earningAccountColumns+` FROM finance_earning_accounts WHERE id = $1`, id))
}

// LockEarningAccount loads an earning account FOR UPDATE.
func (r *AccountRepository) LockEarningAccount(ctx context.Context, q database.Querier,
	id string) (*models.EarningAccount, error) {
	return scanEarningAccount(q.QueryRow(ctx,
		`SELECT `+earningAccountColumns+` FROM finance_earning_accounts WHERE id = $1 FOR UPDATE`, id))
}

// MoveEarningBalances applies signed deltas under an expected version.
func (r *AccountRepository) MoveEarningBalances(ctx context.Context, q database.Querier,
	id string, expectedVersion int64, payableDelta int64, heldDelta int64,
	settledDelta int64) error {
	tag, err := q.Exec(ctx, `
		UPDATE finance_earning_accounts SET
			payable_minor = payable_minor + $3,
			held_minor = held_minor + $4,
			settled_minor = settled_minor + $5,
			version = version + 1,
			updated_at = now()
		WHERE id = $1 AND version = $2
		  AND payable_minor + $3 >= 0
		  AND held_minor + $4 >= 0
		  AND settled_minor + $5 >= 0`,
		id, expectedVersion, payableDelta, heldDelta, settledDelta)
	if err != nil {
		return fmt.Errorf("finance: move earning balances: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrVersionMismatch
	}
	return nil
}

// SetEarningAccountStatus restricts, suspends or closes an earning account.
func (r *AccountRepository) SetEarningAccountStatus(ctx context.Context, q database.Querier,
	id string, expectedVersion int64, status string, restrictions any) error {
	encoded, err := rawJSON(restrictions)
	if err != nil {
		return err
	}
	tag, err := q.Exec(ctx, `
		UPDATE finance_earning_accounts SET
			status = $3, restrictions = $4, version = version + 1, updated_at = now()
		WHERE id = $1 AND version = $2`, id, expectedVersion, status, encoded)
	if err != nil {
		return fmt.Errorf("finance: set earning account status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrVersionMismatch
	}
	return nil
}

// SetPayoutDestination stores the opaque payout destination reference.
//
// It is a reference, not the destination: bank details live in the payout
// provider and in the existing provider_bank_accounts table, never here.
func (r *AccountRepository) SetPayoutDestination(ctx context.Context, q database.Querier,
	id string, expectedVersion int64, destinationRef string) error {
	if q == nil {
		q = r.db
	}
	tag, err := q.Exec(ctx, `
		UPDATE finance_earning_accounts SET
			payout_destination_ref = $3, version = version + 1, updated_at = now()
		WHERE id = $1 AND version = $2`, id, expectedVersion, destinationRef)
	if err != nil {
		return fmt.Errorf("finance: set payout destination: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrVersionMismatch
	}
	return nil
}

// ListWalletsForReconciliation streams wallets for a reconciliation pass.
func (r *AccountRepository) ListWalletsForReconciliation(ctx context.Context,
	q database.Querier, limit int) ([]models.Wallet, error) {
	if q == nil {
		q = r.db
	}
	if limit <= 0 {
		limit = 200
	}
	rows, err := q.Query(ctx,
		`SELECT `+walletColumns+` FROM finance_wallets ORDER BY updated_at LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("finance: list wallets: %w", err)
	}
	defer rows.Close()

	var wallets []models.Wallet
	for rows.Next() {
		var wallet models.Wallet
		if err := rows.Scan(&wallet.ID, &wallet.OrganizationID, &wallet.Currency, &wallet.Status,
			&wallet.AvailableMinor, &wallet.HeldMinor, &wallet.PendingSettlementMinor,
			&wallet.Restrictions, &wallet.LedgerAccountID, &wallet.Version,
			&wallet.CreatedAt, &wallet.UpdatedAt); err != nil {
			return nil, fmt.Errorf("finance: scan wallet: %w", err)
		}
		wallets = append(wallets, wallet)
	}
	return wallets, rows.Err()
}
