package supportrepositories

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	supportmodels "cosmicforge/logistics/services/support-dispute-service/internal/features/support/models"
)

// These tests run the real SQL against a Postgres instance. They are gated on
// SUPPORT_DISPUTE_TEST_DATABASE_URL so plain `go test ./...` stays green without
// a database. The in-memory usecase tests cannot catch SQL bugs (enum/text
// parameter binding, column lists, casts), so this is the regression guard for
// them — notably the two enum-cast fixes in ListComplaintsByComplainant
// (unread subquery) and UpdateComplaintStatus.
//
// To run:
//   bash scripts/bootstrap_services/support-dispute-local-bootstrap.sh
//   SUPPORT_DISPUTE_TEST_DATABASE_URL='postgres://cosmicforge_logistics:cosmicforge_logistics@localhost:5439/support_dispute_service?sslmode=disable' \
//     go test ./internal/features/support/repositories/ -run Integration -v
func integrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("SUPPORT_DISPUTE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set SUPPORT_DISPUTE_TEST_DATABASE_URL to run repository integration tests")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		t.Fatalf("ping: %v", err)
	}
	return pool
}

func TestRepository_Integration(t *testing.T) {
	pool := integrationPool(t)
	repo := NewPostgresSupportRepository(pool)
	ctx := context.Background()

	// Unique complainant so repeated runs don't collide, with cleanup.
	owner := fmt.Sprintf("itest-%d", time.Now().UnixNano())
	ct := supportmodels.ComplainantCustomer
	t.Cleanup(func() {
		// disputes have no ON DELETE CASCADE, so remove them before complaints.
		_, _ = pool.Exec(ctx, `DELETE FROM disputes WHERE complaint_id IN (SELECT id FROM complaints WHERE complainant_id = $1)`, owner)
		_, _ = pool.Exec(ctx, `DELETE FROM complaints WHERE complainant_id = $1`, owner)
		pool.Close()
	})

	// CreateComplaint — INSERT exercising the new columns (category/priority/
	// identity snapshot) and the 'wallet' enum value (migration 003).
	name, phone := "Ada Okafor", "+2348000000000"
	category := "damaged_goods"
	c, err := repo.CreateComplaint(ctx, CreateComplaintInput{
		ComplainantType: ct, ComplainantID: owner, ComplainantName: &name, ComplainantPhone: &phone,
		ServiceType: supportmodels.ServiceTypeWallet, Category: &category, Priority: supportmodels.PriorityNormal,
		Subject: "Damaged", Description: "crushed box",
	})
	if err != nil {
		t.Fatalf("CreateComplaint: %v", err)
	}
	if c.ID == "" || c.ComplainantName == nil || *c.ComplainantName != name {
		t.Fatalf("CreateComplaint did not round-trip the snapshot: %+v", c)
	}

	// Regression for bug #1: ListComplaintsByComplainant runs the unread
	// subquery that reuses $1 across an enum and a text column.
	if _, err := repo.ListComplaintsByComplainant(ctx, ct, owner, 50, 0); err != nil {
		t.Fatalf("ListComplaintsByComplainant: %v", err)
	}

	// An admin message should count as one unread for the customer, and the list
	// must surface it.
	if _, err := repo.CreateChatMessage(ctx, supportmodels.ChatMessage{ComplaintID: c.ID, SenderType: supportmodels.SenderTypeAdmin, SenderID: "admin", Content: "hi"}); err != nil {
		t.Fatalf("CreateChatMessage: %v", err)
	}
	if n, err := repo.CountUnread(ctx, c.ID, string(ct)); err != nil || n != 1 {
		t.Fatalf("CountUnread = %d, err %v; want 1", n, err)
	}
	list, err := repo.ListComplaintsByComplainant(ctx, ct, owner, 50, 0)
	if err != nil {
		t.Fatalf("ListComplaintsByComplainant(2): %v", err)
	}
	if len(list) != 1 || list[0].UnreadCount != 1 {
		t.Fatalf("expected 1 complaint with unread_count 1, got %+v", list)
	}
	if err := repo.MarkRead(ctx, c.ID, string(ct)); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	if n, _ := repo.CountUnread(ctx, c.ID, string(ct)); n != 0 {
		t.Fatalf("CountUnread after MarkRead = %d, want 0", n)
	}

	// Regression for bug #2: UpdateComplaintStatus casts the status enum and
	// exercises the resolved/closed CASE branch.
	if _, err := repo.UpdateComplaintStatus(ctx, c.ID, supportmodels.ComplaintStatusUnderReview, nil); err != nil {
		t.Fatalf("UpdateComplaintStatus(under_review): %v", err)
	}
	note := "resolved by integration test"
	resolved, err := repo.UpdateComplaintStatus(ctx, c.ID, supportmodels.ComplaintStatusResolved, &note)
	if err != nil {
		t.Fatalf("UpdateComplaintStatus(resolved): %v", err)
	}
	if resolved.Status != supportmodels.ComplaintStatusResolved || resolved.ResolvedAt == nil {
		t.Fatalf("resolved status/resolved_at not set: %+v", resolved)
	}

	// Identity refresh.
	if _, err := repo.UpdateComplaintIdentity(ctx, c.ID, &name, &phone); err != nil {
		t.Fatalf("UpdateComplaintIdentity: %v", err)
	}

	// Evidence round-trip.
	url := "https://x/y.jpg"
	if _, err := repo.AddEvidence(ctx, AddEvidenceInput{ComplaintID: c.ID, UploaderType: ct, UploaderID: owner, MediaURL: &url}); err != nil {
		t.Fatalf("AddEvidence: %v", err)
	}
	if ev, err := repo.ListEvidence(ctx, c.ID); err != nil || len(ev) != 1 {
		t.Fatalf("ListEvidence = %d, err %v; want 1", len(ev), err)
	}

	// Dispute create/read/resolve.
	d, err := repo.CreateDispute(ctx, CreateDisputeInput{
		ComplaintID: c.ID, ServiceType: supportmodels.ServiceTypeWallet,
		RespondentType: supportmodels.ComplainantHaulingProvider, RespondentID: "prov-1",
	})
	if err != nil {
		t.Fatalf("CreateDispute: %v", err)
	}
	if _, err := repo.GetDisputeByComplaintID(ctx, c.ID); err != nil {
		t.Fatalf("GetDisputeByComplaintID: %v", err)
	}
	if _, err := repo.ResolveDispute(ctx, d.ID, supportmodels.DisputeOutcomeFavourComplainant, "ok", "admin"); err != nil {
		t.Fatalf("ResolveDispute: %v", err)
	}

	// Audit events.
	if err := repo.RecordEvent(ctx, c.ID, "admin", "admin", "status_changed", map[string]any{"status": "resolved"}); err != nil {
		t.Fatalf("RecordEvent: %v", err)
	}
	if ev, err := repo.ListEvents(ctx, c.ID); err != nil || len(ev) == 0 {
		t.Fatalf("ListEvents = %d, err %v; want >=1", len(ev), err)
	}

	// Admin filtered lists must execute (dynamic WHERE + ORDER BY priority).
	if _, err := repo.ListComplaints(ctx, ComplaintFilter{ComplainantType: ct}, 50, 0); err != nil {
		t.Fatalf("ListComplaints(filter): %v", err)
	}
	if _, err := repo.ListDisputes(ctx, DisputeFilter{Outcome: supportmodels.DisputeOutcomeFavourComplainant}, 50, 0); err != nil {
		t.Fatalf("ListDisputes(filter): %v", err)
	}

	// Help articles query.
	if _, err := repo.ListHelpArticles(ctx, "customer"); err != nil {
		t.Fatalf("ListHelpArticles: %v", err)
	}
}
