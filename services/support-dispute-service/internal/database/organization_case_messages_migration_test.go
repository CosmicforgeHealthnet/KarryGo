package database

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	supportmodels "cosmicforge/logistics/services/support-dispute-service/internal/features/support/models"
	supportrepositories "cosmicforge/logistics/services/support-dispute-service/internal/features/support/repositories"
)

// TestOrganizationCaseMessagesMigrationAndRepository certifies migration 011
// against PostgreSQL: it is a no-op on re-run, its checks refuse invalid
// visibility and file lists, concurrent submissions of one client message id
// store exactly one row, INTERNAL messages are excluded from complainant
// reads, and the down script restores the original schema without deleting
// a message before 011 reapplies cleanly.
func TestOrganizationCaseMessagesMigrationAndRepository(t *testing.T) {
	dsn := os.Getenv("SUPPORT_DISPUTE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set SUPPORT_DISPUTE_TEST_DATABASE_URL")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	t.Cleanup(func() {
		if err := ApplyMigrations(ctx, pool); err != nil {
			t.Errorf("reapply in cleanup: %v", err)
		}
	})
	for i := 0; i < 2; i++ { // the second pass proves the re-run is a no-op
		if err = ApplyMigrations(ctx, pool); err != nil {
			t.Fatalf("apply pass %d: %v", i+1, err)
		}
	}

	repo := supportrepositories.NewPostgresSupportRepository(pool)
	org := "org_msg_" + uuid.NewString()[:8]
	key := uuid.NewString()
	c, err := repo.CreateComplaint(ctx, supportrepositories.CreateComplaintInput{ComplainantType: supportmodels.ComplainantCustomer, ComplainantID: org,
		ServiceType: supportmodels.ServiceTypePlatform, Priority: "normal", Subject: "Late pickups", Description: "Three late pickups",
		OrganizationReference: &org, SourceIdempotencyKey: &key})
	if err != nil {
		t.Fatal(err)
	}

	for name, stmt := range map[string]string{
		"visibility": `INSERT INTO support_chat_messages (complaint_id, sender_type, sender_id, content, visibility) VALUES ($1, 'organization_user', 'usr', 'x', 'PUBLIC')`,
		"file list":  `INSERT INTO support_chat_messages (complaint_id, sender_type, sender_id, content, file_ids) VALUES ($1, 'organization_user', 'usr', 'x', '{"a":1}')`,
		"sender":     `INSERT INTO support_chat_messages (complaint_id, sender_type, sender_id, content) VALUES ($1, 'stranger', 'usr', 'x')`,
	} {
		if _, err := pool.Exec(ctx, stmt, c.ID); err == nil {
			t.Fatalf("%s check accepted an invalid row", name)
		}
	}

	clientID := uuid.NewString()
	hash := "hash-1"
	msg := supportmodels.ChatMessage{ComplaintID: c.ID, SenderType: supportmodels.SenderTypeOrganizationUser, SenderID: "usr_1",
		Content: "Please call the branch.", Visibility: supportmodels.VisibilityShared, FileIDs: []string{"fil_1"}, ClientMessageID: &clientID, RequestHash: &hash}
	var wg sync.WaitGroup
	var mu sync.Mutex
	created, ids := 0, map[string]bool{}
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			stored, isNew, err := repo.CreateOrganizationMessage(ctx, msg)
			if err != nil {
				t.Errorf("concurrent submit: %v", err)
				return
			}
			mu.Lock()
			defer mu.Unlock()
			ids[stored.ID] = true
			if isNew {
				created++
			}
		}()
	}
	wg.Wait()
	if created != 1 || len(ids) != 1 {
		t.Fatalf("six concurrent submissions created %d rows and returned %d ids, want 1 and 1", created, len(ids))
	}
	back, err := repo.GetChatMessageByClientID(ctx, c.ID, clientID)
	if err != nil || len(back.FileIDs) != 1 || back.FileIDs[0] != "fil_1" || back.RequestHash == nil || *back.RequestHash != hash {
		t.Fatalf("read-back = %+v, %v", back, err)
	}
	internalID, internalHash := uuid.NewString(), "hash-2"
	if _, _, err := repo.CreateOrganizationMessage(ctx, supportmodels.ChatMessage{ComplaintID: c.ID, SenderType: supportmodels.SenderTypeOrganizationUser,
		SenderID: "usr_1", Content: "Internal note", Visibility: supportmodels.VisibilityInternal, ClientMessageID: &internalID, RequestHash: &internalHash}); err != nil {
		t.Fatal(err)
	}
	if all, err := repo.ListChatMessages(ctx, c.ID, true, 50, 0); err != nil || len(all) != 2 {
		t.Fatalf("support list = %d, %v; want 2", len(all), err)
	}
	if visible, err := repo.ListChatMessages(ctx, c.ID, false, 50, 0); err != nil || len(visible) != 1 || visible[0].Visibility != supportmodels.VisibilityShared {
		t.Fatalf("complainant list = %+v, %v; want only the SHARED message", visible, err)
	}

	down, err := os.ReadFile(filepath.Join("migrations", "down", "011_organization_case_messages.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(down)); err != nil {
		t.Fatalf("down: %v", err)
	}
	var remaining int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM support_chat_messages WHERE complaint_id = $1`, c.ID).Scan(&remaining); err != nil || remaining != 2 {
		t.Fatalf("down left %d messages (%v), want both kept", remaining, err)
	}
	var visibilityLeft bool
	if err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name='support_chat_messages' AND column_name='visibility')`).Scan(&visibilityLeft); err != nil || visibilityLeft {
		t.Fatalf("down left the visibility column (exists=%v, err=%v)", visibilityLeft, err)
	}
	if err = ApplyMigrations(ctx, pool); err != nil {
		t.Fatalf("reapply after down: %v", err)
	}
}
