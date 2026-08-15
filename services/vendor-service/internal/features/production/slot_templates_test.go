package production

import (
	"testing"
	"time"
)

func TestRuleAppliesUsesBranchLocalClock(t *testing.T) {
	location, err := time.LoadLocation("Europe/Rome")
	if err != nil {
		t.Fatal(err)
	}
	clockStart := time.Date(1, 1, 1, 8, 0, 0, 0, time.UTC)
	clockEnd := time.Date(1, 1, 1, 10, 0, 0, 0, time.UTC)
	windowStart := time.Date(2026, 8, 17, 7, 30, 0, 0, time.UTC)
	if !ruleApplies([]int16{1}, &clockStart, &clockEnd, nil, nil, windowStart, windowStart.Add(15*time.Minute), location) {
		t.Fatal("expected Monday local-time rule to apply")
	}
	sunday := windowStart.AddDate(0, 0, -1)
	if ruleApplies([]int16{1}, &clockStart, &clockEnd, nil, nil, sunday, sunday.Add(15*time.Minute), location) {
		t.Fatal("local weekday rule matched the wrong day")
	}
}

func TestSlotTemplateValidationRequiresIANAAndUniqueWeekdays(t *testing.T) {
	in := slotTemplateRequest{BranchID: "br_1", Capability: "CLOUD_KITCHEN", CapacityPoolID: "pool_1", Label: "Breakfast", DaysOfWeek: []int16{1, 2, 3}, StartLocalTime: "22:00", EndLocalTime: "02:00", Timezone: "Africa/Lagos", GenerationHorizon: 90}
	if err := validateTemplate(in); err != nil {
		t.Fatalf("valid overnight template rejected: %v", err)
	}
	in.DaysOfWeek = []int16{1, 1}
	if err := validateTemplate(in); err == nil {
		t.Fatal("duplicate weekdays accepted")
	}
	in.DaysOfWeek = []int16{1}
	in.Timezone = "Mars/Olympus"
	if err := validateTemplate(in); err == nil {
		t.Fatal("invalid IANA timezone accepted")
	}
}

func TestSlotTemplateValidationBoundsGenerationHorizon(t *testing.T) {
	in := slotTemplateRequest{BranchID: "br_1", Capability: "PASTRY_BAKERY", CapacityPoolID: "pool_1", Label: "Oven", DaysOfWeek: []int16{1}, StartLocalTime: "09:00", EndLocalTime: "10:00", Timezone: "UTC", GenerationHorizon: 367}
	if err := validateTemplate(in); err == nil {
		t.Fatal("unbounded generation horizon accepted")
	}
}
