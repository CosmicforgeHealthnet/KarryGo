package database

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// migrationLockKey serializes every applier of this service's migrations on
// one database (several replicas booting, or parallel test packages). Every
// file re-runs on each boot, and two concurrent runs of the same DDL can
// deadlock or race on catalog rows; holding one session lock makes each run
// see the previous one finished.
const migrationLockKey = "support-dispute-service:apply-migrations"

func ApplyMigrations(ctx context.Context, db *pgxpool.Pool) error {
	entries, err := os.ReadDir("migrations")
	if err != nil {
		return err
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)

	conn, err := db.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration connection: %w", err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock(hashtextextended($1, 0))`, migrationLockKey); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	defer func() {
		_, _ = conn.Exec(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock(hashtextextended($1, 0))`, migrationLockKey)
	}()
	// Session setting on this dedicated connection only; reset before the
	// connection returns to the pool.
	if _, err := conn.Exec(ctx, `SET lock_timeout = '`+lockTimeout+`'`); err != nil {
		return fmt.Errorf("set migration lock timeout: %w", err)
	}
	defer func() { _, _ = conn.Exec(context.WithoutCancel(ctx), `RESET lock_timeout`) }()

	for _, name := range names {
		migrationPath := filepath.Join("migrations", name)
		migration, err := os.ReadFile(migrationPath)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", migrationPath, err)
		}
		if err := applyFile(ctx, conn, string(migration)); err != nil {
			return fmt.Errorf("apply migration %s: %w", migrationPath, err)
		}
	}

	return nil
}

// applyFile runs one idempotent file as one implicit transaction. Re-runs of
// the older files still take locks on live case tables, while live case
// writes lock child tables before the case row (their triggers bump the case
// version). So a migration never waits long for a lock: it gives up after
// lockTimeout -- well before PostgreSQL's deadlock check would pick a live
// writer as the victim -- rolls back completely and, being idempotent, is
// simply retried.
func applyFile(ctx context.Context, conn *pgxpool.Conn, migration string) error {
	var err error
	for attempt := 0; attempt < maxApplyAttempts; attempt++ {
		if _, err = conn.Exec(ctx, migration); err == nil {
			return nil
		}
		var pg *pgconn.PgError
		if !errors.As(err, &pg) || (pg.Code != "55P03" && pg.Code != "40P01" && pg.Code != "40001") {
			return err
		}
		backoff := time.Duration(attempt+1) * 50 * time.Millisecond
		if backoff > time.Second {
			backoff = time.Second
		}
		select {
		case <-ctx.Done():
			return err
		case <-time.After(backoff):
		}
	}
	return err
}

const (
	lockTimeout      = "300ms"
	maxApplyAttempts = 40
)
