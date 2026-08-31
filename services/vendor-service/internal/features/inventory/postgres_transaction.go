package inventory

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresTransactionAccessor exposes the concrete PostgreSQL transaction only
// to infrastructure adapters.
//
// Domain callers continue to depend on InventoryTransaction.
type PostgresTransactionAccessor interface {
	InventoryTransaction

	PGX() pgx.Tx
}

type postgresInventoryTransaction struct {
	ctx context.Context
	tx  pgx.Tx
}

func (t postgresInventoryTransaction) Context() context.Context {
	return t.ctx
}

func (t postgresInventoryTransaction) PGX() pgx.Tx {
	return t.tx
}

// PostgresTransactionBoundary binds all Inventory mutation effects to one
// PostgreSQL commit.
type PostgresTransactionBoundary struct {
	pool *pgxpool.Pool
}

func NewPostgresTransactionBoundary(
	pool *pgxpool.Pool,
) *PostgresTransactionBoundary {
	return &PostgresTransactionBoundary{
		pool: pool,
	}
}

func (b *PostgresTransactionBoundary) WithinInventoryTransaction(
	ctx context.Context,
	work func(InventoryTransaction) error,
) error {
	if b == nil || b.pool == nil {
		return ErrDependencyMissing
	}

	tx, err := b.pool.BeginTx(
		ctx,
		pgx.TxOptions{
			IsoLevel: pgx.ReadCommitted,
		},
	)
	if err != nil {
		return err
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	wrapped := postgresInventoryTransaction{
		ctx: ctx,
		tx:  tx,
	}

	if err := work(wrapped); err != nil {
		return err
	}

	return tx.Commit(ctx)
}
