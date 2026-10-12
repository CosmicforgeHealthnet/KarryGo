package orders

import (
	"testing"
	"time"
)

func TestRecurringOccurrenceCapacityUsesLocalCalendar(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	schedule := RecurringSchedule{BranchID: "branch", Timezone: loc.String(), StartsOn: time.Date(2026, 3, 7, 0, 0, 0, 0, time.UTC), ProductionRequirement: map[string]any{
		"required": true, "branch_id": "branch", "capability": "FOOD",
		"window_start": time.Date(2026, 3, 7, 9, 0, 0, 0, loc),
		"window_end":   time.Date(2026, 3, 7, 11, 0, 0, 0, loc),
		"allocations":  []map[string]any{{"capacity_pool_id": "pool", "quantity": 25}},
		"evidence":     "preserved",
	}}
	for _, day := range []int{7, 8, 9} {
		snapshot, cap, err := recurringOccurrenceCapacity(schedule, time.Date(2026, 3, day, 0, 0, 0, 0, loc))
		if err != nil || cap.WindowStart.In(loc).Day() != day || cap.WindowStart.In(loc).Hour() != 9 || cap.WindowEnd.In(loc).Hour() != 11 || cap.Allocations[0].Quantity != 25 || snapshot["evidence"] != "preserved" {
			t.Fatalf("day %d: cap=%+v snapshot=%v err=%v", day, cap, snapshot, err)
		}
	}
	if schedule.ProductionRequirement["window_start"].(time.Time).In(loc).Day() != 7 {
		t.Fatal("template was mutated")
	}
	schedule.ProductionRequirement["branch_id"] = "another"
	if _, _, err := recurringOccurrenceCapacity(schedule, time.Date(2026, 3, 9, 0, 0, 0, 0, loc)); err == nil {
		t.Fatal("foreign branch capacity accepted")
	}
	delete(schedule.ProductionRequirement, "branch_id")
	schedule.ProductionRequirement["window_start"] = time.Date(2026, 3, 7, 2, 30, 0, 0, loc)
	schedule.ProductionRequirement["window_end"] = time.Date(2026, 3, 7, 3, 30, 0, 0, loc)
	if _, _, err := recurringOccurrenceCapacity(schedule, time.Date(2026, 3, 8, 0, 0, 0, 0, loc)); err == nil {
		t.Fatal("nonexistent DST time silently accepted")
	}
}

func TestRecurringGenerationDatesPreserveDatabaseDates(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	ends := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)
	schedule := RecurringSchedule{StartsOn: time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC), EndsOn: &ends, AdvanceHorizonDays: 14}
	start, end := recurringGenerationDates(schedule, time.Date(2026, 3, 7, 12, 0, 0, 0, loc), loc)
	if start.Day() != 8 || start.Hour() != 0 || end.Day() != 9 || end.Hour() != 0 || start.Location() != loc {
		t.Fatalf("incorrect date range %v to %v", start, end)
	}
	start, _ = recurringGenerationDates(schedule, time.Date(2026, 3, 9, 12, 0, 0, 0, loc), loc)
	if start.Day() != 9 || start.Hour() != 0 {
		t.Fatalf("incorrect current-day start %v", start)
	}
}
