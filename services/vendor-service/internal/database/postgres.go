package database

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPool opens vendor-service's own Postgres pool. Vendor Service owns this
// database exclusively; no other service may connect to it and this package
// never queries another service's database (see
// docs/architecture/01_SERVICE_BOUNDARIES.md).
func NewPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}

	cfg.MaxConns = 10
	cfg.MinConns = 1
	cfg.MaxConnLifetime = time.Hour
	cfg.MaxConnIdleTime = 10 * time.Minute

	return pgxpool.NewWithConfig(ctx, cfg)
}
