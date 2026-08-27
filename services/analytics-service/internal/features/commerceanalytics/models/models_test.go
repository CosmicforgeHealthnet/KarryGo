package models

import (
	"testing"
	"time"
)

func TestResolveWindowBoundsCustomWindows(t *testing.T) {
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	w, err := ResolveWindow(Window7D, time.Time{}, time.Time{}, now)
	if err != nil {
		t.Fatalf("7D window rejected: %v", err)
	}
	if got := int(w.End.Sub(w.Start).Hours() / 24); got != 7 {
		t.Fatalf("7D window = %d days", got)
	}
	_, err = ResolveWindow(WindowCustom, now, now.Add(-time.Hour), now)
	if err == nil {
		t.Fatal("expected inverted custom window to fail")
	}
	_, err = ResolveWindow(WindowCustom, now.AddDate(-2, 0, 0), now, now)
	if err == nil {
		t.Fatal("expected oversized custom window to fail")
	}
}
