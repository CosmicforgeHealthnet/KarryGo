package infra

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cosmicforge/logistics/services/partner-web-service/internal/features/partneraccount"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Uses only an explicitly provisioned Gateway test database. Never falls back
// to API_GATEWAY_DATABASE_URL (which may identify production).
func TestPostgresRefreshRotationAndMigrationReplay(t *testing.T) {
	dsn := os.Getenv("API_GATEWAY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("API_GATEWAY_TEST_DATABASE_URL unset: BLOCKED_BY_TEST_INFRASTRUCTURE")
	}
	ctx := context.Background()
	base, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal("test database configuration invalid")
	}
	defer base.Close()
	schema := "partner_auth_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = base.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal("test schema creation failed")
	}
	defer func() {
		if _, err := base.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error("test schema cleanup failed")
		}
	}()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal("invalid test configuration")
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal("test pool unavailable")
	}
	defer pool.Close()
	apply := func(name string) {
		t.Helper()
		data, err := os.ReadFile(filepath.Join("../../../../migrations", name))
		if err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, string(data)); err != nil {
			t.Fatalf("migration %s failed", name)
		}
	}
	apply("001_partner_schema.sql")
	apply("002_partner_auth.sql")
	apply("010_partner_refresh_cookie.sql")
	apply("010_partner_refresh_cookie_down.sql")
	apply("010_partner_refresh_cookie.sql")
	var id string
	if err = pool.QueryRow(ctx, `INSERT INTO partner_accounts(business_name,business_email) VALUES ('Test','test@example.invalid') RETURNING id`).Scan(&id); err != nil {
		t.Fatal("account fixture failed")
	}
	repo := NewSessionRepo(pool)
	now := time.Now()
	old := strings.Repeat("a", 64)
	p := partneraccount.CreateSessionParams{ID: uuid.NewString(), PartnerID: id, TokenHash: strings.Repeat("b", 64), RefreshHash: old, ExpiresAt: now.Add(time.Hour)}
	if err = repo.CreateSession(ctx, p); err != nil {
		t.Fatal("session insert failed")
	}
	var won atomic.Int32
	var failed atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			next := p
			next.TokenHash = strings.Repeat("c", 64)
			next.RefreshHash = strings.Repeat("d", 64)
			ok, err := repo.Rotate(ctx, old, next, now)
			if err != nil {
				failed.Add(1)
			}
			if ok {
				won.Add(1)
			}
		}()
	}
	wg.Wait()
	if won.Load() != 1 || failed.Load() != 0 {
		t.Fatalf("rotation winners=%d errors=%d", won.Load(), failed.Load())
	}
	if s, err := repo.FindByRefreshHash(ctx, old); err != nil || s != nil {
		t.Fatal("old refresh credential remains valid")
	}
	if found, err := repo.RevokeSession(ctx, p.ID); err != nil || !found {
		t.Fatal("revoke failed")
	}
	if ok, err := repo.Rotate(ctx, strings.Repeat("d", 64), p, now); err != nil || ok {
		t.Fatal("revoked row rotated")
	}
	for _, refreshFirst := range []bool{true, false} {
		name := "logout_wins"
		if refreshFirst {
			name = "refresh_wins"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			fixture := p
			fixture.ID = uuid.NewString()
			fixture.TokenHash = strings.ReplaceAll(uuid.NewString(), "-", "")
			fixture.RefreshHash = strings.Repeat(strings.ReplaceAll(uuid.NewString(), "-", ""), 2)
			if err := repo.CreateSession(ctx, fixture); err != nil {
				t.Fatal("race fixture failed")
			}
			next := fixture
			next.TokenHash = strings.ReplaceAll(uuid.NewString(), "-", "")
			next.RefreshHash = strings.Repeat(strings.ReplaceAll(uuid.NewString(), "-", ""), 2)
			first, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal("begin failed")
			}
			defer first.Rollback(context.Background())
			// Execute the first mutation with the REAL repository, holding its
			// row lock uncommitted while launching the competing pool operation.
			locked := &SessionRepo{pool: first}
			var firstOK bool
			if refreshFirst {
				firstOK, err = locked.Rotate(ctx, fixture.RefreshHash, next, now)
			} else {
				firstOK, err = locked.RevokeSession(ctx, fixture.ID)
			}
			if err != nil || !firstOK {
				t.Fatal("first mutation failed")
			}
			entered := make(chan struct{})
			type result struct {
				ok  bool
				err error
			}
			done := make(chan result, 1)
			go func() {
				close(entered)
				var ok bool
				var err error
				if refreshFirst {
					ok, err = repo.RevokeSession(ctx, fixture.ID)
				} else {
					ok, err = repo.Rotate(ctx, fixture.RefreshHash, next, now)
				}
				done <- result{ok, err}
			}()
			<-entered
			if err := first.Commit(ctx); err != nil {
				t.Fatal("commit failed")
			}
			second := <-done
			if second.err != nil || second.ok != refreshFirst {
				t.Fatal("incorrect race outcome")
			}
			if found, err := repo.RevokeSession(ctx, fixture.ID); err != nil || !found {
				t.Fatal("repeat logout failed")
			}
			var revoked bool
			if err := pool.QueryRow(ctx, `SELECT revoked_at IS NOT NULL FROM partner_sessions WHERE id=$1`, fixture.ID).Scan(&revoked); err != nil || !revoked {
				t.Fatal("active session survived race")
			}
			if ok, err := repo.Rotate(ctx, next.RefreshHash, fixture, now); err != nil || ok {
				t.Fatal("successor revived")
			}
		})
	}
	if found, err := repo.RevokeSession(ctx, uuid.NewString()); err != nil || found {
		t.Fatal("zero-row logout falsely confirmed")
	}
}
