package persistence

import (
	"context"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/config"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/features/operations"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/security"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/testutil"
	"cosmicforge/logistics/services/admin-backoffice-service/migrations"
	"errors"
	"github.com/google/uuid"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPostgresReplayAndMigrationConcurrency(t *testing.T) {
	s := &Store{Pool: testutil.Database(t)}
	ctx := context.Background()
	var wg sync.WaitGroup
	var won atomic.Int32
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := s.Claim(ctx, "same-nonce", "same-jti", time.Now().Add(time.Minute))
			if err == nil {
				won.Add(1)
			} else if !errors.Is(err, ErrReplay) {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if won.Load() != 1 {
		t.Fatalf("atomic replay winners=%d", won.Load())
	}
	// A new nonce conflicting on JTI is rolled back and remains usable.
	if !errors.Is(s.Claim(ctx, "new-nonce", "same-jti", time.Now()), ErrReplay) {
		t.Fatal("JTI reuse accepted")
	}
	if err := s.Claim(ctx, "new-nonce", "new-jti", time.Now().Add(time.Minute)); err != nil {
		t.Fatal("partial claim leaked", err)
	}
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := migrations.Apply(ctx, s.Pool); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if err := s.Ready(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresIdempotencyScopeAndImmutableAudit(t *testing.T) {
	s := &Store{Pool: testutil.Database(t)}
	ctx := context.Background()
	scope := security.Scope{Market: "NG", Organization: "org-1"}
	c := operations.Command{CommandID: uuid.NewString(), Action: "dispatch.provider.suspension.request", Target: operations.Target{Type: "dispatch_provider", ID: "provider-1", ExpectedVersion: 7}, Scope: scope, IdempotencyKey: "idem-1"}
	digest := security.Hash("one exact request")
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			o, err := s.RecordRejected(ctx, c, "staff-1", digest)
			if err != nil {
				t.Error(err)
			} else if o.Status != "REJECTED" || o.ID != c.CommandID {
				t.Errorf("fabricated result: %+v", o)
			}
		}()
	}
	wg.Wait()
	if _, err := s.RecordRejected(ctx, c, "staff-1", security.Hash("tampered")); !errors.Is(err, operations.ErrConflict) {
		t.Fatal("changed payload accepted", err)
	}
	if _, err := s.RecordRejected(ctx, c, "staff-2", digest); !errors.Is(err, operations.ErrConflict) {
		t.Fatal("changed staff accepted", err)
	}
	if _, err := s.Operation(ctx, c.CommandID, "staff-1", security.Scope{Market: "NG", Organization: "org-2"}); !errors.Is(err, operations.ErrNotFound) {
		t.Fatal("cross-tenant operation exposed", err)
	}
	if _, err := s.Operation(ctx, c.CommandID, "staff-2", scope); !errors.Is(err, operations.ErrNotFound) {
		t.Fatal("cross-staff operation exposed", err)
	}
	changedID := c
	changedID.CommandID = uuid.NewString()
	if _, err := s.RecordRejected(ctx, changedID, "staff-1", digest); !errors.Is(err, operations.ErrConflict) {
		t.Fatal("idempotency key rebound", err)
	}
	changedKey := c
	changedKey.IdempotencyKey = "idem-2"
	if _, err := s.RecordRejected(ctx, changedKey, "staff-1", digest); !errors.Is(err, operations.ErrConflict) {
		t.Fatal("command identity rebound", err)
	}
	a := Audit{Correlation: uuid.NewString(), Principal: config.HQPrincipal, Staff: "staff-1", Outcome: "AUTHORIZED", RequestType: "bridge.health"}
	if err := s.Audit(ctx, a); err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{`UPDATE admin_audit_entries SET outcome='tampered'`, `DELETE FROM admin_audit_entries`} {
		if _, err := s.Pool.Exec(ctx, sql); err == nil {
			t.Fatal("evidence mutable", sql)
		}
	}
	var n int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM admin_delegated_operations`).Scan(&n); err != nil || n != 1 {
		t.Fatal("duplicate operations", n, err)
	}
}

func TestPostgresRevocationPrincipalAndRate(t *testing.T) {
	s := &Store{Pool: testutil.Database(t)}
	ctx := context.Background()
	now := time.Now()
	fp := security.Hash("certificate")
	if err := s.Principal(ctx, fp, "spiffe://test/karryam", "test", now); !errors.Is(err, security.ErrDenied) {
		t.Fatal("unregistered principal accepted")
	}
	if _, err := s.Pool.Exec(ctx, `INSERT INTO admin_remote_principals(principal,certificate_sha256,workload_uri,product,audience,expires_at) VALUES($1,$2,'spiffe://test/karryam','karryam','test',$3)`, config.HQPrincipal, fp, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := s.Principal(ctx, fp, "spiffe://test/karryam", "test", now); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE admin_remote_principals SET revoked_at=now()`); err != nil {
		t.Fatal(err)
	}
	if err := s.Principal(ctx, fp, "spiffe://test/karryam", "test", now); !errors.Is(err, security.ErrDenied) {
		t.Fatal("revoked certificate accepted")
	}
	for _, kind := range []string{"staff", "session"} {
		if _, err := s.Pool.Exec(ctx, `INSERT INTO admin_revocations(kind,reference) VALUES($1,'ref')`, kind); err != nil {
			t.Fatal(err)
		}
	}
	if ok, err := s.Revoked(ctx, security.Claims{Subject: "ref"}); !ok || err != nil {
		t.Fatal("staff revocation missed", err)
	}
	if ok, err := s.Revoked(ctx, security.Claims{SessionID: "ref"}); !ok || err != nil {
		t.Fatal("session revocation missed", err)
	}
	var wg sync.WaitGroup
	var admitted atomic.Int32
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := s.Rate(ctx, "staff:permission:NG:org-1", now, 5)
			if err != nil {
				t.Error(err)
			}
			if ok {
				admitted.Add(1)
			}
		}()
	}
	wg.Wait()
	if admitted.Load() != 5 {
		t.Fatalf("rate admitted=%d", admitted.Load())
	}
}
