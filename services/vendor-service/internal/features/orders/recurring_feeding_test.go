package orders

import (
	"testing"
	"time"
)

func TestRecurringScheduleValidatesTimezoneWeekdaysAndVehicleMetadata(t *testing.T) {
	now := time.Date(2026, 1, 5, 12, 0, 0, 0, time.UTC)
	in := RecurringScheduleInput{BusinessID: "b", CorporateRef: "corp", BranchID: "br", Weekdays: []int{1, 3, 5}, MealTime: "12:30", Timezone: "Africa/Lagos", Headcount: 80, AllowanceMinor: 7500, StartsOn: now, IdempotencyKey: "k", TentativeVehicleClass: "VAN"}
	loc, meal, err := validateRecurringInput(in, now)
	if err != nil || loc.String() != "Africa/Lagos" || meal != "12:30" {
		t.Fatalf("valid schedule rejected: loc=%v meal=%s err=%v", loc, meal, err)
	}
	in.Weekdays = []int{1, 1}
	if _, _, err = validateRecurringInput(in, now); err == nil {
		t.Fatal("duplicate weekdays must fail")
	}
	in.Weekdays = []int{1}
	in.Timezone = "Not/AZone"
	if _, _, err = validateRecurringInput(in, now); err == nil {
		t.Fatal("unknown timezone must fail")
	}
}

func TestRecurringOccurrenceKeyIncludesTimezoneAndLocalDate(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	occurrence := time.Date(2026, 3, 9, 12, 0, 0, 0, loc)
	key := occurrence.Format("2006-01-02T15:04") + "@" + loc.String()
	if key != "2026-03-09T12:00@America/New_York" {
		t.Fatalf("unexpected occurrence key %q", key)
	}
}

func TestValidateRecurringInputAcceptsEchoedMealTimeWithSeconds(t *testing.T) {
	now := time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)
	in := RecurringScheduleInput{BusinessID: "b", CorporateRef: "c", BranchID: "br", IdempotencyKey: "k", Headcount: 5, Weekdays: []int{1}, MealTime: "12:30:00", Timezone: "Africa/Lagos", StartsOn: now.AddDate(0, 0, 1)}
	_, meal, err := validateRecurringInput(in, now)
	if err != nil || meal != "12:30" {
		t.Fatalf("meal=%q err=%v", meal, err)
	}
}
