package database

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPool opens pricing-service's own Postgres pool. This service owns its
// database exclusively: it never reads another service's database, and every
// input it prices with arrives on a request or through a governed import.
//
// PostgreSQL is the system of record for policy versions, quotes and
// reconciliations. A cache may accelerate reads of immutable rows, but it can
// never be the source of commercial truth.
func NewPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 20 // quote issuance is on the customer's critical path
	cfg.MinConns = 2
	cfg.MaxConnLifetime = time.Hour
	cfg.MaxConnIdleTime = 10 * time.Minute
	return pgxpool.NewWithConfig(ctx, cfg)
}

// Querier is satisfied by both *pgxpool.Pool and pgx.Tx, so repository methods
// compose into the caller's transaction. Audit and outbox writes must always
// join the transaction of the change they describe.
type Querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// WithinTx runs fn in a transaction, committing on success and rolling back on
// error or panic.
func WithinTx(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) error) (err error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(ctx)
			panic(p)
		}
	}()
	if err = fn(tx); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	return tx.Commit(ctx)
}
