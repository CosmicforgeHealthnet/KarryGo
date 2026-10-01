package shareusecases

import (
	"context"
	m "cosmicforge/logistics/services/customer-service/internal/features/journey_sharing/models"
	r "cosmicforge/logistics/services/customer-service/internal/features/journey_sharing/repositories"
	"cosmicforge/logistics/shared/go/journeyshare"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

type authority struct{ terminal bool }

func (a *authority) Read(_ context.Context, domain, ref, owner string) (journeyshare.Projection, error) {
	return journeyshare.Projection{CustomerID: owner, Authority: domain, SourceRef: ref, ProductCode: map[string]string{"TAXI": "RIDE", "DISPATCH": "SEND", "HAULING": "TRUCK", "VENDOR_COMMERCE": "FOOD"}[domain], Eligible: !a.terminal, Terminal: a.terminal, SourceVersion: 1, PublicStatus: "IN_PROGRESS"}, nil
}
func TestPersistentLifecycleConcurrencyAndPrivacy(t *testing.T) {
	dsn := os.Getenv("CUSTOMER_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires isolated CUSTOMER_TEST_DATABASE_URL")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, file, _, _ := runtime.Caller(0)
	migration, err := os.ReadFile(filepath.Join(filepath.Dir(file), "../../../../migrations/010_journey_share_sessions.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, string(migration)); err != nil {
		t.Fatal(err)
	}
	outboxMigration, err := os.ReadFile(filepath.Join(filepath.Dir(file), "../../../../migrations/011_journey_share_notifications.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, string(outboxMigration)); err != nil {
		t.Fatal(err)
	}
	owner := uuid.NewString()
	if _, err = db.Exec(ctx, `INSERT INTO customers(id,email)VALUES($1,$2)`, owner, owner+"@test.invalid"); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	a := &authority{}
	repo := &r.Postgres{DB: db}
	svc := &Service{Repository: repo, Reader: a, Enabled: true, Now: func() time.Time { return now }}
	created, err := svc.Create(ctx, owner, "create", m.Create{Authority: "TAXI", SourceRef: uuid.NewString(), ProductCode: "RIDE"})
	if err != nil {
		t.Fatal(err)
	}
	var createdIntents int
	if err = db.QueryRow(ctx, `SELECT count(*) FROM journey_share_notification_outbox WHERE session_id=$1 AND event_type='share.created'`, created.Session.ID).Scan(&createdIntents); err != nil || createdIntents != 1 {
		t.Fatal("create must atomically persist one notification intent", createdIntents, err)
	}
	token := created.Token
	if len(token) != 43 {
		t.Fatal("missing capability")
	}
	var persisted, commandData string
	if err = db.QueryRow(ctx, `SELECT token_hash FROM journey_share_sessions WHERE id=$1`, created.Session.ID).Scan(&persisted); err != nil {
		t.Fatal(err)
	}
	if persisted != m.Digest(token) || persisted == token {
		t.Fatal("plaintext persisted")
	}
	_ = db.QueryRow(ctx, `SELECT result::text FROM journey_share_commands WHERE customer_id=$1`, owner).Scan(&commandData)
	if strings.Contains(commandData, token) {
		t.Fatal("replay stored token")
	}
	replay, err := svc.Create(ctx, owner, "create", m.Create{Authority: "TAXI", SourceRef: created.Session.SourceRef, ProductCode: "RIDE"})
	if err != nil || !replay.Replayed || replay.Token != "" {
		t.Fatal("retry semantics", replay, err)
	}
	if _, err = svc.Create(ctx, owner, "create", m.Create{Authority: "TAXI", SourceRef: "different", ProductCode: "RIDE"}); err == nil {
		t.Fatal("key conflict accepted")
	}
	other := uuid.NewString()
	if _, err = repo.Get(ctx, created.Session.ID, other); err == nil {
		t.Fatal("IDOR read")
	}
	if _, err = svc.Mutate(ctx, other, created.Session.ID, "rotate", "rotate", 1); err == nil {
		t.Fatal("IDOR rotate")
	}
	if _, err = svc.Mutate(ctx, other, created.Session.ID, "revoke", "revoke", 1); err == nil {
		t.Fatal("IDOR revoke")
	}
	rotated, err := svc.Mutate(ctx, owner, created.Session.ID, "rotate", "rotate", 1)
	if err != nil || rotated.Token == "" {
		t.Fatal(rotated, err)
	}
	if _, err = svc.Public(ctx, token); err == nil {
		t.Fatal("old token still works")
	}
	if _, err = svc.Public(ctx, rotated.Token); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Mutate(ctx, owner, created.Session.ID, "stale", "revoke", 1); err == nil {
		t.Fatal("stale version accepted")
	}
	var wg sync.WaitGroup
	wins := make(chan bool, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := svc.Mutate(ctx, owner, created.Session.ID, uuid.NewString(), "revoke", 2)
			wins <- e == nil
		}()
	}
	wg.Wait()
	close(wins)
	count := 0
	for win := range wins {
		if win {
			count++
		}
	}
	if count != 1 {
		t.Fatal("concurrency", count)
	}
	if _, err = svc.Public(ctx, rotated.Token); err == nil {
		t.Fatal("revoked token works")
	}
	var revokedIntents int
	if err = db.QueryRow(ctx, `SELECT count(*) FROM journey_share_notification_outbox WHERE session_id=$1 AND event_type='share.revoked'`, created.Session.ID).Scan(&revokedIntents); err != nil || revokedIntents != 1 {
		t.Fatal("concurrent revoke must emit one notification intent", revokedIntents, err)
	}
	for _, domain := range []string{"TAXI", "DISPATCH", "HAULING", "VENDOR_COMMERCE"} {
		in := m.Create{Authority: domain, ProductCode: map[string]string{"TAXI": "RIDE", "DISPATCH": "SEND", "HAULING": "TRUCK", "VENDOR_COMMERCE": "FOOD"}[domain], SourceRef: uuid.NewString()}
		res, e := svc.Create(ctx, owner, uuid.NewString(), in)
		if e != nil {
			t.Fatal(domain, e)
		}
		snapshot, e := svc.Public(ctx, res.Token)
		if e != nil {
			t.Fatal(e)
		}
		raw, _ := json.Marshal(snapshot)
		if strings.Contains(string(raw), owner) {
			t.Fatal("public ownership leak")
		}
	}
	a.terminal = true
	if err = svc.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	sessions, err := repo.List(ctx, owner, "", "", "COMPLETED", 100, 0)
	if err != nil || len(sessions) != 4 {
		t.Fatal("completion sweep", len(sessions), err)
	}
	now = now.Add(time.Hour)
	if err = svc.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	expired, err := repo.List(ctx, owner, "", "", "EXPIRED", 100, 0)
	if err != nil || len(expired) != 4 {
		t.Fatal("expiry sweep", len(expired), err)
	}
	var expiredIntents int
	if err = db.QueryRow(ctx, `SELECT count(*) FROM journey_share_notification_outbox WHERE customer_id=$1 AND event_type='share.expired'`, owner).Scan(&expiredIntents); err != nil || expiredIntents != 4 {
		t.Fatal("expiry must emit one notification intent per share", expiredIntents, err)
	}
	svc.Enabled = false
	if _, err = svc.Public(ctx, token); err == nil {
		t.Fatal("disabled gate ignored")
	}
}
