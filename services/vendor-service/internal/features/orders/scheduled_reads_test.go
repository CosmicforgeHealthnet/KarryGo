package orders

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestScheduleViewsAreWhitelistedBeforePagination(t *testing.T) {
	for _, test := range []struct {
		view      string
		recurring bool
		condition string
	}{
		{"UPCOMING", false, "desired_delivery_at>=now()"},
		{"PAST", false, "desired_delivery_at<now()"},
		{"CANCELLED", false, "scheduling_status='CANCELLED'"},
		{"ACTIVE", true, "status='ACTIVE'"},
		{"PAUSED", true, "status='PAUSED'"},
	} {
		predicate, err := scheduleViewPredicate(test.view, test.recurring)
		if err != nil || !strings.Contains(predicate, test.condition) {
			t.Fatalf("view=%s recurring=%v predicate=%q err=%v", test.view, test.recurring, predicate, err)
		}
	}
	for _, recurring := range []bool{false, true} {
		if predicate, err := scheduleViewPredicate("ALL", recurring); err != nil || predicate != "" {
			t.Fatalf("all view must preserve the base scope: %q %v", predicate, err)
		}
		if _, err := scheduleViewPredicate("ALL' OR true --", recurring); !errors.Is(err, errScheduleView) {
			t.Fatalf("unrecognized filter accepted: %v", err)
		}
	}
	if _, err := scheduleViewPredicate("UPCOMING", true); !errors.Is(err, errScheduleView) {
		t.Fatal("recurring schedules accepted a delivery-time filter")
	}
}

type scheduleReadResolver struct {
	scope      Scope
	permission string
}

func (r *scheduleReadResolver) ResolveScope(_ context.Context, _ string, p string) (Scope, error) {
	r.permission = p
	return r.scope, nil
}

func TestScheduledReadsRespectPermissionAndBranchScope(t *testing.T) {
	r := &scheduleReadResolver{scope: Scope{BranchIDs: []string{"branch-a"}}}
	s := NewSection21Service(Section21Options{Enabled: true, DB: &pgxpool.Pool{}, Scopes: r})
	actor := Actor{VendorID: "vendor-a", StaffID: "staff-a"}
	if _, err := s.scheduledReadScope(context.Background(), actor, "branch-a"); err != nil {
		t.Fatal(err)
	}
	if r.permission != PermOrdersRead {
		t.Fatalf("read asked for %s", r.permission)
	}
	if _, err := s.scheduledReadScope(context.Background(), actor, "branch-b"); !errors.Is(err, ErrSection21Forbidden) {
		t.Fatalf("foreign branch allowed: %v", err)
	}
	r.scope = Scope{}
	if _, err := s.scheduledReadScope(context.Background(), actor, ""); !errors.Is(err, ErrSection21Forbidden) {
		t.Fatalf("empty scope allowed: %v", err)
	}
	r.scope = Scope{Global: true}
	if _, err := s.scheduledReadScope(context.Background(), Actor{StaffID: "staff-a"}, ""); !errors.Is(err, ErrSection21Forbidden) {
		t.Fatalf("missing vendor allowed: %v", err)
	}
	s.enabled = false
	if _, err := s.scheduledReadScope(context.Background(), actor, ""); !errors.Is(err, ErrSection21Disabled) {
		t.Fatalf("disabled feature allowed: %v", err)
	}
}

func TestScheduledEmbeddedRequestsRetainActualFields(t *testing.T) {
	var in scheduledOrderRequest
	if err := json.Unmarshal([]byte(`{"CustomerID":"customer-a","QuoteID":"quote-a","BranchID":"branch-a","DesiredDeliveryAt":"2026-12-01T12:00:00Z","IdempotencyKey":"key-a"}`), &in); err != nil {
		t.Fatal(err)
	}
	if in.CustomerID != "customer-a" || in.QuoteID != "quote-a" || in.DesiredDeliveryAt.IsZero() {
		t.Fatalf("lost embedded fields: %+v", in)
	}
	var recurring recurringScheduleRequest
	if err := json.Unmarshal([]byte(`{"CorporateRef":"corp-a","BranchID":"branch-a","Weekdays":[1,3],"Headcount":80,"MealTime":"12:30","Timezone":"Africa/Lagos"}`), &recurring); err != nil {
		t.Fatal(err)
	}
	if recurring.CorporateRef != "corp-a" || len(recurring.Weekdays) != 2 || recurring.Headcount != 80 {
		t.Fatalf("lost recurring input: %+v", recurring)
	}
}

func TestVendorRFQReadsRequireOrdersReadScope(t *testing.T) {
	r := &scheduleReadResolver{scope: Scope{}}
	s := NewSection21Service(Section21Options{Enabled: true, DB: &pgxpool.Pool{}, Scopes: r})
	actor := Actor{VendorID: "vendor-a", StaffID: "staff-a"}
	if _, err := s.ListVendorRFQs(context.Background(), actor, ScheduledReadFilter{}); !errors.Is(err, ErrSection21Forbidden) {
		t.Fatalf("unscoped inbox allowed: %v", err)
	}
	if _, err := s.GetVendorRFQ(context.Background(), actor, "rfq-a"); !errors.Is(err, ErrSection21Forbidden) {
		t.Fatalf("unscoped detail allowed: %v", err)
	}
	if _, err := s.ListVendorRFQQuotes(context.Background(), actor, "rfq-a"); !errors.Is(err, ErrSection21Forbidden) {
		t.Fatalf("unscoped quotes allowed: %v", err)
	}
	if r.permission != PermOrdersRead {
		t.Fatalf("incorrect permission: %s", r.permission)
	}
}
