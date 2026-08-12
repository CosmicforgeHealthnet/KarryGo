package scheduler

import (
	_ "time/tzdata"

	"testing"
	"time"
)

func mustLoc(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("load location %q: %v", name, err)
	}
	return loc
}

func TestParseCronField(t *testing.T) {
	cases := []struct {
		raw     string
		min     int
		max     int
		wantErr bool
	}{
		{"*", 0, 59, false},
		{"5", 0, 59, false},
		{"1,2,3", 0, 59, false},
		{"*/15", 0, 59, false},
		{"*/0", 0, 59, true},
		{"60", 0, 59, true},
		{"-1", 0, 59, true},
		{"", 0, 59, true},
		{"1-5", 0, 59, true}, // ranges deliberately unsupported
	}
	for _, c := range cases {
		_, err := parseCronField(c.raw, c.min, c.max)
		if c.wantErr && err == nil {
			t.Errorf("parseCronField(%q): expected error, got nil", c.raw)
		}
		if !c.wantErr && err != nil {
			t.Errorf("parseCronField(%q): unexpected error: %v", c.raw, err)
		}
	}
}

func TestParseCron_WrongFieldCount(t *testing.T) {
	if _, err := ParseCron("* * * *"); err == nil {
		t.Fatal("expected error for 4-field expression")
	}
	if _, err := ParseCron("* * * * * *"); err == nil {
		t.Fatal("expected error for 6-field expression")
	}
}

func TestCronSchedule_NextEveryMinute(t *testing.T) {
	cron, err := ParseCron("* * * * *")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	after := time.Date(2026, 1, 1, 10, 30, 15, 0, time.UTC)
	next, err := cron.Next(after, time.UTC)
	if err != nil {
		t.Fatalf("next: %v", err)
	}
	want := time.Date(2026, 1, 1, 10, 31, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("got %v, want %v", next, want)
	}
}

func TestCronSchedule_NextDailyAtHour(t *testing.T) {
	cron, err := ParseCron("0 9 * * *")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	loc := time.UTC
	after := time.Date(2026, 3, 10, 9, 0, 0, 0, loc)
	next, err := cron.Next(after, loc)
	if err != nil {
		t.Fatalf("next: %v", err)
	}
	want := time.Date(2026, 3, 11, 9, 0, 0, 0, loc)
	if !next.Equal(want) {
		t.Fatalf("got %v, want %v", next, want)
	}
}

func TestCronSchedule_StepSyntax(t *testing.T) {
	cron, err := ParseCron("*/15 * * * *")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	after := time.Date(2026, 1, 1, 10, 16, 0, 0, time.UTC)
	next, err := cron.Next(after, time.UTC)
	if err != nil {
		t.Fatalf("next: %v", err)
	}
	want := time.Date(2026, 1, 1, 10, 30, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("got %v, want %v", next, want)
	}
}

func TestCronSchedule_DayOfWeek(t *testing.T) {
	// Every Monday (1) at 08:00.
	cron, err := ParseCron("0 8 * * 1")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	// 2026-01-01 is a Thursday.
	after := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	next, err := cron.Next(after, time.UTC)
	if err != nil {
		t.Fatalf("next: %v", err)
	}
	if next.Weekday() != time.Monday || next.Hour() != 8 {
		t.Fatalf("got %v, expected next Monday at 08:00", next)
	}
	want := time.Date(2026, 1, 5, 8, 0, 0, 0, time.UTC) // first Monday on/after Jan 1 2026
	if !next.Equal(want) {
		t.Fatalf("got %v, want %v", next, want)
	}
}

func TestCronSchedule_UnsatisfiableExpressionErrors(t *testing.T) {
	// Feb 30th never exists.
	cron, err := ParseCron("0 0 30 2 *")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	after := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := cron.Next(after, time.UTC); err == nil {
		t.Fatal("expected lookahead-exhausted error for an unsatisfiable expression")
	}
}

// DST forward (spring-forward): America/New_York moves clocks from 1:59:59
// AM EST directly to 3:00:00 AM EDT on 2026-03-08. A schedule that would
// have fired at 2:30 AM local time never occurs that day -- Next must skip
// over the gap deterministically rather than erroring or double-firing.
func TestCronSchedule_DSTForwardGapIsSkipped(t *testing.T) {
	loc := mustLoc(t, "America/New_York")
	cron, err := ParseCron("30 2 * * *")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	after := time.Date(2026, 3, 7, 12, 0, 0, 0, loc)
	next, err := cron.Next(after, loc)
	if err != nil {
		t.Fatalf("next: %v", err)
	}
	// The 2:30 AM occurrence on 2026-03-08 does not exist (clocks jump from
	// 1:59:59 to 3:00:00), so the first real occurrence is 2026-03-09 02:30 EDT.
	nextLocal := next.In(loc)
	if nextLocal.Day() == 8 {
		t.Fatalf("got an occurrence on the gap day itself: %v", nextLocal)
	}
	want := time.Date(2026, 3, 9, 2, 30, 0, 0, loc)
	if !next.Equal(want) {
		t.Fatalf("got %v (%v), want %v", next, nextLocal, want)
	}
}

