package adminpolicy

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

// PinRevision persists the accepted high-water mark inside this service's own
// database. Only fixed wiring table names are accepted. Older approvals and
// same-version document rewrites cannot replace an already accepted policy.
func PinRevision(ctx context.Context, pool *pgxpool.Pool, table string, engine *Engine) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if engine == nil || pool == nil {
		return ErrBlocked
	}
	switch table {
	case "admin_policy_revisions", "taxi_admin_policy_revisions", "analytics_admin_policy_revisions":
	default:
		return ErrInvalid
	}
	var version int64
	err := pool.QueryRow(ctx, `INSERT INTO `+table+`(policy_id,version,checksum) VALUES($1,$2,$3)
 ON CONFLICT(policy_id) DO UPDATE SET version=excluded.version,checksum=excluded.checksum,accepted_at=now()
 WHERE `+table+`.version<excluded.version OR (`+table+`.version=excluded.version AND `+table+`.checksum=excluded.checksum)
 RETURNING version`, engine.ID(), engine.Version(), engine.Checksum()).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrBlocked
	}
	return err
}
