// Package database owns this service's dedicated PostgreSQL connection.
//
// fleet-partner-service never connects to, queries or migrates another
// service's database. External service IDs are stored as opaque values with
// no cross-database foreign keys.
package database

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PoolOptions bound the dedicated connection pool.
type PoolOptions struct {
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
}

// scanTimestampsInUTC makes every timestamptz this connection scans arrive in
// UTC.
func scanTimestampsInUTC(_ context.Context, conn *pgx.Conn) error {
	conn.TypeMap().RegisterType(&pgtype.Type{Name: "timestamptz", OID: pgtype.TimestamptzOID,
		Codec: &pgtype.TimestamptzCodec{ScanLocation: time.UTC}})
	return nil
}

// NewPool creates the dedicated pgx pool and verifies connectivity with a ping.
func NewPool(ctx context.Context, databaseURL string, opts PoolOptions) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}

	if opts.MaxConns > 0 {
		cfg.MaxConns = opts.MaxConns
	}
	if opts.MinConns > 0 {
		cfg.MinConns = opts.MinConns
	}
	if opts.MaxConnLifetime > 0 {
		cfg.MaxConnLifetime = opts.MaxConnLifetime
	}
	cfg.MaxConnIdleTime = 10 * time.Minute
	// Spec 6: the API speaks UTC ISO 8601. pgx returns timestamptz values in
	// the process's local zone by default, which would leak a "+01:00" style
	// offset into every response on a host not running in UTC. Scanning in
	// UTC keeps the instant and fixes the zone, independent of the host.
	cfg.AfterConnect = scanTimestampsInUTC

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}