// DST backward (fall-back): America/New_York moves clocks from 1:59:59 AM
// EDT back to 1:00:00 AM EST on 2026-11-01, so 1:30 AM local time occurs
// twice. Next must resolve deterministically to the first occurrence
// (the EDT one) since that's the first candidate the minute-by-minute walk
// reaches, and it must not fire the same wall-clock target twice for the
// same day when walked forward again.
func TestCronSchedule_DSTBackwardOverlapResolvesToFirstOccurrence(t *testing.T) {
	loc := mustLoc(t, "America/New_York")
	cron, err := ParseCron("30 1 * * *")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	after := time.Date(2026, 10, 31, 12, 0, 0, 0, loc)
	first, err := cron.Next(after, loc)
	if err != nil {
		t.Fatalf("next: %v", err)
	}
	firstLocal := first.In(loc)
	if firstLocal.Day() != 1 || firstLocal.Month() != time.November || firstLocal.Hour() != 1 || firstLocal.Minute() != 30 {
		t.Fatalf("unexpected first occurrence: %v", firstLocal)
	}

	// Walking forward again from just after the first occurrence must not
	// return the same wall-clock instant a second time; it must advance to
	// the next day's occurrence.
	second, err := cron.Next(first, loc)
	if err != nil {
		t.Fatalf("next (second): %v", err)
	}
	secondLocal := second.In(loc)
	if secondLocal.Day() != 2 {
		t.Fatalf("expected the next occurrence to land on Nov 2, got %v", secondLocal)
	}
	if !second.After(first) {
		t.Fatalf("second occurrence %v must be strictly after first %v", second, first)
	}
}

func TestComputeNextRun_Cron(t *testing.T) {
	task := Task{ScheduleType: ScheduleCron, ScheduleExpression: "0 9 * * *", Timezone: "UTC"}
	after := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	next, err := ComputeNextRun(task, after)
	if err != nil {
		t.Fatalf("compute: %v", err)
	}
	want := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	if next == nil || !next.Equal(want) {
		t.Fatalf("got %v, want %v", next, want)
	}
}

func TestComputeNextRun_Interval(t *testing.T) {
	task := Task{ScheduleType: ScheduleInterval, ScheduleExpression: "300", Timezone: "UTC"}
	after := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	next, err := ComputeNextRun(task, after)
	if err != nil {
		t.Fatalf("compute: %v", err)
	}
	want := after.Add(300 * time.Second)
	if next == nil || !next.Equal(want) {
		t.Fatalf("got %v, want %v", next, want)
	}
}

func TestComputeNextRun_IntervalInvalid(t *testing.T) {
	task := Task{ScheduleType: ScheduleInterval, ScheduleExpression: "not-a-number", Timezone: "UTC"}
	if _, err := ComputeNextRun(task, time.Now()); err == nil {
		t.Fatal("expected error for non-numeric interval")
	}
	task.ScheduleExpression = "-5"
	if _, err := ComputeNextRun(task, time.Now()); err == nil {
		t.Fatal("expected error for non-positive interval")
	}
}

func TestComputeNextRun_OnceFutureAndPast(t *testing.T) {
	future := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	task := Task{ScheduleType: ScheduleOnce, ScheduleExpression: future.Format(time.RFC3339), Timezone: "UTC"}

	before := future.Add(-time.Hour)
	next, err := ComputeNextRun(task, before)
	if err != nil {
		t.Fatalf("compute: %v", err)
	}
	if next == nil || !next.Equal(future) {
		t.Fatalf("got %v, want %v", next, future)
	}

	// Once the target instant has passed, there is no further occurrence.
	after := future.Add(time.Hour)
	next, err = ComputeNextRun(task, after)
	if err != nil {
		t.Fatalf("compute: %v", err)
	}
	if next != nil {
		t.Fatalf("expected nil (no further occurrences), got %v", next)
	}
}

func TestComputeNextRun_UnknownScheduleType(t *testing.T) {
	task := Task{ScheduleType: "WEEKLY", ScheduleExpression: "x", Timezone: "UTC"}
	if _, err := ComputeNextRun(task, time.Now()); err == nil {
		t.Fatal("expected error for unknown schedule type")
	}
}

func TestComputeNextRun_InvalidTimezone(t *testing.T) {
	task := Task{ScheduleType: ScheduleCron, ScheduleExpression: "* * * * *", Timezone: "Not/AZone"}
	if _, err := ComputeNextRun(task, time.Now()); err == nil {
		t.Fatal("expected error for invalid timezone")
	}
}
