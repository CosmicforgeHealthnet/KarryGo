package orders

import (
	"context"
	"cosmicforge/logistics/services/vendor-service/internal/features/checkoutquote"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"testing"
	"time"
)

func TestSchedulingQuoteProjectionRequiresApprovalAndBranchBinding(t *testing.T) {
	now := time.Now().UTC()
	q := checkoutquote.Quote{ID: "q-a", CustomerID: "customer-a", State: checkoutquote.StateApproved, ApprovedAt: &now,
		Lines: []checkoutquote.LineSnapshot{{BusinessID: "vendor-a", BranchID: "branch-a", ItemRef: "own-item"}, {BusinessID: "vendor-b", BranchID: "branch-b", ItemRef: "private-item"}}}
	if approvedQuoteContainsBranch(q, "vendor-a", "branch-b") {
		t.Fatal("foreign branch quote accepted")
	}
	if approvedQuoteContainsBranch(q, "vendor-b", "branch-a") {
		t.Fatal("foreign vendor quote accepted")
	}
	projected, err := schedulingQuote(q, "vendor-a", "branch-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(projected.Lines) != 1 || projected.Lines[0].ItemRef != "own-item" {
		t.Fatal("other vendor lines exposed")
	}
	q.ApprovedAt = nil
	if approvedQuoteContainsBranch(q, "vendor-a", "branch-a") {
		t.Fatal("missing approval accepted")
	}
	q.ApprovedAt = &now
	q.State = checkoutquote.StateExpired
	if approvedQuoteContainsBranch(q, "vendor-a", "branch-a") {
		t.Fatal("expired quote accepted")
	}
}
func TestSchedulingQuoteReadsRequireReadScope(t *testing.T) {
	resolver := &scheduleReadResolver{scope: Scope{BranchIDs: []string{"branch-a"}}}
	s := NewSection21Service(Section21Options{Enabled: true, DB: &pgxpool.Pool{}, Scopes: resolver})
	actor := Actor{StaffID: "staff-a", VendorID: "vendor-a"}
	if _, err := s.ListSchedulingQuotes(context.Background(), actor, ScheduledReadFilter{BranchID: "branch-b"}); !errors.Is(err, ErrSection21Forbidden) {
		t.Fatalf("foreign branch permitted: %v", err)
	}
	if _, err := s.GetSchedulingQuote(context.Background(), actor, "q-a", "branch-b"); !errors.Is(err, ErrSection21Forbidden) {
		t.Fatalf("foreign branch detail permitted: %v", err)
	}
	if resolver.permission != PermOrdersRead {
		t.Fatal("incorrect permission")
	}
}
