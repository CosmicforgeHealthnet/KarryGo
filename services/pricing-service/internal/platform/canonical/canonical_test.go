package canonical

import (
	"strings"
	"testing"
	"time"
)

func TestKeyOrderDoesNotChangeTheSeal(t *testing.T) {
	// Two encodings of the same record with fields declared in a different
	// order must seal identically, or a quote's hash would depend on the Go
	// struct layout at the time it was written.
	a := map[string]any{"base": 30_000, "distance": 120_000, "class": "economy"}
	b := map[string]any{"class": "economy", "distance": 120_000, "base": 30_000}

	hashA, err := Hash(a)
	if err != nil {
		t.Fatalf("hash a: %v", err)
	}
	hashB, err := Hash(b)
	if err != nil {
		t.Fatalf("hash b: %v", err)
	}
	if hashA != hashB {
		t.Fatalf("key order changed the seal: %s vs %s", hashA, hashB)
	}

	encoded, err := Marshal(a)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if got := string(encoded); got != `{"base":30000,"class":"economy","distance":120000}` {
		t.Fatalf("canonical form = %s", got)
	}
}

func TestHashIsStableAndSensitive(t *testing.T) {
	type quote struct {
		QuoteID  string `json:"quote_id"`
		GrossKob int64  `json:"gross_kobo"`
	}

	first, err := Hash(quote{QuoteID: "pq_1", GrossKob: 170_000})
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	again, err := Hash(quote{QuoteID: "pq_1", GrossKob: 170_000})
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if first != again {
		t.Fatal("the same record sealed differently twice")
	}
	if len(first) != 64 {
		t.Fatalf("hash %q is not a 64-character hex digest", first)
	}

	// One kobo of difference must change the seal, otherwise tampering with an
	// amount would go undetected.
	altered, err := Hash(quote{QuoteID: "pq_1", GrossKob: 170_001})
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if altered == first {
		t.Fatal("changing the amount by one kobo did not change the seal")
	}
}

func TestFloatsAreRefused(t *testing.T) {
	cases := []struct {
		name  string
		value any
	}{
		{"a bare float", map[string]any{"fare": 1700.50}},
		{"a multiplier", map[string]any{"express": 1.35}},
		{"nested in a slice", map[string]any{"legs": []any{map[string]any{"km": 5.5}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Hash(tc.value)
			if err == nil {
				t.Fatal("a floating-point value must be refused, not rounded")
			}
			if !strings.Contains(err.Error(), "whole kobo") {
				t.Fatalf("error should explain the rule, got: %v", err)
			}
		})
	}
}

func TestFloatErrorNamesTheField(t *testing.T) {
	_, err := Hash(map[string]any{"route": map[string]any{"distance_km": 5.5}})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "route.distance_km") {
		t.Fatalf("error should name the offending field, got: %v", err)
	}
}

func TestIntegralFloatsSealAsTheEquivalentInteger(t *testing.T) {
	// A float64 holding 5.0 is exact, and Go encodes it as "5", so it seals
	// identically to the integer 5. That is the property we want: the rule is
	// "no AMBIGUOUS number", not "no value that was once a float". What must
	// never happen is 5 and 5.0 producing different seals for the same record.
	fromFloat, err := Hash(map[string]any{"stops": 5.0})
	if err != nil {
		t.Fatalf("an exactly-integral float should seal as an integer: %v", err)
	}
	fromInt, err := Hash(map[string]any{"stops": 5})
	if err != nil {
		t.Fatalf("hash int: %v", err)
	}
	if fromFloat != fromInt {
		t.Fatal("5.0 and 5 sealed differently")
	}

	// A float too large to write without an exponent stays ambiguous and is
	// refused, because "1e+21" has no single integer spelling.
	if _, err := Hash(map[string]any{"amount": 1e21}); err == nil {
		t.Fatal("an exponent-formatted number must be refused")
	}

	// The classic inexact case must be refused.
	if _, err := Hash(map[string]any{"amount": 0.1 + 0.2}); err == nil {
		t.Fatal("0.30000000000000004 must be refused")
	}
}

func TestTimeIsAlwaysUTC(t *testing.T) {
	lagos := time.FixedZone("WAT", 1*60*60)
	instant := time.Date(2026, 9, 29, 15, 4, 5, 0, lagos)

	got := Time(instant)
	if !strings.HasSuffix(got, "Z") {
		t.Fatalf("Time(%v) = %q, want a UTC (Z) timestamp", instant, got)
	}
	if got != "2026-09-29T14:04:05Z" {
		t.Fatalf("Time = %q, want 2026-09-29T14:04:05Z", got)
	}
	// The same instant in two zones must seal identically.
	if Time(instant) != Time(instant.UTC()) {
		t.Fatal("the same instant sealed differently in two zones")
	}
}

func TestMicroRoundsCoordinatesToAFixedGrid(t *testing.T) {
	cases := []struct {
		degrees float64
		want    int64
	}{
		{6.5244, 6_524_400},
		{3.3792, 3_379_200},
		{-6.5244, -6_524_400},
		{0, 0},
		{6.52440049, 6_524_400}, // below the grid, rounds down
		{6.5244005, 6_524_401},  // half rounds away from zero
	}
	for _, tc := range cases {
		if got := Micro(tc.degrees); got != tc.want {
			t.Errorf("Micro(%v) = %d, want %d", tc.degrees, got, tc.want)
		}
	}

	// Two doubles that differ below the grid must seal identically.
	if Micro(6.524400001) != Micro(6.524400002) {
		t.Fatal("imperceptibly different coordinates produced different grid points")
	}
}

func TestCombineDistinguishesPartLists(t *testing.T) {
	a := Combine("aa", "bb")
	b := Combine("aab", "b")
	if a == b {
		t.Fatal("different part lists must not combine to the same seal")
	}
	if a != Combine("aa", "bb") {
		t.Fatal("Combine is not stable")
	}
	if len(a) != 64 {
		t.Fatalf("Combine returned %d characters, want 64", len(a))
	}
}
