package database

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// FleetMarketplaceFeedSetting is the per-connection setting that switches the
// taxi_bookings -> Fleet marketplace feed triggers on (migration 029). Set
// only on connections of a deployment that enabled
// TAXI_FLEET_MARKETPLACE_FEED_ENABLED.
const FleetMarketplaceFeedSetting = "taxi.fleet_marketplace_feed"

// PoolOption adjusts the pool configuration.
type PoolOption func(*pgxpool.Config)

// WithSessionSetting sets a run-time parameter on every pooled connection.
func WithSessionSetting(name string, value string) PoolOption {
	return func(cfg *pgxpool.Config) {
		if cfg.ConnConfig.RuntimeParams == nil {
			cfg.ConnConfig.RuntimeParams = map[string]string{}
		}
		cfg.ConnConfig.RuntimeParams[name] = value
	}
}

func NewPool(ctx context.Context, databaseURL string, options ...PoolOption) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}

	cfg.MaxConns = 10
	cfg.MinConns = 1
	cfg.MaxConnLifetime = time.Hour
	cfg.MaxConnIdleTime = 10 * time.Minute
	for _, option := range options {
		option(cfg)
	}

	return pgxpool.NewWithConfig(ctx, cfg)
}
