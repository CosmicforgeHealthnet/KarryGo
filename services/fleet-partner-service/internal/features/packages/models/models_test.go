package models

import (
	"testing"
	"time"
)

func TestPackageTransitionMatrix(t *testing.T) {
	valid := [][2]string{{StatusCreated, StatusReceived}, {StatusReceived, StatusSorted}, {StatusSorted, StatusLoaded}, {StatusLoaded, StatusInTransit}, {StatusInTransit, StatusOutForDelivery}, {StatusOutForDelivery, StatusDelivered}, {StatusInTransit, StatusFailedAttempt}, {StatusFailedAttempt, StatusReturning}, {StatusReturning, StatusReturned}}
	for _, edge := range valid {
		if !CanTransition(edge[0], edge[1]) {
			t.Fatalf("expected %s -> %s", edge[0], edge[1])
		}
	}
	if CanTransition(StatusCreated, StatusDelivered) || CanTransition(StatusDelivered, StatusCreated) {
		t.Fatal("invalid shortcut/terminal transition allowed")
	}
	for _, state := range []string{StatusCreated, StatusReceived, StatusSorted, StatusLoaded, StatusInTransit, StatusOutForDelivery, StatusFailedAttempt, StatusReturning} {
		if !CanTransition(state, StatusDamaged) || !CanTransition(state, StatusLost) || !CanTransition(state, StatusQuarantined) {
			t.Fatalf("exceptional transition missing from %s", state)
		}
	}
}

func TestDecimalAndFingerprint(t *testing.T) {
	for _, v := range []string{"0.001", "1", "123.456"} {
		if !ValidPositiveDecimal(v) {
			t.Fatalf("valid decimal rejected: %s", v)
		}
	}
	for _, v := range []string{"0", "-1", "1.2345", "NaN"} {
		if ValidPositiveDecimal(v) {
			t.Fatalf("invalid decimal accepted: %s", v)
		}
	}
	now := time.Date(2026, 7, 25, 13, 0, 0, 0, time.UTC)
	a := ScanFingerprint("p", "LOADED", "b", "", "l", "d", "c", now)
	b := ScanFingerprint("p", "LOADED", "b", "", "l", "d", "c", now)
	c := ScanFingerprint("p", "LOADED", "changed", "", "l", "d", "c", now)
	if a != b || a == c {
		t.Fatal("fingerprint is not stable or change-sensitive")
	}
}
