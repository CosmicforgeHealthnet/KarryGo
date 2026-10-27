package models

import (
	"testing"
	"time"
)

func TestMedianDropsOutliers(t *testing.T) {
	// Ten honest reports around ₦950 and two typos.
	prices := []int64{94_000, 95_000, 95_000, 96_000, 94_500, 95_500, 95_000, 97_000, 93_000, 95_000, 9_500, 950_000}
	median, used, dropped := Median(prices)
	if median != 95_000 || used != 10 || dropped != 2 {
		t.Fatalf("median=%d used=%d dropped=%d", median, used, dropped)
	}
	if m, u, d := Median(nil); m != 0 || u != 0 || d != 0 {
		t.Fatal("empty")
	}
	if m, _, _ := Median([]int64{90_000, 100_000}); m != 95_000 {
		t.Fatalf("even median %d", m)
	}
}

func TestDayIsLagosCalendarDay(t *testing.T) {
	// 23:30 UTC on the 27th is 00:30 on the 28th in Lagos.
	got := Day(time.Date(2026, 10, 27, 23, 30, 0, 0, time.UTC))
	if !got.Equal(time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("day = %v", got)
	}
}
