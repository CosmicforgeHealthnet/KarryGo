package orders

import (
	"context"
	"cosmicforge/logistics/services/vendor-service/internal/features/checkoutquote"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"testing"
	"time"
)

func TestRecurringSetupProjectsOnlyFoodAndCheckoutAddress(t *testing.T) {
	now := time.Now().UTC()
	q := SchedulingQuote{QuoteID: "q", CustomerID: "customer", BusinessID: "vendor", BranchID: "branch", Currency: "NGN", QuoteVersion: 2, ApprovedAt: &now, Address: checkoutquote.AddressSnapshot{SourceAddressRef: "address", Address: "Marina"}, Lines: []checkoutquote.LineSnapshot{{BusinessID: "vendor", BranchID: "branch", Domain: "FOOD", ItemRef: "meal", Quantity: 20}}}
	context, err := recurringSetupContext(q, "corporate", "Marina Group")
	if err != nil || context.RequesterID != "customer" || context.DestinationSnapshot["source_address_ref"] != "address" || context.MenuRotation["mode"] != "FIXED" {
		t.Fatalf("invalid context %+v %v", context, err)
	}
	q.Lines[0].BusinessID = "another"
	if _, err = recurringSetupContext(q, "corporate", "Group"); err == nil {
		t.Fatal("foreign menu accepted")
	}
	q.Lines[0].BusinessID = "vendor"
	q.Lines[0].Domain = "SHOP"
	if _, err = recurringSetupContext(q, "corporate", "Group"); err == nil {
		t.Fatal("non-food menu accepted")
	}
	q.Lines[0].Domain = "FOOD"
	q.Address.SourceAddressRef = ""
	if _, err = recurringSetupContext(q, "corporate", "Group"); err == nil {
		t.Fatal("non-authoritative address accepted")
	}
}
func TestRecurringSetupRequiresCorporateReadScope(t *testing.T) {
	resolver := &scheduleReadResolver{scope: Scope{}}
	service := NewSection21Service(Section21Options{Enabled: true, DB: &pgxpool.Pool{}, Scopes: resolver})
	if _, _, err := service.ListRecurringSetupContexts(context.Background(), Actor{StaffID: "staff", VendorID: "vendor"}, ScheduledReadFilter{}); !errors.Is(err, ErrSection21Forbidden) {
		t.Fatalf("empty scope accepted: %v", err)
	}
	if resolver.permission != "vendor.crm.corporate.read" {
		t.Fatal("incorrect corporate permission")
	}
}
func TestRecurringSnapshotComparisonPreservesLargeAmounts(t *testing.T) {
	if equalRecurringSnapshot(map[string]any{"price": int64(9007199254740992)}, map[string]any{"price": int64(9007199254740993)}) {
		t.Fatal("distinct large amounts compared equal")
	}
	if !equalRecurringSnapshot(map[string]any{"a": 1, "b": 2}, map[string]any{"b": 2, "a": 1}) {
		t.Fatal("map ordering changed snapshot equality")
	}
}

func TestRecurringInputKeepsCalendarDateInWesternTimezone(t *testing.T) {
	now := time.Date(2026, 10, 11, 16, 0, 0, 0, time.UTC)
	in := RecurringScheduleInput{BusinessID: "vendor", CorporateRef: "corporate", BranchID: "branch", Weekdays: []int{1}, MealTime: "12:30", Timezone: "America/New_York", Headcount: 20, StartsOn: time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC), AdvanceHorizonDays: 14, IdempotencyKey: "key"}
	if _, _, err := validateRecurringInput(in, now); err != nil {
		t.Fatalf("current local calendar date rejected: %v", err)
	}
	end := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	loc, _ := time.LoadLocation(in.Timezone)
	if nullableDate(&end, loc) != "2026-10-12" {
		t.Fatal("final date moved to previous day")
	}
}
