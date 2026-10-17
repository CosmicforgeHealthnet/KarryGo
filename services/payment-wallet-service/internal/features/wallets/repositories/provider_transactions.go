package walletrepositories

import (
	"context"
	walletmodels "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/models"
)

func (r *PostgresWalletRepository) ListProviderTransactions(ctx context.Context, ownerID, currency string, limit, offset int) ([]walletmodels.WalletTransaction, error) {
	rows, err := r.db.Query(ctx, `
		SELECT t.reference,t.transaction_type,e.side,e.amount_kobo,e.currency,e.memo,e.created_at
		FROM ledger_entries e
		JOIN ledger_transactions t ON t.id=e.transaction_id
		JOIN wallet_accounts a ON a.id=e.account_id
		WHERE a.owner_type=$1 AND a.owner_id=$2 AND a.account_type=$3 AND e.currency=$4
		ORDER BY e.created_at DESC,e.id DESC LIMIT $5 OFFSET $6`,
		walletmodels.OwnerTypeProvider, ownerID, walletmodels.AccountProviderPayable,
		walletmodels.DefaultCurrency(currency), limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []walletmodels.WalletTransaction{}
	for rows.Next() {
		var item walletmodels.WalletTransaction
		if err := rows.Scan(&item.Reference, &item.TransactionType, &item.Side, &item.AmountKobo, &item.Currency, &item.Memo, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
