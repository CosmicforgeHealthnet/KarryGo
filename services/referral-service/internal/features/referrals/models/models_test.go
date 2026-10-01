package referralmodels

import (
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/referralcontract"
)

func TestIsActive(t *testing.T) {
	rules := ActivityRules{CustomerThreshold: 5, ProviderThreshold: 10, Window: 30 * 24 * time.Hour}
	cases := []struct {
		counts ActivityCounts
		want   bool
	}{
		{ActivityCounts{CustomerJobs: 4}, false},
		{ActivityCounts{CustomerJobs: 5}, true},
		{ActivityCounts{ProviderJobs: 9}, false},
		{ActivityCounts{ProviderJobs: 10}, true},
		// Someone with both accounts qualifies through either side, but the
		// two sides are not added together.
		{ActivityCounts{CustomerJobs: 4, ProviderJobs: 9}, false},
		{ActivityCounts{CustomerJobs: 0, ProviderJobs: 12}, true},
	}
	for _, tc := range cases {
		if got := rules.IsActive(tc.counts); got != tc.want {
			t.Errorf("IsActive(%+v) = %v, want %v", tc.counts, got, tc.want)
		}
	}
}

func TestSettledJobFromMarksOnlyDriverJobsInProgramme(t *testing.T) {
	base := referralcontract.JobSettled{SourceService: "s", SourceReference: "r", CustomerID: "c", ProviderID: "p", AmountKobo: 100, JobCompletedAt: time.Now()}
	for providerType, want := range map[string]string{"truck_provider": "hauling", "dispatch": "dispatch", "taxi": "taxi", "vendor_business": ""} {
		payload := base
		payload.ProviderType = providerType
		job := SettledJobFrom(payload)
		if job.ProviderActorType != want || job.InProgramme != (want != "") {
			t.Errorf("%s: actor=%q inProgramme=%v", providerType, job.ProviderActorType, job.InProgramme)
		}
	}
}
