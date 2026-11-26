package models

import (
	"testing"
	"time"
)

func TestAgeingBucketsAndCurrencyIsolation(t *testing.T) {
	cases := map[int]string{-3: "CURRENT", 0: "CURRENT", 1: "DAYS_1_30", 30: "DAYS_1_30", 31: "DAYS_31_60", 61: "DAYS_61_90", 90: "DAYS_61_90", 91: "DAYS_OVER_90"}
	for days, want := range cases {
		if got := AgeingBucketFor(days); got != want {
			t.Errorf("AgeingBucketFor(%d)=%s want %s", days, got, want)
		}
	}
	r := BuildAgeing(time.Now(), []AgeingRow{
		{CustomerID: "c1", Currency: "NGN", OutstandingMinor: 1000, DaysPastDue: 0},
		{CustomerID: "c1", Currency: "NGN", OutstandingMinor: 500, DaysPastDue: 45},
		{CustomerID: "c2", Currency: "NGN", OutstandingMinor: 200, DaysPastDue: 100},
		{CustomerID: "c1", Currency: "USD", OutstandingMinor: 70, DaysPastDue: 10},
	})
	if len(r.Currencies) != 2 || r.Currencies[0].Currency != "NGN" || r.Currencies[1].Currency != "USD" {
		t.Fatalf("currencies = %+v", r.Currencies)
	}
	ngn := r.Currencies[0]
	if ngn.OutstandingMinor != 1700 || ngn.Buckets[0].OutstandingMinor != 1000 || ngn.Buckets[2].OutstandingMinor != 500 || ngn.Buckets[4].OutstandingMinor != 200 {
		t.Fatalf("NGN ageing = %+v", ngn)
	}
	if r.Currencies[1].OutstandingMinor != 70 {
		t.Fatalf("USD mixed with NGN: %+v", r.Currencies[1])
	}
	if len(ngn.Customers) != 2 || ngn.Customers[0].OutstandingMinor != 1500 {
		t.Fatalf("customer ageing = %+v", ngn.Customers)
	}
}

func TestProfitabilityMarginAndSplitEven(t *testing.T) {
	lines := BuildProfitability([]ProfitabilityFact{
		{Kind: "REVENUE", DimensionID: "b1", Currency: "NGN", AmountMinor: 10000, Sources: 2},
		{Kind: "DIRECT_COST", DimensionID: "b1", Currency: "NGN", AmountMinor: 3000, Sources: 1},
		{Kind: "ALLOCATED_COST", DimensionID: "b1", Currency: "NGN", AmountMinor: 1001, Sources: 1},
		{Kind: "REVENUE", DimensionID: "b1", Currency: "USD", AmountMinor: 50, Sources: 1},
	})
	if len(lines) != 2 {
		t.Fatalf("lines = %+v", lines)
	}
	if l := lines[0]; l.Currency != "NGN" || l.MarginMinor != 5999 || l.RevenueSources != 2 {
		t.Fatalf("NGN line = %+v", l)
	}
	if lines[1].MarginMinor != 50 {
		t.Fatalf("USD line = %+v", lines[1])
	}
	parts := SplitEven(1001, 3)
	if parts[0]+parts[1]+parts[2] != 1001 || parts[0] != 334 || parts[2] != 333 {
		t.Fatalf("SplitEven = %v", parts)
	}
}
