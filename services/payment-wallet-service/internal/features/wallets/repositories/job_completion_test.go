package walletrepositories

import (
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/referralcontract"
)

func TestResolveJobCompletedAt(t *testing.T) {
	created := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	settled := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		name       string
		reported   time.Time
		wantAt     time.Time
		wantSource string
	}{
		{"missing falls back to settlement", time.Time{}, settled, referralcontract.CompletedAtFromSettlement},
		{"normal report is trusted", settled.Add(-30 * time.Minute), settled.Add(-30 * time.Minute), referralcontract.CompletedAtFromProvider},
		{"small future skew is clamped", settled.Add(2 * time.Minute), settled, referralcontract.CompletedAtFromProvider},
		{"far future is not trusted", settled.Add(time.Hour), settled, referralcontract.CompletedAtFromSettlement},
		{"before the payment existed is not trusted", created.Add(-time.Hour), settled, referralcontract.CompletedAtFromSettlement},
		{"just before creation within skew is trusted", created.Add(-time.Minute), created.Add(-time.Minute), referralcontract.CompletedAtFromProvider},
	}
	for _, tc := range cases {
		gotAt, gotSource := resolveJobCompletedAt(tc.reported, created, settled)
		if !gotAt.Equal(tc.wantAt) || gotSource != tc.wantSource {
			t.Fatalf("%s: got (%s, %s), want (%s, %s)", tc.name, gotAt, gotSource, tc.wantAt, tc.wantSource)
		}
	}
}
