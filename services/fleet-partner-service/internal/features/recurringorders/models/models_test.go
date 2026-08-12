package models

import (
	"testing"
	"time"
)

func TestRulesAndTimezone(t *testing.T) {
	f, i, n, e := ParseRule(TypeCustom, "FREQ=WEEKLY;INTERVAL=2")
	if e != nil || f != "WEEKLY" || i != 2 || n != "FREQ=WEEKLY;INTERVAL=2" {
		t.Fatalf("%s %d %s %v", f, i, n, e)
	}
	if _, _, _, e = ParseRule(TypeCustom, "FREQ=SECONDLY"); e == nil {
		t.Fatal("invalid RRULE accepted")
	}
	loc, _ := time.LoadLocation("Europe/Rome")
	start := time.Date(2026, 3, 28, 9, 0, 0, 0, loc)
	next, e := Next(start, "Europe/Rome", "DAILY", 1)
	if e != nil || next.In(loc).Hour() != 9 {
		t.Fatalf("DST wall clock drift: %v %v", next, e)
	}
	if OccurrenceKey("x", start) != OccurrenceKey("x", start) {
		t.Fatal("unstable key")
	}
}

func TestDSTNonexistentAndAmbiguousTimesAdvanceDeterministically(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Rome")
	if err != nil {
		t.Fatal(err)
	}
	// The 02:30 wall time does not exist on the spring-forward day. Go's zone
	// rules normalize it deterministically; the recurrence must still advance,
	// produce one stable UTC key, and never move backwards.
	springStart := time.Date(2026, 3, 28, 2, 30, 0, 0, loc)
	spring, err := Next(springStart, "Europe/Rome", "DAILY", 1)
	if err != nil || !spring.After(springStart.UTC()) {
		t.Fatalf("spring=%v err=%v", spring, err)
	}
	if OccurrenceKey("spring", spring) != OccurrenceKey("spring", spring) {
		t.Fatal("spring key changed")
	}

	// 02:30 occurs twice on the fall-back day. The selected instant must be
	// stable and the following recurrence must remain strictly increasing.
	fallStart := time.Date(2026, 10, 24, 2, 30, 0, 0, loc)
	fall, err := Next(fallStart, "Europe/Rome", "DAILY", 1)
	if err != nil || !fall.After(fallStart.UTC()) {
		t.Fatalf("fall=%v err=%v", fall, err)
	}
	again, err := Next(fallStart, "Europe/Rome", "DAILY", 1)
	if err != nil || !again.Equal(fall) {
		t.Fatalf("ambiguous time changed: %v %v", fall, again)
	}
	next, err := Next(fall, "Europe/Rome", "DAILY", 1)
	if err != nil || !next.After(fall) {
		t.Fatalf("post-fall=%v err=%v", next, err)
	}
}
