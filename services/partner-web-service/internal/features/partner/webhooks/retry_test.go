package webhooks

import (
	"testing"
	"time"
)

func TestNextRetryDelaySchedule(t *testing.T) {
	cases := []struct {
		attempts  int
		wantDelay time.Duration
		wantOK    bool
	}{
		{0, 0, false}, // no attempt made yet is not a valid input
		{1, 30 * time.Second, true},
		{2, 5 * time.Minute, true},
		{3, 30 * time.Minute, true},
		{4, 2 * time.Hour, true},
		{5, 0, false}, // 5 attempts exhausted -> failed_permanent
		{6, 0, false},
	}
	for _, tc := range cases {
		delay, ok := NextRetryDelay(tc.attempts)
		if ok != tc.wantOK {
			t.Fatalf("NextRetryDelay(%d) ok = %v, want %v", tc.attempts, ok, tc.wantOK)
		}
		if delay != tc.wantDelay {
			t.Fatalf("NextRetryDelay(%d) delay = %v, want %v", tc.attempts, delay, tc.wantDelay)
		}
	}
}
