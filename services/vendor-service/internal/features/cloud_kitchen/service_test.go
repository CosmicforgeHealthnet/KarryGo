package cloudkitchen

import (
	"testing"
	"time"
)

func TestMatchesRecurrence(t *testing.T) {
	from := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC) // Monday
	if !matchesRecurrence("DAILY", nil, from, from.AddDate(0, 0, 1)) {
		t.Fatal("daily recurrence did not match")
	}
	if !matchesRecurrence("WEEKLY", nil, from, from.AddDate(0, 0, 7)) {
		t.Fatal("weekly recurrence did not match")
	}
	if matchesRecurrence("WEEKLY", nil, from, from.AddDate(0, 0, 1)) {
		t.Fatal("weekly recurrence matched wrong weekday")
	}
	if !matchesRecurrence("SELECTED_WEEKDAYS", []int16{1, 3}, from, from.AddDate(0, 0, 2)) {
		t.Fatal("selected weekday did not match")
	}
}
